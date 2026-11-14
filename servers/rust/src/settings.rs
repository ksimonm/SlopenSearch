//! Index settings lookup: flat or nested keys, with or without the `index.`
//! prefix, and the string/int views OpenSearch reports for setting values.

use serde_json::{Map, Value};

use crate::jsonv::to_f64;
use crate::resp::format_number;

pub fn settings_value_string(v: &Value) -> String {
    match v {
        Value::String(s) => s.clone(),
        Value::Number(n) => {
            if let Some(i) = n.as_i64() {
                i.to_string()
            } else if let Some(u) = n.as_u64() {
                u.to_string()
            } else if let Some(f) = n.as_f64() {
                format_number(f)
            } else {
                n.to_string()
            }
        }
        Value::Bool(b) => b.to_string(),
        Value::Null => "null".to_string(),
        other => other.to_string(),
    }
}

pub fn settings_value_int(v: &Value) -> Option<i64> {
    match v {
        Value::Number(n) => {
            if let Some(i) = n.as_i64() {
                Some(i)
            } else {
                let f = n.as_f64()?;
                if f == f.trunc() {
                    Some(f as i64)
                } else {
                    None
                }
            }
        }
        Value::String(s) => s.trim().parse::<i64>().ok(),
        _ => None,
    }
}

pub fn setting_int(settings: &Map<String, Value>, key: &str) -> Option<i64> {
    let v = find_setting(settings, key)?;
    to_f64(v).map(|f| f as i64)
}

pub fn find_setting<'a>(settings: &'a Map<String, Value>, key: &str) -> Option<&'a Value> {
    if let Some(v) = settings.get(&format!("index.{}", key)) {
        return Some(v);
    }
    if let Some(v) = settings.get(key) {
        return Some(v);
    }
    if let Some(Value::Object(m)) = settings.get("index") {
        if let Some(v) = m.get(key) {
            return Some(v);
        }
    }
    None
}
