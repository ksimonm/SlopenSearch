// Glue between the scripting engines and the APIs that run scripts (search,
// aggregations, update, ingest): script specs, stored-script resolution,
// language checks, the shard-failure envelopes and one entry point that runs
// a prepared script with its bindings. Port of servers/go/script_ctx.go.
//
// The Painless engine is reached only through `painless_backend`; everything
// else here is engine independent.

use crate::scripting::{check_lang, parse_spec, ScriptSpec};
use crate::errors::ApiError;
use crate::search::eval::QueryCtx;
use crate::search::expressions;
use crate::model::{Index, NODE_ID};
use serde_json::{json, Map, Value};
use crate::painless::value::{from_json, to_json, MapKind, PV};
use std::cell::RefCell;
use std::collections::HashMap;
use std::rc::Rc;

// ---- stored scripts ------------------------------------------------------

// ---- script specs --------------------------------------------------------

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

#[derive(Clone)]
enum Engine {
    Painless(Rc<crate::painless::api::Script>),
    Expr(expressions::Expr),
}

/// A script prepared for one context, in either language.
#[derive(Clone)]
pub struct Compiled {
    pub spec: ScriptSpec,
    pub ctx: String,
    engine: Engine,
    /// An expression linked without extra params, and the index (name, uuid, mapping
    /// version) it was linked against.
    linked: RefCell<Option<(String, Rc<expressions::Linked>)>>,
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
    /// Set by `want_double` runs whose result is a double: the result (then not converted to JSON).
    pub want_double: bool,
    pub double: Option<f64>,
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
        return Ok(Compiled { spec, ctx: ctx_name.to_string(), engine: Engine::Expr(ex), linked: RefCell::new(None) });
    }
    check_lang(&spec, ctx_name)?;
    let script = painless_compile(&spec, ctx_name).map_err(PrepErr::Script)?;
    Ok(Compiled { spec, ctx: ctx_name.to_string(), engine: Engine::Painless(script), linked: RefCell::new(None) })
}

impl Compiled {
    /// Links an expression against an index (no-op for painless).
    pub fn link(&self, idx: Option<&Index>, extra: &Map<String, Value>) -> Result<Option<expressions::Linked>, ScriptErr> {
        let ex = match &self.engine {
            Engine::Expr(ex) => ex,
            Engine::Painless(_) => return Ok(None),
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
            let idx = qc.map(|q| q.idx);
            let key = idx.map(|i| format!("{}\u{0}{}\u{0}{}", i.name, i.uuid, i.mapping_version)).unwrap_or_default();
            let cached = match self.linked.borrow().as_ref() {
                Some((k, l)) if o.params.is_empty() && *k == key => Some(l.clone()),
                _ => None,
            };
            let linked = match cached {
                Some(l) => l,
                None => {
                    let l = Rc::new(self.link(idx, &o.params)?.expect("expression"));
                    if o.params.is_empty() {
                        *self.linked.borrow_mut() = Some((key, l.clone()));
                    }
                    l
                }
            };
            let v = linked.eval(qc, o.score, o.value.as_ref().unwrap_or(&Value::Null));
            if matches!(self.ctx.as_str(), "filter" | "aggregation_selector") {
                return Ok(Value::Bool(v != 0.0));
            }
            if o.want_double {
                o.double = Some(v);
                return Ok(Value::Null);
            }
            return Ok(f64_value(v));
        }
        match &self.engine {
            Engine::Painless(script) => painless_backend(self, script, qc, o),
            Engine::Expr(_) => unreachable!(),
        }
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
    let params = crate::search::script_env::format_script_value(&Value::Object(spec.params.clone()));
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
    let cs = prepare_score(raw).map_err(|e| err_for_shard(qc.idx, e, true))?;
    run_score_compiled(qc, &cs, score, fs)
}

thread_local! {
    /// Score scripts of request plans by plan key (`plan_key`).
    static PLANNED: RefCell<Vec<(u64, Rc<Planned>)>> = const { RefCell::new(Vec::new()) };
}

/// A key no other plan has, for `run_planned_score_script`.
pub fn plan_key() -> u64 {
    static NEXT: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(1);
    NEXT.fetch_add(1, std::sync::atomic::Ordering::Relaxed)
}

/// `run_score_script` for a script of a request plan: compiled once per thread, found by
/// the plan's `key` instead of by comparing the script.
pub fn run_planned_score_script(
    qc: &QueryCtx,
    key: u64,
    raw: &Value,
    score: f64,
    fs: bool,
    at: Option<crate::search::painless_doc::DocAt>,
) -> Result<f64, ApiError> {
    let entry = match PLANNED.with(|c| c.borrow().iter().rev().find(|e| e.0 == key).map(|e| e.1.clone())) {
        Some(e) => e,
        None => {
            let cs = prepare_score(raw).map_err(|e| err_for_shard(qc.idx, e, true))?;
            let fast = at.as_ref().and_then(|at| fast_script(qc.idx, &cs, at));
            let e = Rc::new(Planned { cs, fast });
            PLANNED.with(|c| {
                let mut c = c.borrow_mut();
                if c.len() >= 16 {
                    c.remove(0);
                }
                c.push((key, e.clone()));
            });
            e
        }
    };
    if let (Some(f), Some(at), Engine::Painless(script)) = (&entry.fast, &at, &entry.cs.engine) {
        let cols = FastCols { cols: &f.cols, doc: at.doc as usize };
        let r = FAST_SLOTS.with(|s| f.fast.run(script, &f.funcs, &cols, score, &mut s.borrow_mut()));
        // invalid scores: the interpreter's path reports them
        if let Some(s) = r.filter(|s| if fs { *s >= 0.0 } else { !s.is_nan() && *s >= 0.0 }) {
            return Ok(s);
        }
    }
    run_score_compiled(qc, &entry.cs, score, fs)
}

/// A plan's score script: compiled, and its compiled tier (painless::fast) when the
/// script and the fields it reads allow one.
struct Planned {
    cs: Rc<Compiled>,
    fast: Option<FastScript>,
}

struct FastScript {
    fast: crate::painless::fast::Fast,
    cols: Vec<std::sync::Arc<crate::search::painless_doc::DocColumn>>,
    funcs: Rc<HashMap<String, crate::painless::interp::NativeFn>>,
}

struct FastCols<'a> {
    cols: &'a [std::sync::Arc<crate::search::painless_doc::DocColumn>],
    doc: usize,
}

impl crate::painless::fast::Cols for FastCols<'_> {
    fn vals(&self, col: usize) -> &[crate::painless::fast::CV] {
        self.cols[col].vals(self.doc)
    }
}

thread_local! {
    static FAST_SLOTS: RefCell<Vec<PV>> = const { RefCell::new(Vec::new()) };
}

fn fast_disabled() -> bool {
    static OFF: std::sync::OnceLock<bool> = std::sync::OnceLock::new();
    *OFF.get_or_init(|| std::env::var("SLOP_NO_FAST_SCRIPTS").map(|v| v == "true" || v == "1").unwrap_or(false))
}

fn fast_script(idx: &Index, cs: &Compiled, at: &crate::search::painless_doc::DocAt) -> Option<FastScript> {
    let Engine::Painless(script) = &cs.engine else { return None };
    if fast_disabled() || cs.ctx != "score" {
        return None;
    }
    let skel = skeleton(idx);
    let funcs = crate::search::painless_doc::score_bindings("");
    let params = from_json(&Value::Object(cs.spec.params.clone()), MapKind::Hash);
    let mut cols: Vec<std::sync::Arc<crate::search::painless_doc::DocColumn>> = Vec::new();
    let mut names: Vec<String> = Vec::new();
    let mut field = |name: &str| -> Option<(usize, &'static str)> {
        if let Some(i) = names.iter().position(|n| n == name) {
            return Some((i, cols[i].kind));
        }
        let c = crate::search::painless_doc::doc_column(&skel, idx, at.version, at.ids, name)?;
        names.push(name.to_string());
        cols.push(c);
        Some((cols.len() - 1, cols[cols.len() - 1].kind))
    };
    let fast = crate::painless::fast::compile(script, &funcs, &params, &mut field);
    if crate::search::index::trace_on() {
        eprintln!("[trace] compiled tier {}: {}", if fast.is_some() { "yes" } else { "no" }, cs.spec.src);
    }
    Some(FastScript { fast: fast?, cols, funcs })
}

fn run_score_compiled(qc: &QueryCtx, cs: &Compiled, score: f64, fs: bool) -> Result<f64, ApiError> {
    let mut opts = RunOpts { score, want_double: true, ..Default::default() };
    let v = cs.run(Some(qc), &mut opts).map_err(|se| script_shard_error(qc.idx, &se))?;
    let s = opts.double.unwrap_or_else(|| score_value(&v));
    if fs {
        if s < 0.0 {
            return Err(plain_shard_error(
                qc.idx,
                &ApiError::bad(&format!(
                    "script score function must not produce negative scores, but got: [{}]",
                    crate::javafmt::double(s)
                )),
            ));
        }
    } else if s.is_nan() || s < 0.0 {
        return Err(plain_shard_error(
            qc.idx,
            &ApiError::bad(&format!(
                "script_score script returned an invalid score [{}] for doc [{}]. Must be a non-negative score!",
                crate::javafmt::double(s),
                doc_position(qc.idx, &qc.id)
            )),
        ));
    }
    Ok(s)
}

thread_local! {
    /// Inline score scripts of recent requests (a request runs its script per document).
    static SCORE_SCRIPTS: RefCell<Vec<(Value, Rc<Compiled>)>> = const { RefCell::new(Vec::new()) };
}

/// `prepare(raw, "score")`, memoized for inline scripts (stored scripts can change).
fn prepare_score(raw: &Value) -> Result<Rc<Compiled>, PrepErr> {
    let inline = raw.get("id").is_none();
    if inline {
        if let Some(c) = SCORE_SCRIPTS.with(|c| c.borrow().iter().find(|e| e.0 == *raw).map(|e| e.1.clone())) {
            return Ok(c);
        }
    }
    let c = Rc::new(prepare(raw, "score")?);
    if inline {
        SCORE_SCRIPTS.with(|m| {
            let mut m = m.borrow_mut();
            if m.len() >= 32 {
                m.remove(0);
            }
            m.push((raw.clone(), c.clone()));
        });
    }
    Ok(c)
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
    script_score_on(qc, bm, base)
}

/// `script_score` on a document its query matched with score `base`.
pub fn script_score_on(qc: &QueryCtx, bm: &Map<String, Value>, base: f64) -> Result<(bool, f64), ApiError> {
    script_score_keyed(qc, bm, base, None)
}

/// `script_score_on` with the script compiled once per plan `key` (see `plan_key`).
pub fn script_score_keyed(
    qc: &QueryCtx,
    bm: &Map<String, Value>,
    base: f64,
    key: Option<(u64, crate::search::painless_doc::DocAt)>,
) -> Result<(bool, f64), ApiError> {
    let raw = match bm.get("script") {
        Some(r) => r,
        None => return Ok((true, 1.0)),
    };
    let s = if raw.get("lang").and_then(|l| l.as_str()) == Some("knn") {
        crate::search::knn_query::knn_score_script(qc, raw)?
    } else if let Some((key, at)) = key {
        run_planned_score_script(qc, key, raw, base, false, Some(at))?
    } else {
        run_score_script(qc, raw, base, false)?
    };
    if let Some(min) = bm.get("min_score").and_then(|v| v.as_f64()) {
        if s < min {
            return Ok((false, 0.0));
        }
    }
    let boost = bm.get("boost").and_then(|v| v.as_f64()).unwrap_or(1.0);
    Ok((true, s * boost))
}

/// The functions the score context binds (ScoreScriptUtils): saturation,
/// sigmoid, the numeric decay functions and randomScore. None when `name`
/// is not one of them or the arity does not match.
pub fn score_function(name: &str, args: &[Value], qc: &QueryCtx) -> Option<f64> {
    let f = |i: usize| score_value(&args[i]);
    let dist = || ((f(4) - f(0)).abs() - f(2)).max(0.0);
    Some(match (name, args.len()) {
        ("saturation", 2) => f(0) / (f(1) + f(0)),
        ("sigmoid", 3) => {
            let (v, k, e) = (f(0), f(1), f(2));
            v.powf(e) / (k.powf(e) + v.powf(e))
        }
        ("decayNumericLinear", 5) => {
            let scaling = f(1) / (1.0 - f(3));
            ((scaling - dist()) / scaling).max(0.0)
        }
        ("decayNumericExp", 5) => {
            let scaling = f(3).ln() / f(1);
            (scaling * dist()).exp()
        }
        ("decayNumericGauss", 5) => {
            let scaling = 0.5 * f(1).powi(2) / f(3).ln();
            (0.5 * dist().powi(2) / scaling).exp()
        }
        ("randomScore", 1) | ("randomScore", 2) => {
            let mut key = qc.id.clone();
            if let Some(Value::String(field)) = args.get(1) {
                if let Some(v) = qc.values(field).0.first() {
                    key = value_text(v);
                }
            }
            let seed = f(0) as i64;
            let mut h: u32 = 0x811c9dc5;
            for b in format!("{}\u{0}{}", seed, key).bytes() {
                h ^= b as u32;
                h = h.wrapping_mul(0x01000193);
            }
            (h & 0x00FF_FFFF) as f64 / (1u32 << 24) as f64
        }
        _ => return None,
    })
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

/// Runs the init/map/combine/reduce scripts of a scripted_metric
/// aggregation over the documents of one shard; `vm` computes the doc
/// values of a document.
pub fn scripted_metric(
    name: &str,
    spec: &Map<String, Value>,
    docs: &[&Value],
    idx: &Index,
    vm: &dyn Fn(&Value) -> Map<String, Value>,
) -> Result<Value, ApiError> {
    if !spec.contains_key("map_script") {
        return Err(ApiError::bad("Required [map_script]"));
    }
    for (key, label) in [("combine_script", "combineScript"), ("reduce_script", "reduceScript")] {
        if !spec.contains_key(key) {
            return Err(plain_shard_error(
                idx,
                &ApiError::bad(&format!("[{}] must not be null: [{}]", label, name)),
            ));
        }
    }
    let agg_params = spec.get("params").and_then(|p| p.as_object()).cloned().unwrap_or_default();
    let prep = |key: &str, ctx_name: &str| -> Result<Compiled, ApiError> {
        let mut cs = prepare(&spec[key], ctx_name).map_err(|e| err_for_shard(idx, e, false))?;
        for (k, v) in &agg_params {
            cs.spec.params.entry(k.clone()).or_insert_with(|| v.clone());
        }
        Ok(cs)
    };
    let mut vars = Map::new();
    vars.insert("state".into(), Value::Object(Map::new()));
    if spec.contains_key("init_script") {
        let cs = prep("init_script", "aggs_init")?;
        let mut o = RunOpts { vars, ..Default::default() };
        cs.run(None, &mut o).map_err(|se| script_shard_error(idx, &se))?;
        vars = o.vars;
    }
    let map_cs = prep("map_script", "aggs_map")?;
    for d in docs {
        let id = d.get("_id").and_then(|v| v.as_str()).unwrap_or("").to_string();
        let qc = QueryCtx::with_values(idx, &id, d, vm(d));
        let mut o = RunOpts {
            vars,
            score: d.get("_score").and_then(|s| s.as_f64()).unwrap_or(0.0),
            ..Default::default()
        };
        map_cs.run(Some(&qc), &mut o).map_err(|se| script_shard_error(idx, &se))?;
        vars = o.vars;
    }
    let comb_cs = prep("combine_script", "aggs_combine")?;
    let mut o = RunOpts { vars, ..Default::default() };
    let combined = comb_cs.run(None, &mut o).map_err(|se| script_shard_error(idx, &se))?;
    let red_cs = prep("reduce_script", "aggs_reduce")?;
    let mut rvars = Map::new();
    rvars.insert("states".into(), Value::Array(vec![combined]));
    let mut o = RunOpts { vars: rvars, ..Default::default() };
    let res = red_cs.run(None, &mut o).map_err(|se| pipeline_script_error(&se))?;
    Ok(json!({"value": res}))
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
// The functions below are the only ones that talk to the Painless engine
// (crate::painless::api): compiling, binding the context variables and converting values
// between JSON and the engine.

thread_local! {
    static COMPILED: RefCell<HashMap<(String, String), Rc<crate::painless::api::Script>>> = RefCell::new(HashMap::new());
    static SKELETONS: RefCell<HashMap<String, Rc<Index>>> = RefCell::new(HashMap::new());
    /// The skeleton of the last lookup (one per document of a request) and its mapping version.
    static LAST_SKELETON: RefCell<Option<(i64, Rc<Index>)>> = const { RefCell::new(None) };
}

fn from_pl_err(e: crate::painless::api::ScriptErr) -> ScriptErr {
    ScriptErr {
        compile: e.compile,
        link: e.link_err,
        lang: e.lang,
        script: e.script,
        stack: e.stack,
        pos: e.pos.map(|(a, b, c)| (a as i64, b as i64, c as i64)),
        cause_type: e.cause_type,
        reason: e.reason,
        inner: e.inner.map(|c| (c.typ, c.reason)),
        extra: e.extra,
    }
}

fn painless_compile(spec: &ScriptSpec, ctx_name: &str) -> Result<Rc<crate::painless::api::Script>, ScriptErr> {
    let key = (ctx_name.to_string(), spec.src.clone());
    if let Some(s) = COMPILED.with(|c| c.borrow().get(&key).cloned()) {
        return Ok(s);
    }
    let script = Rc::new(crate::painless::api::compile_script(&spec.src, ctx_name, &spec.err_name()).map_err(from_pl_err)?);
    COMPILED.with(|c| c.borrow_mut().insert(key, script.clone()));
    Ok(script)
}

/// The mapping-only copy of an index that the engine's doc lookup needs.
fn skeleton(idx: &Index) -> Rc<Index> {
    if let Some(s) = LAST_SKELETON.with(|c| {
        c.borrow().as_ref().filter(|(v, s)| *v == idx.mapping_version && s.name == idx.name && s.uuid == idx.uuid).map(|e| e.1.clone())
    }) {
        return s;
    }
    let key = format!("{}\u{0}{}\u{0}{}", idx.name, idx.uuid, idx.mapping_version);
    if let Some(s) = SKELETONS.with(|c| c.borrow().get(&key).cloned()) {
        LAST_SKELETON.with(|c| *c.borrow_mut() = Some((idx.mapping_version, s.clone())));
        return s;
    }
    let mut tmp = Index::skeleton(&idx.name);
    tmp.uuid = idx.uuid.clone();
    tmp.settings = idx.settings.clone();
    tmp.mappings = idx.mappings.clone();
    tmp.field_types = idx.field_types.clone();
    let rc = Rc::new(tmp);
    SKELETONS.with(|c| c.borrow_mut().insert(key, rc.clone()));
    rc
}

fn map_get(m: &PV, key: &str) -> Option<PV> {
    match m {
        PV::Map(m) => m.borrow().get(&PV::str(key)),
        _ => None,
    }
}

fn painless_backend(
    cs: &Compiled,
    script: &crate::painless::api::Script,
    qc: Option<&QueryCtx>,
    o: &mut RunOpts,
) -> Result<Value, ScriptErr> {
    let mut params = cs.spec.params.clone();
    for (k, v) in &o.params {
        params.insert(k.clone(), v.clone());
    }
    let params = from_json(&Value::Object(params), MapKind::Hash);
    if let (Some(src), PV::Map(m)) = (&o.source, &params) {
        m.borrow_mut().put(PV::str("_source"), from_json(src, MapKind::Linked));
    }
    let mut b = crate::painless::api::Bindings::default();
    b.vars.insert("params".into(), params.clone());
    b.vars.insert("_score".into(), PV::Double(o.score));
    if let Some(v) = &o.value {
        b.vars.insert("_value".into(), from_json(v, MapKind::Hash));
    }
    let mut var_pvs = Vec::new();
    for (k, v) in &o.vars {
        let pv = from_json(v, if k == "ctx" { MapKind::Linked } else { MapKind::Hash });
        b.vars.insert(k.clone(), pv.clone());
        var_pvs.push((k.clone(), pv));
    }
    let mut values = None;
    if let Some(q) = qc {
        values = q.values_map();
        b.vars.insert("doc".into(), crate::search::painless_doc::doc_lookup_borrowed(skeleton(q.idx), &q.id));
        if cs.ctx == "score" {
            b.funcs = crate::search::painless_doc::score_bindings(&q.id);
        }
    }
    let empty = Map::new();
    let r = crate::search::painless_doc::with_doc_values(values.unwrap_or(&empty), || script.exec(&b, &cs.spec.err_name()))
        .map_err(from_pl_err)?;
    for (k, pv) in var_pvs {
        o.vars.insert(k, to_json(&pv));
    }
    if o.source.is_some() {
        if let Some(src) = map_get(&params, "_source") {
            o.source = Some(to_json(&src));
        }
    }
    if let (true, PV::Double(d)) = (o.want_double, &r) {
        o.double = Some(*d);
        return Ok(Value::Null);
    }
    Ok(to_json(&r))
}

/// Runs a script that works on a `ctx` map (update context).
fn painless_ctx_backend(cs: &Compiled, ctx: &mut Map<String, Value>) -> Result<(), ScriptErr> {
    let mut o = RunOpts::default();
    o.vars.insert("ctx".into(), Value::Object(std::mem::take(ctx)));
    let r = match &cs.engine {
        Engine::Painless(script) => painless_backend(cs, script, None, &mut o).map(|_| ()),
        Engine::Expr(_) => Ok(()),
    };
    if let Some(Value::Object(m)) = o.vars.remove("ctx") {
        *ctx = m;
    }
    r
}
