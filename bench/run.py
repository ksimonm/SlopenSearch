#!/usr/bin/env python3
"""SlopenSearch benchmark: reference OpenSearch vs the Go and Rust servers.

  python bench/run.py --targets ref,go,rust --size small
  python bench/run.py --targets go-mem,rust-mem --size medium --workloads index,search
  python bench/report.py bench/results/<timestamp>

Each target runs alone in a fresh Docker container with the same --cpus/--memory limits;
results go to bench/results/<timestamp>/<target>.json. See bench/README.md.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import platform
import sys
import time
import traceback
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import datagen  # noqa: E402
import loadgen  # noqa: E402
import workloads as W  # noqa: E402
from targets import Sampler, make_box, all_targets, build_images, git_rev, servers_rev, sh  # noqa: E402

ALL_WORKLOADS = ["index", "sweep", "verify", "search", "knn", "mixed", "update", "restart"]
PRESETS = {
    #         search s/query  mixed s  update ops  sweep docs cap
    "tiny":   (4,             8,       500,        10_000),
    "small":  (8,             15,      2_000,      50_000),
    "medium": (10,            20,      5_000,      100_000),
    "large":  (12,            30,      10_000,     100_000),
}


class Run:
    def __init__(self, a, data: Path, out: Path):
        self.a, self.data, self.out = a, data, out
        self.meta = json.loads((data / "meta.json").read_text())
        self.n_docs, self.n_vec = self.meta["docs"], self.meta["vectors"]
        self.queries = json.loads((data / "queries.json").read_text())
        self.search_s, self.mixed_s, self.update_ops, self.sweep_cap = PRESETS[a.size]
        if a.search_duration:
            self.search_s = a.search_duration

    # ------------------------------------------------------------------ helpers
    def call(self, method, path, body=None, timeout=600):
        c = loadgen.Conn(self.t.port, timeout=timeout)
        data = json.dumps(body).encode() if isinstance(body, (dict, list)) else body
        st, resp = c.req(method, path, data)
        try:
            return st, json.loads(resp) if resp else None
        except ValueError:
            return st, resp.decode(errors="replace")

    def must(self, method, path, body=None, timeout=600):
        st, r = self.call(method, path, body, timeout)
        if not 200 <= st < 300:
            raise RuntimeError(f"{method} {path} -> {st}: {str(r)[:500]}")
        return r

    def measure(self, name, fn):
        """Run fn() and attach resource usage of the window to its result."""
        print(f"  [{self.t.name}] {name} ...", flush=True)
        t0 = time.time()
        try:
            res = fn() or {}
        except Exception as e:  # noqa: BLE001 - recorded in the results
            res = {"error": f"{type(e).__name__}: {e}"}
            traceback.print_exc()
        t1 = time.time()
        res["elapsed_s"] = round(t1 - t0, 2)
        res["resources"] = self.sampler.window(res.get("t0", t0), res.get("t1", t1))
        if not self.box.running():
            st = self.box.state()
            res["crash"] = {"oom_killed": st.get("OOMKilled"), "exit_code": st.get("ExitCode"),
                            "logs": self.box.logs(30)}
        self.res["workloads"][name] = res
        self._summary(name, res)
        self.save()
        return res

    def _summary(self, name, r):
        if "error" in r:
            print(f"    ERROR {r['error'][:300]}", flush=True)
        bits = []
        if "seconds" in r:
            bits.append(f"{r['seconds']} s")
        if "docs_per_s" in r:
            bits.append(f"{r['docs_per_s']:,.0f} docs/s")
        for tag, o in r.get("ops", {}).items():
            bits.append(f"{tag}: {o['ops_per_s']:,.1f}/s p50={o['p50']} p99={o['p99']} err={o['errors']}")
        rs = r.get("resources", {})
        if rs:
            bits.append(f"cpu avg {rs.get('cpu_avg')} peak {rs.get('cpu_peak')}, mem peak {rs.get('mem_peak', 0) / 2**20:,.0f} MiB")
        print("    " + " | ".join(bits), flush=True)
        if r.get("first_error"):
            print(f"    first error: {r['first_error'][:300]}", flush=True)
        if "crash" in r:
            print(f"    CONTAINER DIED: {r['crash']['oom_killed']=} exit={r['crash']['exit_code']}", flush=True)

    def save(self):
        (self.out / f"{self.t.name}.json").write_text(json.dumps(self.res, indent=1))

    def load(self, spec, clients, duration=None, deadline=None):
        spec = dict(spec, timeout=self.a.request_timeout)
        return loadgen.run(self.t.port, spec, clients, duration, deadline)

    def guarded(self, path, body):
        """One sequential search with a short timeout (3 x --slow-limit). After a timeout the
        server is still busy with the abandoned query (the SlopenSearch servers do not cancel
        it), so wait until it answers again before going on. Returns (ms, status, response)."""
        c = loadgen.Conn(self.t.port, timeout=min(self.a.request_timeout, 3 * self.a.slow_limit))
        s = time.perf_counter()
        st, data = c.req("POST", path, json.dumps(body).encode())
        ms = round((time.perf_counter() - s) * 1000, 1)
        if st == -1:
            self.drain()
            return ms, st, None
        try:
            return ms, st, json.loads(data)
        except ValueError:
            return ms, st, data.decode(errors="replace")

    def drain(self):
        t0 = time.time()
        while time.time() - t0 < self.a.request_timeout:
            c = loadgen.Conn(self.t.port, timeout=5)
            s = time.time()
            st, _ = c.req("GET", "/_cluster/health")
            if st == 200 and time.time() - s < 1:
                break
        print(f"    (waited {time.time() - t0:.0f}s for the server to finish an abandoned query)", flush=True)

    def probe(self, path, body):
        """One sequential request (also a warm-up); returns (ms, status)."""
        ms, st, _ = self.guarded(path, body)
        return ms, st

    def timed_or_skip(self, name, path, body, run):
        """Run the timed workload unless a single probe request is slower than --slow-limit."""
        ms, st = self.probe(path, body)
        if ms > self.a.slow_limit * 1000 or st == -1:
            r = {"skipped_slow": True, "probe_ms": ms, "probe_status": st}
            print(f"  [{self.t.name}] {name}: SKIPPED, one request took {ms / 1000:.1f}s (status {st})", flush=True)
            self.res["workloads"][name] = r
            self.save()
            return r
        return self.measure(name, run)

    def ops(self, tag, bodies, path):
        return [(tag, "POST", path, json.dumps(b).encode()) for b in bodies]

    # ------------------------------------------------------------------ workloads
    def w_index(self):
        a = self.a
        self.must("PUT", f"/{W.TEXT}", W.TEXT_MAPPING)
        deadline = time.time() + a.index_timeout
        spec = {"type": "bulk", "file": str(self.data / "docs.ndjson"), "index": W.TEXT,
                "batch": a.batch, "max_docs": self.n_docs}
        r = self.measure("index_text", lambda: self.load(spec, a.clients, deadline=deadline))
        self.measure("refresh_text", lambda: self._timed(lambda: self.must("POST", f"/{W.TEXT}/_refresh")))
        cnt = self.must("GET", f"/{W.TEXT}/_count")["count"]
        self.res["counts"]["text_after_index"] = cnt
        if cnt != self.n_docs:
            print(f"    WARNING: {cnt} docs searchable, expected {self.n_docs}", flush=True)
        self.indexed_docs = r.get("docs", 0)
        self.res["text_docs_indexed"] = self.indexed_docs

        if "knn" in self.wl:
            self.must("PUT", f"/{W.VEC}", W.vec_mapping(self.meta["dim"]))
            vspec = {"type": "bulk", "file": str(datagen.vectors_ndjson(self.data)), "index": W.VEC,
                     "batch": max(100, a.batch // 2), "max_docs": self.n_vec}
            self.measure("index_vectors", lambda: self.load(vspec, a.clients, deadline=time.time() + a.index_timeout))
            self.measure("refresh_vectors", lambda: self._timed(lambda: self.must("POST", f"/{W.VEC}/_refresh", timeout=3600)))
            self.res["counts"]["vec_after_index"] = self.must("GET", f"/{W.VEC}/_count")["count"]
        self.measure("idle_after_index", lambda: self._idle(5))
        if self.t.kind == "ref":
            self.call("POST", "/_flush")
        self.res["disk_after_index"] = self.box.disk_bytes()

    def _timed(self, fn):
        t0 = time.time()
        fn()
        return {"t0": t0, "t1": time.time(), "seconds": round(time.time() - t0, 3)}

    def _idle(self, s):
        t0 = time.time()
        time.sleep(s)
        return {"t0": t0, "t1": time.time()}

    def w_sweep(self):
        n = min(self.n_docs, self.sweep_cap)
        out = {}
        for batch, clients in ((100, 4), (1000, 1), (1000, 4), (1000, 8), (5000, 4)):
            self.call("DELETE", f"/{W.SWEEP}")
            self.must("PUT", f"/{W.SWEEP}", W.TEXT_MAPPING)
            spec = {"type": "bulk", "file": str(self.data / "docs.ndjson"), "index": W.SWEEP,
                    "batch": batch, "max_docs": n}
            r = self.measure(f"sweep_b{batch}_c{clients}",
                             lambda: self.load(spec, clients, deadline=time.time() + self.a.index_timeout))
            out[f"b{batch}_c{clients}"] = r.get("docs_per_s")
        self.call("DELETE", f"/{W.SWEEP}")

    def w_verify(self):
        """Sequential pass over the first queries of every set with exact totals."""
        v = {}
        sets = dict(W.text_queries(self.queries, n=10))
        if self.a.queries:
            sets = {k: sets[k] for k in self.a.queries.split(",") if k in sets}
        for name, bodies in sets.items():
            v[name] = []
            for b in bodies:
                ms, st, r = self.guarded(f"/{W.TEXT}/_search", dict(b, track_total_hits=True))
                v[name].append(W.digest(r) if st == 200 else {"status": st, "error": str(r)[:300]})
                if ms > self.a.slow_limit * 1000:
                    break   # too slow to verify all of them
        # search_after: ids of the first 3 pages of the first query
        q0 = W.search_after_queries(self.queries)[0]
        after, ids = None, []
        for _ in range(3 if not self.a.queries or "search_after" in self.a.queries else 0):
            body = dict(q0, size=100, sort=[{"seq": "asc"}])
            if after:
                body["search_after"] = after
            _, st, r = self.guarded(f"/{W.TEXT}/_search", body)
            if st != 200 or not r["hits"]["hits"]:
                break
            ids += [h["_id"] for h in r["hits"]["hits"]]
            after = r["hits"]["hits"][-1]["sort"]
        if ids:
            v["search_after"] = [{"ids": ids}]
        if "knn" in self.wl:
            _, _, qv, truth = datagen.load_vectors(self.data)
            kq = W.knn_queries(qv, truth)
            for name, key in (("knn", "truth"), ("knn_filter", "truth_filtered")):
                rec, res = [], []
                t_end = time.time() + 6 * self.a.slow_limit
                for i, b in enumerate(kq[name][:100]):
                    if time.time() > t_end:
                        break
                    _, st, r = self.guarded(f"/{W.VEC}/_search", b)
                    if st != 200:
                        res.append({"status": st, "error": str(r)[:300]})
                        if st == -1:
                            break
                        continue
                    got = [h["_id"] for h in r["hits"]["hits"]]
                    want = {str(x) for x in truth[key][i][:10]}
                    rec.append(len(want & set(got)) / 10)
                    res.append({"ids": got})
                v[name] = res
                self.res["recall"][name] = round(sum(rec) / len(rec), 4) if rec else None
        self.res["verify"] = v
        return {}

    def w_search(self):
        sets = W.text_queries(self.queries)
        names = self.a.queries.split(",") if self.a.queries else list(sets)
        for name in names:
            if name == "search_after":
                continue
            self.timed_or_skip(f"search_{name}", f"/{W.TEXT}/_search", sets[name][0], lambda: self.load(
                {"type": "list", "ops": self.ops(name, sets[name], f"/{W.TEXT}/_search")},
                self.a.search_clients, duration=self.search_s))
        if not self.a.queries or "search_after" in names:
            spec = {"type": "search_after", "index": W.TEXT, "size": 100, "pages": 10,
                    "queries": W.search_after_queries(self.queries)}
            q0 = dict(spec["queries"][0], size=100, sort=[{"seq": "asc"}])
            self.timed_or_skip("search_search_after", f"/{W.TEXT}/_search", q0,
                               lambda: self.load(spec, self.a.search_clients, duration=self.search_s))

    def w_knn(self):
        _, _, qv, truth = datagen.load_vectors(self.data)
        kq = W.knn_queries(qv, truth)
        for name in ("knn", "knn_filter"):
            self.timed_or_skip(f"search_{name}", f"/{W.VEC}/_search", kq[name][0], lambda: self.load(
                {"type": "list", "ops": self.ops(name, kq[name], f"/{W.VEC}/_search")},
                self.a.search_clients, duration=self.search_s))

    def w_mixed(self):
        sets = W.text_queries(self.queries, n=20)
        reads = []
        for name in ("match", "bool", "filter_term_range", "agg_terms", "sort_date"):
            reads += self.ops(name, sets[name], f"/{W.TEXT}/_search")
        spec = {"type": "mixed", "reads": reads, "write_ratio": 0.2, "write_batch": 10,
                "file": str(self.data / "docs.ndjson"), "index": W.TEXT}
        self.timed_or_skip("mixed_rw", f"/{W.TEXT}/_search", sets["match"][0],
                           lambda: self.load(spec, self.a.mixed_clients, duration=self.mixed_s))
        self.must("POST", f"/{W.TEXT}/_refresh")
        # remove the written documents again so later counts are comparable between targets
        st, r = self.call("POST", f"/{W.TEXT}/_delete_by_query?refresh=true",
                          {"query": {"term": {"mixed": True}}}, timeout=self.a.request_timeout)
        self.res["counts"]["mixed_written_and_removed"] = r.get("deleted") if st == 200 else f"HTTP {st}"

    def w_update(self):
        n = self.indexed_docs or self.n_docs
        m = min(self.update_ops, n // 4)
        self.must("POST", f"/{W.TEXT}/_refresh")
        before = self.must("GET", f"/{W.TEXT}/_count")["count"]
        # ids are spread over the corpus; updates in the first quarter, deletes in the last
        upd = [("update", "POST", f"/{W.TEXT}/_update/{(i * 7919) % (n // 2)}",
                json.dumps({"doc": {"quantity": i % 1000, "rating": 4.5}}).encode()) for i in range(m)]
        cap = self.a.update_timeout
        self.measure("update_single", lambda: self.load({"type": "list", "ops": upd, "once": True}, self.a.clients,
                                                        deadline=time.time() + cap))
        dels = [("delete", "DELETE", f"/{W.TEXT}/_doc/{n - 1 - i}", None) for i in range(m)]
        self.measure("delete_single", lambda: self.load({"type": "list", "ops": dels, "once": True}, self.a.clients,
                                                        deadline=time.time() + cap))
        bspec = {"type": "bulk", "file": str(self.data / "docs.ndjson"), "index": W.TEXT,
                 "batch": self.a.batch, "max_docs": min(n // 2, 100_000), "action": "update"}
        self.measure("bulk_update", lambda: self.load(bspec, self.a.clients, deadline=time.time() + cap))
        users = self.queries["users"][:5]
        dbq = [("delete_by_query", "POST", f"/{W.TEXT}/_delete_by_query?refresh=true",
                json.dumps({"query": {"term": {"user_id": u}}}).encode()) for u in users]
        self.measure("delete_by_query", lambda: self.load({"type": "list", "ops": dbq, "once": True}, 1,
                                                          deadline=time.time() + cap))
        self.must("POST", f"/{W.TEXT}/_refresh")
        after = self.must("GET", f"/{W.TEXT}/_count")["count"]
        self.res["counts"]["text_after_updates"] = after
        self.res["counts"]["removed_by_update_workload"] = before - after
        st, r = self.call("POST", f"/{W.TEXT}/_count", {"query": {"term": {"rating": 4.5}}})
        self.res["counts"]["rating_4_5_after_updates"] = r.get("count") if st == 200 else None

    def w_restart(self):
        if not self.t.persistent:
            return
        if self.t.kind == "ref":
            self.call("POST", "/_flush")
        disk = self.box.disk_bytes()
        expect = self.must("GET", f"/{W.TEXT}/_count")["count"]
        stop_s = self.box.stop()
        t0 = time.time()
        ready_s = self.box.start()
        # the cgroup is recreated on start
        self.sampler.stop()
        self.sampler = Sampler(self.box.cgroup_dir())
        self.sampler.start()
        cnt, avail = None, None
        while time.time() - t0 < 900:
            st, r = self.call("GET", f"/{W.TEXT}/_count")
            if st == 200:
                cnt = r["count"]
                avail = time.time() - t0
                break
            time.sleep(0.1)
        q = W.text_queries(self.queries, n=1)["match"][0]
        s = time.perf_counter()
        st, _ = self.call("POST", f"/{W.TEXT}/_search", q)
        first = (time.perf_counter() - s) * 1000
        self.res["restart"] = {"disk_bytes": disk, "stop_s": round(stop_s, 2), "ready_s": round(ready_s, 2),
                               "data_available_s": round(avail, 2) if avail else None,
                               "count_after": cnt, "count_expected": expect,
                               "first_query_ms": round(first, 1), "first_query_status": st}
        time.sleep(3)
        self.res["restart"]["resources_after"] = self.sampler.window(time.time() - 3, time.time())
        print(f"    restart: {self.res['restart']}", flush=True)

    # ------------------------------------------------------------------ driver
    def target(self, t):
        self.t = t
        self.box = make_box(t, self.a.cpus, self.a.memory, self.a.profile)
        self.res = {"target": t.name, "image": t.image, "persistent": t.persistent, "workloads": {},
                    "counts": {}, "recall": {}, "started": dt.datetime.now().isoformat(timespec="seconds")}
        self.wl = set(self.a.workloads.split(","))
        self.indexed_docs = 0
        print(f"== {t.name} ({t.image}) on :{t.port}", flush=True)
        try:
            self.res["startup_empty_s"] = round(self.box.create_and_start(), 2)
            self.sampler = Sampler(self.box.cgroup_dir())
            self.sampler.start()
            time.sleep(1)
            self.res["idle_empty"] = self.sampler.window(time.time() - 1, time.time())
            info = self.must("GET", "/")
            self.res["version"] = info.get("version", {}).get("number")
            prof = None
            if self.a.profile and t.kind != "ref":
                from native import Profiler
                prof = Profiler(self.box.server_pid(), self.out / f"profile-{t.name}", port=t.port).start()
            steps = [("index", self.w_index), ("sweep", self.w_sweep), ("verify", self.w_verify),
                     ("search", self.w_search), ("knn", self.w_knn), ("mixed", self.w_mixed),
                     ("update", self.w_update), ("restart", self.w_restart)]
            for name, fn in steps:
                if name not in self.wl and name != "index":
                    continue
                if not self.box.running():
                    self.res.setdefault("skipped", []).append(name)
                    continue
                try:
                    fn()
                except Exception as e:  # noqa: BLE001
                    traceback.print_exc()
                    self.res.setdefault("errors", {})[name] = f"{type(e).__name__}: {e}"
            if prof:
                self.res["profile"] = prof.stop()
                print(f"    profile: {self.res['profile']}", flush=True)
            self.res["oom_kills"] = self.sampler.oom_kills()
            if not self.box.running():
                st = self.box.state()
                self.res["died"] = {"oom_killed": st.get("OOMKilled"), "exit_code": st.get("ExitCode"),
                                    "logs": self.box.logs(30)}
            self.res["memory_peak_total"] = self._mem_peak()
        finally:
            self.res["finished"] = dt.datetime.now().isoformat(timespec="seconds")
            self.save()
            if hasattr(self, "sampler"):
                self.sampler.stop()
            if not self.a.keep:
                self.box.remove()

    def _mem_peak(self):
        try:
            return int((self.box.cgroup_dir() / "memory.peak").read_text())
        except Exception:  # noqa: BLE001
            return None


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--targets", default="ref,go,rust",
                    help="comma list of ref, go, rust, go-mem, rust-mem (default ref,go,rust)")
    ap.add_argument("--size", choices=datagen.SIZES, default="small")
    ap.add_argument("--docs", type=int, help="override the number of text documents")
    ap.add_argument("--vectors", type=int, help="override the number of vectors")
    ap.add_argument("--workloads", default=",".join(ALL_WORKLOADS),
                    help=f"comma list of {','.join(ALL_WORKLOADS)} (index always runs)")
    ap.add_argument("--queries", help="only these search workloads (names from workloads.text_queries)")
    ap.add_argument("--cpus", default=os.environ.get("BENCH_CPUS", "4"))
    ap.add_argument("--memory", default=os.environ.get("BENCH_MEMORY", "8g"))
    ap.add_argument("--heap", default=os.environ.get("BENCH_HEAP", "4g"), help="reference OpenSearch JVM heap")
    ap.add_argument("--clients", type=int, default=8, help="bulk/update clients")
    ap.add_argument("--batch", type=int, default=1000, help="bulk batch size of the main indexing run")
    ap.add_argument("--search-clients", type=int, default=4)
    ap.add_argument("--mixed-clients", type=int, default=8)
    ap.add_argument("--search-duration", type=float, help="seconds per search workload (default by size)")
    ap.add_argument("--request-timeout", type=float, default=120, help="client timeout per request (s)")
    ap.add_argument("--slow-limit", type=float, default=10,
                    help="skip a timed search workload when one probe request takes longer (s)")
    ap.add_argument("--update-timeout", type=float, default=300,
                    help="stop each update/delete workload after this many seconds")
    ap.add_argument("--index-timeout", type=float, default=1800, help="give up indexing after this many seconds")
    ap.add_argument("--data-dir", type=Path, default=None, help="generated data (default /tmp/agent-bench/data)")
    ap.add_argument("--out", type=Path, default=None, help="results directory (default bench/results/<timestamp>)")
    ap.add_argument("--no-build", action="store_true", help="do not rebuild the go/rust images")
    ap.add_argument("--keep", action="store_true", help="keep the containers running afterwards")
    ap.add_argument("--profile", action="store_true",
                    help="perf record the server (not ref) during the run; see bench/native.py")
    ap.add_argument("--level", default="adhoc", help="label stored in run.json and the scorecard (quick/full/adhoc)")
    a = ap.parse_args()

    targets = all_targets(a.heap)
    names = a.targets.split(",")
    for n in names:
        if n not in targets:
            ap.error(f"unknown target {n}")
    data = datagen.ensure(a.size, a.data_dir, a.docs, a.vectors)
    if not a.no_build:
        imgs = {targets[n].image for n in names if targets[n].kind == "slop"}
        build_images(imgs)
    out = a.out or Path(__file__).resolve().parent / "results" / dt.datetime.now().strftime("%Y%m%d-%H%M%S")
    out.mkdir(parents=True, exist_ok=True)
    meta = json.loads((data / "meta.json").read_text())
    run_meta = {
        "date": dt.datetime.now().isoformat(timespec="seconds"), "size": a.size, "data": meta,
        "docs_file_bytes": (data / "docs.ndjson").stat().st_size,
        "targets": names, "cpus": a.cpus, "memory": a.memory, "heap": a.heap,
        "clients": a.clients, "batch": a.batch, "search_clients": a.search_clients,
        "mixed_clients": a.mixed_clients, "workloads": a.workloads,
        "slow_limit_s": a.slow_limit, "request_timeout_s": a.request_timeout,
        "search_duration_s": a.search_duration or PRESETS[a.size][0],
        "level": a.level, "git_rev": git_rev(), "servers_rev": servers_rev(), "host": platform.node(), "kernel": platform.release(),
        "cpu_model": _cpu_model(), "host_cpus": os.cpu_count(), "host_mem_gb": _mem_gb(),
        "loadavg_start": os.getloadavg(),
        "images": {n: sh("docker", "image", "inspect", "-f", "{{.Id}}", targets[n].image, check=False)[:19]
                   for n in names},
    }
    (out / "run.json").write_text(json.dumps(run_meta, indent=1))
    r = Run(a, data, out)
    for n in names:
        r.target(targets[n])
    run_meta["loadavg_end"] = os.getloadavg()
    run_meta["finished"] = dt.datetime.now().isoformat(timespec="seconds")
    (out / "run.json").write_text(json.dumps(run_meta, indent=1))
    import report
    (out / "report.md").write_text(report.render(out))
    print(f"results: {out}\nreport:  {out / 'report.md'}")


def _cpu_model():
    try:
        for line in open("/proc/cpuinfo"):
            if line.startswith("model name"):
                return line.split(":", 1)[1].strip()
    except OSError:
        pass
    return platform.processor()


def _mem_gb():
    try:
        for line in open("/proc/meminfo"):
            if line.startswith("MemTotal"):
                return round(int(line.split()[1]) / 2**20)
    except OSError:
        return None


if __name__ == "__main__":
    main()
