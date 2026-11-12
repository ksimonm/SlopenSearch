package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Java java.lang.Character case mapping, as used by Lucene's LowerCaseFilter/UpperCaseFilter
// (CharacterUtils.toLowerCase/toUpperCase: Character.toLowerCase/toUpperCase(int) per code
// point). Go's unicode tables are used except for the code points listed in the generated
// javaLowerDiff/javaUpperDiff tables (case_java_data.go).

// mscJavaLowerRune is Character.toLowerCase(int).
func mscJavaLowerRune(r rune) rune {
	if r < utf8.RuneSelf {
		if 'A' <= r && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}
	if v, ok := javaLowerDiff[r]; ok {
		return v
	}
	return unicode.ToLower(r)
}

// mscJavaUpperRune is Character.toUpperCase(int).
func mscJavaUpperRune(r rune) rune {
	if r < utf8.RuneSelf {
		if 'a' <= r && r <= 'z' {
			return r - ('a' - 'A')
		}
		return r
	}
	if v, ok := javaUpperDiff[r]; ok {
		return v
	}
	return unicode.ToUpper(r)
}

// mscIsJavaMn is Character.getType(cp) == Character.NON_SPACING_MARK.
func mscIsJavaMn(r rune) bool {
	if r < 0x300 {
		return false
	}
	if v, ok := mscJavaMnDiff[r]; ok {
		return v
	}
	return unicode.Is(unicode.Mn, r)
}

// mscMapRunes applies f to every code point of s (invalid UTF-8 bytes are kept as is).
func mscMapRunes(s string, f func(rune) rune) string {
	i := 0
	for i < len(s) {
		c := s[i]
		if c >= utf8.RuneSelf {
			break
		}
		if f(rune(c)) != rune(c) {
			break
		}
		i++
	}
	if i == len(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	b.WriteString(s[:i])
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteByte(s[i])
			i++
			continue
		}
		b.WriteRune(f(r))
		i += size
	}
	return b.String()
}

// luceneLower ports org.apache.lucene.analysis.core.LowerCaseFilter (one token).
func luceneLower(term string) string { return mscMapRunes(term, mscJavaLowerRune) }

// luceneUpper ports org.apache.lucene.analysis.core.UpperCaseFilter (one token).
func luceneUpper(term string) string { return mscMapRunes(term, mscJavaUpperRune) }
