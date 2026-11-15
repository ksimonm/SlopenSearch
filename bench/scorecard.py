#!/usr/bin/env python3
"""Scorecard: one normalized result format for bench/run.py and bench/osb.py runs, ratios
against the reference OpenSearch, and the history across commits.

  python bench/scorecard.py build   bench/results/<ts>      # -> <ts>/scorecard.json
  python bench/scorecard.py report  bench/results/<ts>      # ratios vs ref (same run, else history)
  python bench/scorecard.py append  bench/results/<ts>      # add the run to bench/history.jsonl
  python bench/scorecard.py history [--target go] [--config osb:geonames] [--last 30]

A scorecard record is one target in one benchmark configuration (see bench/README.md):

  {"schema": 1, "run": "20261115-181500", "config": "osb:geonames:sample:1a2b3c:4cpu-8g",
   "kind": "osb", "workload": "geonames", "preset": "sample", "level": "quick",
   "commit": "3f75a11", "date": "...", "host": {...}, "loadavg": [start, end],
   "limits": {"cpus": "4", "memory": "8g", "heap": "4g"}, "target": "go", "image": "sha256:...",
   "tasks": {"term": {"status": "ok", "thr": 12.3, "unit": "ops/s", "p50": 80.1, "p90": 95.0,
                      "p99": 120.0, "err": 0.0, "cpu": 0.98, "cpu_s_op": 0.08, "mem": 123456789}},
   "resources": {"cpu_avg": .., "cpu_peak": .., "mem_avg": .., "mem_peak": .., "disk_bytes": ..},
   "scalars": {"startup_s": .., ...}}       # scalars: lower is better

Ratios are always oriented so that > 1.0 means the SlopenSearch server is better:
throughput ours/ref; latency, CPU seconds per operation, memory, disk and scalars ref/ours.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import sys
from pathlib import Path

BENCH = Path(__file__).resolve().parent
HISTORY = BENCH / "history.jsonl"
SCHEMA = 1


# ---------------------------------------------------------------------------- normalize
def _r(x, nd=4):
    """Round to nd significant digits (compact history lines)."""
    if not isinstance(x, (int, float)) or isinstance(x, bool) or x is None:
        return x
    if x == 0 or not math.isfinite(x):
        return x
    return round(x, max(0, nd - 1 - int(math.floor(math.log10(abs(x))))))


def _clean(d: dict) -> dict:
    return {k: (_r(v) if isinstance(v, float) else v) for k, v in d.items() if v is not None}


def _hash(obj) -> str:
    return hashlib.sha1(json.dumps(obj, sort_keys=True).encode()).hexdigest()[:6]


def _limits(meta) -> dict:
    return {"cpus": str(meta.get("cpus")), "memory": str(meta.get("memory")), "heap": str(meta.get("heap"))}


def _common(meta: dict, run: str) -> dict:
    la0, la1 = meta.get("loadavg_start"), meta.get("loadavg_end")
    return {"schema": SCHEMA, "run": run, "commit": meta.get("git_rev"), "servers_commit": meta.get("servers_rev"),
            "date": meta.get("date"),
            "host": {"name": meta.get("host"), "cpu": meta.get("cpu_model"), "cpus": meta.get("host_cpus"),
                     "mem_gb": meta.get("host_mem_gb"), "kernel": meta.get("kernel")},
            "loadavg": [round(la0[0], 2) if la0 else None, round(la1[0], 2) if la1 else None],
            "limits": _limits(meta)}


def _res(rs: dict | None) -> dict:
    rs = rs or {}
    return {"cpu": rs.get("cpu_avg"), "mem": rs.get("mem_peak"), "cpu_s": rs.get("cpu_seconds")}


def harness_records(d: Path, meta: dict) -> list[dict]:
    out = []
    data = meta.get("data", {})
    cfg_obj = {"size": meta.get("size"), "docs": data.get("docs"), "vectors": data.get("vectors"),
               "workloads": meta.get("workloads"), "clients": meta.get("clients"), "batch": meta.get("batch"),
               "search_clients": meta.get("search_clients"), "dur": meta.get("search_duration_s")}
    cfg = f"harness:{meta.get('size')}:{_hash(cfg_obj)}:{meta.get('cpus')}cpu-{meta.get('memory')}"
    for name in meta.get("targets", []):
        f = d / f"{name}.json"
        if not f.exists():
            continue
        r = json.loads(f.read_text())
        tasks = {}
        for w, x in r.get("workloads", {}).items():
            if x.get("skipped_slow"):
                tasks[w] = {"status": "too_slow", "probe_ms": x.get("probe_ms")}
                continue
            if "error" in x and not x.get("ops"):
                tasks[w] = {"status": "failed", "error": str(x["error"])[:200]}
                continue
            ops = x.get("ops") or {}
            res = _res(x.get("resources"))
            if "seconds" in x and not ops:          # refresh steps
                tasks[w] = _clean({"status": "ok", "secs": x["seconds"]})
                continue
            for tag, o in ops.items():
                key = w if len(ops) == 1 else f"{w}:{tag}"
                thr, unit = o.get("ops_per_s"), "ops/s"
                if x.get("docs_per_s") and len(ops) == 1:
                    thr, unit = x["docs_per_s"], "docs/s"
                t = {"status": "ok" if not x.get("crash") else "crashed", "thr": thr, "unit": unit,
                     "p50": o.get("p50"), "p90": None, "p95": o.get("p95"), "p99": o.get("p99"),
                     "err": round(o["errors"] / o["count"], 4) if o.get("count") else None,
                     "n": o.get("count"), **res}
                if thr and res.get("cpu") is not None and len(ops) == 1:
                    t["cpu_s_op"] = res["cpu"] / thr
                tasks[key] = _clean(t)
        whole = [x.get("resources", {}) for x in r.get("workloads", {}).values() if x.get("resources")]
        rst = r.get("restart") or {}
        rec = {**_common(meta, d.name), "config": cfg, "kind": "harness", "workload": f"harness-{meta.get('size')}",
               "preset": meta.get("size"), "level": meta.get("level", "adhoc"), "target": name,
               "image": meta.get("images", {}).get(name), "version": r.get("version"),
               "params": cfg_obj, "tasks": tasks,
               "resources": _clean({"cpu_avg": _avg([w.get("cpu_avg") for w in whole]),
                                    "cpu_peak": max([w.get("cpu_peak") or 0 for w in whole], default=None),
                                    "mem_avg": _avg([w.get("mem_avg") for w in whole]),
                                    "mem_peak": r.get("memory_peak_total"),
                                    "disk_bytes": r.get("disk_after_index")}),
               "scalars": _clean({"startup_s": r.get("startup_empty_s"),
                                  "restart_available_s": rst.get("data_available_s"),
                                  "first_query_after_restart_ms": rst.get("first_query_ms"),
                                  "disk_bytes": r.get("disk_after_index"),
                                  "mem_peak": r.get("memory_peak_total")}),
               "oom": bool(r.get("oom_kills")) or bool((r.get("died") or {}).get("oom_killed"))}
        if r.get("recall"):
            rec["recall"] = r["recall"]
        out.append(rec)
    return out


def _max(*xs):
    xs = [x for x in xs if isinstance(x, (int, float))]
    return max(xs) if xs else None


def _avg(xs):
    xs = [x for x in xs if isinstance(x, (int, float))]
    return sum(xs) / len(xs) if xs else None


def osb_records(d: Path, meta: dict) -> list[dict]:
    out = []
    wl = meta["workload"]
    cfg_obj = {"preset": meta.get("preset"), "params": meta.get("params"), "procedure": meta.get("procedure")}
    cfg = f"osb:{wl}:{meta.get('preset')}:{_hash(cfg_obj)}:{meta.get('cpus')}cpu-{meta.get('memory')}"
    for name in meta.get("targets", []):
        f = d / f"osb-{wl}-{name}.json"
        if not f.exists():
            continue
        r = json.loads(f.read_text())
        tasks = {}
        for tn, t in (r.get("tasks") or {}).items():
            o = t.get("osb") or {}
            thr, lat = o.get("throughput") or {}, o.get("service_time") or o.get("latency") or {}
            res = _res(t.get("resources"))
            if t["status"] == "no_metrics":
                continue          # admin tasks without samples
            x = {"status": t["status"], "thr": thr.get("median", thr.get("mean")), "unit": thr.get("unit"),
                 "p50": lat.get("50_0", lat.get("100_0") if lat else None), "p90": lat.get("90_0"),
                 "p99": lat.get("99_0"), "err": o.get("error_rate"), "dur_s": o.get("duration"), **res}
            if o.get("error_rate") == 1.0:
                x["status"] = "error"
                x["thr"] = None
            if x["thr"] and res.get("cpu") is not None:
                x["cpu_s_op"] = res["cpu"] / x["thr"]
            tasks[tn] = _clean(x)
        tot = r.get("resources_total") or {}
        rec = {**_common(meta, d.name), "config": cfg, "kind": "osb", "workload": wl, "preset": meta.get("preset"),
               "level": meta.get("level", "adhoc"), "target": name, "image": meta.get("images", {}).get(name),
               "version": r.get("version"), "params": cfg_obj, "proxy": bool(meta.get("proxy")), "tasks": tasks,
               "resources": _clean({"cpu_avg": tot.get("cpu_avg"), "cpu_peak": tot.get("cpu_peak"),
                                    "mem_avg": tot.get("mem_avg"), "mem_peak": _max(r.get("memory_peak_total"), tot.get("mem_peak")),
                                    "disk_bytes": r.get("disk_after_index")}),
               "scalars": _clean({"startup_s": r.get("startup_empty_s"), "disk_bytes": r.get("disk_after_index"),
                                  "mem_peak": _max(r.get("memory_peak_total"), tot.get("mem_peak"))}),
               "oom": bool(r.get("oom_kills")) or bool((r.get("died") or {}).get("oom_killed"))}
        rejected = [f"{c['endpoint']} {e}" for c in r.get("calls", []) for e in c.get("errors", {})]
        if rejected:
            rec["rejected_calls"] = rejected[:40]
        out.append(rec)
    return out


def build(d: Path) -> list[dict]:
    recs = []
    if (d / "run.json").exists():
        meta = json.loads((d / "run.json").read_text())
        recs += harness_records(d, meta)
    for mf in sorted(d.glob("osb-run-*.json")):
        recs += osb_records(d, json.loads(mf.read_text()))
    (d / "scorecard.json").write_text(json.dumps(recs, indent=1))
    return recs


def load(p: Path) -> list[dict]:
    if p.is_dir():
        return json.loads((p / "scorecard.json").read_text()) if (p / "scorecard.json").exists() else build(p)
    return json.loads(p.read_text())


def history(path: Path = HISTORY) -> list[dict]:
    if not path.exists():
        return []
    return [json.loads(line) for line in path.read_text().splitlines() if line.strip()]


def append(recs: list[dict], path: Path = HISTORY) -> int:
    old = history(path)
    have = {(r["run"], r["config"], r["target"]) for r in old}
    new = [r for r in recs if (r["run"], r["config"], r["target"]) not in have]
    with path.open("a") as f:
        for r in new:
            r = {k: v for k, v in r.items() if k not in ("host",)} | {"host": (r.get("host") or {}).get("name")}
            # compact: drop per-task fields that the ratios do not use
            r["tasks"] = {n: {k: v for k, v in t.items() if k not in ("cpu_s", "dur_s", "n", "p95")}
                          for n, t in r.get("tasks", {}).items()}
            f.write(json.dumps(r, separators=(",", ":")) + "\n")
    return len(new)


# ---------------------------------------------------------------------------- ratios
def baseline_for(rec: dict, same_run: list[dict], hist: list[dict]) -> dict | None:
    """The ref record of the same configuration: from the same run, else the latest earlier one."""
    for r in same_run:
        if r["target"] == "ref" and r["config"] == rec["config"]:
            return r
    refs = [r for r in hist if r["target"] == "ref" and r["config"] == rec["config"]]
    before = [r for r in refs if (r.get("date") or "") <= (rec.get("date") or "~")]
    # the latest reference run before this one, else the first one after it (a level that
    # had no baseline yet runs the reference right after the SlopenSearch targets)
    return before[-1] if before else (refs[0] if refs else None)


def ratios(rec: dict, ref: dict) -> dict:
    """Per task: thr, p50, p90, p99, cpu_s_op, mem ratios (>1 = ours better) + statuses."""
    rows = {}
    for tn, rt in ref.get("tasks", {}).items():
        t = rec.get("tasks", {}).get(tn)
        row = {"ref_status": rt.get("status"), "status": (t or {}).get("status", "missing"),
               "ref_err": rt.get("err"), "err": (t or {}).get("err")}
        if t and t.get("status") == "ok" and rt.get("status") == "ok":
            row["thr"] = _div(t.get("thr"), rt.get("thr"))
            for k in ("p50", "p90", "p99", "cpu_s_op", "mem"):
                row[k] = _div(rt.get(k), t.get(k))
        rows[tn] = row
    summ = {}
    for k in ("thr", "p50", "p90", "p99", "cpu_s_op"):
        summ[k] = geomean([r.get(k) for r in rows.values()])
    for k in ("mem_peak", "disk_bytes"):
        summ[k] = _div((ref.get("resources") or {}).get(k), (rec.get("resources") or {}).get(k))
    summ["tasks_ok"] = sum(1 for r in rows.values() if r["status"] == "ok")
    summ["tasks_ref_ok"] = sum(1 for r in rows.values() if r["ref_status"] == "ok")
    summ["tasks"] = len(rows)
    return {"tasks": rows, "summary": summ}


def _div(a, b):
    if isinstance(a, (int, float)) and isinstance(b, (int, float)) and a > 0 and b > 0:
        return a / b
    return None


def geomean(xs):
    xs = [x for x in xs if isinstance(x, (int, float)) and x > 0]
    return math.exp(sum(math.log(x) for x in xs) / len(xs)) if xs else None


def fr(x, nd=2):
    if x is None:
        return "-"
    if x >= 100:
        return f"{x:,.0f}"
    if x >= 0.01:
        return f"{x:.{nd}f}"
    return f"{x:.1e}"


def report(recs: list[dict], hist: list[dict]) -> str:
    out = ["Ratios vs the reference OpenSearch: **> 1.0 = SlopenSearch better**. thr = throughput "
           "ours/ref; p50/p90/p99 = latency ref/ours (OSB: service time); cpu/op = CPU seconds per operation ref/ours; "
           "mem = peak memory ref/ours. `geomean` over the tasks both completed.", ""]
    configs = []
    for r in recs:
        if r["config"] not in configs:
            configs.append(r["config"])
    for cfg in configs:
        group = [r for r in recs if r["config"] == cfg]
        g0 = group[0]
        out.append(f"### {g0['kind']} {g0['workload']} ({g0['preset']}) — `{cfg}`")
        out.append("")
        out.append(f"commit `{g0.get('commit')}` (servers `{g0.get('servers_commit')}`), {g0.get('date')}, limits {g0['limits']['cpus']} CPUs / "
                   f"{g0['limits']['memory']}, load average {g0['loadavg'][0]} → {g0['loadavg'][1]}"
                   + (", through the recording proxy" if g0.get("proxy") else ""))
        out.append("")
        others = [r for r in group if r["target"] != "ref"]
        refs = {r["target"]: baseline_for(r, group, hist) for r in others}
        for r in others:
            b = refs[r["target"]]
            if b is None:
                out.append(f"*{r['target']}: no reference baseline for this configuration*\n")
                continue
            if b not in group:
                out.append(f"{r['target']}: reference baseline from run `{b['run']}` (commit `{b.get('commit')}`, "
                           f"load {b['loadavg'][0]})")
                out.append("")
        if not any(refs.values()):
            continue
        rt = {t: ratios(r, refs[r['target']]) for t, r in ((r['target'], r) for r in others) if refs[r['target']]}
        names = list(rt)
        out.append("| summary | " + " | ".join(names) + " |")
        out.append("|---|" + "---:|" * len(names))
        for k, label in (("thr", "throughput (geomean)"), ("p50", "latency p50 (geomean)"),
                         ("p90", "latency p90 (geomean)"), ("p99", "latency p99 (geomean)"),
                         ("cpu_s_op", "CPU per op (geomean)"), ("mem_peak", "memory peak"),
                         ("disk_bytes", "disk after indexing")):
            out.append(f"| {label} | " + " | ".join(fr(rt[n]["summary"][k]) for n in names) + " |")
        out.append("| tasks ok (ours / ref / all) | " + " | ".join(
            f"{rt[n]['summary']['tasks_ok']} / {rt[n]['summary']['tasks_ref_ok']} / {rt[n]['summary']['tasks']}"
            for n in names) + " |")
        out.append("")
        ref0 = refs[names[0]]
        out.append("| task | ref thr | ref p50 ms | " + " | ".join(f"{n} thr× | {n} p50× | {n} p99×" for n in names) + " |")
        out.append("|---|---:|---:|" + "---:|---:|---:|" * len(names))
        for tn, t in ref0["tasks"].items():
            cells = []
            for n in names:
                row = rt[n]["tasks"].get(tn, {})
                if row.get("status") != "ok":
                    st = row.get("status", "missing")
                    if row.get("err"):
                        st += f" (err {row['err']:.0%})"
                    cells += [st, "", ""]
                else:
                    cells += [fr(row.get("thr")), fr(row.get("p50")), fr(row.get("p99"))]
            u = (t.get("unit") or "").replace("/s", "")
            out.append(f"| {tn} | {fr(t.get('thr'))} {u if t.get('thr') else ''} | {fr(t.get('p50'))} | "
                       + " | ".join(cells) + " |")
        out.append("")
        ref_recs = [r for r in group if r["target"] == "ref"] + [b for b in refs.values() if b]
        ref_rej = {" ".join(c.split(" ", 2)[:2]) for r in ref_recs for c in r.get("rejected_calls", [])}
        rej = {r["target"]: [c for c in r.get("rejected_calls", []) if " ".join(c.split(" ", 2)[:2]) not in ref_rej]
               for r in group if r["target"] != "ref"}
        if any(rej.values()):
            out.append("Calls answered with an error that the reference answered without one (recording proxy):")
            out.append("")
            for n, calls in rej.items():
                for c in calls:
                    out.append(f"- **{n}** `{c[:220]}`")
            out.append("")
    return "\n".join(out)


def history_view(hist: list[dict], target=None, config=None, last=30) -> str:
    rows = []
    for r in hist:
        if r["target"] == "ref" or (target and r["target"] != target) or (config and config not in r["config"]):
            continue
        b = baseline_for(r, [x for x in hist if x["run"] == r["run"]], hist)
        if not b:
            continue
        s = ratios(r, b)["summary"]
        rows.append((r, b, s))
    rows = rows[-last:]
    out = ["| date | servers commit | level | config | target | thr× | p50× | p99× | mem× | ok/ref ok | load | ref run |",
           "|---|---|---|---|---|---:|---:|---:|---:|---|---:|---|"]
    for r, b, s in rows:
        out.append(f"| {(r.get('date') or '')[:16]} | {r.get('servers_commit') or r.get('commit')} | {r.get('level')} | {r['config']} | {r['target']} | "
                   f"{fr(s['thr'])} | {fr(s['p50'])} | {fr(s['p99'])} | {fr(s['mem_peak'])} | "
                   f"{s['tasks_ok']}/{s['tasks_ref_ok']} | {r['loadavg'][0]} | {b['run'] if b['run'] != r['run'] else 'same'} |")
    return "\n".join(out)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    for c in ("build", "report", "append"):
        p = sub.add_parser(c)
        p.add_argument("path", type=Path)
        p.add_argument("--history", type=Path, default=HISTORY)
        if c == "report":
            p.add_argument("--no-write", action="store_true")
    p = sub.add_parser("history")
    p.add_argument("--history", type=Path, default=HISTORY)
    p.add_argument("--target")
    p.add_argument("--config")
    p.add_argument("--last", type=int, default=30)
    a = ap.parse_args()
    if a.cmd == "build":
        recs = build(a.path)
        print(f"{len(recs)} records -> {a.path / 'scorecard.json'}")
    elif a.cmd == "report":
        recs = build(a.path) if a.path.is_dir() else load(a.path)
        md = report(recs, history(a.history))
        print(md)
        if a.path.is_dir() and not a.no_write:
            (a.path / "scorecard.md").write_text(md + "\n")
    elif a.cmd == "append":
        recs = build(a.path) if a.path.is_dir() else load(a.path)
        n = append(recs, a.history)
        print(f"appended {n} records to {a.history}")
    else:
        print(history_view(history(a.history), a.target, a.config, a.last))


if __name__ == "__main__":
    sys.exit(main())
