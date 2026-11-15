//! The sections of GET _nodes/stats, in the order and shape OpenSearch 3.9
//! writes them. Process, OS and memory figures are this process's own (from
//! /proc); the JVM-only parts (memory pools, collectors, buffer pools,
//! classes) are mapped onto the process or reported empty (there is no
//! garbage collector), so that monitoring tools that read them
//! (opensearch-benchmark's telemetry, ...) find every key they expect.

use std::collections::HashMap;
use std::sync::Mutex;
use std::sync::OnceLock;
use std::time::Instant;

use serde_json::{json, Map, Value};

use crate::http::cluster::{node_attributes, node_roles, NODE_HOST};
use crate::http::maintenance::{empty_metric, index_metrics, merge_metrics, STATS_METRIC_NAMES};
use crate::http::shared::resolve_indices;
use crate::model::{now_millis, Store, NODE_ID, NODE_NAME};

const NODE_TRANSPORT: &str = "127.0.0.1:9300";

/// The metrics of _nodes/stats in response order; the section of "breaker" is
/// "breakers" and of "concurrency_limiter" "concurrency_limiters".
/// native_memory is always written.
pub(crate) const NODE_STATS_SECTIONS: &[&str] = &[
    "indices",
    "os",
    "process",
    "jvm",
    "thread_pool",
    "fs",
    "transport",
    "http",
    "breaker",
    "script",
    "discovery",
    "ingest",
    "adaptive_selection",
    "script_cache",
    "indexing_pressure",
    "shard_indexing_pressure",
    "search_backpressure",
    "cluster_manager_throttling",
    "weighted_routing",
    "file_cache",
    "task_cancellation",
    "search_pipeline",
    "resource_usage_stats",
    "segment_replication_backpressure",
    "repositories",
    "admission_control",
    "caches",
    "remote_store",
    "concurrency_limiter",
];

fn process_start() -> Instant {
    static START: OnceLock<Instant> = OnceLock::new();
    *START.get_or_init(Instant::now)
}

/// Milliseconds since the process started (from /proc, else since the first call).
fn uptime_millis() -> i64 {
    let since_boot = std::fs::read_to_string("/proc/uptime")
        .ok()
        .and_then(|s| s.split_whitespace().next().and_then(|x| x.parse::<f64>().ok()));
    let started = std::fs::read_to_string("/proc/self/stat").ok().and_then(|s| {
        let rest = s.rsplit_once(')').map(|(_, r)| r.to_string())?;
        rest.split_whitespace().nth(19).and_then(|x| x.parse::<f64>().ok())
    });
    match (since_boot, started) {
        (Some(up), Some(ticks)) if up * 100.0 >= ticks => ((up - ticks / 100.0) * 1000.0) as i64,
        _ => process_start().elapsed().as_millis() as i64,
    }
}

#[derive(Default)]
struct Peaks {
    threads: u64,
    heap: u64,
}

fn peaks() -> &'static Mutex<Peaks> {
    static P: OnceLock<Mutex<Peaks>> = OnceLock::new();
    P.get_or_init(|| Mutex::new(Peaks::default()))
}

/// "Key: value" lines of a /proc file (kB values in bytes).
fn proc_fields(path: &str) -> HashMap<String, u64> {
    let mut out = HashMap::new();
    let Ok(text) = std::fs::read_to_string(path) else { return out };
    for line in text.lines() {
        let Some((k, v)) = line.split_once(':') else { continue };
        let mut f = v.split_whitespace();
        let Some(Ok(mut n)) = f.next().map(|x| x.parse::<u64>()) else { continue };
        if f.next() == Some("kB") {
            n *= 1024;
        }
        out.insert(k.trim().to_string(), n);
    }
    out
}

fn os_memory() -> (u64, u64, u64, u64) {
    let m = proc_fields("/proc/meminfo");
    let total = m.get("MemTotal").copied().unwrap_or(0);
    let free = m.get("MemAvailable").or(m.get("MemFree")).copied().unwrap_or(0);
    if total == 0 {
        return (1 << 34, 1 << 33, 0, 0);
    }
    (total, free, m.get("SwapTotal").copied().unwrap_or(0), m.get("SwapFree").copied().unwrap_or(0))
}

fn pct(part: u64, whole: u64) -> i64 {
    if whole == 0 {
        0
    } else {
        (part as f64 * 100.0 / whole as f64).round() as i64
    }
}

/// ByteSizeValue's string: "614.3mb", "1gb".
fn human_size(n: i64) -> String {
    let units = [("pb", 1i64 << 50), ("tb", 1 << 40), ("gb", 1 << 30), ("mb", 1 << 20), ("kb", 1 << 10)];
    for (u, f) in units {
        if n >= f {
            let v = (n as f64 / f as f64 * 10.0).round() / 10.0;
            let s = format!("{:.1}", v);
            return format!("{}{}", s.strip_suffix(".0").unwrap_or(&s), u);
        }
    }
    format!("{}b", n)
}

fn os_stats() -> Value {
    let (total, free, swap_total, swap_free) = os_memory();
    let mut load = [0.0f64; 3];
    if let Ok(s) = std::fs::read_to_string("/proc/loadavg") {
        for (i, f) in s.split_whitespace().take(3).enumerate() {
            load[i] = f.parse().unwrap_or(0.0);
        }
    }
    json!({
        "timestamp": now_millis(),
        "cpu": {"percent": 0, "load_average": {"1m": load[0], "5m": load[1], "15m": load[2]}},
        "mem": {"total_in_bytes": total, "free_in_bytes": free, "used_in_bytes": total - free,
            "free_percent": pct(free, total), "used_percent": pct(total - free, total)},
        "swap": {"total_in_bytes": swap_total, "free_in_bytes": swap_free, "used_in_bytes": swap_total - swap_free},
    })
}

fn process_cpu_millis() -> i64 {
    let Ok(s) = std::fs::read_to_string("/proc/self/stat") else { return 0 };
    let rest = s.rsplit_once(')').map(|(_, r)| r).unwrap_or(&s);
    let f: Vec<&str> = rest.split_whitespace().collect();
    if f.len() < 13 {
        return 0;
    }
    let ut: i64 = f[11].parse().unwrap_or(0);
    let st: i64 = f[12].parse().unwrap_or(0);
    (ut + st) * 10
}

fn max_open_files() -> i64 {
    if let Ok(s) = std::fs::read_to_string("/proc/self/limits") {
        for line in s.lines() {
            if let Some(rest) = line.strip_prefix("Max open files") {
                if let Some(Ok(n)) = rest.split_whitespace().next().map(|x| x.parse::<i64>()) {
                    return n;
                }
            }
        }
    }
    65535
}

fn process_stats() -> Value {
    let fds = std::fs::read_dir("/proc/self/fd").map(|d| d.count()).unwrap_or(0);
    json!({
        "timestamp": now_millis(),
        "open_file_descriptors": fds,
        "max_file_descriptors": max_open_files(),
        "cpu": {"percent": 0, "total_in_millis": process_cpu_millis()},
        "mem": {"total_virtual_in_bytes": proc_fields("/proc/self/status").get("VmSize").copied().unwrap_or(0)},
    })
}

fn heap_max() -> u64 {
    os_memory().0
}

fn mem_pool(used: u64, max: u64, peak: u64, peak_max: u64, gc_used: u64, gc_max: u64) -> Value {
    let usage = if gc_max > 0 { pct(gc_used, gc_max) } else { -1 };
    json!({
        "used_in_bytes": used, "max_in_bytes": max, "peak_used_in_bytes": peak, "peak_max_in_bytes": peak_max,
        "last_gc_stats": {"used_in_bytes": gc_used, "max_in_bytes": gc_max, "usage_percent": usage},
    })
}

fn buffer_pool() -> Value {
    json!({"count": 0, "used_in_bytes": 0, "total_capacity_in_bytes": 0})
}

/// The JVM section over the process: its resident memory is the heap (all of
/// it "old"; there is no garbage collector, so no young objects and no
/// collections).
fn jvm_stats() -> Value {
    let status = proc_fields("/proc/self/status");
    let used = status.get("VmRSS").copied().unwrap_or(0);
    let threads = status.get("Threads").copied().unwrap_or(1);
    let max = heap_max();
    let (peak_threads, peak_heap) = {
        let mut p = peaks().lock().unwrap_or_else(|e| e.into_inner());
        p.threads = p.threads.max(threads);
        p.heap = p.heap.max(used).max(status.get("VmHWM").copied().unwrap_or(0));
        (p.threads, p.heap)
    };
    json!({
        "timestamp": now_millis(),
        "uptime_in_millis": uptime_millis(),
        "mem": {
            "heap_used_in_bytes": used,
            "heap_used_percent": pct(used, max),
            "heap_committed_in_bytes": used,
            "heap_max_in_bytes": max,
            "non_heap_used_in_bytes": 0,
            "non_heap_committed_in_bytes": 0,
            "pools": {
                "young": mem_pool(0, 0, 0, 0, 0, 0),
                "old": mem_pool(used, max, peak_heap, max, used, max),
                "survivor": mem_pool(0, 0, 0, 0, 0, 0),
            },
        },
        "threads": {"count": threads, "peak_count": peak_threads},
        "gc": {"collectors": {
            "young": {"collection_count": 0, "collection_time_in_millis": 0},
            "old": {"collection_count": 0, "collection_time_in_millis": 0},
        }},
        "buffer_pools": {
            "mapped": buffer_pool(),
            "direct": buffer_pool(),
            "mapped - 'non-volatile memory'": buffer_pool(),
        },
        "classes": {"current_loaded_count": 0, "total_loaded_count": 0, "total_unloaded_count": 0},
    })
}

/// OpenSearch's core thread pools (name order).
fn thread_pool_stats() -> Value {
    let procs = std::thread::available_parallelism().map(|n| n.get()).unwrap_or(1) as i64;
    let size = |n: &str| -> i64 {
        match n {
            "generic" => 4,
            "management" | "refresh" | "system_read" | "system_write" => 1,
            "search" => procs * 3 / 2 + 1,
            "index_searcher" | "write" => procs,
            _ => 0,
        }
    };
    let names = [
        "analyze", "fetch_shard_started", "fetch_shard_store", "flush", "force_merge", "generic", "get",
        "index_searcher", "listener", "management", "merge", "refresh", "remote_download", "remote_purge",
        "remote_recovery", "remote_refresh_retry", "remote_state_checksum", "remote_state_read", "search",
        "search_throttled", "snapshot", "snapshot_deletion", "stream_search", "system_read", "system_write",
        "translog_recovery", "translog_sync", "translog_transfer", "warmer", "write",
    ];
    let mut out = Map::new();
    for n in names {
        let t = size(n);
        let mut p = json!({"threads": t, "queue": 0, "active": 0, "rejected": 0, "largest": t, "completed": 0});
        if matches!(n, "index_searcher" | "search" | "search_throttled" | "stream_search") {
            p["total_wait_time_in_nanos"] = json!(0);
        }
        out.insert(n.to_string(), p);
    }
    Value::Object(out)
}

fn fs_stats() -> Value {
    let dir = std::env::var("SLOP_DATA_DIR").unwrap_or_else(|_| "data".to_string());
    json!({
        "timestamp": now_millis(),
        "total": {"total_in_bytes": 0, "free_in_bytes": 0, "available_in_bytes": 0, "cache_reserved_in_bytes": 0},
        "data": [{"path": dir, "mount": "/", "type": "ext4", "total_in_bytes": 0, "free_in_bytes": 0,
            "available_in_bytes": 0, "cache_reserved_in_bytes": 0}],
        "io_stats": {},
    })
}

fn breaker_stats() -> Value {
    let max = heap_max() as i64;
    let b = |limit: i64, overhead: f64| {
        json!({"limit_size_in_bytes": limit, "limit_size": human_size(limit), "estimated_size_in_bytes": 0,
            "estimated_size": "0b", "overhead": overhead, "tripped": 0})
    };
    json!({
        "request": b(max * 6 / 10, 1.0),
        "fielddata": b(max * 4 / 10, 1.03),
        "in_flight_requests": b(max, 2.0),
        "parent": b(max * 95 / 100, 1.0),
    })
}

fn script_stats() -> Value {
    json!({"compilations": 0, "cache_evictions": 0, "compilation_limit_triggered": 0})
}

const SCRIPT_CONTEXTS: &[&str] = &[
    "aggregation_selector", "aggs", "aggs_combine", "aggs_init", "aggs_map", "aggs_reduce", "analysis",
    "bucket_aggregation", "field", "filter", "ingest", "interval", "moving-function", "number_sort",
    "painless_test", "processor_conditional", "score", "script_heuristic", "similarity", "similarity_weight",
    "string_sort", "template", "terms_set", "trigger", "update",
];

fn pipeline_totals() -> Value {
    json!({"count": 0, "time_in_millis": 0, "current": 0, "failed": 0})
}

fn backpressure_task() -> Value {
    let tracker = |unit: &str| {
        let mut m = Map::new();
        m.insert("cancellation_count".into(), json!(0));
        m.insert(format!("current_max_{}", unit), json!(0));
        m.insert(format!("current_avg_{}", unit), json!(0));
        m
    };
    let mut heap = tracker("bytes");
    heap.insert("rolling_avg_bytes".into(), json!(0));
    json!({
        "resource_tracker_stats": {"heap_usage_tracker": heap, "elapsed_time_tracker": tracker("millis"),
            "cpu_usage_tracker": tracker("millis")},
        "completion_count": 0,
        "cancellation_stats": {"cancellation_count": 0, "cancellation_limit_reached_count": 0},
    })
}

fn rejection_counts() -> Value {
    json!({"transport": {"rejection_count": {}}})
}

fn section(store: &Store, name: &str, index_metric: &str, level: &str) -> Option<Value> {
    Some(match name {
        "indices" => indices_stats(store, index_metric, level),
        "os" => os_stats(),
        "process" => process_stats(),
        "jvm" => jvm_stats(),
        "thread_pool" => thread_pool_stats(),
        "fs" => fs_stats(),
        "transport" => json!({"server_open": 0, "total_outbound_connections": 0, "rx_count": 0,
            "rx_size_in_bytes": 0, "tx_count": 0, "tx_size_in_bytes": 0}),
        "http" => json!({"current_open": 1, "total_opened": 1}),
        "breaker" => breaker_stats(),
        "script" => script_stats(),
        "discovery" => json!({
            "cluster_state_queue": {"total": 0, "pending": 0, "committed": 0},
            "published_cluster_states": {"full_states": 0, "incompatible_diffs": 0, "compatible_diffs": 0},
            "cluster_state_stats": {"overall": {"update_count": 0, "total_time_in_millis": 0, "failed_count": 0}},
        }),
        "ingest" => json!({"total": pipeline_totals(), "pipelines": {}}),
        "adaptive_selection" => json!({}),
        "script_cache" => {
            let ctxs: Vec<Value> = SCRIPT_CONTEXTS
                .iter()
                .map(|c| json!({"context": c, "compilations": 0, "cache_evictions": 0, "compilation_limit_triggered": 0}))
                .collect();
            json!({"sum": script_stats(), "contexts": ctxs})
        }
        "indexing_pressure" => json!({"memory": {
            "current": {"combined_coordinating_and_primary_in_bytes": 0, "coordinating_in_bytes": 0,
                "primary_in_bytes": 0, "replica_in_bytes": 0, "all_in_bytes": 0},
            "total": {"combined_coordinating_and_primary_in_bytes": 0, "coordinating_in_bytes": 0,
                "primary_in_bytes": 0, "replica_in_bytes": 0, "all_in_bytes": 0,
                "coordinating_rejections": 0, "primary_rejections": 0, "replica_rejections": 0},
            "limit_in_bytes": heap_max() / 10,
        }}),
        "shard_indexing_pressure" => json!({"stats": {},
            "total_rejections_breakup_shadow_mode": {"node_limits": 0, "no_successful_request_limits": 0,
                "throughput_degradation_limits": 0},
            "enabled": false, "enforced": false}),
        "search_backpressure" => json!({"search_task": backpressure_task(),
            "search_shard_task": backpressure_task(), "mode": "monitor_only"}),
        "cluster_manager_throttling" => json!({"stats": {"total_throttled_tasks": 0, "throttled_tasks_per_task_type": {}}}),
        "weighted_routing" => json!({"stats": {"fail_open_count": 0}}),
        "task_cancellation" => json!({
            "search_task": {"current_count_post_cancel": 0, "total_count_post_cancel": 0},
            "search_shard_task": {"current_count_post_cancel": 0, "total_count_post_cancel": 0},
        }),
        "search_pipeline" => json!({
            "total_request": pipeline_totals(), "total_response": pipeline_totals(), "pipelines": {},
            "system_generated_processors": {"request_processors": [], "response_processors": []},
            "system_generated_factories": {"request_processor_factories": [], "response_processor_factories": []},
        }),
        "resource_usage_stats" => {
            let (total, free, _, _) = os_memory();
            json!({NODE_ID: {"timestamp": now_millis(), "cpu_utilization_percent": "0.0",
                "memory_utilization_percent": format!("{:.1}", (total - free) as f64 * 100.0 / total as f64),
                "native_memory_utilization_percent": "0.0",
                "io_usage_stats": {"max_io_utilization_percent": "0.0"}}})
        }
        "segment_replication_backpressure" => json!({"total_rejected_requests": 0}),
        "repositories" => json!([]),
        "admission_control" => json!({"global_io_usage": rejection_counts(), "global_cpu_usage": rejection_counts(),
            "global_native_memory_usage": rejection_counts()}),
        "caches" => json!({"request_cache": {"size_in_bytes": 0, "evictions": 0, "hit_count": 0, "miss_count": 0,
            "item_count": 0, "store_name": "opensearch_onheap"}}),
        "remote_store" => json!({"last_successful_fetch_of_pinned_timestamps": -1}),
        "concurrency_limiter" => json!({}),
        _ => return None,
    })
}

pub(crate) fn build_node_stats(store: &Store, metrics: &[String], index_metric: &str, level: &str) -> Map<String, Value> {
    let mut stats = Map::new();
    stats.insert("timestamp".into(), json!(now_millis()));
    stats.insert("name".into(), json!(NODE_NAME));
    stats.insert("transport_address".into(), json!(NODE_TRANSPORT));
    stats.insert("host".into(), json!(NODE_HOST));
    stats.insert("ip".into(), json!(NODE_HOST));
    stats.insert("roles".into(), node_roles());
    stats.insert("attributes".into(), node_attributes());
    let want = |m: &str| metrics.iter().any(|x| x == m || (m == "breaker" && x == "breakers"));
    for name in NODE_STATS_SECTIONS {
        if !want(name) {
            continue;
        }
        if let Some(v) = section(store, name, index_metric, level) {
            let key = match *name {
                "breaker" => "breakers",
                "concurrency_limiter" => "concurrency_limiters",
                n => n,
            };
            stats.insert(key.to_string(), v);
        }
    }
    let rss = proc_fields("/proc/self/status").get("VmRSS").copied().unwrap_or(0);
    stats.insert("native_memory".into(), json!({"total_estimated_bytes": rss}));
    stats
}

/// The indices section: the index stats of the node's open indices added up,
/// with search request phases and status counters.
fn indices_stats(store: &Store, index_metric: &str, level: &str) -> Value {
    let requested: Option<Vec<String>> = if !index_metric.is_empty() && !index_metric.contains("_all") {
        Some(index_metric.split(',').map(|m| if m == "merge" { "merges".to_string() } else { m.to_string() }).collect())
    } else {
        None
    };
    let mut total = Map::new();
    let mut per_index = Map::new();
    if let Ok(indices) = resolve_indices(store, "*", false, true, false) {
        for idx in indices {
            if idx.closed {
                continue;
            }
            let m = index_metrics(idx, requested.as_ref());
            if let Some(o) = m.as_object() {
                merge_metrics(&mut total, o);
            }
            per_index.insert(idx.name.clone(), m);
        }
    }
    // in metric order, also without indices
    let mut out = Map::new();
    for name in STATS_METRIC_NAMES {
        if requested.as_ref().is_some_and(|r| !r.iter().any(|x| x == name)) {
            continue;
        }
        let mut v = total.remove(*name).unwrap_or_else(|| empty_metric(name));
        if *name == "search" {
            let phase = || json!({"time_in_millis": 0, "current": 0, "total": 0});
            v["request"] = json!({"took": phase(), "dfs_pre_query": phase(), "query": phase(), "fetch": phase(),
                "dfs_query": phase(), "expand": phase(), "can_match": phase()});
        }
        out.insert(name.to_string(), v);
    }
    let status = || json!({"success": 0, "user_error": 0, "system_failure": 0});
    out.insert("status_counter".into(), json!({"doc_status": status(), "search_response_status": status()}));
    if level == "indices" {
        out.insert("indices".into(), Value::Object(per_index));
    }
    Value::Object(out)
}
