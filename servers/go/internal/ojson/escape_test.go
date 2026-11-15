package ojson

import "testing"

func TestJacksonEscapes(t *testing.T) {
	tests := []struct{ in, want string }{
		{`{"a":"plain"}`, `{"a":"plain"}`},
		{`"a\u001fb\u0001"`, `"a\u001Fb\u0001"`},
		{`"\\u001f"`, `"\\u001f"`},
		{`"\\\u001f\n"`, `"\\\u001F\n"`},
		{`"x y"`, "\"x y\""},
		{`"😀"`, `"😀"`},
	}
	for _, tt := range tests {
		if got := string(JacksonEscapes([]byte(tt.in))); got != tt.want {
			t.Errorf("JacksonEscapes(%s) = %s, want %s", tt.in, got, tt.want)
		}
	}
	// test_error_message_escapes_control_chars_upper_case
	if got := string(JacksonEscapes(Marshal(map[string]any{"r": "a\x1fb\x01"}))); got != `{"r":"a\u001Fb\u0001"}` {
		t.Errorf("JacksonEscapes(Marshal) = %s", got)
	}
}
