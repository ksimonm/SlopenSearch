package main

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// knn on nested fields: k counts parent documents
// ---------------------------------------------------------------------------

// knnExplain explains a top-level knn query hit, or returns nil.
func knnExplain(idx *Index, query map[string]any, id string, score float64) any {
	kq, ok := query["knn"].(map[string]any)
	if !ok || len(query) != 1 || len(kq) != 1 {
		return nil
	}
	for field, raw := range kq {
		p, ok := raw.(map[string]any)
		if !ok {
			return nil
		}
		k, hasK := toFloat(p["k"])
		if !hasK {
			return nil
		}
		def, _ := knnFieldDef(idx, field)
		fi := knnInfo(def)
		if fi.engine == "lucene" {
			return orderedJSON{{"value", scoreJSON(score)},
				{"description", fmt.Sprintf("within top %d docs", int(k))}, {"details", []any{}}}
		}
		qv, _ := floatList(p["vector"])
		qc := &queryCtx{idx: idx, id: id, doc: idx.Docs[id]}
		dv, _ := qc.docVector(field)
		raw := 0.0
		formula := "`1 / (1 + rawScore)`"
		if len(dv) == len(qv) {
			raw, _ = knnDistanceScore(fi.space, qv, dv)
		}
		switch fi.space {
		case "innerproduct":
			raw = -raw
			formula = "`rawScore >= 0 ? 1 + rawScore : 1 / (1 - rawScore)`"
		case "cosinesimil":
			raw = 1 - raw
			formula = "`(1 + rawScore) / 2`"
		}
		return orderedJSON{{"value", scoreJSON(score)},
			{"description", "the type of knn search executed was Approximate-NN"},
			{"details", []any{orderedJSON{{"value", scoreJSON(score)},
				{"description", fmt.Sprintf("the type of knn search executed at leaf was Approximate-NN with vectorDataType = %s, spaceType = %s where score is computed as %s from:",
					strings.ToUpper(fi.dataType), fi.space, formula)},
				{"details", []any{orderedJSON{{"value", scoreJSON(float64(float32(raw)))},
					{"description", "rawScore, returned from FAISS library"}, {"details", []any{}}}}}}}}}
	}
	return nil
}

type knnParentHit struct {
	score  float64
	offset int // best child
}

type knnNestedEntry struct {
	idx  *Index
	path string
	hits map[string]knnParentHit
}

var (
	knnNestedMu    sync.Mutex
	knnNestedCache = map[uintptr][]*knnNestedEntry{}
)

// knnNestedTopK evaluates a knn query for the nested child in c: the k
// best parents (by their best child) match; without expand_nested_docs only
// the best child of each of them matches.
func (c *queryCtx) knnNestedTopK(p map[string]any, k int, filter map[string]any,
	score func(*queryCtx) (bool, float64, float64), boost float64) (bool, float64, *apiError) {
	hits, err := c.knnNestedHits(p, k, filter, score)
	if err != nil {
		return false, 0, err
	}
	h, ok := hits[c.id]
	if !ok {
		return false, 0, nil
	}
	if expand, _ := p["expand_nested_docs"].(bool); !expand && c.nestedOffset != h.offset {
		return false, 0, nil
	}
	ok, _, s := score(c)
	if !ok {
		return false, 0, nil
	}
	return true, s * boost, nil
}

func (c *queryCtx) knnNestedHits(p map[string]any, k int, filter map[string]any,
	score func(*queryCtx) (bool, float64, float64)) (map[string]knnParentHit, *apiError) {
	key := reflect.ValueOf(p).Pointer()
	knnNestedMu.Lock()
	for _, e := range knnNestedCache[key] {
		if e.idx == c.idx && e.path == c.nestedPath {
			knnNestedMu.Unlock()
			return e.hits, nil
		}
	}
	knnNestedMu.Unlock()
	type cand struct {
		id    string
		hit   knnParentHit
		order int
	}
	var cands []cand
	for ord, id := range c.idx.Order {
		doc, ok := c.idx.Docs[id]
		if !ok {
			continue
		}
		parent := &queryCtx{idx: c.idx, id: id, doc: doc}
		if filter != nil {
			m, _, err := parent.eval(filter)
			if err != nil {
				return nil, err
			}
			if !m {
				continue
			}
		}
		objects, ok := nestedObjectsForCtx(parent, c.nestedPath)
		if !ok {
			continue
		}
		found := false
		var best knnParentHit
		for i, obj := range objects {
			child := &queryCtx{idx: c.idx, id: id, doc: doc, nestedPath: c.nestedPath, nestedRoot: obj, nestedOffset: i}
			if ok, _, s := score(child); ok && (!found || s > best.score) {
				best = knnParentHit{score: s, offset: i}
				found = true
			}
		}
		if found {
			cands = append(cands, cand{id, best, ord})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].hit.score > cands[j].hit.score })
	if len(cands) > k {
		cands = cands[:k]
	}
	out := map[string]knnParentHit{}
	for _, cd := range cands {
		out[cd.id] = cd.hit
	}
	knnNestedMu.Lock()
	if len(knnNestedCache) > 512 {
		knnNestedCache = map[uintptr][]*knnNestedEntry{}
	}
	knnNestedCache[key] = append(knnNestedCache[key], &knnNestedEntry{idx: c.idx, path: c.nestedPath, hits: out})
	knnNestedMu.Unlock()
	return out, nil
}
