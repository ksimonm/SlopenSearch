package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// time helpers used by date range queries
// ---------------------------------------------------------------------------

func timeLoadLocation(name string) (*time.Location, error) { return time.LoadLocation(name) }

func timeNowIn(loc *time.Location) (time.Time, int) {
	now := time.Now().In(loc)
	_, off := now.Zone()
	return now, off
}

// ---------------------------------------------------------------------------
// multi_match / combined_fields
// ---------------------------------------------------------------------------

func stripFieldBoost(f string) (string, float64) {
	if i := strings.Index(f, "^"); i >= 0 {
		b, err := strconv.ParseFloat(f[i+1:], 64)
		if err == nil {
			return f[:i], b
		}
		return f[:i], 1
	}
	return f, 1
}

func (c *queryCtx) fieldMatchedCount(field string, terms []string) int {
	vals, ok := c.values(field)
	if !ok {
		return 0
	}
	analyzer := c.indexAnalyzerName(field)
	tf := map[string]int{}
	for _, v := range vals {
		s, _ := valueScalarString(v)
		for _, t := range analyzeTokensWithIndex(c.idx, analyzer, s) {
			tf[t.term]++
		}
	}
	n := 0
	for _, t := range terms {
		if tf[t] > 0 {
			n++
		}
	}
	return n
}

func (c *queryCtx) bm25FieldScore(field string, qtokens []string, operator string, boost float64) float64 {
	if c.idx == nil || c.id == "" {
		return -1
	}
	def, _ := mappingFieldDef(c.idx, field)
	ftype, _ := def["type"].(string)
	if ftype != "" && ftype != "text" && ftype != "match_only_text" && ftype != "search_as_you_type" {
		return -1
	}
	analyzer := c.indexAnalyzerName(field)
	docTerms := func(vals []any) []string {
		out := []string{}
		for _, v := range vals {
			s, _ := valueScalarString(v)
			for _, t := range analyzeTokensWithIndex(c.idx, analyzer, s) {
				out = append(out, t.term)
			}
		}
		return out
	}
	var myTokens []string
	var st *fieldTermStats
	if c.vals == nil {
		st = termStatsFor(c.idx, resolveAliasField(c.idx, field), analyzer, docTerms)
		myTokens = st.docTokens[c.id]
	} else {
		myTokens = docTerms(c.vals[resolveAliasField(c.idx, field)])
	}
	if len(myTokens) == 0 {
		return 0
	}
	tf := map[string]int{}
	for _, t := range myTokens {
		tf[t]++
	}
	for _, qt := range qtokens {
		if tf[qt] == 0 {
			if operator == "and" {
				return 0
			}
		}
	}
	if st == nil {
		st = termStatsFor(c.idx, resolveAliasField(c.idx, field), analyzer, docTerms)
	}
	N := st.n
	df := st.df
	if N == 0 {
		return 0
	}
	avgdl := st.sumdl / float64(N)
	if avgdl == 0 {
		avgdl = 1
	}
	const k1 = 1.2
	bParam := 0.75
	if v, ok := def["norms"].(bool); ok && !v {
		bParam = 0
	}
	docsOnly := false
	if io, _ := def["index_options"].(string); io == "docs" {
		docsOnly = true
	}
	dl := float64(len(myTokens))
	score := 0.0
	for _, qt := range qtokens {
		f := float64(tf[qt])
		if f == 0 {
			continue
		}
		if docsOnly {
			f = 1
		}
		d := float64(df[qt])
		idf := math.Log(1 + (float64(N)-d+0.5)/(d+0.5))
		score += idf * f / (f + k1*(1-bParam+bParam*dl/avgdl))
	}
	return score * boost
}

func (c *queryCtx) evalMultiMatch(bm map[string]any) (bool, float64, *apiError) {
	query, _ := toString(bm["query"])
	if query == "" {
		return false, 0, errParsing("[multi_match] requires 'query'")
	}
	typ, _ := bm["type"].(string)
	if typ == "" {
		typ = "best_fields"
	}
	switch typ {
	case "best_fields", "most_fields", "cross_fields", "phrase", "phrase_prefix", "bool_prefix":
	default:
		return false, 0, &apiError{Type: "parse_exception", Status: 400,
			Reason: "failed to parse [multi_match] query type [" + typ + "]. unknown type."}
	}
	var fields []string
	switch t := bm["fields"].(type) {
	case []any:
		for _, e := range t {
			if s, ok := toString(e); ok {
				fields = append(fields, s)
			}
		}
	case string:
		fields = []string{t}
	}
	boost := 1.0
	if b, ok := toFloat(bm["boost"]); ok {
		boost *= b
	}
	tie := 0.0
	if t, ok := toFloat(bm["tie_breaker"]); ok {
		tie = t
	}
	if len(fields) == 0 {
		fields = []string{"*"}
	}
	expanded := []string{}
	fromWildcard := map[string]bool{}
	for _, f := range fields {
		name, _ := stripFieldBoost(f)
		if strings.ContainsAny(name, "*?") {
			re := wildcardRegex(name)
			for path, vals := range c.valuesMap() {
				if re.MatchString(path) && len(vals) > 0 {
					expanded = append(expanded, path)
					fromWildcard[path] = true
				}
			}
			continue
		}
		expanded = append(expanded, f)
	}
	best := 0.0
	sum := 0.0
	matchedAny := false
	switch typ {
	case "phrase", "phrase_prefix":
		for _, f := range expanded {
			name, fb := stripFieldBoost(f)
			var ok bool
			if typ == "phrase" {
				slop := 0.0
				if v, has := bm["slop"]; has {
					if f, ok := toFloat(v); ok {
						slop = f
					}
				}
				ok, _, _ = c.evalMatchPhrase(name, query, boost*fb, slop)
			} else {
				bm2 := map[string]any{name: map[string]any{"query": query, "boost": boost * fb}}
				q := map[string]any{"match_phrase_prefix": bm2}
				ok, _, _ = c.eval(q)
			}
			if ok {
				matchedAny = true
				sum += fb
			}
		}
	case "bool_prefix":
		for _, f := range expanded {
			name, fb := stripFieldBoost(f)
			ok, sc, err := c.evalMatchBoolPrefix(name, query, boost*fb)
			if err != nil {
				return false, 0, err
			}
			if ok {
				matchedAny = true
				sum += sc
			}
		}
	case "cross_fields":
		if _, has := bm["fuzziness"]; has {
			return false, 0, errParsing("Fuzziness not allowed for type [cross_fields]")
		}
		terms := analyze(query)
		operator, _ := bm["operator"].(string)
		operator = strings.ToLower(operator)
		required := 1
		if operator == "and" {
			required = len(terms)
		}
		if v, has := bm["minimum_should_match"]; has {
			req, merr := msmRequired(v, len(terms), operator == "and")
			if merr != nil {
				return false, 0, merr
			}
			required = req
		}
		matchedTerms := 0
		for _, term := range terms {
			bestTerm := 0.0
			for _, f := range expanded {
				name, fb := stripFieldBoost(f)
				if sc := c.bm25FieldScore(name, []string{term}, "or", boost*fb); sc > bestTerm {
					bestTerm = sc
				}
			}
			if bestTerm > 0 {
				matchedTerms++
				sum += bestTerm
			}
		}
		if matchedTerms >= required {
			matchedAny = true
		}
	default:
		operator, _ := bm["operator"].(string)
		operator = strings.ToLower(operator)
		fuzz, hasFuzz := bm["fuzziness"]
		for _, f := range expanded {
			name, fb := stripFieldBoost(f)
			if fromWildcard[name] {
				if def, ok := mappingFieldDef(c.idx, name); ok {
					ft, _ := def["type"].(string)
					if ft != "" && ft != "text" && ft != "keyword" && ft != "constant_keyword" &&
						ft != "wildcard" && ft != "match_only_text" && ft != "search_as_you_type" && ft != "flat_object" {
						continue
					}
				}
			}
			var sc float64
			ok := false
			usedBM25 := false
			if !hasFuzz {
				terms := c.analyzeField(name, bm, query)
				required := 1
				if operator == "and" {
					required = len(terms)
				}
				if v, has := bm["minimum_should_match"]; has {
					req, merr := msmRequired(v, len(terms), operator == "and")
					if merr != nil {
						return false, 0, merr
					}
					required = req
					if len(terms) == 1 {
						required = 1
					}
				}
				if required > 1 && c.fieldMatchedCount(name, terms) < required {
					continue
				}
				if bm25 := c.bm25FieldScore(name, terms, operator, boost*fb); bm25 >= 0 {
					ok = bm25 > 0
					sc = bm25
					usedBM25 = true
				}
			}
			if !usedBM25 {
				opts := map[string]any{"query": query}
				if operator != "" {
					opts["operator"] = operator
				}
				if hasFuzz {
					opts["fuzziness"] = fuzz
				}
				if v, has := bm["lenient"]; has {
					opts["lenient"] = v
				}
				var err *apiError
				ok, sc, err = c.evalMatch(name, opts, boost*fb)
				if err != nil {
					return false, 0, err
				}
			}
			if ok {
				matchedAny = true
				if sc > best {
					best = sc
				}
				sum += sc
			}
		}
	}
	if !matchedAny {
		return false, 0, nil
	}
	switch typ {
	case "most_fields", "cross_fields", "bool_prefix":
		return true, sum, nil
	default:
		if tie > 0 {
			return true, best + tie*(sum-best), nil
		}
		return true, best, nil
	}
}

func (c *queryCtx) evalCombinedFields(bm map[string]any) (bool, float64, *apiError) {
	query, _ := toString(bm["query"])
	var fields []string
	switch t := bm["fields"].(type) {
	case []any:
		for _, e := range t {
			if s, ok := toString(e); ok {
				fields = append(fields, s)
			}
		}
	case string:
		fields = []string{t}
	}
	if len(fields) == 0 {
		return false, 0, nil
	}
	expanded := []string{}
	for _, f := range fields {
		name, fb := stripFieldBoost(f)
		if fb < 1 {
			reason := "weight must be greater or equal to 1"
			return false, 0, matchSearchError("query_shard_exception", reason)
		}
		if strings.ContainsAny(name, "*?") {
			re := wildcardRegex(name)
			for path, vals := range c.valuesMap() {
				if re.MatchString(path) && len(vals) > 0 {
					expanded = append(expanded, f)
					break
				}
			}
			re2 := wildcardRegex(name)
			for path := range c.valuesMap() {
				if re2.MatchString(path) && !containsString(expanded, path) {
					expanded = append(expanded, path)
				}
			}
			continue
		}
		expanded = append(expanded, f)
	}
	seen := map[string]bool{}
	finalFields := []string{}
	for _, f := range expanded {
		name, _ := stripFieldBoost(f)
		if seen[name] {
			continue
		}
		seen[name] = true
		finalFields = append(finalFields, f)
	}
	expanded = finalFields
	for _, f := range expanded {
		name, _ := stripFieldBoost(f)
		def, ok := mappingFieldDef(c.idx, name)
		if !ok {
			continue
		}
		ft, _ := def["type"].(string)
		if ft != "" && ft != "text" && ft != "match_only_text" && ft != "search_as_you_type" {
			return false, 0, matchSearchError("query_shard_exception",
				"Field ["+name+"] of type ["+ft+"] does not support [combined_fields] queries")
		}
	}
	terms := analyze(query)
	if len(terms) == 0 {
		return false, 0, nil
	}
	operator, _ := bm["operator"].(string)
	operator = strings.ToLower(operator)
	required := 1
	if operator == "and" {
		required = len(terms)
	}
	if v, has := bm["minimum_should_match"]; has {
		req, merr := msmRequired(v, len(terms), operator == "and")
		if merr != nil {
			return false, 0, merr
		}
		required = req
	}
	matchedTerms := 0
	total := 0.0
	for _, term := range terms {
		bestTerm := 0.0
		for _, f := range expanded {
			name, fb := stripFieldBoost(f)
			if sc := c.bm25FieldScore(name, []string{term}, "or", fb); sc > bestTerm {
				bestTerm = sc
			}
		}
		if bestTerm > 0 {
			matchedTerms++
			total += bestTerm
		}
	}
	if matchedTerms >= required {
		return true, total, nil
	}
	return false, 0, nil
}

func containsString(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// nested / join queries
// ---------------------------------------------------------------------------

func (c *queryCtx) evalNested(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[nested] query malformed")
	}
	inner, ok := bm["query"].(map[string]any)
	if !ok {
		return false, 0, errParsing("[nested] requires 'query'")
	}
	path, _ := bm["path"].(string)
	scoreMode, _ := bm["score_mode"].(string)
	if scoreMode == "" {
		scoreMode = "avg"
	}
	ignoreUnmapped, _ := bm["ignore_unmapped"].(bool)
	def, mapped := mappingFieldDef(c.idx, path)
	if !mapped {
		if ignoreUnmapped {
			return false, 0, nil
		}
		return false, 0, matchSearchError("query_shard_exception",
			"failed to create query: [nested] failed to find nested object under path ["+path+"]")
	}
	if t, _ := def["type"].(string); t != "nested" {
		return false, 0, matchSearchError("query_shard_exception",
			"failed to create query: [nested] nested object under path ["+path+"] is not of nested type")
	}
	objects, ok := nestedObjectsForCtx(c, path)
	if !ok || len(objects) == 0 {
		return false, 0, nil
	}
	found := false
	total := 0.0
	count := 0
	best := 0.0
	worst := 0.0
	parentMeta := c.nestedMeta()
	var parentNested any
	if parentMeta != nil {
		parentNested = parentMeta
	}
	for i, obj := range objects {
		child := &queryCtx{idx: c.idx, id: c.id, doc: c.doc, vals: c.vals,
			nestedPath: path, nestedRoot: obj, nestedOffset: i, nestedParent: parentNested}
		matched, sc, err := child.eval(inner)
		if err != nil {
			return false, 0, err
		}
		if !matched {
			continue
		}
		if !found {
			best, worst = sc, sc
		} else {
			if sc > best {
				best = sc
			}
			if sc < worst {
				worst = sc
			}
		}
		found = true
		total += sc
		count++
	}
	if !found {
		return false, 0, nil
	}
	switch scoreMode {
	case "none":
		return true, 0, nil
	case "max":
		return true, best, nil
	case "min":
		return true, worst, nil
	case "sum":
		return true, total, nil
	case "avg":
		return true, total / float64(count), nil
	}
	return true, total / float64(count), nil
}

func nestedObjectsForCtx(c *queryCtx, path string) ([]map[string]any, bool) {
	if c.nestedRoot != nil {
		lookup := path
		if path == c.nestedPath {
			lookup = ""
		} else if strings.HasPrefix(path, c.nestedPath+".") {
			lookup = strings.TrimPrefix(path, c.nestedPath+".")
		} else {
			return nil, false
		}
		return nestedObjectsAt(c.nestedRoot, lookup), true
	}
	return nestedObjectsAt(c.doc, path), true
}

func nestedSourceFilter(obj map[string]any, path string, spec any) any {
	if spec == nil {
		return obj
	}
	if b, ok := spec.(bool); ok {
		if b {
			return obj
		}
		return nil
	}
	var includes []string
	switch t := spec.(type) {
	case string:
		includes = append(includes, t)
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok {
				includes = append(includes, s)
			}
		}
	}
	if len(includes) == 0 {
		return obj
	}
	res := map[string]any{}
	for _, inc := range includes {
		rel := inc
		if strings.HasPrefix(rel, path+".") {
			rel = strings.TrimPrefix(rel, path+".")
		}
		vals, ok := nestedObjectValues(obj, rel)
		if !ok || len(vals) == 0 {
			continue
		}
		parts := strings.Split(rel, ".")
		cur := res
		for i, part := range parts {
			if i == len(parts)-1 {
				cur[part] = vals[0]
			} else {
				next, ok := cur[part].(map[string]any)
				if !ok {
					next = map[string]any{}
					cur[part] = next
				}
				cur = next
			}
		}
	}
	return res
}

func nestedSortCompare(a, b any) int {
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if aok && bok {
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		default:
			return 0
		}
	}
	as, _ := valueScalarString(a)
	bs, _ := valueScalarString(b)
	return strings.Compare(as, bs)
}

func nestedSortOutput(obj map[string]any, path string, sp searchSortSpec) any {
	if sp.field == "_score" {
		return nil
	}
	rel := sp.field
	if strings.HasPrefix(rel, path+".") {
		rel = strings.TrimPrefix(rel, path+".")
	}
	vals, ok := nestedObjectValues(obj, rel)
	if !ok || len(vals) == 0 {
		return nil
	}
	return vals[0]
}

func collectNestedInnerHits(c *queryCtx, query map[string]any, omitScores bool) map[string]any {
	out := map[string]any{}
	collectNestedInnerHitsClause(c, query, omitScores, out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectNestedInnerHitsClause(c *queryCtx, query map[string]any, omitScores bool, out map[string]any) {
	for clause, raw := range query {
		bm, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch clause {
		case "nested":
			path, _ := bm["path"].(string)
			inner, _ := bm["query"].(map[string]any)
			if inner == nil {
				continue
			}
			spec, has := bm["inner_hits"]
			if has {
				collectOneNestedInnerHits(c, path, inner, spec, omitScores, out)
			}
		case "bool":
			for _, key := range []string{"must", "should", "filter", "must_not"} {
				for _, q := range asQueryList(bm[key]) {
					if qm, ok := q.(map[string]any); ok {
						collectNestedInnerHitsClause(c, qm, omitScores, out)
					}
				}
			}
		case "constant_score":
			if qm, ok := bm["filter"].(map[string]any); ok {
				collectNestedInnerHitsClause(c, qm, omitScores, out)
			}
		case "dis_max":
			for _, q := range asQueryList(bm["queries"]) {
				if qm, ok := q.(map[string]any); ok {
					collectNestedInnerHitsClause(c, qm, omitScores, out)
				}
			}
		case "boosting":
			for _, key := range []string{"positive", "negative"} {
				if qm, ok := bm[key].(map[string]any); ok {
					collectNestedInnerHitsClause(c, qm, omitScores, out)
				}
			}
		case "function_score", "script_score":
			if qm, ok := bm["query"].(map[string]any); ok {
				collectNestedInnerHitsClause(c, qm, omitScores, out)
			}
		}
	}
}

func collectOneNestedInnerHits(c *queryCtx, path string, inner map[string]any, spec any, omitScores bool, out map[string]any) {
	objects, ok := nestedObjectsForCtx(c, path)
	if !ok {
		return
	}
	name := path
	specMap, _ := spec.(map[string]any)
	if specMap == nil {
		specMap = map[string]any{}
	}
	if n, _ := specMap["name"].(string); n != "" {
		name = n
	}
	from := 0
	if n, ok := toFloat(specMap["from"]); ok {
		from = int(n)
	}
	size := 3
	if n, ok := toFloat(specMap["size"]); ok {
		size = int(n)
	}
	type nestedInnerHit struct {
		score    float64
		entry    map[string]any
		obj      map[string]any
		sortVals []any
	}
	var matched []nestedInnerHit
	parentMeta := c.nestedMeta()
	var parentNested any
	if parentMeta != nil {
		parentNested = parentMeta
	}
	for i, obj := range objects {
		child := &queryCtx{idx: c.idx, id: c.id, doc: c.doc, vals: c.vals,
			nestedPath: path, nestedRoot: obj, nestedOffset: i, nestedParent: parentNested}
		ok, sc, err := child.eval(inner)
		if err != nil || !ok {
			continue
		}
		entry := map[string]any{
			"_index":  c.idx.Name,
			"_id":     c.id,
			"_nested": child.nestedMeta(),
		}
		src := nestedSourceFilter(obj, path, specMap["_source"])
		if src != nil {
			entry["_source"] = src
		}
		if omitScores {
			entry["_score"] = nil
		} else {
			entry["_score"] = scoreJSON(sc)
		}
		if deeper := collectNestedInnerHits(child, inner, omitScores); len(deeper) > 0 {
			entry["inner_hits"] = deeper
		}
		matched = append(matched, nestedInnerHit{score: sc, entry: entry, obj: obj})
	}
	if rawSort, has := specMap["sort"]; has {
		if specs, err := parseSortSpecs(rawSort, ""); err == nil && len(specs) > 0 {
			for i := range matched {
				vals := make([]any, 0, len(specs))
				for _, sp := range specs {
					vals = append(vals, nestedSortOutput(matched[i].obj, path, sp))
				}
				matched[i].sortVals = vals
				matched[i].entry["sort"] = vals
			}
			sort.SliceStable(matched, func(i, j int) bool {
				for k, sp := range specs {
					cmp := nestedSortCompare(matched[i].sortVals[k], matched[j].sortVals[k])
					if cmp != 0 {
						if sp.desc {
							return cmp > 0
						}
						return cmp < 0
					}
				}
				return false
			})
		}
	}
	total := len(matched)
	if from > len(matched) {
		from = len(matched)
	}
	end := from + size
	if size < 0 || end > len(matched) {
		end = len(matched)
	}
	if end < from {
		end = from
	}
	hits := make([]any, 0, end-from)
	for _, m := range matched[from:end] {
		hits = append(hits, m.entry)
	}
	var maxScore any
	if !omitScores && len(matched) > 0 {
		m := matched[0].score
		for _, x := range matched[1:] {
			if x.score > m {
				m = x.score
			}
		}
		maxScore = scoreJSON(m)
	}
	out[name] = map[string]any{"hits": map[string]any{
		"total":     map[string]any{"value": total, "relation": "eq"},
		"max_score": maxScore,
		"hits":      hits,
	}}
}

func findJoinField(idx *Index) (string, map[string]any, bool) {
	props, _ := idx.Mappings["properties"].(map[string]any)
	for name, raw := range props {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := def["type"].(string); t == "join" {
			relations, _ := def["relations"].(map[string]any)
			return name, relations, true
		}
	}
	return "", nil, false
}

func joinParts(v any) (string, string) {
	switch t := v.(type) {
	case string:
		return t, ""
	case map[string]any:
		name, _ := t["name"].(string)
		parent, _ := t["parent"].(string)
		return name, parent
	}
	return "", ""
}

func joinRelationChildren(relations map[string]any, parent string) []string {
	switch t := relations[parent].(type) {
	case string:
		return []string{t}
	case []any:
		var out []string
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func joinValidName(relations map[string]any, name string) bool {
	if _, ok := relations[name]; ok {
		return true
	}
	for parent := range relations {
		for _, child := range joinRelationChildren(relations, parent) {
			if child == name {
				return true
			}
		}
	}
	return false
}

func joinParentRelation(relations map[string]any, child string) string {
	for parent := range relations {
		for _, c := range joinRelationChildren(relations, parent) {
			if c == child {
				return parent
			}
		}
	}
	return ""
}

func joinError(clause, reason string) *apiError {
	return matchSearchError("query_shard_exception", "["+clause+"] "+reason)
}

func (c *queryCtx) joinChildContexts(childType string, inner map[string]any) ([]*queryCtx, []float64, *apiError) {
	var ctxs []*queryCtx
	var scores []float64
	for _, id := range c.idx.Order {
		doc, live := c.idx.Docs[id]
		if !live {
			continue
		}
		name, parent := joinParts(doc[fieldOfJoin(c.idx)])
		if parent != c.id {
			continue
		}
		if childType != "" && name != childType {
			continue
		}
		cc := newQueryCtxForDoc(c.idx, id, doc)
		ok, sc, err := cc.eval(inner)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			ctxs = append(ctxs, cc)
			scores = append(scores, sc)
		}
	}
	return ctxs, scores, nil
}

func fieldOfJoin(idx *Index) string {
	field, _, _ := findJoinField(idx)
	return field
}

func (c *queryCtx) evalHasChild(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[has_child] query malformed")
	}
	childType, _ := bm["type"].(string)
	inner, ok := bm["query"].(map[string]any)
	if !ok {
		return false, 0, errParsing("[has_child] requires 'query'")
	}
	ignoreUnmapped, _ := bm["ignore_unmapped"].(bool)
	field, relations, hasJoin := findJoinField(c.idx)
	if !hasJoin {
		if ignoreUnmapped {
			return false, 0, nil
		}
		return false, 0, joinError("has_child", "no join field has been configured")
	}
	if childType != "" && !joinValidName(relations, childType) {
		return false, 0, joinError("has_child",
			"join field ["+field+"] doesn't hold ["+childType+"] as a child")
	}
	ctxs, scores, err := c.joinChildContexts(childType, inner)
	if err != nil {
		return false, 0, err
	}
	count := len(ctxs)
	if minRaw, ok := toFloat(bm["min_children"]); ok && count < int(minRaw) {
		return false, 0, nil
	}
	if maxRaw, ok := toFloat(bm["max_children"]); ok && count > int(maxRaw) {
		return false, 0, nil
	}
	if count == 0 {
		return false, 0, nil
	}
	scoreMode, _ := bm["score_mode"].(string)
	if scoreMode == "" {
		scoreMode = "none"
	}
	switch scoreMode {
	case "max":
		best := scores[0]
		for _, s := range scores[1:] {
			if s > best {
				best = s
			}
		}
		return true, best, nil
	case "min":
		worst := scores[0]
		for _, s := range scores[1:] {
			if s < worst {
				worst = s
			}
		}
		return true, worst, nil
	case "sum":
		total := 0.0
		for _, s := range scores {
			total += s
		}
		return true, total, nil
	case "avg":
		total := 0.0
		for _, s := range scores {
			total += s
		}
		return true, total / float64(count), nil
	}
	return true, 1, nil
}

func (c *queryCtx) evalHasParent(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[has_parent] query malformed")
	}
	parentType, _ := bm["parent_type"].(string)
	if parentType == "" {
		parentType, _ = bm["type"].(string)
	}
	inner, ok := bm["query"].(map[string]any)
	if !ok {
		return false, 0, errParsing("[has_parent] requires 'query'")
	}
	ignoreUnmapped, _ := bm["ignore_unmapped"].(bool)
	field, relations, hasJoin := findJoinField(c.idx)
	if !hasJoin {
		if ignoreUnmapped {
			return false, 0, nil
		}
		return false, 0, joinError("has_parent", "no join field has been configured")
	}
	if parentType != "" && !joinValidName(relations, parentType) {
		return false, 0, joinError("has_parent",
			"join field ["+field+"] doesn't hold ["+parentType+"] as a parent")
	}
	parentID := c.idx.Routing[c.id]
	if parentID == "" {
		_, parentID = joinParts(c.doc[field])
	}
	if parentID == "" {
		return false, 0, nil
	}
	doc, live := c.idx.Docs[parentID]
	if !live {
		return false, 0, nil
	}
	name, _ := joinParts(doc[field])
	if parentType != "" && name != parentType {
		return false, 0, nil
	}
	cc := newQueryCtxForDoc(c.idx, parentID, doc)
	ok, sc, err := cc.eval(inner)
	if err != nil {
		return false, 0, err
	}
	if !ok {
		return false, 0, nil
	}
	if scoreFlag, _ := bm["score"].(bool); scoreFlag {
		return true, sc, nil
	}
	return true, 1, nil
}

func (c *queryCtx) evalParentId(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[parent_id] query malformed")
	}
	childType, _ := bm["type"].(string)
	parentID, _ := bm["id"].(string)
	ignoreUnmapped, _ := bm["ignore_unmapped"].(bool)
	field, relations, hasJoin := findJoinField(c.idx)
	if !hasJoin {
		if ignoreUnmapped {
			return false, 0, nil
		}
		return false, 0, joinError("parent_id", "no join field has been configured")
	}
	name, parent := joinParts(c.doc[field])
	if name != childType || parent != parentID {
		return false, 0, nil
	}
	parentType := joinParentRelation(relations, childType)
	df := 0
	total := 0
	for _, id := range c.idx.Order {
		doc, live := c.idx.Docs[id]
		if !live {
			continue
		}
		n, p := joinParts(doc[field])
		if n == "" {
			continue
		}
		total++
		if n == parentType && id == parentID {
			df++
		}
		if p == parentID && n != "" {
			df++
		}
	}
	score := 1.0
	if total > 0 && df > 0 {
		idf := math.Log(1 + (float64(total)-float64(df)+0.5)/(float64(df)+0.5))
		score = idf / (1 + 1.2)
	}
	return true, score, nil
}

func joinInnerSource(doc map[string]any, spec any) map[string]any {
	body := map[string]any{"_source": spec}
	inc, exc, omit, ok := sourceFilterFromBody(body)
	if !ok || omit {
		if omit {
			return nil
		}
		return doc
	}
	return filterSourceDoc(doc, inc, exc)
}

func joinSortOutput(ctx *queryCtx, sp searchSortSpec) any {
	if sp.field == "_score" {
		return nil
	}
	vals, ok := ctx.values(sp.field)
	if !ok || len(vals) == 0 {
		return nil
	}
	return vals[0]
}

func joinInnerHitEntry(c *queryCtx, ctx *queryCtx, doc map[string]any, spec map[string]any, sortSpecs []searchSortSpec, score float64, includeRouting bool) (map[string]any, []any) {
	entry := map[string]any{
		"_index": c.idx.Name,
		"_id":    ctx.id,
	}
	if includeRouting {
		if rv := c.idx.Routing[ctx.id]; rv != "" {
			entry["_routing"] = rv
		}
	}
	var sortVals []any
	if len(sortSpecs) > 0 {
		sortVals = make([]any, 0, len(sortSpecs))
		for _, sp := range sortSpecs {
			sortVals = append(sortVals, joinSortOutput(ctx, sp))
		}
		entry["_score"] = nil
	} else {
		entry["_score"] = scoreJSON(score)
	}
	src := joinInnerSource(doc, spec["_source"])
	if src != nil {
		entry["_source"] = src
	}
	if len(sortVals) > 0 {
		entry["sort"] = sortVals
	}
	return entry, sortVals
}

func joinInnerHitsSortAndPage(matched []map[string]any, sortVals [][]any, specs []searchSortSpec, from, size int) []any {
	if len(specs) > 0 {
		idx := make([]int, len(matched))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(i, j int) bool {
			a, b := idx[i], idx[j]
			for k, sp := range specs {
				cmp := nestedSortCompare(sortVals[a][k], sortVals[b][k])
				if cmp != 0 {
					if sp.desc {
						return cmp > 0
					}
					return cmp < 0
				}
			}
			return false
		})
		sortedHits := make([]map[string]any, len(matched))
		sortedVals := make([][]any, len(sortVals))
		for i, j := range idx {
			sortedHits[i] = matched[j]
			sortedVals[i] = sortVals[j]
		}
		matched, sortVals = sortedHits, sortedVals
	}
	if from > len(matched) {
		from = len(matched)
	}
	end := from + size
	if size < 0 || end > len(matched) {
		end = len(matched)
	}
	if end < from {
		end = from
	}
	hits := make([]any, 0, end-from)
	for _, h := range matched[from:end] {
		hits = append(hits, h)
	}
	return hits
}

func joinInnerHitsMaxScore(scores []float64, sorted bool) any {
	if sorted || len(scores) == 0 {
		return nil
	}
	m := scores[0]
	for _, s := range scores[1:] {
		if s > m {
			m = s
		}
	}
	return scoreJSON(m)
}

func collectJoinInnerHits(c *queryCtx, query map[string]any, omitScores bool) map[string]any {
	out := map[string]any{}
	collectJoinInnerHitsClause(c, query, omitScores, out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectJoinInnerHitsClause(c *queryCtx, query map[string]any, omitScores bool, out map[string]any) {
	for clause, raw := range query {
		bm, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch clause {
		case "has_child", "has_parent":
			if spec, has := bm["inner_hits"]; has {
				collectOneJoinInnerHits(c, clause, bm, spec, out)
			}
		case "bool":
			for _, key := range []string{"must", "should", "filter", "must_not"} {
				for _, q := range asQueryList(bm[key]) {
					if qm, ok := q.(map[string]any); ok {
						collectJoinInnerHitsClause(c, qm, omitScores, out)
					}
				}
			}
		case "constant_score":
			if qm, ok := bm["filter"].(map[string]any); ok {
				collectJoinInnerHitsClause(c, qm, omitScores, out)
			}
		case "dis_max":
			for _, q := range asQueryList(bm["queries"]) {
				if qm, ok := q.(map[string]any); ok {
					collectJoinInnerHitsClause(c, qm, omitScores, out)
				}
			}
		case "boosting":
			for _, key := range []string{"positive", "negative"} {
				if qm, ok := bm[key].(map[string]any); ok {
					collectJoinInnerHitsClause(c, qm, omitScores, out)
				}
			}
		case "function_score", "script_score":
			if qm, ok := bm["query"].(map[string]any); ok {
				collectJoinInnerHitsClause(c, qm, omitScores, out)
			}
		}
	}
}

func collectOneJoinInnerHits(c *queryCtx, clause string, bm map[string]any, spec any, out map[string]any) {
	specMap, _ := spec.(map[string]any)
	if specMap == nil {
		specMap = map[string]any{}
	}
	sortSpecs, _ := parseSortSpecs(specMap["sort"], "")
	from := 0
	if n, ok := toFloat(specMap["from"]); ok {
		from = int(n)
	}
	size := 3
	if n, ok := toFloat(specMap["size"]); ok {
		size = int(n)
	}
	var name string
	var hits []map[string]any
	var sortVals [][]any
	var scores []float64
	if clause == "has_child" {
		childType, _ := bm["type"].(string)
		inner, _ := bm["query"].(map[string]any)
		name = childType
		ctxs, cscores, err := c.joinChildContexts(childType, inner)
		if err != nil {
			return
		}
		for i, ctx := range ctxs {
			entry, sv := joinInnerHitEntry(c, ctx, ctx.doc, specMap, sortSpecs, cscores[i], true)
			hits = append(hits, entry)
			sortVals = append(sortVals, sv)
			scores = append(scores, cscores[i])
		}
	} else {
		parentType, _ := bm["parent_type"].(string)
		if parentType == "" {
			parentType, _ = bm["type"].(string)
		}
		inner, _ := bm["query"].(map[string]any)
		name = parentType
		field, _, hasJoin := findJoinField(c.idx)
		if !hasJoin {
			return
		}
		parentID := c.idx.Routing[c.id]
		if parentID == "" {
			_, parentID = joinParts(c.doc[field])
		}
		if parentID == "" {
			return
		}
		doc, live := c.idx.Docs[parentID]
		if !live {
			return
		}
		pctx := newQueryCtxForDoc(c.idx, parentID, doc)
		ok, sc, err := pctx.eval(inner)
		if err != nil || !ok {
			return
		}
		entry, sv := joinInnerHitEntry(c, pctx, doc, specMap, sortSpecs, sc, false)
		hits = append(hits, entry)
		sortVals = append(sortVals, sv)
		scores = append(scores, sc)
	}
	if n, _ := specMap["name"].(string); n != "" {
		name = n
	}
	total := len(hits)
	page := joinInnerHitsSortAndPage(hits, sortVals, sortSpecs, from, size)
	var maxScore any
	if clause == "has_child" {
		maxScore = joinInnerHitsMaxScore(scores, len(sortSpecs) > 0)
	} else if len(sortSpecs) > 0 {
		maxScore = nil
	} else {
		maxScore = joinInnerHitsMaxScore(scores, false)
	}
	out[name] = map[string]any{"hits": map[string]any{
		"total":     map[string]any{"value": total, "relation": "eq"},
		"max_score": maxScore,
		"hits":      page,
	}}
}

// ---------------------------------------------------------------------------
// geo queries
// ---------------------------------------------------------------------------

func geoPointOf(v any) (float64, float64, bool) {
	switch t := v.(type) {
	case map[string]any:
		lat, ok1 := toFloat(t["lat"])
		lon, ok2 := toFloat(t["lon"])
		if ok1 && ok2 {
			return lat, lon, true
		}
		if typ, _ := t["type"].(string); strings.EqualFold(typ, "point") {
			if coords, ok := t["coordinates"].([]any); ok && len(coords) >= 2 {
				lon, ok1 := toFloat(coords[0])
				lat, ok2 := toFloat(coords[1])
				if ok1 && ok2 {
					return lat, lon, true
				}
			}
		}
	case []any:
		if len(t) == 2 {
			lon, ok1 := toFloat(t[0])
			lat, ok2 := toFloat(t[1])
			if ok1 && ok2 {
				return lat, lon, true
			}
		}
	case string:
		parts := strings.Split(t, ",")
		if len(parts) == 2 {
			lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			lon, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if err1 == nil && err2 == nil {
				return lat, lon, true
			}
		}
		u := strings.ToUpper(strings.TrimSpace(t))
		if strings.HasPrefix(u, "POINT") {
			body := strings.TrimSpace(strings.TrimPrefix(u, "POINT"))
			body = strings.TrimPrefix(body, "(")
			body = strings.TrimSuffix(body, ")")
			fields := strings.Fields(body)
			if len(fields) == 2 {
				lon, e1 := strconv.ParseFloat(fields[0], 64)
				lat, e2 := strconv.ParseFloat(fields[1], 64)
				if e1 == nil && e2 == nil {
					return lat, lon, true
				}
			}
		}
		if lat, lon, ok := decodeGeohash(t); ok {
			return lat, lon, true
		}
	}
	return 0, 0, false
}

func unitToMeters(unit string) float64 {
	switch strings.ToLower(unit) {
	case "mi", "miles":
		return 1609.344
	case "yd", "yards":
		return 0.9144
	case "ft", "feet":
		return 0.3048
	case "in", "inch":
		return 0.0254
	case "km", "kilometers":
		return 1000
	case "cm":
		return 0.01
	case "mm":
		return 0.001
	case "nmi", "nauticalmiles":
		return 1852
	default:
		return 1
	}
}

func parseDistanceMeters(v any) (float64, bool) {
	switch t := v.(type) {
	case json.Number:
		f, _ := t.Float64()
		return f, true
	case float64:
		return t, true
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		i := 0
		for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.' || s[i] == '-' || s[i] == '+' || s[i] == 'e') {
			i++
		}
		num, err := strconv.ParseFloat(strings.TrimSpace(s[:i]), 64)
		if err != nil {
			return 0, false
		}
		unit := strings.TrimSpace(s[i:])
		return num * unitToMeters(unit), true
	}
	return 0, false
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371008.8
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

func geoFieldTypeOf(idx *Index, field string) (string, bool) {
	def, ok := mappingFieldDef(idx, field)
	if !ok {
		return "", false
	}
	t, _ := def["type"].(string)
	return t, true
}

func geoQueryShardError(reason string) *apiError {
	return matchSearchError("query_shard_exception", reason)
}

func geoFloatString(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

func (c *queryCtx) evalGeoDistance(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[geo_distance] query malformed")
	}
	distance, hasDist := parseDistanceMeters(bm["distance"])
	if !hasDist {
		return false, 0, errParsing("[geo_distance] requires 'distance'")
	}
	field := ""
	var pointRaw any
	for k, v := range bm {
		if k == "distance" || k == "distance_type" || k == "boost" || k == "_name" ||
			k == "validation_method" || k == "ignore_unmapped" {
			continue
		}
		field = k
		pointRaw = v
		break
	}
	if field == "" {
		return false, 0, errParsing("[geo_distance] requires a field")
	}
	ignoreUnmapped, _ := bm["ignore_unmapped"].(bool)
	ftype, mapped := geoFieldTypeOf(c.idx, field)
	if !mapped {
		if ignoreUnmapped {
			return false, 0, nil
		}
		return false, 0, geoQueryShardError("failed to find geo field [" + field + "]")
	}
	if ftype != "geo_point" && ftype != "geo_shape" {
		return false, 0, geoQueryShardError("type [" + ftype + "] for field [" + field +
			"] is not supported for [geo_distance] queries. Must be one of [geo_point] or [geo_shape]")
	}
	plat, plon, ok := geoPointOf(pointRaw)
	if !ok {
		return false, 0, errParsing("failed to parse geo_distance point")
	}
	validationMethod, _ := bm["validation_method"].(string)
	if plat < -90 || plat > 90 {
		latText := geoFloatString(plat)
		if strings.EqualFold(validationMethod, "IGNORE_MALFORMED") {
			reason := "failed to create query: invalid latitude " + latText +
				"; must be between -90.0 and 90.0"
			e := geoQueryShardError(reason)
			e.CausedBy = &apiError{Type: "illegal_argument_exception", Status: 400,
				Reason: "invalid latitude " + latText + "; must be between -90.0 and 90.0"}
			return false, 0, e
		}
		e := geoQueryShardError("couldn't validate latitude/ longitude values")
		e.CausedBy = &apiError{Type: "query_shard_exception", Status: 400,
			Reason: "couldn't validate latitude/ longitude values",
			CausedBy: &apiError{Type: "query_validation_exception", Status: 400,
				Reason: "Validation Failed: 1: [geo_distance] center point latitude is invalid: " +
					latText + ";"}}
		return false, 0, e
	}
	boost := 1.0
	if b, ok := toFloat(bm["boost"]); ok {
		boost = b
	}
	vals, present := c.values(field)
	if !present {
		return false, 0, nil
	}
	for _, v := range vals {
		lat, lon, ok := geoPointOf(v)
		if !ok {
			continue
		}
		if haversineMeters(plat, plon, lat, lon) <= distance {
			return true, boost, nil
		}
	}
	return false, 0, nil
}

func parseWktBBox(s string) (top, left, bottom, right float64, ok bool) {
	u := strings.ToUpper(strings.TrimSpace(s))
	if !strings.HasPrefix(u, "BBOX") {
		return 0, 0, 0, 0, false
	}
	body := strings.TrimSpace(strings.TrimPrefix(u, "BBOX"))
	body = strings.TrimPrefix(body, "(")
	body = strings.TrimSuffix(body, ")")
	parts := strings.Split(body, ",")
	if len(parts) != 4 {
		return 0, 0, 0, 0, false
	}
	nums := make([]float64, 4)
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return 0, 0, 0, 0, false
		}
		nums[i] = f
	}
	return nums[2], nums[0], nums[3], nums[1], true
}

func (c *queryCtx) evalGeoBoundingBox(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[geo_bounding_box] query malformed")
	}
	field := ""
	var spec map[string]any
	for k, v := range bm {
		if k == "boost" || k == "_name" || k == "validation_method" || k == "type" || k == "ignore_unmapped" {
			continue
		}
		field = k
		spec, _ = v.(map[string]any)
		break
	}
	if field == "" || spec == nil {
		return false, 0, errParsing("[geo_bounding_box] requires a field")
	}
	ignoreUnmapped, _ := bm["ignore_unmapped"].(bool)
	ftype, mapped := geoFieldTypeOf(c.idx, field)
	if !mapped {
		if ignoreUnmapped {
			return false, 0, nil
		}
		return false, 0, geoQueryShardError("failed to find geo field [" + field + "]")
	}
	if ftype != "geo_point" && ftype != "geo_shape" {
		return false, 0, geoQueryShardError("type [" + ftype + "] for field [" + field +
			"] is not supported for [geo_bounding_box] queries. Must be one of [geo_point] or [geo_shape]")
	}
	var top, left, bottom, right float64
	have := false
	corner := func(key string) (float64, float64, bool) {
		v, ok := spec[key]
		if !ok {
			return 0, 0, false
		}
		return geoPointOf(v)
	}
	if lat, lon, ok := corner("top_left"); ok {
		top, left, have = lat, lon, true
	}
	if lat, lon, ok := corner("bottom_right"); ok {
		bottom, right = lat, lon
	}
	if lat, lon, ok := corner("top_right"); ok {
		top, right, have = lat, lon, true
	}
	if lat, lon, ok := corner("bottom_left"); ok {
		bottom, left = lat, lon
	}
	if !have {
		if w, ok := spec["wkt"].(string); ok {
			t, l, b, r, ok2 := parseWktBBox(w)
			if ok2 {
				top, left, bottom, right, have = t, l, b, r, true
			}
		}
	}
	if !have {
		top, _ = toFloat(spec["top"])
		left, _ = toFloat(spec["left"])
		bottom, _ = toFloat(spec["bottom"])
		right, _ = toFloat(spec["right"])
	}
	if top < bottom {
		return false, 0, &apiError{Type: "illegal_argument_exception", Status: 400,
			Reason: "top is below bottom corner: " + formatNumber(top) + " vs. " + formatNumber(bottom)}
	}
	boost := 1.0
	if b, ok := toFloat(bm["boost"]); ok {
		boost = b
	}
	vals, present := c.values(field)
	if !present {
		return false, 0, nil
	}
	wrap := left > right
	for _, v := range vals {
		lat, lon, ok := geoPointOf(v)
		if !ok {
			continue
		}
		if lat <= top && lat >= bottom && (wrap && (lon >= left || lon <= right) || !wrap && lon >= left && lon <= right) {
			return true, boost, nil
		}
	}
	return false, 0, nil
}

func (c *queryCtx) evalGeoPolygon(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[geo_polygon] query malformed")
	}
	field := ""
	var spec map[string]any
	for k, v := range bm {
		if k == "boost" || k == "_name" || k == "validation_method" || k == "ignore_unmapped" {
			continue
		}
		field = k
		spec, _ = v.(map[string]any)
		break
	}
	if field == "" || spec == nil {
		return false, 0, errParsing("[geo_polygon] requires a field")
	}
	ignoreUnmapped, _ := bm["ignore_unmapped"].(bool)
	ftype, mapped := geoFieldTypeOf(c.idx, field)
	if !mapped {
		if ignoreUnmapped {
			return false, 0, nil
		}
		return false, 0, geoQueryShardError("failed to find geo field [" + field + "]")
	}
	if ftype != "geo_point" && ftype != "geo_shape" {
		return false, 0, geoQueryShardError("type [" + ftype + "] for field [" + field +
			"] is not supported for [geo_polygon] queries. Must be one of [geo_point] or [geo_shape]")
	}
	pts, _ := spec["points"].([]any)
	ring := make([][2]float64, 0, len(pts))
	for _, p := range pts {
		lat, lon, ok := geoPointOf(p)
		if !ok {
			return false, 0, errParsing("failed to parse geo_polygon point")
		}
		ring = append(ring, [2]float64{lon, lat})
	}
	if len(ring) < 3 {
		return false, 0, errParsing("failed to parse geo_polygon")
	}
	boost := 1.0
	if b, ok := toFloat(bm["boost"]); ok {
		boost = b
	}
	vals, present := c.values(field)
	if !present {
		return false, 0, nil
	}
	for _, v := range vals {
		lat, lon, ok := geoPointOf(v)
		if !ok {
			continue
		}
		if pointInRing(lon, lat, ring) {
			return true, boost, nil
		}
	}
	return false, 0, nil
}

func pointInRing(lon, lat float64, ring [][2]float64) bool {
	inside := false
	n := len(ring)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		xi, yi := ring[i][0], ring[i][1]
		xj, yj := ring[j][0], ring[j][1]
		if (yi > lat) != (yj > lat) && lon < (xj-xi)*(lat-yi)/(yj-yi)+xi {
			inside = !inside
		}
	}
	return inside
}

type geoShape struct {
	typ        string
	lat, lon   float64
	radiusM    float64
	top, left  float64
	bot, right float64
	points     [][2]float64 // lon, lat
}

func shapeCoordPair(v any) (float64, float64, bool) {
	arr, ok := v.([]any)
	if !ok || len(arr) < 2 {
		return 0, 0, false
	}
	lon, ok1 := toFloat(arr[0])
	lat, ok2 := toFloat(arr[1])
	return lon, lat, ok1 && ok2
}

func parseWktNumbers(s string) []float64 {
	repl := strings.NewReplacer("(", " ", ")", " ", ",", " ")
	fields := strings.Fields(repl.Replace(s))
	out := make([]float64, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}

func parseGeoShape(v any) (*geoShape, bool) {
	if s, ok := v.(string); ok {
		return parseWktGeoShape(s)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	typ, _ := m["type"].(string)
	gs := &geoShape{typ: strings.ToLower(typ)}
	switch gs.typ {
	case "point":
		if c, ok := m["coordinates"].(string); ok {
			lat, lon, ok := geoPointOf(c)
			if !ok {
				return nil, false
			}
			gs.lat, gs.lon = lat, lon
			return gs, true
		}
		lon, lat, ok := shapeCoordPair(m["coordinates"])
		if !ok {
			return nil, false
		}
		gs.lon, gs.lat = lon, lat
		return gs, true
	case "circle":
		if r, ok := m["radius"].(string); ok {
			gs.radiusM, _ = parseDistanceMeters(r)
		} else if r, ok := toFloat(m["radius"]); ok {
			gs.radiusM = r
		}
		lon, lat, ok := shapeCoordPair(m["coordinates"])
		if !ok {
			return nil, false
		}
		gs.lon, gs.lat = lon, lat
		return gs, true
	case "envelope":
		coords, _ := m["coordinates"].([]any)
		if len(coords) < 2 {
			return nil, false
		}
		left, top, ok1 := shapeCoordPair(coords[0])
		right, bot, ok2 := shapeCoordPair(coords[1])
		if !ok1 || !ok2 {
			return nil, false
		}
		gs.left, gs.top, gs.right, gs.bot = left, top, right, bot
		return gs, true
	case "polygon":
		rings, _ := m["coordinates"].([]any)
		if len(rings) == 0 {
			return nil, false
		}
		ring, _ := rings[0].([]any)
		for _, pt := range ring {
			lon, lat, ok := shapeCoordPair(pt)
			if ok {
				gs.points = append(gs.points, [2]float64{lon, lat})
			}
		}
		return gs, len(gs.points) >= 3
	case "linestring":
		coords, _ := m["coordinates"].([]any)
		for _, pt := range coords {
			lon, lat, ok := shapeCoordPair(pt)
			if ok {
				gs.points = append(gs.points, [2]float64{lon, lat})
			}
		}
		return gs, len(gs.points) >= 2
	case "multipoint":
		coords, _ := m["coordinates"].([]any)
		for _, pt := range coords {
			lon, lat, ok := shapeCoordPair(pt)
			if ok {
				gs.points = append(gs.points, [2]float64{lon, lat})
			}
		}
		return gs, len(gs.points) > 0
	case "geometrycollection":
		geoms, _ := m["geometries"].([]any)
		if len(geoms) == 0 {
			return nil, false
		}
		gs.typ = "multipoint"
		for _, g := range geoms {
			sub, ok := parseGeoShape(g)
			if !ok {
				return nil, false
			}
			gs.points = append(gs.points, shapeVertices(sub)...)
		}
		return gs, len(gs.points) > 0
	}
	return nil, false
}

func parseWktGeoShape(s string) (*geoShape, bool) {
	u := strings.ToUpper(strings.TrimSpace(s))
	switch {
	case strings.HasPrefix(u, "POINT"):
		nums := parseWktNumbers(strings.TrimSpace(s[len("POINT"):]))
		if len(nums) < 2 {
			return nil, false
		}
		return &geoShape{typ: "point", lon: nums[0], lat: nums[1]}, true
	case strings.HasPrefix(u, "BBOX"):
		top, left, bottom, right, ok := parseWktBBox(s)
		if !ok {
			return nil, false
		}
		return &geoShape{typ: "envelope", top: top, left: left, bot: bottom, right: right}, true
	case strings.HasPrefix(u, "MULTIPOINT"):
		body := strings.TrimSpace(s[len("MULTIPOINT"):])
		body = strings.TrimSpace(strings.TrimPrefix(body, "("))
		body = strings.TrimSpace(strings.TrimSuffix(body, ")"))
		gs := &geoShape{typ: "multipoint"}
		for _, pt := range strings.Split(body, ",") {
			nums := parseWktNumbers(pt)
			if len(nums) >= 2 {
				gs.points = append(gs.points, [2]float64{nums[0], nums[1]})
			}
		}
		return gs, len(gs.points) > 0
	case strings.HasPrefix(u, "POLYGON"):
		nums := parseWktNumbers(strings.TrimSpace(s[len("POLYGON"):]))
		gs := &geoShape{typ: "polygon"}
		for i := 0; i+1 < len(nums); i += 2 {
			gs.points = append(gs.points, [2]float64{nums[i], nums[i+1]})
		}
		return gs, len(gs.points) >= 3
	case strings.HasPrefix(u, "LINESTRING"):
		nums := parseWktNumbers(strings.TrimSpace(s[len("LINESTRING"):]))
		gs := &geoShape{typ: "linestring"}
		for i := 0; i+1 < len(nums); i += 2 {
			gs.points = append(gs.points, [2]float64{nums[i], nums[i+1]})
		}
		return gs, len(gs.points) >= 2
	}
	return nil, false
}

func shapeContainsPoint(gs *geoShape, lon, lat float64) bool {
	switch gs.typ {
	case "point":
		return math.Abs(gs.lon-lon) < 1e-9 && math.Abs(gs.lat-lat) < 1e-9
	case "envelope":
		return lat <= gs.top && lat >= gs.bot && lon >= gs.left && lon <= gs.right
	case "circle":
		return haversineMeters(gs.lat, gs.lon, lat, lon) <= gs.radiusM
	case "polygon":
		return pointInRing(lon, lat, gs.points)
	case "linestring":
		for i := 0; i+1 < len(gs.points); i++ {
			if pointOnSegment(lon, lat, gs.points[i], gs.points[i+1]) {
				return true
			}
		}
		return false
	case "multipoint":
		for _, p := range gs.points {
			if math.Abs(p[0]-lon) < 1e-9 && math.Abs(p[1]-lat) < 1e-9 {
				return true
			}
		}
	}
	return false
}

func pointOnSegment(px, py float64, a, b [2]float64) bool {
	cross := (b[0]-a[0])*(py-a[1]) - (b[1]-a[1])*(px-a[0])
	if math.Abs(cross) > 1e-9 {
		return false
	}
	if px < math.Min(a[0], b[0])-1e-9 || px > math.Max(a[0], b[0])+1e-9 {
		return false
	}
	if py < math.Min(a[1], b[1])-1e-9 || py > math.Max(a[1], b[1])+1e-9 {
		return false
	}
	return true
}

func shapeVertices(gs *geoShape) [][2]float64 {
	switch gs.typ {
	case "point":
		return [][2]float64{{gs.lon, gs.lat}}
	case "envelope":
		return [][2]float64{{gs.left, gs.top}, {gs.right, gs.top}, {gs.right, gs.bot}, {gs.left, gs.bot}}
	case "circle":
		return [][2]float64{{gs.lon, gs.lat}}
	case "polygon", "linestring", "multipoint":
		return gs.points
	}
	return nil
}

func shapeEdges(gs *geoShape) [][2][2]float64 {
	pts := shapeVertices(gs)
	if gs.typ == "envelope" || gs.typ == "polygon" || gs.typ == "circle" {
		if len(pts) >= 3 {
			pts = append(append([][2]float64{}, pts...), pts[0])
		}
	}
	var out [][2][2]float64
	for i := 0; i+1 < len(pts); i++ {
		out = append(out, [2][2]float64{pts[i], pts[i+1]})
	}
	return out
}

func segmentsIntersect(a, b, c, d [2]float64) bool {
	orient := func(p, q, r [2]float64) float64 {
		return (q[0]-p[0])*(r[1]-p[1]) - (q[1]-p[1])*(r[0]-p[0])
	}
	o1 := orient(a, b, c)
	o2 := orient(a, b, d)
	o3 := orient(c, d, a)
	o4 := orient(c, d, b)
	if (o1 > 0 && o2 < 0 || o1 < 0 && o2 > 0) && (o3 > 0 && o4 < 0 || o3 < 0 && o4 > 0) {
		return true
	}
	if math.Abs(o1) < 1e-9 && pointOnSegment(c[0], c[1], a, b) {
		return true
	}
	if math.Abs(o2) < 1e-9 && pointOnSegment(d[0], d[1], a, b) {
		return true
	}
	if math.Abs(o3) < 1e-9 && pointOnSegment(a[0], a[1], c, d) {
		return true
	}
	if math.Abs(o4) < 1e-9 && pointOnSegment(b[0], b[1], c, d) {
		return true
	}
	return false
}

func shapeIntersectsShape(a, b *geoShape) bool {
	if a == nil || b == nil {
		return false
	}
	if a.typ == "point" {
		return shapeContainsPoint(b, a.lon, a.lat)
	}
	if b.typ == "point" {
		return shapeContainsPoint(a, b.lon, b.lat)
	}
	for _, p := range shapeVertices(a) {
		if shapeContainsPoint(b, p[0], p[1]) {
			return true
		}
	}
	for _, p := range shapeVertices(b) {
		if shapeContainsPoint(a, p[0], p[1]) {
			return true
		}
	}
	ae := shapeEdges(a)
	be := shapeEdges(b)
	for _, e1 := range ae {
		for _, e2 := range be {
			if segmentsIntersect(e1[0], e1[1], e2[0], e2[1]) {
				return true
			}
		}
	}
	return false
}

func shapeContainsShape(outer, inner *geoShape) bool {
	if outer == nil || inner == nil {
		return false
	}
	if inner.typ == "point" {
		return shapeContainsPoint(outer, inner.lon, inner.lat)
	}
	for _, p := range shapeVertices(inner) {
		if !shapeContainsPoint(outer, p[0], p[1]) {
			return false
		}
	}
	return true
}

func lookupIndexedShape(spec map[string]any) (any, bool) {
	idxName, _ := spec["index"].(string)
	id, _ := spec["id"].(string)
	path, _ := spec["path"].(string)
	if globalServer == nil || idxName == "" || id == "" {
		return nil, false
	}
	// Callers already hold globalServer.mu (read or write); taking it again
	// here deadlocks once a writer is queued.
	idx, ok := globalServer.indices[idxName]
	if !ok {
		return nil, false
	}
	doc, ok := idx.Docs[id]
	if !ok {
		return nil, false
	}
	var cur any = doc
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur = m[part]
	}
	return cur, cur != nil
}

func (c *queryCtx) evalGeoShape(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[geo_shape] query malformed")
	}
	field := ""
	var spec map[string]any
	for k, v := range bm {
		if k == "boost" || k == "_name" || k == "ignore_unmapped" || k == "relation" {
			continue
		}
		field = k
		spec, _ = v.(map[string]any)
		break
	}
	if field == "" || spec == nil {
		return false, 0, errParsing("[geo_shape] requires a field")
	}
	ignoreUnmapped, _ := bm["ignore_unmapped"].(bool)
	ftype, mapped := geoFieldTypeOf(c.idx, field)
	if !mapped {
		if ignoreUnmapped {
			return false, 0, nil
		}
		return false, 0, geoQueryShardError("failed to find type for field [" + field + "]")
	}
	if ftype != "geo_point" && ftype != "geo_shape" {
		return false, 0, geoQueryShardError("type [" + ftype + "] for field [" + field +
			"] is not supported for [geo_shape] queries. Must be one of [geo_point] or [geo_shape]")
	}
	var shapeRaw any
	if s, has := spec["shape"]; has {
		shapeRaw = s
	} else if is, has := spec["indexed_shape"].(map[string]any); has {
		v, ok := lookupIndexedShape(is)
		if !ok {
			return false, 0, errParsing("failed to load indexed shape")
		}
		shapeRaw = v
	} else {
		shapeRaw = spec
	}
	qs, ok := parseGeoShape(shapeRaw)
	if !ok {
		return false, 0, errParsing("failed to parse geo_shape")
	}
	relation, _ := spec["relation"].(string)
	if relation == "" {
		relation, _ = bm["relation"].(string)
	}
	if relation == "" {
		relation = "INTERSECTS"
	}
	relation = strings.ToUpper(relation)
	boost := 1.0
	if b, ok := toFloat(bm["boost"]); ok {
		boost = b
	}
	vals, present := c.values(field)
	if !present {
		return false, 0, nil
	}
	for _, v := range vals {
		var ds *geoShape
		if ftype == "geo_point" {
			lat, lon, ok := geoPointOf(v)
			if !ok {
				continue
			}
			ds = &geoShape{typ: "point", lat: lat, lon: lon}
		} else {
			var ok bool
			ds, ok = parseGeoShape(v)
			if !ok {
				continue
			}
		}
		matched := false
		switch relation {
		case "DISJOINT":
			matched = !shapeIntersectsShape(qs, ds)
		case "WITHIN":
			matched = shapeContainsShape(qs, ds)
		case "CONTAINS":
			matched = shapeContainsShape(ds, qs)
		default:
			matched = shapeIntersectsShape(qs, ds)
		}
		if matched {
			return true, boost, nil
		}
	}
	return false, 0, nil
}

// ---------------------------------------------------------------------------
// rank_feature / distance_feature
// ---------------------------------------------------------------------------

func rankFeatureFieldDef(idx *Index, field string) (map[string]any, string) {
	if def, ok := mappingFieldDef(idx, field); ok {
		t, _ := def["type"].(string)
		if t == "rank_feature" || t == "rank_features" {
			return def, t
		}
		return def, t
	}
	for i := strings.LastIndex(field, "."); i > 0; i = strings.LastIndex(field[:i], ".") {
		parent := field[:i]
		if def, ok := mappingFieldDef(idx, parent); ok {
			t, _ := def["type"].(string)
			if t == "rank_features" {
				return def, t
			}
		}
	}
	return nil, ""
}

func (c *queryCtx) evalRankFeature(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[rank_feature] query malformed")
	}
	field, _ := bm["field"].(string)
	boost := 1.0
	if b, ok := toFloat(bm["boost"]); ok {
		boost = b
	}
	fnCount := 0
	for _, k := range []string{"saturation", "log", "sigmoid", "linear"} {
		if _, has := bm[k]; has {
			fnCount++
		}
	}
	if fnCount > 1 {
		return false, 0, &apiError{Type: "x_content_parse_exception", Status: 400,
			Reason: "Failed to build [feature] after last required field arrived",
			CausedBy: &apiError{Type: "illegal_argument_exception", Status: 400,
				Reason: "Can only specify one of [log], [saturation], [sigmoid] and [linear]"}}
	}
	def, ftype := rankFeatureFieldDef(c.idx, field)
	if ftype != "rank_feature" && ftype != "rank_features" {
		if ftype == "" {
			ftype = "unmapped"
		}
		reason := "[rank_feature] query only works on [rank_feature] fields and features of " +
			"[rank_features] fields, not [" + ftype + "]"
		e := geoQueryShardError("failed to create query: " + reason)
		e.CausedBy = &apiError{Type: "illegal_argument_exception", Status: 400, Reason: "failed to create query: " + reason}
		return false, 0, e
	}
	vals, present := c.values(field)
	if !present {
		return false, 0, nil
	}
	f, ok := toFloat(vals[0])
	if !ok {
		return false, 0, nil
	}
	if def != nil {
		if pos, has := def["positive_score_impact"].(bool); has && !pos {
			if f > 0 {
				f = 1 / f
			}
		}
	}
	sc := f
	if sat, ok := bm["saturation"].(map[string]any); ok {
		pivot := 3.0
		if p, ok := toFloat(sat["pivot"]); ok {
			pivot = p
		}
		sc = f / (f + pivot)
	} else if lg, ok := bm["log"].(map[string]any); ok {
		sc = math.Log(1 + f)
		if s, ok := toFloat(lg["scaling_factor"]); ok {
			sc *= s
		}
	} else if sg, ok := bm["sigmoid"].(map[string]any); ok {
		pivot := 1.0
		exponent := 1.0
		if p, ok := toFloat(sg["pivot"]); ok {
			pivot = p
		}
		if e, ok := toFloat(sg["exponent"]); ok {
			exponent = e
		}
		fe := math.Pow(f, exponent)
		pe := math.Pow(pivot, exponent)
		sc = fe / (fe + pe)
	} else if ln, ok := bm["linear"].(map[string]any); ok {
		sc = f
		if s, ok := toFloat(ln["scaling_factor"]); ok {
			sc *= s
		}
	} else if ftype == "rank_features" {
		sc = f / (f + 5)
	} else {
		sc = f / (f + 1)
	}
	return true, boost * sc, nil
}

func datePivotMillis(v any) (float64, bool) {
	if f, ok := toFloat(v); ok {
		return f, true
	}
	s, ok := v.(string)
	if !ok {
		return 0, false
	}
	s = strings.TrimSpace(strings.ToLower(s))
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.' || s[i] == '-' || s[i] == '+') {
		i++
	}
	num, err := strconv.ParseFloat(strings.TrimSpace(s[:i]), 64)
	if err != nil {
		return 0, false
	}
	unit := strings.TrimSpace(s[i:])
	mult := 0.0
	switch unit {
	case "", "ms":
		mult = 1
	case "s":
		mult = 1000
	case "m":
		mult = 60000
	case "h":
		mult = 3600000
	case "d":
		mult = 86400000
	default:
		return 0, false
	}
	return num * mult, true
}

func (c *queryCtx) evalDistanceFeature(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[distance_feature] query malformed")
	}
	field, _ := bm["field"].(string)
	boost := 1.0
	if b, ok := toFloat(bm["boost"]); ok {
		boost = b
	}
	def, _ := mappingFieldDef(c.idx, field)
	ftype, _ := def["type"].(string)
	if ftype != "date" && ftype != "date_nanos" && ftype != "geo_point" {
		shown := ftype
		if shown == "" {
			shown = "unmapped"
		}
		reason := "Illegal data type of [" + shown + "]![distance_feature] query can only be run on " +
			"a date, date_nanos or geo_point field type!"
		e := geoQueryShardError("failed to create query: " + reason)
		e.CausedBy = &apiError{Type: "illegal_argument_exception", Status: 400, Reason: reason}
		return false, 0, e
	}
	vals, present := c.values(field)
	if !present {
		return false, 0, nil
	}
	if ftype == "geo_point" {
		pivot, ok := parseDistanceMeters(bm["pivot"])
		if !ok {
			return false, 0, nil
		}
		olat, olon, ok := geoPointOf(bm["origin"])
		if !ok {
			return false, 0, nil
		}
		lat, lon, ok := geoPointOf(vals[0])
		if !ok {
			return false, 0, nil
		}
		dist := haversineMeters(olat, olon, lat, lon)
		return true, boost * pivot / (pivot + dist), nil
	}
	v, ok := toFloat(vals[0])
	if !ok {
		return false, 0, nil
	}
	pivot, ok := datePivotMillis(bm["pivot"])
	if !ok {
		return false, 0, nil
	}
	var origin float64
	if o, ok := toFloat(bm["origin"]); ok {
		origin = o
	} else if s, isStr := bm["origin"].(string); isStr {
		if o, err := parseQueryDate(def, bm["origin"], ""); err == nil {
			origin, _ = toFloat(o)
			if !strings.Contains(s, "T") {
				origin += 86399999
			}
		}
	}
	dist := math.Abs(v - origin)
	if dist == 0 {
		return true, boost, nil
	}
	return true, boost * pivot / (pivot + dist), nil
}

// ---------------------------------------------------------------------------
// wrapper / pinned / percolate / terms_set
// ---------------------------------------------------------------------------

func (c *queryCtx) evalWrapper(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[wrapper] query malformed")
	}
	raw, ok := bm["query"]
	if !ok {
		return false, 0, errParsing("[wrapper] requires 'query'")
	}
	switch t := raw.(type) {
	case string:
		decoded, err := base64.StdEncoding.DecodeString(t)
		if err != nil {
			if uerr := json.Unmarshal([]byte(t), &map[string]any{}); uerr != nil {
				return false, 0, errParsing("failed to parse wrapper query")
			}
			decoded = []byte(t)
		}
		var inner map[string]any
		if err := json.Unmarshal(decoded, &inner); err != nil {
			return false, 0, errParsing("failed to parse wrapper query")
		}
		return c.eval(inner)
	case map[string]any:
		return c.eval(t)
	}
	return false, 0, errParsing("failed to parse wrapper query")
}

func (c *queryCtx) evalPinned(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[pinned] query malformed")
	}
	boost := 1.0
	if b, ok := toFloat(bm["boost"]); ok {
		boost = b
	}
	pinned := false
	for _, key := range []string{"ids"} {
		if list, ok := bm[key].([]any); ok {
			for _, e := range list {
				if s, _ := toString(e); s == c.id {
					pinned = true
				}
			}
		}
	}
	if docs, ok := bm["docs"].([]any); ok {
		for _, e := range docs {
			if m, ok := e.(map[string]any); ok {
				if s, _ := toString(m["_id"]); s == c.id {
					pinned = true
				}
			}
		}
	}
	if pinned {
		return true, boost, nil
	}
	if organic, ok := bm["organic"].(map[string]any); ok {
		return c.eval(organic)
	}
	return false, 0, nil
}

func percolateFieldName(bm map[string]any) string {
	field, _ := bm["field"].(string)
	if field == "" {
		field = "query"
	}
	return field
}

func percolateInputDocs(idx *Index, bm map[string]any) ([]map[string]any, *apiError) {
	if d, ok := bm["document"].(map[string]any); ok {
		return []map[string]any{d}, nil
	}
	if raw, ok := bm["documents"].([]any); ok {
		var docs []map[string]any
		for _, e := range raw {
			if m, ok := e.(map[string]any); ok {
				docs = append(docs, m)
			}
		}
		return docs, nil
	}
	if idv, has := bm["id"]; has {
		id, _ := toString(idv)
		idxName, _ := bm["index"].(string)
		if idxName == "" {
			idxName = idx.Name
		}
		var target *Index
		if idxName == idx.Name {
			target = idx
		}
		var doc map[string]any
		if target != nil {
			doc = target.Docs[id]
		}
		if doc == nil {
			return nil, &apiError{Type: "resource_not_found_exception", Status: 404,
				Reason: "indexed document [" + idxName + "/" + id + "] couldn't be found"}
		}
		return []map[string]any{doc}, nil
	}
	return nil, errParsing("[percolate] requires 'document'")
}

func percolateInputCtx(idx *Index, doc map[string]any) *queryCtx {
	inputCtx := newIndexCtx(idx, "_percolate")
	inputCtx.doc = doc
	if err := inputCtx.buildDocValues(); err != nil {
		return &queryCtx{idx: idx, id: "_percolate", doc: doc}
	}
	return &queryCtx{idx: idx, id: "_percolate", doc: doc, vals: inputCtx.values}
}

func (c *queryCtx) evalPercolate(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[percolate] query malformed")
	}
	field := percolateFieldName(bm)
	def, mapped := mappingFieldDef(c.idx, field)
	ftype := ""
	if mapped {
		ftype, _ = def["type"].(string)
	}
	if ftype != "percolator" {
		if ftype == "" {
			ftype = "object"
		}
		return false, 0, matchShardError(
			fmt.Sprintf("expected field [%s] to be of type [percolator], but is of type [%s]", field, ftype))
	}
	docs, derr := percolateInputDocs(c.idx, bm)
	if derr != nil {
		return false, 0, derr
	}
	stored, ok := c.doc[field].(map[string]any)
	if !ok {
		return false, 0, nil
	}
	best := 0.0
	found := false
	for _, docRaw := range docs {
		inputQC := percolateInputCtx(c.idx, docRaw)
		matched, sc, err := inputQC.eval(stored)
		if err != nil {
			return false, 0, err
		}
		if matched {
			found = true
			if !found || sc > best {
				best = sc
			}
		}
	}
	if !found {
		return false, 0, nil
	}
	return true, best, nil
}

func collectPercolateClauses(q map[string]any, out *[]map[string]any) {
	for clause, raw := range q {
		bm, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch clause {
		case "percolate":
			*out = append(*out, bm)
		case "bool":
			for _, key := range []string{"must", "should", "filter", "must_not"} {
				for _, item := range asQueryList(bm[key]) {
					if child, ok := item.(map[string]any); ok {
						collectPercolateClauses(child, out)
					}
				}
			}
		case "constant_score":
			if child, ok := bm["filter"].(map[string]any); ok {
				collectPercolateClauses(child, out)
			}
		case "dis_max":
			for _, item := range asQueryList(bm["queries"]) {
				if child, ok := item.(map[string]any); ok {
					collectPercolateClauses(child, out)
				}
			}
		case "boosting":
			for _, key := range []string{"positive", "negative"} {
				if child, ok := bm[key].(map[string]any); ok {
					collectPercolateClauses(child, out)
				}
			}
		case "function_score", "script_score":
			if child, ok := bm["query"].(map[string]any); ok {
				collectPercolateClauses(child, out)
			}
		}
	}
}

func percolateSlotsForHit(c *queryCtx, query map[string]any) []any {
	var clauses []map[string]any
	collectPercolateClauses(query, &clauses)
	if len(clauses) == 0 {
		return nil
	}
	slotSet := map[int]bool{}
	for _, bm := range clauses {
		field := percolateFieldName(bm)
		docs, err := percolateInputDocs(c.idx, bm)
		if err != nil {
			continue
		}
		stored, ok := c.doc[field].(map[string]any)
		if !ok {
			continue
		}
		for slot, docRaw := range docs {
			inputQC := percolateInputCtx(c.idx, docRaw)
			matched, _, err := inputQC.eval(stored)
			if err == nil && matched {
				slotSet[slot] = true
			}
		}
	}
	if len(slotSet) == 0 {
		return nil
	}
	slots := make([]int, 0, len(slotSet))
	for s := range slotSet {
		slots = append(slots, s)
	}
	sort.Ints(slots)
	out := make([]any, 0, len(slots))
	for _, s := range slots {
		out = append(out, int64(s))
	}
	return out
}

func percolateHighlights(c *queryCtx, query map[string]any, cfg *highlightConfig) map[string]any {
	var clauses []map[string]any
	collectPercolateClauses(query, &clauses)
	if len(clauses) == 0 {
		return nil
	}
	bm := clauses[0]
	field := percolateFieldName(bm)
	stored, ok := c.doc[field].(map[string]any)
	if !ok {
		return nil
	}
	docs, err := percolateInputDocs(c.idx, bm)
	if err != nil || len(docs) == 0 {
		return nil
	}
	terms, qfields, qpatterns := queryHighlightInfo(stored)
	out := map[string]any{}
	for slot, docRaw := range docs {
		local := *cfg
		local.terms, local.queryFields, local.queryPatterns = terms, qfields, qpatterns
		local.query = stored
		rendered := local.render(c.idx, docRaw)
		for name, val := range rendered {
			if len(docs) == 1 {
				out[name] = val
			} else {
				out[fmt.Sprintf("%d_%s", slot, name)] = val
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (c *queryCtx) evalTermsSet(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[terms_set] query malformed")
	}
	field := ""
	var spec map[string]any
	for k, v := range bm {
		if k == "boost" || k == "_name" {
			continue
		}
		field = k
		spec, _ = v.(map[string]any)
		break
	}
	if field == "" || spec == nil {
		return false, 0, errParsing("[terms_set] requires a field")
	}
	terms, _ := spec["terms"].([]any)
	boost := 1.0
	if b, ok := toFloat(bm["boost"]); ok {
		boost = b
	}
	vals, present := c.values(field)
	if !present {
		return false, 0, nil
	}
	matched := 0
	for _, item := range terms {
		for _, v := range vals {
			if v == nil {
				continue
			}
			vs, _ := valueScalarString(v)
			is, _ := valueScalarString(item)
			if vs == is || numericEqual(v, item) {
				matched++
				break
			}
		}
	}
	required := 1
	if name, ok := spec["minimum_should_match_field"].(string); ok {
		if reqVals, ok := c.values(name); ok && len(reqVals) > 0 {
			required, _ = numericInt(reqVals[0])
		}
	}
	if scriptSpec, ok := spec["minimum_should_match_script"].(map[string]any); ok {
		if src, ok := scriptSpec["source"].(string); ok {
			if n, ok := scriptMinimumShouldMatch(src, len(terms)); ok {
				required = n
			}
		}
	}
	if matched >= required {
		return true, float64(matched) * boost, nil
	}
	return false, 0, nil
}

func numericInt(v any) (int, bool) {
	if n, ok := integerOf(v); ok {
		return int(n), true
	}
	if f, ok := toFloat(v); ok {
		return int(f), true
	}
	return 0, false
}

func scriptMinimumShouldMatch(src string, count int) (int, bool) {
	s := strings.ReplaceAll(src, " ", "")
	if s == "params.num_terms" {
		return count, true
	}
	if strings.HasPrefix(s, "Math.min(params.num_terms,") {
		inner := strings.TrimSuffix(strings.TrimPrefix(s, "Math.min(params.num_terms,"), ")")
		if n, err := strconv.Atoi(inner); err == nil {
			if n < count {
				return n, true
			}
			return count, true
		}
	}
	if strings.HasPrefix(s, "Math.max(params.num_terms,") {
		inner := strings.TrimSuffix(strings.TrimPrefix(s, "Math.max(params.num_terms,"), ")")
		if n, err := strconv.Atoi(inner); err == nil {
			if n > count {
				return n, true
			}
			return count, true
		}
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	return 0, false
}

// ---------------------------------------------------------------------------
// span queries
// ---------------------------------------------------------------------------

func spanKind(body map[string]any) (string, map[string]any) {
	for k, v := range body {
		if k == "boost" || k == "_name" {
			continue
		}
		bm, _ := v.(map[string]any)
		return k, bm
	}
	return "", nil
}

func isSpanType(k string) bool {
	switch k {
	case "span_term", "span_near", "span_or", "span_not", "span_containing", "span_within",
		"span_multi", "field_masking_span", "span_first":
		return true
	}
	return false
}

func spanTermValue(bm map[string]any) (string, string, *apiError) {
	keys := []string{}
	for k := range bm {
		if k == "boost" || k == "_name" {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) > 1 {
		sort.Strings(keys)
		return "", "", errParsing("[span_term] query doesn't support multiple fields, found [" +
			strings.Join(keys, "] and [") + "]")
	}
	if len(keys) == 0 {
		return "", "", errParsing("span_term malformed")
	}
	field := keys[0]
	val := bm[field]
	term := ""
	if m, ok := val.(map[string]any); ok {
		term, _ = toString(m["value"])
	} else {
		term, _ = toString(val)
	}
	return field, term, nil
}

func (c *queryCtx) evalSpanTerm(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("span_term malformed")
	}
	spans, _, err := c.spanTermSpans(bm)
	if err != nil {
		return false, 0, err
	}
	return len(spans) > 0, 1, nil
}

func (c *queryCtx) spanTermSpans(bm map[string]any) ([]intervalSpan, string, *apiError) {
	field, term, err := spanTermValue(bm)
	if err != nil {
		return nil, "", err
	}
	def, known := mappingFieldDef(c.idx, field)
	if !known {
		return nil, field, nil
	}
	ftype, _ := def["type"].(string)
	if ftype == "match_only_text" {
		reason := "field \"" + field + "\" was indexed without position data; cannot run SpanTermQuery (term=" + term + ")"
		e := matchSearchError("illegal_state_exception", reason)
		e.Status = 500
		return nil, field, e
	}
	if ftype == "integer" || ftype == "long" || ftype == "short" || ftype == "byte" || ftype == "double" ||
		ftype == "float" || ftype == "half_float" || ftype == "scaled_float" || ftype == "unsigned_long" ||
		ftype == "date" || ftype == "date_nanos" || ftype == "boolean" || ftype == "ip" {
		return nil, field, matchSearchError("query_shard_exception",
			"Cannot extract a term from a query of type ["+ftype+"]")
	}
	spans := []intervalSpan{}
	for _, p := range c.intervalPositions(field) {
		if p.term == term {
			spans = append(spans, intervalSpan{p.pos, p.pos})
		}
	}
	return spans, field, nil
}

func (c *queryCtx) spanQuery(body any) ([]intervalSpan, string, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return nil, "", errParsing("span query malformed")
	}
	kind, inner := spanKind(bm)
	if kind == "" || inner == nil {
		return nil, "", errParsing("span query malformed")
	}
	switch kind {
	case "span_term":
		return c.spanTermSpans(inner)
	case "span_near":
		clauses, _ := inner["clauses"].([]any)
		if len(clauses) == 0 {
			return nil, "", errParsing("span_near must include [clauses]")
		}
		lists := [][]intervalSpan{}
		field := ""
		for _, cl := range clauses {
			cm, _ := cl.(map[string]any)
			ck, cinner := spanKind(cm)
			if !isSpanType(ck) {
				return nil, "", errParsing("span_near [clauses] must be of type span query")
			}
			spans, f, err := c.spanQuery(cm)
			if err != nil {
				return nil, "", err
			}
			if field == "" {
				field = f
			} else if f != field {
				term := ""
				if ck == "span_term" {
					_, term, _ = spanTermValue(cinner)
				}
				return nil, "", matchSearchError("query_shard_exception",
					"Cannot add clause "+f+":"+term+" to SpanNearQuery for field "+field)
			}
			lists = append(lists, spans)
		}
		slop := 0
		if v, ok := toFloat(inner["slop"]); ok {
			slop = int(v)
		}
		inOrder := true
		if v, ok := inner["in_order"].(bool); ok {
			inOrder = v
		}
		if inOrder {
			return combineOrderedIntervals(lists, slop), field, nil
		}
		return combineUnorderedIntervals(lists, slop), field, nil
	case "span_or":
		clauses, _ := inner["clauses"].([]any)
		field := ""
		spans := []intervalSpan{}
		for _, cl := range clauses {
			cm, _ := cl.(map[string]any)
			sp, f, err := c.spanQuery(cm)
			if err != nil {
				return nil, "", err
			}
			if field == "" {
				field = f
			} else if f != field {
				return nil, "", matchSearchError("query_shard_exception", "Clauses must have same field.")
			}
			spans = append(spans, sp...)
		}
		return spans, field, nil
	case "span_first":
		match, _ := inner["match"].(map[string]any)
		end := 1
		if v, ok := toFloat(inner["end"]); ok {
			end = int(v)
		}
		spans, field, err := c.spanQuery(match)
		if err != nil {
			return nil, "", err
		}
		out := []intervalSpan{}
		for _, sp := range spans {
			if sp.start < end {
				out = append(out, sp)
			}
		}
		return out, field, nil
	case "span_not":
		include, _ := inner["include"].(map[string]any)
		exclude, _ := inner["exclude"].(map[string]any)
		inc, field, err := c.spanQuery(include)
		if err != nil {
			return nil, "", err
		}
		exc, _, err := c.spanQuery(exclude)
		if err != nil {
			return nil, "", err
		}
		pre, hasPre := toFloat(inner["pre"])
		post, hasPost := toFloat(inner["post"])
		dist, hasDist := toFloat(inner["dist"])
		out := []intervalSpan{}
		for _, a := range inc {
			drop := false
			for _, b := range exc {
				if hasPre {
					if b.end < a.start && float64(a.start-b.end) <= pre {
						drop = true
					}
				} else if hasPost {
					if b.start > a.end && float64(b.start-a.end) <= post {
						drop = true
					}
				} else if hasDist {
					d := b.start - a.end
					if d < 0 {
						d = a.start - b.end
					}
					if d < 0 {
						d = 0
					}
					if float64(d) <= dist {
						drop = true
					}
				} else if b.start <= a.end && a.start <= b.end {
					drop = true
				}
				if drop {
					break
				}
			}
			if !drop {
				out = append(out, a)
			}
		}
		return out, field, nil
	case "span_containing":
		big, _ := inner["big"].(map[string]any)
		little, _ := inner["little"].(map[string]any)
		bigs, field, err := c.spanQuery(big)
		if err != nil {
			return nil, "", err
		}
		littles, _, err := c.spanQuery(little)
		if err != nil {
			return nil, "", err
		}
		out := []intervalSpan{}
		for _, a := range bigs {
			for _, b := range littles {
				if a.start <= b.start && a.end >= b.end {
					out = append(out, a)
					break
				}
			}
		}
		return out, field, nil
	case "span_within":
		big, _ := inner["big"].(map[string]any)
		little, _ := inner["little"].(map[string]any)
		bigs, field, err := c.spanQuery(big)
		if err != nil {
			return nil, "", err
		}
		littles, _, err := c.spanQuery(little)
		if err != nil {
			return nil, "", err
		}
		out := []intervalSpan{}
		for _, b := range littles {
			for _, a := range bigs {
				if a.start <= b.start && a.end >= b.end {
					out = append(out, b)
					break
				}
			}
		}
		return out, field, nil
	case "span_multi":
		match, _ := inner["match"].(map[string]any)
		mk, mbm := spanKind(match)
		if mk != "prefix" && mk != "wildcard" && mk != "regexp" && mk != "fuzzy" {
			return nil, "", errParsing("[span_multi] [match] must be of type multi term query")
		}
		field, val := "", ""
		for k, v := range mbm {
			field = k
			if m, ok := v.(map[string]any); ok {
				if x, ok := m["value"]; ok {
					val, _ = toString(x)
				} else if x, ok := m["pattern"]; ok {
					val, _ = toString(x)
				} else if x, ok := m["prefix"]; ok {
					val, _ = toString(x)
				} else if x, ok := m["term"]; ok {
					val, _ = toString(x)
				}
			} else {
				val, _ = toString(v)
			}
			break
		}
		spans := []intervalSpan{}
		positions := c.intervalPositions(field)
		switch mk {
		case "prefix":
			val = strings.ToLower(val)
			for _, p := range positions {
				if strings.HasPrefix(p.term, val) {
					spans = append(spans, intervalSpan{p.pos, p.pos})
				}
			}
		case "wildcard":
			re := wildcardRegex(strings.ToLower(val))
			for _, p := range positions {
				if re.MatchString(p.term) {
					spans = append(spans, intervalSpan{p.pos, p.pos})
				}
			}
		case "regexp":
			re, rerr := regexp.Compile(val)
			if rerr != nil {
				return nil, "", errParsing("invalid regexp")
			}
			for _, p := range positions {
				if re.MatchString(p.term) {
					spans = append(spans, intervalSpan{p.pos, p.pos})
				}
			}
		case "fuzzy":
			for _, p := range positions {
				if fuzzyDistance(p.term, val, true) <= autoFuzziness(val) {
					spans = append(spans, intervalSpan{p.pos, p.pos})
				}
			}
		}
		return spans, field, nil
	case "field_masking_span":
		query, _ := inner["query"].(map[string]any)
		target, _ := inner["field"].(string)
		spans, _, err := c.spanQuery(query)
		if err != nil {
			return nil, "", err
		}
		return spans, target, nil
	}
	return nil, "", errParsing("unknown span query [" + kind + "]")
}

func (c *queryCtx) evalSpanNear(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("span_near malformed")
	}
	spans, _, err := c.spanQuery(map[string]any{"span_near": bm})
	if err != nil {
		return false, 0, err
	}
	return len(spans) > 0, 1, nil
}

func (c *queryCtx) evalSpanOr(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("span_or malformed")
	}
	spans, _, err := c.spanQuery(map[string]any{"span_or": bm})
	if err != nil {
		return false, 0, err
	}
	return len(spans) > 0, 1, nil
}

func (c *queryCtx) evalSpanFirst(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("span_first malformed")
	}
	spans, _, err := c.spanQuery(map[string]any{"span_first": bm})
	if err != nil {
		return false, 0, err
	}
	return len(spans) > 0, 1, nil
}

func (c *queryCtx) evalSpanNot(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("span_not malformed")
	}
	spans, _, err := c.spanQuery(map[string]any{"span_not": bm})
	if err != nil {
		return false, 0, err
	}
	return len(spans) > 0, 1, nil
}

func (c *queryCtx) evalSpanContaining(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("span_containing malformed")
	}
	spans, _, err := c.spanQuery(map[string]any{"span_containing": bm})
	if err != nil {
		return false, 0, err
	}
	return len(spans) > 0, 1, nil
}

func (c *queryCtx) evalSpanWithin(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("span_within malformed")
	}
	spans, _, err := c.spanQuery(map[string]any{"span_within": bm})
	if err != nil {
		return false, 0, err
	}
	return len(spans) > 0, 1, nil
}

func (c *queryCtx) evalSpanMulti(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("span_multi malformed")
	}
	spans, _, err := c.spanQuery(map[string]any{"span_multi": bm})
	if err != nil {
		return false, 0, err
	}
	return len(spans) > 0, 1, nil
}

type intervalSpan struct {
	start int
	end   int
}

func (c *queryCtx) evalIntervals(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("intervals malformed")
	}
	field := ""
	var rules map[string]any
	for k, v := range bm {
		if k == "boost" || k == "_name" {
			continue
		}
		field = k
		rules, _ = v.(map[string]any)
		break
	}
	if field == "" || rules == nil {
		return false, 0, errParsing("intervals malformed")
	}
	def, known := mappingFieldDef(c.idx, field)
	if !known {
		return false, 0, nil
	}
	ftype, _ := def["type"].(string)
	if ftype == "match_only_text" {
		return false, 0, matchSearchError("query_shard_exception",
			"failed to create query: Cannot create intervals over field ["+field+"] with no positions indexed")
	}
	if ftype != "" && ftype != "text" && ftype != "match_only_text" && ftype != "search_as_you_type" {
		return false, 0, matchSearchError("query_shard_exception",
			"Can only use interval queries on text fields - not on ["+field+"] which is of type ["+ftype+"]")
	}
	ruleKeys := []string{}
	for k := range rules {
		if k == "boost" || k == "_name" {
			continue
		}
		ruleKeys = append(ruleKeys, k)
	}
	if len(ruleKeys) > 1 {
		sort.Strings(ruleKeys)
		return false, 0, errParsing("Only one interval rule can be specified, found [" +
			strings.Join(ruleKeys, "] and [") + "]")
	}
	if len(ruleKeys) == 0 {
		return false, 0, nil
	}
	if !isKnownIntervalType(ruleKeys[0]) {
		return false, 0, errParsing("Unknown interval type [" + ruleKeys[0] + "]")
	}
	positions := c.intervalPositions(field)
	spans, err := c.intervalRule(rules, positions, field)
	if err != nil {
		return false, 0, err
	}
	if len(spans) == 0 {
		return false, 0, nil
	}
	return true, 1, nil
}

func isKnownIntervalType(k string) bool {
	switch k {
	case "match", "prefix", "wildcard", "regexp", "fuzzy", "all_of", "any_of":
		return true
	}
	return false
}

func (c *queryCtx) intervalPositions(field string) []analyzedToken {
	vals := c.mustValues(field)
	def, _ := mappingFieldDef(c.idx, field)
	gap := 100
	if g, ok := toFloat(def["position_increment_gap"]); ok {
		gap = int(g)
	}
	out := []analyzedToken{}
	last := -1
	for _, v := range vals {
		s, _ := valueScalarString(v)
		toks := analyzeTokensWithIndex(c.idx, c.indexAnalyzerName(field), s)
		base := 0
		if last >= 0 {
			base = last + gap + 1
		}
		for _, t := range toks {
			out = append(out, analyzedToken{term: t.term, pos: t.pos + base})
			if t.pos+base > last {
				last = t.pos + base
			}
		}
	}
	return out
}

func intervalTermSpans(positions []analyzedToken, terms []string) [][]intervalSpan {
	out := make([][]intervalSpan, 0, len(terms))
	for _, term := range terms {
		list := []intervalSpan{}
		for _, p := range positions {
			if p.term == term {
				list = append(list, intervalSpan{p.pos, p.pos})
			}
		}
		if len(list) == 0 {
			return nil
		}
		out = append(out, list)
	}
	return out
}

func combineOrderedIntervals(lists [][]intervalSpan, maxGaps int) []intervalSpan {
	if len(lists) == 0 {
		return nil
	}
	cur := lists[0]
	for i := 1; i < len(lists); i++ {
		next := []intervalSpan{}
		for _, a := range cur {
			for _, b := range lists[i] {
				if b.start <= a.end {
					continue
				}
				if maxGaps >= 0 && b.start-a.end-1 > maxGaps {
					continue
				}
				next = append(next, intervalSpan{a.start, b.end})
			}
		}
		cur = next
	}
	return cur
}

func combineUnorderedIntervals(lists [][]intervalSpan, maxGaps int) []intervalSpan {
	if len(lists) == 0 {
		return nil
	}
	cur := lists[0]
	for i := 1; i < len(lists); i++ {
		next := []intervalSpan{}
		for _, a := range cur {
			for _, b := range lists[i] {
				gap := 0
				if b.start > a.end {
					gap = b.start - a.end - 1
				} else if a.start > b.end {
					gap = a.start - b.end - 1
				}
				if maxGaps >= 0 && gap > maxGaps {
					continue
				}
				start, end := a.start, a.end
				if b.start < start {
					start = b.start
				}
				if b.end > end {
					end = b.end
				}
				next = append(next, intervalSpan{start, end})
			}
		}
		cur = next
	}
	return cur
}

func (c *queryCtx) intervalRule(rule map[string]any, positions []analyzedToken, field string) ([]intervalSpan, *apiError) {
	kind := ""
	var bm map[string]any
	for k, v := range rule {
		if k == "boost" || k == "_name" {
			continue
		}
		kind = k
		bm, _ = v.(map[string]any)
		break
	}
	if kind == "" || bm == nil {
		return nil, errParsing("intervals malformed")
	}
	var spans []intervalSpan
	switch kind {
	case "match":
		q, _ := bm["query"].(string)
		analyzer := ""
		if a, ok := bm["analyzer"].(string); ok {
			analyzer = a
		} else {
			analyzer = c.searchAnalyzerName(field, bm)
		}
		toks := analyzeTokensWithIndex(c.idx, analyzer, q)
		terms := []string{}
		for _, t := range toks {
			terms = append(terms, t.term)
		}
		lists := intervalTermSpans(positions, terms)
		if lists == nil {
			return nil, nil
		}
		maxGaps := -1
		if g, ok := toFloat(bm["max_gaps"]); ok {
			maxGaps = int(g)
		}
		ordered, _ := bm["ordered"].(bool)
		if ordered {
			spans = combineOrderedIntervals(lists, maxGaps)
		} else {
			spans = combineUnorderedIntervals(lists, maxGaps)
		}
	case "prefix":
		pat, _ := bm["prefix"].(string)
		pat = strings.ToLower(pat)
		for _, p := range positions {
			if strings.HasPrefix(p.term, pat) {
				spans = append(spans, intervalSpan{p.pos, p.pos})
			}
		}
	case "wildcard":
		pat, _ := bm["pattern"].(string)
		re := wildcardRegex(strings.ToLower(pat))
		for _, p := range positions {
			if re.MatchString(p.term) {
				spans = append(spans, intervalSpan{p.pos, p.pos})
			}
		}
	case "regexp":
		pat, _ := bm["pattern"].(string)
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, errParsing("invalid regexp")
		}
		for _, p := range positions {
			if re.MatchString(p.term) {
				spans = append(spans, intervalSpan{p.pos, p.pos})
			}
		}
	case "fuzzy":
		term, _ := bm["term"].(string)
		fuzz := autoFuzziness(term)
		if v, ok := bm["fuzziness"]; ok {
			if n, ok2 := fuzzinessValue(v, term); ok2 {
				fuzz = n
			}
		}
		prefixLen := 0
		if pl, ok := toFloat(bm["prefix_length"]); ok {
			prefixLen = int(pl)
		}
		for _, p := range positions {
			if len(p.term) <= prefixLen || len(term) <= prefixLen {
				continue
			}
			if p.term[:prefixLen] != term[:prefixLen] {
				continue
			}
			if fuzzyDistance(p.term, term, true) <= fuzz {
				spans = append(spans, intervalSpan{p.pos, p.pos})
			}
		}
	case "all_of":
		subs, _ := bm["intervals"].([]any)
		lists := [][]intervalSpan{}
		for _, sub := range subs {
			sm, _ := sub.(map[string]any)
			sp, err := c.intervalRule(sm, positions, field)
			if err != nil {
				return nil, err
			}
			lists = append(lists, sp)
		}
		maxGaps := -1
		if g, ok := toFloat(bm["max_gaps"]); ok {
			maxGaps = int(g)
		}
		ordered, _ := bm["ordered"].(bool)
		if ordered {
			spans = combineOrderedIntervals(lists, maxGaps)
		} else {
			spans = combineUnorderedIntervals(lists, maxGaps)
		}
	case "any_of":
		subs, _ := bm["intervals"].([]any)
		for _, sub := range subs {
			sm, _ := sub.(map[string]any)
			sp, err := c.intervalRule(sm, positions, field)
			if err != nil {
				return nil, err
			}
			spans = append(spans, sp...)
		}
	default:
		return nil, errParsing("Unknown interval type [" + kind + "]")
	}
	if f, ok := bm["filter"].(map[string]any); ok {
		filtered, err := c.filterIntervalSpans(spans, f, positions, field)
		if err != nil {
			return nil, err
		}
		spans = filtered
	}
	return spans, nil
}

func (c *queryCtx) filterIntervalSpans(spans []intervalSpan, filter map[string]any, positions []analyzedToken, field string) ([]intervalSpan, *apiError) {
	if len(spans) == 0 {
		return spans, nil
	}
	kind := ""
	var bm map[string]any
	for k, v := range filter {
		kind = k
		bm, _ = v.(map[string]any)
		break
	}
	if kind == "script" {
		src, _ := bm["source"].(string)
		parts := strings.Fields(src)
		if len(parts) < 3 {
			return spans, nil
		}
		op := parts[1]
		limit, _ := strconv.ParseFloat(parts[2], 64)
		out := []intervalSpan{}
		for _, sp := range spans {
			val := float64(sp.start)
			if strings.HasSuffix(parts[0], ".end") {
				val = float64(sp.end)
			}
			keep := false
			switch op {
			case ">":
				keep = val > limit
			case ">=":
				keep = val >= limit
			case "<":
				keep = val < limit
			case "<=":
				keep = val <= limit
			case "==":
				keep = val == limit
			}
			if keep {
				out = append(out, sp)
			}
		}
		return out, nil
	}
	other, err := c.intervalRule(bm, positions, field)
	if err != nil {
		return nil, err
	}
	out := []intervalSpan{}
	for _, sp := range spans {
		keep := false
		for _, o := range other {
			switch kind {
			case "before":
				keep = o.start > sp.end
			case "after":
				keep = o.end < sp.start
			case "containing":
				keep = sp.start <= o.start && sp.end >= o.end
			case "not_containing":
				keep = !(sp.start <= o.start && sp.end >= o.end)
			case "contained_by":
				keep = o.start <= sp.start && o.end >= sp.end
			case "not_contained_by":
				keep = !(o.start <= sp.start && o.end >= sp.end)
			case "not_overlapping":
				keep = !(o.start <= sp.end && sp.start <= o.end)
			}
			if keep {
				break
			}
		}
		if kind == "not_containing" || kind == "not_contained_by" || kind == "not_overlapping" {
			if len(other) == 0 {
				keep = true
			}
		}
		if keep {
			out = append(out, sp)
		}
	}
	return out, nil
}

func (c *queryCtx) evalTermSingle(field string, bm map[string]any) (bool, float64, *apiError) {
	q, _ := bm["query"].(string)
	if field == "" {
		for f := range c.valuesMap() {
			ok, _, _ := c.evalTerm(f, q, 1.0)
			if ok {
				return true, 1, nil
			}
		}
		return false, 0, nil
	}
	ok, _, err := c.evalTerm(field, q, 1.0)
	return ok, 1, err
}

func (c *queryCtx) evalIntervalsSimple(pred func(field string, tokens []string) bool) bool {
	for f, vals := range c.valuesMap() {
		tokens := []string{}
		for _, v := range vals {
			s, ok := valueScalarString(v)
			if !ok {
				continue
			}
			tokens = append(tokens, analyze(s)...)
		}
		if pred(f, tokens) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// more_like_this
// ---------------------------------------------------------------------------

func mltFieldTokens(idx *Index, field, text string) []string {
	if def, ok := mappingFieldDef(idx, field); ok {
		switch t, _ := def["type"].(string); t {
		case "keyword", "constant_keyword", "wildcard":
			return []string{text}
		}
	}
	return analyze(text)
}

func mltLikeTexts(c *queryCtx, raw any, fields []string, include bool, excluded map[string]bool) []string {
	var texts []string
	add := func(v any) {
		switch t := v.(type) {
		case string:
			texts = append(texts, t)
		case map[string]any:
			if doc, ok := t["doc"].(map[string]any); ok {
				texts = append(texts, collectDocTexts(doc, fields)...)
				return
			}
			if id, ok := toString(t["_id"]); ok {
				if sd, live := c.idx.Docs[id]; live {
					texts = append(texts, collectDocTexts(sd, fields)...)
					if !include {
						excluded[id] = true
					}
				}
				return
			}
			texts = append(texts, collectDocTexts(t, fields)...)
		}
	}
	switch t := raw.(type) {
	case []any:
		for _, e := range t {
			add(e)
		}
	default:
		add(raw)
	}
	return texts
}

func mltTermSet(c *queryCtx, raw any, fields []string) map[string]bool {
	out := map[string]bool{}
	for _, text := range mltLikeTexts(c, raw, fields, true, map[string]bool{}) {
		if len(fields) == 0 {
			for _, tok := range analyze(text) {
				out[tok] = true
			}
			continue
		}
		for _, f := range fields {
			for _, tok := range mltFieldTokens(c.idx, f, text) {
				out[tok] = true
			}
		}
	}
	return out
}

func mltDocHasTerm(idx *Index, doc map[string]any, fields []string, term string) bool {
	found := false
	for _, f := range fields {
		v, ok := fieldValue(doc, f)
		if !ok {
			continue
		}
		var walk func(any)
		walk = func(x any) {
			switch t := x.(type) {
			case string:
				for _, tok := range mltFieldTokens(idx, f, t) {
					if tok == term {
						found = true
					}
				}
			case []any:
				for _, e := range t {
					walk(e)
				}
			}
		}
		walk(v)
		if found {
			return true
		}
	}
	return false
}

func (c *queryCtx) evalMoreLikeThis(bm map[string]any) (bool, float64, *apiError) {
	var fields []string
	switch t := bm["fields"].(type) {
	case []any:
		for _, e := range t {
			if s, ok := toString(e); ok {
				fields = append(fields, s)
			}
		}
	case string:
		fields = []string{t}
	}
	likeRaw, hasLike := bm["like"]
	if !hasLike {
		return false, 0, errParsing("more_like_this requires 'like' to be specified")
	}
	excluded := map[string]bool{}
	include, _ := bm["include"].(bool)
	texts := mltLikeTexts(c, likeRaw, fields, include, excluded)
	if excluded[c.id] || len(texts) == 0 {
		return false, 0, nil
	}
	targets := fields
	if len(targets) == 0 {
		for f := range c.valuesMap() {
			targets = append(targets, f)
		}
	}
	stop := map[string]bool{}
	switch t := bm["stop_words"].(type) {
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok {
				stop[s] = true
			}
		}
	case string:
		if t != "" && t != "none" && t != "_none_" {
			for _, s := range strings.Split(t, ",") {
				stop[strings.TrimSpace(s)] = true
			}
		}
	}
	unlike := map[string]bool{}
	if raw, ok := bm["unlike"]; ok {
		unlike = mltTermSet(c, raw, fields)
	}
	tf := map[string]int{}
	for _, text := range texts {
		var toks []string
		if len(fields) == 0 {
			toks = analyze(text)
		} else {
			for _, f := range fields {
				toks = append(toks, mltFieldTokens(c.idx, f, text)...)
			}
		}
		for _, tok := range toks {
			if stop[tok] || unlike[tok] {
				continue
			}
			tf[tok]++
		}
	}
	minTermFreq := 2
	if n, ok := toFloat(bm["min_term_freq"]); ok {
		minTermFreq = int(n)
	}
	maxTermFreq := math.MaxInt32
	if n, ok := toFloat(bm["max_term_freq"]); ok {
		maxTermFreq = int(n)
	}
	minDocFreq := 5
	if n, ok := toFloat(bm["min_doc_freq"]); ok {
		minDocFreq = int(n)
	}
	maxDocFreq := math.MaxInt32
	if n, ok := toFloat(bm["max_doc_freq"]); ok {
		maxDocFreq = int(n)
	}
	terms := map[string]bool{}
	for term, freq := range tf {
		if freq < minTermFreq || freq > maxTermFreq {
			continue
		}
		df := 0
		for _, id := range c.idx.Order {
			doc, live := c.idx.Docs[id]
			if !live {
				continue
			}
			if mltDocHasTerm(c.idx, doc, targets, term) {
				df++
			}
		}
		if df < minDocFreq || df > maxDocFreq {
			continue
		}
		terms[term] = true
	}
	if len(terms) == 0 {
		return false, 0, nil
	}
	minMatch := 0
	if msm, ok := bm["minimum_should_match"]; ok {
		minMatch = minimumShouldMatch(msm, len(terms), 0)
	} else {
		minMatch = minimumShouldMatch("30%", len(terms), 0)
		if minMatch < 1 {
			minMatch = 1
		}
	}
	matched := 0
	for term := range terms {
		if mltDocHasTerm(c.idx, c.doc, targets, term) {
			matched++
		}
	}
	if matched < minMatch {
		return false, 0, nil
	}
	boost := 1.0
	if b, ok := toFloat(bm["boost"]); ok {
		boost = b
	}
	return true, float64(matched) * boost, nil
}

func collectDocTexts(doc map[string]any, fields []string) []string {
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			out = append(out, t)
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
	if len(fields) > 0 {
		for _, f := range fields {
			if v, ok := fieldValue(doc, f); ok {
				walk(v)
			}
		}
		return out
	}
	for k, v := range doc {
		if strings.HasPrefix(k, "_") {
			continue
		}
		walk(v)
	}
	return out
}

// ---------------------------------------------------------------------------
// knn
// ---------------------------------------------------------------------------

func (c *queryCtx) evalKnn(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[knn] query malformed")
	}
	field, _ := bm["field"].(string)
	vecRaw, _ := bm["query_vector"].([]any)
	var qv []float64
	for _, e := range vecRaw {
		if f, ok := toFloat(e); ok {
			qv = append(qv, f)
		}
	}
	if field == "" || len(qv) == 0 {
		return false, 0, errParsing("[knn] requires field and query_vector")
	}
	vals, present := c.values(field)
	if !present {
		return false, 0, nil
	}
	var dv []float64
	for _, e := range vals {
		if f, ok := toFloat(e); ok {
			dv = append(dv, f)
		}
	}
	if len(dv) != len(qv) {
		return false, 0, nil
	}
	dot, na, nb := 0.0, 0.0, 0.0
	for i := range qv {
		dot += qv[i] * dv[i]
		na += qv[i] * qv[i]
		nb += dv[i] * dv[i]
	}
	if na == 0 || nb == 0 {
		return false, 0, nil
	}
	cos := dot / (math.Sqrt(na) * math.Sqrt(nb))
	sim := (cos + 1) / 2
	return true, sim, nil
}

// ---------------------------------------------------------------------------
// values map access helper
// ---------------------------------------------------------------------------

func (c *queryCtx) valuesMap() map[string][]any {
	if c.vals != nil {
		return c.vals
	}
	return c.idx.DocValues[c.id]
}

// ---------------------------------------------------------------------------
// function_score
// ---------------------------------------------------------------------------

func hasScoreFunctionKey(bm map[string]any) bool {
	for _, k := range []string{"weight", "field_value_factor", "random_score", "script_score",
		"gauss", "exp", "linear"} {
		if _, ok := bm[k]; ok {
			return true
		}
	}
	return false
}

func javaDoubleString(f float64) string {
	if math.IsInf(f, 1) {
		return "Infinity"
	}
	if math.IsInf(f, -1) {
		return "-Infinity"
	}
	if math.IsNaN(f) {
		return "NaN"
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

func newFunctionScoreFailure(marker, reason string) *apiError {
	return &apiError{Type: "search_phase_execution_exception", Status: 400,
		Reason: reason, SearchFailure: marker}
}

func parseDecayDurationMillis(v any) (float64, bool) {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		i := 0
		for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.' || s[i] == '-' || s[i] == '+') {
			i++
		}
		num, err := strconv.ParseFloat(strings.TrimSpace(s[:i]), 64)
		if err != nil {
			return 0, false
		}
		mult := 1.0
		switch strings.TrimSpace(s[i:]) {
		case "d":
			mult = 86400000
		case "h":
			mult = 3600000
		case "m":
			mult = 60000
		case "s":
			mult = 1000
		case "ms":
			mult = 1
		case "micros":
			mult = 0.001
		case "nanos":
			mult = 0.000001
		}
		return num * mult, true
	default:
		return toFloat(v)
	}
}

func decayDateMillis(def map[string]any, v any) (int64, bool) {
	if s, ok := v.(string); ok {
		return parseDateAny(def, s)
	}
	return integerOf(v)
}

func (c *queryCtx) evalFunctionScore(body any) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("[function_score] query malformed")
	}
	inner, ok := bm["query"].(map[string]any)
	if !ok {
		inner = map[string]any{"match_all": map[string]any{}}
	}
	matched, base, err := c.eval(inner)
	if err != nil {
		return false, 0, err
	}
	if !matched {
		return false, 0, nil
	}
	if b, ok := toFloat(bm["boost"]); ok {
		base *= b
	}
	functions := asQueryList(bm["functions"])
	if len(functions) == 0 {
		if f, ok := bm["function"].(map[string]any); ok {
			functions = []any{f}
		} else if hasScoreFunctionKey(bm) {
			functions = []any{bm}
		}
	}
	scoreMode, _ := bm["score_mode"].(string)
	if scoreMode == "" {
		scoreMode = "multiply"
	}
	type appliedFunction struct {
		value  float64
		weight float64
	}
	var applied []appliedFunction
	for _, fnRaw := range functions {
		fn, ok := fnRaw.(map[string]any)
		if !ok {
			continue
		}
		if filter, ok := fn["filter"].(map[string]any); ok {
			fm, _, ferr := c.eval(filter)
			if ferr != nil {
				return false, 0, ferr
			}
			if !fm {
				continue
			}
		}
		weight := 1.0
		if w, ok := toFloat(fn["weight"]); ok {
			weight = w
		}
		fv, ferr := c.applyScoreFunction(fn, base)
		if ferr != nil {
			return false, 0, ferr
		}
		applied = append(applied, appliedFunction{value: fv, weight: weight})
	}
	finalScore := 1.0
	if len(applied) > 0 {
		switch scoreMode {
		case "first":
			finalScore = applied[0].value * applied[0].weight
		case "max":
			finalScore = applied[0].value * applied[0].weight
			for _, a := range applied[1:] {
				if v := a.value * a.weight; v > finalScore {
					finalScore = v
				}
			}
		case "min":
			finalScore = applied[0].value * applied[0].weight
			for _, a := range applied[1:] {
				if v := a.value * a.weight; v < finalScore {
					finalScore = v
				}
			}
		case "sum":
			finalScore = 0
			for _, a := range applied {
				finalScore += a.value * a.weight
			}
		case "avg":
			weighted, totalWeight := 0.0, 0.0
			for _, a := range applied {
				weighted += a.value * a.weight
				totalWeight += a.weight
			}
			if totalWeight != 0 {
				finalScore = weighted / totalWeight
			} else {
				finalScore = 0
			}
		default:
			finalScore = 1
			for _, a := range applied {
				finalScore *= a.value * a.weight
			}
		}
	}
	boostMode, _ := bm["boost_mode"].(string)
	var combined float64
	switch boostMode {
	case "replace":
		combined = finalScore
	case "sum":
		combined = base + finalScore
	case "avg":
		combined = (base + finalScore) / 2
	case "max":
		combined = math.Max(base, finalScore)
	case "min":
		combined = math.Min(base, finalScore)
	default:
		combined = base * finalScore
	}
	if mb, ok := toFloat(bm["max_boost"]); ok && combined > mb {
		combined = mb
	}
	if ms, ok := toFloat(bm["min_score"]); ok && combined < ms {
		return false, 0, nil
	}
	return true, combined, nil
}

func (c *queryCtx) applyScoreFunction(fn map[string]any, baseScore float64) (float64, *apiError) {
	if fvf, ok := fn["field_value_factor"].(map[string]any); ok {
		field, _ := fvf["field"].(string)
		factor := 1.0
		if v, ok := toFloat(fvf["factor"]); ok {
			factor = v
		}
		modifier, _ := fvf["modifier"].(string)
		missingRaw, hasMissing := fvf["missing"]
		var value float64
		vals, present := c.values(field)
		if present && len(vals) > 0 {
			n, ok := toFloat(vals[0])
			if !ok {
				return 0, newFunctionScoreFailure("fvf_unmapped",
					"Unable to find a field mapper for field ["+field+"]. No 'missing' value defined.")
			}
			value = n
		} else if hasMissing {
			n, ok := toFloat(missingRaw)
			if !ok {
				return 0, newFunctionScoreFailure("fvf_missing",
					"Missing value for field ["+field+"]")
			}
			value = n
		} else if _, mapped := mappingFieldDef(c.idx, field); !mapped {
			return 0, newFunctionScoreFailure("fvf_unmapped",
				"Unable to find a field mapper for field ["+field+"]. No 'missing' value defined.")
		} else {
			return 0, newFunctionScoreFailure("fvf_missing",
				"Missing value for field ["+field+"]")
		}
		result := value * factor
		switch modifier {
		case "log":
			result = math.Log10(result)
		case "log1p":
			result = math.Log10(1 + result)
		case "log2p":
			result = math.Log10(2 + result)
		case "ln":
			result = math.Log(result)
		case "ln1p":
			result = math.Log(1 + result)
		case "ln2p":
			result = math.Log(2 + result)
		case "square":
			result = result * result
		case "sqrt":
			result = math.Sqrt(result)
		case "reciprocal":
			result = 1 / result
		}
		if result < 0 || math.IsNaN(result) {
			reason := "field value function must not produce negative scores, but got: [" +
				javaDoubleString(result) + "] for field value: [" + javaDoubleString(value) +
				"]; consider using log1p or log2p instead of log to avoid negative scores"
			return 0, newFunctionScoreFailure("fvf_log", reason)
		}
		return result, nil
	}
	if rs, ok := fn["random_score"].(map[string]any); ok {
		seed := int64(0)
		if s, ok := rs["seed"]; ok {
			seed, _ = integerOf(s)
		}
		if seed == 0 {
			seed = 42
		}
		return pseudoRandom(c.id, seed), nil
	}
	if ss, ok := fn["script_score"].(map[string]any); ok {
		cs, se, ae := prepareScript(ss["script"], "score")
		if se != nil || ae != nil {
			return 0, scriptErrForShard(c.idx, se, ae, true)
		}
		v, rerr := cs.run(c, runOpts{score: float64(float32(baseScore))})
		if rerr != nil {
			return 0, scriptShardError(c.idx, rerr)
		}
		score := plF64(v)
		if score < 0 {
			return 0, plainShardError(c.idx, errBad("script score function must not produce negative scores, but got: ["+plFmtDouble(score)+"]"))
		}
		return float64(float32(score)), nil
	}
	for _, fnName := range []string{"gauss", "linear", "exp"} {
		decRaw, ok := fn[fnName].(map[string]any)
		if !ok {
			continue
		}
		field := ""
		var spec map[string]any
		for k, v := range decRaw {
			if k == "boost" || k == "_name" {
				continue
			}
			field = k
			spec, _ = v.(map[string]any)
			break
		}
		if field == "" || spec == nil {
			return 1, nil
		}
		vals, present := c.values(field)
		if !present || len(vals) == 0 {
			return 1, nil
		}
		def, _ := mappingFieldDef(c.idx, field)
		ftype, _ := def["type"].(string)
		decayV := 0.5
		if d, ok := toFloat(spec["decay"]); ok {
			decayV = d
		}
		geo := ftype == "geo_point" || ftype == "geo_shape"
		if !geo {
			switch spec["origin"].(type) {
			case map[string]any, []any:
				geo = true
			}
		}
		var dist, scale, offset float64
		if geo {
			olat, olon, ok := geoPointOf(spec["origin"])
			if !ok {
				return 1, nil
			}
			lat, lon, ok := geoPointOf(vals[0])
			if !ok {
				return 1, nil
			}
			dist = haversineMeters(olat, olon, lat, lon)
			if s, ok := parseDistanceMeters(spec["scale"]); ok {
				scale = s
			}
			if o, ok := parseDistanceMeters(spec["offset"]); ok {
				offset = o
			}
		} else if ftype == "date" || ftype == "date_nanos" {
			origin, ok := decayDateMillis(def, spec["origin"])
			if !ok {
				origin = 0
			}
			val, ok := decayDateMillis(def, vals[0])
			if !ok {
				return 1, nil
			}
			dist = math.Abs(float64(val - origin))
			if s, ok := parseDecayDurationMillis(spec["scale"]); ok {
				scale = s
			}
			if o, ok := parseDecayDurationMillis(spec["offset"]); ok {
				offset = o
			}
		} else {
			val, _ := toFloat(vals[0])
			origin := 0.0
			if o, ok := toFloat(spec["origin"]); ok {
				origin = o
			}
			if s, ok := toFloat(spec["scale"]); ok {
				scale = s
			}
			if o, ok := toFloat(spec["offset"]); ok {
				offset = o
			}
			dist = math.Abs(val - origin)
		}
		if dist <= offset {
			return 1, nil
		}
		if scale <= 0 {
			return 1, nil
		}
		x := (dist - offset) / scale
		switch fnName {
		case "gauss":
			return math.Pow(decayV, x*x), nil
		case "exp":
			return math.Pow(decayV, x), nil
		case "linear":
			v := 1 - (1-decayV)*x
			if v < 0 {
				return 0, nil
			}
			return v, nil
		}
	}
	return 1, nil
}

func pseudoRandom(id string, seed int64) float64 {
	h := uint64(seed)*1469598103934665603 + 14695981039346656037
	for i := 0; i < len(id); i++ {
		h ^= uint64(id[i])
		h *= 1099511628211
	}
	return float64(h>>11) / float64(1<<53)
}

// ---------------------------------------------------------------------------
// query_string parser
// ---------------------------------------------------------------------------

type qsParser struct {
	tokens       []string
	pos          int
	default_     string
	fields       []string
	operator     string
	simple       bool
	c            *queryCtx
	allowLeading bool
	hasFields    bool
	plusUsed     bool
	prefixOp     bool
	fuzzyOp      bool
	phraseOp     bool
	slopOp       bool
	notOp        bool
	andOp        bool
	quoteSuffix  string
}

func tokenizeQueryString(s string) []string {
	var out []string
	i := 0
	for i < len(s) {
		ch := s[i]
		if ch == ' ' || ch == '\t' || ch == '\n' {
			i++
			continue
		}
		if ch == '"' {
			j := i + 1
			for j < len(s) && s[j] != '"' {
				j++
			}
			if j < len(s) {
				j++
			}
			for j < len(s) && s[j] != ' ' && s[j] != '\t' && s[j] != '\n' {
				j++
			}
			out = append(out, s[i:j])
			i = j
			continue
		}
		if ch == '(' || ch == ')' {
			out = append(out, string(ch))
			i++
			continue
		}
		j := i
		for j < len(s) && s[j] != ' ' && s[j] != '\t' && s[j] != '\n' && s[j] != '(' && s[j] != ')' {
			if s[j] == '\\' && j+1 < len(s) {
				j += 2
				continue
			}
			j++
		}
		part := s[i:j]
		if strings.ContainsAny(part, "[{") && !strings.ContainsAny(part, "]}") {
			for j < len(s) && s[j] != ']' && s[j] != '}' {
				j++
			}
			if j < len(s) {
				j++
			}
		}
		out = append(out, s[i:j])
		i = j
	}
	return out
}

func (p *qsParser) parse() (map[string]any, *apiError) {
	q, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	return q, nil
}

func (p *qsParser) parseOr() (map[string]any, *apiError) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.pos < len(p.tokens) {
		tok := p.tokens[p.pos]
		if tok == "OR" || tok == "||" {
			p.pos++
			if p.pos >= len(p.tokens) {
				return nil, errParsing("trailing operator")
			}
			right, err := p.parseAnd()
			if err != nil {
				return nil, err
			}
			left = boolQuery("should", []any{left, right}, 0)
			continue
		}
		break
	}
	return left, nil
}

func (p *qsParser) parseAnd() (map[string]any, *apiError) {
	must := []any{}
	should := []any{}
	mustNot := []any{}
	forceMust := false
	for p.pos < len(p.tokens) {
		tok := p.tokens[p.pos]
		if p.simple && (tok == "AND" || tok == "OR" || tok == "NOT") {
			// plain term in simple query syntax
		} else if tok == "OR" || tok == "||" || tok == ")" {
			break
		}
		if !p.simple && (tok == "AND" || tok == "&&") {
			p.pos++
			forceMust = true
			continue
		}
		if !p.simple && tok == "NOT" {
			p.pos++
			inner, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			mustNot = append(mustNot, inner)
			forceMust = false
			continue
		}
		sign := 0
		if len(tok) > 1 && (strings.HasPrefix(tok, "+") || strings.HasPrefix(tok, "-") || strings.HasPrefix(tok, "!")) {
			switch tok[0] {
			case '+':
				sign = 1
				p.plusUsed = true
			case '-', '!':
				sign = -1
			}
			p.tokens[p.pos] = tok[1:]
		} else if tok == "-" || tok == "+" || tok == "!" {
			if tok == "-" || tok == "!" {
				sign = -1
			} else {
				sign = 1
				p.plusUsed = true
			}
			p.pos++
		}
		if p.pos >= len(p.tokens) {
			if p.simple && sign > 0 {
				return map[string]any{"match_none": map[string]any{}}, nil
			}
			break
		}
		q, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		if sign < 0 {
			if p.simple && !p.notOp {
				if p.operator == "AND" {
					must = append(must, q)
				} else {
					should = append(should, q)
				}
				forceMust = false
				continue
			}
			mustNot = append(mustNot, q)
			forceMust = false
			continue
		}
		if sign > 0 || forceMust {
			if forceMust && len(should) > 0 {
				must = append(must, should...)
				should = nil
			}
			must = append(must, q)
		} else if p.operator == "AND" {
			must = append(must, q)
		} else {
			should = append(should, q)
		}
		forceMust = false
	}
	if forceMust {
		return nil, errParsing("trailing operator")
	}
	bm := map[string]any{}
	if len(must) > 0 {
		bm["must"] = must
	}
	if len(should) > 0 {
		bm["should"] = should
	}
	if len(mustNot) > 0 {
		bm["must_not"] = mustNot
	}
	if len(bm) == 0 {
		return map[string]any{"match_all": map[string]any{}}, nil
	}
	return map[string]any{"bool": bm}, nil
}
func (p *qsParser) parseUnary() (map[string]any, *apiError) {
	if p.pos < len(p.tokens) {
		tok := p.tokens[p.pos]
		if tok == "NOT" || tok == "!" {
			p.pos++
			inner, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return map[string]any{"bool": map[string]any{"must_not": []any{inner}}}, nil
		}
		if strings.HasPrefix(tok, "!") && len(tok) > 1 {
			p.tokens[p.pos] = tok[1:]
			inner, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return map[string]any{"bool": map[string]any{"must_not": []any{inner}}}, nil
		}
		if tok == "-" || (strings.HasPrefix(tok, "-") && len(tok) > 1) {
			if tok == "-" {
				p.pos++
				inner, err := p.parseUnary()
				if err != nil {
					return nil, err
				}
				return map[string]any{"bool": map[string]any{"must_not": []any{inner}}}, nil
			}
			p.tokens[p.pos] = tok[1:]
			inner, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return map[string]any{"bool": map[string]any{"must_not": []any{inner}}}, nil
		}
	}
	return p.parseTerm()
}

func parsePhraseToken(tok string) (string, float64, bool) {
	if len(tok) == 0 || tok[0] != '"' {
		return "", 0, false
	}
	end := -1
	for i := 1; i < len(tok); i++ {
		if tok[i] == '"' {
			end = i
			break
		}
	}
	if end < 0 {
		return strings.Trim(tok, "\""), 0, false
	}
	phrase := tok[1:end]
	slop := 0.0
	rest := tok[end+1:]
	if strings.HasPrefix(rest, "~") {
		if f, err := strconv.ParseFloat(rest[1:], 64); err == nil {
			slop = f
		}
	}
	return phrase, slop, true
}

func (p *qsParser) parseTerm() (map[string]any, *apiError) {
	if p.pos >= len(p.tokens) {
		return map[string]any{"match_all": map[string]any{}}, nil
	}
	tok := p.tokens[p.pos]
	p.pos++
	if tok == "(" {
		q, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.pos < len(p.tokens) && p.tokens[p.pos] == ")" {
			p.pos++
		}
		if p.pos < len(p.tokens) && strings.HasPrefix(p.tokens[p.pos], "^") {
			if f, err := strconv.ParseFloat(strings.TrimPrefix(p.tokens[p.pos], "^"), 64); err == nil {
				applyQueryBoost(q, f)
			}
			p.pos++
		}
		return q, nil
	}
	if strings.HasPrefix(tok, "_exists_:") {
		field := strings.TrimPrefix(tok, "_exists_:")
		return map[string]any{"exists": map[string]any{"field": field}}, nil
	}
	// field:value
	fieldColon := -1
	if !p.simple {
		fieldColon = strings.Index(tok, ":")
	}
	if i := fieldColon; i > 0 && !strings.HasPrefix(tok, "_") || (i > 0 && strings.HasPrefix(tok, "_") && strings.Contains(tok, ":")) {
		field := tok[:i]
		value := tok[i+1:]
		rawField := field
		if rawField != "*" && strings.ContainsAny(rawField, "*?") && !strings.Contains(rawField, "\\") {
			return nil, errParsing("invalid field wildcard")
		}
		if strings.Contains(value, ":") {
			return nil, errParsing("invalid field syntax")
		}
		field = unescapeQueryString(field)
		if strings.Contains(rawField, "\\*") || strings.Contains(rawField, "\\?") {
			field = p.default_
		} else if _, known := mappingFieldDef(p.c.idx, field); !known && field != "_id" && field != "_index" && field != "_routing" {
			if value == "*" {
				return map[string]any{"match_none": map[string]any{}}, nil
			}
			field = p.default_
		}
		if value == "" && p.pos < len(p.tokens) {
			// field:(...)
			if p.tokens[p.pos] == "(" {
				p.pos++
				q, err := p.parseOr()
				if err != nil {
					return nil, err
				}
				if p.pos < len(p.tokens) && p.tokens[p.pos] == ")" {
					p.pos++
				}
				out := p.prefixField(field, q)
				if p.pos < len(p.tokens) && strings.HasPrefix(p.tokens[p.pos], "^") {
					if f, err := strconv.ParseFloat(strings.TrimPrefix(p.tokens[p.pos], "^"), 64); err == nil {
						applyQueryBoost(out, f)
					}
					p.pos++
				}
				return out, nil
			}
		}
		if strings.HasPrefix(value, "\"") {
			for !strings.Contains(strings.TrimPrefix(value, "\""), "\"") && p.pos < len(p.tokens) {
				value += " " + p.tokens[p.pos]
				p.pos++
			}
			phrase, slop, _ := parsePhraseToken(value)
			if p.simple && !p.slopOp {
				slop = 0
			}
			if slop > 0 {
				return map[string]any{"match_phrase": map[string]any{field: map[string]any{"query": phrase, "slop": slop}}}, nil
			}
			return map[string]any{"match_phrase": map[string]any{field: phrase}}, nil
		}
		return p.valueQuery(field, value)
	}
	if strings.HasPrefix(tok, "\"") {
		phrase, slop, _ := parsePhraseToken(tok)
		if p.simple && !p.slopOp {
			slop = 0
		}
		if p.hasFields {
			if len(p.fields) == 0 {
				return map[string]any{"match_none": map[string]any{}}, nil
			}
			phraseFields := p.fields
			if p.simple && p.quoteSuffix != "" {
				phraseFields = []string{}
				for _, f := range p.fields {
					name, fb := stripFieldBoost(f)
					if fb != 1 {
						phraseFields = append(phraseFields, name+p.quoteSuffix+"^"+strconv.FormatFloat(fb, 'f', -1, 64))
					} else {
						phraseFields = append(phraseFields, name+p.quoteSuffix)
					}
				}
			}
			mq := multiFieldQuery("match_phrase", phraseFields, phrase)
			if slop > 0 {
				applyQueryPhraseSlop(mq, slop)
			}
			return mq, nil
		}
		if slop > 0 {
			return map[string]any{"match_phrase": map[string]any{p.default_: map[string]any{"query": phrase, "slop": slop}}}, nil
		}
		return map[string]any{"match_phrase": map[string]any{p.default_: phrase}}, nil
	}
	if p.simple {
		if i := strings.Index(tok, "\\-"); i > 0 {
			tok = tok[:i]
		}
		tok = strings.TrimSuffix(tok, "!")
	}
	if p.simple {
		if strings.Contains(tok, ":") && !strings.Contains(tok, "\\:") {
			return map[string]any{"match_none": map[string]any{}}, nil
		}
		target := p.fields
		if !p.hasFields {
			target = textFieldNames(p.c)
		}
		if strings.ContainsAny(tok, "*?") {
			if !p.prefixOp {
				return map[string]any{"match_none": map[string]any{}}, nil
			}
			return multiFieldQuery("wildcard", target, strings.ToLower(tok)), nil
		}
		if strings.HasPrefix(tok, "/") && strings.HasSuffix(tok, "/") && len(tok) > 1 {
			return multiFieldQuery("regexp", target, tok[1:len(tok)-1]), nil
		}
		if i := strings.LastIndex(tok, "~"); i > 0 {
			if !p.fuzzyOp {
				return map[string]any{"match_none": map[string]any{}}, nil
			}
			base := tok[:i]
			fuzz := any(2)
			if spec := tok[i+1:]; spec != "" {
				fuzz = lintValue(spec)
			}
			return multiFieldOpts("fuzzy", target, map[string]any{"value": base, "fuzziness": fuzz}), nil
		}
	}
	if p.hasFields {
		if len(p.fields) == 0 {
			return map[string]any{"match_none": map[string]any{}}, nil
		}
		return multiFieldQuery("match", p.fields, tok), nil
	}
	return p.valueQuery(p.default_, tok)
}

func (p *qsParser) prefixField(field string, q map[string]any) map[string]any {
	// rewrite inner default fields to the given field
	return rewriteQSFiels(q, field)
}

func rewriteQSFiels(q map[string]any, field string) map[string]any {
	out := map[string]any{}
	for k, v := range q {
		switch k {
		case "match", "match_phrase", "term", "wildcard", "prefix":
			out[k] = map[string]any{field: v.(map[string]any)[firstKey(v.(map[string]any))]}
		case "bool":
			bm := map[string]any{}
			for bk, bv := range v.(map[string]any) {
				switch bk {
				case "must", "should", "must_not", "filter":
					list := []any{}
					for _, e := range asQueryList(bv) {
						list = append(list, rewriteQSFiels(e.(map[string]any), field))
					}
					bm[bk] = list
				default:
					bm[bk] = bv
				}
			}
			out["bool"] = bm
		default:
			out[k] = v
		}
	}
	return out
}

func firstKey(m map[string]any) string {
	for k := range m {
		return k
	}
	return ""
}

func textFieldNames(c *queryCtx) []string {
	all := []string{}
	for f := range c.valuesMap() {
		def, ok := mappingFieldDef(c.idx, f)
		if !ok {
			continue
		}
		typ, _ := def["type"].(string)
		if typ == "text" || typ == "match_only_text" || typ == "search_as_you_type" ||
			typ == "keyword" || typ == "constant_keyword" || typ == "wildcard" || typ == "flat_object" {
			all = append(all, f)
		}
	}
	sort.Strings(all)
	return all
}

func multiFieldOpts(kind string, fields []string, opts map[string]any) map[string]any {
	shoulds := []any{}
	for _, f := range fields {
		name, _ := stripFieldBoost(f)
		inner := map[string]any{}
		for k, v := range opts {
			inner[k] = v
		}
		shoulds = append(shoulds, map[string]any{kind: map[string]any{name: inner}})
	}
	return map[string]any{"bool": map[string]any{"should": shoulds}}
}

func multiFieldQuery(kind string, fields []string, value string) map[string]any {
	shoulds := []any{}
	for _, f := range fields {
		name, _ := stripFieldBoost(f)
		shoulds = append(shoulds, map[string]any{kind: map[string]any{name: value}})
	}
	return map[string]any{"bool": map[string]any{"should": shoulds}}
}

func unescapeQueryString(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			b.WriteByte(s[i])
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func applyQueryBoost(q map[string]any, boost float64) map[string]any {
	for k, v := range q {
		switch k {
		case "match":
			if bm, ok := v.(map[string]any); ok {
				for f, raw := range bm {
					if opts, ok := raw.(map[string]any); ok {
						opts["boost"] = boost
					} else {
						bm[f] = map[string]any{"query": raw, "boost": boost}
					}
				}
			}
		case "match_phrase":
			if bm, ok := v.(map[string]any); ok {
				for f, raw := range bm {
					if opts, ok := raw.(map[string]any); ok {
						opts["boost"] = boost
					} else {
						bm[f] = map[string]any{"query": raw, "boost": boost}
					}
				}
			}
		case "term", "prefix", "wildcard", "regexp", "fuzzy":
			if bm, ok := v.(map[string]any); ok {
				for f, raw := range bm {
					if opts, ok := raw.(map[string]any); ok {
						opts["boost"] = boost
					} else {
						bm[f] = map[string]any{"value": raw, "boost": boost}
					}
				}
			}
		case "bool":
			if bm, ok := v.(map[string]any); ok {
				for _, key := range []string{"must", "should", "must_not", "filter"} {
					if list, ok := bm[key].([]any); ok {
						for _, e := range list {
							if em, ok := e.(map[string]any); ok {
								applyQueryBoost(em, boost)
							}
						}
					}
				}
			}
		}
	}
	return q
}

func (p *qsParser) valueQuery(field, value string) (map[string]any, *apiError) {
	value = unescapeQueryString(value)
	if i := strings.LastIndex(value, "^"); i > 0 {
		if f, err := strconv.ParseFloat(value[i+1:], 64); err == nil {
			q, qerr := p.valueQuery(field, value[:i])
			if qerr != nil {
				return nil, qerr
			}
			return applyQueryBoost(q, f), nil
		}
	}
	if strings.HasSuffix(value, "^") {
		return nil, errParsing("invalid boost")
	}
	if fieldUnderNested(p.c.idx, field) {
		return map[string]any{"match_none": map[string]any{}}, nil
	}
	if value == "*" {
		return map[string]any{"exists": map[string]any{"field": field}}, nil
	}
	// range syntax field:>=5 / field:[a TO b] / field:{a TO b}
	if strings.HasPrefix(value, ">") || strings.HasPrefix(value, "<") {
		op := "gt"
		if strings.HasPrefix(value, ">=") {
			op = "gte"
			value = value[2:]
		} else if strings.HasPrefix(value, "<=") {
			op = "lte"
			value = value[2:]
		} else if strings.HasPrefix(value, ">") {
			op = "gt"
			value = value[1:]
		} else {
			op = "lt"
			value = value[1:]
		}
		return map[string]any{"range": map[string]any{field: map[string]any{op: lintValue(value)}}}, nil
	}
	if strings.HasPrefix(value, "[") || strings.HasPrefix(value, "{") {
		inclusiveLow := strings.HasPrefix(value, "[")
		body := strings.Trim(value, "[]{}")
		// value may span tokens
		if !strings.Contains(body, "TO") && p.pos < len(p.tokens) {
			for p.pos < len(p.tokens) && !strings.Contains(body, "TO") {
				body += " " + p.tokens[p.pos]
				p.pos++
			}
			body = strings.Trim(body, "[]{}")
		}
		loTO := strings.Index(body, " TO ")
		if loTO < 0 {
			return nil, errParsing("failed to parse range in query string")
		}
		lo := strings.TrimSpace(body[:loTO])
		hi := strings.TrimSpace(body[loTO+4:])
		cond := map[string]any{}
		if lo != "*" && lo != "" {
			if inclusiveLow {
				cond["gte"] = lintValue(lo)
			} else {
				cond["gt"] = lintValue(lo)
			}
		}
		if hi != "*" && hi != "" {
			if strings.HasSuffix(value, "]") {
				cond["lte"] = lintValue(hi)
			} else {
				cond["lt"] = lintValue(hi)
			}
		}
		return map[string]any{"range": map[string]any{field: cond}}, nil
	}
	if len(value) >= 2 && strings.HasPrefix(value, "/") && strings.HasSuffix(value, "/") {
		pattern := value[1 : len(value)-1]
		if _, err := regexp.Compile(pattern); err != nil {
			return map[string]any{"match_none": map[string]any{}}, nil
		}
		return map[string]any{"regexp": map[string]any{field: map[string]any{"value": pattern}}}, nil
	}
	if i := strings.LastIndex(value, "~"); i > 0 {
		term := value[:i]
		spec := value[i+1:]
		fuzziness := any("AUTO")
		if spec != "" {
			fuzziness = lintValue(spec)
		}
		return map[string]any{"fuzzy": map[string]any{field: map[string]any{"value": term, "fuzziness": fuzziness}}}, nil
	}
	if strings.ContainsAny(value, "*?") {
		if !p.allowLeading && (strings.HasPrefix(value, "*") || strings.HasPrefix(value, "?")) {
			return nil, errParsing("leading wildcard disallowed")
		}
		return map[string]any{"wildcard": map[string]any{field: map[string]any{"value": strings.ToLower(value)}}}, nil
	}
	def, ok := mappingFieldDef(p.c.idx, field)
	ftype, _ := def["type"].(string)
	if (ftype == "keyword" || ftype == "constant_keyword") && def != nil {
		if split, _ := def["split_queries_on_whitespace"].(bool); split && len(strings.Fields(value)) > 1 {
			toks := strings.Fields(value)
			clauses := make([]any, 0, len(toks))
			for _, t := range toks {
				clauses = append(clauses, map[string]any{"term": map[string]any{field: lintValue(t)}})
			}
			kind := "should"
			if p.operator == "AND" {
				kind = "must"
			}
			return boolQuery(kind, clauses, 0), nil
		}
	}
	if !ok || ftype == "text" || ftype == "match_only_text" || ftype == "search_as_you_type" || ftype == "" {
		return map[string]any{"match": map[string]any{field: value}}, nil
	}
	if ftype == "date" || ftype == "date_nanos" {
		ctx := &indexCtx{idx: nil, indexCoerce: true}
		if _, err := ctx.parseDateValue("_value", ftype, def, value); err != nil {
			return map[string]any{"match_none": map[string]any{}}, nil
		}
		return map[string]any{"term": map[string]any{field: value}}, nil
	}
	if isNumericType(ftype) {
		if _, isNum := toFloat(lintValue(value)); !isNum {
			return map[string]any{"match_none": map[string]any{}}, nil
		}
	}
	return map[string]any{"term": map[string]any{field: lintValue(value)}}, nil
}

func lintValue(v string) any {
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	return v
}

func boolQuery(kind string, clauses []any, msm int) map[string]any {
	bm := map[string]any{kind: clauses}
	if msm > 0 {
		bm["minimum_should_match"] = msm
	}
	return map[string]any{"bool": bm}
}

func qsParseError(query string) *apiError {
	return &apiError{Type: "search_phase_execution_exception", Status: 400, Reason: "all shards failed",
		RootCause: &apiError{Type: "query_shard_exception", Status: 400, Reason: "Failed to parse query [" + query + "]"},
		CausedBy: &apiError{Type: "query_shard_exception", Status: 400, Reason: "Failed to parse query [" + query + "]",
			CausedBy: &apiError{Type: "parse_exception", Status: 400, Reason: "Failed to parse query [" + query + "]"}}}
}

func applyQueryLenient(q map[string]any) {
	for k, v := range q {
		switch k {
		case "match":
			if bm, ok := v.(map[string]any); ok {
				for f, raw := range bm {
					if opts, ok := raw.(map[string]any); ok {
						opts["lenient"] = true
					} else {
						bm[f] = map[string]any{"query": raw, "lenient": true}
					}
				}
			}
		case "bool":
			if bm, ok := v.(map[string]any); ok {
				for _, key := range []string{"must", "should", "must_not", "filter"} {
					if list, ok := bm[key].([]any); ok {
						for _, e := range list {
							if em, ok := e.(map[string]any); ok {
								applyQueryLenient(em)
							}
						}
					}
				}
			}
		}
	}
}

func applyQueryPhraseSlop(q map[string]any, slop any) {
	for k, v := range q {
		switch k {
		case "match_phrase":
			if bm, ok := v.(map[string]any); ok {
				for f, raw := range bm {
					if opts, ok := raw.(map[string]any); ok {
						opts["slop"] = slop
					} else {
						bm[f] = map[string]any{"query": raw, "slop": slop}
					}
				}
			}
		case "bool":
			if bm, ok := v.(map[string]any); ok {
				for _, key := range []string{"must", "should", "must_not", "filter"} {
					if list, ok := bm[key].([]any); ok {
						for _, e := range list {
							if em, ok := e.(map[string]any); ok {
								applyQueryPhraseSlop(em, slop)
							}
						}
					}
				}
			}
		}
	}
}

func applyQueryFuzzyTranspositions(q map[string]any, transpositions bool) {
	for k, v := range q {
		switch k {
		case "fuzzy":
			if bm, ok := v.(map[string]any); ok {
				for f, raw := range bm {
					if opts, ok := raw.(map[string]any); ok {
						opts["transpositions"] = transpositions
					} else {
						bm[f] = map[string]any{"value": raw, "transpositions": transpositions}
					}
				}
			}
		case "bool":
			if bm, ok := v.(map[string]any); ok {
				for _, key := range []string{"must", "should", "must_not", "filter"} {
					if list, ok := bm[key].([]any); ok {
						for _, e := range list {
							if em, ok := e.(map[string]any); ok {
								applyQueryFuzzyTranspositions(em, transpositions)
							}
						}
					}
				}
			}
		}
	}
}

func (c *queryCtx) evalQueryString(body any, simple bool) (bool, float64, *apiError) {
	bm, ok := body.(map[string]any)
	if !ok {
		return false, 0, errParsing("query_string malformed")
	}
	if simple {
		allowed := map[string]bool{"query": true, "fields": true, "default_operator": true, "flags": true,
			"minimum_should_match": true, "lenient": true, "analyze_wildcard": true, "fuzzy_transpositions": true,
			"quote_field_suffix": true, "default_field": true, "auto_generate_synonyms_phrase_query": true,
			"boost": true, "_name": true, "fuzzy_prefix_length": true, "fuzzy_max_expansions": true,
			"allow_leading_wildcard": true, "phrase_slop": true, "tie_breaker": true, "type": true}
		for k := range bm {
			if !allowed[k] {
				return false, 0, errParsing("[simple_query_string] unsupported field [" + k + "]")
			}
		}
	}
	query, _ := toString(bm["query"])
	if strings.TrimSpace(query) == "" {
		return false, 0, nil
	}
	if query == "" && simple {
		query, _ = toString(bm["query"])
	}
	operator := "OR"
	if o, ok := bm["default_operator"].(string); ok {
		operator = strings.ToUpper(o)
		if operator != "AND" && operator != "OR" {
			return false, 0, &apiError{Type: "illegal_argument_exception", Status: 400,
				Reason: "No enum constant org.opensearch.index.query.Operator." + operator}
		}
	}
	defaultField := ""
	if df, ok := bm["default_field"].(string); ok {
		defaultField = df
	}
	if defaultField == "" {
		defaultField = "*"
	}
	var fields []string
	switch t := bm["fields"].(type) {
	case []any:
		for _, e := range t {
			if s, ok := toString(e); ok {
				fields = append(fields, s)
			}
		}
	case string:
		fields = []string{t}
	}
	if len(fields) > 0 {
		expanded := []string{}
		for _, f := range fields {
			name, _ := stripFieldBoost(f)
			if strings.ContainsAny(name, "*?") {
				re := wildcardRegex(name)
				paths := []string{}
				for path := range c.valuesMap() {
					if !re.MatchString(path) {
						continue
					}
					def, ok := mappingFieldDef(c.idx, path)
					if !ok {
						continue
					}
					typ, _ := def["type"].(string)
					if typ == "text" || typ == "match_only_text" || typ == "search_as_you_type" ||
						typ == "keyword" || typ == "constant_keyword" || typ == "wildcard" || typ == "flat_object" {
						paths = append(paths, path)
					}
				}
				sort.Strings(paths)
				expanded = append(expanded, paths...)
			} else {
				expanded = append(expanded, f)
			}
		}
		fields = expanded
	}
	if !simple && len(fields) == 0 && defaultField != "*" &&
		!strings.ContainsAny(query, ":\"()[]{}^~*?\\+-!&|") && len(strings.Fields(query)) > 1 {
		if def, ok := mappingFieldDef(c.idx, defaultField); ok {
			ftype, _ := def["type"].(string)
			split, _ := def["split_queries_on_whitespace"].(bool)
			if (ftype == "keyword" || ftype == "constant_keyword" || ftype == "wildcard") && !split {
				return c.eval(map[string]any{"term": map[string]any{defaultField: query}})
			}
		}
	}
	if t, _ := bm["type"].(string); t == "phrase" {
		var q map[string]any
		if len(fields) > 0 {
			q = multiFieldQuery("match_phrase", fields, query)
		} else {
			q = map[string]any{"match_phrase": map[string]any{defaultField: query}}
			if defaultField == "*" {
				q = expandMultifield(q, c)
			}
		}
		if v, has := bm["phrase_slop"]; has {
			applyQueryPhraseSlop(q, v)
		}
		return c.eval(q)
	}
	if !simple && !balancedQueryParens(query) {
		return false, 0, qsParseError(query)
	}
	allowLeading := true
	if v, ok := bm["allow_leading_wildcard"].(bool); ok {
		allowLeading = v
	}
	_, hasFields := bm["fields"]
	prefixOp, fuzzyOp, phraseOp, slopOp, notOp, andOp := true, true, true, true, true, true
	quoteSuffix, _ := bm["quote_field_suffix"].(string)
	if fs, ok := bm["flags"].(string); ok {
		allowed := map[string]bool{}
		all := false
		for _, f := range strings.Split(fs, "|") {
			switch f {
			case "ALL":
				all = true
			case "NONE":
			case "AND", "OR", "NOT", "PREFIX", "PHRASE", "PRECEDENCE", "ESCAPE", "WHITESPACE", "FUZZY", "NEAR", "SLOP":
				allowed[f] = true
			default:
				return false, 0, &apiError{Type: "illegal_argument_exception", Status: 400,
					Reason: "Unknown simple_query_string flag [" + f + "]"}
			}
		}
		if !all {
			prefixOp = allowed["PREFIX"]
			fuzzyOp = allowed["FUZZY"]
			phraseOp = allowed["PHRASE"]
			slopOp = allowed["SLOP"]
			notOp = false
			andOp = allowed["AND"]
		}
	}
	p := &qsParser{tokens: tokenizeQueryString(query), default_: defaultField, fields: fields, operator: operator, simple: simple, c: c, allowLeading: allowLeading, hasFields: hasFields,
		prefixOp: prefixOp, fuzzyOp: fuzzyOp, phraseOp: phraseOp, slopOp: slopOp, notOp: notOp, andOp: andOp, quoteSuffix: quoteSuffix}
	q, err := p.parse()
	if err != nil {
		if simple {
			return c.eval(map[string]any{"match_none": map[string]any{}})
		}
		return false, 0, qsParseError(query)
	}
	if qa, ok := bm["quote_analyzer"].(string); ok && qa != "" && !simple {
		setPhraseAnalyzer(q, qa)
	}
	if simple {
		if bq, ok := q["bool"].(map[string]any); ok {
			if p.plusUsed {
				if should, ok := bq["should"].([]any); ok && len(should) > 0 {
					must, _ := bq["must"].([]any)
					must = append(must, should...)
					bq["must"] = must
					delete(bq, "should")
				}
			} else if operator != "AND" {
				if mn, ok := bq["must_not"].([]any); ok {
					should, _ := bq["should"].([]any)
					for _, e := range mn {
						should = append(should, map[string]any{"bool": map[string]any{"must_not": []any{e}}})
					}
					bq["should"] = should
					delete(bq, "must_not")
				}
			}
		}
	}
	if v, has := bm["phrase_slop"]; has {
		applyQueryPhraseSlop(q, v)
	}
	if v, ok := bm["fuzzy_transpositions"].(bool); ok {
		applyQueryFuzzyTranspositions(q, v)
	}
	if v, has := bm["minimum_should_match"]; has {
		if bq, ok := q["bool"].(map[string]any); ok {
			if _, hasShould := bq["should"]; hasShould {
				bq["minimum_should_match"] = v
			}
		}
	}
	if defaultField == "*" && len(fields) == 0 {
		q = expandMultifield(q, c)
	}
	if v, ok := bm["lenient"].(bool); ok && v {
		applyQueryLenient(q)
	}
	return c.eval(q)
}

// expandMultifield rewrites queries against "*" default field into multi_match.
func expandMultifield(q map[string]any, c *queryCtx) map[string]any {
	for k, v := range q {
		switch k {
		case "match", "match_phrase":
			bm, _ := v.(map[string]any)
			field, val := "", any("")
			for f, vv := range bm {
				field = f
				val = vv
			}
			if field == "*" {
				all := []string{}
				for f := range c.valuesMap() {
					def, ok := mappingFieldDef(c.idx, f)
					if !ok {
						continue
					}
					typ, _ := def["type"].(string)
					if typ == "text" || typ == "match_only_text" || typ == "search_as_you_type" ||
						typ == "keyword" || typ == "constant_keyword" || typ == "wildcard" || typ == "flat_object" {
						all = append(all, f)
					}
				}
				sort.Strings(all)
				kind := "match"
				if k == "match_phrase" {
					kind = "match_phrase"
				}
				shoulds := []any{}
				for _, f := range all {
					shoulds = append(shoulds, map[string]any{kind: map[string]any{f: val}})
				}
				return map[string]any{"bool": map[string]any{"should": shoulds}}
			}
		case "bool":
			bm, _ := v.(map[string]any)
			for _, key := range []string{"must", "should", "must_not", "filter"} {
				if list, ok := bm[key].([]any); ok {
					out := []any{}
					for _, e := range list {
						if em, ok := e.(map[string]any); ok {
							out = append(out, expandMultifield(em, c))
						} else {
							out = append(out, e)
						}
					}
					bm[key] = out
				}
			}
		}
	}
	return q
}

func fieldUnderNested(idx *Index, field string) bool {
	parts := strings.Split(field, ".")
	props := ensureProperties(idx.Mappings)
	for i := 0; i < len(parts)-1; i++ {
		def, ok := props[parts[i]].(map[string]any)
		if !ok {
			return false
		}
		if t, _ := def["type"].(string); t == "nested" {
			return true
		}
		next, ok := def["properties"].(map[string]any)
		if !ok {
			return false
		}
		props = next
	}
	return false
}

func balancedQueryParens(s string) bool {
	depth := 0
	inQuote := false
	for _, ch := range s {
		if ch == '"' {
			inQuote = !inQuote
			continue
		}
		if inQuote {
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

// fieldTermStats holds the per-field corpus statistics BM25 needs (document
// count, total length, document frequencies) plus each document's analysed
// tokens, so scoring a hit does not re-analyse the whole index.
type fieldTermStats struct {
	gen       uint64
	mapVer    int64
	settings  int
	n         int
	sumdl     float64
	df        map[string]int
	docTokens map[string][]string
}

type termStatsKey struct {
	idx             *Index
	field, analyzer string
}

var (
	termStatsMu    sync.Mutex
	termStatsCache = map[termStatsKey]*fieldTermStats{}
)

func termStatsFor(idx *Index, field, analyzer string, docTerms func([]any) []string) *fieldTermStats {
	key := termStatsKey{idx, field, analyzer}
	termStatsMu.Lock()
	st := termStatsCache[key]
	termStatsMu.Unlock()
	if st != nil && st.gen == idx.dvGen && st.mapVer == idx.MappingVersion && st.settings == len(idx.Settings) {
		return st
	}
	st = &fieldTermStats{gen: idx.dvGen, mapVer: idx.MappingVersion, settings: len(idx.Settings),
		df: map[string]int{}, docTokens: map[string][]string{}}
	for id, dv := range idx.DocValues {
		vals := dv[field]
		if len(vals) == 0 {
			continue
		}
		st.n++
		toks := docTerms(vals)
		st.docTokens[id] = toks
		st.sumdl += float64(len(toks))
		seen := map[string]bool{}
		for _, t := range toks {
			if !seen[t] {
				seen[t] = true
				st.df[t]++
			}
		}
	}
	termStatsMu.Lock()
	if len(termStatsCache) > 256 {
		termStatsCache = map[termStatsKey]*fieldTermStats{}
	}
	termStatsCache[key] = st
	termStatsMu.Unlock()
	return st
}
