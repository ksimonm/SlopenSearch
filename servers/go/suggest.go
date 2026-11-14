package main

import ()

func evalSuggest(suggest map[string]any, indices []*Index, typed bool, searchBody map[string]any) (map[string]any, *apiError) {
	out := map[string]any{}
	globalText := suggest["text"]
	for name, raw := range suggest {
		if name == "text" {
			continue
		}
		spec, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := spec["term"].(map[string]any); ok {
			key := name
			if typed {
				key = "term#" + name
			}
			entries, err := termSuggester(spec, t, indices, globalText)
			if err != nil {
				return nil, err
			}
			out[key] = entries
		} else if t, ok := spec["phrase"].(map[string]any); ok {
			key := name
			if typed {
				key = "phrase#" + name
			}
			entries, err := phraseSuggest(spec, t, indices, globalText)
			if err != nil {
				return nil, err
			}
			out[key] = entries
		} else if t, ok := spec["completion"].(map[string]any); ok {
			key := name
			if typed {
				key = "completion#" + name
			}
			entries, err := completionSuggest(spec, t, indices, globalText, searchBody)
			if err != nil {
				return nil, err
			}
			out[key] = entries
		}
	}
	return out, nil
}

func suggestText(spec map[string]any) string {
	if v, ok := spec["text"]; ok {
		s, _ := toString(v)
		return s
	}
	if v, ok := spec["prefix"]; ok {
		s, _ := toString(v)
		return s
	}
	return ""
}
