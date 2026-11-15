use serde_json::{Map, Value};
use std::collections::HashMap;

use crate::settings::{find_setting, setting_int, settings_value_string};

pub(crate) mod contexts;
pub(crate) mod routing;
pub(crate) mod tracked;
pub(crate) mod visibility;

pub(crate) use contexts::{PitState, ScrollState};
pub(crate) use tracked::{SeqNos, Tracked};

pub const VERSION_CREATED: &str = "137307827";
pub const CLUSTER_NAME: &str = "opensearch";
pub const CLUSTER_UUID: &str = "slopensearch-cluster-uuid";
pub const NODE_ID: &str = "slopensearch-node-id01";
pub const NODE_NAME: &str = "slopensearch-node";
pub const NODE_VERSION: &str = "3.9.0";
pub const LUCENE_VERSION: &str = "10.5.1";

#[derive(Clone)]
pub struct Index {
    pub name: String,
    pub uuid: String,
    pub creation_date: i64,
    pub mappings: Tracked<Value>,
    pub settings: Tracked<Map<String, Value>>,
    pub docs: HashMap<String, Value>,
    pub order: Vec<String>,
    pub field_types: Tracked<HashMap<String, String>>,
    pub closed: bool,
    pub aliases: Tracked<Map<String, Value>>,
    pub mapping_version: i64,
    pub pending: HashMap<String, Value>,
    pub routing: HashMap<String, String>,
    pub seg_gen: i64,
    pub deletes: i64,
    pub data_stream: String,
    pub verified: bool,
    pub index_total: i64,
    pub delete_total: i64,
    pub force_merged: bool,
    pub doc_values: HashMap<String, Map<String, Value>>,
    pub doc_ignored: HashMap<String, Vec<String>>,
    pub versions: HashMap<String, i64>,
    pub seq_nos: SeqNos,
    pub raw_sources: HashMap<String, Vec<u8>>,
    /// Last refreshed versions of updated documents, request cache.
    pub searcher: visibility::SearcherState,
}

impl Index {
    /// Minimal index used where OpenSearch only needs the mapping machinery
    /// (for example validating an alias filter query).
    pub fn skeleton(name: &str) -> Self {
        Index {
            name: name.to_string(),
            uuid: String::new(),
            creation_date: 0,
            mappings: Value::Object(Map::new()).into(),
            settings: Map::new().into(),
            docs: HashMap::new(),
            order: Vec::new(),
            field_types: HashMap::new().into(),
            closed: false,
            aliases: Map::new().into(),
            mapping_version: 0,
            pending: HashMap::new(),
            routing: HashMap::new(),
            seg_gen: 0,
            deletes: 0,
            data_stream: String::new(),
            verified: false,
            index_total: 0,
            delete_total: 0,
            force_merged: false,
            doc_values: HashMap::new(),
            doc_ignored: HashMap::new(),
            versions: HashMap::new(),
            seq_nos: SeqNos::default(),
            raw_sources: HashMap::new(),
            searcher: Default::default(),
        }
    }

    pub fn is_hidden(&self) -> bool {
        match find_setting(&self.settings, "hidden") {
            Some(v) => settings_value_string(v) == "true",
            None => false,
        }
    }

    pub fn block_on(&self, name: &str) -> bool {
        match find_setting(&self.settings, &format!("blocks.{}", name)) {
            Some(v) => settings_value_string(v) == "true",
            None => false,
        }
    }

    pub fn refresh_disabled(&self) -> bool {
        match find_setting(&self.settings, "refresh_interval") {
            Some(v) => settings_value_string(v) == "-1",
            None => false,
        }
    }

    pub fn num_shards(&self) -> i64 {
        setting_int(&self.settings, "number_of_shards").unwrap_or(1)
    }

    pub fn num_replicas(&self) -> i64 {
        setting_int(&self.settings, "number_of_replicas").unwrap_or(1)
    }

    pub fn store_bytes(&self) -> i64 {
        if self.docs.is_empty() {
            return 0;
        }
        self.docs.len() as i64 * 64 + 64
    }

    pub fn ensure_field_types(&mut self) {
        let mut fresh = HashMap::new();
        build_field_types(&self.mappings, "", &mut fresh);
        *self.field_types = fresh;
    }
}

#[derive(Clone)]
pub struct DataStreamInfo {
    pub name: String,
    pub backing_indices: Vec<String>,
    pub generation: i64,
    pub timestamp_field: String,
    pub template: String,
}

pub struct Store {
    pub indices: HashMap<String, Index>,
    pub counter: i64,
    pub state_version: i64,
    pub cluster_persistent: Tracked<Map<String, Value>>,
    pub cluster_transient: Map<String, Value>,
    pub legacy_templates: Tracked<HashMap<String, Map<String, Value>>>,
    pub composable_templates: Tracked<HashMap<String, Map<String, Value>>>,
    pub component_templates: Tracked<HashMap<String, Map<String, Value>>>,
    pub data_streams: Tracked<HashMap<String, DataStreamInfo>>,
    pub pipelines: Tracked<HashMap<String, Map<String, Value>>>,
    pub search_pipelines: Tracked<HashMap<String, Map<String, Value>>>,
    pub scripts: Tracked<HashMap<String, Map<String, Value>>>,
    pub scrolls: HashMap<String, ScrollState>,
    pub scroll_counter: u64,
    pub pits: HashMap<String, PitState>,
    pub pit_counter: u64,
}

impl Store {
    pub fn new() -> Self {
        Store {
            indices: HashMap::new(),
            counter: 0,
            state_version: 1,
            cluster_persistent: Map::new().into(),
            cluster_transient: Map::new(),
            legacy_templates: HashMap::new().into(),
            composable_templates: HashMap::new().into(),
            component_templates: HashMap::new().into(),
            data_streams: HashMap::new().into(),
            pipelines: HashMap::new().into(),
            search_pipelines: HashMap::new().into(),
            scripts: HashMap::new().into(),
            scrolls: HashMap::new(),
            scroll_counter: 0,
            pits: HashMap::new(),
            pit_counter: 0,
        }
    }

    pub fn next_id(&mut self) -> String {
        self.counter += 1;
        format!("{:020x}", self.counter)
    }

    pub fn bump_state(&mut self) {
        self.state_version += 1;
    }
}

pub fn now_millis() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_millis() as i64)
        .unwrap_or(0)
}

static UUID_STATE: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(0x9E3779B97F4A7C15);

/// Starts the uuid sequence elsewhere (a restarted node must not hand out the uuids
/// of indices it created before).
pub fn reseed_uuid(seed: u64) {
    UUID_STATE.store(seed | 1, std::sync::atomic::Ordering::Relaxed);
}

pub fn random_uuid() -> String {
    use std::sync::atomic::Ordering;
    let mut bytes = [0u8; 16];
    let mut x = UUID_STATE.fetch_add(0x9E3779B97F4A7C15, Ordering::Relaxed);
    for chunk in bytes.chunks_mut(8) {
        x ^= x << 13;
        x ^= x >> 7;
        x ^= x << 17;
        for (i, b) in chunk.iter_mut().enumerate() {
            *b = (x >> (i * 8)) as u8;
        }
    }
    let hex: String = bytes.iter().map(|b| format!("{:02x}", b)).collect();
    format!(
        "{}-{}-{}-{}-{}",
        &hex[0..8],
        &hex[8..12],
        &hex[12..16],
        &hex[16..20],
        &hex[20..32]
    )
}

fn build_field_types(mappings: &Value, prefix: &str, out: &mut HashMap<String, String>) {
    let props = match mappings.get("properties").and_then(|p| p.as_object()) {
        Some(p) => p,
        None => return,
    };
    for (name, raw) in props {
        let def = match raw.as_object() {
            Some(d) => d,
            None => continue,
        };
        let path = if prefix.is_empty() {
            name.clone()
        } else {
            format!("{}.{}", prefix, name)
        };
        if let Some(t) = def.get("type").and_then(|v| v.as_str()) {
            out.insert(path.clone(), t.to_string());
        }
        if let Some(fields) = def.get("fields").and_then(|f| f.as_object()) {
            for (fname, fraw) in fields {
                if let Some(ft) = fraw.get("type").and_then(|v| v.as_str()) {
                    out.insert(format!("{}.{}", path, fname), ft.to_string());
                }
            }
        }
        build_field_types(&Value::Object(def.clone()), &path, out);
    }
}
