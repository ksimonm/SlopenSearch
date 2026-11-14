package main

import (
	"encoding/json"
	"hash/fnv"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// Painless engine: script contexts, compilation cache, error rendering and
// the _scripts/painless/_execute API.

var plScoreFuncs = plParseSigs(
	"saturation:double(double,double)", "sigmoid:double(double,double,double)",
	"decayNumericGauss:double(double,double,double,double,double)",
	"decayNumericExp:double(double,double,double,double,double)",
	"decayNumericLinear:double(double,double,double,double,double)",
	"decayGeoGauss:double(String,String,String,double,def)", "decayGeoExp:double(String,String,String,double,def)",
	"decayGeoLinear:double(String,String,String,double,def)",
	"decayDateGauss:double(String,String,String,double,def)", "decayDateExp:double(String,String,String,double,def)",
	"decayDateLinear:double(String,String,String,double,def)",
	"randomScore:double(int)", "randomScore:double(int,String)",
)

func plVars(kv ...string) map[string]string {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return m
}

var plContexts = map[string]*plCheckCtx{
	"painless_test":         {vars: plVars("params", "Map"), ret: "Object"},
	"filter":                {vars: plVars("params", "Map", "doc", "Map"), ret: "boolean"},
	"score":                 {vars: plVars("params", "Map", "doc", "Map", "_score", "double"), funcs: plScoreFuncs, ret: "double"},
	"field":                 {vars: plVars("params", "Map", "doc", "Map"), ret: "Object"},
	"number_sort":           {vars: plVars("params", "Map", "doc", "Map", "_score", "double"), ret: "double"},
	"string_sort":           {vars: plVars("params", "Map", "doc", "Map", "_score", "double"), ret: "String"},
	"aggs":                  {vars: plVars("params", "Map", "doc", "Map", "_value", "def", "_score", "double"), ret: "Object"},
	"aggs_init":             {vars: plVars("params", "Map", "state", "Map"), ret: "void"},
	"aggs_map":              {vars: plVars("params", "Map", "state", "Map", "doc", "Map", "_score", "double"), ret: "void"},
	"aggs_combine":          {vars: plVars("params", "Map", "state", "Map"), ret: "Object"},
	"aggs_reduce":           {vars: plVars("params", "Map", "states", "List"), ret: "Object"},
	"bucket_aggregation":    {vars: plVars("params", "Map"), ret: "Object"},
	"aggregation_selector":  {vars: plVars("params", "Map"), ret: "boolean"},
	"terms_set":             {vars: plVars("params", "Map", "doc", "Map"), ret: "Object"},
	"update":                {vars: plVars("params", "Map", "ctx", "Map"), ret: "void"},
	"ingest":                {vars: plVars("params", "Map", "ctx", "Map"), ret: "void"},
	"processor_conditional": {vars: plVars("params", "Map", "ctx", "Map"), ret: "boolean"},
	"template":              {vars: plVars("params", "Map"), ret: "String"},
}

type plScript struct {
	src  string
	prog *plProgram
	info *plInfo
	ctx  *plCheckCtx
}

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
	inner     *plCompileErr
	extra     orderedJSON
}

func (e *plScriptErr) object(withCause bool) orderedJSON {
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
	o := orderedJSON{{"type", "script_exception"}, {"reason", kind}}
	o = append(o, e.extra...)
	stack := make([]any, len(e.stack))
	for i, s := range e.stack {
		stack[i] = s
	}
	o = append(o, orderedKV{"script_stack", stack}, orderedKV{"script", e.script}, orderedKV{"lang", lang})
	if e.pos != nil {
		o = append(o, orderedKV{"position", orderedJSON{{"offset", e.pos[0]}, {"start", e.pos[1]}, {"end", e.pos[2]}}})
	}
	if withCause {
		o = append(o, orderedKV{"caused_by", e.causeObject()})
	}
	return o
}

func (e *plScriptErr) causeObject() orderedJSON {
	var reason any = e.reason
	if e.nilReason {
		reason = nil
	}
	c := orderedJSON{{"type", e.causeType}, {"reason", reason}}
	if e.inner != nil {
		var ir any = e.inner.reason
		if e.inner.nilReason {
			ir = nil
		}
		c = append(c, orderedKV{"caused_by", orderedJSON{{"type", e.inner.typ}, {"reason", ir}}})
	}
	return c
}

// errorBody renders the full {"error": ..., "status": 400} response.
func (e *plScriptErr) errorBody() orderedJSON {
	errObj := orderedJSON{{"root_cause", []any{e.object(false)}}}
	errObj = append(errObj, e.object(true)...)
	return orderedJSON{{"error", errObj}, {"status", 400}}
}

// asAPIError wraps the script error for callers that report apiErrors.
func (e *plScriptErr) asAPIError() *apiError {
	return &apiError{Type: "script_exception", Status: 400, Reason: map[bool]string{true: "compile error", false: "runtime error"}[e.compile],
		Extra: e, CausedBy: &apiError{Type: e.causeType, Reason: e.reason}}
}

func plCompileFailure(src, name string, ce *plCompileErr) *plScriptErr {
	e := &plScriptErr{compile: true, script: name, causeType: ce.typ, reason: ce.reason, nilReason: ce.nilReason, inner: ce.cause, stack: []string{}}
	if ce.noPos {
		return e
	}
	off := ce.offset
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

func plRuntimeFailure(s *plScript, name string, re *plRtErr) *plScriptErr {
	e := &plScriptErr{script: name, causeType: re.typ, reason: re.reason, nilReason: re.nilReason, extra: re.extra}
	src := s.src
	off := re.offset
	if off > len(src) {
		off = len(src)
	}
	start, end := 0, len(src)
	for _, st := range s.prog.stmts {
		if st <= off && st >= start {
			start = st
		}
	}
	for _, st := range s.prog.stmts {
		if st > off && st < end {
			end = st
		}
	}
	e.stack = append(append([]string{}, re.frames...), src[start:end], strings.Repeat(" ", off-start)+"^---- HERE")
	e.pos = []int{off, start, end}
	return e
}

var plCache sync.Map // context + "\x00" + source -> *plScript

func plCompile(src, ctxName string) (*plScript, *plCompileErr) {
	key := ctxName + "\x00" + src
	if v, ok := plCache.Load(key); ok {
		return v.(*plScript), nil
	}
	ctx := plContexts[ctxName]
	if ctx == nil {
		ctx = plContexts["painless_test"]
	}
	prog, err := plParse(src)
	if err != nil {
		return nil, err
	}
	info, err := plCheck(prog, ctx)
	if err != nil {
		return nil, err
	}
	s := &plScript{src: src, prog: prog, info: info, ctx: ctx}
	plCache.Store(key, s)
	return s, nil
}

// plCompileScript compiles and converts errors to script exceptions.
func plCompileScript(src, ctxName, name string) (*plScript, *plScriptErr) {
	s, err := plCompile(src, ctxName)
	if err != nil {
		if name == "" {
			name = src
		}
		return nil, plCompileFailure(src, name, err)
	}
	return s, nil
}

type plBindings struct {
	vars  map[string]any
	funcs map[string]func(args []any, off int) (any, *plRtErr)
}

// exec runs a compiled script and applies the context's return conversion.
func (s *plScript) exec(b plBindings, name string) (any, *plScriptErr) {
	in := &plInterp{prog: s.prog, info: s.info, funcs: b.funcs}
	env := &plEnv{vars: map[string]*plVar{}}
	for k, t := range s.ctx.vars {
		env.vars[k] = &plVar{v: b.vars[k], typ: t}
	}
	v, rerr := in.run(env)
	if rerr == nil {
		v, rerr = s.convertReturn(v)
	}
	if rerr != nil {
		if name == "" {
			name = s.src
		}
		return nil, plRuntimeFailure(s, name, rerr)
	}
	return v, nil
}

// retNode finds the expression whose value the script returns, for error
// offsets of failed return conversions.
func (s *plScript) retNode() plNode {
	var found plNode
	var walk func(stmts []plNode)
	walk = func(stmts []plNode) {
		for _, st := range stmts {
			switch n := st.(type) {
			case *plSReturn:
				if n.x != nil {
					found = n.x
				}
			case *plSIf:
				if b, ok := n.then.(*plSBlock); ok {
					walk(b.stmts)
				} else {
					walk([]plNode{n.then})
				}
			case *plSBlock:
				walk(n.stmts)
			}
		}
		if len(stmts) > 0 {
			if es, ok := stmts[len(stmts)-1].(*plSExpr); ok {
				found = es.x
			}
		}
	}
	walk(s.prog.body.stmts)
	return found
}

func (s *plScript) convertReturn(v any) (any, *plRtErr) {
	off := 0
	if n := s.retNode(); n != nil {
		off = n.nodePos()
	}
	switch s.ctx.ret {
	case "boolean":
		if b, ok := v.(bool); ok {
			return b, nil
		}
		if v == nil {
			return nil, plNPE(off)
		}
		return nil, &plRtErr{typ: "class_cast_exception", offset: off,
			reason: "cannot cast def [" + plDefTypeName(v) + "] to boolean"}
	case "double":
		if plIsNumber(v) {
			return plF64(v), nil
		}
		if v == nil {
			return nil, plNPE(off)
		}
		return nil, &plRtErr{typ: "class_cast_exception", offset: off,
			reason: "cannot implicitly cast def [" + plDefTypeName(v) + "] to double"}
	case "String":
		if v == nil {
			return nil, nil
		}
		if str, ok := v.(string); ok {
			return str, nil
		}
		return nil, &plRtErr{typ: "class_cast_exception", offset: off,
			reason: "cannot implicitly cast def [" + plDefTypeName(v) + "] to java.lang.String"}
	}
	return v, nil
}

// ---- doc lookup ----

var plTextFielddataReason = "Text fields are not optimised for operations that require per-document field data like aggregations and sorting, so these operations are disabled by default. Please use a keyword field instead. Alternatively, set fielddata=true on [%s] in order to load field data by uninverting the inverted index. Note that this can use significant memory."

// plDocLookup builds the doc variable for one document.
func plDocLookup(qc *queryCtx) *plDocMap {
	cache := map[string]*plDocValues{}
	has := func(field string) bool {
		if field == "_id" || field == "_index" || field == "_seq_no" || field == "_version" {
			return true
		}
		_, ok := mappingFieldDef(qc.idx, field)
		return ok
	}
	get := func(field string, off int) (any, *plRtErr) {
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
	return &plDocMap{get: get, has: has}
}

func plDocValuesFor(qc *queryCtx, field string, off int) (*plDocValues, *plRtErr) {
	switch field {
	case "_id":
		return &plDocValues{kind: "Strings", field: field, vals: []any{qc.id}}, nil
	case "_index":
		return &plDocValues{kind: "Strings", field: field, vals: []any{qc.idx.Name}}, nil
	case "_seq_no":
		return &plDocValues{kind: "Longs", field: field, vals: []any{qc.idx.SeqNos[qc.id]}}, nil
	}
	def, ok := mappingFieldDef(qc.idx, field)
	if !ok {
		return nil, &plRtErr{typ: "illegal_argument_exception", offset: off,
			reason: "No field found for [" + field + "] in mapping",
			frames: []string{"org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:89)",
				"org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:55)"}}
	}
	ft, _ := def["type"].(string)
	if ft == "alias" {
		return plDocValuesFor(qc, resolveAliasField(qc.idx, field), off)
	}
	vals, _ := qc.values(field)
	if ft == "knn_vector" {
		return plKnnDocValues(field, vals), nil
	}
	dv := &plDocValues{field: field}
	switch ft {
	case "text", "match_only_text", "search_as_you_type":
		if fd, _ := def["fielddata"].(bool); !fd {
			return nil, &plRtErr{typ: "illegal_argument_exception", offset: off,
				reason: strings.Replace(plTextFielddataReason, "%s", field, 1),
				frames: []string{
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
			s, _ := valueScalarString(v)
			for _, t := range analyzeTokensWithIndex(qc.idx, analyzer, s) {
				if !seen[t.term] {
					seen[t.term] = true
					terms = append(terms, t.term)
				}
			}
		}
		sort.Strings(terms)
		dv.kind = "Strings"
		for _, t := range terms {
			dv.vals = append(dv.vals, t)
		}
		return dv, nil
	case "integer", "long", "short", "byte", "unsigned_long", "token_count":
		dv.kind = "Longs"
		for _, v := range vals {
			if f, ok := toFloat(v); ok {
				dv.vals = append(dv.vals, int64(f))
			}
		}
	case "float", "double", "half_float", "scaled_float":
		dv.kind = "Doubles"
		for _, v := range vals {
			if f, ok := toFloat(v); ok {
				dv.vals = append(dv.vals, f)
			}
		}
	case "boolean":
		dv.kind = "Booleans"
		for _, v := range vals {
			if b, ok := v.(bool); ok {
				dv.vals = append(dv.vals, b)
			}
		}
		sort.SliceStable(dv.vals, func(i, j int) bool { return !dv.vals[i].(bool) && dv.vals[j].(bool) })
		return dv, nil
	case "date", "date_nanos":
		dv.kind = "Dates"
		ms := []int64{}
		for _, v := range vals {
			if f, ok := toFloat(v); ok {
				ms = append(ms, int64(f))
			}
		}
		sort.Slice(ms, func(i, j int) bool { return ms[i] < ms[j] })
		for _, m := range ms {
			dv.vals = append(dv.vals, &plZDT{t: plTimeFromMillis(m)})
		}
		return dv, nil
	case "geo_point":
		dv.kind = "GeoPoints"
		for _, v := range vals {
			if m, ok := v.(map[string]any); ok {
				lat, _ := toFloat(m["lat"])
				lon, _ := toFloat(m["lon"])
				dv.vals = append(dv.vals, &plGeoPoint{lat: lat, lon: lon})
			}
		}
		return dv, nil
	default:
		dv.kind = "Strings"
		for _, v := range vals {
			s, ok := valueScalarString(v)
			if !ok {
				s = plToString(v)
			}
			dv.vals = append(dv.vals, s)
		}
		sort.SliceStable(dv.vals, func(i, j int) bool { return dv.vals[i].(string) < dv.vals[j].(string) })
		return dv, nil
	}
	sort.SliceStable(dv.vals, func(i, j int) bool { c, _ := plCompare(dv.vals[i], dv.vals[j]); return c < 0 })
	return dv, nil
}

// ---- score functions ----

func plScoreBindings(qc *queryCtx) map[string]func(args []any, off int) (any, *plRtErr) {
	f := func(a []any, i int) float64 { return plF64(a[i]) }
	dist := func(a []any) float64 {
		d := math.Abs(f(a, 4)-f(a, 0)) - f(a, 2)
		if d < 0 {
			d = 0
		}
		return d
	}
	m := plKnnBindings()
	for k, v := range map[string]func(args []any, off int) (any, *plRtErr){
		"saturation": func(a []any, off int) (any, *plRtErr) { return f(a, 0) / (f(a, 1) + f(a, 0)), nil },
		"sigmoid": func(a []any, off int) (any, *plRtErr) {
			v, k, e := f(a, 0), f(a, 1), f(a, 2)
			return math.Pow(v, e) / (math.Pow(k, e) + math.Pow(v, e)), nil
		},
		"decayNumericLinear": func(a []any, off int) (any, *plRtErr) {
			scaling := f(a, 1) / (1.0 - f(a, 3))
			return math.Max(0.0, (scaling-dist(a))/scaling), nil
		},
		"decayNumericExp": func(a []any, off int) (any, *plRtErr) {
			scaling := math.Log(f(a, 3)) / f(a, 1)
			return math.Exp(scaling * dist(a)), nil
		},
		"decayNumericGauss": func(a []any, off int) (any, *plRtErr) {
			scaling := 0.5 * math.Pow(f(a, 1), 2.0) / math.Log(f(a, 3))
			return math.Exp(0.5 * math.Pow(dist(a), 2.0) / scaling), nil
		},
		"randomScore": func(a []any, off int) (any, *plRtErr) {
			h := fnv.New32a()
			key := qc.id
			if len(a) == 2 {
				if vals, ok := qc.values(plToString(a[1])); ok && len(vals) > 0 {
					key = plToString(plFromJSON(vals[0], plHash))
				}
			}
			h.Write([]byte(itoa(int(plI64(a[0]))) + "\x00" + key))
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
		writeError(w, &apiError{Type: "parse_exception", Status: 400, Reason: "request body or source parameter is required"})
		return
	}
	scriptRaw, ok := body["script"]
	if !ok {
		writeError(w, errBad("Required [script]"))
		return
	}
	sm, ok := scriptRaw.(map[string]any)
	if !ok {
		col := strings.Index(string(raw), `"script"`) + len(`"script"`) + 2
		writeError(w, &apiError{Type: "x_content_parse_exception", Status: 400,
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
		writeError(w, &apiError{Type: "x_content_parse_exception", Status: 400,
			Reason:   "Failed to build [painless_execute_request] after last required field arrived",
			CausedBy: &apiError{Type: "unsupported_operation_exception", Reason: "unsupported script context name [" + ctxName + "]"}})
		return
	}
	if _, hasID := sm["id"]; hasID {
		writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: only inline scripts are supported;"})
		return
	}
	src, _ := sm["source"].(string)
	if len(src) > 65535 {
		writeError(w, errBad("exceeded max allowed inline script size in bytes [65535] with size ["+itoa(len(src))+"] for script ["+src+"]"))
		return
	}
	lang, _ := sm["lang"].(string)
	switch lang {
	case "", "painless":
	case "expression":
		writeError(w, errBad("expression engine does not know how to handle script context ["+ctxName+"]"))
		return
	case "mustache":
		writeError(w, errBad("mustache engine does not know how to handle context ["+ctxName+"]"))
		return
	default:
		writeError(w, errBad("script_lang not supported ["+lang+"]"))
		return
	}
	params := plFromJSON(map[string]any{}, plHash).(*plMap)
	if p, ok := sm["params"].(map[string]any); ok {
		params = plFromJSON(p, plHash).(*plMap)
	}
	b := plBindings{vars: map[string]any{"params": params}}
	if ctxName != "painless_test" {
		setup, _ := body["context_setup"].(map[string]any)
		idxName, _ := setup["index"].(string)
		s.mu.RLock()
		idx := s.indices[idxName]
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
		b.vars["doc"] = plDocLookup(qc)
		if ctxName == "score" {
			score := 0.0
			if q, ok := setup["query"].(map[string]any); ok {
				if matched, sc, qerr := qc.eval(q); qerr == nil && matched {
					score = float64(float32(sc))
				}
			}
			b.vars["_score"] = score
			b.funcs = plScoreBindings(qc)
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
		result = plToString(v)
	case "score":
		result = float64(float32(plF64(v)))
	default:
		result = plToJSON(v)
	}
	writeJSON(w, 200, orderedJSON{{"result", result}})
}

// plTempIndex is a single-document copy of an index, like the in-memory
// index the execute API builds for context_setup.
func plTempIndex(idx *Index, doc map[string]any) *Index {
	tmp := &Index{Name: idx.Name, UUID: idx.UUID, Settings: idx.Settings,
		Mappings:   deepCopySource(idx.Mappings).(map[string]any),
		Docs:       map[string]map[string]any{"_execute": doc},
		Order:      []string{"_execute"},
		DocValues:  map[string]map[string][]any{},
		Versions:   map[string]int64{"_execute": 1},
		SeqNos:     map[string]int64{"_execute": 0},
		Routing:    map[string]string{},
		FieldTypes: map[string]string{},
	}
	ensureFieldTypes(tmp)
	_ = tmp.computeDocValues("_execute", doc)
	return tmp
}

var _ = json.Marshal
