package geohash

import "testing"

// Expected values come from tests/reference/test_38_parity_ingest_geo.jsonl
// (OpenSearch 3.9): geohash points are the bottom-left corner of their cell,
// grid ties are ordered by the signed long hash.

func TestPoint(t *testing.T) {
	tests := []struct {
		hash     string
		lat, lon float64
	}{
		{"u4", 56.25, 0},
		{"s", 0, 0},
		{"u4pruydqqvj", 57.64910995960236, 10.407439023256302},
		// at most 12 characters are used
		{"u4pruydqqvj8x", 57.649109959602356, 10.407439693808556},
	}
	for _, tt := range tests {
		lat, lon, err := Point(tt.hash)
		if err != "" {
			t.Fatalf("%s: %s", tt.hash, err)
		}
		// docvalues are Lucene-quantized; the corner must be within a few ulps
		if d := lat - tt.lat; d > 1e-6 || d < -1e-6 {
			t.Errorf("%s: lat %v, want %v", tt.hash, lat, tt.lat)
		}
		if d := lon - tt.lon; d > 1e-6 || d < -1e-6 {
			t.Errorf("%s: lon %v, want %v", tt.hash, lon, tt.lon)
		}
	}
}

func TestPointErrors(t *testing.T) {
	tests := map[string]string{
		"":             "empty geohash",
		"DRM3BTEV3E86": "unsupported symbol [D] in geohash [DRM3BTEV3E86]",
		"u4pruydqqvjA": "unsupported symbol [A] in geohash [u4pruydqqvjA]",
		"abc":          "unsupported symbol [a] in geohash [abc]",
		"u4!":          "unsupported symbol [!] in geohash [u4!]",
	}
	for in, want := range tests {
		if _, _, err := Point(in); err != want {
			t.Errorf("Point(%q) = %q, want %q", in, err, want)
		}
	}
}

func TestEncode(t *testing.T) {
	if got := Encode(40.7128, -74.006, 12); got != "dr5regw3ppyz" {
		t.Errorf("Encode = %s", got)
	}
	if got := Encode(-0.0001, 179.9999, 12); got != "rzzzzzzzmtm7" {
		t.Errorf("Encode = %s", got)
	}
}

func TestLongEncodeOrder(t *testing.T) {
	// geohash_grid precision 12, equal counts, in OpenSearch's order
	order := []string{"dr5regw3ppyz", "dr5regw3pg6f", "u4pruydqqvj8", "u4pruydqqvj0",
		"u40000000000", "s00000000000", "rzzzzzzzmtm7"}
	for i := 1; i < len(order); i++ {
		if LongEncode(order[i-1]) <= LongEncode(order[i]) {
			t.Errorf("%s should sort before %s", order[i-1], order[i])
		}
	}
}

func TestNeighbors(t *testing.T) {
	n := Neighbors("u33d")
	if len(n) != 8 {
		t.Fatalf("neighbors %v", n)
	}
	for _, h := range n {
		if len(h) != 4 || h == "u33d" {
			t.Errorf("bad neighbor %s", h)
		}
	}
}
