package main

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// Analysis chains: an analyzer resolved into char filters, a tokenizer and
// token filters, from the global registry, the index settings or inline
// definitions of an _analyze request. Running a chain mirrors Lucene's token
// stream contract closely enough for _analyze (including explain), indexing
// and query analysis.

type chainCharFilter struct {
	name   string
	typ    string
	params map[string]any
}

type chainFilter struct {
	name string
	spec filterSpec
}

type analysisChain struct {
	name    string
	custom  bool   // reported as custom_analyzer in explain output
	builtin string // prebuilt analyzer evaluated by globalAnalyzerTokens
	chars   []chainCharFilter
	tokName string
	tok     tokenizerFunc
	filters []chainFilter
	gap     int
	// keywordAttr reports whether a built-in analyzer carries the keyword
	// attribute (stemming language analyzers).
	keywordAttr bool
}

// chainRun is the result of analyzing one text.
type chainRun struct {
	toks        []analyzedToken
	posCount    int // positions consumed, trailing holes included
	finalOffset int
	charTexts   []string
	stages      [][]analyzedToken // tokenizer output, then after each filter
	stageCounts []int
}

// filteringTypes remove tokens but keep their positions (Lucene's
// FilteringTokenFilter), so they never change the consumed position count.
var filteringTypes = map[string]bool{
	"stop": true, "length": true, "keep": true, "keep_types": true,
	"predicate_token_filter": true, "limit": true, "fingerprint": true,
}

// keywordAttrTypes are token filters adding Lucene's KeywordAttribute.
var keywordAttrTypes = map[string]bool{
	"keyword_marker": true, "keyword_repeat": true, "stemmer": true, "porter_stem": true,
	"snowball": true, "kstem": true, "stemmer_override": true, "hunspell": true,
}

func filterHasKeywordAttr(typ string) bool {
	return keywordAttrTypes[typ] || strings.HasSuffix(typ, "_stem")
}

func lastPosition(toks []analyzedToken) int {
	last := -1
	for _, t := range toks {
		if t.pos > last {
			last = t.pos
		}
	}
	return last
}

func copyTokens(toks []analyzedToken) []analyzedToken {
	return append([]analyzedToken(nil), toks...)
}

// run analyzes one text. With stages set, every intermediate stream is kept.
func (c *analysisChain) run(idx *Index, text string, stages bool) (chainRun, *apiError) {
	res := chainRun{finalOffset: utf16Offset(text)}
	if c.builtin != "" {
		toks, ok := globalAnalyzerTokens(c.builtin, text)
		if !ok {
			toks = tokenizePositions(text, false)
		}
		res.toks = toks
		res.posCount = len(tokenizePositions(text, false))
		if n := lastPosition(toks) + 1; n > res.posCount {
			res.posCount = n
		}
		return res, nil
	}
	cur := text
	var corrections []func(int) int
	for _, cf := range c.chars {
		out, corr, err := applyCharFilter(cf, cur)
		if err != nil {
			return res, err
		}
		cur = out
		corrections = append(corrections, corr)
		if stages {
			res.charTexts = append(res.charTexts, out)
		}
	}
	toks := c.tok(cur)
	if len(corrections) > 0 {
		correct := func(o int) int {
			for i := len(corrections) - 1; i >= 0; i-- {
				o = corrections[i](o)
			}
			return o
		}
		for i := range toks {
			toks[i].start = correct(toks[i].start)
			toks[i].end = correct(toks[i].end)
		}
		res.finalOffset = correct(utf16Offset(cur))
	}
	posCount := lastPosition(toks) + 1
	if stages {
		res.stages = append(res.stages, copyTokens(toks))
		res.stageCounts = append(res.stageCounts, posCount)
	}
	for fi, f := range c.filters {
		spec := f.spec
		spec.finalOffset = res.finalOffset
		if spec.typ == "synonym" || spec.typ == "synonym_graph" {
			spec.ruleAnalyzer = c.prefixAnalyzer(idx, fi)
		}
		before := lastPosition(toks)
		out, err := applyFilterOne(idx, copyTokens(toks), spec)
		if err != nil {
			return res, err
		}
		if !filteringTypes[spec.typ] && len(toks) > 0 {
			posCount += lastPosition(out) - before
		}
		toks = out
		if stages {
			res.stages = append(res.stages, copyTokens(toks))
			res.stageCounts = append(res.stageCounts, posCount)
		}
	}
	res.toks = toks
	res.posCount = posCount
	return res, nil
}

// prefixAnalyzer analyzes synonym rules with the tokenizer and the filters
// preceding filter i, as OpenSearch does.
func (c *analysisChain) prefixAnalyzer(idx *Index, i int) func(string) []analyzedToken {
	return func(s string) []analyzedToken {
		sub := &analysisChain{tok: c.tok, filters: c.filters[:i]}
		if sub.tok == nil {
			sub.tok, _ = buildTokenizer("whitespace", nil, 1)
		}
		r, err := sub.run(idx, s, false)
		if err != nil {
			return nil
		}
		return r.toks
	}
}

// ---------------------------------------------------------------- resolving

type analysisScope struct {
	idx *Index
}

func (sc analysisScope) global() bool { return sc.idx == nil }

func (sc analysisScope) setting(key string) (any, bool) {
	if sc.idx == nil {
		return nil, false
	}
	return findSetting(sc.idx.Settings, key)
}

func (sc analysisScope) maxNgramDiff() int {
	if sc.idx != nil {
		if n, ok := settingInt(sc.idx.Settings, "max_ngram_diff"); ok {
			return n
		}
	}
	return 1
}

func (sc analysisScope) maxShingleDiff() int {
	if sc.idx != nil {
		if n, ok := settingInt(sc.idx.Settings, "max_shingle_diff"); ok {
			return n
		}
	}
	return 3
}

func (sc analysisScope) componentType(kind, name string) (string, bool) {
	raw, ok := sc.setting("analysis." + kind + "." + name + ".type")
	if !ok {
		return "", false
	}
	s, _ := raw.(string)
	return s, true
}

func (sc analysisScope) componentDefined(kind, name string) bool {
	if sc.idx == nil {
		return false
	}
	return len(subSettings(sc.idx.Settings, "analysis."+kind+"."+name)) > 0
}

func (sc analysisScope) componentParams(kind, name string) map[string]any {
	if sc.idx == nil {
		return nil
	}
	return subSettings(sc.idx.Settings, "analysis."+kind+"."+name)
}

func notFound(sc analysisScope, kind, name string) *apiError {
	if sc.global() {
		return analyzeRequestError("illegal_argument_exception", "failed to find global "+kind+" under ["+name+"]")
	}
	return analyzeRequestError("illegal_argument_exception", "failed to find "+kind+" under ["+name+"]")
}

func anonymousJSON(def map[string]any) string {
	b, _ := json.Marshal(def)
	return string(b)
}

// resolveTokenizer resolves a tokenizer name or inline definition.
func (sc analysisScope) resolveTokenizer(name string, inline map[string]any) (string, tokenizerFunc, *apiError) {
	if inline != nil {
		typ, _ := inline["type"].(string)
		if typ == "" {
			return "", nil, analyzeRequestError("illegal_argument_exception", "Missing [type] setting for anonymous tokenizer: "+anonymousJSON(inline))
		}
		if !tokenizerTypeKnown[typ] {
			return "", nil, analyzeRequestError("illegal_argument_exception", "failed to find global tokenizer under ["+typ+"]")
		}
		tok, err := buildTokenizer(typ, inline, sc.maxNgramDiff())
		if err != nil {
			return "", nil, err
		}
		return "__anonymous__" + typ, tok, nil
	}
	if typ, ok := sc.componentType("tokenizer", name); ok {
		tok, err := buildTokenizer(typ, sc.componentParams("tokenizer", name), sc.maxNgramDiff())
		if err != nil {
			return "", nil, err
		}
		if tok == nil {
			return "", nil, notFound(sc, "tokenizer", name)
		}
		return name, tok, nil
	}
	if tokenizerTypeKnown[name] {
		tok, err := buildTokenizer(name, nil, sc.maxNgramDiff())
		if err != nil {
			return "", nil, err
		}
		return name, tok, nil
	}
	return "", nil, notFound(sc, "tokenizer", name)
}

func (sc analysisScope) withLimits(p map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range p {
		out[k] = v
	}
	out["_max_ngram_diff"] = sc.maxNgramDiff()
	out["_max_shingle_diff"] = sc.maxShingleDiff()
	return out
}

// resolveFilter resolves a token filter reference.
func (sc analysisScope) resolveFilter(c analyzeComponent) (chainFilter, *apiError) {
	if c.inline != nil {
		typ, _ := c.inline["type"].(string)
		if !knownFilterTypes[typ] {
			return chainFilter{}, analyzeRequestError("illegal_argument_exception", "failed to find global filter under ["+typ+"]")
		}
		return chainFilter{name: "__anonymous__" + typ, spec: filterSpec{typ: typ, params: sc.withLimits(c.inline)}}, nil
	}
	if typ, ok := sc.componentType("filter", c.name); ok {
		return chainFilter{name: c.name, spec: filterSpec{typ: typ, params: sc.withLimits(sc.componentParams("filter", c.name))}}, nil
	}
	if spec, ok := globalFilterSpec(c.name); ok {
		spec.params = sc.withLimits(spec.params)
		return chainFilter{name: c.name, spec: spec}, nil
	}
	return chainFilter{}, notFound(sc, "filter", c.name)
}

var charFilterTypeKnown = map[string]bool{"html_strip": true, "mapping": true, "pattern_replace": true}

func (sc analysisScope) resolveCharFilter(c analyzeComponent) (chainCharFilter, *apiError) {
	if c.inline != nil {
		typ, _ := c.inline["type"].(string)
		if !charFilterTypeKnown[typ] {
			return chainCharFilter{}, analyzeRequestError("illegal_argument_exception", "failed to find global char_filter under ["+typ+"]")
		}
		return chainCharFilter{name: "__anonymous__" + typ, typ: typ, params: c.inline}, nil
	}
	if typ, ok := sc.componentType("char_filter", c.name); ok {
		return chainCharFilter{name: c.name, typ: typ, params: sc.componentParams("char_filter", c.name)}, nil
	}
	if charFilterTypeKnown[c.name] {
		return chainCharFilter{name: c.name, typ: c.name}, nil
	}
	return chainCharFilter{}, notFound(sc, "char_filter", c.name)
}

// validateFilter applies a filter once so configuration errors surface when
// the chain is built.
func validateChainFilter(idx *Index, f chainFilter) *apiError {
	dummy := []analyzedToken{{term: "x", start: 0, end: 1, typ: "word"}}
	_, err := applyFilterOne(idx, dummy, f.spec)
	return err
}

func validateChainCharFilter(cf chainCharFilter) *apiError {
	_, _, err := applyCharFilter(cf, "")
	return err
}

// buildRequestChain builds the custom chain of an _analyze request.
func (sc analysisScope) buildRequestChain(tokName string, tokInline map[string]any, chars, filters []analyzeComponent) (*analysisChain, *apiError) {
	c := &analysisChain{custom: true, gap: 100}
	name, tok, err := sc.resolveTokenizer(tokName, tokInline)
	if err != nil {
		return nil, err
	}
	c.tokName, c.tok = name, tok
	for _, comp := range chars {
		cf, err := sc.resolveCharFilter(comp)
		if err != nil {
			return nil, err
		}
		if err := validateChainCharFilter(cf); err != nil {
			return nil, err
		}
		c.chars = append(c.chars, cf)
	}
	for _, comp := range filters {
		f, err := sc.resolveFilter(comp)
		if err != nil {
			return nil, err
		}
		if err := validateChainFilter(sc.idx, f); err != nil {
			return nil, err
		}
		c.filters = append(c.filters, f)
	}
	return c, nil
}

// prebuiltChain returns the chain of a prebuilt analyzer, or nil.
func prebuiltChain(name string) *analysisChain {
	c, err := typedAnalyzerChain(name, name, nil)
	if err != nil || c == nil {
		if builtinAnalyzer(name) {
			return &analysisChain{name: name, builtin: name, keywordAttr: languageAnalyzerHasKeyword(name)}
		}
		return nil
	}
	return c
}

func languageAnalyzerHasKeyword(name string) bool {
	switch name {
	case "standard", "simple", "whitespace", "keyword", "stop", "pattern", "fingerprint", "lowercase", "cjk", "thai":
		return false
	}
	return builtinAnalyzer(name)
}

func stopFilterSpec(p map[string]any, def string) filterSpec {
	params := map[string]any{}
	if v, ok := p["stopwords"]; ok {
		params["stopwords"] = v
	} else {
		params["stopwords"] = def
	}
	if v, ok := p["stopwords_path"]; ok {
		params["stopwords_path"] = v
	}
	if v, ok := p["ignore_case"]; ok {
		params["ignore_case"] = v
	}
	return filterSpec{typ: "stop", params: params}
}

// typedAnalyzerChain builds the analyzers OpenSearch implements as fixed
// Lucene chains (standard, simple, stop, ...) with their parameters. It
// returns nil for types handled elsewhere (language analyzers).
func typedAnalyzerChain(name, typ string, p map[string]any) (*analysisChain, *apiError) {
	c := &analysisChain{name: name}
	tok := func(t string, params map[string]any) *apiError {
		f, err := buildTokenizer(t, params, 1)
		if err != nil {
			return err
		}
		c.tokName, c.tok = t, f
		return nil
	}
	maxLen := map[string]any{}
	if v, ok := p["max_token_length"]; ok {
		maxLen["max_token_length"] = v
	}
	switch typ {
	case "standard":
		if err := tok("standard", maxLen); err != nil {
			return nil, err
		}
		c.filters = []chainFilter{{name: "lowercase", spec: filterSpec{typ: "lowercase"}}, {name: "stop", spec: stopFilterSpec(p, "_none_")}}
	case "simple":
		if err := tok("lowercase", nil); err != nil {
			return nil, err
		}
	case "whitespace":
		if err := tok("whitespace", maxLen); err != nil {
			return nil, err
		}
	case "keyword":
		if err := tok("keyword", nil); err != nil {
			return nil, err
		}
	case "stop":
		if err := tok("lowercase", nil); err != nil {
			return nil, err
		}
		c.filters = []chainFilter{{name: "stop", spec: stopFilterSpec(p, "_english_")}}
	case "pattern":
		params := map[string]any{}
		for _, k := range []string{"pattern", "flags"} {
			if v, ok := p[k]; ok {
				params[k] = v
			}
		}
		if err := tok("pattern", params); err != nil {
			return nil, err
		}
		if fsBool(p, "lowercase", true) {
			c.filters = append(c.filters, chainFilter{name: "lowercase", spec: filterSpec{typ: "lowercase"}})
		}
		c.filters = append(c.filters, chainFilter{name: "stop", spec: stopFilterSpec(p, "_none_")})
	case "fingerprint":
		if err := tok("standard", nil); err != nil {
			return nil, err
		}
		fp := map[string]any{"separator": fsString(p, "separator", " ")}
		if v, ok := p["max_output_size"]; ok {
			fp["max_output_size"] = v
		}
		c.filters = []chainFilter{
			{name: "lowercase", spec: filterSpec{typ: "lowercase"}},
			{name: "asciifolding", spec: filterSpec{typ: "asciifolding"}},
			{name: "stop", spec: stopFilterSpec(p, "_none_")},
			{name: "fingerprint", spec: filterSpec{typ: "fingerprint", params: fp}},
		}
	case "snowball":
		lang := fsString(p, "language", "English")
		if err := tok("standard", nil); err != nil {
			return nil, err
		}
		def := "_none_"
		if strings.EqualFold(lang, "english") {
			def = "_english_"
		}
		c.filters = []chainFilter{
			{name: "lowercase", spec: filterSpec{typ: "lowercase"}},
			{name: "stop", spec: stopFilterSpec(p, def)},
			{name: "snowball", spec: filterSpec{typ: "snowball", params: map[string]any{"language": lang}}},
		}
		c.keywordAttr = true
	case "standard_html_strip":
		if err := tok("standard", maxLen); err != nil {
			return nil, err
		}
		c.chars = []chainCharFilter{{name: "html_strip", typ: "html_strip"}}
		c.filters = []chainFilter{{name: "lowercase", spec: filterSpec{typ: "lowercase"}}, {name: "stop", spec: stopFilterSpec(p, "_none_")}}
	default:
		return nil, nil
	}
	return c, nil
}

var analyzerTypeKnown = map[string]bool{
	"custom": true, "standard": true, "simple": true, "whitespace": true, "keyword": true,
	"stop": true, "pattern": true, "fingerprint": true, "snowball": true, "standard_html_strip": true,
}

func analyzerTypeValid(typ string) bool {
	return analyzerTypeKnown[typ] || (builtinAnalyzer(typ) && typ != "lowercase")
}

// indexAnalyzerChain builds an analyzer configured in the index settings.
// The returned error is the bare message used inside "Failed to build
// analyzers" reports.
func (sc analysisScope) indexAnalyzerChain(name string) (*analysisChain, *apiError) {
	p := sc.componentParams("analyzer", name)
	typ, _ := p["type"].(string)
	tokName, hasTok := p["tokenizer"].(string)
	if typ == "" && hasTok {
		typ = "custom"
	}
	gap := 100
	if n, ok := toFloat(p["position_increment_gap"]); ok {
		gap = int(n)
	}
	if typ != "custom" {
		c, err := typedAnalyzerChain(name, typ, p)
		if err != nil {
			return nil, err
		}
		if c == nil {
			// Language analyzers keep their prebuilt implementation.
			c = &analysisChain{name: name, builtin: typ, keywordAttr: languageAnalyzerHasKeyword(typ)}
		}
		c.gap = gap
		return c, nil
	}
	c := &analysisChain{name: name, custom: true, gap: gap}
	if !hasTok {
		return nil, filterError("Custom Analyzer [" + name + "] must be configured with a tokenizer")
	}
	tn, tok, err := sc.resolveTokenizer(tokName, nil)
	if err != nil {
		return nil, filterError("Custom Analyzer [" + name + "] failed to find tokenizer under name [" + tokName + "]")
	}
	c.tokName, c.tok = tn, tok
	for _, cfName := range filterNames(p["char_filter"]) {
		cf, err := sc.resolveCharFilter(analyzeComponent{name: cfName})
		if err != nil {
			return nil, filterError("Custom Analyzer [" + name + "] failed to find char_filter under name [" + cfName + "]")
		}
		c.chars = append(c.chars, cf)
	}
	for _, fName := range filterNames(p["filter"]) {
		f, err := sc.resolveFilter(analyzeComponent{name: fName})
		if err != nil {
			return nil, filterError("Custom Analyzer [" + name + "] failed to find filter under name [" + fName + "]")
		}
		c.filters = append(c.filters, f)
	}
	return c, nil
}

// indexHasAnalyzer reports whether the analyzer is configured on the index.
func (sc analysisScope) indexHasAnalyzer(name string) bool {
	return sc.componentDefined("analyzer", name)
}

// analyzerChain resolves an analyzer name in this scope.
func (sc analysisScope) analyzerChain(name string) (*analysisChain, *apiError) {
	if sc.idx != nil && sc.indexHasAnalyzer(name) {
		return sc.indexAnalyzerChain(name)
	}
	if sc.idx != nil && (name == "default" || name == "default_search") {
		return sc.defaultIndexChain(), nil
	}
	c := prebuiltChain(name)
	if c == nil {
		if sc.global() {
			return nil, analyzeRequestError("illegal_argument_exception", "failed to find global analyzer ["+name+"]")
		}
		return nil, analyzeRequestError("illegal_argument_exception", "failed to find analyzer ["+name+"]")
	}
	if sc.idx != nil {
		c.gap = 100
	}
	return c, nil
}

// defaultIndexChain is the index default analyzer (analysis.analyzer.default
// or standard).
func (sc analysisScope) defaultIndexChain() *analysisChain {
	if sc.idx != nil && sc.indexHasAnalyzer("default") {
		if c, err := sc.indexAnalyzerChain("default"); err == nil {
			return c
		}
	}
	c := prebuiltChain("standard")
	if sc.idx != nil {
		c.gap = 100
	}
	return c
}

// normalizerFilterAllowed lists the filters normalizers may use.
var normalizerFilterAllowed = map[string]bool{
	"lowercase": true, "uppercase": true, "asciifolding": true, "cjk_width": true,
	"decimal_digit": true, "elision": true, "german_normalization": true,
	"hindi_normalization": true, "indic_normalization": true, "persian_normalization": true,
	"scandinavian_folding": true, "scandinavian_normalization": true,
	"serbian_normalization": true, "sorani_normalization": true, "arabic_normalization": true,
	"bengali_normalization": true, "trim": true, "truncate": true, "greek_lowercase": true,
	"irish_lowercase": true, "turkish_lowercase": true, "pattern_replace": true,
	"apostrophe": true, "classic": true, "reverse": true, "limit": true, "unique": true,
	"remove_duplicates": true, "length": true, "russian_lowercase": true,
}

var normalizerCharFilterAllowed = map[string]bool{"mapping": true, "pattern_replace": true}

// normalizerChain resolves a normalizer (keyword tokenizer plus filters).
func (sc analysisScope) normalizerChain(name string) (*analysisChain, *apiError) {
	kw, _ := buildTokenizer("keyword", nil, 1)
	c := &analysisChain{name: name, custom: true, tokName: "keyword", tok: kw}
	if sc.idx != nil && sc.componentDefined("normalizer", name) {
		p := sc.componentParams("normalizer", name)
		for _, cfName := range filterNames(p["char_filter"]) {
			cf, err := sc.resolveCharFilter(analyzeComponent{name: cfName})
			if err != nil {
				return nil, err
			}
			c.chars = append(c.chars, cf)
		}
		for _, fName := range filterNames(p["filter"]) {
			f, err := sc.resolveFilter(analyzeComponent{name: fName})
			if err != nil {
				return nil, err
			}
			c.filters = append(c.filters, f)
		}
		return c, nil
	}
	if builtinNormalizer(name) {
		c.filters = []chainFilter{{name: name, spec: filterSpec{typ: name}}}
		return c, nil
	}
	return nil, analyzeRequestError("illegal_argument_exception", "failed to find normalizer under ["+name+"]")
}

// ---------------------------------------------------------------- settings validation

func analysisNames(settings map[string]any, kind string) []string {
	seen := map[string]bool{}
	for _, prefix := range []string{"index.analysis." + kind + ".", "analysis." + kind + "."} {
		for k := range settings {
			if !strings.HasPrefix(k, prefix) {
				continue
			}
			rest := k[len(prefix):]
			if i := strings.Index(rest, "."); i > 0 {
				seen[rest[:i]] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func plainArgError(reason string) *apiError {
	return &apiError{Type: "illegal_argument_exception", Status: 400, Reason: reason}
}

// validateAnalysisSettings builds every analysis component configured in the
// (flattened) index settings and reports the first error OpenSearch would.
func validateAnalysisSettings(settings map[string]any) *apiError {
	sc := analysisScope{idx: &Index{Settings: settings}}
	for _, name := range analysisNames(settings, "char_filter") {
		typ, ok := sc.componentType("char_filter", name)
		if !ok {
			return plainArgError("char_filter [" + name + "] must specify either an analyzer type, or a tokenizer")
		}
		if !charFilterTypeKnown[typ] {
			return plainArgError("Unknown char_filter type [" + typ + "] for [" + name + "]")
		}
		if err := validateChainCharFilter(chainCharFilter{name: name, typ: typ, params: sc.componentParams("char_filter", name)}); err != nil {
			return plainArgError(err.Reason)
		}
	}
	for _, name := range analysisNames(settings, "tokenizer") {
		typ, ok := sc.componentType("tokenizer", name)
		if !ok {
			return plainArgError("tokenizer [" + name + "] must specify either an analyzer type, or a tokenizer")
		}
		if !tokenizerTypeKnown[typ] {
			return plainArgError("Unknown tokenizer type [" + typ + "] for [" + name + "]")
		}
		if _, err := buildTokenizer(typ, sc.componentParams("tokenizer", name), sc.maxNgramDiff()); err != nil {
			return plainArgError(err.Reason)
		}
	}
	for _, name := range analysisNames(settings, "filter") {
		typ, ok := sc.componentType("filter", name)
		if !ok {
			return plainArgError("filter [" + name + "] must specify either an analyzer type, or a tokenizer")
		}
		if !knownFilterTypes[typ] {
			return plainArgError("Unknown filter type [" + typ + "] for [" + name + "]")
		}
		if typ == "synonym" || typ == "synonym_graph" {
			continue
		}
		f := chainFilter{name: name, spec: filterSpec{typ: typ, params: sc.withLimits(sc.componentParams("filter", name))}}
		if err := validateChainFilter(sc.idx, f); err != nil && err.Status == 400 {
			return plainArgError(err.Reason)
		}
	}
	analyzers := analysisNames(settings, "analyzer")
	for _, name := range analyzers {
		p := sc.componentParams("analyzer", name)
		typ, _ := p["type"].(string)
		if typ == "" {
			if _, ok := p["tokenizer"]; !ok {
				return plainArgError("analyzer [" + name + "] must specify either an analyzer type, or a tokenizer")
			}
			continue
		}
		if !analyzerTypeValid(typ) {
			return plainArgError("Unknown analyzer type [" + typ + "] for [" + name + "]")
		}
	}
	var failed []string
	var suppressed []*apiError
	for _, name := range analyzers {
		c, err := sc.indexAnalyzerChain(name)
		if err == nil && c != nil {
			for _, f := range c.filters {
				if ferr := validateChainFilter(sc.idx, f); ferr != nil {
					err = ferr
					break
				}
			}
		}
		if err != nil {
			failed = append(failed, name)
			suppressed = append(suppressed, &apiError{Type: "illegal_argument_exception", Reason: "[" + name + "] " + err.Reason,
				CausedBy: &apiError{Type: "illegal_argument_exception", Reason: err.Reason}})
		}
	}
	if len(failed) > 0 {
		return &apiError{Type: "illegal_argument_exception", Status: 400,
			Reason: "Failed to build analyzers: [" + strings.Join(failed, ", ") + "]", Suppressed: suppressed}
	}
	for _, name := range analysisNames(settings, "normalizer") {
		p := sc.componentParams("normalizer", name)
		for _, cfName := range filterNames(p["char_filter"]) {
			cf, err := sc.resolveCharFilter(analyzeComponent{name: cfName})
			if err != nil {
				return plainArgError("Custom normalizer [" + name + "] failed to find char_filter under name [" + cfName + "]")
			}
			if !normalizerCharFilterAllowed[cf.typ] {
				return plainArgError("Custom normalizer [" + name + "] may not use char filter [" + cfName + "]")
			}
		}
		for _, fName := range filterNames(p["filter"]) {
			f, err := sc.resolveFilter(analyzeComponent{name: fName})
			if err != nil {
				return plainArgError("Custom normalizer [" + name + "] failed to find filter under name [" + fName + "]")
			}
			if !normalizerFilterAllowed[f.spec.typ] {
				return plainArgError("Custom normalizer [" + name + "] may not use filter [" + fName + "]")
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------- index helpers

// indexAnalyzerTokens analyzes text with an analyzer configured on the
// index; ok is false when the index does not configure it.
func indexAnalyzerTokens(idx *Index, analyzer, text string) ([]analyzedToken, bool) {
	sc := analysisScope{idx: idx}
	if idx == nil || analyzer == "" || !sc.indexHasAnalyzer(analyzer) {
		return nil, false
	}
	c, err := sc.indexAnalyzerChain(analyzer)
	if err != nil {
		return nil, false
	}
	r, rerr := c.run(idx, text, false)
	if rerr != nil {
		return nil, false
	}
	return r.toks, true
}

// tokenBytes formats a term like Lucene's BytesRef.toString().
func tokenBytes(term string) string {
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < len(term); i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strconv.FormatUint(uint64(term[i]), 16))
	}
	b.WriteByte(']')
	return b.String()
}
