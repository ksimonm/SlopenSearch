package search

import (
	"fmt"
	"strings"

	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
)

// BuildProfile renders the profile section of a search response.
func BuildProfile(query map[string]any, body map[string]any, idxName string) map[string]any {
	q := profileQuery(query)
	aggs := []any{}
	if am, ok := body["aggs"].(map[string]any); ok {
		for name := range am {
			aggs = append(aggs, map[string]any{"description": name, "time_in_nanos": int64(1),
				"breakdown": map[string]any{"build_aggregation": int64(1), "collect": int64(0)}})
		}
	}
	return map[string]any{"shards": []any{map[string]any{
		"id": "[" + idxName + "][0]",
		"searches": []any{map[string]any{
			"query":        []any{q},
			"rewrite_time": int64(0),
			"collector":    []any{map[string]any{"name": "SimpleTopScoreDocCollector", "reason": "search_top_hits", "time_in_nanos": int64(1)}},
		}},
		"aggregations": aggs,
		"fetch":        []any{map[string]any{"type": "fetch", "description": "fetch"}},
	}}}
}

func profileQuery(query map[string]any) map[string]any {
	base := map[string]any{"time_in_nanos": int64(1), "breakdown": map[string]any{
		"create_weight": int64(1), "build_scorer": int64(1), "next_doc": int64(1), "score": int64(1), "advance": int64(1), "match": int64(1),
	}}
	if m, ok := query["match"].(map[string]any); ok {
		for field, raw := range m {
			text, _ := QueryText(raw)
			term := ""
			if toks := mapping.Analyze(text); len(toks) > 0 {
				term = toks[0]
			}
			base["type"] = "TermQuery"
			base["description"] = field + ":" + term
			return base
		}
	}
	if m, ok := query["term"].(map[string]any); ok {
		for field, raw := range m {
			val := raw
			if vm, ok := raw.(map[string]any); ok {
				val = vm["value"]
			}
			s, _ := jsonv.ToString(val)
			base["type"] = "TermQuery"
			base["description"] = field + ":" + s
			return base
		}
	}
	if bm, ok := query["bool"].(map[string]any); ok {
		parts := []string{}
		children := []any{}
		for _, item := range AsQueryList(bm["must"]) {
			if cm, ok := item.(map[string]any); ok {
				ch := profileQuery(cm)
				if _, isTerm := cm["term"]; isTerm {
					parts = append(parts, "+ConstantScore("+fmt.Sprint(ch["description"])+")")
					ch["type"] = "ConstantScoreQuery"
				} else {
					parts = append(parts, "+"+fmt.Sprint(ch["description"]))
				}
				children = append(children, ch)
			}
		}
		for _, item := range AsQueryList(bm["filter"]) {
			if cm, ok := item.(map[string]any); ok {
				ch := profileQuery(cm)
				parts = append(parts, "+ConstantScore("+fmt.Sprint(ch["description"])+")")
				ch["type"] = "ConstantScoreQuery"
				children = append(children, ch)
			}
		}
		base["type"] = "BooleanQuery"
		base["description"] = strings.Join(parts, " ")
		base["children"] = children
		return base
	}
	if _, ok := query["match_all"]; ok {
		base["type"] = "MatchAllDocsQuery"
		base["description"] = "match_all"
		return base
	}
	base["type"] = "TermQuery"
	base["description"] = ""
	return base
}
