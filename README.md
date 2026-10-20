# tickit

tickit is an issue tracker in the manner of Jira and a wiki in the manner of Confluence, used by several people at once: open a board, a page or an issue and see the others on it and their changes as they happen. It runs as a public demo of [lith](https://github.com/zweiundeins/lith) at tickitdemo.zweiundeins.gmbh: one Go binary, one SQLite file, pages rendered on the server and kept current over SSE with Datastar.

Visitors need no account: each browser is a guest with a pseudonym and a face. Accounts are for staff, who moderate and run the demo.

## A short tour

- **Projects and issues.** Projects with a workflow, components, sprints and WIP limits; issues with type, status, priority, assignee, labels, points, estimates, due dates, parents, links, watchers and comments with mentions. Every issue has a history and can be shown as it was at any earlier version.
- **Board and backlog.** Drag a card, or move it by keyboard from its menu. The card moves at once, marked pending, and shows on every other open board a tick or two later, with a short animated move. Of two people who move the same card at once, the second sees "Sunny Otter moved WEB-12 to In Review first", and nothing of their move is stored.
- **Issue list, search and saved filters.** A filtered, sorted, paged list that every tab with the same filter shares as one live view; a search over issues, pages and comments with a typeahead in the header; CSV export.
- **Spaces and pages.** A page tree per space, templates, labels, the trash, versions with a slider through them and a comparison of two versions, comments with replies, and chips that show an issue's live status inside a page.
- **Co-editing.** Up to 20 people type in the same paragraph at once. The server orders and rebases every edit, so nobody's keystroke is lost; other people's carets and selections show in their colours, and the editor says "Saving" and "Saved".
- **Multiplayer everywhere.** Faces of the people on an issue, a board or a page; "Sunny Otter is editing the description"; a field conflict that says who changed the field and offers to save yours anyway; notifications in a bell that every tab of the person updates; reconnect after a deploy without a reload.
- **The demo.** Seeded content, a nightly reset at 04:00 Zurich time announced ten minutes ahead, rate limits and caps for guests, and moderation and administration pages for staff.

## Run it in development

You need Go 1.27.2 (`go` fetches it with `GOTOOLCHAIN=auto`).

    go tool task dev     # http://127.0.0.1:8080, seeded on first start, rebuilt on every change

`go tool task dev` serves with `-dev`, which writes the demo's seed into an empty database (`data/tickit.db`): three projects with their issues and history, two spaces with pages, versions and comments, all by six seeded people over the last 30 days. To seed by hand: `go run ./cmd/tickit seed -db data/tickit.db` (`-force` empties a database first and keeps its staff accounts). After editing a `.templ` file, run `go tool templ generate`; the generated files are committed.

`tickit serve` listens on `127.0.0.1:7105` and serves metrics on `127.0.0.1:7205` (`-metrics ""` turns them off). `tickit serve -h` lists every setting; each can also come from the environment, `-db` as `TICKIT_DB`.

## Run the tests

    go tool task test        # vet, formatting, go test ./...
    go tool task test-race   # the same with the race detector: slow, for milestones
    go tool task e2e         # browser tests in headless Chromium: CHROME=/usr/bin/chromium, LITH_CHROME_NO_SANDBOX=1 where it has no sandbox
    go tool task security    # the security suite, the fuzzers, the binary's security smoke check, govulncheck
    go tool task load        # the load of NFR-38 against the built binary (below)
    go tool task smoke       # the built binary as the host runs it
    go tool task vuln        # govulncheck

- **Unit and server tests** run the server in process behind its whole edge (`internal/web/webtest`) and commands on a fresh database (`internal/dbtest`). Tests name the requirement they prove (`// FR-11.2`); [docs/status.md](docs/status.md) lists them per requirement.
- **Browser tests** in `e2e/` drive Chromium and Firefox through `internal/browsertest` and skip when no browser is found. Some take more settings: `COEDIT_EDITORS` and `COEDIT_SECONDS` for co-editing in browsers, `LOAD_EDITORS` and `LOAD_SECONDS` for the same with Go clients, `TICKIT_SHOTS=<dir>` for screenshots.
- **Security tests** are part of `go test ./...` (rights per role, cross-site posts, headers, rate limits, malformed input to every action, every GET leaving the database unchanged, no secret in a server log); `go tool task security` adds a fuzz run of every family of actions (`FUZZTIME`, 30 s each by default), `deploy/security-smoke.sh` against the production binary and govulncheck. [docs/security.md](docs/security.md) has the threat model, the controls and the test of each.
- **The load test** (`cmd/tickit-load`) writes 10,000 issues, 2,000 pages and 1,000,000 events through the real commands once (about 10 minutes, into `LOAD_DIR`, `/tmp/tickit-load` by default), starts the binary as a process of its own with the unit's `GOMEMLIMIT`, opens 200 streams as browsers do, sends the actions of NFR-38, restarts the server and checks that every stream comes back. `LOAD_SECONDS` sets the measured time (60), `LOAD_SUSTAIN` a longer run for the growth of the database (`25m`), and `SERVER_CPUS` and `LOAD_CPUS` pin the two processes to different cores (`SERVER_CPUS=0-5 LOAD_CPUS=12-19`). [benchmarks/RESULTS.md](benchmarks/RESULTS.md) has the numbers.

## Make a staff account

On the host, as the user that runs tickit:

    TICKIT_PASSWORD='…' tickit account create -db /var/lib/tickit/tickit.db \
        -email ada@example.com -username ada -name 'Ada Brunner' -role admin

The role is `none`, `moderator` or `admin`. Without `TICKIT_PASSWORD`, the password is read from the first line of standard input; it is never a flag.

## The repository

| Path | What it holds |
| --- | --- |
| `cmd/tickit` | the binary: `serve`, `backup`, `account create`, `seed`, `migrations-pending`, and its wiring |
| `cmd/tickit-load` | the load test of NFR-38: `gen` writes the data, `run` drives the binary |
| `cmd/vendor-starbase` | copies Starbase's components and fonts from a checkout |
| `internal/` | the domain packages (`people`, `access`, `tabs`, `tracker`, `wiki`, `doc`, `markup`, `search`, `notify`, `activity`, `demo`), the design system (`ui`), the pages (`views`) and the server (`web`); [AGENTS.md](AGENTS.md) says what each holds |
| `editor/` | the source of the `sb-rich-editor` component (ProseMirror); its bundle is committed in `internal/web/static/editor/` |
| `e2e/` | browser tests of the multiplayer criteria |
| `migrations/` | the SQL migrations, applied at startup |
| `deploy/` | systemd units, the Caddy block, the deploy script, backups, monitoring |
| `benchmarks/RESULTS.md` | measured numbers with date, machine and command |
| `docs/` | requirements, status, architecture, the editor, security, accessibility, the design system, deploying |

## Read on

- [docs/requirements.md](docs/requirements.md): what tickit must do.
- [docs/status.md](docs/status.md): which requirements are done, and the tests that prove them.
- [docs/architecture.md](docs/architecture.md): how the code is cut.
- [docs/editor.md](docs/editor.md): how co-editing works.
- [docs/security.md](docs/security.md): the threat model, the controls and their tests.
- [docs/accessibility.md](docs/accessibility.md): the accessibility checks (axe with `TICKIT_AXE`, 320 pixels, a keyboard walk) and what is open.
- [docs/design-system.md](docs/design-system.md): tokens and components.
- [docs/deploy.md](docs/deploy.md) and [deploy/README.md](deploy/README.md): the host, deploys, backups and monitoring.
- [benchmarks/RESULTS.md](benchmarks/RESULTS.md): the measured numbers.
- [AGENTS.md](AGENTS.md): the layout and the project's rules, for people and coding agents.
