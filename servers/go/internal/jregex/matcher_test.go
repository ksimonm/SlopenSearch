package jregex

import (
	"reflect"
	"testing"
)

func TestSplitLikeJava(t *testing.T) {
	cases := []struct {
		pat, in string
		limit   int
		want    []string
	}{
		{",\\s*", "a, b,c,,", 0, []string{"a", "b", "c"}},
		{"(?=b)", "abab", 0, []string{"a", "ba", "b"}},
		{"a", "aaxa", 2, []string{"", "axa"}},
		{"x*+", "axxbxc", 0, []string{"a", "", "b", "", "c"}},
		{"z", "abc", 0, []string{"abc"}},
		{",", "a,b,,", -1, []string{"a", "b", "", ""}},
		{",", "a,b,c", 1, []string{"a,b,c"}},
		{"", "abc", 0, []string{"a", "b", "c"}},
	}
	for _, c := range cases {
		got := mustCompile(t, c.pat, 0).Split(c.in, c.limit)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("split(%q, %q, %d) = %q, want %q", c.pat, c.in, c.limit, got, c.want)
		}
	}
}

func TestMatcherEntryPoints(t *testing.T) {
	re := mustCompile(t, "(?<=foo)bar", 0)
	if m := re.FindFrom([]rune("foobar"), 3, nil); m == nil || m[0] != 3 {
		t.Errorf("lookbehind before the start position: %v", m)
	}
	re = mustCompile(t, "(a)(b)?", 0)
	if m := re.MatchAll([]rune("a"), nil); !reflect.DeepEqual(m, []int{0, 1, 0, 1, -1, -1}) {
		t.Errorf("MatchAll = %v", m)
	}
	if m := re.MatchAll([]rune("ac"), nil); m != nil {
		t.Errorf("MatchAll must anchor the end: %v", m)
	}
	if m := re.MatchPrefix([]rune("abc"), nil); m == nil || m[1] != 2 {
		t.Errorf("MatchPrefix = %v", m)
	}
	if m := mustCompile(t, "\\d", 0).MatchPrefix([]rune("a1"), nil); m != nil {
		t.Errorf("lookingAt must anchor the start: %v", m)
	}
	re = mustCompile(t, "(?<year>\\d{4})", 0)
	if g, ok := re.GroupIndex("year"); !ok || g != 1 {
		t.Errorf("GroupIndex = %d %v", g, ok)
	}
	if !mustCompile(t, "(ab)\\1", 0).MatchString("abab") || mustCompile(t, "a++a", 0).MatchString("aaa") {
		t.Error("backreference / possessive")
	}
}

func TestBudget(t *testing.T) {
	re := mustCompile(t, "(a|aa)+b", 0)
	b := &Budget{Steps: 1000}
	in := []rune("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaac")
	if m := re.FindFrom(in, 0, b); m != nil || !b.Exceeded {
		t.Errorf("budget not enforced: %v %v", m, b.Exceeded)
	}
	b = &Budget{Steps: 1000}
	if m := mustCompile(t, "c", 0).FindFrom(in, 0, b); m == nil || b.Exceeded {
		t.Errorf("small match exceeded the budget")
	}
}

func TestGroupNameErrors(t *testing.T) {
	cases := map[string]string{
		"(?<1a>x)":       "capturing group name does not start with a Latin letter near index 3\n(?<1a>x)\n   ^",
		"(?<a>x)(?<a>y)": "Named capturing group <a> is already defined near index 11\n(?<a>x)(?<a>y)\n           ^",
		"(?<a-b>x)":      "named capturing group is missing trailing '>' near index 4\n(?<a-b>x)\n    ^",
		"a{2,1}":         "Illegal repetition range near index 5\na{2,1}\n     ^",
	}
	for p, want := range cases {
		_, err := Compile(p, 0)
		if err == nil || err.Message() != want {
			t.Errorf("Compile(%q) error = %v, want %q", p, err, want)
		}
	}
}

func TestExpandReplacementErrors(t *testing.T) {
	re := mustCompile(t, "(?<x>a)", 0)
	in := []rune("a")
	caps := re.findAt(in, 0)
	cases := map[string]string{
		"$":     "Illegal group reference: group index is missing",
		"$a":    "Illegal group reference",
		"${y}":  "No group with name {y}",
		"${1x}": "capturing group name {1x} starts with digit character",
		"${x":   "named capturing group is missing trailing '}'",
		"${}":   "named capturing group has 0 length name",
		"\\":    "character to be escaped is missing",
		"$2":    "No group 2",
	}
	for repl, want := range cases {
		_, err := re.ExpandReplacement(in, caps, repl)
		if err == nil || err.Reason != want {
			t.Errorf("ExpandReplacement(%q) = %v, want %q", repl, err, want)
		}
	}
	out, err := re.ExpandReplacement(in, caps, "[${x}$12\\$]")
	if err != nil || string(out) != "[aa2$]" {
		t.Errorf("ExpandReplacement = %q %v", string(out), err)
	}
}
