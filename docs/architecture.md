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
| `internal/markup` | the text markup of the seed and of what people type into descriptions and comments: `Parse` (strict), `ParseMode` (`Text`, `Comment`: what the markup does not define stays text), `Write` (a document back to markup, for editing it) | none (pure Go) |
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
- Issue numbers per project come from the project's stream state (`project.number_assigned`); the guard table `issue_keys (key PRIMARY KEY, project_id, number, issue_id, UNIQUE (project_id, number))`, written by the command that creates or moves an issue, keeps them unique (NFR-18) and keeps every key an issue ever had, so old keys resolve after a move and a deleted issue's key answers 410 (FR-11, FR-12, FR-29).

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

`tabs.Act` gives a guest acting for the first time a row in `people`, runs the command in a savepoint and records its outcome in the tab's row whatever it was: the op id, and for a refusal its message and the field it belongs to (`tabs.Refuse(field, msg)`). The tab row keeps the newest 16 outcomes (not only the last), so two quick actions in one tab both settle. A refusal that implements `OutcomeData() json.RawMessage` adds `data` to its outcome: `tracker.Conflict` does, with who changed the field, when, both values and the version to resend with. The tab view patches them as the signal `_outcomes` (op id to `{ok, msg, field, data}`, `data` only when set), which the page's controls read to end their pending state (MP-09) and to show a refusal next to the control (`views.Act`, `views.Settle`, `views.Failed` build those expressions).

An action whose answer the page needs, such as the key of a new issue to navigate to, uses `s.actResult` and answers with a signal patch only (`_goto`), never with elements.

Login and logout are actions too (`POST /cmd/login`, `POST /cmd/logout`), outside `tabs.Act`: they answer with signals, the login form's message or `_goto`. The password is checked outside the writer (argon2id is too slow for it); the `people.Login` command checks again that nothing changed and writes the login session.

## Streams, views and topics

- Each document opens one stream, `GET /stream?tab=<tab>&v=<view>,<view>`, from `data-init` on `#stream`. Each `v` names a view factory registered by an area (`issue:<id>`, `board:<projectID>`, `list:<query>`, `filters:`, `search:<query>`, `page:<id>`, `tree:<spaceID>`) with `s.views.Register(kind, f)`, where `f(ctx, q, actor, arg) (live.View, error)` checks that the actor may read it (`access.ErrNotFound` answers 404, a denial 403, an unknown kind 400). Every stream also gets the person's view (`person:<id>`: name, bell, unread count), the demo view (`demo`: the reset notice), a `connected` view that hides "Reconnecting", the session's prefs view (`prefs:<tab>`, the signal `_prefs`), and the tab view last. The page's views reach `openStream` as the signal `_views`, never inside an expression.
- The stream's `Spec.ID` is `<personID>.<tab>`, so `hub.Watchers(key)` yields who watches a view without a read (MP-01, MP-02). A kind registered with `s.views.RegisterWithPresence` makes every stream that watches one of its views watch `presence:<key>` too: a shared view with `Presence: true` that lists one face per person in the order they came, with the tab count in the tooltip. The page puts `views.Presence(views.PresenceID(key), nil)` where the row goes (a shared `ui.PresenceRow`); the document's own style puts the viewer's face first, outlined, and shows at most 8.
- A side panel or a tab switch adds a view to the open stream with `POST /cmd/watch` (`hub.Watch`), and records it in the tab row so that a reconnect restores it; `POST /cmd/unwatch` takes it off.
- A view's key holds everything its render depends on. Shared views render nothing about the viewer; per-viewer parts go in `tab:<id>`, `person:<id>` or signals.
- Topics: `project:<id>`, `projects` (any change to any project: the project list, pickers), `issue:<id>`, `issues` (any change to any issue or issue comment: lists, search, chips, open counts), `board:<projectID>` (a project's board and backlog), `filters` (saved filters), `space:<id>`, `page:<id>`, `pages` (any page change), `person:<id>`, `people` (names and faces), `tab:<id>`, `demo`. A command touches every topic whose views it changes.

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

## The issue page

`GET /{key}` (`/WEB-12`, any case) is the issue's page; another spelling or an old key answers 301 to the key now, an unknown key 404 with a search box, a deleted issue's key 410 with who deleted it and when, and `/ticket/<id>` and `/dashboard/<id>` 301 to the key (FR-12). The page is a static frame (`views.IssuePage`, `issueFrame`) holding the regions of three views:

| View spec | Key | Topics | Renders |
| --- | --- | --- | --- |
| `issue:<id>` (with presence) | `issue:<id>` | `issue:<id>`, `people`, `project:<project>`; again every 30 s for relative times | `#issue-main`, `#issue-desc`, `#issue-related`, `#issue-side`, `#issue-comments`, `#issue-history`, `#issue-activity`, `#issue-mentions`, and the signal `$_is.max` (the version) |
| `issue-typing:<id>` | `issue-typing:<id>` | `issue-typing:<id>`, `people`; every second | `#issue-typing-summary`, `#issue-typing-description`, `#issue-typing-comment` (MP-07) |
| `issue-at:<id>.<v>` | `issue:<id>:at:<v>` | `people` | `#issue-asof`: the issue at version v (FR-70); tabs at one version share it |

The regions are siblings, never nested, since a patch skips an element inside a `data-ignore-morph` one. Fields people type in are in the frame (the comment box) or marked `data-ignore-morph` with their value in a `_is` signal (the editors in place), so no frame resets them. Elements the morph would reset keep their state with `data-preserve-attr` (`hidden`, `aria-busy`, `disabled`).

The page's state is the signal `_is`: the issue's id and key, the viewer's id, and per field the pending op (`ops`), the refusal (`msg`), a conflict's data (`conflict`), a request with no outcome (`fail`) and the value sent (`want`). Every change is `POST /cmd/issues/{id}/{field}` with `{tab, op, seen, value, ref}` through `tabs.Act`; `seen` is the version the control showed (a menu keeps the version it opened at). The fields: `summary`, `description` (markup, `markup.Text`), `type`, `status`, `priority`, `assignee` (`me` for the actor), `parent`, `due`, `points`, `estimate`, `remaining` ("2h 30m"), `sprint`, `labels` and `components` (`ref` add or remove), `watch` (toggles), `link` (`value` a key, `ref` the link type), `unlink`, `subtask`, `comment` (markup, `markup.Comment`), `cedit`, `cdelete`, `chide` (`ref` the comment's id), `delete`. `POST /cmd/issues/{id}/typing {tab, op, field, stop}` is a caret-class action sent without waiting: it writes `issue_typing` (migration 011) and touches `issue-typing:<id>`; an indicator goes 5 s after the last keystroke, and a save or a comment ends it at once. Controls in the regions carry `data-act`, `data-field`, `data-value` and `data-ref`, and the frame's one delegated handler runs them, so frames carry attributes rather than expressions.

What the shared views cannot know about the viewer comes from the document: `viewerStyle` shows the viewer's own comments' Edit and Delete, the watch button's state and hides the viewer's own typing indicator; `$_is.ro` makes a Viewer's page read-only (comments stay open) and `$_is.mod` shows Hide to a project's Admin or a moderator. A comment's draft stays in the tab across a reconnect and a reload (`sessionStorage`, which is per tab).

## Issue panel

From a board, a list or a search, an issue opens in `ui.SidePanel` beside the page (FR-13.2). To wire it:

1. Put `@views.IssuePanel(me.ID, moderates)` once in the page's body (`moderates`: `access.SiteCan(me, access.Moderate)`).
2. Give every link to an issue the issue's id in `data-issue-panel`: `<a href="/WEB-12" { views.IssueLinkAttrs(issue.ID)... }>`. Keep the `href`: a click with Ctrl, Cmd, Shift or Alt, a middle click, or a page without the panel follows the link to the issue's page.

A plain click opens the panel, puts the issue's URL in the address bar (`history.pushState`) and adds `issue:<id>` and `issue-typing:<id>` to the tab's stream with `POST /cmd/watch`; the stream fills the panel's regions with the same view the issue page shows. Escape, the close button, a click beside the panel or the browser's Back close it: the views come off the stream (`POST /cmd/unwatch`) and the page's own URL comes back. The page never left, so its filter, its page and its scroll position stay. The page needs no view of its own for the panel and no route.

## Create dialog

`views.CreateIssueDialog` is the header's create dialog (FR-10), with the id `views.CreateID`, so the header's Create opens it (`command="show-modal" commandfor="create"`). `s.render` puts it on every page whose `Page.Create` is nil: it lists the projects the person may create issues in and preselects `Page.Project` (a project's id: set it on a board, a backlog or a project's list), else the first. A board column's "+" opens it with that column's status: give the button `data-on:click={ views.CreateWithStatus }`, `data-status="<status id>"` and `data-project="<project id>"`. `POST /cmd/issues/create {tab, op, project, type, summary, description, priority, assignee, labels, parent, status, stay}` answers `$_goto` to the new issue, or, on a page with the issue panel (`stay`), opens the new issue in the panel. A refusal stays next to its field (`$_new.field`, `$_new.msg`) with what the person typed.

## Event sourcing

- Streams: `project`, `issue`, `space`, `page`, `comment` (issue and page comments). Kind and type names are permanent once the demo runs.
- Each domain package exports `Parts() []events.Part` (its projectors and upcasters); `cmd/tickit` builds one `events.Store` from all of them and starts one `events.Runner`. A package whose commands append events also exports `Bind(*events.Store)`, which `cmd/tickit` calls with that store, so that every package's projectors of a stream kind run in the commands that append to it; without it (tests, tools) the package's commands use a store of its own parts alone.
- Deciders decide from the stream's state row, never from a projection. Pages render from projections. Inline projectors keep read models; reactions (notifications) are catch-up projectors that send their command with `writer.Once` keyed by the event's position.
- Every event carries `Meta{At, Actor: personID, Causation: opID}` (FR-155).
- Per-field conflicts (MP-08): an issue's state row records, per field, the stream version that last changed it (`IssueState.Changed`). Who changed it and when are that event's metadata, which the command reads from the log when it refuses (an `Evolve` sees no metadata); the value is the field's value in the state. A change carries the version the page showed (`Seen`); it applies when its field has not changed since, or when the last change was the actor's own, and is refused otherwise with the message of MP-08.3 as a `*tracker.Conflict`.

## The tracker

`internal/tracker` holds the Jira side; its package documentation lists the API.

- Streams and their event types (permanent): `project` (`project.created`, `.edited`, `.archived`, `.unarchived`, `.component_defined`, `.component_removed`, `.number_assigned`, `.wip_limit_set`, `.sprint_created`, `.sprint_started`, `.sprint_completed`), `issue` (`issue.created`, `.summary_changed`, `.description_changed`, `.type_changed`, `.status_changed`, `.moved`, `.priority_changed`, `.assignee_changed`, `.parent_changed`, `.due_changed`, `.points_changed`, `.estimate_changed`, `.remaining_changed`, `.sprint_changed`, `.labels_changed`, `.components_changed`, `.watchers_added`, `.watcher_removed`, `.link_added`, `.link_removed`, `.deleted`, `.project_changed`), `comment` (`comment.added`, `.edited`, `.deleted`, `.hidden`, `.unhidden`; a comment names its subject, `issue` now and `page` later). Statuses have stable ids (`todo`, `inreview`) and names; issues and URLs hold the ids.
- A command reads the states it decides on with a decider that appends nothing (a peek), checks its right with `access.Require` against the issue's project, then executes. Creating an issue appends to the project's stream (the number) and the issue's in one savepoint; a link appends to both issues; a comment appends to its stream and adds watchers on its issue's.
- The seed acts as seeded people (`people.Seed`) with a past `By.At`; the tracker's access checks let them, since no request acts as one.
- Ranks are fractional indexes (`RankBetween`), one order per project shared by the board's columns and the backlog. A move names its neighbours by id; the command reads their ranks, and the project's last rank for a new issue, from the `issues` projection and hands them to the decider. This departs from "deciders decide from the state row": a rank is no rule, and a stale neighbour only places a card beside another one, so ranks need no guard table.
- Rules across issues live in tables the commands write: `issue_keys`, `issue_parents` (children counts for type changes, deletes and moves, FR-19.2), `tracker_actors` (who acted in a project when: assignable for 24 hours, FR-17.1). Saved filters (`saved_filters`, `saved_filter_stars`) are relational.
- Projections: `tracker_projects` keeps `projects`, `statuses` (workflow order, WIP limits), `components`, `sprints`; `tracker_issues` keeps `issues`, `issue_labels`, `labels`, `issue_links`, `issue_watchers`; `tracker_comments` keeps `comments`. A project's open issue count is counted from `issues` when read, and an issue's status order is joined from `statuses`: a table belongs to one projector, which sees one stream kind, and a workflow reordered later (FR-36) then changes no issue row.
- Search matches text with `instr` on a lower-case column that Go fills (keys, summary, description text; comments have their own), so `%` and `_` match themselves and case folds beyond ASCII. Every filter runs in one statement whose lists are JSON parameters (`json_each`); each sort has its own ORDER BY.

## Notifications

`internal/notify` keeps `notifications` (migration 050): one row per recipient with its kind (`assigned`, `mentioned`, `commented`, `status_changed`, `page_commented`), the subject's kind, id, label (issue key or page title, as it was) and href, the actor, a text (the sentence around a mention, a comment's start), what merged changes add (`detail`), the source position, and read and dismissed times.

- **Reactions.** Two catch-up projectors, `notify_issues` and `notify_comments` (`notify.Parts()`, with `SkipHistory`, so a new projector, a restart or a deploy notifies nobody of the past, FR-116), send one command per event with `writer.Once(notify.OnceKey(position))`. The command reads the issue as the log held it at the event's position (key, watchers), so a retry decides the same; `UNIQUE (recipient, source, kind)` and the merge's `last_source` keep a replay from adding anything once the op ids are pruned. Assignment of someone else (FR-111), a mention a text adds (FR-112, from the events' `mentions` and `mentions_added`), a comment or a status change to a watched issue's watchers but the actor (FR-113), merged into the recipient's unread notification of the same subject and actor from the last minute (FR-113.2). Only guests and accounts are told. `cmd/tickit` calls `notify.Bind(w)`; tests bind their writer.
- **Wiki.** One file, `internal/notify/wiki.go`: its `init` appends the page stream's projector to `extra` (page mentions) and sets `commentables[tracker.SubjectPage]` to `{Kind: PageCommented, At: pageAt}`, where `pageAt(tx, id, upto)` names the page and lists its watchers as of the position (FR-114).
- **Actions** (FR-115), each through `tabs.Act`, 403 for someone else's notification, 404 for an unknown one: `POST /cmd/notify/read {id}`, `/read-all`, `/dismiss {id}`, `/dismiss-all`, `/open {id}` (marks it read and answers `$_goto` with its subject), `/seen {subject}` (marks the person's notifications of a subject read: a page that opens the subject renders `views.NotifySeen(subjectID)` when `notify.HasUnread` says so, since GET never writes). Every change touches `people.PersonTopic(recipient)`.
- **Retention** (FR-117): `notify.Sweep`, in the hourly sweep, deletes dismissed rows, read rows older than 30 days, and each person's beyond the newest 500.
- **The bell.** `web.Server.bell` reads `views.Bell` (`notify.Panel`: unread count, newest 30, how many more; the actors) for every document (`views.Shell.Bell`), and the person view (`person:<id>`) renders it again on every change, for every tab of the person (FR-110.3, MP-01.2). The header (`ui.Header`) renders `ui.BellCount(n)` (`#tk-bell-count`, hidden at 0) in the bell button, which is described by `ui.UnreadStatus(n)` (`#tk-unread`, `role="status"`: the polite live region of NFR-48.2), and `views.NotificationPanel(bell)` (`#tk-notify-panel`) inside the popover `#tk-notifications`. The person view patches those three by id, with the person's name, and the signal `$_unread` (`views.UnreadSignal`), from which the panel's `data-effect` keeps the window title's count, "tickit (3)" (FR-110.2). The panel's own signal is `$_notify.op`.

## Activity and the home page

`internal/activity` reads feeds from `lith_events` and stores nothing (FR-69): `events.SinceUntil` in windows going back from the head (512 positions, doubling, at most 20,000 events), events of the types a source names decoded with the domain's streams, kept when the person may read their subject's scope, consecutive edits of one page by one person within 10 minutes as one entry (FR-69.3). `activity.Home(readable)`, `activity.Project(id)`, `activity.Space(id)`, 50 a page; `activity.Mine` is what a person changed lately. Sources are registered per stream kind (`tracker.go`); the wiki adds its page stream and `commentSubjects["page"]` in one file, `internal/activity/wiki.go`.

- Views: `activity-home:<pages>` (key `activity:home:<hash of the readable scopes>:<pages>`, shared by everyone who reads the same scopes), `activity-project:<id>`, `activity-space:<id>`; topics `lith:events` (`events.Topic`) and `people`, rendered again every minute for relative times. A project's or a space's page renders `views.Activity(feed)` from `s.ScopeActivity(ctx, q, scope, now)` and names its view in `Page.Views`.
- The home page `/` (FR-59, `routes_notify.go`): the person's work in the view `home:` (key `home:<person>`: counts, open issues assigned to them, "recently edited by you" from the log, the projects and spaces they may read) and the home feed, 50 more with `?activity=n`. Recently viewed needs a write per view and is not there yet.

## The issue list and saved filters

- `GET /issues` is the list (FR-50 to FR-55). Its filter, sort and page are the URL's parameters as `tracker.Filter.Values` and `tracker.Sort.Values` name them, "me" kept. The filter form is a plain GET form in the document (it works without script); the list itself, `#issue-list`, is the shared view `list:<query>`: the query string in base64url, or `.<hash>` of a query that `list_queries` holds when it is longer than a view's name allows. Its key is `list:` and `tracker.Query.Key()` (the projects the person reads, the filter with "me" resolved, the sort, the page), so tabs that see the same list share one render whoever they are (MP-11). It listens to `issues`, `projects` and `people`, so a change that moves an issue into or out of the filter shows in the next frame (MP-03).
- The page's script reads the form (`views/list.go`, no person's value in an expression), puts the query in the address (`pushState` for a filter, a sort or a page, `replaceState` while typing; `popstate` restores the form) and posts `POST /cmd/list/view {tab, view, query}`, at most every 300 ms while typing. The handler stores the query in the tab's row (area `list`), and after the commit, in commit order, moves the open stream from the old list's view to the new one with `hub.Unwatch` and `hub.Watch`; it answers the new view's name as `$_views`, which a reconnect opens. The stream stays open, so only the new list's frame arrives. `GET /issues` without a query redirects (303) to the list the session showed last, so a tab keeps its filter when the person opens an issue and comes back (FR-55.2).
- A click on a row or its summary opens the issue in the side panel (FR-50.3, "Issue panel"): the list's body holds `views.IssuePanel` and each summary link `views.IssueLinkAttrs`; so do the results of `/search`. `GET /issues.csv?<query>` exports the whole result (FR-152). `/search` with a list's filter and no text (`/search?assignee=me`) redirects to `/issues` with it.
- Saved filters (FR-56): `/filters` lists every one, the person's starred first, as the person's own view `filters:` (key `filters:<person>`, topic `filters`). `POST /cmd/filters/save {name, query}`, `/rename {filter, name}`, `/delete {filter}` and `/star {filter, star}` run the tracker's commands; only the owner (or a site admin) renames and deletes. The list's "Filters" menu links each to `/issues?<query>`.

## Search

`internal/search` is the search across issues and pages (FR-57), an FTS5 index that is a projection of the event streams (NFR-44).

- Each kind of document has two tables (`search.Tables(kind)`, `migrations/060_search.sql`): its rows (`search_issues`, `search_pages`, `search_comments`: id, scope, key and old keys, the folded title, href, place, labels and people as JSON, hidden, updated) and their text in an FTS5 table beside them (`<table>_fts`: key, title, body; `unicode61` without diacritics, prefix indexes of 2 and 3). The row's `n` is the text's rowid.
- Only one projector writes a kind's tables, through `search.Put(tx, kind, doc)`, `search.Patch(tx, kind, id, func(*search.Doc))`, `search.Hide(tx, kind, id, hidden)` and `search.Remove(tx, kind, id)` in its inline handlers. A handler builds the document from its event and from the document as indexed (`Patch`), never from another projection, which may be rebuilding at the time. Each write touches `search.Topic` (`search`).
- The tracker's parts hold two of them: `search_issues` on the issue stream (created, summary, description, assignee, labels, moves to another project with the old key kept, deletes) and `search_comments` on the comment stream, which indexes comments on issues and on pages (scope: the subject's id, key: its kind; a hidden comment stays indexed but out of results).
- The wiki's hook: the wiki adds a projector of its page stream to `wiki.Parts()` named `search.PagesProjector` at `search.PagesVersion` that writes `search.Tables(search.Page)`:

```go
var pageSearch = events.NewProjector(PageStream, search.PagesProjector, search.PagesVersion,
	events.Handle(PageCreatedType, func(tx *writer.Tx, r events.Record[string, PageCreated]) error {
		return search.Put(tx, search.Page, search.Doc{ID: r.ID, Scope: r.Data.Space, Title: r.Data.Title, Body: text,
			Labels: labels, People: []string{r.Actor}, Place: spaceKey, Href: pageURL, Updated: r.At})
	}),
	events.Handle(PageRenamedType, func(tx *writer.Tx, r events.Record[string, PageRenamed]) error {
		return search.Patch(tx, search.Page, r.ID, func(d *search.Doc) { d.Title, d.Updated = r.Data.To, r.At })
	}),
	events.Handle(PageDeletedType, func(tx *writer.Tx, r events.Record[string, PageDeleted]) error {
		return search.Remove(tx, search.Page, r.ID)
	}),
	// every other type of the stream: events.Ignore
).Tables(search.Tables(search.Page)...)
```

  `Scope` is the space's id (the scopes row that access reads), `Place` what results show beside the title (the space's key), `Href` the page's address, `Body` the plain text (`tracker.DocText` reads a document's), `People` who a person filter finds it by (author, editors). A page moved to another space patches Scope, Place and Href. Page comments need nothing more: they are comments.
- `search.Search(ctx, q, search.Query{Text, Kinds, Projects, Spaces, InProjects, InSpaces, Labels, People, Offset, Limit})` matches every word, the last as a prefix while someone types, and ranks in tiers: the exact issue key (old keys too) or the exact page title, then key and title matches, then body matches, then comment matches (leading to their subject), each tier by bm25 (key and title weigh more), then the newest. `Projects` and `Spaces` are what the person may read. A hit has its snippet as parts with the matched words marked (`search.Part`), which the page renders escaped in `<mark>`.
- `GET /search?q=&kind=&project=&space=&label=&person=&page=` is the results page (FR-57.2) with the kinds' counts; its results are the shared view `search:<params in base64url>`, keyed by the scopes the person reads and the params, listening to `search`, `issues`, `people` and the swap topics of the index's rebuilds (`search.ProjectionTopics`).
- The header's search box (FR-57.1): `views.SearchBoxAttrs()` on the input and `views.SearchSuggestSlot()` after it in the form (`ui.HeaderProps.SearchAttrs`, `SearchSuggest`). While someone types two characters or more, at most every 300 ms, the box asks `GET /search/suggest?q=`, a read that answers a patch of `#search-suggest` (a popover in the top layer, anchored under the box) with the first 5 issues and 5 pages and a link to every result; Down goes into it, Escape closes it, Enter submits the form to `/search`. A polite live region says how many suggestions there are.
- Rebuilds: a new version of one of the projectors makes lith's runner rebuild its tables into copies, FTS5 table included, and swap them in. Tested in `internal/search` (`TestRebuild`): the results of a fixed set of queries are the same before and after, the copies and the retired tables are gone, and searches that run while it rebuilds in batches of 10 answer from the old index, never empty (NFR-44). lith's copy takes the table's `CREATE VIRTUAL TABLE` statement and FTS5 renames its shadow tables with the table, so no projector of its own is needed.

## The edge

`web.Server.Handler` puts the edge around the routes, inside out: the actor (`people.Middleware`), `edge.Sessions` (`__Host-sid` with `-secure`), `edge.CSRF` with `Datastar: ["POST /cmd/"]` and no exemptions, `edge.SecurityHeaders` with `DefaultCSP` and HSTS with `-secure`, `X-Robots-Tag: noindex, nofollow`, `edge.Compress`, `edge.Recover`, `edge.LogRequests`. `cmd/tickit` mounts `/healthz` and `/readyz` beside it and serves metrics on a loopback listener of their own (`-metrics`, 127.0.0.1:7205). Pages register with `s.get(pattern, handler, probes...)` and actions with `s.post`, which refuses anything outside `POST /cmd/`.

## Migrations

`migrations/NNN_name.sql`, applied at startup. Numbers are taken by area, so parallel work does not collide; gaps are allowed:

| Numbers | Area |
| --- | --- |
| 001 to 009 | people, access, tabs, security log |
| 010 to 029 | tracker (011: who types on an issue) |
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
