package main

import "strings"

// The built-in language analyzers of OpenSearch 3.9, i.e. the Lucene 10.5.1
// analyzer classes (ArabicAnalyzer ... TurkishAnalyzer, CJKAnalyzer,
// ThaiAnalyzer) with the stopwords / stem_exclusion settings OpenSearch's
// *AnalyzerProvider classes read.

// langWordSet is a Lucene CharArraySet.
type langWordSet struct {
	words      map[string]bool
	ignoreCase bool
}

func newLangWordSet(words []string, ignoreCase bool) langWordSet {
	s := langWordSet{words: make(map[string]bool, len(words)), ignoreCase: ignoreCase}
	for _, w := range words {
		if ignoreCase {
			w = luceneLower(w)
		}
		s.words[w] = true
	}
	return s
}

func (s langWordSet) contains(term string) bool {
	if len(s.words) == 0 {
		return false
	}
	if s.ignoreCase {
		term = luceneLower(term)
	}
	return s.words[term]
}

// Token stream stages.

func langMap(toks []analyzedToken, f func(string) string) []analyzedToken {
	for i := range toks {
		toks[i].term = f(toks[i].term)
	}
	return toks
}

// langStem applies a stemmer to tokens not marked as keywords.
func langStem(toks []analyzedToken, f func(string) string) []analyzedToken {
	for i := range toks {
		if !toks[i].keyword {
			toks[i].term = f(toks[i].term)
		}
	}
	return toks
}

func langStop(toks []analyzedToken, set langWordSet) []analyzedToken {
	out := toks[:0]
	for _, t := range toks {
		if !set.contains(t.term) {
			out = append(out, t)
		}
	}
	return out
}

func langKeywords(toks []analyzedToken, set langWordSet) []analyzedToken {
	for i := range toks {
		if set.contains(toks[i].term) {
			toks[i].keyword = true
		}
	}
	return toks
}

// luceneElision ports ElisionFilter: strip a leading article before the
// first apostrophe (' or U+2019).
func luceneElision(term string, articles langWordSet) string {
	idx := strings.IndexAny(term, "'’")
	if idx < 0 {
		return term
	}
	if articles.contains(term[:idx]) {
		_, w := firstRune(term[idx:])
		return term[idx+w:]
	}
	return term
}

func firstRune(s string) (rune, int) {
	for _, r := range s {
		return r, len(string(r))
	}
	return 0, 0
}

var (
	frenchArticles  = newLangWordSet([]string{"l", "m", "t", "qu", "n", "s", "j", "d", "c", "jusqu", "quoiqu", "lorsqu", "puisqu"}, true)
	catalanArticles = newLangWordSet([]string{"d", "l", "m", "n", "s", "t"}, true)
	italianArticles = newLangWordSet([]string{"c", "l", "all", "dall", "dell", "nell", "sull", "coll", "pell", "gl", "agl",
		"dagl", "degl", "negl", "sugl", "un", "m", "t", "s", "v", "d"}, true)
	irishArticles     = newLangWordSet([]string{"d", "m", "b"}, true)
	irishHyphenations = newLangWordSet([]string{"h", "n", "t"}, true)
	dutchStemOverride = map[string]string{"fiets": "fiets", "bromfiets": "bromfiets", "ei": "eier", "kind": "kinder"}
)

// languageAnalyzerNames lists the language analyzers this server implements.
var languageAnalyzerNames = map[string]bool{
	"arabic": true, "armenian": true, "basque": true, "bengali": true, "brazilian": true,
	"bulgarian": true, "catalan": true, "cjk": true, "czech": true, "danish": true, "dutch": true,
	"english": true, "estonian": true, "finnish": true, "french": true, "galician": true,
	"german": true, "greek": true, "hindi": true, "hungarian": true, "indonesian": true,
	"irish": true, "italian": true, "latvian": true, "lithuanian": true, "norwegian": true,
	"persian": true, "portuguese": true, "romanian": true, "russian": true, "sorani": true,
	"spanish": true, "swedish": true, "turkish": true, "thai": true, "chinese": true,
}

func isLanguageAnalyzer(name string) bool { return languageAnalyzerNames[name] }

// langDefaultStop is the default stopword set of a language analyzer.
func langDefaultStop(name string) []string {
	switch name {
	case "english", "chinese":
		out := make([]string, 0, len(defaultEnglishStop))
		for w := range defaultEnglishStop {
			out = append(out, w)
		}
		return out
	}
	return luceneStopwords[name]
}

// langStopSetting resolves the "stopwords" setting (Analysis.parseStopWords).
func langStopSetting(name string, params map[string]any) langWordSet {
	ignoreCase := fsBool(params, "stopwords_case", false)
	raw, ok := params["stopwords"]
	if !ok || raw == nil {
		return newLangWordSet(langDefaultStop(name), false)
	}
	if s, isStr := raw.(string); isStr && s == "_none_" {
		return langWordSet{}
	}
	var words []string
	for _, w := range fsStrings(params, "stopwords") {
		if w == "_english_" {
			for k := range defaultEnglishStop {
				words = append(words, k)
			}
			continue
		}
		if len(w) > 2 && w[0] == '_' && w[len(w)-1] == '_' && namedStopLanguages[w[1:len(w)-1]] {
			words = append(words, luceneStopwords[w[1:len(w)-1]]...)
			continue
		}
		words = append(words, w)
	}
	return newLangWordSet(words, ignoreCase)
}

// langStemExclusion resolves "stem_exclusion" (Analysis.parseStemExclusion).
func langStemExclusion(params map[string]any) langWordSet {
	raw, ok := params["stem_exclusion"]
	if !ok || raw == nil {
		return langWordSet{}
	}
	if s, isStr := raw.(string); isStr && s == "_none_" {
		return langWordSet{}
	}
	return newLangWordSet(fsStrings(params, "stem_exclusion"), false)
}

func snowballStem(name string) func(string) string {
	f, _ := snowballStemFunc(name)
	return f
}

// languageAnalyzerTokens runs a language analyzer over text. params holds the
// analyzer settings (nil for the pre-built analyzers).
func languageAnalyzerTokens(name, text string, params map[string]any) ([]analyzedToken, bool) {
	if !languageAnalyzerNames[name] {
		return nil, false
	}
	stop := langStopSetting(name, params)
	excl := langStemExclusion(params)
	std := func() []analyzedToken { return tokenizePositionsCase(text, false, false, 255) }
	// The common shape: standard, lowercase, stop, keyword marker, stemmer.
	simple := func(stem func(string) string) []analyzedToken {
		t := langMap(std(), luceneLower)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		return langStem(t, stem)
	}
	switch name {
	case "arabic":
		t := langMap(std(), luceneLower)
		t = langMap(t, decimalDigitLucene)
		t = langStop(t, stop)
		t = langMap(t, arabicNormalizeLucene)
		t = langKeywords(t, excl)
		return langStem(t, arabicStemLucene), true
	case "armenian", "basque", "danish", "estonian", "finnish", "hungarian", "lithuanian",
		"norwegian", "russian", "swedish", "catalan":
		if name == "catalan" {
			t := langMap(std(), func(s string) string { return luceneElision(s, catalanArticles) })
			t = langMap(t, luceneLower)
			t = langStop(t, stop)
			t = langKeywords(t, excl)
			return langStem(t, snowballStem("Catalan")), true
		}
		return simple(snowballStem(capitalizeFirst(name))), true
	case "bengali", "hindi":
		t := langMap(std(), luceneLower)
		t = langMap(t, decimalDigitLucene)
		t = langKeywords(t, excl)
		t = langMap(t, indicNormalizeLucene)
		if name == "bengali" {
			t = langMap(t, bengaliNormalizeLucene)
			t = langStop(t, stop)
			return langStem(t, bengaliStemLucene), true
		}
		t = langMap(t, hindiNormalizeLucene)
		t = langStop(t, stop)
		return langStem(t, hindiStemLucene), true
	case "brazilian":
		return simple(brazilianStem), true
	case "bulgarian":
		return simple(bulgarianStemLucene), true
	case "czech":
		return simple(czechStemLucene), true
	case "galician":
		return simple(galicianRSLPStem), true
	case "indonesian":
		return simple(func(s string) string { return indonesianStemLucene(s, true) }), true
	case "latvian":
		return simple(latvianStemLucene), true
	case "portuguese":
		return simple(portugueseLightStem), true
	case "spanish":
		return simple(spanishLightStem), true
	case "dutch":
		t := langMap(std(), luceneLower)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		for i := range t {
			if t[i].keyword {
				continue
			}
			if v, ok := dutchStemOverride[t[i].term]; ok {
				t[i].term = v
				t[i].keyword = true
			}
		}
		return langStem(t, snowballStem("Dutch")), true
	case "english":
		t := langMap(std(), englishPossessive)
		t = langMap(t, luceneLower)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		return langStem(t, porterStemLucene), true
	case "french", "italian":
		articles, stem := frenchArticles, frenchLightStem
		if name == "italian" {
			articles, stem = italianArticles, italianLightStem
		}
		t := langMap(std(), func(s string) string { return luceneElision(s, articles) })
		t = langMap(t, luceneLower)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		return langStem(t, stem), true
	case "german":
		t := langMap(std(), luceneLower)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		t = langMap(t, germanNormalizeLucene)
		return langStem(t, germanLightStem), true
	case "greek":
		t := langMap(std(), greekLowerLucene)
		t = langStop(t, stop)
		return langStem(t, greekStemLucene), true
	case "irish":
		t := langStop(std(), irishHyphenations)
		t = langMap(t, func(s string) string { return luceneElision(s, irishArticles) })
		t = langMap(t, irishLowerLucene)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		return langStem(t, snowballStem("Irish")), true
	case "persian":
		t := tokenizePositionsCase(persianCharFilter(text), false, false, 255)
		t = langMap(t, luceneLower)
		t = langMap(t, decimalDigitLucene)
		t = langMap(t, arabicNormalizeLucene)
		t = langMap(t, persianNormalizeLucene)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		return langStem(t, persianStemLucene), true
	case "romanian":
		t := langMap(std(), luceneLower)
		t = langStop(t, stop)
		t = langMap(t, romanianNormalize)
		t = langKeywords(t, excl)
		return langStem(t, snowballStem("Romanian")), true
	case "sorani":
		t := langMap(std(), soraniNormalizeLucene)
		t = langMap(t, luceneLower)
		t = langMap(t, decimalDigitLucene)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		return langStem(t, soraniStemLucene), true
	case "turkish":
		t := langMap(std(), apostropheLucene)
		t = langMap(t, turkishLowerLucene)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		return langStem(t, snowballStem("Turkish")), true
	case "cjk":
		t := langMap(std(), cjkWidthLucene)
		t = langMap(t, luceneLower)
		t = cjkBigramLucene(t, cjkBigramAll, false)
		return langStop(t, stop), true
	case "thai":
		t := thaiAnalyzedTokens(text)
		t = langMap(t, luceneLower)
		t = langMap(t, decimalDigitLucene)
		return langStop(t, stop), true
	case "chinese":
		// OpenSearch maps "chinese" to a StandardAnalyzer with English stopwords.
		t := langMap(std(), luceneLower)
		return langStop(t, stop), true
	}
	return nil, false
}

// languageAnalyzerPositions is the number of positions a language analyzer
// consumes for text, trailing stopword holes included.
func languageAnalyzerPositions(name, text string, toks []analyzedToken) int {
	n := lastPosition(toks) + 1
	switch name {
	case "cjk":
		return n
	case "thai":
		if m := len(thaiTokenize(text)); m > n {
			n = m
		}
		return n
	case "persian":
		text = persianCharFilter(text)
	}
	if m := len(tokenizePositionsCase(text, false, false, 255)); m > n {
		n = m
	}
	return n
}

// standardTokenizerTokens runs Lucene's StandardTokenizer (uax_standard.go),
// optionally lowercasing like the standard analyzer's LowerCaseFilter.
func standardTokenizerTokens(text string, lower bool, maxLen int) []analyzedToken {
	ut := uaxStandardTokens(text, maxLen)
	out := make([]analyzedToken, len(ut))
	pos := -1
	for i, t := range ut {
		pos += t.posInc
		term := t.term
		if lower {
			term = luceneLower(term)
		}
		out[i] = analyzedToken{term: term, start: t.start, end: t.end, pos: pos, typ: t.typ}
	}
	return out
}

// thaiAnalyzedTokens runs the Thai tokenizer.
func thaiAnalyzedTokens(text string) []analyzedToken {
	tt := thaiTokenize(text)
	out := make([]analyzedToken, len(tt))
	for i, t := range tt {
		out[i] = analyzedToken{term: t.term, start: t.start, end: t.end, pos: i, typ: "word"}
	}
	return out
}
