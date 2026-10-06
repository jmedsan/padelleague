#!/bin/bash
# Fails when an e2e spec skips a test on a non-phone viewport but no mobile
# project runs that spec: the test then skips everywhere and never executes
# (notification-dismiss "mobile: dismiss via bell" and leveled-league "renders
# correctly on phone" sat in that state). The mobile project only runs the specs
# listed in MOBILE_VIEWPORT_SPECS (e2e/playwright.config.ts). Also fails when
# that list names a spec that does not exist.
set -u
cd "$(dirname "$0")/.."

listed=$(sed -n "/^const MOBILE_VIEWPORT_SPECS = \[/,/^\];/p" e2e/playwright.config.ts | grep -o "'[^']*\.spec\.ts'" | tr -d "'")
fail=0

for f in $listed; do
	if [ ! -f "e2e/tests/$f" ]; then
		echo "FAIL: MOBILE_VIEWPORT_SPECS lists $f, which does not exist"
		fail=1
	fi
done

# A skip whose condition is "not on the phone": !isMobile, project name !== 'mobile',
# or a viewport-width test. The condition may span lines, so match per file.
gated=$(perl -0ne 'print "$ARGV\n" if /skip\([^;]*?(!isMobile\(|!== .mobile.|viewportSize)|if \(!isMobile\(\w+\)\)\s*\{\s*test\.skip/s' e2e/tests/*.spec.ts)
for path in $gated; do
	f=$(basename "$path")
	if ! echo "$listed" | grep -qx "$f"; then
		echo "FAIL: $f skips tests on non-phone viewports but is not in MOBILE_VIEWPORT_SPECS, so the mobile project never runs them"
		fail=1
	fi
done
exit $fail
