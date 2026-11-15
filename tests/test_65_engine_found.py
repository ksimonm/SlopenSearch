"""Cases found while running the OpenSearch benchmark (OSB) geonames workload against
the Rust engine.

- a scroll's later pages are not limited by `index.max_result_window` (only the page
  size is); OSB's `scroll` task (25 pages of 1000 hits, `size` and `sort=_doc` in the
  URL) failed on page 11 with "Result window is too large";
- the page size of a scroll is the `size` URL parameter when given, and the URL
  parameters of the starting request (`sort`, `q`) apply to every page;
- the keys of a search hit come in OpenSearch's order (`_source` before `sort`, then
  `matched_queries`, `_explanation`, `inner_hits`);
- large `terms` queries (OSB `large_terms`: ~45 k values) inside `bool` `filter` and
  `must_not`, next to a `match` on a keyword field.
"""
import json

import pytest

pytestmark = pytest.mark.search_api


def _bulk(client, name, docs):
    lines = []
    for i, d in docs:
        lines += [json.dumps({"index": {"_id": str(i)}}), json.dumps(d)]
    r = client.post(f"/{name}/_bulk", content=("\n".join(lines) + "\n").encode(),
                    headers={"Content-Type": "application/x-ndjson"}, params={"refresh": "true"})
    assert r.status_code == 200 and not r.json()["errors"], r.text


@pytest.fixture
def numbered(client, index_factory):
    name = index_factory(settings={"number_of_shards": 1, "number_of_replicas": 0, "max_result_window": 5},
                         mappings={"properties": {"n": {"type": "integer"}, "k": {"type": "keyword"},
                                                  "t": {"type": "text"}}})
    _bulk(client, name, [(i, {"n": i, "k": "even" if i % 2 == 0 else "odd", "t": f"word{i % 3} common"})
                         for i in range(23)])
    return name


def _scroll_all(client, name, params, body):
    r = client.post(f"/{name}/_search", params={"scroll": "1m", **params}, json=body)
    assert r.status_code == 200, r.text
    pages = [r.json()]
    sid = r.json()["_scroll_id"]
    while pages[-1]["hits"]["hits"]:
        r = client.request("GET", "/_search/scroll", json={"scroll_id": sid, "scroll": "1m"})
        assert r.status_code == 200, r.text
        pages.append(r.json())
        sid = r.json()["_scroll_id"]
        assert len(pages) < 50
    r = client.request("DELETE", "/_search/scroll", json={"scroll_id": [sid]})
    assert r.status_code == 200, r.text
    return pages


def test_scroll_pages_beyond_max_result_window(client, numbered):
    pages = _scroll_all(client, numbered, {"size": "4", "sort": "_doc"}, {"query": {"match_all": {}}})
    assert [len(p["hits"]["hits"]) for p in pages] == [4, 4, 4, 4, 4, 3, 0]
    ids = [h["_id"] for p in pages for h in p["hits"]["hits"]]
    assert ids == [str(i) for i in range(23)]
    for p in pages:
        assert p["hits"]["total"] == {"value": 23, "relation": "eq"}
        assert p["hits"]["max_score"] is None
    h = pages[2]["hits"]["hits"][0]
    assert h["_score"] is None and h["sort"] == [8]


def test_scroll_url_sort_applies_to_every_page(client, numbered):
    pages = _scroll_all(client, numbered, {"sort": "n:desc"}, {"size": 3, "query": {"term": {"k": "even"}}})
    ns = [h["_source"]["n"] for p in pages for h in p["hits"]["hits"]]
    assert ns == list(range(22, -1, -2))
    assert pages[1]["hits"]["hits"][0]["sort"] == [16]


def test_scroll_url_query_applies_to_every_page(client, numbered):
    pages = _scroll_all(client, numbered, {"q": "k:odd", "size": "2", "sort": "n"}, {})
    ns = [h["_source"]["n"] for p in pages for h in p["hits"]["hits"]]
    assert ns == list(range(1, 23, 2))


def test_scroll_scored_pages_max_score(client, numbered):
    pages = _scroll_all(client, numbered, {"size": "2"}, {"query": {"match": {"t": "word1 common"}}})
    hits = [h for p in pages for h in p["hits"]["hits"]]
    assert len(hits) == 23
    best = pages[0]["hits"]["hits"][0]["_score"]
    assert best == max(h["_score"] for h in hits)
    # the maximum over every hit, not over the page
    assert [p["hits"]["max_score"] for p in pages] == [best] * len(pages)


@pytest.mark.parametrize("extra", [{}, {"sort": ["_score"]}, {"sort": ["n"], "track_scores": True}])
def test_max_score_is_over_every_hit(client, index_factory, extra):
    name = index_factory(settings={"number_of_shards": 1, "number_of_replicas": 0},
                         mappings={"properties": {"n": {"type": "integer"}, "t": {"type": "text"}}})
    _bulk(client, name, [(i, {"n": i, "t": f"word{i % 3} common" + " x" * i}) for i in range(23)])
    q = {"query": {"match": {"t": "word1 common"}}}
    top = client.post(f"/{name}/_search", json={**q, "size": 1}).json()["hits"]["hits"][0]["_score"]
    for frm in (4, 30):
        r = client.post(f"/{name}/_search", json={**q, **extra, "from": frm, "size": 2})
        assert r.status_code == 200, r.text
        assert r.json()["hits"]["max_score"] == top
    r = client.post(f"/{name}/_search", json={**q, "sort": [{"_score": "asc"}], "from": 4, "size": 2})
    assert r.json()["hits"]["max_score"] is None


def test_hit_key_order(client, numbered):
    r = client.post(f"/{numbered}/_search", json={
        "query": {"match": {"t": {"query": "word1", "_name": "nm"}}}, "sort": ["n"], "size": 1,
        "highlight": {"fields": {"t": {}}}, "version": True, "seq_no_primary_term": True,
        "docvalue_fields": ["k"]})
    assert r.status_code == 200, r.text
    assert list(r.json()["hits"]["hits"][0]) == [
        "_index", "_id", "_version", "_seq_no", "_primary_term", "_score", "_source", "fields",
        "highlight", "sort", "matched_queries"]
    r = client.post(f"/{numbered}/_search", json={
        "query": {"match": {"t": {"query": "word1", "_name": "nm"}}}, "sort": [{"n": "desc"}], "size": 1,
        "explain": True})
    assert r.status_code == 200, r.text
    assert list(r.json()["hits"]["hits"][0]) == [
        "_shard", "_node", "_index", "_id", "_score", "_source", "sort", "matched_queries", "_explanation"]


def test_scroll_hit_key_order(client, numbered):
    pages = _scroll_all(client, numbered, {"size": "4", "sort": "_doc"}, {"query": {"match_all": {}}})
    for p in pages[:2]:
        assert list(p["hits"]["hits"][0]) == ["_index", "_id", "_score", "_source", "sort"]
