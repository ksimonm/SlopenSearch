package mapping

// EncodeGeohash encodes a point as a geohash of the given precision.
func EncodeGeohash(lat, lon float64, precision int) string {
	const base32 = "0123456789bcdefghjkmnpqrstuvwxyz"
	latLo, latHi := -90.0, 90.0
	lonLo, lonHi := -180.0, 180.0
	out := []byte{}
	bit, ch := 0, 0
	even := true
	for len(out) < precision {
		if even {
			mid := (lonLo + lonHi) / 2
			if lon >= mid {
				ch = ch<<1 | 1
				lonLo = mid
			} else {
				ch <<= 1
				lonHi = mid
			}
		} else {
			mid := (latLo + latHi) / 2
			if lat >= mid {
				ch = ch<<1 | 1
				latLo = mid
			} else {
				ch <<= 1
				latHi = mid
			}
		}
		even = !even
		bit++
		if bit == 5 {
			out = append(out, base32[ch])
			bit, ch = 0, 0
		}
	}
	return string(out)
}
