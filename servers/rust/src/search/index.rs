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
use crate::search::eval::{as_query_list, minimum_should_match, minimum_should_match_checked, QueryCtx};

static NULL: Value = Value::Null;

#[derive(Clone, Copy, PartialEq)]
pub enum Kind {
    Text,
    Exact,
    Numeric,
    /// geo_point latitudes (in `nums`), for distance prefilters.
    Geo,
    /// Numeric aggregation values per document (`fwd`), from the stored source.
    Metric,
}

fn field_key(kind: Kind, field: &str) -> String {
    match kind {
        Kind::Text => format!("t\u{1}{}", field),
        Kind::Exact => format!("k\u{1}{}", field),
        Kind::Numeric => format!("n\u{1}{}", field),
        Kind::Geo => format!("g\u{1}{}", field),
        Kind::Metric => format!("a\u{1}{}", field),
    }
}

fn parse_key(key: &str) -> (Kind, &str) {
    match key.split_once('\u{1}') {
        Some(("t", f)) => (Kind::Text, f),
        Some(("n", f)) => (Kind::Numeric, f),
        Some(("g", f)) => (Kind::Geo, f),
        Some(("a", f)) => (Kind::Metric, f),
        Some((_, f)) => (Kind::Exact, f),
        None => (Kind::Exact, key),
    }
}

fn schema(idx: &Index) -> (u64, u64) {
    (idx.mappings.generation(), idx.settings.generation())
}

/// What the terms of field `key` depend on besides the doc values: text
/// fields are analyzed with the analyzer the mapping and the analysis
/// settings select. A field whose signature changes is rebuilt.
fn signature(idx: &Index, key: &str) -> String {
    let (kind, field) = parse_key(key);
    match kind {
        Kind::Text => {
            let def = mapping_field_def(idx, field).map(|d| Value::Object(d.clone())).unwrap_or(Value::Null);
            format!("{}\u{1}{}\u{1}{}", idx.settings.generation(), resolve_alias_field(idx, field), def)
        }
        Kind::Exact | Kind::Numeric | Kind::Geo => resolve_alias_field(idx, field),
        Kind::Metric => {
            // the aggregation reading of the source depends on the mapping
            let def = mapping_field_def(idx, field).map(|d| Value::Object(d.clone())).unwrap_or(Value::Null);
            let dynamic = idx.mappings.get("dynamic").cloned().unwrap_or(Value::Null);
            format!("{}\u{1}{}\u{1}{}", idx.settings.generation(), def, dynamic)
        }
    }
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
    analyzer: Option<std::sync::Arc<FieldAnalyzer>>,
    ctx: QueryCtx<'a>,
}

impl<'a> Indexer<'a> {
    fn new(idx: &'a Index, key: &str) -> Self {
        let (kind, field) = parse_key(key);
        let ctx = QueryCtx::new(idx, "", &NULL);
        let analyzer = match kind {
            Kind::Text => Some(ctx.field_analyzer(field, None, Side::Index)),
            Kind::Exact | Kind::Numeric | Kind::Geo | Kind::Metric => None,
        };
        Indexer { kind, path: resolve_alias_field(idx, field), analyzer, ctx }
    }

    fn add(&self, f: &mut FieldIndex, doc: u32, dv: &Map<String, Value>) {
        if self.kind == Kind::Numeric {
            if let Some(raw) = dv.get(&self.path) {
                match raw {
                    Value::Array(a) => f.add_numbers(doc, a),
                    other => f.add_numbers(doc, std::slice::from_ref(other)),
                }
            }
            return;
        }
        if self.kind == Kind::Geo {
            // the latitudes of the values `eval_geo_distance` reads
            if let Some(raw) = dv.get(&self.path) {
                let lats: Vec<Value> = crate::search::eval::array_values(raw)
                    .iter()
                    .filter_map(crate::mapping::geo::geo_point_of)
                    .filter_map(|(lat, _)| serde_json::Number::from_f64(lat).map(Value::Number))
                    .collect();
                f.add_numbers(doc, &lats);
            }
            return;
        }
        f.add(doc, self.terms(dv));
    }

    fn terms(&self, dv: &Map<String, Value>) -> Vec<String> {
        let mut out = Vec::new();
        let Some(raw) = dv.get(&self.path) else { return out };
        let vals = match raw {
            Value::Array(a) => a.as_slice(),
            other => std::slice::from_ref(other),
        };
        for v in vals {
            match self.kind {
                Kind::Numeric | Kind::Geo | Kind::Metric => {}
                Kind::Text => {
                    let s = match v {
                        Value::String(s) => std::borrow::Cow::Borrowed(s.as_str()),
                        other => std::borrow::Cow::Owned(value_scalar_string(other).unwrap_or_default()),
                    };
                    out.extend(self.ctx.terms_ref(self.analyzer.as_deref().unwrap(), &s));
                }
                Kind::Exact => {
                    if let Some(s) = value_scalar_string(v) {
                        out.push(s);
                    }
                }
            }
        }
        out
    }
}

fn threads() -> usize {
    std::thread::available_parallelism().map(|n| n.get()).unwrap_or(1).min(16)
}

/// Adds the documents `docs` (ascending doc numbers above every number of
/// the field, all live) to the field `key`; large batches are analyzed on
/// several threads, each building the postings of a contiguous chunk.
fn fill(idx: &Index, key: &str, f: &mut FieldIndex, docs: &[(u32, String)]) {
    let ix = Indexer::new(idx, key);
    if ix.kind == Kind::Metric {
        use rayon::prelude::*;
        let field = parse_key(key).1;
        let value = |id: &String| idx.docs.get(id).and_then(|doc| crate::aggs::metric_column_value(idx, doc, field));
        if docs.len() < 4096 {
            for (n, id) in docs {
                f.set_fwd(*n, value(id));
            }
        } else {
            let vals: Vec<Option<f64>> = docs.par_iter().map(|(_, id)| value(id)).collect();
            for ((n, _), v) in docs.iter().zip(vals) {
                f.set_fwd(*n, v);
            }
        }
        return;
    }
    let threads = threads();
    let min_chunk = if ix.kind == Kind::Text { 256 } else { 8192 };
    if docs.len() < 2 * min_chunk || threads < 2 {
        for (n, id) in docs {
            if let Some(dv) = idx.doc_values.get(id) {
                ix.add(f, *n, dv);
            }
        }
        return;
    }
    use rayon::prelude::*;
    let chunk = docs.len().div_ceil(threads * 2).max(min_chunk);
    // chunks in order, merged pairwise in parallel (each merge appends
    // the postings of higher doc numbers)
    let built = docs
        .par_chunks(chunk)
        .map(|part| {
            let ix = Indexer::new(idx, key);
            let mut local = FieldIndex::default();
            for (n, id) in part {
                if let Some(dv) = idx.doc_values.get(id) {
                    ix.add(&mut local, *n, dv);
                }
            }
            local
        })
        .reduce(FieldIndex::default, |mut a, b| {
            a.append(b);
            a
        });
    f.append(built);
}

/// Builds the fields `keys` over the documents `docs`, several fields at once.
fn fill_fields(idx: &Index, inv: &mut InvertedIndex, keys: &[String], docs: &[(u32, String)]) {
    let mut taken: Vec<(String, FieldIndex)> = keys
        .iter()
        .map(|k| {
            let f = inv.fields.remove(k).unwrap_or_else(|| FieldIndex { sig: signature(idx, k), ..FieldIndex::default() });
            (k.clone(), f)
        })
        .collect();
    if taken.len() > 1 && docs.len() >= 128 {
        use rayon::prelude::*;
        taken.par_iter_mut().for_each(|(k, f)| fill(idx, k, f, docs));
    } else {
        for (k, f) in taken.iter_mut() {
            fill(idx, k, f, docs);
        }
    }
    for (k, mut f) in taken {
        f.sort_nums();
        inv.fields.insert(k, f);
    }
}

/// Numbers the documents `ids` that have doc values and adds them to every field.
fn add_docs<'a>(idx: &Index, inv: &mut InvertedIndex, ids: impl Iterator<Item = &'a String>) {
    let docs: Vec<(u32, String)> = ids
        .filter(|id| idx.doc_values.contains_key(*id))
        .map(|id| (inv.alloc(id), id.clone()))
        .collect();
    let keys: Vec<String> = inv.fields.keys().cloned().collect();
    fill_fields(idx, inv, &keys, &docs);
}

fn renumber(idx: &Index, inv: &mut InvertedIndex) {
    inv.reset_docs();
    // in index order (doc numbers then follow it until documents change)
    let mut seen: HashSet<&str> = HashSet::with_capacity(idx.doc_values.len());
    let mut ids: Vec<&String> = Vec::with_capacity(idx.doc_values.len());
    for id in idx.order.iter().chain(idx.doc_values.keys()) {
        if idx.doc_values.contains_key(id) && seen.insert(id.as_str()) {
            ids.push(id);
        }
    }
    add_docs(idx, inv, ids.into_iter());
    inv.numbered = true;
}

fn sync_locked(idx: &Index, inv: &mut InvertedIndex) {
    let sch = schema(idx);
    if inv.schema != sch {
        let stale: Vec<String> = inv.fields.iter().filter(|(k, f)| f.sig != signature(idx, k)).map(|(k, _)| k.clone()).collect();
        for k in stale {
            inv.fields.remove(&k);
        }
        inv.schema = sch;
        inv.eager = None;
    }
    let dirty = idx.doc_values.take_dirty();
    let docs_dirty = idx.docs.take_dirty();
    if dirty.all || docs_dirty.all || !inv.missing_known {
        inv.missing = idx.docs.keys().filter(|id| !idx.doc_values.contains_key(*id)).cloned().collect();
        inv.missing_known = true;
    } else {
        for id in dirty.ids.iter().chain(docs_dirty.ids.iter()) {
            if idx.docs.contains_key(id) && !idx.doc_values.contains_key(id) {
                inv.missing.insert(id.clone());
            } else {
                inv.missing.remove(id);
            }
        }
    }
    if dirty.all || docs_dirty.all || !inv.numbered {
        renumber(idx, inv);
        return;
    }
    if dirty.ids.is_empty() && docs_dirty.ids.is_empty() {
        return;
    }
    let mut ids: Vec<String> = dirty.ids.into_iter().chain(docs_dirty.ids).collect::<HashSet<String>>().into_iter().collect();
    ids.sort();
    for id in &ids {
        inv.kill(id);
    }
    add_docs(idx, inv, ids.iter());
    if inv.dead > 1024 && inv.dead > inv.live_count() {
        renumber(idx, inv);
    }
}

fn up_to_date(idx: &Index, inv: &InvertedIndex, keys: &[String]) -> bool {
    inv.numbered
        && inv.schema == schema(idx)
        && !idx.doc_values.is_dirty()
        && !idx.docs.is_dirty()
        && keys.iter().all(|k| inv.fields.get(k).is_some_and(|f| f.nums_ready()))
}

/// Builds the fields `keys` that are missing (the index is in sync).
fn build_missing(idx: &Index, inv: &mut InvertedIndex, keys: &[String]) {
    let missing: Vec<String> = keys.iter().filter(|k| !inv.fields.contains_key(*k)).cloned().collect();
    if !missing.is_empty() {
        let t0 = std::time::Instant::now();
        let docs: Vec<(u32, String)> = inv
            .ids
            .iter()
            .enumerate()
            .filter_map(|(n, id)| id.as_ref().map(|id| (n as u32, id.clone())))
            .collect();
        fill_fields(idx, inv, &missing, &docs);
        trace_dur(&format!("built {} fields of {} over {} docs", missing.len(), idx.name, docs.len()), t0);
    }
    for key in keys {
        if let Some(f) = inv.fields.get_mut(key) {
            f.sort_nums();
        }
    }
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
    build_missing(idx, &mut inv, keys);
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

/// The fields indexed at write time: every plain text, keyword, numeric and
/// date field of the mapping (others are built on first use).
fn eager_keys(idx: &Index) -> Vec<String> {
    let mut keys = Vec::new();
    let mut fields: Vec<(&String, &String)> = idx.field_types.iter().collect();
    fields.sort();
    for (field, ftype) in fields {
        let kind = match ftype.as_str() {
            "text" => Kind::Text,
            "keyword" => Kind::Exact,
            "integer" | "long" | "short" | "byte" | "float" | "double" | "half_float" | "date" => Kind::Numeric,
            "geo_point" => Kind::Geo,
            _ => continue,
        };
        if !plain_field(idx, field) || ftype_of(idx, field) != *ftype {
            continue;
        }
        keys.push(field_key(kind, field));
        if kind == Kind::Numeric {
            keys.push(field_key(Kind::Metric, field));
        }
    }
    keys
}

/// Brings the inverted index of `idx` up to date with its documents and
/// builds every eagerly indexed field (after writes and at startup), so the
/// first search does not pay for indexing.
pub fn index_now(idx: &Index) {
    if disabled() || idx.uuid.is_empty() || idx.closed {
        return;
    }
    {
        let inv = idx.inverted.0.read().unwrap_or_else(|e| e.into_inner());
        if inv.eager.as_ref().is_some_and(|(s, keys)| *s == schema(idx) && up_to_date(idx, &inv, keys)) {
            return;
        }
    }
    let mut inv = idx.inverted.0.write().unwrap_or_else(|e| e.into_inner());
    sync_locked(idx, &mut inv);
    let keys = match &inv.eager {
        Some((s, keys)) if *s == schema(idx) => keys.clone(),
        _ => {
            let keys = eager_keys(idx);
            inv.eager = Some((schema(idx), keys.clone()));
            keys
        }
    };
    build_missing(idx, &mut inv, &keys);
    inv.touch();
}

/// [`index_now`] for the indices a request may have changed.
pub fn after_write(store: &crate::model::Store) {
    for idx in store.indices.values() {
        index_now(idx);
    }
}

/// [`index_now`] for every index of the store, several indices at once.
pub fn index_all(store: &crate::model::Store) {
    let t0 = std::time::Instant::now();
    let mut list: Vec<&Index> = store.indices.values().collect();
    list.sort_by_key(|i| std::cmp::Reverse(i.docs.len()));
    use rayon::prelude::*;
    list.par_iter().for_each(|idx| index_now(idx));
    trace_dur(&format!("indexed {} indices", list.len()), t0);
}

fn trace_dur(label: &str, t0: std::time::Instant) {
    if trace_on() {
        eprintln!("[trace] {:>8.2} ms {}", t0.elapsed().as_secs_f64() * 1000.0, label);
    }
}

/// Whether every document has doc values (None: index-backed execution is off).
pub fn all_docs_have_values(idx: &Index) -> Option<bool> {
    if disabled() {
        return None;
    }
    Some(with_index(idx, &[], |inv| inv.missing.is_empty()))
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
    /// Documents with a numeric value in [lo, hi].
    Range(String, f64, f64),
    And(Vec<Plan>),
    Or(Vec<Plan>),
}

impl Plan {
    fn keys(&self, out: &mut Vec<String>) {
        match self {
            Plan::Any(k, _) | Plan::All(k, _) | Plan::Prefix(k, _) | Plan::Exists(k) | Plan::Range(k, _, _) => {
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
    if let FieldAnalyzer::Legacy(_) = *ctx.field_analyzer(field, None, Side::Search) {
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

/// The plan of a `range` on a numeric or date field: the bounds parsed as the
/// evaluator parses them, widened so that rounding can only add candidates.
fn plan_range(ctx: &QueryCtx, field: &str, cond: &Map<String, Value>) -> Option<Plan> {
    let idx = ctx.idx;
    if !plain_field(idx, field) {
        return None;
    }
    if !keys_subset(cond, &["gte", "gt", "lte", "lt", "from", "to", "include_lower", "include_upper", "boost", "_name", "format", "time_zone", "relation"]) {
        return None;
    }
    let mut def = mapping_field_def(idx, field)?.clone();
    if let Some(f) = cond.get("format").and_then(|v| v.as_str()) {
        def.insert("format".to_string(), Value::String(f.to_string()));
    }
    let ftype = def.get("type").and_then(|t| t.as_str()).unwrap_or("").to_string();
    let is_date = ftype == "date";
    if !is_date && !matches!(ftype.as_str(), "integer" | "long" | "short" | "byte" | "float" | "double" | "half_float") {
        return None;
    }
    let tz = cond.get("time_zone").and_then(|r| r.as_str()).unwrap_or("").to_string();
    let bound = |v: &Value| -> Option<(f64, f64)> {
        if is_date {
            let a = to_f64(&crate::search::eval::parse_query_date(&def, v, &tz, false).ok()?)?;
            let b = to_f64(&crate::search::eval::parse_query_date(&def, v, &tz, true).ok()?)?;
            let slack = 2.0 * 86_400_000.0;
            Some((a.min(b) - slack, a.max(b) + slack))
        } else {
            let f = match v {
                Value::Number(n) => n.as_f64()?,
                Value::String(s) => s.trim().parse::<f64>().ok()?,
                _ => return None,
            };
            if !f.is_finite() {
                return None;
            }
            let slack = (f.abs() * 0.01).max(1.0);
            Some((f - slack, f + slack))
        }
    };
    let pick = |a: &str, b: &str, c: &str| cond.get(a).filter(|v| !v.is_null()).or_else(|| cond.get(b).filter(|v| !v.is_null())).or_else(|| cond.get(c).filter(|v| !v.is_null()));
    let lo = match pick("gte", "gt", "from") {
        Some(v) => bound(v)?.0,
        None => f64::NEG_INFINITY,
    };
    let hi = match pick("lte", "lt", "to") {
        Some(v) => bound(v)?.1,
        None => f64::INFINITY,
    };
    Some(Plan::Range(field_key(Kind::Numeric, field), lo, hi))
}

/// The plan of a `geo_distance` on a geo_point field: the documents with a
/// point whose latitude is within the distance (a great-circle distance is
/// never shorter than the latitude difference), widened for rounding. Only
/// for queries `eval_geo_distance` accepts without error.
fn plan_geo_distance(ctx: &QueryCtx, bm: &Map<String, Value>) -> Option<Plan> {
    let mut field = None;
    for (k, v) in bm {
        match k.as_str() {
            "distance" | "distance_type" | "boost" | "_name" | "validation_method" | "ignore_unmapped" => {}
            _ if field.is_none() => field = Some((k, v)),
            _ => return None,
        }
    }
    let (field, point) = field?;
    if !plain_field(ctx.idx, field) || ftype_of(ctx.idx, field) != "geo_point" {
        return None;
    }
    let distance = crate::mapping::geo::parse_distance_meters(bm.get("distance")?)?;
    let (plat, plon) = crate::mapping::geo::geo_point_of(point)?;
    if !crate::mapping::geo::valid_latlon(plat, plon) || !distance.is_finite() || distance < 0.0 {
        return None;
    }
    let deg = distance / (6371008.8 * std::f64::consts::PI / 180.0);
    let slack = deg * 0.01 + 0.001;
    Some(Plan::Range(field_key(Kind::Geo, field), plat - deg - slack, plat + deg + slack))
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
        "range" => {
            let bm = bm?;
            if bm.len() != 1 {
                return None;
            }
            let (field, cond) = single(bm)?;
            plan_range(ctx, field, cond.as_object()?)
        }
        "geo_distance" => plan_geo_distance(ctx, bm?),
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
        Plan::Range(k, lo, hi) => {
            let f = &inv.fields[k];
            let start = f.nums.partition_point(|e| e.0 < *lo);
            let mut out: Vec<u32> = f.nums[start..]
                .iter()
                .take_while(|e| e.0 <= *hi)
                .map(|e| e.1)
                .filter(|d| inv.live(*d))
                .collect();
            out.sort_unstable();
            out.dedup();
            out
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

/// The documents a search has to evaluate, as positions in the index order.
pub struct Candidates {
    /// Positions in `Index::order`, ascending.
    pub positions: Vec<usize>,
    /// For a top-level `match`: the evaluator's result for each position,
    /// computed from the postings (the same BM25 arithmetic).
    pub results: Option<Vec<(bool, f64)>>,
    /// Whether an id occurs more than once in the index order.
    pub dup_ids: bool,
}

impl Candidates {
    /// The index into `positions` of order position `i`, when it is a candidate.
    pub fn find(&self, i: usize) -> Option<usize> {
        self.positions.binary_search(&i).ok()
    }
}

const NO_DOC: u32 = u32::MAX;

/// Doc number per position of the index order (NO_DOC: not indexed), cached
/// per index content.
fn order_docs(idx: &Index, inv: &InvertedIndex) -> (std::sync::Arc<Vec<u32>>, bool) {
    let last = idx.order.last().and_then(|id| inv.doc_of(id));
    {
        let c = inv.order_map.lock().unwrap_or_else(|e| e.into_inner());
        if let Some(m) = c.as_ref() {
            if m.version == inv.version && m.docs.len() == idx.order.len() && m.last == last {
                return (m.docs.clone(), m.dups);
            }
        }
    }
    let mut seen = vec![false; inv.ids.len()];
    let mut dups = false;
    let docs: Vec<u32> = idx
        .order
        .iter()
        .map(|id| match inv.doc_of(id) {
            Some(d) => {
                dups |= std::mem::replace(&mut seen[d as usize], true);
                d
            }
            None => NO_DOC,
        })
        .collect();
    let docs = std::sync::Arc::new(docs);
    *inv.order_map.lock().unwrap_or_else(|e| e.into_inner()) =
        Some(crate::model::inverted::OrderMap { version: inv.version, last, docs: docs.clone(), dups });
    (docs, dups)
}

/// A top-level `match` the postings answer exactly (see [`match_results`]).
struct DirectMatch {
    key: String,
    def: Map<String, Value>,
    qtokens: Vec<String>,
    required: usize,
    boost: f64,
}

/// The direct form of a top-level `match` query: the evaluator would score
/// every document from the index statistics (`QueryCtx::eval_match` reaches
/// `score_match` for every indexed document under the conditions of
/// `plan_match`), so the result of each document follows from the postings.
fn direct_match(ctx: &QueryCtx, q: &Map<String, Value>, p: &Plan) -> Option<DirectMatch> {
    let (kind, body) = single(q)?;
    if kind != "match" {
        return None;
    }
    let (key, qtokens) = match p {
        Plan::Any(k, t) | Plan::All(k, t) => (k.clone(), t.clone()),
        _ => return None,
    };
    let (field, raw) = single(body.as_object()?)?;
    let (_, opts) = crate::search::eval::query_text(raw);
    let mut boost = 1.0;
    if let Some(b) = opts.and_then(|m| m.get("boost")).and_then(to_f64) {
        boost *= b;
    }
    let operator = opts.and_then(|m| m.get("operator")).and_then(|o| o.as_str()).unwrap_or("or").to_lowercase();
    let mut required = 1usize;
    if qtokens.len() > 1 {
        if let Some(spec) = opts.and_then(|m| m.get("minimum_should_match")) {
            required = minimum_should_match_checked(spec, qtokens.len()).ok()?;
        } else if operator == "and" {
            required = qtokens.len();
        }
    }
    if required == 0 {
        return None;
    }
    let def = mapping_field_def(ctx.idx, field)?.clone();
    Some(DirectMatch { key, def, qtokens, required, boost })
}

/// [`DirectMatch`] results of the documents `docs`.
fn match_results(inv: &InvertedIndex, dm: &DirectMatch, docs: impl Iterator<Item = u32>) -> Vec<(bool, f64)> {
    let f = &inv.fields[&dm.key];
    let lists: Vec<Option<&Vec<(u32, u32)>>> = dm.qtokens.iter().map(|t| f.postings.get(t)).collect();
    let dfs: Vec<usize> = dm
        .qtokens
        .iter()
        .zip(&lists)
        .map(|(t, p)| match p {
            Some(p) => cached_df(inv, &dm.key, t, p),
            None => 0,
        })
        .collect();
    let mut st = MatchStats { total_docs: f.docs_with, total_len: f.sum_len as usize, dfs, dl: 0, tfs: vec![0; dm.qtokens.len()] };
    // postings are sorted by doc and the documents come mostly ascending:
    // continue where each list was left
    let mut at = vec![0usize; lists.len()];
    docs.map(|doc| {
        st.dl = f.len_of(doc) as usize;
        for (qi, l) in lists.iter().enumerate() {
            st.tfs[qi] = match l {
                Some(p) => {
                    let mut i = at[qi];
                    if i >= p.len() || p[i].0 > doc {
                        i = p.partition_point(|e| e.0 < doc);
                    } else {
                        while i < p.len() && p[i].0 < doc {
                            i += 1;
                        }
                    }
                    at[qi] = i;
                    if i < p.len() && p[i].0 == doc {
                        p[i].1 as usize
                    } else {
                        0
                    }
                }
                None => 0,
            };
        }
        crate::search::eval::score_match(&dm.def, "text", &dm.qtokens, dm.required, dm.boost, &st)
    })
    .collect()
}

/// A top-level `range` on a numeric or date field: decided per candidate
/// from its doc values with the evaluator's own comparison
/// (`QueryCtx::range_on_values`), without the per-document query dispatch.
struct DirectRange {
    field: String,
    cond: Map<String, Value>,
}

fn direct_range(q: &Map<String, Value>, p: &Plan) -> Option<DirectRange> {
    let (kind, body) = single(q)?;
    if kind != "range" || !matches!(p, Plan::Range(..)) {
        return None;
    }
    let (field, cond) = single(body.as_object()?)?;
    Some(DirectRange { field: field.clone(), cond: cond.as_object()?.clone() })
}

/// A query the index answers per document exactly as the evaluator would
/// (`QueryCtx::eval`): a `match`, a keyword/text `term`, a numeric/date
/// `range`, or a `bool` of these (`eval_bool`'s arithmetic replayed on the
/// clause results).
enum Direct {
    Match(DirectMatch),
    Range(DirectRange),
    /// Matches the documents with the term (constant score `boost`).
    Term { key: String, term: String, boost: f64 },
    Bool(Box<DirectBool>),
}

struct DirectBool {
    must: Vec<Direct>,
    filter: Vec<Direct>,
    should: Vec<Direct>,
    must_not: Vec<Direct>,
    msm: Option<Value>,
    boost: f64,
    adjust: bool,
}

impl Direct {
    fn keys(&self, out: &mut Vec<String>) {
        let mut add = |k: String| {
            if !out.contains(&k) {
                out.push(k);
            }
        };
        match self {
            Direct::Match(m) => add(m.key.clone()),
            Direct::Range(r) => add(field_key(Kind::Numeric, &r.field)),
            Direct::Term { key, .. } => add(key.clone()),
            Direct::Bool(b) => {
                for d in b.must.iter().chain(&b.filter).chain(&b.should).chain(&b.must_not) {
                    d.keys(out);
                }
            }
        }
    }
}

fn direct_query(ctx: &QueryCtx, q: &Map<String, Value>) -> Option<Direct> {
    let p = plan(ctx, q)?;
    let (kind, body) = single(q)?;
    match kind.as_str() {
        "match" => direct_match(ctx, q, &p).map(Direct::Match),
        "range" => direct_range(q, &p).map(Direct::Range),
        "term" => {
            let (key, mut terms) = match p {
                Plan::Any(k, t) if t.len() == 1 => (k, t),
                _ => return None,
            };
            let (_, raw) = single(body.as_object()?)?;
            let boost = raw.as_object().and_then(|m| m.get("boost")).and_then(to_f64).unwrap_or(1.0);
            Some(Direct::Term { key, term: terms.pop()?, boost })
        }
        "bool" => {
            let bm = body.as_object()?;
            let list = |k: &str| -> Option<Vec<Direct>> {
                as_query_list(bm.get(k)).iter().map(|c| c.as_object().and_then(|m| direct_query(ctx, m))).collect()
            };
            Some(Direct::Bool(Box::new(DirectBool {
                must: list("must")?,
                filter: list("filter")?,
                should: list("should")?,
                must_not: list("must_not")?,
                msm: bm.get("minimum_should_match").cloned(),
                boost: bm.get("boost").and_then(to_f64).unwrap_or(1.0),
                adjust: bm.get("adjust_pure_negative").and_then(|v| v.as_bool()).unwrap_or(true),
            })))
        }
        _ => None,
    }
}

/// [`Direct`] with what it reads from the index resolved (shared by threads).
enum Prepared<'a> {
    Match { dm: &'a DirectMatch, lists: Vec<Option<&'a Vec<(u32, u32)>>>, dfs: Vec<usize>, f: &'a FieldIndex },
    /// Index into the range preparations.
    Range { field: &'a str, prep: usize },
    Term { list: Option<&'a Vec<(u32, u32)>>, boost: f64 },
    Bool { b: &'a DirectBool, must: Vec<Prepared<'a>>, filter: Vec<Prepared<'a>>, should: Vec<Prepared<'a>>, must_not: Vec<Prepared<'a>> },
}

fn prepare<'a>(ctx: &QueryCtx, inv: &'a InvertedIndex, d: &'a Direct, preps: &mut Vec<std::rc::Rc<crate::search::eval::RangePrep>>) -> Option<Prepared<'a>> {
    Some(match d {
        Direct::Match(dm) => {
            let f = &inv.fields[&dm.key];
            let lists: Vec<Option<&Vec<(u32, u32)>>> = dm.qtokens.iter().map(|t| f.postings.get(t)).collect();
            let dfs = dm
                .qtokens
                .iter()
                .zip(&lists)
                .map(|(t, p)| match p {
                    Some(p) => cached_df(inv, &dm.key, t, p),
                    None => 0,
                })
                .collect();
            Prepared::Match { dm, lists, dfs, f }
        }
        Direct::Range(dr) => {
            let prep = ctx.range_prep(&dr.field, &dr.cond);
            if prep.ftype == "wildcard" || prep.bounds.is_err() {
                return None;
            }
            preps.push(prep);
            Prepared::Range { field: &dr.field, prep: preps.len() - 1 }
        }
        Direct::Term { key, term, boost } => Prepared::Term { list: inv.fields[key].postings.get(term), boost: *boost },
        Direct::Bool(b) => {
            let mut all = |v: &'a [Direct]| -> Option<Vec<Prepared<'a>>> { v.iter().map(|d| prepare(ctx, inv, d, preps)).collect() };
            let must = all(&b.must)?;
            let filter = all(&b.filter)?;
            let should = all(&b.should)?;
            let must_not = all(&b.must_not)?;
            Prepared::Bool { b, must, filter, should, must_not }
        }
    })
}

fn tf_of(list: Option<&Vec<(u32, u32)>>, doc: u32) -> u32 {
    match list {
        Some(p) => match p.binary_search_by_key(&doc, |e| e.0) {
            Ok(i) => p[i].1,
            Err(_) => 0,
        },
        None => 0,
    }
}

/// The evaluator's result for document `doc` (None: undecidable here).
fn direct_result(idx: &Index, inv: &InvertedIndex, ctx: &QueryCtx, preps: &[&crate::search::eval::RangePrep], p: &Prepared, doc: u32) -> Option<(bool, f64)> {
    Some(match p {
        Prepared::Match { dm, lists, dfs, f } => {
            let st = MatchStats {
                total_docs: f.docs_with,
                total_len: f.sum_len as usize,
                dfs: dfs.clone(),
                dl: f.len_of(doc) as usize,
                tfs: lists.iter().map(|l| tf_of(*l, doc) as usize).collect(),
            };
            crate::search::eval::score_match(&dm.def, "text", &dm.qtokens, dm.required, dm.boost, &st)
        }
        Prepared::Range { field, prep } => {
            let raw = inv.ids.get(doc as usize)?.as_ref().and_then(|id| idx.doc_values.get(id)).and_then(|dv| dv.get(*field));
            match raw {
                Some(Value::Array(a)) if !a.is_empty() => ctx.range_on_values(preps[*prep], a, 1.0).ok()?,
                Some(Value::Array(_)) | None => (false, 0.0),
                Some(v) => ctx.range_on_values(preps[*prep], std::slice::from_ref(v), 1.0).ok()?,
            }
        }
        Prepared::Term { list, boost } => {
            if tf_of(*list, doc) > 0 {
                (true, *boost)
            } else {
                (false, 0.0)
            }
        }
        Prepared::Bool { b, must, filter, should, must_not } => {
            // `eval_bool`, clause by clause and in the same order
            let mut score = 0.0;
            for c in must.iter().chain(filter.iter()) {
                let (m, sc) = direct_result(idx, inv, ctx, preps, c, doc)?;
                if !m {
                    return Some((false, 0.0));
                }
                score += sc;
            }
            if !filter.is_empty() {
                let mut fscore = 0.0;
                for c in filter {
                    fscore += direct_result(idx, inv, ctx, preps, c, doc)?.1;
                }
                score -= fscore;
            }
            let mut should_matched = 0usize;
            for c in should {
                let (m, sc) = direct_result(idx, inv, ctx, preps, c, doc)?;
                if m {
                    should_matched += 1;
                    score += sc;
                }
            }
            let has_positives = !must.is_empty() || !filter.is_empty();
            let msm = match &b.msm {
                Some(spec) => minimum_should_match(spec, should.len(), should_matched),
                None if !has_positives => 1,
                None => 0,
            };
            if !should.is_empty() && should_matched < msm {
                return Some((false, 0.0));
            }
            if must.is_empty() && filter.is_empty() && should.is_empty() && !must_not.is_empty() && !b.adjust {
                return Some((false, 0.0));
            }
            for c in must_not {
                if direct_result(idx, inv, ctx, preps, c, doc)?.0 {
                    return Some((false, 0.0));
                }
            }
            if must.is_empty() && filter.is_empty() && should.is_empty() {
                if !must_not.is_empty() {
                    return Some((true, 0.0));
                }
                return Some((true, b.boost));
            }
            (true, b.boost * score)
        }
    })
}

/// [`Direct`] results of the documents `docs`, on several threads for large sets.
fn direct_results(idx: &Index, inv: &InvertedIndex, d: &Direct, docs: &[u32]) -> Option<Vec<(bool, f64)>> {
    let ctx = QueryCtx::new(idx, "", &NULL);
    // range bounds (and `now`) prepared once per request
    let mut owned = Vec::new();
    let p = prepare(&ctx, inv, d, &mut owned)?;
    let preps: Vec<&crate::search::eval::RangePrep> = owned.iter().map(|r| &**r).collect();
    if docs.len() < 8192 {
        return docs.iter().map(|&doc| direct_result(idx, inv, &ctx, &preps, &p, doc)).collect();
    }
    use rayon::prelude::*;
    let (p, preps) = (&p, &preps);
    let parts: Option<Vec<Vec<(bool, f64)>>> = docs
        .par_chunks(4096)
        .map(|chunk| {
            let ctx = QueryCtx::new(idx, "", &NULL);
            chunk.iter().map(|&doc| direct_result(idx, inv, &ctx, preps, p, doc)).collect()
        })
        .collect();
    Some(parts?.into_iter().flatten().collect())
}

/// The documents `query` can match, or None when every document has to be
/// evaluated (unsupported query, or documents without doc values).
pub fn candidates(idx: &Index, query: &Map<String, Value>, extra_values: bool) -> Option<Candidates> {
    if disabled() || extra_values || query.is_empty() || idx.uuid.is_empty() {
        return None;
    }
    let ctx = QueryCtx::new(idx, "", &NULL);
    let p = plan(&ctx, query)?;
    let direct = direct_match(&ctx, query, &p);
    let general = if direct.is_none() { direct_query(&ctx, query) } else { None };
    let mut keys = Vec::new();
    p.keys(&mut keys);
    if let Some(g) = &general {
        g.keys(&mut keys);
    }
    with_index(idx, &keys, |inv| {
        if keys.iter().any(|k| inv.fields[k].non_numeric) {
            return None;
        }
        let docs = run(inv, &p);
        let mut bits = vec![false; inv.ids.len()];
        for d in docs {
            bits[d as usize] = true;
        }
        let (order, dup_ids) = order_docs(idx, inv);
        let positions: Vec<usize> = order
            .iter()
            .enumerate()
            .filter(|(_, d)| **d != NO_DOC && bits[**d as usize])
            .map(|(i, _)| i)
            .collect();
        let mut results = direct.as_ref().map(|dm| match_results(inv, dm, positions.iter().map(|i| order[*i])));
        if let Some(g) = &general {
            let docs: Vec<u32> = positions.iter().map(|i| order[*i]).collect();
            results = direct_results(idx, inv, g, &docs);
        }
        Some(Candidates { positions, results, dup_ids })
    })
}

/// The metric column of a numeric/date field (see `Kind::Metric`) and the
/// index content version it belongs to.
pub fn metric_column(idx: &Index, field: &str) -> Option<(u64, std::sync::Arc<Vec<Option<f64>>>)> {
    if disabled() || idx.uuid.is_empty() || !plain_field(idx, field) {
        return None;
    }
    if !matches!(ftype_of(idx, field).as_str(), "integer" | "long" | "short" | "byte" | "float" | "double" | "half_float" | "scaled_float" | "date") {
        return None;
    }
    let key = field_key(Kind::Metric, field);
    Some(with_index(idx, std::slice::from_ref(&key), |inv| (inv.version, inv.fields[&key].fwd.clone())))
}

/// The doc numbers of `ids` and the index content version they belong to
/// (None when some id is not indexed).
pub fn doc_numbers(idx: &Index, ids: &[&str]) -> Option<(u64, Vec<u32>)> {
    if disabled() || idx.uuid.is_empty() {
        return None;
    }
    with_index(idx, &[], |inv| {
        let nums: Option<Vec<u32>> = ids.iter().map(|id| inv.doc_of(id)).collect();
        nums.map(|n| (inv.version, n))
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

thread_local! {
    static TRACE_START: std::cell::Cell<Option<std::time::Instant>> = const { std::cell::Cell::new(None) };
}

/// `SLOP_TRACE=true`: prints the time since the last `trace("start")` of this thread.
fn trace_on() -> bool {
    static ON: std::sync::OnceLock<bool> = std::sync::OnceLock::new();
    *ON.get_or_init(|| std::env::var("SLOP_TRACE").map(|v| v == "true" || v == "1").unwrap_or(false))
}

pub fn trace(label: &str) {
    if !trace_on() {
        return;
    }
    let now = std::time::Instant::now();
    TRACE_START.with(|s| {
        if label == "start" {
            s.set(Some(now));
        } else if let Some(t) = s.get() {
            eprintln!("[trace] {:>8.2} ms {}", (now - t).as_secs_f64() * 1000.0, label);
        }
    });
}
