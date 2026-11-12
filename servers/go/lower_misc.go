package main

// Language specific lowercase / normalization filters of Lucene 10.5.1 analysis-common.

// greekLowerLucene ports org.apache.lucene.analysis.el.GreekLowerCaseFilter.
func greekLowerLucene(term string) string {
	return mscMapRunes(term, func(r rune) rune {
		switch r {
		case 'ς':
			return 'σ'
		case 'Ά', 'ά':
			return 'α'
		case 'Έ', 'έ':
			return 'ε'
		case 'Ή', 'ή':
			return 'η'
		case 'Ί', 'Ϊ', 'ί', 'ϊ', 'ΐ':
			return 'ι'
		case 'Ύ', 'Ϋ', 'ύ', 'ϋ', 'ΰ':
			return 'υ'
		case 'Ό', 'ό':
			return 'ο'
		case 'Ώ', 'ώ':
			return 'ω'
		case '΢':
			return 'ς'
		}
		return mscJavaLowerRune(r)
	})
}

// irishLowerLucene ports org.apache.lucene.analysis.ga.IrishLowerCaseFilter: "nA..."/"tA..."
// (n or t followed by an uppercase vowel) becomes "n-a..."/"t-a...", the rest is lowercased with
// Character.toLowerCase(char), i.e. per UTF-16 unit, so supplementary characters stay unchanged.
func irishLowerLucene(term string) string {
	rs := []rune(term)
	idx := 0
	if len(rs) > 1 && (rs[0] == 'n' || rs[0] == 't') && mscIrishUpperVowel(rs[1]) {
		rs = append(rs[:1], append([]rune{'-'}, rs[1:]...)...)
		idx = 2
	}
	for i := idx; i < len(rs); i++ {
		if rs[i] <= 0xFFFF {
			rs[i] = mscJavaLowerRune(rs[i])
		}
	}
	return string(rs)
}

func mscIrishUpperVowel(r rune) bool {
	switch r {
	case 'A', 'E', 'I', 'O', 'U', 'Á', 'É', 'Í', 'Ó', 'Ú':
		return true
	}
	return false
}

// turkishLowerLucene ports org.apache.lucene.analysis.tr.TurkishLowerCaseFilter.
func turkishLowerLucene(term string) string {
	buf := []rune(term)
	n := len(buf)
	iOrAfter := false
	for i := 0; i < n; {
		ch := buf[i]
		iOrAfter = ch == 'I' || (iOrAfter && mscIsJavaMn(ch))
		if iOrAfter {
			switch ch {
			case '̇':
				copy(buf[i:], buf[i+1:n])
				n--
				continue
			case 'I':
				if mscTurkishBeforeDot(buf[i+1 : n]) {
					buf[i] = 'i'
				} else {
					buf[i] = 'ı'
					iOrAfter = false
				}
				i++
				continue
			}
		}
		buf[i] = mscJavaLowerRune(ch)
		i++
	}
	return string(buf[:n])
}

func mscTurkishBeforeDot(s []rune) bool {
	for _, ch := range s {
		if !mscIsJavaMn(ch) {
			return false
		}
		if ch == '̇' {
			return true
		}
	}
	return false
}

// apostropheLucene ports org.apache.lucene.analysis.tr.ApostropheFilter: strips everything from
// the first apostrophe (U+0027 or U+2019) on.
func apostropheLucene(term string) string {
	for i, r := range term {
		if r == '\'' || r == '’' {
			return term[:i]
		}
	}
	return term
}

// serbianNormalizeLucene ports org.apache.lucene.analysis.sr.SerbianNormalizationFilter
// (Cyrillic and Latin to "bald" Latin).
func serbianNormalizeLucene(term string) string {
	return mscSerbian(term, mscSerbianBald)
}

// serbianNormalizeRegularLucene ports org.apache.lucene.analysis.sr.SerbianNormalizationRegularFilter
// (Cyrillic to Latin with diacritics).
func serbianNormalizeRegularLucene(term string) string {
	return mscSerbian(term, mscSerbianRegular)
}

func mscSerbian(term string, m map[rune]string) string {
	changed := false
	for _, r := range term {
		if _, ok := m[r]; ok {
			changed = true
			break
		}
	}
	if !changed {
		return term
	}
	out := make([]rune, 0, len(term))
	for _, r := range term {
		if v, ok := m[r]; ok {
			out = append(out, []rune(v)...)
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}

var mscSerbianBald = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'ђ': "dj", 'đ': "dj", 'е': "e",
	'ж': "z", 'з': "z", 'ž': "z", 'и': "i", 'ј': "j", 'к': "k", 'л': "l", 'љ': "lj",
	'м': "m", 'н': "n", 'њ': "nj", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t",
	'ћ': "c", 'ц': "c", 'ч': "c", 'č': "c", 'ć': "c", 'у': "u", 'ф': "f", 'х': "h",
	'џ': "dz", 'ш': "s", 'š': "s",
}

var mscSerbianRegular = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'ђ': "đ", 'е': "e", 'ж': "ž",
	'з': "z", 'и': "i", 'ј': "j", 'к': "k", 'л': "l", 'љ': "lj", 'м': "m", 'н': "n",
	'њ': "nj", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'ћ': "ć", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "c", 'ч': "č", 'џ': "dž", 'ш': "š",
}

// cjkWidthLucene ports org.apache.lucene.analysis.cjk.CJKWidthFilter: folds fullwidth ASCII
// variants to Basic Latin and halfwidth Katakana to standard Katakana (combining the halfwidth
// voiced / semi-voiced sound marks into the preceding kana where possible).
func cjkWidthLucene(term string) string {
	need := false
	for _, r := range term {
		if r >= 0xFF01 && r <= 0xFF9F {
			need = true
			break
		}
	}
	if !need {
		return term
	}
	text := []rune(term)
	n := len(text)
	for i := 0; i < n; i++ {
		ch := text[i]
		if ch >= 0xFF01 && ch <= 0xFF5E {
			text[i] -= 0xFEE0
		} else if ch >= 0xFF65 && ch <= 0xFF9F {
			if (ch == 0xFF9E || ch == 0xFF9F) && i > 0 && mscCJKCombine(text, i, ch) {
				copy(text[i:], text[i+1:n])
				n--
				i--
			} else {
				text[i] = rune(mscKanaNorm[ch-0xFF65])
			}
		}
	}
	return string(text[:n])
}

func mscCJKCombine(text []rune, pos int, ch rune) bool {
	prev := text[pos-1]
	if prev >= 0x30A6 && prev <= 0x30FD {
		if ch == 0xFF9F {
			text[pos-1] += rune(mscKanaCombineHalfVoiced[prev-0x30A6])
		} else {
			text[pos-1] += rune(mscKanaCombineVoiced[prev-0x30A6])
		}
		return text[pos-1] != prev
	}
	return false
}

// halfwidth kana mappings: 0xFF65-0xFF9F
var mscKanaNorm = [...]uint16{
	0x30fb, 0x30f2, 0x30a1, 0x30a3, 0x30a5, 0x30a7, 0x30a9, 0x30e3, 0x30e5,
	0x30e7, 0x30c3, 0x30fc, 0x30a2, 0x30a4, 0x30a6, 0x30a8, 0x30aa, 0x30ab,
	0x30ad, 0x30af, 0x30b1, 0x30b3, 0x30b5, 0x30b7, 0x30b9, 0x30bb, 0x30bd,
	0x30bf, 0x30c1, 0x30c4, 0x30c6, 0x30c8, 0x30ca, 0x30cb, 0x30cc, 0x30cd,
	0x30ce, 0x30cf, 0x30d2, 0x30d5, 0x30d8, 0x30db, 0x30de, 0x30df, 0x30e0,
	0x30e1, 0x30e2, 0x30e4, 0x30e6, 0x30e8, 0x30e9, 0x30ea, 0x30eb, 0x30ec,
	0x30ed, 0x30ef, 0x30f3, 0x3099, 0x309A,
}

// kana combining diffs: 0x30A6-0x30FD
var mscKanaCombineVoiced = [...]uint8{
	78, 0, 0, 0, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 0,
	1, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 0, 1, 0, 0, 1, 0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 8, 8, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1,
}

var mscKanaCombineHalfVoiced = [...]uint8{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 2, 0, 0, 2,
	0, 0, 2, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}
