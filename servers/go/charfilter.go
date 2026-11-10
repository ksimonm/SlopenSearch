package main

import (
	"regexp"
	"strings"
	"unicode"
)

// Character filter execution with Lucene-style offset correction. Tokens are
// produced over the filtered text, then their UTF-16 offsets are mapped back to
// the original input using boundary information recorded per output rune.

type charFilterSpec struct {
	typ    string
	params map[string]any
}

type cfRune struct {
	r          rune
	start, end int // source UTF-16 offsets
}

func globalCharSpecs(comps []analyzeComponent) []charFilterSpec {
	out := make([]charFilterSpec, 0, len(comps))
	for _, c := range comps {
		typ := c.name
		if c.inline != nil {
			if t, ok := c.inline["type"].(string); ok && t != "" {
				typ = t
			}
		}
		out = append(out, charFilterSpec{typ: typ, params: c.inline})
	}
	return out
}

func indexCharSpecs(idx *Index, names []string) []charFilterSpec {
	out := make([]charFilterSpec, 0, len(names))
	for _, name := range names {
		typ := name
		var params map[string]any
		if idx != nil {
			if raw, ok := findSetting(idx.Settings, "analysis.char_filter."+name+".type"); ok {
				typ, _ = raw.(string)
				params = subSettings(idx.Settings, "analysis.char_filter."+name)
			}
		}
		out = append(out, charFilterSpec{typ: typ, params: params})
	}
	return out
}

func runesToCF(text string) []cfRune {
	out := []cfRune{}
	off := 0
	for _, r := range text {
		out = append(out, cfRune{r: r, start: off, end: off + runeWidth(r)})
		off += runeWidth(r)
	}
	return out
}

func cfToString(rs []cfRune) string {
	var b strings.Builder
	for _, r := range rs {
		b.WriteRune(r.r)
	}
	return b.String()
}

func htmlEntity(s string) (string, bool) {
	switch s {
	case "amp":
		return "&", true
	case "lt":
		return "<", true
	case "gt":
		return ">", true
	case "quot":
		return "\"", true
	case "apos":
		return "'", true
	case "nbsp":
		return " ", true
	case "copy":
		return "©", true
	case "reg":
		return "®", true
	case "eacute":
		return "é", true
	case "egrave":
		return "è", true
	case "agrave":
		return "à", true
	case "ccedil":
		return "ç", true
	case "uuml":
		return "ü", true
	case "ouml":
		return "ö", true
	case "auml":
		return "ä", true
	}
	if strings.HasPrefix(s, "#x") || strings.HasPrefix(s, "#X") {
		n, err := parseHex(s[2:])
		if err == nil {
			return string(rune(n)), true
		}
	}
	if strings.HasPrefix(s, "#") {
		n, err := parseDec(s[1:])
		if err == nil {
			return string(rune(n)), true
		}
	}
	return "", false
}

func parseHex(s string) (int, error) {
	n := 0
	for _, r := range s {
		n *= 16
		switch {
		case r >= '0' && r <= '9':
			n += int(r - '0')
		case r >= 'a' && r <= 'f':
			n += int(r-'a') + 10
		case r >= 'A' && r <= 'F':
			n += int(r-'A') + 10
		default:
			return 0, errBad("bad hex")
		}
	}
	return n, nil
}

func parseDec(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errBad("bad dec")
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

var htmlBlockTags = map[string]bool{
	"p": true, "/p": true, "div": true, "/div": true, "br": true, "br/": true,
	"li": true, "/li": true, "ul": true, "/ul": true, "ol": true, "/ol": true,
	"tr": true, "/tr": true, "td": true, "/td": true, "th": true, "/th": true,
	"table": true, "/table": true, "h1": true, "/h1": true, "h2": true, "/h2": true,
	"h3": true, "/h3": true, "h4": true, "/h4": true, "h5": true, "/h5": true,
	"h6": true, "/h6": true, "script": true, "/script": true,
}

func htmlStrip(rs []cfRune, escaped map[string]bool) []cfRune {
	out := []cfRune{}
	i := 0
	for i < len(rs) {
		r := rs[i].r
		if r == '&' {
			// Decode the longest known entity within a short window.
			j := i + 1
			for j < len(rs) && j-i < 12 && rs[j].r != ';' {
				j++
			}
			if j < len(rs) && rs[j].r == ';' {
				name := cfToString(rs[i+1 : j])
				if dec, ok := htmlEntity(name); ok {
					for _, dr := range dec {
						out = append(out, cfRune{r: dr, start: rs[i].start, end: rs[i].end})
					}
					i = j + 1
					continue
				}
			}
			out = append(out, rs[i])
			i++
			continue
		}
		if r != '<' {
			out = append(out, rs[i])
			i++
			continue
		}
		// Comment
		if i+3 < len(rs) && rs[i+1].r == '!' && rs[i+2].r == '-' && rs[i+3].r == '-' {
			j := i + 4
			for j+2 < len(rs) && !(rs[j].r == '-' && rs[j+1].r == '-' && rs[j+2].r == '>') {
				j++
			}
			if j+2 < len(rs) {
				i = j + 3
			} else {
				i = len(rs)
			}
			continue
		}
		// Find the end of the tag.
		j := i + 1
		for j < len(rs) && rs[j].r != '>' {
			j++
		}
		if j >= len(rs) {
			i++
			continue
		}
		inner := strings.TrimSpace(cfToString(rs[i+1 : j]))
		lower := strings.ToLower(inner)
		name := lower
		if at := strings.IndexAny(name, " \t\r\n/"); at >= 0 {
			name = name[:at]
		}
		if escaped[name] || escaped[lower] {
			out = append(out, rs[i:j+1]...)
			i = j + 1
			continue
		}
		if name == "script" {
			out = append(out, cfRune{r: '\n', start: rs[i].start, end: rs[i].end})
			// Skip to </script>.
			k := j + 1
			for k < len(rs) {
				if rs[k].r == '<' {
					m := k + 1
					for m < len(rs) && rs[m].r != '>' {
						m++
					}
					if m < len(rs) {
						closing := strings.ToLower(strings.TrimSpace(cfToString(rs[k+1 : m])))
						if strings.HasPrefix(closing, "/script") {
							k = m + 1
							break
						}
					}
					k = m + 1
					continue
				}
				k++
			}
			i = k
			continue
		}
		if htmlBlockTags[name] {
			out = append(out, cfRune{r: '\n', start: rs[i].start, end: rs[i].end})
		}
		i = j + 1
	}
	return out
}

func mappingEscape(s string) string {
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\r`, "\r")
	s = strings.ReplaceAll(s, `\u0020`, " ")
	return s
}

func mappingApply(rs []cfRune, mappings []string) []cfRune {
	for _, m := range mappings {
		parts := strings.SplitN(m, "=>", 2)
		if len(parts) != 2 {
			continue
		}
		from := mappingEscape(strings.TrimSpace(parts[0]))
		to := mappingEscape(strings.TrimSpace(parts[1]))
		if from == "" {
			continue
		}
		fr := []rune(from)
		tr := []rune(to)
		out := []cfRune{}
		for i := 0; i < len(rs); {
			if i+len(fr) <= len(rs) {
				match := true
				for k, r := range fr {
					if rs[i+k].r != r {
						match = false
						break
					}
				}
				if match {
					s0 := rs[i].start
					e0 := rs[i+len(fr)-1].end
					for _, r := range tr {
						out = append(out, cfRune{r: r, start: s0, end: e0})
					}
					i += len(fr)
					continue
				}
			}
			out = append(out, rs[i])
			i++
		}
		rs = out
	}
	return rs
}

func runeBoundary(rs []cfRune, text string, originalLen int) []int {
	total := utf16Offset(text)
	boundary := make([]int, total+1)
	off := 0
	for _, c := range rs {
		w := runeWidth(c.r)
		for k := 0; k < w; k++ {
			boundary[off+k] = c.start
		}
		off += w
	}
	boundary[total] = originalLen
	return boundary
}

func runCharFilterSpecs(specs []charFilterSpec, text string) (string, []int, []int, bool) {
	if len(specs) == 0 {
		return text, nil, nil, true
	}
	originalLen := utf16Offset(text)
	rs := runesToCF(text)
	var insertions []int
	for _, spec := range specs {
		switch spec.typ {
		case "html_strip":
			escaped := map[string]bool{}
			for _, e := range fsStrings(spec.params, "escaped_tags") {
				escaped[strings.ToLower(e)] = true
			}
			rs = htmlStrip(rs, escaped)
		case "mapping":
			rs = mappingApply(rs, fsStrings(spec.params, "mappings"))
		case "pattern_replace":
			pat := fsString(spec.params, "pattern", "")
			repl := fsString(spec.params, "replacement", "")
			newRS, ins := patternReplaceChar(rs, pat, repl)
			rs = newRS
			insertions = append(insertions, ins...)
		}
	}
	filtered := cfToString(rs)
	if len(insertions) > 0 {
		// Zero-width insertions use Lucene's boundary correction where an
		// insertion at filtered offset p shifts offsets >= p.
		return filtered, nil, insertions, true
	}
	return filtered, runeBoundary(rs, filtered, originalLen), nil, true
}

func remapFilteredTokens(toks []analyzedToken, boundary, insertions []int, originalLen int) {
	if len(insertions) > 0 {
		for i := range toks {
			toks[i].start = correctInsertionOffset(toks[i].start, insertions)
			toks[i].end = correctInsertionOffset(toks[i].end, insertions)
		}
		return
	}
	if len(boundary) == 0 {
		return
	}
	for i := range toks {
		s := toks[i].start
		e := toks[i].end
		if s < len(boundary) {
			toks[i].start = boundary[s]
		}
		if e < len(boundary) {
			toks[i].end = boundary[e]
		} else {
			toks[i].end = originalLen
		}
	}
}

func correctInsertionOffset(o int, insertions []int) int {
	n := 0
	for _, p := range insertions {
		if p <= o {
			n++
		}
	}
	return o - n
}

func patternReplaceChar(rs []cfRune, pattern, repl string) ([]cfRune, []int) {
	if pattern == "" {
		return rs, nil
	}
	// Zero-width lookaround insertion: (?<=A)(?=B).
	if strings.Contains(pattern, "(?<=") && strings.Contains(pattern, "(?=") && !strings.Contains(pattern, "(?:") {
		start := strings.Index(pattern, "(?<=")
		end := strings.Index(pattern, "(?=")
		if end > start {
			behind := pattern[start+4 : strings.Index(pattern[start:], ")")+start]
			lookStart := end + 4
			lookEnd := strings.Index(pattern[lookStart:], ")") + lookStart
			ahead := pattern[lookStart:lookEnd]
			bre, err1 := regexp.Compile(behind)
			are, err2 := regexp.Compile(ahead)
			if err1 == nil && err2 == nil {
				out := []cfRune{}
				insertions := []int{}
				for i := 0; i < len(rs); i++ {
					out = append(out, rs[i])
					if i+1 >= len(rs) {
						continue
					}
					before := cfToString(rs[:i+1])
					after := cfToString(rs[i+1:])
					if bre.MatchString(before) && are.MatchString(after) {
						// Only if the match is anchored at the boundary.
						loc := bre.FindStringIndex(before)
						aloc := are.FindStringIndex(after)
						if loc != nil && loc[1] == len(before) && aloc != nil && aloc[0] == 0 {
							prevEnd := rs[i].end
							for _, r := range repl {
								out = append(out, cfRune{r: r, start: prevEnd, end: prevEnd})
							}
							insertions = append(insertions, rs[i].end)
						}
					}
				}
				return out, insertions
			}
		}
	}
	// One-sided lookahead: base(?=look)
	if at := strings.Index(pattern, "(?="); at >= 0 && !strings.Contains(pattern, "(?<=") {
		base := pattern[:at]
		lookStart := at + 4
		lookEnd := strings.Index(pattern[lookStart:], ")") + lookStart
		look := pattern[lookStart:lookEnd]
		bre, err1 := regexp.Compile(base)
		lre, err2 := regexp.Compile(look)
		if err1 == nil && err2 == nil {
			return replaceWithLookahead(rs, bre, lre, repl), nil
		}
	}
	re, err := regexp.Compile(normalizeRegexpPattern(pattern))
	if err != nil {
		return rs, nil
	}
	return replaceAllWithRunes(rs, re, repl), nil
}

func normalizeRegexpPattern(p string) string {
	// Go's regexp does not understand \p{Lower}; translate to Unicode classes.
	p = strings.ReplaceAll(p, `\p{Lower}`, `\p{Ll}`)
	p = strings.ReplaceAll(p, `\p{Upper}`, `\p{Lu}`)
	return p
}

func replaceAllWithRunes(rs []cfRune, re *regexp.Regexp, repl string) []cfRune {
	text := cfToString(rs)
	// Map byte offsets to rune offsets.
	byteToRune := make([]int, len(text)+1)
	{
		ri := 0
		for bi := range text {
			byteToRune[bi] = ri
			_, w := decodeRuneAt(text, bi)
			if w > 0 {
				ri++
			}
		}
		byteToRune[len(text)] = ri
	}
	matches := re.FindAllStringSubmatchIndex(text, -1)
	out := []cfRune{}
	last := 0
	for _, m := range matches {
		start := byteToRune[m[0]]
		end := byteToRune[m[1]]
		out = append(out, rs[last:start]...)
		replText := re.ExpandString(nil, repl, text, m)
		s0 := rs[start].start
		e0 := rs[end-1].end
		if end == start {
			s0, e0 = rs[start].start, rs[start].start
		}
		for _, r := range string(replText) {
			out = append(out, cfRune{r: r, start: s0, end: e0})
		}
		last = end
	}
	out = append(out, rs[last:]...)
	return out
}

func replaceWithLookahead(rs []cfRune, base, look *regexp.Regexp, repl string) []cfRune {
	text := cfToString(rs)
	out := []cfRune{}
	pos := 0
	for pos < len(text) {
		locs := base.FindStringSubmatchIndex(text[pos:])
		if locs == nil {
			break
		}
		m := locs
		absStart := pos + m[0]
		absEnd := pos + m[1]
		if !look.MatchString(text[absEnd:]) {
			// Copy one byte to avoid an infinite loop.
			out = append(out, rs[pos:pos+1]...)
			pos++
			continue
		}
		startRune := byteToRuneIndex(text, absStart)
		endRune := byteToRuneIndex(text, absEnd)
		out = append(out, rs[posRuneIndex(text, pos):startRune]...)
		replText := base.ExpandString(nil, repl, text, m)
		s0 := rs[startRune].start
		e0 := rs[endRune-1].end
		for _, r := range string(replText) {
			out = append(out, cfRune{r: r, start: s0, end: e0})
		}
		pos = absEnd
	}
	out = append(out, rs[posRuneIndex(text, pos):]...)
	return out
}

func decodeRuneAt(s string, i int) (rune, int) {
	for _, r := range s[i:] {
		return r, len(string(r))
	}
	return 0, 0
}

func byteToRuneIndex(s string, byteOff int) int {
	n := 0
	for i := range s {
		if i >= byteOff {
			return n
		}
		n++
	}
	return n
}

func posRuneIndex(s string, byteOff int) int {
	return byteToRuneIndex(s, byteOff)
}

var _ = unicode.IsLetter
