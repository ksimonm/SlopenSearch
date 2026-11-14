package mapping

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/model"
	"slopensearch/internal/settings"
)

// IndexKnnEnabled reports whether the index has index.knn = true.
func IndexKnnEnabled(idx *model.Index) bool {
	if idx == nil {
		return false
	}
	v, ok := settings.Find(idx.Settings, "knn")
	return ok && settings.ValueString(v) == "true"
}

func knnValueError(c *ParseContext, path string, preview, causeType, causeReason string) *apierr.Error {
	reason := fmt.Sprintf("failed to parse field [%s] of type [knn_vector] in document with id '%s'. Preview of field's value: '%s'", path, c.docID, preview)
	return mapperErr("mapper_parsing_exception", reason, causeType, causeReason)
}

// javaFloatString renders a float32 like Java's Float.toString.
func javaFloatString(f float32) string {
	if math.IsInf(float64(f), 1) {
		return "Infinity"
	}
	if math.IsInf(float64(f), -1) {
		return "-Infinity"
	}
	if f != f {
		return "NaN"
	}
	a := math.Abs(float64(f))
	if a != 0 && (a < 1e-3 || a >= 1e7) {
		s := strconv.FormatFloat(float64(f), 'E', -1, 32)
		mant, exp, _ := strings.Cut(s, "E")
		if !strings.Contains(mant, ".") {
			mant += ".0"
		}
		e, _ := strconv.Atoi(exp)
		return mant + "E" + strconv.Itoa(e)
	}
	s := strconv.FormatFloat(float64(f), 'f', -1, 32)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

func knnTokenText(v any) string {
	switch t := v.(type) {
	case json.Number:
		return t.String()
	case string:
		return t
	case float64:
		return jsonv.FormatNumber(t)
	}
	return fmt.Sprintf("%v", v)
}

// parseKnnVector validates a knn_vector value against its mapping and returns
// the stored vector values (floats, or signed bytes for byte/binary).
func parseKnnVector(c *ParseContext, path string, def map[string]any, raw any) ([]float64, *apierr.Error) {
	fi := KnnInfo(def)
	dataType := fi.DataType
	expected := fi.Dim
	if dataType == "binary" {
		expected = fi.Dim / 8
	}
	var elems []any
	switch t := raw.(type) {
	case []any:
		elems = t
	case map[string]any:
		return nil, knnValueError(c, path, javaMapDebug(t), "illegal_argument_exception",
			fmt.Sprintf("Vector dimension mismatch. Expected: %d, Given: 0", fi.Dim))
	case string:
		s := strings.TrimRight(t, "=")
		b, err := base64.RawStdEncoding.DecodeString(s)
		if err != nil {
			b, _ = base64.RawStdEncoding.DecodeString(s[:len(s)-len(s)%4])
		}
		if len(b)%4 != 0 {
			return nil, knnValueError(c, path, t, "illegal_argument_exception",
				fmt.Sprintf("Base64 encoded vector for field [%s] has invalid byte length [%d], must be a multiple of 4 (float size)", path, len(b)))
		}
		for i := 0; i+4 <= len(b); i += 4 {
			elems = append(elems, float64(math.Float32frombits(binary.LittleEndian.Uint32(b[i:]))))
		}
	default:
		elems = []any{raw}
	}
	out := make([]float64, 0, len(elems))
	for _, e := range elems {
		var f float64
		switch t := e.(type) {
		case nil:
			return nil, knnValueError(c, path, "null", "input_coercion_exception",
				"Current token (VALUE_NULL) not numeric, cannot use numeric value accessors")
		case []any:
			return nil, knnValueError(c, path, javaMapDebug(t), "input_coercion_exception",
				"Current token (START_ARRAY) not numeric, cannot use numeric value accessors")
		case map[string]any:
			return nil, knnValueError(c, path, "null", "input_coercion_exception",
				"Current token (START_OBJECT) not numeric, cannot use numeric value accessors")
		case bool:
			return nil, knnValueError(c, path, strconv.FormatBool(t), "input_coercion_exception",
				"Current token (VALUE_"+strings.ToUpper(strconv.FormatBool(t))+") not numeric, cannot use numeric value accessors")
		case string:
			v, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
			if err != nil {
				return nil, knnValueError(c, path, t, "number_format_exception", `For input string: "`+t+`"`)
			}
			f = v
		default:
			v, ok := jsonv.ToFloat(e)
			if !ok {
				return nil, knnValueError(c, path, knnTokenText(e), "illegal_argument_exception", "malformed")
			}
			f = v
		}
		switch dataType {
		case "byte", "binary":
			if f != math.Trunc(f) {
				return nil, knnValueError(c, path, knnTokenText(e), "illegal_argument_exception",
					fmt.Sprintf("[data_type] field was set as [%s] in index mapping. But, KNN vector values are floats instead of byte integers", dataType))
			}
			if f < -128 || f > 127 {
				return nil, knnValueError(c, path, knnTokenText(e), "illegal_argument_exception",
					fmt.Sprintf("[data_type] field was set as [%s] in index mapping. But, KNN vector values are not within in the byte range [-128, 127]", dataType))
			}
		default:
			f32 := float32(f)
			if math.IsInf(float64(f32), 0) {
				return nil, knnValueError(c, path, javaFloatString(f32), "illegal_argument_exception",
					"KNN vector values cannot be infinity")
			}
			if f32 != f32 {
				return nil, knnValueError(c, path, "NaN", "illegal_argument_exception",
					"KNN vector values cannot be NaN")
			}
			f = float64(f32)
		}
		out = append(out, f)
	}
	if len(out) != expected {
		if dataType == "binary" {
			return nil, knnValueError(c, path, "null", "illegal_argument_exception",
				fmt.Sprintf("The dimension of the binary vector must be 8 times the length of the provided vector. Expected: %d, Given: %d", fi.Dim, len(out)*8))
		}
		return nil, knnValueError(c, path, "null", "illegal_argument_exception",
			fmt.Sprintf("Vector dimension mismatch. Expected: %d, Given: %d", fi.Dim, len(out)))
	}
	if fi.Space == "cosinesimil" && dataType != "binary" && IndexKnnEnabled(c.Idx) {
		zero := true
		for _, f := range out {
			if f != 0 {
				zero = false
				break
			}
		}
		if zero {
			return nil, knnValueError(c, path, "null", "illegal_argument_exception",
				"zero vector is not supported when space type is [cosinesimil]")
		}
	}
	return out, nil
}

// indexKnnVector stores a knn_vector field value.
func (c *ParseContext) indexKnnVector(path string, def map[string]any, raw any) *apierr.Error {
	if raw == nil {
		return nil
	}
	vec, err := parseKnnVector(c, path, def, raw)
	if err != nil {
		return err
	}
	vals := make([]any, len(vec))
	for i, f := range vec {
		vals[i] = f
	}
	c.Values[path] = append(c.Values[path], vals...)
	return nil
}

// KnnDeriveSource rewrites knn_vector values of a document the way an
// index.knn index returns them from its derived _source: as stored floats
// (or integers for byte/binary vectors). It reports whether anything changed.
func KnnDeriveSource(idx *model.Index, doc map[string]any) bool {
	if !IndexKnnEnabled(idx) {
		return false
	}
	props, _ := idx.Mappings["properties"].(map[string]any)
	return knnDeriveObject(idx, props, doc)
}

func knnDeriveObject(idx *model.Index, props map[string]any, doc map[string]any) bool {
	changed := false
	for k, v := range doc {
		def, ok := props[k].(map[string]any)
		if !ok {
			continue
		}
		if t, _ := def["type"].(string); t == "knn_vector" {
			if v == nil {
				continue
			}
			c := &ParseContext{Idx: idx, Values: map[string][]any{}}
			vec, err := parseKnnVector(c, k, def, v)
			if err != nil {
				continue
			}
			dt := KnnInfo(def).DataType
			out := make([]any, len(vec))
			for i, f := range vec {
				if dt == "byte" || dt == "binary" {
					out[i] = json.Number(strconv.FormatInt(int64(f), 10))
				} else {
					out[i] = json.Number(javaFloatString(float32(f)))
				}
			}
			doc[k] = out
			changed = true
			continue
		}
		sub, ok := def["properties"].(map[string]any)
		if !ok {
			continue
		}
		switch t := v.(type) {
		case map[string]any:
			if knnDeriveObject(idx, sub, t) {
				changed = true
			}
		case []any:
			for _, e := range t {
				if m, ok := e.(map[string]any); ok && knnDeriveObject(idx, sub, m) {
					changed = true
				}
			}
		}
	}
	return changed
}
