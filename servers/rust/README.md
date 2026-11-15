# SlopenSearch — Rust implementation

`axum` + `tokio` + `serde_json` (plus `regex` and `base64`); the server keeps all data
in memory behind one `Arc<Mutex<Store>>` and, by default, persists it to a data
directory (see [Persistence](#persistence)). Much of the query and aggregation code is a port
of the Go server, which remains a useful second reading of the expected behaviour.

## Loop

```bash
make rust-test ARGS="-m aggs"                       # rebuild image, restart, run one area
make rust-test ARGS=tests/test_41_aggs_bucket.py    # one file
make rust-test ARGS="-k terms_order"                # matching tests only
make rust-score                                     # pass rate per test file
```

Without Docker (needs Rust stable and a Python venv with `requirements.txt`):

```bash
make rust-dev                                       # serves :9292
OS_PORT=9292 .venv/bin/pytest -m aggs
```

To see what OpenSearch really answers in a test, without running OpenSearch:
`make ref-show T=<part of the test name>` (or add `TB=short` to a test run: a failing
test then prints the reference exchange). For requests no test makes, start the
reference with `make ref-up` and `curl localhost:9290/...`.

## Persistence

Persistence is on by default: state goes to `./data` (relative to the working
directory; `make rust-dev` therefore uses `servers/rust/data`), which is created when
missing and loaded when it already holds state.

| Setting | Default | Meaning |
|---|---|---|
| `SLOP_DATA_DIR` (or `--data-dir <dir>`, `-Epath.data=<dir>`) | `data` | data directory |
| `SLOP_IN_MEMORY` (or `--in-memory`) | `false` | `true`: keep everything in memory only, nothing is read or written |

```bash
PORT=9292 slopensearch                                    # persists to ./data
SLOP_DATA_DIR=/var/lib/slop PORT=9292 slopensearch        # another directory
SLOP_IN_MEMORY=true PORT=9292 slopensearch                # in-memory, state dies with the process
```

The Docker image uses `SLOP_DATA_DIR=/data`; `compose.yaml` mounts the named volume
`rust-data` there, so the state survives `docker compose restart rust` and rebuilds
(`docker compose down -v` drops it).

`src/storage/` keeps an atomic snapshot (`rust-state.json`) plus a write-ahead journal
(`rust-journal-<gen>.log`): every request that changed durable state appends one fsynced
journal line before its response is sent, so acknowledged writes survive `kill -9`. On
startup the snapshot is loaded, the journal replayed and compacted into a new snapshot.
`rust.lock` stops a second Rust server from using the same directory (the `rust-` prefix
lets a Go server share it). Durable: indices (settings, mappings, aliases, closed state,
documents with version, seq_no, routing and exact `_source`), templates, data streams,
ingest/search pipelines, stored scripts, persistent cluster settings. Not durable, as in
OpenSearch: transient cluster settings, scrolls and PITs; unrefreshed documents are
searchable after recovery.

`tests/test_95_persistence.py` exercises this (with a crash restart):

```bash
CMD="servers/restart.sh --signal KILL 9292 env PORT=9292 SLOP_DATA_DIR=/tmp/slop-data target/release/slopensearch"
OS_RESTART_CMD="$CMD" OS_PORT=9292 .venv/bin/pytest tests/test_95_persistence.py
# against the compose service: OS_RESTART_CMD='docker compose restart rust' OS_PORT=9292
```

(`restart.sh` starts the server if nothing listens on the port yet; the binary path is
relative to the repository root here, adjust it to your build.)

## Status

99.99% of the suite passes (see the table in the [main README](../../README.md#status)).
The open work is one `test_72_knn` test (a literal `NaN` in a vector). HTTP compression
(gzip/deflate/snappy) uses the `flate2` and `snap` crates. The build has a few warnings left (unused values and methods); cleaning them up is welcome.

## Rules

- The test suite is the spec. Match OpenSearch byte for byte where tests check it:
  status codes, `error.type`, key names and order.
- Don't edit `tests/` to make a test pass. If you believe a test is wrong, prove it with
  `make ref-test ARGS=<test>` against the reference.
- Rust-only changes stay in `servers/rust/`.

## Layout

One crate, layered like the Go server. `main.rs` only reads the configuration from the
environment and wires storage and the HTTP server; everything else lives in modules
under `src/`. Inner modules never `use` outer ones:

```
main ─► http ─► suggest ─► search ─► mapping ─► model ─► settings
            ├─► aggs ───►┘    │         │
            ├─► ingest ─► scripting ◄───┤  (search, ingest, storage)
            └─► storage ─► mapping, model, scripting
       (all of them may use the leaf libraries: errors, resp, jsonv, dates,
        analysis, painless, mustache, jregex, ...)
```

Checked with `grep -rhoE 'crate::[a-z_]+' src/<module>`: no module outside `src/http`
names `crate::http`; `search` does not name `aggs`, `suggest`, `ingest` or `storage`;
`mapping` does not name `search`; `model` only names `settings`; `painless` and
`mustache` name nothing in the crate.

Domain and engine modules:

| Module | Contents | Depends on (crate) |
|---|---|---|
| `model/` | the domain state: `Store` (indices, cluster settings, templates, data streams, pipelines, stored scripts, scroll/PIT contexts), `Index` with its accessors, node identity constants, uuids, shard routing; `visibility.rs`: what searches see between refreshes and the request cache | settings |
| `mapping/` | mappings and field types (`mod.rs`: validation, merging, dynamic type inference, `mapping_field_def`, alias resolution); `fields.rs`: index-time parsing of documents into doc values; `knn.rs`/`knn_index.rs`: knn_vector mappings and values; `geo.rs`: geo points and shapes; `completion.rs`: completion field mappings and inputs | analysis, dates, errors, jsonv, model, resp, settings |
| `scripting.rs` | the script service: stored-script registry, script specs, size and language checks | errors |
| `search/` | `eval.rs`/`eval_extra.rs`: query evaluation and scoring (`QueryCtx`); `analyzed.rs`: query-time analysis; `highlight.rs`/`hl_analyzed.rs`; `knn_query.rs`; `fetch.rs` (doc values, `fields`, script fields), `sort.rs` (sort specs, `search_after`), `source.rs` (`_source` filtering), `inner_hits.rs` (inner hits, collapse), `request.rs` (named queries, rescore, terms lookup), `validate.rs` (shard-level query validation); script glue: `script_ctx.rs` (prepared scripts, shard failures), `script_env.rs`, `painless_doc.rs` (the `doc` variable, score functions), `expressions.rs` (Lucene expressions, bound to `QueryCtx`) | analysis, dates, errors, jsonv, mapping, model, painless, resp, scripting, settings |
| `aggs/` | bucket, metric and pipeline aggregations; `hdr.rs`: HDR histogram for percentiles | search, mapping, model, ... |
| `suggest/` | term (`mod.rs`), phrase with collate (`phrase.rs`) and completion (`completion.rs`) suggesters | search, mapping, model, mustache, ... |
| `ingest/` | ingest pipelines: validation, processors, `on_failure`, `_simulate`, search pipelines; `script.rs`: the script subset of ingest processors | errors, hashes, model, resp, scripting, settings |
| `storage/` | durable state: snapshot + synced journal of the `Store` (`storage::open`, `after_request`) | mapping, model, scripting |
| `http/` | the HTTP adapter (see below) | everything above |

Leaf libraries (no server state):

| Module | Contents |
|---|---|
| `errors.rs` | `ApiError`, error and JSON response builders |
| `resp.rs` | response rendering: pretty JSON, `filter_path`, YAML, number formatting, `glob_match` |
| `jsonv.rs` | JSON value helpers (`to_f64`, `value_to_string`, dotted paths), exponent literal preservation, duplicate key detection |
| `settings.rs` | index settings lookup (`find_setting`, `setting_int`, ...) |
| `dates.rs` | date parsing, civil calendar arithmetic, Java date format patterns |
| `analysis/` | tokenizers, token/char filters, analyzers and analysis chains from index settings; `basic.rs`: the simple built-in analysis (lowercasing, CJK bigrams, fuzziness) |
| `lucene/` | Lucene analysis algorithm ports: stemmers, normalizers, case mapping, UAX#29 tokenizer, Thai, generated tables |
| `snowball/` | the Snowball runtime and the generated stemmers |
| `jregex.rs`, `uchar.rs` | backtracking `java.util.regex` subset; Java `Character` classification |
| `painless/` | Painless: lexer, parser, static checks with Painless-exact compile errors, interpreter, Java value semantics, class library, java.time, regex with the limit-factor circuit breaker; `api.rs`: script contexts, compilation, error rendering, `run_with_stack` |
| `mustache.rs` | mustache templates (mustache.java semantics) |
| `hashes.rs` | md5/sha1/sha256/sha3 for the fingerprint processor |

Files of `http/`:

| File | Area |
|---|---|
| `mod.rs`, `compression.rs`, `net_io.rs` | `Req`, request parsing, routing (`dispatch`), response post-processing, `method_not_allowed`, body compression, the TCP listener |
| `shared.rs` | name validation, index pattern resolution, settings views and mapping validation |
| `meta.rs`, `cluster.rs`, `nodes.rs`, `cat.rs`, `tasks_async.rs`, `human.rs` | `_cluster`, `_nodes`, `_tasks`, `_remote`, `_cat`, `human=true` |
| `admin.rs` | index create/delete/get, settings, mappings |
| `alias.rs`, `templates.rs`, `datastreams.rs`, `rollover.rs` | aliases, index templates, data streams, rollover |
| `resize.rs`, `reindex.rs`, `resolve_index.rs`, `maintenance.rs` | clone/shrink/split, `_reindex`, `_resolve/index`, refresh/flush/forcemerge |
| `docs.rs` | document CRUD, `_mget`, `_bulk`, by-query APIs, `_count`, term vectors |
| `search.rs`, `search_templates.rs` | `_search` request handling and response assembly (`run_search`), scroll, PIT, `_field_caps`, `_validate`, `_rank_eval`, `_search_shards`; `_msearch`, search templates, `_render` |
| `ingest_api.rs`, `knn_api.rs`, `analyze_api.rs` | `_ingest` and `_search/pipeline` routes, `_plugins/_knn`, `_analyze` |
| `scripts.rs`, `script_meta.rs`, `painless_exec.rs` | stored scripts, script contexts/languages listing, `_scripts/painless/_execute` |
