# Deploying tickit

One binary and one SQLite file on a Linux server, behind Caddy. The files here:

- `tickit.service`: the systemd unit. It runs as the `tickit` user, keeps the database in `/var/lib/tickit`, listens on the Unix socket `/run/tickit/tickit.sock`, cannot reach localhost, and is capped at 640 MB of memory.
- `tickit-backup.service` and `tickit-backup.timer`: every six hours, `tickit backup` copies the database (VACUUM INTO) into `/var/lib/tickit/backups` and keeps the newest 28 copies.
- `tickit.caddy`: the site block. TLS, `reverse_proxy` to the socket with the client's address in X-Forwarded-For and X-Real-IP, the access log in JSON, no `encode`, and `flush_interval -1` for the streams.
- `tickit-deploy`: installs a binary, restarts the unit, waits for `/healthz`, and puts the old binary back if the new one does not answer.
- `observability/`: the blackbox probes and the Alloy file that ships the journal and the access log to Loki ([Monitoring](#monitoring)).
- `setup-host.sh`: installs the files in `observability/` into the folders the host's Prometheus and Alloy load, and with a key, the user CI deploys as.

## One-time setup

As root on the server, with the binary from `go tool task build` and this folder copied over:

```sh
useradd --system --home /var/lib/tickit --shell /usr/sbin/nologin tickit
install -m 0644 tickit.service tickit-backup.service tickit-backup.timer /etc/systemd/system/
install -m 0755 tickit-deploy /usr/local/bin/
systemctl daemon-reload
systemctl enable tickit.service tickit-backup.timer
tickit-deploy ./tickit
systemctl start tickit-backup.timer
```

Put the real domain into `tickit.caddy` and `observability/tickit.targets.yml`, point its DNS records at the host (Caddy asks for the certificate when it loads the block), and install the block where the host's Caddyfile imports it:

```sh
install -m 0644 tickit.caddy /etc/caddy/sites/
caddy validate --config /etc/caddy/Caddyfile
chown caddy:caddy /var/log/caddy/tickit-access.log
systemctl reload caddy
bash setup-host.sh
```

Run as root, `caddy validate` creates the access log owned by root, and the reload then fails: the `chown` fixes it. `setup-host.sh` installs the monitoring files (below).

## Deploying

`tickit-deploy <binary>` checks that the file is an ELF binary that prints its version (it runs `--version` as the `tickit` user, without network, for ten seconds at most), keeps the running binary aside, restarts `tickit` and waits up to 30 seconds for `/healthz` to answer `ok <new version>`. If it does not, the script puts the old binary back and restarts again. `tickit-deploy rollback` returns to the binary that ran before the last good deploy.

CI can deploy by piping the binary over SSH into `tickit-deploy -`, as a user whose key may run that one command only: `restrict,command="sudo -n /usr/local/bin/tickit-deploy -"` in its `authorized_keys`, and a sudoers line that allows exactly that command. `setup-host.sh` creates such a user when given the public half of CI's key: run it as root, from a copy of this folder (`bash setup-host.sh tickit-deploy-key.pub`). lith's `.github/workflows/deploy-site.yml` is a workflow that deploys this way.

## Monitoring

A host that runs several apps keeps its own configs for Caddy, Prometheus, Alloy and Grafana, and each loads a folder of files that the apps install. lith's guide [Metrics and monitoring](https://lith.zweiundeins.gmbh/docs/guides/monitoring#drop-in-files-for-a-shared-host) describes the folders and the labels. `setup-host.sh` installs tickit's:

- `observability/tickit.targets.yml` as `/etc/prometheus/targets/tickit.yml`: blackbox probes of `/` and `/healthz` (whose body must start with `ok `), labelled `service: tickit`. Prometheus watches the folder.
- `observability/tickit.alloy` as `/etc/alloy/tickit.alloy`: the journal as `{job="tickit"}`, with slog's level as the label `level`, and Caddy's access log as `{job="caddy", site="tickit"}`. The script reloads Alloy and puts the old file back if Alloy refuses the new one.

Each goes in only where its service is installed; run the script again after changing them.

tickit serves no metrics. To add them: register `lith/prom` and `lith/dbmetrics/prom` on a registry served on a loopback port (lith's `site/cmd/lith-site` does it at the public port + 100), let Prometheus reach that port (the unit's `IPAddressDeny=localhost` blocks loopback both ways; lith's `site/deploy/lith-site.service` allows localhost and limits the ports it may bind), and add `observability/tickit.metrics.yml`, a target labelled like lith's `site/deploy/observability/lith-site.metrics.yml`, which `setup-host.sh` installs as `/etc/prometheus/apps/tickit.yml`. The host's "lith apps" dashboard and alert rules then show tickit.

## Backups

The copies stay on the server's disk, so copy `/var/lib/tickit/backups` off the machine as well (restic, rsync). To restore one, stop `tickit`, delete `tickit.db-wal` and `tickit.db-shm` in `/var/lib/tickit`, put the copy in place with `install -o tickit -g tickit -m 0600 <copy> /var/lib/tickit/tickit.db`, and start `tickit`.

## Litestream with two slots

For a continuous replica in object storage, and a rollback that is a cutover instead of a reinstall, an tickit can run two slots of one unit (`tickit@blue`, `tickit@green`) on one data directory. Only one slot runs at a time, since Litestream allows one replicator per database. lith's `backup` module restores a missing database before `sqlite.Open` and replicates while the process runs; the database is then opened with `sqlite.CheckpointExternal`. Caddy proxies to both ports with `lb_try_duration` and `health_uri /readyz`, so requests wait through the few seconds of a handoff.
