// Package murmur3 is MurmurHash3 x86_32 as Lucene's StringHelper and
// OpenSearch's Murmur3HashFunction compute it (signed 32-bit results).
package murmur3

import "unicode/utf16"

// Sum32 is murmurhash3_x86_32 of data with seed.
func Sum32(data []byte, seed int32) int32 {
	const c1 int32 = -862048943
	const c2 int32 = 0x1b873593
	h := seed
	n := len(data) &^ 3
	for i := 0; i < n; i += 4 {
		k := int32(data[i]) | int32(data[i+1])<<8 | int32(data[i+2])<<16 | int32(data[i+3])<<24
		k *= c1
		k = int32(uint32(k)<<15 | uint32(k)>>17)
		k *= c2
		h ^= k
		h = int32(uint32(h)<<13 | uint32(h)>>19)
		h = int32(uint32(h)*5 + 0xe6546b64)
	}
	var k int32
	switch len(data) & 3 {
	case 3:
		k ^= int32(data[n+2]) << 16
		fallthrough
	case 2:
		k ^= int32(data[n+1]) << 8
		fallthrough
	case 1:
		k ^= int32(data[n])
		k *= c1
		k = int32(uint32(k)<<15 | uint32(k)>>17)
		k *= c2
		h ^= k
	}
	h ^= int32(len(data))
	h ^= int32(uint32(h) >> 16)
	h *= -2048144789
	h ^= int32(uint32(h) >> 13)
	h *= -1028477387
	h ^= int32(uint32(h) >> 16)
	return h
}

// HashRouting is Murmur3HashFunction.hash(String): the hash (seed 0) of the
// UTF-16 code units of s, each written low byte first.
func HashRouting(s string) int32 {
	units := utf16.Encode([]rune(s))
	b := make([]byte, 0, 2*len(units))
	for _, u := range units {
		b = append(b, byte(u), byte(u>>8))
	}
	return Sum32(b, 0)
}
