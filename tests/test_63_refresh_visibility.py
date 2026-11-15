"""What searches see between refreshes (refresh_interval -1, so no timing dependence).

- Searches read the last refreshed point in time: a document deleted since is still
  found, counted and aggregated until the next refresh; a document overwritten since is
  found in its refreshed version (with that version's `_version` / `_seq_no`).
- Realtime reads (GET, mget) see the latest write: a deleted document is not found.
  `realtime=false` reads the refreshed version.
- A point in time keeps the view it was opened on, also across a refresh.
- `_delete_by_query` / `_update_by_query` work on the search hits: a hit whose document
  was deleted or overwritten since the refresh is a version conflict ("but no document
  was found" for a deleted one).
- `refresh=true` on a by-query request refreshes the index only when the request wrote
  something: when every hit conflicted (or nothing matched) the index stays unrefreshed.
"""
import pytest

pytestmark = pytest.mark.documents

SETTINGS = {"number_of_shards": 1, "number_of_replicas": 0, "refresh_interval": "-1"}
MAPPINGS = {"properties": {"user_id": {"type": "keyword"}, "quantity": {"type": "integer"}}}


@pytest.fixture
def idx(client, index_factory):
    name = index_factory(settings=SETTINGS, mappings=MAPPINGS)
    lines = []
    for i in range(1, 7):
        lines.append(f'{{"index":{{"_id":"{i}"}}}}')
        lines.append(f'{{"user_id":"{"u1" if i < 5 else "u2"}","quantity":{i}}}')
    r = client.post(f"/{name}/_bulk", params={"refresh": "true"}, content="\n".join(lines) + "\n",
                    headers={"Content-Type": "application/x-ndjson"})
    assert r.status_code == 200 and not r.json()["errors"], r.text
    return name


def _hits(client, name, query=None):
    body = {"sort": ["quantity"], "_source": ["quantity"]}
    if query:
        body["query"] = query
    r = client.post(f"/{name}/_search", json=body)
    assert r.status_code == 200, r.text
    return [(h["_id"], h["_source"]["quantity"]) for h in r.json()["hits"]["hits"]]


def _count(client, name, query=None):
    r = client.post(f"/{name}/_count", json={"query": query} if query else {})
    assert r.status_code == 200, r.text
    return r.json()["count"]


def _by_query(client, name, op, query, **params):
    r = client.post(f"/{name}/_{op}_by_query", params=params, json={"query": query})
    body = r.json()
    keys = ("total", "deleted", "batches", "version_conflicts", "noops")
    return r.status_code, {k: body[k] for k in keys}, body


ALL_SIX = [("1", 1), ("2", 2), ("3", 3), ("4", 4), ("5", 5), ("6", 6)]


# ------------------------------------------------------------ deleted, not refreshed


def test_deleted_documents_stay_searchable_until_refresh(client, idx):
    assert client.delete(f"/{idx}/_doc/3").status_code == 200
    assert client.delete(f"/{idx}/_doc/5").status_code == 200
    assert _hits(client, idx) == ALL_SIX
    assert _count(client, idx) == 6
    assert _count(client, idx, {"term": {"user_id": "u1"}}) == 4
    r = client.post(f"/{idx}/_search", json={"query": {"ids": {"values": ["3"]}},
                                             "version": True, "seq_no_primary_term": True})
    hit = r.json()["hits"]["hits"][0]
    assert (hit["_id"], hit["_version"], hit["_seq_no"], hit["_source"]) == (
        "3", 1, 2, {"user_id": "u1", "quantity": 3})
    assert client.post(f"/{idx}/_refresh").status_code == 200
    assert _hits(client, idx) == [("1", 1), ("2", 2), ("4", 4), ("6", 6)]
    assert _count(client, idx) == 4


def test_deleted_documents_are_aggregated_until_refresh(client, idx):
    assert client.delete(f"/{idx}/_doc/2").status_code == 200
    assert client.delete(f"/{idx}/_doc/6").status_code == 200
    body = {"size": 0, "aggs": {"s": {"sum": {"field": "quantity"}}, "t": {"terms": {"field": "user_id"}}}}
    r = client.post(f"/{idx}/_search", json=body)
    assert r.status_code == 200, r.text
    a = r.json()["aggregations"]
    assert r.json()["hits"]["total"]["value"] == 6
    assert a["s"]["value"] == 21.0
    assert [(b["key"], b["doc_count"]) for b in a["t"]["buckets"]] == [("u1", 4), ("u2", 2)]
    assert client.post(f"/{idx}/_refresh").status_code == 200
    a = client.post(f"/{idx}/_search", json=body).json()["aggregations"]
    assert a["s"]["value"] == 13.0
    assert [(b["key"], b["doc_count"]) for b in a["t"]["buckets"]] == [("u1", 3), ("u2", 1)]


def test_realtime_reads_do_not_find_a_deleted_document(client, idx):
    assert client.delete(f"/{idx}/_doc/3").status_code == 200
    assert client.post(f"/{idx}/_update/1", json={"doc": {"quantity": 10}}).status_code == 200
    r = client.get(f"/{idx}/_doc/3")
    assert r.status_code == 404 and r.json() == {"_index": idx, "_id": "3", "found": False}
    r = client.post(f"/{idx}/_mget", json={"ids": ["3", "1"]})
    docs = r.json()["docs"]
    assert docs[0] == {"_index": idx, "_id": "3", "found": False}
    assert (docs[1]["_version"], docs[1]["_source"]["quantity"]) == (2, 10)
    assert client.head(f"/{idx}/_doc/3").status_code == 404


def test_non_realtime_get_reads_the_refreshed_version(client, idx):
    assert client.delete(f"/{idx}/_doc/3").status_code == 200
    assert client.post(f"/{idx}/_update/1", json={"doc": {"quantity": 10}}).status_code == 200
    r = client.get(f"/{idx}/_doc/3", params={"realtime": "false"})
    assert r.status_code == 200, r.text
    assert (r.json()["_version"], r.json()["_seq_no"], r.json()["_source"]) == (
        1, 2, {"user_id": "u1", "quantity": 3})
    r = client.get(f"/{idx}/_doc/1", params={"realtime": "false"})
    assert (r.json()["_version"], r.json()["_source"]) == (1, {"user_id": "u1", "quantity": 1})
    r = client.post(f"/{idx}/_mget", params={"realtime": "false"}, json={"ids": ["3", "1"]})
    assert [(d["_id"], d["found"], d["_version"], d["_source"]["quantity"]) for d in r.json()["docs"]] == [
        ("3", True, 1, 3), ("1", True, 1, 1)]


def test_deleted_then_recreated_document_shows_the_refreshed_version(client, idx):
    assert client.delete(f"/{idx}/_doc/4").status_code == 200
    assert client.put(f"/{idx}/_doc/4", json={"user_id": "u2", "quantity": 40}).status_code == 201
    assert _hits(client, idx, {"term": {"user_id": "u1"}}) == [("1", 1), ("2", 2), ("3", 3), ("4", 4)]
    assert _count(client, idx, {"term": {"user_id": "u2"}}) == 2
    assert client.get(f"/{idx}/_doc/4").json()["_source"]["quantity"] == 40
    assert client.post(f"/{idx}/_refresh").status_code == 200
    assert _hits(client, idx, {"term": {"user_id": "u2"}}) == [("5", 5), ("6", 6), ("4", 40)]


def test_scroll_and_point_in_time_see_unrefreshed_deletes(client, idx):
    assert client.delete(f"/{idx}/_doc/2").status_code == 200
    r = client.post(f"/{idx}/_search", params={"scroll": "1m"},
                    json={"size": 4, "sort": ["quantity"], "_source": False})
    assert r.status_code == 200, r.text
    first = r.json()
    assert first["hits"]["total"]["value"] == 6
    assert [h["_id"] for h in first["hits"]["hits"]] == ["1", "2", "3", "4"]
    r = client.post("/_search/scroll", json={"scroll": "1m", "scroll_id": first["_scroll_id"]})
    assert [h["_id"] for h in r.json()["hits"]["hits"]] == ["5", "6"]
    client.request("DELETE", "/_search/scroll", json={"scroll_id": [first["_scroll_id"]]})

    r = client.post(f"/{idx}/_search/point_in_time", params={"keep_alive": "1m"})
    assert r.status_code == 200, r.text
    pit = r.json()["pit_id"]
    try:
        body = {"size": 10, "pit": {"id": pit}, "sort": ["quantity"], "_source": False}
        r = client.post("/_search", json=body)
        assert [h["_id"] for h in r.json()["hits"]["hits"]] == ["1", "2", "3", "4", "5", "6"]
        # the point in time keeps its view across a refresh
        assert client.post(f"/{idx}/_refresh").status_code == 200
        r = client.post("/_search", json=body)
        assert [h["_id"] for h in r.json()["hits"]["hits"]] == ["1", "2", "3", "4", "5", "6"]
    finally:
        client.request("DELETE", "/_search/point_in_time", json={"pit_id": [pit]})
    assert _count(client, idx) == 5


# ------------------------------------------------------- by-query on the refreshed view


def test_delete_by_query_conflicts_on_a_deleted_document(client, idx):
    assert client.delete(f"/{idx}/_doc/3").status_code == 200
    assert client.delete(f"/{idx}/_doc/5").status_code == 200
    status, counts, body = _by_query(client, idx, "delete", {"term": {"user_id": "u1"}})
    assert status == 409, body
    assert counts == {"total": 4, "deleted": 3, "batches": 1, "version_conflicts": 1, "noops": 0}
    assert [(f["id"], f["status"], f["cause"]["type"], f["cause"]["reason"]) for f in body["failures"]] == [
        ("3", 409, "version_conflict_engine_exception",
         "[3]: version conflict, required seqNo [2], primary term [1]. but no document was found")]
    # nothing was refreshed: the deleted documents are still counted
    assert _count(client, idx) == 6
    assert client.post(f"/{idx}/_refresh").status_code == 200
    assert _hits(client, idx) == [("6", 6)]


def test_update_by_query_conflicts_on_unrefreshed_writes(client, idx):
    assert client.post(f"/{idx}/_update/1", json={"doc": {"quantity": 50}}).status_code == 200
    assert client.delete(f"/{idx}/_doc/2").status_code == 200
    status, counts, body = _by_query(client, idx, "update", {"ids": {"values": ["1", "2", "3"]}})
    assert status == 409, body
    assert counts == {"total": 3, "deleted": 0, "batches": 1, "version_conflicts": 2, "noops": 0}
    assert body["updated"] == 1
    assert [(f["id"], f["cause"]["reason"]) for f in body["failures"]] == [
        ("1", "[1]: version conflict, required seqNo [0], primary term [1]. "
              "current document has seqNo [6] and primary term [1]"),
        ("2", "[2]: version conflict, required seqNo [1], primary term [1]. but no document was found")]
    assert client.get(f"/{idx}/_doc/1").json()["_source"]["quantity"] == 50
    assert client.get(f"/{idx}/_doc/3").json()["_version"] == 2


def test_update_by_query_proceed_counts_conflicts(client, idx):
    assert client.delete(f"/{idx}/_doc/2").status_code == 200
    status, counts, body = _by_query(client, idx, "update", {"term": {"user_id": "u1"}}, conflicts="proceed")
    assert status == 200, body
    assert counts == {"total": 4, "deleted": 0, "batches": 1, "version_conflicts": 1, "noops": 0}
    assert (body["updated"], body["failures"]) == (3, [])


# -------------------------------------------------------- refresh=true on by-query


def _unrefreshed_writes(client, idx):
    assert client.post(f"/{idx}/_update/1", json={"doc": {"quantity": 50}}).status_code == 200
    assert client.put(f"/{idx}/_doc/2", json={"user_id": "u1", "quantity": 20}).status_code == 200
    assert client.put(f"/{idx}/_doc/7", json={"user_id": "u2", "quantity": 70}).status_code == 201
    assert client.delete(f"/{idx}/_doc/6").status_code == 200


def test_delete_by_query_refresh_with_only_conflicts_does_not_refresh(client, idx):
    _unrefreshed_writes(client, idx)
    q = {"ids": {"values": ["1", "2"]}}
    status, counts, body = _by_query(client, idx, "delete", q, refresh="true", conflicts="proceed")
    assert status == 200, body
    assert counts == {"total": 2, "deleted": 0, "batches": 1, "version_conflicts": 2, "noops": 0}
    assert _hits(client, idx) == ALL_SIX
    status, counts, body = _by_query(client, idx, "delete", q, refresh="true")
    assert status == 409, body
    assert counts == {"total": 2, "deleted": 0, "batches": 1, "version_conflicts": 2, "noops": 0}
    assert _hits(client, idx) == ALL_SIX


def test_by_query_refresh_without_matches_does_not_refresh(client, idx):
    _unrefreshed_writes(client, idx)
    q = {"ids": {"values": ["nope"]}}
    for op in ("delete", "update"):
        status, counts, body = _by_query(client, idx, op, q, refresh="true")
        assert status == 200, body
        assert counts == {"total": 0, "deleted": 0, "batches": 0, "version_conflicts": 0, "noops": 0}
    assert _hits(client, idx) == ALL_SIX


def test_delete_by_query_refresh_after_a_delete_refreshes_the_index(client, idx):
    _unrefreshed_writes(client, idx)
    status, counts, body = _by_query(client, idx, "delete", {"ids": {"values": ["1", "3"]}},
                                     refresh="true", conflicts="proceed")
    assert status == 200, body
    assert counts == {"total": 2, "deleted": 1, "batches": 1, "version_conflicts": 1, "noops": 0}
    assert _hits(client, idx) == [("4", 4), ("5", 5), ("2", 20), ("1", 50), ("7", 70)]


def test_update_by_query_refresh_with_only_conflicts_does_not_refresh(client, idx):
    _unrefreshed_writes(client, idx)
    status, counts, body = _by_query(client, idx, "update", {"ids": {"values": ["1"]}},
                                     refresh="true", conflicts="proceed")
    assert status == 200, body
    assert counts == {"total": 1, "deleted": 0, "batches": 1, "version_conflicts": 1, "noops": 0}
    assert body["updated"] == 0
    assert _hits(client, idx) == ALL_SIX


# ------------------------------------------------ async and sliced delete_by_query


def test_async_delete_by_query_conflicts_on_unrefreshed_writes(client, idx):
    _unrefreshed_writes(client, idx)
    r = client.post(f"/{idx}/_delete_by_query", params={"wait_for_completion": "false", "conflicts": "proceed"},
                    json={"query": {"term": {"user_id": "u1"}}})
    assert r.status_code == 200, r.text
    task = r.json()["task"]
    r = client.get(f"/_tasks/{task}", params={"wait_for_completion": "true", "timeout": "20s"})
    assert r.status_code == 200, r.text
    st = r.json()["response"]
    assert {k: st[k] for k in ("total", "deleted", "version_conflicts")} == {
        "total": 4, "deleted": 2, "version_conflicts": 2}
    assert client.get(f"/{idx}/_doc/3").status_code == 404
    assert client.get(f"/{idx}/_doc/1").json()["_source"]["quantity"] == 50


def test_sliced_delete_by_query_conflicts_on_unrefreshed_writes(client, idx):
    _unrefreshed_writes(client, idx)
    r = client.post(f"/{idx}/_delete_by_query", params={"slices": "2", "conflicts": "proceed"},
                    json={"query": {"term": {"user_id": "u1"}}})
    assert r.status_code == 200, r.text
    body = r.json()
    assert {k: body[k] for k in ("total", "deleted", "version_conflicts")} == {
        "total": 4, "deleted": 2, "version_conflicts": 2}
    assert client.get(f"/{idx}/_doc/3").status_code == 404
    assert client.get(f"/{idx}/_doc/2").json()["_source"]["quantity"] == 20
