package mapping

// mergeMappingsDeep merges src into dst; values in src win.
func MergeDeep(dst, src map[string]any) {
	for k, v := range src {
		if k == "properties" {
			sv, ok := v.(map[string]any)
			if !ok {
				dst[k] = v
				continue
			}
			dv, ok := dst["properties"].(map[string]any)
			if !ok {
				dv = map[string]any{}
				dst["properties"] = dv
			}
			for fk, fv := range sv {
				dfv, ok := dv[fk].(map[string]any)
				sfv, ok2 := fv.(map[string]any)
				if ok && ok2 {
					MergeDeep(dfv, sfv)
				} else {
					dv[fk] = fv
				}
			}
			continue
		}
		dst[k] = v
	}
}
