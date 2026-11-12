package main

import (
	"strconv"
	"strings"
	"unicode"
)

// A small backtracking implementation of java.util.regex, used where the
// analysis components of OpenSearch take Java patterns (pattern tokenizer,
// pattern_replace char and token filters, pattern_capture, ...). Go's regexp
// package lacks lookaround, backreferences and possessive quantifiers, and
// treats some classes differently. Matching works on code points; callers map
// rune indices back to UTF-16 offsets.

const (
	jFlagCaseInsensitive = 1 << iota
	jFlagMultiline
	jFlagDotall
	jFlagUnicodeCase
	jFlagComments
	jFlagUnixLines
	jFlagUnicodeClass
	jFlagLiteral
	jFlagCanonEq
)

// javaRegexFlags parses the pipe separated flag names accepted by
// OpenSearch (Regex.flagsFromString).
func javaRegexFlags(s string) (int, *apiError) {
	f := 0
	for _, name := range strings.Split(s, "|") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		switch strings.ToUpper(name) {
		case "CASE_INSENSITIVE":
			f |= jFlagCaseInsensitive
		case "MULTILINE":
			f |= jFlagMultiline
		case "DOTALL":
			f |= jFlagDotall
		case "UNICODE_CASE":
			f |= jFlagUnicodeCase
		case "COMMENTS":
			f |= jFlagComments
		case "UNIX_LINES":
			f |= jFlagUnixLines
		case "UNICODE_CHARACTER_CLASS":
			f |= jFlagUnicodeClass
		case "LITERAL":
			f |= jFlagLiteral
		case "CANON_EQ":
			f |= jFlagCanonEq
		default:
			return 0, analyzeRequestError("illegal_argument_exception", "Unknown regex flag ["+name+"]")
		}
	}
	return f, nil
}

type jNodeKind int

const (
	jnLit jNodeKind = iota
	jnSet
	jnSeq
	jnAlt
	jnGroup
	jnRepeat
	jnAssert
	jnLook
	jnAtomic
	jnBackref
	jnEmpty
)

type jNode struct {
	kind     jNodeKind
	r        rune
	fold     int // case folding flags for literals/backrefs
	set      func(rune) bool
	subs     []*jNode
	group    int
	min, max int // max -1: unbounded
	mode     byte
	assert   byte
	multi    bool
	unixLn   bool
	ahead    bool
	neg      bool
	asciiWrd bool
}

type jRegex struct {
	root   *jNode
	groups int
	names  map[string]int
	src    string
}

type jPatternError struct {
	desc  string
	index int
	pat   string
}

func (e *jPatternError) message() string {
	var b strings.Builder
	b.WriteString(e.desc)
	if e.index >= 0 {
		b.WriteString(" near index ")
		b.WriteString(strconv.Itoa(e.index))
	}
	b.WriteString("\n")
	b.WriteString(e.pat)
	if e.index >= 0 && e.index < len([]rune(e.pat)) {
		b.WriteString("\n")
		b.WriteString(strings.Repeat(" ", e.index))
		b.WriteString("^")
	}
	return b.String()
}

func (e *jPatternError) apiError() *apiError {
	return analyzeRequestError("pattern_syntax_exception", e.message())
}

type jParser struct {
	src    []rune
	pos    int
	flags  int
	groups int
	names  map[string]int
	pat    string
}

func compileJavaRegex(pattern string, flags int) (*jRegex, *jPatternError) {
	p := &jParser{src: []rune(pattern), flags: flags, names: map[string]int{}, pat: pattern}
	if flags&jFlagLiteral != 0 {
		seq := &jNode{kind: jnSeq}
		for _, r := range p.src {
			seq.subs = append(seq.subs, p.lit(r))
		}
		return &jRegex{root: seq, src: pattern, names: p.names}, nil
	}
	n, err := p.parseAlt()
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.src) {
		if p.src[p.pos] == ')' {
			return nil, &jPatternError{"Unmatched closing ')'", p.pos - 1, pattern}
		}
		return nil, &jPatternError{"Unexpected internal error", p.pos, pattern}
	}
	return &jRegex{root: n, groups: p.groups, names: p.names, src: pattern}, nil
}

func (p *jParser) errAt(desc string, idx int) *jPatternError {
	return &jPatternError{desc, idx, p.pat}
}

func (p *jParser) more() bool { return p.pos < len(p.src) }

func (p *jParser) peek() rune {
	if p.pos < len(p.src) {
		return p.src[p.pos]
	}
	return -1
}

func (p *jParser) skipComments() {
	if p.flags&jFlagComments == 0 {
		return
	}
	for p.more() {
		r := p.src[p.pos]
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v' {
			p.pos++
			continue
		}
		if r == '#' {
			for p.more() && p.src[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		break
	}
}

func (p *jParser) parseAlt() (*jNode, *jPatternError) {
	var alts []*jNode
	for {
		seq, err := p.parseSeq()
		if err != nil {
			return nil, err
		}
		alts = append(alts, seq)
		if p.peek() == '|' {
			p.pos++
			continue
		}
		break
	}
	if len(alts) == 1 {
		return alts[0], nil
	}
	return &jNode{kind: jnAlt, subs: alts}, nil
}

func (p *jParser) parseSeq() (*jNode, *jPatternError) {
	seq := &jNode{kind: jnSeq}
	for {
		p.skipComments()
		if !p.more() {
			break
		}
		r := p.peek()
		if r == '|' || r == ')' {
			break
		}
		atom, err := p.parseAtom()
		if err != nil {
			return nil, err
		}
		if atom == nil {
			continue
		}
		atom, err = p.parseQuant(atom)
		if err != nil {
			return nil, err
		}
		seq.subs = append(seq.subs, atom)
	}
	return seq, nil
}

func (p *jParser) parseQuant(atom *jNode) (*jNode, *jPatternError) {
	for {
		p.skipComments()
		if !p.more() {
			return atom, nil
		}
		r := p.peek()
		min, max := -1, -1
		switch r {
		case '*':
			min, max = 0, -1
			p.pos++
		case '+':
			min, max = 1, -1
			p.pos++
		case '?':
			min, max = 0, 1
			p.pos++
		case '{':
			start := p.pos
			p.pos++
			n1, ok := p.readInt()
			if !ok {
				return nil, p.errAt("Illegal repetition", start)
			}
			min, max = n1, n1
			if p.peek() == ',' {
				p.pos++
				if p.peek() == '}' {
					max = -1
				} else {
					n2, ok := p.readInt()
					if !ok {
						return nil, p.errAt("Unclosed counted closure", p.pos)
					}
					max = n2
				}
			}
			if p.peek() != '}' {
				return nil, p.errAt("Unclosed counted closure", p.pos)
			}
			p.pos++
			if max >= 0 && max < min {
				return nil, p.errAt("Illegal repetition range", start)
			}
		default:
			return atom, nil
		}
		mode := byte('g')
		if p.peek() == '?' {
			mode = 'l'
			p.pos++
		} else if p.peek() == '+' {
			mode = 'p'
			p.pos++
		}
		atom = &jNode{kind: jnRepeat, subs: []*jNode{atom}, min: min, max: max, mode: mode}
	}
}

func (p *jParser) readInt() (int, bool) {
	start := p.pos
	for p.more() && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
		p.pos++
	}
	if p.pos == start {
		return 0, false
	}
	n, err := strconv.Atoi(string(p.src[start:p.pos]))
	return n, err == nil
}

func (p *jParser) lit(r rune) *jNode {
	return &jNode{kind: jnLit, r: r, fold: p.flags & (jFlagCaseInsensitive | jFlagUnicodeCase)}
}

func (p *jParser) parseAtom() (*jNode, *jPatternError) {
	r := p.src[p.pos]
	switch r {
	case '(':
		return p.parseGroup()
	case '[':
		p.pos++
		set, err := p.parseClass()
		if err != nil {
			return nil, err
		}
		return &jNode{kind: jnSet, set: set}, nil
	case '.':
		p.pos++
		dotall := p.flags&jFlagDotall != 0
		unix := p.flags&jFlagUnixLines != 0
		return &jNode{kind: jnSet, set: func(c rune) bool {
			if dotall {
				return true
			}
			if unix {
				return c != '\n'
			}
			return !isJavaLineTerm(c)
		}}, nil
	case '^':
		p.pos++
		return &jNode{kind: jnAssert, assert: '^', multi: p.flags&jFlagMultiline != 0, unixLn: p.flags&jFlagUnixLines != 0}, nil
	case '$':
		p.pos++
		return &jNode{kind: jnAssert, assert: '$', multi: p.flags&jFlagMultiline != 0, unixLn: p.flags&jFlagUnixLines != 0}, nil
	case '\\':
		return p.parseEscape()
	case '*', '+', '?':
		return nil, p.errAt("Dangling meta character '"+string(r)+"'", p.pos)
	case '{':
		return nil, p.errAt("Illegal repetition", p.pos)
	}
	p.pos++
	return p.lit(r), nil
}

func (p *jParser) parseGroup() (*jNode, *jPatternError) {
	p.pos++ // (
	if p.peek() == '?' {
		p.pos++
		c := p.peek()
		switch c {
		case ':':
			p.pos++
			return p.groupBody(&jNode{kind: jnGroup, group: -1})
		case '=', '!':
			p.pos++
			return p.groupBody(&jNode{kind: jnLook, ahead: true, neg: c == '!'})
		case '>':
			p.pos++
			return p.groupBody(&jNode{kind: jnAtomic})
		case '<':
			p.pos++
			c2 := p.peek()
			if c2 == '=' || c2 == '!' {
				p.pos++
				return p.groupBody(&jNode{kind: jnLook, ahead: false, neg: c2 == '!'})
			}
			start := p.pos
			for p.more() && p.src[p.pos] != '>' {
				p.pos++
			}
			if !p.more() {
				return nil, p.errAt("named capturing group is missing trailing '>'", p.pos)
			}
			name := string(p.src[start:p.pos])
			p.pos++
			p.groups++
			p.names[name] = p.groups
			return p.groupBody(&jNode{kind: jnGroup, group: p.groups})
		default:
			// inline flags (?idmsux-idmsux) or (?flags:X)
			on := true
			saved := p.flags
			newFlags := p.flags
			for p.more() {
				ch := p.src[p.pos]
				var bit int
				switch ch {
				case 'i':
					bit = jFlagCaseInsensitive
				case 'm':
					bit = jFlagMultiline
				case 's':
					bit = jFlagDotall
				case 'u':
					bit = jFlagUnicodeCase
				case 'x':
					bit = jFlagComments
				case 'd':
					bit = jFlagUnixLines
				case 'U':
					bit = jFlagUnicodeClass
				case '-':
					on = false
					p.pos++
					continue
				case ')':
					p.pos++
					p.flags = newFlags
					return nil, nil
				case ':':
					p.pos++
					p.flags = newFlags
					n, err := p.groupBody(&jNode{kind: jnGroup, group: -1})
					p.flags = saved
					return n, err
				default:
					return nil, p.errAt("Unknown inline modifier", p.pos)
				}
				if on {
					newFlags |= bit
				} else {
					newFlags &^= bit
				}
				p.pos++
			}
			return nil, p.errAt("Unclosed group", len(p.src))
		}
	}
	p.groups++
	return p.groupBody(&jNode{kind: jnGroup, group: p.groups})
}

func (p *jParser) groupBody(n *jNode) (*jNode, *jPatternError) {
	saved := p.flags
	body, err := p.parseAlt()
	if err != nil {
		return nil, err
	}
	p.flags = saved
	if p.peek() != ')' {
		return nil, p.errAt("Unclosed group", len(p.src))
	}
	p.pos++
	n.subs = []*jNode{body}
	return n, nil
}

func isJavaLineTerm(c rune) bool {
	return c == '\n' || c == '\r' || c == 0x85 || c == 0x2028 || c == 0x2029
}

func asciiWordRune(c rune) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func unicodeWordRune(c rune) bool {
	return c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c) || unicode.Is(unicode.Mn, c) ||
		unicode.Is(unicode.Pc, c) || unicode.Is(unicode.Me, c) || unicode.Is(unicode.Mc, c) || c == 0x200C || c == 0x200D
}

func javaSpace(c rune) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == 0x0B || c == '\f' || c == '\r'
}

// parseEscape handles a backslash escape outside a character class.
func (p *jParser) parseEscape() (*jNode, *jPatternError) {
	start := p.pos
	p.pos++
	if !p.more() {
		return nil, p.errAt("Unexpected internal error", len(p.src))
	}
	c := p.src[p.pos]
	switch c {
	case 'b', 'B', 'A', 'z', 'Z', 'G':
		p.pos++
		return &jNode{kind: jnAssert, assert: byte(c), asciiWrd: p.flags&jFlagUnicodeClass == 0, unixLn: p.flags&jFlagUnixLines != 0}, nil
	case 'Q':
		p.pos++
		seq := &jNode{kind: jnSeq}
		for p.more() {
			if p.src[p.pos] == '\\' && p.pos+1 < len(p.src) && p.src[p.pos+1] == 'E' {
				p.pos += 2
				return seq, nil
			}
			seq.subs = append(seq.subs, p.lit(p.src[p.pos]))
			p.pos++
		}
		return seq, nil
	case 'E':
		p.pos++
		return nil, nil
	case 'k':
		p.pos++
		if p.peek() != '<' {
			return nil, p.errAt("\\k is not followed by '<' for named capturing group", p.pos)
		}
		p.pos++
		s := p.pos
		for p.more() && p.src[p.pos] != '>' {
			p.pos++
		}
		name := string(p.src[s:p.pos])
		p.pos++
		g, ok := p.names[name]
		if !ok {
			return nil, p.errAt("named capturing group <"+name+"> does not exist", p.pos-1)
		}
		return &jNode{kind: jnBackref, group: g, fold: p.flags & (jFlagCaseInsensitive | jFlagUnicodeCase)}, nil
	}
	if c >= '1' && c <= '9' {
		p.pos++
		g := int(c - '0')
		// Java takes further digits while the group number stays valid.
		for p.more() && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
			ng := g*10 + int(p.src[p.pos]-'0')
			if ng > p.groups {
				break
			}
			g = ng
			p.pos++
		}
		return &jNode{kind: jnBackref, group: g, fold: p.flags & (jFlagCaseInsensitive | jFlagUnicodeCase)}, nil
	}
	p.pos = start
	set, r, err := p.classEscape()
	if err != nil {
		return nil, err
	}
	if set != nil {
		return &jNode{kind: jnSet, set: set}, nil
	}
	return p.lit(r), nil
}

// classEscape parses an escape valid both inside and outside a class. It
// returns either a set or a single literal rune.
func (p *jParser) classEscape() (func(rune) bool, rune, *jPatternError) {
	p.pos++ // backslash
	if !p.more() {
		return nil, 0, p.errAt("Unexpected internal error", len(p.src))
	}
	c := p.src[p.pos]
	p.pos++
	uni := p.flags&jFlagUnicodeClass != 0
	switch c {
	case 'd':
		if uni {
			return unicode.IsDigit, 0, nil
		}
		return func(r rune) bool { return r >= '0' && r <= '9' }, 0, nil
	case 'D':
		if uni {
			return func(r rune) bool { return !unicode.IsDigit(r) }, 0, nil
		}
		return func(r rune) bool { return !(r >= '0' && r <= '9') }, 0, nil
	case 'w':
		if uni {
			return unicodeWordRune, 0, nil
		}
		return asciiWordRune, 0, nil
	case 'W':
		if uni {
			return func(r rune) bool { return !unicodeWordRune(r) }, 0, nil
		}
		return func(r rune) bool { return !asciiWordRune(r) }, 0, nil
	case 's':
		if uni {
			return unicode.IsSpace, 0, nil
		}
		return javaSpace, 0, nil
	case 'S':
		if uni {
			return func(r rune) bool { return !unicode.IsSpace(r) }, 0, nil
		}
		return func(r rune) bool { return !javaSpace(r) }, 0, nil
	case 'h':
		return javaHorizSpace, 0, nil
	case 'H':
		return func(r rune) bool { return !javaHorizSpace(r) }, 0, nil
	case 'v':
		return javaVertSpace, 0, nil
	case 'V':
		return func(r rune) bool { return !javaVertSpace(r) }, 0, nil
	case 'R':
		return javaVertSpace, 0, nil
	case 'p', 'P':
		name := ""
		if p.peek() == '{' {
			s := p.pos + 1
			e := s
			for e < len(p.src) && p.src[e] != '}' {
				e++
			}
			if e >= len(p.src) {
				return nil, 0, p.errAt("Unclosed character family", len(p.src))
			}
			name = string(p.src[s:e])
			p.pos = e + 1
		} else if p.more() {
			name = string(p.src[p.pos])
			p.pos++
		}
		set := javaUnicodeProperty(name, uni)
		if set == nil {
			return nil, 0, p.errAt("Unknown character property name {"+name+"}", p.pos-1)
		}
		if c == 'P' {
			inner := set
			set = func(r rune) bool { return !inner(r) }
		}
		return set, 0, nil
	case 't':
		return nil, '\t', nil
	case 'n':
		return nil, '\n', nil
	case 'r':
		return nil, '\r', nil
	case 'f':
		return nil, '\f', nil
	case 'a':
		return nil, 7, nil
	case 'e':
		return nil, 27, nil
	case '0':
		v := 0
		n := 0
		for n < 3 && p.more() && p.src[p.pos] >= '0' && p.src[p.pos] <= '7' {
			nv := v*8 + int(p.src[p.pos]-'0')
			if nv > 0377 {
				break
			}
			v = nv
			p.pos++
			n++
		}
		if n == 0 {
			return nil, 0, p.errAt("Illegal octal escape sequence", p.pos)
		}
		return nil, rune(v), nil
	case 'x':
		if p.peek() == '{' {
			s := p.pos + 1
			e := s
			for e < len(p.src) && p.src[e] != '}' {
				e++
			}
			v, err := strconv.ParseUint(string(p.src[s:e]), 16, 32)
			if err != nil || e >= len(p.src) {
				return nil, 0, p.errAt("Illegal hexadecimal escape sequence", p.pos)
			}
			p.pos = e + 1
			return nil, rune(v), nil
		}
		if p.pos+2 > len(p.src) {
			return nil, 0, p.errAt("Illegal hexadecimal escape sequence", p.pos)
		}
		v, err := strconv.ParseUint(string(p.src[p.pos:p.pos+2]), 16, 32)
		if err != nil {
			return nil, 0, p.errAt("Illegal hexadecimal escape sequence", p.pos)
		}
		p.pos += 2
		return nil, rune(v), nil
	case 'u':
		if p.pos+4 > len(p.src) {
			return nil, 0, p.errAt("Illegal Unicode escape sequence", p.pos)
		}
		v, err := strconv.ParseUint(string(p.src[p.pos:p.pos+4]), 16, 32)
		if err != nil {
			return nil, 0, p.errAt("Illegal Unicode escape sequence", p.pos)
		}
		p.pos += 4
		r := rune(v)
		// Surrogate pair written as two \u escapes.
		if r >= 0xD800 && r <= 0xDBFF && p.pos+6 <= len(p.src) && p.src[p.pos] == '\\' && p.src[p.pos+1] == 'u' {
			if v2, err := strconv.ParseUint(string(p.src[p.pos+2:p.pos+6]), 16, 32); err == nil && v2 >= 0xDC00 && v2 <= 0xDFFF {
				p.pos += 6
				r = (r-0xD800)<<10 + (rune(v2) - 0xDC00) + 0x10000
			}
		}
		return nil, r, nil
	case 'c':
		if !p.more() {
			return nil, 0, p.errAt("Illegal control escape sequence", p.pos)
		}
		r := p.src[p.pos] ^ 64
		p.pos++
		return nil, r, nil
	}
	if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
		return nil, 0, p.errAt("Illegal/unsupported escape sequence", p.pos-1)
	}
	return nil, c, nil
}

func javaHorizSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == 0xA0 || r == 0x1680 || r == 0x180e || (r >= 0x2000 && r <= 0x200a) ||
		r == 0x202f || r == 0x205f || r == 0x3000
}

func javaVertSpace(r rune) bool {
	return r == '\n' || r == 0x0B || r == '\f' || r == '\r' || r == 0x85 || r == 0x2028 || r == 0x2029
}

func javaUnicodeProperty(name string, uni bool) func(rune) bool {
	posix := map[string]func(rune) bool{
		"Lower":  func(r rune) bool { return r >= 'a' && r <= 'z' },
		"Upper":  func(r rune) bool { return r >= 'A' && r <= 'Z' },
		"ASCII":  func(r rune) bool { return r < 0x80 },
		"Alpha":  func(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') },
		"Digit":  func(r rune) bool { return r >= '0' && r <= '9' },
		"Alnum":  func(r rune) bool { return asciiWordRune(r) && r != '_' },
		"Punct":  func(r rune) bool { return r < 0x80 && strings.ContainsRune("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", r) },
		"Graph":  func(r rune) bool { return r > 0x20 && r < 0x7f },
		"Print":  func(r rune) bool { return r >= 0x20 && r < 0x7f },
		"Blank":  func(r rune) bool { return r == ' ' || r == '\t' },
		"Cntrl":  func(r rune) bool { return r < 0x20 || r == 0x7f },
		"XDigit": func(r rune) bool { return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') },
		"Space":  javaSpace,
	}
	if uni {
		switch name {
		case "Lower":
			return unicode.IsLower
		case "Upper":
			return unicode.IsUpper
		case "Alpha":
			return unicode.IsLetter
		case "Digit":
			return unicode.IsDigit
		case "Alnum":
			return func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
		case "Punct":
			return unicode.IsPunct
		case "Space":
			return unicode.IsSpace
		}
	}
	if f, ok := posix[name]; ok {
		return f
	}
	switch name {
	case "javaLowerCase":
		return unicode.IsLower
	case "javaUpperCase":
		return unicode.IsUpper
	case "javaWhitespace":
		return unicode.IsSpace
	case "javaLetter", "IsLetter", "IsL":
		return unicode.IsLetter
	case "javaDigit", "IsDigit":
		return unicode.IsDigit
	case "javaLetterOrDigit":
		return func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
	case "IsAlphabetic", "Alphabetic":
		return func(r rune) bool {
			return unicode.IsLetter(r) || unicode.Is(unicode.Nl, r) || unicode.Is(unicode.Other_Alphabetic, r)
		}
	case "IsLowercase", "Lowercase":
		return func(r rune) bool { return unicode.IsLower(r) || unicode.Is(unicode.Other_Lowercase, r) }
	case "IsUppercase", "Uppercase":
		return func(r rune) bool { return unicode.IsUpper(r) || unicode.Is(unicode.Other_Uppercase, r) }
	case "IsWhite_Space", "IsWhiteSpace", "White_Space":
		return func(r rune) bool { return unicode.Is(unicode.White_Space, r) }
	case "IsPunctuation":
		return unicode.IsPunct
	case "IsIdeographic", "Ideographic":
		return func(r rune) bool { return unicode.Is(unicode.Ideographic, r) }
	case "L&", "LC":
		return func(r rune) bool { return unicode.IsUpper(r) || unicode.IsLower(r) || unicode.IsTitle(r) }
	}
	n := name
	for _, pre := range []string{"general_category=", "gc="} {
		n = strings.TrimPrefix(n, pre)
	}
	if strings.HasPrefix(n, "Is") && len(n) > 2 {
		if t, ok := unicode.Categories[n[2:]]; ok {
			return func(r rune) bool { return unicode.Is(t, r) }
		}
		if t, ok := unicode.Scripts[n[2:]]; ok {
			return func(r rune) bool { return unicode.Is(t, r) }
		}
	}
	if t, ok := unicode.Categories[n]; ok {
		return func(r rune) bool { return unicode.Is(t, r) }
	}
	for _, pre := range []string{"script=", "sc="} {
		if strings.HasPrefix(n, pre) {
			if t, ok := unicode.Scripts[n[len(pre):]]; ok {
				return func(r rune) bool { return unicode.Is(t, r) }
			}
		}
	}
	if strings.HasPrefix(n, "In") {
		if t, ok := unicode.Scripts[n[2:]]; ok {
			return func(r rune) bool { return unicode.Is(t, r) }
		}
	}
	return nil
}

// parseClass parses a character class after the opening '['.
func (p *jParser) parseClass() (func(rune) bool, *jPatternError) {
	negate := false
	if p.peek() == '^' {
		negate = true
		p.pos++
	}
	var parts []func(rune) bool
	var inter []func(rune) bool
	first := true
	fold := p.flags & (jFlagCaseInsensitive | jFlagUnicodeCase)
	for {
		if !p.more() {
			return nil, p.errAt("Unclosed character class", len(p.src)-1)
		}
		c := p.src[p.pos]
		if c == ']' && !first {
			p.pos++
			break
		}
		first = false
		if c == '[' {
			p.pos++
			sub, err := p.parseClass()
			if err != nil {
				return nil, err
			}
			parts = append(parts, sub)
			continue
		}
		if c == '&' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '&' {
			p.pos += 2
			inter = append(inter, unionSet(parts))
			parts = nil
			continue
		}
		var lo rune
		if c == '\\' {
			set, r, err := p.classEscape()
			if err != nil {
				return nil, err
			}
			if set != nil {
				parts = append(parts, set)
				continue
			}
			lo = r
		} else {
			lo = c
			p.pos++
		}
		if p.peek() == '-' && p.pos+1 < len(p.src) && p.src[p.pos+1] != ']' && p.src[p.pos+1] != '[' {
			p.pos++
			var hi rune
			if p.src[p.pos] == '\\' {
				set, r, err := p.classEscape()
				if err != nil {
					return nil, err
				}
				if set != nil {
					return nil, p.errAt("Illegal character range", p.pos-1)
				}
				hi = r
			} else {
				hi = p.src[p.pos]
				p.pos++
			}
			if hi < lo {
				return nil, p.errAt("Illegal character range", p.pos-1)
			}
			l, h := lo, hi
			parts = append(parts, func(r rune) bool {
				if r >= l && r <= h {
					return true
				}
				if fold != 0 {
					for _, f := range foldVariants(r, fold) {
						if f >= l && f <= h {
							return true
						}
					}
				}
				return false
			})
			continue
		}
		l := lo
		parts = append(parts, func(r rune) bool { return runeEqFold(r, l, fold) })
	}
	set := unionSet(parts)
	if len(inter) > 0 {
		all := append(inter, set)
		set = func(r rune) bool {
			for _, f := range all {
				if !f(r) {
					return false
				}
			}
			return true
		}
	}
	if negate {
		inner := set
		set = func(r rune) bool { return !inner(r) }
	}
	return set, nil
}

func unionSet(parts []func(rune) bool) func(rune) bool {
	ps := append([]func(rune) bool{}, parts...)
	return func(r rune) bool {
		for _, f := range ps {
			if f(r) {
				return true
			}
		}
		return false
	}
}

func foldVariants(r rune, fold int) []rune {
	if fold&jFlagCaseInsensitive == 0 {
		return nil
	}
	if fold&jFlagUnicodeCase == 0 {
		if r >= 'a' && r <= 'z' {
			return []rune{r - 32}
		}
		if r >= 'A' && r <= 'Z' {
			return []rune{r + 32}
		}
		return nil
	}
	var out []rune
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		out = append(out, f)
	}
	return out
}

func runeEqFold(a, b rune, fold int) bool {
	if a == b {
		return true
	}
	if fold&jFlagCaseInsensitive == 0 {
		return false
	}
	for _, f := range foldVariants(a, fold) {
		if f == b {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- matching

type jMatcher struct {
	re    *jRegex
	in    []rune
	caps  []int
	steps int
}

func (m *jMatcher) match(n *jNode, i int, k func(int) bool) bool {
	m.steps++
	switch n.kind {
	case jnEmpty:
		return k(i)
	case jnLit:
		if i < len(m.in) && runeEqFold(m.in[i], n.r, n.fold) {
			return k(i + 1)
		}
		return false
	case jnSet:
		if i < len(m.in) && n.set(m.in[i]) {
			return k(i + 1)
		}
		return false
	case jnSeq:
		return m.seq(n.subs, 0, i, k)
	case jnAlt:
		for _, s := range n.subs {
			if m.match(s, i, k) {
				return true
			}
		}
		return false
	case jnGroup:
		if n.group < 0 {
			return m.match(n.subs[0], i, k)
		}
		g := n.group
		os, oe := m.caps[2*g], m.caps[2*g+1]
		if m.match(n.subs[0], i, func(e int) bool {
			ps, pe := m.caps[2*g], m.caps[2*g+1]
			m.caps[2*g], m.caps[2*g+1] = i, e
			if k(e) {
				return true
			}
			m.caps[2*g], m.caps[2*g+1] = ps, pe
			return false
		}) {
			return true
		}
		m.caps[2*g], m.caps[2*g+1] = os, oe
		return false
	case jnRepeat:
		if n.mode == 'p' {
			pos := i
			count := 0
			for n.max < 0 || count < n.max {
				end := -1
				if !m.match(n.subs[0], pos, func(e int) bool { end = e; return true }) || end == pos {
					if end == pos && end >= 0 {
						count++
					}
					break
				}
				pos = end
				count++
			}
			if count < n.min {
				return false
			}
			return k(pos)
		}
		return m.rep(n, 0, i, -1, k)
	case jnAssert:
		return m.assertAt(n, i) && k(i)
	case jnLook:
		var ok bool
		saved := append([]int{}, m.caps...)
		if n.ahead {
			ok = m.match(n.subs[0], i, func(int) bool { return true })
		} else {
			ok = false
			for j := i; j >= 0 && !ok; j-- {
				ok = m.match(n.subs[0], j, func(e int) bool { return e == i })
			}
		}
		if n.neg {
			copy(m.caps, saved)
			if ok {
				return false
			}
			return k(i)
		}
		if !ok {
			copy(m.caps, saved)
			return false
		}
		if k(i) {
			return true
		}
		copy(m.caps, saved)
		return false
	case jnAtomic:
		end := -1
		saved := append([]int{}, m.caps...)
		if !m.match(n.subs[0], i, func(e int) bool { end = e; return true }) {
			return false
		}
		if k(end) {
			return true
		}
		copy(m.caps, saved)
		return false
	case jnBackref:
		g := n.group
		if 2*g+1 >= len(m.caps) || m.caps[2*g] < 0 {
			return false
		}
		s, e := m.caps[2*g], m.caps[2*g+1]
		l := e - s
		if i+l > len(m.in) {
			return false
		}
		for x := 0; x < l; x++ {
			if !runeEqFold(m.in[i+x], m.in[s+x], n.fold) {
				return false
			}
		}
		return k(i + l)
	}
	return false
}

func (m *jMatcher) seq(subs []*jNode, idx, i int, k func(int) bool) bool {
	if idx == len(subs) {
		return k(i)
	}
	return m.match(subs[idx], i, func(e int) bool { return m.seq(subs, idx+1, e, k) })
}

func (m *jMatcher) rep(n *jNode, count, i, last int, k func(int) bool) bool {
	canMore := n.max < 0 || count < n.max
	// An iteration that consumed nothing cannot help further.
	if count > n.min && i == last {
		canMore = false
	}
	tryMore := func() bool {
		if !canMore {
			return false
		}
		return m.match(n.subs[0], i, func(e int) bool {
			if e == i && count >= n.min {
				return false
			}
			return m.rep(n, count+1, e, i, k)
		})
	}
	if n.mode == 'l' {
		if count >= n.min && k(i) {
			return true
		}
		return tryMore()
	}
	if tryMore() {
		return true
	}
	return count >= n.min && k(i)
}

func (m *jMatcher) assertAt(n *jNode, i int) bool {
	in := m.in
	isTerm := isJavaLineTerm
	if n.unixLn {
		isTerm = func(r rune) bool { return r == '\n' }
	}
	switch n.assert {
	case '^':
		if i == 0 {
			return true
		}
		if !n.multi {
			return false
		}
		if i >= len(in) {
			return false
		}
		prev := in[i-1]
		if prev == '\r' && in[i] == '\n' {
			return false
		}
		return isTerm(prev)
	case '$':
		if n.multi {
			if i == len(in) {
				return true
			}
			if in[i] == '\n' && i > 0 && in[i-1] == '\r' {
				return false
			}
			return isTerm(in[i])
		}
		return m.atEndOrFinalTerm(i, isTerm)
	case 'A':
		return i == 0
	case 'z':
		return i == len(in)
	case 'Z':
		return m.atEndOrFinalTerm(i, isTerm)
	case 'G':
		return true
	case 'b', 'B':
		word := unicodeWordRune
		if n.asciiWrd {
			word = asciiWordRune
		}
		before := i > 0 && word(in[i-1])
		after := i < len(in) && word(in[i])
		if n.assert == 'b' {
			return before != after
		}
		return before == after
	}
	return false
}

func (m *jMatcher) atEndOrFinalTerm(i int, isTerm func(rune) bool) bool {
	in := m.in
	if i == len(in) {
		return true
	}
	if i == len(in)-1 && isTerm(in[i]) {
		return true
	}
	if i == len(in)-2 && in[i] == '\r' && in[i+1] == '\n' {
		return true
	}
	return false
}

// findAt returns the capture positions (rune indices) of the first match at
// or after start, or nil.
func (re *jRegex) findAt(in []rune, start int) []int {
	m := &jMatcher{re: re, in: in, caps: make([]int, 2*(re.groups+1))}
	for s := start; s <= len(in); s++ {
		for x := range m.caps {
			m.caps[x] = -1
		}
		end := -1
		if m.match(re.root, s, func(e int) bool { end = e; return true }) {
			m.caps[0], m.caps[1] = s, end
			return append([]int{}, m.caps...)
		}
	}
	return nil
}

// findAll returns every non-overlapping match, Java Matcher.find style.
func (re *jRegex) findAll(in []rune) [][]int {
	var out [][]int
	pos := 0
	for pos <= len(in) {
		m := re.findAt(in, pos)
		if m == nil {
			break
		}
		out = append(out, m)
		if m[1] == m[0] {
			pos = m[1] + 1
		} else {
			pos = m[1]
		}
	}
	return out
}

// fullMatch reports whether the whole input matches (Matcher.matches).
func (re *jRegex) fullMatch(in []rune) bool {
	m := &jMatcher{re: re, in: in, caps: make([]int, 2*(re.groups+1))}
	for x := range m.caps {
		m.caps[x] = -1
	}
	return m.match(re.root, 0, func(e int) bool { return e == len(in) })
}

// expandReplacement applies a Java replacement string ($n, ${name}, \x).
func (re *jRegex) expandReplacement(in []rune, caps []int, repl string) ([]rune, *apiError) {
	var out []rune
	rr := []rune(repl)
	for i := 0; i < len(rr); i++ {
		c := rr[i]
		if c == '\\' {
			i++
			if i < len(rr) {
				out = append(out, rr[i])
			} else {
				return nil, analyzeRequestError("illegal_argument_exception", "character to be escaped is missing")
			}
			continue
		}
		if c != '$' {
			out = append(out, c)
			continue
		}
		i++
		if i >= len(rr) {
			return nil, analyzeRequestError("illegal_argument_exception", "Illegal group reference: group index is missing")
		}
		g := -1
		if rr[i] == '{' {
			e := i + 1
			for e < len(rr) && rr[e] != '}' {
				e++
			}
			name := string(rr[i+1 : e])
			n, ok := re.names[name]
			if !ok {
				return nil, analyzeRequestError("illegal_argument_exception", "No group with name {"+name+"}")
			}
			g = n
			i = e
		} else if rr[i] >= '0' && rr[i] <= '9' {
			g = int(rr[i] - '0')
			if g > re.groups {
				return nil, analyzeRequestError("index_out_of_bounds_exception", "No group "+strconv.Itoa(g))
			}
			for i+1 < len(rr) && rr[i+1] >= '0' && rr[i+1] <= '9' {
				ng := g*10 + int(rr[i+1]-'0')
				if ng > re.groups {
					break
				}
				g = ng
				i++
			}
		} else {
			return nil, analyzeRequestError("illegal_argument_exception", "Illegal group reference")
		}
		if caps[2*g] >= 0 {
			out = append(out, in[caps[2*g]:caps[2*g+1]]...)
		}
	}
	return out, nil
}

// replace replaces the first or every match in s.
func (re *jRegex) replace(s, repl string, all bool) (string, *apiError) {
	in := []rune(s)
	var out []rune
	last := 0
	pos := 0
	for pos <= len(in) {
		m := re.findAt(in, pos)
		if m == nil {
			break
		}
		out = append(out, in[last:m[0]]...)
		r, err := re.expandReplacement(in, m, repl)
		if err != nil {
			return "", err
		}
		out = append(out, r...)
		last = m[1]
		if m[1] == m[0] {
			pos = m[1] + 1
		} else {
			pos = m[1]
		}
		if !all {
			break
		}
	}
	if last < len(in) {
		out = append(out, in[last:]...)
	}
	return string(out), nil
}

// compileJavaRegexAPI compiles and converts syntax errors to API errors.
func compileJavaRegexAPI(pattern string, flags int) (*jRegex, *apiError) {
	re, err := compileJavaRegex(pattern, flags)
	if err != nil {
		return nil, err.apiError()
	}
	return re, nil
}
