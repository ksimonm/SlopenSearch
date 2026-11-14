package search

import (
	"encoding/json"
	"strings"

	"slopensearch/internal/jsonv"
)

func NormalizeNumbers(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, e := range t {
			t[k] = NormalizeNumbers(e)
		}
		return t
	case []any:
		for i, e := range t {
			t[i] = NormalizeNumbers(e)
		}
		return t
	case json.Number:
		if strings.ContainsAny(t.String(), ".eE") {
			f, _ := t.Float64()
			return jsonv.Float(f)
		}
		return t
	}
	return v
}
