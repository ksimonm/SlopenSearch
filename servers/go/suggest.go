package main

import (
	"strings"
)

func collectFieldTerms(idx *Index, field string) (map[string]int, []string) {
	freq := map[string]int{}
	order := []string{}
	for _, id := range idx.Order {
		v, ok := fieldValue(idx.Docs[id], field)
		if !ok {
			continue
		}
		for _, s := range docStrings(v) {
			for _, tok := range analyze(s) {
				if _, seen := freq[tok]; !seen {
					order = append(order, tok)
				}
				freq[tok]++
			}
		}
	}
	return freq, order
}

func evalSuggest(suggest map[string]any, indices []*Index, typed bool, searchBody map[string]any) (map[string]any, *apiError) {
	out := map[string]any{}
	var idx *Index
	if len(indices) > 0 {
		idx = indices[0]
	}
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
			out[key] = []any{phraseSuggester(spec, t, idx)}
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

func phraseSuggester(spec, body map[string]any, idx *Index) map[string]any {
	field, _ := toString(body["field"])
	input := suggestText(spec)
	_, order := collectFieldTerms(idx, field)

	tokens := analyze(input)
	corrected := make([]string, 0, len(tokens))
	changed := false
	for _, tok := range tokens {
		best := tok
		bestDist := 1 << 30
		for _, c := range order {
			d := levenshtein(tok, c)
			if d == 0 {
				best = tok
				bestDist = 0
				break
			}
			if d <= 2 && d < bestDist {
				bestDist = d
				best = c
			}
		}
		if best != tok {
			changed = true
		}
		corrected = append(corrected, best)
	}

	options := []any{}
	if changed {
		phrase := strings.Join(corrected, " ")
		options = append(options, map[string]any{"text": phrase, "score": 0.9})
	}
	return map[string]any{"text": input, "offset": 0, "length": len(input), "options": options}
}
