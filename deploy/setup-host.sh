#!/usr/bin/env bash
#
# Host setup (see docs/deploy.md). Run as root on the server, from a copy of
# this folder:
#
#   bash setup-host.sh [tickit-deploy-key.pub]
#
# The drop-ins in observability/ go into the folders the host's Prometheus,
# Alloy and Grafana load, each only where that service is installed. With the
# public half of the CI deploy key (a .pub file), it also installs
# /usr/local/bin/tickit-deploy and creates the `tickit-deploy` user. That user's
# only key is forced to `sudo -n /usr/local/bin/tickit-deploy -` (no shell, no
# forwarding), and sudo allows exactly that command. Rerunning replaces the
# drop-ins and the key. It never restarts tickit.
set -euo pipefail
NAME=tickit # the unit, the Caddy site, its drop-ins and alert rules
HERE=$(cd "$(dirname "$0")" && pwd)
OBS=$HERE/observability
# Alloy's HTTP server, for its reload endpoint.
ALLOY_URL=${ALLOY_URL:-http://127.0.0.1:12345}

installed() { [[ "$(systemctl show -p LoadState --value "$1" 2>/dev/null)" == loaded ]]; }

deploy_user() {
	local keyfile=$1 pubkey deploy=$NAME-deploy
	local homedir=/var/lib/$deploy
	[[ -f "$keyfile" ]] || { echo "!! no such file: $keyfile" >&2; exit 2; }
	pubkey=$(tr -d '\r' < "$keyfile" | sed '/^$/d')
	[[ "$pubkey" == ssh-* && "$pubkey" != *$'\n'* ]] || { echo "!! that does not look like one public key" >&2; exit 2; }

	install -m 0755 -o root -g root "$HERE/$deploy" "/usr/local/bin/$deploy"
	id "$deploy" >/dev/null 2>&1 ||
		useradd --system --create-home --home-dir "$homedir" --shell /bin/sh "$deploy"
	install -d -m 0700 -o "$deploy" -g "$deploy" "$homedir/.ssh"
	printf 'restrict,command="sudo -n /usr/local/bin/%s -" %s\n' "$deploy" "$pubkey" > "$homedir/.ssh/authorized_keys"
	chown "$deploy": "$homedir/.ssh/authorized_keys"
	chmod 0600 "$homedir/.ssh/authorized_keys"

	echo "$deploy ALL=(root) NOPASSWD: /usr/local/bin/$deploy -" > "/etc/sudoers.d/$deploy.tmp"
	chmod 0440 "/etc/sudoers.d/$deploy.tmp"
	visudo -cqf "/etc/sudoers.d/$deploy.tmp"
	mv -f "/etc/sudoers.d/$deploy.tmp" "/etc/sudoers.d/$deploy"
	echo ">> ready: CI can deploy as $deploy@$(hostname)"
}

# The old tickit's drop-in kept it on loopback. The new unit does that itself,
# so the drop-in goes once that unit is installed; the running process keeps
# its settings until its next restart, which is the next deploy.
old_dropin() {
	local conf=/etc/systemd/system/$NAME.service.d/listen.conf
	[[ -f "$conf" ]] || return 0
	if grep -q '^SocketBindDeny=any' "/etc/systemd/system/$NAME.service" 2>/dev/null; then
		rm -f "$conf"
		rmdir --ignore-fail-on-non-empty "$(dirname "$conf")"
		systemctl daemon-reload
		echo ">> removed the old $conf"
	else
		echo ">> kept $conf: install the new $NAME.service first"
	fi
}

# Prometheus watches both folders (file_sd), so it needs no reload.
prometheus_dropins() {
	if ! installed prometheus; then
		echo ">> no Prometheus here: no scrape target, no probes"
		return
	fi
	install -D -m 0644 "$OBS/$NAME.metrics.yml" "/etc/prometheus/apps/$NAME.yml"
	echo ">> Prometheus scrapes $NAME: /etc/prometheus/apps/$NAME.yml"
	install -D -m 0644 "$OBS/$NAME.targets.yml" "/etc/prometheus/targets/$NAME.yml"
	echo ">> the blackbox exporter probes $NAME: /etc/prometheus/targets/$NAME.yml"
}

# Grafana's alert rules, reloaded as the host's install.sh does: through the
# admin API with the password from /etc/grafana/grafana.env, never on the
# command line. A rule uid that another file uses keeps Grafana from
# starting, and a file the reload refuses goes back to the old one.
grafana_reload() {
	local env=/etc/grafana/grafana.env user pass
	user=$(sed -n 's/^GF_SECURITY_ADMIN_USER=//p' "$env" 2>/dev/null | tr -d '"')
	pass=$(sed -n 's/^GF_SECURITY_ADMIN_PASSWORD=//p' "$env" 2>/dev/null | tr -d '"')
	[[ -n $pass ]] && systemctl is-active --quiet grafana-server || return 100
	printf 'user = "%s:%s"\n' "${user:-admin}" "$pass" |
		curl -fsS -K - -X POST -o /dev/null http://127.0.0.1:3000/api/admin/provisioning/alerting/reload
}

grafana_rules() {
	local src=$OBS/rules-$NAME.yaml dir=/etc/grafana/provisioning/alerting
	local dst=$dir/rules-$NAME.yaml uid other old rc=0
	if ! installed grafana-server; then
		echo ">> no Grafana here: no alert rules"
		return
	fi
	if cmp -s "$src" "$dst"; then
		echo ">> Grafana has $NAME's alert rules: $dst"
		return
	fi
	while read -r uid; do
		other=$(grep -lE "^ *- uid: *$uid\$" "$dir"/*.yaml 2>/dev/null | grep -vxF "$dst" || true)
		[[ -z $other ]] || { echo "!! the rule uid $uid is in $other too" >&2; exit 1; }
	done < <(sed -n 's/^ *- uid: *//p' "$src")
	old=$(mktemp)
	cp "$dst" "$old" 2>/dev/null || rm -f "$old"
	install -D -m 0640 -g grafana "$src" "$dst"
	grafana_reload || rc=$?
	if ((rc == 0)); then
		echo ">> Grafana alerts on $NAME: $dst"
	elif ((rc == 100)); then
		echo ">> $NAME's alert rules: $dst; restart grafana-server to load them (no admin password, or not running)"
	else
		if [[ -f "$old" ]]; then install -m 0640 -g grafana "$old" "$dst"; else rm -f "$dst"; fi
		rm -f "$old"
		grafana_reload >/dev/null 2>&1 || true
		echo "!! Grafana refused the new rules-$NAME.yaml and keeps what it had" >&2
		exit 1
	fi
	rm -f "$old"
}

# Alloy loads every .alloy file in /etc/alloy as one config, and a file it
# refuses would stop the logs of all apps at its next start: the old one goes back.
alloy_dropin() {
	local src=$OBS/$NAME.alloy dst=/etc/alloy/$NAME.alloy old out
	if ! installed alloy; then
		echo ">> no Alloy here: no logs to Loki"
		return
	fi
	old=$(mktemp)
	cp "$dst" "$old" 2>/dev/null || rm -f "$old"
	install -D -m 0644 "$src" "$dst"
	if ! systemctl is-active --quiet alloy; then
		echo ">> Alloy loads $dst when it starts"
	elif out=$(curl -sS --fail-with-body --max-time 30 "$ALLOY_URL/-/reload" 2>&1); then
		echo ">> Alloy ships $NAME's logs: $dst"
	else
		if [[ -f "$old" ]]; then install -m 0644 "$old" "$dst"; else rm -f "$dst"; fi
		rm -f "$old"
		curl -sS --max-time 30 "$ALLOY_URL/-/reload" >/dev/null 2>&1 || true
		echo "!! Alloy refused the new $NAME.alloy and keeps what it had: $out" >&2
		exit 1
	fi
	rm -f "$old"
}

[[ $EUID -eq 0 ]] || { echo "!! run as root" >&2; exit 2; }
if [[ -n "${1:-}" ]]; then
	deploy_user "$1"
fi
old_dropin
prometheus_dropins
grafana_rules
alloy_dropin
