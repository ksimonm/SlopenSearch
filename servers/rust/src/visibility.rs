//! What searches see between refreshes, and the shard request cache.
//!
//! A document written without a refresh is not searchable until the next
//! refresh; when it replaces a document, searches keep seeing the replaced
//! (last refreshed) version with its doc values. Writes record that version
//! here ([`before_write`]); a search swaps it in for its duration
//! ([`enter`]/[`leave`]).
//!
//! The request cache keeps the results of `size: 0` searches until the
//! searched indices change (OpenSearch's IndicesRequestCache is keyed by the
//! shard reader and the request; it is used for `size: 0` requests unless
//! `index.requests.cache.enable` is false, and the `request_cache` parameter
//! overrides both). Its observable effect is that a result does not change
//! while the index does not: a stored script replaced in the meantime is not
//! run again.

use crate::Req;
use crate::store::{find_setting, Index, Store};
use serde_json::{Map, Value};
use std::collections::hash_map::DefaultHasher;
use std::collections::HashMap;
use std::hash::{Hash, Hasher};

/// The last refreshed version of a document that has a newer, unrefreshed one.
#[derive(Clone, Default)]
struct Stale {
    doc: Option<Value>,
    dv: Option<Map<String, Value>>,
    ignored: Option<Vec<String>>,
    raw: Option<Vec<u8>>,
    version: Option<i64>,
    seq: Option<i64>,
}

const CACHE_MAX: usize = 256;

#[derive(Clone, Default)]
pub struct SearcherState {
    stale: HashMap<String, Stale>,
    cache: HashMap<String, (u64, Value)>,
}

/// Call before a write replaces the document `id` (before its doc values are
/// recomputed): remembers the version searches see until the next refresh.
pub fn before_write(idx: &mut Index, id: &str) {
    if idx.pending.contains_key(id) {
        // Already unrefreshed: the remembered version (if any) stays.
        return;
    }
    match idx.docs.get(id) {
        Some(doc) => {
            let st = Stale {
                doc: Some(doc.clone()),
                dv: idx.doc_values.get(id).cloned(),
                ignored: idx.doc_ignored.get(id).cloned(),
                raw: idx.raw_sources.get(id).cloned(),
                version: idx.versions.get(id).copied(),
                seq: idx.seq_nos.get(id).copied(),
            };
            idx.searcher.stale.insert(id.to_string(), st);
        }
        None => {
            idx.searcher.stale.remove(id);
        }
    }
}

fn swap_slot<V>(map: &mut HashMap<String, V>, id: &str, slot: &mut Option<V>) {
    let cur = map.remove(id);
    if let Some(v) = slot.take() {
        map.insert(id.to_string(), v);
    }
    *slot = cur;
}

fn swap_doc(idx: &mut Index, id: &str, st: &mut Stale) {
    swap_slot(&mut idx.docs, id, &mut st.doc);
    swap_slot(&mut idx.doc_values, id, &mut st.dv);
    swap_slot(&mut idx.doc_ignored, id, &mut st.ignored);
    swap_slot(&mut idx.raw_sources, id, &mut st.raw);
    swap_slot(&mut idx.versions, id, &mut st.version);
    swap_slot(&mut idx.seq_nos, id, &mut st.seq);
}

/// The swapped-in documents of a search, restored by [`leave`].
pub struct View {
    swapped: Vec<(String, String, Stale, Value)>,
}

/// Makes the last refreshed versions of unrefreshed documents searchable.
pub fn enter(store: &mut Store, names: &[String]) -> View {
    let mut swapped = Vec::new();
    for n in names {
        let Some(idx) = store.indices.get_mut(n) else { continue };
        if idx.pending.is_empty() || idx.searcher.stale.is_empty() {
            continue;
        }
        let ids: Vec<String> = idx
            .searcher
            .stale
            .keys()
            .filter(|id| idx.pending.contains_key(*id) && idx.docs.contains_key(*id))
            .cloned()
            .collect();
        for id in ids {
            let mut st = idx.searcher.stale.remove(&id).unwrap_or_default();
            swap_doc(idx, &id, &mut st);
            let p = idx.pending.remove(&id).unwrap_or(Value::Null);
            swapped.push((n.clone(), id, st, p));
        }
    }
    View { swapped }
}

/// Restores the current versions after [`enter`].
pub fn leave(store: &mut Store, view: View) {
    for (n, id, mut st, p) in view.swapped {
        let Some(idx) = store.indices.get_mut(&n) else { continue };
        swap_doc(idx, &id, &mut st);
        idx.pending.insert(id.clone(), p);
        idx.searcher.stale.insert(id, st);
    }
}

// ------------------------------------------------------------ request cache

fn hash_of(v: &impl Hash) -> u64 {
    let mut h = DefaultHasher::new();
    v.hash(&mut h);
    h.finish()
}

/// Changes whenever what a search of the index can see changes (more often
/// than a Lucene reader: any write counts).
fn fingerprint(idx: &Index) -> u64 {
    hash_of(&(
        &idx.uuid,
        idx.index_total,
        idx.delete_total,
        idx.docs.len(),
        idx.pending.len(),
        idx.mapping_version,
        idx.seg_gen,
        idx.closed,
        serde_json::to_string(&idx.settings).unwrap_or_default(),
        serde_json::to_string(&idx.mappings).unwrap_or_default(),
        serde_json::to_string(&idx.aliases).unwrap_or_default(),
    ))
}

fn indices_fingerprint(store: &Store, names: &[String]) -> Option<u64> {
    let mut parts = Vec::with_capacity(names.len());
    for n in names {
        parts.push((n.as_str(), fingerprint(store.indices.get(n)?)));
    }
    let cluster = (
        serde_json::to_string(&store.cluster_persistent).unwrap_or_default(),
        serde_json::to_string(&store.cluster_transient).unwrap_or_default(),
    );
    Some(hash_of(&(parts, cluster)))
}

/// Whether a request depends on more than the index content and itself:
/// the current time, randomness, or documents of other indices.
fn uncacheable(v: &Value) -> bool {
    match v {
        Value::String(s) => s.contains("now") || s.contains("random"),
        Value::Array(a) => a.iter().any(uncacheable),
        Value::Object(m) => m.iter().any(|(k, x)| {
            matches!(k.as_str(), "index" | "random_score" | "pit" | "profile" | "_index") || uncacheable(x)
        }),
        _ => false,
    }
}

/// The cache key of a search request, when its result may be cached.
pub fn cache_key(store: &Store, req: &Req, names: &[String], body: &Value) -> Option<String> {
    if names.is_empty() || req.param("scroll").is_some() {
        return None;
    }
    if matches!(req.param("search_type"), Some(t) if t != "query_then_fetch") {
        return None;
    }
    let size = req
        .param("size")
        .and_then(|s| s.parse::<i64>().ok())
        .or_else(|| body.get("size").and_then(|v| v.as_i64()));
    match req.param("request_cache") {
        Some("false") => return None,
        Some("true") => {}
        _ => {
            for n in names {
                let idx = store.indices.get(n)?;
                let enabled = find_setting(&idx.settings, "requests.cache.enable")
                    .map(|v| v.as_bool().unwrap_or_else(|| v.as_str() != Some("false")))
                    .unwrap_or(true);
                if !enabled {
                    return None;
                }
            }
        }
    }
    // Hits are fetched for every request: only results without hits are kept.
    if size != Some(0) {
        return None;
    }
    if uncacheable(body) || req.query.iter().any(|(k, v)| k == "q" && v.contains("now")) {
        return None;
    }
    let mut params: Vec<&(String, String)> = req.query.iter().filter(|(k, _)| k != "request_cache").collect();
    params.sort();
    Some(format!("{}|{:?}|{}|{}", req.path, params, names.join(","), body))
}

pub fn cache_get(store: &Store, names: &[String], key: &str) -> Option<Value> {
    let fp = indices_fingerprint(store, names)?;
    let idx = store.indices.get(names.first()?)?;
    match idx.searcher.cache.get(key) {
        Some((f, v)) if *f == fp => Some(v.clone()),
        _ => None,
    }
}

pub fn cache_put(store: &mut Store, names: &[String], key: String, v: &Value) {
    let Some(fp) = indices_fingerprint(store, names) else { return };
    let Some(idx) = names.first().and_then(|n| store.indices.get_mut(n)) else { return };
    let cache = &mut idx.searcher.cache;
    cache.retain(|_, (f, _)| *f == fp);
    if cache.len() >= CACHE_MAX {
        cache.clear();
    }
    cache.insert(key, (fp, v.clone()));
}
