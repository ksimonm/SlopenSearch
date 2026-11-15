//! Copy-on-write containers for the large per-document tables of an `Index`.
//!
//! Scrolls and PITs keep a copy of the searched indices (a point-in-time view).
//! With the tables behind an `Arc`, that copy costs a reference count per table;
//! a later write to the live index copies only the table it changes, and only
//! while some view still shares it. Reads go through `Deref`, writes through
//! `DerefMut` (`Arc::make_mut`), so the call sites are unchanged.

use std::ops::{Deref, DerefMut};
use std::sync::Arc;

#[derive(Default, Debug)]
pub struct Shared<T>(Arc<T>);

impl<T> Clone for Shared<T> {
    fn clone(&self) -> Self {
        Shared(self.0.clone())
    }
}

impl<T> Shared<T> {
    /// Whether both share the same table (no write since one was copied from the other).
    pub fn same(&self, other: &Shared<T>) -> bool {
        Arc::ptr_eq(&self.0, &other.0)
    }
}

impl<T> From<T> for Shared<T> {
    fn from(v: T) -> Self {
        Shared(Arc::new(v))
    }
}

impl<T> Deref for Shared<T> {
    type Target = T;
    fn deref(&self) -> &T {
        &self.0
    }
}

impl<T: Clone> DerefMut for Shared<T> {
    fn deref_mut(&mut self) -> &mut T {
        Arc::make_mut(&mut self.0)
    }
}

impl<'a, T> IntoIterator for &'a Shared<T>
where
    &'a T: IntoIterator,
{
    type Item = <&'a T as IntoIterator>::Item;
    type IntoIter = <&'a T as IntoIterator>::IntoIter;
    fn into_iter(self) -> Self::IntoIter {
        (&*self.0).into_iter()
    }
}

impl<T: serde::Serialize> serde::Serialize for Shared<T> {
    fn serialize<S: serde::Serializer>(&self, s: S) -> Result<S::Ok, S::Error> {
        self.0.serialize(s)
    }
}

impl<A, T: FromIterator<A>> FromIterator<A> for Shared<T> {
    fn from_iter<I: IntoIterator<Item = A>>(iter: I) -> Self {
        Shared(Arc::new(T::from_iter(iter)))
    }
}
