//! Text analysis: char filters, tokenizers and token filters modelled on the
//! Lucene components OpenSearch uses, plus the `_analyze` API.
//!
//! A token stream is a list of tokens with absolute positions (0-based,
//! relative to the analyzed value) and UTF-16 offsets, which is how Lucene
//! reports them. `Stream::end_pos` is the position after the last one
//! (including trailing holes left by removed tokens), used to continue
//! positions across the values of a multi-valued field.
//!
//! Entry points:
//! - [`api::handle_analyze`] for `GET|POST [/{index}]/_analyze`,
//! - [`settings::validate_index_analysis`] when analysis settings are created or updated,
//! - [`index_terms`] / [`search_terms`] for indexing and query-time analysis.
//!
//! Built-in analyzers are created in [`builtin_analyzer`]; language analyzers
//! (english, french, ...) hook in there.

pub mod api;
pub mod charfilters;
pub mod filters;
mod fold_table;
pub mod jregex;
pub mod script;
pub mod search;
pub mod settings;
pub mod stem;
pub mod stopwords;
pub mod tokenizers;
pub mod uchar;

use serde_json::{Map, Value};

use crate::errors::ApiError;
use crate::store::Index;

use charfilters::{CharFilter, OffsetMap};
use filters::Filter;
use tokenizers::Tokenizer;

pub type AResult<T> = Result<T, ApiError>;

/// OpenSearch's language analyzers (resolved by [`builtin_analyzer`] once
/// implemented; mappings may always refer to them).
pub const LANGUAGE_ANALYZERS: &[&str] = &[
    "arabic", "armenian", "basque", "bengali", "brazilian", "bulgarian", "catalan", "cjk", "czech",
    "danish", "dutch", "english", "estonian", "finnish", "french", "galician", "german", "greek",
    "hindi", "hungarian", "indonesian", "irish", "italian", "latvian", "lithuanian", "norwegian",
    "persian", "portuguese", "romanian", "russian", "sorani", "spanish", "swedish", "turkish", "thai",
];

pub fn bad(reason: &str) -> ApiError {
    ApiError::bad(reason)
}

#[derive(Debug, Clone)]
pub struct Token {
    pub term: String,
    pub start: usize,
    pub end: usize,
    pub typ: String,
    pub pos: usize,
    pub pos_len: usize,
    pub keyword: bool,
}

impl Token {
    pub fn new(term: String, start: usize, end: usize, typ: &str, pos: usize) -> Token {
        Token { term, start, end, typ: typ.to_string(), pos, pos_len: 1, keyword: false }
    }
}

#[derive(Debug, Clone, Default)]
pub struct Stream {
    pub tokens: Vec<Token>,
    pub end_pos: usize,
}

impl Stream {
    /// Recomputes `end_pos` so it is at least one past the last token.
    pub fn fix_end(&mut self) {
        if let Some(last) = self.tokens.iter().map(|t| t.pos + 1).max() {
            if last > self.end_pos {
                self.end_pos = last;
            }
        }
    }
}

#[derive(Debug, Clone)]
pub struct Named<T> {
    pub name: String,
    pub inner: T,
}

/// A component reference from a request or from analysis settings: a name,
/// or an anonymous inline definition.
#[derive(Debug, Clone)]
pub enum Spec {
    Name(String),
    Inline(Map<String, Value>),
}

// ----------------------------------------------------------- param helpers

/// Settings values arrive either typed (from a request body) or as strings
/// (from flattened index settings, where arrays may be stored as JSON text).
pub fn p_str(p: &Map<String, Value>, key: &str) -> Option<String> {
    match p.get(key)? {
        Value::String(s) => Some(s.clone()),
        Value::Number(n) => Some(n.to_string()),
        Value::Bool(b) => Some(b.to_string()),
        Value::Array(a) if a.len() == 1 => a[0].as_str().map(|s| s.to_string()),
        _ => None,
    }
}

pub fn p_bool(p: &Map<String, Value>, key: &str, def: bool) -> bool {
    match p.get(key) {
        Some(Value::Bool(b)) => *b,
        Some(Value::String(s)) => match s.as_str() {
            "true" => true,
            "false" => false,
            _ => def,
        },
        _ => def,
    }
}

pub fn p_int(p: &Map<String, Value>, key: &str, def: i64) -> i64 {
    match p.get(key) {
        Some(Value::Number(n)) => n.as_i64().or_else(|| n.as_f64().map(|f| f as i64)).unwrap_or(def),
        Some(Value::String(s)) => s.trim().parse::<i64>().unwrap_or(def),
        _ => def,
    }
}

/// A list setting: a JSON array, a JSON array stored as text, or a single string.
pub fn p_list(p: &Map<String, Value>, key: &str) -> Option<Vec<String>> {
    value_list(p.get(key)?)
}

pub fn value_list(v: &Value) -> Option<Vec<String>> {
    match v {
        Value::Array(a) => Some(
            a.iter()
                .map(|x| match x {
                    Value::String(s) => s.clone(),
                    other => crate::store::value_to_string(other).unwrap_or_default(),
                })
                .collect(),
        ),
        Value::String(s) => {
            let t = s.trim();
            if t.starts_with('[') && t.ends_with(']') {
                if let Ok(Value::Array(a)) = serde_json::from_str::<Value>(t) {
                    return value_list(&Value::Array(a));
                }
            }
            Some(vec![s.clone()])
        }
        Value::Number(_) | Value::Bool(_) => Some(vec![crate::store::value_to_string(v).unwrap_or_default()]),
        _ => None,
    }
}

fn set_nested(root: &mut Map<String, Value>, path: &[&str], v: Value) {
    if path.len() == 1 {
        root.insert(path[0].to_string(), v);
        return;
    }
    let e = root.entry(path[0].to_string()).or_insert_with(|| Value::Object(Map::new()));
    if !e.is_object() {
        *e = Value::Object(Map::new());
    }
    set_nested(e.as_object_mut().unwrap(), &path[1..], v);
}

/// Rebuilds the nested `analysis` object (`{"analyzer": {...}, "filter": {...}}`)
/// from flattened index settings.
pub fn analysis_from_settings(settings: &Map<String, Value>) -> Map<String, Value> {
    let mut root = Map::new();
    for (k, v) in settings {
        let rest = match k.strip_prefix("index.analysis.").or_else(|| k.strip_prefix("analysis.")) {
            Some(r) => r,
            None => {
                if k == "index.analysis" || k == "analysis" {
                    if let Value::Object(m) = v {
                        for (kk, vv) in m {
                            root.insert(kk.clone(), vv.clone());
                        }
                    }
                }
                continue;
            }
        };
        let parts: Vec<&str> = rest.split('.').collect();
        if parts.len() < 2 {
            continue;
        }
        let val = match v {
            Value::String(s) if s.trim_start().starts_with('[') => {
                serde_json::from_str::<Value>(s).ok().filter(|x| x.is_array()).unwrap_or_else(|| v.clone())
            }
            _ => v.clone(),
        };
        set_nested(&mut root, &parts, val);
    }
    root
}

// ------------------------------------------------------------------ context

/// Where components are resolved: an index's analysis settings, or the
/// global registry only.
pub struct Ctx {
    pub analysis: Map<String, Value>,
    pub in_index: bool,
    pub max_ngram_diff: i64,
    pub max_shingle_diff: i64,
}

impl Ctx {
    pub fn global() -> Ctx {
        Ctx { analysis: Map::new(), in_index: false, max_ngram_diff: 1, max_shingle_diff: 3 }
    }

    pub fn for_settings(settings: &Map<String, Value>) -> Ctx {
        let get = |k: &str, d: i64| -> i64 {
            crate::store::find_setting(settings, k)
                .and_then(|v| match v {
                    Value::Number(n) => n.as_i64(),
                    Value::String(s) => s.parse().ok(),
                    _ => None,
                })
                .unwrap_or(d)
        };
        Ctx {
            analysis: analysis_from_settings(settings),
            in_index: true,
            max_ngram_diff: get("max_ngram_diff", 1),
            max_shingle_diff: get("max_shingle_diff", 3),
        }
    }

    pub fn for_index(idx: &Index) -> Ctx {
        Ctx::for_settings(&idx.settings)
    }

    pub fn section(&self, kind: &str, name: &str) -> Option<&Map<String, Value>> {
        self.analysis.get(kind)?.as_object()?.get(name)?.as_object()
    }

    fn not_found(&self, kind: &str, name: &str) -> ApiError {
        if self.in_index {
            bad(&format!("failed to find {} under [{}]", kind, name))
        } else {
            bad(&format!("failed to find global {} under [{}]", kind, name))
        }
    }

    pub fn char_filter(&self, spec: &Spec) -> AResult<Named<CharFilter>> {
        match spec {
            Spec::Name(n) => {
                if let Some(def) = self.section("char_filter", n) {
                    let typ = p_str(def, "type").unwrap_or_default();
                    let cf = charfilters::build(&typ, def)?.ok_or_else(|| self.not_found("char_filter", &typ))?;
                    return Ok(Named { name: n.clone(), inner: cf });
                }
                let cf = charfilters::build(n, &Map::new())?.ok_or_else(|| self.not_found("char_filter", n))?;
                Ok(Named { name: n.clone(), inner: cf })
            }
            Spec::Inline(def) => {
                let typ = p_str(def, "type").unwrap_or_default();
                let cf = charfilters::build(&typ, def)?.ok_or_else(|| self.not_found("char_filter", &typ))?;
                Ok(Named { name: format!("__anonymous__{}", typ), inner: cf })
            }
        }
    }

    pub fn tokenizer(&self, spec: &Spec) -> AResult<Named<Tokenizer>> {
        match spec {
            Spec::Name(n) => {
                if let Some(def) = self.section("tokenizer", n) {
                    let typ = p_str(def, "type").unwrap_or_default();
                    let t = tokenizers::build(&typ, def, self)?.ok_or_else(|| self.not_found("tokenizer", &typ))?;
                    return Ok(Named { name: n.clone(), inner: t });
                }
                let t = tokenizers::build(n, &Map::new(), self)?.ok_or_else(|| self.not_found("tokenizer", n))?;
                Ok(Named { name: n.clone(), inner: t })
            }
            Spec::Inline(def) => {
                let typ = p_str(def, "type").unwrap_or_default();
                let t = tokenizers::build(&typ, def, self)?.ok_or_else(|| self.not_found("tokenizer", &typ))?;
                Ok(Named { name: format!("__anonymous__{}", typ), inner: t })
            }
        }
    }

    /// Builds a token filter. `chain` is the analysis chain before this filter
    /// (synonym rules are analyzed with it).
    pub fn filter(&self, spec: &Spec, chain: &Chain) -> AResult<Named<Filter>> {
        match spec {
            Spec::Name(n) => {
                if let Some(def) = self.section("filter", n) {
                    let typ = p_str(def, "type").unwrap_or_default();
                    let f = filters::build(&typ, def, self, chain)?.ok_or_else(|| self.not_found("filter", &typ))?;
                    return Ok(Named { name: n.clone(), inner: f });
                }
                let f = filters::build(n, &Map::new(), self, chain)?.ok_or_else(|| self.not_found("filter", n))?;
                Ok(Named { name: n.clone(), inner: f })
            }
            Spec::Inline(def) => {
                let typ = p_str(def, "type").unwrap_or_default();
                let f = filters::build(&typ, def, self, chain)?.ok_or_else(|| self.not_found("filter", &typ))?;
                Ok(Named { name: format!("__anonymous__{}", typ), inner: f })
            }
        }
    }

    /// Resolves a named analyzer: index analyzers first, then built-ins.
    pub fn analyzer(&self, name: &str) -> AResult<Option<Analyzer>> {
        if let Some(def) = self.section("analyzer", name) {
            return analyzer_from_def(name, def, self).map(Some);
        }
        builtin_analyzer(name, &Map::new(), self)
    }

    /// Resolves a normalizer by name (index normalizers, or `lowercase`).
    pub fn normalizer(&self, name: &str) -> AResult<Option<Analyzer>> {
        if let Some(def) = self.section("normalizer", name) {
            let mut chain = Chain::new(Named { name: "keyword".to_string(), inner: Tokenizer::Keyword });
            for s in specs_of(def.get("char_filter")) {
                chain.char_filters.push(self.char_filter(&s)?);
            }
            for s in specs_of(def.get("filter")) {
                let f = self.filter(&s, &chain)?;
                chain.filters.push(f);
            }
            return Ok(Some(Analyzer { name: name.to_string(), custom: true, chain, gap: 0 }));
        }
        if name == "lowercase" {
            let mut chain = Chain::new(Named { name: "keyword".to_string(), inner: Tokenizer::Keyword });
            chain.filters.push(Named { name: "lowercase".to_string(), inner: Filter::Lowercase(filters::LowerLang::Plain) });
            return Ok(Some(Analyzer { name: name.to_string(), custom: true, chain, gap: 0 }));
        }
        Ok(None)
    }
}

/// Component references in settings or a request: a string, a list of
/// strings, or inline objects.
pub fn specs_of(v: Option<&Value>) -> Vec<Spec> {
    let v = match v {
        Some(v) => v,
        None => return Vec::new(),
    };
    match v {
        Value::Array(a) => a
            .iter()
            .filter_map(|x| match x {
                Value::String(s) => Some(Spec::Name(s.clone())),
                Value::Object(m) => Some(Spec::Inline(m.clone())),
                _ => None,
            })
            .collect(),
        Value::String(_) => value_list(v).unwrap_or_default().into_iter().map(Spec::Name).collect(),
        Value::Object(m) => vec![Spec::Inline(m.clone())],
        _ => Vec::new(),
    }
}

// ------------------------------------------------------------------ chains

#[derive(Debug, Clone)]
pub struct Chain {
    pub char_filters: Vec<Named<CharFilter>>,
    pub tokenizer: Named<Tokenizer>,
    pub filters: Vec<Named<Filter>>,
}

pub struct Detail {
    pub filtered_texts: Vec<String>,
    pub tokenizer: Stream,
    pub stages: Vec<Stream>,
}

impl Chain {
    pub fn new(tokenizer: Named<Tokenizer>) -> Chain {
        Chain { char_filters: Vec::new(), tokenizer, filters: Vec::new() }
    }

    fn tokenize(&self, text: &str) -> (Stream, Vec<String>) {
        let mut maps: Vec<OffsetMap> = Vec::new();
        let mut cur = text.to_string();
        let mut texts = Vec::new();
        for cf in &self.char_filters {
            let (out, map) = cf.inner.filter(&cur);
            texts.push(out.clone());
            cur = out;
            maps.push(map);
        }
        let mut s = self.tokenizer.inner.tokenize(&cur);
        if !maps.is_empty() {
            for t in s.tokens.iter_mut() {
                t.start = charfilters::correct(&maps, t.start);
                t.end = charfilters::correct(&maps, t.end);
            }
        }
        (s, texts)
    }

    pub fn run(&self, text: &str) -> Stream {
        let (mut s, _) = self.tokenize(text);
        for f in &self.filters {
            s = f.inner.apply(s);
        }
        s
    }

    /// Runs the chain keeping every intermediate result (for `explain`).
    pub fn run_detailed(&self, text: &str) -> Detail {
        let (s, texts) = self.tokenize(text);
        let mut stages = Vec::new();
        let mut cur = s.clone();
        for f in &self.filters {
            cur = f.inner.apply(cur);
            stages.push(cur.clone());
        }
        Detail { filtered_texts: texts, tokenizer: s, stages }
    }

    /// Whether the stream after `upto` filters carries a KeywordAttribute
    /// (it shows up in `explain` output).
    pub fn has_keyword_attr(&self, upto: usize) -> bool {
        self.tokenizer.inner.uses_keyword_attr() || self.filters.iter().take(upto).any(|f| f.inner.uses_keyword_attr())
    }
}

#[derive(Debug, Clone)]
pub struct Analyzer {
    pub name: String,
    /// CustomAnalyzer (explain shows each component) vs a prebuilt one.
    pub custom: bool,
    pub chain: Chain,
    pub gap: usize,
}

impl Analyzer {
    pub fn run(&self, text: &str) -> Stream {
        self.chain.run(text)
    }

    pub fn terms(&self, text: &str) -> Vec<String> {
        self.run(text).tokens.into_iter().map(|t| t.term).collect()
    }
}

fn simple_chain(tok: Tokenizer, tok_name: &str, filters: Vec<(String, Filter)>) -> Chain {
    let mut c = Chain::new(Named { name: tok_name.to_string(), inner: tok });
    for (n, f) in filters {
        c.filters.push(Named { name: n, inner: f });
    }
    c
}

/// Stop words from an analyzer or filter definition (`stopwords` list or
/// `_lang_` name); `def` when unset.
pub fn stopwords_param(p: &Map<String, Value>, def: &[&str]) -> AResult<Vec<String>> {
    if p.contains_key("stopwords_path") && !p.contains_key("stopwords") {
        return Err(bad("IOException while reading stopwords_path: file not readable"));
    }
    match p_list(p, "stopwords") {
        None => Ok(def.iter().map(|s| s.to_string()).collect()),
        Some(list) => {
            let mut out = Vec::new();
            for w in list {
                if w.starts_with('_') && w.ends_with('_') && w.len() > 2 {
                    match stopwords::named(&w) {
                        Some(ws) => out.extend(ws),
                        None => out.push(w),
                    }
                } else {
                    out.push(w);
                }
            }
            Ok(out)
        }
    }
}

fn stop_filter(words: Vec<String>, ignore_case: bool) -> Filter {
    Filter::Stop(filters::StopSet::new(words, ignore_case), true)
}

/// The prebuilt analyzers, and analyzer types usable in index settings
/// (`{"type": "standard", "stopwords": [...]}`). Language analyzers plug in
/// here as well.
pub fn builtin_analyzer(typ: &str, p: &Map<String, Value>, ctx: &Ctx) -> AResult<Option<Analyzer>> {
    let max_len = p_int(p, "max_token_length", 255).max(1) as usize;
    let lower = || ("lowercase".to_string(), Filter::Lowercase(filters::LowerLang::Plain));
    let chain = match typ {
        "standard" | "default" => {
            let stop = stopwords_param(p, &[])?;
            let mut f = vec![lower()];
            if !stop.is_empty() {
                f.push(("stop".to_string(), stop_filter(stop, false)));
            }
            simple_chain(Tokenizer::Standard { max_len }, "standard", f)
        }
        "simple" => simple_chain(Tokenizer::Lowercase { max_len: 255 }, "lowercase", vec![]),
        "whitespace" => simple_chain(Tokenizer::Whitespace { max_len }, "whitespace", vec![]),
        "keyword" => simple_chain(Tokenizer::Keyword, "keyword", vec![]),
        "stop" => {
            let stop = stopwords_param(p, stopwords::ENGLISH)?;
            simple_chain(Tokenizer::Lowercase { max_len: 255 }, "lowercase", vec![("stop".to_string(), stop_filter(stop, false))])
        }
        "pattern" => {
            let pat = p_str(p, "pattern").unwrap_or_else(|| "\\W+".to_string());
            let flags = jregex::parse_flags(&p_str(p, "flags").unwrap_or_default()).map_err(|e| bad(&e))?;
            let re = tokenizers::compile_regex(&pat, flags)?;
            let mut f = Vec::new();
            if p_bool(p, "lowercase", true) {
                f.push(lower());
            }
            let stop = stopwords_param(p, &[])?;
            if !stop.is_empty() {
                f.push(("stop".to_string(), stop_filter(stop, false)));
            }
            simple_chain(Tokenizer::Pattern { re, group: -1 }, "pattern", f)
        }
        "fingerprint" => {
            let sep = p_str(p, "separator").unwrap_or_else(|| " ".to_string());
            let max = p_int(p, "max_output_size", 255).max(0) as usize;
            let mut f = vec![lower(), ("asciifolding".to_string(), Filter::AsciiFolding(false))];
            let stop = stopwords_param(p, &[])?;
            if !stop.is_empty() {
                f.push(("stop".to_string(), stop_filter(stop, false)));
            }
            f.push(("fingerprint".to_string(), Filter::Fingerprint { sep, max }));
            simple_chain(Tokenizer::Standard { max_len: 255 }, "standard", f)
        }
        "classic" => {
            let stop = stopwords_param(p, stopwords::ENGLISH)?;
            let mut f = vec![("classic".to_string(), Filter::Classic), lower()];
            if !stop.is_empty() {
                f.push(("stop".to_string(), stop_filter(stop, false)));
            }
            simple_chain(Tokenizer::Classic { max_len }, "classic", f)
        }
        "snowball" => {
            let lang = p_str(p, "language").unwrap_or_else(|| "English".to_string());
            let default_stop: &[&str] = if lang.eq_ignore_ascii_case("english") { stopwords::ENGLISH } else { &[] };
            let stop = stopwords_param(p, default_stop)?;
            let mut f = vec![lower()];
            if !stop.is_empty() {
                f.push(("stop".to_string(), stop_filter(stop, false)));
            }
            let stem = filters::stemmer_filter(&lang)?;
            f.push(("snowball".to_string(), stem));
            simple_chain(Tokenizer::Standard { max_len: 255 }, "standard", f)
        }
        // Language analyzers ("english", "french", ...) are resolved here.
        lang if crate::lang::chains::LANGUAGE_ANALYZERS.contains(&lang) => simple_chain(
            Tokenizer::Language { name: lang.to_string(), params: p.clone() },
            if lang == "thai" { "thai" } else { "standard" },
            vec![],
        ),
        _ => return Ok(None),
    };
    let _ = ctx;
    Ok(Some(Analyzer { name: typ.to_string(), custom: false, chain, gap: 0 }))
}

/// Builds an analyzer from its index settings definition.
pub fn analyzer_from_def(name: &str, def: &Map<String, Value>, ctx: &Ctx) -> AResult<Analyzer> {
    let typ = p_str(def, "type").unwrap_or_else(|| if def.contains_key("tokenizer") { "custom".to_string() } else { String::new() });
    let gap = p_int(def, "position_increment_gap", 100).max(0) as usize;
    if typ == "custom" {
        let tok = match def.get("tokenizer") {
            Some(Value::String(t)) => t.clone(),
            _ => return Err(bad(&format!("Custom Analyzer [{}] must be configured with a tokenizer", name))),
        };
        let tokenizer = ctx
            .tokenizer(&Spec::Name(tok.clone()))
            .map_err(|_| bad(&format!("Custom Analyzer [{}] failed to find tokenizer under name [{}]", name, tok)))?;
        let mut chain = Chain::new(tokenizer);
        for s in specs_of(def.get("char_filter")) {
            let n = match &s {
                Spec::Name(n) => n.clone(),
                Spec::Inline(_) => String::new(),
            };
            let cf = ctx
                .char_filter(&s)
                .map_err(|_| bad(&format!("Custom Analyzer [{}] failed to find char_filter under name [{}]", name, n)))?;
            chain.char_filters.push(cf);
        }
        for s in specs_of(def.get("filter")) {
            let n = match &s {
                Spec::Name(n) => n.clone(),
                Spec::Inline(_) => String::new(),
            };
            let f = match ctx.filter(&s, &chain) {
                Ok(f) => f,
                Err(e) => {
                    if e.reason.starts_with("failed to find") {
                        return Err(bad(&format!("Custom Analyzer [{}] failed to find filter under name [{}]", name, n)));
                    }
                    return Err(e);
                }
            };
            chain.filters.push(f);
        }
        return Ok(Analyzer { name: name.to_string(), custom: true, chain, gap });
    }
    match builtin_analyzer(&typ, def, ctx)? {
        Some(mut a) => {
            a.name = name.to_string();
            if def.contains_key("position_increment_gap") {
                a.gap = gap;
            }
            Ok(a)
        }
        None => Err(bad(&format!("Unknown analyzer type [{}] for [{}]", typ, name))),
    }
}

// ------------------------------------------------- indexing / search support

/// Whether an analyzer name resolves in the index (or globally).
pub fn analyzer_exists(idx: &Index, name: &str) -> bool {
    let ctx = Ctx::for_index(idx);
    matches!(ctx.analyzer(name), Ok(Some(_)))
}

/// Applies a keyword field's normalizer to a value.
pub fn normalize_keyword(idx: &Index, normalizer: &str, text: &str) -> Option<String> {
    let ctx = Ctx::for_index(idx);
    let n = ctx.normalizer(normalizer).ok()??;
    let s = n.run(text);
    Some(s.tokens.into_iter().map(|t| t.term).collect::<Vec<_>>().join(""))
}
