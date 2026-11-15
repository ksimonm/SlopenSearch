//! Index-backed query execution (see docs/engine-design.md).
//!
//! Keeps the inverted index of an `Index` (`model::inverted`) in step with its
//! doc values, and turns a query into a *candidate set*: a superset of the
//! documents the query can match, computed from postings. The search loop
//! then runs the exact evaluator (`QueryCtx::eval`) on the candidates only, so
//! which documents match and how they score cannot drift from the scanning
//! implementation. Query shapes this module does not understand yield no
//! plan, and the caller scans every document as before.
//!
//! Field kinds: `Text` fields hold the index-time analysis of the field's
//! values (the tokens `QueryCtx::doc_tokens` produces), with term frequencies
//! and token counts for BM25; `Exact` fields hold the values as strings
//! (`value_scalar_string`), for keyword terms, prefixes and exists.

use std::collections::HashSet;

use serde_json::{Map, Value};

use crate::jsonv::{to_f64, value_to_string};
use crate::mapping::fields::value_scalar_string;
use crate::mapping::{mapping_field_def, resolve_alias_field};
use crate::model::inverted::{FieldIndex, InvertedIndex};
use crate::model::Index;
use crate::search::analyzed::{FieldAnalyzer, Side};
use crate::search::eval::{array_values, as_query_list, minimum_should_match, minimum_should_match_checked, QueryCtx};

static NULL: Value = Value::Null;

#[derive(Clone, Copy, PartialEq)]
pub enum Kind {
    Text,
    Exact,
}

fn field_key(kind: Kind, field: &str) -> String {
    match kind {
        Kind::Text => format!("t\u{1}{}", field),
        Kind::Exact => format!("k\u{1}{}", field),
    }
}

fn parse_key(key: &str) -> (Kind, &str) {
    match key.split_once('\u{1}') {
        Some(("t", f)) => (Kind::Text, f),
        Some((_, f)) => (Kind::Exact, f),
        None => (Kind::Exact, key),
    }
}

fn schema(idx: &Index) -> (u64, u64) {
    (idx.mappings.generation(), idx.settings.generation())
}

/// Whether index-backed execution is disabled (`SLOP_NO_INDEX=true`).
pub fn disabled() -> bool {
    static OFF: std::sync::OnceLock<bool> = std::sync::OnceLock::new();
    *OFF.get_or_init(|| std::env::var("SLOP_NO_INDEX").map(|v| v == "true" || v == "1").unwrap_or(false))
}

/// Whether searches also scan and report documents the index missed
/// (`SLOP_VERIFY_INDEX=true`, for testing).
pub fn verify() -> bool {
    static ON: std::sync::OnceLock<bool> = std::sync::OnceLock::new();
    *ON.get_or_init(|| std::env::var("SLOP_VERIFY_INDEX").map(|v| v == "true" || v == "1").unwrap_or(false))
}

/// Produces the terms of one document's values of an indexed field.
struct Indexer<'a> {
    kind: Kind,
    /// The doc values key (the field with aliases resolved).
    path: String,
    analyzer: Option<FieldAnalyzer>,
    ctx: QueryCtx<'a>,
}

impl<'a> Indexer<'a> {
    fn new(idx: &'a Index, key: &str) -> Self {
        let (kind, field) = parse_key(key);
        let ctx = QueryCtx::new(idx, "", &NULL);
        let analyzer = match kind {
            Kind::Text => Some(ctx.field_analyzer(field, None, Side::Index)),
            Kind::Exact => None,
        };
        Indexer { kind, path: resolve_alias_field(idx, field), analyzer, ctx }
    }

    fn terms(&self, dv: &Map<String, Value>) -> Vec<String> {
        let mut out = Vec::new();
        let Some(raw) = dv.get(&self.path) else { return out };
        for v in array_values(raw) {
            match self.kind {
                Kind::Text => {
                    let s = value_scalar_string(&v).unwrap_or_default();
                    out.extend(self.ctx.terms_ref(self.analyzer.as_ref().unwrap(), &s));
                }
                Kind::Exact => {
                    if let Some(s) = value_scalar_string(&v) {
                        out.push(s);
                    }
                }
            }
        }
        out
    }
}

fn index_doc(idx: &Index, inv: &mut InvertedIndex, indexers: &[(String, Indexer)], id: &str) {
    let Some(dv) = idx.doc_values.get(id) else { return };
    let n = inv.alloc(id);
    for (key, ix) in indexers {
        if let Some(f) = inv.fields.get_mut(key) {
            f.add(n, ix.terms(dv));
        }
    }
}

fn indexers_for<'a>(idx: &'a Index, inv: &InvertedIndex) -> Vec<(String, Indexer<'a>)> {
    inv.fields.keys().map(|k| (k.clone(), Indexer::new(idx, k))).collect()
}

fn renumber(idx: &Index, inv: &mut InvertedIndex) {
    inv.reset_docs();
    let indexers = indexers_for(idx, inv);
    for id in idx.doc_values.keys() {
        index_doc(idx, inv, &indexers, id);
    }
    inv.numbered = true;
}

fn sync_locked(idx: &Index, inv: &mut InvertedIndex) {
    let sch = schema(idx);
    if inv.schema != sch {
        inv.fields.clear();
        inv.schema = sch;
    }
    let dirty = idx.doc_values.take_dirty();
    if dirty.all || !inv.numbered {
        renumber(idx, inv);
        return;
    }
    if dirty.ids.is_empty() {
        return;
    }
    let indexers = indexers_for(idx, inv);
    let mut ids: Vec<String> = dirty.ids.into_iter().collect();
    ids.sort();
    for id in &ids {
        inv.kill(id);
        index_doc(idx, inv, &indexers, id);
    }
    if inv.dead > 1024 && inv.dead > inv.live_count() {
        renumber(idx, inv);
    }
}

fn up_to_date(idx: &Index, inv: &InvertedIndex, keys: &[String]) -> bool {
    inv.numbered
        && inv.schema == schema(idx)
        && !idx.doc_values.is_dirty()
        && keys.iter().all(|k| inv.fields.contains_key(k))
}

/// Brings the index up to date and builds the fields `keys` when missing.
fn ensure(idx: &Index, keys: &[String]) {
    {
        let inv = idx.inverted.0.read().unwrap_or_else(|e| e.into_inner());
        if up_to_date(idx, &inv, keys) {
            return;
        }
    }
    let mut inv = idx.inverted.0.write().unwrap_or_else(|e| e.into_inner());
    sync_locked(idx, &mut inv);
    for key in keys {
        if inv.fields.contains_key(key) {
            continue;
        }
        let ix = Indexer::new(idx, key);
        let mut f = FieldIndex::default();
        for n in 0..inv.ids.len() {
            let Some(id) = inv.ids[n].as_deref() else { continue };
            if let Some(dv) = idx.doc_values.get(id) {
                f.add(n as u32, ix.terms(dv));
            }
        }
        inv.fields.insert(key.clone(), f);
    }
    inv.touch();
}

/// Runs `f` on the up-to-date index with the fields `keys` built.
fn with_index<R>(idx: &Index, keys: &[String], f: impl FnOnce(&InvertedIndex) -> R) -> R {
    loop {
        ensure(idx, keys);
        let inv = idx.inverted.0.read().unwrap_or_else(|e| e.into_inner());
        if up_to_date(idx, &inv, keys) {
            return f(&inv);
        }
    }
}

// ------------------------------------------------------------------ plans

/// A superset of the documents a query can match.
enum Plan {
    Empty,
    /// Documents with any of the terms.
    Any(String, Vec<String>),
    /// Documents with all of the terms.
    All(String, Vec<String>),
    /// Documents with a term starting with the prefix.
    Prefix(String, String),
    /// Documents with a value in the field.
    Exists(String),
    Ids(Vec<String>),
    And(Vec<Plan>),
    Or(Vec<Plan>),
}

impl Plan {
    fn keys(&self, out: &mut Vec<String>) {
        match self {
            Plan::Any(k, _) | Plan::All(k, _) | Plan::Prefix(k, _) | Plan::Exists(k) => {
                if !out.contains(k) {
                    out.push(k.clone());
                }
            }
            Plan::And(ps) | Plan::Or(ps) => ps.iter().for_each(|p| p.keys(out)),
            Plan::Empty | Plan::Ids(_) => {}
        }
    }
}

fn ftype_of(idx: &Index, field: &str) -> String {
    mapping_field_def(idx, field)
        .and_then(|d| d.get("type"))
        .and_then(|t| t.as_str())
        .unwrap_or("")
        .to_string()
}

/// Fields the index can serve directly: not an alias, not wildcarded, not
/// inside a nested object (those are evaluated per nested object).
fn plain_field(idx: &Index, field: &str) -> bool {
    !field.is_empty()
        && !field.contains('*')
        && !field.contains('?')
        && !field.contains('#')
        && !field.starts_with('_')
        && resolve_alias_field(idx, field) == field
        && crate::search::eval::nested_ancestor(idx, field).is_none()
}

fn keys_subset(m: &Map<String, Value>, allowed: &[&str]) -> bool {
    m.keys().all(|k| allowed.contains(&k.as_str()))
}

/// The plan of a `match` on a text field (also used by `multi_match`).
fn plan_match(ctx: &QueryCtx, field: &str, raw: &Value) -> Option<Plan> {
    let idx = ctx.idx;
    if !plain_field(idx, field) || ftype_of(idx, field) != "text" {
        return None;
    }
    let (text, opts) = crate::search::eval::query_text(raw);
    if let Some(m) = opts {
        if !keys_subset(m, &["query", "operator", "minimum_should_match", "boost", "_name", "analyzer", "auto_generate_synonyms_phrase_query"]) {
            return None;
        }
    }
    let qtokens = ctx.query_tokens(field, opts, &text);
    if qtokens.is_empty() {
        return Some(Plan::Empty);
    }
    if let FieldAnalyzer::Legacy(_) = ctx.field_analyzer(field, None, Side::Search) {
        if !crate::search::eval::synonym_groups(idx, field).is_empty() {
            return None;
        }
    }
    if ctx.query_graph(field, opts, &text, Side::Search).is_some() {
        return None;
    }
    let operator = opts
        .and_then(|m| m.get("operator"))
        .and_then(|o| o.as_str())
        .unwrap_or("or")
        .to_lowercase();
    let key = field_key(Kind::Text, field);
    if qtokens.len() > 1 {
        if let Some(spec) = opts.and_then(|m| m.get("minimum_should_match")) {
            return match minimum_should_match_checked(spec, qtokens.len()) {
                Ok(n) if n >= qtokens.len() => Some(Plan::All(key, qtokens)),
                Ok(n) if n >= 1 => Some(Plan::Any(key, qtokens)),
                _ => None,
            };
        }
        if operator == "and" {
            return Some(Plan::All(key, qtokens));
        }
    }
    Some(Plan::Any(key, qtokens))
}

fn single(m: &Map<String, Value>) -> Option<(&String, &Value)> {
    m.iter().next()
}

fn plan(ctx: &QueryCtx, q: &Map<String, Value>) -> Option<Plan> {
    let idx = ctx.idx;
    if q.len() != 1 {
        return None;
    }
    let (kind, body) = single(q)?;
    let bm = body.as_object();
    match kind.as_str() {
        "match_none" => Some(Plan::Empty),
        "match" => {
            let bm = bm?;
            if bm.len() != 1 {
                return None;
            }
            let (field, val) = single(bm)?;
            crate::search::eval::validate_match_query(idx, field, val).ok()?;
            plan_match(ctx, field, val)
        }
        "match_phrase" => {
            let bm = bm?;
            if bm.len() != 1 {
                return None;
            }
            let (field, val) = single(bm)?;
            if !plain_field(idx, field) || ftype_of(idx, field) != "text" {
                return None;
            }
            let (text, opts) = crate::search::eval::query_text(val);
            if let Some(m) = opts {
                if !keys_subset(m, &["query", "boost", "_name", "slop", "analyzer"]) {
                    return None;
                }
                if m.get("slop").and_then(to_f64).is_some_and(|s| s < 0.0) {
                    return None;
                }
            }
            if ctx.query_graph(field, opts, &text, Side::Quote).is_some() {
                return None;
            }
            let toks = ctx.query_positions(field, opts, &text, true);
            if toks.is_empty() {
                return None;
            }
            let terms: Vec<String> = toks.into_iter().map(|(t, _)| t).collect();
            Some(Plan::All(field_key(Kind::Text, field), terms))
        }
        "multi_match" => {
            let bm = bm?;
            if !keys_subset(bm, &["query", "fields", "type", "operator", "minimum_should_match", "boost", "tie_breaker", "_name"]) {
                return None;
            }
            let typ = bm.get("type").and_then(|t| t.as_str()).unwrap_or("best_fields");
            if typ != "best_fields" && typ != "most_fields" {
                return None;
            }
            let query = bm.get("query").and_then(value_to_string).unwrap_or_default();
            if query.is_empty() {
                return None;
            }
            let fields: Vec<String> = match bm.get("fields") {
                Some(Value::Array(a)) => a.iter().filter_map(value_to_string).collect(),
                Some(Value::String(s)) => vec![s.clone()],
                _ => return None,
            };
            if fields.is_empty() {
                return None;
            }
            let mut raw = Map::new();
            raw.insert("query".to_string(), Value::String(query));
            for k in ["operator", "minimum_should_match"] {
                if let Some(v) = bm.get(k) {
                    raw.insert(k.to_string(), v.clone());
                }
            }
            let raw = Value::Object(raw);
            let mut parts = Vec::new();
            for f in &fields {
                let name = match f.find('^') {
                    Some(i) => &f[..i],
                    None => f.as_str(),
                };
                parts.push(plan_match(ctx, name, &raw)?);
            }
            Some(Plan::Or(parts))
        }
        "term" => {
            let bm = bm?;
            if bm.len() != 1 {
                return None;
            }
            let (field, raw) = single(bm)?;
            if !plain_field(idx, field) {
                return None;
            }
            let mut val = raw;
            if let Some(m) = raw.as_object() {
                if !keys_subset(m, &["value", "boost", "_name"]) {
                    return None;
                }
                val = m.get("value")?;
            }
            let nval = crate::mapping::fields::normalize_query_value(idx, field, val);
            match ftype_of(idx, field).as_str() {
                "keyword" => match value_scalar_string(&nval) {
                    Some(t) => Some(Plan::Any(field_key(Kind::Exact, field), vec![t])),
                    None => Some(Plan::Empty),
                },
                "text" => {
                    let t = value_scalar_string(&nval)?;
                    Some(Plan::Any(field_key(Kind::Text, field), vec![t]))
                }
                _ => None,
            }
        }
        "terms" => {
            let bm = bm?;
            let mut field = None;
            for k in bm.keys() {
                if k != "boost" && k != "_name" {
                    field = Some(k);
                    break;
                }
            }
            let field = field?;
            if bm.keys().any(|k| k != field && k != "boost" && k != "_name") {
                return None;
            }
            if !plain_field(idx, field) || ftype_of(idx, field) != "keyword" {
                return None;
            }
            if crate::settings::find_setting(&idx.settings, "max_terms_count").is_some() {
                return None;
            }
            let list = bm.get(field)?.as_array()?;
            let mut terms = Vec::new();
            for item in list {
                if item.is_object() {
                    return None;
                }
                let n = crate::mapping::fields::normalize_query_value(idx, field, item);
                terms.push(value_scalar_string(&n).unwrap_or_default());
            }
            Some(Plan::Any(field_key(Kind::Exact, field), terms))
        }
        "prefix" => {
            let bm = bm?;
            if bm.len() != 1 {
                return None;
            }
            let (field, raw) = single(bm)?;
            if !plain_field(idx, field) || ftype_of(idx, field) != "keyword" {
                return None;
            }
            let mut val = raw;
            if let Some(m) = raw.as_object() {
                if !keys_subset(m, &["value", "boost", "_name", "rewrite"]) {
                    return None;
                }
                val = m.get("value")?;
            }
            let nval = crate::mapping::fields::normalize_query_value(idx, field, val);
            match value_scalar_string(&nval) {
                Some(p) => Some(Plan::Prefix(field_key(Kind::Exact, field), p)),
                None => Some(Plan::Empty),
            }
        }
        "exists" => {
            let bm = bm?;
            if !keys_subset(bm, &["field", "boost", "_name"]) {
                return None;
            }
            let field = bm.get("field").and_then(value_to_string)?;
            if !plain_field(idx, &field) || idx.mappings.get("derived").is_some() {
                return None;
            }
            let def = mapping_field_def(idx, &field)?;
            if def.get("type").and_then(|t| t.as_str()) != Some("keyword") {
                return None;
            }
            if def.contains_key("fields") || def.contains_key("properties") || def.contains_key("index") || def.contains_key("doc_values") {
                return None;
            }
            Some(Plan::Exists(field_key(Kind::Exact, &field)))
        }
        "ids" => {
            let bm = bm?;
            let list: Vec<String> = match bm.get("values")? {
                Value::Array(a) => a.iter().map(|v| value_scalar_string(v).unwrap_or_default()).collect(),
                _ => return None,
            };
            Some(Plan::Ids(list))
        }
        "bool" => {
            let bm = bm?;
            if !keys_subset(bm, &["must", "should", "filter", "must_not", "minimum_should_match", "boost", "adjust_pure_negative", "_name"]) {
                return None;
            }
            let mut and = Vec::new();
            for k in ["must", "filter"] {
                for c in as_query_list(bm.get(k)) {
                    if let Some(p) = c.as_object().and_then(|m| plan(ctx, m)) {
                        and.push(p);
                    }
                }
            }
            let should = as_query_list(bm.get("should"));
            let has_positives = bm.get("must").is_some_and(|v| !as_query_list(Some(v)).is_empty())
                || bm.get("filter").is_some_and(|v| !as_query_list(Some(v)).is_empty());
            let msm = match bm.get("minimum_should_match") {
                Some(spec) => minimum_should_match(spec, should.len(), 0),
                None if !has_positives => 1,
                None => 0,
            };
            if !should.is_empty() && msm >= 1 {
                let mut or = Vec::new();
                let mut all = true;
                for c in &should {
                    match c.as_object().and_then(|m| plan(ctx, m)) {
                        Some(p) => or.push(p),
                        None => {
                            all = false;
                            break;
                        }
                    }
                }
                if all {
                    and.push(Plan::Or(or));
                }
            }
            if and.is_empty() {
                return None;
            }
            Some(Plan::And(and))
        }
        "constant_score" => plan(ctx, bm?.get("filter")?.as_object()?),
        "boosting" => plan(ctx, bm?.get("positive")?.as_object()?),
        "dis_max" => {
            let qs = bm?.get("queries")?.as_array()?;
            if qs.is_empty() {
                return None;
            }
            let mut or = Vec::new();
            for c in qs {
                or.push(plan(ctx, c.as_object()?)?);
            }
            Some(Plan::Or(or))
        }
        _ => None,
    }
}

// ------------------------------------------------------------- execution

fn union(a: Vec<u32>, b: &[u32]) -> Vec<u32> {
    let mut out = Vec::with_capacity(a.len() + b.len());
    let (mut i, mut j) = (0, 0);
    while i < a.len() && j < b.len() {
        if a[i] < b[j] {
            out.push(a[i]);
            i += 1;
        } else if a[i] > b[j] {
            out.push(b[j]);
            j += 1;
        } else {
            out.push(a[i]);
            i += 1;
            j += 1;
        }
    }
    out.extend_from_slice(&a[i..]);
    out.extend_from_slice(&b[j..]);
    out
}

fn intersect(a: &[u32], b: &[u32]) -> Vec<u32> {
    let mut out = Vec::with_capacity(a.len().min(b.len()));
    let (mut i, mut j) = (0, 0);
    while i < a.len() && j < b.len() {
        if a[i] < b[j] {
            i += 1;
        } else if a[i] > b[j] {
            j += 1;
        } else {
            out.push(a[i]);
            i += 1;
            j += 1;
        }
    }
    out
}

fn live_docs(inv: &InvertedIndex, p: &[(u32, u32)]) -> Vec<u32> {
    p.iter().map(|e| e.0).filter(|d| inv.live(*d)).collect()
}

/// Live doc numbers of a plan, ascending.
fn run(inv: &InvertedIndex, p: &Plan) -> Vec<u32> {
    match p {
        Plan::Empty => Vec::new(),
        Plan::Any(k, terms) => {
            let f = &inv.fields[k];
            let mut out = Vec::new();
            for t in terms {
                if let Some(ps) = f.postings.get(t) {
                    out = union(out, &live_docs(inv, ps));
                }
            }
            out
        }
        Plan::All(k, terms) => {
            let f = &inv.fields[k];
            let mut lists: Vec<&Vec<(u32, u32)>> = Vec::new();
            for t in terms {
                match f.postings.get(t) {
                    Some(ps) => lists.push(ps),
                    None => return Vec::new(),
                }
            }
            lists.sort_by_key(|l| l.len());
            let mut out = live_docs(inv, lists[0]);
            for l in &lists[1..] {
                let ds: Vec<u32> = l.iter().map(|e| e.0).collect();
                out = intersect(&out, &ds);
            }
            out
        }
        Plan::Prefix(k, prefix) => {
            let f = &inv.fields[k];
            let mut out = Vec::new();
            for (t, ps) in &f.postings {
                if t.starts_with(prefix.as_str()) {
                    out = union(out, &live_docs(inv, ps));
                }
            }
            out
        }
        Plan::Exists(k) => {
            let f = &inv.fields[k];
            (0..f.lens.len() as u32).filter(|d| f.lens[*d as usize] > 0 && inv.live(*d)).collect()
        }
        Plan::Ids(ids) => {
            let mut out: Vec<u32> = ids.iter().filter_map(|id| inv.doc_of(id)).collect();
            out.sort_unstable();
            out.dedup();
            out
        }
        Plan::And(ps) => {
            let mut sets: Vec<Vec<u32>> = ps.iter().map(|p| run(inv, p)).collect();
            sets.sort_by_key(|s| s.len());
            let mut it = sets.into_iter();
            let mut out = it.next().unwrap_or_default();
            for s in it {
                out = intersect(&out, &s);
            }
            out
        }
        Plan::Or(ps) => {
            let mut out = Vec::new();
            for p in ps {
                out = union(out, &run(inv, p));
            }
            out
        }
    }
}

/// The ids of the documents `query` can match, or None when every document
/// has to be evaluated (unsupported query, or documents without doc values).
pub fn candidates(idx: &Index, query: &Map<String, Value>, extra_values: bool) -> Option<HashSet<String>> {
    if disabled() || extra_values || query.is_empty() || idx.uuid.is_empty() {
        return None;
    }
    let ctx = QueryCtx::new(idx, "", &NULL);
    let p = plan(&ctx, query)?;
    let mut keys = Vec::new();
    p.keys(&mut keys);
    with_index(idx, &keys, |inv| {
        let docs = run(inv, &p);
        Some(docs.into_iter().filter_map(|d| inv.id_of(d).map(|s| s.to_string())).collect())
    })
}

/// BM25 inputs of a `match` on a text field from the index: docs with the
/// field, their total token count, the document frequency of each query
/// token, the token count of document `id` and the frequency of each query
/// token in it. None when the document is not indexed.
pub struct MatchStats {
    pub total_docs: usize,
    pub total_len: usize,
    pub dfs: Vec<usize>,
    pub dl: usize,
    pub tfs: Vec<usize>,
}

pub fn match_stats(idx: &Index, field: &str, id: &str, qtokens: &[String]) -> Option<MatchStats> {
    if disabled() || idx.uuid.is_empty() || !plain_field(idx, field) {
        return None;
    }
    let key = field_key(Kind::Text, field);
    with_index(idx, std::slice::from_ref(&key), |inv| {
        let f = &inv.fields[&key];
        let doc = inv.doc_of(id)?;
        let dfs = qtokens
            .iter()
            .map(|t| match f.postings.get(t) {
                Some(p) => cached_df(inv, &key, t, p),
                None => 0,
            })
            .collect();
        Some(MatchStats {
            total_docs: f.docs_with,
            total_len: f.sum_len as usize,
            dfs,
            dl: f.len_of(doc) as usize,
            tfs: qtokens.iter().map(|t| f.tf(t, doc) as usize).collect(),
        })
    })
}

thread_local! {
    /// Live document frequencies, keyed by the index content stamp, field and term.
    static DF_CACHE: std::cell::RefCell<std::collections::HashMap<(u64, String, String), usize>> =
        std::cell::RefCell::new(std::collections::HashMap::new());
}

fn cached_df(inv: &InvertedIndex, key: &str, term: &str, p: &[(u32, u32)]) -> usize {
    if inv.dead == 0 {
        return p.len();
    }
    let ck = (inv.version, key.to_string(), term.to_string());
    if let Some(df) = DF_CACHE.with(|c| c.borrow().get(&ck).copied()) {
        return df;
    }
    let df = p.iter().filter(|e| inv.live(e.0)).count();
    DF_CACHE.with(|c| {
        let mut c = c.borrow_mut();
        if c.len() > 4096 {
            c.clear();
        }
        c.insert(ck, df);
    });
    df
}
