#!/bin/bash
# Maps changed source paths (vs HEAD, plus untracked) to e2e area tags and
# prints a Playwright --grep pattern like "@thread|@scoring" on stdout. Used
# by `make check` to run only the e2e specs for the areas a change touches,
# instead of the full @smoke sweep. Empty output means no mapped area
# changed (caller falls back to @smoke).
set -euo pipefail

changed=$( { git diff --name-only HEAD; git ls-files --others --exclude-standard; } | sort -u)
if [ -z "$changed" ]; then
	exit 0
fi

areas=""
add() { [[ " $areas " == *" $1 "* ]] || areas="$areas $1"; }

while IFS= read -r f; do
	case "$f" in
		views/thread*|handlers/thread*.go|handlers/match_thread*.go) add thread ;;
		league/standings*|league/tiebreak*|handlers/*standings*) add standings ;;
		league/leveled.go|league/rating.go|handlers/admin_pairs.go) add leveled ;;
		handlers/admin_*.go|views/admin/*) add admin ;;
		handlers/*scoring*|handlers/match*.go|league/scoring*) add scoring ;;
		handlers/*schedul*|handlers/*walkover*) add scheduling ;;
		handlers/*competition*|views/competition*) add competitions ;;
		search/*|handlers/search*.go|views/*search*) add search ;;
		notify/*|handlers/notifications*.go|views/*notification*) add notifications ;;
		handlers/documents*.go|views/*document*) add docs ;;
		handlers/*profile*|views/player*) add profile ;;
		routes/routes.go) add routes ;;
	esac
done <<< "$changed"

[ -z "$areas" ] && exit 0
echo "$areas" | tr ' ' '\n' | grep -v '^$' | sed 's/^/@/' | paste -sd'|'
