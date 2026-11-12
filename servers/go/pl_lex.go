package main

import (
	"strings"
)

// Painless engine: lexer.
//
// The engine mirrors the semantics of OpenSearch's Painless closely enough
// that results, error types, messages and positions match a real node. It is
// split over the pl_*.go files: lexing, parsing, static analysis, runtime
// values, the interpreter, the class library and the REST glue.

type plTokKind int

const (
	plTEOF plTokKind = iota
	plTIdent
	plTType
	plTInt
	plTFloat
	plTString
	plTRegex
	plTOp
)

type plTok struct {
	kind  plTokKind
	text  string // operator text, identifier, raw number text, decoded string
	pos   int
	end   int
	flags string // regex flags
}

// plTypeNames are identifiers the lexer turns into type tokens, like the
// Painless lexer does for every class in the whitelist.
var plTypeNames = map[string]bool{
	"def": true, "void": true, "boolean": true, "byte": true, "short": true, "char": true,
	"int": true, "long": true, "float": true, "double": true,
	"Object": true, "String": true, "Integer": true, "Long": true, "Double": true, "Float": true,
	"Short": true, "Byte": true, "Character": true, "Boolean": true, "Number": true,
	"Math": true, "StrictMath": true, "CharSequence": true, "StringBuilder": true,
	"List": true, "ArrayList": true, "LinkedList": true, "Map": true, "HashMap": true,
	"TreeMap": true, "LinkedHashMap": true, "Set": true, "HashSet": true, "TreeSet": true,
	"LinkedHashSet": true, "Collection": true, "Collections": true, "Collectors": true,
	"Comparator": true, "Iterator": true, "Iterable": true, "Arrays": true, "Objects": true,
	"Pattern": true, "Matcher": true, "Optional": true, "OptionalInt": true, "OptionalLong": true,
	"OptionalDouble": true, "Stream": true, "IntStream": true, "LongStream": true,
	"DoubleStream": true, "Deque": true, "ArrayDeque": true, "Queue": true,
	"Exception": true, "RuntimeException": true, "IllegalArgumentException": true,
	"IllegalStateException": true, "ArithmeticException": true, "NullPointerException": true,
	"UnsupportedOperationException": true, "IndexOutOfBoundsException": true,
	"NumberFormatException": true, "ClassCastException": true,
	"Instant": true, "ZonedDateTime": true, "LocalDate": true, "LocalDateTime": true,
	"LocalTime": true, "ZoneId": true, "ZoneOffset": true, "Duration": true, "Period": true,
	"ChronoUnit": true, "DayOfWeek": true, "Month": true, "DateTimeFormatter": true,
	"Debug": true, "Base64": true, "Function": true, "BiFunction": true, "Supplier": true,
	"Consumer": true, "Predicate": true, "BiConsumer": true, "Random": true,
}

var plOps = []string{
	">>>=", "===", "!==", "==~", ">>>", "<<=", ">>=",
	"==", "!=", "<=", ">=", "&&", "||", "++", "--", "+=", "-=", "*=", "/=", "%=",
	"&=", "|=", "^=", "<<", ">>", "=~", "?.", "?:", "->", "::",
	"+", "-", "*", "/", "%", "&", "|", "^", "!", "~", "<", ">", "=", "?", ":",
	";", ",", ".", "(", ")", "[", "]", "{", "}",
}

func plLex(src string) ([]plTok, *plCompileErr) {
	toks := []plTok{}
	i := 0
	n := len(src)
	slashIsRegex := func() bool {
		if len(toks) == 0 {
			return true
		}
		t := toks[len(toks)-1]
		switch t.kind {
		case plTIdent, plTType, plTInt, plTFloat, plTString, plTRegex:
			return false
		case plTOp:
			return t.text != ")" && t.text != "]" && t.text != "}" && t.text != "++" && t.text != "--"
		}
		return true
	}
	for i < n {
		c := src[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		if c == '/' && i+1 < n && src[i+1] == '/' {
			for i < n && src[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < n && src[i+1] == '*' {
			j := strings.Index(src[i+2:], "*/")
			if j < 0 {
				i = n
			} else {
				i = i + 2 + j + 2
			}
			continue
		}
		start := i
		switch {
		case c == '\'' || c == '"':
			q := c
			var sb strings.Builder
			j := i + 1
			closed := false
			for j < n {
				ch := src[j]
				if ch == '\\' {
					if j+1 < n && (src[j+1] == '\\' || src[j+1] == q) {
						sb.WriteByte(src[j+1])
						j += 2
						continue
					}
					end := j + 2
					if end > n {
						end = n
					}
					qs := string(q)
					return nil, &plCompileErr{typ: "illegal_argument_exception", offset: start,
						reason: "unexpected character [" + src[start:end] + "]. The only valid escape sequences in strings starting with [" +
							qs + "] are [\\\\] and [\\" + qs + "].",
						cause: &plCompileErr{typ: "lexer_no_viable_alt_exception", nilReason: true}}
				}
				if ch == q {
					closed = true
					j++
					break
				}
				sb.WriteByte(ch)
				j++
			}
			if !closed {
				return nil, &plCompileErr{typ: "illegal_argument_exception", offset: start,
					reason: "unexpected character [" + src[start:] + "].",
					cause:  &plCompileErr{typ: "lexer_no_viable_alt_exception", nilReason: true}}
			}
			toks = append(toks, plTok{kind: plTString, text: sb.String(), pos: start, end: j})
			i = j
		case c >= '0' && c <= '9' || (c == '.' && i+1 < n && src[i+1] >= '0' && src[i+1] <= '9'):
			j := i
			isFloat := false
			if c == '0' && j+1 < n && (src[j+1] == 'x' || src[j+1] == 'X') {
				j += 2
				for j < n && isHexByte(src[j]) {
					j++
				}
				if j < n && (src[j] == 'l' || src[j] == 'L') {
					j++
				}
				toks = append(toks, plTok{kind: plTInt, text: src[i:j], pos: start, end: j})
				i = j
				continue
			}
			for j < n && src[j] >= '0' && src[j] <= '9' {
				j++
			}
			if j < n && src[j] == '.' && j+1 < n && src[j+1] >= '0' && src[j+1] <= '9' {
				isFloat = true
				j++
				for j < n && src[j] >= '0' && src[j] <= '9' {
					j++
				}
			} else if j < n && src[j] == '.' && !(j+1 < n && (isAlphaByte(src[j+1]) || src[j+1] == '_')) {
				// "1." is a double literal
				isFloat = true
				j++
			}
			if j < n && (src[j] == 'e' || src[j] == 'E') {
				k := j + 1
				if k < n && (src[k] == '+' || src[k] == '-') {
					k++
				}
				if k < n && src[k] >= '0' && src[k] <= '9' {
					isFloat = true
					j = k
					for j < n && src[j] >= '0' && src[j] <= '9' {
						j++
					}
				}
			}
			if j < n {
				switch src[j] {
				case 'f', 'F', 'd', 'D':
					isFloat = true
					j++
				case 'l', 'L':
					if !isFloat {
						j++
					}
				}
			}
			kind := plTInt
			if isFloat {
				kind = plTFloat
			}
			toks = append(toks, plTok{kind: kind, text: src[i:j], pos: start, end: j})
			i = j
		case isAlphaByte(c) || c == '_' || c == '$':
			j := i
			for j < n && (isAlphaByte(src[j]) || src[j] == '_' || src[j] == '$' || (src[j] >= '0' && src[j] <= '9')) {
				j++
			}
			word := src[i:j]
			kind := plTIdent
			if plTypeNames[word] {
				kind = plTType
			}
			toks = append(toks, plTok{kind: kind, text: word, pos: start, end: j})
			i = j
		case c == '/' && slashIsRegex():
			j := i + 1
			var sb strings.Builder
			for j < n && src[j] != '/' && src[j] != '\n' {
				if src[j] == '\\' && j+1 < n {
					sb.WriteByte(src[j])
					sb.WriteByte(src[j+1])
					j += 2
					continue
				}
				sb.WriteByte(src[j])
				j++
			}
			if j >= n || src[j] != '/' {
				return nil, &plCompileErr{typ: "illegal_argument_exception", offset: start,
					reason: "unexpected character [" + src[start:] + "].",
					cause:  &plCompileErr{typ: "lexer_no_viable_alt_exception", nilReason: true}}
			}
			j++
			fs := j
			for j < n && strings.IndexByte("cilmsUux", src[j]) >= 0 {
				j++
			}
			toks = append(toks, plTok{kind: plTRegex, text: sb.String(), flags: src[fs:j], pos: start, end: j})
			i = j
		default:
			matched := ""
			for _, op := range plOps {
				if strings.HasPrefix(src[i:], op) {
					matched = op
					break
				}
			}
			if matched == "" {
				return nil, &plCompileErr{typ: "illegal_argument_exception", offset: start,
					reason: "unexpected character [" + string(c) + "].",
					cause:  &plCompileErr{typ: "lexer_no_viable_alt_exception", nilReason: true}}
			}
			toks = append(toks, plTok{kind: plTOp, text: matched, pos: start, end: i + len(matched)})
			i += len(matched)
		}
	}
	toks = append(toks, plTok{kind: plTEOF, pos: n, end: n})
	return toks, nil
}

func isHexByte(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
