# Deploying tickit

[docs/deploy.md](../docs/deploy.md) has the first install, deploys, rollbacks, backups and restores, the monitoring and every setting. The files here:

- `tickit.service`: the systemd unit. It runs as `tickit`, keeps the database in `/var/lib/tickit`, serves on `127.0.0.1:7105` and its metrics on `127.0.0.1:7205`, connects nowhere, may bind those two ports only, and is capped at 640 MB of memory with `GOMEMLIMIT=448MiB` for `-streams 200`.
- `tickit.env.example`: optional settings, installed as `/etc/tickit/tickit.env` with mode 0600.
- `tickit-backup.service` and `tickit-backup.timer`: every day at 03:30 Zurich time, `tickit backup` copies the database (VACUUM INTO), checks the copy with `PRAGMA integrity_check`, and keeps the newest 30 in `/var/lib/tickit/backups`.
- `tickitdemo.caddy`: the site block for tickitdemo.zweiundeins.gmbh. TLS, `reverse_proxy` to the loopback port with the client's address in X-Forwarded-For and X-Real-IP, the JSON access log at mode 0640 for 30 days, no `encode`, no file handlers, and `flush_interval -1` for the streams.
- `tickit-deploy`: installs a binary, takes a copy of the database first when the binary migrates it, restarts the unit, waits for `/healthz`, and puts the old binary back if the new one does not answer. `tickit-deploy rollback` returns to the previous binary.
- `observability/`: the metrics target, the blackbox probes, the Alloy file for the journal and the access log, and tickit's Grafana alert rules.
- `setup-host.sh`: installs the files in `observability/` into the folders the host's Prometheus, Alloy and Grafana load, and with a key, the user CI deploys as. It never restarts tickit.
- `smoke-test.sh`: runs a built binary and checks its version, probes, metrics, backup and stop (`go tool task smoke`).
