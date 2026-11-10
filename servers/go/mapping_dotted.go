package main

import (
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Dotted field names
//
// OpenSearch expands names like "a.b.c" into nested object mappings, both for
// explicit mappings and for dynamically detected document fields.
// ---------------------------------------------------------------------------

func dottedConflict(name string) *apiError {
	reason := "can't merge a non object mapping [" + name + "] with an object mapping"
	return &apiError{Type: "illegal_argument_exception", Status: 400, Reason: reason}
}

func isObjectMappingDef(def map[string]any) bool {
	t, _ := def["type"].(string)
	return t == "" || t == "object" || t == "nested"
}

// insertDottedProperty inserts one explicit mapping entry, expanding any dots
// into object mappings.
func insertDottedProperty(root map[string]any, key string, val map[string]any) *apiError {
	name := strings.TrimRight(key, ".")
	if name == "" {
		name = key
	}
	parts := strings.Split(name, ".")
	cur := root
	for i, part := range parts {
		last := i == len(parts)-1
		existingRaw, has := cur[part]
		if last {
			if !has {
				cur[part] = val
				return nil
			}
			existing, ok := existingRaw.(map[string]any)
			if !ok {
				return dottedConflict(part)
			}
			if !isObjectMappingDef(existing) {
				return dottedConflict(part)
			}
			if !isObjectMappingDef(val) {
				return dottedConflict(part)
			}
			mergeMappingsDeep(existing, val)
			return nil
		}
		if !has {
			child := map[string]any{}
			sub := map[string]any{}
			child["properties"] = sub
			cur[part] = child
			cur = sub
			continue
		}
		existing, ok := existingRaw.(map[string]any)
		if !ok || !isObjectMappingDef(existing) {
			return dottedConflict(part)
		}
		sub, ok := existing["properties"].(map[string]any)
		if !ok {
			sub = map[string]any{}
			existing["properties"] = sub
		}
		cur = sub
	}
	return nil
}

// expandDottedProperties rewrites every properties map so dotted names become
// nested object mappings.
func expandDottedProperties(props map[string]any) *apiError {
	keys := sortedAnyKeys(props)
	out := map[string]any{}
	for _, key := range keys {
		def, ok := props[key].(map[string]any)
		if !ok {
			def = map[string]any{}
		}
		if err := insertDottedProperty(out, key, def); err != nil {
			return err
		}
	}
	for k := range props {
		delete(props, k)
	}
	for k, v := range out {
		props[k] = v
	}
	for _, raw := range props {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if sub, ok := def["properties"].(map[string]any); ok {
			if err := expandDottedProperties(sub); err != nil {
				return err
			}
		}
	}
	return nil
}

// expandDottedMapping expands the properties of a full mapping document.
func expandDottedMapping(mappings map[string]any) *apiError {
	if props, ok := mappings["properties"].(map[string]any); ok {
		return expandDottedProperties(props)
	}
	return nil
}

// expandDottedObject rewrites a source document into the nested shape used for
// dynamic mapping detection. The caller keeps the original _source.
func expandDottedObject(src map[string]any) (map[string]any, *apiError) {
	keys := make([]string, 0, len(src))
	for k := range src {
		keys = append(keys, k)
	}
	// Stable order keeps conflict reporting deterministic.
	sortStrings(keys)
	out := map[string]any{}
	for _, key := range keys {
		if err := insertDottedObject(out, key, src[key]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func insertDottedObject(root map[string]any, key string, val any) *apiError {
	name := strings.TrimRight(key, ".")
	if name == "" {
		name = key
	}
	parts := strings.Split(name, ".")
	cur := root
	for i, part := range parts {
		last := i == len(parts)-1
		existing, has := cur[part]
		if last {
			if !has {
				cur[part] = expandDottedValue(val)
				return nil
			}
			em, eok := existing.(map[string]any)
			vm, vok := val.(map[string]any)
			if eok && vok {
				merged, err := expandDottedObject(mergeDottedDocs(em, vm))
				if err != nil {
					return err
				}
				cur[part] = merged
				return nil
			}
			if eok != vok {
				return dottedConflict(part)
			}
			cur[part] = expandDottedValue(val)
			return nil
		}
		if !has {
			child := map[string]any{}
			cur[part] = child
			cur = child
			continue
		}
		em, ok := existing.(map[string]any)
		if !ok {
			return dottedConflict(part)
		}
		cur = em
	}
	return nil
}

func expandDottedValue(val any) any {
	switch t := val.(type) {
	case map[string]any:
		out, err := expandDottedObject(t)
		if err != nil {
			return t
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = expandDottedValue(e)
		}
		return out
	}
	return val
}

func mergeDottedDocs(a, b map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		am, aok := out[k].(map[string]any)
		bm, bok := v.(map[string]any)
		if aok && bok {
			out[k] = mergeDottedDocs(am, bm)
		} else {
			out[k] = v
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Mapping validation that needs index settings
// ---------------------------------------------------------------------------

// builtinAnalyzer reports whether name is an analyzer that OpenSearch ships
// with, so a mapping may reference it without declaring it in settings.
func builtinAnalyzer(name string) bool {
	switch name {
	case "standard", "simple", "whitespace", "keyword", "stop", "english",
		"lowercase", "pattern", "fingerprint",
		// Language analyzers.
		"arabic", "armenian", "basque", "bengali", "brazilian", "bulgarian",
		"catalan", "cjk", "czech", "danish", "dutch", "estonian", "finnish",
		"french", "galician", "german", "greek", "hindi", "hungarian",
		"indonesian", "irish", "italian", "latvian", "lithuanian",
		"norwegian", "persian", "portuguese", "romanian", "russian",
		"sorani", "spanish", "swedish", "thai", "turkish":
		return true
	}
	return false
}

func builtinNormalizer(name string) bool {
	switch name {
	case "lowercase", "uppercase", "asciifolding":
		return true
	}
	return false
}

func settingsHasAnalyzer(settings map[string]any, key, name string) bool {
	if _, ok := findSetting(settings, "analysis."+key+"."+name); ok {
		return true
	}
	prefix := "index.analysis." + key + "." + name + "."
	for k := range settings {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

func settingsAnalyzerExists(settings map[string]any, name string) bool {
	if builtinAnalyzer(name) {
		return true
	}
	return settingsHasAnalyzer(settings, "analyzer", name)
}

func normalizerFound(settings map[string]any, name string) bool {
	if builtinNormalizer(name) {
		return true
	}
	return settingsHasAnalyzer(settings, "normalizer", name)
}

// validateMappingAnalyzers checks analyzers and normalizers referenced from a
// mapping against the index settings.
func validateMappingAnalyzers(props map[string]any, settings map[string]any) *apiError {
	for _, name := range sortedAnyKeys(props) {
		def, ok := props[name].(map[string]any)
		if !ok {
			continue
		}
		if norm, ok := def["normalizer"].(string); ok && norm != "" && !normalizerFound(settings, norm) {
			reason := "normalizer [" + norm + "] not found for field [" + name + "]"
			return &apiError{Type: "mapper_parsing_exception", Status: 400,
				Reason:    "Failed to parse mapping [_doc]: " + reason,
				RootCause: &apiError{Type: "mapper_parsing_exception", Reason: reason},
				CausedBy:  &apiError{Type: "mapper_parsing_exception", Reason: reason}}
		}
		for _, key := range []string{"analyzer", "search_analyzer", "search_quote_analyzer"} {
			if a, ok := def[key].(string); ok && a != "" && !settingsAnalyzerExists(settings, a) {
				reason := "analyzer [" + a + "] has not been configured in mappings"
				return &apiError{Type: "mapper_parsing_exception", Status: 400,
					Reason:    "Failed to parse mapping [_doc]: " + reason,
					RootCause: &apiError{Type: "mapper_parsing_exception", Reason: "Failed to parse mapping [_doc]: " + reason},
					CausedBy:  &apiError{Type: "illegal_argument_exception", Reason: reason}}
			}
		}
		if sub, ok := def["properties"].(map[string]any); ok {
			if err := validateMappingAnalyzers(sub, settings); err != nil {
				return err
			}
		}
		if fields, ok := def["fields"].(map[string]any); ok {
			if err := validateMappingAnalyzers(fields, settings); err != nil {
				return err
			}
		}
	}
	return nil
}

// builtinMetadataFields lists the built-in field names checked against
// index.mapping.field_name_length.limit when an index is created.
var builtinMetadataFields = []string{
	"_ignored", "_id", "_index", "_routing", "_source", "_data_stream_timestamp",
}

func validateBuiltinFieldNameLength(settings map[string]any) *apiError {
	limit, ok := settingInt(settings, "mapping.field_name_length.limit")
	if !ok {
		return nil
	}
	for _, name := range builtinMetadataFields {
		if len(name) > limit {
			return &apiError{Type: "illegal_argument_exception", Status: 400,
				Reason: "Field name [" + name + "] is longer than the limit of [" + strconv.Itoa(limit) + "] characters"}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Per-document mapping limits
// ---------------------------------------------------------------------------

func mappingIntSetting(idx *Index, key string, def int) int {
	if idx != nil {
		if v, ok := settingInt(idx.Settings, key); ok {
			return v
		}
	}
	return def
}

func checkDocFieldNameLengths(v any, limit int) *apiError {
	if limit <= 0 {
		return nil
	}
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			for _, part := range strings.Split(k, ".") {
				if len(part) > limit {
					return &apiError{Type: "illegal_argument_exception", Status: 400,
						Reason: "Field name [" + part + "] is longer than the limit of [" + strconv.Itoa(limit) + "] characters"}
				}
			}
			if err := checkDocFieldNameLengths(child, limit); err != nil {
				return err
			}
		}
	case []any:
		for _, e := range t {
			if err := checkDocFieldNameLengths(e, limit); err != nil {
				return err
			}
		}
	}
	return nil
}

func countNestedDocuments(props map[string]any, doc map[string]any) int {
	count := 0
	for name, rawDef := range props {
		def, ok := rawDef.(map[string]any)
		if !ok {
			continue
		}
		v, present := doc[name]
		if !present {
			continue
		}
		sub, _ := def["properties"].(map[string]any)
		switch t, _ := def["type"].(string); t {
		case "nested":
			values := []any{v}
			if arr, ok := v.([]any); ok {
				values = arr
			}
			for _, el := range values {
				if m, ok := el.(map[string]any); ok {
					count++
					if sub != nil {
						count += countNestedDocuments(sub, m)
					}
				}
			}
		default:
			if sub == nil {
				continue
			}
			switch tv := v.(type) {
			case map[string]any:
				count += countNestedDocuments(sub, tv)
			case []any:
				for _, el := range tv {
					if m, ok := el.(map[string]any); ok {
						count += countNestedDocuments(sub, m)
					}
				}
			}
		}
	}
	return count
}

func nestedObjectsLimitError(limit int) *apiError {
	reason := "The number of nested documents has exceeded the allowed limit of [" + strconv.Itoa(limit) +
		"]. This limit can be set by changing the [index.mapping.nested_objects.limit] index level setting."
	return &apiError{Type: "mapper_parsing_exception", Status: 400, Reason: reason}
}

// indexDocCount returns the number of documents including nested documents,
// which is what _cat/indices reports as docs.count.
func indexDocCount(idx *Index) int {
	count := len(idx.Docs)
	props, _ := idx.Mappings["properties"].(map[string]any)
	if props == nil {
		return count
	}
	for _, doc := range idx.Docs {
		count += countNestedDocuments(props, doc)
	}
	return count
}
