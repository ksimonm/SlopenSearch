//! Request rewriting before evaluation: named queries, rescore, terms
//! lookups, inlined stored scripts.

use super::fetch::*;
use super::sort::*;
use crate::errors::ApiError;
use crate::jsonv::to_f64;
use crate::model::{Index, Store};
use serde_json::{Map, Value};
use std::collections::HashMap;

pub(crate) fn strip_query_names(v: &Value) -> Value {
    match v {
        Value::Object(m) => {
            let mut out = Map::new();
            for (k, val) in m {
                if k == "_name" {
                    continue;
                }
                out.insert(k.clone(), strip_query_names(val));
            }
            Value::Object(out)
        }
        Value::Array(a) => Value::Array(a.iter().map(strip_query_names).collect()),
        other => other.clone(),
    }
}

pub(crate) fn find_query_name(v: &Value) -> Option<String> {
    match v {
        Value::Object(m) => {
            if let Some(n) = m.get("_name").and_then(|x| x.as_str()) {
                return Some(n.to_string());
            }
            for val in m.values() {
                if let Some(n) = find_query_name(val) {
                    return Some(n);
                }
            }
            None
        }
        Value::Array(a) => a.iter().find_map(find_query_name),
        _ => None,
    }
}

pub(crate) fn walk_named_queries(
    out: &mut Vec<(String, f64)>,
    idx: &Index,
    id: &str,
    doc: &Value,
    extra: &HashMap<String, Map<String, Value>>,
    q: &Value,
) -> Result<(), ApiError> {
    let Some(m) = q.as_object() else { return Ok(()) };
    let compound = m.contains_key("bool")
        || m.contains_key("constant_score")
        || m.contains_key("function_score");
    if !compound {
        if let Some(name) = find_query_name(q) {
            let clean = strip_query_names(q);
            let cm = clean.as_object().cloned().unwrap_or_default();
            let (ok, score) = eval_doc(idx, id, doc, extra, &cm)?;
            if ok {
                out.push((name, score));
            }
        }
    }
    if let Some(b) = m.get("bool").and_then(|v| v.as_object()) {
        if let Some(name) = b.get("_name").and_then(|v| v.as_str()) {
            let clean = strip_query_names(q);
            let cm = clean.as_object().cloned().unwrap_or_default();
            let (ok, score) = eval_doc(idx, id, doc, extra, &cm)?;
            if ok {
                out.push((name.to_string(), score));
            }
        }
        for key in ["must", "should", "filter"] {
            if let Some(clauses) = b.get(key) {
                match clauses {
                    Value::Array(a) => {
                        for c in a {
                            walk_named_queries(out, idx, id, doc, extra, c)?;
                        }
                    }
                    other => walk_named_queries(out, idx, id, doc, extra, other)?,
                }
            }
        }
        return Ok(());
    }
    if let Some(cs) = m.get("constant_score").and_then(|v| v.as_object()) {
        if let Some(name) = cs.get("_name").and_then(|v| v.as_str()) {
            let clean = strip_query_names(q);
            let cm = clean.as_object().cloned().unwrap_or_default();
            let (ok, score) = eval_doc(idx, id, doc, extra, &cm)?;
            if ok {
                out.push((name.to_string(), score));
            }
        }
        if let Some(f) = cs.get("filter") {
            walk_named_queries(out, idx, id, doc, extra, f)?;
        }
        return Ok(());
    }
    if let Some(fs) = m.get("function_score").and_then(|v| v.as_object()) {
        if let Some(name) = fs.get("_name").and_then(|v| v.as_str()) {
            let clean = strip_query_names(q);
            let cm = clean.as_object().cloned().unwrap_or_default();
            let (ok, score) = eval_doc(idx, id, doc, extra, &cm)?;
            if ok {
                out.push((name.to_string(), score));
            }
        }
        if let Some(f) = fs.get("query") {
            walk_named_queries(out, idx, id, doc, extra, f)?;
        }
        return Ok(());
    }
    Ok(())
}

pub(crate) fn apply_rescore(
    store: &Store,
    matched: &mut [MultiHit],
    raw: &Value,
) -> Result<(), ApiError> {
    let configs: Vec<Value> = match raw {
        Value::Array(a) => a.clone(),
        other => vec![other.clone()],
    };
    for cfg in configs {
        let Some(cm) = cfg.as_object() else { continue };
        let Some(qm) = cm.get("query").and_then(|v| v.as_object()) else { continue };
        let Some(rqv) = qm.get("rescore_query") else { continue };
        let Some(rq) = rqv.as_object() else { continue };
        let window = cm.get("window_size").and_then(to_f64).map(|n| n as usize).unwrap_or(10);
        let mode = qm.get("score_mode").and_then(|v| v.as_str()).unwrap_or("total");
        let qw = qm.get("query_weight").and_then(to_f64).unwrap_or(1.0);
        let rw = qm.get("rescore_query_weight").and_then(to_f64).unwrap_or(1.0);
        let mut extras: HashMap<String, HashMap<String, Map<String, Value>>> = HashMap::new();
        for h in matched.iter_mut().take(window) {
            let idx = &store.indices[h.idx_name];
            let extra = extras
                .entry(h.idx_name.to_string())
                .or_insert_with(|| precompute_doc_values(idx));
            let (ok, rs) = eval_doc(idx, &h.id, &h.doc, extra, rq)?;
            let base = h.score;
            let combined = if !ok {
                match mode {
                    "multiply" | "avg" | "max" | "min" => base,
                    _ => base * qw,
                }
            } else {
                match mode {
                    "multiply" => (base * qw) * (rs * rw),
                    "avg" => (base * qw + rs * rw) / 2.0,
                    "max" => (base * qw).max(rs * rw),
                    "min" => (base * qw).min(rs * rw),
                    _ => base * qw + rs * rw,
                }
            };
            h.score = combined;
        }
    }
    Ok(())
}

pub(crate) fn resolve_terms_lookups(store: &Store, v: &mut Value) -> Result<(), ApiError> {
    match v {
        Value::Object(m) => {
            if let Some(terms) = m.get_mut("terms").and_then(|t| t.as_object_mut()) {
                let keys: Vec<String> = terms.keys().cloned().collect();
                for field in keys {
                    let lookup = match terms.get(&field).and_then(|x| x.as_object()) {
                        Some(o) if o.contains_key("index") && o.contains_key("id") => o.clone(),
                        _ => continue,
                    };
                    let index = lookup
                        .get("index")
                        .and_then(|x| x.as_str())
                        .unwrap_or("")
                        .to_string();
                    let id = lookup
                        .get("id")
                        .and_then(|x| x.as_str())
                        .unwrap_or("")
                        .to_string();
                    let path = lookup
                        .get("path")
                        .and_then(|x| x.as_str())
                        .unwrap_or("")
                        .to_string();
                    let idx = match store.indices.get(&index) {
                        Some(i) => i,
                        None => {
                            return Err(ApiError::new(
                                "index_not_found_exception",
                                &format!("no such index [{}]", index),
                                404,
                            ))
                        }
                    };
                    let mut vals = Vec::new();
                    if let Some(doc) = idx.docs.get(&id) {
                        let parts: Vec<&str> = path.split('.').filter(|p| !p.is_empty()).collect();
                        lookup_path_values(doc, &parts, &mut vals);
                    }
                    terms.insert(field, Value::Array(vals));
                }
            }
            for (_, child) in m.iter_mut() {
                resolve_terms_lookups(store, child)?;
            }
        }
        Value::Array(a) => {
            for child in a.iter_mut() {
                resolve_terms_lookups(store, child)?;
            }
        }
        _ => {}
    }
    Ok(())
}

pub(crate) fn lookup_path_values(v: &Value, parts: &[&str], out: &mut Vec<Value>) {
    if parts.is_empty() {
        match v {
            Value::Array(a) => {
                for e in a {
                    lookup_path_values(e, parts, out);
                }
            }
            Value::Null => {}
            other => out.push(other.clone()),
        }
        return;
    }
    match v {
        Value::Object(m) => {
            if let Some(next) = m.get(parts[0]) {
                lookup_path_values(next, &parts[1..], out);
            }
        }
        Value::Array(a) => {
            for e in a {
                lookup_path_values(e, parts, out);
            }
        }
        _ => {}
    }
}

/// Replaces `{"script": {"id": ...}}` references to stored scripts by the stored
/// source, so every scripted part of a search reads the script as it is right now.
pub(crate) fn inline_stored_scripts(store: &Store, v: &mut Value) {
    match v {
        Value::Object(m) => {
            for (k, val) in m.iter_mut() {
                if k == "script" {
                    if let Some(sm) = val.as_object_mut() {
                        if !sm.contains_key("source") {
                            let stored = sm
                                .get("id")
                                .and_then(|i| i.as_str())
                                .and_then(|i| store.scripts.get(i))
                                .and_then(|st| st.get("script"))
                                .and_then(|st| st.as_object());
                            if let Some(st) = stored {
                                sm.remove("id");
                                for (sk, sv) in st {
                                    if sk == "source" || sk == "lang" {
                                        sm.insert(sk.clone(), sv.clone());
                                    }
                                }
                                continue;
                            }
                        }
                    }
                }
                inline_stored_scripts(store, val);
            }
        }
        Value::Array(a) => {
            for x in a {
                inline_stored_scripts(store, x);
            }
        }
        _ => {}
    }
}
