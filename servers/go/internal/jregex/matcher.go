package jregex

// Matcher-style entry points used by Painless and the ingest processors
// (java.util.regex.Matcher find/matches/lookingAt, Pattern.split).

// Pattern flag bits, as accepted by Compile.
const (
	CaseInsensitive = flagCaseInsensitive
	Multiline       = flagMultiline
	Dotall          = flagDotall
	UnicodeCase     = flagUnicodeCase
	Comments        = flagComments
	UnixLines       = flagUnixLines
	UnicodeClass    = flagUnicodeClass
	Literal         = flagLiteral
	CanonEq         = flagCanonEq
)

// Budget, when Steps is positive, bounds the matcher steps of one call;
// running out sets Exceeded and the call reports no match.
type Budget struct {
	Steps    int
	Exceeded bool
}

type abortMatch struct{}

func asciiAlpha(c rune) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// Index is PatternSyntaxException.getIndex (-1 when unknown).
func (e *PatternError) Index() int { return e.index }

// Description is PatternSyntaxException.getDescription.
func (e *PatternError) Description() string { return e.desc }

// String returns the source pattern.
func (re *Regexp) String() string { return re.src }

// GroupIndex returns the number of a named capturing group.
func (re *Regexp) GroupIndex(name string) (int, bool) {
	g, ok := re.names[name]
	return g, ok
}

// GroupNames returns the named groups and their numbers.
func (re *Regexp) GroupNames() map[string]int { return re.names }

func (re *Regexp) newMatcher(in []rune, b *Budget) *jMatcher {
	m := &jMatcher{re: re, in: in, caps: make([]int, 2*(re.groups+1))}
	if b != nil && b.Steps > 0 {
		m.limit = b.Steps
	}
	return m
}

func (m *jMatcher) reset() {
	for x := range m.caps {
		m.caps[x] = -1
	}
}

// run calls f, turning a step-limit abort into b.Exceeded.
func run(b *Budget, f func()) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(abortMatch); !ok {
				panic(r)
			}
			b.Exceeded = true
		}
	}()
	f()
}

// FindFrom returns the capture positions (rune indices, -1 for unset
// groups) of the first match at or after start, like Matcher.find(int).
// Look-behind sees the input before start.
func (re *Regexp) FindFrom(in []rune, start int, b *Budget) []int {
	if b == nil {
		return re.findAt(in, start)
	}
	var out []int
	run(b, func() {
		m := re.newMatcher(in, b)
		for s := start; s <= len(in); s++ {
			m.reset()
			end := -1
			if m.match(re.root, s, func(e int) bool { end = e; return true }) {
				m.caps[0], m.caps[1] = s, end
				out = append([]int{}, m.caps...)
				return
			}
		}
	})
	return out
}

// MatchAll matches the whole input (Matcher.matches) and returns the
// captures, or nil.
func (re *Regexp) MatchAll(in []rune, b *Budget) []int {
	return re.anchored(in, true, b)
}

// MatchPrefix matches at the start of the input (Matcher.lookingAt).
func (re *Regexp) MatchPrefix(in []rune, b *Budget) []int {
	return re.anchored(in, false, b)
}

func (re *Regexp) anchored(in []rune, full bool, b *Budget) []int {
	var out []int
	f := func() {
		m := re.newMatcher(in, b)
		m.reset()
		end := -1
		if m.match(re.root, 0, func(e int) bool {
			if full && e != len(in) {
				return false
			}
			end = e
			return true
		}) {
			m.caps[0], m.caps[1] = 0, end
			out = append([]int{}, m.caps...)
		}
	}
	if b == nil {
		f()
	} else {
		run(b, f)
	}
	return out
}

// FindString reports whether the pattern occurs anywhere in s.
func (re *Regexp) FindString(s string) bool {
	return re.findAt([]rune(s), 0) != nil
}

// MatchString reports whether the whole of s matches.
func (re *Regexp) MatchString(s string) bool {
	return re.FullMatch([]rune(s))
}

// Split splits s around matches like java.util.regex.Pattern.split(s, limit).
func (re *Regexp) Split(s string, limit int) []string {
	in := []rune(s)
	index := 0
	limited := limit > 0
	var list []string
	matched := false
	pos := 0
	for pos <= len(in) {
		m := re.findAt(in, pos)
		if m == nil {
			break
		}
		if m[1] == m[0] {
			pos = m[1] + 1
		} else {
			pos = m[1]
		}
		if !limited || len(list) < limit-1 {
			if index == 0 && m[0] == 0 && m[0] == m[1] {
				continue
			}
			list = append(list, string(in[index:m[0]]))
			index = m[1]
			matched = true
		} else if len(list) == limit-1 {
			list = append(list, string(in[index:]))
			index = m[1]
			matched = true
		}
	}
	if !matched || index == 0 {
		return []string{s}
	}
	if !limited || len(list) < limit {
		list = append(list, string(in[index:]))
	}
	if limit == 0 {
		n := len(list)
		for n > 0 && list[n-1] == "" {
			n--
		}
		list = list[:n]
	}
	return list
}
