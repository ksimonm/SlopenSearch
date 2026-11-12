package main

import (
	"strconv"
	"strings"
	"unicode"
)

// Tokenizers. A tokenizer is built once from its settings (which is where
// configuration errors surface, as in OpenSearch) and then run on texts.
// Offsets are UTF-16 code units of the text handed to the tokenizer.

type tokenizerFunc func(text string) []analyzedToken

// runeOffsets returns the UTF-16 offset of every rune boundary.
func runeOffsets(rs []rune) []int {
	offs := make([]int, len(rs)+1)
	for i, r := range rs {
		offs[i+1] = offs[i] + runeWidth(r)
	}
	return offs
}

// javaIsWhitespace mirrors Character.isWhitespace.
func javaIsWhitespace(r rune) bool {
	switch r {
	case '\t', '\n', 0x0B, '\f', '\r', 0x1C, 0x1D, 0x1E, 0x1F:
		return true
	case 0xA0, 0x2007, 0x202F:
		return false
	}
	return unicode.In(r, unicode.Zs, unicode.Zl, unicode.Zp)
}

func javaIsLetter(r rune) bool {
	return unicode.In(r, unicode.Lu, unicode.Ll, unicode.Lt, unicode.Lm, unicode.Lo)
}

func javaIsPunct(r rune) bool {
	return unicode.In(r, unicode.Ps, unicode.Pe, unicode.Po, unicode.Pc, unicode.Pd, unicode.Pi, unicode.Pf)
}

func javaIsSymbol(r rune) bool {
	return unicode.In(r, unicode.Sc, unicode.Sm, unicode.So, unicode.Sk)
}

// charTokenize implements Lucene's CharTokenizer: runs of token chars, split
// when they reach maxLen UTF-16 units.
func charTokenize(text string, isTokenChar func(rune) bool, normalize func(rune) rune, maxLen int) []analyzedToken {
	if maxLen <= 0 {
		maxLen = 255
	}
	rs := []rune(text)
	offs := runeOffsets(rs)
	out := []analyzedToken{}
	pos := 0
	i := 0
	for i < len(rs) {
		if !isTokenChar(rs[i]) {
			i++
			continue
		}
		start := i
		var term []rune
		length := 0
		for i < len(rs) && isTokenChar(rs[i]) {
			w := runeWidth(rs[i])
			if length+w > maxLen && length > 0 {
				break
			}
			r := rs[i]
			if normalize != nil {
				r = normalize(r)
			}
			term = append(term, r)
			length += w
			i++
			if length >= maxLen {
				break
			}
		}
		out = append(out, analyzedToken{term: string(term), start: offs[start], end: offs[i], pos: pos, typ: "word"})
		pos++
	}
	return out
}

func tokenizerParamError(reason string) *apiError {
	return analyzeRequestError("illegal_argument_exception", reason)
}

// tokenizerTypeKnown lists the tokenizer types available without plugins.
var tokenizerTypeKnown = map[string]bool{
	"standard": true, "keyword": true, "whitespace": true, "pattern": true,
	"lowercase": true, "letter": true, "classic": true, "uax_url_email": true,
	"simple_pattern": true, "simple_pattern_split": true, "char_group": true,
	"path_hierarchy": true, "PathHierarchy": true, "ngram": true, "nGram": true,
	"edge_ngram": true, "edgeNGram": true, "thai": true,
}

// buildTokenizer creates a tokenizer of the given type. maxNgramDiff is the
// index.max_ngram_diff limit in effect.
func buildTokenizer(typ string, params map[string]any, maxNgramDiff int) (tokenizerFunc, *apiError) {
	maxLen := paramInt(params, "max_token_length", 255)
	switch typ {
	case "standard":
		if maxLen > 1024*1024 {
			return nil, tokenizerParamError("maxTokenLength must be greater than 0 and less than 1048576 passed: " + strconv.Itoa(maxLen))
		}
		return func(text string) []analyzedToken { return tokenizePositionsCase(text, false, false, maxLen) }, nil
	case "keyword":
		return func(text string) []analyzedToken {
			return []analyzedToken{{term: text, start: 0, end: utf16Offset(text), pos: 0, typ: "word"}}
		}, nil
	case "whitespace":
		return func(text string) []analyzedToken {
			return charTokenize(text, func(r rune) bool { return !javaIsWhitespace(r) }, nil, maxLen)
		}, nil
	case "letter":
		return func(text string) []analyzedToken { return charTokenize(text, javaIsLetter, nil, maxLen) }, nil
	case "lowercase":
		return func(text string) []analyzedToken {
			return charTokenize(text, javaIsLetter, unicode.ToLower, maxLen)
		}, nil
	case "uax_url_email":
		return func(text string) []analyzedToken { return uaxTokens(text, maxLen) }, nil
	case "classic":
		return func(text string) []analyzedToken { return classicTokens(text, maxLen) }, nil
	case "thai":
		return func(text string) []analyzedToken { return thaiTokens(text) }, nil
	case "char_group":
		return buildCharGroupTokenizer(params, maxLen)
	case "ngram", "nGram", "edge_ngram", "edgeNGram":
		return buildNgramTokenizer(typ == "edge_ngram" || typ == "edgeNGram", params, maxNgramDiff)
	case "pattern":
		return buildPatternTokenizer(params)
	case "simple_pattern", "simple_pattern_split":
		pat := fsString(params, "pattern", "")
		re, err := compileJavaRegexAPI(pat, 0)
		if err != nil {
			return nil, err
		}
		if typ == "simple_pattern" {
			return func(text string) []analyzedToken { return simplePatternTokens(re, text) }, nil
		}
		return func(text string) []analyzedToken { return simplePatternSplitTokens(re, text) }, nil
	case "path_hierarchy", "PathHierarchy":
		return buildPathHierarchyTokenizer(params)
	}
	return nil, nil
}

func buildCharGroupTokenizer(params map[string]any, maxLen int) (tokenizerFunc, *apiError) {
	var classes []func(rune) bool
	chars := map[rune]bool{}
	for _, c := range paramStrings(params, "tokenize_on_chars") {
		rs := []rune(c)
		if len(rs) == 1 {
			chars[rs[0]] = true
			continue
		}
		if strings.HasPrefix(c, "\\") {
			switch c {
			case "\\n":
				chars['\n'] = true
			case "\\t":
				chars['\t'] = true
			case "\\r":
				chars['\r'] = true
			case "\\f":
				chars['\f'] = true
			case "\\\\":
				chars['\\'] = true
			default:
				if strings.HasPrefix(c, "\\u") && len(c) == 6 {
					if v, err := strconv.ParseUint(c[2:], 16, 32); err == nil {
						chars[rune(v)] = true
						continue
					}
				}
				return nil, tokenizerParamError("Unsupported escape [" + c + "]")
			}
			continue
		}
		switch c {
		case "letter":
			classes = append(classes, javaIsLetter)
		case "digit":
			classes = append(classes, func(r rune) bool { return unicode.Is(unicode.Nd, r) })
		case "whitespace":
			classes = append(classes, javaIsWhitespace)
		case "punctuation":
			classes = append(classes, javaIsPunct)
		case "symbol":
			classes = append(classes, javaIsSymbol)
		default:
			return nil, tokenizerParamError("Invalid escaped char in [" + c + "]")
		}
	}
	split := func(r rune) bool {
		if chars[r] {
			return true
		}
		for _, f := range classes {
			if f(r) {
				return true
			}
		}
		return false
	}
	return func(text string) []analyzedToken {
		return charTokenize(text, func(r rune) bool { return !split(r) }, nil, maxLen)
	}, nil
}

// tokenCharMatcher builds the token_chars matcher used by the ngram
// tokenizers; nil means every character is a token character.
func tokenCharMatcher(params map[string]any) (func(rune) bool, *apiError) {
	classes := paramStrings(params, "token_chars")
	if len(classes) == 0 {
		return nil, nil
	}
	var fs []func(rune) bool
	for _, c := range classes {
		c = strings.ToLower(strings.TrimSpace(c))
		switch c {
		case "letter":
			fs = append(fs, javaIsLetter)
		case "digit":
			fs = append(fs, func(r rune) bool { return unicode.Is(unicode.Nd, r) })
		case "whitespace":
			fs = append(fs, javaIsWhitespace)
		case "punctuation":
			fs = append(fs, javaIsPunct)
		case "symbol":
			fs = append(fs, javaIsSymbol)
		case "custom":
			custom, ok := params["custom_token_chars"].(string)
			if !ok {
				return nil, tokenizerParamError("Token type: 'custom' requires setting `custom_token_chars`")
			}
			set := map[rune]bool{}
			for _, r := range custom {
				set[r] = true
			}
			fs = append(fs, func(r rune) bool { return set[r] })
		default:
			return nil, tokenizerParamError("Unknown token type: '" + c + "', must be one of [symbol, digit, letter, whitespace, custom, punctuation]")
		}
	}
	return func(r rune) bool {
		for _, f := range fs {
			if f(r) {
				return true
			}
		}
		return false
	}, nil
}

func buildNgramTokenizer(edge bool, params map[string]any, maxNgramDiff int) (tokenizerFunc, *apiError) {
	minGram := paramInt(params, "min_gram", 1)
	maxGram := paramInt(params, "max_gram", 2)
	if !edge && maxGram-minGram > maxNgramDiff {
		return nil, tokenizerParamError("The difference between max_gram and min_gram in NGram Tokenizer must be less than or equal to: [" +
			strconv.Itoa(maxNgramDiff) + "] but was [" + strconv.Itoa(maxGram-minGram) + "]. This limit can be set by changing the [index.max_ngram_diff] index level setting.")
	}
	if minGram < 1 {
		return nil, tokenizerParamError("minGram must be greater than zero")
	}
	if minGram > maxGram {
		return nil, tokenizerParamError("minGram must not be greater than maxGram")
	}
	isTokenChar, err := tokenCharMatcher(params)
	if err != nil {
		return nil, err
	}
	return func(text string) []analyzedToken {
		return ngramTokenize(text, minGram, maxGram, edge, isTokenChar)
	}, nil
}

// ngramTokenize mirrors Lucene's NGramTokenizer: grams never cross non token
// characters; for each start the grams grow from minGram to maxGram.
func ngramTokenize(text string, minGram, maxGram int, edge bool, isTokenChar func(rune) bool) []analyzedToken {
	rs := []rune(text)
	offs := runeOffsets(rs)
	out := []analyzedToken{}
	pos := 0
	i := 0
	for i < len(rs) {
		if isTokenChar != nil && !isTokenChar(rs[i]) {
			i++
			continue
		}
		j := i
		for j < len(rs) && (isTokenChar == nil || isTokenChar(rs[j])) {
			j++
		}
		for s := i; s < j; s++ {
			if edge && s > i {
				break
			}
			for n := minGram; n <= maxGram && s+n <= j; n++ {
				out = append(out, analyzedToken{term: string(rs[s : s+n]), start: offs[s], end: offs[s+n], pos: pos, typ: "word"})
				pos++
			}
		}
		i = j
	}
	return out
}

func buildPatternTokenizer(params map[string]any) (tokenizerFunc, *apiError) {
	pat := `\W+`
	if v, ok := params["pattern"].(string); ok {
		pat = v
	}
	flags, ferr := javaRegexFlags(strings.Join(paramStrings(params, "flags"), "|"))
	if ferr != nil {
		return nil, ferr
	}
	re, err := compileJavaRegexAPI(pat, flags)
	if err != nil {
		return nil, err
	}
	group := paramInt(params, "group", -1)
	return func(text string) []analyzedToken { return patternTokenize(re, group, text) }, nil
}

func patternTokenize(re *jRegex, group int, text string) []analyzedToken {
	rs := []rune(text)
	offs := runeOffsets(rs)
	out := []analyzedToken{}
	pos := 0
	emit := func(s, e int) {
		out = append(out, analyzedToken{term: string(rs[s:e]), start: offs[s], end: offs[e], pos: pos, typ: "word"})
		pos++
	}
	matches := re.findAll(rs)
	if group >= 0 {
		for _, m := range matches {
			if 2*group+1 >= len(m) || m[2*group] < 0 {
				continue
			}
			if m[2*group] == m[2*group+1] {
				continue
			}
			emit(m[2*group], m[2*group+1])
		}
		return out
	}
	index := 0
	for _, m := range matches {
		if m[0]-index > 0 {
			emit(index, m[0])
		}
		index = m[1]
	}
	if len(rs)-index > 0 {
		emit(index, len(rs))
	}
	return out
}

// longestAt returns the end of the longest match starting exactly at i, or -1.
func (re *jRegex) longestAt(in []rune, i int) int {
	m := &jMatcher{re: re, in: in, caps: make([]int, 2*(re.groups+1))}
	for x := range m.caps {
		m.caps[x] = -1
	}
	best := -1
	m.match(re.root, i, func(e int) bool {
		if e > best {
			best = e
		}
		return best == len(in) || m.steps > 200000
	})
	return best
}

func simplePatternTokens(re *jRegex, text string) []analyzedToken {
	rs := []rune(text)
	offs := runeOffsets(rs)
	out := []analyzedToken{}
	pos := 0
	for i := 0; i < len(rs); {
		e := re.longestAt(rs, i)
		if e > i {
			out = append(out, analyzedToken{term: string(rs[i:e]), start: offs[i], end: offs[e], pos: pos, typ: "word"})
			pos++
			i = e
			continue
		}
		i++
	}
	return out
}

func simplePatternSplitTokens(re *jRegex, text string) []analyzedToken {
	rs := []rune(text)
	offs := runeOffsets(rs)
	out := []analyzedToken{}
	pos := 0
	start := 0
	for i := 0; i < len(rs); {
		e := re.longestAt(rs, i)
		if e > i {
			if i > start {
				out = append(out, analyzedToken{term: string(rs[start:i]), start: offs[start], end: offs[i], pos: pos, typ: "word"})
				pos++
			}
			i = e
			start = e
			continue
		}
		i++
	}
	if start < len(rs) {
		out = append(out, analyzedToken{term: string(rs[start:]), start: offs[start], end: offs[len(rs)], pos: pos, typ: "word"})
	}
	return out
}

func singleCharParam(params map[string]any, key string, def rune) (rune, *apiError) {
	v, ok := params[key].(string)
	if !ok {
		return def, nil
	}
	rs := []rune(v)
	if len(rs) != 1 {
		return 0, tokenizerParamError(key + " must be a one char value")
	}
	return rs[0], nil
}

func buildPathHierarchyTokenizer(params map[string]any) (tokenizerFunc, *apiError) {
	delim, err := singleCharParam(params, "delimiter", '/')
	if err != nil {
		return nil, err
	}
	repl, err := singleCharParam(params, "replacement", delim)
	if err != nil {
		return nil, err
	}
	skip := paramInt(params, "skip", 0)
	reverse := fsBool(params, "reverse", false)
	return func(text string) []analyzedToken {
		if reverse {
			return reversePathTokens(text, delim, repl, skip)
		}
		return pathTokens(text, delim, repl, skip)
	}, nil
}

// pathTokens ports Lucene's PathHierarchyTokenizer.
func pathTokens(text string, delim, repl rune, skip int) []analyzedToken {
	in := []rune(text)
	offs := runeOffsets(in)
	out := []analyzedToken{}
	var result []rune
	startPos := 0
	skipped := 0
	endDelim := false
	p := 0
	for {
		term := append([]rune{}, result...)
		added := false
		extra := []rune{}
		if endDelim {
			extra = append(extra, repl)
			endDelim = false
			added = true
		}
		done := false
		eof := false
		for {
			if p >= len(in) {
				eof = true
				break
			}
			c := in[p]
			p++
			if !added {
				added = true
				skipped++
				if skipped > skip {
					if c == delim {
						extra = append(extra, repl)
					} else {
						extra = append(extra, c)
					}
				} else {
					startPos++
				}
			} else if c == delim {
				if skipped > skip {
					endDelim = true
					break
				}
				skipped++
				if skipped > skip {
					extra = append(extra, repl)
				} else {
					startPos++
				}
			} else if skipped > skip {
				extra = append(extra, c)
			} else {
				startPos++
			}
		}
		if eof {
			if skipped > skip && added {
				term = append(term, extra...)
				end := startPos + len(term)
				out = append(out, analyzedToken{term: string(term), start: offs[min(startPos, len(in))], end: offs[min(end, len(in))], typ: "word"})
			}
			done = true
		} else {
			term = append(term, extra...)
			end := startPos + len(term)
			out = append(out, analyzedToken{term: string(term), start: offs[min(startPos, len(in))], end: offs[min(end, len(in))], typ: "word"})
			result = term
		}
		if done {
			break
		}
	}
	for i := range out {
		out[i].pos = i
	}
	return out
}

func reversePathTokens(text string, delim, repl rune, skip int) []analyzedToken {
	in := []rune(text)
	offs := runeOffsets(in)
	positions := []int{0}
	buf := make([]rune, 0, len(in))
	for i, c := range in {
		if c == delim {
			positions = append(positions, i+1)
			buf = append(buf, repl)
		} else {
			buf = append(buf, c)
		}
	}
	if positions[len(positions)-1] < len(in) {
		positions = append(positions, len(in))
	}
	count := len(positions)
	out := []analyzedToken{}
	idx := count - 1 - skip
	if idx < 0 {
		return out
	}
	end := positions[idx]
	for k := 0; k < count-skip-1; k++ {
		s := positions[k]
		out = append(out, analyzedToken{term: string(buf[s:end]), start: offs[s], end: offs[end], pos: k, typ: "word"})
	}
	return out
}

// thaiTokens approximates Lucene's ThaiTokenizer (dictionary based word
// breaking is not available): runs of letters and digits become words.
func thaiTokens(text string) []analyzedToken {
	return thaiAnalyzedTokens(text)
}

// ---------------------------------------------------------------- classic

var classicRules struct {
	alphanum, apostrophe, acronym, company, email, host, num, acronymDep *jRegex
}

func init() {
	cj := `\x{3100}-\x{312f}\x{3040}-\x{309F}\x{30A0}-\x{30FF}\x{31F0}-\x{31FF}\x{3300}-\x{337f}\x{3400}-\x{4dbf}\x{4e00}-\x{9fff}\x{f900}-\x{faff}\x{ff65}-\x{ff9f}`
	letter := `[\p{L}&&[^` + cj + `]]`
	digit := `\p{Nd}`
	alnum := `(?:` + letter + `|[\x{0E00}-\x{0E59}]|` + digit + `)+`
	alpha := `(?:` + letter + `)+`
	p := `[_\-/.,]`
	hasDigit := `(?:` + letter + `|` + digit + `)*` + digit + `(?:` + letter + `|` + digit + `)*`
	mk := func(s string) *jRegex {
		re, err := compileJavaRegex(s, 0)
		if err != nil {
			panic(err.message())
		}
		return re
	}
	classicRules.alphanum = mk(alnum)
	classicRules.apostrophe = mk(alpha + `(?:'` + alpha + `)+`)
	classicRules.acronym = mk(letter + `\.(?:` + letter + `\.)+`)
	classicRules.company = mk(alpha + `[&@]` + alpha)
	classicRules.email = mk(alnum + `(?:[._\-]` + alnum + `)*@` + alnum + `(?:[.\-]` + alnum + `)+`)
	classicRules.host = mk(alnum + `(?:\.` + alnum + `)+`)
	classicRules.num = mk(`(?:` + alnum + p + hasDigit +
		`|` + hasDigit + p + alnum +
		`|` + alnum + `(?:` + p + hasDigit + p + alnum + `)+` +
		`|` + hasDigit + `(?:` + p + alnum + p + hasDigit + `)+` +
		`|` + alnum + p + hasDigit + `(?:` + p + alnum + p + hasDigit + `)+` +
		`|` + hasDigit + p + alnum + `(?:` + p + hasDigit + p + alnum + `)+)`)
	classicRules.acronymDep = mk(alnum + `\.(?:` + alnum + `\.)+`)
}

func isClassicCJ(r rune) bool {
	return (r >= 0x3100 && r <= 0x312f) || (r >= 0x3040 && r <= 0x309F) || (r >= 0x30A0 && r <= 0x30FF) ||
		(r >= 0x31F0 && r <= 0x31FF) || (r >= 0x3300 && r <= 0x337f) || (r >= 0x3400 && r <= 0x4dbf) ||
		(r >= 0x4e00 && r <= 0x9fff) || (r >= 0xf900 && r <= 0xfaff) || (r >= 0xff65 && r <= 0xff9f)
}

// classicTokens implements the JFlex grammar of Lucene's ClassicTokenizer:
// longest match wins, ties go to the earlier rule.
func classicTokens(text string, maxLen int) []analyzedToken {
	rs := []rune(text)
	offs := runeOffsets(rs)
	out := []analyzedToken{}
	pos := 0
	rules := []struct {
		re  *jRegex
		typ string
	}{
		{classicRules.alphanum, "<ALPHANUM>"},
		{classicRules.apostrophe, "<APOSTROPHE>"},
		{classicRules.acronym, "<ACRONYM>"},
		{classicRules.company, "<COMPANY>"},
		{classicRules.email, "<EMAIL>"},
		{classicRules.host, "<HOST>"},
		{classicRules.num, "<NUM>"},
		{nil, "<CJ>"},
		{classicRules.acronymDep, "<ACRONYM_DEP>"},
	}
	i := 0
	for i < len(rs) {
		// Limit the candidate to the run of characters any rule can use.
		j := i
		for j < len(rs) && !unicode.IsSpace(rs[j]) {
			j++
		}
		cand := rs[:j]
		best, bestTyp := -1, ""
		for _, rule := range rules {
			var e int
			if rule.re == nil {
				e = -1
				if isClassicCJ(rs[i]) {
					e = i + 1
				}
			} else {
				e = rule.re.longestAt(cand, i)
			}
			if e > best {
				best, bestTyp = e, rule.typ
			}
		}
		if best <= i {
			i++
			continue
		}
		term := string(rs[i:best])
		typ := bestTyp
		if typ == "<ACRONYM_DEP>" {
			typ = "<HOST>"
			term = string(rs[i : best-1])
		}
		if offs[best]-offs[i] <= maxLen {
			out = append(out, analyzedToken{term: term, start: offs[i], end: offs[best], pos: pos, typ: typ})
		}
		pos++
		i = best
	}
	return out
}
