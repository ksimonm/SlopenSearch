package main

import (
	"slopensearch/internal/painless"
	"slopensearch/internal/search"
)

// Glue between the scripting engines and the APIs that run scripts
// (search, aggregations, update, ingest): script specs, stored-script
// resolution, language checks and the shard-failure envelopes.

// ---- shard failure envelopes ----

// ---- bindings ----

// ---- compiled scripts ----

// scriptToString renders org.opensearch.script.Script#toString.
func scriptToString(raw any) string {
	spec, err := search.ParseScriptSpec(raw)
	if err != nil {
		return ""
	}
	params := painless.ToString(search.ScriptParams(spec.Params))
	if spec.Id != "" {
		return "Script{type=stored, lang='null', idOrCode='" + spec.Id + "', options=null, params=" + params + "}"
	}
	return "Script{type=inline, lang='" + spec.Lang + "', idOrCode='" + spec.Src + "', options={}, params=" + params + "}"
}

// ---- aggregation scripts ----

// scriptSortValue evaluates a _script sort for one hit outside the main sort
// pass (e.g. for search_after); errors yield no value.
func scriptSortValue(sp search.SearchSortSpec, h search.MultiHit) any {
	ctxName := "number_sort"
	if sp.ScriptType == "string" {
		ctxName = "string_sort"
	}
	cs, se, ae := search.PrepareScript(sp.ScriptRaw, ctxName)
	if se != nil || ae != nil {
		return nil
	}
	v, rerr := cs.Run(search.NewQueryCtxForDoc(h.Idx, h.Id, h.Doc), search.RunOpts{Score: h.Score})
	if rerr != nil {
		return nil
	}
	if ctxName == "number_sort" {
		return painless.F64(v)
	}
	return v
}
