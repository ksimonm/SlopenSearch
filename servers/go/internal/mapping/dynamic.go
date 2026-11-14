package mapping

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"slopensearch/internal/analysis"
	"slopensearch/internal/apierr"
	"slopensearch/internal/dates"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/model"
	"slopensearch/internal/wildcard"
)

var validTypes = map[string]bool{
	"text": true, "keyword": true, "integer": true, "long": true, "short": true,
	"byte": true, "float": true, "double": true, "half_float": true,
	"scaled_float": true, "unsigned_long": true, "date": true, "date_nanos": true,
	"boolean": true, "object": true,
	"nested": true, "completion": true, "geo_point": true, "geo_shape": true,
	"ip": true, "binary": true, "wildcard": true, "constant_keyword": true,
	"flat_object": true, "token_count": true, "search_as_you_type": true,
	"rank_feature": true, "rank_features": true, "match_only_text": true,
	"percolator": true, "join": true, "alias": true, "version": true,
	"integer_range": true, "float_range": true, "long_range": true,
	"double_range": true, "date_range": true, "ip_range": true,
	"xy_point": true, "knn_vector": true,
}

func IsCJK(r rune) bool {
	switch {
	case r >= 0x3040 && r <= 0x30FF:
		return true
	case r >= 0x31F0 && r <= 0x31FF:
		return true
	case r >= 0x3400 && r <= 0x4DBF:
		return true
	case r >= 0x4E00 && r <= 0x9FFF:
		return true
	case r >= 0xF900 && r <= 0xFAFF:
		return true
	case r >= 0xFF66 && r <= 0xFF9D:
		return true
	}
	return false
}

func Analyze(text string) []string {
	out := []string{}
	var buf []rune
	flush := func() {
		if len(buf) > 0 {
			out = append(out, strings.ToLower(string(buf)))
			buf = buf[:0]
		}
	}
	for _, r := range text {
		switch {
		case IsCJK(r):
			flush()
			out = append(out, strings.ToLower(string(r)))
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			buf = append(buf, r)
		default:
			flush()
		}
	}
	flush()
	return out
}

func BuildFieldTypes(mappings map[string]any, prefix string, out map[string]string) {
	if prefix == "" {
		if derived, ok := mappings["derived"].(map[string]any); ok {
			for name, raw := range derived {
				if def, ok := raw.(map[string]any); ok {
					if t, ok := def["type"].(string); ok {
						out[name] = t
					}
				}
			}
		}
	}
	props, ok := mappings["properties"].(map[string]any)
	if !ok {
		return
	}
	for name, raw := range props {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		if t, ok := def["type"].(string); ok {
			out[path] = t
		}
		if fields, ok := def["fields"].(map[string]any); ok {
			for fname, fraw := range fields {
				if fdef, ok := fraw.(map[string]any); ok {
					if ft, ok := fdef["type"].(string); ok {
						out[path+"."+fname] = ft
					}
				}
			}
		}
		BuildFieldTypes(def, path, out)
	}
}

// javaMapDebug renders a mapping value the way Java's Map.toString does.
func javaMapDebug(v any) string {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+"="+javaMapDebug(t[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, javaMapDebug(e))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case string:
		return t
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", t)
	}
}

func valueAtPath(doc map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var cur any = doc
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func FieldValue(doc map[string]any, path string) (any, bool) {
	if v, ok := valueAtPath(doc, path); ok {
		return v, true
	}
	parts := strings.Split(path, ".")
	for i := len(parts) - 1; i >= 1; i-- {
		if v, ok := valueAtPath(doc, strings.Join(parts[:i], ".")); ok {
			return v, true
		}
	}
	return nil, false
}

func DynamicString(v any) string {
	switch d := v.(type) {
	case bool:
		if d {
			return "true"
		}
		return "false"
	case string:
		if d == "" {
			return "true"
		}
		return d
	}
	return "true"
}

func RootDynamicString(idx *model.Index) string {
	return DynamicString(idx.Mappings["dynamic"])
}

func inferDynamicValue(idx *model.Index, v any) map[string]any {
	switch t := v.(type) {
	case bool:
		return map[string]any{"type": "boolean"}
	case json.Number:
		// Like Jackson: the token kind decides, so 3.0 and 1e2 are floats.
		if strings.ContainsAny(string(t), ".eE") {
			return map[string]any{"type": "float"}
		}
		if _, err := t.Float64(); err == nil {
			return map[string]any{"type": "long"}
		}
	case float64:
		if t == math.Trunc(t) {
			return map[string]any{"type": "long"}
		}
		return map[string]any{"type": "float"}
	case string:
		return inferDynamicString(idx, t)
	case []any:
		for _, e := range t {
			if e == nil {
				continue
			}
			if d := inferDynamicValue(idx, e); d != nil {
				return d
			}
		}
		return nil
	case map[string]any:
		props := map[string]any{}
		for k, val := range t {
			if d := inferDynamicValue(idx, val); d != nil {
				props[k] = d
			}
		}
		if len(props) == 0 {
			return map[string]any{"type": "object"}
		}
		return map[string]any{"properties": props}
	}
	return nil
}

func inferDynamicString(idx *model.Index, s string) map[string]any {
	if nd, _ := idx.Mappings["numeric_detection"].(bool); nd {
		if _, err := strconv.ParseInt(s, 10, 64); err == nil {
			return map[string]any{"type": "long"}
		}
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			return map[string]any{"type": "float"}
		}
	}
	dateDetection := true
	if b, ok := idx.Mappings["date_detection"].(bool); ok {
		dateDetection = b
	}
	if dateDetection {
		if fmts, ok := idx.Mappings["dynamic_date_formats"].([]any); ok && len(fmts) > 0 {
			for _, f := range fmts {
				fs, _ := f.(string)
				if fs != "" && dates.FormatMatches(fs, s) {
					print := fs
					if i := strings.Index(fs, "||"); i >= 0 {
						print = fs[:i]
					}
					return map[string]any{"type": "date", "format": fs, "print_format": print}
				}
			}
		} else if dates.IsISOLike(s) {
			return map[string]any{"type": "date"}
		} else if dates.IsSlashLike(s) {
			return map[string]any{"type": "date",
				"format":       "yyyy/MM/dd HH:mm:ss||yyyy/MM/dd||epoch_millis",
				"print_format": "yyyy/MM/dd HH:mm:ss"}
		}
	}
	return map[string]any{
		"type": "text",
		"fields": map[string]any{
			"keyword": map[string]any{"type": "keyword", "ignore_above": 256},
		},
	}
}

func dynamicTypeNameOf(def map[string]any) string {
	switch t, _ := def["type"].(string); t {
	case "boolean":
		return "boolean"
	case "long", "integer":
		return "long"
	case "float", "double":
		return "float"
	case "date":
		return "date"
	case "object", "nested":
		return "object"
	}
	if _, ok := def["properties"]; ok {
		return "object"
	}
	return "string"
}

func mappingTypePlaceholderOf(def map[string]any) string {
	switch t, _ := def["type"].(string); t {
	case "text":
		return "text"
	case "keyword":
		return "keyword"
	case "boolean":
		return "boolean"
	case "long", "integer":
		return "long"
	case "float", "double":
		return "float"
	case "date":
		return "date"
	case "object", "nested":
		return "object"
	}
	if _, ok := def["properties"]; ok {
		return "object"
	}
	return "string"
}

func substitutePlaceholders(v any, name, dynType string) any {
	switch t := v.(type) {
	case string:
		s := strings.ReplaceAll(t, "{name}", name)
		return strings.ReplaceAll(s, "{dynamic_type}", dynType)
	case []any:
		for i := range t {
			t[i] = substitutePlaceholders(t[i], name, dynType)
		}
	case map[string]any:
		for k := range t {
			t[k] = substitutePlaceholders(t[k], name, dynType)
		}
	}
	return v
}

func containsDynamicTypePlaceholder(v any) bool {
	switch t := v.(type) {
	case string:
		return strings.Contains(t, "{dynamic_type}")
	case []any:
		for _, e := range t {
			if containsDynamicTypePlaceholder(e) {
				return true
			}
		}
	case map[string]any:
		for _, e := range t {
			if containsDynamicTypePlaceholder(e) {
				return true
			}
		}
	}
	return false
}

func matchDynamicTemplate(idx *model.Index, path, name, dynType string) (map[string]any, bool) {
	templates, _ := idx.Mappings["dynamic_templates"].([]any)
	for _, t := range templates {
		obj, ok := t.(map[string]any)
		if !ok {
			continue
		}
		var spec map[string]any
		for _, v := range obj {
			spec, _ = v.(map[string]any)
			break
		}
		if spec == nil {
			continue
		}
		regex := false
		if mp, ok := spec["match_pattern"].(string); ok && mp == "regex" {
			regex = true
		}
		matchOne := func(p, s string) bool {
			if regex {
				re, err := regexp.Compile(p)
				return err == nil && re.MatchString(s)
			}
			return wildcard.Match(p, s)
		}
		if p, ok := spec["match"].(string); ok && !matchOne(p, name) {
			continue
		}
		if p, ok := spec["unmatch"].(string); ok && matchOne(p, name) {
			continue
		}
		if p, ok := spec["path_match"].(string); ok && !wildcard.Match(p, path) {
			continue
		}
		if p, ok := spec["path_unmatch"].(string); ok && wildcard.Match(p, path) {
			continue
		}
		if mt, ok := spec["match_mapping_type"].(string); ok && mt != dynType {
			continue
		}
		if m, ok := spec["mapping"].(map[string]any); ok {
			return m, true
		}
	}
	return nil, false
}

// DynamicDefFor resolves the mapping for an undeclared field, applying dynamic
// templates and dynamic type detection.
func DynamicDefFor(idx *model.Index, path, name string, raw any) (map[string]any, bool, *apierr.Error) {
	if arr, ok := raw.([]any); ok {
		firstKind := ""
		firstObj := false
		for _, el := range arr {
			if el == nil {
				continue
			}
			inf := inferDynamicValue(idx, el)
			if inf == nil {
				continue
			}
			kind := mappingTypePlaceholderOf(inf)
			isObj := kind == "object"
			if firstKind == "" {
				firstKind, firstObj = kind, isObj
				continue
			}
			if isObj != firstObj {
				return nil, false, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
					Reason: "can't merge a non object mapping [" + name + "] with an object mapping"}
			}
			if !isObj && kind != firstKind {
				from, to := firstKind, kind
				if from == "string" {
					from = "text"
				}
				if to == "string" {
					to = "text"
				}
				return nil, false, &apierr.Error{Type: "illegal_argument_exception", Status: 400,
					Reason: fmt.Sprintf("mapper [%s] cannot be changed from type [%s] to [%s]", name, from, to)}
			}
		}
	}
	inferred := inferDynamicValue(idx, raw)
	if inferred == nil {
		return nil, false, nil
	}
	dynType := dynamicTypeNameOf(inferred)
	placeholder := mappingTypePlaceholderOf(inferred)
	tmpl, matched := matchDynamicTemplate(idx, path, name, dynType)
	var def map[string]any
	if tmpl != nil {
		def, _ = jsonv.DeepCopy(tmpl).(map[string]any)
		if def == nil {
			def = map[string]any{}
		}
		for k := range def {
			def[k] = substitutePlaceholders(def[k], name, placeholder)
		}
		if _, has := def["type"]; !has {
			for k, v := range inferred {
				if _, ok := def[k]; !ok {
					def[k] = v
				}
			}
		}
	} else {
		def = inferred
	}
	if t, _ := def["type"].(string); t == "text" && tmpl != nil && containsDynamicTypePlaceholder(tmpl) {
		delete(def, "doc_values")
	}
	if a, ok := def["analyzer"].(string); ok && a != "" && !MatchAnalyzerExists(idx, a) {
		return nil, matched, &apierr.Error{Type: "mapper_parsing_exception", Status: 400,
			Reason:   "failed to parse",
			CausedBy: &apierr.Error{Type: "illegal_argument_exception", Reason: "analyzer [" + a + "] has not been configured in mappings"}}
	}
	if ct, ok := def["copy_to"].(string); ok {
		def["copy_to"] = []any{ct}
	}
	typ, _ := def["type"].(string)
	if m, ok := raw.(map[string]any); ok && (typ == "object" || typ == "nested" || typ == "") {
		props := map[string]any{}
		for k, v := range m {
			cp := k
			if path != "" {
				cp = path + "." + k
			}
			cd, _, err := DynamicDefFor(idx, cp, k, v)
			if err != nil {
				return nil, matched, err
			}
			if cd != nil {
				props[k] = cd
			}
		}
		if len(props) > 0 {
			def["properties"] = props
		}
	}
	return def, matched, nil
}

func EnsureFieldTypes(idx *model.Index) {
	fresh := map[string]string{}
	BuildFieldTypes(idx.Mappings, "", fresh)
	for path, t := range fresh {
		if t == "alias" {
			if target := ResolveAliasField(idx, path); target != path {
				if tt, ok := fresh[target]; ok {
					fresh[path] = tt
				}
			}
		}
	}
	idx.FieldTypes = fresh
}

// AnalysisIndex is the analysis view of idx (nil for nil).
func AnalysisIndex(idx *model.Index) *analysis.Index {
	if idx == nil {
		return nil
	}
	return &analysis.Index{Settings: idx.Settings}
}
