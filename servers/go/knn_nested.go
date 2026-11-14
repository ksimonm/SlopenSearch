package main

import (
	"reflect"
	"sort"
	"sync"
)

// ---------------------------------------------------------------------------
// knn on nested fields: k counts parent documents
// ---------------------------------------------------------------------------

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
