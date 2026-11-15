// Package model is the domain layer: the cluster state (State) with its
// indices (Index: metadata, settings, mappings, documents and their versions)
// and data streams, plus the accessors that need nothing but that data.
package model

import (
	"time"

	"slopensearch/internal/settings"
)

// Index is one index: metadata, mappings, settings and the documents with
// their versions, sequence numbers and indexed values.
type Index struct {
	Name           string
	UUID           string
	CreationDate   int64
	Mappings       map[string]any
	Settings       map[string]any // canonical flat form, keys prefixed "index."
	Docs           map[string]map[string]any
	Order          []string
	FieldTypes     map[string]string
	Closed         bool
	Aliases        map[string]map[string]any
	MappingVersion int64
	Pending        map[string]map[string]any // indexed but not yet refreshed (search_interval=-1)
	Routing        map[string]string         // doc id -> routing value
	SegGen         int64
	Deletes        int64
	DataStream     string // non-empty when this index backs a data stream
	Verified       bool
	IndexTotal     int64
	DeleteTotal    int64
	ForceMerged    bool
	DocValues      map[string]map[string][]any // doc id -> field path -> indexed values
	DocIgnored     map[string][]string         // doc id -> ignored (malformed) fields
	RawDocs        map[string][]byte           // doc id -> original _source bytes
	Versions       map[string]int64
	SeqNos         map[string]int64
	PrimaryTerms   map[string]int64
	NextSeq        int64

	DVGen uint64 // bumped whenever DocValues changes; validates derived caches

	Stale map[string]*DocState // last refreshed versions of documents written since (visibility.go)

	pendingSince time.Time // when unrefreshed writes were first seen (refresh.go)
	refreshGen   uint64    // bumped by every refresh (visibility.go)
}

// NewIndex returns an empty index with every map allocated.
func NewIndex(name string) *Index {
	idx := &Index{
		Name:       name,
		Mappings:   map[string]any{},
		Settings:   map[string]any{},
		Docs:       map[string]map[string]any{},
		FieldTypes: map[string]string{},
		Aliases:    map[string]map[string]any{},
	}
	idx.EnsureDocState()
	return idx
}

// EnsureDocState allocates the per-document maps that are still nil.
func (idx *Index) EnsureDocState() {
	if idx.Versions == nil {
		idx.Versions = map[string]int64{}
	}
	if idx.SeqNos == nil {
		idx.SeqNos = map[string]int64{}
	}
	if idx.PrimaryTerms == nil {
		idx.PrimaryTerms = map[string]int64{}
	}
	if idx.Pending == nil {
		idx.Pending = map[string]map[string]any{}
	}
	if idx.RawDocs == nil {
		idx.RawDocs = map[string][]byte{}
	}
	if idx.Routing == nil {
		idx.Routing = map[string]string{}
	}
}

// Hidden reports the index.hidden setting.
func (idx *Index) Hidden() bool { return idx.boolSetting("hidden") }

// Blocked reports whether the index.blocks.<name> block is set.
func (idx *Index) Blocked(name string) bool { return idx.boolSetting("blocks." + name) }

func (idx *Index) boolSetting(key string) bool {
	v, ok := settings.Find(idx.Settings, key)
	if !ok {
		return false
	}
	return settings.ValueString(v) == "true"
}

// RefreshDisabled reports refresh_interval -1: documents become searchable
// only on an explicit refresh.
func (idx *Index) RefreshDisabled() bool {
	v, ok := settings.Find(idx.Settings, "refresh_interval")
	if !ok {
		return false
	}
	return settings.ValueString(v) == "-1"
}

// NumShards is index.number_of_shards (default 1).
func (idx *Index) NumShards() int {
	if v, ok := settings.Int(idx.Settings, "number_of_shards"); ok {
		return v
	}
	return 1
}

// NumReplicas is index.number_of_replicas (default 1).
func (idx *Index) NumReplicas() int {
	if v, ok := settings.Int(idx.Settings, "number_of_replicas"); ok {
		return v
	}
	return 1
}
