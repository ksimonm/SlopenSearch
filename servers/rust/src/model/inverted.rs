//! The inverted index of an index's doc values (see docs/engine-design.md).
//!
//! `DocValues` is the per-document field value table the query evaluator reads;
//! it notes which documents changed since the index last caught up. The
//! `InvertedIndex` maps each indexed field to term -> postings over internal
//! document numbers. A changed document gets a new number (the old one is
//! marked dead), so postings stay sorted by document number without
//! rewriting; the table is renumbered when the dead numbers outweigh the live
//! ones. Which terms a field has (analysis) is decided by the search layer,
//! which fills the fields lazily and keeps them up to date (`search::index`).
//!
//! The index is always derivable from `DocValues` (it is never persisted): a
//! clone starts empty and is rebuilt on first use.

use serde_json::{Map, Value};
use std::collections::{HashMap, HashSet};
use std::ops::Deref;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Mutex, RwLock};


/// Documents whose doc values changed since the inverted index caught up.
#[derive(Default)]
pub struct Dirty {
    pub ids: HashSet<String>,
    /// Everything may have changed (cleared, replaced or cloned).
    pub all: bool,
}

/// A map keyed by document id that notes which ids changed (for the
/// inverted index). Reads go through `Deref`; every mutation goes through the
/// methods below.
pub struct TrackedMap<V> {
    map: HashMap<String, V>,
    dirty: Mutex<Dirty>,
    flag: AtomicBool,
}

/// Doc values per document id.
pub type DocValues = TrackedMap<Map<String, Value>>;
/// `_source` per document id.
pub type Docs = TrackedMap<Value>;

impl<V> Default for TrackedMap<V> {
    fn default() -> Self {
        TrackedMap { map: HashMap::new(), dirty: Mutex::new(Dirty::default()), flag: AtomicBool::new(false) }
    }
}

impl<V: Clone> Clone for TrackedMap<V> {
    fn clone(&self) -> Self {
        TrackedMap {
            map: self.map.clone(),
            dirty: Mutex::new(Dirty { ids: HashSet::new(), all: true }),
            flag: AtomicBool::new(true),
        }
    }
}

impl<V> Deref for TrackedMap<V> {
    type Target = HashMap<String, V>;
    fn deref(&self) -> &HashMap<String, V> {
        &self.map
    }
}

impl<V> TrackedMap<V> {
    fn mark(&mut self, id: &str) {
        let d = self.dirty.get_mut().unwrap_or_else(|e| e.into_inner());
        if !d.all {
            d.ids.insert(id.to_string());
        }
        *self.flag.get_mut() = true;
    }

    pub fn insert(&mut self, id: String, value: V) -> Option<V> {
        self.mark(&id);
        self.map.insert(id, value)
    }

    pub fn remove(&mut self, id: &str) -> Option<V> {
        let old = self.map.remove(id);
        if old.is_some() {
            self.mark(id);
        }
        old
    }

    pub fn clear(&mut self) {
        self.map.clear();
        let d = self.dirty.get_mut().unwrap_or_else(|e| e.into_inner());
        d.all = true;
        d.ids.clear();
        *self.flag.get_mut() = true;
    }

    /// Whether some id changed since the last [`TrackedMap::take_dirty`].
    pub fn is_dirty(&self) -> bool {
        self.flag.load(Ordering::Acquire)
    }

    /// The changed ids; resets the tracking.
    pub fn take_dirty(&self) -> Dirty {
        let mut d = self.dirty.lock().unwrap_or_else(|e| e.into_inner());
        self.flag.store(false, Ordering::Release);
        std::mem::take(&mut *d)
    }
}

/// Postings of one field: term -> (doc number, term frequency), sorted by doc
/// number (dead numbers included; check [`InvertedIndex::live`]).
#[derive(Default)]
pub struct FieldIndex {
    pub postings: hashbrown::HashMap<String, Vec<(u32, u32)>>,
    /// Per doc number: the number of tokens (0: no value).
    pub lens: Vec<u32>,
    /// Live documents with at least one token, and their total token count.
    pub docs_with: usize,
    pub sum_len: u64,
    /// Numeric fields: (value, doc number), sorted by value up to `nums_sorted`.
    pub nums: Vec<(f64, u32)>,
    pub nums_sorted: usize,
    /// A numeric field had a value that is not a number (the column is incomplete).
    pub non_numeric: bool,
    /// What the analysis of the field depends on (see `search::index`): the
    /// field is rebuilt when it changes.
    pub sig: String,
    /// Metric columns: the aggregation value per doc number (None: none,
    /// several, or not computed; the aggregation reads the document).
    pub fwd: std::sync::Arc<Vec<Option<f64>>>,
}

impl FieldIndex {
    /// Adds the tokens of document `doc` (higher than every number added before).
    pub fn add(&mut self, doc: u32, tokens: Vec<String>) {
        let len = tokens.len() as u32;
        let d = doc as usize;
        if self.lens.len() <= d {
            self.lens.resize(d + 1, 0);
        }
        self.lens[d] = len;
        if len == 0 {
            return;
        }
        self.docs_with += 1;
        self.sum_len += len as u64;
        // documents come in ascending order: a term's last posting is this
        // document's when the term occurred before in it
        for t in tokens {
            match self.postings.get_mut(&t) {
                Some(p) => match p.last_mut() {
                    Some(last) if last.0 == doc => last.1 += 1,
                    _ => p.push((doc, 1)),
                },
                None => {
                    self.postings.insert(t, vec![(doc, 1)]);
                }
            }
        }
    }

    /// Appends `other`, built over documents numbered above every document of
    /// this field.
    pub fn append(&mut self, mut other: FieldIndex) {
        if self.postings.is_empty() && self.lens.iter().all(|l| *l == 0) && self.nums.is_empty() && self.fwd.is_empty() {
            // nothing here yet: take the other side as it is
            other.sig = std::mem::take(&mut self.sig);
            other.nums_sorted = 0;
            *self = other;
            return;
        }
        if self.postings.len() < other.postings.len() && self.lens.len() <= other.lens.len() {
            // merge the smaller map into the larger one, keeping doc order
            std::mem::swap(&mut self.postings, &mut other.postings);
            for (t, p) in other.postings.drain() {
                match self.postings.get_mut(&t) {
                    Some(mine) => {
                        let mut merged = p;
                        merged.extend_from_slice(mine);
                        *mine = merged;
                    }
                    None => {
                        self.postings.insert(t, p);
                    }
                }
            }
        }
        if self.lens.len() < other.lens.len() {
            self.lens.resize(other.lens.len(), 0);
        }
        for (d, l) in other.lens.iter().enumerate() {
            if *l > 0 {
                self.lens[d] = *l;
            }
        }
        self.docs_with += other.docs_with;
        self.sum_len += other.sum_len;
        for (t, p) in other.postings {
            match self.postings.get_mut(&t) {
                Some(mine) => mine.extend_from_slice(&p),
                None => {
                    self.postings.insert(t, p);
                }
            }
        }
        self.nums.extend_from_slice(&other.nums);
        self.non_numeric |= other.non_numeric;
        if !other.fwd.is_empty() {
            let fwd = std::sync::Arc::make_mut(&mut self.fwd);
            if fwd.len() < other.fwd.len() {
                fwd.resize(other.fwd.len(), None);
            }
            for (d, v) in other.fwd.iter().enumerate() {
                if v.is_some() {
                    fwd[d] = *v;
                }
            }
        }
    }

    /// Adds the numeric values of document `doc`.
    pub fn add_numbers(&mut self, doc: u32, values: &[Value]) {
        let d = doc as usize;
        if self.lens.len() <= d {
            self.lens.resize(d + 1, 0);
        }
        let mut n = 0u32;
        for v in values {
            match v {
                Value::Number(x) => match x.as_f64() {
                    Some(f) if f.is_finite() => {
                        self.nums.push((f, doc));
                        n += 1;
                    }
                    _ => self.non_numeric = true,
                },
                Value::Null => {}
                _ => self.non_numeric = true,
            }
        }
        if n > 0 {
            self.lens[d] = n;
            self.docs_with += 1;
            self.sum_len += n as u64;
        }
    }

    /// Whether the numeric column is ready for range lookups.
    pub fn nums_ready(&self) -> bool {
        self.nums_sorted == self.nums.len()
    }

    /// Sorts the values added since the last call and merges them in.
    pub fn sort_nums(&mut self) {
        if self.nums_ready() {
            return;
        }
        let cmp = |a: &(f64, u32), b: &(f64, u32)| a.0.total_cmp(&b.0).then(a.1.cmp(&b.1));
        let k = self.nums_sorted;
        self.nums[k..].sort_by(cmp);
        if k > 0 {
            let tail = self.nums.split_off(k);
            let head = std::mem::take(&mut self.nums);
            let mut out = Vec::with_capacity(head.len() + tail.len());
            let (mut i, mut j) = (0, 0);
            while i < head.len() && j < tail.len() {
                if cmp(&tail[j], &head[i]) == std::cmp::Ordering::Less {
                    out.push(tail[j]);
                    j += 1;
                } else {
                    out.push(head[i]);
                    i += 1;
                }
            }
            out.extend_from_slice(&head[i..]);
            out.extend_from_slice(&tail[j..]);
            self.nums = out;
        }
        self.nums_sorted = self.nums.len();
    }

    /// Sets the metric column value of document `doc`.
    pub fn set_fwd(&mut self, doc: u32, v: Option<f64>) {
        let d = doc as usize;
        if self.fwd.len() <= d {
            if v.is_none() {
                return;
            }
            std::sync::Arc::make_mut(&mut self.fwd).resize(d + 1, None);
        }
        if self.fwd[d] != v {
            std::sync::Arc::make_mut(&mut self.fwd)[d] = v;
        }
    }

    fn kill(&mut self, doc: u32) {
        let d = doc as usize;
        if self.fwd.get(d).is_some_and(|v| v.is_some()) {
            std::sync::Arc::make_mut(&mut self.fwd)[d] = None;
        }
        if let Some(l) = self.lens.get_mut(d) {
            if *l > 0 {
                self.docs_with -= 1;
                self.sum_len -= *l as u64;
            }
            *l = 0;
        }
    }

    /// Term frequency of `term` in document `doc`.
    pub fn tf(&self, term: &str, doc: u32) -> u32 {
        match self.postings.get(term) {
            Some(p) => match p.binary_search_by_key(&doc, |e| e.0) {
                Ok(i) => p[i].1,
                Err(_) => 0,
            },
            None => 0,
        }
    }

    pub fn len_of(&self, doc: u32) -> u32 {
        self.lens.get(doc as usize).copied().unwrap_or(0)
    }
}

/// Document numbering and the indexed fields of one index.
#[derive(Default)]
pub struct InvertedIndex {
    /// Doc number -> id (None: dead).
    pub ids: Vec<Option<String>>,
    pub by_id: hashbrown::HashMap<String, u32>,
    pub dead: usize,
    /// Field key (kind prefix + field name) -> postings.
    pub fields: HashMap<String, FieldIndex>,
    /// The mapping and settings generations the fields were analyzed with.
    pub schema: (u64, u64),
    /// Whether the numbering covers every document of the doc values.
    pub numbered: bool,
    /// Ids of documents (in `docs`) without doc values, when `missing_known`.
    pub missing: HashSet<String>,
    pub missing_known: bool,
    /// Doc number per position of the index order (see `search::index`).
    pub order_map: Mutex<Option<OrderMap>>,
    /// The eagerly indexed fields, for the mapping/settings generations.
    pub eager: Option<((u64, u64), Vec<String>)>,
    /// Process-wide unique stamp of the current content (see [`InvertedIndex::touch`]).
    pub version: u64,
}

static VERSIONS: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(1);

impl InvertedIndex {
    /// Gives the content a new stamp (after any change).
    pub fn touch(&mut self) {
        self.version = VERSIONS.fetch_add(1, Ordering::Relaxed);
    }

    pub fn live(&self, doc: u32) -> bool {
        matches!(self.ids.get(doc as usize), Some(Some(_)))
    }

    pub fn live_count(&self) -> usize {
        self.by_id.len()
    }

    pub fn doc_of(&self, id: &str) -> Option<u32> {
        self.by_id.get(id).copied()
    }

    /// Forgets every document and every field's postings (field keys stay).
    pub fn reset_docs(&mut self) {
        self.ids.clear();
        self.by_id.clear();
        self.dead = 0;
        for f in self.fields.values_mut() {
            let sig = std::mem::take(&mut f.sig);
            *f = FieldIndex { sig, ..FieldIndex::default() };
        }
    }

    pub fn kill(&mut self, id: &str) {
        if let Some(n) = self.by_id.remove(id) {
            self.ids[n as usize] = None;
            self.dead += 1;
            for f in self.fields.values_mut() {
                f.kill(n);
            }
        }
    }

    pub fn alloc(&mut self, id: &str) -> u32 {
        let n = self.ids.len() as u32;
        self.ids.push(Some(id.to_string()));
        self.by_id.insert(id.to_string(), n);
        n
    }
}

/// Doc numbers of the index order at one content version.
pub struct OrderMap {
    pub version: u64,
    pub last: Option<u32>,
    pub docs: std::sync::Arc<Vec<u32>>,
    pub dups: bool,
}

/// The inverted index of an `Index`, behind its own lock so searches holding
/// a shared reference can bring it up to date.
#[derive(Default)]
pub struct SharedInverted(pub RwLock<InvertedIndex>);

impl Clone for SharedInverted {
    fn clone(&self) -> Self {
        SharedInverted::default()
    }
}
