package main

import (
	"net/http"

	"slopensearch/internal/model"
)

func (s *Server) handleSearchShards(w http.ResponseWriter, r *http.Request, indexSpec string) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		methodNotAllowed(w, r, "GET", "POST")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	indices, apiErr := s.resolveIndices(indexSpec, resolveOpts{missing404: true, wildcardClosed: true})
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	sf := newShardFilter(r, indices)
	indicesOut := map[string]any{}
	shardsOut := []any{}
	for _, idx := range indices {
		indicesOut[idx.Name] = map[string]any{}
		for shard := 0; shard < idx.NumShards(); shard++ {
			if !sf.allowsShard(idx, shard) {
				continue
			}
			shardsOut = append(shardsOut, []any{map[string]any{
				"index": idx.Name, "shard": shard, "primary": true, "state": "STARTED", "node": model.NodeID,
			}})
		}
	}
	writeJSON(w, 200, map[string]any{
		"nodes": map[string]any{model.NodeID: map[string]any{
			"name": model.NodeName, "transport_address": nodeTransport,
		}},
		"indices": indicesOut,
		"shards":  shardsOut,
	})
}
