package main

// Ports of the Lucene 10.5.1 light/minimal stemmers for French, Spanish,
// Italian, Portuguese and Galician, the RSLP stemmers (Portuguese, Galician),
// the Brazilian stemmer and the Romanian normalization filter. Each entry
// point reproduces the effect of the corresponding TokenFilter on one
// non-keyword token. Lucene works on UTF-16 chars; these work on runes, which
// is equivalent for the BMP text the algorithms deal with.

import (
	"strings"
	"unicode"
	"unicode/utf16"
)

// romEndsWith ports StemmerUtil.endsWith(char[], int, String).
func romEndsWith(s []rune, n int, suffix string) bool {
	suf := []rune(suffix)
	if len(suf) > n {
		return false
	}
	for i := len(suf) - 1; i >= 0; i-- {
		if s[n-(len(suf)-i)] != suf[i] {
			return false
		}
	}
	return true
}

// romDelete ports StemmerUtil.delete.
func romDelete(s []rune, pos, n int) int {
	if pos < n-1 {
		copy(s[pos:], s[pos+1:n])
	}
	return n - 1
}

// ---------------------------------------------------------------------------
// fr.FrenchLightStemmer

func frenchLightStem(term string) string {
	s := []rune(term)
	return string(s[:romFrLightStem(s, len(s))])
}

func romFrLightStem(s []rune, n int) int {
	if n > 5 && s[n-1] == 'x' {
		if s[n-3] == 'a' && s[n-2] == 'u' && s[n-4] != 'e' {
			s[n-2] = 'l'
		}
		n--
	}
	if n > 3 && s[n-1] == 'x' {
		n--
	}
	if n > 3 && s[n-1] == 's' {
		n--
	}
	if n > 9 && romEndsWith(s, n, "issement") {
		n -= 6
		s[n-1] = 'r'
		return romFrLightNorm(s, n)
	}
	if n > 8 && romEndsWith(s, n, "issant") {
		n -= 4
		s[n-1] = 'r'
		return romFrLightNorm(s, n)
	}
	if n > 6 && romEndsWith(s, n, "ement") {
		n -= 4
		if n > 3 && romEndsWith(s, n, "ive") {
			n--
			s[n-1] = 'f'
		}
		return romFrLightNorm(s, n)
	}
	if n > 11 && romEndsWith(s, n, "ficatrice") {
		n -= 5
		s[n-2] = 'e'
		s[n-1] = 'r'
		return romFrLightNorm(s, n)
	}
	if n > 10 && romEndsWith(s, n, "ficateur") {
		n -= 4
		s[n-2] = 'e'
		s[n-1] = 'r'
		return romFrLightNorm(s, n)
	}
	if n > 9 && romEndsWith(s, n, "catrice") {
		n -= 3
		s[n-4] = 'q'
		s[n-3] = 'u'
		s[n-2] = 'e'
		return romFrLightNorm(s, n)
	}
	if n > 8 && romEndsWith(s, n, "cateur") {
		n -= 2
		s[n-4] = 'q'
		s[n-3] = 'u'
		s[n-2] = 'e'
		s[n-1] = 'r'
		return romFrLightNorm(s, n)
	}
	if n > 8 && romEndsWith(s, n, "atrice") {
		n -= 4
		s[n-2] = 'e'
		s[n-1] = 'r'
		return romFrLightNorm(s, n)
	}
	if n > 7 && romEndsWith(s, n, "ateur") {
		n -= 3
		s[n-2] = 'e'
		s[n-1] = 'r'
		return romFrLightNorm(s, n)
	}
	if n > 6 && romEndsWith(s, n, "trice") {
		n--
		s[n-3] = 'e'
		s[n-2] = 'u'
		s[n-1] = 'r'
	}
	if n > 5 && romEndsWith(s, n, "ième") {
		return romFrLightNorm(s, n-4)
	}
	if n > 7 && romEndsWith(s, n, "teuse") {
		n -= 2
		s[n-1] = 'r'
		return romFrLightNorm(s, n)
	}
	if n > 6 && romEndsWith(s, n, "teur") {
		n--
		s[n-1] = 'r'
		return romFrLightNorm(s, n)
	}
	if n > 5 && romEndsWith(s, n, "euse") {
		return romFrLightNorm(s, n-2)
	}
	if n > 8 && romEndsWith(s, n, "ère") {
		n--
		s[n-2] = 'e'
		return romFrLightNorm(s, n)
	}
	if n > 7 && romEndsWith(s, n, "ive") {
		n--
		s[n-1] = 'f'
		return romFrLightNorm(s, n)
	}
	if n > 4 && (romEndsWith(s, n, "folle") || romEndsWith(s, n, "molle")) {
		n -= 2
		s[n-1] = 'u'
		return romFrLightNorm(s, n)
	}
	if n > 9 && romEndsWith(s, n, "nnelle") {
		return romFrLightNorm(s, n-5)
	}
	if n > 9 && romEndsWith(s, n, "nnel") {
		return romFrLightNorm(s, n-3)
	}
	if n > 4 && romEndsWith(s, n, "ète") {
		n--
		s[n-2] = 'e'
	}
	if n > 8 && romEndsWith(s, n, "ique") {
		n -= 4
	}
	if n > 8 && romEndsWith(s, n, "esse") {
		return romFrLightNorm(s, n-3)
	}
	if n > 7 && romEndsWith(s, n, "inage") {
		return romFrLightNorm(s, n-3)
	}
	if n > 9 && romEndsWith(s, n, "isation") {
		n -= 7
		if n > 5 && romEndsWith(s, n, "ual") {
			s[n-2] = 'e'
		}
		return romFrLightNorm(s, n)
	}
	if n > 9 && romEndsWith(s, n, "isateur") {
		return romFrLightNorm(s, n-7)
	}
	if n > 8 && romEndsWith(s, n, "ation") {
		return romFrLightNorm(s, n-5)
	}
	if n > 8 && romEndsWith(s, n, "ition") {
		return romFrLightNorm(s, n-5)
	}
	return romFrLightNorm(s, n)
}

func romFrLightNorm(s []rune, n int) int {
	if n > 4 {
		for i := 0; i < n; i++ {
			switch s[i] {
			case 'à', 'á', 'â':
				s[i] = 'a'
			case 'ô':
				s[i] = 'o'
			case 'è', 'é', 'ê':
				s[i] = 'e'
			case 'ù', 'û':
				s[i] = 'u'
			case 'î':
				s[i] = 'i'
			case 'ç':
				s[i] = 'c'
			}
		}
		ch := s[0]
		for i := 1; i < n; i++ {
			if s[i] == ch && unicode.IsLetter(ch) {
				n = romDelete(s, i, n)
				i--
			} else {
				ch = s[i]
			}
		}
	}
	if n > 4 && romEndsWith(s, n, "ie") {
		n -= 2
	}
	if n > 4 {
		if s[n-1] == 'r' {
			n--
		}
		if s[n-1] == 'e' {
			n--
		}
		if s[n-1] == 'e' {
			n--
		}
		if s[n-1] == s[n-2] && unicode.IsLetter(s[n-1]) {
			n--
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// fr.FrenchMinimalStemmer

func frenchMinimalStem(term string) string {
	s := []rune(term)
	n := len(s)
	if n < 6 {
		return term
	}
	if s[n-1] == 'x' {
		if s[n-3] == 'a' && s[n-2] == 'u' {
			s[n-2] = 'l'
		}
		return string(s[:n-1])
	}
	if s[n-1] == 's' {
		n--
	}
	if s[n-1] == 'r' {
		n--
	}
	if s[n-1] == 'e' {
		n--
	}
	if s[n-1] == 'é' {
		n--
	}
	if s[n-1] == s[n-2] && unicode.IsLetter(s[n-1]) {
		n--
	}
	return string(s[:n])
}

// ---------------------------------------------------------------------------
// es.SpanishLightStemmer / es.SpanishMinimalStemmer / it.ItalianLightStemmer

// romFoldVowels applies the accent folding shared by the Spanish and Italian
// stemmers; withEnye also folds 'ñ' (SpanishMinimalStemmer only).
func romFoldVowels(s []rune, withEnye bool) {
	for i, c := range s {
		switch c {
		case 'à', 'á', 'â', 'ä':
			s[i] = 'a'
		case 'ò', 'ó', 'ô', 'ö':
			s[i] = 'o'
		case 'è', 'é', 'ê', 'ë':
			s[i] = 'e'
		case 'ù', 'ú', 'û', 'ü':
			s[i] = 'u'
		case 'ì', 'í', 'î', 'ï':
			s[i] = 'i'
		case 'ñ':
			if withEnye {
				s[i] = 'n'
			}
		}
	}
}

func spanishLightStem(term string) string {
	s := []rune(term)
	n := len(s)
	if n < 5 {
		return term
	}
	romFoldVowels(s, false)
	switch s[n-1] {
	case 'o', 'a', 'e':
		n--
	case 's':
		if s[n-2] == 'e' && s[n-3] == 's' && s[n-4] == 'e' {
			n -= 2
		} else if s[n-2] == 'e' && s[n-3] == 'c' {
			s[n-3] = 'z'
			n -= 2
		} else if s[n-2] == 'o' || s[n-2] == 'a' || s[n-2] == 'e' {
			n -= 2
		}
	}
	return string(s[:n])
}

func spanishMinimalStem(term string) string {
	s := []rune(term)
	n := len(s)
	if n < 4 || s[n-1] != 's' {
		return term
	}
	romFoldVowels(s, true)
	// s[n-1] == 's' here.
	switch {
	case s[n-2] == 'a' || s[n-2] == 'o':
		n--
	case s[n-2] == 'e':
		if s[n-3] == 's' && s[n-4] == 'e' {
			n -= 2
		} else if s[n-3] == 'c' {
			s[n-3] = 'z'
			n -= 2
		} else {
			n -= 2
		}
	default:
		n--
	}
	return string(s[:n])
}

func italianLightStem(term string) string {
	s := []rune(term)
	n := len(s)
	if n < 6 {
		return term
	}
	romFoldVowels(s, false)
	switch s[n-1] {
	case 'e':
		if s[n-2] == 'i' || s[n-2] == 'h' {
			n -= 2
		} else {
			n--
		}
	case 'i':
		if s[n-2] == 'h' || s[n-2] == 'i' {
			n -= 2
		} else {
			n--
		}
	case 'a', 'o':
		if s[n-2] == 'i' {
			n -= 2
		} else {
			n--
		}
	}
	return string(s[:n])
}

// ---------------------------------------------------------------------------
// pt.PortugueseLightStemmer

func portugueseLightStem(term string) string {
	s := []rune(term)
	n := len(s)
	if n < 4 {
		return term
	}
	n = romPtLightRemoveSuffix(s, n)
	if n > 3 && s[n-1] == 'a' {
		n = romPtLightNormFeminine(s, n)
	}
	if n > 4 {
		switch s[n-1] {
		case 'e', 'a', 'o':
			n--
		}
	}
	for i := 0; i < n; i++ {
		switch s[i] {
		case 'à', 'á', 'â', 'ä', 'ã':
			s[i] = 'a'
		case 'ò', 'ó', 'ô', 'ö', 'õ':
			s[i] = 'o'
		case 'è', 'é', 'ê', 'ë':
			s[i] = 'e'
		case 'ù', 'ú', 'û', 'ü':
			s[i] = 'u'
		case 'ì', 'í', 'î', 'ï':
			s[i] = 'i'
		case 'ç':
			s[i] = 'c'
		}
	}
	return string(s[:n])
}

func romPtLightRemoveSuffix(s []rune, n int) int {
	if n > 4 && romEndsWith(s, n, "es") {
		switch s[n-3] {
		case 'r', 's', 'l', 'z':
			return n - 2
		}
	}
	if n > 3 && romEndsWith(s, n, "ns") {
		s[n-2] = 'm'
		return n - 1
	}
	if n > 4 && (romEndsWith(s, n, "eis") || romEndsWith(s, n, "éis")) {
		s[n-3] = 'e'
		s[n-2] = 'l'
		return n - 1
	}
	if n > 4 && romEndsWith(s, n, "ais") {
		s[n-2] = 'l'
		return n - 1
	}
	if n > 4 && romEndsWith(s, n, "óis") {
		s[n-3] = 'o'
		s[n-2] = 'l'
		return n - 1
	}
	if n > 4 && romEndsWith(s, n, "is") {
		s[n-1] = 'l'
		return n
	}
	if n > 3 && (romEndsWith(s, n, "ões") || romEndsWith(s, n, "ães")) {
		n--
		s[n-2] = 'ã'
		s[n-1] = 'o'
		return n
	}
	if n > 6 && romEndsWith(s, n, "mente") {
		return n - 5
	}
	if n > 3 && s[n-1] == 's' {
		return n - 1
	}
	return n
}

func romPtLightNormFeminine(s []rune, n int) int {
	if n > 7 && (romEndsWith(s, n, "inha") || romEndsWith(s, n, "iaca") || romEndsWith(s, n, "eira")) {
		s[n-1] = 'o'
		return n
	}
	if n > 6 {
		if romEndsWith(s, n, "osa") || romEndsWith(s, n, "ica") || romEndsWith(s, n, "ida") ||
			romEndsWith(s, n, "ada") || romEndsWith(s, n, "iva") || romEndsWith(s, n, "ama") {
			s[n-1] = 'o'
			return n
		}
		if romEndsWith(s, n, "ona") {
			s[n-3] = 'ã'
			s[n-2] = 'o'
			return n - 1
		}
		if romEndsWith(s, n, "ora") {
			return n - 1
		}
		if romEndsWith(s, n, "esa") {
			s[n-3] = 'ê'
			return n - 1
		}
		if romEndsWith(s, n, "na") {
			s[n-1] = 'o'
			return n
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// pt.RSLPStemmerBase and the stemmers built on it.

// romRSLPStepMin resolves a step's minimum length as the Step constructor
// does (a header min of 0 means the minimum of rule.min+len(suffix)).
func romRSLPStepMin(st *rslpStep) int {
	if st.min != 0 {
		return st.min
	}
	m := int(^uint(0) >> 1)
	for i := range st.rules {
		if v := st.rules[i].min + len([]rune(st.rules[i].suffix)); v < m {
			m = v
		}
	}
	return m
}

func romRSLPRuleMatches(r *rslpRule, s []rune, n int) bool {
	if n-len([]rune(r.suffix)) < r.min || !romEndsWith(s, n, r.suffix) {
		return false
	}
	if len(r.exceptWhole) > 0 {
		w := string(s[:n])
		for _, e := range r.exceptWhole {
			if e == w {
				return false
			}
		}
	}
	for _, e := range r.exceptSuffix {
		if romEndsWith(s, n, e) {
			return false
		}
	}
	return true
}

// romRSLPApply ports Step.apply. It returns the (possibly regrown) buffer and
// the new length; a replacement may be longer than the suffix it replaces.
func romRSLPApply(st *rslpStep, s []rune, n int) ([]rune, int) {
	if n < romRSLPStepMin(st) {
		return s, n
	}
	if len(st.suffixes) > 0 {
		found := false
		for _, suf := range st.suffixes {
			if romEndsWith(s, n, suf) {
				found = true
				break
			}
		}
		if !found {
			return s, n
		}
	}
	for i := range st.rules {
		r := &st.rules[i]
		if romRSLPRuleMatches(r, s, n) {
			out := append(append([]rune{}, s[:n-len([]rune(r.suffix))]...), []rune(r.replacement)...)
			return out, len(out)
		}
	}
	return s, n
}

func romRSLPStep(steps []rslpStep, name string) *rslpStep {
	for i := range steps {
		if steps[i].name == name {
			return &steps[i]
		}
	}
	panic("rslp: missing step " + name)
}

// portugueseMinimalStem ports pt.PortugueseMinimalStemmer (RSLP plural step).
func portugueseMinimalStem(term string) string {
	s := []rune(term)
	s, n := romRSLPApply(romRSLPStep(rslpPortuguese, "Plural"), s, len(s))
	return string(s[:n])
}

// galicianMinimalStem ports gl.GalicianMinimalStemmer (RSLP plural step).
func galicianMinimalStem(term string) string {
	s := []rune(term)
	s, n := romRSLPApply(romRSLPStep(rslpGalician, "Plural"), s, len(s))
	return string(s[:n])
}

// portugueseRSLPStem ports pt.PortugueseStemmer (PortugueseStemFilter).
func portugueseRSLPStem(term string) string {
	s := []rune(term)
	n := len(s)
	st := func(name string) {
		s, n = romRSLPApply(romRSLPStep(rslpPortuguese, name), s, n)
	}
	st("Plural")
	st("Adverb")
	st("Feminine")
	st("Augmentative")
	old := n
	st("Noun")
	if n == old {
		st("Verb")
		if n == old {
			st("Vowel")
		}
	}
	for i := 0; i < n; i++ {
		switch s[i] {
		case 'à', 'á', 'â', 'ã', 'ä', 'å':
			s[i] = 'a'
		case 'ç':
			s[i] = 'c'
		case 'è', 'é', 'ê', 'ë':
			s[i] = 'e'
		case 'ì', 'í', 'î', 'ï':
			s[i] = 'i'
		case 'ñ':
			s[i] = 'n'
		case 'ò', 'ó', 'ô', 'õ', 'ö':
			s[i] = 'o'
		case 'ù', 'ú', 'û', 'ü':
			s[i] = 'u'
		case 'ý', 'ÿ':
			s[i] = 'y'
		}
	}
	return string(s[:n])
}

// galicianRSLPStem ports gl.GalicianStemmer (GalicianStemFilter).
func galicianRSLPStem(term string) string {
	s := []rune(term)
	n := len(s)
	st := func(name string) {
		s, n = romRSLPApply(romRSLPStep(rslpGalician, name), s, n)
	}
	st("Plural")
	st("Unification")
	st("Adverb")
	for {
		old := n
		st("Augmentative")
		if n == old {
			break
		}
	}
	old := n
	st("Noun")
	if n == old {
		st("Verb")
	}
	st("Vowel")
	for i := 0; i < n; i++ {
		switch s[i] {
		case 'á':
			s[i] = 'a'
		case 'é', 'ê':
			s[i] = 'e'
		case 'í':
			s[i] = 'i'
		case 'ó':
			s[i] = 'o'
		case 'ú':
			s[i] = 'u'
		}
	}
	return string(s[:n])
}

// ---------------------------------------------------------------------------
// br.BrazilianStemmer as applied by br.BrazilianStemFilter.

const (
	romBrR1 = iota
	romBrR2
	romBrRV
)

const (
	romBrRemove = iota
	romBrReplace
	romBrNoop // replaceSuffix whose result Lucene discards ("logias")
)

// romBrRule is one "if (suffix(..)) { CT = ...; return true; }" branch of
// BrazilianStemmer.step1/step2. minLen is the enclosing length guard (on CT
// for step1, on RV for step2).
type romBrRule struct {
	minLen   int
	suffix   string
	region   int
	preceded string
	op       int
	from, to string
}

// romBrRegion is a nullable Java String.
type romBrRegion struct {
	s  string
	ok bool
}

func romBrIsVowel(c rune) bool {
	return c == 'a' || c == 'e' || c == 'i' || c == 'o' || c == 'u'
}

func romBrGetR1(v romBrRegion) romBrRegion {
	if !v.ok {
		return v
	}
	r := []rune(v.s)
	i := len(r) - 1
	j := 0
	for ; j < i; j++ {
		if romBrIsVowel(r[j]) {
			break
		}
	}
	if !(j < i) {
		return romBrRegion{}
	}
	for ; j < i; j++ {
		if !romBrIsVowel(r[j]) {
			break
		}
	}
	if !(j < i) {
		return romBrRegion{}
	}
	return romBrRegion{string(r[j+1:]), true}
}

func romBrGetRV(value string) romBrRegion {
	r := []rune(value)
	i := len(r) - 1
	if i > 0 && !romBrIsVowel(r[1]) {
		j := 2
		for ; j < i; j++ {
			if romBrIsVowel(r[j]) {
				break
			}
		}
		if j < i {
			return romBrRegion{string(r[j+1:]), true}
		}
	}
	if i > 1 && romBrIsVowel(r[0]) && romBrIsVowel(r[1]) {
		j := 2
		for ; j < i; j++ {
			if !romBrIsVowel(r[j]) {
				break
			}
		}
		if j < i {
			return romBrRegion{string(r[j+1:]), true}
		}
	}
	if i > 2 {
		return romBrRegion{string(r[3:]), true}
	}
	return romBrRegion{}
}

func romBrSuffix(v romBrRegion, suf string) bool {
	return v.ok && strings.HasSuffix(v.s, suf)
}

func romBrRemoveSuffix(v, suf string) string {
	if !strings.HasSuffix(v, suf) {
		return v
	}
	return v[:len(v)-len(suf)]
}

func romBrReplaceSuffix(v, from, to string) string {
	w := romBrRemoveSuffix(v, from)
	if w == v {
		return v
	}
	return w + to
}

func romBrSuffixPreceded(v romBrRegion, suf, pre string) bool {
	if !romBrSuffix(v, suf) {
		return false
	}
	return strings.HasSuffix(romBrRemoveSuffix(v.s, suf), pre)
}

// romJavaLower approximates Java's String.toLowerCase(Locale) for a non-Turkic
// locale: per-code-point lowercase plus the two context/length special cases
// (U+0130 -> "i̇" and Greek final sigma).
func romJavaLower(term string) string {
	r := []rune(term)
	var b strings.Builder
	for i, c := range r {
		switch c {
		case 'İ':
			b.WriteString("i̇")
		case 'Σ':
			if romFinalSigma(r, i) {
				b.WriteRune('ς')
			} else {
				b.WriteRune('σ')
			}
		default:
			b.WriteRune(unicode.ToLower(c))
		}
	}
	return b.String()
}

func romCaseIgnorable(c rune) bool {
	switch c {
	case '\'', '.', ':', '^', '`', 0x00A8, 0x00AD, 0x00AF, 0x00B4, 0x00B7, 0x00B8, 0x2018, 0x2019, 0x2024, 0x2027:
		return true
	}
	return unicode.In(c, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk)
}

func romCased(c rune) bool {
	return unicode.IsUpper(c) || unicode.IsLower(c) || unicode.IsTitle(c) ||
		unicode.In(c, unicode.Other_Lowercase, unicode.Other_Uppercase)
}

func romFinalSigma(r []rune, i int) bool {
	before := false
	for j := i - 1; j >= 0; j-- {
		if romCaseIgnorable(r[j]) {
			continue
		}
		before = romCased(r[j])
		break
	}
	if !before {
		return false
	}
	for j := i + 1; j < len(r); j++ {
		if romCaseIgnorable(r[j]) {
			continue
		}
		return !romCased(r[j])
	}
	return true
}

func romBrChangeTerm(value string) string {
	value = romJavaLower(value)
	var b strings.Builder
	for _, c := range value {
		switch c {
		case 'á', 'â', 'ã':
			b.WriteByte('a')
		case 'é', 'ê':
			b.WriteByte('e')
		case 'í':
			b.WriteByte('i')
		case 'ó', 'ô', 'õ':
			b.WriteByte('o')
		case 'ú', 'ü':
			b.WriteByte('u')
		case 'ç':
			b.WriteByte('c')
		case 'ñ':
			b.WriteByte('n')
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// romUTF16Len is Java's String.length().
func romUTF16Len(s string) int {
	n := 0
	for _, c := range s {
		n += len(utf16.Encode([]rune{c}))
	}
	return n
}

func romBrCreateCT(term string) string {
	ct := romBrChangeTerm(term)
	if romUTF16Len(ct) < 2 {
		return ct
	}
	switch ct[0] {
	case '"', '\'', '-', ',', ';', '.', '?', '!':
		ct = ct[1:]
	}
	if romUTF16Len(ct) < 2 {
		return ct
	}
	switch ct[len(ct)-1] {
	case '-', ',', ';', '.', '?', '!', '\'', '"':
		ct = ct[:len(ct)-1]
	}
	return ct
}

// romBrStem ports BrazilianStemmer.stem; ok=false stands for Java's null.
func romBrStem(term string) (string, bool) {
	ct := romBrCreateCT(term)
	if l := romUTF16Len(ct); !(l < 30 && l > 2) {
		return "", false
	}
	for _, c := range ct {
		// Character.isLetter(char): a supplementary character is two
		// surrogates, neither of which is a letter.
		if c > 0xFFFF || !unicode.IsLetter(c) {
			return ct, true
		}
	}
	ctR := romBrRegion{ct, true}
	r1 := romBrGetR1(ctR)
	regions := [3]romBrRegion{r1, romBrGetR1(r1), romBrGetRV(ct)}
	rv := regions[romBrRV]

	apply := func(r *romBrRule) {
		switch r.op {
		case romBrRemove:
			ct = romBrRemoveSuffix(ct, r.from)
		case romBrReplace:
			ct = romBrReplaceSuffix(ct, r.from, r.to)
		}
	}
	altered := false
	ctLen := len([]rune(ct))
	for i := range romBrStep1 {
		r := &romBrStep1[i]
		if ctLen >= r.minLen && strings.HasSuffix(ct, r.suffix) && romBrSuffix(regions[r.region], r.suffix) &&
			(r.preceded == "" || romBrSuffixPreceded(ctR, r.suffix, r.preceded)) {
			apply(r)
			altered = true
			break
		}
	}
	if !altered && rv.ok {
		rvLen := len([]rune(rv.s))
		for i := range romBrStep2 {
			r := &romBrStep2[i]
			if rvLen >= r.minLen && strings.HasSuffix(rv.s, r.suffix) {
				apply(r)
				altered = true
				break
			}
		}
	}
	// Steps 3-5 test the RV computed before steps 1/2 (Lucene never updates it).
	if rv.ok {
		if altered {
			if romBrSuffix(rv, "i") && romBrSuffixPreceded(rv, "i", "c") {
				ct = romBrRemoveSuffix(ct, "i")
			}
		} else {
			for _, suf := range []string{"os", "a", "i", "o"} {
				if romBrSuffix(rv, suf) {
					ct = romBrRemoveSuffix(ct, suf)
					break
				}
			}
		}
		if romBrSuffix(rv, "e") {
			if romBrSuffixPreceded(rv, "e", "gu") {
				ct = romBrRemoveSuffix(romBrRemoveSuffix(ct, "e"), "u")
			} else if romBrSuffixPreceded(rv, "e", "ci") {
				ct = romBrRemoveSuffix(romBrRemoveSuffix(ct, "e"), "i")
			} else {
				ct = romBrRemoveSuffix(ct, "e")
			}
		}
	}
	return ct, true
}

// brazilianStem reproduces BrazilianStemFilter on one non-keyword token: the
// term is replaced only when the stemmer returns a non-null, different value.
func brazilianStem(term string) string {
	s, ok := romBrStem(term)
	if !ok || s == term {
		return term
	}
	return s
}

// ---------------------------------------------------------------------------
// ro.RomanianNormalizationFilter (applies to keyword tokens too).

func romanianNormalize(term string) string {
	return strings.Map(func(c rune) rune {
		switch c {
		case 'Ş':
			return 'Ș'
		case 'ş':
			return 'ș'
		case 'Ţ':
			return 'Ț'
		case 'ţ':
			return 'ț'
		}
		return c
	}, term)
}
