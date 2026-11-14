package mapping

import (
	"slopensearch/internal/model"
)

// FieldAnalyzer is the analyzer of a field: override if set, else the
// mapping's analyzer ("keyword" for keyword fields, "standard" by default).
func FieldAnalyzer(idx *model.Index, field string, override string) string {
	if override != "" {
		return override
	}
	def, ok := FieldDef(idx, field)
	if !ok {
		return "standard"
	}
	if t, _ := def["type"].(string); t == "keyword" {
		return "keyword"
	}
	if a, _ := def["analyzer"].(string); a != "" {
		return a
	}
	return "standard"
}
