package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"slopensearch/internal/apierr"
	"slopensearch/internal/model"
	"slopensearch/internal/search"
)

type pitContext struct {
	idx       *model.Index
	hits      []search.MultiHit
	shards    int
	keepAlive int64
	creation  int64
	indexName string
	snap      *model.Snapshot // the versions of the hits when it was opened
}

func (s *Server) openPit(w http.ResponseWriter, r *http.Request, name string) {
	keep := r.URL.Query().Get("keep_alive")
	if keep == "" {
		writeError(w, &apierr.Error{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: keep alive not specified;"})
		return
	}
	d, ok := parseKeepAlive(keep)
	if !ok {
		writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400, Reason: "failed to parse [" + keep + "]"})
		return
	}
	if d > 24*time.Hour {
		reason := fmt.Sprintf("Keep alive for request (%s) is too large. It must be less than (1d). [point_in_time.max_keep_alive]", keep)
		writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400, Reason: reason})
		return
	}
	unlock := s.lockSearchView()
	idx, ok := s.Indices[name]
	if !ok {
		unlock()
		writeError(w, apierr.IndexNotFound(name))
		return
	}
	hits := make([]search.MultiHit, 0, len(idx.Order))
	var ids []string
	for _, id := range idx.Order {
		doc, live := idx.Docs[id]
		if !live {
			continue
		}
		if _, pending := idx.Pending[id]; pending {
			continue
		}
		hits = append(hits, search.MultiHit{Idx: idx, Id: id, Doc: doc, Score: 1})
		ids = append(ids, id)
	}
	snap := idx.Snapshot(ids)
	shards := idx.NumShards()
	creation := time.Now().UnixMilli()
	unlock()

	s.pitMu.Lock()
	s.pitCounter++
	pid := fmt.Sprintf("slopensearch-pit-%d", s.pitCounter)
	s.pits[pid] = &pitContext{idx: idx, hits: hits, shards: shards,
		keepAlive: int64(d / time.Millisecond), creation: creation, indexName: name, snap: snap}
	s.pitMu.Unlock()

	writeJSON(w, 200, map[string]any{
		"pit_id":        pid,
		"_shards":       map[string]any{"total": shards, "successful": shards, "skipped": 0, "failed": 0},
		"creation_time": creation,
	})
}

func (s *Server) getPit(id string) (*pitContext, bool) {
	s.pitMu.Lock()
	defer s.pitMu.Unlock()
	p, ok := s.pits[id]
	return p, ok
}

func (s *Server) handlePitRoute(w http.ResponseWriter, r *http.Request, rest []string) {
	switch r.Method {
	case http.MethodGet:
		s.pitMu.Lock()
		pits := make([]any, 0, len(s.pits))
		for id, p := range s.pits {
			pits = append(pits, map[string]any{
				"pit_id":        id,
				"keep_alive":    p.keepAlive,
				"creation_time": p.creation,
			})
		}
		s.pitMu.Unlock()
		writeJSON(w, 200, map[string]any{"pits": pits})
	case http.MethodDelete:
		body, apiErr := readJSON(r)
		if apiErr != nil {
			writeError(w, apiErr)
			return
		}
		var ids []string
		switch t := body["pit_id"].(type) {
		case string:
			ids = append(ids, t)
		case []any:
			for _, v := range t {
				if sv, ok := v.(string); ok {
					ids = append(ids, sv)
				}
			}
		}
		if len(ids) == 0 {
			writeError(w, &apierr.Error{Type: "action_request_validation_exception", Status: 400,
				Reason: "Validation Failed: 1: no pit ids specified;"})
			return
		}
		for _, id := range ids {
			if !strings.HasPrefix(id, "slopensearch-pit-") {
				writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400, Reason: "invalid id: [" + id + "]"})
				return
			}
		}
		out := make([]any, 0, len(ids))
		s.pitMu.Lock()
		for _, id := range ids {
			delete(s.pits, id)
			out = append(out, map[string]any{"successful": true, "pit_id": id})
		}
		s.pitMu.Unlock()
		writeJSON(w, 200, map[string]any{"pits": out})
	default:
		methodNotAllowed(w, r, "GET", "DELETE")
	}
}
