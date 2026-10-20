#!/usr/bin/env bash
#
# Runs a built binary on a fresh database and checks what the host relies on:
# --version, /healthz answering "ok <version>", /readyz, the home page with
# HSTS under -secure, /metrics on its own port, migrations-pending, backup,
# and a clean exit within the unit's stop timeout on SIGTERM.
#
#   bash deploy/smoke-test.sh [./tickit]
set -euo pipefail
BIN=$(realpath "${1:-./tickit}")
PORT=${PORT:-17105}
METRICS=$((PORT + 100))
URL=http://127.0.0.1:$PORT
tmp=$(mktemp -d)
pid=
trap '[[ -n $pid ]] && kill -9 "$pid" 2>/dev/null; rm -rf "$tmp"' EXIT
fail() { echo "!! $*" >&2; [[ -f $tmp/log ]] && tail -n 20 "$tmp/log" >&2; exit 1; }

version=$("$BIN" --version | awk '{print $2}')
[[ -n "$version" ]] || fail "--version prints no version"
echo ">> $version"

"$BIN" serve -addr "127.0.0.1:$PORT" -metrics "127.0.0.1:$METRICS" -db "$tmp/tickit.db" -secure 2>"$tmp/log" &
pid=$!
for ((i = 0; i < 100; i++)); do
	curl -fsS "$URL/healthz" >/dev/null 2>&1 && break
	sleep 0.1
done

[[ "$(curl -fsS "$URL/healthz")" == "ok $version" ]] || fail "/healthz does not answer ok $version"
curl -fsS -o /dev/null "$URL/readyz" || fail "/readyz is not 200"
headers=$(curl -fsS -D - -o /dev/null "$URL/") || fail "/ is not 2xx"
grep -qi '^strict-transport-security:' <<<"$headers" || fail "no HSTS with -secure"
grep -qi '^set-cookie: __Host-' <<<"$headers" || echo ">> (no __Host- cookie on /: set on the first action)"
curl -fsS "http://127.0.0.1:$METRICS/metrics" | grep -q "^tickit_build_info{version=\"$version\"} 1" || fail "/metrics has no tickit_build_info"
curl -fsS -o /dev/null "$URL/metrics" 2>/dev/null && fail "/metrics answers on the public port"
[[ "$("$BIN" migrations-pending -db "$tmp/tickit.db")" == 0 ]] || fail "migrations-pending is not 0 on a migrated database"
"$BIN" backup -db "$tmp/tickit.db" -backups "$tmp/backups" 2>>"$tmp/log" || fail "backup failed"
compgen -G "$tmp/backups/tickit-*.db" >/dev/null || fail "backup wrote no copy"

kill -TERM "$pid"
for ((i = 0; i < 300; i++)); do
	kill -0 "$pid" 2>/dev/null || break
	sleep 0.1
done
kill -0 "$pid" 2>/dev/null && fail "still running 30 s after SIGTERM"
wait "$pid" || fail "exit status $? after SIGTERM"
pid=
echo ">> ok: healthz, readyz, /, metrics, migrations-pending, backup, stop"
