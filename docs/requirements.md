# tickit: requirements for the rewrite on lith

Draft, 16 October 2026. The open questions of section 11 are decided.

tickit becomes a multiplayer clone of Jira and Confluence, run as a public demo of lith at `tickit.zweiundeins.gmbh` (it replaces the old demo at `tickitdemo.zweiundeins.gmbh`), and is rewritten from scratch on lith. This document states what the rewrite must do, what it should and could do, and what waits. It does not prescribe tickit's present architecture (NATS, command and query buses, five database files); section 2 lists the constraints that come from lith instead.

## About this document

### Sources

- tickit `origin/master` at `4d93827`, read from a detached worktree: README, `docs/`, the code, the seed and the git history (19 commits, 4 July 2025 to 15 October 2026).
- The user's uncommitted work in `/develop/tickit`, on `ca4686c`: the diff to `internal/bootstrap/deps.go`, `internal/realtime/renderer_issue_table.go`, `internal/routes/pages.go`, `internal/routes/routes_updates.go` and `web/ui/pages/dashboard.templ`, and the untracked `internal/issues/filters.go`, `internal/issues/realtime_event_handlers.go`, `CLAUDE.md` and `.kiro/specs/user-management/`.
- lith, branch `v0.3.0` at `3fcebd5`: `skills/lith/SKILL.md` and its references, the guides on events, views, streams, pages, edge, deploying, monitoring and backups, and the demos Town in time, board, cursor party and guestbook in `site/internal/demos`.
- The earlier analysis, `tickit-analysis.md` of 16 October 2026: the mapping onto lith, what works and what is broken, the security findings and 13 decisions.
- A benchmark against what people expect from Jira and Confluence. Where tickit's sources say nothing, the source reads "inferred (Jira)", "inferred (Confluence)", "inferred (demo)" or "inferred (lith)".

### Conventions

- Paths are relative to the tickit repository. `lith:` marks a path in the lith repository, `wip:` the user's uncommitted work in `/develop/tickit`, `kiro:` a file in `/develop/tickit/.kiro/specs/user-management/` (untracked), and "analysis §n" a section of the earlier analysis.
- Priorities: **Must** is what the demo needs to feel like Jira and Confluence used together, live, by several people. **Should**, **Could** and **Won't for now** follow MoSCoW.
- Today's status, in tickit as it is: **works**; **partly works**; **stub** (a form, a route or a type exists and does nothing); **broken** (wired, but wrong or failing); **missing**.
- Requirement IDs: FR functional, MP multiplayer, DM demo operation, NFR non-functional. Numbers leave gaps between areas, so that a review can add requirements without renumbering.
- Each requirement has one line with its priority, its source and today's status, then numbered acceptance criteria, so that a test can cite "FR-10.2".

## 1. Purpose and scope

### 1.1 What tickit is for

tickit is a public demo of an issue tracker in the manner of Jira and a wiki in the manner of Confluence, used together by several people at once. A visitor opens a board, a page or an issue and sees the other people on it, their moves on the board, their edits to the page and their changes to the issue as they happen. The demo shows what lith gives an app: every change reaches every open page within a tick, history comes from the event log, and presence comes from the hub.

tickit is also lith's first app with logins (analysis §2, gap G8), so its auth code becomes the reference for later lith apps.

### 1.2 Who uses it

- **Visitors** of the demo: developers who evaluate lith and Datastar, and prospective clients of zwei und eins gmbh. They try it alone in two browser windows or together with colleagues, mostly for a few minutes, on desktop and phone.
- **Moderators**, staff of zwei und eins gmbh, who keep the public content clean between resets.
- **Site admins**, who run the host, reset the demo and manage accounts.
- **lith's developers**, who use tickit as a test bed for `lith/events`, the hub and load tests.

### 1.3 What tickit is not

- A place for confidential data: everything in the demo is public and is wiped at the next reset (DM-03).
- A product with an API for other clients: the JSON `/api/*` routes go (Q10).
- A helpdesk with a customer portal. The "Internal" comment checkbox and the "Max (Agent)" placeholder (`web/ui/components/issues.templ:188-198`) are not carried over.
- Multi-tenant, mail-driven, or integrated with Git, Slack or other tools.
- An importer or exporter of real Jira or Confluence data.

## 2. Platform constraints from lith

The rewrite inherits these from lith and its template. They constrain the design; the requirements below state behaviour.

1. One static Go binary and one SQLite file. lith requires Go 1.27.2 (analysis §1); tickit pins lith with a `replace` to the `v0.3.0` branch until it is tagged (Q11).
2. Only the writer writes. Every change is a command that the writer applies in arrival order; checks that read before they write run inside the command (`lith:skills/lith/SKILL.md`, rule 1).
3. A page region is a view with a key and topics. A commit touches topics, and in the next tick, at most 50 ms later, the hub renders each changed view once for every stream that watches it (rules 3 and 4; `lith:site/content/docs/guides/views.md:259`).
4. No optimistic updates. The page shows the person's own input at once, marked pending; outcomes arrive with the next frame; a refusal appears next to where the person acted (rule 5). Keystrokes in an editor are the person's own input.
5. State that must survive a deploy lives in SQLite: tab state per tab, preferences per session. No maps of tabs in memory (rule 6).
6. GET never writes (rule 7).
7. Issues, pages and the other event-sourced records are streams in `lith/events`. A decider reads its stream's state row; inline projectors keep read models in the same savepoint; a reaction to an event (a notification, a mail) is a catch-up projector that sends its command with `writer.Once` keyed by the event's position; a projection table guards no rule (`lith:site/content/docs/guides/events.md:162`, `:192`).
8. Streams are bounded: a cap on open streams (200 in the template), a 10 s deadline on each write, the newest frame wins, a heartbeat every 20 s and a page watchdog after 50 s of silence, and a reconnect after a deploy (`lith:site/content/docs/guides/views.md:228-259`, `lith:sse/stream.go:51-56`).
9. The request edge: an opaque session cookie (`__Host-sid`, 400 days), CSRF checks from `Sec-Fetch-Site` or `Origin`, a Content Security Policy with a nonce that allows scripts from the app only, the client address only from the local proxy, and rate limiters (`lith:site/content/docs/guides/edge.md:45-103`).
10. Keys are rowids or time-ordered (rule 10); tickit keeps UUID strings at demo size (Q3).
11. Datastar 1.0.4. A field someone types in does not sit in a region the stream morphs; an editor is an island the stream does not morph (`lith:site/content/docs/guides/pages.md:49`).
12. Deployment from lith's template: the systemd unit, `app-deploy` with a health check and rollback, a Caddy block without `encode`, and drop-ins for a shared host (`lith:site/content/docs/guides/deploying.md`).

## 3. Roles and permissions

### 3.1 Roles

- **Guest**: anyone with a browser session. A guest has a pseudonymous identity made from a hash of the session (a name such as "Sunny Otter" and a face), the way lith's board demo draws its visitors (`lith:site/internal/demos/board/avatar.go:10-16`). Under Q14's proposal, a guest has the rights of a Member in every project and space marked open, and all seeded ones are open. Without it, a guest has the rights of a Viewer.
- **Viewer**, **Member**, **Admin** of a project or a space: roles per project and per space.
- **Moderator**: a logged-in account that may hide or delete any content and block a guest.
- **Site admin**: a logged-in account that manages accounts, roles, projects, spaces and the demo's reset.

tickit today has four site roles (Admin, ProjectManager, Developer, Viewer) and three project roles (ProjectAdmin, Developer, Viewer), and a member's permissions in a project are the union of both (`internal/users/rbac.go:9-22`, `:221-268`). Nothing enforces them on issue routes. Q15 proposes the simpler model above: ProjectAdmin becomes Admin, Developer becomes Member, and ProjectManager's right to create projects becomes a Member right with a rate limit.

### 3.2 Permissions

| Action | Viewer | Member | Admin of the project or space | Moderator | Site admin |
| --- | --- | --- | --- | --- | --- |
| Read issues, boards, pages, comments and history | yes | yes | yes | yes | yes |
| Comment on issues and pages | yes | yes | yes | yes | yes |
| Create and edit issues, move cards, create and edit pages | no | yes | yes | no | yes |
| Delete an issue | no | no | yes | yes (hide or delete) | yes |
| Move a page to the trash | no | pages they created | yes | yes (hide or delete) | yes |
| Restore a page version | no | yes | yes | no | yes |
| Edit or delete a comment | own | own | own, hide others | any | any |
| Configure workflow, board columns, components, templates | no | no | yes | no | yes |
| Manage members and roles of a project or space | no | no | yes | no | yes |
| Create a project or a space | no | yes, rate-limited (Q24) | yes | no | yes |
| Archive a project or a space | no | no | yes | no | yes |
| Block a guest, hide any content | no | no | no | yes | yes |
| Manage accounts and site roles, reset the demo | no | no | no | no | yes |

Moderators and site admins also hold the rights of their roles in each project and space; their columns list what their site role adds. tickit lets a Viewer comment (`internal/users/rbac.go:105-114`) while calling the role "read-only"; the table keeps the comment right.

## 4. Domain model

### 4.1 People and access

**Person.** Everyone who acts. Fields: id, kind (guest or account), display name, avatar seed and hue, created at, last seen at. A guest's id, name and face derive from a hash of the session id, never the session id itself. An account adds email (unique, compared without case), username (unique, compared without case, 2 to 50 characters), full name, password hash, site role (none, moderator, admin), status (Pending, Active, Inactive), email verified, last login at. tickit's `users` table holds the account fields (`internal/users/model.go:6-19`).

**Login session.** Maps lith's session id to an account. Fields: session id, account, created at, expires at, last seen at, user agent. Logout, password change, password reset and deactivation delete rows. tickit writes `user_sessions` and never reads it (analysis §1).

**Tab.** One loaded page. Fields: session, tab id, page kind, state as JSON (filters, sort, page number, pending operation id, last refusal, slider position, editor session), seen at. A sweep deletes tabs not seen for 3 days.

**Membership.** Person, project or space, role (Viewer, Member, Admin), added at, added by. Unique per person and project or space.

### 4.2 Jira side

**Project.** Fields: id, key (2 to 6 letters A to Z, unique among all projects including archived; `internal/projects/service.go:13`), name (1 to 100 characters), description, lead, archived, created at, created by, next issue number. The creator becomes Admin in the same command.

**Issue.** Fields: id, project, number, key (`<project key>-<number>`, `internal/issues/model.go:63-66`), type, summary (1 to 255 characters), description, status, priority, assignee (a person or none), reporter (the person who created it), labels, components, parent, sprint, rank, due date, story points, original estimate, remaining estimate, watchers, created at, updated at, resolved at, version, deleted. Rules:

- Numbers start at 1 per project, grow by one, are never reused, and stay unique under concurrent creation.
- Types: Epic, Story, Task, Bug, Subtask. An epic has no parent. A story, task or bug may have an epic as parent. A subtask has a parent that is not a subtask or an epic, in the same project.
- Priorities, in order of severity: Low, Medium (the default), High, Critical (`internal/issues/model.go:31-45`).
- The version counts the events of the issue's stream; a page sends the version it showed with each change (MP-08).

**Workflow and status.** Each project has one workflow: an ordered list of statuses, each with a category (To Do, In Progress or Done), and the allowed transitions. The default workflow holds tickit's seven statuses (`internal/issues/model.go:9-29`): Backlog, Selected for Development and To Do in the To Do category; In Progress, In Review and Blocked in In Progress; Done in Done. Every transition is allowed. A new issue starts in To Do (`internal/issues/commands.go:122`). Entering the Done category sets resolved at; leaving it clears it.

**Issue link.** Two issues and a link type: blocks and is blocked by, relates to, duplicates and is duplicated by. Shown on both issues with the inverse wording. No link to itself, no duplicate pair.

**Comment.** Fields: id, issue or page, author, body (rich text without headings and tables), created at, edited at, deleted or hidden. A comment's mentions notify their people.

**Label.** A word of 1 to 50 characters without spaces, per project or space, created on first use.

**Component.** Name and description per project, set by its Admins.

**Board.** One per project. Columns, each mapped to one or more statuses of the workflow, in order; an optional work-in-progress limit per column; quick filters.

**Sprint.** Name, goal, start, end, state (future, active, closed), issues. At most one active sprint per project.

**Saved filter.** Name, owner, the filter as structured fields and text, starred by.

**Dashboard.** Name, owner, gadgets in a grid; a gadget is a kind (filter results, counts by status, assigned to me, activity) with its settings.

### 4.3 Confluence side

**Space.** Fields: id, key (2 to 10 letters A to Z, unique), name, description, home page, archived, created at, created by.

**Page.** Fields: id, space, parent page (none for the home page and top-level pages), position among its siblings, title (1 to 255 characters, unique within the space without regard to case), body (a document in the editor's schema), current version number, created at and by, last edited at and by, labels, state (current or trashed), version of its stream.

**Page version.** Page, number from 1, title, body, the people who edited since the version before, created at, an optional message. Versions never change once stored.

**Page comment.** A comment (above) on a page, with one level of replies.

**Inline comment.** A comment anchored to a range of text in a page. The anchor moves with the text as people edit. Fields add the anchor and resolved at; a comment whose text was deleted shows as detached.

**Template.** Name, description, body, scope (one space or all). Seeded templates: meeting notes, decision log, retrospective, product requirements, how-to.

**Reference between a page and an issue.** An issue chip in a page names an issue by key. The references are derived from page bodies and give each issue its list of pages that mention it. An explicit link from an issue to a page is stored as its own record.

### 4.4 Shared records

**Notification.** Fields: id, recipient, kind (assigned, mentioned, commented on a watched issue, status changed on a watched issue, page commented), subject (issue, page or comment), actor, text, created at, read at, dismissed at. One row per recipient.

**Activity entry.** Read from the event log: actor, verb, subject, details (field, old value, new value), time, position. Not stored a second time.

**Image.** Fields: id, kind (library or upload), uploader, media type, width, height, size, SHA-256, moderation state, created at.

**Moderation record.** Target, action (hide, restore, delete, block guest, unblock), moderator, reason, at.

## 5. Functional requirements

### 5.1 Projects

#### FR-01 Project list and keys
Must · Source: `web/ui/components/projects.templ:23-67`; `internal/projects/model.go:10-18` · Today: partly works. The admin panel lists projects; nothing else does.

1. A projects page lists every project the person can read with key, name, lead and the number of open issues, archived projects last and dimmed.
2. Each project has a page of its own with its board, its issue list and its recent activity.
3. A project's key appears in every issue key of the project.

#### FR-02 Create a project
Should · Source: `web/ui/components/projects.templ:5-21`; `internal/projects/service.go:35-52` · Today: partly works. The form posts to an unauthenticated API; keys are not checked for uniqueness; the creator is not recorded (`internal/projects/service.go:31-33`).

1. A Member creates a project with name, key and description; the key is turned to upper case and must match 2 to 6 letters A to Z.
2. A key used by any project, archived ones included, is refused with "The key ABC is taken." next to the field, and nothing is stored.
3. The creator becomes the project's Admin in the same command, and the project gets the default workflow (FR-35) and a board (FR-40).

#### FR-03 Edit a project
Could · Source: `web/ui/components/projects.templ:41-50` · Today: broken. `PatchProject` is not registered (`internal/bootstrap/deps.go:163-165`), so every save fails.

1. A project's Admin changes its name, description and lead; the key stays.
2. Every open page that shows the project shows the new name in the next frame.

#### FR-04 Archive a project
Could · Source: `web/ui/components/projects.templ:53-61`; `internal/projects/service.go:76-91` · Today: broken. The buttons post the literal text `{project.ID}`, and `ArchiveProject` would blank the project's name and key (analysis §1).

1. A project's Admin archives and unarchives it. An archived project's issues are read-only, and it takes no new issues.
2. Archived projects drop out of pickers and default filters, and stay reachable by URL and by issue key.

#### FR-05 Project members and roles
Could · Source: `internal/routes/api.go:53-59`; `kiro:requirements.md` requirement 3 · Today: broken. The membership commands are not registered on the command bus (analysis §2).

1. A project's Admin lists, adds, removes and changes the role of members.
2. The last Admin of a project cannot be removed or demoted.
3. A removed member loses access at once: their open pages of the project stop showing its issues in the next frame.

#### FR-06 Delete a project
Won't for now · Source: `internal/projects/commands.go:117-132` · Today: broken. `DELETE /api/projects/{id}` needs no login and reads the id from the query string.

1. Archiving (FR-04) replaces deletion in the first version.

#### FR-07 Change a project key
Won't for now · Source: `internal/projects/commands.go:40-48` · Today: missing.

1. Keys stay as created, so issue keys and links never change.

### 5.2 Issues

#### FR-10 Create an issue
Must · Source: `web/ui/pages/create_ticket.templ:12`; `internal/issues/commands.go:121-177` · Today: stub. The Create Ticket form has no action; the old index page posts to an unauthenticated API with the browser session as reporter (`internal/issues/routes.go:15-19`).

1. A Create button in the header opens a dialog from every page, with the current project preselected; a "+" at the top of a board column creates an issue in that column's first status.
2. Project, type and summary are required; summary has 1 to 255 characters after trimming. Priority defaults to Medium, status to the workflow's start status, reporter to the person who creates it.
3. The new issue gets the next number of its project (FR-11) and appears on every open board, list and search that includes it in the next frame (MP-03).
4. A refusal (empty summary, archived project, assignee who is not a member) appears next to its field, and the dialog keeps what the person typed.

#### FR-11 Issue numbers and keys
Must · Source: `internal/issues/model.go:51`, `:63-66`; analysis §1 · Today: broken. The counter never advances, so all 100 local issues have number 1 (`internal/issues/projection.go:373-409`).

1. The first issue of a project gets number 1, each next one the number after the highest ever given in that project.
2. Two issues created in the same project at the same moment get different numbers; a uniqueness constraint on project and number holds.
3. A deleted issue's number is not given again.

#### FR-12 Open an issue by key
Must · Source: `internal/routes/pages.go:53-96`; `web/ui/components/issues.templ:83` · Today: broken. The route lowercases the key's project part while keys are stored in upper case (`internal/routes/pages.go:61`).

1. `/<KEY>-<number>`, for example `/WEB-12`, opens the issue; the key is matched without regard to case.
2. An unknown key answers 404 with a search box; a deleted issue's key answers 410 with "WEB-12 was deleted by Sunny Otter on 16 October".
3. Each issue has one canonical URL; tickit's old `/ticket/<id>` and `/dashboard/<id>` redirect to it (Q3).

#### FR-13 View an issue
Must · Source: `web/ui/pages/ticket_detail.templ:10-81`; `web/ui/components/issues.templ:132-206` · Today: partly works. The page renders, but shows the reporter as an id (`web/ui/pages/ticket_detail.templ:53`), and an unknown id panics into a 500 (`internal/routes/pages.go:128-133`).

1. The issue page shows key, type, summary, description, status, priority, assignee, reporter, labels, due date, parent and children, links, watchers, pages that mention it (FR-101), comments and history, with names and avatars instead of ids.
2. From a board, a list or a search, an issue opens in a side panel beside it; the panel's URL is the issue's URL, and closing it returns to the list with its filter and scroll position.
3. The people viewing the issue appear in its header (MP-02).

#### FR-14 Edit summary and description
Must · Source: `internal/issues/commands.go:254-339` · Today: stub. `PatchIssue` takes both fields, but the page offers no way to edit them.

1. A Member clicks the summary or the description to edit it in place; Enter or a click outside saves, Escape cancels.
2. A summary that is empty or longer than 255 characters is refused next to the field; the field keeps the typed text.
3. A save that changes nothing stores nothing (as `internal/issues/commands.go:314-317` does today).

#### FR-15 Rich-text description
Should · Source: inferred (Jira) · Today: missing. The description is plain text.

1. The description uses the page editor (FR-82) with headings, lists, code, links, issue chips and mentions.
2. Two people who edit one description at once both keep their edits (MP-05), once Q22 decides that descriptions are co-edited.

#### FR-16 Change status
Must · Source: `web/ui/components/issues.templ:149-156`; `internal/issues/model.go:9-29` · Today: partly works. The drawer changes the status on every signal change; nobody else sees it until they reload.

1. A Member changes the status from the issue's status menu, from the board (FR-41) or by bulk edit (FR-58); the menu offers the transitions the workflow allows (FR-35).
2. The change appears on every open board, list, panel and page chip that shows the issue in the next frame, and in the issue's history (FR-68).

#### FR-17 Assign and unassign
Must · Source: `web/ui/components/issues.templ:167`; `internal/users/project_integration.go:104-136`; `docs/user-management/INTEGRATION_IMPLEMENTATION.md:78-84` · Today: stub. The page shows a select with one option, "Me", that does nothing; the API checks membership.

1. The assignee picker lists the project's members and the guests who acted in the project in the last 24 hours, with avatars, searchable by name; "Assign to me" and "Unassigned" are one click each.
2. Assigning someone who cannot be assigned is refused with the reason; a site admin can be assigned anywhere.
3. Assigning someone else notifies them (FR-111) and adds them as a watcher (FR-27).

#### FR-18 Change priority
Must · Source: `web/ui/components/issues.templ:157-164`; `internal/issues/model.go:31-45` · Today: partly works. Same as status: it saves, and others do not see it.

1. A Member picks Low, Medium, High or Critical; each shows with an icon and its name, never colour alone.

#### FR-19 Issue types
Must · Source: inferred (Jira); `internal/issues/model.go:47-48` ("Extendable for more fields") · Today: missing.

1. Every issue has one of Epic, Story, Task, Bug and Subtask, shown with its icon on cards, lists, chips and the issue page.
2. A Member changes the type of an issue within the rules of section 4.2; changing an epic with children to another type is refused with "Move the 4 child issues first."

#### FR-20 Epics and their issues
Should · Source: inferred (Jira) · Today: missing.

1. An epic lists its child issues with status and a progress bar of done against all, live.
2. A story, task or bug picks its epic from a picker of the project's epics; the card shows the epic's name.

#### FR-21 Subtasks
Should · Source: inferred (Jira) · Today: missing.

1. An issue's page has an "Add subtask" field that creates a subtask with the next number of the project.
2. Subtasks show on the parent with their status; the board shows them as cards with the parent's key.

#### FR-22 Issue links
Should · Source: inferred (Jira) · Today: missing.

1. A Member links two issues as "blocks", "relates to" or "duplicates", by key or by search; both issues show the link, the other with the inverse wording ("is blocked by").
2. A link to the issue itself or a second identical link is refused; removing a link removes it from both.

#### FR-23 Labels on issues
Should · Source: inferred (Jira); `internal/issues/model.go:47-48` ("e.g. labels") · Today: missing.

1. A Member adds and removes labels with autocomplete from the project's labels; a new word creates a label.
2. A label on a card or list is a link to the issues with that label.

#### FR-24 Components
Could · Source: inferred (Jira) · Today: missing.

1. A project's Admin defines components; a Member puts an issue in any number of them; the list filters by component.

#### FR-25 Due date
Should · Source: inferred (Jira) · Today: missing.

1. A Member sets or clears a due date with a date picker; cards and lists show it.
2. An issue past its due date and not in the Done category shows as overdue, by text and icon.

#### FR-26 Estimates
Could · Source: inferred (Jira) · Today: missing.

1. An issue takes story points (0 to 100) and an original and a remaining estimate in hours and minutes; the board and the backlog show sums per column and per sprint.

#### FR-27 Watchers
Should · Source: inferred (Jira) · Today: missing.

1. A Member watches and unwatches an issue with one button; the issue shows how many watch and who.
2. The reporter, the assignee, people who comment and people mentioned become watchers.
3. Watchers get notified as FR-113 says.

#### FR-28 Delete an issue
Should · Source: `internal/issues/commands.go:237-252`; `internal/users/rbac.go:38` (delete_issue) · Today: broken. The route reads the id from the query string and appends a delete of the id "" (`internal/issues/routes.go:89`).

1. A project's Admin, a moderator or a site admin deletes an issue after confirming; it disappears from every open board, list and search in the next frame.
2. The issue's history and number stay; its key answers as FR-12.2 says; chips on pages show it as deleted.

#### FR-29 Move an issue to another project
Could · Source: `web/ui/components/issues.templ:168-176` · Today: partly works. The drawer changes the project, and the issue keeps its number from the old project.

1. Moving gives the issue the next number in the target project; the old key keeps opening it (FR-12).
2. A move into a project whose workflow lacks the issue's status asks for a status of the target workflow.

#### FR-30 Attachments on issues
Won't for now · Source: `web/ui/pages/create_ticket.templ:33`; `internal/routes/routes_upload.go` · Today: stub. The form has a file input that nothing reads; `/upload` takes files without a login when `UPLOAD_TOKEN` is unset.

1. Issues take no files in the first version. Images in pages follow FR-84 and FR-85.

### 5.3 Workflows and statuses

#### FR-35 Default workflow
Must · Source: `internal/issues/model.go:9-29`; `internal/issues/commands.go:92-119` · Today: partly works. The seven statuses exist and are validated; there are no categories and no transition rules.

1. Every project starts with the default workflow of section 4.2: seven statuses in three categories, every transition allowed.
2. A status not in the project's workflow is refused.
3. Lists sort statuses in workflow order, not alphabetically.

#### FR-36 Configure a workflow
Could · Source: inferred (Jira) · Today: missing.

1. A project's Admin adds, renames, reorders and removes statuses and sets each one's category.
2. Removing a status that issues hold asks for the status to move them to, and moves them in the same command.
3. Board columns follow (FR-43).

#### FR-37 Transition rules
Could · Source: inferred (Jira) · Today: missing.

1. A project's Admin restricts which status may follow which; menus and the board offer only allowed moves, and a forbidden move is refused with "Done can only follow In Review."

#### FR-38 Resolution time
Should · Source: inferred (Jira) · Today: missing.

1. Entering a status of the Done category sets the issue's resolved time; leaving it clears it; lists and dashboards use it for "resolved".

### 5.4 Boards, backlog and sprints

#### FR-40 Kanban board
Must · Source: inferred (Jira) · Today: missing.

1. Every project has a board with one column per status of the To Do, In Progress and Done categories except Backlog, in workflow order; each column shows its issue count.
2. A card shows type icon, key, summary, priority, assignee's avatar, labels, due date and, when set, epic and story points.
3. A click on a card opens the issue in a side panel (FR-13.2).
4. The people on the board appear above it (MP-02); with 200 issues the board renders within the budget of NFR-38.

#### FR-41 Move cards
Must · Source: inferred (Jira); `lith:site/internal/demos/board/how.md:1-17` · Today: missing.

1. A Member drags a card to another column to change its status, or within a column to change its rank; every other open board shows the move in the next frame (MP-04).
2. Each card has a menu with "Move to" and "Move up or down" that does the same with the keyboard (NFR-47).
3. A move the workflow forbids is refused and the card returns to its column with the reason beside it.

#### FR-42 Board quick filters
Should · Source: inferred (Jira); `web/ui/pages/dashboard.templ:38-41` ("User: Me") · Today: missing.

1. Above the board, avatars of assignees, "Only my issues", type, label and a text box filter the cards; the filter lives in the URL and the tab (FR-55).

#### FR-43 Configure board columns
Could · Source: inferred (Jira) · Today: missing.

1. A project's Admin maps one or more statuses to each column, reorders columns and sets a work-in-progress limit; a column over its limit shows it in colour and text.

#### FR-44 Swimlanes
Could · Source: inferred (Jira) · Today: missing.

1. The board groups cards in rows by epic, by assignee or not at all, chosen per tab.

#### FR-45 Backlog
Should · Source: inferred (Jira) · Today: missing.

1. A project's backlog lists the issues not in the Done category and not in an active sprint, in rank order; a Member drags to rank them, live for everyone.
2. Selecting several issues moves them to a sprint or to the top or bottom at once.

#### FR-46 Sprints
Could · Source: inferred (Jira) · Today: missing.

1. A Member creates a sprint with name, goal and dates, adds issues from the backlog, starts it and completes it; completing moves unfinished issues to the backlog or the next sprint, as chosen.
2. With an active sprint, the board shows that sprint's issues only, and its header shows days left and points done.

### 5.5 Search, filters, bulk edit and dashboards

#### FR-50 Issue list
Must · Source: `web/ui/components/issues.templ:27-130`; `internal/realtime/renderer_issue_table.go:53-150` · Today: works. All issues render in one table without paging, with ids where a name is missing.

1. The issue list shows key, type, summary with priority icon, status, assignee, reporter, project, created, updated and due date; columns show names, never ids.
2. It lists only issues of projects the person can read, and shows how many match.
3. A click on a row opens the issue in a side panel (FR-13.2).

#### FR-51 Filter issues
Must · Source: `internal/issues/queries.go:20-48`; `web/ui/pages/dashboard.templ:28-47`; `wip:web/ui/pages/dashboard.templ` · Today: partly works. Project and text work; the user, filter and sort selects are placeholders; the uncommitted status and priority filters use values that no issue has ("open", "urgent").

1. The list filters by project, type, status or status category, priority, assignee (with "me" and "unassigned"), reporter, label, component, sprint, due date range and created range; filters combine with AND, values within one filter with OR.
2. Filter values come from the model (section 4.2), never from free text.
3. Changing a filter shows the first page of the new result (`docs/event-driven-grouping-requirements.md:72-81`).

#### FR-52 Text search in issues
Must · Source: `internal/issues/projection.go:229-233`; `web/ui/pages/dashboard.templ:44` · Today: works. A substring search on title and description, where `%` and `_` act as wildcards.

1. Text matches key, summary, description and comments, without regard to case; typing a key such as "web-12" puts that issue first.
2. Results update while typing, at most one request per 300 ms; `%` and `_` match themselves.

#### FR-53 Sort issues
Must · Source: `web/ui/components/issues.templ:33-75`; `internal/issues/projection.go:242-261` · Today: partly works. A column header cycles ascending, descending and none; the Project column sorts by nothing, and priority and status sort alphabetically.

1. A header click cycles ascending, descending and the default (created, newest first); the header shows the direction to screen readers (`aria-sort`).
2. Priority sorts by severity, status by workflow order, project by name, people by display name; ties sort by key.

#### FR-54 Pages of results
Must · Source: `web/ui/pages/dashboard.templ:53-58`; `docs/event-driven-grouping-requirements.md:16-22`; `wip:internal/issues/filters.go:91-115` · Today: stub. The page buttons do nothing.

1. The list shows 25 issues a page, the page count and the total; the page number is in the URL.
2. When other people's changes move issues across page boundaries, every open page shows its new 25 rows in the next frame, without gaps or duplicates.
3. A page number past the end shows the last page.

#### FR-55 Filter, sort and page in the URL and the tab
Must · Source: `wip:web/ui/pages/dashboard.templ`; `web/ui/js/signal_storage.ts:1-2`; analysis §4, decision 9 · Today: partly works. The uncommitted dashboard writes filters to the URL and reloads the whole page on every keystroke.

1. The URL holds filter, sort and page, so that a copied link shows the same list; back and forward restore earlier filters.
2. A tab keeps its filter when the person opens an issue and returns.
3. Typing in the search box updates the list without a full page load.

#### FR-56 Saved filters
Should · Source: inferred (Jira) · Today: missing.

1. A person saves the current filter with a name, stars it, and finds it in a "Filters" menu; in the demo every saved filter is visible to everyone.
2. A saved filter feeds a board quick filter, a dashboard gadget (FR-60) or an issue list on a page (FR-104).
3. The owner renames and deletes it.

#### FR-57 Search across pages and issues
Must · Source: inferred (Confluence, Jira) · Today: missing.

1. One search box in the header searches issues (key, summary, description, comments) and pages (title, body, page comments) and shows the first 5 of each while typing.
2. Enter opens a results page with both kinds, filters by kind, project, space, label and person, and a snippet with the matched words highlighted.
3. Results rank title and key matches above body matches; an exact issue key or page title comes first.

#### FR-58 Bulk edit
Could · Source: inferred (Jira) · Today: missing.

1. In the issue list a Member selects issues and changes status, assignee, priority, labels or sprint for all of them, or deletes them with the right of FR-28.
2. Each issue changes or is refused on its own; the result names the issues refused and why.

#### FR-59 Home page with your work
Should · Source: inferred (Jira, Confluence) · Today: missing. `/` redirects to the dashboard (`internal/routes/pages.go:103-106`).

1. The home page shows the issues assigned to the person, the pages and issues they viewed or edited recently, and the activity feed (FR-69), all live.
2. A first-time visitor sees the seeded projects and spaces and the demo notice (DM-10).

#### FR-60 Dashboards
Could · Source: inferred (Jira) · Today: missing.

1. A person creates a dashboard and adds gadgets: results of a saved filter, counts by status as a chart, issues assigned to me, activity, created against resolved per day.
2. Gadgets update live when the issues behind them change.

#### FR-61 Query language
Won't for now · Source: inferred (Jira) · Today: missing.

1. The structured filters of FR-51 replace a JQL-like language in the first version.

### 5.6 Comments, mentions, activity and history

#### FR-65 Comment on an issue
Must · Source: `web/ui/components/issues.templ:188-198`; analysis decision 13 · Today: stub. A fixed comment by "Max (Agent)" and a form that posts nowhere.

1. Anyone who can read an issue adds a comment of 1 to 10,000 characters with bold, italic, code, links, lists, mentions and issue chips.
2. The comment appears for everyone viewing the issue in the next frame, newest last, with author, avatar and relative time ("2 minutes ago", in English).
3. The comment box keeps a draft per issue and tab across a reconnect, and shows "Sunny Otter is typing" to others (MP-07).

#### FR-66 Edit and delete own comments
Should · Source: inferred (Jira) · Today: missing.

1. The author edits or deletes their comment; an edited comment shows "edited"; a deleted one leaves "Comment deleted" in the thread.
2. A project's or space's Admin and a moderator hide any comment (DM-07).

#### FR-67 Mentions
Should · Source: inferred (Jira, Confluence) · Today: missing.

1. Typing "@" in a comment or a page opens a picker of people (members, seeded people, guests active in the last 24 hours) filtered as the person types.
2. A mention shows as the person's name with avatar and links to their activity.
3. A new mention notifies the person once (FR-112); editing a text without adding the mention again notifies nobody.

#### FR-68 Issue history
Must · Source: `web/ui/components/issues.templ:200-203`; `lith:site/content/docs/guides/events.md:136-151` · Today: stub. A fixed line, "Ticket created by John".

1. An issue's History tab lists every change since it was created: who, when, which field, the old and the new value.
2. A change appears in the history of every open page of the issue in the next frame.
3. The issue's Activity tab interleaves comments and history in time order.

#### FR-69 Activity feed
Should · Source: inferred (Jira, Confluence); `lith:site/content/docs/guides/events.md:234-240` · Today: missing.

1. The home page (FR-59), each project and each space show a feed of recent changes: issues created, moved and commented, pages created, edited and commented, with actor, subject and time, newest first, 50 at a time.
2. The feed shows only subjects the person can read and updates live.
3. Consecutive edits of one page by one person within 10 minutes show as one entry.

#### FR-70 Issue or page as of an earlier version
Should · Source: `lith:site/internal/demos/town/how.md` ("The slider", "Views keyed by position") · Today: missing.

1. An issue and a page have a history slider; dragging it shows the issue or page as it stood at that version, with who changed what at that point.
2. While the slider is not at the newest version, the view says so and takes no edits; "Now" returns to the live view.
3. Two tabs at the same version share one render (NFR-37).

### 5.7 Spaces and pages

#### FR-75 Spaces and their home pages
Must · Source: inferred (Confluence) · Today: missing.

1. A spaces page lists every space with key, name and description; each space opens on its home page with the page tree beside it (FR-78).
2. The seed holds at least two spaces with content (DM-02).

#### FR-76 Create a space
Should · Source: inferred (Confluence) · Today: missing.

1. A Member creates a space with name, key (2 to 10 letters A to Z, unique) and description; the space gets a home page, and the creator becomes its Admin.
2. A taken key is refused next to the field.

#### FR-77 Space settings and archive
Could · Source: inferred (Confluence) · Today: missing.

1. A space's Admin edits name and description, manages members (as FR-05) and archives the space; an archived space is read-only.

#### FR-78 Page tree
Must · Source: inferred (Confluence) · Today: missing.

1. Beside every page, the space's page tree shows the pages in their order, with the current page highlighted and its ancestors expanded; expanding and collapsing is per tab.
2. "Create child page" on any page creates a page under it.
3. A page created, renamed, moved or trashed by anyone shows in every open tree of the space in the next frame.

#### FR-79 Move and reorder pages
Should · Source: inferred (Confluence) · Today: missing.

1. A Member drags a page in the tree to another place or parent, or uses "Move" with a picker; the page's children move with it.
2. A move under the page itself or one of its descendants is refused.
3. A page title already used in the target space is refused with the conflicting title.

#### FR-80 View a page
Must · Source: inferred (Confluence) · Today: missing.

1. A page shows breadcrumbs, title, labels, author, last editor and time of the last edit, the body, linked issues as live chips (FR-100), and comments.
2. The people viewing or editing the page appear in its header (MP-02).
3. A viewer who is not editing sees other people's edits as they happen, under Q18's proposal.

#### FR-81 Create a page
Must · Source: inferred (Confluence) · Today: missing.

1. A Member creates a page in a space, blank or from a template (FR-92), and lands in the editor with the cursor in the title.
2. A title used by another page of the space, compared without case, is refused next to the title.
3. The new page appears in the tree of everyone in the space in the next frame.

#### FR-82 Rich-text editor
Must · Source: inferred (Confluence) · Today: missing.

1. The editor has headings (three levels), paragraphs, bold, italic, strikethrough, inline code, links, bullet and numbered lists, task lists, quotes, code blocks with a language, horizontal rules, mentions (FR-67), issue chips (FR-100) and page links (FR-103).
2. Markdown shortcuts work as people type them ("# " for a heading, "- " for a list, three backticks for code), and so do the usual keyboard shortcuts (Ctrl or Cmd with B, I, K, Z).
3. Pasting HTML keeps the supported formatting and drops everything else; pasting an issue key or a tickit URL offers to turn it into a chip or a page link.
4. Undo and redo undo the person's own changes, not other people's.

#### FR-83 Tables in pages
Should · Source: inferred (Confluence) · Today: missing.

1. The editor inserts tables, adds and removes rows and columns, and has a header row; cells hold paragraphs, lists, links and chips.

#### FR-84 Images from the library
Should · Source: inferred (Confluence, demo) · Today: missing.

1. The editor inserts images from a library of seeded images (diagrams, screenshots, photos, about 40), with alt text the person can change.
2. Images render at most at the page's width and keep their aspect ratio.

#### FR-85 Image uploads
Could · Source: inferred (Confluence); `docs/user-management/PROFILE_MANAGEMENT.md:51-57` · Today: stub. `/upload` stores files without a login when `UPLOAD_TOKEN` is unset, under the client's file name (analysis §5).

1. A Member uploads a PNG, JPEG or WebP image of up to 5 MB into a page; the server decodes and re-encodes it, strips metadata, and stores it under a generated name.
2. Uploads follow the limits and moderation of DM-04 and DM-07; a moderator removes an image from every page that shows it.

#### FR-86 Edit a page together
Must · Source: inferred (Confluence); `lith:skills/lith/SKILL.md` rule 5 · Today: missing.

1. Any Member opens the editor on a page; any number of people edit the same page at once, without locks, and see each other's edits as MP-05 says.
2. The title is edited the same way; a title change that would collide with another page is refused for the person who typed it, and their text stays in the field.
3. Each editor's avatar shows "editing" in the page header (MP-07).

#### FR-87 Page versions
Must · Source: inferred (Confluence); analysis decision 13 · Today: missing.

1. A page keeps versions: under Q18's proposal, a new version is cut when nobody has edited for 60 seconds, when the last editor leaves, and when someone clicks "Save version" with an optional message.
2. The page's history lists every version with number, editors, time and message.
3. Any version opens read-only, with its number and "This is an old version" and a link to the current one.

#### FR-88 Compare versions
Should · Source: inferred (Confluence) · Today: missing.

1. Two versions of a page compare side by side or inline, with added text marked as added and deleted text struck out, by text and colour.

#### FR-89 Restore a version
Should · Source: inferred (Confluence) · Today: missing.

1. A Member restores an old version; the restore becomes a new version with the message "Restored version 3", and people in the editor see the content change as with any edit.

#### FR-90 Page comments
Should · Source: inferred (Confluence) · Today: missing.

1. Below the page, people comment and reply one level deep, as FR-65 and FR-66 say for issues.

#### FR-91 Inline comments
Could · Source: inferred (Confluence) · Today: missing.

1. A person selects text and comments on it; the text shows highlighted, and the comment opens beside it.
2. The highlight moves with its text as people edit; a comment whose text was deleted lists as "detached" at the end.
3. Resolving hides the highlight and keeps the comment in a resolved list.

#### FR-92 Page templates
Could · Source: inferred (Confluence) · Today: missing.

1. Creating a page offers the five seeded templates of section 4.3 and the space's own; a space's Admin creates a template from any page.

#### FR-93 Labels on pages
Could · Source: inferred (Confluence) · Today: missing.

1. A Member labels a page as FR-23 says for issues; a label lists the pages and issues that carry it.

#### FR-94 Page trash
Should · Source: inferred (Confluence) · Today: missing.

1. A page's creator, the space's Admin and a moderator move a page to the space's trash; its children move up to its parent.
2. The trash lists trashed pages with who trashed them and when; the space's Admin restores one to its old place or deletes it for good.

#### FR-95 Watch a page
Could · Source: inferred (Confluence) · Today: missing.

1. A person watches a page and is notified of comments on it (FR-114); the page's creator watches it from the start.

### 5.8 Links between pages and issues

#### FR-100 Issue chips on pages
Must · Source: inferred (Confluence) · Today: missing.

1. In the editor, typing an issue key or picking an issue from a search inserts a chip that shows type icon, key, summary and status.
2. A chip shows the issue as it is now: when someone changes its status or summary, every open page with the chip shows the change in the next frame.
3. A chip of a deleted or unknown issue says so, and a click on a chip opens the issue.

#### FR-101 Pages that mention an issue
Must · Source: inferred (Confluence, Jira) · Today: missing.

1. An issue lists the pages whose current version holds a chip of it, with space and title, live.
2. Removing the last chip from a page removes the page from the list in the next frame.

#### FR-102 Link a page from an issue
Should · Source: inferred (Jira) · Today: missing.

1. On an issue, "Link page" picks a page by search and adds it to the issue's pages, beside those of FR-101; the page shows the issue under "Linked issues".

#### FR-103 Links between pages
Should · Source: inferred (Confluence) · Today: missing.

1. The link dialog in the editor searches pages by title; a page link keeps working when the target is renamed or moved.
2. A page lists the pages that link to it ("Linked from").

#### FR-104 Issue list on a page
Could · Source: inferred (Confluence) · Today: missing.

1. A page holds a table of the issues matching a saved filter (FR-56) or a filter typed in place, live, with key, summary, status and assignee.

#### FR-105 Create an issue from a page
Could · Source: inferred (Confluence) · Today: missing.

1. Selecting text on a page offers "Create issue": the text becomes the summary, the new issue's chip replaces the selection, and the issue links back to the page.

### 5.9 Notifications

#### FR-110 Notification panel
Must · Source: `web/ui/components/notification.templ:11-44`; `web/ui/layouts/base.templ:80-84` · Today: works. The panel lists notifications with an unread count and puts the count into the window title; dates show in German (`web/ui/components/notification.templ:15`).

1. A bell in the header shows the unread count; the panel lists notifications newest first with actor, text, subject and relative time in English, and a click opens the subject.
2. The window title shows the unread count, "tickit (3)".
3. A new notification reaches every open tab of its recipient in the next frame.

#### FR-111 Notify on assignment
Must · Source: analysis decision 6; `docs/user-management/INTEGRATION_IMPLEMENTATION.md:34-44` · Today: missing.

1. When someone assigns an issue to another person, that person gets "Sunny Otter assigned WEB-12 to you"; assigning oneself notifies nobody.

#### FR-112 Notify on mention
Should · Source: inferred (Jira, Confluence) · Today: missing.

1. A new mention in a comment or a page notifies the mentioned person once, with the sentence around the mention.

#### FR-113 Notify watchers
Should · Source: inferred (Jira) · Today: missing.

1. A comment on, or a status change of, a watched issue notifies its watchers except the person who acted.
2. Several changes to one issue by one person within a minute become one notification.

#### FR-114 Notify on page comments
Could · Source: inferred (Confluence) · Today: missing.

1. A comment on a page notifies the page's watchers except the commenter.

#### FR-115 Mark read and dismiss
Must · Source: `web/ui/components/notification.templ:16-20`, `:34-35`; `internal/notifications/commands.go:93-207` · Today: partly works. Mark read and delete work for a person's own notifications; general notifications can never be marked read (analysis §2).

1. A person marks one or all notifications read and dismisses one or all; every open tab of theirs shows it in the next frame.
2. Opening a notification's subject marks it read.

#### FR-116 No notifications for views and restarts
Must · Source: `internal/routes/pages.go:130-159`, `:178-198`; `cmd/web/main.go:159-161`; analysis decision 6 · Today: broken. Every view of an issue sends the viewer an "Issue Viewed" notification, and every server start sends everyone "Server Started".

1. Viewing a page or an issue creates no notification and writes nothing.
2. A restart or a deploy creates no notification.

#### FR-117 Notification retention
Should · Source: inferred (lith); `lith:site/content/docs/guides/events.md:305-313` · Today: missing.

1. Dismissed notifications are deleted; read notifications older than 30 days are deleted by a daily sweep; a person keeps at most the newest 500.

#### FR-118 Email notifications
Won't for now · Source: inferred (Jira); `internal/users/email_service.go:57-60` · Today: missing. No mail goes out in production.

1. The first version sends no mail (Q16).

### 5.10 Identity, accounts and sign-in

#### FR-120 Guest identity
Must · Source: inferred (demo); `lith:site/internal/demos/board/avatar.go:10-40`; Q14 · Today: missing. Every page but login requires an account (`internal/routes/pages.go:99-101`).

1. A visitor without an account gets a guest identity from their session: a name of an adjective and an animal and a face in a colour, the same in every tab of that browser and on every page.
2. The guest's name and face appear wherever they act or are present; the session id never appears on a page or in an event.
3. A private window or another browser is another guest, so one person can try the multiplayer features alone.

#### FR-121 Change one's display name
Could · Source: inferred (demo) · Today: missing.

1. A guest or an account picks a display name of 2 to 30 characters and a colour; names are checked against DM-06's word list, and two people may share a name.

#### FR-122 Log in
Must · Source: `kiro:requirements.md` requirement 2; `internal/users/command_handlers.go:352-475`; `web/ui/pages/login.templ:11-29` · Today: works. Password login works for verified, active accounts.

1. An account logs in with email and password; on success lith's session id is renewed (`Renew`) and the person lands on the page they asked for, or the home page.
2. An unknown email and a wrong password get the same message, "Email or password is wrong.", after the same work (NFR-10).
3. A pending or deactivated account learns its state only after a correct password.

#### FR-123 Log out
Must · Source: `kiro:requirements.md` requirement 2.7; `internal/users/routes.go:395-415` · Today: partly works. `GET /logout` expires the cookie only; the logout command is a TODO (`internal/users/command_handlers.go:786-794`).

1. Logout is a POST; it deletes the login session's row and clears the cookie (`Clear`); the browser continues as a new guest.
2. The account's other sessions stay logged in.

#### FR-124 Login sessions expire
Must · Source: `internal/session/session_config.go:30-41`; `kiro:requirements.md` requirement 2.6 · Today: partly works. The cookie lives 24 hours; a copied cookie stays valid for 30 days, and nothing on the server can end it (analysis §5).

1. A login session ends 24 hours after login (a setting); the next request after that is a guest's.
2. Password change, password reset and deactivation end the account's sessions at once (NFR-04).

#### FR-125 Change password
Should · Source: `kiro:requirements.md` requirement 4.3; `internal/users/command_handlers.go:1004-1092` · Today: partly works. The route works; no page offers it.

1. An account changes its password on a settings page by giving the current one and the new one twice; the policy of NFR-07 applies.
2. Every other session of the account ends; the current one stays.

#### FR-126 Register an account
Could · Source: `kiro:requirements.md` requirement 1; `web/ui/pages/login.templ:48-62` · Today: partly works. Registration works through the JSON API only; the form has no action, and without mail nobody can verify (`internal/users/email_service.go:57-60`).

1. A visitor registers with email, username and password; the account starts Pending with no site role, and the guest's content stays with the guest.
2. Registration answers the same whether the email is taken or not; the mail says which (NFR-10).

#### FR-127 Verify an email address
Could · Source: `kiro:requirements.md` requirements 1.6 and 1.7; `internal/users/command_handlers.go:276-350` · Today: partly works. A known token verifies; `GET /auth/verify-email` writes.

1. The mailed link opens a page with a button that posts the token; the token is single-use, valid 24 hours and stored hashed (NFR-08).
2. An expired token offers to send a new mail, at most 3 an hour per account.

#### FR-128 Reset a forgotten password
Could · Source: `kiro:requirements.md` requirement 6; `web/ui/pages/forgot_password.templ` · Today: partly works. The `/api/auth/*` routes work; the pages post without a CSRF token; in production the mail fails, which tells whether the email exists.

1. "Forgot password" takes an email and always answers "If an account uses this email, we sent a link."
2. The link is valid 1 hour and once; a reset ends every session of the account.

#### FR-129 See and end one's sessions
Could · Source: `docs/technical/USER_ROUTES_IMPLEMENTATION.md:113-118` · Today: broken. The routes take the user and the session from the request (`internal/users/routes.go:1937`, `:1965-1972`).

1. An account lists its login sessions with browser, created and last seen, and ends any of them; it can never see or end another account's.

#### FR-130 Edit a profile
Could · Source: `docs/user-management/PROFILE_MANAGEMENT.md`; `kiro:requirements.md` requirement 4 · Today: broken. The commands are not registered (analysis §2).

1. An account changes full name and username; a new email needs verification first (FR-127).

#### FR-131 Avatar upload
Won't for now · Source: `docs/user-management/PROFILE_MANAGEMENT.md:41-57`; `internal/users/avatar_service.go` · Today: broken. Not registered.

1. Faces come from the identity's seed (FR-120).

#### FR-132 Project invitations
Won't for now · Source: `kiro:requirements.md` requirement 5; `internal/users/command_handlers.go:1545-1855` · Today: broken. Not registered; acceptance is not bound to the invitee (analysis §5).

1. Guests and seeded accounts make invitations unnecessary in the demo.

#### FR-133 Magic mail login
Won't for now · Source: `kiro:requirements.md` requirement 11 · Today: broken. Not registered, and the mailed URL is not a route (`internal/users/email_service.go:144`).

1. No passwordless login in the first version.

#### FR-134 WebAuthn and multi-factor login
Won't for now · Source: `kiro:requirements.md` requirement 8; `internal/users/webauthn_service.go` · Today: broken. Not registered; the relying party is fixed to localhost (`internal/users/webauthn_service.go:25-27`).

1. No second factor in the first version (Q1).

#### FR-135 OAuth login
Won't for now · Source: `kiro:requirements.md` requirements 7 and 9; `docs/user-management/oauth_implementation_summary.md` · Today: stub. The callback returns part of the code as JSON (`internal/users/routes.go:604-621`).

1. No GitHub, Google or Microsoft login in the first version (Q1).

#### FR-136 Single sign-on
Won't for now · Source: `kiro:requirements.md` requirement 10; `docs/user-management/SSO_IMPLEMENTATION.md` · Today: stub. `internal/users/sso_*.go` exists and is not wired.

1. No OIDC, SAML or Entra ID in the first version (Q1).

#### FR-137 Header and navigation
Must · Source: `web/ui/layouts/base.templ:58-74` · Today: partly works. Every visitor sees "Admin Panel"; the logout link shows the account's id, "Logout (<uuid>)".

1. The header holds tickit's name, links to Projects, Spaces, Filters and Dashboards, the search box (FR-57), Create (FR-10), the bell (FR-110) and the person's face and name with a menu.
2. Moderation and administration links show only to those who may use them.

### 5.11 Administration

#### FR-140 Account list
Should · Source: `web/ui/pages/admin_panel.templ:9-29`; `docs/user-management/ADMIN_INTERFACE.md:11-51` · Today: works. The admin page lists accounts with name, email, role and status.

1. A site admin lists accounts with name, email, site role, status and last login, 50 a page, searchable by name or email, filterable by role and status.
2. A second tab lists the guests active in the last 24 hours with name, first and last activity, and number of changes.

#### FR-141 Create an account
Could · Source: `web/ui/components/users.templ:12-36` · Today: broken. The form posts to `/api/users`, which is not a route.

1. A site admin creates an account with email, name and site role, and sets a first password that must be changed at the first login.

#### FR-142 Assign site roles
Should · Source: `web/ui/components/users.templ:58-73`; `docs/user-management/ADMIN_INTERFACE.md:107-109` · Today: broken. Role assignment records the actor "admin" (`internal/users/routes.go:1806`) and the command is not registered.

1. A site admin makes an account a moderator, an admin or neither; the change applies to the account's open pages in the next frame.
2. The last site admin cannot remove their own role.

#### FR-143 Deactivate and reactivate accounts
Should · Source: `kiro:requirements.md` requirements 12.3 and 12.4; `web/ui/components/users.templ:75-84` · Today: broken. The buttons post to a route that does not exist.

1. A site admin deactivates an account: its sessions end, it cannot log in, and its content stays with its name marked "deactivated".
2. Reactivating restores login.

#### FR-144 Admin statistics
Could · Source: `docs/user-management/ADMIN_INTERFACE.md:111-113`, `:146-153` · Today: broken. `GET /api/admin/stats` calls a handler that is not registered.

1. A site admin sees counts of accounts, guests active today, issues, pages, open streams and the writer's and hub's numbers.

#### FR-145 Export accounts
Won't for now · Source: `docs/user-management/ADMIN_INTERFACE.md:92-100`; `kiro:requirements.md` requirement 12.6 · Today: stub. The query parameter `export` exists in the docs only.

1. No CSV or JSON export of accounts in the first version.

### 5.12 Import, export and API

#### FR-150 Import tickit's old data
Won't for now · Source: analysis §3, decision 4 · Today: missing.

1. The rewrite starts from the seed (DM-02); nothing of today's databases moves.

#### FR-151 JSON API
Won't for now · Source: `internal/routes/api.go:26-111`; analysis decision 10 · Today: broken. The issue, project and notification routes need no login (analysis §5).

1. The first version has no JSON API; every action is a form or Datastar action of the pages.

#### FR-152 Export issues and pages
Could · Source: inferred (Jira, Confluence) · Today: missing.

1. The issue list exports its current result as CSV; a page exports as Markdown.

### 5.13 Audit

#### FR-155 Every change has an author and a time
Must · Source: analysis §1; `lith:site/content/docs/guides/events.md:117` · Today: broken. Every stored event has the aggregate "unknown" or "system", and issue events carry no author.

1. Every change to an issue, a page, a project, a space or a comment is stored with the acting person, the time in milliseconds and the action's operation id.
2. Rebuilding every read model from the stored events yields the same tables (`lith:site/content/docs/guides/events.md:181-196`); a test checks it.

#### FR-156 Security log
Should · Source: `docs/user-management/SECURITY_IMPLEMENTATION.md:64-76`; analysis decision 2 · Today: stub. `internal/users/audit.go` writes log lines; no user event is stored.

1. Logins, failed logins, logouts, password changes, role changes, deactivations and moderation actions are stored with account, time and outcome, never with a password, token or session id.
2. A site admin reads them, filtered by account and kind; entries older than 90 days are deleted.

## 6. Multiplayer

### 6.1 What lith offers and what it lacks

All numbers in this section are server-side unless they say otherwise. A tick is 50 ms (`lith:site/content/docs/guides/views.md:259`); "within two ticks" means that the stream write of the frame starts at most 100 ms after the commit, at the 95th percentile, at the capacity of NFR-38.

| Need | lith offers | The platform lacks |
| --- | --- | --- |
| Presence on an issue, a board, a page | A view with `Presence: true` renders again when a stream starts or stops watching it; `hub.Watchers(key)` lists the tab ids watching (`lith:live/hub.go:152-154`, `:383-395`); the board demo's row of faces (`lith:site/internal/demos/board/how.md:93-103`) | Idle detection ("away") needs the page to report input; that is app code |
| Live updates everywhere | Topics, views rendered once per tick for every watcher, frames equal to the last one not sent, fewer frames for slow links (`lith:site/content/docs/guides/views.md`, `streams.md:143`) | An activity table fed by several stream kinds needs G1 (analysis §2), or reads `lith_events` directly |
| Live board moves | The board demo: a version per card, the later move refused in its tab, view transitions; 100 tabs at one move a second cost 8 to 13 ms of CPU per move (`lith:site/internal/demos/board/how.md:1-17`, `:141`) | Nothing |
| Conflicts on issue fields | `events.Exactly(v)` refuses an append to a stream at another version; refusals travel in the tab's row (`lith:site/content/docs/guides/events.md:107-115`; Town's `Place`) | A check per field is app code in the decider |
| History and "as of a version" | The event log with positions, `events.StateAt`, `LoadUntil`, history tables keyed by position, Town in time's slider with views keyed by position (`lith:site/content/docs/guides/events.md:242-289`) | Diffs between two page versions are app code |
| Carets and typing indicators | The cursor party: pointer moves as `w.Send` commands into tab rows, numbered so that a late move changes nothing, one shared layer view (`lith:site/internal/demos/cursor-party/how.md`) | Mapping a caret through other people's edits belongs to the editor engine |
| Co-editing rich text | The writer orders every command, so it can act as the sequencer of edit operations; `events.Exactly` gives "apply only on top of version v" | A way to deliver operations without gaps, since a view's frames replace each other and a slow stream skips frames; code that applies and checks the editor's operations on the server; see Appendix A |
| Reconnect and catch-up | `HeartbeatSignal`, the page watchdog, `Spec.Reconnect` after a deploy, current frames on every new stream (`lith:site/content/docs/guides/views.md:228-252`, `pages.md:135-153`) | An editor's unacknowledged local edits are resent by app code |

### 6.2 Multiplayer requirements

#### MP-01 One identity across tabs
Must · Source: `lith:site/internal/demos/board/queries.go:51`; inferred (demo) · Today: missing.

1. A person with three tabs on one board shows as one face in its presence row, with a tab count only in the tooltip.
2. An action in any tab carries the same actor; notifications reach all tabs of the person.

#### MP-02 Presence
Must · Source: inferred (Jira, Confluence); `docs/user-management/UI_EVENTS_IMPLEMENTATION.md:23`, `:113-120` (online status) · Today: missing.

1. An issue, a board, a page and a space's page tree show the faces and names of the people who have it open, at most 8 faces and "+n" for the rest; the person's own face comes first.
2. A face appears in every other tab within two ticks of the newcomer's stream opening.
3. A face goes within 1 s when its last tab closes, and within 60 s when its connection drops without closing.
4. A person who has given no input for 5 minutes shows as away, by a dimmed face and the word in the tooltip.
5. Presence shows only pseudonyms and account names, never session ids or addresses.

#### MP-03 Live updates everywhere
Must · Source: `docs/event-driven-grouping-requirements.md:5-28`, `:472-567`; analysis §4 · Today: broken. On master only notifications reach the browser; issues and projects change for others only on reload.

1. A committed change reaches, within two ticks, every open view that shows it: issue lists and search results (when the change moves an issue into or out of the filter, too), issue pages and panels, boards and backlogs, epics' child lists, page trees, pages, chips on pages, the lists of pages on issues, comments, notifications, activity feeds and dashboards.
2. A view whose content the change does not alter sends no frame (`docs/event-driven-grouping-requirements.md:543-545`).
3. A frame never moves keyboard focus, never resets a field someone is typing in, and keeps the scroll position.

#### MP-04 Live board moves
Must · Source: inferred (Jira); `lith:site/internal/demos/board/how.md:1-90` · Today: missing.

1. A card one person drops in another column shows there on every other open board within two ticks, with a short animated move where the browser supports view transitions.
2. The person who dropped it sees the card in its new place at once, marked pending until the frame confirms the move.
3. Of two people who move the same card at once, the first wins; the second sees the card in the first one's place and "Sunny Otter moved WEB-12 to In Review first" beside it, and nothing of the second move is stored.
4. Card order within a column is the same on every board after every move.

#### MP-05 Co-editing a page without losing edits
Must · Source: inferred (Confluence); `lith:skills/lith/SKILL.md` rule 5 · Today: missing.

1. Up to 20 people edit one page at once, in the same paragraph and the same sentence, and no keystroke of anyone is lost: after they stop, every editor and every viewer has the same document, which holds every person's insertions and the effect of every deletion.
2. A person's own keystrokes show at once, without waiting for the server.
3. Another person's edit shows within 250 ms at the 95th percentile, on a link with a 50 ms round trip, with 20 editors each typing 5 characters a second.
4. After the last keystroke, all editors hold the same document within 1 s.
5. Every edit the server has confirmed survives a crash or a deploy; the editor shows "Saving" while it holds unconfirmed edits and "Saved" when none are left.
6. A person who loses the connection keeps typing; after a reconnect within 60 s their edits merge into the page; after a longer gap they merge, or, if they cannot, they stay as a copy the person can see beside the page, never silently dropped.
7. Undo removes the person's own last change, never someone else's.
8. No part of a page's document is ever a Datastar signal: no request carries the document, and no signal patch or morph from the server changes the editor's content or moves its caret (Appendix A.2, rules 1 to 3).
9. A slow or reconnecting tab misses no confirmed edit: whatever frames its stream skips, the next one it gets brings every edit since the version the tab last confirmed.

#### MP-06 Other people's carets and selections
Should · Source: inferred (Confluence); `lith:site/internal/demos/cursor-party/how.md` · Today: missing.

1. In the editor, every other editor's caret shows as a thin line in their colour with their name, and their selection as a light band in that colour.
2. Carets move at most every 100 ms and stay at the right place in the text while others edit around them.
3. A caret that has not moved for 30 s hides its name label; it goes when its person leaves the editor.

#### MP-07 Editing and typing indicators
Should · Source: inferred (Jira, Confluence) · Today: missing.

1. While someone edits an issue's summary or description, other viewers of the issue see "Sunny Otter is editing the description" at that field, within two ticks.
2. While someone types a comment, other viewers see "Sunny Otter is typing" below the comments.
3. A page's presence row marks editors apart from readers.
4. An indicator goes 5 s after the last keystroke, or at once when the field closes or the tab goes.

#### MP-08 Conflicts on issue fields
Must · Source: analysis decision 7; `lith:site/internal/demos/town/how.md` ("The decider") · Today: missing. Patches overwrite whatever the projection holds; two people changing one field see the last write win, without a word.

1. Each change carries the version of the issue the page showed.
2. A change to a field that nobody changed since that version applies, even if other fields changed meanwhile.
3. A change to a field that someone else changed since that version is refused, and nothing is stored. The second editor sees, at the field: "Sunny Otter changed the priority to High 5 seconds ago. Your Critical was not saved." with a button "Use Critical anyway" that sends the change again against the new version.
4. The second editor's input stays in the field until they act on the message.

#### MP-09 Pending state and refusals
Must · Source: `lith:skills/lith/SKILL.md` rule 5; `lith:skills/lith/references/recipes.md` ("A command"); analysis §4 · Today: missing. The issue drawer saves on every signal change, 100 ms after the last, with no pending state (`web/ui/components/issues.templ:137`).

1. Every control that sends a change shows it pending (read-only, `aria-busy`) from the click until the frame that carries its operation id arrives.
2. A refusal shows next to the control, in words that say why, and stays until the next change there.
3. A request that reaches no outcome (network gone, rate limit, server stopping) shows "This did not go through" with a retry, and the person's input stays.
4. A retried or double-clicked action applies once (`writer.Once`).

#### MP-10 Reconnect and catch-up
Must · Source: `web/ui/layouts/base.templ:49-57`; `lith:site/content/docs/guides/pages.md:135-153`; `lith:site/content/docs/guides/views.md:228-250` · Today: works. The page polls `/api/updates/status` every 15 s and shows "Not connected to server."

1. When the stream drops, the page shows "Reconnecting" within 50 s at the latest (the watchdog), at once when a request fails.
2. The page reopens its stream by itself, after a deploy too, and then shows the current state of every view without a reload.
3. An editor with unconfirmed edits sends them after the reconnect (MP-05.6).

#### MP-11 One render per shared view
Must · Source: `docs/event-driven-grouping-requirements.md:5-35`, `:97-100`, `:119-132`; analysis §1 · Today: broken. Each event renders twice per tab, never once per group (`internal/routes/routes_updates.go:302-433`).

1. Tabs that show the same list (same filter, sort and page) watch one view, keyed by the normalised filter; the hub renders it once per tick in which it changed, however many tabs watch it.
2. A view exists while a tab watches it and goes with the last one; nothing about it stays in memory after.
3. A view whose render fails keeps its last frame, logs the error, and holds up no other view.
4. The hub's statistics show renders per view key, so that the cost of each list is visible (NFR-76).

#### MP-12 Follow another person
Won't for now · Source: inferred (Confluence) · Today: missing.

1. No "follow" mode that moves one person's view with another's in the first version.

#### MP-13 Edits caught by a reset or a removed page
Must · Source: review; DM-03, DM-07, FR-94 · Today: missing.

1. When the demo resets, or the page someone edits is trashed, hidden or deleted, their editor stops sending, keeps the text it had not had confirmed, and says so: "This page was reset. Your last changes were not saved." with a button that copies them.
2. No edit made before a reset is applied after it, even to a seeded page with the same id and address: every edit names the page's stream version, and a reset starts the streams anew.
3. MP-05.6's merge after a reconnect never applies to a page that was reset or removed meanwhile.

#### MP-14 Refused while editing
Should · Source: review; DM-04, DM-07, MP-09 · Today: missing.

1. An editor whose edits are refused, by a rate limit or because the guest is blocked, keeps the person's text marked unsaved and says why at the editor, not in a passing message.
2. After a rate limit, it sends the held edits by itself; a blocked guest's text stays in their browser only.

## 7. Demo operation

#### DM-01 Visitors write without an account
Must · Source: Q14; `lith:site/internal/demos/board/how.md:93-103`; `lith:site/internal/demos/town/how.md` ("The decider": the actor is a hash of the session) · Today: missing. Every page but login requires an account, and the login page offers no way in to a visitor.

1. A first-time visitor lands on the home page with the demo notice (DM-10) and can create an issue, move a card and edit a page without signing up.
2. A guest has the rights of a Member in every open project and space (section 3.1); moderation and administration need an account.
3. The login page publishes no shared passwords.

#### DM-02 Seeded content
Must · Source: `cmd/dev/seed.go:80-142`; `internal/migrations/seed.go:25-206`; analysis decision 4 · Today: partly works. The seed makes 5 projects with random names and 20 issues each of placeholder text, all in To Do and all number 1, and 4 accounts with published passwords (`docs/technical/migrations.md:94-108`).

1. The seed describes one fictional company in coherent English: 3 projects with 40 to 60 issues each, across every type, status and priority, with epics and their children, subtasks, links, labels, due dates (some overdue), comments, and a history of changes spread over the 30 days before the seed runs.
2. It holds 2 spaces with 15 to 25 pages each, in a tree three levels deep, with tables, code blocks, library images, issue chips, page links, page comments, and three or more versions on at least five pages.
3. Six seeded people with names and faces act in the history; they are not accounts anyone can log in to.
4. The seed is the same at every run, with dates relative to the run, and finishes in under 30 s.

#### DM-03 Periodic reset
Must · Source: inferred (demo); `lith:site/internal/demos/board/how.md:130-139` · Today: missing.

1. The demo resets every night at 04:00 Europe/Zurich (Q21), and when a site admin clicks "Reset now".
2. Ten minutes before, every open page shows "The demo resets in 10 minutes. Your changes will be deleted."
3. A reset deletes every visitor's issues, pages, comments, images, notifications, tab state, events and guest rows, keeps staff accounts and the moderation log, and runs the seed.
4. During a reset, actions answer "The demo is resetting" and pages show it; within 10 s after it, every open page shows the seeded state without a reload.

#### DM-04 Rate limits on every change
Must · Source: `lith:site/internal/demo/kit.go:55-95`; `docs/user-management/SECURITY_IMPLEMENTATION.md:17-38` · Today: stub. `internal/users/security.go` has rate limiters that nothing uses.

1. Every action is limited per session and per client address; an address gets four times a session's allowance, since people share addresses. Starting values:

   | Action | A session may | Shares the site's limit per address |
   | --- | --- | --- |
   | Create an issue or a page | 1 every 5 s, 5 at once | yes |
   | Comment | 1 every 3 s, 3 at once | yes |
   | Change an issue field, move a card | 2 a second, 20 at once | yes |
   | Send a batch of page edits | 10 a second, 50 at once | no |
   | Move a caret or report typing | 15 a second, 30 at once | no |
   | Create a project or a space | 2 a day | yes |
   | Upload an image (FR-85) | 1 every 30 s, 3 at once | yes |

2. An action over a limit answers 429 and shows "Too many changes at once. Wait a moment." as MP-09.3 says.
3. Across all guests, at most 1,000 new issues, 300 new pages and 200 new images a day; past that, creating answers "The demo is full until the next reset".

#### DM-05 Content limits
Must · Source: inferred (demo); `lith:site/content/docs/guides/events.md:74-75` (title of 1 to 200 characters) · Today: missing. Titles and descriptions have no length limit.

1. Limits: summary and page title 255 characters, issue description 20,000, comment 10,000, page body 200 KB as stored and 2,000 blocks, table 50 rows by 20 columns, label 50, display name 30, 10 mentions per comment.
2. Text over a limit is refused next to its field with the limit in words; the editor stops accepting input at the page limit and says why.

#### DM-06 Safe rendering of public content
Must · Source: inferred (demo); `lith:site/internal/demos/guestbook/commands.go:43-45` · Today: missing.

1. Rich text is stored as a document of the editor's schema and rendered by the server; HTML, scripts, styles, frames and event handlers that people type or paste never reach another browser.
2. Links accept only `http`, `https` and `mailto`; they show their domain on hover and carry `rel="nofollow ugc noopener"`.
3. Images come from the library or from uploads (FR-85); a page never loads an image from another site.
4. Display names, labels and space and project names are checked against a word list kept in the repository.

#### DM-07 Moderation
Should · Source: inferred (demo) · Today: missing.

1. A moderator hides, restores or deletes any issue, comment, page, page version or image; hidden content leaves every open page within two ticks and shows to moderators as hidden.
2. A moderator blocks a guest (by the hash of their session) and their client address for 24 hours; a blocked guest still reads but every change answers "Changes from this browser are paused."
3. A moderation page lists the newest issues, pages, comments and images of visitors, newest first, with one-click hide.
4. Every moderation action is stored with moderator, target, reason and time (FR-156).

#### DM-08 Report content
Could · Source: inferred (demo) · Today: missing.

1. Any visitor reports an issue, a comment, a page or an image with a reason; moderators see open reports first on the moderation page.

#### DM-09 Search engines stay out of visitor content
Must · Source: inferred (demo) · Today: missing.

1. Every page answers with `X-Robots-Tag: noindex, nofollow`, and `robots.txt` disallows everything but the home page.

#### DM-10 Demo notice and privacy page
Must · Source: inferred (demo); `web/ui/layouts/base.templ:85` ("Maintained by zwei und eins gmbh") · Today: partly works. The footer names the maintainer; nothing tells visitors that the content is public.

1. Every page shows a notice: "Public demo. Everything you write is visible to everyone and deleted at the nightly reset (next in 5 h). Do not enter personal data.", with a link to the privacy page.
2. The privacy page says what tickit stores (NFR-25 to NFR-31), for how long, that zwei und eins gmbh runs it, and whom to contact.

#### DM-11 First-visit hint
Could · Source: inferred (demo) · Today: missing.

1. A first visit shows a dismissible hint: "Open this page in a private window to see a second person arrive."

## 8. Non-functional requirements

### 8.1 Security

#### NFR-01 Access control on every request and every view
Must · Source: analysis §5; `internal/routes/api.go:26-50` · Today: broken. The issue, project and notification APIs need no login; nothing checks project roles on issue routes.

1. Every action checks the person's right for the project or space it touches (section 3.2); a script that sends every action without the right gets 403 for each and changes nothing.
2. A view renders only what its watcher may read; a change of role or membership applies to that person's open views within two ticks.

#### NFR-02 One route per action, the actor from the session
Must · Source: `internal/routes/routes_updates.go:20-89`, `:109-113`; `internal/issues/routes.go:15-19` · Today: broken. `POST /command` runs any registered command with parameters from the client; `POST /signals` trusts a client-sent connection id; the reporter is the browser session.

1. Each action has its own route under `POST /cmd/`, with its own validation and authorisation; no route runs a command named by the client.
2. Actor, reporter, author and recipient come from the session, never from the request.
3. Tab state is keyed by session and tab, so one session can neither read nor change another's.

#### NFR-03 Cross-site requests
Must · Source: `internal/users/csrf.go:105-124`; `lith:site/content/docs/guides/edge.md:56-63` · Today: partly works. nosurf exempts every `/api/*` path; the forgot and reset password pages post without a token.

1. `edge.CSRF` checks every request but GET, HEAD and OPTIONS, without exemptions; a cross-site POST answers 403.

#### NFR-04 Login sessions on the server
Must · Source: `internal/sharedhttp/session.go:142-153`; `internal/session/session_config.go:105-114`; analysis §5 · Today: broken. A signed cookie alone says who is logged in, and nothing can revoke it.

1. The cookie holds only lith's opaque session id; the login session is a row on the server (section 4.1).
2. Login renews the id; logout, password change, password reset and deactivation delete the rows; a cookie copied before logout is a guest's afterwards.

#### NFR-05 No default secrets
Must · Source: `config/config.go:66` · Today: broken. The session secret defaults to "session-secret".

1. No secret has a default value; the binary refuses to start in production without the settings it needs, and names the missing one.

#### NFR-06 GET never writes
Must · Source: `lith:skills/lith/SKILL.md` rule 7; `internal/routes/pages.go:145-149`; `internal/routes/users.go:41` · Today: broken. Viewing an issue writes a notification; `GET /logout` and `GET /auth/verify-email` change state.

1. A test sends every GET route of the app and finds the database unchanged.

#### NFR-07 Passwords
Must · Source: `internal/users/validation.go:34-66`; `internal/users/password_hasher.go:15-26`; analysis §5 · Today: partly works. bcrypt at cost 12 cuts passwords at 72 bytes while 128 characters are allowed; the composition rule demands upper case, lower case and a digit.

1. A password has 8 to 128 characters, and every character counts: two passwords that differ only after the 72nd byte are different passwords.
2. Passwords are stored with argon2id, or bcrypt at cost 12 over a SHA-256 of the password; no composition rule (Q23).
3. Seeded staff passwords come from the environment at seeding and appear in no file of the repository.

#### NFR-08 Tokens
Should · Source: `internal/users/store.go:1326`, `:1439`; `internal/users/command_handlers.go:958-963` · Today: broken. Tokens are stored in plain text and "single use" is checked and written in separate transactions.

1. Every token the app mails or links has at least 128 random bits, is stored as a SHA-256 hash, expires (verification 24 hours, password reset 1 hour), and is used once, checked and spent in one command.

#### NFR-09 Sign-in rate limits
Must · Source: `docs/user-management/SECURITY_IMPLEMENTATION.md:17-38` · Today: stub. Limiters and lockout exist in `internal/users/security.go` and run nowhere.

1. Login, registration and password reset take 30 requests a minute per client address, 10 at once.
2. After 5 failed logins for one account from one address within 15 minutes, that address waits 15 minutes before it may try that account again.

#### NFR-10 No account enumeration
Must · Source: `internal/users/command_handlers.go:367-385`; `internal/users/routes.go:155-160` · Today: broken. Login skips the hash for unknown emails and reveals "email not verified" before checking the password; registration answers 409 for a taken email.

1. Login, registration and password reset answer the same way, in the same time within 50 ms, whether or not the email has an account.

#### NFR-11 No secrets in logs
Must · Source: `internal/users/routes.go:272`; `internal/routes/routes_updates.go:64-67` · Today: broken. Login logs the password in clear; `/command` logs its parameters.

1. No log line holds a password, a token, a session id, a cookie or a link with a token; a test greps the logs of a full test run for the seeded secrets.

#### NFR-12 Client address
Must · Source: `internal/users/security.go:258-283`; `lith:site/content/docs/guides/edge.md:94-103` · Today: broken. The address comes from `X-Forwarded-For` as the client sent it.

1. The app takes the client's address only from `edge.ClientIP`, which trusts forwarding headers from the local proxy alone.

#### NFR-13 Output encoding and Content Security Policy
Must · Source: `web/ui/helpers/helpers.go:62`, `:82`, `:137`; `lith:site/content/docs/guides/edge.md:67-92` · Today: partly works. templ escapes HTML; helpers put values into Datastar expressions inside single quotes; no CSP is set.

1. No value from a person goes into a Datastar expression or a script; values reach the page as escaped HTML or as signals.
2. Every page has the CSP of `edge.DefaultCSP()` with a nonce per request; the editor and every other script are served by the app.

#### NFR-14 No debug or development code in production
Must · Source: `internal/routes/users.go:43-45`; `web/ui/layouts/base.templ:86-87`; `internal/routes/api.go:131-133` · Today: broken. `/auth/test-session` dumps session values, `/auth/clear-cookies` is a GET, the Datastar inspector ships, and magic mail is routed.

1. The production binary has no debug route, no live-reload route, no inspector and no magic mail.

#### NFR-15 Image uploads
Should · Source: `internal/routes/routes_upload.go:13`, `:39-40`; `deploy/tickitdemo.caddy` (`/uploads/*`) · Today: broken. Uploads need no login when `UPLOAD_TOKEN` is unset, keep the client's file name, overwrite, and are served by Caddy.

1. Without FR-85 the app has no upload route.
2. With FR-85: uploads need a session that may write, follow DM-04, are decoded and re-encoded (5 MB and 4,096 by 4,096 pixels at most), get generated names, and are served by the app with the media type it determined and `X-Content-Type-Options: nosniff`.

#### NFR-16 Unknown ids and malformed input
Must · Source: `internal/routes/pages.go:128-133`; `internal/routes/routes_updates.go:315` · Today: broken. An unknown issue id panics; a notification event without data panics the stream.

1. An unknown id answers 404 and malformed input 400; a fuzz test of every action finds no 500.

#### NFR-17 Atomic changes
Must · Source: analysis §1; `internal/bootstrap/deps.go:57-82`; `internal/issues/commands.go:164-176`, `:211-224` · Today: broken. A command writes the event store, the projection and the bus in three transactions across five database files; `UpdateIssue` and `UpdateProject` replace the whole record and blank the fields they do not name.

1. A change and all its effects (events, state rows, read models, guard tables, tab state) commit in one transaction of the writer, or none of them do.
2. A change to some fields of an issue, a page, a project or a space leaves every other field as it was.

#### NFR-18 Uniqueness
Must · Source: `internal/users/command_handlers.go:187`, `:220`; `internal/users/store.go:28`; analysis §5 · Today: broken. Username uniqueness is checked in one transaction and inserted in another, without a constraint.

1. Constraints keep unique: account email and username (without regard to case), project key, space key, issue number per project, page title per space (without regard to case), membership per person and project or space. Rules across streams live in tables no projector writes (`lith:site/content/docs/guides/events.md:192`).

#### NFR-19 Security headers
Must · Source: `deploy/tickitdemo.caddy`; `lith:site/content/docs/guides/edge.md:67-70` · Today: partly works. Caddy sets HSTS, `nosniff`, `X-Frame-Options` and `Referrer-Policy`; there is no CSP.

1. `edge.SecurityHeaders` sets CSP, HSTS, `nosniff`, `Referrer-Policy`, `Cross-Origin-Opener-Policy` and `frame-ancestors 'none'` on every response.

#### NFR-20 Dependency checks
Should · Source: inferred (lith) · Today: missing.

1. CI runs `govulncheck` and fails on a finding in reachable code.

### 8.2 Privacy

#### NFR-25 Little personal data
Must · Source: `docs/user-management/ADMIN_INTERFACE.md:140-144`; analysis §3 · Today: broken. `user_sessions` keeps IP addresses and user agents of every login.

1. A guest gives no personal data. An account holds email, username and full name.
2. Login sessions keep user agent and times, no address; addresses live only in the limiters' memory and in access logs.

#### NFR-26 Events without personal data
Must · Source: analysis §2 (`internal/users/events.go:7-10`, `:27-34`, `:62-66`); `lith:site/content/docs/guides/events.md:313` · Today: broken. User events defined today carry tokens, emails, addresses and credential bytes; none is stored yet.

1. An event names its actor by person id only; no email, address, user agent, token or credential goes into an event.

#### NFR-27 Erasing a person
Should · Source: `lith:site/content/docs/guides/events.md:309`; inferred · Today: missing.

1. A site admin erases an account: its personal fields are blanked, its sessions and tokens deleted, and its changes show as by "Deleted person".
2. The nightly reset erases every guest and their content.

#### NFR-28 Log retention
Must · Source: `deploy/tickitdemo.caddy` (`roll_keep_for 720h`) · Today: partly works. Caddy keeps access logs 30 days; the app logs request parameters.

1. Access logs and the app's logs are kept 30 days; the app's logs hold no text of issues, pages or comments.

#### NFR-29 No third parties
Must · Source: `lith:site/content/docs/guides/edge.md:71-76` · Today: works. All scripts and styles come from the app.

1. Pages load nothing from another origin: no fonts, scripts, images, analytics or embeds.

#### NFR-30 Pseudonyms in presence and activity
Must · Source: `lith:site/internal/demos/board/avatar.go:10-16` · Today: missing.

1. Presence, activity and history show guests by their pseudonym and face, and accounts by display name; never a session id, an address or an email.

#### NFR-31 One cookie and a privacy page
Must · Source: inferred (demo) · Today: partly works. tickit sets a session cookie and a `connections` cookie (`internal/realtime/middleware.go:259-271`) and says nothing about them.

1. tickit sets one cookie, lith's session cookie, needed for the demo to work; it shows no cookie banner.
2. The privacy page of DM-10 lists the cookie, the data, its retention and the reset; Q25 asks for a legal check of the wording.

### 8.3 Performance and realtime

#### NFR-35 From commit to screen
Must · Source: `lith:site/content/docs/guides/views.md:259`; `docs/event-driven-grouping-requirements.md:136-146` (BatchWindow); analysis §1 · Today: broken. Issue changes reach no other browser on master.

1. At the load of NFR-38, the write of a changed view's frame starts within 50 ms of the commit at the median and within 100 ms at the 95th percentile, measured with the hub's statistics and the tick overrun log.
2. Ticks overrun on fewer than 1 in 1,000 ticks at that load.

#### NFR-36 Action answer time
Must · Source: `lith:skills/lith/references/recipes.md` ("A command": `Do` costs about half a millisecond) · Today: missing. Nothing is measured.

1. An action answers 204 within 50 ms of its arrival at the 95th percentile at the load of NFR-38.

#### NFR-37 Frame sizes and slow links
Should · Source: `lith:site/internal/demos/board/how.md:141`; `lith:site/content/docs/guides/streams.md:143` · Today: missing.

1. A frame of 25 issue rows is at most 40 KB of HTML, a board of 100 cards at most 60 KB, before compression.
2. A slow link gets at most four frames a second per view (`Spec.Slow`), and a slow tab holds up no other.

#### NFR-38 Capacity
Must · Source: `lith:site/content/docs/guides/deploying.md:24`, `:49`; inferred (demo) · Today: missing.

1. The demo serves 200 open tabs (the stream cap), of which 100 watch one board and 20 co-edit one page, while one card moves a second, 20 editors type 5 characters a second each and 10 issue fields change a second, meeting NFR-35 and NFR-36.
2. It does so with 10,000 issues, 2,000 pages and 1,000,000 events, under the unit's memory limit.
3. A load test in the repository (`lith/loadtest` or a browser test) measures it, and the numbers go into `benchmarks/RESULTS.md` with date and machine.

#### NFR-39 Page load
Must · Source: inferred (lith) · Today: missing. Nothing is measured.

1. Home, board, issue, issue list and page documents answer within 200 ms at the 95th percentile at the data of NFR-38.2; each stream's first frames go out within one tick of its opening.

#### NFR-40 Search speed
Should · Source: inferred · Today: missing.

1. A search over 10,000 issues and 2,000 pages answers within 200 ms at the 95th percentile.

#### NFR-41 Bounded resources
Must · Source: `lith:skills/lith/SKILL.md` rule 9; `lith:site/content/docs/guides/deploying.md:49` · Today: missing. Subscription contexts live in a global map and leak when a stream never opens (analysis §5).

1. The 201st stream answers 503, which the page retries; the process stays under the unit's memory limit at the load of NFR-38; tab rows go after 3 days; the nightly reset bounds the event log.

#### NFR-42 Reconnect after a deploy
Must · Source: `lith:site/content/docs/guides/pages.md:135-153` · Today: missing. A deploy cuts streams, and pages wait for the 15 s status poll.

1. Within 10 s after the new process answers `/healthz`, every page that was open shows the current state without a reload.

### 8.4 Accessibility

#### NFR-43 Growth of the event log from co-editing
Must · Source: review; NFR-38; gap G6 (analysis §2); `lith:site/content/docs/guides/events.md` ("Retention", "Closing the books") · Today: missing.

1. Edit operations are kept until the page version after them is cut (Q18), then folded into that version, so that a page's stream holds its versions and only the operations since the last one. Rebuilds and "as of a version" (FR-70) still work after the fold.
2. Starting value: after 24 hours at NFR-38's load without a reset, the database holds less than 500 MB.
3. The fold runs as commands of its own, at most a batch of operations per command, so that it never holds up the writer for more than 5 ms.

#### NFR-44 Search stays right through rebuilds
Must · Source: review; Q11; `lith:site/content/docs/guides/events.md` ("Rebuilds") · Today: missing.

1. The search index over issues and pages (FR-57) is a projection, and a rebuild of its projector, an FTS5 table included, gives the same results for a fixed set of queries before and after.
2. While it rebuilds, search answers from the old index; it is never empty.

#### NFR-45 WCAG 2.2 AA
Should · Source: inferred · Today: partly works. Pico CSS gives labelled form fields; several controls are clickable divs and links.

1. The home page, a board, an issue, an issue list, a page in view and in edit, search and the notification panel pass WCAG 2.2 AA, checked with axe and by hand with a keyboard and a screen reader.

#### NFR-46 Zoom
Must · Source: `web/ui/layouts/base.templ:25` · Today: broken. The viewport sets `maximum-scale=1, user-scalable=0`.

1. People can zoom to 400 % and every page still works at a width of 320 CSS pixels.

#### NFR-47 Keyboard
Must · Source: `web/ui/components/issues.templ:49-73`; `web/ui/components/notification.templ:29` · Today: partly works. Sort headers are links with click handlers; the notification bar is a clickable div.

1. Every action works by keyboard alone, board moves included (WCAG 2.5.7), through FR-41.2's menu.
2. The page tree follows the ARIA tree pattern; side panels and dialogs keep focus inside, close with Escape, and return focus to where it came from.
3. Sort headers are buttons with `aria-sort`; the bell is a button with `aria-expanded`.

#### NFR-48 Live updates and assistive technology
Must · Source: `lith:site/content/docs/guides/pages.md:93-100`; inferred · Today: missing.

1. A frame never moves focus (MP-03.3).
2. The unread count and refusals reach screen readers through polite live regions; other people's edits, carets and presence changes are not announced one by one, and the presence row tells on request who is there.

#### NFR-49 Accessible editor
Should · Source: inferred (Confluence) · Today: missing.

1. Headings, lists, tables and links in the editor are real elements of their kind; toolbar buttons have names and show their state (`aria-pressed`); other people's carets are hidden from screen readers.

#### NFR-50 Semantics, colour and contrast
Must · Source: `web/ui/components/issues.templ:30`, `:93-96` · Today: partly works. The issue table has `role="grid"` without grid behaviour; priority shows as a coloured dot with a tooltip.

1. Tables are plain tables; type, priority and status never rely on colour alone; text and controls meet AA contrast.

#### NFR-51 Language and dates
Must · Source: `web/ui/components/notification.templ:15`; `web/ui/layouts/base.templ:22` · Today: broken. The interface is English and relative dates are German.

1. Pages declare `lang="en"`; relative times are English; hovering shows the absolute date and time with its time zone.

### 8.5 Browsers

#### NFR-55 Supported browsers
Must · Source: `lith:site/content/docs/guides/streams.md:23`; `lith:site/content/docs/guides/edge.md:58-63` · Today: partly works. tickit runs in current browsers; nothing is tested.

1. The current and the previous major version of Chrome, Edge, Firefox and Safari on desktop, Safari on iOS and Chrome on Android pass the browser tests of section 12.
2. Safari before 26.3 gets brotli instead of zstd and works the same.
3. A browser that sends neither `Sec-Fetch-Site` nor `Origin` can read but not change anything.

#### NFR-56 Small screens and touch
Should · Source: inferred · Today: partly works. Pico CSS lays pages out at phone width; the slide-in panel covers the screen.

1. Every page works at 320 CSS pixels; the board scrolls sideways with columns at least 260 pixels wide; cards move by touch drag and by FR-41.2's menu; the editor takes typing, lists and links on touch.

### 8.6 Deployment on the shared host

#### NFR-60 Build
Must · Source: `go.mod:3-5`; analysis §1 · Today: missing. tickit builds with Go 1.24 and pulls in NATS, toolbelt, quic-go and hugo; `*_templ.go` is gitignored, so a fresh checkout does not build.

1. One static binary, built with `CGO_ENABLED=0` and Go 1.27.2, with templates, static files and migrations embedded, and the generated templ files committed.
2. lith comes through a `replace` to the `v0.3.0` branch until it is tagged (Q11); NATS, toolbelt, quic-go and hugo are gone from `go.mod`.
3. `tickit --version` and `/healthz` print the version from `git describe`.

#### NFR-61 systemd unit
Must · Source: `deploy/systemd/tickit.service.d/listen.conf`; `lith:site/content/docs/guides/deploying.md` · Today: partly works. A hand-made unit runs the binary; the drop-in keeps it on loopback.

1. The unit `tickit` comes from lith's template: an unprivileged user, the database in `StateDirectory`, a Unix socket or `127.0.0.1:7105`, no network but localhost, a memory limit, and a stop timeout that covers draining the writer.

#### NFR-62 Caddy block
Must · Source: `deploy/tickitdemo.caddy`; analysis §5 · Today: broken. The block compresses responses, the SSE streams included, and serves `/static` and `/uploads` from disk.

1. `/etc/caddy/sites/tickit.caddy` only proxies to the app, without `encode` and without file handlers, replaces `X-Forwarded-For` as the template's block does, and keeps its JSON access log at mode 0640 for 30 days.

#### NFR-63 Drop-ins for monitoring
Must · Source: `deploy/setup-host.sh`; `docs/INSTALL.md:247-261`; `lith:site/content/docs/guides/monitoring.md:168-178` · Today: partly works. Probes and the Alloy file exist; the login probe looks for "TickIt" with a module of its own; tickit has no metrics target.

1. `deploy/` holds `tickit.targets.yml` (probes of `/` with `http_2xx` and `/healthz` with `http_2xx_ok`), `tickit.metrics.yml` for `/etc/prometheus/apps/` with the labels `app` and `slot`, and `tickit.alloy` for the journal and the access log.
2. `deploy/setup-host.sh` installs them, reloads Alloy and puts the old file back if Alloy refuses the new one, and never restarts `tickit`.

#### NFR-64 Deploy and rollback
Must · Source: `deploy.sh`; `lith:site/content/docs/guides/deploying.md:93-115` · Today: partly works. `deploy.sh` copies three binaries with rsync and restarts by hand.

1. `app-deploy` installs a new binary, waits up to 30 s for `/healthz` to answer `ok <version>`, and puts the old binary back if it does not; `app-deploy rollback` returns to the last good one.

#### NFR-65 Migrations
Must · Source: `CLAUDE.md` ("Database & migrations (NOT automatic)"); analysis §1 (`cmd/migrate/main.go:74`) · Today: broken. Migrations run by hand, cover only the users tables, and `cmd/migrate` sets `user_version` before its runner looks.

1. Embedded migrations run when the binary opens the database; an older binary refuses a newer schema; a copy is taken before every deploy that migrates (NFR-71).

#### NFR-66 Shutdown
Must · Source: `lith:skills/lith/SKILL.md` rule 2 · Today: missing.

1. On stop, the HTTP server drains first, the writer still applies the actions in flight, and every stream ends with `sse.Reconnect`.

#### NFR-67 Configuration
Must · Source: `docs/INSTALL.md:143-160`; `config/config.go:45-68` · Today: partly works. A `.env` in the working directory sets port, host and session secret.

1. Settings come from flags or from `/etc/tickit/tickit.env` (mode 0600); the repository documents every setting with its default.

### 8.7 Backups

#### NFR-70 Daily copies
Should · Source: `docs/INSTALL.md:277-300`; `lith:site/content/docs/guides/backups.md:9-42` · Today: broken. The documented script copies WAL-mode database files with `cp`, which is not a consistent copy.

1. The template's backup timer writes a `VACUUM INTO` copy every day, keeps 30, and checks each with `PRAGMA integrity_check`.
2. A restore from a copy is tried once before the cut-over and its steps are written down.

#### NFR-71 A copy before a migrating deploy
Must · Source: `lith:site/content/docs/guides/deploying.md:115` · Today: missing.

1. A deploy whose binary migrates the schema takes a copy first, so that a rollback past the migration has a database to go with the old binary.

#### NFR-72 Recovery by reseed
Must · Source: DM-02; analysis decision 4 · Today: missing.

1. With the database lost, a restore of the latest copy brings back staff accounts and the moderation log, and the seed brings back the demo content.

### 8.8 Observability

#### NFR-75 Health and readiness
Must · Source: `lith:site/content/docs/guides/monitoring.md:156-165` · Today: missing. The probe reads the login page.

1. `/healthz` answers `ok <version>` while the process serves; `/readyz` answers 200 while the read pool, the writer, the hub and the event runner run, and 503 naming the failing check.

#### NFR-76 Metrics
Must · Source: `docs/INSTALL.md:256` ("tickit serves no metrics"); `docs/event-driven-grouping-requirements.md:119-132`; `lith:skills/lith/SKILL.md` (`prom`) · Today: missing.

1. A metrics listener on loopback exports lith's collector (writer, hub, stream budget, event store and runner) and tickit's own counters: guests active, open editors per page (maximum), edit batches a second, refusals by kind, rate-limited requests, moderation actions, resets and their duration.
2. The grouping document's metrics map onto these: groups are watched views, update frequency is renders by view key, query time is render time, connection rates are streams opened and closed, cache hits are frames kept.

#### NFR-77 Logs
Must · Source: `lith:site/content/docs/guides/monitoring.md:147-155` · Today: partly works. zerolog writes to the journal, with debug lines of every signal and the password at login.

1. The app logs with slog to the journal, at Info and above in production, one line per request from `edge.LogRequests`, and the lines lith names (render failures, overruns, parked projectors); NFR-11 holds.

#### NFR-78 Alerts and dashboard
Should · Source: `lith:site/content/docs/guides/monitoring.md` ("The dashboard", "Alerts") · Today: missing.

1. The host's lith dashboard shows tickit; alerts fire for failed probes, a 5xx rate over 1 % for 5 minutes, tick overruns, a parked projector and a failed nightly reset.

## 9. Out of scope for the first version

- Sign-in with WebAuthn, a second factor, OAuth, SSO or magic mail (FR-133 to FR-136); project invitations, avatar uploads and account export (FR-131, FR-132, FR-145).
- Mail of any kind (FR-118, Q16).
- Files attached to issues (FR-30); images from other sites (DM-06).
- A query language (FR-61), a JSON API (FR-151), imports of tickit's old data or of real Jira and Confluence content (FR-150).
- Deleting projects and changing project keys (FR-06, FR-07).
- Following another person's view (MP-12).
- Page restrictions per page, drafts hidden from readers (Q18), PDF export, macros other than issue chips and issue lists, templates with variables, page analytics.
- Work logs, releases and fix versions, roadmaps, automation rules, custom fields, issue security levels, a service desk.
- More than one tenant, languages other than English, native apps, offline use beyond MP-05.6.
- TLS and HTTP/3 in the app (Caddy terminates TLS), the Windows build, NATS.

## 10. Gap detection

### 10.1 Capabilities that Jira and Confluence users expect

"Expected from" names the product whose users take the capability for granted. "Rewrite" gives the priority and the requirement.

| Capability | Expected from | tickit today | Rewrite | Note |
| --- | --- | --- | --- | --- |
| Projects with keys | Jira | partly works | Must, FR-01 | Keys exist; listed only in the admin panel |
| Issue numbers per project (KEY-123) | Jira | broken | Must, FR-11 | Every issue is number 1 |
| Open an issue by its key | Jira | broken | Must, FR-12 | The route lowercases the key |
| Create an issue from anywhere | Jira | stub | Must, FR-10 | The form has no action |
| Edit summary and description in place | Jira | stub | Must, FR-14 | API only |
| Issue types: epic, story, task, bug, subtask | Jira | missing | Must, FR-19 | |
| Epics with child issues | Jira | missing | Should, FR-20 | |
| Subtasks | Jira | missing | Should, FR-21 | |
| Links between issues | Jira | missing | Should, FR-22 | |
| Statuses | Jira | partly works | Must, FR-35 | Seven fixed statuses, no categories |
| Configurable workflows | Jira | missing | Could, FR-36, FR-37 | |
| Kanban board with columns from statuses | Jira | missing | Must, FR-40 | |
| Drag and drop on the board, live | Jira | missing | Must, FR-41, MP-04 | lith's board demo is the pattern |
| Backlog | Jira | missing | Should, FR-45 | |
| Sprints | Jira | missing | Could, FR-46 | |
| Assignee | Jira | stub | Must, FR-17 | The picker has one option, "Me", that does nothing |
| Reporter | Jira | broken | Must, FR-10 | The reporter is the browser session's id |
| Priority | Jira | works | Must, FR-18 | Not live for others |
| Watchers | Jira | missing | Should, FR-27 | |
| Labels | Jira, Confluence | missing | Should, FR-23; Could, FR-93 | |
| Components | Jira | missing | Could, FR-24 | |
| Due date | Jira | missing | Should, FR-25 | |
| Estimates and story points | Jira | missing | Could, FR-26 | |
| Comments | Jira, Confluence | stub | Must, FR-65; Should, FR-90 | A fixed placeholder comment |
| @mentions | Jira, Confluence | missing | Should, FR-67 | |
| Issue history | Jira | stub | Must, FR-68 | A fixed placeholder line; lith's events give it |
| Activity feed | Jira, Confluence | missing | Should, FR-69 | |
| Issue or page as of a past version | neither (lith's Town in time) | missing | Should, FR-70 | A feature only an event log makes cheap |
| Filters on the issue list | Jira | partly works | Must, FR-51 | Project and text only |
| Text search in issues | Jira | works | Must, FR-52 | Wildcards leak into LIKE |
| Sorting | Jira | partly works | Must, FR-53 | Priority and status sort alphabetically |
| Pages of results | Jira | stub | Must, FR-54 | Buttons do nothing |
| Shareable filter links | Jira | partly works | Must, FR-55 | Uncommitted work reloads the page per keystroke |
| Saved filters | Jira | missing | Should, FR-56 | |
| Bulk edit | Jira | missing | Could, FR-58 | |
| Notifications | Jira, Confluence | partly works | Must, FR-110 to FR-116 | Live, but only "Issue Viewed" and "Server Started" create them |
| Dashboards | Jira | missing | Could, FR-60; Should, FR-59 | |
| Query language (JQL) | Jira | missing | Won't for now, FR-61 | |
| Attachments | Jira, Confluence | stub | Won't for now, FR-30; Should, FR-84 | Unauthenticated `/upload` |
| Spaces | Confluence | missing | Must, FR-75 | |
| Page tree | Confluence | missing | Must, FR-78 | |
| Rich-text editor | Confluence | missing | Must, FR-82 | |
| Tables, code blocks, images in pages | Confluence | missing | Must (code), FR-82; Should, FR-83, FR-84 | |
| Several people editing one page at once | Confluence | missing | Must, FR-86, MP-05 | The hard part; see Appendix A |
| Page versions | Confluence | missing | Must, FR-87 | |
| Diff and restore of versions | Confluence | missing | Should, FR-88, FR-89 | |
| Page comments | Confluence | missing | Should, FR-90 | |
| Inline comments | Confluence | missing | Could, FR-91 | Anchors must follow edits |
| Templates | Confluence | missing | Could, FR-92 | |
| Issue chips on pages, live | Confluence with Jira | missing | Must, FR-100 | |
| Pages listed on an issue | Jira with Confluence | missing | Must, FR-101 | |
| Links between pages | Confluence | missing | Should, FR-103 | |
| Search across pages and issues | Confluence with Jira | missing | Must, FR-57 | |
| Page trash | Confluence | missing | Should, FR-94 | |
| Presence on issues, boards, pages | Confluence (editor), Jira (none by default) | missing | Must, MP-02 | lith's hub has presence views |
| Live updates everywhere | neither by default | partly works | Must, MP-03 | Only notifications are live on master |
| Carets of other editors | Confluence | missing | Should, MP-06 | |
| "Is editing" and typing indicators | Confluence | missing | Should, MP-07 | |
| A clear message when two people change one field | Jira | missing | Must, MP-08 | Last write wins today, silently |
| Pending state and refusals | neither | missing | Must, MP-09 | |
| Visitors try it without an account | demo | missing | Must, DM-01 | Every page needs a login |
| Seeded, coherent content | demo | partly works | Must, DM-02 | Placeholder text, all To Do, all number 1 |
| Periodic reset | demo | missing | Must, DM-03 | |
| Rate limits and content limits | demo | stub | Must, DM-04, DM-05 | Limiters exist, unused |
| Moderation | demo | missing | Should, DM-07 | |
| Login | both | works | Must, FR-122 | |
| Registration and password reset | both | partly works | Could, FR-126 to FR-128 | No mail in production |
| Account administration | both | partly works | Should, FR-140 to FR-143 | List works, edits do not |
| Access control per project | both | broken | Must, NFR-01 | APIs without login; roles not enforced |
| Health, metrics, backups | operations | missing | Must, NFR-75, NFR-76; Should, NFR-70 | `cp` of WAL files as backup |

### 10.2 Contradictions within tickit's sources

1. **Status and priority values.** The model has seven statuses ("To Do", "In Review", "Blocked" and four more) and four priorities ending in "Critical" (`internal/issues/model.go:9-45`); the uncommitted dashboard filters by "open", "in_progress", "resolved", "closed" and by "urgent" (`wip:web/ui/pages/dashboard.templ`). A filter on those values matches no issue.
2. **Documented as done, not wired.** `kiro:tasks.md` marks all 20 tasks done; the OAuth summary calls the integration "complete and ready for production use" (`docs/user-management/oauth_implementation_summary.md:184`); the SSO page claims SAML 2.0 support (`docs/user-management/SSO_IMPLEMENTATION.md:196`) and lists SAML as "Future Implementation" (`:82-90`); the admin page claims MFA reset and SSO configuration (`docs/user-management/ADMIN_INTERFACE.md:207-208`), which have no route. In the code, 6 of 33 user commands are registered (`internal/bootstrap/deps.go:168-173`).
3. **Rebuildable read models.** `docs/technical/CQRS.md:48-52` and `CLAUDE.md` say the read models rebuild from the event log; only notification events have deserializers, and `RebuildAllFromEvents` has no caller (analysis §1).
4. **Server-side sessions.** `CLAUDE.md` describes "a server-side store via `internal/session`"; the store is a signed cookie (`internal/session/session_config.go:103-116`).
5. **Password rules.** `kiro:requirements.md` 1.5 asks for 8 characters; `kiro:design.md` asks for 8 with mixed case and digits and no reuse of the last 5; `internal/users/validation.go:34-66` demands upper case, lower case and a digit, keeps no history, and allows 128 characters that bcrypt cuts at 72 bytes; the reset page promises "at least 8 characters" only (`web/ui/pages/forgot_password.templ:79`); the seeded passwords "admin123" and "pm123" fail the code's own rule (`internal/migrations/seed.go:64`, `:105`).
6. **The Viewer role.** It is "Read-only access to assigned projects" (`internal/users/rbac.go:107`) and holds the permission to comment (`:109-112`).
7. **Two role systems.** A member's permissions in a project are the union of site role and project role (`internal/users/rbac.go:221-268`), so a site Developer who is a project Viewer may edit that project's issues.
8. **"Issue Viewed".** The code comments say the notification goes "to all connected clients" (`internal/routes/pages.go:144`, `:191`); `CreateNotification` addresses the viewer only.
9. **General notifications.** The page lists the person's and the general notifications (`internal/notifications/service.go:144-187`); the live renderer lists only the person's (`internal/realtime/renderer_notifications.go:35`), so the general ones vanish at the first live update; and nobody can mark them read.
10. **The stream endpoint.** The grouping document names `/api/updates` (`docs/event-driven-grouping-requirements.md:212`, `:584`); the code serves `/updates` (`internal/routes/api.go:17`). The document asks for one query per group; the code renders twice per tab.
11. **Requirements that name libraries.** `kiro:requirements.md` requires Gorilla Sessions (2.9), nosurf (12.10) and Goth (1.2, 10.2); on lith, `edge.Sessions` and `edge.CSRF` take their place.
12. **Not revealing accounts.** `kiro:requirements.md` 6.8 forbids revealing whether an account exists; in production the reset mail fails for existing accounts only and the request returns that error (`internal/users/command_handlers.go:847`).
13. **Development mode.** `docs/INSTALL.md:121`, `:148-150` sets `GO_ENV` and `DATABASE_URL`, which no code reads; mail and magic mail switch on `ENVIRONMENT` (`internal/users/email_service.go:157`), so a host set up by the guide runs them in development mode and writes verification and reset links to the journal.
14. **Magic mail.** Meant for development only (`kiro:requirements.md` 11.1, 11.7), routed always (`internal/routes/api.go:131-133`), and its mailed URL `/auth/magic-login` is not a route (`internal/users/email_service.go:144`).
15. **What tickit is.** The README promises "ticket management and event organization" (`README.md:3`); nothing in the code organises events.
16. **Session lifetime.** The cookie lives 24 hours (`internal/session/session_config.go:34-35`), the cookie codec accepts it for 30 days (analysis §5), and `kiro:design.md` asks for configurable idle and absolute timeouts.
17. **The Project column.** Its sort key is "project" (`web/ui/components/issues.templ:39`), which the query does not accept, so it sorts by id (`internal/issues/projection.go:243-257`).
18. **Language.** The interface is English; notification times are German (`web/ui/components/notification.templ:15`).

### 10.3 Gaps within tickit's sources

- No source says who may read a project's issues: `view_project` exists in the RBAC table (`internal/users/rbac.go:40`) and no route checks it.
- No source says what deleting an issue does to its key, its history and the pages that mention it, or what moving an issue does to its number.
- No source gives a number for latency, capacity, page load or search; the grouping document has goals without figures.
- No source covers accessibility, browser support or privacy beyond "GDPR: audit logging" (`docs/user-management/SECURITY_IMPLEMENTATION.md:218`).
- No source covers comments, history, attachments or labels beyond placeholders and a code comment (`internal/issues/model.go:47-48`).
- No source covers two people changing the same thing at once.
- No source limits the rate of actions other than sign-in.
- No source covers the wiki side, boards, presence or the demo's operation; sections 5.4, 5.7, 5.8, 6 and 7 come from the benchmark.

### 10.4 Gaps found in review

The review of this draft found these, now requirements or notes:

- **A reset or a removed page during an edit** (MP-13): DM-03 deletes everything at the reset, and MP-05.6 merges an editor's held edits after a reconnect. Nothing said what happens when the two meet; an edit must never land on the reseeded page.
- **Refusals in the editor** (MP-14): MP-09 covers controls that send one change; an editor sends a stream of them.
- **The event log under co-editing** (NFR-43): at NFR-38's load, 20 editors send up to 200 edit batches a second, about 17 million a day. Without folding them into versions, the log outgrows the host within days whenever a reset fails, and `lith/events` has no retention per stream yet (G6).
- **Search through rebuilds** (NFR-44): whether lith's rebuild into copies handles an FTS5 table is untested (Q11). It needs a test in lith before tickit relies on it.
- **Order of work.** MP-05 decides the editor, the shape of page events, a new delivery path in lith (operations without gaps; section 6.1) and NFR-43. Build it first, as the spike of Appendix B, before the rest of the pages side.
- **WebKit.** Section 12, item 9 asks for browser tests in WebKit. The development machine has Playwright's WebKit build, but it needs system libraries installed before it runs.

## 11. Open questions for the user

Decided on 16 October 2026: every proposal below holds. Co-editing runs on lith, with the writer ordering every edit (Q19); no CRDT. Each question has its decision and the reason for it, and lists the requirements it blocked. Questions 1 to 13 are the analysis's decisions (analysis §6); 14 to 25 come from the widened target.

| # | Question | Decision | Reason | Blocked |
| --- | --- | --- | --- | --- |
| Q1 | Which account features survive? | Port what works (password login, change and reset password, the account list); drop WebAuthn, MFA, OAuth and SSO for the first version. With Q14's guests, registration, verification and reset drop to Could. | None of the dropped features works today (analysis §2); demo visitors need none of them. | FR-122 to FR-136, NFR-07 to NFR-10 |
| Q2 | Are users event-sourced? | No, they stay relational. Guests and accounts share one table of people; the security log (FR-156) is a plain table. | Erasing a person is an update of one row; tickit's account code decides from tables already. | Section 4.1, FR-120, FR-156, NFR-26, NFR-27 |
| Q3 | Which keys? | UUID strings for issues, pages, projects and spaces; people see issue keys and page titles. | At demo size random keys cost nothing measurable (analysis §2); old `/ticket/<uuid>` links keep resolving. | FR-12, FR-13, FR-80, FR-103 |
| Q4 | Import today's data or reseed? | Reseed. | Today's data is placeholder text with every issue numbered 1 and 604 "Server Started" notifications; the demo resets nightly anyway. | FR-150, DM-02, section 12 |
| Q5 | Issue numbers per project? | Yes, `KEY-123`; `/KEY-123` opens the issue; a moved issue gets a new number and its old key keeps working. | It is how Jira users name issues. | FR-11, FR-12, FR-29 |
| Q6 | What notifies, and how are notifications stored? | Drop "Issue Viewed" and "Server Started". One row per recipient in a plain table; assignment notifies (Must), mentions and watched issues (Should), page comments (Could); broadcasts only for reset warnings. | Retention per stream is missing in `lith/events` (gap G6); a table keeps read state per person. | FR-110 to FR-118, DM-03 |
| Q7 | Concurrent edits of issue fields? | Expected versions, checked per field (MP-08). | A check of the whole issue refuses two people who change different fields of one issue, which happens all the time on a live board. | MP-08, FR-14 to FR-18, FR-41 |
| Q8 | Carry over today's sessions? | No: everyone logs in again; with Q14, only staff log in at all. | The old cookie holds a user id signed with a key that may be the default (analysis §3). | FR-122, FR-124, section 12 |
| Q9 | Where do filters live? | In the URL, mirrored in the tab's state. | Links reproduce a list; a tab keeps its filter across navigation. | FR-42, FR-55, FR-56 |
| Q10 | Keep the JSON `/api/*`? | Drop it. | Nothing but the pages uses it, and it is the largest hole in the demo today. | FR-151, NFR-01, NFR-02 |
| Q11 | How to pin lith? | `replace` to the `v0.3.0` branch (now `3fcebd5`, which documents G2 and G3) until the tag; bring G1, G5, G6 and the needs of section 6.1 to lith before tagging, and check whether a projector's rebuild, which copies tables from their `CREATE` statements, handles an FTS5 search table (`lith:site/content/docs/guides/events.md:185`). | `events` is unreleased and may change (analysis §6). | NFR-60, MP-05, FR-57, FR-69 |
| Q12 | The demo until the cut-over? | Hotfix it now (analysis §6, slice 0), or take it offline. | Its APIs take writes without a login today. | Nothing in the rewrite |
| Q13 | Comments and history now or later? | Issue comments (FR-65), issue history (FR-68) and page versions (FR-87) are Must; page comments, diff and restore Should; inline comments Could. | Comments, history and versions are what make it feel like Jira and Confluence, and lith's events give history at little cost. | FR-65 to FR-70, FR-87 to FR-91 |
| Q14 | Anonymous visitors or logins? | Guests act without an account under a pseudonym, with Member rights in open projects and spaces; accounts exist for moderators and admins only; registration is off. | A second person must be there within seconds (a private window); the host sends no mail, so registration cannot verify anyone; lith's own demos work this way; DM-03 to DM-09 contain abuse. With logins required, strangers share a few seeded accounts with public passwords, and presence shows them all under one name. | Section 3, FR-120 to FR-128, DM-01, DM-04 to DM-08 |
| Q15 | Which role model? | Site roles none, moderator, admin; project and space roles Viewer, Member, Admin. | tickit's two overlapping role systems grant by union (section 10.2, item 7). | Section 3, FR-05, FR-142, NFR-01 |
| Q16 | Does the first version send mail? | No. | `internal/users/email_service.go` has no production sender; guests need no mail. | FR-118, FR-126 to FR-128, NFR-08 |
| Q17 | Which default workflow? | tickit's seven statuses in three categories, every transition allowed; the board shows all but Backlog. The alternative is Jira's leaner Backlog, To Do, In Progress, In Review, Done. | The seed and tickit's code use the seven; the board then has six columns, of which "Selected for Development" and "To Do" mean nearly the same. | FR-35 to FR-38, FR-40, FR-43, DM-02 |
| Q18 | Do readers see edits live, or only after Publish? | Live; versions are cut after 60 s without edits, when the last editor leaves, and on "Save version". | Watching a page change is the point of the demo; Confluence's drafts hide edits until Publish. | FR-80, FR-86, FR-87, MP-05 |
| Q19 | How does co-editing work? | On lith: Appendix A, option 1, operations as commands ordered by the writer; no CRDT. Option 3 (block locks) is an early slice if option 1 runs late. | Option 1 keeps the server authoritative and maps onto `events.Exactly`. | MP-05, MP-06, FR-15, FR-82, FR-86, FR-91 |
| Q20 | Which images? | The seeded library (Should); uploads Could, behind moderation; never images from other sites. | Uploads to a public demo invite content that moderators must catch before the reset. | FR-84, FR-85, NFR-15, DM-06 |
| Q21 | When does the demo reset? | Nightly at 04:00 Europe/Zurich, plus "Reset now". | Predictable and outside European working hours; a reset after a quiet hour, as the board demo does, would wipe a page its author comes back to after lunch. | DM-03, DM-10 |
| Q22 | Are issue descriptions co-edited? | Yes, with the page editor once MP-05 works; until then plain text under MP-08's field check. | One editor for descriptions and pages; a description is where two people collide most. | FR-14, FR-15 |
| Q23 | Which password rules? | 8 to 128 characters, every character significant, no composition rule. | The sources disagree (section 10.2, item 5). | NFR-07 |
| Q24 | Who creates projects and spaces? | Members, guests included, 2 per session a day. | Creating a project belongs to the Jira experience; the limit and the reset bound the clutter. | FR-02, FR-76, DM-04 |
| Q25 | Does the privacy page and the single cookie need a legal check? | Yes: zwei und eins gmbh checks the privacy page against the Swiss FADP and the GDPR before go-live; one strictly necessary cookie and no banner. | The demo takes free text from the public. | NFR-31, DM-10 |

Q14, Q13, Q19 and Q18 block the most Must requirements; Q1 blocks the most requirements in number, most of them Could or Won't for now.

## 12. Acceptance of the rewrite

The rewrite replaces the current demo at `tickitdemo.zweiundeins.gmbh`, served at `tickit.zweiundeins.gmbh`, when all of this is true:

1. Every Must requirement passes its acceptance criteria, and each criterion has an automated test or a written manual check in the repository.
2. This document reflects the decisions of section 11 (16 October 2026) and any made since.
3. A security script run against a production build shows: every action without the right answers 403 and changes nothing; a cross-site POST answers 403; every GET leaves the database unchanged; the logs of a full test run hold none of the seeded secrets; a fuzz run of every action finds no 500 (NFR-01 to NFR-16).
4. Browser tests with at least three headless browsers show: a face appears within two ticks and leaves within 1 s of its tab closing (MP-02); a card moved in one browser shows in the others within 250 ms on loopback (MP-04); of two simultaneous moves of one card, the second is refused with the first mover's name; five scripted editors typing in the same paragraph for 60 s end with identical documents that hold every typed character (MP-05); a field conflict shows MP-08's message; after a server restart every page shows the current state without a reload (MP-10).
5. The load of NFR-38 is measured and recorded in `benchmarks/RESULTS.md`, and NFR-35 and NFR-36 hold.
6. Every decider has `eventstest` cases, every event type a JSON sample checked by `eventstest.Samples`, and a test rebuilds every read model from the events (FR-155.2).
7. On the host, the seed runs in under 30 s, the nightly reset has run three times without error, a moderator account exists, and the rate limits of DM-04 answer 429 when exceeded.
8. axe finds no serious or critical issue on the pages of NFR-45; a keyboard-only walk through creating an issue, moving a card and editing a page succeeds; every page works at 400 % zoom.
9. The browser tests pass in Chromium, Firefox and WebKit.
10. `app-deploy` deploys and rolls back on the host; the probes stay green for 24 hours; Prometheus scrapes the metrics; Alloy ships the logs; the Caddy block has no `encode`; the NATS port and the old binaries are gone.
11. A restore from a daily copy has been tried on the host and its steps are written down.
12. README, the project's `CLAUDE.md` and the deploy README describe the new tickit; the old documents of features not built are removed or marked as history.
13. The current demo's databases are archived for 30 days and then deleted; nothing of them is imported (Q4).

## 13. Traceability

Generated from the requirement blocks above: one row per requirement with its priority, its source and today's status.

### 13.1 Requirements by area and priority

| Area | Must | Should | Could | Won't for now | All |
| --- | --- | --- | --- | --- | --- |
| Projects | 1 | 1 | 3 | 2 | 7 |
| Issues | 9 | 8 | 3 | 1 | 21 |
| Workflows and statuses | 1 | 1 | 2 | 0 | 4 |
| Boards, backlog and sprints | 2 | 2 | 3 | 0 | 7 |
| Search, filters, bulk edit and dashboards | 7 | 2 | 2 | 1 | 12 |
| Comments, mentions, activity and history | 2 | 4 | 0 | 0 | 6 |
| Spaces and pages | 7 | 8 | 6 | 0 | 21 |
| Links between pages and issues | 2 | 2 | 2 | 0 | 6 |
| Notifications | 4 | 3 | 1 | 1 | 9 |
| Identity, accounts and sign-in | 5 | 1 | 6 | 6 | 18 |
| Administration | 0 | 3 | 2 | 1 | 6 |
| Import, export and API | 0 | 0 | 1 | 2 | 3 |
| Audit | 1 | 1 | 0 | 0 | 2 |
| Multiplayer | 10 | 3 | 0 | 1 | 14 |
| Demo operation | 8 | 1 | 2 | 0 | 11 |
| Non-functional: Security | 17 | 3 | 0 | 0 | 20 |
| Non-functional: Privacy | 6 | 1 | 0 | 0 | 7 |
| Non-functional: Performance and realtime | 8 | 2 | 0 | 0 | 10 |
| Non-functional: Accessibility | 5 | 2 | 0 | 0 | 7 |
| Non-functional: Browsers | 1 | 1 | 0 | 0 | 2 |
| Non-functional: Deployment on the shared host | 8 | 0 | 0 | 0 | 8 |
| Non-functional: Backups | 2 | 1 | 0 | 0 | 3 |
| Non-functional: Observability | 3 | 1 | 0 | 0 | 4 |
| **All** | **109** | **51** | **33** | **15** | **208** |

### 13.2 Requirements to sources

| ID | Requirement | Priority | Source | Today |
| --- | --- | --- | --- | --- |
| FR-01 | Project list and keys | Must | `web/ui/components/projects.templ:23-67`; `internal/projects/model.go:10-18` | partly works |
| FR-02 | Create a project | Should | `web/ui/components/projects.templ:5-21`; `internal/projects/service.go:35-52` | partly works |
| FR-03 | Edit a project | Could | `web/ui/components/projects.templ:41-50` | broken |
| FR-04 | Archive a project | Could | `web/ui/components/projects.templ:53-61`; `internal/projects/service.go:76-91` | broken |
| FR-05 | Project members and roles | Could | `internal/routes/api.go:53-59`; `kiro:requirements.md` requirement 3 | broken |
| FR-06 | Delete a project | Won't for now | `internal/projects/commands.go:117-132` | broken |
| FR-07 | Change a project key | Won't for now | `internal/projects/commands.go:40-48` | missing |
| FR-10 | Create an issue | Must | `web/ui/pages/create_ticket.templ:12`; `internal/issues/commands.go:121-177` | stub |
| FR-11 | Issue numbers and keys | Must | `internal/issues/model.go:51`, `:63-66`; analysis §1 | broken |
| FR-12 | Open an issue by key | Must | `internal/routes/pages.go:53-96`; `web/ui/components/issues.templ:83` | broken |
| FR-13 | View an issue | Must | `web/ui/pages/ticket_detail.templ:10-81`; `web/ui/components/issues.templ:132-206` | partly works |
| FR-14 | Edit summary and description | Must | `internal/issues/commands.go:254-339` | stub |
| FR-15 | Rich-text description | Should | inferred (Jira) | missing |
| FR-16 | Change status | Must | `web/ui/components/issues.templ:149-156`; `internal/issues/model.go:9-29` | partly works |
| FR-17 | Assign and unassign | Must | `web/ui/components/issues.templ:167`; `internal/users/project_integration.go:104-136`; `docs/user-management/INTEGRATION_IMPLEMENTATION.md:78-84` | stub |
| FR-18 | Change priority | Must | `web/ui/components/issues.templ:157-164`; `internal/issues/model.go:31-45` | partly works |
| FR-19 | Issue types | Must | inferred (Jira); `internal/issues/model.go:47-48` ("Extendable for more fields") | missing |
| FR-20 | Epics and their issues | Should | inferred (Jira) | missing |
| FR-21 | Subtasks | Should | inferred (Jira) | missing |
| FR-22 | Issue links | Should | inferred (Jira) | missing |
| FR-23 | Labels on issues | Should | inferred (Jira); `internal/issues/model.go:47-48` ("e.g. labels") | missing |
| FR-24 | Components | Could | inferred (Jira) | missing |
| FR-25 | Due date | Should | inferred (Jira) | missing |
| FR-26 | Estimates | Could | inferred (Jira) | missing |
| FR-27 | Watchers | Should | inferred (Jira) | missing |
| FR-28 | Delete an issue | Should | `internal/issues/commands.go:237-252`; `internal/users/rbac.go:38` (delete_issue) | broken |
| FR-29 | Move an issue to another project | Could | `web/ui/components/issues.templ:168-176` | partly works |
| FR-30 | Attachments on issues | Won't for now | `web/ui/pages/create_ticket.templ:33`; `internal/routes/routes_upload.go` | stub |
| FR-35 | Default workflow | Must | `internal/issues/model.go:9-29`; `internal/issues/commands.go:92-119` | partly works |
| FR-36 | Configure a workflow | Could | inferred (Jira) | missing |
| FR-37 | Transition rules | Could | inferred (Jira) | missing |
| FR-38 | Resolution time | Should | inferred (Jira) | missing |
| FR-40 | Kanban board | Must | inferred (Jira) | missing |
| FR-41 | Move cards | Must | inferred (Jira); `lith:site/internal/demos/board/how.md:1-17` | missing |
| FR-42 | Board quick filters | Should | inferred (Jira); `web/ui/pages/dashboard.templ:38-41` ("User: Me") | missing |
| FR-43 | Configure board columns | Could | inferred (Jira) | missing |
| FR-44 | Swimlanes | Could | inferred (Jira) | missing |
| FR-45 | Backlog | Should | inferred (Jira) | missing |
| FR-46 | Sprints | Could | inferred (Jira) | missing |
| FR-50 | Issue list | Must | `web/ui/components/issues.templ:27-130`; `internal/realtime/renderer_issue_table.go:53-150` | works |
| FR-51 | Filter issues | Must | `internal/issues/queries.go:20-48`; `web/ui/pages/dashboard.templ:28-47`; `wip:web/ui/pages/dashboard.templ` | partly works |
| FR-52 | Text search in issues | Must | `internal/issues/projection.go:229-233`; `web/ui/pages/dashboard.templ:44` | works |
| FR-53 | Sort issues | Must | `web/ui/components/issues.templ:33-75`; `internal/issues/projection.go:242-261` | partly works |
| FR-54 | Pages of results | Must | `web/ui/pages/dashboard.templ:53-58`; `docs/event-driven-grouping-requirements.md:16-22`; `wip:internal/issues/filters.go:91-115` | stub |
| FR-55 | Filter, sort and page in the URL and the tab | Must | `wip:web/ui/pages/dashboard.templ`; `web/ui/js/signal_storage.ts:1-2`; analysis §4, decision 9 | partly works |
| FR-56 | Saved filters | Should | inferred (Jira) | missing |
| FR-57 | Search across pages and issues | Must | inferred (Confluence, Jira) | missing |
| FR-58 | Bulk edit | Could | inferred (Jira) | missing |
| FR-59 | Home page with your work | Should | inferred (Jira, Confluence) | missing |
| FR-60 | Dashboards | Could | inferred (Jira) | missing |
| FR-61 | Query language | Won't for now | inferred (Jira) | missing |
| FR-65 | Comment on an issue | Must | `web/ui/components/issues.templ:188-198`; analysis decision 13 | stub |
| FR-66 | Edit and delete own comments | Should | inferred (Jira) | missing |
| FR-67 | Mentions | Should | inferred (Jira, Confluence) | missing |
| FR-68 | Issue history | Must | `web/ui/components/issues.templ:200-203`; `lith:site/content/docs/guides/events.md:136-151` | stub |
| FR-69 | Activity feed | Should | inferred (Jira, Confluence); `lith:site/content/docs/guides/events.md:234-240` | missing |
| FR-70 | Issue or page as of an earlier version | Should | `lith:site/internal/demos/town/how.md` ("The slider", "Views keyed by position") | missing |
| FR-75 | Spaces and their home pages | Must | inferred (Confluence) | missing |
| FR-76 | Create a space | Should | inferred (Confluence) | missing |
| FR-77 | Space settings and archive | Could | inferred (Confluence) | missing |
| FR-78 | Page tree | Must | inferred (Confluence) | missing |
| FR-79 | Move and reorder pages | Should | inferred (Confluence) | missing |
| FR-80 | View a page | Must | inferred (Confluence) | missing |
| FR-81 | Create a page | Must | inferred (Confluence) | missing |
| FR-82 | Rich-text editor | Must | inferred (Confluence) | missing |
| FR-83 | Tables in pages | Should | inferred (Confluence) | missing |
| FR-84 | Images from the library | Should | inferred (Confluence, demo) | missing |
| FR-85 | Image uploads | Could | inferred (Confluence); `docs/user-management/PROFILE_MANAGEMENT.md:51-57` | stub |
| FR-86 | Edit a page together | Must | inferred (Confluence); `lith:skills/lith/SKILL.md` rule 5 | missing |
| FR-87 | Page versions | Must | inferred (Confluence); analysis decision 13 | missing |
| FR-88 | Compare versions | Should | inferred (Confluence) | missing |
| FR-89 | Restore a version | Should | inferred (Confluence) | missing |
| FR-90 | Page comments | Should | inferred (Confluence) | missing |
| FR-91 | Inline comments | Could | inferred (Confluence) | missing |
| FR-92 | Page templates | Could | inferred (Confluence) | missing |
| FR-93 | Labels on pages | Could | inferred (Confluence) | missing |
| FR-94 | Page trash | Should | inferred (Confluence) | missing |
| FR-95 | Watch a page | Could | inferred (Confluence) | missing |
| FR-100 | Issue chips on pages | Must | inferred (Confluence) | missing |
| FR-101 | Pages that mention an issue | Must | inferred (Confluence, Jira) | missing |
| FR-102 | Link a page from an issue | Should | inferred (Jira) | missing |
| FR-103 | Links between pages | Should | inferred (Confluence) | missing |
| FR-104 | Issue list on a page | Could | inferred (Confluence) | missing |
| FR-105 | Create an issue from a page | Could | inferred (Confluence) | missing |
| FR-110 | Notification panel | Must | `web/ui/components/notification.templ:11-44`; `web/ui/layouts/base.templ:80-84` | works |
| FR-111 | Notify on assignment | Must | analysis decision 6; `docs/user-management/INTEGRATION_IMPLEMENTATION.md:34-44` | missing |
| FR-112 | Notify on mention | Should | inferred (Jira, Confluence) | missing |
| FR-113 | Notify watchers | Should | inferred (Jira) | missing |
| FR-114 | Notify on page comments | Could | inferred (Confluence) | missing |
| FR-115 | Mark read and dismiss | Must | `web/ui/components/notification.templ:16-20`, `:34-35`; `internal/notifications/commands.go:93-207` | partly works |
| FR-116 | No notifications for views and restarts | Must | `internal/routes/pages.go:130-159`, `:178-198`; `cmd/web/main.go:159-161`; analysis decision 6 | broken |
| FR-117 | Notification retention | Should | inferred (lith); `lith:site/content/docs/guides/events.md:305-313` | missing |
| FR-118 | Email notifications | Won't for now | inferred (Jira); `internal/users/email_service.go:57-60` | missing |
| FR-120 | Guest identity | Must | inferred (demo); `lith:site/internal/demos/board/avatar.go:10-40`; Q14 | missing |
| FR-121 | Change one's display name | Could | inferred (demo) | missing |
| FR-122 | Log in | Must | `kiro:requirements.md` requirement 2; `internal/users/command_handlers.go:352-475`; `web/ui/pages/login.templ:11-29` | works |
| FR-123 | Log out | Must | `kiro:requirements.md` requirement 2.7; `internal/users/routes.go:395-415` | partly works |
| FR-124 | Login sessions expire | Must | `internal/session/session_config.go:30-41`; `kiro:requirements.md` requirement 2.6 | partly works |
| FR-125 | Change password | Should | `kiro:requirements.md` requirement 4.3; `internal/users/command_handlers.go:1004-1092` | partly works |
| FR-126 | Register an account | Could | `kiro:requirements.md` requirement 1; `web/ui/pages/login.templ:48-62` | partly works |
| FR-127 | Verify an email address | Could | `kiro:requirements.md` requirements 1.6 and 1.7; `internal/users/command_handlers.go:276-350` | partly works |
| FR-128 | Reset a forgotten password | Could | `kiro:requirements.md` requirement 6; `web/ui/pages/forgot_password.templ` | partly works |
| FR-129 | See and end one's sessions | Could | `docs/technical/USER_ROUTES_IMPLEMENTATION.md:113-118` | broken |
| FR-130 | Edit a profile | Could | `docs/user-management/PROFILE_MANAGEMENT.md`; `kiro:requirements.md` requirement 4 | broken |
| FR-131 | Avatar upload | Won't for now | `docs/user-management/PROFILE_MANAGEMENT.md:41-57`; `internal/users/avatar_service.go` | broken |
| FR-132 | Project invitations | Won't for now | `kiro:requirements.md` requirement 5; `internal/users/command_handlers.go:1545-1855` | broken |
| FR-133 | Magic mail login | Won't for now | `kiro:requirements.md` requirement 11 | broken |
| FR-134 | WebAuthn and multi-factor login | Won't for now | `kiro:requirements.md` requirement 8; `internal/users/webauthn_service.go` | broken |
| FR-135 | OAuth login | Won't for now | `kiro:requirements.md` requirements 7 and 9; `docs/user-management/oauth_implementation_summary.md` | stub |
| FR-136 | Single sign-on | Won't for now | `kiro:requirements.md` requirement 10; `docs/user-management/SSO_IMPLEMENTATION.md` | stub |
| FR-137 | Header and navigation | Must | `web/ui/layouts/base.templ:58-74` | partly works |
| FR-140 | Account list | Should | `web/ui/pages/admin_panel.templ:9-29`; `docs/user-management/ADMIN_INTERFACE.md:11-51` | works |
| FR-141 | Create an account | Could | `web/ui/components/users.templ:12-36` | broken |
| FR-142 | Assign site roles | Should | `web/ui/components/users.templ:58-73`; `docs/user-management/ADMIN_INTERFACE.md:107-109` | broken |
| FR-143 | Deactivate and reactivate accounts | Should | `kiro:requirements.md` requirements 12.3 and 12.4; `web/ui/components/users.templ:75-84` | broken |
| FR-144 | Admin statistics | Could | `docs/user-management/ADMIN_INTERFACE.md:111-113`, `:146-153` | broken |
| FR-145 | Export accounts | Won't for now | `docs/user-management/ADMIN_INTERFACE.md:92-100`; `kiro:requirements.md` requirement 12.6 | stub |
| FR-150 | Import tickit's old data | Won't for now | analysis §3, decision 4 | missing |
| FR-151 | JSON API | Won't for now | `internal/routes/api.go:26-111`; analysis decision 10 | broken |
| FR-152 | Export issues and pages | Could | inferred (Jira, Confluence) | missing |
| FR-155 | Every change has an author and a time | Must | analysis §1; `lith:site/content/docs/guides/events.md:117` | broken |
| FR-156 | Security log | Should | `docs/user-management/SECURITY_IMPLEMENTATION.md:64-76`; analysis decision 2 | stub |
| MP-01 | One identity across tabs | Must | `lith:site/internal/demos/board/queries.go:51`; inferred (demo) | missing |
| MP-02 | Presence | Must | inferred (Jira, Confluence); `docs/user-management/UI_EVENTS_IMPLEMENTATION.md:23`, `:113-120` (online status) | missing |
| MP-03 | Live updates everywhere | Must | `docs/event-driven-grouping-requirements.md:5-28`, `:472-567`; analysis §4 | broken |
| MP-04 | Live board moves | Must | inferred (Jira); `lith:site/internal/demos/board/how.md:1-90` | missing |
| MP-05 | Co-editing a page without losing edits | Must | inferred (Confluence); `lith:skills/lith/SKILL.md` rule 5 | missing |
| MP-06 | Other people's carets and selections | Should | inferred (Confluence); `lith:site/internal/demos/cursor-party/how.md` | missing |
| MP-07 | Editing and typing indicators | Should | inferred (Jira, Confluence) | missing |
| MP-08 | Conflicts on issue fields | Must | analysis decision 7; `lith:site/internal/demos/town/how.md` ("The decider") | missing |
| MP-09 | Pending state and refusals | Must | `lith:skills/lith/SKILL.md` rule 5; `lith:skills/lith/references/recipes.md` ("A command"); analysis §4 | missing |
| MP-10 | Reconnect and catch-up | Must | `web/ui/layouts/base.templ:49-57`; `lith:site/content/docs/guides/pages.md:135-153`; `lith:site/content/docs/guides/views.md:228-250` | works |
| MP-11 | One render per shared view | Must | `docs/event-driven-grouping-requirements.md:5-35`, `:97-100`, `:119-132`; analysis §1 | broken |
| MP-12 | Follow another person | Won't for now | inferred (Confluence) | missing |
| MP-13 | Edits caught by a reset or a removed page | Must | review; DM-03, DM-07, FR-94 | missing |
| MP-14 | Refused while editing | Should | review; DM-04, DM-07, MP-09 | missing |
| DM-01 | Visitors write without an account | Must | Q14; `lith:site/internal/demos/board/how.md:93-103`; `lith:site/internal/demos/town/how.md` ("The decider": the actor is a hash of the session) | missing |
| DM-02 | Seeded content | Must | `cmd/dev/seed.go:80-142`; `internal/migrations/seed.go:25-206`; analysis decision 4 | partly works |
| DM-03 | Periodic reset | Must | inferred (demo); `lith:site/internal/demos/board/how.md:130-139` | missing |
| DM-04 | Rate limits on every change | Must | `lith:site/internal/demo/kit.go:55-95`; `docs/user-management/SECURITY_IMPLEMENTATION.md:17-38` | stub |
| DM-05 | Content limits | Must | inferred (demo); `lith:site/content/docs/guides/events.md:74-75` (title of 1 to 200 characters) | missing |
| DM-06 | Safe rendering of public content | Must | inferred (demo); `lith:site/internal/demos/guestbook/commands.go:43-45` | missing |
| DM-07 | Moderation | Should | inferred (demo) | missing |
| DM-08 | Report content | Could | inferred (demo) | missing |
| DM-09 | Search engines stay out of visitor content | Must | inferred (demo) | missing |
| DM-10 | Demo notice and privacy page | Must | inferred (demo); `web/ui/layouts/base.templ:85` ("Maintained by zwei und eins gmbh") | partly works |
| DM-11 | First-visit hint | Could | inferred (demo) | missing |
| NFR-01 | Access control on every request and every view | Must | analysis §5; `internal/routes/api.go:26-50` | broken |
| NFR-02 | One route per action, the actor from the session | Must | `internal/routes/routes_updates.go:20-89`, `:109-113`; `internal/issues/routes.go:15-19` | broken |
| NFR-03 | Cross-site requests | Must | `internal/users/csrf.go:105-124`; `lith:site/content/docs/guides/edge.md:56-63` | partly works |
| NFR-04 | Login sessions on the server | Must | `internal/sharedhttp/session.go:142-153`; `internal/session/session_config.go:105-114`; analysis §5 | broken |
| NFR-05 | No default secrets | Must | `config/config.go:66` | broken |
| NFR-06 | GET never writes | Must | `lith:skills/lith/SKILL.md` rule 7; `internal/routes/pages.go:145-149`; `internal/routes/users.go:41` | broken |
| NFR-07 | Passwords | Must | `internal/users/validation.go:34-66`; `internal/users/password_hasher.go:15-26`; analysis §5 | partly works |
| NFR-08 | Tokens | Should | `internal/users/store.go:1326`, `:1439`; `internal/users/command_handlers.go:958-963` | broken |
| NFR-09 | Sign-in rate limits | Must | `docs/user-management/SECURITY_IMPLEMENTATION.md:17-38` | stub |
| NFR-10 | No account enumeration | Must | `internal/users/command_handlers.go:367-385`; `internal/users/routes.go:155-160` | broken |
| NFR-11 | No secrets in logs | Must | `internal/users/routes.go:272`; `internal/routes/routes_updates.go:64-67` | broken |
| NFR-12 | Client address | Must | `internal/users/security.go:258-283`; `lith:site/content/docs/guides/edge.md:94-103` | broken |
| NFR-13 | Output encoding and Content Security Policy | Must | `web/ui/helpers/helpers.go:62`, `:82`, `:137`; `lith:site/content/docs/guides/edge.md:67-92` | partly works |
| NFR-14 | No debug or development code in production | Must | `internal/routes/users.go:43-45`; `web/ui/layouts/base.templ:86-87`; `internal/routes/api.go:131-133` | broken |
| NFR-15 | Image uploads | Should | `internal/routes/routes_upload.go:13`, `:39-40`; `deploy/tickitdemo.caddy` (`/uploads/*`) | broken |
| NFR-16 | Unknown ids and malformed input | Must | `internal/routes/pages.go:128-133`; `internal/routes/routes_updates.go:315` | broken |
| NFR-17 | Atomic changes | Must | analysis §1; `internal/bootstrap/deps.go:57-82`; `internal/issues/commands.go:164-176`, `:211-224` | broken |
| NFR-18 | Uniqueness | Must | `internal/users/command_handlers.go:187`, `:220`; `internal/users/store.go:28`; analysis §5 | broken |
| NFR-19 | Security headers | Must | `deploy/tickitdemo.caddy`; `lith:site/content/docs/guides/edge.md:67-70` | partly works |
| NFR-20 | Dependency checks | Should | inferred (lith) | missing |
| NFR-25 | Little personal data | Must | `docs/user-management/ADMIN_INTERFACE.md:140-144`; analysis §3 | broken |
| NFR-26 | Events without personal data | Must | analysis §2 (`internal/users/events.go:7-10`, `:27-34`, `:62-66`); `lith:site/content/docs/guides/events.md:313` | broken |
| NFR-27 | Erasing a person | Should | `lith:site/content/docs/guides/events.md:309`; inferred | missing |
| NFR-28 | Log retention | Must | `deploy/tickitdemo.caddy` (`roll_keep_for 720h`) | partly works |
| NFR-29 | No third parties | Must | `lith:site/content/docs/guides/edge.md:71-76` | works |
| NFR-30 | Pseudonyms in presence and activity | Must | `lith:site/internal/demos/board/avatar.go:10-16` | missing |
| NFR-31 | One cookie and a privacy page | Must | inferred (demo) | partly works |
| NFR-35 | From commit to screen | Must | `lith:site/content/docs/guides/views.md:259`; `docs/event-driven-grouping-requirements.md:136-146` (BatchWindow); analysis §1 | broken |
| NFR-36 | Action answer time | Must | `lith:skills/lith/references/recipes.md` ("A command": `Do` costs about half a millisecond) | missing |
| NFR-37 | Frame sizes and slow links | Should | `lith:site/internal/demos/board/how.md:141`; `lith:site/content/docs/guides/streams.md:143` | missing |
| NFR-38 | Capacity | Must | `lith:site/content/docs/guides/deploying.md:24`, `:49`; inferred (demo) | missing |
| NFR-39 | Page load | Must | inferred (lith) | missing |
| NFR-40 | Search speed | Should | inferred | missing |
| NFR-41 | Bounded resources | Must | `lith:skills/lith/SKILL.md` rule 9; `lith:site/content/docs/guides/deploying.md:49` | missing |
| NFR-42 | Reconnect after a deploy | Must | `lith:site/content/docs/guides/pages.md:135-153` | missing |
| NFR-43 | Growth of the event log from co-editing | Must | review; NFR-38; gap G6 | missing |
| NFR-44 | Search stays right through rebuilds | Must | review; Q11 | missing |
| NFR-45 | WCAG 2.2 AA | Should | inferred | partly works |
| NFR-46 | Zoom | Must | `web/ui/layouts/base.templ:25` | broken |
| NFR-47 | Keyboard | Must | `web/ui/components/issues.templ:49-73`; `web/ui/components/notification.templ:29` | partly works |
| NFR-48 | Live updates and assistive technology | Must | `lith:site/content/docs/guides/pages.md:93-100`; inferred | missing |
| NFR-49 | Accessible editor | Should | inferred (Confluence) | missing |
| NFR-50 | Semantics, colour and contrast | Must | `web/ui/components/issues.templ:30`, `:93-96` | partly works |
| NFR-51 | Language and dates | Must | `web/ui/components/notification.templ:15`; `web/ui/layouts/base.templ:22` | broken |
| NFR-55 | Supported browsers | Must | `lith:site/content/docs/guides/streams.md:23`; `lith:site/content/docs/guides/edge.md:58-63` | partly works |
| NFR-56 | Small screens and touch | Should | inferred | partly works |
| NFR-60 | Build | Must | `go.mod:3-5`; analysis §1 | missing |
| NFR-61 | systemd unit | Must | `deploy/systemd/tickit.service.d/listen.conf`; `lith:site/content/docs/guides/deploying.md` | partly works |
| NFR-62 | Caddy block | Must | `deploy/tickitdemo.caddy`; analysis §5 | broken |
| NFR-63 | Drop-ins for monitoring | Must | `deploy/setup-host.sh`; `docs/INSTALL.md:247-261`; `lith:site/content/docs/guides/monitoring.md:168-178` | partly works |
| NFR-64 | Deploy and rollback | Must | `deploy.sh`; `lith:site/content/docs/guides/deploying.md:93-115` | partly works |
| NFR-65 | Migrations | Must | `CLAUDE.md` ("Database & migrations (NOT automatic)"); analysis §1 (`cmd/migrate/main.go:74`) | broken |
| NFR-66 | Shutdown | Must | `lith:skills/lith/SKILL.md` rule 2 | missing |
| NFR-67 | Configuration | Must | `docs/INSTALL.md:143-160`; `config/config.go:45-68` | partly works |
| NFR-70 | Daily copies | Should | `docs/INSTALL.md:277-300`; `lith:site/content/docs/guides/backups.md:9-42` | broken |
| NFR-71 | A copy before a migrating deploy | Must | `lith:site/content/docs/guides/deploying.md:115` | missing |
| NFR-72 | Recovery by reseed | Must | DM-02; analysis decision 4 | missing |
| NFR-75 | Health and readiness | Must | `lith:site/content/docs/guides/monitoring.md:156-165` | missing |
| NFR-76 | Metrics | Must | `docs/INSTALL.md:256` ("tickit serves no metrics"); `docs/event-driven-grouping-requirements.md:119-132`; `lith:skills/lith/SKILL.md` (`prom`) | missing |
| NFR-77 | Logs | Must | `lith:site/content/docs/guides/monitoring.md:147-155` | partly works |
| NFR-78 | Alerts and dashboard | Should | `lith:site/content/docs/guides/monitoring.md` ("The dashboard", "Alerts") | missing |

## Appendix A. Co-editing rich text on lith

MP-05 states the behaviour: up to 20 people in one page, no edit lost, the person's own keystrokes at once, others' edits within 250 ms. Q19 is decided: option 1, operations ordered by lith's writer. A CRDT in the page (Yjs or the like) is ruled out: the server holds, checks and renders the document. Option 3 stays as an early slice.

### A.1 Constraints every option meets

- lith is server-authoritative: the writer orders every command, and views render from SQLite. A person's own keystrokes are their own input, which lith's rule 5 lets the page show at once.
- The editor is an island the stream does not morph (`lith:site/content/docs/guides/pages.md:49`); its content changes through a channel of its own.
- A view's frames replace each other, and a slow stream skips frames (`lith:site/content/docs/guides/streams.md:143`, `newest-frame.js`). Edit operations must never be skipped, so every frame for an editor carries everything since the version its tab last confirmed. A tab view per editor does this with lith as it is; it needs a recipe.
- Every change passes the single writer. Twenty editors sending ten batches a second make 200 commands a second; `BenchmarkExecute` appended 6,675 events a second for one producer on the development machine (`lith:site/content/docs/guides/events.md:378`).
- The server checks what it stores: sizes, the schema and links (DM-05, DM-06).

### A.2 The editor and Datastar

Datastar runs every page; the editor is the one place where the browser holds state of its own. Four rules keep the two apart:

1. **One component owns the document.** The editor is a Rocket component (`sb-rich-editor`) with its own shadow DOM. Datastar never morphs inside it, no `data-bind` touches its document, and no signal holds any part of it. A document in a signal would be overwritten by the next signal patch while the person types, merged into rather than replaced (Datastar never removes a signal's keys), and sent with every request.
2. **Steps go out as an action's payload.** The component emits its unconfirmed steps as an event; the page sends them with `@post` and the steps as the payload, not as signals. The command applies them at the expected version (`events.Exactly`) or refuses.
3. **Confirmed steps come in through an attribute.** The stream sets an attribute of the component to every step since the version the tab last confirmed, so a skipped frame loses nothing (MP-05.9). The component reads it the way Starbase's value components read a server-set value: the server's word wins, and the component rebases its own unconfirmed steps on top.
4. **Signals for what surrounds the editor.** Small `_`-prefixed signals carry the toolbar's state, "Saving" and "Saved", conflict notices and presence. They never hold content.

ProseMirror's `collab` module, the only merge logic in the browser, stays inside the component and rebases only the person's own unconfirmed steps; the order of all steps is the writer's.

### A.3 Option 1: operations as commands, ordered by the writer

The editor is ProseMirror (or Tiptap, which builds on it) with its `prosemirror-collab` module. The page sends its unconfirmed steps with the document version they apply to. A command runs `events.Execute` on the page's stream with `events.Exactly(version)`: at that version, the decider checks the steps, applies them to the server's copy of the document, stores them as one event and moves the version on; at any other version it refuses. The refused page takes the missing steps from its stream, rebases its own steps on them (the collab module does this) and sends again. The writer is the central authority that `prosemirror-collab` expects.

- No keystroke is lost: a refused batch is rebased and resent, never dropped.
- The server holds the real document, so it validates content, renders the page for readers, cuts versions (FR-87), computes diffs (FR-88), feeds search (FR-57), and the version slider (FR-70) reads the page's stream as Town in time reads a tile's.
- Carets map through other people's steps in the browser, with ProseMirror's position mapping (MP-06).
- The server must apply ProseMirror steps in Go for the demo's schema: the step types (replace, replace around, add and remove mark, attribute) and the schema's node rules, ported or run in a JavaScript engine embedded in Go.
- Under heavy contention pages retry more often; each retry costs a round trip. The load of NFR-38 measures it.
- The step log grows with every keystroke batch; versions keep snapshots, and steps older than the newest version can go (`events.Compact`).

### A.4 Option 3: locks per block

A page is a list of blocks (paragraph, heading, list, table, code). A person who starts typing in a block takes a lease on it, a command renewed every 5 s while they type and ending 15 s after the last renewal; others see the block locked with the holder's face. Saving a block is a command with the block's expected version. The page view renders the blocks as HTML; each tab's editor island takes over only the block it holds.

- It uses lith as it is: views, topics, tab state and `events.Exactly`, with no merge code.
- Validation, versions, diffs and search work on the server without new code.
- Two people cannot type in the same block, which breaks MP-05.1 ("in the same paragraph"); a lease stalls up to 15 s when its holder vanishes; splitting and joining blocks (Enter, Backspace at a block's start) need care.

### A.5 The two options side by side

| | Option 1, operations ordered by the writer | Option 3, locks per block |
| --- | --- | --- |
| Two people in one paragraph | yes | no |
| Edits kept after a conflict | yes, rebased and resent | yes, within a lease |
| The server checks content | yes | yes |
| The server renders the page for readers | yes | yes |
| Versions, diffs and search on the server | yes | yes |
| Carets follow other people's edits | yes, in the browser | not needed |
| New server code | ProseMirror steps in Go | leases and block commands |
| Platform work in lith | a recipe for cumulative operation frames | none |

The decision (Q19) is option 1, with option 3 as an early slice that ships the page editor, versions and history before co-editing within one paragraph works.

## Appendix B. The first spike: co-editing one page

What it proves: that Appendix A's option 1 meets MP-05 and Appendix A.2's rules on lith, before the rest of the pages side is built on it.

Scope:

- `sb-rich-editor`, a Rocket component: ProseMirror with `prosemirror-collab`, a schema of paragraphs, headings, lists, bold, italic, links and code, steps out as an event, confirmed steps in through an attribute.
- One page, one stream in `lith/events`: a decider that applies a batch of steps at an expected version, written in Go for the spike's schema, and a projector that keeps the page's current HTML for readers.
- A view per editing tab that carries the steps since the tab's confirmed version (section 6.1), and a page view for readers.
- Versions cut after 60 s without edits (Q18), and the fold of older steps into them (NFR-43).

Done when:

1. Section 12, item 4's co-editing test passes: five scripted editors typing in one paragraph for 60 s end with identical documents holding every typed character, in Chromium and Firefox.
2. MP-05.2 to MP-05.5 and MP-05.8 and MP-05.9 hold, measured at 20 editors typing 5 characters a second on a link with a 50 ms round trip.
3. A test shows that a signal patch, a morph and a reconnect during typing change nothing in the editor but what the confirmed steps bring.
4. The numbers, the steps per second through the writer and the time from a keystroke to the other editors, are written into `benchmarks/RESULTS.md`.

Open for the spike to answer: whether `sb-rich-editor` lives in tickit or in Starbase's component catalog, and whether applying ProseMirror steps in Go or in a JavaScript engine embedded in Go is less work for the schema of section 4.3.

