use axum::http::{header, StatusCode};
use axum::response::{IntoResponse, Response};
use serde_json::{json, Map, Value};

use crate::resp::pretty_json;

#[derive(Debug, Clone)]
pub struct ApiError {
    pub typ: String,
    pub reason: String,
    pub status: u16,
    pub extra: Option<Value>,
    pub fields: Map<String, Value>,
    pub caused_by: Option<Box<ApiError>>,
    pub top_type: Option<String>,
    pub top_reason: Option<String>,
    pub root_cause: Option<Box<ApiError>>,
    /// A complete pre-rendered response body (e.g. shard failure envelopes).
    pub raw: Option<Box<Value>>,
    /// Fields of the top-level error only (not copied to root_cause).
    pub top_fields: Map<String, Value>,
}

impl ApiError {
    pub fn new(typ: &str, reason: &str, status: u16) -> Self {
        ApiError {
            typ: typ.to_string(),
            reason: reason.to_string(),
            status,
            extra: None,
            fields: Map::new(),
            caused_by: None,
            top_type: None,
            top_reason: None,
            root_cause: None,
            raw: None,
            top_fields: Map::new(),
        }
    }

    pub fn with_fields(mut self, fields: Map<String, Value>) -> Self {
        self.fields = fields;
        self
    }

    pub fn field(mut self, k: &str, v: Value) -> Self {
        self.fields.insert(k.to_string(), v);
        self
    }

    pub fn caused_by(mut self, e: ApiError) -> Self {
        self.caused_by = Some(Box::new(e));
        self
    }

    pub fn not_found(index: &str) -> Self {
        ApiError::new(
            "index_not_found_exception",
            &format!("no such index [{}]", index),
            404,
        )
    }

    pub fn bad(reason: &str) -> Self {
        ApiError::new("illegal_argument_exception", reason, 400)
    }

    pub fn parsing(reason: &str) -> Self {
        ApiError::new("parsing_exception", reason, 400)
    }

    pub fn validation(reason: &str) -> Self {
        ApiError::new("action_request_validation_exception", reason, 400)
    }

    pub fn to_value(&self) -> Value {
        if let Some(raw) = &self.raw {
            return (**raw).clone();
        }
        let (rc_type, rc_reason) = match &self.root_cause {
            Some(rc) => (rc.typ.clone(), rc.reason.clone()),
            None => (self.typ.clone(), self.reason.clone()),
        };
        let mut root_cause = json!({"type": rc_type, "reason": rc_reason});
        let mut err = json!({
            "root_cause": [root_cause.clone()],
            "type": self.typ.clone(),
            "reason": self.reason.clone(),
        });
        let typ = self.top_type.clone().unwrap_or_else(|| self.typ.clone());
        let reason = self
            .top_reason
            .clone()
            .unwrap_or_else(|| self.reason.clone());
        err["type"] = json!(typ);
        err["reason"] = json!(reason);

        let mut fields = self.fields.clone();
        if self.typ == "index_not_found_exception" && !fields.contains_key("index") {
            let mut name = self.reason.clone();
            if let Some(i) = name.find('[') {
                let tail = &name[i..];
                if let Some(end) = tail.find(']') {
                    name = tail[1..end].to_string();
                } else {
                    name = tail[1..].to_string();
                }
            }
            fields.insert("resource.type".to_string(), json!("index_or_alias"));
            fields.insert("resource.id".to_string(), json!(name));
            fields.insert("index_uuid".to_string(), json!("_na_"));
            fields.insert("index".to_string(), json!(name));
        }
        if self.typ == "shard_not_found_exception" {
            if let Some(extra) = &self.extra {
                fields.insert("shard".to_string(), extra.clone());
            }
        }
        if let Some(obj) = err.as_object_mut() {
            if let Some(rc_obj) = root_cause.as_object_mut() {
                for (k, v) in &fields {
                    obj.insert(k.clone(), v.clone());
                    rc_obj.insert(k.clone(), v.clone());
                }
            }
            if let Some(rc_obj) = root_cause.as_object() {
                obj.insert("root_cause".to_string(), json!([rc_obj.clone()]));
            }
            if let Some(cb) = &self.caused_by {
                obj.insert("caused_by".to_string(), nested_error_value(cb));
            }
            for (k, v) in &self.top_fields {
                obj.insert(k.clone(), v.clone());
            }
        }
        json!({"error": err, "status": self.status})
    }
}

pub(crate) fn nested_error_value(e: &ApiError) -> Value {
    let mut v = json!({"type": e.typ, "reason": e.reason});
    if let Some(o) = v.as_object_mut() {
        for (k, val) in &e.fields {
            o.insert(k.clone(), val.clone());
        }
        if let Some(cb) = &e.caused_by {
            o.insert("caused_by".to_string(), nested_error_value(cb));
        }
    }
    v
}

pub fn json_value_response(status: u16, v: &Value) -> Response {
    let body = serde_json::to_string(v).unwrap_or_else(|_| "{}".to_string());
    (
        StatusCode::from_u16(status).unwrap_or(StatusCode::INTERNAL_SERVER_ERROR),
        [(header::CONTENT_TYPE, "application/json; charset=UTF-8")],
        body,
    )
        .into_response()
}

pub fn json_response(status: u16, v: Value) -> Response {
    json_value_response(status, &v)
}

pub fn pretty_response(status: u16, v: &Value) -> Response {
    let body = pretty_json(v);
    (
        StatusCode::from_u16(status).unwrap_or(StatusCode::INTERNAL_SERVER_ERROR),
        [(header::CONTENT_TYPE, "application/json; charset=UTF-8")],
        body,
    )
        .into_response()
}

pub fn text_response(status: u16, content_type: &str, body: String) -> Response {
    (
        StatusCode::from_u16(status).unwrap_or(StatusCode::INTERNAL_SERVER_ERROR),
        [(header::CONTENT_TYPE, content_type.to_string())],
        body,
    )
        .into_response()
}

pub fn err_response(e: ApiError) -> Response {
    let status = e.status;
    json_response(status, e.to_value())
}

pub fn write_simple_error(status: u16, msg: &str) -> Response {
    json_response(status, json!({"error": msg, "status": status}))
}

pub fn method_not_allowed(req: &crate::Req, allowed: &[&str]) -> Response {
    let allow = if allowed.is_empty() {
        "GET,PUT,POST,DELETE,HEAD,OPTIONS".to_string()
    } else {
        allowed.join(",")
    };
    if req.method == "OPTIONS" {
        // RestController answers OPTIONS with the allowed methods and no body
        let mut resp = text_response(200, "text/plain; charset=UTF-8", String::new());
        resp.headers_mut().insert(header::ALLOW, allow.parse().unwrap());
        return resp;
    }
    let v = json!({
        "error": format!(
            "Incorrect HTTP method for uri [{}] and method [{}], allowed: [{}]",
            req.path, req.method, allow.replace(',', ", ")
        ),
        "status": 405,
    });
    let mut resp = json_response(405, v);
    resp.headers_mut().insert(
        header::ALLOW,
        allow.parse().unwrap(),
    );
    resp
}

pub fn no_handler(method: &str, path: &str) -> Response {
    json_response(
        400,
        json!({
            "error": format!("no handler found for uri [{}] and method [{}]", path, method),
        }),
    )
}

/// Lucene's LevenshteinDistance similarity: 1 - distance / max(len).
fn levenshtein_similarity(a: &str, b: &str) -> f64 {
    let a: Vec<char> = a.chars().collect();
    let b: Vec<char> = b.chars().collect();
    if a.is_empty() || b.is_empty() {
        return if a == b { 1.0 } else { 0.0 };
    }
    let mut prev: Vec<usize> = (0..=b.len()).collect();
    for i in 1..=a.len() {
        let mut cur = vec![i; b.len() + 1];
        for j in 1..=b.len() {
            let cost = if a[i - 1] == b[j - 1] { 0 } else { 1 };
            cur[j] = (prev[j] + 1).min(cur[j - 1] + 1).min(prev[j - 1] + cost);
        }
        prev = cur;
    }
    1.0 - prev[b.len()] as f64 / a.len().max(b.len()) as f64
}

/// BaseRestHandler.unrecognized: "request [p] contains unrecognized parameter(s): [a] -> did you mean [b]?, ..."
pub fn unrecognized_message(path: &str, invalid: &[&str], candidates: &[&str], kind: &str) -> String {
    let mut msg = format!(
        "request [{}] contains unrecognized {}{}: ",
        path,
        kind,
        if invalid.len() > 1 { "s" } else { "" }
    );
    let mut first = true;
    for inv in invalid {
        if !first {
            msg.push_str(", ");
        }
        first = false;
        msg.push_str(&format!("[{}]", inv));
        let mut scored: Vec<(f64, &str)> = candidates
            .iter()
            .map(|c| (levenshtein_similarity(inv, c), *c))
            .filter(|(d, _)| *d > 0.5)
            .collect();
        scored.sort_by(|x, y| y.0.partial_cmp(&x.0).unwrap_or(std::cmp::Ordering::Equal).then(x.1.cmp(y.1)));
        if !scored.is_empty() {
            msg.push_str(" -> did you mean ");
            if scored.len() == 1 {
                msg.push_str(&format!("[{}]?", scored[0].1));
            } else {
                let names: Vec<&str> = scored.iter().map(|x| x.1).collect();
                msg.push_str(&format!("any of [{}]?", names.join(", ")));
            }
        }
    }
    msg
}

/// Booleans.parseBoolean: only "true", "false" and the empty value are accepted.
pub fn strict_bool(v: &str) -> Result<bool, ApiError> {
    match v {
        "" | "true" => Ok(true),
        "false" => Ok(false),
        _ => Err(ApiError::new(
            "illegal_argument_exception",
            &format!("Failed to parse value [{}] as only [true] or [false] are allowed.", v),
            400,
        )),
    }
}

/// The `stack_trace` written with error_trace=true: Throwable.toString of the
/// exception followed by a frame.
pub fn stack_trace(err: &Map<String, Value>) -> String {
    let typ = err.get("type").and_then(|v| v.as_str()).unwrap_or("exception");
    let reason = err.get("reason").and_then(|v| v.as_str()).unwrap_or("");
    let class: String = typ
        .split('_')
        .map(|w| {
            let mut c = w.chars();
            match c.next() {
                Some(f) => f.to_uppercase().collect::<String>() + c.as_str(),
                None => String::new(),
            }
        })
        .collect();
    let head = match typ {
        "illegal_argument_exception" | "illegal_state_exception" | "number_format_exception"
        | "null_pointer_exception" | "array_index_out_of_bounds_exception"
        | "unsupported_operation_exception" | "class_cast_exception" => {
            format!("java.lang.{}: {}", class, reason)
        }
        _ => {
            let mut prefix = String::new();
            if let Some(ix) = err.get("index").and_then(|v| v.as_str()) {
                prefix.push_str(&format!("[{}]", ix));
                if let Some(sh) = err.get("shard") {
                    let sh = sh.as_str().map(|s| s.to_string()).unwrap_or_else(|| sh.to_string());
                    prefix.push_str(&format!("[{}]", sh));
                }
                prefix.push(' ');
            }
            format!("{}{}[{}]", prefix, class, reason)
        }
    };
    format!(
        "{}\n\tat org.opensearch.rest.RestController.dispatchRequest(RestController.java:367)",
        head
    )
}

/// Jackson's error for a body that is not valid UTF-8, wrapped the way a
/// document parse failure is reported.
pub fn invalid_utf8_error(body: &[u8], e: &std::str::Utf8Error) -> ApiError {
    let at = e.valid_up_to();
    let lead = body[at];
    let (what, byte, offset) = if (0xC2..=0xF4).contains(&lead) {
        let k = at + e.error_len().unwrap_or(1).max(1);
        match body.get(k) {
            Some(b) => ("middle", *b, k + 1),
            None => ("start", lead, at + 1),
        }
    } else {
        ("start", lead, at + 1)
    };
    let reason = format!(
        "Invalid UTF-8 {} byte 0x{:x}\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); byte offset: #{}]",
        what, byte, offset
    );
    ApiError::new("mapper_parsing_exception", "failed to parse", 400).caused_by(
        ApiError::new("json_parse_exception", &reason, 400)
            .caused_by(ApiError::new("stream_read_exception", &reason, 400)),
    )
}
