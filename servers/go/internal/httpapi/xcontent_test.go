package httpapi

import "testing"

func TestYAMLToJSON(t *testing.T) {
	cases := map[string]string{
		"---\na: 1\nb:\n  c: [x, 'y z']\n":                  `{"a":1,"b":{"c":["x","y z"]}}`,
		"f: yes\ng: 0x1F\nh: 1e3\np: 012\nr: 1_000\ni: ~\n": `{"f":true,"g":31,"h":1000.0,"p":10,"r":"1_000","i":null}`,
		"k: |\n  l1\n  l2\nt: 'it''s'\nn: \"1\"\n":          `{"k":"l1\nl2\n","t":"it's","n":"1"}`,
		"- 1\n- {a: b}\n":                                   `[1,{"a":"b"}]`,
		"a: &x 1\nb: *x\n":                                  `{"a":1,"b":1}`,
	}
	for in, want := range cases {
		got, err := yamlToJSON([]byte(in))
		if err != nil || string(got) != want {
			t.Errorf("yamlToJSON(%q) = %s, %v; want %s", in, got, err, want)
		}
	}
	if _, err := yamlToJSON([]byte("a: 1\na: 2\n")); err == nil {
		t.Error("duplicate keys must fail")
	}
}

func TestCBORToJSON(t *testing.T) {
	cases := map[string]string{
		// {"a": 1, "b": [-3, true, null]}
		"\xa2\x61a\x01\x61b\x83\x22\xf5\xf6": `{"a":1,"b":[-3,true,null]}`,
		// indefinite map, float32 1.1, half 1.0, double 100.0
		"\xbf\x61x\xfa\x3f\x8c\xcc\xcd\x61y\xf9\x3c\x00\x61z\xfb\x40\x59\x00\x00\x00\x00\x00\x00\xff": `{"x":1.1,"y":1.0,"z":100.0}`,
		// byte string, indefinite text string
		"\xa2\x61b\x42\x01\x02\x61s\x7f\x62ab\x61c\xff": `{"b":"AQI=","s":"abc"}`,
	}
	for in, want := range cases {
		got, err := cborToJSON([]byte(in))
		if err != nil || string(got) != want {
			t.Errorf("cborToJSON(%x) = %s, %v; want %s", in, got, err, want)
		}
	}
	if _, err := cborToJSON([]byte("\xa1\x61")); err == nil {
		t.Error("truncated input must fail")
	}
}
