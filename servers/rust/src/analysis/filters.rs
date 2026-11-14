//! Token filters. Each filter maps a whole token stream to a new one; tokens
//! carry absolute positions, so filters that drop tokens leave holes the way
//! Lucene's FilteringTokenFilter does.

use std::collections::{BTreeMap, HashMap, HashSet};

use serde_json::{Map, Value};

use super::jregex::{self, JRegex};
use super::script::Script;
use super::stem::{self, Stemmer};
use super::uchar::{self, utf16_len};
use super::{bad, p_bool, p_int, p_list, p_str, specs_of, stopwords_param, AResult, Chain, Ctx, Named, Spec, Stream, Token};
use crate::errors::ApiError;

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum LowerLang {
    Plain,
    Greek,
    Turkish,
    Irish,
}

#[derive(Debug, Clone)]
pub struct StopSet {
    words: HashSet<String>,
    ignore_case: bool,
}

impl StopSet {
    pub fn new(words: Vec<String>, ignore_case: bool) -> StopSet {
        let words = words.into_iter().map(|w| if ignore_case { uchar::lower_str(&w) } else { w }).collect();
        StopSet { words, ignore_case }
    }
    pub fn contains(&self, t: &str) -> bool {
        if self.ignore_case {
            self.words.contains(&uchar::lower_str(t))
        } else {
            self.words.contains(t)
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum Norm {
    German,
    ScandinavianFolding,
    ScandinavianNormalization,
    Arabic,
    Serbian,
    DecimalDigit,
    /// A language normalization filter of `crate::lang` by name.
    Lang(&'static str),
}

#[derive(Debug, Clone)]
pub struct WdOpts {
    split_case: bool,
    split_nums: bool,
    gen_words: bool,
    gen_nums: bool,
    cat_words: bool,
    cat_nums: bool,
    cat_all: bool,
    preserve: bool,
    possessive: bool,
    adjust: bool,
    protected: HashSet<String>,
    types: HashMap<char, u8>,
}

#[derive(Debug, Clone)]
pub struct SynMap {
    rules: HashMap<Vec<String>, (Vec<Vec<String>>, bool)>,
    max_len: usize,
}

#[derive(Debug, Clone)]
pub enum Filter {
    Lowercase(LowerLang),
    Uppercase,
    AsciiFolding(bool),
    Stop(StopSet, bool),
    Length { min: usize, max: usize },
    Truncate(usize),
    Trim,
    Reverse,
    Unique(bool),
    Limit { max: usize },
    Elision { articles: HashSet<String>, ignore_case: bool },
    Apostrophe,
    Classic,
    PatternReplace { re: JRegex, repl: String, all: bool },
    PatternCapture { res: Vec<JRegex>, preserve: bool },
    Fingerprint { sep: String, max: usize },
    Condition { filters: Vec<Named<Filter>>, script: Script },
    Predicate(Script),
    Multiplexer { chains: Vec<Vec<Named<Filter>>>, preserve: bool },
    Decompounder { words: HashSet<String>, min_word: usize, min_sub: usize, max_sub: usize, longest: bool },
    DelimitedPayload(char),
    Noop,
    MinHash,
    KeywordRepeat,
    RemoveDuplicates,
    KeywordMarker { words: HashSet<String>, ignore_case: bool, re: Option<JRegex> },
    StemmerOverride(HashMap<String, String>),
    Stem(Stemmer),
    NGram { min: usize, max: usize, preserve: bool },
    EdgeNGram { min: usize, max: usize, preserve: bool, back: bool },
    Shingle { min: usize, max: usize, unigrams: bool, unigrams_if_none: bool, sep: String, filler: String },
    WordDelimiter(Box<WdOpts>, bool),
    CommonGrams { words: HashSet<String>, ignore_case: bool, query: bool },
    CjkWidth,
    CjkBigram { unigrams: bool, ignored: HashSet<String> },
    Synonym(Box<SynMap>, bool),
    Normalize(Norm),
}

pub fn stemmer_filter(lang: &str) -> AResult<Filter> {
    Ok(Filter::Stem(stem::snowball_for(lang)?))
}

fn word_set(p: &Map<String, Value>, key: &str, ignore_case: bool) -> HashSet<String> {
    p_list(p, key)
        .unwrap_or_default()
        .into_iter()
        .map(|w| if ignore_case { uchar::lower_str(&w) } else { w })
        .collect()
}

fn regex_param(p: &Map<String, Value>, key: &str) -> AResult<Option<JRegex>> {
    match p_str(p, key) {
        None => Ok(None),
        Some(pat) => {
            let flags = jregex::parse_flags(&p_str(p, "flags").unwrap_or_default()).map_err(|e| bad(&e))?;
            Ok(Some(super::tokenizers::compile_regex(&pat, flags)?))
        }
    }
}

const FRENCH_ARTICLES: &[&str] = &["l", "m", "t", "qu", "n", "s", "j", "d", "c", "jusqu", "quoiqu", "lorsqu", "puisqu"];

/// Builds a filter chain from names/inline definitions (for condition and
/// multiplexer sub-filters).
fn sub_chain(specs: &[Spec], ctx: &Ctx, chain: &Chain) -> AResult<Vec<Named<Filter>>> {
    let mut out = Vec::new();
    let mut c = chain.clone();
    for s in specs {
        let f = ctx.filter(s, &c)?;
        c.filters.push(f.clone());
        out.push(f);
    }
    Ok(out)
}

/// Builds a token filter of the given type; Ok(None) for unknown types.
pub fn build(typ: &str, p: &Map<String, Value>, ctx: &Ctx, chain: &Chain) -> AResult<Option<Filter>> {
    Ok(Some(match typ {
        "lowercase" => match p_str(p, "language").as_deref() {
            Some("greek") => Filter::Lowercase(LowerLang::Greek),
            Some("turkish") => Filter::Lowercase(LowerLang::Turkish),
            Some("irish") => Filter::Lowercase(LowerLang::Irish),
            Some(_) | None => Filter::Lowercase(LowerLang::Plain),
        },
        "uppercase" => Filter::Uppercase,
        "asciifolding" => Filter::AsciiFolding(p_bool(p, "preserve_original", false)),
        "stop" => {
            let words = stopwords_param(p, super::stopwords::ENGLISH)?;
            Filter::Stop(StopSet::new(words, p_bool(p, "ignore_case", false)), p_bool(p, "remove_trailing", true))
        }
        "length" => Filter::Length {
            min: p_int(p, "min", 0).max(0) as usize,
            max: p_int(p, "max", i32::MAX as i64).max(0) as usize,
        },
        "truncate" => {
            let n = p_int(p, "length", 10);
            if n < 1 {
                return Err(bad("length parameter must be provided"));
            }
            Filter::Truncate(n as usize)
        }
        "trim" => Filter::Trim,
        "reverse" => Filter::Reverse,
        "unique" => Filter::Unique(p_bool(p, "only_on_same_position", false)),
        "limit" => Filter::Limit { max: p_int(p, "max_token_count", 1).max(0) as usize },
        "elision" => {
            let ic = p_bool(p, "articles_case", false);
            match p_list(p, "articles") {
                Some(_) => Filter::Elision { articles: word_set(p, "articles", ic), ignore_case: ic },
                None => {
                    if p.contains_key("articles_path") {
                        return Err(bad("IOException while reading articles_path: file not readable"));
                    }
                    Filter::Elision { articles: FRENCH_ARTICLES.iter().map(|s| s.to_string()).collect(), ignore_case: true }
                }
            }
        }
        "apostrophe" => Filter::Apostrophe,
        "classic" => Filter::Classic,
        "pattern_replace" => {
            let re = match regex_param(p, "pattern")? {
                Some(r) => r,
                None => return Err(bad("pattern is missing for [pattern_replace] token filter")),
            };
            Filter::PatternReplace { re, repl: p_str(p, "replacement").unwrap_or_default(), all: p_bool(p, "all", true) }
        }
        "pattern_capture" => {
            let mut res = Vec::new();
            let flags = jregex::parse_flags(&p_str(p, "flags").unwrap_or_default()).map_err(|e| bad(&e))?;
            for pat in p_list(p, "patterns").unwrap_or_default() {
                res.push(super::tokenizers::compile_regex(&pat, flags)?);
            }
            if res.is_empty() {
                return Err(bad("required setting 'patterns' is missing for token filter [pattern_capture]"));
            }
            Filter::PatternCapture { res, preserve: p_bool(p, "preserve_original", true) }
        }
        "fingerprint" => Filter::Fingerprint {
            sep: p_str(p, "separator").unwrap_or_else(|| " ".to_string()),
            max: p_int(p, "max_output_size", 255).max(0) as usize,
        },
        "condition" | "predicate_token_filter" => {
            let src = match p.get("script") {
                Some(Value::Object(m)) => m.get("source").and_then(|v| v.as_str()).unwrap_or("").to_string(),
                Some(Value::String(s)) => s.clone(),
                _ => return Err(bad(&format!("[script] is required for [{}]", typ))),
            };
            let script = Script::parse(&src).map_err(|e| ApiError::new("script_exception", &format!("compile error: {}", e), 400))?;
            if typ == "condition" {
                let specs = specs_of(p.get("filter"));
                Filter::Condition { filters: sub_chain(&specs, ctx, chain)?, script }
            } else {
                Filter::Predicate(script)
            }
        }
        "multiplexer" => {
            let mut chains = Vec::new();
            for entry in p_list(p, "filters").unwrap_or_default() {
                let specs: Vec<Spec> = entry.split(',').map(|s| s.trim()).filter(|s| !s.is_empty()).map(|s| Spec::Name(s.to_string())).collect();
                chains.push(sub_chain(&specs, ctx, chain)?);
            }
            Filter::Multiplexer { chains, preserve: p_bool(p, "preserve_original", true) }
        }
        "dictionary_decompounder" | "hyphenation_decompounder" => {
            if typ == "hyphenation_decompounder" {
                if p.contains_key("hyphenation_patterns_path") {
                    return Err(bad("Exception while reading hyphenation_patterns_path."));
                }
                return Err(bad("hyphenation_patterns_path is a required setting."));
            }
            let words = match p_list(p, "word_list") {
                Some(_) => word_set(p, "word_list", false),
                None => {
                    if p.contains_key("word_list_path") {
                        return Err(bad("IOException while reading word_list_path: file not readable"));
                    }
                    return Err(bad("word_list must be provided for [dictionary_decompounder], either as a path to a file, or directly"));
                }
            };
            Filter::Decompounder {
                words,
                min_word: p_int(p, "min_word_size", 5).max(0) as usize,
                min_sub: p_int(p, "min_subword_size", 2).max(0) as usize,
                max_sub: p_int(p, "max_subword_size", 15).max(0) as usize,
                longest: p_bool(p, "only_longest_match", false),
            }
        }
        "delimited_payload" | "delimited_payload_filter" => {
            let d = p_str(p, "delimiter").unwrap_or_else(|| "|".to_string());
            Filter::DelimitedPayload(d.chars().next().unwrap_or('|'))
        }
        "flatten_graph" => Filter::Noop,
        "min_hash" => Filter::MinHash,
        "hunspell" => {
            let locale = p_str(p, "locale").or_else(|| p_str(p, "language")).or_else(|| p_str(p, "lang")).unwrap_or_default();
            return Err(ApiError::new(
                "illegal_state_exception",
                &format!("Failed to load hunspell dictionary for locale: {}", locale),
                500,
            ));
        }
        "keyword_repeat" => Filter::KeywordRepeat,
        "remove_duplicates" => Filter::RemoveDuplicates,
        "keyword_marker" => {
            let ic = p_bool(p, "ignore_case", false);
            let re = regex_param(p, "keywords_pattern")?;
            if p_list(p, "keywords").is_some() && re.is_some() {
                return Err(bad("cannot specify both `keywords_pattern` and `keywords` or `keywords_path`"));
            }
            Filter::KeywordMarker { words: word_set(p, "keywords", ic), ignore_case: ic, re }
        }
        "stemmer_override" => {
            let mut rules = HashMap::new();
            for line in p_list(p, "rules").unwrap_or_default() {
                let parts: Vec<&str> = line.split("=>").collect();
                if parts.len() != 2 {
                    return Err(bad(&format!("Invalid Keyword override Rule:{}", line)));
                }
                let to = parts[1].trim().to_string();
                for from in parts[0].split(',') {
                    let f = from.trim();
                    if !f.is_empty() && !rules.contains_key(f) {
                        rules.insert(f.to_string(), to.clone());
                    }
                }
            }
            Filter::StemmerOverride(rules)
        }
        "porter_stem" => Filter::Stem(Stemmer::Porter),
        "kstem" => Filter::Stem(stem::stemmer_for("kstem")?),
        "stemmer" => {
            let lang = p_str(p, "language").or_else(|| p_str(p, "name")).unwrap_or_else(|| "english".to_string());
            Filter::Stem(stem::stemmer_for(&lang)?)
        }
        "snowball" => Filter::Stem(stem::snowball_for(&p_str(p, "language").unwrap_or_else(|| "English".to_string()))?),
        "russian_stem" | "czech_stem" | "french_stem" | "german_stem" | "dutch_stem" | "brazilian_stem" | "arabic_stem"
        | "persian_stem" | "greek_stem" | "hindi_stem" | "bengali_stem" | "sorani_stem" | "latvian_stem"
        | "indonesian_stem" | "galician_stem" | "portuguese_stem" | "spanish_stem" => {
            // german_stem is Lucene's GermanStemFilter, not the German Snowball stemmer.
            let lang = if typ == "german_stem" { typ } else { typ.trim_end_matches("_stem") };
            Filter::Stem(stem::stemmer_for(lang)?)
        }
        "ngram" | "nGram" | "edge_ngram" | "edgeNGram" => {
            let edge = typ.starts_with('e');
            let min = p_int(p, "min_gram", 1);
            let max = p_int(p, "max_gram", if edge { 1 } else { 2 });
            if !edge && max - min > ctx.max_ngram_diff {
                return Err(bad(&format!(
                    "The difference between max_gram and min_gram in NGram Tokenizer must be less than or equal to: [{}] but was [{}]. This limit can be set by changing the [index.max_ngram_diff] index level setting.",
                    ctx.max_ngram_diff,
                    max - min
                )));
            }
            if min < 1 {
                return Err(bad("minGram must be greater than zero"));
            }
            if min > max {
                return Err(bad("minGram must not be greater than maxGram"));
            }
            let preserve = p_bool(p, "preserve_original", false);
            if edge {
                let back = p_str(p, "side").map(|s| s == "back").unwrap_or(false);
                Filter::EdgeNGram { min: min as usize, max: max as usize, preserve, back }
            } else {
                Filter::NGram { min: min as usize, max: max as usize, preserve }
            }
        }
        "shingle" => {
            let min = p_int(p, "min_shingle_size", 2);
            let max = p_int(p, "max_shingle_size", 2);
            let unigrams = p_bool(p, "output_unigrams", true);
            let diff = max - min + if unigrams { 1 } else { 0 };
            if diff > ctx.max_shingle_diff {
                return Err(bad(&format!(
                    "In Shingle TokenFilter the difference between max_shingle_size and min_shingle_size (and +1 if outputting unigrams) must be less than or equal to: [{}] but was [{}]. This limit can be set by changing the [index.max_shingle_diff] index level setting.",
                    ctx.max_shingle_diff, diff
                )));
            }
            if max < 2 {
                return Err(bad("Max shingle size must be >= 2"));
            }
            if min < 2 {
                return Err(bad("Min shingle size must be >= 2"));
            }
            if min > max {
                return Err(bad("Min shingle size must be <= max shingle size"));
            }
            Filter::Shingle {
                min: min as usize,
                max: max as usize,
                unigrams,
                unigrams_if_none: p_bool(p, "output_unigrams_if_no_shingles", false),
                sep: p_str(p, "token_separator").unwrap_or_else(|| " ".to_string()),
                filler: p_str(p, "filler_token").unwrap_or_else(|| "_".to_string()),
            }
        }
        "word_delimiter" | "word_delimiter_graph" => {
            let mut types = HashMap::new();
            for rule in p_list(p, "type_table").unwrap_or_default() {
                let parts: Vec<&str> = rule.split("=>").collect();
                if parts.len() != 2 {
                    return Err(bad(&format!("Invalid Mapping Rule : [{}]", rule)));
                }
                let lhs = parts[0].trim();
                let ch = match lhs {
                    "\\t" => '\t',
                    "\\n" => '\n',
                    "\\r" => '\r',
                    "\\\\" => '\\',
                    "\\u200D" => '\u{200D}',
                    _ => {
                        if let Some(h) = lhs.strip_prefix("\\u") {
                            u32::from_str_radix(h, 16).ok().and_then(char::from_u32).ok_or_else(|| bad(&format!("Invalid Mapping Rule : [{}]", rule)))?
                        } else {
                            let cs: Vec<char> = lhs.chars().collect();
                            if cs.len() != 1 {
                                return Err(bad(&format!("Invalid Mapping Rule : [{}]", rule)));
                            }
                            cs[0]
                        }
                    }
                };
                let t = match parts[1].trim() {
                    "LOWER" => WD_LOWER,
                    "UPPER" => WD_UPPER,
                    "ALPHA" => WD_LOWER | WD_UPPER,
                    "DIGIT" => WD_DIGIT,
                    "ALPHANUM" => WD_LOWER | WD_UPPER | WD_DIGIT,
                    "SUBWORD_DELIM" => WD_DELIM,
                    other => return Err(bad(&format!("Invalid Mapping Rule : [{}]", other))),
                };
                types.insert(ch, t);
            }
            Filter::WordDelimiter(
                Box::new(WdOpts {
                    split_case: p_bool(p, "split_on_case_change", true),
                    split_nums: p_bool(p, "split_on_numerics", true),
                    gen_words: p_bool(p, "generate_word_parts", true),
                    gen_nums: p_bool(p, "generate_number_parts", true),
                    cat_words: p_bool(p, "catenate_words", false),
                    cat_nums: p_bool(p, "catenate_numbers", false),
                    cat_all: p_bool(p, "catenate_all", false),
                    preserve: p_bool(p, "preserve_original", false),
                    possessive: p_bool(p, "stem_english_possessive", true),
                    adjust: p_bool(p, "adjust_offsets", true),
                    protected: word_set(p, "protected_words", false),
                    types,
                }),
                typ == "word_delimiter_graph",
            )
        }
        "common_grams" => {
            let ic = p_bool(p, "ignore_case", false);
            if p_list(p, "common_words").is_none() {
                return Err(bad("missing or empty [common_words] or [common_words_path] configuration for common_grams token filter"));
            }
            Filter::CommonGrams { words: word_set(p, "common_words", ic), ignore_case: ic, query: p_bool(p, "query_mode", false) }
        }
        "cjk_width" => Filter::CjkWidth,
        "cjk_bigram" => Filter::CjkBigram {
            unigrams: p_bool(p, "output_unigrams", false),
            ignored: p_list(p, "ignored_scripts").unwrap_or_default().into_iter().collect(),
        },
        "synonym" | "synonym_graph" => {
            if p.contains_key("synonyms_path") && !p.contains_key("synonyms") {
                return Err(bad("IOException while reading synonyms_path_path: file not readable"));
            }
            let rules = p_list(p, "synonyms").unwrap_or_default();
            let expand = p_bool(p, "expand", true);
            let lenient = p_bool(p, "lenient", false);
            let wordnet = p_str(p, "format").map(|f| f == "wordnet").unwrap_or(false);
            let map = build_synonyms(&rules, expand, lenient, wordnet, chain).map_err(|_| bad("Failed to build synonyms"))?;
            Filter::Synonym(Box::new(map), typ == "synonym_graph")
        }
        "german_normalization" => Filter::Normalize(Norm::German),
        "scandinavian_folding" => Filter::Normalize(Norm::ScandinavianFolding),
        "scandinavian_normalization" => Filter::Normalize(Norm::ScandinavianNormalization),
        "arabic_normalization" => Filter::Normalize(Norm::Arabic),
        "serbian_normalization" => Filter::Normalize(Norm::Serbian),
        "decimal_digit" => Filter::Normalize(Norm::DecimalDigit),
        "sorani_normalization" => Filter::Normalize(Norm::Lang("sorani_normalization")),
        "persian_normalization" => Filter::Normalize(Norm::Lang("persian_normalization")),
        "hindi_normalization" => Filter::Normalize(Norm::Lang("hindi_normalization")),
        "indic_normalization" => Filter::Normalize(Norm::Lang("indic_normalization")),
        "bengali_normalization" => Filter::Normalize(Norm::Lang("bengali_normalization")),
        _ => return Ok(None),
    }))
}

/// Whether a filter adds Lucene's KeywordAttribute to the stream.
impl Filter {
    pub fn uses_keyword_attr(&self) -> bool {
        matches!(
            self,
            Filter::KeywordMarker { .. } | Filter::KeywordRepeat | Filter::StemmerOverride(_) | Filter::Stem(_)
        )
    }

    /// Whether a normalizer may use this filter (it works per character).
    pub fn normalizer_safe(&self) -> bool {
        matches!(
            self,
            Filter::Lowercase(_)
                | Filter::Uppercase
                | Filter::AsciiFolding(_)
                | Filter::Normalize(_)
                | Filter::CjkWidth
                | Filter::Elision { .. }
                | Filter::Trim
                | Filter::PatternReplace { .. }
        )
    }
}

// ------------------------------------------------------------------ helpers

/// Position increments of a stream (first token relative to -1).
fn incs(tokens: &[Token]) -> Vec<usize> {
    let mut prev: i64 = -1;
    tokens
        .iter()
        .map(|t| {
            let d = (t.pos as i64 - prev).max(0) as usize;
            prev = t.pos as i64;
            d
        })
        .collect()
}

fn map_terms(mut s: Stream, f: impl Fn(&Token) -> String) -> Stream {
    for t in s.tokens.iter_mut() {
        t.term = f(t);
    }
    s
}

fn ascii_fold(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for c in s.chars() {
        let v = c as u32;
        if v < 0x80 {
            out.push(c);
            continue;
        }
        match super::fold_table::FOLD.binary_search_by_key(&v, |(k, _)| *k) {
            Ok(i) => out.push_str(super::fold_table::FOLD[i].1),
            Err(_) => out.push(c),
        }
    }
    out
}

fn greek_lower(s: &str) -> String {
    s.chars()
        .map(|c| match c {
            'Σ' | 'ς' | 'σ' => 'σ',
            'Ά' | 'ά' => 'α',
            'Έ' | 'έ' => 'ε',
            'Ή' | 'ή' => 'η',
            'Ί' | 'ί' | 'Ϊ' | 'ϊ' | 'ΐ' => 'ι',
            'Ό' | 'ό' => 'ο',
            'Ύ' | 'ύ' | 'Ϋ' | 'ϋ' | 'ΰ' => 'υ',
            'Ώ' | 'ώ' => 'ω',
            _ => uchar::lower_char(c),
        })
        .collect()
}

fn turkish_lower(s: &str) -> String {
    let c: Vec<char> = s.chars().collect();
    let mut out = String::new();
    let mut i = 0;
    while i < c.len() {
        let ch = c[i];
        if ch == 'I' || ch == 'İ' {
            // A following COMBINING DOT ABOVE makes 'I' a dotted i.
            if ch == 'I' && i + 1 < c.len() && c[i + 1] == '\u{307}' {
                out.push('i');
                i += 2;
                continue;
            }
            out.push(if ch == 'I' { 'ı' } else { 'i' });
            i += 1;
            continue;
        }
        out.push(uchar::lower_char(ch));
        i += 1;
    }
    out
}

fn irish_lower(s: &str) -> String {
    let c: Vec<char> = s.chars().collect();
    let is_upper_vowel = |ch: char| "AEIOUÁÉÍÓÚ".contains(ch);
    if c.len() > 1 && (c[0] == 'n' || c[0] == 't') && is_upper_vowel(c[1]) {
        let mut out = String::new();
        out.push(c[0]);
        out.push('-');
        out.push_str(&uchar::lower_str(&c[1..].iter().collect::<String>()));
        return out;
    }
    uchar::lower_str(s)
}

fn german_normalize(s: &str) -> String {
    #[derive(PartialEq)]
    enum St {
        N,
        V,
        U,
    }
    let mut out: Vec<char> = Vec::new();
    let mut state = St::N;
    for c in s.chars() {
        match c {
            'a' | 'o' => {
                state = St::U;
                out.push(c);
            }
            'u' => {
                state = if state == St::N { St::U } else { St::V };
                out.push(c);
            }
            'e' => {
                if state != St::U {
                    out.push(c);
                }
                state = St::V;
            }
            'i' | 'q' | 'y' => {
                state = St::V;
                out.push(c);
            }
            'ä' => {
                out.push('a');
                state = St::V;
            }
            'ö' => {
                out.push('o');
                state = St::V;
            }
            'ü' => {
                out.push('u');
                state = St::V;
            }
            'ß' => {
                out.push('s');
                out.push('s');
                state = St::N;
            }
            _ => {
                out.push(c);
                state = St::N;
            }
        }
    }
    out.into_iter().collect()
}

fn scandinavian(s: &str, fold: bool) -> String {
    let c: Vec<char> = s.chars().collect();
    let mut out = String::new();
    let mut i = 0;
    while i < c.len() {
        let ch = c[i];
        let next = c.get(i + 1).copied();
        if fold {
            match ch {
                'å' | 'à' | 'á' | 'â' | 'ä' | 'æ' => out.push('a'),
                'Å' | 'À' | 'Á' | 'Â' | 'Ä' | 'Æ' => out.push('A'),
                'ö' | 'ø' | 'ô' | 'ò' | 'ó' => out.push('o'),
                'Ö' | 'Ø' | 'Ô' | 'Ò' | 'Ó' => out.push('O'),
                'a' | 'A' if matches!(next, Some('a' | 'A' | 'e' | 'E' | 'o' | 'O')) => {
                    out.push(ch);
                    i += 1;
                }
                'o' | 'O' if matches!(next, Some('e' | 'E' | 'o' | 'O')) => {
                    out.push(ch);
                    i += 1;
                }
                _ => out.push(ch),
            }
        } else {
            match ch {
                'ä' => out.push('æ'),
                'Ä' => out.push('Æ'),
                'ö' => out.push('ø'),
                'Ö' => out.push('Ø'),
                'a' | 'A' if matches!(next, Some('a' | 'A' | 'o' | 'O')) => {
                    out.push(if ch == 'a' { 'å' } else { 'Å' });
                    i += 1;
                }
                'a' | 'A' if matches!(next, Some('e' | 'E')) => {
                    out.push(if ch == 'a' { 'æ' } else { 'Æ' });
                    i += 1;
                }
                'o' | 'O' if matches!(next, Some('e' | 'E' | 'o' | 'O')) => {
                    out.push(if ch == 'o' { 'ø' } else { 'Ø' });
                    i += 1;
                }
                _ => out.push(ch),
            }
        }
        i += 1;
    }
    out
}

fn arabic_normalize(s: &str) -> String {
    s.chars()
        .filter_map(|c| match c {
            'آ' | 'أ' | 'إ' => Some('ا'),
            'ى' => Some('ي'),
            'ة' => Some('ه'),
            'ـ' => None,
            '\u{064B}'..='\u{0652}' => None,
            _ => Some(c),
        })
        .collect()
}

fn serbian_normalize(s: &str) -> String {
    let mut out = String::new();
    for c in s.chars() {
        match c {
            'а' => out.push('a'),
            'б' => out.push('b'),
            'в' => out.push('v'),
            'г' => out.push('g'),
            'д' => out.push('d'),
            'ђ' | 'đ' => out.push_str("dj"),
            'е' => out.push('e'),
            'ж' | 'з' | 'ž' => out.push('z'),
            'и' => out.push('i'),
            'ј' => out.push('j'),
            'к' => out.push('k'),
            'л' => out.push('l'),
            'љ' => out.push_str("lj"),
            'м' => out.push('m'),
            'н' => out.push('n'),
            'њ' => out.push_str("nj"),
            'о' => out.push('o'),
            'п' => out.push('p'),
            'р' => out.push('r'),
            'с' | 'ш' | 'š' => out.push('s'),
            'т' => out.push('t'),
            'ћ' | 'ц' | 'ч' | 'č' | 'ć' => out.push('c'),
            'у' => out.push('u'),
            'ф' => out.push('f'),
            'х' => out.push('h'),
            'џ' => out.push_str("dz"),
            'ǆ' => out.push_str("dz"),
            'ǉ' => out.push_str("lj"),
            'ǌ' => out.push_str("nj"),
            _ => out.push(c),
        }
    }
    out
}

const DIGIT_ZEROS: &[u32] = &[
    0x660, 0x6F0, 0x7C0, 0x966, 0x9E6, 0xA66, 0xAE6, 0xB66, 0xBE6, 0xC66, 0xCE6, 0xD66, 0xDE6, 0xE50,
    0xED0, 0xF20, 0x1040, 0x1090, 0x17E0, 0x1810, 0x1946, 0x19D0, 0x1A80, 0x1A90, 0x1B50, 0x1BB0,
    0x1C40, 0x1C50, 0xA620, 0xA8D0, 0xA900, 0xA9D0, 0xA9F0, 0xAA50, 0xABF0, 0xFF10, 0x104A0, 0x11066,
    0x110F0, 0x11136, 0x111D0, 0x112F0, 0x114D0, 0x11650, 0x116C0, 0x118E0, 0x16A60, 0x16B50,
    0x1D7CE, 0x1D7D8, 0x1D7E2, 0x1D7EC, 0x1D7F6, 0x1E950,
];

fn decimal_digit(s: &str) -> String {
    s.chars()
        .map(|c| {
            let v = c as u32;
            for z in DIGIT_ZEROS {
                if v >= *z && v <= z + 9 {
                    return char::from_u32('0' as u32 + (v - z)).unwrap();
                }
            }
            c
        })
        .collect()
}

fn truncate_utf16(s: &str, n: usize) -> String {
    let mut out = String::new();
    let mut len = 0;
    for c in s.chars() {
        let w = c.len_utf16();
        if len + w > n {
            break;
        }
        len += w;
        out.push(c);
    }
    out
}

// ------------------------------------------------------------------- apply

impl Filter {
    pub fn apply(&self, mut s: Stream) -> Stream {
        match self {
            Filter::Lowercase(lang) => {
                let lang = *lang;
                map_terms(s, |t| match lang {
                    LowerLang::Plain => uchar::lower_str(&t.term),
                    LowerLang::Greek => greek_lower(&t.term),
                    LowerLang::Turkish => turkish_lower(&t.term),
                    LowerLang::Irish => irish_lower(&t.term),
                })
            }
            Filter::Uppercase => map_terms(s, |t| uchar::upper_str(&t.term)),
            Filter::AsciiFolding(preserve) => {
                let mut out = Vec::with_capacity(s.tokens.len());
                for t in s.tokens {
                    let folded = ascii_fold(&t.term);
                    if *preserve && folded != t.term {
                        let mut f = t.clone();
                        f.term = folded;
                        out.push(f);
                        out.push(t);
                    } else {
                        let mut t = t;
                        t.term = folded;
                        out.push(t);
                    }
                }
                s.tokens = out;
                s
            }
            Filter::Stop(set, remove_trailing) => {
                let n = s.tokens.len();
                let last_end = s.tokens.last().map(|t| t.end);
                let toks = std::mem::take(&mut s.tokens);
                s.tokens = toks
                    .into_iter()
                    .enumerate()
                    .filter(|(i, t)| {
                        if !set.contains(&t.term) || t.keyword && false {
                            return true;
                        }
                        // SuggestStopFilter keeps a trailing stop word with no separator after it.
                        !*remove_trailing && *i + 1 == n && Some(t.end) == last_end
                    })
                    .map(|(_, t)| t)
                    .collect();
                s
            }
            Filter::Length { min, max } => {
                s.tokens.retain(|t| {
                    let l = utf16_len(&t.term);
                    l >= *min && l <= *max
                });
                s
            }
            Filter::Truncate(n) => map_terms(s, |t| if t.keyword { t.term.clone() } else { truncate_utf16(&t.term, *n) }),
            Filter::Trim => map_terms(s, |t| t.term.trim_matches(|c: char| uchar::is_java_whitespace(c)).to_string()),
            Filter::Reverse => map_terms(s, |t| t.term.chars().rev().collect()),
            Filter::Unique(same_pos) => {
                let toks = std::mem::take(&mut s.tokens);
                let inc = incs(&toks);
                let mut seen: HashSet<(String, usize)> = HashSet::new();
                let mut kept: Vec<(Token, usize)> = Vec::new();
                for (t, i) in toks.into_iter().zip(inc) {
                    let key = (t.term.clone(), if *same_pos { t.pos } else { 0 });
                    if seen.contains(&key) {
                        continue;
                    }
                    seen.insert(key);
                    kept.push((t, i));
                }
                if *same_pos {
                    s.tokens = kept.into_iter().map(|(t, _)| t).collect();
                } else {
                    // Dropped tokens' increments are lost: positions close up.
                    let mut pos: i64 = -1;
                    for (mut t, i) in kept {
                        pos += i as i64;
                        t.pos = pos.max(0) as usize;
                        s.tokens.push(t);
                    }
                    s.end_pos = s.tokens.last().map(|t| t.pos + 1).unwrap_or(0);
                }
                s
            }
            Filter::Limit { max } => {
                s.tokens.truncate(*max);
                s
            }
            Filter::Elision { articles, ignore_case } => map_terms(s, |t| {
                let c: Vec<char> = t.term.chars().collect();
                if let Some(i) = c.iter().position(|&x| x == '\'' || x == '\u{2019}') {
                    let pre: String = c[..i].iter().collect();
                    let key = if *ignore_case { uchar::lower_str(&pre) } else { pre };
                    if articles.contains(&key) {
                        return c[i + 1..].iter().collect();
                    }
                }
                t.term.clone()
            }),
            Filter::Apostrophe => map_terms(s, |t| match t.term.find(|c| c == '\'' || c == '\u{2019}') {
                Some(i) => t.term[..i].to_string(),
                None => t.term.clone(),
            }),
            Filter::Classic => map_terms(s, |t| match t.typ.as_str() {
                "<APOSTROPHE>" => {
                    let l = t.term.len();
                    if l >= 2 && (t.term.ends_with("'s") || t.term.ends_with("'S")) {
                        t.term[..l - 2].to_string()
                    } else {
                        t.term.clone()
                    }
                }
                "<ACRONYM>" => t.term.replace('.', ""),
                _ => t.term.clone(),
            }),
            Filter::PatternReplace { re, repl, all } => map_terms(s, |t| re.replace(&t.term, repl, *all)),
            Filter::PatternCapture { res, preserve } => {
                let mut out = Vec::new();
                for t in s.tokens {
                    let caps = pattern_captures(res, &t.term, *preserve);
                    if *preserve || caps.is_empty() {
                        out.push(t.clone());
                    }
                    for c in caps {
                        let mut x = t.clone();
                        x.term = c;
                        out.push(x);
                    }
                }
                s.tokens = out;
                s
            }
            Filter::Fingerprint { sep, max } => {
                if s.tokens.is_empty() {
                    return s;
                }
                let end = s.tokens.iter().map(|t| t.end).max().unwrap_or(0);
                let mut terms: Vec<String> = s.tokens.iter().map(|t| t.term.clone()).collect();
                terms.sort();
                terms.dedup();
                let joined = terms.join(sep);
                if utf16_len(&joined) > *max {
                    s.tokens.clear();
                    return s;
                }
                s.tokens = vec![Token::new(joined, 0, end, "fingerprint", 0)];
                s.end_pos = 1;
                s
            }
            Filter::Predicate(script) => {
                let inc = incs(&s.tokens);
                let toks = std::mem::take(&mut s.tokens);
                s.tokens = toks.into_iter().zip(inc).filter(|(t, i)| script.test(t, *i)).map(|(t, _)| t).collect();
                s
            }
            Filter::Condition { filters, script } => {
                let inc = incs(&s.tokens);
                let toks = std::mem::take(&mut s.tokens);
                for (t, i) in toks.into_iter().zip(inc) {
                    if script.test(&t, i) {
                        let mut sub = Stream { tokens: vec![t], end_pos: s.end_pos };
                        for f in filters {
                            sub = f.inner.apply(sub);
                        }
                        s.tokens.extend(sub.tokens);
                    } else {
                        s.tokens.push(t);
                    }
                }
                s
            }
            Filter::Multiplexer { chains, preserve } => {
                let toks = std::mem::take(&mut s.tokens);
                for t in toks {
                    let mut outs: Vec<Token> = Vec::new();
                    if *preserve {
                        outs.push(t.clone());
                    }
                    for ch in chains {
                        let mut sub = Stream { tokens: vec![t.clone()], end_pos: t.pos + 1 };
                        for f in ch {
                            sub = f.inner.apply(sub);
                        }
                        outs.extend(sub.tokens);
                    }
                    let mut seen = HashSet::new();
                    for o in outs {
                        if seen.insert(o.term.clone()) {
                            let mut o = o;
                            o.pos = t.pos;
                            s.tokens.push(o);
                        }
                    }
                }
                s
            }
            Filter::Decompounder { words, min_word, min_sub, max_sub, longest } => {
                let mut out = Vec::new();
                for t in s.tokens {
                    out.push(t.clone());
                    let c: Vec<char> = t.term.chars().collect();
                    if c.len() < *min_word {
                        continue;
                    }
                    let mut i = 0;
                    while i + min_sub <= c.len() {
                        let mut best: Option<String> = None;
                        for j in *min_sub..=*max_sub {
                            if i + j > c.len() {
                                break;
                            }
                            let sub: String = c[i..i + j].iter().collect();
                            if words.contains(&sub) {
                                if *longest {
                                    best = Some(sub);
                                } else {
                                    let mut x = t.clone();
                                    x.term = sub;
                                    out.push(x);
                                }
                            }
                        }
                        if let Some(b) = best {
                            let mut x = t.clone();
                            x.term = b;
                            out.push(x);
                        }
                        i += 1;
                    }
                }
                s.tokens = out;
                s
            }
            Filter::DelimitedPayload(d) => map_terms(s, |t| match t.term.find(*d) {
                Some(i) => t.term[..i].to_string(),
                None => t.term.clone(),
            }),
            Filter::Noop => s,
            Filter::MinHash => {
                if s.tokens.is_empty() {
                    return s;
                }
                let end = s.tokens.iter().map(|t| t.end).max().unwrap_or(0);
                // Deterministic minimum of a 64-bit FNV-1a hash over the terms.
                let mut best = u64::MAX;
                for t in &s.tokens {
                    let mut h: u64 = 0xcbf29ce484222325;
                    for b in t.term.as_bytes() {
                        h ^= *b as u64;
                        h = h.wrapping_mul(0x100000001b3);
                    }
                    best = best.min(h);
                }
                let term: String = (0..4).map(|i| char::from_u32(((best >> (i * 16)) & 0x7FFF) as u32 + 0x100).unwrap_or('x')).collect();
                s.tokens = vec![Token::new(term, 0, end, "MIN_HASH", 0)];
                s.end_pos = 1;
                s
            }
            Filter::KeywordRepeat => {
                let mut out = Vec::new();
                for t in s.tokens {
                    let mut k = t.clone();
                    k.keyword = true;
                    out.push(k);
                    out.push(t);
                }
                s.tokens = out;
                s
            }
            Filter::RemoveDuplicates => {
                let mut seen: HashSet<(String, usize)> = HashSet::new();
                s.tokens.retain(|t| seen.insert((t.term.clone(), t.pos)));
                s
            }
            Filter::KeywordMarker { words, ignore_case, re } => {
                for t in s.tokens.iter_mut() {
                    let key = if *ignore_case { uchar::lower_str(&t.term) } else { t.term.clone() };
                    let m = words.contains(&key)
                        || re.as_ref().map(|r| r.matches(&t.term.chars().collect::<Vec<_>>())).unwrap_or(false);
                    if m {
                        t.keyword = true;
                    }
                }
                s
            }
            Filter::StemmerOverride(rules) => {
                for t in s.tokens.iter_mut() {
                    if t.keyword {
                        continue;
                    }
                    if let Some(v) = rules.get(&t.term) {
                        t.term = v.clone();
                        t.keyword = true;
                    }
                }
                s
            }
            Filter::Stem(st) => {
                for t in s.tokens.iter_mut() {
                    if !t.keyword {
                        t.term = st.stem(&t.term);
                    }
                }
                s
            }
            Filter::NGram { min, max, preserve } => {
                let mut out = Vec::new();
                for t in s.tokens {
                    let c: Vec<char> = t.term.chars().collect();
                    let mut any = false;
                    for st in 0..c.len() {
                        for size in *min..=*max {
                            if st + size > c.len() {
                                break;
                            }
                            let mut x = t.clone();
                            x.term = c[st..st + size].iter().collect();
                            out.push(x);
                            any = true;
                        }
                    }
                    if *preserve && (c.len() < *min || c.len() > *max || !any) {
                        out.push(t);
                    }
                }
                s.tokens = out;
                s
            }
            Filter::EdgeNGram { min, max, preserve, back } => {
                let mut out = Vec::new();
                for t in s.tokens {
                    let c: Vec<char> = t.term.chars().collect();
                    for size in *min..=*max {
                        if size > c.len() {
                            break;
                        }
                        let mut x = t.clone();
                        x.term = if *back { c[c.len() - size..].iter().collect() } else { c[..size].iter().collect() };
                        out.push(x);
                    }
                    if *preserve && (c.len() < *min || c.len() > *max) {
                        out.push(t);
                    }
                }
                s.tokens = out;
                s
            }
            Filter::Shingle { min, max, unigrams, unigrams_if_none, sep, filler } => shingle(s, *min, *max, *unigrams, *unigrams_if_none, sep, filler),
            Filter::WordDelimiter(o, graph) => word_delimiter(s, o, *graph),
            Filter::CommonGrams { words, ignore_case, query } => common_grams(s, words, *ignore_case, *query),
            Filter::CjkWidth => map_terms(s, |t| cjk_width(&t.term)),
            Filter::CjkBigram { unigrams, ignored } => cjk_bigram(s, *unigrams, ignored),
            Filter::Synonym(map, graph) => synonyms(s, map, *graph),
            Filter::Normalize(n) => {
                let n = *n;
                map_terms(s, |t| match n {
                    Norm::German => german_normalize(&t.term),
                    Norm::ScandinavianFolding => scandinavian(&t.term, true),
                    Norm::ScandinavianNormalization => scandinavian(&t.term, false),
                    Norm::Arabic => arabic_normalize(&t.term),
                    Norm::Serbian => serbian_normalize(&t.term),
                    Norm::DecimalDigit => decimal_digit(&t.term),
                    Norm::Lang(name) => crate::lang::chains::normalize_term(name, &t.term),
                })
            }
        }
    }
}

/// Lucene PatternCaptureGroupTokenFilter: the non-empty groups of every
/// match of every pattern, ordered by start offset.
fn pattern_captures(res: &[JRegex], term: &str, preserve: bool) -> Vec<String> {
    let c: Vec<char> = term.chars().collect();
    let mut found: Vec<(usize, usize, String)> = Vec::new();
    for (pi, re) in res.iter().enumerate() {
        for caps in re.find_all(&c) {
            let groups = if re.groups == 0 { vec![0] } else { (1..=re.groups).collect::<Vec<_>>() };
            for g in groups {
                if let Some(Some((s, e))) = caps.get(g) {
                    if s == e || (preserve && *s == 0 && *e == c.len()) {
                        continue;
                    }
                    found.push((*s, pi, c[*s..*e].iter().collect()));
                }
            }
        }
    }
    // Stable sort by start offset (ties keep pattern order).
    found.sort_by_key(|(s, pi, _)| (*s, *pi));
    let mut seen = HashSet::new();
    found.into_iter().filter(|(_, _, t)| seen.insert(t.clone())).map(|(_, _, t)| t).collect()
}

// ------------------------------------------------------------------ shingle

#[allow(clippy::too_many_arguments)]
fn shingle(s: Stream, min: usize, max: usize, unigrams: bool, unigrams_if_none: bool, sep: &str, filler: &str) -> Stream {
    if s.tokens.is_empty() {
        return s;
    }
    // The window of real tokens and fillers for position holes.
    struct W {
        tok: Option<Token>,
        start: usize,
        end: usize,
    }
    let mut win: Vec<W> = Vec::new();
    let inc = incs(&s.tokens);
    let first_pos = s.tokens[0].pos;
    let mut fillers_before = 0;
    for (k, (t, i)) in s.tokens.iter().zip(inc).enumerate() {
        if i > 1 {
            let n = (i - 1).min(max - 1);
            if k == 0 {
                fillers_before = n;
            }
            for _ in 0..n {
                win.push(W { tok: None, start: t.start, end: t.start });
            }
        }
        if i == 0 && k > 0 {
            continue; // stacked tokens are not shingled
        }
        win.push(W { tok: Some(t.clone()), start: t.start, end: t.end });
    }
    let base = first_pos.saturating_sub(fillers_before);
    let text = |w: &W| w.tok.as_ref().map(|t| t.term.clone()).unwrap_or_else(|| filler.to_string());
    let mut out = Vec::new();
    let mut shingles = 0;
    for i in 0..win.len() {
        let pos = base + i;
        if unigrams {
            if let Some(t) = &win[i].tok {
                let mut u = t.clone();
                u.pos = pos;
                out.push(u);
            }
        }
        for size in min.max(2)..=max {
            if i + size > win.len() {
                break;
            }
            if win[i..i + size].iter().all(|w| w.tok.is_none()) {
                continue;
            }
            let term: Vec<String> = win[i..i + size].iter().map(text).collect();
            let mut t = Token::new(term.join(sep), win[i].start, win[i + size - 1].end, "shingle", pos);
            t.pos_len = if unigrams { size } else { (size + 1).saturating_sub(min).max(1) };
            out.push(t);
            shingles += 1;
        }
    }
    if shingles == 0 && !unigrams && unigrams_if_none {
        return s;
    }
    let end_pos = base + win.len();
    Stream { tokens: out, end_pos }
}

// --------------------------------------------------------- word delimiter

const WD_LOWER: u8 = 1;
const WD_UPPER: u8 = 2;
const WD_DIGIT: u8 = 4;
const WD_DELIM: u8 = 8;

fn wd_type(o: &WdOpts, c: char) -> u8 {
    if let Some(t) = o.types.get(&c) {
        return *t;
    }
    if c.is_uppercase() && uchar::is_letter(c) {
        WD_UPPER
    } else if c.is_lowercase() && uchar::is_letter(c) {
        WD_LOWER
    } else if uchar::is_letter(c) {
        WD_LOWER | WD_UPPER
    } else if uchar::is_mark(c) {
        WD_LOWER | WD_UPPER | WD_DIGIT
    } else if c.is_numeric() {
        WD_DIGIT
    } else {
        WD_DELIM
    }
}

fn is_alpha(t: u8) -> bool {
    t & (WD_LOWER | WD_UPPER) != 0
}

fn is_digit_t(t: u8) -> bool {
    t & WD_DIGIT != 0
}

fn is_delim(t: u8) -> bool {
    t & WD_DELIM != 0
}

fn wd_break(o: &WdOpts, last: u8, t: u8) -> bool {
    if t & last != 0 {
        return false;
    }
    if !o.split_case && is_alpha(last) && is_alpha(t) {
        return false;
    }
    if last & WD_UPPER != 0 && is_alpha(t) {
        return false;
    }
    if !o.split_nums && ((is_alpha(last) && is_digit_t(t)) || (is_digit_t(last) && is_alpha(t))) {
        return false;
    }
    true
}

/// Subword spans (char indices) of a term, Lucene's WordDelimiterIterator.
fn wd_parts(o: &WdOpts, c: &[char]) -> (Vec<(usize, usize)>, bool) {
    let types: Vec<u8> = c.iter().map(|&ch| wd_type(o, ch)).collect();
    let len = c.len();
    let ends_possessive = |pos: usize, end_bounds: usize| -> bool {
        o.possessive
            && pos > 2
            && c[pos - 2] == '\''
            && (c[pos - 1] == 's' || c[pos - 1] == 'S')
            && is_alpha(types[pos - 3])
            && (pos == end_bounds || is_delim(types[pos]))
    };
    let mut start_bounds = 0;
    let mut end_bounds = len;
    let final_possessive = ends_possessive(len, len);
    if final_possessive {
        end_bounds -= 2;
    }
    while start_bounds < len && is_delim(types[start_bounds]) {
        start_bounds += 1;
    }
    while end_bounds > start_bounds && is_delim(types[end_bounds - 1]) {
        end_bounds -= 1;
    }
    let mut parts = Vec::new();
    let mut current = start_bounds;
    let mut skip_possessive = false;
    loop {
        if skip_possessive {
            current += 2;
            skip_possessive = false;
        }
        while current < end_bounds && is_delim(types[current]) {
            current += 1;
        }
        if current >= end_bounds {
            break;
        }
        let mut last = types[current];
        let mut end = current + 1;
        while end < end_bounds {
            let t = types[end];
            if wd_break(o, last, t) {
                break;
            }
            last = t;
            end += 1;
        }
        if end + 1 < end_bounds && ends_possessive(end + 2, end_bounds) {
            skip_possessive = true;
        }
        parts.push((current, end));
        current = end;
    }
    let single = parts.len() == 1 && parts[0] == (0, len) && !final_possessive;
    (parts, single)
}

fn word_delimiter(s: Stream, o: &WdOpts, graph: bool) -> Stream {
    let mut out: Vec<Token> = Vec::new();
    let mut shift: i64 = 0;
    for t in s.tokens {
        let base = (t.pos as i64 + shift).max(0) as usize;
        if o.protected.contains(&t.term) {
            let mut x = t.clone();
            x.pos = base;
            out.push(x);
            continue;
        }
        let c: Vec<char> = t.term.chars().collect();
        let (parts, single) = wd_parts(o, &c);
        if single || c.is_empty() {
            let mut x = t.clone();
            x.pos = base;
            out.push(x);
            continue;
        }
        // UTF-16 offsets within the term.
        let mut off = vec![0usize; c.len() + 1];
        for i in 0..c.len() {
            off[i + 1] = off[i] + c[i].len_utf16();
        }
        let can_adjust = o.adjust && t.end >= t.start && t.end - t.start == off[c.len()];
        let span = |a: usize, b: usize| -> (usize, usize) {
            if can_adjust {
                (t.start + off[a], t.start + off[b])
            } else {
                (t.start, t.end)
            }
        };
        let is_num = |p: &(usize, usize)| c[p.0..p.1].iter().all(|ch| is_digit_t(wd_type(o, *ch)));
        // Parts that are emitted, with their position index.
        let emitted: Vec<(usize, usize)> = parts.iter().copied().filter(|p| if is_num(p) { o.gen_nums } else { o.gen_words }).collect();
        // Concatenations: (first part index, last part index) over `parts`.
        let mut cats: Vec<(usize, usize)> = Vec::new();
        if o.cat_all {
            if parts.len() > 1 {
                cats.push((0, parts.len() - 1));
            }
        } else {
            let mut i = 0;
            while i < parts.len() {
                let num = is_num(&parts[i]);
                let mut j = i;
                while j + 1 < parts.len() && is_num(&parts[j + 1]) == num {
                    j += 1;
                }
                if j > i && ((num && o.cat_nums) || (!num && o.cat_words)) {
                    cats.push((i, j));
                }
                i = j + 1;
            }
        }
        if emitted.is_empty() && cats.is_empty() && !o.preserve {
            shift -= 1;
            continue;
        }
        // Position of each part: emitted parts take consecutive positions.
        let mut part_pos: Vec<Option<usize>> = vec![None; parts.len()];
        let mut k = 0;
        for (pi, p) in parts.iter().enumerate() {
            if emitted.contains(p) {
                part_pos[pi] = Some(k);
                k += 1;
            }
        }
        let npos = k.max(1);
        let cat_token = |a: usize, b: usize| -> Token {
            let term: String = (a..=b).map(|i| c[parts[i].0..parts[i].1].iter().collect::<String>()).collect();
            let (st, _) = span(parts[a].0, parts[a].1);
            let (_, en) = span(parts[b].0, parts[b].1);
            let p0 = (a..=b).find_map(|i| part_pos[i]).unwrap_or(0);
            let plen = (a..=b).filter(|i| part_pos[*i].is_some()).count().max(1);
            let mut x = Token::new(term, st, en, &t.typ, base + p0);
            x.pos_len = if graph { plen } else { 1 };
            x
        };
        let part_token = |pi: usize| -> Token {
            let (a, b) = parts[pi];
            let (st, en) = span(a, b);
            Token::new(c[a..b].iter().collect(), st, en, &t.typ, base + part_pos[pi].unwrap())
        };
        let mut group: Vec<Token> = Vec::new();
        if o.preserve {
            let mut x = t.clone();
            x.pos = base;
            x.pos_len = if graph { npos } else { 1 };
            group.push(x);
        }
        if graph {
            // Longer spans first at each position.
            let mut cs: Vec<Token> = cats.iter().map(|(a, b)| cat_token(*a, *b)).collect();
            cs.sort_by_key(|x| (x.pos, std::cmp::Reverse(x.pos_len)));
            let mut ps: Vec<Token> = (0..parts.len()).filter(|i| part_pos[*i].is_some()).map(part_token).collect();
            let mut all: Vec<Token> = Vec::new();
            all.append(&mut cs);
            all.append(&mut ps);
            all.sort_by_key(|x| (x.pos, std::cmp::Reverse(x.pos_len)));
            group.extend(all);
        } else {
            for pi in 0..parts.len() {
                if part_pos[pi].is_some() {
                    group.push(part_token(pi));
                }
                for (a, b) in &cats {
                    if (*a..=*b).find(|i| part_pos[*i].is_some()).unwrap_or(*a) == pi {
                        group.push(cat_token(*a, *b));
                    }
                }
            }
            // Concatenations of parts that are not emitted themselves.
            for (a, b) in &cats {
                if (*a..=*b).all(|i| part_pos[i].is_none()) {
                    group.push(cat_token(*a, *b));
                }
            }
        }
        out.extend(group);
        shift += npos as i64 - 1;
    }
    let mut st = Stream { tokens: out, end_pos: (s.end_pos as i64 + shift).max(0) as usize };
    st.fix_end();
    st
}

// ------------------------------------------------------------ common grams

fn common_grams(s: Stream, words: &HashSet<String>, ignore_case: bool, query: bool) -> Stream {
    let is_common = |t: &str| if ignore_case { words.contains(&uchar::lower_str(t)) } else { words.contains(t) };
    let toks = &s.tokens;
    let mut all: Vec<Token> = Vec::new();
    for i in 0..toks.len() {
        all.push(toks[i].clone());
        if i + 1 < toks.len() && (is_common(&toks[i].term) || is_common(&toks[i + 1].term)) {
            let mut g = Token::new(
                format!("{}_{}", toks[i].term, toks[i + 1].term),
                toks[i].start,
                toks[i + 1].end,
                "gram",
                toks[i].pos,
            );
            g.pos_len = 2;
            all.push(g);
        }
    }
    if !query {
        return Stream { tokens: all, end_pos: s.end_pos };
    }
    // CommonGramsQueryFilter: a token is kept unless the next one is a gram;
    // the last token is dropped if the previous kept one was a gram.
    let mut out = Vec::new();
    let mut prev: Option<Token> = None;
    let mut prev_type = String::new();
    for cur in all {
        if prev.is_some() && cur.typ != "gram" {
            let mut p = prev.take().unwrap();
            prev_type = p.typ.clone();
            p.pos_len = 1;
            out.push(p);
        }
        prev = Some(cur);
    }
    if let Some(mut p) = prev {
        if prev_type != "gram" {
            p.pos_len = 1;
            out.push(p);
        }
    }
    Stream { tokens: out, end_pos: s.end_pos }
}

// --------------------------------------------------------------------- CJK

const KANA_NORM: [u32; 59] = [
    0x30fb, 0x30f2, 0x30a1, 0x30a3, 0x30a5, 0x30a7, 0x30a9, 0x30e3, 0x30e5, 0x30e7, 0x30c3, 0x30fc,
    0x30a2, 0x30a4, 0x30a6, 0x30a8, 0x30aa, 0x30ab, 0x30ad, 0x30af, 0x30b1, 0x30b3, 0x30b5, 0x30b7,
    0x30b9, 0x30bb, 0x30bd, 0x30bf, 0x30c1, 0x30c4, 0x30c6, 0x30c8, 0x30ca, 0x30cb, 0x30cc, 0x30cd,
    0x30ce, 0x30cf, 0x30d2, 0x30d5, 0x30d8, 0x30db, 0x30de, 0x30df, 0x30e0, 0x30e1, 0x30e2, 0x30e4,
    0x30e6, 0x30e8, 0x30e9, 0x30ea, 0x30eb, 0x30ec, 0x30ed, 0x30ef, 0x30f3, 0x3099, 0x309a,
];

fn cjk_width(s: &str) -> String {
    let mut out: Vec<char> = Vec::new();
    for c in s.chars() {
        let v = c as u32;
        if (0xFF01..=0xFF5E).contains(&v) {
            out.push(char::from_u32(v - 0xFEE0).unwrap());
        } else if (0xFF65..=0xFF9F).contains(&v) {
            let k = KANA_NORM[(v - 0xFF65) as usize];
            if (k == 0x3099 || k == 0x309A) && !out.is_empty() {
                let prev = *out.last().unwrap() as u32;
                let combined = if k == 0x3099 {
                    // Voiced: カ→ガ etc., and ウ→ヴ.
                    if (0x30AB..=0x30C2).contains(&prev) && prev % 2 == 1 || (0x30C4..=0x30C8).contains(&prev) && prev % 2 == 0 || (0x30CF..=0x30DD).contains(&prev) && (prev - 0x30CF) % 3 == 0 {
                        Some(prev + 1)
                    } else if prev == 0x30A6 {
                        Some(0x30F4)
                    } else {
                        None
                    }
                } else if (0x30CF..=0x30DD).contains(&prev) && (prev - 0x30CF) % 3 == 0 {
                    Some(prev + 2)
                } else {
                    None
                };
                match combined {
                    Some(cv) => {
                        out.pop();
                        out.push(char::from_u32(cv).unwrap());
                    }
                    None => out.push(char::from_u32(k).unwrap()),
                }
            } else {
                out.push(char::from_u32(k).unwrap());
            }
        } else {
            out.push(c);
        }
    }
    out.into_iter().collect()
}

fn cjk_bigram(s: Stream, unigrams: bool, ignored: &HashSet<String>) -> Stream {
    let script_of = |typ: &str| -> Option<&'static str> {
        match typ {
            "<IDEOGRAPHIC>" => Some("han"),
            "<HIRAGANA>" => Some("hiragana"),
            "<KATAKANA>" => Some("katakana"),
            "<HANGUL>" => Some("hangul"),
            _ => None,
        }
    };
    let mut out: Vec<Token> = Vec::new();
    // Buffer of CJK chars: (char, start, end).
    let mut buf: Vec<(char, usize, usize)> = Vec::new();
    let mut pos: i64 = -1;
    let flush = |buf: &mut Vec<(char, usize, usize)>, out: &mut Vec<Token>, pos: &mut i64| {
        if buf.is_empty() {
            return;
        }
        if buf.len() == 1 {
            *pos += 1;
            out.push(Token::new(buf[0].0.to_string(), buf[0].1, buf[0].2, "<SINGLE>", *pos as usize));
        } else {
            for i in 0..buf.len() {
                if unigrams {
                    *pos += 1;
                    out.push(Token::new(buf[i].0.to_string(), buf[i].1, buf[i].2, "<SINGLE>", *pos as usize));
                    if i + 1 < buf.len() {
                        let mut t = Token::new(format!("{}{}", buf[i].0, buf[i + 1].0), buf[i].1, buf[i + 1].2, "<DOUBLE>", *pos as usize);
                        t.pos_len = 2;
                        out.push(t);
                    }
                } else if i + 1 < buf.len() {
                    *pos += 1;
                    out.push(Token::new(format!("{}{}", buf[i].0, buf[i + 1].0), buf[i].1, buf[i + 1].2, "<DOUBLE>", *pos as usize));
                }
            }
        }
        buf.clear();
    };
    let inc = incs(&s.tokens);
    for (t, i) in s.tokens.iter().zip(inc) {
        let cjk = script_of(&t.typ).filter(|sc| !ignored.contains(*sc));
        match cjk {
            Some(_) => {
                if let Some(last) = buf.last() {
                    if last.2 != t.start {
                        flush(&mut buf, &mut out, &mut pos);
                    }
                }
                // Each char of the token (offsets per char when they line up).
                let chars: Vec<char> = t.term.chars().collect();
                let aligned = t.end - t.start == utf16_len(&t.term);
                let mut o = t.start;
                for ch in chars {
                    let w = ch.len_utf16();
                    if aligned {
                        buf.push((ch, o, o + w));
                        o += w;
                    } else {
                        buf.push((ch, t.start, t.end));
                    }
                }
            }
            None => {
                flush(&mut buf, &mut out, &mut pos);
                let mut x = t.clone();
                pos += i.max(1) as i64;
                x.pos = pos as usize;
                out.push(x);
            }
        }
    }
    flush(&mut buf, &mut out, &mut pos);
    let end_pos = (pos + 1).max(0) as usize;
    Stream { tokens: out, end_pos }
}

// ---------------------------------------------------------------- synonyms

fn split_unescaped(s: &str, sep: char) -> Vec<String> {
    let mut out = Vec::new();
    let mut cur = String::new();
    let mut chars = s.chars().peekable();
    while let Some(c) = chars.next() {
        if c == '\\' {
            if let Some(n) = chars.next() {
                cur.push(n);
            }
            continue;
        }
        if c == sep {
            out.push(std::mem::take(&mut cur));
            continue;
        }
        cur.push(c);
    }
    out.push(cur);
    out
}

fn analyze_phrase(chain: &Chain, phrase: &str) -> Result<Vec<String>, String> {
    let s = chain.run(phrase);
    let mut prev: i64 = -1;
    let mut out = Vec::new();
    for t in s.tokens {
        if t.pos as i64 - prev != 1 && !out.is_empty() {
            return Err("position increment != 1".to_string());
        }
        prev = t.pos as i64;
        out.push(t.term);
    }
    if out.is_empty() {
        return Err(format!("term: {} was completely eliminated by analyzer", phrase));
    }
    Ok(out)
}

fn build_synonyms(rules: &[String], expand: bool, lenient: bool, wordnet: bool, chain: &Chain) -> Result<SynMap, String> {
    let mut map = SynMap { rules: HashMap::new(), max_len: 0 };
    let mut add = |map: &mut SynMap, input: Vec<String>, output: Vec<String>, keep: bool| {
        map.max_len = map.max_len.max(input.len());
        let e = map.rules.entry(input).or_insert_with(|| (Vec::new(), false));
        if !e.0.contains(&output) {
            e.0.push(output);
        }
        e.1 |= keep;
    };
    let mut groups: Vec<Vec<String>> = Vec::new();
    if wordnet {
        let mut cur_id = String::new();
        let mut cur: Vec<String> = Vec::new();
        for line in rules {
            let line = line.trim();
            if !line.starts_with("s(") {
                continue;
            }
            let id = line[2..].split(',').next().unwrap_or("").to_string();
            let q1 = line.find('\'').ok_or("bad wordnet line")?;
            let rest = &line[q1 + 1..];
            let mut word = String::new();
            let rc: Vec<char> = rest.chars().collect();
            let mut i = 0;
            while i < rc.len() {
                if rc[i] == '\'' {
                    if i + 1 < rc.len() && rc[i + 1] == '\'' {
                        word.push('\'');
                        i += 2;
                        continue;
                    }
                    break;
                }
                word.push(rc[i]);
                i += 1;
            }
            if id != cur_id && !cur.is_empty() {
                groups.push(std::mem::take(&mut cur));
            }
            cur_id = id;
            cur.push(word);
        }
        if !cur.is_empty() {
            groups.push(cur);
        }
        for g in groups {
            let mut inputs = Vec::new();
            for w in &g {
                match analyze_phrase(chain, w) {
                    Ok(a) => inputs.push(a),
                    Err(e) => {
                        if !lenient {
                            return Err(e);
                        }
                    }
                }
            }
            add_group(&mut map, &inputs, expand, &mut add);
        }
        return Ok(map);
    }
    for line in rules {
        let l = line.trim();
        if l.is_empty() || l.starts_with('#') {
            continue;
        }
        let sides: Vec<&str> = l.split("=>").collect();
        if sides.len() > 2 {
            return Err("more than one explicit mapping specified on the same line".to_string());
        }
        let parse_side = |side: &str| -> Result<Vec<Vec<String>>, String> {
            let mut out = Vec::new();
            for part in split_unescaped(side, ',') {
                let p = part.trim();
                if p.is_empty() {
                    continue;
                }
                match analyze_phrase(chain, p) {
                    Ok(a) => out.push(a),
                    Err(e) => {
                        if !lenient {
                            return Err(e);
                        }
                    }
                }
            }
            Ok(out)
        };
        if sides.len() == 2 {
            let inputs = parse_side(sides[0])?;
            let outputs = parse_side(sides[1])?;
            if outputs.is_empty() || inputs.is_empty() {
                if lenient {
                    continue;
                }
                return Err("invalid rule".to_string());
            }
            for i in &inputs {
                for o in &outputs {
                    add(&mut map, i.clone(), o.clone(), false);
                }
            }
        } else {
            let inputs = parse_side(sides[0])?;
            add_group(&mut map, &inputs, expand, &mut add);
        }
    }
    Ok(map)
}

fn add_group(map: &mut SynMap, inputs: &[Vec<String>], expand: bool, add: &mut impl FnMut(&mut SynMap, Vec<String>, Vec<String>, bool)) {
    if expand {
        for i in 0..inputs.len() {
            for j in 0..inputs.len() {
                if i != j {
                    add(map, inputs[i].clone(), inputs[j].clone(), true);
                }
            }
        }
    } else if !inputs.is_empty() {
        for i in inputs {
            add(map, i.clone(), inputs[0].clone(), false);
        }
    }
}

fn synonyms(s: Stream, map: &SynMap, graph: bool) -> Stream {
    let toks = s.tokens;
    let n = toks.len();
    let mut out: Vec<Token> = Vec::new();
    let mut shift: i64 = 0;
    let mut pending: BTreeMap<usize, Vec<Token>> = BTreeMap::new();
    let mut trailing: Vec<(usize, Token)> = Vec::new();
    let mut i = 0;
    let syn = |term: &str, start: usize, end: usize, pos: usize, pos_len: usize| -> Token {
        let mut t = Token::new(term.to_string(), start, end, "SYNONYM", pos);
        t.pos_len = pos_len;
        t
    };
    while i < n {
        let mut best: Option<(usize, &(Vec<Vec<String>>, bool))> = None;
        for l in (1..=map.max_len.min(n - i)).rev() {
            let key: Vec<String> = toks[i..i + l].iter().map(|t| t.term.clone()).collect();
            if let Some(e) = map.rules.get(&key) {
                best = Some((l, e));
                break;
            }
        }
        let base = (toks[i].pos as i64 + shift).max(0) as usize;
        match best {
            None => {
                let mut t = toks[i].clone();
                t.pos = base;
                out.push(t);
                if let Some(p) = pending.remove(&i) {
                    for mut x in p {
                        x.pos = base;
                        out.push(x);
                    }
                }
                i += 1;
            }
            Some((l, (outputs, keep))) => {
                let start = toks[i].start;
                let end = toks[i + l - 1].end;
                if graph {
                    let m = outputs.iter().map(|o| o.len()).max().unwrap_or(0).max(if *keep { l } else { 0 });
                    for p in 0..m {
                        for o in outputs {
                            if p < o.len() {
                                let plen = if p + 1 == o.len() { m - (o.len() - 1) } else { 1 };
                                out.push(syn(&o[p], start, end, base + p, plen));
                            }
                        }
                        if *keep && p < l {
                            let mut t = toks[i + p].clone();
                            t.pos = base + p;
                            t.pos_len = if p + 1 == l { m - (l - 1) } else { 1 };
                            out.push(t);
                        }
                    }
                    shift += m as i64 - l as i64;
                } else {
                    let mut produced = 0usize;
                    for j in 0..l {
                        let mut stack: Vec<Token> = Vec::new();
                        if *keep {
                            stack.push(toks[i + j].clone());
                        }
                        for o in outputs {
                            if j < o.len() {
                                let last = j + 1 == o.len();
                                let (st, en, plen) = if last && o.len() < l {
                                    (toks[i + j].start, toks[i + l - 1].end, if *keep { l - j } else { 1 })
                                } else {
                                    (toks[i + j].start, toks[i + j].end, 1)
                                };
                                stack.push(syn(&o[j], st, en, 0, plen));
                            }
                        }
                        if let Some(p) = pending.remove(&(i + j)) {
                            stack.extend(p);
                        }
                        if !stack.is_empty() {
                            for mut x in stack {
                                x.pos = base + produced;
                                out.push(x);
                            }
                            produced += 1;
                        }
                    }
                    for o in outputs {
                        for w in l..o.len() {
                            let idx = i + w;
                            if idx < n {
                                pending.entry(idx).or_default().push(syn(&o[w], toks[idx].start, toks[idx].end, 0, 1));
                            } else {
                                trailing.push((idx - n, syn(&o[w], toks[n - 1].start, toks[n - 1].end, 0, 1)));
                            }
                        }
                    }
                    shift += produced as i64 - l as i64;
                }
                i += l;
            }
        }
    }
    let mut end_pos = (s.end_pos as i64 + shift).max(0) as usize;
    if !trailing.is_empty() {
        trailing.sort_by_key(|(k, _)| *k);
        let last = out.last().map(|t| t.pos).unwrap_or(0);
        for (k, mut t) in trailing {
            t.pos = last + 1 + k;
            end_pos = end_pos.max(t.pos + 1);
            out.push(t);
        }
    }
    let mut st = Stream { tokens: out, end_pos };
    st.fix_end();
    st
}
