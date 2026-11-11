#!/usr/bin/env bash
# Refresh the embedded copy of the HOP asset manifest and the vendored v<N> assets from hel.ink, then rebuild.
# The version comes from the manifest ("version": "v9"), never from this script, so the next
# version needs no edit here. With a HEL.ink checkout as $1 nothing is fetched from the network.
set -euo pipefail
cd "$(dirname "$0")/.."
fetch() { # fetch <path under hop-assets/> <destination>
  mkdir -p "$(dirname "$2")"
  if [[ -n "${HUB:-}" ]]; then cp "$HUB/public/hop-assets/$1" "$2"; else curl -fsS "https://hel.ink/hop-assets/$1" -o "$2"; fi
}
HUB="${1:-}"
fetch apps.json web/hopassets/apps.json.new
python3 -c 'import json,sys; json.load(open(sys.argv[1]))' web/hopassets/apps.json.new
version=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' web/hopassets/apps.json.new)
[[ "$version" =~ ^v[0-9]+$ ]] || { echo "bad manifest version: $version" >&2; exit 1; }
mv web/hopassets/apps.json.new web/hopassets/apps.json
for file in hop-bento.css hop-home.css hop-home.js hop-hero3d.js hop-motion.js; do
  fetch "$version/$file" "web/static/hop-home/$file"
done
# the home page tiles: one light and one dark picture for every app in the manifest
for slug in $(python3 -c 'import json; print(" ".join(a["slug"] for a in json.load(open("web/hopassets/apps.json"))["apps"]))'); do
  for mode in light dark; do
    if [[ -n "$HUB" ]]; then [[ -f "$HUB/public/hop-assets/$version/home/$slug-$mode.webp" ]] || continue; fi
    fetch "$version/home/$slug-$mode.webp" "web/static/hop-home/home/$slug-$mode.webp" 2>/dev/null || true
  done
done
fetch "$version/hop-brand.css" web/static/hop-brand/hop-brand.css
# the name typefaces NoteHop shows itself (its own and the family list's)
for f in notehop-name.woff2 payhop-name.woff2; do fetch "$version/fonts/brand/$f" "web/static/hop-brand/fonts/brand/$f"; done
git --no-pager diff --stat -- web/hopassets/apps.json
