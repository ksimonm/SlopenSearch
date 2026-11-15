package ingest

import (
	"testing"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
)

// Expected values are the rendered set values OpenSearch 3.9 returns from
// _ingest/pipeline/_simulate, taken from
// tests/reference/test_38_parity_ingest_geo.jsonl (test_set_template,
// test_list_lookup_*, test_simulate_template_compile_error).

const templateDoc = `{"a":"he said \"hi\" <b>&'\\ é\n","n":1,"f":1.5,"f0":1.0,"t":true,"z":null,
"obj":{"x":1,"y":"s"},"tags":["p","q"],"nums":[1,2.5],"nested":{"deep":{"v":"dv"}},
"empty":"","earr":[],"objs":[{"k":"a"},{"k":"b"}]}`

func templateEnv(t *testing.T) *ingestEnv {
	t.Helper()
	src := map[string]any{}
	if err := jsonv.Unmarshal([]byte(templateDoc), &src); err != nil {
		t.Fatal(err)
	}
	return &ingestEnv{source: src, meta: map[string]any{"_index": "ix", "_id": "id1"}, timestamp: "ts"}
}

func TestRenderTemplates(t *testing.T) {
	env := templateEnv(t)
	tests := []struct{ tpl, want string }{
		{"{{a}}", `he said \"hi\" <b>&'\\ é\n`},
		{"{{{a}}}", "he said \"hi\" <b>&'\\ é\n"},
		{"{{n}}|{{f}}|{{f0}}|{{t}}", "1|1.5|1.0|true"},
		{"[{{z}}][{{missing}}][{{n.x}}][{{tags.x}}]", "[][][][]"},
		{"{{obj}}", "{x=1, y=s}"},
		{"{{tags}} {{nums}}", "{0=p, 1=q} {0=1, 1=2.5}"},
		{"{{objs}}", "{0={k=a}, 1={k=b}}"},
		{"{{_source.nested.deep.v}} {{_index}}/{{_id}} [{{_version}}] [{{_ingest.nope}}]", "dv ix/id1 [] []"},
		{"{{#tags}}<{{.}}>{{/tags}}", "<p><q>"},
		{"{{^missing}}none{{/missing}}{{^tags}}none{{/tags}}", "none"},
		{"{{#earr}}x{{/earr}}{{^earr}}e{{/earr}}", "e"},
		{"{{#objs}}{{k}};{{/objs}}", "a;b;"},
		{"{{#toJson}}obj{{/toJson}} {{#toJson}}tags{{/toJson}}", `{"x":1,"y":"s"} ["p","q"]`},
		{"{{#join delimiter='|'}}tags{{/join delimiter='|'}}", "p|q"},
		{"{{=<% %>=}}<% n %>", "1"},
		{"{{#url}}{{a}}{{/url}}", "he+said+%5C%22hi%5C%22+%3Cb%3E%26%27%5C%5C+%C3%A9%5Cn"},
		{"x}} {", "x}} {"},
	}
	for _, tt := range tests {
		if got := env.render(tt.tpl); got != tt.want {
			t.Errorf("render(%q) = %q, want %q", tt.tpl, got, tt.want)
		}
	}
}

func TestRenderValueTemplatesKeys(t *testing.T) {
	env := &ingestEnv{source: map[string]any{"a": "va", "k": "kk"}, meta: map[string]any{}}
	got := env.renderValue(map[string]any{"{{k}}": "{{a}}", "l": []any{"{{a}}", 1}})
	m := got.(map[string]any)
	if m["kk"] != "va" || m["l"].([]any)[0] != "va" {
		t.Errorf("renderValue = %v", got)
	}
}

func TestRenderLookupFailure(t *testing.T) {
	env := templateEnv(t)
	for tpl, want := range map[string][3]string{
		"{{tags.5}}":  {"Failed to get value for tags.5 @[query-template:1]", "5 @[query-template:1]", "index_out_of_bounds_exception"},
		"{{tags.-1}}": {"Failed to get value for tags.-1 @[query-template:1]", "position >= 0 @[query-template:1]", "illegal_argument_exception"},
	} {
		err := func() (err *apierr.Error) {
			defer func() {
				if r := recover(); r != nil {
					err = r.(templateFailure).err
				}
			}()
			env.render(tpl)
			return nil
		}()
		if err == nil {
			t.Fatalf("%s: no error", tpl)
		}
		if err.Type != "mustache_exception" || err.Reason != want[0] || err.Status != 500 ||
			err.CausedBy.Reason != want[1] || err.CausedBy.CausedBy.Type != want[2] {
			t.Errorf("%s: %+v", tpl, err)
		}
	}
}

func TestCheckTemplates(t *testing.T) {
	err := checkTemplates("set", map[string]any{"tag": "t1", "field": "x", "value": "{{#a}}"})
	if err == nil || err.Type != "script_exception" || err.Reason != "Failed to close 'a' tag @[query-template:1]" ||
		err.Fields["processor_tag"] != "t1" || err.Fields["script"] != "{{#a}}" {
		t.Errorf("set value: %+v", err)
	}
	err = checkTemplates("fail", map[string]any{"message": "{{/a}}"})
	if err == nil || err.Fields["property_name"] != "message" ||
		err.Reason != "Mismatched start/end tags: null != a in query-template:1 @[query-template:1]" {
		t.Errorf("fail message: %+v", err)
	}
	if err := checkTemplates("set", map[string]any{"field": "x", "value": "x}} {"}); err != nil {
		t.Errorf("literal: %+v", err)
	}
}
