package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"slopensearch/internal/analysis"
	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
	"slopensearch/internal/ojson"
	"slopensearch/internal/search"
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
func validateSuggestBody(v any) *apierr.Error {
	m, ok := v.(map[string]any)
	if !ok {
		return &apierr.Error{Type: "parsing_exception", Status: 400,
			Reason: "Unknown key for a " + searchValueType(v) + " in [suggest]."}
	}
	for _, name := range mapping.SortedAnyKeys(m) {
		if name == "text" {
			continue
		}
		spec, ok := m[name].(map[string]any)
		if !ok {
			continue
		}
		found := false
		for _, k := range mapping.SortedAnyKeys(spec) {
			switch k {
			case "text", "prefix", "regex":
				continue
			}
			if _, isObj := spec[k].(map[string]any); !isObj {
				continue
			}
			if !suggesterTypes[k] {
				return &apierr.Error{Type: "named_object_not_found_exception", Status: 400,
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
			return &apierr.Error{Type: "parse_exception", Status: 400, Reason: "missing suggestion object"}
		}
	}
	return nil
}

var completionSuggestOptions = map[string]bool{
	"field": true, "analyzer": true, "size": true, "shard_size": true,
	"skip_duplicates": true, "fuzzy": true, "regex": true, "contexts": true,
}

func validateCompletionOptions(body map[string]any) *apierr.Error {
	for _, k := range mapping.SortedAnyKeys(body) {
		if !completionSuggestOptions[k] {
			return &apierr.Error{Type: "x_content_parse_exception", Status: 400,
				Reason: "[completion] unknown field [" + k + "]"}
		}
	}
	for _, k := range []string{"size", "shard_size"} {
		if v, has := body[k]; has {
			if n, ok := jsonv.ToFloat(v); ok && n <= 0 {
				reason := "size must be positive"
				if k == "shard_size" {
					reason = "shardSize must be positive"
				}
				return &apierr.Error{Type: "x_content_parse_exception", Status: 400,
					Reason:   "[completion] failed to parse field [" + k + "]",
					CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: reason}}
			}
		}
	}
	if f, ok := jsonv.ToString(body["field"]); !ok || f == "" {
		return &apierr.Error{Type: "parse_exception", Status: 400, Reason: "the required field option [field] is missing"}
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
	for _, k := range mapping.SortedAnyKeys(resp) {
		if !seen[k] {
			out = append(out, ojson.KV{k, resp[k]})
		}
	}
	return out
}

// ---------------------------------------------------------------- mapping

// ---------------------------------------------------------------- indexing

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
				if f, ok := jsonv.ToFloat(wv); ok {
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
					ctx[k] = mapping.CompletionValueList(cv)
				}
			}
			for _, in := range mapping.CompletionValueList(t["input"]) {
				if s, ok := jsonv.ScalarString(in); ok {
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
func completionKey(toks []analysis.Token, preserveSep, preservePos bool) (string, bool) {
	var labels []string
	last := -1
	for _, t := range toks {
		if t.Pos <= last {
			continue // stacked token: keep the first path only
		}
		gap := t.Pos - last
		if !preservePos && gap > 1 {
			gap = 1
		}
		for i := 1; i < gap; i++ {
			labels = append(labels, "")
		}
		labels = append(labels, t.Term)
		last = t.Pos
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
		if n, ok := jsonv.ToFloat(m["min_length"]); ok {
			f.minLength = int(n)
		}
		if n, ok := jsonv.ToFloat(m["prefix_length"]); ok {
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
	if n, ok := jsonv.ToFloat(v); ok {
		return min(max(int(n), 0), 2)
	}
	s, _ := jsonv.ToString(v)
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

// fuzzyPrefixMatch returns the length of the shortest prefix of key within
// maxEdits of query (the first prefixLen units matching exactly), or -1. Like
// Lucene's FSTUtil.intersectPrefixPaths, matching stops at the first accepted
// prefix.
func fuzzyPrefixMatch(q, k []int, maxEdits, prefixLen int, transpose bool) int {
	p := min(prefixLen, len(q))
	if len(k) < p {
		return -1
	}
	for i := 0; i < p; i++ {
		if q[i] != k[i] {
			return -1
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
			return p + j
		}
	}
	return -1
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
	for _, e := range mapping.CompletionValueList(v) {
		switch t := e.(type) {
		case map[string]any:
			q := completionQueryCtx{boost: 1}
			q.value, _ = jsonv.ScalarString(t["context"])
			if b, ok := jsonv.ToFloat(t["boost"]); ok {
				q.boost = b
			}
			if p, ok := t["prefix"].(bool); ok {
				q.prefix = p
			}
			out = append(out, q)
		default:
			if s, ok := jsonv.ScalarString(e); ok {
				out = append(out, completionQueryCtx{value: s, boost: 1})
			}
		}
	}
	return out
}

func geoContextPoint(v any) (float64, float64, bool) {
	if s, ok := v.(string); ok && !strings.Contains(s, ",") {
		if lat, lon, ok := mapping.DecodeGeohash(s); ok {
			return lat, lon, true
		}
	}
	return search.GeoPointOf(v)
}

// geohashNeighbors returns the cells around a geohash at the same precision.
func geohashNeighbors(hash string) []string {
	lat, lon, ok := mapping.DecodeGeohash(hash)
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
			_, ok1 := jsonv.ToFloat(t[0])
			_, ok2 := jsonv.ToFloat(t[1])
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
			if p, ok := jsonv.ToFloat(m["precision"]); ok {
				precision = int(p)
			}
			if b, ok := jsonv.ToFloat(m["boost"]); ok {
				boost = b
			}
			for _, nb := range mapping.CompletionValueList(m["neighbours"]) {
				if n, ok := jsonv.ToFloat(nb); ok {
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
func indexedContexts(ctxs []mapping.CompletionContextDef, e completionEntry, doc map[string]any) map[string][]string {
	out := map[string][]string{}
	for _, cd := range ctxs {
		vals := append([]any{}, e.contexts[cd.Name]...)
		if cd.Path != "" {
			if pv, ok := mapping.FieldValue(doc, cd.Path); ok {
				vals = append(vals, mapping.CompletionValueList(pv)...)
			}
		}
		if cd.Typ == "geo" {
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
					out[cd.Name] = append(out[cd.Name], geohashEncode(lat, lon, cd.Precision))
				}
			}
			continue
		}
		for _, v := range vals {
			if s, ok := jsonv.ScalarString(v); ok {
				out[cd.Name] = append(out[cd.Name], s)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- the suggester

type completionHit struct {
	idx      *model.Index
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

func completionSuggest(spec, body map[string]any, indices []*model.Index, globalText any, searchBody map[string]any) ([]any, *apierr.Error) {
	if err := validateCompletionOptions(body); err != nil {
		return nil, err
	}
	field, _ := jsonv.ToString(body["field"])
	var first *model.Index
	if len(indices) > 0 {
		first = indices[0]
	}

	var text string
	var regex *regexp.Regexp
	hasText := false
	if rv, ok := spec["regex"]; ok {
		text, _ = jsonv.ScalarString(rv)
		hasText = true
		re, err := regexp.Compile("^(?s:" + text + ")")
		if err != nil {
			return nil, suggestShardError(first, "invalid regular expression ["+text+"]")
		}
		regex = re
	} else {
		for _, k := range []string{"prefix", "text"} {
			if v, ok := spec[k]; ok {
				text, _ = jsonv.ScalarString(v)
				hasText = true
				break
			}
		}
		if !hasText && globalText != nil {
			text, _ = jsonv.ScalarString(globalText)
			hasText = true
		}
	}
	if !hasText {
		return nil, suggestShardError(first, "The required text option is missing")
	}

	size := 5
	if n, ok := jsonv.ToFloat(body["size"]); ok {
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
		idx *model.Index
		pos int
		def map[string]any
	}
	var targets []target
	notCompletion := false
	for i, ix := range indices {
		def, ok := mapping.FieldDef(ix, field)
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

	includes, excludes, omitSource, _ := search.SourceFilterFromBody(searchBody)

	var hits []*completionHit
	for _, tg := range targets {
		ix, def := tg.idx, tg.def
		ctxs := mapping.CompletionContextDefs(def)
		qctx := map[string][]completionQueryCtx{}
		if raw, has := body["contexts"]; has {
			cm, _ := raw.(map[string]any)
			if len(ctxs) == 0 {
				return nil, suggestShardError(first, "suggester ["+field+"] doesn't expect any context")
			}
			for _, name := range mapping.SortedAnyKeys(cm) {
				var cd *mapping.CompletionContextDef
				for i := range ctxs {
					if ctxs[i].Name == name {
						cd = &ctxs[i]
					}
				}
				if cd == nil {
					return nil, suggestShardError(first, "Unknown context name ["+name+"], must be one of ["+mapping.CompletionContextNames(ctxs)+"]")
				}
				if cd.Typ == "geo" {
					qctx[name] = parseGeoQueryContexts(cm[name], cd.Precision)
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
		if n, ok := jsonv.ToFloat(def["max_input_length"]); ok && n > 0 {
			maxLen = int(n)
		}

		var qkey string
		var qunits []int
		if regex == nil {
			k, ok := completionKey(analysis.Tokens(mapping.AnalysisIndex(ix), searchAnalyzer, text), preserveSep, preservePos)
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
			v, ok := mapping.FieldValue(doc, field)
			if !ok {
				continue
			}
			var hit *completionHit
			for _, e := range completionEntriesOf(v) {
				input := analysis.TruncateUTF16(e.input, maxLen)
				key, ok := completionKey(analysis.Tokens(mapping.AnalysisIndex(ix), analyzer, input), preserveSep, preservePos)
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
					plen := len(qunits)
					if exact {
						if !strings.HasPrefix(key, qkey) {
							continue
						}
					} else if plen = fuzzyPrefixMatch(qunits, kunits, fuzzy.maxEdits, fuzzy.prefixLength, fuzzy.transpose); plen < 0 {
						continue
					}
					boost = float64(commonPrefixLen(qunits, kunits[:plen]))
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
						qs, asked := qctx[cd.Name]
						if !asked {
							continue
						}
						for _, val := range indexed[cd.Name] {
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
							ctxHits = append(ctxHits, completionCtxHit{cd.Name, val, sc})
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
		opt := ojson.Object{{"text", h.text}, {"_index", h.idx.Name}, {"_id", h.id},
			{"_score", search.ScoreJSON(float64(float32(h.score)))}}
		if !omitSource {
			src := h.idx.Docs[h.id]
			if len(includes) > 0 || len(excludes) > 0 {
				src = search.FilterSourceDoc(src, includes, excludes)
			}
			opt = append(opt, ojson.KV{"_source", src})
		}
		if len(h.contexts) > 0 {
			sort.SliceStable(h.contexts, func(i, j int) bool { return h.contexts[i].score > h.contexts[j].score })
			ctxOut := ojson.Object{}
			for _, ch := range h.contexts {
				pos := ctxOut.Index(ch.name)
				if pos < 0 {
					ctxOut = append(ctxOut, ojson.KV{ch.name, []any{ch.value}})
					continue
				}
				vals := ctxOut[pos].V.([]any)
				dup := false
				for _, x := range vals {
					if x == ch.value {
						dup = true
					}
				}
				if !dup {
					ctxOut[pos].V = append(vals, ch.value)
				}
			}
			opt = append(opt, ojson.KV{"contexts", ctxOut})
		}
		options = append(options, opt)
	}
	entry := ojson.Object{{"text", text}, {"offset", 0}, {"length", analysis.UTF16Len([]rune(text))}, {"options", options}}
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
