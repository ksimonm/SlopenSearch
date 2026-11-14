package jsonv

import (
	"encoding/json"
	"strconv"
)

// ScalarString renders a JSON scalar (numbers in Java style).
func ScalarString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case float64:
		return FormatNumber(t), true
	case json.Number:
		return t.String(), true
	case bool:
		return strconv.FormatBool(t), true
	}
	return "", false
}

// ToString renders a JSON scalar or int64 (numbers in plain decimal form).
func ToString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	case json.Number:
		return t.String(), true
	case int64:
		return strconv.FormatInt(t, 10), true
	case bool:
		return strconv.FormatBool(t), true
	}
	return "", false
}
