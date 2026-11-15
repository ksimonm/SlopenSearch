package search

import (
	"encoding/json"
	"math"

	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
)

// Numbers in responses are rendered the way Jackson writes the Java type
// OpenSearch holds them in: doubles with Double.toString (1.0E7), floats
// with Float.toString. These helpers wrap float64 values accordingly.

// JavaDoubles wraps every finite float64 in v (lists and maps included) as a
// Java double. Script results hold doubles as float64 (integers are ints).
func JavaDoubles(v any) any {
	switch t := v.(type) {
	case float64:
		if math.IsInf(t, 0) || math.IsNaN(t) {
			return t
		}
		return jsonv.Float(t)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = JavaDoubles(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = JavaDoubles(e)
		}
		return out
	}
	return v
}

// JavaFieldValues renders the values of a float, half_float, double or
// scaled_float field for docvalue_fields (doc values read as doubles) or for
// the fields API (fromSource: the _source values parsed by the field type,
// so a half_float or scaled_float keeps the precision it was sent with).
func JavaFieldValues(doc map[string]any, field, ftype string, docValues []any, fromSource bool) []any {
	switch ftype {
	case "float", "half_float", "double", "scaled_float":
	default:
		return docValues
	}
	if fromSource {
		if raw, ok := mapping.FieldValue(doc, field); ok {
			var src []any
			var flat func(v any)
			flat = func(v any) {
				if l, ok := v.([]any); ok {
					for _, e := range l {
						flat(e)
					}
				} else if v != nil {
					src = append(src, v)
				}
			}
			flat(raw)
			out := make([]any, 0, len(src))
			for _, v := range src {
				f, ok := jsonv.ToFloat(v)
				if !ok {
					return docValues
				}
				if ftype == "scaled_float" {
					ftype = "double"
				}
				out = append(out, JavaFieldNumber(ftype, f, false))
			}
			if len(out) == len(docValues) {
				return out
			}
		}
	}
	out := make([]any, len(docValues))
	for i, v := range docValues {
		out[i] = JavaFieldNumber(ftype, v, true)
	}
	return out
}

// JavaFieldNumber wraps a float64 value of a field of type ftype as the
// Java type its doc values hold: float and half_float values are floats,
// double and scaled_float values doubles. asDouble widens floats (doc
// values fields and numeric_type double sorts read them as doubles).
func JavaFieldNumber(ftype string, v any, asDouble bool) any {
	var f float64
	switch t := v.(type) {
	case float64:
		f = t
	case jsonv.Float:
		f = float64(t)
	case int64:
		f = float64(t)
	case json.Number:
		var err error
		if f, err = t.Float64(); err != nil {
			return v
		}
	default:
		return v
	}
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return v
	}
	switch ftype {
	case "float", "half_float":
		if asDouble {
			return jsonv.Float(f)
		}
		return jsonv.Float32(float32(f))
	case "double", "scaled_float":
		return jsonv.Float(f)
	}
	return v
}
