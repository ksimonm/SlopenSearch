package httpapi

import "time"

// scheduledRefresh applies the periodic refreshes of the indices
// (model.Index.TickRefresh); it takes the write lock only when one is due.
func (s *Server) scheduledRefresh() {
	now := time.Now()
	s.mu.RLock()
	due := false
	for _, idx := range s.Indices {
		if idx.RefreshTickDue(now) {
			due = true
			break
		}
	}
	s.mu.RUnlock()
	if !due {
		return
	}
	s.mu.Lock()
	for _, idx := range s.Indices {
		idx.TickRefresh(now)
	}
	s.mu.Unlock()
}
