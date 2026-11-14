//! Highlighting of text fields through their analyzers: the document value
//! is analyzed with the field's index analyzer and every token whose term is
//! a query term is marked at the token's offsets (UTF-16 code units, mapped
//! back to bytes). Query text of full-text queries is analyzed with the
//! field's search analyzer like the query itself; term-level values are
//! taken as they are.

use crate::analysis::search::{FieldAnalyzer, Side};
use crate::analysis::Analyzer;
use crate::eval::QueryCtx;
use crate::store::{value_to_string, Index};
use serde_json::{Map, Value};
use std::collections::HashSet;

/// The analyzers of a text field, when the analysis module can build them.
pub struct FieldAnalyzers {
    index: Analyzer,
    search: Analyzer,
    field: String,
}

fn resolved(fa: FieldAnalyzer) -> Option<Analyzer> {
    match fa {
        FieldAnalyzer::Resolved(a) => Some(a),
        FieldAnalyzer::Legacy(_) => None,
    }
}

impl FieldAnalyzers {
    pub fn new(idx: &Index, field: &str) -> Option<FieldAnalyzers> {
        let doc = Value::Null;
        let qc = QueryCtx::new(idx, "", &doc);
        let index = resolved(qc.field_analyzer(field, None, Side::Index))?;
        let search = resolved(qc.field_analyzer(field, None, Side::Search))?;
        Some(FieldAnalyzers { index, search, field: field.to_string() })
    }

    /// Query-time terms of `text` (`exact` for term-level values).
    pub fn query_terms(&self, idx: &Index, text: &str, opts: Option<&Map<String, Value>>, exact: bool) -> Vec<String> {
        if exact {
            return vec![text.to_string()];
        }
        let over = opts
            .and_then(|m| m.get("analyzer"))
            .and_then(|v| v.as_str())
            .filter(|s| !s.is_empty());
        if over.is_some() {
            let doc = Value::Null;
            let qc = QueryCtx::new(idx, "", &doc);
            if let Some(a) = resolved(qc.field_analyzer(&self.field, opts, Side::Search)) {
                return a.terms(text);
            }
        }
        self.search.terms(text)
    }

    /// Byte spans of the tokens of `text` whose term is one of `terms`.
    pub fn spans(&self, text: &str, terms: &[String]) -> Vec<(usize, usize)> {
        let set: HashSet<&str> = terms.iter().map(|s| s.as_str()).collect();
        if set.is_empty() {
            return Vec::new();
        }
        // UTF-16 offset -> byte offset.
        let mut byte_at: Vec<usize> = Vec::with_capacity(text.len() + 1);
        for (i, c) in text.char_indices() {
            for _ in 0..c.len_utf16() {
                byte_at.push(i);
            }
        }
        byte_at.push(text.len());
        let conv = |u: usize| -> usize { byte_at.get(u).copied().unwrap_or(text.len()) };
        let mut out = Vec::new();
        let mut last = 0usize;
        for t in self.index.run(text).tokens {
            if !set.contains(t.term.as_str()) {
                continue;
            }
            let (s, e) = (conv(t.start), conv(t.end));
            if s < last || e <= s {
                continue;
            }
            out.push((s, e));
            last = e;
        }
        out
    }
}

/// A scalar term-level value as text.
pub fn exact_value(v: &Value) -> Option<String> {
    match v {
        Value::Object(m) => m.get("value").and_then(value_to_string),
        _ => value_to_string(v),
    }
}
