package main

// Faithful ports of Lucene 10.5.1 light/minimal stemmers and normalization
// filters for German, Swedish, Norwegian, Finnish, Hungarian, Russian and
// English, plus the Scandinavian folding/normalization filters. Every entry
// point reproduces the effect of the corresponding Lucene TokenFilter on a
// single non-keyword token. Lucene operates on UTF-16 chars; these ports work
// on runes, which is equivalent for the BMP text these algorithms target.

import (
	"unicode/utf8"
)

// gmcEndsWith ports StemmerUtil.endsWith(char[], int, String).
func gmcEndsWith(s []rune, n int, suffix string) bool {
	i := n
	for len(suffix) > 0 {
		r, size := utf8.DecodeLastRuneInString(suffix)
		i--
		if i < 0 || s[i] != r {
			return false
		}
		suffix = suffix[:len(suffix)-size]
	}
	return true
}

// gmcDelete ports StemmerUtil.delete: removes s[pos], returns the new length.
func gmcDelete(s []rune, pos, n int) int {
	if pos < n-1 {
		copy(s[pos:], s[pos+1:n])
	}
	return n - 1
}

// ---------------------------------------------------------------------------
// German

// germanLightStem ports org.apache.lucene.analysis.de.GermanLightStemmer
// (GermanLightStemFilter).
func germanLightStem(term string) string {
	s := []rune(term)
	n := len(s)
	for i := 0; i < n; i++ {
		switch s[i] {
		case 'ä', 'à', 'á', 'â':
			s[i] = 'a'
		case 'ö', 'ò', 'ó', 'ô':
			s[i] = 'o'
		case 'ï', 'ì', 'í', 'î':
			s[i] = 'i'
		case 'ü', 'ù', 'ú', 'û':
			s[i] = 'u'
		}
	}
	n = gmcGermanLightStep1(s, n)
	n = gmcGermanLightStep2(s, n)
	return string(s[:n])
}

func gmcGermanStEnding(ch rune) bool {
	switch ch {
	case 'b', 'd', 'f', 'g', 'h', 'k', 'l', 'm', 'n', 't':
		return true
	}
	return false
}

func gmcGermanLightStep1(s []rune, n int) int {
	if n > 5 && s[n-3] == 'e' && s[n-2] == 'r' && s[n-1] == 'n' {
		return n - 3
	}
	if n > 4 && s[n-2] == 'e' {
		switch s[n-1] {
		case 'm', 'n', 'r', 's':
			return n - 2
		}
	}
	if n > 3 && s[n-1] == 'e' {
		return n - 1
	}
	if n > 3 && s[n-1] == 's' && gmcGermanStEnding(s[n-2]) {
		return n - 1
	}
	return n
}

func gmcGermanLightStep2(s []rune, n int) int {
	if n > 5 && s[n-3] == 'e' && s[n-2] == 's' && s[n-1] == 't' {
		return n - 3
	}
	if n > 4 && s[n-2] == 'e' && (s[n-1] == 'r' || s[n-1] == 'n') {
		return n - 2
	}
	if n > 4 && s[n-2] == 's' && s[n-1] == 't' && gmcGermanStEnding(s[n-3]) {
		return n - 2
	}
	return n
}

// germanMinimalStem ports org.apache.lucene.analysis.de.GermanMinimalStemmer
// (GermanMinimalStemFilter).
func germanMinimalStem(term string) string {
	s := []rune(term)
	n := len(s)
	if n < 5 {
		return term
	}
	for i := 0; i < n; i++ {
		switch s[i] {
		case 'ä':
			s[i] = 'a'
		case 'ö':
			s[i] = 'o'
		case 'ü':
			s[i] = 'u'
		}
	}
	return string(s[:gmcGermanMinimalLen(s, n)])
}

func gmcGermanMinimalLen(s []rune, n int) int {
	if n > 6 && s[n-3] == 'n' && s[n-2] == 'e' && s[n-1] == 'n' {
		return n - 3
	}
	if n > 5 {
		switch s[n-1] {
		case 'n', 's', 'r':
			if s[n-2] == 'e' {
				return n - 2
			}
		case 'e':
			if s[n-2] == 's' {
				return n - 2
			}
		}
	}
	switch s[n-1] {
	case 'n', 'e', 's', 'r':
		return n - 1
	}
	return n
}

// germanNormalizeLucene ports org.apache.lucene.analysis.de.GermanNormalizationFilter.
func germanNormalizeLucene(term string) string {
	const (
		stN = 0 // ordinary state
		stV = 1 // stops 'u' from entering umlaut state
		stU = 2 // umlaut state, allows e-deletion
	)
	in := []rune(term)
	out := make([]rune, 0, len(in)+2)
	state := stN
	for _, c := range in {
		switch c {
		case 'a', 'o':
			state = stU
			out = append(out, c)
		case 'u':
			if state == stN {
				state = stU
			} else {
				state = stV
			}
			out = append(out, c)
		case 'e':
			if state != stU {
				out = append(out, c)
			}
			state = stV
		case 'i', 'q', 'y':
			state = stV
			out = append(out, c)
		case 'ä':
			out = append(out, 'a')
			state = stV
		case 'ö':
			out = append(out, 'o')
			state = stV
		case 'ü':
			out = append(out, 'u')
			state = stV
		case 'ß':
			out = append(out, 's', 's')
			state = stN
		default:
			state = stN
			out = append(out, c)
		}
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// Swedish

// swedishLightStem ports org.apache.lucene.analysis.sv.SwedishLightStemmer.
func swedishLightStem(term string) string {
	s := []rune(term)
	n := len(s)
	if n > 4 && s[n-1] == 's' {
		n--
	}
	return string(s[:gmcSwedishLightLen(s, n)])
}

func gmcSwedishLightLen(s []rune, n int) int {
	if n > 7 && (gmcEndsWith(s, n, "elser") || gmcEndsWith(s, n, "heten")) {
		return n - 5
	}
	if n > 6 && (gmcEndsWith(s, n, "arne") ||
		gmcEndsWith(s, n, "erna") ||
		gmcEndsWith(s, n, "ande") ||
		gmcEndsWith(s, n, "else") ||
		gmcEndsWith(s, n, "aste") ||
		gmcEndsWith(s, n, "orna") ||
		gmcEndsWith(s, n, "aren")) {
		return n - 4
	}
	if n > 5 && (gmcEndsWith(s, n, "are") || gmcEndsWith(s, n, "ast") || gmcEndsWith(s, n, "het")) {
		return n - 3
	}
	if n > 4 && (gmcEndsWith(s, n, "ar") ||
		gmcEndsWith(s, n, "er") ||
		gmcEndsWith(s, n, "or") ||
		gmcEndsWith(s, n, "en") ||
		gmcEndsWith(s, n, "at") ||
		gmcEndsWith(s, n, "te") ||
		gmcEndsWith(s, n, "et")) {
		return n - 2
	}
	if n > 3 {
		switch s[n-1] {
		case 't', 'a', 'e', 'n':
			return n - 1
		}
	}
	return n
}

// swedishMinimalStem ports org.apache.lucene.analysis.sv.SwedishMinimalStemmer.
func swedishMinimalStem(term string) string {
	s := []rune(term)
	n := len(s)
	if n > 4 && s[n-1] == 's' {
		n--
	}
	return string(s[:gmcSwedishMinimalLen(s, n)])
}

func gmcSwedishMinimalLen(s []rune, n int) int {
	if n > 6 && (gmcEndsWith(s, n, "arne") ||
		gmcEndsWith(s, n, "erna") ||
		gmcEndsWith(s, n, "arna") ||
		gmcEndsWith(s, n, "orna") ||
		gmcEndsWith(s, n, "aren")) {
		return n - 4
	}
	if n > 5 && gmcEndsWith(s, n, "are") {
		return n - 3
	}
	if n > 4 && (gmcEndsWith(s, n, "ar") ||
		gmcEndsWith(s, n, "at") ||
		gmcEndsWith(s, n, "er") ||
		gmcEndsWith(s, n, "et") ||
		gmcEndsWith(s, n, "or") ||
		gmcEndsWith(s, n, "en")) {
		return n - 2
	}
	if n > 3 {
		switch s[n-1] {
		case 'a', 'e', 'n':
			return n - 1
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Norwegian

// Flags for norwegianLightStem / norwegianMinimalStem (NorwegianLightStemmer.BOKMAAL/NYNORSK).
const (
	gmcNorwegianBokmaal = 1
	gmcNorwegianNynorsk = 2
)

func gmcNorwegianCheckFlags(flags int) (bokmaal, nynorsk bool) {
	if flags <= 0 || flags > gmcNorwegianBokmaal+gmcNorwegianNynorsk {
		panic("invalid flags")
	}
	return flags&gmcNorwegianBokmaal != 0, flags&gmcNorwegianNynorsk != 0
}

// norwegianLightStem ports org.apache.lucene.analysis.no.NorwegianLightStemmer.
// flags: 1 = bokmål, 2 = nynorsk, 3 = both (other values panic, as Lucene throws).
func norwegianLightStem(term string, flags int) string {
	bm, nn := gmcNorwegianCheckFlags(flags)
	s := []rune(term)
	n := len(s)
	if n > 4 && s[n-1] == 's' {
		n--
	}
	return string(s[:gmcNorwegianLightLen(s, n, bm, nn)])
}

func gmcNorwegianLightLen(s []rune, n int, bm, nn bool) int {
	e := func(suf string) bool { return gmcEndsWith(s, n, suf) }
	if n > 7 && ((e("heter") && bm) || (e("heten") && bm) || (e("heita") && nn)) {
		return n - 5
	}
	if n > 8 && nn && (e("heiter") || e("leiken") || e("leikar")) {
		return n - 6
	}
	if n > 5 && (e("dom") || (e("het") && bm)) {
		return n - 3
	}
	if n > 6 && nn && (e("heit") || e("semd") || e("leik")) {
		return n - 4
	}
	if n > 7 && (e("elser") || e("elsen")) {
		return n - 5
	}
	if n > 6 && ((e("ende") && bm) ||
		(e("ande") && nn) ||
		e("else") ||
		(e("este") && bm) ||
		(e("aste") && nn) ||
		(e("eren") && bm) ||
		(e("aren") && nn)) {
		return n - 4
	}
	if n > 5 && ((e("ere") && bm) ||
		(e("are") && nn) ||
		(e("est") && bm) ||
		(e("ast") && nn) ||
		e("ene") ||
		(e("ane") && nn)) {
		return n - 3
	}
	if n > 4 && (e("er") ||
		e("en") ||
		e("et") ||
		(e("ar") && nn) ||
		(e("st") && bm) ||
		e("te")) {
		return n - 2
	}
	if n > 3 {
		switch s[n-1] {
		case 'a', 'e', 'n':
			return n - 1
		}
	}
	return n
}

// norwegianMinimalStem ports org.apache.lucene.analysis.no.NorwegianMinimalStemmer.
// flags: 1 = bokmål, 2 = nynorsk, 3 = both (other values panic, as Lucene throws).
func norwegianMinimalStem(term string, flags int) string {
	_, nn := gmcNorwegianCheckFlags(flags)
	s := []rune(term)
	n := len(s)
	if n > 4 && s[n-1] == 's' {
		n--
	}
	return string(s[:gmcNorwegianMinimalLen(s, n, nn)])
}

func gmcNorwegianMinimalLen(s []rune, n int, nn bool) int {
	if n > 5 && (gmcEndsWith(s, n, "ene") || (gmcEndsWith(s, n, "ane") && nn)) {
		return n - 3
	}
	if n > 4 && (gmcEndsWith(s, n, "er") ||
		gmcEndsWith(s, n, "en") ||
		gmcEndsWith(s, n, "et") ||
		(gmcEndsWith(s, n, "ar") && nn)) {
		return n - 2
	}
	if n > 3 {
		switch s[n-1] {
		case 'a', 'e':
			return n - 1
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Finnish

// finnishLightStem ports org.apache.lucene.analysis.fi.FinnishLightStemmer.
func finnishLightStem(term string) string {
	s := []rune(term)
	n := len(s)
	if n < 4 {
		return term
	}
	for i := 0; i < n; i++ {
		switch s[i] {
		case 'ä', 'å':
			s[i] = 'a'
		case 'ö':
			s[i] = 'o'
		}
	}
	n = gmcFinnishStep1(s, n)
	n = gmcFinnishStep2(s, n)
	n = gmcFinnishStep3(s, n)
	n = gmcFinnishNorm1(s, n)
	n = gmcFinnishNorm2(s, n)
	return string(s[:n])
}

func gmcFinnishIsVowel(ch rune) bool {
	switch ch {
	case 'a', 'e', 'i', 'o', 'u', 'y':
		return true
	}
	return false
}

func gmcFinnishStep1(s []rune, n int) int {
	if n > 8 {
		if gmcEndsWith(s, n, "kin") {
			return gmcFinnishStep1(s, n-3)
		}
		if gmcEndsWith(s, n, "ko") {
			return gmcFinnishStep1(s, n-2)
		}
	}
	if n > 11 {
		if gmcEndsWith(s, n, "dellinen") {
			return n - 8
		}
		if gmcEndsWith(s, n, "dellisuus") {
			return n - 9
		}
	}
	return n
}

func gmcFinnishStep2(s []rune, n int) int {
	if n > 5 {
		if gmcEndsWith(s, n, "lla") || gmcEndsWith(s, n, "tse") || gmcEndsWith(s, n, "sti") {
			return n - 3
		}
		if gmcEndsWith(s, n, "ni") {
			return n - 2
		}
		if gmcEndsWith(s, n, "aa") {
			return n - 1
		}
	}
	return n
}

func gmcFinnishStep3(s []rune, n int) int {
	if n > 8 {
		if gmcEndsWith(s, n, "nnen") {
			s[n-4] = 's'
			return n - 3
		}
		if gmcEndsWith(s, n, "ntena") {
			s[n-5] = 's'
			return n - 4
		}
		if gmcEndsWith(s, n, "tten") {
			return n - 4
		}
		if gmcEndsWith(s, n, "eiden") {
			return n - 5
		}
	}
	if n > 6 {
		if gmcEndsWith(s, n, "neen") ||
			gmcEndsWith(s, n, "niin") ||
			gmcEndsWith(s, n, "seen") ||
			gmcEndsWith(s, n, "teen") ||
			gmcEndsWith(s, n, "inen") {
			return n - 4
		}
		if s[n-3] == 'h' && gmcFinnishIsVowel(s[n-2]) && s[n-1] == 'n' {
			return n - 3
		}
		if gmcEndsWith(s, n, "den") {
			s[n-3] = 's'
			return n - 2
		}
		if gmcEndsWith(s, n, "ksen") {
			s[n-4] = 's'
			return n - 3
		}
		if gmcEndsWith(s, n, "ssa") ||
			gmcEndsWith(s, n, "sta") ||
			gmcEndsWith(s, n, "lla") ||
			gmcEndsWith(s, n, "lta") ||
			gmcEndsWith(s, n, "tta") ||
			gmcEndsWith(s, n, "ksi") ||
			gmcEndsWith(s, n, "lle") {
			return n - 3
		}
	}
	if n > 5 {
		if gmcEndsWith(s, n, "na") || gmcEndsWith(s, n, "ne") {
			return n - 2
		}
		if gmcEndsWith(s, n, "nei") {
			return n - 3
		}
	}
	if n > 4 {
		if gmcEndsWith(s, n, "ja") || gmcEndsWith(s, n, "ta") {
			return n - 2
		}
		if s[n-1] == 'a' {
			return n - 1
		}
		if s[n-1] == 'n' && gmcFinnishIsVowel(s[n-2]) {
			return n - 2
		}
		if s[n-1] == 'n' {
			return n - 1
		}
	}
	return n
}

func gmcFinnishNorm1(s []rune, n int) int {
	if n > 5 && gmcEndsWith(s, n, "hde") {
		s[n-3] = 'k'
		s[n-2] = 's'
		s[n-1] = 'i'
	}
	if n > 4 {
		if gmcEndsWith(s, n, "ei") || gmcEndsWith(s, n, "at") {
			return n - 2
		}
	}
	if n > 3 {
		switch s[n-1] {
		case 't', 's', 'j', 'e', 'a', 'i':
			return n - 1
		}
	}
	return n
}

func gmcFinnishNorm2(s []rune, n int) int {
	if n > 8 {
		if s[n-1] == 'e' || s[n-1] == 'o' || s[n-1] == 'u' {
			n--
		}
	}
	if n > 4 {
		if s[n-1] == 'i' {
			n--
		}
		if n > 4 {
			ch := s[0]
			for i := 1; i < n; i++ {
				if s[i] == ch && (ch == 'k' || ch == 'p' || ch == 't') {
					n = gmcDelete(s, i, n)
					i--
				} else {
					ch = s[i]
				}
			}
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Hungarian

// hungarianLightStem ports org.apache.lucene.analysis.hu.HungarianLightStemmer.
func hungarianLightStem(term string) string {
	s := []rune(term)
	n := len(s)
	for i := 0; i < n; i++ {
		switch s[i] {
		case 'á':
			s[i] = 'a'
		case 'ë', 'é':
			s[i] = 'e'
		case 'í':
			s[i] = 'i'
		case 'ó', 'ő', 'õ', 'ö':
			s[i] = 'o'
		case 'ú', 'ű', 'ũ', 'û', 'ü':
			s[i] = 'u'
		}
	}
	n = gmcHungarianRemoveCase(s, n)
	n = gmcHungarianRemovePossessive(s, n)
	n = gmcHungarianRemovePlural(s, n)
	n = gmcHungarianNormalize(s, n)
	return string(s[:n])
}

func gmcHungarianRemoveCase(s []rune, n int) int {
	e := func(suf string) bool { return gmcEndsWith(s, n, suf) }
	if n > 6 && e("kent") {
		return n - 4
	}
	if n > 5 {
		if e("nak") || e("nek") || e("val") || e("vel") || e("ert") || e("rol") ||
			e("ban") || e("ben") || e("bol") || e("nal") || e("nel") || e("hoz") ||
			e("hez") || e("tol") {
			return n - 3
		}
		if e("al") || e("el") {
			if !gmcFinnishIsVowel(s[n-3]) && s[n-3] == s[n-4] {
				return n - 3
			}
		}
	}
	if n > 4 {
		if e("at") || e("et") || e("ot") || e("va") || e("ve") || e("ra") ||
			e("re") || e("ba") || e("be") || e("ul") || e("ig") {
			return n - 2
		}
		if (e("on") || e("en")) && !gmcFinnishIsVowel(s[n-3]) {
			return n - 2
		}
		switch s[n-1] {
		case 't', 'n':
			return n - 1
		case 'a', 'e':
			if s[n-2] == s[n-3] && !gmcFinnishIsVowel(s[n-2]) {
				return n - 2
			}
		}
	}
	return n
}

func gmcHungarianRemovePossessive(s []rune, n int) int {
	e := func(suf string) bool { return gmcEndsWith(s, n, suf) }
	if n > 6 {
		if !gmcFinnishIsVowel(s[n-5]) && (e("atok") || e("otok") || e("etek")) {
			return n - 4
		}
		if e("itek") || e("itok") {
			return n - 4
		}
	}
	if n > 5 {
		if !gmcFinnishIsVowel(s[n-4]) && (e("unk") || e("tok") || e("tek")) {
			return n - 3
		}
		if gmcFinnishIsVowel(s[n-4]) && e("juk") {
			return n - 3
		}
		if e("ink") {
			return n - 3
		}
	}
	if n > 4 {
		if !gmcFinnishIsVowel(s[n-3]) &&
			(e("am") || e("em") || e("om") || e("ad") || e("ed") || e("od") || e("uk")) {
			return n - 2
		}
		if gmcFinnishIsVowel(s[n-3]) && (e("nk") || e("ja") || e("je")) {
			return n - 2
		}
		if e("im") || e("id") || e("ik") {
			return n - 2
		}
	}
	if n > 3 {
		switch s[n-1] {
		case 'a', 'e':
			if !gmcFinnishIsVowel(s[n-2]) {
				return n - 1
			}
		case 'm', 'd':
			if gmcFinnishIsVowel(s[n-2]) {
				return n - 1
			}
		case 'i':
			return n - 1
		}
	}
	return n
}

func gmcHungarianRemovePlural(s []rune, n int) int {
	if n > 3 && s[n-1] == 'k' {
		switch s[n-2] {
		case 'a', 'o', 'e':
			if n > 4 {
				return n - 2
			}
			return n - 1
		default:
			return n - 1
		}
	}
	return n
}

func gmcHungarianNormalize(s []rune, n int) int {
	if n > 3 {
		switch s[n-1] {
		case 'a', 'e', 'i', 'o':
			return n - 1
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Russian

// russianLightStem ports org.apache.lucene.analysis.ru.RussianLightStemmer.
func russianLightStem(term string) string {
	s := []rune(term)
	n := gmcRussianRemoveCase(s, len(s))
	n = gmcRussianNormalize(s, n)
	return string(s[:n])
}

func gmcRussianNormalize(s []rune, n int) int {
	if n > 3 {
		switch s[n-1] {
		case 'ь', 'и':
			return n - 1
		case 'н':
			if s[n-2] == 'н' {
				return n - 1
			}
		}
	}
	return n
}

var gmcRussianCase3 = []string{
	"иям", "иях", "оях", "ями", "оям", "оьв", "ами", "его", "ему", "ери",
	"ими", "ого", "ому", "ыми", "оев",
}

var gmcRussianCase2 = []string{
	"ая", "яя", "ях", "юю", "ах", "ею", "их", "ия", "ию", "ьв", "ою", "ую",
	"ям", "ых", "ея", "ам", "ем", "ей", "ём", "ев", "ий", "им", "ое", "ой",
	"ом", "ов", "ые", "ый", "ым", "ми",
}

func gmcRussianRemoveCase(s []rune, n int) int {
	if n > 6 && (gmcEndsWith(s, n, "иями") || gmcEndsWith(s, n, "оями")) {
		return n - 4
	}
	if n > 5 {
		for _, suf := range gmcRussianCase3 {
			if gmcEndsWith(s, n, suf) {
				return n - 3
			}
		}
	}
	if n > 4 {
		for _, suf := range gmcRussianCase2 {
			if gmcEndsWith(s, n, suf) {
				return n - 2
			}
		}
	}
	if n > 3 {
		switch s[n-1] {
		case 'а', 'е', 'и', 'о', 'у', 'й', 'ы', 'я', 'ь':
			return n - 1
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// English

// englishMinimalStem ports org.apache.lucene.analysis.en.EnglishMinimalStemmer.
func englishMinimalStem(term string) string {
	s := []rune(term)
	n := len(s)
	if n < 3 || s[n-1] != 's' {
		return term
	}
	switch s[n-2] {
	case 'u', 's':
		return term
	case 'e':
		if n > 3 && s[n-3] == 'i' && s[n-4] != 'a' && s[n-4] != 'e' {
			s[n-3] = 'y'
			return string(s[:n-2])
		}
		if s[n-3] == 'i' || s[n-3] == 'a' || s[n-3] == 'o' || s[n-3] == 'e' {
			return term
		}
	}
	return string(s[:n-1])
}

// englishPossessive ports org.apache.lucene.analysis.en.EnglishPossessiveFilter.
func englishPossessive(term string) string {
	s := []rune(term)
	n := len(s)
	if n >= 2 && (s[n-2] == '\'' || s[n-2] == '’' || s[n-2] == '＇') &&
		(s[n-1] == 's' || s[n-1] == 'S') {
		return string(s[:n-2])
	}
	return term
}

// ---------------------------------------------------------------------------
// Scandinavian

// scandinavianFoldLucene ports org.apache.lucene.analysis.miscellaneous.ScandinavianFoldingFilter.
func scandinavianFoldLucene(term string) string {
	b := []rune(term)
	n := len(b)
	for i := 0; i < n; i++ {
		c := b[i]
		switch {
		case c == 'å' || c == 'ä' || c == 'æ':
			b[i] = 'a'
		case c == 'Å' || c == 'Ä' || c == 'Æ':
			b[i] = 'A'
		case c == 'ø' || c == 'ö':
			b[i] = 'o'
		case c == 'Ø' || c == 'Ö':
			b[i] = 'O'
		case n-1 > i:
			d := b[i+1]
			if (c == 'a' || c == 'A') &&
				(d == 'a' || d == 'A' || d == 'e' || d == 'E' || d == 'o' || d == 'O') {
				n = gmcDelete(b, i+1, n)
			} else if (c == 'o' || c == 'O') && (d == 'e' || d == 'E' || d == 'o' || d == 'O') {
				n = gmcDelete(b, i+1, n)
			}
		}
	}
	return string(b[:n])
}

// scandinavianNormalizeLucene ports
// org.apache.lucene.analysis.miscellaneous.ScandinavianNormalizationFilter
// (ScandinavianNormalizer with ALL_FOLDINGS: AA, AO, AE, OE, OO).
func scandinavianNormalizeLucene(term string) string {
	b := []rune(term)
	n := len(b)
	for i := 0; i < n; i++ {
		c := b[i]
		switch {
		case c == 'ä':
			b[i] = 'æ'
		case c == 'Ä':
			b[i] = 'Æ'
		case c == 'ö':
			b[i] = 'ø'
		case c == 'Ö':
			b[i] = 'Ø'
		case n-1 > i:
			d := b[i+1]
			aaOrAo := d == 'a' || d == 'A' || d == 'o' || d == 'O'
			isE := d == 'e' || d == 'E'
			oeOrOo := isE || d == 'o' || d == 'O'
			switch {
			case c == 'a' && aaOrAo:
				n = gmcDelete(b, i+1, n)
				b[i] = 'å'
			case c == 'A' && aaOrAo:
				n = gmcDelete(b, i+1, n)
				b[i] = 'Å'
			case c == 'a' && isE:
				n = gmcDelete(b, i+1, n)
				b[i] = 'æ'
			case c == 'A' && isE:
				n = gmcDelete(b, i+1, n)
				b[i] = 'Æ'
			case c == 'o' && oeOrOo:
				n = gmcDelete(b, i+1, n)
				b[i] = 'ø'
			case c == 'O' && oeOrOo:
				n = gmcDelete(b, i+1, n)
				b[i] = 'Ø'
			}
		}
	}
	return string(b[:n])
}
