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

- Benchmarks with large data sets that compare the reference OpenSearch, Go and Rust on
  indexing and search throughput, latency, memory and CPU (in progress).
- Persistence: every write is fsynced before the response; group commit already batches
  concurrent writes, but sequential writes are 2–4x slower than in memory. Consider an
  OpenSearch-like `index.translog.durability: async` mode.
