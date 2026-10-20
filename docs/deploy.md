# Deploying tickit

tickit runs as one static binary and one SQLite file on the shared host, behind its Caddy, at https://tickitdemo.zweiundeins.gmbh. The unit is `tickit`, the site is `tickitdemo`. This page covers the first install, staff accounts, deploys and rollbacks, backups and restores, and the monitoring. lith's guides [Deploying](https://lith.zweiundeins.gmbh/docs/guides/deploying), [Metrics and monitoring](https://lith.zweiundeins.gmbh/docs/guides/monitoring) and [Backups](https://lith.zweiundeins.gmbh/docs/guides/backups) explain the design these files come from.

## Where things live

| On the host | From | What it is |
| --- | --- | --- |
| `/opt/tickit/tickit` | `go tool task build` | the binary, root-owned; `tickit.prev` beside it is the rollback target |
| `/var/lib/tickit/tickit.db` | the unit's `StateDirectory` | the database (with `-wal` and `-shm`), owned by `tickit`, mode 0600 |
| `/var/lib/tickit/backups/` | `tickit-backup.timer` | the daily copies, the newest 30 |
| `/var/lib/tickit/backups/pre-deploy/` | `tickit-deploy` | the copies taken before a deploy that migrates, the newest 10 |
| `/etc/tickit/tickit.env` | `deploy/tickit.env.example` | optional settings, root-owned, mode 0600 |
| `/etc/systemd/system/tickit.service` | `deploy/tickit.service` | the unit |
| `/etc/systemd/system/tickit-backup.{service,timer}` | `deploy/` | the daily backup |
| `/usr/local/bin/tickit-deploy` | `deploy/tickit-deploy` | deploy and rollback |
| `/etc/caddy/sites/tickitdemo.caddy` | `deploy/tickitdemo.caddy` | the site block |
| `/var/log/caddy/tickitdemo-access.log` | Caddy | the access log, JSON, mode 0640, kept 30 days |
| `/etc/prometheus/apps/tickit.yml` | `deploy/observability/tickit.metrics.yml` | the metrics target, `{app="tickit", slot="prod"}` |
| `/etc/prometheus/targets/tickitdemo.yml` | `deploy/observability/tickitdemo.targets.yml` | the blackbox probes |
| `/etc/alloy/tickitdemo.alloy` | `deploy/observability/tickitdemo.alloy` | the journal and the access log, to Loki |
| `/etc/grafana/provisioning/alerting/rules-tickit.yaml` | `deploy/observability/rules-tickit.yaml` | tickit's alert rules |

The process listens on `127.0.0.1:7105` for Caddy and serves `/metrics` on `127.0.0.1:7205` for Prometheus. The unit allows loopback only, in both directions (`IPAddressDeny=any`, `IPAddressAllow=localhost`), and lets the process bind those two ports and no other. tickit sends no mail and connects nowhere.

## First install

As root on the host, with the binary from `go tool task build` and a copy of `deploy/` in the current directory.

### Moving the old tickit aside

The host ran the previous tickit as the same unit name, from `/opt/tickit`, as the user `tickit`. The rewrite does not read its data. Stop it and keep its folder:

```sh
systemctl stop tickit
mv /opt/tickit /opt/tickit-v1
install -d -m 0755 -o root -g root /opt/tickit
usermod -d /var/lib/tickit tickit
```

The user `tickit` stays. On a host without it: `useradd --system --home /var/lib/tickit --shell /usr/sbin/nologin tickit`.

### Units, settings and the first deploy

```sh
install -m 0644 tickit.service tickit-backup.service tickit-backup.timer /etc/systemd/system/
install -D -m 0600 -o root -g root tickit.env.example /etc/tickit/tickit.env
install -m 0755 tickit-deploy /usr/local/bin/
systemctl daemon-reload
systemctl enable tickit.service tickit-backup.timer
tickit-deploy ./tickit
systemctl start tickit-backup.timer
```

systemd reads the env file as root before it starts the process, so mode 0600 root:root is enough. `tickit-deploy` creates the database on the first start and runs the migrations.

### The Caddy block

DNS: `tickitdemo.zweiundeins.gmbh` has A and AAAA records pointing at the host since the move of 2026-10-12. Caddy asks for the certificate when it loads the block, so on another host the records come first.

```sh
install -m 0644 tickitdemo.caddy /etc/caddy/sites/
runuser -u caddy -- env HOME=/var/lib/caddy caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
systemctl reload caddy
```

`caddy validate` runs as the caddy user: run as root, it creates the access log owned by root and the next reload fails. The block replaces the old one, which compressed the streams and served `/static` and `/uploads` from disk. The new one only proxies: tickit compresses and serves everything itself and sets its own security headers.

### Monitoring drop-ins

```sh
bash setup-host.sh
```

`setup-host.sh` installs the four files in `observability/` where the host's Prometheus, Alloy and Grafana load them, each only if that service is installed. It reloads Alloy and puts the old file back if Alloy refuses the new one, reloads Grafana's alert rules through its admin API (or says to restart Grafana when it has no admin password), and never restarts tickit. It also removes the old `/etc/systemd/system/tickit.service.d/listen.conf` once the new unit is installed, since the unit itself keeps tickit on loopback. Run it again after changing a file in `observability/`.

### A restore, once, before the cut-over

Take a copy and restore it as [Restoring a copy](#restoring-a-copy) describes, then check that the site answers (NFR-70). The procedure is then known to work on this host before anybody depends on it.

### Deploys from CI

`setup-host.sh` with the public half of a deploy key also creates the user `tickit-deploy`, whose key may run `sudo -n /usr/local/bin/tickit-deploy -` and nothing else:

```sh
ssh-keygen -t ed25519 -N "" -C "ci@tickit" -f tickit-deploy-key   # on your machine
bash setup-host.sh tickit-deploy-key.pub                           # on the host, as root
```

A job then deploys with `ssh tickit-deploy@<host> < tickit`. lith's `site/deploy/README.md` shows a GitHub workflow that does this, with the key as an environment secret. Deleting `/var/lib/tickit-deploy/.ssh/authorized_keys` takes the access away.

## Staff accounts

Visitors need no account. Staff accounts (moderators and admins) are made on the host, as the `tickit` user, with the password on standard input so it appears in no process list and no shell history:

```sh
read -rsp 'Password: ' pw; echo
printf '%s\n' "$pw" | runuser -u tickit -- /opt/tickit/tickit account create \
	-db /var/lib/tickit/tickit.db -email ada@example.com -username ada -name 'Ada Admin' -role admin
unset pw
```

`TICKIT_PASSWORD` in the environment works too, and wins over standard input. The password has 8 to 128 characters; `-role` is `none`, `moderator` or `admin`. Always run it as `tickit`: run as root, it would leave root-owned `-wal` and `-shm` files that the server cannot open. It works while the server runs. Staff accounts survive the nightly reset.

## Deploying

```sh
tickit-deploy ./tickit       # or: ssh tickit-deploy@<host> < tickit
```

`tickit-deploy`:

1. checks that the file is an ELF binary and runs `--version` as `tickit`, without network, for ten seconds at most;
2. runs the new binary's `migrations-pending` on the database. If the binary refuses the database (its schema is newer than the binary's newest migration), the script stops and nothing changes. If the binary would apply migrations, the script first takes a copy with `tickit backup` into `/var/lib/tickit/backups/pre-deploy` (NFR-71), and stops if the copy fails;
3. keeps the running binary aside, moves the new one in place and restarts `tickit`;
4. waits up to 30 seconds for `http://127.0.0.1:7105/healthz` to answer `ok <new version>`;
5. if it does not, puts the old binary back. If the new binary migrated the database and the old one refuses it, the script stops `tickit`, moves the migrated database to `tickit.db.failed-<time>` and puts the copy from step 2 in its place, then starts the old binary.

On a restart every open page reconnects: the streams end with `sse.Reconnect`, and the pages open them again on the new process (NFR-42). The unit's `TimeoutStopSec=30` covers the drain: the streams end at once, requests get ten seconds and still reach the writer, then the writer commits its queue.

Migrations run when the binary opens the database (NFR-65). `tickit migrations-pending -db /var/lib/tickit/tickit.db` prints how many the binary would apply.

## Rolling back

```sh
tickit-deploy rollback
```

returns to the binary that ran before the last good deploy, with the same checks and health wait. Across a deploy that migrated, the previous binary refuses the database, and `rollback` says so and changes nothing. Going back past a migration takes the copy from before it, and loses what was written since:

```sh
ls /var/lib/tickit/backups/pre-deploy/       # the copy from before the migrating deploy
systemctl stop tickit
# then the steps of "Restoring a copy" with that copy, but without starting tickit
tickit-deploy rollback
```

## Backups

`tickit-backup.timer` runs `tickit backup` every day at 03:30 Europe/Zurich, half an hour before the nightly reset. It writes a consistent copy with `VACUUM INTO` while the server runs, checks it with `PRAGMA integrity_check`, and keeps the newest 30 in `/var/lib/tickit/backups` (NFR-70). A copy that fails the check stays as `tickit-<time>.db.bad`, no older copy is deleted, and the unit fails, which `systemctl list-units --failed` and the journal show:

```sh
systemctl list-timers tickit-backup.timer
journalctl -u tickit-backup.service -n 20
```

The copies sit on the host's disk. Copy `/var/lib/tickit/backups` off the machine as well (restic, rsync) to survive a lost disk.

### Restoring a copy

```sh
systemctl stop tickit
cd /var/lib/tickit
ls backups/                                   # pick a copy: tickit-<UTC time>.db
runuser -u tickit -- /opt/tickit/tickit migrations-pending -db backups/tickit-20261021T013000Z.db
mkdir -p aside && mv tickit.db tickit.db-wal tickit.db-shm aside/ 2>/dev/null
install -o tickit -g tickit -m 0600 backups/tickit-20261021T013000Z.db tickit.db
systemctl start tickit
curl -fsS http://127.0.0.1:7105/readyz
```

`migrations-pending` opens the copy read-only: it prints a number for a copy the binary can run on (and migrates at the start), and fails for one that is not a database or is newer than the binary. Delete `aside/` once the restored database has served for a while.

### A lost database

With the database lost, the latest copy brings back the staff accounts and the moderation log, and the seed brings back the demo content (NFR-72): restore the copy as above, log in as an admin and use "Reset now", or wait for the reset at 04:00.

## The nightly reset

The demo resets every night at 04:00 Europe/Zurich (DM-03). The app schedules it itself, so the host needs no timer or cron job for it. The reset counts itself in `tickit_resets_total{outcome}`, which the alert rules read.

## Metrics, probes, logs and alerts

- **Metrics:** Prometheus's `apps` job scrapes `127.0.0.1:7205` as `{app="tickit", slot="prod"}`. The host's Grafana dashboard "lith apps" shows tickit under the namespace `tickit`. `deploy/observability/tickit.metrics.yml` lists the metrics: lith's writer, live hub, stream budget and event store (`tickit_writer_*`, `tickit_live_*`, `tickit_sse_budget_*`, `tickit_events_*`), tickit's own counters (`tickit_rate_limited_total`, `tickit_refusals_total`, `tickit_logins_total`, `tickit_edit_batches_total`, `tickit_page_editors_max`, `tickit_moderation_actions_total`, `tickit_resets_total{outcome}`, `tickit_reset_seconds`, `tickit_guests_active`), `tickit_build_info{version}`, `go_*` and `process_*`.
- **Probes:** the blackbox exporter fetches `https://tickitdemo.zweiundeins.gmbh/` with `http_2xx` and `/healthz` with `http_2xx_ok` (a 200 whose body starts with `ok `), labelled `service: tickitdemo`.
- **Health:** `/healthz` answers `ok <version>` while the process serves; `/readyz` answers 200 while the read pool answers and the writer, the hub and the event runner run, else 503 naming the failing check (NFR-75). Neither goes through the request log.
- **Logs:** tickit logs slog's text format to the journal, at Info and above. Alloy ships it to Loki as `{job="tickit", level="..."}`, and the access log as `{job="caddy", site="tickitdemo"}` through the host's `caddy_logs` pipeline. Loki and Caddy keep 30 days (NFR-28); the journal is a size-capped buffer.
- **Alerts:** `rules-tickit.yaml` fires when more than 1% of requests answer a 5xx for 5 minutes (from the access log, with at least 5 errors), when a reset fails (`tickit_resets_total{outcome="failed"}` rises), and when no reset succeeds for 26 hours. The host's rules cover the rest of NFR-78: a failed probe ("External endpoint unreachable"), tick overruns and a parked projector (`rules-lith-apps.yaml`), and a unit that fails or keeps restarting (`rules-services.yaml`).

To check the metrics by hand: `curl -s http://127.0.0.1:7205/metrics | grep ^tickit_`.

## Settings

Every flag can also come from the environment as `TICKIT_<FLAG>`, upper case with `-` as `_`: `-session-ttl` is `TICKIT_SESSION_TTL`. A flag on the command line wins over the environment. On the host the unit's command line sets `-addr`, `-metrics`, `-db`, `-secure` and `-streams`; the rest can go into `/etc/tickit/tickit.env`. No setting is a secret, and none has a default that would be one (NFR-05).

`tickit serve` (the default command):

| Flag | Variable | Default | On the host | What it does |
| --- | --- | --- | --- | --- |
| `-addr` | `TICKIT_ADDR` | `127.0.0.1:7105` | `127.0.0.1:7105` | listen on host:port, or on a Unix socket as `unix:/path` |
| `-metrics` | `TICKIT_METRICS` | `127.0.0.1:7205` | `127.0.0.1:7205` | Prometheus metrics on this loopback host:port; empty for none; a non-loopback address is refused |
| `-db` | `TICKIT_DB` | `data/tickit.db` | `/var/lib/tickit/tickit.db` | the SQLite database |
| `-synchronous` | `TICKIT_SYNCHRONOUS` | `NORMAL` | `NORMAL` | `NORMAL` (a power loss can undo up to about 35 s of commits) or `FULL` (one fsync per batch, loses none) |
| `-streams` | `TICKIT_STREAMS` | `200` | `200` | how many streams may be open at once; more get 503, which the page retries. Change it with the unit's memory settings |
| `-session-ttl` | `TICKIT_SESSION_TTL` | `24h` | `24h` | how long a login lasts |
| `-secure` | `TICKIT_SECURE` | `false` | `true` | served over HTTPS: a `__Host-` cookie and HSTS |
| `-dev` | `TICKIT_DEV` | `false` | `false` | development: static files as written, not minified |
| `-debug` | `TICKIT_DEBUG` | `false` | `false` | log at debug level |

`tickit backup`:

| Flag | Variable | Default | On the host | What it does |
| --- | --- | --- | --- | --- |
| `-db` | `TICKIT_DB` | `data/tickit.db` | `/var/lib/tickit/tickit.db` | the database to copy |
| `-backups` | `TICKIT_BACKUPS` | `data/backups` | `/var/lib/tickit/backups` | the directory of the copies |
| `-keep` | `TICKIT_KEEP` | `30` | `30` | how many copies to keep |

`tickit account create`:

| Flag | Variable | Default | What it does |
| --- | --- | --- | --- |
| `-db` | `TICKIT_DB` | `data/tickit.db` | the database |
| `-email` | `TICKIT_EMAIL` | none | the account's email address |
| `-username` | `TICKIT_USERNAME` | none | 2 to 50 letters, digits, `.`, `-` or `_` |
| `-name` | `TICKIT_NAME` | the username | the full name shown beside what the account does |
| `-role` | `TICKIT_ROLE` | `none` | the site role: `none`, `moderator` or `admin` |
| (none) | `TICKIT_PASSWORD` | standard input | the password, 8 to 128 characters; never a flag |

`tickit migrations-pending` takes `-db` (`TICKIT_DB`, default `data/tickit.db`) and prints how many migrations the binary would apply to it. `tickit --version` prints `tickit <version>`, the version from `git describe` that the build stamps into `main.version`.

The unit also sets `GOMEMLIMIT=448MiB`, beside `MemoryHigh=512M` and `MemoryMax=640M`. The live hub gives the streams half of `GOMEMLIMIT` for their encoders and frames, which holds 200 streams once the budget moves them to smaller windows; change the three and `-streams` together. `go tool task dev` serves on `127.0.0.1:8080` with `-dev` and without metrics.

## Checking a build locally

`go tool task smoke` builds the binary as CI does (`CGO_ENABLED=0`, `-trimpath`, the version from `git describe`) and runs `deploy/smoke-test.sh` on it: `--version`, `/healthz`, `/readyz`, the home page with HSTS under `-secure`, `/metrics` on its own port and not on the public one, `migrations-pending`, `backup`, and an exit within 30 seconds of SIGTERM. `go tool task vuln` runs govulncheck, which CI runs on every push and which fails on a vulnerability in code the binary calls.
