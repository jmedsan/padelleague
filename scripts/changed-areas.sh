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
		# views/partials/* that belong to one area; cross-cutting partials
		# (empty-state, entity-links, badges, date-picker, ...) stay unmapped.
		views/partials/contact-links.html|views/partials/avatar-upload.html|views/partials/stats-tiles.html|views/partials/competition-stats-table.html|views/partials/result-history-section.html) add profile ;;
		views/partials/timeline.html|views/partials/time-picker.html) add thread ;;
		views/partials/score-input.html|views/partials/match-card.html) add scoring ;;
		views/partials/standings-table.html) add standings ;;
		views/partials/competition-card.html|views/partials/competition-header.html|views/partials/announcement-card.html) add competitions ;;
		views/partials/health-item-row.html|views/partials/outstanding-match.html|views/partials/pair-flag-toggle.html|views/partials/pair-level-select.html|views/partials/penalty-entry.html) add admin ;;
		views/partials/password-set.html|handlers/auth.go|handlers/password_reset.go) add auth ;;
		views/partials/simple-link-row.html) add search ;;
		views/thread*|handlers/thread*.go|handlers/match_thread*.go) add thread ;;
		league/standings*|league/tiebreak*|handlers/*standings*) add standings ;;
		league/leveled.go|league/rating.go|handlers/admin_pairs.go) add leveled ;;
		handlers/admin_*.go|views/admin/*) add admin ;;
		handlers/*scoring*|handlers/match*.go|league/scoring*) add scoring ;;
		handlers/*schedul*|handlers/*walkover*) add scheduling ;;
		handlers/*competition*|views/competition*) add competitions ;;
		search/*|handlers/search*.go|views/*search*) add search ;;
		notify/*|handlers/notifications*.go|views/*notification*) add notifications ;;
		handlers/document*.go|views/*document*) add docs ;;
		handlers/*profile*|handlers/player*.go|views/player*) add profile ;;
		routes/routes.go) add routes ;;
	esac
done <<< "$changed"

[ -z "$areas" ] && exit 0
echo "$areas" | tr ' ' '\n' | grep -v '^$' | sed 's/^/@/' | paste -sd'|'
