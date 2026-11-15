"""Ingest templates and geohashes.

- ingest processors render `{{...}}` properties with the mustache engine on the
  document's template model (fields, metadata, `_source`, `_ingest`): JSON escaping for
  `{{x}}`, lists and maps rendered as Java maps, sections, `toJson`/`join`/`url`;
  compile errors are reported when the pipeline is created, failing list lookups
  while it runs;
- a geohash read as a geo point is the bottom-left corner of its cell (at most 12
  characters are used, letters are case-sensitive); geohash_grid and geotile_grid
  buckets with equal counts are ordered by their cell hash, descending.
"""
import re

import pytest

pytestmark = pytest.mark.parity

DOC = {"a": 'he said "hi" <b>&\'\\ \u00e9\n', "n": 1, "f": 1.5, "f0": 1.0, "big": 12345678901234,
       "t": True, "z": None, "obj": {"x": 1, "y": "s"}, "tags": ["p", "q"], "nums": [1, 2.5],
       "nested": {"deep": {"v": "dv"}}, "empty": "", "earr": [], "objs": [{"k": "a"}, {"k": "b"}]}


def _simulate(client, processors, doc=None):
    return client.post("/_ingest/pipeline/_simulate", json={
        "pipeline": {"processors": processors},
        "docs": [{"_index": "ix", "_id": "id1", "_source": DOC if doc is None else doc}]})


def _source(client, processors, doc=None):
    r = _simulate(client, processors, doc)
    assert r.status_code == 200, r.text
    return r.json()["docs"][0]["doc"]["_source"]


# --------------------------------------------------------------- template rendering


@pytest.mark.parametrize("template,expected", [
    ("{{a}}", 'he said \\"hi\\" <b>&\'\\\\ \u00e9\\n'),
    ("{{{a}}}", DOC["a"]),
    ("{{&a}}", DOC["a"]),
    ("{{ a }}", 'he said \\"hi\\" <b>&\'\\\\ \u00e9\\n'),
    ("{{n}}|{{f}}|{{f0}}|{{big}}|{{t}}", "1|1.5|1.0|12345678901234|true"),
    ("[{{z}}][{{missing}}][{{z.x}}][{{n.x}}][{{tags.x}}][{{obj.x.y}}]", "[][][][][][]"),
    ("{{obj}}", "{x=1, y=s}"),
    ("{{tags}} {{nums}}", "{0=p, 1=q} {0=1, 1=2.5}"),
    ("{{objs}}", "{0={k=a}, 1={k=b}}"),
    ("{{tags.0}}{{tags.1}} {{nested.deep.v}}", "pq dv"),
    ("{{_source.a}}", 'he said \\"hi\\" <b>&\'\\\\ \u00e9\\n'),
    ("{{_source.nested.deep.v}} {{_index}}/{{_id}} [{{_version}}] [{{_ingest.nope}}]", "dv ix/id1 [] []"),
    ("{{#tags}}<{{.}}>{{/tags}}", "<p><q>"),
    ("{{#nums}}{{.}} {{/nums}}", "1 2.5 "),
    ("{{^missing}}none{{/missing}}{{^tags}}none{{/tags}}", "none"),
    ("{{#t}}yes{{/t}}{{#empty}}x{{/empty}}{{^empty}}e{{/empty}}", "yese"),
    ("{{#earr}}x{{/earr}}{{^earr}}e{{/earr}}", "e"),
    ("{{#obj}}{{x}}-{{y}}{{/obj}}", "1-s"),
    ("{{#objs}}{{k}};{{/objs}}", "a;b;"),
    ("{{#toJson}}obj{{/toJson}} {{#toJson}}tags{{/toJson}}", '{"x":1,"y":"s"} ["p","q"]'),
    ("{{#join}}tags{{/join}} {{#join delimiter='|'}}tags{{/join delimiter='|'}}", "p,q p|q"),
    ("{{! comment }}c", "c"),
    ("{{=<% %>=}}<% n %>", "1"),
    ("{{=<% %>}}", ""),
    ("{{#url}}{{a}}{{/url}}", "he+said+%5C%22hi%5C%22+%3Cb%3E%26%27%5C%5C+%C3%A9%5Cn"),
    ("{{{n}}}}", "1}"),
    ("x}} {", "x}} {"),
])
def test_set_template(client, template, expected):
    assert _source(client, [{"set": {"field": "out", "value": template}}])["out"] == expected


def test_ingest_timestamp_template(client):
    src = _source(client, [{"set": {"field": "ts", "value": "{{_ingest.timestamp}}"}},
                           {"set": {"field": "ts2", "value": "{{{_ingest.timestamp}}}"}}])
    for v in (src["ts"], src["ts2"]):
        assert re.fullmatch(r"\d{4}-\d\d-\d\dT\d\d:\d\d(:\d\d(\.\d{1,9})?)?Z", v), v


def test_set_object_value_templates_keys_and_values(client):
    src = _source(client, [{"set": {"field": "out", "value": {"{{k}}": "{{a}}", "l": ["{{a}}", 1]}}}],
                  {"a": "va", "k": "kk"})
    assert src["out"] == {"kk": "va", "l": ["va", 1]}


def test_set_field_name_templates(client):
    src = _source(client, [{"set": {"field": "{{k}}_f", "value": "v"}},
                           {"set": {"field": "o.{{k}}", "value": "{{n}}"}}], {"k": "kk", "n": 3})
    assert src == {"k": "kk", "n": 3, "kk_f": "v", "o": {"kk": "3"}}


def test_append_templates(client):
    src = _source(client, [{"append": {"field": "tags", "value": ["{{a}}", "{{k}}"]}},
                           {"append": {"field": "{{k}}", "value": "{{n}}"}}],
                  {"a": "va", "k": "kk", "n": 3, "tags": ["p"]})
    assert src["tags"] == ["p", "va", "kk"]
    assert src["kk"] == ["3"]


def test_rename_templates(client):
    src = _source(client, [{"rename": {"field": "{{k}}x", "target_field": "{{a}}_t"}}],
                  {"a": "va", "k": "kk", "kkx": 1})
    assert src == {"a": "va", "k": "kk", "va_t": 1}


def test_fail_message_template(client):
    r = _simulate(client, [{"fail": {"message": "bad {{a}} {{{a}}} {{tags}}"}}], {"a": "v\"a", "tags": ["p", "q"]})
    assert r.json()["docs"][0]["error"] == {
        "root_cause": [{"type": "fail_processor_exception", "reason": 'bad v\\"a v"a {0=p, 1=q}'}],
        "type": "fail_processor_exception", "reason": 'bad v\\"a v"a {0=p, 1=q}'}


def test_ignore_empty_value_and_override_with_templates(client):
    src = _source(client, [{"set": {"field": "out", "value": "{{missing}}", "ignore_empty_value": True}},
                           {"set": {"field": "out2", "value": "", "ignore_empty_value": True}},
                           {"set": {"field": "a", "value": "{{k}}", "override": False}}], {"a": "va", "k": "kk"})
    assert src == {"a": "va", "k": "kk", "out2": ""}


# --------------------------------------------------------------- failing lookups


def _lookup_error(name, inner, typ, msg):
    return {"root_cause": [{"type": "mustache_exception", "reason": f"Failed to get value for {name} @[query-template:1]"}],
            "type": "mustache_exception", "reason": f"Failed to get value for {name} @[query-template:1]",
            "caused_by": {"type": "mustache_exception", "reason": f"{inner} @[query-template:1]",
                          "caused_by": {"type": typ, "reason": msg}}}


@pytest.mark.parametrize("template,error", [
    ("{{tags.5}}", _lookup_error("tags.5", "5", "index_out_of_bounds_exception", "5")),
    ("{{tags.-1}}", _lookup_error("tags.-1", "position >= 0", "illegal_argument_exception", "position >= 0")),
])
def test_list_lookup_out_of_range_fails_document(client, template, error):
    r = _simulate(client, [{"set": {"field": "out", "value": template}}])
    assert r.status_code == 200
    assert r.json()["docs"][0]["error"] == error


def test_list_lookup_failure_on_failure_message(client):
    src = _source(client, [{"set": {"field": "out", "value": "{{tags.9}}", "on_failure": [
        {"set": {"field": "err", "value": "{{_ingest.on_failure_message}}|{{_ingest.on_failure_processor_type}}"}}]}}],
        {"tags": ["p"]})
    assert src == {"tags": ["p"], "err": "9|set"}


def test_list_lookup_failure_when_indexing(client, index_factory):
    idx = index_factory(settings={"number_of_shards": 1, "number_of_replicas": 0})
    pid = f"{idx}-oob"
    assert client.put(f"/_ingest/pipeline/{pid}", json={"processors": [
        {"set": {"field": "out", "value": "{{tags.9}}"}}]}).status_code == 200
    try:
        r = client.put(f"/{idx}/_doc/1", params={"pipeline": pid}, json={"tags": ["p"]})
        assert r.status_code == 500
        assert r.json() == {"error": _lookup_error("tags.9", "9", "index_out_of_bounds_exception", "9"), "status": 500}
    finally:
        client.delete(f"/_ingest/pipeline/{pid}")


# --------------------------------------------------------------- compile errors


def _compile_error(reason, script, processor, **fields):
    head = {"type": "script_exception", "reason": reason, "processor_type": processor, **fields,
            "script_stack": [], "script": script, "lang": "mustache"}
    return {"error": {"root_cause": [head], **head, "caused_by": {"type": "mustache_exception", "reason": reason}},
            "status": 400}


@pytest.mark.parametrize("template,reason", [
    ("{{#a}}x", "Failed to close 'a' tag @[query-template:1]"),
    ("{{^a}}x", "Failed to close 'a' tag @[query-template:1]"),
    ("{{#a", "Failed to close 'a' tag @[query-template:1]"),
    ("{{/a}}", "Mismatched start/end tags: null != a in query-template:1 @[query-template:1]"),
    ("{{/a}}{{#b}}", "Mismatched start/end tags: null != a in query-template:1 @[query-template:1]"),
    ("{{#a}}{{/a}} {{/c}}", "Mismatched start/end tags: null != c in query-template:1 @[query-template:1]"),
    ("{{#a}}x{{/b}}", "Mismatched start/end tags: a != b in query-template:1 @[query-template:1]"),
    ("{{a", "Improperly closed variable @[query-template:1]"),
    ("x{{a", "Improperly closed variable @[query-template:1]"),
    ("{{a}", "Improperly closed variable @[query-template:1]"),
    ("{{{a}}", "Improperly closed variable: a in query-template:1@6 @[query-template:1]"),
    ("x{{{a}} y", "Improperly closed variable: a in query-template:1@7 @[query-template:1]"),
    ("{{>p}}", "Partial templates are not supported in OpenSearch Mustache scripts: [p]"),
])
def test_simulate_template_compile_error(client, template, reason):
    r = _simulate(client, [{"set": {"field": "out", "value": template}}])
    assert r.status_code == 400
    assert r.json() == _compile_error(reason, template, "set")


@pytest.mark.parametrize("processor,expected", [
    ({"set": {"field": "x", "value": "{{#a}}"}},
     _compile_error("Failed to close 'a' tag @[query-template:1]", "{{#a}}", "set")),
    ({"set": {"tag": "t1", "field": "x", "value": "{{#a}}"}},
     _compile_error("Failed to close 'a' tag @[query-template:1]", "{{#a}}", "set", processor_tag="t1")),
    ({"set": {"field": "{{a", "value": "v"}},
     _compile_error("Improperly closed variable @[query-template:1]", "{{a", "set", property_name="field")),
    ({"fail": {"message": "{{/a}}"}},
     _compile_error("Mismatched start/end tags: null != a in query-template:1 @[query-template:1]", "{{/a}}", "fail",
                    property_name="message")),
    ({"append": {"field": "x", "value": ["ok", "{{#q}}"]}},
     _compile_error("Failed to close 'q' tag @[query-template:1]", "{{#q}}", "append")),
], ids=["set-value", "set-tag", "set-field", "fail-message", "append-list"])
def test_put_pipeline_template_compile_error(client, processor, expected):
    r = client.put("/_ingest/pipeline/slopensearch-test-38-bad", json={"processors": [processor]})
    assert r.status_code == 400
    assert r.json() == expected
    assert client.get("/_ingest/pipeline/slopensearch-test-38-bad").status_code == 404


# --------------------------------------------------------------- geohash points

GEO_DOCS = {"1": "u4pruydqqvj", "2": "u4", "3": "s", "4": {"lat": 40.7128, "lon": -74.006},
            "5": "u4pruydqqvj8x", "6": "dr5regw3pg6f", "7": "-0.0001,179.9999"}


@pytest.fixture
def geo(client, index_factory):
    idx = index_factory(mappings={"properties": {"p": {"type": "geo_point"}}},
                        settings={"number_of_shards": 1, "number_of_replicas": 0})
    for i, p in GEO_DOCS.items():
        r = client.put(f"/{idx}/_doc/{i}", json={"p": p})
        assert r.status_code == 201, r.text
    client.post(f"/{idx}/_refresh")
    return idx


def _hits(client, idx, body):
    r = client.post(f"/{idx}/_search", json={"sort": [{"_id": "asc"}], "_source": False, **body})
    assert r.status_code == 200, r.text
    return {h["_id"]: h.get("fields") for h in r.json()["hits"]["hits"]}


def test_geohash_point_is_cell_corner_in_docvalue_fields(client, geo):
    hits = _hits(client, geo, {"docvalue_fields": ["p"], "query": {"ids": {"values": ["1", "2", "3", "5"]}}})
    assert hits == {"1": {"p": ["57.649109959602356, 10.407439023256302"]}, "2": {"p": ["56.25, 0.0"]},
                    "3": {"p": ["0.0, 0.0"]}, "5": {"p": ["57.649109959602356, 10.407439693808556"]}}


def test_geohash_point_is_cell_corner_in_fields(client, geo):
    hits = _hits(client, geo, {"fields": ["p"], "query": {"ids": {"values": ["1", "2", "3"]}}})
    assert hits == {"1": {"p": [{"coordinates": [10.407439023256302, 57.649109959602356], "type": "Point"}]},
                    "2": {"p": [{"coordinates": [0.0, 56.25], "type": "Point"}]},
                    "3": {"p": [{"coordinates": [0.0, 0.0], "type": "Point"}]}}


def test_geohash_point_in_script(client, geo):
    r = client.post(f"/{geo}/_search", json={"query": {"ids": {"values": ["2"]}}, "_source": False, "script_fields": {
        "s": {"script": "doc['p'].value.lat + ',' + doc['p'].value.lon"}}})
    assert r.json()["hits"]["hits"][0]["fields"] == {"s": ["56.25,0.0"]}


@pytest.mark.parametrize("value,reason", [
    ("DRM3BTEV3E86", "unsupported symbol [D] in geohash [DRM3BTEV3E86]"),
    ("u4pruydqqvjA", "unsupported symbol [A] in geohash [u4pruydqqvjA]"),
    ("abc", "unsupported symbol [a] in geohash [abc]"),
    ("u4!", "unsupported symbol [!] in geohash [u4!]"),
    ("", "empty geohash"),
], ids=["upper", "upper-last", "a", "bang", "empty"])
def test_invalid_geohash_point(client, geo, value, reason):
    r = client.put(f"/{geo}/_doc/bad", json={"p": value})
    assert r.status_code == 400
    err = r.json()["error"]
    assert err["type"] == "mapper_parsing_exception"
    assert err["reason"] == "failed to parse field [p] of type [geo_point]"
    assert err["caused_by"]["type"] == "parse_exception"
    assert err["caused_by"]["reason"] == reason


def test_geo_distance_sort_from_geohash_origin(client, geo):
    r = client.post(f"/{geo}/_search", json={"_source": False, "sort": [
        {"_geo_distance": {"p": "u4pruydqqvj", "order": "asc", "unit": "km"}}]})
    hits = [(h["_id"], h["sort"][0]) for h in r.json()["hits"]["hits"]]
    assert [h[0] for h in hits][:3] == ["1", "5", "2"]
    assert hits[0][1] == 0.0
    assert hits[2][1] == pytest.approx(649.317, rel=1e-4)


def test_geo_distance_query_from_geohash_origin(client, geo):
    hits = _hits(client, geo, {"query": {"geo_distance": {"distance": "500km", "p": "u4pr"}}})
    assert list(hits) == ["1", "5"]


def _agg(client, idx, agg):
    r = client.post(f"/{idx}/_search", json={"size": 0, "aggs": {"g": agg}})
    assert r.status_code == 200, r.text
    return r.json()["aggregations"]["g"]["buckets"]


@pytest.mark.parametrize("precision,buckets", [
    (1, [("u", 3), ("d", 2), ("s", 1), ("r", 1)]),
    (3, [("u4p", 2), ("dr5", 2), ("u40", 1), ("s00", 1), ("rzz", 1)]),
    (5, [("u4pru", 2), ("dr5re", 2), ("u4000", 1), ("s0000", 1), ("rzzzz", 1)]),
    ("10km", [("u4pru", 2), ("dr5re", 2), ("u4000", 1), ("s0000", 1), ("rzzzz", 1)]),
    (12, [("dr5regw3ppyz", 1), ("dr5regw3pg6f", 1), ("u4pruydqqvj8", 1), ("u4pruydqqvj0", 1),
          ("u40000000000", 1), ("s00000000000", 1), ("rzzzzzzzmtm7", 1)]),
])
def test_geohash_grid_buckets(client, geo, precision, buckets):
    got = _agg(client, geo, {"geohash_grid": {"field": "p", "precision": precision}})
    assert [(b["key"], b["doc_count"]) for b in got] == buckets


@pytest.mark.parametrize("precision,buckets", [
    (3, [("3/4/2", 3), ("3/2/3", 2), ("3/7/4", 1), ("3/4/4", 1)]),
    (9, [("9/270/155", 2), ("9/150/192", 2), ("9/511/256", 1), ("9/256/256", 1), ("9/256/158", 1)]),
])
def test_geotile_grid_buckets(client, geo, precision, buckets):
    got = _agg(client, geo, {"geotile_grid": {"field": "p", "precision": precision}})
    assert [(b["key"], b["doc_count"]) for b in got] == buckets


# --------------------------------------------------------------- completion geo contexts


@pytest.fixture
def places(client, index_factory):
    idx = index_factory(mappings={"properties": {"s": {"type": "completion", "contexts": [
        {"name": "loc", "type": "geo", "precision": 4}]}}},
        settings={"number_of_shards": 1, "number_of_replicas": 0})
    docs = {"1": {"input": "pizza berlin", "contexts": {"loc": {"lat": 52.52, "lon": 13.405}}},
            "2": {"input": "pizza paris", "contexts": {"loc": "u09tvw"}},
            "3": {"input": "pizza potsdam", "contexts": {"loc": [{"lat": 52.39, "lon": 13.06}]}},
            "4": {"input": "pizza nyc", "contexts": {"loc": "dr5regw3pg6f"}}}
    for i, s in docs.items():
        assert client.put(f"/{idx}/_doc/{i}", json={"s": s}).status_code == 201
    client.post(f"/{idx}/_refresh")
    return idx


def _suggest(client, idx, contexts):
    r = client.post(f"/{idx}/_search", json={"_source": False, "suggest": {"x": {
        "prefix": "pizza", "completion": {"field": "s", "size": 10, "contexts": contexts}}}})
    assert r.status_code == 200, r.text
    return [(o["_id"], o["contexts"]) for o in r.json()["suggest"]["x"][0]["options"]]


@pytest.mark.parametrize("contexts", [{"loc": {"lat": 52.52, "lon": 13.405}}, {"loc": "u33d"}],
                         ids=["point", "geohash"])
def test_completion_geo_context_precision(client, places, contexts):
    assert _suggest(client, places, contexts) == [("1", {"loc": ["u33d"]}), ("3", {"loc": ["u336"]})]


def test_completion_geo_context_geohash_input_truncated(client, places):
    assert _suggest(client, places, {"loc": "u09t"}) == [("2", {"loc": ["u09t"]})]
    assert _suggest(client, places, {"loc": "u09tvw"}) == [("2", {"loc": ["u09t"]})]
    assert _suggest(client, places, {"loc": "dr5r"}) == [("4", {"loc": ["dr5r"]})]
