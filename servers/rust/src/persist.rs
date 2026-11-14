//! Durable state. By default the server persists to `./data` (relative to the working
//! directory); `SLOP_DATA_DIR`, `--data-dir <dir>` or `-Epath.data=<dir>` choose another
//! directory, and `SLOP_IN_MEMORY=true` (or `--in-memory`) keeps everything in memory only.
//!
//! Layout of the data directory (file names carry a `rust-` prefix so that the Go server
//! can share a directory name without clashing; `rust.lock` keeps a second Rust server
//! off the same directory):
//! - `rust-state.json`: an atomic snapshot (write to a temp file, fsync, rename, fsync dir) of
//!   the durable cluster state, tagged with a generation number `G`;
//! - `rust-journal-G.log`: the write-ahead journal on top of that snapshot. After every
//!   request that changed durable state, one line holding all of the request's changes
//!   is appended and fsynced before the response leaves the server, so acknowledged
//!   writes survive a crash (`kill -9`). A torn last line (crash mid-write) belongs to a
//!   request that was never acknowledged and is dropped on load.
//!
//! On startup the snapshot is loaded, its journal replayed, and a fresh snapshot of
//! generation `G+1` written (the journal is also compacted that way when it grows
//! large). Durable: indices (settings, mappings, aliases, open/closed state, documents
//! with versions, seq_nos, routing and exact `_source` bytes, delete tombstones), index,
//! component and legacy templates, data streams, ingest and search pipelines, stored
//! scripts and persistent cluster settings. Not durable, as in OpenSearch: transient
//! cluster settings, scroll contexts and PITs. Documents that were not yet refreshed are
//! searchable after a restart (recovery refreshes the shard).
//!
//! Changes are detected after each request by comparing the store with what was last
//! persisted: global metadata and per-index metadata by their serialized form, and
//! documents by their seq_no (every write assigns `seq_no = index_total` and bumps it).
//! When the number of documents with a new seq_no does not account for the growth of
//! `index_total` (writes that did not record a seq_no, or one id written twice in one
//! request) the whole index is written out again, which is always correct.

use crate::store::{DataStreamInfo, Index, Store};
use serde_json::{json, Map, Value};
use std::collections::{HashMap, HashSet};
use std::fs::{self, File, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::{Mutex, OnceLock};

const SNAPSHOT: &str = "rust-state.json";
const LOCK: &str = "rust.lock";
/// Compact the journal into a new snapshot once it grows past this size.
const COMPACT_BYTES: u64 = 64 << 20;

struct Tracked {
    uuid: String,
    meta: String,
    counters: [i64; 4],
}

struct Persister {
    dir: PathBuf,
    gen: u64,
    journal: File,
    journal_bytes: u64,
    global: String,
    indices: HashMap<String, Tracked>,
    /// Held (locked) for the lifetime of the process.
    _lock: File,
}

static PERSISTER: OnceLock<Mutex<Persister>> = OnceLock::new();

fn truthy(v: &str) -> bool {
    matches!(v.trim().to_ascii_lowercase().as_str(), "true" | "1" | "yes" | "on")
}

/// The data directory, or `None` in in-memory mode.
fn data_dir() -> Option<PathBuf> {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let mut dir = None;
    let mut i = 0;
    while i < args.len() {
        let a = &args[i];
        if a == "--in-memory" {
            return None;
        }
        if let Some(v) = a.strip_prefix("--data-dir=").or_else(|| a.strip_prefix("-Epath.data=")) {
            dir = Some(PathBuf::from(v));
        } else if a == "--data-dir" && i + 1 < args.len() {
            dir = Some(PathBuf::from(&args[i + 1]));
            i += 1;
        }
        i += 1;
    }
    if std::env::var("SLOP_IN_MEMORY").map(|v| truthy(&v)).unwrap_or(false) {
        return None;
    }
    dir.or_else(|| match std::env::var("SLOP_DATA_DIR") {
        Ok(v) if !v.is_empty() => Some(PathBuf::from(v)),
        _ => None,
    })
    .or_else(|| Some(PathBuf::from("data")))
}

/// Builds the store: empty in in-memory mode, else recovered from the data directory
/// (created when missing).
pub fn open() -> Store {
    let dir = match data_dir() {
        Some(d) => d,
        None => return Store::new(),
    };
    match recover(&dir) {
        Ok(store) => {
            println!("slopensearch (rust): data directory {}", dir.display());
            store
        }
        Err(e) => {
            eprintln!("slopensearch (rust): cannot use data directory {}: {}", dir.display(), e);
            std::process::exit(1);
        }
    }
}

fn recover(dir: &Path) -> Result<Store, String> {
    fs::create_dir_all(dir).map_err(|e| e.to_string())?;
    let lock = OpenOptions::new()
        .create(true)
        .truncate(false)
        .write(true)
        .open(dir.join(LOCK))
        .map_err(|e| e.to_string())?;
    lock.try_lock()
        .map_err(|_| "it is in use by another slopensearch (rust) process".to_string())?;
    let mut store = Store::new();
    let mut gen = 0u64;
    let snap_path = dir.join(SNAPSHOT);
    if snap_path.exists() {
        let bytes = fs::read(&snap_path).map_err(|e| e.to_string())?;
        let snap: Value = serde_json::from_slice(&bytes).map_err(|e| format!("{}: {}", SNAPSHOT, e))?;
        gen = snap.get("gen").and_then(|v| v.as_u64()).unwrap_or(0);
        if let Some(g) = snap.get("global") {
            apply_global(&mut store, g);
        }
        for ix in snap.get("indices").and_then(|v| v.as_array()).into_iter().flatten() {
            apply_index(&mut store, ix);
            let name = ix.get("name").and_then(|v| v.as_str()).unwrap_or("").to_string();
            for d in ix.get("docs").and_then(|v| v.as_array()).into_iter().flatten() {
                apply_doc(&mut store, &name, d, false);
            }
        }
    }
    let journal_path = dir.join(journal_name(gen));
    if let Ok(bytes) = fs::read(&journal_path) {
        let mut start = 0;
        while let Some(off) = bytes[start..].iter().position(|&b| b == b'\n') {
            let line = &bytes[start..start + off];
            start += off + 1;
            match serde_json::from_slice::<Value>(line) {
                Ok(v) => {
                    for rec in v.get("recs").and_then(|r| r.as_array()).into_iter().flatten() {
                        apply_record(&mut store, rec);
                    }
                }
                // a torn write: the request was never acknowledged
                Err(_) => break,
            }
        }
    }
    for idx in store.indices.values_mut() {
        idx.ensure_field_types();
        crate::fields::recompute_all_doc_values(idx);
    }
    crate::script_ctx::sync_stored(&store.scripts);
    crate::store::reseed_uuid(crate::store::now_millis() as u64 ^ ((std::process::id() as u64) << 32));

    // start a new generation from the recovered state
    let journal = create_journal(dir, gen + 1)?;
    let mut p = Persister {
        dir: dir.to_path_buf(),
        gen: gen + 1,
        journal,
        journal_bytes: 0,
        global: String::new(),
        indices: HashMap::new(),
        _lock: lock,
    };
    p.write_snapshot(&store)?;
    remove_stale_journals(dir, p.gen);
    let _ = PERSISTER.set(Mutex::new(p));
    Ok(store)
}

fn journal_name(gen: u64) -> String {
    format!("rust-journal-{}.log", gen)
}

fn create_journal(dir: &Path, gen: u64) -> Result<File, String> {
    let f = OpenOptions::new()
        .create(true)
        .truncate(true)
        .write(true)
        .open(dir.join(journal_name(gen)))
        .map_err(|e| e.to_string())?;
    f.sync_all().map_err(|e| e.to_string())?;
    sync_dir(dir);
    Ok(f)
}

fn remove_stale_journals(dir: &Path, keep: u64) {
    let keep = journal_name(keep);
    if let Ok(entries) = fs::read_dir(dir) {
        for e in entries.flatten() {
            let name = e.file_name().to_string_lossy().to_string();
            if name.starts_with("rust-journal-") && name.ends_with(".log") && name != keep {
                let _ = fs::remove_file(e.path());
            }
        }
    }
}

fn sync_dir(dir: &Path) {
    if let Ok(d) = File::open(dir) {
        let _ = d.sync_all();
    }
}

/// Records what the request changed. Called with the store lock held, before the
/// response is sent.
pub fn after_request(store: &Store, method: &str, path: &str) {
    if method == "GET" || method == "HEAD" || read_only_post(path) {
        return;
    }
    let cell = match PERSISTER.get() {
        Some(c) => c,
        None => return,
    };
    let mut p = cell.lock().unwrap_or_else(|e| e.into_inner());
    let recs = p.diff(store);
    if recs.is_empty() {
        return;
    }
    let mut line = serde_json::to_vec(&json!({ "recs": recs })).unwrap_or_default();
    line.push(b'\n');
    let res = p.journal.write_all(&line).and_then(|_| p.journal.sync_data());
    if let Err(e) = res {
        eprintln!("slopensearch (rust): journal write failed: {}", e);
        return;
    }
    p.journal_bytes += line.len() as u64;
    if p.journal_bytes > COMPACT_BYTES {
        if let Err(e) = p.compact(store) {
            eprintln!("slopensearch (rust): journal compaction failed: {}", e);
        }
    }
}

/// Read APIs that are often sent as POST and never change durable state; skipping
/// them saves the comparison of the whole store.
fn read_only_post(path: &str) -> bool {
    let last = path.trim_end_matches('/').rsplit('/').next().unwrap_or("");
    let prev = path.trim_end_matches('/').rsplit('/').nth(1).unwrap_or("");
    matches!(
        last,
        "_search" | "_msearch" | "_count" | "_mget" | "_analyze" | "_field_caps" | "_validate"
            | "_termvectors" | "_mtermvectors" | "_search_shards" | "_rank_eval"
    ) || matches!(prev, "_explain" | "_termvectors" | "_validate" | "_render")
        || (prev == "_search" && last == "template")
        || (prev == "_msearch" && last == "template")
}

impl Persister {
    fn diff(&mut self, store: &Store) -> Vec<Value> {
        let mut recs = Vec::new();
        let global = global_json(store);
        let gs = global.to_string();
        if gs != self.global {
            recs.push(json!({"t": "global", "v": global}));
            self.global = gs;
        }
        let gone: Vec<String> = self
            .indices
            .keys()
            .filter(|n| !store.indices.contains_key(*n))
            .cloned()
            .collect();
        for name in gone {
            self.indices.remove(&name);
            recs.push(json!({"t": "drop", "name": name}));
        }
        let mut names: Vec<&String> = store.indices.keys().collect();
        names.sort();
        for name in names {
            let idx = &store.indices[name];
            let meta = index_meta(idx);
            let ms = meta.to_string();
            let counters = counters(idx);
            let full = match self.indices.get(name) {
                None => true,
                Some(t) if t.uuid != idx.uuid => true,
                Some(t) => {
                    if t.meta != ms {
                        recs.push(json!({"t": "meta", "name": name, "meta": meta}));
                    }
                    if t.counters[0] != idx.index_total {
                        let wm = t.counters[0];
                        let mut changed: Vec<(&String, i64)> = idx
                            .seq_nos
                            .iter()
                            .filter(|(_, s)| **s >= wm)
                            .map(|(id, s)| (id, *s))
                            .collect();
                        changed.sort_by_key(|(_, s)| *s);
                        if (changed.len() as i64) < idx.index_total - wm {
                            true
                        } else {
                            for (id, _) in changed {
                                recs.push(doc_record(idx, id, true));
                            }
                            false
                        }
                    } else {
                        false
                    }
                }
            };
            if full {
                recs.push(full_index(idx, true));
            } else if self.indices[name].counters != counters {
                recs.push(json!({"t": "counters", "name": name, "v": counters.to_vec()}));
            }
            self.indices.insert(
                name.clone(),
                Tracked { uuid: idx.uuid.clone(), meta: ms, counters },
            );
        }
        recs
    }

    fn write_snapshot(&mut self, store: &Store) -> Result<(), String> {
        let mut names: Vec<&String> = store.indices.keys().collect();
        names.sort();
        let indices: Vec<Value> = names.iter().map(|n| full_index(&store.indices[*n], false)).collect();
        let snap = json!({"gen": self.gen, "global": global_json(store), "indices": indices});
        let tmp = self.dir.join(format!("{}.tmp", SNAPSHOT));
        {
            let mut f = File::create(&tmp).map_err(|e| e.to_string())?;
            f.write_all(&serde_json::to_vec(&snap).map_err(|e| e.to_string())?)
                .map_err(|e| e.to_string())?;
            f.sync_all().map_err(|e| e.to_string())?;
        }
        fs::rename(&tmp, self.dir.join(SNAPSHOT)).map_err(|e| e.to_string())?;
        sync_dir(&self.dir);
        self.global = global_json(store).to_string();
        self.indices = store
            .indices
            .iter()
            .map(|(n, idx)| {
                (
                    n.clone(),
                    Tracked { uuid: idx.uuid.clone(), meta: index_meta(idx).to_string(), counters: counters(idx) },
                )
            })
            .collect();
        Ok(())
    }

    fn compact(&mut self, store: &Store) -> Result<(), String> {
        let next = self.gen + 1;
        let journal = create_journal(&self.dir, next)?;
        let prev = self.gen;
        self.gen = next;
        if let Err(e) = self.write_snapshot(store) {
            self.gen = prev;
            let _ = fs::remove_file(self.dir.join(journal_name(next)));
            return Err(e);
        }
        self.journal = journal;
        self.journal_bytes = 0;
        remove_stale_journals(&self.dir, next);
        Ok(())
    }
}

// ---- serialization -----------------------------------------------------------------

fn sorted_obj<T, F: Fn(&T) -> Value>(m: &HashMap<String, T>, f: F) -> Value {
    let mut keys: Vec<&String> = m.keys().collect();
    keys.sort();
    let mut out = Map::new();
    for k in keys {
        out.insert(k.clone(), f(&m[k]));
    }
    Value::Object(out)
}

fn global_json(store: &Store) -> Value {
    let obj = |m: &Map<String, Value>| Value::Object(m.clone());
    let mut streams: Vec<&DataStreamInfo> = store.data_streams.values().collect();
    streams.sort_by(|a, b| a.name.cmp(&b.name));
    json!({
        "counter": store.counter,
        "scroll_counter": store.scroll_counter,
        "pit_counter": store.pit_counter,
        "state_version": store.state_version,
        "cluster_persistent": store.cluster_persistent,
        "legacy_templates": sorted_obj(&store.legacy_templates, obj),
        "composable_templates": sorted_obj(&store.composable_templates, obj),
        "component_templates": sorted_obj(&store.component_templates, obj),
        "data_streams": streams.iter().map(|d| json!({
            "name": d.name,
            "backing_indices": d.backing_indices,
            "generation": d.generation,
            "timestamp_field": d.timestamp_field,
            "template": d.template,
        })).collect::<Vec<_>>(),
        "pipelines": sorted_obj(&store.pipelines, obj),
        "search_pipelines": sorted_obj(&store.search_pipelines, obj),
        "scripts": sorted_obj(&store.scripts, obj),
    })
}

fn index_meta(idx: &Index) -> Value {
    json!({
        "uuid": idx.uuid,
        "creation_date": idx.creation_date,
        "mappings": idx.mappings,
        "settings": idx.settings,
        "field_types": sorted_obj(&idx.field_types, |s| json!(s)),
        "closed": idx.closed,
        "aliases": idx.aliases,
        "mapping_version": idx.mapping_version,
        "data_stream": idx.data_stream,
        "verified": idx.verified,
        "force_merged": idx.force_merged,
    })
}

fn counters(idx: &Index) -> [i64; 4] {
    [idx.index_total, idx.delete_total, idx.deletes, idx.seg_gen]
}

/// Current state of one document id: the document, or its delete tombstone.
fn doc_record(idx: &Index, id: &str, tagged: bool) -> Value {
    let mut r = Map::new();
    if tagged {
        r.insert("t".into(), json!("doc"));
        r.insert("i".into(), json!(idx.name));
    }
    r.insert("id".into(), json!(id));
    if let Some(v) = idx.versions.get(id) {
        r.insert("v".into(), json!(v));
    }
    if let Some(s) = idx.seq_nos.get(id) {
        r.insert("s".into(), json!(s));
    }
    match idx.docs.get(id) {
        Some(doc) => {
            r.insert("src".into(), doc.clone());
            if let Some(raw) = idx.raw_sources.get(id) {
                match std::str::from_utf8(raw) {
                    Ok(s) => r.insert("raw".into(), json!(s)),
                    Err(_) => r.insert("raw64".into(), json!(base64::encode(raw))),
                };
            }
            if let Some(rt) = idx.routing.get(id) {
                r.insert("r".into(), json!(rt));
            }
        }
        None => {
            r.insert("del".into(), json!(true));
        }
    }
    Value::Object(r)
}

fn full_index(idx: &Index, tagged: bool) -> Value {
    let mut ids: HashSet<&String> = idx.versions.keys().collect();
    ids.extend(idx.docs.keys());
    let mut ids: Vec<&String> = ids.into_iter().collect();
    ids.sort_by_key(|id| idx.seq_nos.get(*id).copied().unwrap_or(-1));
    let docs: Vec<Value> = ids.iter().map(|id| doc_record(idx, id, false)).collect();
    let mut r = json!({
        "name": idx.name,
        "meta": index_meta(idx),
        "counters": counters(idx).to_vec(),
        "order": idx.order,
        "docs": docs,
    });
    if tagged {
        r.as_object_mut().unwrap().insert("t".into(), json!("index"));
    }
    r
}

// ---- replay ------------------------------------------------------------------------

fn str_of(v: &Value, k: &str) -> String {
    v.get(k).and_then(|x| x.as_str()).unwrap_or("").to_string()
}

fn map_of(v: &Value, k: &str) -> Map<String, Value> {
    v.get(k).and_then(|x| x.as_object()).cloned().unwrap_or_default()
}

fn obj_hash(v: &Value, k: &str) -> HashMap<String, Map<String, Value>> {
    map_of(v, k)
        .into_iter()
        .filter_map(|(n, x)| match x {
            Value::Object(m) => Some((n, m)),
            _ => None,
        })
        .collect()
}

fn apply_record(store: &mut Store, rec: &Value) {
    match rec.get("t").and_then(|t| t.as_str()).unwrap_or("") {
        "global" => {
            if let Some(g) = rec.get("v") {
                apply_global(store, g);
            }
        }
        "drop" => {
            store.indices.remove(&str_of(rec, "name"));
        }
        "index" => {
            apply_index(store, rec);
            let name = str_of(rec, "name");
            for d in rec.get("docs").and_then(|v| v.as_array()).into_iter().flatten() {
                apply_doc(store, &name, d, false);
            }
        }
        "meta" => {
            if let (Some(idx), Some(meta)) = (store.indices.get_mut(&str_of(rec, "name")), rec.get("meta")) {
                apply_meta(idx, meta);
            }
        }
        "counters" => {
            if let Some(idx) = store.indices.get_mut(&str_of(rec, "name")) {
                apply_counters(idx, rec.get("v"));
            }
        }
        "doc" => {
            let name = str_of(rec, "i");
            apply_doc(store, &name, rec, true);
        }
        _ => {}
    }
}

fn apply_global(store: &mut Store, g: &Value) {
    store.counter = g.get("counter").and_then(|v| v.as_i64()).unwrap_or(0);
    // ids of scrolls and PITs are never reused, although the contexts themselves die
    store.scroll_counter = g.get("scroll_counter").and_then(|v| v.as_u64()).unwrap_or(0);
    store.pit_counter = g.get("pit_counter").and_then(|v| v.as_u64()).unwrap_or(0);
    store.state_version = g.get("state_version").and_then(|v| v.as_i64()).unwrap_or(1);
    store.cluster_persistent = map_of(g, "cluster_persistent");
    store.legacy_templates = obj_hash(g, "legacy_templates");
    store.composable_templates = obj_hash(g, "composable_templates");
    store.component_templates = obj_hash(g, "component_templates");
    store.pipelines = obj_hash(g, "pipelines");
    store.search_pipelines = obj_hash(g, "search_pipelines");
    store.scripts = obj_hash(g, "scripts");
    store.data_streams.clear();
    for d in g.get("data_streams").and_then(|v| v.as_array()).into_iter().flatten() {
        let info = DataStreamInfo {
            name: str_of(d, "name"),
            backing_indices: d
                .get("backing_indices")
                .and_then(|v| v.as_array())
                .map(|a| a.iter().filter_map(|x| x.as_str().map(String::from)).collect())
                .unwrap_or_default(),
            generation: d.get("generation").and_then(|v| v.as_i64()).unwrap_or(1),
            timestamp_field: str_of(d, "timestamp_field"),
            template: str_of(d, "template"),
        };
        store.data_streams.insert(info.name.clone(), info);
    }
}

fn apply_meta(idx: &mut Index, m: &Value) {
    idx.uuid = str_of(m, "uuid");
    idx.creation_date = m.get("creation_date").and_then(|v| v.as_i64()).unwrap_or(0);
    idx.mappings = m.get("mappings").cloned().unwrap_or_else(|| Value::Object(Map::new()));
    idx.settings = map_of(m, "settings");
    idx.field_types = map_of(m, "field_types")
        .into_iter()
        .map(|(k, v)| (k, v.as_str().unwrap_or("").to_string()))
        .collect();
    idx.closed = m.get("closed").and_then(|v| v.as_bool()).unwrap_or(false);
    idx.aliases = map_of(m, "aliases");
    idx.mapping_version = m.get("mapping_version").and_then(|v| v.as_i64()).unwrap_or(1);
    idx.data_stream = str_of(m, "data_stream");
    idx.verified = m.get("verified").and_then(|v| v.as_bool()).unwrap_or(false);
    idx.force_merged = m.get("force_merged").and_then(|v| v.as_bool()).unwrap_or(false);
}

fn apply_counters(idx: &mut Index, v: Option<&Value>) {
    let get = |i: usize| v.and_then(|a| a.get(i)).and_then(|x| x.as_i64()).unwrap_or(0);
    idx.index_total = get(0);
    idx.delete_total = get(1);
    idx.deletes = get(2);
    idx.seg_gen = get(3);
}

/// A full index record: replaces any index of that name.
fn apply_index(store: &mut Store, rec: &Value) {
    let name = str_of(rec, "name");
    let mut idx = Index::skeleton(&name);
    if let Some(m) = rec.get("meta") {
        apply_meta(&mut idx, m);
    }
    apply_counters(&mut idx, rec.get("counters"));
    idx.order = rec
        .get("order")
        .and_then(|v| v.as_array())
        .map(|a| a.iter().filter_map(|x| x.as_str().map(String::from)).collect())
        .unwrap_or_default();
    store.indices.insert(name, idx);
}

fn apply_doc(store: &mut Store, index: &str, d: &Value, track_order: bool) {
    let idx = match store.indices.get_mut(index) {
        Some(i) => i,
        None => return,
    };
    let id = str_of(d, "id");
    match d.get("v").and_then(|v| v.as_i64()) {
        Some(v) => idx.versions.insert(id.clone(), v),
        None => idx.versions.remove(&id),
    };
    match d.get("s").and_then(|v| v.as_i64()) {
        Some(s) => {
            idx.seq_nos.insert(id.clone(), s);
            if track_order && s >= idx.index_total {
                idx.index_total = s + 1;
            }
        }
        None => {
            idx.seq_nos.remove(&id);
        }
    }
    if d.get("del").and_then(|v| v.as_bool()) == Some(true) {
        idx.docs.remove(&id);
        idx.raw_sources.remove(&id);
        idx.routing.remove(&id);
        return;
    }
    if track_order && !idx.docs.contains_key(&id) {
        idx.order.push(id.clone());
    }
    idx.docs.insert(id.clone(), d.get("src").cloned().unwrap_or_else(|| Value::Object(Map::new())));
    if let Some(raw) = d.get("raw").and_then(|v| v.as_str()) {
        idx.raw_sources.insert(id.clone(), raw.as_bytes().to_vec());
    } else if let Some(raw) = d.get("raw64").and_then(|v| v.as_str()).and_then(|s| base64::decode(s).ok()) {
        idx.raw_sources.insert(id.clone(), raw);
    } else {
        idx.raw_sources.remove(&id);
    }
    match d.get("r").and_then(|v| v.as_str()) {
        Some(r) => idx.routing.insert(id, r.to_string()),
        None => idx.routing.remove(&id),
    };
}
