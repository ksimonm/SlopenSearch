package search

import (
	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/painless"
)

// evalScriptFields evaluates the script_fields section of a search body.
// All script fields of a hit share one params._source map, so a script that
// mutates it is seen by the next one.
func EvalScriptFields(hit MultiHit, specs map[string]any) (map[string]any, *apierr.Error) {
	out := map[string]any{}
	qc := NewQueryCtxForDoc(hit.Idx, hit.Id, hit.Doc)
	src := map[string]any{}
	for k, v := range hit.Doc {
		if k != "_ignored" {
			src[k] = v
		}
	}
	source := painless.FromJSON(DeepCopySource(src), painless.Linked).(*painless.Map)
	for _, name := range jsonv.SortedKeys(specs) {
		spec, _ := specs[name].(map[string]any)
		ignore, _ := spec["ignore_failure"].(bool)
		cs, se, ae := PrepareScript(spec["script"], "field")
		if se != nil || ae != nil {
			return nil, ScriptErrForShard(hit.Idx, se, ae, false)
		}
		v, rerr := cs.Run(qc, RunOpts{Source: source})
		if rerr != nil {
			if ignore {
				continue
			}
			return nil, ScriptShardError(hit.Idx, rerr)
		}
		if vals, ok := fieldScriptValues(v); ok {
			out[name] = vals
		}
	}
	return out, nil
}
