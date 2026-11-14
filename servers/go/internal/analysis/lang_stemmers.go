package analysis

import (
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/lucene"
	"slopensearch/internal/snowball"
)

// Stemmer selection, following OpenSearch's StemmerTokenFilterFactory and
// SnowballTokenFilterFactory on top of the Lucene 10.5.1 ports
// (romance_stem.go, gmc_*.go, kstem.go, ind_lucene.go, stem_*.go and the
// generated Snowball stemmers).

// capitalizeFirst mirrors org.opensearch.core.common.Strings.capitalize.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	rs := []rune(s)
	rs[0] = []rune(strings.ToUpper(string(rs[0])))[0]
	return string(rs)
}

// stemmerLanguageFunc resolves the "language" of a stemmer token filter. The
// returned name is the Snowball class name reported when it is unknown.
func stemmerLanguageFunc(language string) (func(string) string, string, bool) {
	l := strings.ToLower(language)
	byName := map[string]func(string) string{
		"arabic":             lucene.ArabicStem,
		"armenian":           nil,
		"basque":             nil,
		"bengali":            lucene.BengaliStem,
		"brazilian":          lucene.BrazilianStem,
		"bulgarian":          lucene.BulgarianStem,
		"catalan":            nil,
		"czech":              lucene.CzechStem,
		"danish":             nil,
		"dutch":              nil,
		"english":            lucene.PorterStem,
		"light_english":      lucene.KStem,
		"lightenglish":       lucene.KStem,
		"kstem":              lucene.KStem,
		"porter":             lucene.PorterStem,
		"porter2":            nil,
		"minimal_english":    lucene.EnglishMinimalStem,
		"minimalenglish":     lucene.EnglishMinimalStem,
		"plural_english":     englishPluralStem,
		"pluralenglish":      englishPluralStem,
		"possessive_english": lucene.EnglishPossessive,
		"possessiveenglish":  lucene.EnglishPossessive,
		"estonian":           nil,
		"finnish":            nil,
		"light_finish":       lucene.FinnishLightStem,
		"lightfinish":        lucene.FinnishLightStem,
		"light_finnish":      lucene.FinnishLightStem,
		"lightfinnish":       lucene.FinnishLightStem,
		"french":             nil,
		"light_french":       lucene.FrenchLightStem,
		"lightfrench":        lucene.FrenchLightStem,
		"minimal_french":     lucene.FrenchMinimalStem,
		"minimalfrench":      lucene.FrenchMinimalStem,
		"galician":           lucene.GalicianRSLPStem,
		"minimal_galician":   lucene.GalicianMinimalStem,
		"german":             nil,
		"german2":            nil,
		"light_german":       lucene.GermanLightStem,
		"lightgerman":        lucene.GermanLightStem,
		"minimal_german":     lucene.GermanMinimalStem,
		"minimalgerman":      lucene.GermanMinimalStem,
		"greek":              lucene.GreekStem,
		"hindi":              lucene.HindiStem,
		"hungarian":          nil,
		"light_hungarian":    lucene.HungarianLightStem,
		"lighthungarian":     lucene.HungarianLightStem,
		"indonesian":         func(t string) string { return lucene.IndonesianStem(t, true) },
		"irish":              nil,
		"italian":            nil,
		"light_italian":      lucene.ItalianLightStem,
		"lightitalian":       lucene.ItalianLightStem,
		"latvian":            lucene.LatvianStem,
		"lithuanian":         nil,
		"norwegian":          nil,
		"light_norwegian":    func(t string) string { return lucene.NorwegianLightStem(t, lucene.NorwegianBokmaal) },
		"lightnorwegian":     func(t string) string { return lucene.NorwegianLightStem(t, lucene.NorwegianBokmaal) },
		"minimal_norwegian":  func(t string) string { return lucene.NorwegianMinimalStem(t, lucene.NorwegianBokmaal) },
		"light_nynorsk":      func(t string) string { return lucene.NorwegianLightStem(t, lucene.NorwegianNynorsk) },
		"lightnynorsk":       func(t string) string { return lucene.NorwegianLightStem(t, lucene.NorwegianNynorsk) },
		"minimal_nynorsk":    func(t string) string { return lucene.NorwegianMinimalStem(t, lucene.NorwegianNynorsk) },
		"minimalnynorsk":     func(t string) string { return lucene.NorwegianMinimalStem(t, lucene.NorwegianNynorsk) },
		"persian":            lucene.PersianStem,
		"portuguese":         nil,
		"light_portuguese":   lucene.PortugueseLightStem,
		"lightportuguese":    lucene.PortugueseLightStem,
		"minimal_portuguese": lucene.PortugueseMinimalStem,
		"minimalportuguese":  lucene.PortugueseMinimalStem,
		"portuguese_rslp":    lucene.PortugueseRSLPStem,
		"romanian":           nil,
		"russian":            nil,
		"light_russian":      lucene.RussianLightStem,
		"lightrussian":       lucene.RussianLightStem,
		"spanish":            nil,
		"light_spanish":      lucene.SpanishLightStem,
		"lightspanish":       lucene.SpanishLightStem,
		"sorani":             lucene.SoraniStem,
		"swedish":            nil,
		"light_swedish":      lucene.SwedishLightStem,
		"lightswedish":       lucene.SwedishLightStem,
		"turkish":            nil,
	}
	// "minimalNorwegian" is matched case-sensitively upstream.
	if language == "minimalNorwegian" {
		return func(t string) string { return lucene.NorwegianMinimalStem(t, lucene.NorwegianBokmaal) }, "", true
	}
	if f, ok := byName[l]; ok {
		if f != nil {
			return f, "", true
		}
		snow := map[string]string{"porter2": "English", "german2": "German"}[l]
		if snow == "" {
			snow = capitalizeFirst(l)
		}
		g, _ := snowball.StemFunc(snow)
		return g, "", true
	}
	cls := capitalizeFirst(language)
	if g, ok := snowball.StemFunc(cls); ok {
		return g, "", true
	}
	return nil, cls, false
}

var pluralOesExceptions = []string{"shoes", "canoes", "oboes"}
var pluralChesExceptions = []string{"cliches", "avalanches", "mustaches", "moustaches", "quiches",
	"headaches", "heartaches", "porsches", "tranches", "caches"}

// pluralException ports EnglishPluralStemmer.isException: the rule and the
// term agree on their common tail.
func pluralException(s []rune, rules []string) bool {
	for _, rule := range rules {
		r := []rune(rule)
		matched := true
		for rp, sp := len(r)-1, len(s)-1; rp >= 0 && sp >= 0; rp, sp = rp-1, sp-1 {
			if r[rp] != s[sp] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// englishPluralStem ports OpenSearch's EnglishPluralStemFilter
// (plural_english).
func englishPluralStem(term string) string {
	s := []rune(term)
	n := len(s)
	if n < 3 || s[n-1] != 's' {
		return term
	}
	switch s[n-2] {
	case 'u', 's':
		return term
	case 'e':
		if n > 4 && s[n-3] == 'i' {
			s[n-3] = 'y'
			return string(s[:n-2])
		}
		if n > 3 {
			if n > 4 && s[n-3] == 'x' {
				return string(s[:n-2])
			}
			if s[n-3] == 'o' {
				if pluralException(s, pluralOesExceptions) {
					return string(s[:n-1])
				}
				return string(s[:n-2])
			}
			if n > 4 {
				if s[n-4] == 's' && (s[n-3] == 'h' || s[n-3] == 's') {
					return string(s[:n-2])
				}
				if s[n-4] == 'c' && s[n-3] == 'h' {
					if pluralException(s, pluralChesExceptions) {
						return string(s[:n-1])
					}
					return string(s[:n-2])
				}
			}
		}
	}
	return string(s[:n-1])
}

// stemmerError is the error StemmerTokenFilterFactory raises for an
// unknown language (SnowballFilter's class lookup failure).
func stemmerError(cls string) *apierr.Error {
	return &apierr.Error{Type: "illegal_argument_exception", Status: 400,
		Reason:    "Invalid stemmer class specified: " + cls,
		RootCause: &apierr.Error{Type: "illegal_argument_exception", Status: 400, Reason: "Invalid stemmer class specified: " + cls},
		CausedBy:  &apierr.Error{Type: "class_not_found_exception", Reason: "org.tartarus.snowball.ext." + cls + "Stemmer"}}
}
