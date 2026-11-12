package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// mappingValidationError carries the OpenSearch mapper validation message and
// the type of the caused_by error used when the error is wrapped in an index
// creation response.
type mappingValidationError struct {
	Reason string
	Cause  string
}

func (e *mappingValidationError) Error() string { return e.Reason }

func mve(reason, cause string) *mappingValidationError {
	if cause == "" {
		cause = "mapper_parsing_exception"
	}
	return &mappingValidationError{Reason: reason, Cause: cause}
}

var allowedMappingParams = map[string]bool{
	"type": true, "properties": true, "fields": true, "analyzer": true,
	"search_analyzer": true, "search_quote_analyzer": true, "normalizer": true,
	"index": true, "doc_values": true, "store": true, "coerce": true,
	"ignore_malformed": true, "null_value": true, "boost": true, "copy_to": true,
	"dynamic": true, "enabled": true, "format": true, "ignore_above": true,
	"index_options": true, "norms": true, "similarity": true, "term_vector": true,
	"positions": true, "position_increment_gap": true, "fielddata": true,
	"eager_global_ordinals": true, "split_queries_on_whitespace": true,
	"max_shingle_size": true, "path": true, "scaling_factor": true,
	"on_script_error": true, "script": true, "value": true, "meta": true,
	"_meta": true, "index_phrases": true, "index_prefixes": true,
	"preserve_separator": true, "max_input_length": true, "relations": true,
	"ignore_z_value": true, "orientation": true, "points_only": true,
	"tree": true, "precision": true, "strategy": true,
	"distance_error_pct": true, "fielddata_frequency_filter": true,
	"loading": true, "chunk_size": true, "positive_score_impact": true,
	"enable_position_increments": true, "include_in_parent": true,
	"include_in_root": true, "preserve_separators": true, "preserve_position_increments": true, "contexts": true,
	"quote_analyzer": true, "dimension": true, "method": true, "model_id": true,
	"space_type": true, "ef_construction": true, "m": true, "encoder": true,
}

func sortedAnyKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func intFromAny(v any) (int64, bool) {
	f, ok := toFloat(v)
	if !ok {
		return 0, false
	}
	return int64(f), true
}

func javaFloatRepr(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

func javaMetaRepr(v any) string {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return "Integer[" + strconv.FormatInt(i, 10) + "]"
		}
		return "Double[" + t.String() + "]"
	case float64:
		if t == float64(int64(t)) {
			return "Integer[" + strconv.FormatInt(int64(t), 10) + "]"
		}
		return "Double[" + formatNumber(t) + "]"
	case map[string]any:
		return "LinkedHashMap[" + javaMapDebug(v) + "]"
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, javaMetaRepr(e))
		}
		return "ArrayList[" + strings.Join(parts, ", ") + "]"
	default:
		return "String[" + previewValue(v) + "]"
	}
}

func previewJSON(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return previewValue(v)
	}
}

var knownDateFormatParts = map[string]bool{
	"epoch_millis": true, "epoch_second": true, "date_optional_time": true,
	"strict_date_optional_time": true, "strict_date_optional_time_nanos": true,
	"date_optional_time_nanos": true, "date_time": true,
	"strict_date_time": true, "basic_date_time": true, "strict_date": true,
	"date": true, "basic_date": true,
}

func validateDateFormat(name, fmtStr string) *mappingValidationError {
	if fmtStr == "" {
		return nil
	}
	for _, part := range strings.Split(fmtStr, "||") {
		p := strings.TrimSpace(part)
		if p == "" || knownDateFormatParts[p] {
			continue
		}
		for _, ch := range p {
			if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' {
				if !strings.ContainsRune("yMdHmsS", ch) {
					return mve(fmt.Sprintf("Error parsing [format] on field [%s]: Invalid format: [%s]: Unknown pattern letter: %c", name, p, ch), "illegal_argument_exception")
				}
			}
		}
	}
	return nil
}

func validateProps(props map[string]any) *mappingValidationError {
	for _, name := range sortedAnyKeys(props) {
		raw := props[name]
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		t, _ := def["type"].(string)
		if t != "" && !validTypes[t] {
			return mve(fmt.Sprintf("No handler for type [%s] declared on field [%s]", t, name), "")
		}
		if t == "knn_vector" && def["dimension"] == nil {
			return mve(fmt.Sprintf("Dimension value missing for vector: %s", name), "illegal_argument_exception")
		}
		if t == "search_as_you_type" {
			if n, ok := intFromAny(def["max_shingle_size"]); ok {
				if n < 2 || n > 4 {
					return mve(fmt.Sprintf("[max_shingle_size] must be at least [2] and at most [4], got [%d]", n), "")
				}
			}
		}
		if t == "scaled_float" {
			sf, has := def["scaling_factor"]
			if !has {
				return mve("Field [scaling_factor] is required", "illegal_argument_exception")
			}
			f, _ := toFloat(sf)
			if f <= 0 {
				return mve(fmt.Sprintf("[scaling_factor] must be a positive number, got [%s]", javaFloatRepr(f)), "illegal_argument_exception")
			}
		}
		if t == "text" {
			if ip, ok := def["index_prefixes"].(map[string]any); ok {
				min := int64(2)
				if v, ok := intFromAny(ip["min_chars"]); ok {
					min = v
				}
				max := int64(5)
				if v, ok := intFromAny(ip["max_chars"]); ok {
					max = v
				}
				if min <= 0 {
					return mve(fmt.Sprintf("min_chars [%d] must be greater than zero", min), "illegal_argument_exception")
				}
				if max > 20 {
					return mve(fmt.Sprintf("max_chars [%d] must be less than 20", max), "illegal_argument_exception")
				}
				if min >= max {
					return mve(fmt.Sprintf("min_chars [%d] must be less than max_chars [%d]", min, max), "illegal_argument_exception")
				}
			}
		}
		if t == "flat_object" {
			if p, ok := def["properties"]; ok {
				return mve(fmt.Sprintf("Mapping definition for [%s] has unsupported parameters:  [properties : %s]", name, javaMapDebug(p)), "")
			}
		}
		if t == "text" && def["null_value"] != nil {
			return mve(fmt.Sprintf("unknown parameter [null_value] on mapper [%s] of type [text]", name), "")
		}
		if t == "constant_keyword" {
			if _, ok := def["value"]; !ok {
				return mve(fmt.Sprintf("Field [%s] is missing required parameter [value]", name), "parse_exception")
			}
		}
		if t == "token_count" {
			a, _ := def["analyzer"].(string)
			if a == "" {
				return mve(fmt.Sprintf("Analyzer must be set for field [%s] but wasn't.", name), "")
			}
		}
		if t == "match_only_text" {
			if io, ok := def["index_options"].(string); ok && io != "docs" {
				return mve(fmt.Sprintf("Unknown value [%s] for field [index_options] - accepted values are [docs]", io), "")
			}
			if b, ok := def["norms"].(bool); ok && b {
				return mve("Norms cannot be enabled on for match_only_text field", "")
			}
		}
		if t == "text" {
			if tv, ok := def["term_vector"].(string); ok {
				switch tv {
				case "no", "yes", "with_offsets", "with_positions", "with_positions_offsets", "with_positions_payloads", "with_positions_offsets_payloads":
				default:
					return mve(fmt.Sprintf("Unknown value [%s] for field [term_vector] - accepted values are [no, yes, with_positions, with_offsets, with_positions_offsets, with_positions_payloads, with_positions_offsets_payloads]", tv), "")
				}
			}
		}
		if t == "keyword" || t == "text" {
			if io, ok := def["index_options"].(string); ok {
				valid := false
				accepted := "[positions, docs, freqs, offsets]"
				if t == "keyword" {
					accepted = "[docs, freqs]"
					valid = io == "docs" || io == "freqs"
				} else {
					valid = io == "positions" || io == "docs" || io == "freqs" || io == "offsets"
				}
				if !valid {
					return mve(fmt.Sprintf("Unknown value [%s] for field [index_options] - accepted values are %s", io, accepted), "")
				}
			}
		}
		if meta, ok := def["meta"].(map[string]any); ok {
			for _, mk := range sortedAnyKeys(meta) {
				mv := meta[mk]
				if _, isStr := mv.(string); !isStr {
					return mve(fmt.Sprintf("[meta] values can only be strings, but got %s for field [%s]", javaMetaRepr(mv), name), "")
				}
			}
		}
		if t == "date" || t == "date_nanos" {
			if fs, ok := def["format"].(string); ok {
				if err := validateDateFormat(name, fs); err != nil {
					return err
				}
			}
		}
		if t == "integer" || t == "long" || t == "short" || t == "byte" {
			if nv, ok := def["null_value"]; ok && nv != nil {
				if _, isNum := toFloat(nv); !isNum {
					return mve("For input string: \""+previewValue(nv)+"\"", "number_format_exception")
				}
			}
		}
		for _, k := range sortedAnyKeys(def) {
			if !allowedMappingParams[k] {
				return mve(fmt.Sprintf("unknown parameter [%s] on mapper [%s] of type [%s]", k, name, t), "")
			}
		}
		if sub, ok := def["properties"].(map[string]any); ok {
			if err := validateProps(sub); err != nil {
				return err
			}
		}
		if fields, ok := def["fields"].(map[string]any); ok {
			for _, fname := range sortedAnyKeys(fields) {
				if strings.Contains(fname, ".") {
					return mve(fmt.Sprintf("Field name [%s] which is a multi field of [%s] cannot contain '.'", fname, name), "")
				}
				if ft, ok := fields[fname].(map[string]any)["type"].(string); ok {
					if ft == "object" || ft == "nested" {
						return mve(fmt.Sprintf("Type [%s] cannot be used in multi field", ft), "")
					}
				}
			}
			if err := validateProps(fields); err != nil {
				return err
			}
		}
	}
	return nil
}

func countMappingType(props map[string]any, wanted string) int {
	n := 0
	for _, raw := range props {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := def["type"].(string); t == wanted {
			n++
		}
		if sub, ok := def["properties"].(map[string]any); ok {
			n += countMappingType(sub, wanted)
		}
	}
	return n
}

// validatePropsMeta performs the checks OpenSearch runs before mapper parsing.
func validatePropsMeta(props map[string]any) *apiError {
	var walk func(props map[string]any) *apiError
	walk = func(props map[string]any) *apiError {
		for _, name := range sortedAnyKeys(props) {
			if name == "" || strings.Contains(name, "..") {
				reason := "name cannot be empty string"
				return wrappedMappingError(reason, "illegal_argument_exception")
			}
			switch name {
			case "_id", "_source", "_index", "_ignored":
				reason := fmt.Sprintf("Field [%s] is defined more than once", name)
				return wrappedMappingError(reason, "mapper_parsing_exception")
			}
			def, ok := props[name].(map[string]any)
			if !ok {
				continue
			}
			t, _ := def["type"].(string)
			if _, has := def["enabled"]; has && t != "" && t != "object" && t != "nested" {
				reason := fmt.Sprintf("unknown parameter [enabled] on mapper [%s] of type [%s]", name, t)
				return wrappedMappingError(reason, "mapper_parsing_exception")
			}
			if ia, has := def["ignore_above"]; has {
				if _, ok := toFloat(ia); !ok {
					reason := fmt.Sprintf("Failed to parse value [%s] as only [true] or [false] are allowed.", previewJSON(ia))
					return &apiError{Type: "mapper_parsing_exception", Status: 400,
						Reason:   "Failed to parse mapping [_doc]: " + reason,
						CausedBy: &apiError{Type: "mapper_parsing_exception", Reason: "failed to parse"}}
				}
			}
			if sub, ok := def["properties"].(map[string]any); ok {
				if err := walk(sub); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(props); err != nil {
		return err
	}
	if countMappingType(props, "join") > 1 {
		return wrappedMappingError("Field [_parent_join] is defined more than once", "illegal_argument_exception")
	}
	return nil
}

func wrappedMappingError(reason, cause string) *apiError {
	return &apiError{Type: "mapper_parsing_exception", Status: 400,
		Reason:   "Failed to parse mapping [_doc]: " + reason,
		CausedBy: &apiError{Type: cause, Reason: reason}}
}

func aliasTarget(props map[string]any, path string) map[string]any {
	parts := strings.Split(path, ".")
	cur := props
	for i, part := range parts {
		def, ok := cur[part].(map[string]any)
		if !ok {
			return nil
		}
		if i+1 == len(parts) {
			return def
		}
		cur, _ = def["properties"].(map[string]any)
		if cur == nil {
			return nil
		}
	}
	return nil
}

func aliasInvalidError(reason string) *apiError {
	return &apiError{Type: "mapper_parsing_exception", Status: 400, Reason: reason,
		CausedBy: &apiError{Type: "mapper_parsing_exception", Reason: reason}}
}

func validateAliasMappings(props map[string]any) *apiError {
	for _, name := range sortedAnyKeys(props) {
		def, ok := props[name].(map[string]any)
		if !ok {
			continue
		}
		if t, _ := def["type"].(string); t != "alias" {
			continue
		}
		path, _ := def["path"].(string)
		if path == "" {
			return wrappedMappingError(fmt.Sprintf("The [path] property must be specified for field [%s].", name), "mapper_parsing_exception")
		}
		var unsupported []string
		for _, k := range sortedAnyKeys(def) {
			if k == "type" || k == "path" {
				continue
			}
			unsupported = append(unsupported, k+" : "+previewValue(def[k]))
		}
		if len(unsupported) > 0 {
			reason := fmt.Sprintf("Mapping definition for [%s] has unsupported parameters:  [%s]", name, strings.Join(unsupported, ", "))
			return wrappedMappingError(reason, "mapper_parsing_exception")
		}
		target := aliasTarget(props, path)
		if target == nil {
			return aliasInvalidError(fmt.Sprintf("Invalid [path] value [%s] for field alias [%s]: an alias must refer to an existing field in the mappings.", path, name))
		}
		tt, _ := target["type"].(string)
		if tt == "alias" {
			return aliasInvalidError(fmt.Sprintf("Invalid [path] value [%s] for field alias [%s]: an alias cannot refer to another alias.", path, name))
		}
		if tt == "" || target["properties"] != nil {
			return aliasInvalidError(fmt.Sprintf("Invalid [path] value [%s] for field alias [%s]: an alias must refer to an existing field in the mappings.", path, name))
		}
		if scope, _, found := strings.Cut(path, "."); found {
			if scopeDef, ok := props[scope].(map[string]any); ok {
				if st, _ := scopeDef["type"].(string); st == "nested" {
					return &apiError{Type: "illegal_argument_exception", Status: 400,
						Reason: fmt.Sprintf("Invalid [path] value [%s] for field alias [%s]: an alias must have the same nested scope as its target. The alias is not nested, but the target's nested scope is [%s].", path, name, scope)}
				}
			}
		}
	}
	return nil
}

func validateCopyToTargets(props map[string]any) *apiError {
	for _, name := range sortedAnyKeys(props) {
		def, ok := props[name].(map[string]any)
		if !ok {
			continue
		}
		var targets []string
		switch t := def["copy_to"].(type) {
		case string:
			targets = []string{t}
		case []any:
			for _, e := range t {
				if s, ok := e.(string); ok {
					targets = append(targets, s)
				}
			}
		}
		for _, target := range targets {
			if td := aliasTarget(props, target); td != nil {
				if td["properties"] != nil {
					return &apiError{Type: "illegal_argument_exception", Status: 400,
						Reason: fmt.Sprintf("Cannot copy to field [%s] since it is mapped as an object", target)}
				}
				if tt, _ := td["type"].(string); tt == "object" || tt == "nested" {
					return &apiError{Type: "illegal_argument_exception", Status: 400,
						Reason: fmt.Sprintf("Cannot copy to field [%s] since it is mapped as an object", target)}
				}
			}
		}
		if sub, ok := def["properties"].(map[string]any); ok {
			if err := validateCopyToTargets(sub); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateJoinPlacement(props map[string]any, prefix string) *apiError {
	for _, name := range sortedAnyKeys(props) {
		def, ok := props[name].(map[string]any)
		if !ok {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		if t, _ := def["type"].(string); t == "join" && prefix != "" {
			reason := fmt.Sprintf("join field [%s] cannot be added inside an object or in a multi-field", path)
			return wrappedMappingError(reason, "mapper_parsing_exception")
		}
		if sub, ok := def["properties"].(map[string]any); ok {
			if err := validateJoinPlacement(sub, path); err != nil {
				return err
			}
		}
		if fields, ok := def["fields"].(map[string]any); ok {
			for _, fname := range sortedAnyKeys(fields) {
				if fdef, ok := fields[fname].(map[string]any); ok {
					if ft, _ := fdef["type"].(string); ft == "join" {
						reason := fmt.Sprintf("join field [%s.%s] cannot be added inside an object or in a multi-field", path, fname)
						return wrappedMappingError(reason, "mapper_parsing_exception")
					}
				}
			}
		}
	}
	return nil
}

func validateMappingSource(mappings map[string]any) *apiError {
	if src, ok := mappings["_source"].(map[string]any); ok {
		for _, k := range sortedAnyKeys(src) {
			switch k {
			case "enabled", "includes", "excludes", "include", "exclude":
			default:
				reason := fmt.Sprintf("unknown parameter [%s] on mapper [_source] of type [null]", k)
				return wrappedMappingError(reason, "illegal_argument_exception")
			}
		}
	}
	if rt, ok := mappings["runtime"]; ok {
		reason := fmt.Sprintf("Root mapping definition has unsupported parameters:  [runtime : %s]", javaMapDebug(rt))
		return wrappedMappingError(reason, "illegal_argument_exception")
	}
	return nil
}

// normalizeKnnVectors fills the implicit mapper parameters (doc_values and
// similar) so they are rendered back in the mapping.
func normalizeKnnVectors(props map[string]any) {
	for _, raw := range props {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch t, _ := def["type"].(string); t {
		case "knn_vector":
			if _, has := def["doc_values"]; !has {
				def["doc_values"] = true
			}
		case "wildcard":
			if _, has := def["doc_values"]; !has {
				def["doc_values"] = true
			}
		case "join":
			if _, has := def["eager_global_ordinals"]; !has {
				def["eager_global_ordinals"] = true
			}
		case "search_as_you_type":
			if _, has := def["doc_values"]; !has {
				def["doc_values"] = false
			}
			if _, has := def["max_shingle_size"]; !has {
				def["max_shingle_size"] = 3
			}
		}
		if sub, ok := def["properties"].(map[string]any); ok {
			normalizeKnnVectors(sub)
		}
		if fields, ok := def["fields"].(map[string]any); ok {
			normalizeKnnVectors(fields)
		}
	}
}
