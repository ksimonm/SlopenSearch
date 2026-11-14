//! `human=true` rendering: human readable companions written before the raw
//! `*_in_bytes` / `*_in_millis` stats fields and the index date/version settings.

use serde_json::{Map, Value};

/// Strings.format1Decimals: one truncated decimal, dropped when it is zero.
fn format1(value: f64, suffix: &str) -> String {
    let p = format!("{:?}", value);
    match p.find('.') {
        Some(ix) => {
            let frac = p.as_bytes().get(ix + 1).copied().unwrap_or(b'0');
            if frac == b'0' || !frac.is_ascii_digit() {
                format!("{}{}", &p[..ix], suffix)
            } else {
                format!("{}{}{}", &p[..ix], frac as char, suffix)
            }
        }
        None => format!("{}{}", p, suffix),
    }
}

/// ByteSizeValue.toString.
pub fn bytes_string(b: i64) -> String {
    let units: [(f64, &str); 5] = [
        (1125899906842624.0, "pb"),
        (1099511627776.0, "tb"),
        (1073741824.0, "gb"),
        (1048576.0, "mb"),
        (1024.0, "kb"),
    ];
    let f = b as f64;
    for (size, suffix) in units {
        if f >= size {
            return format1(f / size, suffix);
        }
    }
    format!("{}b", b)
}

/// TimeValue.toString for a millisecond duration.
pub fn millis_string(ms: i64) -> String {
    if ms < 0 {
        return ms.to_string();
    }
    if ms == 0 {
        return "0s".to_string();
    }
    let f = ms as f64;
    let units: [(f64, &str); 4] = [
        (86_400_000.0, "d"),
        (3_600_000.0, "h"),
        (60_000.0, "m"),
        (1000.0, "s"),
    ];
    for (size, suffix) in units {
        if f >= size {
            return format1(f / size, suffix);
        }
    }
    format!("{}ms", ms)
}

/// Adds `x` before every `x_in_bytes` and `x_in_millis` field.
pub fn humanize_stats(v: &mut Value) {
    match v {
        Value::Object(m) => {
            let old = std::mem::take(m);
            for (k, mut val) in old {
                humanize_stats(&mut val);
                if let Some(n) = val.as_i64() {
                    if let Some(base) = k.strip_suffix("_in_bytes") {
                        if !m.contains_key(base) {
                            m.insert(base.to_string(), Value::String(bytes_string(n)));
                        }
                    } else if let Some(base) = k.strip_suffix("_in_millis") {
                        if !m.contains_key(base) {
                            m.insert(base.to_string(), Value::String(millis_string(n)));
                        }
                    }
                }
                m.insert(k, val);
            }
        }
        Value::Array(a) => a.iter_mut().for_each(humanize_stats),
        _ => {}
    }
}

fn millis_of(v: &Value) -> Option<i64> {
    match v {
        Value::String(s) => s.parse().ok(),
        v => v.as_i64(),
    }
}

fn humanize_index_settings(m: &mut Map<String, Value>, prefix: &str) {
    let old = std::mem::take(m);
    for (k, mut val) in old {
        let full = format!("{}{}", prefix, k);
        if full == "index.creation_date" {
            if let Some(ms) = millis_of(&val) {
                m.insert(
                    format!("{}_string", k),
                    Value::String(crate::cluster::format_utc_millis(ms)),
                );
            }
        } else if full == "index.version.created" || full == "index.version.upgraded" {
            m.insert(
                format!("{}_string", k),
                Value::String(crate::store::NODE_VERSION.to_string()),
            );
        } else if let Value::Object(inner) = &mut val {
            if full == "index" || full == "index.version" {
                humanize_index_settings(inner, &format!("{}.", full));
            }
        }
        m.insert(k, val);
    }
}

/// Adds `creation_date_string` and `version.created_string` to a get-settings
/// response (`{index: {settings: ...}}`), nested or flat.
pub fn humanize_settings(v: &mut Value) {
    if let Some(indices) = v.as_object_mut() {
        for (_, iv) in indices.iter_mut() {
            for part in ["settings", "defaults"] {
                if let Some(s) = iv.get_mut(part).and_then(|s| s.as_object_mut()) {
                    humanize_index_settings(s, "");
                }
            }
        }
    }
}
