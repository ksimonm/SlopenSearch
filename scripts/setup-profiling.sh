#!/bin/sh
# One-time root setup for profiling on a Linux dev host (run: sudo sh scripts/setup-profiling.sh).
#  - perf for users, also on the Docker containers of bench/ (they run under another uid: -1)
#  - kernel symbols in perf reports
#  - valgrind (callgrind, cachegrind) and heaptrack (allocations) when apt is available
# The user-level part (inferno for flamegraphs) is `make profiling-tools`.
set -e
if [ "$(id -u)" != 0 ]; then
    echo "run as root: sudo sh $0" >&2
    exit 1
fi
cat > /etc/sysctl.d/60-slop-profiling.conf <<EOF
kernel.perf_event_paranoid = -1
kernel.kptr_restrict = 0
EOF
sysctl -q -p /etc/sysctl.d/60-slop-profiling.conf
echo "perf_event_paranoid=$(cat /proc/sys/kernel/perf_event_paranoid) kptr_restrict=$(cat /proc/sys/kernel/kptr_restrict)"
if command -v apt-get >/dev/null; then
    pkgs="valgrind heaptrack"
    command -v perf >/dev/null || pkgs="$pkgs linux-tools-common linux-tools-$(uname -r)"
    apt-get install -y $pkgs
fi
