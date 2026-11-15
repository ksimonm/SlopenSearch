//! Top-k selection helpers for searches whose response only reads one page.

use std::cmp::Ordering;

/// A scored hit (score, index-order position) ordered by how much *worse* it
/// is: lower score, then later position, is greater. A max-heap of these keeps
/// its worst kept hit on top. Scores must not be NaN.
#[derive(Clone, Copy)]
pub struct Worse(pub f64, pub usize);

impl PartialEq for Worse {
    fn eq(&self, o: &Self) -> bool {
        self.cmp(o) == Ordering::Equal
    }
}

impl Eq for Worse {}

impl PartialOrd for Worse {
    fn partial_cmp(&self, o: &Self) -> Option<Ordering> {
        Some(self.cmp(o))
    }
}

impl Ord for Worse {
    fn cmp(&self, o: &Self) -> Ordering {
        // as the stable score sort compares (-0.0 == 0.0)
        o.0.partial_cmp(&self.0).unwrap_or(Ordering::Equal).then(self.1.cmp(&o.1))
    }
}
