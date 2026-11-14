package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Completion suggester: a model of Lucene's NRT completion suggester as used by
// OpenSearch's completion field. Every input of a document is analyzed with the
// field's analyzer and concatenated into one key (tokens joined by the 0x1F
// separator, position holes kept as empty labels); a query matches an input
// when its analyzed key is a prefix of the input key (exactly, within an edit
// distance, or by regular expression).

const (
	completionSep  = "\x1f"
	completionHole = "\x1e"
)

// ---------------------------------------------------------------- request validation

var suggesterTypes = map[string]bool{"term": true, "phrase": true, "completion": true}

// validateSuggestBody checks the shape of the "suggest" section of a search
// request (parse-time errors, answered before any shard is searched).
func validateSuggestBody(v any) *apiError {
	m, ok := v.(map[string]any)
	if !ok {
		return &apiError{Type: "parsing_exception", Status: 400,
			Reason: "Unknown key for a " + searchValueType(v) + " in [suggest]."}
	}
	for _, name := range sortedAnyKeys(m) {
		if name == "text" {
			continue
		}
		spec, ok := m[name].(map[string]any)
		if !ok {
			continue
		}
		found := false
		for _, k := range sortedAnyKeys(spec) {
			switch k {
			case "text", "prefix", "regex":
				continue
			}
			if _, isObj := spec[k].(map[string]any); !isObj {
				continue
			}
			if !suggesterTypes[k] {
				return &apiError{Type: "named_object_not_found_exception", Status: 400,
					Reason: "unknown field [" + k + "]"}
			}
			found = true
			if k == "completion" {
				if err := validateCompletionOptions(spec[k].(map[string]any)); err != nil {
					return err
				}
			}
		}
		if !found {
			return &apiError{Type: "parse_exception", Status: 400, Reason: "missing suggestion object"}
		}
	}
	return nil
}

var completionSuggestOptions = map[string]bool{
	"field": true, "analyzer": true, "size": true, "shard_size": true,
	"skip_duplicates": true, "fuzzy": true, "regex": true, "contexts": true,
}

func validateCompletionOptions(body map[string]any) *apiError {
	for _, k := range sortedAnyKeys(body) {
		if !completionSuggestOptions[k] {
			return &apiError{Type: "x_content_parse_exception", Status: 400,
				Reason: "[completion] unknown field [" + k + "]"}
		}
	}
	for _, k := range []string{"size", "shard_size"} {
		if v, has := body[k]; has {
			if n, ok := toFloat(v); ok && n <= 0 {
				reason := "size must be positive"
				if k == "shard_size" {
					reason = "shardSize must be positive"
				}
				return &apiError{Type: "x_content_parse_exception", Status: 400,
					Reason:   "[completion] failed to parse field [" + k + "]",
					CausedBy: &apiError{Type: "illegal_argument_exception", Reason: reason}}
			}
		}
	}
	if f, ok := toString(body["field"]); !ok || f == "" {
		return &apiError{Type: "parse_exception", Status: 400, Reason: "the required field option [field] is missing"}
	}
	return nil
}

// isSuggestOnlySearch reports whether a search body only asks for suggestions;
// such a request skips the query phase and answers zero hits.
func isSuggestOnlySearch(body map[string]any) bool {
	if _, ok := body["suggest"].(map[string]any); !ok {
		return false
	}
	for _, k := range []string{"query", "aggs", "aggregations", "post_filter", "knn"} {
		if _, has := body[k]; has {
			return false
		}
	}
	return true
}

// orderSearchResponse renders the top-level search response keys in the
// order OpenSearch writes them.
func orderSearchResponse(resp map[string]any) orderedJSON {
	first := []string{"_scroll_id", "took", "timed_out", "terminated_early", "_shards", "hits",
		"aggregations", "suggest", "profile"}
	out := orderedJSON{}
	seen := map[string]bool{}
	for _, k := range first {
		if v, ok := resp[k]; ok {
			out = append(out, orderedKV{k, v})
			seen[k] = true
		}
	}
	for _, k := range sortedAnyKeys(resp) {
		if !seen[k] {
			out = append(out, orderedKV{k, resp[k]})
		}
	}
	return out
}

// ---------------------------------------------------------------- mapping

type completionContextDef struct {
	name      string
	typ       string // "category" or "geo"
	path      string
	precision int
}

func completionContextDefs(def map[string]any) []completionContextDef {
	list, _ := def["contexts"].([]any)
	var out []completionContextDef
	for _, raw := range list {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		c := completionContextDef{precision: 6}
		c.name, _ = toString(m["name"])
		t, _ := toString(m["type"])
		c.typ = strings.ToLower(t)
		c.path, _ = toString(m["path"])
		if p, ok := toFloat(m["precision"]); ok {
			c.precision = int(p)
		}
		out = append(out, c)
	}
	return out
}

func completionContextNames(ctxs []completionContextDef) string {
	names := make([]string, len(ctxs))
	for i, c := range ctxs {
		names[i] = c.name
	}
	return strings.Join(names, ", ")
}

// validateCompletionMapping checks the completion specific mapping parameters.
func validateCompletionMapping(def map[string]any) *mappingValidationError {
	if v, has := def["max_input_length"]; has {
		if n, ok := intFromAny(v); ok && n <= 0 {
			return mve(fmt.Sprintf("[max_input_length] must be > 0 but was [%d]", n), "")
		}
	}
	if raw, has := def["contexts"]; has {
		list, ok := raw.([]any)
		if !ok {
			list = []any{raw}
		}
		for _, e := range list {
			m, ok := e.(map[string]any)
			if !ok {
				return mve("expected map or list of maps for [contexts] but got "+previewValue(e), "")
			}
			if _, ok := m["name"]; !ok {
				return mve("missing [name] in context mapping", "")
			}
			t, ok := toString(m["type"])
			if !ok {
				return mve("missing [type] in context mapping", "")
			}
			switch strings.ToLower(t) {
			case "category", "geo":
			default:
				return mve("No context type for ["+t+"]", "")
			}
		}
	}
	return nil
}

// normalizeCompletionMapping fills the implicit completion parameters so the
// mapping renders like OpenSearch's.
func normalizeCompletionMapping(def map[string]any) {
	if _, has := def["analyzer"]; !has {
		def["analyzer"] = "simple"
	}
	for _, k := range []string{"preserve_separators", "preserve_position_increments"} {
		if _, has := def[k]; !has {
			def[k] = true
		}
	}
	if _, has := def["max_input_length"]; !has {
		def["max_input_length"] = 50
	}
	if list, ok := def["contexts"].([]any); ok {
		for _, e := range list {
			m, ok := e.(map[string]any)
			if !ok {
				continue
			}
			if t, ok := m["type"].(string); ok {
				m["type"] = strings.ToUpper(t)
				if strings.EqualFold(t, "geo") {
					if _, has := m["precision"]; !has {
						m["precision"] = 6
					}
				}
			}
		}
	}
}

// ---------------------------------------------------------------- indexing

func completionParseError(causeType, reason string) *apiError {
	e := mapperErr("mapper_parsing_exception", "failed to parse", causeType, reason)
	if causeType == "parsing_exception" {
		e.RootCause = &apiError{Type: causeType, Reason: reason}
	}
	return e
}

// checkCompletionContexts validates the contexts of a completion value at
// index time against the field's context mappings.
func (c *indexCtx) checkCompletionContexts(path string, def map[string]any, v any) *apiError {
	ctxs := completionContextDefs(def)
	known := map[string]bool{}
	for _, cd := range ctxs {
		known[cd.name] = true
	}
	count := 0
	var visit func(v any) *apiError
	visit = func(v any) *apiError {
		switch t := v.(type) {
		case []any:
			for _, e := range t {
				if err := visit(e); err != nil {
					return err
				}
			}
		case map[string]any:
			if cm, ok := t["contexts"].(map[string]any); ok {
				for _, name := range sortedAnyKeys(cm) {
					if !known[name] {
						return completionParseError("illegal_argument_exception",
							"Unknown context name ["+name+"], must be one of ["+completionContextNames(ctxs)+"]")
					}
					count += len(completionValueList(cm[name]))
				}
			}
		}
		return nil
	}
	if err := visit(v); err != nil {
		return err
	}
	if len(ctxs) == 0 {
		return nil
	}
	for _, cd := range ctxs {
		if cd.path != "" && c.doc != nil {
			if pv, ok := fieldValue(c.doc, cd.path); ok {
				count += len(completionValueList(pv))
			}
		}
	}
	if count == 0 {
		return &apiError{Type: "illegal_argument_exception", Status: 400,
			Reason: "Contexts are mandatory in context enabled completion field [" + path + "]"}
	}
	return nil
}

func completionValueList(v any) []any {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		return t
	}
	return []any{v}
}

// completionEntry is one input of a document with its weight and contexts.
type completionEntry struct {
	input    string
	weight   float64
	contexts map[string][]any
}

func completionEntriesOf(v any) []completionEntry {
	var out []completionEntry
	var visit func(v any)
	visit = func(v any) {
		switch t := v.(type) {
		case string:
			out = append(out, completionEntry{input: t, weight: 1})
		case []any:
			for _, e := range t {
				visit(e)
			}
		case map[string]any:
			w := 1.0
			if wv, ok := t["weight"]; ok {
				if f, ok := toFloat(wv); ok {
					w = f
				} else if s, ok := wv.(string); ok {
					if n, err := strconv.ParseFloat(s, 64); err == nil {
						w = n
					}
				}
			}
			ctx := map[string][]any{}
			if cm, ok := t["contexts"].(map[string]any); ok {
				for k, cv := range cm {
					ctx[k] = completionValueList(cv)
				}
			}
			for _, in := range completionValueList(t["input"]) {
				if s, ok := valueScalarString(in); ok {
					out = append(out, completionEntry{input: s, weight: w, contexts: ctx})
				}
			}
		}
	}
	visit(v)
	return out
}

// ---------------------------------------------------------------- analysis

// completionKey concatenates analyzed tokens the way Lucene's
// ConcatenateGraphFilter does: positions joined by the separator, holes left
// by removed tokens kept as empty labels.
func completionKey(toks []analyzedToken, preserveSep, preservePos bool) (string, bool) {
	var labels []string
	last := -1
	for _, t := range toks {
		if t.pos <= last {
			continue // stacked token: keep the first path only
		}
		gap := t.pos - last
		if !preservePos && gap > 1 {
			gap = 1
		}
		for i := 1; i < gap; i++ {
			labels = append(labels, "")
		}
		labels = append(labels, t.term)
		last = t.pos
	}
	if len(labels) == 0 {
		return "", false
	}
	if preserveSep {
		return strings.Join(labels, completionSep), true
	}
	return strings.Join(labels, ""), true
}

// ---------------------------------------------------------------- fuzzy matching

type completionFuzzy struct {
	maxEdits      int
	transpose     bool
	minLength     int
	prefixLength  int
	unicodeAware  bool
	autoFuzziness bool
}

func parseCompletionFuzzy(raw any, text string) completionFuzzy {
	f := completionFuzzy{transpose: true, minLength: 3, prefixLength: 1}
	fuzziness := any("AUTO")
	if m, ok := raw.(map[string]any); ok {
		if v, ok := m["fuzziness"]; ok {
			fuzziness = v
		}
		if b, ok := m["transpositions"].(bool); ok {
			f.transpose = b
		}
		if n, ok := toFloat(m["min_length"]); ok {
			f.minLength = int(n)
		}
		if n, ok := toFloat(m["prefix_length"]); ok {
			f.prefixLength = int(n)
		}
		if b, ok := m["unicode_aware"].(bool); ok {
			f.unicodeAware = b
		}
	}
	f.maxEdits = fuzzinessDistance(fuzziness, text)
	return f
}

func fuzzinessDistance(v any, text string) int {
	if n, ok := toFloat(v); ok {
		return min(max(int(n), 0), 2)
	}
	s, _ := toString(v)
	up := strings.ToUpper(strings.TrimSpace(s))
	if strings.HasPrefix(up, "AUTO") {
		low, high := 3, 6
		if rest, ok := strings.CutPrefix(up, "AUTO:"); ok {
			if a, b, ok := strings.Cut(rest, ","); ok {
				if x, err := strconv.Atoi(a); err == nil {
					low = x
				}
				if y, err := strconv.Atoi(b); err == nil {
					high = y
				}
			}
		}
		n := utf8.RuneCountInString(text)
		switch {
		case n < low:
			return 0
		case n < high:
			return 1
		}
		return 2
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return min(max(int(n), 0), 2)
	}
	return 0
}

func completionUnits(s string, unicode bool) []int {
	if unicode {
		out := make([]int, 0, len(s))
		for _, r := range s {
			out = append(out, int(r))
		}
		return out
	}
	out := make([]int, len(s))
	for i := 0; i < len(s); i++ {
		out[i] = int(s[i])
	}
	return out
}

// fuzzyPrefixMatch reports whether some prefix of key is within maxEdits of
// query, the first prefixLen units matching exactly.
func fuzzyPrefixMatch(q, k []int, maxEdits, prefixLen int, transpose bool) bool {
	p := min(prefixLen, len(q))
	if len(k) < p {
		return false
	}
	for i := 0; i < p; i++ {
		if q[i] != k[i] {
			return false
		}
	}
	a, b := q[p:], k[p:]
	n, m := len(a), len(b)
	d := make([][]int, n+1)
	for i := range d {
		d[i] = make([]int, m+1)
		d[i][0] = i
	}
	for j := 0; j <= m; j++ {
		d[0][j] = j
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if transpose && i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	for j := 0; j <= m; j++ {
		if d[n][j] <= maxEdits {
			return true
		}
	}
	return false
}

func commonPrefixLen(a, b []int) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// ---------------------------------------------------------------- contexts

type completionQueryCtx struct {
	value  string
	boost  float64
	prefix bool
}

func parseCategoryQueryContexts(v any) []completionQueryCtx {
	var out []completionQueryCtx
	for _, e := range completionValueList(v) {
		switch t := e.(type) {
		case map[string]any:
			q := completionQueryCtx{boost: 1}
			q.value, _ = valueScalarString(t["context"])
			if b, ok := toFloat(t["boost"]); ok {
				q.boost = b
			}
			if p, ok := t["prefix"].(bool); ok {
				q.prefix = p
			}
			out = append(out, q)
		default:
			if s, ok := valueScalarString(e); ok {
				out = append(out, completionQueryCtx{value: s, boost: 1})
			}
		}
	}
	return out
}

func geoContextPoint(v any) (float64, float64, bool) {
	if s, ok := v.(string); ok && !strings.Contains(s, ",") {
		if lat, lon, ok := decodeGeohash(s); ok {
			return lat, lon, true
		}
	}
	return geoPointOf(v)
}

// geohashNeighbors returns the cells around a geohash at the same precision.
func geohashNeighbors(hash string) []string {
	lat, lon, ok := decodeGeohash(hash)
	if !ok {
		return nil
	}
	bits := len(hash) * 5
	lonBits := (bits + 1) / 2
	latBits := bits / 2
	w := 360.0 / float64(uint64(1)<<uint(lonBits))
	h := 180.0 / float64(uint64(1)<<uint(latBits))
	var out []string
	for _, dy := range []float64{-1, 0, 1} {
		for _, dx := range []float64{-1, 0, 1} {
			if dx == 0 && dy == 0 {
				continue
			}
			nlat := lat + dy*h
			if nlat > 90 || nlat < -90 {
				continue
			}
			nlon := lon + dx*w
			if nlon >= 180 {
				nlon -= 360
			} else if nlon < -180 {
				nlon += 360
			}
			out = append(out, geohashEncode(nlat, nlon, len(hash)))
		}
	}
	return out
}

func parseGeoQueryContexts(v any, mappingPrecision int) []completionQueryCtx {
	var list []any
	switch t := v.(type) {
	case []any:
		// a [lon, lat] pair is one point, otherwise a list of contexts
		if len(t) == 2 {
			_, ok1 := toFloat(t[0])
			_, ok2 := toFloat(t[1])
			if ok1 && ok2 {
				list = []any{v}
				break
			}
		}
		list = t
	default:
		list = []any{v}
	}
	var out []completionQueryCtx
	for _, e := range list {
		point := e
		precision := 12
		boost := 1.0
		var neighbours []int
		if m, ok := e.(map[string]any); ok {
			if c, has := m["context"]; has {
				point = c
			}
			if p, ok := toFloat(m["precision"]); ok {
				precision = int(p)
			}
			if b, ok := toFloat(m["boost"]); ok {
				boost = b
			}
			for _, nb := range completionValueList(m["neighbours"]) {
				if n, ok := toFloat(nb); ok {
					neighbours = append(neighbours, int(n))
				}
			}
		}
		lat, lon, ok := geoContextPoint(point)
		if !ok {
			continue
		}
		minPrecision := min(precision, mappingPrecision)
		hash := geohashEncode(lat, lon, minPrecision)
		locations := []string{hash}
		if len(neighbours) == 0 && len(hash) == mappingPrecision {
			locations = append(locations, geohashNeighbors(hash)...)
		} else {
			for _, np := range neighbours {
				if np < len(hash) {
					tr := hash[:np]
					locations = append(locations, tr)
					locations = append(locations, geohashNeighbors(tr)...)
				}
			}
		}
		seen := map[string]bool{}
		for _, loc := range locations {
			if seen[loc] {
				continue
			}
			seen[loc] = true
			out = append(out, completionQueryCtx{value: loc, boost: boost, prefix: len(loc) < mappingPrecision})
		}
	}
	return out
}

// indexedContexts returns the context values indexed for an entry.
func indexedContexts(ctxs []completionContextDef, e completionEntry, doc map[string]any) map[string][]string {
	out := map[string][]string{}
	for _, cd := range ctxs {
		vals := append([]any{}, e.contexts[cd.name]...)
		if cd.path != "" {
			if pv, ok := fieldValue(doc, cd.path); ok {
				vals = append(vals, completionValueList(pv)...)
			}
		}
		if cd.typ == "geo" {
			var points []any
			for _, v := range vals {
				if arr, ok := v.([]any); ok && len(arr) != 2 {
					points = append(points, arr...)
				} else {
					points = append(points, v)
				}
			}
			for _, p := range points {
				if lat, lon, ok := geoContextPoint(p); ok {
					out[cd.name] = append(out[cd.name], geohashEncode(lat, lon, cd.precision))
				}
			}
			continue
		}
		for _, v := range vals {
			if s, ok := valueScalarString(v); ok {
				out[cd.name] = append(out[cd.name], s)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- the suggester

type completionHit struct {
	idx      *Index
	idxPos   int
	docPos   int
	id       string
	text     string
	score    float64
	contexts []completionCtxHit
}

type completionCtxHit struct {
	name, value string
	score       float64
}

func completionSuggest(spec, body map[string]any, indices []*Index, globalText any, searchBody map[string]any) ([]any, *apiError) {
	if err := validateCompletionOptions(body); err != nil {
		return nil, err
	}
	field, _ := toString(body["field"])
	var first *Index
	if len(indices) > 0 {
		first = indices[0]
	}

	var text string
	var regex *regexp.Regexp
	hasText := false
	if rv, ok := spec["regex"]; ok {
		text, _ = valueScalarString(rv)
		hasText = true
		re, err := regexp.Compile("^(?s:" + text + ")")
		if err != nil {
			return nil, suggestShardError(first, "invalid regular expression ["+text+"]")
		}
		regex = re
	} else {
		for _, k := range []string{"prefix", "text"} {
			if v, ok := spec[k]; ok {
				text, _ = valueScalarString(v)
				hasText = true
				break
			}
		}
		if !hasText && globalText != nil {
			text, _ = valueScalarString(globalText)
			hasText = true
		}
	}
	if !hasText {
		return nil, suggestShardError(first, "The required text option is missing")
	}

	size := 5
	if n, ok := toFloat(body["size"]); ok {
		size = int(n)
	}
	skipDup, _ := body["skip_duplicates"].(bool)
	var fuzzy *completionFuzzy
	if fv, ok := body["fuzzy"]; ok && regex == nil {
		if b, isBool := fv.(bool); !isBool || b {
			f := parseCompletionFuzzy(fv, text)
			fuzzy = &f
		}
	}

	// resolve the field in every index
	type target struct {
		idx *Index
		pos int
		def map[string]any
	}
	var targets []target
	notCompletion := false
	for i, ix := range indices {
		def, ok := mappingFieldDef(ix, field)
		if !ok {
			continue
		}
		if t, _ := def["type"].(string); t != "completion" {
			notCompletion = true
			continue
		}
		targets = append(targets, target{ix, i, def})
	}
	if len(targets) == 0 {
		if notCompletion {
			return nil, suggestShardError(first, "Field ["+field+"] is not a completion suggest field")
		}
		return nil, suggestShardError(first, "no mapping found for field ["+field+"]")
	}

	includes, excludes, omitSource, _ := sourceFilterFromBody(searchBody)

	var hits []*completionHit
	for _, tg := range targets {
		ix, def := tg.idx, tg.def
		ctxs := completionContextDefs(def)
		qctx := map[string][]completionQueryCtx{}
		if raw, has := body["contexts"]; has {
			cm, _ := raw.(map[string]any)
			if len(ctxs) == 0 {
				return nil, suggestShardError(first, "suggester ["+field+"] doesn't expect any context")
			}
			for _, name := range sortedAnyKeys(cm) {
				var cd *completionContextDef
				for i := range ctxs {
					if ctxs[i].name == name {
						cd = &ctxs[i]
					}
				}
				if cd == nil {
					return nil, suggestShardError(first, "Unknown context name ["+name+"], must be one of ["+completionContextNames(ctxs)+"]")
				}
				if cd.typ == "geo" {
					qctx[name] = parseGeoQueryContexts(cm[name], cd.precision)
				} else {
					qctx[name] = parseCategoryQueryContexts(cm[name])
				}
			}
		}
		if len(ctxs) > 0 && len(qctx) == 0 {
			return nil, suggestShardError(first, "Missing mandatory contexts in context query")
		}

		analyzer, _ := def["analyzer"].(string)
		if analyzer == "" {
			analyzer = "simple"
		}
		searchAnalyzer, _ := def["search_analyzer"].(string)
		if searchAnalyzer == "" {
			searchAnalyzer = analyzer
		}
		if a, ok := body["analyzer"].(string); ok && a != "" {
			searchAnalyzer = a
		}
		preserveSep, preservePos := true, true
		if b, ok := def["preserve_separators"].(bool); ok {
			preserveSep = b
		}
		if b, ok := def["preserve_position_increments"].(bool); ok {
			preservePos = b
		}
		maxLen := 50
		if n, ok := toFloat(def["max_input_length"]); ok && n > 0 {
			maxLen = int(n)
		}

		var qkey string
		var qunits []int
		if regex == nil {
			k, ok := completionKey(analyzeTokensWithIndex(ix, searchAnalyzer, text), preserveSep, preservePos)
			if !ok {
				continue
			}
			qkey = k
			if fuzzy != nil {
				qunits = completionUnits(qkey, fuzzy.unicodeAware)
			}
		}

		for docPos, id := range ix.Order {
			doc, live := ix.Docs[id]
			if !live {
				continue
			}
			v, ok := fieldValue(doc, field)
			if !ok {
				continue
			}
			var hit *completionHit
			for _, e := range completionEntriesOf(v) {
				input := truncateUTF16(e.input, maxLen)
				key, ok := completionKey(analyzeTokensWithIndex(ix, analyzer, input), preserveSep, preservePos)
				if !ok {
					continue
				}
				boost := 0.0
				switch {
				case regex != nil:
					if !regex.MatchString(key) {
						continue
					}
				case fuzzy != nil:
					kunits := completionUnits(key, fuzzy.unicodeAware)
					exact := fuzzy.maxEdits == 0 || len(qunits) <= fuzzy.prefixLength || len(qunits) < fuzzy.minLength
					if exact {
						if !strings.HasPrefix(key, qkey) {
							continue
						}
					} else if !fuzzyPrefixMatch(qunits, kunits, fuzzy.maxEdits, fuzzy.prefixLength, fuzzy.transpose) {
						continue
					}
					boost = float64(commonPrefixLen(qunits, kunits))
				default:
					if !strings.HasPrefix(key, qkey) {
						continue
					}
				}
				var ctxHits []completionCtxHit
				best := 0.0
				if len(qctx) > 0 {
					indexed := indexedContexts(ctxs, e, doc)
					for _, cd := range ctxs {
						qs, asked := qctx[cd.name]
						if !asked {
							continue
						}
						for _, val := range indexed[cd.name] {
							matched := false
							cb := 0.0
							for _, q := range qs {
								if val == q.value || (q.prefix && strings.HasPrefix(val, q.value)) {
									if !matched || q.boost > cb {
										cb = q.boost
									}
									matched = true
								}
							}
							if !matched {
								continue
							}
							sc := completionScore(e.weight, cb+boost)
							ctxHits = append(ctxHits, completionCtxHit{cd.name, val, sc})
							best = max(best, sc)
						}
					}
					if len(ctxHits) == 0 {
						continue
					}
				} else {
					best = completionScore(e.weight, boost)
				}
				if hit == nil {
					hit = &completionHit{idx: ix, idxPos: tg.pos, docPos: docPos, id: id, text: input, score: best}
				} else if best > hit.score || (best == hit.score && input < hit.text) {
					hit.text, hit.score = input, best
				}
				hit.contexts = append(hit.contexts, ctxHits...)
			}
			if hit != nil {
				hits = append(hits, hit)
			}
		}
	}

	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.text != b.text {
			return a.text < b.text
		}
		if a.idxPos != b.idxPos {
			return a.idxPos < b.idxPos
		}
		return a.docPos < b.docPos
	})

	options := []any{}
	seenText := map[string]bool{}
	for _, h := range hits {
		if len(options) >= size {
			break
		}
		if skipDup {
			if seenText[h.text] {
				continue
			}
			seenText[h.text] = true
		}
		opt := orderedJSON{{"text", h.text}, {"_index", h.idx.Name}, {"_id", h.id},
			{"_score", scoreJSON(float64(float32(h.score)))}}
		if !omitSource {
			src := h.idx.Docs[h.id]
			if len(includes) > 0 || len(excludes) > 0 {
				src = filterSourceDoc(src, includes, excludes)
			}
			opt = append(opt, orderedKV{"_source", src})
		}
		if len(h.contexts) > 0 {
			sort.SliceStable(h.contexts, func(i, j int) bool { return h.contexts[i].score > h.contexts[j].score })
			ctxOut := orderedJSON{}
			for _, ch := range h.contexts {
				pos := findKey(ctxOut, ch.name)
				if pos < 0 {
					ctxOut = append(ctxOut, orderedKV{ch.name, []any{ch.value}})
					continue
				}
				vals := ctxOut[pos].v.([]any)
				dup := false
				for _, x := range vals {
					if x == ch.value {
						dup = true
					}
				}
				if !dup {
					ctxOut[pos].v = append(vals, ch.value)
				}
			}
			opt = append(opt, orderedKV{"contexts", ctxOut})
		}
		options = append(options, opt)
	}
	entry := orderedJSON{{"text", text}, {"offset", 0}, {"length", utf16Len([]rune(text))}, {"options", options}}
	return []any{entry}, nil
}

// completionScore mirrors Lucene's CompletionScorer.score(weight, boost).
func completionScore(weight, boost float64) float64 {
	if boost == 0 {
		return weight
	}
	if weight == 0 {
		return boost
	}
	return weight * boost
}
