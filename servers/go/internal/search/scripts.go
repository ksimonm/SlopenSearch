package search

import (
	"slopensearch/internal/apierr"
	"slopensearch/internal/expressions"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/model"
	"slopensearch/internal/ojson"
	"slopensearch/internal/painless"
)

// scriptSpec is a parsed "script" object.
type ScriptSpec struct {
	Src    string
	Lang   string
	Id     string // stored script id, if referenced by id
	Params map[string]any
}

// errName is the "script" value reported in script exceptions.
func (s *ScriptSpec) ErrName() string {
	if s.Id != "" {
		return s.Id
	}
	return s.Src
}

// parseScriptSpec reads a script given as a string or an object and
// resolves stored scripts.
func ParseScriptSpec(raw any) (*ScriptSpec, *apierr.Error) {
	switch t := raw.(type) {
	case string:
		return &ScriptSpec{Src: t, Lang: "painless", Params: map[string]any{}}, nil
	case map[string]any:
		spec := &ScriptSpec{Lang: "painless", Params: map[string]any{}}
		if p, ok := t["params"].(map[string]any); ok {
			spec.Params = p
		}
		src, hasSrc := t["source"]
		if !hasSrc {
			src, hasSrc = t["inline"]
		}
		id, hasID := t["id"].(string)
		if hasSrc && hasID {
			return nil, &apierr.Error{Type: "x_content_parse_exception", Status: 400,
				Reason:   "[script] failed to parse field [source]",
				CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: "must only use one of [source, id] when specifying a script"}}
		}
		lang, hasLang := t["lang"].(string)
		if hasID {
			if hasLang {
				return nil, apierr.BadRequest("illegally specified <lang> for a stored script")
			}
			spec.Id = id
			sc, err := StoredScript(id)
			if err != nil {
				return nil, err
			}
			spec.Src, _ = sc["source"].(string)
			if l, ok := sc["lang"].(string); ok {
				spec.Lang = l
			}
			return spec, nil
		}
		if s, ok := src.(string); ok {
			spec.Src = s
		}
		if hasLang {
			spec.Lang = lang
		}
		return spec, nil
	}
	return nil, apierr.Parsing("unexpected token, expected a script")
}

func StoredScript(id string) (map[string]any, *apierr.Error) {
	if Cluster == nil {
		return nil, &apierr.Error{Type: "resource_not_found_exception", Status: 404, Reason: "unable to find script [" + id + "] in cluster state"}
	}
	sc, ok := Cluster.StoredScript(id)
	if !ok {
		return nil, &apierr.Error{Type: "resource_not_found_exception", Status: 404,
			Reason: "unable to find script [" + id + "] in cluster state"}
	}
	return sc, nil
}

var expressionContexts = map[string]bool{
	"aggregation_selector": true, "aggs": true, "bucket_aggregation": true, "field": true,
	"filter": true, "number_sort": true, "score": true, "terms_set": true,
}

// checkScriptLang rejects languages that cannot run in a context.
func CheckScriptLang(spec *ScriptSpec, ctxName string) *apierr.Error {
	switch spec.Lang {
	case "painless":
		return nil
	case "expression":
		if !expressionContexts[ctxName] {
			return apierr.BadRequest("expression engine does not know how to handle script context [" + ctxName + "]")
		}
		return nil
	case "mustache":
		return apierr.BadRequest("mustache engine does not know how to handle context [" + ctxName + "]")
	}
	return apierr.BadRequest("script_lang not supported [" + spec.Lang + "]")
}

// compilePainless compiles a painless spec for a context.
func CompilePainless(spec *ScriptSpec, ctxName string) (*Script, *ScriptError) {
	return CompileScript(spec.Src, ctxName, spec.ErrName())
}

// scriptParams converts request params into a painless params map.
func ScriptParams(p map[string]any) *painless.Map {
	if p == nil {
		return painless.NewMap(painless.Hash)
	}
	return painless.FromJSON(p, painless.Hash).(*painless.Map)
}

// shardFailure is a pre-rendered search_phase_execution_exception.
type ShardFailure struct {
	Raw    ojson.Object // complete response body, when set
	Status int
	Index  string
	Root   ojson.Object
	Shard  ojson.Object
	Top    ojson.Object
	phase  string
}

// StatusCode and ResponseBody implement apierr.Response.
func (f *ShardFailure) StatusCode() int { return f.Status }

func (f *ShardFailure) ResponseBody() any { return f.body() }

func (f *ShardFailure) body() ojson.Object {
	if f.Raw != nil {
		return f.Raw
	}
	phase := f.phase
	if phase == "" {
		phase = "query"
	}
	errObj := ojson.Object{
		{"root_cause", []any{f.Root}},
		{"type", "search_phase_execution_exception"},
		{"reason", "all shards failed"},
		{"phase", phase},
		{"grouped", true},
		{"failed_shards", []any{ojson.Object{
			{"shard", 0},
			{"index", f.Index},
			{"node", model.NodeID},
			{"reason", f.Shard},
		}}},
		{"caused_by", f.Top},
	}
	return ojson.Object{{"error", errObj}, {"status", f.Status}}
}

// scriptShardError wraps a script exception raised while executing a query
// phase script.
func ScriptShardError(idx *model.Index, se *ScriptError) *apierr.Error {
	f := &ShardFailure{Status: 400, Index: idx.Name, Root: se.Object(false), Shard: se.Object(true), Top: se.Object(true)}
	return &apierr.Error{Type: "script_exception", Status: 400, Reason: "script error", Shard: f}
}

// scriptQueryCompileError wraps a compile error of a query script, which
// fails while the shard creates the query.
func scriptQueryCompileError(idx *model.Index, se *ScriptError) *apierr.Error {
	if !se.Compile {
		return ScriptShardError(idx, se)
	}
	shard := ojson.Object{{"type", "query_shard_exception"}, {"reason", "failed to create query: compile error"},
		{"index", idx.Name}, {"index_uuid", idx.UUID}, {"caused_by", se.Object(true)}}
	f := &ShardFailure{Status: 400, Index: idx.Name, Root: se.Object(false), Shard: shard, Top: se.Object(true)}
	return &apierr.Error{Type: "script_exception", Status: 400, Reason: "script error", Shard: f}
}

// plainShardError reports a non-script exception raised on the shard.
func PlainShardError(idx *model.Index, e *apierr.Error) *apierr.Error {
	if e.Shard != nil {
		return e
	}
	status := e.Status
	if status == 0 {
		status = 400
	}
	obj := ojson.Object{{"type", e.Type}, {"reason", e.Reason}}
	top := ojson.Object{{"type", e.Type}, {"reason", e.Reason}, {"caused_by", ojson.Object{{"type", e.Type}, {"reason", e.Reason}}}}
	if status == 404 {
		top = obj
	}
	f := &ShardFailure{Status: status, Index: idx.Name, Root: obj, Shard: obj, Top: top}
	return &apierr.Error{Type: e.Type, Status: status, Reason: e.Reason, Shard: f}
}

// scriptErrForShard turns any script setup or execution error into a shard
// failure.
func ScriptErrForShard(idx *model.Index, se *ScriptError, ae *apierr.Error, query bool) *apierr.Error {
	if se != nil {
		if query {
			return scriptQueryCompileError(idx, se)
		}
		return ScriptShardError(idx, se)
	}
	if ae == nil {
		return nil
	}
	if ae.Type == "x_content_parse_exception" || (ae.Type == "illegal_argument_exception" && ae.Reason == "illegally specified <lang> for a stored script") {
		return ae
	}
	return PlainShardError(idx, ae)
}

// compiledScript is a script prepared for one context, in either language.
type CompiledScript struct {
	spec   *ScriptSpec
	ctx    string
	pl     *Script
	Ex     *expressions.Script
	linked map[*model.Index]*expressions.Linked
}

// prepareScript parses, resolves and compiles a script for a context.
func PrepareScript(raw any, ctxName string) (*CompiledScript, *ScriptError, *apierr.Error) {
	spec, ae := ParseScriptSpec(raw)
	if ae != nil {
		return nil, nil, ae
	}
	return prepareSpec(spec, ctxName)
}

func prepareSpec(spec *ScriptSpec, ctxName string) (*CompiledScript, *ScriptError, *apierr.Error) {
	cs := &CompiledScript{spec: spec, ctx: ctxName}
	switch spec.Lang {
	case "expression":
		ex, se, ae := PrepareExpression(spec.Src, ctxName)
		if se != nil || ae != nil {
			if se != nil {
				se.Script = spec.ErrName()
			}
			return nil, se, ae
		}
		cs.Ex = ex
		cs.linked = map[*model.Index]*expressions.Linked{}
		return cs, nil, nil
	}
	if ae := CheckScriptLang(spec, ctxName); ae != nil {
		return nil, nil, ae
	}
	pl, se := CompilePainless(spec, ctxName)
	if se != nil {
		return nil, se, nil
	}
	cs.pl = pl
	return cs, nil, nil
}

// runOpts carries the per-execution bindings of a script.
type RunOpts struct {
	Score    float64
	Value    any
	Source   *painless.Map  // params._source
	Vars     map[string]any // extra context variables (ctx, state, states, ...)
	Params   map[string]any // extra params (bucket paths, num_terms, ...)
	PlParams *painless.Map  // reuse this params map (shared state)
}

// link binds an expression to an index (cached).
func (cs *CompiledScript) link(idx *model.Index, extra map[string]any) (*expressions.Linked, *ScriptError) {
	if l, ok := cs.linked[idx]; ok && extra == nil {
		return l, nil
	}
	params := map[string]any{}
	for k, v := range cs.spec.Params {
		params[k] = v
	}
	for k, v := range extra {
		params[k] = v
	}
	needsScore := cs.ctx == "score" || cs.ctx == "number_sort" || cs.ctx == "aggs"
	l, err := expressions.Link(cs.Ex, expressions.Env{Fields: exprFields(idx), Params: params, ToFloat: jsonv.ToFloat,
		AllowValue: cs.ctx == "aggs", AllowScore: needsScore})
	if err != nil {
		se := expressionScriptErr(err, cs.spec.Src)
		se.Script = cs.spec.ErrName()
		return nil, se
	}
	if extra == nil {
		cs.linked[idx] = l
	}
	return l, nil
}

// run executes the script for one document (qc may be nil for scripts
// without document access).
func (cs *CompiledScript) Run(qc *QueryCtx, o RunOpts) (any, *ScriptError) {
	if cs.Ex != nil {
		var idx *model.Index
		if qc != nil {
			idx = qc.Idx
		}
		var extra map[string]any
		if len(o.Params) > 0 {
			extra = o.Params
		}
		l, se := cs.link(idx, extra)
		if se != nil {
			return nil, se
		}
		v := l.Eval(exprDoc{qc}, o.Score, o.Value)
		switch cs.ctx {
		case "filter", "aggregation_selector":
			return v != 0, nil
		}
		return v, nil
	}
	params := o.PlParams
	if params == nil {
		params = ScriptParams(cs.spec.Params)
	}
	for k, v := range o.Params {
		params.Put(k, painless.FromJSON(v, painless.Hash))
	}
	if o.Source != nil {
		params.Put("_source", o.Source)
	}
	b := painless.Bindings{Vars: map[string]any{"params": params, "_score": o.Score, "_value": painless.FromJSON(o.Value, painless.Hash)}}
	for k, v := range o.Vars {
		b.Vars[k] = v
	}
	if qc != nil {
		b.Vars["doc"] = DocLookup(qc)
		if cs.ctx == "score" {
			b.Funcs = ScoreBindings(qc)
		}
	}
	return cs.pl.Exec(b, cs.spec.ErrName())
}
