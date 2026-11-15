package analysis

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"slopensearch/internal/apierr"
)

// tok is the expected shape of one _analyze token.
type tok struct {
	term       string
	start, end int
	typ        string
	pos        int
}

// analyzeGlobal resolves an index-less _analyze request body the way
// httpapi does (tokenizer, else filter/char_filter on keyword, else
// analyzer, else the default) and analyzes its single text.
func analyzeGlobal(t *testing.T, body string) ([]Token, *apierr.Error) {
	t.Helper()
	var b map[string]any
	if err := json.Unmarshal([]byte(body), &b); err != nil {
		t.Fatalf("bad test body: %v", err)
	}
	sc := Scope{}
	filters, aerr := ParseComponents(b["filter"])
	if aerr != nil {
		return nil, aerr
	}
	chars, aerr := ParseComponents(b["char_filter"])
	if aerr != nil {
		return nil, aerr
	}
	var c *Chain
	switch tk := b["tokenizer"].(type) {
	case string:
		c, aerr = sc.BuildRequestChain(tk, nil, chars, filters)
	case map[string]any:
		c, aerr = sc.BuildRequestChain("", tk, chars, filters)
	default:
		if len(filters) > 0 || len(chars) > 0 {
			c, aerr = sc.BuildRequestChain("keyword", nil, chars, filters)
		} else if a, _ := b["analyzer"].(string); a != "" {
			c, aerr = sc.AnalyzerChain(a)
		} else {
			c = sc.DefaultIndexChain()
		}
	}
	if aerr != nil {
		return nil, aerr
	}
	text, _ := b["text"].(string)
	res, aerr := c.Run(nil, text, false)
	if aerr != nil {
		return nil, aerr
	}
	return res.Tokens, nil
}

func fmtToks(toks []tok) string {
	parts := make([]string, len(toks))
	for i, x := range toks {
		parts[i] = fmt.Sprintf("%q[%d-%d,%s,%d]", x.term, x.start, x.end, x.typ, x.pos)
	}
	return strings.Join(parts, " ")
}

// TestAnalyzeMatchesOpenSearch replays index-less _analyze exchanges recorded
// against OpenSearch 3.9 (tests/reference/test_51_analysis.jsonl); case names
// are those of the recording tests.
func TestAnalyzeMatchesOpenSearch(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []tok
	}{
		{"builtin_analyzer_full_token_shape[standard]", `{"analyzer":"standard","text":"The QUICK brown-fox's e-mail: foo@bar.com, 3.14 jumped! the fox"}`, []tok{{"the", 0, 3, "<ALPHANUM>", 0}, {"quick", 4, 9, "<ALPHANUM>", 1}, {"brown", 10, 15, "<ALPHANUM>", 2}, {"fox's", 16, 21, "<ALPHANUM>", 3}, {"e", 22, 23, "<ALPHANUM>", 4}, {"mail", 24, 28, "<ALPHANUM>", 5}, {"foo", 30, 33, "<ALPHANUM>", 6}, {"bar.com", 34, 41, "<ALPHANUM>", 7}, {"3.14", 43, 47, "<NUM>", 8}, {"jumped", 48, 54, "<ALPHANUM>", 9}, {"the", 56, 59, "<ALPHANUM>", 10}, {"fox", 60, 63, "<ALPHANUM>", 11}}},
		{"builtin_analyzer_full_token_shape[simple]", `{"analyzer":"simple","text":"The QUICK brown-fox's e-mail: foo@bar.com, 3.14 jumped! the fox"}`, []tok{{"the", 0, 3, "word", 0}, {"quick", 4, 9, "word", 1}, {"brown", 10, 15, "word", 2}, {"fox", 16, 19, "word", 3}, {"s", 20, 21, "word", 4}, {"e", 22, 23, "word", 5}, {"mail", 24, 28, "word", 6}, {"foo", 30, 33, "word", 7}, {"bar", 34, 37, "word", 8}, {"com", 38, 41, "word", 9}, {"jumped", 48, 54, "word", 10}, {"the", 56, 59, "word", 11}, {"fox", 60, 63, "word", 12}}},
		{"builtin_analyzer_full_token_shape[whitespace]", `{"analyzer":"whitespace","text":"The QUICK brown-fox's e-mail: foo@bar.com, 3.14 jumped! the fox"}`, []tok{{"The", 0, 3, "word", 0}, {"QUICK", 4, 9, "word", 1}, {"brown-fox's", 10, 21, "word", 2}, {"e-mail:", 22, 29, "word", 3}, {"foo@bar.com,", 30, 42, "word", 4}, {"3.14", 43, 47, "word", 5}, {"jumped!", 48, 55, "word", 6}, {"the", 56, 59, "word", 7}, {"fox", 60, 63, "word", 8}}},
		{"builtin_analyzer_full_token_shape[stop]", `{"analyzer":"stop","text":"The QUICK brown-fox's e-mail: foo@bar.com, 3.14 jumped! the fox"}`, []tok{{"quick", 4, 9, "word", 1}, {"brown", 10, 15, "word", 2}, {"fox", 16, 19, "word", 3}, {"s", 20, 21, "word", 4}, {"e", 22, 23, "word", 5}, {"mail", 24, 28, "word", 6}, {"foo", 30, 33, "word", 7}, {"bar", 34, 37, "word", 8}, {"com", 38, 41, "word", 9}, {"jumped", 48, 54, "word", 10}, {"fox", 60, 63, "word", 12}}},
		{"builtin_analyzer_full_token_shape[pattern]", `{"analyzer":"pattern","text":"The QUICK brown-fox's e-mail: foo@bar.com, 3.14 jumped! the fox"}`, []tok{{"the", 0, 3, "word", 0}, {"quick", 4, 9, "word", 1}, {"brown", 10, 15, "word", 2}, {"fox", 16, 19, "word", 3}, {"s", 20, 21, "word", 4}, {"e", 22, 23, "word", 5}, {"mail", 24, 28, "word", 6}, {"foo", 30, 33, "word", 7}, {"bar", 34, 37, "word", 8}, {"com", 38, 41, "word", 9}, {"3", 43, 44, "word", 10}, {"14", 45, 47, "word", 11}, {"jumped", 48, 54, "word", 12}, {"the", 56, 59, "word", 13}, {"fox", 60, 63, "word", 14}}},
		{"builtin_analyzer_full_token_shape[fingerprint]", `{"analyzer":"fingerprint","text":"The QUICK brown-fox's e-mail: foo@bar.com, 3.14 jumped! the fox"}`, []tok{{"3.14 bar.com brown e foo fox fox's jumped mail quick the", 0, 63, "fingerprint", 0}}},
		{"default_analyzer_is_standard_when_nothing_given", `{"text":"Hello World"}`, []tok{{"hello", 0, 5, "<ALPHANUM>", 0}, {"world", 6, 11, "<ALPHANUM>", 1}}},
		{"empty_text_yields_no_tokens", `{"analyzer":"standard","text":""}`, []tok{}},
		{"whitespace_only_text_yields_no_tokens", `{"analyzer":"standard","text":"   \t\n "}`, []tok{}},
		{"language_analyzer_stems[english]", `{"analyzer":"english","text":"The foxes were running quickly through the libraries' gardens"}`, []tok{{"fox", 4, 9, "<ALPHANUM>", 1}, {"were", 10, 14, "<ALPHANUM>", 2}, {"run", 15, 22, "<ALPHANUM>", 3}, {"quickli", 23, 30, "<ALPHANUM>", 4}, {"through", 31, 38, "<ALPHANUM>", 5}, {"librari", 43, 52, "<ALPHANUM>", 7}, {"garden", 54, 61, "<ALPHANUM>", 8}}},
		{"english_analyzer_positions_skip_stopwords", `{"analyzer":"english","text":"the cats are running"}`, []tok{{"cat", 4, 8, "<ALPHANUM>", 1}, {"run", 13, 20, "<ALPHANUM>", 3}}},
		{"standard_tokenizer_corner_cases[emails_urls]", `{"tokenizer":"standard","text":"john.doe@example.com visit https://example.com/path?q=1"}`, []tok{{"john.doe", 0, 8, "<ALPHANUM>", 0}, {"example.com", 9, 20, "<ALPHANUM>", 1}, {"visit", 21, 26, "<ALPHANUM>", 2}, {"https", 27, 32, "<ALPHANUM>", 3}, {"example.com", 35, 46, "<ALPHANUM>", 4}, {"path", 47, 51, "<ALPHANUM>", 5}, {"q", 52, 53, "<ALPHANUM>", 6}, {"1", 54, 55, "<NUM>", 7}}},
		{"standard_tokenizer_corner_cases[numbers]", `{"tokenizer":"standard","text":"1,234.56 and 3.14 and 192.168.0.1 and v2.0"}`, []tok{{"1,234.56", 0, 8, "<NUM>", 0}, {"and", 9, 12, "<ALPHANUM>", 1}, {"3.14", 13, 17, "<NUM>", 2}, {"and", 18, 21, "<ALPHANUM>", 3}, {"192.168.0.1", 22, 33, "<NUM>", 4}, {"and", 34, 37, "<ALPHANUM>", 5}, {"v2.0", 38, 42, "<ALPHANUM>", 6}}},
		{"standard_tokenizer_corner_cases[apostrophes]", `{"tokenizer":"standard","text":"don't O'Reilly's rock'n'roll"}`, []tok{{"don't", 0, 5, "<ALPHANUM>", 0}, {"O'Reilly's", 6, 16, "<ALPHANUM>", 1}, {"rock'n'roll", 17, 28, "<ALPHANUM>", 2}}},
		{"standard_tokenizer_corner_cases[emoji]", `{"tokenizer":"standard","text":"I ❤️ 🍕 pizza 👍🏽"}`, []tok{{"I", 0, 1, "<ALPHANUM>", 0}, {"❤️", 2, 4, "<EMOJI>", 1}, {"🍕", 5, 7, "<EMOJI>", 2}, {"pizza", 8, 13, "<ALPHANUM>", 3}, {"👍🏽", 14, 18, "<EMOJI>", 4}}},
		{"standard_tokenizer_corner_cases[symbols]", `{"tokenizer":"standard","text":"$100 50% #hashtag @mention C++ C#"}`, []tok{{"100", 1, 4, "<NUM>", 0}, {"50", 5, 7, "<NUM>", 1}, {"hashtag", 10, 17, "<ALPHANUM>", 2}, {"mention", 19, 26, "<ALPHANUM>", 3}, {"C", 27, 28, "<ALPHANUM>", 4}, {"C", 31, 32, "<ALPHANUM>", 5}}},
		{"standard_tokenizer_max_token_length", `{"tokenizer":{"type":"standard","max_token_length":5},"text":"abcdefghijkl xyz"}`, []tok{{"abcde", 0, 5, "<ALPHANUM>", 0}, {"fghij", 5, 10, "<ALPHANUM>", 1}, {"kl", 10, 12, "<ALPHANUM>", 2}, {"xyz", 13, 16, "<ALPHANUM>", 3}}},
		{"offsets_are_utf16_code_units", `{"tokenizer":"whitespace","text":"😀a b"}`, []tok{{"😀a", 0, 3, "word", 0}, {"b", 4, 5, "word", 1}}},
		{"named_tokenizer[letter]", `{"tokenizer":"letter","text":"it's 42 jumbo-jet über_cool"}`, []tok{{"it", 0, 2, "word", 0}, {"s", 3, 4, "word", 1}, {"jumbo", 8, 13, "word", 2}, {"jet", 14, 17, "word", 3}, {"über", 18, 22, "word", 4}, {"cool", 23, 27, "word", 5}}},
		{"named_tokenizer[uax_url_email]", `{"tokenizer":"uax_url_email","text":"Email john.doe@example.com or visit https://example.com/a?b=c now"}`, []tok{{"Email", 0, 5, "<ALPHANUM>", 0}, {"john.doe@example.com", 6, 26, "<EMAIL>", 1}, {"or", 27, 29, "<ALPHANUM>", 2}, {"visit", 30, 35, "<ALPHANUM>", 3}, {"https://example.com/a?b=c", 36, 61, "<URL>", 4}, {"now", 62, 65, "<ALPHANUM>", 5}}},
		{"named_tokenizer[path_hierarchy]", `{"tokenizer":"path_hierarchy","text":"/usr/local/bin"}`, []tok{{"/usr", 0, 4, "word", 0}, {"/usr/local", 0, 10, "word", 1}, {"/usr/local/bin", 0, 14, "word", 2}}},
		{"inline_tokenizer[ngram_2_3]", `{"tokenizer":{"type":"ngram","min_gram":2,"max_gram":3},"text":"abcd"}`, []tok{{"ab", 0, 2, "word", 0}, {"abc", 0, 3, "word", 1}, {"bc", 1, 3, "word", 2}, {"bcd", 1, 4, "word", 3}, {"cd", 2, 4, "word", 4}}},
		{"inline_tokenizer[ngram_token_chars]", `{"tokenizer":{"type":"ngram","min_gram":2,"max_gram":3,"token_chars":["letter","digit"]},"text":"ab-12 c"}`, []tok{{"ab", 0, 2, "word", 0}, {"12", 3, 5, "word", 1}}},
		{"inline_tokenizer[edge_ngram_letters]", `{"tokenizer":{"type":"edge_ngram","min_gram":2,"max_gram":4,"token_chars":["letter"]},"text":"Hello World 42"}`, []tok{{"He", 0, 2, "word", 0}, {"Hel", 0, 3, "word", 1}, {"Hell", 0, 4, "word", 2}, {"Wo", 6, 8, "word", 3}, {"Wor", 6, 9, "word", 4}, {"Worl", 6, 10, "word", 5}}},
		{"inline_tokenizer[pattern_comma_drops_empty]", `{"tokenizer":{"type":"pattern","pattern":","},"text":"a,b,,c, d"}`, []tok{{"a", 0, 1, "word", 0}, {"b", 2, 3, "word", 1}, {"c", 5, 6, "word", 2}, {" d", 7, 9, "word", 3}}},
		{"inline_tokenizer[pattern_group_1]", `{"tokenizer":{"type":"pattern","pattern":"\"((?:\\\\\"|[^\"]|\\\\\")+)\"","group":1},"text":"comment \"hello world\" and \"x y\""}`, []tok{{"hello world", 9, 20, "word", 0}, {"x y", 27, 30, "word", 1}}},
		{"inline_tokenizer[pattern_flags]", `{"tokenizer":{"type":"pattern","pattern":"X","flags":"CASE_INSENSITIVE"},"text":"aXbxc"}`, []tok{{"a", 0, 1, "word", 0}, {"b", 2, 3, "word", 1}, {"c", 4, 5, "word", 2}}},
		{"inline_tokenizer[simple_pattern]", `{"tokenizer":{"type":"simple_pattern","pattern":"[0123456789]{3}"},"text":"fd-786-335-514-x"}`, []tok{{"786", 3, 6, "word", 0}, {"335", 7, 10, "word", 1}, {"514", 11, 14, "word", 2}}},
		{"inline_tokenizer[simple_pattern_split_plus]", `{"tokenizer":{"type":"simple_pattern_split","pattern":"_+"},"text":"a__b___c_"}`, []tok{{"a", 0, 1, "word", 0}, {"b", 3, 4, "word", 1}, {"c", 7, 8, "word", 2}}},
		{"inline_tokenizer[char_group_max_len]", `{"tokenizer":{"type":"char_group","tokenize_on_chars":["whitespace"],"max_token_length":3},"text":"abcdefg hi"}`, []tok{{"abc", 0, 3, "word", 0}, {"def", 3, 6, "word", 1}, {"g", 6, 7, "word", 2}, {"hi", 8, 10, "word", 3}}},
		{"inline_tokenizer[whitespace_max_len_splits]", `{"tokenizer":{"type":"whitespace","max_token_length":3},"text":"abcdefg hi"}`, []tok{{"abc", 0, 3, "word", 0}, {"def", 3, 6, "word", 1}, {"g", 6, 7, "word", 2}, {"hi", 8, 10, "word", 3}}},
		{"normalizing_filters[plain_lowercase_greek]", `{"tokenizer":"whitespace","filter":["lowercase"],"text":"ΟΔΥΣΣΕΥΣ"}`, []tok{{"οδυσσευσ", 0, 8, "word", 0}}},
		{"normalizing_filters[turkish_lowercase]", `{"tokenizer":"whitespace","filter":[{"type":"lowercase","language":"turkish"}],"text":"ISTANBUL İstanbul"}`, []tok{{"ıstanbul", 0, 8, "word", 0}, {"istanbul", 9, 17, "word", 1}}},
		{"normalizing_filters[asciifolding]", `{"tokenizer":"whitespace","filter":["asciifolding"],"text":"Açaí à la carte Ærøskøbing ﬁ ß"}`, []tok{{"Acai", 0, 4, "word", 0}, {"a", 5, 6, "word", 1}, {"la", 7, 9, "word", 2}, {"carte", 10, 15, "word", 3}, {"AEroskobing", 16, 26, "word", 4}, {"fi", 27, 28, "word", 5}, {"ss", 29, 30, "word", 6}}},
		{"asciifolding_preserve_original_order_and_positions", `{"tokenizer":"whitespace","filter":[{"type":"asciifolding","preserve_original":true}],"text":"café naive"}`, []tok{{"cafe", 0, 4, "word", 0}, {"café", 0, 4, "word", 0}, {"naive", 5, 10, "word", 1}}},
		{"stop_filter[default_english_case_sensitive]", `{"tokenizer":"whitespace","filter":["stop"],"text":"The quick and the dead is not a fox"}`, []tok{{"The", 0, 3, "word", 0}, {"quick", 4, 9, "word", 1}, {"dead", 18, 22, "word", 4}, {"fox", 32, 35, "word", 8}}},
		{"stop_filter[custom_list]", `{"tokenizer":"whitespace","filter":[{"type":"stop","stopwords":["foo","Bar"]}],"text":"foo bar Bar baz"}`, []tok{{"bar", 4, 7, "word", 1}, {"baz", 12, 15, "word", 3}}},
		{"stop_filter[keep_trailing]", `{"tokenizer":"whitespace","filter":[{"type":"stop","stopwords":"_english_","remove_trailing":false}],"text":"a fox in the"}`, []tok{{"fox", 2, 5, "word", 1}, {"the", 9, 12, "word", 3}}},
		{"stop_filter[remove_trailing_default]", `{"tokenizer":"whitespace","filter":[{"type":"stop","stopwords":"_english_"}],"text":"a fox in the"}`, []tok{{"fox", 2, 5, "word", 1}}},
		{"stop_filter_leaves_position_holes", `{"tokenizer":"whitespace","filter":["stop"],"text":"fox in the hole"}`, []tok{{"fox", 0, 3, "word", 0}, {"hole", 11, 15, "word", 3}}},
		{"stemmer_filters[porter_stem]", `{"tokenizer":"standard","filter":["porter_stem"],"text":"running ponies caresses relational generalization"}`, []tok{{"run", 0, 7, "<ALPHANUM>", 0}, {"poni", 8, 14, "<ALPHANUM>", 1}, {"caress", 15, 23, "<ALPHANUM>", 2}, {"relat", 24, 34, "<ALPHANUM>", 3}, {"gener", 35, 49, "<ALPHANUM>", 4}}},
		{"stemmer_filters[kstem]", `{"tokenizer":"standard","filter":["kstem"],"text":"running ponies caresses relational generalization"}`, []tok{{"running", 0, 7, "<ALPHANUM>", 0}, {"pony", 8, 14, "<ALPHANUM>", 1}, {"caress", 15, 23, "<ALPHANUM>", 2}, {"relational", 24, 34, "<ALPHANUM>", 3}, {"generalization", 35, 49, "<ALPHANUM>", 4}}},
		{"stemmer_filters[snowball_english]", `{"tokenizer":"standard","filter":[{"type":"snowball","language":"English"}],"text":"running ponies caresses relational generalization"}`, []tok{{"run", 0, 7, "<ALPHANUM>", 0}, {"poni", 8, 14, "<ALPHANUM>", 1}, {"caress", 15, 23, "<ALPHANUM>", 2}, {"relat", 24, 34, "<ALPHANUM>", 3}, {"general", 35, 49, "<ALPHANUM>", 4}}},
		{"stemmer_filters[possessive_english]", `{"tokenizer":"standard","filter":[{"type":"stemmer","language":"possessive_english"}],"text":"John's dogs' cat's"}`, []tok{{"John", 0, 6, "<ALPHANUM>", 0}, {"dogs", 7, 11, "<ALPHANUM>", 1}, {"cat", 13, 18, "<ALPHANUM>", 2}}},
		{"keyword_protection_filters[keyword_repeat_dedup]", `{"tokenizer":"whitespace","filter":["keyword_repeat","porter_stem","remove_duplicates"],"text":"running fox"}`, []tok{{"running", 0, 7, "word", 0}, {"run", 0, 7, "word", 0}, {"fox", 8, 11, "word", 1}}},
		{"synonym_filters[expand]", `{"tokenizer":"whitespace","filter":[{"type":"synonym","synonyms":["quick, fast, speedy"]}],"text":"the quick fox"}`, []tok{{"the", 0, 3, "word", 0}, {"quick", 4, 9, "word", 1}, {"fast", 4, 9, "SYNONYM", 1}, {"speedy", 4, 9, "SYNONYM", 1}, {"fox", 10, 13, "word", 2}}},
		{"synonym_filters[synonym_multiword_single_input_squashed]", `{"tokenizer":"whitespace","filter":[{"type":"synonym","synonyms":["ny, new york"]}],"text":"visit ny today"}`, []tok{{"visit", 0, 5, "word", 0}, {"ny", 6, 8, "word", 1}, {"new", 6, 8, "SYNONYM", 1}, {"today", 9, 14, "word", 2}, {"york", 9, 14, "SYNONYM", 2}}},
		{"synonym_filters[synonym_multiword_input]", `{"tokenizer":"whitespace","filter":[{"type":"synonym","synonyms":["ny, new york"]}],"text":"visit new york today"}`, []tok{{"visit", 0, 5, "word", 0}, {"new", 6, 9, "word", 1}, {"ny", 6, 14, "SYNONYM", 1}, {"york", 10, 14, "word", 2}, {"today", 15, 20, "word", 3}}},
		{"synonym_filters[graph_multiword_input]", `{"tokenizer":"whitespace","filter":[{"type":"synonym_graph","synonyms":["ny, new york"]}],"text":"visit new york today"}`, []tok{{"visit", 0, 5, "word", 0}, {"ny", 6, 14, "SYNONYM", 1}, {"new", 6, 9, "word", 1}, {"york", 10, 14, "word", 2}, {"today", 15, 20, "word", 3}}},
		{"shingle_filter[default]", `{"tokenizer":"whitespace","filter":["shingle"],"text":"a b c"}`, []tok{{"a", 0, 1, "word", 0}, {"a b", 0, 3, "shingle", 0}, {"b", 2, 3, "word", 1}, {"b c", 2, 5, "shingle", 1}, {"c", 4, 5, "word", 2}}},
		{"shingle_filter[2_3_no_unigrams]", `{"tokenizer":"whitespace","filter":[{"type":"shingle","min_shingle_size":2,"max_shingle_size":3,"output_unigrams":false}],"text":"a b c d"}`, []tok{{"a b", 0, 3, "shingle", 0}, {"a b c", 0, 5, "shingle", 0}, {"b c", 2, 5, "shingle", 1}, {"b c d", 2, 7, "shingle", 1}, {"c d", 4, 7, "shingle", 2}}},
		{"shingle_filter[filler_token_custom]", `{"tokenizer":"whitespace","filter":["stop",{"type":"shingle","filler_token":"X"}],"text":"a fox in the hole"}`, []tok{{"X fox", 2, 5, "shingle", 0}, {"fox", 2, 5, "word", 1}, {"fox X", 2, 13, "shingle", 1}, {"X hole", 13, 17, "shingle", 2}, {"hole", 13, 17, "word", 3}}},
		{"ngram_token_filters_keep_original_offsets[edge_2_3_drops_short]", `{"tokenizer":"whitespace","filter":[{"type":"edge_ngram","min_gram":2,"max_gram":3}],"text":"abcd e"}`, []tok{{"ab", 0, 4, "word", 0}, {"abc", 0, 4, "word", 0}}},
		{"ngram_token_filters_keep_original_offsets[edge_side_back]", `{"tokenizer":"whitespace","filter":[{"type":"edge_ngram","min_gram":1,"max_gram":2,"side":"back"}],"text":"abc"}`, []tok{{"c", 0, 3, "word", 0}, {"bc", 0, 3, "word", 0}}},
		{"ngram_token_filters_keep_original_offsets[ngram_2_3]", `{"tokenizer":"whitespace","filter":[{"type":"ngram","min_gram":2,"max_gram":3}],"text":"abcd"}`, []tok{{"ab", 0, 4, "word", 0}, {"abc", 0, 4, "word", 0}, {"bc", 0, 4, "word", 0}, {"bcd", 0, 4, "word", 0}, {"cd", 0, 4, "word", 0}}},
		{"word_delimiter_filters[wd_default]", `{"tokenizer":"whitespace","filter":["word_delimiter"],"text":"Wi-Fi PowerShot500 O'Neil's SD500 wi_fi"}`, []tok{{"Wi", 0, 2, "word", 0}, {"Fi", 3, 5, "word", 1}, {"Power", 6, 11, "word", 2}, {"Shot", 11, 15, "word", 3}, {"500", 15, 18, "word", 4}, {"O", 19, 20, "word", 5}, {"Neil", 21, 25, "word", 6}, {"SD", 28, 30, "word", 7}, {"500", 30, 33, "word", 8}, {"wi", 34, 36, "word", 9}, {"fi", 37, 39, "word", 10}}},
		{"misc_token_filters[length]", `{"tokenizer":"whitespace","filter":[{"type":"length","min":2,"max":4}],"text":"a bb ccc dddd eeeee"}`, []tok{{"bb", 2, 4, "word", 1}, {"ccc", 5, 8, "word", 2}, {"dddd", 9, 13, "word", 3}}},
		{"misc_token_filters[length_counts_utf16_units]", `{"tokenizer":"whitespace","filter":[{"type":"length","min":0,"max":2}],"text":"ab abc é😀"}`, []tok{{"ab", 0, 2, "word", 0}}},
		{"misc_token_filters[truncate_utf16_units]", `{"tokenizer":"whitespace","filter":[{"type":"truncate","length":2}],"text":"😀😀😀 éé"}`, []tok{{"😀", 0, 6, "word", 0}, {"éé", 7, 9, "word", 1}}},
		{"misc_token_filters[reverse_surrogates]", `{"tokenizer":"whitespace","filter":["reverse"],"text":"😀a"}`, []tok{{"a😀", 0, 3, "word", 0}}},
		{"misc_token_filters[pattern_replace_groups]", `{"tokenizer":"whitespace","filter":[{"type":"pattern_replace","pattern":"(\\d+)","replacement":"<$1>"}],"text":"a1b22 c"}`, []tok{{"a<1>b<22>", 0, 5, "word", 0}, {"c", 6, 7, "word", 1}}},
		{"misc_token_filters[pattern_replace_first_only]", `{"tokenizer":"whitespace","filter":[{"type":"pattern_replace","pattern":"a","replacement":"","all":false}],"text":"banana"}`, []tok{{"bnana", 0, 6, "word", 0}}},
		{"misc_token_filters[pattern_capture_preserve]", `{"tokenizer":"whitespace","filter":[{"type":"pattern_capture","preserve_original":true,"patterns":["(\\p{Ll}+|\\p{Lu}\\p{Ll}+|\\p{Lu}+)","(\\d+)"]}],"text":"camelCaseABC123"}`, []tok{{"camelCaseABC123", 0, 15, "word", 0}, {"camel", 0, 15, "word", 0}, {"Case", 0, 15, "word", 0}, {"ABC", 0, 15, "word", 0}, {"123", 0, 15, "word", 0}}},
		{"misc_token_filters[pattern_capture_no_preserve]", `{"tokenizer":"whitespace","filter":[{"type":"pattern_capture","preserve_original":false,"patterns":["([a-z]+)@([a-z.]+)"]}],"text":"joe@ex.com"}`, []tok{{"joe", 0, 10, "word", 0}, {"ex.com", 0, 10, "word", 0}}},
		{"char_filters[html_standard_offsets]", `{"tokenizer":"standard","char_filter":["html_strip"],"text":"<p>I&apos;m so <b>happy</b>!</p>"}`, []tok{{"I'm", 3, 11, "<ALPHANUM>", 0}, {"so", 12, 14, "<ALPHANUM>", 1}, {"happy", 18, 27, "<ALPHANUM>", 2}}},
		{"char_filters[mapping_emoticons]", `{"tokenizer":"whitespace","char_filter":[{"type":"mapping","mappings":[":) => _happy_",":( => _sad_"]}],"text":"I'm :) not :("}`, []tok{{"I'm", 0, 3, "word", 0}, {"_happy_", 4, 6, "word", 1}, {"not", 7, 10, "word", 2}, {"_sad_", 11, 13, "word", 3}}},
		{"char_filters[pattern_replace_card]", `{"tokenizer":"standard","char_filter":[{"type":"pattern_replace","pattern":"(\\d+)-(?=\\d)","replacement":"$1_"}],"text":"My credit card is 123-456-789"}`, []tok{{"My", 0, 2, "<ALPHANUM>", 0}, {"credit", 3, 9, "<ALPHANUM>", 1}, {"card", 10, 14, "<ALPHANUM>", 2}, {"is", 15, 17, "<ALPHANUM>", 3}, {"123_456_789", 18, 29, "<NUM>", 4}}},
		{"char_filters[pattern_replace_camel_offsets]", `{"tokenizer":"standard","char_filter":[{"type":"pattern_replace","pattern":"(?<=\\p{Lower})(?=\\p{Upper})","replacement":" "}],"text":"camelCaseString foo"}`, []tok{{"camel", 0, 4, "<ALPHANUM>", 0}, {"Case", 5, 8, "<ALPHANUM>", 1}, {"String", 9, 15, "<ALPHANUM>", 2}, {"foo", 16, 19, "<ALPHANUM>", 3}}},
		{"filter_only_defaults_to_keyword_tokenizer", `{"filter":["lowercase"],"text":"Hello World"}`, []tok{{"hello world", 0, 11, "word", 0}}},
		{"standard_analyzer_scripts[cjk_extb]", `{"analyzer":"standard","text":"𠀀𠀁 𪚥"}`, []tok{{"𠀀", 0, 2, "<IDEOGRAPHIC>", 0}, {"𠀁", 2, 4, "<IDEOGRAPHIC>", 1}, {"𪚥", 5, 7, "<IDEOGRAPHIC>", 2}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := analyzeGlobal(t, tc.body)
			if err != nil {
				t.Fatalf("body %s: %s: %s", tc.body, err.Type, err.Reason)
			}
			gt := make([]tok, len(got))
			for i, g := range got {
				gt[i] = tok{g.Term, g.Start, g.End, g.Type, g.Pos}
			}
			if g, w := fmtToks(gt), fmtToks(tc.want); g != w {
				t.Errorf("body %s\n got: %s\nwant: %s", tc.body, g, w)
			}
		})
	}
}

// TestAnalyzeErrors checks request errors against the recorded OpenSearch
// responses (type and reason prefix).
func TestAnalyzeErrors(t *testing.T) {
	cases := []struct {
		name, body, typ, reason string
	}{
		{"unknown_analyzer", `{"analyzer":"nonexistent","text":"x"}`, "illegal_argument_exception", "failed to find global analyzer [nonexistent]"},
		{"plugin_analyzer_absent", `{"analyzer":"kuromoji","text":"x"}`, "illegal_argument_exception", "failed to find global analyzer [kuromoji]"},
		{"unknown_tokenizer", `{"tokenizer":"nonexistent","text":"x"}`, "illegal_argument_exception", "failed to find global tokenizer under [nonexistent]"},
		{"unknown_inline_tokenizer", `{"tokenizer":{"type":"nope"},"text":"x"}`, "illegal_argument_exception", "failed to find global tokenizer under [nope]"},
		{"unknown_char_filter", `{"tokenizer":"keyword","char_filter":["nonexistent"],"text":"x"}`, "illegal_argument_exception", "failed to find global char_filter under [nonexistent]"},
		{"unknown_filter", `{"tokenizer":"standard","filter":[{"type":"nope"}],"text":"x"}`, "illegal_argument_exception", "failed to find global filter under [nope]"},
		{"anonymous_filter_without_type", `{"tokenizer":"standard","filter":[{"stopwords":["x"]}],"text":"x"}`, "illegal_argument_exception", `Missing [type] setting for anonymous filter: {"stopwords":["x"]}`},
		{"stemmer_unknown_language", `{"tokenizer":"standard","filter":[{"type":"stemmer","language":"klingon"}],"text":"x"}`, "illegal_argument_exception", "Invalid stemmer class specified: Klingon"},
		{"edge_ngram_min_gt_max", `{"tokenizer":{"type":"edge_ngram","min_gram":3,"max_gram":2},"text":"ab"}`, "illegal_argument_exception", "minGram must not be greater than maxGram"},
		{"ngram_diff_limit", `{"tokenizer":{"type":"ngram","min_gram":1,"max_gram":3},"text":"abcd"}`, "illegal_argument_exception", "The difference between max_gram and min_gram in NGram Tokenizer must be less than or equal to: [1] but was [2]."},
		{"ngram_custom_without_chars", `{"tokenizer":{"type":"ngram","min_gram":2,"max_gram":2,"token_chars":["custom"]},"text":"ab"}`, "illegal_argument_exception", "Token type: 'custom' requires setting `custom_token_chars`"},
		{"pattern_tokenizer_bad_regex", `{"tokenizer":{"type":"pattern","pattern":"("},"text":"x"}`, "pattern_syntax_exception", "Unclosed group near index 1"},
		{"shingle_min_gt_max", `{"tokenizer":"whitespace","filter":[{"type":"shingle","min_shingle_size":3,"max_shingle_size":2}],"text":"a b"}`, "illegal_argument_exception", "Min shingle size must be <= max shingle size"},
		{"synonym_bad_rule", `{"tokenizer":"whitespace","filter":[{"type":"synonym","synonyms":["a => b => c"]}],"text":"foo"}`, "illegal_argument_exception", "Failed to build synonyms"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := analyzeGlobal(t, tc.body)
			if err == nil {
				t.Fatalf("body %s: expected %s error, got none", tc.body, tc.typ)
			}
			if err.Type != tc.typ || !strings.HasPrefix(err.Reason, tc.reason) {
				t.Errorf("body %s\n got: %s: %q\nwant: %s: %q...", tc.body, err.Type, err.Reason, tc.typ, tc.reason)
			}
		})
	}
}

func TestUTF16Helpers(t *testing.T) {
	cases := []struct {
		in       string
		width    int
		trunc    int
		truncOut string
	}{
		{"", 0, 3, ""},
		{"abc", 3, 2, "ab"},
		{"é😀", 3, 2, "é"},
		{"😀😀😀", 6, 2, "😀"},
		{"😀😀😀", 6, 3, "😀"},
		{"a𠀀b", 4, 0, ""},
		{"a𠀀b", 4, 10, "a𠀀b"},
	}
	for _, tc := range cases {
		if got := UTF16Offset(tc.in); got != tc.width {
			t.Errorf("UTF16Offset(%q) = %d, want %d", tc.in, got, tc.width)
		}
		if got := TruncateUTF16(tc.in, tc.trunc); got != tc.truncOut {
			t.Errorf("TruncateUTF16(%q, %d) = %q, want %q", tc.in, tc.trunc, got, tc.truncOut)
		}
	}
}
