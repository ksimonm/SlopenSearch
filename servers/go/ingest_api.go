package main

import (
	"net/http"
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/ingest"
	"slopensearch/internal/painless"
	"slopensearch/internal/wildcard"
)

// ingestScripts compiles the scripts of script processors for the ingest
// engine.
type ingestScripts struct{}

func (ingestScripts) Compile(raw map[string]any, ctxName, tag string) (ingest.Script, *apierr.Error) {
	script, spec, err := compileIngestScript(raw, ctxName, tag)
	if err != nil {
		return nil, err
	}
	return ingestScript{script, spec}, nil
}

type ingestScript struct {
	script *plScript
	spec   *scriptSpec
}

func (sc ingestScript) Run(ctx *painless.Map) *apierr.Error {
	_, se := sc.script.exec(painless.Bindings{Vars: map[string]any{"params": scriptParams(sc.spec.params), "ctx": ctx}}, sc.spec.errName())
	if se != nil {
		return ingestScriptError(se, "")
	}
	return nil
}

func (s *Server) handleIngestRoute(w http.ResponseWriter, r *http.Request, segs []string) bool {
	if len(segs) < 2 || segs[1] != "pipeline" {
		if len(segs) >= 3 && segs[1] == "processor" && segs[2] == "grok" && r.Method == http.MethodGet {
			writeJSON(w, 200, map[string]any{"patterns": ingest.GrokPatterns()})
			return true
		}
		return false
	}
	rest := segs[2:]
	if len(rest) == 1 && rest[0] == "_simulate" {
		s.simulatePipeline(w, r, "")
		return true
	}
	if len(rest) == 2 && rest[1] == "_simulate" {
		s.simulatePipeline(w, r, rest[0])
		return true
	}
	if len(rest) == 0 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, r, "GET")
			return true
		}
		s.ingestMu.RLock()
		out := map[string]any{}
		for name, body := range s.IngestPipelines {
			out[name] = body
		}
		s.ingestMu.RUnlock()
		writeJSON(w, 200, out)
		return true
	}
	nameExpr := rest[0]
	switch r.Method {
	case http.MethodGet:
		names := s.resolvePipelineNames(nameExpr)
		if len(names) == 0 {
			if !strings.ContainsAny(nameExpr, "*,") {
				writeJSON(w, 404, map[string]any{})
			} else {
				writeJSON(w, 200, map[string]any{})
			}
			return true
		}
		s.ingestMu.RLock()
		out := map[string]any{}
		for _, n := range names {
			if b, ok := s.IngestPipelines[n]; ok {
				out[n] = b
			}
		}
		s.ingestMu.RUnlock()
		writeJSON(w, 200, out)
		return true
	case http.MethodPut:
		body, apiErr := readJSON(r)
		if apiErr != nil {
			writeError(w, apiErr)
			return true
		}
		norm, apiErr := s.ingest.ValidatePipeline(nameExpr, body)
		if apiErr != nil {
			writeError(w, apiErr)
			return true
		}
		s.ingestMu.Lock()
		s.IngestPipelines[nameExpr] = norm
		s.ingestMu.Unlock()
		writeJSON(w, 200, map[string]any{"acknowledged": true})
		return true
	case http.MethodDelete:
		s.ingestMu.Lock()
		_, ok := s.IngestPipelines[nameExpr]
		delete(s.IngestPipelines, nameExpr)
		s.ingestMu.Unlock()
		if !ok {
			writeError(w, &apierr.Error{Type: "resource_not_found_exception", Status: 404,
				Reason: "pipeline [" + nameExpr + "] is missing"})
			return true
		}
		writeJSON(w, 200, map[string]any{"acknowledged": true})
		return true
	}
	methodNotAllowed(w, r, "GET", "PUT", "DELETE")
	return true
}

func (s *Server) resolvePipelineNames(expr string) []string {
	parts := strings.Split(expr, ",")
	out := []string{}
	seen := map[string]bool{}
	s.ingestMu.RLock()
	defer s.ingestMu.RUnlock()
	for _, p := range parts {
		if strings.Contains(p, "*") {
			for n := range s.IngestPipelines {
				if wildcard.Match(p, n) && !seen[n] {
					seen[n] = true
					out = append(out, n)
				}
			}
		} else if _, ok := s.IngestPipelines[p]; ok && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) simulatePipeline(w http.ResponseWriter, r *http.Request, stored string) {
	body, apiErr := readJSON(r)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	var pipeline map[string]any
	name := stored
	if stored != "" {
		s.ingestMu.RLock()
		p, ok := s.IngestPipelines[stored]
		s.ingestMu.RUnlock()
		if !ok {
			writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
				Reason: "pipeline [" + stored + "] does not exist"})
			return
		}
		pipeline = p
	} else {
		raw, ok := body["pipeline"]
		if !ok {
			writeError(w, &apierr.Error{Type: "parse_exception", Status: 400,
				Reason: "[pipeline] required property is missing", Fields: map[string]any{"property_name": "pipeline"}})
			return
		}
		pm, ok := raw.(map[string]any)
		if !ok {
			writeError(w, &apierr.Error{Type: "parse_exception", Status: 400, Reason: "[pipeline] required property is missing",
				Fields: map[string]any{"property_name": "pipeline"}})
			return
		}
		var err *apierr.Error
		pipeline, err = s.ingest.ValidatePipeline("_simulate", pm)
		if err != nil {
			writeError(w, err)
			return
		}
		name = "_simulate_pipeline"
	}
	rawDocs, ok := body["docs"]
	if !ok {
		writeError(w, &apierr.Error{Type: "parse_exception", Status: 400,
			Reason: "[docs] required property is missing", Fields: map[string]any{"property_name": "docs"}})
		return
	}
	docs, ok := rawDocs.([]any)
	if !ok {
		writeError(w, &apierr.Error{Type: "parse_exception", Status: 400,
			Reason: "[docs] required property is missing", Fields: map[string]any{"property_name": "docs"}})
		return
	}
	if len(docs) == 0 {
		writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
			Reason: "must specify at least one document in [docs]"})
		return
	}
	out, err := s.ingest.Simulate(pipeline, name, docs, r.URL.Query().Get("verbose") == "true")
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"docs": out})
}

// ingestScriptError converts a script exception into the error an ingest
// processor reports.
func ingestScriptError(se *plScriptErr, tag string) *apierr.Error {
	fields := map[string]any{"script_stack": anySlice(se.stack), "script": se.script, "lang": map[bool]string{true: "expression", false: "painless"}[se.lang == "expression"]}
	if se.pos != nil {
		fields["position"] = map[string]any{"offset": se.pos[0], "start": se.pos[1], "end": se.pos[2]}
	}
	kind := "runtime error"
	if se.compile {
		kind = "compile error"
		fields["processor_type"] = "script"
		if tag != "" {
			fields["processor_tag"] = tag
		}
	}
	for _, kv := range se.extra {
		fields[kv.K] = kv.V
	}
	return &apierr.Error{Type: "script_exception", Status: 400, Reason: kind, Fields: fields,
		CausedBy: &apierr.Error{Type: se.causeType, Reason: se.reason}}
}

// compileIngestScript compiles a script processor's script.
func compileIngestScript(raw map[string]any, ctxName, tag string) (*plScript, *scriptSpec, *apierr.Error) {
	spec, ae := parseScriptSpec(raw)
	if ae != nil {
		return nil, nil, ae
	}
	if spec.lang == "expression" {
		_, se, ae := exPrepare(spec.src, ctxName)
		if se != nil {
			return nil, nil, ingestScriptError(se, tag)
		}
		ae.ForceTop = true
		return nil, nil, ae
	}
	if ae := checkScriptLang(spec, ctxName); ae != nil {
		ae.ForceTop = true
		return nil, nil, ae
	}
	script, se := compilePainless(spec, ctxName)
	if se != nil {
		return nil, nil, ingestScriptError(se, tag)
	}
	return script, spec, nil
}

func anySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
