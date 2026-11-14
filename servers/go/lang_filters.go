package main

import (
	"slopensearch/internal/snowball"

	"slopensearch/internal/lucene"
)

// Language-specific token filters (stemmers, normalizers, language
// lowercasing, elision), mapped the way OpenSearch's analysis-common module
// registers them, on top of the Lucene 10.5.1 ports.

// langTermFilters are filters that rewrite every token's term and ignore the
// keyword attribute.
var langTermFilters = map[string]func(string) string{
	"uppercase":                  lucene.Upper,
	"german_normalization":       lucene.GermanNormalize,
	"scandinavian_folding":       lucene.ScandinavianFold,
	"scandinavian_normalization": lucene.ScandinavianNormalize,
	"arabic_normalization":       lucene.ArabicNormalize,
	"persian_normalization":      lucene.PersianNormalize,
	"sorani_normalization":       lucene.SoraniNormalize,
	"hindi_normalization":        lucene.HindiNormalize,
	"indic_normalization":        lucene.IndicNormalize,
	"bengali_normalization":      lucene.BengaliNormalize,
	"serbian_normalization":      lucene.SerbianNormalize,
	"decimal_digit":              lucene.DecimalDigit,
	"cjk_width":                  lucene.CJKWidth,
	"apostrophe":                 lucene.Apostrophe,
	"turkish_lowercase":          lucene.TurkishLower,
	"irish_lowercase":            lucene.IrishLower,
	"greek_lowercase":            lucene.GreekLower,
}

// langStemFilters are stemming filters that skip keyword tokens.
var langStemFilters = map[string]func(string) string{
	"kstem":          lucene.KStem,
	"porter_stem":    lucene.PorterStem,
	"russian_stem":   snowballStem("Russian"),
	"czech_stem":     lucene.CzechStem,
	"french_stem":    snowballStem("French"),
	"dutch_stem":     snowballStem("Dutch"),
	"german_stem":    lucene.GermanStemOld,
	"brazilian_stem": lucene.BrazilianStem,
	"arabic_stem":    lucene.ArabicStem,
	"persian_stem":   lucene.PersianStem,
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
		f := lucene.Lower
		if hasLang {
			switch {
			case equalFoldASCII(lang, "greek"):
				f = lucene.GreekLower
			case equalFoldASCII(lang, "irish"):
				f = lucene.IrishLower
			case equalFoldASCII(lang, "turkish"):
				f = lucene.TurkishLower
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
		f, ok := snowball.StemFunc(cls)
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
