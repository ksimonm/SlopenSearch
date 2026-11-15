package mapping

import (
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/model"
	"slopensearch/internal/settings"
)

var KnownBodyKeys = map[string]bool{
	"properties": true, "dynamic": true, "_source": true, "_meta": true,
	"dynamic_templates": true, "date_detection": true, "numeric_detection": true,
	"_routing": true, "_field_names": true, "runtime": true, "_size": true,
	"_timestamp": true, "_ttl": true, "_doc": false,
}

func FieldMappings(idx *model.Index, spec string, includeDefaults bool) map[string]any {
	leaves := []fieldLeaf{}
	if props, ok := idx.Mappings["properties"].(map[string]any); ok {
		collectLeaves(props, "", &leaves)
	}
	patterns := strings.Split(spec, ",")
	result := map[string]any{}
	for _, leaf := range leaves {
		if !settings.MatchAny(patterns, leaf.path) {
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

// MergeChecked merges new mapping properties into existing ones,
// returning OpenSearch-compatible conflict errors.
func MergeChecked(existing, incoming map[string]any) *apierr.Error {
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

func mergeFieldDef(name string, oldDef, newDef map[string]any) *apierr.Error {
	oldType, _ := oldDef["type"].(string)
	newType, _ := newDef["type"].(string)
	if oldType == "alias" && newType != "alias" {
		return apierr.BadRequest("Cannot merge a field alias mapping [" + name + "] with a mapping that is not for a field alias.")
	}
	if newType == "alias" && oldType != "alias" {
		return apierr.BadRequest("Cannot merge a field mapping [" + name + "] with a mapping that is not for a field alias.")
	}
	oldObj, oldNested := fieldObjectShape(oldDef)
	newObj, newNested := fieldObjectShape(newDef)
	if oldObj && newObj {
		if oldNested != newNested {
			if oldNested {
				return apierr.BadRequest("cannot change object mapping from nested to non-nested")
			}
			return apierr.BadRequest("cannot change object mapping from non-nested to nested")
		}
		if oe, hasOld := oldDef["enabled"].(bool); hasOld {
			ne, hasNew := newDef["enabled"].(bool)
			if hasNew && oe != ne {
				return &apierr.Error{Type: "mapper_exception", Status: 500,
					Reason: "the [enabled] parameter can't be updated for the object mapping [" + name + "]"}
			}
		}
		oldProps, _ := oldDef["properties"].(map[string]any)
		newProps, _ := newDef["properties"].(map[string]any)
		if oldProps != nil && newProps != nil {
			if err := MergeChecked(oldProps, newProps); err != nil {
				return err
			}
		}
	} else if oldObj != newObj {
		return apierr.BadRequest("can't merge a non object mapping [" + name + "] with an object mapping")
	}
	if newType != "" && oldType != "" && newType != oldType {
		return apierr.BadRequest("mapper [" + name + "] cannot be changed from type [" + oldType + "] to [" + newType + "]")
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
		if settings.ValueString(oldVal) != settings.ValueString(newVal) {
			return &apierr.Error{Type: "illegal_argument_exception", Status: 400,
				Reason: "Mapper for [" + name + "] conflicts with existing mapper:\n\tCannot update parameter [" + param + "] from [" + settings.ValueString(oldVal) + "] to [" + settings.ValueString(newVal) + "]"}
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
