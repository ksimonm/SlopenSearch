#!/bin/sh
# CPU profile of a running Rust server with its built-in sampling profiler (no perf, no root).
#
#   scripts/profile.sh [-p PORT] [-f HZ] [-o DIR] -- COMMAND...   profile while COMMAND runs
#   scripts/profile.sh [-p PORT] [-f HZ] [-o DIR] -s SECONDS        profile for SECONDS
#
# Writes DIR/stacks.folded, DIR/top.txt (bench/proftop.py) and, with inferno installed
# (`make profiling-tools`), DIR/flamegraph.svg. DIR defaults to bench/results/profile-<time>.
# Drill down: python3 bench/proftop.py DIR/stacks.folded --callers <function>
set -e
port=9200 hz=999 out= secs=
while getopts p:f:o:s: opt; do
    case $opt in
        p) port=$OPTARG ;;
        f) hz=$OPTARG ;;
        o) out=$OPTARG ;;
        s) secs=$OPTARG ;;
        *) sed -n '2,10p' "$0"; exit 2 ;;
    esac
done
shift $((OPTIND - 1))
[ "$1" = "--" ] && shift
if [ -z "$secs" ] && [ $# -eq 0 ]; then
    sed -n '2,10p' "$0"
    exit 2
fi
root=$(cd "$(dirname "$0")/.." && pwd)
out=${out:-$root/bench/results/profile-$(date +%Y%m%d-%H%M%S)}
mkdir -p "$out"
url=http://127.0.0.1:$port/_slop/profile
curl -fsS -XPOST "$url/start?frequency=$hz" >/dev/null
stop() { curl -fsS -XPOST "$url/stop?format=folded" -o "$out/stacks.folded"; }
trap 'stop; exit 130' INT TERM
if [ -n "$secs" ]; then sleep "$secs"; else "$@" || true; fi
trap - INT TERM
stop
python3 "$root/bench/proftop.py" "$out/stacks.folded" > "$out/top.txt"
if command -v inferno-flamegraph >/dev/null; then
    inferno-flamegraph < "$out/stacks.folded" > "$out/flamegraph.svg"
fi
head -25 "$out/top.txt"
echo "profile: $out"
