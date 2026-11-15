//! Terms columns: the values a `terms` aggregation reads per document,
//! cached per index content (see docs/engine-design.md).
//!
//! `terms_agg` asks `doc_field_values` for the values of every document and
//! keys its buckets by their string. That is a function of the stored
//! document, the mapping and the field, so it is computed once per index
//! content version into a column by doc number (the values interned: the
//! distinct raw values, and the bucket key of each) and the bucketing reads
//! the column. Buckets are created in the order the documents first show
//! their value, from that first raw value, as the scan does.

use std::collections::HashMap;
use std::sync::{Arc, Mutex};

use serde_json::Value;

use crate::jsonv::value_to_string;
use crate::model::Index;

struct Column {
    /// Distinct raw values, and the bucket (key string) of each.
    raw: Vec<Value>,
    group: Vec<u32>,
    /// Bucket key strings.
    ids: Vec<String>,
    /// Values of doc number `d`: `tids[offsets[d]..offsets[d + 1]]`.
    offsets: Vec<u32>,
    tids: Vec<u32>,
}

static COLUMNS: Mutex<Vec<(String, Arc<Column>)>> = Mutex::new(Vec::new());

fn column(idx: &Index, field: &str, version: u64) -> Option<Arc<Column>> {
    crate::search::index::with_doc_ids(idx, |v, ids| {
        if v != version {
            return None;
        }
        let key = format!("{}\u{1}{}\u{1}{}\u{1}{}\u{1}{}", idx.uuid, v, idx.mappings.generation(), idx.settings.generation(), field);
        if let Some(c) = COLUMNS.lock().unwrap_or_else(|e| e.into_inner()).iter().find(|e| e.0 == key) {
            return Some(c.1.clone());
        }
        use rayon::prelude::*;
        let vals: Vec<Vec<Value>> = ids
            .par_iter()
            .with_min_len(2048)
            .map(|id| {
                let Some(id) = id else { return Vec::new() };
                let Some(doc) = idx.docs.get(id) else { return Vec::new() };
                super::doc_field_values_at(idx, doc, Some(id), field, "")
            })
            .collect();
        let mut col = Column { raw: Vec::new(), group: Vec::new(), ids: Vec::new(), offsets: Vec::with_capacity(vals.len() + 1), tids: Vec::new() };
        let mut by_raw: HashMap<(String, String), u32> = HashMap::new();
        let mut by_id: HashMap<String, u32> = HashMap::new();
        col.offsets.push(0);
        for dv in vals {
            for v in dv {
                let id = value_to_string(&v).unwrap_or_default();
                let rk = (id.clone(), v.to_string());
                let tid = match by_raw.get(&rk) {
                    Some(t) => *t,
                    None => {
                        let g = match by_id.get(&id) {
                            Some(g) => *g,
                            None => {
                                col.ids.push(id.clone());
                                by_id.insert(id, (col.ids.len() - 1) as u32);
                                (col.ids.len() - 1) as u32
                            }
                        };
                        col.raw.push(v);
                        col.group.push(g);
                        let t = (col.raw.len() - 1) as u32;
                        by_raw.insert(rk, t);
                        t
                    }
                };
                col.tids.push(tid);
            }
            col.offsets.push(col.tids.len() as u32);
        }
        let col = Arc::new(col);
        let mut cache = COLUMNS.lock().unwrap_or_else(|e| e.into_inner());
        let stale = format!("{}\u{1}", idx.uuid);
        let live = format!("{}\u{1}{}\u{1}", idx.uuid, v);
        cache.retain(|e| !e.0.starts_with(&stale) || e.0.starts_with(&live));
        if cache.len() >= 16 {
            cache.remove(0);
        }
        cache.push((key, col.clone()));
        Some(col)
    })?
}

/// One bucket: its key string, the raw value it was created from, and its
/// documents (with their doc numbers), in document order.
pub(super) struct Group<'a> {
    pub id: String,
    pub raw: Value,
    pub docs: Vec<&'a Value>,
    pub nums: Vec<u32>,
}

pub(super) struct FastBuckets<'a> {
    pub version: u64,
    /// In the order the documents first show them.
    pub groups: Vec<Group<'a>>,
}

/// The buckets of the documents `docs` for `field`, when their doc numbers
/// are known (root documents or a registered bucket).
pub(super) fn bucket_docs<'a>(idx: &Index, docs: &[&'a Value], field: &str) -> Option<FastBuckets<'a>> {
    let (version, nums) = super::docs_numbers(docs)?;
    let col = column(idx, field, version)?;
    let mut slot: Vec<u32> = vec![u32::MAX; col.ids.len()];
    let mut groups: Vec<Group<'a>> = Vec::new();
    for (i, d) in docs.iter().enumerate() {
        let n = *nums.get(i)? as usize;
        let (lo, hi) = (*col.offsets.get(n)? as usize, *col.offsets.get(n + 1)? as usize);
        for &tid in &col.tids[lo..hi] {
            let g = col.group[tid as usize] as usize;
            if slot[g] == u32::MAX {
                slot[g] = groups.len() as u32;
                groups.push(Group { id: col.ids[g].clone(), raw: col.raw[tid as usize].clone(), docs: Vec::new(), nums: Vec::new() });
            }
            let b = &mut groups[slot[g] as usize];
            b.docs.push(*d);
            b.nums.push(n as u32);
        }
    }
    Some(FastBuckets { version, groups })
}
