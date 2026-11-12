//! Index-time and query-time analysis for queries: which analyzer applies
//! to a field (index analyzer, search_analyzer, search_quote_analyzer, the
//! index's `default`/`default_search`, or a query's `analyzer` override).

use serde_json::{Map, Value};

use super::{Analyzer, Ctx, Token};
use crate::eval::{mapping_field_def, resolve_alias_field, QueryCtx};

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum Side {
    Index,
    Search,
    Quote,
}

/// A resolved analyzer, or the name of one this module can't build (handled
/// by the simple legacy analysis).
pub enum FieldAnalyzer {
    Resolved(Analyzer),
    Legacy(String),
}

fn resolve(ctx: &Ctx, name: &str) -> FieldAnalyzer {
    match ctx.analyzer(name) {
        Ok(Some(a)) => FieldAnalyzer::Resolved(a),
        _ => FieldAnalyzer::Legacy(name.to_string()),
    }
}

impl<'a> QueryCtx<'a> {
    pub fn field_analyzer(&self, field: &str, opts: Option<&Map<String, Value>>, side: Side) -> FieldAnalyzer {
        let ctx = Ctx::for_index(self.idx);
        if side != Side::Index {
            if let Some(a) = opts.and_then(|m| m.get("analyzer")).and_then(|v| v.as_str()) {
                if !a.is_empty() {
                    return resolve(&ctx, a);
                }
            }
        }
        let resolved = resolve_alias_field(self.idx, field);
        let def = mapping_field_def(self.idx, &resolved);
        let get = |k: &str| -> Option<String> {
            def.and_then(|d| d.get(k)).and_then(|v| v.as_str()).filter(|s| !s.is_empty() && *s != "default").map(|s| s.to_string())
        };
        if side == Side::Quote {
            if let Some(a) = get("search_quote_analyzer") {
                return resolve(&ctx, &a);
            }
        }
        if side != Side::Index {
            if let Some(a) = get("search_analyzer") {
                return resolve(&ctx, &a);
            }
        }
        if let Some(a) = get("analyzer") {
            return resolve(&ctx, &a);
        }
        if side != Side::Index && ctx.section("analyzer", "default_search").is_some() {
            return resolve(&ctx, "default_search");
        }
        if ctx.section("analyzer", "default").is_some() {
            return resolve(&ctx, "default");
        }
        resolve(&ctx, "standard")
    }

    fn terms_with(&self, fa: FieldAnalyzer, text: &str) -> Vec<String> {
        match fa {
            FieldAnalyzer::Resolved(a) => a.terms(text),
            FieldAnalyzer::Legacy(name) => self.legacy_analyze(&name, text),
        }
    }

    fn positions_with(&self, fa: FieldAnalyzer, text: &str) -> Vec<(String, usize)> {
        match fa {
            FieldAnalyzer::Resolved(a) => a.run(text).tokens.into_iter().map(|t| (t.term, t.pos)).collect(),
            FieldAnalyzer::Legacy(name) => crate::eval::analyze_positions(&name, text),
        }
    }

    /// Query-time terms of `text` for `field`.
    pub fn query_tokens(&self, field: &str, opts: Option<&Map<String, Value>>, text: &str) -> Vec<String> {
        let fa = self.field_analyzer(field, opts, Side::Search);
        self.terms_with(fa, text)
    }

    /// Query-time (term, position) pairs; `quote` selects the phrase analyzer.
    pub fn query_positions(&self, field: &str, opts: Option<&Map<String, Value>>, text: &str, quote: bool) -> Vec<(String, usize)> {
        let fa = self.field_analyzer(field, opts, if quote { Side::Quote } else { Side::Search });
        self.positions_with(fa, text)
    }

    /// Index-time terms of a value of `field`.
    pub fn doc_tokens(&self, field: &str, text: &str) -> Vec<String> {
        let fa = self.field_analyzer(field, None, Side::Index);
        self.terms_with(fa, text)
    }

    /// Index-time (term, position) pairs of a value of `field`.
    pub fn doc_positions(&self, field: &str, text: &str) -> Vec<(String, usize)> {
        let fa = self.field_analyzer(field, None, Side::Index);
        self.positions_with(fa, text)
    }

    /// The query token stream when it is a graph (multi-word synonyms).
    fn query_graph(&self, field: &str, opts: Option<&Map<String, Value>>, text: &str, side: Side) -> Option<Vec<Token>> {
        match self.field_analyzer(field, opts, side) {
            FieldAnalyzer::Resolved(a) => {
                let toks = a.run(text).tokens;
                if toks.iter().any(|t| t.pos_len > 1) {
                    Some(toks)
                } else {
                    None
                }
            }
            FieldAnalyzer::Legacy(_) => None,
        }
    }

    fn doc_value_positions(&self, field: &str) -> Vec<Vec<(String, usize)>> {
        self.must_values(field)
            .iter()
            .map(|v| self.doc_positions(field, &crate::fields::value_scalar_string(v).unwrap_or_default()))
            .collect()
    }

    /// `match` with a graph token stream: each multi-position segment is an
    /// alternative of its paths (phrases when `auto_phrase`), segments are
    /// combined with the operator. None when the query is not a graph.
    pub fn graph_match(&self, field: &str, opts: Option<&Map<String, Value>>, text: &str, and: bool, auto_phrase: bool) -> Option<bool> {
        let toks = self.query_graph(field, opts, text, Side::Search)?;
        let docs = self.doc_value_positions(field);
        let has_term = |t: &str| docs.iter().any(|d| d.iter().any(|(x, _)| x == t));
        let has_phrase = |p: &[String]| docs.iter().any(|d| phrase_in(d, p, 0));
        let mut results = Vec::new();
        for seg in segments(&toks) {
            let paths = graph_paths(&seg);
            let ok = if paths.iter().all(|p| p.len() == 1) {
                paths.iter().any(|p| has_term(&p[0]))
            } else {
                paths.iter().any(|p| {
                    if p.len() == 1 {
                        has_term(&p[0])
                    } else if auto_phrase {
                        has_phrase(p)
                    } else if and {
                        p.iter().all(|t| has_term(t))
                    } else {
                        p.iter().any(|t| has_term(t))
                    }
                })
            };
            results.push(ok);
        }
        Some(if and { results.iter().all(|x| *x) } else { results.iter().any(|x| *x) })
    }

    /// `match_phrase` with a graph token stream: any path as a phrase.
    pub fn graph_phrase_match(&self, field: &str, opts: Option<&Map<String, Value>>, text: &str, slop: usize) -> Option<bool> {
        let toks = self.query_graph(field, opts, text, Side::Quote)?;
        let docs = self.doc_value_positions(field);
        let paths = graph_paths(&toks);
        Some(paths.iter().any(|p| docs.iter().any(|d| phrase_in(d, p, slop))))
    }
}

/// Whether the terms occur in order (within `slop` extra positions overall).
fn phrase_in(doc: &[(String, usize)], phrase: &[String], slop: usize) -> bool {
    if phrase.is_empty() {
        return false;
    }
    for (t, p) in doc {
        if *t != phrase[0] {
            continue;
        }
        let mut cur = *p;
        let mut used = 0usize;
        let mut ok = true;
        for w in &phrase[1..] {
            let next = doc
                .iter()
                .filter(|(x, q)| x == w && *q > cur && used + (*q - cur - 1) <= slop)
                .map(|(_, q)| *q)
                .min();
            match next {
                Some(q) => {
                    used += q - cur - 1;
                    cur = q;
                }
                None => {
                    ok = false;
                    break;
                }
            }
        }
        if ok && used <= slop {
            return true;
        }
    }
    false
}

/// Splits a token graph into segments at positions no token spans across.
fn segments(toks: &[Token]) -> Vec<Vec<Token>> {
    let mut sorted: Vec<Token> = toks.to_vec();
    sorted.sort_by_key(|t| t.pos);
    let mut out: Vec<Vec<Token>> = Vec::new();
    let mut cur: Vec<Token> = Vec::new();
    let mut reach = 0usize;
    for t in sorted {
        if !cur.is_empty() && t.pos >= reach {
            out.push(std::mem::take(&mut cur));
        }
        reach = reach.max(t.pos + t.pos_len.max(1));
        cur.push(t);
    }
    if !cur.is_empty() {
        out.push(cur);
    }
    out
}

/// All term paths through a token graph, from its first to its last position.
fn graph_paths(toks: &[Token]) -> Vec<Vec<String>> {
    let start = toks.iter().map(|t| t.pos).min().unwrap_or(0);
    let end = toks.iter().map(|t| t.pos + t.pos_len.max(1)).max().unwrap_or(0);
    let mut out = Vec::new();
    fn walk(toks: &[Token], at: usize, end: usize, path: &mut Vec<String>, out: &mut Vec<Vec<String>>) {
        if out.len() >= 64 {
            return;
        }
        if at >= end {
            out.push(path.clone());
            return;
        }
        let mut any = false;
        for t in toks.iter().filter(|t| t.pos == at) {
            any = true;
            path.push(t.term.clone());
            walk(toks, at + t.pos_len.max(1), end, path, out);
            path.pop();
        }
        if !any {
            walk(toks, at + 1, end, path, out);
        }
    }
    walk(toks, start, end, &mut Vec::new(), &mut out);
    out
}
