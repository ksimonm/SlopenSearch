// knn_vector values at index time: validation, doc values and derived source.

use crate::knn::f32r;
use serde_json::{json, Map, Value};

/// Whether an index has `index.knn` enabled.
pub fn index_knn(idx: &crate::store::Index) -> bool {
    idx.settings.get("index.knn").and_then(|v| v.as_str()) == Some("true")
}

/// Why a document vector could not be parsed.
pub struct VecErr {
    pub typ: String,
    pub reason: String,
    pub preview: String,
    /// The cause repeats itself one level deeper (jackson coercion errors).
    pub nested: bool,
}

fn verr(typ: &str, reason: &str, preview: &str) -> VecErr {
    VecErr { typ: typ.into(), reason: reason.into(), preview: preview.into(), nested: false }
}

/// Java toString of a parsed JSON value (lists and maps).
pub fn java_text(v: &Value) -> String {
    match v {
        Value::Array(a) => format!("[{}]", a.iter().map(java_text).collect::<Vec<_>>().join(", ")),
        Value::Object(m) => format!(
            "{{{}}}",
            m.iter().map(|(k, v)| format!("{}={}", k, java_text(v))).collect::<Vec<_>>().join(", ")
        ),
        Value::String(s) => s.clone(),
        Value::Null => "null".into(),
        other => other.to_string(),
    }
}

pub fn data_type_of(def: &Map<String, Value>) -> String {
    def.get("data_type").and_then(|d| d.as_str()).unwrap_or("float").to_lowercase()
}

fn space_of(def: &Map<String, Value>) -> Option<String> {
    def.get("method")
        .and_then(|m| m.get("space_type"))
        .or_else(|| def.get("space_type"))
        .and_then(|s| s.as_str())
        .map(|s| s.to_string())
}

/// Base64 decoding like java.util.Base64's basic decoder: padding is
/// optional and the unused bits of a final partial group are ignored.
fn lenient_base64(s: &str) -> Option<Vec<u8>> {
    let mut out = Vec::new();
    let mut acc: u32 = 0;
    let mut bits = 0;
    for c in s.trim_end_matches('=').bytes() {
        let v = match c {
            b'A'..=b'Z' => c - b'A',
            b'a'..=b'z' => c - b'a' + 26,
            b'0'..=b'9' => c - b'0' + 52,
            b'+' => 62,
            b'/' => 63,
            _ => return None,
        } as u32;
        acc = (acc << 6) | v;
        bits += 6;
        if bits >= 8 {
            bits -= 8;
            out.push((acc >> bits) as u8);
            acc &= (1 << bits) - 1;
        }
    }
    Some(out)
}

/// Decodes a base64 vector of little-endian floats.
pub fn base64_floats(path: &str, s: &str) -> Result<Vec<f64>, VecErr> {
    let bytes = lenient_base64(s).ok_or_else(|| {
        verr("illegal_argument_exception", &format!("Failed to decode Base64 vector for field [{}]", path), s)
    })?;
    if bytes.len() % 4 != 0 {
        return Err(verr(
            "illegal_argument_exception",
            &format!(
                "Base64 encoded vector for field [{}] has invalid byte length [{}], must be a multiple of 4 (float size)",
                path,
                bytes.len()
            ),
            s,
        ));
    }
    Ok(bytes
        .chunks(4)
        .map(|c| f32::from_le_bytes([c[0], c[1], c[2], c[3]]) as f64)
        .collect())
}

/// Parses one element of a vector the way Float.parseFloat would.
fn element_float(v: &Value) -> Result<f64, VecErr> {
    match v {
        Value::Number(n) => Ok(n.as_f64().unwrap_or(0.0)),
        Value::String(s) => s.trim().parse::<f32>().map(|f| f as f64).map_err(|_| {
            verr("number_format_exception", &format!("For input string: \"{}\"", s), s)
        }),
        _ => {
            let tok = match v {
                Value::Null => "VALUE_NULL",
                Value::Bool(true) => "VALUE_TRUE",
                Value::Bool(false) => "VALUE_FALSE",
                Value::Array(_) => "START_ARRAY",
                _ => "START_OBJECT",
            };
            let mut e = verr(
                "input_coercion_exception",
                &format!(
                    "Current token ({}) not numeric, cannot use numeric value accessors\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); byte offset: #0]",
                    tok
                ),
                &java_text(v),
            );
            e.nested = true;
            Err(e)
        }
    }
}

/// Validates a document's vector against its knn_vector mapping and returns
/// the doc values to index (empty for null).
pub fn parse_doc_vector(
    path: &str,
    def: &Map<String, Value>,
    raw: &Value,
    knn_index: bool,
) -> Result<Vec<Value>, VecErr> {
    if raw.is_null() {
        return Ok(Vec::new());
    }
    let dim = def.get("dimension").and_then(|d| d.as_i64()).unwrap_or(0);
    let dt = data_type_of(def);
    let mut out: Vec<f64> = Vec::new();
    match raw {
        Value::String(s) if dt == "float" || dt == "half_float" => {
            out = base64_floats(path, s)?;
        }
        Value::Array(a) => {
            for el in a {
                let f = element_float(el)?;
                let text = java_text(el);
                if dt == "byte" || dt == "binary" {
                    if f.fract() != 0.0 && dt == "byte" {
                        return Err(verr(
                            "illegal_argument_exception",
                            "[data_type] field was set as [byte] in index mapping. But, KNN vector values are floats instead of byte integers",
                            &text,
                        ));
                    }
                    if !(-128.0..=127.0).contains(&f) {
                        return Err(verr(
                            "illegal_argument_exception",
                            &format!(
                                "[data_type] field was set as [{}] in index mapping. But, KNN vector values are not within in the byte range [-128, 127]",
                                dt
                            ),
                            &text,
                        ));
                    }
                    out.push(f.trunc());
                    continue;
                }
                let fv = f as f32;
                if fv.is_nan() {
                    return Err(verr("illegal_argument_exception", "KNN vector values cannot be NaN", "NaN"));
                }
                if fv.is_infinite() {
                    let p = if fv > 0.0 { "Infinity" } else { "-Infinity" };
                    return Err(verr("illegal_argument_exception", "KNN vector values cannot be infinity", p));
                }
                out.push(fv as f64);
            }
        }
        Value::Number(_) => out.push(element_float(raw)?),
        _ => {}
    }
    let preview = match raw {
        Value::Object(_) => java_text(raw),
        Value::String(s) => s.clone(),
        _ => "null".to_string(),
    };
    if dt == "binary" {
        if out.len() as i64 * 8 != dim {
            return Err(verr(
                "illegal_argument_exception",
                &format!(
                    "The dimension of the binary vector must be 8 times the length of the provided vector. Expected: {}, Given: {}",
                    dim,
                    out.len() * 8
                ),
                &preview,
            ));
        }
    } else if out.len() as i64 != dim {
        return Err(verr(
            "illegal_argument_exception",
            &format!("Vector dimension mismatch. Expected: {}, Given: {}", dim, out.len()),
            &preview,
        ));
    }
    if knn_index && space_of(def).as_deref() == Some("cosinesimil") && out.iter().all(|x| *x == 0.0) {
        return Err(verr(
            "illegal_argument_exception",
            "zero vector is not supported when space type is [cosinesimil]",
            &preview,
        ));
    }
    Ok(out
        .into_iter()
        .map(|x| if dt == "byte" || dt == "binary" { json!(x as i64) } else { json!(f32r(x)) })
        .collect())
}

/// Replaces vectors in a source document by their stored (derived) form, as
/// `index.knn.derived_source` does.
pub fn derive_source(mappings: &Value, doc: &mut Map<String, Value>) {
    if let Some(props) = mappings.get("properties").and_then(|p| p.as_object()) {
        derive_obj(props, doc);
    }
}

fn derive_obj(props: &Map<String, Value>, doc: &mut Map<String, Value>) {
    for (k, v) in doc.iter_mut() {
        let def = match props.get(k).and_then(|d| d.as_object()) {
            Some(d) => d,
            None => continue,
        };
        if def.get("type").and_then(|t| t.as_str()) == Some("knn_vector") {
            if v.is_null() {
                continue;
            }
            if let Ok(vals) = parse_doc_vector(k, def, v, false) {
                *v = Value::Array(vals);
            }
            continue;
        }
        if let Some(sub) = def.get("properties").and_then(|p| p.as_object()) {
            match v {
                Value::Object(m) => derive_obj(sub, m),
                Value::Array(a) => {
                    for item in a.iter_mut() {
                        if let Value::Object(m) = item {
                            derive_obj(sub, m);
                        }
                    }
                }
                _ => {}
            }
        }
    }
}
