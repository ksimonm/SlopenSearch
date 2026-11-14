package search

import (
	"math"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/model"
	"slopensearch/internal/painless"
)

func (c *QueryCtx) evalScriptQuery(body any) (bool, float64, *apierr.Error) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, apierr.Parsing("[script] query malformed")
	}
	boost := 1.0
	if b, ok := jsonv.ToFloat(bm["boost"]); ok {
		boost = b
	}
	cs, se, ae := PrepareScript(bm["script"], "filter")
	if se != nil || ae != nil {
		return false, 0, ScriptErrForShard(c.Idx, se, ae, true)
	}
	v, rerr := cs.Run(c, RunOpts{})
	if rerr != nil {
		return false, 0, ScriptShardError(c.Idx, rerr)
	}
	if b, _ := v.(bool); b {
		return true, boost, nil
	}
	return false, 0, nil
}

// luceneDocNum is the position of a document in its index, which stands in
// for the Lucene doc id in error messages.
func luceneDocNum(idx *model.Index, id string) int {
	for i, d := range idx.Order {
		if d == id {
			return i
		}
	}
	return 0
}

func (c *QueryCtx) evalScriptScore(body any) (bool, float64, *apierr.Error) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, apierr.Parsing("[script_score] query malformed")
	}
	inner, ok := bm["query"].(map[string]any)
	if !ok {
		return false, 0, apierr.Parsing("[script_score] requires 'query'")
	}
	matched, base, err := c.Eval(inner)
	if err != nil || !matched {
		return false, 0, err
	}
	if bm["script"] == nil {
		return true, base, nil
	}
	if sm, ok := bm["script"].(map[string]any); ok && sm["lang"] == "knn" {
		score, kerr := c.knnScriptScore(sm)
		if kerr != nil {
			return false, 0, kerr
		}
		return knnScriptScoreFinish(bm, score)
	}
	cs, se, ae := PrepareScript(bm["script"], "score")
	if se != nil || ae != nil {
		return false, 0, ScriptErrForShard(c.Idx, se, ae, true)
	}
	v, rerr := cs.Run(c, RunOpts{Score: float64(float32(base))})
	if rerr != nil {
		return false, 0, ScriptShardError(c.Idx, rerr)
	}
	score := painless.F64(v)
	if score < 0 || math.IsNaN(score) {
		return false, 0, PlainShardError(c.Idx, apierr.BadRequest("script_score script returned an invalid score ["+
			painless.FormatDouble(score)+"] for doc ["+Itoa(luceneDocNum(c.Idx, c.ID))+"]. Must be a non-negative score!"))
	}
	score = float64(float32(score))
	if b, ok := jsonv.ToFloat(bm["boost"]); ok {
		score = float64(float32(score * b))
	}
	if ms, ok := jsonv.ToFloat(bm["min_score"]); ok && score < ms {
		return false, 0, nil
	}
	return true, score, nil
}
