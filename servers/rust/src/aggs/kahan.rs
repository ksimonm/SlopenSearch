//! OpenSearch's CompensatedSum: the Kahan summation the sum, avg, stats,
//! extended_stats and weighted_avg aggregators accumulate their doubles with.

#[derive(Default, Clone, Copy)]
pub struct CompensatedSum {
    value: f64,
    delta: f64,
}

impl CompensatedSum {
    pub fn add(&mut self, v: f64) {
        // a non-finite value turns the running tally into Inf/NaN
        if !v.is_finite() {
            self.value += v;
        }
        if self.value.is_finite() {
            let corrected = v + self.delta;
            let updated = self.value + corrected;
            self.delta = corrected - (updated - self.value);
            self.value = updated;
        }
    }

    pub fn value(&self) -> f64 {
        self.value
    }
}

/// The compensated sum of `vals`.
pub fn sum(vals: impl IntoIterator<Item = f64>) -> f64 {
    let mut s = CompensatedSum::default();
    for v in vals {
        s.add(v);
    }
    s.value()
}

#[cfg(test)]
mod tests {
    #[test]
    fn compensates() {
        let v = [0.1; 10];
        assert_eq!(super::sum(v), 1.0);
        assert_ne!(v.iter().sum::<f64>(), 1.0);
    }
}
