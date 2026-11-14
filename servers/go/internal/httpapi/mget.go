package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/search"
)

var mgetParams = map[string]bool{
	"realtime": true, "preference": true, "refresh": true, "routing": true,
	"_source": true, "_source_includes": true, "_source_excludes": true,
	"stored_fields": true, "error_trace": true, "human": true, "pretty": true,
	"filter_path": true,
}

func (s *Server) handleMget(w http.ResponseWriter, r *http.Request, pathIndex string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		methodNotAllowed(w, r, "GET", "POST")
		return
	}
	if err := unknownParams(r, mgetParams); err != nil {
		writeError(w, err)
		return
	}
	body, _, berr := readDocBody(r)
	if berr != nil {
		writeError(w, berr)
		return
	}
	for k := range body {
		if k != "docs" && k != "ids" {
			writeError(w, &apierr.Error{Type: "parsing_exception", Status: 400,
				Reason: "unknown key [" + k + "] for a START_ARRAY, expected [docs] or [ids]"})
			return
		}
	}
	var entries []map[string]any
	if raw, ok := body["docs"]; ok {
		arr, _ := raw.([]any)
		if len(arr) == 0 {
			writeError(w, &apierr.Error{Type: "action_request_validation_exception", Status: 400,
				Reason: "Validation Failed: 1: no documents to get;"})
			return
		}
		for i, e := range arr {
			m, ok := e.(map[string]any)
			if !ok {
				writeError(w, &apierr.Error{Type: "action_request_validation_exception", Status: 400,
					Reason: fmt.Sprintf("Validation Failed: 1: id is missing for doc %d;", i)})
				return
			}
			if _, ok := m["_id"]; !ok {
				writeError(w, &apierr.Error{Type: "action_request_validation_exception", Status: 400,
					Reason: fmt.Sprintf("Validation Failed: 1: id is missing for doc %d;", i)})
				return
			}
			if pathIndex == "" {
				if _, ok := m["_index"]; !ok {
					writeError(w, &apierr.Error{Type: "action_request_validation_exception", Status: 400,
						Reason: fmt.Sprintf("Validation Failed: 1: index is missing for doc %d;", i)})
					return
				}
			}
			entries = append(entries, m)
		}
	} else if raw, ok := body["ids"]; ok {
		arr, _ := raw.([]any)
		if len(arr) == 0 || pathIndex == "" {
			if pathIndex == "" {
				writeError(w, &apierr.Error{Type: "action_request_validation_exception", Status: 400,
					Reason: "Validation Failed: 1: index is missing for doc 0;"})
				return
			}
			writeError(w, &apierr.Error{Type: "action_request_validation_exception", Status: 400,
				Reason: "Validation Failed: 1: no documents to get;"})
			return
		}
		for _, e := range arr {
			id, _ := valueToScriptString(e)
			entries = append(entries, map[string]any{"_id": id})
		}
	} else {
		writeError(w, &apierr.Error{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: no documents to get;"})
		return
	}
	urlInc, urlExc, urlOmit, _ := sourceFilterFromQuery(r.URL.Query())
	urlStored := r.URL.Query().Get("stored_fields")
	realtime := r.URL.Query().Get("realtime") != "false"
	docs := make([]any, 0, len(entries))
	for _, e := range entries {
		idxName, _ := e["_index"].(string)
		if idxName == "" {
			idxName = pathIndex
		}
		id, _ := e["_id"].(string)
		doc := map[string]any{"_index": idxName, "_id": id}
		idx, ok := s.Indices[idxName]
		if !ok {
			err := apierr.IndexNotFound(idxName)
			doc["error"] = errorObject(err)
			docs = append(docs, doc)
			continue
		}
		routing, _ := e["routing"].(string)
		if idx.RoutingRequired() && routing == "" {
			doc["error"] = errorObject(routingMissing(idx, id))
			docs = append(docs, doc)
			continue
		}
		src, found := idx.Docs[id]
		if !found || !idx.RoutingMatches(id, routing) {
			doc["found"] = false
			docs = append(docs, doc)
			continue
		}
		if !realtime {
			if _, pending := idx.Pending[id]; pending {
				doc["found"] = false
				docs = append(docs, doc)
				continue
			}
		}
		doc["_version"] = idx.CurrentVersion(id)
		doc["_seq_no"] = idx.SeqNos[id]
		term := idx.PrimaryTerms[id]
		if term == 0 {
			term = 1
		}
		doc["_primary_term"] = term
		doc["found"] = true
		inc, exc, omit := urlInc, urlExc, urlOmit
		if v, ok := e["_source"]; ok {
			i2, e2, o2 := sourceSpecFromValue(v)
			inc, exc, omit = i2, e2, o2
		}
		if !idx.SourceEnabled() {
			omit = true
		}
		storedRaw := urlStored
		if v, ok := e["stored_fields"]; ok {
			switch t := v.(type) {
			case []any:
				var parts []string
				for _, x := range t {
					if s, ok := x.(string); ok {
						parts = append(parts, s)
					}
				}
				storedRaw = strings.Join(parts, ",")
			case string:
				storedRaw = t
			}
		}
		if storedRaw != "" {
			omit = true
		}
		if !omit {
			doc["_source"] = search.FilterSource(src, inc, exc)
		}
		if fields := storedFieldsFor(idx, src, storedSpec(storedRaw, idx)); fields != nil {
			doc["fields"] = fields
		}
		if rr, ok := idx.Routing[id]; ok && rr != "" {
			doc["_routing"] = rr
		}
		docs = append(docs, doc)
	}
	writeJSON(w, 200, map[string]any{"docs": docs})
}

func errorObject(e *apierr.Error) map[string]any {
	rc := e
	if e.RootCause != nil {
		rc = e.RootCause
	}
	out := map[string]any{"type": e.Type, "reason": e.Reason,
		"root_cause": []any{map[string]any{"type": rc.Type, "reason": rc.Reason}}}
	if e.Fields != nil {
		for k, v := range e.Fields {
			out[k] = v
		}
	}
	if e.CausedBy != nil {
		out["caused_by"] = errorObject(e.CausedBy)
	}
	return out
}
