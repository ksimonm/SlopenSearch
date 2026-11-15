#!/bin/sh
# Create bench/.venv-osb with opensearch-benchmark and the client-side compat patches
# (bench/slop_osb_compat.py, active only with SLOP_OSB_COMPAT=1). Needs uv or python3.11.
set -e
cd "$(dirname "$0")"
if [ ! -x .venv-osb/bin/python ]; then
  if command -v uv >/dev/null; then uv venv -q -p 3.11 .venv-osb
  else python3.11 -m venv .venv-osb; fi
fi
if command -v uv >/dev/null; then uv pip install -q -p .venv-osb/bin/python -r requirements-osb.txt
else .venv-osb/bin/pip install -q -r requirements-osb.txt; fi
SP=$(.venv-osb/bin/python -c 'import sysconfig; print(sysconfig.get_paths()["purelib"])')
ln -sf "$PWD/slop_osb_compat.py" "$SP/slop_osb_compat.py"
echo "import slop_osb_compat" > "$SP/slop_osb_compat.pth"
.venv-osb/bin/opensearch-benchmark --version
