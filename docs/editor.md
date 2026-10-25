# Co-editing pages and descriptions

How several people edit one page, or one issue's description, at once (MP-05, MP-06, MP-13, MP-14, FR-15, FR-82 to FR-91, NFR-43). The requirements decide option 1 of Appendix A: the writer orders every edit and the server keeps the document; no CRDT. This file records how that is built and the two places where the build differs from Appendix A's wording.

## Decisions

1. **The writer rebases.** A commit names the stream version it was made on (`base`). The writer maps its steps over the commits since `base`, applies what still applies, and appends one event with `events.Exactly(head)`. This is the protocol of `@stepwisehq/prosemirror-collab-commit` (MIT). Appendix A.3 says a commit at another version is refused; a simulation of 20 editors at 5 characters a second over a 50 ms round trip showed why that cannot meet MP-05.3 on lith: with refusals, other editors see a keystroke after 2.2 s at the 95th percentile and the writer refuses 339 posts a second; with rebasing, after 98 ms and with no refusals. The writer still refuses a commit from another epoch (a reset, MP-13) and one made on a version older than the last fold.
2. **A Go core, with ProseMirror as the test oracle.** `internal/doc` ports what the server runs of prosemirror-model and prosemirror-transform for tickit's fixed schema: the model, the steps replace, replaceAround, addMark, removeMark and attr, `StepMap` and `Mapping`, and the rebase. The real prosemirror-transform runs in goja inside `go test` and must agree byte for byte. Running ProseMirror in goja inside the writer would cost 6 to 32 % of the writer for one busy page and stall it 34 to 44 ms when a 200 KB document loads.
3. **ProseMirror in light DOM.** `sb-rich-editor` is a Rocket component whose shadow root holds the toolbar and status; ProseMirror mounts in a light-DOM child the server renders with `data-ignore-morph` and `data-ignore`, so a morph changes only the host's attributes and Datastar never reads what people type. Appendix A.2 rule 1 says the component owns a shadow DOM; prosemirror-view needed Safari selection fixes in shadow roots in four releases from 2023 to 2025, so the document stays out of it. The browser tests in WebKit confirm the mount.
4. **One view per editing tab** whose frame sets the host's `batches` attribute to every commit after the version the tab acknowledged (kept in the tab's row), so a skipped frame loses nothing (MP-05.9).
5. **Fold at version cuts.** A version is cut after 60 s without edits, when the last editor leaves, on "Save version", and at least every 10 minutes of continuous editing; the commits up to a version are folded into a snapshot 15 minutes later (NFR-43).

## Subjects

A co-edited document belongs to a subject, and the subjects share one implementation, `internal/collab`: a `collab.Kind` per subject is a stream kind of its own keyed by the subject's id, with a head table and an editors table of the same shape, and the commit, the reads through the shared caches, the editors' rows, the head projector and the fold. The subject's package keeps what is its own: who may edit, when the subject is gone, how versions are cut and what they record, and the views.

| Subject | Kind | Head, editors | Versions | Views | Routes |
| --- | --- | --- | --- | --- | --- |
| A page's body (`internal/wiki`) | `pagedoc` | `page_doc`, `page_editors` | `page.version_cut` | `editor:<page>.<tab>`, `carets:<page>`, `pagedoc:<page>` | `/cmd/pages/{id}/open`, `/commit`, `/presence` |
| An issue's description (`internal/tracker`, description.go) | `issuedoc` | `issue_doc`, `issue_editors` | `issue.description_changed` | `desc-editor:<issue>.<tab>`, `desc-carets:<issue>`, `issue-desc:<issue>` | `/cmd/issues/{id}/description/open`, `/close`, `/commit`, `/presence` |

The rest of this file describes pages; "Issue descriptions" below says where descriptions differ.

## The schema

`internal/doc/schema.json` is the one source: the browser builds its ProseMirror schema from it (adding `toDOM`, `parseDOM` and node views per name), the oracle builds the same schema in goja, and the Go core reads it. Order matters: the first block type is the default, and mark order is the nesting order. The server's rules beyond the schema (step types, sizes, attribute formats, allowed link schemes, the image library) are in `internal/doc` and listed in its package documentation.

The `limits` object of schema.json holds DM-05's and DM-06's numbers, which the editor's `filterTransaction` and the server's rules both read: `storedBytes` is the UTF-8 length of `JSON.stringify(doc.toJSON())` (no HTML escaping, as `internal/doc` writes it), `blocks` counts every block node but the doc, `tableRows` and `tableColumns` bound a table (columns with colspans), `attrLength` bounds every attribute string, `linkSchemes`, `codeLanguages` and `headingLevels` are allowlists. A commit whose steps all drop in the writer's rebase is still appended, with no steps, because the editor confirms a commit by finding its `ref`.

## HTML of a document

The Go renderer (reader view, comments, descriptions, versions) and the browser's `toDOM` produce the same elements, so one stylesheet (`static/css/prose.css`) styles both. The container is `<div class="prose">`. `doc.RenderHTML` renders the blocks; the caller writes the container. Every text and attribute is escaped.

| Node or mark | HTML |
| --- | --- |
| paragraph | `<p>` |
| heading, level 1 to 3 | `<h2>`, `<h3>`, `<h4>` (the page title is the page's `h1`) |
| blockquote | `<blockquote>` |
| code_block | `<pre data-language="go"><code>…</code></pre>` (`data-language` omitted when empty) |
| horizontal_rule | `<hr>` |
| bullet_list, ordered_list | `<ul>`, `<ol start="3">` (`start` omitted when 1) |
| list_item | `<li>` |
| task_list | `<ul class="task-list">` |
| task_item | `<li class="task-item" data-checked="true">` (or `"false"`) holding `<input type="checkbox" aria-label="Done" disabled checked>` (`checked` only when checked; the editor's node view makes it clickable) then the content in `<div>` |
| table, table_row | `<table><tbody>` with `<tr>` |
| table_cell, table_header | `<td>`, `<th>`, with `colspan` and `rowspan` when not 1 |
| image | `<figure class="doc-image"><img src="/img/lib/<imageId>" alt="…" loading="lazy"></figure>`, for a library image and an uploaded one (FR-85) alike |
| hard_break | `<br>` |
| issue_chip | `<a class="issue-chip" data-key="WEB-12" href="/WEB-12">` holding the type icon `<svg class="tk-type" data-type="bug" viewBox="0 0 16 16" aria-hidden="true">` (two paths, `tk-type__tile` and `tk-type__glyph`), `<span class="issue-chip__key">WEB-12</span>`, `<span class="issue-chip__summary">Fix the login</span>` and `<span class="tk-lozenge issue-chip__status" data-cat="progress">In Review</span>`, from the issue as it is at render time; a deleted or unknown key carries `data-state="deleted"` or `"unknown"` and holds the key and `<span class="issue-chip__summary">deleted issue</span>` (or `unknown issue`) |
| mention | `<a class="mention" data-person="<person id>" href="/people/<person id>">@Sunny Otter</a>`; the name is the person's name now, else the node's `label` |
| issue_list | `<div class="issue-list" data-filter="status=todo" data-saved-filter="" data-limit="10">` holding `<p class="issue-list__desc">Issues matching <code>status=todo</code>, at most 10</p>` (or "Issues of the saved filter <strong>Name</strong>"); the reader view adds the rows after it (`table.issue-list__table` with key, summary, status and assignee, then a link to the issue list), versions and the editor's `toDOM` do not |
| page_link | `<a class="page-link" data-page="<page id>" href="/pages/<page id>">Title</a>`; a removed page renders `<span class="page-link" data-state="gone">a removed page</span>` |
| strong, em, strike, code | `<strong>`, `<em>`, `<s>`, `<code>` |
| link | `<a href="…" rel="nofollow ugc noopener" title="<host>">`; an unsafe href renders as `<span class="link-unsafe">` |

The editor's node view of a removed page keeps its element, an `<a class="page-link" data-state="gone">` without `href`; the reader view writes the `<span>`. A link's `title` is the URL's host with its port, or the address of a `mailto` link. A link mark around an atom renders no `<a>` around it (a chip is a link of its own). The comparison of two versions (FR-88) renders the newer one with added text in `<ins class="diff-ins">` and removed text in `<del class="diff-del">`; a block added or removed as a whole also carries `data-diff="ins"` or `data-diff="del"` on its element.

## Wire format

- **Step**: prosemirror-transform's `Step.toJSON()`. Positions count UTF-16 units, as in the browser.
- **Commit**: `POST /cmd/pages/{page}/commit` (a description's: `POST /cmd/issues/{issue}/description/commit`) with the payload `{tab, epoch, base, ref, steps}`; `ref` is the commit's random id: a commit whose ref and client are among the commits since its base applied already and answers 204 again, so a resent commit applies once without an op id per commit (at 100 commits a second those would hold 1.4 GB a day, NFR-43.2). Answers: 204 applied (or applied before); 409 reset, removed, or `base` older than the fold (the next frame says which); 422 refused by a rule (the refusal is in the tab's row); 429 rate limit.
- **Presence**: `POST /cmd/pages/{page}/presence` with `{tab, epoch, ack, v, anchor, head, seq}`, at most every 100 ms; `seq` makes an overtaken post change nothing.
- **Editor frame**: the tab's view patches the host element:

```html
<sb-rich-editor id="editor" page="…" epoch="…" client="…" base="41" head="57"
  batches='[{"v":56,"ref":"x81k","client":"c_77","steps":[…]}]'
  carets='[{"id":"c_77","name":"Sunny Otter","hue":7,"v":56,"anchor":128,"head":128,"idle":false}]'
  chips='{"WEB-12":{"summary":"Fix the login","status":"In Review","category":"progress","type":"bug"}}'
  state="ok" refusal="" label="Page content" data-cmd="/cmd/pages/…">
  <script type="application/json" data-ignore-morph></script>
  <div class="pm-mount prose" data-ignore-morph data-ignore></div>
</sb-rich-editor>
```

`page` is the subject's id, whatever the subject; `data-cmd` is where the page's wiring posts open, commits and presence (`/cmd/pages/<id>`, or `/cmd/issues/<id>/description`), so one set of expressions serves both.

`batches` holds every commit after the tab's acknowledged version, as the writer applied it, at most 500 a frame. `state` is `ok`, `gap` (the tab's version was folded away), `reset` (another epoch, or the tab's row is gone), `gone` (trashed, hidden or deleted) or `readonly`. `client` is the first 12 hex digits of SHA-256 of the tab id, so tab ids stay private. The page's GET puts the document at version `base` into the script element; later frames leave it empty, and the morph keeps the old one.

The component emits `sb-commit` (the page posts it as the payload, never as signals), `sb-presence`, `sb-ack`, `sb-status` (`saving`, `saved`), `sb-stopped` (with the number of unsaved steps) and `sb-select` (where a selection of text is, for "Create issue"). Toolbar state, status and refusals live in `_`-prefixed signals; no signal ever holds content.

## Events and tables

- `collab`'s three event types, defined once per Kind as `<kind>.created`, `.committed`, `.snapshot`.
- `pagedoc` stream, keyed by page id: `pagedoc.created {epoch, doc}`, `pagedoc.committed {ref, client, base, steps}` (the steps as applied), `pagedoc.snapshot {epoch, doc}` (written by the fold). Its stream version is the collab version.
- `page` stream: title, place, labels, state, and `page.version_cut {number, upto, doc, editors, message, reason}`; never folded.
- `page_doc` (the head document per page, with a checkpoint every K commits), `page_versions`, `page_editors` (the editing tabs' rows: ack, selection, seq), `page_chips` (issue keys per page, for live chips and FR-101).
- `issuedoc` stream, keyed by issue id, with the same three types; `issue_doc` and `issue_editors` (migration 076), and `issues.desc_upto`, the description stream's version that the issue's last `issue.description_changed` holds.
- An in-process LRU of immutable documents keyed by kind, subject, epoch, version and last ref, shared by the writer, the views and the jobs of every Kind.

## How the server side is built

`internal/collab`, used by `internal/wiki` and `internal/web/routes_editor.go`, in the order of this file:

- **Commit.** `wiki.Commit` wraps `collab.Commit` with the page's gate (the page current, the right to edit) and parses the steps in `Validate`, outside the writer (`doc.ParseCommit`); in the writer it reads the page's state (gone: 409) and right (403), the document stream's state row (epoch, version, last ref; another epoch: 409), the commits since `base` from the commit cache or `lith_events` (one folded away: 409; one with the same ref and client: applied already, 204), the head document from the document cache, rebases (`doc.Rebase`), checks (`doc.Rules.Check` with the image library and the uploaded images of FR-85, looked up in the commit's transaction), and appends with `events.Exactly(head)`; a commit whose steps all dropped appends `"steps":[]`. The route runs it as `demo.Guard(commit)`, with no op id.
- **Refusals** go into the editing tab's row (`page_editors.refusal`, `{"ref", "message"}`), not into the tab's outcomes: a commit runs outside `tabs.Act`, since its frame confirms it by ref and ten commits a second should rewrite no tab row. A commit the demo's guard refuses (a blocked guest) answers 403 and pauses the tab's editor: `collab.Kind.Pause` (`wiki.PauseEditor`, `tracker.PauseDescriptionEditor`) writes `{"paused": true, "message"}` there once, and the editor view renders the host `readonly` with the message as plain `refusal`, so the editor stops sending, says why at the editor ("Changes from this browser are paused.") and keeps the text unsaved in the browser (MP-14). Opening the editor again clears it.
- **After a gap** the editor cannot merge (`state="gap"`: the commits after its version were folded and are in no cache), the page shows the person's unsaved text as a copy beside the editor (`#wk-copy`, the editor's `html()`) and keeps it in the tab's `sessionStorage` (`tickit-copy:<page>`), so the page loaded again shows the page as it is with the copy beside it until the person dismisses it (MP-05.6). The fold alone makes no gap while the process's commit cache holds the folded commits: the editor then merges as after a short gap.
- **The editor view** is `editor:<page>.<tab>`, watched with `hub.Watch` by `POST /cmd/pages/{page}/open {tab, op, base, client}` and recorded in the tab's watched views after the action (`wiki.WatchEditor`, since `tabs.Act` saves the row it read before its command), so a reconnect restores it. Its frame renders the host with every attribute; `state` is `reset` when the tab's row is gone (a reset wipes `page_editors`) or the epoch changed, `gone` for a trashed or deleted page, `gap` when the commits after the tab's ack were folded, `readonly` without the right to edit or when the tab is paused.
- **Carets** differ from the frame above: the host's `carets` attribute follows the signal `$_carets` (`data-attr:carets`, kept through the tab's frames by `data-preserve-attr="carets"`), which one shared view per page, `carets:<page>`, patches. Its watchers are the page's editing tabs, so a tab that goes takes its caret with it, and one render serves every editor. The page's presence row marks as "editing" the people whose tabs watch it.
- **`client`** is 12 random hex digits the editor's GET picks (it knows no tab id yet), sent with `open` and every commit; like a hash of the tab id, it keeps tab ids private.
- **The epoch** is `e` and the demo's epoch (`demo_state`), which every reset bumps: the seed stays the same at every run and no edit made before a reset applies after it.
- **page_doc** holds the head's version and ref and the document as of a checkpoint every 50 commits; a reader that misses the cache applies at most 50 commits (or starts at a snapshot). The cache (`DocCacheBytes`, 96 MB) keys documents by page, epoch, version and last ref.
- **Versions** are cut by `wiki.CutDue`, which `web.Server.WikiJobs` sends every 3 s: when no tab watches the page's carets view any more ("left"), after 60 s without a commit ("idle"), or when the first change since the last version is 10 minutes old ("interval"); "Save version" is `wiki.CutVersion`. A version holds the title and the document; its editors are the people who committed or renamed since the version before.
- **The fold** is `wiki.Fold` with lith's `events.Compact` (no SQL of its own): it folds the commits stored before a time, 15 minutes ago or the oldest commit not in a version yet, whichever is earlier, so no commit after its page's last version is folded (`collab_uncut`, migration 087, keeps each document's oldest such commit: a commit inserts it, a version deletes it, so the fold reads its bound from an index; a page in the trash or deleted holds nothing back, since no version is cut for it: `TrashPage` and `DeletePage` delete its row, `RestorePage` puts back the oldest of its events after its last version that the fold left, and migration 089 deleted the rows gone pages and issues kept before); 200 events a command (3.6 ms through the writer for a 116 KB page, benchmarks/RESULTS.md); the job sends up to 20 of them every 10 seconds, until one folds nothing: 24,000 events a minute, against the 7,000 commits a minute of 20 editors at 5 characters a second.

## Issue descriptions

An issue's description is the second subject (FR-15, Q22). It uses the page editor with the same toolbar, chips, mentions, page links, code, tables and task lists, and the same protocol; what differs:

- **Start.** `tracker.CreateIssue` starts the description's stream in the same savepoint as the issue's, with the description it is created with (the create dialog's markup, the seed's document) or a blank one, so both streams are at version 1. An issue from before this existed gets its stream from its description at the first open (`ensureDesc`).
- **In place.** "Edit" on the issue's page sets `$_is.desc` and fetches the host with `GET /issues/{id}/description/editor`, a read that answers a patch of `#issue-desc-editor` with the document at its head and a client id of its own; the host's `data-init` posts open. `POST .../description/open` writes the tab's row through `tabs.Act` and then records both editing views in the tab's row and on its stream (`hub.Watch`); "Done", once nothing waits to be saved, removes the host and posts `close`, which deletes the row and takes both views off. In the issue panel, Edit opens the issue's page with `?edit=description`.
- **Readers** see the description live through `issue-desc:<id>`, which renders the head document into `#issue-desc-body` at every commit; the issue's own view renders only the heading and Edit. Keystrokes in the editor post the typing indicator of MP-07 ("is editing the description").
- **Versions** are cut as the issue's own `issue.description_changed {to, mentions_added, upto, editors}`: by `tracker.CutDescriptions`, which `WikiJobs` sends beside `CutDue`, with the same rules (60 s idle, every editor gone, every 10 minutes), and at once by `tracker.SetDescription`, which replaces the whole description in one commit (`collab.Kind.CommitWhole`) for the seed and tests. So the history (FR-68: "changed the description with Rainy Fox", Compare at `/issues/{id}/description?v=<version>`), the state as of a version (FR-70), the list's and the search index's text, mentions (FR-112: the people a version mentions first, told once) and the activity feeds follow the versions, while the page shows the document live. A version that changed nothing is left out of the history.
- **Rules.** `doc.Rules` and DM-05's limits of a description: 20,000 characters of text, 50 mentions (`checkDesc`); a refusal goes into the tab's row of `issue_editors`.
- **Gone.** A deleted or hidden issue answers a commit 409 and its editors' frames `gone`; a Viewer, or an archived project, `readonly` and 403.
- **Fold.** `tracker.FoldDescriptions` folds the commits of descriptions up to their versions as `wiki.Fold` does pages' (FoldAfter, at most FoldBatch events a command); a deleted or hidden issue's commits hold nothing back (`DeleteIssue` and `HideIssue` delete its row, unhiding puts it back as `RestorePage` does), so the bound is the same index lookup.

## Issue lists and issues from a selection

- **Issue lists** (FR-104) are `issue_list` atom blocks: `filter` (the issue list's query string, at most 500 characters), `savedFilter` (a saved filter's id) and `limit` (1 to 25). The form in the editor's node view changes them with attr steps, so co-editors converge as for any other change. The host's `lists` attribute names the page's hidden `#wk-lists`, which the view `pagelists:<page>` patches with each list's rows (`[data-list]`, keyed `savedFilter|filter|limit` as `doc.IssueList.Key` writes it) and the saved filters (`[data-filters] > [data-id]`); the node views copy the rows, so the preview is live. A description's editor has no `lists` and shows no rows. The reader view renders the rows in place (docs/architecture.md, "The wiki").
- **Create issue** (FR-105): in the editor, `track(id)` keeps the selection under an id, mapped through every transaction, and `chipTracked(id, key)` replaces it with the chip in a commit of its own once `POST /cmd/pages/issue` answered the key; a selection someone deleted meanwhile gets the chip where it was. In view mode the page sends the selected text, its top-level block and which occurrence in that block it is, and the server finds it (`doc.FindInBlock`, within one paragraph, white space folded) and commits the chip on the head (`wiki.ChipSelection`, `collab.CommitWhole`).

## Inline comments

A person selects text of a page, in view or in the editor, and comments on it (FR-91): the text shows highlighted for every reader and editor, the comment sits beside it, the highlight follows the text as people edit, a comment whose text was deleted lists as detached, and resolving hides the highlight.

**Decision: anchors outside the document, mapped on the server through each commit's steps; no `comment` mark in the schema.** A mark would move with its text through the rebase for free and survive versions, but:

- Comments are not content. Everyone who reads a space may comment, Viewers too (3.2), while only Members edit; a mark would make every comment, resolve and reopen a commit by someone who may not edit, credited as an editor of the next version, and cut versions that change nothing a reader sees.
- Versions, compare and restore would carry the marks: a restore would bring back highlights of old comments, and compare would show marks added and removed as changes.
- A mark is copied with its text (a paste duplicates a comment's highlight), overlapping comments need marks that do not exclude each other (`excludes`, which the Go port and the oracle tests would have to cover), and the schema change would meet the other schema changes in flight (FR-104) in `schema.json`, the editor and the oracle.

What anchors cost instead, and how it is paid:

- **Convergence.** `page_anchors` holds each comment's range in the head document. Every commit maps the page's anchors through its applied steps in the commit's own command (`collab.Config.OnCommit`, `wiki.mapAnchors`, `doc.MapRange` with the Go `StepMap`, which the oracle tests check against prosemirror-transform), so the anchors are always at `page_doc`'s version and every reader of a version gets the same positions. The start maps with assoc 1 and the end with assoc -1: text typed at either edge stays outside; typing over the whole range keeps the comment on what replaced it. A range that collapses or holds no text any more is detached for good; undoing the delete does not bring it back.
- **Editors.** Each editor's frame carries the host's `comments` attribute, `{v, list: [{id, from, to}]}` at the head the frame's `batches` reach, so the editor applies the batches and then draws the ranges from the same version; it maps them through the confirmed commits after `v` (the step maps the carets keep) and its unconfirmed steps, and between frames the decorations map through every transaction (`editor/src/comments.js`). The decorations are inclusive at their end in the editor alone, so that Chromium keeps the caret after text typed at a highlight's end; the next frame draws the server's range again.
- **Rebuilds.** The fold removes the commits a rebuild would map through, so `page_anchors` is state the commands write, as `page_titles` is, not a projection. The comment's stream holds where it was made (`inline.anchored {page, v, from, to, quote}`), and the projection `page_inline` keeps that and the resolution.
- **Creating one.** `POST /cmd/pages/comment/inline {tab, op, page, epoch, base, from, to, text, body}`: the range at version `base` of the epoch, and the characters of its text nodes without separators (`text`). The command maps the range over the commits since `base` (`collab.Kind.StepsSince`), and refuses with "select it again" when the epoch changed, the commits were folded away, nothing of the text is left, or the text there is not `text` any more. It answers the new comment's id as `$_icMade`. In the editor the range is the selection mapped back through the unconfirmed steps (`commentRange()`); in view it comes from the reader's HTML, whose elements are the document's nodes (`internal/web/static/wiki/inline.js`, which counts as `internal/doc` sizes nodes), at the `data-v` and `data-epoch` of `#page-body`. A miscount there ends as a refusal, never as a wrong anchor.

The rest is a page comment's: an inline comment is a comment with the subject `page` (its body, replies one level deep, edit, delete, hide by a moderator, search, the watchers' notifications, the feeds) plus its stream `inline`, keyed by the comment's id, with `inline.anchored`, `.resolved` and `.reopened` (`ResolveInline`, `POST /cmd/pages/comment/resolve {comment, resolved}`, anyone who may comment). The page's comments below it leave inline ones out.

- **Readers.** The reader view renders the head with `doc.RenderHighlighted`: the text of each open comment (not resolved, hidden, deleted or detached) in `<mark class="tk-ic" data-c="<ids>">`, atoms left alone; the block cache renders a block again when the highlights inside it change. The editor's decorations make the same element.
- **The list.** The shared view `inline:<page>` (topics `inline:<page>`, which every inline comment, every comment on the page and every detached anchor touches, and `people`) renders `#page-inline`: the comments on text in the order of the text, then the detached ones, then the resolved ones folded away. It sits beside the text from a page 64rem wide, and below it on a narrower one.
- **Versions and compare** render their documents as before, without highlights: anchors belong to the head, so a resolved or detached comment never reaches an old version's render.
