//! Mappings and field types: validation, merging, dynamic type inference.

use serde_json::{json, Map, Value};

pub(crate) mod completion;
pub(crate) mod fields;
pub(crate) mod geo;
pub(crate) mod knn;
pub(crate) mod knn_index;
pub(crate) mod nonstd_json;

use crate::dates::parse_date_string;
use crate::model::Index;

pub fn valid_type(t: &str) -> bool {
    matches!(
        t,
        "text" | "keyword" | "integer" | "long" | "short" | "byte" | "float" | "double"
            | "half_float" | "scaled_float" | "unsigned_long" | "date" | "date_nanos"
            | "boolean" | "object" | "nested" | "completion" | "geo_point" | "geo_shape"
            | "ip" | "binary" | "wildcard" | "constant_keyword" | "flat_object"
            | "token_count" | "search_as_you_type" | "rank_feature" | "rank_features"
            | "match_only_text" | "percolator" | "join" | "alias" | "version"
            | "integer_range" | "float_range" | "long_range" | "double_range"
            | "date_range" | "ip_range" | "xy_point" | "knn_vector"
    )
}

pub fn infer_type(v: &Value) -> Option<Value> {
    match v {
        Value::Bool(_) => Some(json!({"type": "boolean"})),
        Value::Number(n) => {
            // the JSON token decides: `3.0` maps as float, `3` as long
            n.as_f64()?;
            if !n.is_f64() {
                Some(json!({"type": "long"}))
            } else {
                Some(json!({"type": "float"}))
            }
        }
        Value::String(s) => {
            if parse_date_string(s).is_some() {
                Some(json!({"type": "date"}))
            } else {
                Some(json!({
                    "type": "text",
                    "fields": {"keyword": {"type": "keyword", "ignore_above": 256}}
                }))
            }
        }
        Value::Array(a) => a
            .iter()
            .find_map(|e| if e.is_null() { None } else { infer_type(e) }),
        Value::Object(m) => {
            let mut props = Map::new();
            for (k, val) in m {
                if let Some(t) = infer_type(val) {
                    props.insert(k.clone(), t);
                }
            }
            Some(json!({"properties": Value::Object(props)}))
        }
        _ => None,
    }
}

pub fn ensure_properties(mappings: &mut Value) -> &mut Map<String, Value> {
    let obj = mappings.as_object_mut().unwrap();
    if !obj.get("properties").map(|p| p.is_object()).unwrap_or(false) {
        obj.insert("properties".to_string(), Value::Object(Map::new()));
    }
    obj.get_mut("properties").unwrap().as_object_mut().unwrap()
}

pub fn merge_mapping(props: &mut Map<String, Value>, src: &Map<String, Value>) {
    for (k, v) in src {
        if k.starts_with('_') {
            continue;
        }
        if !props.contains_key(k) {
            if let Some(t) = infer_type(v) {
                props.insert(k.clone(), t);
            }
            continue;
        }
        let sub = props
            .get(k)
            .and_then(|p| p.get("properties"))
            .and_then(|p| p.as_object())
            .cloned();
        if let Some(mut sub) = sub {
            match v {
                Value::Object(m) => merge_mapping(&mut sub, m),
                Value::Array(a) => {
                    for e in a {
                        if let Some(m) = e.as_object() {
                            merge_mapping(&mut sub, m);
                        }
                    }
                }
                _ => {}
            }
            if let Some(obj) = props.get_mut(k).and_then(|p| p.as_object_mut()) {
                obj.insert("properties".to_string(), Value::Object(sub));
            }
        }
    }
}

pub fn resolve_alias_field(idx: &Index, field: &str) -> String {
    let mut field = field.to_string();
    for _ in 0..10 {
        let def = match mapping_field_def(idx, &field) {
            Some(d) => d,
            None => return field,
        };
        if def.get("type").and_then(|t| t.as_str()) == Some("alias") {
            let path = def.get("path").and_then(|p| p.as_str()).unwrap_or("");
            if path.is_empty() || path == field {
                return field;
            }
            field = path.to_string();
            continue;
        }
        return field;
    }
    field
}

pub fn mapping_field_def<'a>(idx: &'a Index, field: &str) -> Option<&'a Map<String, Value>> {
    if let Some(d) = idx
        .mappings
        .get("derived")
        .and_then(|m| m.get(field))
        .and_then(|d| d.as_object())
    {
        return Some(d);
    }
    let props = idx.mappings.get("properties")?.as_object()?;
    let parts: Vec<&str> = field.split('.').collect();
    let mut cur = props;
    for i in 0..parts.len() {
        if i + 1 < parts.len() {
            let joined = parts[i..].join(".");
            if let Some(def) = cur.get(&joined).and_then(|d| d.as_object()) {
                return Some(def);
            }
        }
        let def = cur.get(parts[i])?.as_object()?;
        if i == parts.len() - 1 {
            return Some(def);
        }
        let rest = parts[i + 1..].join(".");
        if let Some(sub) = def.get("fields").and_then(|f| f.as_object()) {
            if let Some(sd) = sub.get(&rest).and_then(|d| d.as_object()) {
                return Some(sd);
            }
        }
        cur = def.get("properties")?.as_object()?;
    }
    None
}
