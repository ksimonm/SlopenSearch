import gzip
import json
import re
import socket
import time
import uuid
import zlib

import httpx
import pytest

pytestmark = pytest.mark.http

JSON = {"Content-Type": "application/json"}
NDJSON = {"Content-Type": "application/x-ndjson"}
PLAIN = {"Accept-Encoding": "identity"}
SINGLE = {"number_of_shards": 1, "number_of_replicas": 0}

TASK_KEYS = ["node", "id", "type", "action", "status", "description", "start_time_in_millis",
             "running_time_in_nanos", "cancellable", "cancelled", "headers"]
STATUS_KEYS = ["total", "updated", "created", "deleted", "batches", "version_conflicts", "noops",
               "retries", "throttled_millis", "requests_per_second", "throttled_until_millis"]
RESPONSE_KEYS = ["took", "timed_out", "total", "updated", "created", "deleted", "batches",
                 "version_conflicts", "noops", "retries", "throttled", "throttled_millis",
                 "requests_per_second", "throttled_until", "throttled_until_millis", "failures"]
RPS_REASON = "[requests_per_second] must be a float greater than 0. Use -1 to disable throttling."


# --------------------------------------------------------------------------- helpers


def _uniq(prefix="slopensearch-t62"):
    return f"{prefix}-{uuid.uuid4().hex[:10]}"


def _err(resp):
    return resp.json()["error"]


def _seed(client, index, n=10):
    lines = "".join(json.dumps({"index": {"_id": str(i)}}) + "\n" + json.dumps({"n": i, "tag": "t"}) + "\n"
                    for i in range(n))
    r = client.post(f"/{index}/_bulk", content=lines.encode(), headers=NDJSON, params={"refresh": "true"})
    assert r.status_code == 200 and not r.json()["errors"], r.text


def _count(client, index):
    assert client.post(f"/{index}/_refresh").status_code == 200
    return client.get(f"/{index}/_count").json()["count"]


def _wait(client, tid):
    r = client.get(f"/_tasks/{tid}", params={"wait_for_completion": "true", "timeout": "30s"})
    assert r.status_code == 200, r.text
    body = r.json()
    assert body["completed"] is True
    return body


def _processed(status):
    return status["updated"] + status["created"] + status["deleted"]


def _wait_first_batch(client, tid):
    """Poll a throttled task until its first batch is fully done (it then sleeps ~1000s)."""
    deadline = time.time() + 20
    while time.time() < deadline:
        body = client.get(f"/_tasks/{tid}").json()
        if _processed(body["task"]["status"]) >= 1:
            return body
        time.sleep(0.1)
    pytest.fail(f"task {tid} never finished its first batch")


def _raw(client, data, timeout=10):
    """Send raw bytes and return the response head (status line + headers)."""
    url = client.base_url
    with socket.create_connection((url.host, url.port), timeout=timeout) as s:
        s.sendall(data)
        out = b""
        while b"\r\n\r\n" not in out:
            chunk = s.recv(65536)
            if not chunk:
                break
            out += chunk
    return out


# --------------------------------------------------------------------------- fixtures


@pytest.fixture(scope="module")
def node_id(client):
    (nid,) = client.get("/_nodes/_local").json()["nodes"].keys()
    return nid


@pytest.fixture
def src(client, index_factory):
    name = index_factory(settings=SINGLE)
    _seed(client, name)
    return name


@pytest.fixture
def tasks(client, index_factory):
    """Start async tasks; cancel them and drop their stored results afterwards."""
    started = []

    def _start(method_path, body=None, headers=None, **params):
        params.setdefault("wait_for_completion", "false")
        r = client.post(method_path, json=body, headers=headers, params=params)
        assert r.status_code == 200, r.text
        tid = r.json()["task"]
        started.append(tid)
        return tid

    yield _start

    for tid in started:
        client.post(f"/_tasks/{tid}/_cancel")
        client.get(f"/_tasks/{tid}", params={"wait_for_completion": "true", "timeout": "30s"})
        client.delete(f"/.tasks/_doc/{tid}")


@pytest.fixture
def dest(client):
    name = _uniq("slopensearch-t62-dst")
    yield name
    client.delete(f"/{name}", params={"ignore_unavailable": "true"})


@pytest.fixture
def idx(index_factory):
    return index_factory(settings=SINGLE)


@pytest.fixture
def doc(client, idx):
    r = client.put(f"/{idx}/_doc/1", json={"a": "x", "n": 1}, params={"refresh": "true"})
    assert r.status_code == 201
    return idx


# =========================================================================== async tasks


def test_reindex_async_returns_task_id(client, src, dest, tasks, node_id):
    tid = tasks("/_reindex", {"source": {"index": src}, "dest": {"index": dest}})
    assert re.fullmatch(re.escape(node_id) + r":[0-9]+", tid)
    assert re.fullmatch(r"[A-Za-z0-9_-]{22}", node_id)


@pytest.mark.parametrize("endpoint,body", [
    ("_update_by_query", None),
    ("_delete_by_query", {"query": {"match_all": {}}}),
    ("_forcemerge", None),
])
def test_async_endpoints_return_only_task(client, src, endpoint, body, node_id):
    r = client.post(f"/{src}/{endpoint}", json=body, params={"wait_for_completion": "false"})
    assert r.status_code == 200
    assert list(r.json()) == ["task"]
    tid = r.json()["task"]
    node, _, num = tid.partition(":")
    assert node == node_id and num.isdigit()
    _wait(client, tid)
    client.delete(f"/.tasks/_doc/{tid}")


def test_reindex_async_stored_result_shape(client, src, dest, tasks, node_id):
    tid = tasks("/_reindex", {"source": {"index": src, "size": 3}, "dest": {"index": dest}})
    body = _wait(client, tid)
    assert list(body) == ["completed", "task", "response"]
    task = body["task"]
    assert list(task) == TASK_KEYS
    assert task["node"] == node_id
    assert f"{task['node']}:{task['id']}" == tid
    assert task["type"] == "transport"
    assert task["action"] == "indices:data/write/reindex"
    assert task["description"] == f"reindex from [{src}] to [{dest}]"
    assert task["cancellable"] is True and task["cancelled"] is False
    assert task["headers"] == {}
    assert isinstance(task["start_time_in_millis"], int) and isinstance(task["running_time_in_nanos"], int)
    assert list(task["status"]) == STATUS_KEYS
    resp = body["response"]
    assert list(resp) == RESPONSE_KEYS
    assert {k: resp[k] for k in ("timed_out", "total", "updated", "created", "deleted", "batches",
                                 "version_conflicts", "noops", "retries", "failures")} == {
        "timed_out": False, "total": 10, "updated": 0, "created": 10, "deleted": 0, "batches": 4,
        "version_conflicts": 0, "noops": 0, "retries": {"bulk": 0, "search": 0}, "failures": []}
    assert resp["requests_per_second"] == -1.0
    assert resp["throttled"] == "0s" and resp["throttled_until"] == "0s"
    st = task["status"]
    assert (st["total"], st["created"], st["batches"]) == (10, 10, 4)


def test_reindex_async_copies_documents(client, src, dest, tasks):
    tid = tasks("/_reindex", {"source": {"index": src}, "dest": {"index": dest}})
    _wait(client, tid)
    assert _count(client, dest) == 10


def test_completed_task_get_without_wait_is_stable(client, src, dest, tasks):
    tid = tasks("/_reindex", {"source": {"index": src}, "dest": {"index": dest}})
    waited = _wait(client, tid)
    again = client.get(f"/_tasks/{tid}")
    assert again.status_code == 200
    assert again.json() == waited
    assert client.get(f"/_tasks/{tid}", params={"wait_for_completion": "true"}).json() == waited


def test_update_by_query_async_result(client, src, tasks):
    tid = tasks(f"/{src}/_update_by_query", scroll_size="4")
    body = _wait(client, tid)
    assert body["task"]["action"] == "indices:data/write/update/byquery"
    assert body["task"]["description"] == f"update-by-query [{src}]"
    resp = body["response"]
    assert list(resp) == RESPONSE_KEYS
    assert (resp["total"], resp["updated"], resp["created"], resp["batches"]) == (10, 10, 0, 3)


def test_delete_by_query_async_result(client, src, tasks):
    tid = tasks(f"/{src}/_delete_by_query", {"query": {"range": {"n": {"gte": 4}}}})
    body = _wait(client, tid)
    assert body["task"]["action"] == "indices:data/write/delete/byquery"
    assert body["task"]["description"] == f"delete-by-query [{src}]"
    resp = body["response"]
    assert list(resp) == RESPONSE_KEYS
    assert (resp["total"], resp["deleted"], resp["batches"], resp["failures"]) == (6, 6, 1, [])
    assert _count(client, src) == 4


def test_forcemerge_async_result(client, src, node_id):
    r = client.post(f"/{src}/_forcemerge", params={"wait_for_completion": "false"})
    tid = r.json()["task"]
    body = _wait(client, tid)
    client.delete(f"/.tasks/_doc/{tid}")
    task = body["task"]
    assert list(body) == ["completed", "task", "response"]
    assert "status" not in task
    assert task["action"] == "indices:admin/forcemerge"
    assert task["cancellable"] is False
    assert task["description"] == (f"Force-merge indices [{src}], maxSegments[-1], onlyExpungeDeletes[false], "
                                   "flush[true], primaryOnly[false]")
    assert body["response"] == {"_shards": {"total": 1, "successful": 1, "failed": 0}}


@pytest.mark.parametrize("scroll_size,batches", [("1", 10), ("3", 4), ("5", 2), ("1000", 1)])
def test_scroll_size_sets_batches(client, src, tasks, scroll_size, batches):
    tid = tasks(f"/{src}/_update_by_query", scroll_size=scroll_size)
    resp = _wait(client, tid)["response"]
    assert resp["batches"] == batches
    assert resp["updated"] == 10


def test_async_slices_result(client, src, tasks):
    tid = tasks(f"/{src}/_update_by_query", slices="2")
    body = _wait(client, tid)
    resp = body["response"]
    assert list(resp)[-2:] == ["slices", "failures"]
    assert [s["slice_id"] for s in resp["slices"]] == [0, 1]
    assert sum(s["updated"] for s in resp["slices"]) == 10
    assert resp["updated"] == 10
    assert "failures" not in resp["slices"][0]
    assert resp["slices"][0]["throttled"] == "0s"
    status = body["task"]["status"]
    assert list(status)[-1] == "slices"
    assert [s["slice_id"] for s in status["slices"]] == [0, 1]


def test_async_slices_auto_on_one_shard_has_no_slices(client, src, dest, tasks):
    tid = tasks("/_reindex", {"source": {"index": src}, "dest": {"index": dest}}, slices="auto")
    resp = _wait(client, tid)["response"]
    assert "slices" not in resp
    assert resp["created"] == 10


def test_async_requests_per_second_echoed(client, src, tasks):
    tid = tasks(f"/{src}/_update_by_query", requests_per_second="5000")
    body = _wait(client, tid)
    assert body["response"]["requests_per_second"] == 5000.0
    assert body["task"]["status"]["requests_per_second"] == 5000.0


def test_async_failure_is_stored_as_error(client, dest, tasks):
    missing = _uniq("slopensearch-t62-missing")
    tid = tasks("/_reindex", {"source": {"index": missing}, "dest": {"index": dest}})
    body = _wait(client, tid)
    assert list(body) == ["completed", "task", "error"]
    assert body["error"]["type"] == "index_not_found_exception"
    assert body["error"]["reason"] == f"no such index [{missing}]"
    assert body["error"]["index"] == missing
    assert body["task"]["status"]["total"] == 0


def test_task_headers_carry_opaque_id(client, src, dest, tasks):
    tid = tasks("/_reindex", {"source": {"index": src}, "dest": {"index": dest}},
                headers={"X-Opaque-Id": "t62-opaque"})
    body = _wait(client, tid)
    assert body["task"]["headers"] == {"X-Opaque-Id": "t62-opaque"}


# --------------------------------------------------------------- running tasks


def test_running_task_status(client, src, tasks):
    tid = tasks(f"/{src}/_update_by_query", requests_per_second="0.001", scroll_size="1")
    body = _wait_first_batch(client, tid)
    assert body["completed"] is False
    assert "response" not in body
    task = body["task"]
    assert task["cancellable"] is True and task["cancelled"] is False
    st = task["status"]
    assert (st["total"], st["updated"], st["batches"]) == (10, 1, 1)
    assert st["requests_per_second"] == 0.001
    assert st["throttled_until_millis"] > 0


def test_running_task_wait_timeout_504(client, src, tasks):
    tid = tasks(f"/{src}/_update_by_query", requests_per_second="0.001", scroll_size="1")
    _wait_first_batch(client, tid)
    r = client.get(f"/_tasks/{tid}", params={"wait_for_completion": "true", "timeout": "200ms"})
    assert r.status_code == 504
    assert _err(r)["type"] == "timeout_exception"
    assert _err(r)["reason"].startswith("Timed out waiting for completion of [")


def test_tasks_list_detailed_shows_running_reindex(client, src, dest, tasks, node_id):
    tid = tasks("/_reindex", {"source": {"index": src, "size": 1}, "dest": {"index": dest}},
                requests_per_second="0.001")
    _wait_first_batch(client, tid)
    r = client.get("/_tasks", params={"actions": "*reindex", "detailed": "true"})
    assert r.status_code == 200
    task = r.json()["nodes"][node_id]["tasks"][tid]
    assert task["action"] == "indices:data/write/reindex"
    assert task["description"] == f"reindex from [{src}] to [{dest}]"
    assert task["status"]["created"] == 1
    plain = client.get("/_tasks", params={"actions": "*reindex"}).json()["nodes"][node_id]["tasks"][tid]
    assert "description" not in plain
    assert "status" not in plain


def test_rethrottle_running_update_by_query(client, src, tasks, node_id):
    tid = tasks(f"/{src}/_update_by_query", requests_per_second="0.001", scroll_size="1")
    _wait_first_batch(client, tid)
    r = client.post(f"/_update_by_query/{tid}/_rethrottle", params={"requests_per_second": "0.0005"})
    assert r.status_code == 200
    body = r.json()
    assert list(body) == ["nodes"]
    task = body["nodes"][node_id]["tasks"][tid]
    assert task["action"] == "indices:data/write/update/byquery"
    assert task["status"]["requests_per_second"] == 0.0005
    st = client.get(f"/_tasks/{tid}").json()["task"]["status"]
    assert st["requests_per_second"] == 0.0005


def test_rethrottle_delete_by_query(client, src, tasks, node_id):
    tid = tasks(f"/{src}/_delete_by_query", {"query": {"match_all": {}}},
                requests_per_second="0.001", scroll_size="1")
    _wait_first_batch(client, tid)
    r = client.post(f"/_delete_by_query/{tid}/_rethrottle", params={"requests_per_second": "0.0005"})
    assert r.status_code == 200
    task = r.json()["nodes"][node_id]["tasks"][tid]
    assert task["status"]["requests_per_second"] == 0.0005
    assert task["status"]["deleted"] == 1


def test_rethrottle_unlimited_finishes_reindex(client, src, dest, tasks, node_id):
    tid = tasks("/_reindex", {"source": {"index": src, "size": 1}, "dest": {"index": dest}},
                requests_per_second="0.001")
    _wait_first_batch(client, tid)
    r = client.post(f"/_reindex/{tid}/_rethrottle", params={"requests_per_second": "-1"})
    assert r.status_code == 200
    assert r.json()["nodes"][node_id]["tasks"][tid]["status"]["requests_per_second"] == -1.0
    resp = _wait(client, tid)["response"]
    assert (resp["created"], resp["batches"], resp["requests_per_second"]) == (10, 10, -1.0)
    assert _count(client, dest) == 10


def test_cancel_running_task(client, src, tasks, node_id):
    tid = tasks(f"/{src}/_update_by_query", requests_per_second="0.001", scroll_size="1")
    _wait_first_batch(client, tid)
    r = client.post(f"/_tasks/{tid}/_cancel")
    assert r.status_code == 200
    task = r.json()["nodes"][node_id]["tasks"][tid]
    assert task["cancelled"] is True
    assert "status" not in task
    body = _wait(client, tid)
    assert body["task"]["cancelled"] is True
    resp = body["response"]
    assert resp["canceled"] == "by user request"
    assert (resp["updated"], resp["batches"]) == (1, 1)
    assert list(resp).index("canceled") == list(resp).index("requests_per_second") + 1


def test_rethrottle_finished_task_is_missing(client, src, dest, tasks, node_id):
    tid = tasks("/_reindex", {"source": {"index": src}, "dest": {"index": dest}})
    _wait(client, tid)
    r = client.post(f"/_reindex/{tid}/_rethrottle", params={"requests_per_second": "-1"})
    assert r.status_code == 200
    assert r.json() == {"node_failures": [{
        "type": "failed_node_exception", "reason": f"Failed node [{node_id}]", "node_id": node_id,
        "caused_by": {"type": "resource_not_found_exception", "reason": f"task [{tid}] is missing"}}],
        "nodes": {}}


# --------------------------------------------------------------- task errors


@pytest.mark.parametrize("endpoint", ["_reindex", "_update_by_query", "_delete_by_query"])
def test_rethrottle_unknown_task(client, node_id, endpoint):
    tid = f"{node_id}:999999999"
    r = client.post(f"/{endpoint}/{tid}/_rethrottle", params={"requests_per_second": "-1"})
    assert r.status_code == 200
    body = r.json()
    assert body["nodes"] == {}
    (failure,) = body["node_failures"]
    assert failure["caused_by"] == {"type": "resource_not_found_exception", "reason": f"task [{tid}] is missing"}


def test_rethrottle_requires_requests_per_second(client, node_id):
    r = client.post(f"/_reindex/{node_id}:1/_rethrottle")
    assert r.status_code == 400
    assert _err(r)["type"] == "illegal_argument_exception"
    assert _err(r)["reason"] == "requests_per_second is a required parameter"


@pytest.mark.parametrize("rps", ["0", "abc"])
def test_rethrottle_invalid_requests_per_second(client, node_id, rps):
    r = client.post(f"/_reindex/{node_id}:1/_rethrottle", params={"requests_per_second": rps})
    assert r.status_code == 400
    assert _err(r)["type"] == "illegal_argument_exception"
    assert _err(r)["reason"] == RPS_REASON
    if rps == "abc":
        assert _err(r)["caused_by"] == {"type": "number_format_exception", "reason": 'For input string: "abc"'}
    else:
        assert "caused_by" not in _err(r)


def test_rethrottle_malformed_task_id(client):
    r = client.post("/_delete_by_query/bogus/_rethrottle", params={"requests_per_second": "-1"})
    assert r.status_code == 400
    assert _err(r)["reason"] == "malformed task id bogus"


def test_cancel_unknown_task(client, node_id):
    tid = f"{node_id}:999999999"
    r = client.post(f"/_tasks/{tid}/_cancel")
    assert r.status_code == 200
    assert r.json() == {"node_failures": [{
        "type": "failed_node_exception", "reason": f"Failed node [{node_id}]", "node_id": node_id,
        "caused_by": {"type": "resource_not_found_exception", "reason": f"task [{tid}] is not found"}}],
        "nodes": {}}


def test_cancel_malformed_task_id(client):
    r = client.post("/_tasks/bogus/_cancel")
    assert r.status_code == 400
    assert _err(r)["type"] == "illegal_argument_exception"
    assert _err(r)["reason"] == "malformed task id bogus"


@pytest.mark.parametrize("suffix,bad", [("abc", "abc"), ("", "")])
def test_task_id_non_numeric(client, node_id, suffix, bad):
    r = client.get(f"/_tasks/{node_id}:{suffix}")
    assert r.status_code == 400
    assert _err(r)["reason"] == f"malformed task id {node_id}:{suffix}"
    assert _err(r)["caused_by"] == {"type": "number_format_exception", "reason": f'For input string: "{bad}"'}


def test_task_id_too_many_colons(client):
    r = client.get("/_tasks/a:b:c")
    assert r.status_code == 400
    assert _err(r)["reason"] == "malformed task id a:b:c"
    assert _err(r)["caused_by"]["reason"] == 'For input string: "b:c"'


def test_task_id_empty_node(client):
    r = client.get("/_tasks/:12")
    assert r.status_code == 404
    err = _err(r)
    assert err["type"] == "resource_not_found_exception"
    assert err["reason"] == ("task [:12] belongs to the node [] which isn't part of the cluster "
                             "and there is no record of the task")
    assert err["root_cause"][0]["reason"] == "task [:12] isn't running and hasn't stored its results"
    assert err["caused_by"] == err["root_cause"][0]


def test_task_id_negative(client, node_id):
    r = client.get(f"/_tasks/{node_id}:-5")
    assert r.status_code == 404
    assert _err(r)["reason"] == f"task [{node_id}:-5] isn't running and hasn't stored its results"


def test_unknown_task_wait_for_completion_404(client, node_id):
    r = client.get(f"/_tasks/{node_id}:999999999", params={"wait_for_completion": "true"})
    assert r.status_code == 404
    assert _err(r)["type"] == "resource_not_found_exception"


@pytest.mark.parametrize("rps", ["0", "-2", "abc"])
def test_async_invalid_requests_per_second(client, src, rps):
    r = client.post(f"/{src}/_update_by_query", params={"wait_for_completion": "false", "requests_per_second": rps})
    assert r.status_code == 400
    assert _err(r)["reason"] == RPS_REASON


@pytest.mark.parametrize("slices,cause", [("0", False), ("x", True)])
def test_async_invalid_slices(client, src, dest, slices, cause):
    r = client.post("/_reindex", json={"source": {"index": src}, "dest": {"index": dest}},
                    params={"wait_for_completion": "false", "slices": slices})
    assert r.status_code == 400
    assert _err(r)["reason"] == f'[slices] must be a positive integer or the string "auto", but was [{slices}]'
    assert ("caused_by" in _err(r)) is cause


def test_scroll_size_zero_rejected(client, src):
    r = client.post(f"/{src}/_update_by_query", params={"wait_for_completion": "false", "scroll_size": "0"})
    assert r.status_code == 400
    assert _err(r)["type"] == "action_request_validation_exception"
    assert _err(r)["reason"] == "Validation Failed: 1: [size] cannot be [0] in a scroll context;"


def test_wait_for_completion_invalid_boolean(client, src):
    r = client.post(f"/{src}/_update_by_query", params={"wait_for_completion": "bogus"})
    assert r.status_code == 400
    assert _err(r)["reason"] == "Failed to parse value [bogus] as only [true] or [false] are allowed."


# =========================================================================== HTTP: compression


def test_gzip_request_body_index(client, idx):
    r = client.put(f"/{idx}/_doc/1", content=gzip.compress(b'{"a":"hello"}'),
                   headers={**JSON, "Content-Encoding": "gzip"}, params={"refresh": "true"})
    assert r.status_code == 201
    assert client.get(f"/{idx}/_source/1").json() == {"a": "hello"}


def test_gzip_request_body_search(client, doc):
    r = client.post(f"/{doc}/_search", content=gzip.compress(b'{"query":{"match":{"a":"x"}}}'),
                    headers={**JSON, "Content-Encoding": "gzip"})
    assert r.status_code == 200
    assert [h["_id"] for h in r.json()["hits"]["hits"]] == ["1"]


def test_gzip_request_body_bulk(client, idx):
    payload = b'{"index":{"_id":"g1"}}\n{"a":1}\n{"index":{"_id":"g2"}}\n{"a":2}\n'
    r = client.post(f"/{idx}/_bulk", content=gzip.compress(payload),
                    headers={**NDJSON, "Content-Encoding": "gzip"}, params={"refresh": "true"})
    assert r.status_code == 200
    assert [i["index"]["status"] for i in r.json()["items"]] == [201, 201]
    assert _count(client, idx) == 2


def test_deflate_request_body(client, doc):
    r = client.post(f"/{doc}/_count", content=zlib.compress(b'{"query":{"term":{"n":1}}}'),
                    headers={**JSON, "Content-Encoding": "deflate"})
    assert r.status_code == 200
    assert r.json()["count"] == 1


@pytest.mark.parametrize("accept,encoding", [
    ("identity", None), ("gzip", "gzip"), ("deflate", "deflate"), ("gzip, deflate", "gzip"),
    ("br", None), ("zstd", None), ("gzip;q=0", None),
])
def test_response_compression(client, doc, accept, encoding):
    r = client.get(f"/{doc}/_doc/1", headers={"Accept-Encoding": accept})
    assert r.status_code == 200
    assert r.headers.get("content-encoding") == encoding
    assert r.json()["_source"] == {"a": "x", "n": 1}


def test_response_compression_wildcard_is_snappy(client, doc):
    r = client.get(f"/{doc}/_doc/1", headers={"Accept-Encoding": "*"})
    assert r.status_code == 200
    assert r.headers["content-encoding"] == "snappy"
    assert r.content.startswith(b"\xff\x06\x00\x00sNaPpY")


def test_gzip_response_length_is_compressed_size(client, doc):
    plain = client.get(f"/{doc}/_doc/1", headers=PLAIN)
    with httpx.Client(base_url=str(client.base_url), timeout=30) as raw:
        with raw.stream("GET", f"/{doc}/_doc/1", headers={"Accept-Encoding": "gzip"}) as r:
            wire = b"".join(r.iter_raw())
            assert int(r.headers["content-length"]) == len(wire)
    assert gzip.decompress(wire) == plain.content
    assert int(plain.headers["content-length"]) == len(plain.content)


def test_max_content_length_413(client):
    head = _raw(client, b"POST /_search HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\n"
                b"Content-Length: 104857601\r\n\r\n")
    status_line, _, headers = head.partition(b"\r\n")
    assert status_line == b"HTTP/1.1 413 Request Entity Too Large"
    assert b"content-length: 0" in headers.lower()


# =========================================================================== HTTP: content type


@pytest.mark.parametrize("ctype", [None, "text/plain", "text/plain; charset=UTF-8", "*/*",
                                   "application/x-www-form-urlencoded"])
@pytest.mark.parametrize("method,path", [("PUT", "/{i}/_doc/1"), ("POST", "/{i}/_search")])
def test_unsupported_content_type_406(client, idx, method, path, ctype):
    headers = {"Content-Type": ctype} if ctype else {}
    r = client.request(method, path.format(i=idx), content=b'{"a":1}', headers=headers)
    assert r.status_code == 406
    assert r.json() == {"error": f"Content-Type header [{ctype or ''}] is not supported", "status": 406}


def test_bulk_text_plain_406(client, idx):
    r = client.post(f"/{idx}/_bulk", content=b'{"index":{}}\n{"a":1}\n', headers={"Content-Type": "text/plain"})
    assert r.status_code == 406
    assert r.json() == {"error": "Content-Type header [text/plain] is not supported", "status": 406}


def test_body_less_search_needs_no_content_type(client, doc):
    r = client.post(f"/{doc}/_search", content=b"")
    assert r.status_code == 200
    assert r.json()["hits"]["total"]["value"] == 1


@pytest.mark.parametrize("ctype", [
    "application/json;charset=utf-8", "application/json; charset=ISO-8859-1", "application/JSON",
    "application/json; foo=bar", "application/json;", "application/vnd.opensearch+json;compatible-with=7",
])
def test_accepted_json_content_types(client, idx, ctype):
    r = client.put(f"/{idx}/_doc/1", content=b'{"a":"x"}', headers={"Content-Type": ctype})
    assert r.status_code == 201
    assert r.headers["content-type"] == "application/json; charset=UTF-8"


def test_charset_parameter_is_ignored(client, idx):
    r = client.put(f"/{idx}/_doc/1", content='{"a":"é€"}'.encode(),
                   headers={"Content-Type": "application/json; charset=ISO-8859-1"})
    assert r.status_code == 201
    assert client.get(f"/{idx}/_source/1").json() == {"a": "é€"}


def test_latin1_bytes_are_invalid_utf8(client, idx):
    r = client.put(f"/{idx}/_doc/1", content='{"a":"é"}'.encode("latin-1"),
                   headers={"Content-Type": "application/json; charset=ISO-8859-1"})
    assert r.status_code == 400
    assert _err(r)["type"] == "mapper_parsing_exception"
    assert _err(r)["caused_by"]["type"] == "json_parse_exception"


def test_malformed_content_type_400(client, idx):
    r = client.put(f"/{idx}/_doc/1", content=b'{"a":1}', headers={"Content-Type": "bogus"})
    assert r.status_code == 400
    assert r.json() == {"error": {
        "root_cause": [{"type": "content_type_header_exception",
                        "reason": "java.lang.IllegalArgumentException: invalid Content-Type header [bogus]"}],
        "type": "content_type_header_exception",
        "reason": "java.lang.IllegalArgumentException: invalid Content-Type header [bogus]",
        "caused_by": {"type": "illegal_argument_exception", "reason": "invalid Content-Type header [bogus]"}},
        "status": 400}


# =========================================================================== HTTP: methods, HEAD


def test_head_doc_has_get_content_length(client, doc):
    get = client.get(f"/{doc}/_doc/1", headers=PLAIN)
    for accept in ("identity", "gzip"):
        r = client.head(f"/{doc}/_doc/1", headers={"Accept-Encoding": accept})
        assert r.status_code == 200
        assert r.content == b""
        assert "content-encoding" not in r.headers
        assert int(r.headers["content-length"]) == len(get.content)
        assert r.headers["content-type"] == "application/json; charset=UTF-8"


def test_head_missing_doc_has_get_content_length(client, doc):
    get = client.get(f"/{doc}/_doc/nope", headers=PLAIN)
    assert get.status_code == 404
    r = client.head(f"/{doc}/_doc/nope", headers=PLAIN)
    assert r.status_code == 404
    assert r.content == b""
    assert int(r.headers["content-length"]) == len(get.content)


def test_head_index_has_get_content_length(client, idx):
    get = client.get(f"/{idx}", headers=PLAIN)
    r = client.head(f"/{idx}", headers=PLAIN)
    assert r.status_code == 200
    assert r.content == b""
    assert int(r.headers["content-length"]) == len(get.content)


@pytest.mark.parametrize("method", ["PATCH", "TRACE", "CONNECT"])
def test_doc_wrong_method_405(client, doc, method):
    r = client.request(method, f"/{doc}/_doc/1")
    assert r.status_code == 405
    assert set(r.headers["allow"].split(",")) == {"POST", "GET", "DELETE", "PUT", "HEAD"}
    body = r.json()
    assert body["status"] == 405
    assert body["error"].startswith(f"Incorrect HTTP method for uri [/{doc}/_doc/1] and method [{method}], allowed: [")


def test_doc_options(client, doc):
    r = client.options(f"/{doc}/_doc/1")
    assert r.status_code == 200
    assert r.content == b""
    assert set(r.headers["allow"].split(",")) == {"POST", "GET", "DELETE", "PUT", "HEAD"}
    assert r.headers["content-type"] == "text/plain; charset=UTF-8"


@pytest.mark.parametrize("bad", ["%", "%zz"])
def test_invalid_percent_escape_is_405(client, doc, bad):
    r = client.get(f"/{doc}/_doc/{bad}")
    assert r.status_code == 405
    body = r.json()
    assert list(body) == ["error", "status"] and body["status"] == 405
    prefix, _, allowed = body["error"].partition("allowed: [")
    assert prefix == "Unexpected HTTP method, "
    assert set(allowed.rstrip("]").split(", ")) == {"POST", "GET", "DELETE", "PUT", "HEAD"}


# =========================================================================== HTTP: paths


@pytest.mark.parametrize("path", ["/{i}/", "/{i}/_doc/1/", "/_cluster/health/", "/{i}/_count/"])
def test_trailing_slash_is_ignored(client, doc, path):
    r = client.get(path.format(i=doc))
    assert r.status_code == 200


@pytest.mark.parametrize("path", ["/{i}//_doc/1", "/{i}/_doc//1", "/_cluster//health"])
def test_double_slash_has_no_handler(client, doc, path):
    p = path.format(i=doc)
    r = client.get(p)
    assert r.status_code == 400
    assert r.json() == {"error": f"no handler found for uri [{p}] and method [GET]"}


@pytest.mark.parametrize("encoded,decoded,location", [
    ("a%2Fb", "a/b", "a%2Fb"),
    ("a%20b", "a b", "a+b"),
    ("%C3%A9%E2%82%AC", "é€", "%C3%A9%E2%82%AC"),
    ("a%3Fb", "a?b", "a%3Fb"),
    ("a%23b", "a#b", "a%23b"),
    ("%25", "%", "%25"),
    ("a+b", "a+b", "a%2Bb"),
])
def test_encoded_doc_ids(client, idx, encoded, decoded, location):
    r = client.put(f"/{idx}/_doc/{encoded}", json={"x": 1})
    assert r.status_code == 201
    assert r.json()["_id"] == decoded
    assert r.headers["location"] == f"/{idx}/_doc/{location}"
    g = client.get(f"/{idx}/_doc/{encoded}")
    assert g.status_code == 200 and g.json()["_id"] == decoded


def test_plus_in_path_is_not_a_space(client, idx):
    assert client.put(f"/{idx}/_doc/a+b", json={"x": 1}).status_code == 201
    assert client.get(f"/{idx}/_doc/a%2Bb").json()["found"] is True
    assert client.get(f"/{idx}/_doc/a%20b").json()["found"] is False


def test_unencoded_slash_in_id_has_no_handler(client, idx):
    r = client.get(f"/{idx}/_doc/a/b")
    assert r.status_code == 400
    assert r.json() == {"error": f"no handler found for uri [/{idx}/_doc/a/b] and method [GET]"}


def test_percent_encoded_index_name(client, doc):
    encoded = "".join(f"%{ord(ch):02X}" for ch in doc)
    r = client.get(f"/{encoded}/_doc/1")
    assert r.status_code == 200
    assert r.json()["_index"] == doc


def test_encoded_comma_splits_indices(client, doc):
    missing = _uniq("slopensearch-t62-missing")
    r = client.get(f"/{doc}%2C{missing}/_count")
    assert r.status_code == 404
    assert _err(r)["reason"] == f"no such index [{missing}]"


def test_unicode_index_name_in_error(client):
    r = client.get("/%C3%A9t62-missing")
    assert r.status_code == 404
    assert _err(r)["reason"] == "no such index [ét62-missing]"
    assert _err(r)["index"] == "ét62-missing"


# =========================================================================== HTTP: query string


def test_unknown_parameter(client, doc):
    r = client.get(f"/{doc}/_doc/1", params={"bogus": "1"})
    assert r.status_code == 400
    assert r.json() == {"error": {
        "root_cause": [{"type": "illegal_argument_exception",
                        "reason": f"request [/{doc}/_doc/1] contains unrecognized parameter: [bogus]"}],
        "type": "illegal_argument_exception",
        "reason": f"request [/{doc}/_doc/1] contains unrecognized parameter: [bogus]"}, "status": 400}


def test_unknown_parameters_plural(client, doc):
    r = client.get(f"/{doc}/_doc/1?a=1&&b")
    assert r.status_code == 400
    assert _err(r)["reason"] == f"request [/{doc}/_doc/1] contains unrecognized parameters: [a], [b]"


def test_unknown_parameter_did_you_mean(client, doc):
    r = client.get(f"/{doc}/_doc/1", params={"_sourc": "1"})
    assert r.status_code == 400
    assert _err(r)["reason"] == f"request [/{doc}/_doc/1] contains unrecognized parameter: [_sourc] -> did you mean [_source]?"


def test_empty_parameter_name_uses_value(client, doc):
    r = client.get(f"/{doc}/_doc/1?=x")
    assert r.status_code == 400
    assert _err(r)["reason"] == f"request [/{doc}/_doc/1] contains unrecognized parameter: [x]"


@pytest.mark.parametrize("query,expected", [
    ("_source=false&_source=true", {"a": "x", "n": 1}),
    ("_source=true&_source=false", None),
    ("_source_includes=a&_source_includes=n", {"n": 1}),
])
def test_repeated_parameter_last_wins(client, doc, query, expected):
    body = client.get(f"/{doc}/_doc/1?{query}").json()
    assert body.get("_source") == expected


def test_repeated_filter_path_last_wins(client, doc):
    r = client.get(f"/{doc}/_doc/1?filter_path=_id&filter_path=_index")
    assert r.json() == {"_index": doc}


@pytest.mark.parametrize("param,value", [("realtime", "yes"), ("realtime", "False"), ("pretty", "bogus"),
                                         ("human", "bogus"), ("error_trace", "1")])
def test_strict_boolean_parameters(client, doc, param, value):
    r = client.get(f"/{doc}/_doc/1", params={param: value})
    assert r.status_code == 400
    assert _err(r)["type"] == "illegal_argument_exception"
    assert _err(r)["reason"] == f"Failed to parse value [{value}] as only [true] or [false] are allowed."


@pytest.mark.parametrize("query", ["realtime=", "realtime", "refresh=", "refresh"])
def test_empty_boolean_parameter_means_true(client, doc, query):
    r = client.get(f"/{doc}/_doc/1?{query}")
    assert r.status_code == 200
    assert r.json()["found"] is True


@pytest.mark.parametrize("query", ["pretty", "pretty=", "pretty=false&pretty"])
def test_bare_pretty_is_pretty(client, doc, query):
    r = client.get(f"/{doc}/_doc/1?{query}&filter_path=_id")
    assert r.text == '{\n  "_id" : "1"\n}\n'


@pytest.mark.parametrize("value,expected", [
    ("false", None), ("true", {"a": "x", "n": 1}),
    ("FALSE", {}), ("0", {}), ("no", {}), ("n", {"n": 1}), ("a,n", {"a": "x", "n": 1}),
])
def test_source_param_only_exact_booleans(client, doc, value, expected):
    # anything but exactly "true"/"false" is a list of field patterns
    r = client.get(f"/{doc}/_doc/1", params={"_source": value})
    assert r.status_code == 200
    assert r.json().get("_source") == expected


def test_pretty_does_not_reformat_source(client, doc):
    r = client.get(f"/{doc}/_doc/1", params={"pretty": "true"})
    assert r.text.startswith('{\n  "_index" : ')
    assert r.text.endswith('  "found" : true,\n  "_source":{"a":"x","n":1}\n}\n')


def test_pretty_with_filter_path_reformats_source(client, doc):
    r = client.get(f"/{doc}/_doc/1", params={"pretty": "true", "filter_path": "_source.a"})
    assert r.text == '{\n  "_source" : {\n    "a" : "x"\n  }\n}\n'


def test_pretty_error_response(client):
    r = client.get("/%C3%A9t62-missing", params={"pretty": "true"})
    assert r.status_code == 404
    assert r.text.startswith('{\n  "error" : {\n    "root_cause" : [\n      {\n        "type" : "index_not_found_exception",')
    assert r.text.endswith('  "status" : 404\n}\n')


@pytest.mark.parametrize("fp,expected", [
    ("_source.a", {"_source": {"a": "x"}}),
    ("-_source,-_seq_no,-_primary_term", {"_index": None, "_id": "1", "_version": 1, "found": True}),
    ("", None),
])
def test_filter_path_on_get(client, doc, fp, expected):
    body = client.get(f"/{doc}/_doc/1", params={"filter_path": fp}).json()
    if expected is None:
        assert body["_source"] == {"a": "x", "n": 1} and body["found"] is True
    else:
        if "_index" in expected:
            expected["_index"] = doc
        assert body == expected


def test_human_on_index_stats(client, doc):
    fp = "_all.primaries.docs,_all.primaries.store.size*"
    plain = client.get(f"/{doc}/_stats/store,docs", params={"filter_path": fp}).json()
    human = client.get(f"/{doc}/_stats/store,docs", params={"filter_path": fp, "human": "true"}).json()
    assert set(plain["_all"]["primaries"]["store"]) == {"size_in_bytes"}
    store = human["_all"]["primaries"]["store"]
    assert list(store) == ["size", "size_in_bytes"]
    assert re.fullmatch(r"[0-9.]+k?b", store["size"])
    assert human["_all"]["primaries"]["docs"] == plain["_all"]["primaries"]["docs"]


def test_human_on_index_settings(client, idx):
    fp = "*.settings.index.creation_date*,*.settings.index.version"
    plain = client.get(f"/{idx}/_settings", params={"filter_path": fp}).json()[idx]["settings"]["index"]
    human = client.get(f"/{idx}/_settings", params={"filter_path": fp, "human": "true"}).json()[idx]["settings"]["index"]
    assert set(plain) == {"creation_date", "version"} and set(plain["version"]) == {"created"}
    assert list(human) == ["creation_date_string", "creation_date", "version"]
    assert list(human["version"]) == ["created_string", "created"]
    assert human["version"]["created_string"] == "3.9.0"
    assert re.fullmatch(r"\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z", human["creation_date_string"])


def test_yaml_format_on_get(client, doc):
    r = client.get(f"/{doc}/_doc/1", params={"format": "yaml", "filter_path": "_id,found,_source"})
    assert r.headers["content-type"].startswith("application/yaml")
    assert r.text == '---\n_id: "1"\nfound: true\n_source:\n  a: "x"\n  "n": 1\n'


def test_error_trace_on_not_found(client):
    r = client.get("/slopensearch-t62-missing-index", params={"error_trace": "true"})
    assert r.status_code == 404
    err = _err(r)
    assert list(err)[-1] == "stack_trace"
    assert list(err["root_cause"][0])[-1] == "stack_trace"
    assert err["stack_trace"].startswith("[slopensearch-t62-missing-index] IndexNotFoundException[no such index")
    r = client.get("/slopensearch-t62-missing-index", params={"error_trace": "false"})
    assert "stack_trace" not in _err(r)


def test_error_trace_on_success_changes_nothing(client, doc):
    a = client.get(f"/{doc}/_doc/1", params={"error_trace": "true"})
    b = client.get(f"/{doc}/_doc/1")
    assert a.status_code == 200 and a.json() == b.json()


# =========================================================================== HTTP: X-Opaque-Id


@pytest.mark.parametrize("method,path,status", [
    ("GET", "/{i}/_doc/1", 200), ("GET", "/{i}/_doc/nope", 404), ("HEAD", "/{i}/_doc/1", 200),
    ("GET", "/{i}/_doc/1?bogus=1", 400), ("POST", "/{i}/_search", 200),
])
def test_opaque_id_echoed(client, doc, method, path, status):
    r = client.request(method, path.format(i=doc), headers={"X-Opaque-Id": "t62-req-42"})
    assert r.status_code == status
    assert r.headers["x-opaque-id"] == "t62-req-42"


def test_no_opaque_id_header_by_default(client, doc):
    assert "x-opaque-id" not in client.get(f"/{doc}/_doc/1").headers


def test_long_opaque_id_echoed(client, doc):
    value = "o" * 2000
    r = client.get(f"/{doc}/_doc/1", headers={"X-Opaque-Id": value})
    assert r.headers["x-opaque-id"] == value


def test_duplicate_opaque_id_400(client, doc):
    r = client.get(f"/{doc}/_doc/1", headers=[("X-Opaque-Id", "a"), ("X-Opaque-Id", "b")])
    assert r.status_code == 400
    assert r.json() == {"error": "multiple values for single-valued header [X-Opaque-Id].", "status": 400}
    assert r.headers.get_list("x-opaque-id") == ["a"]


# =========================================================================== HTTP: nesting, long names


def _nested_obj(depth):
    return '{"q":' + '{"a":' * (depth - 1) + "1" + "}" * (depth - 1) + "}"


def test_object_depth_limit(client, index_factory):
    ok = index_factory(settings=SINGLE)
    assert client.put(f"/{ok}/_doc/1", content=_nested_obj(20), headers=JSON).status_code == 201
    bad = index_factory(settings=SINGLE)
    r = client.put(f"/{bad}/_doc/1", content=_nested_obj(21), headers=JSON)
    assert r.status_code == 400
    reason = ("The depth of the field has exceeded the allowed limit of [20]. This limit can be set by "
              "changing the [index.mapping.depth.limit] index level setting.")
    assert r.json() == {"error": {
        "root_cause": [{"type": "parse_exception", "reason": reason}],
        "type": "mapper_parsing_exception", "reason": "failed to parse",
        "caused_by": {"type": "parse_exception", "reason": reason}}, "status": 400}


def test_array_depth_limit(client, index_factory):
    ok = index_factory(settings=SINGLE)
    doc = '{"a":' + "[" * 20 + "1" + "]" * 20 + "}"
    assert client.put(f"/{ok}/_doc/1", content=doc, headers=JSON).status_code == 201
    bad = index_factory(settings=SINGLE)
    doc = '{"a":' + "[" * 21 + "1" + "]" * 21 + "}"
    r = client.put(f"/{bad}/_doc/1", content=doc, headers=JSON)
    assert r.status_code == 400
    assert _err(r)["type"] == "mapper_parsing_exception"
    assert _err(r)["caused_by"]["reason"].startswith(
        "The depth of the nested array field has exceeded the allowed limit of [20].")


def test_nested_bool_query_20_levels(client, doc):
    q = {"term": {"n": 1}}
    for _ in range(20):
        q = {"bool": {"must": [q]}}
    r = client.post(f"/{doc}/_search", json={"query": q})
    assert r.status_code == 200
    assert r.json()["hits"]["total"]["value"] == 1


@pytest.mark.parametrize("length", [1000, 40000])
def test_long_field_name(client, idx, length):
    name = "f" * length
    r = client.put(f"/{idx}/_doc/1", json={name: "v"}, params={"refresh": "true"})
    assert r.status_code == 201
    assert client.get(f"/{idx}/_source/1").json() == {name: "v"}
    props = client.get(f"/{idx}/_mapping").json()[idx]["mappings"]["properties"]
    assert list(props) == [name]
    r = client.post(f"/{idx}/_count", json={"query": {"term": {f"{name}.keyword": "v"}}})
    assert r.json()["count"] == 1


@pytest.mark.parametrize("field,reason", [
    ("", "field name cannot be an empty string"),
    (".a", "object field starting or ending with a [.] makes object resolution ambiguous: [.a]"),
    ("a..b", "object field starting or ending with a [.] makes object resolution ambiguous: [a..b]"),
])
def test_invalid_field_names(client, idx, field, reason):
    r = client.put(f"/{idx}/_doc/1", json={field: 1})
    assert r.status_code == 400
    assert _err(r)["type"] == "mapper_parsing_exception"
    assert _err(r)["caused_by"] == {"type": "illegal_argument_exception", "reason": reason}
