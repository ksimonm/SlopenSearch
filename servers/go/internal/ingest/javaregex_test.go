package ingest

import "testing"

func TestGrokTranslate(t *testing.T) {
	cases := []struct{ in, out, msg string }{
		{`(?<a.b>x)\k<a.b>`, `(?<g1>x)\k<g1>`, ""},
		{`(?m)a.\h[\h]`, `(?s)a.[0-9a-fA-F][0-9a-fA-F]`, ""},
		{`[(?<x>]`, `[(?<x>]`, ""},
		{`\(?<x>`, `\(?<x>`, ""},
		{`(?<=a)(?<!b)`, `(?<=a)(?<!b)`, ""},
		{`(?<1x>a)`, "", "invalid group name <1x>"},
		{`(?<>a)`, "", "group name is empty"},
		{`\k<zz>`, "", "undefined name <zz> reference"},
		{`\k<1>`, "", "invalid backref number/name"},
	}
	for _, c := range cases {
		out, _, msg := grokTranslate(c.in)
		if out != c.out || msg != c.msg {
			t.Errorf("grokTranslate(%q) = %q, %q; want %q, %q", c.in, out, msg, c.out, c.msg)
		}
	}
}

func TestGrokRegex(t *testing.T) {
	g, msg := grokRegexCompile(`(?<x>a)|(?<x>b)`)
	if g == nil {
		t.Fatal(msg)
	}
	caps, ok := g.find("b")
	if !ok || caps[1] != nil || caps[2] == nil || *caps[2] != "b" {
		t.Errorf("duplicate names: %v %v", caps, ok)
	}
	for p, want := range map[string]string{
		"a)":       "unmatched close parenthesis",
		"(?<x>a":   "end pattern with unmatched parenthesis",
		"[a":       "premature end of char-class",
		"*a":       "target of repeat operator is not specified",
		"a{2,1}":   "upper is smaller than lower in repeat range",
		"[z-a]":    "empty range in char class",
		`\p{Foo}`:  "invalid character property name <Foo>",
		"x(?<=a+)": "invalid pattern in look-behind",
	} {
		if g, msg := grokRegexCompile(p); g != nil || msg != want {
			t.Errorf("grokRegexCompile(%q) = %q, want %q", p, msg, want)
		}
	}
	if g, _ := grokRegexCompile(`^(?<x>b)$`); g == nil {
		t.Fatal("compile")
	} else if caps, ok := g.find("a\nb\nc"); !ok || *caps[1] != "b" {
		t.Error("^ and $ are line anchors in Ruby syntax")
	}
}
