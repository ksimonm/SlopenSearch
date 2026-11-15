package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"slopensearch/internal/aggs"
	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
	"slopensearch/internal/ojson"
	"slopensearch/internal/painless"
	"slopensearch/internal/search"
	"slopensearch/internal/settings"
	"slopensearch/internal/suggest"
)

type fieldSpec struct {
	name   string
	format string
}

func parseFieldSpecs(raw any) []fieldSpec {
	var out []fieldSpec
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	for _, e := range list {
		switch t := e.(type) {
		case string:
			out = append(out, fieldSpec{name: t})
		case map[string]any:
			name, _ := jsonv.ToString(t["field"])
			if name == "" {
				continue
			}
			format, _ := jsonv.ToString(t["format"])
			out = append(out, fieldSpec{name: name, format: format})
		}
	}
	return out
}

// docValuesOutput returns the values to render for docvalue_fields / fields.
func docValuesOutput(idx *model.Index, id, field string, spec fieldSpec, fallbackSource bool) ([]any, bool, *apierr.Error) {
	field = mapping.ResolveAliasField(idx, field)
	if i := strings.Index(field, "#"); i > 0 {
		base := field[:i]
		if bdef, ok := mapping.FieldDef(idx, base); ok {
			if bt, _ := bdef["type"].(string); bt == "join" {
				name, parent := search.JoinParts(idx.Docs[id][base])
				relations, _ := bdef["relations"].(map[string]any)
				if name != "" && parent != "" && search.JoinParentRelation(relations, name) == field[i+1:] {
					return []any{parent}, true, nil
				}
				return nil, false, nil
			}
		}
	}
	def, hasDef := mapping.FieldDef(idx, field)
	if !hasDef {
		return nil, false, nil
	}
	if t, _ := def["type"].(string); t == "knn_vector" {
		return search.KnnFieldOutput(idx, id, field, def, fallbackSource)
	}
	ftype := ""
	hasDV := true
	if hasDef {
		ftype, _ = def["type"].(string)
		if dv, ok := def["doc_values"].(bool); ok && !dv {
			hasDV = false
		}
		if ftype == "binary" {
			if dv, ok := def["doc_values"].(bool); !ok || !dv {
				hasDV = false
			}
		}
	}
	if spec.format != "" && spec.format != "use_field_mapping" {
		switch ftype {
		case "integer", "long", "short", "byte", "unsigned_long":
			reason := "Field [" + field + "] of type [" + ftype + "] doesn't support formats."
			return nil, false, &apierr.Error{Type: "search_phase_execution_exception", Status: 400, Reason: "all shards failed",
				RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason},
				CausedBy:  &apierr.Error{Type: "illegal_argument_exception", Reason: reason}}
		}
	}
	var vals []any
	if dv := idx.DocValues[id]; dv != nil {
		vals = dv[field]
	}
	if ftype == "join" {
		out := []any{}
		for _, v := range vals {
			switch t := v.(type) {
			case map[string]any:
				if n, _ := t["name"].(string); n != "" {
					out = append(out, n)
				}
			case string:
				out = append(out, t)
			}
		}
		if len(out) == 0 {
			return nil, false, nil
		}
		return out, true, nil
	}
	formatted := func(list []any) []any {
		out := make([]any, 0, len(list))
		for _, v := range list {
			out = append(out, search.FormatDocValue(idx, field, ftype, v, spec.format))
		}
		return out
	}
	if search.TextLikeTypes[ftype] || !hasDV {
		if !fallbackSource {
			if search.TextLikeTypes[ftype] {
				reason := "Text fields are not optimised for operations that require per-document field data like aggregations and sorting, so these operations are disabled by default. Please use a keyword field instead. Alternatively, set fielddata=true on [" + field + "] in order to load field data by uninverting the inverted index. Note that this can use significant memory."
				return nil, false, &apierr.Error{Type: "search_phase_execution_exception", Status: 400, Reason: "all shards failed",
					RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason},
					CausedBy:  &apierr.Error{Type: "illegal_argument_exception", Reason: reason}}
			}
			if !hasDV && (len(vals) > 0 || ftype == "binary") {
				reason := "Can't load fielddata on [" + field + "] because fielddata is unsupported on fields of type [" + ftype + "]. Use doc values instead."
				return nil, false, &apierr.Error{Type: "search_phase_execution_exception", Status: 400, Reason: "all shards failed",
					RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason},
					CausedBy:  &apierr.Error{Type: "illegal_argument_exception", Reason: reason}}
			}
			return nil, false, nil
		}
		// fall back to _source values
		if v, ok := mapping.FieldValue(idx.Docs[id], field); ok {
			list := flattenAny(v)
			return formatted(list), true, nil
		}
		return nil, false, nil
	}
	if len(vals) == 0 {
		return nil, false, nil
	}
	res := formatted(vals)
	if spec.format == "" || spec.format == "use_field_mapping" {
		res = search.JavaFieldValues(idx.Docs[id], field, ftype, res, fallbackSource)
	}
	if ftype == "geo_point" && spec.format == "" {
		for i, v := range res {
			if m, ok := v.(map[string]any); ok {
				lat, _ := jsonv.ToFloat(m["lat"])
				lon, _ := jsonv.ToFloat(m["lon"])
				if fallbackSource {
					res[i] = map[string]any{"type": "Point", "coordinates": []any{lon, lat}}
				} else {
					res[i] = painless.FormatDouble(lat) + ", " + painless.FormatDouble(lon)
				}
			}
		}
	}
	return res, true, nil
}

func flattenAny(v any) []any {
	switch t := v.(type) {
	case []any:
		out := []any{}
		for _, e := range t {
			out = append(out, flattenAny(e)...)
		}
		return out
	case nil:
		return nil
	default:
		return []any{v}
	}
}

var searchBodyKeys = map[string]bool{
	"query": true, "post_filter": true, "sort": true, "from": true, "size": true,
	"_source": true, "aggs": true, "aggregations": true, "highlight": true,
	"track_scores": true, "docvalue_fields": true, "fields": true,
	"script_fields": true, "suggest": true, "search_pipeline": true,
	"timeout": true, "terminate_after": true, "min_score": true, "profile": true,
	"explain": true, "version": true, "seq_no_primary_term": true,
	"stored_fields": true, "collapse": true, "slice": true, "pit": true,
	"search_after": true, "runtime_mappings": true, "indices_boost": true,
	"stats": true, "track_total_hits": true, "rescore": true, "ext": true,
	"derived": true,
	"scroll":  true, "allow_no_indices": true, "allow_partial_search_results": true,
	"ignore_unavailable": true, "expand_wildcards": true, "batched_reduce_size": true,
	"ccs_minimize_roundtrips": true, "max_concurrent_shard_requests": true,
	"pre_filter_shard_size": true, "request_cache": true, "routing": true,
	"preference": true, "search_type": true, "typed_keys": true,
	"rest_total_hits_as_int": true, "source": true, "seq_no_primary_term ": true,
}

func validateSearchBody(body map[string]any) *apierr.Error {
	for k, v := range body {
		if !searchBodyKeys[k] {
			return &apierr.Error{Type: "parsing_exception", Status: 400,
				Reason: "Unknown key for a " + search.TokenType(v) + " in [" + k + "]."}
		}
	}
	if sg, ok := body["suggest"]; ok {
		if err := suggest.ValidateBody(sg); err != nil {
			return err
		}
	}
	if q, ok := body["query"]; ok {
		if q == nil {
			return &apierr.Error{Type: "parsing_exception", Status: 400,
				Reason: "Unknown key for a VALUE_NULL in [query]."}
		}
		if qm, ok := q.(map[string]any); ok && len(qm) == 0 {
			return &apierr.Error{Type: "illegal_argument_exception", Status: 400,
				Reason: "query malformed, empty clause found at [1:9]"}
		}
	}
	return nil
}

var searchURLParams = map[string]bool{
	"q": true, "df": true, "analyzer": true, "analyze_wildcard": true,
	"default_operator": true, "lenient": true, "quote_analyzer": true,
	"quote_field_suffix": true, "fuzzy_transpositions": true,
	"fuzzy_max_expansions": true, "fuzzy_prefix_length": true, "rewrite": true,
	"minimum_should_match": true, "boost": true, "sort": true, "size": true,
	"from": true, "_source": true, "_source_includes": true, "_source_excludes": true,
	"source": true, "source_content_type": true, "source_includes": true, "source_excludes": true,
	"include_named_queries_score": true,
	"timeout":                     true, "terminate_after": true, "track_total_hits": true,
	"search_type": true, "routing": true, "preference": true, "request_cache": true,
	"allow_no_indices": true, "ignore_unavailable": true, "expand_wildcards": true,
	"allow_partial_search_results": true, "typed_keys": true,
	"rest_total_hits_as_int": true, "search_pipeline": true, "verbose_pipeline": true,
	"scroll": true, "version": true, "seq_no_primary_term": true, "explain": true,
	"stored_fields": true, "docvalue_fields": true, "fields": true, "stats": true,
	"batched_reduce_size": true, "max_concurrent_shard_requests": true,
	"pre_filter_shard_size": true, "ccs_minimize_roundtrips": true,
	"pretty": true, "human": true, "error_trace": true, "filter_path": true,
}

func validateSearchURLParams(r *http.Request) *apierr.Error {
	for k := range r.URL.Query() {
		if !searchURLParams[k] {
			return &apierr.Error{Type: "illegal_argument_exception", Status: 400,
				Reason: "request [" + r.URL.Path + "] contains unrecognized parameter: [" + k + "]"}
		}
	}
	return nil
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request, name string) {
	if _, has := r.URL.Query()["source"]; has && r.URL.Query().Get("source_content_type") == "" {
		writeError(w, &apierr.Error{Type: "illegal_state_exception", Status: 500,
			Reason: "source and source_content_type parameters are required"})
		return
	}
	body, bodyRaw, apiErr := readJSONRaw(r)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	pitID := ""
	var pitCtx *pitContext
	if rawPit, has := body["pit"]; has && rawPit != nil {
		pm, _ := rawPit.(map[string]any)
		pitID, _ = pm["id"].(string)
		if name != "" {
			writeError(w, &apierr.Error{Type: "action_request_validation_exception", Status: 400,
				Reason: "Validation Failed: 1: [indices] cannot be used with point in time;"})
			return
		}
		if !strings.HasPrefix(pitID, "slopensearch-pit-") {
			writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400, Reason: "invalid id: [" + pitID + "]"})
			return
		}
		ctx, ok := s.getPit(pitID)
		if !ok {
			reason := "No search context found for id [" + pitID + "]"
			writeError(w, &apierr.Error{Type: "search_phase_execution_exception", Status: 404,
				Reason: reason, RootCause: &apierr.Error{Type: "search_context_missing_exception", Reason: reason}})
			return
		}
		pitCtx = ctx
	}
	if search.SortParsedFirst(bodyRaw) {
		if serr := search.CheckSortClauses(bodyRaw); serr != nil {
			writeError(w, serr)
			return
		}
	}
	if apiErr := validateSearchBody(body); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if apiErr := validateSearchURLParams(r); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	o := s.parseResolveParams(r, specOpts{closedError: true})
	var indices []*model.Index
	if pitCtx != nil {
		indices = []*model.Index{pitCtx.idx}
	} else {
		var apiErr2 *apierr.Error
		indices, apiErr2 = s.resolveSpec(name, o)
		if apiErr2 != nil {
			writeError(w, apiErr2)
			return
		}
	}

	pipe, pipeErr := s.resolveSearchPipeline(r, body, indices)
	if pipeErr != nil {
		writeError(w, pipeErr)
		return
	}
	var pipeState *searchPipeState
	var verboseResults []any
	verbosePipeline := queryBool(r, "verbose_pipeline")
	if pipe != nil {
		var perr *apierr.Error
		pipeState, verboseResults, perr = applySearchRequestProcessors(body, pipe, verbosePipeline)
		if perr != nil {
			writeError(w, perr)
			return
		}
	}

	if st := r.URL.Query().Get("search_type"); st != "" {
		switch st {
		case "query_then_fetch", "dfs_query_then_fetch":
		case "query_and_fetch", "fetch_by_query":
			writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
				Reason: "Unsupported search type [" + st + "]"})
			return
		default:
			writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
				Reason: "No search type for [" + st + "]"})
			return
		}
	}
	if rc := r.URL.Query().Get("request_cache"); rc != "" && rc != "true" && rc != "false" {
		writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
			Reason: "Failed to parse value [" + rc + "] as only [true] or [false] are allowed."})
		return
	}
	for _, spec := range []struct {
		key string
		min int
		msg string
	}{
		{"batched_reduce_size", 2, "batchedReduceSize must be >= 2"},
		{"pre_filter_shard_size", 1, "preFilterShardSize must be >= 1"},
		{"max_concurrent_shard_requests", 1, "maxConcurrentShardRequests must be >= 1"},
	} {
		if v := r.URL.Query().Get(spec.key); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < spec.min {
				writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400, Reason: spec.msg})
				return
			}
		}
	}
	for _, tv := range []string{r.URL.Query().Get("timeout")} {
		if tv == "" {
			continue
		}
		if !validTimeoutString(tv) {
			writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400, Reason: "failed to parse [" + tv + "]"})
			return
		}
	}
	if bt, ok := body["timeout"].(string); ok && bt != "" && !validTimeoutString(bt) {
		writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400, Reason: "failed to parse [" + bt + "]"})
		return
	}
	query, _ := body["query"].(map[string]any)
	if vals, has := r.URL.Query()["q"]; has {
		var qerr *apierr.Error
		query, qerr = parseQParamOpts(vals[0], r.URL.Query().Get("df"), r.URL.Query().Get("default_operator"))
		if qerr != nil {
			writeError(w, qerr)
			return
		}
	}
	if query == nil {
		query = map[string]any{"match_all": map[string]any{}}
	}
	var vidx *model.Index
	if pitCtx != nil {
		vidx = pitCtx.idx
	} else if len(indices) > 0 {
		vidx = indices[0]
	}
	if vidx != nil {
		if defs, ok := body["derived"]; ok && defs != nil {
			oldDerived, hadDerived := vidx.Mappings["derived"]
			if derr := mapping.ApplyRequestDerived(vidx, defs); derr != nil {
				writeError(w, derr)
				return
			}
			defer func() {
				if hadDerived {
					vidx.Mappings["derived"] = oldDerived
				} else {
					delete(vidx.Mappings, "derived")
				}
				mapping.RecomputeAllDocValues(vidx)
				mapping.EnsureFieldTypes(vidx)
			}()
		}
		if kerr := search.CheckKnnClauses(bodyRaw); kerr != nil {
			writeError(w, kerr)
			return
		}
		if verr := search.ValidateQuery(vidx, query); verr != nil {
			if verr.Clause != "" {
				annotateQueryClause(verr, bodyRaw)
				writeError(w, verr)
			} else if verr.ForceTop {
				writeError(w, verr)
			} else {
				writeQueryShardError(w, vidx, verr)
			}
			return
		}
	}

	if body["size"] == nil {
		if _, exists := body["size"]; exists {
			writeError(w, apierr.Parsing("Unknown key for a VALUE_NULL in [size]."))
			return
		}
	}
	if body["from"] == nil {
		if _, exists := body["from"]; exists {
			writeError(w, apierr.Parsing("Unknown key for a VALUE_NULL in [from]."))
			return
		}
	}
	from := jsonv.Int(body, "from", 0)
	size := jsonv.Int(body, "size", 10)
	if v := r.URL.Query().Get("from"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
				Reason: "Failed to parse int parameter [from] with value [" + v + "]"})
			return
		}
		from = n
	}
	if v := r.URL.Query().Get("size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
				Reason: "Failed to parse int parameter [size] with value [" + v + "]"})
			return
		}
		size = n
	}
	if from < 0 {
		writeError(w, apierr.BadRequest(fmt.Sprintf("[from] parameter cannot be negative, found [%d]", from)))
		return
	}
	if size < 0 {
		writeError(w, apierr.BadRequest(fmt.Sprintf("[size] parameter cannot be negative, found [%d]", size)))
		return
	}
	scrollParam := r.URL.Query().Get("scroll")
	scrollID := ""
	restInt := r.URL.Query().Get("rest_total_hits_as_int") == "true"
	if scrollParam != "" {
		if verr := validateScrollParams(r, body, from, size); verr != nil {
			writeError(w, verr)
			return
		}
	}
	sliceID, sliceMax := -1, 0
	if rawSlice, ok := body["slice"].(map[string]any); ok {
		if scrollParam == "" && body["pit"] == nil {
			reason := "`slice` cannot be used outside of a scroll context or PIT context"
			writeError(w, &apierr.Error{Type: "search_phase_execution_exception", Status: 500,
				Reason: reason, Phase: "query",
				RootCause: &apierr.Error{Type: "search_exception", Reason: reason}})
			return
		}
		if f, ok := jsonv.ToFloat(rawSlice["id"]); ok {
			sliceID = int(f)
		}
		if f, ok := jsonv.ToFloat(rawSlice["max"]); ok {
			sliceMax = int(f)
		}
		if sliceMax <= 1 {
			writeError(w, &apierr.Error{Type: "x_content_parse_exception", Status: 400,
				Reason:   "max must be greater than 1",
				CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: "max must be greater than 1"}})
			return
		}
		if sliceID >= sliceMax {
			writeError(w, &apierr.Error{Type: "x_content_parse_exception", Status: 400,
				Reason:   "max must be greater than id",
				CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: "max must be greater than id"}})
			return
		}
	}
	for _, idx := range indices {
		window := 10000
		if v, ok := settings.Find(idx.Settings, "max_result_window"); ok {
			if n, ok := settings.ValueInt(v); ok {
				window = n
			}
		}
		if from+size > window {
			reason := fmt.Sprintf("Result window is too large, from + size must be less than or equal to: [%d] but was [%d]. See the scroll api for a more efficient way to request large data sets. This limit can be set by changing the [index.max_result_window] index level setting.", window, from+size)
			if r.URL.Query().Get("scroll") != "" {
				reason = fmt.Sprintf("Batch size is too large, size must be less than or equal to: [%d] but was [%d].", window, size)
			}
			writeError(w, &apierr.Error{Type: "search_phase_execution_exception", Status: 400,
				Reason: reason, Phase: "query", RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason}})
			return
		}
	}

	boostMap := map[*model.Index]float64{}
	if rawBoost, ok := body["indices_boost"]; ok && rawBoost != nil {
		var entries []any
		if arr, isArr := rawBoost.([]any); isArr {
			entries = arr
		} else {
			entries = []any{rawBoost}
		}
		for _, ent := range entries {
			obj, _ := ent.(map[string]any)
			for pat, val := range obj {
				f, _ := jsonv.ToFloat(val)
				boosted, berr := s.resolveSpec(pat, o)
				if berr != nil {
					writeError(w, berr)
					return
				}
				for _, bi := range boosted {
					boostMap[bi] = f
				}
			}
		}
	}
	var matched []search.MultiHit
	shards := 0
	sf := newShardFilter(r, indices)
	dfsSearch := r.URL.Query().Get("search_type") == "dfs_query_then_fetch"
	if pitCtx != nil {
		shards = pitCtx.shards
		for _, h := range pitCtx.hits {
			qc := search.NewQueryCtxForDoc(h.Idx, h.Id, h.Doc)
			ok, score, err := qc.Eval(query)
			if err != nil {
				if isQueryShardCause(err) {
					writeQueryShardError(w, h.Idx, err)
				} else {
					writeError(w, err)
				}
				return
			}
			if ok {
				if f, has := boostMap[h.Idx]; has {
					score *= f
				}
				matched = append(matched, search.MultiHit{Idx: h.Idx, Id: h.Id, Doc: h.Doc, Score: score})
			}
		}
	} else {
		for _, idx := range indices {
			if err := readBlockError(idx); err != nil {
				writeError(w, err)
				return
			}
			q := query
			if f := searchAliasFilter(idx, name); f != nil {
				q = map[string]any{"bool": map[string]any{"must": []any{query, f}}}
			}
			if sf != nil {
				shards += len(sf.allowed[idx])
			} else {
				shards += idx.NumShards()
			}
			for _, id := range idx.Order {
				if !sf.allows(idx, id) {
					continue
				}
				doc, live := idx.Docs[id]
				if !live {
					continue
				}
				if _, pending := idx.Pending[id]; pending {
					continue
				}
				qc := search.NewQueryCtxForDoc(idx, id, doc)
				ok, score, err := qc.Eval(q)
				if err != nil {
					if isQueryShardCause(err) {
						writeQueryShardError(w, idx, err)
					} else {
						writeError(w, err)
					}
					return
				}
				if ok {
					if f, has := boostMap[idx]; has {
						score *= f
					}
					score *= searchShardMultiplier(idx, id, q, dfsSearch)
					matched = append(matched, search.MultiHit{Idx: idx, Id: id, Doc: doc, Score: score})
				}
			}
		}
	}
	if ms, ok := body["min_score"]; ok {
		if f, ok := jsonv.ToFloat(ms); ok {
			kept := matched[:0]
			for _, h := range matched {
				if h.Score >= f {
					kept = append(kept, h)
				}
			}
			matched = kept
		}
	}
	aggMatched := append([]search.MultiHit{}, matched...)
	if pf, ok := body["post_filter"].(map[string]any); ok && len(pf) > 0 {
		kept := matched[:0]
		for _, h := range matched {
			qc := search.NewQueryCtxForDoc(h.Idx, h.Id, h.Doc)
			ok, _, err := qc.Eval(pf)
			if err != nil {
				writeError(w, err)
				return
			}
			if ok {
				kept = append(kept, h)
			}
		}
		matched = kept
	}
	total := 0

	searchSortSpecs, sortErr := search.ParseSortSpecs(body["sort"], r.URL.Query().Get("sort"))
	if serr := search.CheckSortClauses(bodyRaw); serr != nil {
		sortErr = serr
	}
	if sortErr != nil {
		writeError(w, sortErr)
		return
	}
	hasScoreSort := false
	allScoreSort := len(searchSortSpecs) > 0
	for _, sp := range searchSortSpecs {
		if sp.Field == "_score" {
			hasScoreSort = true
		} else {
			allScoreSort = false
		}
	}
	sortedByField := len(searchSortSpecs) > 0 && !allScoreSort
	for _, sp := range searchSortSpecs {
		if sp.Field == "_shard_doc" && pitCtx == nil {
			writeError(w, &apierr.Error{Type: "action_request_validation_exception", Status: 400,
				Reason: "Validation Failed: 1: _shard_doc is only supported with point-in-time (PIT). Add a PIT or remove _shard_doc.;"})
			return
		}
	}
	if after, ok := body["search_after"].([]any); ok {
		if len(searchSortSpecs) == 0 {
			reason := "Sort must contain at least one field."
			writeError(w, &apierr.Error{Type: "search_phase_execution_exception", Status: 400,
				Reason: reason, Phase: "query",
				RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason}})
			return
		}
		if len(after) != len(searchSortSpecs) {
			reason := fmt.Sprintf("search_after has %d value(s) but sort has %d.", len(after), len(searchSortSpecs))
			writeError(w, &apierr.Error{Type: "search_phase_execution_exception", Status: 400,
				Reason: reason, Phase: "query",
				RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason}})
			return
		}
		if r.URL.Query().Get("scroll") != "" {
			reason := "`search_after` cannot be used in a scroll context."
			writeError(w, &apierr.Error{Type: "search_phase_execution_exception", Status: 500,
				Reason: reason, Phase: "query",
				RootCause: &apierr.Error{Type: "search_exception", Reason: reason}})
			return
		}
		if len(indices) > 0 {
			for i, sp := range searchSortSpecs {
				def, ok := mapping.FieldDef(indices[0], sp.Field)
				if !ok {
					continue
				}
				ft, _ := def["type"].(string)
				if mapping.IsNumericType(ft) && ft != "date" && ft != "date_nanos" {
					if _, conv := jsonv.ToFloat(after[i]); !conv {
						reason := "Failed to parse search_after value for field [" + sp.Field + "]."
						writeError(w, &apierr.Error{Type: "search_phase_execution_exception", Status: 400,
							Reason: reason, Phase: "query",
							RootCause: &apierr.Error{Type: "illegal_argument_exception", Reason: reason}})
						return
					}
				}
			}
		}
		if from != 0 {
			reason := "`from` parameter must be set to 0 when `search_after` is used."
			writeError(w, &apierr.Error{Type: "search_phase_execution_exception", Status: 500,
				Reason: reason, Phase: "query",
				RootCause: &apierr.Error{Type: "search_exception", Reason: reason}})
			return
		}
	}
	failedShards := 0
	partialReason := ""
	var shardFailures []any
	if sortedByField {
		for _, sp := range searchSortSpecs {
			if sp.Field == "_score" || sp.Field == "_doc" || sp.Field == "_id" || sp.Field == "_index" || sp.Field == "_script" || sp.Field == "_geo_distance" || sp.Field == "_shard_doc" {
				continue
			}
			anyMapped := false
			for _, idx := range indices {
				if kerr := search.CheckKnnSort(idx, sp.Field); kerr != nil {
					writeError(w, kerr)
					return
				}
				if search.SortFieldMapped(idx, sp.Field) {
					anyMapped = true
					break
				}
			}
			if !anyMapped && sp.Unmapped == "" {
				reason := "No mapping found for [" + sp.Field + "] in order to sort on"
				writeError(w, &apierr.Error{Type: "search_phase_execution_exception", Status: 400,
					Reason: reason, Phase: "query",
					RootCause: &apierr.Error{Type: "query_shard_exception", Reason: reason}})
				return
			}
			if !anyMapped || sp.Unmapped != "" {
				continue
			}
			for _, idx := range indices {
				if search.SortFieldMapped(idx, sp.Field) {
					continue
				}
				failedShards++
				partialReason = "No mapping found for [" + sp.Field + "] in order to sort on"
				shardFailures = append(shardFailures, map[string]any{
					"index":  idx.Name,
					"shard":  0,
					"reason": map[string]any{"type": "query_shard_exception", "reason": "No mapping found for [" + sp.Field + "] in order to sort on"},
				})
				kept := matched[:0]
				for _, h := range matched {
					if h.Idx != idx {
						kept = append(kept, h)
					}
				}
				matched = kept
			}
		}
	}
	if failedShards > 0 {
		allowPartial := true
		if v, ok := body["allow_partial_search_results"].(bool); ok {
			allowPartial = v
		}
		if vals, has := r.URL.Query()["allow_partial_search_results"]; has {
			allowPartial = vals[0] != "false"
		}
		if !allowPartial {
			writeError(w, &apierr.Error{Type: "search_phase_execution_exception", Status: 400,
				Reason: partialReason, Phase: "query",
				RootCause: &apierr.Error{Type: "query_shard_exception", Reason: partialReason}})
			return
		}
	}
	if sliceMax > 0 {
		kept := matched[:0]
		for _, h := range matched {
			n, err := strconv.Atoi(h.Id)
			if err != nil {
				n = int(search.HashString(h.Id))
			}
			if n%sliceMax == sliceID {
				kept = append(kept, h)
			}
		}
		matched = kept
	}
	terminateAfter := 0
	if n, ok := jsonv.ToFloat(body["terminate_after"]); ok {
		terminateAfter = int(n)
	}
	if v := r.URL.Query().Get("terminate_after"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			terminateAfter = n
		}
	}
	terminatedEarly := false
	if terminateAfter > 0 && len(matched) > terminateAfter {
		matched = matched[:terminateAfter]
		terminatedEarly = true
	}
	total = len(matched)
	if after, ok := body["search_after"].([]any); ok && len(after) > 0 && len(searchSortSpecs) > 0 {
		kept := matched[:0]
		for _, h := range matched {
			if search.IsAfter(h, searchSortSpecs, after) {
				kept = append(kept, h)
			}
		}
		matched = kept
	}
	if rerr := search.ApplyRescore(matched, body); rerr != nil {
		writeError(w, rerr)
		return
	}
	if len(searchSortSpecs) == 0 {
		sort.SliceStable(matched, func(i, j int) bool {
			return matched[i].Score > matched[j].Score
		})
	}
	if sortedByField {
		if err := search.SortHits(matched, searchSortSpecs, indices); err != nil {
			writeError(w, err)
			return
		}
	}
	trackScores, _ := body["track_scores"].(bool)
	collapseFields := map[string]map[string]any{}
	collapseInner := map[string]map[string]any{}
	if body["collapse"] != nil {
		var cerr *apierr.Error
		matched, collapseFields, collapseInner, cerr = search.ApplyCollapse(matched, body)
		if cerr != nil {
			writeError(w, cerr)
			return
		}
	}

	if from > len(matched) {
		from = len(matched)
	}
	end := from + size
	if end > len(matched) {
		end = len(matched)
	}
	page := matched[from:end]
	if scrollParam != "" {
		sid := s.storeScroll(matched, end, size, total, restInt)
		defer func() { _ = sid }()
		scrollID = sid
	}

	srcIncludes, srcExcludes, omitSource, bodyHasSource := search.ParseSourceFilter(body)
	qIncludes, qExcludes, qOmit, qHas := sourceFilterFromQuery(r.URL.Query())
	if qHas || qOmit {
		srcIncludes, srcExcludes, omitSource = qIncludes, qExcludes, qOmit
	}
	requestSourceExplicit := bodyHasSource || qHas || qOmit
	if _, hasSF := body["script_fields"].(map[string]any); hasSF {
		if _, hasSrc := body["_source"]; !hasSrc && r.URL.Query().Get("_source") == "" {
			omitSource = true
		}
	}
	storedRaw, hasStored := body["stored_fields"]
	storedURL := r.URL.Query().Get("stored_fields")
	if storedURL != "" {
		storedRaw, hasStored = storedURL, true
	}
	storedNone := false
	storedNames := []string{}
	if hasStored {
		items := []any{}
		switch sv := storedRaw.(type) {
		case string:
			for _, part := range strings.Split(sv, ",") {
				items = append(items, strings.TrimSpace(part))
			}
		case []any:
			items = sv
		}
		for _, item := range items {
			name, _ := jsonv.ToString(item)
			if name == "_none_" {
				storedNone = true
			} else if name != "" {
				storedNames = append(storedNames, name)
			}
		}
		if _, hasSrc := body["_source"]; !hasSrc && r.URL.Query().Get("_source") == "" {
			omitSource = true
		}
	}
	if storedNone {
		srcTrue := false
		if v, ok := body["_source"].(bool); ok && v {
			srcTrue = true
		}
		if qv := r.URL.Query().Get("_source"); qv == "true" {
			srcTrue = true
		}
		if srcTrue {
			writeError(w, &apierr.Error{Type: "search_phase_execution_exception", Status: 500, Reason: "all shards failed",
				RootCause: &apierr.Error{Type: "search_exception", Reason: "[stored_fields] cannot be disabled if [_source] is requested"}})
			return
		}
	}
	hlCfg, hlParseErr := search.ParseHighlightConfig(body, query)
	if hlParseErr != nil {
		writeError(w, hlParseErr)
		return
	}
	if list, ok := body["fields"].([]any); ok {
		for _, e := range list {
			if m, ok := e.(map[string]any); ok {
				if _, has := m["include_unmapped"]; has {
					reason := "[docvalues_field] unknown field [include_unmapped]"
					writeError(w, &apierr.Error{Type: "x_content_parse_exception", Status: 400, Reason: reason,
						RootCause: &apierr.Error{Type: "parsing_exception", Reason: reason}})
					return
				}
			}
		}
	}
	dvFields := parseFieldSpecs(body["docvalue_fields"])
	paramFields := parseFieldSpecs(body["fields"])
	if v := r.URL.Query().Get("docvalue_fields"); v != "" {
		items := []any{}
		for _, part := range strings.Split(v, ",") {
			items = append(items, strings.TrimSpace(part))
		}
		dvFields = parseFieldSpecs(items)
	}
	if v := r.URL.Query().Get("fields"); v != "" {
		items := []any{}
		for _, part := range strings.Split(v, ",") {
			items = append(items, strings.TrimSpace(part))
		}
		paramFields = parseFieldSpecs(items)
	}

	versionFlag := false
	if v, ok := body["version"].(bool); ok {
		versionFlag = v
	}
	if vals, has := r.URL.Query()["version"]; has {
		versionFlag = vals[0] == "true"
	}
	explainFlag := false
	if v, ok := body["explain"].(bool); ok {
		explainFlag = v
	}
	if r.URL.Query().Get("explain") == "true" {
		explainFlag = true
	}
	seqNoTermFlag := false
	if v, ok := body["seq_no_primary_term"].(bool); ok {
		seqNoTermFlag = v
	}
	if vals, has := r.URL.Query()["seq_no_primary_term"]; has {
		seqNoTermFlag = vals[0] == "true"
	}
	omitScores := sortedByField && !hasScoreSort && !trackScores
	maxScoreAllowed := !sortedByField || trackScores || (len(searchSortSpecs) > 0 && searchSortSpecs[0].Field == "_score" && searchSortSpecs[0].Desc)
	var maxScore any
	if maxScoreAllowed && len(indices) == 0 {
		maxScore = 0.0
	} else if maxScoreAllowed && len(page) > 0 {
		m := page[0].Score
		for _, h := range page[1:] {
			if h.Score > m {
				m = h.Score
			}
		}
		maxScore = search.ScoreJSON(m)
		if math.IsInf(m, 0) || math.IsNaN(m) {
			maxScore = nil
		}
	}
	trackTotal := true
	trackTotalVal := 10000
	trackExplicit := false
	if tv, has := body["track_total_hits"]; has {
		switch t := tv.(type) {
		case bool:
			trackTotal = t
			if t {
				trackTotalVal = -1
			}
		case float64:
			n := int(t)
			if n < 0 {
				writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
					Reason: fmt.Sprintf("value must be >= 0, got %d", n)})
				return
			}
			trackTotalVal = n
			trackExplicit = true
		case json.Number:
			n64, err := t.Int64()
			if err != nil {
				writeError(w, &apierr.Error{Type: "number_format_exception", Status: 400,
					Reason: "For input string: \"" + t.String() + "\""})
				return
			}
			n := int(n64)
			if n < 0 {
				writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
					Reason: fmt.Sprintf("value must be >= 0, got %d", n)})
				return
			}
			trackTotalVal = n
			trackExplicit = true
		case string:
			n, err := strconv.Atoi(t)
			if err != nil {
				writeError(w, &apierr.Error{Type: "number_format_exception", Status: 400,
					Reason: "For input string: \"" + t + "\""})
				return
			}
			if n < 0 {
				writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
					Reason: fmt.Sprintf("value must be >= 0, got %d", n)})
				return
			}
			trackTotalVal = n
			trackExplicit = true
		}
	}
	if vals, has := r.URL.Query()["track_total_hits"]; has {
		v := vals[0]
		if v == "false" {
			trackTotal = false
		} else if v != "true" {
			n, err := strconv.Atoi(v)
			if err != nil {
				writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
					Reason: "Failed to parse int parameter [track_total_hits] with value [" + v + "]"})
				return
			}
			if n < 0 {
				writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
					Reason: fmt.Sprintf("value must be >= 0, got %d", n)})
				return
			}
			trackTotalVal = n
			trackExplicit = true
		}
	}
	restInt = r.URL.Query().Get("rest_total_hits_as_int") == "true"
	if restInt && trackExplicit && trackTotalVal >= 0 && trackTotalVal < total {
		writeError(w, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
			Reason: fmt.Sprintf("[rest_total_hits_as_int] cannot be used if the tracking of total hits is not accurate, got %d", trackTotalVal)})
		return
	}
	totalObj := map[string]any{"value": total, "relation": "eq"}
	if trackTotalVal >= 0 && trackTotalVal < total {
		totalObj = map[string]any{"value": trackTotalVal, "relation": "gte"}
	}
	var totalOut any = totalObj
	if restInt {
		if trackExplicit {
			totalOut = totalObj["value"]
		} else {
			totalOut = total
		}
	}
	hits := make([]any, 0, len(page))
	for _, h := range page {
		hitIncludes, hitExcludes, hitOmit := srcIncludes, srcExcludes, omitSource
		if !requestSourceExplicit {
			mInc, mExc, mEnabled := mappingSourceSpec(h.Idx)
			hitIncludes, hitExcludes = mInc, mExc
			if !mEnabled {
				hitOmit = true
			}
		}
		src := search.FilterSource(h.Doc, hitIncludes, hitExcludes)
		var scoreVal any = h.Score
		if omitScores {
			scoreVal = nil
		} else {
			scoreVal = search.ScoreJSON(h.Score)
		}
		entry := map[string]any{
			"_index": h.Idx.Name, "_id": h.Id, "_score": scoreVal,
		}
		if rv, ok := h.Idx.Routing[h.Id]; ok && rv != "" {
			entry["_routing"] = rv
		}
		if versionFlag {
			entry["_version"] = h.Idx.CurrentVersion(h.Id)
		}
		if seqNoTermFlag {
			entry["_seq_no"] = h.Idx.SeqNos[h.Id]
			entry["_primary_term"] = int64(1)
		}
		if ign := h.Idx.DocIgnored[h.Id]; len(ign) > 0 {
			vals := make([]any, 0, len(ign))
			for _, f := range ign {
				vals = append(vals, f)
			}
			entry["_ignored"] = vals
		}
		if explainFlag {
			entry["_shard"] = "[" + h.Idx.Name + "][" + strconv.Itoa(h.Idx.DocShard(h.Id)) + "]"
			entry["_node"] = "slopensearch-node-1"
			if ss, ok := query["script_score"].(map[string]any); ok && len(query) == 1 {
				entry["_explanation"] = map[string]any{"value": search.ScoreJSON(h.Score),
					"description": "script score function, computed with script:\"" + scriptToString(ss["script"]) + "\"",
					"details":     []any{}}
			} else if kexp := search.KnnExplain(h.Idx, query, h.Id, h.Score); kexp != nil {
				entry["_explanation"] = kexp
			} else {
				field, term := explainLabel(query)
				qc := search.NewQueryCtxForDoc(h.Idx, h.Id, h.Doc)
				exp := qc.Bm25Explain(field, term, h.Score)
				exp["value"] = search.ScoreJSON(h.Score)
				entry["_explanation"] = exp
			}
		}
		if sortedByField && len(searchSortSpecs) > 0 && !allScoreSort {
			sortVals := make([]any, 0, len(searchSortSpecs))
			for _, sp := range searchSortSpecs {
				sortVals = append(sortVals, search.SortOutputValue(h, sp))
			}
			entry["sort"] = sortVals
		}
		if !hitOmit {
			entry["_source"] = src
		}
		if storedNone {
			delete(entry, "_id")
		}
		hitCtx := search.NewQueryCtxForDoc(h.Idx, h.Id, h.Doc)
		if hlCfg != nil && h.Idx.SourceEnabled() {
			if hlErr := hlCfg.Validate(h.Idx); hlErr != nil {
				writeError(w, hlErr)
				return
			}
			hl := search.PercolateHighlights(hitCtx, query, hlCfg)
			if len(hl) == 0 {
				hl = hlCfg.Render(h.Idx, h.Doc)
			}
			if len(hl) > 0 {
				entry["highlight"] = hl
			}
		}
		if cf, ok := collapseFields[h.Id]; ok {
			if existing, ok := entry["fields"].(map[string]any); ok {
				for k, v := range cf {
					existing[k] = v
				}
			} else {
				entry["fields"] = cf
			}
		}
		nestedInner := search.CollectNestedInnerHits(hitCtx, query, omitScores)
		if joinInner := search.CollectJoinInnerHits(hitCtx, query, omitScores); len(joinInner) > 0 {
			if nestedInner == nil {
				nestedInner = map[string]any{}
			}
			for k, v := range joinInner {
				nestedInner[k] = v
			}
		}
		if ci, ok := collapseInner[h.Id]; ok {
			if nestedInner == nil {
				nestedInner = map[string]any{}
			}
			for k, v := range ci {
				nestedInner[k] = v
			}
		}
		if len(nestedInner) > 0 {
			entry["inner_hits"] = nestedInner
		}
		if len(dvFields) > 0 || len(paramFields) > 0 || len(storedNames) > 0 {
			out := map[string]any{}
			collect := func(specs []fieldSpec, fallback bool) *apierr.Error {
				for _, spec := range specs {
					names := []string{spec.name}
					if strings.ContainsAny(spec.name, "*?") {
						names = nil
						re := search.WildcardRegex(spec.name)
						for _, f := range search.MappedLeafFields(h.Idx) {
							if re.MatchString(f) {
								names = append(names, f)
							}
						}
						sort.Strings(names)
					}
					for _, name := range names {
						vals, found, derr := docValuesOutput(h.Idx, h.Id, name, spec, fallback)
						if derr != nil {
							return derr
						}
						if !found {
							continue
						}
						if existing, done := out[name]; done && !search.IsKnnVectorField(h.Idx, name) {
							if list, ok := existing.([]any); ok {
								out[name] = append(list, vals...)
								continue
							}
						}
						out[name] = vals
					}
				}
				return nil
			}
			if derr := collect(dvFields, false); derr != nil {
				writeError(w, derr)
				return
			}
			if derr := collect(paramFields, true); derr != nil {
				writeError(w, derr)
				return
			}
			for _, name := range storedNames {
				names := []string{name}
				if strings.ContainsAny(name, "*?") {
					names = nil
					re := search.WildcardRegex(name)
					for _, f := range search.MappedLeafFields(h.Idx) {
						if re.MatchString(f) {
							names = append(names, f)
						}
					}
					sort.Strings(names)
				}
				for _, n := range names {
					def, ok := mapping.FieldDef(h.Idx, n)
					if !ok {
						continue
					}
					stored, _ := def["store"].(bool)
					if !stored {
						continue
					}
					vals, found, derr := docValuesOutput(h.Idx, h.Id, n, fieldSpec{name: n}, true)
					if derr != nil {
						writeError(w, derr)
						return
					}
					if found {
						out[n] = vals
					}
				}
			}
			if len(out) > 0 {
				entry["fields"] = out
			}
		}
		if sf, ok := body["script_fields"].(map[string]any); ok && len(sf) > 0 {
			got, serr := search.EvalScriptFields(h, sf)
			if serr != nil {
				writeError(w, serr)
				return
			}
			if len(got) == 0 {
				// nothing to add
			} else if existing, ok := entry["fields"].(map[string]any); ok {
				for k, v := range got {
					existing[k] = v
				}
			} else {
				entry["fields"] = got
			}
		}
		if slots := search.PercolateSlots(hitCtx, query); len(slots) > 0 {
			if existing, ok := entry["fields"].(map[string]any); ok {
				existing["_percolator_document_slot"] = slots
			} else {
				entry["fields"] = map[string]any{"_percolator_document_slot": slots}
			}
		}
		named := map[string]float64{}
		qcNamed := search.NewQueryCtxForDoc(h.Idx, h.Id, h.Doc)
		search.CollectNamedQueries(query, qcNamed, named)
		if pf, ok := body["post_filter"].(map[string]any); ok {
			search.CollectNamedQueries(pf, qcNamed, named)
		}
		if len(named) > 0 {
			if r.URL.Query().Get("include_named_queries_score") == "true" {
				scored := map[string]any{}
				for n, sc := range named {
					scored[n] = search.ScoreJSON(sc)
				}
				entry["matched_queries"] = scored
			} else {
				names := make([]string, 0, len(named))
				for n := range named {
					names = append(names, n)
				}
				entry["matched_queries"] = names
			}
		}
		hits = append(hits, entry)
	}

	resp := map[string]any{
		"took":      1,
		"timed_out": false,
		"_shards":   shardStatusObject(shards, failedShards, shardFailures),
		"hits": map[string]any{
			"total":     totalOut,
			"max_score": maxScore,
			"hits":      hits,
		},
	}

	if suggest.IsSuggestOnly(body) {
		resp["hits"] = map[string]any{"total": map[string]any{"value": 0, "relation": "eq"}, "max_score": nil, "hits": []any{}}
	}
	var aggIdx *model.Index
	if len(indices) > 0 {
		aggIdx = indices[0]
	}
	if aggIdx != nil {
		for _, key := range []string{"aggregations", "aggs"} {
			if aggSpec, ok := body[key].(map[string]any); ok {
				terms := queryHighlightTerms(body["query"])
				aggHits := make([]aggs.Hit, 0, len(aggMatched))
				for _, h := range aggMatched {
					aggHits = append(aggHits, aggs.Hit{Idx: h.Idx, ID: h.Id, Doc: h.Doc, Score: h.Score, Terms: terms})
				}
				typed := r.URL.Query().Get("typed_keys") != "" && r.URL.Query().Get("typed_keys") != "false"
				out, err := aggs.Evaluate(aggSpec, aggHits, aggIdx, "", typed)
				if err != nil {
					writeError(w, err)
					return
				}
				resp["aggregations"] = out
				break
			}
		}
		if suggestSpec, ok := body["suggest"].(map[string]any); ok {
			typed := r.URL.Query().Get("typed_keys") != "" && r.URL.Query().Get("typed_keys") != "false"
			sres, serr := suggest.Evaluate(suggestSpec, indices, typed, body)
			if serr != nil {
				writeError(w, serr)
				return
			}
			if len(sres) > 0 {
				resp["suggest"] = sres
			}
		}
	}

	if terminateAfter > 0 {
		resp["terminated_early"] = terminatedEarly
	}
	profileFlag := false
	if v, ok := body["profile"].(bool); ok {
		profileFlag = v
	}
	if profileFlag && len(indices) > 0 {
		resp["profile"] = search.BuildProfile(query, body, indices[0].Name)
	}
	if !trackTotal {
		if hm, ok := resp["hits"].(map[string]any); ok {
			if restInt {
				hm["total"] = -1
			} else {
				delete(hm, "total")
			}
		}
	}
	if pipe != nil {
		if perr := applySearchResponseProcessors(resp, pipe, pipeState); perr != nil {
			writeError(w, perr)
			return
		}
		if verbosePipeline {
			resp["processor_results"] = verboseResults
		}
	}
	if scrollID != "" {
		resp["_scroll_id"] = scrollID
	}
	if pitID != "" {
		ordered := ojson.Object{{K: "pit_id", V: pitID}}
		for k, v := range resp {
			ordered = append(ordered, ojson.KV{K: k, V: v})
		}
		writeJSON(w, 200, ordered)
		return
	}

	if terminateAfter > 0 {
		ordered := ojson.Object{{K: "took", V: resp["took"]}, {K: "timed_out", V: resp["timed_out"]}, {K: "terminated_early", V: terminatedEarly}}
		for k, v := range resp {
			if k == "took" || k == "timed_out" || k == "terminated_early" {
				continue
			}
			ordered = append(ordered, ojson.KV{K: k, V: v})
		}
		writeJSON(w, 200, ordered)
		return
	}
	if _, ok := resp["suggest"]; ok {
		writeJSON(w, 200, orderSearchResponse(resp))
		return
	}
	writeJSON(w, 200, resp)
}

func validTimeoutString(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return false
	}
	switch s[i:] {
	case "ms", "s", "m", "h", "d":
		return true
	}
	return false
}

func isQueryShardCause(e *apierr.Error) bool {
	return e.Type == "mapper_parsing_exception" || e.Type == "query_shard_exception" || e.SearchFailure != ""
}

func jsonKeyValuePos(body []byte, key string) (int, int) {
	needle := []byte(`"` + key + `"`)
	idx := bytes.Index(body, needle)
	if idx < 0 {
		return 1, 0
	}
	i := idx + len(needle)
	for i < len(body) && (body[i] == ' ' || body[i] == '\t' || body[i] == '\r' || body[i] == '\n') {
		i++
	}
	if i < len(body) && body[i] == ':' {
		i++
	}
	for i < len(body) && (body[i] == ' ' || body[i] == '\t' || body[i] == '\r' || body[i] == '\n') {
		i++
	}
	line, col := 1, 1
	for j := 0; j < i; j++ {
		if body[j] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

func annotateQueryClause(err *apierr.Error, raw []byte) {
	if err.Clause == "" {
		return
	}
	line, col := 1, 0
	if len(raw) > 0 {
		if err.ClauseEnd {
			line, col = jsonClauseEndPos(raw, err.Clause)
		} else {
			line, col = jsonKeyValuePos(raw, err.Clause)
		}
	}
	err.Fields = map[string]any{"line": line, "col": col}
	if !err.NoCausedBy {
		err.CausedBy = &apierr.Error{Type: "named_object_not_found_exception",
			Reason: fmt.Sprintf("[%d:%d] unknown field [%s]", line, col, err.Clause)}
	}
}

func jsonClauseEndPos(body []byte, key string) (int, int) {
	needle := []byte(`"` + key + `"`)
	idx := bytes.Index(body, needle)
	if idx < 0 {
		return 1, 0
	}
	i := idx + len(needle)
	for i < len(body) && body[i] != ':' {
		i++
	}
	if i < len(body) {
		i++
	}
	for i < len(body) && (body[i] == ' ' || body[i] == '\t' || body[i] == '\r' || body[i] == '\n') {
		i++
	}
	if i >= len(body) || body[i] != '{' {
		return jsonKeyValuePos(body, key)
	}
	depth := 0
	inStr := false
	esc := false
	for ; i < len(body); i++ {
		ch := body[i]
		if inStr {
			if esc {
				esc = false
			} else if ch == '\\' {
				esc = true
			} else if ch == '"' {
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				line, col := 1, 1
				for j := 0; j < i; j++ {
					if body[j] == '\n' {
						line++
						col = 1
					} else {
						col++
					}
				}
				return line, col
			}
		}
	}
	return jsonKeyValuePos(body, key)
}

func writeQueryShardError(w http.ResponseWriter, idx *model.Index, cause *apierr.Error) {
	if cause.Shard != nil {
		writeJSON(w, cause.Shard.StatusCode(), cause.Shard.ResponseBody())
		return
	}
	if cause.SearchFailure != "" {
		writeCustomSearchFailure(w, idx, cause)
		return
	}
	innerType, innerReason := cause.Type, cause.Reason
	if cause.CausedBy != nil {
		innerType, innerReason = cause.CausedBy.Type, cause.CausedBy.Reason
	}
	if innerType == "illegal_argument_exception" && strings.HasPrefix(innerReason, "Failed to parse value [") &&
		strings.Contains(innerReason, "as only [true] or [false] are allowed.") {
		if i := strings.Index(innerReason, "["); i >= 0 {
			if j := strings.Index(innerReason[i+1:], "]"); j >= 0 {
				innerReason = "Can't parse boolean value [" + innerReason[i+1:i+1+j] + "], expected [true] or [false]"
			}
		}
	}
	rootType := "query_shard_exception"
	rootReason := "failed to create query: " + innerReason
	if cause.Type == "query_shard_exception" && cause.CausedBy == nil {
		rootReason = innerReason
	}
	shardType := "query_shard_exception"
	shardReasonText := rootReason
	shardCaused := ojson.Object{{"type", innerType}, {"reason", innerReason}}
	rootExtra := ojson.Object{{"index", idx.Name}, {"index_uuid", idx.UUID}}
	shardExtra := ojson.Object{{"index", idx.Name}, {"index_uuid", idx.UUID}}
	var topCaused ojson.Object
	if cause.Type == "parse_exception" {
		parseReason := cause.Reason
		parseInner := cause.Reason
		if cause.CausedBy != nil {
			parseInner = cause.CausedBy.Reason
		} else if !strings.Contains(parseReason, ": [") {
			parseReason = parseReason + ": [" + parseReason + "]"
		}
		innerCaused := ojson.Object{
			{"type", "illegal_argument_exception"},
			{"reason", parseInner},
			{"caused_by", ojson.Object{
				{"type", "date_time_parse_exception"},
				{"reason", "Failed to parse with all enclosed parsers"},
			}},
		}
		parseCaused := ojson.Object{
			{"type", "parse_exception"},
			{"reason", parseReason},
			{"caused_by", innerCaused},
		}
		rootType = "parse_exception"
		rootReason = parseReason
		rootExtra = nil
		topCaused = parseCaused
		if cause.CausedBy != nil {
			// Term-level date parsing fails while creating the shard query, so
			// the parse_exception is wrapped in a query_shard_exception.
			shardReasonText = "failed to create query: " + parseReason
			shardCaused = parseCaused
		} else {
			// Range date parsing fails at query parse time and is reported
			// directly as a parse_exception shard failure.
			shardType = "parse_exception"
			shardReasonText = parseReason
			shardCaused = innerCaused
			shardExtra = nil
		}
	}
	rootCauseObj := ojson.Object{{"type", rootType}, {"reason", rootReason}}
	rootCauseObj = append(rootCauseObj, rootExtra...)
	shardReasonObj := ojson.Object{{"type", shardType}, {"reason", shardReasonText}}
	shardReasonObj = append(shardReasonObj, shardExtra...)
	shardReasonObj = append(shardReasonObj, ojson.KV{"caused_by", shardCaused})
	if topCaused == nil {
		topCaused = shardReasonObj
	}
	errObj := ojson.Object{
		{"root_cause", []any{rootCauseObj}},
		{"type", "search_phase_execution_exception"},
		{"reason", "all shards failed"},
		{"phase", "query"},
		{"grouped", true},
		{"failed_shards", []any{ojson.Object{
			{"shard", 0},
			{"index", idx.Name},
			{"node", model.NodeID},
			{"reason", shardReasonObj},
		}}},
		{"caused_by", topCaused},
	}
	writeJSON(w, 400, ojson.Object{{"error", errObj}, {"status", 400}})
}

func writeCustomSearchFailure(w http.ResponseWriter, idx *model.Index, e *apierr.Error) {
	reason := e.Reason
	status := 400
	rootType := "exception"
	shardType := "exception"
	shardReason := reason
	var topCaused ojson.Object
	switch e.SearchFailure {
	case "fvf_unmapped":
		rootType = "exception"
		shardType = "query_shard_exception"
		shardReason = "failed to create query: " + reason
		topCaused = ojson.Object{{"type", "exception"}, {"reason", reason}}
	case "fvf_log":
		rootType = "illegal_argument_exception"
		shardType = "illegal_argument_exception"
		topCaused = ojson.Object{{"type", "illegal_argument_exception"}, {"reason", reason},
			{"caused_by", ojson.Object{{"type", "illegal_argument_exception"}, {"reason", reason}}}}
	case "fvf_missing":
		status = 500
		topCaused = ojson.Object{{"type", "exception"}, {"reason", reason}}
	default:
		topCaused = ojson.Object{{"type", e.Type}, {"reason", reason}}
	}
	failedReason := ojson.Object{{"type", shardType}, {"reason", shardReason}}
	if e.SearchFailure == "fvf_unmapped" {
		failedReason = append(failedReason,
			ojson.KV{"index", idx.Name}, ojson.KV{"index_uuid", idx.UUID},
			ojson.KV{"caused_by", ojson.Object{{"type", "exception"}, {"reason", reason}}})
	}
	errObj := ojson.Object{
		{"root_cause", []any{ojson.Object{{"type", rootType}, {"reason", reason}}}},
		{"type", "search_phase_execution_exception"},
		{"reason", "all shards failed"},
		{"phase", "query"},
		{"grouped", true},
		{"failed_shards", []any{ojson.Object{
			{"shard", 0},
			{"index", idx.Name},
			{"node", model.NodeID},
			{"reason", failedReason},
		}}},
		{"caused_by", topCaused},
	}
	writeJSON(w, status, ojson.Object{{"error", errObj}, {"status", status}})
}

func queryHighlightTerms(q any) []string {
	out := []string{}
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			for _, w := range strings.Fields(t) {
				out = append(out, w)
			}
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(q)
	return out
}

func shardStatusObject(total, failed int, failures []any) map[string]any {
	out := map[string]any{"total": total, "successful": total - failed, "skipped": 0, "failed": failed}
	if len(failures) > 0 {
		out["failures"] = failures
	}
	return out
}

// orderSearchResponse renders the top-level search response keys in the
// order OpenSearch writes them.
func orderSearchResponse(resp map[string]any) ojson.Object {
	first := []string{"_scroll_id", "took", "timed_out", "terminated_early", "_shards", "hits",
		"aggregations", "suggest", "profile"}
	out := ojson.Object{}
	seen := map[string]bool{}
	for _, k := range first {
		if v, ok := resp[k]; ok {
			out = append(out, ojson.KV{k, v})
			seen[k] = true
		}
	}
	for _, k := range jsonv.SortedKeys(resp) {
		if !seen[k] {
			out = append(out, ojson.KV{k, resp[k]})
		}
	}
	return out
}
