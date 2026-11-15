package aggs

import "math"

// compensatedSum is OpenSearch's CompensatedSum: the Kahan summation the sum,
// avg, stats, extended_stats and weighted_avg aggregators accumulate doubles
// with. Pipeline aggregations add plainly.
type compensatedSum struct {
	value, delta float64
}

func (s *compensatedSum) add(v float64) {
	// a non-finite value turns the running tally into Inf/NaN
	if math.IsInf(v, 0) || math.IsNaN(v) {
		s.value += v
	}
	if !math.IsInf(s.value, 0) && !math.IsNaN(s.value) {
		corrected := v + s.delta
		updated := s.value + corrected
		s.delta = corrected - (updated - s.value)
		s.value = updated
	}
}

func kahanSum(vals []float64) float64 {
	var s compensatedSum
	for _, v := range vals {
		s.add(v)
	}
	return s.value
}

func kahanMean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	return kahanSum(vals) / float64(len(vals))
}
