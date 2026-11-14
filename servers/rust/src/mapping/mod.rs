//! Mappings and field types: validation, merging, dynamic type inference.

use serde_json::{json, Map, Value};

use crate::dates::parse_date_string;

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
