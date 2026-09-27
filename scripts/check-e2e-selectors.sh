#!/bin/bash
# Fails when an e2e selector points at markup that no longer exists, so a
# renamed data-testid / aria-label breaks `make invariants` in seconds
# instead of hanging a Playwright run to its timeout. Also bans exact
# `[aria-label="x"]` matchers: labels carry dynamic state ("cambiar vista:
# Admin"), so tests must match a prefix (`^=`) or use getByLabel.
set -u
cd "$(dirname "$0")/.."

specs=$(find e2e -name '*.ts' -not -path '*/node_modules/*')
markup=$(find views static/js -name '*.html' -o -name '*.js')
fail=0

# Every aria-label value the app can render, one per line; template
# branches ({{if}}Rondas{{else}}Jornadas{{end}}) stay inside one value, so a
# literal only has to be a substring of some value.
labels=$(grep -hoE 'aria-label="[^"]*"' $markup | sed 's/^aria-label="//; s/"$//')

exact=$(grep -nE '\[aria-label="' $specs || true)
if [ -n "$exact" ]; then
	echo "FAIL: exact [aria-label=\"...\"] matcher in e2e — use [aria-label^=\"...\"] or getByLabel()"
	echo "$exact"
	fail=1
fi

testids=$(grep -hoE "getByTestId\('[^']+'\)|data-testid=\\\\?\"[^\"\\\\]+" $specs \
	| sed -E "s/getByTestId\('([^']+)'\)/\1/; s/data-testid=\\\\?\"//" | sort -u)
for id in $testids; do
	if ! grep -qF "data-testid=\"$id\"" $markup; then
		echo "FAIL: e2e uses data-testid \"$id\" but no template or script renders it"
		grep -nF "$id" $specs | head -3
		fail=1
	fi
done

arias=$(grep -hoE "aria-label[\^\*\$]?=\\\\?\"[^\"\\\\]+|getByLabel\('[^']+'" $specs \
	| sed -E "s/aria-label[^=]?=\\\\?\"//; s/getByLabel\('//; s/'$//" | sort -u)
while IFS= read -r label; do
	# Interpolated labels (`${x}`) are checked by the test that builds them.
	case "$label" in ''|*'${'*) continue ;; esac
	if ! grep -qF -- "$label" <<<"$labels"; then
		echo "FAIL: e2e uses aria-label \"$label\" but no template or script renders it"
		grep -nF -- "$label" $specs | head -3
		fail=1
	fi
done <<<"$arias"


# An assertion inside `if (await x.isVisible())` silently passes once x stops
# rendering (the R-167 onboarding test asserted nothing for weeks). Such
# guards may only drive navigation/setup; the expect() must be unconditional.
guarded=$(awk '
	/if \(await .*isVisible\(\)/ { depth = 0; inside = 1; start = NR }
	inside {
		n = gsub(/\{/, "{"); m = gsub(/\}/, "}"); depth += n - m
		if ($0 ~ /expect\(/) { print FILENAME ":" NR ": " $0; hit = 1 }
		if (depth <= 0 && NR > start) inside = 0
	}
' $specs)
if [ -n "$guarded" ]; then
	echo "FAIL: expect() inside an isVisible() guard — the assertion vanishes when the element does; assert unconditionally"
	echo "$guarded"
	fail=1
fi


# `new Date(Date.now() + N*86400000).toISOString().slice(0,10)` computes a
# "today plus/minus N days" date in UTC, then reads it back as a calendar
# date in UTC — wrong for the league (Atlantic/Canary, ahead of UTC) during
# the hour(s) after local midnight, when the UTC calendar day hasn't rolled
# over yet. Every night in that window, "tomorrow" computed this way is
# still "today" in the league. Use helpers.ts's leagueDate(offsetDays)
# instead, which reads the calendar date in the league's own timezone.
# jornadaHiISO derives a date by adding whole days (via jornadaRange) to a
# FIXED start/end date pair (competitionDates(), literal 2036 dates) — not to
# Date.now() — so it isn't subject to the today-relative UTC bug and is
# exempted by name.
today_relative=$(grep -nE '86400000|24 ?\* ?60 ?\* ?60 ?\* ?1000|toISOString\(\)\.slice\(0, ?10\)|toISOString\(\)\.split\(.T.\)' $specs \
	| grep -v 'e2e/helpers\.ts:' | grep -v 'e2e/scenario-helpers\.ts:.*MS_PER_DAY' | grep -v 'jornadaRange(startISO, endISO, target, n)\.hi\.toISOString' || true)
if [ -n "$today_relative" ]; then
	echo "FAIL: today-relative date computed via Date.now()+N*86400000/toISOString() (UTC) instead of leagueDate(offsetDays) from helpers.ts"
	echo "$today_relative"
	fail=1
fi

exit $fail
