package ojson

import (
	"encoding/json"
	"testing"
)

func TestObjectMarshal(t *testing.T) {
	tests := []struct {
		name string
		obj  Object
		want string
	}{
		{"empty", Object{}, `{}`},
		{"nil", nil, `{}`},
		// members keep their order (OpenSearch writes "took" before "timed_out")
		{"order", Object{{"took", 2}, {"timed_out", false}, {"_shards", Object{{"total", 1}}}},
			`{"took":2,"timed_out":false,"_shards":{"total":1}}`},
		{"reverse alphabetical", Object{{"z", 1}, {"a", 2}, {"m", 3}}, `{"z":1,"a":2,"m":3}`},
		{"duplicate keys kept", Object{{"a", 1}, {"a", 2}}, `{"a":1,"a":2}`},
		{"verbatim", Object{{"v", Verbatim(`{"b":1,"a":2.0}`)}}, `{"v":{"b":1,"a":2.0}}`},
		{"raw message", Object{{"v", json.RawMessage(`[1.0,2]`)}}, `{"v":[1.0,2]}`},
		{"json.Number", Object{{"n", json.Number("1.50")}}, `{"n":1.50}`},
		{"null", Object{{"n", nil}}, `{"n":null}`},
		// no HTML escaping, in keys or values
		{"html", Object{{"<k>", "a<b>&c"}}, `{"<k>":"a<b>&c"}`},
		{"nested slice", Object{{"hits", []any{Object{{"_id", "1"}, {"_index", "i"}}}}},
			`{"hits":[{"_id":"1","_index":"i"}]}`},
	}
	for _, tt := range tests {
		b, err := tt.obj.MarshalJSON()
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if string(b) != tt.want {
			t.Errorf("%s: got %s, want %s", tt.name, b, tt.want)
		}
		if got := string(Marshal(tt.obj)); got != tt.want {
			t.Errorf("%s: Marshal got %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestMarshal(t *testing.T) {
	tests := []struct {
		v    any
		want string
	}{
		{"a<b>&c", `"a<b>&c"`},
		{map[string]any{"b": 1, "a": 2}, `{"a":2,"b":1}`},
		{[]any{1, "x", nil}, `[1,"x",null]`},
		{Verbatim(`1.0`), `1.0`},
		// errors yield whatever was encoded (nothing here)
		{func() {}, ``},
	}
	for _, tt := range tests {
		if got := string(Marshal(tt.v)); got != tt.want {
			t.Errorf("Marshal(%T) = %s, want %s", tt.v, got, tt.want)
		}
	}
}

func TestIndex(t *testing.T) {
	o := Object{{"a", 1}, {"b", 2}, {"a", 3}}
	tests := []struct {
		key  string
		want int
	}{{"a", 0}, {"b", 1}, {"c", -1}, {"", -1}}
	for _, tt := range tests {
		if got := o.Index(tt.key); got != tt.want {
			t.Errorf("Index(%q) = %d, want %d", tt.key, got, tt.want)
		}
	}
}
