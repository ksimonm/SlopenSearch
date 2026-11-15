//! Completion fields and the completion suggester: mapping parameters and
//! contexts (category / geo), index-time parsing of suggestion inputs, and
//! prefix / fuzzy / regex lookups modelled on Lucene's NRTSuggester.

use crate::mapping::completion::*;
use crate::errors::ApiError;
use crate::jsonv::{field_value, locate, to_f64, value_to_string};
use crate::model::Index;
use serde_json::{json, Map, Value};

const SEP: char = '\u{1f}';

// ------------------------------------------------------------ java helpers

fn java_string_hash(s: &str) -> i32 {
    let mut h: i32 = 0;
    for u in s.encode_utf16() {
        h = h.wrapping_mul(31).wrapping_add(u as i32);
    }
    h
}

fn table_size_for(n: usize) -> usize {
    let mut c = 1usize;
    while c < n {
        c <<= 1;
    }
    c
}

/// Iteration order of a java.util.HashMap / HashSet holding `keys` (inserted
/// in this order, distinct), created with the given initial capacity.
pub fn java_hash_order(keys: &[String], initial_capacity: usize) -> Vec<usize> {
    let mut cap = table_size_for(initial_capacity.max(1));
    for size in 1..=keys.len() {
        if size > cap * 3 / 4 {
            cap *= 2;
        }
    }
    let mut idx: Vec<usize> = (0..keys.len()).collect();
    idx.sort_by_key(|&i| {
        let h = java_string_hash(&keys[i]) as u32;
        ((h ^ (h >> 16)) as usize) & (cap - 1)
    });
    idx
}

fn pos_prefix(raw: &str, path: &[&str]) -> String {
    match locate(raw, path) {
        Some((l, c)) => format!("[{}:{}] ", l, c),
        None => String::new(),
    }
}

/// Request-level checks of the `suggest` section that OpenSearch makes while
/// parsing the search body (they carry body positions).
pub fn check_suggest_body(body: &Map<String, Value>, raw: &str) -> Option<ApiError> {
    let suggest = body.get("suggest")?.as_object()?;
    for (name, sv) in suggest {
        if name == "text" {
            continue;
        }
        let spec = match sv.as_object() {
            Some(s) => s,
            None => continue,
        };
        let mut found = false;
        for (k, v) in spec {
            if !v.is_object() {
                continue;
            }
            if !matches!(k.as_str(), "term" | "phrase" | "completion") {
                let reason = format!("{}unknown field [{}]", pos_prefix(raw, &["suggest", name, k]), k);
                return Some(ApiError::new("named_object_not_found_exception", &reason, 400));
            }
            found = true;
            if k == "phrase" {
                if let Some(e) = v.as_object().and_then(|m| crate::suggest::phrase::check_body(name, m, raw)) {
                    return Some(e);
                }
            }
            if k == "completion" {
                if let Some(size) = v.get("size") {
                    if to_f64(size).map(|s| s <= 0.0).unwrap_or(false) {
                        let reason = format!(
                            "{}[completion] failed to parse field [size]",
                            pos_prefix(raw, &["suggest", name, k, "size"])
                        );
                        let mut e = ApiError::new("x_content_parse_exception", &reason, 400);
                        e.caused_by =
                            Some(Box::new(ApiError::new("illegal_argument_exception", "size must be positive", 400)));
                        return Some(e);
                    }
                }
            }
        }
        if !found {
            return Some(ApiError::new("parse_exception", "missing suggestion object", 400));
        }
    }
    None
}

// ------------------------------------------------------------- mappings

// ------------------------------------------------------------- geohash

// ------------------------------------------------------------ indexing

// --------------------------------------------------------------- analysis

/// The analyzed form of a text as the completion field indexes it: tokens
/// joined by separators, holes kept as extra separators.
fn analyzed_key(an: &crate::analysis::Analyzer, text: &str, preserve_sep: bool, preserve_pos: bool) -> Vec<char> {
    let stream = an.run(text);
    let mut out = vec![];
    let mut prev: i64 = -1;
    for (i, t) in stream.tokens.iter().enumerate() {
        let inc = if preserve_pos { (t.pos as i64 - prev).max(if i == 0 { 1 } else { 0 }) } else { 1 };
        if inc == 0 && i > 0 {
            continue;
        }
        let seps = if i == 0 { inc - 1 } else { inc };
        if preserve_sep {
            for _ in 0..seps {
                out.push(SEP);
            }
        }
        out.extend(t.term.chars());
        prev = t.pos as i64;
    }
    out
}

fn truncate_utf16(s: &str, max: usize) -> String {
    let mut n = 0;
    let mut out = String::new();
    for c in s.chars() {
        n += c.len_utf16();
        if n > max {
            break;
        }
        out.push(c);
    }
    out
}

// ------------------------------------------------------------- matching

struct Fuzzy {
    edits: usize,
    transpositions: bool,
    min_length: usize,
    prefix_length: usize,
    unicode_aware: bool,
}

fn parse_fuzziness(v: Option<&Value>, text: &str) -> usize {
    let auto = |lo: usize, hi: usize| {
        let n = text.chars().count();
        if n < lo {
            0
        } else if n < hi {
            1
        } else {
            2
        }
    };
    match v {
        None => auto(3, 6),
        Some(Value::String(s)) => {
            let u = s.to_uppercase();
            if u == "AUTO" {
                auto(3, 6)
            } else if let Some(rest) = u.strip_prefix("AUTO:") {
                let mut it = rest.split(',');
                let lo = it.next().and_then(|x| x.trim().parse().ok()).unwrap_or(3);
                let hi = it.next().and_then(|x| x.trim().parse().ok()).unwrap_or(6);
                auto(lo, hi)
            } else {
                s.trim().parse::<f64>().map(|f| (f as usize).min(2)).unwrap_or(0)
            }
        }
        Some(other) => to_f64(other).map(|f| (f as usize).min(2)).unwrap_or(0),
    }
}

fn parse_fuzzy(body: &Map<String, Value>, text: &str) -> Option<Fuzzy> {
    let f = body.get("fuzzy")?;
    let empty = Map::new();
    let m = match f {
        Value::Bool(true) => &empty,
        Value::Object(m) => m,
        _ => return None,
    };
    let num = |keys: &[&str], d: usize| keys.iter().find_map(|k| m.get(*k).and_then(to_f64)).map(|x| x as usize).unwrap_or(d);
    let flag = |k: &str, d: bool| m.get(k).map(|v| v.as_bool().unwrap_or(v.as_str() == Some("true"))).unwrap_or(d);
    Some(Fuzzy {
        edits: parse_fuzziness(m.get("fuzziness"), text),
        transpositions: flag("transpositions", true),
        min_length: num(&["min_length", "fuzzy_min_length"], 3),
        prefix_length: num(&["prefix_length", "fuzzy_prefix_length"], 1),
        unicode_aware: flag("unicode_aware", false),
    })
}

fn units(s: &[char], unicode: bool) -> Vec<u32> {
    if unicode {
        s.iter().map(|c| *c as u32).collect()
    } else {
        let st: String = s.iter().collect();
        st.bytes().map(|b| b as u32).collect()
    }
}

/// Edit distance between `q` and every prefix of `k`: returns the last DP row.
fn prefix_distances(q: &[u32], k: &[u32], transpositions: bool) -> Vec<usize> {
    let n = q.len();
    let m = k.len();
    // d[i][j]: distance between q[..i] and k[..j]
    let mut d = vec![vec![0usize; m + 1]; n + 1];
    for (i, row) in d.iter_mut().enumerate() {
        row[0] = i;
    }
    for j in 0..=m {
        d[0][j] = j;
    }
    for i in 1..=n {
        for j in 1..=m {
            let cost = if q[i - 1] == k[j - 1] { 0 } else { 1 };
            let mut v = (d[i - 1][j] + 1).min(d[i][j - 1] + 1).min(d[i - 1][j - 1] + cost);
            if transpositions && i > 1 && j > 1 && q[i - 1] == k[j - 2] && q[i - 2] == k[j - 1] {
                v = v.min(d[i - 2][j - 2] + 1);
            }
            d[i][j] = v;
        }
    }
    d.pop().unwrap_or_default()
}

/// Returns the fuzzy boost (common prefix length) when `key` matches.
fn fuzzy_match(q: &[u32], key: &[u32], f: &Fuzzy) -> Option<f32> {
    let exact = q.len() <= f.prefix_length || q.len() < f.min_length;
    let edits = if exact { 0 } else { f.edits };
    let p = if exact { q.len() } else { f.prefix_length };
    if key.len() < p || key[..p] != q[..p] {
        return None;
    }
    let row = prefix_distances(&q[p..], &key[p..], f.transpositions);
    if !row.iter().any(|d| *d <= edits) {
        return None;
    }
    // FuzzyCompletionWeight boosts by the exact prefix shared with the
    // query; the last letter of the query never counts.
    let common = q.iter().zip(key.iter()).take_while(|(a, b)| a == b).count();
    Some(common.min(q.len().saturating_sub(1)) as f32)
}

/// Lucene RegExp syntax (default flags) to a Rust regex.
fn lucene_regex(re: &str) -> String {
    let mut out = String::new();
    let chars: Vec<char> = re.chars().collect();
    let mut i = 0;
    let mut in_class = false;
    while i < chars.len() {
        let c = chars[i];
        if c == '\\' && i + 1 < chars.len() {
            out.push_str(&regex::escape(&chars[i + 1].to_string()));
            i += 2;
            continue;
        }
        if in_class {
            if c == ']' {
                in_class = false;
            }
            out.push(c);
            i += 1;
            continue;
        }
        match c {
            '[' => {
                in_class = true;
                out.push(c);
            }
            '@' => out.push_str("(?s:.*)"),
            '#' => out.push_str("(?:)"),
            '"' => {
                let mut lit = String::new();
                i += 1;
                while i < chars.len() && chars[i] != '"' {
                    lit.push(chars[i]);
                    i += 1;
                }
                out.push_str(&regex::escape(&lit));
            }
            '.' => out.push_str("(?s:.)"),
            _ => out.push(c),
        }
        i += 1;
    }
    out
}

// --------------------------------------------------------- query contexts

struct QueryCtx {
    mapping: usize,
    value: String,
    prefix: bool,
    boost: f32,
}

fn shard_err(index: &str, reason: &str) -> ApiError {
    let o = json!({"type": "illegal_argument_exception", "reason": reason});
    crate::search::script_ctx::shard_failure(index, 400, o.clone(), o.clone(), o, "query")
}

fn parse_query_contexts(maps: &[CtxMapping], q: &Map<String, Value>, index: &str) -> Result<Vec<QueryCtx>, ApiError> {
    let mut out = vec![];
    for (name, v) in q {
        let Some(mi) = maps.iter().position(|m| &m.name == name) else {
            return Err(shard_err(index, &unknown_ctx_reason(maps, name)));
        };
        let cm = &maps[mi];
        let list: Vec<&Value> = match v {
            Value::Array(a) if !(cm.typ == CtxType::Geo && a.first().map(|x| x.is_number()).unwrap_or(false)) => {
                a.iter().collect()
            }
            other => vec![other],
        };
        for item in list {
            match cm.typ {
                CtxType::Category => {
                    let (value, prefix, boost) = match item {
                        Value::Object(o) => (
                            o.get("context").and_then(value_to_string).unwrap_or_default(),
                            o.get("prefix").and_then(|v| v.as_bool()).unwrap_or(false),
                            o.get("boost").and_then(to_f64).unwrap_or(1.0) as f32,
                        ),
                        other => (value_to_string(other).unwrap_or_default(), false, 1.0),
                    };
                    out.push(QueryCtx { mapping: mi, value, prefix, boost });
                }
                CtxType::Geo => {
                    let (point, precision, boost, neighbours): (Option<(f64, f64)>, usize, f32, Vec<usize>) = match item {
                        Value::Object(o) if o.contains_key("context") => (
                            o.get("context").and_then(parse_point),
                            o.get("precision").and_then(geo_precision).unwrap_or(12),
                            o.get("boost").and_then(to_f64).unwrap_or(1.0) as f32,
                            o.get("neighbours")
                                .and_then(|v| v.as_array())
                                .map(|a| a.iter().filter_map(geo_precision).collect())
                                .unwrap_or_default(),
                        ),
                        other => (parse_point(other), 12, 1.0, vec![]),
                    };
                    let Some((lat, lon)) = point else { continue };
                    let min_p = cm.precision.min(precision);
                    let gh = geohash_encode(lat, lon, min_p);
                    let mut locs = vec![gh.clone()];
                    if neighbours.is_empty() && gh.len() == cm.precision {
                        locs.extend(geohash_neighbors(&gh));
                    } else {
                        for n in neighbours {
                            if n < gh.len() {
                                let t: String = gh.chars().take(n).collect();
                                locs.push(t.clone());
                                locs.extend(geohash_neighbors(&t));
                            }
                        }
                    }
                    for l in locs {
                        let prefix = l.len() < cm.precision;
                        out.push(QueryCtx { mapping: mi, value: l, prefix, boost });
                    }
                }
            }
        }
    }
    Ok(out)
}

// ------------------------------------------------------------- suggester

struct Hit {
    text: String,
    score: f32,
    index_ord: usize,
    doc_ord: usize,
    index: String,
    id: String,
    contexts: Vec<(usize, String)>,
}

fn score_of(weight: i64, boost: f32) -> f32 {
    let w = weight as f32;
    if boost == 0.0 {
        w
    } else if w == 0.0 {
        boost
    } else {
        w * boost
    }
}

pub fn f32_value(x: f32) -> Value {
    format!("{:?}", x).parse::<f64>().map(|f| json!(f)).unwrap_or(json!(0))
}

pub fn completion_suggest(
    spec: &Map<String, Value>,
    body: &Map<String, Value>,
    indices: &[&Index],
    global_text: Option<&str>,
    search_body: &Map<String, Value>,
) -> Result<Value, ApiError> {
    let field = match body.get("field").and_then(value_to_string) {
        Some(f) => f,
        None => return Err(ApiError::new("parse_exception", "the required field option [field] is missing", 400)),
    };
    let regex_mode = spec.contains_key("regex");
    let text = spec
        .get("regex")
        .or_else(|| spec.get("prefix"))
        .or_else(|| spec.get("text"))
        .and_then(value_to_string)
        .or_else(|| global_text.map(|s| s.to_string()));
    let size = body.get("size").and_then(to_f64).map(|s| s as usize).unwrap_or(5);
    let skip_dups = body.get("skip_duplicates").and_then(|v| v.as_bool()).unwrap_or(false);
    let query_contexts = body.get("contexts").and_then(|v| v.as_object());
    let mut hits: Vec<Hit> = vec![];
    for (ii, idx) in indices.iter().enumerate() {
        let def = match crate::mapping::mapping_field_def(idx, &field) {
            Some(d) => d.clone(),
            None => return Err(shard_err(&idx.name, &format!("no mapping found for field [{}]", field))),
        };
        if def.get("type").and_then(|v| v.as_str()) != Some("completion") {
            return Err(shard_err(&idx.name, &format!("Field [{}] is not a completion suggest field", field)));
        }
        let Some(text) = text.clone() else {
            return Err(shard_err(&idx.name, "The required text option is missing"));
        };
        let maps = context_mappings(&def);
        let qctx = match query_contexts {
            Some(q) if !q.is_empty() => {
                if maps.is_empty() {
                    return Err(shard_err(&idx.name, &format!("suggester [{}] doesn't expect any context", field)));
                }
                parse_query_contexts(&maps, q, &idx.name)?
            }
            _ => {
                if !maps.is_empty() {
                    return Err(shard_err(&idx.name, "Missing mandatory contexts in context query"));
                }
                vec![]
            }
        };
        let ctx = crate::analysis::Ctx::for_settings(&idx.settings);
        let index_an_name = def.get("analyzer").and_then(|v| v.as_str()).unwrap_or("simple").to_string();
        let search_an_name =
            def.get("search_analyzer").and_then(|v| v.as_str()).map(|s| s.to_string()).unwrap_or(index_an_name.clone());
        let (Ok(Some(ia)), Ok(Some(sa))) = (ctx.analyzer(&index_an_name), ctx.analyzer(&search_an_name)) else {
            continue;
        };
        let preserve_sep = def.get("preserve_separators").and_then(|v| v.as_bool()).unwrap_or(true);
        let preserve_pos = def.get("preserve_position_increments").and_then(|v| v.as_bool()).unwrap_or(true);
        let max_len = def.get("max_input_length").and_then(|v| v.as_i64()).unwrap_or(50).max(1) as usize;
        if text.is_empty() {
            continue;
        }
        enum Mode {
            Prefix(Vec<char>),
            Fuzzy(Vec<u32>, Fuzzy),
            Regex(regex::Regex),
        }
        let mode = if regex_mode {
            match regex::Regex::new(&format!("^(?:{})", lucene_regex(&text))) {
                Ok(r) => Mode::Regex(r),
                Err(_) => continue,
            }
        } else {
            let qkey = analyzed_key(&sa, &text, preserve_sep, preserve_pos);
            if qkey.is_empty() {
                continue;
            }
            match parse_fuzzy(body, &text) {
                Some(f) => Mode::Fuzzy(units(&qkey, f.unicode_aware), f),
                None => Mode::Prefix(qkey),
            }
        };
        for (di, id) in idx.order.iter().enumerate() {
            if idx.pending.contains_key(id) {
                continue;
            }
            let Some(doc) = idx.docs.get(id) else { continue };
            let Some(v) = field_value(doc, &field) else { continue };
            let Ok(inputs) = collect_inputs(&field, &def, v, doc) else { continue };
            // (score, text, key, contexts matched)
            let mut paths: Vec<(f32, String, Vec<char>, Option<(usize, String)>)> = vec![];
            for inp in inputs {
                let surface = truncate_utf16(&inp.text, max_len);
                let key = analyzed_key(&ia, &surface, preserve_sep, preserve_pos);
                let inner = match &mode {
                    Mode::Prefix(q) => {
                        if key.len() >= q.len() && key[..q.len()] == q[..] {
                            Some(0.0f32)
                        } else {
                            None
                        }
                    }
                    Mode::Fuzzy(q, f) => fuzzy_match(q, &units(&key, f.unicode_aware), f),
                    Mode::Regex(r) => {
                        let ks: String = key.iter().collect();
                        if r.is_match(&ks) {
                            Some(0.0)
                        } else {
                            None
                        }
                    }
                };
                let Some(inner) = inner else { continue };
                if maps.is_empty() {
                    paths.push((score_of(inp.weight, inner), surface, key, None));
                    continue;
                }
                for c in &inp.contexts {
                    let mut boost: Option<f32> = None;
                    for q in &qctx {
                        if q.mapping == c.0 && (c.1 == q.value || (q.prefix && c.1.starts_with(&q.value))) {
                            boost = Some(boost.map_or(q.boost, |b: f32| b.max(q.boost)));
                        }
                    }
                    if let Some(b) = boost {
                        paths.push((score_of(inp.weight, b + inner), surface.clone(), key.clone(), Some(c.clone())));
                    }
                }
            }
            if paths.is_empty() {
                continue;
            }
            paths.sort_by(|a, b| b.0.partial_cmp(&a.0).unwrap_or(std::cmp::Ordering::Equal).then(a.2.cmp(&b.2)));
            let mut contexts: Vec<(usize, String)> = vec![];
            for p in &paths {
                if let Some(c) = &p.3 {
                    if !contexts.contains(c) {
                        contexts.push(c.clone());
                    }
                }
            }
            hits.push(Hit {
                text: paths[0].1.clone(),
                score: paths[0].0,
                index_ord: ii,
                doc_ord: di,
                index: idx.name.clone(),
                id: id.clone(),
                contexts,
            });
        }
    }
    hits.sort_by(|a, b| {
        b.score
            .partial_cmp(&a.score)
            .unwrap_or(std::cmp::Ordering::Equal)
            .then_with(|| a.text.encode_utf16().cmp(b.text.encode_utf16()))
            .then(a.index_ord.cmp(&b.index_ord))
            .then(a.doc_ord.cmp(&b.doc_ord))
    });
    if skip_dups {
        let mut seen = std::collections::HashSet::new();
        hits.retain(|h| seen.insert(h.text.clone()));
    }
    hits.truncate(size);
    let source_spec = search_body.get("_source");
    let mut options = vec![];
    for h in hits {
        let mut o = Map::new();
        o.insert("text".into(), json!(h.text));
        o.insert("_index".into(), json!(h.index));
        o.insert("_id".into(), json!(h.id));
        o.insert("_score".into(), f32_value(h.score));
        let idx = indices[h.index_ord];
        if let Some(doc) = idx.docs.get(&h.id) {
            match source_spec {
                Some(Value::Bool(false)) => {}
                None | Some(Value::Bool(true)) | Some(Value::Null) => {
                    o.insert("_source".into(), doc.clone());
                }
                Some(spec) => {
                    o.insert("_source".into(), crate::search::source::filter_source(doc, spec));
                }
            }
        }
        let def = crate::mapping::mapping_field_def(idx, &field).cloned().unwrap_or_default();
        let maps = context_mappings(&def);
        if !maps.is_empty() {
            o.insert("contexts".into(), named_contexts(&maps, &h.contexts));
        }
        options.push(Value::Object(o));
    }
    let text = text.unwrap_or_default();
    Ok(json!([{"text": text, "offset": 0, "length": text.encode_utf16().count(), "options": options}]))
}

/// ContextMappings.getNamedContexts: a HashMap of HashSets, in Java order.
fn named_contexts(maps: &[CtxMapping], ctxs: &[(usize, String)]) -> Value {
    let mut names: Vec<String> = vec![];
    let mut sets: Vec<Vec<String>> = vec![];
    for (mi, v) in ctxs {
        let name = &maps[*mi].name;
        let pos = match names.iter().position(|n| n == name) {
            Some(p) => p,
            None => {
                names.push(name.clone());
                sets.push(vec![]);
                names.len() - 1
            }
        };
        if !sets[pos].contains(v) {
            sets[pos].push(v.clone());
        }
    }
    let mut out = Map::new();
    for i in java_hash_order(&names, ctxs.len()) {
        let set = &sets[i];
        let vals: Vec<Value> = java_hash_order(set, 16).into_iter().map(|j| json!(set[j])).collect();
        out.insert(names[i].clone(), Value::Array(vals));
    }
    Value::Object(out)
}

/// A suggest-only search skips the query phase: no hits, total 0.
pub fn clear_hits(resp: &mut Map<String, Value>) {
    if let Some(Value::Object(h)) = resp.get_mut("hits") {
        if let Some(t) = h.get_mut("total") {
            if t.is_number() {
                *t = json!(0);
            } else {
                *t = json!({"value": 0, "relation": "eq"});
            }
        }
        if h.contains_key("max_score") {
            h.insert("max_score".into(), Value::Null);
        }
        h.insert("hits".into(), json!([]));
    }
}
