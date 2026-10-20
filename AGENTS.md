# tickit: notes for coding agents

tickit is a Go, SQLite and Datastar application on lith (github.com/zweiundeins/lith), started from lith's template. Read the lith skill before changing code. Its rules shape every change here (one writer, commands that touch topics, views rendered once per tick for all tabs, outcomes on the stream, tab state in SQLite), and its references hold the recipes, the review checklist and the Datastar gotchas. The skill ships with lith, so it matches the lith version in go.mod:

    $(go list -m -f '{{.Dir}}' github.com/zweiundeins/lith)/skills/lith/SKILL.md

## Layout

- `cmd/tickit`: the binary (`serve`, `backup`, `account create`, `--version`) and its wiring: database, event store and runner, writer, render loop, the metrics listener (`-metrics`, loopback only), `/healthz` and `/readyz`, shutdown, the hourly sweep. Every flag can also come from the environment (`-db` as `TICKIT_DB`, `-session-ttl` as `TICKIT_SESSION_TTL`). `parts()` in `main.go` is where each domain's `Parts()` joins the one event store. Defaults: `127.0.0.1:7105`, metrics on `127.0.0.1:7205`, `data/tickit.db`; `go tool task dev` serves on `127.0.0.1:8080` without metrics.
- `internal/people`: persons (guests from a hash of the session, accounts, the six seeded people), names and faces (`avatar.go`, from lith's board demo), `Middleware` and `Actor(ctx)`, `Ensure`, `Lookup`, `Rename`, passwords (argon2id), `Verify` and the `Login`, `Logout`, `CreateAccount`, `LogSecurity` and `Sweep` commands, the in-memory `Lockout`.
- `internal/access`: the rights of requirements 3.2 as a table, `Can`, `SiteCan`, `Require` (an `*access.Denial`, answered with 403), `ErrNotFound`, scopes and memberships (`AddScope`, `SetMember`, `RemoveMember`, `SetArchived`).
- `internal/tabs`: the tab's row (`State`: outcomes, area states, watched views), `Act` and `By`, `Refuse(field, msg)`, `Load`/`Store`/`Update` for an area's state, the tab view (`View`, the signal `_outcomes`), `Sweep`.
- `internal/demo`: the nightly reset's notice (`Notice`, `NextReset`); the demo area adds the seed and the reset.
- `internal/views`: the document (`Layout`, `Shell`), the person menu, faces and presence rows, the home, login, privacy, 404 and 410 pages, and the Datastar expressions (`expr.go`: `Act`, `Pending`, `Busy`, `Settle`, `Failed` for areas).
- `internal/web`: the server (`server.go`: `New`, `Handler` with the edge chain, `get` and `post` registration), actions (`action.go`: `Payload`, `decode`, rate-limit classes, `act`, `actResult`), pages (`render.go`: `render`, `notFound`, `gone`), the stream (`stream.go`: the view registry, presence, `/cmd/watch`), metrics (`metrics.go`), and one `routes_<area>.go` per area: each area adds its routes and views in its own file only. `static/vendor` holds Starbase's patched Datastar build; its `VERSION` says which. lith's `newest-frame.js` (`live.NewestFrame`) is served beside them as `/static/lith/newest-frame.<hash>.js`, and the page loads it after Datastar. Scripts and stylesheets go out minified (`edge.Minify(minify.Bytes)`, from `github.com/zweiundeins/lith/minify`), and as written with `-dev`.
- `internal/web/webtest`: the server in process for tests (`Start`, `Client`, `Get`, `Post`, `Listen`, `Next`, `Silent`, `Fingerprint`, `Account`, `Login`).
- `internal/dbtest`: a migrated database and writer for command tests (`Open`, `Do`, `View`, `Touched`, `Run` for an events runner).
- `internal/browsertest`: lith's headless Chromium and Firefox driver (MIT), for `e2e/`.
- `e2e`: browser tests; `go tool task e2e` (`CHROME`, and `LITH_CHROME_NO_SANDBOX=1` where Chromium has no sandbox).
- `migrations`: `NNN_name.sql`, applied in order at startup. Add a file for every schema change, never edit one that has shipped.
- `deploy`: systemd units, the Caddy site block, deploy script, backups, and in `deploy/observability` the probes and log shipping a host's Prometheus and Alloy load (`deploy/README.md`).
- `benchmarks/RESULTS.md`: measured numbers, each with its date, machine and command.

## Commands

- `go tool task dev`: serve on http://127.0.0.1:8080 with `-dev`, static files as written, and rebuild on every change.
- `go tool task test`: vet, gofmt, `go test ./...`. `go tool task e2e`: the browser tests. `go tool task test-race` adds the race detector, which is several times slower: run it at milestones, not after every change.
- `go tool task build`: the static binary.
- `go tool templ generate` after editing a `.templ` file. The `_templ.go` files are committed.

## Project rules

- `docs/requirements.md` says what tickit must do; `docs/architecture.md` says how the code is cut (packages, ids, actions, views and topics, events, migration numbers by area). Read both before a change, and keep the second true.
- Cite the requirement a test proves in its name or a one-line comment (`// FR-11.2`).
- Every change is a `POST /cmd/...` action through `tabs.Act`; the actor comes from the session. No route runs a command the client names.
- UI text and docs are English. In prose, use no em dashes or en dashes.
- Pages use the design system (`internal/ui`, `static/css`), never raw colours or sizes.
- Browser tests live in `e2e/` and skip without Chromium (`CHROME=/usr/bin/chromium`).
