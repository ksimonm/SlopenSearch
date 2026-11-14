package main

import (
	"bytes"
	crand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

type orderedKV struct {
	k string
	v any
}

type orderedJSON []orderedKV

type verbatimJSON []byte

func (v verbatimJSON) MarshalJSON() ([]byte, error) { return v, nil }

func (o orderedJSON) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, kv := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(jsonMustMarshal(kv.k))
		b.WriteByte(':')
		switch t := kv.v.(type) {
		case verbatimJSON:
			b.Write(t)
		case json.RawMessage:
			b.Write(t)
		default:
			var vb bytes.Buffer
			enc := json.NewEncoder(&vb)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(kv.v); err != nil {
				return nil, err
			}
			b.Write(bytes.TrimRight(vb.Bytes(), "\n"))
		}
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func ensureDocState(idx *Index) {
	if idx.Versions == nil {
		idx.Versions = map[string]int64{}
	}
	if idx.SeqNos == nil {
		idx.SeqNos = map[string]int64{}
	}
	if idx.PrimaryTerms == nil {
		idx.PrimaryTerms = map[string]int64{}
	}
	if idx.Pending == nil {
		idx.Pending = map[string]map[string]any{}
	}
	if idx.RawDocs == nil {
		idx.RawDocs = map[string][]byte{}
	}
	if idx.Routing == nil {
		idx.Routing = map[string]string{}
	}
}

func (s *Server) resolveWriteTargetV2(name string) (*Index, *apiError) {
	idx, err := s.resolveWriteTarget(name)
	if err == nil {
		return idx, nil
	}
	if err.Type != "index_not_found_exception" {
		return nil, err
	}
	if verr := validateIndexName(name); verr != nil {
		return nil, verr
	}
	return s.createIndexInternal(name, map[string]any{}, false)
}

func unknownParams(r *http.Request, allowed map[string]bool) *apiError {
	var bad []string
	for k := range r.URL.Query() {
		if !allowed[k] {
			bad = append(bad, k)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return &apiError{Type: "illegal_argument_exception", Status: 400,
		Reason: unrecognizedParamsReason(r.URL.Path, bad, allowed)}
}

func refreshValue(r *http.Request) (string, *apiError) {
	if _, ok := r.URL.Query()["refresh"]; !ok {
		return "", nil
	}
	v := r.URL.Query().Get("refresh")
	switch v {
	case "":
		return "true", nil
	case "true", "false", "wait_for":
		return v, nil
	default:
		return "", &apiError{Type: "illegal_argument_exception", Status: 400,
			Reason: "Unknown value for refresh: [" + v + "]."}
	}
}

func readDocBody(r *http.Request) (map[string]any, []byte, *apiError) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, nil, errBad("failed to read body")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, raw, &apiError{Type: "parse_exception", Status: 400, Reason: "request body is required"}
	}
	if trimmed[0] != '{' {
		return nil, raw, &apiError{Type: "mapper_parsing_exception", Status: 400,
			Reason:   "Failed to parse request body",
			CausedBy: &apiError{Type: "not_x_content_exception", Reason: "Compressor detection can only be called on some xcontent bytes or compressed xcontent bytes"}}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		ct := "x_content_parse_exception"
		if strings.Contains(err.Error(), "unexpected EOF") || strings.Contains(err.Error(), "unexpected end") {
			ct = "unexpected_end_of_input_exception"
		}
		return nil, raw, &apiError{Type: "mapper_parsing_exception", Status: 400,
			Reason:   "Failed to parse request body",
			CausedBy: &apiError{Type: ct, Reason: err.Error()}}
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, raw, &apiError{Type: "mapper_parsing_exception", Status: 400,
			Reason:   "Failed to parse request body",
			CausedBy: &apiError{Type: "illegal_argument_exception", Reason: "trailing data"}}
	}
	if e := invalidUTF8Error(raw); e != nil {
		return nil, raw, e
	}
	return out, raw, nil
}

func nextSeq(idx *Index) int64 {
	ensureDocState(idx)
	n := idx.NextSeq
	idx.NextSeq++
	return n
}

func currentVersion(idx *Index, id string) int64 {
	ensureDocState(idx)
	if v, ok := idx.Versions[id]; ok {
		return v
	}
	if _, ok := idx.Docs[id]; ok {
		return 1
	}
	return 0
}

func mappingRoutingRequired(idx *Index) bool {
	m, _ := idx.Mappings["_routing"].(map[string]any)
	if m == nil {
		return false
	}
	b, _ := m["required"].(bool)
	return b
}

func mappingSourceEnabled(idx *Index) bool {
	m, _ := idx.Mappings["_source"].(map[string]any)
	if m == nil {
		return true
	}
	if b, ok := m["enabled"].(bool); ok {
		return b
	}
	return true
}

func routingMissing(idx *Index, id string) *apiError {
	return &apiError{Type: "routing_missing_exception", Status: 400,
		Reason: "routing is required for [" + idx.Name + "]/[" + id + "]"}
}

func shardOf(v string, n int) int {
	if n <= 1 {
		return 0
	}
	h := fnv.New32a()
	h.Write([]byte(v))
	return int(h.Sum32() % uint32(n))
}

func routingMatches(idx *Index, id, routing string) bool {
	if idx.numShards() <= 1 {
		return true
	}
	assigned := idx.Routing[id]
	if assigned == "" {
		assigned = id
	}
	probe := routing
	if probe == "" {
		probe = id
	}
	return shardOf(probe, idx.numShards()) == shardOf(assigned, idx.numShards())
}

func requestRouting(r *http.Request) string {
	return r.URL.Query().Get("routing")
}

func parseVersionParams(r *http.Request) (int64, string, bool, *apiError) {
	q := r.URL.Query()
	vt := q.Get("version_type")
	if _, ok := q["version"]; !ok {
		return 0, vt, false, nil
	}
	v, err := strconv.ParseInt(q.Get("version"), 10, 64)
	if err != nil {
		return 0, vt, true, &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: illegal version value [" + q.Get("version") + "];"}
	}
	if vt == "" {
		vt = "internal"
	}
	return v, vt, true, nil
}

func versionConflict(idx *Index, id, reason string) *apiError {
	return &apiError{Type: "version_conflict_engine_exception", Status: 409, Reason: reason,
		Fields: map[string]any{"index": idx.Name, "shard": "0"}}
}

func plannedVersion(idx *Index, id string, r *http.Request) (int64, *apiError) {
	old := currentVersion(idx, id)
	_, exists := idx.Docs[id]
	v, vt, has, err := parseVersionParams(r)
	if err != nil {
		return 0, err
	}
	if !has {
		if vt == "external" || vt == "external_gte" {
			return 0, &apiError{Type: "action_request_validation_exception", Status: 400,
				Reason: "Validation Failed: 1: illegal version value [null];"}
		}
		if vt != "" && vt != "internal" {
			return 0, &apiError{Type: "illegal_argument_exception", Status: 400, Reason: "No version type match [" + vt + "]"}
		}
		return old + 1, nil
	}
	switch vt {
	case "internal":
		return 0, &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: internal versioning can not be used for optimistic concurrency control. Please use `if_seq_no` and `if_primary_term` instead;"}
	case "external":
		if v < 0 {
			return 0, &apiError{Type: "action_request_validation_exception", Status: 400,
				Reason: "Validation Failed: 1: illegal version value [" + strconv.FormatInt(v, 10) + "];"}
		}
		if exists && old >= v {
			return 0, versionConflict(idx, id, fmt.Sprintf("[%s]: version conflict, current version [%d] is higher or equal to the one provided [%d]", id, old, v))
		}
		return v, nil
	case "external_gte":
		if exists && old > v {
			return 0, versionConflict(idx, id, fmt.Sprintf("[%s]: version conflict, current version [%d] is higher than the one provided [%d]", id, old, v))
		}
		return v, nil
	default:
		return 0, &apiError{Type: "illegal_argument_exception", Status: 400, Reason: "No version type match [" + vt + "]"}
	}
}

func checkIfSeq(r *http.Request, idx *Index, id string) *apiError {
	q := r.URL.Query()
	seqRaw, hasSeq := q["if_seq_no"]
	termRaw, hasTerm := q["if_primary_term"]
	if !hasSeq && !hasTerm {
		return nil
	}
	if hasSeq && !hasTerm {
		return &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: ifSeqNo is set, but primary term is [0];"}
	}
	if !hasSeq && hasTerm {
		return &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: ifPrimaryTerm is set, but seq no is [-2];"}
	}
	seq, _ := strconv.ParseInt(seqRaw[0], 10, 64)
	term, _ := strconv.ParseInt(termRaw[0], 10, 64)
	curSeq, ok := idx.SeqNos[id]
	if !ok {
		curSeq = -2
	}
	curTerm := idx.PrimaryTerms[id]
	if curTerm == 0 {
		curTerm = 1
	}
	if !ok || curSeq != seq || curTerm != term {
		return versionConflict(idx, id, fmt.Sprintf("[%s]: version conflict, required seqNo [%d], primary term [%d]. current document has seqNo [%d] and primary term [%d]", id, seq, term, curSeq, curTerm))
	}
	return nil
}

func waitForActiveShards(r *http.Request, idx *Index) *apiError {
	v := r.URL.Query().Get("wait_for_active_shards")
	if v == "" || v == "1" || v == "all" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err == nil && n > idx.numShards() {
		return &apiError{Type: "unavailable_shards_exception", Status: 503,
			Reason: fmt.Sprintf("Timeout waiting for [%d] shards to become active, only [%d] are active", n, idx.numShards())}
	}
	return nil
}

var indexDocParams = map[string]bool{
	"op_type": true, "version": true, "version_type": true, "if_seq_no": true,
	"if_primary_term": true, "routing": true, "refresh": true, "timeout": true,
	"wait_for_active_shards": true, "pipeline": true, "require_alias": true,
	"error_trace": true, "human": true, "pretty": true, "filter_path": true,
	"source": true, "source_content_type": true,
}

var getDocParams = map[string]bool{
	"realtime": true, "version": true, "version_type": true, "routing": true,
	"preference": true, "_source": true, "_source_includes": true, "_source_excludes": true,
	"stored_fields": true, "refresh": true, "error_trace": true, "human": true,
	"pretty": true, "filter_path": true, "format": true,
}

var deleteDocParams = map[string]bool{
	"version": true, "version_type": true, "if_seq_no": true, "if_primary_term": true,
	"routing": true, "refresh": true, "timeout": true, "wait_for_active_shards": true,
	"error_trace": true, "human": true, "pretty": true, "filter_path": true,
}

var updateDocParams = map[string]bool{
	"if_seq_no": true, "if_primary_term": true, "routing": true, "refresh": true,
	"retry_on_conflict": true, "timeout": true, "wait_for_active_shards": true,
	"version": true, "version_type": true, "_source": true, "_source_includes": true,
	"_source_excludes": true, "error_trace": true, "human": true, "pretty": true,
	"filter_path": true,
}

func storedFieldSet(idx *Index) map[string]bool {
	out := map[string]bool{}
	props, _ := idx.Mappings["properties"].(map[string]any)
	var walk func(prefix string, m map[string]any)
	walk = func(prefix string, m map[string]any) {
		for name, raw := range m {
			d, _ := raw.(map[string]any)
			if d == nil {
				continue
			}
			path := name
			if prefix != "" {
				path = prefix + "." + name
			}
			if b, _ := d["store"].(bool); b {
				out[path] = true
			}
			if sub, ok := d["properties"].(map[string]any); ok {
				walk(path, sub)
			}
		}
	}
	walk("", props)
	return out
}

func storedSpec(raw string, idx *Index) []string {
	if raw == "" {
		return nil
	}
	if raw == "*" {
		var all []string
		for p := range storedFieldSet(idx) {
			all = append(all, p)
		}
		sort.Strings(all)
		return all
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func storedFieldsFor(idx *Index, doc map[string]any, spec []string) map[string]any {
	if len(spec) == 0 {
		return nil
	}
	stored := storedFieldSet(idx)
	out := map[string]any{}
	for _, p := range spec {
		if p == "*" {
			for f := range stored {
				addStoredField(out, doc, f)
			}
			continue
		}
		if !stored[p] {
			continue
		}
		addStoredField(out, doc, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func addStoredField(out map[string]any, doc map[string]any, path string) {
	v, ok := fieldValue(doc, path)
	if !ok {
		return
	}
	vals := []any{}
	switch t := v.(type) {
	case []any:
		vals = append(vals, t...)
	default:
		vals = append(vals, t)
	}
	out[path] = vals
}

func sourceSpecFromValue(v any) ([]string, []string, bool) {
	switch t := v.(type) {
	case bool:
		if !t {
			return nil, nil, true
		}
		return nil, nil, false
	case string:
		if t == "false" {
			return nil, nil, true
		}
		if t == "true" || t == "" {
			return nil, nil, false
		}
		return splitSourceList(t), nil, false
	case []any:
		var inc []string
		for _, e := range t {
			if s, ok := e.(string); ok {
				inc = append(inc, s)
			}
		}
		return inc, nil, false
	case map[string]any:
		var inc, exc []string
		if raw, ok := t["includes"]; ok {
			inc, _, _ = sourceSpecFromValue(raw)
		}
		if raw, ok := t["excludes"]; ok {
			exc, _, _ = sourceSpecFromValue(raw)
		}
		return inc, exc, false
	}
	return nil, nil, false
}

func checkObjectDepth(v any, depth, limit int) *apiError {
	if depth > limit {
		reason := "The depth of the field has exceeded the allowed limit of [" + strconv.Itoa(limit) + "]. This limit can be set by changing the [index.mapping.depth.limit] index level setting."
		return &apiError{Type: "mapper_parsing_exception", Status: 400,
			Reason:    "failed to parse",
			RootCause: &apiError{Type: "parse_exception", Reason: reason},
			CausedBy:  &apiError{Type: "parse_exception", Reason: reason}}
	}
	switch t := v.(type) {
	case map[string]any:
		for _, e := range t {
			if err := checkObjectDepth(e, depth+1, limit); err != nil {
				return err
			}
		}
	case []any:
		for _, e := range t {
			if err := checkObjectDepth(e, depth, limit); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkArrayDepth enforces index.mapping.depth.limit on arrays nested in
// arrays (DocumentParser's field array depth).
func checkArrayDepth(v any, depth, limit int) *apiError {
	switch t := v.(type) {
	case map[string]any:
		for _, e := range t {
			if err := checkArrayDepth(e, depth, limit); err != nil {
				return err
			}
		}
	case []any:
		if depth+1 > limit {
			reason := "The depth of the nested array field has exceeded the allowed limit of [" + strconv.Itoa(limit) + "]. This limit can be set by changing the [index.mapping.depth.limit] index level setting."
			return &apiError{Type: "mapper_parsing_exception", Status: 400,
				Reason:    "failed to parse",
				RootCause: &apiError{Type: "parse_exception", Reason: reason},
				CausedBy:  &apiError{Type: "parse_exception", Reason: reason}}
		}
		for _, e := range t {
			if err := checkArrayDepth(e, depth+1, limit); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkMixedArray(idx *Index, v any, path string) *apiError {
	if enabled, ok := idx.Mappings["enabled"].(bool); ok && !enabled {
		return nil
	}
	switch t := v.(type) {
	case map[string]any:
		for k, e := range t {
			p := k
			if path != "" {
				p = path + "." + k
			}
			if def, ok := mappingFieldDef(idx, p); ok {
				if b, ok := def["enabled"].(bool); ok && !b {
					continue
				}
			}
			if err := checkMixedArray(idx, e, p); err != nil {
				return err
			}
		}
	case []any:
		hasObj, hasScalar := false, false
		for _, e := range t {
			switch e.(type) {
			case map[string]any:
				hasObj = true
			case nil:
			default:
				hasScalar = true
			}
			if err := checkMixedArray(idx, e, path); err != nil {
				return err
			}
		}
		if hasObj && hasScalar {
			return &apiError{Type: "illegal_argument_exception", Status: 400,
				Reason: "can't merge a non object mapping [" + path + "] with an object mapping"}
		}
	}
	return nil
}

type rawFloat float64

func (f rawFloat) MarshalJSON() ([]byte, error) {
	s := strconv.FormatFloat(float64(f), 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return []byte(s), nil
}

func normalizeNumbers(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, e := range t {
			t[k] = normalizeNumbers(e)
		}
		return t
	case []any:
		for i, e := range t {
			t[i] = normalizeNumbers(e)
		}
		return t
	case json.Number:
		if strings.ContainsAny(t.String(), ".eE") {
			f, _ := t.Float64()
			return rawFloat(f)
		}
		return t
	}
	return v
}

func parseOrderedValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			out := orderedJSON{}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, _ := kt.(string)
				v, err := parseOrderedValue(dec)
				if err != nil {
					return nil, err
				}
				out = append(out, orderedKV{key, v})
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return out, nil
		case '[':
			out := []any{}
			for dec.More() {
				v, err := parseOrderedValue(dec)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return out, nil
		}
	case json.Number:
		if strings.ContainsAny(t.String(), ".eE") {
			f, _ := t.Float64()
			return rawFloat(f), nil
		}
		return t, nil
	case string, bool, nil:
		return t, nil
	}
	return nil, nil
}

func filterRawSource(raw []byte, includes, excludes []string) ([]byte, bool) {
	for _, p := range append(append([]string{}, includes...), excludes...) {
		if strings.Contains(p, ".") || strings.ContainsAny(p, "*?") {
			return nil, false
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	v, err := parseOrderedValue(dec)
	if err != nil {
		return nil, false
	}
	obj, ok := v.(orderedJSON)
	if !ok {
		return nil, false
	}
	var out orderedJSON
	if len(includes) == 0 {
		out = obj
	} else {
		want := map[string]bool{}
		for _, p := range includes {
			want[p] = true
		}
		for _, kv := range obj {
			if want[kv.k] {
				out = append(out, kv)
			}
		}
	}
	if len(excludes) > 0 {
		drop := map[string]bool{}
		for _, p := range excludes {
			drop[p] = true
		}
		kept := out[:0]
		for _, kv := range out {
			if !drop[kv.k] {
				kept = append(kept, kv)
			}
		}
		out = kept
	}
	b, err := out.MarshalJSON()
	if err != nil {
		return nil, false
	}
	return b, true
}

func mappingShapeConflict(props map[string]any, doc map[string]any) *apiError {
	for k, v := range doc {
		def, ok := props[k].(map[string]any)
		if !ok {
			continue
		}
		if _, isObj := def["properties"].(map[string]any); !isObj {
			continue
		}
		switch t := v.(type) {
		case map[string]any:
			if err := mappingShapeConflict(def["properties"].(map[string]any), t); err != nil {
				return err
			}
		case []any:
			for _, e := range t {
				if m, ok := e.(map[string]any); ok {
					if err := mappingShapeConflict(def["properties"].(map[string]any), m); err != nil {
						return err
					}
				}
			}
		case nil:
		default:
			return &apiError{Type: "mapper_parsing_exception", Status: 400,
				Reason: "object mapping for [" + k + "] tried to parse field [" + k + "] as object, but found a concrete value"}
		}
	}
	return nil
}

var percolatorReservedKeys = map[string]bool{
	"boost": true, "_name": true, "case_insensitive": true, "rewrite": true, "flags": true,
	"max_expansions": true, "fuzziness": true, "prefix_length": true, "transpositions": true,
	"operator": true, "analyzer": true, "minimum_should_match": true, "zero_terms_query": true,
	"cutoff_frequency": true, "lenient": true, "fuzzy_transpositions": true,
	"auto_generate_synonyms_phrase_query": true, "type": true, "id": true, "path": true,
	"indexed_shape": true, "shape": true, "relation": true, "ignore_unmapped": true,
	"validation_method": true, "ignore_malformed": true, "value": true, "values": true,
	"query": true, "fields": true, "default_field": true, "field": true, "document": true,
	"documents": true, "index": true, "name": true, "slop": true, "in_order": true,
	"score_mode": true, "inner_hits": true, "sort": true, "size": true, "from": true,
	"_source": true, "time_zone": true, "format": true, "locale": true,
	"minimum_should_match_script": true, "boost_mode": true, "functions": true,
	"random_score": true, "weight": true, "field_value_factor": true, "script_score": true,
	"gauss": true, "exp": true, "linear": true, "origin": true, "scale": true, "offset": true,
	"decay": true, "missing": true, "factor": true, "modifier": true, "seed": true,
	"multi": true, "minimum_number_should_match": true, "tie_breaker": true, "disable_coord": true,
	"positive": true, "negative": true, "negative_boost": true, "adjust_pure_negative": true,
	"should": true, "must": true, "must_not": true, "filter": true, "queries": true,
	"clauses": true, "include": true, "exclude": true, "little": true, "big": true,
	"big_field": true, "little_field": true, "positive_boost": true, "negative_boost_boost": true,
	"distance": true, "distance_type": true, "unit": true, "validation_methods": true,
	"top_left": true, "bottom_right": true, "points": true, "ignore_z_value": true,
	"orientation": true, "boost_mode_boost": true, "span_term": true, "script": true,
	"params": true, "source": true, "lang": true, "organic": true, "docs": true,
}

func percolatorFieldRefs(q map[string]any, out *[]string) {
	for clause, raw := range q {
		bm, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch clause {
		case "bool":
			for _, key := range []string{"must", "should", "filter", "must_not"} {
				for _, item := range asQueryList(bm[key]) {
					if child, ok := item.(map[string]any); ok {
						percolatorFieldRefs(child, out)
					}
				}
			}
		case "constant_score":
			if child, ok := bm["filter"].(map[string]any); ok {
				percolatorFieldRefs(child, out)
			}
		case "dis_max":
			for _, item := range asQueryList(bm["queries"]) {
				if child, ok := item.(map[string]any); ok {
					percolatorFieldRefs(child, out)
				}
			}
		case "boosting":
			for _, key := range []string{"positive", "negative"} {
				if child, ok := bm[key].(map[string]any); ok {
					percolatorFieldRefs(child, out)
				}
			}
		case "function_score", "script_score", "wrapper":
			if child, ok := bm["query"].(map[string]any); ok {
				percolatorFieldRefs(child, out)
			}
		case "nested", "has_child", "has_parent":
			if child, ok := bm["query"].(map[string]any); ok {
				percolatorFieldRefs(child, out)
			}
		case "span_near", "span_or":
			for _, item := range asQueryList(bm["clauses"]) {
				if child, ok := item.(map[string]any); ok {
					percolatorFieldRefs(child, out)
				}
			}
		case "span_not", "span_containing", "span_within":
			for _, key := range []string{"include", "exclude", "little", "big"} {
				if child, ok := bm[key].(map[string]any); ok {
					percolatorFieldRefs(child, out)
				}
			}
		case "exists":
			if f, ok := bm["field"].(string); ok {
				*out = append(*out, f)
			}
		case "multi_match", "query_string", "simple_query_string", "combined_fields", "more_like_this":
			if raw, ok := bm["fields"].([]any); ok {
				for _, e := range raw {
					if s, ok := e.(string); ok {
						if i := strings.Index(s, "^"); i >= 0 {
							s = s[:i]
						}
						if i := strings.Index(s, ":"); i > 0 {
							s = s[i+1:]
						}
						*out = append(*out, s)
					}
				}
			}
			if f, ok := bm["default_field"].(string); ok && f != "" {
				*out = append(*out, f)
			}
		case "ids", "match_all", "match_none", "script", "percolate", "knn":
		default:
			for k := range bm {
				if !percolatorReservedKeys[k] {
					*out = append(*out, k)
				}
			}
		}
	}
}

func percolatorFields(idx *Index) []string {
	props, _ := idx.Mappings["properties"].(map[string]any)
	var out []string
	for name, raw := range props {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := def["type"].(string); t == "percolator" {
			out = append(out, name)
		}
	}
	return out
}

func percolatorParseError(reason string, fields map[string]any, caused *apiError) *apiError {
	cause := &apiError{Type: "parsing_exception", Status: 400, Reason: reason, Fields: fields}
	if caused != nil {
		cause = caused
	}
	root := &apiError{Type: cause.Type, Status: cause.Status, Reason: cause.Reason, Fields: cause.Fields}
	return &apiError{Type: "mapper_parsing_exception", Status: 400, Reason: "failed to parse",
		RootCause: root, CausedBy: cause, Fields: cause.Fields}
}

func validatePercolatorDocs(idx *Index, body map[string]any, raw []byte) *apiError {
	fields := percolatorFields(idx)
	for _, field := range fields {
		rawQ, ok := body[field]
		if !ok {
			continue
		}
		qm, ok := rawQ.(map[string]any)
		if !ok {
			col := len(raw)
			if col == 0 {
				col = 1
			}
			return percolatorParseError("[_na] query malformed, must start with start_object",
				map[string]any{"line": 1, "col": col}, nil)
		}
		if verr := validateQuery(idx, qm); verr != nil {
			if verr.Clause != "" {
				annotateQueryClause(verr, raw)
			}
			root := &apiError{Type: verr.Type, Status: verr.Status, Reason: verr.Reason, Fields: verr.Fields}
			return &apiError{Type: "mapper_parsing_exception", Status: 400, Reason: "failed to parse",
				RootCause: root, CausedBy: verr, Fields: verr.Fields}
		}
		var refs []string
		percolatorFieldRefs(qm, &refs)
		for _, f := range refs {
			if f == "_id" || f == "_index" || f == "_routing" {
				continue
			}
			if _, mapped := mappingFieldDef(idx, f); !mapped {
				reason := "No field mapping can be found for the field with name [" + f + "]"
				cause := &apiError{Type: "query_shard_exception", Status: 400, Reason: reason,
					Fields: map[string]any{"index": idx.Name, "index_uuid": idx.UUID}}
				return &apiError{Type: "mapper_parsing_exception", Status: 400, Reason: "failed to parse",
					RootCause: cause, CausedBy: cause, Fields: cause.Fields}
			}
		}
	}
	return nil
}

func joinDocError(reason string) *apiError {
	return &apiError{Type: "mapper_parsing_exception", Reason: "failed to parse", Status: 400,
		CausedBy: &apiError{Type: "illegal_argument_exception", Reason: reason}}
}

func validateJoinDoc(idx *Index, body map[string]any, routing string) *apiError {
	field, relations, hasJoin := findJoinField(idx)
	if !hasJoin {
		return nil
	}
	raw, ok := body[field]
	if !ok {
		return nil
	}
	var name, parent string
	switch t := raw.(type) {
	case string:
		name = t
	case map[string]any:
		name, _ = t["name"].(string)
		parent, _ = t["parent"].(string)
	default:
		return nil
	}
	if name == "" {
		return joinDocError("join name is missing")
	}
	if !joinValidName(relations, name) {
		return joinDocError("unknown join name [" + name + "] for field [" + field + "]")
	}
	if _, isParent := relations[name]; isParent {
		return nil
	}
	if parent == "" {
		return joinDocError("[parent] is missing for join field [" + field + "]")
	}
	if routing == "" {
		return joinDocError("[routing] is missing for join field [" + field + "]")
	}
	return nil
}

func (s *Server) handleIndexDocV2(w http.ResponseWriter, r *http.Request, name, id, opType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := unknownParams(r, indexDocParams); err != nil {
		writeError(w, err)
		return
	}
	rv, rerr := refreshValue(r)
	if rerr != nil {
		writeError(w, rerr)
		return
	}
	if op := r.URL.Query().Get("op_type"); op != "" {
		opType = op
	}
	if opType != "create" && opType != "index" {
		writeError(w, &apiError{Type: "illegal_argument_exception", Status: 400,
			Reason: fmt.Sprintf("opType must be 'create' or 'index', found: [%s]", opType)})
		return
	}
	if id == "" && r.Method == http.MethodPut {
		methodNotAllowed(w, r, "POST")
		return
	}
	if len(id) > 512 {
		writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: fmt.Sprintf("Validation Failed: 1: id is too long, must be no longer than 512 bytes but was: %d;", len(id))})
		return
	}
	idx, apiErr := s.resolveWriteTargetV2(name)
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
	if apiErr := waitForActiveShards(r, idx); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if idx.DataStream != "" && opType != "create" && id != "" {
		writeError(w, &apiError{Type: "illegal_argument_exception", Status: 400,
			Reason: "only write ops with an op_type of create are allowed in data streams"})
		return
	}
	routing := requestRouting(r)
	if routing == "" {
		routing = aliasRoutingFor(idx, name)
	}
	autoID := id == ""
	if id == "" {
		id = randomID20(s)
	}
	if mappingRoutingRequired(idx) && routing == "" {
		if autoID {
			writeError(w, &apiError{Type: "null_pointer_exception", Status: 500, Reason: "id must not be null"})
			return
		}
		writeError(w, routingMissing(idx, id))
		return
	}
	if p := r.URL.Query().Get("pipeline"); p != "" && p != "_none" {
		if _, ok := s.ingestPipelines[p]; !ok {
			writeError(w, &apiError{Type: "illegal_argument_exception", Status: 400,
				Reason: "pipeline with id [" + p + "] does not exist"})
			return
		}
	}
	body, rawBody, berr := readDocBody(r)
	if berr != nil {
		writeError(w, berr)
		return
	}
	if idx.DataStream != "" {
		if derr := s.validateDataStreamDoc(idx, s.resolveDataStream(idx.DataStream), body); derr != nil {
			writeError(w, derr)
			return
		}
	}
	ensureDocState(idx)
	if derr := checkMixedArray(idx, body, ""); derr != nil {
		writeError(w, derr)
		return
	}
	if err := checkIfSeq(r, idx, id); err != nil {
		writeError(w, err)
		return
	}
	_, existing := idx.Docs[id]
	if opType == "create" && existing {
		cur := idx.Versions[id]
		if cur == 0 {
			cur = 1
		}
		writeError(w, versionConflict(idx, id, fmt.Sprintf("[%s]: version conflict, document already exists (current version [%d])", id, cur)))
		return
	}
	ver, verr := plannedVersion(idx, id, r)
	if verr != nil {
		writeError(w, verr)
		return
	}
	params := map[string]any{}
	if p := r.URL.Query().Get("pipeline"); p != "" {
		params["pipeline"] = p
	}
	body, apiErr = s.applyPipelines(idx, body, params)
	if apiErr != nil {
		if apiErr.Type == "drop_processor_exception" {
			writeJSON(w, 200, map[string]any{"_index": idx.Name, "_id": id, "_version": -3,
				"result": "noop", "_shards": map[string]any{"total": 0, "successful": 0, "failed": 0}})
			return
		}
		writeError(w, apiErr)
		return
	}
	if v, ok := params["_index"].(string); ok && v != "" && v != idx.Name {
		target, terr := s.resolveWriteTargetV2(v)
		if terr != nil {
			writeError(w, terr)
			return
		}
		idx = target
		ensureDocState(idx)
	}
	if v, ok := params["_id"].(string); ok && v != "" {
		id = v
	}
	if v, ok := params["_routing"].(string); ok {
		routing = v
	}
	if params["pipeline"] != nil {
		rawBody = nil
	} else {
		if _, ok := findSetting(idx.Settings, "default_pipeline"); ok {
			rawBody = nil
		}
		if _, ok := findSetting(idx.Settings, "final_pipeline"); ok {
			rawBody = nil
		}
	}
	if derr := validatePercolatorDocs(idx, body, rawBody); derr != nil {
		writeError(w, derr)
		return
	}
	if derr := validateJoinDoc(idx, body, routing); derr != nil {
		writeError(w, derr)
		return
	}
	props := ensureProperties(idx.Mappings)
	if derr := mappingShapeConflict(props, body); derr != nil {
		writeError(w, derr)
		return
	}
	if derr := checkDynamicStrict(idx, props, body); derr != nil {
		writeError(w, derr)
		return
	}
	if derr := mergeMappingForIndex(idx, props, body); derr != nil {
		writeError(w, derr)
		return
	}
	ensureFieldTypes(idx)
	if derr := checkTotalFields(idx); derr != nil {
		writeError(w, derr)
		return
	}
	if derr := idx.computeDocValues(id, body); derr != nil {
		writeError(w, derr)
		return
	}
	if !existing {
		idx.Order = append(idx.Order, id)
	}
	idx.Docs[id] = body
	idx.RawDocs[id] = append([]byte(nil), rawBody...)
	idx.Versions[id] = ver
	seq := nextSeq(idx)
	idx.SeqNos[id] = seq
	idx.PrimaryTerms[id] = 1
	idx.IndexTotal++
	if routing != "" {
		idx.Routing[id] = routing
	} else {
		delete(idx.Routing, id)
	}
	if rv == "true" || rv == "wait_for" {
		delete(idx.Pending, id)
	} else {
		markPending(idx, id, body, existing)
	}
	status := 201
	result := "created"
	if existing {
		status = 200
		result = "updated"
	}
	resp := orderedJSON{
		{"_index", idx.Name}, {"_id", id}, {"_version", ver}, {"result", result},
	}
	if rv == "true" {
		resp = append(resp, orderedKV{"forced_refresh", true})
	}
	resp = append(resp, orderedKV{"_shards", map[string]any{"total": idx.numShards(), "successful": idx.numShards(), "failed": 0}},
		orderedKV{"_seq_no", seq}, orderedKV{"_primary_term", int64(1)})
	if routing != "" {
		resp = append(resp, orderedKV{"_routing", routing})
	}
	if status == 201 {
		loc := "/" + idx.Name + "/_doc/" + javaURLEncode(id)
		if routing != "" {
			loc += "?routing=" + javaURLEncode(routing)
		}
		w.Header().Set("Location", loc)
	}
	writeJSON(w, status, resp)
}

func randUint64() uint64 {
	var b [8]byte
	if _, err := crand.Read(b[:]); err != nil {
		return 0
	}
	return binary.LittleEndian.Uint64(b[:])
}

func randomID20(s *Server) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	b := make([]byte, 20)
	for i := range b {
		b[i] = alphabet[int(randUint64()%uint64(len(alphabet)))]
	}
	return string(b)
}

func (s *Server) handleGetDocV2(w http.ResponseWriter, r *http.Request, name, id string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := unknownParams(r, getDocParams); err != nil {
		writeError(w, err)
		return
	}
	if err := strictBoolParam(r, "realtime"); err != nil {
		writeError(w, err)
		return
	}
	idx, apiErr := s.resolveReadTarget(name)
	if apiErr != nil {
		if apiErr.Type == "index_not_found_exception" {
			apiErr.Fields = map[string]any{"index": name, "resource.type": "index_expression", "resource.id": name, "index_uuid": "_na_"}
		}
		writeError(w, apiErr)
		return
	}
	if idx.Closed {
		writeError(w, errIndexClosed(idx))
		return
	}
	if apiErr := readBlockError(idx); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	routing := requestRouting(r)
	if mappingRoutingRequired(idx) && routing == "" {
		writeError(w, routingMissing(idx, id))
		return
	}
	doc, ok := idx.Docs[id]
	if !ok || !routingMatches(idx, id, routing) {
		writeJSON(w, 404, map[string]any{"_index": idx.Name, "_id": id, "found": false})
		return
	}
	if r.URL.Query().Get("realtime") == "false" {
		if _, pending := idx.Pending[id]; pending {
			writeJSON(w, 404, map[string]any{"_index": idx.Name, "_id": id, "found": false})
			return
		}
	}
	ver := currentVersion(idx, id)
	if vraw := r.URL.Query().Get("version"); vraw != "" {
		if want, err := strconv.ParseInt(vraw, 10, 64); err == nil && want != ver {
			writeError(w, versionConflict(idx, id, fmt.Sprintf("[%s]: version conflict, current version [%d] is different than the one provided [%d]", id, ver, want)))
			return
		}
	}
	seq := idx.SeqNos[id]
	term := idx.PrimaryTerms[id]
	if term == 0 {
		term = 1
	}
	sourceEnabled := mappingSourceEnabled(idx)
	includes, excludes, omit, _ := sourceFilterFromQuery(r.URL.Query())
	if !sourceEnabled {
		omit = true
	}
	if len(includes) == 0 && len(excludes) == 0 {
		mInc, mExc, _ := mappingSourceSpec(idx)
		includes, excludes = mInc, mExc
	}
	storedSpecRaw := r.URL.Query().Get("stored_fields")
	if storedSpecRaw != "" {
		_, _, _, explicitSource := sourceFilterFromQuery(r.URL.Query())
		if !explicitSource {
			omit = true
		}
	}
	resp := orderedJSON{
		{"_index", idx.Name}, {"_id", id}, {"_version", ver}, {"_seq_no", seq},
		{"_primary_term", term},
	}
	if ign := idx.DocIgnored[id]; len(ign) > 0 {
		vals := make([]any, 0, len(ign))
		for _, f := range ign {
			vals = append(vals, f)
		}
		resp = append(resp, orderedKV{"_ignored", vals})
	}
	resp = append(resp, orderedKV{"found", true})
	if !omit {
		if len(includes) == 0 && len(excludes) == 0 {
			if raw, ok := idx.RawDocs[id]; ok && len(raw) > 0 {
				resp = append(resp, orderedKV{"_source", verbatimJSON(raw)})
			} else {
				resp = append(resp, orderedKV{"_source", doc})
			}
		} else if raw, ok := idx.RawDocs[id]; ok {
			if filtered, ok2 := filterRawSource(raw, includes, excludes); ok2 {
				resp = append(resp, orderedKV{"_source", verbatimJSON(filtered)})
			} else {
				resp = append(resp, orderedKV{"_source", normalizeNumbers(filterSourceDoc(doc, includes, excludes))})
			}
		} else {
			resp = append(resp, orderedKV{"_source", normalizeNumbers(filterSourceDoc(doc, includes, excludes))})
		}
	}
	if fields := storedFieldsFor(idx, doc, storedSpec(storedSpecRaw, idx)); fields != nil {
		resp = append(resp, orderedKV{"fields", fields})
	}
	if rr, ok := idx.Routing[id]; ok && rr != "" {
		resp = append(resp, orderedKV{"_routing", rr})
	}
	writeJSON(w, 200, resp)
}

func (s *Server) handleGetSourceV2(w http.ResponseWriter, r *http.Request, name, id string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := unknownParams(r, getDocParams); err != nil {
		writeError(w, err)
		return
	}
	idx, apiErr := s.resolveReadTarget(name)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if idx.Closed {
		writeError(w, errIndexClosed(idx))
		return
	}
	if apiErr := readBlockError(idx); apiErr != nil {
		writeError(w, apiErr)
		return
	}
	routing := requestRouting(r)
	if mappingRoutingRequired(idx) && routing == "" {
		writeError(w, routingMissing(idx, id))
		return
	}
	doc, ok := idx.Docs[id]
	if !ok || !routingMatches(idx, id, routing) {
		writeError(w, &apiError{Type: "resource_not_found_exception", Status: 404,
			Reason: "Document not found [" + idx.Name + "]/[" + id + "]"})
		return
	}
	if !mappingSourceEnabled(idx) {
		writeError(w, &apiError{Type: "resource_not_found_exception", Status: 404,
			Reason: "Source not found [" + idx.Name + "]/[" + id + "]"})
		return
	}
	includes, excludes, _, _ := sourceFilterFromQuery(r.URL.Query())
	mInc, mExc, _ := mappingSourceSpec(idx)
	if len(includes) == 0 && len(excludes) == 0 {
		includes, excludes = mInc, mExc
	}
	if raw, ok := idx.RawDocs[id]; ok && len(raw) > 0 {
		if len(includes) == 0 && len(excludes) == 0 {
			writeJSON(w, 200, verbatimJSON(raw))
			return
		}
		if filtered, ok2 := filterRawSource(raw, includes, excludes); ok2 {
			writeJSON(w, 200, verbatimJSON(filtered))
			return
		}
	}
	writeJSON(w, 200, normalizeNumbers(filterSourceDoc(doc, includes, excludes)))
}

func (s *Server) handleDeleteDocV2(w http.ResponseWriter, r *http.Request, name, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := unknownParams(r, deleteDocParams); err != nil {
		writeError(w, err)
		return
	}
	rv, rerr := refreshValue(r)
	if rerr != nil {
		writeError(w, rerr)
		return
	}
	idx, apiErr := s.resolveWriteTarget(name)
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
	if mappingRoutingRequired(idx) && routing == "" {
		writeError(w, routingMissing(idx, id))
		return
	}
	ensureDocState(idx)
	if err := checkIfSeq(r, idx, id); err != nil {
		writeError(w, err)
		return
	}
	ver, verr := plannedVersion(idx, id, r)
	if verr != nil {
		writeError(w, verr)
		return
	}
	_, exists := idx.Docs[id]
	if exists && !routingMatches(idx, id, routing) {
		exists = false
	}
	seq := nextSeq(idx)
	idx.SeqNos[id] = seq
	idx.PrimaryTerms[id] = 1
	if !exists {
		idx.Versions[id] = ver
		resp := orderedJSON{
			{"_index", idx.Name}, {"_id", id}, {"_version", ver}, {"result", "not_found"},
			{"_shards", map[string]any{"total": idx.numShards(), "successful": idx.numShards(), "failed": 0}},
			{"_seq_no", seq}, {"_primary_term", int64(1)},
		}
		writeJSON(w, 404, resp)
		return
	}
	delete(idx.Docs, id)
	delete(idx.Pending, id)
	delete(idx.RawDocs, id)
	idx.removeDocValues(id)
	idx.DeleteTotal++
	idx.Versions[id] = ver
	resp := orderedJSON{
		{"_index", idx.Name}, {"_id", id}, {"_version", ver}, {"result", "deleted"},
	}
	if rv == "true" {
		resp = append(resp, orderedKV{"forced_refresh", true})
	}
	resp = append(resp, orderedKV{"_shards", map[string]any{"total": idx.numShards(), "successful": idx.numShards(), "failed": 0}},
		orderedKV{"_seq_no", seq}, orderedKV{"_primary_term", int64(1)})
	writeJSON(w, 200, resp)
}

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

func (s *Server) handleUpdateDocV2(w http.ResponseWriter, r *http.Request, name, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := unknownParams(r, updateDocParams); err != nil {
		writeError(w, err)
		return
	}
	if _, ok := r.URL.Query()["version"]; ok {
		writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: version is not allowed in update requests;"})
		return
	}
	rv, rerr := refreshValue(r)
	if rerr != nil {
		writeError(w, rerr)
		return
	}
	idx, apiErr := s.resolveWriteTargetV2(name)
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
	if mappingRoutingRequired(idx) && routing == "" {
		writeError(w, routingMissing(idx, id))
		return
	}
	body, _, berr := readDocBody(r)
	if berr != nil {
		writeError(w, berr)
		return
	}
	allowed := map[string]bool{"doc": true, "upsert": true, "script": true, "scripted_upsert": true,
		"doc_as_upsert": true, "detect_noop": true, "_source": true, "retry_on_conflict": true}
	for k := range body {
		if !allowed[k] {
			writeError(w, &apiError{Type: "x_content_parse_exception", Status: 400,
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
		writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: script or doc is missing;"})
		return
	}
	if hasDoc && hasScript {
		writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: can't provide both script and doc;"})
		return
	}
	ensureDocState(idx)
	if err := checkIfSeq(r, idx, id); err != nil {
		writeError(w, err)
		return
	}
	old, exists := idx.Docs[id]
	if exists && !routingMatches(idx, id, routing) {
		exists = false
	}
	if exists && !mappingSourceEnabled(idx) {
		writeError(w, &apiError{Type: "document_source_missing_exception", Status: 400,
			Reason: "[" + id + "]: document source missing", Fields: map[string]any{"index": idx.Name, "shard": "0"}})
		return
	}
	if !exists {
		if !hasUpsert && !docAsUpsert {
			if hasScript && scriptedUpsert {
				writeError(w, &apiError{Type: "document_missing_exception", Status: 404,
					Reason: "[" + id + "]: document missing", Fields: map[string]any{"index": idx.Name, "shard": "0"}})
				return
			}
			writeError(w, &apiError{Type: "document_missing_exception", Status: 404,
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
			resp := orderedJSON{
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
		src, apiErr = s.applyPipelines(idx, src, pparams)
		if apiErr != nil {
			writeError(w, apiErr)
			return
		}
		props := ensureProperties(idx.Mappings)
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
		ensureFieldTypes(idx)
		if derr := idx.computeDocValues(id, src); derr != nil {
			writeError(w, derr)
			return
		}
		idx.Order = append(idx.Order, id)
		idx.Docs[id] = src
		delete(idx.RawDocs, id)
		idx.Versions[id] = 1
		seq := nextSeq(idx)
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
		resp := orderedJSON{
			{"_index", idx.Name}, {"_id", id}, {"_version", int64(1)}, {"result", "created"},
		}
		if rv == "true" {
			resp = append(resp, orderedKV{"forced_refresh", true})
		}
		resp = append(resp, orderedKV{"_shards", map[string]any{"total": idx.numShards(), "successful": idx.numShards(), "failed": 0}},
			orderedKV{"_seq_no", seq}, orderedKV{"_primary_term", int64(1)})
		if getSpecified(r, body) {
			resp = append(resp, orderedKV{"get", updateGetObject(idx, id, src, seq, r, body)})
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
		seq := nextSeq(idx)
		delete(idx.Docs, id)
		delete(idx.Pending, id)
		idx.removeDocValues(id)
		idx.DeleteTotal++
		idx.Versions[id] = ver
		idx.SeqNos[id] = seq
		resp := orderedJSON{
			{"_index", idx.Name}, {"_id", id}, {"_version", ver}, {"result", "deleted"},
			{"_shards", map[string]any{"total": idx.numShards(), "successful": idx.numShards(), "failed": 0}},
			{"_seq_no", seq}, {"_primary_term", int64(1)},
		}
		writeJSON(w, 200, resp)
		return
	}
	pparams := map[string]any{}
	if p := r.URL.Query().Get("pipeline"); p != "" {
		pparams["pipeline"] = p
	}
	newDoc, apiErr = s.applyPipelines(idx, newDoc, pparams)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if op == "none" || op == "noop" || (detectNoop && reflect.DeepEqual(old, newDoc)) {
		resp := orderedJSON{
			{"_index", idx.Name}, {"_id", id}, {"_version", idx.Versions[id]}, {"result", "noop"},
			{"_shards", map[string]any{"total": 0, "successful": 0, "failed": 0}},
			{"_seq_no", idx.SeqNos[id]}, {"_primary_term", int64(1)},
		}
		if getSpecified(r, body) {
			resp = append(resp, orderedKV{"get", updateGetObject(idx, id, old, idx.SeqNos[id], r, body)})
		}
		writeJSON(w, 200, resp)
		return
	}
	props := ensureProperties(idx.Mappings)
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
	ensureFieldTypes(idx)
	if derr := idx.computeDocValues(id, newDoc); derr != nil {
		writeError(w, derr)
		return
	}
	ver := idx.Versions[id] + 1
	seq := nextSeq(idx)
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
		markPending(idx, id, newDoc, true)
	}
	resp := orderedJSON{
		{"_index", idx.Name}, {"_id", id}, {"_version", ver}, {"result", "updated"},
	}
	if rv == "true" {
		resp = append(resp, orderedKV{"forced_refresh", true})
	}
	resp = append(resp, orderedKV{"_shards", map[string]any{"total": idx.numShards(), "successful": idx.numShards(), "failed": 0}},
		orderedKV{"_seq_no", seq}, orderedKV{"_primary_term", int64(1)})
	if getSpecified(r, body) {
		resp = append(resp, orderedKV{"get", updateGetObject(idx, id, newDoc, seq, r, body)})
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

func updateGetObject(idx *Index, id string, src map[string]any, seq int64, r *http.Request, body map[string]any) orderedJSON {
	includes, excludes, _, _ := sourceFilterFromQuery(r.URL.Query())
	if v, ok := body["_source"]; ok {
		inc, exc, omit := sourceSpecFromValue(v)
		if omit {
			return orderedJSON{{"_seq_no", seq}, {"_primary_term", int64(1)}, {"found", true}}
		}
		if inc != nil || exc != nil {
			includes, excludes = inc, exc
		}
	}
	out := orderedJSON{{"_seq_no", seq}, {"_primary_term", int64(1)}, {"found", true}}
	if mappingSourceEnabled(idx) {
		out = append(out, orderedKV{"_source", filterSourceDoc(src, includes, excludes)})
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
func updateScriptFailure(se *plScriptErr, cause *apiError) *apiError {
	var causeObj orderedJSON
	if se != nil {
		causeObj = se.object(true)
	} else {
		causeObj = orderedJSON{{"type", cause.Type}, {"reason", cause.Reason}}
		if cause.CausedBy != nil {
			causeObj = append(causeObj, orderedKV{"caused_by", orderedJSON{{"type", cause.CausedBy.Type}, {"reason", cause.CausedBy.Reason}}})
		}
	}
	body := orderedJSON{{"error", orderedJSON{
		{"root_cause", []any{orderedJSON{{"type", "illegal_argument_exception"}, {"reason", "failed to execute script"}}}},
		{"type", "illegal_argument_exception"}, {"reason", "failed to execute script"},
		{"caused_by", causeObj},
	}}, {"status", 400}}
	e := &apiError{Type: "illegal_argument_exception", Status: 400, Reason: "failed to execute script",
		Shard: &shardFailure{raw: body, status: 400}}
	if se != nil {
		e.CausedBy = &apiError{Type: "script_exception", Reason: map[bool]string{true: "compile error", false: "runtime error"}[se.compile]}
		e.Extra = se
	} else {
		e.CausedBy = cause
	}
	return e
}

// runUpdateScript runs an update-context script (update, update_by_query,
// reindex) against a copy of ctx and reports the resulting operation.
func runUpdateScript(raw any, source map[string]any, index, id string, version int64) (updateScriptResult, *apiError) {
	return runUpdateScriptCtx(raw, source, index, id, version, "index", nil)
}

func runUpdateScriptCtx(raw any, source map[string]any, index, id string, version int64, op string, routing any) (updateScriptResult, *apiError) {
	spec, ae := parseScriptSpec(raw)
	if ae != nil {
		return updateScriptResult{}, updateScriptFailure(nil, ae)
	}
	if spec.lang == "expression" {
		_, se, ae := exPrepare(spec.src, "update")
		if se != nil {
			se.script = spec.errName()
		}
		return updateScriptResult{}, updateScriptFailure(se, ae)
	}
	if ae := checkScriptLang(spec, "update"); ae != nil {
		return updateScriptResult{}, updateScriptFailure(nil, ae)
	}
	script, se := compilePainless(spec, "update")
	if se != nil {
		return updateScriptResult{}, updateScriptFailure(se, nil)
	}
	ctx := newPlMap(plHash)
	ctx.put("_index", index)
	ctx.put("_id", id)
	ctx.put("_version", int64(version))
	if rs, ok := routing.(string); ok && rs == "" {
		routing = nil
	}
	ctx.put("_routing", plFromJSON(routing, plLinked))
	ctx.put("op", op)
	ctx.put("_now", nowMillis())
	ctx.put("_source", plFromJSON(deepCopySource(source), plLinked))
	_, rerr := script.exec(plBindings{vars: map[string]any{"params": scriptParams(spec.params), "ctx": ctx}}, spec.errName())
	if rerr != nil {
		return updateScriptResult{}, updateScriptFailure(rerr, nil)
	}
	res := updateScriptResult{op: "index"}
	rawOp, _ := ctx.get("op")
	res.rawOp = plToString(rawOp)
	switch res.rawOp {
	case "delete":
		res.op = "delete"
	case "index", "create":
		res.op = "index"
	default:
		res.op = "noop"
	}
	if src, ok := ctx.get("_source"); ok {
		if m, ok := plToJSON(src).(map[string]any); ok {
			res.source = m
		}
	}
	if res.source == nil {
		res.source = map[string]any{}
	}
	res.id, _ = ctx.get("_id")
	res.index, _ = ctx.get("_index")
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

var mgetParams = map[string]bool{
	"realtime": true, "preference": true, "refresh": true, "routing": true,
	"_source": true, "_source_includes": true, "_source_excludes": true,
	"stored_fields": true, "error_trace": true, "human": true, "pretty": true,
	"filter_path": true,
}

func (s *Server) handleMget(w http.ResponseWriter, r *http.Request, pathIndex string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		methodNotAllowed(w, r, "GET", "POST")
		return
	}
	if err := unknownParams(r, mgetParams); err != nil {
		writeError(w, err)
		return
	}
	body, _, berr := readDocBody(r)
	if berr != nil {
		writeError(w, berr)
		return
	}
	for k := range body {
		if k != "docs" && k != "ids" {
			writeError(w, &apiError{Type: "parsing_exception", Status: 400,
				Reason: "unknown key [" + k + "] for a START_ARRAY, expected [docs] or [ids]"})
			return
		}
	}
	var entries []map[string]any
	if raw, ok := body["docs"]; ok {
		arr, _ := raw.([]any)
		if len(arr) == 0 {
			writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
				Reason: "Validation Failed: 1: no documents to get;"})
			return
		}
		for i, e := range arr {
			m, ok := e.(map[string]any)
			if !ok {
				writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
					Reason: fmt.Sprintf("Validation Failed: 1: id is missing for doc %d;", i)})
				return
			}
			if _, ok := m["_id"]; !ok {
				writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
					Reason: fmt.Sprintf("Validation Failed: 1: id is missing for doc %d;", i)})
				return
			}
			if pathIndex == "" {
				if _, ok := m["_index"]; !ok {
					writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
						Reason: fmt.Sprintf("Validation Failed: 1: index is missing for doc %d;", i)})
					return
				}
			}
			entries = append(entries, m)
		}
	} else if raw, ok := body["ids"]; ok {
		arr, _ := raw.([]any)
		if len(arr) == 0 || pathIndex == "" {
			if pathIndex == "" {
				writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
					Reason: "Validation Failed: 1: index is missing for doc 0;"})
				return
			}
			writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
				Reason: "Validation Failed: 1: no documents to get;"})
			return
		}
		for _, e := range arr {
			id, _ := valueToScriptString(e)
			entries = append(entries, map[string]any{"_id": id})
		}
	} else {
		writeError(w, &apiError{Type: "action_request_validation_exception", Status: 400,
			Reason: "Validation Failed: 1: no documents to get;"})
		return
	}
	urlInc, urlExc, urlOmit, _ := sourceFilterFromQuery(r.URL.Query())
	urlStored := r.URL.Query().Get("stored_fields")
	realtime := r.URL.Query().Get("realtime") != "false"
	docs := make([]any, 0, len(entries))
	for _, e := range entries {
		idxName, _ := e["_index"].(string)
		if idxName == "" {
			idxName = pathIndex
		}
		id, _ := e["_id"].(string)
		doc := map[string]any{"_index": idxName, "_id": id}
		idx, ok := s.indices[idxName]
		if !ok {
			err := errNotFound(idxName)
			doc["error"] = errorObject(err)
			docs = append(docs, doc)
			continue
		}
		routing, _ := e["routing"].(string)
		if mappingRoutingRequired(idx) && routing == "" {
			doc["error"] = errorObject(routingMissing(idx, id))
			docs = append(docs, doc)
			continue
		}
		src, found := idx.Docs[id]
		if !found || !routingMatches(idx, id, routing) {
			doc["found"] = false
			docs = append(docs, doc)
			continue
		}
		if !realtime {
			if _, pending := idx.Pending[id]; pending {
				doc["found"] = false
				docs = append(docs, doc)
				continue
			}
		}
		doc["_version"] = currentVersion(idx, id)
		doc["_seq_no"] = idx.SeqNos[id]
		term := idx.PrimaryTerms[id]
		if term == 0 {
			term = 1
		}
		doc["_primary_term"] = term
		doc["found"] = true
		inc, exc, omit := urlInc, urlExc, urlOmit
		if v, ok := e["_source"]; ok {
			i2, e2, o2 := sourceSpecFromValue(v)
			inc, exc, omit = i2, e2, o2
		}
		if !mappingSourceEnabled(idx) {
			omit = true
		}
		storedRaw := urlStored
		if v, ok := e["stored_fields"]; ok {
			switch t := v.(type) {
			case []any:
				var parts []string
				for _, x := range t {
					if s, ok := x.(string); ok {
						parts = append(parts, s)
					}
				}
				storedRaw = strings.Join(parts, ",")
			case string:
				storedRaw = t
			}
		}
		if storedRaw != "" {
			omit = true
		}
		if !omit {
			doc["_source"] = filterSourceDoc(src, inc, exc)
		}
		if fields := storedFieldsFor(idx, src, storedSpec(storedRaw, idx)); fields != nil {
			doc["fields"] = fields
		}
		if rr, ok := idx.Routing[id]; ok && rr != "" {
			doc["_routing"] = rr
		}
		docs = append(docs, doc)
	}
	writeJSON(w, 200, map[string]any{"docs": docs})
}

func errorObject(e *apiError) map[string]any {
	rc := e
	if e.RootCause != nil {
		rc = e.RootCause
	}
	out := map[string]any{"type": e.Type, "reason": e.Reason,
		"root_cause": []any{map[string]any{"type": rc.Type, "reason": rc.Reason}}}
	if e.Fields != nil {
		for k, v := range e.Fields {
			out[k] = v
		}
	}
	if e.CausedBy != nil {
		out["caused_by"] = errorObject(e.CausedBy)
	}
	return out
}

// markPending records an unrefreshed write. A document that searches could
// already see stays visible: a refresh only changes which version they read,
// it never makes an existing document disappear.
func markPending(idx *Index, id string, doc map[string]any, existed bool) {
	if _, pending := idx.Pending[id]; existed && !pending {
		noteStaleOverwrite(idx, id)
		return
	}
	idx.Pending[id] = doc
}
