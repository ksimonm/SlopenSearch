package main

// Glue between the scripting engines and the APIs that run scripts
// (search, aggregations, update, ingest): script specs, stored-script
// resolution, language checks and the shard-failure envelopes.

// scriptSpec is a parsed "script" object.
type scriptSpec struct {
	src    string
	lang   string
	id     string // stored script id, if referenced by id
	params map[string]any
}

// errName is the "script" value reported in script exceptions.
func (s *scriptSpec) errName() string {
	if s.id != "" {
		return s.id
	}
	return s.src
}

// parseScriptSpec reads a script given as a string or an object and
// resolves stored scripts.
func parseScriptSpec(raw any) (*scriptSpec, *apiError) {
	switch t := raw.(type) {
	case string:
		return &scriptSpec{src: t, lang: "painless", params: map[string]any{}}, nil
	case map[string]any:
		spec := &scriptSpec{lang: "painless", params: map[string]any{}}
		if p, ok := t["params"].(map[string]any); ok {
			spec.params = p
		}
		src, hasSrc := t["source"]
		if !hasSrc {
			src, hasSrc = t["inline"]
		}
		id, hasID := t["id"].(string)
		if hasSrc && hasID {
			return nil, &apiError{Type: "x_content_parse_exception", Status: 400,
				Reason:   "[script] failed to parse field [source]",
				CausedBy: &apiError{Type: "illegal_argument_exception", Reason: "must only use one of [source, id] when specifying a script"}}
		}
		lang, hasLang := t["lang"].(string)
		if hasID {
			if hasLang {
				return nil, errBad("illegally specified <lang> for a stored script")
			}
			spec.id = id
			sc, err := storedScript(id)
			if err != nil {
				return nil, err
			}
			spec.src, _ = sc["source"].(string)
			if l, ok := sc["lang"].(string); ok {
				spec.lang = l
			}
			return spec, nil
		}
		if s, ok := src.(string); ok {
			spec.src = s
		}
		if hasLang {
			spec.lang = lang
		}
		return spec, nil
	}
	return nil, errParsing("unexpected token, expected a script")
}

func storedScript(id string) (map[string]any, *apiError) {
	if globalServer == nil {
		return nil, &apiError{Type: "resource_not_found_exception", Status: 404, Reason: "unable to find script [" + id + "] in cluster state"}
	}
	globalServer.scriptsMu.RLock()
	sc, ok := globalServer.scripts[id]
	globalServer.scriptsMu.RUnlock()
	if !ok {
		return nil, &apiError{Type: "resource_not_found_exception", Status: 404,
			Reason: "unable to find script [" + id + "] in cluster state"}
	}
	return sc, nil
}

var expressionContexts = map[string]bool{
	"aggregation_selector": true, "aggs": true, "bucket_aggregation": true, "field": true,
	"filter": true, "number_sort": true, "score": true, "terms_set": true,
}

// checkScriptLang rejects languages that cannot run in a context.
func checkScriptLang(spec *scriptSpec, ctxName string) *apiError {
	switch spec.lang {
	case "painless":
		return nil
	case "expression":
		if !expressionContexts[ctxName] {
			return errBad("expression engine does not know how to handle script context [" + ctxName + "]")
		}
		return nil
	case "mustache":
		return errBad("mustache engine does not know how to handle context [" + ctxName + "]")
	}
	return errBad("script_lang not supported [" + spec.lang + "]")
}

// compilePainless compiles a painless spec for a context.
func compilePainless(spec *scriptSpec, ctxName string) (*plScript, *plScriptErr) {
	return plCompileScript(spec.src, ctxName, spec.errName())
}

// scriptParams converts request params into a painless params map.
func scriptParams(p map[string]any) *plMap {
	if p == nil {
		return newPlMap(plHash)
	}
	return plFromJSON(p, plHash).(*plMap)
}

// ---- shard failure envelopes ----

// shardFailure is a pre-rendered search_phase_execution_exception.
type shardFailure struct {
	raw    orderedJSON // complete response body, when set
	status int
	index  string
	root   orderedJSON
	shard  orderedJSON
	top    orderedJSON
	phase  string
}

func (f *shardFailure) body() orderedJSON {
	if f.raw != nil {
		return f.raw
	}
	phase := f.phase
	if phase == "" {
		phase = "query"
	}
	errObj := orderedJSON{
		{"root_cause", []any{f.root}},
		{"type", "search_phase_execution_exception"},
		{"reason", "all shards failed"},
		{"phase", phase},
		{"grouped", true},
		{"failed_shards", []any{orderedJSON{
			{"shard", 0},
			{"index", f.index},
			{"node", nodeID},
			{"reason", f.shard},
		}}},
		{"caused_by", f.top},
	}
	return orderedJSON{{"error", errObj}, {"status", f.status}}
}

// scriptShardError wraps a script exception raised while executing a query
// phase script.
func scriptShardError(idx *Index, se *plScriptErr) *apiError {
	f := &shardFailure{status: 400, index: idx.Name, root: se.object(false), shard: se.object(true), top: se.object(true)}
	return &apiError{Type: "script_exception", Status: 400, Reason: "script error", Shard: f}
}

// scriptQueryCompileError wraps a compile error of a query script, which
// fails while the shard creates the query.
func scriptQueryCompileError(idx *Index, se *plScriptErr) *apiError {
	if !se.compile {
		return scriptShardError(idx, se)
	}
	shard := orderedJSON{{"type", "query_shard_exception"}, {"reason", "failed to create query: compile error"},
		{"index", idx.Name}, {"index_uuid", idx.UUID}, {"caused_by", se.object(true)}}
	f := &shardFailure{status: 400, index: idx.Name, root: se.object(false), shard: shard, top: se.object(true)}
	return &apiError{Type: "script_exception", Status: 400, Reason: "script error", Shard: f}
}

// plainShardError reports a non-script exception raised on the shard.
func plainShardError(idx *Index, e *apiError) *apiError {
	if e.Shard != nil {
		return e
	}
	status := e.Status
	if status == 0 {
		status = 400
	}
	obj := orderedJSON{{"type", e.Type}, {"reason", e.Reason}}
	top := orderedJSON{{"type", e.Type}, {"reason", e.Reason}, {"caused_by", orderedJSON{{"type", e.Type}, {"reason", e.Reason}}}}
	if status == 404 {
		top = obj
	}
	f := &shardFailure{status: status, index: idx.Name, root: obj, shard: obj, top: top}
	return &apiError{Type: e.Type, Status: status, Reason: e.Reason, Shard: f}
}

// scriptErrForShard turns any script setup or execution error into a shard
// failure.
func scriptErrForShard(idx *Index, se *plScriptErr, ae *apiError, query bool) *apiError {
	if se != nil {
		if query {
			return scriptQueryCompileError(idx, se)
		}
		return scriptShardError(idx, se)
	}
	if ae == nil {
		return nil
	}
	if ae.Type == "x_content_parse_exception" || (ae.Type == "illegal_argument_exception" && ae.Reason == "illegally specified <lang> for a stored script") {
		return ae
	}
	return plainShardError(idx, ae)
}

// ---- bindings ----

// fieldScriptValues converts a script result into script_fields values.
func fieldScriptValues(v any) ([]any, bool) {
	switch t := v.(type) {
	case nil:
		return []any{nil}, true
	case *plList, *plDocValues, *plArray, *plSet:
		vals := plToJSON(t).([]any)
		if len(vals) == 0 {
			return nil, false
		}
		return vals, true
	}
	return []any{plToJSON(v)}, true
}

// ---- compiled scripts ----

// compiledScript is a script prepared for one context, in either language.
type compiledScript struct {
	spec   *scriptSpec
	ctx    string
	pl     *plScript
	ex     *exScript
	linked map[*Index]*exLinked
}

// prepareScript parses, resolves and compiles a script for a context.
func prepareScript(raw any, ctxName string) (*compiledScript, *plScriptErr, *apiError) {
	spec, ae := parseScriptSpec(raw)
	if ae != nil {
		return nil, nil, ae
	}
	return prepareSpec(spec, ctxName)
}

func prepareSpec(spec *scriptSpec, ctxName string) (*compiledScript, *plScriptErr, *apiError) {
	cs := &compiledScript{spec: spec, ctx: ctxName}
	switch spec.lang {
	case "expression":
		ex, se, ae := exPrepare(spec.src, ctxName)
		if se != nil || ae != nil {
			if se != nil {
				se.script = spec.errName()
			}
			return nil, se, ae
		}
		cs.ex = ex
		cs.linked = map[*Index]*exLinked{}
		return cs, nil, nil
	}
	if ae := checkScriptLang(spec, ctxName); ae != nil {
		return nil, nil, ae
	}
	pl, se := compilePainless(spec, ctxName)
	if se != nil {
		return nil, se, nil
	}
	cs.pl = pl
	return cs, nil, nil
}

// runOpts carries the per-execution bindings of a script.
type runOpts struct {
	score    float64
	value    any
	source   *plMap         // params._source
	vars     map[string]any // extra context variables (ctx, state, states, ...)
	params   map[string]any // extra params (bucket paths, num_terms, ...)
	plParams *plMap         // reuse this params map (shared state)
}

// link binds an expression to an index (cached).
func (cs *compiledScript) link(idx *Index, extra map[string]any) (*exLinked, *plScriptErr) {
	if l, ok := cs.linked[idx]; ok && extra == nil {
		return l, nil
	}
	params := map[string]any{}
	for k, v := range cs.spec.params {
		params[k] = v
	}
	for k, v := range extra {
		params[k] = v
	}
	needsScore := cs.ctx == "score" || cs.ctx == "number_sort" || cs.ctx == "aggs"
	l, err := exLink(cs.ex, idx, params, cs.ctx == "aggs", needsScore)
	if err != nil {
		se := err.scriptErr(cs.spec.src)
		se.script = cs.spec.errName()
		return nil, se
	}
	if extra == nil {
		cs.linked[idx] = l
	}
	return l, nil
}

// run executes the script for one document (qc may be nil for scripts
// without document access).
func (cs *compiledScript) run(qc *queryCtx, o runOpts) (any, *plScriptErr) {
	if cs.ex != nil {
		var idx *Index
		if qc != nil {
			idx = qc.idx
		}
		var extra map[string]any
		if len(o.params) > 0 {
			extra = o.params
		}
		l, se := cs.link(idx, extra)
		if se != nil {
			return nil, se
		}
		v := l.eval(qc, o.score, o.value)
		switch cs.ctx {
		case "filter", "aggregation_selector":
			return v != 0, nil
		}
		return v, nil
	}
	params := o.plParams
	if params == nil {
		params = scriptParams(cs.spec.params)
	}
	for k, v := range o.params {
		params.put(k, plFromJSON(v, plHash))
	}
	if o.source != nil {
		params.put("_source", o.source)
	}
	b := plBindings{vars: map[string]any{"params": params, "_score": o.score, "_value": plFromJSON(o.value, plHash)}}
	for k, v := range o.vars {
		b.vars[k] = v
	}
	if qc != nil {
		b.vars["doc"] = plDocLookup(qc)
		if cs.ctx == "score" {
			b.funcs = plScoreBindings(qc)
		}
	}
	return cs.pl.exec(b, cs.spec.errName())
}

// scriptToString renders org.opensearch.script.Script#toString.
func scriptToString(raw any) string {
	spec, err := parseScriptSpec(raw)
	if err != nil {
		return ""
	}
	params := plToString(scriptParams(spec.params))
	if spec.id != "" {
		return "Script{type=stored, lang='null', idOrCode='" + spec.id + "', options=null, params=" + params + "}"
	}
	return "Script{type=inline, lang='" + spec.lang + "', idOrCode='" + spec.src + "', options={}, params=" + params + "}"
}

// ---- aggregation scripts ----

// aggScriptRun runs an aggregation script for one document; with a field
// it is a value script evaluated once per field value (_value).
func aggScriptRun(cs *compiledScript, idx *Index, doc map[string]any, fieldVals []any, useField bool) ([]any, *apiError) {
	id, _ := doc["__slop_id"].(string)
	qc := newQueryCtxForDoc(idx, id, doc)
	score, _ := toFloat(doc["__slop_score"])
	out := []any{}
	if useField {
		for _, v := range fieldVals {
			r, se := cs.run(qc, runOpts{value: v, score: score})
			if se != nil {
				return nil, scriptShardError(idx, se)
			}
			out = append(out, r)
		}
		return out, nil
	}
	r, se := cs.run(qc, runOpts{score: score})
	if se != nil {
		return nil, scriptShardError(idx, se)
	}
	if items, ok := plToItems(r); ok {
		return append(out, items...), nil
	}
	if r == nil {
		return out, nil
	}
	return append(out, r), nil
}

// prepareAggScript compiles the script of an aggregation for the aggs context.
func prepareAggScript(idx *Index, raw any) (*compiledScript, *apiError) {
	cs, se, ae := prepareScript(raw, "aggs")
	if se != nil || ae != nil {
		return nil, scriptErrForShard(idx, se, ae, false)
	}
	return cs, nil
}

// evalTermScript evaluates a terms-like script (composite sources) for one
// document and returns its first value as a term.
func evalTermScript(doc map[string]any, idx *Index, raw any) (any, bool) {
	cs, err := prepareAggScript(idx, raw)
	if err != nil {
		return nil, false
	}
	res, err := aggScriptRun(cs, idx, doc, nil, false)
	if err != nil || len(res) == 0 || res[0] == nil {
		return nil, false
	}
	return termsScriptKey(res[0], false, "", cs.ex != nil), true
}

// pipelineScriptError reports a failing pipeline aggregation script, which
// runs on the coordinating node during the reduce (fetch) phase.
func pipelineScriptError(se *plScriptErr) *apiError {
	body := orderedJSON{{"error", orderedJSON{
		{"root_cause", []any{}}, {"type", "search_phase_execution_exception"}, {"reason", ""},
		{"phase", "fetch"}, {"grouped", true}, {"failed_shards", []any{}}, {"caused_by", se.object(true)},
	}}, {"status", 400}}
	return &apiError{Type: "search_phase_execution_exception", Status: 400, Shard: &shardFailure{raw: body, status: 400}}
}

// runPipelineScript evaluates a bucket_script / bucket_selector script with
// the bucket path values as params.
func runPipelineScript(raw any, ctxName string, vals map[string]any) (any, *apiError) {
	cs, se, ae := prepareScript(raw, ctxName)
	if ae != nil {
		return nil, ae
	}
	if se != nil {
		return nil, pipelineScriptError(se)
	}
	v, rerr := cs.run(nil, runOpts{params: vals})
	if rerr != nil {
		return nil, pipelineScriptError(rerr)
	}
	return v, nil
}

// scriptedMetric runs the init/map/combine/reduce scripts of a
// scripted_metric aggregation over the given documents (one shard).
func scriptedMetric(name string, spec map[string]any, docs []map[string]any, idx *Index) (any, *apiError) {
	if _, ok := spec["map_script"]; !ok {
		return nil, errBad("Required [map_script]")
	}
	for _, req := range []struct{ key, label string }{{"combine_script", "combineScript"}, {"reduce_script", "reduceScript"}} {
		if _, ok := spec[req.key]; !ok {
			return nil, plainShardError(idx, errBad("["+req.label+"] must not be null: ["+name+"]"))
		}
	}
	aggParams, _ := spec["params"].(map[string]any)
	params := scriptParams(aggParams)
	state := newPlMap(plHash)
	prep := func(key, ctxName string) (*compiledScript, *apiError) {
		cs, se, ae := prepareScript(spec[key], ctxName)
		if se != nil || ae != nil {
			return nil, scriptErrForShard(idx, se, ae, false)
		}
		return cs, nil
	}
	vars := map[string]any{"state": state}
	if _, ok := spec["init_script"]; ok {
		cs, err := prep("init_script", "aggs_init")
		if err != nil {
			return nil, err
		}
		if _, se := cs.run(nil, runOpts{plParams: params, vars: vars}); se != nil {
			return nil, scriptShardError(idx, se)
		}
	}
	mapCS, err := prep("map_script", "aggs_map")
	if err != nil {
		return nil, err
	}
	for _, d := range docs {
		id, _ := d["__slop_id"].(string)
		score, _ := toFloat(d["__slop_score"])
		if _, se := mapCS.run(newQueryCtxForDoc(idx, id, d), runOpts{plParams: params, vars: vars, score: score}); se != nil {
			return nil, scriptShardError(idx, se)
		}
	}
	combCS, err := prep("combine_script", "aggs_combine")
	if err != nil {
		return nil, err
	}
	combined, se := combCS.run(nil, runOpts{plParams: params, vars: vars})
	if se != nil {
		return nil, scriptShardError(idx, se)
	}
	redCS, err := prep("reduce_script", "aggs_reduce")
	if err != nil {
		return nil, err
	}
	res, se := redCS.run(nil, runOpts{plParams: params, vars: map[string]any{"states": newPlList(combined)}})
	if se != nil {
		return nil, pipelineScriptError(se)
	}
	return orderedJSON{{"value", plToJSON(res)}}, nil
}

// scriptValueDocs evaluates the script of a numeric bucket aggregation
// without a field and exposes its values as a synthetic field of copied
// documents.
func scriptValueDocs(spec map[string]any, docs []map[string]any, idx *Index, field string) ([]map[string]any, string, *apiError) {
	raw, ok := spec["script"]
	if !ok || field != "" {
		return docs, field, nil
	}
	cs, err := prepareAggScript(idx, raw)
	if err != nil {
		return nil, field, err
	}
	const synth = "__slop_script"
	out := make([]map[string]any, 0, len(docs))
	for _, d := range docs {
		res, err := aggScriptRun(cs, idx, d, nil, false)
		if err != nil {
			return nil, field, err
		}
		vals := []any{}
		for _, r := range res {
			if plIsNumber(r) {
				vals = append(vals, plF64(r))
			}
		}
		cp := make(map[string]any, len(d)+1)
		for k, v := range d {
			cp[k] = v
		}
		if len(vals) == 1 {
			cp[synth] = vals[0]
		} else if len(vals) > 1 {
			cp[synth] = vals
		}
		out = append(out, cp)
	}
	return out, synth, nil
}

// scriptSortValue evaluates a _script sort for one hit outside the main sort
// pass (e.g. for search_after); errors yield no value.
func scriptSortValue(sp searchSortSpec, h multiHit) any {
	ctxName := "number_sort"
	if sp.scriptType == "string" {
		ctxName = "string_sort"
	}
	cs, se, ae := prepareScript(sp.scriptRaw, ctxName)
	if se != nil || ae != nil {
		return nil
	}
	v, rerr := cs.run(newQueryCtxForDoc(h.idx, h.id, h.doc), runOpts{score: h.score})
	if rerr != nil {
		return nil
	}
	if ctxName == "number_sort" {
		return plF64(v)
	}
	return v
}
