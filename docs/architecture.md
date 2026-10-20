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
| `internal/demo` | the seed, the nightly reset and its notice, content limits, the word list, moderation | mixed |
| `internal/ui` | the design system: templ components (shell, buttons, fields, menus, dialogs, panels, avatars, chips, icons) | none |
| `internal/views` | the app's pages and regions, built from `internal/ui` | none |
| `internal/web` | routes, handlers, live views, the edge chain, rate-limit classes, static files | none |
| `internal/web/webtest` | the server in process behind the whole edge, a browser's requests, streams read event by event, a fingerprint of the database (tests only) | none |
| `internal/dbtest` | a migrated database with a writer whose commits are recorded by topic, an events runner (tests only) | none |
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
- `access.Can(ctx, q, actor, right, scope)` answers from `scopes` and `memberships`, which commands write directly (no projector writes them; `access.AddScope` writes a scope with its creator as Admin). Every command that changes a project or a space checks its right inside `Apply` with `access.Require(tx, actor, right, scope)`, which refuses with an `*access.Denial`: a `*writer.Refusal` whose message says why, which the action answers with 403 (NFR-01). A scope the actor may not read answers `access.ErrNotFound`, as an unknown one does.
- Under Q14 a guest has Member rights in every open scope; staff log in. Site roles: none, moderator, admin.

## Actions

Every change is `POST /cmd/<area>/<action>` with a Datastar payload `{tab, op, ...}` (NFR-02). The handler, in `internal/web`:

1. checks the rate-limit class of the action per client address, then per person (DM-04, `edge.ClientKey`; the classes are in `internal/web/action.go`),
2. reads the payload with `sse.ReadSignalsLimit` and validates its shape (400 on malformed input, never 500, NFR-16),
3. takes the actor from the session, never from the payload (`tabs.By{Actor, Op, At}`, which commands carry and turn into `events.Meta` with `By.Meta()`),
4. runs `w.Do(ctx, writer.OnceResult(sid+":"+op, tabs.Act{...Cmd: cmd}))` and answers 204, or 403 when the command was refused for want of a right, or 400, 404, 429, 503 for requests that reached no outcome.

In an area's handler that is `a, ok := s.decode(w, r, classField, &req)`, then `s.act(w, r, a, cmd)`. `OnceResult` rather than `Once`, so that a retry gets the first answer (a denial's 403, a result) while `tabs.Act` records refusals as outcomes either way.

`tabs.Act` gives a guest acting for the first time a row in `people`, runs the command in a savepoint and records its outcome in the tab's row whatever it was: the op id, and for a refusal its message and the field it belongs to (`tabs.Refuse(field, msg)`). The tab row keeps the newest 16 outcomes (not only the last), so two quick actions in one tab both settle. The tab view patches them as the signal `_outcomes` (op id to `{ok, msg, field}`), which the page's controls read to end their pending state (MP-09) and to show a refusal next to the control (`views.Act`, `views.Settle`, `views.Failed` build those expressions).

An action whose answer the page needs, such as the key of a new issue to navigate to, uses `s.actResult` and answers with a signal patch only (`_goto`), never with elements.

Login and logout are actions too (`POST /cmd/login`, `POST /cmd/logout`), outside `tabs.Act`: they answer with signals, the login form's message or `_goto`. The password is checked outside the writer (argon2id is too slow for it); the `people.Login` command checks again that nothing changed and writes the login session.

## Streams, views and topics

- Each document opens one stream, `GET /stream?tab=<tab>&v=<view>,<view>`, from `data-init` on `#stream`. Each `v` names a view factory registered by an area (`issue:<id>`, `board:<projectID>`, `list:<hash>`, `page:<id>`, `tree:<spaceID>`) with `s.views.Register(kind, f)`, where `f(ctx, q, actor, arg) (live.View, error)` checks that the actor may read it (`access.ErrNotFound` answers 404, a denial 403, an unknown kind 400). Every stream also gets the person's view (`person:<id>`: name, bell, unread count), the demo view (`demo`: the reset notice), a `connected` view that hides "Reconnecting", the session's prefs view (`prefs:<tab>`, the signal `_prefs`), and the tab view last. The page's views reach `openStream` as the signal `_views`, never inside an expression.
- The stream's `Spec.ID` is `<personID>.<tab>`, so `hub.Watchers(key)` yields who watches a view without a read (MP-01, MP-02). A kind registered with `s.views.RegisterWithPresence` makes every stream that watches one of its views watch `presence:<key>` too: a shared view with `Presence: true` that lists one face per person in the order they came, with the tab count in the tooltip. The page puts `views.Presence(views.PresenceID(key), nil)` where the row goes (a shared `ui.PresenceRow`); the document's own style puts the viewer's face first, outlined, and shows at most 8.
- A side panel or a tab switch adds a view to the open stream with `POST /cmd/watch` (`hub.Watch`), and records it in the tab row so that a reconnect restores it; `POST /cmd/unwatch` takes it off.
- A view's key holds everything its render depends on. Shared views render nothing about the viewer; per-viewer parts go in `tab:<id>`, `person:<id>` or signals.
- Topics: `project:<id>`, `issue:<id>`, `issues` (any change to any issue: lists, search, chips), `board:<projectID>`, `space:<id>`, `page:<id>`, `pages` (any page change), `person:<id>`, `people` (names and faces), `tab:<id>`, `demo`. A command touches every topic whose views it changes.

## Pages and the document

A page handler answers with `s.render(w, r, status, build)`, where `build` reads in the request's one read transaction and returns a `web.Page`:

```go
return Page{
	Title:      "Board",                 // the document's title adds "tickit"
	Nav:        "projects",              // the header link marked current (ui.MainNav), or "moderation", "admin"
	Sidebar:    ui.SideNav(sideNav),     // nil for none; SidebarLabel names it ("Project" by default)
	Width:      "full",                  // "" (80rem), "prose" (46rem), "full"
	Views:      []string{"board:" + id}, // the stream's views, besides the person's, the demo's, the prefs' and the tab's
	Components: []string{"select"},      // Starbase components, loaded with what they render
	Body:       views.Board(b),
}, nil
```

`Query` fills the header's search box, `Scripts` loads further modules by their static name, and `Create` replaces the header's create dialog (id `views.CreateID`, a placeholder until the tracker brings one). `views.Layout` renders the rest: the head with the bundle, the favicon, the font preloads, the import map with the CSP nonce, Datastar and `newest-frame.js`; `#stream`; `ui.Shell` with the header, the demo notice, the first-visit hint (DM-11) and the footer; "Reconnecting" (MP-10.1) and `$_problem` as floating notices.

Display preferences (theme `system`, `dark` or `light`; style `pixel` or `smooth`; density `comfortable` or `compact`; a dismissed demo notice; a dismissed hint) are the session's row in `prefs` (`tabs.Prefs`, `tabs.GetPrefs`). `POST /cmd/prefs {tab, op, key, value}` sets one through `tabs.Act` (`tabs.SetPref`, 400 for an unknown key or value) and touches `tabs.PrefsTopic(sid)`; every stream watches `tabs.PrefsView`, which patches `$_prefs`, and the document puts it on `<html>`, so every tab of the session follows. GET draws them from the first byte (`ui.Prefs`); no cookie carries them (NFR-31).

A person's face is `ui.FaceOf(id)`, and their hue an index of 0 to 15 (`people.Person.Hue`, `data-hue`; the stylesheet turns it into degrees, times 22.5). `views.UIPerson` is the one place that makes a `ui.Person` of a `people.Person`; seeded people keep their own hue.

## Event sourcing

- Streams: `project`, `issue`, `space`, `page`, `comment` (issue and page comments). Kind and type names are permanent once the demo runs.
- Each domain package exports `Parts() []events.Part` (its projectors and upcasters); `cmd/tickit` builds one `events.Store` from all of them and starts one `events.Runner`.
- Deciders decide from the stream's state row, never from a projection. Pages render from projections. Inline projectors keep read models; reactions (notifications) are catch-up projectors that send their command with `writer.Once` keyed by the event's position.
- Every event carries `Meta{At, Actor: personID, Causation: opID}` (FR-155).
- Per-field conflicts (MP-08): an issue's state row records, per field, the stream version that last changed it, who changed it and when. A change carries the version the page showed; it applies when its field has not changed since, and is refused otherwise with the message of MP-08.3.

## The edge

`web.Server.Handler` puts the edge around the routes, inside out: the actor (`people.Middleware`), `edge.Sessions` (`__Host-sid` with `-secure`), `edge.CSRF` with `Datastar: ["POST /cmd/"]` and no exemptions, `edge.SecurityHeaders` with `DefaultCSP` and HSTS with `-secure`, `X-Robots-Tag: noindex, nofollow`, `edge.Compress`, `edge.Recover`, `edge.LogRequests`. `cmd/tickit` mounts `/healthz` and `/readyz` beside it and serves metrics on a loopback listener of their own (`-metrics`, 127.0.0.1:7205). Pages register with `s.get(pattern, handler, probes...)` and actions with `s.post`, which refuses anything outside `POST /cmd/`.

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
- `internal/web/webtest` runs the server in process behind the whole edge, opens a stream, posts an action as a browser would (Datastar and `Sec-Fetch-Site` headers) and reads frames; `internal/dbtest` runs commands against a fresh database and records the topics they touch.
- GET never writes (NFR-06): every GET route registers with `s.get` and a probe URL (a pattern with wildcards needs one), and `TestGetNeverWrites` requests every probe as four kinds of visitor and compares a fingerprint of every table before and after. An area that adds a page with an id in its path gives a probe that names something its test data holds.
- `e2e/` drives real browsers with `internal/browsertest`; it skips when no Chromium is found (`CHROME` names the binary; `/usr/bin/chromium` on the development machine).
