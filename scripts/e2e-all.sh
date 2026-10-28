#!/bin/sh
# Runs every browser test against a fresh server and an empty database.
# Needs Go, Node 18+ and Playwright (npm i -D playwright@1 && npx playwright install chromium).
#
#   ./scripts/e2e-all.sh              # all suites
#   ./scripts/e2e-all.sh e2e-live     # only the ones named
set -u
cd "$(dirname "$0")/.."
PORT="${PORT:-8099}"
TMP="$(mktemp -d)"
BIN="$TMP/notehop"
trap 'kill "$PID" 2>/dev/null; rm -rf "$TMP"' EXIT

go build -o "$BIN" ./cmd/notehop || exit 1
SUITES="${*:-e2e e2e-features e2e-server-features e2e-files e2e-live e2e-blocks e2e-nearby e2e-integrations e2e-types e2e-recovery e2e-ai e2e-web e2e-home e2e-compose}"
FAILED=""
PID=""
for s in $SUITES; do
	[ -n "$PID" ] && kill "$PID" 2>/dev/null && wait "$PID" 2>/dev/null
	rm -f "$TMP"/test.db*
	"$BIN" -addr "127.0.0.1:$PORT" -db "$TMP/test.db" >"$TMP/server.log" 2>&1 &
	PID=$!
	sleep 1
	if NOTEHOP_URL="http://127.0.0.1:$PORT" NOTEHOP_BIN="$BIN" node "scripts/$s.js" >"$TMP/out.txt" 2>&1; then
		echo "ok   $s ($(grep -c '^PASS\|✔' "$TMP/out.txt") checks)"
	else
		echo "FAIL $s"
		grep -v '^PASS' "$TMP/out.txt" | head -20
		FAILED="$FAILED $s"
	fi
done
[ -z "$FAILED" ] || { echo "failed:$FAILED"; exit 1; }
