// The _scripts/painless/_execute API; the engine lives in crate::painless.

use std::rc::Rc;
use serde_json::{json, Map, Value};
use crate::errors::{err_response, json_response, ApiError};
use crate::search::eval::QueryCtx;
use crate::painless::value::*;
use crate::model::Index;
use crate::painless::api::{compile_script, Bindings};
use crate::search::painless_doc::{doc_lookup, score_bindings};
use crate::model::Store;
use crate::http::Req;
use axum::response::Response;

pub fn handle_execute(store: &mut Store, req: &Req, _lang: &str) -> Response {
    if req.method != "POST" && req.method != "GET" {
        return crate::http::method_not_allowed(req, &["POST"]);
    }
    execute(store, req)
}

fn bad(reason: &str) -> Response {
    err_response(ApiError::bad(reason))
}

fn execute(store: &mut Store, req: &Req) -> Response {
    let raw = String::from_utf8_lossy(&req.body).to_string();
    if raw.trim().is_empty() && req.param("source").is_none() {
        return err_response(ApiError::new("parse_exception", "request body or source parameter is required", 400));
    }
    let body = match req.json_body() {
        Ok(m) => m,
        Err(e) => return err_response(e),
    };
    let sm = match body.get("script") {
        None => return bad("Required [script]"),
        Some(Value::Object(m)) => m.clone(),
        Some(_) => {
            let col = raw.find("\"script\"").map(|i| i + "\"script\"".len() + 2).unwrap_or(0);
            let reason = format!("[1:{}] [painless_execute_request] script doesn't support values of type: VALUE_STRING", col);
            let mut e = ApiError::new("x_content_parse_exception", &reason, 400);
            e.root_cause = Some(Box::new(ApiError::new("x_content_parse_exception", &reason, 400)));
            return err_response(e);
        }
    };
    let ctx_name = body.get("context").and_then(|v| v.as_str()).unwrap_or("painless_test").to_string();
    if !matches!(ctx_name.as_str(), "painless_test" | "filter" | "score") {
        let reason = "Failed to build [painless_execute_request] after last required field arrived";
        let mut e = ApiError::new("x_content_parse_exception", reason, 400);
        e.root_cause = Some(Box::new(ApiError::new("x_content_parse_exception", reason, 400)));
        e.caused_by = Some(Box::new(ApiError::new(
            "unsupported_operation_exception",
            &format!("unsupported script context name [{}]", ctx_name),
            400,
        )));
        return err_response(e);
    }
    if sm.contains_key("id") {
        return err_response(ApiError::validation("Validation Failed: 1: only inline scripts are supported;"));
    }
    let src = sm.get("source").and_then(|v| v.as_str()).unwrap_or("").to_string();
    if src.len() > 65535 {
        return bad(&format!(
            "exceeded max allowed inline script size in bytes [65535] with size [{}] for script [{}]",
            src.len(),
            src
        ));
    }
    match sm.get("lang").and_then(|v| v.as_str()).unwrap_or("") {
        "" | "painless" => {}
        "expression" => return bad(&format!("expression engine does not know how to handle script context [{}]", ctx_name)),
        "mustache" => return bad(&format!("mustache engine does not know how to handle context [{}]", ctx_name)),
        other => return bad(&format!("script_lang not supported [{}]", other)),
    }
    let params = match sm.get("params") {
        Some(p @ Value::Object(_)) => from_json(p, MapKind::Hash),
        _ => PV::map(PMap::new(MapKind::Hash)),
    };
    let mut b = Bindings::default();
    b.vars.insert("params".into(), params);
    if ctx_name != "painless_test" {
        let setup = body.get("context_setup").and_then(|v| v.as_object()).cloned().unwrap_or_default();
        let idx_name = setup.get("index").and_then(|v| v.as_str()).unwrap_or("").to_string();
        let idx = match store.indices.get(&idx_name) {
            Some(i) => i,
            None => {
                let mut f = Map::new();
                f.insert("resource.type".into(), json!("index_expression"));
                f.insert("resource.id".into(), json!(idx_name));
                f.insert("index_uuid".into(), json!("_na_"));
                f.insert("index".into(), json!(idx_name));
                return err_response(ApiError::not_found(&idx_name).with_fields(f));
            }
        };
        let doc = setup.get("document").and_then(|v| v.as_object()).cloned().unwrap_or_default();
        let mut tmp = Index::skeleton(&idx.name);
        tmp.uuid = idx.uuid.clone();
        tmp.settings = idx.settings.clone();
        tmp.mappings = idx.mappings.clone();
        tmp.field_types = idx.field_types.clone();
        let id = "_execute";
        let values: Map<String, Value> = match crate::mapping::fields::doc_values_for(&tmp, id, &doc) {
            Ok((vals, _)) => vals.into_iter().map(|(k, v)| (k, Value::Array(v))).collect(),
            Err(e) => return err_response(e),
        };
        tmp.docs.insert(id.into(), Value::Object(doc.clone()));
        tmp.order.push(id.into());
        tmp.seq_nos.insert(id.into(), 0);
        tmp.versions.insert(id.into(), 1);
        tmp.doc_values.insert(id.into(), values.clone());
        if ctx_name == "score" {
            let mut score = 0.0;
            if let Some(Value::Object(q)) = setup.get("query") {
                let docv = Value::Object(doc.clone());
                let qc = QueryCtx::new(&tmp, id, &docv);
                if let Ok((true, sc)) = qc.eval(q) {
                    score = sc as f32 as f64;
                }
            }
            b.vars.insert("_score".into(), PV::Double(score));
            b.funcs = score_bindings(id.to_string());
        }
        let tmp = Rc::new(tmp);
        b.vars.insert("doc".into(), doc_lookup(tmp, id, Some(values)));
    }
    let script = match compile_script(&src, &ctx_name, "") {
        Ok(s) => s,
        Err(e) => return json_response(400, e.error_body()),
    };
    let v = match script.exec(&b, "") {
        Ok(v) => v,
        Err(e) => return json_response(400, e.error_body()),
    };
    let result = match ctx_name.as_str() {
        "painless_test" => Value::String(jstr(&v)),
        "score" => {
            let f = f64_of(&v) as f32 as f64;
            serde_json::Number::from_f64(f).map(Value::Number).unwrap_or(Value::Null)
        }
        _ => to_json(&v),
    };
    json_response(200, json!({ "result": result }))
}
