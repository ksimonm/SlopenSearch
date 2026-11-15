# Roadmap

Planned work that is bigger than a single fix. Both servers get each item; the test
suite stays the specification, so every item starts with tests verified against a real
OpenSearch 3.9.0.

## Security: authentication and TLS

Today both servers speak plain HTTP and accept every request, so they must stay on
localhost or a trusted network. This is the main blocker for real use.

- TLS on the HTTP port (certificate and key from files, optional client certificates).
- HTTP basic authentication with users and roles, compatible with the request and error
  shapes of the OpenSearch security plugin (`401` with `WWW-Authenticate`, `403`
  `security_exception`), and `_plugins/_security/authinfo`.
- Index- and cluster-level permissions per role.
- Tests: a new opt-in suite run against the reference OpenSearch with the security plugin
  enabled.

## Multiple nodes

Both servers are single-node; `test_96_multinode.py` and `test_97_cross_dc.py` are not
implemented.

- Node discovery and cluster-manager election; cluster state published to all nodes.
- Primary and replica shards, shard allocation and recovery, `number_of_replicas` > 0
  turning the cluster green.
- Request routing and coordination: fan-out search, per-shard failures, write
  replication, `wait_for_active_shards`.
- Cross-cluster search and replication for the cross-DC suite.

## Performance

The benchmark harness is in [bench/](bench/README.md); the first report, from a busy
machine, is [docs/benchmarks.md](docs/benchmarks.md). Ordered by impact:

- **Inverted index and doc values.** Both servers scan every document for every query:
  a `match` over 100k docs takes 12–16 s (reference: 11 ms), term/range filters are
  75–200x slower, k-NN over 20k vectors is 100x slower or worse. Needs a real inverted
  index per field, columnar doc values for filters, sorts and aggregations, and
  float32 vector storage.
- **Memory.** Go holds ~13x the source size in RAM, Rust ~45x; both are OOM-killed
  indexing 1M docs (0.73 GB) in 8 GB. Store raw `_source` bytes compactly, and add a
  memory circuit breaker that answers `429` instead of dying.
- **Concurrency in Rust.** A global store mutex keeps Rust at one core; use per-index
  locks.
- **Write path.** Go's journal diffs the whole state after each write, so single
  updates drop from 320/s at 10k docs to 21/s at 100k; journal the operation instead
  (Rust: in progress). Every write is fsynced before the response; consider an
  OpenSearch-like `index.translog.durability: async` mode.
- **Cancellation.** Cancel a search when the client disconnects.
- **Rust as PID 1** ignores SIGTERM, so `docker stop` waits for the timeout.

## Exact aggregations at scale

- `cardinality` is an exact set; above `precision_threshold` OpenSearch uses HyperLogLog++
  (Murmur3 x64_128, its own bias tables), so large-cardinality values differ. Port
  OpenSearch's `HyperLogLogPlusPlus`.
- `percentiles` match t-digest only for small inputs; port t-digest 3.x `MergingDigest`
  and HDR `DoubleHistogram`.
