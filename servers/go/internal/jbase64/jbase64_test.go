package jbase64

import "testing"

// expected values: tests/reference/test_37_parity_time_hash.jsonl
func TestDecodeString(t *testing.T) {
	cases := []struct{ in, out, err string }{
		{"SGVsbG8=", "Hello", ""},
		{"SGVsbG8", "Hello", ""},
		{"SGVsbA", "Hell", ""},
		{"SGVsbA=", "", "Input byte array has wrong 4-byte ending unit"},
		{"SGVsbA==", "Hell", ""},
		{"S", "", "Input byte[] should at least have 2 bytes for base64 bytes"},
		{"SGVsbG8=x", "", "Input byte array has incorrect ending byte at 8"},
		{"SGV sbG8=", "", "Illegal base64 character 20"},
		{"SGVsbH9=", "Hell\x7f", ""},
		{"!!!", "", "Illegal base64 character 21"},
		{"", "", ""},
		{"w6k=", "é", ""},
		{"SGVsbA===", "", "Input byte array has incorrect ending byte at 8"},
		{"SGVsb", "", "Last unit does not have enough valid bits"},
		{"=", "", "Input byte[] should at least have 2 bytes for base64 bytes"},
		{"SGVs==", "", "Input byte array has wrong 4-byte ending unit"},
		{"4pyTIMOgIGxhIG1vZGU", "✓ à la mode", ""},
		{"SGé=", "", "Illegal base64 character -17"},
	}
	for _, c := range cases {
		got, err := DecodeString(c.in)
		if c.err != "" {
			if err == nil || err.Error() != c.err {
				t.Errorf("%q: error %v, want %q", c.in, err, c.err)
			}
			continue
		}
		if err != nil || string(got) != c.out {
			t.Errorf("%q: %q, %v; want %q", c.in, got, err, c.out)
		}
	}
}
