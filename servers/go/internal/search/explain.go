package search

import (
	"fmt"
	"math"

	"slopensearch/internal/analysis"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
)

func (c *QueryCtx) Bm25Explain(field, term string, score float64) map[string]any {
	desc := fmt.Sprintf("weight(%s:%s in 0) [PerFieldSimilarity], result of:", field, term)
	generic := map[string]any{
		"value":       score,
		"description": desc,
		"details": []any{map[string]any{
			"value":       score,
			"description": "sum of:",
			"details":     []any{},
		}},
	}
	if c.Idx == nil || field == "" || term == "" {
		return generic
	}
	def, _ := mapping.FieldDef(c.Idx, field)
	ftype, _ := def["type"].(string)
	if ftype != "" && ftype != "text" && ftype != "match_only_text" && ftype != "search_as_you_type" {
		return generic
	}
	analyzer := c.indexAnalyzerName(field)
	docTerms := func(vals []any) []string {
		out := []string{}
		for _, v := range vals {
			s, _ := jsonv.ScalarString(v)
			for _, t := range analysis.Tokens(mapping.AnalysisIndex(c.Idx), analyzer, s) {
				out = append(out, t.Term)
			}
		}
		return out
	}
	myVals := []any{}
	if dv, ok := c.rawValues(); ok {
		myVals = dv[mapping.ResolveAliasField(c.Idx, field)]
	}
	myTokens := docTerms(myVals)
	freq := 0
	for _, t := range myTokens {
		if t == term {
			freq++
		}
	}
	N := 0
	avgdl := 0.0
	df := 0
	for _, dv := range c.Idx.DocValues {
		vals := dv[mapping.ResolveAliasField(c.Idx, field)]
		if len(vals) == 0 {
			continue
		}
		N++
		toks := docTerms(vals)
		avgdl += float64(len(toks))
		seen := false
		for _, t := range toks {
			if t == term {
				seen = true
				break
			}
		}
		if seen {
			df++
		}
	}
	if N == 0 || freq == 0 {
		return generic
	}
	avgdl /= float64(N)
	if avgdl == 0 {
		avgdl = 1
	}
	const k1 = 1.2
	const b = 0.75
	dl := float64(len(myTokens))
	idf := math.Log(1 + (float64(N)-float64(df)+0.5)/(float64(df)+0.5))
	tf := float64(freq) / (float64(freq) + k1*(1-b+b*dl/avgdl))
	inner := map[string]any{
		"value":       score,
		"description": fmt.Sprintf("score(freq=%.1f), computed as boost * idf * tf from:", float64(freq)),
		"details": []any{
			map[string]any{
				"value":       idf,
				"description": "idf, computed as log(1 + (N - n + 0.5) / (n + 0.5)) from:",
				"details": []any{
					map[string]any{"value": float64(df), "description": "n, number of documents containing term", "details": []any{}},
					map[string]any{"value": float64(N), "description": "N, total number of documents with field", "details": []any{}},
				},
			},
			map[string]any{
				"value":       tf,
				"description": "tf, computed as freq / (freq + k1 * (1 - b + b * dl / avgdl)) from:",
				"details": []any{
					map[string]any{"value": k1, "description": "k1, term saturation parameter", "details": []any{}},
					map[string]any{"value": b, "description": "b, length normalization parameter", "details": []any{}},
					map[string]any{"value": dl, "description": "dl, length of field", "details": []any{}},
					map[string]any{"value": avgdl, "description": "avgdl, average length of field", "details": []any{}},
				},
			},
		},
	}
	return map[string]any{"value": score, "description": desc, "details": []any{inner}}
}
