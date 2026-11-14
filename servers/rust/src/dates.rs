//! Dates as OpenSearch handles them: epoch values, ISO date strings, civil
//! calendar arithmetic and Java date format patterns.

use serde_json::Value;

use crate::jsonv::to_f64;

fn days_from_civil(y: i64, m: i64, d: i64) -> i64 {
    let y = if m <= 2 { y - 1 } else { y };
    let era = if y >= 0 { y } else { y - 399 } / 400;
    let yoe = y - era * 400;
    let doy = (153 * (if m > 2 { m - 3 } else { m + 9 }) + 2) / 5 + d - 1;
    let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    era * 146097 + doe - 719468
}

pub fn parse_date_string(s: &str) -> Option<f64> {
    let s = s.trim();
    let bytes = s.as_bytes();
    if bytes.len() < 10 {
        return None;
    }
    if bytes[4] != b'-' || bytes[7] != b'-' {
        return None;
    }
    let y = s[0..4].parse::<i64>().ok()?;
    let mo = s[5..7].parse::<i64>().ok()?;
    let d = s[8..10].parse::<i64>().ok()?;
    if !(1..=12).contains(&mo) || !(1..=31).contains(&d) {
        return None;
    }
    let (mut hh, mut mi, mut ss, mut ms) = (0i64, 0i64, 0i64, 0i64);
    if bytes.len() > 10 {
        if bytes[10] != b'T' && bytes[10] != b' ' {
            return None;
        }
        let rest = &s[11..];
        if rest.len() < 8 {
            return None;
        }
        if rest.as_bytes()[2] != b':' || rest.as_bytes()[5] != b':' {
            return None;
        }
        hh = rest[0..2].parse::<i64>().ok()?;
        mi = rest[3..5].parse::<i64>().ok()?;
        ss = rest[6..8].parse::<i64>().ok()?;
        if rest.len() > 8 && rest.as_bytes()[8] == b'.' {
            let frac = &rest[9..];
            let digits: String = frac.chars().take_while(|c| c.is_ascii_digit()).collect();
            if !digits.is_empty() {
                let mut v: i64 = digits.parse().ok()?;
                for _ in digits.len()..3 {
                    v *= 10;
                }
                for _ in 3..digits.len() {
                    v /= 10;
                }
                ms = v;
            }
        }
    }
    let days = days_from_civil(y, mo, d);
    Some(((days * 86400 + hh * 3600 + mi * 60 + ss) * 1000 + ms) as f64)
}

pub fn parse_date(v: &Value) -> Option<f64> {
    if let Some(f) = to_f64(v) {
        return Some(f);
    }
    parse_date_string(v.as_str()?)
}
