#!/bin/bash
# Fails when an e2e test creates its own competition and then logs in as a
# player without asPlayerOn (e2e/helpers.ts). A competition's calendar starts
# as a draft that players cannot see, so that test asserts against an empty
# page (census #3, #6, #30). A test that publishes through the real
# "Publicar calendario" button instead also passes.
set -u
cd "$(dirname "$0")/.."

offenders=$(awk '
function flush() {
	if (creates && player && !helper) print file ":" start ": " title
	creates = player = helper = 0
}
FNR == 1 { flush(); file = FILENAME; start = 0 }
/^[ \t]*test(\.(only|skip|fixme))?\(/ { flush(); start = FNR; title = $0; sub(/^[ \t]*/, "", title) }
/apiCreate(Record)?\([^)]*'\''competitions'\''|\/api\/collections\/competitions\/records'\''/ { creates = 1 }
/login(As|ViaForm)\(/ && !/ADMIN_EMAIL/ { player = 1 }
/asPlayerOn\(|Publicar calendario/ { helper = 1 }
END { flush() }
' e2e/tests/*.spec.ts)
if [ -n "$offenders" ]; then
	echo "FAIL: test creates a competition and logs in a player without asPlayerOn — its calendar stays a draft players cannot see:"
	echo "$offenders"
	exit 1
fi
