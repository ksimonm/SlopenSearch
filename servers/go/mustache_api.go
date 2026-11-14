package main

import "slopensearch/internal/mustache"

// mustacheScriptError is the script_exception of a failed template.
func mustacheScriptError(src string, e *mustache.Error) *apiError {
	body := orderedJSON{{"error", orderedJSON{
		{"root_cause", []any{orderedJSON{{"type", "script_exception"}, {"reason", e.Reason}, {"script_stack", []any{}}, {"script", src}, {"lang", "mustache"}}}},
		{"type", "script_exception"}, {"reason", e.Reason}, {"script_stack", []any{}}, {"script", src}, {"lang", "mustache"},
		{"caused_by", orderedJSON{{"type", "mustache_exception"}, {"reason", e.Reason}}},
	}}, {"status", 400}}
	return &apiError{Type: "script_exception", Status: 400, Reason: e.Reason, Fields: map[string]any{"lang": "mustache"},
		CausedBy: &apiError{Type: "mustache_exception", Reason: e.Reason},
		Shard:    &shardFailure{raw: body, status: 400}}
}
