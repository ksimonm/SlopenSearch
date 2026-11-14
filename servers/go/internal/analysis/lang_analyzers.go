package analysis

import (
	"strings"

	"slopensearch/internal/lucene"
	"slopensearch/internal/snowball"
)

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
			w = lucene.Lower(w)
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
		term = lucene.Lower(term)
	}
	return s.words[term]
}

// Token stream stages.

func langMap(toks []Token, f func(string) string) []Token {
	for i := range toks {
		toks[i].Term = f(toks[i].Term)
	}
	return toks
}

// langStem applies a stemmer to tokens not marked as keywords.
func langStem(toks []Token, f func(string) string) []Token {
	for i := range toks {
		if !toks[i].Keyword {
			toks[i].Term = f(toks[i].Term)
		}
	}
	return toks
}

func langStop(toks []Token, set langWordSet) []Token {
	out := toks[:0]
	for _, t := range toks {
		if !set.contains(t.Term) {
			out = append(out, t)
		}
	}
	return out
}

func langKeywords(toks []Token, set langWordSet) []Token {
	for i := range toks {
		if set.contains(toks[i].Term) {
			toks[i].Keyword = true
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
	ignoreCase := ParamBool(params, "stopwords_case", false)
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
	f, _ := snowball.StemFunc(name)
	return f
}

// languageAnalyzerTokens runs a language analyzer over text. params holds the
// analyzer settings (nil for the pre-built analyzers).
func languageAnalyzerTokens(name, text string, params map[string]any) ([]Token, bool) {
	if !languageAnalyzerNames[name] {
		return nil, false
	}
	stop := langStopSetting(name, params)
	excl := langStemExclusion(params)
	std := func() []Token { return tokenizePositionsCase(text, false, false, 255) }
	// The common shape: standard, lowercase, stop, keyword marker, stemmer.
	simple := func(stem func(string) string) []Token {
		t := langMap(std(), lucene.Lower)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		return langStem(t, stem)
	}
	switch name {
	case "arabic":
		t := langMap(std(), lucene.Lower)
		t = langMap(t, lucene.DecimalDigit)
		t = langStop(t, stop)
		t = langMap(t, lucene.ArabicNormalize)
		t = langKeywords(t, excl)
		return langStem(t, lucene.ArabicStem), true
	case "armenian", "basque", "danish", "estonian", "finnish", "hungarian", "lithuanian",
		"norwegian", "russian", "swedish", "catalan":
		if name == "catalan" {
			t := langMap(std(), func(s string) string { return luceneElision(s, catalanArticles) })
			t = langMap(t, lucene.Lower)
			t = langStop(t, stop)
			t = langKeywords(t, excl)
			return langStem(t, snowballStem("Catalan")), true
		}
		return simple(snowballStem(capitalizeFirst(name))), true
	case "bengali", "hindi":
		t := langMap(std(), lucene.Lower)
		t = langMap(t, lucene.DecimalDigit)
		t = langKeywords(t, excl)
		t = langMap(t, lucene.IndicNormalize)
		if name == "bengali" {
			t = langMap(t, lucene.BengaliNormalize)
			t = langStop(t, stop)
			return langStem(t, lucene.BengaliStem), true
		}
		t = langMap(t, lucene.HindiNormalize)
		t = langStop(t, stop)
		return langStem(t, lucene.HindiStem), true
	case "brazilian":
		return simple(lucene.BrazilianStem), true
	case "bulgarian":
		return simple(lucene.BulgarianStem), true
	case "czech":
		return simple(lucene.CzechStem), true
	case "galician":
		return simple(lucene.GalicianRSLPStem), true
	case "indonesian":
		return simple(func(s string) string { return lucene.IndonesianStem(s, true) }), true
	case "latvian":
		return simple(lucene.LatvianStem), true
	case "portuguese":
		return simple(lucene.PortugueseLightStem), true
	case "spanish":
		return simple(lucene.SpanishLightStem), true
	case "dutch":
		t := langMap(std(), lucene.Lower)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		for i := range t {
			if t[i].Keyword {
				continue
			}
			if v, ok := dutchStemOverride[t[i].Term]; ok {
				t[i].Term = v
				t[i].Keyword = true
			}
		}
		return langStem(t, snowballStem("Dutch")), true
	case "english":
		t := langMap(std(), lucene.EnglishPossessive)
		t = langMap(t, lucene.Lower)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		return langStem(t, lucene.PorterStem), true
	case "french", "italian":
		articles, stem := frenchArticles, lucene.FrenchLightStem
		if name == "italian" {
			articles, stem = italianArticles, lucene.ItalianLightStem
		}
		t := langMap(std(), func(s string) string { return luceneElision(s, articles) })
		t = langMap(t, lucene.Lower)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		return langStem(t, stem), true
	case "german":
		t := langMap(std(), lucene.Lower)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		t = langMap(t, lucene.GermanNormalize)
		return langStem(t, lucene.GermanLightStem), true
	case "greek":
		t := langMap(std(), lucene.GreekLower)
		t = langStop(t, stop)
		return langStem(t, lucene.GreekStem), true
	case "irish":
		t := langStop(std(), irishHyphenations)
		t = langMap(t, func(s string) string { return luceneElision(s, irishArticles) })
		t = langMap(t, lucene.IrishLower)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		return langStem(t, snowballStem("Irish")), true
	case "persian":
		t := tokenizePositionsCase(lucene.PersianCharFilter(text), false, false, 255)
		t = langMap(t, lucene.Lower)
		t = langMap(t, lucene.DecimalDigit)
		t = langMap(t, lucene.ArabicNormalize)
		t = langMap(t, lucene.PersianNormalize)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		return langStem(t, lucene.PersianStem), true
	case "romanian":
		t := langMap(std(), lucene.Lower)
		t = langStop(t, stop)
		t = langMap(t, lucene.RomanianNormalize)
		t = langKeywords(t, excl)
		return langStem(t, snowballStem("Romanian")), true
	case "sorani":
		t := langMap(std(), lucene.SoraniNormalize)
		t = langMap(t, lucene.Lower)
		t = langMap(t, lucene.DecimalDigit)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		return langStem(t, lucene.SoraniStem), true
	case "turkish":
		t := langMap(std(), lucene.Apostrophe)
		t = langMap(t, lucene.TurkishLower)
		t = langStop(t, stop)
		t = langKeywords(t, excl)
		return langStem(t, snowballStem("Turkish")), true
	case "cjk":
		t := langMap(std(), lucene.CJKWidth)
		t = langMap(t, lucene.Lower)
		t = cjkBigramLucene(t, cjkBigramAll, false)
		return langStop(t, stop), true
	case "thai":
		t := thaiAnalyzedTokens(text)
		t = langMap(t, lucene.Lower)
		t = langMap(t, lucene.DecimalDigit)
		return langStop(t, stop), true
	case "chinese":
		// OpenSearch maps "chinese" to a StandardAnalyzer with English stopwords.
		t := langMap(std(), lucene.Lower)
		return langStop(t, stop), true
	}
	return nil, false
}

// languageAnalyzerPositions is the number of positions a language analyzer
// consumes for text, trailing stopword holes included.
func languageAnalyzerPositions(name, text string, toks []Token) int {
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
		text = lucene.PersianCharFilter(text)
	}
	if m := len(tokenizePositionsCase(text, false, false, 255)); m > n {
		n = m
	}
	return n
}

// standardTokenizerTokens runs Lucene's StandardTokenizer (uax_standard.go),
// optionally lowercasing like the standard analyzer's LowerCaseFilter.
func standardTokenizerTokens(text string, lower bool, maxLen int) []Token {
	ut := lucene.StandardTokens(text, maxLen)
	out := make([]Token, len(ut))
	pos := -1
	for i, t := range ut {
		pos += t.PosInc
		term := t.Term
		if lower {
			term = lucene.Lower(term)
		}
		out[i] = Token{Term: term, Start: t.Start, End: t.End, Pos: pos, Type: t.Type}
	}
	return out
}

// thaiAnalyzedTokens runs the Thai tokenizer.
func thaiAnalyzedTokens(text string) []Token {
	tt := thaiTokenize(text)
	out := make([]Token, len(tt))
	for i, t := range tt {
		out[i] = Token{Term: t.term, Start: t.start, End: t.end, Pos: i, Type: "word"}
	}
	return out
}
