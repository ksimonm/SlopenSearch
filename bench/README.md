# Benchmarks

Speed, memory and CPU of the reference OpenSearch 3.9.0, the Go server and the Rust server
on the same synthetic data, each alone in a Docker container with the same limits.

```bash
.venv/bin/pip install -r bench/requirements.txt         # numpy (data generation, k-NN ground truth)
make bench                                              # ref,go,rust on the small data set
make bench SIZE=medium TARGETS=go,rust ARGS="--workloads index,search --cpus 8 --memory 16g"
.venv/bin/python bench/run.py --targets ref,go-mem,rust-mem --size large --queries filter_term_range,sort_price
.venv/bin/python bench/report.py bench/results/<timestamp>   # re-render a report
```

Needs Docker, the image `opensearchproject/opensearch:3.9.0` and Python 3.11+ on the host.
The Go and Rust images are built from this checkout as `slopbench-go:latest` /
`slopbench-rust:latest` (`--no-build` reuses them). Results land in
`bench/results/<timestamp>/`: `run.json` (parameters, host, image ids), one `<target>.json`
per target (every number, all latency percentiles, resource samples per workload,
correctness digests) and `report.md`. `bench/results/sample/` is a committed example (a
`tiny` run made while developing the harness, on a busy machine); the
latest full report is [docs/benchmarks.md](../docs/benchmarks.md).

## Targets

| name | image | port | storage |
|---|---|---|---|
| `ref` | opensearch 3.9.0, single node, security plugin off, heap `--heap` (4g) | 9590 | volume `bench-ref-data` |
| `go` | Go server, persistence on (default) | 9591 | volume `bench-go-data` |
| `rust` | Rust server, persistence on (default) | 9592 | volume `bench-rust-data` |
| `go-mem` | Go server, `SLOP_IN_MEMORY=true` | 9593 | none |
| `rust-mem` | Rust server, `SLOP_IN_MEMORY=true` | 9594 | none |

Targets run one after another, never at the same time. Every target gets a fresh
container `bench-<name>` and an empty named volume (Docker's disk, not tmpfs, so fsyncs are
real), `--cpus 4 --memory 8g --memory-swap 8g` by default (`--cpus`, `--memory`, `--heap`,
or `BENCH_CPUS`/`BENCH_MEMORY`/`BENCH_HEAP`). The container and its volume are removed at
the end (`--keep` keeps them). The developer stack of `compose.yaml` (ports 9290-9292) is
never touched.

CPU and memory come from the container's cgroup (v2), sampled every 250 ms:
CPU in cores (1.0 = one core busy; the limit is `--cpus`), memory = `memory.current -
inactive_file` (what `docker stats` shows; includes the JVM heap and active page cache),
plus the cgroup's `memory.peak` over the whole run and its OOM-kill counter.

## Data sets

`bench/datagen.py` writes seeded, reproducible data to `/tmp/agent-bench/data/<size>-...`
(`--data-dir` or `BENCH_DATA_DIR`) once and reuses it.

| size | text docs | NDJSON | vectors (128-dim float) |
|---|---:|---:|---:|
| tiny | 10 k | 7 MB | 5 k |
| small | 100 k | 73 MB | 20 k |
| medium | 500 k | 360 MB | 100 k |
| large | 2 M | 1.45 GB | 500 k |

`--docs N` / `--vectors N` override the counts (any scale: the generator streams, 8 processes).

- **Text documents**: `title` (3-10 words), `body` (log-normal length, median ~100 words),
  words drawn from a 50 000-word pseudo vocabulary with a Zipf distribution (s = 1.07), so
  there are stop-word-like terms in almost every document and long-tail terms in a few;
  `tags` (1-4 of 300 keywords, Zipf), `category` (20 keywords), `user_id` (20 000
  keywords), `seq` (unique long, used for stable sorting and `search_after`), `price`,
  `quantity`, `rating`, `created` (2015-2025), `location` (geo_point).
- **Vectors**: 64 Gaussian clusters, a category `cat` (0-9) for filtered k-NN, 200 query
  vectors and their exact top-100 L2 neighbours (brute force, with and without the
  category filter) for recall.

## Workloads (`--workloads`, default all)

| name | what it does |
|---|---|
| `index` (always) | create `bench-text` (explicit mapping, 1 shard, 0 replicas) and bulk-load the whole corpus: `--clients` (8) processes, `--batch` (1000) docs per request; then `_refresh`. Same for the vectors (`knn_vector`, hnsw/faiss/l2) if `knn` is selected. Disk size afterwards (`du` of the data directory, after `_flush` on ref). |
| `sweep` | bulk throughput on a separate index of up to 50-100 k docs for batch × clients = 100×4, 1000×1, 1000×4, 1000×8, 5000×4 |
| `verify` | correctness: 10 queries per search workload, sequential, `track_total_hits: true`; records total hits, returned ids, aggregation results; k-NN recall@10 against the exact neighbours |
| `search` | each query type in a closed loop with `--search-clients` (4) for N seconds (by size, `--search-duration`): `match` (mid-frequency term), `match_frequent` (stop-word-like term), `match_and` (two terms, operator and), `multi_match` (title^2, body), `bool` (must match + term + range filters + must_not), `filter_term_range` (term + date range, no scoring), `geo_distance`, `sort_price`, `sort_date`, `agg_terms` (terms on tags), `agg_date_histogram` (monthly + avg sub-agg), `agg_metrics` (avg/max/sum over a range), `highlight`, `deep_from` (from 5000), `search_after` (10 pages of 100, sorted by `seq`). `--queries a,b` picks some. |
| `knn` | k-NN queries (k = 10) with and without a term filter |
| `mixed` | `--mixed-clients` (8): 80 % searches (match, bool, filter, terms agg, sort) and 20 % bulk writes of 10 new docs, concurrently; read and write latencies separately |
| `update` | partial `_update` of single docs, single `DELETE`s, bulk partial updates (batch 1000), `_delete_by_query` (one user each); then the doc count |
| `restart` | persistent targets: graceful `docker stop`, `docker start`; time until HTTP answers, until the text index answers `_count`, doc count before/after, first query latency, memory after restart |

Every workload reports ops/s, latency p50/p95/p99/max (ms), errors and status codes, and
CPU average/peak and memory average/peak of the container during the workload.

### Slow targets

The SlopenSearch servers evaluate queries by scanning documents (there is no inverted
index), so full-text queries get linearly slower with the corpus. To keep a run bounded,
every timed search workload starts with one sequential *probe* request: if it takes more
than `--slow-limit` (10 s) the timed run is skipped and the report shows `too slow` with the
probe time. Probes and verify requests time out after 3 × `--slow-limit`; as the servers do
not cancel abandoned queries, the harness then waits (up to `--request-timeout`, 120 s)
until the server answers again. Indexing stops after `--index-timeout` (1800 s), each
update/delete workload after `--update-timeout` (300 s); the report flags incomplete steps.

## Caveats

- One machine runs both the load generator and the target. The client processes are cheap
  (pre-encoded bodies, one keep-alive connection each) but share the host CPUs; limits are
  applied to the target only. On a busy host absolute numbers are noisy — compare targets
  of the same run, and look at `loadavg_start/end` in `run.json`.
- Closed loop: throughput and latency are coupled (a slow server simply gets fewer
  requests). Latencies are client-side and include HTTP and JSON encoding.
- Durations are short (seconds per query type); the JVM of the reference is warmed only
  by the indexing and the probe request. Run longer (`--search-duration`) for stable numbers.
- Score ties: different engines may order equally scored hits differently, so the top-id
  overlap of full-text queries can be below 100 % without a bug. Hit totals and
  aggregations must match exactly.
- The reference indexes with `refresh_interval` 1 s (default) and one shard; the
  SlopenSearch servers have no segments, so `_refresh` is near free for them.
- Reference k-NN is approximate (HNSW), the SlopenSearch servers compute exact distances;
  recall shows the difference.
