//! `_source` filtering and stored fields.

use super::fetch::*;
use crate::search::eval::mapping_field_def;
use crate::model::Index;
use serde_json::{json, Map, Value};

pub(crate) fn collect_leaf_paths(v: &Value, prefix: &str, out: &mut Vec<String>) {
    match v {
        Value::Object(m) => {
            if m.is_empty() {
                if !prefix.is_empty() {
                    out.push(prefix.to_string());
                }
                return;
            }
            for (k, child) in m {
                let p = if prefix.is_empty() {
                    k.clone()
                } else {
                    format!("{}.{}", prefix, k)
                };
                if child.is_object() || child.is_array() {
                    collect_leaf_paths(child, &p, out);
                } else {
                    out.push(p);
                }
            }
        }
        Value::Array(a) => {
            if a.is_empty() {
                if !prefix.is_empty() {
                    out.push(prefix.to_string());
                }
                return;
            }
            for child in a {
                if child.is_object() || child.is_array() {
                    collect_leaf_paths(child, prefix, out);
                } else {
                    out.push(prefix.to_string());
                }
            }
        }
        _ => {
            if !prefix.is_empty() {
                out.push(prefix.to_string());
            }
        }
    }
}

pub(crate) fn segment_match(pattern: &str, value: &str) -> bool {
    if pattern == "*" {
        return true;
    }
    if !pattern.contains('*') && !pattern.contains('?') {
        return pattern == value;
    }
    let p: Vec<char> = pattern.chars().collect();
    let v: Vec<char> = value.chars().collect();
    let (mut i, mut j, mut star, mut mark) = (0usize, 0usize, None::<usize>, 0usize);
    while j < v.len() {
        if i < p.len() && (p[i] == '?' || p[i] == v[j]) {
            i += 1;
            j += 1;
        } else if i < p.len() && p[i] == '*' {
            star = Some(i);
            mark = j;
            i += 1;
        } else if let Some(s) = star {
            i = s + 1;
            mark += 1;
            j = mark;
        } else {
            return false;
        }
    }
    while i < p.len() && p[i] == '*' {
        i += 1;
    }
    i == p.len()
}

pub(crate) fn path_matches(pattern: &str, leaf: &str) -> bool {
    if pattern == "*" {
        return true;
    }
    let ps: Vec<&str> = pattern.split('.').collect();
    let ls: Vec<&str> = leaf.split('.').collect();
    for (i, p) in ps.iter().enumerate() {
        match ls.get(i) {
            Some(l) => {
                if !segment_match(p, l) {
                    return false;
                }
            }
            None => return false,
        }
    }
    true
}

pub(crate) fn merge_path(out: &mut Map<String, Value>, src: &Value, segs: &[String]) {
    if segs.is_empty() {
        return;
    }
    let joined = segs.join(".");
    if let Some(v) = src.as_object().and_then(|o| o.get(&joined)) {
        out.insert(joined, v.clone());
        return;
    }
    let seg = &segs[0];
    if segs.len() == 1 {
        if let Some(o) = src.as_object() {
            if let Some(v) = o.get(seg) {
                out.insert(seg.clone(), v.clone());
            }
        }
        return;
    }
    let Some(child) = src.as_object().and_then(|o| o.get(seg)) else {
        return;
    };
    if let Some(arr) = child.as_array() {
        if !out.get(seg).map(|v| v.is_array()).unwrap_or(false) {
            out.insert(seg.clone(), Value::Array(vec![]));
        }
        if let Some(out_arr) = out.get_mut(seg).and_then(|v| v.as_array_mut()) {
            if out_arr.len() < arr.len() {
                out_arr.resize(arr.len(), Value::Object(Map::new()));
            }
            for (i, e) in arr.iter().enumerate() {
                if e.is_object() {
                    if !out_arr[i].is_object() {
                        out_arr[i] = Value::Object(Map::new());
                    }
                    if let Some(m) = out_arr[i].as_object_mut() {
                        merge_path(m, e, &segs[1..]);
                    }
                }
            }
        }
    } else if child.is_object() {
        if !out.get(seg).map(|v| v.is_object()).unwrap_or(false) {
            out.insert(seg.clone(), Value::Object(Map::new()));
        }
        if let Some(m) = out.get_mut(seg).and_then(|v| v.as_object_mut()) {
            merge_path(m, child, &segs[1..]);
        }
    }
}

pub(crate) fn remove_path(out: &mut Map<String, Value>, segs: &[String]) {
    if segs.is_empty() {
        return;
    }
    let seg = &segs[0];
    if segs.len() == 1 {
        out.remove(seg);
        return;
    }
    match out.get_mut(seg) {
        Some(Value::Object(m)) => remove_path(m, &segs[1..]),
        Some(Value::Array(a)) => {
            for e in a.iter_mut() {
                if let Some(m) = e.as_object_mut() {
                    remove_path(m, &segs[1..]);
                }
            }
        }
        _ => {}
    }
}

pub(crate) fn mapping_source_spec(idx: &Index) -> Option<Value> {
    let src = idx.mappings.get("_source")?.as_object()?;
    let inc = src
        .get("includes")
        .or_else(|| src.get("include"))
        .cloned()
        .unwrap_or_else(|| json!([]));
    let exc = src
        .get("excludes")
        .or_else(|| src.get("exclude"))
        .cloned()
        .unwrap_or_else(|| json!([]));
    Some(json!({"includes": inc, "excludes": exc}))
}

pub(crate) fn source_enabled_search(idx: &Index) -> bool {
    idx.mappings
        .get("_source")
        .and_then(|s| s.get("enabled"))
        .and_then(|v| v.as_bool())
        .unwrap_or(true)
}

pub(crate) fn stored_paths(idx: &Index, patterns: &[String]) -> Vec<String> {
    let mut out = Vec::new();
    fn walk(
        props: &Map<String, Value>,
        prefix: &str,
        patterns: &[String],
        out: &mut Vec<String>,
    ) {
        for (name, raw) in props {
            let Some(def) = raw.as_object() else { continue };
            let path = if prefix.is_empty() {
                name.clone()
            } else {
                format!("{}.{}", prefix, name)
            };
            let matched = patterns.iter().any(|p| {
                p == "*"
                    || crate::resp::glob_match(p, &path)
                    || (p == &path)
            });
            if def.get("store").and_then(|v| v.as_bool()) == Some(true) && matched {
                out.push(path.clone());
            }
            if let Some(sub) = def.get("properties").and_then(|p| p.as_object()) {
                walk(sub, &path, patterns, out);
            }
        }
    }
    if let Some(props) = idx.mappings.get("properties").and_then(|p| p.as_object()) {
        walk(props, "", patterns, &mut out);
    }
    out
}

pub(crate) fn format_stored_value(ftype: &str, v: &Value) -> Value {
    format_doc_value(ftype, v, "")
}

pub(crate) fn format_stored(idx: &Index, field: &str, v: &Value) -> Value {
    let def = mapping_field_def(idx, field);
    let ftype = def
        .as_ref()
        .and_then(|d| d.get("type"))
        .and_then(|t| t.as_str())
        .unwrap_or("");
    if matches!(ftype, "date" | "date_nanos") {
        if let Some(s) = v.as_str() {
            if let Some(d) = def.as_ref() {
                if let Some(ms) = crate::fields::parse_date_any(d, s) {
                    return format_doc_value("date", &json!(ms), "");
                }
            }
        }
    }
    v.clone()
}

pub(crate) fn filter_source(doc: &Value, spec: &Value) -> Value {
    let mut includes: Vec<String> = Vec::new();
    let mut excludes: Vec<String> = Vec::new();
    match spec {
        Value::String(s) => {
            for p in s.split(',') {
                if !p.is_empty() {
                    includes.push(p.to_string());
                }
            }
        }
        Value::Array(a) => {
            for e in a {
                if let Some(s) = e.as_str() {
                    includes.push(s.to_string());
                }
            }
        }
        Value::Object(m) => {
            for key in ["includes", "include"] {
                if let Some(a) = m.get(key).and_then(|v| v.as_array()) {
                    for e in a {
                        if let Some(s) = e.as_str() {
                            includes.push(s.to_string());
                        }
                    }
                } else if let Some(s) = m.get(key).and_then(|v| v.as_str()) {
                    includes.push(s.to_string());
                }
            }
            for key in ["excludes", "exclude"] {
                if let Some(a) = m.get(key).and_then(|v| v.as_array()) {
                    for e in a {
                        if let Some(s) = e.as_str() {
                            excludes.push(s.to_string());
                        }
                    }
                } else if let Some(s) = m.get(key).and_then(|v| v.as_str()) {
                    excludes.push(s.to_string());
                }
            }
        }
        _ => {}
    }
    if includes.is_empty() {
        includes.push("*".to_string());
    }
    let mut leaves = Vec::new();
    collect_leaf_paths(doc, "", &mut leaves);
    let mut out = Map::new();
    for leaf in leaves {
        let inc = includes.iter().any(|p| path_matches(p, &leaf));
        let exc = excludes.iter().any(|p| path_matches(p, &leaf));
        if inc && !exc {
            let segs: Vec<String> = leaf.split('.').map(|s| s.to_string()).collect();
            merge_path(&mut out, doc, &segs);
        }
    }
    for pat in &excludes {
        if let Some(prefix) = pat.strip_suffix(".*") {
            if !prefix.is_empty() && includes.iter().any(|p| path_matches(p, prefix)) {
                if let Some(v) = crate::jsonv::field_value(doc, prefix) {
                    if v.is_object() && out.get(prefix).is_none() {
                        let segs: Vec<&str> = prefix.split('.').collect();
                        if segs.len() == 1 {
                            out.insert(prefix.to_string(), json!({}));
                        } else {
                            let mut cur = &mut out;
                            for seg in &segs[..segs.len() - 1] {
                                if !cur.get(*seg).map(|v| v.is_object()).unwrap_or(false) {
                                    cur.insert(seg.to_string(), Value::Object(Map::new()));
                                }
                                cur = cur.get_mut(*seg).unwrap().as_object_mut().unwrap();
                            }
                            cur.insert(segs[segs.len() - 1].to_string(), json!({}));
                        }
                    }
                }
            }
        }
    }
    Value::Object(out)
}
