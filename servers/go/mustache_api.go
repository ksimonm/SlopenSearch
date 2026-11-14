package main

import (
	"slopensearch/internal/apierr"
	"slopensearch/internal/mustache"
	"slopensearch/internal/ojson"
	"slopensearch/internal/search"
)

// mustacheScriptError is the script_exception of a failed template.
func mustacheScriptError(src string, e *mustache.Error) *apierr.Error {
	body := ojson.Object{{"error", ojson.Object{
		{"root_cause", []any{ojson.Object{{"type", "script_exception"}, {"reason", e.Reason}, {"script_stack", []any{}}, {"script", src}, {"lang", "mustache"}}}},
		{"type", "script_exception"}, {"reason", e.Reason}, {"script_stack", []any{}}, {"script", src}, {"lang", "mustache"},
		{"caused_by", ojson.Object{{"type", "mustache_exception"}, {"reason", e.Reason}}},
	}}, {"status", 400}}
	return &apierr.Error{Type: "script_exception", Status: 400, Reason: e.Reason, Fields: map[string]any{"lang": "mustache"},
		CausedBy: &apierr.Error{Type: "mustache_exception", Reason: e.Reason},
		Shard:    &search.ShardFailure{Raw: body, Status: 400}}
}
