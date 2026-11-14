// knn_vector mapping parsing, validation and normalization (k-NN plugin).

use crate::errors::ApiError;
use serde_json::{json, Map, Value};

const SPACES: [&str; 6] = ["l2", "cosinesimil", "l1", "linf", "innerproduct", "hamming"];
const KNOWN_PARAMS: [&str; 9] = [
    "type", "dimension", "data_type", "space_type", "method", "mode", "compression_level",
    "doc_values", "model_id",
];

fn mapping_err(cause_type: &str, reason: &str) -> ApiError {
    let mut e = ApiError::new(
        "mapper_parsing_exception",
        &format!("Failed to parse mapping [_doc]: {}", reason),
        400,
    );
    e.caused_by = Some(Box::new(ApiError::new(cause_type, reason, 400)));
    e
}

fn validation(msg: &str) -> ApiError {
    mapping_err("validation_exception", &format!("Validation Failed: 1: {};", msg))
}

fn as_str_val(v: &Value) -> String {
    match v {
        Value::String(s) => s.clone(),
        other => other.to_string(),
    }
}

/// Validate and normalize one knn_vector field definition in place.
fn normalize_field(name: &str, def: &mut Map<String, Value>, knn: bool) -> Result<(), ApiError> {
    for k in def.keys() {
        if !KNOWN_PARAMS.contains(&k.as_str()) {
            return Err(mapping_err(
                "mapper_parsing_exception",
                &format!("unknown parameter [{}] on mapper [{}] of type [knn_vector]", k, name),
            ));
        }
    }
    let dim = match def.get("dimension") {
        None => return Ok(()),
        Some(v) => {
            let parsed = match v {
                Value::Number(n) => n.as_i64(),
                Value::String(s) => s.trim().parse::<i64>().ok(),
                _ => None,
            };
            match parsed {
                Some(d) => d,
                None => {
                    return Err(mapping_err(
                        "illegal_argument_exception",
                        &format!(
                            "Unable to parse [dimension] from provided value [{}] for vector [{}]",
                            as_str_val(v),
                            name
                        ),
                    ))
                }
            }
        }
    };
    if dim <= 0 {
        return Err(mapping_err(
            "illegal_argument_exception",
            &format!("Dimension value must be greater than 0 for vector: {}", name),
        ));
    }
    def.insert("dimension".to_string(), json!(dim));
    let data_type = match def.get("data_type") {
        None => "float".to_string(),
        Some(v) => {
            let s = as_str_val(v).to_lowercase();
            if !["binary", "byte", "float", "half_float"].contains(&s.as_str()) {
                return Err(mapping_err(
                    "illegal_argument_exception",
                    "Invalid value provided for [data_type] field. Supported values are [binary,byte,float,half_float]",
                ));
            }
            s
        }
    };
    if let Some(v) = def.get("mode") {
        let s = as_str_val(v);
        if !["in_memory", "on_disk"].contains(&s.as_str()) {
            return Err(mapping_err(
                "mapper_parsing_exception",
                &format!("Unknown value [{}] for field [mode] - accepted values are [null, in_memory, on_disk]", s),
            ));
        }
    }
    if let Some(v) = def.get("compression_level") {
        let s = as_str_val(v);
        if !["1x", "2x", "4x", "8x", "16x", "32x"].contains(&s.as_str()) {
            return Err(mapping_err(
                "mapper_parsing_exception",
                &format!("Unknown value [{}] for field [compression_level] - accepted values are [, 1x, 2x, 4x, 8x, 16x, 32x]", s),
            ));
        }
    }
    let top_space = def.get("space_type").map(as_str_val);
    let method = def.get("method").and_then(|m| m.as_object()).cloned();
    let mut engine = "faiss".to_string();
    let mut method_name: Option<String> = None;
    let mut method_space: Option<String> = None;
    if let Some(m) = &method {
        if let Some(e) = m.get("engine") {
            let e = as_str_val(e);
            if !["faiss", "lucene", "nmslib"].contains(&e.as_str()) {
                return Err(mapping_err("mapper_parsing_exception", &format!("Invalid engine: {}", e)));
            }
            engine = e;
        }
        match m.get("name") {
            Some(n) => method_name = Some(as_str_val(n)),
            None => return Err(mapping_err("mapper_parsing_exception", "name needs to be set")),
        }
        method_space = m.get("space_type").map(as_str_val);
        if let (Some(a), Some(b)) = (&method_space, &top_space) {
            if a != b {
                return Err(mapping_err(
                    "mapper_parsing_exception",
                    &format!("Cannot specify conflicting space types: \"[{}]\" \"[{}]\"", a, b),
                ));
            }
        }
    }
    let space = method_space.clone().or_else(|| top_space.clone());
    if let Some(s) = &space {
        if !SPACES.contains(&s.as_str()) {
            return Err(mapping_err(
                "illegal_argument_exception",
                &format!(
                    "Unable to find space: {} . Valid values are: [l2, cosinesimil, l1, linf, innerproduct, hamming]",
                    s
                ),
            ));
        }
        let binary = data_type == "binary";
        if (s == "hamming") != binary {
            return Err(mapping_err(
                "illegal_argument_exception",
                &format!("Space type [{}] is not supported with [{}] data type", s, data_type),
            ));
        }
    }
    if engine == "nmslib" {
        return Err(mapping_err(
            "illegal_argument_exception",
            "nmslib engine is deprecated in OpenSearch  and cannot be used for new index creation in OpenSearch from  3.0.0.",
        ));
    }
    if let Some(n) = &method_name {
        let ok = match engine.as_str() {
            "lucene" => n == "hnsw",
            _ => n == "hnsw" || n == "ivf",
        };
        if !ok {
            return Err(mapping_err("illegal_argument_exception", &format!("Invalid method name: {}", n)));
        }
        if n == "ivf" && knn {
            return Err(validation("\"method\" requires training."));
        }
    }
    if knn {
        if let Some(params) = method.as_ref().and_then(|m| m.get("parameters")).and_then(|p| p.as_object()) {
            for (k, v) in params {
                match k.as_str() {
                    "m" | "ef_construction" | "ef_search" => {
                        if !v.is_i64() {
                            return Err(validation(&format!(
                                "value is not an instance of Integer for Integer parameter [{}].",
                                k
                            )));
                        }
                    }
                    "encoder" => {}
                    _ => return Err(validation(&format!("Unknown parameter '{}' found", k))),
                }
            }
        }
        if dim > 16000 {
            return Err(validation(&format!(
                "Dimension value cannot be greater than 16000 for vector with engine: {}",
                engine
            )));
        }
        if data_type == "binary" && dim % 8 != 0 {
            return Err(validation("Dimension should be multiply of 8 for binary vector data type"));
        }
        if let Some(s) = &space {
            if s == "l1" || s == "linf" {
                return Err(validation(&format!(
                    "\"{}\" with \"{}\" configuration does not support space type: \"{}\".",
                    method_name.as_deref().unwrap_or("hnsw"),
                    engine,
                    s
                )));
            }
        }
    }
    // Normalized view.
    let old = std::mem::take(def);
    let mut out = Map::new();
    out.insert("type".to_string(), json!("knn_vector"));
    if !knn {
        out.insert("doc_values".to_string(), old.get("doc_values").cloned().unwrap_or(json!(true)));
    } else if let Some(dv) = old.get("doc_values") {
        out.insert("doc_values".to_string(), dv.clone());
    }
    out.insert("dimension".to_string(), json!(dim));
    if old.contains_key("data_type") {
        out.insert("data_type".to_string(), json!(data_type.to_uppercase()));
    }
    for k in ["mode", "compression_level", "model_id"] {
        if let Some(v) = old.get(k) {
            out.insert(k.to_string(), v.clone());
        }
    }
    if let Some(m) = &method {
        let mut nm = Map::new();
        nm.insert("engine".to_string(), json!(engine));
        let ms = space.clone().unwrap_or_else(|| {
            if data_type == "binary" { "hamming".to_string() } else { "l2".to_string() }
        });
        nm.insert("space_type".to_string(), json!(ms));
        nm.insert("name".to_string(), json!(method_name.clone().unwrap_or_default()));
        let params = m.get("parameters").cloned().unwrap_or_else(|| json!({}));
        let params = match params {
            Value::Object(p) => {
                let mut keys: Vec<&String> = p.keys().collect();
                keys.sort();
                let mut sp = Map::new();
                for k in keys {
                    sp.insert(k.clone(), p[k].clone());
                }
                Value::Object(sp)
            }
            other => other,
        };
        nm.insert("parameters".to_string(), params);
        out.insert("method".to_string(), Value::Object(nm));
    }
    if let Some(s) = &top_space {
        out.insert("space_type".to_string(), json!(s));
    }
    *def = out;
    Ok(())
}

/// Walk mapping properties and normalize every knn_vector field.
pub fn normalize_knn_props(props: &mut Map<String, Value>, knn: bool) -> Result<(), ApiError> {
    for (name, def) in props.iter_mut() {
        let m = match def.as_object_mut() {
            Some(m) => m,
            None => continue,
        };
        if m.get("type").and_then(|t| t.as_str()) == Some("knn_vector") {
            normalize_field(name, m, knn)?;
            continue;
        }
        if let Some(sub) = m.get_mut("properties").and_then(|p| p.as_object_mut()) {
            normalize_knn_props(sub, knn)?;
        }
    }
    Ok(())
}

/// Effective vector settings of a knn_vector field definition.
#[derive(Clone, Debug)]
pub struct VecField {
    pub space: String,
}

pub fn vec_field(def: &Value) -> Option<VecField> {
    if def.get("type").and_then(|t| t.as_str()) != Some("knn_vector") {
        return None;
    }
    let data_type = def
        .get("data_type")
        .and_then(|d| d.as_str())
        .unwrap_or("float")
        .to_lowercase();
    let method = def.get("method");
    let space = method
        .and_then(|m| m.get("space_type"))
        .or_else(|| def.get("space_type"))
        .and_then(|s| s.as_str())
        .map(|s| s.to_string())
        .unwrap_or_else(|| if data_type == "binary" { "hamming".into() } else { "l2".into() });
    Some(VecField { space })
}

/// Raw distance/similarity between two vectors for a space.
pub fn raw(space: &str, a: &[f64], b: &[f64]) -> f64 {
    // float arithmetic, like Lucene's VectorUtil
    let pairs = || a.iter().zip(b).map(|(x, y)| (*x as f32, *y as f32));
    (match space {
        "l2" => pairs().map(|(x, y)| (x - y) * (x - y)).sum::<f32>(),
        "l1" => pairs().map(|(x, y)| (x - y).abs()).sum::<f32>(),
        "linf" => pairs().map(|(x, y)| (x - y).abs()).fold(0.0, f32::max),
        "innerproduct" => pairs().map(|(x, y)| x * y).sum::<f32>(),
        "cosinesimil" => {
            let dot: f32 = pairs().map(|(x, y)| x * y).sum();
            let na: f32 = pairs().map(|(x, _)| x * x).sum();
            let nb: f32 = pairs().map(|(_, y)| y * y).sum();
            dot / ((na as f64) * (nb as f64)).sqrt() as f32
        }
        "hamming" => a
            .iter()
            .zip(b)
            .map(|(x, y)| (((*x as i64) ^ (*y as i64)) as u8).count_ones() as f32)
            .sum(),
        _ => 0.0,
    }) as f64
}

/// OpenSearch score for a raw value of a space.
pub fn score(space: &str, r: f64) -> f64 {
    let s = match space {
        "l2" => 1.0 / (1.0 + r),
        "l1" | "linf" | "hamming" => 1.0 / (1.0 + r),
        "cosinesimil" => (1.0 + r) / 2.0,
        "innerproduct" => {
            if r >= 0.0 {
                1.0 + r
            } else {
                1.0 / (1.0 - r)
            }
        }
        _ => 0.0,
    };
    s as f32 as f64
}

/// Mapping definition of a (dotted) field path.
pub fn field_def<'a>(mappings: &'a Value, path: &str) -> Option<&'a Value> {
    let mut cur = mappings;
    for part in path.split('.') {
        cur = cur.get("properties")?.get(part)?;
    }
    Some(cur)
}

/// Vector stored in a document source under a dotted path.
pub fn source_vector(doc: &Value, path: &str) -> Option<Vec<f64>> {
    let mut cur = doc;
    for part in path.split('.') {
        cur = cur.get(part)?;
    }
    cur.as_array()?
        .iter()
        .map(|v| match v {
            Value::Number(n) => n.as_f64(),
            Value::String(s) => s.parse().ok(),
            _ => None,
        })
        .collect()
}

/// Rounds a score to the float precision Lucene scores have, as the shortest
/// decimal of the f32 value (so it renders like Java's Float.toString digits).
pub fn f32r(x: f64) -> f64 {
    if !x.is_finite() {
        return x;
    }
    let f = x as f32;
    format!("{}", f).parse::<f64>().unwrap_or(x)
}
