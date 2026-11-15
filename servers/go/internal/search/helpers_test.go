package search

import (
	"encoding/json"
	"reflect"
	"testing"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("bad JSON %s: %v", s, err)
	}
	return v
}

// Source filtering results are the "_source" of the hits/documents in
// tests/reference/test_21_documents.jsonl (test_get_source_filtering, on
// docGet) and test_33_search_api.jsonl (test_source_filtering and
// test_uri_source_*, on docCorpus).
const (
	docGet    = `{"a":1,"o":{"x":1,"y":[1,2],"deep":{"z":"q"}},"t":"hello world"}`
	docCorpus = `{"title":"quick brown fox","body":"the quick brown fox jumps over the lazy dog","tag":"a","n":1,"nums":[3,10],"price":1.5,"date":"2024-01-01T00:00:00Z","loc":{"lat":0,"lon":0},"items":[{"name":"x","qty":5},{"name":"y","qty":1}],"obj":{"a":"p","b":1}}`
)

func TestFilterSource(t *testing.T) {
	tests := []struct {
		doc                string
		includes, excludes []string
		want               string
	}{
		{docGet, []string{"a"}, nil, `{"a":1}`},
		{docGet, []string{"o.x"}, nil, `{"o":{"x":1}}`},
		{docGet, []string{"o.*"}, nil, `{"o":{"deep":{"z":"q"},"x":1,"y":[1,2]}}`},
		{docGet, []string{"o"}, nil, `{"o":{"x":1,"y":[1,2],"deep":{"z":"q"}}}`},
		{docGet, []string{"o.deep.z"}, nil, `{"o":{"deep":{"z":"q"}}}`},
		{docGet, []string{"a", "t"}, nil, `{"a":1,"t":"hello world"}`},
		{docGet, []string{"*"}, nil, docGet},
		{docGet, []string{"nomatch"}, nil, `{}`},
		{docGet, []string{"O"}, nil, `{}`}, // case-sensitive
		{docGet, []string{"t*"}, nil, `{"t":"hello world"}`},
		{docGet, nil, []string{"o"}, `{"a":1,"t":"hello world"}`},
		{docGet, nil, []string{"o.y", "t"}, `{"a":1,"o":{"deep":{"z":"q"},"x":1}}`},
		{docGet, nil, []string{"*"}, `{}`},
		{docGet, nil, []string{"o.*"}, `{"a":1,"t":"hello world","o":{}}`},
		{docGet, []string{"o"}, []string{"o.deep"}, `{"o":{"x":1,"y":[1,2]}}`},
		{docGet, nil, nil, docGet},
		{docCorpus, []string{"tag", "obj.*"}, nil, `{"obj":{"a":"p","b":1},"tag":"a"}`},
		{docCorpus, []string{"items.name", "n"}, nil, `{"items":[{"name":"x"},{"name":"y"}],"n":1}`},
		{docCorpus, []string{"*"}, []string{"body", "items", "loc", "title", "nums"},
			`{"date":"2024-01-01T00:00:00Z","price":1.5,"obj":{"a":"p","b":1},"tag":"a","n":1}`},
		{docCorpus, []string{"obj.a"}, nil, `{"obj":{"a":"p"}}`},
		{docCorpus, []string{"tag", "n"}, nil, `{"tag":"a","n":1}`},
		{docCorpus, nil, []string{"title", "body", "items", "loc", "obj", "nums", "date", "price"}, `{"tag":"a","n":1}`},
		{docCorpus, []string{"obj"}, []string{"obj.b"}, `{"obj":{"a":"p"}}`},
	}
	for _, tt := range tests {
		doc := decode(t, tt.doc).(map[string]any)
		got := FilterSource(doc, tt.includes, tt.excludes)
		if want := decode(t, tt.want); !reflect.DeepEqual(any(got), want) {
			t.Errorf("FilterSource(inc=%q, exc=%q) = %v, want %s", tt.includes, tt.excludes, got, tt.want)
		}
		// the input document is left alone
		if !reflect.DeepEqual(any(doc), decode(t, tt.doc)) {
			t.Errorf("FilterSource(inc=%q, exc=%q) modified its input: %v", tt.includes, tt.excludes, doc)
		}
	}
}

func TestParseSourceFilter(t *testing.T) {
	tests := []struct {
		body               string
		includes, excludes []string
		omit, ok           bool
	}{
		{`{}`, nil, nil, false, false},
		{`{"_source":false}`, nil, nil, true, true},
		{`{"_source":true}`, nil, nil, false, true},
		{`{"_source":"false"}`, nil, nil, true, true},
		{`{"_source":"true"}`, nil, nil, false, true},
		{`{"_source":"obj.a"}`, []string{"obj.a"}, nil, false, true},
		{`{"_source":"a, t,"}`, []string{"a", "t"}, nil, false, true},
		{`{"_source":["tag","obj.*"]}`, []string{"tag", "obj.*"}, nil, false, true},
		{`{"_source":{"includes":["obj"],"excludes":["obj.b"]}}`, []string{"obj"}, []string{"obj.b"}, false, true},
		{`{"_source":{"excludes":["*"]}}`, []string{}, []string{"*"}, false, true},
		{`{"_source":{"includes":"n"}}`, []string{"n"}, []string{}, false, true},
		{`{"_source":1}`, nil, nil, false, false},
	}
	for _, tt := range tests {
		inc, exc, omit, ok := ParseSourceFilter(decode(t, tt.body).(map[string]any))
		if !reflect.DeepEqual(inc, tt.includes) || !reflect.DeepEqual(exc, tt.excludes) || omit != tt.omit || ok != tt.ok {
			t.Errorf("ParseSourceFilter(%s) = %q, %q, %v, %v, want %q, %q, %v, %v",
				tt.body, inc, exc, omit, ok, tt.includes, tt.excludes, tt.omit, tt.ok)
		}
	}
}

func TestStringList(t *testing.T) {
	tests := []struct {
		in   any
		want []string
	}{
		{"a", []string{"a"}},
		{decode(t, `["a",1,2.5,true,null,{}]`), []string{"a", "1", "2.5", "true"}},
		{nil, []string{}},
		{3.0, []string{}},
	}
	for _, tt := range tests {
		if got := StringList(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("StringList(%#v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseSortSpecs(t *testing.T) {
	// Errors are those of tests/reference/test_33_search_api.jsonl
	// (test_sort_invalid_order, test_sort_mode_invalid,
	// test_sort_format_option_not_supported, test_sort_geo_distance_bad_unit)
	// and test_43_scripting.jsonl (test_script_sort_requires_type,
	// test_script_sort_unknown_type). Only the parts the reference shows
	// without an XContent position are checked for the latter ones.
	errTests := []struct {
		sort, typ, reason, cause string
	}{
		{`[{"n":"up"}]`, "illegal_argument_exception", "No enum constant org.opensearch.search.sort.SortOrder.UP", ""},
		{`[{"n":{"order":"sideways"}}]`, "illegal_argument_exception", "No enum constant org.opensearch.search.sort.SortOrder.SIDEWAYS", ""},
		{`[{"n":{"order":"asc","mode":"bogus"}}]`, "x_content_parse_exception", "", "Unknown SortMode [bogus]"},
		{`[{"date":{"order":"desc","format":"yyyy-MM-dd"}}]`, "x_content_parse_exception", "", ""},
		{`[{"_geo_distance":{"loc":{"lat":0,"lon":0},"unit":"parsecs"}}]`, "illegal_argument_exception", "No distance unit match [parsecs]", ""},
		{`{"_script":{"script":{"source":"1"}}}`, "illegal_argument_exception", "Required [type]", ""},
		{`{"_script":{"type":"bogus","script":{"source":"1"}}}`, "x_content_parse_exception", "", "Unknown ScriptSortType [bogus]"},
	}
	for _, tt := range errTests {
		_, err := ParseSortSpecs(decode(t, tt.sort), "")
		if err == nil {
			t.Errorf("ParseSortSpecs(%s) succeeded", tt.sort)
			continue
		}
		if err.Type != tt.typ || err.Status != 400 || (tt.reason != "" && err.Reason != tt.reason) {
			t.Errorf("ParseSortSpecs(%s) = %s %d %q, want %s 400 %q", tt.sort, err.Type, err.Status, err.Reason, tt.typ, tt.reason)
		}
		if tt.cause != "" && (err.CausedBy == nil || err.CausedBy.Type != "illegal_argument_exception" || err.CausedBy.Reason != tt.cause) {
			t.Errorf("ParseSortSpecs(%s) caused_by = %+v, want illegal_argument_exception %q", tt.sort, err.CausedBy, tt.cause)
		}
	}

	okTests := []struct {
		sort  string
		uri   string
		field []string
		desc  []bool
	}{
		{`"n"`, "", []string{"n"}, []bool{false}},
		{`[{"n":"desc"},"_score",{"tag":{"order":"ASC"}}]`, "", []string{"n", "_score", "tag"}, []bool{true, false, false}},
		{`null`, "n:desc,tag", []string{"n", "tag"}, []bool{true, false}},
		{`[""]`, "", nil, nil},
	}
	for _, tt := range okTests {
		specs, err := ParseSortSpecs(decode(t, tt.sort), tt.uri)
		if err != nil {
			t.Errorf("ParseSortSpecs(%s, %q): %v", tt.sort, tt.uri, err)
			continue
		}
		var fields []string
		var desc []bool
		for _, sp := range specs {
			fields = append(fields, sp.Field)
			desc = append(desc, sp.Desc)
		}
		if !reflect.DeepEqual(fields, tt.field) || !reflect.DeepEqual(desc, tt.desc) {
			t.Errorf("ParseSortSpecs(%s, %q) = %v %v, want %v %v", tt.sort, tt.uri, fields, desc, tt.field, tt.desc)
		}
	}
}

func TestWildcards(t *testing.T) {
	tests := []struct {
		pattern, s string
		want       bool
	}{
		{"*", "", true},
		{"*", "anything", true},
		{"t*", "t", true},
		{"t*", "title", true},
		{"t*", "at", false},
		{"o?j", "obj", true},
		{"o?j", "oj", false},
		{"*.b", "obj.b", true},
		{"a*b*c", "aXbYc", true},
		{"a*b*c", "aXbY", false},
		{"O", "o", false},
	}
	for _, tt := range tests {
		if got := WildcardMatch(tt.pattern, tt.s); got != tt.want {
			t.Errorf("WildcardMatch(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.want)
		}
		if got := WildcardRegex(tt.pattern).MatchString(tt.s); got != tt.want {
			t.Errorf("WildcardRegex(%q) matches %q = %v, want %v", tt.pattern, tt.s, got, tt.want)
		}
	}
	// regex metacharacters are literal, a backslash escapes * and ?
	regexTests := []struct {
		pattern, s string
		want       bool
	}{
		{"a.c", "abc", false},
		{"a.c", "a.c", true},
		{`a\*`, "a*", true},
		{`a\*`, "ab", false},
		{`a\?`, "a?", true},
		{"(x)+", "(x)+", true},
	}
	for _, tt := range regexTests {
		if got := WildcardRegex(tt.pattern).MatchString(tt.s); got != tt.want {
			t.Errorf("WildcardRegex(%q) matches %q = %v, want %v", tt.pattern, tt.s, got, tt.want)
		}
	}
}

func TestLevenshteinAndFuzziness(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"quick", "quick", 0},
		{"quikc", "quick", 2}, // a transposition is two edits
		{"kitten", "sitting", 3},
		{"brwn", "brown", 1},
		{"café", "cafe", 1}, // runes, not bytes
	}
	for _, tt := range tests {
		if got := Levenshtein(tt.a, tt.b); got != tt.want {
			t.Errorf("Levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
	// AUTO fuzziness: 0 edits for 1-2 characters, 1 for 3-5, 2 above.
	fuzz := []struct {
		term string
		want int
	}{{"", 0}, {"ab", 0}, {"abc", 1}, {"abcde", 1}, {"abcdef", 2}, {"éé", 0}, {"ééé", 1}}
	for _, tt := range fuzz {
		if got := autoFuzziness(tt.term); got != tt.want {
			t.Errorf("autoFuzziness(%q) = %d, want %d", tt.term, got, tt.want)
		}
	}
}

func TestScoreJSON(t *testing.T) {
	tests := []struct {
		v    float64
		want string
	}{
		{1, "1.0"},
		{0, "0.0"},
		// scores are floats: a float32 value prints in its shortest float form
		{float64(float32(54.81589)), "54.81589"},
		{0.5, "0.5"},
	}
	for _, tt := range tests {
		b, err := json.Marshal(ScoreJSON(tt.v))
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != tt.want {
			t.Errorf("ScoreJSON(%v) = %s, want %s", tt.v, b, tt.want)
		}
	}
}
