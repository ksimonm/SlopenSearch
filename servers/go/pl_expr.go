package main

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// Lucene expressions ("lang": "expression"): a JavaScript-like numeric
// language. Everything is a double; variables are bound at link time to doc
// values, _score, _value or numeric params.

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

type exScript struct {
	src  string
	root exNode
	vars []*exVar
}

type exErr struct {
	link   bool
	reason string
	offset int
	eof    bool
}

func (e *exErr) scriptErr(src string) *plScriptErr {
	se := &plScriptErr{compile: !e.link, script: src, lang: "expression", causeType: "parse_exception", reason: e.reason}
	if e.link {
		se.linkErr = true
	}
	if e.eof {
		se.inner = &plCompileErr{typ: "no_viable_alt_exception", nilReason: true}
	}
	off := e.offset
	if off > len(src) {
		off = len(src)
	}
	se.stack = []string{src, strings.Repeat(" ", off) + "^---- HERE"}
	return se
}

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

func (p *exParser) errHere() *exErr {
	p.skip()
	if p.i >= len(p.src) {
		return &exErr{reason: "unexpected end of expression", offset: len(p.src), eof: true}
	}
	return &exErr{reason: "unexpected character '" + string(p.src[p.i]) + "' on line (1) position (" + itoa(p.i) + ")", offset: p.i}
}

func exCompile(src string) (*exScript, *exErr) {
	p := &exParser{src: src}
	n, err := p.cond()
	if err != nil {
		return nil, err
	}
	p.skip()
	if p.i < len(p.src) {
		return nil, p.errHere()
	}
	return &exScript{src: src, root: n, vars: p.vars}, nil
}

func (p *exParser) cond() (exNode, *exErr) {
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

func (p *exParser) binary(level int) (exNode, *exErr) {
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

func (p *exParser) unary() (exNode, *exErr) {
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

func (p *exParser) primary() (exNode, *exErr) {
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
			return nil, &exErr{reason: "unexpected character '" + text + "'", offset: p.i}
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

// exBinding resolves one variable for a document.
type exBinding func(qc *queryCtx, score float64, value any) float64

type exLinked struct {
	script *exScript
	binds  map[*exVar]exBinding
}

func plParseDocVar(name string) (field, member string, fieldOff int, ok bool) {
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

// exLink binds the variables of an expression for an index and context.
// params holds numeric parameters (or bucket path values).
func exLink(s *exScript, idx *Index, params map[string]any, allowValue, allowScore bool) (*exLinked, *exErr) {
	l := &exLinked{script: s, binds: map[*exVar]exBinding{}}
	for _, v := range s.vars {
		name := v.name
		switch name {
		case "_score":
			if !allowScore {
				break
			}
			l.binds[v] = func(qc *queryCtx, score float64, value any) float64 { return score }
			continue
		case "_value":
			if allowValue {
				l.binds[v] = func(qc *queryCtx, score float64, value any) float64 { f, _ := toFloat(value); return f }
				continue
			}
		}
		if field, member, foff, ok := plParseDocVar(name); ok {
			if idx == nil {
				return nil, &exErr{link: true, reason: "Unknown variable [doc]", offset: v.pos}
			}
			def, found := mappingFieldDef(idx, field)
			if !found {
				return nil, &exErr{link: true, reason: "Field [" + field + "] does not exist in mappings", offset: v.pos + foff}
			}
			ft, _ := def["type"].(string)
			kind := ""
			switch {
			case ft == "date" || ft == "date_nanos":
				kind = "date"
			case isNumericType(ft) || ft == "boolean":
				kind = "num"
			case ft == "geo_point":
				kind = "geo"
			default:
				return nil, &exErr{link: true, reason: "Field [" + field + "] must be numeric, date, or geopoint", offset: v.pos + foff}
			}
			b, err := exDocBinding(field, kind, member, v.pos)
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
					return nil, &exErr{link: true, reason: "Parameter [" + base + "] must be a numeric type", offset: v.pos}
				}
				if _, isStr := pv.(string); isStr || isBool {
					return nil, &exErr{link: true, reason: "Parameter [" + base + "] must be a numeric type", offset: v.pos}
				}
			}
			l.binds[v] = func(qc *queryCtx, score float64, value any) float64 { return f }
			continue
		}
		return nil, &exErr{link: true, reason: "Unknown variable [" + base + "]", offset: v.pos}
	}
	return l, nil
}

func exNumericValues(qc *queryCtx, field, kind string) []float64 {
	vals, _ := qc.values(field)
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

func exDocBinding(field, kind, member string, pos int) (exBinding, *exErr) {
	agg := func(f func([]float64) float64) exBinding {
		return func(qc *queryCtx, score float64, value any) float64 {
			vals := exNumericValues(qc, field, kind)
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
			return func(qc *queryCtx, score float64, value any) float64 {
				vals, _ := qc.values(field)
				if len(vals) == 0 {
					return 0
				}
				m, _ := vals[0].(map[string]any)
				f, _ := toFloat(m[member])
				return f
			}, nil
		case "empty":
			return func(qc *queryCtx, score float64, value any) float64 {
				vals, _ := qc.values(field)
				if len(vals) == 0 {
					return 1
				}
				return 0
			}, nil
		}
		return nil, &exErr{link: true, reason: "Member variable [" + member + "] does not exist for geo field [" + field + "].", offset: pos}
	}
	switch member {
	case "", "value":
		return agg(first), nil
	case "empty":
		return func(qc *queryCtx, score float64, value any) float64 {
			if len(exNumericValues(qc, field, kind)) == 0 {
				return 1
			}
			return 0
		}, nil
	case "length", "count()":
		return func(qc *queryCtx, score float64, value any) float64 {
			return float64(len(exNumericValues(qc, field, kind)))
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
			"year":           func(ms int64) float64 { return float64(plTimeFromMillis(ms).Year()) },
			"month":          func(ms int64) float64 { return float64(plTimeFromMillis(ms).Month()) - 1 },
			"monthOfYear":    func(ms int64) float64 { return float64(plTimeFromMillis(ms).Month()) },
			"dayOfMonth":     func(ms int64) float64 { return float64(plTimeFromMillis(ms).Day()) },
			"dayOfYear":      func(ms int64) float64 { return float64(plTimeFromMillis(ms).YearDay()) },
			"dayOfWeek":      func(ms int64) float64 { return float64((int(plTimeFromMillis(ms).Weekday())+6)%7 + 1) },
			"hourOfDay":      func(ms int64) float64 { return float64(plTimeFromMillis(ms).Hour()) },
			"minutes":        func(ms int64) float64 { return float64(plTimeFromMillis(ms).Minute()) },
			"minuteOfHour":   func(ms int64) float64 { return float64(plTimeFromMillis(ms).Minute()) },
			"seconds":        func(ms int64) float64 { return float64(plTimeFromMillis(ms).Second()) },
			"secondOfMinute": func(ms int64) float64 { return float64(plTimeFromMillis(ms).Second()) },
			"millis":         func(ms int64) float64 { return float64(ms) },
		}
		if strings.HasPrefix(member, "date.") || strings.HasPrefix(member, "get") {
			if fn, ok := extract[part]; ok {
				return agg(func(v []float64) float64 { return fn(int64(v[0])) }), nil
			}
		}
	}
	return nil, &exErr{link: true, reason: "Member method [" + member + "] does not exist for numeric field [" + field + "].", offset: pos}
}

// eval computes the expression for one document.
func (l *exLinked) eval(qc *queryCtx, score float64, value any) float64 {
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
			return l.binds[t](qc, score, value)
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
func exCheckFuncs(n exNode, src string) *exErr {
	switch t := n.(type) {
	case *exCall:
		ar, ok := exFuncArity[t.fn]
		if !ok {
			return &exErr{reason: "Invalid expression '" + src + "': Unrecognized function call (" + t.fn + ").", offset: t.pos}
		}
		if ar != len(t.args) {
			return &exErr{reason: "Invalid expression '" + src + "': Expected (" + itoa(ar) + ") arguments for function call (" + t.fn + "), but found (" + itoa(len(t.args)) + ").", offset: t.pos}
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

// exPrepare compiles and checks an expression for a context.
func exPrepare(src, ctxName string) (*exScript, *plScriptErr, *apiError) {
	s, err := exCompile(src)
	if err == nil {
		err = exCheckFuncs(s.root, src)
	}
	if err != nil {
		return nil, err.scriptErr(src), nil
	}
	if !expressionContexts[ctxName] {
		return nil, nil, errBad("expression engine does not know how to handle script context [" + ctxName + "]")
	}
	return s, nil, nil
}
