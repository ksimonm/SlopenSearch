package search

import (
	"sort"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
)

// ApplyCollapse applies the collapse section of a search body to the sorted
// hits and returns the kept hits with their per-hit collapse data.
func ApplyCollapse(matched []MultiHit, body map[string]any) ([]MultiHit, map[string]map[string]any, map[string]map[string]any, *apierr.Error) {
	collapse, ok := body["collapse"].(map[string]any)
	if !ok {
		return matched, nil, nil, nil
	}
	if _, has := body["rescore"]; has {
		return nil, nil, nil, &apierr.Error{Type: "search_phase_execution_exception", Status: 500, Reason: "all shards failed",
			RootCause: &apierr.Error{Type: "search_exception", Reason: "cannot use `collapse` in conjunction with `rescore`"}}
	}
	field, _ := collapse["field"].(string)
	var def map[string]any
	found := false
	for _, h := range matched {
		if d, ok := mapping.FieldDef(h.Idx, field); ok {
			def = d
			found = true
			break
		}
	}
	if !found {
		reason := "no mapping found for `" + field + "` in order to collapse on"
		return nil, nil, nil, &apierr.Error{Type: "search_phase_execution_exception", Status: 400, Reason: reason,
			RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason}}
	}
	ftype, _ := def["type"].(string)
	if ftype == "text" || ftype == "match_only_text" || ftype == "search_as_you_type" {
		reason := "unknown type for collapse field `" + field + "`, only keywords and numbers are accepted"
		return nil, nil, nil, &apierr.Error{Type: "search_phase_execution_exception", Status: 400, Reason: reason,
			RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason}}
	}
	for _, h := range matched {
		if vals := h.Idx.DocValues[h.Id]; vals != nil && len(vals[field]) > 1 {
			reason := "the collapse field must be single valued"
			return nil, nil, nil, &apierr.Error{Type: "search_phase_execution_exception", Status: 500, Reason: "all shards failed",
				RootCause: &apierr.Error{Type: "illegal_state_exception", Reason: reason},
				CausedBy:  &apierr.Error{Type: "illegal_state_exception", Reason: reason}}
		}
	}
	groups := map[string][]MultiHit{}
	order := []string{}
	for _, h := range matched {
		key := ""
		vals := h.Idx.DocValues[h.Id]
		if vals != nil && len(vals[field]) > 0 {
			s, _ := jsonv.ScalarString(vals[field][0])
			key = s
		} else {
			key = "\x00missing:" + h.Idx.Name + ":" + h.Id
		}
		if _, exists := groups[key]; !exists {
			order = append(order, key)
		}
		groups[key] = append(groups[key], h)
	}
	collapsed := make([]MultiHit, 0, len(order))
	fieldsByID := map[string]map[string]any{}
	for _, key := range order {
		g := groups[key]
		if len(g) == 0 {
			continue
		}
		h := g[0]
		collapsed = append(collapsed, h)
		vals := h.Idx.DocValues[h.Id]
		if vals != nil && len(vals[field]) > 0 {
			fieldsByID[h.Id] = map[string]any{field: []any{vals[field][0]}}
		}
	}
	innerByID := map[string]map[string]any{}
	innerRaw := collapse["inner_hits"]
	if innerRaw != nil {
		specs := []map[string]any{}
		switch t := innerRaw.(type) {
		case map[string]any:
			specs = append(specs, t)
		case []any:
			for _, e := range t {
				if m, ok := e.(map[string]any); ok {
					specs = append(specs, m)
				}
			}
		}
		for _, spec := range specs {
			name, _ := spec["name"].(string)
			if name == "" {
				return nil, nil, nil, &apierr.Error{Type: "illegal_argument_exception", Status: 400, Reason: "Field name cannot be null"}
			}
			size := 3
			if n, ok := jsonv.ToFloat(spec["size"]); ok {
				size = int(n)
			}
			srcFalse := false
			if v, ok := spec["_source"].(bool); ok && !v {
				srcFalse = true
			}
			var specs2 []SortSpec
			if rawSort, has := spec["sort"]; has {
				var serr *apierr.Error
				specs2, serr = ParseSortSpecs(rawSort, "")
				if serr != nil {
					return nil, nil, nil, serr
				}
			}
			for _, key := range order {
				outer := groups[key][0]
				g := append([]MultiHit{}, groups[key]...)
				if len(specs2) > 0 {
					sort.SliceStable(g, func(i, j int) bool {
						for _, sp := range specs2 {
							c := CompareSortSpec(g[i], g[j], sp)
							if c != 0 {
								if sp.Desc {
									return c > 0
								}
								return c < 0
							}
						}
						return false
					})
				}
				if len(g) > size {
					g = g[:size]
				}
				innerHits := []any{}
				for _, ih := range g {
					e := map[string]any{"_index": ih.Idx.Name, "_id": ih.Id, "_score": nil}
					if len(specs2) > 0 {
						sv := []any{}
						for _, sp := range specs2 {
							sv = append(sv, SortOutputValue(ih, sp))
						}
						e["sort"] = sv
					}
					if !srcFalse {
						e["_source"] = ih.Doc
					}
					innerHits = append(innerHits, e)
				}
				obj := map[string]any{"hits": map[string]any{
					"total":     map[string]any{"value": len(groups[key]), "relation": "eq"},
					"max_score": nil,
					"hits":      innerHits,
				}}
				if innerByID[outer.Id] == nil {
					innerByID[outer.Id] = map[string]any{}
				}
				innerByID[outer.Id][name] = obj
			}
		}
	}
	return collapsed, fieldsByID, innerByID, nil
}

// ApplyRescore applies the rescore section of a search body to the hits.
func ApplyRescore(matched []MultiHit, body map[string]any) *apierr.Error {
	raw, has := body["rescore"]
	if !has || raw == nil {
		return nil
	}
	if s, ok := body["sort"].([]any); ok && len(s) > 0 {
		return &apierr.Error{Type: "search_phase_execution_exception", Status: 400, Reason: "all shards failed",
			RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: "Cannot use [sort] option in conjunction with [rescore]."}}
	}
	specs := []map[string]any{}
	switch t := raw.(type) {
	case map[string]any:
		specs = append(specs, t)
	case []any:
		for _, e := range t {
			if m, ok := e.(map[string]any); ok {
				specs = append(specs, m)
			}
		}
	}
	for _, spec := range specs {
		window := 10
		if n, ok := jsonv.ToFloat(spec["window_size"]); ok {
			window = int(n)
		}
		qm, _ := spec["query"].(map[string]any)
		if qm == nil {
			continue
		}
		rq, _ := qm["rescore_query"].(map[string]any)
		if rq == nil {
			continue
		}
		mode := "total"
		if m, ok := qm["score_mode"].(string); ok && m != "" {
			mode = m
		}
		qw := 1.0
		if v, ok := jsonv.ToFloat(qm["query_weight"]); ok {
			qw = v
		}
		rw := 1.0
		if v, ok := jsonv.ToFloat(qm["rescore_query_weight"]); ok {
			rw = v
		}
		order := make([]int, len(matched))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(a, b int) bool {
			return matched[order[a]].Score > matched[order[b]].Score
		})
		if window < len(order) {
			order = order[:window]
		}
		for _, i := range order {
			h := &matched[i]
			qc := NewQueryCtxForDoc(h.Idx, h.Id, h.Doc)
			ok, rs, err := qc.Eval(rq)
			if err != nil {
				return err
			}
			base := h.Score * qw
			if !ok || rs == 0 {
				h.Score = base
				continue
			}
			weighted := rs * rw
			switch mode {
			case "multiply":
				h.Score = base * weighted
			case "avg":
				h.Score = (base + weighted) / 2
			case "max":
				if weighted > base {
					h.Score = weighted
				} else {
					h.Score = base
				}
			case "min":
				if weighted < base {
					h.Score = weighted
				} else {
					h.Score = base
				}
			default:
				h.Score = base + weighted
			}
		}
	}
	return nil
}
