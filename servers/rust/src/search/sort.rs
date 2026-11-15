//! Sort specs, sort values of hits, `search_after`.

use super::fetch::*;
use crate::search::eval::{integer_of, QueryCtx};
use crate::mapping::mapping_field_def;
use crate::errors::ApiError;
use crate::jsonv::{field_value, to_f64, value_to_string};
use crate::model::{Index, Store};
use serde_json::{json, Map, Value};
use std::cmp::Ordering;

pub(crate) fn compare_sort_values(a: &Value, b: &Value) -> Ordering {
    if let (Value::Number(na), Value::Number(nb)) = (a, b) {
        if let (Some(x), Some(y)) = (na.as_u64(), nb.as_u64()) {
            return x.cmp(&y);
        }
        if let (Some(x), Some(y)) = (na.as_i64(), nb.as_i64()) {
            return x.cmp(&y);
        }
        if let (Some(x), Some(y)) = (na.as_u64(), nb.as_i64()) {
            if y < 0 {
                return Ordering::Greater;
            }
            return x.cmp(&(y as u64));
        }
        if let (Some(x), Some(y)) = (na.as_i64(), nb.as_u64()) {
            if x < 0 {
                return Ordering::Less;
            }
            return (x as u64).cmp(&y);
        }
    }
    if (a.is_string() && b.is_number()) || (a.is_number() && b.is_string()) {
        if let (Some(x), Some(y)) = (to_f64(a), to_f64(b)) {
            return x.partial_cmp(&y).unwrap_or(Ordering::Equal);
        }
    }
    if let Some(as_) = a.as_str() {
        let bs = value_to_string(b).unwrap_or_default();
        if let (Some(x), Some(y)) = (crate::search::eval::ip_to_16(as_), crate::search::eval::ip_to_16(&bs)) {
            return x.cmp(&y);
        }
        return as_.cmp(&bs);
    }
    if let (Some(af), Some(bf)) = (to_f64(a), to_f64(b)) {
        return af.partial_cmp(&bf).unwrap_or(Ordering::Equal);
    }
    let as_ = value_to_string(a).unwrap_or_default();
    let bs = value_to_string(b).unwrap_or_default();
    as_.cmp(&bs)
}

#[derive(Clone)]
pub(crate) struct MultiHit {
    pub(crate) idx_name: String,
    pub(crate) id: String,
    pub(crate) doc: Value,
    pub(crate) score: f64,
    pub(crate) pos: i64,
    pub(crate) named: Vec<(String, f64)>,
}

#[derive(Clone, Default)]
pub(crate) struct SortSpec {
    pub(crate) field: String,
    pub(crate) desc: bool,
    pub(crate) kind: u8,
    pub(crate) mode: Option<String>,
    pub(crate) missing: Option<Value>,
    pub(crate) nested: Option<String>,
    pub(crate) nested_filter: Option<Value>,
    pub(crate) unmapped_type: Option<String>,
    pub(crate) numeric_type: Option<String>,
    pub(crate) script_raw: Value,
    pub(crate) script_type: String,
    pub(crate) geo: Value,
    pub(crate) geo_unit: String,
    pub(crate) geo_ignore: bool,
}

pub(crate) fn sort_parse_error(reason: &str) -> ApiError {
    let mut e = ApiError::new("x_content_parse_exception", reason, 400);
    e.caused_by = Some(Box::new(ApiError::new(
        "illegal_argument_exception",
        reason,
        400,
    )));
    e
}

pub(crate) fn sort_shard_error(typ: &str, reason: &str) -> ApiError {
    let mut e = ApiError::new(
        "search_phase_execution_exception",
        "all shards failed",
        400,
    );
    let cause = ApiError::new(typ, reason, 400);
    e.root_cause = Some(Box::new(cause.clone()));
    e.caused_by = Some(Box::new(cause));
    e.field("lang", json!("painless"))
}

pub(crate) fn parse_sort_specs(raw: Option<&Value>) -> Result<Vec<SortSpec>, ApiError> {
    let items: Vec<Value> = match raw {
        None | Some(Value::Null) => return Ok(vec![]),
        Some(Value::Array(a)) => a.clone(),
        Some(other) => vec![other.clone()],
    };
    let mut out = Vec::new();
    for item in items {
        let mut sp = SortSpec::default();
        match &item {
            Value::String(f) => {
                sp.field = f.clone();
                sp.kind = if f == "_score" { 1 } else if f == "_id" { 2 } else if f == "_index" { 3 } else if f == "_doc" { 4 } else { 0 };
                if sp.kind == 1 {
                    sp.desc = true;
                }
            }
            Value::Object(m) => {
                if let Some(g) = m.get("_geo_distance") {
                    sp.kind = 6;
                    if let Some(go) = g.as_object() {
                        for (k, v) in go {
                            match k.as_str() {
                                "order" => sp.desc = value_to_string(v).unwrap_or_default() == "desc",
                                "unit" => sp.geo_unit = value_to_string(v).unwrap_or_default(),
                                "mode" => sp.mode = v.as_str().map(|x| x.to_string()),
                                "ignore_unmapped" => sp.geo_ignore = v.as_bool().unwrap_or(false),
                                "nested" => {}
                                _ => {
                                    sp.geo = v.clone();
                                    sp.field = k.clone();
                                }
                            }
                        }
                    }
                    if sp.geo_unit.is_empty() {
                        sp.geo_unit = "m".to_string();
                    }
                    if !matches!(sp.geo_unit.as_str(), "m" | "km" | "mi" | "yd" | "ft" | "in" | "cm" | "mm" | "nmi") {
                        return Err(ApiError::new(
                            "illegal_argument_exception",
                            &format!("No distance unit match [{}]", sp.geo_unit),
                            400,
                        ));
                    }
                    out.push(sp);
                    continue;
                }
                if let Some(sc) = m.get("_script") {
                    sp.kind = 5;
                    if let Some(so) = sc.as_object() {
                        sp.desc = so
                            .get("order")
                            .map(|v| value_to_string(v).unwrap_or_default() == "desc")
                            .unwrap_or(false);
                        let st = match so.get("type").and_then(|v| v.as_str()) {
                            Some(t) => t.to_string(),
                            None => return Err(ApiError::bad("Required [type]")),
                        };
                        if !matches!(st.as_str(), "number" | "string" | "version") {
                            return Err(sort_parse_error("[_script] failed to parse field [type]").caused_by(
                                ApiError::bad(&format!("Unknown ScriptSortType [{}]", st)),
                            ));
                        }
                        sp.script_type = st;
                        sp.script_raw = so.get("script").cloned().unwrap_or(Value::Null);
                    }
                    out.push(sp);
                    continue;
                }
                let Some((f, opts)) = m.iter().next() else { continue };
                sp.field = f.clone();
                sp.kind = if f == "_score" { 1 } else if f == "_id" { 2 } else if f == "_index" { 3 } else if f == "_doc" { 4 } else { 0 };
                if let Some(op) = opts.as_object() {
                    if op.contains_key("format") {
                        return Err(sort_parse_error(
                            "[1:1] [field_sort] unknown field [format]",
                        ));
                    }
                    for (k, v) in op {
                        match k.as_str() {
                            "order" => {
                                let od = value_to_string(v).unwrap_or_default();
                                if od.eq_ignore_ascii_case("asc") {
                                    sp.desc = false;
                                } else if od.eq_ignore_ascii_case("desc") {
                                    sp.desc = true;
                                } else {
                                    return Err(ApiError::new(
                                        "illegal_argument_exception",
                                        &format!("No enum constant org.opensearch.search.sort.SortOrder.{}", od.to_uppercase()),
                                        400,
                                    ));
                                }
                            }
                            "mode" => {
                                let mo = value_to_string(v).unwrap_or_default();
                                if !matches!(mo.as_str(), "min" | "max" | "sum" | "avg" | "median") {
                                    let mut e = ApiError::new(
                                        "x_content_parse_exception",
                                        "[1:1] failed to parse field [mode]",
                                        400,
                                    );
                                    e.caused_by = Some(Box::new(ApiError::new(
                                        "illegal_argument_exception",
                                        &format!("Unknown SortMode [{}]", mo),
                                        400,
                                    )));
                                    return Err(e);
                                }
                                sp.mode = Some(mo);
                            }
                            "missing" => sp.missing = Some(v.clone()),
                            "nested" => {
                                if let Some(path) = v.get("path").and_then(|p| p.as_str()) {
                                    sp.nested = Some(path.to_string());
                                }
                                if let Some(f) = v.get("filter") {
                                    sp.nested_filter = Some(f.clone());
                                }
                            }
                            "nested_path" => {
                                sp.nested = Some(value_to_string(v).unwrap_or_default())
                            }
                            "unmapped_type" => {
                                sp.unmapped_type = v.as_str().map(|x| x.to_string())
                            }
                            "numeric_type" => {
                                sp.numeric_type = v.as_str().map(|x| x.to_string())
                            }
                            _ => {}
                        }
                    }
                } else if let Some(od) = opts.as_str() {
                    if od.eq_ignore_ascii_case("desc") {
                        sp.desc = true;
                    } else if !od.eq_ignore_ascii_case("asc") {
                        return Err(ApiError::new(
                            "illegal_argument_exception",
                            &format!("No enum constant org.opensearch.search.sort.SortOrder.{}", od.to_uppercase()),
                            400,
                        ));
                    }
                }
            }
            _ => {}
        }
        out.push(sp);
    }
    Ok(out)
}

pub(crate) fn sort_mapping_def(idx: &Index, field: &str) -> Option<Map<String, Value>> {
    let field = &crate::mapping::resolve_alias_field(idx, field);
    if let Some(d) = mapping_field_def(idx, field) {
        return Some(d.clone());
    }
    if crate::mapping::fields::flat_object_root(idx, field).is_some() {
        let mut d = Map::new();
        d.insert("type".to_string(), json!("keyword"));
        return Some(d);
    }
    let (parent, sub) = field.rsplit_once('.')?;
    let pd = mapping_field_def(idx, parent)?;
    pd.get("fields")
        .and_then(|f| f.get(sub))
        .and_then(|d| d.as_object())
        .cloned()
}

pub(crate) fn nested_parent_of(idx: &Index, field: &str) -> Option<String> {
    let mut parts: Vec<&str> = field.split('.').collect();
    while parts.len() > 1 {
        parts.pop();
        let path = parts.join(".");
        if let Some(def) = mapping_field_def(idx, &path) {
            if def.get("type").and_then(|t| t.as_str()) == Some("nested") {
                return Some(path);
            }
        }
    }
    None
}

pub(crate) fn sort_missing_sentinel(sp: &SortSpec, ftype: &str, desc: bool) -> Value {
    match sp.numeric_type.as_deref() {
        Some("long") => {
            if desc { json!(-9223372036854775808i64) } else { json!(9223372036854775807i64) }
        }
        Some("double") => {
            if desc { json!("-Infinity") } else { json!("Infinity") }
        }
        _ => sort_type_min_max(ftype, desc),
    }
}

pub(crate) fn sort_type_min_max(ftype: &str, desc: bool) -> Value {
    match ftype {
        "integer" | "short" | "byte" => {
            if desc { json!(-2147483648i64) } else { json!(2147483647i64) }
        }
        "long" => {
            if desc { json!(-9223372036854775808i64) } else { json!(9223372036854775807i64) }
        }
        "float" | "double" | "half_float" | "scaled_float" => {
            if desc { json!("-Infinity") } else { json!("Infinity") }
        }
        "date" | "date_nanos" => {
            if desc { json!(-9223372036854775808i64) } else { json!(9223372036854775807i64) }
        }
        _ => Value::Null,
    }
}

pub(crate) fn source_sort_list(idx: &Index, id: &str, field: &str) -> Vec<Value> {
    let Some(doc) = idx.docs.get(id) else { return vec![] };
    let mut paths = vec![field.to_string()];
    let mut base = field.to_string();
    while let Some(pos) = base.rfind('.') {
        base.truncate(pos);
        paths.push(base.clone());
    }
    for p in paths {
        if let Some(v) = field_value(doc, &p) {
            let list = flatten_any(v);
            if !list.is_empty() {
                return list;
            }
        }
    }
    vec![]
}

pub(crate) fn convert_sort_scalar(sp: &SortSpec, ftype: &str, v: &Value) -> Value {
    if matches!(sp.numeric_type.as_deref(), Some("date_nanos")) && ftype != "date_nanos" {
        if let Some(ms) = integer_of(v) {
            return json!(ms * 1_000_000);
        }
        if let Some(ms) = crate::dates::parse_date(v) {
            return json!((ms * 1_000_000.0) as i64);
        }
    }
    if ftype == "date_nanos" || matches!(sp.numeric_type.as_deref(), Some("date_nanos")) {
        if let Some(n) = integer_of(v) {
            return json!(n);
        }
        if let Some(s) = v.as_str() {
            if let Some(ns) = crate::mapping::fields::parse_date_nanos_any(&Map::new(), s) {
                return json!(ns);
            }
        }
        if let Some(ms) = crate::dates::parse_date(v) {
            return json!((ms * 1_000_000.0) as i64);
        }
    }
    if ftype == "date" {
        if let Some(ms) = crate::dates::parse_date(v) {
            return json!(ms as i64);
        }
    }
    if ftype == "ip" {
        if let Some(s) = v.as_str() {
            if let Ok(c) = crate::mapping::fields::canonical_ip(s) {
                return Value::String(c);
            }
        }
    }
    if sp.numeric_type.as_deref() == Some("long") {
        if let Some(n) = to_f64(v) {
            return json!(n as i64);
        }
    }
    v.clone()
}

pub(crate) fn apply_sort_mode(sp: &SortSpec, ftype: &str, mut vals: Vec<Value>, desc: bool) -> Option<Value> {
    if vals.is_empty() {
        return None;
    }
    if vals.len() == 1 {
        return Some(vals.remove(0));
    }
    let numeric = matches!(
        ftype,
        "integer" | "short" | "byte" | "long" | "float" | "double" | "half_float" | "scaled_float" | "unsigned_long"
    );
    let mode = sp
        .mode
        .clone()
        .unwrap_or_else(|| if desc { "max".to_string() } else { "min".to_string() });
    if !numeric {
        return match mode.as_str() {
            "min" => vals.into_iter().min_by(compare_sort_values),
            "max" => vals.into_iter().max_by(compare_sort_values),
            _ => Some(Value::Null),
        };
    }
    let nums: Vec<f64> = vals.iter().filter_map(to_f64).collect();
    if nums.is_empty() {
        return None;
    }
    let out = match mode.as_str() {
        "min" => nums.iter().cloned().fold(f64::INFINITY, f64::min),
        "max" => nums.iter().cloned().fold(f64::NEG_INFINITY, f64::max),
        "sum" => nums.iter().sum(),
        "avg" => nums.iter().sum::<f64>() / nums.len() as f64,
        "median" => {
            let mut n = nums.clone();
            n.sort_by(|a, b| a.partial_cmp(b).unwrap_or(Ordering::Equal));
            let m = n.len() / 2;
            if n.len() % 2 == 0 {
                (n[m - 1] + n[m]) / 2.0
            } else {
                n[m]
            }
        }
        _ => nums[0],
    };
    let integral = matches!(ftype, "integer" | "short" | "byte" | "long" | "unsigned_long");
    Some(if integral {
        json!(out.round() as i64)
    } else if out.is_finite() {
        json!(out)
    } else if out > 0.0 {
        json!("Infinity")
    } else {
        json!("-Infinity")
    })
}

pub(crate) fn nested_sort_values(idx: &Index, id: &str, sp: &SortSpec, path: &str) -> Vec<Value> {
    let rel = sp
        .field
        .strip_prefix(&format!("{}.", path))
        .unwrap_or(&sp.field)
        .to_string();
    let Some(doc) = idx.docs.get(id) else { return vec![] };
    let Some(items) = field_value(doc, path) else { return vec![] };
    let arr = match items {
        Value::Array(a) => a.clone(),
        other => vec![other.clone()],
    };
    let filter = sp.nested_filter.as_ref().and_then(|f| {
        f.get("term").and_then(|t| t.as_object()).and_then(|m| {
            m.iter().next().map(|(k, v)| {
                let kk = k.strip_prefix(&format!("{}.", path)).unwrap_or(k).to_string();
                (kk, v.clone())
            })
        })
    });
    let mut out = Vec::new();
    for item in arr {
        if let Some((fk, fv)) = &filter {
            let ok = field_value(&item, fk)
                .map(|v| crate::search::eval::array_values(v).iter().any(|x| x == fv))
                .unwrap_or(false);
            if !ok {
                continue;
            }
        }
        if let Some(v) = field_value(&item, &rel) {
            out.extend(flatten_any(v));
        }
    }
    out
}

pub(crate) fn eval_sort_script(store: &Store, sp: &SortSpec, h: &MultiHit) -> Result<Value, ApiError> {
    use crate::search::script_ctx as sc;
    let idx = match store.indices.get(&h.idx_name) {
        Some(i) => i,
        None => return Ok(Value::Null),
    };
    let ctx_name = if sp.script_type == "string" { "string_sort" } else { "number_sort" };
    let cs = sc::prepare(&sp.script_raw, ctx_name).map_err(|e| sc::err_for_shard(idx, e, false))?;
    let qc = QueryCtx::new(idx, &h.id, &h.doc);
    let mut opts = sc::RunOpts { score: h.score, ..Default::default() };
    let v = cs.run(Some(&qc), &mut opts).map_err(|se| sc::script_shard_error(idx, &se))?;
    if ctx_name == "string_sort" {
        return match v {
            Value::String(_) | Value::Null => Ok(v),
            other => {
                let mut se = sc::ScriptErr::new(
                    false,
                    "class_cast_exception",
                    &format!("cannot implicitly cast def [{}] to java.lang.String", sc::def_type_name(&other)),
                );
                se.script = cs.spec.err_name();
                se.stack = vec![cs.spec.src.clone()];
                Err(sc::script_shard_error(idx, &se))
            }
        };
    }
    Ok(match &v {
        Value::Number(n) => sc::f64_value(n.as_f64().unwrap_or(0.0)),
        Value::Bool(b) => sc::f64_value(if *b { 1.0 } else { 0.0 }),
        Value::String(s) if matches!(s.as_str(), "NaN" | "Infinity" | "-Infinity") => v.clone(),
        _ => sc::f64_value(0.0),
    })
}

pub(crate) fn geo_point(v: &Value) -> Option<(f64, f64)> {
    match v {
        Value::Object(m) => Some((
            m.get("lat").and_then(to_f64)?,
            m.get("lon").and_then(to_f64)?,
        )),
        Value::Array(a) if a.len() == 2 => {
            let lon = a[0].as_f64().or_else(|| a[0].as_i64().map(|x| x as f64))?;
            let lat = a[1].as_f64().or_else(|| a[1].as_i64().map(|x| x as f64))?;
            Some((lat, lon))
        }
        Value::String(s) => {
            let t = s.trim();
            if t.to_ascii_uppercase().starts_with("POINT") {
                let inner = t
                    .split_once('(')
                    .and_then(|(_, r)| r.rsplit_once(')'))
                    .map(|(l, _)| l)
                    .unwrap_or(t);
                let parts: Vec<&str> = inner.split_whitespace().collect();
                if parts.len() >= 2 {
                    let lon: f64 = parts[0].parse().ok()?;
                    let lat: f64 = parts[1].parse().ok()?;
                    return Some((lat, lon));
                }
                return None;
            }
            let mut it = t.split(',');
            let lat = it.next()?.trim().parse::<f64>().ok();
            let lon = it.next().and_then(|v| v.trim().parse::<f64>().ok());
            if let (Some(lat), Some(lon)) = (lat, lon) {
                return Some((lat, lon));
            }
            crate::geohash::decode(t)
        }
        _ => None,
    }
}

pub(crate) fn haversine_km(a: (f64, f64), b: (f64, f64)) -> f64 {
    let r = 6371.0;
    let (lat1, lon1) = (a.0.to_radians(), a.1.to_radians());
    let (lat2, lon2) = (b.0.to_radians(), b.1.to_radians());
    let dlat = lat2 - lat1;
    let dlon = lon2 - lon1;
    let x = (dlat / 2.0).sin().powi(2) + lat1.cos() * lat2.cos() * (dlon / 2.0).sin().powi(2);
    2.0 * r * x.sqrt().asin()
}

pub(crate) fn compute_sort_cell(store: &Store, h: &MultiHit, sp: &SortSpec) -> Result<(bool, Value), ApiError> {
    if sp.kind == 1 {
        return Ok((false, json!(h.score)));
    }
    if sp.kind == 2 {
        return Ok((false, json!(h.id)));
    }
    if sp.kind == 3 {
        return Ok((false, json!(h.idx_name)));
    }
    if sp.kind == 4 {
        return Ok((false, json!(h.pos)));
    }
    if sp.kind == 5 {
        return Ok((false, eval_sort_script(store, sp, h)?));
    }
    let idx = &store.indices[&h.idx_name];
    if sp.kind == 6 {
        let origins: Vec<(f64, f64)> = match &sp.geo {
            Value::Array(a)
                if a.len() == 2 && a.iter().all(|v| v.is_number()) =>
            {
                geo_point(&sp.geo).into_iter().collect()
            }
            Value::Array(a) => a.iter().filter_map(geo_point).collect(),
            other => geo_point(other).into_iter().collect(),
        };
        if origins.is_empty() {
            return Ok((false, json!("Infinity")));
        }
        let pts: Vec<(f64, f64)> = match field_value(&h.doc, &sp.field) {
            Some(v @ Value::Array(a)) if a.len() == 2 && a.iter().all(|x| x.is_number()) => {
                geo_point(v).into_iter().collect()
            }
            Some(v) => flatten_any(v).iter().filter_map(geo_point).collect(),
            None => Vec::new(),
        };
        if pts.is_empty() {
            return Ok((false, json!("Infinity")));
        }
        let mut best = f64::INFINITY;
        for p in pts {
          for origin in &origins {
            let mut d = haversine_km(*origin, p);
            match sp.geo_unit.as_str() {
                "km" => {}
                "mi" => d *= 0.621371,
                "yd" => d *= 1093.61,
                "ft" => d *= 3280.84,
                "in" => d *= 39370.1,
                "cm" => d *= 100000.0,
                "mm" => d *= 1_000_000.0,
                "nmi" => d *= 0.539957,
                _ => d *= 1000.0,
            }
            best = best.min(d);
          }
        }
        return Ok((false, if best.is_finite() { json!(best) } else { json!("Infinity") }));
    }
    let nested_missing = sp.nested.is_none()
        && sp.field.contains('.')
        && nested_parent_of(idx, &sp.field).is_some();
    let def = sort_mapping_def(idx, &sp.field);
    let ftype = def
        .as_ref()
        .and_then(|d| d.get("type"))
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    if def.is_none() {
        if let Some(ut) = &sp.unmapped_type {
            let sentinel = match ut.as_str() {
                "long" => json!(9223372036854775807i64),
                "integer" => json!(2147483647i64),
                "double" | "float" => json!("Infinity"),
                _ => Value::Null,
            };
            return Ok((false, sentinel));
        }
        return Ok((true, Value::Null));
    }
    let text_fielddata = ftype == "text"
        && def
            .as_ref()
            .and_then(|d| d.get("fielddata"))
            .and_then(|v| v.as_bool())
            .unwrap_or(false);
    if ftype == "binary" {
        let reason = format!(
            "Can't load fielddata on [{}] because fielddata is unsupported on fields of type [binary]. Use doc values instead.",
            sp.field
        );
        return Err(sort_shard_error("illegal_argument_exception", &reason));
    }
    if ftype == "text" && !text_fielddata {
        let reason = format!(
            "Text fields are not optimised for operations that require per-document field data like aggregations and sorting, so these operations are disabled by default. Please use a keyword field instead. Alternatively, set fielddata=true on [{}] in order to load field data by uninverting the inverted index. Note that this can use significant memory.",
            sp.field
        );
        return Err(sort_shard_error("illegal_argument_exception", &reason));
    }
    let mut vals: Vec<Value> = Vec::new();
    if let Some(path) = &sp.nested {
        let nested_def = mapping_field_def(idx, path);
        if nested_def.map(|d| d.get("type").and_then(|t| t.as_str()) == Some("nested")) != Some(true) {
            return Err(sort_shard_error(
                "query_shard_exception",
                &format!("[nested] failed to find nested object under path [{}]", path),
            ));
        }
        vals = nested_sort_values(idx, &h.id, sp, path);
    } else if nested_missing {
        vals = vec![];
    } else {
        let sf = crate::mapping::resolve_alias_field(idx, &sp.field);
        if let Some(dv) = idx.doc_values.get(&h.id).and_then(|m| m.get(&sf)) {
            vals = crate::search::eval::array_values(dv);
        }
    }
    if vals.is_empty() && sp.nested.is_none() && !nested_missing {
        let sf = crate::mapping::resolve_alias_field(idx, &sp.field);
        vals = source_sort_list(idx, &h.id, &sf);
    }
    if text_fielddata {
        vals = vals
            .iter()
            .filter_map(|v| v.as_str())
            .flat_map(crate::analysis::basic::analyze)
            .map(Value::String)
            .collect();
    }
    vals = vals
        .iter()
        .map(|v| convert_sort_scalar(sp, &ftype, v))
        .collect();
    if vals.is_empty() {
        if let Some(m) = &sp.missing {
            if m.as_str() == Some("_first") {
                let mut v = sort_type_min_max(&ftype, !sp.desc);
                if v.is_null() {
                    v = Value::Null;
                }
                return Ok((false, v));
            }
            if m.as_str() == Some("_last") {
                return Ok((false, sort_missing_sentinel(sp, &ftype, sp.desc)));
            }
            return Ok((false, convert_sort_scalar(sp, &ftype, m)));
        }
        return Ok((true, sort_missing_sentinel(sp, &ftype, sp.desc)));
    }
    let numeric = matches!(ftype.as_str(), "integer" | "short" | "byte" | "long" | "float" | "double" | "half_float" | "scaled_float" | "unsigned_long");
    if !numeric {
        if let Some(mode) = &sp.mode {
            if matches!(mode.as_str(), "avg" | "median" | "sum") {
                return Err(sort_shard_error(
                    "query_shard_exception",
                    "we only support AVG, MEDIAN and SUM on number based fields",
                ));
            }
        }
    }
    match apply_sort_mode(sp, &ftype, vals, sp.desc) {
        Some(v) => Ok((false, v)),
        None => Ok((true, sort_missing_sentinel(sp, &ftype, sp.desc))),
    }
}

pub(crate) fn cmp_sort_cells(a: &(bool, Value), b: &(bool, Value)) -> Ordering {
    match (a.0, b.0) {
        (true, false) => return Ordering::Greater,
        (false, true) => return Ordering::Less,
        _ => {}
    }
    compare_sort_values(&a.1, &b.1)
}

pub(crate) fn sort_after_value(v: &Value, sp: &SortSpec) -> Option<Value> {
    if sp.kind == 0 {
        let idx_dummy_type = sp.field.as_str();
        let _ = idx_dummy_type;
        if let Some(ms) = crate::dates::parse_date(v) {
            if matches!(sp.numeric_type.as_deref(), Some("date_nanos")) {
                return Some(json!((ms * 1_000_000.0) as i64));
            }
            if v.is_string() && sp.field != "" {
                return Some(json!(ms as i64));
            }
        }
    }
    Some(v.clone())
}

pub(crate) fn sort_after_gt(hit: &[Value], after: &[Value], specs: &[SortSpec]) -> bool {
    for (i, sp) in specs.iter().enumerate() {
        let a = hit.get(i).cloned().unwrap_or(Value::Null);
        let b = sort_after_value(&after[i], sp).unwrap_or(Value::Null);
        let mut c = compare_sort_values(&a, &b);
        if sp.desc {
            c = c.reverse();
        }
        match c {
            Ordering::Greater => return true,
            Ordering::Less => return false,
            Ordering::Equal => {}
        }
    }
    false
}

pub(crate) fn sort_after_valid(sp: &SortSpec, v: &Value, numeric: bool) -> bool {
    if sp.kind != 0 {
        return true;
    }
    if !numeric {
        return true;
    }
    if v.is_number() {
        return true;
    }
    let Some(text) = v.as_str() else { return false };
    if crate::dates::parse_date(v).is_some() {
        return true;
    }
    text.parse::<f64>().is_ok()
}
