// The k-NN plugin's REST endpoints under /_plugins/_knn.

use axum::response::Response;
use serde_json::{json, Map, Value};

use crate::errors::{err_response, json_response, ApiError};
use crate::shared::{parse_resolve_params, resolve_spec2, ResolveOpts2};
use crate::store::{Store, CLUSTER_NAME, NODE_ID};
use crate::Req;

const CLUSTER_STATS: [&str; 2] = ["circuit_breaker_triggered", "model_index_status"];

const NODE_STATS: [&str; 30] = [
    "hit_count", "miss_count", "graph_memory_usage", "graph_memory_usage_percentage",
    "graph_index_requests", "graph_index_errors", "graph_query_requests", "graph_query_errors",
    "knn_query_requests", "cache_capacity_reached", "load_success_count", "load_exception_count",
    "indices_in_cache", "script_compilations", "script_compilation_errors", "script_query_requests",
    "script_query_errors", "nmslib_initialized", "faiss_initialized", "lucene_initialized",
    "indexing_from_model_degraded", "training_requests", "training_errors", "training_memory_usage",
    "training_memory_usage_percentage", "eviction_count", "total_load_time",
    "knn_query_with_filter_requests", "min_score_query_requests", "max_distance_query_requests",
];

fn node_stat(name: &str) -> Value {
    match name {
        "graph_memory_usage_percentage" | "training_memory_usage_percentage" => json!(0.0),
        "indices_in_cache" => json!({}),
        "nmslib_initialized" | "faiss_initialized" | "lucene_initialized" | "indexing_from_model_degraded"
        | "cache_capacity_reached" => json!(false),
        _ => json!(0),
    }
}

pub fn handle(store: &mut Store, req: &Req, segs: &[String]) -> Response {
    match segs.first().map(|s| s.as_str()) {
        Some("stats") => stats(req, segs.get(1).map(|s| s.as_str()).unwrap_or("")),
        Some("warmup") if segs.len() >= 2 => warmup(store, req, &segs[1]),
        _ => err_response(ApiError::bad(&format!(
            "no handler found for uri [{}] and method [{}]",
            req.path, req.method
        ))),
    }
}

fn stats(req: &Req, filter: &str) -> Response {
    let wanted: Vec<&str> = filter.split(',').filter(|s| !s.is_empty()).collect();
    for w in &wanted {
        if !CLUSTER_STATS.contains(w) && !NODE_STATS.contains(w) {
            return err_response(ApiError::bad(&format!(
                "request [{}] contains unrecognized stat: [{}]",
                req.path, w
            )));
        }
    }
    let want = |s: &str| wanted.is_empty() || wanted.contains(&s);
    let mut out = Map::new();
    out.insert("_nodes".into(), json!({"total": 1, "successful": 1, "failed": 0}));
    out.insert("cluster_name".into(), json!(CLUSTER_NAME));
    for s in CLUSTER_STATS {
        if want(s) {
            let v = if s == "circuit_breaker_triggered" { json!(false) } else { Value::Null };
            out.insert(s.into(), v);
        }
    }
    let mut node = Map::new();
    for s in NODE_STATS {
        if want(s) {
            node.insert(s.into(), node_stat(s));
        }
    }
    out.insert("nodes".into(), json!({ NODE_ID: node }));
    json_response(200, Value::Object(out))
}

fn warmup(store: &mut Store, req: &Req, spec: &str) -> Response {
    let o = parse_resolve_params(req, ResolveOpts2::default());
    let names = match resolve_spec2(store, spec, &o) {
        Ok(n) => n,
        Err(e) => return err_response(e),
    };
    let mut shards = 0;
    for n in &names {
        let idx = &store.indices[n];
        if !crate::knn_index::index_knn(idx) {
            return err_response(ApiError::new(
                "k_n_n_invalid_indices_exception",
                "Warm up request rejected. One or more indices have 'index.knn' set to false.",
                500,
            ));
        }
        shards += idx.num_shards();
    }
    json_response(200, json!({"_shards": {"total": shards, "successful": shards, "failed": 0}}))
}
