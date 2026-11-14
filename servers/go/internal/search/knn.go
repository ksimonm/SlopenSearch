package search

import (
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
)

func knnFieldDef(idx *model.Index, path string) (map[string]any, bool) {
	props, _ := idx.Mappings["properties"].(map[string]any)
	parts := strings.Split(path, ".")
	nested := false
	for i, p := range parts {
		def, ok := props[p].(map[string]any)
		if !ok {
			return nil, false
		}
		if i == len(parts)-1 {
			return def, nested
		}
		if t, _ := def["type"].(string); t == "nested" {
			nested = true
		}
		props, _ = def["properties"].(map[string]any)
	}
	return nil, false
}

// knnDistanceScore returns the raw distance-like value and the OpenSearch score.
func knnDistanceScore(space string, q, d []float64) (float64, float64) {
	switch space {
	case "cosinesimil":
		dot, na, nb := 0.0, 0.0, 0.0
		for i := range q {
			dot += q[i] * d[i]
			na += q[i] * q[i]
			nb += d[i] * d[i]
		}
		cos := 0.0
		if na > 0 && nb > 0 {
			cos = dot / math.Sqrt(na*nb)
		}
		return 1 - cos, float64(float32((1 + cos) / 2))
	case "innerproduct":
		ip := 0.0
		for i := range q {
			ip += q[i] * d[i]
		}
		if ip >= 0 {
			return -ip, float64(float32(1 + ip))
		}
		return -ip, float64(float32(1 / (1 - ip)))
	case "hamming":
		bits := 0
		for i := range q {
			x := uint8(int8(q[i])) ^ uint8(int8(d[i]))
			for x != 0 {
				bits += int(x & 1)
				x >>= 1
			}
		}
		return float64(bits), float64(float32(1 / (1 + float64(bits))))
	case "l1":
		s := 0.0
		for i := range q {
			s += math.Abs(q[i] - d[i])
		}
		return s, float64(float32(1 / (1 + s)))
	case "linf":
		s := 0.0
		for i := range q {
			s = math.Max(s, math.Abs(q[i]-d[i]))
		}
		return s, float64(float32(1 / (1 + s)))
	default:
		s := 0.0
		for i := range q {
			s += (q[i] - d[i]) * (q[i] - d[i])
		}
		return s, float64(float32(1 / (1 + s)))
	}
}

type knnCacheEntry struct {
	body   map[string]any
	idx    *model.Index
	scores map[string]float64
}

var (
	knnCacheMu sync.Mutex
	knnCache   = map[uintptr][]*knnCacheEntry{}
	knnCacheN  int
)

func floatList(v any) ([]float64, bool) {
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]float64, 0, len(arr))
	for _, e := range arr {
		f, ok := jsonv.ToFloat(e)
		if !ok {
			return nil, false
		}
		out = append(out, f)
	}
	return out, true
}

func (c *QueryCtx) docVector(field string) ([]float64, bool) {
	vals, present := c.Values(field)
	if !present || len(vals) == 0 {
		return nil, false
	}
	if len(vals) == 1 {
		if arr, ok := floatList(vals[0]); ok {
			return arr, true
		}
	}
	var dv []float64
	for _, e := range vals {
		f, ok := jsonv.ToFloat(e)
		if !ok {
			return nil, false
		}
		dv = append(dv, f)
	}
	return dv, true
}

func knnParseErr(reason string) *apierr.Error {
	return &apierr.Error{Type: "parsing_exception", Reason: reason, Status: 400}
}

func (c *QueryCtx) evalKnnField(field string, p map[string]any) (bool, float64, *apierr.Error) {
	vec, ok := floatList(p["vector"])
	if !ok {
		return false, 0, knnParseErr("[knn] failed to parse field [vector]")
	}
	_, hasK := p["k"]
	_, hasMin := p["min_score"]
	_, hasMax := p["max_distance"]
	set := 0
	for _, b := range []bool{hasK, hasMin, hasMax} {
		if b {
			set++
		}
	}
	if set != 1 {
		return false, 0, apierr.BadRequest("[knn] requires exactly one of k, distance or score to be set")
	}
	boost := 1.0
	if b, ok := jsonv.ToFloat(p["boost"]); ok {
		boost = b
	}
	def, _ := knnFieldDef(c.Idx, field)
	fi := mapping.KnnInfo(def)
	score := func(qc *QueryCtx) (bool, float64, float64) {
		dv, ok := qc.docVector(field)
		if !ok || len(dv) != len(vec) {
			return false, 0, 0
		}
		d, s := knnDistanceScore(fi.Space, vec, dv)
		return true, d, s
	}
	filter, _ := p["filter"].(map[string]any)
	if hasK {
		kf, _ := jsonv.ToFloat(p["k"])
		k := int(kf)
		if k <= 0 || k > 10000 {
			return false, 0, apierr.BadRequest("[knn] requires k to be in the range (0, 10000]")
		}
		if c.NestedRoot != nil {
			return c.knnNestedTopK(p, k, filter, score, boost)
		}
		scores, err := c.knnTopK(p, field, k, filter, score)
		if err != nil {
			return false, 0, err
		}
		s, ok := scores[c.ID]
		if !ok {
			return false, 0, nil
		}
		return true, s * boost, nil
	}
	if filter != nil {
		m, _, err := c.Eval(filter)
		if err != nil || !m {
			return false, 0, err
		}
	}
	ok, d, s := score(c)
	if !ok {
		return false, 0, nil
	}
	if hasMin {
		ms, _ := jsonv.ToFloat(p["min_score"])
		if ms <= 0 {
			return false, 0, apierr.BadRequest("[knn] requires minScore to be greater than 0")
		}
		return s >= ms, s * boost, nil
	}
	md, _ := jsonv.ToFloat(p["max_distance"])
	if filter == nil && fi.Engine == "faiss" {
		// faiss range search excludes the boundary; exact (filtered) search keeps it
		return d < md, s * boost, nil
	}
	return d <= md, s * boost, nil
}

func (c *QueryCtx) knnTopK(p map[string]any, field string, k int, filter map[string]any,
	score func(*QueryCtx) (bool, float64, float64)) (map[string]float64, *apierr.Error) {
	key := reflect.ValueOf(p).Pointer()
	knnCacheMu.Lock()
	for _, e := range knnCache[key] {
		if e.idx == c.Idx {
			knnCacheMu.Unlock()
			return e.scores, nil
		}
	}
	knnCacheMu.Unlock()
	type cand struct {
		id    string
		score float64
		ord   int
	}
	var cands []cand
	for ord, id := range c.Idx.Order {
		doc, ok := c.Idx.Docs[id]
		if !ok {
			continue
		}
		qc := &QueryCtx{Idx: c.Idx, ID: id, Doc: doc}
		if filter != nil {
			m, _, err := qc.Eval(filter)
			if err != nil {
				return nil, err
			}
			if !m {
				continue
			}
		}
		if ok, _, s := score(qc); ok {
			cands = append(cands, cand{id, s, ord})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	if len(cands) > k {
		cands = cands[:k]
	}
	out := map[string]float64{}
	for _, cd := range cands {
		out[cd.id] = cd.score
	}
	knnCacheMu.Lock()
	if knnCacheN > 512 {
		knnCache = map[uintptr][]*knnCacheEntry{}
		knnCacheN = 0
	}
	knnCache[key] = append(knnCache[key], &knnCacheEntry{body: p, idx: c.Idx, scores: out})
	knnCacheN++
	knnCacheMu.Unlock()
	return out, nil
}
