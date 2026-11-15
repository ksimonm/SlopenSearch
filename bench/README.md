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

Needs Docker, the image `opensearchproject/opensearch:3.9.0` and Python 3.11+ on the host
(the Makefile uses `.venv/bin/python`; override with `BENCH_PY=...`). The standard
opensearch-benchmark workloads, the levels `make bench-quick` / `make bench-full`, the
scorecard with ratios vs the reference, the committed history `bench/history.jsonl` and
CPU profiling are described below.
The Go and Rust images are built from this checkout as `slopbench-go:latest` /
`slopbench-rust:latest` (`--no-build` reuses them). Results land in
`bench/results/<timestamp>/`: `run.json` (parameters, host, image ids), one `<target>.json`
per target (every number, all latency percentiles, resource samples per workload,
correctness digests) and `report.md`. `bench/results/sample/` is a committed example (a
`tiny` run made while developing the harness, on a busy machine); the
latest full report is [docs/benchmarks.md](../docs/benchmarks.md).

## Levels, scorecard and history

```bash
make bench-osb-setup     # once: bench/.venv-osb with opensearch-benchmark 2.5.0 (Python 3.11 via uv)
make bench-quick         # ~5 min: Go + Rust; ratios vs the cached reference baseline
make bench-full          # hours: large harness + full OSB geonames and http_logs
make bench-history       # ratios commit by commit (ARGS="--target go --config osb:geonames")
make bench-scorecard DIR=bench/results/<timestamp>   # ratio report of one run
make bench-osb WORKLOAD=geonames PRESET=sample TARGETS=ref,go,rust ARGS="--params ingest_percentage=2"
```

| level | harness (`bench/run.py`) | OSB (`bench/osb.py`) | targets |
|---|---|---|---|
| `quick` | `tiny` (10 k docs): index, 5 search types × 3 s, updates | geonames `--test-mode` (1 k docs, 1 iteration per task) through the recording proxy | go, rust (+ ref only when no baseline) |
| `full` | `large` (2 M docs), every workload | geonames and http_logs with the workloads' own iteration counts and target throughputs | go, rust (+ ref only when no baseline) |

`bench/levels.py` runs a level into `bench/results/<timestamp>/` and, when
`bench/history.jsonl` has no reference record of exactly that configuration, also runs the
reference into `bench/results/<timestamp>-ref/` (`--ref always|never` overrides). Then it
appends every record to `bench/history.jsonl` (committed, one line per run and target) and
writes `scorecard.md` with the ratios. Re-run the reference (`--ref always`) after
changing the limits, the machine or the level parameters (they are part of the
configuration key, so a changed parameter simply finds no baseline).

**Scorecard format** (`bench/scorecard.py`, `scorecard.json` in the results directory and
one line per record in `history.jsonl`): one record per target and configuration, the
same for harness and OSB runs:

- `config`: `harness:<size>:<hash of the parameters>:<cpus>cpu-<memory>` or
  `osb:<workload>:<preset>:<hash>:<cpus>cpu-<memory>`; ratios only compare equal configs
- `run`, `commit`, `date`, `level`, `host` (name, CPU model, CPUs, RAM, kernel),
  `loadavg` (1-minute load at start and end), `limits` (cpus, memory, reference heap),
  `target`, `image`, `version`, `params`, `proxy` (OSB calls went through the proxy)
- `tasks.<name>`: `status` (`ok`, `error` = every request failed, `timeout`, `too_slow`,
  `failed`, `not_run`), `thr` + `unit` (OSB: median throughput; harness: ops/s or docs/s),
  `p50` / `p90` / `p99` latency in ms (harness: p95 instead of p90), `err` (error rate
  0-1), `cpu` (average cores of the cgroup during the task), `mem` (peak bytes during the
  task), `cpu_s_op` (CPU seconds per operation)
- `resources`: `cpu_avg`, `cpu_peak`, `mem_avg`, `mem_peak` (cgroup `memory.peak` over the
  whole run), `disk_bytes` (after indexing); `scalars` (lower is better: startup, restart,
  disk, memory); `oom`; `rejected_calls` (OSB with proxy)

**Ratios** are oriented so that **> 1.0 means the SlopenSearch server is better**:
throughput ours/ref; latency, CPU seconds per operation, memory and disk ref/ours. The
summary is the geometric mean over the tasks both targets completed, plus the count of
tasks that completed (`ours / ref / all`) — a server that fails a task does not get a
ratio for it, so always read the two together.

## opensearch-benchmark (OSB)

`bench/osb.py` runs standard [OSB](https://github.com/opensearch-project/opensearch-benchmark)
workloads with `--pipeline=benchmark-only --target-hosts=127.0.0.1:<port>` against the
same containers as the harness (fresh container and volume per target, equal
`--cpus/--memory`, cgroup sampler). OSB lives in its own venv `bench/.venv-osb` (Python
3.11; its pinned dependencies do not install on Python 3.14) and keeps its workloads,
corpora, logs and test runs under `BENCHMARK_HOME=~/.cache/slopbench` (`.osb/…`, on disk,
not tmpfs): the geonames corpus is 253 MB compressed / 3.5 GB, http_logs 1.2 / 31 GB,
nyc_taxis 4.5 / 74 GB.

| preset | what |
|---|---|
| `test` | OSB `--test-mode`: 1 k documents, 1 warm-up + 1 measured iteration per task. Smoke test and API coverage; the numbers are single requests. |
| `sample` | `ingest_percentage=1` (geonames: 114 k docs), 2 warm-up + 10 iterations per task, `target_throughput=100000` (i.e. closed loop, as fast as one client can go). Feasible for the scanning servers. |
| `full` | the workload's defaults (geonames: 11.4 M docs, up to 1000 iterations, throttled to the workload's target throughputs). |

`--params k=v` adds or overrides workload parameters (JSON values), `--include-tasks` /
`--exclude-tasks` pick query tasks, `--procedure` a test procedure.

How a run survives a server that is slow or rejects calls:

- The schedule comes from `opensearch-benchmark info`; the leading setup block
  (delete/create index, cluster health, bulk, refresh, force-merge) runs as one OSB
  invocation, the query tasks in chunks of `--chunk` (6) via `--include-tasks`, all with
  `--on-error=continue` and a client `timeout` of `--request-timeout` s. A failing request
  shows up as the task's `error_rate`; OSB does not abort.
- A watchdog follows OSB's log (`Choosing … for [task]` lines give the start of every
  task, which also gives the per-task CPU/memory windows). When one task runs longer than
  `--task-timeout` (600 s), OSB is killed, the task is recorded as `timeout`, the server
  gets `--drain-timeout` s to finish the abandoned requests (the SlopenSearch servers do
  not cancel them) or its container is restarted, and the other tasks of the chunk are
  re-run.
- **Client-side compat shim.** OSB aborts the whole run (not a task) when
  `_nodes/stats` lacks `jvm.gc.collectors.*.collection_time_in_millis` or
  `jvm.mem.pools` — which both SlopenSearch servers omit. `bench/slop_osb_compat.py`
  (loaded into the OSB venv through a `.pth` file when `SLOP_OSB_COMPAT=1`, which osb.py
  sets for every target) makes OSB's telemetry treat missing statistics as zero and logs
  `slop_osb_compat:` lines, which osb.py stores as `compat_warnings` per invocation. It
  does not change requests or timings and is a no-op for the reference.
- `--proxy` routes OSB through `bench/proxy.py`, a recording HTTP proxy that stores every
  endpoint (method + path pattern), its query parameters, status codes, and error
  type/reason with an example request — the list of APIs a workload needs and which of
  them a server rejects. It adds latency (Python, one thread per connection), so timed
  runs go direct; `quick` uses it because test-mode numbers are smoke-only anyway.

Results: `osb-<workload>-<target>.json` (tasks with OSB's op metrics, windows, resources,
invocations, calls), `osb-run-<workload>.json` (run metadata), and OSB's own Markdown
results and console log per invocation (`osb-<target>-<label>.md/.log`).

### OSB calls the SlopenSearch servers reject

From geonames and http_logs in test mode through the proxy (2026-11-15, main `3f75a11`);
everything else OSB calls (`GET /`, `_cluster/health` with `wait_for_status` /
`wait_for_no_relocating_shards`, `_nodes/_all`, `_nodes/stats/_all`, `PUT` index with the
workload's settings and mappings, `_bulk`, `_forcemerge`, `_cat/indices`, search with
`scroll` / `sort` / `request_cache`, `_search/scroll`, `DELETE _search/scroll`) is accepted.

GAPS_PLACEHOLDER

## Profiling

`--profile` (bench/run.py and bench/osb.py) attaches `perf record -F 99 -g` to the server
process for the whole target run (not the reference) and writes
`profile-<target>/{perf.data,perf.script,stacks.folded}` plus `flamegraph.svg` when
`inferno-flamegraph` (`cargo install inferno`) or `flamegraph.pl` is on the PATH
(`perf.script` also opens directly in <https://www.speedscope.app>). It works for Docker
targets (via the container's host PID) and best for the native targets, which are built
with symbols and frame pointers:

| target | how | port |
|---|---|---|
| `go-native` | `go build` of servers/go (with `--profile`: unstripped) | 9595 |
| `rust-native` | `cargo build --release` into `servers/rust/target-bench[-prof]` (with `--profile`: `-C force-frame-pointers=yes -g`) | 9596 |

Native targets run under `systemd-run --user --scope -p CPUQuota=<cpus×100>% -p
MemoryMax=<memory> -p MemorySwapMax=0`, i.e. the same limits as the containers, and the
cgroup sampler reads that scope; data goes to `~/.cache/slopbench/native/<target>`.

```bash
sudo sysctl kernel.perf_event_paranoid=1        # Ubuntu defaults to 4 = no perf for users
.venv/bin/python bench/osb.py --targets go-native --preset sample --profile
.venv/bin/python bench/run.py --targets rust-native --size small --workloads index,search --profile
```

Manually, for any running server: `perf record -F 99 -g -p <pid> -- sleep 30`, then
`perf script | inferno-collapse-perf | inferno-flamegraph > flame.svg`. The Go server has
no pprof endpoint; perf works on Go binaries (frame pointers are on by default).

## Targets

| name | image | port | storage |
|---|---|---|---|
| `ref` | opensearch 3.9.0, single node, security plugin off, heap `--heap` (4g) | 9590 | volume `bench-ref-data` |
| `go` | Go server, persistence on (default) | 9591 | volume `bench-go-data` |
| `rust` | Rust server, persistence on (default) | 9592 | volume `bench-rust-data` |
| `go-mem` | Go server, `SLOP_IN_MEMORY=true` | 9593 | none |
| `rust-mem` | Rust server, `SLOP_IN_MEMORY=true` | 9594 | none |
| `go-native` | Go binary on the host in a systemd scope (see Profiling) | 9595 | `~/.cache/slopbench/native/go-native` |
| `rust-native` | Rust binary on the host in a systemd scope | 9596 | `~/.cache/slopbench/native/rust-native` |

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

`bench/datagen.py` writes seeded, reproducible data to `~/.cache/slopbench/data/<size>-...`
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
