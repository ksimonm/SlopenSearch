package main

import (
	"strconv"

	"slopensearch/internal/jsonv"
)

// Script query and script_score query evaluation. The scripting engines
// live in pl_*.go (Painless) and pl_expr.go (Lucene expressions).

func valueToScriptString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case float64:
		return jsonv.FormatNumber(t), true
	case int64:
		return strconv.FormatInt(t, 10), true
	case bool:
		return strconv.FormatBool(t), true
	case nil:
		return "null", true
	}
	return "", false
}
