package main

import "unicode/utf8"

// Light stemmers of Lucene 10.5.1 analysis-common: Bulgarian, Czech, Latvian, Indonesian.
// Like Lucene they work in place on a buffer and some rules rely on characters left in the
// buffer beyond the current length, so the buffer is kept at full size.

// mscEndsWith is StemmerUtil.endsWith(s, n, suffix).
func mscEndsWith(s []rune, n int, suffix string) bool {
	j := n
	for k := len(suffix); k > 0; {
		r, size := utf8.DecodeLastRuneInString(suffix[:k])
		k -= size
		j--
		if j < 0 || s[j] != r {
			return false
		}
	}
	return true
}

// mscStartsWith is StemmerUtil.startsWith(s, n, prefix).
func mscStartsWith(s []rune, n int, prefix string) bool {
	j := 0
	for _, r := range prefix {
		if j >= n || s[j] != r {
			return false
		}
		j++
	}
	return true
}

// mscDeleteN is StemmerUtil.deleteN(s, pos, n, nChars).
func mscDeleteN(s []rune, pos, n, nChars int) int {
	if pos+nChars < n {
		copy(s[pos:], s[pos+nChars:n])
	}
	return n - nChars
}

// bulgarianStemLucene ports org.apache.lucene.analysis.bg.BulgarianStemFilter (BulgarianStemmer).
func bulgarianStemLucene(term string) string {
	s := []rune(term)
	n := mscBulgarianStem(s, len(s))
	return string(s[:n])
}

func mscBulgarianStem(s []rune, n int) int {
	if n < 4 {
		return n
	}
	if n > 5 && mscEndsWith(s, n, "ища") {
		return n - 3
	}
	n = mscBulgarianRemoveArticle(s, n)
	n = mscBulgarianRemovePlural(s, n)
	if n > 3 {
		if mscEndsWith(s, n, "я") {
			n--
		}
		if mscEndsWith(s, n, "а") || mscEndsWith(s, n, "о") || mscEndsWith(s, n, "е") {
			n--
		}
	}
	if n > 4 && mscEndsWith(s, n, "ен") {
		s[n-2] = 'н'
		n--
	}
	if n > 5 && s[n-2] == 'ъ' {
		s[n-2] = s[n-1]
		n--
	}
	return n
}

func mscBulgarianRemoveArticle(s []rune, n int) int {
	if n > 6 && mscEndsWith(s, n, "ият") {
		return n - 3
	}
	if n > 5 {
		if mscEndsWith(s, n, "ът") || mscEndsWith(s, n, "то") || mscEndsWith(s, n, "те") ||
			mscEndsWith(s, n, "та") || mscEndsWith(s, n, "ия") {
			return n - 2
		}
	}
	if n > 4 && mscEndsWith(s, n, "ят") {
		return n - 2
	}
	return n
}

func mscBulgarianRemovePlural(s []rune, n int) int {
	if n > 6 {
		if mscEndsWith(s, n, "овци") {
			return n - 3
		}
		if mscEndsWith(s, n, "ове") {
			return n - 3
		}
		if mscEndsWith(s, n, "еве") {
			s[n-3] = 'й'
			return n - 2
		}
	}
	if n > 5 {
		if mscEndsWith(s, n, "ища") {
			return n - 3
		}
		if mscEndsWith(s, n, "та") {
			return n - 2
		}
		if mscEndsWith(s, n, "ци") {
			s[n-2] = 'к'
			return n - 1
		}
		if mscEndsWith(s, n, "зи") {
			s[n-2] = 'г'
			return n - 1
		}
		if s[n-3] == 'е' && s[n-1] == 'и' {
			s[n-3] = 'я'
			return n - 1
		}
	}
	if n > 4 {
		if mscEndsWith(s, n, "си") {
			s[n-2] = 'х'
			return n - 1
		}
		if mscEndsWith(s, n, "и") {
			return n - 1
		}
	}
	return n
}

// czechStemLucene ports org.apache.lucene.analysis.cz.CzechStemFilter (CzechStemmer).
// Input is expected lowercase, with diacritics.
func czechStemLucene(term string) string {
	s := []rune(term)
	n := mscCzechRemoveCase(s, len(s))
	n = mscCzechRemovePossessives(s, n)
	if n > 0 {
		n = mscCzechNormalize(s, n)
	}
	return string(s[:n])
}

func mscCzechEndsAny(s []rune, n int, suffixes []string) bool {
	for _, x := range suffixes {
		if mscEndsWith(s, n, x) {
			return true
		}
	}
	return false
}

var mscCzechSuffix3 = []string{
	"ech", "ich", "ích", "ého", "ěmi", "emi", "ému", "ěte", "ete", "ěti", "eti", "ího", "iho",
	"ími", "ímu", "imu", "ách", "ata", "aty", "ých", "ama", "ami", "ové", "ovi", "ými",
}

var mscCzechSuffix2 = []string{"em", "es", "ém", "ím", "ům", "at", "ám", "os", "us", "ým", "mi", "ou"}

func mscCzechRemoveCase(s []rune, n int) int {
	if n > 7 && mscEndsWith(s, n, "atech") {
		return n - 5
	}
	if n > 6 && (mscEndsWith(s, n, "ětem") || mscEndsWith(s, n, "etem") || mscEndsWith(s, n, "atům")) {
		return n - 4
	}
	if n > 5 && mscCzechEndsAny(s, n, mscCzechSuffix3) {
		return n - 3
	}
	if n > 4 && mscCzechEndsAny(s, n, mscCzechSuffix2) {
		return n - 2
	}
	if n > 3 {
		switch s[n-1] {
		case 'a', 'e', 'i', 'o', 'u', 'ů', 'y', 'á', 'é', 'í', 'ý', 'ě':
			return n - 1
		}
	}
	return n
}

func mscCzechRemovePossessives(s []rune, n int) int {
	if n > 5 && (mscEndsWith(s, n, "ov") || mscEndsWith(s, n, "in") || mscEndsWith(s, n, "ův")) {
		return n - 2
	}
	return n
}

func mscCzechNormalize(s []rune, n int) int {
	if mscEndsWith(s, n, "čt") {
		s[n-2] = 'c'
		s[n-1] = 'k'
		return n
	}
	if mscEndsWith(s, n, "št") {
		s[n-2] = 's'
		s[n-1] = 'k'
		return n
	}
	switch s[n-1] {
	case 'c', 'č':
		s[n-1] = 'k'
		return n
	case 'z', 'ž':
		s[n-1] = 'h'
		return n
	}
	if n > 1 && s[n-2] == 'e' {
		s[n-2] = s[n-1]
		return n - 1
	}
	if n > 2 && s[n-2] == 'ů' {
		s[n-2] = 'o'
		return n
	}
	return n
}

// latvianStemLucene ports org.apache.lucene.analysis.lv.LatvianStemFilter (LatvianStemmer).
func latvianStemLucene(term string) string {
	s := []rune(term)
	n := len(s)
	numVowels := 0
	for _, c := range s {
		switch c {
		case 'a', 'e', 'i', 'o', 'u', 'ā', 'ī', 'ē', 'ū':
			numVowels++
		}
	}
	for _, a := range mscLatvianAffixes {
		al := utf8.RuneCountInString(a.affix)
		if numVowels > a.vc && n >= al+3 && mscEndsWith(s, n, a.affix) {
			n -= al
			if a.palatalizes {
				n = mscLatvianUnpalatalize(s, n)
			}
			return string(s[:n])
		}
	}
	return term
}

type mscLatvianAffix struct {
	affix       string
	vc          int
	palatalizes bool
}

var mscLatvianAffixes = []mscLatvianAffix{
	{"ajiem", 3, false}, {"ajai", 3, false},
	{"ajam", 2, false}, {"ajām", 2, false},
	{"ajos", 2, false}, {"ajās", 2, false},
	{"iem", 2, true}, {"ajā", 2, false},
	{"ais", 2, false}, {"ai", 2, false},
	{"ei", 2, false}, {"ām", 1, false},
	{"am", 1, false}, {"ēm", 1, false},
	{"īm", 1, false}, {"im", 1, false},
	{"um", 1, false}, {"us", 1, true},
	{"as", 1, false}, {"ās", 1, false},
	{"es", 1, false}, {"os", 1, true},
	{"ij", 1, false}, {"īs", 1, false},
	{"ēs", 1, false}, {"is", 1, false},
	{"ie", 1, false}, {"u", 1, true},
	{"a", 1, true}, {"i", 1, true},
	{"e", 1, false}, {"ā", 1, false},
	{"ē", 1, false}, {"ī", 1, false},
	{"ū", 1, false}, {"o", 1, false},
	{"s", 0, false}, {"š", 0, false},
}

func mscLatvianUnpalatalize(s []rune, n int) int {
	if s[n] == 'u' {
		if mscEndsWith(s, n, "kš") {
			n++
			s[n-2] = 's'
			s[n-1] = 't'
			return n
		}
		if mscEndsWith(s, n, "ņņ") {
			s[n-2] = 'n'
			s[n-1] = 'n'
			return n
		}
	}
	switch {
	case mscEndsWith(s, n, "pj") || mscEndsWith(s, n, "bj") || mscEndsWith(s, n, "mj") || mscEndsWith(s, n, "vj"):
		return n - 1
	case mscEndsWith(s, n, "šņ"):
		s[n-2], s[n-1] = 's', 'n'
	case mscEndsWith(s, n, "žņ"):
		s[n-2], s[n-1] = 'z', 'n'
	case mscEndsWith(s, n, "šļ"):
		s[n-2], s[n-1] = 's', 'l'
	case mscEndsWith(s, n, "žļ"):
		s[n-2], s[n-1] = 'z', 'l'
	case mscEndsWith(s, n, "ļņ"):
		s[n-2], s[n-1] = 'l', 'n'
	case mscEndsWith(s, n, "ļļ"):
		s[n-2], s[n-1] = 'l', 'l'
	case s[n-1] == 'č':
		s[n-1] = 'c'
	case s[n-1] == 'ļ':
		s[n-1] = 'l'
	case s[n-1] == 'ņ':
		s[n-1] = 'n'
	}
	return n
}

// indonesianStemLucene ports org.apache.lucene.analysis.id.IndonesianStemFilter (IndonesianStemmer);
// the filter's default is stemDerivational=true, false only strips particles and possessives.
func indonesianStemLucene(term string, stemDerivational bool) string {
	st := mscIndonesianStemmer{text: []rune(term)}
	n := st.stem(len(st.text), stemDerivational)
	return string(st.text[:n])
}

const (
	mscIdRemovedKE   = 1
	mscIdRemovedPENG = 2
	mscIdRemovedDI   = 4
	mscIdRemovedMENG = 8
	mscIdRemovedTER  = 16
	mscIdRemovedBER  = 32
	mscIdRemovedPE   = 64
)

type mscIndonesianStemmer struct {
	text         []rune
	numSyllables int
	flags        int
}

func mscIdIsVowel(c rune) bool {
	switch c {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}

func (st *mscIndonesianStemmer) stem(n int, stemDerivational bool) int {
	st.flags = 0
	st.numSyllables = 0
	for i := 0; i < n; i++ {
		if mscIdIsVowel(st.text[i]) {
			st.numSyllables++
		}
	}
	if st.numSyllables > 2 {
		n = st.removeParticle(n)
	}
	if st.numSyllables > 2 {
		n = st.removePossessivePronoun(n)
	}
	if stemDerivational {
		n = st.stemDerivational(n)
	}
	return n
}

func (st *mscIndonesianStemmer) stemDerivational(n int) int {
	old := n
	if st.numSyllables > 2 {
		n = st.removeFirstOrderPrefix(n)
	}
	if old != n {
		old = n
		if st.numSyllables > 2 {
			n = st.removeSuffix(n)
		}
		if old != n {
			if st.numSyllables > 2 {
				n = st.removeSecondOrderPrefix(n)
			}
		}
	} else {
		if st.numSyllables > 2 {
			n = st.removeSecondOrderPrefix(n)
		}
		if st.numSyllables > 2 {
			n = st.removeSuffix(n)
		}
	}
	return n
}

func (st *mscIndonesianStemmer) removeParticle(n int) int {
	t := st.text
	if mscEndsWith(t, n, "kah") || mscEndsWith(t, n, "lah") || mscEndsWith(t, n, "pun") {
		st.numSyllables--
		return n - 3
	}
	return n
}

func (st *mscIndonesianStemmer) removePossessivePronoun(n int) int {
	t := st.text
	if mscEndsWith(t, n, "ku") || mscEndsWith(t, n, "mu") {
		st.numSyllables--
		return n - 2
	}
	if mscEndsWith(t, n, "nya") {
		st.numSyllables--
		return n - 3
	}
	return n
}

// del records the flag, decrements the syllable count and deletes k leading chars.
func (st *mscIndonesianStemmer) del(n, k, flag int) int {
	st.flags |= flag
	st.numSyllables--
	return mscDeleteN(st.text, 0, n, k)
}

func (st *mscIndonesianStemmer) removeFirstOrderPrefix(n int) int {
	t := st.text
	switch {
	case mscStartsWith(t, n, "meng"):
		return st.del(n, 4, mscIdRemovedMENG)
	case mscStartsWith(t, n, "meny") && n > 4 && mscIdIsVowel(t[4]):
		t[3] = 's'
		return st.del(n, 3, mscIdRemovedMENG)
	case mscStartsWith(t, n, "men"):
		return st.del(n, 3, mscIdRemovedMENG)
	case mscStartsWith(t, n, "mem"):
		return st.del(n, 3, mscIdRemovedMENG)
	case mscStartsWith(t, n, "me"):
		return st.del(n, 2, mscIdRemovedMENG)
	case mscStartsWith(t, n, "peng"):
		return st.del(n, 4, mscIdRemovedPENG)
	case mscStartsWith(t, n, "peny") && n > 4 && mscIdIsVowel(t[4]):
		t[3] = 's'
		return st.del(n, 3, mscIdRemovedPENG)
	case mscStartsWith(t, n, "peny"):
		return st.del(n, 4, mscIdRemovedPENG)
	case mscStartsWith(t, n, "pen") && n > 3 && mscIdIsVowel(t[3]):
		t[2] = 't'
		return st.del(n, 2, mscIdRemovedPENG)
	case mscStartsWith(t, n, "pen"):
		return st.del(n, 3, mscIdRemovedPENG)
	case mscStartsWith(t, n, "pem"):
		return st.del(n, 3, mscIdRemovedPENG)
	case mscStartsWith(t, n, "di"):
		return st.del(n, 2, mscIdRemovedDI)
	case mscStartsWith(t, n, "ter"):
		return st.del(n, 3, mscIdRemovedTER)
	case mscStartsWith(t, n, "ke"):
		return st.del(n, 2, mscIdRemovedKE)
	}
	return n
}

func (st *mscIndonesianStemmer) removeSecondOrderPrefix(n int) int {
	t := st.text
	switch {
	case mscStartsWith(t, n, "ber"):
		return st.del(n, 3, mscIdRemovedBER)
	case n == 7 && mscStartsWith(t, n, "belajar"):
		return st.del(n, 3, mscIdRemovedBER)
	case mscStartsWith(t, n, "be") && n > 4 && !mscIdIsVowel(t[2]) && t[3] == 'e' && t[4] == 'r':
		return st.del(n, 2, mscIdRemovedBER)
	case mscStartsWith(t, n, "per"):
		return st.del(n, 3, 0)
	case n == 7 && mscStartsWith(t, n, "pelajar"):
		return st.del(n, 3, 0)
	case mscStartsWith(t, n, "pe"):
		return st.del(n, 2, mscIdRemovedPE)
	}
	return n
}

func (st *mscIndonesianStemmer) removeSuffix(n int) int {
	t, f := st.text, st.flags
	if mscEndsWith(t, n, "kan") && f&mscIdRemovedKE == 0 && f&mscIdRemovedPENG == 0 && f&mscIdRemovedPE == 0 {
		st.numSyllables--
		return n - 3
	}
	if mscEndsWith(t, n, "an") && f&mscIdRemovedDI == 0 && f&mscIdRemovedMENG == 0 && f&mscIdRemovedTER == 0 {
		st.numSyllables--
		return n - 2
	}
	if mscEndsWith(t, n, "i") && !mscEndsWith(t, n, "si") && f&mscIdRemovedBER == 0 &&
		f&mscIdRemovedKE == 0 && f&mscIdRemovedPENG == 0 {
		st.numSyllables--
		return n - 1
	}
	return n
}
