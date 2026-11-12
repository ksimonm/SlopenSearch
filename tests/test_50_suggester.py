"""Suggesters: term, phrase and completion, plus how ``suggest`` sits in a search request.

Indices whose scores are asserted use one shard, so the term statistics the suggesters
score with are exact and the answers are deterministic.
"""

import json
import uuid

import pytest

ONE_SHARD = {"number_of_shards": 1, "number_of_replicas": 0}

TEXT_MAPPING = {"properties": {"body": {"type": "text"}}}

TEXT_DOCS = [
    {"body": "mechanical keyboard"},
    {"body": "wireless keyboard"},
    {"body": "gaming mouse"},
]

COMPLETION_MAPPING = {"properties": {"suggest": {"type": "completion"}}}

COMPLETION_DOCS = [
    {"suggest": {"input": ["laptop", "notebook"], "weight": 10}},
    {"suggest": {"input": ["laptop stand"], "weight": 5}},
    {"suggest": {"input": ["mouse"], "weight": 3}},
]

# Eight short documents: "keyboard" is in 3 of them, its near misses in one each.
KB_DOCS = [
    {"body": "mechanical keyboard"},
    {"body": "wireless keyboard"},
    {"body": "gaming mouse"},
    {"body": "keyboard shortcuts"},
    {"body": "keyboards are great"},
    {"body": "keybard typo once"},
    {"body": "the quick brown fox"},
    {"body": "the quick brown dog"},
]

# Eleven one-word documents with chosen document frequencies:
# keyboard 1, keyboards 4, keybard 2, mouse 3, house 1.
FREQ_DOCS = (
    [{"body": "keyboard"}]
    + [{"body": "keyboards"}] * 4
    + [{"body": "keybard"}] * 2
    + [{"body": "mouse"}] * 3
    + [{"body": "house"}]
)

PHRASE_SETTINGS = {
    **ONE_SHARD,
    "analysis": {
        "analyzer": {
            "trigram": {"type": "custom", "tokenizer": "standard", "filter": ["lowercase", "shingle"]},
            "reverse": {"type": "custom", "tokenizer": "standard", "filter": ["lowercase", "reverse"]},
            "only_shingles": {"type": "custom", "tokenizer": "standard", "filter": ["lowercase", "bigrams"]},
        },
        "filter": {
            "shingle": {"type": "shingle", "min_shingle_size": 2, "max_shingle_size": 3},
            "bigrams": {"type": "shingle", "output_unigrams": False},
        },
    },
}

PHRASE_MAPPING = {
    "properties": {
        "title": {
            "type": "text",
            "fields": {
                "trigram": {"type": "text", "analyzer": "trigram"},
                "reverse": {"type": "text", "analyzer": "reverse"},
                "shingles": {"type": "text", "analyzer": "only_shingles"},
            },
        },
        "n": {"type": "integer"},
    }
}

PHRASE_DOCS = [
    {"title": "noble warriors"},
    {"title": "nobel prize"},
    {"title": "the quick brown fox"},
    {"title": "the quick brown fox jumps"},
    {"title": "lazy dog sleeps"},
    {"title": "quick brown dog"},
]

TYPO = "the qick brwn fox"
TYPO_OPTIONS = [
    {"text": "the quick brown fox", "score": 0.16425854},
    {"text": "the quick brwn fox", "score": 0.0042022066},
    {"text": "the qick brown fox", "score": 0.002681878},
]

PRODUCT_DOCS = {
    "a": {"suggest": {"input": ["laptop", "notebook"], "weight": 10}, "name": "a", "n": 1},
    "b": {"suggest": {"input": ["laptop stand"], "weight": 5}, "name": "b", "n": 2},
    "c": {"suggest": {"input": ["lamp"], "weight": 3}, "name": "c", "n": 3},
    "d": {"suggest": "lantern", "name": "d", "n": 4},
    "e": {"suggest": ["ladder", "lasso"], "name": "e", "n": 5},
    "f": {"suggest": {"input": ["Läufer"], "weight": 7}, "name": "f", "n": 6},
}

DUP_DOCS = {
    "1": {"suggest": {"input": ["laptop"], "weight": 10}},
    "2": {"suggest": {"input": ["laptop"], "weight": 5}},
    "3": {"suggest": {"input": ["lamp"], "weight": 3}},
}

CONTEXT_MAPPING = {
    "properties": {
        "suggest": {
            "type": "completion",
            "contexts": [
                {"name": "cat", "type": "category"},
                {"name": "loc", "type": "geo", "precision": 4},
            ],
        },
        "song": {
            "type": "completion",
            "contexts": [{"name": "genre", "type": "category", "path": "genre"}],
        },
        "genre": {"type": "keyword"},
    }
}

CONTEXT_DOCS = {
    "timmy": {"suggest": {"input": ["timmy's"], "weight": 5,
                          "contexts": {"cat": ["cafe", "food"], "loc": {"lat": 43.662, "lon": -79.380}}}},
    "monkey": {"suggest": {"input": ["monkey"], "weight": 3,
                           "contexts": {"cat": ["food"], "loc": {"lat": 43.6624803, "lon": -79.3863353}}}},
    "mcd": {"suggest": {"input": ["mcdonalds"], "weight": 8,
                        "contexts": {"cat": ["food"], "loc": [{"lat": 52.52, "lon": 13.40}]}}},
    "rock": {"song": {"input": ["rock anthem"], "weight": 2}, "genre": "rock"},
    "rap": {"song": {"input": ["rap anthem"], "weight": 4}, "genre": ["rap", "pop"]},
}

TORONTO = {"lat": 43.662, "lon": -79.380}
BERLIN = {"lat": 52.52, "lon": 13.40}

PARAMS_SETTINGS = {
    **ONE_SHARD,
    "analysis": {
        "analyzer": {
            "stopper": {"type": "custom", "tokenizer": "standard", "filter": ["lowercase", "stop"]},
        }
    },
}

PARAMS_MAPPING = {
    "properties": {
        "plain": {"type": "completion"},
        "nosep": {"type": "completion", "preserve_separators": False},
        "stop": {"type": "completion", "analyzer": "stopper"},
        "stop_nopos": {"type": "completion", "analyzer": "stopper", "preserve_position_increments": False},
        "short": {"type": "completion", "max_input_length": 5},
        "kw": {"type": "completion", "analyzer": "keyword"},
        "split": {"type": "completion", "analyzer": "standard", "search_analyzer": "keyword"},
    }
}

PARAMS_INPUTS = ["The Beatles", "foo bar", "Hello World"]


def _index_with_ids(client, index, docs):
    lines = []
    for doc_id, doc in docs.items():
        lines.append(json.dumps({"index": {"_id": doc_id}}))
        lines.append(json.dumps(doc, ensure_ascii=False))
    r = client.post(
        f"/{index}/_bulk",
        content=("\n".join(lines) + "\n").encode(),
        headers={"Content-Type": "application/x-ndjson"},
        params={"refresh": "true"},
    )
    assert r.status_code == 200, r.text
    assert not r.json()["errors"], r.text


@pytest.fixture
def text_index(client, index_factory, bulk_index):
    name = index_factory(mappings=TEXT_MAPPING)
    bulk_index(name, TEXT_DOCS)
    return name


@pytest.fixture
def completion_index(client, index_factory, bulk_index):
    name = index_factory(mappings=COMPLETION_MAPPING)
    bulk_index(name, COMPLETION_DOCS)
    return name


@pytest.fixture
def kb_index(index_factory, bulk_index):
    name = index_factory(mappings=TEXT_MAPPING, settings=ONE_SHARD)
    bulk_index(name, KB_DOCS)
    return name


@pytest.fixture
def freq_index(index_factory, bulk_index):
    name = index_factory(mappings=TEXT_MAPPING, settings=ONE_SHARD)
    bulk_index(name, FREQ_DOCS)
    return name


@pytest.fixture
def phrase_index(index_factory, bulk_index):
    name = index_factory(mappings=PHRASE_MAPPING, settings=PHRASE_SETTINGS)
    bulk_index(name, PHRASE_DOCS)
    return name


@pytest.fixture
def product_index(client, index_factory):
    name = index_factory(
        mappings={"properties": {"suggest": {"type": "completion"}, "name": {"type": "keyword"},
                                 "n": {"type": "integer"}, "body": {"type": "text"}}},
        settings=ONE_SHARD,
    )
    _index_with_ids(client, name, PRODUCT_DOCS)
    return name


@pytest.fixture
def dup_index(client, index_factory):
    name = index_factory(mappings=COMPLETION_MAPPING, settings=ONE_SHARD)
    _index_with_ids(client, name, DUP_DOCS)
    return name


@pytest.fixture
def context_index(client, index_factory):
    name = index_factory(mappings=CONTEXT_MAPPING, settings=ONE_SHARD)
    _index_with_ids(client, name, CONTEXT_DOCS)
    return name


@pytest.fixture
def params_index(client, index_factory):
    name = index_factory(mappings=PARAMS_MAPPING, settings=PARAMS_SETTINGS)
    _index_with_ids(client, name, {"1": {f: {"input": PARAMS_INPUTS} for f in PARAMS_MAPPING["properties"]}})
    return name


def _suggest(client, index, suggest, **params):
    r = client.post(f"/{index}/_search", json={"suggest": suggest}, params=params or None)
    assert r.status_code == 200, r.text
    return r.json()["suggest"]


def _error(client, index, body, status=400):
    r = client.post(f"/{index}/_search", json=body)
    assert r.status_code == status, r.text
    return r.json()["error"]


def _term(client, index, text, **opts):
    res = _suggest(client, index, {"s": {"text": text, "term": {"field": "body", **opts}}})
    return res["s"]


def _term_options(client, index, text, **opts):
    entries = _term(client, index, text, **opts)
    assert len(entries) == 1, entries
    return entries[0]["options"]


def _phrase(client, index, phrase, text=TYPO):
    res = _suggest(client, index, {"s": {"text": text, "phrase": {"field": "title.trigram", **phrase}}})
    assert len(res["s"]) == 1
    return res["s"][0]


def _completion(client, index, completion, prefix="la", source=False, key="prefix"):
    body = {"suggest": {"s": {key: prefix, "completion": {"field": "suggest", **completion}}}}
    if source is not None:
        body["_source"] = source
    r = client.post(f"/{index}/_search", json=body)
    assert r.status_code == 200, r.text
    entries = r.json()["suggest"]["s"]
    assert len(entries) == 1
    return entries[0]


def _ids(entry):
    return [(o["_id"], o["_score"]) for o in entry["options"]]


def _texts(entry):
    return [o["text"] for o in entry["options"]]


def _assert_shard_failure(error, reason):
    assert error["type"] == "search_phase_execution_exception"
    assert error["reason"] == "all shards failed"
    assert error["root_cause"][0] == {"type": "illegal_argument_exception", "reason": reason}
    assert error["caused_by"]["type"] == "illegal_argument_exception"
    assert error["caused_by"]["reason"] == reason


# --------------------------------------------------------------------------- basics


@pytest.mark.smoke
def test_term_suggester_fixes_typo(client, text_index):
    res = _suggest(
        client,
        text_index,
        {"s": {"text": "keybord", "term": {"field": "body"}}},
    )
    texts = [o["text"] for o in res["s"][0]["options"]]
    assert "keyboard" in texts


def test_phrase_suggester(client, text_index):
    res = _suggest(
        client,
        text_index,
        {"s": {"text": "mechanical keybord", "phrase": {"field": "body"}}},
    )
    options = res["s"][0]["options"]
    assert isinstance(options, list)
    if options:
        assert any("keyboard" in o["text"] for o in options)


def test_completion_suggester(client, completion_index):
    res = _suggest(
        client,
        completion_index,
        {"s": {"prefix": "lap", "completion": {"field": "suggest"}}},
    )
    texts = [o["text"] for o in res["s"][0]["options"]]
    assert "laptop" in texts


def test_completion_suggester_ranking(client, completion_index):
    res = _suggest(
        client,
        completion_index,
        {"s": {"prefix": "lap", "completion": {"field": "suggest"}}},
    )
    scores = [o["_score"] for o in res["s"][0]["options"]]
    assert scores == sorted(scores, reverse=True)


# --------------------------------------------------------------------------- term suggester


def test_term_entry_and_option_shape(client, kb_index):
    entries = _term(client, kb_index, "keybord")
    assert entries == [{
        "text": "keybord", "offset": 0, "length": 7,
        "options": [
            {"text": "keyboard", "score": 0.85714287, "freq": 3},
            {"text": "keybard", "score": 0.85714287, "freq": 1},
            {"text": "keyboards", "score": 0.71428573, "freq": 1},
        ],
    }]
    assert list(entries[0]) == ["text", "offset", "length", "options"]
    assert list(entries[0]["options"][0]) == ["text", "score", "freq"]


def test_term_one_entry_per_token_with_offsets(client, kb_index):
    entries = _term(client, kb_index, "keybord mouze")
    assert [(e["text"], e["offset"], e["length"]) for e in entries] == [
        ("keybord", 0, 7), ("mouze", 8, 5)]
    assert entries[1]["options"] == [{"text": "mouse", "score": 0.8, "freq": 1}]


def test_term_offsets_skip_punctuation_and_whitespace(client, freq_index):
    entries = _term(client, freq_index, "  the, mous!! keybord ")
    assert [(e["text"], e["offset"], e["length"]) for e in entries] == [
        ("the", 2, 3), ("mous", 7, 4), ("keybord", 14, 7)]
    assert entries[0]["options"] == []
    assert entries[1]["options"] == [{"text": "mouse", "score": 0.75, "freq": 3}]


def test_term_text_is_analyzed_with_field_analyzer(client, freq_index):
    entries = _term(client, freq_index, "MOUS")
    assert entries == [{"text": "mous", "offset": 0, "length": 4,
                        "options": [{"text": "mouse", "score": 0.75, "freq": 3}]}]


def test_term_analyzer_option_overrides_field_analyzer(client, kb_index):
    entries = _term(client, kb_index, "Keybord-Mouze", analyzer="whitespace")
    assert entries == [{"text": "Keybord-Mouze", "offset": 0, "length": 13, "options": []}]


def test_term_standard_analyzer_splits_hyphen(client, kb_index):
    entries = _term(client, kb_index, "Keybord-Mouze")
    assert [(e["text"], e["offset"], e["length"]) for e in entries] == [
        ("keybord", 0, 7), ("mouze", 8, 5)]


def test_term_unknown_analyzer(client, freq_index):
    err = _error(client, freq_index, {"suggest": {"s": {"text": "mous", "term": {"field": "body", "analyzer": "nope"}}}})
    _assert_shard_failure(err, "analyzer [nope] doesn't exists")


def test_term_missing_mode_skips_known_terms(client, freq_index):
    entries = _term(client, freq_index, "keyboard keybord", suggest_mode="missing")
    assert entries[0] == {"text": "keyboard", "offset": 0, "length": 8, "options": []}
    assert _texts(entries[1]) == ["keybard", "keyboard", "keyboards"]


def test_term_default_mode_is_missing(client, freq_index):
    assert _term(client, freq_index, "keyboard") == _term(client, freq_index, "keyboard", suggest_mode="missing")


def test_term_popular_mode_suggests_more_frequent_terms(client, freq_index):
    entries = _term(client, freq_index, "keyboard keybord", suggest_mode="popular")
    assert entries[0]["options"] == [
        {"text": "keyboards", "score": 0.875, "freq": 4},
        {"text": "keybard", "score": 0.85714287, "freq": 2},
    ]
    assert entries[1]["options"] == [
        {"text": "keybard", "score": 0.85714287, "freq": 2},
        {"text": "keyboard", "score": 0.85714287, "freq": 1},
        {"text": "keyboards", "score": 0.71428573, "freq": 4},
    ]


def test_term_popular_mode_drops_less_frequent_terms(client, freq_index):
    opts = _term_options(client, freq_index, "mouse", suggest_mode="popular", max_term_freq=10, prefix_length=0)
    assert opts == []


def test_term_always_mode_suggests_less_frequent_terms(client, freq_index):
    opts = _term_options(client, freq_index, "mouse", suggest_mode="always", max_term_freq=10, prefix_length=0)
    assert opts == [{"text": "house", "score": 0.8, "freq": 1}]


def test_term_always_mode_on_existing_term(client, freq_index):
    opts = _term_options(client, freq_index, "keyboard", suggest_mode="always")
    assert opts == [
        {"text": "keyboards", "score": 0.875, "freq": 4},
        {"text": "keybard", "score": 0.85714287, "freq": 2},
    ]


def test_term_max_term_freq_default_skips_frequent_input(client, kb_index):
    # "keyboard" is in 3 of 8 docs, above the default max_term_freq of 1%.
    assert _term_options(client, kb_index, "keyboard", suggest_mode="always") == []


def test_term_max_term_freq_absolute(client, kb_index):
    opts = _term_options(client, kb_index, "keyboard", suggest_mode="always", max_term_freq=5)
    assert _texts({"options": opts}) == ["keyboards", "keybard"]


def test_term_max_term_freq_fraction(client, kb_index):
    opts = _term_options(client, kb_index, "keyboard", suggest_mode="always", max_term_freq=0.5)
    assert _texts({"options": opts}) == ["keyboards", "keybard"]


def test_term_max_term_freq_absolute_below_freq(client, kb_index):
    assert _term_options(client, kb_index, "keyboard", suggest_mode="always", max_term_freq=2) == []


def test_term_sort_score_breaks_ties_by_frequency(client, freq_index):
    opts = _term_options(client, freq_index, "keybord", sort="score")
    assert [(o["text"], o["freq"]) for o in opts] == [("keybard", 2), ("keyboard", 1), ("keyboards", 4)]


def test_term_sort_frequency(client, freq_index):
    opts = _term_options(client, freq_index, "keybord", sort="frequency")
    assert opts == [
        {"text": "keyboards", "score": 0.71428573, "freq": 4},
        {"text": "keybard", "score": 0.85714287, "freq": 2},
        {"text": "keyboard", "score": 0.85714287, "freq": 1},
    ]


def test_term_sort_unknown(client, freq_index):
    err = _error(client, freq_index, {"suggest": {"s": {"text": "x", "term": {"field": "body", "sort": "nope"}}}})
    assert err["type"] == "illegal_argument_exception"
    assert err["reason"] == "No enum constant org.opensearch.search.suggest.SortBy.NOPE"


def test_term_suggest_mode_unknown(client, freq_index):
    err = _error(client, freq_index, {"suggest": {"s": {"text": "x", "term": {"field": "body", "suggest_mode": "nope"}}}})
    assert err["type"] == "illegal_argument_exception"
    assert err["reason"] == (
        "No enum constant org.opensearch.search.suggest.term.TermSuggestionBuilder.SuggestMode.NOPE")


@pytest.mark.parametrize("distance,expected", [
    ("internal", [("keyboard", 0.85714287), ("keybard", 0.85714287), ("keyboards", 0.71428573)]),
    ("damerau_levenshtein", [("keyboard", 0.85714287), ("keybard", 0.85714287), ("keyboards", 0.71428573)]),
    ("levenshtein", [("keyboard", 0.875), ("keybard", 0.85714287), ("keyboards", 0.7777778)]),
    ("jaro_winkler", [("keyboard", 0.9791666), ("keyboards", 0.962963), ("keybard", 0.9428572)]),
    ("ngram", [("keybard", 0.85714287), ("keyboard", 0.8125), ("keyboards", 0.7222222)]),
])
def test_term_string_distance(client, kb_index, distance, expected):
    opts = _term_options(client, kb_index, "keybord", string_distance=distance)
    assert [(o["text"], o["score"]) for o in opts] == expected


def test_term_string_distance_unknown(client, kb_index):
    err = _error(client, kb_index, {"suggest": {"s": {"text": "x", "term": {"field": "body", "string_distance": "bogus"}}}})
    assert err == {
        "root_cause": [{"type": "illegal_argument_exception", "reason": "Illegal distance option bogus"}],
        "type": "illegal_argument_exception",
        "reason": "Illegal distance option bogus",
    }


def test_term_max_edits_one(client, kb_index):
    assert _term_options(client, kb_index, "keybrd", max_edits=1) == [
        {"text": "keybard", "score": 0.8333333, "freq": 1}]


def test_term_max_edits_default_two(client, kb_index):
    assert _term_options(client, kb_index, "keybrd") == [
        {"text": "keybard", "score": 0.8333333, "freq": 1},
        {"text": "keyboard", "score": 0.6666666, "freq": 3},
    ]


def test_term_max_edits_out_of_range(client, kb_index):
    err = _error(client, kb_index, {"suggest": {"s": {"text": "x", "term": {"field": "body", "max_edits": 3}}}})
    assert err["type"] == "illegal_argument_exception"
    assert err["reason"] == "maxEdits must be between 1 and 2"


def test_term_prefix_length_default_requires_first_char(client, kb_index):
    assert _term_options(client, kb_index, "jeyboard") == []


def test_term_prefix_length_zero(client, kb_index):
    assert _term_options(client, kb_index, "jeyboard", prefix_length=0) == [
        {"text": "keyboard", "score": 0.875, "freq": 3}]


def test_term_min_word_length(client, kb_index):
    assert _term_options(client, kb_index, "mous") == [{"text": "mouse", "score": 0.75, "freq": 1}]
    assert _term_options(client, kb_index, "mous", min_word_length=5) == []


def test_term_min_doc_freq_absolute(client, kb_index):
    assert _term_options(client, kb_index, "keybord", min_doc_freq=2) == [
        {"text": "keyboard", "score": 0.85714287, "freq": 3}]


def test_term_min_doc_freq_fraction(client, freq_index):
    # 0.3 of 11 docs: only "keyboards" (4 docs) is frequent enough.
    assert _term_options(client, freq_index, "keybord", min_doc_freq=0.3) == [
        {"text": "keyboards", "score": 0.71428573, "freq": 4}]


def test_term_size(client, kb_index):
    assert _term_options(client, kb_index, "keybord", size=1) == [
        {"text": "keyboard", "score": 0.85714287, "freq": 3}]


def test_term_size_zero_rejected(client, freq_index):
    err = _error(client, freq_index, {"suggest": {"s": {"text": "x", "term": {"field": "body", "size": 0}}}})
    assert err["type"] == "illegal_argument_exception"
    assert err["reason"] == "size must be positive"


def test_term_shard_size_limits_candidates(client, freq_index):
    assert _term_options(client, freq_index, "keybord", shard_size=1) == [
        {"text": "keybard", "score": 0.85714287, "freq": 2}]


def test_term_global_text_shared_by_suggestions(client, kb_index):
    res = _suggest(client, kb_index, {
        "text": "keybord",
        "a": {"term": {"field": "body"}},
        "b": {"term": {"field": "body", "size": 1}},
    })
    assert _texts(res["a"][0]) == ["keyboard", "keybard", "keyboards"]
    assert res["b"] == [{"text": "keybord", "offset": 0, "length": 7,
                         "options": [{"text": "keyboard", "score": 0.85714287, "freq": 3}]}]


def test_term_own_text_overrides_global_text(client, freq_index):
    res = _suggest(client, freq_index, {"text": "keybord", "s": {"text": "mous", "term": {"field": "body"}}})
    assert res["s"] == [{"text": "mous", "offset": 0, "length": 4,
                         "options": [{"text": "mouse", "score": 0.75, "freq": 3}]}]


def test_term_numeric_text_is_accepted(client, freq_index):
    res = _suggest(client, freq_index, {"s": {"text": 5, "term": {"field": "body"}}})
    assert res["s"] == [{"text": "5", "offset": 0, "length": 1, "options": []}]


def test_term_empty_text_gives_no_entries(client, freq_index):
    assert _suggest(client, freq_index, {"s": {"text": "", "term": {"field": "body"}}}) == {"s": []}


def test_term_missing_text(client, freq_index):
    err = _error(client, freq_index, {"suggest": {"s": {"term": {"field": "body"}}}})
    _assert_shard_failure(err, "The required text option is missing")


def test_term_missing_field_option(client, freq_index):
    err = _error(client, freq_index, {"suggest": {"s": {"text": "x", "term": {}}}})
    assert err == {
        "root_cause": [{"type": "parse_exception", "reason": "the required field option [field] is missing"}],
        "type": "parse_exception",
        "reason": "the required field option [field] is missing",
    }


def test_term_unmapped_field(client, freq_index):
    err = _error(client, freq_index, {"suggest": {"s": {"text": "x", "term": {"field": "nope"}}}})
    _assert_shard_failure(err, "no mapping found for field [nope]")
    assert err["failed_shards"][0]["index"] == freq_index
    assert err["failed_shards"][0]["reason"] == {
        "type": "illegal_argument_exception", "reason": "no mapping found for field [nope]"}


def test_term_unknown_option(client, freq_index):
    err = _error(client, freq_index, {"suggest": {"s": {"text": "x", "term": {"field": "body", "bogus": 1}}}})
    assert err["type"] == "parsing_exception"
    assert err["reason"] == "suggester[term] doesn't support field [bogus]"


def test_term_on_empty_index(client, index_factory):
    name = index_factory(mappings=TEXT_MAPPING, settings=ONE_SHARD)
    assert _suggest(client, name, {"s": {"text": "appel", "term": {"field": "body"}}}) == {
        "s": [{"text": "appel", "offset": 0, "length": 5, "options": []}]}


def test_term_merges_frequencies_across_indices(client, index_factory, bulk_index):
    a = index_factory(mappings=TEXT_MAPPING, settings=ONE_SHARD)
    b = index_factory(mappings=TEXT_MAPPING, settings=ONE_SHARD)
    bulk_index(a, [{"body": "apple banana"}, {"body": "cherry"}])
    bulk_index(b, [{"body": "apple grape"}])
    r = client.post(f"/{a},{b}/_search", json={
        "size": 0, "suggest": {"t": {"text": "appel grap", "term": {"field": "body"}}}})
    assert r.status_code == 200, r.text
    assert r.json()["suggest"]["t"] == [
        {"text": "appel", "offset": 0, "length": 5, "options": [{"text": "apple", "score": 0.8, "freq": 2}]},
        {"text": "grap", "offset": 6, "length": 4, "options": [{"text": "grape", "score": 0.75, "freq": 1}]},
    ]


# --------------------------------------------------------------------------- phrase suggester


def test_phrase_corrects_several_words(client, phrase_index):
    entry = _phrase(client, phrase_index, {})
    assert entry == {"text": TYPO, "offset": 0, "length": 17, "options": TYPO_OPTIONS}
    assert list(entry["options"][0]) == ["text", "score"]


def test_phrase_size(client, phrase_index):
    assert _phrase(client, phrase_index, {"size": 1})["options"] == TYPO_OPTIONS[:1]


def test_phrase_highlight(client, phrase_index):
    opts = _phrase(client, phrase_index, {"highlight": {"pre_tag": "<em>", "post_tag": "</em>"}})["options"]
    assert opts == [
        {"text": "the quick brown fox", "highlighted": "the <em>quick brown</em> fox", "score": 0.16425854},
        {"text": "the quick brwn fox", "highlighted": "the <em>quick</em> brwn fox", "score": 0.0042022066},
        {"text": "the qick brown fox", "highlighted": "the qick <em>brown</em> fox", "score": 0.002681878},
    ]


def test_phrase_highlight_needs_both_tags(client, phrase_index):
    err = _error(client, phrase_index, {"suggest": {"s": {"text": TYPO, "phrase": {
        "field": "title.trigram", "highlight": {"pre_tag": "<b>"}}}}})
    assert err["type"] == "illegal_argument_exception"
    assert err["reason"] == "Pre and post tag must both be null or both not be null."


def test_phrase_confidence_zero_includes_input(client, phrase_index):
    opts = _phrase(client, phrase_index, {"confidence": 0})["options"]
    assert opts == TYPO_OPTIONS + [{"text": TYPO, "score": 0.00051038945}]


def test_phrase_high_confidence_keeps_best_only(client, phrase_index):
    assert _phrase(client, phrase_index, {"confidence": 10})["options"] == TYPO_OPTIONS[:1]


def test_phrase_correct_input_has_no_options(client, phrase_index):
    entry = _phrase(client, phrase_index, {}, text="the quick brown fox")
    assert entry == {"text": "the quick brown fox", "offset": 0, "length": 19, "options": []}


def test_phrase_correct_input_with_zero_confidence(client, phrase_index):
    opts = _phrase(client, phrase_index, {"confidence": 0}, text="the quick brown fox")["options"]
    assert opts == [{"text": "the quick brown fox", "score": 0.20169695}]


def test_phrase_negative_confidence(client, phrase_index):
    err = _error(client, phrase_index, {"suggest": {"s": {"text": TYPO, "phrase": {
        "field": "title.trigram", "confidence": -1}}}})
    assert err["type"] == "illegal_argument_exception"
    assert err["reason"] == "confidence must be >= 0.0"


@pytest.mark.parametrize("max_errors", [2, 0.5])
def test_phrase_max_errors(client, phrase_index, max_errors):
    assert _phrase(client, phrase_index, {"max_errors": max_errors})["options"] == TYPO_OPTIONS


def test_phrase_max_errors_negative(client, phrase_index):
    err = _error(client, phrase_index, {"suggest": {"s": {"text": TYPO, "phrase": {
        "field": "title.trigram", "max_errors": -1}}}})
    assert err["type"] == "illegal_argument_exception"
    assert err["reason"] == "max_error must be > 0.0"


def test_phrase_gram_size_zero(client, phrase_index):
    err = _error(client, phrase_index, {"suggest": {"s": {"text": TYPO, "phrase": {
        "field": "title.trigram", "gram_size": 0}}}})
    assert err["type"] == "illegal_argument_exception"
    assert err["reason"] == "gramSize must be >= 1"


def test_phrase_real_word_error_likelihood_must_be_positive(client, phrase_index):
    err = _error(client, phrase_index, {"suggest": {"s": {"text": TYPO, "phrase": {
        "field": "title.trigram", "real_word_error_likelihood": -1}}}})
    assert err["type"] == "illegal_argument_exception"
    assert err["reason"] == "real_word_error_likelihood must be > 0.0"


def test_phrase_real_words_are_kept_by_default(client, phrase_index):
    # "noble" is a real word in the index, so the default generator (missing mode) leaves it.
    assert _phrase(client, phrase_index, {}, text="noble prize")["options"] == []


def test_phrase_direct_generator_always_mode(client, phrase_index):
    entry = _phrase(client, phrase_index, {
        "direct_generator": [{"field": "title.trigram", "suggest_mode": "always"}]}, text="noble prize")
    assert entry["options"] == [{"text": "nobel prize", "score": 0.19572009}]


def test_phrase_direct_generator_min_word_length(client, phrase_index):
    entry = _phrase(client, phrase_index, {
        "direct_generator": [{"field": "title.trigram", "min_word_length": 5}]})
    assert entry["options"] == []


def test_phrase_direct_generator_size(client, phrase_index):
    entry = _phrase(client, phrase_index, {"direct_generator": [{"field": "title.trigram", "size": 1}]})
    assert entry["options"] == TYPO_OPTIONS


def test_phrase_direct_generator_pre_and_post_filter(client, phrase_index):
    # A first-letter typo is only reachable through the reversed field.
    plain = _phrase(client, phrase_index, {
        "direct_generator": [{"field": "title.trigram", "suggest_mode": "always"}]}, text="obrwn fox")
    assert plain["options"] == []
    reverse = _phrase(client, phrase_index, {"direct_generator": [
        {"field": "title.trigram", "suggest_mode": "always"},
        {"field": "title.reverse", "suggest_mode": "always", "pre_filter": "reverse", "post_filter": "reverse"},
    ]}, text="obrwn fox")
    assert reverse["options"] == [{"text": "brown fox", "score": 0.23895085}]


def test_phrase_direct_generator_requires_field(client, phrase_index):
    err = _error(client, phrase_index, {"suggest": {"s": {"text": TYPO, "phrase": {
        "field": "title.trigram", "direct_generator": [{"suggest_mode": "always"}]}}}})
    assert err["type"] == "illegal_argument_exception"
    assert err["reason"] == "Required [field]"


def test_phrase_direct_generator_unknown_option(client, phrase_index):
    err = _error(client, phrase_index, {"suggest": {"s": {"text": TYPO, "phrase": {
        "field": "title.trigram", "direct_generator": [{"field": "title.trigram", "bogus": 1}]}}}})
    assert err["type"] == "x_content_parse_exception"
    assert err["reason"].endswith("[direct_generator] unknown field [bogus]")


def test_phrase_smoothing_laplace(client, phrase_index):
    opts = _phrase(client, phrase_index, {"smoothing": {"laplace": {"alpha": 0.7}}})["options"]
    assert opts == [
        {"text": "the quick brown fox", "score": 0.01407423},
        {"text": "the quick brwn fox", "score": 0.005397958},
    ]


def test_phrase_smoothing_stupid_backoff(client, phrase_index):
    opts = _phrase(client, phrase_index, {"smoothing": {"stupid_backoff": {"discount": 0.5}}})["options"]
    assert opts == [
        {"text": "the quick brown fox", "score": 0.16425854},
        {"text": "the quick brwn fox", "score": 0.0051009655},
        {"text": "the qick brown fox", "score": 0.0035867528},
    ]


def test_phrase_smoothing_linear(client, phrase_index):
    opts = _phrase(client, phrase_index, {"smoothing": {"linear": {
        "trigram_lambda": 0.5, "bigram_lambda": 0.3, "unigram_lambda": 0.2}}})["options"]
    assert opts == [
        {"text": "the quick brown fox", "score": 0.028985709},
        {"text": "the quick brwn fox", "score": 0.0012655164},
        {"text": "the qick brown fox", "score": 0.0012402945},
    ]


def test_phrase_smoothing_linear_lambdas_must_sum_to_one(client, phrase_index):
    err = _error(client, phrase_index, {"suggest": {"s": {"text": TYPO, "phrase": {
        "field": "title.trigram", "smoothing": {"linear": {
            "trigram_lambda": 0.5, "bigram_lambda": 0.3, "unigram_lambda": 0.3}}}}}})
    assert err["type"] == "illegal_argument_exception"
    assert err["reason"] == "linear smoothing lambdas must sum to 1"


@pytest.mark.parametrize("model", ["linear_interpolation", "foo"])
def test_phrase_smoothing_unknown_model(client, phrase_index, model):
    err = _error(client, phrase_index, {"suggest": {"s": {"text": TYPO, "phrase": {
        "field": "title.trigram", "smoothing": {model: {}}}}}})
    assert err["type"] == "illegal_argument_exception"
    assert err["reason"] == f"suggester[phrase] doesn't support object field [{model}]"


def test_phrase_collate_filters_options(client, phrase_index):
    opts = _phrase(client, phrase_index, {
        "collate": {"query": {"source": {"match_phrase": {"title": "{{suggestion}}"}}}}})["options"]
    assert opts == TYPO_OPTIONS[:1]


def test_phrase_collate_prune_marks_matches(client, phrase_index):
    opts = _phrase(client, phrase_index, {
        "size": 5,
        "collate": {"query": {"source": {"match_phrase": {"title": "{{suggestion}}"}}}, "prune": True}})["options"]
    assert opts == [
        {**TYPO_OPTIONS[0], "collate_match": True},
        {**TYPO_OPTIONS[1], "collate_match": False},
        {**TYPO_OPTIONS[2], "collate_match": False},
    ]
    assert list(opts[0]) == ["text", "score", "collate_match"]


def test_phrase_collate_params(client, phrase_index):
    opts = _phrase(client, phrase_index, {"collate": {
        "query": {"source": {"match_phrase": {"{{f}}": "{{suggestion}}"}}},
        "params": {"f": "title"}, "prune": True}})["options"]
    assert [o["collate_match"] for o in opts] == [True, False, False]


def test_phrase_collate_without_query_is_ignored(client, phrase_index):
    assert _phrase(client, phrase_index, {"collate": {"prune": True}})["options"] == TYPO_OPTIONS


def test_phrase_token_limit(client, phrase_index):
    assert _phrase(client, phrase_index, {"token_limit": 1})["options"] == []


def test_phrase_shard_size(client, phrase_index):
    assert _phrase(client, phrase_index, {"shard_size": 1})["options"] == TYPO_OPTIONS[:1]


def test_phrase_separator_changes_scores(client, phrase_index):
    opts = _phrase(client, phrase_index, {"separator": "|"})["options"]
    assert opts == [
        {"text": "the quick brown fox", "score": 0.001385712},
        {"text": "the qick brown fox", "score": 0.0008409832},
        {"text": "the quick brwn fox", "score": 0.0008409832},
    ]


def test_phrase_analyzer_option(client, phrase_index):
    entry = _phrase(client, phrase_index, {"analyzer": "whitespace"}, text="The qick brwn fox")
    assert entry["options"] == [
        {"text": "The quick brown fox", "score": 0.00283747},
        {"text": "The qick brown fox", "score": 0.0017220496},
        {"text": "The quick brwn fox", "score": 0.0017220496},
    ]


def test_phrase_unknown_analyzer(client, phrase_index):
    err = _error(client, phrase_index, {"suggest": {"s": {"text": TYPO, "phrase": {
        "field": "title.trigram", "analyzer": "nope"}}}})
    _assert_shard_failure(err, "analyzer [nope] doesn't exists")


def test_phrase_on_field_without_shingles(client, phrase_index):
    res = _suggest(client, phrase_index, {"s": {"text": TYPO, "phrase": {"field": "title"}}})
    assert res["s"][0]["options"] == [
        {"text": "the quick brown fox", "score": 0.016547594},
        {"text": "the qick brown fox", "score": 0.01004267},
        {"text": "the quick brwn fox", "score": 0.01004267},
    ]


def test_phrase_gram_size_on_field_without_shingles(client, phrase_index):
    res = _suggest(client, phrase_index, {"s": {"text": TYPO, "phrase": {"field": "title", "gram_size": 2}}})
    assert res["s"][0]["options"] == [
        {"text": "the quick brown fox", "score": 0.0050149364},
        {"text": "the quick brwn fox", "score": 0.0030435454},
        {"text": "the qick brown fox", "score": 0.0030435454},
    ]


def test_phrase_field_without_unigrams_needs_explicit_generator(client, phrase_index):
    err = _error(client, phrase_index, {"suggest": {"s": {"text": TYPO, "phrase": {"field": "title.shingles"}}}})
    _assert_shard_failure(err, (
        "The default candidate generator for phrase suggest can't operate on field: [title.shingles] "
        "since it doesn't emit unigrams. If this is intentional try to set the candidate generator "
        "field explicitly"))


def test_phrase_unmapped_field(client, phrase_index):
    err = _error(client, phrase_index, {"suggest": {"s": {"text": TYPO, "phrase": {"field": "nope"}}}})
    _assert_shard_failure(err, "no mapping found for field [nope]")


def test_phrase_on_numeric_field_has_no_options(client, phrase_index):
    res = _suggest(client, phrase_index, {"s": {"text": TYPO, "phrase": {"field": "n"}}})
    assert res["s"] == [{"text": TYPO, "offset": 0, "length": 17, "options": []}]


def test_phrase_missing_field_option(client, phrase_index):
    err = _error(client, phrase_index, {"suggest": {"s": {"text": TYPO, "phrase": {}}}})
    assert err["type"] == "parse_exception"
    assert err["reason"] == "the required field option [field] is missing"


def test_phrase_on_empty_index(client, index_factory):
    name = index_factory(mappings=TEXT_MAPPING, settings=ONE_SHARD)
    assert _suggest(client, name, {"s": {"text": "appel", "phrase": {"field": "body"}}}) == {
        "s": [{"text": "appel", "offset": 0, "length": 5, "options": []}]}


# --------------------------------------------------------------------------- completion suggester


def test_completion_option_shape(client, product_index):
    entry = _completion(client, product_index, {}, prefix="lap", source=None)
    assert entry["text"] == "lap" and entry["offset"] == 0 and entry["length"] == 3
    assert entry["options"] == [
        {"text": "laptop", "_index": product_index, "_id": "a", "_score": 10.0, "_source": PRODUCT_DOCS["a"]},
        {"text": "laptop stand", "_index": product_index, "_id": "b", "_score": 5.0,
         "_source": PRODUCT_DOCS["b"]},
    ]
    assert list(entry["options"][0]) == ["text", "_index", "_id", "_score", "_source"]


def test_completion_orders_by_weight_then_text(client, product_index):
    entry = _completion(client, product_index, {})
    assert [(o["text"], o["_id"], o["_score"]) for o in entry["options"]] == [
        ("laptop", "a", 10.0), ("laptop stand", "b", 5.0), ("lamp", "c", 3.0),
        ("ladder", "e", 1.0), ("lantern", "d", 1.0)]


def test_completion_default_size_is_five(client, product_index):
    assert len(_completion(client, product_index, {})["options"]) == 5


def test_completion_size(client, product_index):
    assert _ids(_completion(client, product_index, {"size": 2})) == [("a", 10.0), ("b", 5.0)]


def test_completion_size_zero_rejected(client, product_index):
    err = _error(client, product_index, {"suggest": {"s": {"prefix": "la", "completion": {
        "field": "suggest", "size": 0}}}})
    assert err["type"] == "x_content_parse_exception"
    assert err["reason"].endswith("[completion] failed to parse field [size]")
    assert err["caused_by"] == {"type": "illegal_argument_exception", "reason": "size must be positive"}


def test_completion_source_disabled(client, product_index):
    entry = _completion(client, product_index, {}, prefix="lap", source=False)
    assert entry["options"] == [
        {"text": "laptop", "_index": product_index, "_id": "a", "_score": 10.0},
        {"text": "laptop stand", "_index": product_index, "_id": "b", "_score": 5.0},
    ]


def test_completion_source_filtering(client, product_index):
    entry = _completion(client, product_index, {}, prefix="lap", source=["name"])
    assert [o["_source"] for o in entry["options"]] == [{"name": "a"}, {"name": "b"}]


def test_completion_one_option_per_document(client, product_index):
    # Both "ladder" and "lasso" of document e match; it is suggested once.
    entry = _completion(client, product_index, {}, prefix="la", source=False)
    assert [o["_id"] for o in entry["options"]].count("e") == 1


def test_completion_matches_any_input(client, product_index):
    entry = _completion(client, product_index, {}, prefix="no")
    assert [(o["text"], o["_id"]) for o in entry["options"]] == [("notebook", "a")]


def test_completion_only_matches_from_start_of_input(client, product_index):
    assert _completion(client, product_index, {}, prefix="stand")["options"] == []


def test_completion_text_key_works_like_prefix(client, product_index):
    entry = _completion(client, product_index, {}, prefix="lam", key="text")
    assert entry["text"] == "lam"
    assert [(o["text"], o["_id"]) for o in entry["options"]] == [("lamp", "c")]


def test_completion_prefix_is_lowercased_by_simple_analyzer(client, product_index):
    assert _ids(_completion(client, product_index, {}, prefix="LAP")) == [("a", 10.0), ("b", 5.0)]


def test_completion_keeps_original_input_case(client, product_index):
    entry = _completion(client, product_index, {}, prefix="lä")
    assert [(o["text"], o["_id"], o["_score"]) for o in entry["options"]] == [("Läufer", "f", 7.0)]


def test_completion_empty_prefix_has_no_options(client, product_index):
    entry = _completion(client, product_index, {}, prefix="")
    assert entry == {"text": "", "offset": 0, "length": 0, "options": []}


def test_completion_missing_prefix(client, product_index):
    err = _error(client, product_index, {"suggest": {"s": {"completion": {"field": "suggest"}}}})
    _assert_shard_failure(err, "The required text option is missing")


def test_completion_fuzzy_default(client, product_index):
    entry = _completion(client, product_index, {"fuzzy": True}, prefix="lpa")
    assert [(o["text"], o["_score"]) for o in entry["options"]] == [
        ("laptop", 10.0), ("laptop stand", 5.0), ("lamp", 3.0), ("ladder", 1.0), ("lantern", 1.0)]


def test_completion_fuzzy_score_scales_with_exact_prefix(client, product_index):
    entry = _completion(client, product_index, {"fuzzy": {"fuzziness": 1}}, prefix="lapt0p")
    assert _ids(entry) == [("a", 40.0), ("b", 20.0)]


def test_completion_fuzzy_two_edits(client, product_index):
    entry = _completion(client, product_index, {"fuzzy": {"fuzziness": 2}}, prefix="lxptxp")
    assert _ids(entry) == [("a", 10.0), ("b", 5.0)]


def test_completion_fuzzy_auto_short_prefix_is_exact(client, product_index):
    assert _completion(client, product_index, {"fuzzy": {"fuzziness": "AUTO"}}, prefix="lx")["options"] == []


def test_completion_fuzzy_transpositions(client, dup_index):
    on = _completion(client, dup_index, {"fuzzy": {"fuzziness": 1}}, prefix="lpatop")
    assert _ids(on) == [("1", 10.0), ("2", 5.0)]
    off = _completion(client, dup_index, {"fuzzy": {"fuzziness": 1, "transpositions": False}}, prefix="lpatop")
    assert off["options"] == []
    off2 = _completion(client, dup_index, {"fuzzy": {"fuzziness": 2, "transpositions": False}}, prefix="lpatop")
    assert _ids(off2) == [("1", 10.0), ("2", 5.0)]


def test_completion_fuzzy_prefix_length(client, product_index):
    assert _completion(client, product_index, {"fuzzy": {"fuzziness": 1}}, prefix="xaptop")["options"] == []
    entry = _completion(client, product_index, {"fuzzy": {"fuzziness": 1, "prefix_length": 0}}, prefix="xaptop")
    assert _ids(entry) == [("a", 10.0), ("b", 5.0)]


def test_completion_fuzzy_min_length(client, dup_index):
    entry = _completion(client, dup_index, {"fuzzy": {"fuzziness": 1, "min_length": 3}}, prefix="laptxp")
    assert _ids(entry) == [("1", 40.0), ("2", 20.0)]
    assert _completion(client, dup_index, {"fuzzy": {"fuzziness": 1, "min_length": 7}},
                       prefix="laptxp")["options"] == []


def test_completion_fuzzy_unicode_aware(client, product_index):
    assert _completion(client, product_index, {"fuzzy": {"fuzziness": 1}}, prefix="lauf")["options"] == []
    entry = _completion(client, product_index, {"fuzzy": {"fuzziness": 1, "unicode_aware": True}}, prefix="lauf")
    assert [(o["text"], o["_id"], o["_score"]) for o in entry["options"]] == [("Läufer", "f", 7.0)]


@pytest.mark.parametrize("regex,expected", [
    ("la[mn].*", [("c", 3.0), ("d", 1.0)]),
    ("l.p", [("a", 10.0), ("b", 5.0)]),
    ("no", [("a", 10.0)]),
])
def test_completion_regex(client, product_index, regex, expected):
    entry = _completion(client, product_index, {}, prefix=regex, key="regex")
    assert entry["text"] == regex and entry["length"] == len(regex)
    assert _ids(entry) == expected


def test_completion_regex_options(client, dup_index):
    entry = _completion(client, dup_index, {"regex": {"flags": "ALL", "max_determinized_states": 1000}},
                        prefix="la.*", key="regex")
    assert _ids(entry) == [("1", 10.0), ("2", 5.0), ("3", 3.0)]


def test_completion_duplicates_kept_by_default(client, dup_index):
    entry = _completion(client, dup_index, {})
    assert [(o["text"], o["_id"]) for o in entry["options"]] == [("laptop", "1"), ("laptop", "2"), ("lamp", "3")]


def test_completion_skip_duplicates(client, dup_index):
    entry = _completion(client, dup_index, {"skip_duplicates": True})
    assert [(o["text"], o["_id"], o["_score"]) for o in entry["options"]] == [
        ("laptop", "1", 10.0), ("lamp", "3", 3.0)]


def test_completion_on_text_field(client, product_index):
    err = _error(client, product_index, {"suggest": {"s": {"prefix": "la", "completion": {"field": "body"}}}})
    _assert_shard_failure(err, "Field [body] is not a completion suggest field")


def test_completion_on_keyword_field(client, product_index):
    err = _error(client, product_index, {"suggest": {"s": {"prefix": "a", "completion": {"field": "name"}}}})
    _assert_shard_failure(err, "Field [name] is not a completion suggest field")


def test_completion_unmapped_field(client, product_index):
    err = _error(client, product_index, {"suggest": {"s": {"prefix": "la", "completion": {"field": "nope"}}}})
    _assert_shard_failure(err, "no mapping found for field [nope]")


def test_completion_missing_field_option(client, product_index):
    err = _error(client, product_index, {"suggest": {"s": {"prefix": "la", "completion": {}}}})
    assert err["type"] == "parse_exception"
    assert err["reason"] == "the required field option [field] is missing"


def test_completion_contexts_on_field_without_contexts(client, product_index):
    err = _error(client, product_index, {"suggest": {"s": {"prefix": "la", "completion": {
        "field": "suggest", "contexts": {"cat": ["a"]}}}}})
    _assert_shard_failure(err, "suggester [suggest] doesn't expect any context")


def test_completion_merges_across_indices(client, index_factory):
    a = index_factory(mappings=COMPLETION_MAPPING, settings=ONE_SHARD)
    b = index_factory(mappings=COMPLETION_MAPPING, settings=ONE_SHARD)
    _index_with_ids(client, a, {"1": {"suggest": "apple"}, "2": {"suggest": "apricot"}})
    _index_with_ids(client, b, {"3": {"suggest": {"input": "apex", "weight": 9}}})
    r = client.post(f"/{a},{b}/_search", json={
        "_source": False, "suggest": {"c": {"prefix": "ap", "completion": {"field": "suggest"}}}})
    assert r.status_code == 200, r.text
    assert [(o["text"], o["_index"], o["_id"], o["_score"]) for o in r.json()["suggest"]["c"][0]["options"]] == [
        ("apex", b, "3", 9.0), ("apple", a, "1", 1.0), ("apricot", a, "2", 1.0)]


def test_completion_on_empty_index(client, index_factory):
    name = index_factory(mappings=COMPLETION_MAPPING, settings=ONE_SHARD)
    assert _suggest(client, name, {"s": {"prefix": "ap", "completion": {"field": "suggest"}}}) == {
        "s": [{"text": "ap", "offset": 0, "length": 2, "options": []}]}


# --------------------------------------------------------------------------- completion: indexing


def _index_error(client, index, doc):
    r = client.post(f"/{index}/_doc", json=doc)
    assert r.status_code == 400, r.text
    return r.json()["error"]


@pytest.mark.parametrize("weight,reason", [
    (-1, "weight must be in the interval [0..2147483647], but was [-1]"),
    ("abc", "weight must be an integer, but was [abc]"),
    (2.5, "weight must be an integer, but was [2.5]"),
])
def test_completion_invalid_weight(client, completion_index, weight, reason):
    err = _index_error(client, completion_index, {"suggest": {"input": "x", "weight": weight}})
    assert err["type"] == "mapper_parsing_exception"
    assert err["reason"] == "failed to parse"
    assert err["caused_by"] == {"type": "illegal_argument_exception", "reason": reason}


def test_completion_unknown_input_key(client, completion_index):
    err = _index_error(client, completion_index, {"suggest": {"input": "x", "bogus": 1}})
    assert err["type"] == "mapper_parsing_exception"
    assert err["caused_by"]["reason"] == "unknown field name [bogus], must be one of [input, weight, contexts]"


def test_completion_reserved_character_in_input(client, completion_index):
    err = _index_error(client, completion_index, {"suggest": {"input": "a\u001fb"}})
    assert err["type"] == "mapper_parsing_exception"
    assert err["caused_by"] == {
        "type": "illegal_argument_exception",
        "reason": "Illegal input [a\u001fb] UTF-16 codepoint [0x1f] at position 1 is a reserved character",
    }


def test_completion_numeric_value_rejected(client, completion_index):
    err = _index_error(client, completion_index, {"suggest": 123})
    assert err["type"] == "mapper_parsing_exception"
    assert err["caused_by"]["type"] == "parsing_exception"
    assert err["caused_by"]["reason"] == "failed to parse [suggest]: expected text or object, but got VALUE_NUMBER"


# --------------------------------------------------------------------------- completion: mapping parameters


def test_completion_mapping_defaults(client, index_factory):
    name = index_factory(mappings=COMPLETION_MAPPING)
    r = client.get(f"/{name}/_mapping")
    assert r.json()[name]["mappings"]["properties"]["suggest"] == {
        "type": "completion",
        "analyzer": "simple",
        "preserve_separators": True,
        "preserve_position_increments": True,
        "max_input_length": 50,
    }


def test_completion_mapping_with_contexts_round_trip(client, context_index):
    props = client.get(f"/{context_index}/_mapping").json()[context_index]["mappings"]["properties"]
    assert props["suggest"]["contexts"] == [
        {"name": "cat", "type": "CATEGORY"},
        {"name": "loc", "type": "GEO", "precision": 4},
    ]
    assert props["song"]["contexts"] == [{"name": "genre", "type": "CATEGORY", "path": "genre"}]


def test_completion_mapping_search_analyzer_round_trip(client, params_index):
    props = client.get(f"/{params_index}/_mapping").json()[params_index]["mappings"]["properties"]
    assert props["split"] == {
        "type": "completion", "analyzer": "standard", "search_analyzer": "keyword",
        "preserve_separators": True, "preserve_position_increments": True, "max_input_length": 50,
    }
    assert props["short"]["max_input_length"] == 5
    assert props["nosep"]["preserve_separators"] is False
    assert props["stop_nopos"]["preserve_position_increments"] is False


def _param_texts(client, index, field, prefix):
    r = client.post(f"/{index}/_search", json={
        "_source": False, "suggest": {"s": {"prefix": prefix, "completion": {"field": field}}}})
    assert r.status_code == 200, r.text
    return _texts(r.json()["suggest"]["s"][0])


@pytest.mark.parametrize("field,prefix,expected", [
    ("plain", "foob", []),
    ("nosep", "foob", ["foo bar"]),
    ("plain", "foo b", ["foo bar"]),
    ("nosep", "foo b", ["foo bar"]),
])
def test_completion_preserve_separators(client, params_index, field, prefix, expected):
    assert _param_texts(client, params_index, field, prefix) == expected


@pytest.mark.parametrize("field,prefix,expected", [
    ("stop", "beat", []),
    ("stop_nopos", "beat", ["The Beatles"]),
    ("plain", "beat", []),
    ("stop", "the b", ["The Beatles"]),
    ("stop_nopos", "the b", ["The Beatles"]),
])
def test_completion_preserve_position_increments(client, params_index, field, prefix, expected):
    assert _param_texts(client, params_index, field, prefix) == expected


def test_completion_max_input_length_truncates_input(client, params_index):
    assert _param_texts(client, params_index, "short", "hello w") == []
    # The suggested text is the truncated input.
    assert _param_texts(client, params_index, "short", "hello") == ["Hello"]
    assert _param_texts(client, params_index, "plain", "hello") == ["Hello World"]


def test_completion_keyword_analyzer_is_case_sensitive(client, params_index):
    assert _param_texts(client, params_index, "kw", "hel") == []
    assert _param_texts(client, params_index, "kw", "Hel") == ["Hello World"]


def test_completion_search_analyzer(client, params_index):
    assert _param_texts(client, params_index, "split", "hel") == ["Hello World"]
    assert _param_texts(client, params_index, "split", "Hel") == []
    assert _param_texts(client, params_index, "split", "hello w") == []


@pytest.mark.parametrize("field,error_type,reason", [
    ({"type": "completion", "analyzer": "nope"}, "mapper_parsing_exception",
     "Failed to parse mapping [_doc]: analyzer [nope] has not been configured in mappings"),
    ({"type": "completion", "contexts": [{"name": "a", "type": "weird"}]}, "mapper_parsing_exception",
     "Failed to parse mapping [_doc]: No context type for [weird]"),
    ({"type": "completion", "contexts": [{"type": "category"}]}, "mapper_parsing_exception",
     "Failed to parse mapping [_doc]: missing [name] in context mapping"),
    ({"type": "completion", "bogus": 1}, "mapper_parsing_exception",
     "Failed to parse mapping [_doc]: unknown parameter [bogus] on mapper [x] of type [completion]"),
    ({"type": "completion", "max_input_length": 0}, "mapper_parsing_exception",
     "Failed to parse mapping [_doc]: [max_input_length] must be > 0 but was [0]"),
])
def test_completion_invalid_mapping(client, field, error_type, reason):
    name = f"slopensearch-test-{uuid.uuid4().hex[:8]}"
    r = client.put(f"/{name}", json={"mappings": {"properties": {"x": field}}})
    if r.status_code == 200:
        client.delete(f"/{name}")
    assert r.status_code == 400, r.text
    err = r.json()["error"]
    assert err["type"] == error_type
    assert err["reason"] == reason


# --------------------------------------------------------------------------- completion: contexts


def _ctx(client, index, prefix, contexts, field="suggest"):
    comp = {"field": field}
    if contexts is not None:
        comp["contexts"] = contexts
    r = client.post(f"/{index}/_search", json={
        "_source": False, "suggest": {"s": {"prefix": prefix, "completion": comp}}})
    return r


def _ctx_options(client, index, prefix, contexts, field="suggest"):
    r = _ctx(client, index, prefix, contexts, field)
    assert r.status_code == 200, r.text
    return [(o["_id"], o["_score"], o["contexts"]) for o in r.json()["suggest"]["s"][0]["options"]]


def test_completion_context_required_at_query_time(client, context_index):
    r = _ctx(client, context_index, "m", None)
    assert r.status_code == 400, r.text
    _assert_shard_failure(r.json()["error"], "Missing mandatory contexts in context query")


def test_completion_context_required_at_index_time(client, context_index):
    r = client.post(f"/{context_index}/_doc", json={"suggest": {"input": "zebra"}})
    assert r.status_code == 400, r.text
    assert r.json()["error"] == {
        "root_cause": [{"type": "illegal_argument_exception",
                        "reason": "Contexts are mandatory in context enabled completion field [suggest]"}],
        "type": "illegal_argument_exception",
        "reason": "Contexts are mandatory in context enabled completion field [suggest]",
    }


def test_completion_unknown_context_at_index_time(client, context_index):
    err = _index_error(client, context_index, {"suggest": {"input": "zz", "contexts": {"nope": ["a"]}}})
    assert err["type"] == "mapper_parsing_exception"
    assert err["caused_by"] == {"type": "illegal_argument_exception",
                                "reason": "Unknown context name [nope], must be one of [cat, loc]"}


def test_completion_unknown_context_at_query_time(client, context_index):
    r = _ctx(client, context_index, "m", {"bogus": ["x"]})
    assert r.status_code == 400, r.text
    _assert_shard_failure(r.json()["error"], "Unknown context name [bogus], must be one of [cat, loc]")


def test_completion_category_context(client, context_index):
    assert _ctx_options(client, context_index, "m", {"cat": ["food"]}) == [
        ("mcd", 8.0, {"cat": ["food"]}), ("monkey", 3.0, {"cat": ["food"]})]
    assert _ctx_options(client, context_index, "t", {"cat": ["cafe"]}) == [
        ("timmy", 5.0, {"cat": ["cafe"]})]
    assert _ctx_options(client, context_index, "m", {"cat": ["cafe"]}) == []


def test_completion_category_context_option_shape(client, context_index):
    r = _ctx(client, context_index, "t", {"cat": ["food"]})
    opt = r.json()["suggest"]["s"][0]["options"][0]
    assert list(opt) == ["text", "_index", "_id", "_score", "contexts"]
    assert opt["text"] == "timmy's"


def test_completion_category_context_boost(client, context_index):
    assert _ctx_options(client, context_index, "t", {"cat": [{"context": "cafe", "boost": 4}, "food"]}) == [
        ("timmy", 20.0, {"cat": ["cafe", "food"]})]
    assert _ctx_options(client, context_index, "m", {"cat": [{"context": "cafe", "boost": 4}, "food"]}) == [
        ("mcd", 8.0, {"cat": ["food"]}), ("monkey", 3.0, {"cat": ["food"]})]


def test_completion_category_context_prefix(client, context_index):
    assert _ctx_options(client, context_index, "t", {"cat": [{"context": "ca", "prefix": True}]}) == [
        ("timmy", 5.0, {"cat": ["cafe"]})]
    assert _ctx_options(client, context_index, "t", {"cat": [{"context": "ca"}]}) == []


def test_completion_category_context_from_path(client, context_index):
    assert _ctx_options(client, context_index, "r", {"genre": ["rap"]}, field="song") == [
        ("rap", 4.0, {"genre": ["rap"]})]
    assert _ctx_options(client, context_index, "r", {"genre": ["pop"]}, field="song") == [
        ("rap", 4.0, {"genre": ["pop"]})]
    assert _ctx_options(client, context_index, "r", {"genre": ["rock", "rap"]}, field="song") == [
        ("rap", 4.0, {"genre": ["rap"]}), ("rock", 2.0, {"genre": ["rock"]})]


def test_completion_geo_context(client, context_index):
    assert _ctx_options(client, context_index, "m", {"loc": TORONTO}) == [("monkey", 3.0, {"loc": ["dpz8"]})]
    assert _ctx_options(client, context_index, "t", {"loc": TORONTO}) == [("timmy", 5.0, {"loc": ["dpz8"]})]


def test_completion_geo_context_geohash_string(client, context_index):
    assert _ctx_options(client, context_index, "m", {"loc": "dpz8"}) == [("monkey", 3.0, {"loc": ["dpz8"]})]


def test_completion_geo_context_precision(client, context_index):
    assert _ctx_options(client, context_index, "m", {"loc": [{"context": TORONTO, "precision": 1}]}) == [
        ("monkey", 3.0, {"loc": ["dpz8"]})]


def test_completion_geo_context_boost(client, context_index):
    assert _ctx_options(client, context_index, "m", {"loc": [
        {"context": TORONTO, "precision": 2, "boost": 2},
        {"context": BERLIN, "precision": 2},
    ]}) == [("mcd", 8.0, {"loc": ["u33d"]}), ("monkey", 6.0, {"loc": ["dpz8"]})]


def test_completion_category_and_geo_contexts_combine(client, context_index):
    assert _ctx_options(client, context_index, "m", {"cat": ["food"], "loc": TORONTO}) == [
        ("mcd", 8.0, {"cat": ["food"]}), ("monkey", 3.0, {"loc": ["dpz8"], "cat": ["food"]})]


# --------------------------------------------------------------------------- suggest in a search request


def test_suggest_only_request_skips_hits(client, product_index):
    r = client.post(f"/{product_index}/_search", json={
        "suggest": {"s": {"prefix": "lap", "completion": {"field": "suggest"}}}})
    assert r.status_code == 200, r.text
    body = r.json()
    assert list(body) == ["took", "timed_out", "_shards", "hits", "suggest"]
    assert body["hits"] == {"total": {"value": 0, "relation": "eq"}, "max_score": None, "hits": []}


def test_suggest_with_query(client, product_index):
    r = client.post(f"/{product_index}/_search", json={
        "_source": False,
        "query": {"term": {"name": "c"}},
        "suggest": {"s": {"prefix": "lap", "completion": {"field": "suggest"}}}})
    assert r.status_code == 200, r.text
    body = r.json()
    assert body["hits"]["total"] == {"value": 1, "relation": "eq"}
    assert [h["_id"] for h in body["hits"]["hits"]] == ["c"]
    assert _ids(body["suggest"]["s"][0]) == [("a", 10.0), ("b", 5.0)]


def test_suggest_with_size_zero_query(client, freq_index):
    r = client.post(f"/{freq_index}/_search", json={
        "size": 0, "query": {"match": {"body": "mouse"}},
        "suggest": {"s": {"text": "mous", "term": {"field": "body"}}}})
    assert r.status_code == 200, r.text
    body = r.json()
    assert body["hits"] == {"total": {"value": 3, "relation": "eq"}, "max_score": None, "hits": []}
    assert body["suggest"]["s"][0]["options"] == [{"text": "mouse", "score": 0.75, "freq": 3}]


def test_suggest_empty_object_is_omitted(client, freq_index):
    r = client.post(f"/{freq_index}/_search", json={"size": 0, "suggest": {}})
    assert r.status_code == 200, r.text
    assert "suggest" not in r.json()


def test_multiple_suggestions_of_different_types(client, product_index):
    res = _suggest(client, product_index, {
        "text": "lapto",
        "t": {"term": {"field": "name"}},
        "c": {"prefix": "lap", "completion": {"field": "suggest", "size": 1}},
    })
    assert set(res) == {"t", "c"}
    assert res["t"] == [{"text": "lapto", "offset": 0, "length": 5, "options": []}]
    assert _ids(res["c"][0]) == [("a", 10.0)]


def test_suggestions_are_returned_sorted_by_name(client, product_index):
    r = client.post(f"/{product_index}/_search", content=json.dumps({"_source": False, "suggest": {
        "zeta": {"prefix": "lap", "completion": {"field": "suggest"}},
        "alpha": {"prefix": "lam", "completion": {"field": "suggest"}},
        "mid": {"prefix": "no", "completion": {"field": "suggest"}},
    }}), headers={"Content-Type": "application/json"})
    assert r.status_code == 200, r.text
    assert list(r.json()["suggest"]) == ["alpha", "mid", "zeta"]


def test_typed_keys(client, index_factory):
    name = index_factory(mappings={"properties": {"body": {"type": "text"}, "sug": {"type": "completion"}}},
                         settings=ONE_SHARD)
    _index_with_ids(client, name, {"1": {"body": "apple banana", "sug": "apple"},
                                   "2": {"body": "cherry", "sug": "apricot"}})
    r = client.post(f"/{name}/_search", params={"typed_keys": "true"}, json={"_source": False, "suggest": {
        "text": "appel",
        "t": {"term": {"field": "body"}},
        "p": {"phrase": {"field": "body"}},
        "c": {"prefix": "ap", "completion": {"field": "sug"}},
    }})
    assert r.status_code == 200, r.text
    res = r.json()["suggest"]
    assert list(res) == ["completion#c", "phrase#p", "term#t"]
    assert res["term#t"][0]["options"] == [{"text": "apple", "score": 0.8, "freq": 1}]
    assert res["phrase#p"][0]["options"] == [{"text": "apple", "score": 0.56325024}]
    assert [o["_id"] for o in res["completion#c"][0]["options"]] == ["1", "2"]


def test_typed_keys_false_keeps_plain_names(client, freq_index):
    res = _suggest(client, freq_index, {"s": {"text": "mous", "term": {"field": "body"}}}, typed_keys="false")
    assert list(res) == ["s"]


def test_suggest_via_get_source_param(client, freq_index):
    r = client.get(f"/{freq_index}/_search", params={
        "source": json.dumps({"suggest": {"s": {"text": "mous", "term": {"field": "body"}}}}),
        "source_content_type": "application/json"})
    assert r.status_code == 200, r.text
    assert r.json()["suggest"]["s"][0]["options"] == [{"text": "mouse", "score": 0.75, "freq": 3}]


def test_suggest_in_msearch(client, freq_index):
    payload = json.dumps({"index": freq_index}) + "\n" + json.dumps(
        {"size": 0, "suggest": {"s": {"text": "mous", "term": {"field": "body"}}}}) + "\n"
    r = client.post("/_msearch", content=payload.encode(), headers={"Content-Type": "application/x-ndjson"})
    assert r.status_code == 200, r.text
    assert r.json()["responses"][0]["suggest"]["s"][0]["options"] == [{"text": "mouse", "score": 0.75, "freq": 3}]


def test_count_rejects_suggest(client, freq_index):
    r = client.post(f"/{freq_index}/_count", json={"suggest": {"s": {"text": "mous", "term": {"field": "body"}}}})
    assert r.status_code == 400, r.text
    err = r.json()["error"]
    assert err["type"] == "parsing_exception"
    assert err["reason"] == "request does not support [suggest]"


def test_unknown_suggester_type(client, freq_index):
    err = _error(client, freq_index, {"suggest": {"s": {"text": "x", "foo": {"field": "body"}}}})
    assert err["type"] == "named_object_not_found_exception"
    assert err["reason"].endswith("unknown field [foo]")
    assert err["root_cause"][0]["type"] == "named_object_not_found_exception"


@pytest.mark.parametrize("suggestion", [{}, {"text": "x"}])
def test_suggestion_without_suggester(client, freq_index, suggestion):
    err = _error(client, freq_index, {"suggest": {"s": suggestion}})
    assert err == {
        "root_cause": [{"type": "parse_exception", "reason": "missing suggestion object"}],
        "type": "parse_exception",
        "reason": "missing suggestion object",
    }


def test_suggest_must_be_an_object(client, freq_index):
    err = _error(client, freq_index, {"suggest": "x"})
    assert err["type"] == "parsing_exception"
    assert err["reason"] == "Unknown key for a VALUE_STRING in [suggest]."
