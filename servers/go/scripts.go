package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/painless"
	"slopensearch/internal/search"
)

// Stored scripts, _render and script fields.

// handleScriptsRoute dispatches /_scripts/* and /_render/* endpoints.
func (s *Server) handleScriptsRoute(w http.ResponseWriter, r *http.Request, segs []string) bool {
	if segs[0] != "_scripts" || len(segs) < 2 {
		return false
	}
	if len(segs) == 3 && segs[1] == "painless" && segs[2] == "_execute" {
		s.handlePainlessExecute(w, r)
		return true
	}
	id := strings.Join(segs[1:], "/")
	switch r.Method {
	case http.MethodPut, http.MethodPost:
		ctxName := ""
		if len(segs) == 3 {
			id, ctxName = segs[1], segs[2]
		}
		body, apiErr := readJSON(r)
		if apiErr != nil {
			writeError(w, apiErr)
			return true
		}
		sc, verr := buildStoredScript(body)
		if verr != nil {
			writeError(w, verr)
			return true
		}
		if ctxName != "" {
			if !knownScriptContext(ctxName) {
				writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
					Reason:   "failed to parse/compile stored script [" + id + "]",
					CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: "Unknown context [" + ctxName + "]"}})
				return true
			}
			src, _ := sc["source"].(string)
			switch sc["lang"] {
			case "painless":
				if _, se := search.CompileScript(src, ctxName, ""); se != nil {
					writeJSON(w, 400, se.ErrorBody())
					return true
				}
			case "expression":
				if _, se, ae := search.PrepareExpression(src, ctxName); se != nil || ae != nil {
					if se != nil {
						writeJSON(w, 400, se.ErrorBody())
					} else {
						writeError(w, ae)
					}
					return true
				}
			}
		}
		s.scriptsMu.Lock()
		s.Scripts[id] = sc
		s.scriptsMu.Unlock()
		writeJSON(w, 200, map[string]any{"acknowledged": true})
		return true
	case http.MethodGet:
		s.scriptsMu.RLock()
		sc, ok := s.Scripts[id]
		s.scriptsMu.RUnlock()
		if !ok {
			writeJSON(w, 404, map[string]any{"_id": id, "found": false})
			return true
		}
		writeJSON(w, 200, map[string]any{"_id": id, "found": true, "script": sc})
		return true
	case http.MethodDelete:
		s.scriptsMu.Lock()
		_, ok := s.Scripts[id]
		delete(s.Scripts, id)
		s.scriptsMu.Unlock()
		if !ok {
			writeError(w, &apierr.Error{Type: "resource_not_found_exception", Status: 404,
				Reason: "stored script [" + id + "] does not exist and cannot be deleted"})
			return true
		}
		writeJSON(w, 200, map[string]any{"acknowledged": true})
		return true
	}
	methodNotAllowed(w, r, "GET", "PUT", "POST", "DELETE")
	return true
}

// runPainless executes a painless/expression script against a context map.
func runPainless(source string, ctx map[string]any) (any, *apierr.Error) {
	return nil, nil
}

// buildStoredScript validates a stored script body like StoredScriptSource.
func buildStoredScript(body map[string]any) (map[string]any, *apierr.Error) {
	raw, has := body["script"]
	if !has {
		if len(body) == 0 {
			return map[string]any{"lang": "mustache", "source": ""}, nil
		}
		raw = body
	}
	sc, ok := raw.(map[string]any)
	if !ok {
		return nil, apierr.Parsing("unexpected token [VALUE_STRING], expected [{, <source>]")
	}
	if len(sc) == 0 {
		return map[string]any{"lang": "mustache", "source": ""}, nil
	}
	lang, hasLang := sc["lang"].(string)
	if !hasLang {
		return nil, apierr.BadRequest("must specify lang for stored script")
	}
	switch lang {
	case "painless", "expression", "mustache":
	default:
		return nil, apierr.BadRequest("unable to put stored script with unsupported lang [" + lang + "]")
	}
	srcRaw, hasSrc := sc["source"]
	if !hasSrc {
		return nil, apierr.BadRequest("must specify source for stored script")
	}
	out := map[string]any{"lang": lang}
	switch src := srcRaw.(type) {
	case string:
		if src == "" && lang != "mustache" {
			return nil, apierr.BadRequest("source cannot be empty")
		}
		out["source"] = src
	case map[string]any:
		enc, _ := json.Marshal(src)
		out["source"] = string(enc)
		if lang == "mustache" {
			out["options"] = map[string]any{"content_type": "application/json; charset=UTF-8"}
		}
	default:
		out["source"] = painless.ToString(painless.FromJSON(srcRaw, painless.Linked))
	}
	if opts, ok := sc["options"].(map[string]any); ok {
		extra := painless.NewMap(painless.Hash)
		for k, v := range opts {
			if k != "content_type" {
				extra.Put(k, painless.FromJSON(v, painless.Linked))
			}
		}
		if extra.Size() > 0 {
			return nil, apierr.BadRequest("illegal compiler options [" + painless.ToString(extra) + "] specified")
		}
		if ct, ok := opts["content_type"]; ok {
			out["options"] = map[string]any{"content_type": ct}
		}
	}
	return out, nil
}

var scriptContextNames = func() map[string]bool {
	var listing struct {
		Contexts []struct {
			Name string `json:"name"`
		} `json:"contexts"`
	}
	_ = json.Unmarshal([]byte(scriptContextListing), &listing)
	out := map[string]bool{}
	for _, c := range listing.Contexts {
		out[c.Name] = true
	}
	return out
}()

func knownScriptContext(name string) bool { return scriptContextNames[name] }
