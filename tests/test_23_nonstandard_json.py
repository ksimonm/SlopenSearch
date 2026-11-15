"""Jackson's non-standard number tokens (NaN, Infinity, ...) in document bodies.

OpenSearch reads JSON with Jackson, which rejects these tokens with a json_parse_exception.
Where the error surfaces and how it is wrapped depends on the API: per item in _bulk,
for the whole request in _update and in bulk update items, and as a parse_exception
once an ingest pipeline is involved. The byte offset points just past the token and is
counted from the start of the document (or bulk line) that holds it.
"""
import uuid

import pytest

pytestmark = pytest.mark.documents

NDJSON = {"Content-Type": "application/x-ndjson"}
JSON = {"Content-Type": "application/json"}


def _jackson(token, offset):
    reason = (f"Non-standard token '{token}': enable `JsonReadFeature.ALLOW_NON_NUMERIC_NUMBERS` to allow\n"
              " at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); "
              f"byte offset: #{offset}]")
    return {"type": "json_parse_exception", "reason": reason,
            "caused_by": {"type": "stream_read_exception", "reason": reason}}


def _bulk(client, lines, **params):
    return client.post("/_bulk", content=("\n".join(lines) + "\n").encode(), headers=NDJSON, params=params)


def _raw(client, method, url, raw, **params):
    return client.request(method, url, content=raw.encode(), headers=JSON, params=params)


@pytest.fixture
def idx(index_factory):
    return index_factory(mappings={"properties": {
        "n": {"type": "float"}, "v": {"type": "knn_vector", "dimension": 2}}},
        settings={"index.knn": True, "number_of_shards": 1, "number_of_replicas": 0})


@pytest.fixture
def pipeline(client):
    pid = f"slopensearch-nonstd-{uuid.uuid4().hex[:8]}"
    r = client.put(f"/_ingest/pipeline/{pid}", json={"processors": [{"set": {"field": "a", "value": 1}}]})
    assert r.status_code == 200, r.text
    yield pid
    client.delete(f"/_ingest/pipeline/{pid}")


def _count(client, idx):
    client.post(f"/{idx}/_refresh")
    return client.get(f"/{idx}/_count").json()["count"]


# ------------------------------------------------------------------- _bulk index/create


@pytest.mark.parametrize("token,doc,offset", [
    ("NaN", '{"n":NaN}', 8),
    ("Infinity", '{"n":Infinity}', 13),
    ("-Infinity", '{"n":-Infinity}', 14),
    ("+Infinity", '{"n":+Infinity}', 14),
    ("NaN", '{"m":"x","n":[1, NaN]}', 20),
])
def test_bulk_index_item_fails(client, idx, token, doc, offset):
    r = _bulk(client, [f'{{"index":{{"_index":"{idx}","_id":"1"}}}}', doc])
    assert r.status_code == 200, r.text
    body = r.json()
    assert body["errors"] is True
    assert body["items"] == [{"index": {
        "_index": idx, "_id": "1", "status": 400,
        "error": {"type": "mapper_parsing_exception", "reason": "failed to parse",
                  "caused_by": _jackson(token, offset)}}}]


def test_bulk_create_item_fails(client, idx):
    r = _bulk(client, [f'{{"create":{{"_index":"{idx}","_id":"4"}}}}', '{"n":Infinity}'])
    item = r.json()["items"][0]["create"]
    assert item["status"] == 400
    assert item["error"] == {"type": "mapper_parsing_exception", "reason": "failed to parse",
                             "caused_by": _jackson("Infinity", 13)}


def test_bulk_knn_vector_item_fails_with_field_error(client, idx):
    r = _bulk(client, [f'{{"index":{{"_index":"{idx}","_id":"1"}}}}', '{"v":[NaN,1]}'])
    assert r.json()["items"][0]["index"]["error"] == {
        "type": "mapper_parsing_exception",
        "reason": "failed to parse field [v] of type [knn_vector] in document with id '1'. "
                  "Preview of field's value: '[1]'",
        "caused_by": _jackson("NaN", 9)}


def test_bulk_other_items_still_indexed(client, idx):
    r = _bulk(client, [
        f'{{"index":{{"_index":"{idx}","_id":"1"}}}}', '{"n":1}',
        f'{{"index":{{"_index":"{idx}","_id":"2"}}}}', '{"n":NaN}',
        f'{{"index":{{"_index":"{idx}","_id":"3"}}}}', '{"v":[1,2]}',
    ])
    assert [i["index"]["status"] for i in r.json()["items"]] == [201, 400, 201]
    assert _count(client, idx) == 2


def test_bulk_action_line_fails_whole_request(client, idx):
    r = _bulk(client, [
        f'{{"index":{{"_index":"{idx}","_id":"1"}}}}', '{"n":1}',
        f'{{"index":{{"_index":"{idx}","_id":NaN}}}}', '{"n":1}',
    ])
    assert r.status_code == 400
    offset = len(f'{{"index":{{"_index":"{idx}","_id":NaN')
    cause = _jackson("NaN", offset)
    assert r.json() == {"error": {"root_cause": [{"type": cause["type"], "reason": cause["reason"]}], **cause},
                        "status": 400}
    assert _count(client, idx) == 0


# ------------------------------------------------------------------- updates


def _update_error(token, offset):
    reason = "[1:9] [UpdateRequest] failed to parse field [doc]"
    return {"error": {"root_cause": [{"type": "x_content_parse_exception", "reason": reason}],
                      "type": "x_content_parse_exception", "reason": reason,
                      "caused_by": _jackson(token, offset)},
            "status": 400}


def test_update_api_doc(client, idx):
    assert client.put(f"/{idx}/_doc/1", json={"n": 1}).status_code == 201
    r = _raw(client, "POST", f"/{idx}/_update/1", '{"doc":{"n":NaN}}')
    assert r.status_code == 400
    assert r.json() == _update_error("NaN", 15)
    assert client.get(f"/{idx}/_doc/1").json()["_source"] == {"n": 1}


def test_bulk_update_fails_whole_request(client, idx):
    r = _bulk(client, [
        f'{{"index":{{"_index":"{idx}","_id":"7"}}}}', '{"n":1}',
        f'{{"update":{{"_index":"{idx}","_id":"7"}}}}', '{"doc":{"n":-Infinity}}',
    ], refresh="true")
    assert r.status_code == 400
    assert r.json() == _update_error("-Infinity", 21)
    assert _count(client, idx) == 0


# ------------------------------------------------------------------- ingest


def _ingest_error(token, offset):
    reason = "Failed to parse content to map"
    return {"error": {"root_cause": [{"type": "parse_exception", "reason": reason}],
                      "type": "parse_exception", "reason": reason,
                      "caused_by": _jackson(token, offset)},
            "status": 400}


def test_index_with_pipeline(client, idx, pipeline):
    r = _raw(client, "PUT", f"/{idx}/_doc/5", '{"v":[NaN,1]}', pipeline=pipeline)
    assert r.status_code == 400
    assert r.json() == _ingest_error("NaN", 9)


def test_index_with_default_pipeline(client, index_factory, pipeline):
    name = index_factory(settings={"index.default_pipeline": pipeline})
    r = _raw(client, "POST", f"/{name}/_doc", '{"x":{"y":+Infinity}}')
    assert r.status_code == 400
    assert r.json() == _ingest_error("+Infinity", 19)


def test_bulk_with_pipeline_item_fails(client, idx, pipeline):
    r = _bulk(client, [f'{{"index":{{"_index":"{idx}","_id":"6","pipeline":"{pipeline}"}}}}', '{"n":NaN}'])
    assert r.status_code == 200, r.text
    body = r.json()
    assert body["errors"] is True
    assert body["items"] == [{"index": {
        "_index": idx, "_id": "6", "status": 400,
        "error": {"type": "parse_exception", "reason": "Failed to parse content to map",
                  "caused_by": _jackson("NaN", 8)}}}]


def test_simulate_inline_pipeline(client):
    raw = ('{"pipeline":{"processors":[{"set":{"field":"a","value":1}}]},'
           '"docs":[{"_source":{"n":NaN}}]}')
    r = _raw(client, "POST", "/_ingest/pipeline/_simulate", raw)
    assert r.status_code == 400
    assert r.json() == _ingest_error("NaN", raw.index("NaN") + 3)


def test_simulate_stored_pipeline(client, pipeline):
    raw = '{"docs":[{"_source":{"n":NaN}}]}'
    r = _raw(client, "POST", f"/_ingest/pipeline/{pipeline}/_simulate", raw)
    assert r.status_code == 400
    assert r.json() == _ingest_error("NaN", 28)
