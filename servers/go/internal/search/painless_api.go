package search

import (
	"hash/fnv"
	"math"
	"sort"
	"strings"

	"slopensearch/internal/analysis"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/mapping"
	"slopensearch/internal/ojson"
	"slopensearch/internal/painless"
)

// ScriptError is a script_exception: a compile or runtime error with the
// script position.
type ScriptError struct {
	Compile   bool
	linkErr   bool
	Lang      string
	Script    string
	Stack     []string
	Pos       []int // offset, start, end; nil when unknown
	CauseType string
	Reason    string
	nilReason bool
	inner     *painless.CompileError
	Extra     ojson.Object
}

func (e *ScriptError) Object(withCause bool) ojson.Object {
	kind := "runtime error"
	if e.Compile {
		kind = "compile error"
	}
	if e.linkErr {
		kind = "link error"
	}
	lang := e.Lang
	if lang == "" {
		lang = "painless"
	}
	o := ojson.Object{{"type", "script_exception"}, {"reason", kind}}
	o = append(o, e.Extra...)
	stack := make([]any, len(e.Stack))
	for i, s := range e.Stack {
		stack[i] = s
	}
	o = append(o, ojson.KV{"script_stack", stack}, ojson.KV{"script", e.Script}, ojson.KV{"lang", lang})
	if e.Pos != nil {
		o = append(o, ojson.KV{"position", ojson.Object{{"offset", e.Pos[0]}, {"start", e.Pos[1]}, {"end", e.Pos[2]}}})
	}
	if withCause {
		o = append(o, ojson.KV{"caused_by", e.causeObject()})
	}
	return o
}

func (e *ScriptError) causeObject() ojson.Object {
	var reason any = e.Reason
	if e.nilReason {
		reason = nil
	}
	c := ojson.Object{{"type", e.CauseType}, {"reason", reason}}
	if e.inner != nil {
		var ir any = e.inner.Reason
		if e.inner.NilReason {
			ir = nil
		}
		c = append(c, ojson.KV{"caused_by", ojson.Object{{"type", e.inner.Type}, {"reason", ir}}})
	}
	return c
}

// ErrorBody renders the full {"error": ..., "status": 400} response.
func (e *ScriptError) ErrorBody() ojson.Object {
	errObj := ojson.Object{{"root_cause", []any{e.Object(false)}}}
	errObj = append(errObj, e.Object(true)...)
	return ojson.Object{{"error", errObj}, {"status", 400}}
}

func plCompileFailure(src, name string, ce *painless.CompileError) *ScriptError {
	e := &ScriptError{Compile: true, Script: name, CauseType: ce.Type, Reason: ce.Reason, nilReason: ce.NilReason, inner: ce.Cause, Stack: []string{}}
	if ce.NoPos {
		return e
	}
	off := ce.Offset
	start := off - 25
	if start < 0 {
		start = 0
	}
	end := off + 25
	if end > len(src) {
		end = len(src)
	}
	snippet := src[start:end]
	pad := off - start
	if start > 0 {
		snippet = "... " + snippet
		pad += 4
	}
	if end < len(src) {
		snippet += " ..."
	}
	e.Stack = []string{snippet, strings.Repeat(" ", pad) + "^---- HERE"}
	e.Pos = []int{off, start, end}
	return e
}

func plRuntimeFailure(s *Script, name string, re *painless.RuntimeError) *ScriptError {
	e := &ScriptError{Script: name, CauseType: re.Type, Reason: re.Reason, nilReason: re.NilReason}
	for _, f := range re.Extra {
		e.Extra = append(e.Extra, ojson.KV{f.Name, f.Value})
	}
	src := s.s.Source()
	off := re.Offset
	if off > len(src) {
		off = len(src)
	}
	start, end := s.s.StatementAround(off)
	e.Stack = append(append([]string{}, re.Frames...), src[start:end], strings.Repeat(" ", off-start)+"^---- HERE")
	e.Pos = []int{off, start, end}
	return e
}

// Script is a compiled Painless script.
type Script struct {
	s *painless.Script
}

// CompileScript compiles and converts errors to script exceptions.
func CompileScript(src, ctxName, name string) (*Script, *ScriptError) {
	s, err := painless.Compile(src, ctxName)
	if err != nil {
		if name == "" {
			name = src
		}
		return nil, plCompileFailure(src, name, err)
	}
	return &Script{s}, nil
}

// Exec runs a compiled script and applies the context's return conversion.
func (s *Script) Exec(b painless.Bindings, name string) (any, *ScriptError) {
	v, rerr := s.s.Run(b)
	if rerr != nil {
		if name == "" {
			name = s.s.Source()
		}
		return nil, plRuntimeFailure(s, name, rerr)
	}
	return v, nil
}

var plTextFielddataReason = "Text fields are not optimised for operations that require per-document field data like aggregations and sorting, so these operations are disabled by default. Please use a keyword field instead. Alternatively, set fielddata=true on [%s] in order to load field data by uninverting the inverted index. Note that this can use significant memory."

// DocLookup builds the doc variable for one document.
func DocLookup(qc *QueryCtx) *painless.DocMap {
	cache := map[string]*painless.DocValues{}
	has := func(field string) bool {
		if field == "_id" || field == "_index" || field == "_seq_no" || field == "_version" {
			return true
		}
		_, ok := mapping.FieldDef(qc.Idx, field)
		return ok
	}
	get := func(field string, off int) (any, *painless.RuntimeError) {
		if dv, ok := cache[field]; ok {
			return dv, nil
		}
		dv, err := plDocValuesFor(qc, field, off)
		if err != nil {
			return nil, err
		}
		cache[field] = dv
		return dv, nil
	}
	return &painless.DocMap{Get: get, Has: has}
}

func plDocValuesFor(qc *QueryCtx, field string, off int) (*painless.DocValues, *painless.RuntimeError) {
	switch field {
	case "_id":
		return &painless.DocValues{Kind: "Strings", Field: field, Values: []any{qc.ID}}, nil
	case "_index":
		return &painless.DocValues{Kind: "Strings", Field: field, Values: []any{qc.Idx.Name}}, nil
	case "_seq_no":
		return &painless.DocValues{Kind: "Longs", Field: field, Values: []any{qc.Idx.SeqNos[qc.ID]}}, nil
	}
	def, ok := mapping.FieldDef(qc.Idx, field)
	if !ok {
		return nil, &painless.RuntimeError{Type: "illegal_argument_exception", Offset: off,
			Reason: "No field found for [" + field + "] in mapping",
			Frames: []string{"org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:89)",
				"org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:55)"}}
	}
	ft, _ := def["type"].(string)
	if ft == "alias" {
		return plDocValuesFor(qc, mapping.ResolveAliasField(qc.Idx, field), off)
	}
	vals, _ := qc.Values(field)
	if ft == "knn_vector" {
		return plKnnDocValues(field, vals), nil
	}
	dv := &painless.DocValues{Field: field}
	switch ft {
	case "text", "match_only_text", "search_as_you_type":
		if fd, _ := def["fielddata"].(bool); !fd {
			return nil, &painless.RuntimeError{Type: "illegal_argument_exception", Offset: off,
				Reason: strings.Replace(plTextFielddataReason, "%s", field, 1),
				Frames: []string{
					"org.opensearch.index.mapper.TextFieldMapper$TextFieldType.fielddataBuilder(TextFieldMapper.java:1004)",
					"org.opensearch.index.fielddata.IndexFieldDataService.getForField(IndexFieldDataService.java:134)",
					"org.opensearch.index.query.QueryShardContext.lambda$lookup$0(QueryShardContext.java:530)",
					"org.opensearch.search.lookup.SearchLookup.lambda$new$0(SearchLookup.java:118)",
					"org.opensearch.search.lookup.LeafDocLookup.lambda$get$0(LeafDocLookup.java:93)",
					"org.opensearch.secure_sm.AccessController.doPrivileged(AccessController.java:76)",
					"org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:93)",
					"org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:55)"}}
		}
		analyzer, _ := def["analyzer"].(string)
		terms := []string{}
		seen := map[string]bool{}
		for _, v := range vals {
			s, _ := jsonv.ScalarString(v)
			for _, t := range analysis.Tokens(mapping.AnalysisIndex(qc.Idx), analyzer, s) {
				if !seen[t.Term] {
					seen[t.Term] = true
					terms = append(terms, t.Term)
				}
			}
		}
		sort.Strings(terms)
		dv.Kind = "Strings"
		for _, t := range terms {
			dv.Values = append(dv.Values, t)
		}
		return dv, nil
	case "integer", "long", "short", "byte", "unsigned_long", "token_count":
		dv.Kind = "Longs"
		for _, v := range vals {
			if f, ok := jsonv.ToFloat(v); ok {
				dv.Values = append(dv.Values, int64(f))
			}
		}
	case "float", "double", "half_float", "scaled_float":
		dv.Kind = "Doubles"
		for _, v := range vals {
			if f, ok := jsonv.ToFloat(v); ok {
				dv.Values = append(dv.Values, f)
			}
		}
	case "boolean":
		dv.Kind = "Booleans"
		for _, v := range vals {
			if b, ok := v.(bool); ok {
				dv.Values = append(dv.Values, b)
			}
		}
		sort.SliceStable(dv.Values, func(i, j int) bool { return !dv.Values[i].(bool) && dv.Values[j].(bool) })
		return dv, nil
	case "date", "date_nanos":
		dv.Kind = "Dates"
		ms := []int64{}
		for _, v := range vals {
			if f, ok := jsonv.ToFloat(v); ok {
				ms = append(ms, int64(f))
			}
		}
		sort.Slice(ms, func(i, j int) bool { return ms[i] < ms[j] })
		for _, m := range ms {
			dv.Values = append(dv.Values, painless.NewZDT(painless.TimeFromMillis(m)))
		}
		return dv, nil
	case "geo_point":
		dv.Kind = "GeoPoints"
		for _, v := range vals {
			if m, ok := v.(map[string]any); ok {
				lat, _ := jsonv.ToFloat(m["lat"])
				lon, _ := jsonv.ToFloat(m["lon"])
				dv.Values = append(dv.Values, &painless.GeoPoint{Lat: lat, Lon: lon})
			}
		}
		return dv, nil
	default:
		dv.Kind = "Strings"
		for _, v := range vals {
			s, ok := jsonv.ScalarString(v)
			if !ok {
				s = painless.ToString(v)
			}
			dv.Values = append(dv.Values, s)
		}
		sort.SliceStable(dv.Values, func(i, j int) bool { return dv.Values[i].(string) < dv.Values[j].(string) })
		return dv, nil
	}
	sort.SliceStable(dv.Values, func(i, j int) bool { c, _ := painless.Compare(dv.Values[i], dv.Values[j]); return c < 0 })
	return dv, nil
}

func ScoreBindings(qc *QueryCtx) map[string]func(args []any, off int) (any, *painless.RuntimeError) {
	f := func(a []any, i int) float64 { return painless.F64(a[i]) }
	dist := func(a []any) float64 {
		d := math.Abs(f(a, 4)-f(a, 0)) - f(a, 2)
		if d < 0 {
			d = 0
		}
		return d
	}
	m := plKnnBindings()
	for k, v := range map[string]func(args []any, off int) (any, *painless.RuntimeError){
		"saturation": func(a []any, off int) (any, *painless.RuntimeError) { return f(a, 0) / (f(a, 1) + f(a, 0)), nil },
		"sigmoid": func(a []any, off int) (any, *painless.RuntimeError) {
			v, k, e := f(a, 0), f(a, 1), f(a, 2)
			return math.Pow(v, e) / (math.Pow(k, e) + math.Pow(v, e)), nil
		},
		"decayNumericLinear": func(a []any, off int) (any, *painless.RuntimeError) {
			scaling := f(a, 1) / (1.0 - f(a, 3))
			return math.Max(0.0, (scaling-dist(a))/scaling), nil
		},
		"decayNumericExp": func(a []any, off int) (any, *painless.RuntimeError) {
			scaling := math.Log(f(a, 3)) / f(a, 1)
			return math.Exp(scaling * dist(a)), nil
		},
		"decayNumericGauss": func(a []any, off int) (any, *painless.RuntimeError) {
			scaling := 0.5 * math.Pow(f(a, 1), 2.0) / math.Log(f(a, 3))
			return math.Exp(0.5 * math.Pow(dist(a), 2.0) / scaling), nil
		},
		"randomScore": func(a []any, off int) (any, *painless.RuntimeError) {
			h := fnv.New32a()
			key := qc.ID
			if len(a) == 2 {
				if vals, ok := qc.Values(painless.ToString(a[1])); ok && len(vals) > 0 {
					key = painless.ToString(painless.FromJSON(vals[0], painless.Hash))
				}
			}
			h.Write([]byte(Itoa(int(painless.I64(a[0]))) + "\x00" + key))
			return float64(h.Sum32()&0x00FFFFFF) / float64(1<<24), nil
		},
	} {
		m[k] = v
	}
	return m
}
