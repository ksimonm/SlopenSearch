package model

// Document state of an index: sequence numbers, versions, routing and the
// unrefreshed (pending) writes.

// AllocSeqNo returns the next sequence number of the index.
func (idx *Index) AllocSeqNo() int64 {
	idx.EnsureDocState()
	n := idx.NextSeq
	idx.NextSeq++
	return n
}

// CurrentVersion is the _version of a document (0 if it does not exist).
func (idx *Index) CurrentVersion(id string) int64 {
	idx.EnsureDocState()
	if v, ok := idx.Versions[id]; ok {
		return v
	}
	if _, ok := idx.Docs[id]; ok {
		return 1
	}
	return 0
}

// RoutingRequired reports _routing.required in the mappings.
func (idx *Index) RoutingRequired() bool {
	m, _ := idx.Mappings["_routing"].(map[string]any)
	if m == nil {
		return false
	}
	b, _ := m["required"].(bool)
	return b
}

// SourceEnabled reports _source.enabled in the mappings (default true).
func (idx *Index) SourceEnabled() bool {
	m, _ := idx.Mappings["_source"].(map[string]any)
	if m == nil {
		return true
	}
	if b, ok := m["enabled"].(bool); ok {
		return b
	}
	return true
}

// RoutingMatches reports whether a request for document id with routing
// addresses the shard that holds it.
func (idx *Index) RoutingMatches(id, routing string) bool {
	if idx.NumShards() <= 1 {
		return true
	}
	return idx.ShardFor(id, routing) == idx.DocShard(id)
}

// MarkPending records an unrefreshed write. A document that searches could
// already see stays visible: a refresh only changes which version they read,
// it never makes an existing document disappear.
func (idx *Index) MarkPending(id string, doc map[string]any, existed bool) {
	if _, pending := idx.Pending[id]; existed && !pending {
		idx.noteStaleOverwrite(id)
		return
	}
	idx.Pending[id] = doc
}
