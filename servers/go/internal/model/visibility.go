package model

// What searches see between refreshes. A search reads the last refreshed
// point in time: a document written since is not searchable yet, and when
// the write replaced or deleted a searchable document, searches keep seeing
// that (last refreshed) version with its doc values, _version and _seq_no.
// Writes record that version here (NoteWrite) before changing the
// document; a search swaps it in for its duration (EnterSearchView, under
// the write lock) and restores the current one afterwards (Leave).
// Realtime reads (GET, mget) and writes always see the current version.

// DocState is one version of a document: everything a search reads of it.
type DocState struct {
	Doc     map[string]any
	DV      map[string][]any
	Ignored []string
	Raw     []byte
	Version int64
	Seq     int64
	Term    int64
	Routing string
	has     uint8  // which of the fields above the version has
	gen     uint64 // refresh generation it was recorded in
}

const (
	hasDoc uint8 = 1 << iota
	hasDV
	hasIgnored
	hasRaw
	hasVersion
	hasSeq
	hasTerm
	hasRouting
)

// swapEntry exchanges (*v, bit of *has) with the entry id of *m.
func swapEntry[V any](m *map[string]V, id string, v *V, has *uint8, bit uint8) {
	cur, ok := (*m)[id]
	if *has&bit != 0 {
		if *m == nil {
			*m = map[string]V{}
		}
		(*m)[id] = *v
	} else {
		delete(*m, id)
	}
	*v = cur
	if ok {
		*has |= bit
	} else {
		*has &^= bit
	}
}

// swapDoc exchanges st with the index's current state of document id.
func (idx *Index) swapDoc(id string, st *DocState) {
	swapEntry(&idx.Docs, id, &st.Doc, &st.has, hasDoc)
	swapEntry(&idx.DocValues, id, &st.DV, &st.has, hasDV)
	swapEntry(&idx.DocIgnored, id, &st.Ignored, &st.has, hasIgnored)
	swapEntry(&idx.RawDocs, id, &st.Raw, &st.has, hasRaw)
	swapEntry(&idx.Versions, id, &st.Version, &st.has, hasVersion)
	swapEntry(&idx.SeqNos, id, &st.Seq, &st.has, hasSeq)
	swapEntry(&idx.PrimaryTerms, id, &st.Term, &st.has, hasTerm)
	swapEntry(&idx.Routing, id, &st.Routing, &st.has, hasRouting)
}

func readEntry[V any](m map[string]V, id string, v *V, has *uint8, bit uint8) {
	if x, ok := m[id]; ok {
		*v = x
		*has |= bit
	}
}

// captureDoc copies the current state of document id.
func (idx *Index) captureDoc(id string) *DocState {
	st := &DocState{gen: idx.refreshGen}
	readEntry(idx.Docs, id, &st.Doc, &st.has, hasDoc)
	readEntry(idx.DocValues, id, &st.DV, &st.has, hasDV)
	readEntry(idx.DocIgnored, id, &st.Ignored, &st.has, hasIgnored)
	readEntry(idx.RawDocs, id, &st.Raw, &st.has, hasRaw)
	readEntry(idx.Versions, id, &st.Version, &st.has, hasVersion)
	readEntry(idx.SeqNos, id, &st.Seq, &st.has, hasSeq)
	readEntry(idx.PrimaryTerms, id, &st.Term, &st.has, hasTerm)
	readEntry(idx.Routing, id, &st.Routing, &st.has, hasRouting)
	return st
}

// changed reports whether document id was written since st was captured
// (every write gives it a new seqNo).
func (idx *Index) changed(id string, st *DocState) bool {
	seq, ok := idx.SeqNos[id]
	_, live := idx.Docs[id]
	return ok != (st.has&hasSeq != 0) || seq != st.Seq || live != (st.has&hasDoc != 0)
}

// Snapshot is the versions of some documents at one point in time (a point
// in time search context keeps what it saw when it was opened).
type Snapshot struct {
	idx    *Index
	ids    []string
	states []*DocState
}

// Snapshot captures the current versions of the documents ids.
func (idx *Index) Snapshot(ids []string) *Snapshot {
	sn := &Snapshot{idx: idx, ids: ids, states: make([]*DocState, len(ids))}
	for i, id := range ids {
		sn.states[i] = idx.captureDoc(id)
	}
	return sn
}

// Changed reports whether some of the documents were written since.
func (sn *Snapshot) Changed() bool {
	for i, id := range sn.ids {
		if sn.idx.changed(id, sn.states[i]) {
			return true
		}
	}
	return false
}

// Enter swaps the captured versions in for the documents written since
// (under the write lock); the returned function restores the current ones.
func (sn *Snapshot) Enter() func() {
	type swap struct {
		id  string
		cur *DocState
	}
	var swapped []swap
	for i, id := range sn.ids {
		if sn.idx.changed(id, sn.states[i]) {
			cur := *sn.states[i]
			sn.idx.swapDoc(id, &cur) // the index has the captured version, cur the current one
			swapped = append(swapped, swap{id, &cur})
		}
	}
	if len(swapped) == 0 {
		return func() {}
	}
	sn.idx.DVGen ^= 1 << 62
	return func() {
		for _, sw := range swapped {
			sn.idx.swapDoc(sw.id, sw.cur)
		}
		sn.idx.DVGen ^= 1 << 62
	}
}

// staleVisible reports whether searches see the recorded version st of
// document id rather than its current one: recorded since the last refresh,
// and the document was rewritten without a refresh (pending) or deleted.
func (idx *Index) staleVisible(id string, st *DocState) bool {
	if st == nil || st.gen != idx.refreshGen {
		return false
	}
	if _, pending := idx.Pending[id]; pending {
		return true
	}
	_, live := idx.Docs[id]
	return !live
}

// NoteWrite is called before a write changes or deletes document id: it
// remembers the version searches keep seeing until the next refresh.
func (idx *Index) NoteWrite(id string) {
	idx.EnsureDocState()
	if st, ok := idx.Stale[id]; ok {
		if idx.staleVisible(id, st) {
			return // already rewritten since the refresh: the recorded version stays
		}
		delete(idx.Stale, id)
	}
	if _, pending := idx.Pending[id]; pending {
		return // never searchable
	}
	if _, live := idx.Docs[id]; !live {
		return
	}
	if idx.Stale == nil {
		idx.Stale = map[string]*DocState{}
	}
	idx.Stale[id] = idx.captureDoc(id)
}

// Refresh makes every write searchable.
func (idx *Index) Refresh() {
	idx.Pending = nil
	idx.Stale = nil
	idx.refreshGen++
}

// HasStale reports whether searches of the index see older versions of
// some documents than realtime reads.
func (idx *Index) HasStale() bool { return len(idx.Stale) > 0 }

// StaleSeqNo is the _seq_no searches see of document id, when they see an
// older version than the current one.
func (idx *Index) StaleSeqNo(id string) (int64, bool) {
	st := idx.Stale[id]
	if !idx.staleVisible(id, st) {
		return 0, false
	}
	return st.Seq, st.has&hasSeq != 0
}

// SearchView holds the documents swapped in by EnterSearchView.
type SearchView struct {
	idx     *Index
	ids     []string
	pending []map[string]any
	wasPend []bool
}

// EnterSearchView makes the last refreshed versions of the documents
// written since searchable; Leave restores the current ones. It needs the
// write lock. It returns nil when there is nothing to swap.
func (idx *Index) EnterSearchView() *SearchView {
	if len(idx.Stale) == 0 {
		return nil
	}
	v := &SearchView{idx: idx}
	for id, st := range idx.Stale {
		if !idx.staleVisible(id, st) {
			delete(idx.Stale, id)
			continue
		}
		p, was := idx.Pending[id]
		delete(idx.Pending, id)
		idx.swapDoc(id, st)
		v.ids = append(v.ids, id)
		v.pending = append(v.pending, p)
		v.wasPend = append(v.wasPend, was)
	}
	if len(v.ids) == 0 {
		return nil
	}
	// derived caches of the doc values must not mix the two views; the
	// flipped bit never collides with a generation of the current view
	idx.DVGen ^= 1 << 63
	return v
}

// Leave restores the current versions after EnterSearchView.
func (v *SearchView) Leave() {
	if v == nil {
		return
	}
	idx := v.idx
	for i, id := range v.ids {
		idx.swapDoc(id, idx.Stale[id])
		if v.wasPend[i] {
			if idx.Pending == nil {
				idx.Pending = map[string]map[string]any{}
			}
			idx.Pending[id] = v.pending[i]
		}
	}
	idx.DVGen ^= 1 << 63
}

// AppendOrder adds a new document to the end of the index order. A document
// created again after a delete moves to the end (it is in a new segment).
func (idx *Index) AppendOrder(id string) {
	if _, had := idx.Versions[id]; had {
		for i, o := range idx.Order {
			if o == id {
				order := make([]string, 0, len(idx.Order))
				order = append(order, idx.Order[:i]...)
				idx.Order = append(order, idx.Order[i+1:]...)
				break
			}
		}
	}
	idx.Order = append(idx.Order, id)
}
