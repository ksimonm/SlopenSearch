//! Numbers in responses are written the way Jackson writes the Java type
//! OpenSearch holds them in: doubles with Double.toString (`1.0E7`), floats
//! with Float.toString. These helpers render field values, sort values and
//! scores accordingly.

use serde_json::Value;

use crate::javafmt::{double_value, float_value};
use crate::jsonv::to_f64;
use crate::model::Index;

use super::fetch::flatten_any;

fn finite(v: &Value) -> Option<f64> {
    match v {
        Value::Number(_) => to_f64(v).filter(|f| f.is_finite()),
        _ => None,
    }
}

/// A value of a field of type `ftype` as the Java type its doc values hold:
/// float and half_float values are floats, double and scaled_float values
/// doubles; `as_double` widens floats (docvalue_fields and numeric_type
/// double sorts read them as doubles). Other values are returned as they are.
pub fn java_field_number(ftype: &str, v: &Value, as_double: bool) -> Value {
    let Some(f) = finite(v) else { return v.clone() };
    match ftype {
        "float" | "half_float" if !as_double => float_value(f as f32),
        "float" | "half_float" | "double" | "scaled_float" => double_value(f),
        _ => v.clone(),
    }
}

/// The values of a float, half_float, double or scaled_float field for
/// docvalue_fields (`doc_values`: read as doubles) or for the fields API
/// (`from_source`: the _source values parsed by the field type, so a
/// half_float or scaled_float keeps the precision it was sent with).
pub fn java_field_values(idx: &Index, id: &str, field: &str, ftype: &str, doc_values: Vec<Value>, from_source: bool) -> Vec<Value> {
    if !matches!(ftype, "float" | "half_float" | "double" | "scaled_float") {
        return doc_values;
    }
    if from_source {
        let src = idx
            .docs
            .get(id)
            .and_then(|d| crate::jsonv::field_value(d, field))
            .map(flatten_any)
            .unwrap_or_default();
        let src: Vec<&Value> = src.iter().filter(|v| !v.is_null()).collect();
        let parsed: Option<Vec<Value>> = src
            .iter()
            .map(|v| {
                to_f64(v).filter(|f| f.is_finite()).map(|f| match ftype {
                    "float" | "half_float" => float_value(f as f32),
                    _ => double_value(f),
                })
            })
            .collect();
        if let Some(p) = parsed.filter(|p| p.len() == doc_values.len()) {
            return p;
        }
    }
    doc_values.iter().map(|v| java_field_number(ftype, v, true)).collect()
}

/// A score: a Java float.
pub fn score_value(s: f64) -> Value {
    let f = s as f32;
    if f.is_finite() || !s.is_finite() {
        float_value(f)
    } else {
        double_value(s)
    }
}
