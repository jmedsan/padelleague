#!/bin/bash
# Fails when a top-level test.describe (or a top-level test outside any
# describe) in e2e/tests/*.spec.ts carries no area tag. `make e2e AREA=x`
# selects by tag, so an untagged block is silently skipped by every area run
# (z-admin-settings' "league defaults" block was). The tag must sit on the
# call's first line: `test.describe('title', { tag: '@area' }, () => {`.
set -u
cd "$(dirname "$0")/.."

untagged=$(grep -nE "^test(\.describe)?(\.(serial|parallel|only|skip|fixme))?\(" e2e/tests/*.spec.ts | grep -vE "tag: *\[?'@[a-z-]+'" || true)
if [ -n "$untagged" ]; then
	echo "FAIL: top-level test/describe without an area tag — add { tag: '@<area>' } on the same line, or AREA runs skip it:"
	echo "$untagged"
	exit 1
fi
