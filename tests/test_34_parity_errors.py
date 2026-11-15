"""Small parity cases found while unit-testing the servers' internals.

- mustache.java compile errors name the template line as `@[query-template:N]`;
- Jackson escapes control characters in strings it writes with upper-case hex (`\\u001F`),
  while `_source` is passed through as it was sent;
- field_sort parse errors carry the `[line:column]` of the offending token;
- date math without a `/unit` rounds nothing, so `||+1d` is an exact instant.
"""
import pytest

pytestmark = pytest.mark.parity


def _error(reason_type, reason, caused_by=None):
    err = {"root_cause": [{"type": reason_type, "reason": reason}], "type": reason_type, "reason": reason}
    if caused_by:
        err["caused_by"] = caused_by
    return {"error": err, "status": 400}


@pytest.fixture
def idx(client, index_factory):
    name = index_factory(mappings={"properties": {
        "d": {"type": "date"}, "k": {"type": "keyword"}, "n": {"type": "long"}}},
        settings={"number_of_shards": 1, "number_of_replicas": 0})
    for i, (d, k) in enumerate([("2024-01-02T10:00:00Z", "a\u001fb"), ("2024-01-03T23:00:00Z", "x")], 1):
        r = client.put(f"/{name}/_doc/{i}", json={"d": d, "k": k, "n": i}, params={"refresh": "true"})
        assert r.status_code == 201, r.text
    return name


# --------------------------------------------------------------- mustache compile errors


@pytest.mark.parametrize("source,reason", [
    ("{{q", "Improperly closed variable @[query-template:1]"),
    ("x\n\n{{q", "Improperly closed variable @[query-template:3]"),
    ("a\nb {{#x}}c", "Failed to close 'x' tag @[query-template:2]"),
], ids=["variable", "variable_line3", "section"])
def test_render_template_compile_error(client, source, reason):
    r = client.post("/_render/template", json={"source": source, "params": {"q": "a"}})
    assert r.status_code == 400
    script = {"type": "script_exception", "reason": reason, "script_stack": [], "script": source, "lang": "mustache"}
    assert r.json() == {"error": {"root_cause": [script], **script,
                                  "caused_by": {"type": "mustache_exception", "reason": reason}},
                        "status": 400}


# --------------------------------------------------------------- control characters


def test_error_message_escapes_control_chars_upper_case(client, idx):
    r = client.post(f"/{idx}/_search", json={"query": {"term": {"n": "a\u001fb\u0001"}}})
    assert r.status_code == 400
    assert '\\"a\\u001Fb\\u0001\\"' in r.text
    assert "\\u001f" not in r.text
    err = r.json()["error"]
    assert err["root_cause"][0]["reason"] == 'failed to create query: For input string: "a\u001fb\u0001"'
    assert err["caused_by"]["caused_by"] == {"type": "number_format_exception",
                                             "reason": 'For input string: "a\u001fb\u0001"'}


def test_source_keeps_control_chars_as_sent(client, idx):
    r = client.get(f"/{idx}/_doc/1")
    assert '"k":"a\\u001fb"' in r.text


# --------------------------------------------------------------- field_sort parse errors


def _sort_error(pos, field, cause):
    return _error("x_content_parse_exception", f"[{pos}] [field_sort] failed to parse field [{field}]",
                  {"type": "illegal_argument_exception", "reason": cause})


@pytest.mark.parametrize("body,expected", [
    ('{"sort":[{"n":{"mode":"bogus"}}]}', _sort_error("1:23", "mode", "Unknown SortMode [bogus]")),
    ('{"sort":[{"n":{"order":"up"}}]}',
     _sort_error("1:24", "order", "No enum constant org.opensearch.search.sort.SortOrder.UP")),
    ('{"sort":["k",{"n":{"order":"up"}}]}',
     _sort_error("1:28", "order", "No enum constant org.opensearch.search.sort.SortOrder.UP")),
    ('{"query":{"match_all":{}},"sort":[{"n":{"mode":"bogus"}}]}',
     _sort_error("1:48", "mode", "Unknown SortMode [bogus]")),
    ('{\n  "sort": [\n    {"n": {"mode": "bogus"}}\n  ]\n}', _sort_error("3:20", "mode", "Unknown SortMode [bogus]")),
    ('{"sort":[{"n":{"numeric_type":"x"}}]}',
     _sort_error("1:31", "numeric_type", "invalid value for [numeric_type], must be [long, double, date, date_nanos], got x")),
    ('{"sort":[{"n":{"foo":1}}]}', _error("x_content_parse_exception", "[1:16] [field_sort] unknown field [foo]")),
    ('{"sort":[{"n":{"missing":{}}}]}',
     _error("x_content_parse_exception", "[1:26] [field_sort] missing doesn't support values of type: START_OBJECT")),
    ('{"sort":[{"n":"up"}]}',
     _error("illegal_argument_exception", "No enum constant org.opensearch.search.sort.SortOrder.UP")),
], ids=["mode", "order", "second_sort", "after_query", "multiline", "numeric_type", "unknown_param",
        "missing_object", "short_order"])
def test_field_sort_parse_error(client, idx, body, expected):
    r = client.post(f"/{idx}/_search", content=body.encode(), headers={"Content-Type": "application/json"})
    assert r.status_code == 400
    assert r.json() == expected


# --------------------------------------------------------------- date math rounding


@pytest.mark.parametrize("op,value,ids", [
    ("lte", "2024-01-02T12:00:00Z||+1d", ["1"]),
    ("gt", "2024-01-02T12:00:00Z||+1d", ["2"]),
    ("lt", "2024-01-04T12:00:00Z||-1d", ["1"]),
    ("gte", "2024-01-02T12:00:00Z||+1d/d", ["2"]),
    ("lte", "2024-01-02T12:00:00Z||+1d/d", ["1", "2"]),
    ("gte", "2024-01-03T10:00:00Z||+12h", ["2"]),
    ("lt", "2024-01-03T10:00:00Z||+13h", ["1"]),
])
def test_range_date_math_rounds_only_with_unit(client, idx, op, value, ids):
    r = client.post(f"/{idx}/_search", json={"query": {"range": {"d": {op: value}}}, "sort": ["n"]})
    assert r.status_code == 200, r.text
    assert [h["_id"] for h in r.json()["hits"]["hits"]] == ids
