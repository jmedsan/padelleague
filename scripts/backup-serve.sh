#!/usr/bin/env bash
# Boots a kept scenario server on a production backup, so a prod state can be
# reproduced locally. Decrypts the emailed backup (BACKUP_ENCRYPTION_KEY),
# unzips it into a /tmp/padelleague-test-* data dir and serves it on a free
# port with the same bookkeeping as `make scenario-serve`, so `make
# scenario-stop` and `make scenario-migrate` work on it. Pending migrations
# run on boot, as in a deploy.
#
# Nothing leaves the machine: the backup's SMTP settings (prod relay) are
# disabled in data.db before boot, mail goes to Mailpit (127.0.0.1:1025, run
# `make mail` to read it), push is off (no VAPID keys) and the backup email
# cron is off (no BACKUP_EMAIL). Log in with the prod credentials.
#
# Usage: scripts/backup-serve.sh <backup.zip.enc | backup.zip>
set -euo pipefail

src=${1:?usage: scripts/backup-serve.sh <backup.zip.enc | backup.zip>}
[ -f "$src" ] || { echo "no such file: $src" >&2; exit 1; }
[ -x ./padelleague ] || { echo "./padelleague is missing: run make build" >&2; exit 1; }
command -v sqlite3 >/dev/null || { echo "sqlite3 is required" >&2; exit 1; }

dir=$(mktemp -d /tmp/padelleague-test-backup-XXXXXX)
zip="$dir.zip"
trap 'rm -f "$zip"' EXIT
case "$src" in
*.enc)
	: "${BACKUP_ENCRYPTION_KEY:?BACKUP_ENCRYPTION_KEY is required to decrypt (set in .env)}"
	openssl enc -d -aes-256-cbc -pbkdf2 -pass env:BACKUP_ENCRYPTION_KEY -in "$src" -out "$zip"
	;;
*) cp "$src" "$zip" ;;
esac
unzip -q "$zip" -d "$dir"
[ -f "$dir/data.db" ] || { echo "backup has no data.db" >&2; exit 1; }

sqlite3 "$dir/data.db" "UPDATE _params SET value = json_set(value,
	'$.smtp.enabled', json('false'), '$.smtp.host', '127.0.0.1', '$.smtp.port', 1025,
	'$.smtp.username', '', '$.smtp.password', '', '$.smtp.tls', json('false'))
	WHERE id = 'settings'"
enabled=$(sqlite3 "$dir/data.db" "SELECT json_extract(value, '$.smtp.enabled') FROM _params WHERE id = 'settings'")
[ "$enabled" = "0" ] || { echo "could not disable the backup's SMTP settings, refusing to boot" >&2; exit 1; }

port=$(node e2e/find-free-port.mjs)
url="http://127.0.0.1:$port"
log="$dir/server.log"
env -i HOME="$HOME" PATH="$PATH" APP_ENV=prod APP_URL="$url" \
	SMTP_HOST=127.0.0.1 SMTP_PORT=1025 SMTP_SENDER_ADDRESS=backup@example.com \
	setsid ./padelleague serve --http="127.0.0.1:$port" --dir="$dir" >"$log" 2>&1 < /dev/null &
pid=$!

for _ in $(seq 1 150); do
	curl -sf "$url/healthz" >/dev/null && break
	kill -0 "$pid" 2>/dev/null || { echo "server exited; log:" >&2; cat "$log" >&2; exit 1; }
	sleep 0.2
done
if grep -q 'smtp: failed' "$log"; then
	kill "$pid"
	echo "SMTP override failed, server stopped; log:" >&2; cat "$log" >&2; exit 1
fi

mkdir -p "e2e/.test-data/$port"
echo "$pid" >"e2e/.test-data/$port/scenario.pid"
echo "$dir" >"e2e/.test-data/$port/scenario.dir"
echo "backup $(basename "$src") served at $url"
echo "data dir: $dir (log: $log)"
echo "stop with: make scenario-stop PORT=$port"
