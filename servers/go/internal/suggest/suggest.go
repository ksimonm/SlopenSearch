// Package suggest implements the suggesters of the search API: term,
// phrase (with collate) and completion (with contexts and fuzzy matching).
package suggest

import (
	"slopensearch/internal/apierr"
	"slopensearch/internal/model"
)

// Evaluate runs the suggesters of a search request (spec is the "suggest"
// object) over indices; typed renders typed_keys names.
func Evaluate(suggest map[string]any, indices []*model.Index, typed bool, searchBody map[string]any) (map[string]any, *apierr.Error) {
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
