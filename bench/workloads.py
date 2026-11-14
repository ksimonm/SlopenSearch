"""Index definitions, query sets and the correctness digest used by run.py."""

from __future__ import annotations

import hashlib
import json
import random

TEXT = "bench-text"
VEC = "bench-vec"
SWEEP = "bench-sweep"

TEXT_MAPPING = {
    "settings": {"number_of_shards": 1, "number_of_replicas": 0},
    "mappings": {"properties": {
        "seq": {"type": "long"},
        "title": {"type": "text"},
        "body": {"type": "text"},
        "tags": {"type": "keyword"},
        "category": {"type": "keyword"},
        "user_id": {"type": "keyword"},
        "price": {"type": "float"},
        "quantity": {"type": "integer"},
        "rating": {"type": "float"},
        "created": {"type": "date"},
        "location": {"type": "geo_point"},
    }},
}


def vec_mapping(dim: int) -> dict:
    return {
        "settings": {"number_of_shards": 1, "number_of_replicas": 0, "index.knn": True},
        "mappings": {"properties": {
            "vid": {"type": "long"},
            "cat": {"type": "integer"},
            "vec": {"type": "knn_vector", "dimension": dim,
                    "method": {"name": "hnsw", "space_type": "l2", "engine": "faiss"}},
        }},
    }


def text_queries(q: dict, n: int = 50, seed: int = 7) -> dict[str, list[dict]]:
    """Search bodies per workload name; every workload gets `n` seeded variants."""
    r = random.Random(seed)
    mid, freq, rare = q["mid"], q["frequent"], q["rare"]
    tags, users, cats = q["tags"], q["users"], q["categories"]
    years = list(range(2015, 2025))

    def year_range():
        y = r.choice(years)
        return {"range": {"created": {"gte": f"{y}-01-01", "lt": f"{y + 1}-01-01"}}}

    gens = {
        "match": lambda: {"query": {"match": {"body": r.choice(mid)}}},
        "match_frequent": lambda: {"query": {"match": {"body": r.choice(freq)}}},
        "match_and": lambda: {"query": {"match": {"body": {
            "query": f"{r.choice(freq)} {r.choice(mid)}", "operator": "and"}}}},
        "multi_match": lambda: {"query": {"multi_match": {
            "query": f"{r.choice(mid)} {r.choice(rare)}", "fields": ["title^2", "body"]}}},
        "bool": lambda: {"query": {"bool": {
            "must": [{"match": {"body": r.choice(mid)}}],
            "filter": [{"term": {"category": r.choice(cats)}},
                       {"range": {"price": {"gte": 5, "lte": 200}}}],
            "must_not": [{"term": {"tags": r.choice(tags)}}]}}},
        "filter_term_range": lambda: {"query": {"bool": {"filter": [
            {"term": {"tags": r.choice(tags)}}, year_range()]}}},
        "geo_distance": lambda: {"query": {"bool": {"filter": [{"geo_distance": {
            "distance": "300km",
            "location": {"lat": round(r.uniform(-50, 60), 3), "lon": round(r.uniform(-170, 170), 3)}}}]}}},
        "sort_price": lambda: {"query": {"term": {"category": r.choice(cats)}},
                               "sort": [{"price": "desc"}, {"seq": "asc"}], "size": 20},
        "sort_date": lambda: {"query": {"match": {"body": r.choice(mid)}},
                              "sort": [{"created": "desc"}, {"seq": "asc"}], "size": 20},
        "agg_terms": lambda: {"size": 0, "query": {"match": {"body": r.choice(mid)}},
                              "aggs": {"t": {"terms": {"field": "tags", "size": 20}}}},
        "agg_date_histogram": lambda: {"size": 0, "query": {"term": {"category": r.choice(cats)}},
                                       "aggs": {"h": {"date_histogram": {"field": "created", "calendar_interval": "month"},
                                                      "aggs": {"p": {"avg": {"field": "price"}}}}}},
        "agg_metrics": lambda: {"size": 0, "query": {"range": {"quantity": {"gte": r.randrange(0, 500), "lt": 1000}}},
                                "aggs": {"avg_price": {"avg": {"field": "price"}},
                                         "max_rating": {"max": {"field": "rating"}},
                                         "sum_qty": {"sum": {"field": "quantity"}}}},
        "highlight": lambda: {"query": {"match": {"body": r.choice(mid)}},
                              "highlight": {"fields": {"body": {}}}, "size": 10},
        "deep_from": lambda: {"query": {"match": {"body": r.choice(freq)}},
                              "sort": [{"seq": "asc"}], "from": 5000, "size": 20},
    }
    return {name: [g() for _ in range(n)] for name, g in gens.items()}


def search_after_queries(q: dict, n: int = 20, seed: int = 8) -> list[dict]:
    r = random.Random(seed)
    return [{"query": {"bool": {"filter": [{"term": {"category": r.choice(q["categories"])}}]}}}
            for _ in range(n)]


def knn_queries(qv, truth: dict, k: int = 10) -> dict[str, list[dict]]:
    plain, filt = [], []
    for i, v in enumerate(qv.tolist()):
        vec = [round(x, 4) for x in v]
        plain.append({"size": k, "_source": False, "docvalue_fields": ["vid"],
                      "query": {"knn": {"vec": {"vector": vec, "k": k}}}})
        filt.append({"size": k, "_source": False, "docvalue_fields": ["vid"],
                     "query": {"knn": {"vec": {"vector": vec, "k": k,
                                               "filter": {"term": {"cat": truth["query_cat"][i]}}}}}})
    return {"knn": plain, "knn_filter": filt}


def digest(resp: dict) -> dict:
    """What the correctness check compares: total hits, top ids, aggregation results."""
    out = {}
    hits = resp.get("hits", {})
    tot = hits.get("total")
    out["total"] = tot.get("value") if isinstance(tot, dict) else tot
    out["ids"] = [h.get("_id") for h in hits.get("hits", [])]
    if "aggregations" in resp:
        out["aggs"] = _agg_digest(resp["aggregations"])
    if any("highlight" in h for h in hits.get("hits", [])):
        out["highlight"] = sum(1 for h in hits["hits"] if h.get("highlight"))
    return out


def _round(x):
    if isinstance(x, float):
        return float(f"{x:.4g}")
    return x


def _agg_digest(a):
    if isinstance(a, dict):
        out = {}
        for k, v in a.items():
            if k in ("doc_count_error_upper_bound", "sum_other_doc_count", "key_as_string"):
                continue
            out[k] = _agg_digest(v)
        return out
    if isinstance(a, list):
        return [_agg_digest(x) for x in a]
    return _round(a)


def agg_hash(d) -> str:
    return hashlib.sha1(json.dumps(d, sort_keys=True).encode()).hexdigest()[:12]
