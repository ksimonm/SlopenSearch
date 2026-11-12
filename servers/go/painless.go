package main

import (
	"math"
	"strconv"
)

// Script query and script_score query evaluation. The scripting engines
// live in pl_*.go (Painless) and pl_expr.go (Lucene expressions).

func isAlphaByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func valueToScriptString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case float64:
		return formatNumber(t), true
	case int64:
		return strconv.FormatInt(t, 10), true
	case bool:
		return strconv.FormatBool(t), true
	case nil:
		return "null", true
	}
	return "", false
}

func (c *queryCtx) evalScriptQuery(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[script] query malformed")
	}
	boost := 1.0
	if b, ok := toFloat(bm["boost"]); ok {
		boost = b
	}
	cs, se, ae := prepareScript(bm["script"], "filter")
	if se != nil || ae != nil {
		return false, 0, scriptErrForShard(c.idx, se, ae, true)
	}
	v, rerr := cs.run(c, runOpts{})
	if rerr != nil {
		return false, 0, scriptShardError(c.idx, rerr)
	}
	if b, _ := v.(bool); b {
		return true, boost, nil
	}
	return false, 0, nil
}

// luceneDocNum is the position of a document in its index, which stands in
// for the Lucene doc id in error messages.
func luceneDocNum(idx *Index, id string) int {
	for i, d := range idx.Order {
		if d == id {
			return i
		}
	}
	return 0
}

func (c *queryCtx) evalScriptScore(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[script_score] query malformed")
	}
	inner, ok := bm["query"].(map[string]any)
	if !ok {
		return false, 0, errParsing("[script_score] requires 'query'")
	}
	matched, base, err := c.eval(inner)
	if err != nil || !matched {
		return false, 0, err
	}
	if bm["script"] == nil {
		return true, base, nil
	}
	cs, se, ae := prepareScript(bm["script"], "score")
	if se != nil || ae != nil {
		return false, 0, scriptErrForShard(c.idx, se, ae, true)
	}
	v, rerr := cs.run(c, runOpts{score: float64(float32(base))})
	if rerr != nil {
		return false, 0, scriptShardError(c.idx, rerr)
	}
	score := plF64(v)
	if score < 0 || math.IsNaN(score) {
		return false, 0, plainShardError(c.idx, errBad("script_score script returned an invalid score ["+
			plFmtDouble(score)+"] for doc ["+itoa(luceneDocNum(c.idx, c.id))+"]. Must be a non-negative score!"))
	}
	score = float64(float32(score))
	if b, ok := toFloat(bm["boost"]); ok {
		score = float64(float32(score * b))
	}
	if ms, ok := toFloat(bm["min_score"]); ok && score < ms {
		return false, 0, nil
	}
	return true, score, nil
}
