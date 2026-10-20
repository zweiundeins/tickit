# Co-editing pages

How several people edit one page at once (MP-05, MP-06, MP-13, MP-14, FR-82 to FR-91, NFR-43). The requirements decide option 1 of Appendix A: the writer orders every edit and the server keeps the document; no CRDT. This file records how that is built and the two places where the build differs from Appendix A's wording.

## Decisions

1. **The writer rebases.** A commit names the stream version it was made on (`base`). The writer maps its steps over the commits since `base`, applies what still applies, and appends one event with `events.Exactly(head)`. This is the protocol of `@stepwisehq/prosemirror-collab-commit` (MIT). Appendix A.3 says a commit at another version is refused; a simulation of 20 editors at 5 characters a second over a 50 ms round trip showed why that cannot meet MP-05.3 on lith: with refusals, other editors see a keystroke after 2.2 s at the 95th percentile and the writer refuses 339 posts a second; with rebasing, after 98 ms and with no refusals. The writer still refuses a commit from another epoch (a reset, MP-13) and one made on a version older than the last fold.
2. **A Go core, with ProseMirror as the test oracle.** `internal/doc` ports what the server runs of prosemirror-model and prosemirror-transform for tickit's fixed schema: the model, the steps replace, replaceAround, addMark, removeMark and attr, `StepMap` and `Mapping`, and the rebase. The real prosemirror-transform runs in goja inside `go test` and must agree byte for byte. Running ProseMirror in goja inside the writer would cost 6 to 32 % of the writer for one busy page and stall it 34 to 44 ms when a 200 KB document loads.
3. **ProseMirror in light DOM.** `sb-rich-editor` is a Rocket component whose shadow root holds the toolbar and status; ProseMirror mounts in a light-DOM child the server renders with `data-ignore-morph` and `data-ignore`, so a morph changes only the host's attributes and Datastar never reads what people type. Appendix A.2 rule 1 says the component owns a shadow DOM; prosemirror-view needed Safari selection fixes in shadow roots in four releases from 2023 to 2025, so the document stays out of it. The browser tests in WebKit confirm the mount.
4. **One view per editing tab** whose frame sets the host's `batches` attribute to every commit after the version the tab acknowledged (kept in the tab's row), so a skipped frame loses nothing (MP-05.9).
5. **Fold at version cuts.** A version is cut after 60 s without edits, when the last editor leaves, on "Save version", and at least every 10 minutes of continuous editing; the commits up to a version are folded into a snapshot 15 minutes later (NFR-43).

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
| image | `<figure class="doc-image"><img src="/img/lib/<imageId>" alt="…" loading="lazy"></figure>` |
| hard_break | `<br>` |
| issue_chip | `<a class="issue-chip" data-key="WEB-12" href="/WEB-12">` holding the type icon `<svg class="tk-type" data-type="bug" viewBox="0 0 16 16" aria-hidden="true">` (two paths, `tk-type__tile` and `tk-type__glyph`), `<span class="issue-chip__key">WEB-12</span>`, `<span class="issue-chip__summary">Fix the login</span>` and `<span class="tk-lozenge issue-chip__status" data-cat="progress">In Review</span>`, from the issue as it is at render time; a deleted or unknown key carries `data-state="deleted"` or `"unknown"` and holds the key and `<span class="issue-chip__summary">deleted issue</span>` (or `unknown issue`) |
| mention | `<a class="mention" data-person="<person id>" href="/people/<person id>">@Sunny Otter</a>`; the name is the person's name now, else the node's `label` |
| page_link | `<a class="page-link" data-page="<page id>" href="/pages/<page id>">Title</a>`; a removed page renders `<span class="page-link" data-state="gone">a removed page</span>` |
| strong, em, strike, code | `<strong>`, `<em>`, `<s>`, `<code>` |
| link | `<a href="…" rel="nofollow ugc noopener" title="<host>">`; an unsafe href renders as `<span class="link-unsafe">` |

The editor's node view of a removed page keeps its element, an `<a class="page-link" data-state="gone">` without `href`; the reader view writes the `<span>`. A link's `title` is the URL's host with its port, or the address of a `mailto` link. A link mark around an atom renders no `<a>` around it (a chip is a link of its own). The comparison of two versions (FR-88) renders the newer one with added text in `<ins class="diff-ins">` and removed text in `<del class="diff-del">`; a block added or removed as a whole also carries `data-diff="ins"` or `data-diff="del"` on its element.

## Wire format

- **Step**: prosemirror-transform's `Step.toJSON()`. Positions count UTF-16 units, as in the browser.
- **Commit**: `POST /cmd/pages/{page}/commit` with the payload `{tab, epoch, base, ref, steps}`; `ref` is the commit's random id: a commit whose ref and client are among the commits since its base applied already and answers 204 again, so a resent commit applies once without an op id per commit (at 100 commits a second those would hold 1.4 GB a day, NFR-43.2). Answers: 204 applied (or applied before); 409 reset, removed, or `base` older than the fold (the next frame says which); 422 refused by a rule (the refusal is in the tab's row); 429 rate limit.
- **Presence**: `POST /cmd/pages/{page}/presence` with `{tab, epoch, ack, v, anchor, head, seq}`, at most every 100 ms; `seq` makes an overtaken post change nothing.
- **Editor frame**: the tab's view patches the host element:

```html
<sb-rich-editor id="editor" page="…" epoch="…" client="…" base="41" head="57"
  batches='[{"v":56,"ref":"x81k","client":"c_77","steps":[…]}]'
  carets='[{"id":"c_77","name":"Sunny Otter","hue":7,"v":56,"anchor":128,"head":128,"idle":false}]'
  chips='{"WEB-12":{"summary":"Fix the login","status":"In Review","category":"progress","type":"bug"}}'
  state="ok" refusal="">
  <script type="application/json" data-ignore-morph></script>
  <div class="pm-mount prose" data-ignore-morph data-ignore></div>
</sb-rich-editor>
```

`batches` holds every commit after the tab's acknowledged version, as the writer applied it, at most 500 a frame. `state` is `ok`, `gap` (the tab's version was folded away), `reset` (another epoch, or the tab's row is gone), `gone` (trashed, hidden or deleted) or `readonly`. `client` is the first 12 hex digits of SHA-256 of the tab id, so tab ids stay private. The page's GET puts the document at version `base` into the script element; later frames leave it empty, and the morph keeps the old one.

The component emits `sb-commit` (the page posts it as the payload, never as signals), `sb-presence`, `sb-ack`, `sb-status` (`saving`, `saved`) and `sb-stopped` (with the number of unsaved steps). Toolbar state, status and refusals live in `_`-prefixed signals; no signal ever holds content.

## Events and tables

- `pagedoc` stream, keyed by page id: `pagedoc.created {epoch, doc}`, `pagedoc.committed {ref, client, base, steps}` (the steps as applied), `pagedoc.snapshot {epoch, doc}` (written by the fold). Its stream version is the collab version.
- `page` stream: title, place, labels, state, and `page.version_cut {number, upto, doc, editors, message, reason}`; never folded.
- `page_doc` (the head document per page, with a checkpoint every K commits), `page_versions`, `page_editors` (the editing tabs' rows: ack, selection, seq), `page_chips` (issue keys per page, for live chips and FR-101).
- An in-process LRU of immutable documents keyed by page, version and last ref, shared by the writer, the views and the jobs.

## How the server side is built

`internal/wiki` and `internal/web/routes_editor.go`, in the order of this file:

- **Commit.** `wiki.Commit` parses the steps in `Validate`, outside the writer (`doc.ParseCommit`); in the writer it reads the page's state (gone: 409) and right (403), the document stream's state row (epoch, version, last ref; another epoch: 409), the commits since `base` from the commit cache or `lith_events` (one folded away: 409; one with the same ref and client: applied already, 204), the head document from the document cache, rebases (`doc.Rebase`), checks (`doc.Rules.Check` with the image library), and appends with `events.Exactly(head)`; a commit whose steps all dropped appends `"steps":[]`. The route runs it as `demo.Guard(commit)`, with no op id.
- **Refusals** go into the editing tab's row (`page_editors.refusal`, `{"ref", "message"}`), not into the tab's outcomes: a commit runs outside `tabs.Act`, since its frame confirms it by ref and ten commits a second should rewrite no tab row. A commit the demo's guard refuses (a blocked guest) answers 403 and pauses the tab's editor: `wiki.PauseEditor` writes `{"paused": true, "message"}` there once, and the editor view renders the host `readonly` with the message as plain `refusal`, so the editor stops sending, says why at the editor ("Changes from this browser are paused.") and keeps the text unsaved in the browser (MP-14). Opening the editor again clears it.
- **After a gap** the editor cannot merge (`state="gap"`: the commits after its version were folded and are in no cache), the page shows the person's unsaved text as a copy beside the editor (`#wk-copy`, the editor's `html()`) and keeps it in the tab's `sessionStorage` (`tickit-copy:<page>`), so the page loaded again shows the page as it is with the copy beside it until the person dismisses it (MP-05.6). The fold alone makes no gap while the process's commit cache holds the folded commits: the editor then merges as after a short gap.
- **The editor view** is `editor:<page>.<tab>`, watched with `hub.Watch` by `POST /cmd/pages/{page}/open {tab, op, base, client}` and recorded in the tab's watched views after the action (`wiki.WatchEditor`, since `tabs.Act` saves the row it read before its command), so a reconnect restores it. Its frame renders the host with every attribute; `state` is `reset` when the tab's row is gone (a reset wipes `page_editors`) or the epoch changed, `gone` for a trashed or deleted page, `gap` when the commits after the tab's ack were folded, `readonly` without the right to edit or when the tab is paused.
- **Carets** differ from the frame above: the host's `carets` attribute follows the signal `$_carets` (`data-attr:carets`, kept through the tab's frames by `data-preserve-attr="carets"`), which one shared view per page, `carets:<page>`, patches. Its watchers are the page's editing tabs, so a tab that goes takes its caret with it, and one render serves every editor. The page's presence row marks as "editing" the people whose tabs watch it.
- **`client`** is 12 random hex digits the editor's GET picks (it knows no tab id yet), sent with `open` and every commit; like a hash of the tab id, it keeps tab ids private.
- **The epoch** is `e` and the demo's epoch (`demo_state`), which every reset bumps: the seed stays the same at every run and no edit made before a reset applies after it.
- **page_doc** holds the head's version and ref and the document as of a checkpoint every 50 commits; a reader that misses the cache applies at most 50 commits (or starts at a snapshot). The cache (`DocCacheBytes`, 96 MB) keys documents by page, epoch, version and last ref.
- **Versions** are cut by `wiki.CutDue`, which `web.Server.WikiJobs` sends every 3 s: when no tab watches the page's carets view any more ("left"), after 60 s without a commit ("idle"), or when the first change since the last version is 10 minutes old ("interval"); "Save version" is `wiki.CutVersion`. A version holds the title and the document; its editors are the people who committed or renamed since the version before.
- **The fold** is `wiki.Fold` with lith's `events.Compact` (no SQL of its own): it folds the commits stored before a time, 15 minutes ago or the oldest commit not in a version yet, whichever is earlier, so no commit after its page's last version is folded; 200 events a command (3.6 ms through the writer for a 116 KB page, benchmarks/RESULTS.md); the job sends up to 20 of them every 10 seconds, until one folds nothing: 24,000 events a minute, against the 7,000 commits a minute of 20 editors at 5 characters a second.
