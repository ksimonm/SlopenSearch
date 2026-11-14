package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
	"slopensearch/internal/ojson"
	"slopensearch/internal/painless"
	"slopensearch/internal/search"
)

// Painless engine: script contexts, compilation cache, error rendering and
// the _scripts/painless/_execute API.

// ---- doc lookup ----

// ---- score functions ----

// ---- _execute ----

func (s *Server) handlePainlessExecute(w http.ResponseWriter, r *http.Request) {
	body, raw, apiErr := readJSONRaw(r)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		writeError(w, &apierr.Error{Type: "parse_exception", Status: 400, Reason: "request body or source parameter is required"})
		return
	}
	scriptRaw, ok := body["script"]
	if !ok {
		writeError(w, apierr.BadRequest("Required [script]"))
		return
	}
	sm, ok := scriptRaw.(map[string]any)
	if !ok {
		col := strings.Index(string(raw), `"script"`) + len(`"script"`) + 2
		writeError(w, &apierr.Error{Type: "x_content_parse_exception", Status: 400,
			Reason: "[1:" + search.Itoa(col) + "] [painless_execute_request] script doesn't support values of type: VALUE_STRING"})
		return
	}
	ctxName := "painless_test"
	if c, ok := body["context"].(string); ok {
		ctxName = c
	}
	switch ctxName {
	case "painless_test", "filter", "score":
	default:
		writeError(w, &apierr.Error{Type: "x_content_parse_exception", Status: 400,
			Reason:   "Failed to build [painless_execute_request] after last required field arrived",
			CausedBy: &apierr.Error{Type: "unsupported_operation_exception", Reason: "unsupported script context name [" + ctxName + "]"}})
		return
	}
	if _, hasID := sm["id"]; hasID {
		writeError(w, &apierr.Error{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: only inline scripts are supported;"})
		return
	}
	src, _ := sm["source"].(string)
	if len(src) > 65535 {
		writeError(w, apierr.BadRequest("exceeded max allowed inline script size in bytes [65535] with size ["+search.Itoa(len(src))+"] for script ["+src+"]"))
		return
	}
	lang, _ := sm["lang"].(string)
	switch lang {
	case "", "painless":
	case "expression":
		writeError(w, apierr.BadRequest("expression engine does not know how to handle script context ["+ctxName+"]"))
		return
	case "mustache":
		writeError(w, apierr.BadRequest("mustache engine does not know how to handle context ["+ctxName+"]"))
		return
	default:
		writeError(w, apierr.BadRequest("script_lang not supported ["+lang+"]"))
		return
	}
	params := painless.FromJSON(map[string]any{}, painless.Hash).(*painless.Map)
	if p, ok := sm["params"].(map[string]any); ok {
		params = painless.FromJSON(p, painless.Hash).(*painless.Map)
	}
	b := painless.Bindings{Vars: map[string]any{"params": params}}
	if ctxName != "painless_test" {
		setup, _ := body["context_setup"].(map[string]any)
		idxName, _ := setup["index"].(string)
		s.mu.RLock()
		idx := s.Indices[idxName]
		s.mu.RUnlock()
		if idx == nil {
			e := apierr.IndexNotFound(idxName)
			e.Fields = map[string]any{"index": idxName, "resource.id": idxName, "resource.type": "index_expression", "index_uuid": "_na_"}
			writeError(w, e)
			return
		}
		doc, _ := setup["document"].(map[string]any)
		if doc == nil {
			doc = map[string]any{}
		}
		tmp := plTempIndex(idx, doc)
		qc := search.NewQueryCtxForDoc(tmp, "_execute", doc)
		b.Vars["doc"] = search.DocLookup(qc)
		if ctxName == "score" {
			score := 0.0
			if q, ok := setup["query"].(map[string]any); ok {
				if matched, sc, qerr := qc.Eval(q); qerr == nil && matched {
					score = float64(float32(sc))
				}
			}
			b.Vars["_score"] = score
			b.Funcs = search.ScoreBindings(qc)
		}
	}
	script, cerr := search.CompileScript(src, ctxName, "")
	if cerr != nil {
		writeJSON(w, 400, cerr.ErrorBody())
		return
	}
	v, rerr := script.Exec(b, "")
	if rerr != nil {
		writeJSON(w, 400, rerr.ErrorBody())
		return
	}
	var result any
	switch ctxName {
	case "painless_test":
		result = painless.ToString(v)
	case "score":
		result = float64(float32(painless.F64(v)))
	default:
		result = painless.ToJSON(v)
	}
	writeJSON(w, 200, ojson.Object{{"result", result}})
}

// plTempIndex is a single-document copy of an index, like the in-memory
// index the execute API builds for context_setup.
func plTempIndex(idx *model.Index, doc map[string]any) *model.Index {
	tmp := &model.Index{Name: idx.Name, UUID: idx.UUID, Settings: idx.Settings,
		Mappings:   search.DeepCopySource(idx.Mappings).(map[string]any),
		Docs:       map[string]map[string]any{"_execute": doc},
		Order:      []string{"_execute"},
		DocValues:  map[string]map[string][]any{},
		Versions:   map[string]int64{"_execute": 1},
		SeqNos:     map[string]int64{"_execute": 0},
		Routing:    map[string]string{},
		FieldTypes: map[string]string{},
	}
	mapping.EnsureFieldTypes(tmp)
	_ = mapping.ComputeDocValues(tmp, "_execute", doc)
	return tmp
}

var _ = json.Marshal
