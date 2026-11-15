//! What searches see between refreshes, and the shard request cache.
//!
//! A document written without a refresh is not searchable until the next
//! refresh; when it replaces or deletes a document, searches keep seeing the
//! replaced (last refreshed) version with its doc values. Writes record that
//! version here ([`before_write`]); a search swaps it in for its duration
//! ([`enter`]/[`leave`]). Realtime reads (GET) see the current version.
//!
//! The request cache keeps the results of `size: 0` searches until the
//! searched indices change (OpenSearch's IndicesRequestCache is keyed by the
//! shard reader and the request; it is used for `size: 0` requests unless
//! `index.requests.cache.enable` is false, and the `request_cache` parameter
//! overrides both). Its observable effect is that a result does not change
//! while the index does not: a stored script replaced in the meantime is not
//! run again.

use super::{Index, Store};
use crate::settings::find_setting;
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
    routing: Option<String>,
}

const CACHE_MAX: usize = 256;

/// The request cache, shared by concurrent searches.
#[derive(Default)]
pub struct RequestCache(std::sync::Mutex<HashMap<String, (u64, Value)>>);

impl Clone for RequestCache {
    fn clone(&self) -> Self {
        RequestCache(std::sync::Mutex::new(self.0.lock().unwrap_or_else(|e| e.into_inner()).clone()))
    }
}

#[derive(Clone, Default)]
pub struct SearcherState {
    stale: HashMap<String, Stale>,
    cache: RequestCache,
    /// When the oldest unrefreshed write was first seen (see [`scheduled_refresh`]).
    pending_since: Option<std::time::Instant>,
}

/// `index.refresh_interval` in milliseconds; `None` when periodic refreshes are off.
fn refresh_interval_ms(idx: &Index) -> Option<u64> {
    let Some(v) = find_setting(&idx.settings, "refresh_interval") else { return Some(1000) };
    let s = crate::settings::settings_value_string(v);
    let s = s.trim();
    if s.starts_with('-') {
        return None;
    }
    let split = s.find(|c: char| !c.is_ascii_digit()).unwrap_or(s.len());
    let n: u64 = s[..split].parse().ok()?;
    let mult = match &s[split..] {
        "nanos" => return Some(n / 1_000_000),
        "micros" => return Some(n / 1000),
        "ms" | "" => 1,
        "s" => 1000,
        "m" => 60_000,
        "h" => 3_600_000,
        "d" => 86_400_000,
        _ => return Some(1000),
    };
    Some(n.saturating_mul(mult))
}

/// OpenSearch refreshes every shard each `index.refresh_interval` (1s by
/// default; a search-idle shard refreshes before it is searched), so a write
/// is searchable at the latest one interval after it happened. Called before
/// and after every request: the first call that sees unrefreshed writes starts
/// their clock, a later one past the interval refreshes the index.
pub fn scheduled_refresh(store: &mut Store) {
    let now = std::time::Instant::now();
    for idx in store.indices.values_mut() {
        if idx.pending.is_empty() && idx.searcher.stale.is_empty() {
            idx.searcher.pending_since = None;
            continue;
        }
        let Some(since) = idx.searcher.pending_since else {
            idx.searcher.pending_since = Some(now);
            continue;
        };
        if idx.closed {
            continue;
        }
        if let Some(ms) = refresh_interval_ms(idx) {
            if now.duration_since(since).as_millis() as u64 >= ms {
                refresh(idx);
                idx.searcher.pending_since = None;
            }
        }
    }
}

/// Whether nothing about the index waits for a refresh: a search swaps
/// nothing in and a scheduled refresh changes nothing.
pub fn quiescent(idx: &Index) -> bool {
    idx.pending.is_empty() && idx.searcher.stale.is_empty() && idx.searcher.pending_since.is_none()
}

/// A refresh: every write becomes searchable.
pub fn refresh(idx: &mut Index) {
    idx.pending.clear();
    idx.searcher.stale.clear();
}

/// Whether searches see the remembered version of `id`: the document was
/// rewritten (pending) or deleted since the last refresh.
fn stale_visible(idx: &Index, id: &str) -> bool {
    idx.searcher.stale.contains_key(id) && (idx.pending.contains_key(id) || !idx.docs.contains_key(id))
}

/// Call before a write replaces or deletes the document `id` (before its doc
/// values, version and seqNo change): remembers the version searches see
/// until the next refresh.
pub fn before_write(idx: &mut Index, id: &str) {
    if idx.pending.contains_key(id) || stale_visible(idx, id) {
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
                routing: idx.routing.get(id).cloned(),
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
    let cur = idx.docs.remove(id);
    if let Some(v) = st.doc.take() {
        idx.docs.insert(id.to_string(), v);
    }
    st.doc = cur;
    let cur = idx.doc_values.remove(id);
    if let Some(v) = st.dv.take() {
        idx.doc_values.insert(id.to_string(), v);
    }
    st.dv = cur;
    swap_slot(&mut idx.doc_ignored, id, &mut st.ignored);
    swap_slot(&mut idx.raw_sources, id, &mut st.raw);
    swap_slot(&mut idx.versions, id, &mut st.version);
    swap_slot(&mut idx.seq_nos, id, &mut st.seq);
    swap_slot(&mut idx.routing, id, &mut st.routing);
}

/// The swapped-in documents of a search, restored by [`leave`].
pub struct View {
    swapped: Vec<(String, String, Stale, Option<Value>)>,
}

/// Makes the last refreshed versions of unrefreshed documents searchable.
pub fn enter(store: &mut Store, names: &[String]) -> View {
    let mut swapped = Vec::new();
    for n in names {
        let Some(idx) = store.indices.get_mut(n) else { continue };
        if idx.searcher.stale.is_empty() {
            continue;
        }
        let ids: Vec<String> = idx
            .searcher
            .stale
            .keys()
            .filter(|id| idx.pending.contains_key(*id) || !idx.docs.contains_key(*id))
            .cloned()
            .collect();
        for id in ids {
            let mut st = idx.searcher.stale.remove(&id).unwrap_or_default();
            swap_doc(idx, &id, &mut st);
            let p = idx.pending.remove(&id);
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
        if let Some(p) = p {
            idx.pending.insert(id.clone(), p);
        }
        idx.searcher.stale.insert(id, st);
    }
}

/// Adds a new document at the end of the index order. A document created
/// again after a delete moves to the end (it is in a new segment).
pub fn push_order(idx: &mut Index, id: &str) {
    if idx.versions.contains_key(id) {
        if let Some(pos) = idx.order.iter().position(|o| o == id) {
            idx.order.remove(pos);
        }
    }
    idx.order.push(id.to_string());
}

/// Runs `f` on the last refreshed versions of every index (a search-like read).
pub fn with_view<R>(store: &mut Store, f: impl FnOnce(&mut Store) -> R) -> R {
    let names: Vec<String> = store.indices.keys().cloned().collect();
    let view = enter(store, &names);
    let r = f(store);
    leave(store, view);
    r
}

/// The `_seq_no` searches see of `id` and the current one (None: deleted):
/// the version conflict of writing back a search hit of a by-query request.
pub fn conflict(idx: &Index, id: &str, seen: i64) -> Option<Value> {
    let reason = match (idx.docs.contains_key(id), idx.seq_nos.get(id)) {
        (true, Some(have)) if *have == seen => return None,
        (true, Some(have)) => format!(
            "[{}]: version conflict, required seqNo [{}], primary term [1]. current document has seqNo [{}] and primary term [1]",
            id, seen, have
        ),
        (true, None) => return None,
        (false, _) => format!(
            "[{}]: version conflict, required seqNo [{}], primary term [1]. but no document was found",
            id, seen
        ),
    };
    Some(serde_json::json!({"index": idx.name, "id": id, "cause": {
        "type": "version_conflict_engine_exception", "reason": reason,
        "index": idx.name, "shard": "0", "index_uuid": idx.uuid.clone()}, "status": 409}))
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
        idx.searcher.stale.len(),
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
pub fn cache_key(store: &Store, path: &str, query: &[(String, String)], names: &[String], body: &Value) -> Option<String> {
    // a repeated parameter: the last value wins
    let param = |k: &str| query.iter().rev().find(|(qk, _)| qk == k).map(|(_, v)| v.as_str());
    if names.is_empty() || param("scroll").is_some() {
        return None;
    }
    if matches!(param("search_type"), Some(t) if t != "query_then_fetch") {
        return None;
    }
    let size = param("size")
        .and_then(|s| s.parse::<i64>().ok())
        .or_else(|| body.get("size").and_then(|v| v.as_i64()));
    match param("request_cache") {
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
    if uncacheable(body) || query.iter().any(|(k, v)| k == "q" && v.contains("now")) {
        return None;
    }
    let mut params: Vec<&(String, String)> = query.iter().filter(|(k, _)| k != "request_cache").collect();
    params.sort();
    Some(format!("{}|{:?}|{}|{}", path, params, names.join(","), body))
}

pub fn cache_get(store: &Store, names: &[String], key: &str) -> Option<Value> {
    let fp = indices_fingerprint(store, names)?;
    let idx = store.indices.get(names.first()?)?;
    match idx.searcher.cache.0.lock().unwrap_or_else(|e| e.into_inner()).get(key) {
        Some((f, v)) if *f == fp => Some(v.clone()),
        _ => None,
    }
}

pub fn cache_put(store: &Store, names: &[String], key: String, v: &Value) {
    let Some(fp) = indices_fingerprint(store, names) else { return };
    let Some(idx) = names.first().and_then(|n| store.indices.get(n)) else { return };
    let mut cache = idx.searcher.cache.0.lock().unwrap_or_else(|e| e.into_inner());
    cache.retain(|_, (f, _)| *f == fp);
    if cache.len() >= CACHE_MAX {
        cache.clear();
    }
    cache.insert(key, (fp, v.clone()));
}
