package httpapi

import (
	"net/http"

	"slopensearch/internal/model"
)

// Searches read the last refreshed versions of the documents
// (model.Index.EnterSearchView). Swapping them in changes the index, so a
// search takes the write lock while some index has such versions, and the
// read lock otherwise.

// lockSearchView locks the server for a search and returns the unlock.
func (s *Server) lockSearchView() func() {
	s.mu.RLock()
	stale := false
	for _, idx := range s.Indices {
		if idx.HasStale() {
			stale = true
			break
		}
	}
	if !stale {
		return s.mu.RUnlock
	}
	s.mu.RUnlock()
	s.mu.Lock()
	views := enterSearchViews(s.Indices)
	return func() {
		leaveSearchViews(views)
		s.mu.Unlock()
	}
}

// enterSearchViews swaps in the refreshed versions of every index (under
// the write lock).
func enterSearchViews(indices map[string]*model.Index) []*model.SearchView {
	var views []*model.SearchView
	for _, idx := range indices {
		if v := idx.EnterSearchView(); v != nil {
			views = append(views, v)
		}
	}
	return views
}

func leaveSearchViews(views []*model.SearchView) {
	for i := len(views) - 1; i >= 0; i-- {
		views[i].Leave()
	}
}

// lockGetView locks the server for a get: realtime=false reads the last
// refreshed versions, like a search.
func (s *Server) lockGetView(r *http.Request) func() {
	if r.URL.Query().Get("realtime") == "false" {
		return s.lockSearchView()
	}
	s.mu.RLock()
	return s.mu.RUnlock
}

// lockSearchFor locks the server for a search; a search of a point in time
// reads the versions of the documents it was opened on.
func (s *Server) lockSearchFor(pit *pitContext) func() {
	if pit == nil || pit.snap == nil {
		return s.lockSearchView()
	}
	s.mu.RLock()
	if !pit.snap.Changed() {
		return s.mu.RUnlock
	}
	s.mu.RUnlock()
	s.mu.Lock()
	leave := pit.snap.Enter()
	return func() {
		leave()
		s.mu.Unlock()
	}
}
