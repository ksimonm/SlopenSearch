package search

import (
	"encoding/json"

	"slopensearch/internal/apierr"
	"slopensearch/internal/mustache"
	"slopensearch/internal/ojson"
)

// MustacheError is the script_exception of a failed template.
func MustacheError(src string, e *mustache.Error) *apierr.Error {
	body := ojson.Object{{"error", ojson.Object{
		{"root_cause", []any{ojson.Object{{"type", "script_exception"}, {"reason", e.Reason}, {"script_stack", []any{}}, {"script", src}, {"lang", "mustache"}}}},
		{"type", "script_exception"}, {"reason", e.Reason}, {"script_stack", []any{}}, {"script", src}, {"lang", "mustache"},
		{"caused_by", ojson.Object{{"type", "mustache_exception"}, {"reason", e.Reason}}},
	}}, {"status", 400}}
	return &apierr.Error{Type: "script_exception", Status: 400, Reason: e.Reason, Fields: map[string]any{"lang": "mustache"},
		CausedBy: &apierr.Error{Type: "mustache_exception", Reason: e.Reason},
		Shard:    &ShardFailure{Raw: body, Status: 400}}
}

// TokenType is the XContent token name of a JSON value, as error messages
// report it.
func TokenType(v any) string {
	switch v.(type) {
	case nil:
		return "VALUE_NULL"
	case bool:
		return "VALUE_BOOLEAN"
	case json.Number, float64, int, int64:
		return "VALUE_NUMBER"
	case string:
		return "VALUE_STRING"
	case []any:
		return "START_ARRAY"
	case map[string]any:
		return "START_OBJECT"
	}
	return "VALUE_NULL"
}
