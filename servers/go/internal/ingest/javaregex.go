package ingest

import (
	"strconv"
	"strings"
	"sync"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jregex"
)

// Regular expressions of the ingest processors. OpenSearch compiles gsub,
// split and kv patterns with java.util.regex and grok patterns with Joni
// (Oniguruma, Ruby syntax); both run here on jregex.

type javaCompiled struct {
	re  *jregex.Regexp
	err *jregex.PatternError
}

var javaPatternCache sync.Map

func compileJava(pattern string) (*jregex.Regexp, *jregex.PatternError) {
	if c, ok := javaPatternCache.Load(pattern); ok {
		cc := c.(javaCompiled)
		return cc.re, cc.err
	}
	re, err := jregex.Compile(pattern, 0)
	javaPatternCache.Store(pattern, javaCompiled{re, err})
	return re, err
}

// javaPattern compiles a pattern at execution time; an invalid one fails the
// document with the PatternSyntaxException.
func javaPattern(pattern string) (*jregex.Regexp, *apierr.Error) {
	re, err := compileJava(pattern)
	if err != nil {
		return nil, &apierr.Error{Type: "pattern_syntax_exception", Status: 400, Reason: err.Message()}
	}
	return re, nil
}

// javaStringSplit is String.split(regex, limit).
func javaStringSplit(s, sep string, limit int) ([]string, *apierr.Error) {
	re, err := javaPattern(sep)
	if err != nil {
		return nil, err
	}
	return re.Split(s, limit), nil
}

// javaReplaceAll is Matcher.replaceAll(replacement).
func javaReplaceAll(re *jregex.Regexp, s, repl string) (string, *apierr.Error) {
	out, err := re.Replace(s, repl, true)
	if err != nil {
		return "", &apierr.Error{Type: err.Type, Status: 400, Reason: err.Reason}
	}
	return out, nil
}

// ---------------------------------------------------------------- grok

// grokRegex is a compiled grok expression: named groups are renumbered
// (Oniguruma names allow characters Java names do not), names[g] is the
// original name of group g.
type grokRegex struct {
	re    *jregex.Regexp
	names []string
}

type grokCompiled struct {
	g   *grokRegex
	msg string
}

var grokCache sync.Map

// grokTranslate rewrites an Oniguruma (Ruby syntax) pattern for jregex:
// named groups become (?<gN>...), \k<name> follows, the Ruby (?m) flag is
// Java's DOTALL and \h is a hex digit. It returns an Oniguruma error message
// for invalid names.
func grokTranslate(p string) (string, []string, string) {
	src := []rune(p)
	var b strings.Builder
	names := []string{""}
	byName := map[string]int{}
	inClass := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if c == '\\' && i+1 < len(src) {
			n := src[i+1]
			switch {
			case n == 'h' && !inClass:
				b.WriteString("[0-9a-fA-F]")
			case n == 'h':
				b.WriteString("0-9a-fA-F")
			case n == 'H' && !inClass:
				b.WriteString("[^0-9a-fA-F]")
			case n == 'k' && !inClass && i+2 < len(src) && src[i+2] == '<':
				end := i + 3
				for end < len(src) && src[end] != '>' {
					end++
				}
				name := string(src[i+3 : min(end, len(src))])
				if name == "" || name[0] >= '0' && name[0] <= '9' || name[0] == '-' {
					return "", nil, "invalid backref number/name"
				}
				g, ok := byName[name]
				if !ok {
					return "", nil, "undefined name <" + name + "> reference"
				}
				b.WriteString(`\k<g` + strconv.Itoa(g) + ">")
				i = end
				continue
			default:
				b.WriteRune(c)
				b.WriteRune(n)
			}
			i++
			continue
		}
		if inClass {
			if c == ']' {
				inClass = false
			}
			b.WriteRune(c)
			continue
		}
		if c == '[' {
			inClass = true
			b.WriteRune(c)
			// a leading ']' (after an optional '^') is a literal
			if i+1 < len(src) && src[i+1] == '^' {
				b.WriteRune('^')
				i++
			}
			if i+1 < len(src) && src[i+1] == ']' {
				b.WriteRune(']')
				i++
			}
			continue
		}
		if c == '(' && i+2 < len(src) && src[i+1] == '?' && src[i+2] == '<' &&
			(i+3 >= len(src) || src[i+3] != '=' && src[i+3] != '!') {
			end := i + 3
			for end < len(src) && src[end] != '>' {
				end++
			}
			if end >= len(src) {
				return "", nil, "invalid group name <" + string(src[i+3:]) + ">"
			}
			name := string(src[i+3 : end])
			if name == "" {
				return "", nil, "group name is empty"
			}
			if name[0] >= '0' && name[0] <= '9' {
				return "", nil, "invalid group name <" + name + ">"
			}
			names = append(names, name)
			g := len(names) - 1
			if _, dup := byName[name]; !dup {
				byName[name] = g
			}
			b.WriteString("(?<g" + strconv.Itoa(g) + ">")
			i = end
			continue
		}
		if c == '(' && i+1 < len(src) && src[i+1] == '?' {
			// inline options (?imx-imx) / (?imx-imx:...): Ruby m is DOTALL
			j := i + 2
			for j < len(src) && strings.ContainsRune("imx-", src[j]) {
				j++
			}
			if j > i+2 && j < len(src) && (src[j] == ')' || src[j] == ':') {
				b.WriteString("(?")
				for _, f := range src[i+2 : j] {
					if f == 'm' {
						f = 's'
					}
					b.WriteRune(f)
				}
				i = j - 1
				continue
			}
		}
		b.WriteRune(c)
	}
	return b.String(), names, ""
}

// onigMessage maps a java.util.regex error to the Oniguruma message.
func onigMessage(e *jregex.PatternError) string {
	d := e.Description()
	switch {
	case d == "Unclosed group":
		return "end pattern with unmatched parenthesis"
	case d == "Unmatched closing ')'":
		return "unmatched close parenthesis"
	case d == "Unclosed character class":
		return "premature end of char-class"
	case strings.HasPrefix(d, "Dangling meta character"):
		return "target of repeat operator is not specified"
	case d == "Illegal repetition range":
		return "upper is smaller than lower in repeat range"
	case d == "Illegal character range":
		return "empty range in char class"
	case strings.HasPrefix(d, "Unknown character property name {"):
		name := strings.TrimSuffix(strings.TrimPrefix(d, "Unknown character property name {"), "}")
		return "invalid character property name <" + name + ">"
	}
	return d
}

func grokRegexCompile(expanded string) (*grokRegex, string) {
	if c, ok := grokCache.Load(expanded); ok {
		cc := c.(grokCompiled)
		return cc.g, cc.msg
	}
	g, msg := grokRegexCompileUncached(expanded)
	grokCache.Store(expanded, grokCompiled{g, msg})
	return g, msg
}

func grokRegexCompileUncached(expanded string) (*grokRegex, string) {
	tr, names, msg := grokTranslate(expanded)
	if msg != "" {
		return nil, msg
	}
	re, err := jregex.Compile(tr, jregex.Multiline)
	if err != nil {
		return nil, onigMessage(err)
	}
	if re.VariableLookbehind() {
		return nil, "invalid pattern in look-behind"
	}
	return &grokRegex{re: re, names: names}, ""
}

// find returns the value of every named group of the first match (nil
// strings for groups that did not participate), or false.
func (g *grokRegex) find(s string) ([]*string, bool) {
	in := []rune(s)
	caps := g.re.FindFrom(in, 0, nil)
	if caps == nil {
		return nil, false
	}
	out := make([]*string, len(g.names))
	for i := 1; i < len(g.names); i++ {
		idx, ok := g.re.GroupIndex("g" + strconv.Itoa(i))
		if ok && caps[2*idx] >= 0 {
			v := string(in[caps[2*idx]:caps[2*idx+1]])
			out[i] = &v
		}
	}
	return out, true
}
