#!/usr/bin/env python3
"""Markdown report from a results directory: python bench/report.py bench/results/<ts> [-o out.md]"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import workloads as W  # noqa: E402


def mib(b):
    return "-" if b is None else f"{b / 2**20:,.0f}"


def num(x, fmt="{:,.1f}"):
    return "-" if x is None else fmt.format(x)


def load(d: Path):
    run = json.loads((d / "run.json").read_text())
    res = {}
    for n in run["targets"]:
        p = d / f"{n}.json"
        if p.exists():
            res[n] = json.loads(p.read_text())
    return run, res


def render(d: Path) -> str:
    run, res = load(Path(d))
    names = [n for n in run["targets"] if n in res]
    data = run["data"]
    L = []
    L.append(f"# SlopenSearch benchmark — {run['date'][:10]}\n")
    L.append(f"- size **{run['size']}**: {data['docs']:,} text documents "
             f"({run['docs_file_bytes'] / 1e9:.2f} GB NDJSON), {data['vectors']:,} vectors × {data['dim']} dims")
    L.append(f"- every target alone in Docker with `--cpus {run['cpus']} --memory {run['memory']}` (no swap); "
             f"reference OpenSearch heap {run['heap']}")
    L.append(f"- clients: bulk/update {run['clients']} (batch {run['batch']}), search {run['search_clients']}, "
             f"mixed {run['mixed_clients']}; {run['search_duration_s']} s per search workload")
    L.append(f"- host `{run['host']}`: {run['cpu_model']}, {run['host_cpus']} CPUs, {run['host_mem_gb']} GB RAM, "
             f"kernel {run['kernel']}; load average at start {run['loadavg_start'][0]:.1f}"
             + (f", at end {run['loadavg_end'][0]:.1f}" if run.get("loadavg_end") else ""))
    L.append(f"- git `{run['git_rev']}`; targets: " + ", ".join(
        f"{n} ({res[n].get('version') or '?'})" for n in names) + "\n")

    def w(n, k):
        return res[n]["workloads"].get(k, {})

    # ---------------------------------------------------------------- overview
    L.append("## Indexing\n")
    L.append("| | " + " | ".join(names) + " |")
    L.append("|---|" + "---:|" * len(names))
    rows = [
        ("startup, empty (s)", lambda n: num(res[n].get("startup_empty_s"), "{:.2f}")),
        ("text bulk (docs/s)", lambda n: num(w(n, "index_text").get("docs_per_s"), "{:,.0f}")),
        ("text bulk p50 / p99 per batch (ms)", lambda n: _pp(w(n, "index_text"), "bulk")),
        ("text docs indexed", lambda n: _docs(res[n], data)),
        ("refresh after bulk (s)", lambda n: num(w(n, "refresh_text").get("seconds"), "{:.2f}")),
        ("vector bulk (docs/s)", lambda n: num(w(n, "index_vectors").get("docs_per_s"), "{:,.0f}")),
        ("vector refresh (s)", lambda n: num(w(n, "refresh_vectors").get("seconds"), "{:.2f}")),
        ("CPU during text bulk (avg / peak cores)", lambda n: _cpu(w(n, "index_text"))),
        ("memory peak during text bulk (MiB)", lambda n: mib(w(n, "index_text").get("resources", {}).get("mem_peak"))),
        ("memory idle after indexing (MiB)", lambda n: mib(w(n, "idle_after_index").get("resources", {}).get("mem_avg"))),
        ("memory peak, whole run (MiB)", lambda n: mib(_peak(res[n]))),
        ("on-disk size after indexing (MiB)", lambda n: mib(res[n].get("disk_after_index"))),
        ("OOM-killed", lambda n: "**yes**" if (res[n].get("died") or {}).get("oom_killed")
         or res[n].get("oom_kills") else "no"),
    ]
    for label, f in rows:
        L.append(f"| {label} | " + " | ".join(f(n) for n in names) + " |")
    L.append("")

    sweep = sorted({k for n in names for k in res[n]["workloads"] if k.startswith("sweep_")},
                   key=lambda k: (int(k.split("_")[1][1:]), int(k.split("_")[2][1:])))
    if sweep:
        L.append("### Bulk batch size / client sweep (docs/s, separate index)\n")
        L.append("| batch × clients | " + " | ".join(names) + " |")
        L.append("|---|" + "---:|" * len(names))
        for k in sweep:
            _, b, c = k.split("_")
            L.append(f"| {b[1:]} × {c[1:]} | " + " | ".join(
                num(w(n, k).get("docs_per_s"), "{:,.0f}") + _err(w(n, k)) for n in names) + " |")
        L.append("")

    # ---------------------------------------------------------------- search
    sk = [k for k in _ordered_keys(res, names) if k.startswith("search_")]
    if sk:
        L.append("## Search throughput (ops/s) and latency p50 / p95 / p99 (ms)\n")
        L.append(f"{run['search_clients']} concurrent clients, closed loop. `!n` = n failed requests; "
                 f"*too slow* = a single probe request took longer than {run.get('slow_limit_s', 10)} s, "
                 "so the timed run was skipped (the probe time is shown). Correctness is in its own section.\n")
        L.append("| workload | " + " | ".join(f"{n} ops/s | {n} p50/p95/p99" for n in names) + " |")
        L.append("|---|" + "---:|---:|" * len(names))
        for k in sk:
            cells = []
            for n in names:
                o = next(iter(w(n, k).get("ops", {}).values()), None)
                if w(n, k).get("skipped_slow"):
                    cells += ["too slow", _probe(w(n, k))]
                    continue
                if not o:
                    cells += [("error" if "error" in w(n, k) else "-"), "-"]
                    continue
                e = f" !{o['errors']}" if o["errors"] else ""
                cells += [f"{o['ops_per_s']:,.1f}{e}", f"{o['p50']} / {o['p95']} / {o['p99']}"]
            L.append(f"| {k[7:]} | " + " | ".join(cells) + " |")
        L.append("")
        L.append("### CPU (avg cores) and memory peak (MiB) during search\n")
        L.append("| workload | " + " | ".join(names) + " |")
        L.append("|---|" + "---:|" * len(names))
        for k in sk:
            L.append(f"| {k[7:]} | " + " | ".join(
                f"{num(w(n, k).get('resources', {}).get('cpu_avg'), '{:.2f}')} / "
                f"{mib(w(n, k).get('resources', {}).get('mem_peak'))}" for n in names) + " |")
        L.append("")

    # ---------------------------------------------------------------- mixed / updates
    other = [("mixed_rw", "read"), ("mixed_rw", "write"), ("update_single", "update"),
             ("delete_single", "delete"), ("bulk_update", "bulk"), ("delete_by_query", "delete_by_query")]
    if any(w(n, k) for n in names for k, _ in other):
        L.append("## Mixed load, updates and deletes\n")
        L.append(f"mixed: {run['mixed_clients']} clients, 80 % searches / 20 % bulk writes of 10 docs. "
                 f"Updates/deletes: {run['clients']} clients, single-document requests; bulk_update in batches "
                 f"of {run['batch']} (docs/s); delete_by_query sequential, one user each.\n")
        L.append("| workload | " + " | ".join(f"{n} ops/s | {n} p50/p99 | {n} CPU/MiB" for n in names) + " |")
        L.append("|---|" + "---:|---:|---:|" * len(names))
        for k, tag in other:
            cells = []
            for n in names:
                r = w(n, k)
                o = r.get("ops", {}).get(tag)
                if r.get("skipped_slow"):
                    cells += ["too slow", _probe(r), "-"]
                    continue
                if not o:
                    cells += ["error" if "error" in r else "-", "-", "-"]
                    continue
                e = f" !{o['errors']}" if o["errors"] else ""
                rate = f"{r['docs_per_s']:,.0f} docs/s" if tag == "bulk" and r.get("docs_per_s") else f"{o['ops_per_s']:,.1f}"
                rs = r.get("resources", {})
                cells += [rate + e, f"{o['p50']} / {o['p99']}",
                          f"{num(rs.get('cpu_avg'), '{:.2f}')} / {mib(rs.get('mem_peak'))}"]
            label = f"{k} ({tag})" if k == "mixed_rw" else k
            L.append(f"| {label} | " + " | ".join(cells) + " |")
        L.append("")

    # ---------------------------------------------------------------- restart
    if any(res[n].get("restart") for n in names):
        L.append("## Restart (persistent targets)\n")
        L.append("Graceful `docker stop`, then `docker start`; *data available* = until `_count` on the "
                 "text index answers.\n")
        L.append("| | " + " | ".join(names) + " |")
        L.append("|---|" + "---:|" * len(names))
        rr = {n: res[n].get("restart") or {} for n in names}
        for label, key, fmt in (("on-disk size (MiB)", "disk_bytes", None), ("stop (s)", "stop_s", "{:.2f}"),
                                ("start until HTTP ready (s)", "ready_s", "{:.2f}"),
                                ("start until data available (s)", "data_available_s", "{:.2f}"),
                                ("first query after restart (ms)", "first_query_ms", "{:.1f}")):
            L.append(f"| {label} | " + " | ".join(
                (mib(rr[n].get(key)) if fmt is None else num(rr[n].get(key), fmt)) if rr[n] else "-"
                for n in names) + " |")
        L.append("| docs after restart / before | " + " | ".join(
            f"{rr[n].get('count_after')} / {rr[n].get('count_expected')}" if rr[n] else "-" for n in names) + " |")
        L.append("| memory after restart (MiB) | " + " | ".join(
            mib((rr[n].get("resources_after") or {}).get("mem_avg")) if rr[n] else "-" for n in names) + " |")
        L.append("")

    # ---------------------------------------------------------------- correctness
    L.append("## Correctness (compared with the first target)\n")
    base = names[0]
    L.append(f"Sequential pass over 10 queries per workload with `track_total_hits: true`. "
             f"*totals* = queries whose hit count equals {base}'s; *top ids* = mean overlap of the "
             f"returned ids with {base}'s (score ties may legitimately reorder); *aggs* = identical "
             f"aggregation results (floats to 4 digits). k-NN: recall@10 against exact brute-force "
             f"neighbours.\n")
    L.append("| workload | " + " | ".join(names[1:]) + " |")
    L.append("|---|" + "---|" * (len(names) - 1))
    bv = res[base].get("verify", {})
    for k, items in bv.items():
        cells = []
        for n in names[1:]:
            ov = res[n].get("verify", {}).get(k)
            cells.append(_cmp(items, ov) if ov is not None else "-")
        L.append(f"| {k} | " + " | ".join(cells) + " |")
    L.append("")
    L.append("| | " + " | ".join(names) + " |")
    L.append("|---|" + "---:|" * len(names))
    for key in ("knn", "knn_filter"):
        L.append(f"| recall@10 {key} | " + " | ".join(num(res[n].get("recall", {}).get(key), "{:.3f}") for n in names) + " |")
    for key in ("text_after_index", "vec_after_index", "removed_by_update_workload", "rating_4_5_after_updates"):
        L.append(f"| count {key} | " + " | ".join(str(res[n].get("counts", {}).get(key, "-")) for n in names) + " |")
    L.append("")

    # ---------------------------------------------------------------- problems
    probs = []
    for n in names:
        r = res[n]
        if r.get("died"):
            probs.append(f"- **{n}** container died (OOMKilled={r['died'].get('oom_killed')}, "
                         f"exit {r['died'].get('exit_code')})")
        for k, e in (r.get("errors") or {}).items():
            probs.append(f"- **{n}** step {k}: `{e[:300]}`")
        for k, x in r["workloads"].items():
            if "error" in x:
                probs.append(f"- **{n}** {k}: `{x['error'][:300]}`")
            elif x.get("first_error"):
                probs.append(f"- **{n}** {k}: first failed request: `{x['first_error'][:300]}`")
            if x.get("truncated"):
                probs.append(f"- **{n}** {k}: stopped at the time limit after {x.get('docs', 0):,} docs")
        if r.get("skipped"):
            probs.append(f"- **{n}** skipped: {', '.join(r['skipped'])}")
    if probs:
        L.append("## Errors and anomalies\n")
        L += probs
        L.append("")
    return "\n".join(L) + "\n"


def _ordered_keys(res, names):
    keys = []
    for n in names:
        for k in res[n]["workloads"]:
            if k not in keys:
                keys.append(k)
    return keys


def _peak(r):
    vals = [x.get("resources", {}).get("mem_peak") for x in r["workloads"].values()]
    vals.append(r.get("memory_peak_total"))
    vals = [v for v in vals if v]
    return max(vals) if vals else None


def _probe(r):
    st = r.get("probe_status")
    return f"1 query: {r['probe_ms'] / 1000:,.1f} s" + (" (timeout)" if st == -1 else "")


def _close(a, b) -> bool:
    """Same structure, numbers within 0.1 % (float vs double rounding)."""
    if isinstance(a, dict):
        return isinstance(b, dict) and a.keys() == b.keys() and all(_close(a[k], b[k]) for k in a)
    if isinstance(a, list):
        return isinstance(b, list) and len(a) == len(b) and all(_close(x, y) for x, y in zip(a, b))
    if isinstance(a, (int, float)) and isinstance(b, (int, float)) and not isinstance(a, bool):
        return a == b or abs(a - b) <= 1e-3 * max(abs(a), abs(b))
    return a == b


def _pp(r, tag):
    o = r.get("ops", {}).get(tag)
    return f"{o['p50']} / {o['p99']}" if o else "-"


def _cpu(r):
    rs = r.get("resources", {})
    return f"{num(rs.get('cpu_avg'), '{:.2f}')} / {num(rs.get('cpu_peak'), '{:.2f}')}" if rs else "-"


def _docs(r, data):
    n = r.get("text_docs_indexed")
    if n is None:
        return "-"
    return f"{n:,}" + ("" if n == data["docs"] else " (incomplete)")


def _err(r):
    o = r.get("ops", {}).get("bulk")
    return f" !{o['errors']}" if o and o["errors"] else ""


def _cmp(base: list, other: list) -> str:
    n = min(len(base), len(other))
    if n == 0:
        return "-"
    errs = sum(1 for o in other[:n] if "status" in o)
    tot = sum(1 for b, o in zip(base, other) if "total" in b and b.get("total") == o.get("total"))
    has_tot = sum(1 for b in base[:n] if "total" in b)
    ov = []
    for b, o in zip(base, other):
        bi, oi = b.get("ids") or [], o.get("ids") or []
        if bi or oi:
            ov.append(len(set(bi) & set(oi)) / max(len(bi), len(oi)))
    agg = [W.agg_hash(b["aggs"]) == W.agg_hash(o.get("aggs")) for b, o in zip(base, other) if "aggs" in b]
    near = [_close(b["aggs"], o.get("aggs")) for b, o in zip(base, other) if "aggs" in b]
    parts = []
    if has_tot:
        parts.append(f"totals {tot}/{has_tot}")
    if ov:
        parts.append(f"top ids {sum(ov) / len(ov):.0%}")
    if agg:
        parts.append(f"aggs {sum(agg)}/{len(agg)}")
        if sum(near) > sum(agg):
            parts.append(f"{sum(near)}/{len(near)} within 0.1 %")
    if errs:
        parts.append(f"**{errs} errors**")
    bad = (has_tot and tot < has_tot) or (near and sum(near) < len(near)) or errs
    s = ", ".join(parts)
    return f"**{s}**" if bad and not errs else s


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    ap.add_argument("dir", type=Path)
    ap.add_argument("-o", "--output", type=Path)
    a = ap.parse_args()
    md = render(a.dir)
    if a.output:
        a.output.write_text(md)
    else:
        print(md)
