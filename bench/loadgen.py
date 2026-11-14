"""Closed-loop load generator: N client processes, each with one keep-alive connection.

Processes (not threads) so that the Python client is not the bottleneck: JSON bodies are
prepared up front as bytes and responses are only inspected as far as needed.
"""

from __future__ import annotations

import http.client
import json
import multiprocessing as mp
import random
import time

HEADERS = {"Content-Type": "application/json"}
NDJSON = {"Content-Type": "application/x-ndjson"}


class Conn:
    def __init__(self, port: int, timeout: float = 300):
        self.port, self.timeout = port, timeout
        self.c: http.client.HTTPConnection | None = None

    def req(self, method: str, path: str, body: bytes | None = None, headers=HEADERS):
        """Returns (status, body bytes); status 0 = connection error, -1 = client timeout."""
        for attempt in range(2):
            if self.c is None:
                self.c = http.client.HTTPConnection("127.0.0.1", self.port, timeout=self.timeout)
            try:
                self.c.request(method, path, body, headers)
                r = self.c.getresponse()
                return r.status, r.read()
            except TimeoutError:
                self.c.close()
                self.c = None
                return -1, b"client timeout after %ds" % self.timeout
            except (OSError, http.client.HTTPException) as e:
                # a stale keep-alive connection is retried once
                self.c.close()
                self.c = None
                if attempt == 1:
                    return 0, str(e).encode()
        return 0, b""


class Stats:
    def __init__(self):
        self.tags: dict[str, dict] = {}
        self.first_error: str | None = None

    def add(self, tag: str, lat_ms: float, status: int, body: bytes | None = None, ok=None):
        t = self.tags.setdefault(tag, {"lat": [], "err": 0, "status": {}})
        t["lat"].append(lat_ms)
        t["status"][status] = t["status"].get(status, 0) + 1
        good = (200 <= status < 300) if ok is None else ok
        if not good:
            t["err"] += 1
            if self.first_error is None:
                self.first_error = f"{tag}: HTTP {status}: {(body or b'')[:400].decode(errors='replace')}"


def _bulk_batches(spec, w, nw):
    """Yield bulk bodies of this worker (batches w, w+nw, ...) built from the docs file."""
    b, n = spec["batch"], spec["max_docs"]
    index, action, off = spec["index"], spec.get("action", "index"), spec.get("id_offset", 0)
    buf, k = [], 0
    with open(spec["file"], "rb") as f:
        for i, line in enumerate(f):
            if i >= n:
                break
            if (i // b) % nw != w:
                continue
            doc_id = i + off
            if action == "update":
                buf.append(b'{"update":{"_index":"%s","_id":"%d"}}\n{"doc":{"quantity":%d,"rating":1.5}}\n'
                           % (index.encode(), doc_id, i % 997))
            else:
                buf.append(b'{"index":{"_index":"%s","_id":"%d"}}\n' % (index.encode(), doc_id))
                buf.append(line if line.endswith(b"\n") else line + b"\n")
            k += 1
            if k == b:
                yield b"".join(buf), k
                buf, k = [], 0
    if buf:
        yield b"".join(buf), k


def _bulk_ok(body: bytes) -> tuple[bool, int, str | None]:
    head = body[:200]
    if b'"errors":false' in head:
        return True, 0, None
    try:
        r = json.loads(body)
    except ValueError:
        return False, -1, body[:300].decode(errors="replace")
    bad = [it for it in r.get("items", []) for v in it.values() if v.get("status", 500) >= 300]
    return not bad, len(bad), json.dumps(bad[0])[:300] if bad else None


def _worker(args):
    port, spec, w, nw, start_at, duration, deadline, q = args
    rnd = random.Random(1000 + w)
    conn = Conn(port, spec.get("timeout", 300))
    st = Stats()
    extra = {"docs": 0, "item_errors": 0, "truncated": False}
    while time.time() < start_at:
        time.sleep(0.005)
    t0 = time.time()
    end = t0 + duration if duration else None
    kind = spec["type"]

    def timed(tag, method, path, body, headers=HEADERS):
        s = time.perf_counter()
        status, data = conn.req(method, path, body, headers)
        return (time.perf_counter() - s) * 1000, status, data

    if kind == "bulk":
        for body, k in _bulk_batches(spec, w, nw):
            if deadline and time.time() > deadline:
                extra["truncated"] = True
                break
            lat, status, data = timed("bulk", "POST", "/_bulk", body, NDJSON)
            ok, nbad, err = (False, -1, None) if status != 200 else _bulk_ok(data)
            if nbad > 0:
                extra["item_errors"] += nbad
                if st.first_error is None:
                    st.first_error = f"bulk item: {err}"
            st.add("bulk", lat, status, data, ok=status == 200 and nbad == 0)
            extra["docs"] += k
    elif kind == "list":
        ops = spec["ops"]
        if spec.get("once"):
            for i in range(w, len(ops), nw):
                tag, method, path, body = ops[i]
                lat, status, data = timed(tag, method, path, body)
                st.add(tag, lat, status, data, ok=_ok(tag, status, data))
        else:
            i = rnd.randrange(len(ops))
            while time.time() < end:
                tag, method, path, body = ops[i % len(ops)]
                i += 1
                lat, status, data = timed(tag, method, path, body)
                st.add(tag, lat, status, data, ok=_ok(tag, status, data))
    elif kind == "search_after":
        # one op = walk `pages` pages with search_after; latency recorded per page
        index, size, pages, queries = spec["index"], spec["size"], spec["pages"], spec["queries"]
        i = w
        while time.time() < end:
            q0 = dict(queries[i % len(queries)])
            i += nw
            after = None
            for _ in range(pages):
                body = dict(q0, size=size, sort=[{"seq": "asc"}])
                if after is not None:
                    body["search_after"] = after
                lat, status, data = timed("page", "POST", f"/{index}/_search", json.dumps(body).encode())
                st.add("page", lat, status, data)
                if status != 200:
                    break
                hits = json.loads(data)["hits"]["hits"]
                if not hits:
                    break
                after = hits[-1]["sort"]
    elif kind == "mixed":
        reads, wr = spec["reads"], spec["write_ratio"]
        pool = []
        with open(spec["file"], "rb") as f:
            for i, line in enumerate(f):
                if i >= 2000:
                    break
                pool.append(line.rstrip(b"\n"))
        index, n = spec["index"].encode(), 0
        while time.time() < end:
            if rnd.random() < wr:
                parts = []
                for _ in range(spec["write_batch"]):
                    parts.append(b'{"index":{"_index":"%s","_id":"mix-%d-%d"}}\n%s\n'
                                 % (index, w, n, pool[rnd.randrange(len(pool))]))
                    n += 1
                lat, status, data = timed("write", "POST", "/_bulk", b"".join(parts), NDJSON)
                ok = status == 200 and _bulk_ok(data)[0]
                st.add("write", lat, status, data, ok=ok)
            else:
                tag, method, path, body = reads[rnd.randrange(len(reads))]
                lat, status, data = timed("read", method, path, body)
                st.add("read", lat, status, data, ok=_ok("read", status, data))
    t1 = time.time()
    q.put({"w": w, "t0": t0, "t1": t1, "stats": st.tags, "first_error": st.first_error, **extra})


def _ok(tag, status, data):
    if status == 404 and tag == "delete":
        return True   # already deleted by another client: fine
    return 200 <= status < 300


def pct(lat: list[float], p: float) -> float | None:
    if not lat:
        return None
    s = sorted(lat)
    k = min(len(s) - 1, max(0, int(round(p / 100 * len(s) + 0.5)) - 1))
    return round(s[k], 2)


def run(port: int, spec: dict, clients: int, duration: float | None = None,
        deadline: float | None = None) -> dict:
    """Run `spec` with `clients` processes. Returns merged stats (latency in ms)."""
    ctx = mp.get_context("spawn")
    q = ctx.Queue()
    start_at = time.time() + 1.5
    procs = [ctx.Process(target=_worker, args=((port, spec, w, clients, start_at, duration, deadline, q),))
             for w in range(clients)]
    for p in procs:
        p.start()
    res = [q.get() for _ in procs]
    for p in procs:
        p.join()
    t0, t1 = min(r["t0"] for r in res), max(r["t1"] for r in res)
    wall = t1 - t0
    merged: dict[str, dict] = {}
    for r in res:
        for tag, s in r["stats"].items():
            m = merged.setdefault(tag, {"lat": [], "err": 0, "status": {}})
            m["lat"] += s["lat"]
            m["err"] += s["err"]
            for k, v in s["status"].items():
                m["status"][str(k)] = m["status"].get(str(k), 0) + v
    out = {"t0": t0, "t1": t1, "wall_s": round(wall, 3), "clients": clients, "ops": {},
           "docs": sum(r["docs"] for r in res), "item_errors": sum(r["item_errors"] for r in res),
           "truncated": any(r["truncated"] for r in res),
           "first_error": next((r["first_error"] for r in res if r["first_error"]), None)}
    for tag, m in merged.items():
        lat = m["lat"]
        out["ops"][tag] = {
            "count": len(lat), "errors": m["err"], "status": m["status"],
            "ops_per_s": round(len(lat) / wall, 2) if wall else None,
            "p50": pct(lat, 50), "p95": pct(lat, 95), "p99": pct(lat, 99),
            "max": round(max(lat), 2) if lat else None,
            "mean": round(sum(lat) / len(lat), 2) if lat else None,
        }
    if out["docs"]:
        out["docs_per_s"] = round(out["docs"] / wall, 1)
    return out
