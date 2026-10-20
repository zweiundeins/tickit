# Security of the public demo

tickit runs as a public demo at `tickitdemo.zweiundeins.gmbh`: anyone on the internet uses it without an account, as a guest with a Member's rights in every open project and space (requirements 3.1, Q14). This file says whom the demo defends against, which control stops what, which test proves each control, how to run the security suite, and what the review of October 2026 found. Requirement ids are those of `docs/requirements.md`, sections 7 and 8.1 to 8.2.

## Threat model

**Who comes.** Guests, many of them scripts: they send any request to any route, with any body, from any number of sessions (a session costs nothing) and from a few addresses. Spammers and vandals among them write offensive content, and try to undo what moderators remove. A visitor may hold a copied cookie of a staff account (a shared computer, a leaked log). Other sites try cross-site requests from a visitor's browser. The few staff accounts are moderators and site admins; their passwords are set from the environment at seeding (NFR-07.3).

**What is worth taking.**

- The staff's rights: a site admin creates accounts and resets the demo for everyone; a moderator hides content and blocks guests.
- Everyone's work: issues, pages and comments until the nightly reset (DM-03). The reset bounds every vandal's damage to one day; a reset outside the schedule destroys everyone's work of that day.
- The staff's browsers: script that runs in a moderator's page acts with the moderator's rights, since the page posts to `/cmd/` from the site's own origin.
- Availability: one process with one writer, 200 streams at most (`-streams`), the memory of a systemd unit.
- Personal data, of which there is little: staff emails, user agents of login sessions, client addresses in the limiters' memory (NFR-25).

**What the demo accepts.** Guests read and write the open projects and spaces; that is the demo. Everything they write is public until the reset. Closed scopes exist (a project or space without `open`), and the suite tests them, but no route of the demo makes one.

## Controls and their tests

The tests are in `internal/web` unless the table says otherwise. "The suite" is `security_test.go`, `security_read_test.go`, `security_fuzz_test.go` and `security_timing_test.go`, on the world of `security_fixture_test.go`: a closed project SEC and a closed space SECS with an Admin (owner), a Member and a Viewer, an open project OPN and space OPS, a plain guest, an account without a role, a moderator, a site admin, a blocked guest and an account whose first password must change.

| Control | Where | Proved by |
| --- | --- | --- |
| Every action is `POST /cmd/...`, registered with `s.post`; the server lists them (`Server.Actions`) | `server.go` | `TestEveryActionCovered` fails for an action or an issue field without a case |
| The right of every action is checked inside the writer (`access.Require`, owner checks, `requireAdmin`, `access.Moderate`); a scope the actor may not read answers 404 (NFR-01.1) | domain packages | `TestEveryActionRefused`: every action as each person without the right answers 403 (404 for a closed scope's outsider, 409 for a commit there, as for a removed page) and the database is unchanged but for the tab's outcome and the op's stored answer; `TestEveryActionAllowed`: whoever has the right gets through and something changes |
| The actor comes from the session; payload fields are targets at most (NFR-02.2) | `action.go` `decode` | `TestEveryActionAllowed` forges `actor`, `reporter`, `author`, `person`, `by`, `sid`, `session`, `user`, `owner`, `recipient`, `creator`, `account` in every action and finds every new event by the session's person, none naming the forged one, no row in the forged session's tab |
| An action's stored answer is replayed only to a retry of the same action: the once key is session, path and op id | `action.go` `run` | `TestOpIDReplayAcrossActions` |
| Tab state and a tab's own views belong to its session: the tab and preferences views are keyed by tab and a hash of the session, an editor's view by tab and person (NFR-02.3) | `tabs.View`, `tabs.PrefsView`, `routes_editor.go` `editorView` | `TestForgedTabs`, `TestTabViewsBySession`, `TestEditorViewByPerson`: a forged tab id in `/stream`, `/cmd/watch`, `/cmd/unwatch`, an editor's open and presence reads and changes nothing of the tab; `tab:`, `person:` and `prefs:` are no view a URL may name |
| `edge.CSRF` with Datastar's header on `POST /cmd/`, no exemptions (NFR-03, NFR-55.3) | `server.go` `Handler` | `TestEveryActionCrossSite` (every action, five kinds of foreign request, three people), `TestCrossSite`, `deploy/security-smoke.sh` |
| Login sessions are server rows keyed by SHA-256 of the session id; login renews the id; logout, password change, ended sessions and deactivation delete rows; expiry is absolute and checked on every request (NFR-04, FR-124) | `internal/people` | `TestLogin`, `TestAccounts`; `TestLogoutEndsStreams`: an account's open streams end within `LoginCheck` (2 s) once its session no longer resolves to it |
| No secret has a default; staff passwords come from the environment (NFR-05, NFR-07.3) | `cmd/tickit` | review (below) |
| GET never writes (NFR-06) | every GET handler | `TestEveryGetReadOnly`: every probe of every GET route, the pages of the fixture's closed and open scopes, a stream of every registered view kind (196 URLs, 71 of them streams) (a kind without a probe fails the test), as nine kinds of visitor, and an unchanged fingerprint of every table; `TestGetNeverWrites` |
| argon2id, 19 MiB, two passes; 8 to 128 characters, every one counts (NFR-07); at most `MaxHashing` (4) hashes at once | `people/password.go` | `internal/people`: `TestPasswords` (NFR-07), `TestHashesAtOnce` |
| 30 sign-ins a minute per address; 5 failures for one email from one address lock it for 15 minutes; also for the current password of a password change; the lockout keeps at most 50,000 keys (NFR-09) | `routes_core.go`, `routes_admin.go`, `people/lockout.go` | `TestSignInLimits`, `TestPasswordChangeLockout`, `internal/people` `TestLockoutBounded` |
| An unknown email does the work of a wrong password and hears the same (NFR-10) | `people.Verify` | `TestLogin`, `TestLoginTiming` (numbers below) |
| No password, token, session id or cookie in a log line (NFR-11) | every `slog` call, `edge.LogRequests` (path without query) | every test's server: `webtest` records each password sent and each session cookie set, and fails the test whose server log holds one; `TestLogin`; `deploy/security-smoke.sh` greps the binary's log |
| The client address is `edge.ClientKey`: forwarding headers count from loopback alone (NFR-12) | lith's `edge` | lith's `TestClientIPTrustsOnlyTheLocalProxy`; the suite gives each person an address of their own through `X-Forwarded-For` from loopback |
| No person's value in a Datastar expression or a script; every page has `edge.DefaultCSP` with a nonce of its own; the editor's script of the document escapes `<` (NFR-13) | `internal/views`, `routes_editor.go` | `TestEveryGetReadOnly` (each page's own nonce, every inline script with it, every script from `/static/`, no inline handler), `TestEditorScriptHoldsNoMarkup`, `TestHeaders`; review (below) |
| Links accept `http`, `https`, `mailto` with `rel="nofollow ugc noopener"`; images only from the library (DM-06.2, DM-06.3) | `doc.SafeHref`, `doc.RenderHTML`, `doc.Rules` | `internal/doc` tests; CSP `img-src 'self' data:` |
| No debug route, inspector or magic mail in the binary (NFR-14) | `cmd/tickit` | `TestHeaders`, `deploy/security-smoke.sh` on the built binary |
| Malformed input answers 400, an unknown id 404, never 500 (NFR-16) | `decode`, commands' `Validate` | `TestMalformedEveryAction` (about 13,000 requests), seven fuzz targets, `TestCommitBeyondHead`, `TestMalformedActions` |
| Security headers, HSTS with `-secure`, `X-Robots-Tag`, robots.txt (NFR-19, DM-09) | `server.go` `Handler` | `TestHeaders`, `TestEveryGetReadOnly`, `deploy/security-smoke.sh` |
| govulncheck (NFR-20) | | `go tool task vuln`, `go tool task security` |
| Events name their actor by person id and hold no email, address, user agent, token or credential (NFR-26) | event types | review (below) |
| Rate limits per session and address on every action (DM-04, NFR-41); one address holds at most 40 streams and opens them at one a second after a burst of 40; the hub holds 200 | `action.go`, `stream.go` | `TestRateLimits`, `TestCommitAnswers`, `TestStreamsPerAddress` |
| The demo's guard: 503 during a reset, 403 for a blocked guest, the caps of new issues and pages (DM-03.4, DM-04.3, DM-07.2), also for typing and carets, which skip `tabs.Act` | `demo.Guard`, `demo.Check` | `TestBlockedGuest`, `TestBlockedGuestTypes`, `TestSubtaskCap`, `TestResetNow`, the blocked guest of `TestEveryActionRefused` |
| What a moderator hid stays hidden: only staff restore a page or show a comment a moderator hid; the hidden page answers 404 to everyone else (DM-07.1) | `wiki.RestorePage`, `tracker.HideComment`, `routes_wiki.go` `readable` | `TestModeratorHideHolds` |
| An account whose first password must change gets 403 from every action, view, watch, commit and caret until it changes it (FR-141.1) | `mustChange` | `TestMustChangeEverywhere`, the `must` person of `TestEveryActionRefused` |
| No edit from before a reset applies after it: every commit names the epoch, which each reset bumps (MP-13.2) | `wiki.Commit` | `TestCommitAnswers`, `internal/wiki` |

## Running the suite

    go tool task security                 # the suite, 30 s of each fuzzer, the binary's check, govulncheck
    FUZZTIME=3m go tool task security     # the recorded run below

The task builds the static binary first (`go tool task build`, no `-dev`), runs `go test -count=1 ./...` (every test's server log is searched for that test's secrets), runs each fuzz target for `FUZZTIME`, runs `deploy/security-smoke.sh ./tickit` against the binary as production starts it (`-secure`), and ends with govulncheck. One part alone:

    go test -run 'TestEvery|TestForgedTabs|TestMalformedEveryAction' ./internal/web/
    go test -run '^$' -fuzz '^FuzzEditorActions$' -fuzztime 3m ./internal/web/
    go test -run TestLoginTiming -v ./internal/web/

A new action fails `TestEveryActionCovered` until it has a case in `secCases` (`security_test.go`): its pattern, the right it needs, who may take it, and a request built from the fixture. A new view kind fails `TestEveryGetReadOnly` until a page names it or the test adds a probe.

The suite lifts the rate limits (`web.Unlimited`, in `export_security_test.go`), since a 429 would hide the 403 it looks for; the limits have tests of their own.

## Recorded runs

**Fuzzing**, 20 October 2026, on the development machine (20 cores, `-parallel 8`), `go test -run '^$' -fuzz '^<target>$' -fuzztime 3m ./internal/web/`:

| Target | Actions | Time | Runs | Corpus | 500s or panics |
| --- | --- | --- | --- | --- | --- |
| `FuzzCoreActions` | watch, unwatch, login, rename, prefs, list view, filters | 3 min | 700,800 | 388 | none |
| `FuzzIssueActions` | create and every field of `/cmd/issues/{id}/{field}`, typing | 3 min | 712,533 | 407 | none |
| `FuzzBoardActions` | projects, the board's move, the backlog, sprints | 3 min | 1,100,336 | 410 | none |
| `FuzzWikiActions` | spaces and page actions | 3 min | 1,049,913 | 372 | none |
| `FuzzEditorActions` | an editor's open, commit and presence | 3 min | 795,581 | 328 | none |
| `FuzzNotifyActions` | notifications | 3 min | 458,612 | 371 | none |
| `FuzzStaffActions` | moderation, admin, the account's own | 3 min | 879,940 | 363 | none |

Each target starts the fixture once per worker and sends what the fuzzer makes of a case's body to that case's route, as someone who may take it; the seeds are every case's valid body, half of it, and it with single quotes. The corpora stay in the Go build cache, not in the repository.

**Login timing (NFR-10)**, `TestLoginTiming`, 100 interleaved tries each, a new client address per try: a wrong password for an account took 20.6 ms on average (median 18.4 ms), an email without an account 22.4 ms (median 19.3 ms). The difference, under 2 ms, is within the 50 ms of NFR-10.

**govulncheck v1.8.0**, 20 October 2026: no vulnerability in code tickit calls. It reports GO-2026-5932 in `golang.org/x/crypto` v0.57.0, a module tickit requires, in code tickit does not call; there is no fixed version yet.

**Routes of the production build** (`go build` without `-dev`; `-dev` changes only whether static files are minified and whether `serve` seeds an empty database): 58 actions under `POST /cmd/`, the GET routes of `Server.Probes`, `/healthz` and `/readyz` outside the edge, `/metrics` on the loopback listener alone. `GET /design` is the design system's page in production too.

## Findings of the review, October 2026

The review covered the authorization of every action and view, the edge (CSRF, headers, cookies, CSP), output encoding in every view, the editor's commit path, sessions and passwords, logs and events, the demo's guard and moderation, and resource bounds. Each fix has a test that fails without it.

| Severity | Finding | Where | Status |
| --- | --- | --- | --- |
| High | Any guest could start the demo's reset: an action's stored answer was keyed by session and op id only, so `POST /cmd/admin/reset` with the op id of the guest's own earlier rename replayed its OK, and the handler started the reset | `internal/web/action.go:188` (`run`), `internal/web/routes_admin.go:473` | Fixed: the key holds the path; a replayed OK starts no reset. `TestOpIDReplayAcrossActions` |
| High | Stored script in every editor's page: the document went into a `<script type="application/json">` with `<` unescaped, so a page's text `</script><div data-init='...'>` ran as Datastar in the browser of whoever opened the editor, a moderator included | `internal/web/routes_editor.go:88` | Fixed: `<` goes out as `<`. `TestEditorScriptHoldsNoMarkup` |
| Medium | A logout, password change or ended session left the account's open streams running, so a copied cookie kept admin views (account emails, the security log) | `internal/web/stream.go:299` (`endWithLogin`) | Fixed: an account's stream ends within 2 s once its session no longer resolves to it. `TestLogoutEndsStreams` |
| Medium | A guest who created a space or project, and so is its Admin, could restore the page or show the comment a moderator hid; the hidden page stayed readable at its address | `internal/wiki/page.go:803`, `internal/tracker/comment.go:332`, `internal/web/routes_wiki.go:113` | Fixed: only staff undo a staff member's hide; the page answers 404 to everyone else. `TestModeratorHideHolds` |
| Low | A tab's outcomes, preferences and editor views were keyed by tab id alone: a stream naming another session's tab id got that tab's frames, or stole them. It needs the tab's 128-bit id, which Caddy's access log holds (`/stream?tab=`) | `internal/tabs/tabs.go:345`, `internal/tabs/prefs.go:132`, `internal/web/routes_editor.go:315` | Fixed. `TestTabViewsBySession`, `TestEditorViewByPerson`, `TestForgedTabs` |
| Low | An account with a first password to change could still read every view by the stream and `/cmd/watch`, change the list's view, commit to pages and send carets | `internal/web/action.go:152` (`mustChange`), `stream.go:193`, `stream.go:414`, `routes_editor.go:227`, `routes_issues.go:280` | Fixed. `TestMustChangeEverywhere` |
| Low | A blocked guest's typing indicators and carets went on, also during a reset | `internal/web/action.go:162` (`paused`), `routes_tracker.go:674`, `routes_editor.go:286` | Fixed. `TestBlockedGuestTypes` |
| Low | One client could open all 200 streams, and everyone else got 503 | `internal/web/stream.go:264` | Fixed: 40 streams per address, one a second after a burst of 40; loopback without a proxy (probes, load tests) keeps the hub's cap alone. `TestStreamsPerAddress` |
| Low | Sign-ins from many addresses ran argon2id without a bound, 19 MiB each: 32 at once took 608 MiB | `internal/people/password.go:50` | Fixed: 4 at once, the others wait. `TestHashesAtOnce` |
| Low | The lockout grew with every email tried (up to 4 KiB each) and scanned all its keys on every failure past 10,000 | `internal/people/lockout.go:38` | Fixed: emails clipped to 254 bytes, at most 50,000 keys, one scan a minute, locked keys kept. `TestLockoutBounded` |
| Low | A password change had no lockout: a copied session could guess the current password at 30 a minute | `internal/web/routes_admin.go:525` | Fixed. `TestPasswordChangeLockout` |
| Low | Subtasks did not count against the demo's cap of new issues | `internal/web/routes_tracker.go:957` | Fixed. `TestSubtaskCap` |
| Low | A closed project's issue keys showed through `/ticket/{id}` (a 301 to the key) and through the refusals of a link or a parent by key | `internal/web/routes_tracker.go:145`, `:968` (`readableRef`) | Fixed. `TestClosedKeysStayUnknown` |
| Low | Typing on, and linking from, an issue of a closed project answered 204 to an outsider, the typing after a write the command then refused | `internal/web/routes_tracker.go:677`, `:941` | Fixed: the read right first, 404. `TestClosedIssueTypingAndLink` |
| Low (NFR-16) | A commit on a version beyond the page's head answered 500 | `internal/wiki/pagedoc.go:426` | Fixed: 409, the editor starts again. `TestCommitBeyondHead` |

### Accepted, with the reason

- **CSP `script-src 'unsafe-eval'` and `style-src 'unsafe-inline'`.** Datastar compiles its expressions with `Function`, so `'unsafe-eval'` is part of `edge.DefaultCSP`; it makes any attribute a person could inject into a page a script, which is why no person's value goes into an expression (NFR-13.1), checked in every view of `internal/views` and `internal/ui`: signals are built with `json.Marshal`, ids reach handlers in escaped `data-*` attributes, documents render through `doc.RenderHTML`, which escapes every text and attribute. Inline styles carry no script under this CSP.
- **`-secure` is a flag, off by default.** The development server runs over plain HTTP. `deploy/tickit.service` passes `-secure`, and `deploy/smoke-test.sh` and `deploy/security-smoke.sh` fail a binary without HSTS and the `__Host-` cookie under it.
- **`GET /design` in production.** The design system's page renders fixed samples and the session's display preferences; it reads no data and writes nothing (`TestEveryGetReadOnly`).
- **A changed membership or role in a project or space keeps the person's open views until they reconnect** (NFR-01.2 holds for site roles: `dropStaffViews`). Every project and space the demo makes is open, where a guest is a Member whatever the memberships say, and no route closes one; the case exists only in tests.
- **Page actions other than a commit name no version.** A rename or a move sent after a reset acts on the reseeded page with the same id; it is a new action on the page as it is, not an edit from before the reset (MP-13.2 is about the editor's commits, which name the epoch). During the reset every action answers 503.
- **Reads without a rate limit**: `/search/suggest`, `/wiki/pick`, `/issues.csv` and the pages. Each read is bounded (results per page, `tracker.MaxExport` rows for the CSV) and runs on the database's readers, never the writer; the streams have their limits.
- **Guests' session cookies last 400 days** (lith's default `Max-Age`). A guest's identity is a pseudonym derived from it; a login session on top of it expires on the server after 24 hours (`-session-ttl`).
- **Address blocks live in memory** and a restart forgets them (NFR-25.2 keeps addresses out of the database); blocks of a guest's person stay in `blocks`.
- **govulncheck's GO-2026-5932** is in code of `golang.org/x/crypto` that tickit does not call.

### Checked and found sound

- **Output encoding.** templ escapes text and attributes; every `templ.Raw` is `doc.RenderHTML`, the version diff, or the importmap with its nonce; every `templ.SafeURL` has a fixed prefix or a server-built path. Names, summaries, labels, comments, page titles, search snippets (`<mark>` around escaped parts), notification texts and the editor's `chips`, `batches` and `carets` (`json.Marshal`, escaped by templ, drawn as text nodes by the editor) reach the page escaped. The CSV export prefixes `'` before `=`, `+`, `-`, `@`, tab and carriage return.
- **Link schemes and images.** `doc.SafeHref` refuses control characters and spaces, lowercases the scheme, refuses `//host`; the renderer checks again. Images render only as `/img/lib/<id>` of the library; templates, set-document and restore pass `doc.Rules`.
- **The editor's commit path.** The body is capped at 256 KB before it is read, Go's decoder at 10,000 levels of nesting, `doc.ParseCommit` at 256 KB and 500 steps; the right and the epoch are checked first, `doc.Rules.Check` (size, blocks, tables, attributes, schemes, languages, heading levels) after the rebase.
- **Passwords and sessions.** argon2id with a 16-byte salt and a 32-byte key compared in constant time; session ids of 256 bits; the cookie `HttpOnly`, `SameSite=Lax`, `Path=/`, and `__Host-` with `Secure` under `-secure`.
- **Logs.** lith's request log writes the method, the path without its query, the status, the size, the time and the address; no tickit log call writes a password, an email, a cookie or a session id.
- **Events (NFR-26).** No event type of the six streams has a field for an email, an address, a user agent, a token or a session; metadata is the time, the person id and the op id; the editor's client id is 12 random hex digits. The security and moderation logs have no address column.
- **No default secrets (NFR-05).** There is no session secret; `tickit account create` reads the password from `TICKIT_PASSWORD` or standard input, never from a flag; the seed creates no account.
