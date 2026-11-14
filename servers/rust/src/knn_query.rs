// The knn query (request parsing, shard checks, exact k-NN / radial search)
// and the knn_score script (lang "knn").

use crate::errors::ApiError;
use crate::eval::QueryCtx;
use crate::knn::{f32r, field_def, raw, score, source_vector};
use crate::store::{to_f64, Index};
use serde_json::{json, Map, Value};

// ---------------------------------------------------------------- errors

fn raw_error(status: u16, err: Value) -> ApiError {
    let mut e = ApiError::new(
        err.get("type").and_then(|t| t.as_str()).unwrap_or(""),
        err.get("reason").and_then(|t| t.as_str()).unwrap_or(""),
        status,
    );
    e.raw = Some(Box::new(json!({"error": err, "status": status})));
    e
}

/// A query_shard_exception ("failed to create query: ...") raised while the
/// shard builds the query.
pub fn shard_err(idx: &Index, reason: &str, cause_type: &str) -> ApiError {
    let shard = json!({
        "type": "query_shard_exception",
        "reason": format!("failed to create query: {}", reason),
        "index": idx.name,
        "index_uuid": idx.uuid,
        "caused_by": {"type": cause_type, "reason": reason},
    });
    let root = json!({
        "type": "query_shard_exception",
        "reason": format!("failed to create query: {}", reason),
        "index": idx.name,
        "index_uuid": idx.uuid,
    });
    crate::script_ctx::shard_failure(&idx.name, 400, root, shard.clone(), shard, "query")
}

fn iae(reason: &str) -> ApiError {
    ApiError::new("illegal_argument_exception", reason, 400)
}

// ---------------------------------------------------------------- request parsing

/// 1-based line and column of byte offset `pos` in `raw`.
fn line_col(raw: &[u8], pos: usize) -> (usize, usize) {
    let mut line = 1;
    let mut col = 1;
    for &b in &raw[..pos.min(raw.len())] {
        if b == b'\n' {
            line += 1;
            col = 1;
        } else {
            col += 1;
        }
    }
    (line, col)
}

/// Locates `"key"` after the first `"knn"` in the request body.
fn key_pos(raw: &[u8], key: &str) -> usize {
    let text = String::from_utf8_lossy(raw);
    let start = text.find("\"knn\"").unwrap_or(0);
    let needle = format!("\"{}\"", key);
    text[start..].find(&needle).map(|i| i + start).unwrap_or(0)
}

/// Byte offset of the value following `"key":` (skipping whitespace), and
/// optionally the first token inside it when it is an array.
fn value_pos(raw: &[u8], key: &str, into_array: bool) -> usize {
    let mut p = key_pos(raw, key) + key.len() + 2;
    let skip_ws = |p: &mut usize| {
        while *p < raw.len() && (raw[*p] as char).is_whitespace() {
            *p += 1;
        }
    };
    skip_ws(&mut p);
    if p < raw.len() && raw[p] == b':' {
        p += 1;
    }
    skip_ws(&mut p);
    if into_array && p < raw.len() && raw[p] == b'[' {
        p += 1;
        skip_ws(&mut p);
    }
    p
}

fn loc(raw: &[u8], pos: usize) -> String {
    let (l, c) = line_col(raw, pos);
    format!("[{}:{}]", l, c)
}

fn xcontent(raw: &[u8], pos: usize, field: &str, cause: Value) -> ApiError {
    let reason = format!("{} [knn] failed to parse field [{}]", loc(raw, pos), field);
    let rc = json!({"type": "x_content_parse_exception", "reason": reason});
    raw_error(400, json!({"root_cause": [rc], "type": "x_content_parse_exception", "reason": reason, "caused_by": cause}))
}

const KNN_FIELDS: [&str; 10] = [
    "vector", "k", "min_score", "max_distance", "filter", "boost", "_name", "method_parameters",
    "rescore", "expand_nested_docs",
];

/// Validates every knn query of a search request the way the k-NN plugin's
/// parser does, before any shard runs.
pub fn validate_tree(q: &Value, raw: &[u8]) -> Result<(), ApiError> {
    let m = match q.as_object() {
        Some(m) => m,
        None => return Ok(()),
    };
    for (k, v) in m {
        match k.as_str() {
            "knn" => validate_knn(v, raw)?,
            "bool" => {
                for c in ["must", "should", "filter", "must_not"] {
                    match v.get(c) {
                        Some(Value::Array(a)) => {
                            for x in a {
                                validate_tree(x, raw)?;
                            }
                        }
                        Some(x) => validate_tree(x, raw)?,
                        None => {}
                    }
                }
            }
            "dis_max" => {
                if let Some(a) = v.get("queries").and_then(|a| a.as_array()) {
                    for x in a {
                        validate_tree(x, raw)?;
                    }
                }
            }
            "constant_score" => {
                if let Some(f) = v.get("filter") {
                    validate_tree(f, raw)?;
                }
            }
            "boosting" => {
                for c in ["positive", "negative"] {
                    if let Some(f) = v.get(c) {
                        validate_tree(f, raw)?;
                    }
                }
            }
            "function_score" | "script_score" | "nested" | "has_child" | "has_parent" => {
                if let Some(f) = v.get("query") {
                    validate_tree(f, raw)?;
                }
            }
            _ => {}
        }
    }
    Ok(())
}

fn validate_knn(body: &Value, raw: &[u8]) -> Result<(), ApiError> {
    let m = match body.as_object() {
        Some(m) => m,
        None => return Ok(()),
    };
    let mut it = m.iter();
    let (fname, spec) = match it.next() {
        Some(x) => x,
        None => return Err(iae("[knn] field 'vector' requires to be non-null and non-empty")),
    };
    let spec = match spec.as_object() {
        Some(s) => s,
        None => return Ok(()),
    };
    let mut has_vector = false;
    for (k, v) in spec {
        if !KNN_FIELDS.contains(&k.as_str()) {
            let reason = format!("{} [knn] unknown field [{}]", loc(raw, key_pos(raw, k)), k);
            return Err(ApiError::new("x_content_parse_exception", &reason, 400));
        }
        match k.as_str() {
            "vector" => {
                let pos = value_pos(raw, "vector", true);
                match v {
                    Value::Array(a) => {
                        if a.is_empty() {
                            return Err(xcontent(raw, pos, "vector", json!({
                                "type": "illegal_argument_exception",
                                "reason": "[knn] field 'vector' requires to be non-null and non-empty"})));
                        }
                        for x in a {
                            if let Value::String(s) = x {
                                if s.trim().parse::<f32>().is_err() {
                                    return Err(xcontent(raw, pos, "vector", json!({
                                        "type": "number_format_exception",
                                        "reason": format!("For input string: \"{}\"", s)})));
                                }
                            }
                        }
                    }
                    Value::String(s) => {
                        if s.trim().parse::<f32>().is_err() {
                            return Err(xcontent(raw, pos, "vector", json!({
                                "type": "number_format_exception",
                                "reason": format!("For input string: \"{}\"", s)})));
                        }
                    }
                    _ => {}
                }
                has_vector = true;
            }
            "method_parameters" => {
                if let Some(mp) = v.as_object() {
                    for pk in mp.keys() {
                        if pk != "ef_search" && pk != "nprobes" {
                            let pos = value_pos(raw, "method_parameters", false);
                            let end = raw[pos..].iter().position(|b| *b == b'}').map(|i| i + pos).unwrap_or(pos);
                            let (l, c) = line_col(raw, end);
                            let cause = json!({"type": "parsing_exception",
                                "reason": format!("[knn] unknown method parameter found [{}]", pk),
                                "line": l, "col": c});
                            let reason = format!("[{}:{}] [knn] failed to parse field [method_parameters]", l, c);
                            return Err(raw_error(400, json!({"root_cause": [cause.clone()],
                                "type": "x_content_parse_exception", "reason": reason, "caused_by": cause})));
                        }
                    }
                }
            }
            _ => {}
        }
    }
    if let Some((other, _)) = it.next() {
        let (l, c) = line_col(raw, key_pos(raw, other));
        let reason = format!("[knn] query doesn't support multiple fields, found [{}] and [{}]", fname, other);
        let o = json!({"type": "parsing_exception", "reason": reason, "line": l, "col": c});
        return Err(raw_error(400, json!({"root_cause": [o.clone()], "type": "parsing_exception",
            "reason": reason, "line": l, "col": c})));
    }
    if !has_vector {
        return Err(iae("[knn] requires query vector"));
    }
    let set = ["k", "max_distance", "min_score"].iter().filter(|k| spec.contains_key(**k)).count();
    if set != 1 {
        return Err(iae("[knn] requires exactly one of k, distance or score to be set"));
    }
    if let Some(k) = spec.get("k") {
        let k = to_f64(k).unwrap_or(0.0) as i64;
        if k <= 0 || k > 10000 {
            return Err(iae("[knn] requires k to be in the range (0, 10000]"));
        }
    }
    if let Some(ms) = spec.get("min_score") {
        if to_f64(ms).unwrap_or(0.0) <= 0.0 {
            return Err(iae("[knn] requires minScore to be greater than 0"));
        }
    }
    Ok(())
}

// ---------------------------------------------------------------- query execution

fn query_vector(spec: &Map<String, Value>) -> Vec<f64> {
    match spec.get("vector") {
        Some(Value::Array(a)) => a.iter().filter_map(to_f64).collect(),
        Some(v) => to_f64(v).into_iter().collect(),
        None => Vec::new(),
    }
}

struct KnnField {
    space: String,
    data_type: String,
    engine: String,
}

fn knn_field(def: &Value) -> KnnField {
    let vf = crate::knn::vec_field(def).map(|v| v.space).unwrap_or_else(|| "l2".into());
    KnnField {
        space: vf,
        data_type: def.get("data_type").and_then(|d| d.as_str()).unwrap_or("float").to_lowercase(),
        engine: def
            .get("method")
            .and_then(|m| m.get("engine"))
            .and_then(|e| e.as_str())
            .unwrap_or("faiss")
            .to_string(),
    }
}

/// The nested object path that contains `field`, if any.
fn nested_parent(mappings: &Value, field: &str) -> Option<String> {
    let parts: Vec<&str> = field.split('.').collect();
    let mut cur = mappings;
    let mut found = None;
    for (i, part) in parts.iter().enumerate() {
        cur = cur.get("properties")?.get(*part)?;
        if i + 1 < parts.len() && cur.get("type").and_then(|t| t.as_str()) == Some("nested") {
            found = Some(parts[..=i].join("."));
        }
    }
    found
}

fn check_byte_vector(dt: &str, q: &[f64]) -> Result<(), String> {
    if dt != "byte" {
        return Ok(());
    }
    for x in q {
        if x.fract() != 0.0 {
            return Err("[data_type] field was set as [byte] in index mapping. But, KNN vector values are floats instead of byte integers".into());
        }
        if !(-128.0..=127.0).contains(x) {
            return Err("[data_type] field was set as [byte] in index mapping. But, KNN vector values are not within in the byte range [-128, 127]".into());
        }
    }
    Ok(())
}

/// Child objects of `path` in a source document.
fn children(doc: &Value, path: &str) -> Vec<Value> {
    let mut cur = vec![doc.clone()];
    for part in path.split('.') {
        let mut next = Vec::new();
        for c in cur {
            match c.get(part) {
                Some(Value::Array(a)) => next.extend(a.iter().cloned()),
                Some(v @ Value::Object(_)) => next.push(v.clone()),
                _ => {}
            }
        }
        cur = next;
    }
    cur
}

impl<'a> QueryCtx<'a> {
    pub fn eval_knn_query(&self, field: &str, spec: &Map<String, Value>) -> Result<(bool, f64), ApiError> {
        let q = query_vector(spec);
        let def = match field_def(&self.idx.mappings, field) {
            Some(d) if d.get("type").and_then(|t| t.as_str()) == Some("knn_vector") => d,
            _ => {
                return Err(shard_err(
                    self.idx,
                    &format!("Field '{}' is not knn_vector type.", field),
                    "illegal_argument_exception",
                ))
            }
        };
        if !crate::knn_index::index_knn(self.idx) {
            return Err(shard_err(
                self.idx,
                &format!("Field '{}' is not built for ANN search.", field),
                "illegal_argument_exception",
            ));
        }
        let kf = knn_field(def);
        let nested = nested_parent(&self.idx.mappings, field);
        let expand = spec.get("expand_nested_docs").and_then(|v| v.as_bool()) == Some(true);
        if expand && nested.is_none() {
            return Err(shard_err(
                self.idx,
                "Invalid value provided for the [expand_nested_docs] field. [expand_nested_docs] is only supported with a nested field.",
                "illegal_argument_exception",
            ));
        }
        let dim = def.get("dimension").and_then(|d| d.as_i64()).unwrap_or(0);
        let qdim = if kf.data_type == "binary" { q.len() as i64 * 8 } else { q.len() as i64 };
        if qdim != dim {
            return Err(shard_err(
                self.idx,
                &format!("Query vector has invalid dimension: {}. Dimension should be: {}", qdim, dim),
                "illegal_argument_exception",
            ));
        }
        if let Err(r) = check_byte_vector(&kf.data_type, &q) {
            return Err(shard_err(self.idx, &r, "illegal_argument_exception"));
        }
        if kf.space == "cosinesimil" && q.iter().all(|x| *x == 0.0) {
            return Err(shard_err(self.idx, "Cannot normalize a zero-length vector", "illegal_argument_exception"));
        }
        if let Some(md) = spec.get("max_distance").and_then(to_f64) {
            if md < 0.0 && kf.space != "innerproduct" {
                return Err(shard_err(
                    self.idx,
                    &format!("[knn] requires distance to be non-negative for space type: {}", kf.space.to_uppercase()),
                    "illegal_argument_exception",
                ));
            }
        }
        let boost = spec.get("boost").and_then(to_f64).unwrap_or(1.0);
        let filter = spec.get("filter").and_then(|f| f.as_object());
        let scorer = |v: &[f64]| -> Option<(f64, f64)> {
            if v.len() != q.len() {
                return None;
            }
            let r = raw(&kf.space, &q, v);
            if !r.is_finite() {
                return None;
            }
            Some((score(&kf.space, r), r))
        };
        // Per document: the scores of its candidate vectors (children for a
        // nested field).
        let doc_scores = |doc: &Value| -> Vec<(f64, f64)> {
            match &nested {
                Some(np) => {
                    let rel = &field[np.len() + 1..];
                    children(doc, np)
                        .iter()
                        .map(|c| source_vector(c, rel).and_then(|v| scorer(&v)))
                        .map(|x| x.unwrap_or((f64::NAN, f64::NAN)))
                        .collect()
                }
                None => source_vector(doc, field).and_then(|v| scorer(&v)).into_iter().collect(),
            }
        };
        // The vector being evaluated: inside a nested query the current child.
        let (this_score, child_idx) = match &nested {
            Some(np) => {
                if self.scope_path.as_deref() != Some(np.as_str()) {
                    return Ok((false, 0.0));
                }
                let child = match &self.scope_obj {
                    Some(c) => c,
                    None => return Ok((false, 0.0)),
                };
                let rel = &field[np.len() + 1..];
                let s = match source_vector(child, rel).and_then(|v| scorer(&v)) {
                    Some(s) => s,
                    None => return Ok((false, 0.0)),
                };
                let idx = children(self.doc, np).iter().position(|c| c == child);
                (s, idx)
            }
            None => match doc_scores(self.doc).first() {
                Some(s) => (*s, None),
                None => return Ok((false, 0.0)),
            },
        };
        if let Some(f) = filter {
            // the filter applies to the (parent) document
            if !QueryCtx::new(self.idx, &self.id, self.doc).eval(f)?.0 {
                return Ok((false, 0.0));
            }
        }
        let (sc, r) = this_score;
        let best_of = |scores: &[(f64, f64)]| -> Option<(usize, f64)> {
            let mut best: Option<(usize, f64)> = None;
            for (i, (s, _)) in scores.iter().enumerate() {
                if s.is_nan() {
                    continue;
                }
                if best.map(|(_, b)| *s > b).unwrap_or(true) {
                    best = Some((i, *s));
                }
            }
            best
        };
        // Radial search.
        let radial = |s: f64, r: f64| -> Option<bool> {
            if let Some(ms) = spec.get("min_score").and_then(to_f64) {
                return Some(s >= ms);
            }
            if let Some(md) = spec.get("max_distance").and_then(to_f64) {
                let dist = match kf.space.as_str() {
                    "cosinesimil" => 1.0 - r,
                    "innerproduct" => -r,
                    _ => r,
                };
                let inclusive = kf.engine == "lucene" || filter.is_some();
                return Some(if inclusive { dist <= md } else { dist < md });
            }
            None
        };
        if let Some(ok) = radial(sc, r) {
            return Ok((ok, f32r(sc * boost)));
        }
        let k = spec.get("k").and_then(to_f64).unwrap_or(10.0) as usize;
        let mine = doc_scores(self.doc);
        let (best_i, best) = match best_of(&mine) {
            Some(b) => b,
            None => return Ok((false, 0.0)),
        };
        if nested.is_some() && !expand && child_idx != Some(best_i) {
            return Ok((false, 0.0));
        }
        let me = self.idx.order.iter().position(|x| x == &self.id).unwrap_or(0);
        let mut better = 0usize;
        for (pos, id) in self.idx.order.iter().enumerate() {
            if pos == me || self.idx.pending.contains_key(id) {
                continue;
            }
            let doc = match self.idx.docs.get(id) {
                Some(d) => d,
                None => continue,
            };
            let osc = match best_of(&doc_scores(doc)) {
                Some((_, s)) => s,
                None => continue,
            };
            if osc < best || (osc == best && pos > me) {
                continue;
            }
            if let Some(f) = filter {
                if !QueryCtx::new(self.idx, id, doc).eval(f)?.0 {
                    continue;
                }
            }
            better += 1;
            if better >= k {
                return Ok((false, 0.0));
            }
        }
        Ok((better < k, f32r(sc * boost)))
    }
}

// ---------------------------------------------------------------- knn_score script

fn knn_space_score(space: &str, q: &[f64], v: &[f64]) -> f64 {
    let r = raw(space, q, v);
    let s = match space {
        "l2" => 1.0 / (1.0 + r),
        "l1" | "linf" | "hamming" => 1.0 / (1.0 + r),
        "cosinesimil" => {
            if r.is_nan() {
                0.5
            } else {
                (1.0 + r) / 2.0
            }
        }
        "innerproduct" => {
            if r >= 0.0 {
                1.0 + r
            } else {
                1.0 / (1.0 - r)
            }
        }
        _ => 0.0,
    };
    f32r(s)
}

/// Runs a `{"lang": "knn", "source": "knn_score"}` script for the current doc.
pub fn knn_score_script(qc: &QueryCtx, script: &Value) -> Result<f64, ApiError> {
    let idx = qc.idx;
    let src = script.get("source").or_else(|| script.get("inline")).and_then(|s| s.as_str()).unwrap_or("");
    if src != "knn_score" {
        let cause = json!({"type": "illegal_argument_exception", "reason": format!("Unknown script name {}", src)});
        let shard = json!({
            "type": "query_shard_exception",
            "reason": format!("failed to create query: Failed to compile inline script [{}] using lang [knn]", src),
            "index": idx.name,
            "index_uuid": idx.uuid,
            "caused_by": cause,
        });
        return Err(crate::script_ctx::shard_failure(&idx.name, 400, cause.clone(), shard, cause, "query"));
    }
    let params = script.get("params").and_then(|p| p.as_object()).cloned().unwrap_or_default();
    for p in ["field", "query_value", "space_type"] {
        if !params.contains_key(p) {
            return Err(shard_err(idx, &format!("Missing parameter [{}]", p), "illegal_argument_exception"));
        }
    }
    let field = params["field"].as_str().unwrap_or("").to_string();
    let space = params["space_type"].as_str().unwrap_or("").to_string();
    if !["l2", "l1", "linf", "cosinesimil", "innerproduct", "hamming", "hammingbit"].contains(&space.as_str()) {
        return Err(shard_err(idx, "Invalid space type. Please refer to the available space types.", "illegal_argument_exception"));
    }
    let def = match field_def(&idx.mappings, &field) {
        Some(d) if d.get("type").and_then(|t| t.as_str()) == Some("knn_vector") => d,
        _ => {
            return Err(shard_err(
                idx,
                &format!("Incompatible field_type for {} space. The field type must be knn_vector.", space),
                "illegal_argument_exception",
            ))
        }
    };
    let dt = def.get("data_type").and_then(|d| d.as_str()).unwrap_or("float").to_uppercase();
    if space == "hamming" && dt != "BINARY" {
        return Err(shard_err(
            idx,
            &format!("Incompatible field_type for hamming space. The data type should be [BINARY] but got {}", dt),
            "illegal_argument_exception",
        ));
    }
    if space != "hamming" && dt == "BINARY" {
        return Err(shard_err(
            idx,
            &format!("Incompatible field_type for {} space. The data type should be [HALF_FLOAT, BYTE, FLOAT] but got BINARY", space),
            "illegal_argument_exception",
        ));
    }
    let q: Vec<f64> = match &params["query_value"] {
        Value::Array(a) => a.iter().filter_map(to_f64).collect(),
        Value::String(s) => crate::knn_index::base64_floats(&field, s).unwrap_or_default(),
        _ => Vec::new(),
    };
    let dim = def.get("dimension").and_then(|d| d.as_i64()).unwrap_or(0);
    let expected = if dt == "BINARY" { dim / 8 } else { dim };
    if q.len() as i64 != expected {
        return Err(shard_err(
            idx,
            &format!("Object's length={} does not match the expected length={}.", q.len(), expected),
            "illegal_state_exception",
        ));
    }
    match source_vector(qc.doc, &field) {
        Some(v) if v.len() == q.len() => Ok(knn_space_score(&space, &q, &v)),
        _ => Ok(0.0),
    }
}
