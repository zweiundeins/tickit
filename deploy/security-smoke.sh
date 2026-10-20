#!/usr/bin/env bash
#
# Checks a built binary, as production runs it (-secure, no -dev), over
# HTTP from outside (docs/security.md): the security headers and noindex
# on pages, robots.txt, the session cookie's flags, no debug or
# development route, a cross-site POST and a POST without Datastar's
# header refused with 403, and the stream's views refused without a tab.
#
#   bash deploy/security-smoke.sh [./tickit]
set -euo pipefail
BIN=$(realpath "${1:-./tickit}")
PORT=${PORT:-17305}
URL=http://127.0.0.1:$PORT
tmp=$(mktemp -d)
pid=
trap '[[ -n $pid ]] && kill -9 "$pid" 2>/dev/null; rm -rf "$tmp"' EXIT
fail() { echo "!! $*" >&2; [[ -f $tmp/log ]] && tail -n 20 "$tmp/log" >&2; exit 1; }

"$BIN" serve -addr "127.0.0.1:$PORT" -metrics "127.0.0.1:$((PORT + 100))" -db "$tmp/tickit.db" -secure 2>"$tmp/log" &
pid=$!
for ((i = 0; i < 100; i++)); do
	curl -fsS "$URL/healthz" >/dev/null 2>&1 && break
	sleep 0.1
done

for path in / /privacy /login /projects /spaces /no/such/page; do
	h=$(curl -sS -D - -o /dev/null "$URL$path")
	for want in "content-security-policy: .*frame-ancestors 'none'" "content-security-policy: .*'nonce-" "strict-transport-security: max-age=" \
		"x-content-type-options: nosniff" "referrer-policy: " "cross-origin-opener-policy: same-origin" "x-robots-tag: noindex, nofollow"; do
		grep -qiE "^$want" <<<"$h" || fail "$path lacks $want"
	done
	if c=$(grep -i '^set-cookie:' <<<"$h"); then
		grep -qi '__Host-sid=' <<<"$c" && grep -qi 'secure' <<<"$c" && grep -qi 'httponly' <<<"$c" && grep -qi 'samesite=lax' <<<"$c" && grep -qi 'path=/' <<<"$c" ||
			fail "$path sets a cookie without __Host-, Secure, HttpOnly, SameSite=Lax, Path=/: $c"
	fi
done
[[ "$(curl -fsS "$URL/robots.txt")" == $'User-agent: *\nAllow: /$\nDisallow: /' ]] || fail "robots.txt"

for path in /debug/pprof/ /debug/vars /debug/pprof/heap /reload /hotreload /metrics /auth/test-session /auth/clear-cookies /static/datastar-inspector.js; do
	code=$(curl -s -o /dev/null -w '%{http_code}' "$URL$path")
	[[ $code == 404 ]] || fail "$path answers $code in production"
done
grep -rqi 'inspector' <(curl -fsS "$URL/") && fail "the page loads an inspector"

body='{"tab":"tab1","op":"op1","name":"Mallory"}'
for hdr in "Sec-Fetch-Site: cross-site" "Origin: https://evil.example" "X-None: 1"; do
	code=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "$hdr" -H 'Datastar-Request: true' -H 'Content-Type: application/json' -d "$body" "$URL/cmd/people/rename")
	[[ $code == 403 ]] || fail "a POST with $hdr answers $code"
done
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Sec-Fetch-Site: same-origin' -H 'Content-Type: application/json' -d "$body" "$URL/cmd/people/rename")
[[ $code == 403 ]] || fail "a POST without Datastar's header answers $code"
code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 "$URL/stream?v=admin-demo:")
[[ $code == 400 ]] || fail "a stream without a tab answers $code"
grep -qiE 'password|__Host-sid=' "$tmp/log" && fail "the log holds a password or a cookie"

kill -TERM "$pid"
wait "$pid" 2>/dev/null || true
pid=
echo ">> ok: headers, cookie, robots.txt, no debug routes, cross-site POSTs refused"
