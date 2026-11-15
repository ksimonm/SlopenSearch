// Package geohash implements geohashes (base 32, longitude bit first) as
// Lucene's Geohash and OpenSearch's GeoPoint read and write them.
package geohash

import "strings"

// Base32 is the geohash alphabet; a character's index is its 5 bits.
const Base32 = "0123456789bcdefghjkmnpqrstuvwxyz"

// MaxPrecision is the longest geohash a point is read from (longer hashes
// are accepted, their extra characters ignored).
const MaxPrecision = 12

// Encode returns the geohash of precision characters of the cell holding
// (lat, lon).
func Encode(lat, lon float64, precision int) string {
	latLo, latHi := -90.0, 90.0
	lonLo, lonHi := -180.0, 180.0
	out := make([]byte, 0, precision)
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
			out = append(out, Base32[ch])
			bit, ch = 0, 0
		}
	}
	return string(out)
}

// Cell is the bounding box of a geohash cell.
type Cell struct{ LatLo, LatHi, LonLo, LonHi float64 }

// Bounds returns the cell of a geohash (letters are case-sensitive); the
// empty hash is the whole world. bad is the first character that is not a
// geohash character (ok is false then).
func Bounds(s string) (c Cell, bad rune, ok bool) {
	c = Cell{-90, 90, -180, 180}
	even := true
	for _, r := range s {
		idx := strings.IndexRune(Base32, r)
		if idx < 0 {
			return c, r, false
		}
		for shift := 4; shift >= 0; shift-- {
			one := (idx>>shift)&1 == 1
			if even {
				mid := (c.LonLo + c.LonHi) / 2
				if one {
					c.LonLo = mid
				} else {
					c.LonHi = mid
				}
			} else {
				mid := (c.LatLo + c.LatHi) / 2
				if one {
					c.LatLo = mid
				} else {
					c.LatHi = mid
				}
			}
			even = !even
		}
	}
	return c, 0, true
}

// Center returns the center (lat, lon) of a geohash cell.
func Center(s string) (lat, lon float64, ok bool) {
	c, _, ok := Bounds(s)
	if !ok || s == "" {
		return 0, 0, false
	}
	return (c.LatLo + c.LatHi) / 2, (c.LonLo + c.LonHi) / 2, true
}

// Point is the point a geohash stands for where OpenSearch reads a geo point
// (GeoPoint#resetFromGeoHash): the bottom-left corner of its cell, of at most
// MaxPrecision characters. err is "empty geohash" or "unsupported symbol [c]
// in geohash [s]".
func Point(s string) (lat, lon float64, err string) {
	if s == "" {
		return 0, 0, "empty geohash"
	}
	if _, bad, ok := Bounds(s); !ok {
		return 0, 0, "unsupported symbol [" + string(bad) + "] in geohash [" + s + "]"
	}
	cell := s
	if len(cell) > MaxPrecision {
		cell = cell[:MaxPrecision]
	}
	c, _, _ := Bounds(cell)
	return c.LatLo, c.LonLo, ""
}

// Decode is Point for callers that only need to know whether s is a geohash.
func Decode(s string) (lat, lon float64, ok bool) {
	lat, lon, err := Point(s)
	return lat, lon, err == ""
}

// LongEncode is Lucene's Geohash#longEncode of a geohash: its bits followed
// by 4 bits of length (12 characters use all 64 bits). Bucket ties of
// geohash_grid are ordered by it.
func LongEncode(hash string) int64 {
	if len(hash) > MaxPrecision {
		hash = hash[:MaxPrecision]
	}
	var l uint64
	for i := 0; i < len(hash); i++ {
		b := strings.IndexByte(Base32, hash[i])
		if b < 0 {
			b = 0
		}
		l = l<<5 | uint64(b)
	}
	return int64(l<<4 | uint64(len(hash)))
}

// Neighbors returns the (up to eight) cells of the same precision around a
// geohash cell.
func Neighbors(hash string) []string {
	c, _, ok := Bounds(hash)
	if !ok || hash == "" {
		return nil
	}
	lat, lon := (c.LatLo+c.LatHi)/2, (c.LonLo+c.LonHi)/2
	w, h := c.LonHi-c.LonLo, c.LatHi-c.LatLo
	var out []string
	for _, dy := range []float64{-1, 0, 1} {
		for _, dx := range []float64{-1, 0, 1} {
			if dx == 0 && dy == 0 {
				continue
			}
			nlat := lat + dy*h
			if nlat > 90 || nlat < -90 {
				continue
			}
			nlon := lon + dx*w
			if nlon >= 180 {
				nlon -= 360
			} else if nlon < -180 {
				nlon += 360
			}
			out = append(out, Encode(nlat, nlon, len(hash)))
		}
	}
	return out
}
