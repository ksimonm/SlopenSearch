# Benchmarks: reference OpenSearch vs the Go and Rust servers

First results of the harness in [bench/](../bench/README.md), 2026-11-15.

**Read with care: the machine was not idle.** Several other agents ran test suites at the
same time (load average 3.5-16.5 on 16 CPUs during the runs), so absolute numbers are
noisy and the reference OpenSearch is probably understated (it is the only target that
uses all 4 of its CPUs). Rerun on a quiet machine with `make bench SIZE=...`.

- Host: Intel i7-11850H (8 cores / 16 threads), 91 GB RAM, NVMe, Linux 7.0, Docker.
- Each target alone in its own container, one after another, `--cpus 4 --memory 8g`
  (no swap), data on a Docker volume (real disk). Reference: OpenSearch 3.9.0 single
  node, security off, heap 4 GB. Go/Rust: images built from the servers of main `7c83c2a` (unchanged
  on this branch), persistence on.
- Clients on the same host: 8 bulk clients × 1000 docs, 4 search clients, 8 mixed clients.

## Runs

1. **large, scaled down to 1 M documents (0.73 GB NDJSON) and 200 k vectors** — the
   planned 2 M docs / 500 k vectors were not attempted after it was clear that the
   SlopenSearch servers do not fit 1 M docs into 8 GB, and that their search cost grows
   linearly with the corpus (minutes per query at 2 M docs), so most search workloads
   would not have finished in the ~45 min budget. Only selected search workloads were
   run (`--queries filter_term_range,sort_price,agg_date_histogram,agg_metrics,geo_distance,search_after,match`).
2. **small: 100 k documents (73 MB), 20 k vectors**, same workload selection, so that
   all three targets survive indexing and can be compared on search, updates and restart.

A `tiny` run with every workload (10 k docs) is in
[bench/results/sample/report.md](../bench/results/sample/report.md).

## Main results

| | ref | Go | Rust |
|---|---:|---:|---:|
| **1 M docs fit in 8 GB?** | yes (4.8 GB peak, mostly the 4 GB heap) | **no, OOM-killed after ~888 k docs** | **no, OOM-killed after ~257 k docs** |
| memory after indexing 100 k docs + 20 k vectors | 4,680 MiB (heap 4 GB preallocated) | 1,244 MiB | 2,280 MiB |
| memory peak, 100 k run | 4,810 MiB | 2,755 MiB | 8,084 MiB (!) |
| bulk indexing 100 k docs (docs/s) | 5,682 | 3,576 | 7,648 |
| bulk indexing 1 M docs (docs/s) | 12,344 | ~2,000 until OOM | ~2,000 until OOM |
| CPU used while indexing (cores of 4) | 3.97 | 1.97 | 0.97 |
| on-disk size, 100 k docs + 20 k vectors | 106 MiB | 98 MiB | 186 MiB |
| term + date-range filter, 100 k (ops/s, p50) | 368 /s, 7 ms | 4.9 /s, 819 ms | 1.9 /s, 2,089 ms |
| `match` full text, 100 k (one query) | 11 ms p50 at 320 /s | 15.6 s | 12.3 s |
| k-NN k=10 over 20 k vectors | 430 /s, 6 ms | 4.8 /s, 706 ms | one query 11.2 s |
| single-doc `_update`, 100 k (ops/s, p50) | 437 /s, 13 ms | 21 /s, 342 ms | 284 /s, 28 ms |
| restart until data available, 100 k | 48 s | 3.9 s | 13.8 s (+ 61 s stop) |
| first query after restart | 127 ms | 11.3 s | 7.1 s |
| correctness (hit totals, aggs, k-NN recall) | reference | all equal | all equal except float rounding in date_histogram avg and 2 docs fewer removed by `_delete_by_query` |

## Findings

- **No inverted index.** Both SlopenSearch servers evaluate every query by scanning all
  documents. A `match` query costs ~0.1-0.15 ms *per document* (so ~12-16 s at 100 k docs,
  minutes at 1 M), vs ~2 ms total in OpenSearch. Even filters (term + range, geo_distance)
  are 75-200× slower than the reference at 100 k docs and scale linearly. This is the
  dominant bottleneck; everything else is secondary.
- **Memory blow-up.** Go needs ~9.6 KB of RAM per 0.73 KB source document (13×), Rust
  ~33 KB (45×; it reached 6.5-8 GB with only 100 k docs + 20 k vectors during searches).
  With an 8 GB limit Go dies at ~0.9 M and Rust at ~0.26 M documents. Both are
  OOM-killed without any back-pressure (no circuit breaker / 429), and the bulk clients
  see connection resets.
- **Rust is single-threaded.** One `Arc<Mutex<Store>>` serializes all requests: CPU never
  exceeds 1.0 core of the 4 available, during indexing and search alike; with 4 search
  clients every request waits for the others (latency ≈ 4 × service time).
- **Go writes get slower with the index size.** The journal diffs the whole in-memory
  state after every state-changing request: single `_update` went from 320/s (10 k docs)
  to 21/s (100 k docs), and bulk throughput falls from ~12 k docs/s (10 k) to ~2 k docs/s
  near 1 M docs. Go also uses only ~2 of 4 cores while indexing.
- **Rust ignores SIGTERM** (PID 1 in the container, no signal handler): `docker stop`
  always waits the full timeout and then kills it (60 s here). Data survived (the
  journal is fsynced), but shutdowns are slow and never graceful.
- **Cold first query after restart**: Go 11 s and Rust 7 s for the first `match` (vs
  their ~12-16 s steady state — i.e. no extra warm-up cost, simply the scan), the
  reference 127 ms. Restart itself is fast for the SlopenSearch servers (4-14 s vs 48 s
  JVM start + recovery).
- **Correctness held up**: hit totals, sorted ids, aggregations, `search_after` pages and
  k-NN results agree with the reference in every comparison (k-NN is exact in both
  SlopenSearch servers: recall 1.0; the reference HNSW reached 0.992 at 200 k vectors).
  Two discrepancies in Rust: `avg` inside `date_histogram` differs in the 4th digit
  (likely averaging the double source values instead of the float-indexed values; Go
  matches exactly), and after 2,000 single deletes, `_delete_by_query` on 5 users removed
  19 documents where the reference and Go removed 21 (all requests returned 200; not yet
  investigated).
- `agg_date_histogram` on Go is as fast as the reference (271/s vs 236/s): the repeated
  identical `size: 0` bodies hit Go's request cache, as they do OpenSearch's.

## What to fix in the servers (in order of impact)

1. Inverted index (term → postings per field) and doc values / per-field columnar arrays
   for filters, sorts and aggregations, instead of scanning `_source`-like structures.
2. Compact document storage (keep `_source` as raw bytes, not a parsed tree; intern
   field names; store vectors as `[]float32`), and a memory circuit breaker that rejects
   bulk requests with 429 before the OOM killer strikes.
3. Rust: replace the global store mutex with per-index `RwLock`s so searches run in
   parallel; handle SIGTERM (or run with `tini`).
4. Go: make journal writes proportional to the change, not to the state (append the
   operation instead of diffing the full state).
5. Cancel queries when the client disconnects (both servers keep computing abandoned
   queries; the harness has to wait for them).

The two generated reports follow unchanged.

---

## Run 1: large, scaled to 1 M docs (2026-11-15)

- size **large**: 1,000,000 text documents (0.73 GB NDJSON), 200,000 vectors × 128 dims
- every target alone in Docker with `--cpus 4 --memory 8g` (no swap); reference OpenSearch heap 4g
- clients: bulk/update 8 (batch 1000), search 4, mixed 8; 12 s per search workload
- host `p7760`: 11th Gen Intel(R) Core(TM) i7-11850H @ 2.50GHz, 16 CPUs, 91 GB RAM, kernel 7.0.0-43-generic; load average at start 16.5, at end 9.5
- git `aa282ec`; targets: ref (3.9.0), go (3.9.0), rust (3.9.0)

### Indexing

| | ref | go | rust |
|---|---:|---:|---:|
| startup, empty (s) | 29.31 | 0.32 | 0.44 |
| text bulk (docs/s) | 12,344 | 2,012 | 2,009 |
| text bulk p50 / p99 per batch (ms) | 547.41 / 2079.23 | 1508.34 / 18981.46 | 1.11 / 13890.18 |
| text docs indexed | 1,000,000 | 888,000 (incomplete) | 257,000 (incomplete) |
| refresh after bulk (s) | 6.09 | - | - |
| vector bulk (docs/s) | 11,619 | - | - |
| vector refresh (s) | 14.25 | - | - |
| CPU during text bulk (avg / peak cores) | 3.87 / 4.76 | 1.53 / 4.35 | 0.96 / 1.02 |
| memory peak during text bulk (MiB) | 4,708 | 8,172 | 8,162 |
| memory idle after indexing (MiB) | 4,809 | - | - |
| memory peak, whole run (MiB) | 4,988 | 8,172 | 8,162 |
| on-disk size after indexing (MiB) | 1,736 | - | - |
| OOM-killed | no | **yes** | **yes** |

### Bulk batch size / client sweep (docs/s, separate index)

| batch × clients | ref | go | rust |
|---|---:|---:|---:|
| 100 × 4 | 17,849 | - | - |
| 1000 × 1 | 11,349 | - | - |
| 1000 × 4 | 30,291 | - | - |
| 1000 × 8 | 28,685 | - | - |
| 5000 × 4 | 26,908 | - | - |

### Search throughput (ops/s) and latency p50 / p95 / p99 (ms)

4 concurrent clients, closed loop. `!n` = n failed requests; *too slow* = a single probe request took longer than 10 s, so the timed run was skipped (the probe time is shown). Correctness is in its own section.

| workload | ref ops/s | ref p50/p95/p99 | go ops/s | go p50/p95/p99 | rust ops/s | rust p50/p95/p99 |
|---|---:|---:|---:|---:|---:|---:|
| filter_term_range | 1,222.0 | 2.32 / 4.53 / 32.21 | - | - | - | - |
| sort_price | 1,182.0 | 3.06 / 4.6 / 8.44 | - | - | - | - |
| agg_date_histogram | 3,043.9 | 1.09 / 1.84 / 3.19 | - | - | - | - |
| agg_metrics | 5,867.3 | 0.57 / 0.8 / 1.11 | - | - | - | - |
| geo_distance | 1,471.0 | 2.42 / 4.25 / 6.68 | - | - | - | - |
| match | 1,236.2 | 2.01 / 8.21 / 11.55 | - | - | - | - |
| search_after | 129.7 | 25.11 / 43.89 / 56.96 | - | - | - | - |
| knn | 785.9 | 3.58 / 11.59 / 38.06 | - | - | - | - |
| knn_filter | 572.5 | 6.46 / 10.96 / 17.38 | - | - | - | - |

### CPU (avg cores) and memory peak (MiB) during search

| workload | ref | go | rust |
|---|---:|---:|---:|
| filter_term_range | 3.88 / 4,919 | - / - | - / - |
| sort_price | 3.71 / 4,921 | - / - | - / - |
| agg_date_histogram | 3.63 / 4,927 | - / - | - / - |
| agg_metrics | 2.89 / 4,928 | - / - | - / - |
| geo_distance | 3.49 / 4,928 | - / - | - / - |
| match | 2.29 / 4,928 | - / - | - / - |
| search_after | 2.56 / 4,928 | - / - | - / - |
| knn | 3.83 / 4,937 | - / - | - / - |
| knn_filter | 3.54 / 4,937 | - / - | - / - |

### Mixed load, updates and deletes

mixed: 8 clients, 80 % searches / 20 % bulk writes of 10 docs. Updates/deletes: 8 clients, single-document requests; bulk_update in batches of 1000 (docs/s); delete_by_query sequential, one user each.

| workload | ref ops/s | ref p50/p99 | ref CPU/MiB | go ops/s | go p50/p99 | go CPU/MiB | rust ops/s | rust p50/p99 | rust CPU/MiB |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| mixed_rw (read) | 807.6 | 2.87 / 56.12 | 3.96 / 4,968 | - | - | - | - | - | - |
| mixed_rw (write) | 204.9 | 9.93 / 72.63 | 3.96 / 4,968 | - | - | - | - | - | - |
| update_single | 856.2 | 8.11 / 13.84 | 1.27 / 4,971 | - | - | - | - | - | - |
| delete_single | 940.6 | 8.04 / 16.03 | 0.61 / 4,975 | - | - | - | - | - | - |
| bulk_update | 11,227 docs/s | 590.15 / 1442.34 | 3.92 / 4,988 | - | - | - | - | - | - |
| delete_by_query | 10.1 | 98.84 / 105.98 | 1.04 / 4,986 | - | - | - | - | - | - |

### Restart (persistent targets)

Graceful `docker stop`, then `docker start`; *data available* = until `_count` on the text index answers.

| | ref | go | rust |
|---|---:|---:|---:|
| on-disk size (MiB) | 1,141 | - | - |
| stop (s) | 1.64 | - | - |
| start until HTTP ready (s) | 35.62 | - | - |
| start until data available (s) | 35.68 | - | - |
| first query after restart (ms) | 55.8 | - | - |
| docs after restart / before | 989764 / 989764 | - | - |
| memory after restart (MiB) | 4,594 | - | - |

### Correctness (compared with the first target)

Sequential pass over 10 queries per workload with `track_total_hits: true`. *totals* = queries whose hit count equals ref's; *top ids* = mean overlap of the returned ids with ref's (score ties may legitimately reorder); *aggs* = identical aggregation results (floats to 4 digits). k-NN: recall@10 against exact brute-force neighbours.

| workload | go | rust |
|---|---|---|
| filter_term_range | - | - |
| sort_price | - | - |
| agg_date_histogram | - | - |
| agg_metrics | - | - |
| geo_distance | - | - |
| match | - | - |
| search_after | - | - |
| knn | - | - |
| knn_filter | - | - |

| | ref | go | rust |
|---|---:|---:|---:|
| recall@10 knn | 0.992 | - | - |
| recall@10 knn_filter | 1.000 | - | - |
| count text_after_index | 1000000 | - | - |
| count vec_after_index | 200000 | - | - |
| count removed_by_update_workload | 10236 | - | - |
| count rating_4_5_after_updates | 30510 | - | - |

### Errors and anomalies

- **go** container died (OOMKilled=True, exit 137)
- **go** step index: `RuntimeError: GET /bench-text/_count -> 0: [Errno 111] Connection refused`
- **go** index_text: first failed request: `bulk: HTTP 0: [Errno 104] Connection reset by peer`
- **go** refresh_text: `RuntimeError: POST /bench-text/_refresh -> 0: [Errno 111] Connection refused`
- **go** skipped: sweep, verify, search, knn, mixed, update, restart
- **rust** container died (OOMKilled=True, exit 137)
- **rust** step index: `RuntimeError: GET /bench-text/_count -> 0: [Errno 111] Connection refused`
- **rust** index_text: first failed request: `bulk: HTTP 0: [Errno 104] Connection reset by peer`
- **rust** refresh_text: `RuntimeError: POST /bench-text/_refresh -> 0: [Errno 111] Connection refused`
- **rust** skipped: sweep, verify, search, knn, mixed, update, restart


---

## Run 2: small, 100 k docs (2026-11-15)

- size **small**: 100,000 text documents (0.07 GB NDJSON), 20,000 vectors × 128 dims
- every target alone in Docker with `--cpus 4 --memory 8g` (no swap); reference OpenSearch heap 4g
- clients: bulk/update 8 (batch 1000), search 4, mixed 8; 8 s per search workload
- host `p7760`: 11th Gen Intel(R) Core(TM) i7-11850H @ 2.50GHz, 16 CPUs, 91 GB RAM, kernel 7.0.0-43-generic; load average at start 7.6, at end 3.5
- git `aa282ec`; targets: ref (3.9.0), go (3.9.0), rust (3.9.0)

### Indexing

| | ref | go | rust |
|---|---:|---:|---:|
| startup, empty (s) | 41.07 | 0.45 | 0.34 |
| text bulk (docs/s) | 5,682 | 3,576 | 7,648 |
| text bulk p50 / p99 per batch (ms) | 1282.26 / 3199.55 | 1886.79 / 5167.25 | 702.2 / 3465.9 |
| text docs indexed | 100,000 | 100,000 | 100,000 |
| refresh after bulk (s) | 0.44 | 0.01 | 0.38 |
| vector bulk (docs/s) | 4,095 | 2,587 | 3,145 |
| vector refresh (s) | 0.12 | 0.01 | 0.25 |
| CPU during text bulk (avg / peak cores) | 3.97 / 4.66 | 1.97 / 4.00 | 0.97 / 1.00 |
| memory peak during text bulk (MiB) | 4,635 | 1,283 | 2,260 |
| memory idle after indexing (MiB) | 4,680 | 1,244 | 2,280 |
| memory peak, whole run (MiB) | 4,810 | 2,755 | 8,084 |
| on-disk size after indexing (MiB) | 106 | 98 | 186 |
| OOM-killed | no | no | no |

### Bulk batch size / client sweep (docs/s, separate index)

| batch × clients | ref | go | rust |
|---|---:|---:|---:|
| 100 × 4 | 7,120 | 1,718 | 2,846 |
| 1000 × 1 | 7,214 | 3,241 | 5,165 |
| 1000 × 4 | 14,560 | 5,825 | 5,726 |
| 1000 × 8 | 11,084 | 4,252 | 6,782 |
| 5000 × 4 | 13,052 | 15,428 | 4,905 |

### Search throughput (ops/s) and latency p50 / p95 / p99 (ms)

4 concurrent clients, closed loop. `!n` = n failed requests; *too slow* = a single probe request took longer than 10 s, so the timed run was skipped (the probe time is shown). Correctness is in its own section.

| workload | ref ops/s | ref p50/p95/p99 | go ops/s | go p50/p95/p99 | rust ops/s | rust p50/p95/p99 |
|---|---:|---:|---:|---:|---:|---:|
| filter_term_range | 368.4 | 7.31 / 38.68 / 48.22 | 4.9 | 819.32 / 1239.02 / 1392.5 | 1.9 | 2088.61 / 2443.63 / 2443.63 |
| sort_price | 222.1 | 15.05 / 38.0 / 44.68 | 3.4 | 1069.82 / 2132.88 / 2181.88 | 0.2 | 25699.48 / 29413.7 / 29413.7 |
| agg_date_histogram | 235.5 | 13.84 / 35.25 / 54.52 | 271.1 | 2.51 / 8.11 / 600.02 | 0.9 | 3388.53 / 8286.38 / 8286.38 |
| agg_metrics | 354.1 | 7.98 / 25.08 / 45.0 | 1.7 | 2045.23 / 4822.09 / 4822.09 | too slow | 1 query: 23.2 s |
| geo_distance | 220.7 | 16.8 / 33.42 / 42.83 | 6.9 | 504.49 / 1233.44 / 1261.76 | 1.1 | 3371.01 / 4695.38 / 4695.38 |
| match | 319.6 | 10.96 / 25.1 / 32.59 | too slow | 1 query: 15.6 s | too slow | 1 query: 12.3 s |
| search_after | 93.7 | 31.54 / 56.13 / 85.24 | 5.3 | 633.0 / 1132.74 / 1755.87 | 0.7 | 5483.02 / 5536.8 / 5604.62 |
| knn | 429.8 | 5.81 / 18.96 / 27.77 | 4.8 | 706.13 / 1378.42 / 1412.19 | too slow | 1 query: 11.2 s |
| knn_filter | 1,001.7 | 3.61 / 6.34 / 10.41 | 7.8 | 452.98 / 797.61 / 877.36 | too slow | 1 query: 10.9 s |

### CPU (avg cores) and memory peak (MiB) during search

| workload | ref | go | rust |
|---|---:|---:|---:|
| filter_term_range | 3.88 / 4,746 | 3.14 / 2,129 | 0.99 / 6,497 |
| sort_price | 3.29 / 4,747 | 2.28 / 2,237 | 0.99 / 6,482 |
| agg_date_histogram | 3.28 / 4,767 | 1.95 / 2,517 | 0.99 / 6,517 |
| agg_metrics | 3.28 / 4,774 | 3.05 / 2,628 | - / - |
| geo_distance | 2.99 / 4,775 | 3.79 / 2,632 | 1.00 / 7,085 |
| match | 2.37 / 4,776 | - / - | - / - |
| search_after | 2.44 / 4,778 | 3.46 / 2,727 | 1.00 / 7,085 |
| knn | 2.73 / 4,781 | 3.68 / 2,755 | - / - |
| knn_filter | 3.54 / 4,782 | 3.80 / 2,752 | - / - |

### Mixed load, updates and deletes

mixed: 8 clients, 80 % searches / 20 % bulk writes of 10 docs. Updates/deletes: 8 clients, single-document requests; bulk_update in batches of 1000 (docs/s); delete_by_query sequential, one user each.

| workload | ref ops/s | ref p50/p99 | ref CPU/MiB | go ops/s | go p50/p99 | go CPU/MiB | rust ops/s | rust p50/p99 | rust CPU/MiB |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| mixed_rw (read) | 288.5 | 7.75 / 77.89 | 4.00 / 4,797 | too slow | 1 query: 12.1 s | - | 0.1 | 37749.22 / 56789.49 | 1.00 / 7,028 |
| mixed_rw (write) | 70.7 | 23.59 / 249.67 | 4.00 / 4,797 | too slow | 1 query: 12.1 s | - | - | - | - |
| update_single | 437.4 | 12.52 / 49.73 | 3.74 / 4,802 | 20.9 | 342.03 / 946.46 | 1.00 / 2,680 | 283.7 | 27.99 / 33.12 | 0.36 / 6,711 |
| delete_single | 646.5 | 11.81 / 30.2 | 3.42 / 4,804 | 26.2 | 239.81 / 948.79 | 1.04 / 2,681 | 315.0 | 25.01 / 29.99 | 0.28 / 6,710 |
| bulk_update | 3,129 docs/s | 2457.19 / 3646.15 | 3.92 / 4,809 | 15,910 docs/s | 529.24 / 653.71 | 1.47 / 1,922 | 6,002 docs/s | 785.13 / 4094.17 | 0.95 / 6,848 |
| delete_by_query | 3.4 | 185.55 / 744.4 | 3.32 / 4,810 | 5.9 | 168.27 / 178.44 | 0.93 / 1,940 | 3.5 | 272.42 / 318.84 | 0.86 / 6,746 |

### Restart (persistent targets)

Graceful `docker stop`, then `docker start`; *data available* = until `_count` on the text index answers.

| | ref | go | rust |
|---|---:|---:|---:|
| on-disk size (MiB) | 246 | 209 | 241 |
| stop (s) | 2.67 | 0.38 | 60.85 |
| start until HTTP ready (s) | 48.12 | 3.88 | 13.76 |
| start until data available (s) | 48.28 | 3.92 | 13.82 |
| first query after restart (ms) | 127.2 | 11288.3 | 7067.8 |
| docs after restart / before | 97979 / 97979 | 97979 / 97979 | 97981 / 97981 |
| memory after restart (MiB) | 4,618 | 2,639 | 2,982 |

### Correctness (compared with the first target)

Sequential pass over 10 queries per workload with `track_total_hits: true`. *totals* = queries whose hit count equals ref's; *top ids* = mean overlap of the returned ids with ref's (score ties may legitimately reorder); *aggs* = identical aggregation results (floats to 4 digits). k-NN: recall@10 against exact brute-force neighbours.

| workload | go | rust |
|---|---|---|
| filter_term_range | totals 10/10, top ids 47% | totals 10/10, top ids 47% |
| sort_price | totals 10/10, top ids 100% | totals 4/4, top ids 100% |
| agg_date_histogram | totals 10/10, aggs 10/10 | totals 10/10, aggs 3/10, 10/10 within 0.1 % |
| agg_metrics | totals 10/10, aggs 10/10 | totals 1/1, aggs 1/1 |
| geo_distance | totals 10/10, top ids 61% | totals 10/10, top ids 59% |
| match | totals 1/1, top ids 100% | totals 2/2, top ids 95% |
| search_after | top ids 100% | top ids 100% |
| knn | top ids 100% | top ids 100% |
| knn_filter | top ids 100% | top ids 100% |

| | ref | go | rust |
|---|---:|---:|---:|
| recall@10 knn | 1.000 | 1.000 | 1.000 |
| recall@10 knn_filter | 1.000 | 1.000 | 1.000 |
| count text_after_index | 100000 | 100000 | 100000 |
| count vec_after_index | 20000 | 20000 | 20000 |
| count removed_by_update_workload | 2021 | 2021 | 2019 |
| count rating_4_5_after_updates | 1194 | 1194 | 1194 |

