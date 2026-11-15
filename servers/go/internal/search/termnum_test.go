package search

import "testing"

// Expected errors from OpenSearch 3.9 term queries on number fields.
func TestCheckQueryNumber(t *testing.T) {
	tests := []struct{ ftype, in, typ, reason string }{
		{"long", "ab", "number_format_exception", `For input string: "ab"`},
		{"long", "a\x1fb\x01", "number_format_exception", "For input string: \"a\x1fb\x01\""},
		{"unsigned_long", "0x10", "number_format_exception", `For input string: "0x10"`},
		{"double", "", "number_format_exception", "empty String"},
		{"integer", " ", "number_format_exception", "empty String"},
		{"long", "99", "", ""},
		{"long", "1e3", "", ""},
		{"double", " 5", "", ""},
		{"long", "99999999999999999999", "illegal_argument_exception",
			"Value [[39 39 39 39 39 39 39 39 39 39 39 39 39 39 39 39 39 39 39 39]] is out of range for a long"},
		{"short", "99999999999999999999", "illegal_argument_exception",
			"Value [[39 39 39 39 39 39 39 39 39 39 39 39 39 39 39 39 39 39 39 39]] is out of range for an integer"},
	}
	for _, tt := range tests {
		err := checkQueryNumber(tt.ftype, tt.in)
		if tt.typ == "" {
			if err != nil {
				t.Errorf("checkQueryNumber(%s, %q) = %v", tt.ftype, tt.in, err)
			}
			continue
		}
		if err == nil || err.Type != tt.typ || err.Reason != tt.reason {
			t.Errorf("checkQueryNumber(%s, %q) = %+v, want %s %s", tt.ftype, tt.in, err, tt.typ, tt.reason)
		}
	}
}
