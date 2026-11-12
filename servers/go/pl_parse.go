package main

import (
	"strings"
)

// Painless engine: AST and recursive-descent parser.

type plNode interface{ nodePos() int }

type plBase struct{ pos int }

func (b plBase) nodePos() int { return b.pos }

// ---- statements ----

type plSBlock struct {
	plBase
	stmts []plNode
}
type plSDecl struct {
	plBase
	typ   string
	names []string
	inits []plNode // nil entries for declarations without initializer
	npos  []int
}
type plSExpr struct {
	plBase
	x plNode
}
type plSIf struct {
	plBase
	cond      plNode
	then, els plNode
}
type plSWhile struct {
	plBase
	cond plNode
	body plNode
}
type plSDo struct {
	plBase
	body plNode
	cond plNode
}
type plSFor struct {
	plBase
	init   plNode
	cond   plNode
	update []plNode
	body   plNode
}
type plSForEach struct {
	plBase
	typ  string
	name string
	iter plNode
	body plNode
}
type plSBreak struct{ plBase }
type plSContinue struct{ plBase }
type plSReturn struct {
	plBase
	x plNode
}
type plSThrow struct {
	plBase
	x plNode
}
type plCatch struct {
	typ  string
	name string
	body *plSBlock
}
type plSTry struct {
	plBase
	body    *plSBlock
	catches []plCatch
}

type plFunc struct {
	plBase
	ret    string
	name   string
	ptypes []string
	pnames []string
	body   *plSBlock
}

// ---- expressions ----

type plELit struct {
	plBase
	val any
	typ string
}
type plERegex struct {
	plBase
	pattern string
	flags   string
}
type plEVar struct {
	plBase
	name string
}
type plEType struct { // a type used as a value: static access (Math.max)
	plBase
	name string
}
type plEField struct {
	plBase
	obj      plNode
	name     string
	nullSafe bool
}
type plECall struct {
	plBase
	obj      plNode // nil for local function calls
	name     string
	args     []plNode
	nullSafe bool
}
type plEIndex struct {
	plBase
	obj plNode
	idx plNode
}
type plENew struct {
	plBase
	typ  string
	args []plNode
}
type plENewArray struct {
	plBase
	elem    string // element type, e.g. int
	dims    []plNode
	init    []plNode
	rank    int
	hasInit bool
}
type plEList struct {
	plBase
	elems []plNode
}
type plEMap struct {
	plBase
	keys, vals []plNode
}
type plEUnary struct {
	plBase
	op string
	x  plNode
}
type plEBinary struct {
	plBase
	op   string
	l, r plNode
	opos int
}
type plEInstanceof struct {
	plBase
	x   plNode
	typ string
}
type plECast struct {
	plBase
	typ string
	x   plNode
}
type plECond struct {
	plBase
	c, a, b plNode
}
type plEElvis struct {
	plBase
	a, b plNode
}
type plEAssign struct {
	plBase
	target plNode
	op     string // "=" or binary op of a compound assignment
	val    plNode
}
type plEIncDec struct {
	plBase
	target plNode
	op     string // "++" or "--"
	prefix bool
}
type plELambda struct {
	plBase
	ptypes []string
	pnames []string
	body   plNode // expression or *plSBlock
}
type plEFuncRef struct {
	plBase
	typ    string
	method string
}

// ---- parser ----

type plParser struct {
	src   string
	toks  []plTok
	i     int
	stmts []int // start offsets of every statement, for runtime error spans
}

type plProgram struct {
	src   string
	funcs map[string]*plFunc
	body  *plSBlock
	stmts []int
}

func plParse(src string) (*plProgram, *plCompileErr) {
	toks, lerr := plLex(src)
	if lerr != nil {
		return nil, lerr
	}
	p := &plParser{src: src, toks: toks}
	prog := &plProgram{src: src, funcs: map[string]*plFunc{}}
	body := &plSBlock{}
	// functions come first: "type name ( ... ) {"
	for p.isFuncStart() {
		f, err := p.parseFunc()
		if err != nil {
			return nil, err
		}
		prog.funcs[f.name] = f
	}
	for p.peek().kind != plTEOF {
		s, err := p.parseStatement(true)
		if err != nil {
			return nil, err
		}
		body.stmts = append(body.stmts, s)
	}
	prog.body = body
	prog.stmts = p.stmts
	return prog, nil
}

func (p *plParser) peek() plTok { return p.toks[p.i] }
func (p *plParser) peekAt(k int) plTok {
	if p.i+k < len(p.toks) {
		return p.toks[p.i+k]
	}
	return p.toks[len(p.toks)-1]
}
func (p *plParser) next() plTok {
	t := p.toks[p.i]
	if p.i < len(p.toks)-1 {
		p.i++
	}
	return t
}
func (p *plParser) isOp(s string) bool {
	t := p.peek()
	return t.kind == plTOp && t.text == s
}
func (p *plParser) acceptOp(s string) bool {
	if p.isOp(s) {
		p.next()
		return true
	}
	return false
}

func (p *plParser) tokText(t plTok) string {
	if t.kind == plTEOF {
		return "<EOF>"
	}
	return p.src[t.pos:t.end]
}

func (p *plParser) errUnexpected(expecting string) *plCompileErr {
	t := p.peek()
	if t.kind == plTEOF {
		return &plCompileErr{typ: "illegal_argument_exception", reason: "unexpected end of script.",
			offset: t.pos, cause: &plCompileErr{typ: "no_viable_alt_exception", nilReason: true}}
	}
	reason := "unexpected token ['" + p.tokText(t) + "']"
	if expecting != "" {
		reason += " was expecting one of [" + expecting + "]."
	} else {
		reason += "."
	}
	return &plCompileErr{typ: "illegal_argument_exception", reason: reason, offset: t.pos}
}

func (p *plParser) expectOp(s string) *plCompileErr {
	if p.acceptOp(s) {
		return nil
	}
	if p.peek().kind == plTEOF {
		return p.errUnexpected("")
	}
	return p.errUnexpected("'" + s + "'")
}

// isFuncStart: TYPE ID '(' at top level.
func (p *plParser) isFuncStart() bool {
	t := p.peek()
	if t.kind != plTType {
		return false
	}
	k := 1
	for p.peekAt(k).kind == plTOp && p.peekAt(k).text == "[" && p.peekAt(k+1).kind == plTOp && p.peekAt(k+1).text == "]" {
		k += 2
	}
	return p.peekAt(k).kind == plTIdent && p.peekAt(k+1).kind == plTOp && p.peekAt(k+1).text == "("
}

func (p *plParser) parseTypeName() string {
	t := p.next()
	name := t.text
	for p.isOp("[") && p.peekAt(1).kind == plTOp && p.peekAt(1).text == "]" {
		p.next()
		p.next()
		name += "[]"
	}
	return name
}

func (p *plParser) parseFunc() (*plFunc, *plCompileErr) {
	pos := p.peek().pos
	ret := p.parseTypeName()
	name := p.next().text
	p.next() // (
	f := &plFunc{plBase: plBase{pos}, ret: ret, name: name}
	if !p.isOp(")") {
		for {
			if p.peek().kind != plTType {
				return nil, p.errUnexpected("")
			}
			pt := p.parseTypeName()
			if p.peek().kind != plTIdent {
				return nil, p.errUnexpected("")
			}
			f.ptypes = append(f.ptypes, pt)
			f.pnames = append(f.pnames, p.next().text)
			if !p.acceptOp(",") {
				break
			}
		}
	}
	if err := p.expectOp(")"); err != nil {
		return nil, err
	}
	if !p.isOp("{") {
		return nil, p.errUnexpected("'{'")
	}
	b, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	f.body = b
	return f, nil
}

func (p *plParser) parseBlock() (*plSBlock, *plCompileErr) {
	pos := p.next().pos // {
	b := &plSBlock{plBase: plBase{pos}}
	for !p.isOp("}") {
		if p.peek().kind == plTEOF {
			return nil, p.errUnexpected("")
		}
		s, err := p.parseStatement(false)
		if err != nil {
			return nil, err
		}
		b.stmts = append(b.stmts, s)
	}
	p.next()
	return b, nil
}

// endStatement consumes the terminator of a simple statement: ';', or
// nothing before '}' / EOF.
func (p *plParser) endStatement(top bool) *plCompileErr {
	if p.acceptOp(";") {
		return nil
	}
	t := p.peek()
	if t.kind == plTEOF {
		return nil
	}
	if !top && t.kind == plTOp && t.text == "}" {
		return nil
	}
	if top {
		return p.errUnexpected("{<EOF>, ';'}")
	}
	return p.errUnexpected("{';', '}'}")
}

// parseTrailer parses the body of if/while/for: a block or one statement.
func (p *plParser) parseTrailer() (plNode, *plCompileErr) {
	if p.isOp("{") {
		return p.parseBlock()
	}
	return p.parseStatement(false)
}

func (p *plParser) isDeclStart() bool {
	t := p.peek()
	if t.kind != plTType {
		return false
	}
	k := 1
	for p.peekAt(k).kind == plTOp && p.peekAt(k).text == "[" && p.peekAt(k+1).kind == plTOp && p.peekAt(k+1).text == "]" {
		k += 2
	}
	return p.peekAt(k).kind == plTIdent
}

func (p *plParser) parseStatement(top bool) (plNode, *plCompileErr) {
	t := p.peek()
	p.stmts = append(p.stmts, t.pos)
	if t.kind == plTIdent {
		switch t.text {
		case "if":
			p.next()
			if err := p.expectOp("("); err != nil {
				return nil, err
			}
			c, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.expectOp(")"); err != nil {
				return nil, err
			}
			th, err := p.parseTrailer()
			if err != nil {
				return nil, err
			}
			s := &plSIf{plBase: plBase{t.pos}, cond: c, then: th}
			if p.peek().kind == plTIdent && p.peek().text == "else" {
				p.next()
				el, err := p.parseTrailer()
				if err != nil {
					return nil, err
				}
				s.els = el
			}
			return s, nil
		case "while":
			p.next()
			if err := p.expectOp("("); err != nil {
				return nil, err
			}
			c, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.expectOp(")"); err != nil {
				return nil, err
			}
			if p.acceptOp(";") {
				return &plSWhile{plBase: plBase{t.pos}, cond: c, body: &plSBlock{}}, nil
			}
			b, err := p.parseTrailer()
			if err != nil {
				return nil, err
			}
			return &plSWhile{plBase: plBase{t.pos}, cond: c, body: b}, nil
		case "do":
			p.next()
			if !p.isOp("{") {
				return nil, p.errUnexpected("'{'")
			}
			b, err := p.parseBlock()
			if err != nil {
				return nil, err
			}
			if !(p.peek().kind == plTIdent && p.peek().text == "while") {
				return nil, p.errUnexpected("'while'")
			}
			p.next()
			if err := p.expectOp("("); err != nil {
				return nil, err
			}
			c, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.expectOp(")"); err != nil {
				return nil, err
			}
			if err := p.endStatement(top); err != nil {
				return nil, err
			}
			return &plSDo{plBase: plBase{t.pos}, body: b, cond: c}, nil
		case "for":
			return p.parseFor(t)
		case "break":
			p.next()
			if err := p.endStatement(top); err != nil {
				return nil, err
			}
			return &plSBreak{plBase{t.pos}}, nil
		case "continue":
			p.next()
			if err := p.endStatement(top); err != nil {
				return nil, err
			}
			return &plSContinue{plBase{t.pos}}, nil
		case "return":
			p.next()
			s := &plSReturn{plBase: plBase{t.pos}}
			if !p.isOp(";") && !p.isOp("}") && p.peek().kind != plTEOF {
				x, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				s.x = x
			}
			if err := p.endStatement(top); err != nil {
				return nil, err
			}
			return s, nil
		case "throw":
			p.next()
			x, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.endStatement(top); err != nil {
				return nil, err
			}
			return &plSThrow{plBase: plBase{t.pos}, x: x}, nil
		case "try":
			p.next()
			if !p.isOp("{") {
				return nil, p.errUnexpected("'{'")
			}
			b, err := p.parseBlock()
			if err != nil {
				return nil, err
			}
			s := &plSTry{plBase: plBase{t.pos}, body: b}
			for p.peek().kind == plTIdent && p.peek().text == "catch" {
				p.next()
				if err := p.expectOp("("); err != nil {
					return nil, err
				}
				ct := p.parseTypeName()
				cn := p.next().text
				if err := p.expectOp(")"); err != nil {
					return nil, err
				}
				cb, err := p.parseBlock()
				if err != nil {
					return nil, err
				}
				s.catches = append(s.catches, plCatch{typ: ct, name: cn, body: cb})
			}
			return s, nil
		}
	}
	if t.kind == plTOp && t.text == "{" {
		return p.parseBlock()
	}
	if t.kind == plTOp && t.text == ";" {
		p.next()
		return &plSBlock{plBase: plBase{t.pos}}, nil
	}
	if p.isDeclStart() {
		d, err := p.parseDecl()
		if err != nil {
			return nil, err
		}
		if err := p.endStatement(top); err != nil {
			return nil, err
		}
		return d, nil
	}
	x, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if err := p.endStatement(top); err != nil {
		return nil, err
	}
	return &plSExpr{plBase: plBase{t.pos}, x: x}, nil
}

func (p *plParser) parseDecl() (*plSDecl, *plCompileErr) {
	pos := p.peek().pos
	typ := p.parseTypeName()
	d := &plSDecl{plBase: plBase{pos}, typ: typ}
	for {
		nt := p.peek()
		if nt.kind != plTIdent {
			return nil, p.errUnexpected("")
		}
		p.next()
		d.names = append(d.names, nt.text)
		d.npos = append(d.npos, nt.pos)
		if p.acceptOp("=") {
			x, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			d.inits = append(d.inits, x)
		} else {
			d.inits = append(d.inits, nil)
		}
		if !p.acceptOp(",") {
			break
		}
	}
	return d, nil
}

func (p *plParser) parseFor(t plTok) (plNode, *plCompileErr) {
	p.next()
	if err := p.expectOp("("); err != nil {
		return nil, err
	}
	// for (x in expr) / for (def x : expr) / for (Type x : expr)
	if p.peek().kind == plTIdent && p.peekAt(1).kind == plTIdent && p.peekAt(1).text == "in" {
		name := p.next().text
		p.next()
		it, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if err := p.expectOp(")"); err != nil {
			return nil, err
		}
		b, err := p.parseTrailer()
		if err != nil {
			return nil, err
		}
		return &plSForEach{plBase: plBase{t.pos}, typ: "def", name: name, iter: it, body: b}, nil
	}
	if p.isDeclStart() {
		save := p.i
		typ := p.parseTypeName()
		if p.peek().kind == plTIdent && p.peekAt(1).kind == plTOp && p.peekAt(1).text == ":" {
			name := p.next().text
			p.next()
			it, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.expectOp(")"); err != nil {
				return nil, err
			}
			b, err := p.parseTrailer()
			if err != nil {
				return nil, err
			}
			return &plSForEach{plBase: plBase{t.pos}, typ: typ, name: name, iter: it, body: b}, nil
		}
		p.i = save
	}
	s := &plSFor{plBase: plBase{t.pos}}
	if !p.isOp(";") {
		if p.isDeclStart() {
			d, err := p.parseDecl()
			if err != nil {
				return nil, err
			}
			s.init = d
		} else {
			x, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			s.init = &plSExpr{plBase: plBase{x.nodePos()}, x: x}
		}
	}
	if err := p.expectOp(";"); err != nil {
		return nil, err
	}
	if !p.isOp(";") {
		c, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		s.cond = c
	}
	if err := p.expectOp(";"); err != nil {
		return nil, err
	}
	if !p.isOp(")") {
		for {
			x, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			s.update = append(s.update, x)
			if !p.acceptOp(",") {
				break
			}
		}
	}
	if err := p.expectOp(")"); err != nil {
		return nil, err
	}
	if p.acceptOp(";") {
		s.body = &plSBlock{}
		return s, nil
	}
	b, err := p.parseTrailer()
	if err != nil {
		return nil, err
	}
	s.body = b
	return s, nil
}

// ---- expressions ----

var plAssignOps = map[string]string{
	"=": "=", "+=": "+", "-=": "-", "*=": "*", "/=": "/", "%=": "%",
	"&=": "&", "|=": "|", "^=": "^", "<<=": "<<", ">>=": ">>", ">>>=": ">>>",
}

func (p *plParser) parseExpr() (plNode, *plCompileErr) {
	pos := p.peek().pos
	lhs, err := p.parseBinary(0)
	if err != nil {
		return nil, err
	}
	t := p.peek()
	if t.kind == plTOp {
		if op, ok := plAssignOps[t.text]; ok {
			p.next()
			rhs, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			return &plEAssign{plBase: plBase{pos}, target: lhs, op: op, val: rhs}, nil
		}
		if t.text == "?" {
			p.next()
			a, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.expectOp(":"); err != nil {
				return nil, err
			}
			b, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			return &plECond{plBase: plBase{pos}, c: lhs, a: a, b: b}, nil
		}
	}
	return lhs, nil
}

// binary precedence levels, lowest first.
var plBinLevels = [][]string{
	{"?:"},
	{"||"},
	{"&&"},
	{"|"},
	{"^"},
	{"&"},
	{"==", "!=", "===", "!=="},
	{"<", "<=", ">", ">=", "instanceof"},
	{"<<", ">>", ">>>"},
	{"=~", "==~"},
	{"+", "-"},
	{"*", "/", "%"},
}

func (p *plParser) parseBinary(level int) (plNode, *plCompileErr) {
	if level >= len(plBinLevels) {
		return p.parseUnary()
	}
	pos := p.peek().pos
	if plBinLevels[level][0] == "?:" {
		lhs, err := p.parseBinary(level + 1)
		if err != nil {
			return nil, err
		}
		if p.isOp("?:") {
			p.next()
			rhs, err := p.parseBinary(level) // right assoc
			if err != nil {
				return nil, err
			}
			return &plEElvis{plBase: plBase{pos}, a: lhs, b: rhs}, nil
		}
		return lhs, nil
	}
	lhs, err := p.parseBinary(level + 1)
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		op := ""
		if t.kind == plTOp {
			for _, o := range plBinLevels[level] {
				if t.text == o {
					op = o
				}
			}
		} else if t.kind == plTIdent && t.text == "instanceof" {
			for _, o := range plBinLevels[level] {
				if o == "instanceof" {
					op = o
				}
			}
		}
		if op == "" {
			return lhs, nil
		}
		p.next()
		if op == "instanceof" {
			if p.peek().kind != plTType {
				if p.peek().kind == plTIdent {
					return nil, &plCompileErr{typ: "illegal_argument_exception", offset: p.peek().pos,
						reason: "Not a type [" + p.peek().text + "]."}
				}
				return nil, p.errUnexpected("")
			}
			tn := p.parseTypeName()
			lhs = &plEInstanceof{plBase: plBase{pos}, x: lhs, typ: tn}
			continue
		}
		rhs, err := p.parseBinary(level + 1)
		if err != nil {
			return nil, err
		}
		lhs = &plEBinary{plBase: plBase{pos}, op: op, l: lhs, r: rhs, opos: t.pos}
	}
}

// isCastAhead reports whether "( TYPE [[]]* )" starts at the current token.
func (p *plParser) isCastAhead() bool {
	if !p.isOp("(") || p.peekAt(1).kind != plTType {
		return false
	}
	k := 2
	for p.peekAt(k).kind == plTOp && p.peekAt(k).text == "[" && p.peekAt(k+1).kind == plTOp && p.peekAt(k+1).text == "]" {
		k += 2
	}
	if !(p.peekAt(k).kind == plTOp && p.peekAt(k).text == ")") {
		return false
	}
	// "(Type) ..." followed by '.' is a parenthesised static access, not a cast
	nt := p.peekAt(k + 1)
	if nt.kind == plTOp && (nt.text == "." || nt.text == ")" || nt.text == ";" || nt.text == "," || nt.text == "?") {
		return false
	}
	return nt.kind != plTEOF
}

func (p *plParser) parseUnary() (plNode, *plCompileErr) {
	t := p.peek()
	if t.kind == plTOp {
		switch t.text {
		case "++", "--":
			p.next()
			x, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return &plEIncDec{plBase: plBase{t.pos}, target: x, op: t.text, prefix: true}, nil
		case "!", "~", "-", "+":
			p.next()
			// fold negative numeric literals so that -2147483648 is legal
			if t.text == "-" && (p.peek().kind == plTInt || p.peek().kind == plTFloat) {
				nt := p.peek()
				after := p.peekAt(1)
				if !(after.kind == plTOp && (after.text == "." || after.text == "[")) {
					p.next()
					lit, err := plNumLit(nt, true, t.pos)
					if err != nil {
						return nil, err
					}
					return lit, nil
				}
			}
			x, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return &plEUnary{plBase: plBase{t.pos}, op: t.text, x: x}, nil
		case "(":
			if p.isCastAhead() {
				p.next()
				typ := p.parseTypeName()
				p.next() // )
				x, err := p.parseUnary()
				if err != nil {
					return nil, err
				}
				return &plECast{plBase: plBase{t.pos}, typ: typ, x: x}, nil
			}
		}
	}
	return p.parsePostfix()
}

func (p *plParser) parsePostfix() (plNode, *plCompileErr) {
	x, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind != plTOp {
			return x, nil
		}
		switch t.text {
		case ".", "?.":
			p.next()
			nt := p.peek()
			if nt.kind != plTIdent && nt.kind != plTType {
				return nil, p.errUnexpected("")
			}
			p.next()
			if p.isOp("(") {
				args, err := p.parseArgs()
				if err != nil {
					return nil, err
				}
				x = &plECall{plBase: plBase{t.pos}, obj: x, name: nt.text, args: args, nullSafe: t.text == "?."}
			} else {
				x = &plEField{plBase: plBase{t.pos}, obj: x, name: nt.text, nullSafe: t.text == "?."}
			}
		case "[":
			p.next()
			idx, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.expectOp("]"); err != nil {
				return nil, err
			}
			x = &plEIndex{plBase: plBase{t.pos}, obj: x, idx: idx}
		case "++", "--":
			p.next()
			x = &plEIncDec{plBase: plBase{t.pos}, target: x, op: t.text}
		default:
			return x, nil
		}
	}
}

func (p *plParser) parseArgs() ([]plNode, *plCompileErr) {
	p.next() // (
	args := []plNode{}
	if p.acceptOp(")") {
		return args, nil
	}
	for {
		a, err := p.parseArg()
		if err != nil {
			return nil, err
		}
		args = append(args, a)
		if p.acceptOp(",") {
			continue
		}
		if err := p.expectOp(")"); err != nil {
			return nil, err
		}
		return args, nil
	}
}

// parseArg parses a call argument, which may be a lambda or a function reference.
func (p *plParser) parseArg() (plNode, *plCompileErr) {
	t := p.peek()
	// x -> ...
	if t.kind == plTIdent && p.peekAt(1).kind == plTOp && p.peekAt(1).text == "->" {
		p.next()
		p.next()
		body, err := p.parseLambdaBody()
		if err != nil {
			return nil, err
		}
		return &plELambda{plBase: plBase{t.pos}, ptypes: []string{"def"}, pnames: []string{t.text}, body: body}, nil
	}
	// (a, b) -> ... / (int a) -> ... / () -> ...
	if t.kind == plTOp && t.text == "(" {
		k := 1
		ok := true
		for {
			nt := p.peekAt(k)
			if nt.kind == plTOp && nt.text == ")" {
				break
			}
			if nt.kind == plTType {
				k++
				nt = p.peekAt(k)
			}
			if nt.kind != plTIdent {
				ok = false
				break
			}
			k++
			nt = p.peekAt(k)
			if nt.kind == plTOp && nt.text == "," {
				k++
				continue
			}
			if !(nt.kind == plTOp && nt.text == ")") {
				ok = false
			}
			break
		}
		if ok && p.peekAt(k+1).kind == plTOp && p.peekAt(k+1).text == "->" {
			p.next()
			lam := &plELambda{plBase: plBase{t.pos}}
			for !p.isOp(")") {
				typ := "def"
				if p.peek().kind == plTType {
					typ = p.next().text
				}
				lam.ptypes = append(lam.ptypes, typ)
				lam.pnames = append(lam.pnames, p.next().text)
				p.acceptOp(",")
			}
			p.next() // )
			p.next() // ->
			body, err := p.parseLambdaBody()
			if err != nil {
				return nil, err
			}
			lam.body = body
			return lam, nil
		}
	}
	// Type::method / this::fn
	if (t.kind == plTType || t.kind == plTIdent) && p.peekAt(1).kind == plTOp && p.peekAt(1).text == "::" {
		p.next()
		p.next()
		m := p.next()
		return &plEFuncRef{plBase: plBase{t.pos}, typ: t.text, method: m.text}, nil
	}
	return p.parseExpr()
}

func (p *plParser) parseLambdaBody() (plNode, *plCompileErr) {
	if p.isOp("{") {
		return p.parseBlock()
	}
	return p.parseExpr()
}

func plNumLit(t plTok, neg bool, pos int) (plNode, *plCompileErr) {
	v, typ, err := plParseNumber(t.text, t.kind == plTFloat, neg)
	if err != nil {
		return nil, &plCompileErr{typ: "illegal_argument_exception", offset: pos, reason: err.Error()}
	}
	return &plELit{plBase: plBase{pos}, val: v, typ: typ}, nil
}

func (p *plParser) parsePrimary() (plNode, *plCompileErr) {
	t := p.peek()
	switch t.kind {
	case plTEOF:
		return nil, p.errUnexpected("")
	case plTInt, plTFloat:
		p.next()
		return plNumLit(t, false, t.pos)
	case plTString:
		p.next()
		return &plELit{plBase: plBase{t.pos}, val: t.text, typ: "String"}, nil
	case plTRegex:
		p.next()
		return &plERegex{plBase: plBase{t.pos}, pattern: t.text, flags: t.flags}, nil
	case plTType:
		p.next()
		return &plEType{plBase: plBase{t.pos}, name: t.text}, nil
	case plTIdent:
		switch t.text {
		case "true", "false":
			p.next()
			return &plELit{plBase: plBase{t.pos}, val: t.text == "true", typ: "boolean"}, nil
		case "null":
			p.next()
			return &plELit{plBase: plBase{t.pos}, val: nil, typ: "null"}, nil
		case "new":
			return p.parseNew()
		}
		p.next()
		if p.isOp("(") {
			args, err := p.parseArgs()
			if err != nil {
				return nil, err
			}
			return &plECall{plBase: plBase{t.pos}, name: t.text, args: args}, nil
		}
		return &plEVar{plBase: plBase{t.pos}, name: t.text}, nil
	case plTOp:
		switch t.text {
		case "(":
			p.next()
			x, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.expectOp(")"); err != nil {
				return nil, err
			}
			return x, nil
		case "[":
			return p.parseListOrMap()
		}
	}
	return nil, p.errUnexpected("")
}

func (p *plParser) parseListOrMap() (plNode, *plCompileErr) {
	t := p.next() // [
	if p.isOp(":") && p.peekAt(1).kind == plTOp && p.peekAt(1).text == "]" {
		p.next()
		p.next()
		return &plEMap{plBase: plBase{t.pos}}, nil
	}
	if p.acceptOp("]") {
		return &plEList{plBase: plBase{t.pos}}, nil
	}
	first, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.acceptOp(":") {
		m := &plEMap{plBase: plBase{t.pos}}
		v, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		m.keys = append(m.keys, first)
		m.vals = append(m.vals, v)
		for p.acceptOp(",") {
			k, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.expectOp(":"); err != nil {
				return nil, err
			}
			v, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			m.keys = append(m.keys, k)
			m.vals = append(m.vals, v)
		}
		if err := p.expectOp("]"); err != nil {
			return nil, err
		}
		return m, nil
	}
	l := &plEList{plBase: plBase{t.pos}, elems: []plNode{first}}
	for p.acceptOp(",") {
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		l.elems = append(l.elems, e)
	}
	if err := p.expectOp("]"); err != nil {
		return nil, err
	}
	return l, nil
}

func (p *plParser) parseNew() (plNode, *plCompileErr) {
	t := p.next() // new
	nt := p.peek()
	if nt.kind == plTIdent {
		return nil, &plCompileErr{typ: "illegal_argument_exception", offset: t.pos, reason: "Not a type [" + nt.text + "]."}
	}
	if nt.kind != plTType {
		return nil, p.errUnexpected("")
	}
	p.next()
	typ := nt.text
	if p.isOp("[") {
		a := &plENewArray{plBase: plBase{t.pos}, elem: typ}
		for p.isOp("[") {
			p.next()
			if p.acceptOp("]") {
				a.rank++
				continue
			}
			d, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if err := p.expectOp("]"); err != nil {
				return nil, err
			}
			a.dims = append(a.dims, d)
			a.rank++
		}
		if p.isOp("{") {
			p.next()
			a.hasInit = true
			for !p.isOp("}") {
				e, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				a.init = append(a.init, e)
				if !p.acceptOp(",") {
					break
				}
			}
			if err := p.expectOp("}"); err != nil {
				return nil, err
			}
		}
		return a, nil
	}
	if !p.isOp("(") {
		return nil, p.errUnexpected("'('")
	}
	args, err := p.parseArgs()
	if err != nil {
		return nil, err
	}
	return &plENew{plBase: plBase{t.pos}, typ: typ, args: args}, nil
}

func plIsPrimitive(t string) bool {
	switch t {
	case "boolean", "byte", "short", "char", "int", "long", "float", "double":
		return true
	}
	return false
}

func plIsNumericType(t string) bool {
	switch t {
	case "byte", "short", "char", "int", "long", "float", "double":
		return true
	}
	return false
}

var _ = strings.TrimSpace
