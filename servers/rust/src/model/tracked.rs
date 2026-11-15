//! Change tracking for the persisted parts of the store (see `storage`): instead of
//! comparing serialized state after every request, the containers below remember
//! that they may have changed.

use std::collections::HashMap;
use std::ops::{Deref, DerefMut};
use std::sync::atomic::{AtomicBool, Ordering};

/// A value that notes every mutable access. Reads go through `Deref`; any
/// `&mut` access (method calls, `*x = ...`, passing `&mut x` on) marks it
/// dirty, so the storage layer only re-serializes what may have changed.
#[derive(Clone, Default, Debug)]
pub struct Tracked<T> {
    value: T,
    dirty: bool,
    /// Changes on every mutable access (process-wide counter): derived data
    /// such as the inverted index notices that the value changed.
    gen: u64,
}

static GENERATION: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(1);

fn next_generation() -> u64 {
    GENERATION.fetch_add(1, Ordering::Relaxed)
}

impl<T> Tracked<T> {
    pub fn new(value: T) -> Self {
        Tracked { value, dirty: true, gen: next_generation() }
    }

    /// A number that changes whenever the value may have changed.
    pub fn generation(&self) -> u64 {
        self.gen
    }

    /// Whether the value was mutably accessed since the last call; resets the flag.
    pub fn take_dirty(&mut self) -> bool {
        std::mem::replace(&mut self.dirty, false)
    }
}

impl<T> From<T> for Tracked<T> {
    fn from(value: T) -> Self {
        Tracked::new(value)
    }
}

impl<T> Deref for Tracked<T> {
    type Target = T;
    fn deref(&self) -> &T {
        &self.value
    }
}

impl<T> DerefMut for Tracked<T> {
    fn deref_mut(&mut self) -> &mut T {
        self.dirty = true;
        self.gen = next_generation();
        &mut self.value
    }
}

impl<T: PartialEq> PartialEq<T> for Tracked<T> {
    fn eq(&self, other: &T) -> bool {
        self.value == *other
    }
}

impl<'a, T> IntoIterator for &'a Tracked<T>
where
    &'a T: IntoIterator,
{
    type Item = <&'a T as IntoIterator>::Item;
    type IntoIter = <&'a T as IntoIterator>::IntoIter;
    fn into_iter(self) -> Self::IntoIter {
        self.value.into_iter()
    }
}

impl<A, T: FromIterator<A>> FromIterator<A> for Tracked<T> {
    fn from_iter<I: IntoIterator<Item = A>>(iter: I) -> Self {
        Tracked::new(T::from_iter(iter))
    }
}

impl<T: serde::Serialize> serde::Serialize for Tracked<T> {
    fn serialize<S: serde::Serializer>(&self, s: S) -> Result<S::Ok, S::Error> {
        self.value.serialize(s)
    }
}

/// Whether document writes are logged for the storage layer (on when persistence is
/// on; in-memory mode never drains the log).
static LOG_WRITES: AtomicBool = AtomicBool::new(false);

pub fn enable_write_log() {
    LOG_WRITES.store(true, Ordering::Relaxed);
}

/// The seq_no of every document id, plus a log of the ids that were assigned a seq_no
/// since the storage layer last looked. Every document write (index, update, delete)
/// assigns a new seq_no, so the log names exactly the documents a request touched.
#[derive(Clone, Default, Debug)]
pub struct SeqNos {
    map: crate::model::shared::Shared<HashMap<String, i64>>,
    log: Vec<(String, i64)>,
    /// Set when the map was changed other than through `insert`.
    untracked: bool,
}

impl SeqNos {
    /// Whether both share the same table (see `model::shared`).
    pub fn same(&self, other: &SeqNos) -> bool {
        self.map.same(&other.map)
    }

    pub fn insert(&mut self, id: String, seq: i64) -> Option<i64> {
        if LOG_WRITES.load(Ordering::Relaxed) {
            self.log.push((id.clone(), seq));
        }
        self.map.insert(id, seq)
    }

    /// The (id, seq_no) assignments since the last call, and whether the map was
    /// also changed in other ways.
    pub fn take_log(&mut self) -> (Vec<(String, i64)>, bool) {
        (std::mem::take(&mut self.log), std::mem::replace(&mut self.untracked, false))
    }
}

impl Deref for SeqNos {
    type Target = HashMap<String, i64>;
    fn deref(&self) -> &Self::Target {
        &self.map
    }
}

impl DerefMut for SeqNos {
    fn deref_mut(&mut self) -> &mut Self::Target {
        self.untracked = true;
        &mut self.map
    }
}
