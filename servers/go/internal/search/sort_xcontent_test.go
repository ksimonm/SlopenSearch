package search

import "testing"

// Expected reasons from OpenSearch 3.9 (test_field_sort_parse_error and probes).
func TestCheckSortClauses(t *testing.T) {
	tests := []struct{ body, reason, cause string }{
		{`{"sort":[{"n":{"mode":"bogus"}}]}`, "[1:23] [field_sort] failed to parse field [mode]", "Unknown SortMode [bogus]"},
		{`{"sort":["k",{"n":{"order":"up"}}]}`, "[1:28] [field_sort] failed to parse field [order]",
			"No enum constant org.opensearch.search.sort.SortOrder.UP"},
		{"{\n  \"sort\": [\n    {\"n\": {\"mode\": \"bogus\"}}\n  ]\n}", "[3:20] [field_sort] failed to parse field [mode]",
			"Unknown SortMode [bogus]"},
		{`{"sort":[{"n":{"numeric_type":"x"}}]}`, "[1:31] [field_sort] failed to parse field [numeric_type]",
			"invalid value for [numeric_type], must be [long, double, date, date_nanos], got x"},
		{`{"sort":[{"n":{"foo":1}}]}`, "[1:16] [field_sort] unknown field [foo]", ""},
		{`{"sort":[{"n":{"format":"x"}}]}`, "[1:16] [field_sort] unknown field [format]", ""},
		{`{"sort":[{"n":{"missing":{}}}]}`, "[1:26] [field_sort] missing doesn't support values of type: START_OBJECT", ""},
		{`{"sort":[{"n":{"order":1}}]}`, "[1:24] [field_sort] order doesn't support values of type: VALUE_NUMBER", ""},
		{`{"sort":[{"n":{"order":null}}]}`, "[1:24] [field_sort] order doesn't support values of type: VALUE_NULL", ""},
		{`{"sort":[{"n":{"foo":1,"mode":"bogus"}}]}`, "[1:16] [field_sort] unknown field [foo]", ""},
		{`{"sort":{"n":{"mode":"bogus"}}}`, "[1:22] [field_sort] failed to parse field [mode]", "Unknown SortMode [bogus]"},
		{`{"sort":[{"_score":{"mode":"bogus"}}]}`, "[1:21] [_score] unknown field [mode]", ""},
		{`{"sort":[{"é":{"order":"up"}}]}`, "[1:25] [field_sort] failed to parse field [order]",
			"No enum constant org.opensearch.search.sort.SortOrder.UP"},
		{`{"sort":[{"n":{"mode":"MAX","order":"DESC","missing":null,"numeric_type":"LONG"}}]}`, "", ""},
		{`{"sort":[{"n":"up"}]}`, "", ""},
		{`{"sort":[{"_geo_distance":{"p":[0,0],"foo":1}}]}`, "", ""},
	}
	for _, tt := range tests {
		err := CheckSortClauses([]byte(tt.body))
		if tt.reason == "" {
			if err != nil {
				t.Errorf("CheckSortClauses(%s) = %v", tt.body, err)
			}
			continue
		}
		if err == nil || err.Type != "x_content_parse_exception" || err.Reason != tt.reason {
			t.Errorf("CheckSortClauses(%s) = %v, want %s", tt.body, err, tt.reason)
			continue
		}
		cause := ""
		if err.CausedBy != nil {
			cause = err.CausedBy.Reason
		}
		if cause != tt.cause {
			t.Errorf("CheckSortClauses(%s) cause = %q, want %q", tt.body, cause, tt.cause)
		}
	}
	if !SortParsedFirst([]byte(`{"size":1,"sort":[]}`)) || SortParsedFirst([]byte(`{"query":{},"sort":[]}`)) ||
		SortParsedFirst([]byte(`{"size":"x","sort":[]}`)) {
		t.Error("SortParsedFirst")
	}
}
