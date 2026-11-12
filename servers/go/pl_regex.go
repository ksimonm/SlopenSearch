package main

import (
	"strconv"
	"strings"
	"unicode"
)

// Painless regex circuit breaker. In "limited" mode Painless wraps the input
// of every matcher and fails once the (backtracking) java.util.regex engine
// has read more than limit-factor * length characters. Go's regexp never
// backtracks, so a small backtracking matcher replays the match only to
// count character reads; the match result itself still comes from regexp.

type rxNode struct {
	kind     byte // 'c' char set, 'g' group, 'a' alternation, '^', '$', 'b' word boundary
	set      func(r rune) bool
	alts     [][]*rxNode
	min, max int // repetition bounds (max -1 = unbounded)
	lazy     bool
}

type rxParser struct {
	src   []rune
	i     int
	icase bool
	bad   bool
}

func rxCompile(pattern string, icase bool) ([]*rxNode, bool) {
	p := &rxParser{src: []rune(pattern), icase: icase}
	seq := p.alternation()
	if p.bad || p.i < len(p.src) {
		return nil, false
	}
	return seq, true
}

func (p *rxParser) alternation() []*rxNode {
	alts := [][]*rxNode{p.sequence()}
	for p.i < len(p.src) && p.src[p.i] == '|' {
		p.i++
		alts = append(alts, p.sequence())
	}
	if len(alts) == 1 {
		return alts[0]
	}
	return []*rxNode{{kind: 'a', alts: alts, min: 1, max: 1}}
}

func (p *rxParser) sequence() []*rxNode {
	var seq []*rxNode
	for p.i < len(p.src) && p.src[p.i] != '|' && p.src[p.i] != ')' {
		n := p.atom()
		if p.bad {
			return nil
		}
		p.quantifier(n)
		seq = append(seq, n)
	}
	return seq
}

func (p *rxParser) quantifier(n *rxNode) {
	n.min, n.max = 1, 1
	if p.i >= len(p.src) {
		return
	}
	switch p.src[p.i] {
	case '*':
		n.min, n.max = 0, -1
		p.i++
	case '+':
		n.min, n.max = 1, -1
		p.i++
	case '?':
		n.min, n.max = 0, 1
		p.i++
	case '{':
		end := strings.IndexRune(string(p.src[p.i:]), '}')
		if end < 0 {
			return
		}
		body := string(p.src[p.i+1 : p.i+end])
		lo, hi, has := strings.Cut(body, ",")
		a, err := strconv.Atoi(lo)
		if err != nil {
			return
		}
		n.min, n.max = a, a
		if has {
			n.max = -1
			if hi != "" {
				b, err := strconv.Atoi(hi)
				if err != nil {
					return
				}
				n.max = b
			}
		}
		p.i += end + 1
	default:
		return
	}
	if p.i < len(p.src) && p.src[p.i] == '?' {
		n.lazy = true
		p.i++
	} else if p.i < len(p.src) && p.src[p.i] == '+' {
		p.i++ // possessive: treated as greedy
	}
}

func (p *rxParser) fold(f func(rune) bool) func(rune) bool {
	if !p.icase {
		return f
	}
	return func(r rune) bool { return f(unicode.ToLower(r)) || f(unicode.ToUpper(r)) }
}

func rxClassEscape(c rune) (func(rune) bool, bool) {
	switch c {
	case 'd':
		return func(r rune) bool { return r >= '0' && r <= '9' }, true
	case 'D':
		return func(r rune) bool { return !(r >= '0' && r <= '9') }, true
	case 'w':
		return func(r rune) bool {
			return r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		}, true
	case 'W':
		return func(r rune) bool {
			return !(r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z')
		}, true
	case 's':
		return func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v' }, true
	case 'S':
		return func(r rune) bool { return !(r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v') }, true
	}
	return nil, false
}

func rxLiteralEscape(c rune) rune {
	switch c {
	case 't':
		return '\t'
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 'f':
		return '\f'
	}
	return c
}

func (p *rxParser) atom() *rxNode {
	c := p.src[p.i]
	p.i++
	switch c {
	case '.':
		return &rxNode{kind: 'c', set: func(r rune) bool { return r != '\n' }}
	case '^':
		return &rxNode{kind: '^'}
	case '$':
		return &rxNode{kind: '$'}
	case '(':
		if p.i < len(p.src) && p.src[p.i] == '?' {
			// only non-capturing and named groups are modelled
			if p.i+1 < len(p.src) && p.src[p.i+1] == ':' {
				p.i += 2
			} else if p.i+1 < len(p.src) && p.src[p.i+1] == '<' && p.i+2 < len(p.src) && p.src[p.i+2] != '=' && p.src[p.i+2] != '!' {
				end := strings.IndexRune(string(p.src[p.i:]), '>')
				if end < 0 {
					p.bad = true
					return nil
				}
				p.i += end + 1
			} else {
				p.bad = true
				return nil
			}
		}
		inner := p.alternation()
		if p.i >= len(p.src) || p.src[p.i] != ')' {
			p.bad = true
			return nil
		}
		p.i++
		return &rxNode{kind: 'g', alts: [][]*rxNode{inner}}
	case '[':
		return p.class()
	case '\\':
		if p.i >= len(p.src) {
			p.bad = true
			return nil
		}
		e := p.src[p.i]
		p.i++
		if e == 'b' {
			return &rxNode{kind: 'b'}
		}
		if f, ok := rxClassEscape(e); ok {
			return &rxNode{kind: 'c', set: f}
		}
		if e >= '1' && e <= '9' || e == 'k' || e == 'p' || e == 'P' || e == 'Q' {
			p.bad = true
			return nil
		}
		lit := rxLiteralEscape(e)
		return &rxNode{kind: 'c', set: p.fold(func(r rune) bool { return r == lit })}
	}
	lit := c
	return &rxNode{kind: 'c', set: p.fold(func(r rune) bool { return r == lit })}
}

func (p *rxParser) class() *rxNode {
	neg := false
	if p.i < len(p.src) && p.src[p.i] == '^' {
		neg = true
		p.i++
	}
	var parts []func(rune) bool
	first := true
	for p.i < len(p.src) && (p.src[p.i] != ']' || first) {
		first = false
		c := p.src[p.i]
		p.i++
		if c == '\\' && p.i < len(p.src) {
			e := p.src[p.i]
			p.i++
			if f, ok := rxClassEscape(e); ok {
				parts = append(parts, f)
				continue
			}
			c = rxLiteralEscape(e)
		}
		if p.i+1 < len(p.src) && p.src[p.i] == '-' && p.src[p.i+1] != ']' {
			hi := p.src[p.i+1]
			p.i += 2
			lo := c
			parts = append(parts, func(r rune) bool { return r >= lo && r <= hi })
			continue
		}
		lit := c
		parts = append(parts, func(r rune) bool { return r == lit })
	}
	if p.i >= len(p.src) {
		p.bad = true
		return nil
	}
	p.i++
	set := func(r rune) bool {
		for _, f := range parts {
			if f(r) {
				return true
			}
		}
		return false
	}
	set = p.fold(set)
	if neg {
		inner := set
		set = func(r rune) bool { return !inner(r) }
	}
	return &rxNode{kind: 'c', set: set}
}

// rxCounter replays a backtracking match and counts character reads.
type rxCounter struct {
	in    []rune
	reads int
	limit int
}

type rxAbort struct{}

func (c *rxCounter) read(i int) rune {
	c.reads++
	if c.reads > c.limit {
		panic(rxAbort{})
	}
	return c.in[i]
}

func (c *rxCounter) isWord(i int) bool {
	if i < 0 || i >= len(c.in) {
		return false
	}
	r := c.read(i)
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// seq matches nodes[k:] at position i and then calls k2.
func (c *rxCounter) seq(nodes []*rxNode, k, i int, cont func(int) bool) bool {
	if k == len(nodes) {
		return cont(i)
	}
	n := nodes[k]
	next := func(j int) bool { return c.seq(nodes, k+1, j, cont) }
	return c.rep(n, 0, i, -1, next)
}

// rep matches n repeated (count so far) starting at i.
func (c *rxCounter) rep(n *rxNode, count, i, last int, cont func(int) bool) bool {
	canMore := n.max < 0 || count < n.max
	if count >= n.min && n.lazy {
		if cont(i) {
			return true
		}
	}
	if canMore && !(count >= n.min && i == last) {
		if c.once(n, i, func(j int) bool { return c.rep(n, count+1, j, i, cont) }) {
			return true
		}
	}
	if count >= n.min && !n.lazy {
		return cont(i)
	}
	return false
}

func (c *rxCounter) once(n *rxNode, i int, cont func(int) bool) bool {
	switch n.kind {
	case 'c':
		if i >= len(c.in) || !n.set(c.read(i)) {
			return false
		}
		return cont(i + 1)
	case '^':
		return i == 0 && cont(i)
	case '$':
		return i == len(c.in) && cont(i)
	case 'b':
		if c.isWord(i-1) != c.isWord(i) {
			return cont(i)
		}
		return false
	case 'g', 'a':
		for _, alt := range n.alts {
			if c.seq(alt, 0, i, cont) {
				return true
			}
		}
	}
	return false
}

// plRegexExceeds reports whether matching (or finding from pos) would read
// more than limit characters.
func plRegexExceeds(pattern, flags, input string, full bool, pos, limit int) bool {
	nodes, ok := rxCompile(pattern, strings.Contains(flags, "i"))
	if !ok {
		return false
	}
	c := &rxCounter{in: []rune(input), limit: limit}
	exceeded := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, isAbort := r.(rxAbort); !isAbort {
					panic(r)
				}
				exceeded = true
			}
		}()
		if full {
			c.seq(nodes, 0, 0, func(j int) bool { return j == len(c.in) })
			return
		}
		for start := pos; start <= len(c.in); start++ {
			if c.seq(nodes, 0, start, func(int) bool { return true }) {
				return
			}
		}
	}()
	return exceeded
}
