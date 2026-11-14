package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

var knownMappingBodyKeys = map[string]bool{
	"properties": true, "dynamic": true, "_source": true, "_meta": true,
	"dynamic_templates": true, "date_detection": true, "numeric_detection": true,
	"_routing": true, "_field_names": true, "runtime": true, "_size": true,
	"_timestamp": true, "_ttl": true, "_doc": false,
}

func (s *Server) getMappingAPI(w http.ResponseWriter, r *http.Request, spec, fields string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	o := s.parseResolveParams(r, resolveOpts2{})
	indices, apiErr2 := s.resolveSpec2(spec, o)
	if apiErr2 != nil {
		writeError(w, apiErr2)
		return
	}
	includeDefaults := paramBool(r, "include_defaults")
	out := map[string]any{}
	for _, idx := range indices {
		if err := metaReadBlock(idx); err != nil {
			writeError(w, err)
			return
		}
		if fields == "" {
			mappings := idx.Mappings
			if mappings == nil {
				mappings = map[string]any{}
			}
			out[idx.Name] = map[string]any{"mappings": knnOrderMappings(mappings)}
		} else {
			out[idx.Name] = map[string]any{"mappings": fieldMappings(idx, fields, includeDefaults)}
		}
	}
	writeJSON(w, 200, out)
}

func fieldMappings(idx *Index, spec string, includeDefaults bool) map[string]any {
	leaves := []fieldLeaf{}
	if props, ok := idx.Mappings["properties"].(map[string]any); ok {
		collectLeaves(props, "", &leaves)
	}
	patterns := strings.Split(spec, ",")
	result := map[string]any{}
	for _, leaf := range leaves {
		if !matchAnySettingPattern(patterns, leaf.path) {
			continue
		}
		def := map[string]any{}
		for k, v := range leaf.def {
			def[k] = v
		}
		if includeDefaults {
			mergeFieldDefaults(def)
		}
		result[leaf.path] = map[string]any{
			"full_name": leaf.path,
			"mapping":   map[string]any{leaf.leafName: def},
		}
	}
	return result
}

type fieldLeaf struct {
	path     string
	leafName string
	def      map[string]any
}

func collectLeaves(props map[string]any, prefix string, out *[]fieldLeaf) {
	for name, raw := range props {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		if sub, ok := def["properties"].(map[string]any); ok {
			collectLeaves(sub, path, out)
		}
		if _, hasType := def["type"]; hasType {
			*out = append(*out, fieldLeaf{path: path, leafName: name, def: def})
		}
		if fields, ok := def["fields"].(map[string]any); ok {
			for fname, fraw := range fields {
				if fdef, ok := fraw.(map[string]any); ok {
					*out = append(*out, fieldLeaf{path: path + "." + fname, leafName: fname, def: fdef})
				}
			}
		}
	}
}

func mergeFieldDefaults(def map[string]any) {
	t, _ := def["type"].(string)
	base := map[string]any{}
	switch t {
	case "float", "double", "integer", "long", "short", "byte", "half_float", "scaled_float":
		base = map[string]any{"index": true, "doc_values": true, "store": false,
			"coerce": true, "ignore_malformed": false, "null_value": nil}
	case "keyword", "constant_keyword", "wildcard":
		base = map[string]any{"index": true, "doc_values": true, "store": false,
			"ignore_above": 2147483647, "null_value": nil}
	case "text":
		base = map[string]any{"index": true, "store": false, "analyzer": "default",
			"search_analyzer": "default", "boost": 1.0, "null_value": nil}
	case "date":
		base = map[string]any{"index": true, "doc_values": true, "store": false, "null_value": nil}
	case "boolean":
		base = map[string]any{"index": true, "doc_values": true, "store": false, "null_value": nil}
	case "object", "nested":
		base = map[string]any{"dynamic": "true", "enabled": true}
	case "ip", "binary", "geo_point", "geo_shape", "completion":
		base = map[string]any{"index": true, "doc_values": true, "store": false, "null_value": nil}
	}
	for k, v := range base {
		if _, ok := def[k]; !ok {
			def[k] = v
		}
	}
}

func (s *Server) putMappingAPI(w http.ResponseWriter, r *http.Request, spec string) {
	body, apiErr2 := readJSON(r)
	if apiErr2 != nil {
		writeError(w, apiErr2)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.parseResolveParams(r, resolveOpts2{})
	indices, apiErr3 := s.resolveSpec2(spec, o)
	if apiErr3 != nil {
		writeError(w, apiErr3)
		return
	}
	for k := range body {
		if !knownMappingBodyKeys[k] {
			writeError(w, &apiError{Type: "illegal_argument_exception", Status: 400,
				Reason: "Types cannot be provided in put mapping requests"})
			return
		}
	}
	if v, ok := body["dynamic"]; ok {
		s, valid := parseDynamicSetting(v)
		if !valid {
			writeError(w, dynamicConvertError("dynamic", v))
			return
		}
		body["dynamic"] = s
	}
	if props, ok := body["properties"].(map[string]any); ok {
		if err := expandDottedProperties(props); err != nil {
			reason := err.Reason
			writeError(w, &apiError{Type: "mapper_parsing_exception", Status: 400,
				Reason:   "Failed to parse mapping [_doc]: " + reason,
				CausedBy: &apiError{Type: "illegal_argument_exception", Reason: reason}})
			return
		}
		if err := validateProps(props); err != nil {
			writeError(w, &apiError{Type: "mapper_parsing_exception", Status: 400, Reason: err.Reason})
			return
		}
		normalizeKnnVectors(props)
		if err := normalizeDynamicProps(props, ""); err != nil {
			writeError(w, err)
			return
		}
	}
	if src, ok := body["_source"].(map[string]any); ok {
		for _, k := range sortedAnyKeys(src) {
			switch k {
			case "enabled", "includes", "excludes", "include", "exclude":
			default:
				writeError(w, &apiError{Type: "mapper_parsing_exception", Status: 400,
					Reason: fmt.Sprintf("unknown parameter [%s] on mapper [_source] of type [null]", k)})
				return
			}
		}
	}
	for _, idx := range indices {
		if err := metaReadBlock(idx); err != nil {
			writeError(w, err)
			return
		}
		if rv, ok := body["_routing"].(map[string]any); ok {
			if reqRaw, has := rv["required"]; has {
				oldReq := false
				if old, ok := idx.Mappings["_routing"].(map[string]any); ok {
					if v, ok := old["required"].(bool); ok {
						oldReq = v
					}
				}
				newReq, _ := reqRaw.(bool)
				if oldReq != newReq {
					writeError(w, &apiError{Type: "illegal_argument_exception", Status: 400,
						Reason: "Mapper for [_routing] conflicts with existing mapper:\n\tCannot update parameter [required] from [" + strconv.FormatBool(oldReq) + "] to [" + strconv.FormatBool(newReq) + "]"})
					return
				}
			}
		}
		existingProps := ensureProperties(idx.Mappings)
		if props, ok := body["properties"].(map[string]any); ok {
			if err := knnPutMappingCheck(idx, existingProps, props); err != nil {
				writeError(w, err)
				return
			}
			if err := validateMappingAnalyzers(props, idx.Settings); err != nil {
				writeError(w, err)
				return
			}
			if err := mergeMappingChecked(existingProps, props); err != nil {
				writeError(w, err)
				return
			}
		}
		if d, ok := body["dynamic"]; ok {
			idx.Mappings["dynamic"] = d
		}
		if src, ok := body["_source"].(map[string]any); ok {
			if newEnabled, has := src["enabled"]; has {
				oldEnabled := true
				if old, ok := idx.Mappings["_source"].(map[string]any); ok {
					if v, ok := old["enabled"].(bool); ok {
						oldEnabled = v
					}
				}
				newBool, _ := newEnabled.(bool)
				if oldEnabled != newBool {
					writeError(w, &apiError{Type: "illegal_argument_exception", Status: 400,
						Reason: "Mapper for [_source] conflicts with existing mapper:\n\tCannot update parameter [enabled] from [" + strconv.FormatBool(oldEnabled) + "] to [" + strconv.FormatBool(newBool) + "]"})
					return
				}
			}
		}
		for k, v := range body {
			if k == "properties" || k == "dynamic" {
				continue
			}
			idx.Mappings[k] = v
		}
		if derr := checkTotalFields(idx); derr != nil {
			writeError(w, derr)
			return
		}
		if derr := checkNestedFields(idx); derr != nil {
			writeError(w, derr)
			return
		}
		idx.MappingVersion++
		ensureFieldTypes(idx)
	}
	s.bumpState()
	writeJSON(w, 200, map[string]any{"acknowledged": true})
}

// mergeMappingChecked merges new mapping properties into existing ones,
// returning OpenSearch-compatible conflict errors.
func mergeMappingChecked(existing, incoming map[string]any) *apiError {
	for name, rawNew := range incoming {
		newDef, ok := rawNew.(map[string]any)
		if !ok {
			continue
		}
		oldRaw, exists := existing[name]
		if !exists {
			existing[name] = newDef
			continue
		}
		oldDef, ok := oldRaw.(map[string]any)
		if !ok {
			existing[name] = newDef
			continue
		}
		if err := mergeFieldDef(name, oldDef, newDef); err != nil {
			return err
		}
	}
	return nil
}

func fieldObjectShape(def map[string]any) (objectLike, nested bool) {
	t, _ := def["type"].(string)
	_, hasProps := def["properties"].(map[string]any)
	return hasProps || t == "object" || t == "nested", t == "nested"
}

func mapperParamDefault(typ, param string) (any, bool) {
	switch param {
	case "analyzer":
		if typ == "text" || typ == "match_only_text" || typ == "search_as_you_type" {
			return "default", true
		}
	case "normalizer":
		if typ == "keyword" || typ == "constant_keyword" || typ == "wildcard" {
			return "default", true
		}
	case "index":
		return true, true
	case "doc_values":
		return true, true
	case "index_options":
		if typ == "text" {
			return "positions", true
		}
	case "null_value":
		return nil, true
	case "enabled":
		return true, true
	}
	return nil, false
}

func mergeFieldDef(name string, oldDef, newDef map[string]any) *apiError {
	oldType, _ := oldDef["type"].(string)
	newType, _ := newDef["type"].(string)
	if oldType == "alias" && newType != "alias" {
		return errBad("Cannot merge a field alias mapping [" + name + "] with a mapping that is not for a field alias.")
	}
	if newType == "alias" && oldType != "alias" {
		return errBad("Cannot merge a field mapping [" + name + "] with a mapping that is not for a field alias.")
	}
	oldObj, oldNested := fieldObjectShape(oldDef)
	newObj, newNested := fieldObjectShape(newDef)
	if oldObj && newObj {
		if oldNested != newNested {
			if oldNested {
				return errBad("cannot change object mapping from nested to non-nested")
			}
			return errBad("cannot change object mapping from non-nested to nested")
		}
		if oe, hasOld := oldDef["enabled"].(bool); hasOld {
			ne, hasNew := newDef["enabled"].(bool)
			if hasNew && oe != ne {
				return &apiError{Type: "mapper_exception", Status: 500,
					Reason: "the [enabled] parameter can't be updated for the object mapping [" + name + "]"}
			}
		}
		oldProps, _ := oldDef["properties"].(map[string]any)
		newProps, _ := newDef["properties"].(map[string]any)
		if oldProps != nil && newProps != nil {
			if err := mergeMappingChecked(oldProps, newProps); err != nil {
				return err
			}
		}
	} else if oldObj != newObj {
		return errBad("can't merge a non object mapping [" + name + "] with an object mapping")
	}
	if newType != "" && oldType != "" && newType != oldType {
		return errBad("mapper [" + name + "] cannot be changed from type [" + oldType + "] to [" + newType + "]")
	}
	effType := oldType
	if effType == "" {
		effType = newType
	}
	for _, param := range []string{"analyzer", "normalizer", "index", "doc_values", "index_options", "null_value", "enabled", "value"} {
		oldVal, oldHas := oldDef[param]
		newVal, newHas := newDef[param]
		if !oldHas && !newHas {
			continue
		}
		if !oldHas {
			def, ok := mapperParamDefault(effType, param)
			if !ok {
				continue
			}
			oldVal = def
		}
		if !newHas {
			def, ok := mapperParamDefault(effType, param)
			if !ok {
				continue
			}
			newVal = def
		}
		if settingsValueString(oldVal) != settingsValueString(newVal) {
			return &apiError{Type: "illegal_argument_exception", Status: 400,
				Reason: "Mapper for [" + name + "] conflicts with existing mapper:\n\tCannot update parameter [" + param + "] from [" + settingsValueString(oldVal) + "] to [" + settingsValueString(newVal) + "]"}
		}
	}
	for pk, pv := range newDef {
		switch pk {
		case "type", "properties":
			continue
		case "fields":
			oldFields, _ := oldDef["fields"].(map[string]any)
			if oldFields == nil {
				oldFields = map[string]any{}
				oldDef["fields"] = oldFields
			}
			newFields, _ := pv.(map[string]any)
			for fk, fv := range newFields {
				newF, okNew := fv.(map[string]any)
				oldF, okOld := oldFields[fk].(map[string]any)
				if okNew && okOld {
					if err := mergeFieldDef(name+"."+fk, oldF, newF); err != nil {
						return err
					}
					continue
				}
				oldFields[fk] = fv
			}
			continue
		}
		oldDef[pk] = pv
	}
	return nil
}
