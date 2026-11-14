package httpapi

import (
	"fmt"
	"net/http"
	"strconv"

	"slopensearch/internal/apierr"
	"slopensearch/internal/mapping"
)

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
			out[idx.Name] = map[string]any{"mappings": mapping.KnnOrderMappings(mappings)}
		} else {
			out[idx.Name] = map[string]any{"mappings": mapping.FieldMappings(idx, fields, includeDefaults)}
		}
	}
	writeJSON(w, 200, out)
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
		if !mapping.KnownBodyKeys[k] {
			writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
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
		if err := mapping.ExpandDottedProperties(props); err != nil {
			reason := err.Reason
			writeError(w, &apierr.Error{Type: "mapper_parsing_exception", Status: 400,
				Reason:   "Failed to parse mapping [_doc]: " + reason,
				CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: reason}})
			return
		}
		if err := mapping.ValidateProps(props); err != nil {
			writeError(w, &apierr.Error{Type: "mapper_parsing_exception", Status: 400, Reason: err.Reason})
			return
		}
		mapping.NormalizeKnnVectors(props)
		if err := normalizeDynamicProps(props, ""); err != nil {
			writeError(w, err)
			return
		}
	}
	if src, ok := body["_source"].(map[string]any); ok {
		for _, k := range mapping.SortedAnyKeys(src) {
			switch k {
			case "enabled", "includes", "excludes", "include", "exclude":
			default:
				writeError(w, &apierr.Error{Type: "mapper_parsing_exception", Status: 400,
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
					writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
						Reason: "Mapper for [_routing] conflicts with existing mapper:\n\tCannot update parameter [required] from [" + strconv.FormatBool(oldReq) + "] to [" + strconv.FormatBool(newReq) + "]"})
					return
				}
			}
		}
		existingProps := mapping.EnsureProperties(idx.Mappings)
		if props, ok := body["properties"].(map[string]any); ok {
			if err := mapping.KnnPutMappingCheck(idx, existingProps, props); err != nil {
				writeError(w, err)
				return
			}
			if err := mapping.ValidateAnalyzers(props, idx.Settings); err != nil {
				writeError(w, err)
				return
			}
			if err := mapping.MergeChecked(existingProps, props); err != nil {
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
					writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
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
		mapping.EnsureFieldTypes(idx)
	}
	s.bumpState()
	writeJSON(w, 200, map[string]any{"acknowledged": true})
}
