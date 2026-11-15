"""Deterministic synthetic corpora for the benchmarks.

Two data sets per size, generated once into <data-dir>/<size>/ and reused:

  docs.ndjson     one JSON document (the _source) per line: title, body, tags, category,
                  user_id, seq, price, quantity, rating, created, location
  vectors.f32     float32 matrix, n x dim, row-major (clustered Gaussian vectors)
  vec_cat.u8      one category byte (0..9) per vector, for filtered k-NN
  vec_queries.f32 query vectors (n_queries x dim)
  vec_truth.json  exact top-k neighbours (L2) of every query, unfiltered and filtered
  queries.json    seeded query terms: frequent / mid / rare words, tag and user values
  meta.json       the parameters; a mismatch regenerates the set

Words follow a Zipf distribution over a pseudo-word vocabulary, so posting lists have
a realistic mix of very long (stop-word-like) and very short lengths.
"""

from __future__ import annotations

import argparse
import json
import multiprocessing as mp
import os
import time
from pathlib import Path

import numpy as np

SIZES = {
    #          docs        vectors   vector queries
    "tiny":   (10_000,     5_000,    100),
    "small":  (100_000,    20_000,   200),
    "medium": (500_000,    100_000,  200),
    "large":  (2_000_000,  500_000,  200),
}
DIM = 128
VOCAB = 50_000
N_TAGS = 300
N_USERS = 20_000
CATEGORIES = ["books", "music", "garden", "tools", "toys", "sports", "food", "auto",
              "health", "beauty", "office", "pets", "games", "travel", "home", "baby",
              "movies", "shoes", "jewelry", "outdoor"]
SEED = 20261115
GEN_VERSION = 1
DATE_FROM = 1420070400  # 2015-01-01
DATE_TO = 1767225600    # 2026-01-01
TRUTH_K = 100


def default_data_dir() -> Path:
    return Path(os.environ.get("BENCH_DATA_DIR", Path.home() / ".cache" / "slopbench" / "data"))


def _vocabulary(rng: np.random.Generator) -> list[str]:
    """VOCAB distinct pronounceable pseudo-words; frequent ranks get the short words."""
    cons = list("bcdfghjklmnprstvwz") + ["ch", "sh", "th", "st", "tr", "br", "pl"]
    vow = ["a", "e", "i", "o", "u", "ai", "ea", "ou", "y"]
    words: list[str] = []
    seen: set[str] = set()
    while len(words) < VOCAB:
        m = 20_000
        syl = rng.choice([1, 2, 2, 3, 3, 3, 4], m)
        c = rng.integers(0, len(cons), (m, 5))
        v = rng.integers(0, len(vow), (m, 4))
        tail = rng.random(m) < 0.4
        for i in range(m):
            w = "".join(cons[c[i, j]] + vow[v[i, j]] for j in range(syl[i]))
            if tail[i]:
                w += cons[c[i, 4]]
            if w not in seen:
                seen.add(w)
                words.append(w)
    words = words[:VOCAB]
    words.sort(key=lambda s: (len(s), s))
    return words


def _zipf_cdf(n: int, s: float) -> np.ndarray:
    p = 1.0 / np.arange(1, n + 1) ** s
    c = np.cumsum(p)
    return c / c[-1]


def _sample(rng, cdf, size):
    return np.searchsorted(cdf, rng.random(size), side="right").clip(0, len(cdf) - 1)


DOC_CHUNK = 20_000


def _doc_chunk(args) -> bytes:
    """Documents [start, start+m) from their own seeded generator (parallel, deterministic)."""
    start, m, words = args
    rng = np.random.default_rng([SEED + 2, start])
    wcdf = _zipf_cdf(VOCAB, 1.07)
    tcdf = _zipf_cdf(N_TAGS, 0.9)
    ccdf = _zipf_cdf(len(CATEGORIES), 0.6)
    tags = [f"tag-{i:03d}" for i in range(N_TAGS)]
    warr = np.array(words, dtype=object)
    blen = np.clip(rng.lognormal(4.6, 0.55, m).astype(int), 10, 1200)   # ~ 100 words median
    tlen = rng.integers(3, 11, m)
    total = int(blen.sum() + tlen.sum())
    toks = warr[_sample(rng, wcdf, total)]
    ntags = rng.integers(1, 5, m)
    tag_ids = _sample(rng, tcdf, int(ntags.sum()))
    cats = _sample(rng, ccdf, m)
    users = rng.integers(0, N_USERS, m)
    price = np.round(rng.lognormal(3.0, 1.0, m), 2)
    qty = rng.integers(0, 1000, m)
    rating = np.round(rng.uniform(1, 5, m), 1)
    created = rng.integers(DATE_FROM, DATE_TO, m)
    lat = np.round(rng.uniform(-60, 70, m), 5)
    lon = np.round(rng.uniform(-180, 180, m), 5)
    pos = tpos = 0
    lines = []
    for i in range(m):
        tl, bl, nt = int(tlen[i]), int(blen[i]), int(ntags[i])
        title = " ".join(toks[pos:pos + tl]); pos += tl
        body = " ".join(toks[pos:pos + bl]); pos += bl
        doc_tags = sorted({tags[t] for t in tag_ids[tpos:tpos + nt]}); tpos += nt
        doc = {
            "seq": start + i,
            "title": title.capitalize(),
            "body": body,
            "tags": doc_tags,
            "category": CATEGORIES[cats[i]],
            "user_id": f"u{users[i]:05d}",
            "price": float(price[i]),
            "quantity": int(qty[i]),
            "rating": float(rating[i]),
            "created": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(int(created[i]))),
            "location": {"lat": float(lat[i]), "lon": float(lon[i])},
        }
        lines.append(json.dumps(doc, separators=(",", ":")))
    return ("\n".join(lines) + "\n").encode()


def gen_docs(path: Path, n: int, words: list[str]) -> None:
    tmp = path.with_suffix(".tmp")
    t0 = time.time()
    jobs = [(s, min(DOC_CHUNK, n - s), words) for s in range(0, n, DOC_CHUNK)]
    with open(tmp, "wb") as f, mp.get_context("fork").Pool(min(8, os.cpu_count() or 1)) as pool:
        for k, blob in enumerate(pool.imap(_doc_chunk, jobs)):
            f.write(blob)
            if k % 10 == 9 or k == len(jobs) - 1:
                done = min(n, (k + 1) * DOC_CHUNK)
                print(f"  docs {done:>9,}/{n:,}  {time.time() - t0:5.1f}s", flush=True)
    tmp.rename(path)


def gen_vectors(d: Path, n: int, nq: int, rng: np.random.Generator) -> None:
    n_clusters = 64
    centers = rng.normal(0, 1, (n_clusters, DIM)).astype(np.float32)
    out = np.empty((n, DIM), dtype=np.float32)
    assign = rng.integers(0, n_clusters, n)
    step = 100_000
    for s in range(0, n, step):
        e = min(n, s + step)
        out[s:e] = centers[assign[s:e]] + rng.normal(0, 0.6, (e - s, DIM)).astype(np.float32)
    out = np.round(out, 4)
    cat = rng.integers(0, 10, n).astype(np.uint8)
    qassign = rng.integers(0, n_clusters, nq)
    q = np.round(centers[qassign] + rng.normal(0, 0.6, (nq, DIM)).astype(np.float32), 4)
    q = q.astype(np.float32)
    qcat = rng.integers(0, 10, nq)
    # exact L2 ground truth
    out64 = out.astype(np.float64)
    sq = (out64 ** 2).sum(1)
    truth, truth_f = [], []
    for i in range(nq):
        dist = sq - 2 * out64 @ q[i].astype(np.float64)
        top = np.argpartition(dist, TRUTH_K)[:TRUTH_K]
        truth.append(top[np.argsort(dist[top])].tolist())
        mask = cat == qcat[i]
        idx = np.nonzero(mask)[0]
        dm = dist[idx]
        k = min(TRUTH_K, len(idx) - 1)
        topf = idx[np.argpartition(dm, k)[:k]]
        truth_f.append(topf[np.argsort(dist[topf])].tolist())
    out.tofile(d / "vectors.f32")
    cat.tofile(d / "vec_cat.u8")
    q.tofile(d / "vec_queries.f32")
    (d / "vec_truth.json").write_text(json.dumps(
        {"k": TRUTH_K, "query_cat": qcat.tolist(), "truth": truth, "truth_filtered": truth_f}))


def gen_queries(d: Path, rng: np.random.Generator, words: list[str]) -> None:
    """Query terms by frequency band (rank in the Zipf vocabulary)."""
    def pick(lo, hi, k):
        return [words[i] for i in rng.integers(lo, hi, k)]
    q = {
        "frequent": pick(0, 50, 200),       # stop-word-like, huge posting lists
        "mid": pick(100, 2000, 400),
        "rare": pick(5000, 30000, 400),
        "tags": [f"tag-{i:03d}" for i in rng.integers(0, 60, 200)],
        "users": [f"u{i:05d}" for i in rng.integers(0, N_USERS, 200)],
        "categories": CATEGORIES,
    }
    (d / "queries.json").write_text(json.dumps(q))


def ensure(size: str, data_dir: Path | None = None, docs: int | None = None,
           vectors: int | None = None) -> Path:
    """Generate the data set for `size` unless an identical one already exists."""
    n_docs, n_vec, n_q = SIZES[size]
    n_docs = docs or n_docs
    n_vec = vectors or n_vec
    d = (data_dir or default_data_dir()) / f"{size}-{n_docs}-{n_vec}"
    meta = {"version": GEN_VERSION, "seed": SEED, "docs": n_docs, "vectors": n_vec,
            "vector_queries": n_q, "dim": DIM}
    mpath = d / "meta.json"
    if mpath.exists() and json.loads(mpath.read_text()) == meta:
        return d
    d.mkdir(parents=True, exist_ok=True)
    print(f"generating data set {d} ({n_docs:,} docs, {n_vec:,} vectors)", flush=True)
    t0 = time.time()
    rng = np.random.default_rng(SEED)
    words = _vocabulary(rng)
    gen_queries(d, np.random.default_rng(SEED + 1), words)
    gen_docs(d / "docs.ndjson", n_docs, words)
    gen_vectors(d, n_vec, n_q, np.random.default_rng(SEED + 3))
    mpath.write_text(json.dumps(meta))
    print(f"data set ready in {time.time() - t0:.0f}s, docs.ndjson "
          f"{(d / 'docs.ndjson').stat().st_size / 1e9:.2f} GB", flush=True)
    return d


def vectors_ndjson(d: Path) -> Path:
    """The vectors as bulk-ready documents {"vid", "cat", "vec"}, written on first use."""
    p = d / "vectors.ndjson"
    if p.exists():
        return p
    v, cat, _, _ = load_vectors(d)
    tmp = p.with_suffix(".tmp")
    with open(tmp, "w") as f:
        for s in range(0, len(v), 10_000):
            rows = v[s:s + 10_000].tolist()
            f.write("".join(
                '{"vid":%d,"cat":%d,"vec":[%s]}\n' % (s + i, cat[s + i], ",".join("%g" % x for x in r))
                for i, r in enumerate(rows)))
    tmp.rename(p)
    return p


def load_vectors(d: Path):
    meta = json.loads((d / "meta.json").read_text())
    v = np.fromfile(d / "vectors.f32", dtype=np.float32).reshape(meta["vectors"], meta["dim"])
    cat = np.fromfile(d / "vec_cat.u8", dtype=np.uint8)
    q = np.fromfile(d / "vec_queries.f32", dtype=np.float32).reshape(meta["vector_queries"], meta["dim"])
    truth = json.loads((d / "vec_truth.json").read_text())
    return v, cat, q, truth


if __name__ == "__main__":
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--size", choices=SIZES, default="small")
    ap.add_argument("--data-dir", type=Path, default=None)
    ap.add_argument("--docs", type=int, help="override the number of text documents")
    ap.add_argument("--vectors", type=int, help="override the number of vectors")
    a = ap.parse_args()
    print(ensure(a.size, a.data_dir, a.docs, a.vectors))
