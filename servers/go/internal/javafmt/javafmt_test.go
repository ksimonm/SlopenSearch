package javafmt

import (
	"math"
	"testing"
)

// Expected strings are what OpenSearch 3.9 (JDK 21+) writes for these values.
func TestDouble(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0.0"}, {math.Copysign(0, -1), "-0.0"}, {1, "1.0"}, {-2.5, "-2.5"},
		{1e7, "1.0E7"}, {9999999, "9999999.0"}, {1.5e20, "1.5E20"}, {1e-4, "1.0E-4"}, {0.001, "0.001"},
		{123456789.125, "1.23456789125E8"}, {99.99000000000001, "99.99000000000001"},
		{3.750000000003337e19, "3.750000000003337E19"}, {2.25e40, "2.25E40"},
		{float64(float32(1e-4)), "9.999999747378752E-5"}, {1.0001659393310547e-4, "1.0001659393310547E-4"},
		{2e22, "2.0E22"}, {math.MaxFloat64, "1.7976931348623157E308"}, {5e-324, "4.9E-324"},
		{math.NaN(), "NaN"}, {math.Inf(1), "Infinity"}, {math.Inf(-1), "-Infinity"},
	}
	for _, tt := range tests {
		if got := Double(tt.in); got != tt.want {
			t.Errorf("Double(%v) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestFloat(t *testing.T) {
	tests := []struct {
		in   float32
		want string
	}{
		{1e7, "1.0E7"}, {1e-4, "1.0E-4"}, {1.5e20, "1.5E20"}, {3.4e38, "3.4E38"}, {0.5, "0.5"},
		{1.00016594e-4, "1.00016594E-4"}, {65504, "65504.0"}, {1000.5, "1000.5"}, {1e17, "1.0E17"},
		{0.2876821, "0.2876821"},
	}
	for _, tt := range tests {
		if got := Float(tt.in); got != tt.want {
			t.Errorf("Float(%v) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
