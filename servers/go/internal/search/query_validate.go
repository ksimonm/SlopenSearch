package search

import (
	"fmt"
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
)

var knownClauses = map[string]bool{
	"match_all": true, "match_none": true, "match": true, "term": true, "terms": true,
	"ids": true, "range": true, "exists": true, "prefix": true, "wildcard": true,
	"regexp": true, "fuzzy": true, "match_phrase": true, "match_phrase_prefix": true,
	"match_bool_prefix": true, "multi_match": true, "bool": true, "constant_score": true,
	"dis_max": true, "boosting": true, "nested": true, "has_child": true, "has_parent": true,
	"parent_id":    true,
	"geo_distance": true, "geo_bounding_box": true, "geo_polygon": true, "geo_shape": true, "script": true,
	"script_score": true, "function_score": true, "query_string": true,
	"simple_query_string": true, "combined_fields": true, "more_like_this": true,
	"rank_feature": true, "distance_feature": true, "wrapper": true, "percolate": true,
	"terms_set": true, "span_term": true, "span_near": true, "span_or": true,
	"span_first": true, "span_not": true, "span_containing": true, "span_within": true,
	"span_multi": true, "field_masking_span": true, "intervals": true,
	"random_score": true, "knn": true,
}

// ValidateQuery checks a query against the mappings of idx before any
// document is evaluated (the shard-level parse errors).
func ValidateQuery(idx *model.Index, query map[string]any) *apierr.Error {
	for clause, raw := range query {
		if !knownClauses[clause] {
			return &apierr.Error{Type: "parsing_exception", Status: 400,
				Reason: "unknown query [" + clause + "]", Clause: clause}
		}
		switch clause {
		case "term":
			if err := KnnTermCheck(idx, raw); err != nil {
				return err
			}
			if err := validateTermQuery(idx, raw); err != nil {
				return err
			}
		case "knn":
			if err := ValidateKnnQuery(idx, raw); err != nil {
				return err
			}
		case "terms":
			if err := validateTermsQuery(raw); err != nil {
				return err
			}
		case "range":
			if err := validateRangeQuery(idx, raw); err != nil {
				return err
			}
		case "bool":
			if bm, ok := raw.(map[string]any); ok {
				for _, key := range []string{"must", "should", "filter", "must_not"} {
					for _, q := range AsQueryList(bm[key]) {
						if err := validateNestedQuery(idx, q); err != nil {
							return err
						}
					}
				}
			}
		case "constant_score":
			if bm, ok := raw.(map[string]any); ok {
				if err := validateNestedQuery(idx, bm["filter"]); err != nil {
					return err
				}
			}
		case "dis_max":
			if bm, ok := raw.(map[string]any); ok {
				queries := AsQueryList(bm["queries"])
				if len(queries) == 0 {
					return &apierr.Error{Type: "parsing_exception", Status: 400, Clause: "queries",
						NoCausedBy: true,
						Reason:     "[_na] query malformed, must start with start_object"}
				}
				for _, q := range queries {
					if err := validateNestedQuery(idx, q); err != nil {
						return err
					}
				}
			}
		case "boosting":
			if bm, ok := raw.(map[string]any); ok {
				if _, has := bm["negative_boost"]; !has {
					return &apierr.Error{Type: "parsing_exception", Status: 400, Clause: "boosting",
						NoCausedBy: true,
						Reason:     "[boosting] query requires 'negative_boost' to be set to be a positive value'"}
				}
				for _, key := range []string{"positive", "negative"} {
					if err := validateNestedQuery(idx, bm[key]); err != nil {
						return err
					}
				}
			}
		case "more_like_this":
			if bm, ok := raw.(map[string]any); ok {
				if _, has := bm["like"]; !has {
					return &apierr.Error{Type: "parsing_exception", Status: 400,
						Reason: "more_like_this requires 'like' to be specified",
						Clause: "more_like_this", ClauseEnd: true, NoCausedBy: true}
				}
			}
		case "function_score":
			if err := validateFunctionScore(idx, raw); err != nil {
				return err
			}
			if bm, ok := raw.(map[string]any); ok {
				if err := validateNestedQuery(idx, bm["query"]); err != nil {
					return err
				}
			}
		case "percolate":
			if bm, ok := raw.(map[string]any); ok {
				field := PercolateFieldName(bm)
				def, mapped := mapping.FieldDef(idx, field)
				ftype := ""
				if mapped {
					ftype, _ = def["type"].(string)
				}
				if ftype != "percolator" {
					if ftype == "" {
						ftype = "object"
					}
					return MatchShardError(
						fmt.Sprintf("expected field [%s] to be of type [percolator], but is of type [%s]", field, ftype))
				}
			}
		case "script_score", "nested", "has_child", "has_parent", "wrapper":
			if bm, ok := raw.(map[string]any); ok {
				if err := validateNestedQuery(idx, bm["query"]); err != nil {
					return err
				}
			}
		case "span_near", "span_or":
			if bm, ok := raw.(map[string]any); ok {
				for _, q := range AsQueryList(bm["clauses"]) {
					if err := validateNestedQuery(idx, q); err != nil {
						return err
					}
				}
			}
		case "span_not", "span_containing", "span_within":
			if bm, ok := raw.(map[string]any); ok {
				for _, key := range []string{"include", "exclude", "little", "big"} {
					if err := validateNestedQuery(idx, bm[key]); err != nil {
						return err
					}
				}
			}
		case "span_multi", "field_masking_span":
			if bm, ok := raw.(map[string]any); ok {
				if err := validateNestedQuery(idx, bm["match"]); err != nil {
					return err
				}
				if err := validateNestedQuery(idx, bm["query"]); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateNestedQuery(idx *model.Index, raw any) *apierr.Error {
	if qm, ok := raw.(map[string]any); ok {
		return ValidateQuery(idx, qm)
	}
	return nil
}

func validateFunctionScore(idx *model.Index, raw any) *apierr.Error {
	bm, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	if sm, ok := bm["score_mode"].(string); ok {
		switch sm {
		case "multiply", "sum", "avg", "max", "min", "first":
		default:
			return &apierr.Error{Type: "illegal_argument_exception", Status: 400, ForceTop: true,
				Reason: "No enum constant org.opensearch.common.lucene.search.function.FunctionScoreQuery.ScoreMode." +
					strings.ToUpper(sm)}
		}
	}
	if bmMode, ok := bm["boost_mode"].(string); ok {
		switch bmMode {
		case "multiply", "replace", "sum", "avg", "max", "min":
		default:
			return &apierr.Error{Type: "illegal_argument_exception", Status: 400, ForceTop: true,
				Reason: "No enum constant org.opensearch.common.lucene.search.function.FunctionScoreQuery.BoostMode." +
					strings.ToUpper(bmMode)}
		}
	}
	functions := AsQueryList(bm["functions"])
	if len(functions) == 0 {
		if f, ok := bm["function"].(map[string]any); ok {
			functions = []any{f}
		} else if HasScoreFunctionKey(bm) {
			functions = []any{bm}
		}
	}
	for _, fnRaw := range functions {
		fn, ok := fnRaw.(map[string]any)
		if !ok {
			continue
		}
		if w, ok := fn["weight"]; ok {
			if f, ok := jsonv.ToFloat(w); ok && f < 0 {
				return &apierr.Error{Type: "illegal_argument_exception", Status: 400, ForceTop: true,
					Reason: "[weight] cannot be negative for a filtering function"}
			}
		}
		if fvf, ok := fn["field_value_factor"].(map[string]any); ok {
			field, _ := fvf["field"].(string)
			if _, mapped := mapping.FieldDef(idx, field); !mapped {
				if _, hasMissing := fvf["missing"]; !hasMissing {
					return NewFunctionScoreFailure("fvf_unmapped",
						"Unable to find a field mapper for field ["+field+"]. No 'missing' value defined.")
				}
			}
		}
		if filter, ok := fn["filter"]; ok {
			if err := validateNestedQuery(idx, filter); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateTermsQuery(raw any) *apierr.Error {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	if _, ok := m["case_insensitive"]; ok {
		return &apierr.Error{Type: "parsing_exception", Status: 400,
			Reason: "[terms] query does not support [case_insensitive]",
			Clause: "case_insensitive", NoCausedBy: true}
	}
	field := ""
	var val any
	for k, v := range m {
		if k == "boost" || k == "_name" {
			continue
		}
		field, val = k, v
		break
	}
	if field == "" {
		return nil
	}
	if _, isArr := val.([]any); !isArr {
		if _, isObj := val.(map[string]any); !isObj {
			return &apierr.Error{Type: "parsing_exception", Status: 400,
				Reason: "[terms] query does not support [" + field + "]",
				Clause: field, NoCausedBy: true}
		}
	}
	return nil
}

func validateTermQuery(idx *model.Index, raw any) *apierr.Error {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	field, val, ok := SingleEntry(m)
	if !ok {
		return nil
	}
	if vm, ok := val.(map[string]any); ok {
		for k := range vm {
			switch k {
			case "value", "boost", "case_insensitive", "_name", "rewrite":
			default:
				return &apierr.Error{Type: "parsing_exception", Status: 400,
					Reason: "[term] query does not support [" + k + "]",
					Clause: k, NoCausedBy: true}
			}
		}
		if ci, has := vm["case_insensitive"]; has {
			if _, isBool := ci.(bool); !isBool {
				return &apierr.Error{Type: "illegal_argument_exception", Status: 400,
					Reason:   "Failed to parse value [" + mapping.PreviewValue(ci) + "] as only [true] or [false] are allowed.",
					ForceTop: true}
			}
		}
	}
	def, ok := mapping.FieldDef(idx, field)
	if !ok {
		return nil
	}
	ftype, _ := def["type"].(string)
	if ftype == "" {
		return nil
	}
	if vm, ok := val.(map[string]any); ok {
		v, has := vm["value"]
		if !has {
			return nil
		}
		val = v
	}
	if val == nil {
		return nil
	}
	if ftype == "boolean" {
		if _, isBool := val.(bool); !isBool {
			s, isStr := jsonv.ScalarString(val)
			if !isStr || (s != "true" && s != "false") {
				reason := fmt.Sprintf("Failed to parse value [%s] as only [true] or [false] are allowed.", mapping.PreviewValue(val))
				return &apierr.Error{Type: "mapper_parsing_exception", Status: 400,
					Reason:   reason,
					CausedBy: &apierr.Error{Type: "illegal_argument_exception", Status: 400, Reason: reason}}
			}
		}
	}
	_, err := ParseTermValue(ftype, def, val)
	if err != nil && (ftype == "date" || ftype == "date_nanos") {
		return TermDateParseError(err)
	}
	return err
}

func validateRangeQuery(idx *model.Index, raw any) *apierr.Error {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	field, condRaw, ok := SingleEntry(m)
	if !ok {
		return nil
	}
	def, ok := mapping.FieldDef(idx, field)
	if !ok {
		return nil
	}
	ftype, _ := def["type"].(string)
	cond, ok := condRaw.(map[string]any)
	if !ok || ftype == "" {
		return nil
	}
	tz, _ := cond["time_zone"].(string)
	format, _ := cond["format"].(string)
	lower, upper := RangeQueryBounds(cond)
	bounds := []struct {
		spec    RangeBoundSpec
		roundUp bool
	}{
		{lower, !lower.Incl},
		{upper, upper.Incl},
	}
	for _, b := range bounds {
		if !b.spec.Present {
			continue
		}
		bound := b.spec.Value
		var err *apierr.Error
		switch ftype {
		case "date", "date_nanos", "date_range":
			_, err = ParseRangeDateBound(def, bound, tz, format, b.roundUp)
			if err != nil && err.RootCause != nil {
				err = err.RootCause
			}
		case "ip", "ip_range":
			ctx := &mapping.ParseContext{IndexCoerce: true}
			_, err = ctx.ParseIPValue(field, bound)
		case "integer", "long", "short", "byte", "unsigned_long":
			ctx := &mapping.ParseContext{IndexCoerce: true}
			if IsFractionalNumber(bound) {
				_, err = ctx.ParseFloatValue(field, ftype, def, bound)
			} else {
				_, err = ctx.ParseIntValue(field, ftype, def, bound)
			}
		case "float", "double", "half_float", "scaled_float":
			ctx := &mapping.ParseContext{IndexCoerce: true}
			_, err = ctx.ParseFloatValue(field, ftype, def, bound)
		case "integer_range", "long_range":
			ctx := &mapping.ParseContext{IndexCoerce: true}
			if IsFractionalNumber(bound) {
				_, err = ctx.ParseFloatValue(field, strings.TrimSuffix(ftype, "_range"), def, bound)
			} else {
				_, err = ctx.ParseIntValue(field, strings.TrimSuffix(ftype, "_range"), def, bound)
			}
		case "float_range", "double_range":
			ctx := &mapping.ParseContext{IndexCoerce: true}
			_, err = ctx.ParseFloatValue(field, strings.TrimSuffix(ftype, "_range"), def, bound)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
