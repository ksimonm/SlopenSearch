// Package model has the domain state of the server: indices with their
// settings, mappings and documents. It holds data only; the operations on it
// live in the server.
package model

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

	Stale map[string]StaleMark // unrefreshed overwrites of searchable docs
}

// StaleMark records an unrefreshed overwrite of a searchable document.
type StaleMark struct {
	Seq     int64                     // seq_no of the overwrite
	Pending map[string]map[string]any // Pending map at the time; a refresh replaces it
}
