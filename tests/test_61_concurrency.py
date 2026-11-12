"""Concurrent requests: results that are deterministic even though requests race.

Every other file sends one request at a time, so locking bugs (deadlocks, lost updates,
torn reads) go unnoticed there. Here many threads hit the server at once. The worker
threads use their own httpx clients (not recorded); the final checks go through the
`client` fixture. Every request has a bounded timeout, so a hang fails instead of
blocking the run.
"""

import json
import os
import random
import threading
import time
import uuid
from concurrent.futures import ThreadPoolExecutor

import httpx
import pytest

pytestmark = pytest.mark.concurrency

NDJSON = {"Content-Type": "application/x-ndjson"}
SINGLE = {"number_of_shards": 1, "number_of_replicas": 0}
REQUEST_TIMEOUT = float(os.getenv("OS_TIMEOUT", "30"))
# Upper bound for one whole concurrent phase of a test.
PHASE_TIMEOUT = 90


# --------------------------------------------------------------------------- helpers


def _uniq(prefix="slopensearch-conc"):
    return f"{prefix}-{uuid.uuid4().hex[:10]}"


def _ndjson(lines):
    return "".join(json.dumps(ln, ensure_ascii=False) + "\n" for ln in lines).encode()


def _bulk(c, lines, index=None, **params):
    path = f"/{index}/_bulk" if index else "/_bulk"
    return c.post(path, content=_ndjson(lines), headers=NDJSON, params=params)


def _refresh(c, index):
    r = c.post(f"/{index}/_refresh")
    assert r.status_code == 200, r.text


def _count(c, index, query=None):
    r = c.post(f"/{index}/_count", json={"query": query} if query else {})
    assert r.status_code == 200, r.text
    return r.json()["count"]


def _err_type(r):
    return r.json()["error"]["type"]


class _Workers:
    """Runs callables on a thread pool; each thread gets its own httpx client."""

    def __init__(self, base_url):
        self.base_url = base_url
        self.local = threading.local()
        self.lock = threading.Lock()
        self.clients = []

    def _client(self):
        c = getattr(self.local, "c", None)
        if c is None:
            c = httpx.Client(base_url=self.base_url, timeout=REQUEST_TIMEOUT)
            self.local.c = c
            with self.lock:
                self.clients.append(c)
        return c

    def run(self, tasks, workers=8):
        """Run zero-argument-plus-client callables concurrently; return their results in order."""
        ex = ThreadPoolExecutor(max_workers=workers)
        deadline = time.monotonic() + PHASE_TIMEOUT
        try:
            futures = [ex.submit(lambda t=t: t(self._client())) for t in tasks]
            return [f.result(timeout=max(0.1, deadline - time.monotonic())) for f in futures]
        finally:
            ex.shutdown(wait=False, cancel_futures=True)

    def map(self, fn, items, workers=8):
        return self.run([lambda c, i=i: fn(c, i) for i in items], workers=workers)

    def close(self):
        for c in self.clients:
            c.close()


@pytest.fixture
def pool(base_url):
    w = _Workers(base_url)
    yield w
    w.close()


@pytest.fixture
def idx(index_factory):
    return index_factory(
        settings=SINGLE,
        mappings={"properties": {"tag": {"type": "keyword"}, "n": {"type": "long"}, "title": {"type": "text"}}},
    )


@pytest.fixture
def scratch(client):
    """Index names created by the test itself; deleted afterwards."""
    names = []
    yield names
    for n in names:
        client.delete(f"/{n}", params={"ignore_unavailable": "true"})


def _seed(client, index, n, tag_of=lambda i: "even" if i % 2 == 0 else "odd"):
    lines = []
    for i in range(n):
        lines.append({"index": {"_id": str(i)}})
        lines.append({"tag": tag_of(i), "n": i, "title": f"doc number {i}"})
    r = _bulk(client, lines, index=index, refresh="true")
    assert r.status_code == 200 and r.json()["errors"] is False, r.text


def _readers_until(pool, writer, reader, readers=4):
    """Run `writer(c)` once and `reader(c)` in a loop on other threads until the writer ends.

    Returns (writer result, list of every reader result)."""
    done = threading.Event()
    seen = [[] for _ in range(readers)]

    def write(c):
        try:
            return writer(c)
        finally:
            done.set()

    def read(k):
        def _r(c):
            while True:
                last = done.is_set()
                seen[k].append(reader(c))
                if last:
                    return None
        return _r

    results = pool.run([write] + [read(k) for k in range(readers)], workers=readers + 1)
    return results[0], seen


# =========================================================================== single-doc writes


def test_parallel_index_distinct_ids(client, pool, idx):
    rs = pool.map(lambda c, i: c.put(f"/{idx}/_doc/{i}", json={"n": i}), range(200), workers=16)
    assert all(r.status_code == 201 for r in rs), [r.text for r in rs if r.status_code != 201][:3]
    bodies = [r.json() for r in rs]
    assert all(b["_version"] == 1 and b["result"] == "created" for b in bodies)
    assert sorted(b["_seq_no"] for b in bodies) == list(range(200))
    assert {b["_primary_term"] for b in bodies} == {1}
    _refresh(client, idx)
    assert _count(client, idx) == 200


def test_parallel_index_auto_ids(client, pool, idx):
    rs = pool.map(lambda c, i: c.post(f"/{idx}/_doc", json={"n": i}), range(150), workers=16)
    assert all(r.status_code == 201 for r in rs)
    ids = [r.json()["_id"] for r in rs]
    assert len(set(ids)) == 150
    assert sorted(r.json()["_seq_no"] for r in rs) == list(range(150))
    _refresh(client, idx)
    assert _count(client, idx) == 150
    r = client.post(f"/{idx}/_search", json={"size": 0, "aggs": {"s": {"sum": {"field": "n"}}}})
    assert r.json()["aggregations"]["s"]["value"] == sum(range(150))


def test_parallel_index_refresh_wait_for_is_visible(pool, idx):
    def put_then_search(c, i):
        r = c.put(f"/{idx}/_doc/{i}", json={"n": i}, params={"refresh": "wait_for"})
        assert r.status_code == 201, r.text
        s = c.post(f"/{idx}/_search", json={"query": {"term": {"n": i}}})
        return s.json()["hits"]["total"]["value"]

    assert pool.map(put_then_search, range(40), workers=8) == [1] * 40


def test_parallel_index_refresh_true_is_visible(pool, idx):
    def put_then_count(c, i):
        r = c.put(f"/{idx}/_doc/{i}", json={"tag": f"t{i}"}, params={"refresh": "true"})
        assert r.status_code == 201, r.text
        return c.post(f"/{idx}/_count", json={"query": {"term": {"tag": f"t{i}"}}}).json()["count"]

    assert pool.map(put_then_count, range(40), workers=8) == [1] * 40


def test_parallel_realtime_get_after_index(pool, idx):
    def put_then_get(c, i):
        assert c.put(f"/{idx}/_doc/{i}", json={"n": i}).status_code == 201
        g = c.get(f"/{idx}/_doc/{i}")
        return g.status_code, g.json()["_source"]["n"]

    assert pool.map(put_then_get, range(60), workers=12) == [(200, i) for i in range(60)]


def test_parallel_create_same_id_exactly_one_wins(client, pool, idx):
    rs = pool.map(
        lambda c, i: c.put(f"/{idx}/_doc/same", json={"who": i}, params={"op_type": "create"}),
        range(24), workers=12,
    )
    statuses = sorted(r.status_code for r in rs)
    assert statuses == [201] + [409] * 23
    assert {_err_type(r) for r in rs if r.status_code == 409} == {"version_conflict_engine_exception"}
    winner = next(r.json()["_seq_no"] for r in rs if r.status_code == 201)
    g = client.get(f"/{idx}/_doc/same").json()
    assert g["_version"] == 1 and g["_seq_no"] == winner


def test_parallel_create_endpoint_same_id_exactly_one_wins(pool, idx):
    rs = pool.map(lambda c, i: c.put(f"/{idx}/_create/x", json={"who": i}), range(20), workers=10)
    assert sorted(r.status_code for r in rs) == [201] + [409] * 19


def test_parallel_overwrite_same_id_versions_are_dense(client, pool, idx):
    rs = pool.map(lambda c, i: c.put(f"/{idx}/_doc/hot", json={"n": i}), range(50), workers=10)
    assert all(r.status_code in (200, 201) for r in rs)
    bodies = [r.json() for r in rs]
    assert sorted(b["_version"] for b in bodies) == list(range(1, 51))
    assert sorted(b["_seq_no"] for b in bodies) == list(range(50))
    assert [b["result"] for b in bodies].count("created") == 1
    last = max(bodies, key=lambda b: b["_version"])
    g = client.get(f"/{idx}/_doc/hot").json()
    assert g["_version"] == 50
    assert g["_seq_no"] == last["_seq_no"]
    # the stored source is the one written by the request that got the last version
    winner = json.loads(rs[bodies.index(last)].request.content)["n"]
    assert g["_source"]["n"] == winner


def test_parallel_delete_same_doc_exactly_one_deletes(client, pool, idx):
    assert client.put(f"/{idx}/_doc/d", json={"n": 1}).status_code == 201
    rs = pool.map(lambda c, i: c.delete(f"/{idx}/_doc/d"), range(20), workers=10)
    results = sorted(r.json()["result"] for r in rs)
    assert results == ["deleted"] + ["not_found"] * 19
    assert sorted(r.status_code for r in rs) == [200] + [404] * 19
    assert client.get(f"/{idx}/_doc/d").status_code == 404


def test_parallel_index_and_delete_distinct_docs(client, pool, idx):
    _seed(client, idx, 100)

    def op(c, i):
        if i < 100:
            return c.delete(f"/{idx}/_doc/{i}").status_code
        return c.put(f"/{idx}/_doc/{i}", json={"n": i}).status_code

    statuses = pool.map(op, range(200), workers=16)
    assert statuses == [200] * 100 + [201] * 100
    _refresh(client, idx)
    assert _count(client, idx) == 100
    assert _count(client, idx, {"range": {"n": {"lt": 100}}}) == 0


# =========================================================================== updates


def test_parallel_scripted_updates_with_retry_on_conflict(client, pool, idx):
    assert client.put(f"/{idx}/_doc/c", json={"n": 0}).status_code == 201
    body = {"script": {"source": "ctx._source.n += 1", "lang": "painless"}}
    rs = pool.map(
        lambda c, i: c.post(f"/{idx}/_update/c", json=body, params={"retry_on_conflict": "100"}),
        range(60), workers=12,
    )
    assert all(r.status_code == 200 for r in rs), [r.text for r in rs if r.status_code != 200][:3]
    g = client.get(f"/{idx}/_doc/c").json()
    assert g["_source"]["n"] == 60
    assert g["_version"] == 61
    assert sorted(r.json()["_version"] for r in rs) == list(range(2, 62))


def test_parallel_scripted_updates_without_retry_lose_nothing(client, pool, idx):
    assert client.put(f"/{idx}/_doc/c", json={"n": 0}).status_code == 201
    body = {"script": "ctx._source.n += 1"}
    rs = pool.map(lambda c, i: c.post(f"/{idx}/_update/c", json=body), range(60), workers=12)
    assert {r.status_code for r in rs} <= {200, 409}
    assert {_err_type(r) for r in rs if r.status_code == 409} <= {"version_conflict_engine_exception"}
    ok = sum(r.status_code == 200 for r in rs)
    assert ok >= 1
    g = client.get(f"/{idx}/_doc/c").json()
    assert g["_source"]["n"] == ok
    assert g["_version"] == ok + 1


def test_parallel_partial_doc_updates_merge_all_fields(client, pool, idx):
    assert client.put(f"/{idx}/_doc/m", json={"base": True}).status_code == 201
    rs = pool.map(
        lambda c, i: c.post(f"/{idx}/_update/m", json={"doc": {f"f{i}": i}},
                            params={"retry_on_conflict": "100"}),
        range(30), workers=10,
    )
    assert all(r.status_code == 200 for r in rs)
    src = client.get(f"/{idx}/_doc/m").json()["_source"]
    assert src == {"base": True, **{f"f{i}": i for i in range(30)}}


def test_parallel_upserts_same_id(client, pool, idx):
    body = {"script": {"source": "ctx._source.n += params.k", "params": {"k": 1}}, "upsert": {"n": 1}}
    rs = pool.map(
        lambda c, i: c.post(f"/{idx}/_update/u", json=body, params={"retry_on_conflict": "100"}),
        range(40), workers=10,
    )
    assert all(r.status_code in (200, 201) for r in rs), [r.text for r in rs if r.status_code >= 300][:3]
    assert [r.json()["result"] for r in rs].count("created") == 1
    g = client.get(f"/{idx}/_doc/u").json()
    assert g["_source"]["n"] == 40 and g["_version"] == 40


def test_parallel_noop_updates_keep_version(client, pool, idx):
    assert client.put(f"/{idx}/_doc/n", json={"v": 1}).status_code == 201
    rs = pool.map(lambda c, i: c.post(f"/{idx}/_update/n", json={"doc": {"v": 1}}), range(30), workers=10)
    assert all(r.status_code == 200 and r.json()["result"] == "noop" for r in rs)
    assert client.get(f"/{idx}/_doc/n").json()["_version"] == 1


# =========================================================================== optimistic concurrency


def test_if_seq_no_exactly_one_winner(client, pool, idx):
    r = client.put(f"/{idx}/_doc/occ", json={"n": 0})
    seq, term = r.json()["_seq_no"], r.json()["_primary_term"]
    rs = pool.map(
        lambda c, i: c.put(f"/{idx}/_doc/occ", json={"n": i},
                           params={"if_seq_no": seq, "if_primary_term": term}),
        range(1, 25), workers=12,
    )
    assert sorted(r.status_code for r in rs) == [200] + [409] * 23
    assert {_err_type(r) for r in rs if r.status_code == 409} == {"version_conflict_engine_exception"}
    winner = next(r for r in rs if r.status_code == 200)
    g = client.get(f"/{idx}/_doc/occ").json()
    assert g["_version"] == 2
    assert g["_source"]["n"] == json.loads(winner.request.content)["n"]


def test_if_seq_no_delete_exactly_one_winner(client, pool, idx):
    r = client.put(f"/{idx}/_doc/occ", json={"n": 0})
    seq, term = r.json()["_seq_no"], r.json()["_primary_term"]
    rs = pool.map(
        lambda c, i: c.delete(f"/{idx}/_doc/occ", params={"if_seq_no": seq, "if_primary_term": term}),
        range(16), workers=8,
    )
    statuses = sorted(r.status_code for r in rs)
    assert statuses[0] == 200 and statuses.count(200) == 1
    assert set(statuses[1:]) <= {404, 409}


def test_compare_and_set_loop_counter(client, pool, idx):
    assert client.put(f"/{idx}/_doc/cas", json={"n": 0}).status_code == 201

    def increment(c, _):
        conflicts = 0
        for _attempt in range(500):
            g = c.get(f"/{idx}/_doc/cas").json()
            r = c.put(f"/{idx}/_doc/cas", json={"n": g["_source"]["n"] + 1},
                      params={"if_seq_no": g["_seq_no"], "if_primary_term": g["_primary_term"]})
            if r.status_code == 200:
                return conflicts
            assert r.status_code == 409, r.text
            conflicts += 1
        raise AssertionError("no progress after 500 attempts")

    pool.map(increment, range(40), workers=8)
    g = client.get(f"/{idx}/_doc/cas").json()
    assert g["_source"]["n"] == 40
    assert g["_version"] == 41


def test_external_versions_highest_wins(client, pool, idx):
    versions = list(range(1, 41))
    random.Random(7).shuffle(versions)
    rs = pool.map(
        lambda c, v: c.put(f"/{idx}/_doc/ext", json={"v": v},
                           params={"version": v, "version_type": "external"}),
        versions, workers=10,
    )
    assert {r.status_code for r in rs} <= {200, 201, 409}
    by_version = {json.loads(r.request.content)["v"]: r.status_code for r in rs}
    assert by_version[40] in (200, 201)
    g = client.get(f"/{idx}/_doc/ext").json()
    assert g["_version"] == 40 and g["_source"] == {"v": 40}


# =========================================================================== bulk


def test_parallel_bulks_distinct_ids(client, pool, idx):
    def bulk(c, k):
        lines = []
        for i in range(100):
            lines += [{"index": {"_id": f"{k}-{i}"}}, {"n": k * 100 + i, "tag": f"b{k}"}]
        r = _bulk(c, lines, index=idx)
        assert r.status_code == 200, r.text
        return r.json()

    results = pool.map(bulk, range(12), workers=6)
    assert all(b["errors"] is False for b in results)
    seqs = sorted(it["index"]["_seq_no"] for b in results for it in b["items"])
    assert seqs == list(range(1200))
    _refresh(client, idx)
    assert _count(client, idx) == 1200
    r = client.post(f"/{idx}/_search", json={"size": 0, "aggs": {"t": {"terms": {"field": "tag", "size": 20}}}})
    buckets = r.json()["aggregations"]["t"]["buckets"]
    assert sorted((b["key"], b["doc_count"]) for b in buckets) == sorted((f"b{k}", 100) for k in range(12))


def test_parallel_bulk_create_same_ids_one_winner_per_id(client, pool, idx):
    def bulk(c, k):
        lines = []
        for i in range(50):
            lines += [{"create": {"_id": str(i)}}, {"writer": k}]
        r = _bulk(c, lines, index=idx)
        assert r.status_code == 200, r.text
        return [it["create"]["status"] for it in r.json()["items"]]

    per_writer = pool.map(bulk, range(8), workers=8)
    for i in range(50):
        column = sorted(statuses[i] for statuses in per_writer)
        assert column == [201] + [409] * 7, (i, column)
    _refresh(client, idx)
    assert _count(client, idx) == 50


def test_parallel_bulk_updates_same_doc_with_retry(client, pool, idx):
    assert client.put(f"/{idx}/_doc/c", json={"n": 0}).status_code == 201

    def bulk(c, k):
        lines = []
        for _ in range(10):
            lines += [{"update": {"_id": "c", "retry_on_conflict": 100}}, {"script": "ctx._source.n++"}]
        r = _bulk(c, lines, index=idx)
        assert r.status_code == 200, r.text
        return r.json()["errors"]

    assert pool.map(bulk, range(8), workers=8) == [False] * 8
    g = client.get(f"/{idx}/_doc/c").json()
    assert g["_source"]["n"] == 80 and g["_version"] == 81


def test_parallel_bulks_into_many_indices(client, pool, scratch):
    names = [_uniq("slopensearch-conc-multi") for _ in range(4)]
    for n in names:
        scratch.append(n)
        assert client.put(f"/{n}", json={"settings": SINGLE}).status_code == 200

    def bulk(c, k):
        lines = []
        for i in range(40):
            lines += [{"index": {"_index": names[i % 4], "_id": f"{k}-{i}"}}, {"k": k}]
        r = _bulk(c, lines)
        return r.status_code, r.json()["errors"]

    assert pool.map(bulk, range(10), workers=5) == [(200, False)] * 10
    for n in names:
        _refresh(client, n)
        assert _count(client, n) == 100


# =========================================================================== index and mapping admin


def test_parallel_create_same_index_exactly_one_wins(client, pool, scratch):
    name = _uniq()
    scratch.append(name)
    rs = pool.map(lambda c, i: c.put(f"/{name}", json={"settings": SINGLE}), range(12), workers=12)
    assert sorted(r.status_code for r in rs) == [200] + [400] * 11
    assert {_err_type(r) for r in rs if r.status_code == 400} == {"resource_already_exists_exception"}
    assert client.get(f"/{name}").status_code == 200


def test_parallel_create_and_delete_distinct_indices(client, pool, scratch):
    prefix = _uniq("slopensearch-conc-many")
    names = [f"{prefix}-{i}" for i in range(12)]
    scratch.extend(names)
    rs = pool.map(lambda c, n: c.put(f"/{n}", json={"settings": SINGLE}), names, workers=12)
    assert all(r.status_code == 200 and r.json()["acknowledged"] for r in rs)
    r = client.get(f"/_cat/indices/{prefix}-*", params={"h": "index", "format": "json"})
    assert sorted(e["index"] for e in r.json()) == sorted(names)
    rs = pool.map(lambda c, n: c.delete(f"/{n}"), names, workers=12)
    assert all(r.status_code == 200 for r in rs)
    r = client.get(f"/{prefix}-*")
    assert r.status_code == 200 and r.json() == {}


def test_parallel_delete_same_index(client, pool, scratch):
    name = _uniq()
    scratch.append(name)
    assert client.put(f"/{name}").status_code == 200
    rs = pool.map(lambda c, i: c.delete(f"/{name}"), range(10), workers=10)
    statuses = [r.status_code for r in rs]
    assert set(statuses) <= {200, 404} and 200 in statuses
    assert {_err_type(r) for r in rs if r.status_code == 404} <= {"index_not_found_exception"}
    assert client.head(f"/{name}").status_code == 404


def test_parallel_index_into_auto_created_index(client, pool, scratch):
    name = _uniq()
    scratch.append(name)
    rs = pool.map(lambda c, i: c.put(f"/{name}/_doc/{i}", json={"n": i, "s": f"v{i}"}), range(30), workers=15)
    assert all(r.status_code == 201 for r in rs), [r.text for r in rs if r.status_code != 201][:3]
    _refresh(client, name)
    assert _count(client, name) == 30
    props = client.get(f"/{name}/_mapping").json()[name]["mappings"]["properties"]
    assert props["n"] == {"type": "long"}
    assert props["s"]["type"] == "text"


def test_parallel_dynamic_mapping_distinct_new_fields(client, pool, idx):
    rs = pool.map(lambda c, i: c.put(f"/{idx}/_doc/{i}", json={f"new_{i}": i}), range(30), workers=15)
    assert all(r.status_code == 201 for r in rs)
    props = client.get(f"/{idx}/_mapping").json()[idx]["mappings"]["properties"]
    for i in range(30):
        assert props[f"new_{i}"] == {"type": "long"}
    _refresh(client, idx)
    for i in (0, 17, 29):
        assert _count(client, idx, {"term": {f"new_{i}": i}}) == 1


def test_parallel_dynamic_mapping_same_new_field(client, pool, idx):
    rs = pool.map(lambda c, i: c.put(f"/{idx}/_doc/{i}", json={"shared": i * 1.5}), range(30), workers=15)
    assert all(r.status_code == 201 for r in rs)
    props = client.get(f"/{idx}/_mapping").json()[idx]["mappings"]["properties"]
    assert props["shared"] == {"type": "float"}
    _refresh(client, idx)
    assert _count(client, idx, {"range": {"shared": {"gte": 0}}}) == 30


def test_parallel_put_mapping_distinct_fields(client, pool, idx):
    rs = pool.map(
        lambda c, i: c.put(f"/{idx}/_mapping", json={"properties": {f"k{i}": {"type": "keyword"}}}),
        range(20), workers=10,
    )
    assert all(r.status_code == 200 and r.json() == {"acknowledged": True} for r in rs)
    props = client.get(f"/{idx}/_mapping").json()[idx]["mappings"]["properties"]
    assert all(props[f"k{i}"] == {"type": "keyword"} for i in range(20))


def test_parallel_put_mapping_conflicting_types_one_type_wins(client, pool, idx):
    types = ["keyword", "long"] * 8
    rs = pool.map(
        lambda c, t: c.put(f"/{idx}/_mapping", json={"properties": {"contested": {"type": t}}}),
        types, workers=16,
    )
    final = client.get(f"/{idx}/_mapping").json()[idx]["mappings"]["properties"]["contested"]["type"]
    assert final in ("keyword", "long")
    for t, r in zip(types, rs):
        if t == final:
            assert r.status_code == 200, r.text
        else:
            assert r.status_code == 400 and _err_type(r) == "illegal_argument_exception", r.text


def test_settings_updates_while_searching(client, pool, idx):
    _seed(client, idx, 50)

    def writer(c):
        for i in range(20):
            r = c.put(f"/{idx}/_settings",
                      json={"index": {"max_result_window": 10000 + i, "refresh_interval": f"{i + 1}s"}})
            assert r.status_code == 200, r.text

    def reader(c):
        r = c.post(f"/{idx}/_search", json={"size": 5, "query": {"term": {"tag": "even"}}})
        return r.status_code, r.json()["hits"]["total"]["value"]

    _, seen = _readers_until(pool, writer, reader)
    assert all(x == (200, 25) for s in seen for x in s)
    s = client.get(f"/{idx}/_settings").json()[idx]["settings"]["index"]
    assert s["max_result_window"] == "10019" and s["refresh_interval"] == "20s"


def test_put_mapping_while_searching_and_indexing(client, pool, idx):
    _seed(client, idx, 40)

    def writer(c):
        for i in range(15):
            r = c.put(f"/{idx}/_mapping", json={"properties": {f"extra{i}": {"type": "integer"}}})
            assert r.status_code == 200, r.text
            r = c.put(f"/{idx}/_doc/x{i}", json={f"extra{i}": i, "tag": "extra"}, params={"refresh": "true"})
            assert r.status_code == 201, r.text

    def reader(c):
        r = c.post(f"/{idx}/_search", json={
            "size": 0, "query": {"terms": {"tag": ["even", "odd"]}},
            "aggs": {"t": {"terms": {"field": "tag"}}, "m": {"max": {"field": "n"}}},
        })
        a = r.json()["aggregations"]
        return r.status_code, r.json()["hits"]["total"]["value"], a["m"]["value"], \
            sorted((b["key"], b["doc_count"]) for b in a["t"]["buckets"])

    _, seen = _readers_until(pool, writer, reader)
    assert all(x == (200, 40, 39.0, [("even", 20), ("odd", 20)]) for s in seen for x in s)
    assert _count(client, idx, {"term": {"tag": "extra"}}) == 15


# =========================================================================== reads while writing


def test_count_never_exceeds_indexed_and_is_monotonic(client, pool, index_factory):
    # no periodic refresh: only the writer's refresh=true makes documents visible
    idx = index_factory(settings={**SINGLE, "refresh_interval": "-1"})
    sent = [0]

    def writer(c):
        for k in range(20):
            lines = []
            for i in range(25):
                lines += [{"index": {"_id": f"{k}-{i}"}}, {"n": i}]
            sent[0] += 25  # raised before the request, so a count can never exceed it
            r = _bulk(c, lines, index=idx, refresh="true")
            assert r.status_code == 200 and r.json()["errors"] is False

    def reader(c):
        r = c.post(f"/{idx}/_count")
        assert r.status_code == 200, r.text
        return r.json()["count"], sent[0]

    _, seen = _readers_until(pool, writer, reader)
    for s in seen:
        counts = [cnt for cnt, _ in s]
        assert counts == sorted(counts), "a refreshed count went backwards"
        assert all(cnt <= limit for cnt, limit in s)
        assert all(cnt % 25 == 0 for cnt in counts), "a bulk became visible only partly"
    assert seen[0][-1][0] == 500
    assert _count(client, idx) == 500


def test_aggregations_consistent_within_response_during_indexing(pool, index_factory):
    idx = index_factory(settings={**SINGLE, "refresh_interval": "-1"},
                        mappings={"properties": {"tag": {"type": "keyword"}, "n": {"type": "long"}}})
    def writer(c):
        for k in range(15):
            lines = []
            for i in range(20):
                lines += [{"index": {}}, {"tag": f"t{i % 4}", "n": 1}]
            assert _bulk(c, lines, index=idx, refresh="true").status_code == 200

    def reader(c):
        r = c.post(f"/{idx}/_search", json={
            "size": 0, "track_total_hits": True,
            "aggs": {"t": {"terms": {"field": "tag"}}, "s": {"sum": {"field": "n"}}, "v": {"value_count": {"field": "n"}}},
        })
        assert r.status_code == 200, r.text
        b = r.json()
        total = b["hits"]["total"]["value"]
        a = b["aggregations"]
        return total, sum(x["doc_count"] for x in a["t"]["buckets"]), a["s"]["value"], a["v"]["value"]

    _, seen = _readers_until(pool, writer, reader)
    for s in seen:
        for total, buckets, s_sum, vc in s:
            assert total == buckets == s_sum == vc
            assert total % 20 == 0 and total <= 300
    assert seen[0][-1][0] == 300


def test_terms_lookup_while_lookup_doc_and_indices_change(client, pool, idx, index_factory):
    lookup = index_factory(settings=SINGLE)
    other = index_factory(settings=SINGLE)
    _seed(client, idx, 30, tag_of=lambda i: "abc"[i % 3])
    assert client.put(f"/{lookup}/_doc/L", json={"vals": ["a", "b"]}, params={"refresh": "true"}).status_code == 201

    def writer(c):
        for i in range(30):
            vals = ["a", "b", "c"] if i % 2 == 0 else ["a", "b"]
            assert c.put(f"/{lookup}/_doc/L", json={"vals": vals}).status_code == 200
            assert c.put(f"/{other}/_doc/{i}", json={"i": i}, params={"refresh": "true"}).status_code == 201
            r = c.put(f"/{idx}/_mapping", json={"properties": {f"w{i}": {"type": "keyword"}}})
            assert r.status_code == 200
        assert c.put(f"/{lookup}/_doc/L", json={"vals": ["a", "b"]}).status_code == 200

    def reader(c):
        r = c.post(f"/{idx}/_search", json={
            "size": 0, "query": {"terms": {"tag": {"index": lookup, "id": "L", "path": "vals"}}},
        })
        assert r.status_code == 200, r.text
        return r.json()["hits"]["total"]["value"]

    _, seen = _readers_until(pool, writer, reader, readers=6)
    assert all(v in (20, 30) for s in seen for v in s)
    assert all(s[-1] == 20 for s in seen)
    assert _count(client, other) == 30


def test_scripts_while_indexing(client, pool, idx):
    _seed(client, idx, 40)

    def writer(c):
        for i in range(30):
            r = c.post(f"/{idx}/_update/{i}", json={"script": "ctx._source.n += 1000"})
            assert r.status_code == 200, r.text
            if i % 5 == 0:
                _refresh(c, idx)

    def reader(c):
        r = c.post(f"/{idx}/_search", json={
            "size": 40, "_source": False,
            "query": {"script_score": {"query": {"match_all": {}},
                                       "script": {"source": "doc['n'].value % 1000 + 1"}}},
            "script_fields": {"m": {"script": {"source": "doc['n'].value % 1000"}}},
            "aggs": {"s": {"sum": {"script": {"source": "doc['n'].value % 1000"}}}},
        })
        assert r.status_code == 200, r.text
        b = r.json()
        return b["hits"]["total"]["value"], b["aggregations"]["s"]["value"], \
            sorted(h["fields"]["m"][0] for h in b["hits"]["hits"])

    _, seen = _readers_until(pool, writer, reader)
    expected = (40, float(sum(range(40))), list(range(40)))
    assert all(x == expected for s in seen for x in s)
    _refresh(client, idx)
    assert _count(client, idx, {"range": {"n": {"gte": 1000}}}) == 30


def test_analyze_while_indices_with_analyzers_come_and_go(client, pool, scratch):
    prefix = _uniq("slopensearch-conc-an")
    settings = {**SINGLE, "analysis": {"analyzer": {"my": {"type": "custom", "tokenizer": "whitespace",
                                                           "filter": ["lowercase", "reverse"]}}}}
    stable = f"{prefix}-stable"
    scratch.append(stable)
    assert client.put(f"/{stable}", json={"settings": settings}).status_code == 200

    def writer(c):
        for i in range(10):
            name = f"{prefix}-{i}"
            scratch.append(name)
            assert c.put(f"/{name}", json={"settings": settings}).status_code == 200
            assert c.delete(f"/{name}").status_code == 200

    def reader(c):
        a = c.post("/_analyze", json={"analyzer": "standard", "text": "Quick Brown-Fox 42"})
        b = c.post(f"/{stable}/_analyze", json={"analyzer": "my", "text": "Hello World"})
        assert a.status_code == 200 and b.status_code == 200, (a.text, b.text)
        return [t["token"] for t in a.json()["tokens"]], [t["token"] for t in b.json()["tokens"]]

    _, seen = _readers_until(pool, writer, reader)
    assert all(x == (["quick", "brown", "fox", "42"], ["olleh", "dlrow"]) for s in seen for x in s)


def test_parallel_analyze_matches_serial(client, pool):
    cases = [
        ("standard", "The QUICK brown fox"), ("whitespace", "a b  c"), ("simple", "Foo-Bar 99 baz"),
        ("keyword", "Keep As Is"), ("stop", "the cat and the hat"), ("english", "running dogs jumped"),
    ] * 8
    serial = {}
    for an, text in cases[:6]:
        r = client.post("/_analyze", json={"analyzer": an, "text": text})
        serial[(an, text)] = [t["token"] for t in r.json()["tokens"]]

    def analyze(c, case):
        r = c.post("/_analyze", json={"analyzer": case[0], "text": case[1]})
        return [t["token"] for t in r.json()["tokens"]]

    assert pool.map(analyze, cases, workers=12) == [serial[c] for c in cases]


def test_wildcard_search_while_indices_created_and_deleted(client, pool, scratch):
    prefix = _uniq("slopensearch-conc-wild")
    stable = f"{prefix}-stable"
    scratch.append(stable)
    assert client.put(f"/{stable}", json={"settings": SINGLE}).status_code == 200
    _seed(client, stable, 10)

    def writer(c):
        for i in range(12):
            name = f"{prefix}-tmp{i}"
            scratch.append(name)
            assert c.put(f"/{name}", json={"settings": SINGLE}).status_code == 200
            assert c.delete(f"/{name}").status_code == 200

    def reader(c):
        r = c.post(f"/{prefix}-stable,{prefix}-tmp*/_count", params={"ignore_unavailable": "true"})
        s = c.post(f"/{prefix}-stable/_search", json={"size": 0})
        assert s.status_code == 200, s.text
        return r.status_code, s.json()["hits"]["total"]["value"]

    _, seen = _readers_until(pool, writer, reader)
    assert all(x == (200, 10) for s in seen for x in s)


def test_mget_versions_monotonic_while_updating(client, pool, idx):
    _seed(client, idx, 5)

    def writer(c):
        for i in range(25):
            r = c.post(f"/{idx}/_bulk", content=_ndjson(
                [x for k in range(5) for x in ({"update": {"_id": str(k)}}, {"doc": {"r": i}})]), headers=NDJSON)
            assert r.status_code == 200 and r.json()["errors"] is False

    def reader(c):
        r = c.post(f"/{idx}/_mget", json={"ids": [str(k) for k in range(5)]})
        assert r.status_code == 200, r.text
        return [d["_version"] for d in r.json()["docs"]]

    _, seen = _readers_until(pool, writer, reader)
    for s in seen:
        for k in range(5):
            col = [v[k] for v in s]
            assert col == sorted(col)
        assert s[-1] == [26] * 5


def test_refresh_flush_forcemerge_while_indexing(client, pool, idx):
    def bulk(c, k):
        lines = []
        for i in range(50):
            lines += [{"index": {"_id": f"{k}-{i}"}}, {"n": i}]
        r = _bulk(c, lines, index=idx)
        return r.status_code, r.json()["errors"]

    def maintain(c, k):
        op = ["_refresh", "_flush", "_forcemerge"][k % 3]
        return c.post(f"/{idx}/{op}").status_code, False

    tasks = [lambda c, k=k: bulk(c, k) for k in range(10)] + [lambda c, k=k: maintain(c, k) for k in range(9)]
    random.Random(3).shuffle(tasks)
    assert sorted(pool.run(tasks, workers=8)) == [(200, False)] * 19
    _refresh(client, idx)
    assert _count(client, idx) == 500


# =========================================================================== aliases


def test_atomic_alias_swap_while_searching(client, pool, index_factory):
    a = index_factory(settings=SINGLE)
    b = index_factory(settings=SINGLE)
    _seed(client, a, 5, tag_of=lambda i: "a")
    _seed(client, b, 7, tag_of=lambda i: "b")
    alias = _uniq("slopensearch-conc-alias")
    assert client.post("/_aliases", json={"actions": [{"add": {"index": a, "alias": alias}}]}).status_code == 200

    def writer(c):
        cur, nxt = a, b
        for _ in range(20):
            r = c.post("/_aliases", json={"actions": [
                {"remove": {"index": cur, "alias": alias}}, {"add": {"index": nxt, "alias": alias}}]})
            assert r.status_code == 200, r.text
            cur, nxt = nxt, cur

    def reader(c):
        r = c.post(f"/{alias}/_search", json={"size": 20})
        assert r.status_code == 200, r.text
        hits = r.json()["hits"]
        return hits["total"]["value"], {h["_index"] for h in hits["hits"]}, {h["_source"]["tag"] for h in hits["hits"]}

    _, seen = _readers_until(pool, writer, reader, readers=6)
    for s in seen:
        for total, indices, tags in s:
            assert (total, indices, tags) in ((5, {a}, {"a"}), (7, {b}, {"b"}))
    r = client.get(f"/_alias/{alias}")
    assert list(r.json()) == [a]


def test_parallel_add_distinct_aliases(client, pool, idx):
    prefix = _uniq("slopensearch-conc-al")
    rs = pool.map(lambda c, i: c.put(f"/{idx}/_alias/{prefix}-{i}"), range(20), workers=10)
    assert all(r.status_code == 200 for r in rs)
    aliases = client.get(f"/{idx}/_alias").json()[idx]["aliases"]
    assert sorted(aliases) == sorted(f"{prefix}-{i}" for i in range(20))


# =========================================================================== scroll and PIT


def test_concurrent_scrolls_see_snapshot(client, pool, idx):
    _seed(client, idx, 120)

    def scroll_all(c, k):
        if k == 0:
            # one task keeps indexing while the scrolls run
            for i in range(120, 220):
                assert c.put(f"/{idx}/_doc/{i}", json={"n": i}, params={"refresh": "true"}).status_code == 201
            return None
        r = c.post(f"/{idx}/_search", params={"scroll": "1m"},
                   json={"size": 7, "sort": ["_doc"], "query": {"range": {"n": {"gte": 0}}}})
        assert r.status_code == 200, r.text
        b = r.json()
        total = b["hits"]["total"]["value"]
        ids = [h["_id"] for h in b["hits"]["hits"]]
        sid = b["_scroll_id"]
        while True:
            r = c.post("/_search/scroll", json={"scroll": "1m", "scroll_id": sid})
            assert r.status_code == 200, r.text
            b = r.json()
            if not b["hits"]["hits"]:
                break
            ids += [h["_id"] for h in b["hits"]["hits"]]
            sid = b["_scroll_id"]
        c.request("DELETE", "/_search/scroll", json={"scroll_id": [sid]})
        return total, len(ids), len(set(ids))

    results = pool.map(scroll_all, range(9), workers=9)[1:]
    for total, n, distinct in results:
        assert total == n == distinct
        assert 120 <= total <= 220
    _refresh(client, idx)
    assert _count(client, idx) == 220


def test_concurrent_pits_see_snapshot_while_indexing(client, pool, idx):
    _seed(client, idx, 100)
    pits = []
    for _ in range(6):
        r = client.post(f"/{idx}/_search/point_in_time", params={"keep_alive": "1m"})
        assert r.status_code == 200, r.text
        pits.append(r.json()["pit_id"])

    def task(c, k):
        if k == len(pits):
            for i in range(100, 200):
                assert c.put(f"/{idx}/_doc/{i}", json={"n": i, "tag": "new"}, params={"refresh": "true"}).status_code == 201
            return None
        seen = []
        after = None
        while True:
            body = {"size": 9, "pit": {"id": pits[k], "keep_alive": "1m"}, "sort": [{"n": "asc"}]}
            if after:
                body["search_after"] = after
            r = c.post("/_search", json=body)
            assert r.status_code == 200, r.text
            hits = r.json()["hits"]["hits"]
            if not hits:
                return seen
            seen += [h["_source"]["n"] for h in hits]
            after = hits[-1]["sort"]

    try:
        results = pool.map(task, range(len(pits) + 1), workers=len(pits) + 1)
    finally:
        client.request("DELETE", "/_search/point_in_time", json={"pit_id": pits})
    assert results[:-1] == [list(range(100))] * len(pits)
    _refresh(client, idx)
    assert _count(client, idx) == 200


def test_parallel_msearch(client, pool, idx):
    _seed(client, idx, 60)
    lines = []
    for tag in ("even", "odd"):
        lines += [{}, {"size": 0, "query": {"term": {"tag": tag}}}]
    lines += [{}, {"size": 0, "aggs": {"m": {"max": {"field": "n"}}}}]

    def ms(c, _):
        r = c.post(f"/{idx}/_msearch", content=_ndjson(lines), headers=NDJSON)
        rs = r.json()["responses"]
        return [x["hits"]["total"]["value"] for x in rs] + [rs[2]["aggregations"]["m"]["value"]]

    assert pool.map(ms, range(40), workers=10) == [[30, 30, 60, 59.0]] * 40


# =========================================================================== by-query APIs


def test_update_by_query_vs_overwrites_conflicts_proceed(client, pool, index_factory):
    idx = index_factory(settings={**SINGLE, "refresh_interval": "-1"},
                        mappings={"properties": {"n": {"type": "long"}, "p": {"type": "keyword"}}})
    lines = []
    for i in range(400):
        lines += [{"index": {"_id": str(i)}}, {"n": 0, "p": "even" if i % 2 == 0 else "odd"}]
    assert _bulk(client, lines, index=idx, refresh="true").json()["errors"] is False

    def task(c, k):
        if k == 0:
            r = c.post(f"/{idx}/_update_by_query",
                       params={"conflicts": "proceed", "scroll_size": "25"},
                       json={"script": {"source": "ctx._source.n += 1"}})
            assert r.status_code == 200, r.text
            return r.json()
        # overwrite the even docs; the update_by_query either got there first (and its
        # result is overwritten) or loses with a version conflict
        for i in range(k - 1, 400, 4 * 2):
            assert c.put(f"/{idx}/_doc/{i}", json={"n": 100, "p": "even"}).status_code == 200
        return None

    # writers k=1,3,5,7 cover ids 0,2,4,6 (mod 8): every even id
    tasks = [lambda c: task(c, 0)] + [lambda c, k=k: task(c, k) for k in (1, 3, 5, 7)]
    res = pool.run(tasks, workers=5)[0]
    assert res["total"] == 400
    assert res["updated"] + res["version_conflicts"] == 400
    assert res["failures"] == []
    _refresh(client, idx)
    assert _count(client, idx, {"bool": {"filter": [{"term": {"p": "odd"}}, {"term": {"n": 1}}]}}) == 200
    assert _count(client, idx, {"bool": {"filter": [{"term": {"p": "even"}}, {"term": {"n": 100}}]}}) == 200


def test_delete_by_query_while_indexing_other_docs(client, pool, idx):
    _seed(client, idx, 300, tag_of=lambda i: "old")

    def task(c, k):
        if k == 0:
            r = c.post(f"/{idx}/_delete_by_query", params={"conflicts": "proceed", "scroll_size": "50"},
                       json={"query": {"term": {"tag": "old"}}})
            assert r.status_code == 200, r.text
            return r.json()
        lines = []
        for i in range(50):
            lines += [{"index": {"_id": f"new-{k}-{i}"}}, {"tag": "new", "n": i}]
        r = _bulk(c, lines, index=idx)
        assert r.status_code == 200 and r.json()["errors"] is False
        return None

    res = pool.run([lambda c, k=k: task(c, k) for k in range(6)], workers=6)[0]
    assert res["deleted"] == 300 and res["version_conflicts"] == 0 and res["failures"] == []
    _refresh(client, idx)
    assert _count(client, idx, {"term": {"tag": "old"}}) == 0
    assert _count(client, idx, {"term": {"tag": "new"}}) == 250


def test_stored_script_replaced_while_in_use(client, pool, idx):
    _seed(client, idx, 10)
    sid = _uniq("slopensearch-conc-script")
    assert client.put(f"/_scripts/{sid}", json={"script": {"lang": "painless", "source": "doc['n'].value * 2"}}).status_code == 200
    try:
        def writer(c):
            for i in range(15):
                mult = 3 if i % 2 == 0 else 2
                r = c.put(f"/_scripts/{sid}",
                          json={"script": {"lang": "painless", "source": f"doc['n'].value * {mult}"}})
                assert r.status_code == 200, r.text

        def reader(c):
            r = c.post(f"/{idx}/_search", json={"size": 0, "aggs": {"s": {"sum": {"script": {"id": sid}}}}})
            assert r.status_code == 200, r.text
            return r.json()["aggregations"]["s"]["value"]

        _, seen = _readers_until(pool, writer, reader)
        assert all(v in (90.0, 135.0) for s in seen for v in s)
        assert all(s[-1] == 90.0 for s in seen)
    finally:
        client.delete(f"/_scripts/{sid}")


# =========================================================================== load


@pytest.fixture(scope="module")
def loaded(base_url):
    """10,000 small docs written by parallel bulks; shared by the load tests below."""
    name = _uniq("slopensearch-conc-load")
    with httpx.Client(base_url=base_url, timeout=REQUEST_TIMEOUT) as c:
        r = c.put(f"/{name}", json={"settings": SINGLE, "mappings": {"properties": {
            "n": {"type": "long"}, "cat": {"type": "keyword"}, "v": {"type": "double"},
            "title": {"type": "text"}}}})
        assert r.status_code == 200, r.text
        w = _Workers(base_url)

        def bulk(c2, k):
            lines = []
            for i in range(k * 1000, (k + 1) * 1000):
                lines += [{"index": {"_id": str(i)}},
                          {"n": i, "cat": f"c{i % 10}", "v": (i % 100) / 4, "title": f"item {i % 7} of the load"}]
            r = _bulk(c2, lines, index=name)
            assert r.status_code == 200 and r.json()["errors"] is False, r.text[:500]

        try:
            w.map(bulk, range(10), workers=5)
        finally:
            w.close()
        _refresh(c, name)
        yield name
        c.delete(f"/{name}")


def test_load_count_and_aggregations(client, loaded):
    assert _count(client, loaded) == 10000
    r = client.post(f"/{loaded}/_search", json={"size": 0, "aggs": {
        "cats": {"terms": {"field": "cat", "size": 20}, "aggs": {"s": {"sum": {"field": "n"}}}},
        "h": {"histogram": {"field": "n", "interval": 2500}},
        "st": {"stats": {"field": "v"}},
    }})
    assert r.status_code == 200, r.text
    a = r.json()["aggregations"]
    assert sorted((b["key"], b["doc_count"]) for b in a["cats"]["buckets"]) == [(f"c{k}", 1000) for k in range(10)]
    for b in a["cats"]["buckets"]:
        k = int(b["key"][1:])
        assert b["s"]["value"] == sum(range(k, 10000, 10))
    assert [(b["key"], b["doc_count"]) for b in a["h"]["buckets"]] == [(k * 2500.0, 2500) for k in range(4)]
    assert a["st"]["count"] == 10000 and a["st"]["min"] == 0 and a["st"]["max"] == 24.75


def test_load_search_after_deep_pagination(client, loaded):
    seen = []
    after = None
    while True:
        body = {"size": 1000, "sort": [{"n": "desc"}], "_source": ["n"]}
        if after:
            body["search_after"] = after
        r = client.post(f"/{loaded}/_search", json=body)
        assert r.status_code == 200, r.text
        hits = r.json()["hits"]["hits"]
        if not hits:
            break
        seen += [h["_source"]["n"] for h in hits]
        after = hits[-1]["sort"]
    assert seen == list(range(9999, -1, -1))


def test_load_parallel_queries(pool, loaded):
    queries = [
        ({"query": {"term": {"cat": "c3"}}, "size": 3, "sort": [{"n": "asc"}]}, lambda b: [h["_source"]["n"] for h in b["hits"]["hits"]] == [3, 13, 23]),
        ({"query": {"range": {"n": {"gte": 500, "lt": 1500}}}, "size": 0}, lambda b: b["hits"]["total"]["value"] == 1000),
        ({"query": {"match": {"title": "item 3"}}, "size": 0, "track_total_hits": True},
         lambda b: b["hits"]["total"]["value"] == 10000),
        ({"query": {"match_phrase": {"title": "item 3"}}, "size": 0, "track_total_hits": True},
         lambda b: b["hits"]["total"]["value"] == 1429),
        ({"size": 2, "sort": [{"v": "desc"}, {"n": "asc"}]}, lambda b: [h["_source"]["n"] for h in b["hits"]["hits"]] == [99, 199]),
        ({"size": 0, "aggs": {"c": {"cardinality": {"field": "cat"}}}}, lambda b: b["aggregations"]["c"]["value"] == 10),
        ({"size": 0, "aggs": {"r": {"range": {"field": "v", "ranges": [{"to": 10}, {"from": 10}]}}}},
         lambda b: [x["doc_count"] for x in b["aggregations"]["r"]["buckets"]] == [4000, 6000]),
        ({"from": 9990, "size": 10, "sort": [{"n": "asc"}]}, lambda b: [h["_source"]["n"] for h in b["hits"]["hits"]] == list(range(9990, 10000))),
    ]

    def run(c, k):
        body, check = queries[k % len(queries)]
        r = c.post(f"/{loaded}/_search", json=body)
        assert r.status_code == 200, r.text
        return check(r.json())

    assert pool.map(run, range(80), workers=12) == [True] * 80


def test_load_concurrent_updates_and_aggs(client, pool, loaded):
    """Readers aggregate over the whole set while writers rewrite docs without changing totals."""

    def writer(c):
        for k in range(10):
            lines = []
            for i in range(k, 10000, 50):
                lines += [{"update": {"_id": str(i)}}, {"doc": {"title": f"touched {k}"}}]
            r = _bulk(c, lines, index=loaded, refresh="true")
            assert r.status_code == 200 and r.json()["errors"] is False

    def reader(c):
        r = c.post(f"/{loaded}/_search", json={"size": 0, "track_total_hits": True,
                                               "aggs": {"s": {"sum": {"field": "n"}}, "c": {"terms": {"field": "cat"}}}})
        assert r.status_code == 200, r.text
        b = r.json()
        return b["hits"]["total"]["value"], b["aggregations"]["s"]["value"], \
            sum(x["doc_count"] for x in b["aggregations"]["c"]["buckets"])

    _, seen = _readers_until(pool, writer, reader)
    assert all(x == (10000, float(sum(range(10000))), 10000) for s in seen for x in s)
    assert _count(client, loaded, {"match": {"title": "touched"}}) == 2000
