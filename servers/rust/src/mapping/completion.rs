//! Completion fields at index time: mapping normalization and validation,
//! context mappings (category and geo), and parsing of document values into
//! weighted inputs.

use crate::jsonv::{doc_strings, field_value, to_f64, value_to_string};
use serde_json::{json, Map, Value};
use crate::errors::ApiError;

#[derive(Clone, Copy, PartialEq)]
pub(crate) enum CtxType {
    Category,
    Geo,
}

#[derive(Clone)]
pub(crate) struct CtxMapping {
    pub(crate) name: String,
    pub(crate) typ: CtxType,
    pub(crate) precision: usize,
    pub(crate) path: Option<String>,
}

pub(crate) fn context_mappings(def: &Map<String, Value>) -> Vec<CtxMapping> {
    let mut out = vec![];
    if let Some(arr) = def.get("contexts").and_then(|v| v.as_array()) {
        for c in arr {
            let Some(m) = c.as_object() else { continue };
            let name = m.get("name").and_then(value_to_string).unwrap_or_default();
            let typ = match m.get("type").and_then(|v| v.as_str()).map(|s| s.to_lowercase()).as_deref() {
                Some("geo") => CtxType::Geo,
                _ => CtxType::Category,
            };
            let precision = m.get("precision").and_then(geo_precision).unwrap_or(6);
            let path = m.get("path").and_then(value_to_string);
            out.push(CtxMapping { name, typ, precision, path });
        }
    }
    out
}

/// A geohash precision given as a level or as a distance ("5km").
pub(crate) fn geo_precision(v: &Value) -> Option<usize> {
    if let Some(n) = v.as_i64() {
        return Some(n.clamp(1, 12) as usize);
    }
    let s = v.as_str()?.trim().to_string();
    if let Ok(n) = s.parse::<i64>() {
        return Some(n.clamp(1, 12) as usize);
    }
    let meters = crate::jsonv::to_f64(&json!(s.trim_end_matches(|c: char| c.is_alphabetic())))?
        * match s.trim_start_matches(|c: char| c.is_ascii_digit() || c == '.').to_lowercase().as_str() {
            "km" => 1000.0,
            "mi" | "miles" => 1609.344,
            "cm" => 0.01,
            "mm" => 0.001,
            _ => 1.0,
        };
    // Width of a geohash cell at the equator, per level.
    let widths = [
        5_009_400.0, 1_252_300.0, 156_500.0, 39_100.0, 4_900.0, 1_200.0, 152.9, 38.2, 4.8, 1.2, 0.149, 0.037,
    ];
    for (i, w) in widths.iter().enumerate() {
        if *w <= meters {
            return Some(i + 1);
        }
    }
    Some(12)
}

/// Shows the defaults OpenSearch reports for a completion field mapping.
pub fn normalize_completion_def(m: &mut Map<String, Value>) {
    let mut out = Map::new();
    out.insert("type".into(), json!("completion"));
    out.insert("analyzer".into(), m.get("analyzer").cloned().unwrap_or(json!("simple")));
    if let Some(sa) = m.get("search_analyzer") {
        out.insert("search_analyzer".into(), sa.clone());
    }
    out.insert(
        "preserve_separators".into(),
        m.get("preserve_separators").cloned().map(bool_like).unwrap_or(json!(true)),
    );
    out.insert(
        "preserve_position_increments".into(),
        m.get("preserve_position_increments").cloned().map(bool_like).unwrap_or(json!(true)),
    );
    out.insert(
        "max_input_length".into(),
        m.get("max_input_length").cloned().map(int_like).unwrap_or(json!(50)),
    );
    if let Some(Value::Array(ctxs)) = m.get("contexts") {
        let mut norm = vec![];
        for c in ctxs {
            let Some(cm) = c.as_object() else {
                norm.push(c.clone());
                continue;
            };
            let typ = cm.get("type").and_then(|v| v.as_str()).map(|s| s.to_lowercase());
            if cm.get("name").is_none() || !matches!(typ.as_deref(), Some("category") | Some("geo")) {
                norm.push(c.clone());
                continue;
            }
            let mut n = Map::new();
            n.insert("name".into(), cm["name"].clone());
            let geo = typ.as_deref() == Some("geo");
            n.insert("type".into(), json!(if geo { "GEO" } else { "CATEGORY" }));
            if geo {
                n.insert("precision".into(), json!(cm.get("precision").and_then(geo_precision).unwrap_or(6)));
            }
            if let Some(p) = cm.get("path") {
                n.insert("path".into(), p.clone());
            }
            norm.push(Value::Object(n));
        }
        out.insert("contexts".into(), Value::Array(norm));
    }
    for (k, v) in m.iter() {
        if !out.contains_key(k) {
            out.insert(k.clone(), v.clone());
        }
    }
    *m = out;
}

pub(crate) fn bool_like(v: Value) -> Value {
    match &v {
        Value::String(s) if s == "true" => json!(true),
        Value::String(s) if s == "false" => json!(false),
        _ => v,
    }
}

pub(crate) fn int_like(v: Value) -> Value {
    match &v {
        Value::String(s) => s.parse::<i64>().map(|n| json!(n)).unwrap_or(v),
        _ => v,
    }
}

pub(crate) fn mapping_err(reason: &str, cause_type: &str, root_is_cause: bool) -> ApiError {
    let mut e = ApiError::new("mapper_parsing_exception", &format!("Failed to parse mapping [_doc]: {}", reason), 400);
    e.caused_by = Some(Box::new(ApiError::new(cause_type, reason, 400)));
    if root_is_cause {
        e.root_cause = Some(Box::new(ApiError::new(cause_type, reason, 400)));
    }
    e
}

/// Validates completion field parameters in a mapping's properties.
pub fn validate_completion_props(props: &Map<String, Value>) -> Result<(), ApiError> {
    for raw in props.values() {
        let Some(def) = raw.as_object() else { continue };
        if def.get("type").and_then(|v| v.as_str()) == Some("completion") {
            if let Some(n) = def.get("max_input_length").and_then(|v| v.as_i64()) {
                if n <= 0 {
                    return Err(mapping_err(
                        &format!("[max_input_length] must be > 0 but was [{}]", n),
                        "illegal_argument_exception",
                        false,
                    ));
                }
            }
            if let Some(ctxs) = def.get("contexts") {
                let list: Vec<&Value> = match ctxs {
                    Value::Array(a) => a.iter().collect(),
                    other => vec![other],
                };
                for c in list {
                    let Some(cm) = c.as_object() else { continue };
                    if !cm.contains_key("name") {
                        return Err(mapping_err("missing [name] in context mapping", "parse_exception", true));
                    }
                    let Some(t) = cm.get("type").and_then(value_to_string) else {
                        return Err(mapping_err("missing [type] in context mapping", "parse_exception", true));
                    };
                    if !matches!(t.to_lowercase().as_str(), "category" | "geo") {
                        return Err(mapping_err(
                            &format!("No context type for [{}]", t),
                            "illegal_argument_exception",
                            false,
                        ));
                    }
                }
            }
        }
        if let Some(sub) = def.get("properties").and_then(|p| p.as_object()) {
            validate_completion_props(sub)?;
        }
        if let Some(sub) = def.get("fields").and_then(|p| p.as_object()) {
            validate_completion_props(sub)?;
        }
    }
    Ok(())
}

/// Parses a geo point: {lat, lon}, "lat,lon", a geohash, or [lon, lat].
pub(crate) fn parse_point(v: &Value) -> Option<(f64, f64)> {
    match v {
        Value::Object(m) => Some((m.get("lat").and_then(to_f64)?, m.get("lon").and_then(to_f64)?)),
        Value::String(s) => {
            if let Some((a, b)) = s.split_once(',') {
                return Some((a.trim().parse().ok()?, b.trim().parse().ok()?));
            }
            let (a, b, c, d) = crate::geohash::bounds(s)?;
            Some(((a + b) / 2.0, (c + d) / 2.0))
        }
        Value::Array(a) if a.len() >= 2 && a[0].is_number() => Some((to_f64(&a[1])?, to_f64(&a[0])?)),
        _ => None,
    }
}

pub(crate) fn unknown_ctx_reason(maps: &[CtxMapping], name: &str) -> String {
    let listed: Vec<&str> = maps.iter().map(|m| m.name.as_str()).collect();
    format!("Unknown context name [{}], must be one of [{}]", name, listed.join(", "))
}

pub(crate) fn failed_to_parse(cause_type: &str, reason: &str) -> ApiError {
    let mut e = ApiError::new("mapper_parsing_exception", "failed to parse", 400);
    e.caused_by = Some(Box::new(ApiError::new(cause_type, reason, 400)));
    e
}

pub(crate) fn json_kind(v: &Value) -> &'static str {
    match v {
        Value::Null => "VALUE_NULL",
        Value::Bool(_) => "VALUE_BOOLEAN",
        Value::Number(_) => "VALUE_NUMBER",
        Value::String(_) => "VALUE_STRING",
        Value::Array(_) => "START_ARRAY",
        Value::Object(_) => "START_OBJECT",
    }
}

pub(crate) fn check_reserved(s: &str) -> Result<(), ApiError> {
    let mut pos = 0usize;
    for ch in s.chars() {
        let cp = ch as u32;
        if cp == 0x1f || cp == 0x1e || cp == 0 {
            return Err(failed_to_parse(
                "illegal_argument_exception",
                &format!(
                    "Illegal input [{}] UTF-16 codepoint [0x{:x}] at position {} is a reserved character",
                    s, cp, pos
                ),
            ));
        }
        pos += ch.len_utf16();
    }
    Ok(())
}

/// One suggestion path source: an input with its weight and contexts.
#[derive(Clone)]
pub(crate) struct Input {
    pub(crate) text: String,
    pub(crate) weight: i64,
    pub(crate) contexts: Vec<(usize, String)>,
}

/// Context values of the documents's own fields (`path` contexts).
pub(crate) fn path_contexts(maps: &[CtxMapping], doc: &Value) -> Vec<(usize, String)> {
    let mut out = vec![];
    for (mi, m) in maps.iter().enumerate() {
        let Some(p) = &m.path else { continue };
        let Some(v) = field_value(doc, p) else { continue };
        match m.typ {
            CtxType::Category => {
                for s in doc_strings(v) {
                    out.push((mi, s));
                }
            }
            CtxType::Geo => {
                let pts: Vec<&Value> = match v {
                    Value::Array(a) if !a.first().map(|x| x.is_number()).unwrap_or(false) => a.iter().collect(),
                    other => vec![other],
                };
                for p in pts {
                    if let Some((lat, lon)) = parse_point(p) {
                        out.push((mi, crate::geohash::encode(lat, lon, m.precision)));
                    }
                }
            }
        }
    }
    out
}

pub(crate) fn parse_doc_contexts(maps: &[CtxMapping], v: &Value) -> Result<Vec<(usize, String)>, ApiError> {
    let mut out = vec![];
    let Some(m) = v.as_object() else { return Ok(out) };
    for (name, cv) in m {
        let Some(mi) = maps.iter().position(|c| &c.name == name) else {
            return Err(failed_to_parse("illegal_argument_exception", &unknown_ctx_reason(maps, name)));
        };
        let cm = &maps[mi];
        match cm.typ {
            CtxType::Category => {
                let vals: Vec<&Value> = match cv {
                    Value::Array(a) => a.iter().collect(),
                    other => vec![other],
                };
                for x in vals {
                    match x {
                        Value::String(_) | Value::Number(_) | Value::Bool(_) => {
                            out.push((mi, value_to_string(x).unwrap_or_default()))
                        }
                        Value::Null => {}
                        _ => {
                            return Err(failed_to_parse(
                                "parse_exception",
                                &format!("context array must have string, number or boolean values, but was [{}]", json_kind(x)),
                            ))
                        }
                    }
                }
            }
            CtxType::Geo => {
                let pts: Vec<&Value> = match cv {
                    Value::Array(a) if !a.first().map(|x| x.is_number()).unwrap_or(false) => a.iter().collect(),
                    other => vec![other],
                };
                for p in pts {
                    if let Value::String(s) = p {
                        if !s.contains(',') {
                            out.push((mi, s.chars().take(cm.precision).collect()));
                            continue;
                        }
                    }
                    if let Some((lat, lon)) = parse_point(p) {
                        out.push((mi, crate::geohash::encode(lat, lon, cm.precision)));
                    }
                }
            }
        }
    }
    Ok(out)
}

pub(crate) fn parse_weight(w: &Value) -> Result<i64, ApiError> {
    let wi = match w {
        Value::Number(n) => n.as_i64(),
        Value::String(s) => s.parse::<i64>().ok().filter(|_| s.trim() == *s),
        _ => None,
    };
    match wi {
        Some(n) if (0..=i32::MAX as i64).contains(&n) => Ok(n),
        Some(n) => Err(failed_to_parse(
            "illegal_argument_exception",
            &format!("weight must be in the interval [0..2147483647], but was [{}]", n),
        )),
        None => Err(failed_to_parse(
            "illegal_argument_exception",
            &format!("weight must be an integer, but was [{}]", crate::mapping::fields::preview_value(w)),
        )),
    }
}

pub(crate) fn parse_inputs(
    path: &str,
    maps: &[CtxMapping],
    v: &Value,
    out: &mut Vec<Input>,
    top: bool,
) -> Result<(), ApiError> {
    let bad = |other: &Value| {
        let reason = format!("failed to parse [{}]: expected text or object, but got {}", path, json_kind(other));
        let mut e = ApiError::new("mapper_parsing_exception", "failed to parse", 400);
        e.caused_by = Some(Box::new(ApiError::new("parsing_exception", &reason, 400)));
        e.root_cause = Some(Box::new(ApiError::new("parsing_exception", &reason, 400)));
        e
    };
    match v {
        Value::String(s) => {
            check_reserved(s)?;
            out.push(Input { text: s.clone(), weight: 1, contexts: vec![] });
        }
        Value::Array(a) if top => {
            for e in a {
                parse_inputs(path, maps, e, out, false)?;
            }
        }
        Value::Object(m) => {
            for k in m.keys() {
                if !matches!(k.as_str(), "input" | "weight" | "contexts") {
                    return Err(failed_to_parse(
                        "illegal_argument_exception",
                        &format!("unknown field name [{}], must be one of [input, weight, contexts]", k),
                    ));
                }
            }
            let weight = match m.get("weight") {
                Some(w) => parse_weight(w)?,
                None => 1,
            };
            let contexts = match m.get("contexts") {
                Some(c) if !c.is_null() => {
                    if maps.is_empty() {
                        vec![]
                    } else {
                        parse_doc_contexts(maps, c)?
                    }
                }
                _ => vec![],
            };
            let mut texts = vec![];
            match m.get("input") {
                Some(Value::String(s)) => texts.push(s.clone()),
                Some(Value::Array(a)) => {
                    for e in a {
                        match e {
                            Value::String(s) => texts.push(s.clone()),
                            Value::Null => {}
                            other => return Err(bad(other)),
                        }
                    }
                }
                Some(Value::Null) | None => {}
                Some(other) => return Err(bad(other)),
            }
            for t in texts {
                check_reserved(&t)?;
                out.push(Input { text: t, weight, contexts: contexts.clone() });
            }
        }
        Value::Null => {}
        other => return Err(bad(other)),
    }
    Ok(())
}

pub(crate) fn collect_inputs(path: &str, def: &Map<String, Value>, v: &Value, doc: &Value) -> Result<Vec<Input>, ApiError> {
    let maps = context_mappings(def);
    let mut inputs = vec![];
    parse_inputs(path, &maps, v, &mut inputs, true)?;
    if !maps.is_empty() {
        let extra = path_contexts(&maps, doc);
        for i in inputs.iter_mut() {
            for c in &extra {
                if !i.contexts.contains(c) {
                    i.contexts.push(c.clone());
                }
            }
        }
    }
    Ok(inputs)
}

/// Index-time parsing of a completion field value; returns the inputs.
pub fn parse_completion(
    path: &str,
    def: &Map<String, Value>,
    v: &Value,
    doc: &Map<String, Value>,
) -> Result<Value, ApiError> {
    let docv = Value::Object(doc.clone());
    let inputs = collect_inputs(path, def, v, &docv)?;
    if !context_mappings(def).is_empty() && inputs.iter().any(|i| i.contexts.is_empty()) {
        return Err(ApiError::new(
            "illegal_argument_exception",
            &format!("Contexts are mandatory in context enabled completion field [{}]", path),
            400,
        ));
    }
    Ok(Value::Array(inputs.into_iter().map(|i| Value::String(i.text)).collect()))
}
