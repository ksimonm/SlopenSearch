package mapping

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
)

// ---------------------------------------------------------------------------
// knn_vector mappings (index.knn = true)
// ---------------------------------------------------------------------------

var knnSpaceTypes = []string{"l2", "cosinesimil", "l1", "linf", "innerproduct", "hamming"}

func knnValidation(msg string) *apierr.Error {
	r := "Validation Failed: 1: " + msg + ";"
	return wrappedError(r, "validation_exception")
}

// KnnIndexMappings validates and normalizes knn_vector fields of an index
// that has index.knn enabled.
func KnnIndexMappings(props map[string]any) *apierr.Error {
	for _, name := range SortedAnyKeys(props) {
		def, ok := props[name].(map[string]any)
		if !ok {
			continue
		}
		if sub, ok := def["properties"].(map[string]any); ok {
			if err := KnnIndexMappings(sub); err != nil {
				return err
			}
		}
		if t, _ := def["type"].(string); t != "knn_vector" {
			continue
		}
		if err := knnValidateField(name, def); err != nil {
			return err
		}
	}
	return nil
}

func knnValidateField(name string, def map[string]any) *apierr.Error {
	for _, k := range SortedAnyKeys(def) {
		if !knnFieldParams[k] {
			return wrappedError(fmt.Sprintf("unknown parameter [%s] on mapper [%s] of type [knn_vector]", k, name), "mapper_parsing_exception")
		}
	}
	var dim int64
	switch d := def["dimension"].(type) {
	case float64:
		dim = int64(d)
	case int64:
		dim = d
	case int:
		dim = int64(d)
	case string:
		n, err := strconv.ParseInt(d, 10, 64)
		if err != nil {
			return wrappedError(fmt.Sprintf("Unable to parse [dimension] from provided value [%s] for vector [%s]", d, name), "illegal_argument_exception")
		}
		dim = n
	default:
		if f, ok := jsonv.ToFloat(d); ok {
			dim = int64(f)
		}
	}
	if dim <= 0 {
		return wrappedError("Dimension value must be greater than 0 for vector: "+name, "illegal_argument_exception")
	}
	if m, ok := def["mode"]; ok {
		s, _ := m.(string)
		if s != "in_memory" && s != "on_disk" {
			return wrappedError(fmt.Sprintf("Unknown value [%v] for field [mode] - accepted values are [null, in_memory, on_disk]", m), "mapper_parsing_exception")
		}
	}
	if c, ok := def["compression_level"]; ok {
		s, _ := c.(string)
		switch s {
		case "1x", "2x", "4x", "8x", "16x", "32x":
		default:
			return wrappedError(fmt.Sprintf("Unknown value [%v] for field [compression_level] - accepted values are [, 1x, 2x, 4x, 8x, 16x, 32x]", c), "mapper_parsing_exception")
		}
	}
	dataType := "float"
	if dt, ok := def["data_type"]; ok {
		s, _ := dt.(string)
		s = strings.ToLower(s)
		switch s {
		case "binary", "byte", "float", "half_float":
		default:
			return wrappedError("Invalid value provided for [data_type] field. Supported values are [binary,byte,float,half_float]", "illegal_argument_exception")
		}
		dataType = s
		def["data_type"] = strings.ToUpper(s)
	}
	topSpace, _ := def["space_type"].(string)
	engine, methodName, methodSpace := "faiss", "hnsw", ""
	var params map[string]any
	method, hasMethod := def["method"].(map[string]any)
	if hasMethod {
		n, ok := method["name"].(string)
		if !ok {
			return wrappedError("name needs to be set", "mapper_parsing_exception")
		}
		methodName = n
		if e, ok := method["engine"].(string); ok {
			engine = e
		}
		switch engine {
		case "faiss", "lucene":
		case "nmslib":
			return wrappedError("nmslib engine is deprecated in OpenSearch  and cannot be used for new index creation in OpenSearch from  3.0.0.", "illegal_argument_exception")
		default:
			return wrappedError("Invalid engine: "+engine, "mapper_parsing_exception")
		}
		methodSpace, _ = method["space_type"].(string)
		if topSpace != "" && methodSpace != "" && topSpace != methodSpace {
			return wrappedError(fmt.Sprintf(`Cannot specify conflicting space types: "[%s]" "[%s]"`, methodSpace, topSpace), "mapper_parsing_exception")
		}
		params, _ = method["parameters"].(map[string]any)
	}
	space := topSpace
	if space == "" {
		space = methodSpace
	}
	if space != "" {
		found := false
		for _, s := range knnSpaceTypes {
			if s == space {
				found = true
			}
		}
		if !found {
			return wrappedError("Unable to find space: "+space+" . Valid values are: [l2, cosinesimil, l1, linf, innerproduct, hamming]", "illegal_argument_exception")
		}
		if space == "hamming" && dataType != "binary" {
			return wrappedError(fmt.Sprintf("Space type [hamming] is not supported with [%s] data type", dataType), "illegal_argument_exception")
		}
		if dataType == "binary" && space != "hamming" {
			return wrappedError(fmt.Sprintf("Space type [%s] is not supported with [binary] data type", space), "illegal_argument_exception")
		}
	}
	if hasMethod {
		switch {
		case methodName == "hnsw":
		case methodName == "ivf" && engine == "faiss":
			return knnValidation(`"method" requires training.`)
		default:
			return wrappedError("Invalid method name: "+methodName, "illegal_argument_exception")
		}
	}
	if dim > 16000 {
		return knnValidation("Dimension value cannot be greater than 16000 for vector with engine: " + engine)
	}
	if dataType == "binary" && dim%8 != 0 {
		return knnValidation("Dimension should be multiply of 8 for binary vector data type")
	}
	if space == "l1" || space == "linf" {
		return knnValidation(fmt.Sprintf(`"%s" with "%s" configuration does not support space type: "%s".`, methodName, engine, space))
	}
	for _, k := range SortedAnyKeys(params) {
		switch k {
		case "m", "ef_construction", "ef_search":
			if n, isNum := params[k].(json.Number); isNum && !strings.ContainsAny(n.String(), ".eE") {
				continue
			}
			if _, ok := params[k].(float64); !ok {
				if _, ok := params[k].(int64); !ok {
					return knnValidation(fmt.Sprintf("value is not an instance of Integer for Integer parameter [%s].", k))
				}
			}
		case "encoder":
		default:
			return knnValidation(fmt.Sprintf("Unknown parameter '%s' found", k))
		}
	}
	if dv, ok := def["doc_values"].(bool); ok && dv {
		delete(def, "doc_values")
	}
	if hasMethod {
		ms := space
		if ms == "" {
			ms = "l2"
			if dataType == "binary" {
				ms = "hamming"
			}
		}
		if params == nil {
			params = map[string]any{}
		}
		method["engine"] = engine
		method["space_type"] = ms
		method["parameters"] = params
	}
	return nil
}

// ---------------------------------------------------------------------------
// knn query
// ---------------------------------------------------------------------------
