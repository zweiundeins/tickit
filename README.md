# tickit

tickit is an issue tracker in the manner of Jira and a wiki in the manner of Confluence, used by several people at once: open a board, a page or an issue and see the others on it and their changes as they happen. It runs as a public demo of [lith](https://github.com/zweiundeins/lith) at tickitdemo.zweiundeins.gmbh: one Go binary, one SQLite file, pages rendered on the server and kept current over SSE with Datastar.

Visitors need no account: each browser is a guest with a pseudonym and a face. Accounts are for staff, who moderate and run the demo.

## Run it

You need Go 1.27.2 (`go` fetches it with `GOTOOLCHAIN=auto`).

    go tool task dev     # http://127.0.0.1:8080, rebuilt on every change
    go tool task test    # vet, formatting, tests
    go tool task e2e     # the browser tests, with Chromium (CHROME=/usr/bin/chromium)
    go tool task build   # the static binary ./tickit

`tickit serve` listens on `127.0.0.1:7105` and serves metrics on `127.0.0.1:7205` (`-metrics ""` turns them off); the database is `data/tickit.db`. `tickit serve -h` lists every setting; each can also come from the environment, `-db` as `TICKIT_DB`.

## Make a staff account

On the host, as the user that runs tickit:

    TICKIT_PASSWORD='…' tickit account create -db /var/lib/tickit/tickit.db \
        -email ada@example.com -username ada -name 'Ada Brunner' -role admin

The role is `none`, `moderator` or `admin`. Without `TICKIT_PASSWORD`, the password is read from the first line of standard input; it is never a flag.

## Read on

- `docs/requirements.md`: what tickit must do.
- `docs/architecture.md`: how the code is cut.
- `AGENTS.md`: the layout and the project's rules.
- `deploy/README.md`: the host, deploys, backups and monitoring.
