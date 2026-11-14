package main

import (
	"encoding/json"
	"hash/fnv"
	"math"
	"net/http"
	"sort"
	"strings"

	"slopensearch/internal/analysis"
	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
	"slopensearch/internal/ojson"
	"slopensearch/internal/painless"
)

// Painless engine: script contexts, compilation cache, error rendering and
// the _scripts/painless/_execute API.

// plScriptErr is a script_exception: a compile or runtime error with the
// script position.
type plScriptErr struct {
	compile   bool
	linkErr   bool
	lang      string
	script    string
	stack     []string
	pos       []int // offset, start, end; nil when unknown
	causeType string
	reason    string
	nilReason bool
	inner     *painless.CompileError
	extra     ojson.Object
}

func (e *plScriptErr) object(withCause bool) ojson.Object {
	kind := "runtime error"
	if e.compile {
		kind = "compile error"
	}
	if e.linkErr {
		kind = "link error"
	}
	lang := e.lang
	if lang == "" {
		lang = "painless"
	}
	o := ojson.Object{{"type", "script_exception"}, {"reason", kind}}
	o = append(o, e.extra...)
	stack := make([]any, len(e.stack))
	for i, s := range e.stack {
		stack[i] = s
	}
	o = append(o, ojson.KV{"script_stack", stack}, ojson.KV{"script", e.script}, ojson.KV{"lang", lang})
	if e.pos != nil {
		o = append(o, ojson.KV{"position", ojson.Object{{"offset", e.pos[0]}, {"start", e.pos[1]}, {"end", e.pos[2]}}})
	}
	if withCause {
		o = append(o, ojson.KV{"caused_by", e.causeObject()})
	}
	return o
}

func (e *plScriptErr) causeObject() ojson.Object {
	var reason any = e.reason
	if e.nilReason {
		reason = nil
	}
	c := ojson.Object{{"type", e.causeType}, {"reason", reason}}
	if e.inner != nil {
		var ir any = e.inner.Reason
		if e.inner.NilReason {
			ir = nil
		}
		c = append(c, ojson.KV{"caused_by", ojson.Object{{"type", e.inner.Type}, {"reason", ir}}})
	}
	return c
}

// errorBody renders the full {"error": ..., "status": 400} response.
func (e *plScriptErr) errorBody() ojson.Object {
	errObj := ojson.Object{{"root_cause", []any{e.object(false)}}}
	errObj = append(errObj, e.object(true)...)
	return ojson.Object{{"error", errObj}, {"status", 400}}
}

// asAPIError wraps the script error for callers that report apiErrors.
func (e *plScriptErr) asAPIError() *apierr.Error {
	return &apierr.Error{Type: "script_exception", Status: 400, Reason: map[bool]string{true: "compile error", false: "runtime error"}[e.compile],
		Extra: e, CausedBy: &apierr.Error{Type: e.causeType, Reason: e.reason}}
}

func plCompileFailure(src, name string, ce *painless.CompileError) *plScriptErr {
	e := &plScriptErr{compile: true, script: name, causeType: ce.Type, reason: ce.Reason, nilReason: ce.NilReason, inner: ce.Cause, stack: []string{}}
	if ce.NoPos {
		return e
	}
	off := ce.Offset
	start := off - 25
	if start < 0 {
		start = 0
	}
	end := off + 25
	if end > len(src) {
		end = len(src)
	}
	snippet := src[start:end]
	pad := off - start
	if start > 0 {
		snippet = "... " + snippet
		pad += 4
	}
	if end < len(src) {
		snippet += " ..."
	}
	e.stack = []string{snippet, strings.Repeat(" ", pad) + "^---- HERE"}
	e.pos = []int{off, start, end}
	return e
}

func plRuntimeFailure(s *plScript, name string, re *painless.RuntimeError) *plScriptErr {
	e := &plScriptErr{script: name, causeType: re.Type, reason: re.Reason, nilReason: re.NilReason}
	for _, f := range re.Extra {
		e.extra = append(e.extra, ojson.KV{f.Name, f.Value})
	}
	src := s.s.Source()
	off := re.Offset
	if off > len(src) {
		off = len(src)
	}
	start, end := s.s.StatementAround(off)
	e.stack = append(append([]string{}, re.Frames...), src[start:end], strings.Repeat(" ", off-start)+"^---- HERE")
	e.pos = []int{off, start, end}
	return e
}

// plScript is a compiled Painless script.
type plScript struct {
	s *painless.Script
}

// plCompileScript compiles and converts errors to script exceptions.
func plCompileScript(src, ctxName, name string) (*plScript, *plScriptErr) {
	s, err := painless.Compile(src, ctxName)
	if err != nil {
		if name == "" {
			name = src
		}
		return nil, plCompileFailure(src, name, err)
	}
	return &plScript{s}, nil
}

// exec runs a compiled script and applies the context's return conversion.
func (s *plScript) exec(b painless.Bindings, name string) (any, *plScriptErr) {
	v, rerr := s.s.Run(b)
	if rerr != nil {
		if name == "" {
			name = s.s.Source()
		}
		return nil, plRuntimeFailure(s, name, rerr)
	}
	return v, nil
}

// ---- doc lookup ----

var plTextFielddataReason = "Text fields are not optimised for operations that require per-document field data like aggregations and sorting, so these operations are disabled by default. Please use a keyword field instead. Alternatively, set fielddata=true on [%s] in order to load field data by uninverting the inverted index. Note that this can use significant memory."

// plDocLookup builds the doc variable for one document.
func plDocLookup(qc *queryCtx) *painless.DocMap {
	cache := map[string]*painless.DocValues{}
	has := func(field string) bool {
		if field == "_id" || field == "_index" || field == "_seq_no" || field == "_version" {
			return true
		}
		_, ok := mapping.FieldDef(qc.idx, field)
		return ok
	}
	get := func(field string, off int) (any, *painless.RuntimeError) {
		if dv, ok := cache[field]; ok {
			return dv, nil
		}
		dv, err := plDocValuesFor(qc, field, off)
		if err != nil {
			return nil, err
		}
		cache[field] = dv
		return dv, nil
	}
	return &painless.DocMap{Get: get, Has: has}
}

func plDocValuesFor(qc *queryCtx, field string, off int) (*painless.DocValues, *painless.RuntimeError) {
	switch field {
	case "_id":
		return &painless.DocValues{Kind: "Strings", Field: field, Values: []any{qc.id}}, nil
	case "_index":
		return &painless.DocValues{Kind: "Strings", Field: field, Values: []any{qc.idx.Name}}, nil
	case "_seq_no":
		return &painless.DocValues{Kind: "Longs", Field: field, Values: []any{qc.idx.SeqNos[qc.id]}}, nil
	}
	def, ok := mapping.FieldDef(qc.idx, field)
	if !ok {
		return nil, &painless.RuntimeError{Type: "illegal_argument_exception", Offset: off,
			Reason: "No field found for [" + field + "] in mapping",
			Frames: []string{"org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:89)",
				"org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:55)"}}
	}
	ft, _ := def["type"].(string)
	if ft == "alias" {
		return plDocValuesFor(qc, mapping.ResolveAliasField(qc.idx, field), off)
	}
	vals, _ := qc.values(field)
	if ft == "knn_vector" {
		return plKnnDocValues(field, vals), nil
	}
	dv := &painless.DocValues{Field: field}
	switch ft {
	case "text", "match_only_text", "search_as_you_type":
		if fd, _ := def["fielddata"].(bool); !fd {
			return nil, &painless.RuntimeError{Type: "illegal_argument_exception", Offset: off,
				Reason: strings.Replace(plTextFielddataReason, "%s", field, 1),
				Frames: []string{
					"org.opensearch.index.mapper.TextFieldMapper$TextFieldType.fielddataBuilder(TextFieldMapper.java:1004)",
					"org.opensearch.index.fielddata.IndexFieldDataService.getForField(IndexFieldDataService.java:134)",
					"org.opensearch.index.query.QueryShardContext.lambda$lookup$0(QueryShardContext.java:530)",
					"org.opensearch.search.lookup.SearchLookup.lambda$new$0(SearchLookup.java:118)",
					"org.opensearch.search.lookup.LeafDocLookup.lambda$get$0(LeafDocLookup.java:93)",
					"org.opensearch.secure_sm.AccessController.doPrivileged(AccessController.java:76)",
					"org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:93)",
					"org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:55)"}}
		}
		analyzer, _ := def["analyzer"].(string)
		terms := []string{}
		seen := map[string]bool{}
		for _, v := range vals {
			s, _ := jsonv.ScalarString(v)
			for _, t := range analysis.Tokens(mapping.AnalysisIndex(qc.idx), analyzer, s) {
				if !seen[t.Term] {
					seen[t.Term] = true
					terms = append(terms, t.Term)
				}
			}
		}
		sort.Strings(terms)
		dv.Kind = "Strings"
		for _, t := range terms {
			dv.Values = append(dv.Values, t)
		}
		return dv, nil
	case "integer", "long", "short", "byte", "unsigned_long", "token_count":
		dv.Kind = "Longs"
		for _, v := range vals {
			if f, ok := jsonv.ToFloat(v); ok {
				dv.Values = append(dv.Values, int64(f))
			}
		}
	case "float", "double", "half_float", "scaled_float":
		dv.Kind = "Doubles"
		for _, v := range vals {
			if f, ok := jsonv.ToFloat(v); ok {
				dv.Values = append(dv.Values, f)
			}
		}
	case "boolean":
		dv.Kind = "Booleans"
		for _, v := range vals {
			if b, ok := v.(bool); ok {
				dv.Values = append(dv.Values, b)
			}
		}
		sort.SliceStable(dv.Values, func(i, j int) bool { return !dv.Values[i].(bool) && dv.Values[j].(bool) })
		return dv, nil
	case "date", "date_nanos":
		dv.Kind = "Dates"
		ms := []int64{}
		for _, v := range vals {
			if f, ok := jsonv.ToFloat(v); ok {
				ms = append(ms, int64(f))
			}
		}
		sort.Slice(ms, func(i, j int) bool { return ms[i] < ms[j] })
		for _, m := range ms {
			dv.Values = append(dv.Values, painless.NewZDT(painless.TimeFromMillis(m)))
		}
		return dv, nil
	case "geo_point":
		dv.Kind = "GeoPoints"
		for _, v := range vals {
			if m, ok := v.(map[string]any); ok {
				lat, _ := jsonv.ToFloat(m["lat"])
				lon, _ := jsonv.ToFloat(m["lon"])
				dv.Values = append(dv.Values, &painless.GeoPoint{Lat: lat, Lon: lon})
			}
		}
		return dv, nil
	default:
		dv.Kind = "Strings"
		for _, v := range vals {
			s, ok := jsonv.ScalarString(v)
			if !ok {
				s = painless.ToString(v)
			}
			dv.Values = append(dv.Values, s)
		}
		sort.SliceStable(dv.Values, func(i, j int) bool { return dv.Values[i].(string) < dv.Values[j].(string) })
		return dv, nil
	}
	sort.SliceStable(dv.Values, func(i, j int) bool { c, _ := painless.Compare(dv.Values[i], dv.Values[j]); return c < 0 })
	return dv, nil
}

// ---- score functions ----

func plScoreBindings(qc *queryCtx) map[string]func(args []any, off int) (any, *painless.RuntimeError) {
	f := func(a []any, i int) float64 { return painless.F64(a[i]) }
	dist := func(a []any) float64 {
		d := math.Abs(f(a, 4)-f(a, 0)) - f(a, 2)
		if d < 0 {
			d = 0
		}
		return d
	}
	m := plKnnBindings()
	for k, v := range map[string]func(args []any, off int) (any, *painless.RuntimeError){
		"saturation": func(a []any, off int) (any, *painless.RuntimeError) { return f(a, 0) / (f(a, 1) + f(a, 0)), nil },
		"sigmoid": func(a []any, off int) (any, *painless.RuntimeError) {
			v, k, e := f(a, 0), f(a, 1), f(a, 2)
			return math.Pow(v, e) / (math.Pow(k, e) + math.Pow(v, e)), nil
		},
		"decayNumericLinear": func(a []any, off int) (any, *painless.RuntimeError) {
			scaling := f(a, 1) / (1.0 - f(a, 3))
			return math.Max(0.0, (scaling-dist(a))/scaling), nil
		},
		"decayNumericExp": func(a []any, off int) (any, *painless.RuntimeError) {
			scaling := math.Log(f(a, 3)) / f(a, 1)
			return math.Exp(scaling * dist(a)), nil
		},
		"decayNumericGauss": func(a []any, off int) (any, *painless.RuntimeError) {
			scaling := 0.5 * math.Pow(f(a, 1), 2.0) / math.Log(f(a, 3))
			return math.Exp(0.5 * math.Pow(dist(a), 2.0) / scaling), nil
		},
		"randomScore": func(a []any, off int) (any, *painless.RuntimeError) {
			h := fnv.New32a()
			key := qc.id
			if len(a) == 2 {
				if vals, ok := qc.values(painless.ToString(a[1])); ok && len(vals) > 0 {
					key = painless.ToString(painless.FromJSON(vals[0], painless.Hash))
				}
			}
			h.Write([]byte(itoa(int(painless.I64(a[0]))) + "\x00" + key))
			return float64(h.Sum32()&0x00FFFFFF) / float64(1<<24), nil
		},
	} {
		m[k] = v
	}
	return m
}

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
			Reason: "[1:" + itoa(col) + "] [painless_execute_request] script doesn't support values of type: VALUE_STRING"})
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
		writeError(w, apierr.BadRequest("exceeded max allowed inline script size in bytes [65535] with size ["+itoa(len(src))+"] for script ["+src+"]"))
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
			e := errNotFound(idxName)
			e.Fields = map[string]any{"index": idxName, "resource.id": idxName, "resource.type": "index_expression", "index_uuid": "_na_"}
			writeError(w, e)
			return
		}
		doc, _ := setup["document"].(map[string]any)
		if doc == nil {
			doc = map[string]any{}
		}
		tmp := plTempIndex(idx, doc)
		qc := newQueryCtxForDoc(tmp, "_execute", doc)
		b.Vars["doc"] = plDocLookup(qc)
		if ctxName == "score" {
			score := 0.0
			if q, ok := setup["query"].(map[string]any); ok {
				if matched, sc, qerr := qc.eval(q); qerr == nil && matched {
					score = float64(float32(sc))
				}
			}
			b.Vars["_score"] = score
			b.Funcs = plScoreBindings(qc)
		}
	}
	script, cerr := plCompileScript(src, ctxName, "")
	if cerr != nil {
		writeJSON(w, 400, cerr.errorBody())
		return
	}
	v, rerr := script.exec(b, "")
	if rerr != nil {
		writeJSON(w, 400, rerr.errorBody())
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
		Mappings:   deepCopySource(idx.Mappings).(map[string]any),
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
