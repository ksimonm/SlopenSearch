// Package jsonv has helpers for decoded JSON values.
package jsonv

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// FormatNumber renders a number like OpenSearch renders whole doubles (no
// fraction) and other doubles (shortest form).
func FormatNumber(f float64) string {
	if f == float64(int64(f)) && f < 1e15 && f > -1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

var jsonStringEscaper = strings.NewReplacer(
	"\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r", "\t", "\\t",
	"\b", "\\b", "\f", "\\f",
)

// EncodeString quotes s as a JSON string (Jackson escaping).
func EncodeString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' && r != '\b' && r != '\f' {
			sb.WriteString(fmt.Sprintf("\\u%04x", r))
		} else {
			sb.WriteString(jsonStringEscaper.Replace(string(r)))
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// SortStrings sorts a in place (insertion sort, stable).
func SortStrings(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

// DeepCopy copies maps and slices of a decoded JSON value.
func DeepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[k] = DeepCopy(val)
		}
		return m
	case []any:
		a := make([]any, len(t))
		for i, e := range t {
			a[i] = DeepCopy(e)
		}
		return a
	default:
		return v
	}
}

// SortedKeys returns the keys of m in sorted order.
func SortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	SortStrings(keys)
	return keys
}

// Unmarshal decodes JSON with numbers as json.Number.
func Unmarshal(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return dec.Decode(v)
}

// ToInt64 returns an integral JSON value (int, int64, uint64 within range,
// integral json.Number) as an int64.
func ToInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case int64:
		return t, true
	case int:
		return int64(t), true
	case uint64:
		if t <= 1<<63-1 {
			return int64(t), true
		}
		return 0, false
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n, true
		}
	}
	return 0, false
}

// Int reads body[key] as a number truncated to an int, or def when it is
// missing or not numeric.
func Int(body map[string]any, key string, def int) int {
	if v, ok := body[key]; ok {
		if f, ok := ToFloat(v); ok {
			return int(f)
		}
	}
	return def
}

// NormalizeNumbers replaces, in place, json.Numbers written with a fraction or
// an exponent by Float values, so they render as doubles.
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
			return Float(f)
		}
		return t
	}
	return v
}
