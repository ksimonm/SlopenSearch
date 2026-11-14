package main

import (
	"math"
	"sort"
	"strings"
)

func osaDistance(a, b []rune) int {
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := 0; j <= len(b); j++ {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+cost)
			}
		}
	}
	return d[len(a)][len(b)]
}

func suggestShardError(idx *Index, reason string) *apiError {
	leaf := orderedJSON{{"type", "illegal_argument_exception"}, {"reason", reason}}
	top := orderedJSON{{"type", "illegal_argument_exception"}, {"reason", reason}, {"caused_by", leaf}}
	name := ""
	if idx != nil {
		name = idx.Name
	}
	f := &shardFailure{status: 400, index: name, root: leaf, shard: leaf, top: top}
	return &apiError{Type: "search_phase_execution_exception", Status: 400, Reason: "all shards failed", Shard: f}
}

func suggestIllegal(reason string) *apiError {
	return &apiError{Type: "illegal_argument_exception", Status: 400, Reason: reason}
}

var termSuggestOptions = map[string]bool{
	"field": true, "analyzer": true, "size": true, "shard_size": true, "suggest_mode": true,
	"accuracy": true, "sort": true, "string_distance": true, "max_edits": true,
	"max_inspections": true, "max_term_freq": true, "prefix_length": true,
	"min_word_length": true, "min_doc_freq": true, "lowercase_terms": true, "exact_match": true,
}

type termCand struct {
	text  string
	score float64
	freq  int
}

func termSuggester(spec, body map[string]any, indices []*Index, globalText any) ([]any, *apiError) {
	for k := range body {
		if !termSuggestOptions[k] {
			return nil, &apiError{Type: "parsing_exception", Status: 400, Reason: "suggester[term] doesn't support field [" + k + "]"}
		}
	}
	field, ok := toString(body["field"])
	if !ok || field == "" {
		return nil, &apiError{Type: "parse_exception", Status: 400, Reason: "the required field option [field] is missing"}
	}
	num := func(k string, def float64) float64 {
		if n, ok := toFloat(body[k]); ok {
			return n
		}
		return def
	}
	size := int(num("size", 5))
	if size <= 0 {
		return nil, suggestIllegal("size must be positive")
	}
	shardSize := max(size, 5)
	if _, ok := body["shard_size"]; ok {
		shardSize = int(num("shard_size", 5))
		if shardSize <= 0 {
			return nil, suggestIllegal("shardSize must be positive")
		}
	}
	mode := "missing"
	if s, ok := toString(body["suggest_mode"]); ok {
		mode = strings.ToLower(s)
		if mode != "missing" && mode != "popular" && mode != "always" {
			return nil, suggestIllegal("No enum constant org.opensearch.search.suggest.term.TermSuggestionBuilder.SuggestMode." + strings.ToUpper(s))
		}
	}
	sortBy := "score"
	if s, ok := toString(body["sort"]); ok {
		sortBy = strings.ToLower(s)
		if sortBy != "score" && sortBy != "frequency" {
			return nil, suggestIllegal("No enum constant org.opensearch.search.suggest.SortBy." + strings.ToUpper(s))
		}
	}
	distance := "internal"
	if s, ok := toString(body["string_distance"]); ok {
		distance = strings.ToLower(s)
		switch distance {
		case "internal", "damerau_levenshtein", "levenshtein", "jaro_winkler", "ngram":
		default:
			return nil, suggestIllegal("Illegal distance option " + s)
		}
	}
	maxEdits := int(num("max_edits", 2))
	if maxEdits < 1 || maxEdits > 2 {
		return nil, suggestIllegal("maxEdits must be between 1 and 2")
	}
	accuracy := num("accuracy", 0.5)
	maxTermFreq := num("max_term_freq", 0.01)
	prefixLen := int(num("prefix_length", 1))
	minWordLen := int(num("min_word_length", 4))
	minDocFreq := num("min_doc_freq", 0)

	var idx *Index
	if len(indices) > 0 {
		idx = indices[0]
	}
	rawText, hasText := spec["text"]
	if !hasText && globalText != nil {
		rawText, hasText = globalText, true
	}
	if !hasText {
		return nil, suggestShardError(idx, "The required text option is missing")
	}
	input, _ := toString(rawText)
	if f, ok := rawText.(float64); ok {
		input = formatNumber(f)
	}

	analyzer, _ := toString(body["analyzer"])
	found := false
	for _, ix := range indices {
		if _, ok := mappingFieldDef(ix, field); ok {
			found = true
		}
	}
	if !found {
		return nil, suggestShardError(idx, "no mapping found for field ["+field+"]")
	}
	if analyzer != "" {
		if _, ok := globalAnalyzerTokens(analyzer, ""); !ok && prebuiltChain(analyzer) == nil &&
			(idx == nil || !(analysisScope{idx: idx}).indexHasAnalyzer(analyzer)) {
			return nil, suggestShardError(idx, "analyzer ["+analyzer+"] doesn't exists")
		}
	}

	df := map[string]int{}
	maxDoc := 0
	for _, ix := range indices {
		fa := fieldAnalyzer(ix, field, "")
		for _, id := range ix.Order {
			doc, live := ix.Docs[id]
			if !live {
				continue
			}
			maxDoc++
			v, ok := fieldValue(doc, field)
			if !ok {
				continue
			}
			seen := map[string]bool{}
			for _, s := range docStrings(v) {
				for _, tok := range analyzeWithIndex(ix, fa, s) {
					if !seen[tok] {
						seen[tok] = true
						df[tok]++
					}
				}
			}
		}
	}
	terms := make([]string, 0, len(df))
	for t := range df {
		terms = append(terms, t)
	}
	sort.Strings(terms)

	if analyzer == "" {
		analyzer = fieldAnalyzer(idx, field, "")
	}
	toks := analyzeTokensWithIndex(idx, analyzer, input)
	entries := []any{}
	for _, tok := range toks {
		options := termCandidates(tok.term, terms, df, maxDoc, mode, distance, maxEdits, accuracy,
			maxTermFreq, prefixLen, minWordLen, minDocFreq, shardSize)
		sort.SliceStable(options, func(i, j int) bool {
			a, b := options[i], options[j]
			if sortBy == "frequency" {
				if a.freq != b.freq {
					return a.freq > b.freq
				}
				if a.score != b.score {
					return a.score > b.score
				}
				return a.text < b.text
			}
			if a.score != b.score {
				return a.score > b.score
			}
			if a.freq != b.freq {
				return a.freq > b.freq
			}
			return a.text < b.text
		})
		if len(options) > size {
			options = options[:size]
		}
		opts := make([]any, 0, len(options))
		for _, c := range options {
			opts = append(opts, orderedJSON{{"text", c.text}, {"score", plJSONFloat32(float32(c.score))}, {"freq", c.freq}})
		}
		entries = append(entries, orderedJSON{{"text", tok.term}, {"offset", tok.start}, {"length", tok.end - tok.start}, {"options", opts}})
	}
	return entries, nil
}

func termCandidates(term string, terms []string, df map[string]int, maxDoc int, mode, distance string,
	maxEdits int, accuracy, maxTermFreq float64, prefixLen, minWordLen int, minDocFreq float64, numSug int) []termCand {
	tr := []rune(term)
	if len(tr) < minWordLen {
		return nil
	}
	docfreq := df[term]
	if mode == "missing" && docfreq > 0 {
		return nil
	}
	if maxTermFreq >= 1 {
		if float64(docfreq) > maxTermFreq {
			return nil
		}
	} else if docfreq > int(math.Ceil(maxTermFreq*float64(maxDoc))) {
		return nil
	}
	if mode != "popular" {
		docfreq = 0
	}
	if minDocFreq >= 1 {
		docfreq = max(docfreq, int(minDocFreq)-1)
	} else if minDocFreq > 0 {
		docfreq = max(docfreq, int(minDocFreq*float64(maxDoc))-1)
	}
	var out []termCand
	for _, c := range terms {
		if c == term {
			continue
		}
		cr := []rune(c)
		ed := osaDistance(tr, cr)
		if ed > maxEdits {
			continue
		}
		// DirectSpellChecker searches edit distance 1 with the given prefix
		// length, then larger distances with a prefix of at least editDistance-1.
		pl := prefixLen
		if ed > 1 {
			pl = max(pl, ed-1)
		}
		if pl > 0 {
			if len(cr) < pl || len(tr) < pl || string(cr[:pl]) != string(tr[:pl]) {
				continue
			}
		}
		f := df[c]
		if f <= docfreq {
			continue
		}
		var score float64
		switch distance {
		case "levenshtein":
			score = float64(float32(1) - float32(levenshtein(term, c))/float32(max(len(tr), len(cr))))
		case "jaro_winkler":
			score = jaroWinkler(tr, cr)
		case "ngram":
			score = ngramDistance(tr, cr, 2)
		default:
			score = float64(float32(1) - float32(ed)/float32(min(len(tr), len(cr))))
		}
		if score < accuracy {
			continue
		}
		out = append(out, termCand{text: c, score: score, freq: f})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		if out[i].freq != out[j].freq {
			return out[i].freq > out[j].freq
		}
		return out[i].text < out[j].text
	})
	if len(out) > numSug {
		out = out[:numSug]
	}
	return out
}

// jaroWinkler mirrors Lucene's JaroWinklerDistance.
func jaroWinkler(s1, s2 []rune) float64 {
	max1, min1 := s1, s2
	if len(s1) <= len(s2) {
		max1, min1 = s2, s1
	}
	rng := max(len(max1)/2-1, 0)
	matchIndexes := make([]int, len(min1))
	for i := range matchIndexes {
		matchIndexes[i] = -1
	}
	matchFlags := make([]bool, len(max1))
	matches := 0
	for mi := 0; mi < len(min1); mi++ {
		c1 := min1[mi]
		for xi, xn := max(mi-rng, 0), min(mi+rng+1, len(max1)); xi < xn; xi++ {
			if !matchFlags[xi] && c1 == max1[xi] {
				matchIndexes[mi] = xi
				matchFlags[xi] = true
				matches++
				break
			}
		}
	}
	ms1 := make([]rune, 0, matches)
	ms2 := make([]rune, 0, matches)
	for i := 0; i < len(min1); i++ {
		if matchIndexes[i] != -1 {
			ms1 = append(ms1, min1[i])
		}
	}
	for i := 0; i < len(max1); i++ {
		if matchFlags[i] {
			ms2 = append(ms2, max1[i])
		}
	}
	transpositions := 0
	for i := range ms1 {
		if ms1[i] != ms2[i] {
			transpositions++
		}
	}
	prefix := 0
	for mi := 0; mi < len(min1); mi++ {
		if s1[mi] == s2[mi] {
			prefix++
		} else {
			break
		}
	}
	if matches == 0 {
		return 0
	}
	m := float32(matches)
	j := (m/float32(len(s1)) + m/float32(len(s2)) + (m-float32(transpositions/2))/m) / 3
	if j < 0.7 {
		return float64(j)
	}
	return float64(j + min(float32(0.1), 1/float32(len(max1)))*float32(prefix)*(1-j))
}

// ngramDistance mirrors Lucene's NGramDistance.
func ngramDistance(s, t []rune, n int) float64 {
	sl, tl := len(s), len(t)
	if sl == 0 || tl == 0 {
		if sl == tl {
			return 1
		}
		return 0
	}
	cost := 0
	if sl < n || tl < n {
		ni := min(sl, tl)
		for i := 0; i < ni; i++ {
			if s[i] == t[i] {
				cost++
			}
		}
		return float64(float32(cost) / float32(max(sl, tl)))
	}
	sa := make([]rune, sl+n-1)
	for i := range sa {
		if i >= n-1 {
			sa[i] = s[i-n+1]
		}
	}
	p := make([]float32, sl+1)
	d := make([]float32, sl+1)
	tj := make([]rune, n)
	for i := 0; i <= sl; i++ {
		p[i] = float32(i)
	}
	for j := 1; j <= tl; j++ {
		if j < n {
			for ti := 0; ti < n-j; ti++ {
				tj[ti] = 0
			}
			for ti := n - j; ti < n; ti++ {
				tj[ti] = t[ti-(n-j)]
			}
		} else {
			copy(tj, t[j-n:j])
		}
		d[0] = float32(j)
		for i := 1; i <= sl; i++ {
			cost = 0
			tn := n
			for ni := 0; ni < n; ni++ {
				if sa[i-1+ni] != tj[ni] {
					cost++
				} else if sa[i-1+ni] == 0 {
					tn--
				}
			}
			ec := float32(cost) / float32(tn)
			d[i] = min(d[i-1]+1, p[i]+1, p[i-1]+ec)
		}
		p, d = d, p
	}
	return float64(1 - p[sl]/float32(max(tl, sl)))
}
