"""Native (non-Docker) targets and CPU profiling.

go-native / rust-native run the server binary built from this checkout directly on the host,
inside a transient systemd user scope with the same limits as the containers
(`systemd-run --user --scope -p CPUQuota=<cpus*100>% -p MemoryMax=<memory> -p MemorySwapMax=0`),
so the cgroup sampler works unchanged. Data lives in ~/.cache/slopbench/native/<name>.

Profiling (`--profile` in bench/run.py and bench/osb.py) attaches `perf record -g` to the
server process for the whole run (native targets, and Docker targets via the container's
host PID) and writes perf.data, perf.script, stacks.folded and, when `inferno-flamegraph`
(cargo install inferno) or `flamegraph.pl` (FlameGraph) is on PATH, flamegraph.svg.
perf needs `kernel.perf_event_paranoid` <= 1, -1 for Docker targets (Ubuntu defaults to 4):
`sudo sh scripts/setup-profiling.sh` sets it for good.
perf.script can also be opened directly in https://www.speedscope.app.

Without perf, the Rust server profiles itself: its built-in sampling profiler
(`/_slop/profile/start|stop`, servers/rust/src/profiler.rs) writes the same stacks.folded and
flamegraph.svg, plus top.txt (bench/proftop.py). Go has no fallback yet.
"""

from __future__ import annotations

import http.client
import os
import shutil
import signal
import subprocess
import time
from collections import Counter
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
NATIVE_HOME = Path(os.environ.get("BENCHMARK_HOME", Path.home() / ".cache" / "slopbench")) / "native"


def build_native(lang: str, profile: bool = False) -> Path:
    """Build the server binary; with profile=True keep symbols and frame pointers."""
    if lang == "go":
        out = NATIVE_HOME / "bin" / "slopensearch-go"
        out.parent.mkdir(parents=True, exist_ok=True)
        flags = [] if profile else ["-trimpath", "-ldflags=-s -w"]
        subprocess.run(["go", "build", *flags, "-o", str(out), "."], cwd=REPO / "servers" / "go", check=True,
                       env=dict(os.environ, CGO_ENABLED="0"))
        return out
    tdir = REPO / "servers" / "rust" / ("target-bench-prof" if profile else "target-bench")
    env = dict(os.environ, CARGO_TARGET_DIR=str(tdir))
    if profile:
        env["RUSTFLAGS"] = (env.get("RUSTFLAGS", "") + " -C force-frame-pointers=yes -g").strip()
    subprocess.run(["cargo", "build", "--release", "-q"], cwd=REPO / "servers" / "rust", check=True, env=env)
    return tdir / "release" / "slopensearch"


class NativeProcess:
    """Same interface as targets.Container, for a host process in a systemd scope."""

    def __init__(self, t, cpus: str, memory: str, profile: bool = False):
        self.t, self.cpus, self.memory, self.profile_build = t, cpus, memory, profile
        self.p: subprocess.Popen | None = None
        self.data = NATIVE_HOME / t.name
        self.unit = f"slopbench-{t.name}"
        self.logf = NATIVE_HOME / f"{t.name}.log"
        self.bin: Path | None = None
        self._state: dict = {}

    def remove(self) -> None:
        subprocess.run(["systemctl", "--user", "stop", f"{self.unit}.scope"], capture_output=True)
        if self.p and self.p.poll() is None:
            self.p.kill()
            self.p.wait()
        shutil.rmtree(self.data, ignore_errors=True)

    def _spawn(self):
        mem = self.memory.upper().rstrip("B")
        env = dict(os.environ, HOST="127.0.0.1", PORT=str(self.t.port), SLOP_DATA_DIR=str(self.data), **self.t.env)
        self.data.mkdir(parents=True, exist_ok=True)
        cmd = ["systemd-run", "--user", "--scope", "--quiet", f"--unit={self.unit}",
               "-p", f"CPUQuota={int(float(self.cpus) * 100)}%", "-p", f"MemoryMax={mem}", "-p", "MemorySwapMax=0",
               str(self.bin)]
        self.p = subprocess.Popen(cmd, env=env, stdout=self.logf.open("ab"), stderr=subprocess.STDOUT,
                                  start_new_session=True)

    def create_and_start(self) -> float:
        self.remove()
        if self.bin is None:
            self.bin = build_native(self.t.native, self.profile_build)
        t0 = time.time()
        self._spawn()
        return self.wait_ready(t0)

    def stop(self, timeout=60) -> float:
        t0 = time.time()
        if self.p and self.p.poll() is None:
            os.kill(self.server_pid(), signal.SIGTERM)
            try:
                self.p.wait(timeout)
            except subprocess.TimeoutExpired:
                self.p.kill()
                self.p.wait()
        return time.time() - t0

    def start(self) -> float:
        t0 = time.time()
        self._spawn()
        return self.wait_ready(t0)

    def wait_ready(self, t0: float, timeout=600) -> float:
        while time.time() - t0 < timeout:
            try:
                c = http.client.HTTPConnection("127.0.0.1", self.t.port, timeout=5)
                c.request("GET", "/")
                r = c.getresponse()
                r.read()
                if r.status == 200:
                    return time.time() - t0
            except OSError:
                pass
            if not self.running():
                raise RuntimeError(f"{self.unit} exited:\n{self.logs(40)}")
            time.sleep(0.1)
        raise RuntimeError(f"{self.unit} not ready after {timeout}s")

    def running(self) -> bool:
        return self.p is not None and self.p.poll() is None

    def server_pid(self) -> int:
        return self.p.pid   # systemd-run --scope execs the command in place

    def state(self) -> dict:
        ev = {}
        try:
            for line in (self.cgroup_dir() / "memory.events").read_text().splitlines():
                k, v = line.split()
                ev[k] = int(v)
        except (OSError, RuntimeError):
            pass
        return {"OOMKilled": ev.get("oom_kill", 0) > 0, "ExitCode": self.p.poll() if self.p else None}

    def logs(self, n=50) -> str:
        try:
            return "\n".join(self.logf.read_text(errors="replace").splitlines()[-n:])[-6000:]
        except OSError:
            return ""

    def disk_bytes(self) -> int | None:
        if self.t.data_path is None:
            return None
        return sum(f.stat().st_size for f in self.data.rglob("*") if f.is_file())

    def cgroup_dir(self) -> Path:
        uid = os.getuid()
        p = Path(f"/sys/fs/cgroup/user.slice/user-{uid}.slice/user@{uid}.service/app.slice/{self.unit}.scope")
        if p.exists():
            return p
        if self.p:
            rel = Path(f"/proc/{self.p.pid}/cgroup").read_text().split("::", 1)[1].strip()
            return Path("/sys/fs/cgroup" + rel)
        raise RuntimeError("cgroup of the native target not found")


class Profiler:
    """perf record -g on one PID; stop() writes perf.script, stacks.folded, flamegraph.svg."""

    def __init__(self, pid: int, out: Path, freq: int = 99, port: int | None = None):
        self.pid, self.out, self.freq, self.port = pid, out, freq, port
        self.p: subprocess.Popen | None = None
        self.error: str | None = None
        self.builtin = False

    def _builtin(self, why: str):
        """The server's own profiler (Rust) when perf cannot run."""
        if self.port is not None:
            st, body = _http(self.port, "POST", "/_slop/profile/start?frequency=499")
            if st == 200:
                self.builtin = True
                return self
            why += f"; built-in profiler: {st} {body[:80]!r}"
        self.error = why
        return self

    def start(self):
        if not shutil.which("perf"):
            return self._builtin("perf not installed")
        par = int(Path("/proc/sys/kernel/perf_event_paranoid").read_text())
        if par > 1 and os.geteuid() != 0:
            return self._builtin(f"kernel.perf_event_paranoid={par}: run `sudo sh scripts/setup-profiling.sh` to profile")
        self.out.mkdir(parents=True, exist_ok=True)
        self.p = subprocess.Popen(["perf", "record", "-F", str(self.freq), "-g", "-p", str(self.pid),
                                   "-o", str(self.out / "perf.data")],
                                  stdout=subprocess.DEVNULL, stderr=(self.out / "perf-record.log").open("w"))
        return self

    def stop(self) -> dict:
        if self.builtin:
            return self._stop_builtin()
        if self.error or not self.p:
            return {"error": self.error}
        self.p.send_signal(signal.SIGINT)
        try:
            self.p.wait(60)
        except subprocess.TimeoutExpired:
            self.p.kill()
        script = self.out / "perf.script"
        with script.open("w") as f:
            subprocess.run(["perf", "script", "-i", str(self.out / "perf.data")], stdout=f,
                           stderr=subprocess.DEVNULL)
        folded = self.out / "stacks.folded"
        n = fold(script, folded)
        res = {"dir": str(self.out), "samples": n, "folded": str(folded)}
        tool = shutil.which("inferno-flamegraph") or shutil.which("flamegraph.pl")
        if tool and n:
            with (self.out / "flamegraph.svg").open("w") as f:
                subprocess.run([tool, str(folded)], stdout=f)
            res["svg"] = str(self.out / "flamegraph.svg")
        return res


    def _stop_builtin(self) -> dict:
        st, body = _http(self.port, "POST", "/_slop/profile/stop?format=folded", timeout=300)
        if st != 200:
            return {"error": f"built-in profiler: {st} {body[:200]!r}"}
        self.out.mkdir(parents=True, exist_ok=True)
        folded = self.out / "stacks.folded"
        folded.write_bytes(body)
        n = sum(int(line.rsplit(b" ", 1)[1]) for line in body.splitlines() if line.strip())
        res = {"dir": str(self.out), "samples": n, "folded": str(folded), "profiler": "builtin"}
        tool = shutil.which("inferno-flamegraph") or shutil.which("flamegraph.pl")
        if tool and n:
            with (self.out / "flamegraph.svg").open("w") as f:
                subprocess.run([tool, str(folded)], stdout=f)
            res["svg"] = str(self.out / "flamegraph.svg")
        if n:
            import proftop
            (self.out / "top.txt").write_text(proftop.report(folded))
            res["top"] = str(self.out / "top.txt")
        return res


def _http(port: int, method: str, path: str, timeout: float = 30) -> tuple[int, bytes]:
    try:
        c = http.client.HTTPConnection("127.0.0.1", port, timeout=timeout)
        c.request(method, path)
        r = c.getresponse()
        return r.status, r.read()
    except OSError as e:
        return 0, str(e).encode()


def fold(script: Path, out: Path) -> int:
    """`perf script` output -> folded stacks (what stackcollapse-perf.pl does, simplified)."""
    stacks: Counter = Counter()
    comm, frames = None, []

    def flush():
        if comm is not None and frames:
            stacks[";".join([comm] + frames[::-1])] += 1

    with script.open(errors="replace") as f:
        for line in f:
            if not line.strip():
                flush()
                comm, frames = None, []
            elif not line[0].isspace():
                comm = line.split()[0]
                frames = []
            else:
                parts = line.strip().split(None, 1)
                if len(parts) == 2:
                    sym = parts[1].rsplit(" (", 1)[0]
                    sym = sym.split("+0x")[0]
                    frames.append(sym.replace(";", ":"))
    flush()
    with out.open("w") as f:
        for k, v in stacks.most_common():
            f.write(f"{k} {v}\n")
    return sum(stacks.values())
