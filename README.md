<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/logo/logo-dark.svg">
    <img src="docs/logo/logo-light.svg" alt="SlopenSearch" width="480">
  </picture>
</p>

<p align="center"><b>An experimental, OpenSearch-compatible search server.</b><br>
The goal: same REST API, same responses, same errors — faster and leaner, in a single small binary.</p>

---

> [!NOTE]
> SlopenSearch is under active development and not production-ready yet: part of the API
> is still missing (see [Status](#status)), there is a single node only, and there is no
> authentication or TLS so far. Until those land, keep it on localhost or a trusted network.
> Planned work is listed in [ROADMAP.md](ROADMAP.md).

SlopenSearch aims to speak the OpenSearch 3.9 REST API closely enough that an existing
client does not notice the difference. There are two independent implementations, one in
**Go** and one in **Rust**, and both are measured against the same black-box test suite.

Two things are measured, and both matter:

- **Compatibility.** The suite *is* the specification: about 7,100 tests, each verified
  against a real OpenSearch 3.9.0, pinning down status codes, error types, response shapes
  and quirks.
- **Performance.** Standard [opensearch-benchmark](https://github.com/opensearch-project/opensearch-benchmark)
  workloads run against the reference OpenSearch and both servers in containers with the
  same CPU and memory limits. Throughput, latency and memory relative to the reference
  are the scoreboard; passing the suite is the baseline, being faster than OpenSearch is
  the goal (see [Performance](#performance)).

## Status

Share of the suite passing (2026-11-15, `make go-score` / `make rust-score`):

| Area | Go | Rust |
|---|---:|---:|
| Cluster, nodes, `_cat` | 100% | 100% |
| Index administration | 100% | 100% |
| Documents, bulk, by-query | 100% | 100% |
| Query DSL (full-text, term-level, compound, geo) | 100% | 100% |
| Search API (sort, paging, highlight, scroll, PIT) | 100% | 100% |
| Mappings and field types | 100% | 100% |
| Aggregations | 100% | 100% |
| Text analysis, languages | 100% | 100% |
| Ingest pipelines | 100% | 100% |
| Scripting (Painless, search templates) | 100% | 100% |
| Suggesters | 100% | 100% |
| Concurrency and load | 100% | 100% |
| HTTP layer, async tasks | 100% | 100% |
| Vector search (k-NN) | 100% | 100% |
| Parity details (number text, routing, regex, formats, time zones, geohash) | 100% | 100% |
| Persistence (opt-in `test_95`) | 100% | 100% |
| **Total** | **100%** | **100%** |

Both servers pass the whole suite (7,089 tests). The multi-node and cross-DC suites (`test_96`, `test_97`) are not implemented:
both servers are single-node (see [ROADMAP.md](ROADMAP.md)).
Both servers store their state on disk by default (`SLOP_DATA_DIR`, default `./data`; in Docker a named volume per server) and survive restarts and crashes; `SLOP_IN_MEMORY=true` keeps everything in memory instead (see [servers/go/README.md](servers/go/README.md#persistence) and [servers/rust/README.md](servers/rust/README.md)).

## Performance

The suite says *whether* a server answers like OpenSearch; the benchmarks say *how well*.
Every run is recorded in [`bench/history.jsonl`](bench/history.jsonl) as ratios against the
reference (`> 1` = better than OpenSearch), so each commit can be compared with the last.

Rust on the opensearch-benchmark **geonames** workload, `sample` preset (1% of the corpus,
113k documents, 10 iterations per operation, 1 client), 4 CPUs and 8 GB per container,
median service time in ms (OpenSearch: reference run of 2026-11-15, same configuration).
`401b8ff` is main before the 2026-11-19 scripting work; bold = at least as fast as OpenSearch.
Single runs on a desktop host: differences under ~30% on the millisecond operations are noise.

| Operation | OpenSearch | Rust, 401b8ff | Rust, now | now vs OpenSearch |
|---|---:|---:|---:|---:|
| `index-append` | 1,681 | 6,226 | 4,255 | 0.40× |
| `index-stats` | 11.6 | 1.4 | 3.7 | **3.09×** |
| `node-stats` | 6.8 | 3.1 | 5.4 | **1.24×** |
| `match-all` | 6.6 | 4.1 | 3.0 | **2.15×** |
| `term` | 6.0 | 0.4 | 0.7 | **8.74×** |
| `phrase` | 5.1 | 0.7 | 1.5 | **3.42×** |
| `country_agg_uncached` | 9.3 | 19.3 | 18.9 | 0.49× |
| `country_agg_cached` | 5.5 | 0.5 | 1.8 | **3.13×** |
| `scroll` | 280 | 1,265 | 1,109 | 0.25× |
| `expression` | 10.1 | 1,129 | 135 | 0.07× |
| `painless_static` | 19.7 | 4,068 | 196 | 0.10× |
| `painless_dynamic` | 10.1 | 4,096 | 200 | 0.05× |
| `decay_geo_gauss_function_score` | 26.9 | 576 | 67.3 | 0.40× |
| `decay_geo_gauss_script_score` | 10.4 | 3,617 | 224 | 0.05× |
| `field_value_function_score` | 6.4 | 383 | 46.4 | 0.14× |
| `field_value_script_score` | 6.9 | 2,715 | 119 | 0.06× |
| `large_terms` | 53.6 | 89.5 | 83.9 | 0.64× |
| `large_filtered_terms` | 57.4 | 104 | 96.2 | 0.60× |
| `large_prohibited_terms` | 51.5 | 82.8 | 94.9 | 0.54× |
| `desc_sort_population` | 7.1 | 8.0 | 6.8 | **1.05×** |
| `asc_sort_population` | 4.8 | 6.9 | 6.0 | 0.80× |
| `asc_sort_with_after_population` | 4.8 | 5.9 | 6.2 | 0.77× |
| `desc_sort_geonameid` | 11.1 | 6.6 | 6.5 | **1.72×** |
| `desc_sort_with_after_geonameid` | 6.7 | 5.9 | 6.7 | **1.00×** |
| `asc_sort_geonameid` | 5.0 | 6.7 | 6.2 | 0.81× |
| `asc_sort_with_after_geonameid` | 7.4 | 6.3 | 6.3 | **1.19×** |
| `numeric-term-cardinality-agg-high` | 268 | 762 | 623 | 0.43× |

Where Rust is still behind: Painless and expression scores (an interpreter per document), the
indexing path, scroll, size-0 aggregations over every document and cardinality. The Go server
does not have the index-backed engine yet and is slower across the board.

```bash
make bench-osb-setup                                   # once: opensearch-benchmark venv
make bench-quick                                       # minutes: Go + Rust vs the cached reference
make bench-osb WORKLOAD=geonames PRESET=sample TARGETS=ref,go,rust
make bench-history                                     # ratios commit by commit
make profile PORT=9292 SECS=20                         # CPU profile of a running Rust server
```

How the benchmarks work, the scorecard format and profiling: [bench/README.md](bench/README.md).
How the engine executes queries from the index: [docs/engine-design.md](docs/engine-design.md).
Working on speed: [CONTRIBUTING.md](CONTRIBUTING.md#making-it-faster-performance-work).

## Quick start

Only Docker is needed.

```bash
make go-up                 # Go server on http://localhost:9291
curl localhost:9291
make go-test               # run the whole suite against it
make down                  # stop everything
```

Same for Rust (`make rust-up`, `:9292`) and for the reference OpenSearch
(`make ref-up`, `:9290`). `make help` lists all targets.

## Working on one implementation

The Go and Rust servers share nothing but the test suite. A Go developer never needs a
Rust toolchain and vice versa — the build happens inside Docker.

| | Go | Rust |
|---|---|---|
| Code | [`servers/go`](servers/go/README.md) | [`servers/rust`](servers/rust/README.md) |
| Run the suite | `make go-test` | `make rust-test` |
| One area | `make go-test ARGS="-m aggs"` | `make rust-test ARGS="-m aggs"` |
| One file | `make go-test ARGS=tests/test_41_aggs_bucket.py` | `make rust-test ARGS=tests/test_41_aggs_bucket.py` |
| Matching tests | `make go-test ARGS="-k terms_order"` | `make rust-test ARGS="-k terms_order"` |
| Pass rate per file | `make go-score` | `make rust-score` |
| Native, no Docker | `make go-dev` | `make rust-dev` |

`*-test` rebuilds the image, restarts the server and runs pytest in a container, so
every run sees your latest code. Test files run in 4 parallel workers; set `JOBS=0` to
run sequentially. Failures are reported one line each; add `TB=long` for full
tracebacks when debugging a few tests (`make go-test TB=long ARGS="-k terms_order"`). Reports from `*-score` are
kept in `.score/<server>/`.

The loop: pick a red area, read what the tests expect and what the real OpenSearch
answered (`make ref-show T=<test name>`), make it green, check nothing else broke.
You don't need to run OpenSearch for this — see [Reference answers](#reference-answers).

## The test suite

Each file has a pytest marker, so an area can be run on its own (`ARGS="-m <marker>"`):

| Marker | File | What it covers |
|---|---|---|
| `cluster` | `test_01_cluster.py` | root, health, state, settings, nodes, tasks, `_cat/*` |
| `admin` | `test_11_index_admin.py` | index lifecycle, settings, aliases, templates, rollover, data streams, reindex |
| `documents` | `test_21_documents.py` | CRUD, versioning, routing, `_mget`, `_update`, `_bulk`, by-query APIs |
| `documents` | `test_23_nonstandard_json.py` | `NaN`/`Infinity` tokens in `_bulk`, `_update` and ingest bodies |
| `ingest` | `test_22_ingest.py` | ingest pipelines and processors, search pipelines |
| `fulltext` | `test_31_fulltext.py` | match family, multi_match, query_string, intervals, span queries |
| `queries` | `test_32_queries.py` | term-level, bool, function_score, nested, join, geo, percolate |
| `search_api` | `test_33_search_api.py` | sort, paging, scroll, PIT, highlight, collapse, rescore, msearch, templates |
| `parity` | `test_34_parity_errors.py` | mustache compile errors, control-character escaping, `field_sort` parse positions, date math rounding |
| `parity` | `test_35_parity_numbers_routing.py` | Murmur3 shard routing; Java `Double`/`Float.toString` text in aggs, sorts, fields, scores |
| `parity` | `test_36_parity_regex_formats.py` | `java.util.regex` in Painless and ingest, grok (Oniguruma), CBOR and YAML bodies and responses |
| `parity` | `test_37_parity_time_hash.py` | IANA time zones and DST in aggs, range queries, ingest; fingerprint hashes; Java base64 |
| `parity` | `test_38_parity_ingest_geo.py` | ingest mustache templates, geohash parsing and grids |
| `parity` | `test_39_parity_*.py` | cases found by the benchmark and by the Rust port: scheduled refresh, by-query conflicts, float precision, Kahan sums |
| `aggs` | `test_41_aggs_bucket.py`, `test_42_aggs_metric.py` | bucket, metric and pipeline aggregations |
| `scripting` | `test_43_scripting.py` | Painless, expressions, stored scripts, mustache |
| — | `test_50_suggester.py` | term, phrase and completion suggesters (contexts, fuzzy, collate, errors) |
| `analysis` | `test_51_analysis.py` | analyzers, tokenizers, token and char filters |
| `concurrency` | `test_61_concurrency.py` | parallel writes, optimistic concurrency, races with admin calls, alias swaps, scroll/PIT snapshots, load |
| `http` | `test_62_tasks_http.py` | async tasks (`wait_for_completion=false`, `_tasks`, `_rethrottle`, cancel), compression, content types, URL encoding, query-string and header handling |
| `documents` | `test_63_refresh_visibility.py` | what search, count, scroll, PIT and by-query APIs see before a refresh |
| `types` | `test_71_field_types.py` | field types and mapping parameters |
| `knn` | `test_72_knn.py` | `knn_vector` mappings, knn query (filters, radial, nested), `knn_score` and painless vector functions |
| `i18n` | `test_80_languages.py` | ~60 scripts, language analyzers, Unicode corner cases |
| `parity`, `mapping`, `smoke` | `test_00`–`test_90` | the original smaller suites |

Opt-in suites that need more than one server process:

| Marker | File | How to run |
|---|---|---|
| `persistence` | `test_95_persistence.py` | `OS_RESTART_CMD="docker compose restart go" OS_PORT=9291 pytest -m persistence` |
| `multinode`, `crossdc` | `test_96_multinode.py`, `test_97_cross_dc.py` | `servers/opensearch-cluster.sh up` — see the script header |

### Adding a test

A test may only encode what real OpenSearch does. Write it, prove it against the
reference, and re-record the reference answers of that file:

```bash
make ref-test ARGS=tests/test_41_aggs_bucket.py
make ref-record ARGS=tests/test_41_aggs_bucket.py
```

This is the one task that needs the reference OpenSearch running (Docker, ~2 GB RAM).

Use unique index names (the `index_factory` fixture does this), clean up anything
cluster-wide you create, and don't depend on timing or test order.

## Reference answers

Every HTTP exchange the suite has with the real OpenSearch 3.9.0 is recorded in
`tests/reference/<test file>.jsonl`: request, body, status code and response, per test.
Working on a server never needs a running OpenSearch:

```bash
make ref-show T=terms_order                          # tests whose name contains the text
make ref-show T=test_41_aggs_bucket.py::test_terms_order_by_count
make go-test TB=short ARGS="-k terms_order"          # failures print the reference exchange
```

The files are plain JSON lines, so `grep` works on them too. Index names carry random
suffixes and differ from your run; long bodies are cut at 8000 characters.

When tests change, the recording is refreshed against the real OpenSearch with
`make ref-record` (all files, about a minute) or `make ref-record ARGS=tests/<file>.py`.

## Reference OpenSearch

`compose.yaml` runs the official `opensearchproject/opensearch:3.9.0` image — the full
distribution with all bundled plugins — with the security plugin disabled, because the
suite talks plain HTTP, and its data kept in RAM (~1.5 GB image, ~2 GB RAM). You need it
only to add or change tests, or to try requests no test makes. More than 4 test workers
overload its cluster manager and requests start timing out.

## Layout

```
compose.yaml           reference OpenSearch, both servers, the test runner
Makefile               all developer commands (make help)
servers/go/            Go implementation
servers/rust/          Rust implementation
servers/*.sh           helpers: restart a server, run a local multi-node topology
tests/                 the compatibility suite (pytest + httpx)
tests/reference/       recorded answers of the real OpenSearch, per test
bench/                 benchmarks: OSB workloads and own harness, scorecard, history.jsonl
docs/engine-design.md  index-backed query execution (inverted index, columns, top-k)
scripts/score.py       pass rate per test file
scripts/ref.py         show and re-record the reference answers
scripts/profile.sh     CPU profile of a running Rust server (make profile)
docs/logo/             logo and icon (SVG)
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Security issues: [SECURITY.md](SECURITY.md).

## Ports

| Port | Service | Override in `.env` |
|---|---|---|
| 9290 | reference OpenSearch | `OPENSEARCH_PORT` |
| 9291 | SlopenSearch (Go) | `GO_PORT` |
| 9292 | SlopenSearch (Rust) | `RUST_PORT` |

The servers themselves listen on `127.0.0.1:9200` by default; set `HOST` and `PORT` to
change that (the Docker images use `HOST=0.0.0.0`, published on localhost only).

## License

[Apache License 2.0](LICENSE).

SlopenSearch is an independent project. It is not affiliated with, endorsed by or
sponsored by the OpenSearch Project or the Linux Foundation. OpenSearch is a trademark
of LF Projects, LLC; the name is used here only to describe API compatibility.
