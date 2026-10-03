#!/usr/bin/env bash
# Prints the port of the kept scenario server: the given port, or the only one
# running. Exits 2 when none is running, 1 when several are, with a message.
# Usage: scripts/scenario-port.sh [port]
set -euo pipefail

port=${1:-}
if [ -z "$port" ]; then
	runs=$(ls -d e2e/.test-data/*/scenario.pid 2>/dev/null | sed 's|e2e/.test-data/\([0-9]*\)/scenario.pid|\1|' || true)
	n=$(echo "$runs" | grep -c . || true)
	if [ "$n" -eq 0 ]; then echo "no scenario server running" >&2; exit 2; fi
	if [ "$n" -gt 1 ]; then
		echo "several scenario servers running, pick one with PORT=<port>:" >&2
		echo "$runs" >&2
		exit 1
	fi
	port=$runs
fi
echo "$port"
