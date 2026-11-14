package search

import (
	"sort"

	"slopensearch/internal/apierr"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
	"slopensearch/internal/painless"
)

// SortHits sorts hits by field sort specs (stable). It first checks that each
// sorted field can be sorted on in every index (geo_distance, nested path, sort
// mode, text and other non-sortable types, doc values) and computes _script sort
// values, which it stores in the specs.
func SortHits(hits []MultiHit, specs []SortSpec, indices []*model.Index) *apierr.Error {
	for _, sp := range specs {
		if sp.Field == "_score" || sp.Field == "_doc" || sp.Field == "_id" || sp.Field == "_index" || sp.Field == "_script" || sp.Field == "_shard_doc" {
			continue
		}
		for _, idx := range indices {
			if sp.Field == "_geo_distance" {
				def, ok := mapping.FieldDef(idx, sp.GeoField)
				t, _ := def["type"].(string)
				if (!ok || t != "geo_point") && !sp.GeoIgnore {
					reason := "failed to find mapper for [" + sp.GeoField + "] for geo distance based sort"
					return &apierr.Error{Type: "search_phase_execution_exception", Status: 400,
						Reason: reason, Phase: "query",
						RootCause: &apierr.Error{Type: "query_shard_exception", Reason: reason}}
				}
				continue
			}
			if sp.NestedPath != "" {
				def, ok := mapping.FieldDef(idx, sp.NestedPath)
				t, _ := def["type"].(string)
				if !ok || t != "nested" {
					reason := "[nested] failed to find nested object under path [" + sp.NestedPath + "]"
					return &apierr.Error{Type: "search_phase_execution_exception", Status: 400,
						Reason: reason, Phase: "query",
						RootCause: &apierr.Error{Type: "query_shard_exception", Reason: reason}}
				}
			}
			if sp.Mode == "avg" || sp.Mode == "sum" || sp.Mode == "median" {
				if def, ok := mapping.FieldDef(idx, sp.Field); ok {
					t, _ := def["type"].(string)
					if !mapping.IsNumericType(t) && t != "date" {
						reason := "we only support AVG, MEDIAN and SUM on number based fields"
						return &apierr.Error{Type: "search_phase_execution_exception", Status: 400,
							Reason: reason, Phase: "query",
							RootCause: &apierr.Error{Type: "query_shard_exception", Reason: reason}}
					}
				}
			}
			if def, ok := mapping.FieldDef(idx, sp.Field); ok {
				t, _ := def["type"].(string)
				fd, _ := def["fielddata"].(bool)
				if (t == "text" || t == "match_only_text") && !fd {
					reason := "Text fields are not optimised for operations that require per-document field data like aggregations and sorting, so these operations are disabled by default. Please use a keyword field instead. Alternatively, set fielddata=true on [" + sp.Field + "] in order to load field data by uninverting the inverted index. Note that this can use significant memory."
					return &apierr.Error{Type: "search_phase_execution_exception", Status: 400,
						Reason: "all shards failed", Phase: "query",
						RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason},
						CausedBy:  &apierr.Error{Type: "illegal_argument_exception", Reason: reason}}
				}
				if t == "geo_shape" {
					reason := "can't sort on geo_shape field without using specific sorting feature, like geo_distance"
					return &apierr.Error{Type: "search_phase_execution_exception", Status: 400,
						Reason: "all shards failed", Phase: "query",
						RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason},
						CausedBy:  &apierr.Error{Type: "illegal_argument_exception", Reason: reason}}
				}
				if t == "rank_feature" || t == "rank_features" {
					reason := "[rank_feature] fields do not support sorting, scripting or aggregating"
					return &apierr.Error{Type: "search_phase_execution_exception", Status: 400,
						Reason: "all shards failed", Phase: "query",
						RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason},
						CausedBy:  &apierr.Error{Type: "illegal_argument_exception", Reason: reason}}
				}
			}
			if err := CheckDocValuesLoadable(idx, sp.Field); err != nil {
				return err
			}
		}
	}
	for i := range specs {
		sp := &specs[i]
		if sp.Field != "_script" {
			continue
		}
		ctxName := "number_sort"
		if sp.ScriptType == "string" {
			ctxName = "string_sort"
		}
		cs, se, ae := PrepareScript(sp.ScriptRaw, ctxName)
		if se != nil || ae != nil {
			return ScriptFailure(indices[0], se, ae, false)
		}
		sp.ScriptVals = map[string]any{}
		for _, h := range hits {
			v, rerr := cs.Run(NewQueryCtxForDoc(h.Idx, h.Id, h.Doc), RunOpts{Score: h.Score})
			if rerr != nil {
				return ScriptShardError(h.Idx, rerr)
			}
			if ctxName == "number_sort" {
				v = painless.F64(v)
			}
			sp.ScriptVals[h.Idx.Name+"\x00"+h.Id] = v
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		for _, sp := range specs {
			c := compareSortSpec(hits[i], hits[j], sp)
			if c != 0 {
				if sp.Desc {
					return c > 0
				}
				return c < 0
			}
		}
		return false
	})
	return nil
}
