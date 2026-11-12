package main

import (
	"math"
	"regexp"
	"strings"
)

// Painless engine: tree-walking interpreter.

// plRtErr is a runtime exception raised inside a script.
type plRtErr struct {
	typ       string
	reason    string
	nilReason bool
	offset    int
	frames    []string // Java frames printed before the script frame
	extra     orderedJSON
	isError   bool // java.lang.Error: not catchable by catch (Exception e)
	thrown    any  // the exception object for catch blocks
}

func plNPE(off int) *plRtErr {
	return &plRtErr{typ: "null_pointer_exception", offset: off,
		reason: `Cannot invoke "Object.getClass()" because "callArgs[0]" is null`}
}

func plIAE(off int, reason string) *plRtErr {
	return &plRtErr{typ: "illegal_argument_exception", reason: reason, offset: off}
}

type plEnv struct {
	vars   map[string]*plVar
	parent *plEnv
}

type plVar struct {
	v   any
	typ string
}

func (e *plEnv) lookup(name string) *plVar {
	for s := e; s != nil; s = s.parent {
		if v, ok := s.vars[name]; ok {
			return v
		}
	}
	return nil
}

func (e *plEnv) child() *plEnv { return &plEnv{vars: map[string]*plVar{}, parent: e} }

// plFn is a callable: a lambda, a function reference or a native function.
type plFn struct {
	pnames []string
	ptypes []string
	body   plNode
	env    *plEnv
	in     *plInterp
	native func(args []any, off int) (any, *plRtErr)
	name   string
}

type plCtl int

const (
	plNormal plCtl = iota
	plBreak
	plContinue
	plReturn
)

const plMaxLoop = 1000000

type plInterp struct {
	prog       *plProgram
	info       *plInfo
	loops      int
	funcs      map[string]func(args []any, off int) (any, *plRtErr)
	regexLimit int
	depth      int
}

// plMaxDepth bounds nested user-function and lambda calls. The JVM would
// throw StackOverflowError; Go would crash the whole process instead.
const plMaxDepth = 2000

func (in *plInterp) enter(off int) *plRtErr {
	in.depth++
	if in.depth > plMaxDepth {
		return &plRtErr{typ: "stack_overflow_error", nilReason: true, isError: true, offset: off}
	}
	return nil
}

func (in *plInterp) loopTick(off int) *plRtErr {
	in.loops++
	if in.loops > plMaxLoop {
		return &plRtErr{typ: "painless_error", isError: true, offset: off,
			reason: "The maximum number of statements that can be executed in a loop has been reached."}
	}
	return nil
}

// run executes the program body, returning the script's value.
func (in *plInterp) run(env *plEnv) (any, *plRtErr) {
	ctl, v, err := in.block(in.prog.body.stmts, env, true)
	if err != nil {
		return nil, err
	}
	if ctl == plReturn {
		return v, nil
	}
	return v, nil
}

func (in *plInterp) block(stmts []plNode, env *plEnv, last bool) (plCtl, any, *plRtErr) {
	var lastVal any
	for i, s := range stmts {
		isLast := last && i == len(stmts)-1
		ctl, v, err := in.stmt(s, env, isLast)
		if err != nil {
			return plNormal, nil, err
		}
		if ctl != plNormal {
			return ctl, v, nil
		}
		if isLast {
			lastVal = v
		}
	}
	return plNormal, lastVal, nil
}

func (in *plInterp) stmt(s plNode, env *plEnv, last bool) (plCtl, any, *plRtErr) {
	switch n := s.(type) {
	case *plSBlock:
		return in.block(n.stmts, env.child(), last)
	case *plSDecl:
		for i, name := range n.names {
			var v any
			if init := n.inits[i]; init != nil {
				x, err := in.eval(init, env)
				if err != nil {
					return plNormal, nil, err
				}
				v, err = in.store(x, n.typ, init.nodePos())
				if err != nil {
					return plNormal, nil, err
				}
			} else {
				v = plZero(n.typ)
			}
			env.vars[name] = &plVar{v: v, typ: n.typ}
		}
		return plNormal, nil, nil
	case *plSExpr:
		v, err := in.eval(n.x, env)
		if err != nil {
			return plNormal, nil, err
		}
		if last {
			if _, isAssign := n.x.(*plEAssign); isAssign {
				return plNormal, nil, nil
			}
			return plNormal, v, nil
		}
		return plNormal, nil, nil
	case *plSIf:
		c, err := in.evalBool(n.cond, env)
		if err != nil {
			return plNormal, nil, err
		}
		if c {
			return in.stmt(n.then, env.child(), last)
		}
		if n.els != nil {
			return in.stmt(n.els, env.child(), last)
		}
		return plNormal, nil, nil
	case *plSWhile:
		for {
			c, err := in.evalBool(n.cond, env)
			if err != nil {
				return plNormal, nil, err
			}
			if !c {
				return plNormal, nil, nil
			}
			if err := in.loopTick(n.pos); err != nil {
				return plNormal, nil, err
			}
			ctl, v, err := in.stmt(n.body, env.child(), false)
			if err != nil {
				return plNormal, nil, err
			}
			if ctl == plBreak {
				return plNormal, nil, nil
			}
			if ctl == plReturn {
				return ctl, v, nil
			}
		}
	case *plSDo:
		for {
			if err := in.loopTick(n.pos); err != nil {
				return plNormal, nil, err
			}
			ctl, v, err := in.stmt(n.body, env.child(), false)
			if err != nil {
				return plNormal, nil, err
			}
			if ctl == plBreak {
				return plNormal, nil, nil
			}
			if ctl == plReturn {
				return ctl, v, nil
			}
			c, err := in.evalBool(n.cond, env)
			if err != nil {
				return plNormal, nil, err
			}
			if !c {
				return plNormal, nil, nil
			}
		}
	case *plSFor:
		fenv := env.child()
		if n.init != nil {
			if _, _, err := in.stmt(n.init, fenv, false); err != nil {
				return plNormal, nil, err
			}
		}
		for {
			if n.cond != nil {
				c, err := in.evalBool(n.cond, fenv)
				if err != nil {
					return plNormal, nil, err
				}
				if !c {
					return plNormal, nil, nil
				}
			}
			if err := in.loopTick(n.pos); err != nil {
				return plNormal, nil, err
			}
			ctl, v, err := in.stmt(n.body, fenv.child(), false)
			if err != nil {
				return plNormal, nil, err
			}
			if ctl == plBreak {
				return plNormal, nil, nil
			}
			if ctl == plReturn {
				return ctl, v, nil
			}
			for _, u := range n.update {
				if _, err := in.eval(u, fenv); err != nil {
					return plNormal, nil, err
				}
			}
		}
	case *plSForEach:
		it, err := in.eval(n.iter, env)
		if err != nil {
			return plNormal, nil, err
		}
		items, rerr := plIterItems(it, n.iter.nodePos())
		if rerr != nil {
			return plNormal, nil, rerr
		}
		for _, item := range items {
			if err := in.loopTick(n.pos); err != nil {
				return plNormal, nil, err
			}
			benv := env.child()
			v, err := in.store(item, n.typ, n.pos)
			if err != nil {
				return plNormal, nil, err
			}
			benv.vars[n.name] = &plVar{v: v, typ: n.typ}
			ctl, rv, err := in.stmt(n.body, benv, false)
			if err != nil {
				return plNormal, nil, err
			}
			if ctl == plBreak {
				break
			}
			if ctl == plReturn {
				return ctl, rv, nil
			}
		}
		return plNormal, nil, nil
	case *plSBreak:
		return plBreak, nil, nil
	case *plSContinue:
		return plContinue, nil, nil
	case *plSReturn:
		if n.x == nil {
			return plReturn, nil, nil
		}
		v, err := in.eval(n.x, env)
		if err != nil {
			return plNormal, nil, err
		}
		return plReturn, v, nil
	case *plSThrow:
		v, err := in.eval(n.x, env)
		if err != nil {
			return plNormal, nil, err
		}
		ex, ok := v.(*plException)
		if !ok {
			if v == nil {
				return plNormal, nil, &plRtErr{typ: "null_pointer_exception", offset: n.x.nodePos(), nilReason: true}
			}
			return plNormal, nil, &plRtErr{typ: "class_cast_exception", offset: n.x.nodePos(),
				reason: "Cannot cast " + plClassName(v) + " to java.lang.Throwable"}
		}
		return plNormal, nil, &plRtErr{typ: ex.typ, reason: ex.msg, nilReason: ex.nilMsg, offset: n.x.nodePos(), thrown: ex}
	case *plSTry:
		ctl, v, err := in.stmt(n.body, env, last)
		if err == nil {
			return ctl, v, nil
		}
		if err.isError {
			return plNormal, nil, err
		}
		for _, ct := range n.catches {
			if plCatchMatches(ct.typ, err.typ) {
				cenv := env.child()
				ex, _ := err.thrown.(*plException)
				if ex == nil {
					ex = &plException{typ: err.typ, msg: err.reason, nilMsg: err.nilReason}
				}
				cenv.vars[ct.name] = &plVar{v: ex, typ: ct.typ}
				return in.stmt(ct.body, cenv, last)
			}
		}
		return plNormal, nil, err
	}
	return plNormal, nil, nil
}

func plCatchMatches(catchType, errType string) bool {
	switch catchType {
	case "Exception", "RuntimeException":
		return true
	}
	return plExceptionType(catchType) == errType
}

// plException is a Java exception object created with new or caught.
type plException struct {
	typ    string
	msg    string
	nilMsg bool
	class  string
}

func plExceptionType(class string) string {
	var sb strings.Builder
	for i, r := range class {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				sb.WriteByte('_')
			}
			sb.WriteRune(r + 32)
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func plZero(typ string) any {
	switch typ {
	case "int":
		return int32(0)
	case "long":
		return int64(0)
	case "float":
		return float32(0)
	case "double":
		return float64(0)
	case "short":
		return int16(0)
	case "byte":
		return int8(0)
	case "char":
		return plChar(0)
	case "boolean":
		return false
	}
	return nil
}

// store converts a value for storage into a variable of the given static type.
func (in *plInterp) store(v any, typ string, off int) (any, *plRtErr) {
	switch typ {
	case "", "def", "Object":
		return v, nil
	case "int", "long", "float", "double", "short", "byte", "char":
		if v == nil {
			return nil, plNPE(off)
		}
		if plIsNumber(v) {
			return plCastNum(v, typ), nil
		}
		return nil, &plRtErr{typ: "class_cast_exception", offset: off,
			reason: "Cannot cast " + plClassName(v) + " to java.lang." + plBoxOf[typ]}
	case "boolean":
		if b, ok := v.(bool); ok {
			return b, nil
		}
		if v == nil {
			return nil, plNPE(off)
		}
		return nil, &plRtErr{typ: "class_cast_exception", offset: off,
			reason: "Cannot cast " + plClassName(v) + " to java.lang.Boolean"}
	case "String":
		if v == nil {
			return nil, nil
		}
		if _, ok := v.(string); ok {
			return v, nil
		}
		return nil, &plRtErr{typ: "class_cast_exception", offset: off,
			reason: "Cannot cast " + plClassName(v) + " to java.lang.String"}
	}
	return v, nil
}

func plClassName(v any) string {
	n, _ := plRuntimeClass(v)
	return n
}

func (in *plInterp) evalBool(x plNode, env *plEnv) (bool, *plRtErr) {
	v, err := in.eval(x, env)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		if v == nil {
			return false, plNPE(x.nodePos())
		}
		return false, &plRtErr{typ: "class_cast_exception", offset: x.nodePos(),
			reason: "Cannot cast " + plClassName(v) + " to java.lang.Boolean"}
	}
	return b, nil
}

func (in *plInterp) eval(x plNode, env *plEnv) (any, *plRtErr) {
	if cv, ok := in.info.consts[x]; ok {
		switch x.(type) {
		case *plELit, *plEField, *plEBinary, *plEUnary, *plECast:
			return cv, nil
		}
	}
	switch n := x.(type) {
	case *plELit:
		return n.val, nil
	case *plERegex:
		return plCompilePattern(n.pattern, n.flags, n.pos)
	case *plEVar:
		v := env.lookup(n.name)
		if v == nil {
			return nil, plIAE(n.pos, "cannot resolve symbol ["+n.name+"]")
		}
		return v.v, nil
	case *plEType:
		return &plStaticRef{name: n.name}, nil
	case *plEField:
		if et, ok := n.obj.(*plEType); ok {
			if f, ok := plStaticFields[et.name][n.name]; ok {
				return f.val, nil
			}
			return plStaticEnum(et.name, n.name, n.pos)
		}
		obj, err := in.eval(n.obj, env)
		if err != nil {
			return nil, err
		}
		if obj == nil {
			if n.nullSafe {
				return nil, nil
			}
			return nil, plNPE(n.pos)
		}
		return plGetField(obj, n.name, n.pos)
	case *plECall:
		return in.evalCall(n, env)
	case *plEIndex:
		obj, err := in.eval(n.obj, env)
		if err != nil {
			return nil, err
		}
		idx, err := in.eval(n.idx, env)
		if err != nil {
			return nil, err
		}
		return plIndexGet(obj, idx, n.pos)
	case *plENew:
		args, err := in.evalArgs(n.args, env)
		if err != nil {
			return nil, err
		}
		return plNew(n.typ, args, n.pos)
	case *plENewArray:
		if n.hasInit {
			a := &plArray{elem: n.elem}
			for _, e := range n.init {
				v, err := in.eval(e, env)
				if err != nil {
					return nil, err
				}
				v, err = in.store(v, n.elem, e.nodePos())
				if err != nil {
					return nil, err
				}
				a.a = append(a.a, v)
			}
			return a, nil
		}
		dims := []int{}
		for _, d := range n.dims {
			v, err := in.eval(d, env)
			if err != nil {
				return nil, err
			}
			dims = append(dims, int(plI64(v)))
		}
		return plMakeArray(n.elem, n.rank, dims), nil
	case *plEList:
		l := &plList{}
		for _, e := range n.elems {
			v, err := in.eval(e, env)
			if err != nil {
				return nil, err
			}
			l.a = append(l.a, v)
		}
		return l, nil
	case *plEMap:
		m := newPlMap(plHash)
		for i := range n.keys {
			k, err := in.eval(n.keys[i], env)
			if err != nil {
				return nil, err
			}
			v, err := in.eval(n.vals[i], env)
			if err != nil {
				return nil, err
			}
			m.put(k, v)
		}
		return m, nil
	case *plEUnary:
		v, err := in.eval(n.x, env)
		if err != nil {
			return nil, err
		}
		return plUnary(n.op, v, n.pos)
	case *plEBinary:
		return in.evalBinary(n, env)
	case *plEInstanceof:
		v, err := in.eval(n.x, env)
		if err != nil {
			return nil, err
		}
		return plInstanceOf(v, n.typ, in.info.types[n.x]), nil
	case *plECast:
		v, err := in.eval(n.x, env)
		if err != nil {
			return nil, err
		}
		return plExplicitCast(v, n.typ, n.x.nodePos())
	case *plECond:
		c, err := in.evalBool(n.c, env)
		if err != nil {
			return nil, err
		}
		var v any
		if c {
			v, err = in.eval(n.a, env)
		} else {
			v, err = in.eval(n.b, env)
		}
		if err != nil {
			return nil, err
		}
		if t := in.info.types[n]; plIsNumericType(t) && plIsNumber(v) {
			return plCastNum(v, t), nil
		}
		return v, nil
	case *plEElvis:
		v, err := in.eval(n.a, env)
		if err != nil {
			return nil, err
		}
		if v != nil {
			return v, nil
		}
		return in.eval(n.b, env)
	case *plEAssign:
		return in.evalAssign(n, env)
	case *plEIncDec:
		return in.evalIncDec(n, env)
	case *plELambda:
		return &plFn{pnames: n.pnames, ptypes: n.ptypes, body: n.body, env: env, in: in}, nil
	case *plEFuncRef:
		return in.funcRef(n, env)
	}
	return nil, &plRtErr{typ: "illegal_state_exception", reason: "unsupported expression", offset: x.nodePos()}
}

func (in *plInterp) evalArgs(args []plNode, env *plEnv) ([]any, *plRtErr) {
	out := make([]any, len(args))
	for i, a := range args {
		v, err := in.eval(a, env)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

type plStaticRef struct{ name string }

func (in *plInterp) evalCall(n *plECall, env *plEnv) (any, *plRtErr) {
	args, err := in.evalArgs(n.args, env)
	if err != nil {
		return nil, err
	}
	if sig, ok := in.info.sigs[n]; ok {
		for i, p := range sig.params {
			if i < len(args) && plIsPrimitive(p) && plIsNumber(args[i]) {
				args[i] = plCastNum(args[i], p)
			}
		}
	}
	if n.obj == nil {
		if f, ok := in.prog.funcs[n.name]; ok {
			return in.callUserFunc(f, args, n.pos)
		}
		if fn, ok := in.funcs[n.name]; ok {
			return fn(args, n.pos)
		}
		return nil, plIAE(n.pos, "Unknown call ["+n.name+"] with ["+itoa(len(args))+"] arguments.")
	}
	if et, ok := n.obj.(*plEType); ok {
		return in.callStatic(et.name, n.name, args, n)
	}
	recv, err := in.eval(n.obj, env)
	if err != nil {
		return nil, err
	}
	if recv == nil {
		if n.nullSafe {
			return nil, nil
		}
		return nil, plNPE(n.pos)
	}
	return in.callMethod(recv, n.name, args, n.pos)
}

func (in *plInterp) callUserFunc(f *plFunc, args []any, off int) (any, *plRtErr) {
	if err := in.enter(off); err != nil {
		return nil, err
	}
	defer func() { in.depth-- }()
	env := &plEnv{vars: map[string]*plVar{}}
	for i, name := range f.pnames {
		v, err := in.store(args[i], f.ptypes[i], off)
		if err != nil {
			return nil, err
		}
		env.vars[name] = &plVar{v: v, typ: f.ptypes[i]}
	}
	_, v, err := in.block(f.body.stmts, env, false)
	if err != nil {
		return nil, err
	}
	return in.store(v, f.ret, off)
}

// callFn invokes a lambda, function reference or native function.
func (in *plInterp) callFn(fnv any, args []any, off int) (any, *plRtErr) {
	if err := in.enter(off); err != nil {
		return nil, err
	}
	defer func() { in.depth-- }()
	fn, ok := fnv.(*plFn)
	if !ok {
		if fnv == nil {
			return nil, plNPE(off)
		}
		return nil, &plRtErr{typ: "class_cast_exception", offset: off,
			reason: "Cannot cast " + plClassName(fnv) + " to a functional interface"}
	}
	if fn.native != nil {
		return fn.native(args, off)
	}
	if len(args) != len(fn.pnames) {
		return nil, plIAE(off, "Incorrect number of parameters for [lambda] in [java.util.function.Function]")
	}
	env := fn.env.child()
	for i, name := range fn.pnames {
		v, err := fn.in.store(args[i], fn.ptypes[i], off)
		if err != nil {
			return nil, err
		}
		env.vars[name] = &plVar{v: v, typ: fn.ptypes[i]}
	}
	if b, ok := fn.body.(*plSBlock); ok {
		_, v, err := fn.in.block(b.stmts, env, true)
		return v, err
	}
	return fn.in.eval(fn.body, env)
}

func (in *plInterp) funcRef(n *plEFuncRef, env *plEnv) (any, *plRtErr) {
	if n.typ == "this" {
		f, ok := in.prog.funcs[n.method]
		if !ok {
			return nil, plIAE(n.pos, "Unknown reference [this::"+n.method+"]")
		}
		return &plFn{name: n.method, native: func(args []any, off int) (any, *plRtErr) {
			return in.callUserFunc(f, args, off)
		}}, nil
	}
	typ, method := n.typ, n.method
	if !plTypeNames[typ] {
		// variable::method binds the receiver
		v := env.lookup(typ)
		if v == nil {
			return nil, plIAE(n.pos, "cannot resolve symbol ["+typ+"]")
		}
		recv := v.v
		return &plFn{native: func(args []any, off int) (any, *plRtErr) {
			return in.callMethod(recv, method, args, off)
		}}, nil
	}
	if method == "new" {
		return &plFn{native: func(args []any, off int) (any, *plRtErr) { return plNew(typ, args, off) }}, nil
	}
	return &plFn{native: func(args []any, off int) (any, *plRtErr) {
		key := method + "/" + itoa(len(args))
		if _, ok := plStaticMethods[typ][key]; ok {
			return in.callStatic(typ, method, args, &plECall{plBase: plBase{off}})
		}
		if len(args) == 0 {
			return nil, plIAE(off, "Unknown reference ["+typ+"::"+method+"]")
		}
		return in.callMethod(args[0], method, args[1:], off)
	}}, nil
}

// ---- operators ----

func plUnary(op string, v any, off int) (any, *plRtErr) {
	switch op {
	case "!":
		b, ok := v.(bool)
		if !ok {
			return nil, &plRtErr{typ: "class_cast_exception", offset: off, reason: "Cannot cast " + plClassName(v) + " to java.lang.Boolean"}
		}
		return !b, nil
	case "-", "+", "~":
		if v == nil {
			return nil, plNPE(off)
		}
		if !plIsNumber(v) {
			return nil, &plRtErr{typ: "class_cast_exception", offset: off, reason: "Cannot apply [" + op + "] operation to type [" + plClassName(v) + "]."}
		}
		t := plPromote(v, int32(0))
		v = plCastNum(v, t)
		switch op {
		case "+":
			return v, nil
		case "-":
			switch x := v.(type) {
			case int32:
				return -x, nil
			case int64:
				return -x, nil
			case float32:
				return -x, nil
			case float64:
				return -x, nil
			}
		case "~":
			switch x := v.(type) {
			case int32:
				return ^x, nil
			case int64:
				return ^x, nil
			}
			return nil, &plRtErr{typ: "class_cast_exception", offset: off, reason: "Cannot apply [~] operation to type [" + plClassName(v) + "]."}
		}
	}
	return v, nil
}

// plArith applies a numeric binary operator with Java promotion rules.
func plArith(op string, a, b any) (any, *plRtErr) {
	switch op {
	case "<<", ">>", ">>>":
		lt := plPromote(a, int32(0))
		n := plI64(b)
		if lt == "long" {
			x := plI64(a)
			s := uint(n & 63)
			switch op {
			case "<<":
				return x << s, nil
			case ">>":
				return x >> s, nil
			}
			return int64(uint64(x) >> s), nil
		}
		x := int32(plI64(a))
		s := uint(n & 31)
		switch op {
		case "<<":
			return x << s, nil
		case ">>":
			return x >> s, nil
		}
		return int32(uint32(x) >> s), nil
	}
	t := plPromote(a, b)
	switch t {
	case "double", "float":
		x, y := plF64(a), plF64(b)
		if t == "float" {
			fx, fy := float32(x), float32(y)
			var r float32
			switch op {
			case "+":
				r = fx + fy
			case "-":
				r = fx - fy
			case "*":
				r = fx * fy
			case "/":
				r = fx / fy
			case "%":
				r = float32(math.Mod(float64(fx), float64(fy)))
			default:
				return nil, &plRtErr{typ: "class_cast_exception", reason: "Cannot apply [" + op + "] operation to types [java.lang.Float] and [java.lang.Float]."}
			}
			return r, nil
		}
		switch op {
		case "+":
			return x + y, nil
		case "-":
			return x - y, nil
		case "*":
			return x * y, nil
		case "/":
			return x / y, nil
		case "%":
			return math.Mod(x, y), nil
		}
		return nil, &plRtErr{typ: "class_cast_exception", reason: "Cannot apply [" + op + "] operation to types [java.lang.Double] and [java.lang.Double]."}
	case "long":
		x, y := plI64(a), plI64(b)
		switch op {
		case "+":
			return x + y, nil
		case "-":
			return x - y, nil
		case "*":
			return x * y, nil
		case "/":
			if y == 0 {
				return nil, &plRtErr{typ: "arithmetic_exception", reason: "/ by zero"}
			}
			if x == math.MinInt64 && y == -1 {
				return x, nil
			}
			return x / y, nil
		case "%":
			if y == 0 {
				return nil, &plRtErr{typ: "arithmetic_exception", reason: "/ by zero"}
			}
			if y == -1 {
				return int64(0), nil
			}
			return x % y, nil
		case "&":
			return x & y, nil
		case "|":
			return x | y, nil
		case "^":
			return x ^ y, nil
		}
	default:
		x, y := int32(plI64(a)), int32(plI64(b))
		switch op {
		case "+":
			return x + y, nil
		case "-":
			return x - y, nil
		case "*":
			return x * y, nil
		case "/":
			if y == 0 {
				return nil, &plRtErr{typ: "arithmetic_exception", reason: "/ by zero"}
			}
			if x == math.MinInt32 && y == -1 {
				return x, nil
			}
			return x / y, nil
		case "%":
			if y == 0 {
				return nil, &plRtErr{typ: "arithmetic_exception", reason: "/ by zero"}
			}
			if y == -1 {
				return int32(0), nil
			}
			return x % y, nil
		case "&":
			return x & y, nil
		case "|":
			return x | y, nil
		case "^":
			return x ^ y, nil
		}
	}
	return nil, &plRtErr{typ: "class_cast_exception", reason: "Cannot apply [" + op + "] operation."}
}

func (in *plInterp) evalBinary(n *plEBinary, env *plEnv) (any, *plRtErr) {
	switch n.op {
	case "&&":
		l, err := in.evalBool(n.l, env)
		if err != nil || !l {
			return false, err
		}
		return in.evalBool(n.r, env)
	case "||":
		l, err := in.evalBool(n.l, env)
		if err != nil {
			return nil, err
		}
		if l {
			return true, nil
		}
		return in.evalBool(n.r, env)
	}
	l, err := in.eval(n.l, env)
	if err != nil {
		return nil, err
	}
	r, err := in.eval(n.r, env)
	if err != nil {
		return nil, err
	}
	return in.binaryOp(n.op, l, r, in.info.types[n.l], in.info.types[n.r], n.pos)
}

func (in *plInterp) binaryOp(op string, l, r any, tl, tr string, off int) (any, *plRtErr) {
	switch op {
	case "==":
		return plEquals(l, r), nil
	case "!=":
		return !plEquals(l, r), nil
	case "===":
		if plIsPrimitive(tl) && plIsPrimitive(tr) {
			return plEquals(l, r), nil
		}
		return plIdentical(l, r), nil
	case "!==":
		if plIsPrimitive(tl) && plIsPrimitive(tr) {
			return !plEquals(l, r), nil
		}
		return !plIdentical(l, r), nil
	case "=~", "==~":
		p, ok := r.(*plPattern)
		if !ok {
			return nil, &plRtErr{typ: "class_cast_exception", offset: off, reason: "Cannot cast " + plClassName(r) + " to java.util.regex.Pattern"}
		}
		s, ok := l.(string)
		if !ok {
			if l == nil {
				return nil, plNPE(off)
			}
			s = plToString(l)
		}
		m := p.matcher(s, in)
		if op == "=~" {
			return m.find(off)
		}
		return m.matches(off)
	case "+":
		_, ls := l.(string)
		_, rs := r.(string)
		if ls || rs || tl == "String" || tr == "String" {
			return plToString(l) + plToString(r), nil
		}
	case "<", "<=", ">", ">=":
		if !plIsNumber(l) || !plIsNumber(r) {
			if l == nil || r == nil {
				return nil, plNPE(off)
			}
			return nil, &plRtErr{typ: "class_cast_exception", offset: off,
				reason: "Cannot apply [" + op + "] operation to types [" + plClassName(l) + "] and [" + plClassName(r) + "]."}
		}
		c, _ := plCompare(l, r)
		if plPromote(l, r) == "double" || plPromote(l, r) == "float" {
			x, y := plF64(l), plF64(r)
			if math.IsNaN(x) || math.IsNaN(y) {
				return false, nil
			}
		}
		switch op {
		case "<":
			return c < 0, nil
		case "<=":
			return c <= 0, nil
		case ">":
			return c > 0, nil
		}
		return c >= 0, nil
	case "&", "|", "^":
		lb, lok := l.(bool)
		rb, rok := r.(bool)
		if lok && rok {
			switch op {
			case "&":
				return lb && rb, nil
			case "|":
				return lb || rb, nil
			}
			return lb != rb, nil
		}
	}
	if l == nil || r == nil {
		return nil, plNPE(off)
	}
	if !plIsNumber(l) || !plIsNumber(r) {
		bad, good, goodT := l, r, tr
		if plIsNumber(l) {
			bad, good, goodT = r, l, tl
		}
		if goodT != "" && goodT != "def" && plIsPrimitive(goodT) {
			return nil, &plRtErr{typ: "class_cast_exception", offset: off,
				frames: []string{"java.base/java.lang.Class.cast(Class.java:3492)"},
				reason: "Cannot cast " + plClassName(bad) + " to java.lang." + plBoxOf[plPromoteStatic(goodT)]}
		}
		_ = good
		return nil, &plRtErr{typ: "class_cast_exception", offset: off,
			reason: "Cannot apply [" + op + "] operation to types [" + plClassName(l) + "] and [" + plClassName(r) + "]."}
	}
	v, err := plArith(op, l, r)
	if err != nil {
		err.offset = off
		return nil, err
	}
	return v, nil
}

func plPromoteStatic(t string) string {
	switch t {
	case "byte", "short", "char":
		return "int"
	}
	return t
}

// ---- assignment ----

func (in *plInterp) evalAssign(n *plEAssign, env *plEnv) (any, *plRtErr) {
	tt := in.info.types[n.target]
	if n.op == "=" {
		v, err := in.eval(n.val, env)
		if err != nil {
			return nil, err
		}
		return in.assignTo(n.target, v, tt, env, n.val.nodePos())
	}
	get, set, err := in.lvalue(n.target, env)
	if err != nil {
		return nil, err
	}
	old, err := get()
	if err != nil {
		return nil, err
	}
	rv, err := in.eval(n.val, env)
	if err != nil {
		return nil, err
	}
	res, err := in.binaryOp(n.op, old, rv, tt, in.info.types[n.val], n.pos)
	if err != nil {
		return nil, err
	}
	res = plCastBack(res, old, tt)
	if err := set(res); err != nil {
		return nil, err
	}
	return res, nil
}

// plCastBack casts a compound-assignment result to the target's type: the
// static type, or for def targets the runtime type of the old value.
func plCastBack(res, old any, tt string) any {
	if plIsNumericType(tt) && plIsNumber(res) {
		return plCastNum(res, tt)
	}
	if (tt == "def" || tt == "" || tt == "Object") && plIsNumber(old) && plIsNumber(res) {
		return plCastNum(res, plPrimName(old))
	}
	return res
}

func (in *plInterp) assignTo(target plNode, v any, tt string, env *plEnv, off int) (any, *plRtErr) {
	switch t := target.(type) {
	case *plEVar:
		slot := env.lookup(t.name)
		if slot == nil {
			return nil, plIAE(t.pos, "cannot resolve symbol ["+t.name+"]")
		}
		sv, err := in.store(v, slot.typ, off)
		if err != nil {
			return nil, err
		}
		slot.v = sv
		return sv, nil
	case *plEField:
		obj, err := in.eval(t.obj, env)
		if err != nil {
			return nil, err
		}
		if obj == nil {
			return nil, plNPE(t.pos)
		}
		return v, plSetField(obj, t.name, v, t.pos)
	case *plEIndex:
		obj, err := in.eval(t.obj, env)
		if err != nil {
			return nil, err
		}
		idx, err := in.eval(t.idx, env)
		if err != nil {
			return nil, err
		}
		if a, ok := obj.(*plArray); ok {
			v, err = in.store(v, a.elem, off)
			if err != nil {
				return nil, err
			}
		}
		return v, plIndexSet(obj, idx, v, t.pos)
	}
	return nil, plIAE(target.nodePos(), "invalid assignment target")
}

// lvalue returns getter and setter closures for a compound assignment target.
func (in *plInterp) lvalue(target plNode, env *plEnv) (func() (any, *plRtErr), func(any) *plRtErr, *plRtErr) {
	switch t := target.(type) {
	case *plEVar:
		slot := env.lookup(t.name)
		if slot == nil {
			return nil, nil, plIAE(t.pos, "cannot resolve symbol ["+t.name+"]")
		}
		return func() (any, *plRtErr) { return slot.v, nil },
			func(v any) *plRtErr {
				sv, err := in.store(v, slot.typ, t.pos)
				if err != nil {
					return err
				}
				slot.v = sv
				return nil
			}, nil
	case *plEField:
		obj, err := in.eval(t.obj, env)
		if err != nil {
			return nil, nil, err
		}
		if obj == nil {
			return nil, nil, plNPE(t.pos)
		}
		return func() (any, *plRtErr) { return plGetField(obj, t.name, t.pos) },
			func(v any) *plRtErr { return plSetField(obj, t.name, v, t.pos) }, nil
	case *plEIndex:
		obj, err := in.eval(t.obj, env)
		if err != nil {
			return nil, nil, err
		}
		idx, err := in.eval(t.idx, env)
		if err != nil {
			return nil, nil, err
		}
		return func() (any, *plRtErr) { return plIndexGet(obj, idx, t.pos) },
			func(v any) *plRtErr {
				if a, ok := obj.(*plArray); ok && plIsNumber(v) && plIsNumericType(a.elem) {
					v = plCastNum(v, a.elem)
				}
				return plIndexSet(obj, idx, v, t.pos)
			}, nil
	}
	return nil, nil, plIAE(target.nodePos(), "invalid assignment target")
}

func (in *plInterp) evalIncDec(n *plEIncDec, env *plEnv) (any, *plRtErr) {
	get, set, err := in.lvalue(n.target, env)
	if err != nil {
		return nil, err
	}
	old, err := get()
	if err != nil {
		return nil, err
	}
	if old == nil {
		return nil, plNPE(n.pos)
	}
	op := "+"
	if n.op == "--" {
		op = "-"
	}
	res, rerr := in.binaryOp(op, old, int32(1), in.info.types[n.target], "int", n.pos)
	if rerr != nil {
		return nil, rerr
	}
	res = plCastBack(res, old, in.info.types[n.target])
	if err := set(res); err != nil {
		return nil, err
	}
	if n.prefix {
		return res, nil
	}
	return old, nil
}

// ---- casts / instanceof ----

func plExplicitCast(v any, typ string, off int) (any, *plRtErr) {
	switch typ {
	case "int", "long", "float", "double", "short", "byte", "char":
		if s, ok := v.(string); ok && typ == "char" && len([]rune(s)) == 1 {
			return plChar([]rune(s)[0]), nil
		}
		if v == nil {
			return nil, plNPE(off)
		}
		if !plIsNumber(v) {
			return nil, &plRtErr{typ: "class_cast_exception", offset: off,
				reason: "Cannot cast " + plClassName(v) + " to " + typ}
		}
		return plCastNum(v, typ), nil
	case "boolean", "Boolean":
		if _, ok := v.(bool); ok || v == nil {
			return v, nil
		}
	case "def", "Object":
		return v, nil
	case "String":
		if _, ok := v.(string); ok || v == nil {
			return v, nil
		}
	case "Integer", "Long", "Double", "Float", "Short", "Byte", "Character":
		if v == nil {
			return nil, nil
		}
		if plIsNumber(v) {
			return plCastNum(v, plUnbox(typ)), nil
		}
	default:
		if v == nil || plInstanceOf(v, typ, "") {
			return v, nil
		}
	}
	return nil, &plRtErr{typ: "class_cast_exception", offset: off,
		reason: "Cannot cast " + plClassName(v) + " to " + plCanon(typ)}
}

func plInstanceOf(v any, typ, static string) bool {
	if typ == "def" {
		return true
	}
	if v == nil {
		return false
	}
	switch typ {
	case "Object":
		return true
	case "Integer":
		_, ok := v.(int32)
		return ok
	case "Long":
		_, ok := v.(int64)
		return ok
	case "Double":
		_, ok := v.(float64)
		return ok
	case "Float":
		_, ok := v.(float32)
		return ok
	case "Short":
		_, ok := v.(int16)
		return ok
	case "Byte":
		_, ok := v.(int8)
		return ok
	case "Character":
		_, ok := v.(plChar)
		return ok
	case "Boolean", "boolean":
		_, ok := v.(bool)
		return ok
	case "Number":
		return plIsNumber(v) && plPrimName(v) != "char"
	case "String", "CharSequence":
		_, ok := v.(string)
		return ok
	case "int", "long", "double", "float", "short", "byte", "char":
		return plPrimName(v) == typ
	case "List", "ArrayList", "Collection", "Iterable":
		switch v.(type) {
		case *plList, *plDocValues:
			return true
		case *plSet:
			return typ == "Collection" || typ == "Iterable"
		}
		return false
	case "Map", "HashMap":
		m, ok := v.(*plMap)
		return ok && (typ == "Map" || m.kind != plTree)
	case "TreeMap":
		m, ok := v.(*plMap)
		return ok && m.kind == plTree
	case "LinkedHashMap":
		m, ok := v.(*plMap)
		return ok && m.kind == plLinked
	case "Set", "HashSet":
		_, ok := v.(*plSet)
		return ok
	case "StringBuilder":
		_, ok := v.(*plStringBuilder)
		return ok
	case "Pattern":
		_, ok := v.(*plPattern)
		return ok
	case "Matcher":
		_, ok := v.(*plMatcher)
		return ok
	case "ZonedDateTime":
		_, ok := v.(*plZDT)
		return ok
	case "LocalDate":
		_, ok := v.(*plLocalDate)
		return ok
	case "Exception", "RuntimeException":
		_, ok := v.(*plException)
		return ok
	}
	if strings.HasSuffix(typ, "[]") {
		a, ok := v.(*plArray)
		return ok && a.elem+"[]" == typ
	}
	if ex, ok := v.(*plException); ok {
		return plExceptionType(typ) == ex.typ
	}
	return false
}

func plMakeArray(elem string, rank int, dims []int) *plArray {
	if len(dims) == 0 {
		return &plArray{elem: elem + strings.Repeat("[]", rank-1)}
	}
	sub := elem + strings.Repeat("[]", rank-1)
	a := &plArray{elem: sub, a: make([]any, dims[0])}
	for i := range a.a {
		if len(dims) > 1 {
			a.a[i] = plMakeArray(elem, rank-1, dims[1:])
		} else if rank == 1 {
			a.a[i] = plZero(elem)
		}
	}
	return a
}

// ---- regex ----

type plPattern struct {
	src   string
	flags string
	re    *regexp.Regexp
}

type plMatcher struct {
	p     *plPattern
	input string
	in    *plInterp
	loc   []int
	pos   int
}

func plCompilePattern(src, flags string, off int) (any, *plRtErr) {
	prefix := ""
	for _, f := range flags {
		switch f {
		case 'i':
			prefix += "i"
		case 'm':
			prefix += "m"
		case 's':
			prefix += "s"
		}
	}
	expr := src
	if prefix != "" {
		expr = "(?" + prefix + ")" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, &plRtErr{typ: "pattern_syntax_exception", offset: off, reason: err.Error()}
	}
	return &plPattern{src: src, flags: flags, re: re}, nil
}

func (p *plPattern) matcher(s string, in *plInterp) *plMatcher {
	return &plMatcher{p: p, input: s, in: in}
}

// checkBudget enforces Painless' regex circuit breaker (limit factor 6).
func (m *plMatcher) checkBudget(off int, full bool) *plRtErr {
	n := len([]rune(m.input))
	limit := 6 * n
	pos := len([]rune(m.input[:m.pos]))
	if !plRegexExceeds(m.p.src, m.p.flags, m.input, full, pos, limit) {
		return nil
	}
	return &plRtErr{typ: "circuit_breaking_exception", offset: off,
		reason: "[scripting] Regular expression considered too many characters, pattern: [" + m.p.src +
			"], limit factor: [6], char limit: [" + itoa(limit) + "], count: [" + itoa(limit+1) +
			"], wrapped: [" + m.input + "], this limit can be changed by changed by the [script.painless.regex.limit-factor] setting"}
}

func (m *plMatcher) find(off int) (bool, *plRtErr) {
	if err := m.checkBudget(off, false); err != nil {
		return false, err
	}
	if m.pos > len(m.input) {
		m.loc = nil
		return false, nil
	}
	loc := m.p.re.FindStringSubmatchIndex(m.input[m.pos:])
	if loc == nil {
		m.loc = nil
		return false, nil
	}
	for i := range loc {
		if loc[i] >= 0 {
			loc[i] += m.pos
		}
	}
	m.loc = loc
	if loc[1] == loc[0] {
		m.pos = loc[1] + 1
	} else {
		m.pos = loc[1]
	}
	return true, nil
}

func (m *plMatcher) matches(off int) (bool, *plRtErr) {
	if err := m.checkBudget(off, true); err != nil {
		return false, err
	}
	re := regexp.MustCompile("^(?:" + m.p.re.String() + ")$")
	loc := re.FindStringSubmatchIndex(m.input)
	m.loc = loc
	return loc != nil, nil
}

func (m *plMatcher) group(i int, off int) (any, *plRtErr) {
	if m.loc == nil {
		return nil, &plRtErr{typ: "illegal_state_exception", offset: off, reason: "No match found"}
	}
	if 2*i+1 >= len(m.loc) {
		return nil, &plRtErr{typ: "index_out_of_bounds_exception", offset: off, reason: "No group " + itoa(i)}
	}
	if m.loc[2*i] < 0 {
		return nil, nil
	}
	return m.input[m.loc[2*i]:m.loc[2*i+1]], nil
}
