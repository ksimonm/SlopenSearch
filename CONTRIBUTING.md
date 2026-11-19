# Contributing to SlopenSearch

Thanks for helping. SlopenSearch wants to be a better OpenSearch, which means two things,
and both count:

1. **Correct: the test suite is the specification.** Each test encodes what a real
   OpenSearch 3.9.0 does, and a server is correct exactly when the tests pass.
2. **Fast and lean: the benchmarks are the scoreboard.** Standard opensearch-benchmark
   workloads run against the reference OpenSearch and both servers under the same CPU and
   memory limits; the ratios (throughput, latency, memory vs the reference) are the
   numbers we work to improve. Passing tests is where a change starts, not where it ends.

A good change keeps the suite green and does not make the benchmarks worse; a performance
change shows its before/after numbers.

Only Docker is needed for tests; benchmarks also need Python 3.11+ on the host (see
[bench/README.md](bench/README.md)). `make help` lists every command.

## Making a test pass (server work)

1. Pick a red area: `make go-score` (or `rust-score`) shows the pass rate per file.
2. Read what the tests expect, and what the real OpenSearch answered:
   `make ref-show T=<test name>`. No OpenSearch needs to run; the answers are recorded
   in `tests/reference/`.
3. Make it green: `make go-test ARGS="-k <expr>"`.
4. Check nothing else broke: `make go-score` and compare with the numbers before.

Rules:

- Don't edit `tests/` to make a server pass. If you believe a test is wrong, prove it with
  `make ref-test ARGS=<test>` and fix the test in a separate change.
- Match OpenSearch byte for byte where tests check it: status codes, `error.type`, key
  names and order.
- The Go and Rust servers share nothing but the suite. Keep Go-only changes in
  `servers/go/` and Rust-only changes in `servers/rust/`.
- Go: `gofmt` and `go vet` clean. Rust: keep `cargo build` free of new warnings in the
  code you touch.
- Use libraries where they make sense. Adding a well-maintained Go module or Rust crate
  is welcome when it replaces code we would otherwise write and maintain ourselves
  (compression, hashing, parsers, protocol details, ...). Prefer widely used, actively
  maintained packages with a permissive license, keep the dependency set small, and say
  in the commit message why it was added. Hand-written ports are still the right call
  where we must match OpenSearch/Lucene bit for bit and no library does that.

Good areas to pick up: the multi-node and cross-DC suites (`test_96`, `test_97`).

## Making it faster (performance work)

Compatibility is close to 100%, so most of the open work is speed and memory. The loop:

1. **Measure.** `make bench-quick` (minutes) or one workload,
   `make bench-osb WORKLOAD=geonames PRESET=sample TARGETS=rust`. `make bench-history`
   shows the ratios vs the reference commit by commit; pick the operation with the worst
   ratio. The [Performance](README.md#performance) table in the README lists them.
2. **Profile.** `make profile PORT=9292 CMD="..."` samples a running Rust server while a
   command runs (no root needed) and prints the hottest functions;
   `python3 bench/proftop.py <dir>/stacks.folded --callers <function>` shows where they are
   called from, and `flamegraph.svg` is next to it. `--profile` on `bench/osb.py` and
   `bench/run.py` profiles a whole benchmark run. `make profiling-setup` (root, once) enables
   perf, valgrind and heaptrack for deeper work (any server, Go included).
3. **Fix the cause, not the benchmark.** Index-backed execution (inverted index, columns,
   top-k) is described in [docs/engine-design.md](docs/engine-design.md). Results must stay
   exactly what the evaluator computes: run the suite, and for index-backed paths also
   `SLOP_VERIFY_INDEX=true`, which reports every document where the fast path and the
   evaluator disagree.
4. **Show the numbers.** Put before/after timings of the affected operations in the commit
   message, and append a benchmark run to `bench/history.jsonl` (`make bench-quick` does
   it) when the change is merged.

Rules: no special-casing of benchmark queries or data, no caching of whole responses to
look fast (OSB's `*_uncached` operations exist to catch that), and memory counts as much
as speed.

## Working with an AI coding agent

Much of SlopenSearch is written with AI coding agents, and you are welcome to use one.
The suite keeps the work honest, as long as the agent follows the same rules as everyone
else. Start the agent in the repository root, pick a server and a test file, and replace
`<server>` (`go` or `rust`) and `<file>` in the prompt below. Where the work is now:

| Server | Open test files |
|---|---|
| Go | `test_72_knn` (1 test), `test_96_multinode`, `test_97_cross_dc` |
| Rust | `test_62_tasks_http`, `test_72_knn` (1 test), `test_96_multinode`, `test_97_cross_dc` |

`make <server>-score` shows the current numbers.

```text
You are working on SlopenSearch, a server that must answer the OpenSearch 3.9 REST API
exactly like OpenSearch does. Read README.md, CONTRIBUTING.md and
servers/<server>/README.md first.

Task: make more tests in tests/<file>.py pass against the <server> server.

How to work:
1. Run `make <server>-score` once and note the pass rate of every test file.
2. Run `make <server>-test ARGS=tests/<file>.py` and group the failures by cause.
   Pick the group that unblocks the most tests and fix it, then the next one.
3. For every failing test, read what the real OpenSearch answered:
   `make ref-show T=<test name>`, or grep tests/reference/<file>.jsonl. Match its status
   code, error type, key names and key order exactly. Don't start OpenSearch yourself.
4. Iterate with `make <server>-test TB=short ARGS="tests/<file>.py -k <expr>"`; failures
   then print the reference exchange as well.
5. At the end, run `make <server>-score` again and compare with step 1. No other file may
   lose passing tests.

Hard rules:
- Never edit anything in tests/ (including tests/reference/). If you believe a test is
  wrong, stop and tell me which test and why, quoting its recorded reference answer.
- Only change files in servers/<server>/.
- New Go modules or Rust crates are allowed when they save us real work (compression,
  parsers, hashing, ...): pick widely used, maintained, permissively licensed ones and
  name each new dependency and the reason in your report.
- Don't special-case test index names or test inputs; implement the real behaviour.

Finish with a short report: which behaviours you implemented, the pass rate of
<file>.py before and after, the total before and after, and anything you
left unfinished.
```

For performance work, the prompt is the same except for the task and the loop:

```text
Task: make <operation> of the geonames workload faster on the <server> server without
changing any response.

How to work:
1. Measure: `make bench-osb WORKLOAD=geonames PRESET=sample TARGETS=<server>` and note
   the p50 of every operation (compare with `make bench-history`).
2. Profile while the operation runs (`make profile ...`, see CONTRIBUTING.md) and find
   where the time goes before changing code.
3. Fix the cause. Keep results identical: run the whole suite, and SLOP_VERIFY_INDEX=true
   for index-backed paths.
4. Measure again and report before/after numbers for every operation, not only the
   target one. No operation may get slower.
```

When one server already passes a file the other one doesn't, you can add to the prompt:
"servers/<other>/ already passes this file; read it as a reference for the behaviour, but
write idiomatic code for this server." Before opening a pull request, read the diff
yourself: you are the author, not the agent.

## Adding or fixing a test

A test may only encode what real OpenSearch does. This is the one kind of work that needs
the reference OpenSearch running (Docker, ~2 GB RAM). Write the test, prove it against the
reference, and re-record that file's reference answers before opening a pull request:

```bash
make ref-test ARGS=tests/test_41_aggs_bucket.py
make ref-record ARGS=tests/test_41_aggs_bucket.py
```

If you can't run the reference, open the pull request anyway and say so; a maintainer
will verify and record it.

- Use unique index names (the `index_factory` fixture does this).
- Clean up anything cluster-wide you create (templates, pipelines, cluster settings).
- Don't depend on timing or on test order; files run in parallel workers.
- Put the test in the file and marker of its area (see the table in the README).

Commit the updated `tests/reference/*.jsonl` together with the test change.

## Reporting a compatibility gap

Open an issue with the request, what OpenSearch 3.9.0 answers, and what SlopenSearch
answers. A failing test in a pull request is even better.

## Pull requests

Keep them focused on one area, describe what changed in the pass rate (`make go-score` /
`make rust-score` before and after) and, for changes that touch search, indexing or
storage, in the benchmark numbers (before and after, same machine and limits). By contributing you agree that your contribution is
licensed under the [Apache License 2.0](LICENSE).
