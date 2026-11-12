// Glue between the scripting engines and the APIs that run scripts (search,
// aggregations, update, ingest): script specs, stored-script resolution,
// language checks, the shard-failure envelopes and one entry point that runs
// a prepared script with its bindings. Port of servers/go/script_ctx.go.
//
// The Painless engine is reached only through `painless_backend`; everything
// else here is engine independent.

use crate::errors::ApiError;
use crate::eval::QueryCtx;
use crate::expressions;
use crate::store::{Index, NODE_ID};
use serde_json::{json, Map, Value};
use std::collections::HashMap;
use std::sync::{OnceLock, RwLock};

// ---- stored scripts ------------------------------------------------------

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

// ---- script specs --------------------------------------------------------

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

/// Reads a script given as a string or an object and resolves stored scripts.
pub fn parse_spec(raw: &Value) -> Result<ScriptSpec, ApiError> {
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
            if expressions::supports_context(ctx_name) {
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

// ---- script errors -------------------------------------------------------

/// A script_exception raised while compiling, linking or running a script.
#[derive(Debug, Clone)]
pub struct ScriptErr {
    pub compile: bool,
    pub link: bool,
    pub lang: String,
    pub script: String,
    pub stack: Vec<String>,
    /// offset, start, end
    pub pos: Option<(i64, i64, i64)>,
    pub cause_type: String,
    /// None renders as JSON null.
    pub reason: Option<String>,
    /// Nested cause of the cause: (type, reason).
    pub inner: Option<(String, Option<String>)>,
    /// Extra keys after "reason" (e.g. Debug.explain's painless_class).
    pub extra: Vec<(String, Value)>,
}

impl ScriptErr {
    pub fn new(compile: bool, cause_type: &str, reason: &str) -> Self {
        ScriptErr {
            compile,
            link: false,
            lang: "painless".into(),
            script: String::new(),
            stack: Vec::new(),
            pos: None,
            cause_type: cause_type.to_string(),
            reason: Some(reason.to_string()),
            inner: None,
            extra: Vec::new(),
        }
    }

    pub fn kind(&self) -> &'static str {
        if self.link {
            "link error"
        } else if self.compile {
            "compile error"
        } else {
            "runtime error"
        }
    }

    /// The script_exception object, optionally with its caused_by.
    pub fn object(&self, with_cause: bool) -> Value {
        let mut o = Map::new();
        o.insert("type".into(), json!("script_exception"));
        o.insert("reason".into(), json!(self.kind()));
        for (k, v) in &self.extra {
            o.insert(k.clone(), v.clone());
        }
        o.insert("script_stack".into(), json!(self.stack));
        o.insert("script".into(), json!(self.script));
        o.insert("lang".into(), json!(if self.lang.is_empty() { "painless" } else { &self.lang }));
        if let Some((off, s, e)) = self.pos {
            o.insert("position".into(), json!({"offset": off, "start": s, "end": e}));
        }
        if with_cause {
            o.insert("caused_by".into(), self.cause_object());
        }
        Value::Object(o)
    }

    pub fn cause_object(&self) -> Value {
        let mut c = Map::new();
        c.insert("type".into(), json!(self.cause_type));
        c.insert("reason".into(), json!(self.reason));
        if let Some((t, r)) = &self.inner {
            c.insert("caused_by".into(), json!({"type": t, "reason": r}));
        }
        Value::Object(c)
    }

    /// The complete {"error": ..., "status": 400} body of a direct failure.
    pub fn error_body(&self) -> Value {
        let mut err = Map::new();
        err.insert("root_cause".into(), json!([self.object(false)]));
        if let Value::Object(o) = self.object(true) {
            err.extend(o);
        }
        json!({"error": err, "status": 400})
    }

    /// Wraps the script error as an ApiError rendering `error_body`.
    pub fn api_error(&self) -> ApiError {
        let mut e = ApiError::new("script_exception", self.kind(), 400);
        e.raw = Some(Box::new(self.error_body()));
        e
    }
}

// ---- shard failure envelopes ---------------------------------------------

fn obj(t: &str, r: &str) -> Value {
    json!({"type": t, "reason": r})
}

/// A search_phase_execution_exception with one failed shard.
pub fn shard_failure(index: &str, status: u16, root: Value, shard: Value, top: Value, phase: &str) -> ApiError {
    let body = json!({
        "error": {
            "root_cause": [root],
            "type": "search_phase_execution_exception",
            "reason": "all shards failed",
            "phase": phase,
            "grouped": true,
            "failed_shards": [{"shard": 0, "index": index, "node": NODE_ID, "reason": shard}],
            "caused_by": top,
        },
        "status": status,
    });
    let mut e = ApiError::new("search_phase_execution_exception", "all shards failed", status);
    e.raw = Some(Box::new(body));
    e
}

/// A script exception raised while executing a query phase script.
pub fn script_shard_error(idx: &Index, se: &ScriptErr) -> ApiError {
    shard_failure(&idx.name, 400, se.object(false), se.object(true), se.object(true), "query")
}

/// A compile error of a query script, which fails while the shard creates
/// the query.
pub fn script_query_compile_error(idx: &Index, se: &ScriptErr) -> ApiError {
    if !se.compile {
        return script_shard_error(idx, se);
    }
    let shard = json!({
        "type": "query_shard_exception",
        "reason": "failed to create query: compile error",
        "index": idx.name,
        "index_uuid": idx.uuid,
        "caused_by": se.object(true),
    });
    shard_failure(&idx.name, 400, se.object(false), shard, se.object(true), "query")
}

/// A non-script exception raised on the shard.
pub fn plain_shard_error(idx: &Index, e: &ApiError) -> ApiError {
    if e.raw.is_some() {
        return e.clone();
    }
    let status = if e.status == 0 { 400 } else { e.status };
    let o = obj(&e.typ, &e.reason);
    let top = if status == 404 {
        o.clone()
    } else {
        json!({"type": e.typ, "reason": e.reason, "caused_by": o.clone()})
    };
    shard_failure(&idx.name, status, o.clone(), o, top, "query")
}

/// Turns any script setup or execution error into a shard failure.
pub fn err_for_shard(idx: &Index, err: PrepErr, query: bool) -> ApiError {
    match err {
        PrepErr::Script(se) => {
            if query {
                script_query_compile_error(idx, &se)
            } else {
                script_shard_error(idx, &se)
            }
        }
        PrepErr::Api(ae) => {
            if ae.typ == "x_content_parse_exception"
                || (ae.typ == "illegal_argument_exception"
                    && ae.reason == "illegally specified <lang> for a stored script")
            {
                return ae;
            }
            plain_shard_error(idx, &ae)
        }
    }
}

/// A failing pipeline aggregation script, which runs on the coordinating
/// node during the reduce (fetch) phase.
pub fn pipeline_script_error(se: &ScriptErr) -> ApiError {
    let body = json!({
        "error": {
            "root_cause": [],
            "type": "search_phase_execution_exception",
            "reason": "",
            "phase": "fetch",
            "grouped": true,
            "failed_shards": [],
            "caused_by": se.object(true),
        },
        "status": 400,
    });
    let mut e = ApiError::new("search_phase_execution_exception", "", 400);
    e.raw = Some(Box::new(body));
    e
}

// ---- prepared scripts ----------------------------------------------------

/// Why a script could not be prepared.
#[derive(Debug, Clone)]
pub enum PrepErr {
    Script(ScriptErr),
    Api(ApiError),
}

impl From<ApiError> for PrepErr {
    fn from(e: ApiError) -> Self {
        PrepErr::Api(e)
    }
}

#[derive(Debug, Clone)]
enum Engine {
    Painless,
    Expr(expressions::Expr),
}

/// A script prepared for one context, in either language.
#[derive(Debug, Clone)]
pub struct Compiled {
    pub spec: ScriptSpec,
    pub ctx: String,
    engine: Engine,
}

/// Per-execution bindings of a script.
#[derive(Default)]
pub struct RunOpts {
    pub score: f64,
    pub value: Option<Value>,
    /// params._source
    pub source: Option<Value>,
    /// Extra context variables (ctx, state, states, ...); scripts may change
    /// them and the changed values are left here.
    pub vars: Map<String, Value>,
    /// Extra params (bucket paths, ...).
    pub params: Map<String, Value>,
}

/// Parses, resolves and compiles a script for a context.
pub fn prepare(raw: &Value, ctx_name: &str) -> Result<Compiled, PrepErr> {
    let spec = parse_spec(raw)?;
    prepare_spec(spec, ctx_name)
}

pub fn prepare_spec(spec: ScriptSpec, ctx_name: &str) -> Result<Compiled, PrepErr> {
    if spec.lang == "expression" {
        let ex = expressions::compile(&spec.src).map_err(|e| PrepErr::Script(e.script_err(&spec.src, &spec.err_name())))?;
        check_lang(&spec, ctx_name)?;
        return Ok(Compiled { spec, ctx: ctx_name.to_string(), engine: Engine::Expr(ex) });
    }
    check_lang(&spec, ctx_name)?;
    painless_compile(&spec, ctx_name).map_err(PrepErr::Script)?;
    Ok(Compiled { spec, ctx: ctx_name.to_string(), engine: Engine::Painless })
}

impl Compiled {
    pub fn is_expression(&self) -> bool {
        matches!(self.engine, Engine::Expr(_))
    }

    /// Links an expression against an index (no-op for painless).
    pub fn link(&self, idx: Option<&Index>, extra: &Map<String, Value>) -> Result<Option<expressions::Linked>, ScriptErr> {
        let ex = match &self.engine {
            Engine::Expr(ex) => ex,
            Engine::Painless => return Ok(None),
        };
        let mut params = self.spec.params.clone();
        for (k, v) in extra {
            params.insert(k.clone(), v.clone());
        }
        let needs_score = matches!(self.ctx.as_str(), "score" | "number_sort" | "aggs");
        expressions::link(ex, idx, &params, self.ctx == "aggs", needs_score)
            .map(Some)
            .map_err(|e| e.script_err(&self.spec.src, &self.spec.err_name()))
    }

    /// Executes the script for one document (qc is None for scripts without
    /// document access).
    pub fn run(&self, qc: Option<&QueryCtx>, o: &mut RunOpts) -> Result<Value, ScriptErr> {
        if let Engine::Expr(_) = &self.engine {
            let linked = self.link(qc.map(|q| q.idx), &o.params)?.expect("expression");
            let v = linked.eval(qc, o.score, o.value.as_ref().unwrap_or(&Value::Null));
            if matches!(self.ctx.as_str(), "filter" | "aggregation_selector") {
                return Ok(Value::Bool(v != 0.0));
            }
            return Ok(f64_value(v));
        }
        painless_backend(self, qc, o)
    }
}

/// A double as a JSON number (non-finite values as strings).
pub fn f64_value(v: f64) -> Value {
    if v.is_finite() {
        serde_json::Number::from_f64(v).map(Value::Number).unwrap_or(Value::Null)
    } else if v.is_nan() {
        json!("NaN")
    } else if v > 0.0 {
        json!("Infinity")
    } else {
        json!("-Infinity")
    }
}

/// The Painless name of the runtime type of a def value.
pub fn def_type_name(v: &Value) -> &'static str {
    match v {
        Value::Number(n) if n.is_i64() || n.is_u64() => "long",
        Value::Number(_) => "double",
        Value::Bool(_) => "boolean",
        Value::String(_) => "java.lang.String",
        Value::Array(_) => "java.util.ArrayList",
        Value::Object(_) => "java.util.HashMap",
        Value::Null => "null",
    }
}

/// Converts a script result into script_fields values; None means the field
/// is omitted from the hit.
pub fn field_script_values(v: Value) -> Option<Vec<Value>> {
    match v {
        Value::Array(a) => {
            if a.is_empty() {
                None
            } else {
                Some(a)
            }
        }
        other => Some(vec![other]),
    }
}

/// Renders org.opensearch.script.Script#toString.
pub fn script_to_string(raw: &Value) -> String {
    let spec = match parse_spec(raw) {
        Ok(s) => s,
        Err(_) => return String::new(),
    };
    let params = crate::painless::format_script_value(&Value::Object(spec.params.clone()));
    match &spec.id {
        Some(id) => format!("Script{{type=stored, lang='null', idOrCode='{}', options=null, params={}}}", id, params),
        None => format!(
            "Script{{type=inline, lang='{}', idOrCode='{}', options={{}}, params={}}}",
            spec.lang, spec.src, params
        ),
    }
}

// ---- query-phase scripts -------------------------------------------------

fn doc_position(idx: &Index, id: &str) -> usize {
    idx.order.iter().position(|d| d == id).unwrap_or(0)
}

/// Converts the result of a score-like script to a double.
pub fn score_value(v: &Value) -> f64 {
    match v {
        Value::Number(n) => n.as_f64().unwrap_or(0.0),
        Value::Bool(b) => {
            if *b {
                1.0
            } else {
                0.0
            }
        }
        Value::String(s) => match s.as_str() {
            "NaN" => f64::NAN,
            "Infinity" => f64::INFINITY,
            "-Infinity" => f64::NEG_INFINITY,
            _ => 0.0,
        },
        _ => 0.0,
    }
}

/// Java's Double.toString for score messages.
fn java_double(f: f64) -> String {
    if f.is_nan() {
        return "NaN".into();
    }
    if f.is_infinite() {
        return if f > 0.0 { "Infinity".into() } else { "-Infinity".into() };
    }
    if f == f.trunc() && f.abs() < 1e7 {
        return format!("{:.1}", f);
    }
    format!("{}", f)
}

/// The `script` query: a filter script per document.
pub fn script_query(qc: &QueryCtx, body: &Value) -> Result<(bool, f64), ApiError> {
    let bm = body
        .as_object()
        .ok_or_else(|| ApiError::parsing("[script] query malformed"))?;
    let raw = bm.get("script").cloned().unwrap_or(Value::Null);
    let cs = prepare(&raw, "filter").map_err(|e| err_for_shard(qc.idx, e, true))?;
    let mut opts = RunOpts::default();
    let v = cs.run(Some(qc), &mut opts).map_err(|se| script_shard_error(qc.idx, &se))?;
    let matched = match v {
        Value::Bool(b) => b,
        other => {
            let mut se = ScriptErr::new(
                false,
                "class_cast_exception",
                &format!("cannot cast def [{}] to boolean", def_type_name(&other)),
            );
            se.script = cs.spec.err_name();
            se.stack = vec![cs.spec.src.clone()];
            return Err(script_shard_error(qc.idx, &se));
        }
    };
    if !matched {
        return Ok((false, 0.0));
    }
    Ok((true, bm.get("boost").and_then(|b| b.as_f64()).unwrap_or(1.0)))
}

/// Runs a score-context script for a document; `fs` selects the
/// function_score error message for negative scores.
pub fn run_score_script(qc: &QueryCtx, raw: &Value, score: f64, fs: bool) -> Result<f64, ApiError> {
    let cs = prepare(raw, "score").map_err(|e| err_for_shard(qc.idx, e, true))?;
    let mut opts = RunOpts { score, ..Default::default() };
    let v = cs.run(Some(qc), &mut opts).map_err(|se| script_shard_error(qc.idx, &se))?;
    let s = score_value(&v);
    if fs {
        if s < 0.0 {
            return Err(plain_shard_error(
                qc.idx,
                &ApiError::bad(&format!(
                    "script score function must not produce negative scores, but got: [{}]",
                    java_double(s)
                )),
            ));
        }
    } else if s.is_nan() || s < 0.0 {
        return Err(plain_shard_error(
            qc.idx,
            &ApiError::bad(&format!(
                "script_score script returned an invalid score [{}] for doc [{}]. Must be a non-negative score!",
                java_double(s),
                doc_position(qc.idx, &qc.id)
            )),
        ));
    }
    Ok(s)
}

/// The `script_score` query.
pub fn script_score_query(qc: &QueryCtx, body: &Value) -> Result<(bool, f64), ApiError> {
    let bm = body
        .as_object()
        .ok_or_else(|| ApiError::parsing("[script_score] query malformed"))?;
    let inner = bm
        .get("query")
        .and_then(|q| q.as_object())
        .ok_or_else(|| ApiError::parsing("[script_score] requires 'query'"))?;
    let (matched, base) = qc.eval(inner)?;
    if !matched {
        return Ok((false, 0.0));
    }
    let raw = match bm.get("script") {
        Some(r) => r.clone(),
        None => return Ok((true, 1.0)),
    };
    let s = run_score_script(qc, &raw, base, false)?;
    if let Some(min) = bm.get("min_score").and_then(|v| v.as_f64()) {
        if s < min {
            return Ok((false, 0.0));
        }
    }
    let boost = bm.get("boost").and_then(|v| v.as_f64()).unwrap_or(1.0);
    Ok((true, s * boost))
}

/// The explanation of a top-level script_score query, if `q` is one.
pub fn script_score_explanation(q: &Value, score: f64) -> Option<Value> {
    let raw = q.get("script_score")?.get("script")?;
    Some(json!({
        "value": score,
        "description": format!("script score function, computed with script:\"{}\"", script_to_string(raw)),
        "details": [],
    }))
}

// ---- aggregation scripts -------------------------------------------------

/// Runs an aggregation script for one document (`vm` holds its doc
/// values); with a field value it is a value script (`_value`).
pub fn agg_script_run(
    idx: &Index,
    doc: &Value,
    raw: &Value,
    extra: &Map<String, Value>,
    value: Option<&Value>,
    vm: Map<String, Value>,
) -> Result<Value, ApiError> {
    let cs = prepare(raw, "aggs").map_err(|e| err_for_shard(idx, e, false))?;
    let id = doc.get("_id").and_then(|v| v.as_str()).unwrap_or("").to_string();
    let qc = QueryCtx::with_values(idx, &id, doc, vm);
    let mut params = Map::new();
    for (k, v) in extra {
        if cs.spec.params.get(k) != Some(v) {
            params.insert(k.clone(), v.clone());
        }
    }
    let mut opts = RunOpts {
        score: doc.get("_score").and_then(|s| s.as_f64()).unwrap_or(0.0),
        value: value.cloned(),
        params,
        ..Default::default()
    };
    cs.run(Some(&qc), &mut opts).map_err(|se| script_shard_error(idx, &se))
}

/// Evaluates a bucket_script / bucket_selector script with the bucket path
/// values as params; failures are reduce-phase errors.
pub fn run_pipeline_script(raw: &Value, ctx_name: &str, vars: &Map<String, Value>) -> Result<Value, ApiError> {
    let cs = match prepare(raw, ctx_name) {
        Ok(c) => c,
        Err(PrepErr::Api(e)) => return Err(e),
        Err(PrepErr::Script(se)) => return Err(pipeline_script_error(&se)),
    };
    let mut opts = RunOpts { params: vars.clone(), ..Default::default() };
    cs.run(None, &mut opts).map_err(|se| pipeline_script_error(&se))
}

// ---- update-context scripts (update, update_by_query, reindex) -----------

/// What an update-context script did to its ctx.
#[derive(Debug, Clone)]
pub struct CtxOutcome {
    /// index, delete or noop
    pub op: String,
    /// ctx.op as the script left it
    pub raw_op: String,
    pub source: Map<String, Value>,
    pub id: Value,
    pub index: Value,
}

/// The "failed to execute script" envelope of the update API.
pub fn update_failure(e: &PrepErr) -> ApiError {
    let cause = match e {
        PrepErr::Script(se) => se.object(true),
        PrepErr::Api(ae) => {
            let mut c = json!({"type": ae.typ, "reason": ae.reason});
            if let Some(cb) = &ae.caused_by {
                c["caused_by"] = json!({"type": cb.typ, "reason": cb.reason});
            }
            c
        }
    };
    let body = json!({
        "error": {
            "root_cause": [{"type": "illegal_argument_exception", "reason": "failed to execute script"}],
            "type": "illegal_argument_exception",
            "reason": "failed to execute script",
            "caused_by": cause,
        },
        "status": 400,
    });
    let mut out = ApiError::new("illegal_argument_exception", "failed to execute script", 400);
    out.raw = Some(Box::new(body));
    out
}

/// The error a by-query or reindex request reports for a failing script.
pub fn bulk_script_failure(e: &PrepErr) -> ApiError {
    match e {
        PrepErr::Script(se) => se.api_error(),
        PrepErr::Api(ae) => match &ae.caused_by {
            Some(cb) => ApiError::new(&cb.typ, &cb.reason, 400),
            None => ae.clone(),
        },
    }
}

/// Validates the ctx changes of an update_by_query script.
pub fn check_by_query_ctx(res: &CtxOutcome, index: &str, id: &str) -> Option<ApiError> {
    if !matches!(res.raw_op.as_str(), "noop" | "index" | "delete") {
        return Some(ApiError::bad(&format!(
            "Operation type [{}] not allowed, only [noop, index, delete] are allowed",
            res.raw_op
        )));
    }
    if value_text(&res.id) != id {
        return Some(ApiError::bad("Modifying [_id] not allowed"));
    }
    if value_text(&res.index) != index {
        return Some(ApiError::bad("Modifying [_index] not allowed"));
    }
    None
}

/// The string form of a scalar ctx value (Painless String.valueOf).
pub fn value_text(v: &Value) -> String {
    match v {
        Value::String(s) => s.clone(),
        Value::Null => "null".into(),
        other => other.to_string(),
    }
}

/// Runs an update-context script against a copy of ctx.
pub fn run_update_script(
    raw: &Value,
    source: &Map<String, Value>,
    index: &str,
    id: &str,
    version: i64,
    op: &str,
    routing: Option<&str>,
) -> Result<CtxOutcome, PrepErr> {
    let cs = prepare(raw, "update")?;
    let mut ctx = Map::new();
    ctx.insert("_index".into(), json!(index));
    ctx.insert("_id".into(), json!(id));
    ctx.insert("_version".into(), json!(version));
    ctx.insert("_routing".into(), routing.filter(|r| !r.is_empty()).map(|r| json!(r)).unwrap_or(Value::Null));
    ctx.insert("op".into(), json!(op));
    ctx.insert("_now".into(), json!(now_millis()));
    ctx.insert("_source".into(), Value::Object(source.clone()));
    painless_ctx_backend(&cs, &mut ctx).map_err(PrepErr::Script)?;
    let raw_op = match ctx.get("op") {
        Some(Value::String(s)) => s.clone(),
        Some(Value::Null) | None => "null".to_string(),
        Some(other) => other.to_string(),
    };
    let op = match raw_op.as_str() {
        "delete" => "delete",
        "index" | "create" => "index",
        _ => "noop",
    }
    .to_string();
    let source = match ctx.remove("_source") {
        Some(Value::Object(m)) => m,
        _ => Map::new(),
    };
    Ok(CtxOutcome {
        op,
        raw_op,
        source,
        id: ctx.remove("_id").unwrap_or(Value::Null),
        index: ctx.remove("_index").unwrap_or(Value::Null),
    })
}

fn now_millis() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_millis() as i64)
        .unwrap_or(0)
}

// ---- painless backend ----------------------------------------------------
//
// The only two functions that talk to the Painless implementation. The
// current engine is the expression subset in painless.rs; errors that it
// cannot describe are reported as generic runtime errors.

fn painless_compile(_spec: &ScriptSpec, _ctx_name: &str) -> Result<(), ScriptErr> {
    Ok(())
}

fn doc_field_refs(src: &str) -> Vec<(String, usize)> {
    let mut out = Vec::new();
    let mut from = 0;
    while let Some(p) = src[from..].find("doc[") {
        let start = from + p + 4;
        let rest = &src[start..];
        let q = rest.chars().next().unwrap_or(' ');
        if q == '\'' || q == '"' {
            if let Some(end) = rest[1..].find(q) {
                out.push((rest[1..1 + end].to_string(), from + p));
            }
        }
        from = start;
    }
    out
}

fn painless_backend(cs: &Compiled, qc: Option<&QueryCtx>, o: &mut RunOpts) -> Result<Value, ScriptErr> {
    let src = cs.spec.src.as_str();
    let name = cs.spec.err_name();
    let mk_err = |cause_type: &str, reason: &str| {
        let mut se = ScriptErr::new(false, cause_type, reason);
        se.script = name.clone();
        se.stack = vec![src.to_string()];
        se
    };
    if let Some(q) = qc {
        for (field, at) in doc_field_refs(src) {
            if crate::eval::mapping_field_def(q.idx, &field).is_none() && field != "_id" && field != "_index" {
                let mut se = mk_err("illegal_argument_exception", &format!("No field found for [{}] in mapping", field));
                se.stack = vec![
                    "org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:89)".into(),
                    "org.opensearch.search.lookup.LeafDocLookup.get(LeafDocLookup.java:55)".into(),
                    snippet(src, at + 4),
                    caret_line(at + 4),
                ];
                se.pos = Some(((at + 4) as i64, 0, src.len() as i64));
                return Err(se);
            }
        }
    }
    let mut params = cs.spec.params.clone();
    for (k, v) in &o.params {
        params.insert(k.clone(), v.clone());
    }
    if let Some(s) = &o.source {
        params.insert("_source".into(), s.clone());
    }
    params.insert("__score".into(), f64_value(o.score));
    if let Some(Value::Object(ctx)) = o.vars.get("ctx") {
        for (k, key) in [("_index", "__ctx_index"), ("_id", "__ctx_id"), ("_version", "__ctx_version"), ("_source", "__ctx_source")] {
            if let Some(v) = ctx.get(k) {
                params.insert(key.into(), v.clone());
            }
        }
    }
    if let Some(v) = &o.value {
        params.insert("_value".into(), v.clone());
    }
    let skeleton;
    let empty = Value::Null;
    let q = match qc {
        Some(q) => q,
        None => {
            skeleton = Index::skeleton("");
            return run_toy(&QueryCtx::new(&skeleton, "", &empty), params, src, &mk_err);
        }
    };
    run_toy(q, params, src, &mk_err)
}

/// Runs a script that works on a `ctx` map (update context).
fn painless_ctx_backend(cs: &Compiled, ctx: &mut Map<String, Value>) -> Result<(), ScriptErr> {
    let index = ctx.get("_index").and_then(|v| v.as_str()).unwrap_or("").to_string();
    let id = ctx.get("_id").and_then(|v| v.as_str()).unwrap_or("").to_string();
    let version = ctx.get("_version").and_then(|v| v.as_i64()).unwrap_or(0);
    let mut source = match ctx.get("_source") {
        Some(Value::Object(m)) => m.clone(),
        _ => Map::new(),
    };
    let mut op = String::new();
    let mut meta = Map::new();
    let idx = Index::skeleton(&index);
    crate::docs::run_ctx_script(&cs.spec.src, &idx, &mut source, &cs.spec.params, &id, version, &mut op, &mut meta)
        .map_err(|(runtime, msg)| {
            let mut se = ScriptErr::new(!runtime, "illegal_argument_exception", &msg);
            se.script = cs.spec.err_name();
            se.stack = vec![cs.spec.src.clone()];
            se
        })?;
    ctx.insert("_source".into(), Value::Object(source));
    if !op.is_empty() {
        ctx.insert("op".into(), json!(op));
    }
    for (k, v) in meta {
        ctx.insert(k, v);
    }
    Ok(())
}

fn run_toy(
    q: &QueryCtx,
    params: Map<String, Value>,
    src: &str,
    mk_err: &dyn Fn(&str, &str) -> ScriptErr,
) -> Result<Value, ScriptErr> {
    let env = crate::painless::ScriptEnv { c: q, params };
    env.eval_script_expression(src).map_err(|m| {
        if m == crate::painless::MISSING_DOC_VALUE {
            let mut se = mk_err("illegal_state_exception", &m);
            se.stack.insert(0, "org.opensearch.index.fielddata.ScriptDocValues$Longs.get(ScriptDocValues.java:145)".into());
            return se;
        }
        mk_err("illegal_argument_exception", &m)
    })
}

fn snippet_start(off: usize) -> usize {
    off.saturating_sub(25)
}

fn caret_line(off: usize) -> String {
    let start = snippet_start(off);
    let pad = off - start + if start > 0 { 4 } else { 0 };
    format!("{}^---- HERE", " ".repeat(pad))
}

fn snippet(src: &str, off: usize) -> String {
    let start = snippet_start(off);
    let end = (off + 25).min(src.len());
    let mut s = src.get(start..end).unwrap_or(src).to_string();
    if start > 0 {
        s = format!("... {}", s);
    }
    if end < src.len() {
        s.push_str(" ...");
    }
    s
}
