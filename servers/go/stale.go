package main

import "reflect"

// With refresh disabled, searches keep reading the last refreshed version
// of a document that was overwritten since. A by-query operation finds the
// document through such a search, so writing it back fails with a version
// conflict, exactly as OpenSearch's if_seq_no check does.

// staleMark records an unrefreshed overwrite of a searchable document.
type staleMark struct {
	seq     int64                     // seq_no of the overwrite
	pending map[string]map[string]any // Pending map at the time; a refresh replaces it
}

// noteStaleOverwrite is called for writes that leave the previously
// refreshed version of the document visible to searches.
func noteStaleOverwrite(idx *Index, id string) {
	if !idx.refreshDisabled() {
		return
	}
	if idx.Pending == nil {
		idx.Pending = map[string]map[string]any{}
	}
	if idx.stale == nil {
		idx.stale = map[string]staleMark{}
	}
	idx.stale[id] = staleMark{seq: idx.SeqNos[id], pending: idx.Pending}
}

// searchSeesStale reports whether searches still see an older version of
// the document than the current one (no refresh since its last write).
func searchSeesStale(idx *Index, id string) bool {
	m, ok := idx.stale[id]
	if !ok {
		return false
	}
	if idx.Pending == nil || reflect.ValueOf(idx.Pending).Pointer() != reflect.ValueOf(m.pending).Pointer() ||
		m.seq != idx.SeqNos[id] {
		delete(idx.stale, id)
		return false
	}
	return true
}
