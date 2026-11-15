//! `_delete_by_query` / `_update_by_query` over an index expression (`_all`,
//! wildcards, lists): the request runs on every open index it resolves to and
//! the reports add up.

use serde_json::{json, Map, Value};

use axum::response::Response;

use crate::errors::{err_response, json_response};
use crate::http::docs::{delete_by_query_one, update_by_query_one};
use crate::http::shared::resolve_indices;
use crate::http::Req;
use crate::model::Store;

pub fn handle_delete_by_query(store: &mut Store, req: &Req, name: &str) -> Response {
    run(store, req, name, true)
}

pub fn handle_update_by_query(store: &mut Store, req: &Req, name: &str) -> Response {
    run(store, req, name, false)
}

fn run(store: &mut Store, req: &Req, name: &str, delete: bool) -> Response {
    let one = |store: &mut Store, name: &str| {
        if delete {
            delete_by_query_one(store, req, name)
        } else {
            update_by_query_one(store, req, name)
        }
    };
    let finish = |r: Result<(u16, Value), Response>| match r {
        Ok((status, v)) => json_response(status, v),
        Err(resp) => resp,
    };
    if name != "_all" && !name.contains([',', '*', '?']) {
        return finish(one(store, name));
    }
    let names: Vec<String> = match resolve_indices(store, name, false, false, false) {
        Ok(v) => v.iter().filter(|i| !i.closed).map(|i| i.name.clone()).collect(),
        Err(e) => return err_response(e),
    };
    if names.len() == 1 {
        return finish(one(store, &names[0]));
    }
    let mut status = 200u16;
    let mut merged: Option<Map<String, Value>> = None;
    for n in &names {
        let (st, v) = match one(store, n) {
            Ok(r) => r,
            Err(resp) => return resp,
        };
        status = status.max(st);
        let Value::Object(o) = v else { continue };
        match &mut merged {
            None => merged = Some(o),
            Some(m) => add_report(m, &o),
        }
    }
    let out = match merged {
        Some(m) => Value::Object(m),
        None => empty_report(delete),
    };
    json_response(status, out)
}

/// Adds the counters of report `b` to `a`.
fn add_report(a: &mut Map<String, Value>, b: &Map<String, Value>) {
    for (k, v) in b {
        match (k.as_str(), a.get_mut(k)) {
            ("took", Some(x)) => *x = json!(x.as_i64().unwrap_or(0).max(v.as_i64().unwrap_or(0))),
            ("timed_out" | "requests_per_second" | "throttled_until_millis", _) => {}
            ("failures", Some(Value::Array(x))) => x.extend(v.as_array().cloned().unwrap_or_default()),
            ("retries", Some(Value::Object(x))) => {
                if let Value::Object(y) = v {
                    for (rk, rv) in y {
                        let sum = x.get(rk).and_then(|n| n.as_i64()).unwrap_or(0) + rv.as_i64().unwrap_or(0);
                        x.insert(rk.clone(), json!(sum));
                    }
                }
            }
            (_, Some(x)) if x.is_i64() && v.is_i64() => *x = json!(x.as_i64().unwrap() + v.as_i64().unwrap()),
            (_, None) => {
                a.insert(k.clone(), v.clone());
            }
            _ => {}
        }
    }
}

fn empty_report(delete: bool) -> Value {
    let mut m = Map::new();
    m.insert("took".into(), json!(0));
    m.insert("timed_out".into(), json!(false));
    m.insert("total".into(), json!(0));
    if !delete {
        m.insert("updated".into(), json!(0));
    }
    m.insert("deleted".into(), json!(0));
    m.insert("batches".into(), json!(0));
    m.insert("version_conflicts".into(), json!(0));
    m.insert("noops".into(), json!(0));
    m.insert("retries".into(), json!({"bulk": 0, "search": 0}));
    m.insert("throttled_millis".into(), json!(0));
    m.insert("requests_per_second".into(), json!(-1.0));
    m.insert("throttled_until_millis".into(), json!(0));
    m.insert("failures".into(), json!([]));
    Value::Object(m)
}
