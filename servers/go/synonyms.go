package main

import (
	"sort"
	"strings"
)

// Synonym token filters: rule parsing like Lucene's SolrSynonymParser and
// WordnetSynonymParser, and the matching/output behaviour of SynonymFilter
// (type "synonym") and SynonymGraphFilter (type "synonym_graph").

type synonymEntry struct {
	outputs  [][]string
	keepOrig bool
}

type synonymMap struct {
	entries map[string]*synonymEntry
	maxLen  int
}

func synKey(words []string) string { return strings.Join(words, "\x00") }

func (m *synonymMap) add(input, output []string, includeOrig bool) {
	k := synKey(input)
	e := m.entries[k]
	if e == nil {
		e = &synonymEntry{}
		m.entries[k] = e
	}
	e.keepOrig = e.keepOrig || includeOrig
	for _, o := range e.outputs {
		if synKey(o) == synKey(output) {
			return
		}
	}
	e.outputs = append(e.outputs, output)
	if len(input) > m.maxLen {
		m.maxLen = len(input)
	}
}

func synonymsError() *apiError {
	return filterError("Failed to build synonyms")
}

// analyzeSynonymPhrase runs a rule phrase through the rule analyzer.
func analyzeSynonymPhrase(analyze func(string) []analyzedToken, phrase string) ([]string, bool) {
	var toks []analyzedToken
	if analyze != nil {
		toks = analyze(phrase)
	} else {
		for i, w := range strings.Fields(phrase) {
			toks = append(toks, analyzedToken{term: w, pos: i})
		}
	}
	out := make([]string, 0, len(toks))
	last := -1
	for _, t := range toks {
		if t.pos != last+1 {
			// Rules may not analyze to holes or stacked tokens.
			return nil, false
		}
		last = t.pos
		out = append(out, t.term)
	}
	return out, len(out) > 0
}

func parseSynonymRules(p map[string]any, analyze func(string) []analyzedToken) (*synonymMap, *apiError) {
	if fsString(p, "synonyms_path", "") != "" {
		return nil, filterError("IOException while reading synonyms_path_path: file not readable")
	}
	lines := fsStrings(p, "synonyms")
	expand := fsBool(p, "expand", true)
	lenient := fsBool(p, "lenient", false)
	m := &synonymMap{entries: map[string]*synonymEntry{}}
	addGroup := func(inputs [][]string) {
		if expand {
			for i := range inputs {
				for j := range inputs {
					if i != j {
						m.add(inputs[i], inputs[j], true)
					}
				}
			}
			return
		}
		for i := range inputs {
			m.add(inputs[i], inputs[0], false)
		}
	}
	phrases := func(s string) ([][]string, bool) {
		var out [][]string
		for _, part := range splitSynonymList(s) {
			words, ok := analyzeSynonymPhrase(analyze, part)
			if !ok {
				return nil, false
			}
			out = append(out, words)
		}
		return out, true
	}
	if fsString(p, "format", "") == "wordnet" {
		var group [][]string
		lastID := ""
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "s(") {
				continue
			}
			id, word, ok := parseWordnetLine(line)
			if !ok {
				if lenient {
					continue
				}
				return nil, synonymsError()
			}
			if id != lastID && len(group) > 0 {
				if len(group) > 1 {
					addGroup(group)
				}
				group = nil
			}
			lastID = id
			words, ok := analyzeSynonymPhrase(analyze, word)
			if !ok {
				if lenient {
					continue
				}
				return nil, synonymsError()
			}
			group = append(group, words)
		}
		if len(group) > 1 {
			addGroup(group)
		}
		return m, nil
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		sides := splitUnescaped(line, "=>")
		switch len(sides) {
		case 1:
			inputs, ok := phrases(sides[0])
			if !ok || len(inputs) == 0 {
				if lenient {
					continue
				}
				return nil, synonymsError()
			}
			addGroup(inputs)
		case 2:
			inputs, ok1 := phrases(sides[0])
			outputs, ok2 := phrases(sides[1])
			if !ok1 || !ok2 || len(inputs) == 0 || len(outputs) == 0 {
				if lenient {
					continue
				}
				return nil, synonymsError()
			}
			for _, in := range inputs {
				for _, out := range outputs {
					m.add(in, out, false)
				}
			}
		default:
			if lenient {
				continue
			}
			return nil, synonymsError()
		}
	}
	return m, nil
}

// splitUnescaped splits s on sep, ignoring backslash escaped separators.
func splitUnescaped(s, sep string) []string {
	var out []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			cur.WriteByte(s[i])
			cur.WriteByte(s[i+1])
			i++
			continue
		}
		if strings.HasPrefix(s[i:], sep) {
			out = append(out, cur.String())
			cur.Reset()
			i += len(sep) - 1
			continue
		}
		cur.WriteByte(s[i])
	}
	return append(out, cur.String())
}

// splitSynonymList splits a comma separated phrase list and unescapes it.
func splitSynonymList(s string) []string {
	var out []string
	for _, part := range splitUnescaped(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var b strings.Builder
		for i := 0; i < len(part); i++ {
			if part[i] == '\\' && i+1 < len(part) {
				i++
			}
			b.WriteByte(part[i])
		}
		out = append(out, b.String())
	}
	return out
}

func parseWordnetLine(line string) (string, string, bool) {
	inner := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(line), "."), ")")
	inner = strings.TrimPrefix(inner, "s(")
	comma := strings.Index(inner, ",")
	if comma < 0 {
		return "", "", false
	}
	id := inner[:comma]
	q1 := strings.Index(inner, "'")
	if q1 < 0 {
		return "", "", false
	}
	var b strings.Builder
	i := q1 + 1
	for i < len(inner) {
		if inner[i] == '\'' {
			if i+1 < len(inner) && inner[i+1] == '\'' {
				b.WriteByte('\'')
				i += 2
				continue
			}
			break
		}
		b.WriteByte(inner[i])
		i++
	}
	return id, b.String(), true
}

// longestSynonymMatch returns the matched entry and input length at token i.
func (m *synonymMap) longestMatch(toks []analyzedToken, i int) (*synonymEntry, int) {
	for l := m.maxLen; l >= 1; l-- {
		if i+l > len(toks) {
			continue
		}
		words := make([]string, l)
		for k := 0; k < l; k++ {
			words[k] = toks[i+k].term
		}
		if e, ok := m.entries[synKey(words)]; ok {
			return e, l
		}
	}
	return nil, 0
}

func synonymFilter(toks []analyzedToken, spec filterSpec, graph bool) ([]analyzedToken, *apiError) {
	m, err := parseSynonymRules(spec.params, spec.ruleAnalyzer)
	if err != nil {
		return nil, err
	}
	if len(m.entries) == 0 || len(toks) == 0 {
		return toks, nil
	}
	if graph {
		return synonymGraphApply(m, toks), nil
	}
	return synonymLegacyApply(m, toks), nil
}

// synonymLegacyApply mirrors SynonymFilter: outputs are laid onto the
// following input slots.
func synonymLegacyApply(m *synonymMap, toks []analyzedToken) []analyzedToken {
	type slot struct {
		orig    *analyzedToken
		inc     int
		outputs []analyzedToken
	}
	slots := make([]slot, len(toks))
	for i := range toks {
		inc := toks[i].pos + 1
		if i > 0 {
			inc = toks[i].pos - toks[i-1].pos
		}
		slots[i].inc = inc
	}
	addOut := func(idx int, t analyzedToken) {
		for idx >= len(slots) {
			slots = append(slots, slot{inc: 1})
		}
		slots[idx].outputs = append(slots[idx].outputs, t)
	}
	for i := 0; i < len(toks); {
		e, l := m.longestMatch(toks, i)
		if e == nil {
			t := toks[i]
			slots[i].orig = &t
			i++
			continue
		}
		if e.keepOrig {
			for k := 0; k < l; k++ {
				t := toks[i+k]
				slots[i+k].orig = &t
			}
		}
		matchEnd := toks[i+l-1].end
		for _, out := range e.outputs {
			for j, w := range out {
				at := i + j
				src := toks[len(toks)-1]
				if at < len(toks) {
					src = toks[at]
				}
				t := analyzedToken{term: w, start: src.start, end: src.end, typ: "SYNONYM"}
				if len(out) == 1 {
					t.end = matchEnd
					if e.keepOrig {
						t.posLen = l
					}
				}
				addOut(at, t)
			}
		}
		i += l
	}
	out := []analyzedToken{}
	pos := -1
	for _, s := range slots {
		if s.orig == nil && len(s.outputs) == 0 {
			continue
		}
		if s.orig != nil {
			pos += s.inc
			t := *s.orig
			t.pos = pos
			out = append(out, t)
		} else {
			pos++
		}
		for _, t := range s.outputs {
			t.pos = pos
			out = append(out, t)
		}
	}
	return out
}

// synonymGraphApply mirrors SynonymGraphFilter: every rule output becomes its
// own path between the match's start and end nodes.
func synonymGraphApply(m *synonymMap, toks []analyzedToken) []analyzedToken {
	out := []analyzedToken{}
	node := -1
	for i := 0; i < len(toks); {
		inc := toks[i].pos + 1
		if i > 0 {
			inc = toks[i].pos - toks[i-1].pos
		}
		e, l := m.longestMatch(toks, i)
		if e == nil {
			t := toks[i]
			node += inc
			t.pos = node
			out = append(out, t)
			i++
			continue
		}
		start := node + inc
		total := 0
		if e.keepOrig {
			total = l - 1
		}
		for _, o := range e.outputs {
			total += len(o) - 1
		}
		end := start + total + 1
		matchStart, matchEnd := toks[i].start, toks[i+l-1].end
		type bt struct {
			tok        analyzedToken
			start, end int
		}
		var buf []bt
		newNodes := 0
		for _, o := range e.outputs {
			pathEnd := end
			if len(o) > 1 {
				pathEnd = start + newNodes + 1
				newNodes += len(o) - 1
			}
			buf = append(buf, bt{analyzedToken{term: o[0], start: matchStart, end: matchEnd, typ: "SYNONYM"}, start, pathEnd})
			for k := 1; k < len(o); k++ {
				ps := pathEnd
				if k == len(o)-1 {
					pathEnd = end
				} else {
					pathEnd++
				}
				buf = append(buf, bt{analyzedToken{term: o[k], start: matchStart, end: matchEnd, typ: "SYNONYM"}, ps, pathEnd})
			}
		}
		if e.keepOrig {
			prev := start
			for k := 0; k < l; k++ {
				ie := end
				if k < l-1 {
					newNodes++
					ie = start + newNodes
				}
				buf = append(buf, bt{toks[i+k], prev, ie})
				prev = ie
			}
		}
		sort.SliceStable(buf, func(a, b int) bool { return buf[a].start < buf[b].start })
		for _, b := range buf {
			t := b.tok
			t.pos = b.start
			t.posLen = b.end - b.start
			out = append(out, t)
		}
		node = end - 1
		i += l
	}
	return out
}
