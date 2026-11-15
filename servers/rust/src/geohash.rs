//! Geohashes (base-32, longitude bit first), as Lucene's `Geohash` encodes them.

pub const CHARS: &[u8] = b"0123456789bcdefghjkmnpqrstuvwxyz";

/// The geohash of `precision` characters of the cell holding (lat, lon).
pub fn encode(lat: f64, lon: f64, precision: usize) -> String {
    let (mut lat_lo, mut lat_hi, mut lon_lo, mut lon_hi) = (-90.0f64, 90.0f64, -180.0f64, 180.0f64);
    let mut out = String::with_capacity(precision);
    let (mut bits, mut nbits, mut even) = (0u32, 0, true);
    while out.len() < precision {
        if even {
            let mid = (lon_lo + lon_hi) / 2.0;
            if lon >= mid {
                bits = (bits << 1) | 1;
                lon_lo = mid;
            } else {
                bits <<= 1;
                lon_hi = mid;
            }
        } else {
            let mid = (lat_lo + lat_hi) / 2.0;
            if lat >= mid {
                bits = (bits << 1) | 1;
                lat_lo = mid;
            } else {
                bits <<= 1;
                lat_hi = mid;
            }
        }
        even = !even;
        nbits += 1;
        if nbits == 5 {
            out.push(CHARS[bits as usize] as char);
            bits = 0;
            nbits = 0;
        }
    }
    out
}

/// Bounds (lat_lo, lat_hi, lon_lo, lon_hi) of a geohash cell (case-insensitive); the
/// empty hash is the whole world, an invalid character gives `None`.
pub fn bounds(s: &str) -> Option<(f64, f64, f64, f64)> {
    let (mut lat_lo, mut lat_hi, mut lon_lo, mut lon_hi) = (-90.0f64, 90.0f64, -180.0f64, 180.0f64);
    let mut even = true;
    for ch in s.chars() {
        let idx = CHARS.iter().position(|c| *c as char == ch.to_ascii_lowercase())?;
        for shift in (0..5).rev() {
            let bit = (idx >> shift) & 1 == 1;
            if even {
                let mid = (lon_lo + lon_hi) / 2.0;
                if bit {
                    lon_lo = mid
                } else {
                    lon_hi = mid
                }
            } else {
                let mid = (lat_lo + lat_hi) / 2.0;
                if bit {
                    lat_lo = mid
                } else {
                    lat_hi = mid
                }
            }
            even = !even;
        }
    }
    Some((lat_lo, lat_hi, lon_lo, lon_hi))
}

/// Center (lat, lon) of a non-empty geohash cell.
pub fn decode(s: &str) -> Option<(f64, f64)> {
    if s.is_empty() {
        return None;
    }
    let (a, b, c, d) = bounds(s)?;
    Some(((a + b) / 2.0, (c + d) / 2.0))
}

/// The (up to eight) cells of the same precision around a geohash cell.
pub fn neighbors(gh: &str) -> Vec<String> {
    let Some((a, b, c, d)) = bounds(gh) else { return vec![] };
    let (lat, lon) = ((a + b) / 2.0, (c + d) / 2.0);
    let (dlat, dlon) = (b - a, d - c);
    let mut out = vec![];
    for i in -1i32..=1 {
        for j in -1i32..=1 {
            if i == 0 && j == 0 {
                continue;
            }
            let nlat = lat + i as f64 * dlat;
            if !(-90.0..=90.0).contains(&nlat) {
                continue;
            }
            let mut nlon = lon + j as f64 * dlon;
            if nlon > 180.0 {
                nlon -= 360.0;
            }
            if nlon < -180.0 {
                nlon += 360.0;
            }
            out.push(encode(nlat, nlon, gh.len()));
        }
    }
    out
}
