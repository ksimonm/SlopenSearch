# SlopenSearch — Go implementation

Built on the standard library (`net/http`, `encoding/json`) so far; third-party modules
are welcome where they save real work (see [CONTRIBUTING.md](../../CONTRIBUTING.md)).
The server is a layered set of packages under `internal/` with a small `main.go` (see
[Layout](#layout)). State is durable by default (stored in `./data`);
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

Unit tests of the `internal/` packages (painless, expressions, mustache, jregex,
analysis, dates, mapping, respfmt, jsonv, ojson and pure search/aggs helpers) run
natively in a few seconds: `make go-unit` (or `cd servers/go && go test ./...`). Their
expected values come from `tests/reference` answers; the black-box suite stays the spec.

To see what OpenSearch really answers in a test, without running OpenSearch:
`make ref-show T=<part of the test name>` (or add `TB=short` to a test run: a failing
test then prints the reference exchange). For requests no test makes, start the
reference with `make ref-up` and `curl localhost:9290/...`.

## Layout

The code is layered: `package main` (`main.go`) only reads the configuration from the
environment and wires the server; everything else lives under `internal/`. Packages
never import `package main`, and inner layers never import outer ones:

```
main ─► httpapi ─► suggest ─► search ─► mapping ─► model
               ├─► aggs ───►┘           │           └─► settings
               ├─► ingest ─► model      └─► analysis, dates
               └─► storage ─► model
         (all of them may use the leaf libraries: apierr, jsonv, ojson, painless, ...)
```

Domain and engine packages:

| Package | Contents | Depends on (internal) |
|---|---|---|
| `internal/model` | the domain state: `State` (indices, cluster settings, templates, data streams, pipelines, stored scripts), `Index` (metadata, settings, mappings, documents, versions, seq_nos, routing, pending/stale writes) with its accessors, `DataStream`, node identity constants | settings |
| `internal/dates` | date parsing and formatting as OpenSearch does it: Java format names and patterns, epoch values, ISO detection, range date math | jsonv, zone |
| `internal/mapping` | mappings and field types: validation, merging, dotted names, dynamic mappings and templates, alias resolution (`FieldDef`), index-time parsing of documents into doc values (`ParseContext`, `ComputeDocValues`), knn_vector/completion/geo field values | analysis, apierr, dates, geohash, jsonv, model, ojson, settings, wildcard |
| `internal/search` | query evaluation (`QueryCtx`, `EvalQuery`), scoring, query_string, inner hits, percolate, function_score, k-NN queries, highlighting, source filtering, sort specs and hit sorting (`SortHits`), `search_after`, collapse, rescore, shard-level query validation, profile; the painless/expressions glue for script queries, scores, fields and sorts. Other indices and stored scripts are reached through `search.Cluster` (`ClusterLookup`, set by the server) | analysis, apierr, dates, expressions, jsonv, mapping, model, mustache, ojson, painless, settings |
| `internal/aggs` | bucket, metric and pipeline aggregations, scripted aggregations (`aggs.Evaluate` over `aggs.Hit`) | search, mapping, model, ... |
| `internal/suggest` | term, phrase (with collate) and completion suggesters (`suggest.Evaluate`) | search, mapping, model, ... |
| `internal/ingest` | ingest pipelines: validation, processors, `on_failure`, `_simulate`, default/final pipelines (`ingest.Engine`); template properties render through `internal/mustache` on the document's template model (`templates.go`); script processors compile through the `ingest.Scripts` interface | apierr, jsonv, model, mustache, ojson, painless, settings, wildcard |
| `internal/storage` | durable state: snapshot + synced journal of a `model.State` (`storage.Open`, `Store.Persist`); the server's locks are passed in (`storage.Locks`) | model |
| `internal/httpapi` | the HTTP adapter: `Server` (embeds `model.State`, owns the locks, scroll/PIT contexts, tasks), routing, request parsing, all REST handlers, response and error rendering, `_cat`, `_nodes`, tasks, the request cache | everything above |

Leaf libraries (no server state):

| Package | Contents | Depends on |
|---|---|---|
| `internal/ojson` | ordered JSON objects (`Object`, `KV`), `Verbatim`, `Marshal` | - |
| `internal/jsonv` | decoded JSON value helpers: `ToFloat`, `ToInt64`, `Int`, `ToString`, `ScalarString`, `Float` (Java-style doubles), `NormalizeNumbers`, `FormatNumber`, `EncodeString`, `DeepCopy`, `SortedKeys`, `Unmarshal` | - |
| `internal/wildcard` | `*` pattern matching | - |
| `internal/geohash` | geohashes: encoding, cell bounds, the cell-corner point a geohash stands for, `LongEncode` (grid bucket order), neighbors | - |
| `internal/settings` | index settings lookup (`Find`, `Int`, `ValueString`, `ValueInt`), setting key patterns (`MatchAny`) | jsonv, wildcard |
| `internal/apierr` | the API error type (`Error`, `Response` for pre-rendered bodies), common errors (`BadRequest`, `Parsing`, `IndexNotFound`) | - |
| `internal/zone` | `java.time.ZoneId.of` (Java's ID rules and error texts) with tz rules from the embedded tz database: offsets, valid offsets of a local time, gaps, transitions | apierr |
| `internal/jbase64` | `java.util.Base64.getDecoder()` semantics and error messages | - |
| `internal/jregex` | backtracking `java.util.regex` subset (lookaround, possessive, `\p{..}`) | - |
| `internal/painless` | Painless: lexer, parser, type checks, interpreter, values with Java semantics, class library, `java.time`, script contexts and compile cache | - |
| `internal/expressions` | Lucene expressions; doc values, fields and params are passed in (`Env`, `Doc`) | painless |
| `internal/mustache` | mustache templates (mustache.java semantics) | painless |
| `internal/snowball` | generated Snowball stemmers and their runtime | - |
| `internal/lucene` | Lucene analysis algorithm ports: stemmers, normalizers, case mapping, ASCII folding, UAX#29 tokenizer, Thai break iterator, generated tables | - |
| `internal/analysis` | tokenizers, token/char filters, synonyms, built-in and language analyzers, analysis chains from index settings (`analysis.Index`) | apierr, jregex, jsonv, lucene, settings, snowball |
| `internal/respfmt` | response rendering: pretty JSON, `filter_path`, `human`, `error_trace`, YAML | jsonv, ojson, wildcard |

Files of `internal/httpapi`:

| File | Area |
|---|---|
| `server.go`, `httpconn.go`, `httplayer.go`, `utf8check.go` | HTTP entry point, routing, error rendering, wire-level quirks |
| `state.go` | `Server`, `NewServer`, the `search.Cluster` adapter |
| `meta.go`, `cluster.go` | index pattern resolution, request helpers, server routes; cluster health, state, stats, settings, allocation explain |
| `admin.go`, `maps_api.go` | index create/delete/get, settings, `_mapping` |
| `alias.go`, `templates.go`, `datastreams.go`, `rollover.go` | aliases, index templates, data streams, rollover |
| `resize.go`, `reindex.go`, `resolve_index.go`, `maintenance.go` | clone/shrink/split, `_reindex`, `_resolve/index`, refresh/flush/forcemerge |
| `documents.go`, `doc_checks.go`, `update.go`, `mget.go` | document index/get/delete, percolator and join document checks, `_update`, `_mget` |
| `bulk.go`, `byquery.go`, `termvectors.go` | `_bulk`, by-query APIs, term vectors |
| `search.go`, `explain.go`, `validate.go`, `field_caps.go`, `source.go` | `_search` request handling and response assembly, `_explain`, `_validate`, `_field_caps` |
| `scroll.go`, `pit.go`, `msearch.go`, `search_templates.go`, `rank_eval.go` | scroll, point in time, `_msearch`, search templates, `_rank_eval` |
| `search_pipelines.go`, `search_shards.go`, `shard_routing.go`, `requestcache.go` | search pipelines, `_search_shards`, shard routing, request cache |
| `ingest_api.go`, `knn_plugin.go`, `analyze_api.go` | `_ingest` routes (and the `ingest.Scripts` adapter), `_plugins/_knn`, `_analyze` |
| `scripts.go`, `script_ctx.go`, `script_meta.go`, `painless.go`, `pl_api.go` | stored scripts, `_scripts/painless/_execute` |
| `storage_api.go` | opening `internal/storage`, which requests are followed by a journal write |
| `cat.go`, `nodes.go`, `tasks*.go` | `_cat`, `_nodes`/`_cluster`, tasks |

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

The startup line says which mode is active. Files are prefixed (`go-snapshot`,
`go-journal-<gen>`, `go.lock`), so the Rust server can use the same directory. The
Docker image stores state in `/data`; `compose.yaml` mounts the named volume `go-data` there, so
data survives `docker compose restart go` (remove it with `docker compose down -v`).

Durable: indices (settings incl. dynamic updates, mappings, aliases, open/closed state,
UUID), documents with `_version`, `_seq_no`, `_primary_term` and routing, index/component/
legacy templates, ingest and search pipelines, stored scripts, data streams and persistent
cluster settings. Not durable, as in OpenSearch: transient cluster settings, scroll and
PIT contexts, task results. Documents that were acknowledged but not refreshed are
searchable after a restart (recovery refreshes).

Every request that may change state is followed by a diff of the in-memory state against
what was already stored; the difference is appended to `go-journal-<gen>` as one CRC-checked
frame and made durable (`fdatasync`) before the response is sent, so acknowledged writes
survive `kill -9`. Concurrent requests share one sync (group commit), and the journal
file is zero-filled ahead of the frames so a sync needs no metadata update.
On start (and when the journal outgrows the snapshot) the full state is written to
`go-snapshot` (temp file, fsync, rename) and a new journal generation begins. One process
per directory (`flock` on `<dir>/go.lock`; a second instance refuses to start). Code: `internal/storage`.

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
