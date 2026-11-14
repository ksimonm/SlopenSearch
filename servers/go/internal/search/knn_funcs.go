package search

import (
	"slopensearch/internal/apierr"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
	"slopensearch/internal/painless"
)

// ---------------------------------------------------------------------------
// knn_score script (lang "knn") and the painless vector functions
// ---------------------------------------------------------------------------

// ---- painless ----

var plKnnFuncs = painless.ParseSigs(
	"l2Squared:float(def,def)", "l1Norm:float(def,def)", "lInfNorm:float(def,def)",
	"cosineSimilarity:float(def,def)", "innerProduct:float(def,def)", "hamming:float(def,def)",
)

func init() {
	for k, v := range plKnnFuncs {
		painless.ScoreFuncs[k] = v
	}
}

// ---- unsupported operations on vector fields ----

// CheckKnnSort reports a sort on a knn_vector field.
func CheckKnnSort(idx *model.Index, field string) *apierr.Error {
	def, _ := mapping.FieldDef(idx, field)
	if t, _ := def["type"].(string); t != "knn_vector" {
		return nil
	}
	e := &apierr.Error{Type: "unsupported_operation_exception", Status: 500, Reason: "knn vector field doesn't support this operation"}
	return ShardError(idx, e)
}
