#!/usr/bin/env python3
"""Benchmark levels: `make bench-quick` and `make bench-full`.

  python bench/levels.py quick [--ref auto|always|never] [--targets go,rust]
  python bench/levels.py full  [--ref auto|always|never] [--osb-workloads geonames,http_logs]

quick (a few minutes): our harness on the `tiny` corpus (index, 5 search types, updates) and
OSB geonames in --test-mode through the recording proxy, Go and Rust only.
full (hours): our harness on the `large` corpus (every workload) and the full OSB
workloads (default geonames with its own iteration counts and target throughputs).

The reference is expensive (JVM start, bigger runs), so by default (`--ref auto`) it only
runs when bench/history.jsonl has no reference record for the exact configuration (same
level parameters, limits); otherwise the latest reference record is the baseline.
Results go to bench/results/<timestamp>/ (+ <timestamp>-ref/ when the reference ran);
both are appended to bench/history.jsonl and the ratio report is printed and written to
bench/results/<timestamp>/scorecard.md.
"""

from __future__ import annotations

import argparse
import datetime as dt
import os
import subprocess
import sys
from pathlib import Path

BENCH = Path(__file__).resolve().parent
sys.path.insert(0, str(BENCH))
import scorecard  # noqa: E402

PY = sys.executable

LEVELS = {
    "quick": {
        "harness": ["--size", "tiny", "--workloads", "index,search,update",
                    "--queries", "match,filter_term_range,sort_price,agg_terms,geo_distance",
                    "--search-duration", "3"],
        "osb": [("geonames", ["--preset", "test", "--proxy"])],
    },
    "full": {
        "harness": ["--size", "large"],
        "osb": [("geonames", ["--preset", "full"]), ("http_logs", ["--preset", "full"])],
    },
}


def run(cmd: list[str]) -> int:
    print("+ " + " ".join(cmd), flush=True)
    return subprocess.call(cmd)


def one_pass(level: str, targets: str, out: Path, osb_workloads, a, build=True):
    L = LEVELS[level]
    common = ["--cpus", a.cpus, "--memory", a.memory, "--heap", a.heap, "--level", level]
    rc = 0
    if not a.skip_harness:
        rc |= run([PY, str(BENCH / "run.py"), "--targets", targets, "--out", str(out), *L["harness"], *common,
                   *([] if build else ["--no-build"])])
        build = False
    if not a.skip_osb:
        for wl, args in L["osb"]:
            if osb_workloads and wl not in osb_workloads:
                continue
            rc |= run([PY, str(BENCH / "osb.py"), "--targets", targets, "--workload", wl, "--out", str(out),
                       *args, *common, *([] if build else ["--no-build"])])
            build = False
    return rc


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("level", choices=list(LEVELS))
    ap.add_argument("--targets", default="go,rust")
    ap.add_argument("--ref", choices=["auto", "always", "never"], default="auto")
    ap.add_argument("--osb-workloads", help="comma list (default: all of the level)")
    ap.add_argument("--skip-harness", action="store_true")
    ap.add_argument("--skip-osb", action="store_true")
    ap.add_argument("--no-append", action="store_true", help="do not add the run to bench/history.jsonl")
    ap.add_argument("--cpus", default=os.environ.get("BENCH_CPUS", "4"))
    ap.add_argument("--memory", default=os.environ.get("BENCH_MEMORY", "8g"))
    ap.add_argument("--heap", default=os.environ.get("BENCH_HEAP", "4g"))
    a = ap.parse_args()
    os.environ.setdefault("BENCH_DATA_DIR", str(Path.home() / ".cache" / "slopbench" / "data"))
    wls = set(a.osb_workloads.split(",")) if a.osb_workloads else None
    ts = dt.datetime.now().strftime("%Y%m%d-%H%M%S")
    out = BENCH / "results" / ts
    rc = one_pass(a.level, a.targets, out, wls, a)
    recs = scorecard.build(out)
    hist = scorecard.history()
    need_ref = a.ref == "always" or (a.ref == "auto" and any(
        scorecard.baseline_for(r, recs, hist) is None for r in recs if r["target"] != "ref"))
    all_recs = list(recs)
    if need_ref and a.ref != "never":
        print("no reference baseline for this configuration in bench/history.jsonl: running ref", flush=True)
        ref_out = BENCH / "results" / f"{ts}-ref"
        rc |= one_pass(a.level, "ref", ref_out, wls, a, build=False)
        ref_recs = scorecard.build(ref_out)
        all_recs = ref_recs + recs
        if not a.no_append:
            scorecard.append(ref_recs)
    if not a.no_append:
        n = scorecard.append(recs)
        print(f"appended {n} records to {scorecard.HISTORY}")
    md = scorecard.report(recs, scorecard.history() if not a.no_append else hist + [r for r in all_recs if r["target"] == "ref"])
    (out / "scorecard.md").write_text(md + "\n")
    print(md)
    print(f"\nresults: {out}\nhistory: python bench/scorecard.py history")
    return rc


if __name__ == "__main__":
    sys.exit(main())
