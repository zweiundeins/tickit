# tickit architecture

tickit is a multiplayer issue tracker and wiki on lith v0.3.0: one Go binary, one SQLite file, server-rendered views pushed over SSE, Datastar 1.0.4 (Starbase's patched build with Rocket) in the browser. `docs/requirements.md` says what it must do; this file says how the code is cut, so that several people (and agents) can work on it at once without stepping on each other. Requirement IDs (FR-11.2, MP-08.3) are cited in code comments and test names where a test proves a criterion.

## Packages

| Package | Holds | Persistence |
| --- | --- | --- |
| `cmd/tickit` | wiring: `serve`, `backup`, `seed`, `--version`; flags and `TICKIT_*` environment | |
| `internal/people` | persons (guests, accounts, seeded people), the identity middleware, names and faces, login sessions, passwords, site roles, the security log | relational |
| `internal/access` | rights per role (requirements 3.2), scopes (projects and spaces as guard rows), memberships, `Can` | relational, written by commands |
| `internal/tabs` | tab rows, the `Act` wrapper, outcomes and refusals, the tab view, preferences, the sweep | relational |
| `internal/tracker` | projects, workflows, issues, comments, links, labels, components, watchers, board ranks, sprints, saved filters | event-sourced, projections |
| `internal/wiki` | spaces, pages, page versions, page comments, templates, trash, references between pages and issues | event-sourced, projections |
| `internal/doc` | the rich-text document: schema, ProseMirror steps applied in Go, HTML rendering, plain text, references, diffs | none (pure Go) |
| `internal/search` | the FTS5 index over issues and pages (a projection), queries | projection |
| `internal/notify` | notifications, the catch-up projectors that create them | relational |
| `internal/activity` | feeds read from `lith_events` | none |
| `internal/demo` | the seed, the nightly reset, rate-limit classes, content limits, the word list, moderation | mixed |
| `internal/ui` | the design system: templ components (shell, buttons, fields, menus, dialogs, panels, avatars, chips, icons) | none |
| `internal/views` | the app's pages and regions, built from `internal/ui` | none |
| `internal/web` | routes, handlers, live views, static files | none |
| `internal/browsertest` | headless Chromium and Firefox for browser tests (from lith's site) | none |
| `editor/` | source of the `sb-rich-editor` Rocket component (ProseMirror); its bundle is built into `internal/web/static/editor/` and committed | |
| `e2e/` | browser tests of the multiplayer criteria; skipped without Chromium | |

Domain packages never import `internal/web` or `internal/views`. `internal/views` imports domain packages for their read types. `internal/web` imports everything.

## Ids and keys

- Issues, pages, projects, spaces and comments have UUIDv7 strings as ids (time-ordered, so they satisfy lith's rule 10 and Q3). People see issue keys (`WEB-12`) and page titles.
- Persons: a guest is `g-` and 20 characters of base32 of SHA-256 of the session id; an account is `a-` and a UUIDv7; a seeded person is `s-` and a slug (`s-ada`). The session id never appears on a page, in an event or in a log line.
- Issue numbers per project come from the project's stream state; a guard table `issue_keys (project_id, number)` with a primary key, written by the creating command, keeps them unique (NFR-18).

## Identity and access

- `edge.Sessions` gives every browser an opaque session id. `people.Middleware` resolves it to an actor for the request: a login session row (`login_sessions`, keyed by SHA-256 of the session id, not expired) gives the account's person; otherwise the guest person derived from the session. `people.Actor(ctx)` returns it. GET never writes: a guest's row in `people` appears with the first command that needs it (`people.Ensure(tx, actor)`), and a guest's name and face derive from the id alone, so a guest without a row still renders.
- `access.Can(q, actor, right, scope)` answers from `scopes` and `memberships`, which commands write directly (no projector writes them). Every command that changes a project or a space checks its right inside `Apply`, from those tables or from the stream's state row, and refuses with a `*writer.Refusal` whose message says why (NFR-01).
- Under Q14 a guest has Member rights in every open scope; staff log in. Site roles: none, moderator, admin.

## Actions

Every change is `POST /cmd/<area>/<action>` with a Datastar payload `{tab, op, ...}` (NFR-02). The handler, in `internal/web`:

1. checks the rate-limit class of the action per session and per client address (DM-04, `edge.ClientKey`),
2. reads the payload with `sse.ReadSignalsLimit` and validates its shape (400 on malformed input, never 500, NFR-16),
3. takes the actor from the session, never from the payload,
4. runs `w.Do(ctx, writer.Once(sid+":"+op, tabs.Act{...Cmd: cmd}))` and answers 204, or 400, 403, 404, 429, 503 for requests that reached no outcome.

`tabs.Act` runs the command in a savepoint and records its outcome in the tab's row whatever it was: the op id, and for a refusal its message and the field it belongs to. The tab row keeps the newest outcomes (not only the last), so two quick actions in one tab both settle. The tab view patches them as the signal `_outcomes` (op id to `{ok, msg, field}`), which the page's controls read to end their pending state (MP-09) and to show a refusal next to the control.

An action whose answer the page needs, such as the key of a new issue to navigate to, uses `writer.OnceResult` and answers with a signal patch only (`_goto`), never with elements.

## Streams, views and topics

- Each document opens one stream, `GET /stream?tab=<tab>&v=<view>,<view>`, from `data-init` on `#stream`. Each `v` names a view factory registered by an area (`issue:<id>`, `board:<projectID>`, `list:<hash>`, `page:<id>`, `tree:<spaceID>`), and the stream handler builds the views after checking that the actor may read them. Every stream also gets the tab view, the person's view (bell, unread count) and the demo view (reset notice).
- The stream's `Spec.ID` is `<personID>.<tab>`, so `hub.Watchers(key)` yields who watches a view without a read (MP-01, MP-02).
- A side panel or a tab switch adds a view to the open stream with `POST /cmd/watch` (`hub.Watch`), and records it in the tab row so that a reconnect restores it.
- A view's key holds everything its render depends on. Shared views render nothing about the viewer; per-viewer parts go in `tab:<id>`, `person:<id>` or signals.
- Topics: `project:<id>`, `issue:<id>`, `issues` (any change to any issue: lists, search, chips), `board:<projectID>`, `space:<id>`, `page:<id>`, `pages` (any page change), `person:<id>`, `people` (names and faces), `tab:<id>`, `demo`. A command touches every topic whose views it changes.

## Event sourcing

- Streams: `project`, `issue`, `space`, `page`, `comment` (issue and page comments). Kind and type names are permanent once the demo runs.
- Each domain package exports `Parts() []events.Part` (its projectors and upcasters); `cmd/tickit` builds one `events.Store` from all of them and starts one `events.Runner`.
- Deciders decide from the stream's state row, never from a projection. Pages render from projections. Inline projectors keep read models; reactions (notifications) are catch-up projectors that send their command with `writer.Once` keyed by the event's position.
- Every event carries `Meta{At, Actor: personID, Causation: opID}` (FR-155).
- Per-field conflicts (MP-08): an issue's state row records, per field, the stream version that last changed it, who changed it and when. A change carries the version the page showed; it applies when its field has not changed since, and is refused otherwise with the message of MP-08.3.

## Migrations

`migrations/NNN_name.sql`, applied at startup. Numbers are taken by area, so parallel work does not collide; gaps are allowed:

| Numbers | Area |
| --- | --- |
| 001 to 009 | people, access, tabs, security log |
| 010 to 029 | tracker |
| 030 to 049 | wiki |
| 050 to 059 | notify, activity |
| 060 to 069 | search |
| 070 to 089 | demo, moderation, images |

Projection tables are created by migrations too, since the runner needs them to exist.

## Frontend

- The design system lives in `internal/ui` (templ components) and `internal/web/static/css/` (tokens first, then base, then components, then one file per area). Pages use tokens, never raw colours or sizes. `/design` renders every component, in light and dark.
- Starbase components are vendored under `internal/web/static/vendor/starbase/`; tickit's own Rocket components (`sb-rich-editor`) are built from `editor/`.
- Datastar expressions live in Go constants (`internal/views/expr.go` and per-area files). No value from a person goes into an expression or a script (NFR-13); it reaches the page as escaped HTML or as a signal.
- Every page works by keyboard (NFR-47), at 320 CSS pixels (NFR-46), with `lang="en"` and English relative times whose title is the absolute time (NFR-51).

## Tests

- `go test ./...` runs everything that needs no browser. The race detector is slow: run `go tool task test-race` at milestones, not after every change.
- Every decider has `eventstest` cases; every event type a JSON sample in `testdata/<kind>/<type>.json` checked by `eventstest.Samples`; a test rebuilds every read model from the events (FR-155.2).
- `internal/web` has helpers that run the server in process, open a stream, post an action as a browser would (Datastar and `Sec-Fetch-Site` headers) and read frames.
- `e2e/` drives real browsers with `internal/browsertest`; it skips when no Chromium is found (`CHROME` names the binary; `/usr/bin/chromium` on the development machine).
