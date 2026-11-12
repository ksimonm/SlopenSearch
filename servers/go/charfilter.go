package main

import (
	"strconv"
	"strings"
)

// Character filter execution with Lucene-style offset correction. Tokens are
// produced over the filtered text, then their UTF-16 offsets are mapped back to
// the original input using boundary information recorded per output rune.

type cfRune struct {
	r          rune
	start, end int // source UTF-16 offsets
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
		closing := strings.HasPrefix(lower, "/")
		name := strings.TrimSpace(strings.TrimPrefix(lower, "/"))
		if at := strings.IndexAny(name, " \t\r\n/"); at >= 0 {
			name = name[:at]
		}
		if escaped[name] {
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
		if closing {
			name = "/" + name
		}
		if htmlBlockTags[name] {
			out = append(out, cfRune{r: '\n', start: rs[i].start, end: rs[i].end})
		}
		i = j + 1
	}
	return out
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

// mappingUnescape decodes the escapes MappingCharFilterFactory accepts.
func mappingUnescape(s string) string {
	rs := []rune(s)
	var out []rune
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		if c == '\\' && i+1 < len(rs) {
			i++
			switch rs[i] {
			case 'n':
				c = '\n'
			case 't':
				c = '\t'
			case 'r':
				c = '\r'
			case 'b':
				c = '\b'
			case 'f':
				c = '\f'
			case 'u':
				if i+4 < len(rs) {
					if v, err := strconv.ParseUint(string(rs[i+1:i+5]), 16, 32); err == nil {
						c = rune(v)
						i += 4
						break
					}
				}
				c = 'u'
			default:
				c = rs[i]
			}
		}
		out = append(out, c)
	}
	return string(out)
}

type mappingRule struct {
	from, to []rune
}

func parseMappingRules(lines []string) ([]mappingRule, *apiError) {
	var rules []mappingRule
	for _, line := range lines {
		at := strings.LastIndex(line, "=>")
		if at < 0 {
			return nil, filterError("Invalid Mapping Rule : [" + line + "]")
		}
		from := mappingUnescape(strings.TrimSpace(line[:at]))
		to := mappingUnescape(strings.TrimSpace(line[at+2:]))
		if from == "" {
			return nil, filterError("Invalid Mapping Rule : [" + line + "], LHS or RHS is empty")
		}
		rules = append(rules, mappingRule{from: []rune(from), to: []rune(to)})
	}
	return rules, nil
}

// mappingApply replaces, at each position, the longest matching key; the
// replacement is not scanned again (Lucene's MappingCharFilter).
func mappingApply(rs []cfRune, rules []mappingRule) []cfRune {
	out := []cfRune{}
	for i := 0; i < len(rs); {
		best := -1
		for ri, rule := range rules {
			if i+len(rule.from) > len(rs) || (best >= 0 && len(rule.from) <= len(rules[best].from)) {
				continue
			}
			match := true
			for k, r := range rule.from {
				if rs[i+k].r != r {
					match = false
					break
				}
			}
			if match {
				best = ri
			}
		}
		if best < 0 {
			out = append(out, rs[i])
			i++
			continue
		}
		rule := rules[best]
		s0 := rs[i].start
		e0 := rs[i+len(rule.from)-1].end
		for _, r := range rule.to {
			out = append(out, cfRune{r: r, start: s0, end: e0})
		}
		i += len(rule.from)
	}
	return out
}

// boundaryCorrection maps filtered offsets back through a rune level filter.
func boundaryCorrection(rs []cfRune, filtered string, inputLen int) func(int) int {
	boundary := runeBoundary(rs, filtered, inputLen)
	return func(o int) int {
		if o >= 0 && o < len(boundary) {
			return boundary[o]
		}
		return inputLen
	}
}

// luceneCorrection implements BaseCharFilter offset correction from
// (offset, cumulative diff) pairs.
type luceneCorrection struct {
	offsets, diffs []int
}

func (c *luceneCorrection) add(off, diff int) {
	if n := len(c.offsets); n > 0 && c.offsets[n-1] == off {
		c.diffs[n-1] = diff
		return
	}
	c.offsets = append(c.offsets, off)
	c.diffs = append(c.diffs, diff)
}

func (c *luceneCorrection) correct(off int) int {
	k := -1
	for i, o := range c.offsets {
		if o <= off {
			k = i
		} else {
			break
		}
	}
	if k < 0 {
		return off
	}
	return off + c.diffs[k]
}

func utf16Len(rs []rune) int {
	n := 0
	for _, r := range rs {
		n += runeWidth(r)
	}
	return n
}

// patternReplaceChars ports Lucene's PatternReplaceCharFilter.
func patternReplaceChars(re *jRegex, repl, text string) (string, func(int) int, *apiError) {
	in := []rune(text)
	var out []rune
	corr := &luceneCorrection{}
	cumulative := 0
	last := 0
	for _, m := range re.findAll(in) {
		out = append(out, in[last:m[0]]...)
		before := utf16Len(out)
		r, err := re.expandReplacement(in, m, repl)
		if err != nil {
			return "", nil, err
		}
		out = append(out, r...)
		groupSize := utf16Len(in[m[0]:m[1]])
		replSize := utf16Len(r)
		if groupSize != replSize {
			if replSize < groupSize {
				cumulative += groupSize - replSize
				corr.add(before+replSize, cumulative)
			} else {
				for i := groupSize; i < replSize; i++ {
					cumulative--
					corr.add(before+i, cumulative)
				}
			}
		}
		last = m[1]
	}
	out = append(out, in[last:]...)
	return string(out), corr.correct, nil
}

// applyCharFilter runs one char filter, returning the filtered text and the
// function mapping its offsets back to the input.
func applyCharFilter(cf chainCharFilter, text string) (string, func(int) int, *apiError) {
	identity := func(o int) int { return o }
	switch cf.typ {
	case "html_strip":
		escaped := map[string]bool{}
		for _, e := range fsStrings(cf.params, "escaped_tags") {
			escaped[strings.ToLower(e)] = true
		}
		rs := htmlStrip(runesToCF(text), escaped)
		out := cfToString(rs)
		return out, boundaryCorrection(rs, out, utf16Offset(text)), nil
	case "mapping":
		if fsString(cf.params, "mappings_path", "") != "" {
			return "", nil, filterError("IOException while reading mappings_path: file not readable")
		}
		if _, ok := cf.params["mappings"]; !ok {
			return "", nil, filterError("mapping requires either `mappings` or `mappings_path` to be configured")
		}
		rules, err := parseMappingRules(fsStrings(cf.params, "mappings"))
		if err != nil {
			return "", nil, err
		}
		rs := mappingApply(runesToCF(text), rules)
		out := cfToString(rs)
		return out, boundaryCorrection(rs, out, utf16Offset(text)), nil
	case "pattern_replace":
		pat := fsString(cf.params, "pattern", "")
		if pat == "" {
			return "", nil, filterError("pattern is missing for [" + cf.name + "] char filter of type 'pattern_replace'")
		}
		flags, ferr := javaRegexFlags(fsString(cf.params, "flags", ""))
		if ferr != nil {
			return "", nil, ferr
		}
		re, err := compileJavaRegexAPI(pat, flags)
		if err != nil {
			return "", nil, err
		}
		return patternReplaceChars(re, fsString(cf.params, "replacement", ""), text)
	}
	return text, identity, nil
}
