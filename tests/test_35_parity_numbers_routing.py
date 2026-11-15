"""Shard routing and the text of Java numbers.

- documents are routed to shards with Murmur3 (x86_32, seed 0) over the UTF-16 code
  units of the routing value (the _id without one), floorMod
  index.number_of_routing_shards (by default number_of_shards * 2^x <= 1024), divided by
  the routing factor; a partitioned index adds the partition offset of the _id;
- Jackson writes Java doubles with Double.toString (1.0E7, 1.5E20, 1.0E-4) and floats
  with Float.toString, so the text of a number depends on the Java type OpenSearch holds
  it in: metric values, sort values, doc value fields, script fields, double terms keys,
  percentile keys and scores.
"""
import json

import pytest

pytestmark = pytest.mark.parity


class Num(str):
    """The source text of a JSON number."""


def _raw(r):
    """The response body with every number kept as its text."""
    return json.loads(r.text, parse_float=Num, parse_int=Num)


# --------------------------------------------------------------------------- routing

IDS = ["1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "abc", "é", "日本", "😀", "a b", "doc-42"]
ROUTINGS = ["x", "y", "user1", "ü", "😀"]


def _routed_index(client, index_factory, settings, partitioned=False):
    name = index_factory(settings={**settings, "number_of_replicas": 0},
                         mappings={"_routing": {"required": True}} if partitioned else None)
    lines = []
    for i in IDS:
        meta = {"_id": i}
        if partitioned:
            meta["routing"] = "r" + str(len(i) % 3)
        lines += [json.dumps({"index": meta}), '{"v":1}']
    for rt in ROUTINGS:
        lines += [json.dumps({"index": {"_id": "rt-" + rt, "routing": rt}}), '{"v":2}']
    r = client.post(f"/{name}/_bulk", content=("\n".join(lines) + "\n").encode(),
                    headers={"Content-Type": "application/x-ndjson"}, params={"refresh": "true"})
    assert r.status_code == 200 and not r.json()["errors"], r.text
    return name


ROUTING_CASES = {
    "2_shards": ({"number_of_shards": 2}, False, [
        ["1", "10", "2", "3", "5", "6", "7", "9", "rt-user1", "rt-y", "é", "日本"],
        ["4", "8", "a b", "abc", "doc-42", "rt-x", "rt-ü", "rt-😀", "😀"],
    ], {"x": 1, "y": 0, "user1": 0, "ü": 1, "😀": 1, "1": 0, "abc": 1}),
    "3_shards": ({"number_of_shards": 3}, False, [
        ["5", "7", "abc", "rt-😀", "日本", "😀"],
        ["10", "2", "3", "4", "rt-user1", "rt-ü", "é"],
        ["1", "6", "8", "9", "a b", "doc-42", "rt-x", "rt-y"],
    ], {"x": 2, "y": 2, "user1": 1, "ü": 1, "😀": 0, "1": 2, "abc": 0}),
    "5_shards": ({"number_of_shards": 5}, False, [
        ["3", "5", "9", "a b", "doc-42", "rt-user1", "rt-x", "rt-y", "rt-ü"],
        ["4"],
        ["10", "7", "8", "rt-😀", "😀"],
        ["2", "6"],
        ["1", "abc", "é", "日本"],
    ], {"x": 0, "y": 0, "user1": 0, "ü": 0, "😀": 2, "1": 4, "abc": 4}),
    "routing_shards": ({"number_of_shards": 4, "number_of_routing_shards": 8}, False, [
        ["6", "doc-42", "rt-y", "rt-😀", "😀"],
        ["1", "2", "3", "4", "8", "9", "abc", "日本"],
        ["a b", "rt-user1", "rt-x", "rt-ü"],
        ["10", "5", "7", "é"],
    ], {"x": 2, "y": 0, "user1": 2, "ü": 2, "😀": 0, "1": 1, "abc": 1}),
    "partitioned": ({"number_of_shards": 6, "routing_partition_size": 2}, True, [
        ["1", "2", "3", "4", "5", "6", "7", "8", "9", "rt-😀", "é", "😀"],
        ["10", "a b", "abc", "doc-42", "日本"],
        ["rt-user1"],
        ["rt-ü"],
        [],
        ["rt-x", "rt-y"],
    ], {"x": 5, "y": 5, "user1": 2, "ü": 3, "😀": 0, "r0": 1, "r1": 0, "r2": 1}),
}


@pytest.mark.parametrize("case", list(ROUTING_CASES))
def test_preference_shards_hold_routed_docs(client, index_factory, case):
    settings, partitioned, shards, _ = ROUTING_CASES[case]
    name = _routed_index(client, index_factory, settings, partitioned)
    for shard, want in enumerate(shards):
        r = client.post(f"/{name}/_search", params={"preference": f"_shards:{shard}", "size": 100},
                        json={"sort": ["_doc"]})
        assert r.status_code == 200, r.text
        assert sorted(h["_id"] for h in r.json()["hits"]["hits"]) == want, f"shard {shard}"


@pytest.mark.parametrize("case", list(ROUTING_CASES))
def test_search_shards_routing(client, index_factory, case):
    settings, partitioned, _, routings = ROUTING_CASES[case]
    name = _routed_index(client, index_factory, settings, partitioned)
    for routing, shard in routings.items():
        r = client.get(f"/{name}/_search_shards", params={"routing": routing})
        assert r.status_code == 200, r.text
        assert [g[0]["shard"] for g in r.json()["shards"]] == [shard], routing


@pytest.mark.parametrize("case", ["3_shards", "partitioned"])
def test_cat_shards_doc_counts(client, index_factory, case):
    settings, partitioned, shards, _ = ROUTING_CASES[case]
    name = _routed_index(client, index_factory, settings, partitioned)
    r = client.get(f"/_cat/shards/{name}", params={"h": "shard,docs", "s": "shard", "format": "json"})
    assert r.status_code == 200, r.text
    assert [(row["shard"], row["docs"]) for row in r.json()] == [(str(i), str(len(ids))) for i, ids in
                                                                 enumerate(shards)]


def test_routing_search_and_get(client, index_factory):
    name = _routed_index(client, index_factory, {"number_of_shards": 5})
    r = client.post(f"/{name}/_search", params={"routing": "😀", "size": 100}, json={"sort": ["_doc"]})
    assert sorted(h["_id"] for h in r.json()["hits"]["hits"]) == ["10", "7", "8", "rt-😀", "😀"]
    # a get with the wrong routing looks at the wrong shard
    assert client.get(f"/{name}/_doc/rt-x", params={"routing": "x"}).json()["found"] is True
    assert client.get(f"/{name}/_doc/rt-x", params={"routing": "😀"}).json()["found"] is False
    assert client.get(f"/{name}/_doc/rt-x", params={"routing": "y"}).json()["found"] is True  # same shard


def test_explain_names_the_shard(client, index_factory):
    name = _routed_index(client, index_factory, {"number_of_shards": 5})
    r = client.post(f"/{name}/_search", params={"explain": "true"}, json={"query": {"ids": {"values": ["abc", "4"]}},
                                                                        "sort": ["_doc"]})
    assert r.status_code == 200, r.text
    assert sorted((h["_id"], h["_shard"]) for h in r.json()["hits"]["hits"]) == [
        ("4", f"[{name}][1]"), ("abc", f"[{name}][4]")]


# --------------------------------------------------------------------------- numbers


@pytest.fixture
def nums(client, index_factory):
    name = index_factory(settings={"number_of_shards": 1, "number_of_replicas": 0}, mappings={"properties": {
        "d": {"type": "double"}, "f": {"type": "float"}, "h": {"type": "half_float"},
        "s": {"type": "scaled_float", "scaling_factor": 100}, "l": {"type": "long"}, "k": {"type": "keyword"}}})
    docs = [
        {"d": 1.0e7, "f": 1.0e7, "h": 1000.5, "s": 12345678.9, "l": 10000000, "k": "a"},
        {"d": 1.5e20, "f": 1.5e20, "h": 0.0001, "s": 0.01, "l": 1, "k": "b"},
        {"d": 1.0e-4, "f": 1.0e-4, "h": 65504, "s": 1e10, "l": 2, "k": "c"},
        {"d": 123456789.125, "f": 3.4e38, "h": 0.5, "s": 99.99, "l": 3, "k": "d"},
    ]
    for i, d in enumerate(docs, 1):
        r = client.put(f"/{name}/_doc/{i}", json=d, params={"refresh": "true"})
        assert r.status_code == 201, r.text
    return name


def _search(client, idx, body):
    r = client.post(f"/{idx}/_search", json=body)
    assert r.status_code == 200, r.text
    return _raw(r)


def test_metric_aggs_of_doubles(client, nums):
    a = _search(client, nums, {"size": 0, "aggs": {
        "avg": {"avg": {"field": "d"}}, "sum": {"sum": {"field": "d"}}, "max": {"max": {"field": "d"}},
        "min": {"min": {"field": "d"}}, "es": {"extended_stats": {"field": "d"}}}})["aggregations"]
    assert a["avg"]["value"] == "3.750000000003337E19"
    assert a["sum"]["value"] == "1.5000000000013346E20"
    assert a["max"]["value"] == "1.5E20"
    assert a["min"]["value"] == "1.0E-4"
    es = a["es"]
    assert [es[k] for k in ["count", "min", "max", "sum", "sum_of_squares", "variance", "std_deviation"]] == [
        "4", "1.0E-4", "1.5E20", "1.5000000000013346E20", "2.25E40", "4.218749999997498E39", "6.495190528381364E19"]
    assert es["std_deviation_bounds"]["lower"] == "-9.240381056759392E19"


def test_metric_aggs_of_floats_halfs_scaled_and_longs(client, nums):
    a = _search(client, nums, {"size": 0, "aggs": {
        "minf": {"min": {"field": "f"}}, "maxf": {"max": {"field": "f"}}, "avgf": {"avg": {"field": "f"}},
        "minh": {"min": {"field": "h"}}, "maxh": {"max": {"field": "h"}},
        "maxs": {"max": {"field": "s"}}, "sums": {"sum": {"field": "s"}},
        "suml": {"sum": {"field": "l"}}, "maxl": {"max": {"field": "l"}}, "avgl": {"avg": {"field": "l"}}}})[
        "aggregations"]
    assert {k: v["value"] for k, v in a.items()} == {
        "minf": "9.999999747378752E-5", "maxf": "3.3999999521443642E38", "avgf": "8.499999880360911E37",
        "minh": "1.0001659393310547E-4", "maxh": "65504.0", "maxs": "1.0E10", "sums": "1.00123457789E10",
        "suml": "1.0000006E7", "maxl": "1.0E7", "avgl": "2500001.5"}


@pytest.mark.parametrize("field,want", [
    ("d", ["1.0E-4", "1.0E7", "1.23456789125E8", "1.5E20"]),
    ("f", ["1.0E-4", "1.0E7", "1.5E20", "3.4E38"]),
    ("h", ["1.00016594E-4", "0.5", "1000.5", "65504.0"]),
    ("s", ["0.01", "99.99000000000001", "1.23456789E7", "1.0E10"]),
    ("l", ["1", "2", "3", "10000000"]),
])
def test_sort_values_are_java_numbers(client, nums, field, want):
    hits = _search(client, nums, {"sort": [{field: "asc"}], "_source": False})["hits"]["hits"]
    assert [h["sort"][0] for h in hits] == want


def test_docvalue_fields_read_doubles(client, nums):
    hits = _search(client, nums, {"_source": False, "docvalue_fields": ["d", "f", "h", "s"], "sort": ["l"]})[
        "hits"]["hits"]
    assert [h["fields"] for h in hits] == [
        {"d": ["1.5E20"], "f": ["1.500000030061316E20"], "h": ["1.0001659393310547E-4"], "s": ["0.01"]},
        {"d": ["1.0E-4"], "f": ["9.999999747378752E-5"], "h": ["65504.0"], "s": ["1.0E10"]},
        {"d": ["1.23456789125E8"], "f": ["3.3999999521443642E38"], "h": ["0.5"], "s": ["99.99000000000001"]},
        {"d": ["1.0E7"], "f": ["1.0E7"], "h": ["1000.5"], "s": ["1.23456789E7"]},
    ]


def test_fields_api_parses_source_values(client, nums):
    hits = _search(client, nums, {"_source": False, "fields": ["d", "f", "h", "s"], "sort": ["l"]})["hits"]["hits"]
    assert [h["fields"] for h in hits] == [
        {"d": ["1.5E20"], "f": ["1.5E20"], "h": ["1.0E-4"], "s": ["0.01"]},
        {"d": ["1.0E-4"], "f": ["1.0E-4"], "h": ["65504.0"], "s": ["1.0E10"]},
        {"d": ["1.23456789125E8"], "f": ["3.4E38"], "h": ["0.5"], "s": ["99.99"]},
        {"d": ["1.0E7"], "f": ["1.0E7"], "h": ["1000.5"], "s": ["1.23456789E7"]},
    ]


def test_script_fields_doubles_and_floats(client, nums):
    hits = _search(client, nums, {"_source": False, "sort": ["l"], "script_fields": {
        "a": {"script": "doc['d'].value * 1"}, "b": {"script": "doc['f'].value"}, "c": {"script": "1.0e7"},
        "e": {"script": "(float) 1.0e7"}, "i": {"script": "doc['l'].value"}}})["hits"]["hits"]
    assert [h["fields"] for h in hits[:2]] == [
        {"a": ["1.5E20"], "b": ["1.500000030061316E20"], "c": ["1.0E7"], "e": ["1.0E7"], "i": ["1"]},
        {"a": ["1.0E-4"], "b": ["9.999999747378752E-5"], "c": ["1.0E7"], "e": ["1.0E7"], "i": ["2"]},
    ]


def test_percentile_keys_and_double_terms(client, nums):
    a = _search(client, nums, {"size": 0, "aggs": {
        "p": {"percentiles": {"field": "d", "percents": [0.0001, 50, 99.99999]}},
        "t": {"terms": {"field": "d"}},
        "r": {"range": {"field": "d", "ranges": [{"to": 1e7}, {"from": 1e7}]}},
        "hist": {"histogram": {"field": "d", "interval": 1e19, "min_doc_count": 1}}}})["aggregations"]
    assert a["p"]["values"] == {"1.0E-4": "1.0E-4", "50.0": "1.23456789125E8", "99.99999": "1.5E20"}
    assert [b["key"] for b in a["t"]["buckets"]] == ["1.0E-4", "1.0E7", "1.23456789125E8", "1.5E20"]
    assert [(b["key"], b.get("from"), b.get("to")) for b in a["r"]["buckets"]] == [
        ("*-1.0E7", None, "1.0E7"), ("1.0E7-*", "1.0E7", None)]
    assert [b["key"] for b in a["hist"]["buckets"]] == ["0.0", "1.5E20"]


def test_pipeline_and_scripted_values(client, nums):
    a = _search(client, nums, {"size": 0, "aggs": {
        "t": {"terms": {"field": "k"}, "aggs": {
            "m": {"max": {"field": "d"}}, "bs": {"bucket_script": {"buckets_path": {"x": "m"}, "script": "params.x * 2"}}}},
        "mb": {"max_bucket": {"buckets_path": "t>m"}},
        "w": {"weighted_avg": {"value": {"field": "d"}, "weight": {"field": "l"}}},
        "s": {"scripted_metric": {
            "init_script": "state.x = 0.0", "map_script": "state.x += doc['d'].value",
            "combine_script": "return state.x", "reduce_script": "double t = 0; for (s in states) { t += s } return t"}}}})[
        "aggregations"]
    assert [(b["m"]["value"], b["bs"]["value"]) for b in a["t"]["buckets"]] == [
        ("1.0E7", "2.0E7"), ("1.5E20", "3.0E20"), ("1.0E-4", "2.0E-4"), ("1.23456789125E8", "2.4691357825E8")]
    assert a["mb"] == {"value": "1.5E20", "keys": ["b"]}
    assert a["w"]["value"] == "1.5000001000036436E13"
    assert a["s"]["value"] == "1.5000000000013346E20"


def test_top_hits_sort_values(client, nums):
    a = _search(client, nums, {"size": 0, "aggs": {"t": {"top_hits": {"sort": [{"d": "desc"}], "_source": False,
                                                                       "size": 2}}}})["aggregations"]
    assert [h["sort"] for h in a["t"]["hits"]["hits"]] == [["1.5E20"], ["1.23456789125E8"]]


@pytest.mark.parametrize("query,score", [
    ({"function_score": {"query": {"match_all": {}}, "boost": 1e8, "functions": [{"weight": 1e9}]}}, "1.0E17"),
    ({"constant_score": {"filter": {"match_all": {}}, "boost": 0.0001}}, "1.0E-4"),
    ({"constant_score": {"filter": {"match_all": {}}, "boost": 3}}, "3.0"),
])
def test_scores_are_java_floats(client, nums, query, score):
    hits = _search(client, nums, {"query": query, "_source": False, "size": 1})["hits"]
    assert hits["max_score"] == score
    assert hits["hits"][0]["_score"] == score
