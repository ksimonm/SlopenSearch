package aggs

import (
	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/model"
	"slopensearch/internal/ojson"
	"slopensearch/internal/painless"
	"slopensearch/internal/search"
)

// aggScriptRun runs an aggregation script for one document; with a field
// it is a value script evaluated once per field value (_value).
func aggScriptRun(cs *search.CompiledScript, idx *model.Index, doc map[string]any, fieldVals []any, useField bool) ([]any, *apierr.Error) {
	id, _ := doc["__slop_id"].(string)
	qc := search.NewQueryCtxForDoc(idx, id, doc)
	score, _ := jsonv.ToFloat(doc["__slop_score"])
	out := []any{}
	if useField {
		for _, v := range fieldVals {
			r, se := cs.Run(qc, search.RunOpts{Value: v, Score: score})
			if se != nil {
				return nil, search.ScriptShardError(idx, se)
			}
			out = append(out, r)
		}
		return out, nil
	}
	r, se := cs.Run(qc, search.RunOpts{Score: score})
	if se != nil {
		return nil, search.ScriptShardError(idx, se)
	}
	if items, ok := painless.ToItems(r); ok {
		return append(out, items...), nil
	}
	if r == nil {
		return out, nil
	}
	return append(out, r), nil
}

// prepareAggScript compiles the script of an aggregation for the aggs context.
func prepareAggScript(idx *model.Index, raw any) (*search.CompiledScript, *apierr.Error) {
	cs, se, ae := search.PrepareScript(raw, "aggs")
	if se != nil || ae != nil {
		return nil, search.ScriptFailure(idx, se, ae, false)
	}
	return cs, nil
}

// evalTermScript evaluates a terms-like script (composite sources) for one
// document and returns its first value as a term.
func evalTermScript(doc map[string]any, idx *model.Index, raw any) (any, bool) {
	cs, err := prepareAggScript(idx, raw)
	if err != nil {
		return nil, false
	}
	res, err := aggScriptRun(cs, idx, doc, nil, false)
	if err != nil || len(res) == 0 || res[0] == nil {
		return nil, false
	}
	return termsScriptKey(res[0], false, "", cs.Ex != nil), true
}

// pipelineScriptError reports a failing pipeline aggregation script, which
// runs on the coordinating node during the reduce (fetch) phase.
func pipelineScriptError(se *search.ScriptError) *apierr.Error {
	body := ojson.Object{{"error", ojson.Object{
		{"root_cause", []any{}}, {"type", "search_phase_execution_exception"}, {"reason", ""},
		{"phase", "fetch"}, {"grouped", true}, {"failed_shards", []any{}}, {"caused_by", se.Object(true)},
	}}, {"status", 400}}
	return &apierr.Error{Type: "search_phase_execution_exception", Status: 400, Shard: &search.ShardFailure{Raw: body, Status: 400}}
}

// runPipelineScript evaluates a bucket_script / bucket_selector script with
// the bucket path values as params.
func runPipelineScript(raw any, ctxName string, vals map[string]any) (any, *apierr.Error) {
	cs, se, ae := search.PrepareScript(raw, ctxName)
	if ae != nil {
		return nil, ae
	}
	if se != nil {
		return nil, pipelineScriptError(se)
	}
	v, rerr := cs.Run(nil, search.RunOpts{Params: vals})
	if rerr != nil {
		return nil, pipelineScriptError(rerr)
	}
	return v, nil
}

// scriptedMetric runs the init/map/combine/reduce scripts of a
// scripted_metric aggregation over the given documents (one shard).
func scriptedMetric(name string, spec map[string]any, docs []map[string]any, idx *model.Index) (any, *apierr.Error) {
	if _, ok := spec["map_script"]; !ok {
		return nil, apierr.BadRequest("Required [map_script]")
	}
	for _, req := range []struct{ key, label string }{{"combine_script", "combineScript"}, {"reduce_script", "reduceScript"}} {
		if _, ok := spec[req.key]; !ok {
			return nil, search.ShardError(idx, apierr.BadRequest("["+req.label+"] must not be null: ["+name+"]"))
		}
	}
	aggParams, _ := spec["params"].(map[string]any)
	params := search.ScriptParams(aggParams)
	state := painless.NewMap(painless.Hash)
	prep := func(key, ctxName string) (*search.CompiledScript, *apierr.Error) {
		cs, se, ae := search.PrepareScript(spec[key], ctxName)
		if se != nil || ae != nil {
			return nil, search.ScriptFailure(idx, se, ae, false)
		}
		return cs, nil
	}
	vars := map[string]any{"state": state}
	if _, ok := spec["init_script"]; ok {
		cs, err := prep("init_script", "aggs_init")
		if err != nil {
			return nil, err
		}
		if _, se := cs.Run(nil, search.RunOpts{PlParams: params, Vars: vars}); se != nil {
			return nil, search.ScriptShardError(idx, se)
		}
	}
	mapCS, err := prep("map_script", "aggs_map")
	if err != nil {
		return nil, err
	}
	for _, d := range docs {
		id, _ := d["__slop_id"].(string)
		score, _ := jsonv.ToFloat(d["__slop_score"])
		if _, se := mapCS.Run(search.NewQueryCtxForDoc(idx, id, d), search.RunOpts{PlParams: params, Vars: vars, Score: score}); se != nil {
			return nil, search.ScriptShardError(idx, se)
		}
	}
	combCS, err := prep("combine_script", "aggs_combine")
	if err != nil {
		return nil, err
	}
	combined, se := combCS.Run(nil, search.RunOpts{PlParams: params, Vars: vars})
	if se != nil {
		return nil, search.ScriptShardError(idx, se)
	}
	redCS, err := prep("reduce_script", "aggs_reduce")
	if err != nil {
		return nil, err
	}
	res, se := redCS.Run(nil, search.RunOpts{PlParams: params, Vars: map[string]any{"states": painless.NewList(combined)}})
	if se != nil {
		return nil, pipelineScriptError(se)
	}
	return ojson.Object{{"value", search.JavaDoubles(painless.ToJSON(res))}}, nil
}

// scriptValueDocs evaluates the script of a numeric bucket aggregation
// without a field and exposes its values as a synthetic field of copied
// documents.
func scriptValueDocs(spec map[string]any, docs []map[string]any, idx *model.Index, field string) ([]map[string]any, string, *apierr.Error) {
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
			if painless.IsNumber(r) {
				vals = append(vals, painless.F64(r))
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
