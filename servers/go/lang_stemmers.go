package main

import "strings"

// Stemmer selection, following OpenSearch's StemmerTokenFilterFactory and
// SnowballTokenFilterFactory on top of the Lucene 10.5.1 ports
// (romance_stem.go, gmc_*.go, kstem.go, ind_lucene.go, stem_*.go and the
// generated Snowball stemmers).

type snowballStemmer interface {
	setCurrent(string)
	stem()
	getCurrent() string
}

// snowballConstructors maps Snowball class names (the part before
// "Stemmer" in org.tartarus.snowball.ext) to their generated ports.
var snowballConstructors = map[string]func() snowballStemmer{
	"Arabic":     func() snowballStemmer { return newArabic() },
	"Armenian":   func() snowballStemmer { return newArmenian() },
	"Basque":     func() snowballStemmer { return newBasque() },
	"Catalan":    func() snowballStemmer { return newCatalan() },
	"Danish":     func() snowballStemmer { return newDanish() },
	"Dutch":      func() snowballStemmer { return newDutch() },
	"English":    func() snowballStemmer { return newEnglish() },
	"Estonian":   func() snowballStemmer { return newEstonian() },
	"Finnish":    func() snowballStemmer { return newFinnish() },
	"French":     func() snowballStemmer { return newFrench() },
	"German":     func() snowballStemmer { return newGerman() },
	"Greek":      func() snowballStemmer { return newGreek() },
	"Hindi":      func() snowballStemmer { return newHindi() },
	"Hungarian":  func() snowballStemmer { return newHungarian() },
	"Indonesian": func() snowballStemmer { return newIndonesian() },
	"Irish":      func() snowballStemmer { return newIrish() },
	"Italian":    func() snowballStemmer { return newItalian() },
	"Lithuanian": func() snowballStemmer { return newLithuanian() },
	"Norwegian":  func() snowballStemmer { return newNorwegian() },
	"Portuguese": func() snowballStemmer { return newPortuguese() },
	"Romanian":   func() snowballStemmer { return newRomanian() },
	"Russian":    func() snowballStemmer { return newRussian() },
	"Serbian":    func() snowballStemmer { return newSerbian() },
	"Spanish":    func() snowballStemmer { return newSpanish() },
	"Swedish":    func() snowballStemmer { return newSwedish() },
	"Turkish":    func() snowballStemmer { return newTurkish() },
	"Porter":     func() snowballStemmer { return newPorterSnowball() },
	"Nepali":     func() snowballStemmer { return newNepali() },
	"Tamil":      func() snowballStemmer { return newTamil() },
	"Yiddish":    func() snowballStemmer { return newYiddish() },
}

// snowballStemFunc returns a stemming function for a Snowball class name, as
// SnowballFilter(TokenStream, String) resolves it.
func snowballStemFunc(name string) (func(string) string, bool) {
	if name == "German2" {
		name = "German"
	}
	ctor, ok := snowballConstructors[name]
	if !ok {
		return nil, false
	}
	return func(term string) string { return stemSnowball(ctor(), term) }, true
}

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
		"arabic":             arabicStemLucene,
		"armenian":           nil,
		"basque":             nil,
		"bengali":            bengaliStemLucene,
		"brazilian":          brazilianStem,
		"bulgarian":          bulgarianStemLucene,
		"catalan":            nil,
		"czech":              czechStemLucene,
		"danish":             nil,
		"dutch":              nil,
		"english":            porterStemLucene,
		"light_english":      kstemLucene,
		"lightenglish":       kstemLucene,
		"kstem":              kstemLucene,
		"porter":             porterStemLucene,
		"porter2":            nil,
		"minimal_english":    englishMinimalStem,
		"minimalenglish":     englishMinimalStem,
		"plural_english":     englishPluralStem,
		"pluralenglish":      englishPluralStem,
		"possessive_english": englishPossessive,
		"possessiveenglish":  englishPossessive,
		"estonian":           nil,
		"finnish":            nil,
		"light_finish":       finnishLightStem,
		"lightfinish":        finnishLightStem,
		"light_finnish":      finnishLightStem,
		"lightfinnish":       finnishLightStem,
		"french":             nil,
		"light_french":       frenchLightStem,
		"lightfrench":        frenchLightStem,
		"minimal_french":     frenchMinimalStem,
		"minimalfrench":      frenchMinimalStem,
		"galician":           galicianRSLPStem,
		"minimal_galician":   galicianMinimalStem,
		"german":             nil,
		"german2":            nil,
		"light_german":       germanLightStem,
		"lightgerman":        germanLightStem,
		"minimal_german":     germanMinimalStem,
		"minimalgerman":      germanMinimalStem,
		"greek":              greekStemLucene,
		"hindi":              hindiStemLucene,
		"hungarian":          nil,
		"light_hungarian":    hungarianLightStem,
		"lighthungarian":     hungarianLightStem,
		"indonesian":         func(t string) string { return indonesianStemLucene(t, true) },
		"irish":              nil,
		"italian":            nil,
		"light_italian":      italianLightStem,
		"lightitalian":       italianLightStem,
		"latvian":            latvianStemLucene,
		"lithuanian":         nil,
		"norwegian":          nil,
		"light_norwegian":    func(t string) string { return norwegianLightStem(t, gmcNorwegianBokmaal) },
		"lightnorwegian":     func(t string) string { return norwegianLightStem(t, gmcNorwegianBokmaal) },
		"minimal_norwegian":  func(t string) string { return norwegianMinimalStem(t, gmcNorwegianBokmaal) },
		"light_nynorsk":      func(t string) string { return norwegianLightStem(t, gmcNorwegianNynorsk) },
		"lightnynorsk":       func(t string) string { return norwegianLightStem(t, gmcNorwegianNynorsk) },
		"minimal_nynorsk":    func(t string) string { return norwegianMinimalStem(t, gmcNorwegianNynorsk) },
		"minimalnynorsk":     func(t string) string { return norwegianMinimalStem(t, gmcNorwegianNynorsk) },
		"persian":            persianStemLucene,
		"portuguese":         nil,
		"light_portuguese":   portugueseLightStem,
		"lightportuguese":    portugueseLightStem,
		"minimal_portuguese": portugueseMinimalStem,
		"minimalportuguese":  portugueseMinimalStem,
		"portuguese_rslp":    portugueseRSLPStem,
		"romanian":           nil,
		"russian":            nil,
		"light_russian":      russianLightStem,
		"lightrussian":       russianLightStem,
		"spanish":            nil,
		"light_spanish":      spanishLightStem,
		"lightspanish":       spanishLightStem,
		"sorani":             soraniStemLucene,
		"swedish":            nil,
		"light_swedish":      swedishLightStem,
		"lightswedish":       swedishLightStem,
		"turkish":            nil,
	}
	// "minimalNorwegian" is matched case-sensitively upstream.
	if language == "minimalNorwegian" {
		return func(t string) string { return norwegianMinimalStem(t, gmcNorwegianBokmaal) }, "", true
	}
	if f, ok := byName[l]; ok {
		if f != nil {
			return f, "", true
		}
		snow := map[string]string{"porter2": "English", "german2": "German"}[l]
		if snow == "" {
			snow = capitalizeFirst(l)
		}
		g, _ := snowballStemFunc(snow)
		return g, "", true
	}
	cls := capitalizeFirst(language)
	if g, ok := snowballStemFunc(cls); ok {
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
func stemmerError(cls string) *apiError {
	return &apiError{Type: "illegal_argument_exception", Status: 400,
		Reason:    "Invalid stemmer class specified: " + cls,
		RootCause: &apiError{Type: "illegal_argument_exception", Status: 400, Reason: "Invalid stemmer class specified: " + cls},
		CausedBy:  &apiError{Type: "class_not_found_exception", Reason: "org.tartarus.snowball.ext." + cls + "Stemmer"}}
}
