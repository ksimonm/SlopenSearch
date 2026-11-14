package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jregex"
)

// Token filter framework. Each filter receives the token stream and returns a
// modified stream, mirroring the Lucene token filter contract closely enough
// for the recorded OpenSearch responses.

type filterSpec struct {
	Type   string
	Params map[string]any
	// finalOffset is the end offset of the analyzed text (Lucene's final
	// offset), used by filters looking at trailing tokens.
	finalOffset int
	// ruleAnalyzer analyzes synonym rules with the preceding chain.
	ruleAnalyzer func(string) []Token
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
		return filterSpec{Type: "stemmer", Params: map[string]any{"language": "english"}}, true
	}
	return filterSpec{Type: name}, true
}

func resolveFilterSpec(idx *Index, c Component) (filterSpec, bool) {
	if c.Inline != nil {
		typ := c.name
		if t, ok := c.Inline["type"].(string); ok && t != "" {
			typ = t
		}
		if !knownFilterTypes[typ] {
			return filterSpec{}, false
		}
		return filterSpec{Type: typ, Params: c.Inline}, true
	}
	if idx != nil {
		if raw, ok := findSetting(idx.Settings, "analysis.filter."+c.name+".type"); ok {
			typ, _ := raw.(string)
			return filterSpec{Type: typ, Params: subSettings(idx.Settings, "analysis.filter."+c.name)}, true
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

func ParamBool(p map[string]any, key string, def bool) bool {
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

func ParamInt(p map[string]any, key string, def int) int {
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
	return FilterNames(p[key])
}

func filterError(reason string) *apierr.Error {
	return &apierr.Error{Type: "illegal_argument_exception", Status: 400, Reason: reason,
		RootCause: &apierr.Error{Type: "illegal_argument_exception", Status: 400, Reason: reason}}
}

func applyGlobalFilters(toks []Token, names []string) ([]Token, bool) {
	comps := make([]Component, 0, len(names))
	for _, n := range names {
		comps = append(comps, Component{name: n})
	}
	out, err := applyFilterComponents(nil, toks, comps)
	if err != nil {
		return nil, false
	}
	return out, true
}

func applyFilterComponents(idx *Index, toks []Token, comps []Component) ([]Token, *apierr.Error) {
	cur := toks
	for _, c := range comps {
		spec, ok := resolveFilterSpec(idx, c)
		if !ok {
			return nil, filterError("failed to find global filter under [" + c.name + "]")
		}
		var err *apierr.Error
		cur, err = applyFilterOne(idx, cur, spec)
		if err != nil {
			return nil, err
		}
	}
	return cur, nil
}

func applyNamedFilterChain(idx *Index, toks []Token, chain string) ([]Token, *apierr.Error) {
	cur := toks
	for _, name := range strings.Split(chain, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		spec, ok := resolveFilterSpec(idx, Component{name: name})
		if !ok {
			return nil, filterError("failed to find global filter under [" + name + "]")
		}
		var err *apierr.Error
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

func TruncateUTF16(s string, n int) string {
	if n <= 0 {
		return ""
	}
	w := 0
	rs := []rune(s)
	for i, r := range rs {
		if w+RuneWidth(r) > n {
			return string(rs[:i])
		}
		w += RuneWidth(r)
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

func applyFilterOne(idx *Index, toks []Token, spec filterSpec) ([]Token, *apierr.Error) {
	if out, err, handled := langFilterOne(toks, spec); handled {
		return out, err
	}
	p := spec.Params
	switch spec.Type {
	case "asciifolding":
		preserve := ParamBool(p, "preserve_original", false)
		out := make([]Token, 0, len(toks))
		for _, t := range toks {
			folded := ASCIIFoldReplacer.Replace(t.Term)
			if preserve && folded != t.Term {
				ft := t
				ft.Term = folded
				out = append(out, ft)
				out = append(out, t)
			} else {
				t.Term = folded
				out = append(out, t)
			}
		}
		toks = out
	case "stop":
		if fsString(p, "stopwords_path", "") != "" {
			return nil, filterError("IOException while reading stopwords_path: file not readable")
		}
		words := stopWordSet(p)
		ignoreCase := ParamBool(p, "ignore_case", false)
		removeTrailing := ParamBool(p, "remove_trailing", true)
		out := make([]Token, 0, len(toks))
		for i, t := range toks {
			key := t.Term
			if ignoreCase {
				key = strings.ToLower(key)
			}
			if words[key] {
				// SuggestStopFilter keeps a final stopword the text ends with.
				if !removeTrailing && i == len(toks)-1 && t.End == spec.finalOffset {
					out = append(out, t)
				}
				continue
			}
			out = append(out, t)
		}
		toks = out
	case "length":
		min := ParamInt(p, "min", 0)
		max := ParamInt(p, "max", 1<<30)
		out := toks[:0]
		for _, t := range toks {
			n := UTF16Offset(t.Term)
			if n >= min && n <= max {
				out = append(out, t)
			}
		}
		toks = out
	case "truncate":
		n := ParamInt(p, "length", 10)
		for i := range toks {
			toks[i].Term = TruncateUTF16(toks[i].Term, n)
		}
	case "trim":
		for i := range toks {
			toks[i].Term = strings.TrimSpace(toks[i].Term)
		}
	case "reverse":
		for i := range toks {
			rs := []rune(toks[i].Term)
			for a, b := 0, len(rs)-1; a < b; a, b = a+1, b-1 {
				rs[a], rs[b] = rs[b], rs[a]
			}
			toks[i].Term = string(rs)
		}
	case "unique":
		// UniqueTokenFilter drops duplicates together with their position
		// increments.
		samePos := ParamBool(p, "only_on_same_position", false)
		seen := map[string]bool{}
		out := []Token{}
		prevIn, pos := -1, -1
		for _, t := range toks {
			inc := t.Pos - prevIn
			prevIn = t.Pos
			if samePos && inc > 0 {
				seen = map[string]bool{}
			}
			dup := seen[t.Term] && (!samePos || inc == 0)
			seen[t.Term] = true
			if dup {
				continue
			}
			pos += inc
			t.Pos = pos
			out = append(out, t)
		}
		toks = out
	case "limit":
		n := ParamInt(p, "max_token_count", 1)
		if len(toks) > n {
			toks = toks[:n]
		}
	case "classic":
		for i := range toks {
			switch toks[i].Type {
			case "<ACRONYM>":
				toks[i].Term = strings.ReplaceAll(toks[i].Term, ".", "")
			case "<APOSTROPHE>":
				toks[i].Term = possessiveStrip(toks[i].Term)
			}
		}
	case "pattern_replace":
		return patternReplaceFilter(toks, p)
	case "pattern_capture":
		return patternCaptureFilter(toks, p)
	case "fingerprint":
		return fingerprintFilter(toks, p), nil
	case "condition", "predicate_token_filter":
		return predicateFilter(idx, toks, p, spec.Type == "condition")
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
			if at := strings.LastIndex(toks[i].Term, delim); at >= 0 {
				toks[i].Term = toks[i].Term[:at]
			}
		}
	case "flatten_graph":
	case "min_hash":
		return minHashFilter(toks), nil
	case "hunspell":
		return nil, &apierr.Error{Type: "illegal_state_exception", Status: 500,
			Reason:    "Failed to load hunspell dictionary for locale: " + fsString(p, "locale", ""),
			RootCause: &apierr.Error{Type: "illegal_state_exception", Status: 500, Reason: "Failed to load hunspell dictionary for locale: " + fsString(p, "locale", "")}}
	case "keyword_repeat":
		out := make([]Token, 0, len(toks)*2)
		for _, t := range toks {
			kw := t
			kw.Keyword = true
			out = append(out, kw)
			out = append(out, t)
		}
		toks = out
	case "remove_duplicates":
		seen := map[string]bool{}
		out := toks[:0]
		for _, t := range toks {
			key := t.Term + "\x00" + strconv.Itoa(t.Pos)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, t)
		}
		toks = out
	case "keyword_marker":
		keywords := fsStrings(p, "keywords")
		ignoreCase := ParamBool(p, "ignore_case", false)
		pattern := fsString(p, "keywords_pattern", "")
		var re *jregex.Regexp
		if pattern != "" {
			var err *apierr.Error
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
			key := toks[i].Term
			if ignoreCase {
				key = strings.ToLower(key)
			}
			// PatternKeywordMarkerFilter requires the whole term to match.
			if set[key] || (re != nil && re.FullMatch([]rune(toks[i].Term))) {
				toks[i].Keyword = true
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
			if v, ok := rules[toks[i].Term]; ok {
				toks[i].Term = v
				toks[i].Keyword = true
			}
		}
	case "ngram", "edge_ngram":
		var err *apierr.Error
		toks, err = ngramFilter(toks, p, spec.Type == "edge_ngram")
		if err != nil {
			return nil, err
		}
	case "shingle":
		var err *apierr.Error
		toks, err = shingleFilter(toks, p)
		if err != nil {
			return nil, err
		}
	case "word_delimiter", "word_delimiter_graph":
		toks = wordDelimiterFilter(toks, p, spec.Type == "word_delimiter_graph")
	case "common_grams":
		toks = commonGramsFilter(toks, p)
	case "synonym", "synonym_graph":
		var err *apierr.Error
		toks, err = synonymFilter(toks, spec, spec.Type == "synonym_graph")
		if err != nil {
			return nil, err
		}
	default:
		return nil, filterError("failed to find global filter under [" + spec.Type + "]")
	}
	return toks, nil
}

func fingerprintFilter(toks []Token, p map[string]any) []Token {
	if len(toks) == 0 {
		return toks
	}
	sep := fsString(p, "separator", " ")
	maxSize := ParamInt(p, "max_output_size", 255)
	seen := map[string]bool{}
	terms := []string{}
	for _, t := range toks {
		if !seen[t.Term] {
			seen[t.Term] = true
			terms = append(terms, t.Term)
		}
	}
	sort.Strings(terms)
	joined := strings.Join(terms, sep)
	if UTF16Offset(joined) > maxSize {
		return []Token{}
	}
	end := 0
	for _, t := range toks {
		if t.End > end {
			end = t.End
		}
	}
	return []Token{{Term: joined, Start: 0, End: end, Pos: 0, Type: "fingerprint"}}
}

func predicateFilter(idx *Index, toks []Token, p map[string]any, condition bool) ([]Token, *apierr.Error) {
	source := ""
	if script, ok := p["script"].(map[string]any); ok {
		source, _ = script["source"].(string)
	}
	// Supported predicates: term length comparisons and position parity.
	matches := func(t Token) bool {
		switch {
		case strings.Contains(source, "getTerm().length() < "):
			n := scriptNumber(source, "getTerm().length() < ")
			return UTF16Offset(t.Term) < n
		case strings.Contains(source, "getTerm().length() > "):
			n := scriptNumber(source, "getTerm().length() > ")
			return UTF16Offset(t.Term) > n
		case strings.Contains(source, "getTerm().length() == "):
			n := scriptNumber(source, "getTerm().length() == ")
			return UTF16Offset(t.Term) == n
		case strings.Contains(source, "getPosition() % 2 == 0"):
			return t.Pos%2 == 0
		case strings.Contains(source, "getPosition() % 2 == 1"):
			return t.Pos%2 == 1
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
	out := make([]Token, 0, len(toks))
	for _, t := range toks {
		if !matches(t) {
			out = append(out, t)
			continue
		}
		part, err := applyNamedFilterChain(idx, []Token{t}, strings.Join(sub, ","))
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

func multiplexerFilter(idx *Index, toks []Token, p map[string]any) ([]Token, *apierr.Error) {
	chains := fsStrings(p, "filters")
	preserve := ParamBool(p, "preserve_original", true)
	out := []Token{}
	seen := map[string]bool{}
	add := func(ts []Token) {
		for _, t := range ts {
			key := t.Term + "\x00" + strconv.Itoa(t.Pos) + "\x00" + strconv.Itoa(t.Start) + "\x00" + strconv.Itoa(t.End) + "\x00" + t.Type
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
		part, err := applyNamedFilterChain(idx, append([]Token{}, toks...), chain)
		if err != nil {
			return nil, err
		}
		add(part)
	}
	return out, nil
}

func decompounderFilter(toks []Token, p map[string]any) []Token {
	words := fsStrings(p, "word_list")
	minSize := ParamInt(p, "min_word_size", 5)
	maxSize := ParamInt(p, "max_subword_size", 15)
	out := make([]Token, 0, len(toks))
	for _, t := range toks {
		out = append(out, t)
		rs := []rune(t.Term)
		for i := 0; i < len(rs); i++ {
			for n := maxSize; n >= minSize; n-- {
				if i+n > len(rs) {
					continue
				}
				sub := string(rs[i : i+n])
				for _, w := range words {
					if w == sub {
						nt := t
						nt.Term = sub
						out = append(out, nt)
						break
					}
				}
			}
		}
	}
	return out
}

func minHashFilter(toks []Token) []Token {
	if len(toks) == 0 {
		return toks
	}
	terms := make([]string, 0, len(toks))
	for _, t := range toks {
		terms = append(terms, t.Term)
	}
	sort.Strings(terms)
	sum := sha256.Sum256([]byte(strings.Join(terms, " ")))
	return []Token{{
		Term: hex.EncodeToString(sum[:8]), Start: toks[0].Start, End: toks[len(toks)-1].End,
		Pos: 0, Type: "MIN_HASH",
	}}
}

func ngramFilter(toks []Token, p map[string]any, edge bool) ([]Token, *apierr.Error) {
	minGram := ParamInt(p, "min_gram", 1)
	defMax := 2
	if edge {
		defMax = 1
	}
	maxGram := ParamInt(p, "max_gram", defMax)
	limit := ParamInt(p, "_max_ngram_diff", 1)
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
	preserve := ParamBool(p, "preserve_original", false)
	back := fsString(p, "side", "front") == "back"
	out := make([]Token, 0, len(toks))
	for _, t := range toks {
		rs := []rune(t.Term)
		base := t
		if edge {
			for n := minGram; n <= maxGram && n <= len(rs); n++ {
				nt := base
				if back {
					nt.Term = string(rs[len(rs)-n:])
				} else {
					nt.Term = string(rs[:n])
				}
				out = append(out, nt)
			}
		} else {
			for k := 0; k < len(rs); k++ {
				for n := minGram; n <= maxGram && k+n <= len(rs); n++ {
					nt := base
					nt.Term = string(rs[k : k+n])
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

func shingleFilter(toks []Token, p map[string]any) ([]Token, *apierr.Error) {
	minS := ParamInt(p, "min_shingle_size", 2)
	maxS := ParamInt(p, "max_shingle_size", 2)
	if minS > maxS {
		return nil, filterError("Min shingle size must be <= max shingle size")
	}
	outputUnigrams := ParamBool(p, "output_unigrams", true)
	outputUnigramsIfNoShingles := ParamBool(p, "output_unigrams_if_no_shingles", false)
	sep := fsString(p, "token_separator", " ")
	filler := fsString(p, "filler_token", "_")
	diff := maxS - minS
	if outputUnigrams {
		diff++
	}
	if limit := ParamInt(p, "_max_shingle_diff", 3); diff > limit {
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
		tok    Token
		isFill bool
	}
	var slots []slot
	lastPos := -1
	realCount := 0
	for _, t := range toks {
		if t.Pos <= lastPos {
			continue
		}
		fill := t.Pos - lastPos - 1
		if fill > maxS-1 {
			fill = maxS - 1
		}
		for k := 0; k < fill; k++ {
			slots = append(slots, slot{tok: Token{Term: filler, Type: "word", Start: t.Start, End: t.Start}, isFill: true})
		}
		slots = append(slots, slot{tok: t})
		realCount++
		lastPos = t.Pos
	}
	if realCount < minS && outputUnigramsIfNoShingles && !outputUnigrams {
		return toks, nil
	}
	minPos, maxPos := 0, len(slots)-1
	for i := range slots {
		slots[i].tok.Pos = i
	}
	out := []Token{}
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
				terms = append(terms, sl.tok.Term)
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
			out = append(out, Token{
				Term: strings.Join(terms, sep), Start: first.Start, End: last.End,
				Pos: pos, Type: "shingle", PosLen: pl,
			})
		}
	}
	return out, nil
}

func wordDelimiterFilter(toks []Token, p map[string]any, graph bool) []Token {
	splitCase := ParamBool(p, "split_on_case_change", true)
	splitNums := ParamBool(p, "split_on_numerics", true)
	genWord := ParamBool(p, "generate_word_parts", true)
	genNum := ParamBool(p, "generate_number_parts", true)
	stemPossessive := ParamBool(p, "stem_english_possessive", true)
	catenateWords := ParamBool(p, "catenate_words", false)
	catenateNumbers := ParamBool(p, "catenate_numbers", false)
	catenateAll := ParamBool(p, "catenate_all", false)
	preserve := ParamBool(p, "preserve_original", false)
	adjust := ParamBool(p, "adjust_offsets", true)
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
	out := []Token{}
	type part struct {
		term       string
		start, end int
		isNum      bool
		alpha      bool
	}
	for _, t := range toks {
		term := t.Term
		if protected[term] {
			nt := t
			nt.Pos = outputPos
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
			off += RuneWidth(r)
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
			parts = append(parts, part{term: pt, start: t.Start + offsets[s], end: t.Start + offsets[e], isNum: isNum, alpha: alpha})
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
				pt.start = t.Start
				pt.end = t.End
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
			catenated = append(catenated, part{term: catTerm(emitted), start: t.Start, end: t.End})
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
				catenated = append(catenated, part{term: catTerm(words), start: t.Start, end: t.End, alpha: true})
			}
			if catenateNumbers && len(nums) > 1 {
				catenated = append(catenated, part{term: catTerm(nums), start: t.Start, end: t.End, isNum: true})
			}
		}
		pl := len(emitted)
		if graph && catenateAll && preserve {
			nt := t
			nt.Pos = outputPos
			nt.PosLen = pl
			out = append(out, nt)
		}
		if graph && len(catenated) > 0 {
			for _, ct := range catenated {
				out = append(out, Token{Term: ct.term, Start: ct.start, End: ct.end, Pos: outputPos, Type: t.Type, PosLen: pl})
			}
		}
		if !graph && len(catenated) > 0 {
			// Classical word_delimiter emits the catenated form after the first part.
			first := true
			for _, pt := range emitted {
				out = append(out, Token{Term: pt.term, Start: pt.start, End: pt.end, Pos: outputPos, Type: t.Type})
				outputPos++
				if first {
					for _, ct := range catenated {
						out = append(out, Token{Term: ct.term, Start: ct.start, End: ct.end, Pos: outputPos - 1, Type: t.Type})
					}
					first = false
				}
			}
			continue
		}
		for _, pt := range emitted {
			out = append(out, Token{Term: pt.term, Start: pt.start, End: pt.end, Pos: outputPos, Type: t.Type})
			outputPos++
		}
	}
	return out
}

func commonGramsFilter(toks []Token, p map[string]any) []Token {
	common := map[string]bool{}
	for _, w := range fsStrings(p, "common_words") {
		if ParamBool(p, "ignore_case", false) {
			w = strings.ToLower(w)
		}
		common[w] = true
	}
	query := ParamBool(p, "query_mode", false)
	sep := "_"
	isCommon := func(t Token) bool {
		key := t.Term
		if ParamBool(p, "ignore_case", false) {
			key = strings.ToLower(key)
		}
		return common[key]
	}
	// CommonGramsFilter: a bigram joins every pair where either side is common.
	out := []Token{}
	for i, t := range toks {
		out = append(out, t)
		if i+1 < len(toks) {
			next := toks[i+1]
			if isCommon(t) || isCommon(next) {
				out = append(out, Token{
					Term: t.Term + sep + next.Term, Start: t.Start, End: next.End,
					Pos: t.Pos, Type: "gram", PosLen: 2,
				})
			}
		}
	}
	if !query {
		return out
	}
	// CommonGramsQueryFilter: keep a unigram only when no gram follows it,
	// and drop the last token when it follows a gram.
	res := []Token{}
	var prev *Token
	prevType := ""
	for i := range out {
		cur := out[i]
		if prev != nil && cur.Type != "gram" {
			t := *prev
			t.PosLen = 0
			res = append(res, t)
			prevType = t.Type
			prev = &out[i]
			continue
		}
		prev = &out[i]
	}
	if prev != nil && prevType != "gram" {
		t := *prev
		t.PosLen = 0
		res = append(res, t)
	}
	return res
}
