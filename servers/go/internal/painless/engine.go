package painless

import (
	"sync"
)

// Painless engine: script contexts, compilation cache and execution.

var ScoreFuncs = ParseSigs(
	"saturation:double(double,double)", "sigmoid:double(double,double,double)",
	"decayNumericGauss:double(double,double,double,double,double)",
	"decayNumericExp:double(double,double,double,double,double)",
	"decayNumericLinear:double(double,double,double,double,double)",
	"decayGeoGauss:double(String,String,String,double,def)", "decayGeoExp:double(String,String,String,double,def)",
	"decayGeoLinear:double(String,String,String,double,def)",
	"decayDateGauss:double(String,String,String,double,def)", "decayDateExp:double(String,String,String,double,def)",
	"decayDateLinear:double(String,String,String,double,def)",
	"randomScore:double(int)", "randomScore:double(int,String)",
)

func plVars(kv ...string) map[string]string {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return m
}

var plContexts = map[string]*plCheckCtx{
	"painless_test":         {vars: plVars("params", "Map"), ret: "Object"},
	"filter":                {vars: plVars("params", "Map", "doc", "Map"), ret: "boolean"},
	"score":                 {vars: plVars("params", "Map", "doc", "Map", "_score", "double"), funcs: ScoreFuncs, ret: "double"},
	"field":                 {vars: plVars("params", "Map", "doc", "Map"), ret: "Object"},
	"number_sort":           {vars: plVars("params", "Map", "doc", "Map", "_score", "double"), ret: "double"},
	"string_sort":           {vars: plVars("params", "Map", "doc", "Map", "_score", "double"), ret: "String"},
	"aggs":                  {vars: plVars("params", "Map", "doc", "Map", "_value", "def", "_score", "double"), ret: "Object"},
	"aggs_init":             {vars: plVars("params", "Map", "state", "Map"), ret: "void"},
	"aggs_map":              {vars: plVars("params", "Map", "state", "Map", "doc", "Map", "_score", "double"), ret: "void"},
	"aggs_combine":          {vars: plVars("params", "Map", "state", "Map"), ret: "Object"},
	"aggs_reduce":           {vars: plVars("params", "Map", "states", "List"), ret: "Object"},
	"bucket_aggregation":    {vars: plVars("params", "Map"), ret: "Object"},
	"aggregation_selector":  {vars: plVars("params", "Map"), ret: "boolean"},
	"terms_set":             {vars: plVars("params", "Map", "doc", "Map"), ret: "Object"},
	"update":                {vars: plVars("params", "Map", "ctx", "Map"), ret: "void"},
	"ingest":                {vars: plVars("params", "Map", "ctx", "Map"), ret: "void"},
	"processor_conditional": {vars: plVars("params", "Map", "ctx", "Map"), ret: "boolean"},
	"template":              {vars: plVars("params", "Map"), ret: "String"},
}

type Script struct {
	src  string
	prog *plProgram
	info *plInfo
	ctx  *plCheckCtx
}

var plCache sync.Map // context + "\x00" + source -> *plEngineScript

func Compile(src, ctxName string) (*Script, *CompileError) {
	key := ctxName + "\x00" + src
	if v, ok := plCache.Load(key); ok {
		return v.(*Script), nil
	}
	ctx := plContexts[ctxName]
	if ctx == nil {
		ctx = plContexts["painless_test"]
	}
	prog, err := plParse(src)
	if err != nil {
		return nil, err
	}
	info, err := plCheck(prog, ctx)
	if err != nil {
		return nil, err
	}
	s := &Script{src: src, prog: prog, info: info, ctx: ctx}
	plCache.Store(key, s)
	return s, nil
}

type Bindings struct {
	Vars  map[string]any
	Funcs map[string]func(args []any, off int) (any, *RuntimeError)
}

// Run executes a compiled script and applies the context's return conversion.
func (s *Script) Run(b Bindings) (any, *RuntimeError) {
	in := &plInterp{prog: s.prog, info: s.info, funcs: b.Funcs}
	env := &plEnv{vars: map[string]*plVar{}}
	for k, t := range s.ctx.vars {
		env.vars[k] = &plVar{v: b.Vars[k], typ: t}
	}
	v, rerr := in.run(env)
	if rerr == nil {
		v, rerr = s.convertReturn(v)
	}
	return v, rerr
}

// Source returns the script Source.
func (s *Script) Source() string { return s.src }

// StatementAround returns the bounds of the statement that contains offset
// off, for the script stack of runtime errors.
func (s *Script) StatementAround(off int) (start, end int) {
	start, end = 0, len(s.src)
	for _, st := range s.prog.stmts {
		if st <= off && st >= start {
			start = st
		}
	}
	for _, st := range s.prog.stmts {
		if st > off && st < end {
			end = st
		}
	}
	return start, end
}

// retNode finds the expression whose value the script returns, for error
// offsets of failed return conversions.
func (s *Script) retNode() plNode {
	var found plNode
	var walk func(stmts []plNode)
	walk = func(stmts []plNode) {
		for _, st := range stmts {
			switch n := st.(type) {
			case *plSReturn:
				if n.x != nil {
					found = n.x
				}
			case *plSIf:
				if b, ok := n.then.(*plSBlock); ok {
					walk(b.stmts)
				} else {
					walk([]plNode{n.then})
				}
			case *plSBlock:
				walk(n.stmts)
			}
		}
		if len(stmts) > 0 {
			if es, ok := stmts[len(stmts)-1].(*plSExpr); ok {
				found = es.x
			}
		}
	}
	walk(s.prog.body.stmts)
	return found
}

func (s *Script) convertReturn(v any) (any, *RuntimeError) {
	off := 0
	if n := s.retNode(); n != nil {
		off = n.nodePos()
	}
	switch s.ctx.ret {
	case "boolean":
		if b, ok := v.(bool); ok {
			return b, nil
		}
		if v == nil {
			return nil, plNPE(off)
		}
		return nil, &RuntimeError{Type: "class_cast_exception", Offset: off,
			Reason: "cannot cast def [" + plDefTypeName(v) + "] to boolean"}
	case "double":
		if IsNumber(v) {
			return F64(v), nil
		}
		if v == nil {
			return nil, plNPE(off)
		}
		return nil, &RuntimeError{Type: "class_cast_exception", Offset: off,
			Reason: "cannot implicitly cast def [" + plDefTypeName(v) + "] to double"}
	case "String":
		if v == nil {
			return nil, nil
		}
		if str, ok := v.(string); ok {
			return str, nil
		}
		return nil, &RuntimeError{Type: "class_cast_exception", Offset: off,
			Reason: "cannot implicitly cast def [" + plDefTypeName(v) + "] to java.lang.String"}
	}
	return v, nil
}
