package httpapi

// Wiring of internal/storage: opening the data directory, which
// requests are followed by a journal write.

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"slopensearch/internal/mapping"
	"slopensearch/internal/storage"
)

// processID is the node's process.id. Inside a container the server usually
// runs as PID 1 on every start; a restart must still be observable, so PID 1
// is replaced by a plausible PID derived from the start time.
var processID = func() int {
	if pid := os.Getpid(); pid > 1 {
		return pid
	}
	return 2 + int(time.Now().UnixNano()/1000%4194000)
}()

// PersistenceDescription says where the state is kept, for the startup line.
func (s *Server) PersistenceDescription() string {
	if s.persist == nil {
		return " (in memory)"
	}
	if abs, err := filepath.Abs(s.persist.Dir()); err == nil {
		return " (data in " + abs + ")"
	}
	return " (data in " + s.persist.Dir() + ")"
}

// OpenPersistence loads the state stored in dir (creating it if needed) and
// starts journaling. An empty dir disables persistence.
func (s *Server) OpenPersistence(dir string) error {
	if dir == "" {
		return nil
	}
	locks := storage.Locks{State: &s.mu, Ingest: &s.ingestMu, Scripts: &s.scriptsMu}
	p, err := storage.Open(dir, &s.State, locks, mapping.RecomputeAllDocValues)
	if err != nil {
		return err
	}
	s.persist = p
	return nil
}

// persistAfterRequest makes the changes of a finished request durable before
// its response is sent.
func (s *Server) persistAfterRequest(method, path string) {
	if s.persist == nil || method == "GET" || method == "HEAD" || readOnlyPath(path) {
		return
	}
	s.persist.Persist()
}

// readOnlyPath reports requests that never change durable state.
func readOnlyPath(path string) bool {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	for i := 0; i < len(segs) && i < 2; i++ {
		switch segs[i] {
		case "_search", "_msearch", "_count", "_analyze", "_mget", "_explain",
			"_field_caps", "_validate", "_termvectors", "_mtermvectors", "_rank_eval",
			"_render", "_refresh", "_cache", "_search_shards":
			// /_search/pipeline/<id> stores a search pipeline
			if segs[i] == "_search" && i+1 < len(segs) && segs[i+1] == "pipeline" {
				return false
			}
			return true
		}
	}
	return false
}
