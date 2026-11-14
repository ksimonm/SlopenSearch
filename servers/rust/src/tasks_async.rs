//! Asynchronous tasks: `wait_for_completion=false` on `_reindex`,
//! `_update_by_query`, `_delete_by_query` and `_forcemerge`, the task registry
//! behind `GET _tasks/<id>`, batch-wise execution throttled by
//! `requests_per_second`, `_rethrottle` and `_cancel`.

use std::collections::HashMap;
use std::future::Future;
use std::sync::{Condvar, Mutex, MutexGuard, OnceLock};
use std::task::{Context, Poll, Waker};
use std::time::{Duration, Instant};

use axum::response::Response;
use serde_json::{json, Map, Value};

use crate::errors::{err_response, json_response, ApiError};
use crate::shared::{resolve_spec2, ResolveOpts2};
use crate::store::{now_millis, Store, NODE_ID, NODE_NAME};
use crate::{Req, Shared};

const RPS_REASON: &str =
    "[requests_per_second] must be a float greater than 0. Use -1 to disable throttling.";

#[derive(Clone, Copy, PartialEq)]
enum Kind {
    Reindex,
    Update,
    Delete,
    Forcemerge,
}

impl Kind {
    fn action(self) -> &'static str {
        match self {
            Kind::Reindex => "indices:data/write/reindex",
            Kind::Update => "indices:data/write/update/byquery",
            Kind::Delete => "indices:data/write/delete/byquery",
            Kind::Forcemerge => "indices:admin/forcemerge",
        }
    }
}

#[derive(Default, Clone)]
struct Counts {
    total: i64,
    updated: i64,
    created: i64,
    deleted: i64,
    batches: i64,
    version_conflicts: i64,
    noops: i64,
}

impl Counts {
    fn add(&mut self, v: &Value) {
        let n = |k: &str| v.get(k).and_then(|x| x.as_i64()).unwrap_or(0);
        self.updated += n("updated");
        self.created += n("created");
        self.deleted += n("deleted");
        self.version_conflicts += n("version_conflicts");
        self.noops += n("noops");
    }
}

struct Done {
    nanos: i64,
    status: Option<Value>,
    body: Value,
    is_error: bool,
}

struct Task {
    id: u64,
    kind: Kind,
    description: String,
    start_ms: i64,
    start: Instant,
    headers: Map<String, Value>,
    c: Counts,
    rps: f64,
    next_at: Option<Instant>,
    cancelled_ms: Option<i64>,
    done: Option<Done>,
}

struct Registry {
    next: u64,
    tasks: HashMap<u64, Task>,
}

fn registry() -> &'static (Mutex<Registry>, Condvar) {
    static REG: OnceLock<(Mutex<Registry>, Condvar)> = OnceLock::new();
    REG.get_or_init(|| {
        (
            Mutex::new(Registry {
                next: 1000,
                tasks: HashMap::new(),
            }),
            Condvar::new(),
        )
    })
}

fn reg_lock() -> MutexGuard<'static, Registry> {
    registry().0.lock().unwrap_or_else(|e| e.into_inner())
}

fn notify() {
    registry().1.notify_all();
}

// ------------------------------------------------------------------ helpers

/// Java's Float.toString rendering of a requests_per_second value.
fn rps_value(f: f64) -> Value {
    let x = f as f32;
    let s = if x == 0.0 {
        "0.0".to_string()
    } else if x.abs() >= 1e-3 && x.abs() < 1e7 {
        format!("{:?}", x)
    } else {
        let e = format!("{:e}", x);
        let (m, ex) = e.split_once('e').unwrap_or((&e, "0"));
        let m = if m.contains('.') { m.to_string() } else { format!("{}.0", m) };
        format!("{}E{}", m, ex)
    };
    Value::Number(serde_json::Number::from_string_unchecked(s))
}

fn number_format(v: &str) -> ApiError {
    ApiError::new(
        "number_format_exception",
        &format!("For input string: \"{}\"", v),
        400,
    )
}

/// Parses requests_per_second; `None` (absent) means unthrottled (-1).
fn parse_rps(v: Option<&str>) -> Result<f64, ApiError> {
    let v = match v {
        None | Some("") => return Ok(-1.0),
        Some(v) => v,
    };
    let f: f32 = match v.trim().parse() {
        Ok(f) => f,
        Err(_) => {
            return Err(ApiError::new("illegal_argument_exception", RPS_REASON, 400)
                .caused_by(number_format(v)))
        }
    };
    if f == -1.0 {
        return Ok(-1.0);
    }
    if f.is_nan() || f <= 0.0 {
        return Err(ApiError::new("illegal_argument_exception", RPS_REASON, 400));
    }
    Ok(f as f64)
}

fn malformed(id: &str) -> ApiError {
    ApiError::new(
        "illegal_argument_exception",
        &format!("malformed task id {}", id),
        400,
    )
}

/// Splits "node:number" the way TaskId does.
fn parse_task_id(id: &str) -> Result<(String, i64), ApiError> {
    let (node, num) = match id.split_once(':') {
        Some(p) => p,
        None => return Err(malformed(id)),
    };
    let ok = !num.is_empty()
        && num
            .strip_prefix('-')
            .unwrap_or(num)
            .chars()
            .all(|c| c.is_ascii_digit())
        && num != "-";
    match num.parse::<i64>() {
        Ok(n) if ok => Ok((node.to_string(), n)),
        _ => Err(malformed(id).caused_by(number_format(num))),
    }
}

fn parse_time_ms(v: &str) -> Option<u64> {
    let v = v.trim();
    let units: [(&str, f64); 7] = [
        ("nanos", 1e-6),
        ("micros", 1e-3),
        ("ms", 1.0),
        ("s", 1000.0),
        ("m", 60_000.0),
        ("h", 3_600_000.0),
        ("d", 86_400_000.0),
    ];
    for (u, mul) in units {
        if let Some(n) = v.strip_suffix(u) {
            if let Ok(f) = n.parse::<f64>() {
                return Some((f * mul) as u64);
            }
        }
    }
    v.parse::<u64>().ok()
}

fn node_failure(reason: &str) -> Response {
    json_response(
        200,
        json!({
            "node_failures": [{
                "type": "failed_node_exception",
                "reason": format!("Failed node [{}]", NODE_ID),
                "node_id": NODE_ID,
                "caused_by": {"type": "resource_not_found_exception", "reason": reason},
            }],
            "nodes": {},
        }),
    )
}

fn node_tasks(tid: &str, task: Value) -> Response {
    let mut tasks = Map::new();
    tasks.insert(tid.to_string(), task);
    json_response(
        200,
        json!({"nodes": {NODE_ID: {
            "name": NODE_NAME,
            "transport_address": "127.0.0.1:9300",
            "host": crate::cluster::NODE_HOST,
            "ip": "127.0.0.1:9300",
            "roles": crate::cluster::node_roles(),
            "attributes": crate::cluster::node_attributes(),
            "tasks": tasks,
        }}}),
    )
}

/// Reads a handler response synchronously (handler bodies are always full).
fn read_response(resp: Response) -> (u16, Value) {
    let status = resp.status().as_u16();
    let fut = axum::body::to_bytes(resp.into_body(), usize::MAX);
    let mut fut = std::pin::pin!(fut);
    let mut cx = Context::from_waker(Waker::noop());
    let bytes = loop {
        if let Poll::Ready(r) = fut.as_mut().poll(&mut cx) {
            break r.map(|b| b.to_vec()).unwrap_or_default();
        }
        std::thread::yield_now();
    };
    (status, serde_json::from_slice(&bytes).unwrap_or(Value::Null))
}

/// Builds a BulkByScrollTask status (or, with `took`, a BulkByScrollResponse).
fn status_json(
    c: &Counts,
    slice: Option<usize>,
    took: Option<i64>,
    response: bool,
    rps: f64,
    until_ms: i64,
    canceled: bool,
) -> Map<String, Value> {
    let mut m = Map::new();
    if let Some(s) = slice {
        m.insert("slice_id".into(), json!(s));
    }
    if let Some(t) = took {
        m.insert("took".into(), json!(t));
        m.insert("timed_out".into(), json!(false));
    }
    m.insert("total".into(), json!(c.total));
    m.insert("updated".into(), json!(c.updated));
    m.insert("created".into(), json!(c.created));
    m.insert("deleted".into(), json!(c.deleted));
    m.insert("batches".into(), json!(c.batches));
    m.insert("version_conflicts".into(), json!(c.version_conflicts));
    m.insert("noops".into(), json!(c.noops));
    m.insert("retries".into(), json!({"bulk": 0, "search": 0}));
    if response {
        m.insert("throttled".into(), json!("0s"));
    }
    m.insert("throttled_millis".into(), json!(0));
    m.insert("requests_per_second".into(), rps_value(rps));
    if canceled {
        m.insert("canceled".into(), json!("by user request"));
    }
    if response {
        m.insert("throttled_until".into(), json!("0s"));
    }
    m.insert("throttled_until_millis".into(), json!(until_ms));
    m
}

fn resource_stats() -> Value {
    let z = json!({"cpu_time_in_nanos": 0, "memory_in_bytes": 0});
    json!({"average": z, "total": z, "min": z, "max": z,
           "thread_info": {"thread_executions": 0, "active_threads": 0}})
}

impl Task {
    fn by_scroll(&self) -> bool {
        self.kind != Kind::Forcemerge
    }

    fn running_status(&self) -> Value {
        let until = self
            .next_at
            .map(|at| at.saturating_duration_since(Instant::now()).as_millis() as i64)
            .unwrap_or(0);
        Value::Object(status_json(&self.c, None, None, false, self.rps, until, false))
    }

    /// The task object; `full` adds status and description.
    fn json(&self, full: bool) -> Map<String, Value> {
        let mut m = Map::new();
        m.insert("node".into(), json!(NODE_ID));
        m.insert("id".into(), json!(self.id));
        m.insert("type".into(), json!("transport"));
        m.insert("action".into(), json!(self.kind.action()));
        if full && self.by_scroll() {
            let st = match &self.done {
                Some(d) => d.status.clone().unwrap_or(Value::Null),
                None => self.running_status(),
            };
            m.insert("status".into(), st);
        }
        if full {
            m.insert("description".into(), json!(self.description));
        }
        m.insert("start_time_in_millis".into(), json!(self.start_ms));
        let nanos = match &self.done {
            Some(d) => d.nanos,
            None => self.start.elapsed().as_nanos() as i64,
        };
        m.insert("running_time_in_nanos".into(), json!(nanos));
        m.insert("cancellable".into(), json!(self.by_scroll()));
        m.insert("cancelled".into(), json!(self.cancelled_ms.is_some()));
        m.insert("headers".into(), Value::Object(self.headers.clone()));
        if let Some(ms) = self.cancelled_ms {
            m.insert("cancellation_time_millis".into(), json!(ms));
        }
        if full && self.done.is_none() {
            m.insert("resource_stats".into(), resource_stats());
        }
        m
    }

    fn get_json(&self) -> Value {
        let mut out = Map::new();
        out.insert("completed".into(), json!(self.done.is_some()));
        out.insert("task".into(), Value::Object(self.json(true)));
        if let Some(d) = &self.done {
            let key = if d.is_error { "error" } else { "response" };
            out.insert(key.into(), d.body.clone());
        }
        Value::Object(out)
    }
}

/// Running tasks for the `_tasks` list API, with status and description.
pub(crate) fn running_tasks() -> Vec<Map<String, Value>> {
    let reg = reg_lock();
    let mut ids: Vec<&u64> = reg.tasks.keys().collect();
    ids.sort();
    ids.into_iter()
        .filter_map(|id| reg.tasks.get(id))
        .filter(|t| t.done.is_none())
        .map(|t| t.json(true))
        .collect()
}

// ------------------------------------------------------------------ routing

fn async_kind(segs: &[String]) -> Option<Kind> {
    let last = segs.last()?;
    match last.as_str() {
        "_reindex" if segs.len() == 1 => Some(Kind::Reindex),
        "_update_by_query" if segs.len() == 2 => Some(Kind::Update),
        "_delete_by_query" if segs.len() == 2 => Some(Kind::Delete),
        "_forcemerge" if segs.len() <= 2 => Some(Kind::Forcemerge),
        _ => None,
    }
}

/// Serves async starts, rethrottle, single-task GET and cancel; `None` when the
/// request is not one of them.
pub fn route(shared: &Shared, req: &Req) -> Option<Response> {
    let segs: Vec<String> = req
        .path
        .split('/')
        .filter(|s| !s.is_empty())
        .map(crate::path_decode)
        .collect();
    if segs.len() == 3
        && segs[2] == "_rethrottle"
        && req.method == "POST"
        && matches!(segs[0].as_str(), "_reindex" | "_update_by_query" | "_delete_by_query")
    {
        return Some(rethrottle(req, &segs[1]));
    }
    if segs.len() >= 2 && segs[0] == "_tasks" && segs[1] != "_cancel" {
        if segs.len() == 3 && segs[2] == "_cancel" && req.method == "POST" {
            return Some(cancel(&segs[1]));
        }
        if segs.len() == 2 && (req.method == "GET" || req.method == "HEAD") {
            return Some(get_task(req, &segs[1]));
        }
        return None;
    }
    let kind = async_kind(&segs)?;
    if req.method != "POST" {
        return None;
    }
    let wfc = req.param("wait_for_completion")?;
    match crate::errors::strict_bool(wfc) {
        Err(e) => Some(err_response(e)),
        Ok(true) => None,
        Ok(false) => Some(start(shared, req, &segs, kind)),
    }
}

fn rethrottle(req: &Req, id: &str) -> Response {
    let rps = match req.param("requests_per_second") {
        None => {
            return err_response(ApiError::bad(
                "requests_per_second is a required parameter",
            ))
        }
        Some(v) => match parse_rps(Some(v)) {
            Ok(f) => f,
            Err(e) => return err_response(e),
        },
    };
    let (node, n) = match parse_task_id(id) {
        Ok(p) => p,
        Err(e) => return err_response(e),
    };
    let missing = format!("task [{}] is missing", id);
    if node != NODE_ID || n < 0 {
        return node_failure(&missing);
    }
    let mut reg = reg_lock();
    let t = match reg.tasks.get_mut(&(n as u64)) {
        Some(t) if t.done.is_none() && t.by_scroll() => t,
        _ => return node_failure(&missing),
    };
    let old = t.rps;
    if let Some(at) = t.next_at {
        let now = Instant::now();
        if rps <= 0.0 {
            t.next_at = None;
        } else if old > 0.0 && rps > old {
            let remaining = at.saturating_duration_since(now).as_secs_f64();
            t.next_at = Some(now + Duration::from_secs_f64(remaining * old / rps));
        }
    }
    t.rps = rps;
    let body = Value::Object(t.json(true));
    drop(reg);
    notify();
    node_tasks(id, body)
}

fn cancel(id: &str) -> Response {
    let (node, n) = match parse_task_id(id) {
        Ok(p) => p,
        Err(e) => return err_response(e),
    };
    let not_found = format!("task [{}] is not found", id);
    if node != NODE_ID || n < 0 {
        return node_failure(&not_found);
    }
    let mut reg = reg_lock();
    let t = match reg.tasks.get_mut(&(n as u64)) {
        Some(t) if t.done.is_none() && t.by_scroll() => t,
        _ => return node_failure(&not_found),
    };
    if t.cancelled_ms.is_none() {
        t.cancelled_ms = Some(now_millis());
    }
    let body = Value::Object(t.json(false));
    drop(reg);
    notify();
    node_tasks(id, body)
}

fn get_task(req: &Req, id: &str) -> Response {
    let wait = match req.param("wait_for_completion") {
        None => false,
        Some(v) => match crate::errors::strict_bool(v) {
            Ok(b) => b,
            Err(e) => return err_response(e),
        },
    };
    let (node, n) = match parse_task_id(id) {
        Ok(p) => p,
        Err(e) => return err_response(e),
    };
    let not_stored = format!("task [{}] isn't running and hasn't stored its results", id);
    if node != NODE_ID {
        let mut e = ApiError::new(
            "resource_not_found_exception",
            &format!(
                "task [{}] belongs to the node [{}] which isn't part of the cluster and there is no record of the task",
                id, node
            ),
            404,
        )
        .caused_by(ApiError::new("resource_not_found_exception", &not_stored, 404));
        e.root_cause = Some(Box::new(ApiError::new(
            "resource_not_found_exception",
            &not_stored,
            404,
        )));
        return err_response(e);
    }
    let not_found = || err_response(ApiError::new("resource_not_found_exception", &not_stored, 404));
    if n < 0 {
        return not_found();
    }
    let timeout = req
        .param("timeout")
        .and_then(parse_time_ms)
        .unwrap_or(30_000);
    let deadline = Instant::now() + Duration::from_millis(timeout);
    let mut reg = reg_lock();
    loop {
        let t = match reg.tasks.get(&(n as u64)) {
            Some(t) => t,
            None => return not_found(),
        };
        if !wait || t.done.is_some() {
            return json_response(200, t.get_json());
        }
        let now = Instant::now();
        if now >= deadline {
            let class = if t.by_scroll() {
                "org.opensearch.index.reindex.BulkByScrollTask"
            } else {
                "org.opensearch.tasks.Task"
            };
            return err_response(ApiError::new(
                "timeout_exception",
                &format!(
                    "Timed out waiting for completion of [{}@{:x}]",
                    class,
                    0x6e8d_0000u64 + t.id
                ),
                504,
            ));
        }
        reg = registry()
            .1
            .wait_timeout(reg, deadline - now)
            .unwrap_or_else(|e| e.into_inner())
            .0;
    }
}

// ------------------------------------------------------------------ start

fn index_names(v: Option<&Value>) -> String {
    match v {
        Some(Value::String(s)) => s.clone(),
        Some(Value::Array(a)) => a
            .iter()
            .map(|x| x.as_str().map(|s| s.to_string()).unwrap_or_else(|| x.to_string()))
            .collect::<Vec<_>>()
            .join(", "),
        _ => String::new(),
    }
}

struct Job {
    kind: Kind,
    index: String,
    base: Req,
    body: Map<String, Value>,
    batch_size: usize,
    sliced: bool,
}

fn start(shared: &Shared, req: &Req, segs: &[String], kind: Kind) -> Response {
    let mut rps = -1.0;
    let mut sliced = false;
    let mut batch_size = 1000usize;
    if kind != Kind::Forcemerge {
        rps = match parse_rps(req.param("requests_per_second")) {
            Ok(f) => f,
            Err(e) => return err_response(e),
        };
        if let Some(v) = req.param("slices") {
            if v != "auto" {
                let reason = format!(
                    "[slices] must be a positive integer or the string \"auto\", but was [{}]",
                    v
                );
                match v.parse::<i64>() {
                    Ok(n) if n > 0 => sliced = n > 1,
                    Ok(_) => return err_response(ApiError::bad(&reason)),
                    Err(_) => {
                        return err_response(ApiError::bad(&reason).caused_by(number_format(v)))
                    }
                }
            }
        }
        if let Some(v) = req.param("scroll_size") {
            match v.parse::<i64>() {
                Ok(0) => {
                    return err_response(ApiError::validation(
                        "Validation Failed: 1: [size] cannot be [0] in a scroll context;",
                    ))
                }
                Ok(n) if n > 0 => batch_size = n as usize,
                _ => {}
            }
        }
    }
    let body = match req.json_body() {
        Ok(b) => b,
        Err(e) => return err_response(e),
    };
    let index = if segs.len() == 2 { segs[0].clone() } else { String::new() };
    let description = match kind {
        Kind::Reindex => {
            let src = body.get("source");
            if let Some(n) = src
                .and_then(|s| s.get("size"))
                .and_then(|v| v.as_i64())
                .filter(|n| *n > 0)
            {
                batch_size = n as usize;
            }
            format!(
                "reindex from [{}] to [{}]",
                index_names(src.and_then(|s| s.get("index"))),
                index_names(body.get("dest").and_then(|d| d.get("index")))
            )
        }
        Kind::Update => format!("update-by-query [{}]", index),
        Kind::Delete => format!("delete-by-query [{}]", index),
        Kind::Forcemerge => format!(
            "Force-merge indices [{}], maxSegments[{}], onlyExpungeDeletes[{}], flush[{}], primaryOnly[{}]",
            if index.is_empty() { "_all" } else { &index },
            req.param("max_num_segments").unwrap_or("-1"),
            req.param("only_expunge_deletes").map(|v| v != "false").unwrap_or(false),
            req.param("flush").map(|v| v != "false").unwrap_or(true),
            req.param("primary_only").map(|v| v != "false").unwrap_or(false),
        ),
    };
    let mut headers = Map::new();
    if let Some(op) = req.headers.get("x-opaque-id").and_then(|v| v.to_str().ok()) {
        headers.insert("X-Opaque-Id".into(), json!(op));
    }
    let id = {
        let mut reg = reg_lock();
        reg.next += 1;
        let id = reg.next;
        reg.tasks.insert(
            id,
            Task {
                id,
                kind,
                description,
                start_ms: now_millis(),
                start: Instant::now(),
                headers,
                c: Counts::default(),
                rps,
                next_at: None,
                cancelled_ms: None,
                done: None,
            },
        );
        id
    };
    let job = Job {
        kind,
        index,
        base: Req {
            method: "POST".into(),
            path: req.path.clone(),
            query: req
                .query
                .iter()
                .filter(|(k, _)| {
                    !matches!(
                        k.as_str(),
                        "wait_for_completion" | "requests_per_second" | "scroll_size" | "max_docs" | "q"
                    )
                })
                .cloned()
                .collect(),
            headers: req.headers.clone(),
            body: req.body.clone(),
        },
        body,
        batch_size,
        sliced,
    };
    let q_param = req.param("q").map(|s| s.to_string());
    let max_docs_param = req.param("max_docs").and_then(|v| v.parse::<usize>().ok());
    let shared = shared.clone();
    let spawned = std::thread::Builder::new()
        .name(format!("task-{}", id))
        .stack_size(256 << 20)
        .spawn(move || run(shared, id, job, q_param, max_docs_param));
    if spawned.is_err() {
        finish_error(id, &ApiError::new("exception", "failed to start task", 500));
    }
    json_response(200, json!({"task": format!("{}:{}", NODE_ID, id)}))
}

// ------------------------------------------------------------------ execution

fn lock_store(shared: &Shared) -> MutexGuard<'_, Store> {
    shared.lock().unwrap_or_else(|e| e.into_inner())
}

fn sub_req(base: &Req, path: String, body: &Map<String, Value>, drop_slices: bool) -> Req {
    Req {
        method: "POST".into(),
        path,
        query: base
            .query
            .iter()
            .filter(|(k, _)| !(drop_slices && k == "slices"))
            .cloned()
            .collect(),
        headers: base.headers.clone(),
        body: serde_json::to_vec(&Value::Object(body.clone())).unwrap_or_default(),
    }
}

fn error_body(v: &Value) -> Value {
    let mut e = v.get("error").cloned().unwrap_or_else(|| v.clone());
    if let Some(m) = e.as_object_mut() {
        m.shift_remove("root_cause");
    }
    e
}

fn finish(id: u64, status: Option<Value>, body: Value, is_error: bool) {
    let mut reg = reg_lock();
    if let Some(t) = reg.tasks.get_mut(&id) {
        t.next_at = None;
        t.done = Some(Done {
            nanos: t.start.elapsed().as_nanos() as i64,
            status,
            body,
            is_error,
        });
    }
    drop(reg);
    notify();
}

fn finish_error(id: u64, e: &ApiError) {
    finish_error_value(id, error_body(&e.to_value()));
}

fn run(shared: Shared, id: u64, job: Job, q_param: Option<String>, max_docs: Option<usize>) {
    if job.kind == Kind::Forcemerge {
        let req = sub_req(&job.base, job.base.path.clone(), &job.body, false);
        let (status, v) = {
            let mut st = lock_store(&shared);
            {
                let r = read_response(crate::dispatch(&mut st, &req));
                // background batches change state outside a request: journal them too
                crate::persist::after_request(&st, &req.method, &req.path);
                r
            }
        };
        if status >= 400 {
            finish(id, None, error_body(&v), true);
        } else {
            finish(id, None, v, false);
        }
        return;
    }
    if job.sliced {
        run_sliced(&shared, id, &job);
        return;
    }
    let docs = {
        let st = lock_store(&shared);
        list_docs(&st, &job, q_param.as_deref(), max_docs)
    };
    let docs = match docs {
        Ok(d) => d,
        Err(e) => return finish_error(id, &e),
    };
    {
        let mut reg = reg_lock();
        if let Some(t) = reg.tasks.get_mut(&id) {
            t.c.total = docs.len() as i64;
        }
    }
    notify();
    let mut failures: Vec<Value> = Vec::new();
    'batches: for chunk in docs.chunks(job.batch_size.max(1)) {
        {
            let mut reg = reg_lock();
            loop {
                let t = match reg.tasks.get_mut(&id) {
                    Some(t) => t,
                    None => return,
                };
                if t.cancelled_ms.is_some() {
                    break 'batches;
                }
                let now = Instant::now();
                match t.next_at {
                    Some(at) if now < at => {
                        reg = registry()
                            .1
                            .wait_timeout(reg, at - now)
                            .unwrap_or_else(|e| e.into_inner())
                            .0;
                    }
                    _ => {
                        t.next_at = None;
                        t.c.batches += 1;
                        break;
                    }
                }
            }
        }
        notify();
        let started = Instant::now();
        // one sub-request per source index of the batch
        let mut groups: Vec<(String, Vec<String>)> = Vec::new();
        for (ix, doc_id) in chunk {
            match groups.last_mut() {
                Some((g, ids)) if g == ix => ids.push(doc_id.clone()),
                _ => groups.push((ix.clone(), vec![doc_id.clone()])),
            }
        }
        let mut delta = Counts::default();
        for (ix, ids) in groups {
            let ids_q = json!({"ids": {"values": ids}});
            let mut body = job.body.clone();
            let path = match job.kind {
                Kind::Reindex => {
                    body.shift_remove("max_docs");
                    body.shift_remove("size");
                    if let Some(src) = body.get_mut("source").and_then(|s| s.as_object_mut()) {
                        src.insert("index".into(), json!(ix));
                        src.insert("query".into(), ids_q);
                    }
                    "/_reindex".to_string()
                }
                _ => {
                    body.shift_remove("max_docs");
                    body.insert("query".into(), ids_q);
                    let op = if job.kind == Kind::Update { "_update_by_query" } else { "_delete_by_query" };
                    format!("/{}/{}", ix, op)
                }
            };
            let req = sub_req(&job.base, path, &body, true);
            let (status, v) = {
                let mut st = lock_store(&shared);
                {
                let r = read_response(crate::dispatch(&mut st, &req));
                // background batches change state outside a request: journal them too
                crate::persist::after_request(&st, &req.method, &req.path);
                r
            }
            };
            if v.get("error").is_some() || (status >= 400 && v.get("failures").is_none()) {
                {
                    let mut reg = reg_lock();
                    if let Some(t) = reg.tasks.get_mut(&id) {
                        let mut c = delta.clone();
                        c.total = 0;
                        add_counts(&mut t.c, &c);
                    }
                }
                let st = {
                    let reg = reg_lock();
                    reg.tasks
                        .get(&id)
                        .map(|t| Value::Object(status_json(&t.c, None, None, false, t.rps, 0, false)))
                };
                return finish(id, st, error_body(&v), true);
            }
            delta.add(&v);
            if let Some(f) = v.get("failures").and_then(|f| f.as_array()) {
                failures.extend(f.iter().cloned());
            }
        }
        let mut reg = reg_lock();
        if let Some(t) = reg.tasks.get_mut(&id) {
            add_counts(&mut t.c, &delta);
            if t.rps > 0.0 {
                let wait = chunk.len() as f64 / t.rps - started.elapsed().as_secs_f64();
                if wait > 0.0 {
                    t.next_at = Some(Instant::now() + Duration::from_secs_f64(wait.min(1e9)));
                }
            }
        }
        drop(reg);
        notify();
        if !failures.is_empty() {
            break;
        }
    }
    let (status, response) = {
        let reg = reg_lock();
        let t = match reg.tasks.get(&id) {
            Some(t) => t,
            None => return,
        };
        let canceled = t.cancelled_ms.is_some();
        let rps = if canceled { -1.0 } else { t.rps };
        let took = t.start.elapsed().as_millis() as i64;
        let status = status_json(&t.c, None, None, false, rps, 0, canceled);
        let mut resp = status_json(&t.c, None, Some(took), true, rps, 0, canceled);
        resp.insert("failures".into(), Value::Array(failures));
        (status, resp)
    };
    finish(id, Some(Value::Object(status)), Value::Object(response), false);
}

fn add_counts(c: &mut Counts, d: &Counts) {
    c.updated += d.updated;
    c.created += d.created;
    c.deleted += d.deleted;
    c.version_conflicts += d.version_conflicts;
    c.noops += d.noops;
}

/// The (index, id) pairs the operation visits, in index order.
fn list_docs(
    store: &Store,
    job: &Job,
    q_param: Option<&str>,
    max_docs_param: Option<usize>,
) -> Result<Vec<(String, String)>, ApiError> {
    let (spec, query, max_docs) = match job.kind {
        Kind::Reindex => {
            let src = job.body.get("source");
            let mut spec = String::new();
            match src.and_then(|s| s.get("index")) {
                Some(Value::String(s)) => spec = s.clone(),
                Some(Value::Array(a)) => {
                    spec = a
                        .iter()
                        .filter_map(|x| x.as_str())
                        .collect::<Vec<_>>()
                        .join(",")
                }
                _ => {}
            }
            let max = job
                .body
                .get("max_docs")
                .or_else(|| job.body.get("size"))
                .and_then(|v| v.as_i64())
                .filter(|n| *n >= 0)
                .map(|n| n as usize);
            (spec, src.and_then(|s| s.get("query")).cloned(), max)
        }
        _ => {
            let q = job.body.get("query").cloned().or_else(|| {
                q_param
                    .filter(|q| !q.is_empty())
                    .map(crate::docs::parse_q_param)
            });
            let max = job
                .body
                .get("max_docs")
                .and_then(|v| v.as_i64())
                .filter(|n| *n >= 0)
                .map(|n| n as usize)
                .or(max_docs_param);
            (job.index.clone(), q, max)
        }
    };
    if job.kind == Kind::Delete && query.is_none() {
        return Err(ApiError::validation("Validation Failed: 1: query is missing;"));
    }
    let o = ResolveOpts2 {
        allow_no_indices: Some(false),
        ..Default::default()
    };
    let names = resolve_spec2(store, &spec, &o)?;
    let qm = query.as_ref().and_then(|q| q.as_object());
    let mut out = Vec::new();
    for name in &names {
        let idx = match store.indices.get(name) {
            Some(i) => i,
            None => continue,
        };
        let extra = crate::search::precompute_doc_values(idx);
        for id in &idx.order {
            if job.kind != Kind::Reindex && idx.pending.contains_key(id) {
                continue;
            }
            let doc = match idx.docs.get(id) {
                Some(d) => d,
                None => continue,
            };
            if let Some(qm) = qm {
                if !crate::search::eval_doc(idx, id, doc, &extra, qm)?.0 {
                    continue;
                }
            }
            out.push((name.clone(), id.clone()));
        }
    }
    if let Some(m) = max_docs {
        out.truncate(m);
    }
    Ok(out)
}

/// Sliced requests run in one go; per-slice batches follow the batch size.
fn run_sliced(shared: &Shared, id: u64, job: &Job) {
    let req = sub_req(&job.base, job.base.path.clone(), &job.body, false);
    let (status, v) = {
        let mut st = lock_store(shared);
        {
                let r = read_response(crate::dispatch(&mut st, &req));
                // background batches change state outside a request: journal them too
                crate::persist::after_request(&st, &req.method, &req.path);
                r
            }
    };
    if v.get("error").is_some() || (status >= 400 && v.get("failures").is_none()) {
        return finish_error_value(id, error_body(&v));
    }
    let bs = job.batch_size.max(1) as i64;
    let batches = |total: i64| if total > 0 { (total + bs - 1) / bs } else { 0 };
    let mut slices: Vec<Counts> = Vec::new();
    for s in v.get("slices").and_then(|s| s.as_array()).cloned().unwrap_or_default() {
        let mut c = Counts::default();
        c.add(&s);
        c.total = s.get("total").and_then(|x| x.as_i64()).unwrap_or(0);
        c.batches = batches(c.total);
        slices.push(c);
    }
    let mut c = Counts::default();
    c.add(&v);
    c.total = v.get("total").and_then(|x| x.as_i64()).unwrap_or(0);
    c.batches = if slices.is_empty() {
        batches(c.total)
    } else {
        slices.iter().map(|s| s.batches).sum()
    };
    let (rps, took) = {
        let mut reg = reg_lock();
        match reg.tasks.get_mut(&id) {
            Some(t) => {
                t.c = c.clone();
                (t.rps, t.start.elapsed().as_millis() as i64)
            }
            None => return,
        }
    };
    let mut status = status_json(&c, None, None, false, rps, 0, false);
    let mut resp = status_json(&c, None, Some(took), true, rps, 0, false);
    if !slices.is_empty() {
        let st: Vec<Value> = slices
            .iter()
            .enumerate()
            .map(|(i, s)| Value::Object(status_json(s, Some(i), None, false, rps, 0, false)))
            .collect();
        let rs: Vec<Value> = slices
            .iter()
            .enumerate()
            .map(|(i, s)| Value::Object(status_json(s, Some(i), None, true, rps, 0, false)))
            .collect();
        status.insert("slices".into(), Value::Array(st));
        resp.insert("slices".into(), Value::Array(rs));
    }
    resp.insert(
        "failures".into(),
        v.get("failures").cloned().unwrap_or_else(|| json!([])),
    );
    finish(id, Some(Value::Object(status)), Value::Object(resp), false);
}

fn finish_error_value(id: u64, body: Value) {
    let status = {
        let reg = reg_lock();
        reg.tasks.get(&id).map(|t| {
            let rps = if t.c.batches == 0 { 0.0 } else { t.rps };
            Value::Object(status_json(&t.c, None, None, false, rps, 0, false))
        })
    };
    finish(id, status, body, true);
}
