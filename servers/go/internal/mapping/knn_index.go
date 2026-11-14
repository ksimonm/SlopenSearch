package mapping

import (
	"strings"

	"slopensearch/internal/apierr"

	"slopensearch/internal/model"
	"slopensearch/internal/ojson"
	"slopensearch/internal/settings"
)

// ---------------------------------------------------------------------------
// knn_vector values at index time
// ---------------------------------------------------------------------------

// KnnPutMappingCheck validates knn_vector fields of a put-mapping request and
// rejects updates of their fixed parameters.
func KnnPutMappingCheck(idx *model.Index, existing, incoming map[string]any) *apierr.Error {
	if IndexKnnEnabled(idx) {
		if err := KnnIndexMappings(incoming); err != nil {
			return err
		}
	}
	return knnMergeConflicts(existing, incoming)
}

func knnMergeConflicts(existing, incoming map[string]any) *apierr.Error {
	for _, name := range SortedAnyKeys(incoming) {
		nd, ok := incoming[name].(map[string]any)
		if !ok {
			continue
		}
		od, ok := existing[name].(map[string]any)
		if !ok {
			continue
		}
		if np, ok := nd["properties"].(map[string]any); ok {
			if op, ok := od["properties"].(map[string]any); ok {
				if err := knnMergeConflicts(op, np); err != nil {
					return err
				}
			}
		}
		if t1, _ := od["type"].(string); t1 != "knn_vector" {
			continue
		}
		if t2, _ := nd["type"].(string); t2 != "knn_vector" {
			continue
		}
		for _, p := range []string{"dimension", "data_type", "space_type", "mode", "compression_level", "model_id"} {
			ov, oh := od[p]
			nv, nh := nd[p]
			os, ns := "undefined", "undefined"
			if p == "data_type" {
				os, ns = "float", "float"
			}
			if oh {
				os = strings.ToLower(settings.ValueString(ov))
			}
			if nh {
				ns = strings.ToLower(settings.ValueString(nv))
			}
			if os != ns {
				return &apierr.Error{Type: "illegal_argument_exception", Status: 400,
					Reason: "Mapper for [" + name + "] conflicts with existing mapper:\n\tCannot update parameter [" + p + "] from [" + os + "] to [" + ns + "]"}
			}
		}
	}
	return nil
}

var knnFieldKeyOrder = []string{"type", "dimension", "data_type", "mode", "compression_level", "method",
	"model_id", "space_type", "doc_values"}

// KnnOrderMappings returns a rendering copy of a mapping in which
// knn_vector fields list their parameters in OpenSearch's order.
func KnnOrderMappings(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	if t, _ := m["type"].(string); t == "knn_vector" {
		return orderKeys(m, knnFieldKeyOrder, func(k string, x any) any {
			if k == "method" {
				if mm, ok := x.(map[string]any); ok {
					return orderKeys(mm, []string{"engine", "space_type", "name", "parameters"}, nil)
				}
			}
			return x
		})
	}
	sub, ok := m["properties"].(map[string]any)
	if !ok {
		return v
	}
	out := make(map[string]any, len(m))
	for k, x := range m {
		out[k] = x
	}
	cp := make(map[string]any, len(sub))
	for n, d := range sub {
		cp[n] = KnnOrderMappings(d)
	}
	out["properties"] = cp
	return out
}

func orderKeys(m map[string]any, first []string, conv func(string, any) any) ojson.Object {
	out := ojson.Object{}
	seen := map[string]bool{}
	add := func(k string) {
		x := m[k]
		if conv != nil {
			x = conv(k, x)
		}
		out = append(out, ojson.KV{k, x})
		seen[k] = true
	}
	for _, k := range first {
		if _, ok := m[k]; ok {
			add(k)
		}
	}
	for _, k := range SortedAnyKeys(m) {
		if !seen[k] {
			add(k)
		}
	}
	return out
}
