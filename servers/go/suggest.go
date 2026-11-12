package main

import (
	"sort"
	"strings"
	"unicode"
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

func evalSuggest(suggest map[string]any, indices []*Index, typed bool) (map[string]any, *apiError) {
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
			out[key] = []any{completionSuggester(spec, t, idx)}
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

func completionInputs(v any) [][2]any {
	var out [][2]any
	add := func(input string, weight float64) {
		out = append(out, [2]any{input, weight})
	}
	switch t := v.(type) {
	case string:
		add(t, 0)
	case map[string]any:
		w, _ := toFloat(t["weight"])
		addAll := func(in any) {
			switch iv := in.(type) {
			case string:
				add(iv, w)
			case []any:
				for _, e := range iv {
					if s, ok := toString(e); ok {
						add(s, w)
					}
				}
			}
		}
		if in, ok := t["input"]; ok {
			addAll(in)
		}
	case []any:
		for _, e := range t {
			if s, ok := toString(e); ok {
				add(s, 0)
				continue
			}
			if m, ok := e.(map[string]any); ok {
				w, _ := toFloat(m["weight"])
				if in, ok := m["input"]; ok {
					switch iv := in.(type) {
					case string:
						add(iv, w)
					case []any:
						for _, x := range iv {
							if s, ok := toString(x); ok {
								add(s, w)
							}
						}
					}
				}
			}
		}
	}
	return out
}

func completionSuggester(spec, body map[string]any, idx *Index) map[string]any {
	field, _ := toString(body["field"])
	prefix := suggestText(spec)
	def, _ := mappingFieldDef(idx, field)
	preserve := true
	if b, ok := def["preserve_separators"].(bool); ok {
		preserve = b
	}
	maxLen := 50
	if n, ok := toFloat(def["max_input_length"]); ok && int(n) > 0 {
		maxLen = int(n)
	}
	fuzzy := 0.0
	if f, ok := body["fuzzy"].(map[string]any); ok {
		if n, ok := toFloat(f["fuzziness"]); ok {
			fuzzy = n
		}
	} else if f, ok := spec["fuzzy"].(map[string]any); ok {
		if n, ok := toFloat(f["fuzziness"]); ok {
			fuzzy = n
		}
	}
	size := 10
	if n, ok := toFloat(body["size"]); ok {
		size = int(n)
		if size < 0 {
			size = 0
		}
	}
	norm := func(s string) string {
		lower := strings.ToLower(s)
		if preserve {
			return lower
		}
		var b strings.Builder
		for _, r := range lower {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	want := norm(prefix)

	type entry struct {
		text   string
		weight float64
		id     string
	}
	var entries []entry
	for _, id := range idx.Order {
		v, ok := fieldValue(idx.Docs[id], field)
		if !ok {
			continue
		}
		for _, pair := range completionInputs(v) {
			text, _ := pair[0].(string)
			weight, _ := pair[1].(float64)
			if len([]rune(text)) > maxLen {
				text = string([]rune(text)[:maxLen])
			}
			if !strings.HasPrefix(norm(text), want) {
				continue
			}
			score := 1.0
			if weight > 0 {
				score = weight
			}
			if fuzzy > 0 {
				score *= 1.0 + fuzzy*2.0
			}
			entries = append(entries, entry{text: text, weight: score, id: id})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].weight > entries[j].weight })

	options := []any{}
	for i, e := range entries {
		if i >= size {
			break
		}
		options = append(options, map[string]any{"text": e.text, "_score": e.weight, "_id": e.id})
	}
	return map[string]any{"text": prefix, "offset": 0, "length": len(prefix), "options": options}
}
