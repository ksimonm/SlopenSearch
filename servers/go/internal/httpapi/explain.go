package httpapi

import (
	"net/http"

	"slopensearch/internal/apierr"
	"slopensearch/internal/search"
)

func (s *Server) handleExplain(w http.ResponseWriter, r *http.Request, name, id string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, apiErr := s.resolveReadTarget(name)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	doc, found := idx.Docs[id]
	if !found {
		writeJSON(w, 404, map[string]any{"_index": idx.Name, "_id": id, "matched": false})
		return
	}
	body, apiErr := readJSON(r)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	query, hasQuery := body["query"]
	if qp := r.URL.Query().Get("q"); qp != "" {
		query = parseQParam(qp)
		hasQuery = true
	}
	if !hasQuery {
		writeError(w, &apierr.Error{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: query is missing;"})
		return
	}
	qmap, ok := query.(map[string]any)
	if !ok {
		writeError(w, apierr.Parsing("[query] malformed"))
		return
	}
	qc := search.NewQueryCtxForDoc(idx, id, doc)
	matched, score, qerr := qc.Eval(qmap)
	if qerr != nil {
		writeError(w, qerr)
		return
	}
	field, term := explainLabel(qmap)
	var explanation map[string]any
	if matched {
		explanation = qc.Bm25Explain(field, term, score)
	} else {
		description := "no matching term"
		if _, isTerm := qmap["term"]; isTerm {
			description = "ConstantScore(" + field + ":" + term + ") doesn't match id 0"
		}
		explanation = map[string]any{
			"value":       0.0,
			"description": description,
			"details":     []any{},
		}
	}
	out := map[string]any{"_index": idx.Name, "_id": id, "matched": matched, "explanation": explanation}
	if sourceRequested(r, body) {
		out["get"] = map[string]any{"_seq_no": idx.SeqNos[id], "_primary_term": int64(1), "found": true,
			"_source": search.FilterSource(doc, nil, nil)}
	}
	writeJSON(w, 200, out)
}

func explainLabel(query map[string]any) (string, string) {
	for clause, raw := range query {
		switch clause {
		case "match", "term":
			m, _ := raw.(map[string]any)
			for f, v := range m {
				if vm, ok := v.(map[string]any); ok {
					if val, ok := vm["query"]; ok {
						s, _ := valueToScriptString(val)
						return f, s
					}
					if val, ok := vm["value"]; ok {
						s, _ := valueToScriptString(val)
						return f, s
					}
				}
				s, _ := valueToScriptString(v)
				return f, s
			}
		}
	}
	return "_id", ""
}

func sourceRequested(r *http.Request, body map[string]any) bool {
	if v, ok := body["_source"]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
		return true
	}
	if v, ok := r.URL.Query()["_source"]; ok {
		return len(v) == 0 || v[0] != "false"
	}
	return false
}
