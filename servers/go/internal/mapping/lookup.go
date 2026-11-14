package mapping

import (
	"strings"

	"slopensearch/internal/analysis"
	"slopensearch/internal/model"
)

func ResolveAliasField(idx *model.Index, field string) string {
	for i := 0; i < 10; i++ {
		def, ok := FieldDef(idx, field)
		if !ok {
			return field
		}
		if t, _ := def["type"].(string); t == "alias" {
			path, _ := def["path"].(string)
			if path == "" || path == field {
				return field
			}
			field = path
			continue
		}
		return field
	}
	return field
}

// mappingFieldDef looks up a field definition by dotted path.
func FieldDef(idx *model.Index, field string) (map[string]any, bool) {
	if derived, ok := idx.Mappings["derived"].(map[string]any); ok {
		if d, ok := derived[field].(map[string]any); ok {
			return d, true
		}
	}
	props, _ := idx.Mappings["properties"].(map[string]any)
	if props == nil {
		return nil, false
	}
	parts := strings.Split(field, ".")
	cur := props
	for i := 0; i < len(parts); i++ {
		// literal dotted key takes precedence
		if i+1 < len(parts) {
			joined := strings.Join(parts[i:], ".")
			if def, ok := cur[joined].(map[string]any); ok {
				return def, true
			}
		}
		def, ok := cur[parts[i]].(map[string]any)
		if !ok {
			return nil, false
		}
		if i == len(parts)-1 {
			return def, true
		}
		if fields, ok := def["fields"].(map[string]any); ok && i+1 == len(parts)-1 {
			if md, ok := fields[parts[i+1]].(map[string]any); ok {
				return md, true
			}
		}
		sub, ok := def["properties"].(map[string]any)
		if !ok {
			return nil, false
		}
		cur = sub
	}
	return nil, false
}

func MatchAnalyzerExists(idx *model.Index, name string) bool {
	if analysis.BuiltinAnalyzer(name) {
		return true
	}
	if idx != nil {
		return settingsHasAnalyzer(idx.Settings, "analyzer", name)
	}
	return false
}
