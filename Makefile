# SlopenSearch developer entry point. Only Docker is required; `make help` lists targets.
#
#   make go-test                       # build + start the Go server, run the whole suite
#   make rust-test ARGS="-m aggs"      # Rust server, one area (pytest marker)
#   make go-test ARGS=tests/test_41_aggs_bucket.py
#   make go-score                      # pass rate per test file

ARGS ?=
# parallel pytest workers (one test file each); more than 4 overloads the reference
# OpenSearch cluster manager. JOBS=0 runs sequentially.
JOBS ?= 4
# traceback style. pytest spends ~0.3 s building a detailed report per failed test, which
# dominates runs with thousands of failures; use TB=long when debugging a few tests.
TB ?= line
COMPOSE := docker compose
# run as the host user so reports and recordings in the working tree are not owned by root
RUN_TESTS := $(COMPOSE) run --rm --build --user "$(shell id -u):$(shell id -g)" tests
PYTEST := pytest -n $(JOBS) --dist loadfile --tb=$(TB)

.DEFAULT_GOAL := help
.PHONY: help go-up rust-up ref-up go-test rust-test ref-test go-score rust-score ref-score \
        ref-show ref-record \
        go-dev rust-dev go-unit test go-logs rust-logs down bench bench-report \
        bench-osb-setup bench-quick bench-full bench-osb bench-history bench-scorecard \
        profile profiling-tools profiling-setup

help: ## list targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

go-up: ## build and start the Go server on :9291
	$(COMPOSE) up -d --build --wait go

rust-up: ## build and start the Rust server on :9292
	$(COMPOSE) up -d --build --wait rust

ref-up: ## start reference OpenSearch 3.9.0 on :9290
	$(COMPOSE) up -d --wait opensearch

go-test: go-up ## run tests against the Go server (ARGS="-m aggs" / a file / -k expr)
	$(RUN_TESTS) env OS_HOST=go $(PYTEST) $(ARGS)

rust-test: rust-up ## run tests against the Rust server
	$(RUN_TESTS) env OS_HOST=rust $(PYTEST) $(ARGS)

ref-test: ref-up ## run tests against reference OpenSearch (to check a new test)
	$(RUN_TESTS) env OS_HOST=opensearch $(PYTEST) $(ARGS)

go-score: go-up ## pass rate per test file for the Go server
	$(RUN_TESTS) env OS_HOST=go python scripts/score.py --jobs $(JOBS) --label go $(ARGS)

rust-score: rust-up ## pass rate per test file for the Rust server
	$(RUN_TESTS) env OS_HOST=rust python scripts/score.py --jobs $(JOBS) --label rust $(ARGS)

ref-show: ## what the reference OpenSearch answered in a test, no OpenSearch needed: make ref-show T=<name part>
	@$(RUN_TESTS) python scripts/ref.py show "$(T)"

ref-record: ref-up ## re-record tests/reference after changing tests (ARGS=test files, default all)
	$(RUN_TESTS) env OS_HOST=opensearch python scripts/ref.py record --jobs $(JOBS) $(ARGS)

ref-score: ref-up ## pass rate per test file for reference OpenSearch (should be 100%)
	$(RUN_TESTS) env OS_HOST=opensearch python scripts/score.py --jobs $(JOBS) --label opensearch $(ARGS)

go-dev: ## run the Go server natively on :9291 (needs Go)
	cd servers/go && PORT=9291 go run .

rust-dev: ## run the Rust server natively on :9292 (needs Rust)
	cd servers/rust && PORT=9292 cargo run --release

go-unit: ## run the Go unit tests natively (needs Go)
	cd servers/go && go test ./...

test: ## run tests against any server: make test PORT=9291 [HOST=host.docker.internal] ARGS=...
	$(RUN_TESTS) env OS_HOST=$(or $(HOST),host.docker.internal) OS_PORT=$(PORT) $(PYTEST) $(ARGS)

# benchmarks run on the host (Python venv with bench/requirements.txt) and start their own
# containers bench-* on ports 9590-9594; the dev stack above is not touched
BENCH_PY ?= .venv/bin/python
SIZE ?= small
TARGETS ?= ref,go,rust

bench: ## benchmark ref vs Go vs Rust in Docker: make bench SIZE=small|medium|large TARGETS=ref,go,rust ARGS=...
	$(BENCH_PY) bench/run.py --size $(SIZE) --targets $(TARGETS) $(ARGS)

bench-report: ## Markdown report of a benchmark run: make bench-report DIR=bench/results/<timestamp>
	$(BENCH_PY) bench/report.py $(DIR)

OSB := bench/.venv-osb/bin/opensearch-benchmark
WORKLOAD ?= geonames
PRESET ?= test

$(OSB):
	sh bench/osb_setup.sh

bench-osb-setup: ## (re)create bench/.venv-osb with opensearch-benchmark and the compat shim
	sh bench/osb_setup.sh

bench-quick: $(OSB) ## few minutes: Go+Rust, tiny harness + OSB geonames test mode, ratios vs cached ref
	$(BENCH_PY) bench/levels.py quick $(ARGS)

bench-full: $(OSB) ## hours: large harness + full OSB geonames and http_logs (ref when no baseline)
	$(BENCH_PY) bench/levels.py full $(ARGS)

bench-osb: $(OSB) ## one OSB workload: make bench-osb WORKLOAD=geonames PRESET=test|sample|full TARGETS=ref,go,rust
	$(BENCH_PY) bench/osb.py --workload $(WORKLOAD) --preset $(PRESET) --targets $(TARGETS) $(ARGS)

bench-scorecard: ## ratios vs ref for one run: make bench-scorecard DIR=bench/results/<timestamp>
	$(BENCH_PY) bench/scorecard.py report $(DIR)

bench-history: ## scorecard ratios commit by commit (bench/history.jsonl): ARGS="--target go"
	$(BENCH_PY) bench/scorecard.py history $(ARGS)

go-logs: ## follow Go server logs
	$(COMPOSE) logs -f go

rust-logs: ## follow Rust server logs
	$(COMPOSE) logs -f rust

down: ## stop everything
	$(COMPOSE) --profile tools down

# profiling: the Rust server samples itself (no perf/root needed), see scripts/profile.sh
profile: ## CPU profile of a running Rust server: make profile [PORT=9292] [SECS=10 | CMD="..."]
	sh scripts/profile.sh -p $(or $(PORT),9292) $(if $(CMD),-- $(CMD),-s $(or $(SECS),10))

profiling-tools: ## user-level profiling tools (inferno flamegraphs)
	cargo install inferno

profiling-setup: ## one-time root setup: perf for users and Docker targets, valgrind, heaptrack
	sudo sh scripts/setup-profiling.sh
