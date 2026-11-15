package mustache

import (
	"testing"

	"slopensearch/internal/jsonv"
)

// Expected values are the "template_output" texts OpenSearch 3.9 returns from
// _render/template for the same sources and params, taken from
// tests/reference/test_43_scripting.jsonl (test_render_template_string_source,
// test_render_template_unclosed_section) and test_33_search_api.jsonl
// (test_render_template_functions). Params are decoded like request bodies
// (numbers as json.Number).

func params(t *testing.T, s string) map[string]any {
	t.Helper()
	m := map[string]any{}
	if err := jsonv.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("bad params %s: %v", s, err)
	}
	return m
}

func TestRender(t *testing.T) {
	tests := []struct{ tpl, params, want string }{
		// variables are JSON-escaped, not HTML-escaped
		{`{"x":"{{q}}"}`, `{"q":"hello"}`, `{"x":"hello"}`},
		{`{"x":"{{q}}"}`, `{}`, `{"x":""}`},
		{`{"x":"{{q}}"}`, `{"q":"a\"b<c>&\\d"}`, `{"x":"a\"b<c>&\\d"}`},
		{`{"x":"{{{q}}}"}`, `{"q":"a<b>"}`, `{"x":"a<b>"}`},
		{`{"x":{{n}}}`, `{"n":5}`, `{"x":5}`},
		{`{"x":{{n}}}`, `{"n":5.5}`, `{"x":5.5}`},
		{`{"x":{{n}}}`, `{"n":true}`, `{"x":true}`},
		{`{"x":"{{n}}"}`, `{"n":null}`, `{"x":""}`},
		// lists and maps print like Java maps
		{`{"x":"{{l}}"}`, `{"l":[1,2]}`, `{"x":"{0=1, 1=2}"}`},
		{`{"x":"{{m}}"}`, `{"m":{"a":1}}`, `{"x":"{a=1}"}`},
		// dotted names, list indexes
		{`{"x":"{{m.a}}"}`, `{"m":{"a":1}}`, `{"x":"1"}`},
		{`{"x":"{{a.b.c}}"}`, `{"a":{"b":{"c":"deep"}}}`, `{"x":"deep"}`},
		{`{"x":"{{l.0}}"}`, `{"l":["first","second"]}`, `{"x":"first"}`},
		// toJson
		{`{"t":{{#toJson}}tags{{/toJson}}}`, `{"tags":["a","b"]}`, `{"t":["a","b"]}`},
		{`{"t":{{#toJson}}l{{/toJson}}}`, `{"l":[1,2.5,true,null,"s"]}`, `{"t":[1,2.5,true,null,"s"]}`},
		{`{"x":"{{#toJson}}m{{/toJson}}"}`, `{"m":{"a":"b"}}`, `{"x":"{"a":"b"}"}`},
		{`{"x": {{#toJson}}missing{{/toJson}}}`, `{}`, `{"x": }`},
		// join
		{`{"x":"{{#join}}tags{{/join}}"}`, `{"tags":["a","b","c"]}`, `{"x":"a,b,c"}`},
		{`{"x":"{{#join}}l{{/join}}"}`, `{"l":[1,2.5,true]}`, `{"x":"1,2.5,true"}`},
		{`{"x":"{{#join delimiter='|'}}tags{{/join delimiter='|'}}"}`, `{"tags":["a","b","c"]}`, `{"x":"a|b|c"}`},
		// url
		{`{"x":"{{#url}}{{q}}{{/url}}"}`, `{"q":"a b&c=d/é"}`, `{"x":"a+b%26c%3Dd%2F%C3%A9"}`},
		{`{"x":"{{#url}}{{l}}{{/url}}"}`, `{"l":["a b","c"]}`, `{"x":"%7B0%3Da+b%2C+1%3Dc%7D"}`},
		{`{"q":"{{x}}","l":{{#toJson}}l{{/toJson}},"j":"{{#join}}l{{/join}}","u":"{{#url}}{{x}}{{/url}}"}`,
			`{"x":"a b&c","l":["p","q"]}`, `{"q":"a b&c","l":["p","q"],"j":"p,q","u":"a+b%26c"}`},
		// sections and inverted sections; 0 is truthy, "", false and [] are not
		{`{"from":{{from}}{{^from}}0{{/from}},"size":{{size}}{{^size}}10{{/size}}}`, `{"size":3}`, `{"from":0,"size":3}`},
		{`{"q":"x"{{#f}},"filter":"{{f}}"{{/f}}}`, `{"f":"yes"}`, `{"q":"x","filter":"yes"}`},
		{`{"q":"x"{{#f}},"filter":"{{f}}"{{/f}}}`, `{}`, `{"q":"x"}`},
		{`{"q":"x"{{#f}},"filter":"y"{{/f}}}`, `{"f":false}`, `{"q":"x"}`},
		{`{"q":"x"{{#f}},"filter":"y"{{/f}}}`, `{"f":[]}`, `{"q":"x"}`},
		{`{"q":"x"{{#f}},"filter":"y"{{/f}}}`, `{"f":""}`, `{"q":"x"}`},
		{`{"q":"x"{{#f}},"filter":"y"{{/f}}}`, `{"f":0}`, `{"q":"x","filter":"y"}`},
		{`{"items":[{{#l}}"{{.}}",{{/l}}"end"]}`, `{"l":["a","b"]}`, `{"items":["a","b","end"]}`},
		{`{"items":[{{#l}}{"n":"{{name}}"},{{/l}}{}]}`, `{"l":[{"name":"a"},{"name":"b"}]}`, `{"items":[{"n":"a"},{"n":"b"},{}]}`},
		// -last is not supported by mustache.java: the inverted section always renders
		{`{"x":"{{#l}}{{.}}{{^-last}},{{/-last}}{{/l}}"}`, `{"l":["a","b"]}`, `{"x":"a,b,"}`},
		// context stack lookup
		{`{"x":"{{#a}}{{b}}{{/a}}"}`, `{"a":{"b":"inner"}}`, `{"x":"inner"}`},
		{`{"x":"{{#a}}{{b}}{{/a}}"}`, `{"a":{"c":"inner"},"b":"outer"}`, `{"x":"outer"}`},
		// comments, delimiter changes, unknown sections, non-JSON output
		{`{"x":"{{! comment }}y"}`, `{}`, `{"x":"y"}`},
		{`{"x":"{{=<% %>=}}<%q%>"}`, `{"q":"swapped"}`, `{"x":"swapped"}`},
		{`{"x":"{{#upper}}a{{/upper}}"}`, `{}`, `{"x":""}`},
		{`{"size": {{s}} }`, `{"s":"abc"}`, `{"size": abc }`},
		{`not json {{q}}`, `{"q":1}`, `not json 1`},
	}
	for _, tt := range tests {
		got, err := Render(tt.tpl, params(t, tt.params))
		if err != nil {
			t.Errorf("Render(%s, %s): %v", tt.tpl, tt.params, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Render(%s, %s) = %s, want %s", tt.tpl, tt.params, got, tt.want)
		}
	}
}

func TestRenderErrors(t *testing.T) {
	tests := []struct{ tpl, reason string }{
		{`{"x":"{{#unclosed}}"}`, "Failed to close 'unclosed' tag @[query-template:1]"},
		// Expected reasons from OpenSearch 3.9 (_render/template).
		{"{{q", "Improperly closed variable @[query-template:1]"},
		{"x\n\n{{q", "Improperly closed variable @[query-template:3]"},
		{"a\n{{q}\n\nb", "Improperly closed variable @[query-template:2]"},
		{"a\nb {{#x}}c", "Failed to close 'x' tag @[query-template:2]"},
		{"a\n{{#x}}\nb\nc", "Failed to close 'x' tag @[query-template:2]"},
		{"{{#a}}\n{{#b}}\n{{/b}}", "Failed to close 'a' tag @[query-template:1]"},
		{"{{#join delimiter=\",\"}}", "Failed to close 'join delimiter=\",\"' tag @[query-template:1]"},
		{"{{#x}}\n\n{{/y}}", "Mismatched start/end tags: x != y in query-template:3 @[query-template:3]"},
		{"{{q\n}}\n{{/y}}", "Mismatched start/end tags: null != y in query-template:2 @[query-template:2]"},
		{"{{^a}}{{/b}}", "Mismatched start/end tags: a != b in query-template:1 @[query-template:1]"},
		{"{{}}", "Empty mustache @[query-template:1]"},
		{"{{{q}}", "Improperly closed variable: q in query-template:1@6 @[query-template:1]"},
		{"{{{q", "Improperly closed variable: q in query-template:1@4 @[query-template:1]"},
		{"{{{q}x", "Improperly closed variable: q}x in query-template:1@5 @[query-template:1]"},
		{"{{{q}xyz", "Improperly closed variable: q}xyz in query-template:1@7 @[query-template:1]"},
		{"a\nbc{{{q}}", "Improperly closed variable: q in query-template:2@8 @[query-template:2]"},
		{"x{{q}}{{{q}}", "Improperly closed variable: q in query-template:1@11 @[query-template:1]"},
		{"\u00e9\U0001F600{{{q}}", "Improperly closed variable: q in query-template:1@9 @[query-template:1]"},
		{"a{{#a}}b\n\nc{{{q}}", "Improperly closed variable: q in query-template:3@7 @[query-template:3]"},
		{"a{{#a}}b{{/a}}{{{q}}", "Improperly closed variable: q in query-template:1@12 @[query-template:1]"},
		{"{{>x}}", "Partial templates are not supported in OpenSearch Mustache scripts: [x]"},
	}
	for _, tt := range tests {
		_, err := Render(tt.tpl, map[string]any{"q": 1})
		if err == nil || err.Reason != tt.reason {
			t.Errorf("Render(%s) error = %v, want %q", tt.tpl, err, tt.reason)
		}
	}
}

func TestTemplateText(t *testing.T) {
	// Object sources are serialized before rendering
	// (test_render_template_object_source).
	src := params(t, `{"query":{"match":{"title":"{{q}}"}}}`)
	text, ok := TemplateText(src)
	if !ok {
		t.Fatal("TemplateText(object) not ok")
	}
	got, err := Render(text, params(t, `{"q":"hello"}`))
	if err != nil || got != `{"query":{"match":{"title":"hello"}}}` {
		t.Errorf("rendered object source = %s, %v", got, err)
	}
	if text, ok := TemplateText(`{"a":1}`); !ok || text != `{"a":1}` {
		t.Errorf("TemplateText(string) = %q, %v", text, ok)
	}
	if _, ok := TemplateText(5); ok {
		t.Error("TemplateText(number) ok")
	}
}
