package search

import (
	"slopensearch/internal/model"
)

// CollectNamedQueries adds the _name of every query clause matching qc's
// document to out, with its score.
func CollectNamedQueries(q map[string]any, qc *QueryCtx, out map[string]float64) {
	walkNamedClause(q, qc, out)
}

func walkNamedClause(cm map[string]any, qc *QueryCtx, out map[string]float64) {
	if cm == nil {
		return
	}
	name, _ := cm["_name"].(string)
	for k, v := range cm {
		switch k {
		case "_name", "boost":
			continue
		case "bool":
			bm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			for _, key := range []string{"must", "should", "filter", "must_not"} {
				for _, item := range AsQueryList(bm[key]) {
					if child, ok := item.(map[string]any); ok {
						walkNamedClause(child, qc, out)
					}
				}
			}
			nm := name
			if n, ok := bm["_name"].(string); ok {
				nm = n
			}
			if nm != "" {
				matched, score, err := qc.Eval(cm)
				if err == nil && matched {
					out[nm] = score
				}
			}
		default:
			nm := name
			if opt, ok := v.(map[string]any); ok {
				if n, ok := opt["_name"].(string); ok {
					nm = n
				}
			}
			if fm, ok := v.(map[string]any); ok {
				for _, fv := range fm {
					if opt, ok := fv.(map[string]any); ok {
						if n, ok := opt["_name"].(string); ok {
							nm = n
						}
					}
				}
			}
			if nm != "" {
				evalMap := cm
				if _, has := cm["_name"]; has {
					evalMap = map[string]any{}
					for ck, cv := range cm {
						if ck != "_name" {
							evalMap[ck] = cv
						}
					}
				}
				matched, score, err := qc.Eval(evalMap)
				if err == nil && matched {
					out[nm] = score
				}
			}
		}
	}
}

// MappedLeafFields lists the leaf field paths of the mappings of idx.
func MappedLeafFields(idx *model.Index) []string {
	out := []string{}
	var walk func(prefix string, props map[string]any)
	walk = func(prefix string, props map[string]any) {
		for name, raw := range props {
			def, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			path := name
			if prefix != "" {
				path = prefix + "." + name
			}
			if sub, ok := def["properties"].(map[string]any); ok {
				walk(path, sub)
				continue
			}
			out = append(out, path)
			if fields, ok := def["fields"].(map[string]any); ok {
				for sub := range fields {
					out = append(out, path+"."+sub)
				}
			}
		}
	}
	props, _ := idx.Mappings["properties"].(map[string]any)
	walk("", props)
	return out
}
