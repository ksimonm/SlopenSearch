//! Token filters and the built-in language analyzers, wired exactly like
//! Lucene 10.5.1's `createComponents` and OpenSearch's analyzer providers.

use super::case_width::{cjk_width, decimal_digit, irish_lowercase, lowercase, turkish_lowercase, uppercase};
use super::snowball::stem_units;
use super::Tok;
use serde_json::{Map, Value};
use std::collections::{HashMap, HashSet};
use std::sync::OnceLock;

/// A word set like Lucene's CharArraySet.
#[derive(Clone, Debug, Default)]
pub struct WordSet {
    pub ignore_case: bool,
    pub words: HashSet<Vec<u16>>,
}

impl WordSet {
    pub fn new<I: IntoIterator<Item = S>, S: AsRef<str>>(words: I, ignore_case: bool) -> WordSet {
        let mut ws = WordSet { ignore_case, words: HashSet::new() };
        for w in words {
            ws.add(w.as_ref());
        }
        ws
    }
    pub fn add(&mut self, w: &str) {
        let mut u: Vec<u16> = w.encode_utf16().collect();
        if self.ignore_case {
            lowercase(&mut u);
        }
        self.words.insert(u);
    }
    pub fn contains(&self, term: &[u16]) -> bool {
        if self.ignore_case {
            let mut t = term.to_vec();
            lowercase(&mut t);
            self.words.contains(&t)
        } else {
            self.words.contains(term)
        }
    }
    pub fn is_empty(&self) -> bool {
        self.words.is_empty()
    }
}

fn data_sets() -> &'static HashMap<&'static str, (bool, &'static [&'static str])> {
    static M: OnceLock<HashMap<&'static str, (bool, &'static [&'static str])>> = OnceLock::new();
    M.get_or_init(|| {
        super::stopwords_data::WORD_SETS.iter().map(|(n, ic, ws)| (*n, (*ic, *ws))).collect()
    })
}

/// A default word set of Lucene by its data name ("english", "articles_french", ...).
pub fn default_set(name: &str) -> WordSet {
    match data_sets().get(name) {
        Some((ic, ws)) => WordSet::new(ws.iter(), *ic),
        None => WordSet::default(),
    }
}

/// The predefined `_lang_` stop sets of OpenSearch (Analysis.NAMED_STOP_WORDS).
pub const NAMED_STOP_SETS: &[&str] = &[
    "arabic", "armenian", "basque", "bengali", "brazilian", "bulgarian", "catalan", "czech", "danish",
    "dutch", "english", "estonian", "finnish", "french", "galician", "german", "greek", "hindi",
    "hungarian", "indonesian", "irish", "italian", "latvian", "lithuanian", "norwegian", "persian",
    "portuguese", "romanian", "russian", "sorani", "spanish", "swedish", "thai", "turkish",
];

fn settings_list(v: &Value) -> Vec<String> {
    match v {
        Value::Array(a) => a.iter().filter_map(scalar).collect(),
        Value::String(s) => {
            let t = s.trim();
            if t.starts_with('[') {
                if let Ok(Value::Array(a)) = serde_json::from_str::<Value>(t) {
                    return a.iter().filter_map(scalar).collect();
                }
            }
            s.split(',').map(|p| p.trim().to_string()).collect()
        }
        other => scalar(other).into_iter().collect(),
    }
}

fn scalar(v: &Value) -> Option<String> {
    match v {
        Value::String(s) => Some(s.clone()),
        Value::Number(n) => Some(n.to_string()),
        Value::Bool(b) => Some(b.to_string()),
        _ => None,
    }
}

fn setting_bool(p: &Map<String, Value>, key: &str, def: bool) -> bool {
    match p.get(key) {
        Some(Value::Bool(b)) => *b,
        Some(Value::String(s)) => s == "true",
        _ => def,
    }
}

/// Analysis.parseWords for `stopwords`-like settings (named `_lang_` sets resolved).
pub fn parse_stopwords(p: &Map<String, Value>, key: &str, default: WordSet, ignore_case: bool) -> WordSet {
    let Some(v) = p.get(key) else { return default };
    if v.as_str() == Some("_none_") {
        return WordSet::default();
    }
    let mut ws = WordSet { ignore_case, words: HashSet::new() };
    for w in settings_list(v) {
        let named = w
            .strip_prefix('_')
            .and_then(|x| x.strip_suffix('_'))
            .filter(|x| NAMED_STOP_SETS.contains(x));
        match named {
            Some(n) => {
                for word in data_sets()[n].1 {
                    ws.add(word);
                }
            }
            None => ws.add(&w),
        }
    }
    ws
}

/// Analysis.parseStemExclusion.
pub fn parse_stem_exclusion(p: &Map<String, Value>) -> WordSet {
    match p.get("stem_exclusion") {
        None => WordSet::default(),
        Some(v) if v.as_str() == Some("_none_") => WordSet::default(),
        Some(v) => WordSet::new(settings_list(v), false),
    }
}

// ------------------------------------------------------------ filters

pub type Filter = Box<dyn Fn(Vec<Tok>) -> Vec<Tok> + Send + Sync>;

fn units(t: &Tok) -> Vec<u16> {
    t.term.encode_utf16().collect()
}

fn set_units(t: &mut Tok, u: &[u16]) {
    t.term = String::from_utf16_lossy(u);
}

/// Applies a term function to every token.
pub fn map_all(f: fn(&mut Vec<u16>)) -> Filter {
    Box::new(move |mut toks: Vec<Tok>| {
        for t in toks.iter_mut() {
            let mut u = units(t);
            f(&mut u);
            set_units(t, &u);
        }
        toks
    })
}

/// Applies a stemmer to tokens not marked as keywords.
pub fn map_stem(f: fn(&mut Vec<u16>)) -> Filter {
    Box::new(move |mut toks: Vec<Tok>| {
        for t in toks.iter_mut().filter(|t| !t.keyword) {
            let mut u = units(t);
            f(&mut u);
            set_units(t, &u);
        }
        toks
    })
}

/// SnowballFilter with a stemmer class ("English", "German", ...).
pub fn snowball(lang: &'static str) -> Filter {
    Box::new(move |mut toks: Vec<Tok>| {
        let Some(mut st) = super::snowball_gen::new_stemmer(lang) else { return toks };
        for t in toks.iter_mut().filter(|t| !t.keyword) {
            let u = units(t);
            let out = stem_units(st.as_mut(), &u);
            set_units(t, &out);
        }
        toks
    })
}

/// StopFilter: removes the words, adding their position increments to the next token.
pub fn stop(set: WordSet, remove_trailing: bool) -> Filter {
    Box::new(move |toks: Vec<Tok>| {
        let n = toks.len();
        let mut out = Vec::with_capacity(n);
        let mut skipped = 0usize;
        for (i, mut t) in toks.into_iter().enumerate() {
            let u = units(&t);
            let last = i + 1 == n;
            if set.contains(&u) && (remove_trailing || !last) {
                skipped += t.pos_inc;
                continue;
            }
            t.pos_inc += skipped;
            skipped = 0;
            out.push(t);
        }
        out
    })
}

/// SetKeywordMarkerFilter.
pub fn keyword_marker(set: WordSet) -> Filter {
    Box::new(move |mut toks: Vec<Tok>| {
        for t in toks.iter_mut() {
            if set.contains(&units(t)) {
                t.keyword = true;
            }
        }
        toks
    })
}

/// StemmerOverrideFilter (exact-match dictionary, marks the result a keyword).
pub fn stemmer_override(dict: Vec<(&'static str, &'static str)>) -> Filter {
    Box::new(move |mut toks: Vec<Tok>| {
        for t in toks.iter_mut().filter(|t| !t.keyword) {
            if let Some((_, to)) = dict.iter().find(|(from, _)| *from == t.term) {
                t.term = to.to_string();
                t.keyword = true;
            }
        }
        toks
    })
}

/// ElisionFilter.
pub fn elision(articles: WordSet) -> Filter {
    Box::new(move |mut toks: Vec<Tok>| {
        for t in toks.iter_mut() {
            let u = units(t);
            if let Some(i) = u.iter().position(|&c| c == 0x27 || c == 0x2019) {
                if articles.contains(&u[..i]) {
                    set_units(t, &u[i + 1..]);
                }
            }
        }
        toks
    })
}

fn apostrophe(term: &mut Vec<u16>) {
    if let Some(i) = term.iter().position(|&c| c == 0x27 || c == 0x2019) {
        term.truncate(i);
    }
}

fn english_possessive(term: &mut Vec<u16>) {
    let n = term.len();
    if n >= 2 && matches!(term[n - 2], 0x27 | 0x2019 | 0xFF07) && matches!(term[n - 1], 0x73 | 0x53) {
        term.truncate(n - 2);
    }
}

fn romanian_normalize(term: &mut Vec<u16>) {
    for c in term.iter_mut() {
        *c = match *c {
            0x015E => 0x0218,
            0x015F => 0x0219,
            0x0162 => 0x021A,
            0x0163 => 0x021B,
            o => o,
        };
    }
}

fn dutch_stem_override() -> Vec<(&'static str, &'static str)> {
    vec![("fiets", "fiets"), ("bromfiets", "bromfiets"), ("ei", "eier"), ("kind", "kinder")]
}

// ------------------------------------------------------------ CJK bigrams

const CJK_HAN: u8 = 1;
const CJK_HIRAGANA: u8 = 2;
const CJK_KATAKANA: u8 = 4;
const CJK_HANGUL: u8 = 8;

/// CJKBigramFilter: forms bigrams of adjacent CJK characters.
pub fn cjk_bigram(flags: u8, output_unigrams: bool) -> Filter {
    Box::new(move |toks: Vec<Tok>| {
        let mut out: Vec<Tok> = Vec::new();
        // buffered CJK characters: (char string, start, end, pos_inc of the original token)
        let mut buf: Vec<(String, usize, usize, usize)> = Vec::new();
        let flush = |buf: &mut Vec<(String, usize, usize, usize)>, out: &mut Vec<Tok>| {
            if buf.is_empty() {
                return;
            }
            let mk = |term: String, s: usize, e: usize, typ: &str, inc: usize, len: usize| Tok {
                term,
                start: s,
                end: e,
                typ: typ.to_string(),
                pos_inc: inc,
                pos_len: len,
                keyword: false,
            };
            let first_inc = buf[0].3;
            if buf.len() == 1 {
                let (c, s, e, _) = buf[0].clone();
                out.push(mk(c, s, e, "<SINGLE>", first_inc, 1));
            } else if output_unigrams {
                for i in 0..buf.len() {
                    let (c, s, e, _) = buf[i].clone();
                    out.push(mk(c.clone(), s, e, "<SINGLE>", if i == 0 { first_inc } else { 1 }, 1));
                    if i + 1 < buf.len() {
                        let (c2, _, e2, _) = buf[i + 1].clone();
                        out.push(mk(c + &c2, s, e2, "<DOUBLE>", 0, 2));
                    }
                }
            } else {
                for i in 0..buf.len() - 1 {
                    let (c, s, _, _) = buf[i].clone();
                    let (c2, _, e2, _) = buf[i + 1].clone();
                    out.push(mk(c + &c2, s, e2, "<DOUBLE>", if i == 0 { first_inc } else { 1 }, 1));
                }
            }
            buf.clear();
        };
        for t in toks {
            let script = match t.typ.as_str() {
                "<IDEOGRAPHIC>" => CJK_HAN,
                "<HIRAGANA>" => CJK_HIRAGANA,
                "<KATAKANA>" => CJK_KATAKANA,
                "<HANGUL>" => CJK_HANGUL,
                _ => 0,
            };
            if script == 0 || flags & script == 0 {
                flush(&mut buf, &mut out);
                out.push(t);
                continue;
            }
            if let Some(last) = buf.last() {
                if last.2 != t.start {
                    flush(&mut buf, &mut out);
                }
            }
            let u = units(&t);
            let per_char = t.end - t.start == u.len();
            let mut off = 0usize;
            let mut first = true;
            for c in t.term.chars() {
                let n = c.len_utf16();
                let (s, e) = if per_char { (t.start + off, t.start + off + n) } else { (t.start, t.end) };
                buf.push((c.to_string(), s, e, if first { t.pos_inc } else { 1 }));
                first = false;
                off += n;
            }
        }
        flush(&mut buf, &mut out);
        out
    })
}

// ------------------------------------------------------------ stemmer filter

/// The error of StemmerTokenFilterFactory for an unknown language.
pub struct StemmerError {
    pub class: String,
}

fn capitalize(s: &str) -> String {
    let mut c = s.chars();
    match c.next() {
        Some(f) => f.to_uppercase().collect::<String>() + c.as_str(),
        None => String::new(),
    }
}

/// A per-term stemmer: a function, or a Snowball class.
#[derive(Debug, Clone, Copy)]
pub enum TermStem {
    F(fn(&mut Vec<u16>)),
    S(&'static str),
    /// EnglishPossessiveFilter (applies to keyword tokens too).
    P,
}

impl PartialEq for TermStem {
    fn eq(&self, other: &TermStem) -> bool {
        match (self, other) {
            (TermStem::F(a), TermStem::F(b)) => *a as usize == *b as usize,
            (TermStem::S(a), TermStem::S(b)) => a == b,
            (TermStem::P, TermStem::P) => true,
            _ => false,
        }
    }
}

impl TermStem {
    pub fn stem(&self, term: &str) -> String {
        let mut u: Vec<u16> = term.encode_utf16().collect();
        match self {
            TermStem::F(f) => f(&mut u),
            TermStem::P => english_possessive(&mut u),
            TermStem::S(c) => {
                if let Some(mut st) = super::snowball_gen::new_stemmer(c) {
                    u = stem_units(st.as_mut(), &u);
                }
            }
        }
        String::from_utf16_lossy(&u)
    }
}

/// The stemmer of a StemmerTokenFilterFactory `language`, if known here.
pub fn term_stemmer(language: &str) -> Option<TermStem> {
    use super::*;
    let l = language.to_lowercase();
    Some(match l.as_str() {
        "arabic" => TermStem::F(arabic_family::arabic_stem),
        "armenian" => TermStem::S("Armenian"),
        "basque" => TermStem::S("Basque"),
        "bengali" => TermStem::F(indic::bengali_stem),
        "brazilian" => TermStem::F(brazilian::brazilian_stem),
        "bulgarian" => TermStem::F(slavic_baltic::bulgarian_stem),
        "catalan" => TermStem::S("Catalan"),
        "czech" => TermStem::F(slavic_baltic::czech_stem),
        "danish" => TermStem::S("Danish"),
        "dutch" => TermStem::S("Dutch"),
        "english" | "porter" => TermStem::F(porter::porter_stem),
        "light_english" | "lightenglish" | "kstem" => TermStem::F(kstem::kstem),
        "porter2" => TermStem::S("English"),
        "minimal_english" | "minimalenglish" => TermStem::F(english_misc::english_minimal_stem),
        "plural_english" | "pluralenglish" => TermStem::F(english_misc::english_plural_stem),
        "possessive_english" | "possessiveenglish" => TermStem::P,
        "estonian" => TermStem::S("Estonian"),
        "finnish" => TermStem::S("Finnish"),
        "light_finish" | "lightfinish" | "light_finnish" | "lightfinnish" => {
            TermStem::F(other_light_stem::finnish_light_stem)
        }
        "french" => TermStem::S("French"),
        "light_french" | "lightfrench" => TermStem::F(romance_stem::french_light_stem),
        "minimal_french" | "minimalfrench" => TermStem::F(romance_stem::french_minimal_stem),
        "galician" => TermStem::F(rslp::galician_stem),
        "minimal_galician" => TermStem::F(rslp::galician_minimal_stem),
        "german" | "german2" => TermStem::S("German"),
        "light_german" | "lightgerman" => TermStem::F(germanic_stem::german_light_stem),
        "minimal_german" | "minimalgerman" => TermStem::F(germanic_stem::german_minimal_stem),
        "greek" => TermStem::F(greek::greek_stem),
        "hindi" => TermStem::F(indic::hindi_stem),
        "hungarian" => TermStem::S("Hungarian"),
        "light_hungarian" | "lighthungarian" => TermStem::F(other_light_stem::hungarian_light_stem),
        "indonesian" => TermStem::F(slavic_baltic::indonesian_stem),
        "irish" => TermStem::S("Irish"),
        "italian" => TermStem::S("Italian"),
        "light_italian" | "lightitalian" => TermStem::F(romance_stem::italian_light_stem),
        "latvian" => TermStem::F(slavic_baltic::latvian_stem),
        "lithuanian" => TermStem::S("Lithuanian"),
        "norwegian" => TermStem::S("Norwegian"),
        "light_norwegian" | "lightnorwegian" => {
            TermStem::F(|t| germanic_stem::norwegian_light_stem(t, germanic_stem::BOKMAAL))
        }
        "minimal_norwegian" | "minimalnorwegian" => {
            TermStem::F(|t| germanic_stem::norwegian_minimal_stem(t, germanic_stem::BOKMAAL))
        }
        "light_nynorsk" | "lightnynorsk" => {
            TermStem::F(|t| germanic_stem::norwegian_light_stem(t, germanic_stem::NYNORSK))
        }
        "minimal_nynorsk" | "minimalnynorsk" => {
            TermStem::F(|t| germanic_stem::norwegian_minimal_stem(t, germanic_stem::NYNORSK))
        }
        "persian" => TermStem::F(arabic_family::persian_stem),
        "portuguese" => TermStem::S("Portuguese"),
        "light_portuguese" | "lightportuguese" => TermStem::F(romance_stem::portuguese_light_stem),
        "minimal_portuguese" | "minimalportuguese" => TermStem::F(romance_stem::portuguese_minimal_stem),
        "portuguese_rslp" => TermStem::F(rslp::portuguese_rslp_stem),
        "romanian" => TermStem::S("Romanian"),
        "russian" => TermStem::S("Russian"),
        "light_russian" | "lightrussian" => TermStem::F(other_light_stem::russian_light_stem),
        "spanish" => TermStem::S("Spanish"),
        "light_spanish" | "lightspanish" => TermStem::F(romance_stem::spanish_light_stem),
        "sorani" => TermStem::F(arabic_family::sorani_stem),
        "swedish" => TermStem::S("Swedish"),
        "light_swedish" | "lightswedish" => TermStem::F(germanic_stem::swedish_light_stem),
        "turkish" => TermStem::S("Turkish"),
        _ => return None,
    })
}


/// StemmerTokenFilterFactory.create for a `language` setting.
pub fn stemmer_filter(language: &str) -> Result<Filter, StemmerError> {
    match term_stemmer(language) {
        Some(TermStem::P) => Ok(map_all(english_possessive)),
        Some(TermStem::F(f)) => Ok(map_stem(f)),
        Some(TermStem::S(c)) => Ok(snowball(c)),
        None => snowball_by_class(&capitalize(language)),
    }
}

/// SnowballFilter(stream, name): the stemmer class looked up by name.
pub fn snowball_by_class(name: &str) -> Result<Filter, StemmerError> {
    const CLASSES: &[&str] = &[
        "Arabic", "Armenian", "Basque", "Catalan", "Danish", "Dutch", "English", "Estonian", "Finnish",
        "French", "German", "Greek", "Hindi", "Hungarian", "Indonesian", "Irish", "Italian",
        "Lithuanian", "Nepali", "Norwegian", "Porter", "Portuguese", "Romanian", "Russian", "Serbian",
        "Spanish", "Swedish", "Tamil", "Turkish", "Yiddish",
    ];
    match CLASSES.iter().find(|c| **c == name) {
        Some(c) => Ok(snowball(c)),
        None => Err(StemmerError { class: name.to_string() }),
    }
}

/// A prebuilt (by-name) token filter of the language area, if it is one.
pub fn prebuilt_filter(name: &str) -> Option<Filter> {
    use super::*;
    Some(match name {
        "lowercase" => map_all(lowercase),
        "uppercase" => map_all(uppercase),
        "decimal_digit" => map_all(decimal_digit),
        "arabic_normalization" => map_all(arabic_family::arabic_normalize),
        "persian_normalization" => map_all(arabic_family::persian_normalize),
        "sorani_normalization" => map_all(arabic_family::sorani_normalize),
        "hindi_normalization" => map_stem(indic::hindi_normalize),
        "bengali_normalization" => map_stem(indic::bengali_normalize),
        "indic_normalization" => map_all(indic::indic_normalize),
        "serbian_normalization" => map_all(slavic_baltic::serbian_normalize),
        "german_normalization" => map_all(germanic_stem::german_normalization),
        "scandinavian_normalization" => map_all(germanic_stem::scandinavian_normalization),
        "scandinavian_folding" => map_all(germanic_stem::scandinavian_folding),
        "cjk_width" => map_all(cjk_width),
        "cjk_bigram" => cjk_bigram(CJK_HAN | CJK_HIRAGANA | CJK_KATAKANA | CJK_HANGUL, false),
        "elision" => elision(default_set("articles_french")),
        "apostrophe" => map_all(apostrophe),
        "stop" => stop(default_set("english_stop_words_set"), true),
        "porter_stem" => map_stem(porter::porter_stem),
        "kstem" => map_stem(kstem::kstem),
        "stemmer" => map_stem(porter::porter_stem),
        "snowball" => snowball("English"),
        "arabic_stem" => map_stem(arabic_family::arabic_stem),
        "brazilian_stem" => map_stem(brazilian::brazilian_stem),
        "czech_stem" => map_stem(slavic_baltic::czech_stem),
        "dutch_stem" => snowball("Dutch"),
        "french_stem" => snowball("French"),
        "german_stem" => map_stem(germanic_stem::german_stem),
        "persian_stem" => map_stem(arabic_family::persian_stem),
        "russian_stem" => snowball("Russian"),
        _ => return None,
    })
}

/// A configured token filter (`{"type": ...}`) of the language area.
/// Returns None when the type is not handled here, Some(Err) for a
/// stemmer configuration error.
pub fn configured_filter(p: &Map<String, Value>) -> Option<Result<Filter, StemmerError>> {
    let typ = p.get("type").and_then(|v| v.as_str())?;
    Some(Ok(match typ {
        "lowercase" => match p.get("language").and_then(|v| v.as_str()) {
            None => map_all(lowercase),
            Some(l) => match l.to_lowercase().as_str() {
                "greek" => map_all(super::greek::greek_lowercase),
                "irish" => map_all(irish_lowercase),
                "turkish" => map_all(turkish_lowercase),
                _ => return None,
            },
        },
        "stop" => {
            let ic = setting_bool(p, "ignore_case", false);
            let set = parse_stopwords(p, "stopwords", default_set("english_stop_words_set"), ic);
            let mut set = set;
            if ic && !set.ignore_case {
                set = WordSet::new(set.words.iter().map(|w| String::from_utf16_lossy(w)), true);
            }
            stop(set, setting_bool(p, "remove_trailing", true))
        }
        "elision" => {
            let ic = setting_bool(p, "articles_case", false);
            let set = match p.get("articles") {
                Some(v) => WordSet::new(settings_list(v), ic),
                None => return None,
            };
            elision(set)
        }
        "stemmer" => {
            let lang = p
                .get("language")
                .and_then(scalar)
                .or_else(|| p.get("name").and_then(scalar))
                .unwrap_or_else(|| "porter".to_string());
            return Some(stemmer_filter(&lang));
        }
        "snowball" => {
            let lang = p.get("language").and_then(scalar).unwrap_or_else(|| "English".to_string());
            return Some(snowball_by_class(&lang));
        }
        "keyword_marker" => {
            let ic = setting_bool(p, "ignore_case", false);
            match p.get("keywords") {
                Some(v) => keyword_marker(WordSet::new(settings_list(v), ic)),
                None => return None,
            }
        }
        "cjk_bigram" => {
            let mut flags = CJK_HAN | CJK_HIRAGANA | CJK_KATAKANA | CJK_HANGUL;
            if let Some(v) = p.get("ignored_scripts") {
                for s in settings_list(v) {
                    flags &= !match s.as_str() {
                        "han" => CJK_HAN,
                        "hiragana" => CJK_HIRAGANA,
                        "katakana" => CJK_KATAKANA,
                        "hangul" => CJK_HANGUL,
                        _ => 0,
                    };
                }
            }
            cjk_bigram(flags, setting_bool(p, "output_unigrams", false))
        }
        other => return prebuilt_filter(other).map(Ok),
    }))
}

// ------------------------------------------------------------ analyzers

/// The built-in analyzer names handled here.
pub const LANGUAGE_ANALYZERS: &[&str] = &[
    "arabic", "armenian", "basque", "bengali", "brazilian", "bulgarian", "catalan", "chinese", "cjk",
    "czech", "danish", "dutch", "english", "estonian", "finnish", "french", "galician", "german",
    "greek", "hindi", "hungarian", "indonesian", "irish", "italian", "latvian", "lithuanian",
    "norwegian", "persian", "portuguese", "romanian", "russian", "sorani", "spanish", "swedish",
    "turkish", "thai",
];

/// Runs a language analyzer (with optional provider settings: stopwords,
/// stopwords_case, stem_exclusion) over a text.
pub fn language_analyzer(name: &str, p: &Map<String, Value>, text: &str) -> Option<Vec<Tok>> {
    use super::*;
    if !LANGUAGE_ANALYZERS.contains(&name) {
        return None;
    }
    let ic = setting_bool(p, "stopwords_case", false);
    let stops = |default: &str| parse_stopwords(p, "stopwords", default_set(default), ic);
    let excl = parse_stem_exclusion(p);
    let mut chain: Vec<Filter> = Vec::new();
    let push_kw = |chain: &mut Vec<Filter>| {
        if !excl.is_empty() {
            chain.push(keyword_marker(excl.clone()));
        }
    };
    let snow_simple = |lang: &'static str| -> Option<&'static str> { Some(lang) };
    let mut source = std_tok::tokenize(text, 255);
    match name {
        "armenian" | "basque" | "danish" | "estonian" | "finnish" | "hungarian" | "lithuanian"
        | "norwegian" | "russian" | "swedish" => {
            let class = snow_simple(match name {
                "armenian" => "Armenian",
                "basque" => "Basque",
                "danish" => "Danish",
                "estonian" => "Estonian",
                "finnish" => "Finnish",
                "hungarian" => "Hungarian",
                "lithuanian" => "Lithuanian",
                "norwegian" => "Norwegian",
                "russian" => "Russian",
                _ => "Swedish",
            })?;
            chain.push(map_all(lowercase));
            chain.push(stop(stops(name), true));
            push_kw(&mut chain);
            chain.push(snowball(class));
        }
        "dutch" => {
            chain.push(map_all(lowercase));
            chain.push(stop(stops(name), true));
            push_kw(&mut chain);
            chain.push(stemmer_override(dutch_stem_override()));
            chain.push(snowball("Dutch"));
        }
        "romanian" => {
            chain.push(map_all(lowercase));
            chain.push(stop(stops(name), true));
            chain.push(map_all(romanian_normalize));
            push_kw(&mut chain);
            chain.push(snowball("Romanian"));
        }
        "turkish" => {
            chain.push(map_all(apostrophe));
            chain.push(map_all(turkish_lowercase));
            chain.push(stop(stops(name), true));
            push_kw(&mut chain);
            chain.push(snowball("Turkish"));
        }
        "catalan" | "french" | "italian" => {
            chain.push(elision(default_set(&format!("articles_{}", name))));
            chain.push(map_all(lowercase));
            chain.push(stop(stops(name), true));
            push_kw(&mut chain);
            chain.push(match name {
                "catalan" => snowball("Catalan"),
                "french" => map_stem(romance_stem::french_light_stem),
                _ => map_stem(romance_stem::italian_light_stem),
            });
        }
        "english" => {
            chain.push(map_all(english_possessive));
            chain.push(map_all(lowercase));
            chain.push(stop(stops(name), true));
            push_kw(&mut chain);
            chain.push(map_stem(porter::porter_stem));
        }
        "spanish" | "portuguese" | "galician" | "latvian" | "czech" | "bulgarian" | "indonesian"
        | "brazilian" => {
            chain.push(map_all(lowercase));
            chain.push(stop(stops(name), true));
            push_kw(&mut chain);
            chain.push(map_stem(match name {
                "spanish" => romance_stem::spanish_light_stem,
                "portuguese" => romance_stem::portuguese_light_stem,
                "galician" => rslp::galician_stem,
                "latvian" => slavic_baltic::latvian_stem,
                "czech" => slavic_baltic::czech_stem,
                "bulgarian" => slavic_baltic::bulgarian_stem,
                "indonesian" => slavic_baltic::indonesian_stem,
                _ => brazilian::brazilian_stem,
            }));
        }
        "german" => {
            chain.push(map_all(lowercase));
            chain.push(stop(stops(name), true));
            chain.push(keyword_marker(excl.clone()));
            chain.push(map_all(germanic_stem::german_normalization));
            chain.push(map_stem(germanic_stem::german_light_stem));
        }
        "greek" => {
            chain.push(map_all(greek::greek_lowercase));
            chain.push(stop(stops(name), true));
            chain.push(map_stem(greek::greek_stem));
        }
        "arabic" => {
            chain.push(map_all(lowercase));
            chain.push(map_all(decimal_digit));
            chain.push(stop(stops(name), true));
            chain.push(map_all(arabic_family::arabic_normalize));
            push_kw(&mut chain);
            chain.push(map_stem(arabic_family::arabic_stem));
        }
        "persian" => {
            chain.push(map_all(lowercase));
            chain.push(map_all(decimal_digit));
            chain.push(map_all(arabic_family::arabic_normalize));
            chain.push(map_all(arabic_family::persian_normalize));
            chain.push(stop(stops(name), true));
            chain.push(map_stem(arabic_family::persian_stem));
        }
        "sorani" => {
            chain.push(map_all(arabic_family::sorani_normalize));
            chain.push(map_all(lowercase));
            chain.push(map_all(decimal_digit));
            chain.push(stop(stops(name), true));
            push_kw(&mut chain);
            chain.push(map_stem(arabic_family::sorani_stem));
        }
        "hindi" | "bengali" => {
            chain.push(map_all(lowercase));
            chain.push(map_all(decimal_digit));
            push_kw(&mut chain);
            chain.push(map_all(indic::indic_normalize));
            if name == "hindi" {
                chain.push(map_stem(indic::hindi_normalize));
                chain.push(stop(stops(name), true));
                chain.push(map_stem(indic::hindi_stem));
            } else {
                chain.push(map_stem(indic::bengali_normalize));
                chain.push(stop(stops(name), true));
                chain.push(map_stem(indic::bengali_stem));
            }
        }
        "irish" => {
            chain.push(stop(default_set("hyphenations_irish"), true));
            chain.push(elision(default_set("articles_irish")));
            chain.push(map_all(irish_lowercase));
            chain.push(stop(stops(name), true));
            push_kw(&mut chain);
            chain.push(snowball("Irish"));
        }
        "thai" => {
            source = thai::thai_tokenize(text);
            chain.push(map_all(lowercase));
            chain.push(map_all(decimal_digit));
            chain.push(stop(stops(name), true));
        }
        "cjk" => {
            chain.push(map_all(cjk_width));
            chain.push(map_all(lowercase));
            chain.push(cjk_bigram(CJK_HAN | CJK_HIRAGANA | CJK_KATAKANA | CJK_HANGUL, false));
            chain.push(stop(stops(name), true));
        }
        "chinese" => {
            chain.push(map_all(lowercase));
            chain.push(stop(default_set("english_stop_words_set"), true));
        }
        _ => return None,
    }
    let mut toks = source;
    for f in &chain {
        toks = f(toks);
    }
    Some(toks)
}

/// The Snowball stemmer class name as a static string, if Lucene ships it.
pub fn snowball_class(name: &str) -> Option<&'static str> {
    const CLASSES: &[&str] = &[
        "Arabic", "Armenian", "Basque", "Catalan", "Danish", "Dutch", "English", "Estonian", "Finnish",
        "French", "German", "Greek", "Hindi", "Hungarian", "Indonesian", "Irish", "Italian",
        "Lithuanian", "Nepali", "Norwegian", "Porter", "Portuguese", "Romanian", "Russian", "Serbian",
        "Spanish", "Swedish", "Tamil", "Turkish", "Yiddish",
    ];
    CLASSES.iter().find(|c| **c == name).copied()
}
