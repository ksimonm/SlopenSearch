//! The phrase suggester, ported from OpenSearch's PhraseSuggester: a noisy
//! channel spell checker over candidate sets drawn by direct candidate
//! generators (Lucene's DirectSpellChecker), scored with n-gram language
//! models (stupid backoff, Laplace, linear interpolation).

use crate::analysis::{Analyzer, Ctx};
use crate::errors::ApiError;
use crate::store::*;
use serde_json::{json, Map, Value};
use std::collections::HashMap;

// ------------------------------------------------------------- statistics

#[derive(Default)]
struct FieldStats {
    df: HashMap<String, u64>,
    ttf: HashMap<String, u64>,
    sum_ttf: u64,
    max_doc: u64,
}

impl FieldStats {
    fn num_terms(&self) -> u64 {
        self.df.len() as u64
    }
    fn term_stats(&self, t: &str) -> (u64, u64) {
        (self.df.get(t).copied().unwrap_or(0), self.ttf.get(t).copied().unwrap_or(0))
    }
    fn freq(&self, t: &str) -> u64 {
        self.ttf.get(t).copied().unwrap_or(0)
    }
}

fn index_analyzer_name(ctx: &Ctx, def: &Map<String, Value>) -> String {
    if let Some(a) = def.get("analyzer").and_then(|v| v.as_str()) {
        return a.to_string();
    }
    if ctx.section("analyzer", "default").is_some() {
        return "default".into();
    }
    "standard".into()
}

fn search_analyzer_name(ctx: &Ctx, def: &Map<String, Value>) -> String {
    if let Some(a) = def.get("search_analyzer").and_then(|v| v.as_str()) {
        return a.to_string();
    }
    if def.get("analyzer").is_none() && ctx.section("analyzer", "default_search").is_some() {
        return "default_search".into();
    }
    index_analyzer_name(ctx, def)
}

/// The `_source` path holding a field's values (multi-fields read their parent).
fn source_path(idx: &Index, field: &str) -> String {
    let mut path = field.to_string();
    while let Some((parent, child)) = path.rsplit_once('.') {
        match crate::eval::mapping_field_def(idx, parent) {
            Some(pd) if pd.get("fields").and_then(|f| f.get(child)).is_some() => path = parent.to_string(),
            _ => break,
        }
    }
    path
}

fn has_terms(def: &Map<String, Value>) -> bool {
    matches!(
        def.get("type").and_then(|v| v.as_str()).unwrap_or("object"),
        "text" | "keyword" | "match_only_text" | "search_as_you_type" | "wildcard" | "constant_keyword"
    )
}

fn field_stats(idx: &Index, field: &str, def: &Map<String, Value>) -> FieldStats {
    let mut st = FieldStats::default();
    let ctx = Ctx::for_index(idx);
    let keyword = def.get("type").and_then(|v| v.as_str()) == Some("keyword");
    let an = if keyword { None } else { ctx.analyzer(&index_analyzer_name(&ctx, def)).ok().flatten() };
    let src = source_path(idx, field);
    for id in &idx.order {
        if idx.pending.contains_key(id) {
            continue;
        }
        let Some(doc) = idx.docs.get(id) else { continue };
        st.max_doc += 1;
        let Some(v) = field_value(doc, &src) else { continue };
        let mut seen: std::collections::HashSet<String> = std::collections::HashSet::new();
        for s in doc_strings(v) {
            let toks: Vec<String> = match &an {
                Some(a) => a.terms(&s),
                None => vec![s.clone()],
            };
            for t in toks {
                st.sum_ttf += 1;
                *st.ttf.entry(t.clone()).or_insert(0) += 1;
                seen.insert(t);
            }
        }
        for t in seen {
            *st.df.entry(t).or_insert(0) += 1;
        }
    }
    st
}

// ------------------------------------------------------------- candidates

#[derive(Clone, Debug)]
struct Cand {
    term: String,
    df: u64,
    ttf: u64,
    string_distance: f64,
    score: f64,
    user_input: bool,
}

struct CandSet {
    original: Cand,
    cands: Vec<Cand>,
}

#[derive(Clone, Copy, PartialEq)]
enum Mode {
    Missing,
    Popular,
    Always,
}

struct Generator {
    stats: FieldStats,
    mode: Mode,
    size: usize,
    accuracy: f32,
    max_edits: usize,
    max_term_freq: f32,
    prefix_length: usize,
    min_word_length: usize,
    min_doc_freq: f32,
    by_freq: bool,
    distance: String,
    pre: Option<Analyzer>,
    post: Option<Analyzer>,
    non_error_likelihood: f64,
}

/// Lucene's analyze-then-keep-last-token pre filter.
fn filter_last(an: &Option<Analyzer>, term: &str) -> String {
    match an {
        None => term.to_string(),
        Some(a) => a.terms(term).pop().unwrap_or_else(|| term.to_string()),
    }
}

impl Generator {
    fn cand_score(&self, ttf: u64, error_score: f64) -> f64 {
        error_score * ((ttf as f64 + 1.0) / (self.stats.sum_ttf as f64 + 1.0))
    }

    fn term_stats(&self, term: &str) -> (u64, u64) {
        self.stats.term_stats(&filter_last(&self.pre, term))
    }

    fn create(&self, term: &str, stats: (u64, u64), channel: f64, user_input: bool) -> Cand {
        Cand {
            term: term.to_string(),
            df: stats.0,
            ttf: stats.1,
            string_distance: channel,
            score: self.cand_score(stats.1, channel),
            user_input,
        }
    }

    fn threshold_frequency(&self, freq: u64) -> f32 {
        if freq == 0 {
            return 0.0;
        }
        let plateau = if self.min_doc_freq >= 1.0 {
            self.min_doc_freq as i64
        } else {
            (self.stats.max_doc as f32 * self.min_doc_freq) as i64
        };
        let v = (freq as f64) * ((freq as f64 - plateau as f64).log10() * (1.0 / 5f64.log10())) + 1.0;
        (v.round() as i64).max(0) as f32
    }

    /// DirectSpellChecker.suggestSimilar.
    fn suggest_similar(&self, text: &str, threshold: f32) -> Vec<(String, f32, u64)> {
        let q: Vec<char> = text.chars().collect();
        if self.min_word_length > 0 && q.len() < self.min_word_length {
            return vec![];
        }
        let mut docfreq = self.stats.df.get(text).copied().unwrap_or(0);
        if self.mode == Mode::Missing && docfreq > 0 {
            return vec![];
        }
        let max_doc = self.stats.max_doc;
        if self.max_term_freq >= 1.0 {
            if docfreq as f32 > self.max_term_freq {
                return vec![];
            }
        } else if docfreq as f64 > (self.max_term_freq * max_doc as f32).ceil() as f64 {
            return vec![];
        }
        if self.mode != Mode::Popular {
            docfreq = 0;
        }
        if threshold >= 1.0 {
            docfreq = docfreq.max(threshold as u64);
        } else if threshold > 0.0 {
            docfreq = docfreq.max(((threshold * max_doc as f32) as i64 - 1).max(0) as u64);
        }
        let mut terms: Vec<(&String, &u64)> = self.stats.df.iter().collect();
        terms.sort();
        let mut out: Vec<(String, f32, u64)> = vec![];
        for (cand, &df) in terms {
            if *cand == text || df <= docfreq {
                continue;
            }
            let cc: Vec<char> = cand.chars().collect();
            let ed = crate::suggest::osa(&q, &cc);
            if ed == 0 || ed > self.max_edits {
                continue;
            }
            let need = if ed == 1 { self.prefix_length } else { self.prefix_length.max(ed - 1) };
            if q.len() < need || cc.len() < need || q[..need] != cc[..need] {
                continue;
            }
            let score = match self.distance.as_str() {
                "levenshtein" => 1.0 - crate::suggest::plain_lev(&q, &cc) as f32 / q.len().max(cc.len()) as f32,
                "jaro_winkler" => crate::suggest::jaro_winkler(&q, &cc),
                "ngram" => crate::suggest::ngram_distance(&q, &cc),
                "damerau_levenshtein" => {
                    1.0 - crate::suggest::osa(&q, &cc) as f32 / q.len().max(cc.len()) as f32
                }
                _ => 1.0 - ed as f32 / q.len().min(cc.len()) as f32,
            };
            if score < self.accuracy {
                continue;
            }
            out.push((cand.clone(), score, df));
        }
        out.sort_by(|a, b| {
            let sc = b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal);
            let fr = b.2.cmp(&a.2);
            let p = if self.by_freq { fr.then(sc) } else { sc.then(fr) };
            p.then(a.0.cmp(&b.0))
        });
        out.truncate(self.size);
        out
    }

    fn draw(&self, set: &mut CandSet) {
        let term = filter_last(&self.pre, &set.original.term);
        let threshold = if self.mode == Mode::Always { 0.0 } else { self.threshold_frequency(set.original.df) };
        let mut new: Vec<Cand> = vec![];
        for (word, score, _) in self.suggest_similar(&term, threshold) {
            let stats = self.stats.term_stats(&word);
            let c = Cand {
                term: word.clone(),
                df: stats.0,
                ttf: stats.1,
                string_distance: score as f64,
                score: self.cand_score(stats.1, score as f64),
                user_input: false,
            };
            match &self.post {
                None => new.push(c),
                Some(a) => {
                    for t in a.run(&c.term).tokens {
                        if t.term == c.term {
                            let st = self.stats.term_stats(&t.term);
                            new.push(Cand {
                                term: t.term,
                                df: st.0,
                                ttf: st.1,
                                string_distance: c.string_distance,
                                score: self.cand_score(c.ttf, c.string_distance),
                                user_input: false,
                            });
                        } else {
                            new.push(Cand {
                                term: t.term,
                                df: c.df,
                                ttf: c.ttf,
                                string_distance: self.non_error_likelihood,
                                score: self.cand_score(c.ttf, c.string_distance),
                                user_input: false,
                            });
                        }
                    }
                }
            }
        }
        for c in new {
            if !set.cands.iter().any(|x| x.term == c.term) {
                set.cands.push(c);
            }
        }
    }
}

// ------------------------------------------------------------- scorers

#[derive(Clone)]
enum Model {
    StupidBackoff(f64),
    Laplace(f64),
    Linear(f64, f64, f64),
}

struct WordScorer<'a> {
    stats: &'a FieldStats,
    vocabulary: f64,
    num_terms: f64,
    real_word_likelihood: f64,
    sep: String,
    model: Model,
}

impl<'a> WordScorer<'a> {
    fn freq(&self, t: &str) -> f64 {
        self.stats.freq(t) as f64
    }
    fn join(&self, parts: &[&str]) -> String {
        parts.join(&self.sep)
    }
    fn channel(&self, c: &Cand) -> f64 {
        if c.string_distance == 1.0 {
            self.real_word_likelihood
        } else {
            c.string_distance
        }
    }
    fn unigram(&self, w: &Cand) -> f64 {
        match self.model {
            Model::Laplace(alpha) => (alpha + self.freq(&w.term)) / (self.vocabulary + alpha * self.num_terms),
            _ => (1.0 + self.freq(&w.term)) / (self.vocabulary + self.num_terms),
        }
    }
    fn bigram(&self, w: &Cand, w1: &Cand) -> f64 {
        match self.model {
            Model::StupidBackoff(discount) => {
                let count = self.freq(&self.join(&[&w1.term, &w.term]));
                if count < 1.0 {
                    discount * self.unigram(w)
                } else {
                    count / (w1.ttf as f64 + 0.00000000001)
                }
            }
            Model::Laplace(alpha) => {
                (alpha + self.freq(&self.join(&[&w1.term, &w.term]))) / (w1.ttf as f64 + alpha * self.num_terms)
            }
            Model::Linear(_, bl, ul) => {
                let count = self.freq(&self.join(&[&w1.term, &w.term]));
                if count < 1.0 {
                    ul * self.unigram(w)
                } else {
                    bl * (count / (0.5 + w1.ttf as f64)) + ul * self.unigram(w)
                }
            }
        }
    }
    fn trigram(&self, w: &Cand, w1: &Cand, w2: &Cand) -> f64 {
        match self.model {
            Model::StupidBackoff(discount) => {
                let bigram = self.freq(&self.join(&[&w1.term, &w.term]));
                if bigram < 1.0 {
                    return discount * self.unigram(w);
                }
                let trigram = self.freq(&self.join(&[&w2.term, &w1.term, &w.term]));
                if trigram < 1.0 {
                    return discount * (bigram / (w1.ttf as f64 + 0.00000000001));
                }
                trigram / (bigram + 0.00000000001)
            }
            Model::Laplace(alpha) => {
                let trigram = self.freq(&self.join(&[&w2.term, &w1.term, &w.term]));
                let bigram = self.freq(&self.join(&[&w1.term, &w.term]));
                (alpha + trigram) / (bigram + alpha * self.num_terms)
            }
            Model::Linear(tl, _, _) => {
                let count = self.freq(&self.join(&[&w.term, &w1.term, &w2.term]));
                if count < 1.0 {
                    return self.bigram(w, w1);
                }
                let bigram = self.freq(&self.join(&[&w.term, &w1.term]));
                tl * (count / (1.0 + bigram)) + self.bigram(w, w1)
            }
        }
    }
    fn score(&self, path: &[Cand], sets: &[CandSet], at: usize, gram: usize) -> f64 {
        let ch = self.channel(&path[at]);
        debug_assert!(at < sets.len());
        if at == 0 || gram == 1 {
            (ch * self.unigram(&path[at])).log10()
        } else if at == 1 || gram == 2 {
            (ch * self.bigram(&path[at], &path[at - 1])).log10()
        } else {
            (ch * self.trigram(&path[at], &path[at - 1], &path[at - 2])).log10()
        }
    }
}

// ------------------------------------------------------------ corrections

struct Correction {
    score: f64,
    path: Vec<Cand>,
}

/// Correction.compareTo: score, then earlier terms are stronger.
fn cmp_corr(a_score: f64, a: &[Cand], b_score: f64, b: &[Cand]) -> std::cmp::Ordering {
    if a_score == b_score {
        for (x, y) in a.iter().zip(b.iter()) {
            let c = x.term.as_bytes().cmp(y.term.as_bytes());
            if c != std::cmp::Ordering::Equal {
                return c.reverse();
            }
        }
        return a.len().cmp(&b.len());
    }
    a_score.partial_cmp(&b_score).unwrap_or(std::cmp::Ordering::Equal)
}

struct Search<'a> {
    scorer: &'a WordScorer<'a>,
    gram: usize,
    max: usize,
    cutoff: f64,
    out: Vec<Correction>,
}

impl<'a> Search<'a> {
    fn update_top(&mut self, path: &[Cand], score: f64) {
        let score = score.exp();
        if score <= self.cutoff {
            return;
        }
        if self.out.len() < self.max {
            self.out.push(Correction { score, path: path.to_vec() });
            return;
        }
        // replace the weakest when this one is stronger
        let mut wi = 0;
        for i in 1..self.out.len() {
            if cmp_corr(self.out[i].score, &self.out[i].path, self.out[wi].score, &self.out[wi].path)
                == std::cmp::Ordering::Less
            {
                wi = i;
            }
        }
        if cmp_corr(self.out[wi].score, &self.out[wi].path, score, path) == std::cmp::Ordering::Less {
            self.out[wi] = Correction { score, path: path.to_vec() };
        }
    }

    fn find(&mut self, sets: &[CandSet], path: &mut Vec<Cand>, ord: usize, left: usize, path_score: f64) {
        let cur = &sets[ord];
        if ord == sets.len() - 1 {
            path[ord] = cur.original.clone();
            let s = self.scorer.score(path, sets, ord, self.gram);
            self.update_top(path, path_score + s);
            if left > 0 {
                for c in &cur.cands {
                    path[ord] = c.clone();
                    let s = self.scorer.score(path, sets, ord, self.gram);
                    self.update_top(path, path_score + s);
                }
            }
        } else if left > 0 {
            path[ord] = cur.original.clone();
            let s = self.scorer.score(path, sets, ord, self.gram);
            self.find(sets, path, ord + 1, left, path_score + s);
            for c in &cur.cands {
                path[ord] = c.clone();
                let s = self.scorer.score(path, sets, ord, self.gram);
                self.find(sets, path, ord + 1, left - 1, path_score + s);
            }
        } else {
            path[ord] = cur.original.clone();
            let s = self.scorer.score(path, sets, ord, self.gram);
            self.find(sets, path, ord + 1, 0, path_score + s);
        }
    }
}

// ------------------------------------------------------------- options

fn bad(reason: &str) -> ApiError {
    ApiError::new("illegal_argument_exception", reason, 400)
}

fn shard_err(index: &str, reason: &str) -> ApiError {
    let o = json!({"type": "illegal_argument_exception", "reason": reason});
    crate::script_ctx::shard_failure(index, 400, o.clone(), o.clone(), o, "query")
}

const PHRASE_OPTIONS: &[&str] = &[
    "field", "analyzer", "size", "shard_size", "gram_size", "real_word_error_likelihood", "confidence",
    "max_errors", "separator", "force_unigrams", "token_limit", "direct_generator", "highlight", "smoothing",
    "collate",
];

pub const GENERATOR_OPTIONS: &[&str] = &[
    "field", "size", "suggest_mode", "accuracy", "sort", "string_distance", "max_edits", "max_inspections",
    "max_term_freq", "prefix_length", "prefix_len", "min_word_length", "min_word_len", "min_doc_freq",
    "pre_filter", "post_filter",
];

fn num(m: &Map<String, Value>, k: &str) -> Option<f64> {
    m.get(k).and_then(to_f64)
}

struct GenSpec {
    raw: Map<String, Value>,
}

fn parse_model(body: &Map<String, Value>) -> Result<Model, ApiError> {
    let Some(sm) = body.get("smoothing").and_then(|v| v.as_object()) else {
        return Ok(Model::StupidBackoff(0.4f32 as f64));
    };
    let Some((name, cfg)) = sm.iter().next() else { return Ok(Model::StupidBackoff(0.4f32 as f64)) };
    let empty = Map::new();
    let cfg = cfg.as_object().unwrap_or(&empty);
    match name.as_str() {
        "stupid_backoff" => Ok(Model::StupidBackoff(num(cfg, "discount").unwrap_or(0.4))),
        "laplace" => Ok(Model::Laplace(num(cfg, "alpha").unwrap_or(0.5))),
        "linear" => {
            let t = num(cfg, "trigram_lambda").unwrap_or(0.0);
            let b = num(cfg, "bigram_lambda").unwrap_or(0.0);
            let u = num(cfg, "unigram_lambda").unwrap_or(0.0);
            let sum = t + b + u;
            if (sum - 1.0).abs() > 0.001 {
                return Err(bad("linear smoothing lambdas must sum to 1"));
            }
            Ok(Model::Linear(t / sum, b / sum, u / sum))
        }
        other => Err(bad(&format!("suggester[phrase] doesn't support object field [{}]", other))),
    }
}

/// Shingle filter settings of an analyzer: (min, max, output_unigrams).
fn shingle_info(ctx: &Ctx, analyzer: &str) -> Option<(usize, usize, bool)> {
    let def = ctx.section("analyzer", analyzer)?;
    let filters: Vec<String> = match def.get("filter") {
        Some(Value::Array(a)) => a.iter().filter_map(|v| v.as_str().map(|s| s.to_string())).collect(),
        Some(Value::String(s)) => {
            if let Ok(Value::Array(a)) = serde_json::from_str::<Value>(s) {
                a.iter().filter_map(|v| v.as_str().map(|s| s.to_string())).collect()
            } else {
                s.split(',').map(|x| x.trim().to_string()).collect()
            }
        }
        _ => vec![],
    };
    for f in filters {
        let fdef = ctx.section("filter", &f);
        let typ = fdef.and_then(|d| d.get("type")).and_then(|v| v.as_str()).unwrap_or(&f).to_string();
        if typ == "shingle" {
            let empty = Map::new();
            let d = fdef.unwrap_or(&empty);
            let gi = |k: &str, dv: usize| d.get(k).and_then(to_f64).map(|x| x as usize).unwrap_or(dv);
            let ou = d
                .get("output_unigrams")
                .map(|v| v.as_bool().unwrap_or(v.as_str() != Some("false")))
                .unwrap_or(true);
            return Some((gi("min_shingle_size", 2), gi("max_shingle_size", 2), ou));
        }
    }
    None
}

fn parse_mode(s: &str) -> Result<Mode, ApiError> {
    match s {
        "missing" => Ok(Mode::Missing),
        "popular" => Ok(Mode::Popular),
        "always" => Ok(Mode::Always),
        o => Err(bad(&format!(
            "No enum constant org.opensearch.search.suggest.term.TermSuggestionBuilder.SuggestMode.{}",
            o.to_uppercase()
        ))),
    }
}

/// Request-level validation of a phrase suggestion (parse time).
fn validate(body: &Map<String, Value>) -> Result<Vec<GenSpec>, ApiError> {
    for k in body.keys() {
        if !PHRASE_OPTIONS.contains(&k.as_str()) {
            return Err(ApiError::parsing(&format!("suggester[phrase] doesn't support field [{}]", k)));
        }
    }
    if let Some(h) = body.get("highlight").and_then(|v| v.as_object()) {
        if h.contains_key("pre_tag") != h.contains_key("post_tag") {
            return Err(bad("Pre and post tag must both be null or both not be null."));
        }
    }
    if let Some(c) = num(body, "confidence") {
        if c < 0.0 {
            return Err(bad("confidence must be >= 0.0"));
        }
    }
    if let Some(m) = num(body, "max_errors") {
        if m <= 0.0 {
            return Err(bad("max_error must be > 0.0"));
        }
    }
    if let Some(g) = num(body, "gram_size") {
        if g < 1.0 {
            return Err(bad("gramSize must be >= 1"));
        }
    }
    if let Some(r) = num(body, "real_word_error_likelihood") {
        if r <= 0.0 {
            return Err(bad("real_word_error_likelihood must be > 0.0"));
        }
    }
    if let Some(t) = num(body, "token_limit") {
        if t <= 0.0 {
            return Err(bad("token_limit must be >= 1"));
        }
    }
    if let Some(s) = num(body, "size") {
        if s <= 0.0 {
            return Err(bad("size must be positive"));
        }
    }
    parse_model(body)?;
    let mut gens = vec![];
    if let Some(dg) = body.get("direct_generator") {
        let list: Vec<&Value> = match dg {
            Value::Array(a) => a.iter().collect(),
            other => vec![other],
        };
        for g in list {
            let Some(m) = g.as_object() else { continue };
            if !m.contains_key("field") {
                return Err(bad("Required [field]"));
            }
            if let Some(s) = m.get("suggest_mode").and_then(|v| v.as_str()) {
                parse_mode(s)?;
            }
            gens.push(GenSpec { raw: m.clone() });
        }
    }
    Ok(gens)
}

/// Parse-time checks that need body positions.
pub fn check_body(name: &str, body: &Map<String, Value>, raw: &str) -> Option<ApiError> {
    let dg = body.get("direct_generator")?;
    let list: Vec<&Value> = match dg {
        Value::Array(a) => a.iter().collect(),
        other => vec![other],
    };
    for (i, g) in list.iter().enumerate() {
        let Some(m) = g.as_object() else { continue };
        for k in m.keys() {
            if !GENERATOR_OPTIONS.contains(&k.as_str()) {
                let is = i.to_string();
                let path: Vec<&str> = if dg.is_array() {
                    vec!["suggest", name, "phrase", "direct_generator", &is, k]
                } else {
                    vec!["suggest", name, "phrase", "direct_generator", k]
                };
                let pos = crate::suggest_completion::locate_key(raw, &path)
                    .map(|(l, c)| format!("[{}:{}] ", l, c))
                    .unwrap_or_default();
                let reason = format!("{}[direct_generator] unknown field [{}]", pos, k);
                return Some(ApiError::new("x_content_parse_exception", &reason, 400));
            }
        }
    }
    None
}

fn build_generator(
    idx: &Index,
    ctx: &Ctx,
    spec: &Map<String, Value>,
    rwel: f64,
) -> Result<Option<Generator>, ApiError> {
    let field = spec.get("field").and_then(value_to_string).unwrap_or_default();
    let Some(def) = crate::eval::mapping_field_def(idx, &field) else {
        return Err(shard_err(&idx.name, &format!("no mapping found for field [{}]", field)));
    };
    if !has_terms(def) {
        return Ok(None);
    }
    let stats = field_stats(idx, &field, def);
    if stats.df.is_empty() {
        return Ok(None);
    }
    let an = |k: &str| -> Result<Option<Analyzer>, ApiError> {
        match spec.get(k).and_then(|v| v.as_str()) {
            None => Ok(None),
            Some(n) => match ctx.analyzer(n) {
                Ok(Some(a)) => Ok(Some(a)),
                _ => Err(shard_err(&idx.name, &format!("analyzer [{}] doesn't exists", n))),
            },
        }
    };
    let mode = parse_mode(spec.get("suggest_mode").and_then(|v| v.as_str()).unwrap_or("missing"))?;
    let gnum = |keys: &[&str]| keys.iter().find_map(|k| num(spec, k));
    Ok(Some(Generator {
        stats,
        mode,
        size: gnum(&["size"]).unwrap_or(5.0) as usize,
        accuracy: gnum(&["accuracy"]).unwrap_or(0.5) as f32,
        max_edits: gnum(&["max_edits"]).unwrap_or(2.0) as usize,
        max_term_freq: gnum(&["max_term_freq"]).unwrap_or(0.01) as f32,
        prefix_length: gnum(&["prefix_length", "prefix_len"]).unwrap_or(1.0) as usize,
        min_word_length: gnum(&["min_word_length", "min_word_len"]).unwrap_or(4.0) as usize,
        min_doc_freq: gnum(&["min_doc_freq"]).unwrap_or(0.0) as f32,
        by_freq: spec.get("sort").and_then(|v| v.as_str()) == Some("frequency"),
        distance: spec.get("string_distance").and_then(|v| v.as_str()).unwrap_or("internal").to_string(),
        pre: an("pre_filter")?,
        post: an("post_filter")?,
        non_error_likelihood: rwel,
    }))
}

pub fn phrase_suggest(
    spec: &Map<String, Value>,
    body: &Map<String, Value>,
    indices: &[&Index],
    global_text: Option<&str>,
) -> Result<Value, ApiError> {
    let field = match body.get("field").and_then(value_to_string) {
        Some(f) => f,
        None => return Err(ApiError::new("parse_exception", "the required field option [field] is missing", 400)),
    };
    let gen_specs = validate(body)?;
    let model = parse_model(body)?;
    let text = spec.get("text").and_then(value_to_string).or_else(|| global_text.map(|s| s.to_string()));
    let size = num(body, "size").unwrap_or(5.0) as usize;
    let shard_size = num(body, "shard_size").map(|s| s as usize).unwrap_or(size.max(5));
    let rwel = num(body, "real_word_error_likelihood").unwrap_or(0.95) as f32 as f64;
    let confidence = num(body, "confidence").map(|c| c as f32).unwrap_or(1.0) as f64;
    let max_errors = num(body, "max_errors").map(|c| c as f32).unwrap_or(0.5);
    let separator = body.get("separator").and_then(value_to_string).unwrap_or_else(|| " ".into());
    let token_limit = num(body, "token_limit").unwrap_or(10.0) as usize;
    let require_unigram = body.get("force_unigrams").and_then(|v| v.as_bool()).unwrap_or(true);
    let highlight = body.get("highlight").and_then(|v| v.as_object()).and_then(|h| {
        Some((h.get("pre_tag").and_then(value_to_string)?, h.get("post_tag").and_then(value_to_string)?))
    });
    let collate = body.get("collate").and_then(|v| v.as_object());
    let collate_query = collate.and_then(|c| c.get("query")).and_then(|q| match q {
        Value::Object(m) => m.get("source").cloned().or_else(|| Some(q.clone())),
        _ => None,
    });
    let collate_params = collate.and_then(|c| c.get("params")).cloned().unwrap_or(json!({}));
    let prune = collate.and_then(|c| c.get("prune")).and_then(|v| v.as_bool()).unwrap_or(false);

    let mut all: Vec<(f32, String, Option<String>, Option<bool>)> = vec![];
    let mut entry_text = text.clone().unwrap_or_default();
    for idx in indices {
        let Some(text) = text.clone() else {
            return Err(shard_err(&idx.name, "The required text option is missing"));
        };
        entry_text = text.clone();
        let ctx = Ctx::for_index(idx);
        let Some(def) = crate::eval::mapping_field_def(idx, &field).cloned() else {
            return Err(shard_err(&idx.name, &format!("no mapping found for field [{}]", field)));
        };
        let an_name = match body.get("analyzer").and_then(|v| v.as_str()) {
            Some(a) => a.to_string(),
            None => search_analyzer_name(&ctx, &def),
        };
        let analyzer = match ctx.analyzer(&an_name) {
            Ok(Some(a)) => a,
            _ => return Err(shard_err(&idx.name, &format!("analyzer [{}] doesn't exists", an_name))),
        };
        let shingle = shingle_info(&ctx, &an_name);
        let gram = match num(body, "gram_size") {
            Some(g) => g as usize,
            None => shingle.map(|s| s.1).unwrap_or(1),
        };
        let mut gen_list = gen_specs.iter().map(|g| g.raw.clone()).collect::<Vec<_>>();
        if gen_list.is_empty() {
            if let Some((min, _, ou)) = shingle {
                if min > 1 && !ou && require_unigram {
                    return Err(shard_err(&idx.name, &format!(
                        "The default candidate generator for phrase suggest can't operate on field: [{}] since it doesn't emit unigrams. If this is intentional try to set the candidate generator field explicitly",
                        field
                    )));
                }
            }
            let mut m = Map::new();
            m.insert("field".into(), json!(field));
            gen_list.push(m);
        }
        let mut gens = vec![];
        for g in &gen_list {
            if let Some(gen) = build_generator(idx, &ctx, g, rwel)? {
                gens.push(gen);
            }
        }
        if !has_terms(&def) || gens.is_empty() {
            continue;
        }
        let stats = field_stats(idx, &field, &def);
        if stats.df.is_empty() {
            continue;
        }
        let scorer = WordScorer {
            stats: &stats,
            vocabulary: stats.sum_ttf as f64,
            num_terms: stats.num_terms() as f64,
            real_word_likelihood: rwel,
            sep: separator.clone(),
            model: model.clone(),
        };
        // candidate sets from the analyzed text
        let stream = analyzer.run(&text);
        let mut sets: Vec<CandSet> = vec![];
        let mut prev_pos: Option<usize> = None;
        for t in &stream.tokens {
            if require_unigram && t.typ == "shingle" {
                continue;
            }
            if prev_pos == Some(t.pos) && t.typ == "SYNONYM" {
                if let Some(cur) = sets.last_mut() {
                    let st = gens[0].term_stats(&t.term);
                    if st.0 > 0 {
                        cur.cands.push(gens[0].create(&t.term, st, rwel, false));
                    }
                }
                continue;
            }
            prev_pos = Some(t.pos);
            let st = gens[0].term_stats(&t.term);
            sets.push(CandSet { original: gens[0].create(&t.term, st, 1.0, true), cands: vec![] });
        }
        if sets.is_empty() || sets.len() >= token_limit {
            continue;
        }
        for set in sets.iter_mut() {
            for g in &gens {
                g.draw(set);
            }
            if set.cands.len() > shard_size {
                set.cands.sort_by(|a, b| b.score.partial_cmp(&a.score).unwrap_or(std::cmp::Ordering::Equal));
                set.cands.truncate(shard_size);
            }
        }
        let mut cutoff = f64::from_bits(1); // Double.MIN_VALUE
        if confidence > 0.0 {
            let originals: Vec<Cand> = sets.iter().map(|s| s.original.clone()).collect();
            let mut sum = 0.0;
            for i in 0..sets.len() {
                sum += scorer.score(&originals, &sets, i, gram);
            }
            cutoff = sum.exp() * confidence;
        }
        let num_miss = if max_errors >= 1.0 {
            max_errors as usize
        } else {
            (max_errors * sets.len() as f32).round() as usize
        };
        let mut search = Search { scorer: &scorer, gram, max: shard_size, cutoff, out: vec![] };
        let mut path: Vec<Cand> = sets.iter().map(|s| s.original.clone()).collect();
        search.find(&sets, &mut path, 0, num_miss.max(1), 0.0);
        let mut corrections = search.out;
        corrections.sort_by(|a, b| cmp_corr(b.score, &b.path, a.score, &a.path));
        for c in corrections {
            let phrase = c.path.iter().map(|x| x.term.as_str()).collect::<Vec<_>>().join(" ");
            let mut collate_match: Option<bool> = None;
            if let Some(q) = &collate_query {
                let matched = collate_matches(idx, q, &collate_params, &phrase)?;
                if prune {
                    collate_match = Some(matched);
                } else if !matched {
                    continue;
                }
            }
            let hl = highlight.as_ref().map(|(pre, post)| highlight_join(&c.path, pre, post));
            all.push((c.score as f32, phrase, hl, collate_match));
        }
    }
    all.sort_by(|a, b| {
        b.0.partial_cmp(&a.0).unwrap_or(std::cmp::Ordering::Equal)
    });
    all.truncate(size);
    let options: Vec<Value> = all
        .into_iter()
        .map(|(score, text, hl, cm)| {
            let mut o = Map::new();
            o.insert("text".into(), json!(text));
            if let Some(h) = hl {
                o.insert("highlighted".into(), json!(h));
            }
            o.insert("score".into(), crate::suggest_completion::f32_value(score));
            if let Some(m) = cm {
                o.insert("collate_match".into(), json!(m));
            }
            Value::Object(o)
        })
        .collect();
    Ok(json!([{"text": entry_text, "offset": 0, "length": entry_text.encode_utf16().count(), "options": options}]))
}

fn highlight_join(path: &[Cand], pre: &str, post: &str) -> String {
    let mut parts = vec![];
    for (i, c) in path.iter().enumerate() {
        if c.user_input {
            parts.push(c.term.clone());
            continue;
        }
        let mut s = String::new();
        if i == 0 || path[i - 1].user_input {
            s.push_str(pre);
        }
        s.push_str(&c.term);
        if i + 1 == path.len() || path[i + 1].user_input {
            s.push_str(post);
        }
        parts.push(s);
    }
    parts.join(" ")
}

fn collate_matches(idx: &Index, template: &Value, params: &Value, phrase: &str) -> Result<bool, ApiError> {
    let mut p = params.as_object().cloned().unwrap_or_default();
    p.insert("suggestion".into(), json!(phrase));
    let src = match template {
        Value::String(s) => s.clone(),
        other => other.to_string(),
    };
    let rendered = crate::render::render_mustache(&src, &Value::Object(p))
        .map_err(|_| ApiError::bad("failed to render collate query"))?;
    let q: Value = serde_json::from_str(&rendered).map_err(|_| ApiError::bad("failed to parse collate query"))?;
    let Some(qm) = q.as_object() else { return Ok(false) };
    let extra = crate::search::precompute_doc_values(idx);
    for id in &idx.order {
        if idx.pending.contains_key(id) {
            continue;
        }
        let Some(doc) = idx.docs.get(id) else { continue };
        let ctx = match extra.get(id) {
            Some(vals) => crate::eval::QueryCtx::with_values(idx, id, doc, vals.clone()),
            None => crate::eval::QueryCtx::new(idx, id, doc),
        };
        if ctx.eval(qm).map(|(ok, _)| ok).unwrap_or(false) {
            return Ok(true);
        }
    }
    Ok(false)
}
