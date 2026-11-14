package jregex

import (
	"fmt"
	"strings"
	"testing"
)

func mustCompile(t *testing.T, pattern string, flags int) *Regexp {
	t.Helper()
	re, err := Compile(pattern, flags)
	if err != nil {
		t.Fatalf("Compile(%q): %s", pattern, err.Message())
	}
	return re
}

// fmtMatches renders each match as text@start (rune index), followed by its
// groups in parentheses; an unset group shows as <nil>.
func fmtMatches(in []rune, ms [][]int) string {
	parts := make([]string, 0, len(ms))
	for _, m := range ms {
		s := fmt.Sprintf("%s@%d", string(in[m[0]:m[1]]), m[0])
		if len(m) > 2 {
			gs := make([]string, 0, len(m)/2-1)
			for g := 2; g+1 < len(m); g += 2 {
				if m[g] < 0 {
					gs = append(gs, "<nil>")
				} else {
					gs = append(gs, string(in[m[g]:m[g+1]]))
				}
			}
			s += "(" + strings.Join(gs, "|") + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

// TestFindAll follows java.util.regex Matcher.find semantics. The first
// cases mirror painless and analysis patterns recorded against OpenSearch
// (tests/reference/test_43_scripting.jsonl, test_51_analysis.jsonl).
func TestFindAll(t *testing.T) {
	cases := []struct {
		name, pattern, flags, in, want string
	}{
		{"literal", `b`, "", "abc", "b@1"},
		{"plus", `a+`, "", "caaab", "aaa@1"},
		{"counted", `[0123456789]{3}`, "", "fd-786-335-514-x", "786@3 335@7 514@11"},
		{"class_run", `[a-z]+`, "", "abc123def", "abc@0 def@6"},
		{"groups", `(\d+)-(\d+)`, "", "10-20", "10-20@0(10|20)"},
		{"groups_email", `(\w+)@(\w+)\.com`, "", "joe@example.com", "joe@example.com@0(joe|example)"},
		{"flag_case_insensitive", `X`, "CASE_INSENSITIVE", "aXbxc", "X@1 x@3"},
		{"end_anchor", `World$`, "", "Hello World", "World@6"},
		{"alternation_leftmost_first", `cat|category`, "", "category", "cat@0"},
		{"greedy", `a.*b`, "", "aXbYb", "aXbYb@0"},
		{"lazy", `a.*?b`, "", "aXbYb", "aXb@0"},
		{"possessive_no_backtrack", `a++a`, "", "aaa", ""},
		{"atomic_no_backtrack", `(?>a+)a`, "", "aaa", ""},
		{"range_quantifier", `a{2,3}`, "", "aaaaaaa", "aaa@0 aaa@3"},
		{"empty_matches", `a*`, "", "baa", "@0 aa@1 @3"},
		{"optional_group_unset", `(a)?b`, "", "b", "b@0(<nil>)"},
		{"non_capturing", `(?:ab)+`, "", "ababx", "abab@0"},
		{"caret_single_line", `^\w+`, "", "ab\ncd", "ab@0"},
		{"caret_multiline", `^\w+`, "MULTILINE", "ab\ncd", "ab@0 cd@3"},
		{"dollar_single_line", `\w+$`, "", "ab\ncd", "cd@3"},
		{"dot_excludes_newline", `.+`, "", "a\nb", "a@0 b@2"},
		{"dotall_inline", `(?s).+`, "", "a\nb", "a\nb@0"},
		{"word_boundary", `\bis\b`, "", "this is", "is@5"},
		{"lookahead", `\d+(?=px)`, "", "10px 20em", "10@0"},
		{"negative_lookahead", `foo(?!bar)`, "", "foobar foobaz", "foo@7"},
		{"lookbehind", `(?<=\$)\d+`, "", "$100 200", "100@1"},
		{"negative_lookbehind", `(?<![$\d])\d+`, "", "$100 200", "200@5"},
		{"camel_split_points", `(?<=\p{Lower})(?=\p{Upper})`, "", "camelCaseString", "@5 @9"},
		{"backref", `(\w)\1`, "", "aabbcd", "aa@0(a) bb@2(b)"},
		{"named_backref", `(?<x>\w)\k<x>`, "", "xyyz", "yy@1(y)"},
		{"inline_case_insensitive", `(?i)hello`, "", "HeLLo", "HeLLo@0"},
		{"case_insensitive_ascii_only", `(?i)é`, "", "É", ""},
		{"unicode_case", `(?iu)é`, "", "É", "É@0"},
		{"negated_class", `[^aeiou\s]+`, "", "hello world", "h@0 ll@2 w@6 rld@8"},
		{"class_intersection", `[a-z&&[^aeiou]]+`, "", "hello", "h@0 ll@2"},
		{"word_is_ascii", `\w+`, "", "café", "caf@0"},
		{"unicode_letters", `\p{L}+`, "", "naïve 42 façade", "naïve@0 façade@9"},
		{"unicode_categories", `\p{Lu}\p{Ll}+`, "", "camelCaseABC", "Case@5"},
		{"non_bmp_is_one_code_point", `.`, "", "😀a", "😀@0 a@1"},
		{"quoted_literal", `\Qa.b\E`, "", "axb a.b", "a.b@4"},
		{"literal_flag", `a.b`, "LITERAL", "axb a.b", "a.b@4"},
		{"comments_flag", `a b # trailing comment`, "COMMENTS", "xab", "ab@1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flags, ferr := ParseFlags(tc.flags)
			if ferr != nil {
				t.Fatalf("ParseFlags(%q): %s", tc.flags, ferr.Reason)
			}
			re := mustCompile(t, tc.pattern, flags)
			in := []rune(tc.in)
			if got := fmtMatches(in, re.FindAll(in)); got != tc.want {
				t.Errorf("FindAll(%q, %q)\n got: %q\nwant: %q", tc.pattern, tc.in, got, tc.want)
			}
		})
	}
}

// TestFullMatch follows Matcher.matches; the \d{3} cases are recorded painless
// results.
func TestFullMatch(t *testing.T) {
	cases := []struct {
		pattern, in string
		want        bool
	}{
		{`^\d{3}$`, "123", true},
		{`^\d{3}$`, "1234", false},
		{`abc`, "abc", true},
		{`b`, "abc", false},
		{`(\d+)-(\d+)`, "10-20", true},
		{`a|ab`, "ab", true}, // backtracks into the second alternative
		{`a*+a`, "aa", false},
		{``, "", true},
	}
	for _, tc := range cases {
		re := mustCompile(t, tc.pattern, 0)
		if got := re.FullMatch([]rune(tc.in)); got != tc.want {
			t.Errorf("FullMatch(%q, %q) = %v, want %v", tc.pattern, tc.in, got, tc.want)
		}
	}
}

func TestReplace(t *testing.T) {
	cases := []struct {
		name, pattern, in, repl string
		all                     bool
		want                    string
	}{
		{"replace_all", `a+`, "caaab", "X", true, "cXb"},
		{"numbered_groups", `(\d+)-(\d+)-(\d+)`, "2021-03-04", "$3/$2", true, "04/03"},
		{"named_groups", `(?<y>\d{4})-(?<m>\d\d)`, "2021-03", "${m}.${y}", true, "03.2021"},
		{"first_only", `a`, "banana", "", false, "bnana"},
		{"every_match", `(\d+)`, "a1b22 c", "<$1>", true, "a<1>b<22> c"},
		{"escaped_dollar", `x`, "x", `\$1`, true, "$1"},
		{"group_number_falls_back_to_one_digit", `(a)`, "a", "$10", true, "a0"},
		{"unset_group_is_empty", `(a)?b`, "b", "[$1]", true, "[]"},
		{"empty_matches_between_chars", `x*`, "abc", "-", true, "-a-b-c-"},
		{"insert_at_lookaround", `(?<=\p{Lower})(?=\p{Upper})`, "camelCaseString", " ", true, "camel Case String"},
		{"no_match", `z`, "abc", "y", true, "abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			re := mustCompile(t, tc.pattern, 0)
			got, err := re.Replace(tc.in, tc.repl, tc.all)
			if err != nil {
				t.Fatalf("Replace: %s: %s", err.Type, err.Reason)
			}
			if got != tc.want {
				t.Errorf("Replace(%q, %q, %q) = %q, want %q", tc.pattern, tc.in, tc.repl, got, tc.want)
			}
		})
	}
}

// TestReplaceErrors checks the Matcher.appendReplacement exceptions.
func TestReplaceErrors(t *testing.T) {
	cases := []struct {
		repl, typ, reason string
	}{
		{"$", "illegal_argument_exception", "Illegal group reference: group index is missing"},
		{"$2", "index_out_of_bounds_exception", "No group 2"},
		{"${nope}", "illegal_argument_exception", "No group with name {nope}"},
		{`x\`, "illegal_argument_exception", "character to be escaped is missing"},
	}
	re := mustCompile(t, `(?<a>x)`, 0)
	for _, tc := range cases {
		_, err := re.Replace("x", tc.repl, true)
		if err == nil {
			t.Errorf("Replace with %q: expected error", tc.repl)
			continue
		}
		if err.Type != tc.typ || err.Reason != tc.reason {
			t.Errorf("Replace with %q: got %s: %q, want %s: %q", tc.repl, err.Type, err.Reason, tc.typ, tc.reason)
		}
	}
}

func TestCompileErrors(t *testing.T) {
	cases := []struct {
		pattern string
		// prefix of PatternSyntaxException.getMessage, "" for any error
		prefix string
	}{
		{"(", "Unclosed group near index 1\n("}, // recorded pattern tokenizer error
		{"[abc", "Unclosed character class"},
		{"*a", "Dangling meta character '*'"},
		{"a)", ""},
		{"a{2,1}", ""},
		{`(?<n>a)\k<m>`, ""},
	}
	for _, tc := range cases {
		re, err := Compile(tc.pattern, 0)
		if err == nil {
			t.Errorf("Compile(%q) = %v, want error", tc.pattern, re)
			continue
		}
		if !strings.HasPrefix(err.Message(), tc.prefix) {
			t.Errorf("Compile(%q) message %q, want prefix %q", tc.pattern, err.Message(), tc.prefix)
		}
	}
}

func TestParseFlagsAndGroups(t *testing.T) {
	f, err := ParseFlags("case_insensitive | COMMENTS")
	if err != nil {
		t.Fatalf("ParseFlags: %s", err.Reason)
	}
	if f != flagCaseInsensitive|flagComments {
		t.Errorf("ParseFlags = %b, want %b", f, flagCaseInsensitive|flagComments)
	}
	if _, err := ParseFlags("BOGUS"); err == nil || err.Reason != "Unknown regex flag [BOGUS]" {
		t.Errorf("ParseFlags(BOGUS) error = %v", err)
	}
	for pattern, n := range map[string]int{`a`: 0, `(a)(?:b)(?<c>c)`: 2, `(?=(x))y`: 1} {
		if got := mustCompile(t, pattern, 0).NumGroups(); got != n {
			t.Errorf("NumGroups(%q) = %d, want %d", pattern, got, n)
		}
	}
}
