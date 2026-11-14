package search

import (
	"reflect"
	"sort"
	"sync"

	"slopensearch/internal/apierr"
	"slopensearch/internal/model"
)

type knnParentHit struct {
	score  float64
	offset int // best child
}

type knnNestedEntry struct {
	idx  *model.Index
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
func (c *QueryCtx) knnNestedTopK(p map[string]any, k int, filter map[string]any,
	score func(*QueryCtx) (bool, float64, float64), boost float64) (bool, float64, *apierr.Error) {
	hits, err := c.knnNestedHits(p, k, filter, score)
	if err != nil {
		return false, 0, err
	}
	h, ok := hits[c.ID]
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

func (c *QueryCtx) knnNestedHits(p map[string]any, k int, filter map[string]any,
	score func(*QueryCtx) (bool, float64, float64)) (map[string]knnParentHit, *apierr.Error) {
	key := reflect.ValueOf(p).Pointer()
	knnNestedMu.Lock()
	for _, e := range knnNestedCache[key] {
		if e.idx == c.Idx && e.path == c.NestedPath {
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
	for ord, id := range c.Idx.Order {
		doc, ok := c.Idx.Docs[id]
		if !ok {
			continue
		}
		parent := &QueryCtx{Idx: c.Idx, ID: id, Doc: doc}
		if filter != nil {
			m, _, err := parent.Eval(filter)
			if err != nil {
				return nil, err
			}
			if !m {
				continue
			}
		}
		objects, ok := nestedObjectsForCtx(parent, c.NestedPath)
		if !ok {
			continue
		}
		found := false
		var best knnParentHit
		for i, obj := range objects {
			child := &QueryCtx{Idx: c.Idx, ID: id, Doc: doc, NestedPath: c.NestedPath, NestedRoot: obj, nestedOffset: i}
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
	knnNestedCache[key] = append(knnNestedCache[key], &knnNestedEntry{idx: c.Idx, path: c.NestedPath, hits: out})
	knnNestedMu.Unlock()
	return out, nil
}
