"""Benchmark targets: Docker containers with equal CPU/memory limits, plus a cgroup sampler.

Every target runs in its own container (bench-<name>) with its own named data volume
(bench-<name>-data) and host port, so the developer stack from compose.yaml (ports
9290-9292) is never touched.
"""

from __future__ import annotations

import http.client
import json
import subprocess
import threading
import time
from dataclasses import dataclass, field
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
REF_IMAGE = "opensearchproject/opensearch:3.9.0"
GO_IMAGE = "slopbench-go:latest"
RUST_IMAGE = "slopbench-rust:latest"


@dataclass
class Target:
    name: str
    image: str
    port: int
    data_path: str | None          # path of the data dir inside the container; None = in memory
    env: dict = field(default_factory=dict)
    kind: str = "slop"             # "ref" | "slop"
    persistent: bool = True
    native: str | None = None      # "go" | "rust": run the binary on the host (bench/native.py)

    @property
    def container(self) -> str:
        return f"bench-{self.name}"

    @property
    def volume(self) -> str:
        return f"bench-{self.name}-data"


def all_targets(heap: str) -> dict[str, Target]:
    return {
        "ref": Target("ref", REF_IMAGE, 9590, "/usr/share/opensearch/data", kind="ref", env={
            "cluster.name": "bench", "discovery.type": "single-node",
            "DISABLE_SECURITY_PLUGIN": "true", "DISABLE_INSTALL_DEMO_CONFIG": "true",
            "OPENSEARCH_JAVA_OPTS": f"-Xms{heap} -Xmx{heap}",
        }),
        "go": Target("go", GO_IMAGE, 9591, "/data"),
        "rust": Target("rust", RUST_IMAGE, 9592, "/data"),
        "go-mem": Target("go-mem", GO_IMAGE, 9593, None, env={"SLOP_IN_MEMORY": "true"}, persistent=False),
        "rust-mem": Target("rust-mem", RUST_IMAGE, 9594, None, env={"SLOP_IN_MEMORY": "true"}, persistent=False),
        "go-native": Target("go-native", "native:go", 9595, "data", native="go"),
        "rust-native": Target("rust-native", "native:rust", 9596, "data", native="rust"),
    }


def make_box(t: Target, cpus: str, memory: str, profile: bool = False):
    """A Container, or a NativeProcess (host binary in a systemd scope) for *-native targets."""
    if t.native:
        from native import NativeProcess
        return NativeProcess(t, cpus, memory, profile)
    return Container(t, cpus, memory)


def sh(*args, check=True, capture=True, timeout=None) -> str:
    r = subprocess.run(list(args), capture_output=capture, text=True, timeout=timeout)
    if check and r.returncode != 0:
        raise RuntimeError(f"{' '.join(args)} failed: {r.stderr.strip()}")
    return (r.stdout or "").strip()


def build_images(only: set[str] | None = None) -> None:
    """Build the Go and Rust images from this checkout under the bench tags."""
    for img, ctx in ((GO_IMAGE, "servers/go"), (RUST_IMAGE, "servers/rust")):
        if only is not None and img not in only:
            continue
        print(f"building {img} from {ctx}", flush=True)
        sh("docker", "build", "-q", "-t", img, str(REPO / ctx), timeout=3600)


def git_rev() -> str:
    try:
        return sh("git", "-C", str(REPO), "rev-parse", "--short", "HEAD")
    except Exception:
        return "?"


class Container:
    def __init__(self, t: Target, cpus: str, memory: str):
        self.t, self.cpus, self.memory = t, cpus, memory
        self.cid: str | None = None

    # --- lifecycle -------------------------------------------------------------------
    def remove(self) -> None:
        sh("docker", "rm", "-f", "-v", self.t.container, check=False)
        sh("docker", "volume", "rm", "-f", self.t.volume, check=False)

    def create_and_start(self) -> float:
        """Fresh container + empty volume; returns seconds until the HTTP API answers."""
        self.remove()
        args = ["docker", "run", "-d", "--name", self.t.container,
                "--cpus", self.cpus, "--memory", self.memory, "--memory-swap", self.memory,
                "--ulimit", "nofile=65536:65536", "--ulimit", "memlock=-1:-1",
                "-p", f"127.0.0.1:{self.t.port}:9200"]
        if self.t.data_path:
            args += ["-v", f"{self.t.volume}:{self.t.data_path}"]
        for k, v in self.t.env.items():
            args += ["-e", f"{k}={v}"]
        args.append(self.t.image)
        t0 = time.time()
        self.cid = sh(*args)
        return self.wait_ready(t0)

    def stop(self, timeout=60) -> float:
        t0 = time.time()
        sh("docker", "stop", "-t", str(timeout), self.t.container, timeout=timeout + 30)
        return time.time() - t0

    def start(self) -> float:
        t0 = time.time()
        sh("docker", "start", self.t.container)
        return self.wait_ready(t0)

    def wait_ready(self, t0: float, timeout=600) -> float:
        while time.time() - t0 < timeout:
            try:
                c = http.client.HTTPConnection("127.0.0.1", self.t.port, timeout=5)
                path = "/_cluster/health?wait_for_status=yellow&timeout=1s" if self.t.kind == "ref" else "/"
                c.request("GET", path)
                r = c.getresponse()
                r.read()
                if r.status == 200:
                    return time.time() - t0
            except OSError:
                pass
            if not self.running():
                raise RuntimeError(f"{self.t.container} exited:\n{self.logs(40)}")
            time.sleep(0.1)
        raise RuntimeError(f"{self.t.container} not ready after {timeout}s")

    def server_pid(self) -> int:
        """Host PID of the container's main process (for perf)."""
        return int(sh("docker", "inspect", "-f", "{{.State.Pid}}", self.t.container))

    def running(self) -> bool:
        return sh("docker", "inspect", "-f", "{{.State.Running}}", self.t.container, check=False) == "true"

    def state(self) -> dict:
        out = sh("docker", "inspect", "-f", "{{json .State}}", self.t.container, check=False)
        try:
            return json.loads(out)
        except ValueError:
            return {}

    def logs(self, n=50) -> str:
        r = subprocess.run(["docker", "logs", "--tail", str(n), self.t.container],
                           capture_output=True, text=True)
        return (r.stdout + r.stderr)[-6000:]

    def disk_bytes(self) -> int | None:
        if not self.t.data_path:
            return None
        out = sh("docker", "exec", self.t.container, "du", "-sk", self.t.data_path, check=False)
        try:
            return int(out.split()[0]) * 1024
        except (ValueError, IndexError):
            return None

    # --- cgroup ----------------------------------------------------------------------
    def cgroup_dir(self) -> Path:
        cid = sh("docker", "inspect", "-f", "{{.Id}}", self.t.container)
        for p in (Path(f"/sys/fs/cgroup/system.slice/docker-{cid}.scope"),
                  Path(f"/sys/fs/cgroup/docker/{cid}")):
            if p.exists():
                return p
        raise RuntimeError("cgroup v2 directory of the container not found")


def _read_kv(p: Path) -> dict[str, int]:
    out = {}
    for line in p.read_text().splitlines():
        k, _, v = line.partition(" ")
        try:
            out[k] = int(v)
        except ValueError:
            pass
    return out


class Sampler(threading.Thread):
    """Polls the container's cgroup: CPU usage (cores) and memory (usage - inactive_file,
    the number `docker stats` shows, plus anonymous memory)."""

    def __init__(self, cg: Path, interval=0.25):
        super().__init__(daemon=True)
        self.cg, self.interval = cg, interval
        self.samples: list[tuple[float, float, int, int]] = []   # t, cpu_usec, mem, anon
        self._stop = threading.Event()
        self.lock = threading.Lock()

    def read(self):
        cpu = _read_kv(self.cg / "cpu.stat")["usage_usec"]
        cur = int((self.cg / "memory.current").read_text())
        ms = _read_kv(self.cg / "memory.stat")
        return time.time(), cpu, cur - ms.get("inactive_file", 0), ms.get("anon", 0)

    def run(self):
        while not self._stop.is_set():
            try:
                s = self.read()
                with self.lock:
                    self.samples.append(s)
            except (OSError, KeyError, ValueError):
                pass   # container restarting
            self._stop.wait(self.interval)

    def stop(self):
        self._stop.set()

    def oom_kills(self) -> int:
        try:
            return _read_kv(self.cg / "memory.events").get("oom_kill", 0)
        except OSError:
            return 0

    def window(self, t0: float, t1: float) -> dict:
        """CPU (in cores: 1.0 = one core busy) and memory stats between t0 and t1."""
        with self.lock:
            s = [x for x in self.samples if t0 - self.interval <= x[0] <= t1 + self.interval]
        if len(s) < 2:
            return {}
        cpu = []
        for a, b in zip(s, s[1:]):
            dt = b[0] - a[0]
            if dt > 0 and b[1] >= a[1]:
                cpu.append((b[1] - a[1]) / 1e6 / dt)
        total_cpu_s = (s[-1][1] - s[0][1]) / 1e6
        wall = s[-1][0] - s[0][0]
        mem = [x[2] for x in s]
        anon = [x[3] for x in s]
        return {
            "cpu_avg": round(total_cpu_s / wall, 3) if wall > 0 else None,
            "cpu_peak": round(max(cpu), 3) if cpu else None,
            "cpu_seconds": round(total_cpu_s, 2),
            "mem_avg": int(sum(mem) / len(mem)),
            "mem_peak": max(mem),
            "anon_peak": max(anon),
            "mem_end": mem[-1],
        }
