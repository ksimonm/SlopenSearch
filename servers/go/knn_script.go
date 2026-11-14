package main

import (
	"fmt"
	"math"
	"math/bits"
	"strings"
)

// ---------------------------------------------------------------------------
// knn_score script (lang "knn") and the painless vector functions
// ---------------------------------------------------------------------------

// knnShardFail renders a "failed to create query" shard failure whose
// cause is an illegal_argument_exception.
func knnShardFail(idx *Index, reason string) *apiError {
	return knnShardFailWith(idx, "failed to create query: "+reason, orderedJSON{
		{"type", "illegal_argument_exception"}, {"reason", reason}}, true)
}

// knnShardFailWith builds a query_shard_exception shard failure with the
// given shard reason and cause; rootIsShard picks whether root_cause and the
// top-level cause are the query_shard_exception or the inner cause.
func knnShardFailWith(idx *Index, shardReason string, cause orderedJSON, rootIsShard bool) *apiError {
	shard := orderedJSON{{"type", "query_shard_exception"}, {"reason", shardReason},
		{"index", idx.Name}, {"index_uuid", idx.UUID}, {"caused_by", cause}}
	root := orderedJSON{{"type", "query_shard_exception"}, {"reason", shardReason},
		{"index", idx.Name}, {"index_uuid", idx.UUID}}
	top := shard
	if !rootIsShard {
		root = cause
		top = cause
	}
	f := &shardFailure{status: 400, index: idx.Name, root: root, shard: shard, top: top}
	return &apiError{Type: "query_shard_exception", Status: 400, Reason: shardReason, Shard: f}
}

var knnScriptSpaces = map[string]bool{
	"l2": true, "l1": true, "linf": true, "cosinesimil": true, "innerproduct": true, "hamming": true,
}

// knnScriptScore computes a knn_score script score for the current document.
func (c *queryCtx) knnScriptScore(sm map[string]any) (float64, *apiError) {
	src, _ := sm["source"].(string)
	if src != "knn_score" {
		return 0, knnShardFailWith(c.idx, fmt.Sprintf("failed to create query: Failed to compile inline script [%s] using lang [knn]", src),
			orderedJSON{{"type", "illegal_argument_exception"}, {"reason", "Unknown script name " + src}}, false)
	}
	params, _ := sm["params"].(map[string]any)
	for _, p := range []string{"field", "query_value", "space_type"} {
		if _, ok := params[p]; !ok {
			return 0, knnShardFail(c.idx, "Missing parameter ["+p+"]")
		}
	}
	field, _ := params["field"].(string)
	space, _ := params["space_type"].(string)
	qv, _ := floatList(params["query_value"])
	if !knnScriptSpaces[space] {
		return 0, knnShardFail(c.idx, "Invalid space type. Please refer to the available space types.")
	}
	def, _ := knnFieldDef(c.idx, field)
	if t, _ := def["type"].(string); t != "knn_vector" {
		return 0, knnShardFail(c.idx, fmt.Sprintf("Incompatible field_type for %s space. The field type must be knn_vector.", space))
	}
	fi := knnInfo(def)
	dt := strings.ToUpper(fi.dataType)
	if space == "hamming" && dt != "BINARY" {
		return 0, knnShardFail(c.idx, "Incompatible field_type for hamming space. The data type should be [BINARY] but got "+dt)
	}
	if space != "hamming" && dt == "BINARY" {
		return 0, knnShardFail(c.idx, fmt.Sprintf("Incompatible field_type for %s space. The data type should be [HALF_FLOAT, BYTE, FLOAT] but got BINARY", space))
	}
	want := fi.dim
	if dt == "BINARY" {
		want = fi.dim / 8
	}
	if len(qv) != want {
		return 0, knnShardFail(c.idx, fmt.Sprintf("Object's length=%d does not match the expected length=%d.", len(qv), want))
	}
	dv, ok := c.docVector(field)
	if !ok || len(dv) != len(qv) {
		return 0, nil
	}
	if space == "l1" || space == "linf" {
		d, _ := knnDistanceScore(space, qv, dv)
		return float64(float32(1 / (1 + d))), nil
	}
	_, s := knnDistanceScore(space, qv, dv)
	return s, nil
}

// ---- painless ----

// plKnnDocValues builds doc['field'] for a knn_vector field: one float[]
// value holding the vector.
func plKnnDocValues(field string, vals []any) *plDocValues {
	dv := &plDocValues{kind: "KNNVector", field: field}
	if len(vals) == 0 {
		return dv
	}
	arr := &plArray{elem: "float"}
	for _, v := range vals {
		f, _ := toFloat(v)
		arr.a = append(arr.a, float32(f))
	}
	dv.vals = []any{arr}
	return dv
}

var plKnnFuncs = plParseSigs(
	"l2Squared:float(def,def)", "l1Norm:float(def,def)", "lInfNorm:float(def,def)",
	"cosineSimilarity:float(def,def)", "innerProduct:float(def,def)", "hamming:float(def,def)",
)

func init() {
	for k, v := range plKnnFuncs {
		plScoreFuncs[k] = v
	}
}

func plKnnRtErr(off int, typ, reason string) *plRtErr {
	return &plRtErr{typ: typ, offset: off, reason: reason}
}

// plKnnArgs extracts the query vector and the document vector.
func plKnnArgs(a []any, off int) ([]float64, []float64, *plRtErr) {
	items, ok := plToItems(a[0])
	if !ok {
		return nil, nil, plKnnRtErr(off, "class_cast_exception", "Cannot cast query vector to java.util.List")
	}
	q := make([]float64, 0, len(items))
	for _, it := range items {
		q = append(q, plF64(it))
	}
	dv, ok := a[1].(*plDocValues)
	if !ok || dv.kind != "KNNVector" {
		return nil, nil, plKnnRtErr(off, "class_cast_exception",
			"class org.opensearch.index.fielddata.ScriptDocValues cannot be cast to class org.opensearch.knn.index.KNNVectorScriptDocValues")
	}
	if len(dv.vals) == 0 {
		return nil, nil, plKnnRtErr(off, "illegal_state_exception",
			"No value found for the field ["+dv.field+"] in the document")
	}
	arr := dv.vals[0].(*plArray)
	d := make([]float64, 0, len(arr.a))
	for _, v := range arr.a {
		d = append(d, plF64(v))
	}
	if len(q) != len(d) {
		return nil, nil, plKnnRtErr(off, "illegal_argument_exception",
			fmt.Sprintf("Number of dimensions of query vector [%d] doesn't match the number of dimensions of document vector [%d]", len(q), len(d)))
	}
	return q, d, nil
}

func plKnnBindings() map[string]func(args []any, off int) (any, *plRtErr) {
	wrap := func(fn func(q, d []float64) float64) func(a []any, off int) (any, *plRtErr) {
		return func(a []any, off int) (any, *plRtErr) {
			q, d, err := plKnnArgs(a, off)
			if err != nil {
				return nil, err
			}
			return float32(fn(q, d)), nil
		}
	}
	return map[string]func(args []any, off int) (any, *plRtErr){
		"l2Squared": wrap(func(q, d []float64) float64 {
			s := float32(0)
			for i := range q {
				x := float32(q[i]) - float32(d[i])
				s += x * x
			}
			return float64(s)
		}),
		"l1Norm": wrap(func(q, d []float64) float64 {
			s := float32(0)
			for i := range q {
				s += float32(math.Abs(float64(float32(q[i]) - float32(d[i]))))
			}
			return float64(s)
		}),
		"lInfNorm": wrap(func(q, d []float64) float64 {
			s := float32(0)
			for i := range q {
				s = float32(math.Max(float64(s), math.Abs(float64(float32(q[i])-float32(d[i])))))
			}
			return float64(s)
		}),
		"innerProduct": wrap(func(q, d []float64) float64 {
			s := float32(0)
			for i := range q {
				s += float32(q[i]) * float32(d[i])
			}
			return float64(s)
		}),
		"cosineSimilarity": wrap(func(q, d []float64) float64 {
			dot, na, nb := float32(0), float32(0), float32(0)
			for i := range q {
				dot += float32(q[i]) * float32(d[i])
				na += float32(q[i]) * float32(q[i])
				nb += float32(d[i]) * float32(d[i])
			}
			return float64(dot) / math.Sqrt(float64(na)*float64(nb))
		}),
		"hamming": wrap(func(q, d []float64) float64 {
			n := 0
			for i := range q {
				n += bits.OnesCount8(uint8(int8(q[i])) ^ uint8(int8(d[i])))
			}
			return float64(n)
		}),
	}
}

// ---- unsupported operations on vector fields ----

// knnUnsupportedSort reports a sort on a knn_vector field.
func knnUnsupportedSort(idx *Index, field string) *apiError {
	def, _ := mappingFieldDef(idx, field)
	if t, _ := def["type"].(string); t != "knn_vector" {
		return nil
	}
	e := &apiError{Type: "unsupported_operation_exception", Status: 500, Reason: "knn vector field doesn't support this operation"}
	return plainShardError(idx, e)
}

// knnScriptScoreFinish applies boost and min_score of a script_score query.
func knnScriptScoreFinish(bm map[string]any, score float64) (bool, float64, *apiError) {
	if b, ok := toFloat(bm["boost"]); ok {
		score = float64(float32(score * b))
	}
	if ms, ok := toFloat(bm["min_score"]); ok && score < ms {
		return false, 0, nil
	}
	return true, score, nil
}
