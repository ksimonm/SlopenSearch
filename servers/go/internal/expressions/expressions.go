// Package expressions implements Lucene expressions ("lang": "expression"):
// a JavaScript-like numeric language. Everything is a double; variables are
// bound at link time to doc values, _score, _value or numeric params.
package expressions

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"slopensearch/internal/painless"
)

type exNode interface{}

type exNum struct{ v float64 }
type exVar struct {
	name string // full variable text, e.g. doc['price'].value
	pos  int
}
type exUnary struct {
	op string
	x  exNode
}
type exBinary struct {
	op   string
	l, r exNode
}
type exCond struct{ c, a, b exNode }
type exCall struct {
	fn   string
	args []exNode
	pos  int
}

// Script is a compiled expression.
type Script struct {
	src  string
	root exNode
	vars []*exVar
}

// Error is a compile (parse_exception) or link error of an expression.
type Error struct {
	Link   bool // a link error (unknown variable, field, ...) rather than a syntax error
	Reason string
	Offset int  // offset of the error in the source
	EOF    bool // the source ended unexpectedly
}

func (e *Error) Error() string { return e.Reason }

type exParser struct {
	src  string
	i    int
	vars []*exVar
}

func (p *exParser) skip() {
	for p.i < len(p.src) && (p.src[p.i] == ' ' || p.src[p.i] == '\t' || p.src[p.i] == '\n' || p.src[p.i] == '\r') {
		p.i++
	}
}

func (p *exParser) peekOp(ops ...string) string {
	p.skip()
	for _, op := range ops {
		if strings.HasPrefix(p.src[p.i:], op) {
			// do not split longer operators
			rest := p.src[p.i+len(op):]
			if (op == "<" || op == ">") && (strings.HasPrefix(rest, "<") || strings.HasPrefix(rest, ">") || strings.HasPrefix(rest, "=")) {
				continue
			}
			if (op == "&" && strings.HasPrefix(rest, "&")) || (op == "|" && strings.HasPrefix(rest, "|")) {
				continue
			}
			if op == "!" && strings.HasPrefix(rest, "=") {
				continue
			}
			return op
		}
	}
	return ""
}

func (p *exParser) errHere() *Error {
	p.skip()
	if p.i >= len(p.src) {
		return &Error{Reason: "unexpected end of expression", Offset: len(p.src), EOF: true}
	}
	return &Error{Reason: "unexpected character '" + string(p.src[p.i]) + "' on line (1) position (" + strconv.Itoa(p.i) + ")", Offset: p.i}
}

func parse(src string) (*Script, *Error) {
	p := &exParser{src: src}
	n, err := p.cond()
	if err != nil {
		return nil, err
	}
	p.skip()
	if p.i < len(p.src) {
		return nil, p.errHere()
	}
	return &Script{src: src, root: n, vars: p.vars}, nil
}

func (p *exParser) cond() (exNode, *Error) {
	c, err := p.binary(0)
	if err != nil {
		return nil, err
	}
	if p.peekOp("?") != "" {
		p.i++
		a, err := p.cond()
		if err != nil {
			return nil, err
		}
		if p.peekOp(":") == "" {
			return nil, p.errHere()
		}
		p.i++
		b, err := p.cond()
		if err != nil {
			return nil, err
		}
		return &exCond{c, a, b}, nil
	}
	return c, nil
}

var exLevels = [][]string{
	{"||"}, {"&&"}, {"|"}, {"^"}, {"&"}, {"==", "!="}, {"<=", ">=", "<", ">"},
	{">>>", "<<", ">>"}, {"+", "-"}, {"*", "/", "%"},
}

func (p *exParser) binary(level int) (exNode, *Error) {
	if level >= len(exLevels) {
		return p.unary()
	}
	l, err := p.binary(level + 1)
	if err != nil {
		return nil, err
	}
	for {
		op := p.peekOp(exLevels[level]...)
		if op == "" {
			return l, nil
		}
		p.i += len(op)
		r, err := p.binary(level + 1)
		if err != nil {
			return nil, err
		}
		l = &exBinary{op, l, r}
	}
}

func (p *exParser) unary() (exNode, *Error) {
	if op := p.peekOp("!", "~", "-", "+"); op != "" {
		p.i++
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		if op == "+" {
			return x, nil
		}
		return &exUnary{op, x}, nil
	}
	return p.primary()
}

func isExIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isExIdent(c byte) bool { return isExIdentStart(c) || (c >= '0' && c <= '9') }

func (p *exParser) primary() (exNode, *Error) {
	p.skip()
	if p.i >= len(p.src) {
		return nil, p.errHere()
	}
	c := p.src[p.i]
	switch {
	case c == '(':
		p.i++
		x, err := p.cond()
		if err != nil {
			return nil, err
		}
		if p.peekOp(")") == "" {
			return nil, p.errHere()
		}
		p.i++
		return x, nil
	case c >= '0' && c <= '9' || c == '.':
		j := p.i
		if strings.HasPrefix(p.src[j:], "0x") || strings.HasPrefix(p.src[j:], "0X") {
			j += 2
			for j < len(p.src) && isHexByte(p.src[j]) {
				j++
			}
			v, _ := strconv.ParseInt(p.src[p.i+2:j], 16, 64)
			p.i = j
			return &exNum{float64(v)}, nil
		}
		for j < len(p.src) && (p.src[j] >= '0' && p.src[j] <= '9' || p.src[j] == '.') {
			j++
		}
		if j < len(p.src) && (p.src[j] == 'e' || p.src[j] == 'E') {
			j++
			if j < len(p.src) && (p.src[j] == '+' || p.src[j] == '-') {
				j++
			}
			for j < len(p.src) && p.src[j] >= '0' && p.src[j] <= '9' {
				j++
			}
		}
		text := p.src[p.i:j]
		v, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, &Error{Reason: "unexpected character '" + text + "'", Offset: p.i}
		}
		if len(text) > 1 && text[0] == '0' && !strings.ContainsAny(text, ".eE") {
			o, _ := strconv.ParseInt(text, 8, 64)
			v = float64(o)
		}
		p.i = j
		return &exNum{v}, nil
	case isExIdentStart(c):
		start := p.i
		j := p.i
		for j < len(p.src) && isExIdent(p.src[j]) {
			j++
		}
		ident := p.src[p.i:j]
		k := j
		for k < len(p.src) && p.src[k] == ' ' {
			k++
		}
		if k < len(p.src) && p.src[k] == '(' && !strings.Contains(ident, ".") {
			p.i = k + 1
			call := &exCall{fn: ident, pos: start}
			if p.peekOp(")") != "" {
				p.i++
				return call, nil
			}
			for {
				a, err := p.cond()
				if err != nil {
					return nil, err
				}
				call.args = append(call.args, a)
				if p.peekOp(",") != "" {
					p.i++
					continue
				}
				if p.peekOp(")") == "" {
					return nil, p.errHere()
				}
				p.i++
				return call, nil
			}
		}
		// variable: ID ([...] | .ID)* with an optional trailing ()
		for j < len(p.src) {
			if p.src[j] == '[' {
				end := strings.IndexByte(p.src[j:], ']')
				if end < 0 {
					p.i = len(p.src)
					return nil, p.errHere()
				}
				j += end + 1
				continue
			}
			if p.src[j] == '.' && j+1 < len(p.src) && isExIdentStart(p.src[j+1]) {
				j++
				for j < len(p.src) && isExIdent(p.src[j]) {
					j++
				}
				if strings.HasPrefix(p.src[j:], "()") {
					j += 2
				}
				continue
			}
			break
		}
		v := &exVar{name: p.src[start:j], pos: start}
		p.vars = append(p.vars, v)
		p.i = j
		return v, nil
	}
	return nil, p.errHere()
}

// ---- linking ----

// Doc gives access to the values of a document.
type Doc interface {
	Values(field string) []any
}

// Kind classifies a mapped field for doc['field'] variables.
type Kind int

const (
	KindOther  Kind = iota // not usable in expressions
	KindNumber             // numeric and boolean fields
	KindDate               // date and date_nanos fields
	KindGeo                // geo_point fields
)

// Env is what Link binds variables against.
type Env struct {
	// Fields resolves a mapped field; nil when there is no index (no doc access).
	Fields func(field string) (kind Kind, found bool)
	// Params holds numeric parameters (or bucket path values).
	Params map[string]any
	// ToFloat converts a JSON value to a number.
	ToFloat    func(v any) (float64, bool)
	AllowValue bool // _value is bound (aggs)
	AllowScore bool // _score is bound
}

// exBinding resolves one variable for a document.
type exBinding func(doc Doc, score float64, value any) float64

// Linked is an expression whose variables are bound.
type Linked struct {
	script *Script
	binds  map[*exVar]exBinding
}

func parseDocVar(name string) (field, member string, fieldOff int, ok bool) {
	if !strings.HasPrefix(name, "doc[") {
		return "", "", 0, false
	}
	end := strings.IndexByte(name, ']')
	if end < 0 {
		return "", "", 0, false
	}
	inner := name[4:end]
	fieldOff = 4
	if len(inner) >= 2 && (inner[0] == '\'' || inner[0] == '"') {
		inner = inner[1 : len(inner)-1]
		fieldOff = 5
	}
	return inner, strings.TrimPrefix(name[end+1:], "."), fieldOff, true
}

// Link binds the variables of an expression.
func Link(s *Script, env Env) (*Linked, *Error) {
	toFloat := env.ToFloat
	params := env.Params
	allowValue, allowScore := env.AllowValue, env.AllowScore
	l := &Linked{script: s, binds: map[*exVar]exBinding{}}
	for _, v := range s.vars {
		name := v.name
		switch name {
		case "_score":
			if !allowScore {
				break
			}
			l.binds[v] = func(doc Doc, score float64, value any) float64 { return score }
			continue
		case "_value":
			if allowValue {
				l.binds[v] = func(doc Doc, score float64, value any) float64 { f, _ := toFloat(value); return f }
				continue
			}
		}
		if field, member, foff, ok := parseDocVar(name); ok {
			if env.Fields == nil {
				return nil, &Error{Link: true, Reason: "Unknown variable [doc]", Offset: v.pos}
			}
			fk, found := env.Fields(field)
			if !found {
				return nil, &Error{Link: true, Reason: "Field [" + field + "] does not exist in mappings", Offset: v.pos + foff}
			}
			kind := ""
			switch fk {
			case KindDate:
				kind = "date"
			case KindNumber:
				kind = "num"
			case KindGeo:
				kind = "geo"
			default:
				return nil, &Error{Link: true, Reason: "Field [" + field + "] must be numeric, date, or geopoint", Offset: v.pos + foff}
			}
			b, err := exDocBinding(field, kind, member, v.pos, toFloat)
			if err != nil {
				return nil, err
			}
			l.binds[v] = b
			continue
		}
		base := name
		if i := strings.IndexAny(base, ".["); i >= 0 {
			base = base[:i]
		}
		if pv, ok := params[base]; ok && base == name {
			f, isNum := toFloat(pv)
			if _, isBool := pv.(bool); isBool || !isNum {
				if pv == nil {
					return nil, &Error{Link: true, Reason: "Parameter [" + base + "] must be a numeric type", Offset: v.pos}
				}
				if _, isStr := pv.(string); isStr || isBool {
					return nil, &Error{Link: true, Reason: "Parameter [" + base + "] must be a numeric type", Offset: v.pos}
				}
			}
			l.binds[v] = func(doc Doc, score float64, value any) float64 { return f }
			continue
		}
		return nil, &Error{Link: true, Reason: "Unknown variable [" + base + "]", Offset: v.pos}
	}
	return l, nil
}

func exNumericValues(doc Doc, field, kind string, toFloat func(any) (float64, bool)) []float64 {
	vals := doc.Values(field)
	out := []float64{}
	for _, v := range vals {
		switch t := v.(type) {
		case bool:
			if t {
				out = append(out, 1)
			} else {
				out = append(out, 0)
			}
		default:
			if f, ok := toFloat(v); ok {
				out = append(out, f)
			}
		}
	}
	sort.Float64s(out)
	return out
}

func exDocBinding(field, kind, member string, pos int, toFloat func(any) (float64, bool)) (exBinding, *Error) {
	agg := func(f func([]float64) float64) exBinding {
		return func(doc Doc, score float64, value any) float64 {
			vals := exNumericValues(doc, field, kind, toFloat)
			if len(vals) == 0 {
				return 0
			}
			return f(vals)
		}
	}
	first := func(v []float64) float64 { return v[0] }
	if kind == "geo" {
		switch member {
		case "lat", "lon":
			return func(doc Doc, score float64, value any) float64 {
				vals := doc.Values(field)
				if len(vals) == 0 {
					return 0
				}
				m, _ := vals[0].(map[string]any)
				f, _ := toFloat(m[member])
				return f
			}, nil
		case "empty":
			return func(doc Doc, score float64, value any) float64 {
				vals := doc.Values(field)
				if len(vals) == 0 {
					return 1
				}
				return 0
			}, nil
		}
		return nil, &Error{Link: true, Reason: "Member variable [" + member + "] does not exist for geo field [" + field + "].", Offset: pos}
	}
	switch member {
	case "", "value":
		return agg(first), nil
	case "empty":
		return func(doc Doc, score float64, value any) float64 {
			if len(exNumericValues(doc, field, kind, toFloat)) == 0 {
				return 1
			}
			return 0
		}, nil
	case "length", "count()":
		return func(doc Doc, score float64, value any) float64 {
			return float64(len(exNumericValues(doc, field, kind, toFloat)))
		}, nil
	case "min()":
		return agg(first), nil
	case "max()":
		return agg(func(v []float64) float64 { return v[len(v)-1] }), nil
	case "sum()":
		return agg(func(v []float64) float64 {
			s := 0.0
			for _, x := range v {
				s += x
			}
			return s
		}), nil
	case "avg()":
		return agg(func(v []float64) float64 {
			s := 0.0
			for _, x := range v {
				s += x
			}
			return s / float64(len(v))
		}), nil
	case "median()":
		return agg(func(v []float64) float64 {
			n := len(v)
			if n%2 == 1 {
				return v[n/2]
			}
			return (v[n/2-1] + v[n/2]) / 2
		}), nil
	}
	if kind == "date" {
		part := strings.TrimPrefix(member, "date.")
		part = strings.TrimSuffix(strings.TrimPrefix(part, "get"), "()")
		if part != "" {
			part = strings.ToLower(part[:1]) + part[1:]
		}
		extract := map[string]func(ms int64) float64{
			"year":           func(ms int64) float64 { return float64(painless.TimeFromMillis(ms).Year()) },
			"month":          func(ms int64) float64 { return float64(painless.TimeFromMillis(ms).Month()) - 1 },
			"monthOfYear":    func(ms int64) float64 { return float64(painless.TimeFromMillis(ms).Month()) },
			"dayOfMonth":     func(ms int64) float64 { return float64(painless.TimeFromMillis(ms).Day()) },
			"dayOfYear":      func(ms int64) float64 { return float64(painless.TimeFromMillis(ms).YearDay()) },
			"dayOfWeek":      func(ms int64) float64 { return float64((int(painless.TimeFromMillis(ms).Weekday())+6)%7 + 1) },
			"hourOfDay":      func(ms int64) float64 { return float64(painless.TimeFromMillis(ms).Hour()) },
			"minutes":        func(ms int64) float64 { return float64(painless.TimeFromMillis(ms).Minute()) },
			"minuteOfHour":   func(ms int64) float64 { return float64(painless.TimeFromMillis(ms).Minute()) },
			"seconds":        func(ms int64) float64 { return float64(painless.TimeFromMillis(ms).Second()) },
			"secondOfMinute": func(ms int64) float64 { return float64(painless.TimeFromMillis(ms).Second()) },
			"millis":         func(ms int64) float64 { return float64(ms) },
		}
		if strings.HasPrefix(member, "date.") || strings.HasPrefix(member, "get") {
			if fn, ok := extract[part]; ok {
				return agg(func(v []float64) float64 { return fn(int64(v[0])) }), nil
			}
		}
	}
	return nil, &Error{Link: true, Reason: "Member method [" + member + "] does not exist for numeric field [" + field + "].", Offset: pos}
}

// Eval computes the expression for one document (doc may be nil when no
// doc variables are bound).
func (l *Linked) Eval(doc Doc, score float64, value any) float64 {
	var ev func(n exNode) float64
	b2f := func(b bool) float64 {
		if b {
			return 1
		}
		return 0
	}
	ev = func(n exNode) float64 {
		switch t := n.(type) {
		case *exNum:
			return t.v
		case *exVar:
			return l.binds[t](doc, score, value)
		case *exUnary:
			x := ev(t.x)
			switch t.op {
			case "-":
				return -x
			case "!":
				return b2f(x == 0)
			case "~":
				return float64(^int64(x))
			}
		case *exCond:
			if ev(t.c) != 0 {
				return ev(t.a)
			}
			return ev(t.b)
		case *exBinary:
			if t.op == "&&" {
				return b2f(ev(t.l) != 0 && ev(t.r) != 0)
			}
			if t.op == "||" {
				return b2f(ev(t.l) != 0 || ev(t.r) != 0)
			}
			a, b := ev(t.l), ev(t.r)
			switch t.op {
			case "+":
				return a + b
			case "-":
				return a - b
			case "*":
				return a * b
			case "/":
				return a / b
			case "%":
				return math.Mod(a, b)
			case "==":
				return b2f(a == b)
			case "!=":
				return b2f(a != b)
			case "<":
				return b2f(a < b)
			case "<=":
				return b2f(a <= b)
			case ">":
				return b2f(a > b)
			case ">=":
				return b2f(a >= b)
			case "&":
				return float64(int64(a) & int64(b))
			case "|":
				return float64(int64(a) | int64(b))
			case "^":
				return float64(int64(a) ^ int64(b))
			case "<<":
				return float64(int64(a) << uint(int64(b)&63))
			case ">>":
				return float64(int64(a) >> uint(int64(b)&63))
			case ">>>":
				return float64(uint64(int64(a)) >> uint(int64(b)&63))
			}
		case *exCall:
			args := make([]float64, len(t.args))
			for i, a := range t.args {
				args[i] = ev(a)
			}
			return exFunc(t.fn, args)
		}
		return 0
	}
	return ev(l.script.root)
}

var exFuncArity = map[string]int{
	"abs": 1, "acos": 1, "acosh": 1, "asin": 1, "asinh": 1, "atan": 1, "atan2": 2, "atanh": 1,
	"ceil": 1, "cos": 1, "cosh": 1, "exp": 1, "floor": 1, "haversin": 4, "ln": 1, "log10": 1,
	"logn": 2, "max": 2, "min": 2, "pow": 2, "sin": 1, "sinh": 1, "sqrt": 1, "tan": 1, "tanh": 1,
}

func exFunc(fn string, a []float64) float64 {
	switch fn {
	case "abs":
		return math.Abs(a[0])
	case "acos":
		return math.Acos(a[0])
	case "acosh":
		return math.Acosh(a[0])
	case "asin":
		return math.Asin(a[0])
	case "asinh":
		return math.Asinh(a[0])
	case "atan":
		return math.Atan(a[0])
	case "atan2":
		return math.Atan2(a[0], a[1])
	case "atanh":
		return math.Atanh(a[0])
	case "ceil":
		return math.Ceil(a[0])
	case "cos":
		return math.Cos(a[0])
	case "cosh":
		return math.Cosh(a[0])
	case "exp":
		return math.Exp(a[0])
	case "floor":
		return math.Floor(a[0])
	case "ln":
		return math.Log(a[0])
	case "log10":
		return math.Log10(a[0])
	case "logn":
		return math.Log(a[1]) / math.Log(a[0])
	case "max":
		return math.Max(a[0], a[1])
	case "min":
		return math.Min(a[0], a[1])
	case "pow":
		return math.Pow(a[0], a[1])
	case "sin":
		return math.Sin(a[0])
	case "sinh":
		return math.Sinh(a[0])
	case "sqrt":
		return math.Sqrt(a[0])
	case "tan":
		return math.Tan(a[0])
	case "tanh":
		return math.Tanh(a[0])
	case "haversin":
		lat1, lon1, lat2, lon2 := a[0]*math.Pi/180, a[1]*math.Pi/180, a[2]*math.Pi/180, a[3]*math.Pi/180
		h := math.Pow(math.Sin((lat2-lat1)/2), 2) + math.Cos(lat1)*math.Cos(lat2)*math.Pow(math.Sin((lon2-lon1)/2), 2)
		return 2 * 6371.0087714 * math.Asin(math.Sqrt(h))
	}
	return 0
}

// exCheckFuncs validates function names and arities at compile time.
func exCheckFuncs(n exNode, src string) *Error {
	switch t := n.(type) {
	case *exCall:
		ar, ok := exFuncArity[t.fn]
		if !ok {
			return &Error{Reason: "Invalid expression '" + src + "': Unrecognized function call (" + t.fn + ").", Offset: t.pos}
		}
		if ar != len(t.args) {
			return &Error{Reason: "Invalid expression '" + src + "': Expected (" + strconv.Itoa(ar) + ") arguments for function call (" + t.fn + "), but found (" + strconv.Itoa(len(t.args)) + ").", Offset: t.pos}
		}
		for _, a := range t.args {
			if err := exCheckFuncs(a, src); err != nil {
				return err
			}
		}
	case *exUnary:
		return exCheckFuncs(t.x, src)
	case *exBinary:
		if err := exCheckFuncs(t.l, src); err != nil {
			return err
		}
		return exCheckFuncs(t.r, src)
	case *exCond:
		for _, x := range []exNode{t.c, t.a, t.b} {
			if err := exCheckFuncs(x, src); err != nil {
				return err
			}
		}
	}
	return nil
}

func isHexByte(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// Compile parses an expression and checks its function calls.
func Compile(src string) (*Script, *Error) {
	s, err := parse(src)
	if err == nil {
		err = exCheckFuncs(s.root, src)
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}
