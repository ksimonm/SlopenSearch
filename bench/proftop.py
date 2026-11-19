#!/usr/bin/env python3
"""Text summary of folded stacks (stacks.folded from bench/native.py or scripts/profile.sh).

  python bench/proftop.py stacks.folded                 top functions by self and total time
  python bench/proftop.py stacks.folded --callers f     who calls functions matching `f`
"""

from __future__ import annotations

import argparse
import sys
from collections import Counter
from pathlib import Path


def load(path: Path) -> list[tuple[list[str], int]]:
    out = []
    for line in path.read_text(errors="replace").splitlines():
        if not line.strip():
            continue
        stack, n = line.rsplit(" ", 1)
        out.append((stack.split(";"), int(n)))
    return out


def short(f: str) -> str:
    parts = f.split("::")
    return "::".join(parts[-2:]) if len(parts) > 2 else f


def report(path: Path, limit: int = 30) -> str:
    stacks = load(path)
    total = sum(n for _, n in stacks) or 1
    own, incl = Counter(), Counter()
    for frames, n in stacks:
        own[frames[-1]] += n
        for f in set(frames[1:]):
            incl[f] += n
    lines = [f"{total} samples", "", "self%  function"]
    lines += [f"{100 * n / total:5.1f}  {f[:160]}" for f, n in own.most_common(limit)]
    lines += ["", "total% function"]
    lines += [f"{100 * n / total:5.1f}  {f[:160]}" for f, n in incl.most_common(limit)]
    return "\n".join(lines) + "\n"


def callers(path: Path, pat: str, depth: int = 4, limit: int = 15) -> str:
    stacks = load(path)
    total = sum(n for _, n in stacks) or 1
    c = Counter()
    for frames, n in stacks:
        for i, f in enumerate(frames):
            if pat in f:
                c[" <- ".join(short(x) for x in frames[max(1, i - depth):i + 1][::-1])] += n
                break
    return "".join(f"{100 * n / total:5.1f}  {k}\n" for k, n in c.most_common(limit))


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("folded", type=Path)
    ap.add_argument("--callers", help="substring of a function name")
    ap.add_argument("--depth", type=int, default=4)
    ap.add_argument("--limit", type=int, default=30)
    a = ap.parse_args()
    sys.stdout.write(callers(a.folded, a.callers, a.depth, a.limit) if a.callers else report(a.folded, a.limit))


if __name__ == "__main__":
    main()
