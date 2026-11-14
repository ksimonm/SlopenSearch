// Package jsonv has helpers for decoded JSON values.
package jsonv

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Float is a double that always renders with a fraction (1.0, not 1), like
// Jackson writes Java doubles.
type Float float64

func (f Float) MarshalJSON() ([]byte, error) {
	s := strconv.FormatFloat(float64(f), 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return []byte(s), nil
}

// ToFloat converts a JSON value (or a Go number, bool or numeric string) to a
// float64.
func ToFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case Float:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case uint64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	case nil:
		return 0, false
	}
	return 0, false
}
