"""What opensearch-benchmark (OSB) needs from a cluster beyond the workloads' own queries.

- `_nodes/stats` has every section of OpenSearch 3.x in its order, and the JVM keys OSB's
  telemetry reads at benchmark start and stop (`jvm.gc.collectors.{young,old}`,
  `jvm.mem.pools.{young,survivor,old}`), plus what the node-stats telemetry device
  flattens (indices, thread pools, breakers, buffer pools, os/process, indexing pressure).
  Values are the server's own; only keys, types and order are checked.
- `_all` is an index expression for the index-level APIs: refresh, flush, force merge,
  cache clear, stats (`GET /_all/_stats/_all?level=shards`, OSB's index-stats device),
  segments, settings, mappings, search, count and the by-query APIs. Requests that would
  change other indices on a shared cluster run with `expand_wildcards=none` (`_all` then
  names no index) or with a query that only matches this test's documents.
- `_stats` takes `_all` as its metric, but not mixed with other metrics.
- Score scripts have OpenSearch's decay functions (ScoreScriptUtils): decayGeo*, decayDate*
  and decayNumeric* in Linear/Exp/Gauss, with distance units, time units, date math and
  geo distance (arc) as OpenSearch computes them, to float precision.
"""
import uuid

import pytest

pytestmark = pytest.mark.parity

SETTINGS = {"number_of_shards": 1, "number_of_replicas": 0}


def _node(client, path, **params):
    r = client.get(path, params=params)
    assert r.status_code == 200, r.text
    nodes = r.json()["nodes"]
    assert len(nodes) == 1
    return next(iter(nodes.values()))


BASE = ["timestamp", "name", "transport_address", "host", "ip", "roles", "attributes"]
SECTIONS = [
    "indices", "os", "process", "jvm", "thread_pool", "fs", "transport", "http", "breakers", "script",
    "discovery", "ingest", "adaptive_selection", "script_cache", "indexing_pressure",
    "shard_indexing_pressure", "search_backpressure", "cluster_manager_throttling", "weighted_routing",
    "task_cancellation", "search_pipeline", "resource_usage_stats", "segment_replication_backpressure",
    "repositories", "admission_control", "caches", "remote_store", "concurrency_limiters",
]


def _ints(obj, keys):
    assert list(obj) == keys, obj
    for k in keys:
        assert isinstance(obj[k], int) and not isinstance(obj[k], bool), (k, obj[k])


# ------------------------------------------------------------------ _nodes/stats


def test_nodes_stats_sections_in_order(client):
    stats = _node(client, "/_nodes/stats")
    assert list(stats) == BASE + SECTIONS + ["native_memory"]
    assert set(stats["native_memory"]) == {"total_estimated_bytes"}


@pytest.mark.parametrize("metric,section", [
    ("jvm", "jvm"), ("breaker", "breakers"), ("script_cache", "script_cache"),
    ("indexing_pressure", "indexing_pressure"), ("search_backpressure", "search_backpressure"),
    ("caches", "caches"), ("repositories", "repositories"), ("concurrency_limiter", "concurrency_limiters"),
])
def test_nodes_stats_one_metric_plus_native_memory(client, metric, section):
    stats = _node(client, f"/_nodes/stats/{metric}")
    assert list(stats) == BASE + [section, "native_memory"]


def test_nodes_stats_all_metric_is_everything(client):
    assert list(_node(client, "/_nodes/stats/_all")) == BASE + SECTIONS + ["native_memory"]


@pytest.mark.parametrize("path", ["/_nodes/stats", "/_nodes/stats/jvm", "/_nodes/stats/_all", "/_nodes/_local/stats/jvm,os"])
def test_nodes_stats_jvm_gc_collectors(client, path):
    jvm = _node(client, path)["jvm"]
    collectors = jvm["gc"]["collectors"]
    for name in ("young", "old"):
        _ints(collectors[name], ["collection_count", "collection_time_in_millis"])


@pytest.mark.parametrize("path", ["/_nodes/stats", "/_nodes/stats/jvm", "/_nodes/stats/_all"])
def test_nodes_stats_jvm_memory_pools(client, path):
    mem = _node(client, path)["jvm"]["mem"]
    assert list(mem) == ["heap_used_in_bytes", "heap_used_percent", "heap_committed_in_bytes", "heap_max_in_bytes",
                         "non_heap_used_in_bytes", "non_heap_committed_in_bytes", "pools"]
    assert list(mem["pools"]) == ["young", "old", "survivor"]
    for pool in mem["pools"].values():
        assert list(pool) == ["used_in_bytes", "max_in_bytes", "peak_used_in_bytes", "peak_max_in_bytes",
                              "last_gc_stats"]
        _ints({k: pool[k] for k in list(pool)[:4]}, list(pool)[:4])
        _ints(pool["last_gc_stats"], ["used_in_bytes", "max_in_bytes", "usage_percent"])
    assert mem["heap_max_in_bytes"] > 0


def test_nodes_stats_jvm_section_shape(client):
    jvm = _node(client, "/_nodes/stats/jvm")["jvm"]
    assert list(jvm) == ["timestamp", "uptime_in_millis", "mem", "threads", "gc", "buffer_pools", "classes"]
    _ints(jvm["threads"], ["count", "peak_count"])
    assert jvm["threads"]["peak_count"] >= jvm["threads"]["count"] > 0
    for pool in ("mapped", "direct", "mapped - 'non-volatile memory'"):
        _ints(jvm["buffer_pools"][pool], ["count", "used_in_bytes", "total_capacity_in_bytes"])
    _ints(jvm["classes"], ["current_loaded_count", "total_loaded_count", "total_unloaded_count"])


def test_nodes_stats_os_and_process(client):
    stats = _node(client, "/_nodes/stats/os,process")
    os_ = stats["os"]
    assert list(os_) == ["timestamp", "cpu", "mem", "swap"]
    assert list(os_["cpu"]) == ["percent", "load_average"]
    assert list(os_["cpu"]["load_average"]) == ["1m", "5m", "15m"]
    _ints(os_["mem"], ["total_in_bytes", "free_in_bytes", "used_in_bytes", "free_percent", "used_percent"])
    assert os_["mem"]["total_in_bytes"] == os_["mem"]["free_in_bytes"] + os_["mem"]["used_in_bytes"]
    _ints(os_["swap"], ["total_in_bytes", "free_in_bytes", "used_in_bytes"])
    proc = stats["process"]
    assert list(proc) == ["timestamp", "open_file_descriptors", "max_file_descriptors", "cpu", "mem"]
    _ints(proc["cpu"], ["percent", "total_in_millis"])
    _ints(proc["mem"], ["total_virtual_in_bytes"])


def test_nodes_stats_thread_pools(client):
    pools = _node(client, "/_nodes/stats/thread_pool")["thread_pool"]
    assert list(pools) == sorted(pools)
    for name in ("generic", "get", "management", "refresh", "search", "write", "flush", "force_merge"):
        keys = ["threads", "queue", "active", "rejected", "largest", "completed"]
        if name == "search":
            keys.append("total_wait_time_in_nanos")
        _ints(pools[name], keys)


def test_nodes_stats_breakers(client):
    breakers = _node(client, "/_nodes/stats/breaker")["breakers"]
    for name in ("request", "fielddata", "in_flight_requests", "parent"):
        b = breakers[name]
        assert list(b) == ["limit_size_in_bytes", "limit_size", "estimated_size_in_bytes", "estimated_size",
                           "overhead", "tripped"]
        assert isinstance(b["limit_size"], str) and isinstance(b["overhead"], float)
    assert list(breakers)[-1] == "parent"


def test_nodes_stats_indexing_pressure(client):
    ip = _node(client, "/_nodes/stats/indexing_pressure")["indexing_pressure"]
    mem = ip["memory"]
    assert list(mem) == ["current", "total", "limit_in_bytes"]
    current = ["combined_coordinating_and_primary_in_bytes", "coordinating_in_bytes", "primary_in_bytes",
               "replica_in_bytes", "all_in_bytes"]
    _ints(mem["current"], current)
    _ints(mem["total"], current + ["coordinating_rejections", "primary_rejections", "replica_rejections"])


def test_nodes_stats_indices_sections_osb_reads(client):
    indices = _node(client, "/_nodes/stats/indices")["indices"]
    assert list(indices) == ["docs", "store", "indexing", "get", "search", "merges", "refresh", "flush", "warmer",
                             "query_cache", "fielddata", "completion", "segments", "translog", "request_cache",
                             "recovery", "status_counter"]
    assert list(indices["search"]["request"]) == ["took", "dfs_pre_query", "query", "fetch", "dfs_query", "expand",
                                                  "can_match"]
    status = indices["status_counter"]
    assert list(status) == ["doc_status", "search_response_status"]
    for v in status.values():
        _ints(v, ["success", "user_error", "system_failure"])
    for path in (("segments", "count"), ("merges", "total_time_in_millis"), ("indexing", "index_time_in_millis"),
                 ("translog", "size_in_bytes"), ("store", "size_in_bytes"), ("query_cache", "memory_size_in_bytes")):
        assert isinstance(indices[path[0]][path[1]], int), path


def test_nodes_stats_indices_with_index_metric(client):
    indices = _node(client, "/_nodes/stats/indices/search,segments")["indices"]
    assert list(indices) == ["search", "segments", "status_counter"]


def test_nodes_stats_script_cache(client):
    sc = _node(client, "/_nodes/stats/script_cache")["script_cache"]
    assert list(sc) == ["sum", "contexts"]
    _ints(sc["sum"], ["compilations", "cache_evictions", "compilation_limit_triggered"])
    contexts = [c["context"] for c in sc["contexts"]]
    assert {"score", "update", "aggs", "ingest", "painless_test"} <= set(contexts)
    assert contexts == sorted(contexts)


def test_nodes_stats_index_stats_osb_reads(client, index_factory):
    """OSB's index-stats device: GET /_all/_stats/_all?level=shards."""
    name = index_factory(settings=SETTINGS)
    r = client.post(f"/{name}/_doc", params={"refresh": "true"}, json={"a": 1})
    assert r.status_code == 201, r.text
    body = client.get("/_all/_stats/_all", params={"level": "shards"}).json()
    assert body["_shards"]["total"] >= 1
    prim = body["_all"]["primaries"]
    for path in (("segments", "count"), ("segments", "memory_in_bytes"), ("segments", "doc_values_memory_in_bytes"),
                 ("segments", "stored_fields_memory_in_bytes"), ("segments", "terms_memory_in_bytes"),
                 ("segments", "norms_memory_in_bytes"), ("segments", "points_memory_in_bytes"),
                 ("merges", "total_time_in_millis"), ("merges", "total_throttled_time_in_millis"),
                 ("merges", "total"), ("indexing", "index_time_in_millis"), ("indexing", "throttle_time_in_millis"),
                 ("refresh", "total_time_in_millis"), ("refresh", "total"), ("flush", "total_time_in_millis"),
                 ("flush", "total")):
        assert isinstance(prim[path[0]][path[1]], int), path
    total = body["_all"]["total"]
    assert isinstance(total["store"]["size_in_bytes"], int)
    assert isinstance(total["translog"]["size_in_bytes"], int)
    idx = body["indices"][name]
    assert idx["primaries"]["docs"]["count"] == 1
    (shard,) = idx["shards"]["0"]
    assert shard["routing"]["primary"] is True
    assert shard["docs"]["count"] == 1
    for section in ("merges", "indexing", "refresh", "flush"):
        assert isinstance(shard[section], dict), section


def test_index_stats_common_shapes(client, index_factory):
    name = index_factory(settings=SETTINGS)
    total = client.get(f"/{name}/_stats").json()["_all"]["total"]
    assert set(total["store"]) == {"size_in_bytes", "reserved_in_bytes"}
    assert set(total["search"]) == {
        "open_contexts", "query_total", "query_time_in_millis", "query_current", "query_failed",
        "concurrent_query_total", "concurrent_query_time_in_millis", "concurrent_query_current",
        "concurrent_avg_slice_count", "startree_query_total", "startree_query_time_in_millis",
        "startree_query_current", "startree_query_failed", "fetch_total", "fetch_time_in_millis", "fetch_current",
        "scroll_total", "scroll_time_in_millis", "scroll_current", "point_in_time_total",
        "point_in_time_time_in_millis", "point_in_time_current", "suggest_total", "suggest_time_in_millis",
        "suggest_current", "search_idle_reactivate_count_total"}
    assert set(total["merges"]["warmer"]) == {
        "total_invocations_count", "total_time_millis", "total_failure_count", "total_bytes_sent",
        "total_bytes_received", "total_send_time_millis", "total_receive_time_millis", "ongoing_count"}
    assert set(total["segments"]) >= {"remote_store", "segment_replication", "file_sizes", "count"}
    assert set(total["translog"]["remote_store"]["upload"]) == {"total_uploads", "total_upload_size"}
    assert "max_last_index_request_timestamp" in total["indexing"]
    assert "getTime" in total["get"]


# ------------------------------------------------------------- _all as an index


NOTHING = {"expand_wildcards": "none"}
SHARDS_0 = {"total": 0, "successful": 0, "failed": 0}


@pytest.mark.parametrize("method,path", [
    ("POST", "/_all/_refresh"), ("GET", "/_all/_refresh"), ("POST", "/_all/_flush"), ("POST", "/_all/_forcemerge"),
    ("POST", "/_all/_cache/clear"), ("POST", "/*/_refresh"), ("POST", "/*/_flush"),
])
def test_all_maintenance_ops_accept_all(client, method, path):
    r = client.request(method, path, params=NOTHING)
    assert r.status_code == 200, r.text
    assert r.json() == {"_shards": SHARDS_0}


@pytest.mark.parametrize("path", ["/_all/_stats", "/_all/_stats/_all", "/_all/_stats/docs,store"])
def test_all_stats_with_nothing_resolved(client, path):
    r = client.get(path, params={**NOTHING, "level": "shards"})
    assert r.status_code == 200, r.text
    assert r.json() == {"_shards": SHARDS_0, "_all": {"primaries": {}, "total": {}}, "indices": {}}


@pytest.fixture
def two(client, index_factory):
    marker = uuid.uuid4().hex
    names = [index_factory(settings=SETTINGS) for _ in range(2)]
    for i, name in enumerate(names):
        for j in range(i + 1):
            r = client.put(f"/{name}/_doc/{j}", params={"refresh": "true"}, json={"marker": marker, "n": j})
            assert r.status_code == 201, r.text
    return names, marker


def test_all_stats_metric_all_lists_the_index(client, two):
    names, _ = two
    body = client.get("/_all/_stats/_all").json()
    for i, name in enumerate(names):
        assert body["indices"][name]["primaries"]["docs"]["count"] == i + 1
        assert body["indices"][name]["total"]["indexing"]["index_total"] == i + 1
    assert body["_all"]["primaries"]["docs"]["count"] >= 3


def test_all_stats_metric_filter(client, two):
    names, _ = two
    body = client.get("/_all/_stats/docs").json()
    assert set(body["indices"][names[1]]["primaries"]) == {"docs"}
    assert body["indices"][names[1]]["primaries"]["docs"]["count"] == 2


@pytest.mark.parametrize("metrics", ["_all,docs", "docs,_all", "_all,bogus"])
def test_stats_all_mixed_with_metrics_400(client, metrics):
    r = client.get(f"/_all/_stats/{metrics}")
    assert r.status_code == 400
    err = r.json()["error"]
    assert err["type"] == "illegal_argument_exception"
    assert err["reason"] == f"request [/_all/_stats/{metrics}] contains _all and individual metrics [{metrics}]"


def test_stats_star_is_not_a_metric(client):
    r = client.get("/_stats/*")
    assert r.status_code == 400
    assert r.json()["error"]["reason"] == "request [/_stats/*] contains unrecognized metric: [*]"


def test_all_segments_settings_mapping_list_the_index(client, two):
    names, _ = two
    seg = client.get("/_all/_segments")
    assert seg.status_code == 200
    assert set(names) <= set(seg.json()["indices"])
    settings = client.get("/_all/_settings/index.number_of_shards")
    assert settings.status_code == 200
    assert settings.json()[names[0]] == {"settings": {"index": {"number_of_shards": "1"}}}
    mapping = client.get("/_all/_mapping")
    assert mapping.status_code == 200
    assert mapping.json()[names[0]]["mappings"]["properties"]["marker"]["type"] == "text"
    field = client.get("/_all/_mapping/field/n")
    assert field.status_code == 200
    assert field.json()[names[1]]["mappings"]["n"]["mapping"] == {"n": {"type": "long"}}


def test_all_search_and_count(client, two):
    _, marker = two
    query = {"match": {"marker": marker}}
    r = client.post("/_all/_search", json={"query": query, "sort": [{"n": "asc"}], "_source": False})
    assert r.status_code == 200, r.text
    assert r.json()["hits"]["total"]["value"] == 3
    r = client.post("/_all/_count", json={"query": query})
    assert r.status_code == 200, r.text
    assert r.json()["count"] == 3


def test_all_delete_by_query(client, two):
    names, marker = two
    r = client.post("/_all/_delete_by_query", params={"refresh": "true"},
                    json={"query": {"bool": {"filter": [{"match": {"marker": marker}}, {"term": {"n": 0}}]}}})
    assert r.status_code == 200, r.text
    body = r.json()
    assert (body["total"], body["deleted"], body["version_conflicts"], body["failures"]) == (2, 2, 0, [])
    for i, name in enumerate(names):
        assert client.get(f"/{name}/_count").json()["count"] == i


def test_all_update_by_query(client, two):
    names, marker = two
    r = client.post("/_all/_update_by_query", params={"refresh": "true"},
                    json={"query": {"match": {"marker": marker}}, "script": {"source": "ctx._source.n += 10"}})
    assert r.status_code == 200, r.text
    body = r.json()
    assert (body["total"], body["updated"], body["failures"]) == (3, 3, [])
    assert list(body)[:5] == ["took", "timed_out", "total", "updated", "deleted"]
    r = client.post(f"/{names[1]}/_search", json={"sort": ["n"], "_source": ["n"]})
    assert [h["_source"]["n"] for h in r.json()["hits"]["hits"]] == [10, 11]


@pytest.mark.parametrize("expr", ["list", "wildcard"])
def test_delete_by_query_over_two_indices(client, two, expr):
    names, marker = two
    target = ",".join(names) if expr == "list" else "slopensearch-test-*"
    r = client.post(f"/{target}/_delete_by_query",
                    json={"query": {"match": {"marker": marker}}}, params={"refresh": "true"})
    assert r.status_code == 200, r.text
    assert (r.json()["total"], r.json()["deleted"]) == (3, 3)
    for name in names:
        assert client.get(f"/{name}/_count").json()["count"] == 0


def test_all_by_query_with_nothing_resolved(client):
    r = client.post("/_all/_delete_by_query", params=NOTHING, json={"query": {"match_all": {}}})
    assert r.status_code == 200, r.text
    body = r.json()
    assert (body["total"], body["deleted"], body["batches"], body["failures"]) == (0, 0, 0, [])


def test_all_is_not_a_document_index(client):
    r = client.put("/_all/_doc/1", json={"a": 1})
    assert r.status_code == 400
    assert r.json()["error"]["type"] == "invalid_index_name_exception"


# --------------------------------------------------------------- decay functions


DOCS = {
    "ams": {"location": "52.37,4.89", "date": "2020-09-13T12:26:40Z", "n": 0},
    "ber": {"location": {"lat": 52.52, "lon": 13.405}, "date": "2020-10-01T00:00:00Z", "n": 150},
    "par": {"location": [2.3522, 48.8566], "date": "2020-08-01", "n": -320},
    "nyc": {"location": "POINT (-74.006 40.7128)", "date": 1610697600000, "n": 1000},
    "syd": {"location": {"lat": -33.8688, "lon": 151.2093}, "date": "2019-12-31T23:59:59Z", "n": 42.5},
}

# scripts and the scores the reference OpenSearch 3.9.0 gives the documents above
DECAY = {
    # opensearch-benchmark geonames, operation decay_geo_gauss_script_score
    "geo_gauss_osb": (
        "decayGeoGauss(params.origin, params.scale, params.offset, params.decay, doc['location'].value)",
        {"origin": "52.37, 4.89", "scale": "1000km", "offset": "0km", "decay": 0.5},
        {"ams": 1.0, "par": 0.8798738, "ber": 0.7939115, "nyc": 4.5171488e-11, "syd": 0.0}),
    "geo_exp_geohash": (
        "decayGeoExp('u173zq', '500mi', '100km', 0.3, doc['location'].value)", {},
        {"ams": 1.0, "par": 0.61047727, "ber": 0.4897741, "nyc": 0.00018020664, "syd": 1.777733e-11}),
    "geo_linear_wkt": (
        "decayGeoLinear(params.o, params.s, params.f, params.d, doc['location'].value)",
        {"o": "POINT (2.35 48.85)", "s": "2000km", "f": "50km", "d": 0.2},
        {"par": 1.0, "ams": 0.84783024, "ber": 0.66880465, "nyc": 0.0, "syd": 0.0}),
    "geo_gauss_meters": (
        "decayGeoGauss('40.7,-74.0', '3000000', '0', 0.9, doc['location'].value)", {},
        {"nyc": 1.0, "par": 0.67102545, "ams": 0.6687218, "ber": 0.620421, "syd": 0.050143287}),
    "date_gauss": (
        "decayDateGauss(params.origin, params.scale, params.offset, params.decay, doc['date'].value)",
        {"origin": "2020-09-13T12:26:40Z", "scale": "30d", "offset": "1d", "decay": 0.5},
        {"ams": 1.0, "ber": 0.8112276, "par": 0.24849801, "nyc": 9.0138765e-06, "syd": 1.4522457e-22}),
    "date_exp_epoch": (
        "decayDateExp('1600000000000', '100h', '0ms', 0.2, doc['date'].value)", {},
        {"ams": 1.0, "ber": 0.0011679722, "par": 5.0077958e-08, "nyc": 1.6971414e-21, "syd": 9.2e-44}),
    "date_linear": (
        "decayDateLinear('2020-09-01', '90d', '12h', 0.5, doc['date'].value)", {},
        {"ams": 0.93323046, "ber": 0.8361111, "par": 0.83055556, "nyc": 0.24537037, "syd": 0.0}),
    "date_gauss_math": (
        "decayDateGauss('2020-09-13||+1M/d', '20d', '0s', 0.5, doc['date'].value)", {},
        {"ber": 0.77916455, "ams": 0.22176531, "par": 9.761754e-05, "nyc": 2.0091039e-07, "syd": 0.0}),
    "numeric_gauss": (
        "decayNumericGauss(10, 300, 5, 0.4, doc['n'].value)", {},
        {"ams": 0.9997455, "syd": 0.9923302, "ber": 0.83064824, "par": 0.34117183, "nyc": 5.1297495e-05}),
    "numeric_exp": (
        "decayNumericExp(params.o, params.s, 0, 0.5, doc['n'].value)", {"o": 100, "s": 250},
        {"ber": 0.8705506, "syd": 0.8526349, "ams": 0.7578583, "par": 0.31208265, "nyc": 0.08246925}),
    "numeric_linear": (
        "decayNumericLinear(0, 500, 10, 0.25, doc['n'].value)", {},
        {"ams": 1.0, "syd": 0.95125, "ber": 0.79, "par": 0.535, "nyc": 0.0}),
    "combined": (
        "_score * 2 * decayGeoGauss('52.37, 4.89', '1000km', '0km', 0.5, doc['location'].value)"
        " + decayDateExp('2020-09-13T12:26:40Z', '30d', '0d', 0.5, doc['date'].value)", {},
        {"ams": 3.0, "ber": 2.2555285, "par": 2.1256125, "nyc": 0.05722699, "syd": 0.0026669053}),
}


@pytest.fixture(scope="module")
def places(client):
    name = f"slopensearch-test-decay-{uuid.uuid4().hex[:8]}"
    r = client.put(f"/{name}", json={"settings": SETTINGS, "mappings": {"properties": {
        "location": {"type": "geo_point"}, "date": {"type": "date"}, "n": {"type": "double"}}}})
    assert r.status_code == 200, r.text
    for doc_id, doc in DOCS.items():
        r = client.put(f"/{name}/_doc/{doc_id}", json=doc)
        assert r.status_code == 201, r.text
    client.post(f"/{name}/_refresh")
    yield name
    client.delete(f"/{name}")


@pytest.mark.parametrize("variant", list(DECAY))
def test_decay_function_scores(client, places, variant):
    source, params, expected = DECAY[variant]
    r = client.post(f"/{places}/_search", json={"size": 10, "_source": False, "query": {"script_score": {
        "query": {"match_all": {}}, "script": {"source": source, "params": params}}}})
    assert r.status_code == 200, r.text
    hits = r.json()["hits"]["hits"]
    assert {h["_id"]: h["_score"] for h in hits} == expected
    scores = [h["_score"] for h in hits]
    assert scores == sorted(scores, reverse=True)


def test_decay_in_function_score_script(client, places):
    source, params, expected = DECAY["geo_gauss_osb"]
    r = client.post(f"/{places}/_search", json={"size": 10, "_source": False, "query": {"function_score": {
        "query": {"match_all": {}}, "functions": [{"script_score": {"script": {"source": source, "params": params}}}],
        "boost_mode": "replace"}}})
    assert r.status_code == 200, r.text
    assert {h["_id"]: h["_score"] for h in r.json()["hits"]["hits"]} == expected


def test_decay_geo_on_a_date_is_a_class_cast(client, places):
    r = client.post(f"/{places}/_search", json={"query": {"script_score": {"query": {"match_all": {}}, "script": {
        "source": "decayGeoGauss('52.37, 4.89', '1km', '0km', 0.5, doc['date'].value)"}}}})
    assert r.status_code == 400
    reason = r.json()["error"]["failed_shards"][0]["reason"]
    assert reason["type"] == "script_exception"
    assert reason["caused_by"] == {"type": "class_cast_exception",
                                   "reason": "Cannot cast java.time.ZonedDateTime to org.opensearch.common.geo.GeoPoint"}
