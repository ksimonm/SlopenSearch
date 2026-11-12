package main

import (
	"net/http"
	"strconv"
	"strings"
)

type reindexFailure struct {
	index string
	id    string
	typ   string
	rex   string
	code  int
}

func (f reindexFailure) json(destUUID string) map[string]any {
	return map[string]any{
		"index": f.index,
		"id":    f.id,
		"cause": map[string]any{
			"type":       f.typ,
			"reason":     f.rex,
			"index":      f.index,
			"shard":      "0",
			"index_uuid": destUUID,
		},
		"status": f.code,
	}
}

func (s *Server) handleReindex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, "POST")
		return
	}
	body, apiErr2 := readJSON(r)
	if apiErr2 != nil {
		writeError(w, apiErr2)
		return
	}
	for k := range body {
		switch k {
		case "source", "dest", "script", "max_docs", "conflicts", "wait_for_completion",
			"refresh", "timeout", "scroll", "size", "slices":
		default:
			writeError(w, &apiError{Type: "x_content_parse_exception", Status: 400,
				Reason: "[1:0] [reindex] unknown field [" + k + "]"})
			return
		}
	}
	srcRaw, _ := body["source"].(map[string]any)
	destRaw, _ := body["dest"].(map[string]any)
	if destRaw == nil {
		writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: index must be specified;"})
		return
	}
	destName, _ := destRaw["index"].(string)
	if destName == "" {
		writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: index must be specified;"})
		return
	}
	if srcRaw == nil || srcRaw["index"] == nil {
		writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: use _all if you really want to copy from all existing indexes;"})
		return
	}
	if remote, ok := srcRaw["remote"].(map[string]any); ok {
		host, _ := remote["host"].(string)
		host = strings.TrimPrefix(host, "http://")
		host = strings.TrimPrefix(host, "https://")
		writeError(w, &apiError{Type: "illegal_argument_exception", Status: 400,
			Reason: "[" + host + "] not allowlisted in reindex.remote.allowlist"})
		return
	}
	var srcSpecs []string
	switch t := srcRaw["index"].(type) {
	case string:
		srcSpecs = []string{t}
	case []any:
		for _, v := range t {
			if s, ok := v.(string); ok {
				srcSpecs = append(srcSpecs, s)
			}
		}
	}
	srcSpec := strings.Join(srcSpecs, ",")
	if srcSpec == "" {
		writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: use _all if you really want to copy from all existing indexes;"})
		return
	}
	if srcSpec == destName {
		writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: reindex cannot write into an index its reading from [" + srcSpec + "];"})
		return
	}
	maxDocs := -1
	if v, ok := body["max_docs"]; ok && v != nil {
		n, isInt := settingsValueInt(v)
		if !isInt {
			writeError(w, &apiError{Type: "x_content_parse_exception", Status: 400,
				Reason: "[1:0] [reindex] failed to parse field [max_docs]",
				CausedBy: &apiError{Type: "illegal_argument_exception",
					Reason: "[max_docs] should be >= [slices]"}})
			return
		}
		if n < 0 {
			writeError(w, &apiError{Type: "x_content_parse_exception", Status: 400,
				Reason: "[1:0] [reindex] failed to parse field [max_docs]",
				CausedBy: &apiError{Type: "illegal_argument_exception",
					Reason: "[max_docs] parameter cannot be negative, found [" + strconv.Itoa(n) + "]"}})
			return
		}
		if n < 1 {
			writeError(w, &apiError{Type: "x_content_parse_exception", Status: 400,
				Reason: "[1:0] [reindex] failed to parse field [max_docs]",
				CausedBy: &apiError{Type: "illegal_argument_exception",
					Reason: "[max_docs] should be >= [slices]"}})
			return
		}
		maxDocs = n
	}
	conflictsProceed := false
	if c, ok := body["conflicts"].(string); ok && c == "proceed" {
		conflictsProceed = true
	}
	size := 1000
	if v, ok := srcRaw["size"]; ok {
		if n, isInt := settingsValueInt(v); isInt && n > 0 {
			size = n
		}
	}
	opType, _ := destRaw["op_type"].(string)
	versionType, _ := destRaw["version_type"].(string)
	scriptRaw, hasScript := body["script"]
	var query map[string]any
	if q, ok := srcRaw["query"].(map[string]any); ok {
		query = q
	}
	var sourceInclude []string
	if si, ok := srcRaw["_source"].([]any); ok {
		for _, f := range si {
			if fs, ok := f.(string); ok {
				sourceInclude = append(sourceInclude, fs)
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	allowNo := false
	srcIndices, apiErr := s.resolveSpec2(srcSpec, resolveOpts2{allowNoIndices: &allowNo})
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	type item struct {
		idx *Index
		id  string
		doc map[string]any
	}
	var items []item
	for _, sidx := range srcIndices {
		for _, id := range sidx.Order {
			doc := sidx.Docs[id]
			if query != nil {
				qc := newQueryCtxForDoc(sidx, id, doc)
				ok, _, qerr := qc.eval(query)
				if qerr != nil {
					writeError(w, qerr)
					return
				}
				if !ok {
					continue
				}
			}
			items = append(items, item{idx: sidx, id: id, doc: doc})
		}
	}
	if maxDocs >= 0 && len(items) > maxDocs {
		items = items[:maxDocs]
	}
	total := len(items)
	batches := 0
	if total > 0 {
		batches = (total + size - 1) / size
	}

	created, updated, deleted, noops, versionConflicts := 0, 0, 0, 0, 0
	failures := []any{}
	destIdx := s.indices[destName]
	destNameErr := validateIndexName(destName)
	for _, it := range items {
		doc := deepCopyJSON(it.doc).(map[string]any)
		id := it.id
		targetName := destName
		if hasScript {
			res, serr := runUpdateScriptCtx(scriptRaw, doc, it.idx.Name, id, -1, "index", it.idx.Routing[id])
			if serr != nil {
				if se, ok := serr.Extra.(*plScriptErr); ok {
					writeJSON(w, 400, se.errorBody())
					return
				}
				if serr.CausedBy != nil {
					writeError(w, &apiError{Type: serr.CausedBy.Type, Status: 400, Reason: serr.CausedBy.Reason})
					return
				}
				writeError(w, serr)
				return
			}
			switch res.rawOp {
			case "noop":
				noops++
				continue
			case "delete":
				deleted++
				continue
			case "index", "create":
			default:
				writeError(w, errBad("Operation type ["+res.rawOp+"] not allowed, only [noop, index, delete] are allowed"))
				return
			}
			doc = res.source
			if res.id == nil {
				id = s.nextID()
			} else {
				id = plToString(res.id)
			}
			if n := plToString(res.index); n != it.idx.Name && res.index != nil {
				targetName = n
			}
		}
		destName := targetName
		destIdx := s.indices[targetName]
		destUUID := "_na_"
		if destIdx != nil {
			destUUID = destIdx.UUID
		}
		if destIdx == nil {
			if destNameErr != nil {
				failures = append(failures, reindexFailure{index: destName, id: it.id,
					typ: destNameErr.Type, rex: destNameErr.Reason, code: destNameErr.Status}.json("_na_"))
				continue
			}
			newIdx, apiErr3 := s.createIndexInternal(destName, map[string]any{}, false)
			if apiErr3 != nil {
				failures = append(failures, reindexFailure{index: destName, id: it.id,
					typ: apiErr3.Type, rex: apiErr3.Reason, code: apiErr3.Status}.json("_na_"))
				continue
			}
			destIdx = newIdx
			destUUID = destIdx.UUID
		}
		existing := false
		if _, has := destIdx.Docs[id]; has {
			existing = true
		}
		if existing && (opType == "create" || versionType == "external" || versionType == "external_gte") {
			versionConflicts++
			if !conflictsProceed {
				reason := ""
				if opType == "create" {
					reason = "[" + id + "]: version conflict, document already exists (current version [1])"
				} else {
					reason = "[" + id + "]: version conflict, current version [1] is higher or equal to the one provided [1]"
				}
				failures = append(failures, reindexFailure{index: destName, id: it.id,
					typ: "version_conflict_engine_exception", rex: reason, code: 409}.json(destUUID))
			}
			continue
		}
		if sourceInclude != nil {
			filtered := map[string]any{}
			for _, f := range sourceInclude {
				if v, ok := doc[f]; ok {
					filtered[f] = v
				}
			}
			doc = filtered
		}
		if existing {
			updated++
		} else {
			created++
			destIdx.Order = append(destIdx.Order, id)
		}
		pparams := map[string]any{}
		if pn, ok := destRaw["pipeline"].(string); ok && pn != "" {
			pparams["pipeline"] = pn
		}
		doc, apiErr = s.applyPipelines(destIdx, doc, pparams)
		if apiErr != nil {
			if existing {
				updated--
			} else {
				created--
				if len(destIdx.Order) > 0 {
					destIdx.Order = destIdx.Order[:len(destIdx.Order)-1]
				}
			}
			failures = append(failures, reindexFailure{index: destName, id: id,
				typ: apiErr.Type, rex: apiErr.Reason, code: apiErr.Status}.json(destUUID))
			continue
		}
		props := ensureProperties(destIdx.Mappings)
		if derr := mergeMappingForIndex(destIdx, props, doc); derr != nil {
			failures = append(failures, reindexFailure{index: destName, id: id,
				typ: derr.Type, rex: derr.Reason, code: derr.Status}.json(destUUID))
			continue
		}
		destIdx.FieldTypes = map[string]string{}
		buildFieldTypes(destIdx.Mappings, "", destIdx.FieldTypes)
		if derr := destIdx.computeDocValues(id, doc); derr != nil {
			if existing {
				updated--
			} else {
				created--
				if len(destIdx.Order) > 0 {
					destIdx.Order = destIdx.Order[:len(destIdx.Order)-1]
				}
			}
			failures = append(failures, reindexFailure{index: destName, id: id,
				typ: derr.Type, rex: derr.Reason, code: derr.Status}.json(destUUID))
			continue
		}
		destIdx.Docs[id] = doc
		destIdx.IndexTotal++
	}
	destIdx = s.indices[destName]
	if destIdx != nil {
		s.bumpState()
	}
	status := 200
	if versionConflicts > 0 && !conflictsProceed {
		status = 409
	} else if len(failures) > 0 {
		status = 400
	}
	writeJSON(w, status, map[string]any{
		"took":                   1,
		"timed_out":              false,
		"total":                  total,
		"updated":                updated,
		"created":                created,
		"deleted":                deleted,
		"batches":                batches,
		"version_conflicts":      versionConflicts,
		"noops":                  noops,
		"retries":                map[string]any{"bulk": 0, "search": 0},
		"throttled_millis":       0,
		"requests_per_second":    -1.0,
		"throttled_until_millis": 0,
		"failures":               failures,
	})
}
