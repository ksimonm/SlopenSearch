#!/usr/bin/env python3
"""Run standard opensearch-benchmark (OSB) workloads against ref, Go and Rust.

  python bench/osb.py --targets ref,go,rust --workload geonames --preset test
  python bench/osb.py --targets go,rust --workload geonames --preset sample --params ingest_percentage=2
  python bench/osb.py --targets ref --workload geonames --preset full

Every target runs alone in a fresh container with the same --cpus/--memory (bench/targets.py)
and OSB drives it with `--pipeline=benchmark-only`. A run is split into OSB invocations:
first the setup tasks (delete/create index, cluster health, bulk, refresh, force-merge),
then the query tasks in one invocation. A watchdog follows OSB's log; when one task runs
longer than --task-timeout, OSB is killed, the task is recorded as `timeout`, and the
remaining tasks are re-run in new invocations (the server is given time to finish the
abandoned request first). Per-task errors are taken from OSB's results (`error_rate`);
with --proxy every HTTP call goes through bench/proxy.py, which records the endpoints and
the errors the server returned (compatibility gaps).

OSB runs from its own venv (bench/.venv-osb, see bench/requirements-osb.txt) with
BENCHMARK_HOME=~/.cache/slopbench (workloads, corpora, OSB logs and test runs live there,
not in /tmp). Results: bench/results/<timestamp>/osb-<target>.json + run.json; then
bench/scorecard.py turns them into scorecard.json (see bench/README.md).
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import re
import signal
import subprocess
import sys
import threading
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import loadgen  # noqa: E402
from proxy import Proxy  # noqa: E402
from targets import Sampler, make_box, all_targets, build_images, git_rev, servers_rev, sh  # noqa: E402

BENCH = Path(__file__).resolve().parent
OSB_BIN = Path(os.environ.get("OSB_BIN", BENCH / ".venv-osb" / "bin" / "opensearch-benchmark"))
OSB_HOME = Path(os.environ.get("BENCHMARK_HOME", Path.home() / ".cache" / "slopbench"))

# workload params per preset. "sample": a slice of the corpus and few iterations without a
# target throughput (closed loop, 1 client), so even slow servers finish in minutes;
# "full": the workload's own defaults (throttled to its target throughputs).
PRESETS = {
    "test": {"test_mode": True, "params": {}},
    "sample": {"test_mode": False, "params": {"ingest_percentage": 1, "warmup_iterations": 2, "iterations": 10, "warmup_time_period": 0,
                                               "target_throughput": 100000}},
    "full": {"test_mode": False, "params": {}},
}
SETUP_RE = re.compile(r"delete-index|create-index|cluster-health|index-append|bulk|refresh|merge|"
                      r"index-|create-|delete-|put-|wait-")
LOG_TASK_RE = re.compile(r"^(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d),(\d+) .*Choosing \[.*\] for \[(.+?)\]\.")


def osb_env() -> dict:
    return dict(os.environ, BENCHMARK_HOME=str(OSB_HOME), SLOP_OSB_COMPAT="1")


def schedule(workload: str, procedure: str | None) -> list[str]:
    """Task names of the test procedure, from `opensearch-benchmark info`."""
    cmd = [str(OSB_BIN), "info", f"--workload={workload}"]
    if procedure:
        cmd.append(f"--test-procedure={procedure}")
    out = subprocess.run(cmd, capture_output=True, text=True, env=osb_env(), timeout=600).stdout
    # the first schedule printed is the selected (or default) procedure
    tasks, in_sched = [], False
    for line in out.splitlines():
        if line.startswith("Schedule:"):
            if in_sched:
                break
            in_sched = True
            continue
        m = re.match(r"^\d+\. (\S+)", line)
        if in_sched and m:
            tasks.append(m.group(1))
        elif in_sched and tasks and line.startswith("==="):
            break
    if not tasks:
        raise RuntimeError(f"no schedule found in `{' '.join(cmd)}`:\n{out[-2000:]}")
    return tasks


def split_setup(tasks: list[str]) -> tuple[list[str], list[str]]:
    """Leading block of setup tasks (index creation, bulk, refresh, merges) and the rest."""
    last = -1
    for i, t in enumerate(tasks[:15]):
        if SETUP_RE.search(t) and not t.endswith("-stats"):
            last = i
    return tasks[:last + 1], tasks[last + 1:]


class OsbRun:
    def __init__(self, a, out: Path):
        self.a, self.out = a, out
        self.preset = PRESETS[a.preset]
        self.params = dict(self.preset["params"])
        for kv in a.params or []:
            k, _, v = kv.partition("=")
            try:
                self.params[k] = json.loads(v)
            except ValueError:
                self.params[k] = v
        self.log_path = OSB_HOME / ".osb" / "logs" / "benchmark.log"

    # ------------------------------------------------------------------ one OSB invocation
    def invoke(self, tasks: list[str], label: str, task_timeout: float) -> dict:
        run_id = f"slop-{self.t.name}-{self.a.workload}-{label}-{int(time.time())}"
        params_file = self.out / f"osb-params-{self.a.workload}.json"
        params_file.write_text(json.dumps(self.params))
        host = f"127.0.0.1:{self.proxy.port if self.proxy else self.t.port}"
        cmd = [str(OSB_BIN), "run", "--pipeline=benchmark-only", f"--target-hosts={host}",
               f"--workload={self.a.workload}", f"--test-run-id={run_id}", "--kill-running-processes",
               "--on-error=continue", f"--client-options=timeout:{int(self.a.request_timeout)}",
               f"--include-tasks={','.join(tasks)}",
               f"--results-file={self.out / f'osb-{self.t.name}-{label}.md'}",
               f"--user-tag=target:{self.t.name}"]
        if self.params:
            cmd.append(f"--workload-params={params_file}")
        if self.a.procedure:
            cmd.append(f"--test-procedure={self.a.procedure}")
        if self.preset["test_mode"]:
            cmd.append("--test-mode")
        if self.a.offline:
            cmd.append("--offline")
        log_off = self.log_path.stat().st_size if self.log_path.exists() else 0
        stdout = (self.out / f"osb-{self.t.name}-{label}.log").open("w")
        print(f"  [{self.t.name}] osb {label}: {len(tasks)} tasks ({', '.join(tasks[:4])}{' ...' if len(tasks) > 4 else ''})",
              flush=True)
        t0 = time.time()
        p = subprocess.Popen(cmd, stdout=stdout, stderr=subprocess.STDOUT, env=osb_env(), start_new_session=True)
        starts: list[tuple[str, float]] = []
        killed = None
        while p.poll() is None:
            time.sleep(1)
            starts = self.task_starts(log_off, set(tasks))
            now = time.time()
            if starts and now - starts[-1][1] > task_timeout:
                killed = starts[-1][0]
            elif not starts and now - t0 > self.a.startup_timeout:
                killed = "(startup)"
            if killed:
                print(f"    task {killed} exceeded {task_timeout:.0f}s: killing OSB", flush=True)
                os.killpg(p.pid, signal.SIGKILL)
                p.wait()
                break
        stdout.close()
        t1 = time.time()
        starts = self.task_starts(log_off, set(tasks))
        compat = self.log_lines(log_off, "slop_osb_compat:")
        res = {"label": label, "tasks": tasks, "returncode": p.returncode, "seconds": round(t1 - t0, 1),
               "killed_in_task": killed, "test_run_id": run_id, "metrics": {}, "windows": {},
               "compat_warnings": sorted(set(compat))}
        for i, (name, ts) in enumerate(starts):
            end = starts[i + 1][1] if i + 1 < len(starts) else t1
            res["windows"][name] = [ts, end]
        tr = OSB_HOME / ".osb" / "benchmarks" / "test-runs" / run_id / "test_run.json"
        if tr.exists() and not killed:
            d = json.loads(tr.read_text())
            for om in d["results"].get("op_metrics", []):
                res["metrics"][om["task"]] = om
            res["osb_results"] = {k: v for k, v in d["results"].items()
                                  if k in ("store_size", "docs_count", "segment_count", "total_time",
                                           "refresh_count", "merge_count")}
            res["cluster"] = d.get("cluster", {}).get("distribution-version")
        if p.returncode not in (0, None) and not killed:
            res["output_tail"] = (self.out / f"osb-{self.t.name}-{label}.log").read_text()[-3000:]
            print(f"    OSB exited {p.returncode}: {res['output_tail'][-500:]}", flush=True)
        return res

    def log_lines(self, offset: int, needle: str) -> list[str]:
        """OSB log lines after offset that contain needle (text after the needle)."""
        out = []
        try:
            with self.log_path.open("rb") as f:
                f.seek(offset)
                for raw in f:
                    line = raw.decode(errors="replace")
                    if needle in line:
                        out.append(line.split(needle, 1)[1].strip()[:300])
        except OSError:
            pass
        return out

    def task_starts(self, offset: int, names: set[str]) -> list[tuple[str, float]]:
        """(task, start epoch) from OSB's log (UTC timestamps) after offset, in order."""
        out, seen = [], set()
        try:
            with self.log_path.open("rb") as f:
                f.seek(offset)
                for raw in f:
                    m = LOG_TASK_RE.match(raw.decode(errors="replace"))
                    if m and m.group(3) in names and m.group(3) not in seen:
                        ts = dt.datetime.strptime(m.group(1), "%Y-%m-%d %H:%M:%S").replace(
                            tzinfo=dt.timezone.utc).timestamp() + int(m.group(2)) / 1000
                        seen.add(m.group(3))
                        out.append((m.group(3), ts))
        except OSError:
            pass
        return out

    def run_tasks(self, tasks: list[str], label: str, task_timeout: float, depth=0):
        """Run tasks; on a watchdog kill record the task as timed out and re-run the others."""
        if not tasks:
            return
        if not self.box.running():
            for t in tasks:
                self.tasks.setdefault(t, {"status": "not_run", "reason": "container not running"})
            return
        if self.a.target_budget and time.time() - self.t_start > self.a.target_budget:
            for t in tasks:
                self.tasks.setdefault(t, {"status": "not_run", "reason": f"--target-budget {self.a.target_budget:.0f}s used up"})
            return
        r = self.invoke(tasks, label, task_timeout)
        self.res["invocations"].append({k: v for k, v in r.items() if k not in ("metrics", "windows")})
        for name, (w0, w1) in r["windows"].items():
            self.windows[name] = (w0, w1)
        if r["killed_in_task"]:
            k = r["killed_in_task"]
            if k in tasks:
                self.tasks[k] = {"status": "timeout", "timeout_s": task_timeout,
                                 "window": self.windows.get(k)}
            self.drain()
            if k not in tasks or depth > 6:
                for t in tasks:
                    self.tasks.setdefault(t, {"status": "not_run", "reason": "OSB did not start"})
                return
            i = tasks.index(k)
            self.run_tasks(tasks[:i], f"{label}a", task_timeout, depth + 1)
            self.run_tasks(tasks[i + 1:], f"{label}b", task_timeout, depth + 1)
            return
        for t in tasks:
            if t in r["metrics"]:
                self.tasks[t] = {"status": "ok", "osb": r["metrics"][t], "window": self.windows.get(t)}
            elif t not in self.tasks:
                # admin tasks (create-index, force-merge, ...) produce no op metrics
                self.tasks[t] = {"status": "no_metrics" if r["returncode"] == 0 else "failed",
                                 "window": self.windows.get(t)}
        if r.get("osb_results"):
            self.res.setdefault("osb_results", {}).update(r["osb_results"])

    def drain(self):
        """Wait until the server answers quickly again (it keeps computing abandoned queries);
        if it is still busy after --drain-timeout, restart the container (persistent data)."""
        t0 = time.time()
        ok = False
        while time.time() - t0 < self.a.drain_timeout and self.box.running():
            c = loadgen.Conn(self.t.port, timeout=5)
            s = time.time()
            st, _ = c.req("GET", "/_cluster/health")
            if st == 200 and time.time() - s < 1:
                ok = True
                break
            time.sleep(1)
        print(f"    (waited {time.time() - t0:.0f}s for the server to become responsive)", flush=True)
        if not ok and self.box.running() and self.t.persistent:
            print("    still busy: restarting the container", flush=True)
            self.box.stop(timeout=10)
            self.box.start()
            self.sampler.stop()
            old = self.sampler.samples
            self.sampler = Sampler(self.box.cgroup_dir())
            self.sampler.samples = list(old)
            self.sampler.start()
            self.res.setdefault("restarts", []).append(round(time.time(), 1))

    # ------------------------------------------------------------------ per target
    def target(self, t, setup: list[str], queries: list[str]):
        self.t = t
        self.box = make_box(t, self.a.cpus, self.a.memory, self.a.profile)
        self.tasks: dict[str, dict] = {}
        self.windows: dict[str, tuple[float, float]] = {}
        self.res = {"kind": "osb", "target": t.name, "image": t.image, "workload": self.a.workload,
                    "procedure": self.a.procedure, "preset": self.a.preset, "params": self.params,
                    "invocations": [], "started": dt.datetime.now().isoformat(timespec="seconds")}
        self.proxy = None
        self.t_start = time.time()
        print(f"== {t.name} ({t.image}) on :{t.port}", flush=True)
        try:
            self.res["startup_empty_s"] = round(self.box.create_and_start(), 2)
            self.sampler = Sampler(self.box.cgroup_dir())
            self.sampler.start()
            info = json.loads(loadgen.Conn(t.port).req("GET", "/")[1])
            prof = None
            if self.a.profile and t.kind != "ref":
                from native import Profiler
                prof = Profiler(self.box.server_pid(), self.out / f"profile-{self.a.workload}-{t.name}").start()
            self.res["version"] = info.get("version", {}).get("number")
            if self.a.proxy:
                self.proxy = Proxy(t.port, timeout=self.a.request_timeout + 30).start()
            t_setup = time.time()
            if self.preset["test_mode"] and not self.a.split:
                self.run_tasks(setup + queries, "all", self.a.task_timeout)
            else:
                self.run_tasks(setup, "setup", self.a.index_timeout)
                self.res["setup_resources"] = self.sampler.window(t_setup, time.time())
                if t.kind == "ref":
                    loadgen.Conn(t.port, timeout=600).req("POST", "/_flush")
                self.res["disk_after_index"] = self.box.disk_bytes()
                chunk = self.a.chunk or len(queries) or 1
                for i in range(0, len(queries), chunk):
                    self.run_tasks(queries[i:i + chunk], f"q{i // chunk}", self.a.task_timeout)
            if prof:
                self.res["profile"] = prof.stop()
                print(f"    profile: {self.res['profile']}", flush=True)
            self.res["disk_end"] = self.box.disk_bytes()
            self.res["oom_kills"] = self.sampler.oom_kills()
            if not self.box.running():
                st = self.box.state()
                self.res["died"] = {"oom_killed": st.get("OOMKilled"), "exit_code": st.get("ExitCode"),
                                    "logs": self.box.logs(30)}
            try:
                self.res["memory_peak_total"] = int((self.box.cgroup_dir() / "memory.peak").read_text())
            except Exception:  # noqa: BLE001
                self.res["memory_peak_total"] = None
            self.res["resources_total"] = self.sampler.window(t_setup, time.time())
            for name, rec in self.tasks.items():
                w = rec.get("window") or self.windows.get(name)
                if w:
                    rec["resources"] = self.sampler.window(w[0], w[1])
            # keep the schedule order
            order = setup + queries
            self.res["tasks"] = {n: self.tasks.get(n, {"status": "not_run"}) for n in order}
        finally:
            if self.proxy:
                self.res["calls"] = self.proxy.stop()["calls"]
            self.res["finished"] = dt.datetime.now().isoformat(timespec="seconds")
            (self.out / f"osb-{self.a.workload}-{t.name}.json").write_text(json.dumps(self.res, indent=1))
            if hasattr(self, "sampler"):
                self.sampler.stop()
            if not self.a.keep:
                self.box.remove()
        self.summary()

    def summary(self):
        for n, r in self.res.get("tasks", {}).items():
            o = r.get("osb") or {}
            thr = o.get("throughput", {})
            lat = o.get("latency", {})
            print(f"    {n:40s} {r['status']:10s} {thr.get('median') or '':>10} {thr.get('unit', '')}"
                  f"  p50={_r(lat.get('50_0', lat.get('100_0')))} p90={_r(lat.get('90_0'))} err={o.get('error_rate', '')}", flush=True)
        for c in self.res.get("calls", []):
            if c["errors"]:
                for e, info in c["errors"].items():
                    print(f"    REJECTED {c['endpoint']}: {info['count']}x {e[:160]}", flush=True)


def _r(x):
    return round(x, 1) if isinstance(x, (int, float)) else "-"


def run_meta(a, names, targets, extra=None) -> dict:
    from run import _cpu_model, _mem_gb
    m = {"kind": "osb", "date": dt.datetime.now().isoformat(timespec="seconds"), "targets": names,
         "cpus": a.cpus, "memory": a.memory, "heap": a.heap, "git_rev": git_rev(), "servers_rev": servers_rev(),
         "host": os.uname().nodename, "kernel": os.uname().release, "cpu_model": _cpu_model(),
         "host_cpus": os.cpu_count(), "host_mem_gb": _mem_gb(), "loadavg_start": os.getloadavg(),
         "images": {n: sh("docker", "image", "inspect", "-f", "{{.Id}}", targets[n].image, check=False)[:19]
                    for n in names}}
    m.update(extra or {})
    return m


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--targets", default="ref,go,rust")
    ap.add_argument("--workload", default="geonames")
    ap.add_argument("--procedure", help="OSB test procedure (default: the workload's default)")
    ap.add_argument("--preset", choices=list(PRESETS), default="test",
                    help="test = OSB --test-mode (1k docs, 1 iteration); sample = ingest_percentage=1, "
                         "2+10 iterations, no target throughput; full = workload defaults")
    ap.add_argument("--params", action="append", help="extra workload param k=v (JSON value), repeatable")
    ap.add_argument("--include-tasks", help="only these query tasks (comma list); setup always runs")
    ap.add_argument("--exclude-tasks", help="skip these query tasks (comma list)")
    ap.add_argument("--split", action="store_true", help="test preset: also split setup and queries")
    ap.add_argument("--proxy", action="store_true", help="route OSB through the recording proxy (compat report)")
    ap.add_argument("--task-timeout", type=float, default=600, help="kill OSB when one query task runs longer (s)")
    ap.add_argument("--index-timeout", type=float, default=3600, help="limit for one setup task (s)")
    ap.add_argument("--request-timeout", type=float, default=120, help="OSB client timeout per request (s)")
    ap.add_argument("--startup-timeout", type=float, default=3600,
                    help="limit until OSB starts the first task (includes downloading and decompressing the corpus)")
    ap.add_argument("--target-budget", type=float, default=0,
                    help="stop starting OSB invocations for a target after this many seconds; the remaining "
                         "tasks are recorded as not_run (0 = no limit)")
    ap.add_argument("--drain-timeout", type=float, default=120,
                    help="after a killed task, wait this long for the server, then restart its container (s)")
    ap.add_argument("--chunk", type=int, default=6,
                    help="query tasks per OSB invocation (a watchdog kill loses the chunk's finished tasks, "
                         "which are re-run; 0 = all in one)")
    ap.add_argument("--cpus", default=os.environ.get("BENCH_CPUS", "4"))
    ap.add_argument("--memory", default=os.environ.get("BENCH_MEMORY", "8g"))
    ap.add_argument("--heap", default=os.environ.get("BENCH_HEAP", "4g"))
    ap.add_argument("--out", type=Path, help="results directory (default bench/results/<timestamp>)")
    ap.add_argument("--level", default="adhoc", help="label stored in the scorecard (quick/full/adhoc)")
    ap.add_argument("--profile", action="store_true",
                    help="perf record the server (not ref) during the run; see bench/native.py")
    ap.add_argument("--no-build", action="store_true")
    ap.add_argument("--keep", action="store_true")
    ap.add_argument("--offline", action="store_true", help="OSB --offline (workloads and corpora already cached)")
    a = ap.parse_args()
    if not OSB_BIN.exists():
        ap.error(f"{OSB_BIN} not found: make bench-osb-setup (or set OSB_BIN)")

    targets = all_targets(a.heap)
    names = a.targets.split(",")
    for n in names:
        if n not in targets:
            ap.error(f"unknown target {n}")
    if not a.no_build:
        build_images({targets[n].image for n in names if targets[n].kind == "slop"})
    out = a.out or BENCH / "results" / dt.datetime.now().strftime("%Y%m%d-%H%M%S")
    out.mkdir(parents=True, exist_ok=True)
    sched = schedule(a.workload, a.procedure)
    setup, queries = split_setup(sched)
    if a.include_tasks:
        keep = a.include_tasks.split(",")
        queries = [q for q in queries if q in keep]
    if a.exclude_tasks:
        drop = set(a.exclude_tasks.split(","))
        queries = [q for q in queries if q not in drop]
    print(f"workload {a.workload}: setup {setup}\n  queries {queries}", flush=True)
    r = OsbRun(a, out)
    meta = run_meta(a, names, targets, {"workload": a.workload, "procedure": a.procedure, "preset": a.preset,
                                         "params": r.params, "level": a.level, "proxy": a.proxy,
                                         "task_timeout_s": a.task_timeout, "request_timeout_s": a.request_timeout,
                                         "osb_version": subprocess.run([str(OSB_BIN), "--version"], capture_output=True,
                                                                       text=True).stdout.strip()})
    mf = out / f"osb-run-{a.workload}.json"
    if mf.exists():
        # another osb.py call with the same workload into this directory (e.g. one target
        # re-run): keep one metadata file listing all targets
        old = json.loads(mf.read_text())
        meta["targets"] = list(dict.fromkeys(old.get("targets", []) + names))
        meta["loadavg_start"] = old.get("loadavg_start", meta["loadavg_start"])
        meta["date"] = old.get("date", meta["date"])
        meta["images"] = {**old.get("images", {}), **meta["images"]}
        meta["invocation_args"] = old.get("invocation_args", []) + [sys.argv[1:]]
    else:
        meta["invocation_args"] = [sys.argv[1:]]
    mf.write_text(json.dumps(meta, indent=1))
    for n in names:
        r.target(targets[n], setup, queries)
    meta["loadavg_end"] = os.getloadavg()
    meta["finished"] = dt.datetime.now().isoformat(timespec="seconds")
    mf.write_text(json.dumps(meta, indent=1))
    print(f"results: {out}")


if __name__ == "__main__":
    main()
