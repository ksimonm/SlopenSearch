package analysis

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"slopensearch/internal/apierr"
)

// Analysis chains: an analyzer resolved into char filters, a tokenizer and
// token filters, from the global registry, the index settings or inline
// definitions of an _analyze request. Running a chain mirrors Lucene's token
// stream contract closely enough for _analyze (including explain), indexing
// and query analysis.

type chainCharFilter struct {
	Name   string
	typ    string
	params map[string]any
}

type chainFilter struct {
	Name string
	Spec filterSpec
}

type Chain struct {
	Name          string
	Custom        bool   // reported as custom_analyzer in explain output
	builtin       string // prebuilt analyzer evaluated by globalAnalyzerTokens
	Chars         []chainCharFilter
	TokenizerName string
	tok           tokenizerFunc
	Filters       []chainFilter
	Gap           int
	// KeywordAttr reports whether a built-in analyzer carries the keyword
	// attribute (stemming language analyzers).
	KeywordAttr bool
	// params are the analyzer settings of a built-in language analyzer.
	params map[string]any
}

// chainRun is the result of analyzing one text.
type chainRun struct {
	Tokens      []Token
	PosCount    int // positions consumed, trailing holes included
	FinalOffset int
	CharTexts   []string
	Stages      [][]Token // tokenizer output, then after each filter
	StageCounts []int
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

func FilterHasKeywordAttr(typ string) bool {
	return keywordAttrTypes[typ] || strings.HasSuffix(typ, "_stem")
}

func lastPosition(toks []Token) int {
	last := -1
	for _, t := range toks {
		if t.Pos > last {
			last = t.Pos
		}
	}
	return last
}

func copyTokens(toks []Token) []Token {
	return append([]Token(nil), toks...)
}

// Run analyzes one text. With stages set, every intermediate stream is kept.
func (c *Chain) Run(idx *Index, text string, stages bool) (chainRun, *apierr.Error) {
	res := chainRun{FinalOffset: UTF16Offset(text)}
	if c.builtin != "" && isLanguageAnalyzer(c.builtin) {
		toks, _ := languageAnalyzerTokens(c.builtin, text, c.params)
		res.Tokens = toks
		res.PosCount = languageAnalyzerPositions(c.builtin, text, toks)
		return res, nil
	}
	if c.builtin != "" {
		toks, ok := GlobalAnalyzerTokens(c.builtin, text)
		if !ok {
			toks = tokenizePositions(text, false)
		}
		res.Tokens = toks
		res.PosCount = len(tokenizePositions(text, false))
		if n := lastPosition(toks) + 1; n > res.PosCount {
			res.PosCount = n
		}
		return res, nil
	}
	cur := text
	var corrections []func(int) int
	for _, cf := range c.Chars {
		out, corr, err := applyCharFilter(cf, cur)
		if err != nil {
			return res, err
		}
		cur = out
		corrections = append(corrections, corr)
		if stages {
			res.CharTexts = append(res.CharTexts, out)
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
			toks[i].Start = correct(toks[i].Start)
			toks[i].End = correct(toks[i].End)
		}
		res.FinalOffset = correct(UTF16Offset(cur))
	}
	posCount := lastPosition(toks) + 1
	if stages {
		res.Stages = append(res.Stages, copyTokens(toks))
		res.StageCounts = append(res.StageCounts, posCount)
	}
	for fi, f := range c.Filters {
		spec := f.Spec
		spec.finalOffset = res.FinalOffset
		if spec.Type == "synonym" || spec.Type == "synonym_graph" {
			spec.ruleAnalyzer = c.prefixAnalyzer(idx, fi)
		}
		before := lastPosition(toks)
		out, err := applyFilterOne(idx, copyTokens(toks), spec)
		if err != nil {
			return res, err
		}
		if !filteringTypes[spec.Type] && len(toks) > 0 {
			posCount += lastPosition(out) - before
		}
		toks = out
		if stages {
			res.Stages = append(res.Stages, copyTokens(toks))
			res.StageCounts = append(res.StageCounts, posCount)
		}
	}
	res.Tokens = toks
	res.PosCount = posCount
	return res, nil
}

// prefixAnalyzer analyzes synonym rules with the tokenizer and the filters
// preceding filter i, as OpenSearch does.
func (c *Chain) prefixAnalyzer(idx *Index, i int) func(string) []Token {
	return func(s string) []Token {
		sub := &Chain{tok: c.tok, Filters: c.Filters[:i]}
		if sub.tok == nil {
			sub.tok, _ = buildTokenizer("whitespace", nil, 1)
		}
		r, err := sub.Run(idx, s, false)
		if err != nil {
			return nil
		}
		return r.Tokens
	}
}

// ---------------------------------------------------------------- resolving

type Scope struct {
	Index *Index
}

func (sc Scope) Global() bool { return sc.Index == nil }

func (sc Scope) setting(key string) (any, bool) {
	if sc.Index == nil {
		return nil, false
	}
	return findSetting(sc.Index.Settings, key)
}

func (sc Scope) maxNgramDiff() int {
	if sc.Index != nil {
		if n, ok := settingInt(sc.Index.Settings, "max_ngram_diff"); ok {
			return n
		}
	}
	return 1
}

func (sc Scope) maxShingleDiff() int {
	if sc.Index != nil {
		if n, ok := settingInt(sc.Index.Settings, "max_shingle_diff"); ok {
			return n
		}
	}
	return 3
}

func (sc Scope) componentType(kind, name string) (string, bool) {
	raw, ok := sc.setting("analysis." + kind + "." + name + ".type")
	if !ok {
		return "", false
	}
	s, _ := raw.(string)
	return s, true
}

func (sc Scope) componentDefined(kind, name string) bool {
	if sc.Index == nil {
		return false
	}
	return len(subSettings(sc.Index.Settings, "analysis."+kind+"."+name)) > 0
}

func (sc Scope) componentParams(kind, name string) map[string]any {
	if sc.Index == nil {
		return nil
	}
	return subSettings(sc.Index.Settings, "analysis."+kind+"."+name)
}

func notFound(sc Scope, kind, name string) *apierr.Error {
	if sc.Global() {
		return RequestError("illegal_argument_exception", "failed to find global "+kind+" under ["+name+"]")
	}
	return RequestError("illegal_argument_exception", "failed to find "+kind+" under ["+name+"]")
}

func AnonymousJSON(def map[string]any) string {
	b, _ := json.Marshal(def)
	return string(b)
}

// resolveTokenizer resolves a tokenizer name or inline definition.
func (sc Scope) resolveTokenizer(name string, inline map[string]any) (string, tokenizerFunc, *apierr.Error) {
	if inline != nil {
		typ, _ := inline["type"].(string)
		if typ == "" {
			return "", nil, RequestError("illegal_argument_exception", "Missing [type] setting for anonymous tokenizer: "+AnonymousJSON(inline))
		}
		if !tokenizerTypeKnown[typ] {
			return "", nil, RequestError("illegal_argument_exception", "failed to find global tokenizer under ["+typ+"]")
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

func (sc Scope) withLimits(p map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range p {
		out[k] = v
	}
	out["_max_ngram_diff"] = sc.maxNgramDiff()
	out["_max_shingle_diff"] = sc.maxShingleDiff()
	return out
}

// resolveFilter resolves a token filter reference.
func (sc Scope) resolveFilter(c Component) (chainFilter, *apierr.Error) {
	if c.Inline != nil {
		typ, _ := c.Inline["type"].(string)
		if !knownFilterTypes[typ] {
			return chainFilter{}, RequestError("illegal_argument_exception", "failed to find global filter under ["+typ+"]")
		}
		return chainFilter{Name: "__anonymous__" + typ, Spec: filterSpec{Type: typ, Params: sc.withLimits(c.Inline)}}, nil
	}
	if typ, ok := sc.componentType("filter", c.name); ok {
		return chainFilter{Name: c.name, Spec: filterSpec{Type: typ, Params: sc.withLimits(sc.componentParams("filter", c.name))}}, nil
	}
	if spec, ok := globalFilterSpec(c.name); ok {
		spec.Params = sc.withLimits(spec.Params)
		return chainFilter{Name: c.name, Spec: spec}, nil
	}
	return chainFilter{}, notFound(sc, "filter", c.name)
}

var charFilterTypeKnown = map[string]bool{"html_strip": true, "mapping": true, "pattern_replace": true}

func (sc Scope) resolveCharFilter(c Component) (chainCharFilter, *apierr.Error) {
	if c.Inline != nil {
		typ, _ := c.Inline["type"].(string)
		if !charFilterTypeKnown[typ] {
			return chainCharFilter{}, RequestError("illegal_argument_exception", "failed to find global char_filter under ["+typ+"]")
		}
		return chainCharFilter{Name: "__anonymous__" + typ, typ: typ, params: c.Inline}, nil
	}
	if typ, ok := sc.componentType("char_filter", c.name); ok {
		return chainCharFilter{Name: c.name, typ: typ, params: sc.componentParams("char_filter", c.name)}, nil
	}
	if charFilterTypeKnown[c.name] {
		return chainCharFilter{Name: c.name, typ: c.name}, nil
	}
	return chainCharFilter{}, notFound(sc, "char_filter", c.name)
}

// validateChainFilter applies a filter once so configuration errors surface when
// the chain is built.
func validateChainFilter(idx *Index, f chainFilter) *apierr.Error {
	dummy := []Token{{Term: "x", Start: 0, End: 1, Type: "word"}}
	_, err := applyFilterOne(idx, dummy, f.Spec)
	return err
}

func validateChainCharFilter(cf chainCharFilter) *apierr.Error {
	_, _, err := applyCharFilter(cf, "")
	return err
}

// BuildRequestChain builds the custom chain of an _analyze request.
func (sc Scope) BuildRequestChain(tokName string, tokInline map[string]any, chars, filters []Component) (*Chain, *apierr.Error) {
	c := &Chain{Custom: true, Gap: 100}
	name, tok, err := sc.resolveTokenizer(tokName, tokInline)
	if err != nil {
		return nil, err
	}
	c.TokenizerName, c.tok = name, tok
	for _, comp := range chars {
		cf, err := sc.resolveCharFilter(comp)
		if err != nil {
			return nil, err
		}
		if err := validateChainCharFilter(cf); err != nil {
			return nil, err
		}
		c.Chars = append(c.Chars, cf)
	}
	for _, comp := range filters {
		f, err := sc.resolveFilter(comp)
		if err != nil {
			return nil, err
		}
		if err := validateChainFilter(sc.Index, f); err != nil {
			return nil, err
		}
		c.Filters = append(c.Filters, f)
	}
	return c, nil
}

// PrebuiltChain returns the chain of a prebuilt analyzer, or nil.
func PrebuiltChain(name string) *Chain {
	c, err := typedAnalyzerChain(name, name, nil)
	if err != nil || c == nil {
		if BuiltinAnalyzer(name) {
			return &Chain{Name: name, builtin: name, KeywordAttr: languageAnalyzerHasKeyword(name)}
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
	return BuiltinAnalyzer(name)
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
	return filterSpec{Type: "stop", Params: params}
}

// typedAnalyzerChain builds the analyzers OpenSearch implements as fixed
// Lucene chains (standard, simple, stop, ...) with their parameters. It
// returns nil for types handled elsewhere (language analyzers).
func typedAnalyzerChain(name, typ string, p map[string]any) (*Chain, *apierr.Error) {
	c := &Chain{Name: name}
	tok := func(t string, params map[string]any) *apierr.Error {
		f, err := buildTokenizer(t, params, 1)
		if err != nil {
			return err
		}
		c.TokenizerName, c.tok = t, f
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
		c.Filters = []chainFilter{{Name: "lowercase", Spec: filterSpec{Type: "lowercase"}}, {Name: "stop", Spec: stopFilterSpec(p, "_none_")}}
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
		c.Filters = []chainFilter{{Name: "stop", Spec: stopFilterSpec(p, "_english_")}}
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
		if ParamBool(p, "lowercase", true) {
			c.Filters = append(c.Filters, chainFilter{Name: "lowercase", Spec: filterSpec{Type: "lowercase"}})
		}
		c.Filters = append(c.Filters, chainFilter{Name: "stop", Spec: stopFilterSpec(p, "_none_")})
	case "fingerprint":
		if err := tok("standard", nil); err != nil {
			return nil, err
		}
		fp := map[string]any{"separator": fsString(p, "separator", " ")}
		if v, ok := p["max_output_size"]; ok {
			fp["max_output_size"] = v
		}
		c.Filters = []chainFilter{
			{Name: "lowercase", Spec: filterSpec{Type: "lowercase"}},
			{Name: "asciifolding", Spec: filterSpec{Type: "asciifolding"}},
			{Name: "stop", Spec: stopFilterSpec(p, "_none_")},
			{Name: "fingerprint", Spec: filterSpec{Type: "fingerprint", Params: fp}},
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
		c.Filters = []chainFilter{
			{Name: "lowercase", Spec: filterSpec{Type: "lowercase"}},
			{Name: "stop", Spec: stopFilterSpec(p, def)},
			{Name: "snowball", Spec: filterSpec{Type: "snowball", Params: map[string]any{"language": lang}}},
		}
		c.KeywordAttr = true
	case "standard_html_strip":
		if err := tok("standard", maxLen); err != nil {
			return nil, err
		}
		c.Chars = []chainCharFilter{{Name: "html_strip", typ: "html_strip"}}
		c.Filters = []chainFilter{{Name: "lowercase", Spec: filterSpec{Type: "lowercase"}}, {Name: "stop", Spec: stopFilterSpec(p, "_none_")}}
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
	return analyzerTypeKnown[typ] || (BuiltinAnalyzer(typ) && typ != "lowercase")
}

// IndexAnalyzerChain builds an analyzer configured in the index settings.
// The returned error is the bare message used inside "Failed to build
// analyzers" reports.
func (sc Scope) IndexAnalyzerChain(name string) (*Chain, *apierr.Error) {
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
			c = &Chain{Name: name, builtin: typ, KeywordAttr: languageAnalyzerHasKeyword(typ), params: p}
		}
		c.Gap = gap
		return c, nil
	}
	c := &Chain{Name: name, Custom: true, Gap: gap}
	if !hasTok {
		return nil, filterError("Custom Analyzer [" + name + "] must be configured with a tokenizer")
	}
	tn, tok, err := sc.resolveTokenizer(tokName, nil)
	if err != nil {
		return nil, filterError("Custom Analyzer [" + name + "] failed to find tokenizer under name [" + tokName + "]")
	}
	c.TokenizerName, c.tok = tn, tok
	for _, cfName := range FilterNames(p["char_filter"]) {
		cf, err := sc.resolveCharFilter(Component{name: cfName})
		if err != nil {
			return nil, filterError("Custom Analyzer [" + name + "] failed to find char_filter under name [" + cfName + "]")
		}
		c.Chars = append(c.Chars, cf)
	}
	for _, fName := range FilterNames(p["filter"]) {
		f, err := sc.resolveFilter(Component{name: fName})
		if err != nil {
			return nil, filterError("Custom Analyzer [" + name + "] failed to find filter under name [" + fName + "]")
		}
		c.Filters = append(c.Filters, f)
	}
	return c, nil
}

// IndexHasAnalyzer reports whether the analyzer is configured on the index.
func (sc Scope) IndexHasAnalyzer(name string) bool {
	return sc.componentDefined("analyzer", name)
}

// AnalyzerChain resolves an analyzer name in this scope.
func (sc Scope) AnalyzerChain(name string) (*Chain, *apierr.Error) {
	if sc.Index != nil && sc.IndexHasAnalyzer(name) {
		return sc.IndexAnalyzerChain(name)
	}
	if sc.Index != nil && (name == "default" || name == "default_search") {
		return sc.DefaultIndexChain(), nil
	}
	c := PrebuiltChain(name)
	if c == nil {
		if sc.Global() {
			return nil, RequestError("illegal_argument_exception", "failed to find global analyzer ["+name+"]")
		}
		return nil, RequestError("illegal_argument_exception", "failed to find analyzer ["+name+"]")
	}
	if sc.Index != nil {
		c.Gap = 100
	}
	return c, nil
}

// DefaultIndexChain is the index default analyzer (analysis.analyzer.default
// or standard).
func (sc Scope) DefaultIndexChain() *Chain {
	if sc.Index != nil && sc.IndexHasAnalyzer("default") {
		if c, err := sc.IndexAnalyzerChain("default"); err == nil {
			return c
		}
	}
	c := PrebuiltChain("standard")
	if sc.Index != nil {
		c.Gap = 100
	}
	return c
}

// NormalizerFilterAllowed lists the filters normalizers may use.
var NormalizerFilterAllowed = map[string]bool{
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

// NormalizerChain resolves a normalizer (keyword tokenizer plus filters).
func (sc Scope) NormalizerChain(name string) (*Chain, *apierr.Error) {
	kw, _ := buildTokenizer("keyword", nil, 1)
	c := &Chain{Name: name, Custom: true, TokenizerName: "keyword", tok: kw}
	if sc.Index != nil && sc.componentDefined("normalizer", name) {
		p := sc.componentParams("normalizer", name)
		for _, cfName := range FilterNames(p["char_filter"]) {
			cf, err := sc.resolveCharFilter(Component{name: cfName})
			if err != nil {
				return nil, err
			}
			c.Chars = append(c.Chars, cf)
		}
		for _, fName := range FilterNames(p["filter"]) {
			f, err := sc.resolveFilter(Component{name: fName})
			if err != nil {
				return nil, err
			}
			c.Filters = append(c.Filters, f)
		}
		return c, nil
	}
	if BuiltinNormalizer(name) {
		c.Filters = []chainFilter{{Name: name, Spec: filterSpec{Type: name}}}
		return c, nil
	}
	return nil, RequestError("illegal_argument_exception", "failed to find normalizer under ["+name+"]")
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

func plainArgError(reason string) *apierr.Error {
	return &apierr.Error{Type: "illegal_argument_exception", Status: 400, Reason: reason}
}

// ValidateSettings builds every analysis component configured in the
// (flattened) index settings and reports the first error OpenSearch would.
func ValidateSettings(settings map[string]any) *apierr.Error {
	sc := Scope{Index: &Index{Settings: settings}}
	for _, name := range analysisNames(settings, "char_filter") {
		typ, ok := sc.componentType("char_filter", name)
		if !ok {
			return plainArgError("char_filter [" + name + "] must specify either an analyzer type, or a tokenizer")
		}
		if !charFilterTypeKnown[typ] {
			return plainArgError("Unknown char_filter type [" + typ + "] for [" + name + "]")
		}
		if err := validateChainCharFilter(chainCharFilter{Name: name, typ: typ, params: sc.componentParams("char_filter", name)}); err != nil {
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
		f := chainFilter{Name: name, Spec: filterSpec{Type: typ, Params: sc.withLimits(sc.componentParams("filter", name))}}
		if err := validateChainFilter(sc.Index, f); err != nil && err.Status == 400 {
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
	var suppressed []*apierr.Error
	for _, name := range analyzers {
		c, err := sc.IndexAnalyzerChain(name)
		if err == nil && c != nil {
			for _, f := range c.Filters {
				if ferr := validateChainFilter(sc.Index, f); ferr != nil {
					err = ferr
					break
				}
			}
		}
		if err != nil {
			failed = append(failed, name)
			suppressed = append(suppressed, &apierr.Error{Type: "illegal_argument_exception", Reason: "[" + name + "] " + err.Reason,
				CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: err.Reason}})
		}
	}
	if len(failed) > 0 {
		return &apierr.Error{Type: "illegal_argument_exception", Status: 400,
			Reason: "Failed to build analyzers: [" + strings.Join(failed, ", ") + "]", Suppressed: suppressed}
	}
	for _, name := range analysisNames(settings, "normalizer") {
		p := sc.componentParams("normalizer", name)
		for _, cfName := range FilterNames(p["char_filter"]) {
			cf, err := sc.resolveCharFilter(Component{name: cfName})
			if err != nil {
				return plainArgError("Custom normalizer [" + name + "] failed to find char_filter under name [" + cfName + "]")
			}
			if !normalizerCharFilterAllowed[cf.typ] {
				return plainArgError("Custom normalizer [" + name + "] may not use char filter [" + cfName + "]")
			}
		}
		for _, fName := range FilterNames(p["filter"]) {
			f, err := sc.resolveFilter(Component{name: fName})
			if err != nil {
				return plainArgError("Custom normalizer [" + name + "] failed to find filter under name [" + fName + "]")
			}
			if !NormalizerFilterAllowed[f.Spec.Type] {
				return plainArgError("Custom normalizer [" + name + "] may not use filter [" + fName + "]")
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------- index helpers

// IndexAnalyzerTokens analyzes text with an analyzer configured on the
// index; ok is false when the index does not configure it.
func IndexAnalyzerTokens(idx *Index, analyzer, text string) ([]Token, bool) {
	sc := Scope{Index: idx}
	if idx == nil || analyzer == "" || !sc.IndexHasAnalyzer(analyzer) {
		return nil, false
	}
	c, err := sc.IndexAnalyzerChain(analyzer)
	if err != nil {
		return nil, false
	}
	r, rerr := c.Run(idx, text, false)
	if rerr != nil {
		return nil, false
	}
	return r.Tokens, true
}

// TokenBytes formats a term like Lucene's BytesRef.toString().
func TokenBytes(term string) string {
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

func BuiltinAnalyzer(name string) bool {
	switch name {
	case "standard", "simple", "whitespace", "keyword", "stop", "english",
		"lowercase", "pattern", "fingerprint",
		// Language analyzers.
		"arabic", "armenian", "basque", "bengali", "brazilian", "bulgarian",
		"catalan", "cjk", "czech", "danish", "dutch", "estonian", "finnish",
		"french", "galician", "german", "greek", "hindi", "hungarian",
		"indonesian", "irish", "italian", "latvian", "lithuanian",
		"norwegian", "persian", "portuguese", "romanian", "russian",
		"sorani", "spanish", "swedish", "thai", "turkish", "chinese":
		return true
	}
	return false
}

func BuiltinNormalizer(name string) bool {
	switch name {
	case "lowercase", "uppercase", "asciifolding":
		return true
	}
	return false
}
