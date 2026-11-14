package model

import "reflect"

// With refresh disabled, searches keep reading the last refreshed version
// of a document that was overwritten since. A by-query operation finds the
// document through such a search, so writing it back fails with a version
// conflict, exactly as OpenSearch's if_seq_no check does.

// noteStaleOverwrite is called for writes that leave the previously
// refreshed version of the document visible to searches.
func (idx *Index) noteStaleOverwrite(id string) {
	if !idx.RefreshDisabled() {
		return
	}
	if idx.Pending == nil {
		idx.Pending = map[string]map[string]any{}
	}
	if idx.Stale == nil {
		idx.Stale = map[string]StaleMark{}
	}
	idx.Stale[id] = StaleMark{Seq: idx.SeqNos[id], Pending: idx.Pending}
}

// SearchSeesStale reports whether searches still see an older version of
// the document than the current one (no refresh since its last write).
func (idx *Index) SearchSeesStale(id string) bool {
	m, ok := idx.Stale[id]
	if !ok {
		return false
	}
	if idx.Pending == nil || reflect.ValueOf(idx.Pending).Pointer() != reflect.ValueOf(m.Pending).Pointer() ||
		m.Seq != idx.SeqNos[id] {
		delete(idx.Stale, id)
		return false
	}
	return true
}
