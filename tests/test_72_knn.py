"""Vector search: knn_vector fields, the knn query, knn_score scripts and painless vector functions.

Black-box compatibility spec recorded against OpenSearch 3.9.0 (full distribution, k-NN plugin).
Datasets are tiny (2-8 dimensions, a handful of docs, one shard), so both the faiss and the
lucene HNSW graphs return exact neighbours and the scores below are exact.

Score mappings OpenSearch uses (d = distance between query and doc vector):
  l2           1 / (1 + d^2)
  cosinesimil  (1 + cos) / 2
  innerproduct 1 + ip when ip >= 0, else 1 / (1 - ip)
  l1, linf     1 / (1 + d)            (knn_score script only; HNSW does not support them)
  hamming      1 / (1 + hamming bits) (binary vectors)
"""

import base64
import json
import struct
import uuid

import pytest

pytestmark = pytest.mark.knn

SETTINGS = {"number_of_shards": 1, "number_of_replicas": 0}
KNN = {"index.knn": True}

# doc id -> vector; "t" is "odd" for docs 1, 3, 5 and "even" for 2, 4; "n" is the id.
VECS = {"1": [0, 0], "2": [1, 0], "3": [0, 2], "4": [3, 4], "5": [-1, -1]}
Q = [1, 1]
L2_SCORES = [("2", 0.5), ("1", 0.33333334), ("3", 0.33333334), ("5", 0.11111111), ("4", 0.071428575)]
COS_SCORES = [("4", 0.99497473), ("2", 0.8535534), ("3", 0.8535534), ("5", 0.0)]
IP_SCORES = [("4", 8.0), ("3", 3.0), ("2", 2.0), ("1", 1.0), ("5", 0.33333334)]


# --------------------------------------------------------------------------- helpers


def _name(prefix="k72"):
    return f"{prefix}-{uuid.uuid4().hex[:10]}"


def _bulk(client, index, docs):
    """docs: dict id -> source."""
    lines = []
    for doc_id, src in docs.items():
        lines.append(json.dumps({"index": {"_id": doc_id}}))
        lines.append(json.dumps(src))
    r = client.post(
        f"/{index}/_bulk",
        content=("\n".join(lines) + "\n").encode(),
        headers={"Content-Type": "application/x-ndjson"},
        params={"refresh": "true"},
    )
    assert r.status_code == 200, r.text
    assert r.json()["errors"] is False, r.text


def _base_docs(vecs=VECS):
    return {i: {"v": v, "n": int(i), "t": "odd" if int(i) % 2 else "even"} for i, v in vecs.items()}


@pytest.fixture
def mk(client, index_factory):
    """Create a one-shard index with the given properties (index.knn on by default) and docs."""

    def _mk(props, knn=True, docs=None):
        settings = dict(SETTINGS, **(KNN if knn else {}))
        name = index_factory(name=_name(), mappings={"properties": props}, settings=settings)
        if docs:
            _bulk(client, name, docs)
        return name

    return _mk


@pytest.fixture
def vec_index(mk):
    """Index the five VECS docs into a 2-dim field "v" (extra field params merged in)."""

    def _vec_index(field=None, knn=True):
        field = field or {}
        props = {"v": dict({"type": "knn_vector", "dimension": 2}, **field),
                 "n": {"type": "integer"}, "t": {"type": "keyword"}}
        docs = _base_docs()
        if "cosinesimil" in json.dumps(field):
            del docs["1"]  # an ANN cosinesimil field rejects the zero vector
        return mk(props, knn=knn, docs=docs)

    return _vec_index


def _search(client, index, body, **params):
    return client.post(f"/{index}/_search", json=body, params=params)


def _ok(client, index, body, **params):
    r = _search(client, index, body, **params)
    assert r.status_code == 200, r.text
    return r.json()


def _hits(client, index, body, **params):
    body = dict(body)
    body.setdefault("_source", False)
    res = _ok(client, index, body, **params)
    return [(h["_id"], h["_score"]) for h in res["hits"]["hits"]]


def _knn(vector=Q, field="v", **kw):
    return {"knn": {field: dict({"vector": vector}, **kw)}}


def _ids(pairs):
    return [i for i, _ in pairs]


def _error(resp):
    return resp.json()["error"]


def _cause(resp):
    """(type, reason) of the innermost interesting cause of an error response."""
    e = _error(resp)
    c = e.get("caused_by") or e
    return c["type"], c["reason"]


def _shard_cause(resp):
    """For search_phase_execution_exception: (type, reason) of the first shard failure."""
    e = _error(resp)
    assert e["type"] == "search_phase_execution_exception", resp.text
    reason = e["failed_shards"][0]["reason"]
    return reason["type"], reason["reason"]


def _put(client, index, doc, doc_id="x"):
    return client.put(f"/{index}/_doc/{doc_id}", json=doc, params={"refresh": "true"})


def _put_raw(client, index, raw, doc_id="x"):
    return client.put(
        f"/{index}/_doc/{doc_id}",
        content=raw.encode(),
        headers={"Content-Type": "application/json"},
        params={"refresh": "true"},
    )


def _props(client, index):
    return client.get(f"/{index}/_mapping").json()[index]["mappings"]["properties"]


@pytest.fixture
def try_create(client):
    """Create an index that is expected to (possibly) fail; always clean up."""
    names = []

    def _create(field, knn=True):
        name = _name("k72e")
        names.append(name)
        body = {"settings": dict(SETTINGS, **(KNN if knn else {})),
                "mappings": {"properties": {"v": field}}}
        return client.put(f"/{name}", json=body)

    yield _create
    for n in names:
        client.delete(f"/{n}", params={"ignore_unavailable": "true"})


# =========================================================================== mapping


def test_mapping_knn_index_defaults(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 2}})
    # with index.knn the field does not echo doc_values (it does without, see test_71)
    assert _props(client, name)["v"] == {"type": "knn_vector", "dimension": 2}


def test_mapping_without_knn_setting_echoes_doc_values(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 2, "space_type": "l1"}}, knn=False)
    assert _props(client, name)["v"] == {"type": "knn_vector", "doc_values": True, "dimension": 2,
                                         "space_type": "l1"}


def test_mapping_method_defaults_filled_in(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 2, "method": {"name": "hnsw"}}})
    v = _props(client, name)["v"]
    assert v == {"type": "knn_vector", "dimension": 2,
                 "method": {"engine": "faiss", "space_type": "l2", "name": "hnsw", "parameters": {}}}
    assert list(v["method"]) == ["engine", "space_type", "name", "parameters"]


def test_mapping_lucene_with_top_level_space_type(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 2, "space_type": "l2",
                                     "method": {"name": "hnsw", "engine": "lucene"}}})
    v = _props(client, name)["v"]
    assert v == {"type": "knn_vector", "dimension": 2,
                 "method": {"engine": "lucene", "space_type": "l2", "name": "hnsw", "parameters": {}},
                 "space_type": "l2"}
    assert list(v) == ["type", "dimension", "method", "space_type"]


def test_mapping_faiss_parameters_round_trip(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 2, "method": {
        "name": "hnsw", "engine": "faiss", "space_type": "innerproduct",
        "parameters": {"m": 16, "ef_construction": 100}}}})
    assert _props(client, name)["v"]["method"] == {
        "engine": "faiss", "space_type": "innerproduct", "name": "hnsw",
        "parameters": {"ef_construction": 100, "m": 16}}


@pytest.mark.parametrize("data_type,dim,extra,shown", [
    ("byte", 2, {}, "BYTE"),
    ("half_float", 2, {}, "HALF_FLOAT"),
    ("binary", 8, {"space_type": "hamming"}, "BINARY"),
])
def test_mapping_data_type_is_upper_cased(client, mk, data_type, dim, extra, shown):
    name = mk({"v": dict({"type": "knn_vector", "dimension": dim, "data_type": data_type}, **extra)})
    assert _props(client, name)["v"] == dict({"type": "knn_vector", "dimension": dim, "data_type": shown}, **extra)


@pytest.mark.parametrize("param,value", [("mode", "on_disk"), ("mode", "in_memory"), ("compression_level", "16x")])
def test_mapping_mode_and_compression_round_trip(client, mk, param, value):
    name = mk({"v": {"type": "knn_vector", "dimension": 2, param: value}})
    assert _props(client, name)["v"] == {"type": "knn_vector", "dimension": 2, param: value}


def test_mapping_cosinesimil_top_level(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 2, "space_type": "cosinesimil"}})
    assert _props(client, name)["v"] == {"type": "knn_vector", "dimension": 2, "space_type": "cosinesimil"}


def test_index_knn_settings(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 2}})
    idx = client.get(f"/{name}/_settings").json()[name]["settings"]["index"]
    assert idx["knn"] == "true"
    assert idx["knn.derived_source"] == {"enabled": "true"}


def test_index_knn_is_not_dynamic(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 2}})
    r = client.put(f"/{name}/_settings", json={"index.knn": False})
    assert r.status_code == 400
    e = _error(r)
    assert e["type"] == "illegal_argument_exception"
    assert e["reason"].startswith("Can't update non dynamic settings [[index.knn]] for open indices [[" + name + "/")


def test_max_dimension_16000(try_create):
    assert try_create({"type": "knn_vector", "dimension": 16000}).status_code == 200


def test_dimension_limit_does_not_apply_without_index_knn(try_create):
    assert try_create({"type": "knn_vector", "dimension": 16001}, knn=False).status_code == 200


MAPPING_ERRORS = [
    ({"dimension": 0}, "illegal_argument_exception", "Dimension value must be greater than 0 for vector: v"),
    ({"dimension": -1}, "illegal_argument_exception", "Dimension value must be greater than 0 for vector: v"),
    ({"dimension": "abc"}, "illegal_argument_exception",
     "Unable to parse [dimension] from provided value [abc] for vector [v]"),
    ({"dimension": 16001}, "validation_exception",
     "Validation Failed: 1: Dimension value cannot be greater than 16000 for vector with engine: faiss;"),
    ({"dimension": 2, "data_type": "int"}, "illegal_argument_exception",
     "Invalid value provided for [data_type] field. Supported values are [binary,byte,float,half_float]"),
    ({"dimension": 7, "data_type": "binary"}, "validation_exception",
     "Validation Failed: 1: Dimension should be multiply of 8 for binary vector data type;"),
    ({"dimension": 2, "space_type": "foo"}, "illegal_argument_exception",
     "Unable to find space: foo . Valid values are: [l2, cosinesimil, l1, linf, innerproduct, hamming]"),
    ({"dimension": 2, "space_type": "hamming"}, "illegal_argument_exception",
     "Space type [hamming] is not supported with [float] data type"),
    ({"dimension": 8, "data_type": "binary", "space_type": "l2"}, "illegal_argument_exception",
     "Space type [l2] is not supported with [binary] data type"),
    ({"dimension": 2, "space_type": "l1"}, "validation_exception",
     'Validation Failed: 1: "hnsw" with "faiss" configuration does not support space type: "l1".;'),
    ({"dimension": 2, "space_type": "linf"}, "validation_exception",
     'Validation Failed: 1: "hnsw" with "faiss" configuration does not support space type: "linf".;'),
    ({"dimension": 2, "method": {"name": "hnsw", "engine": "lucene", "space_type": "l1"}}, "validation_exception",
     'Validation Failed: 1: "hnsw" with "lucene" configuration does not support space type: "l1".;'),
    ({"dimension": 2, "method": {"name": "foo", "engine": "lucene"}}, "illegal_argument_exception",
     "Invalid method name: foo"),
    ({"dimension": 2, "method": {"name": "ivf", "engine": "lucene"}}, "illegal_argument_exception",
     "Invalid method name: ivf"),
    ({"dimension": 2, "method": {"name": "ivf", "engine": "faiss"}}, "validation_exception",
     'Validation Failed: 1: "method" requires training.;'),
    ({"dimension": 2, "method": {"name": "hnsw", "engine": "nmslib"}}, "illegal_argument_exception",
     "nmslib engine is deprecated in OpenSearch  and cannot be used for new index creation in OpenSearch from  3.0.0."),
    ({"dimension": 2, "method": {"name": "hnsw", "engine": "lucene", "parameters": {"m": "x"}}},
     "validation_exception", "Validation Failed: 1: value is not an instance of Integer for Integer parameter [m].;"),
    ({"dimension": 2, "method": {"name": "hnsw", "engine": "lucene", "parameters": {"foo": 1}}},
     "validation_exception", "Validation Failed: 1: Unknown parameter 'foo' found;"),
]


@pytest.mark.parametrize("field,cause_type,cause_reason", MAPPING_ERRORS,
                         ids=[json.dumps(f, sort_keys=True) for f, _, _ in MAPPING_ERRORS])
def test_mapping_validation_errors(try_create, field, cause_type, cause_reason):
    r = try_create(dict({"type": "knn_vector"}, **field))
    assert r.status_code == 400, r.text
    e = _error(r)
    assert e["type"] == "mapper_parsing_exception"
    assert e["reason"] == f"Failed to parse mapping [_doc]: {cause_reason}"
    assert _cause(r) == (cause_type, cause_reason)


MAPPER_ERRORS = [
    ({"dimension": 2, "method": {"name": "hnsw", "engine": "foo"}}, "Invalid engine: foo"),
    ({"dimension": 2, "method": {"engine": "lucene"}}, "name needs to be set"),
    ({"dimension": 2, "mode": "foo"},
     "Unknown value [foo] for field [mode] - accepted values are [null, in_memory, on_disk]"),
    ({"dimension": 2, "compression_level": "3x"},
     "Unknown value [3x] for field [compression_level] - accepted values are [, 1x, 2x, 4x, 8x, 16x, 32x]"),
    ({"dimension": 2, "space_type": "l2", "method": {"name": "hnsw", "space_type": "innerproduct"}},
     'Cannot specify conflicting space types: "[innerproduct]" "[l2]"'),
    ({"dimension": 2, "foo": 1}, "unknown parameter [foo] on mapper [v] of type [knn_vector]"),
]


@pytest.mark.parametrize("field,cause_reason", MAPPER_ERRORS,
                         ids=[json.dumps(f, sort_keys=True) for f, _ in MAPPER_ERRORS])
def test_mapping_parser_errors(try_create, field, cause_reason):
    r = try_create(dict({"type": "knn_vector"}, **field))
    assert r.status_code == 400, r.text
    e = _error(r)
    assert e["type"] == "mapper_parsing_exception"
    assert e["reason"] == f"Failed to parse mapping [_doc]: {cause_reason}"
    assert _cause(r) == ("mapper_parsing_exception", cause_reason)


def test_mapping_update_dimension_conflict(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 2}})
    r = client.put(f"/{name}/_mapping", json={"properties": {"v": {"type": "knn_vector", "dimension": 3}}})
    assert r.status_code == 400
    assert _error(r)["type"] == "illegal_argument_exception"
    assert _error(r)["reason"] == ("Mapper for [v] conflicts with existing mapper:\n"
                                   "\tCannot update parameter [dimension] from [2] to [3]")


def test_mapping_update_space_type_conflict(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 2}})
    r = client.put(f"/{name}/_mapping", json={"properties": {"v": {
        "type": "knn_vector", "dimension": 2, "space_type": "cosinesimil"}}})
    assert r.status_code == 400
    assert _error(r)["reason"] == ("Mapper for [v] conflicts with existing mapper:\n"
                                   "\tCannot update parameter [space_type] from [undefined] to [cosinesimil]")


def test_mapping_update_adds_vector_field(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 2}})
    r = client.put(f"/{name}/_mapping", json={"properties": {"w": {"type": "knn_vector", "dimension": 4}}})
    assert r.status_code == 200, r.text
    assert _props(client, name)["w"] == {"type": "knn_vector", "dimension": 4}


# =========================================================================== indexing


@pytest.fixture
def vidx(mk):
    return mk({
        "v": {"type": "knn_vector", "dimension": 2},
        "b": {"type": "knn_vector", "dimension": 2, "data_type": "byte"},
        "w": {"type": "knn_vector", "dimension": 8, "data_type": "binary", "space_type": "hamming"},
    })


def _parse_failure(r, field, preview):
    assert r.status_code == 400, r.text
    e = _error(r)
    assert e["type"] == "mapper_parsing_exception"
    assert e["reason"] == (f"failed to parse field [{field}] of type [knn_vector] in document with id 'x'. "
                           f"Preview of field's value: '{preview}'")
    return e["caused_by"]


@pytest.mark.parametrize("value,preview,given", [
    ([1, 2, 3], "null", 3),
    ([1], "null", 1),
    ([], "null", 0),
    (5, "null", 1),
    ({"a": 1}, "{a=1}", 0),
])
def test_index_wrong_dimension(client, vidx, value, preview, given):
    cause = _parse_failure(_put(client, vidx, {"v": value}), "v", preview)
    assert cause == {"type": "illegal_argument_exception",
                     "reason": f"Vector dimension mismatch. Expected: 2, Given: {given}"}


def test_index_non_numeric_value(client, vidx):
    cause = _parse_failure(_put(client, vidx, {"v": ["a", "b"]}), "v", "a")
    assert cause == {"type": "number_format_exception", "reason": 'For input string: "a"'}


def test_index_infinity_rejected(client, vidx):
    cause = _parse_failure(_put(client, vidx, {"v": [1e40, 1]}), "v", "Infinity")
    assert cause == {"type": "illegal_argument_exception", "reason": "KNN vector values cannot be infinity"}


def test_index_nan_literal_rejected(client, vidx):
    cause = _parse_failure(_put_raw(client, vidx, '{"v": [NaN, 1]}'), "v", "[1]")
    assert cause["type"] == "json_parse_exception"
    assert cause["reason"].startswith("Non-standard token 'NaN'")


@pytest.mark.parametrize("value", [[None, 1], [[1, 2], [3, 4]]], ids=["null_element", "nested_array"])
def test_index_non_numeric_tokens(client, vidx, value):
    r = _put(client, vidx, {"v": value})
    assert r.status_code == 400
    assert _error(r)["caused_by"]["type"] == "input_coercion_exception"


def test_index_invalid_base64_string(client, vidx):
    cause = _parse_failure(_put(client, vidx, {"v": "NaN"}), "v", "NaN")
    assert cause == {"type": "illegal_argument_exception",
                     "reason": "Base64 encoded vector for field [v] has invalid byte length [2], "
                               "must be a multiple of 4 (float size)"}


def test_index_base64_little_endian_vector(client, vidx):
    raw = base64.b64encode(struct.pack("<2f", 1.0, 2.5)).decode()
    assert _put(client, vidx, {"v": raw}).status_code == 201
    assert client.get(f"/{vidx}/_doc/x").json()["_source"] == {"v": [1.0, 2.5]}
    assert _hits(client, vidx, {"query": _knn([1, 2.5], k=1)}) == [("x", 1.0)]


def test_index_numeric_strings_accepted_and_normalized(client, vidx):
    assert _put(client, vidx, {"v": ["1", "2.5"]}).status_code == 201
    # index.knn derives the vector in _source from the stored vector
    assert client.get(f"/{vidx}/_doc/x").json()["_source"] == {"v": [1.0, 2.5]}


def test_source_kept_verbatim_without_index_knn(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 2}}, knn=False)
    assert _put(client, name, {"v": ["1", "2.5"]}).status_code == 201
    assert client.get(f"/{name}/_doc/x").json()["_source"] == {"v": ["1", "2.5"]}


def test_source_round_trip_ints_become_floats(client, vidx):
    assert _put(client, vidx, {"v": [1, 2], "b": [1, -2], "w": [5]}).status_code == 201
    src = client.get(f"/{vidx}/_doc/x").json()["_source"]
    assert src == {"v": [1.0, 2.0], "b": [1, -2], "w": [5]}
    assert json.dumps(src["v"]) == "[1.0, 2.0]"


def test_null_and_missing_vectors(client, vidx):
    assert _put(client, vidx, {"v": None}, "1").status_code == 201
    assert _put(client, vidx, {"b": [1, 1]}, "2").status_code == 201
    assert _put(client, vidx, {"v": [1, 1]}, "3").status_code == 201
    assert client.get(f"/{vidx}/_doc/1").json()["_source"] == {"v": None}
    assert _hits(client, vidx, {"query": {"exists": {"field": "v"}}}) == [("3", 1.0)]
    assert _hits(client, vidx, {"query": _knn(k=10)}) == [("3", 1.0)]


@pytest.mark.parametrize("value,ok", [([127, -128], True), ([128, 0], False), ([-129, 0], False)])
def test_byte_range(client, vidx, value, ok):
    r = _put(client, vidx, {"b": value})
    if ok:
        assert r.status_code == 201
        return
    cause = _parse_failure(r, "b", str(value[0]))
    assert cause == {"type": "illegal_argument_exception",
                     "reason": "[data_type] field was set as [byte] in index mapping. But, KNN vector values "
                               "are not within in the byte range [-128, 127]"}


def test_byte_rejects_fractions(client, vidx):
    cause = _parse_failure(_put(client, vidx, {"b": [1.5, 2]}), "b", "1.5")
    assert cause["reason"] == ("[data_type] field was set as [byte] in index mapping. But, KNN vector values "
                               "are floats instead of byte integers")


def test_binary_vector_length_is_dimension_over_8(client, vidx):
    assert _put(client, vidx, {"w": [-1]}).status_code == 201
    cause = _parse_failure(_put(client, vidx, {"w": [1, 2]}), "w", "null")
    assert cause["reason"] == ("The dimension of the binary vector must be 8 times the length of the "
                               "provided vector. Expected: 8, Given: 16")


def test_binary_vector_byte_range(client, vidx):
    cause = _parse_failure(_put(client, vidx, {"w": [300]}), "w", "300")
    assert cause["reason"] == ("[data_type] field was set as [binary] in index mapping. But, KNN vector values "
                               "are not within in the byte range [-128, 127]")


def test_fields_and_docvalue_fields(client, vec_index):
    name = vec_index()
    res = _ok(client, name, {"query": {"ids": {"values": ["2"]}}, "_source": False,
                             "fields": ["v"], "docvalue_fields": ["v"]})
    # fields returns the float array; docvalue_fields alone returns the raw little-endian floats, base64
    assert res["hits"]["hits"][0]["fields"] == {"v": [1.0, 0.0]}
    res = _ok(client, name, {"query": {"ids": {"values": ["2"]}}, "_source": False, "docvalue_fields": ["v"]})
    assert res["hits"]["hits"][0]["fields"] == {"v": [base64.b64encode(struct.pack("<2f", 1, 0)).decode()]}


def test_sort_on_vector_unsupported(client, vec_index):
    name = vec_index()
    r = _search(client, name, {"sort": ["v"]})
    assert r.status_code == 500
    assert _shard_cause(r) == ("unsupported_operation_exception", "knn vector field doesn't support this operation")


def test_term_query_on_vector_unsupported(client, vec_index):
    name = vec_index()
    r = _search(client, name, {"query": {"term": {"v": 1}}})
    assert r.status_code == 400
    assert _shard_cause(r) == ("query_shard_exception",
                               f"KNN vector do not support exact searching, use KNN queries instead: [v]")


# =========================================================================== knn query: scores


@pytest.mark.parametrize("engine", ["faiss", "lucene"])
@pytest.mark.parametrize("space,expected", [("l2", L2_SCORES), ("cosinesimil", COS_SCORES),
                                            ("innerproduct", IP_SCORES)])
def test_knn_scores_per_space_type(client, vec_index, engine, space, expected):
    name = vec_index({"method": {"name": "hnsw", "engine": engine, "space_type": space}})
    assert _hits(client, name, {"query": _knn(k=5)}) == expected


@pytest.mark.parametrize("engine", ["faiss", "lucene"])
def test_cosinesimil_rejects_zero_vector(client, mk, engine):
    name = mk({"v": {"type": "knn_vector", "dimension": 2,
                     "method": {"name": "hnsw", "engine": engine, "space_type": "cosinesimil"}}})
    r = _put(client, name, {"v": [0, 0]})
    cause = _parse_failure(r, "v", "null")
    assert cause == {"type": "illegal_argument_exception",
                     "reason": "zero vector is not supported when space type is [cosinesimil]"}


def test_knn_default_mapping_is_faiss_l2(client, vec_index):
    name = vec_index()
    assert _hits(client, name, {"query": _knn(k=5)}) == L2_SCORES


@pytest.mark.parametrize("field", [{"data_type": "byte"}, {"data_type": "half_float"}, {"mode": "on_disk"},
                                   {"compression_level": "16x"}])
def test_knn_scores_other_storage(client, vec_index, field):
    name = vec_index(field)
    assert _hits(client, name, {"query": _knn(k=5)}) == L2_SCORES


def test_knn_byte_lucene_cosine(client, vec_index):
    name = vec_index({"data_type": "byte", "space_type": "cosinesimil",
                                      "method": {"name": "hnsw", "engine": "lucene"}})
    assert _hits(client, name, {"query": _knn(k=5)}) == COS_SCORES


def test_knn_binary_hamming(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 8, "data_type": "binary",
                                     "space_type": "hamming"}},
               docs={"1": {"v": [0]}, "2": {"v": [1]}, "3": {"v": [3]}, "4": {"v": [-1]}, "5": {"v": [127]}})
    assert _hits(client, name, {"query": _knn([7], k=5)}) == [
        ("3", 0.5), ("2", 0.33333334), ("1", 0.25), ("5", 0.2), ("4", 0.16666667)]


def test_knn_binary_query_dimension(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 8, "data_type": "binary",
                                     "space_type": "hamming"}}, docs={"1": {"v": [0]}})
    r = _search(client, name, {"query": _knn([7, 1], k=1)})
    assert r.status_code == 400
    assert _shard_cause(r) == ("query_shard_exception",
                               "failed to create query: Query vector has invalid dimension: 16. Dimension should be: 8")


@pytest.mark.parametrize("vector,reason", [
    ([1.5, 1], "are floats instead of byte integers"),
    ([200, 1], "are not within in the byte range [-128, 127]"),
])
def test_knn_byte_query_vector_validation(client, vec_index, vector, reason):
    name = vec_index({"data_type": "byte"})
    r = _search(client, name, {"query": _knn(vector, k=1)})
    assert r.status_code == 400
    assert _shard_cause(r) == ("query_shard_exception", "failed to create query: [data_type] field was set as "
                               f"[byte] in index mapping. But, KNN vector values {reason}")


# =========================================================================== knn query: k, size, paging


@pytest.fixture
def l2idx(vec_index):
    return vec_index()


def test_knn_k_limits_hits(client, l2idx):
    res = _ok(client, l2idx, {"query": _knn(k=2), "_source": False, "size": 5})
    assert res["hits"]["total"] == {"value": 2, "relation": "eq"}
    assert res["hits"]["max_score"] == 0.5
    assert [(h["_id"], h["_score"]) for h in res["hits"]["hits"]] == L2_SCORES[:2]


def test_knn_size_smaller_than_k(client, l2idx):
    res = _ok(client, l2idx, {"query": _knn(k=5), "_source": False, "size": 2})
    assert res["hits"]["total"] == {"value": 5, "relation": "eq"}
    assert [h["_id"] for h in res["hits"]["hits"]] == ["2", "1"]


def test_knn_k_larger_than_corpus(client, l2idx):
    assert _hits(client, l2idx, {"query": _knn(k=10000)}) == L2_SCORES


def test_knn_from_pages_within_k(client, l2idx):
    res = _ok(client, l2idx, {"query": _knn(k=2), "_source": False, "from": 1})
    assert res["hits"]["total"]["value"] == 2
    assert [(h["_id"], h["_score"]) for h in res["hits"]["hits"]] == [("1", 0.33333334)]


def test_knn_fractional_k_accepted(client, l2idx):
    assert _hits(client, l2idx, {"query": _knn(k=2.5)}) == L2_SCORES[:2]


def test_knn_sort_by_field(client, l2idx):
    res = _ok(client, l2idx, {"query": _knn(k=3), "_source": False, "sort": [{"n": "desc"}]})
    assert res["hits"]["max_score"] is None
    assert [(h["_id"], h["_score"], h["sort"]) for h in res["hits"]["hits"]] == [
        ("3", None, [3]), ("2", None, [2]), ("1", None, [1])]


def test_knn_count(client, l2idx):
    r = client.post(f"/{l2idx}/_count", json={"query": _knn(k=3)})
    assert r.status_code == 200
    assert r.json()["count"] == 3


def test_knn_track_total_hits_false(client, l2idx):
    res = _ok(client, l2idx, {"query": _knn(k=2), "_source": False, "track_total_hits": False})
    assert "total" not in res["hits"]
    assert [h["_id"] for h in res["hits"]["hits"]] == ["2", "1"]


def test_knn_search_min_score(client, l2idx):
    assert _hits(client, l2idx, {"query": _knn(k=2), "min_score": 0.4}) == [("2", 0.5)]


def test_knn_method_parameters_ef_search(client, l2idx):
    assert _hits(client, l2idx, {"query": _knn(k=2, method_parameters={"ef_search": 10})}) == L2_SCORES[:2]


def test_knn_rescore_oversample(client, l2idx):
    assert _hits(client, l2idx, {"query": _knn(k=2, rescore={"oversample_factor": 2})}) == L2_SCORES[:2]


def test_knn_boost(client, l2idx):
    assert _hits(client, l2idx, {"query": _knn(k=2, boost=2)}) == [("2", 1.0), ("1", 0.6666667)]


def test_knn_named_query(client, l2idx):
    res = _ok(client, l2idx, {"query": _knn(k=2, _name="kq"), "_source": False})
    assert [h["matched_queries"] for h in res["hits"]["hits"]] == [["kq"], ["kq"]]


# =========================================================================== knn query: filters, radial


def test_knn_efficient_filter(client, l2idx):
    # the filter is applied during the search: k results among the matching docs
    assert _hits(client, l2idx, {"query": _knn(k=2, filter={"term": {"t": "odd"}})}) == [
        ("1", 0.33333334), ("3", 0.33333334)]


def test_knn_filter_bool_must_not(client, l2idx):
    f = {"bool": {"must_not": {"term": {"n": 2}}}}
    assert _hits(client, l2idx, {"query": _knn(k=2, filter=f)}) == [("1", 0.33333334), ("3", 0.33333334)]


def test_knn_filter_matching_nothing(client, l2idx):
    res = _ok(client, l2idx, {"query": _knn(k=2, filter={"term": {"n": 99}}), "_source": False})
    assert res["hits"]["total"]["value"] == 0
    assert res["hits"]["max_score"] is None


def test_knn_post_filter_applies_after_k(client, l2idx):
    # k=3 picks 2, 1, 3 first; the post_filter then drops the even doc 2
    res = _ok(client, l2idx, {"query": _knn(k=3), "post_filter": {"term": {"t": "odd"}}, "_source": False})
    assert res["hits"]["total"]["value"] == 2
    assert [h["_id"] for h in res["hits"]["hits"]] == ["1", "3"]


def test_knn_bool_must_with_filter_is_post_filtering(client, l2idx):
    # unlike knn.filter, a bool filter runs on the top-k only
    q = {"bool": {"must": [_knn(k=2)], "filter": [{"term": {"t": "odd"}}]}}
    assert _hits(client, l2idx, {"query": q}) == [("1", 0.33333334)]


def test_knn_bool_should_adds_scores(client, l2idx):
    # a term query on a keyword field scores a constant 1.0
    q = {"bool": {"should": [_knn(k=2), {"term": {"t": "odd"}}]}}
    assert _hits(client, l2idx, {"query": q}) == [("1", 1.3333334), ("3", 1.0), ("5", 1.0), ("2", 0.5)]


def test_knn_in_bool_filter_scores_zero(client, l2idx):
    assert _hits(client, l2idx, {"query": {"bool": {"filter": [_knn(k=2)]}}}) == [("1", 0.0), ("2", 0.0)]


def test_knn_in_constant_score(client, l2idx):
    assert _hits(client, l2idx, {"query": {"constant_score": {"filter": _knn(k=2)}}}) == [("1", 1.0), ("2", 1.0)]


def test_knn_in_function_score(client, l2idx):
    q = {"function_score": {"query": _knn(k=2), "boost_mode": "replace",
                            "functions": [{"field_value_factor": {"field": "n"}}]}}
    assert _hits(client, l2idx, {"query": q}) == [("2", 2.0), ("1", 1.0)]


def test_knn_in_dis_max(client, l2idx):
    q = {"dis_max": {"queries": [_knn(k=1), _knn([3, 4], k=1)]}}
    assert _hits(client, l2idx, {"query": q}) == [("4", 1.0), ("2", 0.5)]


def test_knn_radial_min_score(client, l2idx):
    assert _hits(client, l2idx, {"query": _knn(min_score=0.3)}) == L2_SCORES[:3]


def test_knn_radial_max_distance_is_squared_l2(client, l2idx):
    # docs 5 and 4 are at squared distance 8 and 13
    assert _hits(client, l2idx, {"query": _knn(max_distance=5)}) == L2_SCORES[:3]
    assert _hits(client, l2idx, {"query": _knn(max_distance=2.5)}) == L2_SCORES[:3]


def test_knn_radial_max_distance_boundary_faiss_vs_lucene(client, vec_index, l2idx):
    # docs 1 and 3 are at squared distance exactly 2: faiss excludes the boundary,
    # lucene (and faiss with a filter, which searches exactly) include it
    assert _hits(client, l2idx, {"query": _knn(max_distance=2)}) == [("2", 0.5)]
    assert _hits(client, l2idx, {"query": _knn(max_distance=2, filter={"term": {"t": "odd"}})}) == [
        ("1", 0.33333334), ("3", 0.33333334)]
    lucene = vec_index({"method": {"name": "hnsw", "engine": "lucene"}})
    assert _hits(client, lucene, {"query": _knn(max_distance=2)}) == L2_SCORES[:3]


def test_knn_radial_max_distance_zero(client, l2idx):
    assert _hits(client, l2idx, {"query": _knn(max_distance=0)}) == []


def test_knn_radial_innerproduct(client, vec_index):
    name = vec_index({"space_type": "innerproduct"})
    assert _hits(client, name, {"query": _knn(min_score=2.5)}) == [("4", 8.0), ("3", 3.0)]
    # the distance of innerproduct is the negated inner product
    assert _hits(client, name, {"query": _knn(max_distance=-1.5)}) == [("4", 8.0), ("3", 3.0)]
    assert _hits(client, name, {"query": _knn(max_distance=2.5)}) == IP_SCORES


def test_knn_radial_cosine_distance(client, vec_index):
    name = vec_index({"space_type": "cosinesimil"})
    assert _hits(client, name, {"query": _knn(max_distance=0.5)}) == COS_SCORES[:3]


# =========================================================================== knn query: other features


def test_knn_aggregations_over_top_k(client, l2idx):
    body = {"query": _knn(k=3), "size": 0,
            "aggs": {"t": {"terms": {"field": "t"}}, "m": {"avg": {"field": "n"}}}}
    res = _ok(client, l2idx, body)
    assert res["hits"]["total"]["value"] == 3
    assert res["aggregations"]["t"]["buckets"] == [{"key": "odd", "doc_count": 2}, {"key": "even", "doc_count": 1}]
    assert res["aggregations"]["m"] == {"value": 2.0}


def test_knn_source_filtering_and_fields(client, l2idx):
    res = _ok(client, l2idx, {"query": _knn(k=2), "_source": {"excludes": ["v"]}, "fields": ["t"]})
    assert [(h["_id"], h["_source"], h["fields"]) for h in res["hits"]["hits"]] == [
        ("2", {"t": "even", "n": 2}, {"t": ["even"]}), ("1", {"t": "odd", "n": 1}, {"t": ["odd"]})]
    res = _ok(client, l2idx, {"query": _knn(k=1)})
    assert res["hits"]["hits"][0]["_source"] == {"v": [1.0, 0.0], "n": 2, "t": "even"}


def test_knn_with_highlight(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 2}, "d": {"type": "text"}},
               docs={"1": {"v": [0, 0], "d": "red fox"}, "2": {"v": [1, 0], "d": "blue fox"},
                     "3": {"v": [9, 9], "d": "red dog"}})
    q = {"bool": {"must": [_knn(k=2)], "should": [{"match": {"d": "red"}}]}}
    res = _ok(client, name, {"query": q, "_source": False, "highlight": {"fields": {"d": {}}}})
    assert [(h["_id"], h.get("highlight")) for h in res["hits"]["hits"]] == [
        ("1", {"d": ["<em>red</em> fox"]}), ("2", None)]


def test_knn_explain_faiss(client, l2idx):
    res = _ok(client, l2idx, {"query": _knn(k=1), "_source": False, "explain": True})
    assert res["hits"]["hits"][0]["_explanation"] == {
        "value": 0.5, "description": "the type of knn search executed was Approximate-NN",
        "details": [{"value": 0.5,
                     "description": "the type of knn search executed at leaf was Approximate-NN with "
                                    "vectorDataType = FLOAT, spaceType = l2 where score is computed as "
                                    "`1 / (1 + rawScore)` from:",
                     "details": [{"value": 1.0, "description": "rawScore, returned from FAISS library",
                                  "details": []}]}]}


def test_knn_explain_lucene(client, vec_index):
    name = vec_index({"method": {"name": "hnsw", "engine": "lucene"}})
    res = _ok(client, name, {"query": _knn(k=1), "_source": False, "explain": True})
    assert res["hits"]["hits"][0]["_explanation"] == {"value": 0.5, "description": "within top 1 docs",
                                                      "details": []}


def test_knn_multiple_vector_fields(client, mk):
    name = mk({"a": {"type": "knn_vector", "dimension": 2},
                               "b": {"type": "knn_vector", "dimension": 3, "space_type": "innerproduct"}},
               docs={"1": {"a": [0, 0], "b": [1, 1, 1]}, "2": {"a": [5, 5]}, "3": {"b": [2, 0, 0]}})
    assert _hits(client, name, {"query": _knn(field="a", k=5)}) == [("1", 0.33333334), ("2", 0.030303031)]
    assert _hits(client, name, {"query": _knn([1, 1, 1], field="b", k=5)}) == [("1", 4.0), ("3", 3.0)]
    q = {"bool": {"should": [_knn(field="a", k=5), _knn([1, 1, 1], field="b", k=5)]}}
    assert _hits(client, name, {"query": q}) == [("1", 4.3333335), ("3", 3.0), ("2", 0.030303031)]


@pytest.fixture
def nested_idx(mk):
    return mk({
        "p": {"type": "nested", "properties": {"v": {"type": "knn_vector", "dimension": 2}}},
        "t": {"type": "keyword"}}, docs={
        "1": {"t": "a", "p": [{"v": [0, 0]}, {"v": [5, 5]}]},
        "2": {"t": "b", "p": [{"v": [1, 0]}, {"v": [2, 2]}]},
        "3": {"t": "a", "p": [{"v": [9, 9]}]}})


def _inner(res):
    return [(h["_id"], h["_score"], [(x["_nested"]["offset"], x["_score"]) for x in h["inner_hits"]["p"]["hits"]["hits"]])
            for h in res["hits"]["hits"]]


def test_knn_nested_k_counts_parents(client, nested_idx):
    q = {"nested": {"path": "p", "query": _knn(field="p.v", k=2)}}
    assert _hits(client, nested_idx, {"query": q}) == [("2", 0.5), ("1", 0.33333334)]


def test_knn_nested_inner_hits_best_child_only(client, nested_idx):
    q = {"nested": {"path": "p", "query": _knn(field="p.v", k=3), "inner_hits": {"_source": False}}}
    res = _ok(client, nested_idx, {"query": q, "_source": False})
    assert _inner(res) == [("2", 0.5, [(0, 0.5)]), ("1", 0.33333334, [(0, 0.33333334)]),
                           ("3", 0.007751938, [(0, 0.007751938)])]


def test_knn_nested_expand_nested_docs(client, nested_idx):
    q = {"nested": {"path": "p", "query": _knn(field="p.v", k=3, expand_nested_docs=True),
                    "inner_hits": {"_source": False}}}
    res = _ok(client, nested_idx, {"query": q, "_source": False})
    # all children are scored and the parent gets the (default avg) nested score
    assert _inner(res) == [("2", 0.4166667, [(0, 0.5), (1, 0.33333334)]),
                           ("1", 0.18181819, [(0, 0.33333334), (1, 0.030303031)]),
                           ("3", 0.007751938, [(0, 0.007751938)])]


def test_knn_nested_filter_on_parent_field(client, nested_idx):
    q = {"nested": {"path": "p", "query": _knn(field="p.v", k=2, filter={"term": {"t": "a"}})}}
    assert _hits(client, nested_idx, {"query": q}) == [("1", 0.33333334), ("3", 0.007751938)]


def test_knn_nested_field_without_nested_query_matches_nothing(client, nested_idx):
    res = _ok(client, nested_idx, {"query": _knn(field="p.v", k=2)})
    assert res["hits"]["total"]["value"] == 0


def test_knn_nested_source_round_trip(client, nested_idx):
    assert client.get(f"/{nested_idx}/_doc/1").json()["_source"] == {
        "t": "a", "p": [{"v": [0.0, 0.0]}, {"v": [5.0, 5.0]}]}


# =========================================================================== knn query: errors


@pytest.mark.parametrize("params,reason", [
    ({"k": 2, "min_score": 0.3}, "[knn] requires exactly one of k, distance or score to be set"),
    ({"max_distance": 1, "min_score": 0.1}, "[knn] requires exactly one of k, distance or score to be set"),
    ({}, "[knn] requires exactly one of k, distance or score to be set"),
    ({"k": 0}, "[knn] requires k to be in the range (0, 10000]"),
    ({"k": -1}, "[knn] requires k to be in the range (0, 10000]"),
    ({"k": 10001}, "[knn] requires k to be in the range (0, 10000]"),
    ({"min_score": 0}, "[knn] requires minScore to be greater than 0"),
    ({"min_score": -1}, "[knn] requires minScore to be greater than 0"),
])
def test_knn_parameter_errors(client, l2idx, params, reason):
    r = _search(client, l2idx, {"query": _knn(**params)})
    assert r.status_code == 400
    e = _error(r)
    assert (e["type"], e["reason"]) == ("illegal_argument_exception", reason)
    assert e["root_cause"] == [{"type": "illegal_argument_exception", "reason": reason}]


def test_knn_missing_vector(client, l2idx):
    r = _search(client, l2idx, {"query": {"knn": {"v": {"k": 2}}}})
    assert r.status_code == 400
    assert (_error(r)["type"], _error(r)["reason"]) == ("illegal_argument_exception",
                                                        "[knn] requires query vector")


def test_knn_empty_body(client, l2idx):
    r = _search(client, l2idx, {"query": {"knn": {}}})
    assert r.status_code == 400
    assert _error(r)["reason"] == "[knn] field 'vector' requires to be non-null and non-empty"


@pytest.mark.parametrize("vector,cause", [
    ([], ("illegal_argument_exception", "[knn] field 'vector' requires to be non-null and non-empty")),
    ("abc", ("number_format_exception", 'For input string: "abc"')),
    (["a", 1], ("number_format_exception", 'For input string: "a"')),
])
def test_knn_bad_vector_values(client, l2idx, vector, cause):
    r = _search(client, l2idx, {"query": _knn(vector, k=2)})
    assert r.status_code == 400
    e = _error(r)
    assert e["type"] == "x_content_parse_exception"
    assert e["reason"].endswith("[knn] failed to parse field [vector]")
    assert _cause(r) == cause


def test_knn_unknown_parameter(client, l2idx):
    r = client.post(f"/{l2idx}/_search", content=b'{"query":{"knn":{"v":{"vector":[1,1],"k":2,"foo":2}}}}',
                    headers={"Content-Type": "application/json"})
    assert r.status_code == 400
    assert (_error(r)["type"], _error(r)["reason"]) == ("x_content_parse_exception",
                                                        "[1:44] [knn] unknown field [foo]")


def test_knn_unknown_method_parameter(client, l2idx):
    r = _search(client, l2idx, {"query": _knn(k=2, method_parameters={"foo": 10})})
    assert r.status_code == 400
    assert _error(r)["type"] == "x_content_parse_exception"
    assert _cause(r) == ("parsing_exception", "[knn] unknown method parameter found [foo]")


def test_knn_multiple_fields(client, l2idx):
    r = _search(client, l2idx, {"query": {"knn": {"v": {"vector": Q, "k": 1}, "x": {"vector": Q, "k": 1}}}})
    assert r.status_code == 400
    assert (_error(r)["type"], _error(r)["reason"]) == (
        "parsing_exception", "[knn] query doesn't support multiple fields, found [v] and [x]")


@pytest.mark.parametrize("query,reason", [
    (_knn([1, 1, 1], k=2), "Query vector has invalid dimension: 3. Dimension should be: 2"),
    (_knn(field="t", k=2), "Field 't' is not knn_vector type."),
    (_knn(field="nope", k=2), "Field 'nope' is not knn_vector type."),
    (_knn(max_distance=-1), "[knn] requires distance to be non-negative for space type: L2"),
    (_knn(k=2, expand_nested_docs=True),
     "Invalid value provided for the [expand_nested_docs] field. [expand_nested_docs] is only supported "
     "with a nested field."),
], ids=["dimension", "keyword_field", "missing_field", "negative_distance", "expand_not_nested"])
def test_knn_shard_errors(client, l2idx, query, reason):
    r = _search(client, l2idx, {"query": query})
    assert r.status_code == 400
    assert _shard_cause(r) == ("query_shard_exception", f"failed to create query: {reason}")
    assert _error(r)["failed_shards"][0]["reason"]["caused_by"] == {"type": "illegal_argument_exception",
                                                                    "reason": reason}


def test_knn_cosine_zero_query_vector(client, vec_index):
    name = vec_index({"space_type": "cosinesimil"})
    r = _search(client, name, {"query": _knn([0, 0], k=3)})
    assert r.status_code == 400
    assert _shard_cause(r) == ("query_shard_exception",
                               "failed to create query: Cannot normalize a zero-length vector")


def test_knn_query_requires_index_knn(client, vec_index):
    name = vec_index(knn=False)
    r = _search(client, name, {"query": _knn(k=2)})
    assert r.status_code == 400
    assert _shard_cause(r) == ("query_shard_exception",
                               "failed to create query: Field 'v' is not built for ANN search.")


# =========================================================================== knn_score script


def _knn_score(space, query_value=Q, field="v", query=None, **extra):
    return {"script_score": dict({"query": query or {"match_all": {}}, "script": {
        "lang": "knn", "source": "knn_score",
        "params": {"field": field, "query_value": query_value, "space_type": space}}}, **extra)}


@pytest.fixture
def exact_idx(vec_index, client):
    name = vec_index(knn=False)
    _bulk(client, name, {"6": {"n": 6, "t": "even"}})  # no vector: scores 0
    return name


@pytest.mark.parametrize("space,expected", [
    ("l2", L2_SCORES),
    ("l1", [("2", 0.5), ("1", 0.33333334), ("3", 0.33333334), ("5", 0.2), ("4", 0.16666667)]),
    ("linf", [("1", 0.5), ("2", 0.5), ("3", 0.5), ("5", 0.33333334), ("4", 0.25)]),
    # unlike the knn query the zero vector scores (1 + 0) / 2
    ("cosinesimil", [("4", 0.99497473), ("2", 0.8535534), ("3", 0.8535534), ("1", 0.5), ("5", 0.0)]),
    ("innerproduct", IP_SCORES),
])
def test_knn_score_space_types(client, exact_idx, space, expected):
    assert _hits(client, exact_idx, {"query": _knn_score(space)}) == expected + [("6", 0.0)]


def test_knn_score_on_knn_index(client, l2idx):
    assert _hits(client, l2idx, {"query": _knn_score("l1")})[:2] == [("2", 0.5), ("1", 0.33333334)]


def test_knn_score_with_query_filter_min_score_and_boost(client, exact_idx):
    assert _hits(client, exact_idx, {"query": _knn_score("l2", query={"range": {"n": {"gte": 3}}})}) == [
        ("3", 0.33333334), ("5", 0.11111111), ("4", 0.071428575), ("6", 0.0)]
    assert _hits(client, exact_idx, {"query": _knn_score("l2", min_score=0.3)}) == L2_SCORES[:3]
    assert _hits(client, exact_idx, {"query": _knn_score("l2", boost=2)})[:2] == [("2", 1.0), ("1", 0.6666667)]


def test_knn_score_byte_vectors(client, vec_index):
    name = vec_index({"data_type": "byte"})
    assert _hits(client, name, {"query": _knn_score("l1")}) == [
        ("2", 0.5), ("1", 0.33333334), ("3", 0.33333334), ("5", 0.2), ("4", 0.16666667)]


def test_knn_score_binary_hamming(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 8, "data_type": "binary",
                                     "space_type": "hamming"}},
               docs={"1": {"v": [0]}, "2": {"v": [1]}, "3": {"v": [3]}, "4": {"v": [-1]}, "5": {"v": [127]}})
    assert _hits(client, name, {"query": _knn_score("hamming", [7])}) == [
        ("3", 0.5), ("2", 0.33333334), ("1", 0.25), ("5", 0.2), ("4", 0.16666667)]
    r = _search(client, name, {"query": _knn_score("l2", [7])})
    assert r.status_code == 400
    reason = _shard_cause(r)[1]
    # the list of allowed data types comes from a set: its order is not stable
    prefix = "failed to create query: Incompatible field_type for l2 space. The data type should be ["
    assert reason.startswith(prefix) and reason.endswith("] but got BINARY"), reason
    assert sorted(reason[len(prefix):-len("] but got BINARY")].split(", ")) == ["BYTE", "FLOAT", "HALF_FLOAT"]


@pytest.mark.parametrize("query,reason", [
    (_knn_score("hamming"), "Incompatible field_type for hamming space. The data type should be [BINARY] but got FLOAT"),
    (_knn_score("foo"), "Invalid space type. Please refer to the available space types."),
    (_knn_score("l2", [1, 1, 1]), "Object's length=3 does not match the expected length=2."),
    (_knn_score("l2", field="n"), "Incompatible field_type for l2 space. The field type must be knn_vector."),
], ids=["hamming_on_float", "unknown_space", "dimension", "not_vector"])
def test_knn_score_errors(client, exact_idx, query, reason):
    r = _search(client, exact_idx, {"query": query})
    assert r.status_code == 400
    assert _shard_cause(r) == ("query_shard_exception", f"failed to create query: {reason}")


@pytest.mark.parametrize("missing", ["field", "query_value", "space_type"])
def test_knn_score_missing_params(client, exact_idx, missing):
    q = _knn_score("l2")
    del q["script_score"]["script"]["params"][missing]
    r = _search(client, exact_idx, {"query": q})
    assert r.status_code == 400
    assert _shard_cause(r) == ("query_shard_exception", f"failed to create query: Missing parameter [{missing}]")


def test_knn_score_unknown_script_name(client, exact_idx):
    q = _knn_score("l2")
    q["script_score"]["script"]["source"] = "other"
    r = _search(client, exact_idx, {"query": q})
    assert r.status_code == 400
    assert _shard_cause(r) == ("query_shard_exception",
                               "failed to create query: Failed to compile inline script [other] using lang [knn]")
    assert _cause(r) == ("illegal_argument_exception", "Unknown script name other")


# =========================================================================== painless vector functions

NON_ZERO = {"bool": {"must_not": {"ids": {"values": ["1"]}}, "filter": {"exists": {"field": "v"}}}}


def _painless(source, query=None, q=Q):
    return {"script_score": {"query": query or {"exists": {"field": "v"}},
                             "script": {"source": source, "params": {"q": q}}}}


@pytest.mark.parametrize("source,query,expected", [
    ("l2Squared(params.q, doc['v'])", None, [("4", 13.0), ("5", 8.0), ("1", 2.0), ("3", 2.0), ("2", 1.0)]),
    ("1/(1+l2Squared(params.q, doc['v']))", None, L2_SCORES),
    ("l1Norm(params.q, doc['v'])", None, [("4", 5.0), ("5", 4.0), ("1", 2.0), ("3", 2.0), ("2", 1.0)]),
    ("lInfNorm(params.q, doc['v'])", None, [("4", 3.0), ("5", 2.0), ("1", 1.0), ("2", 1.0), ("3", 1.0)]),
    ("1+cosineSimilarity(params.q, doc['v'])", NON_ZERO,
     [("4", 1.9899495), ("2", 1.7071068), ("3", 1.7071068), ("5", 0.0)]),
    ("innerProduct(params.q, doc['v'])+2", NON_ZERO, [("4", 9.0), ("3", 4.0), ("2", 3.0), ("5", 0.0)]),
    ("doc['v'].value[1]+1", NON_ZERO, [("4", 5.0), ("3", 3.0), ("2", 1.0), ("5", 0.0)]),
    ("doc['v'].size()", None, [("1", 1.0), ("2", 1.0), ("3", 1.0), ("4", 1.0), ("5", 1.0)]),
], ids=["l2Squared", "l2_score", "l1Norm", "lInfNorm", "cosineSimilarity", "innerProduct", "value", "size"])
def test_painless_vector_functions(client, exact_idx, source, query, expected):
    assert _hits(client, exact_idx, {"query": _painless(source, query)}) == expected


def test_painless_hamming_on_binary(client, mk):
    name = mk({"v": {"type": "knn_vector", "dimension": 8, "data_type": "binary",
                                     "space_type": "hamming"}},
               docs={"1": {"v": [0]}, "2": {"v": [1]}, "3": {"v": [3]}, "4": {"v": [-1]}, "5": {"v": [127]}})
    assert _hits(client, name, {"query": _painless("hamming(params.q, doc['v'])", {"match_all": {}}, [7])}) == [
        ("4", 5.0), ("5", 4.0), ("1", 3.0), ("2", 2.0), ("3", 1.0)]


def test_painless_cosine_of_zero_vector_is_nan(client, exact_idx):
    r = _search(client, exact_idx, {"query": _painless("cosineSimilarity(params.q, doc['v'])")})
    assert r.status_code == 400
    assert _shard_cause(r) == ("illegal_argument_exception",
                               "script_score script returned an invalid score [NaN] for doc [0]. "
                               "Must be a non-negative score!")


def test_painless_negative_score_rejected(client, exact_idx):
    r = _search(client, exact_idx, {"query": _painless("innerProduct(params.q, doc['v'])")})
    assert r.status_code == 400
    assert _shard_cause(r)[1] == ("script_score script returned an invalid score [-2.0] for doc [4]. "
                                  "Must be a non-negative score!")


@pytest.mark.parametrize("source,query,q", [
    ("l2Squared(params.q, doc['v'])", {"match_all": {}}, Q),       # doc 6 has no vector
    ("l2Squared(params.q, doc['v'])", NON_ZERO, [1, 1, 1]),         # dimension mismatch
    ("l2Squared(params.q, doc['n'])", NON_ZERO, Q),                 # not a vector field
], ids=["missing_value", "dimension", "not_vector"])
def test_painless_vector_runtime_errors(client, exact_idx, source, query, q):
    r = _search(client, exact_idx, {"query": _painless(source, query, q)})
    assert r.status_code == 400
    rc = _error(r)["root_cause"][0]
    assert (rc["type"], rc["reason"], rc["lang"], rc["script"]) == ("script_exception", "runtime error",
                                                                    "painless", source)


# =========================================================================== plugin APIs


def test_knn_stats_filtered(client):
    r = client.get("/_plugins/_knn/stats/hit_count,circuit_breaker_triggered")
    assert r.status_code == 200
    body = r.json()
    assert list(body) == ["_nodes", "cluster_name", "circuit_breaker_triggered", "nodes"]
    assert body["circuit_breaker_triggered"] is False
    for node in body["nodes"].values():
        assert list(node) == ["hit_count"]
        assert isinstance(node["hit_count"], int)


def test_knn_stats_unknown_stat(client):
    r = client.get("/_plugins/_knn/stats/foo")
    assert r.status_code == 400
    assert (_error(r)["type"], _error(r)["reason"]) == (
        "illegal_argument_exception", "request [/_plugins/_knn/stats/foo] contains unrecognized stat: [foo]")


def test_knn_warmup(client, l2idx):
    r = client.get(f"/_plugins/_knn/warmup/{l2idx}")
    assert r.status_code == 200
    assert r.json() == {"_shards": {"total": 1, "successful": 1, "failed": 0}}


def test_knn_warmup_rejects_non_knn_index(client, vec_index):
    name = vec_index(knn=False)
    r = client.get(f"/_plugins/_knn/warmup/{name}")
    assert r.status_code == 500
    assert (_error(r)["type"], _error(r)["reason"]) == (
        "k_n_n_invalid_indices_exception",
        "Warm up request rejected. One or more indices have 'index.knn' set to false.")


def test_knn_warmup_missing_index(client):
    r = client.get(f"/_plugins/_knn/warmup/{_name('missing')}")
    assert r.status_code == 404
    assert _error(r)["type"] == "index_not_found_exception"
