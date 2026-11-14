package main

import (
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// The phrase suggester, ported from OpenSearch's PhraseSuggester: a noisy
// channel spell checker (NoisyChannelSpellChecker) that draws per-token
// candidates with Lucene's DirectSpellChecker (DirectCandidateGenerator),
// scores candidate paths with an n-gram language model (WordScorer:
// stupid backoff, laplace or linear interpolation) and keeps the best
// corrections (CandidateScorer). Arithmetic follows the Java code, including
// its float/double conversions, so scores match the reference exactly.

// ---------------------------------------------------------------- term statistics

type phTermStats struct {
	df  int64
	ttf int64
}

// phTerms is the terms dictionary of one field in one index.
type phTerms struct {
	stats  map[string]phTermStats
	sorted []string
	sumTTF int64
	maxDoc int64
}

func (t *phTerms) get(term string) phTermStats { return t.stats[term] }

// phraseIndexAnalyzer is the analyzer a field is indexed with.
func phraseIndexAnalyzer(idx *Index, def map[string]any) string {
	if a, ok := def["analyzer"].(string); ok && a != "" {
		return a
	}
	return ""
}

// phraseSearchAnalyzer is the search analyzer of a field (TextSearchInfo).
func phraseSearchAnalyzer(idx *Index, def map[string]any) string {
	if t, _ := def["type"].(string); t == "keyword" {
		return "keyword"
	}
	if a, ok := def["search_analyzer"].(string); ok && a != "" {
		return a
	}
	if a, ok := def["analyzer"].(string); ok && a != "" {
		return a
	}
	sc := analysisScope{idx: idx}
	if sc.indexHasAnalyzer("default_search") {
		return "default_search"
	}
	if sc.indexHasAnalyzer("default") {
		return "default"
	}
	return "standard"
}

// phraseFieldTerms collects the inverted index statistics of a field, or nil
// when the field has no terms (unmapped, not indexed, or no values).
func phraseFieldTerms(idx *Index, field string) *phTerms {
	def, ok := mappingFieldDef(idx, field)
	if !ok {
		return nil
	}
	typ, _ := def["type"].(string)
	if typ == "" {
		typ = "text"
	}
	switch typ {
	case "text", "keyword", "match_only_text", "search_as_you_type":
	default:
		return nil
	}
	if b, ok := def["index"].(bool); ok && !b {
		return nil
	}
	analyzer := phraseIndexAnalyzer(idx, def)
	ignoreAbove := -1
	if n, ok := toFloat(def["ignore_above"]); ok {
		ignoreAbove = int(n)
	}
	t := &phTerms{stats: map[string]phTermStats{}}
	for _, id := range idx.Order {
		doc, live := idx.Docs[id]
		if !live {
			continue
		}
		if _, pending := idx.Pending[id]; pending {
			continue
		}
		t.maxDoc++
		v, ok := fieldValue(doc, field)
		if !ok {
			continue
		}
		seen := map[string]bool{}
		for _, s := range docStrings(v) {
			var toks []string
			if typ == "keyword" {
				if ignoreAbove >= 0 && len([]rune(s)) > ignoreAbove {
					continue
				}
				toks = []string{s}
			} else {
				toks = analyzeWithIndex(idx, analyzer, s)
			}
			for _, tok := range toks {
				st := t.stats[tok]
				st.ttf++
				if !seen[tok] {
					seen[tok] = true
					st.df++
				}
				t.stats[tok] = st
				t.sumTTF++
			}
		}
	}
	if len(t.stats) == 0 {
		return nil
	}
	t.sorted = make([]string, 0, len(t.stats))
	for k := range t.stats {
		t.sorted = append(t.sorted, k)
	}
	sort.Strings(t.sorted)
	return t
}

// ---------------------------------------------------------------- candidates

type phCand struct {
	term  string
	stats phTermStats
	dist  float64 // stringDistance (channel score)
	score float64
	user  bool
}

// phCandLess orders candidates like Collections.reverseOrder() of
// Candidate.compareTo: score descending, then term ascending.
func phCandBefore(a, b *phCand) bool {
	if a.score != b.score {
		return a.score > b.score
	}
	return a.term < b.term
}

type phSet struct {
	cands []*phCand
	orig  *phCand
}

func (s *phSet) addCandidates(list []*phCand) {
	seen := map[string]bool{}
	merged := []*phCand{}
	for _, c := range list {
		if !seen[c.term] {
			seen[c.term] = true
			merged = append(merged, c)
		}
	}
	for _, c := range s.cands {
		if !seen[c.term] {
			seen[c.term] = true
			merged = append(merged, c)
		}
	}
	sort.SliceStable(merged, func(i, j int) bool { return phCandBefore(merged[i], merged[j]) })
	s.cands = merged
}

// ---------------------------------------------------------------- direct generator

type phGenSpec struct {
	field          string
	size           int
	preFilter      string
	postFilter     string
	mode           string
	accuracy       float32
	sortBy         string
	distance       string
	maxEdits       int
	maxInspections int
	maxTermFreq    float32
	prefixLength   int
	minWordLength  int
	minDocFreq     float32
}

func defaultPhGenSpec(field string) *phGenSpec {
	return &phGenSpec{field: field, size: 5, mode: "missing", accuracy: 0.5, sortBy: "score",
		distance: "internal", maxEdits: 2, maxInspections: 5, maxTermFreq: 0.01, prefixLength: 1,
		minWordLength: 4, minDocFreq: 0}
}

type phGen struct {
	spec               *phGenSpec
	idx                *Index
	terms              *phTerms
	sumTTF             int64
	nonErrorLikelihood float64
	plateau            int64
	spare              string // BytesRefBuilder shared by pre and post filters
}

func newPhGen(spec *phGenSpec, idx *Index, terms *phTerms, nonErrorLikelihood float64) *phGen {
	g := &phGen{spec: spec, idx: idx, terms: terms, sumTTF: terms.sumTTF, nonErrorLikelihood: nonErrorLikelihood}
	tf := spec.minDocFreq
	if tf >= 1 {
		g.plateau = int64(int32(tf))
	} else {
		g.plateau = int64(int32(float32(terms.maxDoc) * tf))
	}
	return g
}

func (g *phGen) candScore(st phTermStats, errorScore float64) float64 {
	return errorScore * ((float64(st.ttf) + 1) / (float64(g.sumTTF) + 1))
}

func (g *phGen) preFilter(term string) string {
	if g.spec.preFilter == "" {
		return term
	}
	for _, tok := range analyzeTokensWithIndex(g.idx, g.spec.preFilter, term) {
		g.spare = tok.term
	}
	return g.spare
}

func (g *phGen) termStats(term string) phTermStats {
	return g.terms.get(g.preFilter(term))
}

func (g *phGen) createCandidate(term string, st phTermStats, channel float64, user bool) *phCand {
	return &phCand{term: term, stats: st, dist: channel, score: g.candScore(st, channel), user: user}
}

// javaRound mirrors Math.round(double).
func javaRound(x float64) int64 {
	if math.IsNaN(x) {
		return 0
	}
	r := math.Floor(x + 0.5)
	if r >= math.MaxInt64 {
		return math.MaxInt64
	}
	if r <= math.MinInt64 {
		return math.MinInt64
	}
	return int64(r)
}

func (g *phGen) thresholdTermFrequency(docFreq int64) int64 {
	if docFreq > 0 {
		v := javaRound(float64(docFreq)*(phLog10(float64(docFreq-g.plateau))*(1.0/phLog10(5))) + 1)
		return min(max(0, v), math.MaxInt32)
	}
	return 0
}

func (g *phGen) drawCandidates(set *phSet) {
	orig := set.orig
	term := g.preFilter(orig.term)
	threshold := g.spec.minDocFreq
	if g.spec.mode != "always" {
		t := g.thresholdTermFrequency(orig.stats.df)
		if t == math.MaxInt32 {
			return
		}
		if threshold < float32(t) {
			threshold = float32(t)
		}
	}
	words := g.suggestSimilar(term, g.spec.size, threshold)
	cands := []*phCand{}
	for _, w := range words {
		st := g.terms.get(w.term)
		c := &phCand{term: w.term, stats: st, dist: float64(w.score), score: g.candScore(st, float64(w.score))}
		cands = g.postFilter(c, cands)
	}
	set.addCandidates(cands)
}

func (g *phGen) postFilter(c *phCand, out []*phCand) []*phCand {
	if g.spec.postFilter == "" {
		return append(out, c)
	}
	prev := -1
	for _, tok := range analyzeTokensWithIndex(g.idx, g.spec.postFilter, c.term) {
		posInc := tok.pos - prev
		prev = tok.pos
		g.spare = tok.term
		if posInc > 0 && tok.term == c.term {
			st := g.terms.get(tok.term)
			out = append(out, &phCand{term: tok.term, stats: st, dist: c.dist, score: g.candScore(c.stats, c.dist)})
		} else {
			out = append(out, &phCand{term: tok.term, stats: c.stats, dist: g.nonErrorLikelihood, score: g.candScore(c.stats, c.dist)})
		}
	}
	return out
}

// ---------------------------------------------------------------- DirectSpellChecker

type phWord struct {
	term  string
	score float32
	freq  int64
}

type phScoreTerm struct {
	term  string
	boost float32
	df    int64
	score float32
}

// phScoreTermLess is ScoreTerm.compareTo < 0.
func phScoreTermLess(a, b *phScoreTerm) bool {
	if a.term == b.term {
		return false
	}
	if a.boost == b.boost {
		return b.term < a.term
	}
	return a.boost < b.boost
}

func (g *phGen) suggestSimilar(term string, numSug int, threshold float32) []phWord {
	sp := g.spec
	textLen := len([]rune(term))
	if textLen < sp.minWordLength {
		return nil
	}
	docfreq := g.terms.get(term).df
	if sp.mode == "missing" && docfreq > 0 {
		return nil
	}
	maxDoc := g.terms.maxDoc
	if sp.maxTermFreq >= 1 && float32(docfreq) > sp.maxTermFreq {
		return nil
	} else if docfreq > int64(int32(math.Ceil(float64(sp.maxTermFreq*float32(maxDoc))))) {
		return nil
	}
	if sp.mode != "popular" {
		docfreq = 0
	}
	if threshold >= 1 {
		docfreq = max(docfreq, int64(int32(threshold)))
	} else if threshold > 0 {
		docfreq = max(docfreq, int64(int32(threshold*float32(maxDoc)))-1)
	}
	inspections := numSug * sp.maxInspections
	terms := g.fuzzy(term, inspections, docfreq, 1)
	if sp.maxEdits > 1 && len(terms) < inspections {
		seen := map[string]bool{}
		for _, t := range terms {
			seen[t.term] = true
		}
		for _, t := range g.fuzzy(term, inspections, docfreq, sp.maxEdits) {
			if !seen[t.term] {
				seen[t.term] = true
				terms = append(terms, t)
			}
		}
	}
	words := make([]phWord, 0, len(terms))
	for _, t := range terms {
		words = append(words, phWord{term: t.term, score: t.score, freq: t.df})
	}
	byFreq := sp.sortBy == "frequency"
	sort.SliceStable(words, func(i, j int) bool {
		a, b := words[i], words[j]
		if byFreq {
			if a.freq != b.freq {
				return a.freq > b.freq
			}
			if a.score != b.score {
				return a.score > b.score
			}
			return a.term < b.term
		}
		if a.score != b.score {
			return a.score > b.score
		}
		if a.freq != b.freq {
			return a.freq > b.freq
		}
		return a.term < b.term
	})
	if numSug < len(words) {
		words = words[:numSug]
	}
	return words
}

// fuzzy enumerates the terms within editDistance of term (FuzzyTermsEnum with
// transpositions) and keeps the numSug most similar ones.
func (g *phGen) fuzzy(term string, numSug int, docfreq int64, editDistance int) []*phScoreTerm {
	sp := g.spec
	prefix := max(sp.prefixLength, editDistance-1)
	tr := []rune(term)
	pre := tr
	if len(pre) > prefix {
		pre = pre[:prefix]
	}
	var queue []*phScoreTerm
	peek := func() *phScoreTerm {
		m := queue[0]
		for _, q := range queue[1:] {
			if phScoreTermLess(q, m) {
				m = q
			}
		}
		return m
	}
	for _, cand := range g.terms.sorted {
		cr := []rune(cand)
		if len(cr) < len(pre) || string(cr[:len(pre)]) != string(pre) {
			continue
		}
		ed := osaDistance(tr[len(pre):], cr[len(pre):])
		if ed > editDistance {
			continue
		}
		var boost float32 = 1
		if ed > 0 {
			boost = 1 - float32(ed)/float32(min(len(cr), len(tr)))
		}
		if len(queue) >= numSug && boost <= peek().boost {
			continue
		}
		if cand == term {
			continue
		}
		df := g.terms.get(cand).df
		if df <= docfreq {
			continue
		}
		score := boost
		if sp.distance != "internal" {
			score = phStringDistance(sp.distance, term, cand)
		}
		if score < sp.accuracy {
			continue
		}
		queue = append(queue, &phScoreTerm{term: cand, boost: score, df: df, score: score})
		if len(queue) > numSug {
			m := peek()
			for i, q := range queue {
				if q == m {
					queue = append(queue[:i], queue[i+1:]...)
					break
				}
			}
		}
	}
	return queue
}

func phStringDistance(distance, a, b string) float32 {
	ar, br := []rune(a), []rune(b)
	switch distance {
	case "levenshtein":
		return float32(1) - float32(levenshtein(a, b))/float32(max(len(ar), len(br)))
	case "damerau_levenshtein":
		if len(ar) == 0 || len(br) == 0 {
			if len(ar) == len(br) {
				return 1
			}
			return 0
		}
		return float32(1) - float32(osaDistance(ar, br))/float32(max(len(ar), len(br)))
	case "jaro_winkler":
		return float32(jaroWinkler(ar, br))
	case "ngram":
		return float32(ngramDistance(ar, br, 2))
	}
	return 0
}

// ---------------------------------------------------------------- multi generator

type phMultiGen struct {
	gens          []*phGen
	numCandidates int
}

func (m *phMultiGen) termStats(term string) phTermStats { return m.gens[0].termStats(term) }

func (m *phMultiGen) createCandidate(term string, st phTermStats, channel float64, user bool) *phCand {
	return m.gens[0].createCandidate(term, st, channel, user)
}

func (m *phMultiGen) drawCandidates(set *phSet) {
	for _, g := range m.gens {
		g.drawCandidates(set)
	}
	if len(set.cands) > m.numCandidates {
		c := append([]*phCand(nil), set.cands...)
		sort.SliceStable(c, func(i, j int) bool { return c[i].score > c[j].score })
		set.cands = c[:m.numCandidates]
	}
}

// ---------------------------------------------------------------- word scorers

type phScorer struct {
	terms        *phTerms
	vocabulary   int64
	numTerms     int64
	realWord     float64
	separator    string
	model        string // stupid_backoff, laplace, linear
	discount     float64
	alpha        float64
	triLambda    float64
	biLambda     float64
	uniLambda    float64
	frequencyMap map[string]int64
}

func newPhScorer(terms *phTerms, realWord float64, separator string, sm *phSmoothing) *phScorer {
	s := &phScorer{terms: terms, vocabulary: terms.sumTTF, numTerms: int64(len(terms.stats)),
		realWord: realWord, separator: separator}
	if sm == nil {
		s.model = "stupid_backoff"
		s.discount = float64(float32(0.4))
		return s
	}
	s.model = sm.model
	switch sm.model {
	case "stupid_backoff":
		s.discount = sm.discount
	case "laplace":
		s.alpha = sm.alpha
	case "linear":
		sum := sm.uni + sm.bi + sm.tri
		s.uniLambda = sm.uni / sum
		s.biLambda = sm.bi / sum
		s.triLambda = sm.tri / sum
	}
	return s
}

func (s *phScorer) frequency(term string) int64 { return s.terms.get(term).ttf }

func (s *phScorer) join(parts ...string) string { return strings.Join(parts, s.separator) }

func (s *phScorer) channelScore(c *phCand) float64 {
	if c.dist == 1.0 {
		return s.realWord
	}
	return c.dist
}

func (s *phScorer) score(path []*phCand, sets []*phSet, at, gramSize int) float64 {
	if at == 0 || gramSize == 1 {
		return phLog10(s.channelScore(path[at]) * s.unigram(path[at]))
	} else if at == 1 || gramSize == 2 {
		return phLog10(s.channelScore(path[at]) * s.bigram(path[at], path[at-1]))
	}
	return phLog10(s.channelScore(path[at]) * s.trigram(path[at], path[at-1], path[at-2]))
}

func (s *phScorer) unigram(w *phCand) float64 {
	if s.model == "laplace" {
		return (s.alpha + float64(s.frequency(w.term))) / (float64(s.vocabulary) + s.alpha*float64(s.numTerms))
	}
	return (1.0 + float64(s.frequency(w.term))) / float64(s.vocabulary+s.numTerms)
}

func (s *phScorer) bigram(w, w1 *phCand) float64 {
	switch s.model {
	case "laplace":
		return (s.alpha + float64(s.frequency(s.join(w1.term, w.term)))) / (float64(w1.stats.ttf) + s.alpha*float64(s.numTerms))
	case "linear":
		count := s.frequency(s.join(w1.term, w.term))
		if count < 1 {
			return s.uniLambda * s.unigram(w)
		}
		return s.biLambda*(float64(count)/(0.5+float64(w1.stats.ttf))) + s.uniLambda*s.unigram(w)
	}
	count := s.frequency(s.join(w1.term, w.term))
	if count < 1 {
		return s.discount * s.unigram(w)
	}
	return float64(count) / (float64(w1.stats.ttf) + 0.00000000001)
}

func (s *phScorer) trigram(w, w1, w2 *phCand) float64 {
	switch s.model {
	case "laplace":
		tri := s.frequency(s.join(w2.term, w1.term, w.term))
		bi := s.frequency(s.join(w1.term, w.term))
		return (s.alpha + float64(tri)) / (float64(bi) + s.alpha*float64(s.numTerms))
	case "linear":
		// LinearInterpolatingScorer joins the words in reverse order.
		count := s.frequency(s.join(w.term, w1.term, w2.term))
		if count < 1 {
			return s.bigram(w, w1)
		}
		return s.triLambda*(float64(count)/(1.0+float64(s.frequency(s.join(w.term, w1.term))))) + s.bigram(w, w1)
	}
	bi := s.frequency(s.join(w1.term, w.term))
	if bi < 1 {
		return s.discount * s.unigram(w)
	}
	tri := s.frequency(s.join(w2.term, w1.term, w.term))
	if tri < 1 {
		return s.discount * (float64(bi) / (float64(w1.stats.ttf) + 0.00000000001))
	}
	return float64(tri) / (float64(bi) + 0.00000000001)
}

// ---------------------------------------------------------------- corrections

type phCorrection struct {
	score float64
	cands []*phCand
}

func (c *phCorrection) compareTo(otherScore float64, other []*phCand) int {
	if c.score == otherScore {
		limit := min(len(c.cands), len(other))
		for i := 0; i < limit; i++ {
			cmp := strings.Compare(c.cands[i].term, other[i].term)
			if cmp != 0 {
				return -cmp
			}
		}
		return len(c.cands) - len(other)
	}
	if c.score < otherScore {
		return -1
	}
	if c.score > otherScore {
		return 1
	}
	return 0
}

func (c *phCorrection) join(sep, preTag, postTag string, highlight bool) string {
	parts := make([]string, len(c.cands))
	for i, cand := range c.cands {
		if !highlight || cand.user {
			parts[i] = cand.term
			continue
		}
		var b strings.Builder
		if i == 0 || c.cands[i-1].user {
			b.WriteString(preTag)
		}
		b.WriteString(cand.term)
		if len(c.cands) == i+1 || c.cands[i+1].user {
			b.WriteString(postTag)
		}
		parts[i] = b.String()
	}
	return strings.Join(parts, sep)
}

// phQueue is Lucene's PriorityQueue of corrections (least element on top).
type phQueue struct {
	heap    []*phCorrection // 1-based
	size    int
	maxSize int
}

func phCorrLess(a, b *phCorrection) bool { return a.compareTo(b.score, b.cands) < 0 }

func newPhQueue(maxSize int) *phQueue {
	return &phQueue{heap: make([]*phCorrection, maxSize+1), maxSize: maxSize}
}

func (q *phQueue) add(c *phCorrection) {
	q.size++
	q.heap[q.size] = c
	i := q.size
	node := q.heap[i]
	j := i >> 1
	for j > 0 && phCorrLess(node, q.heap[j]) {
		q.heap[i] = q.heap[j]
		i = j
		j >>= 1
	}
	q.heap[i] = node
}

func (q *phQueue) top() *phCorrection { return q.heap[1] }

func (q *phQueue) downHeap(i int) {
	node := q.heap[i]
	j := i << 1
	k := j + 1
	if k <= q.size && phCorrLess(q.heap[k], q.heap[j]) {
		j = k
	}
	for j <= q.size && phCorrLess(q.heap[j], node) {
		q.heap[i] = q.heap[j]
		i = j
		j = i << 1
		k = j + 1
		if k <= q.size && phCorrLess(q.heap[k], q.heap[j]) {
			j = k
		}
	}
	q.heap[i] = node
}

func (q *phQueue) pop() *phCorrection {
	if q.size == 0 {
		return nil
	}
	res := q.heap[1]
	q.heap[1] = q.heap[q.size]
	q.heap[q.size] = nil
	q.size--
	if q.size > 0 {
		q.downHeap(1)
	}
	return res
}

type phCandidateScorer struct {
	scorer   *phScorer
	maxNum   int
	gramSize int
}

func (cs *phCandidateScorer) findBest(sets []*phSet, errorFraction float32, cutoff float64) []*phCorrection {
	if len(sets) == 0 || cs.maxNum <= 0 {
		return nil
	}
	q := newPhQueue(cs.maxNum)
	var numMis int
	if errorFraction >= 1.0 {
		numMis = int(int32(errorFraction))
	} else {
		numMis = int(math.Floor(float64(errorFraction*float32(len(sets))) + 0.5))
	}
	cs.findCandidates(sets, make([]*phCand, len(sets)), 0, max(1, numMis), q, cutoff, 0.0)
	res := make([]*phCorrection, q.size)
	for i := len(res) - 1; i >= 0; i-- {
		res[i] = q.pop()
	}
	return res
}

func (cs *phCandidateScorer) findCandidates(sets []*phSet, path []*phCand, ord, left int, q *phQueue, cutoff, pathScore float64) {
	cur := sets[ord]
	if ord == len(sets)-1 {
		path[ord] = cur.orig
		cs.updateTop(sets, path, q, cutoff, pathScore+cs.scorer.score(path, sets, ord, cs.gramSize))
		if left > 0 {
			for _, c := range cur.cands {
				path[ord] = c
				cs.updateTop(sets, path, q, cutoff, pathScore+cs.scorer.score(path, sets, ord, cs.gramSize))
			}
		}
		return
	}
	if left > 0 {
		path[ord] = cur.orig
		cs.findCandidates(sets, path, ord+1, left, q, cutoff, pathScore+cs.scorer.score(path, sets, ord, cs.gramSize))
		for _, c := range cur.cands {
			path[ord] = c
			cs.findCandidates(sets, path, ord+1, left-1, q, cutoff, pathScore+cs.scorer.score(path, sets, ord, cs.gramSize))
		}
	} else {
		path[ord] = cur.orig
		cs.findCandidates(sets, path, ord+1, 0, q, cutoff, pathScore+cs.scorer.score(path, sets, ord, cs.gramSize))
	}
}

func (cs *phCandidateScorer) updateTop(sets []*phSet, path []*phCand, q *phQueue, cutoff, score float64) {
	score = phExp(score)
	if score > cutoff {
		if q.size < cs.maxNum {
			c := make([]*phCand, len(sets))
			copy(c, path)
			q.add(&phCorrection{score: score, cands: c})
		} else if q.top().compareTo(score, path) < 0 {
			top := q.top()
			copy(top.cands, path)
			top.score = score
			q.downHeap(1)
		}
	}
}

func (cs *phCandidateScorer) score(path []*phCand, sets []*phSet) float64 {
	s := 0.0
	for i := range sets {
		s += cs.scorer.score(path, sets, i, cs.gramSize)
	}
	return phExp(s)
}

// ---------------------------------------------------------------- request parsing

type phSmoothing struct {
	model    string
	discount float64
	alpha    float64
	tri      float64
	bi       float64
	uni      float64
}

type phraseRequest struct {
	field         string
	analyzer      string
	size          int
	shardSize     int
	hasShardSize  bool
	realWord      float32
	confidence    float32
	separator     string
	maxErrors     float32
	gramSize      int
	hasGramSize   bool
	forceUnigrams bool
	tokenLimit    int
	preTag        string
	postTag       string
	highlight     bool
	collateSource any
	collateParams map[string]any
	collatePrune  bool
	smoothing     *phSmoothing
	generators    []*phGenRequest
}

type phGenRequest struct {
	field  string
	opts   map[string]any
	parsed *phGenSpec
}

var phGenFields = map[string]bool{
	"field": true, "pre_filter": true, "post_filter": true, "suggest_mode": true, "min_doc_freq": true,
	"accuracy": true, "size": true, "sort": true, "string_distance": true, "max_inspections": true,
	"max_term_freq": true, "max_edits": true, "min_word_length": true, "prefix_length": true,
}

func phIsScalar(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return false
	}
	return true
}

func phParseErr(reason string) *apiError {
	return &apiError{Type: "parsing_exception", Status: 400, Reason: reason}
}

func phXContentErr(reason string, cause *apiError) *apiError {
	return &apiError{Type: "x_content_parse_exception", Status: 400, Reason: reason, CausedBy: cause}
}

func phNum(v any) float64 {
	f, _ := toFloat(v)
	return f
}

func parsePhraseRequest(body map[string]any) (*phraseRequest, *apiError) {
	r := &phraseRequest{size: 5, realWord: 0.95, confidence: 1.0, separator: " ", maxErrors: 0.5,
		forceUnigrams: true, tokenLimit: 10}
	hasField := false
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := body[k]
		if phIsScalar(v) {
			switch k {
			case "analyzer":
				r.analyzer, _ = toString(v)
			case "field":
				r.field, _ = toString(v)
				hasField = true
			case "size":
				n := int(phNum(v))
				if n <= 0 {
					return nil, suggestIllegal("size must be positive")
				}
				r.size = n
			case "shard_size":
				r.shardSize = int(phNum(v))
				r.hasShardSize = true
			case "real_word_error_likelihood":
				f := float32(phNum(v))
				if f <= 0 {
					return nil, suggestIllegal("real_word_error_likelihood must be > 0.0")
				}
				r.realWord = f
			case "confidence":
				f := float32(phNum(v))
				if f < 0 {
					return nil, suggestIllegal("confidence must be >= 0.0")
				}
				r.confidence = f
			case "separator":
				r.separator, _ = toString(v)
			case "max_errors":
				f := float32(phNum(v))
				if f <= 0 {
					return nil, suggestIllegal("max_error must be > 0.0")
				}
				r.maxErrors = f
			case "gram_size":
				n := int(phNum(v))
				if n < 1 {
					return nil, suggestIllegal("gramSize must be >= 1")
				}
				r.gramSize, r.hasGramSize = n, true
			case "force_unigrams":
				if b, ok := v.(bool); ok {
					r.forceUnigrams = b
				} else if s, ok := v.(string); ok {
					r.forceUnigrams = s == "true"
				}
			case "token_limit":
				n := int(phNum(v))
				if n <= 0 {
					return nil, suggestIllegal("token_limit must be >= 1")
				}
				r.tokenLimit = n
			default:
				return nil, phParseErr("suggester[phrase] doesn't support field [" + k + "]")
			}
			continue
		}
		if arr, ok := v.([]any); ok {
			if k != "direct_generator" {
				return nil, phParseErr("suggester[phrase]  doesn't support array field [" + k + "]")
			}
			for _, e := range arr {
				m, ok := e.(map[string]any)
				if !ok {
					break
				}
				g, err := parsePhGenerator(m)
				if err != nil {
					return nil, err
				}
				r.generators = append(r.generators, g)
			}
			continue
		}
		obj := v.(map[string]any)
		switch k {
		case "smoothing":
			sm, err := parsePhSmoothing(obj)
			if err != nil {
				return nil, err
			}
			r.smoothing = sm
		case "highlight":
			var pre, post *string
			for hk, hv := range obj {
				if !phIsScalar(hv) {
					continue
				}
				s, _ := toString(hv)
				switch hk {
				case "pre_tag":
					pre = &s
				case "post_tag":
					post = &s
				default:
					return nil, phParseErr("suggester[phrase][highlight] doesn't support field [" + hk + "]")
				}
			}
			if (pre == nil) != (post == nil) {
				return nil, suggestIllegal("Pre and post tag must both be null or both not be null.")
			}
			if pre != nil {
				r.highlight, r.preTag, r.postTag = true, *pre, *post
			}
		case "collate":
			for ck, cv := range obj {
				switch ck {
				case "query":
					switch q := cv.(type) {
					case map[string]any:
						if src, ok := q["source"]; ok {
							r.collateSource = src
						} else if src, ok := q["inline"]; ok {
							r.collateSource = src
						}
					case string:
						r.collateSource = q
					}
				case "params":
					if m, ok := cv.(map[string]any); ok {
						r.collateParams = m
					}
				case "prune":
					b, ok := cv.(bool)
					if !ok {
						if s, isStr := cv.(string); isStr && (s == "true" || s == "false") {
							b, ok = s == "true", true
						}
					}
					if !ok {
						return nil, phParseErr("suggester[phrase][collate] prune must be either 'true' or 'false'")
					}
					r.collatePrune = b
				default:
					return nil, phParseErr("suggester[phrase][collate] doesn't support field [" + ck + "]")
				}
			}
		default:
			return nil, phParseErr("suggester[phrase]  doesn't support array field [" + k + "]")
		}
	}
	if !hasField {
		return nil, &apiError{Type: "parse_exception", Status: 400, Reason: "the required field option [field] is missing"}
	}
	if r.field == "" {
		return nil, suggestIllegal("suggestion field name is empty")
	}
	return r, nil
}

func parsePhGenerator(m map[string]any) (*phGenRequest, *apiError) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !phGenFields[k] {
			return nil, phXContentErr("[direct_generator] unknown field ["+k+"]", nil)
		}
	}
	g := &phGenRequest{opts: m}
	if s, ok := m["size"]; ok {
		if phNum(s) <= 0 {
			return nil, phXContentErr("[direct_generator] failed to parse field [size]",
				&apiError{Type: "illegal_argument_exception", Reason: "Size must be positive"})
		}
	}
	if e, ok := m["max_edits"]; ok {
		n := int(phNum(e))
		if n < 1 || n > 2 {
			return nil, phXContentErr("[direct_generator] failed to parse field [max_edits]",
				&apiError{Type: "illegal_argument_exception", Reason: "Illegal max_edits value " + strconv.Itoa(n)})
		}
	}
	f, ok := m["field"]
	if !ok {
		return nil, suggestIllegal("Required [field]")
	}
	g.field, _ = toString(f)
	return g, nil
}

func parsePhSmoothing(obj map[string]any) (*phSmoothing, *apiError) {
	var model *phSmoothing
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		inner, ok := obj[name].(map[string]any)
		if !ok {
			return nil, phParseErr("[smoothing] unknown token [" + phTokenName(obj[name]) + "] after [" + name + "]")
		}
		switch name {
		case "linear":
			sm := &phSmoothing{model: "linear"}
			for k, v := range inner {
				if !phIsScalar(v) {
					return nil, phParseErr("[linear] unknown token [" + phTokenName(v) + "] after [" + k + "]")
				}
				f := phNum(v)
				switch k {
				case "trigram_lambda":
					sm.tri = f
				case "bigram_lambda":
					sm.bi = f
				case "unigram_lambda":
					sm.uni = f
				default:
					return nil, suggestIllegal("suggester[phrase][smoothing][linear] doesn't support field [" + k + "]")
				}
				if f < 0 {
					return nil, suggestIllegal(k + " must be positive")
				}
			}
			if math.Abs(sm.tri+sm.bi+sm.uni-1.0) > 0.001 {
				return nil, suggestIllegal("linear smoothing lambdas must sum to 1")
			}
			model = sm
		case "laplace":
			sm := &phSmoothing{model: "laplace", alpha: 0.5}
			if v, ok := inner["alpha"]; ok && phIsScalar(v) {
				sm.alpha = phNum(v)
			}
			model = sm
		case "stupid_backoff":
			sm := &phSmoothing{model: "stupid_backoff", discount: 0.4}
			if v, ok := inner["discount"]; ok && phIsScalar(v) {
				sm.discount = phNum(v)
			}
			model = sm
		default:
			return nil, suggestIllegal("suggester[phrase] doesn't support object field [" + name + "]")
		}
	}
	return model, nil
}

func phTokenName(v any) string {
	switch v.(type) {
	case []any:
		return "START_ARRAY"
	case string:
		return "VALUE_STRING"
	case bool:
		return "VALUE_BOOLEAN"
	case nil:
		return "VALUE_NULL"
	}
	return "VALUE_NUMBER"
}

// buildPhGenSpec resolves a direct_generator against an index (shard level).
func buildPhGenSpec(idx *Index, g *phGenRequest) (*phGenSpec, *apiError) {
	sp := defaultPhGenSpec(g.field)
	m := g.opts
	if v, ok := m["size"]; ok {
		sp.size = int(phNum(v))
	}
	for _, k := range []string{"pre_filter", "post_filter"} {
		if v, ok := m[k]; ok {
			name, _ := toString(v)
			if !phAnalyzerExists(idx, name) {
				return nil, suggestShardError(idx, "Analyzer ["+name+"] doesn't exists")
			}
			if k == "pre_filter" {
				sp.preFilter = name
			} else {
				sp.postFilter = name
			}
		}
	}
	if v, ok := m["accuracy"]; ok {
		sp.accuracy = float32(phNum(v))
	}
	if v, ok := m["suggest_mode"]; ok {
		s, _ := toString(v)
		s = strings.ToLower(s)
		if s != "missing" && s != "popular" && s != "always" {
			return nil, suggestShardError(idx, "Illegal suggest mode "+s)
		}
		sp.mode = s
	}
	if v, ok := m["sort"]; ok {
		s, _ := toString(v)
		ls := strings.ToLower(s)
		if ls != "score" && ls != "frequency" {
			return nil, suggestShardError(idx, "No enum constant org.opensearch.search.suggest.SortBy."+strings.ToUpper(s))
		}
		sp.sortBy = ls
	}
	if v, ok := m["string_distance"]; ok {
		s, _ := toString(v)
		ls := strings.ToLower(s)
		switch ls {
		case "internal", "damerau_levenshtein", "levenshtein", "jaro_winkler", "ngram":
		default:
			return nil, suggestShardError(idx, "Illegal distance option "+ls)
		}
		sp.distance = ls
	}
	if v, ok := m["max_edits"]; ok {
		sp.maxEdits = int(phNum(v))
	}
	if v, ok := m["max_inspections"]; ok {
		sp.maxInspections = int(phNum(v))
	}
	if v, ok := m["max_term_freq"]; ok {
		sp.maxTermFreq = float32(phNum(v))
	}
	if v, ok := m["prefix_length"]; ok {
		sp.prefixLength = int(phNum(v))
	}
	if v, ok := m["min_word_length"]; ok {
		sp.minWordLength = int(phNum(v))
	}
	if v, ok := m["min_doc_freq"]; ok {
		sp.minDocFreq = float32(phNum(v))
	}
	return sp, nil
}

func phAnalyzerExists(idx *Index, name string) bool {
	if name == "" {
		return false
	}
	if idx != nil && (analysisScope{idx: idx}).indexHasAnalyzer(name) {
		return true
	}
	if _, ok := globalAnalyzerTokens(name, ""); ok {
		return true
	}
	return prebuiltChain(name) != nil
}

// phShingle returns the first shingle filter of an analyzer.
func phShingle(idx *Index, analyzer string) (minS, maxS int, unigrams, found bool) {
	if idx == nil || !(analysisScope{idx: idx}).indexHasAnalyzer(analyzer) {
		return
	}
	c, err := (analysisScope{idx: idx}).indexAnalyzerChain(analyzer)
	if err != nil || c == nil {
		return
	}
	for _, f := range c.filters {
		if f.spec.typ == "shingle" {
			p := f.spec.params
			return fsInt(p, "min_shingle_size", 2), fsInt(p, "max_shingle_size", 2), fsBool(p, "output_unigrams", true), true
		}
	}
	return
}

// ---------------------------------------------------------------- execution

type phOption struct {
	text        string
	highlighted *string
	score       float32
	collate     *bool
}

type phEntry struct {
	cutoff  float64
	options []*phOption
}

func (e *phEntry) addOption(o *phOption) {
	if float64(o.score) > e.cutoff {
		e.options = append(e.options, o)
	}
}

func phraseSuggest(spec, body map[string]any, indices []*Index, globalText any) ([]any, *apiError) {
	req, err := parsePhraseRequest(body)
	if err != nil {
		return nil, err
	}
	var first *Index
	if len(indices) > 0 {
		first = indices[0]
	}
	rawText, hasText := spec["text"]
	if !hasText {
		rawText, hasText = spec["prefix"]
	}
	if !hasText && globalText != nil {
		rawText, hasText = globalText, true
	}
	if !hasText {
		return nil, suggestShardError(first, "The required text option is missing")
	}
	text, _ := toString(rawText)

	var entries []*phEntry
	var lastErr *apiError
	for _, idx := range indices {
		e, err := phraseSuggestIndex(req, idx, text)
		if err != nil {
			lastErr = err
			continue
		}
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		entries = append(entries, &phEntry{cutoff: math.SmallestNonzeroFloat64})
	}
	entry := entries[0]
	if len(entries) > 1 {
		merged := map[string]*phOption{}
		order := []*phOption{}
		for _, e := range entries {
			entry.cutoff = math.Max(entry.cutoff, e.cutoff)
			for _, o := range e.options {
				if m, ok := merged[o.text]; ok {
					if o.score > m.score {
						m.score = o.score
					}
					if o.collate != nil {
						if m.collate == nil {
							m.collate = o.collate
						} else {
							b := *m.collate || *o.collate
							m.collate = &b
						}
					}
					continue
				}
				c := *o
				merged[o.text] = &c
				order = append(order, &c)
			}
		}
		out := &phEntry{cutoff: entry.cutoff}
		for _, o := range order {
			out.addOption(o)
		}
		sort.SliceStable(out.options, func(i, j int) bool {
			a, b := out.options[i], out.options[j]
			if a.score != b.score {
				return a.score > b.score
			}
			return a.text < b.text
		})
		entry = out
	}
	if len(entry.options) > req.size {
		entry.options = entry.options[:req.size]
	}
	opts := make([]any, 0, len(entry.options))
	for _, o := range entry.options {
		oj := orderedJSON{{"text", o.text}}
		if o.highlighted != nil {
			oj = append(oj, orderedKV{"highlighted", *o.highlighted})
		}
		oj = append(oj, orderedKV{"score", plJSONFloat32(o.score)})
		if o.collate != nil {
			oj = append(oj, orderedKV{"collate_match", *o.collate})
		}
		opts = append(opts, oj)
	}
	return []any{orderedJSON{{"text", text}, {"offset", 0}, {"length", utf16Offset(text)}, {"options", opts}}}, nil
}

func phraseSuggestIndex(req *phraseRequest, idx *Index, text string) (*phEntry, *apiError) {
	def, ok := mappingFieldDef(idx, req.field)
	if !ok {
		return nil, suggestShardError(idx, "no mapping found for field ["+req.field+"]")
	}
	analyzer := req.analyzer
	if analyzer == "" {
		analyzer = phraseSearchAnalyzer(idx, def)
	} else if !phAnalyzerExists(idx, analyzer) {
		return nil, suggestShardError(idx, "analyzer ["+analyzer+"] doesn't exists")
	}
	shardSize := max(req.size, 5)
	if req.hasShardSize {
		shardSize = req.shardSize
	}
	var genSpecs []*phGenSpec
	for _, g := range req.generators {
		sp, err := buildPhGenSpec(idx, g)
		if err != nil {
			return nil, err
		}
		genSpecs = append(genSpecs, sp)
	}
	gramSize := 1
	if req.hasGramSize {
		gramSize = req.gramSize
	}
	if !req.hasGramSize || len(genSpecs) == 0 {
		minS, maxS, unigrams, found := phShingle(idx, analyzer)
		if !req.hasGramSize && found {
			gramSize = maxS
		}
		if len(genSpecs) == 0 {
			if found && minS > 1 && !unigrams && req.forceUnigrams {
				return nil, suggestShardError(idx, "The default candidate generator for phrase suggest can't operate on field: ["+
					req.field+"] since it doesn't emit unigrams. If this is intentional try to set the candidate generator field explicitly")
			}
			genSpecs = append(genSpecs, defaultPhGenSpec(req.field))
		}
	}

	realWord := float64(req.realWord)
	entry := &phEntry{cutoff: math.SmallestNonzeroFloat64}
	var gens []*phGen
	for _, sp := range genSpecs {
		if t := phraseFieldTerms(idx, sp.field); t != nil {
			gens = append(gens, newPhGen(sp, idx, t, realWord))
		}
	}
	suggestTerms := phraseFieldTerms(idx, req.field)
	if len(gens) == 0 || suggestTerms == nil {
		return entry, nil
	}
	multi := &phMultiGen{gens: gens, numCandidates: shardSize}

	// NoisyChannelSpellChecker.getCorrections
	var sets []*phSet
	var cur *phSet
	anyUnigram, anyTokens := false, false
	prev := -1
	for _, tok := range analyzeTokensWithIndex(idx, analyzer, text) {
		posInc := tok.pos - prev
		prev = tok.pos
		anyTokens = true
		if req.forceUnigrams && tok.typ == "shingle" {
			continue
		}
		anyUnigram = true
		if posInc == 0 && tok.typ == "SYNONYM" && cur != nil {
			st := multi.termStats(tok.term)
			if st.df > 0 {
				cur.cands = append(cur.cands, multi.createCandidate(tok.term, st, realWord, false))
			}
			continue
		}
		if cur != nil {
			sets = append(sets, cur)
		}
		cur = &phSet{orig: multi.createCandidate(tok.term, multi.termStats(tok.term), 1.0, true)}
	}
	if cur != nil {
		sets = append(sets, cur)
	}
	if req.forceUnigrams && !anyUnigram && anyTokens {
		return nil, &apiError{Type: "search_phase_execution_exception", Status: 500, Reason: "all shards failed",
			Shard: &shardFailure{status: 500, index: idx.Name,
				root:  orderedJSON{{"type", "illegal_state_exception"}, {"reason", "At least one unigram is required but all tokens were ngrams"}},
				shard: orderedJSON{{"type", "illegal_state_exception"}, {"reason", "At least one unigram is required but all tokens were ngrams"}},
				top:   orderedJSON{{"type", "illegal_state_exception"}, {"reason", "At least one unigram is required but all tokens were ngrams"}}}}
	}
	if len(sets) == 0 || len(sets) >= req.tokenLimit {
		return entry, nil
	}
	for _, s := range sets {
		multi.drawCandidates(s)
	}
	scorer := newPhScorer(suggestTerms, realWord, req.separator, req.smoothing)
	cs := &phCandidateScorer{scorer: scorer, maxNum: shardSize, gramSize: gramSize}
	cutoff := math.SmallestNonzeroFloat64
	if req.confidence > 0 {
		path := make([]*phCand, len(sets))
		for i, s := range sets {
			path[i] = s.orig
		}
		cutoff = cs.score(path, sets) * float64(req.confidence)
	}
	corrections := cs.findBest(sets, req.maxErrors, cutoff)
	entry.cutoff = cutoff

	for _, corr := range corrections {
		phrase := corr.join(" ", "", "", false)
		var collate *bool
		match := true
		if req.collateSource != nil {
			m, err := phCollateMatches(idx, req, phrase)
			if err != nil {
				return nil, err
			}
			match = m
			if req.collatePrune {
				collate = &m
			}
		}
		if !match && !req.collatePrune {
			continue
		}
		o := &phOption{text: phrase, score: float32(corr.score), collate: collate}
		if req.highlight {
			h := corr.join(" ", req.preTag, req.postTag, true)
			o.highlighted = &h
		}
		entry.addOption(o)
	}
	return entry, nil
}

// phCollateMatches renders the collate template for a suggestion and reports
// whether its query matches any document of the index.
func phCollateMatches(idx *Index, req *phraseRequest, suggestion string) (bool, *apiError) {
	tpl, ok := mustacheTemplateText(req.collateSource)
	if !ok {
		return false, nil
	}
	vars := map[string]any{}
	for k, v := range req.collateParams {
		vars[k] = v
	}
	vars["suggestion"] = suggestion
	src, merr := mustacheRender(tpl, vars)
	if merr != nil {
		return false, mustacheScriptError(tpl, merr)
	}
	var q map[string]any
	if err := unmarshalJSON([]byte(src), &q); err != nil {
		return false, &apiError{Type: "parsing_exception", Status: 400, Reason: "Failed to parse collate query"}
	}
	for _, id := range idx.Order {
		doc, live := idx.Docs[id]
		if !live {
			continue
		}
		if _, pending := idx.Pending[id]; pending {
			continue
		}
		ok, _, err := newQueryCtxForDoc(idx, id, doc).eval(q)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// ---------------------------------------------------------------- correctly rounded math

// HotSpot's Math.log10 and Math.exp intrinsics are (practically) correctly
// rounded, unlike Go's math.Log10/math.Exp. Phrase scores sum and
// exponentiate many log10 values, so ties between corrections depend on the
// last bit: these helpers compute the correctly rounded results.

const phBigPrec = 160

var (
	phConstOnce sync.Once
	phLn2       *big.Float
	phLn10      *big.Float
)

func phNewBig(f float64) *big.Float { return new(big.Float).SetPrec(phBigPrec).SetFloat64(f) }

// phAtanh2 returns 2*atanh(z) = ln((1+z)/(1-z)) for small |z|.
func phAtanh2(z *big.Float) *big.Float {
	z2 := new(big.Float).SetPrec(phBigPrec).Mul(z, z)
	pow := new(big.Float).SetPrec(phBigPrec).Set(z)
	sum := new(big.Float).SetPrec(phBigPrec).Set(z)
	term := new(big.Float).SetPrec(phBigPrec)
	for n := int64(3); n < 2000; n += 2 {
		pow.Mul(pow, z2)
		term.Quo(pow, new(big.Float).SetPrec(phBigPrec).SetInt64(n))
		if term.Sign() == 0 || term.MantExp(nil) < sum.MantExp(nil)-phBigPrec-8 {
			break
		}
		sum.Add(sum, term)
	}
	return sum.Mul(sum, phNewBig(2))
}

func phConsts() {
	phConstOnce.Do(func() {
		// ln2 = 2*atanh(1/3); ln10 = 3*ln2 + ln(1.25) with ln(1.25) = 2*atanh(1/9).
		third := new(big.Float).SetPrec(phBigPrec).Quo(phNewBig(1), phNewBig(3))
		phLn2 = phAtanh2(third)
		ninth := new(big.Float).SetPrec(phBigPrec).Quo(phNewBig(1), phNewBig(9))
		phLn10 = new(big.Float).SetPrec(phBigPrec).Mul(phLn2, phNewBig(3))
		phLn10.Add(phLn10, phAtanh2(ninth))
	})
}

// phLog10 is a correctly rounded log10.
func phLog10(x float64) float64 {
	switch {
	case math.IsNaN(x) || x < 0:
		return math.NaN()
	case x == 0:
		return math.Inf(-1)
	case math.IsInf(x, 1):
		return x
	case x == 1:
		return 0
	}
	phConsts()
	frac, e := math.Frexp(x) // x = frac * 2^e, frac in [0.5, 1)
	if frac < math.Sqrt2/2 {
		frac *= 2
		e--
	}
	m := phNewBig(frac)
	num := new(big.Float).SetPrec(phBigPrec).Sub(m, phNewBig(1))
	den := new(big.Float).SetPrec(phBigPrec).Add(m, phNewBig(1))
	ln := phAtanh2(num.Quo(num, den))
	ln.Add(ln, new(big.Float).SetPrec(phBigPrec).Mul(phLn2, phNewBig(float64(e))))
	f, _ := ln.Quo(ln, phLn10).Float64()
	return f
}

// phExp is a correctly rounded exp.
func phExp(y float64) float64 {
	switch {
	case math.IsNaN(y):
		return y
	case y > 710:
		return math.Inf(1)
	case y < -746:
		return 0
	case y == 0:
		return 1
	}
	phConsts()
	k := math.Round(y / math.Ln2)
	r := phNewBig(y)
	r.Sub(r, new(big.Float).SetPrec(phBigPrec).Mul(phLn2, phNewBig(k)))
	const halvings = 8
	r.SetMantExp(r, -halvings)
	sum := phNewBig(1)
	term := phNewBig(1)
	for n := int64(1); n < 200; n++ {
		term.Mul(term, r)
		term.Quo(term, new(big.Float).SetPrec(phBigPrec).SetInt64(n))
		if term.Sign() == 0 || term.MantExp(nil) < sum.MantExp(nil)-phBigPrec-8 {
			break
		}
		sum.Add(sum, term)
	}
	for i := 0; i < halvings; i++ {
		sum.Mul(sum, sum)
	}
	sum.SetMantExp(sum, int(k))
	f, _ := sum.Float64()
	return f
}
