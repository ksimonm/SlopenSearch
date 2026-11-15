package aggs

import "testing"

func TestKahanSum(t *testing.T) {
	v := []float64{0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1}
	if got := kahanSum(v); got != 1.0 {
		t.Fatalf("kahanSum = %v, want 1", got)
	}
	if got := sumOf(v); got == 1.0 {
		t.Fatalf("plain sum unexpectedly exact")
	}
}
