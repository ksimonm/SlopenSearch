//! The `_ingest` (pipelines, `_simulate`, grok processor listing) and
//! `_search/pipeline` REST routes.

use crate::errors::{err_response, json_response, ApiError};
use crate::http::Req;
use crate::ingest::{build_inline_pipeline, grok_patterns_json, simulate, validate_search_pipeline, PROCESSOR_TYPES};
use crate::model::Store;
use axum::response::Response;
use regex::Regex;
use serde_json::{json, Map, Value};

pub fn handle_ingest(store: &mut Store, req: &Req, segs: &[String]) -> Response {
    if segs.is_empty() {
        return json_response(404, json!({}));
    }
    if segs[0] == "processor" {
        if segs.len() == 2 && req.method == "GET" {
            if segs[1] == "grok" {
                return json_response(200, grok_patterns_json());
            }
            if PROCESSOR_TYPES.contains(&segs[1].as_str()) || segs[1] == "grok" {
                return json_response(200, json!({"type": segs[1]}));
            }
            return json_response(404, json!({}));
        }
        return json_response(404, json!({}));
    }
    if segs[0] != "pipeline" {
        return json_response(404, json!({}));
    }
    match req.method.as_str() {
        "GET" if segs.len() == 1 => {
            let out: Map<String, Value> = store
                .pipelines
                .iter()
                .map(|(k, v)| (k.clone(), Value::Object(v.clone())))
                .collect();
            json_response(200, Value::Object(out))
        }
        "GET" if segs.len() == 2 => {
            let name = &segs[1];
            if name.contains(',') || name.contains('*') || name.contains('?') {
                let out: Map<String, Value> = store
                    .pipelines
                    .iter()
                    .filter(|(k, _)| name.split(',').any(|part| glob_match(part, k)))
                    .map(|(k, v)| (k.clone(), Value::Object(v.clone())))
                    .collect();
                return json_response(200, Value::Object(out));
            }
            match store.pipelines.get(name) {
                Some(p) => json_response(200, json!({name: Value::Object(p.clone())})),
                None => json_response(404, json!({})),
            }
        }
        "PUT" | "POST" if segs.len() == 2 && segs[1] != "_simulate" => {
            let body = match req.json_body() {
                Ok(b) => b,
                Err(e) => return err_response(e),
            };
            let name = &segs[1];
            match build_inline_pipeline(&body, name) {
                Ok(p) => {
                    store.pipelines.insert(name.clone(), p);
                    json_response(200, json!({"acknowledged": true}))
                }
                Err(e) => err_response(e),
            }
        }
        "DELETE" if segs.len() == 2 => {
            let name = &segs[1];
            if store.pipelines.remove(name).is_some() {
                json_response(200, json!({"acknowledged": true}))
            } else {
                err_response(ApiError::new(
                    "resource_not_found_exception",
                    &format!("pipeline [{}] is missing", name),
                    404,
                ))
            }
        }
        "POST" if segs.len() == 2 && segs[1] == "_simulate" => {
            let body = match req.json_body() {
                Ok(b) => b,
                Err(e) => return err_response(crate::mapping::nonstd_json::find(&req.body).map_or(e, |t| t.ingest_error())),
            };
            simulate(store, req.bool_param("verbose"), body, None)
        }
        "POST" if segs.len() == 3 && segs[2] == "_simulate" => {
            let body = match req.json_body() {
                Ok(b) => b,
                Err(e) => return err_response(crate::mapping::nonstd_json::find(&req.body).map_or(e, |t| t.ingest_error())),
            };
            let name = segs[1].clone();
            if !store.pipelines.contains_key(&name) {
                return err_response(ApiError::bad(&format!(
                    "pipeline [{}] does not exist",
                    name
                )));
            }
            simulate(store, req.bool_param("verbose"), body, Some(name))
        }
        _ => json_response(404, json!({})),
    }
}

fn glob_match(pattern: &str, name: &str) -> bool {
    if pattern == "*" || pattern == "_all" {
        return true;
    }
    let mut p = String::new();
    for c in pattern.chars() {
        match c {
            '*' => p.push_str(".*"),
            '?' => p.push('.'),
            _ => {
                if ".+()|[]{}^$\\".contains(c) {
                    p.push('\\');
                }
                p.push(c)
            }
        }
    }
    Regex::new(&format!("^{}$", p))
        .map(|r| r.is_match(name))
        .unwrap_or(false)
}

pub fn handle_search_pipeline(store: &mut Store, req: &Req, segs: &[String]) -> Response {
    match req.method.as_str() {
        "GET" if segs.is_empty() => {
            let out: Map<String, Value> = store
                .search_pipelines
                .iter()
                .map(|(k, v)| (k.clone(), Value::Object(v.clone())))
                .collect();
            json_response(200, Value::Object(out))
        }
        "GET" if segs.len() == 1 => {
            let name = &segs[0];
            if name.contains(',') || name.contains('*') || name.contains('?') {
                let out: Map<String, Value> = store
                    .search_pipelines
                    .iter()
                    .filter(|(k, _)| name.split(',').any(|part| glob_match(part, k)))
                    .map(|(k, v)| (k.clone(), Value::Object(v.clone())))
                    .collect();
                return json_response(200, Value::Object(out));
            }
            match store.search_pipelines.get(name) {
                Some(p) => json_response(200, json!({name: Value::Object(p.clone())})),
                None => json_response(404, json!({})),
            }
        }
        "PUT" | "POST" if segs.len() == 1 => {
            let body = match req.json_body() {
                Ok(b) => b,
                Err(e) => return err_response(e),
            };
            if let Err(e) = validate_search_pipeline(&body) {
                return err_response(e);
            }
            store.search_pipelines.insert(segs[0].clone(), body);
            json_response(200, json!({"acknowledged": true}))
        }
        "DELETE" if segs.len() == 1 => {
            let name = &segs[0];
            if store.search_pipelines.remove(name).is_some() {
                json_response(200, json!({"acknowledged": true}))
            } else {
                err_response(ApiError::new(
                    "resource_not_found_exception",
                    &format!("pipeline [{}] is missing", name),
                    404,
                ))
            }
        }
        _ => json_response(404, json!({})),
    }
}
