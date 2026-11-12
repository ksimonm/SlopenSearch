package main

import (
	"strconv"
	"strings"
)

// Painless engine: static analysis.
//
// Painless is statically typed with an escape hatch (def). The checker infers
// the static type of every expression so that compile errors match the real
// compiler (illegal casts, unknown whitelist methods, constant folding of
// division by zero, ...) and records the types the interpreter needs for
// conversions (declared variables, ternary promotion, method parameters).

type plCompileErr struct {
	typ       string
	reason    string
	offset    int
	nilReason bool
	noPos     bool
	cause     *plCompileErr
}

type plSig struct {
	params []string
	ret    string
}

// plParseSigs reads "name/ret(p1,p2)" signatures; methods are looked up by
// name and arity, like the Painless whitelist.
func plParseSigs(specs ...string) map[string]plSig {
	out := map[string]plSig{}
	for _, s := range specs {
		name, rest, _ := strings.Cut(s, ":")
		ret, args, _ := strings.Cut(rest, "(")
		args = strings.TrimSuffix(args, ")")
		var params []string
		if args != "" {
			params = strings.Split(args, ",")
		}
		out[name+"/"+strconv.Itoa(len(params))] = plSig{params: params, ret: ret}
	}
	return out
}

var plStaticMethods = map[string]map[string]plSig{
	"Math": plParseSigs(
		"abs:double(double)", "acos:double(double)", "asin:double(double)", "atan:double(double)",
		"cbrt:double(double)", "ceil:double(double)", "cos:double(double)", "cosh:double(double)",
		"exp:double(double)", "expm1:double(double)", "floor:double(double)", "log:double(double)",
		"log10:double(double)", "log1p:double(double)", "rint:double(double)", "signum:double(double)",
		"sin:double(double)", "sinh:double(double)", "sqrt:double(double)", "tan:double(double)",
		"tanh:double(double)", "toDegrees:double(double)", "toRadians:double(double)", "ulp:double(double)",
		"atan2:double(double,double)", "hypot:double(double,double)", "IEEEremainder:double(double,double)",
		"max:double(double,double)", "min:double(double,double)", "pow:double(double,double)",
		"round:long(double)", "random:double()"),
	"Integer": plParseSigs(
		"bitCount:int(int)", "compare:int(int,int)", "max:int(int,int)", "min:int(int,int)", "sum:int(int,int)",
		"parseInt:int(String)", "parseInt:int(String,int)", "signum:int(int)", "reverse:int(int)",
		"highestOneBit:int(int)", "lowestOneBit:int(int)", "numberOfLeadingZeros:int(int)",
		"numberOfTrailingZeros:int(int)", "toBinaryString:String(int)", "toHexString:String(int)",
		"toOctalString:String(int)", "toString:String(int)", "toString:String(int,int)",
		"valueOf:Integer(int)", "hashCode:int(int)", "rotateLeft:int(int,int)", "rotateRight:int(int,int)",
		"toUnsignedLong:long(int)", "compareUnsigned:int(int,int)", "parseUnsignedInt:int(String)"),
	"Long": plParseSigs(
		"bitCount:int(long)", "compare:int(long,long)", "max:long(long,long)", "min:long(long,long)",
		"sum:long(long,long)", "parseLong:long(String)", "parseLong:long(String,int)", "signum:int(long)",
		"numberOfLeadingZeros:int(long)", "numberOfTrailingZeros:int(long)",
		"toBinaryString:String(long)", "toHexString:String(long)", "toOctalString:String(long)",
		"toString:String(long)", "toString:String(long,int)", "valueOf:Long(long)", "hashCode:int(long)"),
	"Double": plParseSigs(
		"compare:int(double,double)", "isFinite:boolean(double)", "isInfinite:boolean(double)",
		"isNaN:boolean(double)", "max:double(double,double)", "min:double(double,double)",
		"sum:double(double,double)", "parseDouble:double(String)", "toString:String(double)",
		"valueOf:Double(double)", "hashCode:int(double)", "doubleToLongBits:long(double)",
		"longBitsToDouble:double(long)", "toHexString:String(double)"),
	"Float": plParseSigs(
		"compare:int(float,float)", "isFinite:boolean(float)", "isInfinite:boolean(float)", "isNaN:boolean(float)",
		"max:float(float,float)", "min:float(float,float)", "sum:float(float,float)",
		"parseFloat:float(String)", "toString:String(float)", "valueOf:Float(float)", "hashCode:int(float)",
		"floatToIntBits:int(float)", "intBitsToFloat:float(int)"),
	"Short": plParseSigs("compare:int(short,short)", "parseShort:short(String)", "toString:String(short)",
		"valueOf:Short(short)", "hashCode:int(short)"),
	"Byte": plParseSigs("compare:int(byte,byte)", "parseByte:byte(String)", "toString:String(byte)",
		"valueOf:Byte(byte)", "hashCode:int(byte)"),
	"Boolean": plParseSigs("compare:int(boolean,boolean)", "logicalAnd:boolean(boolean,boolean)",
		"logicalOr:boolean(boolean,boolean)", "logicalXor:boolean(boolean,boolean)",
		"parseBoolean:boolean(String)", "toString:String(boolean)", "valueOf:Boolean(boolean)",
		"hashCode:int(boolean)"),
	"Character": plParseSigs("isDigit:boolean(int)", "isLetter:boolean(int)", "isLetterOrDigit:boolean(int)",
		"isLowerCase:boolean(int)", "isUpperCase:boolean(int)", "isWhitespace:boolean(int)",
		"isAlphabetic:boolean(int)", "isSpaceChar:boolean(int)", "toLowerCase:char(char)",
		"toUpperCase:char(char)", "getNumericValue:int(int)", "valueOf:Character(char)",
		"compare:int(char,char)", "toString:String(char)", "digit:int(int,int)", "hashCode:int(char)",
		"toChars:char[](int)"),
	"String": plParseSigs("valueOf:String(def)", "join:String(CharSequence,Iterable)",
		"format:String(String,def[])", "copyValueOf:String(char[])"),
	"Objects": plParseSigs("equals:boolean(def,def)", "hashCode:int(def)", "isNull:boolean(def)",
		"nonNull:boolean(def)", "requireNonNull:def(def)", "requireNonNull:def(def,String)",
		"toString:String(def)", "toString:String(def,String)", "compare:int(def,def,Comparator)",
		"deepEquals:boolean(def,def)", "hash:int(def[])", "requireNonNullElse:def(def,def)"),
	"Arrays": plParseSigs("asList:List(def[])", "toString:String(def)", "equals:boolean(def,def)",
		"sort:void(def)", "stream:Stream(def)", "fill:void(def,def)", "hashCode:int(def)",
		"deepToString:String(def)", "copyOf:def(def,int)"),
	"Collections": plParseSigs("sort:void(List)", "sort:void(List,Comparator)", "reverse:void(List)",
		"max:def(Collection)", "min:def(Collection)", "max:def(Collection,Comparator)",
		"min:def(Collection,Comparator)", "shuffle:void(List)", "emptyList:List()", "emptyMap:Map()",
		"emptySet:Set()", "singletonList:List(def)", "singleton:Set(def)", "singletonMap:Map(def,def)",
		"unmodifiableList:List(List)", "unmodifiableMap:Map(Map)", "unmodifiableSet:Set(Set)",
		"frequency:int(Collection,def)", "swap:void(List,int,int)", "nCopies:List(int,def)",
		"reverseOrder:Comparator()"),
	"Collectors": plParseSigs("toList:def()", "toSet:def()", "joining:def()", "joining:def(CharSequence)",
		"joining:def(CharSequence,CharSequence,CharSequence)", "counting:def()", "toMap:def(def,def)",
		"groupingBy:def(def)", "groupingBy:def(def,def)", "summingInt:def(def)", "summingLong:def(def)",
		"summingDouble:def(def)", "averagingInt:def(def)", "averagingDouble:def(def)", "mapping:def(def,def)"),
	"Comparator": plParseSigs("naturalOrder:Comparator()", "reverseOrder:Comparator()",
		"comparing:Comparator(def)", "comparing:Comparator(def,Comparator)", "comparingInt:Comparator(def)",
		"comparingLong:Comparator(def)", "comparingDouble:Comparator(def)",
		"nullsFirst:Comparator(Comparator)", "nullsLast:Comparator(Comparator)"),
	"Debug":   plParseSigs("explain:void(def)"),
	"Instant": plParseSigs("ofEpochMilli:Instant(long)", "ofEpochSecond:Instant(long)", "parse:Instant(CharSequence)"),
	"ZonedDateTime": plParseSigs("parse:ZonedDateTime(CharSequence)", "ofInstant:ZonedDateTime(Instant,ZoneId)",
		"of:ZonedDateTime(int,int,int,int,int,int,int,ZoneId)"),
	"LocalDate":     plParseSigs("of:LocalDate(int,int,int)", "parse:LocalDate(CharSequence)", "ofEpochDay:LocalDate(long)"),
	"LocalDateTime": plParseSigs("of:LocalDateTime(int,int,int,int,int)", "of:LocalDateTime(int,int,int,int,int,int)", "parse:LocalDateTime(CharSequence)"),
	"ZoneId":        plParseSigs("of:ZoneId(String)"),
	"Duration": plParseSigs("ofDays:Duration(long)", "ofHours:Duration(long)", "ofMinutes:Duration(long)",
		"ofSeconds:Duration(long)", "ofMillis:Duration(long)", "ofNanos:Duration(long)",
		"between:Duration(def,def)"),
	"Period":            plParseSigs("ofDays:Period(int)", "ofMonths:Period(int)", "ofYears:Period(int)", "between:Period(LocalDate,LocalDate)"),
	"DayOfWeek":         plParseSigs("of:DayOfWeek(int)", "valueOf:DayOfWeek(String)"),
	"DateTimeFormatter": plParseSigs("ofPattern:DateTimeFormatter(String)"),
	"Pattern":           plParseSigs("compile:Pattern(String)", "compile:Pattern(String,int)", "quote:String(String)"),
	"Matcher":           plParseSigs("quoteReplacement:String(String)"),
	"Base64":            plParseSigs("getEncoder:def()", "getDecoder:def()"),
	"IntStream":         plParseSigs("range:IntStream(int,int)", "rangeClosed:IntStream(int,int)"),
	"LongStream":        plParseSigs("range:LongStream(long,long)", "rangeClosed:LongStream(long,long)"),
	"Stream":            plParseSigs("of:Stream(def[])", "empty:Stream()"),
	"List":              plParseSigs("of:List(def[])", "copyOf:List(Collection)"),
	"Map":               plParseSigs("of:Map()", "of:Map(def,def)", "of:Map(def,def,def,def)", "entry:def(def,def)"),
	"Set":               plParseSigs("of:Set(def[])"),
	"StrictMath":        plParseSigs("abs:double(double)", "sqrt:double(double)", "max:double(double,double)", "min:double(double,double)"),
}

type plConst struct {
	val any
	typ string
}

var plStaticFields = map[string]map[string]plConst{
	"Math":      {"PI": {3.141592653589793, "double"}, "E": {2.718281828459045, "double"}},
	"Integer":   {"MAX_VALUE": {int32(2147483647), "int"}, "MIN_VALUE": {int32(-2147483648), "int"}, "SIZE": {int32(32), "int"}, "BYTES": {int32(4), "int"}},
	"Long":      {"MAX_VALUE": {int64(9223372036854775807), "long"}, "MIN_VALUE": {int64(-9223372036854775808), "long"}, "SIZE": {int32(64), "int"}, "BYTES": {int32(8), "int"}},
	"Short":     {"MAX_VALUE": {int16(32767), "short"}, "MIN_VALUE": {int16(-32768), "short"}},
	"Byte":      {"MAX_VALUE": {int8(127), "byte"}, "MIN_VALUE": {int8(-128), "byte"}},
	"Character": {"MAX_VALUE": {plChar(0xffff), "char"}, "MIN_VALUE": {plChar(0), "char"}},
	"Boolean":   {"TRUE": {true, "Boolean"}, "FALSE": {false, "Boolean"}},
	"Double": {
		"NaN": {plNaN(), "double"}, "POSITIVE_INFINITY": {plInf(1), "double"}, "NEGATIVE_INFINITY": {plInf(-1), "double"},
		"MAX_VALUE": {1.7976931348623157e308, "double"}, "MIN_VALUE": {4.9e-324, "double"}, "MIN_NORMAL": {2.2250738585072014e-308, "double"},
	},
	"Float": {
		"NaN": {float32(plNaN()), "float"}, "POSITIVE_INFINITY": {float32(plInf(1)), "float"},
		"NEGATIVE_INFINITY": {float32(plInf(-1)), "float"}, "MAX_VALUE": {float32(3.4028235e38), "float"},
		"MIN_VALUE": {float32(1.4e-45), "float"},
	},
}

// plStringMethods is the java.lang.String whitelist including Painless
// augmentations.
var plStringMethods = plParseSigs(
	"charAt:char(int)", "chars:IntStream()", "codePointAt:int(int)", "codePointBefore:int(int)",
	"codePointCount:int(int,int)", "compareTo:int(String)", "compareToIgnoreCase:int(String)",
	"concat:String(String)", "contains:boolean(CharSequence)", "contentEquals:boolean(CharSequence)",
	"endsWith:boolean(String)", "equals:boolean(def)", "equalsIgnoreCase:boolean(String)",
	"hashCode:int()", "indexOf:int(def)", "indexOf:int(def,int)", "isEmpty:boolean()",
	"lastIndexOf:int(def)", "lastIndexOf:int(def,int)", "length:int()", "regionMatches:boolean(int,String,int,int)",
	"regionMatches:boolean(boolean,int,String,int,int)", "replace:String(CharSequence,CharSequence)",
	"startsWith:boolean(String)", "startsWith:boolean(String,int)", "subSequence:CharSequence(int,int)",
	"substring:String(int)", "substring:String(int,int)", "toCharArray:char[]()", "toLowerCase:String()",
	"toUpperCase:String()", "trim:String()", "toString:String()", "getClass:def()",
	"decodeBase64:String()", "encodeBase64:String()", "replaceAll:String(Pattern,Function)",
	"replaceFirst:String(Pattern,Function)", "splitOnToken:String[](String)", "splitOnToken:String[](String,int)",
	"codePoints:IntStream()", "strip:String()", "isBlank:boolean()",
)

func plNaN() float64 {
	var z float64
	return z / z
}

func plInf(sign int) float64 {
	var z float64
	if sign < 0 {
		return -1 / z
	}
	return 1 / z
}

// ---- cast legality ----

var plWidening = map[string][]string{
	"byte":  {"short", "int", "long", "float", "double"},
	"short": {"int", "long", "float", "double"},
	"char":  {"int", "long", "float", "double"},
	"int":   {"long", "float", "double"},
	"long":  {"float", "double"},
	"float": {"double"},
}

var plSupertypes = map[string][]string{
	"ArrayList":     {"List", "Collection", "Iterable"},
	"LinkedList":    {"List", "Collection", "Iterable", "Deque", "Queue"},
	"List":          {"Collection", "Iterable"},
	"HashMap":       {"Map"},
	"LinkedHashMap": {"Map", "HashMap"},
	"TreeMap":       {"Map"},
	"HashSet":       {"Set", "Collection", "Iterable"},
	"LinkedHashSet": {"Set", "Collection", "Iterable", "HashSet"},
	"TreeSet":       {"Set", "Collection", "Iterable"},
	"Set":           {"Collection", "Iterable"},
	"Collection":    {"Iterable"},
	"String":        {"CharSequence", "Comparable"},
	"StringBuilder": {"CharSequence"},
	"Integer":       {"Number"}, "Long": {"Number"}, "Double": {"Number"}, "Float": {"Number"},
	"Short": {"Number"}, "Byte": {"Number"},
	"IllegalArgumentException": {"RuntimeException", "Exception"},
	"IllegalStateException":    {"RuntimeException", "Exception"},
	"ArithmeticException":      {"RuntimeException", "Exception"},
	"NullPointerException":     {"RuntimeException", "Exception"},
	"RuntimeException":         {"Exception"},
}

func plIsSubtype(from, to string) bool {
	for _, s := range plSupertypes[from] {
		if s == to {
			return true
		}
	}
	return false
}

func plCanImplicit(from, to string) bool {
	if from == to || to == "def" || from == "def" {
		return true
	}
	if from == "null" {
		return !plIsPrimitive(to)
	}
	if plIsPrimitive(from) {
		for _, w := range plWidening[from] {
			if w == to {
				return true
			}
		}
		return false
	}
	if plIsPrimitive(to) {
		return false
	}
	if to == "Object" {
		return true
	}
	return plIsSubtype(from, to)
}

func plCanExplicit(from, to string, constVal any) bool {
	if plCanImplicit(from, to) {
		return true
	}
	if plIsNumericType(from) && plIsNumericType(to) {
		return true
	}
	if from == "String" && to == "char" {
		s, ok := constVal.(string)
		return ok && len([]rune(s)) == 1
	}
	fp, tp := plIsPrimitive(from), plIsPrimitive(to)
	switch {
	case fp && !tp:
		return to == "Object" || to == "Number" || to == plBoxOf[from]
	case !fp && tp:
		return from == "Object" || from == "Number" || plBoxOf[to] == from
	case !fp && !tp:
		if from == "String" || to == "String" {
			return from == "Object" || to == "Object" || from == "CharSequence" || to == "CharSequence"
		}
		return true
	}
	return false
}

func plCastErr(from, to string, off int) *plCompileErr {
	return &plCompileErr{typ: "class_cast_exception", offset: off,
		reason: "Cannot cast from [" + plCanon(from) + "] to [" + plCanon(to) + "]."}
}

// ---- checker ----

type plCheckCtx struct {
	vars  map[string]string
	funcs map[string]plSig // context functions (score functions, ...)
	ret   string           // required return type of the script
}

type plInfo struct {
	types  map[plNode]string
	sigs   map[plNode]plSig // resolved whitelist method of a call
	consts map[plNode]any
}

type plChecker struct {
	prog     *plProgram
	ctx      *plCheckCtx
	info     *plInfo
	scopes   []map[string]string
	inFunc   *plFunc
	inLambda int
}

func plCheck(prog *plProgram, ctx *plCheckCtx) (*plInfo, *plCompileErr) {
	c := &plChecker{prog: prog, ctx: ctx, info: &plInfo{types: map[plNode]string{}, sigs: map[plNode]plSig{}, consts: map[plNode]any{}}}
	root := map[string]string{}
	for k, v := range ctx.vars {
		root[k] = v
	}
	c.scopes = []map[string]string{root}
	for _, f := range prog.funcs {
		c.inFunc = f
		c.push()
		for i, n := range f.pnames {
			c.declare(n, f.ptypes[i])
		}
		if err := c.stmts(f.body.stmts); err != nil {
			return nil, err
		}
		c.pop()
	}
	c.inFunc = nil
	stmts := prog.body.stmts
	for i, s := range stmts {
		if err := c.stmt(s); err != nil {
			return nil, err
		}
		if i == len(stmts)-1 {
			if es, ok := s.(*plSExpr); ok {
				if _, isAssign := es.x.(*plEAssign); !isAssign {
					if err := c.checkReturn(c.info.types[es.x], es.x); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return c.info, nil
}

func (c *plChecker) checkReturn(t string, x plNode) *plCompileErr {
	want := c.ctx.ret
	if want == "" || want == "def" || want == "Object" || want == "void" || t == "" || t == "def" || t == "void" {
		return nil
	}
	if !plCanImplicit(t, want) {
		if t == "null" && plIsPrimitive(want) {
			return plCastErr(t, want, x.nodePos())
		}
		if plIsPrimitive(t) && !plIsPrimitive(want) && (want == "Number" || want == plBoxOf[t]) {
			return nil
		}
		return plCastErr(t, want, x.nodePos())
	}
	return nil
}

// constFits allows an int constant to narrow implicitly into byte, short or
// char when the value is in range.
func (c *plChecker) constFits(x plNode, to string) bool {
	cv, ok := c.info.consts[x]
	if !ok {
		return false
	}
	v, isInt := cv.(int32)
	if !isInt {
		return false
	}
	switch to {
	case "byte":
		return v >= -128 && v <= 127
	case "short":
		return v >= -32768 && v <= 32767
	case "char":
		return v >= 0 && v <= 65535
	}
	return false
}

func (c *plChecker) push() { c.scopes = append(c.scopes, map[string]string{}) }
func (c *plChecker) pop()  { c.scopes = c.scopes[:len(c.scopes)-1] }
func (c *plChecker) declare(name, typ string) {
	c.scopes[len(c.scopes)-1][name] = typ
}
func (c *plChecker) lookup(name string) (string, bool) {
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if t, ok := c.scopes[i][name]; ok {
			return t, true
		}
	}
	return "", false
}

func (c *plChecker) stmts(list []plNode) *plCompileErr {
	for _, s := range list {
		if err := c.stmt(s); err != nil {
			return err
		}
	}
	return nil
}

func (c *plChecker) checkType(t string, off int) *plCompileErr {
	base := strings.TrimRight(t, "[]")
	if !plTypeNames[base] {
		return &plCompileErr{typ: "illegal_argument_exception", offset: off, reason: "Not a type [" + t + "]."}
	}
	return nil
}

func (c *plChecker) stmt(s plNode) *plCompileErr {
	switch n := s.(type) {
	case *plSBlock:
		c.push()
		defer c.pop()
		return c.stmts(n.stmts)
	case *plSDecl:
		for i, name := range n.names {
			if init := n.inits[i]; init != nil {
				t, err := c.expr(init)
				if err != nil {
					return err
				}
				if !plCanImplicit(t, n.typ) && !c.constFits(init, n.typ) {
					return plCastErr(t, n.typ, init.nodePos())
				}
			}
			c.declare(name, n.typ)
		}
		return nil
	case *plSExpr:
		_, err := c.expr(n.x)
		return err
	case *plSIf:
		if err := c.cond(n.cond); err != nil {
			return err
		}
		c.push()
		if err := c.stmt(n.then); err != nil {
			return err
		}
		c.pop()
		if n.els != nil {
			c.push()
			defer c.pop()
			return c.stmt(n.els)
		}
		return nil
	case *plSWhile:
		if err := c.cond(n.cond); err != nil {
			return err
		}
		c.push()
		defer c.pop()
		return c.stmt(n.body)
	case *plSDo:
		c.push()
		if err := c.stmt(n.body); err != nil {
			return err
		}
		c.pop()
		return c.cond(n.cond)
	case *plSFor:
		c.push()
		defer c.pop()
		if n.init != nil {
			if err := c.stmt(n.init); err != nil {
				return err
			}
		}
		if n.cond != nil {
			if err := c.cond(n.cond); err != nil {
				return err
			}
		}
		for _, u := range n.update {
			if _, err := c.expr(u); err != nil {
				return err
			}
		}
		return c.stmt(n.body)
	case *plSForEach:
		if _, err := c.expr(n.iter); err != nil {
			return err
		}
		c.push()
		defer c.pop()
		c.declare(n.name, n.typ)
		return c.stmt(n.body)
	case *plSReturn:
		if n.x == nil {
			return nil
		}
		t, err := c.expr(n.x)
		if err != nil {
			return err
		}
		if c.inFunc != nil {
			if !plCanImplicit(t, c.inFunc.ret) {
				return plCastErr(t, c.inFunc.ret, n.x.nodePos())
			}
			return nil
		}
		if c.inLambda > 0 {
			return nil
		}
		return c.checkReturn(t, n.x)
	case *plSThrow:
		_, err := c.expr(n.x)
		return err
	case *plSTry:
		if err := c.stmt(n.body); err != nil {
			return err
		}
		for _, ct := range n.catches {
			c.push()
			c.declare(ct.name, ct.typ)
			if err := c.stmt(ct.body); err != nil {
				return err
			}
			c.pop()
		}
		return nil
	}
	return nil
}

func (c *plChecker) cond(x plNode) *plCompileErr {
	t, err := c.expr(x)
	if err != nil {
		return err
	}
	if t != "boolean" && t != "def" && t != "Boolean" {
		return plCastErr(t, "boolean", x.nodePos())
	}
	return nil
}

func (c *plChecker) set(n plNode, t string) (string, *plCompileErr) {
	c.info.types[n] = t
	return t, nil
}

func plBinaryPromote(a, b string) string {
	if a == "def" || b == "def" {
		return "def"
	}
	rank := map[string]int{"byte": 1, "short": 1, "char": 1, "int": 1, "long": 2, "float": 3, "double": 4}
	ra, oka := rank[plUnbox(a)]
	rb, okb := rank[plUnbox(b)]
	if !oka || !okb {
		return ""
	}
	r := ra
	if rb > r {
		r = rb
	}
	return [...]string{"", "int", "long", "float", "double"}[r]
}

func plUnbox(t string) string {
	for p, b := range plBoxOf {
		if b == t {
			return p
		}
	}
	return t
}

var plOpNames = map[string]string{
	"<": "less than", "<=": "less than or equals", ">": "greater than", ">=": "greater than or equals",
	"|": "boolean or", "&": "boolean and", "+": "add", "-": "subtraction", "*": "multiplication",
	"/": "division", "%": "remainder", "<<": "left shift", ">>": "right shift", ">>>": "unsigned shift",
	"^": "xor",
}

func (c *plChecker) expr(x plNode) (string, *plCompileErr) {
	switch n := x.(type) {
	case *plELit:
		c.info.consts[n] = n.val
		return c.set(n, n.typ)
	case *plERegex:
		return c.set(n, "Pattern")
	case *plEVar:
		if t, ok := c.lookup(n.name); ok {
			return c.set(n, t)
		}
		return "", &plCompileErr{typ: "illegal_argument_exception", offset: n.pos, reason: "cannot resolve symbol [" + n.name + "]"}
	case *plEType:
		return c.set(n, "static:"+n.name)
	case *plEField:
		if et, ok := n.obj.(*plEType); ok {
			if f, ok := plStaticFields[et.name][n.name]; ok {
				c.info.consts[n] = f.val
				c.info.types[n.obj] = "static:" + et.name
				return c.set(n, f.typ)
			}
			if plIsEnumClass(et.name) {
				c.info.types[n.obj] = "static:" + et.name
				return c.set(n, et.name)
			}
			return "", &plCompileErr{typ: "illegal_argument_exception", offset: n.pos,
				reason: "static field [" + plCanon(et.name) + ", " + n.name + "] not found"}
		}
		ot, err := c.expr(n.obj)
		if err != nil {
			return "", err
		}
		if strings.HasSuffix(ot, "[]") && n.name == "length" {
			return c.set(n, "int")
		}
		if ot == "String" {
			return "", &plCompileErr{typ: "illegal_argument_exception", offset: n.pos,
				reason: "field [java.lang.String, " + n.name + "] not found"}
		}
		return c.set(n, "def")
	case *plECall:
		return c.call(n)
	case *plEIndex:
		ot, err := c.expr(n.obj)
		if err != nil {
			return "", err
		}
		if _, err := c.expr(n.idx); err != nil {
			return "", err
		}
		if ot == "String" || (plIsPrimitive(ot) && ot != "def") {
			return "", &plCompileErr{typ: "illegal_argument_exception", offset: n.pos,
				reason: "Illegal array access on type [" + plCanon(ot) + "]."}
		}
		if strings.HasSuffix(ot, "[]") {
			return c.set(n, strings.TrimSuffix(ot, "[]"))
		}
		return c.set(n, "def")
	case *plENew:
		if err := c.checkType(n.typ, n.pos); err != nil {
			return "", err
		}
		for _, a := range n.args {
			if _, err := c.expr(a); err != nil {
				return "", err
			}
		}
		return c.set(n, n.typ)
	case *plENewArray:
		for _, d := range n.dims {
			t, err := c.expr(d)
			if err != nil {
				return "", err
			}
			if !plCanImplicit(t, "int") {
				return "", plCastErr(t, "int", d.nodePos())
			}
		}
		for _, e := range n.init {
			t, err := c.expr(e)
			if err != nil {
				return "", err
			}
			if !plCanImplicit(t, n.elem) {
				return "", plCastErr(t, n.elem, e.nodePos())
			}
		}
		return c.set(n, n.elem+strings.Repeat("[]", n.rank))
	case *plEList:
		for _, e := range n.elems {
			if _, err := c.expr(e); err != nil {
				return "", err
			}
		}
		return c.set(n, "ArrayList")
	case *plEMap:
		for i := range n.keys {
			if _, err := c.expr(n.keys[i]); err != nil {
				return "", err
			}
			if _, err := c.expr(n.vals[i]); err != nil {
				return "", err
			}
		}
		return c.set(n, "HashMap")
	case *plEUnary:
		t, err := c.expr(n.x)
		if err != nil {
			return "", err
		}
		switch n.op {
		case "!":
			if t != "boolean" && t != "def" && t != "Boolean" {
				return "", plCastErr(t, "boolean", n.x.nodePos())
			}
			c.foldUnary(n)
			return c.set(n, "boolean")
		case "~":
			p := plBinaryPromote(t, "int")
			if p == "" || p == "float" || p == "double" {
				return "", &plCompileErr{typ: "class_cast_exception", offset: n.pos,
					reason: "cannot apply the bitwise not operator [~] to the type [" + plCanon(t) + "]"}
			}
			c.foldUnary(n)
			return c.set(n, p)
		default:
			p := plBinaryPromote(t, "int")
			if p == "" {
				return "", &plCompileErr{typ: "class_cast_exception", offset: n.pos,
					reason: "cannot apply the " + map[string]string{"-": "negation", "+": "positive"}[n.op] + " operator [" + n.op + "] to the type [" + plCanon(t) + "]"}
			}
			c.foldUnary(n)
			return c.set(n, p)
		}
	case *plEBinary:
		return c.binary(n)
	case *plEInstanceof:
		if _, err := c.expr(n.x); err != nil {
			return "", err
		}
		if err := c.checkType(n.typ, n.pos); err != nil {
			return "", err
		}
		return c.set(n, "boolean")
	case *plECast:
		t, err := c.expr(n.x)
		if err != nil {
			return "", err
		}
		if err := c.checkType(n.typ, n.pos); err != nil {
			return "", err
		}
		if !plCanExplicit(t, n.typ, c.info.consts[n.x]) {
			return "", plCastErr(t, n.typ, n.x.nodePos())
		}
		if cv, ok := c.info.consts[n.x]; ok && plIsPrimitive(n.typ) {
			if s, ok := cv.(string); ok && n.typ == "char" {
				c.info.consts[n] = plChar([]rune(s)[0])
			} else if plIsNumber(cv) && plIsNumericType(n.typ) {
				c.info.consts[n] = plCastNum(cv, n.typ)
			}
		}
		return c.set(n, n.typ)
	case *plECond:
		if err := c.cond(n.c); err != nil {
			return "", err
		}
		ta, err := c.expr(n.a)
		if err != nil {
			return "", err
		}
		tb, err := c.expr(n.b)
		if err != nil {
			return "", err
		}
		rt, err := c.promoteCond(ta, tb, n)
		if err != nil {
			return "", err
		}
		return c.set(n, rt)
	case *plEElvis:
		ta, err := c.expr(n.a)
		if err != nil {
			return "", err
		}
		if plIsPrimitive(ta) {
			return "", &plCompileErr{typ: "illegal_argument_exception", offset: n.pos, noPos: true,
				reason: "Extraneous elvis operator. LHS is a primitive."}
		}
		tb, err := c.expr(n.b)
		if err != nil {
			return "", err
		}
		if ta == "def" || tb == "def" {
			return c.set(n, "def")
		}
		if ta == tb {
			return c.set(n, ta)
		}
		return c.set(n, "def")
	case *plEAssign:
		tt, err := c.target(n.target)
		if err != nil {
			return "", err
		}
		vt, err := c.expr(n.val)
		if err != nil {
			return "", err
		}
		if n.op == "=" {
			if !plCanImplicit(vt, tt) && !c.constFits(n.val, tt) {
				return "", plCastErr(vt, tt, n.val.nodePos())
			}
			return c.set(n, tt)
		}
		if tt == "String" && n.op == "+" {
			return c.set(n, tt)
		}
		if tt != "def" && vt != "def" && plBinaryPromote(tt, vt) == "" && !(tt == "boolean" && (n.op == "&" || n.op == "|" || n.op == "^")) {
			return "", plCastErr(vt, tt, n.val.nodePos())
		}
		return c.set(n, tt)
	case *plEIncDec:
		tt, err := c.target(n.target)
		if err != nil {
			return "", err
		}
		if tt != "def" && !plIsNumericType(plUnbox(tt)) {
			return "", plCastErr(tt, "int", n.pos)
		}
		return c.set(n, tt)
	case *plELambda:
		c.push()
		c.inLambda++
		for i, name := range n.pnames {
			c.declare(name, n.ptypes[i])
		}
		var err *plCompileErr
		if b, ok := n.body.(*plSBlock); ok {
			err = c.stmts(b.stmts)
		} else {
			_, err = c.expr(n.body)
		}
		c.inLambda--
		c.pop()
		if err != nil {
			return "", err
		}
		return c.set(n, "def")
	case *plEFuncRef:
		return c.set(n, "def")
	}
	return "def", nil
}

func plIsEnumClass(name string) bool {
	switch name {
	case "ChronoUnit", "DayOfWeek", "Month", "DateTimeFormatter", "ZoneOffset", "Instant":
		return true
	}
	return false
}

// target types an assignment target.
func (c *plChecker) target(x plNode) (string, *plCompileErr) {
	switch n := x.(type) {
	case *plEVar:
		if t, ok := c.lookup(n.name); ok {
			c.info.types[n] = t
			return t, nil
		}
		return "", &plCompileErr{typ: "illegal_argument_exception", offset: n.pos, reason: "cannot resolve symbol [" + n.name + "]"}
	case *plEField, *plEIndex:
		return c.expr(x)
	}
	return "", &plCompileErr{typ: "illegal_argument_exception", offset: x.nodePos(), reason: "invalid assignment: cannot assign a value to a non-variable expression"}
}

func (c *plChecker) promoteCond(ta, tb string, n *plECond) (string, *plCompileErr) {
	if ta == "def" || tb == "def" {
		return "def", nil
	}
	if ta == tb {
		return ta, nil
	}
	if p := plBinaryPromote(ta, tb); p != "" && plIsPrimitive(ta) && plIsPrimitive(tb) {
		return p, nil
	}
	if ta == "null" && !plIsPrimitive(tb) {
		return tb, nil
	}
	if tb == "null" && !plIsPrimitive(ta) {
		return ta, nil
	}
	// mixed kinds promote to Object, which primitives cannot implicitly reach
	if plIsPrimitive(ta) {
		return "", plCastErr(ta, "Object", n.a.nodePos())
	}
	if plIsPrimitive(tb) {
		return "", plCastErr(tb, "Object", n.b.nodePos())
	}
	return "Object", nil
}

func (c *plChecker) foldUnary(n *plEUnary) {
	cv, ok := c.info.consts[n.x]
	if !ok || cv == nil {
		return
	}
	switch n.op {
	case "!":
		if b, ok := cv.(bool); ok {
			c.info.consts[n] = !b
		}
	case "-":
		if plIsNumber(cv) {
			v, err := plArith("-", plCastNum(int32(0), plPromote(cv, int32(0))), cv)
			if err == nil {
				c.info.consts[n] = v
			}
		}
	case "~":
		if plIsNumber(cv) {
			if plPromote(cv, int32(0)) == "long" {
				c.info.consts[n] = ^plI64(cv)
			} else {
				c.info.consts[n] = ^int32(plI64(cv))
			}
		}
	}
}

func (c *plChecker) binary(n *plEBinary) (string, *plCompileErr) {
	tl, err := c.expr(n.l)
	if err != nil {
		return "", err
	}
	tr, err := c.expr(n.r)
	if err != nil {
		return "", err
	}
	cl, lok := c.info.consts[n.l]
	cr, rok := c.info.consts[n.r]
	switch n.op {
	case "&&", "||":
		for _, side := range []struct {
			t string
			x plNode
		}{{tl, n.l}, {tr, n.r}} {
			if side.t != "boolean" && side.t != "def" && side.t != "Boolean" {
				return "", plCastErr(side.t, "boolean", side.x.nodePos())
			}
		}
		return c.set(n, "boolean")
	case "==", "!=", "===", "!==":
		if tl == "null" && tr == "null" {
			return "", &plCompileErr{typ: "illegal_argument_exception", offset: n.pos, reason: "extraneous comparison of [null] constants"}
		}
		return c.set(n, "boolean")
	case "<", "<=", ">", ">=":
		if tl != "def" && tr != "def" && plBinaryPromote(tl, tr) == "" {
			return "", &plCompileErr{typ: "class_cast_exception", offset: n.pos,
				reason: "cannot apply the " + plOpNames[n.op] + " operator [" + n.op + "] to the types [" + plCanon(tl) + "] and [" + plCanon(tr) + "]"}
		}
		return c.set(n, "boolean")
	case "=~", "==~":
		return c.set(n, "boolean")
	case "+":
		if tl == "String" || tr == "String" {
			if lok && rok {
				c.info.consts[n] = plToString(cl) + plToString(cr)
			}
			return c.set(n, "String")
		}
	case "&", "|", "^":
		if (tl == "boolean" || tl == "Boolean") && (tr == "boolean" || tr == "Boolean") {
			if n.op == "^" {
				return c.set(n, "boolean")
			}
			return "", &plCompileErr{typ: "class_cast_exception", offset: n.pos,
				reason: "cannot apply the " + plOpNames[n.op] + " operator [" + n.op + "] to the types [" + plCanon(tl) + "] and [" + plCanon(tr) + "]"}
		}
	}
	if tl == "def" || tr == "def" {
		return c.set(n, "def")
	}
	var rt string
	switch n.op {
	case "<<", ">>", ">>>":
		rt = plBinaryPromote(tl, "int")
		if plBinaryPromote(tr, "int") == "" {
			rt = ""
		}
	default:
		rt = plBinaryPromote(tl, tr)
	}
	if rt == "" {
		// a String mixed into numeric arithmetic fails casting to the other side
		if tl == "String" && tr != "String" {
			return "", plCastErr(tl, plUnbox(tr), n.pos)
		}
		if tr == "String" && tl != "String" {
			return "", plCastErr(tr, plUnbox(tl), n.pos)
		}
		return "", &plCompileErr{typ: "class_cast_exception", offset: n.pos,
			reason: "cannot apply the " + plOpNames[n.op] + " operator [" + n.op + "] to the types [" + plCanon(tl) + "] and [" + plCanon(tr) + "]"}
	}
	if (rt == "float" || rt == "double") && (n.op == "&" || n.op == "|" || n.op == "^" || n.op == "<<" || n.op == ">>" || n.op == ">>>") {
		return "", &plCompileErr{typ: "class_cast_exception", offset: n.pos,
			reason: "cannot apply the " + plOpNames[n.op] + " operator [" + n.op + "] to the types [" + plCanon(tl) + "] and [" + plCanon(tr) + "]"}
	}
	if lok && rok && cl != nil && cr != nil && plIsNumber(cl) && plIsNumber(cr) {
		v, rerr := plArith(n.op, cl, cr)
		if rerr != nil {
			return "", &plCompileErr{typ: rerr.typ, reason: rerr.reason, offset: n.pos}
		}
		c.info.consts[n] = v
	}
	return c.set(n, rt)
}

func (c *plChecker) call(n *plECall) (string, *plCompileErr) {
	argTypes := make([]string, len(n.args))
	for i, a := range n.args {
		t, err := c.expr(a)
		if err != nil {
			return "", err
		}
		argTypes[i] = t
	}
	key := n.name + "/" + strconv.Itoa(len(n.args))
	checkArgs := func(sig plSig) *plCompileErr {
		for i, p := range sig.params {
			if !plCanImplicit(argTypes[i], p) && !(plIsPrimitive(argTypes[i]) && (p == "Object" || p == "Number")) {
				return plCastErr(argTypes[i], p, n.args[i].nodePos())
			}
		}
		return nil
	}
	if n.obj == nil {
		if f, ok := c.prog.funcs[n.name]; ok {
			if len(f.pnames) != len(n.args) {
				return "", &plCompileErr{typ: "illegal_argument_exception", offset: n.pos,
					reason: "Unknown call [" + n.name + "] with [" + strconv.Itoa(len(n.args)) + "] arguments."}
			}
			if err := checkArgs(plSig{params: f.ptypes}); err != nil {
				return "", err
			}
			c.info.sigs[n] = plSig{params: f.ptypes, ret: f.ret}
			return c.set(n, f.ret)
		}
		if sig, ok := c.ctx.funcs[key]; ok {
			if err := checkArgs(sig); err != nil {
				return "", err
			}
			c.info.sigs[n] = sig
			return c.set(n, sig.ret)
		}
		return "", &plCompileErr{typ: "illegal_argument_exception", offset: n.pos,
			reason: "Unknown call [" + n.name + "] with [" + strconv.Itoa(len(n.args)) + "] arguments."}
	}
	if et, ok := n.obj.(*plEType); ok {
		c.info.types[et] = "static:" + et.name
		sig, ok := plStaticMethods[et.name][key]
		if !ok {
			return "", &plCompileErr{typ: "illegal_argument_exception", offset: n.pos,
				reason: "static method [" + plCanon(et.name) + ", " + key + "] not found"}
		}
		if err := checkArgs(sig); err != nil {
			return "", err
		}
		c.info.sigs[n] = sig
		return c.set(n, sig.ret)
	}
	if v, ok := n.obj.(*plEVar); ok {
		if _, found := c.lookup(v.name); !found {
			return "", &plCompileErr{typ: "illegal_argument_exception", offset: n.pos, reason: "cannot resolve symbol [" + v.name + "]"}
		}
	}
	ot, err := c.expr(n.obj)
	if err != nil {
		return "", err
	}
	if ot == "String" {
		sig, ok := plStringMethods[key]
		if !ok {
			return "", &plCompileErr{typ: "illegal_argument_exception", offset: n.pos,
				reason: "member method [java.lang.String, " + key + "] not found"}
		}
		if err := checkArgs(sig); err != nil {
			return "", err
		}
		c.info.sigs[n] = sig
		if n.nullSafe && plIsPrimitive(sig.ret) {
			return "", &plCompileErr{typ: "illegal_argument_exception", offset: n.pos, noPos: true,
				reason: "Result of null safe operator must be nullable"}
		}
		return c.set(n, sig.ret)
	}
	return c.set(n, "def")
}
