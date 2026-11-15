"""Recording HTTP proxy: sits between a benchmark client (OSB) and a target and records
every call (method, endpoint pattern, query parameters, status, error type/reason, bulk
item errors). Used to find out which OpenSearch APIs a workload needs and which of them a
server rejects; it adds latency, so timed runs normally go direct (bench/osb.py --proxy).

  python bench/proxy.py --listen 9600 --upstream 9591 --out calls.json   # standalone
"""

from __future__ import annotations

import argparse
import http.client
import json
import re
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qsl, urlsplit

HOP = {"connection", "keep-alive", "proxy-connection", "transfer-encoding", "te", "trailer", "upgrade",
       "content-length"}


def endpoint(method: str, path: str) -> str:
    """`PUT /geonames/_doc/12` -> `PUT /{index}/_doc/{id}`: API names (segments starting with
    `_`) are kept, the first other segment is the index, later ones ids/names."""
    segs = [s for s in path.split("/") if s]
    out, seen_api = [], False
    for i, s in enumerate(segs):
        if s.startswith("_"):
            out.append(s)
            seen_api = True
        elif i == 0:
            out.append("{index}")
        elif seen_api and out and out[-1] in ("_nodes", "_cat", "_cluster", "_tasks", "_snapshot", "_ingest",
                                              "_plugins", "_scripts", "_index_template", "_template",
                                              "_component_template", "_alias", "_aliases", "_mapping",
                                              "_settings", "_stats", "_search", "_data_stream"):
            out.append(s if re.fullmatch(r"[a-z_]+", s) and len(s) < 20 else "{name}")
        else:
            out.append("{id}")
    return f"{method} /" + "/".join(out)


class Recorder:
    def __init__(self):
        self.lock = threading.Lock()
        self.calls: dict[str, dict] = {}
        self.log: list[tuple] = []          # (t, endpoint, status, ms)

    def add(self, method, raw_path, status, ms, req_body: bytes, resp: bytes):
        u = urlsplit(raw_path)
        ep = endpoint(method, u.path)
        params = sorted({k for k, _ in parse_qsl(u.query, keep_blank_values=True)})
        err = None
        if status >= 400 or status < 0:
            err = _error_of(resp)
        elif ep.endswith("/_bulk") and b'"errors":true' in resp[:200].replace(b" ", b""):
            err = "bulk item errors: " + _bulk_error(resp)
        with self.lock:
            c = self.calls.setdefault(ep, {"endpoint": ep, "count": 0, "status": {}, "params": set(),
                                           "errors": {}, "example": raw_path[:200], "ms_total": 0.0})
            c["count"] += 1
            c["ms_total"] += ms
            c["status"][str(status)] = c["status"].get(str(status), 0) + 1
            c["params"].update(params)
            if err:
                e = c["errors"].setdefault(err[:300], {"count": 0, "example_path": raw_path[:300],
                                                       "example_body": req_body[:600].decode(errors="replace")})
                e["count"] += 1
            self.log.append((round(time.time(), 3), ep, status, round(ms, 2)))

    def dump(self) -> dict:
        with self.lock:
            calls = []
            for c in sorted(self.calls.values(), key=lambda c: c["endpoint"]):
                c = dict(c, params=sorted(c["params"]), ms_avg=round(c["ms_total"] / max(c["count"], 1), 2))
                c.pop("ms_total")
                calls.append(c)
            return {"calls": calls}


def _error_of(resp: bytes) -> str:
    try:
        d = json.loads(resp)
    except ValueError:
        return resp[:200].decode(errors="replace") or "(empty body)"
    if isinstance(d, dict):
        e = d.get("error")
        if isinstance(e, dict):
            rc = e.get("root_cause") or [{}]
            r0 = rc[0] if rc and isinstance(rc[0], dict) else {}
            return f"{e.get('type')}: {r0.get('reason') or e.get('reason')}"
        if e:
            return str(e)[:200]
        if "found" in d:
            return "not found"
    return json.dumps(d)[:200]


def _bulk_error(resp: bytes) -> str:
    try:
        for it in json.loads(resp).get("items", []):
            for v in it.values():
                if isinstance(v, dict) and v.get("error"):
                    e = v["error"]
                    return f"{e.get('type')}: {e.get('reason')}" if isinstance(e, dict) else str(e)
    except (ValueError, AttributeError):
        pass
    return "?"


def make_handler(upstream_host: str, upstream_port: int, rec: Recorder, timeout: float):
    local = threading.local()

    class H(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def log_message(self, *a):  # quiet
            pass

        def _conn(self, fresh=False):
            if fresh or getattr(local, "c", None) is None:
                local.c = http.client.HTTPConnection(upstream_host, upstream_port, timeout=timeout)
            return local.c

        def _body(self) -> bytes:
            if self.headers.get("Transfer-Encoding", "").lower() == "chunked":
                parts = []
                while True:
                    n = int(self.rfile.readline().split(b";")[0].strip() or b"0", 16)
                    if n == 0:
                        while self.rfile.readline() not in (b"\r\n", b"\n", b""):
                            pass
                        break
                    parts.append(self.rfile.read(n))
                    self.rfile.readline()
                return b"".join(parts)
            n = int(self.headers.get("Content-Length") or 0)
            return self.rfile.read(n) if n else b""

        def _forward(self):
            body = self._body()
            headers = {k: v for k, v in self.headers.items() if k.lower() not in HOP and k.lower() not in ("host", "accept-encoding")}
            t0 = time.perf_counter()
            status, rbody, rheaders = -1, b"", []
            for attempt in (0, 1):
                try:
                    c = self._conn(fresh=attempt == 1)
                    c.request(self.command, self.path, body=body if body or self.command in ("POST", "PUT") else None,
                              headers=headers)
                    r = c.getresponse()
                    status, rbody, rheaders = r.status, r.read(), r.getheaders()
                    break
                except (http.client.RemoteDisconnected, BrokenPipeError, ConnectionResetError) as e:
                    if attempt == 1:
                        rbody = json.dumps({"error": f"proxy: upstream {type(e).__name__}"}).encode()
                except (OSError, http.client.HTTPException) as e:
                    local.c = None
                    rbody = json.dumps({"error": f"proxy: upstream {type(e).__name__}: {e}"}).encode()
                    break
            ms = (time.perf_counter() - t0) * 1000
            rec.add(self.command, self.path, status, ms, body, rbody)
            if status < 0:
                status = 502
            self.send_response(status)
            for k, v in rheaders:
                if k.lower() not in HOP:
                    self.send_header(k, v)
            self.send_header("Content-Length", str(len(rbody)))
            self.end_headers()
            if self.command != "HEAD":
                self.wfile.write(rbody)

        do_GET = do_POST = do_PUT = do_DELETE = do_HEAD = do_PATCH = _forward

    return H


class Proxy:
    """Threaded proxy on 127.0.0.1:<port> (0 = any free port) in front of upstream."""

    def __init__(self, upstream_port: int, port: int = 0, upstream_host="127.0.0.1", timeout=600):
        self.rec = Recorder()
        self.srv = ThreadingHTTPServer(("127.0.0.1", port), make_handler(upstream_host, upstream_port, self.rec, timeout))
        self.srv.daemon_threads = True
        self.port = self.srv.server_address[1]
        self.th = threading.Thread(target=self.srv.serve_forever, daemon=True)

    def start(self):
        self.th.start()
        return self

    def stop(self) -> dict:
        self.srv.shutdown()
        return self.rec.dump()


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--listen", type=int, required=True)
    ap.add_argument("--upstream", type=int, required=True)
    ap.add_argument("--out", required=True)
    a = ap.parse_args()
    p = Proxy(a.upstream, a.listen).start()
    print(f"proxy :{p.port} -> :{a.upstream}; Ctrl-C writes {a.out}")
    try:
        while True:
            time.sleep(3600)
    except KeyboardInterrupt:
        pass
    with open(a.out, "w") as f:
        json.dump(p.stop(), f, indent=1)


if __name__ == "__main__":
    main()
