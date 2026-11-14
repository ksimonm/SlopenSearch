package main

import (
	"net/http"
	"strings"
)

// ---------------------------------------------------------------------------
// _plugins/_knn: stats and warmup
// ---------------------------------------------------------------------------

// knnClusterStats are reported at the top level of the stats response.
var knnClusterStats = []string{"circuit_breaker_triggered", "model_index_status"}

var knnNodeStats = []string{
	"max_distance_query_with_filter_requests", "graph_memory_usage_percentage", "graph_query_requests",
	"graph_memory_usage", "cache_capacity_reached", "load_success_count", "training_memory_usage",
	"indices_in_cache", "script_query_errors", "hit_count", "knn_query_requests", "total_load_time",
	"miss_count", "min_score_query_requests", "knn_query_with_filter_requests", "training_memory_usage_percentage",
	"max_distance_query_requests", "lucene_initialized", "graph_index_errors", "faiss_initialized",
	"load_exception_count", "training_errors", "eviction_count", "nmslib_initialized",
	"script_compilations", "script_query_requests", "graph_stats", "min_score_query_with_filter_requests",
	"remote_vector_index_build_stats", "graph_query_errors", "indexing_from_model_degraded",
	"graph_index_requests", "training_requests", "script_compilation_errors",
}

func knnNodeStatValue(name string) any {
	switch name {
	case "lucene_initialized", "faiss_initialized", "nmslib_initialized", "indexing_from_model_degraded":
		return false
	case "graph_memory_usage_percentage", "training_memory_usage_percentage":
		return 0.0
	case "indices_in_cache":
		return map[string]any{}
	case "graph_stats":
		return orderedJSON{{"refresh", orderedJSON{{"total", 0}, {"total_time_in_millis", 0}}},
			{"merge", orderedJSON{{"current", 0}, {"total", 0}, {"total_time_in_millis", 0},
				{"current_docs", 0}, {"total_docs", 0}, {"total_size_in_bytes", 0}, {"current_size_in_bytes", 0}}}}
	case "remote_vector_index_build_stats":
		return map[string]any{}
	}
	return 0
}

func (s *Server) handleKnnPlugin(w http.ResponseWriter, r *http.Request, segs []string) bool {
	if len(segs) < 3 {
		return false
	}
	switch segs[2] {
	case "stats":
		s.handleKnnStats(w, r, segs)
		return true
	case "warmup":
		if len(segs) != 4 {
			return false
		}
		s.handleKnnWarmup(w, r, segs[3])
		return true
	}
	return false
}

func (s *Server) handleKnnStats(w http.ResponseWriter, r *http.Request, segs []string) {
	// /_plugins/_knn/stats[/{stat}] or /_plugins/_knn/{nodeId}/stats[/{stat}]
	var want []string
	if len(segs) >= 4 {
		want = strings.Split(segs[3], ",")
	}
	known := map[string]bool{}
	for _, n := range knnClusterStats {
		known[n] = true
	}
	for _, n := range knnNodeStats {
		known[n] = true
	}
	sel := map[string]bool{}
	var unknown []string
	for _, n := range want {
		if !known[n] {
			unknown = append(unknown, n)
			continue
		}
		sel[n] = true
	}
	if len(unknown) > 0 {
		writeError(w, errBad("request ["+r.URL.Path+"] contains unrecognized stat: ["+strings.Join(unknown, ", ")+"]"))
		return
	}
	all := len(sel) == 0
	out := orderedJSON{{"_nodes", orderedJSON{{"total", 1}, {"successful", 1}, {"failed", 0}}},
		{"cluster_name", "opensearch"}}
	for _, n := range knnClusterStats {
		if all || sel[n] {
			if n == "circuit_breaker_triggered" {
				out = append(out, orderedKV{n, false})
			} else {
				out = append(out, orderedKV{n, nil})
			}
		}
	}
	node := orderedJSON{}
	for _, n := range knnNodeStats {
		if all || sel[n] {
			node = append(node, orderedKV{n, knnNodeStatValue(n)})
		}
	}
	out = append(out, orderedKV{"nodes", orderedJSON{{nodeID, node}}})
	writeJSON(w, 200, out)
}

func (s *Server) handleKnnWarmup(w http.ResponseWriter, r *http.Request, spec string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	indices, err := s.resolveSpec2(spec, s.parseResolveParams(r, resolveOpts2{}))
	if err != nil {
		writeError(w, err)
		return
	}
	shards := 0
	for _, idx := range indices {
		if !indexKnnEnabled(idx) {
			writeError(w, &apiError{Type: "k_n_n_invalid_indices_exception", Status: 500,
				Reason: "Warm up request rejected. One or more indices have 'index.knn' set to false."})
			return
		}
		shards += idx.numShards()
	}
	writeJSON(w, 200, orderedJSON{{"_shards", orderedJSON{{"total", shards}, {"successful", shards}, {"failed", 0}}}})
}
