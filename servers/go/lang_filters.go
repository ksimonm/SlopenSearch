package main

// Language-specific token filters (stemmers, normalizers, language
// lowercasing, elision), mapped the way OpenSearch's analysis-common module
// registers them, on top of the Lucene 10.5.1 ports.

// langTermFilters are filters that rewrite every token's term and ignore the
// keyword attribute.
var langTermFilters = map[string]func(string) string{
	"uppercase":                  luceneUpper,
	"german_normalization":       germanNormalizeLucene,
	"scandinavian_folding":       scandinavianFoldLucene,
	"scandinavian_normalization": scandinavianNormalizeLucene,
	"arabic_normalization":       arabicNormalizeLucene,
	"persian_normalization":      persianNormalizeLucene,
	"sorani_normalization":       soraniNormalizeLucene,
	"hindi_normalization":        hindiNormalizeLucene,
	"indic_normalization":        indicNormalizeLucene,
	"bengali_normalization":      bengaliNormalizeLucene,
	"serbian_normalization":      serbianNormalizeLucene,
	"decimal_digit":              decimalDigitLucene,
	"cjk_width":                  cjkWidthLucene,
	"apostrophe":                 apostropheLucene,
	"turkish_lowercase":          turkishLowerLucene,
	"irish_lowercase":            irishLowerLucene,
	"greek_lowercase":            greekLowerLucene,
}

// langStemFilters are stemming filters that skip keyword tokens.
var langStemFilters = map[string]func(string) string{
	"kstem":          kstemLucene,
	"porter_stem":    porterStemLucene,
	"russian_stem":   snowballStem("Russian"),
	"czech_stem":     czechStemLucene,
	"french_stem":    snowballStem("French"),
	"dutch_stem":     snowballStem("Dutch"),
	"german_stem":    germanStemOld,
	"brazilian_stem": brazilianStem,
	"arabic_stem":    arabicStemLucene,
	"persian_stem":   persianStemLucene,
}

func init() {
	for name := range langTermFilters {
		knownFilterTypes[name] = true
	}
	for name := range langStemFilters {
		knownFilterTypes[name] = true
	}
}

// langFilterOne applies a language filter; handled is false for filters
// implemented elsewhere.
func langFilterOne(toks []analyzedToken, spec filterSpec) (out []analyzedToken, err *apiError, handled bool) {
	p := spec.params
	if f, ok := langTermFilters[spec.typ]; ok {
		return langMap(toks, f), nil, true
	}
	if f, ok := langStemFilters[spec.typ]; ok {
		if spec.typ == "german_stem" || spec.typ == "brazilian_stem" {
			toks = langKeywords(toks, langStemExclusion(p))
		}
		return langStem(toks, f), nil, true
	}
	switch spec.typ {
	case "lowercase":
		lang, hasLang := p["language"].(string)
		f := luceneLower
		if hasLang {
			switch {
			case equalFoldASCII(lang, "greek"):
				f = greekLowerLucene
			case equalFoldASCII(lang, "irish"):
				f = irishLowerLucene
			case equalFoldASCII(lang, "turkish"):
				f = turkishLowerLucene
			default:
				return nil, filterError("language [" + lang + "] not support for lower case"), true
			}
		}
		return langMap(toks, f), nil, true
	case "elision":
		articles := frenchArticles
		if words := fsStrings(p, "articles"); len(words) > 0 {
			articles = newLangWordSet(words, fsBool(p, "articles_case", false))
		}
		return langMap(toks, func(s string) string { return luceneElision(s, articles) }), nil, true
	case "stemmer":
		language := fsString(p, "language", fsString(p, "name", "porter"))
		f, cls, ok := stemmerLanguageFunc(language)
		if !ok {
			return nil, stemmerError(cls), true
		}
		return langStem(toks, f), nil, true
	case "snowball":
		cls := capitalizeFirst(fsString(p, "language", fsString(p, "name", "English")))
		f, ok := snowballStemFunc(cls)
		if !ok {
			return nil, stemmerError(cls), true
		}
		return langStem(toks, f), nil, true
	case "cjk_bigram":
		return cjkBigramLucene(toks, cjkBigramFlags(p), fsBool(p, "output_unigrams", false)), nil, true
	}
	return toks, nil, false
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}
