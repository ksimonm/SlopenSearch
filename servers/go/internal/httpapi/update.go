package httpapi

import (
	"net/http"
	"reflect"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
	"slopensearch/internal/ojson"
	"slopensearch/internal/painless"
	"slopensearch/internal/search"
)

func deepMergeDoc(dst, patch map[string]any) {
	for k, v := range patch {
		if pm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				deepMergeDoc(dm, pm)
				continue
			}
		}
		dst[k] = v
	}
}

func (s *Server) handleUpdateDoc(w http.ResponseWriter, r *http.Request, name, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := unknownParams(r, updateDocParams); err != nil {
		writeError(w, err)
		return
	}
	if _, ok := r.URL.Query()["version"]; ok {
		writeError(w, &apierr.Error{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: version is not allowed in update requests;"})
		return
	}
	rv, rerr := refreshValue(r)
	if rerr != nil {
		writeError(w, rerr)
		return
	}
	idx, apiErr := s.resolveOrCreateWriteTarget(name)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if idx.Closed {
		writeError(w, errIndexClosed(idx))
		return
	}
	if apiErr := writeBlockError(idx); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	routing := requestRouting(r)
	if idx.RoutingRequired() && routing == "" {
		writeError(w, routingMissing(idx, id))
		return
	}
	body, rawBody, berr := readDocBody(r)
	if berr != nil {
		if t := mapping.FindNonStandardToken(rawBody); t != nil {
			berr = t.ObjectParserError("UpdateRequest")
		}
		writeError(w, berr)
		return
	}
	allowed := map[string]bool{"doc": true, "upsert": true, "script": true, "scripted_upsert": true,
		"doc_as_upsert": true, "detect_noop": true, "_source": true, "retry_on_conflict": true}
	for k := range body {
		if !allowed[k] {
			writeError(w, &apierr.Error{Type: "x_content_parse_exception", Status: 400,
				Reason: "[UpdateRequest] unknown field [" + k + "]"})
			return
		}
	}
	docPatch, hasDoc := body["doc"].(map[string]any)
	scriptRaw, hasScript := body["script"]
	upsert, hasUpsert := body["upsert"].(map[string]any)
	docAsUpsert, _ := body["doc_as_upsert"].(bool)
	scriptedUpsert, _ := body["scripted_upsert"].(bool)
	detectNoop := true
	if b, ok := body["detect_noop"].(bool); ok {
		detectNoop = b
	}
	if !hasDoc && !hasScript {
		writeError(w, &apierr.Error{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: script or doc is missing;"})
		return
	}
	if hasDoc && hasScript {
		writeError(w, &apierr.Error{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: can't provide both script and doc;"})
		return
	}
	idx.EnsureDocState()
	if err := checkIfSeq(r, idx, id); err != nil {
		writeError(w, err)
		return
	}
	old, exists := idx.Docs[id]
	if exists && !idx.RoutingMatches(id, routing) {
		exists = false
	}
	if exists && !idx.SourceEnabled() {
		writeError(w, &apierr.Error{Type: "document_source_missing_exception", Status: 400,
			Reason: "[" + id + "]: document source missing", Fields: map[string]any{"index": idx.Name, "shard": "0"}})
		return
	}
	if !exists {
		if !hasUpsert && !docAsUpsert {
			if hasScript && scriptedUpsert {
				writeError(w, &apierr.Error{Type: "document_missing_exception", Status: 404,
					Reason: "[" + id + "]: document missing", Fields: map[string]any{"index": idx.Name, "shard": "0"}})
				return
			}
			writeError(w, &apierr.Error{Type: "document_missing_exception", Status: 404,
				Reason: "[" + id + "]: document missing", Fields: map[string]any{"index": idx.Name, "shard": "0"}})
			return
		}
		var src map[string]any
		if docAsUpsert {
			src = deepCopyMap(docPatch)
		} else {
			src = deepCopyMap(upsert)
		}
		op := "index"
		if hasScript && scriptedUpsert {
			ver := int64(0)
			res, serr := runUpdateScriptCtx(scriptRaw, src, idx.Name, id, ver, "create", nil)
			if serr != nil {
				writeError(w, serr)
				return
			}
			op = res.op
			src = res.source
		}
		if op == "none" || op == "noop" {
			resp := ojson.Object{
				{"_index", idx.Name}, {"_id", id}, {"_version", int64(-1)}, {"result", "noop"},
				{"_shards", map[string]any{"total": 0, "successful": 0, "failed": 0}},
				{"get", map[string]any{"found": false}},
			}
			writeJSON(w, 200, resp)
			return
		}
		pparams := map[string]any{}
		if p := r.URL.Query().Get("pipeline"); p != "" {
			pparams["pipeline"] = p
		}
		src, apiErr = s.ingest.Apply(idx, src, pparams)
		if apiErr != nil {
			writeError(w, apiErr)
			return
		}
		props := mapping.EnsureProperties(idx.Mappings)
		if derr := mappingShapeConflict(props, src); derr != nil {
			writeError(w, derr)
			return
		}
		if derr := checkDynamicStrict(idx, props, src); derr != nil {
			writeError(w, derr)
			return
		}
		if derr := mergeMappingForIndex(idx, props, src); derr != nil {
			writeError(w, derr)
			return
		}
		mapping.EnsureFieldTypes(idx)
		if derr := mapping.ComputeDocValues(idx, id, src); derr != nil {
			writeError(w, derr)
			return
		}
		idx.Order = append(idx.Order, id)
		idx.Docs[id] = src
		delete(idx.RawDocs, id)
		idx.Versions[id] = 1
		seq := idx.AllocSeqNo()
		idx.SeqNos[id] = seq
		idx.PrimaryTerms[id] = 1
		idx.IndexTotal++
		if routing != "" {
			idx.Routing[id] = routing
		}
		if rv == "true" || rv == "wait_for" {
			delete(idx.Pending, id)
		} else {
			idx.Pending[id] = src
		}
		resp := ojson.Object{
			{"_index", idx.Name}, {"_id", id}, {"_version", int64(1)}, {"result", "created"},
		}
		if rv == "true" {
			resp = append(resp, ojson.KV{"forced_refresh", true})
		}
		resp = append(resp, ojson.KV{"_shards", map[string]any{"total": idx.NumShards(), "successful": idx.NumShards(), "failed": 0}},
			ojson.KV{"_seq_no", seq}, ojson.KV{"_primary_term", int64(1)})
		if getSpecified(r, body) {
			resp = append(resp, ojson.KV{"get", updateGetObject(idx, id, src, seq, r, body)})
		}
		writeJSON(w, 201, resp)
		return
	}
	newDoc := deepCopyMap(old)
	op := "index"
	if hasScript {
		res, serr := runUpdateScript(scriptRaw, newDoc, idx.Name, id, idx.Versions[id])
		if serr != nil {
			writeError(w, serr)
			return
		}
		op = res.op
		newDoc = res.source
	} else {
		deepMergeDoc(newDoc, docPatch)
	}
	if op == "delete" {
		ver := idx.Versions[id] + 1
		seq := idx.AllocSeqNo()
		delete(idx.Docs, id)
		delete(idx.Pending, id)
		mapping.RemoveDocValues(idx, id)
		idx.DeleteTotal++
		idx.Versions[id] = ver
		idx.SeqNos[id] = seq
		resp := ojson.Object{
			{"_index", idx.Name}, {"_id", id}, {"_version", ver}, {"result", "deleted"},
			{"_shards", map[string]any{"total": idx.NumShards(), "successful": idx.NumShards(), "failed": 0}},
			{"_seq_no", seq}, {"_primary_term", int64(1)},
		}
		writeJSON(w, 200, resp)
		return
	}
	pparams := map[string]any{}
	if p := r.URL.Query().Get("pipeline"); p != "" {
		pparams["pipeline"] = p
	}
	newDoc, apiErr = s.ingest.Apply(idx, newDoc, pparams)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if op == "none" || op == "noop" || (detectNoop && reflect.DeepEqual(old, newDoc)) {
		resp := ojson.Object{
			{"_index", idx.Name}, {"_id", id}, {"_version", idx.Versions[id]}, {"result", "noop"},
			{"_shards", map[string]any{"total": 0, "successful": 0, "failed": 0}},
			{"_seq_no", idx.SeqNos[id]}, {"_primary_term", int64(1)},
		}
		if getSpecified(r, body) {
			resp = append(resp, ojson.KV{"get", updateGetObject(idx, id, old, idx.SeqNos[id], r, body)})
		}
		writeJSON(w, 200, resp)
		return
	}
	props := mapping.EnsureProperties(idx.Mappings)
	if derr := mappingShapeConflict(props, newDoc); derr != nil {
		writeError(w, derr)
		return
	}
	if derr := checkDynamicStrict(idx, props, newDoc); derr != nil {
		writeError(w, derr)
		return
	}
	if derr := mergeMappingForIndex(idx, props, newDoc); derr != nil {
		writeError(w, derr)
		return
	}
	mapping.EnsureFieldTypes(idx)
	if derr := mapping.ComputeDocValues(idx, id, newDoc); derr != nil {
		writeError(w, derr)
		return
	}
	ver := idx.Versions[id] + 1
	seq := idx.AllocSeqNo()
	idx.Docs[id] = newDoc
	delete(idx.RawDocs, id)
	idx.Versions[id] = ver
	idx.SeqNos[id] = seq
	idx.PrimaryTerms[id] = 1
	idx.IndexTotal++
	if routing != "" {
		idx.Routing[id] = routing
	}
	if rv == "true" || rv == "wait_for" {
		delete(idx.Pending, id)
	} else {
		idx.MarkPending(id, newDoc, true)
	}
	resp := ojson.Object{
		{"_index", idx.Name}, {"_id", id}, {"_version", ver}, {"result", "updated"},
	}
	if rv == "true" {
		resp = append(resp, ojson.KV{"forced_refresh", true})
	}
	resp = append(resp, ojson.KV{"_shards", map[string]any{"total": idx.NumShards(), "successful": idx.NumShards(), "failed": 0}},
		ojson.KV{"_seq_no", seq}, ojson.KV{"_primary_term", int64(1)})
	if getSpecified(r, body) {
		resp = append(resp, ojson.KV{"get", updateGetObject(idx, id, newDoc, seq, r, body)})
	}
	writeJSON(w, 200, resp)
}

func getSpecified(r *http.Request, body map[string]any) bool {
	if v, ok := body["_source"]; ok {
		if b, ok := v.(bool); ok && !b {
			return false
		}
		return true
	}
	q := r.URL.Query()
	if v, ok := q["_source"]; ok {
		return len(v) == 0 || v[0] != "false"
	}
	return q.Get("_source_includes") != "" || q.Get("_source_excludes") != ""
}

func updateGetObject(idx *model.Index, id string, src map[string]any, seq int64, r *http.Request, body map[string]any) ojson.Object {
	includes, excludes, _, _ := sourceFilterFromQuery(r.URL.Query())
	if v, ok := body["_source"]; ok {
		inc, exc, omit := sourceSpecFromValue(v)
		if omit {
			return ojson.Object{{"_seq_no", seq}, {"_primary_term", int64(1)}, {"found", true}}
		}
		if inc != nil || exc != nil {
			includes, excludes = inc, exc
		}
	}
	out := ojson.Object{{"_seq_no", seq}, {"_primary_term", int64(1)}, {"found", true}}
	if idx.SourceEnabled() {
		out = append(out, ojson.KV{"_source", search.FilterSource(src, includes, excludes)})
	}
	return out
}

type updateScriptResult struct {
	op     string
	source map[string]any
	id     any // ctx._id after the script
	index  any // ctx._index after the script
	rawOp  string
}

// updateScriptFailure wraps a script problem the way the update API reports
// it: "failed to execute script" caused by the script exception.
func updateScriptFailure(se *search.ScriptError, cause *apierr.Error) *apierr.Error {
	var causeObj ojson.Object
	if se != nil {
		causeObj = se.Object(true)
	} else {
		causeObj = ojson.Object{{"type", cause.Type}, {"reason", cause.Reason}}
		if cause.CausedBy != nil {
			causeObj = append(causeObj, ojson.KV{"caused_by", ojson.Object{{"type", cause.CausedBy.Type}, {"reason", cause.CausedBy.Reason}}})
		}
	}
	body := ojson.Object{{"error", ojson.Object{
		{"root_cause", []any{ojson.Object{{"type", "illegal_argument_exception"}, {"reason", "failed to execute script"}}}},
		{"type", "illegal_argument_exception"}, {"reason", "failed to execute script"},
		{"caused_by", causeObj},
	}}, {"status", 400}}
	e := &apierr.Error{Type: "illegal_argument_exception", Status: 400, Reason: "failed to execute script",
		Shard: &search.ShardFailure{Raw: body, Status: 400}}
	if se != nil {
		e.CausedBy = &apierr.Error{Type: "script_exception", Reason: map[bool]string{true: "compile error", false: "runtime error"}[se.Compile]}
		e.Extra = se
	} else {
		e.CausedBy = cause
	}
	return e
}

// runUpdateScript runs an update-context script (update, update_by_query,
// reindex) against a copy of ctx and reports the resulting operation.
func runUpdateScript(raw any, source map[string]any, index, id string, version int64) (updateScriptResult, *apierr.Error) {
	return runUpdateScriptCtx(raw, source, index, id, version, "index", nil)
}

func runUpdateScriptCtx(raw any, source map[string]any, index, id string, version int64, op string, routing any) (updateScriptResult, *apierr.Error) {
	spec, ae := search.ParseScriptSpec(raw)
	if ae != nil {
		return updateScriptResult{}, updateScriptFailure(nil, ae)
	}
	if spec.Lang == "expression" {
		_, se, ae := search.PrepareExpression(spec.Src, "update")
		if se != nil {
			se.Script = spec.ErrName()
		}
		return updateScriptResult{}, updateScriptFailure(se, ae)
	}
	if ae := search.CheckScriptLang(spec, "update"); ae != nil {
		return updateScriptResult{}, updateScriptFailure(nil, ae)
	}
	script, se := search.CompilePainless(spec, "update")
	if se != nil {
		return updateScriptResult{}, updateScriptFailure(se, nil)
	}
	ctx := painless.NewMap(painless.Hash)
	ctx.Put("_index", index)
	ctx.Put("_id", id)
	ctx.Put("_version", int64(version))
	if rs, ok := routing.(string); ok && rs == "" {
		routing = nil
	}
	ctx.Put("_routing", painless.FromJSON(routing, painless.Linked))
	ctx.Put("op", op)
	ctx.Put("_now", nowMillis())
	ctx.Put("_source", painless.FromJSON(jsonv.DeepCopy(source), painless.Linked))
	_, rerr := script.Exec(painless.Bindings{Vars: map[string]any{"params": search.ScriptParams(spec.Params), "ctx": ctx}}, spec.ErrName())
	if rerr != nil {
		return updateScriptResult{}, updateScriptFailure(rerr, nil)
	}
	res := updateScriptResult{op: "index"}
	rawOp, _ := ctx.Get("op")
	res.rawOp = painless.ToString(rawOp)
	switch res.rawOp {
	case "delete":
		res.op = "delete"
	case "index", "create":
		res.op = "index"
	default:
		res.op = "noop"
	}
	if src, ok := ctx.Get("_source"); ok {
		if m, ok := painless.ToJSON(src).(map[string]any); ok {
			res.source = m
		}
	}
	if res.source == nil {
		res.source = map[string]any{}
	}
	res.id, _ = ctx.Get("_id")
	res.index, _ = ctx.Get("_index")
	return res, nil
}

func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	out := map[string]any{}
	for k, v := range m {
		switch t := v.(type) {
		case map[string]any:
			out[k] = deepCopyMap(t)
		case []any:
			out[k] = deepCopyAnySlice(t)
		default:
			out[k] = v
		}
	}
	return out
}

func deepCopyAnySlice(a []any) []any {
	out := make([]any, len(a))
	for i, v := range a {
		switch t := v.(type) {
		case map[string]any:
			out[i] = deepCopyMap(t)
		case []any:
			out[i] = deepCopyAnySlice(t)
		default:
			out[i] = v
		}
	}
	return out
}
