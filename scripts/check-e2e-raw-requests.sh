#!/bin/bash
# Fails when e2e code calls page.request/request.<verb>( or request[verb](
# directly. A raw call doesn't check the response, so a failed setup request (a 403, a 429 on
# auth) passes silently and the test asserts against state that was never
# written (census #2). Use the helpers.ts request helpers, which throw on a
# non-2xx. A call that must stay raw (a multipart upload, a probe whose status
# is the assertion, a poll) carries a `// raw-request: <why>` comment on the
# line above it.
set -u
cd "$(dirname "$0")/.."

raw=$(awk '
	FNR == 1 { prev = "" }
	/request(\.(get|post|patch|put|delete|fetch)|\[[^]]+\])\(/ && prev !~ /\/\/ raw-request:/ { print FILENAME ":" FNR ": " $0 }
	{ prev = $0 }
' e2e/*.ts e2e/tests/*.ts)
if [ -n "$raw" ]; then
	echo "FAIL: raw request call in e2e — use the throwing helpers in e2e/helpers.ts, or mark it with a '// raw-request: <why>' comment on the line above:"
	echo "$raw"
	exit 1
fi
