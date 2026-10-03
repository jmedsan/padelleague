#!/usr/bin/env bash
# Applies the pending migrations to a PocketBase data dir: migrations run when
# the app boots, so boot ./padelleague on a free port against the dir, wait
# until /healthz answers, then stop it. Usage: scripts/migrate-dir.sh <data-dir>
set -euo pipefail

dir=${1:?usage: scripts/migrate-dir.sh <data-dir>}
[ -d "$dir" ] || { echo "no such data dir: $dir" >&2; exit 1; }
[ -x ./padelleague ] || { echo "./padelleague is missing: run make build" >&2; exit 1; }

port=$(node e2e/find-free-port.mjs)
log=$(mktemp)
./padelleague serve --http="127.0.0.1:$port" --dir="$dir" >"$log" 2>&1 &
pid=$!
trap 'kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; rm -f "$log"' EXIT

for _ in $(seq 1 150); do
	if curl -sf "http://127.0.0.1:$port/healthz" >/dev/null; then
		echo "migrations applied to $dir"
		exit 0
	fi
	kill -0 "$pid" 2>/dev/null || break
	sleep 0.2
done
echo "migrating $dir failed; server log:" >&2
cat "$log" >&2
exit 1
