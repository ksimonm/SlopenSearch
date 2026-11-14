//! Helpers over decoded JSON values (serde_json `Value`): numbers, scalar
//! strings, dotted paths.

use serde_json::Value;

use crate::resp::format_number;

pub fn stringify_scalar(v: &Value) -> Value {
    match v {
        Value::Number(n) => {
            if let Some(i) = n.as_i64() {
                Value::String(i.to_string())
            } else if let Some(u) = n.as_u64() {
                Value::String(u.to_string())
            } else if let Some(f) = n.as_f64() {
                Value::String(format_number(f))
            } else {
                v.clone()
            }
        }
        Value::Bool(b) => Value::String(b.to_string()),
        other => other.clone(),
    }
}

pub fn to_f64(v: &Value) -> Option<f64> {
    match v {
        Value::Number(n) => n.as_f64(),
        Value::String(s) => s.trim().parse::<f64>().ok(),
        Value::Bool(b) => Some(if *b { 1.0 } else { 0.0 }),
        _ => None,
    }
}

pub fn value_to_string(v: &Value) -> Option<String> {
    match v {
        Value::String(s) => Some(s.clone()),
        Value::Number(n) => {
            if let Some(i) = n.as_i64() {
                Some(i.to_string())
            } else if let Some(u) = n.as_u64() {
                Some(u.to_string())
            } else if let Some(f) = n.as_f64() {
                Some(format_number(f))
            } else {
                Some(n.to_string())
            }
        }
        Value::Bool(b) => Some(b.to_string()),
        _ => None,
    }
}

pub fn doc_strings(v: &Value) -> Vec<String> {
    match v {
        Value::Array(a) => a.iter().filter_map(value_to_string).collect(),
        Value::Null => vec![],
        other => value_to_string(other).into_iter().collect(),
    }
}

fn value_at_path<'a>(doc: &'a Value, path: &str) -> Option<&'a Value> {
    let mut cur = doc;
    for part in path.split('.') {
        cur = cur.as_object()?.get(part)?;
    }
    Some(cur)
}

pub fn field_value<'a>(doc: &'a Value, path: &str) -> Option<&'a Value> {
    if let Some(v) = value_at_path(doc, path) {
        return Some(v);
    }
    let parts: Vec<&str> = path.split('.').collect();
    for i in (1..parts.len()).rev() {
        if let Some(v) = value_at_path(doc, &parts[..i].join(".")) {
            return Some(v);
        }
    }
    None
}
