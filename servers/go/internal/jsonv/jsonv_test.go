package jsonv

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestFloatMarshal(t *testing.T) {
	// Jackson writes Java doubles with a fraction: script_fields values such
	// as [20.0] or [3.5] in tests/reference/test_43_scripting.jsonl.
	tests := []struct {
		f    float64
		want string
	}{
		{20, "20.0"},
		{0, "0.0"},
		{-3, "-3.0"},
		{3.5, "3.5"},
		{2.302585092994046, "2.302585092994046"},
		{0.1, "0.1"},
	}
	for _, tt := range tests {
		b, err := json.Marshal(map[string]any{"x": []any{Float(tt.f)}})
		if err != nil {
			t.Fatal(err)
		}
		if want := `{"x":[` + tt.want + `]}`; string(b) != want {
			t.Errorf("Float(%v) = %s, want %s", tt.f, b, want)
		}
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		f    float64
		want string
	}{
		{2, "2"}, // {"size":"{{s}}"} with s=2 renders "2"
		{-7, "-7"},
		{0, "0"},
		{5.5, "5.5"},
		{0.25, "0.25"},
		{1e14, "100000000000000"},
	}
	for _, tt := range tests {
		if got := FormatNumber(tt.f); got != tt.want {
			t.Errorf("FormatNumber(%v) = %q, want %q", tt.f, got, tt.want)
		}
	}
}

func TestEncodeString(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", `"plain"`},
		{"", `""`},
		{`a"b`, `"a\"b"`},
		{`a\b`, `"a\\b"`},
		{"l1\nl2\r\t", `"l1\nl2\r\t"`},
		{"\b\f", `"\b\f"`},
		// no HTML escaping, no escaping of '/' or non-ASCII
		{"<a>&/", `"<a>&/"`},
		{"é☃", `"é☃"`},
	}
	for _, tt := range tests {
		if got := EncodeString(tt.in); got != tt.want {
			t.Errorf("EncodeString(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestToFloat(t *testing.T) {
	tests := []struct {
		in     any
		want   float64
		wantOK bool
	}{
		{3.5, 3.5, true},
		{float32(0.5), 0.5, true},
		{Float(2), 2, true},
		{7, 7, true},
		{int64(-7), -7, true},
		{uint64(9), 9, true},
		{json.Number("1e3"), 1000, true},
		{json.Number("x"), 0, false},
		{true, 1, true},
		{false, 0, true},
		{" 2.5 ", 2.5, true},
		{"abc", 0, false},
		{nil, 0, false},
		{[]any{1.0}, 0, false},
		{map[string]any{}, 0, false},
	}
	for _, tt := range tests {
		got, ok := ToFloat(tt.in)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("ToFloat(%#v) = %v, %v, want %v, %v", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestToInt64(t *testing.T) {
	tests := []struct {
		in     any
		want   int64
		wantOK bool
	}{
		{int64(5), 5, true},
		{5, 5, true},
		{uint64(math.MaxInt64), math.MaxInt64, true},
		{uint64(math.MaxInt64) + 1, 0, false},
		{json.Number("-12"), -12, true},
		{json.Number("9223372036854775807"), math.MaxInt64, true},
		{json.Number("5.0"), 0, false},
		{json.Number("1e2"), 0, false},
		{5.0, 0, false},
		{"5", 0, false},
	}
	for _, tt := range tests {
		got, ok := ToInt64(tt.in)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("ToInt64(%#v) = %v, %v, want %v, %v", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestScalarAndToString(t *testing.T) {
	tests := []struct {
		in         any
		scalar     string
		scalarOK   bool
		toString   string
		toStringOK bool
	}{
		{"s", "s", true, "s", true},
		{3.0, "3", true, "3", true},
		{2.5, "2.5", true, "2.5", true},
		{json.Number("1.50"), "1.50", true, "1.50", true},
		{true, "true", true, "true", true},
		{int64(42), "", false, "42", true},
		{nil, "", false, "", false},
		{[]any{"a"}, "", false, "", false},
	}
	for _, tt := range tests {
		if got, ok := ScalarString(tt.in); got != tt.scalar || ok != tt.scalarOK {
			t.Errorf("ScalarString(%#v) = %q, %v, want %q, %v", tt.in, got, ok, tt.scalar, tt.scalarOK)
		}
		if got, ok := ToString(tt.in); got != tt.toString || ok != tt.toStringOK {
			t.Errorf("ToString(%#v) = %q, %v, want %q, %v", tt.in, got, ok, tt.toString, tt.toStringOK)
		}
	}
}

func TestNormalizeNumbers(t *testing.T) {
	// A source written as {"a":1.0,"b":1,"c":1e2} comes back with the
	// doubles as doubles and the long as a long.
	var v any
	if err := Unmarshal([]byte(`{"a":1.0,"b":1,"c":1e2,"d":[2.50,{"e":0.0}]}`), &v); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(NormalizeNumbers(v))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"a":1.0,"b":1,"c":100.0,"d":[2.5,{"e":0.0}]}`; string(b) != want {
		t.Errorf("NormalizeNumbers = %s, want %s", b, want)
	}
}

func TestDeepCopyAndKeys(t *testing.T) {
	orig := map[string]any{"b": []any{map[string]any{"x": 1.0}}, "a": "s", "C": nil}
	cp := DeepCopy(orig).(map[string]any)
	cp["b"].([]any)[0].(map[string]any)["x"] = 2.0
	cp["a"] = "t"
	if orig["b"].([]any)[0].(map[string]any)["x"] != 1.0 || orig["a"] != "s" {
		t.Errorf("DeepCopy shares state with the original: %v", orig)
	}
	if got, want := SortedKeys(orig), []string{"C", "a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SortedKeys = %v, want %v", got, want)
	}
	if got := Int(map[string]any{"n": json.Number("7"), "f": 2.9, "s": "x"}, "f", -1); got != 2 {
		t.Errorf("Int(f) = %d, want 2", got)
	}
	if got := Int(map[string]any{"s": "x"}, "missing", -1); got != -1 {
		t.Errorf("Int(missing) = %d, want -1", got)
	}
}
