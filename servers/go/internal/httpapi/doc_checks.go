package httpapi

import (
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
	"slopensearch/internal/search"
)

func mappingShapeConflict(props map[string]any, doc map[string]any) *apierr.Error {
	for k, v := range doc {
		def, ok := props[k].(map[string]any)
		if !ok {
			continue
		}
		if _, isObj := def["properties"].(map[string]any); !isObj {
			continue
		}
		switch t := v.(type) {
		case map[string]any:
			if err := mappingShapeConflict(def["properties"].(map[string]any), t); err != nil {
				return err
			}
		case []any:
			for _, e := range t {
				if m, ok := e.(map[string]any); ok {
					if err := mappingShapeConflict(def["properties"].(map[string]any), m); err != nil {
						return err
					}
				}
			}
		case nil:
		default:
			return &apierr.Error{Type: "mapper_parsing_exception", Status: 400,
				Reason: "object mapping for [" + k + "] tried to parse field [" + k + "] as object, but found a concrete value"}
		}
	}
	return nil
}

var percolatorReservedKeys = map[string]bool{
	"boost": true, "_name": true, "case_insensitive": true, "rewrite": true, "flags": true,
	"max_expansions": true, "fuzziness": true, "prefix_length": true, "transpositions": true,
	"operator": true, "analyzer": true, "minimum_should_match": true, "zero_terms_query": true,
	"cutoff_frequency": true, "lenient": true, "fuzzy_transpositions": true,
	"auto_generate_synonyms_phrase_query": true, "type": true, "id": true, "path": true,
	"indexed_shape": true, "shape": true, "relation": true, "ignore_unmapped": true,
	"validation_method": true, "ignore_malformed": true, "value": true, "values": true,
	"query": true, "fields": true, "default_field": true, "field": true, "document": true,
	"documents": true, "index": true, "name": true, "slop": true, "in_order": true,
	"score_mode": true, "inner_hits": true, "sort": true, "size": true, "from": true,
	"_source": true, "time_zone": true, "format": true, "locale": true,
	"minimum_should_match_script": true, "boost_mode": true, "functions": true,
	"random_score": true, "weight": true, "field_value_factor": true, "script_score": true,
	"gauss": true, "exp": true, "linear": true, "origin": true, "scale": true, "offset": true,
	"decay": true, "missing": true, "factor": true, "modifier": true, "seed": true,
	"multi": true, "minimum_number_should_match": true, "tie_breaker": true, "disable_coord": true,
	"positive": true, "negative": true, "negative_boost": true, "adjust_pure_negative": true,
	"should": true, "must": true, "must_not": true, "filter": true, "queries": true,
	"clauses": true, "include": true, "exclude": true, "little": true, "big": true,
	"big_field": true, "little_field": true, "positive_boost": true, "negative_boost_boost": true,
	"distance": true, "distance_type": true, "unit": true, "validation_methods": true,
	"top_left": true, "bottom_right": true, "points": true, "ignore_z_value": true,
	"orientation": true, "boost_mode_boost": true, "span_term": true, "script": true,
	"params": true, "source": true, "lang": true, "organic": true, "docs": true,
}

func percolatorFieldRefs(q map[string]any, out *[]string) {
	for clause, raw := range q {
		bm, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch clause {
		case "bool":
			for _, key := range []string{"must", "should", "filter", "must_not"} {
				for _, item := range search.AsQueryList(bm[key]) {
					if child, ok := item.(map[string]any); ok {
						percolatorFieldRefs(child, out)
					}
				}
			}
		case "constant_score":
			if child, ok := bm["filter"].(map[string]any); ok {
				percolatorFieldRefs(child, out)
			}
		case "dis_max":
			for _, item := range search.AsQueryList(bm["queries"]) {
				if child, ok := item.(map[string]any); ok {
					percolatorFieldRefs(child, out)
				}
			}
		case "boosting":
			for _, key := range []string{"positive", "negative"} {
				if child, ok := bm[key].(map[string]any); ok {
					percolatorFieldRefs(child, out)
				}
			}
		case "function_score", "script_score", "wrapper":
			if child, ok := bm["query"].(map[string]any); ok {
				percolatorFieldRefs(child, out)
			}
		case "nested", "has_child", "has_parent":
			if child, ok := bm["query"].(map[string]any); ok {
				percolatorFieldRefs(child, out)
			}
		case "span_near", "span_or":
			for _, item := range search.AsQueryList(bm["clauses"]) {
				if child, ok := item.(map[string]any); ok {
					percolatorFieldRefs(child, out)
				}
			}
		case "span_not", "span_containing", "span_within":
			for _, key := range []string{"include", "exclude", "little", "big"} {
				if child, ok := bm[key].(map[string]any); ok {
					percolatorFieldRefs(child, out)
				}
			}
		case "exists":
			if f, ok := bm["field"].(string); ok {
				*out = append(*out, f)
			}
		case "multi_match", "query_string", "simple_query_string", "combined_fields", "more_like_this":
			if raw, ok := bm["fields"].([]any); ok {
				for _, e := range raw {
					if s, ok := e.(string); ok {
						if i := strings.Index(s, "^"); i >= 0 {
							s = s[:i]
						}
						if i := strings.Index(s, ":"); i > 0 {
							s = s[i+1:]
						}
						*out = append(*out, s)
					}
				}
			}
			if f, ok := bm["default_field"].(string); ok && f != "" {
				*out = append(*out, f)
			}
		case "ids", "match_all", "match_none", "script", "percolate", "knn":
		default:
			for k := range bm {
				if !percolatorReservedKeys[k] {
					*out = append(*out, k)
				}
			}
		}
	}
}

func percolatorFields(idx *model.Index) []string {
	props, _ := idx.Mappings["properties"].(map[string]any)
	var out []string
	for name, raw := range props {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := def["type"].(string); t == "percolator" {
			out = append(out, name)
		}
	}
	return out
}

func percolatorParseError(reason string, fields map[string]any, caused *apierr.Error) *apierr.Error {
	cause := &apierr.Error{Type: "parsing_exception", Status: 400, Reason: reason, Fields: fields}
	if caused != nil {
		cause = caused
	}
	root := &apierr.Error{Type: cause.Type, Status: cause.Status, Reason: cause.Reason, Fields: cause.Fields}
	return &apierr.Error{Type: "mapper_parsing_exception", Status: 400, Reason: "failed to parse",
		RootCause: root, CausedBy: cause, Fields: cause.Fields}
}

func validatePercolatorDocs(idx *model.Index, body map[string]any, raw []byte) *apierr.Error {
	fields := percolatorFields(idx)
	for _, field := range fields {
		rawQ, ok := body[field]
		if !ok {
			continue
		}
		qm, ok := rawQ.(map[string]any)
		if !ok {
			col := len(raw)
			if col == 0 {
				col = 1
			}
			return percolatorParseError("[_na] query malformed, must start with start_object",
				map[string]any{"line": 1, "col": col}, nil)
		}
		if verr := search.ValidateQuery(idx, qm); verr != nil {
			if verr.Clause != "" {
				annotateQueryClause(verr, raw)
			}
			root := &apierr.Error{Type: verr.Type, Status: verr.Status, Reason: verr.Reason, Fields: verr.Fields}
			return &apierr.Error{Type: "mapper_parsing_exception", Status: 400, Reason: "failed to parse",
				RootCause: root, CausedBy: verr, Fields: verr.Fields}
		}
		var refs []string
		percolatorFieldRefs(qm, &refs)
		for _, f := range refs {
			if f == "_id" || f == "_index" || f == "_routing" {
				continue
			}
			if _, mapped := mapping.FieldDef(idx, f); !mapped {
				reason := "No field mapping can be found for the field with name [" + f + "]"
				cause := &apierr.Error{Type: "query_shard_exception", Status: 400, Reason: reason,
					Fields: map[string]any{"index": idx.Name, "index_uuid": idx.UUID}}
				return &apierr.Error{Type: "mapper_parsing_exception", Status: 400, Reason: "failed to parse",
					RootCause: cause, CausedBy: cause, Fields: cause.Fields}
			}
		}
	}
	return nil
}

func joinDocError(reason string) *apierr.Error {
	return &apierr.Error{Type: "mapper_parsing_exception", Reason: "failed to parse", Status: 400,
		CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: reason}}
}

func validateJoinDoc(idx *model.Index, body map[string]any, routing string) *apierr.Error {
	field, relations, hasJoin := search.FindJoinField(idx)
	if !hasJoin {
		return nil
	}
	raw, ok := body[field]
	if !ok {
		return nil
	}
	var name, parent string
	switch t := raw.(type) {
	case string:
		name = t
	case map[string]any:
		name, _ = t["name"].(string)
		parent, _ = t["parent"].(string)
	default:
		return nil
	}
	if name == "" {
		return joinDocError("join name is missing")
	}
	if !search.JoinValidName(relations, name) {
		return joinDocError("unknown join name [" + name + "] for field [" + field + "]")
	}
	if _, isParent := relations[name]; isParent {
		return nil
	}
	if parent == "" {
		return joinDocError("[parent] is missing for join field [" + field + "]")
	}
	if routing == "" {
		return joinDocError("[routing] is missing for join field [" + field + "]")
	}
	return nil
}
