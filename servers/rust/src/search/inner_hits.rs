//! Inner hits (nested, has_child/has_parent, percolate) and field collapsing.

use super::sort::*;
use crate::search::eval::QueryCtx;
use crate::errors::ApiError;
use crate::jsonv::{field_value, to_f64};
use crate::model::{Index, Store};
use serde_json::{json, Map, Value};
use std::cmp::Ordering;

pub(crate) fn collapse_value(doc: &Value, field: &str) -> Option<Value> {
    let v = field_value(doc, field)?;
    match v {
        Value::Array(a) => a.first().cloned(),
        Value::Null => None,
        other => Some(other.clone()),
    }
}

pub(crate) fn ih_collect_offsets(v: &Value, path: &str) -> Vec<(usize, Value)> {
    if path.is_empty() {
        return match v {
            Value::Array(a) => a.iter().cloned().enumerate().collect(),
            other => vec![(0, other.clone())],
        };
    }
    if let Some(arr) = v.as_array() {
        let mut out = Vec::new();
        for e in arr {
            out.extend(ih_collect_offsets(e, path));
        }
        return out;
    }
    let (head, rest) = match path.split_once('.') {
        Some((h, r)) => (h, r),
        None => (path, ""),
    };
    match v.get(head) {
        Some(next) => ih_collect_offsets(next, rest),
        None => Vec::new(),
    }
}

pub(crate) fn ih_scoped_ctx<'a>(
    idx: &'a Index,
    id: &str,
    doc: &'a Value,
    path: &str,
    child: &Value,
) -> QueryCtx<'a> {
    let mut vals = Map::new();
    crate::search::eval_extra::flatten_scope(path, child, &mut vals);
    let mut cc = QueryCtx::with_values(idx, id, doc, vals);
    cc.scope_path = Some(path.to_string());
    cc.scope_obj = Some(child.clone());
    cc
}

pub(crate) fn ih_source(child: &Value, path: &str, spec: Option<&Value>) -> Value {
    let includes = match spec {
        Some(Value::Array(a)) => a.iter().filter_map(|v| v.as_str()).collect::<Vec<_>>(),
        Some(Value::String(s)) => vec![s.as_str()],
        _ => return child.clone(),
    };
    if includes.is_empty() {
        return child.clone();
    }
    let mut out = Map::new();
    for inc in includes {
        if inc == path {
            return child.clone();
        }
        if let Some(rel) = inc.strip_prefix(&format!("{}.", path)) {
            if let Some(v) = child.get(rel) {
                out.insert(rel.to_string(), v.clone());
            }
        }
    }
    Value::Object(out)
}

pub(crate) fn ih_sort_fields(spec: Option<&Value>) -> Vec<(String, bool)> {
    let mut out = Vec::new();
    let arr = match spec {
        Some(Value::Array(a)) => a.clone(),
        Some(other) => vec![other.clone()],
        None => return out,
    };
    for item in arr {
        match &item {
            Value::String(f) => out.push((f.clone(), false)),
            Value::Object(m) => {
                for (f, cfg) in m {
                    let desc = match cfg {
                        Value::String(s) => s == "desc",
                        Value::Object(o) => {
                            o.get("order").and_then(|v| v.as_str()) == Some("desc")
                        }
                        _ => false,
                    };
                    out.push((f.clone(), desc));
                }
            }
            _ => {}
        }
    }
    out
}

pub(crate) fn ih_sort_value(ctx: &QueryCtx, field: &str) -> Value {
    let (vals, _) = ctx.values(field);
    vals.into_iter().next().unwrap_or(Value::Null)
}

pub(crate) fn ih_cmp(a: &Value, b: &Value) -> std::cmp::Ordering {
    let an = a.as_f64();
    let bn = b.as_f64();
    match (an, bn) {
        (Some(x), Some(y)) => x.partial_cmp(&y).unwrap_or(std::cmp::Ordering::Equal),
        _ => a
            .as_str()
            .unwrap_or("")
            .cmp(b.as_str().unwrap_or("")),
    }
}

pub(crate) fn ih_build_group(
    mut hits: Vec<(usize, Value, Vec<Value>, Map<String, Value>)>,
    path: &str,
    spec: &Map<String, Value>,
    outer: Option<(&str, usize)>,
    source_spec: Option<&Value>,
    id: &str,
) -> Value {
    let sorts = ih_sort_fields(spec.get("sort"));
    let scored = hits.iter().any(|h| h.3.contains_key("_score"));
    let score_of = |h: &(usize, Value, Vec<Value>, Map<String, Value>)| {
        h.3.get("_score").and_then(|s| s.as_f64()).unwrap_or(0.0)
    };
    let mut max_score: Option<f64> = None;
    if scored && sorts.is_empty() {
        hits.sort_by(|a, b| {
            score_of(b).partial_cmp(&score_of(a)).unwrap_or(std::cmp::Ordering::Equal).then(a.0.cmp(&b.0))
        });
        max_score = hits.first().map(score_of);
    }
    if !sorts.is_empty() {
        hits.sort_by(|a, b| {
            for i in 0..sorts.len() {
                let mut c = ih_cmp(&a.2[i], &b.2[i]);
                if sorts[i].1 {
                    c = c.reverse();
                }
                if c != std::cmp::Ordering::Equal {
                    return c;
                }
            }
            a.0.cmp(&b.0)
        });
    }
    let from = spec.get("from").and_then(to_f64).map(|n| n as usize).unwrap_or(0);
    let size = spec.get("size").and_then(to_f64).map(|n| n as usize).unwrap_or(3);
    let total = hits.len();
    let mut inner = Vec::new();
    for (offset, child, svals, mut extra) in hits.into_iter().skip(from).take(size) {
        let mut e = Map::new();
        if let Some(v) = extra.remove("_index") {
            e.insert("_index".to_string(), v);
        }
        if let Some(v) = extra.remove("_id") {
            e.insert("_id".to_string(), v);
        } else {
            e.insert("_id".to_string(), json!(id));
        }
        if !path.is_empty() {
            let meta = match outer {
                Some((opath, ooff)) => {
                    let mut m = Map::new();
                    m.insert("field".to_string(), json!(opath));
                    m.insert("offset".to_string(), json!(ooff));
                    let mut sub = Map::new();
                    sub.insert(
                        "field".to_string(),
                        json!(path.rsplit('.').next().unwrap_or(path)),
                    );
                    sub.insert("offset".to_string(), json!(offset));
                    m.insert("_nested".to_string(), Value::Object(sub));
                    Value::Object(m)
                }
                None => json!({"field": path, "offset": offset}),
            };
            e.insert("_nested".to_string(), meta);
        }
        if let Some(sc) = extra.remove("_score") {
            e.insert("_score".to_string(), if sorts.is_empty() { sc } else { Value::Null });
        }
        if source_spec != Some(&Value::Bool(false)) {
            e.insert(
                "_source".to_string(),
                ih_source(&child, path, source_spec),
            );
        }
        if !svals.is_empty() {
            e.insert("sort".to_string(), Value::Array(svals));
        }
        if let Some(r) = extra.remove("_routing") {
            e.insert("_routing".to_string(), r);
        }
        if let Some(ih) = extra.remove("inner_hits") {
            e.insert("inner_hits".to_string(), ih);
        }
        inner.push(Value::Object(e));
    }
    let mut hits_obj = Map::new();
    hits_obj.insert("total".to_string(), json!({"value": total, "relation": "eq"}));
    if scored {
        hits_obj.insert("max_score".to_string(), max_score.map(|m| json!(m)).unwrap_or(Value::Null));
    }
    hits_obj.insert("hits".to_string(), Value::Array(inner));
    let mut group = Map::new();
    group.insert("hits".to_string(), Value::Object(hits_obj));
    Value::Object(group)
}

pub(crate) fn find_percolate_map<'a>(q: &'a Map<String, Value>) -> Option<&'a Map<String, Value>> {
    if let Some(p) = q.get("percolate").and_then(|v| v.as_object()) {
        return Some(p);
    }
    for key in ["bool", "constant_score", "function_score"] {
        if let Some(bm) = q.get(key).and_then(|v| v.as_object()) {
            for (k, v) in bm {
                if k == "query" {
                    if let Some(sub) = v.as_object() {
                        if let Some(p) = find_percolate_map(sub) {
                            return Some(p);
                        }
                    }
                } else if matches!(k.as_str(), "must" | "should" | "filter") {
                    let items: Vec<&Value> = match v {
                        Value::Array(a) => a.iter().collect(),
                        other => vec![other],
                    };
                    for item in items {
                        if let Some(sub) = item.as_object() {
                            if let Some(p) = find_percolate_map(sub) {
                                return Some(p);
                            }
                        }
                    }
                }
            }
        }
    }
    None
}

pub(crate) fn build_query_inner_hits(
    idx: &Index,
    id: &str,
    doc: &Value,
    q: &Map<String, Value>,
    scope_path: Option<&str>,
    scope_obj: Option<&Value>,
    outer: Option<(&str, usize)>,
) -> Result<Map<String, Value>, ApiError> {
    let mut out = Map::new();
    for (key, val) in q {
        if key == "nested" {
            let Some(bm) = val.as_object() else { continue };
            let path = bm.get("path").and_then(|v| v.as_str()).unwrap_or("");
            if path.is_empty() {
                continue;
            }
            let inner = bm.get("query").and_then(|v| v.as_object());
            let children = if let Some(sp) = scope_path {
                if path.starts_with(&format!("{}.", sp)) {
                    match scope_obj {
                        Some(o) => ih_collect_offsets(o, &path[sp.len() + 1..]),
                        None => Vec::new(),
                    }
                } else {
                    ih_collect_offsets(doc, path)
                }
            } else {
                ih_collect_offsets(doc, path)
            };
            let spec = bm.get("inner_hits").and_then(|v| v.as_object());
            let sorts = spec
                .map(|s| ih_sort_fields(s.get("sort")))
                .unwrap_or_default();
            let mut matched = Vec::new();
            for (offset, child) in children {
                if !child.is_object() {
                    continue;
                }
                let cc = ih_scoped_ctx(idx, id, doc, path, &child);
                let mut extra = Map::new();
                if let Some(inner) = inner {
                    let (ok, sc) = cc.eval(inner)?;
                    if !ok {
                        continue;
                    }
                    extra.insert("_index".to_string(), json!(idx.name));
                    extra.insert("_score".to_string(), json!(crate::mapping::knn::f32r(sc)));
                    let sub = build_query_inner_hits(
                        idx,
                        id,
                        doc,
                        inner,
                        Some(path),
                        Some(&child),
                        Some((path, offset)),
                    )?;
                    if !sub.is_empty() {
                        extra.insert("inner_hits".to_string(), Value::Object(sub));
                    }
                }
                let mut svals = Vec::new();
                for (f, _) in &sorts {
                    svals.push(ih_sort_value(&cc, f));
                }
                matched.push((offset, child, svals, extra));
            }
            if let Some(spec) = spec {
                let name = spec
                    .get("name")
                    .and_then(|v| v.as_str())
                    .unwrap_or(path)
                    .to_string();
                let group = ih_build_group(
                    matched,
                    path,
                    spec,
                    outer,
                    spec.get("_source"),
                    id,
                );
                out.insert(name, group);
            }
            continue;
        }
        if key == "has_child" {
            let Some(bm) = val.as_object() else { continue };
            let Some(spec) = bm.get("inner_hits").and_then(|v| v.as_object()) else {
                continue;
            };
            let child_type = bm.get("type").and_then(|v| v.as_str()).unwrap_or("");
            let inner = bm.get("query").and_then(|v| v.as_object());
            let field = match crate::search::eval_extra::find_join_field(idx) {
                Some(f) => f,
                None => continue,
            };
            let sorts = ih_sort_fields(spec.get("sort"));
            let mut matched = Vec::new();
            for cid in &idx.order {
                let Some(cdoc) = idx.docs.get(cid) else { continue };
                let (name, parent) = crate::search::eval_extra::join_parts(cdoc.get(&field));
                if parent != id || (!child_type.is_empty() && name != child_type) {
                    continue;
                }
                let cc = QueryCtx::new(idx, cid, cdoc);
                if let Some(inner) = inner {
                    let (ok, _) = cc.eval(inner)?;
                    if !ok {
                        continue;
                    }
                }
                let mut svals = Vec::new();
                for (f, _) in &sorts {
                    svals.push(ih_sort_value(&cc, f));
                }
                let mut extra = Map::new();
                extra.insert("_id".to_string(), json!(cid));
                if let Some(r) = idx.routing.get(cid) {
                    extra.insert("_routing".to_string(), json!(r));
                }
                matched.push((0usize, cdoc.clone(), svals, extra));
            }
            let name = spec
                .get("name")
                .and_then(|v| v.as_str())
                .unwrap_or(child_type)
                .to_string();
            let group = ih_build_group(
                matched,
                "",
                spec,
                None,
                spec.get("_source"),
                id,
            );
            out.insert(name, group);
            continue;
        }
        if key == "has_parent" {
            let Some(bm) = val.as_object() else { continue };
            let Some(spec) = bm.get("inner_hits").and_then(|v| v.as_object()) else {
                continue;
            };
            let ptype = bm
                .get("parent_type")
                .and_then(|v| v.as_str())
                .or_else(|| bm.get("type").and_then(|v| v.as_str()))
                .unwrap_or("");
            let pid = idx.routing.get(id).cloned().unwrap_or_default();
            let Some(pdoc) = idx.docs.get(&pid) else { continue };
            let field = match crate::search::eval_extra::find_join_field(idx) {
                Some(f) => f,
                None => continue,
            };
            let (name, _) = crate::search::eval_extra::join_parts(pdoc.get(&field));
            if !ptype.is_empty() && name != ptype {
                continue;
            }
            if let Some(inner) = bm.get("query").and_then(|v| v.as_object()) {
                let cc = QueryCtx::new(idx, &pid, pdoc);
                let (ok, _) = cc.eval(inner)?;
                if !ok {
                    continue;
                }
            }
            let mut hit = Map::new();
            hit.insert("_id".to_string(), json!(pid));
            hit.insert("_source".to_string(), pdoc.clone());
            let mut hits_obj = Map::new();
            hits_obj.insert("total".to_string(), json!({"value": 1, "relation": "eq"}));
            hits_obj.insert("hits".to_string(), Value::Array(vec![Value::Object(hit)]));
            let mut group = Map::new();
            group.insert("hits".to_string(), Value::Object(hits_obj));
            let gname = spec
                .get("name")
                .and_then(|v| v.as_str())
                .unwrap_or(ptype)
                .to_string();
            out.insert(gname, Value::Object(group));
            continue;
        }
        if matches!(key.as_str(), "bool" | "constant_score" | "function_score") {
            if let Some(bm) = val.as_object() {
                for (k2, v2) in bm {
                    if k2 == "query" {
                        if let Some(sub) = v2.as_object() {
                            let got = build_query_inner_hits(
                                idx, id, doc, sub, scope_path, scope_obj, outer,
                            )?;
                            for (k, v) in got {
                                out.insert(k, v);
                            }
                        }
                    } else if matches!(k2.as_str(), "must" | "should" | "filter") {
                        let arr = match v2 {
                            Value::Array(a) => a.clone(),
                            other => vec![other.clone()],
                        };
                        for item in arr {
                            if let Some(sub) = item.as_object() {
                                let got = build_query_inner_hits(
                                    idx, id, doc, sub, scope_path, scope_obj, outer,
                                )?;
                                for (k, v) in got {
                                    out.insert(k, v);
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    Ok(out)
}

pub(crate) fn build_collapse_inner_hits(
    store: &Store,
    pool: &[MultiHit],
    field: &str,
    val: &Option<Value>,
    raw: &Value,
) -> Result<Map<String, Value>, ApiError> {
    let configs: Vec<Value> = match raw {
        Value::Array(a) => a.clone(),
        other => vec![other.clone()],
    };
    let mut out = Map::new();
    for cfg in configs {
        let cm = cfg.as_object().cloned().unwrap_or_default();
        let name = cm
            .get("name")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        let size = cm.get("size").and_then(to_f64).map(|n| n as usize).unwrap_or(3);
        let specs = parse_sort_specs(cm.get("sort"))?;
        let source_false = matches!(cm.get("_source"), Some(Value::Bool(false)));
        let mut group: Vec<&MultiHit> = pool
            .iter()
            .filter(|h| &collapse_value(&h.doc, field) == val)
            .collect();
        if !specs.is_empty() {
            let mut keyed: Vec<(Vec<(bool, Value)>, &MultiHit)> = Vec::with_capacity(group.len());
            for h in group {
                let mut cells = Vec::with_capacity(specs.len());
                for sp in &specs {
                    cells.push(compute_sort_cell(store, h, sp)?);
                }
                keyed.push((cells, h));
            }
            keyed.sort_by(|a, b| {
                for i in 0..specs.len() {
                    let ca = &a.0[i];
                    let cb = &b.0[i];
                    let mut c = cmp_sort_cells(ca, cb);
                    if specs[i].desc && !ca.0 && !cb.0 {
                        c = c.reverse();
                    }
                    if c != Ordering::Equal {
                        return c;
                    }
                }
                Ordering::Equal
            });
            group = keyed.into_iter().map(|(_, h)| h).collect();
        }
        let total = group.len();
        group.truncate(size);
        let mut inner = Vec::new();
        for h in group {
            let mut e = Map::new();
            e.insert("_index".to_string(), json!(h.idx_name));
            e.insert("_id".to_string(), json!(h.id));
            e.insert(
                "_score".to_string(),
                if specs.is_empty() { json!(h.score) } else { Value::Null },
            );
            if !specs.is_empty() {
                let mut svals = Vec::with_capacity(specs.len());
                for sp in &specs {
                    svals.push(compute_sort_cell(store, h, sp)?.1);
                }
                e.insert("sort".to_string(), Value::Array(svals));
            }
            if !source_false {
                e.insert("_source".to_string(), h.doc.clone());
            }
            inner.push(Value::Object(e));
        }
        let mut body = Map::new();
        body.insert("total".to_string(), json!({"value": total, "relation": "eq"}));
        body.insert("max_score".to_string(), Value::Null);
        body.insert("hits".to_string(), Value::Array(inner));
        out.insert(name, json!({"hits": body}));
    }
    Ok(out)
}
