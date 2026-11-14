# SlopenSearch — Go implementation

Standard library only (`net/http`, `encoding/json`), no third-party modules.
Everything is in `package main`. State is durable by default (stored in `./data`);
`SLOP_IN_MEMORY=true` keeps it in memory only (see [Persistence](#persistence)).

## Loop

```bash
make go-test ARGS="-m aggs"                       # rebuild image, restart, run one area
make go-test ARGS=tests/test_41_aggs_bucket.py    # one file
make go-test ARGS="-k terms_order"                # matching tests only
make go-score                                     # pass rate per test file
```

Without Docker (needs Go 1.25 and a Python venv with `requirements.txt`):

```bash
make go-dev                                       # serves :9291
OS_PORT=9291 .venv/bin/pytest -m aggs
```

To see what OpenSearch really answers in a test, without running OpenSearch:
`make ref-show T=<part of the test name>` (or add `TB=short` to a test run: a failing
test then prints the reference exchange). For requests no test makes, start the
reference with `make ref-up` and `curl localhost:9290/...`.

## Layout

| File | Area |
|---|---|
| `server.go` | HTTP entry point, routing, ordered JSON rendering |
| `model.go` | indices, documents, node/cluster constants |
| `meta.go` | index pattern resolution, cluster/meta routes |
| `admin.go` | index create/delete/get, settings |
| `maps.go`, `fields.go` | mappings, index-time field extraction |
| `alias.go`, `templates.go`, `datastreams.go`, `rollover.go` | aliases, index templates, data streams, rollover |
| `resize.go`, `reindex.go`, `resolve_index.go`, `maintenance.go` | clone/shrink/split, `_reindex`, `_resolve/index`, refresh/flush/forcemerge |
| `documents_v2.go`, `bulk_v2.go`, `byquery.go`, `termvectors.go` | document CRUD, `_bulk`, by-query APIs, term vectors |
| `search.go`, `eval.go`, `eval_extra.go`, `highlight.go`, `explain.go`, `validate.go` | search API, query evaluation, highlighting, `_explain`, `_validate` |
| `scroll.go`, `pit.go`, `msearch.go`, `search_templates.go`, `rank_eval.go` | scroll, point in time, `_msearch`, search templates, `_rank_eval` |
| `search_pipelines.go`, `search_shards.go`, `shard_routing.go`, `field_caps.go` | search pipelines, `_search_shards`, routing, `_field_caps` |
| `aggs.go`, `aggs_bucket.go`, `aggs_metric.go`, `aggs_significant.go`, `aggs_extra.go`, `suggest.go` | aggregations, suggesters |
| `ingest.go`, `pipeline.go`, `ingest_processors.go`, `ingest_extra.go` | ingest pipelines and processors |
| `persist.go`, `persist_sync_*.go` | durable state: snapshot + synced journal (`SLOP_DATA_DIR`, `SLOP_IN_MEMORY`) |
| `cat.go`, `nodes.go`, `resp.go` | `_cat`, `_nodes`/`_cluster`, response formats (`filter_path`, YAML, ...) |
| `pl_lex.go`, `pl_parse.go`, `pl_check.go`, `pl_interp.go` | Painless: lexer, parser, static type checks, interpreter |
| `pl_value.go`, `pl_lib.go`, `pl_time.go`, `pl_regex.go` | Painless runtime values (Java semantics), class library, `java.time`, regex limit |
| `pl_api.go`, `script_ctx.go`, `script_meta.go` | script contexts, `_scripts/painless/_execute`, script errors, wiring into search/aggs/update/ingest |
| `pl_expr.go`, `mustache.go` | Lucene expressions, mustache templates |
| `painless.go`, `scripts.go`, `search_templates.go` | script queries, stored scripts, `script_fields`, search templates |
| `analysis.go`, `analysis_chain.go`, `analyze_api.go` | analyzers, analysis chains (index/global/inline components, settings validation), the `_analyze` API with `explain` |
| `tokenizers.go`, `filters.go`, `filters_pattern.go`, `synonyms.go`, `charfilter.go`, `asciifold_table.go` | tokenizers, token filters, synonym filters, char filters with Lucene offset correction |
| `jregex.go` | backtracking `java.util.regex` subset (lookaround, possessive, `\p{..}`) for pattern components |

## Persistence

Two modes, chosen by environment variables:

| Variable | Default | Effect |
|---|---|---|
| `SLOP_DATA_DIR` | `data` (relative to the working directory) | where state is stored; created if missing, loaded if present |
| `SLOP_IN_MEMORY` | `false` | `true`: nothing is read or written on disk, state is lost on exit |

```bash
PORT=9291 ./slopensearch                                 # durable, ./data
SLOP_DATA_DIR=/var/tmp/slop PORT=9291 ./slopensearch     # durable, other directory
SLOP_IN_MEMORY=true PORT=9291 ./slopensearch             # in memory only
```

The startup line says which mode is active. The Docker image stores state in
`/var/lib/slopensearch/data`; `compose.yaml` mounts the named volume `go-data` there, so
data survives `docker compose restart go` (remove it with `docker compose down -v`).

Durable: indices (settings incl. dynamic updates, mappings, aliases, open/closed state,
UUID), documents with `_version`, `_seq_no`, `_primary_term` and routing, index/component/
legacy templates, ingest and search pipelines, stored scripts, data streams and persistent
cluster settings. Not durable, as in OpenSearch: transient cluster settings, scroll and
PIT contexts, task results. Documents that were acknowledged but not refreshed are
searchable after a restart (recovery refreshes).

Every request that may change state is followed by a diff of the in-memory state against
what was already stored; the difference is appended to `journal-<gen>` as one CRC-checked
frame and made durable (`fdatasync`) before the response is sent, so acknowledged writes
survive `kill -9`. Concurrent requests share one sync (group commit), and the journal
file is zero-filled ahead of the frames so a sync needs no metadata update.
On start (and when the journal outgrows the snapshot) the full state is written to
`snapshot` (temp file, fsync, rename) and a new journal generation begins. One process
per directory (`flock` on `<dir>/lock`). Code: `persist.go`.

Cost: one sync per state-changing request, about 2-3 ms on a consumer NVMe disk, so
sequential admin-heavy test files run several times slower than in memory (e.g.
`test_11_index_admin.py` ~5 s in memory, ~25 s on ext4); on tmpfs the overhead is small.
OpenSearch pays the same price, which is why the reference container keeps its data on
tmpfs. Use `SLOP_IN_MEMORY=true` (or a tmpfs data dir) for fast test loops.

`_nodes` reports the real PID as `process.id`; when the server runs as PID 1 (in a
container) it reports a start-time-derived number instead, so restarts stay observable.

Running the restart suite natively:

```bash
cd servers/go && go build -o /tmp/slop-9291 . && cd ../..
export OS_RESTART_CMD="SLOP_DATA_DIR=/tmp/slop-data PORT=9291 servers/restart.sh --signal KILL 9291 /tmp/slop-9291"
sh -c "$OS_RESTART_CMD"   # first start
OS_PORT=9291 .venv/bin/pytest tests/test_95_persistence.py
```

With Docker: `OS_RESTART_CMD="docker compose restart go" OS_PORT=9291 .venv/bin/pytest -m persistence`.

## Rules

- The test suite is the spec. Match OpenSearch byte for byte where tests check it:
  status codes, `error.type`, key names and order.
- Don't edit `tests/` to make a test pass. If you believe a test is wrong, prove it with
  `make ref-test ARGS=<test>` against the reference.
- Go-only changes stay in `servers/go/`.
