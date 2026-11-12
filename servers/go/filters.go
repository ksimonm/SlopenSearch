package main

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Token filter framework. Each filter receives the token stream and returns a
// modified stream, mirroring the Lucene token filter contract closely enough
// for the recorded OpenSearch responses.

type filterSpec struct {
	typ    string
	params map[string]any
	// finalOffset is the end offset of the analyzed text (Lucene's final
	// offset), used by filters looking at trailing tokens.
	finalOffset int
	// ruleAnalyzer analyzes synonym rules with the preceding chain.
	ruleAnalyzer func(string) []analyzedToken
}

func subSettings(settings map[string]any, prefix string) map[string]any {
	out := map[string]any{}
	for k, v := range settings {
		rest := ""
		switch {
		case strings.HasPrefix(k, "index."+prefix+"."):
			rest = k[len("index."+prefix+"."):]
		case strings.HasPrefix(k, prefix+"."):
			rest = k[len(prefix+"."):]
		default:
			continue
		}
		if rest != "" && !strings.Contains(rest, ".") {
			out[rest] = v
		}
	}
	if direct, ok := settings["index."+prefix].(map[string]any); ok {
		for k, v := range direct {
			out[k] = v
		}
	}
	if direct, ok := settings[prefix].(map[string]any); ok {
		for k, v := range direct {
			out[k] = v
		}
	}
	return out
}

var knownFilterTypes = map[string]bool{
	"lowercase": true, "uppercase": true, "asciifolding": true, "stop": true,
	"trim": true, "length": true, "reverse": true, "unique": true, "truncate": true,
	"elision": true, "apostrophe": true, "classic": true, "limit": true,
	"pattern_replace": true, "pattern_capture": true, "fingerprint": true,
	"condition": true, "predicate_token_filter": true, "multiplexer": true,
	"dictionary_decompounder": true, "hyphenation_decompounder": true,
	"delimited_payload": true, "delimited_payload_filter": true, "flatten_graph": true,
	"min_hash": true, "hunspell": true, "stemmer_override": true,
	"keyword_repeat": true, "remove_duplicates": true, "keyword_marker": true,
	"porter_stem": true, "english": true, "snowball": true, "stemmer": true,
	"kstem": true, "russian_stem": true, "czech_stem": true, "french_stem": true,
	"german_stem": true, "dutch_stem": true, "brazilian_stem": true,
	"german_normalization": true, "scandinavian_folding": true,
	"scandinavian_normalization": true, "arabic_normalization": true,
	"serbian_normalization": true, "sorani_normalization": true,
	"decimal_digit": true, "turkish_lowercase": true, "irish_lowercase": true,
	"greek_lowercase": true, "shingle": true, "ngram": true, "edge_ngram": true,
	"word_delimiter": true, "word_delimiter_graph": true, "common_grams": true,
	"cjk_width": true, "cjk_bigram": true, "synonym": true, "synonym_graph": true,
}

func globalFilterSpec(name string) (filterSpec, bool) {
	if !knownFilterTypes[name] {
		return filterSpec{}, false
	}
	if name == "english" {
		return filterSpec{typ: "stemmer", params: map[string]any{"language": "english"}}, true
	}
	return filterSpec{typ: name}, true
}

func resolveFilterSpec(idx *Index, c analyzeComponent) (filterSpec, bool) {
	if c.inline != nil {
		typ := c.name
		if t, ok := c.inline["type"].(string); ok && t != "" {
			typ = t
		}
		if !knownFilterTypes[typ] {
			return filterSpec{}, false
		}
		return filterSpec{typ: typ, params: c.inline}, true
	}
	if idx != nil {
		if raw, ok := findSetting(idx.Settings, "analysis.filter."+c.name+".type"); ok {
			typ, _ := raw.(string)
			return filterSpec{typ: typ, params: subSettings(idx.Settings, "analysis.filter."+c.name)}, true
		}
	}
	return globalFilterSpec(c.name)
}

func fsString(p map[string]any, key, def string) string {
	if p != nil {
		if v, ok := p[key].(string); ok {
			return v
		}
	}
	return def
}

func fsBool(p map[string]any, key string, def bool) bool {
	if p != nil {
		switch v := p[key].(type) {
		case bool:
			return v
		case string:
			// Index settings store values as strings.
			switch v {
			case "true":
				return true
			case "false":
				return false
			}
		}
	}
	return def
}

func fsInt(p map[string]any, key string, def int) int {
	if p != nil {
		if n, ok := toFloat(p[key]); ok {
			return int(n)
		}
	}
	return def
}

func fsStrings(p map[string]any, key string) []string {
	if p == nil {
		return nil
	}
	return filterNames(p[key])
}

func filterError(reason string) *apiError {
	return &apiError{Type: "illegal_argument_exception", Status: 400, Reason: reason,
		RootCause: &apiError{Type: "illegal_argument_exception", Status: 400, Reason: reason}}
}

func applyGlobalFilters(toks []analyzedToken, names []string) ([]analyzedToken, bool) {
	comps := make([]analyzeComponent, 0, len(names))
	for _, n := range names {
		comps = append(comps, analyzeComponent{name: n})
	}
	out, err := applyFilterComponents(nil, toks, comps)
	if err != nil {
		return nil, false
	}
	return out, true
}

func applyFilterComponents(idx *Index, toks []analyzedToken, comps []analyzeComponent) ([]analyzedToken, *apiError) {
	cur := toks
	for _, c := range comps {
		spec, ok := resolveFilterSpec(idx, c)
		if !ok {
			return nil, filterError("failed to find global filter under [" + c.name + "]")
		}
		var err *apiError
		cur, err = applyFilterOne(idx, cur, spec)
		if err != nil {
			return nil, err
		}
	}
	return cur, nil
}

func applyNamedFilterChain(idx *Index, toks []analyzedToken, chain string) ([]analyzedToken, *apiError) {
	cur := toks
	for _, name := range strings.Split(chain, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		spec, ok := resolveFilterSpec(idx, analyzeComponent{name: name})
		if !ok {
			return nil, filterError("failed to find global filter under [" + name + "]")
		}
		var err *apiError
		cur, err = applyFilterOne(idx, cur, spec)
		if err != nil {
			return nil, err
		}
	}
	return cur, nil
}

func translateRunes(s string, f func(rune) rune) string {
	rs := []rune(s)
	for i, r := range rs {
		rs[i] = f(r)
	}
	return string(rs)
}

func truncateUTF16(s string, n int) string {
	if n <= 0 {
		return ""
	}
	w := 0
	rs := []rune(s)
	for i, r := range rs {
		if w+runeWidth(r) > n {
			return string(rs[:i])
		}
		w += runeWidth(r)
	}
	return s
}

var defaultEnglishStop = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "but": true, "by": true, "for": true, "if": true, "in": true,
	"into": true, "is": true, "it": true, "no": true, "not": true, "of": true,
	"on": true, "or": true, "such": true, "that": true, "the": true,
	"their": true, "then": true, "there": true, "these": true, "they": true,
	"this": true, "to": true, "was": true, "will": true, "with": true,
}

// stopSetCache holds the predefined sets, built once at startup.
var stopSetCache = func() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for lang, words := range luceneStopwords {
		set := make(map[string]bool, len(words))
		for _, w := range words {
			set[w] = true
		}
		out[lang] = set
	}
	return out
}()

// namedStopLanguages are the _lang_ sets OpenSearch accepts (Analysis.NAMED_STOP_WORDS).
var namedStopLanguages = map[string]bool{
	"arabic": true, "armenian": true, "basque": true, "bengali": true, "brazilian": true,
	"bulgarian": true, "catalan": true, "czech": true, "danish": true, "dutch": true,
	"estonian": true, "finnish": true, "french": true, "galician": true, "german": true,
	"greek": true, "hindi": true, "hungarian": true, "indonesian": true, "irish": true,
	"italian": true, "latvian": true, "lithuanian": true, "norwegian": true, "persian": true,
	"portuguese": true, "romanian": true, "russian": true, "sorani": true, "spanish": true,
	"swedish": true, "thai": true, "turkish": true,
}

// namedStopSet resolves a predefined stopword set such as _french_.
func namedStopSet(name string) (map[string]bool, bool) {
	if name == "_none_" {
		return map[string]bool{}, true
	}
	if name == "_english_" {
		return defaultEnglishStop, true
	}
	if len(name) < 3 || name[0] != '_' || name[len(name)-1] != '_' || !namedStopLanguages[name[1:len(name)-1]] {
		return nil, false
	}
	return languageStopSet(name[1 : len(name)-1])
}

// languageStopSet is the default stopword set of a language analyzer.
func languageStopSet(lang string) (map[string]bool, bool) {
	if lang == "english" {
		return defaultEnglishStop, true
	}
	set, ok := stopSetCache[lang]
	return set, ok
}

func stopWordSet(p map[string]any) map[string]bool {
	words := fsStrings(p, "stopwords")
	if len(words) == 0 {
		return defaultEnglishStop
	}
	set := map[string]bool{}
	for _, w := range words {
		if named, ok := namedStopSet(w); ok {
			for k := range named {
				set[k] = true
			}
			continue
		}
		set[w] = true
	}
	return set
}

var serbianFold = map[rune]string{
	'ć': "c", 'č': "c", 'đ': "dj", 'š': "s", 'ž': "z",
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ж': "z", 'з': "z",
	'и': "i", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r",
	'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "h", 'ц': "c", 'ч': "c", 'ш': "s",
	'ђ': "dj", 'ј': "j", 'љ': "lj", 'њ': "nj", 'ћ': "c", 'џ': "dz",
}

func possessiveStrip(s string) string {
	if strings.HasSuffix(s, "'s") {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "'") {
		return s[:len(s)-1]
	}
	return s
}

func applyFilterOne(idx *Index, toks []analyzedToken, spec filterSpec) ([]analyzedToken, *apiError) {
	if out, err, handled := langFilterOne(toks, spec); handled {
		return out, err
	}
	p := spec.params
	switch spec.typ {
	case "asciifolding":
		preserve := fsBool(p, "preserve_original", false)
		out := make([]analyzedToken, 0, len(toks))
		for _, t := range toks {
			folded := asciiFoldReplacer.Replace(t.term)
			if preserve && folded != t.term {
				ft := t
				ft.term = folded
				out = append(out, ft)
				out = append(out, t)
			} else {
				t.term = folded
				out = append(out, t)
			}
		}
		toks = out
	case "stop":
		if fsString(p, "stopwords_path", "") != "" {
			return nil, filterError("IOException while reading stopwords_path: file not readable")
		}
		words := stopWordSet(p)
		ignoreCase := fsBool(p, "ignore_case", false)
		removeTrailing := fsBool(p, "remove_trailing", true)
		out := make([]analyzedToken, 0, len(toks))
		for i, t := range toks {
			key := t.term
			if ignoreCase {
				key = strings.ToLower(key)
			}
			if words[key] {
				// SuggestStopFilter keeps a final stopword the text ends with.
				if !removeTrailing && i == len(toks)-1 && t.end == spec.finalOffset {
					out = append(out, t)
				}
				continue
			}
			out = append(out, t)
		}
		toks = out
	case "length":
		min := fsInt(p, "min", 0)
		max := fsInt(p, "max", 1<<30)
		out := toks[:0]
		for _, t := range toks {
			n := utf16Offset(t.term)
			if n >= min && n <= max {
				out = append(out, t)
			}
		}
		toks = out
	case "truncate":
		n := fsInt(p, "length", 10)
		for i := range toks {
			toks[i].term = truncateUTF16(toks[i].term, n)
		}
	case "trim":
		for i := range toks {
			toks[i].term = strings.TrimSpace(toks[i].term)
		}
	case "reverse":
		for i := range toks {
			rs := []rune(toks[i].term)
			for a, b := 0, len(rs)-1; a < b; a, b = a+1, b-1 {
				rs[a], rs[b] = rs[b], rs[a]
			}
			toks[i].term = string(rs)
		}
	case "unique":
		// UniqueTokenFilter drops duplicates together with their position
		// increments.
		samePos := fsBool(p, "only_on_same_position", false)
		seen := map[string]bool{}
		out := []analyzedToken{}
		prevIn, pos := -1, -1
		for _, t := range toks {
			inc := t.pos - prevIn
			prevIn = t.pos
			if samePos && inc > 0 {
				seen = map[string]bool{}
			}
			dup := seen[t.term] && (!samePos || inc == 0)
			seen[t.term] = true
			if dup {
				continue
			}
			pos += inc
			t.pos = pos
			out = append(out, t)
		}
		toks = out
	case "limit":
		n := fsInt(p, "max_token_count", 1)
		if len(toks) > n {
			toks = toks[:n]
		}
	case "classic":
		for i := range toks {
			switch toks[i].typ {
			case "<ACRONYM>":
				toks[i].term = strings.ReplaceAll(toks[i].term, ".", "")
			case "<APOSTROPHE>":
				toks[i].term = possessiveStrip(toks[i].term)
			}
		}
	case "pattern_replace":
		return patternReplaceFilter(toks, p)
	case "pattern_capture":
		return patternCaptureFilter(toks, p)
	case "fingerprint":
		return fingerprintFilter(toks, p), nil
	case "condition", "predicate_token_filter":
		return predicateFilter(idx, toks, p, spec.typ == "condition")
	case "multiplexer":
		return multiplexerFilter(idx, toks, p)
	case "dictionary_decompounder":
		return decompounderFilter(toks, p), nil
	case "hyphenation_decompounder":
		if fsString(p, "hyphenation_patterns_path", "") != "" {
			return nil, filterError("Exception while reading hyphenation_patterns_path.")
		}
		return toks, nil
	case "delimited_payload", "delimited_payload_filter":
		delim := fsString(p, "delimiter", "|")
		for i := range toks {
			if at := strings.LastIndex(toks[i].term, delim); at >= 0 {
				toks[i].term = toks[i].term[:at]
			}
		}
	case "flatten_graph":
	case "min_hash":
		return minHashFilter(toks), nil
	case "hunspell":
		return nil, &apiError{Type: "illegal_state_exception", Status: 500,
			Reason:    "Failed to load hunspell dictionary for locale: " + fsString(p, "locale", ""),
			RootCause: &apiError{Type: "illegal_state_exception", Status: 500, Reason: "Failed to load hunspell dictionary for locale: " + fsString(p, "locale", "")}}
	case "keyword_repeat":
		out := make([]analyzedToken, 0, len(toks)*2)
		for _, t := range toks {
			kw := t
			kw.keyword = true
			out = append(out, kw)
			out = append(out, t)
		}
		toks = out
	case "remove_duplicates":
		seen := map[string]bool{}
		out := toks[:0]
		for _, t := range toks {
			key := t.term + "\x00" + strconv.Itoa(t.pos)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, t)
		}
		toks = out
	case "keyword_marker":
		keywords := fsStrings(p, "keywords")
		ignoreCase := fsBool(p, "ignore_case", false)
		pattern := fsString(p, "keywords_pattern", "")
		var re *jRegex
		if pattern != "" {
			var err *apiError
			if re, err = compileJavaRegexAPI(pattern, 0); err != nil {
				return nil, err
			}
		}
		set := map[string]bool{}
		for _, k := range keywords {
			if ignoreCase {
				k = strings.ToLower(k)
			}
			set[k] = true
		}
		for i := range toks {
			key := toks[i].term
			if ignoreCase {
				key = strings.ToLower(key)
			}
			// PatternKeywordMarkerFilter requires the whole term to match.
			if set[key] || (re != nil && re.fullMatch([]rune(toks[i].term))) {
				toks[i].keyword = true
			}
		}
	case "stemmer_override":
		rules := map[string]string{}
		for _, line := range fsStrings(p, "rules") {
			parts := strings.SplitN(line, "=>", 2)
			if len(parts) != 2 {
				continue
			}
			to := strings.TrimSpace(parts[1])
			for _, from := range strings.Split(parts[0], ",") {
				rules[strings.TrimSpace(from)] = to
			}
		}
		for i := range toks {
			if v, ok := rules[toks[i].term]; ok {
				toks[i].term = v
				toks[i].keyword = true
			}
		}
	case "ngram", "edge_ngram":
		var err *apiError
		toks, err = ngramFilter(toks, p, spec.typ == "edge_ngram")
		if err != nil {
			return nil, err
		}
	case "shingle":
		var err *apiError
		toks, err = shingleFilter(toks, p)
		if err != nil {
			return nil, err
		}
	case "word_delimiter", "word_delimiter_graph":
		toks = wordDelimiterFilter(toks, p, spec.typ == "word_delimiter_graph")
	case "common_grams":
		toks = commonGramsFilter(toks, p)
	case "synonym", "synonym_graph":
		var err *apiError
		toks, err = synonymFilter(toks, spec, spec.typ == "synonym_graph")
		if err != nil {
			return nil, err
		}
	default:
		return nil, filterError("failed to find global filter under [" + spec.typ + "]")
	}
	return toks, nil
}

func fingerprintFilter(toks []analyzedToken, p map[string]any) []analyzedToken {
	if len(toks) == 0 {
		return toks
	}
	sep := fsString(p, "separator", " ")
	maxSize := fsInt(p, "max_output_size", 255)
	seen := map[string]bool{}
	terms := []string{}
	for _, t := range toks {
		if !seen[t.term] {
			seen[t.term] = true
			terms = append(terms, t.term)
		}
	}
	sort.Strings(terms)
	joined := strings.Join(terms, sep)
	if utf16Offset(joined) > maxSize {
		return []analyzedToken{}
	}
	end := 0
	for _, t := range toks {
		if t.end > end {
			end = t.end
		}
	}
	return []analyzedToken{{term: joined, start: 0, end: end, pos: 0, typ: "fingerprint"}}
}

func predicateFilter(idx *Index, toks []analyzedToken, p map[string]any, condition bool) ([]analyzedToken, *apiError) {
	source := ""
	if script, ok := p["script"].(map[string]any); ok {
		source, _ = script["source"].(string)
	}
	// Supported predicates: term length comparisons and position parity.
	matches := func(t analyzedToken) bool {
		switch {
		case strings.Contains(source, "getTerm().length() < "):
			n := scriptNumber(source, "getTerm().length() < ")
			return utf16Offset(t.term) < n
		case strings.Contains(source, "getTerm().length() > "):
			n := scriptNumber(source, "getTerm().length() > ")
			return utf16Offset(t.term) > n
		case strings.Contains(source, "getTerm().length() == "):
			n := scriptNumber(source, "getTerm().length() == ")
			return utf16Offset(t.term) == n
		case strings.Contains(source, "getPosition() % 2 == 0"):
			return t.pos%2 == 0
		case strings.Contains(source, "getPosition() % 2 == 1"):
			return t.pos%2 == 1
		}
		return true
	}
	if !condition {
		out := toks[:0]
		for _, t := range toks {
			if matches(t) {
				out = append(out, t)
			}
		}
		return out, nil
	}
	sub := fsStrings(p, "filter")
	out := make([]analyzedToken, 0, len(toks))
	for _, t := range toks {
		if !matches(t) {
			out = append(out, t)
			continue
		}
		part, err := applyNamedFilterChain(idx, []analyzedToken{t}, strings.Join(sub, ","))
		if err != nil {
			return nil, err
		}
		out = append(out, part...)
	}
	return out, nil
}

func scriptNumber(source, marker string) int {
	at := strings.Index(source, marker)
	if at < 0 {
		return 0
	}
	rest := source[at+len(marker):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	n, _ := strconv.Atoi(rest[:end])
	return n
}

func multiplexerFilter(idx *Index, toks []analyzedToken, p map[string]any) ([]analyzedToken, *apiError) {
	chains := fsStrings(p, "filters")
	preserve := fsBool(p, "preserve_original", true)
	out := []analyzedToken{}
	seen := map[string]bool{}
	add := func(ts []analyzedToken) {
		for _, t := range ts {
			key := t.term + "\x00" + strconv.Itoa(t.pos) + "\x00" + strconv.Itoa(t.start) + "\x00" + strconv.Itoa(t.end) + "\x00" + t.typ
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, t)
		}
	}
	if preserve {
		add(toks)
	}
	for _, chain := range chains {
		part, err := applyNamedFilterChain(idx, append([]analyzedToken{}, toks...), chain)
		if err != nil {
			return nil, err
		}
		add(part)
	}
	return out, nil
}

func decompounderFilter(toks []analyzedToken, p map[string]any) []analyzedToken {
	words := fsStrings(p, "word_list")
	minSize := fsInt(p, "min_word_size", 5)
	maxSize := fsInt(p, "max_subword_size", 15)
	out := make([]analyzedToken, 0, len(toks))
	for _, t := range toks {
		out = append(out, t)
		rs := []rune(t.term)
		for i := 0; i < len(rs); i++ {
			for n := maxSize; n >= minSize; n-- {
				if i+n > len(rs) {
					continue
				}
				sub := string(rs[i : i+n])
				for _, w := range words {
					if w == sub {
						nt := t
						nt.term = sub
						out = append(out, nt)
						break
					}
				}
			}
		}
	}
	return out
}

func minHashFilter(toks []analyzedToken) []analyzedToken {
	if len(toks) == 0 {
		return toks
	}
	terms := make([]string, 0, len(toks))
	for _, t := range toks {
		terms = append(terms, t.term)
	}
	sort.Strings(terms)
	sum := sha256.Sum256([]byte(strings.Join(terms, " ")))
	return []analyzedToken{{
		term: hex.EncodeToString(sum[:8]), start: toks[0].start, end: toks[len(toks)-1].end,
		pos: 0, typ: "MIN_HASH",
	}}
}

func ngramFilter(toks []analyzedToken, p map[string]any, edge bool) ([]analyzedToken, *apiError) {
	minGram := fsInt(p, "min_gram", 1)
	defMax := 2
	if edge {
		defMax = 1
	}
	maxGram := fsInt(p, "max_gram", defMax)
	limit := fsInt(p, "_max_ngram_diff", 1)
	if !edge && maxGram-minGram > limit {
		reason := "The difference between max_gram and min_gram in NGram Tokenizer must be less than or equal to: [" + strconv.Itoa(limit) + "] but was [" +
			strconv.Itoa(maxGram-minGram) + "]. This limit can be set by changing the [index.max_ngram_diff] index level setting."
		return nil, filterError(reason)
	}
	if minGram < 1 {
		return nil, filterError("minGram must be greater than zero")
	}
	if minGram > maxGram {
		return nil, filterError("minGram must not be greater than maxGram")
	}
	preserve := fsBool(p, "preserve_original", false)
	back := fsString(p, "side", "front") == "back"
	out := make([]analyzedToken, 0, len(toks))
	for _, t := range toks {
		rs := []rune(t.term)
		base := t
		if edge {
			for n := minGram; n <= maxGram && n <= len(rs); n++ {
				nt := base
				if back {
					nt.term = string(rs[len(rs)-n:])
				} else {
					nt.term = string(rs[:n])
				}
				out = append(out, nt)
			}
		} else {
			for k := 0; k < len(rs); k++ {
				for n := minGram; n <= maxGram && k+n <= len(rs); n++ {
					nt := base
					nt.term = string(rs[k : k+n])
					out = append(out, nt)
				}
			}
		}
		// The original is kept when no gram covers it entirely.
		if preserve && (len(rs) < minGram || len(rs) > maxGram) {
			out = append(out, base)
		}
	}
	return out, nil
}

func shingleFilter(toks []analyzedToken, p map[string]any) ([]analyzedToken, *apiError) {
	minS := fsInt(p, "min_shingle_size", 2)
	maxS := fsInt(p, "max_shingle_size", 2)
	if minS > maxS {
		return nil, filterError("Min shingle size must be <= max shingle size")
	}
	outputUnigrams := fsBool(p, "output_unigrams", true)
	outputUnigramsIfNoShingles := fsBool(p, "output_unigrams_if_no_shingles", false)
	sep := fsString(p, "token_separator", " ")
	filler := fsString(p, "filler_token", "_")
	diff := maxS - minS
	if outputUnigrams {
		diff++
	}
	if limit := fsInt(p, "_max_shingle_diff", 3); diff > limit {
		reason := "In Shingle TokenFilter the difference between max_shingle_size and min_shingle_size (and +1 if " +
			"outputting unigrams) must be less than or equal to: [" + strconv.Itoa(limit) + "] but was [" + strconv.Itoa(diff) + "]. " +
			"This limit can be set by changing the [index.max_shingle_diff] index level setting."
		return nil, filterError(reason)
	}
	if len(toks) == 0 {
		return toks, nil
	}
	// One slot per input position; holes become filler tokens, at most
	// max_shingle_size-1 per hole, with the next token's start offset.
	type slot struct {
		tok    analyzedToken
		isFill bool
	}
	var slots []slot
	lastPos := -1
	realCount := 0
	for _, t := range toks {
		if t.pos <= lastPos {
			continue
		}
		fill := t.pos - lastPos - 1
		if fill > maxS-1 {
			fill = maxS - 1
		}
		for k := 0; k < fill; k++ {
			slots = append(slots, slot{tok: analyzedToken{term: filler, typ: "word", start: t.start, end: t.start}, isFill: true})
		}
		slots = append(slots, slot{tok: t})
		realCount++
		lastPos = t.pos
	}
	if realCount < minS && outputUnigramsIfNoShingles && !outputUnigrams {
		return toks, nil
	}
	minPos, maxPos := 0, len(slots)-1
	for i := range slots {
		slots[i].tok.pos = i
	}
	out := []analyzedToken{}
	for pos := minPos; pos <= maxPos; pos++ {
		s := slots[pos]
		if !s.isFill && outputUnigrams {
			out = append(out, s.tok)
		}
		for size := minS; size <= maxS; size++ {
			if pos+size-1 > maxPos {
				break
			}
			terms := make([]string, 0, size)
			fillerUsed := false
			allFill := true
			for k := 0; k < size; k++ {
				sl := slots[pos+k]
				terms = append(terms, sl.tok.term)
				if sl.isFill {
					fillerUsed = true
				} else {
					allFill = false
				}
			}
			if allFill {
				continue
			}
			first := slots[pos].tok
			last := slots[pos+size-1].tok
			pl := size
			if !outputUnigrams {
				pl = size - 1
			}
			if fillerUsed && pl < 2 {
				pl = 2
			}
			out = append(out, analyzedToken{
				term: strings.Join(terms, sep), start: first.start, end: last.end,
				pos: pos, typ: "shingle", posLen: pl,
			})
		}
	}
	return out, nil
}

func wordDelimiterFilter(toks []analyzedToken, p map[string]any, graph bool) []analyzedToken {
	splitCase := fsBool(p, "split_on_case_change", true)
	splitNums := fsBool(p, "split_on_numerics", true)
	genWord := fsBool(p, "generate_word_parts", true)
	genNum := fsBool(p, "generate_number_parts", true)
	stemPossessive := fsBool(p, "stem_english_possessive", true)
	catenateWords := fsBool(p, "catenate_words", false)
	catenateNumbers := fsBool(p, "catenate_numbers", false)
	catenateAll := fsBool(p, "catenate_all", false)
	preserve := fsBool(p, "preserve_original", false)
	adjust := fsBool(p, "adjust_offsets", true)
	protected := map[string]bool{}
	for _, w := range fsStrings(p, "protected_words") {
		protected[w] = true
	}
	alphaDelims := map[rune]bool{}
	for _, rule := range fsStrings(p, "type_table") {
		parts := strings.SplitN(rule, "=>", 2)
		if len(parts) != 2 {
			continue
		}
		from := strings.TrimSpace(parts[0])
		to := strings.TrimSpace(parts[1])
		if to == "ALPHA" {
			for _, r := range from {
				alphaDelims[r] = true
			}
		}
	}
	outputPos := 0
	out := []analyzedToken{}
	type part struct {
		term       string
		start, end int
		isNum      bool
		alpha      bool
	}
	for _, t := range toks {
		term := t.term
		if protected[term] {
			nt := t
			nt.pos = outputPos
			outputPos++
			out = append(out, nt)
			continue
		}
		if stemPossessive {
			term = possessiveStrip(term)
		}
		rs := []rune(term)
		// Classify each rune.
		classOf := func(r rune) int { // 0 delim, 1 lower, 2 upper, 3 digit
			if unicode.IsDigit(r) {
				return 3
			}
			if unicode.IsLetter(r) {
				if unicode.IsUpper(r) {
					return 2
				}
				return 1
			}
			return 0
		}
		parts := []part{}
		cur := []rune{}
		curStart := 0
		offsets := []int{}
		off := 0
		for _, r := range rs {
			offsets = append(offsets, off)
			off += runeWidth(r)
		}
		offsets = append(offsets, off)
		flush := func(end int) {
			if len(cur) == 0 {
				return
			}
			s := curStart
			e := end
			pt := string(cur)
			isNum := true
			for _, r := range cur {
				if !unicode.IsDigit(r) {
					isNum = false
				}
			}
			alpha := true
			for _, r := range cur {
				if !unicode.IsLetter(r) {
					alpha = false
				}
			}
			parts = append(parts, part{term: pt, start: t.start + offsets[s], end: t.start + offsets[e], isNum: isNum, alpha: alpha})
			cur = cur[:0]
		}
		prevClass := 0
		for i, r := range rs {
			c := classOf(r)
			if c == 0 {
				if alphaDelims[r] {
					c = 1
				} else {
					flush(i)
					prevClass = 0
					continue
				}
			}
			if len(cur) > 0 {
				prev := prevClass
				split := false
				if splitCase && prev == 1 && c == 2 {
					split = true
				}
				if splitNums && ((prev == 1 || prev == 2) && c == 3 || prev == 3 && (c == 1 || c == 2)) {
					split = true
				}
				if split {
					flush(i)
				}
			}
			if len(cur) == 0 {
				curStart = i
			}
			cur = append(cur, r)
			prevClass = c
		}
		flush(len(rs))
		// Choose emitted parts.
		emitted := make([]part, 0, len(parts))
		for _, pt := range parts {
			if pt.isNum && !genNum {
				continue
			}
			if !pt.isNum && !genWord {
				continue
			}
			if !adjust {
				pt.start = t.start
				pt.end = t.end
			}
			emitted = append(emitted, pt)
		}
		if len(emitted) == 0 {
			continue
		}
		var catenated []part
		catTerm := func(ps []part) string {
			var b strings.Builder
			for _, pt := range ps {
				b.WriteString(pt.term)
			}
			return b.String()
		}
		if catenateAll {
			catenated = append(catenated, part{term: catTerm(emitted), start: t.start, end: t.end})
		} else {
			words := []part{}
			nums := []part{}
			for _, pt := range emitted {
				if pt.isNum {
					nums = append(nums, pt)
				} else {
					words = append(words, pt)
				}
			}
			if catenateWords && len(words) > 1 {
				catenated = append(catenated, part{term: catTerm(words), start: t.start, end: t.end, alpha: true})
			}
			if catenateNumbers && len(nums) > 1 {
				catenated = append(catenated, part{term: catTerm(nums), start: t.start, end: t.end, isNum: true})
			}
		}
		pl := len(emitted)
		if graph && catenateAll && preserve {
			nt := t
			nt.pos = outputPos
			nt.posLen = pl
			out = append(out, nt)
		}
		if graph && len(catenated) > 0 {
			for _, ct := range catenated {
				out = append(out, analyzedToken{term: ct.term, start: ct.start, end: ct.end, pos: outputPos, typ: t.typ, posLen: pl})
			}
		}
		if !graph && len(catenated) > 0 {
			// Classical word_delimiter emits the catenated form after the first part.
			first := true
			for _, pt := range emitted {
				out = append(out, analyzedToken{term: pt.term, start: pt.start, end: pt.end, pos: outputPos, typ: t.typ})
				outputPos++
				if first {
					for _, ct := range catenated {
						out = append(out, analyzedToken{term: ct.term, start: ct.start, end: ct.end, pos: outputPos - 1, typ: t.typ})
					}
					first = false
				}
			}
			continue
		}
		for _, pt := range emitted {
			out = append(out, analyzedToken{term: pt.term, start: pt.start, end: pt.end, pos: outputPos, typ: t.typ})
			outputPos++
		}
	}
	return out
}

func commonGramsFilter(toks []analyzedToken, p map[string]any) []analyzedToken {
	common := map[string]bool{}
	for _, w := range fsStrings(p, "common_words") {
		if fsBool(p, "ignore_case", false) {
			w = strings.ToLower(w)
		}
		common[w] = true
	}
	query := fsBool(p, "query_mode", false)
	sep := "_"
	isCommon := func(t analyzedToken) bool {
		key := t.term
		if fsBool(p, "ignore_case", false) {
			key = strings.ToLower(key)
		}
		return common[key]
	}
	// CommonGramsFilter: a bigram joins every pair where either side is common.
	out := []analyzedToken{}
	for i, t := range toks {
		out = append(out, t)
		if i+1 < len(toks) {
			next := toks[i+1]
			if isCommon(t) || isCommon(next) {
				out = append(out, analyzedToken{
					term: t.term + sep + next.term, start: t.start, end: next.end,
					pos: t.pos, typ: "gram", posLen: 2,
				})
			}
		}
	}
	if !query {
		return out
	}
	// CommonGramsQueryFilter: keep a unigram only when no gram follows it,
	// and drop the last token when it follows a gram.
	res := []analyzedToken{}
	var prev *analyzedToken
	prevType := ""
	for i := range out {
		cur := out[i]
		if prev != nil && cur.typ != "gram" {
			t := *prev
			t.posLen = 0
			res = append(res, t)
			prevType = t.typ
			prev = &out[i]
			continue
		}
		prev = &out[i]
	}
	if prev != nil && prevType != "gram" {
		t := *prev
		t.posLen = 0
		res = append(res, t)
	}
	return res
}
