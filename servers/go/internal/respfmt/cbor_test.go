package respfmt

import (
	"encoding/json"
	"testing"

	"slopensearch/internal/ojson"
)

func TestToCBOR(t *testing.T) {
	tree := ojson.Object{
		{K: "took", V: json.Number("4")},
		{K: "status", V: json.Number("400")},
		{K: "neg", V: json.Number("-3")},
		{K: "max_score", V: json.Number("1.5")},
		{K: "avg", V: json.Number("1.5")},
		{K: "hits", V: []any{true, nil, "ab"}},
	}
	want := "\xbf" +
		"\x64took\x04" +
		"\x66status\x19\x01\x90" +
		"\x63neg\x22" +
		"\x69max_score\xfa\x3f\xc0\x00\x00" +
		"\x63avg\xfb\x3f\xf8\x00\x00\x00\x00\x00\x00" +
		"\x64hits\x9f\xf5\xf6\x62ab\xff" +
		"\xff"
	if got := string(toCBOR(tree)); got != want {
		t.Errorf("toCBOR = %x, want %x", got, want)
	}
}
