"""Parity cases found while aligning the Rust server's numbers with OpenSearch.

- sum, avg, stats, extended_stats and weighted_avg add their doubles with Kahan
  summation (CompensatedSum): ten 0.1 sum to 1.0, not 0.9999999999999999;
- a scaled_float is read back from its doc values as Math.round(v * factor) * (1 / factor)
  by metric aggregations too;
- Double.toString picks, of two shortest decimals equally close to the value, the one
  with the even last digit (the float 141.90005 widened reads 141.90005493164062).
"""
import json

import pytest

pytestmark = pytest.mark.parity


class Num(str):
    """The source text of a JSON number."""


def _raw(r):
    return json.loads(r.text, parse_float=Num, parse_int=Num)


@pytest.fixture
def tenths(client, index_factory):
    name = index_factory(settings={"number_of_shards": 1, "number_of_replicas": 0}, mappings={"properties": {
        "d": {"type": "double"}, "f": {"type": "float"}, "w": {"type": "long"},
        "s": {"type": "scaled_float", "scaling_factor": 100}}})
    lines = []
    for i in range(10):
        lines += [json.dumps({"index": {"_id": str(i)}}),
                  json.dumps({"d": 0.1, "f": 141.90005 if i == 3 else 1.5, "w": 1, "s": 99.99})]
    r = client.post(f"/{name}/_bulk", content=("\n".join(lines) + "\n").encode(),
                    headers={"Content-Type": "application/x-ndjson"}, params={"refresh": "true"})
    assert r.status_code == 200 and not r.json()["errors"], r.text
    return name


def test_metric_sums_are_compensated(client, tenths):
    r = client.post(f"/{tenths}/_search", json={"size": 0, "aggs": {
        "sum": {"sum": {"field": "d"}}, "avg": {"avg": {"field": "d"}},
        "es": {"extended_stats": {"field": "d"}},
        "w": {"weighted_avg": {"value": {"field": "d"}, "weight": {"field": "w"}}}}})
    assert r.status_code == 200, r.text
    a = _raw(r)["aggregations"]
    assert a["sum"]["value"] == "1.0"
    assert a["avg"]["value"] == "0.1"
    assert [a["es"][k] for k in ("sum", "avg", "sum_of_squares")] == ["1.0", "0.1", "0.10000000000000002"]
    assert a["w"]["value"] == "0.1"


def test_scaled_float_metric_values_are_doc_values(client, tenths):
    r = client.post(f"/{tenths}/_search", json={"size": 0, "aggs": {
        "avg": {"avg": {"field": "s"}}, "max": {"max": {"field": "s"}}}})
    assert r.status_code == 200, r.text
    a = _raw(r)["aggregations"]
    assert (a["avg"]["value"], a["max"]["value"]) == ("99.99000000000001", "99.99000000000001")


def test_double_to_string_ties_go_to_even(client, tenths):
    r = client.post(f"/{tenths}/_search", json={"size": 1, "_source": False, "sort": [{"f": "desc"}],
                                                "docvalue_fields": ["f"],
                                                "aggs": {"max": {"max": {"field": "f"}}}})
    assert r.status_code == 200, r.text
    body = _raw(r)
    assert body["aggregations"]["max"]["value"] == "141.90005493164062"
    assert body["hits"]["hits"][0]["fields"]["f"] == ["141.90005493164062"]
    assert body["hits"]["hits"][0]["sort"] == ["141.90005"]
