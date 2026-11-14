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

`src/persist.rs` keeps an atomic snapshot (`rust-state.json`) plus a write-ahead journal
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

## Layout

| File | Area |
|---|---|
| `main.rs` | entry point, request abstraction (`Req`), routing |
| `store.rs`, `persist.rs` | indices, documents, node/cluster constants; optional on-disk snapshot + journal |
| `shared.rs`, `errors.rs`, `resp.rs` | name validation and index resolution, error shapes, response formats (`filter_path`, YAML, ...) |
| `meta.rs`, `cluster.rs`, `nodes.rs`, `cat.rs` | `_cluster`, `_nodes`, `_tasks`, `_remote`, `_cat` |
| `admin.rs`, `fields.rs` | index create/delete/get, settings, mappings, index-time field extraction |
| `alias.rs`, `templates.rs`, `datastreams.rs`, `rollover.rs` | aliases, index templates, data streams, rollover |
| `resize.rs`, `reindex.rs`, `resolve_index.rs`, `maintenance.rs` | clone/shrink/split, `_reindex`, `_resolve/index`, refresh/flush/forcemerge |
| `docs.rs` | document CRUD, `_mget`, `_bulk`, by-query APIs |
| `search.rs`, `eval.rs`, `eval_extra.rs`, `highlight.rs`, `suggest.rs` | search API, query evaluation, highlighting, suggesters |
| `aggs.rs`, `hdr.rs` | aggregations; HDR histogram for percentiles |
| `ingest.rs`, `ingest_script.rs`, `hashes.rs` | ingest pipelines and processors, their script subset, fingerprint hashes |
| `pl_*.rs` | Painless engine (port of the Go server's): lexer, parser, static checks with Painless-exact compile errors, interpreter, Java value semantics, class library, java.time, regex with the limit-factor circuit breaker; `pl_api.rs` holds contexts, error rendering, the `doc` lookup and `_scripts/painless/_execute` |
| `painless.rs`, `painless_exec.rs`, `scripts.rs`, `render.rs` | older Painless subset used by search/update/aggs, `_execute` entry point, stored scripts, Mustache templates |

## Status

95.2% of the suite passes (see the table in the [main README](../../README.md#status)).
All of the original suite passes except 18 tests in text analysis (`test_51`) and
languages (`test_80`); the open work is in the newer files: suggesters (`test_50`),
the HTTP layer and async tasks (`test_62`), k-NN (`test_72`) and two concurrency tests
(`test_61`). `make rust-test ARGS=tests/test_62_tasks_http.py` is a good place
to start. The build still has many warnings; cleaning them up is welcome.

## Rules

- The test suite is the spec. Match OpenSearch byte for byte where tests check it:
  status codes, `error.type`, key names and order.
- Don't edit `tests/` to make a test pass. If you believe a test is wrong, prove it with
  `make ref-test ARGS=<test>` against the reference.
- Rust-only changes stay in `servers/rust/`.
