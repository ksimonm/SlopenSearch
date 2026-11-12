package main

import (
	"encoding/json"
	"regexp"
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
		// ExtendNumLet ('_') and MidNum characters keep a token numeric.
		if !unicode.IsDigit(r) && r != '.' && r != ',' && r != '_' {
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
	// Lucene's LowerCaseFilter maps each code point with Java's
	// Character.toLowerCase (capital sigma always becomes medial sigma).
	return luceneLower(s)
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
	if !simple {
		return standardTokenizerTokens(text, lower, maxLen)
	}
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

// asciiFolder folds characters like Lucene's ASCIIFoldingFilter.
type asciiFolder struct{}

var asciiFoldReplacer asciiFolder

func (asciiFolder) Replace(s string) string {
	var b strings.Builder
	changed := false
	for _, r := range s {
		if r >= 0x80 {
			if v, ok := asciiFoldTable[r]; ok {
				b.WriteString(v)
				changed = true
				continue
			}
		}
		b.WriteRune(r)
	}
	if !changed {
		return s
	}
	return b.String()
}

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
	}
	return languageAnalyzerTokens(name, text, nil)
}

// analyzeTokensWithIndex analyzes text with a named analyzer, resolving the
// analyzers configured on the index before the prebuilt ones.
func analyzeTokensWithIndex(idx *Index, analyzer, text string) []analyzedToken {
	if analyzer == "" && idx != nil && (analysisScope{idx: idx}).indexHasAnalyzer("default") {
		analyzer = "default"
	}
	if toks, ok := indexAnalyzerTokens(idx, analyzer, text); ok {
		return toks
	}
	if c := prebuiltChain(analyzer); c != nil && c.builtin == "" {
		if r, err := c.run(idx, text, false); err == nil {
			return r.toks
		}
	}
	if toks, ok := globalAnalyzerTokens(analyzer, text); ok {
		return toks
	}
	return tokenizePositions(text, false)
}

func analyzeWithIndex(idx *Index, analyzer, text string) []string {
	toks := analyzeTokensWithIndex(idx, analyzer, text)
	out := make([]string, 0, len(toks))
	for _, t := range toks {
		out = append(out, t.term)
	}
	return out
}

// patternAnalyzedTokens runs the prebuilt pattern analyzer.
func patternAnalyzedTokens(text string, lower bool) []analyzedToken {
	c, _ := typedAnalyzerChain("pattern", "pattern", map[string]any{"lowercase": lower})
	r, _ := c.run(nil, text, false)
	return r.toks
}

// fingerprintAnalyzedToken runs the prebuilt fingerprint analyzer.
func fingerprintAnalyzedToken(text string) []analyzedToken {
	c, _ := typedAnalyzerChain("fingerprint", "fingerprint", nil)
	r, _ := c.run(nil, text, false)
	return r.toks
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
