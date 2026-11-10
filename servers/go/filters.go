package main

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
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
	switch name {
	case "porter_stem":
		return filterSpec{typ: "stemmer", params: map[string]any{"language": "porter"}}, true
	case "english":
		return filterSpec{typ: "stemmer", params: map[string]any{"language": "english"}}, true
	case "russian_stem", "czech_stem", "french_stem", "german_stem", "dutch_stem", "brazilian_stem":
		return filterSpec{typ: "stemmer", params: map[string]any{"language": name}}, true
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
		if v, ok := p[key].(bool); ok {
			return v
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

var russianStop = map[string]bool{
	"и": true, "в": true, "во": true, "не": true, "что": true, "он": true,
	"на": true, "я": true, "с": true, "со": true, "как": true, "а": true,
	"то": true, "все": true, "она": true, "так": true, "его": true, "но": true,
	"да": true, "ты": true, "к": true, "у": true, "же": true, "вы": true,
	"за": true, "бы": true, "по": true, "только": true, "ее": true, "мне": true,
	"было": true, "вот": true, "от": true, "меня": true, "еще": true, "нет": true,
	"о": true, "из": true, "ему": true, "теперь": true, "когда": true,
	"даже": true, "ну": true, "вдруг": true, "ли": true, "если": true,
	"уже": true, "или": true, "ни": true, "быть": true, "был": true,
	"него": true, "до": true, "вас": true, "нибудь": true, "опять": true,
	"уж": true, "вам": true, "ведь": true, "там": true, "потом": true,
	"себя": true, "ничего": true, "ей": true, "может": true, "они": true,
	"тут": true, "где": true, "есть": true, "надо": true, "ней": true,
	"для": true, "мы": true, "тебя": true, "их": true, "чем": true,
	"была": true, "сам": true, "чтоб": true, "без": true, "будто": true,
	"чего": true, "раз": true, "тоже": true, "себе": true, "под": true,
	"будет": true, "ж": true, "тогда": true, "кто": true, "этот": true,
	"того": true, "потому": true, "этого": true, "какой": true, "совсем": true,
	"ним": true, "здесь": true, "этом": true, "один": true, "почти": true,
	"мой": true, "тем": true, "чтобы": true, "нее": true, "сейчас": true,
	"были": true, "куда": true, "зачем": true, "всех": true, "никогда": true,
	"можно": true, "при": true, "наконец": true, "два": true, "об": true,
	"другой": true, "хоть": true, "после": true, "над": true, "больше": true,
	"тот": true, "через": true, "эти": true, "нас": true, "про": true,
	"всего": true, "них": true, "какая": true, "много": true, "разве": true,
	"три": true, "эту": true, "моя": true, "впрочем": true, "хорошо": true,
	"свою": true, "этой": true, "перед": true, "иногда": true, "лучше": true,
	"чуть": true, "том": true, "нельзя": true, "такой": true, "им": true,
	"более": true, "всегда": true, "конечно": true, "всю": true, "между": true,
}

func stopWordSet(p map[string]any) map[string]bool {
	words := fsStrings(p, "stopwords")
	if len(words) == 0 {
		return defaultEnglishStop
	}
	set := map[string]bool{}
	for _, w := range words {
		switch w {
		case "_english_":
			for k := range defaultEnglishStop {
				set[k] = true
			}
		case "_russian_":
			for k := range russianStop {
				set[k] = true
			}
		case "_none_":
			// empty
		default:
			set[w] = true
		}
	}
	return set
}

func turkishLower(r rune) rune {
	switch r {
	case 'I':
		return 'ı'
	case 'İ':
		return 'i'
	}
	return unicode.ToLower(r)
}

func ingestLower(s, language string) string {
	switch language {
	case "turkish":
		return translateRunes(s, turkishLower)
	case "greek":
		return simpleCaseFold(s)
	case "irish":
		v := simpleCaseFold(s)
		rs := []rune(v)
		if len(rs) > 2 && (rs[0] == 'n' || rs[0] == 't') {
			if strings.ContainsRune("aeiou", rs[1]) {
				return string(rs[:1]) + "-" + string(rs[1:])
			}
		}
		return v
	}
	return simpleCaseFold(s)
}

func germanNormalize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'ä', 'Ä':
			if r == 'Ä' {
				b.WriteRune('A')
			} else {
				b.WriteRune('a')
			}
		case 'ö', 'Ö':
			if r == 'Ö' {
				b.WriteRune('O')
			} else {
				b.WriteRune('o')
			}
		case 'ü', 'Ü':
			if r == 'Ü' {
				b.WriteRune('U')
			} else {
				b.WriteRune('u')
			}
		case 'ß':
			b.WriteString("ss")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func scandinavianFold(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'å', 'Å':
			if r == 'Å' {
				b.WriteRune('A')
			} else {
				b.WriteRune('a')
			}
		case 'æ', 'Æ':
			if r == 'Æ' {
				b.WriteRune('A')
			} else {
				b.WriteRune('a')
			}
		case 'ø', 'Ø':
			if r == 'Ø' {
				b.WriteRune('O')
			} else {
				b.WriteRune('o')
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func arabicNormalize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'أ', 'إ', 'آ', 'ٱ':
			b.WriteRune('ا')
		case 'ؤ':
			b.WriteRune('و')
		case 'ئ', 'ى':
			b.WriteRune('ي')
		case 'ة':
			b.WriteRune('ه')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func serbianNormalize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'ђ', 'Ђ':
			b.WriteString("dj")
		case 'ћ', 'Ћ':
			b.WriteString("c")
		case 'љ', 'Љ':
			b.WriteString("lj")
		case 'њ', 'Њ':
			b.WriteString("nj")
		case 'џ', 'Џ':
			b.WriteString("dz")
		case 'ѐ':
			b.WriteString("e")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func decimalDigit(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '٠' && r <= '٩':
			b.WriteRune('0' + (r - '٠'))
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + (r - '۰'))
		case r >= '०' && r <= '९':
			b.WriteRune('0' + (r - '०'))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Porter stemmer (with step 4 iterated, matching Lucene's stemmer output).
func isCons(b []byte, i int) bool {
	switch b[i] {
	case 'a', 'e', 'i', 'o', 'u':
		return false
	case 'y':
		if i == 0 {
			return true
		}
		return !isCons(b, i-1)
	}
	return true
}

func porterMeasure(b []byte) int {
	n := 0
	i := 0
	for i < len(b) {
		if isCons(b, i) {
			i++
			continue
		}
		// vowel run
		for i < len(b) && !isCons(b, i) {
			i++
		}
		if i < len(b) {
			n++
		}
		for i < len(b) && isCons(b, i) {
			i++
		}
	}
	return n
}

func porterHasVowel(b []byte) bool {
	for i := range b {
		if !isCons(b, i) {
			return true
		}
	}
	return false
}

func porterEnds(b []byte, s string) bool {
	return len(b) >= len(s) && string(b[len(b)-len(s):]) == s
}

func porterSetto(b []byte, s string) []byte {
	return append(b[:0:0], []byte(s)...)
}

func porterReplace(b []byte, from, to string) []byte {
	return append(b[:len(b)-len(from)], []byte(to)...)
}

func porterStem(word string) string {
	if len(word) <= 2 {
		return word
	}
	b := []byte(word)
	// Step 1a
	switch {
	case porterEnds(b, "sses"):
		b = porterReplace(b, "sses", "ss")
	case porterEnds(b, "ies"):
		b = porterReplace(b, "ies", "i")
	case porterEnds(b, "ss"):
	case porterEnds(b, "s"):
		b = porterReplace(b, "s", "")
	}
	// Step 1b
	if porterEnds(b, "eed") {
		if porterMeasure(b[:len(b)-3]) > 0 {
			b = porterReplace(b, "eed", "ee")
		}
	} else {
		changed := false
		if porterEnds(b, "ed") && porterHasVowel(b[:len(b)-2]) {
			b = porterReplace(b, "ed", "")
			changed = true
		} else if porterEnds(b, "ing") && porterHasVowel(b[:len(b)-3]) {
			b = porterReplace(b, "ing", "")
			changed = true
		}
		if changed {
			switch {
			case porterEnds(b, "at"), porterEnds(b, "bl"), porterEnds(b, "iz"):
				b = append(b, 'e')
			case len(b) >= 2 && b[len(b)-1] == b[len(b)-2] && isCons(b, len(b)-1) &&
				!strings.ContainsRune("lsz", rune(b[len(b)-1])):
				b = b[:len(b)-1]
			case porterMeasure(b) == 1 && porterCVC(b):
				b = append(b, 'e')
			}
		}
	}
	// Step 1c
	if porterEnds(b, "y") && porterHasVowel(b[:len(b)-1]) {
		b[len(b)-1] = 'i'
	}
	// Step 2
	type rule struct{ from, to string }
	step2 := []rule{
		{"ational", "ate"}, {"tional", "tion"}, {"enci", "ence"}, {"anci", "ance"},
		{"izer", "ize"}, {"bli", "ble"}, {"alli", "al"}, {"entli", "ent"},
		{"eli", "e"}, {"ousli", "ous"}, {"ization", "ize"}, {"ation", "ate"},
		{"ator", "ate"}, {"alism", "al"}, {"iveness", "ive"}, {"fulness", "ful"},
		{"ousness", "ous"}, {"aliti", "al"}, {"iviti", "ive"}, {"biliti", "ble"},
		{"logi", "log"},
	}
	for _, r := range step2 {
		if porterEnds(b, r.from) {
			if porterMeasure(b[:len(b)-len(r.from)]) > 0 {
				b = porterReplace(b, r.from, r.to)
			}
			break
		}
	}
	// Step 3
	step3 := []rule{
		{"icate", "ic"}, {"ative", ""}, {"alize", "al"}, {"iciti", "ic"},
		{"ical", "ic"}, {"ful", ""}, {"ness", ""},
	}
	for _, r := range step3 {
		if porterEnds(b, r.from) {
			if porterMeasure(b[:len(b)-len(r.from)]) > 0 {
				b = porterReplace(b, r.from, r.to)
			}
			break
		}
	}
	// Step 4 (iterate so e.g. generalize -> general -> gener)
	for {
		step4 := []string{"al", "ance", "ence", "er", "ic", "able", "ible", "ant",
			"ement", "ment", "ent", "ion", "ou", "ism", "ate", "iti", "ous", "ive", "ize"}
		changed := false
		for _, s := range step4 {
			if !porterEnds(b, s) {
				continue
			}
			stem := b[:len(b)-len(s)]
			if s == "ion" && len(stem) > 0 && stem[len(stem)-1] != 's' && stem[len(stem)-1] != 't' {
				continue
			}
			if porterMeasure(stem) > 1 {
				b = stem
				changed = true
			}
			break
		}
		if !changed {
			break
		}
	}
	// Step 5a
	if porterEnds(b, "e") {
		stem := b[:len(b)-1]
		m := porterMeasure(stem)
		if m > 1 || (m == 1 && !porterCVC(stem)) {
			b = stem
		}
	}
	// Step 5b
	if porterEnds(b, "ll") && porterMeasure(b) > 1 {
		b = b[:len(b)-1]
	}
	return string(b)
}

func porterCVC(b []byte) bool {
	if len(b) < 3 {
		return false
	}
	n := len(b)
	if !isCons(b, n-1) || isCons(b, n-2) || !isCons(b, n-3) {
		return false
	}
	switch b[n-1] {
	case 'w', 'x', 'y':
		return false
	}
	return true
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

func stemWith(language, term string) string {
	low := strings.ToLower(term)
	switch language {
	case "", "porter", "english", "porter2", "snowball_english":
		return porterStem(term)
	case "kstem":
		return term
	case "light_english":
		if strings.HasSuffix(low, "ies") && len(low) > 4 {
			return term[:len(term)-3] + "y"
		}
		if strings.HasSuffix(low, "es") && len(low) > 4 {
			return term[:len(term)-2]
		}
		return term
	case "minimal_english":
		if strings.HasSuffix(low, "ies") && len(low) > 4 {
			return term[:len(term)-3] + "y"
		}
		if strings.HasSuffix(low, "s") && !strings.HasSuffix(low, "ss") && len(low) > 3 {
			return term[:len(term)-1]
		}
		return term
	case "possessive_english":
		return possessiveStrip(term)
	case "light_german":
		v := germanNormalize(term)
		for _, suf := range []string{"ern", "er", "en", "es", "e", "s"} {
			if strings.HasSuffix(v, suf) && len(v) > len(suf)+2 {
				return v[:len(v)-len(suf)]
			}
		}
		return v
	case "german", "german2", "snowball_german2":
		v := germanNormalize(term)
		for _, suf := range []string{"ern", "er", "en", "es", "e", "s"} {
			if strings.HasSuffix(v, suf) && len(v) > len(suf)+2 {
				v = v[:len(v)-len(suf)]
			}
		}
		return v
	case "german_stem":
		v := germanNormalize(term)
		for {
			trimmed := false
			for _, suf := range []string{"ern", "er", "en", "es", "e", "s"} {
				if strings.HasSuffix(v, suf) && len(v) > len(suf)+1 {
					v = v[:len(v)-len(suf)]
					trimmed = true
					break
				}
			}
			if !trimmed {
				break
			}
		}
		return v
	case "russian", "light_russian", "russian_stem", "snowball_russian":
		return russianStem(term)
	case "light_french", "french", "french_stem":
		if strings.HasSuffix(low, "aux") {
			return term[:len(term)-3] + "al"
		}
		return term
	case "light_spanish":
		if strings.HasSuffix(low, "os") || strings.HasSuffix(low, "as") {
			return term[:len(term)-2]
		}
		return term
	case "minimal_portuguese":
		if strings.HasSuffix(low, "s") && !strings.HasSuffix(low, "ss") {
			return term[:len(term)-1]
		}
		return term
	case "portuguese", "brazilian", "brazilian_stem":
		v := term
		if strings.HasSuffix(strings.ToLower(v), "as") {
			v = v[:len(v)-2]
		}
		v = strings.ReplaceAll(v, "ç", "c")
		return v
	case "arabic":
		if strings.HasPrefix(term, "ال") {
			return term[len("ال"):]
		}
		return term
	case "czech", "czech_stem":
		if strings.HasSuffix(low, "i") {
			return term[:len(term)-1]
		}
		return term
	case "dutch", "dutch_stem":
		if strings.HasSuffix(low, "en") && len(low) > 4 {
			return term[:len(term)-2]
		}
		if strings.HasSuffix(low, "s") && len(low) > 3 {
			return term[:len(term)-1]
		}
		return term
	}
	return term
}

func russianStem(term string) string {
	low := strings.ToLower(term)
	suffixes := []string{"иями", "ями", "ами", "ого", "его", "ому", "ему", "ыми", "ими",
		"ая", "яя", "ое", "ее", "ые", "ие", "ой", "ый", "ий", "ем", "ом",
		"ам", "ям", "ах", "ях", "ов", "ев", "ью", "ли", "ю", "я", "а", "о", "е", "и", "ы", "у"}
	for _, s := range suffixes {
		if strings.HasSuffix(low, s) && len(low) > len(s)+2 {
			return term[:len(term)-len(s)]
		}
	}
	return term
}

func applyFilterOne(idx *Index, toks []analyzedToken, spec filterSpec) ([]analyzedToken, *apiError) {
	p := spec.params
	switch spec.typ {
	case "lowercase":
		lang := fsString(p, "language", "")
		for i := range toks {
			toks[i].term = ingestLower(toks[i].term, lang)
		}
	case "uppercase":
		for i := range toks {
			toks[i].term = strings.ToUpper(toks[i].term)
		}
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
	case "german_normalization":
		for i := range toks {
			toks[i].term = germanNormalize(toks[i].term)
		}
	case "scandinavian_folding":
		for i := range toks {
			toks[i].term = scandinavianFold(toks[i].term)
		}
	case "scandinavian_normalization":
		// Already normalized form; no change.
	case "arabic_normalization":
		for i := range toks {
			toks[i].term = arabicNormalize(toks[i].term)
		}
	case "serbian_normalization":
		for i := range toks {
			toks[i].term = serbianNormalize(toks[i].term)
		}
	case "sorani_normalization":
		for i := range toks {
			toks[i].term = strings.ReplaceAll(toks[i].term, "ي", "ی")
		}
	case "decimal_digit":
		for i := range toks {
			toks[i].term = decimalDigit(toks[i].term)
		}
	case "turkish_lowercase":
		for i := range toks {
			toks[i].term = translateRunes(toks[i].term, turkishLower)
		}
	case "irish_lowercase":
		for i := range toks {
			toks[i].term = ingestLower(toks[i].term, "irish")
		}
	case "greek_lowercase":
		for i := range toks {
			toks[i].term = simpleCaseFold(toks[i].term)
		}
	case "stop":
		if fsString(p, "stopwords_path", "") != "" {
			return nil, filterError("IOException while reading stopwords_path: file not readable")
		}
		words := stopWordSet(p)
		ignoreCase := fsBool(p, "ignore_case", false)
		removeTrailing := fsBool(p, "remove_trailing", true)
		out := make([]analyzedToken, 0, len(toks))
		for _, t := range toks {
			key := t.term
			if ignoreCase {
				key = strings.ToLower(key)
			}
			if words[key] {
				continue
			}
			out = append(out, t)
		}
		if removeTrailing && len(out) > 0 {
			// Positions already carry holes; nothing further to trim.
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
		samePos := fsBool(p, "only_on_same_position", false)
		seen := map[string]bool{}
		out := toks[:0]
		for _, t := range toks {
			key := t.term
			if samePos {
				key = t.term + "\x00" + strconv.Itoa(t.pos)
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			t.pos = len(out)
			out = append(out, t)
		}
		toks = out
	case "limit":
		n := fsInt(p, "max_token_count", 1)
		if len(toks) > n {
			toks = toks[:n]
		}
	case "elision":
		articles := fsStrings(p, "articles")
		if len(articles) == 0 {
			articles = []string{"l", "m", "t", "qu", "n", "s", "j", "d", "c", "jusqu", "quoiqu", "lorsqu", "puisqu"}
		}
		caseSensitive := fsBool(p, "articles_case", false)
		for i := range toks {
			term := toks[i].term
			idxA := strings.Index(term, "'")
			if idxA <= 0 {
				continue
			}
			prefix := term[:idxA]
			for _, a := range articles {
				match := prefix == a
				if !caseSensitive {
					match = strings.EqualFold(prefix, a)
				}
				if match {
					toks[i].term = term[idxA+1:]
					break
				}
			}
		}
	case "apostrophe":
		for i := range toks {
			if idxA := strings.Index(toks[i].term, "'"); idxA >= 0 {
				toks[i].term = toks[i].term[:idxA]
			}
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
		var re *regexp.Regexp
		if pattern != "" {
			re = regexp.MustCompile(pattern)
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
			if set[key] || (re != nil && re.MatchString(toks[i].term)) {
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
	case "stemmer":
		language := fsString(p, "language", "")
		if language == "" {
			language = fsString(p, "name", "")
		}
		if language == "" {
			language = "english"
		}
		language = strings.ToLower(language)
		invalid := map[string]string{
			"klingon": "Klingon", "lovins": "Lovins", "ukrainian": "Ukrainian",
		}
		if cls, bad := invalid[language]; bad {
			return nil, &apiError{Type: "illegal_argument_exception", Status: 400,
				Reason:    "Invalid stemmer class specified: " + cls,
				RootCause: &apiError{Type: "illegal_argument_exception", Status: 400, Reason: "Invalid stemmer class specified: " + cls},
				CausedBy:  &apiError{Type: "class_not_found_exception", Reason: "org.tartarus.snowball.ext." + cls + "Stemmer"}}
		}
		for i := range toks {
			if toks[i].keyword {
				continue
			}
			toks[i].term = stemWith(language, toks[i].term)
		}
	case "kstem":
		for i := range toks {
			if toks[i].keyword {
				continue
			}
			if strings.HasSuffix(toks[i].term, "ies") && len(toks[i].term) > 4 {
				toks[i].term = toks[i].term[:len(toks[i].term)-3] + "y"
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
	case "cjk_width":
		toks = cjkWidthFilter(toks)
	case "cjk_bigram":
		toks = cjkBigramFilter(toks, p)
	case "synonym", "synonym_graph":
		var err *apiError
		toks, err = inlineSynonymFilter(toks, p, spec.typ == "synonym_graph")
		if err != nil {
			return nil, err
		}
	default:
		return nil, filterError("failed to find global filter under [" + spec.typ + "]")
	}
	return toks, nil
}

func patternGoRegexp(p map[string]any) (*regexp.Regexp, error) {
	pat := fsString(p, "pattern", "")
	for _, f := range fsStrings(p, "flags") {
		if strings.EqualFold(f, "CASE_INSENSITIVE") {
			pat = "(?i)" + pat
		}
	}
	return regexp.Compile(pat)
}

func patternReplaceFilter(toks []analyzedToken, p map[string]any) ([]analyzedToken, *apiError) {
	re, err := patternGoRegexp(p)
	if err != nil {
		return nil, filterError(err.Error())
	}
	repl := fsString(p, "replacement", "")
	all := fsBool(p, "all", true)
	for i := range toks {
		if all {
			toks[i].term = re.ReplaceAllString(toks[i].term, repl)
		} else {
			done := false
			toks[i].term = re.ReplaceAllStringFunc(toks[i].term, func(m string) string {
				if done {
					return m
				}
				done = true
				return re.ReplaceAllString(m, repl)
			})
		}
	}
	return toks, nil
}

func patternCaptureFilter(toks []analyzedToken, p map[string]any) ([]analyzedToken, *apiError) {
	patterns := fsStrings(p, "patterns")
	preserve := fsBool(p, "preserve_original", true)
	res := make([]*regexp.Regexp, 0, len(patterns))
	for _, pat := range patterns {
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, filterError(err.Error())
		}
		res = append(res, re)
	}
	out := make([]analyzedToken, 0, len(toks))
	for _, t := range toks {
		if preserve {
			out = append(out, t)
		}
		for _, re := range res {
			for _, m := range re.FindAllStringSubmatch(t.term, -1) {
				for gi := 1; gi < len(m); gi++ {
					if m[gi] == "" {
						continue
					}
					nt := t
					nt.term = m[gi]
					out = append(out, nt)
				}
			}
		}
	}
	return out, nil
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
	start := toks[0].start
	end := toks[len(toks)-1].end
	return []analyzedToken{{term: joined, start: start, end: end, pos: 0, typ: "fingerprint"}}
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
	if maxGram < minGram {
		return nil, filterError("minGram must not be greater than maxGram")
	}
	if maxGram-minGram > 1 {
		reason := "The difference between max_gram and min_gram in NGram Tokenizer must be less than or equal to: [1] but was [" +
			strconv.Itoa(maxGram-minGram) + "]. This limit can be set by changing the [index.max_ngram_diff] index level setting."
		return nil, filterError(reason)
	}
	preserve := fsBool(p, "preserve_original", false)
	back := fsString(p, "side", "front") == "back"
	out := make([]analyzedToken, 0, len(toks))
	for _, t := range toks {
		rs := []rune(t.term)
		base := t
		if edge {
			if len(rs) >= minGram {
				for n := minGram; n <= maxGram && n <= len(rs); n++ {
					nt := base
					if back {
						nt.term = string(rs[len(rs)-n:])
					} else {
						nt.term = string(rs[:n])
					}
					out = append(out, nt)
				}
				if preserve {
					out = append(out, base)
				}
			}
			continue
		}
		for k := 0; k < len(rs); k++ {
			for n := minGram; n <= maxGram && k+n <= len(rs); n++ {
				nt := base
				nt.term = string(rs[k : k+n])
				out = append(out, nt)
			}
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
	if diff > 3 {
		reason := "In Shingle TokenFilter the difference between max_shingle_size and min_shingle_size (and +1 if " +
			"outputting unigrams) must be less than or equal to: [3] but was [" + strconv.Itoa(diff) + "]. " +
			"This limit can be set by changing the [index.max_shingle_diff] index level setting."
		return nil, filterError(reason)
	}
	if len(toks) == 0 {
		return toks, nil
	}
	minPos, maxPos := toks[0].pos, toks[0].pos
	real := map[int]analyzedToken{}
	order := []int{}
	for _, t := range toks {
		if t.pos < minPos {
			minPos = t.pos
		}
		if t.pos > maxPos {
			maxPos = t.pos
		}
		if _, ok := real[t.pos]; !ok {
			real[t.pos] = t
			order = append(order, t.pos)
		}
	}
	if len(order) < minS && outputUnigramsIfNoShingles && !outputUnigrams {
		return toks, nil
	}
	type slot struct {
		tok     analyzedToken
		isFill  bool
		present bool
	}
	slots := map[int]slot{}
	for pos, t := range real {
		slots[pos] = slot{tok: t, present: true}
	}
	// Fill gaps with filler tokens.
	for pos := minPos; pos <= maxPos; pos++ {
		if _, ok := slots[pos]; ok {
			continue
		}
		prev, hasPrev := real[pos-1]
		next, hasNext := real[pos+1]
		if !hasPrev {
			for q := pos + 1; q <= maxPos; q++ {
				if r, ok := real[q]; ok {
					next, hasNext = r, true
					break
				}
			}
		}
		if !hasNext {
			for q := pos - 1; q >= minPos; q-- {
				if r, ok := real[q]; ok {
					prev, hasPrev = r, true
					break
				}
			}
		}
		ft := analyzedToken{term: filler, typ: "word", pos: pos}
		switch {
		case hasNext && hasPrev:
			ft.start, ft.end = next.start, next.start
		case hasNext:
			ft.start, ft.end = next.start, next.end
		case hasPrev:
			ft.start, ft.end = prev.end, prev.end
		}
		slots[pos] = slot{tok: ft, isFill: true, present: true}
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
			for k := 0; k < size; k++ {
				sl := slots[pos+k]
				terms = append(terms, sl.tok.term)
				if sl.isFill {
					fillerUsed = true
				}
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
	sep := fsString(p, "token_separator", "_")
	out := []analyzedToken{}
	for i, t := range toks {
		key := t.term
		if fsBool(p, "ignore_case", false) {
			key = strings.ToLower(key)
		}
		isCommon := common[key]
		if !query || !isCommon {
			out = append(out, t)
		}
		if isCommon && i+1 < len(toks) {
			next := toks[i+1]
			if next.pos == t.pos+1 {
				pl := 2
				if query {
					pl = 0
				}
				out = append(out, analyzedToken{
					term: t.term + sep + next.term, start: t.start, end: next.end,
					pos: t.pos, typ: "gram", posLen: pl,
				})
			}
		}
	}
	if query {
		// Drop common unigrams only; non-common kept above.
	}
	return out
}

func cjkWidthFilter(toks []analyzedToken) []analyzedToken {
	convert := func(r rune) rune {
		switch {
		case r >= 0xFF01 && r <= 0xFF5E:
			return r - 0xFEE0
		case r == 0x3000:
			return ' '
		case r >= 0xFF66 && r <= 0xFF9D:
			// halfwidth katakana to fullwidth
			table := map[rune]rune{
				'ｦ': 'ヲ', 'ｧ': 'ァ', 'ｨ': 'ィ', 'ｩ': 'ゥ', 'ｪ': 'ェ', 'ｫ': 'ォ',
				'ｬ': 'ャ', 'ｭ': 'ュ', 'ｮ': 'ョ', 'ｯ': 'ッ', 'ｰ': 'ー',
				'ｱ': 'ア', 'ｲ': 'イ', 'ｳ': 'ウ', 'ｴ': 'エ', 'ｵ': 'オ',
				'ｶ': 'カ', 'ｷ': 'キ', 'ｸ': 'ク', 'ｹ': 'ケ', 'ｺ': 'コ',
				'ｻ': 'サ', 'ｼ': 'シ', 'ｽ': 'ス', 'ｾ': 'セ', 'ｿ': 'ソ',
				'ﾀ': 'タ', 'ﾁ': 'チ', 'ﾂ': 'ツ', 'ﾃ': 'テ', 'ﾄ': 'ト',
				'ﾅ': 'ナ', 'ﾆ': 'ニ', 'ﾇ': 'ヌ', 'ﾈ': 'ネ', 'ﾉ': 'ノ',
				'ﾊ': 'ハ', 'ﾋ': 'ヒ', 'ﾌ': 'フ', 'ﾍ': 'ヘ', 'ﾎ': 'ホ',
				'ﾏ': 'マ', 'ﾐ': 'ミ', 'ﾑ': 'ム', 'ﾒ': 'メ', 'ﾓ': 'モ',
				'ﾔ': 'ヤ', 'ﾕ': 'ユ', 'ﾖ': 'ヨ',
				'ﾗ': 'ラ', 'ﾘ': 'リ', 'ﾙ': 'ル', 'ﾚ': 'レ', 'ﾛ': 'ロ',
				'ﾜ': 'ワ', 'ﾝ': 'ン', 'ﾞ': '゛', 'ﾟ': '゜',
			}
			if v, ok := table[r]; ok {
				return v
			}
		}
		return r
	}
	out := make([]analyzedToken, len(toks))
	for i, t := range toks {
		out[i] = t
		out[i].term = translateRunes(t.term, convert)
	}
	return out
}

func cjkBigramFilter(toks []analyzedToken, p map[string]any) []analyzedToken {
	outputUnigrams := fsBool(p, "output_unigrams", false)
	ignored := map[string]bool{}
	for _, s := range fsStrings(p, "ignored_scripts") {
		ignored[strings.ToLower(s)] = true
	}
	isCJKToken := func(t analyzedToken) bool {
		switch t.typ {
		case "<IDEOGRAPHIC>", "<HANGUL>", "<HIRAGANA>", "<KATAKANA>":
			return true
		}
		return false
	}
	type ch struct {
		r          rune
		start, end int
	}
	out := []analyzedToken{}
	var run []ch
	nextPos := 0
	flush := func() {
		if len(run) == 0 {
			return
		}
		base := nextPos
		for i, c := range run {
			pos := base + i
			if outputUnigrams {
				out = append(out, analyzedToken{term: string(c.r), start: c.start, end: c.end, pos: pos, typ: "<SINGLE>"})
			}
			if i+1 < len(run) {
				n := run[i+1]
				out = append(out, analyzedToken{term: string(c.r) + string(n.r), start: c.start, end: n.end, pos: pos, typ: "<DOUBLE>", posLen: 2})
			}
		}
		if outputUnigrams {
			nextPos += len(run)
		} else if len(run) > 1 {
			nextPos += len(run) - 1
		}
		run = run[:0]
	}
	for _, t := range toks {
		script := ""
		switch t.typ {
		case "<KATAKANA>":
			script = "katakana"
		case "<HANGUL>":
			script = "hangul"
		case "<HIRAGANA>":
			script = "hiragana"
		case "<IDEOGRAPHIC>":
			script = "han"
		}
		if !isCJKToken(t) || ignored[script] {
			flush()
			t.pos = nextPos
			nextPos++
			out = append(out, t)
			continue
		}
		off := t.start
		for _, r := range []rune(t.term) {
			c := ch{r: r, start: off, end: off + runeWidth(r)}
			if len(run) > 0 && c.start != run[len(run)-1].end {
				flush()
			}
			run = append(run, c)
			off += runeWidth(r)
		}
	}
	flush()
	return out
}

// inlineSynonymFilter implements the synonym/synonym_graph token filters as
// configured inline in an _analyze request.
func inlineSynonymFilter(toks []analyzedToken, p map[string]any, graph bool) ([]analyzedToken, *apiError) {
	if fsString(p, "synonyms_path", "") != "" {
		return nil, filterError("IOException while reading synonyms_path_path: file not readable")
	}
	lines := fsStrings(p, "synonyms")
	format := fsString(p, "format", "")
	expand := fsBool(p, "expand", true)

	type group struct {
		members  [][]string // each member is a phrase of words
		explicit bool
		rhs      [][]string
	}
	var groups []group
	if format == "wordnet" {
		synsets := map[string][][]string{}
		order := []string{}
		for _, line := range lines {
			line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), "."))
			if !strings.HasPrefix(line, "s(") {
				continue
			}
			inner := strings.TrimSuffix(strings.TrimPrefix(line, "s("), ")")
			fields := strings.Split(inner, ",")
			if len(fields) < 3 {
				continue
			}
			id := fields[0]
			word := strings.ToLower(strings.Trim(fields[2], "'"))
			if _, ok := synsets[id]; !ok {
				order = append(order, id)
			}
			synsets[id] = append(synsets[id], []string{word})
		}
		for _, id := range order {
			groups = append(groups, group{members: synsets[id]})
		}
	} else {
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if strings.Count(line, "=>") > 1 {
				return nil, filterError("Failed to build synonyms")
			}
			if strings.Contains(line, "=>") {
				parts := strings.SplitN(line, "=>", 2)
				lhs := splitSynonymPhrases(parts[0])
				rhs := splitSynonymPhrases(parts[1])
				if len(rhs) == 0 || len(lhs) == 0 {
					return nil, filterError("Failed to build synonyms")
				}
				groups = append(groups, group{members: lhs, explicit: true, rhs: rhs})
				continue
			}
			members := splitSynonymPhrases(line)
			if len(members) == 0 {
				return nil, filterError("Failed to build synonyms")
			}
			groups = append(groups, group{members: members})
		}
	}

	originals := make([]analyzedToken, 0, len(toks))
	generated := []analyzedToken{}
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		matched := false
		for _, g := range groups {
			// Find the longest member matching the input at this position.
			bestMember := -1
			bestLen := 0
			for mi, member := range g.members {
				if i+len(member) > len(toks) {
					continue
				}
				ok := true
				for k, w := range member {
					if strings.ToLower(toks[i+k].term) != w {
						ok = false
						break
					}
				}
				if ok && len(member) > bestLen {
					bestMember = mi
					bestLen = len(member)
				}
			}
			if bestMember < 0 {
				continue
			}
			matched = true
			spanStart := t.start
			spanEnd := toks[i+bestLen-1].end
			if g.explicit {
				for _, rhsMember := range g.rhs {
					for j, w := range rhsMember {
						generated = append(generated, analyzedToken{
							term: w, start: spanStart, end: spanEnd, pos: t.pos + j, typ: "SYNONYM",
						})
					}
				}
				i += bestLen - 1
				break
			}
			if bestMember == 0 {
				// The input matched the canonical member.
				originals = append(originals, t)
				if !expand {
					generated = append(generated, analyzedToken{term: g.members[0][0], start: t.start, end: t.end, pos: t.pos, typ: "SYNONYM"})
					i += bestLen - 1
					break
				}
				for _, alt := range g.members[1:] {
					if len(alt) == 1 {
						generated = append(generated, analyzedToken{term: alt[0], start: t.start, end: t.end, pos: t.pos, typ: "SYNONYM"})
						continue
					}
					for j, w := range alt {
						gpos := t.pos + j
						gstart, gend := t.start, t.end
						if !graph && j > 0 {
							if i+j < len(toks) {
								gstart, gend = toks[i+j].start, toks[i+j].end
							}
						}
						generated = append(generated, analyzedToken{term: w, start: gstart, end: gend, pos: gpos, typ: "SYNONYM"})
					}
					if graph {
						originals[len(originals)-1].posLen = len(alt)
					}
				}
				i += bestLen - 1
				break
			}
			// The input matched a non-canonical member.
			if bestLen == 1 && !g.explicit {
				// Single-word alternative: keep the original and add the others.
				originals = append(originals, t)
				if expand {
					for mi, alt := range g.members {
						if mi == bestMember || len(alt) != 1 {
							continue
						}
						generated = append(generated, analyzedToken{term: alt[0], start: t.start, end: t.end, pos: t.pos, typ: "SYNONYM"})
					}
				} else {
					generated = append(generated, analyzedToken{term: g.members[0][0], start: t.start, end: t.end, pos: t.pos, typ: "SYNONYM"})
					originals = originals[:len(originals)-1]
				}
				i += bestLen - 1
				break
			}
			// Multi-word alternative: contract to the canonical member.
			keys := g.members[0]
			for j, w := range keys {
				generated = append(generated, analyzedToken{
					term: w, start: spanStart, end: spanEnd, pos: t.pos + j, typ: "SYNONYM",
					posLen: func() int {
						if len(keys) == 1 {
							return bestLen
						}
						return 0
					}(),
				})
			}
			i += bestLen - 1
			break
		}
		if !matched {
			originals = append(originals, t)
		}
	}

	out := append(originals, generated...)
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].pos != out[b].pos {
			return out[a].pos < out[b].pos
		}
		as := out[a].typ == "SYNONYM"
		bs := out[b].typ == "SYNONYM"
		if as == bs {
			return false
		}
		if graph {
			return as
		}
		return !as
	})
	return out, nil
}

func splitSynonymPhrases(s string) [][]string {
	out := [][]string{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		words := strings.Fields(strings.ToLower(part))
		if len(words) == 0 {
			continue
		}
		out = append(out, words)
	}
	return out
}
