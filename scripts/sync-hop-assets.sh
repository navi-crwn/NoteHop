#!/usr/bin/env bash
# Refresh the embedded copy of the HOP asset manifest from hel.ink, then rebuild.
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ -n "${1:-}" ]]; then
  cp "$1/public/hop-assets/apps.json" web/hopassets/apps.json.new
else
  curl -fsS https://hel.ink/hop-assets/apps.json -o web/hopassets/apps.json.new
fi
python3 -c 'import json,sys; json.load(open(sys.argv[1]))' web/hopassets/apps.json.new
mv web/hopassets/apps.json.new web/hopassets/apps.json
git --no-pager diff --stat -- web/hopassets/apps.json
