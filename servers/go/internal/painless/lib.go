package painless

import (
	"encoding/base64"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
)

// Painless engine: the class library (whitelisted methods and fields).

// DocValues is org.opensearch.index.fielddata.ScriptDocValues: Longs,
// Doubles, Strings, Booleans, Dates or GeoPoints.
type DocValues struct {
	Kind   string
	Field  string
	Values []any
}

// DocMap is the doc variable (LeafDocLookup).
type DocMap struct {
	Get func(field string, off int) (any, *RuntimeError)
	Has func(field string) bool
}

type GeoPoint struct{ Lat, Lon float64 }

type plStream struct {
	items []any
	kind  string // obj, int, long, double
}

type plOptional struct {
	name    string
	v       any
	present bool
}

type plCollector struct {
	kind                string
	sep, prefix, suffix string
	f1, f2              any
}

func plDynMethodErr(recv any, name string, n int, off int) *RuntimeError {
	cls, _ := plRuntimeClass(recv)
	return plIAE(off, "dynamic method ["+cls+", "+name+"/"+itoa(n)+"] not found")
}

func plUTF16(s string) []uint16 { return utf16.Encode([]rune(s)) }

func plFromUTF16(u []uint16) string { return string(utf16.Decode(u)) }

// ---- fields / indexing ----

func plGetField(obj any, name string, off int) (any, *RuntimeError) {
	switch t := obj.(type) {
	case *Map:
		v, _ := t.Get(name)
		return v, nil
	case *DocMap:
		return t.Get(name, off)
	case *Array:
		if name == "length" {
			return int32(len(t.Items)), nil
		}
	case *List:
		switch name {
		case "length":
			return int32(len(t.a)), nil
		case "empty":
			return len(t.a) == 0, nil
		}
		if i, err := strconv.Atoi(name); err == nil {
			return plIndexGet(t, int32(i), off)
		}
	case *DocValues:
		switch name {
		case "value":
			return t.value(off)
		case "length":
			return int32(len(t.Values)), nil
		case "empty":
			return len(t.Values) == 0, nil
		case "lat", "lon":
			if t.Kind == "GeoPoints" {
				v, err := t.value(off)
				if err != nil {
					return nil, err
				}
				g := v.(*GeoPoint)
				if name == "lat" {
					return g.Lat, nil
				}
				return g.Lon, nil
			}
		}
	case *GeoPoint:
		switch name {
		case "lat":
			return t.Lat, nil
		case "lon":
			return t.Lon, nil
		}
	case *plMapEntryRef:
		switch name {
		case "key":
			return t.e.Key, nil
		case "value":
			return t.e.Value, nil
		}
	}
	// getter shortcut: x.foo -> x.getFoo() / x.isFoo()
	if name != "" {
		capName := strings.ToUpper(name[:1]) + name[1:]
		for _, m := range []string{"get" + capName, "is" + capName} {
			if v, err := plCallKnown(obj, m, off); err == nil || err.Type != "_unknown" {
				return v, err
			}
		}
	}
	cls, _ := plRuntimeClass(obj)
	return nil, plIAE(off, "dynamic getter ["+cls+", "+name+"] not found")
}

// plCallKnown calls a zero-argument method, reporting unknown methods with
// the internal "_unknown" error type.
func plCallKnown(obj any, name string, off int) (any, *RuntimeError) {
	in := &plInterp{info: &plInfo{}}
	v, err := in.callMethod(obj, name, nil, off)
	if err != nil && strings.HasPrefix(err.Reason, "dynamic method [") {
		return nil, &RuntimeError{Type: "_unknown"}
	}
	return v, err
}

func plSetField(obj any, name string, v any, off int) *RuntimeError {
	switch t := obj.(type) {
	case *Map:
		t.Put(name, v)
		return nil
	case *List:
		if i, err := strconv.Atoi(name); err == nil {
			return plIndexSet(t, int32(i), v, off)
		}
	}
	cls, _ := plRuntimeClass(obj)
	return plIAE(off, "dynamic setter ["+cls+", "+name+"] not found")
}

func plListIndex(idx any, n int, off int) (int, *RuntimeError) {
	if !IsNumber(idx) {
		return 0, &RuntimeError{Type: "class_cast_exception", Offset: off, Reason: "Cannot cast " + plClassName(idx) + " to java.lang.Number"}
	}
	i := int(I64(idx))
	if i < 0 {
		i += n
	}
	return i, nil
}

var plArrayListGetFrames = []string{
	"java.base/jdk.internal.util.Preconditions.outOfBounds(Preconditions.java:100)",
	"java.base/jdk.internal.util.Preconditions.outOfBoundsCheckIndex(Preconditions.java:106)",
	"java.base/jdk.internal.util.Preconditions.checkIndex(Preconditions.java:302)",
	"java.base/java.util.Objects.checkIndex(Objects.java:365)",
	"java.base/java.util.ArrayList.get(ArrayList.java:428)",
}

func plOOB(i, n, off int, frames []string) *RuntimeError {
	return &RuntimeError{Type: "index_out_of_bounds_exception", Offset: off, Frames: frames,
		Reason: "Index " + itoa(i) + " out of bounds for length " + itoa(n)}
}

func plIndexGet(obj, idx any, off int) (any, *RuntimeError) {
	switch t := obj.(type) {
	case nil:
		return nil, plNPE(off)
	case *List:
		i, err := plListIndex(idx, len(t.a), off)
		if err != nil {
			return nil, err
		}
		if i < 0 || i >= len(t.a) {
			return nil, plOOB(int(I64(idx)), len(t.a), off, plArrayListGetFrames)
		}
		return t.a[i], nil
	case *Array:
		i, err := plListIndex(idx, len(t.Items), off)
		if err != nil {
			return nil, err
		}
		if i < 0 || i >= len(t.Items) {
			return nil, &RuntimeError{Type: "array_index_out_of_bounds_exception", Offset: off,
				Reason: "Index " + itoa(int(I64(idx))) + " out of bounds for length " + itoa(len(t.Items))}
		}
		return t.Items[i], nil
	case *Map:
		v, _ := t.Get(idx)
		return v, nil
	case *DocMap:
		s, ok := idx.(string)
		if !ok {
			s = ToString(idx)
		}
		return t.Get(s, off+1)
	case *DocValues:
		i, err := plListIndex(idx, len(t.Values), off)
		if err != nil {
			return nil, err
		}
		return t.get(i, off)
	}
	cls, _ := plRuntimeClass(obj)
	return nil, plIAE(off, "Attempting to address a non-array-like type ["+cls+"] as an array.")
}

func plIndexSet(obj, idx, v any, off int) *RuntimeError {
	switch t := obj.(type) {
	case nil:
		return plNPE(off)
	case *List:
		i, err := plListIndex(idx, len(t.a), off)
		if err != nil {
			return err
		}
		if i < 0 || i >= len(t.a) {
			return plOOB(int(I64(idx)), len(t.a), off, nil)
		}
		t.a[i] = v
		return nil
	case *Array:
		i, err := plListIndex(idx, len(t.Items), off)
		if err != nil {
			return err
		}
		if i < 0 || i >= len(t.Items) {
			return &RuntimeError{Type: "array_index_out_of_bounds_exception", Offset: off,
				Reason: "Index " + itoa(int(I64(idx))) + " out of bounds for length " + itoa(len(t.Items))}
		}
		t.Items[i] = v
		return nil
	case *Map:
		t.Put(idx, v)
		return nil
	}
	cls, _ := plRuntimeClass(obj)
	return plIAE(off, "Attempting to address a non-array-like type ["+cls+"] as an array.")
}

func plIterItems(v any, off int) ([]any, *RuntimeError) {
	switch t := v.(type) {
	case nil:
		return nil, plNPE(off)
	case *List:
		return append([]any{}, t.a...), nil
	case *DocValues:
		return append([]any{}, t.Values...), nil
	case *Set:
		return t.items(), nil
	case *Array:
		return append([]any{}, t.Items...), nil
	case *plStream:
		return t.items, nil
	case *plIterator:
		return t.items[t.i:], nil
	case *plEntrySet:
		return append([]any{}, t.l.a...), nil
	}
	cls, _ := plRuntimeClass(v)
	return nil, plIAE(off, "Cannot iterate over ["+cls+"]")
}

// ToItems returns the elements of any collection-like value.
func ToItems(v any) ([]any, bool) {
	switch t := v.(type) {
	case *List:
		return t.a, true
	case *DocValues:
		return t.Values, true
	case *Set:
		return t.items(), true
	case *Array:
		return t.Items, true
	case *plStream:
		return t.items, true
	case *plEntrySet:
		return t.l.a, true
	}
	return nil, false
}

// ---- doc values ----

func (d *DocValues) missingErr(off int) *RuntimeError {
	return &RuntimeError{Type: "illegal_state_exception", Offset: off,
		Frames: []string{"org.opensearch.index.fielddata.ScriptDocValues$" + d.Kind + ".get(ScriptDocValues.java:145)",
			"org.opensearch.index.fielddata.ScriptDocValues$" + d.Kind + ".getValue(ScriptDocValues.java:139)"},
		Reason: "A document doesn't have a value for a field! Use doc[<field>].size()==0 to check if a document is missing a field!"}
}

func (d *DocValues) value(off int) (any, *RuntimeError) {
	if len(d.Values) == 0 {
		return nil, d.missingErr(off)
	}
	return d.Values[0], nil
}

func (d *DocValues) get(i, off int) (any, *RuntimeError) {
	if len(d.Values) == 0 {
		return nil, d.missingErr(off)
	}
	if i < 0 || i >= len(d.Values) {
		return nil, plOOB(i, len(d.Values), off, nil)
	}
	return d.Values[i], nil
}

// ---- construction ----

func plNew(typ string, args []any, off int) (any, *RuntimeError) {
	switch typ {
	case "ArrayList", "LinkedList", "ArrayDeque":
		l := &List{}
		if len(args) == 1 {
			if items, ok := ToItems(args[0]); ok {
				l.a = append(l.a, items...)
			}
		}
		return l, nil
	case "HashMap", "TreeMap", "LinkedHashMap":
		kind := map[string]int{"HashMap": Hash, "TreeMap": plTree, "LinkedHashMap": Linked}[typ]
		m := NewMap(kind)
		if len(args) == 1 {
			if src, ok := args[0].(*Map); ok {
				for _, e := range src.ordered() {
					m.Put(e.Key, e.Value)
				}
			}
		}
		return m, nil
	case "HashSet", "TreeSet", "LinkedHashSet":
		kind := map[string]int{"HashSet": Hash, "TreeSet": plTree, "LinkedHashSet": Linked}[typ]
		s := newPlSet(kind)
		if len(args) == 1 {
			if items, ok := ToItems(args[0]); ok {
				for _, it := range items {
					s.add(it)
				}
			}
		}
		return s, nil
	case "StringBuilder":
		sb := &plStringBuilder{}
		if len(args) == 1 {
			if s, ok := args[0].(string); ok {
				sb.s = []rune(s)
			}
		}
		return sb, nil
	case "Exception", "RuntimeException", "IllegalArgumentException", "IllegalStateException",
		"ArithmeticException", "NullPointerException", "UnsupportedOperationException",
		"IndexOutOfBoundsException", "NumberFormatException", "ClassCastException":
		ex := &plException{typ: plExceptionType(typ), class: typ, nilMsg: true}
		if len(args) >= 1 && args[0] != nil {
			ex.msg = ToString(args[0])
			ex.nilMsg = false
		}
		return ex, nil
	case "Object":
		return &Map{kind: -1}, nil
	case "Random":
		return &plStaticRef{name: "Random"}, nil
	}
	return nil, plIAE(off, "constructor [["+plCanon(typ)+"], <init>/"+itoa(len(args))+"] not found")
}

func plStaticEnum(class, name string, off int) (any, *RuntimeError) {
	switch class {
	case "ChronoUnit":
		if _, ok := plChronoUnits[name]; ok {
			return plChronoUnit(name), nil
		}
	case "DayOfWeek":
		for i, d := range plDayNames {
			if d == name {
				return plDayOfWeek(i + 1), nil
			}
		}
	case "ZoneOffset":
		if name == "UTC" {
			return plZoneID("Z"), nil
		}
	case "Instant":
		if name == "EPOCH" {
			return &plInstant{ms: 0}, nil
		}
	case "DateTimeFormatter":
		switch name {
		case "ISO_LOCAL_DATE":
			return &plDTF{pattern: "yyyy-MM-dd"}, nil
		case "ISO_INSTANT", "ISO_OFFSET_DATE_TIME", "ISO_ZONED_DATE_TIME", "ISO_DATE_TIME":
			return &plDTF{iso: true}, nil
		}
	}
	return nil, plIAE(off, "static field ["+plCanon(class)+", "+name+"] not found")
}

// ---- comparison helpers ----

func (in *plInterp) compareWith(cmp any, a, b any, off int) (int, *RuntimeError) {
	if cmp == nil {
		c, ok := Compare(a, b)
		if !ok {
			if a == nil || b == nil {
				return 0, plNPE(off)
			}
			return 0, &RuntimeError{Type: "class_cast_exception", Offset: off,
				Reason: "class " + plClassName(a) + " cannot be cast to class java.lang.Comparable"}
		}
		return c, nil
	}
	r, err := in.callFn(cmp, []any{a, b}, off)
	if err != nil {
		return 0, err
	}
	return int(I64(r)), nil
}

func (in *plInterp) sortItems(items []any, cmp any, off int) *RuntimeError {
	var ferr *RuntimeError
	sort.SliceStable(items, func(i, j int) bool {
		if ferr != nil {
			return false
		}
		c, err := in.compareWith(cmp, items[i], items[j], off)
		if err != nil {
			ferr = err
			return false
		}
		return c < 0
	})
	return ferr
}

func plNativeFn(f func(args []any, off int) (any, *RuntimeError)) *plFn { return &plFn{native: f} }

func (in *plInterp) comparatorOf(spec string, key any, base any) *plFn {
	switch spec {
	case "natural":
		return plNativeFn(func(args []any, off int) (any, *RuntimeError) {
			c, err := in.compareWith(nil, args[0], args[1], off)
			return int32(c), err
		})
	case "reverse":
		return plNativeFn(func(args []any, off int) (any, *RuntimeError) {
			c, err := in.compareWith(base, args[1], args[0], off)
			return int32(c), err
		})
	case "comparing":
		return plNativeFn(func(args []any, off int) (any, *RuntimeError) {
			ka, err := in.callFn(key, []any{args[0]}, off)
			if err != nil {
				return nil, err
			}
			kb, err := in.callFn(key, []any{args[1]}, off)
			if err != nil {
				return nil, err
			}
			c, err := in.compareWith(base, ka, kb, off)
			return int32(c), err
		})
	}
	return nil
}

// ---- static methods ----

func plParseIntStr(s string, radix int, bits int, off int) (int64, *RuntimeError) {
	n, err := strconv.ParseInt(s, radix, bits)
	if err != nil {
		msg := `For input string: "` + s + `"`
		if radix != 10 {
			msg += " under radix " + itoa(radix)
		}
		return 0, &RuntimeError{Type: "number_format_exception", Offset: off, Reason: msg}
	}
	return n, nil
}

func plJavaRound(f float64) int64 {
	if math.IsNaN(f) {
		return 0
	}
	return plF2L(math.Floor(f + 0.5))
}

func (in *plInterp) callStatic(class, name string, args []any, n *plECall) (any, *RuntimeError) {
	off := n.pos
	a := func(i int) any { return args[i] }
	f := func(i int) float64 { return F64(args[i]) }
	str := func(i int) string {
		if s, ok := args[i].(string); ok {
			return s
		}
		return ToString(args[i])
	}
	key := name + "/" + itoa(len(args))
	switch class {
	case "Math", "StrictMath":
		switch name {
		case "abs":
			return math.Abs(f(0)), nil
		case "acos":
			return math.Acos(f(0)), nil
		case "asin":
			return math.Asin(f(0)), nil
		case "atan":
			return math.Atan(f(0)), nil
		case "cbrt":
			return math.Cbrt(f(0)), nil
		case "ceil":
			return math.Ceil(f(0)), nil
		case "cos":
			return math.Cos(f(0)), nil
		case "cosh":
			return math.Cosh(f(0)), nil
		case "exp":
			return math.Exp(f(0)), nil
		case "expm1":
			return math.Expm1(f(0)), nil
		case "floor":
			return math.Floor(f(0)), nil
		case "log":
			return math.Log(f(0)), nil
		case "log10":
			return math.Log10(f(0)), nil
		case "log1p":
			return math.Log1p(f(0)), nil
		case "rint":
			return math.RoundToEven(f(0)), nil
		case "signum":
			x := f(0)
			if x == 0 || math.IsNaN(x) {
				return x, nil
			}
			return math.Copysign(1, x), nil
		case "sin":
			return math.Sin(f(0)), nil
		case "sinh":
			return math.Sinh(f(0)), nil
		case "sqrt":
			return math.Sqrt(f(0)), nil
		case "tan":
			return math.Tan(f(0)), nil
		case "tanh":
			return math.Tanh(f(0)), nil
		case "toDegrees":
			return f(0) * 180 / math.Pi, nil
		case "toRadians":
			return f(0) / 180 * math.Pi, nil
		case "ulp":
			x := math.Abs(f(0))
			return math.Nextafter(x, math.Inf(1)) - x, nil
		case "atan2":
			return math.Atan2(f(0), f(1)), nil
		case "hypot":
			return math.Hypot(f(0), f(1)), nil
		case "IEEEremainder":
			return math.Remainder(f(0), f(1)), nil
		case "max":
			return math.Max(f(0), f(1)), nil
		case "min":
			return math.Min(f(0), f(1)), nil
		case "pow":
			return math.Pow(f(0), f(1)), nil
		case "round":
			return plJavaRound(f(0)), nil
		case "random":
			return rand.Float64(), nil
		}
	case "Integer", "Long", "Short", "Byte":
		bits := map[string]int{"Integer": 32, "Long": 64, "Short": 16, "Byte": 8}[class]
		prim := plUnbox(class)
		switch name {
		case "parseInt", "parseLong", "parseShort", "parseByte", "parseUnsignedInt":
			radix := 10
			if len(args) == 2 {
				radix = int(I64(a(1)))
			}
			v, err := plParseIntStr(str(0), radix, bits, off)
			if err != nil {
				return nil, err
			}
			return plCastNum(v, prim), nil
		case "valueOf":
			if s, ok := a(0).(string); ok {
				radix := 10
				if len(args) == 2 {
					radix = int(I64(a(1)))
				}
				v, err := plParseIntStr(s, radix, bits, off)
				if err != nil {
					return nil, err
				}
				return plCastNum(v, prim), nil
			}
			return plCastNum(a(0), prim), nil
		case "toString":
			radix := 10
			if len(args) == 2 {
				radix = int(I64(a(1)))
			}
			return strconv.FormatInt(I64(a(0)), radix), nil
		case "toBinaryString", "toHexString", "toOctalString":
			radix := map[string]int{"toBinaryString": 2, "toHexString": 16, "toOctalString": 8}[name]
			v := I64(a(0))
			if bits == 32 {
				return strconv.FormatUint(uint64(uint32(int32(v))), radix), nil
			}
			return strconv.FormatUint(uint64(v), radix), nil
		case "bitCount":
			v := I64(a(0))
			c := 0
			u := uint64(v)
			if class == "Integer" {
				u = uint64(uint32(int32(v)))
			}
			for u != 0 {
				c += int(u & 1)
				u >>= 1
			}
			return int32(c), nil
		case "compare":
			x, y := I64(a(0)), I64(a(1))
			switch {
			case x < y:
				return int32(-1), nil
			case x > y:
				return int32(1), nil
			}
			return int32(0), nil
		case "max":
			if I64(a(0)) >= I64(a(1)) {
				return a(0), nil
			}
			return a(1), nil
		case "min":
			if I64(a(0)) <= I64(a(1)) {
				return a(0), nil
			}
			return a(1), nil
		case "sum":
			v, _ := plArith("+", a(0), a(1))
			return v, nil
		case "signum":
			v := I64(a(0))
			switch {
			case v < 0:
				return int32(-1), nil
			case v > 0:
				return int32(1), nil
			}
			return int32(0), nil
		case "hashCode":
			return plHashCode(a(0)), nil
		case "toUnsignedLong":
			return int64(uint32(int32(I64(a(0))))), nil
		case "numberOfLeadingZeros", "numberOfTrailingZeros", "highestOneBit", "lowestOneBit", "reverse":
			v := uint64(I64(a(0)))
			w := bits
			if w == 32 {
				v = uint64(uint32(v))
			}
			switch name {
			case "numberOfLeadingZeros":
				c := 0
				for i := w - 1; i >= 0 && v&(1<<uint(i)) == 0; i-- {
					c++
				}
				return int32(c), nil
			case "numberOfTrailingZeros":
				if v == 0 {
					return int32(w), nil
				}
				c := 0
				for v&1 == 0 {
					c++
					v >>= 1
				}
				return int32(c), nil
			case "highestOneBit":
				var r uint64
				for i := w - 1; i >= 0; i-- {
					if v&(1<<uint(i)) != 0 {
						r = 1 << uint(i)
						break
					}
				}
				return plCastNum(int64(r), prim), nil
			case "lowestOneBit":
				return plCastNum(int64(v&(-v)), prim), nil
			}
		}
	case "Double", "Float":
		prim := plUnbox(class)
		switch name {
		case "parseDouble", "parseFloat", "valueOf":
			if s, ok := a(0).(string); ok || name != "valueOf" {
				if !ok {
					s = str(0)
				}
				v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
				if err != nil && !strings.Contains(err.Error(), "range") {
					return nil, &RuntimeError{Type: "number_format_exception", Offset: off, Reason: `For input string: "` + s + `"`}
				}
				return plCastNum(v, prim), nil
			}
			return plCastNum(a(0), prim), nil
		case "toString":
			return ToString(plCastNum(a(0), prim)), nil
		case "isNaN":
			return math.IsNaN(f(0)), nil
		case "isInfinite":
			return math.IsInf(f(0), 0), nil
		case "isFinite":
			return !math.IsInf(f(0), 0) && !math.IsNaN(f(0)), nil
		case "compare":
			x, y := f(0), f(1)
			switch {
			case x < y:
				return int32(-1), nil
			case x > y:
				return int32(1), nil
			case x == y && math.Signbit(x) == math.Signbit(y):
				return int32(0), nil
			}
			bx, by := plJavaDoubleBits(x), plJavaDoubleBits(y)
			switch {
			case bx < by:
				return int32(-1), nil
			case bx > by:
				return int32(1), nil
			}
			return int32(0), nil
		case "max":
			return plCastNum(math.Max(f(0), f(1)), prim), nil
		case "min":
			return plCastNum(math.Min(f(0), f(1)), prim), nil
		case "sum":
			return plCastNum(f(0)+f(1), prim), nil
		case "hashCode":
			return plHashCode(a(0)), nil
		case "doubleToLongBits":
			return plJavaDoubleBits(f(0)), nil
		case "longBitsToDouble":
			return math.Float64frombits(uint64(I64(a(0)))), nil
		case "floatToIntBits":
			return int32(math.Float32bits(float32(f(0)))), nil
		case "intBitsToFloat":
			return math.Float32frombits(uint32(I64(a(0)))), nil
		}
	case "Boolean":
		switch name {
		case "parseBoolean":
			return strings.EqualFold(str(0), "true"), nil
		case "valueOf":
			if s, ok := a(0).(string); ok {
				return strings.EqualFold(s, "true"), nil
			}
			return a(0), nil
		case "toString":
			return ToString(a(0)), nil
		case "compare":
			c, _ := Compare(a(0), a(1))
			return int32(c), nil
		case "logicalAnd":
			return a(0).(bool) && a(1).(bool), nil
		case "logicalOr":
			return a(0).(bool) || a(1).(bool), nil
		case "logicalXor":
			return a(0).(bool) != a(1).(bool), nil
		case "hashCode":
			return plHashCode(a(0)), nil
		}
	case "Character":
		r := rune(I64(a(0)))
		switch name {
		case "isDigit":
			return unicode.IsDigit(r), nil
		case "isLetter":
			return unicode.IsLetter(r), nil
		case "isLetterOrDigit":
			return unicode.IsLetter(r) || unicode.IsDigit(r), nil
		case "isLowerCase":
			return unicode.IsLower(r), nil
		case "isUpperCase":
			return unicode.IsUpper(r), nil
		case "isWhitespace":
			return unicode.IsSpace(r), nil
		case "isSpaceChar":
			return unicode.Is(unicode.Zs, r), nil
		case "isAlphabetic":
			return unicode.IsLetter(r), nil
		case "toLowerCase":
			return plChar(unicode.ToLower(r)), nil
		case "toUpperCase":
			return plChar(unicode.ToUpper(r)), nil
		case "getNumericValue", "digit":
			if d, err := strconv.ParseInt(string(r), 36, 32); err == nil {
				return int32(d), nil
			}
			return int32(-1), nil
		case "valueOf":
			return plCastNum(a(0), "char"), nil
		case "toString":
			return ToString(plCastNum(a(0), "char")), nil
		case "compare":
			return int32(I64(a(0)) - I64(a(1))), nil
		case "hashCode":
			return int32(I64(a(0))), nil
		}
	case "String":
		switch name {
		case "valueOf", "copyValueOf":
			if arr, ok := a(0).(*Array); ok && arr.Elem == "char" {
				var sb strings.Builder
				for _, c := range arr.Items {
					sb.WriteString(ToString(c))
				}
				return sb.String(), nil
			}
			return ToString(a(0)), nil
		case "join":
			items, ok := ToItems(a(1))
			if !ok {
				return nil, plNPE(off)
			}
			parts := make([]string, len(items))
			for i, it := range items {
				parts[i] = ToString(it)
			}
			return strings.Join(parts, str(0)), nil
		}
	case "Objects":
		switch name {
		case "equals", "deepEquals":
			return plJavaEquals(a(0), a(1)), nil
		case "hashCode":
			return plHashCode(a(0)), nil
		case "isNull":
			return a(0) == nil, nil
		case "nonNull":
			return a(0) != nil, nil
		case "requireNonNull":
			if a(0) == nil {
				e := &RuntimeError{Type: "null_pointer_exception", Offset: off, NilReason: true,
					Frames: []string{"java.base/java.util.Objects.requireNonNull(Objects.java:220)"}}
				if len(args) == 2 {
					e.NilReason = false
					e.Reason = str(1)
					e.Frames = []string{"java.base/java.util.Objects.requireNonNull(Objects.java:246)"}
				}
				return nil, e
			}
			return a(0), nil
		case "requireNonNullElse":
			if a(0) == nil {
				return a(1), nil
			}
			return a(0), nil
		case "toString":
			if a(0) == nil && len(args) == 2 {
				return a(1), nil
			}
			return ToString(a(0)), nil
		case "compare":
			if plIdentical(a(0), a(1)) {
				return int32(0), nil
			}
			c, err := in.compareWith(a(2), a(0), a(1), off)
			return int32(c), err
		}
	case "Collections":
		switch name {
		case "sort":
			l, ok := a(0).(*List)
			if !ok {
				return nil, plNPE(off)
			}
			var cmp any
			if len(args) == 2 {
				cmp = a(1)
			}
			return nil, in.sortItems(l.a, cmp, off)
		case "reverse":
			if l, ok := a(0).(*List); ok {
				for i, j := 0, len(l.a)-1; i < j; i, j = i+1, j-1 {
					l.a[i], l.a[j] = l.a[j], l.a[i]
				}
			}
			return nil, nil
		case "shuffle":
			if l, ok := a(0).(*List); ok {
				rand.Shuffle(len(l.a), func(i, j int) { l.a[i], l.a[j] = l.a[j], l.a[i] })
			}
			return nil, nil
		case "max", "min":
			items, ok := ToItems(a(0))
			if !ok || len(items) == 0 {
				return nil, &RuntimeError{Type: "no_such_element_exception", Offset: off, NilReason: true}
			}
			var cmp any
			if len(args) == 2 {
				cmp = a(1)
			}
			best := items[0]
			for _, it := range items[1:] {
				c, err := in.compareWith(cmp, it, best, off)
				if err != nil {
					return nil, err
				}
				if (name == "max" && c > 0) || (name == "min" && c < 0) {
					best = it
				}
			}
			return best, nil
		case "emptyList":
			return &List{}, nil
		case "emptyMap":
			return NewMap(Hash), nil
		case "emptySet":
			return newPlSet(Hash), nil
		case "singletonList":
			return NewList(a(0)), nil
		case "singleton":
			s := newPlSet(Hash)
			s.add(a(0))
			return s, nil
		case "singletonMap":
			m := NewMap(Hash)
			m.Put(a(0), a(1))
			return m, nil
		case "unmodifiableList", "unmodifiableMap", "unmodifiableSet":
			return a(0), nil
		case "frequency":
			items, _ := ToItems(a(0))
			c := 0
			for _, it := range items {
				if plJavaEquals(it, a(1)) {
					c++
				}
			}
			return int32(c), nil
		case "swap":
			if l, ok := a(0).(*List); ok {
				i, j := int(I64(a(1))), int(I64(a(2)))
				l.a[i], l.a[j] = l.a[j], l.a[i]
			}
			return nil, nil
		case "nCopies":
			l := &List{}
			for i := int64(0); i < I64(a(0)); i++ {
				l.a = append(l.a, a(1))
			}
			return l, nil
		case "reverseOrder":
			return in.comparatorOf("reverse", nil, nil), nil
		}
	case "Collectors":
		c := &plCollector{kind: name}
		switch name {
		case "joining":
			if len(args) >= 1 {
				c.sep = str(0)
			}
			if len(args) == 3 {
				c.prefix, c.suffix = str(1), str(2)
			}
		default:
			if len(args) >= 1 {
				c.f1 = a(0)
			}
			if len(args) >= 2 {
				c.f2 = a(1)
			}
		}
		return c, nil
	case "Comparator":
		switch name {
		case "naturalOrder":
			return in.comparatorOf("natural", nil, nil), nil
		case "reverseOrder":
			return in.comparatorOf("reverse", nil, nil), nil
		case "comparing", "comparingInt", "comparingLong", "comparingDouble":
			var base any
			if len(args) == 2 {
				base = a(1)
			}
			return in.comparatorOf("comparing", a(0), base), nil
		case "nullsFirst", "nullsLast":
			base := a(0)
			first := name == "nullsFirst"
			return plNativeFn(func(xs []any, o int) (any, *RuntimeError) {
				x, y := xs[0], xs[1]
				switch {
				case x == nil && y == nil:
					return int32(0), nil
				case x == nil:
					if first {
						return int32(-1), nil
					}
					return int32(1), nil
				case y == nil:
					if first {
						return int32(1), nil
					}
					return int32(-1), nil
				}
				c, err := in.compareWith(base, x, y, o)
				return int32(c), err
			}), nil
		}
	case "Arrays":
		switch name {
		case "asList":
			items, _ := ToItems(a(0))
			return NewList(items...), nil
		case "toString":
			items, ok := ToItems(a(0))
			if !ok {
				return "null", nil
			}
			return plJoinToString(items), nil
		case "sort":
			if arr, ok := a(0).(*Array); ok {
				return nil, in.sortItems(arr.Items, nil, off)
			}
			return nil, nil
		case "stream":
			items, _ := ToItems(a(0))
			kind := "obj"
			if arr, ok := a(0).(*Array); ok {
				switch arr.Elem {
				case "int", "long", "double":
					kind = arr.Elem
				}
			}
			return &plStream{items: append([]any{}, items...), kind: kind}, nil
		case "equals":
			x, _ := ToItems(a(0))
			y, _ := ToItems(a(1))
			return plJavaEquals(NewList(x...), NewList(y...)), nil
		case "fill":
			if arr, ok := a(0).(*Array); ok {
				for i := range arr.Items {
					arr.Items[i] = a(1)
				}
			}
			return nil, nil
		}
	case "Debug":
		if name == "explain" {
			pc, jc := plRuntimeClass(a(0))
			if dv, ok := a(0).(*DocValues); ok {
				pc = "org.opensearch.index.fielddata.ScriptDocValues." + dv.Kind
				jc = "org.opensearch.index.fielddata.ScriptDocValues$" + dv.Kind
			}
			eoff := off
			if len(n.args) > 0 {
				eoff = n.args[0].nodePos()
			}
			return nil, &RuntimeError{Type: "painless_explain_error", NilReason: true, Offset: eoff,
				Extra: []plErrField{{"painless_class", pc}, {"to_string", ToString(a(0))}, {"java_class", jc}}}
		}
	case "Pattern":
		switch name {
		case "compile":
			return plCompilePattern(str(0), "", off)
		case "quote":
			return "\\Q" + str(0) + "\\E", nil
		}
	case "IntStream", "LongStream":
		lo, hi := I64(a(0)), I64(a(1))
		if name == "rangeClosed" {
			hi++
		}
		s := &plStream{kind: "int"}
		if class == "LongStream" {
			s.kind = "long"
		}
		for i := lo; i < hi; i++ {
			if s.kind == "int" {
				s.items = append(s.items, int32(i))
			} else {
				s.items = append(s.items, i)
			}
		}
		return s, nil
	case "Stream":
		if name == "empty" {
			return &plStream{kind: "obj"}, nil
		}
		items, _ := ToItems(a(0))
		return &plStream{kind: "obj", items: append([]any{}, items...)}, nil
	case "List":
		items, _ := ToItems(a(0))
		return NewList(items...), nil
	case "Set":
		s := newPlSet(Hash)
		items, _ := ToItems(a(0))
		for _, it := range items {
			s.add(it)
		}
		return s, nil
	case "Map":
		m := NewMap(Hash)
		for i := 0; i+1 < len(args); i += 2 {
			m.Put(args[i], args[i+1])
		}
		return m, nil
	case "Base64":
		return &plStaticRef{name: "Base64." + name}, nil
	}
	if v, err, ok := in.callStaticTime(class, name, args, off); ok {
		return v, err
	}
	return nil, plIAE(off, "static method ["+plCanon(class)+", "+key+"] not found")
}

func plJavaDoubleBits(f float64) int64 {
	if math.IsNaN(f) {
		return 0x7ff8000000000000
	}
	return int64(math.Float64bits(f))
}

// ---- instance methods ----

func (in *plInterp) callMethod(recv any, name string, args []any, off int) (any, *RuntimeError) {
	n := len(args)
	switch t := recv.(type) {
	case string:
		return in.stringMethod(t, name, args, off)
	case *List:
		if v, err, ok := in.listMethod(t, name, args, off); ok {
			return v, err
		}
	case *DocValues:
		switch name {
		case "getValue":
			if n == 0 {
				return t.value(off)
			}
		case "get":
			if n == 1 {
				return t.get(int(I64(args[0])), off)
			}
		case "size":
			return int32(len(t.Values)), nil
		case "isEmpty":
			return len(t.Values) == 0, nil
		case "getLength":
			return int32(len(t.Values)), nil
		case "contains":
			for _, v := range t.Values {
				if plEquals(v, args[0]) {
					return true, nil
				}
			}
			return false, nil
		case "stream":
			return &plStream{kind: "obj", items: append([]any{}, t.Values...)}, nil
		case "iterator":
			return &plIterator{items: append([]any{}, t.Values...)}, nil
		case "toString":
			return ToString(t), nil
		case "getLat", "getLon":
			if t.Kind == "GeoPoints" {
				v, err := t.value(off)
				if err != nil {
					return nil, err
				}
				if name == "getLat" {
					return v.(*GeoPoint).Lat, nil
				}
				return v.(*GeoPoint).Lon, nil
			}
		case "getDate", "getDates":
		case "indexOf":
			for i, v := range t.Values {
				if plEquals(v, args[0]) {
					return int32(i), nil
				}
			}
			return int32(-1), nil
		}
		if t.Kind != "" {
			if v, err, ok := in.collectionAugment(t.Values, name, args, off); ok {
				return v, err
			}
		}
	case *Map:
		if v, err, ok := in.mapMethod(t, name, args, off); ok {
			return v, err
		}
	case *DocMap:
		switch name {
		case "containsKey":
			return t.Has(ToString(args[0])), nil
		case "get":
			return t.Get(ToString(args[0]), off)
		}
	case *Set:
		if v, err, ok := in.setMethod(t, name, args, off); ok {
			return v, err
		}
	case *plEntrySet:
		if v, err, ok := in.listMethod(t.l, name, args, off); ok {
			return v, err
		}
	case *Array:
		if name == "clone" {
			return &Array{Elem: t.Elem, Items: append([]any{}, t.Items...)}, nil
		}
	case *plStringBuilder:
		if v, err, ok := plSBMethod(t, name, args, off); ok {
			return v, err
		}
	case *plPattern:
		switch name {
		case "matcher":
			return t.matcher(ToString(args[0]), in), nil
		case "pattern", "toString":
			return t.src, nil
		case "split":
			parts := t.re.Split(ToString(args[0]), -1)
			for len(parts) > 0 && parts[len(parts)-1] == "" {
				parts = parts[:len(parts)-1]
			}
			arr := &Array{Elem: "String"}
			for _, p := range parts {
				arr.Items = append(arr.Items, p)
			}
			return arr, nil
		case "asPredicate":
			p := t
			return plNativeFn(func(xs []any, o int) (any, *RuntimeError) {
				return p.re.MatchString(ToString(xs[0])), nil
			}), nil
		}
	case *plMatcher:
		switch name {
		case "find":
			return t.find(off)
		case "matches":
			return t.matches(off)
		case "lookingAt":
			loc := t.p.re.FindStringSubmatchIndex(t.input)
			ok := loc != nil && loc[0] == 0
			if ok {
				t.loc = loc
			}
			return ok, nil
		case "group":
			if n == 0 {
				return t.group(0, off)
			}
			if s, ok := args[0].(string); ok {
				idx := t.p.re.SubexpIndex(s)
				if idx < 0 {
					return nil, plIAE(off, "No group with name <"+s+">")
				}
				return t.group(idx, off)
			}
			return t.group(int(I64(args[0])), off)
		case "groupCount":
			return int32(t.p.re.NumSubexp()), nil
		case "start", "end":
			if t.loc == nil {
				return nil, &RuntimeError{Type: "illegal_state_exception", Offset: off, Reason: "No match available"}
			}
			g := 0
			if n == 1 {
				g = int(I64(args[0]))
			}
			i := 2 * g
			if name == "end" {
				i++
			}
			return int32(len(plUTF16(t.input[:t.loc[i]]))), nil
		case "replaceAll", "replaceFirst":
			repl := plJavaReplacement(ToString(args[0]))
			if name == "replaceAll" {
				return t.p.re.ReplaceAllString(t.input, repl), nil
			}
			loc := t.p.re.FindStringSubmatchIndex(t.input)
			if loc == nil {
				return t.input, nil
			}
			out := t.p.re.ExpandString(nil, repl, t.input, loc)
			return t.input[:loc[0]] + string(out) + t.input[loc[1]:], nil
		case "reset":
			t.pos = 0
			t.loc = nil
			return t, nil
		case "hitEnd":
			return t.pos >= len(t.input), nil
		case "namedGroup":
			idx := t.p.re.SubexpIndex(ToString(args[0]))
			if idx < 0 {
				return nil, nil
			}
			return t.group(idx, off)
		}
	case *plStream:
		if v, err, ok := in.streamMethod(t, name, args, off); ok {
			return v, err
		}
	case *plOptional:
		switch name {
		case "get", "getAsInt", "getAsDouble", "getAsLong", "orElseThrow":
			if !t.present {
				return nil, &RuntimeError{Type: "no_such_element_exception", Offset: off, Reason: "No value present"}
			}
			return t.v, nil
		case "isPresent":
			return t.present, nil
		case "isEmpty":
			return !t.present, nil
		case "orElse":
			if t.present {
				return t.v, nil
			}
			return args[0], nil
		case "orElseGet":
			if t.present {
				return t.v, nil
			}
			return in.callFn(args[0], nil, off)
		case "ifPresent":
			if t.present {
				_, err := in.callFn(args[0], []any{t.v}, off)
				return nil, err
			}
			return nil, nil
		case "map":
			if !t.present {
				return t, nil
			}
			v, err := in.callFn(args[0], []any{t.v}, off)
			if err != nil {
				return nil, err
			}
			return &plOptional{name: "Optional", v: v, present: v != nil}, nil
		}
	case *plMapEntryRef:
		switch name {
		case "getKey":
			return t.e.Key, nil
		case "getValue":
			return t.e.Value, nil
		case "setValue":
			old := t.e.Value
			t.e.Value = args[0]
			return old, nil
		}
	case *plIterator:
		switch name {
		case "hasNext":
			return t.i < len(t.items), nil
		case "next":
			if t.i >= len(t.items) {
				return nil, &RuntimeError{Type: "no_such_element_exception", Offset: off, NilReason: true}
			}
			t.i++
			return t.items[t.i-1], nil
		}
	case *plException:
		switch name {
		case "getMessage", "getLocalizedMessage":
			if t.nilMsg {
				return nil, nil
			}
			return t.msg, nil
		case "toString":
			if t.nilMsg {
				return "java.lang." + t.class, nil
			}
			return "java.lang." + t.class + ": " + t.msg, nil
		}
	case *plFn:
		switch name {
		case "reversed":
			return in.comparatorOf("reverse", nil, t), nil
		case "thenComparing":
			next := args[0]
			if fn, ok := next.(*plFn); ok && fn.native == nil && len(fn.pnames) == 1 {
				next = in.comparatorOf("comparing", fn, nil)
			}
			return plNativeFn(func(xs []any, o int) (any, *RuntimeError) {
				c, err := in.compareWith(t, xs[0], xs[1], o)
				if err != nil || c != 0 {
					return int32(c), err
				}
				c, err = in.compareWith(next, xs[0], xs[1], o)
				return int32(c), err
			}), nil
		case "negate":
			return plNativeFn(func(xs []any, o int) (any, *RuntimeError) {
				v, err := in.callFn(t, xs, o)
				if err != nil {
					return nil, err
				}
				b, _ := v.(bool)
				return !b, nil
			}), nil
		case "andThen", "compose":
			other := args[0]
			first, second := any(t), other
			if name == "compose" {
				first, second = other, t
			}
			return plNativeFn(func(xs []any, o int) (any, *RuntimeError) {
				v, err := in.callFn(first, xs, o)
				if err != nil {
					return nil, err
				}
				return in.callFn(second, []any{v}, o)
			}), nil
		}
		return in.callFn(t, args, off)
	case *plStaticRef:
		switch t.name {
		case "Base64.getEncoder":
			if name == "encodeToString" {
				if arr, ok := args[0].(*Array); ok {
					b := make([]byte, len(arr.Items))
					for i, x := range arr.Items {
						b[i] = byte(I64(x))
					}
					return base64.StdEncoding.EncodeToString(b), nil
				}
			}
		case "Random":
			switch name {
			case "nextInt":
				if n == 1 {
					return int32(rand.Intn(int(I64(args[0])))), nil
				}
				return int32(rand.Uint32()), nil
			case "nextDouble":
				return rand.Float64(), nil
			}
		}
	case *GeoPoint:
		switch name {
		case "getLat":
			return t.Lat, nil
		case "getLon":
			return t.Lon, nil
		}
	}
	if v, err, ok := in.timeMethod(recv, name, args, off); ok {
		return v, err
	}
	switch name {
	case "toString":
		if n == 0 {
			return ToString(recv), nil
		}
	case "equals":
		if n == 1 {
			return plJavaEquals(recv, args[0]), nil
		}
	case "hashCode":
		if n == 0 {
			return plHashCode(recv), nil
		}
	case "compareTo":
		if n == 1 {
			c, ok := Compare(recv, args[0])
			if ok {
				if c < 0 {
					return int32(-1), nil
				} else if c > 0 {
					return int32(1), nil
				}
				return int32(0), nil
			}
		}
	}
	if IsNumber(recv) {
		switch name {
		case "intValue":
			return plCastNum(recv, "int"), nil
		case "longValue":
			return plCastNum(recv, "long"), nil
		case "doubleValue":
			return plCastNum(recv, "double"), nil
		case "floatValue":
			return plCastNum(recv, "float"), nil
		case "shortValue":
			return plCastNum(recv, "short"), nil
		case "byteValue":
			return plCastNum(recv, "byte"), nil
		case "isNaN":
			return math.IsNaN(F64(recv)), nil
		case "isInfinite":
			return math.IsInf(F64(recv), 0), nil
		}
	}
	if b, ok := recv.(bool); ok && name == "booleanValue" {
		return b, nil
	}
	if c, ok := recv.(plChar); ok && name == "charValue" {
		return c, nil
	}
	return nil, plDynMethodErr(recv, name, n, off)
}

func plJavaReplacement(r string) string {
	var sb strings.Builder
	for i := 0; i < len(r); i++ {
		c := r[i]
		if c == '\\' && i+1 < len(r) {
			if r[i+1] == '$' {
				sb.WriteString("$$")
			} else {
				sb.WriteByte(r[i+1])
			}
			i++
			continue
		}
		if c == '$' {
			j := i + 1
			for j < len(r) && r[j] >= '0' && r[j] <= '9' {
				j++
			}
			if j > i+1 {
				sb.WriteString("${" + r[i+1:j] + "}")
				i = j - 1
				continue
			}
			if j < len(r) && r[j] == '{' {
				k := strings.IndexByte(r[j:], '}')
				if k > 0 {
					sb.WriteString("${" + r[j+1:j+k] + "}")
					i = j + k
					continue
				}
			}
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

func (in *plInterp) stringMethod(s, name string, args []any, off int) (any, *RuntimeError) {
	n := len(args)
	str := func(i int) string {
		if v, ok := args[i].(string); ok {
			return v
		}
		return ToString(args[i])
	}
	u := plUTF16(s)
	idx := func(i int) int { return int(I64(args[i])) }
	switch name + "/" + itoa(n) {
	case "length/0":
		return int32(len(u)), nil
	case "isEmpty/0":
		return len(u) == 0, nil
	case "isBlank/0":
		return strings.TrimSpace(s) == "", nil
	case "charAt/1":
		i := idx(0)
		if i < 0 || i >= len(u) {
			return nil, &RuntimeError{Type: "string_index_out_of_bounds_exception", Offset: off,
				Reason: "Index " + itoa(i) + " out of bounds for length " + itoa(len(u))}
		}
		return plChar(u[i]), nil
	case "codePointAt/1":
		i := idx(0)
		if i < 0 || i >= len(u) {
			return nil, &RuntimeError{Type: "string_index_out_of_bounds_exception", Offset: off,
				Reason: "Index " + itoa(i) + " out of bounds for length " + itoa(len(u))}
		}
		r := []rune(plFromUTF16(u[i:]))
		return int32(r[0]), nil
	case "substring/1", "substring/2", "subSequence/2":
		b := idx(0)
		e := len(u)
		if n == 2 {
			e = idx(1)
		}
		if b < 0 || e > len(u) || b > e {
			frames := []string{
				"java.base/jdk.internal.util.Preconditions$1.apply(Preconditions.java:55)",
				"java.base/jdk.internal.util.Preconditions$1.apply(Preconditions.java:52)",
				"java.base/jdk.internal.util.Preconditions$4.apply(Preconditions.java:213)",
				"java.base/jdk.internal.util.Preconditions$4.apply(Preconditions.java:210)",
				"java.base/jdk.internal.util.Preconditions.outOfBounds(Preconditions.java:98)",
				"java.base/jdk.internal.util.Preconditions.outOfBoundsCheckFromToIndex(Preconditions.java:112)",
				"java.base/jdk.internal.util.Preconditions.checkFromToIndex(Preconditions.java:349)",
				"java.base/java.lang.String.checkBoundsBeginEnd(String.java:4937)",
				"java.base/java.lang.String.substring(String.java:2899)",
			}
			if n == 1 {
				frames = nil
			}
			reason := "Range [" + itoa(b) + ", " + itoa(e) + ") out of bounds for length " + itoa(len(u))
			if n == 1 {
				reason = "begin " + itoa(b) + ", end " + itoa(e) + ", length " + itoa(len(u))
			}
			return nil, &RuntimeError{Type: "string_index_out_of_bounds_exception", Offset: off, Frames: frames, Reason: reason}
		}
		return plFromUTF16(u[b:e]), nil
	case "indexOf/1", "indexOf/2", "lastIndexOf/1", "lastIndexOf/2":
		var needle []uint16
		switch a := args[0].(type) {
		case string:
			needle = plUTF16(a)
		case plChar:
			needle = []uint16{uint16(a)}
		default:
			if IsNumber(a) {
				needle = plUTF16(string(rune(I64(a))))
			} else {
				needle = plUTF16(ToString(a))
			}
		}
		from := 0
		last := strings.HasPrefix(name, "last")
		if last {
			from = len(u)
		}
		if n == 2 {
			from = idx(1)
		}
		if !last {
			if from < 0 {
				from = 0
			}
			for i := from; i+len(needle) <= len(u); i++ {
				if plU16Eq(u[i:i+len(needle)], needle) {
					return int32(i), nil
				}
			}
			return int32(-1), nil
		}
		start := len(u) - len(needle)
		if from < start {
			start = from
		}
		for i := start; i >= 0; i-- {
			if plU16Eq(u[i:i+len(needle)], needle) {
				return int32(i), nil
			}
		}
		return int32(-1), nil
	case "contains/1":
		return strings.Contains(s, str(0)), nil
	case "contentEquals/1":
		return s == str(0), nil
	case "startsWith/1":
		return strings.HasPrefix(s, str(0)), nil
	case "startsWith/2":
		o := idx(1)
		if o < 0 || o > len(u) {
			return false, nil
		}
		return strings.HasPrefix(plFromUTF16(u[o:]), str(0)), nil
	case "endsWith/1":
		return strings.HasSuffix(s, str(0)), nil
	case "compareTo/1":
		if args[0] == nil {
			return nil, plNPE(off)
		}
		return int32(plStringCompare(s, str(0))), nil
	case "compareToIgnoreCase/1":
		return int32(plStringCompare(strings.ToLower(strings.ToUpper(s)), strings.ToLower(strings.ToUpper(str(0))))), nil
	case "equals/1":
		v, ok := args[0].(string)
		return ok && v == s, nil
	case "equalsIgnoreCase/1":
		v, ok := args[0].(string)
		return ok && strings.EqualFold(v, s), nil
	case "concat/1":
		return s + str(0), nil
	case "replace/2":
		a0, a1 := args[0], args[1]
		if c, ok := a0.(plChar); ok {
			a0 = ToString(c)
		}
		if c, ok := a1.(plChar); ok {
			a1 = ToString(c)
		}
		return strings.ReplaceAll(s, ToString(a0), ToString(a1)), nil
	case "trim/0":
		return strings.TrimFunc(s, func(r rune) bool { return r <= ' ' }), nil
	case "strip/0":
		return strings.TrimSpace(s), nil
	case "toUpperCase/0":
		return strings.ToUpper(s), nil
	case "toLowerCase/0":
		return strings.ToLower(s), nil
	case "toString/0":
		return s, nil
	case "hashCode/0":
		return plStringHash(s), nil
	case "chars/0", "codePoints/0":
		st := &plStream{kind: "int"}
		if name == "chars" {
			for _, c := range u {
				st.items = append(st.items, int32(c))
			}
		} else {
			for _, r := range s {
				st.items = append(st.items, int32(r))
			}
		}
		return st, nil
	case "toCharArray/0":
		arr := &Array{Elem: "char"}
		for _, c := range u {
			arr.Items = append(arr.Items, plChar(c))
		}
		return arr, nil
	case "encodeBase64/0":
		return base64.StdEncoding.EncodeToString([]byte(s)), nil
	case "decodeBase64/0":
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, plIAE(off, "Illegal base64 character")
		}
		return string(b), nil
	case "splitOnToken/1", "splitOnToken/2":
		tok := str(0)
		limit := -1
		if n == 2 {
			limit = idx(1)
		}
		var parts []string
		if limit > 0 {
			parts = strings.SplitN(s, tok, limit)
		} else {
			parts = strings.Split(s, tok)
		}
		arr := &Array{Elem: "String"}
		for _, p := range parts {
			arr.Items = append(arr.Items, p)
		}
		return arr, nil
	case "replaceAll/2", "replaceFirst/2":
		p, ok := args[0].(*plPattern)
		if !ok {
			return nil, &RuntimeError{Type: "class_cast_exception", Offset: off, Reason: "Cannot cast " + plClassName(args[0]) + " to java.util.regex.Pattern"}
		}
		var ferr *RuntimeError
		var sb strings.Builder
		last := 0
		locs := p.re.FindAllStringSubmatchIndex(s, -1)
		for i, loc := range locs {
			if name == "replaceFirst" && i > 0 {
				break
			}
			m := &plMatcher{p: p, input: s, in: in, loc: loc}
			r, err := in.callFn(args[1], []any{m}, off)
			if err != nil {
				ferr = err
				break
			}
			sb.WriteString(s[last:loc[0]])
			sb.WriteString(ToString(r))
			last = loc[1]
		}
		if ferr != nil {
			return nil, ferr
		}
		sb.WriteString(s[last:])
		return sb.String(), nil
	case "getClass/0":
		return nil, plDynMethodErr(s, name, n, off)
	case "regionMatches/4", "regionMatches/5":
		ignore := false
		a := args
		if n == 5 {
			ignore, _ = args[0].(bool)
			a = args[1:]
		}
		to, other, oo, ln := int(I64(a[0])), plUTF16(ToString(a[1])), int(I64(a[2])), int(I64(a[3]))
		if to < 0 || oo < 0 || to+ln > len(u) || oo+ln > len(other) {
			return false, nil
		}
		x, y := plFromUTF16(u[to:to+ln]), plFromUTF16(other[oo:oo+ln])
		if ignore {
			return strings.EqualFold(x, y), nil
		}
		return x == y, nil
	case "codePointCount/2":
		return int32(len([]rune(plFromUTF16(u[idx(0):idx(1)])))), nil
	}
	return nil, plDynMethodErr(s, name, n, off)
}

func plU16Eq(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func plSBMethod(t *plStringBuilder, name string, args []any, off int) (any, *RuntimeError, bool) {
	switch name {
	case "append":
		if len(args) == 1 {
			t.s = append(t.s, []rune(ToString(args[0]))...)
			return t, nil, true
		}
	case "toString":
		return string(t.s), nil, true
	case "reverse":
		for i, j := 0, len(t.s)-1; i < j; i, j = i+1, j-1 {
			t.s[i], t.s[j] = t.s[j], t.s[i]
		}
		return t, nil, true
	case "length":
		return int32(len(plUTF16(string(t.s)))), nil, true
	case "charAt":
		i := int(I64(args[0]))
		if i < 0 || i >= len(t.s) {
			return nil, &RuntimeError{Type: "string_index_out_of_bounds_exception", Offset: off,
				Reason: "index " + itoa(i) + ",length " + itoa(len(t.s))}, true
		}
		return plChar(t.s[i]), nil, true
	case "insert":
		i := int(I64(args[0]))
		ins := []rune(ToString(args[1]))
		t.s = append(t.s[:i], append(ins, t.s[i:]...)...)
		return t, nil, true
	case "deleteCharAt":
		i := int(I64(args[0]))
		t.s = append(t.s[:i], t.s[i+1:]...)
		return t, nil, true
	case "setLength":
		l := int(I64(args[0]))
		for len(t.s) < l {
			t.s = append(t.s, 0)
		}
		t.s = t.s[:l]
		return nil, nil, true
	case "indexOf":
		return int32(strings.Index(string(t.s), ToString(args[0]))), nil, true
	case "isEmpty":
		return len(t.s) == 0, nil, true
	}
	return nil, nil, false
}

func (in *plInterp) collectionAugment(items []any, name string, args []any, off int) (any, *RuntimeError, bool) {
	n := len(args)
	switch name + "/" + itoa(n) {
	case "sum/0", "sum/1":
		s := 0.0
		for _, it := range items {
			v := it
			if n == 1 {
				r, err := in.callFn(args[0], []any{it}, off)
				if err != nil {
					return nil, err, true
				}
				v = r
			}
			if !IsNumber(v) {
				return nil, &RuntimeError{Type: "class_cast_exception", Offset: off, Reason: "Cannot cast " + plClassName(v) + " to java.lang.Number"}, true
			}
			s += F64(v)
		}
		return s, nil, true
	case "collect/1":
		out := &List{}
		for _, it := range items {
			r, err := in.callFn(args[0], []any{it}, off)
			if err != nil {
				return nil, err, true
			}
			out.a = append(out.a, r)
		}
		return out, nil, true
	case "findAll/1":
		out := &List{}
		for _, it := range items {
			r, err := in.callFn(args[0], []any{it}, off)
			if err != nil {
				return nil, err, true
			}
			if b, _ := r.(bool); b {
				out.a = append(out.a, it)
			}
		}
		return out, nil, true
	case "find/1":
		for _, it := range items {
			r, err := in.callFn(args[0], []any{it}, off)
			if err != nil {
				return nil, err, true
			}
			if b, _ := r.(bool); b {
				return it, nil, true
			}
		}
		return nil, nil, true
	case "findResult/1":
		for _, it := range items {
			r, err := in.callFn(args[0], []any{it}, off)
			if err != nil {
				return nil, err, true
			}
			if r != nil {
				return r, nil, true
			}
		}
		return nil, nil, true
	case "any/1", "every/1":
		for _, it := range items {
			r, err := in.callFn(args[0], []any{it}, off)
			if err != nil {
				return nil, err, true
			}
			b, _ := r.(bool)
			if name == "any" && b {
				return true, nil, true
			}
			if name == "every" && !b {
				return false, nil, true
			}
		}
		return name == "every", nil, true
	case "join/1":
		parts := make([]string, len(items))
		for i, it := range items {
			parts[i] = ToString(it)
		}
		return strings.Join(parts, ToString(args[0])), nil, true
	case "each/1", "forEach/1":
		for _, it := range items {
			if _, err := in.callFn(args[0], []any{it}, off); err != nil {
				return nil, err, true
			}
		}
		return nil, nil, true
	case "eachWithIndex/1":
		for i, it := range items {
			if _, err := in.callFn(args[0], []any{it, int32(i)}, off); err != nil {
				return nil, err, true
			}
		}
		return nil, nil, true
	case "groupBy/1":
		m := NewMap(Hash)
		for _, it := range items {
			k, err := in.callFn(args[0], []any{it}, off)
			if err != nil {
				return nil, err, true
			}
			cur, ok := m.Get(k)
			if !ok {
				cur = &List{}
				m.Put(k, cur)
			}
			cur.(*List).a = append(cur.(*List).a, it)
		}
		return m, nil, true
	case "stream/0":
		return &plStream{kind: "obj", items: append([]any{}, items...)}, nil, true
	case "iterator/0":
		return &plIterator{items: append([]any{}, items...)}, nil, true
	case "asList/0":
		return NewList(items...), nil, true
	}
	return nil, nil, false
}

func (in *plInterp) listMethod(l *List, name string, args []any, off int) (any, *RuntimeError, bool) {
	n := len(args)
	switch name + "/" + itoa(n) {
	case "add/1":
		l.a = append(l.a, args[0])
		return true, nil, true
	case "add/2":
		i := int(I64(args[0]))
		if i < 0 || i > len(l.a) {
			return nil, plOOB(i, len(l.a), off, nil), true
		}
		l.a = append(l.a[:i], append([]any{args[1]}, l.a[i:]...)...)
		return nil, nil, true
	case "addAll/1":
		items, ok := ToItems(args[0])
		if !ok {
			return nil, plNPE(off), true
		}
		l.a = append(l.a, items...)
		return len(items) > 0, nil, true
	case "get/1":
		i := int(I64(args[0]))
		if i < 0 || i >= len(l.a) {
			return nil, plOOB(i, len(l.a), off, plArrayListGetFrames), true
		}
		return l.a[i], nil, true
	case "set/2":
		i := int(I64(args[0]))
		if i < 0 || i >= len(l.a) {
			return nil, plOOB(i, len(l.a), off, nil), true
		}
		old := l.a[i]
		l.a[i] = args[1]
		return old, nil, true
	case "remove/1":
		if IsNumber(args[0]) && plPrimName(args[0]) == "int" {
			i := int(I64(args[0]))
			if i < 0 || i >= len(l.a) {
				return nil, plOOB(i, len(l.a), off, nil), true
			}
			old := l.a[i]
			l.a = append(l.a[:i], l.a[i+1:]...)
			return old, nil, true
		}
		for i, v := range l.a {
			if plJavaEquals(v, args[0]) {
				l.a = append(l.a[:i], l.a[i+1:]...)
				return true, nil, true
			}
		}
		return false, nil, true
	case "removeIf/1":
		out := l.a[:0:0]
		removed := false
		for _, v := range l.a {
			r, err := in.callFn(args[0], []any{v}, off)
			if err != nil {
				return nil, err, true
			}
			if b, _ := r.(bool); b {
				removed = true
				continue
			}
			out = append(out, v)
		}
		l.a = out
		return removed, nil, true
	case "size/0", "getLength/0":
		return int32(len(l.a)), nil, true
	case "isEmpty/0":
		return len(l.a) == 0, nil, true
	case "contains/1":
		for _, v := range l.a {
			if plJavaEquals(v, args[0]) {
				return true, nil, true
			}
		}
		return false, nil, true
	case "containsAll/1":
		items, _ := ToItems(args[0])
		for _, it := range items {
			found := false
			for _, v := range l.a {
				if plJavaEquals(v, it) {
					found = true
					break
				}
			}
			if !found {
				return false, nil, true
			}
		}
		return true, nil, true
	case "indexOf/1", "lastIndexOf/1":
		res := int32(-1)
		for i, v := range l.a {
			if plJavaEquals(v, args[0]) {
				res = int32(i)
				if name == "indexOf" {
					break
				}
			}
		}
		return res, nil, true
	case "subList/2":
		b, e := int(I64(args[0])), int(I64(args[1]))
		if b < 0 || e > len(l.a) || b > e {
			return nil, &RuntimeError{Type: "index_out_of_bounds_exception", Offset: off,
				Reason: "fromIndex: " + itoa(b) + ", toIndex: " + itoa(e) + ", size: " + itoa(len(l.a))}, true
		}
		return NewList(l.a[b:e]...), nil, true
	case "sort/1":
		return nil, in.sortItems(l.a, args[0], off), true
	case "clear/0":
		l.a = nil
		return nil, nil, true
	case "toArray/0":
		return &Array{Elem: "Object", Items: append([]any{}, l.a...)}, nil, true
	case "replaceAll/1":
		for i, v := range l.a {
			r, err := in.callFn(args[0], []any{v}, off)
			if err != nil {
				return nil, err, true
			}
			l.a[i] = r
		}
		return nil, nil, true
	case "removeAll/1", "retainAll/1":
		items, _ := ToItems(args[0])
		out := []any{}
		for _, v := range l.a {
			found := false
			for _, it := range items {
				if plJavaEquals(v, it) {
					found = true
				}
			}
			if found == (name == "retainAll") {
				out = append(out, v)
			}
		}
		changed := len(out) != len(l.a)
		l.a = out
		return changed, nil, true
	case "getFirst/0", "getLast/0":
		if len(l.a) == 0 {
			return nil, &RuntimeError{Type: "no_such_element_exception", Offset: off, NilReason: true}, true
		}
		if name == "getFirst" {
			return l.a[0], nil, true
		}
		return l.a[len(l.a)-1], nil, true
	case "equals/1":
		return plJavaEquals(l, args[0]), nil, true
	case "hashCode/0":
		return plHashCode(l), nil, true
	case "toString/0":
		return ToString(l), nil, true
	}
	return in.collectionAugment(l.a, name, args, off)
}

func (in *plInterp) setMethod(s *Set, name string, args []any, off int) (any, *RuntimeError, bool) {
	switch name + "/" + itoa(len(args)) {
	case "add/1":
		return s.add(args[0]), nil, true
	case "addAll/1":
		items, _ := ToItems(args[0])
		changed := false
		for _, it := range items {
			if s.add(it) {
				changed = true
			}
		}
		return changed, nil, true
	case "contains/1":
		_, ok := s.m.Get(args[0])
		return ok, nil, true
	case "remove/1":
		_, ok := s.m.remove(args[0])
		return ok, nil, true
	case "size/0":
		return int32(s.m.Size()), nil, true
	case "isEmpty/0":
		return s.m.Size() == 0, nil, true
	case "clear/0":
		s.m = NewMap(s.m.kind)
		return nil, nil, true
	case "removeIf/1":
		removed := false
		for _, it := range s.items() {
			r, err := in.callFn(args[0], []any{it}, off)
			if err != nil {
				return nil, err, true
			}
			if b, _ := r.(bool); b {
				s.m.remove(it)
				removed = true
			}
		}
		return removed, nil, true
	case "containsAll/1":
		items, _ := ToItems(args[0])
		for _, it := range items {
			if _, ok := s.m.Get(it); !ok {
				return false, nil, true
			}
		}
		return true, nil, true
	}
	return in.collectionAugment(s.items(), name, args, off)
}

func (in *plInterp) mapMethod(m *Map, name string, args []any, off int) (any, *RuntimeError, bool) {
	n := len(args)
	call2 := func(fn any, e *plMapEntry) (any, *RuntimeError) {
		if f, ok := fn.(*plFn); ok && f.native == nil && len(f.pnames) == 1 {
			return in.callFn(fn, []any{&plMapEntryRef{m: m, e: e}}, off)
		}
		return in.callFn(fn, []any{e.Key, e.Value}, off)
	}
	switch name + "/" + itoa(n) {
	case "get/1":
		v, _ := m.Get(args[0])
		return v, nil, true
	case "put/2":
		return m.Put(args[0], args[1]), nil, true
	case "putAll/1":
		if src, ok := args[0].(*Map); ok {
			for _, e := range src.ordered() {
				m.Put(e.Key, e.Value)
			}
		}
		return nil, nil, true
	case "putIfAbsent/2":
		if v, ok := m.Get(args[0]); ok && v != nil {
			return v, nil, true
		}
		m.Put(args[0], args[1])
		return nil, nil, true
	case "containsKey/1":
		_, ok := m.Get(args[0])
		return ok, nil, true
	case "containsValue/1":
		for _, e := range m.entries {
			if plJavaEquals(e.Value, args[0]) {
				return true, nil, true
			}
		}
		return false, nil, true
	case "remove/1":
		v, _ := m.remove(args[0])
		return v, nil, true
	case "size/0":
		return int32(m.Size()), nil, true
	case "isEmpty/0":
		return m.Size() == 0, nil, true
	case "clear/0":
		*m = *NewMap(m.kind)
		return nil, nil, true
	case "keySet/0":
		s := newPlSet(Linked)
		for _, k := range m.keys() {
			s.add(k)
		}
		return s, nil, true
	case "values/0":
		return NewList(m.values()...), nil, true
	case "entrySet/0":
		l := &List{}
		for _, e := range m.ordered() {
			l.a = append(l.a, &plMapEntryRef{m: m, e: e})
		}
		return &plEntrySet{l}, nil, true
	case "getOrDefault/2":
		if v, ok := m.Get(args[0]); ok {
			return v, nil, true
		}
		return args[1], nil, true
	case "computeIfAbsent/2":
		if v, ok := m.Get(args[0]); ok && v != nil {
			return v, nil, true
		}
		v, err := in.callFn(args[1], []any{args[0]}, off)
		if err != nil {
			return nil, err, true
		}
		if v != nil {
			m.Put(args[0], v)
		}
		return v, nil, true
	case "computeIfPresent/2", "compute/2":
		old, ok := m.Get(args[0])
		if name == "computeIfPresent" && (!ok || old == nil) {
			return nil, nil, true
		}
		v, err := in.callFn(args[1], []any{args[0], old}, off)
		if err != nil {
			return nil, err, true
		}
		if v == nil {
			m.remove(args[0])
		} else {
			m.Put(args[0], v)
		}
		return v, nil, true
	case "merge/3":
		old, ok := m.Get(args[0])
		v := args[1]
		if ok && old != nil {
			r, err := in.callFn(args[2], []any{old, args[1]}, off)
			if err != nil {
				return nil, err, true
			}
			v = r
		}
		if v == nil {
			m.remove(args[0])
		} else {
			m.Put(args[0], v)
		}
		return v, nil, true
	case "forEach/1", "each/1":
		for _, e := range m.ordered() {
			if _, err := call2(args[0], e); err != nil {
				return nil, err, true
			}
		}
		return nil, nil, true
	case "collect/1":
		out := &List{}
		for _, e := range m.ordered() {
			r, err := call2(args[0], e)
			if err != nil {
				return nil, err, true
			}
			out.a = append(out.a, r)
		}
		return out, nil, true
	case "findAll/1":
		out := NewMap(m.kind)
		for _, e := range m.ordered() {
			r, err := call2(args[0], e)
			if err != nil {
				return nil, err, true
			}
			if b, _ := r.(bool); b {
				out.Put(e.Key, e.Value)
			}
		}
		return out, nil, true
	case "find/1":
		for _, e := range m.ordered() {
			r, err := call2(args[0], e)
			if err != nil {
				return nil, err, true
			}
			if b, _ := r.(bool); b {
				return &plMapEntryRef{m: m, e: e}, nil, true
			}
		}
		return nil, nil, true
	case "any/1", "every/1":
		for _, e := range m.ordered() {
			r, err := call2(args[0], e)
			if err != nil {
				return nil, err, true
			}
			b, _ := r.(bool)
			if name == "any" && b {
				return true, nil, true
			}
			if name == "every" && !b {
				return false, nil, true
			}
		}
		return name == "every", nil, true
	case "count/1":
		c := int32(0)
		for _, e := range m.ordered() {
			r, err := call2(args[0], e)
			if err != nil {
				return nil, err, true
			}
			if b, _ := r.(bool); b {
				c++
			}
		}
		return c, nil, true
	case "equals/1":
		return plJavaEquals(m, args[0]), nil, true
	case "hashCode/0":
		return plHashCode(m), nil, true
	case "toString/0":
		return ToString(m), nil, true
	}
	return nil, nil, false
}

// plEntrySet is Map.entrySet(): a set view over the entries in map order.
type plEntrySet struct{ l *List }

func (s *plEntrySet) javaString() string { return ToString(s.l) }

// ---- streams ----

func (in *plInterp) streamMethod(s *plStream, name string, args []any, off int) (any, *RuntimeError, bool) {
	n := len(args)
	optName := map[string]string{"obj": "Optional", "int": "OptionalInt", "long": "OptionalLong", "double": "OptionalDouble"}[s.kind]
	mapEach := func(kind string) (any, *RuntimeError, bool) {
		out := &plStream{kind: kind}
		for _, it := range s.items {
			r, err := in.callFn(args[0], []any{it}, off)
			if err != nil {
				return nil, err, true
			}
			switch kind {
			case "int", "long", "double":
				if !IsNumber(r) {
					return nil, &RuntimeError{Type: "class_cast_exception", Offset: off, Reason: "Cannot cast " + plClassName(r) + " to " + kind}, true
				}
				r = plCastNum(r, kind)
			}
			out.items = append(out.items, r)
		}
		return out, nil, true
	}
	switch name + "/" + itoa(n) {
	case "map/1":
		return mapEach(s.kind)
	case "mapToObj/1":
		return mapEach("obj")
	case "mapToInt/1":
		return mapEach("int")
	case "mapToLong/1":
		return mapEach("long")
	case "mapToDouble/1":
		return mapEach("double")
	case "boxed/0":
		return &plStream{kind: "obj", items: s.items}, nil, true
	case "asDoubleStream/0":
		out := &plStream{kind: "double"}
		for _, it := range s.items {
			out.items = append(out.items, F64(it))
		}
		return out, nil, true
	case "asLongStream/0":
		out := &plStream{kind: "long"}
		for _, it := range s.items {
			out.items = append(out.items, I64(it))
		}
		return out, nil, true
	case "filter/1":
		out := &plStream{kind: s.kind}
		for _, it := range s.items {
			r, err := in.callFn(args[0], []any{it}, off)
			if err != nil {
				return nil, err, true
			}
			if b, _ := r.(bool); b {
				out.items = append(out.items, it)
			}
		}
		return out, nil, true
	case "flatMap/1":
		out := &plStream{kind: s.kind}
		for _, it := range s.items {
			r, err := in.callFn(args[0], []any{it}, off)
			if err != nil {
				return nil, err, true
			}
			items, _ := ToItems(r)
			out.items = append(out.items, items...)
		}
		return out, nil, true
	case "sorted/0", "sorted/1":
		items := append([]any{}, s.items...)
		var cmp any
		if n == 1 {
			cmp = args[0]
		}
		if err := in.sortItems(items, cmp, off); err != nil {
			return nil, err, true
		}
		return &plStream{kind: s.kind, items: items}, nil, true
	case "distinct/0":
		seen := newPlSet(Linked)
		for _, it := range s.items {
			seen.add(it)
		}
		return &plStream{kind: s.kind, items: seen.items()}, nil, true
	case "limit/1":
		k := int(I64(args[0]))
		if k > len(s.items) {
			k = len(s.items)
		}
		return &plStream{kind: s.kind, items: s.items[:k]}, nil, true
	case "skip/1":
		k := int(I64(args[0]))
		if k > len(s.items) {
			k = len(s.items)
		}
		return &plStream{kind: s.kind, items: s.items[k:]}, nil, true
	case "peek/1", "forEach/1", "forEachOrdered/1":
		for _, it := range s.items {
			if _, err := in.callFn(args[0], []any{it}, off); err != nil {
				return nil, err, true
			}
		}
		if name == "peek" {
			return s, nil, true
		}
		return nil, nil, true
	case "count/0":
		return int64(len(s.items)), nil, true
	case "sum/0":
		switch s.kind {
		case "int":
			var t int32
			for _, it := range s.items {
				t += int32(I64(it))
			}
			return t, nil, true
		case "long":
			var t int64
			for _, it := range s.items {
				t += I64(it)
			}
			return t, nil, true
		}
		t := 0.0
		for _, it := range s.items {
			t += F64(it)
		}
		return t, nil, true
	case "average/0":
		if len(s.items) == 0 {
			return &plOptional{name: "OptionalDouble"}, nil, true
		}
		t := 0.0
		for _, it := range s.items {
			t += F64(it)
		}
		return &plOptional{name: "OptionalDouble", v: t / float64(len(s.items)), present: true}, nil, true
	case "max/0", "min/0", "max/1", "min/1":
		if len(s.items) == 0 {
			return &plOptional{name: optName}, nil, true
		}
		var cmp any
		if n == 1 {
			cmp = args[0]
		}
		best := s.items[0]
		for _, it := range s.items[1:] {
			c, err := in.compareWith(cmp, it, best, off)
			if err != nil {
				return nil, err, true
			}
			if (name == "max" && c > 0) || (name == "min" && c < 0) {
				best = it
			}
		}
		return &plOptional{name: optName, v: best, present: true}, nil, true
	case "reduce/1":
		if len(s.items) == 0 {
			return &plOptional{name: optName}, nil, true
		}
		acc := s.items[0]
		for _, it := range s.items[1:] {
			r, err := in.callFn(args[0], []any{acc, it}, off)
			if err != nil {
				return nil, err, true
			}
			acc = r
		}
		return &plOptional{name: optName, v: acc, present: true}, nil, true
	case "reduce/2":
		acc := args[0]
		for _, it := range s.items {
			r, err := in.callFn(args[1], []any{acc, it}, off)
			if err != nil {
				return nil, err, true
			}
			acc = r
		}
		if s.kind != "obj" && IsNumber(acc) {
			acc = plCastNum(acc, s.kind)
		}
		return acc, nil, true
	case "anyMatch/1", "allMatch/1", "noneMatch/1":
		for _, it := range s.items {
			r, err := in.callFn(args[0], []any{it}, off)
			if err != nil {
				return nil, err, true
			}
			b, _ := r.(bool)
			switch {
			case name == "anyMatch" && b:
				return true, nil, true
			case name == "allMatch" && !b:
				return false, nil, true
			case name == "noneMatch" && b:
				return false, nil, true
			}
		}
		return name != "anyMatch", nil, true
	case "findFirst/0", "findAny/0":
		if len(s.items) == 0 {
			return &plOptional{name: optName}, nil, true
		}
		return &plOptional{name: optName, v: s.items[0], present: true}, nil, true
	case "toArray/0":
		elem := "Object"
		if s.kind != "obj" {
			elem = s.kind
		}
		return &Array{Elem: elem, Items: append([]any{}, s.items...)}, nil, true
	case "toList/0":
		return NewList(s.items...), nil, true
	case "iterator/0":
		return &plIterator{items: s.items}, nil, true
	case "collect/1":
		c, ok := args[0].(*plCollector)
		if !ok {
			return nil, &RuntimeError{Type: "class_cast_exception", Offset: off, Reason: "Cannot cast " + plClassName(args[0]) + " to java.util.stream.Collector"}, true
		}
		v, err := in.collect(c, s.items, off)
		return v, err, true
	}
	return nil, nil, false
}

func (in *plInterp) collect(c *plCollector, items []any, off int) (any, *RuntimeError) {
	switch c.kind {
	case "toList":
		return NewList(items...), nil
	case "toSet":
		s := newPlSet(Hash)
		for _, it := range items {
			s.add(it)
		}
		return s, nil
	case "joining":
		parts := make([]string, len(items))
		for i, it := range items {
			s, ok := it.(string)
			if !ok {
				return nil, &RuntimeError{Type: "class_cast_exception", Offset: off,
					Reason: "class " + plClassName(it) + " cannot be cast to class java.lang.CharSequence"}
			}
			parts[i] = s
		}
		return c.prefix + strings.Join(parts, c.sep) + c.suffix, nil
	case "counting":
		return int64(len(items)), nil
	case "groupingBy":
		m := NewMap(Hash)
		for _, it := range items {
			k, err := in.callFn(c.f1, []any{it}, off)
			if err != nil {
				return nil, err
			}
			cur, ok := m.Get(k)
			if !ok {
				cur = &List{}
				m.Put(k, cur)
			}
			cur.(*List).a = append(cur.(*List).a, it)
		}
		if sub, ok := c.f2.(*plCollector); ok {
			for _, e := range m.entries {
				v, err := in.collect(sub, e.Value.(*List).a, off)
				if err != nil {
					return nil, err
				}
				e.Value = v
			}
		}
		return m, nil
	case "toMap":
		m := NewMap(Hash)
		for _, it := range items {
			k, err := in.callFn(c.f1, []any{it}, off)
			if err != nil {
				return nil, err
			}
			v, err := in.callFn(c.f2, []any{it}, off)
			if err != nil {
				return nil, err
			}
			if _, dup := m.Get(k); dup {
				return nil, &RuntimeError{Type: "illegal_state_exception", Offset: off, Reason: "Duplicate key " + ToString(k)}
			}
			m.Put(k, v)
		}
		return m, nil
	case "summingInt", "summingLong", "summingDouble", "averagingInt", "averagingDouble":
		total := 0.0
		var itotal int64
		for _, it := range items {
			v, err := in.callFn(c.f1, []any{it}, off)
			if err != nil {
				return nil, err
			}
			total += F64(v)
			itotal += I64(v)
		}
		switch c.kind {
		case "summingInt":
			return int32(itotal), nil
		case "summingLong":
			return itotal, nil
		case "summingDouble":
			return total, nil
		}
		if len(items) == 0 {
			return 0.0, nil
		}
		return total / float64(len(items)), nil
	case "mapping":
		mapped := []any{}
		for _, it := range items {
			v, err := in.callFn(c.f1, []any{it}, off)
			if err != nil {
				return nil, err
			}
			mapped = append(mapped, v)
		}
		if sub, ok := c.f2.(*plCollector); ok {
			return in.collect(sub, mapped, off)
		}
		return NewList(mapped...), nil
	}
	return nil, plIAE(off, "unsupported collector ["+c.kind+"]")
}
