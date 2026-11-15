//! Dates as OpenSearch handles them: epoch values, ISO date strings, civil
//! calendar arithmetic and Java date format patterns.

use serde_json::Value;

use crate::jsonv::to_f64;
pub(crate) use crate::calendar::civil_from_days;
use crate::calendar::days_from_civil;

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

pub(crate) fn date_parts(ms: i64) -> (i64, i64, i64, i64, i64, i64, i64) {
    let secs_total = ms.div_euclid(1000);
    let millis = ms.rem_euclid(1000);
    let days = secs_total.div_euclid(86400);
    let rem = secs_total.rem_euclid(86400);
    let (y, mo, d) = civil_from_days(days);
    (y, mo, d, rem / 3600, (rem % 3600) / 60, rem % 60, millis)
}

pub(crate) fn format_epoch_ms(ms: i64) -> String {
    let (y, mo, d, hh, mi, ss, millis) = date_parts(ms);
    let ys = if y < 0 {
        format!("-{:04}", -y)
    } else {
        format!("{:04}", y)
    };
    format!(
        "{}-{:02}-{:02}T{:02}:{:02}:{:02}.{:03}Z",
        ys, mo, d, hh, mi, ss, millis
    )
}

pub(crate) fn apply_java_date_format(f: &str, ms: i64) -> Option<String> {
    let excluded = [
        "epoch_millis",
        "epoch_second",
        "date_optional_time",
        "strict_date_optional_time",
        "strict_date_optional_time_nanos",
        "date_time",
        "basic_date_time",
        "strict_date_optional_time||epoch_millis",
    ];
    if f.is_empty() || excluded.contains(&f) {
        return None;
    }
    let (y, mo, d, hh, mi, ss, millis) = date_parts(ms);
    let mut out = f.replace("'T'", "T");
    out = out.replace("SSSSSSSSS", &format!("{:09}", millis * 1_000_000));
    out = out.replace("SSS", &format!("{:03}", millis));
    out = out.replace("yyyy", &format!("{:04}", y));
    out = out.replace("yy", &format!("{:02}", y.rem_euclid(100)));
    out = out.replace("MM", &format!("{:02}", mo));
    out = out.replace("dd", &format!("{:02}", d));
    out = out.replace("HH", &format!("{:02}", hh));
    out = out.replace("mm", &format!("{:02}", mi));
    out = out.replace("ss", &format!("{:02}", ss));
    Some(out)
}
