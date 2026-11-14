package painless

import "testing"

// Expected values are the "result" strings OpenSearch 3.9 returns from
// _scripts/painless/_execute (painless_test context) for the same sources, taken
// from tests/reference/test_43_scripting.jsonl.

func execute(t *testing.T, src string) (string, *CompileError, *RuntimeError) {
	t.Helper()
	s, cerr := Compile(src, "painless_test")
	if cerr != nil {
		return "", cerr, nil
	}
	v, rerr := s.Run(Bindings{Vars: map[string]any{"params": NewMap(Hash)}})
	if rerr != nil {
		return "", nil, rerr
	}
	return ToString(v), nil, nil
}

func TestExecuteValues(t *testing.T) {
	tests := []struct{ src, want string }{
		// integer division and remainder truncate toward zero
		{"7/2", "3"},
		{"-7/2", "-3"},
		{"-7%3", "-1"},
		{"10 % 3.5", "3.0"},
		{"5L / 2", "2"},
		{"5 / 2f", "2.5"},
		// int and long overflow wrap around
		{"Integer.MAX_VALUE+1", "-2147483648"},
		{"Integer.MIN_VALUE / -1", "-2147483648"},
		{"Long.MAX_VALUE+1", "-9223372036854775808"},
		{"int a = 1000000; a * a", "-727379968"},
		{"long a = 1000000; a * a", "1000000000000"},
		{"2147483647 + 1L", "2147483648"},
		{"byte b = 127; b++; b", "-128"},
		{"short s = 32767; s += 1; s", "-32768"},
		// shifts use the low bits of the shift distance
		{"1 << 33", "2"},
		{"-7 >> 1", "-4"},
		{"-16 >>> 28", "15"},
		{"~5", "-6"},
		// floating point
		{"0.1+0.2", "0.30000000000000004"},
		{"0.1f + 0.2f", "0.3"},
		{"1/3.0f", "0.33333334"},
		{"1.0/0", "Infinity"},
		{"0.0/0", "NaN"},
		{"-0.0", "-0.0"},
		{"0.0 == -0.0", "true"},
		{"Double.NaN == Double.NaN", "false"},
		{"1e21", "1.0E21"},
		{"12345678.9", "1.23456789E7"},
		{"2.0e-5", "2.0E-5"},
		{"Double.MIN_VALUE", "4.9E-324"},
		{"Float.MAX_VALUE", "3.4028235E38"},
		{"017", "15"},
		// casts
		{"(int)-3.9", "-3"},
		{"(byte)300", "44"},
		{"(byte) -129", "127"},
		{"(short) 70000", "4464"},
		{"(int) 2147483648L", "-2147483648"},
		{"(int) 1e20", "2147483647"},
		{"(long) Double.NaN", "0"},
		{"(double)(float)0.1", "0.10000000149011612"},
		{"(char)65", "A"},
		{"(char) 66 + (char) 1", "67"},
		{"(int) 'Z'.charAt(0)", "90"},
		{"true ? 1 : 2.5", "1.0"},
		// equality and identity
		{"1 == 1L", "true"},
		{"def x = 1000; def y = 1000; x === y", "false"},
		{"def a = [1]; def b = [1]; a == b", "true"},
		{"def a = [1]; def b = [1]; a === b", "false"},
		// increments and compound assignment
		{"int x = 3; x++ + ++x", "8"},
		{"int x = 10; x -= 3; x *= 2; x /= 3; x", "4"},
		{"def x = 1; x += 1.5; x", "2"},
		// strings
		{"'café'.length()", "4"},
		{"'日本語'.substring(1)", "本語"},
		{"'Ünïcödé'.toUpperCase()", "ÜNÏCÖDÉ"},
		{"'abc'.compareTo('abd')", "-1"},
		{"'abc' + 1 + 2", "abc12"},
		{"1 + 2 + 'abc'", "3abc"},
		{"'x' + null", "xnull"},
		{"'abc'.hashCode()", "96354"},
		{"'abc'.chars().sum()", "294"},
		{"'a,b,,c'.splitOnToken(',').length", "4"},
		{"'a::b::c'.splitOnToken('::', 2).length", "2"},
		{"'hello'.encodeBase64()", "aGVsbG8="},
		{"Integer.toHexString(-1)", "ffffffff"},
		{"Integer.toString(255,16)", "ff"},
		{"Double.toString(1e-7)", "1.0E-7"},
		// collections render like Java's toString
		{"[1, 'a', 2.5, null, true]", "[1, a, 2.5, null, true]"},
		{"['k': [1, ['n': null]]]", "{k=[1, {n=null}]}"},
		{"def m = ['z':1,'a':2,'m':3]; m.keySet()", "[a, z, m]"},
		{"def l = [1,2,3]; l[-1]", "3"},
		{"[1,2,3].sum()", "6.0"},
		// null safety and elvis
		{"def m = null; m?.a?.b ?: 'fallback'", "fallback"},
		{"def x = false; x ?: 'elvis'", "false"},
		{"def x = 0; x ?: 'elvis'", "0"},
		// Math rounding
		{"Math.round(-2.5)", "-2"},
		{"Math.rint(2.5)", "2.0"},
		{"Math.abs(Integer.MIN_VALUE)", "2.147483648E9"},
		{"Math.min(-0.0, 0.0)", "-0.0"},
		{"Math.max(3,4)", "4.0"},
		// functions and java.time
		{"int fact(int n) { return n <= 1 ? 1 : n * fact(n - 1); } return fact(10);", "3628800"},
		{"ZonedDateTime.parse('2020-01-02T03:04:05Z').plusDays(30).toString()", "2020-02-01T03:04:05Z"},
		{"ChronoUnit.DAYS.between(LocalDate.of(2020,1,1), LocalDate.of(2021,1,1))", "366"},
		{"DateTimeFormatter.ofPattern('yyyy/MM/dd').format(LocalDate.of(2021, 3, 4))", "2021/03/04"},
		{"return null;", "null"},
	}
	for _, tt := range tests {
		got, cerr, rerr := execute(t, tt.src)
		if cerr != nil {
			t.Errorf("%s: compile error %s: %s", tt.src, cerr.Type, cerr.Reason)
			continue
		}
		if rerr != nil {
			t.Errorf("%s: runtime error %s: %s", tt.src, rerr.Type, rerr.Reason)
			continue
		}
		if got != tt.want {
			t.Errorf("%s = %q, want %q", tt.src, got, tt.want)
		}
	}
}

func TestExecuteErrors(t *testing.T) {
	tests := []struct {
		src, typ, reason string
		runtime          bool
	}{
		{"1/0", "arithmetic_exception", "/ by zero", false}, // constant folding fails at compile time
		{"[1,2,3][5]", "index_out_of_bounds_exception", "Index 5 out of bounds for length 3", true},
		{"x = ", "illegal_argument_exception", "unexpected end of script.", false},
		{"doc['x']", "illegal_argument_exception", "cannot resolve symbol [doc]", false},
		{"int x = 'a'", "class_cast_exception", "Cannot cast from [java.lang.String] to [int].", false},
		{"long y = 5; int x = y; x", "class_cast_exception", "Cannot cast from [long] to [int].", false},
		{"float f = 1.1; f", "class_cast_exception", "Cannot cast from [double] to [float].", false},
		{"(String)5", "class_cast_exception", "Cannot cast from [int] to [java.lang.String].", false},
		{"'abc' < 'abd'", "class_cast_exception", "cannot apply the less than operator [<] to the types [java.lang.String] and [java.lang.String]", false},
		{"false | true", "class_cast_exception", "cannot apply the boolean or operator [|] to the types [boolean] and [boolean]", false},
		{"Math.floorDiv(7, 2)", "illegal_argument_exception", "static method [java.lang.Math, floorDiv/2] not found", false},
		{"'x'.repeat(3)", "illegal_argument_exception", "member method [java.lang.String, repeat/1] not found", false},
		{"new Thread()", "illegal_argument_exception", "Not a type [Thread].", false},
		{"null == null", "illegal_argument_exception", "extraneous comparison of [null] constants", false},
		{"String s = 'abc'; s[1]", "illegal_argument_exception", "Illegal array access on type [java.lang.String].", false},
		{"1_000", "illegal_argument_exception", "unexpected token ['_000'] was expecting one of [{<EOF>, ';'}].", false},
		{"Integer x = 1000; Integer y = 1000; x == y", "class_cast_exception", "Cannot cast from [int] to [java.lang.Integer].", false},
	}
	for _, tt := range tests {
		_, cerr, rerr := execute(t, tt.src)
		var typ, reason string
		switch {
		case cerr != nil && !tt.runtime:
			typ, reason = cerr.Type, cerr.Reason
		case rerr != nil && tt.runtime:
			typ, reason = rerr.Type, rerr.Reason
		default:
			t.Errorf("%s: compile error %v, runtime error %v; want a %s at runtime=%v", tt.src, cerr, rerr, tt.typ, tt.runtime)
			continue
		}
		if typ != tt.typ || reason != tt.reason {
			t.Errorf("%s: got %s [%s], want %s [%s]", tt.src, typ, reason, tt.typ, tt.reason)
		}
	}
}
