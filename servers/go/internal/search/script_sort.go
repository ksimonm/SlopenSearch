package search

import (
	"slopensearch/internal/painless"
)

// scriptSortValue evaluates a _script sort for one hit outside the main sort
// pass (e.g. for search_after); errors yield no value.
func scriptSortValue(sp SortSpec, h MultiHit) any {
	ctxName := "number_sort"
	if sp.ScriptType == "string" {
		ctxName = "string_sort"
	}
	cs, se, ae := PrepareScript(sp.ScriptRaw, ctxName)
	if se != nil || ae != nil {
		return nil
	}
	v, rerr := cs.Run(NewQueryCtxForDoc(h.Idx, h.Id, h.Doc), RunOpts{Score: h.Score})
	if rerr != nil {
		return nil
	}
	if ctxName == "number_sort" {
		return painless.F64(v)
	}
	return v
}
