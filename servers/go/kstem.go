package main

// Port of Lucene 10.5.1 org.apache.lucene.analysis.en.KStemmer as applied by
// org.apache.lucene.analysis.en.KStemFilter (Krovetz stemmer). The dictionary
// tables live in kstem_data_generated.go.

import (
	"sync"
	"unicode/utf16"
)

const ksMaxWordLen = 50

type ksDictEntry struct {
	exception bool
	root      string
	hasRoot   bool // Java root != null
}

var (
	ksDictOnce sync.Once
	ksDict     map[string]*ksDictEntry
)

func ksDictionary() map[string]*ksDictEntry {
	ksDictOnce.Do(func() {
		n := len(ksExceptionWords) + len(ksDirectConflations) + len(ksCountryNationality) +
			len(ksSupplementDict) + len(ksProperNouns)
		for _, d := range [][]string{ksData1, ksData2, ksData3, ksData4, ksData5, ksData6, ksData7, ksData8} {
			n += len(d)
		}
		d := make(map[string]*ksDictEntry, n)
		put := func(key string, e *ksDictEntry) {
			// Lucene throws on duplicates; the shipped tables have none.
			if _, ok := d[key]; !ok {
				d[key] = e
			}
		}
		for _, w := range ksExceptionWords {
			put(w, &ksDictEntry{exception: true, root: w, hasRoot: true})
		}
		for _, p := range ksDirectConflations {
			put(p[0], &ksDictEntry{root: p[1], hasRoot: true})
		}
		for _, p := range ksCountryNationality {
			put(p[0], &ksDictEntry{root: p[1], hasRoot: true})
		}
		def := &ksDictEntry{}
		for _, data := range [][]string{ksData1, ksData2, ksData3, ksData4, ksData5, ksData6, ksData7, ksData8, ksSupplementDict, ksProperNouns} {
			for _, w := range data {
				put(w, def)
			}
		}
		ksDict = d
	})
	return ksDict
}

// ksStemmer mirrors the per-call state of KStemmer. word/wlen model Lucene's
// OpenStringBuilder: setLength only moves wlen, so stale chars beyond it stay.
type ksStemmer struct {
	dict    map[string]*ksDictEntry
	word    []byte
	wlen    int
	j, k    int
	matched *ksDictEntry
}

// kstemLucene returns what KStemFilter emits for one non-keyword token.
func kstemLucene(term string) string {
	length := 0 // UTF-16 length, as Java sees it
	ascii := true
	for i := 0; i < len(term); i++ {
		if term[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		length = len(term)
	} else {
		length = len(utf16.Encode([]rune(term)))
	}
	k := length - 1
	if k <= 1 || k >= ksMaxWordLen-1 {
		return term
	}
	dict := ksDictionary()
	if e, ok := dict[term]; ok {
		if e.hasRoot {
			return e.root
		}
		return term
	}
	if !ascii {
		return term // non a-z char: don't stem
	}
	s := ksStemmer{dict: dict, word: make([]byte, length+16), k: k}
	for i := 0; i < length; i++ {
		ch := term[i]
		if ch < 'a' || ch > 'z' {
			return term
		}
		s.word[i] = ch
	}
	s.wlen = length
	return s.run()
}

func (s *ksStemmer) run() string {
	for {
		s.plural()
		if s.matched != nil {
			break
		}
		s.pastTense()
		if s.matched != nil {
			break
		}
		s.aspect()
		if s.matched != nil {
			break
		}
		s.ityEndings()
		if s.matched != nil {
			break
		}
		s.nessEndings()
		if s.matched != nil {
			break
		}
		s.ionEndings()
		if s.matched != nil {
			break
		}
		s.erAndOrEndings()
		if s.matched != nil {
			break
		}
		s.lyEndings()
		if s.matched != nil {
			break
		}
		s.alEndings()
		if s.matched != nil {
			break
		}
		s.wordInDict()
		s.iveEndings()
		if s.matched != nil {
			break
		}
		s.izeEndings()
		if s.matched != nil {
			break
		}
		s.mentEndings()
		if s.matched != nil {
			break
		}
		s.bleEndings()
		if s.matched != nil {
			break
		}
		s.ismEndings()
		if s.matched != nil {
			break
		}
		s.icEndings()
		if s.matched != nil {
			break
		}
		s.ncyEndings()
		if s.matched != nil {
			break
		}
		s.nceEndings()
		break
	}
	if s.matched != nil && s.matched.hasRoot {
		return s.matched.root
	}
	return string(s.word[:s.wlen])
}

// --- OpenStringBuilder helpers ---

func (s *ksStemmer) setLength(n int) { s.wlen = n }

func (s *ksStemmer) charAt(i int) byte { return s.word[i] }

func (s *ksStemmer) setCharAt(i int, c byte) { s.word[i] = c }

func (s *ksStemmer) write(c byte) {
	if s.wlen >= len(s.word) {
		nb := make([]byte, 2*len(s.word)+1)
		copy(nb, s.word)
		s.word = nb
	}
	s.word[s.wlen] = c
	s.wlen++
}

func (s *ksStemmer) appendStr(str string) {
	for i := 0; i < len(str); i++ {
		s.write(str[i])
	}
}

// --- KStemmer internals ---

func (s *ksStemmer) penultChar() byte { return s.word[s.k-1] }

func (s *ksStemmer) isCons(index int) bool {
	ch := s.word[index]
	if ch == 'a' || ch == 'e' || ch == 'i' || ch == 'o' || ch == 'u' {
		return false
	}
	if ch != 'y' || index == 0 {
		return true
	}
	return !s.isCons(index - 1)
}

func (s *ksStemmer) isVowel(index int) bool { return !s.isCons(index) }

func (s *ksStemmer) stemLength() int { return s.j + 1 }

func (s *ksStemmer) endsInStr(suf string) bool {
	if len(suf) > s.k {
		return false
	}
	r := s.wlen - len(suf)
	s.j = s.k
	for i := 0; i < len(suf); i++ {
		if suf[i] != s.word[r+i] {
			return false
		}
	}
	s.j = r - 1
	return true
}

func (s *ksStemmer) endsIn2(a, b byte) bool {
	if 2 > s.k {
		return false
	}
	if s.word[s.k-1] == a && s.word[s.k] == b {
		s.j = s.k - 2
		return true
	}
	return false
}

func (s *ksStemmer) endsIn3(a, b, c byte) bool {
	if 3 > s.k {
		return false
	}
	if s.word[s.k-2] == a && s.word[s.k-1] == b && s.word[s.k] == c {
		s.j = s.k - 3
		return true
	}
	return false
}

func (s *ksStemmer) endsIn4(a, b, c, d byte) bool {
	if 4 > s.k {
		return false
	}
	if s.word[s.k-3] == a && s.word[s.k-2] == b && s.word[s.k-1] == c && s.word[s.k] == d {
		s.j = s.k - 4
		return true
	}
	return false
}

func (s *ksStemmer) wordInDict() *ksDictEntry {
	if s.matched != nil {
		return s.matched
	}
	e := s.dict[string(s.word[:s.wlen])]
	if e != nil && !e.exception {
		s.matched = e
	}
	return e
}

func (s *ksStemmer) lookup() bool {
	s.matched = s.dict[string(s.word[:s.wlen])]
	return s.matched != nil
}

func (s *ksStemmer) setSuffix(str string) {
	s.setLength(s.j + 1)
	s.appendStr(str)
	s.k = s.j + len(str)
}

func (s *ksStemmer) plural() {
	if s.word[s.k] != 's' {
		return
	}
	if s.endsIn3('i', 'e', 's') {
		s.setLength(s.j + 3)
		s.k--
		if s.lookup() {
			return
		}
		s.k++
		s.write('s')
		s.setSuffix("y")
		s.lookup()
	} else if s.endsIn2('e', 's') {
		s.setLength(s.j + 2)
		s.k--
		tryE := s.j > 0 && !(s.word[s.j] == 's' && s.word[s.j-1] == 's')
		if tryE && s.lookup() {
			return
		}
		s.setLength(s.j + 1)
		s.k--
		if s.lookup() {
			return
		}
		s.write('e')
		s.k++
		if !tryE {
			s.lookup()
		}
		return
	} else {
		if s.wlen > 3 && s.penultChar() != 's' && !s.endsIn3('o', 'u', 's') {
			s.setLength(s.k)
			s.k--
			s.lookup()
		}
	}
}

func (s *ksStemmer) pastTense() {
	if s.wlen <= 4 {
		return
	}
	if s.endsIn3('i', 'e', 'd') {
		s.setLength(s.j + 3)
		s.k--
		if s.lookup() {
			return
		}
		s.k++
		s.write('d')
		s.setSuffix("y")
		s.lookup()
		return
	}
	if s.endsIn2('e', 'd') && s.vowelInStem() {
		s.setLength(s.j + 2)
		s.k = s.j + 1
		if e := s.wordInDict(); e != nil && !e.exception {
			return
		}
		s.setLength(s.j + 1)
		s.k = s.j
		if s.lookup() {
			return
		}
		if s.doubleC(s.k) {
			s.setLength(s.k)
			s.k--
			if s.lookup() {
				return
			}
			s.write(s.word[s.k])
			s.k++
			s.lookup()
			return
		}
		if s.word[0] == 'u' && s.word[1] == 'n' {
			s.write('e')
			s.write('d')
			s.k += 2
			return
		}
		s.setLength(s.j + 1)
		s.write('e')
		s.k = s.j + 1
		return
	}
}

func (s *ksStemmer) doubleC(i int) bool {
	if i < 1 {
		return false
	}
	if s.word[i] != s.word[i-1] {
		return false
	}
	return s.isCons(i)
}

func (s *ksStemmer) vowelInStem() bool {
	for i := 0; i < s.stemLength(); i++ {
		if s.isVowel(i) {
			return true
		}
	}
	return false
}

func (s *ksStemmer) aspect() {
	if s.wlen <= 5 {
		return
	}
	if s.endsIn3('i', 'n', 'g') && s.vowelInStem() {
		s.setCharAt(s.j+1, 'e')
		s.setLength(s.j + 2)
		s.k = s.j + 1
		if e := s.wordInDict(); e != nil && !e.exception {
			return
		}
		s.setLength(s.k)
		s.k--
		if s.lookup() {
			return
		}
		if s.doubleC(s.k) {
			s.k--
			s.setLength(s.k + 1)
			if s.lookup() {
				return
			}
			s.write(s.word[s.k])
			s.k++
			s.lookup()
			return
		}
		if s.j > 0 && s.isCons(s.j) && s.isCons(s.j-1) {
			s.k = s.j
			s.setLength(s.k + 1)
			return
		}
		s.setLength(s.j + 1)
		s.write('e')
		s.k = s.j + 1
		return
	}
}

func (s *ksStemmer) ityEndings() {
	oldK := s.k
	if s.endsIn3('i', 't', 'y') {
		s.setLength(s.j + 1)
		s.k = s.j
		if s.lookup() {
			return
		}
		s.write('e')
		s.k = s.j + 1
		if s.lookup() {
			return
		}
		s.setCharAt(s.j+1, 'i')
		s.appendStr("ty")
		s.k = oldK
		if s.j > 0 && s.word[s.j-1] == 'i' && s.word[s.j] == 'l' {
			s.setLength(s.j - 1)
			s.appendStr("le")
			s.k = s.j
			s.lookup()
			return
		}
		if s.j > 0 && s.word[s.j-1] == 'i' && s.word[s.j] == 'v' {
			s.setLength(s.j + 1)
			s.write('e')
			s.k = s.j + 1
			s.lookup()
			return
		}
		if s.j > 0 && s.word[s.j-1] == 'a' && s.word[s.j] == 'l' {
			s.setLength(s.j + 1)
			s.k = s.j
			s.lookup()
			return
		}
		if s.lookup() {
			return
		}
		s.setLength(s.j + 1)
		s.k = s.j
	}
}

func (s *ksStemmer) nceEndings() {
	oldK := s.k
	if s.endsIn3('n', 'c', 'e') {
		wc := s.word[s.j]
		if !(wc == 'e' || wc == 'a') {
			return
		}
		s.setLength(s.j)
		s.write('e')
		s.k = s.j
		if s.lookup() {
			return
		}
		s.setLength(s.j)
		s.k = s.j - 1
		if s.lookup() {
			return
		}
		s.write(wc)
		s.appendStr("nce")
		s.k = oldK
	}
}

func (s *ksStemmer) nessEndings() {
	if s.endsIn4('n', 'e', 's', 's') {
		s.setLength(s.j + 1)
		s.k = s.j
		if s.word[s.j] == 'i' {
			s.setCharAt(s.j, 'y')
		}
		s.lookup()
	}
}

func (s *ksStemmer) ismEndings() {
	if s.endsIn3('i', 's', 'm') {
		s.setLength(s.j + 1)
		s.k = s.j
		s.lookup()
	}
}

func (s *ksStemmer) mentEndings() {
	oldK := s.k
	if s.endsIn4('m', 'e', 'n', 't') {
		s.setLength(s.j + 1)
		s.k = s.j
		if s.lookup() {
			return
		}
		s.appendStr("ment")
		s.k = oldK
	}
}

func (s *ksStemmer) izeEndings() {
	oldK := s.k
	if s.endsIn3('i', 'z', 'e') {
		s.setLength(s.j + 1)
		s.k = s.j
		if s.lookup() {
			return
		}
		s.write('i')
		if s.doubleC(s.j) {
			s.setLength(s.j)
			s.k = s.j - 1
			if s.lookup() {
				return
			}
			s.write(s.word[s.j-1])
		}
		s.setLength(s.j + 1)
		s.write('e')
		s.k = s.j + 1
		if s.lookup() {
			return
		}
		s.setLength(s.j + 1)
		s.appendStr("ize")
		s.k = oldK
	}
}

func (s *ksStemmer) ncyEndings() {
	if s.endsIn3('n', 'c', 'y') {
		if !(s.word[s.j] == 'e' || s.word[s.j] == 'a') {
			return
		}
		s.setCharAt(s.j+2, 't')
		s.setLength(s.j + 3)
		s.k = s.j + 2
		if s.lookup() {
			return
		}
		s.setCharAt(s.j+2, 'c')
		s.write('e')
		s.k = s.j + 3
		s.lookup()
	}
}

func (s *ksStemmer) bleEndings() {
	oldK := s.k
	if s.endsIn3('b', 'l', 'e') {
		if !(s.word[s.j] == 'a' || s.word[s.j] == 'i') {
			return
		}
		wc := s.word[s.j]
		s.setLength(s.j)
		s.k = s.j - 1
		if s.lookup() {
			return
		}
		if s.doubleC(s.k) {
			s.setLength(s.k)
			s.k--
			if s.lookup() {
				return
			}
			s.k++
			s.write(s.word[s.k-1])
		}
		s.setLength(s.j)
		s.write('e')
		s.k = s.j
		if s.lookup() {
			return
		}
		s.setLength(s.j)
		s.appendStr("ate")
		s.k = s.j + 2
		if s.lookup() {
			return
		}
		s.setLength(s.j)
		s.write(wc)
		s.appendStr("ble")
		s.k = oldK
	}
}

func (s *ksStemmer) icEndings() {
	if s.endsIn2('i', 'c') {
		s.setLength(s.j + 3)
		s.appendStr("al")
		s.k = s.j + 4
		if s.lookup() {
			return
		}
		s.setCharAt(s.j+1, 'y')
		s.setLength(s.j + 2)
		s.k = s.j + 1
		if s.lookup() {
			return
		}
		s.setCharAt(s.j+1, 'e')
		if s.lookup() {
			return
		}
		s.setLength(s.j + 1)
		s.k = s.j
		if s.lookup() {
			return
		}
		s.appendStr("ic")
		s.k = s.j + 2
	}
}

func (s *ksStemmer) ionEndings() {
	oldK := s.k
	if !s.endsIn3('i', 'o', 'n') {
		return
	}
	if s.endsInStr("ization") {
		s.setLength(s.j + 3)
		s.write('e')
		s.k = s.j + 3
		s.lookup()
		return
	}
	if s.endsInStr("ition") {
		s.setLength(s.j + 1)
		s.write('e')
		s.k = s.j + 1
		if s.lookup() {
			return
		}
		s.setLength(s.j + 1)
		s.appendStr("ition")
		s.k = oldK
	} else if s.endsInStr("ation") {
		s.setLength(s.j + 3)
		s.write('e')
		s.k = s.j + 3
		if s.lookup() {
			return
		}
		s.setLength(s.j + 1)
		s.write('e')
		s.k = s.j + 1
		if s.lookup() {
			return
		}
		s.setLength(s.j + 1)
		s.k = s.j
		if s.lookup() {
			return
		}
		s.setLength(s.j + 1)
		s.appendStr("ation")
		s.k = oldK
	}
	if s.endsInStr("ication") {
		s.setLength(s.j + 1)
		s.write('y')
		s.k = s.j + 1
		if s.lookup() {
			return
		}
		s.setLength(s.j + 1)
		s.appendStr("ication")
		s.k = oldK
	}
	s.j = s.k - 3
	s.setLength(s.j + 1)
	s.write('e')
	s.k = s.j + 1
	if s.lookup() {
		return
	}
	s.setLength(s.j + 1)
	s.k = s.j
	if s.lookup() {
		return
	}
	s.setLength(s.j + 1)
	s.appendStr("ion")
	s.k = oldK
}

func (s *ksStemmer) erAndOrEndings() {
	oldK := s.k
	if s.word[s.k] != 'r' {
		return
	}
	if s.endsIn4('i', 'z', 'e', 'r') {
		s.setLength(s.j + 4)
		s.k = s.j + 3
		s.lookup()
		return
	}
	if s.endsIn2('e', 'r') || s.endsIn2('o', 'r') {
		wc := s.word[s.j+1]
		if s.doubleC(s.j) {
			s.setLength(s.j)
			s.k = s.j - 1
			if s.lookup() {
				return
			}
			s.write(s.word[s.j-1])
		}
		if s.word[s.j] == 'i' {
			s.setCharAt(s.j, 'y')
			s.setLength(s.j + 1)
			s.k = s.j
			if s.lookup() {
				return
			}
			s.setCharAt(s.j, 'i')
			s.write('e')
		}
		if s.word[s.j] == 'e' {
			s.setLength(s.j)
			s.k = s.j - 1
			if s.lookup() {
				return
			}
			s.write('e')
		}
		s.setLength(s.j + 2)
		s.k = s.j + 1
		if s.lookup() {
			return
		}
		s.setLength(s.j + 1)
		s.k = s.j
		if s.lookup() {
			return
		}
		s.write('e')
		s.k = s.j + 1
		if s.lookup() {
			return
		}
		s.setLength(s.j + 1)
		s.write(wc)
		s.write('r')
		s.k = oldK
	}
}

func (s *ksStemmer) lyEndings() {
	oldK := s.k
	if s.endsIn2('l', 'y') {
		s.setCharAt(s.j+2, 'e')
		if s.lookup() {
			return
		}
		s.setCharAt(s.j+2, 'y')
		s.setLength(s.j + 1)
		s.k = s.j
		if s.lookup() {
			return
		}
		if s.j > 0 && s.word[s.j-1] == 'a' && s.word[s.j] == 'l' {
			return
		}
		s.appendStr("ly")
		s.k = oldK
		if s.j > 0 && s.word[s.j-1] == 'a' && s.word[s.j] == 'b' {
			s.setCharAt(s.j+2, 'e')
			s.k = s.j + 2
			return
		}
		if s.word[s.j] == 'i' {
			s.setLength(s.j)
			s.write('y')
			s.k = s.j
			if s.lookup() {
				return
			}
			s.setLength(s.j)
			s.appendStr("ily")
			s.k = oldK
		}
		s.setLength(s.j + 1)
		s.k = s.j
	}
}

func (s *ksStemmer) alEndings() {
	oldK := s.k
	if s.wlen < 4 {
		return
	}
	if s.endsIn2('a', 'l') {
		s.setLength(s.j + 1)
		s.k = s.j
		if s.lookup() {
			return
		}
		if s.doubleC(s.j) {
			s.setLength(s.j)
			s.k = s.j - 1
			if s.lookup() {
				return
			}
			s.write(s.word[s.j-1])
		}
		s.setLength(s.j + 1)
		s.write('e')
		s.k = s.j + 1
		if s.lookup() {
			return
		}
		s.setLength(s.j + 1)
		s.appendStr("um")
		s.k = s.j + 2
		if s.lookup() {
			return
		}
		s.setLength(s.j + 1)
		s.appendStr("al")
		s.k = oldK
		if s.j > 0 && s.word[s.j-1] == 'i' && s.word[s.j] == 'c' {
			s.setLength(s.j - 1)
			s.k = s.j - 2
			if s.lookup() {
				return
			}
			s.setLength(s.j - 1)
			s.write('y')
			s.k = s.j - 1
			if s.lookup() {
				return
			}
			s.setLength(s.j - 1)
			s.appendStr("ic")
			s.k = s.j
			s.lookup()
			return
		}
		if s.word[s.j] == 'i' {
			s.setLength(s.j)
			s.k = s.j - 1
			if s.lookup() {
				return
			}
			s.appendStr("ial")
			s.k = oldK
			s.lookup()
		}
	}
}

func (s *ksStemmer) iveEndings() {
	oldK := s.k
	if s.endsIn3('i', 'v', 'e') {
		s.setLength(s.j + 1)
		s.k = s.j
		if s.lookup() {
			return
		}
		s.write('e')
		s.k = s.j + 1
		if s.lookup() {
			return
		}
		s.setLength(s.j + 1)
		s.appendStr("ive")
		if s.j > 0 && s.word[s.j-1] == 'a' && s.word[s.j] == 't' {
			s.setCharAt(s.j-1, 'e')
			s.setLength(s.j)
			s.k = s.j - 1
			if s.lookup() {
				return
			}
			s.setLength(s.j - 1)
			if s.lookup() {
				return
			}
			s.appendStr("ative")
			s.k = oldK
		}
		s.setCharAt(s.j+2, 'o')
		s.setCharAt(s.j+3, 'n')
		if s.lookup() {
			return
		}
		s.setCharAt(s.j+2, 'v')
		s.setCharAt(s.j+3, 'e')
		s.k = oldK
	}
}
