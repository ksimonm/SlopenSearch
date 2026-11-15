//! The script service shared by search, aggregations, updates and ingest:
//! the stored-script registry, parsed script specs, size and language checks.
//! Engines (painless, expressions) are reached from search::script_ctx.

use crate::errors::ApiError;
use serde_json::{Map, Value};
use std::collections::HashMap;
use std::sync::{OnceLock, RwLock};

fn stored_scripts() -> &'static RwLock<HashMap<String, Map<String, Value>>> {
    static S: OnceLock<RwLock<HashMap<String, Map<String, Value>>>> = OnceLock::new();
    S.get_or_init(|| RwLock::new(HashMap::new()))
}

/// Mirrors the cluster's stored scripts so that code without store access
/// (query evaluation, aggregations) can resolve scripts by id.
pub fn sync_stored(scripts: &HashMap<String, Map<String, Value>>) {
    if let Ok(mut g) = stored_scripts().write() {
        *g = scripts.clone();
    }
}

fn script_not_found(id: &str) -> ApiError {
    ApiError::new(
        "resource_not_found_exception",
        &format!("unable to find script [{}] in cluster state", id),
        404,
    )
}

/// The {"lang", "source", ...} object of a stored script.
pub fn stored_script(id: &str) -> Result<Map<String, Value>, ApiError> {
    let g = stored_scripts().read().map_err(|_| script_not_found(id))?;
    let sc = g.get(id).ok_or_else(|| script_not_found(id))?;
    Ok(sc.get("script").and_then(|s| s.as_object()).cloned().unwrap_or_else(|| sc.clone()))
}

/// A parsed "script" object.
#[derive(Debug, Clone)]
pub struct ScriptSpec {
    pub src: String,
    pub lang: String,
    /// Stored script id, when referenced by id.
    pub id: Option<String>,
    pub params: Map<String, Value>,
}

impl ScriptSpec {
    /// The "script" value reported in script exceptions.
    pub fn err_name(&self) -> String {
        self.id.clone().unwrap_or_else(|| self.src.clone())
    }
}

/// Default of script.max_size_in_bytes.
pub const MAX_INLINE_SIZE: usize = 65535;

/// Rejects inline scripts above script.max_size_in_bytes.
pub fn check_size(src: &str) -> Result<(), ApiError> {
    if src.len() > MAX_INLINE_SIZE {
        return Err(ApiError::bad(&format!(
            "exceeded max allowed inline script size in bytes [{}] with size [{}] for script [{}]",
            MAX_INLINE_SIZE,
            src.len(),
            src
        )));
    }
    Ok(())
}

/// Reads a script given as a string or an object and resolves stored scripts.
pub fn parse_spec(raw: &Value) -> Result<ScriptSpec, ApiError> {
    let spec = parse_spec_inner(raw)?;
    if spec.id.is_none() {
        check_size(&spec.src)?;
    }
    Ok(spec)
}

fn parse_spec_inner(raw: &Value) -> Result<ScriptSpec, ApiError> {
    match raw {
        Value::String(s) => Ok(ScriptSpec { src: s.clone(), lang: "painless".into(), id: None, params: Map::new() }),
        Value::Object(t) => {
            let mut spec = ScriptSpec { src: String::new(), lang: "painless".into(), id: None, params: Map::new() };
            if let Some(Value::Object(p)) = t.get("params") {
                spec.params = p.clone();
            }
            let src = t.get("source").or_else(|| t.get("inline"));
            let id = t.get("id").and_then(|v| v.as_str());
            if src.is_some() && id.is_some() {
                let mut e = ApiError::new("x_content_parse_exception", "[script] failed to parse field [source]", 400);
                e.caused_by = Some(Box::new(ApiError::bad(
                    "must only use one of [source, id] when specifying a script",
                )));
                return Err(e);
            }
            let lang = t.get("lang").and_then(|v| v.as_str());
            if let Some(id) = id {
                if lang.is_some() {
                    return Err(ApiError::bad("illegally specified <lang> for a stored script"));
                }
                spec.id = Some(id.to_string());
                let sc = stored_script(id)?;
                spec.src = sc.get("source").and_then(|v| v.as_str()).unwrap_or("").to_string();
                if let Some(l) = sc.get("lang").and_then(|v| v.as_str()) {
                    spec.lang = l.to_string();
                }
                return Ok(spec);
            }
            if let Some(Value::String(s)) = src {
                spec.src = s.clone();
            }
            if let Some(l) = lang {
                spec.lang = l.to_string();
            }
            Ok(spec)
        }
        _ => Err(ApiError::parsing("unexpected token, expected a script")),
    }
}

/// Rejects languages that cannot run in a context.
pub fn check_lang(spec: &ScriptSpec, ctx_name: &str) -> Result<(), ApiError> {
    match spec.lang.as_str() {
        "painless" => Ok(()),
        "expression" => {
            if expression_supports_context(ctx_name) {
                Ok(())
            } else {
                Err(ApiError::bad(&format!(
                    "expression engine does not know how to handle script context [{}]",
                    ctx_name
                )))
            }
        }
        "mustache" => Err(ApiError::bad(&format!(
            "mustache engine does not know how to handle context [{}]",
            ctx_name
        ))),
        other => Err(ApiError::bad(&format!("script_lang not supported [{}]", other))),
    }
}

/// Contexts that the expression engine supports.
/// The script contexts the Lucene expressions engine handles.
pub fn expression_supports_context(ctx: &str) -> bool {
    matches!(
        ctx,
        "aggregation_selector" | "aggs" | "bucket_aggregation" | "field" | "filter" | "number_sort" | "score"
            | "terms_set"
    )
}

/// The error OpenSearch reports for a mustache template `src`: one that does not
/// compile is a `script_exception` (400; `fields` go first, e.g. the ingest processor),
/// a lookup that fails while rendering a `mustache_exception` (500).
pub fn mustache_error(src: &str, e: &crate::mustache::MustacheError, fields: &[(&str, Value)]) -> ApiError {
    if let Some((inner, typ, msg)) = &e.lookup {
        return ApiError::new("mustache_exception", &e.reason, 500).caused_by(
            ApiError::new("mustache_exception", inner, 500).caused_by(ApiError::new(typ, msg, 500)),
        );
    }
    let mut err = ApiError::new("script_exception", &e.reason, 400);
    for (k, v) in fields {
        err = err.field(k, v.clone());
    }
    err.field("script_stack", serde_json::json!([]))
        .field("script", serde_json::json!(src))
        .field("lang", serde_json::json!("mustache"))
        .caused_by(ApiError::new("mustache_exception", &e.reason, 400))
}
