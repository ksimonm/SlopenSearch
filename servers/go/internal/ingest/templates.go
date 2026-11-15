package ingest

import (
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/mustache"
	"slopensearch/internal/ojson"
)

// Template properties of processors (field names, set/append values, the
// fail message, ...) are mustache templates when they contain "{{"
// (ConfigurationUtils#compileTemplate); anything else is taken literally.
// They render through internal/mustache against the document's template
// model.

// templateModel is the mustache model of the document, as
// IngestDocument#createTemplateModel builds it: the source and metadata
// fields, _source (the same fields) and _ingest.
func (e *ingestEnv) templateModel() map[string]any {
	m := make(map[string]any, len(e.source)+len(e.meta)+2)
	for k, v := range e.source {
		m[k] = v
	}
	for k, v := range e.meta {
		if k != "_ingest" {
			m[k] = v
		}
	}
	all := make(map[string]any, len(m))
	for k, v := range m {
		all[k] = v
	}
	m["_source"] = all
	ing := map[string]any{"timestamp": e.timestamp, "pipeline": e.pipeline}
	if extra, ok := e.meta["_ingest"].(map[string]any); ok {
		for k, v := range extra {
			ing[k] = v
		}
	}
	for k, v := range e.onFailure {
		ing[k] = v
	}
	if e.foreachSet != nil {
		ing["_value"] = e.foreachVal
	}
	m["_ingest"] = ing
	return m
}

// templateFailure carries a template that failed to render out of a
// processor; runIngestProcessor recovers it into the processor's error.
type templateFailure struct{ err *apierr.Error }

// recoverTemplateFailure turns a templateFailure panic into *err.
func recoverTemplateFailure(err **apierr.Error) {
	if r := recover(); r != nil {
		f, ok := r.(templateFailure)
		if !ok {
			panic(r)
		}
		*err = f.err
	}
}

// render renders a template property. A failed value lookup fails the
// processor (see templateFailure); a template that does not compile (only
// possible for properties OpenSearch does not compile at pipeline creation)
// is taken literally.
func (e *ingestEnv) render(s string) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	out, err := mustache.Render(s, e.templateModel())
	if err != nil {
		if err.Lookup != nil {
			panic(templateFailure{templateError(s, err, nil)})
		}
		return s
	}
	return out
}

// renderValue renders a templated value (ValueSource): strings, list
// elements, map keys and map values.
func (e *ingestEnv) renderValue(v any) any {
	switch t := v.(type) {
	case string:
		return e.render(t)
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = e.renderValue(x)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[e.render(k)] = e.renderValue(x)
		}
		return out
	}
	return v
}

// templateProperties are the properties OpenSearch compiles as templates when
// a pipeline is created; named ones report their property_name on errors.
var templateProperties = map[string][]struct {
	name  string
	named bool
}{
	"set":    {{"field", true}, {"value", false}},
	"append": {{"field", true}, {"value", false}},
	"fail":   {{"message", true}},
	"rename": {{"field", true}, {"target_field", true}},
}

// checkTemplates reports the first template property of a processor that
// does not compile.
func checkTemplates(typ string, cfg map[string]any) *apierr.Error {
	for _, p := range templateProperties[typ] {
		v, ok := cfg[p.name]
		if !ok {
			continue
		}
		var texts []string
		collectStrings(v, &texts)
		for _, t := range texts {
			if !strings.Contains(t, "{{") {
				continue
			}
			if err := mustache.Compile(t); err != nil {
				fields := ojson.Object{{"processor_type", typ}}
				if tag, ok := cfg["tag"].(string); ok {
					fields = append(fields, ojson.KV{"processor_tag", tag})
				}
				if p.named {
					fields = append(fields, ojson.KV{"property_name", p.name})
				}
				return templateError(t, err, fields)
			}
		}
	}
	return nil
}

func collectStrings(v any, out *[]string) {
	switch t := v.(type) {
	case string:
		*out = append(*out, t)
	case []any:
		for _, x := range t {
			collectStrings(x, out)
		}
	case map[string]any:
		for k, x := range t {
			*out = append(*out, k)
			collectStrings(x, out)
		}
	}
}

// templateError is the error of a template: a failed value lookup is a
// mustache_exception (500) with its cause chain; a compile error is the
// script_exception (400) of a mustache script, with the processor fields.
func templateError(src string, e *mustache.Error, fields ojson.Object) *apierr.Error {
	if l := e.Lookup; l != nil {
		return &apierr.Error{Type: "mustache_exception", Reason: e.Reason, Status: 500,
			CausedBy: &apierr.Error{Type: "mustache_exception", Reason: l.Cause,
				CausedBy: &apierr.Error{Type: l.JavaType, Reason: l.JavaCause}}}
	}
	head := ojson.Object{{"type", "script_exception"}, {"reason", e.Reason}}
	head = append(head, fields...)
	head = append(head, ojson.KV{"script_stack", []any{}}, ojson.KV{"script", src}, ojson.KV{"lang", "mustache"})
	top := append(ojson.Object{{"root_cause", []any{head}}}, head...)
	top = append(top, ojson.KV{"caused_by", ojson.Object{{"type", "mustache_exception"}, {"reason", e.Reason}}})
	flds := map[string]any{"script_stack": []any{}, "script": src, "lang": "mustache"}
	for _, kv := range fields {
		flds[kv.K] = kv.V
	}
	return &apierr.Error{Type: "script_exception", Reason: e.Reason, Status: 400, Fields: flds,
		CausedBy: &apierr.Error{Type: "mustache_exception", Reason: e.Reason},
		Shard:    rawError{body: ojson.Object{{"error", top}, {"status", 400}}}}
}

// rawError is a pre-rendered error response (exact key order).
type rawError struct{ body ojson.Object }

func (r rawError) StatusCode() int    { return 400 }
func (r rawError) ResponseBody() any { return r.body }
