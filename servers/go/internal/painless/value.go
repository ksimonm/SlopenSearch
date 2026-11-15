package painless

import (
	"encoding/json"
	"fmt"
	"math"
	"slopensearch/internal/javafmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Painless engine: runtime values with Java semantics.
//
// Primitive (boxed) values are Go scalars of the matching width: int32 is
// java.lang.Integer, int64 Long, float32 Float, float64 Double, int8 Byte,
// int16 Short, plChar Character, bool Boolean, string String, nil null.

type plChar uint16

type List struct{ a []any }

const (
	Hash = iota
	Linked
	plTree
)

type plMapEntry struct {
	Key, Value any
}

// Map is a java.util.Map: HashMap (hash iteration order), LinkedHashMap
// (insertion order) or TreeMap (sorted keys).
type Map struct {
	kind    int
	entries []*plMapEntry
	index   map[string]int
	cap     int
}

type Set struct{ m *Map }

type Array struct {
	Elem  string
	Items []any
}

type plStringBuilder struct{ s []rune }

type plMapEntryRef struct {
	m *Map
	e *plMapEntry
}

type plIterator struct {
	items []any
	i     int
}

func NewList(items ...any) *List { return &List{a: append([]any{}, items...)} }

func NewMap(kind int) *Map {
	return &Map{kind: kind, index: map[string]int{}, cap: 16}
}

func newPlSet(kind int) *Set { return &Set{m: NewMap(kind)} }

// plKeyID is an identity for map keys following Java equals: Integer 1 and
// Long 1 are different keys.
func plKeyID(k any) string {
	switch t := k.(type) {
	case nil:
		return "n"
	case string:
		return "s" + t
	case int32:
		return "i" + strconv.FormatInt(int64(t), 10)
	case int64:
		return "l" + strconv.FormatInt(t, 10)
	case int16:
		return "h" + strconv.FormatInt(int64(t), 10)
	case int8:
		return "b" + strconv.FormatInt(int64(t), 10)
	case plChar:
		return "c" + strconv.Itoa(int(t))
	case float64:
		return "d" + strconv.FormatUint(math.Float64bits(t), 16)
	case float32:
		return "f" + strconv.FormatUint(uint64(math.Float32bits(t)), 16)
	case bool:
		if t {
			return "zt"
		}
		return "zf"
	}
	return "o" + ToString(k)
}

func (m *Map) Get(k any) (any, bool) {
	if i, ok := m.index[plKeyID(k)]; ok {
		return m.entries[i].Value, true
	}
	return nil, false
}

func (m *Map) Put(k, v any) any {
	id := plKeyID(k)
	if i, ok := m.index[id]; ok {
		old := m.entries[i].Value
		m.entries[i].Value = v
		return old
	}
	m.index[id] = len(m.entries)
	m.entries = append(m.entries, &plMapEntry{Key: k, Value: v})
	for float64(len(m.entries)) > float64(m.cap)*0.75 {
		m.cap *= 2
	}
	return nil
}

func (m *Map) remove(k any) (any, bool) {
	id := plKeyID(k)
	i, ok := m.index[id]
	if !ok {
		return nil, false
	}
	old := m.entries[i].Value
	m.entries = append(m.entries[:i], m.entries[i+1:]...)
	m.reindex()
	return old, true
}

func (m *Map) reindex() {
	m.index = map[string]int{}
	for i, e := range m.entries {
		m.index[plKeyID(e.Key)] = i
	}
}

func (m *Map) Size() int { return len(m.entries) }

// Entries returns the entries in insertion order.
func (m *Map) Entries() []*plMapEntry { return m.entries }

// ordered returns the entries in Java iteration order.
func (m *Map) ordered() []*plMapEntry {
	out := append([]*plMapEntry{}, m.entries...)
	switch m.kind {
	case Hash:
		mask := int32(m.cap - 1)
		sort.SliceStable(out, func(i, j int) bool {
			return plSpread(plHashCode(out[i].Key))&mask < plSpread(plHashCode(out[j].Key))&mask
		})
	case plTree:
		sort.SliceStable(out, func(i, j int) bool {
			c, _ := Compare(out[i].Key, out[j].Key)
			return c < 0
		})
	}
	return out
}

func plSpread(h int32) int32 { return h ^ int32(uint32(h)>>16) }

func (m *Map) keys() []any {
	out := []any{}
	for _, e := range m.ordered() {
		out = append(out, e.Key)
	}
	return out
}

func (m *Map) values() []any {
	out := []any{}
	for _, e := range m.ordered() {
		out = append(out, e.Value)
	}
	return out
}

func (m *Map) copyMap(kind int) *Map {
	n := NewMap(kind)
	for _, e := range m.ordered() {
		n.Put(e.Key, e.Value)
	}
	return n
}

func (s *Set) add(v any) bool {
	if _, ok := s.m.Get(v); ok {
		return false
	}
	s.m.Put(v, true)
	return true
}

func (s *Set) items() []any { return s.m.keys() }

// ---- hashing / equality / ordering ----

func plStringHash(s string) int32 {
	var h int32
	for _, u := range utf16.Encode([]rune(s)) {
		h = 31*h + int32(u)
	}
	return h
}

func plHashCode(v any) int32 {
	switch t := v.(type) {
	case nil:
		return 0
	case string:
		return plStringHash(t)
	case int32:
		return t
	case int16:
		return int32(t)
	case int8:
		return int32(t)
	case plChar:
		return int32(t)
	case int64:
		return int32(t ^ int64(uint64(t)>>32))
	case float64:
		if t == 0 && !math.Signbit(t) {
			return 0
		}
		b := math.Float64bits(t)
		if math.IsNaN(t) {
			b = 0x7ff8000000000000
		}
		return int32(int64(b) ^ int64(b>>32))
	case float32:
		if math.IsNaN(float64(t)) {
			return 0x7fc00000
		}
		return int32(math.Float32bits(t))
	case bool:
		if t {
			return 1231
		}
		return 1237
	case *List:
		var h int32 = 1
		for _, e := range t.a {
			h = 31*h + plHashCode(e)
		}
		return h
	case *Map:
		var h int32
		for _, e := range t.entries {
			h += plHashCode(e.Key) ^ plHashCode(e.Value)
		}
		return h
	case *Set:
		var h int32
		for _, e := range t.m.entries {
			h += plHashCode(e.Key)
		}
		return h
	}
	return plStringHash(ToString(v))
}

func IsNumber(v any) bool {
	switch v.(type) {
	case int32, int64, float32, float64, int8, int16, plChar:
		return true
	}
	return false
}

// plEquals implements Painless == on def values (numbers compare by value
// after promotion, everything else via equals()).
func plEquals(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if IsNumber(a) && IsNumber(b) {
		switch plPromote(a, b) {
		case "double", "float":
			return F64(a) == F64(b)
		default:
			return I64(a) == I64(b)
		}
	}
	return plJavaEquals(a, b)
}

// plJavaEquals is Object.equals: boxed numbers are only equal to the same type.
func plJavaEquals(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	switch x := a.(type) {
	case *List:
		y, ok := b.(*List)
		if !ok {
			if dv, ok := b.(*DocValues); ok {
				y = &List{a: dv.Values}
			} else {
				return false
			}
		}
		if len(x.a) != len(y.a) {
			return false
		}
		for i := range x.a {
			if !plJavaEquals(x.a[i], y.a[i]) {
				return false
			}
		}
		return true
	case *Map:
		y, ok := b.(*Map)
		if !ok || x.Size() != y.Size() {
			return false
		}
		for _, e := range x.entries {
			v, ok := y.Get(e.Key)
			if !ok || !plJavaEquals(e.Value, v) {
				return false
			}
		}
		return true
	case *Set:
		y, ok := b.(*Set)
		if !ok || x.m.Size() != y.m.Size() {
			return false
		}
		for _, e := range x.m.entries {
			if _, ok := y.m.Get(e.Key); !ok {
				return false
			}
		}
		return true
	case float64:
		y, ok := b.(float64)
		return ok && math.Float64bits(x) == math.Float64bits(y) || ok && x == y && x != 0
	case float32:
		y, ok := b.(float32)
		return ok && x == y
	}
	if IsNumber(a) || IsNumber(b) {
		return fmt.Sprintf("%T", a) == fmt.Sprintf("%T", b) && I64(a) == I64(b)
	}
	switch x := a.(type) {
	case string:
		y, ok := b.(string)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case *plZDT:
		y, ok := b.(*plZDT)
		return ok && x.t.Equal(y.t)
	case *plLocalDate:
		y, ok := b.(*plLocalDate)
		return ok && x.t.Equal(y.t)
	}
	return a == b
}

// plIdentical implements === (reference equality). Boxed integral values
// from -128..127 are cached by the JVM and therefore identical.
func plIdentical(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	switch x := a.(type) {
	case int32, int64, int16, int8, plChar:
		if fmt.Sprintf("%T", a) != fmt.Sprintf("%T", b) {
			return false
		}
		v := I64(x)
		return v == I64(b) && v >= -128 && v <= 127
	case float32, float64:
		return false
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	}
	return a == b
}

// Compare implements Comparable.compareTo for naturally ordered values.
func Compare(a, b any) (int, bool) {
	if IsNumber(a) && IsNumber(b) {
		switch plPromote(a, b) {
		case "double", "float":
			x, y := F64(a), F64(b)
			switch {
			case x < y:
				return -1, true
			case x > y:
				return 1, true
			}
			return 0, true
		default:
			x, y := I64(a), I64(b)
			switch {
			case x < y:
				return -1, true
			case x > y:
				return 1, true
			}
			return 0, true
		}
	}
	switch x := a.(type) {
	case string:
		if y, ok := b.(string); ok {
			return plStringCompare(x, y), true
		}
	case bool:
		if y, ok := b.(bool); ok {
			if x == y {
				return 0, true
			}
			if !x {
				return -1, true
			}
			return 1, true
		}
	case *plZDT:
		if y, ok := b.(*plZDT); ok {
			return x.t.Compare(y.t), true
		}
	case *plLocalDate:
		if y, ok := b.(*plLocalDate); ok {
			return x.t.Compare(y.t), true
		}
	}
	return 0, false
}

// plStringCompare is String.compareTo over UTF-16 code units.
func plStringCompare(a, b string) int {
	ua := utf16.Encode([]rune(a))
	ub := utf16.Encode([]rune(b))
	n := len(ua)
	if len(ub) < n {
		n = len(ub)
	}
	for i := 0; i < n; i++ {
		if ua[i] != ub[i] {
			return int(ua[i]) - int(ub[i])
		}
	}
	return len(ua) - len(ub)
}

// ---- numbers ----

func plPromote(a, b any) string {
	ta, tb := plPrimName(a), plPrimName(b)
	switch {
	case ta == "double" || tb == "double":
		return "double"
	case ta == "float" || tb == "float":
		return "float"
	case ta == "long" || tb == "long":
		return "long"
	}
	return "int"
}

func plPrimName(v any) string {
	switch v.(type) {
	case int32:
		return "int"
	case int64:
		return "long"
	case float32:
		return "float"
	case float64:
		return "double"
	case int8:
		return "byte"
	case int16:
		return "short"
	case plChar:
		return "char"
	case bool:
		return "boolean"
	}
	return ""
}

func I64(v any) int64 {
	switch t := v.(type) {
	case int32:
		return int64(t)
	case int64:
		return t
	case int16:
		return int64(t)
	case int8:
		return int64(t)
	case plChar:
		return int64(t)
	case float32:
		return plF2L(float64(t))
	case float64:
		return plF2L(t)
	}
	return 0
}

func F64(v any) float64 {
	switch t := v.(type) {
	case int32:
		return float64(t)
	case int64:
		return float64(t)
	case int16:
		return float64(t)
	case int8:
		return float64(t)
	case plChar:
		return float64(t)
	case float32:
		return float64(t)
	case float64:
		return t
	}
	return 0
}

// plF2I / plF2L are Java's (int)/(long) casts of a double.
func plF2I(f float64) int32 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= math.MaxInt32:
		return math.MaxInt32
	case f <= math.MinInt32:
		return math.MinInt32
	}
	return int32(f)
}

func plF2L(f float64) int64 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= math.MaxInt64:
		return math.MaxInt64
	case f <= math.MinInt64:
		return math.MinInt64
	}
	return int64(f)
}

// plCastNum converts a numeric value to the given primitive type with Java
// narrowing/widening semantics.
func plCastNum(v any, to string) any {
	isFloat := false
	switch v.(type) {
	case float32, float64:
		isFloat = true
	}
	switch to {
	case "int":
		if isFloat {
			return plF2I(F64(v))
		}
		return int32(I64(v))
	case "long":
		if isFloat {
			return plF2L(F64(v))
		}
		return I64(v)
	case "short":
		if isFloat {
			return int16(plF2I(F64(v)))
		}
		return int16(I64(v))
	case "byte":
		if isFloat {
			return int8(plF2I(F64(v)))
		}
		return int8(I64(v))
	case "char":
		if isFloat {
			return plChar(uint16(plF2I(F64(v))))
		}
		return plChar(uint16(I64(v)))
	case "float":
		if _, ok := v.(float64); ok {
			return float32(v.(float64))
		}
		if l, ok := v.(int64); ok {
			return float32(l)
		}
		return float32(F64(v))
	case "double":
		return F64(v)
	}
	return v
}

// plParseNumber parses a numeric literal the way the Painless compiler does.
func plParseNumber(text string, isFloat, neg bool) (any, string, error) {
	t := text
	if isFloat {
		suffix := t[len(t)-1]
		body := t
		typ := "double"
		if suffix == 'f' || suffix == 'F' {
			typ = "float"
			body = t[:len(t)-1]
		} else if suffix == 'd' || suffix == 'D' {
			body = t[:len(t)-1]
		}
		if neg {
			body = "-" + body
		}
		if typ == "float" {
			f, err := strconv.ParseFloat(body, 32)
			if err != nil && !strings.Contains(err.Error(), "range") {
				return nil, "", fmt.Errorf("Invalid float constant [%s].", text)
			}
			return float32(f), "float", nil
		}
		f, err := strconv.ParseFloat(body, 64)
		if err != nil && !strings.Contains(err.Error(), "range") {
			return nil, "", fmt.Errorf("Invalid double constant [%s].", text)
		}
		return f, "double", nil
	}
	long := false
	if strings.HasSuffix(t, "l") || strings.HasSuffix(t, "L") {
		long = true
		t = t[:len(t)-1]
	}
	base := 10
	if strings.HasPrefix(t, "0x") || strings.HasPrefix(t, "0X") {
		base = 16
		t = t[2:]
	} else if len(t) > 1 && t[0] == '0' {
		base = 8
		t = t[1:]
	}
	if neg {
		t = "-" + t
	}
	if long {
		n, err := strconv.ParseInt(t, base, 64)
		if err != nil {
			u, uerr := strconv.ParseUint(strings.TrimPrefix(t, "-"), base, 64)
			if uerr != nil || base == 10 {
				return nil, "", fmt.Errorf("Invalid long constant [%s].", text)
			}
			n = int64(u)
			if neg {
				n = -n
			}
		}
		return n, "long", nil
	}
	n, err := strconv.ParseInt(t, base, 32)
	if err != nil {
		u, uerr := strconv.ParseUint(strings.TrimPrefix(t, "-"), base, 32)
		if uerr != nil || base == 10 {
			return nil, "", fmt.Errorf("Invalid int constant [%s].", text)
		}
		n = int64(int32(uint32(u)))
		if neg {
			n = -n
		}
	}
	return int32(n), "int", nil
}

// ---- string conversion ----

// FormatDouble is Double.toString.
func FormatDouble(f float64) string { return javafmt.Double(f) }

func plFmtFloat(f float32) string { return javafmt.Float(f) }

func ToString(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case int16:
		return strconv.FormatInt(int64(t), 10)
	case int8:
		return strconv.FormatInt(int64(t), 10)
	case plChar:
		return string(utf16.Decode([]uint16{uint16(t)}))
	case float64:
		return FormatDouble(t)
	case float32:
		return plFmtFloat(t)
	case *List:
		return plJoinToString(t.a)
	case *DocValues:
		return plJoinToString(t.Values)
	case *Set:
		return plJoinToString(t.items())
	case *Map:
		var sb strings.Builder
		sb.WriteByte('{')
		for i, e := range t.ordered() {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(ToString(e.Key))
			sb.WriteByte('=')
			sb.WriteString(ToString(e.Value))
		}
		sb.WriteByte('}')
		return sb.String()
	case *plMapEntryRef:
		return ToString(t.e.Key) + "=" + ToString(t.e.Value)
	case *plStringBuilder:
		return string(t.s)
	case *Array:
		return "[" + plArrayCode(t.Elem) + "@" + fmt.Sprintf("%x", plHashCode(t.Items))
	case *plPattern:
		return t.src
	case *plOptional:
		if !t.present {
			return t.name + ".empty"
		}
		return t.name + "[" + ToString(t.v) + "]"
	case *GeoPoint:
		return FormatDouble(t.Lat) + ", " + FormatDouble(t.Lon)
	case plStringer:
		return t.javaString()
	}
	return fmt.Sprint(v)
}

type plStringer interface{ javaString() string }

func plArrayCode(elem string) string {
	switch elem {
	case "int":
		return "I"
	case "long":
		return "J"
	case "double":
		return "D"
	case "float":
		return "F"
	case "boolean":
		return "Z"
	case "byte":
		return "B"
	case "short":
		return "S"
	case "char":
		return "C"
	}
	return "L" + plCanon(elem) + ";"
}

func plJoinToString(items []any) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, e := range items {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(ToString(e))
	}
	sb.WriteByte(']')
	return sb.String()
}

// ---- type names ----

var plCanonNames = map[string]string{
	"String": "java.lang.String", "Object": "java.lang.Object", "Integer": "java.lang.Integer",
	"Long": "java.lang.Long", "Double": "java.lang.Double", "Float": "java.lang.Float",
	"Short": "java.lang.Short", "Byte": "java.lang.Byte", "Character": "java.lang.Character",
	"Boolean": "java.lang.Boolean", "Number": "java.lang.Number", "Math": "java.lang.Math",
	"StrictMath": "java.lang.StrictMath", "CharSequence": "java.lang.CharSequence",
	"StringBuilder": "java.lang.StringBuilder", "Iterable": "java.lang.Iterable",
	"List": "java.util.List", "ArrayList": "java.util.ArrayList", "LinkedList": "java.util.LinkedList",
	"Map": "java.util.Map", "HashMap": "java.util.HashMap", "TreeMap": "java.util.TreeMap",
	"LinkedHashMap": "java.util.LinkedHashMap", "Set": "java.util.Set", "HashSet": "java.util.HashSet",
	"TreeSet": "java.util.TreeSet", "LinkedHashSet": "java.util.LinkedHashSet",
	"Collection": "java.util.Collection", "Collections": "java.util.Collections",
	"Collectors": "java.util.stream.Collectors", "Comparator": "java.util.Comparator",
	"Iterator": "java.util.Iterator", "Arrays": "java.util.Arrays", "Objects": "java.util.Objects",
	"Optional": "java.util.Optional", "OptionalInt": "java.util.OptionalInt",
	"OptionalLong": "java.util.OptionalLong", "OptionalDouble": "java.util.OptionalDouble",
	"Stream": "java.util.stream.Stream", "IntStream": "java.util.stream.IntStream",
	"LongStream": "java.util.stream.LongStream", "DoubleStream": "java.util.stream.DoubleStream",
	"Pattern": "java.util.regex.Pattern", "Matcher": "java.util.regex.Matcher",
	"Instant": "java.time.Instant", "ZonedDateTime": "java.time.ZonedDateTime",
	"LocalDate": "java.time.LocalDate", "LocalDateTime": "java.time.LocalDateTime",
	"LocalTime": "java.time.LocalTime", "ZoneId": "java.time.ZoneId", "ZoneOffset": "java.time.ZoneOffset",
	"Duration": "java.time.Duration", "Period": "java.time.Period", "DayOfWeek": "java.time.DayOfWeek",
	"Month": "java.time.Month", "ChronoUnit": "java.time.temporal.ChronoUnit",
	"DateTimeFormatter": "java.time.format.DateTimeFormatter", "Base64": "java.util.Base64",
	"Exception": "java.lang.Exception", "RuntimeException": "java.lang.RuntimeException",
	"IllegalArgumentException":      "java.lang.IllegalArgumentException",
	"IllegalStateException":         "java.lang.IllegalStateException",
	"ArithmeticException":           "java.lang.ArithmeticException",
	"NullPointerException":          "java.lang.NullPointerException",
	"UnsupportedOperationException": "java.lang.UnsupportedOperationException",
	"Debug":                         "org.opensearch.painless.api.Debug",
}

// plCanon is the canonical (error message) name of a static type.
func plCanon(t string) string {
	if strings.HasSuffix(t, "[]") {
		return plCanon(strings.TrimSuffix(t, "[]")) + "[]"
	}
	if n, ok := plCanonNames[t]; ok {
		return n
	}
	return t
}

var plBoxOf = map[string]string{
	"int": "Integer", "long": "Long", "double": "Double", "float": "Float",
	"short": "Short", "byte": "Byte", "char": "Character", "boolean": "Boolean",
}

// plRuntimeClass returns the painless and java class names of a value.
func plRuntimeClass(v any) (string, string) {
	switch t := v.(type) {
	case int32:
		return "java.lang.Integer", "java.lang.Integer"
	case int64:
		return "java.lang.Long", "java.lang.Long"
	case float32:
		return "java.lang.Float", "java.lang.Float"
	case float64:
		return "java.lang.Double", "java.lang.Double"
	case int16:
		return "java.lang.Short", "java.lang.Short"
	case int8:
		return "java.lang.Byte", "java.lang.Byte"
	case plChar:
		return "java.lang.Character", "java.lang.Character"
	case bool:
		return "java.lang.Boolean", "java.lang.Boolean"
	case string:
		return "java.lang.String", "java.lang.String"
	case *List:
		return "java.util.ArrayList", "java.util.ArrayList"
	case *Map:
		switch t.kind {
		case Linked:
			return "java.util.LinkedHashMap", "java.util.LinkedHashMap"
		case plTree:
			return "java.util.TreeMap", "java.util.TreeMap"
		}
		return "java.util.HashMap", "java.util.HashMap"
	case *Set:
		return "java.util.HashSet", "java.util.HashSet"
	case *DocValues:
		return "org.opensearch.index.fielddata.ScriptDocValues." + t.Kind,
			"org.opensearch.index.fielddata.ScriptDocValues$" + t.Kind
	case *plStringBuilder:
		return "java.lang.StringBuilder", "java.lang.StringBuilder"
	case *plPattern:
		return "java.util.regex.Pattern", "java.util.regex.Pattern"
	case *plMatcher:
		return "java.util.regex.Matcher", "java.util.regex.Matcher"
	case *Array:
		return plCanon(t.Elem) + "[]", plCanon(t.Elem) + "[]"
	case *plZDT:
		return "java.time.ZonedDateTime", "java.time.ZonedDateTime"
	case *plLocalDate:
		return "java.time.LocalDate", "java.time.LocalDate"
	case *plInstant:
		return "java.time.Instant", "java.time.Instant"
	case *plDuration:
		return "java.time.Duration", "java.time.Duration"
	case *GeoPoint:
		return "org.opensearch.common.geo.GeoPoint", "org.opensearch.common.geo.GeoPoint"
	case *plMapEntryRef:
		return "java.util.Map$Entry", "java.util.HashMap$Node"
	case *plStream:
		return "java.util.stream.Stream", "java.util.stream.ReferencePipeline"
	case *plOptional:
		return "java.util." + t.name, "java.util." + t.name
	case *plFn:
		return "java.util.function.Function", "java.util.function.Function"
	}
	return "java.lang.Object", "java.lang.Object"
}

// plDefTypeName names a def value in cast errors: boxed primitives by their
// primitive name ("def [long]").
func plDefTypeName(v any) string {
	if p := plPrimName(v); p != "" {
		return p
	}
	n, _ := plRuntimeClass(v)
	return n
}

// ---- JSON conversion ----

// FromJSON converts decoded JSON (json.Number, map, slice) to Painless
// values. Objects become maps of the given kind.
func FromJSON(v any, kind int) any {
	switch t := v.(type) {
	case json.Number:
		s := t.String()
		if !strings.ContainsAny(s, ".eE") {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				if n >= math.MinInt32 && n <= math.MaxInt32 {
					return int32(n)
				}
				return n
			}
		}
		f, _ := t.Float64()
		return f
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1e15 {
			if t >= math.MinInt32 && t <= math.MaxInt32 {
				return int32(t)
			}
			return int64(t)
		}
		return t
	case int:
		if t >= math.MinInt32 && t <= math.MaxInt32 {
			return int32(t)
		}
		return int64(t)
	case int64:
		if t >= math.MinInt32 && t <= math.MaxInt32 {
			return int32(t)
		}
		return t
	case []any:
		l := &List{a: make([]any, len(t))}
		for i, e := range t {
			l.a[i] = FromJSON(e, kind)
		}
		return l
	case map[string]any:
		m := NewMap(kind)
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			m.Put(k, FromJSON(t[k], kind))
		}
		return m
	}
	return v
}

// ToJSON converts a Painless value back to plain Go values for responses
// and documents.
func ToJSON(v any) any {
	switch t := v.(type) {
	case nil, string, bool, int64, float64:
		return t
	case int32:
		return int64(t)
	case int16:
		return int64(t)
	case int8:
		return int64(t)
	case plChar:
		return ToString(t)
	case float32:
		return JSONFloat32(t)
	case *List:
		out := make([]any, len(t.a))
		for i, e := range t.a {
			out[i] = ToJSON(e)
		}
		return out
	case *DocValues:
		out := make([]any, len(t.Values))
		for i, e := range t.Values {
			out[i] = ToJSON(e)
		}
		return out
	case *Set:
		items := t.items()
		out := make([]any, len(items))
		for i, e := range items {
			out[i] = ToJSON(e)
		}
		return out
	case *Array:
		out := make([]any, len(t.Items))
		for i, e := range t.Items {
			out[i] = ToJSON(e)
		}
		return out
	case *Map:
		out := map[string]any{}
		for _, e := range t.entries {
			out[ToString(e.Key)] = ToJSON(e.Value)
		}
		return out
	}
	return ToString(v)
}

// JSONFloat32 renders a float the way Jackson prints a Java float.
type JSONFloat32 float32

func (f JSONFloat32) MarshalJSON() ([]byte, error) {
	d := float64(f)
	if math.IsNaN(d) || math.IsInf(d, 0) {
		return []byte(`"` + plFmtFloat(float32(f)) + `"`), nil
	}
	return []byte(plFmtFloat(float32(f))), nil
}
