package main

import (
	"unicode"
)

// ---------------------------------------------------------------------------
// German (old, Caumanns-style) stemmer: org.apache.lucene.analysis.de.GermanStemmer
// as applied by GermanStemFilter.

// germanStemOld ports org.apache.lucene.analysis.de.GermanStemmer.stem (GermanStemFilter).
// The term is lowercased (Java String.toLowerCase semantics) and returned
// unstemmed if it contains a non-letter.
func germanStemOld(term string) string {
	lower := gmcJavaLowerCase(term)
	for _, c := range lower {
		// Java checks UTF-16 chars: a supplementary code point is a surrogate pair, never a letter.
		if c > 0xFFFF || !unicode.IsLetter(c) {
			return string(lower)
		}
	}
	g := &gmcGermanStemmer{sb: append([]rune(nil), lower...)}
	g.substitute()
	g.strip()
	g.optimize()
	g.resubstitute()
	g.removeParticleDenotion()
	return string(g.sb)
}

type gmcGermanStemmer struct {
	sb         []rune
	substCount int
}

func (g *gmcGermanStemmer) endsWith2(a, b rune) bool {
	n := len(g.sb)
	return g.sb[n-2] == a && g.sb[n-1] == b
}

func (g *gmcGermanStemmer) strip() {
	for len(g.sb) > 3 {
		n := len(g.sb)
		switch {
		case n+g.substCount > 5 && g.endsWith2('n', 'd'):
			g.sb = g.sb[:n-2]
		case n+g.substCount > 4 && g.endsWith2('e', 'm'):
			g.sb = g.sb[:n-2]
		case n+g.substCount > 4 && g.endsWith2('e', 'r'):
			g.sb = g.sb[:n-2]
		case g.sb[n-1] == 'e', g.sb[n-1] == 's', g.sb[n-1] == 'n', g.sb[n-1] == 't':
			g.sb = g.sb[:n-1]
		default:
			return
		}
	}
}

func (g *gmcGermanStemmer) optimize() {
	n := len(g.sb)
	if n > 5 && string(g.sb[n-5:]) == "erin*" {
		g.sb = g.sb[:n-1]
		g.strip()
	}
	n = len(g.sb)
	if n > 0 && g.sb[n-1] == 'z' {
		g.sb[n-1] = 'x'
	}
}

func (g *gmcGermanStemmer) removeParticleDenotion() {
	b := g.sb
	if len(b) > 4 {
		for c := 0; c < len(b)-3; c++ {
			if b[c] == 'g' && b[c+1] == 'e' && b[c+2] == 'g' && b[c+3] == 'e' {
				g.sb = append(b[:c:c], b[c+2:]...)
				return
			}
		}
	}
}

func gmcRuneInsert(b []rune, pos int, rs ...rune) []rune {
	out := make([]rune, 0, len(b)+len(rs))
	out = append(out, b[:pos]...)
	out = append(out, rs...)
	return append(out, b[pos:]...)
}

func gmcRuneDelete(b []rune, from, to int) []rune {
	return append(b[:from], b[to:]...)
}

func (g *gmcGermanStemmer) substitute() {
	g.substCount = 0
	b := g.sb
	for c := 0; c < len(b); c++ {
		switch {
		case c > 0 && b[c] == b[c-1]:
			b[c] = '*'
		case b[c] == 'ä':
			b[c] = 'a'
		case b[c] == 'ö':
			b[c] = 'o'
		case b[c] == 'ü':
			b[c] = 'u'
		case b[c] == 'ß':
			b[c] = 's'
			b = gmcRuneInsert(b, c+1, 's')
			g.substCount++
		}
		if c < len(b)-1 {
			switch {
			case c < len(b)-2 && b[c] == 's' && b[c+1] == 'c' && b[c+2] == 'h':
				b[c] = '$'
				b = gmcRuneDelete(b, c+1, c+3)
				g.substCount += 2
			case b[c] == 'c' && b[c+1] == 'h':
				b[c] = '§'
				b = gmcRuneDelete(b, c+1, c+2)
				g.substCount++
			case b[c] == 'e' && b[c+1] == 'i':
				b[c] = '%'
				b = gmcRuneDelete(b, c+1, c+2)
				g.substCount++
			case b[c] == 'i' && b[c+1] == 'e':
				b[c] = '&'
				b = gmcRuneDelete(b, c+1, c+2)
				g.substCount++
			case b[c] == 'i' && b[c+1] == 'g':
				b[c] = '#'
				b = gmcRuneDelete(b, c+1, c+2)
				g.substCount++
			case b[c] == 's' && b[c+1] == 't':
				b[c] = '!'
				b = gmcRuneDelete(b, c+1, c+2)
				g.substCount++
			}
		}
	}
	g.sb = b
}

func (g *gmcGermanStemmer) resubstitute() {
	b := g.sb
	for c := 0; c < len(b); c++ {
		switch b[c] {
		case '*':
			b[c] = b[c-1]
		case '$':
			b[c] = 's'
			b = gmcRuneInsert(b, c+1, 'c', 'h')
		case '§':
			b[c] = 'c'
			b = gmcRuneInsert(b, c+1, 'h')
		case '%':
			b[c] = 'e'
			b = gmcRuneInsert(b, c+1, 'i')
		case '&':
			b[c] = 'i'
			b = gmcRuneInsert(b, c+1, 'e')
		case '#':
			b[c] = 'i'
			b = gmcRuneInsert(b, c+1, 'g')
		case '!':
			b[c] = 's'
			b = gmcRuneInsert(b, c+1, 't')
		}
	}
	g.sb = b
}

// gmcJavaLowerCase mirrors java.lang.String.toLowerCase(Locale) for a
// non-Turkic/Lithuanian locale: per-code-point lowercase, with U+0130 mapped
// to "i̇" and Greek capital sigma mapped to final sigma at word end.
func gmcJavaLowerCase(s string) []rune {
	in := []rune(s)
	out := make([]rune, 0, len(in))
	for i, c := range in {
		switch c {
		case 'İ':
			out = append(out, 'i', '̇')
		case 'Σ':
			if gmcJavaFinalSigma(in, i) {
				out = append(out, 'ς')
			} else {
				out = append(out, 'σ')
			}
		default:
			out = append(out, unicode.ToLower(c))
		}
	}
	return out
}

// gmcJavaFinalSigma ports ConditionalSpecialCasing.isFinalCased: preceded by a
// cased letter and not followed by one (skipping case-ignorable characters).
func gmcJavaFinalSigma(s []rune, idx int) bool {
	i := idx - 1
	for i >= 0 && gmcJavaCaseIgnorable(s[i]) {
		i--
	}
	if i < 0 || !gmcJavaCased(s[i]) {
		return false
	}
	j := idx + 1
	for j < len(s) && gmcJavaCaseIgnorable(s[j]) {
		j++
	}
	return j >= len(s) || !gmcJavaCased(s[j])
}

func gmcJavaCased(c rune) bool {
	return unicode.IsLower(c) || unicode.IsUpper(c) || unicode.IsTitle(c) ||
		unicode.Is(unicode.Other_Lowercase, c) || unicode.Is(unicode.Other_Uppercase, c)
}

func gmcJavaCaseIgnorable(c rune) bool {
	switch c {
	case '\'', '­', '’':
		return true
	}
	return unicode.In(c, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk)
}

// ---------------------------------------------------------------------------
// Porter stemmer: org.apache.lucene.analysis.en.PorterStemmer as applied by
// PorterStemFilter.

type gmcPorter struct {
	b        []rune
	j, k, k0 int
}

// porterStemLucene ports org.apache.lucene.analysis.en.PorterStemmer (PorterStemFilter).
func porterStemLucene(term string) string {
	p := &gmcPorter{b: []rune(term)}
	p.k = len(p.b) - 1
	p.k0 = 0
	if p.k > p.k0+1 {
		p.step1()
		p.step2()
		p.step3()
		p.step4()
		p.step5()
		p.step6()
	}
	return string(p.b[:p.k+1])
}

func (p *gmcPorter) cons(i int) bool {
	switch p.b[i] {
	case 'a', 'e', 'i', 'o', 'u':
		return false
	case 'y':
		if i == p.k0 {
			return true
		}
		return !p.cons(i - 1)
	}
	return true
}

func (p *gmcPorter) m() int {
	n := 0
	i := p.k0
	for {
		if i > p.j {
			return n
		}
		if !p.cons(i) {
			break
		}
		i++
	}
	i++
	for {
		for {
			if i > p.j {
				return n
			}
			if p.cons(i) {
				break
			}
			i++
		}
		i++
		n++
		for {
			if i > p.j {
				return n
			}
			if !p.cons(i) {
				break
			}
			i++
		}
		i++
	}
}

func (p *gmcPorter) vowelinstem() bool {
	for i := p.k0; i <= p.j; i++ {
		if !p.cons(i) {
			return true
		}
	}
	return false
}

func (p *gmcPorter) doublec(j int) bool {
	if j < p.k0+1 {
		return false
	}
	if p.b[j] != p.b[j-1] {
		return false
	}
	return p.cons(j)
}

func (p *gmcPorter) cvc(i int) bool {
	if i < p.k0+2 || !p.cons(i) || p.cons(i-1) || !p.cons(i-2) {
		return false
	}
	ch := p.b[i]
	return ch != 'w' && ch != 'x' && ch != 'y'
}

// ends works on ASCII suffixes only (all Porter suffixes are ASCII).
func (p *gmcPorter) ends(s string) bool {
	l := len(s)
	o := p.k - l + 1
	if o < p.k0 {
		return false
	}
	for i := 0; i < l; i++ {
		if p.b[o+i] != rune(s[i]) {
			return false
		}
	}
	p.j = p.k - l
	return true
}

func (p *gmcPorter) setto(s string) {
	l := len(s)
	o := p.j + 1
	for i := 0; i < l; i++ {
		p.b[o+i] = rune(s[i])
	}
	p.k = p.j + l
}

func (p *gmcPorter) r(s string) {
	if p.m() > 0 {
		p.setto(s)
	}
}

func (p *gmcPorter) step1() {
	if p.b[p.k] == 's' {
		if p.ends("sses") {
			p.k -= 2
		} else if p.ends("ies") {
			p.setto("i")
		} else if p.b[p.k-1] != 's' {
			p.k--
		}
	}
	if p.ends("eed") {
		if p.m() > 0 {
			p.k--
		}
	} else if (p.ends("ed") || p.ends("ing")) && p.vowelinstem() {
		p.k = p.j
		if p.ends("at") {
			p.setto("ate")
		} else if p.ends("bl") {
			p.setto("ble")
		} else if p.ends("iz") {
			p.setto("ize")
		} else if p.doublec(p.k) {
			ch := p.b[p.k]
			p.k--
			if ch == 'l' || ch == 's' || ch == 'z' {
				p.k++
			}
		} else if p.m() == 1 && p.cvc(p.k) {
			p.setto("e")
		}
	}
}

func (p *gmcPorter) step2() {
	if p.ends("y") && p.vowelinstem() {
		p.b[p.k] = 'i'
	}
}

// gmcPorterRules is a list of (suffix, replacement) pairs tried in order; the
// first matching suffix wins (and r() is applied), as in the Java switch.
type gmcPorterRule struct{ suf, rep string }

var gmcPorterStep3 = map[rune][]gmcPorterRule{
	'a': {{"ational", "ate"}, {"tional", "tion"}},
	'c': {{"enci", "ence"}, {"anci", "ance"}},
	'e': {{"izer", "ize"}},
	'l': {{"bli", "ble"}, {"alli", "al"}, {"entli", "ent"}, {"eli", "e"}, {"ousli", "ous"}},
	'o': {{"ization", "ize"}, {"ation", "ate"}, {"ator", "ate"}},
	's': {{"alism", "al"}, {"iveness", "ive"}, {"fulness", "ful"}, {"ousness", "ous"}},
	't': {{"aliti", "al"}, {"iviti", "ive"}, {"biliti", "ble"}},
	'g': {{"logi", "log"}},
}

var gmcPorterStep4 = map[rune][]gmcPorterRule{
	'e': {{"icate", "ic"}, {"ative", ""}, {"alize", "al"}},
	'i': {{"iciti", "ic"}},
	'l': {{"ical", "ic"}, {"ful", ""}},
	's': {{"ness", ""}},
}

func (p *gmcPorter) applyRules(rules []gmcPorterRule) {
	for _, ru := range rules {
		if p.ends(ru.suf) {
			p.r(ru.rep)
			return
		}
	}
}

func (p *gmcPorter) step3() {
	if p.k == p.k0 {
		return
	}
	p.applyRules(gmcPorterStep3[p.b[p.k-1]])
}

func (p *gmcPorter) step4() {
	p.applyRules(gmcPorterStep4[p.b[p.k]])
}

func (p *gmcPorter) step5() {
	if p.k == p.k0 {
		return
	}
	ok := false
	switch p.b[p.k-1] {
	case 'a':
		ok = p.ends("al")
	case 'c':
		ok = p.ends("ance") || p.ends("ence")
	case 'e':
		ok = p.ends("er")
	case 'i':
		ok = p.ends("ic")
	case 'l':
		ok = p.ends("able") || p.ends("ible")
	case 'n':
		ok = p.ends("ant") || p.ends("ement") || p.ends("ment") || p.ends("ent")
	case 'o':
		ok = (p.ends("ion") && p.j >= 0 && (p.b[p.j] == 's' || p.b[p.j] == 't')) || p.ends("ou")
	case 's':
		ok = p.ends("ism")
	case 't':
		ok = p.ends("ate") || p.ends("iti")
	case 'u':
		ok = p.ends("ous")
	case 'v':
		ok = p.ends("ive")
	case 'z':
		ok = p.ends("ize")
	}
	if ok && p.m() > 1 {
		p.k = p.j
	}
}

func (p *gmcPorter) step6() {
	p.j = p.k
	if p.b[p.k] == 'e' {
		a := p.m()
		if a > 1 || a == 1 && !p.cvc(p.k-1) {
			p.k--
		}
	}
	if p.b[p.k] == 'l' && p.doublec(p.k) && p.m() > 1 {
		p.k--
	}
}
