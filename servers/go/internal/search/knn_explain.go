package search

import (
	"fmt"
	"strings"

	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
	"slopensearch/internal/ojson"
)

// ---------------------------------------------------------------------------
// knn on nested fields: k counts parent documents
// ---------------------------------------------------------------------------

// KnnExplain explains a top-level knn query hit, or returns nil.
func KnnExplain(idx *model.Index, query map[string]any, id string, score float64) any {
	kq, ok := query["knn"].(map[string]any)
	if !ok || len(query) != 1 || len(kq) != 1 {
		return nil
	}
	for field, raw := range kq {
		p, ok := raw.(map[string]any)
		if !ok {
			return nil
		}
		k, hasK := jsonv.ToFloat(p["k"])
		if !hasK {
			return nil
		}
		def, _ := knnFieldDef(idx, field)
		fi := mapping.KnnInfo(def)
		if fi.Engine == "lucene" {
			return ojson.Object{{"value", ScoreJSON(score)},
				{"description", fmt.Sprintf("within top %d docs", int(k))}, {"details", []any{}}}
		}
		qv, _ := floatList(p["vector"])
		qc := &QueryCtx{Idx: idx, ID: id, Doc: idx.Docs[id]}
		dv, _ := qc.docVector(field)
		raw := 0.0
		formula := "`1 / (1 + rawScore)`"
		if len(dv) == len(qv) {
			raw, _ = knnDistanceScore(fi.Space, qv, dv)
		}
		switch fi.Space {
		case "innerproduct":
			raw = -raw
			formula = "`rawScore >= 0 ? 1 + rawScore : 1 / (1 - rawScore)`"
		case "cosinesimil":
			raw = 1 - raw
			formula = "`(1 + rawScore) / 2`"
		}
		return ojson.Object{{"value", ScoreJSON(score)},
			{"description", "the type of knn search executed was Approximate-NN"},
			{"details", []any{ojson.Object{{"value", ScoreJSON(score)},
				{"description", fmt.Sprintf("the type of knn search executed at leaf was Approximate-NN with vectorDataType = %s, spaceType = %s where score is computed as %s from:",
					strings.ToUpper(fi.DataType), fi.Space, formula)},
				{"details", []any{ojson.Object{{"value", ScoreJSON(float64(float32(raw)))},
					{"description", "rawScore, returned from FAISS library"}, {"details", []any{}}}}}}}}}
	}
	return nil
}
