"""Parity cases found by the benchmark harness (bench/).

- OpenSearch refreshes every index each `index.refresh_interval` (1s by default; a
  search-idle shard refreshes before it is searched): a write made without `refresh` is
  searchable at the latest one interval later, without an explicit refresh;
  `refresh_interval: -1` turns that off.
- `_delete_by_query` searches the last refreshed versions and deletes with their seqNo:
  documents overwritten since are version conflicts (409 unless `conflicts=proceed`),
  reported with the seqNo the search saw.
- Metric aggregations read doc values: `float` / `half_float` values are averaged and
  summed at their indexed precision, not as the doubles of `_source`.
"""
import time

import pytest

pytestmark = pytest.mark.parity

SINGLE = {"number_of_shards": 1, "number_of_replicas": 0}
MAPPINGS = {"properties": {"user_id": {"type": "keyword"}, "quantity": {"type": "integer"}}}


def _seed(client, name):
    lines = []
    for i in range(1, 7):
        lines.append(f'{{"index":{{"_id":"{i}"}}}}')
        lines.append(f'{{"user_id":"{"u1" if i < 5 else "u2"}","quantity":{i}}}')
    r = client.post(f"/{name}/_bulk", params={"refresh": "true"}, content="\n".join(lines) + "\n",
                    headers={"Content-Type": "application/x-ndjson"})
    assert r.status_code == 200 and not r.json()["errors"], r.text
    # unrefreshed writes: an update, an overwrite, a new document and a delete
    assert client.post(f"/{name}/_update/1", json={"doc": {"quantity": 50}}).status_code == 200
    assert client.put(f"/{name}/_doc/2", json={"user_id": "u1", "quantity": 20}).status_code == 200
    assert client.put(f"/{name}/_doc/7", json={"user_id": "u1", "quantity": 70}).status_code == 201
    assert client.delete(f"/{name}/_doc/5").status_code == 200


def _u1(client, name):
    r = client.post(f"/{name}/_search", json={
        "query": {"term": {"user_id": "u1"}}, "sort": ["quantity"], "_source": ["quantity"]})
    assert r.status_code == 200, r.text
    return [(h["_id"], h["_source"]["quantity"]) for h in r.json()["hits"]["hits"]]


def _dbq(client, name, **params):
    r = client.post(f"/{name}/_delete_by_query", params=params, json={"query": {"term": {"user_id": "u1"}}})
    body = r.json()
    return r.status_code, {k: body[k] for k in ("total", "deleted", "batches", "version_conflicts", "noops")}, body


# ------------------------------------------------------------ scheduled refresh


@pytest.mark.parametrize("settings", [SINGLE, {**SINGLE, "refresh_interval": "1s"}], ids=["default", "1s"])
def test_unrefreshed_writes_become_searchable_after_the_interval(client, index_factory, settings):
    name = index_factory(settings=settings, mappings=MAPPINGS)
    _seed(client, name)
    time.sleep(2.5)
    assert _u1(client, name) == [("3", 3), ("4", 4), ("2", 20), ("1", 50), ("7", 70)]
    r = client.get(f"/{name}/_count")
    assert r.status_code == 200 and r.json()["count"] == 6
    status, counts, body = _dbq(client, name)
    assert status == 200, body
    assert counts == {"total": 5, "deleted": 5, "batches": 1, "version_conflicts": 0, "noops": 0}
    assert body["failures"] == []


def test_refresh_interval_minus_one_keeps_the_refreshed_view(client, index_factory):
    name = index_factory(settings={**SINGLE, "refresh_interval": "-1"}, mappings=MAPPINGS)
    _seed(client, name)
    time.sleep(2.5)
    assert _u1(client, name) == [("1", 1), ("2", 2), ("3", 3), ("4", 4)]
    r = client.post(f"/{name}/_count", json={"query": {"term": {"user_id": "u1"}}})
    assert r.status_code == 200 and r.json()["count"] == 4
    assert client.post(f"/{name}/_refresh").status_code == 200
    assert _u1(client, name) == [("3", 3), ("4", 4), ("2", 20), ("1", 50), ("7", 70)]


# ------------------------------------------------- delete_by_query on the refreshed view


def test_delete_by_query_conflicts_on_unrefreshed_overwrites(client, index_factory):
    name = index_factory(settings={**SINGLE, "refresh_interval": "-1"}, mappings=MAPPINGS)
    _seed(client, name)
    status, counts, body = _dbq(client, name)
    assert status == 409, body
    assert counts == {"total": 4, "deleted": 2, "batches": 1, "version_conflicts": 2, "noops": 0}
    assert [(f["index"], f["id"], f["status"], f["cause"]["type"]) for f in body["failures"]] == [
        (name, "1", 409, "version_conflict_engine_exception"),
        (name, "2", 409, "version_conflict_engine_exception")]
    assert list(body["failures"][0]) == ["index", "id", "cause", "status"]
    assert list(body["failures"][0]["cause"]) == ["type", "reason", "index", "shard", "index_uuid"]
    # the deleted documents are gone for realtime reads; the overwritten ones stay
    assert client.get(f"/{name}/_doc/3").status_code == 404
    assert client.get(f"/{name}/_doc/1").json()["_source"] == {"user_id": "u1", "quantity": 50}


def test_delete_by_query_conflict_reason_names_the_refreshed_seq_no(client, index_factory):
    name = index_factory(settings={**SINGLE, "refresh_interval": "-1"}, mappings=MAPPINGS)
    _seed(client, name)
    status, _, body = _dbq(client, name)
    assert status == 409, body
    assert [(f["cause"]["reason"], f["cause"]["index"], f["cause"]["shard"]) for f in body["failures"]] == [
        ("[1]: version conflict, required seqNo [0], primary term [1]. "
         "current document has seqNo [6] and primary term [1]", name, "0"),
        ("[2]: version conflict, required seqNo [1], primary term [1]. "
         "current document has seqNo [7] and primary term [1]", name, "0")]


def test_delete_by_query_conflicts_proceed(client, index_factory):
    name = index_factory(settings={**SINGLE, "refresh_interval": "-1"}, mappings=MAPPINGS)
    _seed(client, name)
    status, counts, body = _dbq(client, name, conflicts="proceed")
    assert status == 200, body
    assert counts == {"total": 4, "deleted": 2, "batches": 1, "version_conflicts": 2, "noops": 0}
    assert body["failures"] == []
    assert client.post(f"/{name}/_refresh").status_code == 200
    assert _u1(client, name) == [("2", 20), ("1", 50), ("7", 70)]


# ------------------------------------------------------- float precision in aggregations


@pytest.fixture
def prices(client, index_factory):
    name = index_factory(settings=SINGLE, mappings={"properties": {
        "price": {"type": "float"}, "half": {"type": "half_float"}, "created": {"type": "date"}}})
    values = [12.34, 5.67, 101.99, 0.1, 33.33, 7.77, 19.99, 250.05]
    lines = []
    for i, p in enumerate(values):
        lines.append(f'{{"index":{{"_id":"{i}"}}}}')
        lines.append(f'{{"price":{p},"half":{p},"created":"2020-0{1 + i % 2}-1{i}T00:00:00Z"}}')
    r = client.post(f"/{name}/_bulk", params={"refresh": "true"}, content="\n".join(lines) + "\n",
                    headers={"Content-Type": "application/x-ndjson"})
    assert r.status_code == 200 and not r.json()["errors"], r.text
    return name


def test_float_metrics_inside_date_histogram(client, prices):
    r = client.post(f"/{prices}/_search", json={"size": 0, "aggs": {"h": {
        "date_histogram": {"field": "created", "calendar_interval": "month"},
        "aggs": {"p": {"avg": {"field": "price"}}, "s": {"sum": {"field": "price"}},
                 "st": {"stats": {"field": "price"}}}}}})
    assert r.status_code == 200, r.text
    b = r.json()["aggregations"]["h"]["buckets"]
    assert [(x["doc_count"], x["p"]["value"], x["s"]["value"]) for x in b] == [
        (4, 41.91249990463257, 167.64999961853027), (4, 65.8975007776171, 263.5900031104684)]
    assert b[0]["st"] == {"count": 4, "min": 12.34000015258789, "max": 101.98999786376953,
                          "avg": 41.91249990463257, "sum": 167.64999961853027}
    assert b[1]["st"] == {"count": 4, "min": 0.10000000149011612, "max": 250.0500030517578,
                          "avg": 65.8975007776171, "sum": 263.5900031104684}


def test_float_and_half_float_top_level_metrics(client, prices):
    r = client.post(f"/{prices}/_search", json={"size": 0, "aggs": {
        "a": {"avg": {"field": "price"}}, "s": {"sum": {"field": "price"}},
        "ha": {"avg": {"field": "half"}}, "hmin": {"min": {"field": "half"}}, "hmax": {"max": {"field": "half"}}}})
    assert r.status_code == 200, r.text
    a = r.json()["aggregations"]
    assert {k: v["value"] for k, v in a.items()} == {
        "a": 53.90500034112483, "s": 431.24000272899866,
        "ha": 53.90165710449219, "hmin": 0.0999755859375, "hmax": 250.0}
