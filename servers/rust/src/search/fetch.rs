//! Fetch phase helpers: doc values of a hit, `docvalue_fields` / `fields`
//! formatting, script fields, field types.

use crate::dates::*;
use crate::search::eval::{integer_of, QueryCtx};
use crate::mapping::mapping_field_def;
use crate::mapping::fields::doc_values_for;
use crate::errors::ApiError;
use crate::jsonv::{field_value, to_f64, value_to_string};
use crate::model::Index;
use serde_json::{json, Map, Value};
use std::collections::HashMap;

pub(crate) fn query_text(raw: &Value) -> (String, Option<&Map<String, Value>>) {
    crate::search::eval::query_text(raw)
}

pub fn field_type(idx: &Index, field: &str) -> String {
    idx.field_types
        .get(field)
        .cloned()
        .unwrap_or_else(|| "text".to_string())
}

pub(crate) fn is_numeric_type(t: &str) -> bool {
    matches!(
        t,
        "integer" | "long" | "short" | "byte" | "float" | "double" | "half_float"
            | "scaled_float" | "date"
    )
}

/// Builds doc values for docs that have none yet (Go's newQueryCtxForDoc does
/// this lazily and mutates the index; we keep a per-request side table).
pub fn precompute_doc_values(idx: &Index) -> HashMap<String, Map<String, Value>> {
    let mut extra = HashMap::new();
    if crate::search::index::all_docs_have_values(idx) == Some(true) {
        return extra;
    }
    for id in &idx.order {
        if idx.doc_values.contains_key(id) || idx.pending.contains_key(id) {
            continue;
        }
        let doc = match idx.docs.get(id) {
            Some(d) => d,
            None => continue,
        };
        if let Some(m) = doc.as_object() {
            if let Ok((vals, _)) = doc_values_for(idx, id, m) {
                let mut vm = Map::new();
                for (k, v) in vals {
                    vm.insert(k, Value::Array(v));
                }
                extra.insert(id.clone(), vm);
            }
        }
    }
    extra
}

pub fn eval_doc(
    idx: &Index,
    id: &str,
    doc: &Value,
    extra: &HashMap<String, Map<String, Value>>,
    query: &Map<String, Value>,
) -> Result<(bool, f64), ApiError> {
    match extra.get(id) {
        Some(vals) => QueryCtx::with_values(idx, id, doc, vals.clone()).eval(query),
        None => QueryCtx::new(idx, id, doc).eval(query),
    }
}

pub(crate) fn search_phase_error(reason: &str) -> ApiError {
    let mut e = ApiError::new("search_phase_execution_exception", "all shards failed", 400);
    e.root_cause = Some(Box::new(ApiError::new(
        "illegal_argument_exception",
        reason,
        400,
    )));
    e.caused_by = Some(Box::new(ApiError::new(
        "illegal_argument_exception",
        reason,
        400,
    )));
    e
}

pub(crate) struct FieldSpec {
    pub(crate) name: String,
    pub(crate) format: String,
}

pub(crate) fn parse_field_specs(raw: Option<&Value>) -> Vec<FieldSpec> {
    let mut out = vec![];
    let list = match raw.and_then(|v| v.as_array()) {
        Some(l) => l,
        None => return out,
    };
    for e in list {
        match e {
            Value::String(s) => out.push(FieldSpec {
                name: s.clone(),
                format: String::new(),
            }),
            Value::Object(m) => {
                let name = m
                    .get("field")
                    .and_then(value_to_string)
                    .unwrap_or_default();
                if name.is_empty() {
                    continue;
                }
                let format = m
                    .get("format")
                    .and_then(value_to_string)
                    .unwrap_or_default();
                out.push(FieldSpec { name, format });
            }
            _ => {}
        }
    }
    out
}

pub(crate) fn text_like_type(t: &str) -> bool {
    matches!(
        t,
        "text" | "match_only_text" | "search_as_you_type" | "completion" | "annotated_text"
    )
}

pub(crate) fn flatten_any(v: &Value) -> Vec<Value> {
    match v {
        Value::Array(a) => {
            let mut out = vec![];
            for e in a {
                out.extend(flatten_any(e));
            }
            out
        }
        Value::Null => vec![],
        other => vec![other.clone()],
    }
}

pub(crate) fn format_doc_value(ftype: &str, v: &Value, format: &str) -> Value {
    match ftype {
        "date_nanos" => {
            let Some(ns) = integer_of(v) else {
                return v.clone();
            };
            let ms = ns.div_euclid(1_000_000);
            if matches!(
                format,
                "strict_date_optional_time_nanos" | "date_optional_time_nanos"
            ) {
                let (y, mo, d, hh, mi, ss, millis) = date_parts(ms);
                let nanos = ns.rem_euclid(1_000_000_000);
                return Value::String(format!(
                    "{:04}-{:02}-{:02}T{:02}:{:02}:{:02}.{:03}{:06}Z",
                    y, mo, d, hh, mi, ss, millis,
                    nanos % 1_000_000
                ));
            }
            match format {
                "" | "strict_date_optional_time" | "date_optional_time"
                | "strict_date_optional_time_nanos" | "date_optional_time_nanos" => {
                    Value::String(format_epoch_ms(ms))
                }
                "epoch_millis" => Value::String(ms.to_string()),
                "epoch_second" => Value::String((ms / 1000).to_string()),
                other => match apply_java_date_format(other, ms) {
                    Some(s) => Value::String(s),
                    None => Value::String(ms.to_string()),
                },
            }
        }
        "ip" => match v.as_str().and_then(|s| crate::mapping::fields::canonical_ip(s).ok()) {
            Some(c) => Value::String(c),
            None => v.clone(),
        },
        "date" => {
            let ms = match integer_of(v) {
                Some(n) => n,
                None => return v.clone(),
            };
            match format {
                "" | "strict_date_optional_time" | "date_optional_time"
                | "strict_date_optional_time_nanos" | "date_optional_time_nanos" => {
                    Value::String(format_epoch_ms(ms))
                }
                "epoch_millis" => Value::String(ms.to_string()),
                "epoch_second" => Value::String((ms / 1000).to_string()),
                other => match apply_java_date_format(other, ms) {
                    Some(s) => Value::String(s),
                    None => Value::String(ms.to_string()),
                },
            }
        }
        "float" | "double" | "half_float" | "scaled_float" => {
            if format.is_empty() || format == "use_field_mapping" {
                return v.clone();
            }
            let decimals = format
                .split_once('.')
                .map(|(_, f)| f.chars().filter(|c| *c == '0' || *c == '#').count())
                .unwrap_or(0);
            match to_f64(v) {
                Some(n) => Value::String(format!("{:.*}", decimals, n)),
                None => v.clone(),
            }
        }
        _ => v.clone(),
    }
}

pub(crate) fn geo_pair(v: &Value) -> Option<(f64, f64)> {
    let o = v.as_object()?;
    let lat = o.get("lat").and_then(|x| x.as_f64())?;
    let lon = o.get("lon").and_then(|x| x.as_f64())?;
    Some((lat, lon))
}

pub(crate) fn geo_num(n: f64) -> String {
    if n.fract() == 0.0 {
        format!("{:.1}", n)
    } else {
        format!("{}", n)
    }
}

pub(crate) fn format_geo(v: &Value, wkt: bool, as_shape: bool) -> Value {
    let Some((lat, lon)) = geo_pair(v) else { return v.clone() };
    if wkt {
        return json!(format!("POINT ({} {})", geo_num(lon), geo_num(lat)));
    }
    if as_shape {
        return json!({"type": "Point", "coordinates": [lon, lat]});
    }
    json!(format!("{}, {}", geo_num(lat), geo_num(lon)))
}

pub(crate) fn doc_values_output(
    idx: &Index,
    id: &str,
    field: &str,
    spec: &FieldSpec,
    fallback_source: bool,
) -> Result<Option<Vec<Value>>, ApiError> {
    let resolved = crate::mapping::resolve_alias_field(idx, field);
    let mut field = resolved.as_str();
    let mut join_suffix: Option<String> = None;
    if mapping_field_def(idx, field).is_none() {
        if let Some((base, suffix)) = field.split_once('#') {
            if mapping_field_def(idx, base)
                .and_then(|d| d.get("type"))
                .and_then(|t| t.as_str())
                == Some("join")
            {
                join_suffix = Some(suffix.to_string());
                field = base;
            }
        }
    }
    let def = mapping_field_def(idx, field);
    if let Some(out) = crate::search::knn_query::docvalue_output(idx, id, field, def, fallback_source) {
        return Ok(out);
    }
    let (ftype, has_dv) = match def {
        Some(d) => (
            d.get("type")
                .and_then(|t| t.as_str())
                .unwrap_or("")
                .to_string(),
            !matches!(d.get("doc_values").and_then(|v| v.as_bool()), Some(false)),
        ),
        None => (String::new(), true),
    };
    let mut out_format = spec.format.clone();
    if out_format.is_empty() {
        if let Some(d) = def {
            if matches!(ftype.as_str(), "date" | "date_nanos") {
                if let Some(mf) = d.get("format").and_then(|v| v.as_str()) {
                    if !mf.is_empty()
                        && !mf.contains("||")
                        && !matches!(
                            mf,
                            "strict_date_optional_time"
                                | "date_optional_time"
                                | "strict_date_optional_time_nanos"
                                | "epoch_millis"
                                | "epoch_second"
                                | "date_time"
                                | "basic_date_time"
                        )
                    {
                        out_format = mf.to_string();
                    }
                }
            }
        }
    }
    let vals: Option<Vec<Value>> = idx
        .doc_values
        .get(id)
        .and_then(|dv| dv.get(field))
        .map(crate::search::eval::array_values);
    if text_like_type(&ftype) || !has_dv {
        if !fallback_source {
            if !has_dv && vals.as_ref().map(|v| !v.is_empty()).unwrap_or(false) {
                let reason = format!(
                    "Can't load fielddata on [{}] because fielddata is unsupported on fields of type [{}]. Use doc values instead.",
                    field, ftype
                );
                return Err(search_phase_error(&reason));
            }
            return Ok(None);
        }
        if let Some(doc) = idx.docs.get(id) {
            if let Some(v) = field_value(doc, field) {
                let list = flatten_any(v);
                if ftype == "geo_point" {
                    return Ok(Some(
                        list.iter()
                            .map(|x| {
                                format_geo(x, out_format == "wkt", out_format.is_empty())
                            })
                            .collect(),
                    ));
                }
                if ftype.is_empty() && fallback_source && list.iter().all(|x| x.is_object()) {
                    return Ok(None);
                }
                return Ok(Some(
                    list.iter()
                        .map(|x| format_doc_value(&ftype, x, &out_format))
                        .collect(),
                ));
            }
        }
        return Ok(None);
    }
    let vals = match vals {
        Some(v) if out_format.is_empty() || out_format == "use_field_mapping" => {
            Some(crate::search::javanum::java_field_values(idx, id, field, &ftype, v, fallback_source))
        }
        other => other,
    };
    match vals {
        Some(v) if !v.is_empty() => Ok(Some(
            v.iter()
                .map(|x| {
                    if ftype == "join" {
                        return join_output_value(x, join_suffix.as_deref());
                    }
                    if ftype == "geo_point" {
                        format_geo(x, out_format == "wkt", fallback_source && out_format.is_empty())
                    } else {
                        format_doc_value(&ftype, x, &out_format)
                    }
                })
                .collect(),
        )),
        _ => Ok(None),
    }
}

pub(crate) fn join_output_value(v: &Value, suffix: Option<&str>) -> Value {
    let (name, parent) = match v {
        Value::String(s) => (s.clone(), String::new()),
        Value::Object(m) => (
            m.get("name")
                .and_then(|x| x.as_str())
                .unwrap_or("")
                .to_string(),
            m.get("parent")
                .and_then(|x| x.as_str())
                .unwrap_or("")
                .to_string(),
        ),
        _ => (String::new(), String::new()),
    };
    match suffix {
        Some(s) if s == name => Value::String(parent),
        Some(s) => {
            let _ = s;
            Value::String(parent)
        }
        None => Value::String(name),
    }
}

/// Evaluates one script field for a hit; None omits the field.
pub(crate) fn eval_script_field(
    idx: &Index,
    id: &str,
    doc: &Value,
    sf: &Value,
    source: &mut Option<Value>,
) -> Result<Option<Vec<Value>>, ApiError> {
    use crate::search::script_ctx as sc;
    let (raw, ignore_failure) = match sf {
        Value::Object(m) if m.contains_key("script") => (
            m.get("script").cloned().unwrap_or(Value::Null),
            m.get("ignore_failure").and_then(|v| v.as_bool()).unwrap_or(false),
        ),
        other => (other.clone(), false),
    };
    let cs = sc::prepare(&raw, "field").map_err(|e| sc::err_for_shard(idx, e, false))?;
    let qc = QueryCtx::new(idx, id, doc);
    let mut opts = sc::RunOpts { source: source.take(), ..Default::default() };
    let res = cs.run(Some(&qc), &mut opts);
    *source = opts.source.take();
    match res {
        Ok(v) => Ok(sc::field_script_values(v)),
        Err(_) if ignore_failure => Ok(None),
        Err(se) => Err(sc::script_shard_error(idx, &se)),
    }
}
