package mapping

import (
	"fmt"
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
)

type CompletionContextDef struct {
	Name      string
	Typ       string // "category" or "geo"
	Path      string
	Precision int
}

func CompletionContextDefs(def map[string]any) []CompletionContextDef {
	list, _ := def["contexts"].([]any)
	var out []CompletionContextDef
	for _, raw := range list {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		c := CompletionContextDef{Precision: 6}
		c.Name, _ = jsonv.ToString(m["name"])
		t, _ := jsonv.ToString(m["type"])
		c.Typ = strings.ToLower(t)
		c.Path, _ = jsonv.ToString(m["path"])
		if p, ok := jsonv.ToFloat(m["precision"]); ok {
			c.Precision = int(p)
		}
		out = append(out, c)
	}
	return out
}

func CompletionContextNames(ctxs []CompletionContextDef) string {
	names := make([]string, len(ctxs))
	for i, c := range ctxs {
		names[i] = c.Name
	}
	return strings.Join(names, ", ")
}

// validateCompletionMapping checks the completion specific mapping parameters.
func validateCompletionMapping(def map[string]any) *validationError {
	if v, has := def["max_input_length"]; has {
		if n, ok := intFromAny(v); ok && n <= 0 {
			return mve(fmt.Sprintf("[max_input_length] must be > 0 but was [%d]", n), "")
		}
	}
	if raw, has := def["contexts"]; has {
		list, ok := raw.([]any)
		if !ok {
			list = []any{raw}
		}
		for _, e := range list {
			m, ok := e.(map[string]any)
			if !ok {
				return mve("expected map or list of maps for [contexts] but got "+PreviewValue(e), "")
			}
			if _, ok := m["name"]; !ok {
				return mve("missing [name] in context mapping", "")
			}
			t, ok := jsonv.ToString(m["type"])
			if !ok {
				return mve("missing [type] in context mapping", "")
			}
			switch strings.ToLower(t) {
			case "category", "geo":
			default:
				return mve("No context type for ["+t+"]", "")
			}
		}
	}
	return nil
}

// normalizeCompletionMapping fills the implicit completion parameters so the
// mapping renders like OpenSearch's.
func normalizeCompletionMapping(def map[string]any) {
	if _, has := def["analyzer"]; !has {
		def["analyzer"] = "simple"
	}
	for _, k := range []string{"preserve_separators", "preserve_position_increments"} {
		if _, has := def[k]; !has {
			def[k] = true
		}
	}
	if _, has := def["max_input_length"]; !has {
		def["max_input_length"] = 50
	}
	if list, ok := def["contexts"].([]any); ok {
		for _, e := range list {
			m, ok := e.(map[string]any)
			if !ok {
				continue
			}
			if t, ok := m["type"].(string); ok {
				m["type"] = strings.ToUpper(t)
				if strings.EqualFold(t, "geo") {
					if _, has := m["precision"]; !has {
						m["precision"] = 6
					}
				}
			}
		}
	}
}

func completionParseError(causeType, reason string) *apierr.Error {
	e := mapperErr("mapper_parsing_exception", "failed to parse", causeType, reason)
	if causeType == "parsing_exception" {
		e.RootCause = &apierr.Error{Type: causeType, Reason: reason}
	}
	return e
}

// checkCompletionContexts validates the contexts of a completion value at
// index time against the field's context mappings.
func (c *ParseContext) checkCompletionContexts(path string, def map[string]any, v any) *apierr.Error {
	ctxs := CompletionContextDefs(def)
	known := map[string]bool{}
	for _, cd := range ctxs {
		known[cd.Name] = true
	}
	count := 0
	var visit func(v any) *apierr.Error
	visit = func(v any) *apierr.Error {
		switch t := v.(type) {
		case []any:
			for _, e := range t {
				if err := visit(e); err != nil {
					return err
				}
			}
		case map[string]any:
			if cm, ok := t["contexts"].(map[string]any); ok {
				for _, name := range jsonv.SortedKeys(cm) {
					if !known[name] {
						return completionParseError("illegal_argument_exception",
							"Unknown context name ["+name+"], must be one of ["+CompletionContextNames(ctxs)+"]")
					}
					count += len(CompletionValueList(cm[name]))
				}
			}
		}
		return nil
	}
	if err := visit(v); err != nil {
		return err
	}
	if len(ctxs) == 0 {
		return nil
	}
	for _, cd := range ctxs {
		if cd.Path != "" && c.Doc != nil {
			if pv, ok := FieldValue(c.Doc, cd.Path); ok {
				count += len(CompletionValueList(pv))
			}
		}
	}
	if count == 0 {
		return &apierr.Error{Type: "illegal_argument_exception", Status: 400,
			Reason: "Contexts are mandatory in context enabled completion field [" + path + "]"}
	}
	return nil
}

func CompletionValueList(v any) []any {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		return t
	}
	return []any{v}
}
