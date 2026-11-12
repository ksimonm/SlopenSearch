package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
)

// resolveTemplate finds the template text of a render / search template
// request: an inline source (which wins), the legacy "inline" key or a
// stored script id.
func (s *Server) resolveTemplate(body map[string]any) (text string, rendered bool, err *apiError) {
	source, hasSource := body["source"]
	if !hasSource {
		source, hasSource = body["inline"]
	}
	if hasSource {
		text, ok := mustacheTemplateText(source)
		if !ok {
			return "", false, errParsing("template source must be a string or object")
		}
		return text, false, nil
	}
	id, _ := body["id"].(string)
	if id == "" {
		return "", false, &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: template is missing;2: template's script type is missing;"}
	}
	sc, serr := storedScript(id)
	if serr != nil {
		return "", false, serr
	}
	src, _ := sc["source"].(string)
	if lang, _ := sc["lang"].(string); lang != "" && lang != "mustache" {
		if lang == "painless" {
			script, se := plCompileScript(src, "template", id)
			if se != nil {
				return "", false, &apiError{Type: "script_exception", Status: 400, Reason: "compile error",
					Shard: &shardFailure{raw: se.errorBody(), status: 400}}
			}
			v, se := script.exec(plBindings{vars: map[string]any{"params": scriptParams(templateParams(body))}}, id)
			if se != nil {
				return "", false, &apiError{Type: "script_exception", Status: 400, Reason: "runtime error",
					Shard: &shardFailure{raw: se.errorBody(), status: 400}}
			}
			return plToString(v), true, nil
		}
		return "", false, errBad(lang + " engine does not know how to handle context [template]")
	}
	return src, false, nil
}

func templateParams(body map[string]any) map[string]any {
	params, _ := body["params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
	}
	return params
}

// renderTemplateText renders the template of a request to text.
func (s *Server) renderTemplateText(body map[string]any) (string, *apiError) {
	text, rendered, err := s.resolveTemplate(body)
	if err != nil || rendered {
		return text, err
	}
	out, merr := mustacheRender(text, templateParams(body))
	if merr != nil {
		return "", mustacheScriptError(text, merr)
	}
	return out, nil
}

var searchTemplateKeys = map[string]bool{"source": true, "inline": true, "id": true, "params": true,
	"explain": true, "profile": true, "file": true}

// renderTemplateBody renders a search template into a search request body.
func (s *Server) renderTemplateBody(body map[string]any) (map[string]any, *apiError) {
	text, err := s.renderTemplateText(body)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if uerr := unmarshalJSON([]byte(text), &out); uerr != nil || out == nil {
		reason := "Unexpected character: expected a value"
		if uerr != nil {
			reason = uerr.Error()
		}
		return nil, &apiError{Type: "json_parse_exception", Status: 400, Reason: reason,
			CausedBy: &apiError{Type: "stream_read_exception", Reason: reason}}
	}
	for _, k := range []string{"size", "from", "terminate_after"} {
		if str, ok := out[k].(string); ok {
			n, perr := strconv.Atoi(strings.TrimSpace(str))
			if perr != nil {
				return nil, &apiError{Type: "number_format_exception", Status: 400, Reason: `For input string: "` + str + `"`}
			}
			out[k] = json.Number(strconv.Itoa(n))
		}
	}
	for k, v := range body {
		if k == "explain" || k == "profile" {
			out[k] = v
		}
	}
	return out, nil
}

func (s *Server) executeSearchRaw(r *http.Request, name string, searchBody map[string]any) (int, []byte) {
	bodyBytes, _ := json.Marshal(searchBody)
	target := "/" + name + "/_search"
	if name == "" {
		target = "/_search"
	}
	req := httptest.NewRequest(http.MethodPost, target+"?"+r.URL.RawQuery, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleSearch(rec, req, name)
	return rec.Code, rec.Body.Bytes()
}

func (s *Server) executeTemplateSearch(r *http.Request, name string, tplBody map[string]any) (int, []byte, *apiError) {
	searchBody, apiErr := s.renderTemplateBody(tplBody)
	if apiErr != nil {
		return 0, nil, apiErr
	}
	code, out := s.executeSearchRaw(r, name, searchBody)
	return code, out, nil
}

func (s *Server) handleSearchTemplate(w http.ResponseWriter, r *http.Request, name string) {
	body, raw, apiErr := readJSONRaw(r)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	for _, k := range sortedKeys(body) {
		if !searchTemplateKeys[k] {
			col := strings.Index(string(raw), `"`+k+`"`) + 1
			writeError(w, &apiError{Type: "x_content_parse_exception", Status: 400,
				Reason: "[1:" + itoa(col) + "] [search_template] unknown field [" + k + "]"})
			return
		}
	}
	code, out, apiErr := s.executeTemplateSearch(r, name, body)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if code == 200 {
		if t := bytes.TrimSpace(out); len(t) > 0 && t[len(t)-1] == '}' {
			out = append(append(append([]byte{}, t[:len(t)-1]...), []byte(`,"status":200`)...), '}')
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(out)
}

func (s *Server) handleRenderTemplate(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		methodNotAllowed(w, r, "POST", "GET")
		return
	}
	body, apiErr := readJSON(r)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if id != "" {
		body["id"] = id
	}
	out, apiErr := s.renderTemplateText(body)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	// the rendered template is written verbatim, valid JSON or not
	writeJSON(w, 200, verbatimJSON(`{"template_output":`+out+`}`))
}

func parseMsearchPairs(raw []byte) ([][2]map[string]any, *apiError) {
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		return nil, &apiError{Type: "illegal_argument_exception", Status: 400,
			Reason: "The msearch request must be terminated by a newline [\n]"}
	}
	lines := strings.Split(string(raw), "\n")
	lines = lines[:len(lines)-1]
	pairs := [][2]map[string]any{}
	for i := 0; i+1 < len(lines); i += 2 {
		var header, body map[string]any
		if err := json.Unmarshal([]byte(lines[i]), &header); err != nil {
			return nil, errParsing("Failed to parse request body")
		}
		if err := json.Unmarshal([]byte(lines[i+1]), &body); err != nil {
			return nil, errParsing("Failed to parse request body")
		}
		for k := range header {
			if !msearchHeaderKeys[k] {
				return nil, &apiError{Type: "illegal_argument_exception", Status: 400,
					Reason: "key [" + k + "] is not supported in the metadata section"}
			}
		}
		pairs = append(pairs, [2]map[string]any{header, body})
	}
	if len(pairs) == 0 {
		return nil, &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: no requests added;"}
	}
	return pairs, nil
}

func (s *Server) handleMsearchTemplate(w http.ResponseWriter, r *http.Request, pathIndex string) {
	raw, err := ioReadAll(r)
	if err != nil {
		writeError(w, &apiError{Type: "illegal_argument_exception", Status: 400, Reason: "failed to read request body"})
		return
	}
	pairs, apiErr := parseMsearchPairs(raw)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	responses := []any{}
	for _, pr := range pairs {
		idx := pathIndex
		if iv, ok := pr[0]["index"]; ok {
			switch t := iv.(type) {
			case string:
				idx = t
			case []any:
				parts := []string{}
				for _, e := range t {
					if sv, ok := toString(e); ok && sv != "" {
						parts = append(parts, sv)
					}
				}
				idx = strings.Join(parts, ",")
			}
		}
		code, out, apiErr := s.executeTemplateSearch(r, idx, pr[1])
		var item map[string]any
		if apiErr != nil {
			item = errorJSON(apiErr)
			code = apiErr.Status
		} else {
			_ = json.Unmarshal(out, &item)
			if item == nil {
				item = map[string]any{}
			}
		}
		item["status"] = code
		responses = append(responses, item)
	}
	writeJSON(w, 200, map[string]any{"took": 1, "responses": responses})
}

func ioReadAll(r *http.Request) ([]byte, error) {
	var b bytes.Buffer
	_, err := b.ReadFrom(r.Body)
	return b.Bytes(), err
}

func errorJSON(e *apiError) map[string]any {
	status := e.Status
	if status == 0 {
		status = 400
	}
	body := map[string]any{"error": map[string]any{"type": e.Type, "reason": e.Reason}, "status": status}
	if e.CausedBy != nil {
		body["error"].(map[string]any)["caused_by"] = map[string]any{"type": e.CausedBy.Type, "reason": e.CausedBy.Reason}
	}
	if e.Fields != nil {
		body["error"].(map[string]any)["lang"] = e.Fields["lang"]
	}
	return body
}
