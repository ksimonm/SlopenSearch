package search

import "slopensearch/internal/model"

// Cluster gives query evaluation access to the rest of the cluster state:
// other indices (terms lookup, indexed shapes) and stored scripts. The server
// sets it at startup.
var Cluster ClusterLookup

// ClusterLookup looks up cluster state outside the index being searched.
type ClusterLookup interface {
	// Index returns an index by name; the caller holds the state lock.
	Index(name string) (*model.Index, bool)
	// StoredScript returns a stored script; it takes the scripts lock.
	StoredScript(id string) (map[string]any, bool)
}
