package main

import "strings"

// Highlighting of text fields through their analyzers: the document value is
// analyzed with the field's index analyzer and every token whose term is a
// query term is marked, at the token's offsets. Query text of full-text
// queries is analyzed with the field's search analyzer, like the query
// itself; term-level values are taken as they are.

type hlQueryText struct {
	text  string
	opts  any
	exact bool
}

// queryHighlightTexts collects the query texts of the clauses the
// highlighter extracts terms from (see queryHighlightInfo).
func queryHighlightTexts(q map[string]any) []hlQueryText {
	var out []hlQueryText
	var walk func(m map[string]any)
	walk = func(m map[string]any) {
		for clause, raw := range m {
			mm, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			switch clause {
			case "match", "match_phrase", "term":
				for field, val := range mm {
					if field == "_name" || field == "boost" {
						continue
					}
					opts := any(nil)
					if vm, ok := val.(map[string]any); ok {
						opts = vm
						val = vm["value"]
						if val == nil {
							val = vm["query"]
						}
					}
					text, _ := queryText(val)
					out = append(out, hlQueryText{text: text, opts: opts, exact: clause == "term"})
				}
			case "terms":
				for field, val := range mm {
					if field == "_name" || field == "boost" {
						continue
					}
					if arr, ok := val.([]any); ok {
						for _, e := range arr {
							text, _ := toString(e)
							out = append(out, hlQueryText{text: text, exact: true})
						}
					}
				}
			case "multi_match":
				text, _ := toString(mm["query"])
				out = append(out, hlQueryText{text: text, opts: mm})
			case "query_string", "simple_query_string":
				text, _ := toString(mm["query"])
				clean := strings.NewReplacer("(", " ", ")", " ", ":", " ", "\"", " ", "AND", " ", "OR", " ", "NOT", " ").Replace(text)
				out = append(out, hlQueryText{text: clean, opts: mm})
			case "bool":
				for _, key := range []string{"must", "should", "filter", "must_not"} {
					for _, item := range asQueryList(mm[key]) {
						if child, ok := item.(map[string]any); ok {
							walk(child)
						}
					}
				}
			}
		}
	}
	walk(q)
	return out
}

// hlAnalyzedSpans marks the tokens of a text field value matching the query.
func hlAnalyzedSpans(idx *Index, field, text string, query map[string]any) []hlSpan {
	qc := &queryCtx{idx: idx}
	terms := map[string]bool{}
	for _, qt := range queryHighlightTexts(query) {
		if qt.exact {
			terms[qt.text] = true
			continue
		}
		for _, t := range analyzeTokensWithIndex(idx, qc.searchAnalyzerName(field, qt.opts), qt.text) {
			terms[t.term] = true
		}
	}
	if len(terms) == 0 {
		return nil
	}
	toks := analyzeTokensWithIndex(idx, qc.indexAnalyzerName(field), text)
	// UTF-16 offset -> byte offset.
	byteAt := make([]int, 0, len(text)+1)
	for i, r := range text {
		for k := 0; k < runeWidth(r); k++ {
			byteAt = append(byteAt, i)
		}
	}
	byteAt = append(byteAt, len(text))
	conv := func(u int) int {
		if u < 0 {
			return 0
		}
		if u >= len(byteAt) {
			return len(text)
		}
		return byteAt[u]
	}
	spans := []hlSpan{}
	last := -1
	for _, t := range toks {
		if !terms[t.term] {
			continue
		}
		s, e := conv(t.start), conv(t.end)
		if s < last || e <= s {
			continue
		}
		spans = append(spans, hlSpan{s, e})
		last = e
	}
	return spans
}
