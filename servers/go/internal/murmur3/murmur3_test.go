package murmur3

import "testing"

func TestSum32(t *testing.T) {
	// Lucene StringHelper.murmurhash3_x86_32 reference values.
	tests := []struct {
		in   string
		seed int32
		want int32
	}{
		{"", 0, 0},
		{"hello", 0, 0x248bfa47},
		{"The quick brown fox jumps over the lazy dog", 0, 0x2e4ff723},
	}
	for _, tt := range tests {
		if got := Sum32([]byte(tt.in), tt.seed); got != tt.want {
			t.Errorf("Sum32(%q, %d) = %#x, want %#x", tt.in, tt.seed, got, tt.want)
		}
	}
	// UTF-16LE: "a" hashes the bytes 61 00
	if HashRouting("a") != Sum32([]byte{0x61, 0}, 0) {
		t.Error("HashRouting does not hash UTF-16LE")
	}
	if HashRouting("😀") != Sum32([]byte{0x3d, 0xd8, 0x00, 0xde}, 0) {
		t.Error("HashRouting does not hash surrogate pairs")
	}
}
