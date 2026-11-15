//! The HTTP adapter: request parsing, routing, response post-processing
//! (filter_path, pretty, human, compression) and the REST handlers in the
//! submodules. Everything below `crate::http` is reached from here; nothing
//! outside this module depends on it.

pub(crate) mod admin;
pub(crate) mod alias;
pub(crate) mod analyze_api;
pub(crate) mod cat;
pub(crate) mod cluster;
pub(crate) mod compression;
pub(crate) mod datastreams;
pub(crate) mod docs;
pub(crate) mod human;
pub(crate) mod ingest_api;
pub(crate) mod knn_api;
pub(crate) mod maintenance;
pub(crate) mod meta;
pub(crate) mod net_io;
pub(crate) mod nodes;
pub(crate) mod painless_exec;
pub(crate) mod reindex;
pub(crate) mod resize;
pub(crate) mod resolve_index;
pub(crate) mod rollover;
pub(crate) mod script_meta;
pub(crate) mod scripts;
pub(crate) mod search;
pub(crate) mod search_templates;
pub(crate) mod shared;
pub(crate) mod tasks_async;
pub(crate) mod templates;
pub(crate) mod xcontent;

use axum::extract::State;
use axum::http::{header, HeaderMap, StatusCode, Uri};
use axum::response::{IntoResponse, Response};
use axum::routing::any;
use axum::Router;
use serde_json::{json, Map, Value};
use std::sync::{Arc, Mutex};
use crate::errors::{self, ApiError};
use crate::model::Store;
use crate::{resp, storage};

use crate::jsonv::fix_exponent_literals;
use errors::{json_response, no_handler, text_response};
use resp::{filter_path_value, pretty_json};

pub type Shared = Arc<Mutex<Store>>;

/// Common request abstraction shared by all handlers.
pub struct Req {
    pub method: String,
    pub path: String,
    pub query: Vec<(String, String)>,
    pub headers: HeaderMap,
    pub body: Vec<u8>,
}

impl Req {
    pub fn param(&self, k: &str) -> Option<&str> {
        // a repeated parameter: the last value wins
        self.query
            .iter()
            .rev()
            .find(|(qk, _)| qk == k)
            .map(|(_, v)| v.as_str())
    }

    pub fn param_or(&self, k: &str, default: &str) -> String {
        self.param(k).unwrap_or(default).to_string()
    }

    pub fn has_param(&self, k: &str) -> bool {
        self.query.iter().any(|(qk, _)| qk == k)
    }

    /// Mirrors Go's queryBool: presence with empty value counts as true.
    pub fn bool_param(&self, k: &str) -> bool {
        match self.param(k) {
            None => false,
            Some(v) => v == "true" || v.is_empty(),
        }
    }

    pub fn json_body(&self) -> Result<Map<String, Value>, ApiError> {
        let mut s = String::from_utf8_lossy(&self.body).to_string();
        if s.trim().is_empty() {
            if let Some(src) = self.param("source") {
                s = src.to_string();
            } else {
                return Ok(Map::new());
            }
        }
        match serde_json::from_str::<Value>(&s) {
            Ok(Value::Object(mut m)) => {
                let mut v = Value::Object(std::mem::take(&mut m));
                fix_exponent_literals(&s, &mut v);
                match v {
                    Value::Object(m) => Ok(m),
                    _ => Ok(Map::new()),
                }
            }
            Ok(_) => Err(ApiError::new(
                "json_parse_exception",
                "Failed to parse request body",
                400,
            )),
            Err(e) => Err(ApiError::new(
                "json_parse_exception",
                &format!("Failed to parse request body: {}", e),
                400,
            )),
        }
    }
}

fn pct_decode_impl(s: &str, plus_as_space: bool) -> String {
    let bytes = s.as_bytes();
    let mut out: Vec<u8> = Vec::with_capacity(bytes.len());
    let mut i = 0;
    while i < bytes.len() {
        if bytes[i] == b'%' && i + 2 < bytes.len() {
            let hex = std::str::from_utf8(&bytes[i + 1..i + 3]).unwrap_or("");
            if let Ok(b) = u8::from_str_radix(hex, 16) {
                out.push(b);
                i += 3;
                continue;
            }
        }
        if plus_as_space && bytes[i] == b'+' {
            out.push(b' ');
        } else {
            out.push(bytes[i]);
        }
        i += 1;
    }
    String::from_utf8_lossy(&out).to_string()
}

fn pct_decode(s: &str) -> String {
    pct_decode_impl(s, true)
}

/// Path segments keep a literal '+'; only query strings map '+' to space.
pub(crate) fn path_decode(s: &str) -> String {
    pct_decode_impl(s, false)
}

fn parse_query(uri: &Uri) -> Vec<(String, String)> {
    let mut out = vec![];
    if let Some(q) = uri.query() {
        for pair in q.split('&') {
            if pair.is_empty() {
                continue;
            }
            let (k, v) = match pair.split_once('=') {
                Some((k, v)) => (k, v),
                None => (pair, ""),
            };
            // `?=x` names the parameter after its value
            let k = if k.is_empty() { v } else { k };
            out.push((pct_decode(k), pct_decode(v)));
        }
    }
    out
}


async fn handle(State(store): State<Shared>, request: axum::extract::Request) -> axum::response::Response {
    let head = request.method() == axum::http::Method::HEAD;
    let accept = request
        .headers()
        .get("accept-encoding")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("")
        .to_string();
    let resp = handle_plain(store, request).await;
    if head {
        return resp;
    }
    compression::encode_response(&accept, resp).await
}

async fn handle_plain(store: Shared, request: axum::extract::Request) -> axum::response::Response {
    let method = request.method().clone();
    let uri = request.uri().clone();
    let headers = request.headers().clone();
    // http.max_content_length: OpenSearch answers 413 with an empty body, as
    // soon as the announced length is known to be too large
    let announced = headers
        .get("content-length")
        .and_then(|v| v.to_str().ok())
        .and_then(|v| v.trim().parse::<u64>().ok());
    if announced.is_some_and(|n| n > MAX_CONTENT_LENGTH as u64) {
        let mut resp = StatusCode::PAYLOAD_TOO_LARGE.into_response();
        resp.headers_mut().insert("content-length", 0.into());
        resp.headers_mut().insert("connection", "close".parse().unwrap());
        return resp;
    }
    let body = match axum::body::to_bytes(request.into_body(), MAX_CONTENT_LENGTH).await {
        Ok(b) => b,
        Err(_) => return StatusCode::PAYLOAD_TOO_LARGE.into_response(),
    };
    let mut opaque_values = headers.get_all("x-opaque-id").iter();
    let opaque_id = opaque_values.next().cloned();
    if opaque_values.next().is_some() {
        let mut resp = errors::write_simple_error(400, "multiple values for single-valued header [X-Opaque-Id].");
        if let Some(op) = opaque_id {
            resp.headers_mut().insert("x-opaque-id", op);
        }
        return resp;
    }
    let mut req = Req {
        method: method.as_str().to_string(),
        path: uri.path().to_string(),
        query: parse_query(&uri),
        headers,
        body: body.to_vec(),
    };

    // Content-Type gate (mirrors Go ServeHTTP)
    if !req.body.is_empty() {
        let full = req
            .headers
            .get("content-type")
            .and_then(|v| v.to_str().ok())
            .unwrap_or("")
            .to_string();
        let ct = full.split(';').next().unwrap_or("").trim().to_ascii_lowercase();
        if !full.is_empty() && !ct.contains('/') {
            let reason = format!("invalid Content-Type header [{}]", full);
            let mut resp = errors::json_response(
                400,
                serde_json::json!({"error": {
                    "root_cause": [{"type": "content_type_header_exception",
                                    "reason": format!("java.lang.IllegalArgumentException: {}", reason)}],
                    "type": "content_type_header_exception",
                    "reason": format!("java.lang.IllegalArgumentException: {}", reason),
                    "caused_by": {"type": "illegal_argument_exception", "reason": reason}},
                    "status": 400}),
            );
            resp.headers_mut()
                .insert("x-opensearch-version", "OpenSearch/3.9.0 (opensearch)".parse().unwrap());
            return resp;
        }
        let ok = matches!(
            ct.as_str(),
            "application/json"
                | "application/x-ndjson"
                | "application/cbor"
                | "application/yaml"
                | "application/x-yaml"
                | "text/yaml"
                | "application/yml"
        ) || (ct.starts_with("application/vnd.opensearch+") && ct.len() > "application/vnd.opensearch+".len());
        if !ok {
            let mut resp = errors::write_simple_error(
                406,
                &format!("Content-Type header [{}] is not supported", full),
            );
            resp.headers_mut()
                .insert("x-opensearch-version", "OpenSearch/3.9.0 (opensearch)".parse().unwrap());
            return resp;
        }
    }

    match compression::decode_request_body(&req.headers, std::mem::take(&mut req.body), MAX_CONTENT_LENGTH) {
        Ok(b) => req.body = b,
        Err(e) => {
            let mut resp = errors::err_response(e);
            resp.headers_mut()
                .insert("x-opensearch-version", "OpenSearch/3.9.0 (opensearch)".parse().unwrap());
            return resp;
        }
    }

    xcontent::transcode_request(&mut req);

    for flag in ["pretty", "human", "error_trace"] {
        if let Some(v) = req.param(flag) {
            if let Err(e) = errors::strict_bool(v) {
                let mut resp = errors::err_response(e);
                resp.headers_mut()
                    .insert("x-opensearch-version", "OpenSearch/3.9.0 (opensearch)".parse().unwrap());
                return resp;
            }
        }
    }

    // The handlers are synchronous and can be CPU-heavy (searches over many docs) and
    // wait on the store lock: run them on the blocking pool so the async runtime keeps
    // accepting and answering connections.
    let (mut resp, req) = match tokio::task::spawn_blocking(move || {
        if let Some(resp) = tasks_async::route(&store, &req) {
            return (resp, req);
        }
        let resp = {
            let mut guard = store.lock().unwrap_or_else(|e| e.into_inner());
            // Requests that may compile or run scripts get a large stack: the
            // Painless parser, checker and interpreter recurse.
            let scripted = req.body.windows(6).any(|w| w == b"script") || !guard.pipelines.is_empty();
            let resp = if scripted {
                let store: &mut Store = &mut guard;
                crate::painless::api::run_with_stack(|| dispatch(store, &req))
            } else {
                dispatch(&mut guard, &req)
            };
            storage::after_request(&guard, &req.method, &req.path);
            resp
        };
        (resp, req)
    })
    .await
    {
        Ok(r) => r,
        Err(_) => return StatusCode::INTERNAL_SERVER_ERROR.into_response(),
    };

    // Post-processing (mirrors Go ServeHTTP)
    let status = resp.status().as_u16();
    let mut ctype = resp
        .headers()
        .get("content-type")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("")
        .to_string();
    if ctype.is_empty() {
        ctype = "application/json; charset=UTF-8".to_string();
        resp.headers_mut()
            .insert("content-type", ctype.parse().unwrap());
    }
    let ordered = resp.headers_mut().remove("x-lite-ordered").is_some();
    // offset of a raw `_source` written as the last top-level field
    let raw_source_at: Option<usize> = resp
        .headers_mut()
        .remove("x-raw-source-at")
        .and_then(|v| v.to_str().ok().and_then(|s| s.parse().ok()));
    let fp = req.param("filter_path").unwrap_or("").to_string();
    let pretty = req.has_param("pretty") && req.param("pretty") != Some("false");
    let error_trace = req.bool_param("error_trace");
    if ctype.starts_with("application/json") {
        let (mut parts, resp_body) = resp.into_parts();
        let body_bytes = axum::body::to_bytes(resp_body, usize::MAX)
            .await
            .unwrap_or_default();
        let mut body: Vec<u8> = body_bytes.to_vec();
        let mut reencoded = false;
        if !fp.is_empty() && status < 400 {
            if let Ok(v) = serde_json::from_slice::<Value>(&body) {
                body = serde_json::to_vec(&filter_path_value(&v, &fp)).unwrap_or_default();
                reencoded = true;
            }
        }
        if !ordered || reencoded {
            if pretty {
                match raw_source_at.filter(|_| !reencoded) {
                    Some(at) => body = resp::pretty_with_raw_source(&body, at),
                    None => {
                        if let Ok(v) = serde_json::from_slice::<Value>(&body) {
                            body = pretty_json(&v).into_bytes();
                        }
                    }
                }
            } else {
                while body.last() == Some(&b'\n') {
                    body.pop();
                }
            }
        }
        if status >= 400 && error_trace {
            if let Ok(mut v) = serde_json::from_slice::<Value>(&body) {
                if let Some(err_obj) = v.get_mut("error").and_then(|e| e.as_object_mut()) {
                    let stk = Value::String(errors::stack_trace(err_obj));
                    if let Some(rc) = err_obj.get_mut("root_cause").and_then(|r| r.as_array_mut()) {
                        for c in rc.iter_mut() {
                            if let Some(cm) = c.as_object_mut() {
                                let st = Value::String(errors::stack_trace(cm));
                                cm.insert("stack_trace".to_string(), st);
                            }
                        }
                    }
                    err_obj.insert("stack_trace".to_string(), stk);
                }
                body = serde_json::to_vec(&v).unwrap_or_default();
            }
        }
        if status >= 400 {
            body = resp::upper_case_unicode_escapes(&body);
        }
        match xcontent::response_format(&req) {
            "yaml" => {
                if let Ok(v) = serde_json::from_slice::<Value>(&body) {
                    parts.headers.insert("content-type", "application/yaml".parse().unwrap());
                    body = resp::to_yaml(&v).into_bytes();
                }
            }
            "cbor" => {
                if let Ok(v) = serde_json::from_slice::<Value>(&body) {
                    parts.headers.insert("content-type", "application/cbor".parse().unwrap());
                    body = xcontent::to_cbor(&v);
                }
            }
            "smile" => {
                parts.headers.insert("content-type", "application/smile".parse().unwrap());
            }
            _ => {}
        }
        resp = axum::response::Response::from_parts(parts, axum::body::Body::from(body));
    }
    resp.headers_mut().insert(
        "x-opensearch-version",
        "OpenSearch/3.9.0 (opensearch)".parse().unwrap(),
    );
    if let Some(op) = opaque_id {
        resp.headers_mut().insert("x-opaque-id", op);
    }
    if req.method == "HEAD" {
        let (mut parts, b) = resp.into_parts();
        let bytes = axum::body::to_bytes(b, usize::MAX).await.unwrap_or_default();
        if !parts.headers.contains_key("content-length") {
            parts.headers.insert("content-length", bytes.len().into());
        }
        resp = axum::response::Response::from_parts(parts, axum::body::Body::empty());
    }
    resp
}

/// True when the path has a `%` not followed by two hex digits.
fn has_bad_escape(path: &str) -> bool {
    let b = path.as_bytes();
    (0..b.len()).any(|i| {
        b[i] == b'%'
            && !(i + 2 < b.len() && b[i + 1].is_ascii_hexdigit() && b[i + 2].is_ascii_hexdigit())
    })
}

pub fn dispatch(store: &mut Store, req: &Req) -> axum::response::Response {
    if req.method != "OPTIONS" && has_bad_escape(&req.path) {
        // the URI cannot be decoded: RestController reports the methods of the route
        let probe = Req {
            method: "OPTIONS".to_string(),
            path: req.path.clone(),
            query: vec![],
            headers: HeaderMap::new(),
            body: vec![],
        };
        let r = dispatch(store, &probe);
        if let Some(allow) = r.headers().get("allow").and_then(|v| v.to_str().ok()) {
            return json_response(
                405,
                serde_json::json!({
                    "error": format!("Unexpected HTTP method, allowed: [{}]", allow.replace(',', ", ")),
                    "status": 405,
                }),
            );
        }
    }
    let path = req.path.trim_matches('/').to_string();
    let segs: Vec<String> = if path.is_empty() {
        vec![]
    } else {
        path.split('/').map(path_decode).collect()
    };

    if segs.is_empty() || segs[0].is_empty() {
        return cluster::handle_root(store, req);
    }
    if let Some(resp) = meta::route_server_endpoints(store, req, &segs) {
        return resp;
    }
    let index = segs[0].as_str();

    match segs[0].as_str() {
        "_bulk" => return docs::handle_bulk(store, req, ""),
        "_plugins" if segs.len() >= 2 && segs[1] == "_knn" => return knn_api::handle(store, req, &segs[2..]),
        "_mget" => return docs::handle_mget(store, req, ""),
        "_analyze" => return analyze_api::handle_analyze(store, req, ""),
        "_search" => {
            if segs.len() >= 2 && segs[1] == "pipeline" {
                return ingest_api::handle_search_pipeline(store, req, &segs[2..]);
            }
            if segs.len() >= 2 && segs[1] == "scroll" {
                return search::handle_scroll(store, req, &segs[2..]);
            }
            if segs.len() >= 2 && segs[1] == "point_in_time" {
                return search::handle_pit_route(store, req, &segs[2..]);
            }
            return search::handle_search(store, req, "");
        }
        "_msearch" => {
            let template = segs.len() >= 2 && segs[1] == "template";
            return search_templates::handle_msearch(store, req, "", template);
        }
        "_render" => {
            let id = if segs.len() >= 3 { Some(segs[2].as_str()) } else { None };
            return search_templates::handle_render(store, req, id);
        }
        "_count" => return docs::handle_count(store, req, ""),
        "_field_caps" => return search::handle_field_caps(store, req, ""),
        "_settings" => {
            let filter = segs[1..].join("/");
            if req.method != "GET" && req.method != "HEAD" {
                return method_not_allowed(req, &["GET", "HEAD"]);
            }
            return admin::get_settings_api(store, req, "*", &filter);
        }
        "_refresh" | "_flush" | "_forcemerge" => {
            if req.method != "POST" && req.method != "GET" {
                return method_not_allowed(req, &["POST", "GET"]);
            }
            let op = segs[0].trim_start_matches('_');
            return maintenance::maintenance_op(store, req, "*", op);
        }
        "_cache" => {
            if segs.len() == 2 && segs[1] == "clear" {
                return maintenance::maintenance_op(store, req, "*", "cache");
            }
            return no_handler(&req.method, &req.path);
        }
        "_stats" => {
            if req.method != "GET" && req.method != "HEAD" {
                return method_not_allowed(req, &["GET", "HEAD"]);
            }
            let metrics = segs[1..].join(",");
            return maintenance::handle_stats_api(store, req, "*", &metrics);
        }
        "_segments" => return maintenance::handle_segments_api(store, req, "*"),
        "_recovery" => return maintenance::handle_recovery_api(store, req, "*"),
        "_all" if segs.len() == 1 => {
            return match req.method.as_str() {
                "GET" | "HEAD" => admin::get_index_meta(store, req, "_all"),
                "DELETE" => admin::delete_indices(store, req, "_all"),
                _ => method_not_allowed(req, &["GET", "HEAD", "DELETE"]),
            };
        }
        "_mapping" if segs.len() >= 3 && segs[1] == "field" => {
            return admin::get_mapping_api(store, req, "*", &segs[2..].join(","));
        }
        "_alias" => {
            let alias_spec = segs[1..].join(",");
            return match req.method.as_str() {
                "GET" => alias::get_aliases_global(store, req, &alias_spec, false),
                "HEAD" => {
                    if alias_spec.is_empty() {
                        return json_response(200, Value::Null);
                    }
                    alias::head_aliases_global(store, req, &alias_spec)
                }
                _ => method_not_allowed(req, &["GET", "HEAD"]),
            };
        }
        "_aliases" => {
            let alias_spec = segs[1..].join(",");
            return match req.method.as_str() {
                "POST" => alias::post_aliases(store, req),
                "GET" => alias::get_aliases_global(store, req, &alias_spec, true),
                "HEAD" => {
                    if alias_spec.is_empty() {
                        return json_response(200, Value::Null);
                    }
                    alias::head_aliases_global(store, req, &alias_spec)
                }
                _ => method_not_allowed(req, &["GET", "POST", "HEAD"]),
            };
        }
        "_cat" => return cat::handle_cat(store, req, &segs[1..]),
        _ => {}
    }

    if segs[0].starts_with('_') && segs[0] != "_all" {
        if req.method == "PUT" {
            return admin::create_index(store, req, &segs[0]);
        }
        return errors::err_response(ApiError::new(
            "invalid_index_name_exception",
            &format!("Invalid index name [{}], must not start with '_'.", segs[0]),
            400,
        ));
    }

    match segs.len() {
        1 => match req.method.as_str() {
            "PUT" => admin::create_index(store, req, index),
            "DELETE" => admin::delete_indices(store, req, index),
            "GET" | "HEAD" => admin::get_index_meta(store, req, index),
            _ => method_not_allowed(req, &["GET", "PUT", "DELETE", "HEAD"]),
        },
        2 => match segs[1].as_str() {
            "_mapping" | "_mappings" => match req.method.as_str() {
                "GET" | "HEAD" => admin::get_mapping_api(store, req, index, ""),
                "PUT" | "POST" => admin::put_mapping_api(store, req, index),
                _ => method_not_allowed(req, &["GET", "PUT", "POST", "HEAD"]),
            },
            "_settings" => match req.method.as_str() {
                "GET" | "HEAD" => admin::get_settings_api(store, req, index, ""),
                "PUT" => admin::put_settings_api(store, req, index),
                _ => method_not_allowed(req, &["GET", "PUT", "HEAD"]),
            },
            "_analyze" => analyze_api::handle_analyze(store, req, index),
            "_bulk" => docs::handle_bulk(store, req, index),
            "_mget" => docs::handle_mget(store, req, index),
            "_termvectors" => docs::handle_termvectors(store, req, index, None),
            "_mtermvectors" => docs::handle_mtermvectors(store, req, index),
            "_update_by_query" => docs::handle_update_by_query(store, req, index),
            "_delete_by_query" => docs::handle_delete_by_query(store, req, index),
            "_search" => search::handle_search(store, req, index),
            "_search_shards" => search::handle_search_shards(store, req, index),
            "_rank_eval" => search::handle_rank_eval(store, req, index),
            "_msearch" => search_templates::handle_msearch(store, req, index, false),
            "_count" => docs::handle_count(store, req, index),
            "_field_caps" => search::handle_field_caps(store, req, index),
            "_refresh" | "_flush" | "_forcemerge" => {
                if req.method != "POST" && req.method != "GET" {
                    return method_not_allowed(req, &["POST", "GET"]);
                }
                let op = segs[1].trim_start_matches('_');
                maintenance::maintenance_op(store, req, index, op)
            }
            "_cache" => {
                if segs.len() == 2 {
                    return maintenance::maintenance_op(store, req, index, "cache");
                }
                no_handler(&req.method, &req.path)
            }
            "_stats" => {
                if req.method != "GET" && req.method != "HEAD" {
                    return method_not_allowed(req, &["GET", "HEAD"]);
                }
                maintenance::handle_stats_api(store, req, index, "")
            }
            "_segments" => maintenance::handle_segments_api(store, req, index),
            "_recovery" => maintenance::handle_recovery_api(store, req, index),
            "_rollover" => rollover::handle_rollover(store, req, index, ""),
            "_close" => admin::handle_close_open(store, req, index, true),
            "_open" => admin::handle_close_open(store, req, index, false),
            "_alias" | "_aliases" => match req.method.as_str() {
                "GET" | "HEAD" => alias::get_aliases_indexed(store, req, index, ""),
                "DELETE" => alias::delete_alias_api(store, req, index, ""),
                _ => method_not_allowed(req, &["GET", "HEAD", "DELETE"]),
            },
            "_doc" => {
                if req.method == "POST" {
                    let op = req.param("op_type").unwrap_or("index").to_string();
                    docs::handle_index_doc(store, req, index, "", &op)
                } else {
                    method_not_allowed(req, &["POST"])
                }
            }
            _ => no_handler(&req.method, &req.path),
        },
        3 => match segs[1].as_str() {
            "_doc" => match req.method.as_str() {
                "PUT" | "POST" => {
                    let op = req.param("op_type").unwrap_or("index").to_string();
                    docs::handle_index_doc(store, req, index, &segs[2], &op)
                }
                "GET" | "HEAD" => docs::handle_get_doc(store, req, index, &segs[2]),
                "DELETE" => docs::handle_delete_doc(store, req, index, &segs[2]),
                _ => method_not_allowed(req, &["HEAD", "PUT", "GET", "POST", "DELETE"]),
            },
            "_source" => {
                if req.method != "GET" && req.method != "HEAD" {
                    return method_not_allowed(req, &["GET", "HEAD"]);
                }
                docs::handle_get_source(store, req, index, &segs[2])
            }
            "_update" => docs::handle_update_doc(store, req, index, &segs[2]),
            "_termvectors" => docs::handle_termvectors(store, req, index, Some(&segs[2])),
            "_explain" => docs::handle_explain(store, req, index, &segs[2]),
            "_search" if segs[2] == "template" => {
                search_templates::handle_search_template(store, req, index)
            }
            "_search" if segs[2] == "point_in_time" => {
                search::handle_pit_create(store, req, index)
            }
            "_search_shards" => search::handle_search_shards(store, req, index),
            "_validate" if segs[2] == "query" => {
                search::handle_validate_query(store, req, index)
            }
            "_msearch" if segs[2] == "template" => {
                search_templates::handle_msearch(store, req, index, true)
            }
            "_rollover" => rollover::handle_rollover(store, req, index, &segs[2]),
            "_clone" | "_shrink" | "_split" => {
                resize::handle_resize(store, req, index, segs[1].trim_start_matches('_'), &segs[2])
            }
            "_create" => docs::handle_index_doc(store, req, index, &segs[2], "create"),
            "_alias" | "_aliases" => match req.method.as_str() {
                "GET" | "HEAD" => alias::get_aliases_indexed(store, req, index, &segs[2]),
                "PUT" | "POST" => alias::put_alias_api(store, req, index, &segs[2]),
                "DELETE" => alias::delete_alias_api(store, req, index, &segs[2]),
                _ => method_not_allowed(req, &["GET", "PUT", "POST", "DELETE", "HEAD"]),
            },
            "_settings" => {
                if req.method != "GET" && req.method != "HEAD" {
                    return method_not_allowed(req, &["GET", "HEAD"]);
                }
                admin::get_settings_api(store, req, index, &segs[2])
            }
            "_stats" => {
                if req.method != "GET" && req.method != "HEAD" {
                    return method_not_allowed(req, &["GET", "HEAD"]);
                }
                maintenance::handle_stats_api(store, req, index, &segs[2])
            }
            "_cache" => {
                if segs[2] == "clear" {
                    return maintenance::maintenance_op(store, req, index, "cache");
                }
                no_handler(&req.method, &req.path)
            }
            "_block" => {
                if req.method != "PUT" {
                    return method_not_allowed(req, &["PUT"]);
                }
                admin::put_block_api(store, req, index, &segs[2])
            }
            _ => no_handler(&req.method, &req.path),
        },
        _ => {
            if segs.len() == 4 && segs[1] == "_mapping" && segs[2] == "field" {
                return admin::get_mapping_api(store, req, index, &segs[3]);
            }
            no_handler(&req.method, &req.path)
        }
    }
}

/// Mirrors OpenSearch's default http.max_content_length (100mb).
const MAX_CONTENT_LENGTH: usize = 100 << 20;

/// Binds the TCP listener of the HTTP server.
pub(crate) async fn listen(addr: &str) -> std::io::Result<net_io::OsListener> {
    tokio::net::TcpListener::bind(addr).await.map(net_io::OsListener)
}

/// Serves requests on `listener` until the server fails.
pub(crate) async fn serve(listener: net_io::OsListener, store: Shared) -> std::io::Result<()> {
    let app = Router::new().fallback(any(handle)).with_state(store);
    axum::serve(listener, app).await
}

pub fn method_not_allowed(req: &Req, allowed: &[&str]) -> Response {
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
