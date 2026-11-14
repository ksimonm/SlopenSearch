package search

import (
	"slopensearch/internal/painless"
)

// fieldScriptValues converts a script result into script_fields values.
func fieldScriptValues(v any) ([]any, bool) {
	switch t := v.(type) {
	case nil:
		return []any{nil}, true
	case *painless.List, *painless.DocValues, *painless.Array, *painless.Set:
		vals := painless.ToJSON(t).([]any)
		if len(vals) == 0 {
			return nil, false
		}
		return vals, true
	}
	return []any{painless.ToJSON(v)}, true
}
