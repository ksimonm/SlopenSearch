// Package snowball has Go ports of the Snowball stemmers bundled with Apache
// Lucene (org.tartarus.snowball.ext), generated from Lucene's Java sources,
// and the small runtime they need.
package snowball

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

// StemFunc returns a stemming function for a Snowball class name, as
// SnowballFilter(TokenStream, String) resolves it.
func StemFunc(name string) (func(string) string, bool) {
	if name == "German2" {
		name = "German"
	}
	ctor, ok := snowballConstructors[name]
	if !ok {
		return nil, false
	}
	return func(term string) string { return stemSnowball(ctor(), term) }, true
}
