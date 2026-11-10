package main

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Analysis framework: analyzers, tokenizers, token filters and the _analyze API.

// analyzeWithIndex tokenizes text with a named analyzer, resolving custom
// analyzers configured in index settings. Falls back to the built-in set.
var englishStopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "but": true, "by": true, "for": true, "if": true, "in": true,
	"into": true, "is": true, "it": true, "no": true, "not": true, "of": true,
	"on": true, "or": true, "such": true, "that": true, "the": true, "their": true,
	"then": true, "there": true, "these": true, "they": true, "this": true,
	"to": true, "was": true, "will": true, "with": true, "were": true, "through": true,
}

func englishStem(word string) string {
	w := strings.ToLower(word)
	if len(w) <= 3 {
		return w
	}
	switch {
	case strings.HasSuffix(w, "ies") && len(w) > 4:
		return w[:len(w)-3] + "i"
	case strings.HasSuffix(w, "ing") && len(w) > 5:
		w = w[:len(w)-3]
		if len(w) > 2 && w[len(w)-1] == w[len(w)-2] && !strings.ContainsRune("aeiou", rune(w[len(w)-1])) {
			w = w[:len(w)-1]
		}
		return w
	case strings.HasSuffix(w, "ed") && len(w) > 4:
		w = w[:len(w)-2]
		if len(w) > 2 && w[len(w)-1] == w[len(w)-2] && !strings.ContainsRune("aeiou", rune(w[len(w)-1])) {
			w = w[:len(w)-1]
		}
		return w
	case strings.HasSuffix(w, "es") && len(w) > 3:
		return w[:len(w)-2]
	case strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && len(w) > 3:
		return w[:len(w)-1]
	}
	return w
}

func simpleAnalyze(text string) []string {
	out := []string{}
	var buf []rune
	flush := func() {
		if len(buf) > 0 {
			out = append(out, strings.ToLower(string(buf)))
			buf = buf[:0]
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) {
			buf = append(buf, r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

type analyzedToken struct {
	term    string
	pos     int
	start   int
	end     int
	typ     string
	posLen  int
	keyword bool
}

// runeWidth is the number of UTF-16 code units the rune occupies. Lucene
// reports token offsets in UTF-16 code units, not bytes or code points.
func runeWidth(r rune) int {
	if r > 0xFFFF {
		return 2
	}
	return 1
}

func utf16Offset(text string) int {
	n := 0
	for _, r := range text {
		n += runeWidth(r)
	}
	return n
}

func isHanRune(r rune) bool {
	return (r >= 0x3400 && r <= 0x4DBF) || (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0xF900 && r <= 0xFAFF) || (r >= 0x20000 && r <= 0x2FA1F)
}

func isHangulRune(r rune) bool {
	return (r >= 0xAC00 && r <= 0xD7AF) || (r >= 0x1100 && r <= 0x11FF) ||
		(r >= 0x3130 && r <= 0x318F) || (r >= 0xFFA0 && r <= 0xFFDC)
}

func isHiraganaRune(r rune) bool {
	return (r >= 0x3040 && r <= 0x309F)
}

func isKatakanaRune(r rune) bool {
	return (r >= 0x30A0 && r <= 0x30FF) || (r >= 0x31F0 && r <= 0x31FF) ||
		(r >= 0xFF66 && r <= 0xFF9D)
}

func isSEARune(r rune) bool {
	return (r >= 0x0E00 && r <= 0x0E7F) || (r >= 0x0E80 && r <= 0x0EFF) ||
		(r >= 0x1000 && r <= 0x109F) || (r >= 0x1780 && r <= 0x17FF)
}

func isEmojiStart(r rune) bool {
	return (r >= 0x1F000 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) ||
		(r >= 0x2B00 && r <= 0x2BFF) || r == 0x2764
}

func isEmojiPart(r rune) bool {
	return isEmojiStart(r) || r == 0x200D || r == 0xFE0F ||
		(r >= 0x1F3FB && r <= 0x1F3FF) || (r >= 0xE0020 && r <= 0xE007F)
}

func standardTokenType(rs []rune) string {
	numeric := len(rs) > 0
	for _, r := range rs {
		if !unicode.IsDigit(r) && r != '.' && r != ',' {
			numeric = false
			break
		}
	}
	if numeric {
		return "<NUM>"
	}
	return "<ALPHANUM>"
}

func tokenTypeForRunes(rs []rune) string {
	if len(rs) == 0 {
		return "<ALPHANUM>"
	}
	allHangul, allHiragana, allKatakana := true, true, true
	for _, r := range rs {
		if !isHangulRune(r) {
			allHangul = false
		}
		if !isHiraganaRune(r) {
			allHiragana = false
		}
		if !isKatakanaRune(r) {
			allKatakana = false
		}
	}
	switch {
	case allHangul:
		return "<HANGUL>"
	case allHiragana:
		return "<HIRAGANA>"
	case allKatakana:
		return "<KATAKANA>"
	}
	return standardTokenType(rs)
}

func simpleCaseFold(s string) string {
	s = strings.ToLower(s)
	// Lucene's lowercase filter uses simple case folding: Greek capital sigma
	// always maps to sigma, never to final sigma.
	s = strings.ReplaceAll(s, "ς", "σ")
	return s
}

// splitLongToken splits a token at maxLen UTF-16 code units.
func splitLongToken(term string, start, end, pos int, typ string, maxLen int) []analyzedToken {
	if maxLen <= 0 {
		maxLen = 255
	}
	if end-start <= maxLen {
		return []analyzedToken{{term: term, start: start, end: end, pos: pos, typ: typ}}
	}
	out := []analyzedToken{}
	rs := []rune(term)
	ri := 0
	cur := start
	for cur < end {
		chunkEnd := cur + maxLen
		if chunkEnd > end {
			chunkEnd = end
		}
		var chunk []rune
		width := 0
		for ri < len(rs) {
			w := runeWidth(rs[ri])
			if width+w > chunkEnd-cur {
				break
			}
			chunk = append(chunk, rs[ri])
			width += w
			ri++
		}
		out = append(out, analyzedToken{term: string(chunk), start: cur, end: chunkEnd, pos: pos, typ: typ})
		pos++
		cur = chunkEnd
	}
	return out
}

func tokenizePositions(text string, simple bool) []analyzedToken {
	return tokenizePositionsCase(text, simple, true, 255)
}

func tokenizePositionsCase(text string, simple, lower bool, maxLen int) []analyzedToken {
	out := []analyzedToken{}
	rs := []rune(text)
	offs := make([]int, len(rs)+1)
	for i, r := range rs {
		offs[i+1] = offs[i] + runeWidth(r)
	}
	var buf []rune
	pos := 0
	startIdx := 0
	emit := func(term string, start, end int, typ string) {
		chunks := splitLongToken(term, start, end, pos, typ, maxLen)
		out = append(out, chunks...)
		pos += len(chunks)
	}
	flush := func(endIdx int) {
		if len(buf) == 0 {
			return
		}
		typ := "word"
		if !simple {
			typ = tokenTypeForRunes(buf)
		}
		term := string(buf)
		if lower {
			term = simpleCaseFold(term)
		}
		emit(term, offs[startIdx], offs[endIdx], typ)
		buf = buf[:0]
	}
	isWord := func(r rune) bool {
		if unicode.IsLetter(r) || unicode.IsMark(r) {
			return true
		}
		return !simple && (unicode.IsDigit(r) || r == '_')
	}
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case !simple && isEmojiStart(r):
			flush(i)
			j := i + 1
			for j < len(rs) && isEmojiPart(rs[j]) {
				j++
			}
			term := string(rs[i:j])
			if lower {
				term = simpleCaseFold(term)
			}
			emit(term, offs[i], offs[j], "<EMOJI>")
			i = j - 1
		case !simple && isSEARune(r):
			flush(i)
			j := i + 1
			for j < len(rs) && isSEARune(rs[j]) {
				j++
			}
			term := string(rs[i:j])
			if lower {
				term = simpleCaseFold(term)
			}
			emit(term, offs[i], offs[j], "<SOUTHEAST_ASIAN>")
			i = j - 1
		case isHanRune(r) && !simple:
			flush(i)
			term := string(r)
			if lower {
				term = simpleCaseFold(term)
			}
			emit(term, offs[i], offs[i+1], "<IDEOGRAPHIC>")
		case !simple && isHangulRune(r):
			flush(i)
			j := i + 1
			for j < len(rs) && isHangulRune(rs[j]) {
				j++
			}
			term := string(rs[i:j])
			if lower {
				term = simpleCaseFold(term)
			}
			emit(term, offs[i], offs[j], "<HANGUL>")
			i = j - 1
		case !simple && isKatakanaRune(r):
			flush(i)
			j := i + 1
			for j < len(rs) && isKatakanaRune(rs[j]) {
				j++
			}
			term := string(rs[i:j])
			if lower {
				term = simpleCaseFold(term)
			}
			emit(term, offs[i], offs[j], "<KATAKANA>")
			i = j - 1
		case !simple && isHiraganaRune(r):
			flush(i)
			term := string(r)
			if lower {
				term = simpleCaseFold(term)
			}
			emit(term, offs[i], offs[i+1], "<HIRAGANA>")
		case isWord(r):
			if len(buf) == 0 {
				startIdx = i
			}
			buf = append(buf, r)
		case !simple && r == '.' && len(buf) > 0 && i+1 < len(rs) &&
			isWord(buf[len(buf)-1]) && isWord(rs[i+1]) && rs[i-1] != '.':
			buf = append(buf, r)
		case !simple && r == ',' && len(buf) > 0 && i+1 < len(rs) &&
			unicode.IsDigit(buf[len(buf)-1]) && unicode.IsDigit(rs[i+1]):
			buf = append(buf, r)
		case !simple && r == '\'' && len(buf) > 0 && i+1 < len(rs) &&
			unicode.IsLetter(buf[len(buf)-1]) && unicode.IsLetter(rs[i+1]):
			buf = append(buf, r)
		default:
			flush(i)
		}
	}
	flush(len(rs))
	return out
}

type synonymGroup struct {
	phrases [][]string
}

func synonymGroups(idx *Index, analyzer string) []synonymGroup {
	if idx == nil || analyzer == "" {
		return nil
	}
	raw, ok := findSetting(idx.Settings, "analysis.analyzer."+analyzer+".filter")
	if !ok {
		return nil
	}
	names := []string{}
	switch t := raw.(type) {
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok {
				names = append(names, s)
			}
		}
	case string:
		v := strings.TrimSpace(t)
		if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
			v = strings.TrimSpace(v[1 : len(v)-1])
		}
		for _, s := range strings.Fields(v) {
			if s != "" {
				names = append(names, s)
			}
		}
	}
	groups := []synonymGroup{}
	for _, name := range names {
		typ, _ := findSetting(idx.Settings, "analysis.filter."+name+".type")
		ts, _ := typ.(string)
		if ts != "synonym" && ts != "synonym_graph" {
			continue
		}
		syn, _ := findSetting(idx.Settings, "analysis.filter."+name+".synonyms")
		lines := []string{}
		switch t := syn.(type) {
		case []any:
			for _, e := range t {
				if s, ok := e.(string); ok {
					lines = append(lines, s)
				}
			}
		case string:
			v := strings.TrimSpace(t)
			if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
				v = strings.TrimSpace(v[1 : len(v)-1])
			}
			lines = append(lines, v)
		}
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, ",")
			phrases := [][]string{}
			for _, p := range parts {
				terms := strings.Fields(strings.ToLower(strings.TrimSpace(p)))
				if len(terms) > 0 {
					phrases = append(phrases, terms)
				}
			}
			if len(phrases) > 0 {
				groups = append(groups, synonymGroup{phrases: phrases})
			}
		}
	}
	return groups
}

func synonymAlternatives(idx *Index, analyzer, text string) ([][]string, bool) {
	groups := synonymGroups(idx, analyzer)
	if len(groups) == 0 {
		return nil, false
	}
	base := tokenizePositions(text, false)
	terms := make([]string, 0, len(base))
	for _, t := range base {
		terms = append(terms, t.term)
	}
	alts := [][]string{terms}
	matched := false
	for _, g := range groups {
		for _, phrase := range g.phrases {
			for i := 0; i+len(phrase) <= len(terms); i++ {
				ok := true
				for j, p := range phrase {
					if terms[i+j] != p {
						ok = false
						break
					}
				}
				if !ok {
					continue
				}
				for _, alt := range g.phrases {
					if len(alt) == len(phrase) {
						same := true
						for j := range alt {
							if alt[j] != phrase[j] {
								same = false
								break
							}
						}
						if same {
							continue
						}
					}
					alts = append(alts, alt)
					matched = true
				}
			}
		}
	}
	return alts, matched
}

func analyzeWithSynonyms(idx *Index, analyzer, text string) ([]analyzedToken, bool) {
	groups := synonymGroups(idx, analyzer)
	if len(groups) == 0 {
		return nil, false
	}
	base := tokenizePositions(text, false)
	out := append([]analyzedToken{}, base...)
	for _, g := range groups {
		for _, phrase := range g.phrases {
			for i := 0; i+len(phrase) <= len(base); i++ {
				ok := true
				for j, p := range phrase {
					if base[i+j].term != p {
						ok = false
						break
					}
				}
				if !ok {
					continue
				}
				for _, alt := range g.phrases {
					spanStart := base[i].start
					spanEnd := base[i+len(phrase)-1].end
					for j, term := range alt {
						out = append(out, analyzedToken{term: term, pos: base[i].pos + j, start: spanStart, end: spanEnd, typ: "SYNONYM"})
					}
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].pos != out[j].pos {
			return out[i].pos < out[j].pos
		}
		return out[i].term < out[j].term
	})
	dedup := out[:0]
	lastTerm, lastPos := "", -1
	for _, t := range out {
		if t.term == lastTerm && t.pos == lastPos {
			continue
		}
		dedup = append(dedup, t)
		lastTerm, lastPos = t.term, t.pos
	}
	return dedup, true
}

var asciiFoldReplacer = strings.NewReplacer(
	"ß", "ss", "ẞ", "SS", "æ", "ae", "Æ", "AE", "œ", "oe", "Œ", "OE",
	"ø", "o", "Ø", "O", "ł", "l", "Ł", "L", "đ", "d", "Đ", "D", "þ", "th", "Þ", "TH",
	"à", "a", "á", "a", "â", "a", "ã", "a", "ä", "a", "å", "a",
	"À", "A", "Á", "A", "Â", "A", "Ã", "A", "Ä", "A", "Å", "A",
	"ç", "c", "Ç", "C", "è", "e", "é", "e", "ê", "e", "ë", "e",
	"È", "E", "É", "E", "Ê", "E", "Ë", "E", "ì", "i", "í", "i", "î", "i", "ï", "i",
	"Ì", "I", "Í", "I", "Î", "I", "Ï", "I", "ñ", "n", "Ñ", "N",
	"ò", "o", "ó", "o", "ô", "o", "õ", "o", "ö", "o",
	"Ò", "O", "Ó", "O", "Ô", "O", "Õ", "O", "Ö", "O",
	"ù", "u", "ú", "u", "û", "u", "ü", "u",
	"Ù", "U", "Ú", "U", "Û", "U", "Ü", "U", "ý", "y", "ÿ", "y", "Ý", "Y",
)

func filterNames(raw any) []string {
	out := []string{}
	switch t := raw.(type) {
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
	case string:
		v := strings.TrimSpace(t)
		if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
			v = strings.TrimSpace(v[1 : len(v)-1])
		}
		out = append(out, strings.Fields(v)...)
	}
	return out
}

func keywordAnalyzedTokens(text string) []analyzedToken {
	return []analyzedToken{{term: text, start: 0, end: utf16Offset(text), pos: 0, typ: "word"}}
}

func whitespaceAnalyzedTokens(text string) []analyzedToken {
	rs := []rune(text)
	offs := make([]int, len(rs)+1)
	for i, r := range rs {
		offs[i+1] = offs[i] + runeWidth(r)
	}
	out := []analyzedToken{}
	pos := 0
	i := 0
	for i < len(rs) {
		if unicode.IsSpace(rs[i]) {
			i++
			continue
		}
		j := i
		for j < len(rs) && !unicode.IsSpace(rs[j]) {
			j++
		}
		out = append(out, analyzedToken{term: string(rs[i:j]), start: offs[i], end: offs[j], pos: pos, typ: "word"})
		pos++
		i = j
	}
	return out
}

func patternAnalyzedTokens(text string, lower bool) []analyzedToken {
	rs := []rune(text)
	offs := make([]int, len(rs)+1)
	for i, r := range rs {
		offs[i+1] = offs[i] + runeWidth(r)
	}
	out := []analyzedToken{}
	pos := 0
	i := 0
	isWord := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
	}
	for i < len(rs) {
		if !isWord(rs[i]) {
			i++
			continue
		}
		j := i
		for j < len(rs) && isWord(rs[j]) {
			j++
		}
		term := string(rs[i:j])
		if lower {
			term = strings.ToLower(term)
		}
		out = append(out, analyzedToken{term: term, start: offs[i], end: offs[j], pos: pos, typ: "word"})
		pos++
		i = j
	}
	return out
}

func fingerprintAnalyzedToken(text string) []analyzedToken {
	terms := []string{}
	seen := map[string]bool{}
	for _, t := range tokenizePositions(text, false) {
		if !seen[t.term] {
			seen[t.term] = true
			terms = append(terms, t.term)
		}
	}
	sort.Strings(terms)
	return []analyzedToken{{term: strings.Join(terms, " "), start: 0, end: utf16Offset(text), pos: 0, typ: "fingerprint"}}
}

func cjkAnalyzedTokens(text string) []analyzedToken {
	rs := []rune(text)
	offs := make([]int, len(rs)+1)
	for i, r := range rs {
		offs[i+1] = offs[i] + runeWidth(r)
	}
	out := []analyzedToken{}
	pos := 0
	i := 0
	for i < len(rs) {
		r := rs[i]
		if isHanRune(r) || isHangulRune(r) {
			j := i
			for j < len(rs) && (isHanRune(rs[j]) || isHangulRune(rs[j])) {
				j++
			}
			runes := rs[i:j]
			if len(runes) == 1 {
				out = append(out, analyzedToken{term: strings.ToLower(string(runes[0])), start: offs[i], end: offs[i+1], pos: pos, typ: "<SINGLE>"})
				pos++
			} else {
				for k := 0; k+1 < len(runes); k++ {
					out = append(out, analyzedToken{term: strings.ToLower(string(runes[k : k+2])), start: offs[i+k], end: offs[i+k+2], pos: pos, typ: "<DOUBLE>"})
					pos++
				}
			}
			i = j
			continue
		}
		// Non-CJK run: use the standard tokenizer with lowercasing.
		j := i
		for j < len(rs) && !isHanRune(rs[j]) && !isHangulRune(rs[j]) {
			j++
		}
		for _, t := range tokenizePositions(string(rs[i:j]), false) {
			delta := offs[i]
			t.start += delta
			t.end += delta
			t.pos += pos
			out = append(out, t)
		}
		pos += len(tokenizePositions(string(rs[i:j]), false))
		i = j
	}
	return out
}

func applyGlobalCharFilter(text, name string) (string, bool) {
	switch name {
	case "html_strip":
		var b strings.Builder
		inTag := false
		for _, r := range text {
			if r == '<' {
				inTag = true
				continue
			}
			if inTag {
				if r == '>' {
					inTag = false
				}
				continue
			}
			b.WriteRune(r)
		}
		return b.String(), true
	case "lowercase":
		return strings.ToLower(text), true
	case "uppercase":
		return strings.ToUpper(text), true
	}
	return "", false
}

var globalTokenizers = map[string]bool{
	"standard": true, "keyword": true, "whitespace": true, "pattern": true,
	"lowercase": true, "letter": true, "classic": true, "uax_url_email": true,
	"simple_pattern": true, "simple_pattern_split": true, "char_group": true,
	"path_hierarchy": true, "ngram": true, "edge_ngram": true, "thai": true,
}

var globalFilters = map[string]bool{
	"lowercase": true, "uppercase": true, "asciifolding": true, "stop": true,
	"trim": true, "porter_stem": true, "english": true, "snowball": true,
	"keyword_marker": true, "length": true, "reverse": true, "unique": true,
	"word_delimiter": true, "word_delimiter_graph": true, "shingle": true,
	"edge_ngram": true, "ngram": true, "synonym": true, "synonym_graph": true,
	"stemmer": true, "kstem": true, "stemmer_override": true, "keyword_repeat": true,
	"remove_duplicates": true, "common_grams": true,
	"cjk_bigram": true, "cjk_width": true, "truncate": true, "elision": true, "limit": true,
	"pattern_replace": true, "fingerprint": true, "multiplexer": true,
	"pattern_capture": true, "predicate_token_filter": true,
	"dictionary_decompounder": true, "hyphenation_decompounder": true,
	"delimited_payload": true, "delimited_payload_filter": true,
	"flatten_graph": true, "min_hash": true, "hunspell": true,
	"arabic_normalization": true, "persian_normalization": true,
	"german_normalization": true, "hindi_normalization": true,
	"indic_normalization": true, "sorani_normalization": true,
	"serbian_normalization": true, "scandinavian_folding": true,
	"scandinavian_normalization": true, "decimal_digit": true,
	"turkish_lowercase": true, "irish_lowercase": true,
}

var globalCharFilters = map[string]bool{
	"html_strip": true, "mapping": true, "pattern_replace": true,
}

func paramInt(params map[string]any, key string, def int) int {
	if params != nil {
		if n, ok := toFloat(params[key]); ok {
			return int(n)
		}
	}
	return def
}

func paramStrings(params map[string]any, key string) []string {
	if params == nil {
		return nil
	}
	return filterNames(params[key])
}

func tokenizerPatternRegexp(params map[string]any) (*regexp.Regexp, *apiError) {
	pat := `\W+`
	if params != nil {
		if v, ok := params["pattern"].(string); ok {
			pat = v
		}
	}
	flags := paramStrings(params, "flags")
	for _, f := range flags {
		if strings.EqualFold(f, "CASE_INSENSITIVE") {
			pat = "(?i)" + pat
		}
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		reason := err.Error()
		if strings.Contains(reason, "missing closing )") {
			reason = "Unclosed group near index " + strconv.Itoa(len(pat)) + "\n" + pat
			return nil, &apiError{Type: "pattern_syntax_exception", Status: 400, Reason: reason,
				RootCause: &apiError{Type: "pattern_syntax_exception", Status: 400, Reason: reason}}
		}
		return nil, &apiError{Type: "pattern_syntax_exception", Status: 400, Reason: reason,
			RootCause: &apiError{Type: "pattern_syntax_exception", Status: 400, Reason: reason}}
	}
	return re, nil
}

func tokenizeRunsByClass(text string, allow func(rune) bool, maxLen int, lower bool) []analyzedToken {
	rs := []rune(text)
	offs := make([]int, len(rs)+1)
	for i, r := range rs {
		offs[i+1] = offs[i] + runeWidth(r)
	}
	out := []analyzedToken{}
	pos := 0
	i := 0
	for i < len(rs) {
		if !allow(rs[i]) {
			i++
			continue
		}
		j := i
		for j < len(rs) && allow(rs[j]) {
			j++
		}
		term := string(rs[i:j])
		if lower {
			term = simpleCaseFold(term)
		}
		chunks := splitLongToken(term, offs[i], offs[j], pos, "word", maxLen)
		out = append(out, chunks...)
		pos += len(chunks)
		i = j
	}
	return out
}

func ngramTextTokens(text string, minGram, maxGram int, edge, preserve bool, sideBack bool, chars []string, customChars string, maxLen int) ([]analyzedToken, *apiError) {
	allowed := func(r rune) bool {
		if len(chars) == 0 {
			return true
		}
		for _, c := range chars {
			switch c {
			case "letter":
				if unicode.IsLetter(r) {
					return true
				}
			case "digit":
				if unicode.IsDigit(r) {
					return true
				}
			case "whitespace":
				if unicode.IsSpace(r) {
					return true
				}
			case "punctuation":
				if unicode.IsPunct(r) || unicode.IsSymbol(r) {
					return true
				}
			case "symbol":
				if unicode.IsSymbol(r) {
					return true
				}
			case "custom":
				if strings.ContainsRune(customChars, r) {
					return true
				}
			default:
				if len([]rune(c)) == 1 && []rune(c)[0] == r {
					return true
				}
			}
		}
		return false
	}
	rs := []rune(text)
	offs := make([]int, len(rs)+1)
	for i, r := range rs {
		offs[i+1] = offs[i] + runeWidth(r)
	}
	out := []analyzedToken{}
	pos := 0
	i := 0
	for i < len(rs) {
		if !allowed(rs[i]) {
			i++
			continue
		}
		j := i
		for j < len(rs) && allowed(rs[j]) {
			j++
		}
		runes := rs[i:j]
		if edge {
			if sideBack {
				for n := minGram; n <= maxGram && n <= len(runes); n++ {
					out = append(out, analyzedToken{term: string(runes[len(runes)-n:]), start: offs[i], end: offs[j], pos: pos, typ: "word"})
					pos++
				}
			} else {
				for n := minGram; n <= maxGram && n <= len(runes); n++ {
					out = append(out, analyzedToken{term: string(runes[:n]), start: offs[i], end: offs[j], pos: pos, typ: "word"})
					pos++
				}
			}
			if preserve {
				out = append(out, analyzedToken{term: string(runes), start: offs[i], end: offs[j], pos: pos, typ: "word"})
				pos++
			}
		} else {
			for k := 0; k < len(runes); k++ {
				for n := minGram; n <= maxGram && k+n <= len(runes); n++ {
					out = append(out, analyzedToken{term: string(runes[k : k+n]), start: offs[i], end: offs[j], pos: pos, typ: "word"})
					pos++
				}
			}
		}
		i = j
	}
	return out, nil
}

func pathHierarchyTokens(text string, params map[string]any) []analyzedToken {
	delim := "/"
	repl := "/"
	if params != nil {
		if v, ok := params["delimiter"].(string); ok && v != "" {
			delim = v
		}
		if v, ok := params["replacement"].(string); ok {
			repl = v
		}
	}
	skip := paramInt(params, "skip", 0)
	reverse := false
	if params != nil {
		if v, ok := params["reverse"].(bool); ok {
			reverse = v
		}
	}
	type seg struct {
		text  string
		start int
		end   int
	}
	offsets := []int{}
	off := 0
	for _, r := range text {
		offsets = append(offsets, off)
		off += runeWidth(r)
	}
	offsets = append(offsets, off)
	parts := []seg{}
	last := 0
	for i := 0; i+len(delim) <= len(text); {
		if text[i:i+len(delim)] == delim {
			parts = append(parts, seg{text: text[last:i], start: last, end: i})
			last = i + len(delim)
			i += len(delim)
			continue
		}
		i++
	}
	parts = append(parts, seg{text: text[last:], start: last, end: len(text)})
	out := []analyzedToken{}
	pos := 0
	if reverse {
		n := len(parts) - 1 - skip
		for k := 0; k <= n; k++ {
			start := parts[k].start
			end := parts[n].end
			tok := text[start:end]
			if k < n {
				tok = tok + repl
			}
			out = append(out, analyzedToken{term: tok, start: offsets[start], end: offsets[end], pos: pos, typ: "word"})
			pos++
		}
		return out
	}
	if skip < len(parts) {
		for k := skip; k < len(parts); k++ {
			var start int
			if k == 0 {
				start = 0
			} else {
				start = parts[k-1].end
			}
			end := parts[k].end
			tok := text[start:end]
			if k > 0 && repl != delim {
				tok = repl + text[parts[k].start:end]
			}
			out = append(out, analyzedToken{term: tok, start: offsets[start], end: offsets[end], pos: pos, typ: "word"})
			pos++
		}
	}
	return out
}

func charGroupTokens(text string, params map[string]any, maxLen int) []analyzedToken {
	chars := paramStrings(params, "tokenize_on_chars")
	splitOn := func(r rune) bool {
		for _, c := range chars {
			switch c {
			case "whitespace":
				if unicode.IsSpace(r) {
					return true
				}
			case "punctuation":
				if unicode.IsPunct(r) {
					return true
				}
			case "symbol":
				if unicode.IsSymbol(r) {
					return true
				}
			case "letter":
				if unicode.IsLetter(r) {
					return true
				}
			case "digit":
				if unicode.IsDigit(r) {
					return true
				}
			default:
				if len([]rune(c)) == 1 && []rune(c)[0] == r {
					return true
				}
			}
		}
		return false
	}
	rs := []rune(text)
	offs := make([]int, len(rs)+1)
	for i, r := range rs {
		offs[i+1] = offs[i] + runeWidth(r)
	}
	out := []analyzedToken{}
	pos := 0
	i := 0
	for i < len(rs) {
		if splitOn(rs[i]) {
			i++
			continue
		}
		j := i
		for j < len(rs) && !splitOn(rs[j]) {
			j++
		}
		chunks := splitLongToken(string(rs[i:j]), offs[i], offs[j], pos, "word", maxLen)
		out = append(out, chunks...)
		pos += len(chunks)
		i = j
	}
	return out
}

func uaxTokens(text string, maxLen int) []analyzedToken {
	// Recognize emails and URLs first, then tokenize the remaining runs.
	re := regexp.MustCompile(`(?:https?://|www\.)[^\s]+|[\w.+-]+@[\w.-]+\.[A-Za-z]{2,}`)
	matches := re.FindAllStringIndex(text, -1)
	out := []analyzedToken{}
	pos := 0
	last := 0
	addStandard := func(part string, base int) {
		offs := 0
		for _, t := range tokenizePositionsCase(part, false, false, maxLen) {
			t.start += base
			t.end += base
			t.pos += pos
			out = append(out, t)
			offs = t.pos - pos + 1
		}
		_ = offs
		pos += len(tokenizePositionsCase(part, false, false, maxLen))
	}
	for _, m := range matches {
		addStandard(text[last:m[0]], last)
		tok := text[m[0]:m[1]]
		typ := "<URL>"
		if strings.Contains(tok, "@") && !strings.HasPrefix(tok, "http") {
			typ = "<EMAIL>"
		}
		if maxLen > 0 && utf16Offset(tok) > maxLen && typ == "<EMAIL>" {
			at := strings.Index(tok, "@")
			addStandard(tok[:at], m[0])
			out = append(out, analyzedToken{term: tok[at+1:], start: utf16Offset(text[:m[0]+at+1]), end: utf16Offset(text[:m[1]]), pos: pos, typ: "<URL>"})
			pos++
		} else {
			out = append(out, analyzedToken{term: tok, start: utf16Offset(text[:m[0]]), end: utf16Offset(text[:m[1]]), pos: pos, typ: typ})
			pos++
		}
		last = m[1]
	}
	addStandard(text[last:], last)
	return out
}

func buildTokenizerTokens(name string, params map[string]any, text string) ([]analyzedToken, *apiError) {
	maxLen := paramInt(params, "max_token_length", 255)
	switch name {
	case "", "standard":
		return tokenizePositionsCase(text, false, false, maxLen), nil
	case "keyword":
		return []analyzedToken{{term: text, start: 0, end: utf16Offset(text), pos: 0, typ: "word"}}, nil
	case "whitespace":
		base := whitespaceAnalyzedTokens(text)
		out := []analyzedToken{}
		pos := 0
		for _, t := range base {
			chunks := splitLongToken(t.term, t.start, t.end, pos, "word", maxLen)
			out = append(out, chunks...)
			pos += len(chunks)
		}
		return out, nil
	case "lowercase":
		return []analyzedToken{{term: simpleCaseFold(text), start: 0, end: utf16Offset(text), pos: 0, typ: "word"}}, nil
	case "letter":
		return tokenizeRunsByClass(text, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsMark(r) }, maxLen, false), nil
	case "uax_url_email":
		return uaxTokens(text, maxLen), nil
	case "classic":
		return uaxTokens(text, maxLen), nil
	case "ngram", "edge_ngram":
		minGram := paramInt(params, "min_gram", 1)
		maxGram := paramInt(params, "max_gram", 2)
		if name == "edge_ngram" && minGram > maxGram {
			return nil, &apiError{Type: "illegal_argument_exception", Status: 400, Reason: "minGram must not be greater than maxGram",
				RootCause: &apiError{Type: "illegal_argument_exception", Status: 400, Reason: "minGram must not be greater than maxGram"}}
		}
		chars := paramStrings(params, "token_chars")
		for _, c := range chars {
			switch c {
			case "letter", "digit", "whitespace", "punctuation", "symbol", "custom":
			default:
				if len([]rune(c)) != 1 {
					reason := "Unknown token type: '" + c + "', must be one of [letter, digit, whitespace, punctuation, symbol, custom]"
					return nil, &apiError{Type: "illegal_argument_exception", Status: 400, Reason: reason,
						RootCause: &apiError{Type: "illegal_argument_exception", Status: 400, Reason: reason}}
				}
			}
		}
		if name == "ngram" {
			limit := paramInt(params, "_max_ngram_diff", 1)
			if maxGram-minGram > limit {
				reason := "The difference between max_gram and min_gram in NGram Tokenizer must be less than or equal to: [" + strconv.Itoa(limit) + "] but was [" + strconv.Itoa(maxGram-minGram) + "]. This limit can be set by changing the [index.max_ngram_diff] index level setting."
				return nil, &apiError{Type: "illegal_argument_exception", Status: 400, Reason: reason,
					RootCause: &apiError{Type: "illegal_argument_exception", Status: 400, Reason: reason}}
			}
		}
		custom := ""
		if params != nil {
			if v, ok := params["custom_token_chars"].(string); ok {
				custom = v
			}
		}
		hasCustom := false
		for _, c := range chars {
			if c == "custom" {
				hasCustom = true
			}
		}
		if hasCustom && custom == "" {
			reason := "Token type: 'custom' requires setting `custom_token_chars`"
			return nil, &apiError{Type: "illegal_argument_exception", Status: 400, Reason: reason,
				RootCause: &apiError{Type: "illegal_argument_exception", Status: 400, Reason: reason}}
		}
		preserve := false
		if params != nil {
			if v, ok := params["preserve_original"].(bool); ok {
				preserve = v
			}
		}
		sideBack := false
		if params != nil {
			if v, ok := params["side"].(string); ok && v == "back" {
				sideBack = true
			}
		}
		return ngramTextTokens(text, minGram, maxGram, name == "edge_ngram", preserve, sideBack, chars, custom, maxLen)
	case "pattern":
		re, apiErr := tokenizerPatternRegexp(params)
		if apiErr != nil {
			return nil, apiErr
		}
		group := paramInt(params, "group", -1)
		lower := true
		if params != nil {
			if v, ok := params["lowercase"].(bool); ok {
				lower = v
			}
		}
		out := []analyzedToken{}
		pos := 0
		if group >= 0 {
			for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
				if 2*group+1 >= len(m) || m[2*group] < 0 {
					continue
				}
				start, end := m[2*group], m[2*group+1]
				term := text[start:end]
				if lower {
					term = simpleCaseFold(term)
				}
				out = append(out, analyzedToken{term: term, start: utf16Offset(text[:start]), end: utf16Offset(text[:end]), pos: pos, typ: "word"})
				pos++
			}
			return out, nil
		}
		last := 0
		for _, m := range re.FindAllStringIndex(text, -1) {
			if m[0] > last {
				term := text[last:m[0]]
				if lower {
					term = simpleCaseFold(term)
				}
				out = append(out, analyzedToken{term: term, start: utf16Offset(text[:last]), end: utf16Offset(text[:m[0]]), pos: pos, typ: "word"})
				pos++
			}
			last = m[1]
		}
		if last < len(text) {
			term := text[last:]
			if lower {
				term = simpleCaseFold(term)
			}
			out = append(out, analyzedToken{term: term, start: utf16Offset(text[:last]), end: utf16Offset(text[len(text):]), pos: pos, typ: "word"})
			pos++
		}
		return out, nil
	case "simple_pattern", "simple_pattern_split":
		pat := ""
		if params != nil {
			pat, _ = params["pattern"].(string)
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, analyzeRequestError("illegal_argument_exception", err.Error())
		}
		out := []analyzedToken{}
		pos := 0
		if name == "simple_pattern" {
			for _, m := range re.FindAllStringIndex(text, -1) {
				if m[0] == m[1] {
					continue
				}
				out = append(out, analyzedToken{term: text[m[0]:m[1]], start: utf16Offset(text[:m[0]]), end: utf16Offset(text[:m[1]]), pos: pos, typ: "word"})
				pos++
			}
			return out, nil
		}
		last := 0
		for _, m := range re.FindAllStringIndex(text, -1) {
			if m[0] > last {
				out = append(out, analyzedToken{term: text[last:m[0]], start: utf16Offset(text[:last]), end: utf16Offset(text[:m[0]]), pos: pos, typ: "word"})
				pos++
			}
			last = m[1]
		}
		if last < len(text) {
			out = append(out, analyzedToken{term: text[last:], start: utf16Offset(text[:last]), end: utf16Offset(text[len(text):]), pos: pos, typ: "word"})
			pos++
		}
		return out, nil
	case "char_group":
		return charGroupTokens(text, params, maxLen), nil
	case "path_hierarchy":
		return pathHierarchyTokens(text, params), nil
	case "thai":
		// The Thai tokenizer dictionary is not bundled; fall back to grouping
		// Thai runs so the offsets and types stay sensible.
		return tokenizePositionsCase(text, false, false, maxLen), nil
	}
	return nil, analyzeRequestError("illegal_argument_exception", "failed to find global tokenizer under ["+name+"]")
}

func globalTokenizerTokens(name, text string) ([]analyzedToken, bool) {
	toks, err := buildTokenizerTokens(name, nil, text)
	if err != nil {
		return nil, false
	}
	return toks, true
}

func globalAnalyzerTokens(name, text string) ([]analyzedToken, bool) {
	switch name {
	case "", "standard":
		return tokenizePositions(text, false), true
	case "simple":
		return tokenizePositions(text, true), true
	case "whitespace":
		return whitespaceAnalyzedTokens(text), true
	case "keyword":
		return keywordAnalyzedTokens(text), true
	case "stop":
		out := []analyzedToken{}
		for _, t := range tokenizePositions(text, true) {
			if !englishStopWords[t.term] {
				out = append(out, t)
			}
		}
		return out, true
	case "pattern":
		return patternAnalyzedTokens(text, true), true
	case "fingerprint":
		return fingerprintAnalyzedToken(text), true
	case "lowercase":
		return []analyzedToken{{term: strings.ToLower(text), start: 0, end: utf16Offset(text), pos: 0, typ: "word"}}, true
	case "cjk":
		return cjkAnalyzedTokens(text), true
	case "english":
		out := []analyzedToken{}
		for _, t := range tokenizePositions(text, false) {
			if englishStopWords[t.term] {
				continue
			}
			t.term = englishStem(t.term)
			out = append(out, t)
		}
		return out, true
	}
	if builtinAnalyzer(name) {
		// Language analyzers share the standard tokenizer upstream; stemming is
		// implemented separately where the language is supported.
		return tokenizePositions(text, false), true
	}
	return nil, false
}

// applyGlobalCharComponents runs anonymous char filters, returning the filtered
// text plus boundary/insertion data used to remap token offsets.
func applyGlobalCharComponents(comps []analyzeComponent, text string) (string, []int, []int, bool) {
	return runCharFilterSpecs(globalCharSpecs(comps), text)
}

// analyzeTokensGlobal builds tokens for a global _analyze request, including
// anonymous tokenizer/char_filter/filter components.
func analyzeTokensGlobal(analyzer, tokenizer string, tokenizerParams map[string]any, charFilters []analyzeComponent, filters []analyzeComponent, text string) ([]analyzedToken, bool) {
	original := text
	originalLen := utf16Offset(text)
	text, boundary, insertions, ok := applyGlobalCharComponents(charFilters, text)
	if !ok {
		return nil, false
	}
	var toks []analyzedToken
	if analyzer == "" && tokenizer == "" && len(filters) == 0 && len(charFilters) == 0 {
		// A bare _analyze request uses the standard analyzer, which includes
		// the lowercase filter (unlike the bare standard tokenizer).
		analyzer = "standard"
	}
	if analyzer != "" {
		toks, ok = globalAnalyzerTokens(analyzer, text)
		if !ok {
			return nil, false
		}
	} else {
		var apiErr *apiError
		toks, apiErr = buildTokenizerTokens(tokenizer, tokenizerParams, text)
		if apiErr != nil {
			return nil, false
		}
	}
	remapFilteredTokens(toks, boundary, insertions, originalLen)
	var ferr *apiError
	toks, ferr = applyFilterComponents(nil, toks, filters)
	if ferr != nil {
		return nil, false
	}
	_ = original
	return toks, true
}

func normalizerExistsForIndex(idx *Index, name string) bool {
	if idx == nil {
		return builtinNormalizer(name)
	}
	if builtinNormalizer(name) {
		return true
	}
	_, ok := findSetting(idx.Settings, "analysis.normalizer."+name+".type")
	return ok
}

func applyIndexNormalizer(idx *Index, name string, toks []analyzedToken) []analyzedToken {
	filterNamesList := []string{name}
	if idx != nil {
		if raw, ok := findSetting(idx.Settings, "analysis.normalizer."+name+".filter"); ok {
			filterNamesList = filterNames(raw)
		}
	}
	for _, f := range filterNamesList {
		switch f {
		case "lowercase":
			for i := range toks {
				toks[i].term = strings.ToLower(toks[i].term)
				toks[i].typ = "word"
			}
		case "uppercase":
			for i := range toks {
				toks[i].term = strings.ToUpper(toks[i].term)
				toks[i].typ = "word"
			}
		case "asciifolding":
			for i := range toks {
				toks[i].term = asciiFoldReplacer.Replace(toks[i].term)
				toks[i].typ = "word"
			}
		}
	}
	return toks
}

// applyIndexCharFilters runs named char filters configured on an index.
func applyIndexCharFilters(idx *Index, names []string, text string) (string, []int, []int, bool) {
	if idx == nil || len(names) == 0 {
		return text, nil, nil, true
	}
	return runCharFilterSpecs(indexCharSpecs(idx, names), text)
}

func indexTokenizerType(idx *Index, name string) (string, bool) {
	if idx == nil || name == "" {
		return "", false
	}
	raw, ok := findSetting(idx.Settings, "analysis.tokenizer."+name+".type")
	if !ok {
		return "", false
	}
	t, _ := raw.(string)
	return t, true
}

func resolveTokenizerTokens(idx *Index, name, text string) ([]analyzedToken, bool) {
	orig := name
	if typ, ok := indexTokenizerType(idx, name); ok {
		name = typ
		if typ == "ngram" || typ == "edge_ngram" {
			minGram, maxGram := 1, 2
			if n, ok := settingInt(idx.Settings, "analysis.tokenizer."+orig+".min_gram"); ok {
				minGram = n
			}
			if n, ok := settingInt(idx.Settings, "analysis.tokenizer."+orig+".max_gram"); ok {
				maxGram = n
			}
			return ngramTokens(text, minGram, maxGram, typ == "edge_ngram"), true
		}
	}
	return globalTokenizerTokens(name, text)
}

func ngramTokens(text string, minGram, maxGram int, edge bool) []analyzedToken {
	rs := []rune(text)
	offs := make([]int, len(rs)+1)
	for i, r := range rs {
		offs[i+1] = offs[i] + runeWidth(r)
	}
	out := []analyzedToken{}
	pos := 0
	for i := 0; i < len(rs); i++ {
		for n := minGram; n <= maxGram && i+n <= len(rs); n++ {
			out = append(out, analyzedToken{term: strings.ToLower(string(rs[i : i+n])), start: offs[i], end: offs[i+n], pos: pos, typ: "word"})
			pos++
			if edge {
				break
			}
		}
	}
	return out
}

func synonymExpandedTokens(idx *Index, analyzer string, toks []analyzedToken) []analyzedToken {
	groups := synonymGroups(idx, analyzer)
	if len(groups) == 0 {
		return toks
	}
	out := append([]analyzedToken{}, toks...)
	for _, g := range groups {
		for _, phrase := range g.phrases {
			for i := 0; i+len(phrase) <= len(toks); i++ {
				ok := true
				for j, p := range phrase {
					if toks[i+j].term != p {
						ok = false
						break
					}
				}
				if !ok {
					continue
				}
				for _, alt := range g.phrases {
					spanStart := toks[i].start
					spanEnd := toks[i+len(phrase)-1].end
					for j, term := range alt {
						out = append(out, analyzedToken{term: term, pos: toks[i].pos + j, start: spanStart, end: spanEnd, typ: "SYNONYM"})
					}
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].pos != out[j].pos {
			return out[i].pos < out[j].pos
		}
		return out[i].term < out[j].term
	})
	dedup := out[:0]
	lastTerm, lastPos := "", -1
	for _, t := range out {
		if t.term == lastTerm && t.pos == lastPos {
			continue
		}
		dedup = append(dedup, t)
		lastTerm, lastPos = t.term, t.pos
	}
	return dedup
}

func analyzeIndexComponentTokens(idx *Index, tokenizer string, charComps, filterComps []analyzeComponent, text string) []analyzedToken {
	cnames := make([]string, 0, len(charComps))
	for _, c := range charComps {
		cnames = append(cnames, c.name)
	}
	originalLen := utf16Offset(text)
	filtered, boundary, insertions, _ := applyIndexCharFilters(idx, cnames, text)
	if tokenizer == "" {
		tokenizer = "keyword"
	}
	toks, ok := resolveTokenizerTokens(idx, tokenizer, filtered)
	if !ok {
		toks = tokenizePositions(filtered, false)
	}
	toks, _ = applyFilterComponents(idx, toks, filterComps)
	remapFilteredTokens(toks, boundary, insertions, originalLen)
	return toks
}

func analyzeCustomWithIndex(idx *Index, analyzer, text string) ([]analyzedToken, bool) {
	if idx == nil || analyzer == "" {
		return nil, false
	}
	rawType, hasType := findSetting(idx.Settings, "analysis.analyzer."+analyzer+".type")
	analyzerType, _ := rawType.(string)
	rawTok, hasTok := findSetting(idx.Settings, "analysis.analyzer."+analyzer+".tokenizer")
	tok, _ := rawTok.(string)
	if !hasTok && !hasType {
		return nil, false
	}
	if analyzerType == "custom" || (!hasTok && analyzerType == "") {
		if !hasTok {
			return nil, false
		}
	} else if !hasTok && analyzerType != "custom" {
		tok = analyzerType
	}

	cfNames := filterNames(mustFindSetting(idx, "analysis.analyzer."+analyzer+".char_filter"))
	originalLen := utf16Offset(text)
	filtered, boundary, insertions, _ := applyIndexCharFilters(idx, cfNames, text)
	var toks []analyzedToken
	var ok bool
	if analyzerType == "custom" || hasTok {
		toks, ok = resolveTokenizerTokens(idx, tok, filtered)
		if !ok {
			toks = tokenizePositions(filtered, false)
		}
	} else {
		toks, ok = globalAnalyzerTokens(tok, filtered)
		if !ok {
			toks = tokenizePositions(filtered, false)
		}
	}
	rawFilters, _ := findSetting(idx.Settings, "analysis.analyzer."+analyzer+".filter")
	fnames := filterNames(rawFilters)
	apply := []string{}
	for _, f := range fnames {
		if t, ok := findSetting(idx.Settings, "analysis.filter."+f+".type"); ok {
			ts, _ := t.(string)
			if ts == "synonym" || ts == "synonym_graph" {
				continue
			}
			if ts == "stop" {
				// Custom stopword lists are applied below.
				continue
			}
		}
		apply = append(apply, f)
	}
	comps := make([]analyzeComponent, 0, len(apply))
	for _, f := range apply {
		comps = append(comps, analyzeComponent{name: f})
	}
	toks, _ = applyFilterComponents(idx, toks, comps)
	for _, f := range fnames {
		typ, _ := findSetting(idx.Settings, "analysis.filter."+f+".type")
		ts, _ := typ.(string)
		if ts == "stop" {
			words := map[string]bool{}
			if raw, ok := findSetting(idx.Settings, "analysis.filter."+f+".stopwords"); ok {
				for _, w := range filterNames(raw) {
					words[strings.ToLower(w)] = true
				}
			}
			if len(words) > 0 {
				out := toks[:0]
				for _, t := range toks {
					if !words[strings.ToLower(t.term)] {
						out = append(out, t)
					}
				}
				toks = out
			}
		}
	}
	toks = synonymExpandedTokens(idx, analyzer, toks)
	remapFilteredTokens(toks, boundary, insertions, originalLen)
	return toks, true
}

func mustFindSetting(idx *Index, key string) any {
	v, _ := findSetting(idx.Settings, key)
	return v
}

func analyzeTokensWithIndex(idx *Index, analyzer, text string) []analyzedToken {
	if toks, ok := analyzeCustomWithIndex(idx, analyzer, text); ok {
		return toks
	}
	if toks, ok := analyzeWithSynonyms(idx, analyzer, text); ok {
		return toks
	}
	if toks, ok := globalAnalyzerTokens(analyzer, text); ok {
		return toks
	}
	return tokenizePositions(text, false)
}

func analyzeWithIndex(idx *Index, analyzer, text string) []string {
	if toks, ok := analyzeWithSynonyms(idx, analyzer, text); ok {
		out := make([]string, 0, len(toks))
		for _, t := range toks {
			out = append(out, t.term)
		}
		return out
	}
	if toks, ok := analyzeCustomWithIndex(idx, analyzer, text); ok {
		out := make([]string, 0, len(toks))
		for _, t := range toks {
			out = append(out, t.term)
		}
		return out
	}
	switch analyzer {
	case "simple":
		return simpleAnalyze(text)
	case "stop":
		toks := simpleAnalyze(text)
		out := make([]string, 0, len(toks))
		for _, t := range toks {
			if !englishStopWords[t] {
				out = append(out, t)
			}
		}
		return out
	case "english":
		toks := analyze(text)
		out := make([]string, 0, len(toks))
		for _, t := range toks {
			if englishStopWords[t] {
				continue
			}
			out = append(out, englishStem(t))
		}
		return out
	default:
		if analyzer == "" || analyzer == "standard" {
			out := []string{}
			for _, t := range tokenizePositions(text, false) {
				out = append(out, t.term)
			}
			return out
		}
		return analyzeWith(analyzer, text)
	}
}

func analyzeRequestError(etype, reason string) *apiError {
	return &apiError{Type: etype, Status: 400, Reason: reason,
		RootCause: &apiError{Type: etype, Status: 400, Reason: reason}}
}

func analyzeStringValues(raw any) ([]string, bool) {
	switch t := raw.(type) {
	case string:
		return []string{t}, true
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, len(out) > 0
	}
	return nil, false
}

type analyzeComponent struct {
	name   string
	inline map[string]any
}

func parseAnalyzeComponents(raw any) ([]analyzeComponent, *apiError) {
	if raw == nil {
		return nil, nil
	}
	out := []analyzeComponent{}
	list, ok := raw.([]any)
	if !ok {
		list = []any{raw}
	}
	for _, e := range list {
		switch t := e.(type) {
		case string:
			out = append(out, analyzeComponent{name: t})
		case map[string]any:
			typ, _ := t["type"].(string)
			if typ == "" {
				enc, _ := json.Marshal(t)
				return nil, analyzeRequestError("illegal_argument_exception",
					"Missing [type] setting for anonymous filter: "+string(enc))
			}
			out = append(out, analyzeComponent{name: typ, inline: t})
		}
	}
	return out, nil
}

func analyzePositionGap(def map[string]any) int {
	if def != nil {
		if n, ok := toFloat(def["position_increment_gap"]); ok {
			return int(n)
		}
		if n, ok := toFloat(def["position_offset_gap"]); ok {
			return int(n)
		}
	}
	return 100
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request, name string) {
	var idx *Index
	if name != "" {
		s.mu.RLock()
		var apiErr *apiError
		idx, apiErr = s.getIndex(name)
		s.mu.RUnlock()
		if apiErr != nil {
			writeError(w, apiErr)
			return
		}
	}
	body, apiErr := readJSON(r)
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}

	texts, hasText := analyzeStringValues(body["text"])
	if !hasText {
		writeError(w, analyzeRequestError("action_request_validation_exception", "Validation Failed: 1: text is missing;"))
		return
	}

	analyzer, _ := body["analyzer"].(string)
	normalizer, _ := body["normalizer"].(string)
	tokenizerName := ""
	tokenizerParams := map[string]any{}
	if raw, ok := body["tokenizer"]; ok {
		switch t := raw.(type) {
		case string:
			tokenizerName = t
		case map[string]any:
			tokenizerParams = t
			tokenizerName, _ = t["type"].(string)
			if tokenizerName == "" {
				tokenizerName = "standard"
			}
		}
	}
	filters, ferr := parseAnalyzeComponents(body["filter"])
	if ferr != nil {
		writeError(w, ferr)
		return
	}
	charFilters, cerr := parseAnalyzeComponents(body["char_filter"])
	if cerr != nil {
		writeError(w, cerr)
		return
	}

	if normalizer != "" {
		if idx == nil {
			writeError(w, analyzeRequestError("action_request_validation_exception",
				"Validation Failed: 1: index is required if normalizer is specified;"))
			return
		}
		if analyzer != "" || tokenizerName != "" || len(filters) > 0 || len(charFilters) > 0 {
			writeError(w, analyzeRequestError("action_request_validation_exception",
				"Validation Failed: 1: index is required if normalizer is specified;2: tokenizer/analyze should be null if normalizer is specified;3: cannot define extra components on a named normalizer;"))
			return
		}
	}
	if analyzer != "" && (tokenizerName != "" || len(filters) > 0 || len(charFilters) > 0) {
		writeError(w, analyzeRequestError("action_request_validation_exception",
			"Validation Failed: 1: cannot define extra components on a named analyzer;"))
		return
	}

	var def map[string]any
	if field, ok := body["field"].(string); ok && field != "" {
		if idx == nil {
			writeError(w, analyzeRequestError("illegal_argument_exception", "analysis based on a specific field requires an index"))
			return
		}
		def, _ = mappingFieldDef(idx, field)
		if def != nil {
			if t, _ := def["type"].(string); t != "" && t != "text" && t != "match_only_text" && t != "search_as_you_type" && t != "keyword" {
				writeError(w, analyzeRequestError("illegal_argument_exception",
					"Can't process field ["+field+"], Analysis requests are only supported on tokenized fields"))
				return
			}
			if a, ok := def["search_analyzer"].(string); ok && a != "" {
				analyzer = a
			} else if a, ok := def["analyzer"].(string); ok && a != "" {
				analyzer = a
			} else if n, ok := def["normalizer"].(string); ok && n != "" {
				normalizer = n
			} else if t, _ := def["type"].(string); t == "keyword" {
				analyzer = "keyword"
			} else if _, ok := findSetting(idx.Settings, "analysis.analyzer.default.tokenizer"); ok {
				analyzer = "default"
			} else if _, ok := findSetting(idx.Settings, "analysis.analyzer.default.type"); ok {
				analyzer = "default"
			} else {
				// Text fields without an explicit analyzer use the standard analyzer.
				analyzer = "standard"
			}
		}
	}

	// Validate and resolve components against the global or index context.
	globalCtx := idx == nil
	knownAnalyzer := func(a string) bool {
		if a == "" {
			return true
		}
		if globalCtx {
			_, ok := globalAnalyzerTokens(a, "")
			return ok
		}
		if builtinAnalyzer(a) {
			return true
		}
		if _, ok := findSetting(idx.Settings, "analysis.analyzer."+a+".type"); ok {
			return true
		}
		_, ok := findSetting(idx.Settings, "analysis.analyzer."+a+".tokenizer")
		return ok
	}
	if !knownAnalyzer(analyzer) {
		if globalCtx {
			writeError(w, analyzeRequestError("illegal_argument_exception", "failed to find global analyzer ["+analyzer+"]"))
		} else {
			writeError(w, analyzeRequestError("illegal_argument_exception", "failed to find analyzer ["+analyzer+"]"))
		}
		return
	}
	knownFilter := func(f string) bool {
		if knownFilterTypes[f] {
			return true
		}
		if !globalCtx {
			if _, ok := findSetting(idx.Settings, "analysis.filter."+f+".type"); ok {
				return true
			}
		}
		return false
	}
	for _, f := range filters {
		if !knownFilter(f.name) {
			if globalCtx {
				writeError(w, analyzeRequestError("illegal_argument_exception", "failed to find global filter under ["+f.name+"]"))
			} else {
				writeError(w, analyzeRequestError("illegal_argument_exception", "failed to find filter under ["+f.name+"]"))
			}
			return
		}
		spec, ok := resolveFilterSpec(idx, f)
		if ok {
			dummy := []analyzedToken{{term: "x", start: 0, end: 1, typ: "word"}}
			if _, apiErr := applyFilterOne(idx, dummy, spec); apiErr != nil {
				writeError(w, apiErr)
				return
			}
		}
	}
	if tokenizerName != "" {
		if !globalTokenizers[tokenizerName] {
			if _, ok := indexTokenizerType(idx, tokenizerName); !ok {
				if globalCtx {
					writeError(w, analyzeRequestError("illegal_argument_exception", "failed to find global tokenizer under ["+tokenizerName+"]"))
				} else {
					writeError(w, analyzeRequestError("illegal_argument_exception", "failed to find tokenizer under ["+tokenizerName+"]"))
				}
				return
			}
		}
		if _, apiErr := buildTokenizerTokens(tokenizerName, tokenizerParams, "x"); apiErr != nil && globalCtx {
			writeError(w, apiErr)
			return
		}
	}
	for _, cf := range charFilters {
		ok := globalCharFilters[cf.name]
		if !ok && !globalCtx {
			if _, exists := findSetting(idx.Settings, "analysis.char_filter."+cf.name+".type"); exists {
				ok = true
			}
		}
		if !ok {
			writeError(w, analyzeRequestError("illegal_argument_exception", "failed to find global char_filter under ["+cf.name+"]"))
			return
		}
	}
	if normalizer != "" && !normalizerExistsForIndex(idx, normalizer) {
		writeError(w, analyzeRequestError("illegal_argument_exception", "failed to find normalizer under ["+normalizer+"]"))
		return
	}

	var toks []analyzedToken
	componentMode := analyzer == "" && normalizer == ""
	gap := analyzePositionGap(def)
	if analyzer != "" && !componentMode {
		gap = 0
	}
	offsetBase := 0
	posBase := 0
	for ti, text := range texts {
		var part []analyzedToken
		rawCount := 0
		switch {
		case normalizer != "":
			part = []analyzedToken{{term: text, start: 0, end: utf16Offset(text), pos: 0, typ: "word"}}
			part = applyIndexNormalizer(idx, normalizer, part)
			rawCount = 1
		case analyzer != "":
			if idx != nil {
				part = analyzeTokensWithIndex(idx, analyzer, text)
				if part == nil || len(part) == 0 {
					part = []analyzedToken{}
				}
				rawCount = len(tokenizePositions(text, false))
			} else {
				var ok bool
				part, ok = globalAnalyzerTokens(analyzer, text)
				if !ok {
					part = nil
				}
				rawCount = len(tokenizePositions(text, false))
			}
		default:
			cfNames := make([]string, 0, len(charFilters))
			for _, cf := range charFilters {
				cfNames = append(cfNames, cf.name)
			}
			fNames := make([]string, 0, len(filters))
			for _, f := range filters {
				fNames = append(fNames, f.name)
			}
			if idx != nil {
				part = analyzeIndexComponentTokens(idx, tokenizerName, charFilters, filters, text)
				rawCount = len(tokenizePositions(text, false))
			} else {
				var ok bool
				part, ok = analyzeTokensGlobal("", tokenizerName, tokenizerParams, charFilters, filters, text)
				if !ok {
					part = []analyzedToken{}
				}
				var raw []analyzedToken
				raw, _ = analyzeTokensGlobal("", tokenizerName, tokenizerParams, charFilters, nil, text)
				rawCount = len(raw)
			}
		}
		for i := range part {
			part[i].start += offsetBase
			part[i].end += offsetBase
			part[i].pos += posBase
		}
		toks = append(toks, part...)
		l := utf16Offset(text)
		if ti+1 < len(texts) {
			offsetBase += l + 1
		}
		posBase += rawCount + gap
	}

	limit := 10000
	if idx != nil {
		if n, ok := settingInt(idx.Settings, "analyze.max_token_count"); ok {
			limit = n
		}
	}
	if len(toks) > limit {
		writeError(w, &apiError{Type: "illegal_state_exception", Status: 500,
			Reason: "The number of tokens produced by calling _analyze has exceeded the allowed maximum of [" + strconv.Itoa(limit) + "]. This limit can be set by changing the [index.analyze.max_token_count] index level setting.",
			RootCause: &apiError{Type: "illegal_state_exception", Status: 500,
				Reason: "The number of tokens produced by calling _analyze has exceeded the allowed maximum of [" + strconv.Itoa(limit) + "]. This limit can be set by changing the [index.analyze.max_token_count] index level setting."}})
		return
	}

	out := make([]any, 0, len(toks))
	for _, t := range toks {
		typ := t.typ
		if typ == "" {
			typ = "<ALPHANUM>"
		}
		entry := map[string]any{
			"token": t.term, "start_offset": t.start, "end_offset": t.end,
			"type": typ, "position": t.pos,
		}
		if t.posLen > 1 {
			entry["positionLength"] = t.posLen
		}
		out = append(out, entry)
	}
	writeJSON(w, 200, map[string]any{"tokens": out})
}
