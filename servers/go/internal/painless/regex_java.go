package painless

import (
	"strings"
	"sync"
	"unicode/utf16"

	"slopensearch/internal/jregex"
)

// java.util.regex.Pattern and Matcher for Painless, on top of jregex (a
// java.util.regex emulation): lookaround, backreferences, possessive
// quantifiers, named groups and PatternSyntaxException messages behave like
// Java. Positions are rune indices internally and UTF-16 offsets in scripts.

type plPattern struct {
	src   string
	flags string
	re    *jregex.Regexp
}

type plMatcher struct {
	p     *plPattern
	input string
	in    *plInterp
	runes []rune
	loc   []int // rune indices of the last match, nil when none
	pos   int   // rune index where the next find() starts
	first bool  // no search yet / reset
}

// plRegexFlags maps Painless regex literal flags to Pattern flags (ERegex).
func plRegexFlags(flags string) int {
	f := 0
	for _, c := range flags {
		switch c {
		case 'c':
			f |= jregex.CanonEq
		case 'i':
			f |= jregex.CaseInsensitive
		case 'l':
			f |= jregex.Literal
		case 'm':
			f |= jregex.Multiline
		case 's':
			f |= jregex.Dotall
		case 'U':
			f |= jregex.UnicodeClass
		case 'u':
			f |= jregex.UnicodeCase
		case 'x':
			f |= jregex.Comments
		}
	}
	return f
}

// plJavaFlagsValue is Pattern.flags() for the literal flags.
func plJavaFlagsValue(flags string) int32 {
	v := int32(0)
	for _, c := range flags {
		switch c {
		case 'c':
			v |= 0x80
		case 'i':
			v |= 0x02
		case 'l':
			v |= 0x10
		case 'm':
			v |= 0x08
		case 's':
			v |= 0x20
		case 'U':
			v |= 0x100
		case 'u':
			v |= 0x40
		case 'x':
			v |= 0x04
		}
	}
	return v
}

// plRegexMemberSigs is the Painless whitelist (java.util.regex.txt plus the
// java.lang.Object methods) for statically typed Pattern / Matcher receivers.
var plRegexMemberSigs = map[string]map[string]plSig{
	"Pattern": ParseSigs("asPredicate:def()", "flags:int()", "matcher:Matcher(CharSequence)",
		"pattern:String()", "quote:String(String)", "split:def(CharSequence)", "split:def(CharSequence,int)",
		"splitAsStream:def(CharSequence)", "toString:String()", "equals:boolean(def)", "hashCode:int()"),
	"Matcher": ParseSigs("end:int()", "end:int(int)", "find:boolean()", "find:boolean(int)",
		"group:String()", "group:String(int)", "namedGroup:String(String)", "groupCount:int()",
		"hasAnchoringBounds:boolean()", "hasTransparentBounds:boolean()", "hitEnd:boolean()",
		"lookingAt:boolean()", "matches:boolean()", "pattern:Pattern()", "quoteReplacement:String(String)",
		"region:Matcher(int,int)", "regionEnd:int()", "regionStart:int()", "replaceAll:String(String)",
		"replaceFirst:String(String)", "requireEnd:boolean()", "reset:Matcher()", "start:int()",
		"start:int(int)", "useAnchoringBounds:Matcher(boolean)", "usePattern:Matcher(Pattern)",
		"useTransparentBounds:Matcher(boolean)", "toString:String()", "equals:boolean(def)", "hashCode:int()"),
}

type plCompiled struct {
	re  *jregex.Regexp
	err *jregex.PatternError
}

var plPatternCache sync.Map

func plCompileJava(src, flags string) (*jregex.Regexp, *jregex.PatternError) {
	key := flags + "/" + src
	if c, ok := plPatternCache.Load(key); ok {
		cc := c.(plCompiled)
		return cc.re, cc.err
	}
	re, err := jregex.Compile(src, plRegexFlags(flags))
	plPatternCache.Store(key, plCompiled{re, err})
	return re, err
}

// plRegexCompileError is the compile error of an invalid regex constant.
func plRegexCompileError(src, flags string, off int) *CompileError {
	_, perr := plCompileJava(src, flags)
	if perr == nil {
		return nil
	}
	at := off
	if perr.Index() >= 0 {
		at = off + 1 + perr.Index()
	}
	return &CompileError{Type: "illegal_argument_exception", Offset: at,
		Reason: "invalid regular expression: could not compile regex constant [" + src + "] with flags [" + flags + "]",
		Cause:  &CompileError{Type: "pattern_syntax_exception", Reason: perr.Message()}}
}

func plCompilePattern(src, flags string, off int) (any, *RuntimeError) {
	re, err := plCompileJava(src, flags)
	if err != nil {
		return nil, &RuntimeError{Type: "pattern_syntax_exception", Offset: off, Reason: err.Message()}
	}
	return &plPattern{src: src, flags: flags, re: re}, nil
}

func (p *plPattern) matcher(s string, in *plInterp) *plMatcher {
	return &plMatcher{p: p, input: s, in: in, runes: []rune(s), first: true}
}

func plUTF16Len(rs []rune) int {
	n := 0
	for _, r := range rs {
		n += utf16.RuneLen(r)
	}
	return n
}

// plRuneIndex converts a UTF-16 offset to a rune index (-1 if out of range).
func plRuneIndex(rs []rune, u int) int {
	n := 0
	for i, r := range rs {
		if n >= u {
			return i
		}
		n += utf16.RuneLen(r)
	}
	if n == u {
		return len(rs)
	}
	return -1
}

// plRegexStepCap bounds patterns the character-read replay cannot model.
const plRegexStepCap = 5_000_000

func (m *plMatcher) breakerError(off, limit int) *RuntimeError {
	return &RuntimeError{Type: "circuit_breaking_exception", Offset: off,
		Reason: "[scripting] Regular expression considered too many characters, pattern: [" + m.p.src +
			"], limit factor: [6], char limit: [" + itoa(limit) + "], count: [" + itoa(limit+1) +
			"], wrapped: [" + m.input + "], this limit can be changed by changed by the [script.painless.regex.limit-factor] setting"}
}

// checkBudget enforces Painless' regex circuit breaker (limit factor 6).
func (m *plMatcher) checkBudget(off int, full bool, pos int) *RuntimeError {
	limit := 6 * len(m.runes)
	if !plRegexExceeds(m.p.src, m.p.flags, m.input, full, pos, limit) {
		return nil
	}
	return m.breakerError(off, limit)
}

func (m *plMatcher) budget() *jregex.Budget { return &jregex.Budget{Steps: plRegexStepCap} }

func (m *plMatcher) setMatch(loc []int) bool {
	m.loc = loc
	m.first = false
	if loc == nil {
		return false
	}
	if loc[1] == loc[0] {
		m.pos = loc[1] + 1
	} else {
		m.pos = loc[1]
	}
	return true
}

func (m *plMatcher) search(from, off int) (bool, *RuntimeError) {
	if err := m.checkBudget(off, false, from); err != nil {
		return false, err
	}
	if from > len(m.runes) {
		m.loc = nil
		m.first = false
		return false, nil
	}
	b := m.budget()
	loc := m.p.re.FindFrom(m.runes, from, b)
	if b.Exceeded {
		return false, m.breakerError(off, 6*len(m.runes))
	}
	return m.setMatch(loc), nil
}

func (m *plMatcher) find(off int) (bool, *RuntimeError) {
	from := m.pos
	if m.first {
		from = 0
	}
	return m.search(from, off)
}

func (m *plMatcher) matches(off int) (bool, *RuntimeError) {
	if err := m.checkBudget(off, true, 0); err != nil {
		return false, err
	}
	b := m.budget()
	loc := m.p.re.MatchAll(m.runes, b)
	if b.Exceeded {
		return false, m.breakerError(off, 6*len(m.runes))
	}
	return m.setMatch(loc), nil
}

func (m *plMatcher) lookingAt(off int) (bool, *RuntimeError) {
	b := m.budget()
	loc := m.p.re.MatchPrefix(m.runes, b)
	if b.Exceeded {
		return false, m.breakerError(off, 6*len(m.runes))
	}
	return m.setMatch(loc), nil
}

func plMatcherErr(etype, reason string, off int, frames ...string) *RuntimeError {
	fs := make([]string, len(frames))
	for i, f := range frames {
		fs[i] = "java.base/java.util.regex.Matcher." + f
	}
	return &RuntimeError{Type: etype, Reason: reason, Offset: off, Frames: fs}
}

// checkGroup mirrors Matcher.checkMatch / checkGroup for group-like calls;
// site is the frame of the public method ("group(Matcher.java:685)", ...).
func (m *plMatcher) checkGroup(g, off int, site ...string) *RuntimeError {
	if m.loc == nil {
		return plMatcherErr("illegal_state_exception", "No match found", off, append([]string{"checkMatch(Matcher.java:1850)"}, site...)...)
	}
	if g < 0 || g > m.p.re.NumGroups() {
		return plMatcherErr("index_out_of_bounds_exception", "No group "+itoa(g), off, append([]string{"checkGroup(Matcher.java:1845)"}, site...)...)
	}
	return nil
}

func (m *plMatcher) group(g int, off int, site ...string) (any, *RuntimeError) {
	if err := m.checkGroup(g, off, site...); err != nil {
		return nil, err
	}
	if m.loc[2*g] < 0 {
		return nil, nil
	}
	return string(m.runes[m.loc[2*g]:m.loc[2*g+1]]), nil
}

func (m *plMatcher) offset(g int, end bool, off int, site string) (any, *RuntimeError) {
	if err := m.checkGroup(g, off, site); err != nil {
		return nil, err
	}
	i := m.loc[2*g]
	if end {
		i = m.loc[2*g+1]
	}
	if i < 0 {
		return int32(-1), nil
	}
	return int32(plUTF16Len(m.runes[:i])), nil
}

// replacementFrames are the Matcher frames of a replacement string error.
func replacementFrames(reason string) []string {
	switch {
	case strings.HasPrefix(reason, "No group with name"):
		return []string{"appendExpandedReplacement(Matcher.java:1103)"}
	case strings.HasPrefix(reason, "No group "):
		return []string{"checkGroup(Matcher.java:1845)", "start(Matcher.java:523)", "appendExpandedReplacement(Matcher.java:1134)"}
	case reason == "character to be escaped is missing":
		return []string{"appendExpandedReplacement(Matcher.java:1063)"}
	case reason == "Illegal group reference: group index is missing":
		return []string{"appendExpandedReplacement(Matcher.java:1073)"}
	case reason == "Illegal group reference":
		return []string{"appendExpandedReplacement(Matcher.java:1111)"}
	case reason == "named capturing group is missing trailing '}'":
		return []string{"appendExpandedReplacement(Matcher.java:1094)"}
	case strings.HasSuffix(reason, "starts with digit character"):
		return []string{"appendExpandedReplacement(Matcher.java:1098)"}
	}
	return nil
}

// replace implements Matcher.replaceAll / replaceFirst(String).
func (m *plMatcher) replace(repl string, all bool, off int) (any, *RuntimeError) {
	if err := m.checkBudget(off, false, 0); err != nil {
		return nil, err
	}
	in := m.runes
	var out []rune
	last, pos := 0, 0
	for pos <= len(in) {
		b := m.budget()
		loc := m.p.re.FindFrom(in, pos, b)
		if b.Exceeded {
			return nil, m.breakerError(off, 6*len(in))
		}
		if loc == nil {
			break
		}
		r, err := m.p.re.ExpandReplacement(in, loc, repl)
		if err != nil {
			site := "replaceFirst(Matcher.java:1444)"
			if all {
				site = "replaceAll(Matcher.java:1227)"
			}
			frames := append(replacementFrames(err.Reason), "appendReplacement(Matcher.java:1041)", site)
			return nil, plMatcherErr(err.Type, err.Reason, off, frames...)
		}
		out = append(out, in[last:loc[0]]...)
		out = append(out, r...)
		last = loc[1]
		if loc[1] == loc[0] {
			pos = loc[1] + 1
		} else {
			pos = loc[1]
		}
		if !all {
			break
		}
	}
	out = append(out, in[last:]...)
	m.loc = nil
	m.first = true
	m.pos = 0
	return string(out), nil
}

// replaceWith implements the String.replaceAll/replaceFirst(Pattern,
// Function) augmentation: the function receives the live matcher.
func (p *plPattern) replaceWith(in *plInterp, s string, fn any, first bool, off int) (any, *RuntimeError) {
	m := p.matcher(s, in)
	var sb strings.Builder
	last := 0
	for {
		ok, err := m.find(off)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		loc := m.loc
		r, err := in.callFn(fn, []any{m}, off)
		if err != nil {
			return nil, err
		}
		sb.WriteString(string(m.runes[last:loc[0]]))
		sb.WriteString(ToString(r))
		last = loc[1]
		if first {
			break
		}
	}
	sb.WriteString(string(m.runes[last:]))
	return sb.String(), nil
}

func (p *plPattern) split(s string, limit int) *Array {
	arr := &Array{Elem: "String"}
	for _, part := range p.re.Split(s, limit) {
		arr.Items = append(arr.Items, part)
	}
	return arr
}

func (p *plPattern) method(in *plInterp, name string, args []any, off int) (any, *RuntimeError, bool) {
	n := len(args)
	switch name {
	case "matcher":
		return p.matcher(ToString(args[0]), in), nil, true
	case "pattern", "toString":
		return p.src, nil, true
	case "flags":
		return plJavaFlagsValue(p.flags), nil, true
	case "split":
		limit := 0
		if n == 2 {
			limit = int(I64(args[1]))
		}
		return p.split(ToString(args[0]), limit), nil, true
	case "splitAsStream":
		parts := p.re.Split(ToString(args[0]), 0)
		if len(parts) == 1 && parts[0] == "" {
			parts = nil
		}
		st := &plStream{kind: "obj"}
		for _, part := range parts {
			st.items = append(st.items, part)
		}
		return st, nil, true
	case "asPredicate":
		re := p.re
		return plNativeFn(func(xs []any, o int) (any, *RuntimeError) {
			return re.FindString(ToString(xs[0])), nil
		}), nil, true
	}
	return nil, nil, false
}

// plWrongMethodType is the error of calling an int-taking Matcher method
// with a String through def (the whitelist has no String overloads).
func plWrongMethodType(ret string, off int) *RuntimeError {
	return &RuntimeError{Type: "wrong_method_type_exception", Offset: off,
		Reason: "cannot convert MethodHandle(Matcher,int)" + ret + " to (Object,String)Object"}
}

func (m *plMatcher) method(name string, args []any, off int) (any, *RuntimeError, bool) {
	n := len(args)
	intArg := func() (int, bool) {
		_, isStr := args[0].(string)
		return int(I64(args[0])), !isStr
	}
	switch name {
	case "find":
		if n == 1 {
			u, _ := intArg()
			idx := plRuneIndex(m.runes, u)
			if u < 0 || idx < 0 {
				return nil, plMatcherErr("index_out_of_bounds_exception", "Illegal start index", off, "find(Matcher.java:810)"), true
			}
			m.loc = nil
			m.first = false
			v, err := m.search(idx, off)
			return v, err, true
		}
		v, err := m.find(off)
		return v, err, true
	case "matches":
		v, err := m.matches(off)
		return v, err, true
	case "lookingAt":
		v, err := m.lookingAt(off)
		return v, err, true
	case "group":
		if n == 0 {
			v, err := m.group(0, off, "group(Matcher.java:685)", "group(Matcher.java:643)")
			return v, err, true
		}
		g, ok := intArg()
		if !ok {
			return nil, plWrongMethodType("String", off), true
		}
		v, err := m.group(g, off, "group(Matcher.java:686)")
		if err != nil && err.Type == "illegal_state_exception" {
			err.Frames[1] = "java.base/java.util.regex.Matcher.group(Matcher.java:685)"
		}
		return v, err, true
	case "namedGroup":
		name := ToString(args[0])
		g, ok := m.p.re.GroupIndex(name)
		if !ok {
			return nil, plMatcherErr("illegal_argument_exception", "No group with name <"+name+">", off,
				"getMatchedGroupIndex(Matcher.java:1839)", "group(Matcher.java:720)"), true
		}
		v, err := m.group(g, off, "group(Matcher.java:720)")
		return v, err, true
	case "groupCount":
		return int32(m.p.re.NumGroups()), nil, true
	case "start", "end":
		g := 0
		if n == 1 {
			var ok bool
			if g, ok = intArg(); !ok {
				return nil, plWrongMethodType("int", off), true
			}
		}
		site := "start(Matcher.java:493)"
		switch {
		case name == "end" && n == 0:
			site = "end(Matcher.java:557)"
		case name == "end":
			site = "end(Matcher.java:591)"
		case n == 1:
			site = "start(Matcher.java:523)"
		}
		v, err := m.offset(g, name == "end", off, site)
		return v, err, true
	case "replaceAll", "replaceFirst":
		v, err := m.replace(ToString(args[0]), name == "replaceAll", off)
		return v, err, true
	case "reset":
		m.pos = 0
		m.loc = nil
		m.first = true
		return m, nil, true
	case "pattern":
		return m.p, nil, true
	case "hitEnd":
		return m.pos >= len(m.runes), nil, true
	case "regionStart":
		return int32(0), nil, true
	case "regionEnd":
		return int32(plUTF16Len(m.runes)), nil, true
	case "hasAnchoringBounds":
		return true, nil, true
	case "hasTransparentBounds":
		return false, nil, true
	}
	return nil, nil, false
}
