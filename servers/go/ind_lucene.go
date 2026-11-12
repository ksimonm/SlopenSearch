package main

// Ports of Lucene 10.5.1 normalizers/stemmers for Arabic, Persian, Sorani, Hindi,
// Bengali, the generic Indic normalizer and DecimalDigitFilter. Lucene works on the
// UTF-16 char[] of a term; these ports do the same (over []uint16), so length checks and
// char-level edge cases (supplementary characters, surrogates) behave exactly like Java.

import (
	"sort"
	"strings"
	"unicode/utf16"
)

// ---- shared helpers (StemmerUtil) ----

func indEncode(s string) []uint16 { return utf16.Encode([]rune(s)) }

func indDecode(s []uint16, n int) string { return string(utf16.Decode(s[:n])) }

// indDelete is StemmerUtil.delete: removes s[pos], returns the new length.
func indDelete(s []uint16, pos, n int) int {
	copy(s[pos:n-1], s[pos+1:n])
	return n - 1
}

// indDeleteN is StemmerUtil.deleteN.
func indDeleteN(s []uint16, pos, n, k int) int {
	copy(s[pos:n-k], s[pos+k:n])
	return n - k
}

// indEndsWith is StemmerUtil.endsWith.
func indEndsWith(s []uint16, n int, suf []uint16) bool {
	if len(suf) > n {
		return false
	}
	off := n - len(suf)
	for i, c := range suf {
		if s[off+i] != c {
			return false
		}
	}
	return true
}

func indU16s(list ...string) [][]uint16 {
	out := make([][]uint16, len(list))
	for i, s := range list {
		out[i] = indEncode(s)
	}
	return out
}

type indSuffixGroupSpec struct {
	minLen   int // the group applies when len > minLen
	cut      int // chars removed when a suffix of the group matches
	suffixes []string
}

type indSuffixGroup struct {
	minLen, cut int
	suffixes    [][]uint16
}

func indBuildSuffixGroups(specs []indSuffixGroupSpec) []indSuffixGroup {
	out := make([]indSuffixGroup, len(specs))
	for i, sp := range specs {
		out[i] = indSuffixGroup{sp.minLen, sp.cut, indU16s(sp.suffixes...)}
	}
	return out
}

// indStemGroups is the shared shape of HindiStemmer.stem and BengaliStemmer.stem.
func indStemGroups(groups []indSuffixGroup, s []uint16, n int) int {
	for _, g := range groups {
		if n > g.minLen {
			for _, suf := range g.suffixes {
				if indEndsWith(s, n, suf) {
					return n - g.cut
				}
			}
		}
	}
	return n
}

// ---- ar.ArabicNormalizer / ArabicNormalizationFilter ----

func indArabicNormalize(s []uint16, n int) int {
	for i := 0; i < n; i++ {
		switch s[i] {
		case 0x0622, 0x0623, 0x0625: // alef madda, hamza above, hamza below
			s[i] = 0x0627
		case 0x0649: // dotless yeh
			s[i] = 0x064A
		case 0x0629: // teh marbuta
			s[i] = 0x0647
		case 0x0640, 0x064D, 0x064C, 0x064B, 0x064E, 0x064F, 0x0650, 0x0651, 0x0652:
			// tatweel and harakat
			n = indDelete(s, i, n)
			i--
		}
	}
	return n
}

// arabicNormalizeLucene ports org.apache.lucene.analysis.ar.ArabicNormalizationFilter.
func arabicNormalizeLucene(term string) string {
	s := indEncode(term)
	return indDecode(s, indArabicNormalize(s, len(s)))
}

// ---- ar.ArabicStemmer / ArabicStemFilter ----

var indArabicPrefixes = indU16s(
	"ال",  // al
	"وال", // wal
	"بال", // bal
	"كال", // kal
	"فال", // fal
	"لل",  // ll
	"و",   // w
)

var indArabicSuffixes = indU16s(
	"ها", // ha
	"ان", // an
	"ات", // at
	"ون", // wn
	"ين", // yn
	"يه", // yh
	"ية", // yp
	"ه",  // h
	"ة",  // p
	"ي",  // y
)

func indArabicStem(s []uint16, n int) int {
	// stemPrefix: first matching prefix only
	for _, p := range indArabicPrefixes {
		if indArabicStartsWithCheckLength(s, n, p) {
			n = indDeleteN(s, 0, n, len(p))
			break
		}
	}
	// stemSuffix: every suffix in turn, on the shrinking term
	for _, suf := range indArabicSuffixes {
		if n >= len(suf)+2 && indEndsWith(s, n, suf) {
			n = indDeleteN(s, n-len(suf), n, len(suf))
		}
	}
	return n
}

func indArabicStartsWithCheckLength(s []uint16, n int, p []uint16) bool {
	if len(p) == 1 && n < 4 { // wa- prefix requires at least 3 characters
		return false
	}
	if n < len(p)+2 { // other prefixes require only 2
		return false
	}
	for i, c := range p {
		if s[i] != c {
			return false
		}
	}
	return true
}

// arabicStemLucene ports org.apache.lucene.analysis.ar.ArabicStemFilter (non-keyword token).
func arabicStemLucene(term string) string {
	s := indEncode(term)
	return indDecode(s, indArabicStem(s, len(s)))
}

// ---- fa.PersianNormalizer / PersianStemmer / PersianCharFilter ----

func indPersianNormalize(s []uint16, n int) int {
	for i := 0; i < n; i++ {
		switch s[i] {
		case 0x06CC, 0x06D2: // farsi yeh, yeh barree
			s[i] = 0x064A
		case 0x06A9: // keheh
			s[i] = 0x0643
		case 0x06C0, 0x06C1: // heh yeh, heh goal
			s[i] = 0x0647
		case 0x0654: // hamza above
			n = indDelete(s, i, n)
			i--
		}
	}
	return n
}

// persianNormalizeLucene ports org.apache.lucene.analysis.fa.PersianNormalizationFilter.
func persianNormalizeLucene(term string) string {
	s := indEncode(term)
	return indDecode(s, indPersianNormalize(s, len(s)))
}

var indPersianSuffixes = indU16s(
	"ات",   // at
	"ان",   // an
	"ترين", // tryn
	"تر",   // tr
	"يي",   // yy
	"ي",    // y
	"ها",   // ha
	"‌",    // zwnj
)

// persianStemLucene ports org.apache.lucene.analysis.fa.PersianStemFilter (non-keyword token).
func persianStemLucene(term string) string {
	s := indEncode(term)
	n := len(s)
	for _, suf := range indPersianSuffixes {
		if n >= len(suf)+2 && indEndsWith(s, n, suf) {
			n = indDeleteN(s, n-len(suf), n, len(suf))
		}
	}
	return indDecode(s, n)
}

// persianCharFilter ports org.apache.lucene.analysis.fa.PersianCharFilter: every
// ZERO WIDTH NON-JOINER becomes a space. Both are one UTF-16 char, so the text length
// never changes and offsets need no correction (Lucene's correct() is the identity).
func persianCharFilter(text string) string {
	return strings.ReplaceAll(text, "‌", " ")
}

// ---- ckb.SoraniNormalizer / SoraniStemmer ----

func indSoraniNormalize(s []uint16, n int) int {
	const heh, ae = 0x0647, 0x06D5
	for i := 0; i < n; i++ {
		switch c := s[i]; c {
		case 0x064A, 0x0649: // yeh, dotless yeh
			s[i] = 0x06CC
		case 0x0643: // kaf
			s[i] = 0x06A9
		case 0x200C: // zwnj
			if i > 0 && s[i-1] == heh {
				s[i-1] = ae
			}
			n = indDelete(s, i, n)
			i--
		case heh:
			if i == n-1 {
				s[i] = ae
			}
		case 0x0629: // teh marbuta
			s[i] = ae
		case 0x06BE: // heh doachashmee
			s[i] = heh
		case 0x0631: // reh
			if i == 0 {
				s[i] = 0x0695
			}
		case 0x0692: // rreh above
			s[i] = 0x0695
		case 0x0640, 0x064D, 0x064C, 0x064B, 0x064E, 0x064F, 0x0650, 0x0651, 0x0652:
			n = indDelete(s, i, n)
			i--
		default:
			if indFormatBMP[c] { // Character.getType(char) == FORMAT
				n = indDelete(s, i, n)
				i--
			}
		}
	}
	return n
}

// soraniNormalizeLucene ports org.apache.lucene.analysis.ckb.SoraniNormalizationFilter.
func soraniNormalizeLucene(term string) string {
	s := indEncode(term)
	return indDecode(s, indSoraniNormalize(s, len(s)))
}

var (
	indSorDa     = indEncode("دا")
	indSorNa     = indEncode("نا")
	indSorAwa    = indEncode("ەوە")
	indSorMan    = indEncode("مان")
	indSorYan    = indEncode("یان")
	indSorTan    = indEncode("تان")
	indSorEki    = indEncode("ێکی")
	indSorYaki   = indEncode("یەکی")
	indSorEk     = indEncode("ێک")
	indSorYak    = indEncode("یەک")
	indSorAka    = indEncode("ەکە")
	indSorKa     = indEncode("کە")
	indSorAkan   = indEncode("ەکان")
	indSorKan    = indEncode("کان")
	indSorYani   = indEncode("یانی")
	indSorAni    = indEncode("انی")
	indSorAn     = indEncode("ان")
	indSorYana   = indEncode("یانە")
	indSorAna    = indEncode("انە")
	indSorAya    = indEncode("ایە")
	indSorAya2   = indEncode("ەیە")
	indSorA      = indEncode("ە")
	indSorI      = indEncode("ی")
	indSorEnding = indEndsWith
)

func indSoraniStem(s []uint16, n int) int {
	ew := indEndsWith
	// postposition
	if n > 5 && ew(s, n, indSorDa) {
		n -= 2
	} else if n > 4 && ew(s, n, indSorNa) {
		n--
	} else if n > 6 && ew(s, n, indSorAwa) {
		n -= 3
	}
	// possessive pronoun
	if n > 6 && (ew(s, n, indSorMan) || ew(s, n, indSorYan) || ew(s, n, indSorTan)) {
		n -= 3
	}
	// indefinite singular ezafe
	if n > 6 && ew(s, n, indSorEki) {
		return n - 3
	} else if n > 7 && ew(s, n, indSorYaki) {
		return n - 4
	}
	switch {
	case n > 5 && ew(s, n, indSorEk): // indefinite singular
		return n - 2
	case n > 6 && ew(s, n, indSorYak):
		return n - 3
	case n > 6 && ew(s, n, indSorAka): // definite singular
		return n - 3
	case n > 5 && ew(s, n, indSorKa):
		return n - 2
	case n > 7 && ew(s, n, indSorAkan): // definite plural
		return n - 4
	case n > 6 && ew(s, n, indSorKan):
		return n - 3
	case n > 7 && ew(s, n, indSorYani): // indefinite plural ezafe
		return n - 4
	case n > 6 && ew(s, n, indSorAni):
		return n - 3
	case n > 6 && ew(s, n, indSorYan): // indefinite plural
		return n - 3
	case n > 5 && ew(s, n, indSorAn):
		return n - 2
	case n > 7 && ew(s, n, indSorYana): // demonstrative plural
		return n - 4
	case n > 6 && ew(s, n, indSorAna):
		return n - 3
	case n > 5 && (ew(s, n, indSorAya) || ew(s, n, indSorAya2)): // demonstrative singular
		return n - 2
	case n > 4 && ew(s, n, indSorA):
		return n - 1
	case n > 4 && ew(s, n, indSorI): // absolute singular ezafe
		return n - 1
	}
	return n
}

// soraniStemLucene ports org.apache.lucene.analysis.ckb.SoraniStemFilter (non-keyword token).
func soraniStemLucene(term string) string {
	s := indEncode(term)
	return indDecode(s, indSoraniStem(s, len(s)))
}

// ---- in.IndicNormalizer / IndicNormalizationFilter ----

// The nine scripts of IndicNormalizer, each a 128-char Unicode block starting at
// 0x0900 + 0x80*k (Devanagari, Bengali, Gurmukhi, Gujarati, Oriya, Tamil, Telugu,
// Kannada, Malayalam); the flag of script k is 1<<k.
const (
	indDeva = 1 << iota
	indBeng
	indGuru
	indGujr
	indOrya
	indTaml
	indTelu
	indKnda
	indMlym
)

// indIndicBlock is Character.UnicodeBlock.of(c) restricted to the nine blocks: the
// script index 0..8, or -1.
func indIndicBlock(c uint16) int {
	if c >= 0x0900 && c < 0x0D80 {
		return int(c-0x0900) >> 7
	}
	return -1
}

// indIndicDecomps is IndicNormalizer.decompositions: ch1, ch2, ch3, res, flags
// (offsets into the script block, -1 = none, 0xFF = ZWJ).
var indIndicDecomps = [][5]int{
	{0x05, 0x3E, 0x45, 0x11, indDeva | indGujr},
	{0x05, 0x3E, 0x46, 0x12, indDeva},
	{0x05, 0x3E, 0x47, 0x13, indDeva | indGujr},
	{0x05, 0x3E, 0x48, 0x14, indDeva | indGujr},
	{0x05, 0x3E, -1, 0x06, indDeva | indBeng | indGuru | indGujr | indOrya},
	{0x05, 0x45, -1, 0x72, indDeva},
	{0x05, 0x45, -1, 0x0D, indGujr},
	{0x05, 0x46, -1, 0x04, indDeva},
	{0x05, 0x47, -1, 0x0F, indGujr},
	{0x05, 0x48, -1, 0x10, indGuru | indGujr},
	{0x05, 0x49, -1, 0x11, indDeva | indGujr},
	{0x05, 0x4A, -1, 0x12, indDeva},
	{0x05, 0x4B, -1, 0x13, indDeva | indGujr},
	{0x05, 0x4C, -1, 0x14, indDeva | indGuru | indGujr},
	{0x06, 0x45, -1, 0x11, indDeva | indGujr},
	{0x06, 0x46, -1, 0x12, indDeva},
	{0x06, 0x47, -1, 0x13, indDeva | indGujr},
	{0x06, 0x48, -1, 0x14, indDeva | indGujr},
	{0x07, 0x57, -1, 0x08, indMlym},
	{0x09, 0x41, -1, 0x0A, indDeva},
	{0x09, 0x57, -1, 0x0A, indTaml | indMlym},
	{0x0E, 0x46, -1, 0x10, indMlym},
	{0x0F, 0x45, -1, 0x0D, indDeva},
	{0x0F, 0x46, -1, 0x0E, indDeva},
	{0x0F, 0x47, -1, 0x10, indDeva},
	{0x0F, 0x57, -1, 0x10, indOrya},
	{0x12, 0x3E, -1, 0x13, indMlym},
	{0x12, 0x4C, -1, 0x14, indTelu | indKnda},
	{0x12, 0x55, -1, 0x13, indTelu},
	{0x12, 0x57, -1, 0x14, indTaml | indMlym},
	{0x13, 0x57, -1, 0x14, indOrya},
	{0x15, 0x3C, -1, 0x58, indDeva},
	{0x16, 0x3C, -1, 0x59, indDeva | indGuru},
	{0x17, 0x3C, -1, 0x5A, indDeva | indGuru},
	{0x1C, 0x3C, -1, 0x5B, indDeva | indGuru},
	{0x21, 0x3C, -1, 0x5C, indDeva | indBeng | indOrya},
	{0x22, 0x3C, -1, 0x5D, indDeva | indBeng | indOrya},
	{0x23, 0x4D, 0xFF, 0x7A, indMlym},
	{0x24, 0x4D, 0xFF, 0x4E, indBeng},
	{0x28, 0x3C, -1, 0x29, indDeva},
	{0x28, 0x4D, 0xFF, 0x7B, indMlym},
	{0x2B, 0x3C, -1, 0x5E, indDeva | indGuru},
	{0x2F, 0x3C, -1, 0x5F, indDeva | indBeng},
	{0x2C, 0x41, 0x41, 0x0B, indTelu},
	{0x30, 0x3C, -1, 0x31, indDeva},
	{0x30, 0x4D, 0xFF, 0x7C, indMlym},
	{0x32, 0x4D, 0xFF, 0x7D, indMlym},
	{0x33, 0x3C, -1, 0x34, indDeva},
	{0x33, 0x4D, 0xFF, 0x7E, indMlym},
	{0x35, 0x41, -1, 0x2E, indTelu},
	{0x3E, 0x45, -1, 0x49, indDeva | indGujr},
	{0x3E, 0x46, -1, 0x4A, indDeva},
	{0x3E, 0x47, -1, 0x4B, indDeva | indGujr},
	{0x3E, 0x48, -1, 0x4C, indDeva | indGujr},
	{0x3F, 0x55, -1, 0x40, indKnda},
	{0x41, 0x41, -1, 0x42, indGuru},
	{0x46, 0x3E, -1, 0x4A, indTaml | indMlym},
	{0x46, 0x42, 0x55, 0x4B, indKnda},
	{0x46, 0x42, -1, 0x4A, indKnda},
	{0x46, 0x46, -1, 0x48, indMlym},
	{0x46, 0x55, -1, 0x47, indTelu | indKnda},
	{0x46, 0x56, -1, 0x48, indTelu | indKnda},
	{0x46, 0x57, -1, 0x4C, indTaml | indMlym},
	{0x47, 0x3E, -1, 0x4B, indBeng | indOrya | indTaml | indMlym},
	{0x47, 0x57, -1, 0x4C, indBeng | indOrya},
	{0x4A, 0x55, -1, 0x4B, indKnda},
	{0x72, 0x3F, -1, 0x07, indGuru},
	{0x72, 0x40, -1, 0x08, indGuru},
	{0x72, 0x47, -1, 0x0F, indGuru},
	{0x73, 0x41, -1, 0x09, indGuru},
	{0x73, 0x42, -1, 0x0A, indGuru},
	{0x73, 0x4B, -1, 0x13, indGuru},
}

// indIndicDecompMask[k] is ScriptData.decompMask of script k: bit ch set when some
// decomposition for that script starts with ch.
var indIndicDecompMask = func() (m [9][2]uint64) {
	for k := 0; k < 9; k++ {
		for _, d := range indIndicDecomps {
			if d[4]&(1<<k) != 0 {
				m[k][d[0]>>6] |= 1 << (d[0] & 63)
			}
		}
	}
	return m
}()

func indIndicNormalize(s []uint16, n int) int {
	for i := 0; i < n; i++ {
		k := indIndicBlock(s[i])
		if k >= 0 {
			ch := int(s[i]) - (0x0900 + k<<7)
			if indIndicDecompMask[k][ch>>6]&(1<<(ch&63)) != 0 {
				n = indIndicCompose(ch, k, s, i, n)
			}
		}
	}
	return n
}

func indIndicCompose(ch0, k int, s []uint16, pos, n int) int {
	if pos+1 >= n { // need at least 2 chars
		return n
	}
	base := 0x0900 + k<<7
	ch1 := int(s[pos+1]) - base
	if indIndicBlock(s[pos+1]) != k { // needs to be the same writing system
		return n
	}
	ch2 := -1
	if pos+2 < n {
		ch2 = int(s[pos+2]) - base
		if s[pos+2] == 0x200D { // ZWJ
			ch2 = 0xFF
		} else if indIndicBlock(s[pos+2]) != k { // still allow a 2-char match
			ch2 = -1
		}
	}
	for _, d := range indIndicDecomps {
		if d[0] == ch0 && d[4]&(1<<k) != 0 {
			if d[1] == ch1 && (d[2] < 0 || d[2] == ch2) {
				s[pos] = uint16(base + d[3])
				n = indDelete(s, pos+1, n)
				if d[2] >= 0 {
					n = indDelete(s, pos+1, n)
				}
				return n
			}
		}
	}
	return n
}

// indicNormalizeLucene ports org.apache.lucene.analysis.in.IndicNormalizationFilter.
func indicNormalizeLucene(term string) string {
	s := indEncode(term)
	return indDecode(s, indIndicNormalize(s, len(s)))
}

// ---- hi.HindiNormalizer / HindiStemmer ----

// indHindiMap holds HindiNormalizer's plain one-char replacements.
var indHindiMap = map[uint16]uint16{
	0x0901: 0x0902, // candrabindu -> bindu
	// nukta forms
	0x0929: 0x0928, 0x0931: 0x0930, 0x0934: 0x0933, 0x0958: 0x0915, 0x0959: 0x0916,
	0x095A: 0x0917, 0x095B: 0x091C, 0x095C: 0x0921, 0x095D: 0x0922, 0x095E: 0x092B,
	0x095F: 0x092F,
	// chandra/short -> replace
	0x0945: 0x0947, 0x0946: 0x0947, 0x0949: 0x094B, 0x094A: 0x094B, 0x090D: 0x090F,
	0x090E: 0x090F, 0x0911: 0x0913, 0x0912: 0x0913, 0x0972: 0x0905,
	// long -> short independent vowels
	0x0906: 0x0905, 0x0908: 0x0907, 0x090A: 0x0909, 0x0960: 0x090B, 0x0961: 0x090C,
	0x0910: 0x090F, 0x0914: 0x0913,
	// long -> short dependent vowels
	0x0940: 0x093F, 0x0942: 0x0941, 0x0944: 0x0943, 0x0963: 0x0962, 0x0948: 0x0947,
	0x094C: 0x094B,
}

func indHindiNormalize(s []uint16, n int) int {
	for i := 0; i < n; i++ {
		switch c := s[i]; c {
		case 0x0928: // dead n -> bindu
			if i+1 < n && s[i+1] == 0x094D {
				s[i] = 0x0902
				n = indDelete(s, i+1, n)
			}
		case 0x093C, 0x200D, 0x200C, 0x094D: // nukta, zwj, zwnj, virama -> delete
			n = indDelete(s, i, n)
			i--
		default:
			if r, ok := indHindiMap[c]; ok {
				s[i] = r
			}
		}
	}
	return n
}

// hindiNormalizeLucene ports org.apache.lucene.analysis.hi.HindiNormalizationFilter (non-keyword token).
func hindiNormalizeLucene(term string) string {
	s := indEncode(term)
	return indDecode(s, indHindiNormalize(s, len(s)))
}

// hindiStemLucene ports org.apache.lucene.analysis.hi.HindiStemFilter (non-keyword token).
func hindiStemLucene(term string) string {
	s := indEncode(term)
	return indDecode(s, indStemGroups(indHindiSuffixGroups, s, len(s)))
}

// ---- bn.BengaliNormalizer / BengaliStemmer ----

func indBengaliNormalize(s []uint16, n int) int {
	for i := 0; i < n; i++ {
		switch s[i] {
		case 0x0981: // delete chandrabindu
			n = indDelete(s, i, n)
			i--
		case 0x09C0: // dirgho i kar -> rossho i kar
			s[i] = 0x09BF
		case 0x09C2: // dirgho u kar -> rossho u kar
			s[i] = 0x09C1
		case 0x0995: // khio (ka + hoshonto + murdorno sh)
			if i+2 < n && s[i+1] == 0x09CD && s[i+2] == 0x09BF {
				if i == 0 {
					s[i] = 0x0996
					n = indDelete(s, i+2, n)
					n = indDelete(s, i+1, n)
				} else {
					s[i+1] = 0x0996
					n = indDelete(s, i+2, n)
				}
			}
		case 0x0999: // nga to anusvara
			s[i] = 0x0982
		case 0x09AF: // ja phala
			if i-2 == 0 && s[i-1] == 0x09CD {
				s[i-1] = 0x09C7
				if i+1 < n && s[i+1] == 0x09BE {
					n = indDelete(s, i+1, n)
				}
				n = indDelete(s, i, n)
				i--
			} else if i-1 >= 0 && s[i-1] == 0x09CD {
				n = indDelete(s, i, n)
				n = indDelete(s, i-1, n)
				i -= 2
			}
		case 0x09AC: // ba phala
			if (i >= 1 && s[i-1] != 0x09CD) || i == 0 {
				break
			}
			if i-2 == 0 {
				n = indDelete(s, i, n)
				n = indDelete(s, i-1, n)
				i -= 2
			} else if i-5 >= 0 && s[i-3] == 0x09CD {
				n = indDelete(s, i, n)
				n = indDelete(s, i-1, n)
				i -= 2
			} else if i-2 >= 0 {
				s[i-1] = s[i-2]
				n = indDelete(s, i, n)
				i--
			}
		case 0x0983: // visarga
			if i == n-1 {
				if n <= 3 {
					s[i] = 0x09B9
				} else {
					n = indDelete(s, i, n)
				}
			} else {
				s[i] = s[i+1]
			}
		case 0x09B6, 0x09B7: // all sh
			s[i] = 0x09B8
		case 0x09A3: // na
			s[i] = 0x09A8
		case 0x09DC, 0x09DD: // ra
			s[i] = 0x09B0
		case 0x09CE: // khanda ta
			s[i] = 0x09A4
		}
	}
	return n
}

// bengaliNormalizeLucene ports org.apache.lucene.analysis.bn.BengaliNormalizationFilter (non-keyword token).
func bengaliNormalizeLucene(term string) string {
	s := indEncode(term)
	return indDecode(s, indBengaliNormalize(s, len(s)))
}

// bengaliStemLucene ports org.apache.lucene.analysis.bn.BengaliStemFilter (non-keyword token).
func bengaliStemLucene(term string) string {
	s := indEncode(term)
	return indDecode(s, indStemGroups(indBengaliSuffixGroups, s, len(s)))
}

// ---- core.DecimalDigitFilter ----

// indDigitValue is Java's Character.isDigit(cp) ? Character.getNumericValue(cp) : -1,
// using the JDK 25 (Unicode 16) table in ind_tables.go.
func indDigitValue(cp rune) int {
	i := sort.Search(len(indDigitZeros), func(j int) bool { return indDigitZeros[j] > cp }) - 1
	if i >= 0 && cp-indDigitZeros[i] <= 9 {
		return int(cp - indDigitZeros[i])
	}
	return -1
}

// decimalDigitLucene ports org.apache.lucene.analysis.core.DecimalDigitFilter: every
// non-ASCII decimal digit becomes the ASCII digit of the same value.
func decimalDigitLucene(term string) string {
	s := indEncode(term)
	n := len(s)
	for i := 0; i < n; i++ {
		cp := rune(s[i])
		if utf16.IsSurrogate(cp) && cp < 0xDC00 && i+1 < n && s[i+1] >= 0xDC00 && s[i+1] < 0xE000 {
			cp = utf16.DecodeRune(cp, rune(s[i+1])) // Character.codePointAt
		}
		if cp > 0x7F {
			if v := indDigitValue(cp); v >= 0 {
				s[i] = uint16('0' + v)
				if cp > 0xFFFF {
					n = indDelete(s, i+1, n)
				}
			}
		}
	}
	return indDecode(s, n)
}
