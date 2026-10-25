# sb-rich-editor

The browser side of co-editing pages (docs/editor.md): a Rocket component that wraps ProseMirror and speaks the commit protocol, where the writer rebases a stale commit over the commits since its base. The protocol, the remote carets and the comment highlights are lith-coedit's shared modules (github.com/zweiundeins/lith-coedit, `editor/`, its README); this folder holds tickit's component, node views, toolbar and notices on them, the build and the browser tests; the built files live in `internal/web/static/editor/` and are committed, so the Go build needs no Node.

## Files

| Path | What it is |
| --- | --- |
| `src/sb-rich-editor.js` | The Rocket component: props, the shadow root (toolbar, dialogs, status line), host methods. Loads the core on first use. |
| `src/core.js` | The editor: ProseMirror view on lith-coedit's `Session` (frames in, commits and presence out, stopped states), tickit's attributes, toolbar state, notices, dialogs, paste. |
| `src/schema.js` | The schema from `internal/docnodes/schema.json` (bundled at build time) plus `toDOM` and `parseDOM` per node and mark. |
| `src/nodeviews.js` | Node views: issue chips, mentions, page links, task items, images, issue lists with their filter form. |
| `src/commands.js` | Toolbar commands, input rules, keymaps. |
| `src/limits.js` | DM-05's limits as a `filterTransaction`. |
| `src/uploads.js` | Image uploads (FR-85): the placeholder while a file travels, the multipart post. |
| `src/styles.js` | Structural CSS for the light-DOM editor. |
| `build.mjs` | The esbuild build; prints the sizes. |
| `lith-coedit.mjs` | Where lith-coedit's modules are (`go list -m` of the module go.mod names), the alias `lith-coedit` the builds use, and ProseMirror resolved from this folder's `node_modules`. |
| `test/authority/worker.js` | The fake writer in a SharedWorker for the browser tests: lith-coedit's `test/authority/authority.js` with tickit's seed and chips. |
| `test/harness/`, `test/server.mjs`, `test/run.mjs` | The browser tests. |

## Markup the server renders

```html
<sb-rich-editor id="editor" page="p_…" epoch="e_…" client="c_31a9…" base="41" head="57"
  batches='[{"v":56,"ref":"x81k","client":"c_77","steps":[…]}]'
  carets='[{"id":"c_77","name":"Sunny Otter","hue":7,"v":56,"anchor":128,"head":128,"idle":false}]'
  chips='{"WEB-12":{"summary":"Fix the login","status":"In Review","category":"progress","type":"bug"}}'
  state="ok" refusal=""
  data-on:sb-commit="…" data-on:sb-presence="…" data-on:sb-status="…" data-on:sb-stopped="…">
  <script type="application/json" data-ignore-morph>{…document at version base…}</script>
  <div class="pm-mount prose" data-ignore-morph data-ignore></div>
</sb-rich-editor>
```

Every frame renders every attribute, because Datastar's morph removes attributes the new element lacks. Later frames leave the script empty; with `data-ignore-morph` on both sides the morph keeps the old one, and the same holds for `.pm-mount`, where ProseMirror edits. The JSON in the script must not contain `</script`: Go's `encoding/json` escapes `<` as `<`, which is enough. The server may render the reader HTML inside `.pm-mount` for the first paint; ProseMirror replaces it when it mounts.

The page loads `/static/editor/sb-rich-editor.min.<hash>.js` (from `static.URL("editor/sb-rich-editor.min.js")`) as a module after the import map that maps `datastar` to the vendored build. The core comes by dynamic import (see "Build" for why that works with lith's hashed names).

## Attributes

| Attribute | Value |
| --- | --- |
| `page` | Page id. It goes into every event. |
| `epoch` | The epoch of the page's stream. A value other than the one the editor started with stops it as `reset`. |
| `client` | This tab as others see it (12 hex digits of SHA-256 of the tab id). Its own entry in `carets` is left out. |
| `tab` | Optional. When set, `sb-commit` and `sb-presence` carry it, and uploads need it; otherwise the page adds the tab to the payload. tickit sets it from `$_tab` (`data-attr:tab`, kept through frames by `data-preserve-attr`). |
| `base` | Stream version of the document in the script. Read once, at mount. |
| `head` | Stream version of the newest commit. |
| `batches` | JSON `[{v, ref, client, steps}]`: every commit after the version the tab acknowledged, as the writer applied it, at most 500. The editor applies them in version order and waits at a gap for the next frame. |
| `carets` | JSON `[{id, name, hue, v, anchor, head, idle}]`. `hue` is the face's index, 0 to 15. Positions are at version `v`. |
| `comments` | JSON `{v, list: [{id, from, to}]}`: the ranges of a page's inline comments at version `v` (FR-91), drawn as `<mark class="tk-ic" data-c="<id>">` and mapped like carets. |
| `chips` | JSON. Issue keys map to `{summary, status, category, type}`, or to `{state: "deleted"}` or `{state: "unknown"}`. Optional maps `"people": {id: {name}}` and `"pages": {id: {title}}` or `{state: "gone"}` name mentions and page links. Issue keys cannot collide with those two names. |
| `state` | `ok`, `gap`, `reset`, `gone` or `readonly`. |
| `refusal` | JSON `{"ref": "…", "message": "…"}` for a commit the writer refused (the editor takes its steps out and shows the message), or plain text to show. Each ref is handled once, so the row may keep it. |
| `readonly` | Boolean attribute: present means read-only. `readonly="false"` is read-only too (Rocket reads booleans as HTML does). |
| `images` | JSON `[{id, alt}]`: the image library the Image button offers. Without it and without `upload`, the button fires `sb-insert-image`. |
| `upload` | Optional. The URL image files go to (FR-85): a file pasted, dropped or chosen with the Image dialog's "Upload an image" is posted there as `multipart/form-data` with `tab`, `op` and `file`, and the header `Datastar-Request: true`. Needs `tab`. A placeholder shows the progress where the image goes; the answer `{id, width, height}` puts the image there, any other answer's text shows as a refusal. |
| `label` | Accessible name of the text area (default "Page content"). |
| `lists` | Optional: the id of the page's element whose `[data-list]` children hold each issue list's rows by `savedFilter|filter|limit` and whose `[data-filters] > [data-id]` the saved filters (FR-104). Without it an issue list shows no rows. |

## Events

All bubble and are composed. The detail never goes into a signal; commits and presence go out as `@post` payloads.

| Event | Detail | When |
| --- | --- | --- |
| `sb-commit` | `{page, tab?, epoch, base, ref, steps}` | One commit at a time, at most 20 steps. The same detail, same `ref`, goes out again when a post fails, and when an answered commit has not shown up in a frame after 3 s. |
| `sb-presence` | `{page, tab?, epoch, ack, v, anchor, head, seq}` | At most every 100 ms, after the selection or the document changed and after a frame was applied. `ack` and `v` are the confirmed version; `anchor` and `head` are mapped back through the unconfirmed steps to that version. `seq` grows by one per event. |
| `sb-ack` | `{page, epoch, ack}` | After a frame's commits were applied. |
| `sb-status` | `{page, status, unsaved}` | `saving` while steps are unconfirmed, `saved` when none are left; only on change. |
| `sb-stopped` | `{page, state, unsaved}` | The editor stopped sending: `reset`, `gone`, `gap`, `readonly`, or `diverged` (a commit from the server did not parse or apply). |
| `sb-refused` | `{page, ref, message, dropped}` | A refusal arrived; `dropped` counts the steps taken out. |
| `sb-limit` | `{page, limit, max, value, message}` | Input stopped at a DM-05 limit: `size`, `blocks`, `table-rows`, `table-columns`. |
| `sb-paste-offer` | `{page, kind}` | A pasted issue key or tickit URL got an offer (`chip` or `page`). |
| `sb-insert-image` | `{page}` | Image button without `images` and `upload`. Cancel it (`preventDefault`) and call `insertImage`. |
| `sb-uploaded` | `{page, id, width, height}` | An uploaded image went into the document. While uploads travel, `sb-status` says `saving` and counts them in `unsaved`. |
| `sb-select` | `{page, empty, text, top, left, bottom}` | The selection of text changed, at most once a frame: its text and where it is in viewport pixels, for "Create issue" (FR-105). |
| `sb-ready` | `{page}` | The editor mounted. |
| `sb-copied` | `{page, ok}` | The copy button ran. |

## Methods

| Method | Does |
| --- | --- |
| `outcomes(map)` | The page hands it `$_outcomes` at every change: the editor shows the words of its own failed ops, a commit's ref (a rate limit, a failed write: the server's notice) and an upload's op (its refusal). No words come from a status. |
| `posted(ref, ok, status?)` | The page calls it when the `@post` of commit `ref` settled. `ok` false means a network failure: the commit goes out again after 1 s, then 2, 4, 8 and 15 s. `ok` true means the server answered; the frame confirms. With a `status`: 429 and 5xx back off and resend, 409 and 422 wait for the frame that says why. |
| `copy()` | Puts the content on the clipboard as HTML and text (MP-13). |
| `insertImage(id, alt)`, `insertChip(key)`, `insertMention(personId, label)`, `insertPageLink(pageId)` | Insert at the selection. |
| `insertIssueList(attrs)` | Inserts an issue list at the selection and puts the focus in its filter field. |
| `selectionText()`, `track(id)`, `chipTracked(id, key)` | The selected text as a summary; keep the selection under id, mapped through every change; replace it with key's chip as a commit (FR-105). |
| `focus()`, `html()`, `text()`, `json()` | Focus, and the current content. |
| `commentRange()` | The selection as an inline comment's range `{v, epoch, from, to, text}`: at the confirmed version, mapped back through the unconfirmed steps, with the characters of its text nodes; `null` without selected text. |
| `view`, `editor` | The ProseMirror view and the editor object, for tests. |

## Wiring the page

```html
data-on:sb-commit="(el.__status = 0, @post(el.dataset.cmd + '/commit',
    {headers: {'Tab-Id': $_tab, 'Op-Id': evt.detail.ref}, payload: {...evt.detail, tab: $_tab}, requestCancellation: 'disabled', retry: 'never', retryMaxCount: 0})
  .then(() => el.posted(evt.detail.ref, true, el.__status || undefined), () => el.posted(evt.detail.ref, false)))"
data-on:datastar-fetch="evt.detail.el === el && evt.detail.type === 'error' && (el.__status = +evt.detail.argsRaw.status)"
data-effect="el.outcomes && el.outcomes($_outcomes)"
data-on:sb-presence="@post(el.dataset.cmd + '/presence',
    {payload: {...evt.detail, tab: $_tab}, retry: 'never', retryMaxCount: 0}).catch(() => {})"
data-on:sb-status="$_editor.status = evt.detail.status"
data-on:sb-stopped="$_editor.stopped = evt.detail.state"
```

- `data-cmd` on the host is where the subject's actions are: `/cmd/pages/<id>` for a page, `/cmd/issues/<id>/description` for an issue's description (docs/editor.md, "Subjects").
- `requestCancellation: 'disabled'` keeps a commit from being aborted by the next post to the same URL. Presence may keep the default: a newer position replaces an older one anyway.
- `retryMaxCount: 0` makes a network failure reject at once, so the component decides when to resend. Datastar retries network errors up to `retryMaxCount` even with `retry: 'never'`; `retry` governs only HTTP statuses.
- The `{...evt.detail}` spread is fine in a Datastar expression; `el` is the host.

### What Datastar's @post returns

Measured in the browser tests with the vendored Datastar 1.0.4 build (`test/harness/post.html`):

| Answer | Promise |
| --- | --- |
| 204, 409, 422, 429, 500 | resolves, with `undefined` |
| connection closed without an answer | rejects with Datastar's `FetchFailed` error |
| aborted because a newer post to the same URL started (`requestCancellation: 'auto'`) | resolves |

So `.then` means "the server answered", not "applied". The status reaches the page only through Datastar's `datastar-fetch` event (type `error`, `argsRaw.status`), which does not say which request it belongs to; the component therefore treats every answer the same way and lets the frame confirm or refuse. A page that knows the status can pass it as the third argument of `posted`. Chromium itself sent the POST three times when the connection closed without an answer, even with `retryMaxCount: 0`: the commit route has to be idempotent by ref, which `writer.Once("commit:" + page + ":" + ref)` makes it.

## What the Go side has to match

- **Confirming a commit.** The writer appends a commit even when rebasing dropped some or all of its steps, with the steps as applied, so the editor finds its `ref` in `batches`. The editor compares those steps one by one (after a round through ProseMirror's JSON) with its own rebased copies of the commit's steps. Equal, the usual case, they only confirm. Different, the editor undoes its unconfirmed steps, applies the writer's and rebases the rest on top, so its confirmed document always equals the writer's; it logs a warning and counts `editor.stats.resyncs`. Undo loses the history of those steps in that case. In the Node tests, 4,200 own commits over random edits needed no resync, including commits where the writer dropped steps.
- **The rebase** is lith-coedit's `editor/test/authority/rebase.js` (collab-commit's `applyCommit`): map each step through the inverted earlier steps of the commit, the commits since `base` and the mapped earlier steps, with mirrors. The Go core must drop exactly the steps it drops; a test in lith-coedit checks the file against prosemirror-go's oracle.
- **Refusals** name the refused commit's ref in `refusal` as JSON `{"ref", "message"}`; otherwise the editor cannot take that commit's steps out and the next commit would build on steps the writer never applied.
- **`readonly` state** keeps applying `batches` but sends nothing, and keeps the person's unsaved text in the browser (MP-14.2).
- **Limits** in `src/limits.js`: size is the UTF-8 length of `JSON.stringify(doc.toJSON())`; blocks counts every block node except the doc; tables count `TableMap` width and height. `limits.textLength` in schema.json is not enforced: the docs do not say what it measures.
- **`data-checked`** on a task item is always rendered, `"true"` or `"false"`.

## Build

```sh
cd editor
npm ci
node build.mjs
```

`lith-coedit.mjs` asks `go list -m` where the lith-coedit module is (with go.mod's `replace`, the checkout beside this one), so Go must be on the path. It writes three files to `internal/web/static/editor/`:

- `sb-rich-editor.min.js`, the component;
- `pm-core.min.js`, ProseMirror, lith-coedit's modules and the editor;
- `LICENSES.txt`, the licences of the bundled packages.

lith's `edge.Static` serves a file as `name.<first 12 hex digits of SHA-256>.ext` with an immutable cache, and serves `*.min.js` as written. The build computes that name for the core and puts it into the component's dynamic import, so `import('./pm-core.min.<hash>.js')` resolves, relative to the component's own hashed URL, to the core's immutable URL. A test against lith v0.3.0's `edge.Static` confirmed the name and the `immutable` header. With another `StaticHashLen`, lith still finds the file by its plain name, with `no-cache`. Splitting keeps the component small and lets the toolbar render while the core loads; any change of the core changes the component's bytes and so its URL.

Sizes of the current build (brotli at quality 11, gzip at level 9):

| File | Minified | gzip | brotli |
| --- | --- | --- | --- |
| `sb-rich-editor.min.js` | 15.1 KB | 5.4 KB | 4.7 KB |
| `pm-core.min.js` | 302.5 KB | 94.8 KB | 82.2 KB |

Pinned versions are in `package.json`: prosemirror-model 1.25.12, -transform 1.12.2, -view 1.42.6, -state 1.4.4, -tables 1.8.5, -history 1.5.1, -commands 1.7.2, -keymap 1.2.3, -inputrules 1.5.1, -schema-list 1.5.1, -gapcursor 1.4.1, -dropcursor 1.8.4. collab-commit is a dev dependency for its licence text: lith-coedit's `editor/src/collab.js` holds the adapted code, because the package's `main` names a missing `dist/index.js` and the editor needs per-step tags its state does not carry. lith-coedit's modules import prosemirror-model, -state, -transform and -view, which the build takes from this folder, so the bundle holds one copy of each; keep the versions equal to lith-coedit's `editor/package.json`.

## Tests

```sh
node test/run.mjs [filter]           # the browser tests, about 90 s with CONVERGE_SECONDS=40
CONVERGE_SECONDS=60 EDITORS=20 node test/run.mjs converge
```

The browser tests need Chromium (`CHROME=/usr/bin/chromium` is the default; without it they skip). Each editor is a page of its own, with its own Datastar and component, as each person has a tab of their own. The fake writer runs in a SharedWorker: it is lith-coedit's fake writer: it orders commits, rebases them with `rebase.js`, keeps versions, and every 50 ms renders each changed tab's frame of commits after its ack. Datastar's `@post` goes through a `fetch` shim to the worker, 25 ms each way; frames come back after 25 ms and are applied as Datastar applies a stream's `datastar-patch-elements`, as a morph of the host. A slow stream is simulated as lith's newest-frame.js behaves: a held-back frame that a newer one overtakes is never applied, and the newest always is. The pages are served with lith's DefaultCSP; one test drops `'unsafe-eval'` and runs Datastar in nonce mode. Results go to `test/out/results.json`.

The tests use no Chromium-only API except the launcher; puppeteer-core drives Firefox over WebDriver BiDi (`BROWSER=firefox CHROME=/path/to/firefox`), untried here.

Results on 2026-10-20 (Intel i5-13500, headless Chromium 154, 25 ms each way, a 50 ms tick, 5 characters a second per editor, keystroke to the other editors' screens):

| Run | Keystrokes | p50 | p95 | max | Settled after the last keystroke |
| --- | --- | --- | --- | --- | --- |
| 5 editors, 60 s | 1,478 | 98 ms | 144 ms | 202 ms | 9 ms |
| 20 editors, 30 s | 2,852 | 111 ms | 173 ms | 299 ms | 9 ms |
| 3 editors, 10 s, 60 % of frames held back 100 to 300 ms | 150 | 197 ms | 370 ms | 498 ms | 4 ms |

Every run ended with identical documents in every tab and in the writer, holding every typed character as often as it was typed, with no resend and no resync.

The other browser tests cover: the HTML that `toDOM` writes against docs/editor.md's table; a reconnect with unconfirmed steps (the same ref goes out again and applies once); a reset (the editor stops sending, keeps its text and copies it); frames, a morph of the region around the editor and a signal patch, which leave content, caret, focus and the ProseMirror view untouched (Appendix B, done 3); undo of own changes only; input rules and shortcuts; paste of hostile HTML, and the chip and page-link offers; the 200 KB and 50-row limits; the toolbar's names, `aria-pressed`, single tab stop and keyboard path; remote carets and selections; the link dialog's scheme check; the task checkbox; a refused commit; the `readonly` and `gone` states; and what `@post` returns.

## Keyboard

Mod is Ctrl, or Cmd on a Mac. Mod-B bold, Mod-I italic, Mod-Shift-S strikethrough, Mod-E inline code, Mod-K link, Mod-Z undo, Shift-Mod-Z and Mod-Y redo, Shift-Enter line break, Mod-Enter leaves a code block or checks a task, Tab and Shift-Tab move between table cells and indent list items (outside a list or table, Tab leaves the editor), Alt-F10 moves to the toolbar, where the arrow keys, Home and End move and Escape returns to the text. Backspace right after an input rule undoes it.

Input rules: `# `, `## `, `### ` headings; `- ` and `* ` bulleted list; `1. ` numbered list; `[ ] ` task list; `> ` quote; three backticks a code block; `---` a horizontal rule; an issue key followed by a space a chip.

## Classes the design system styles

Remote carets: `span.tk-caret[data-hue]` (aria-hidden) holding `span.tk-caret__tag`; `.tk-caret--tag` while the name shows (3 s after the caret moved, or near the reader's own caret), `.tk-caret--idle` after 30 s without a move. Selections: `span.tk-remote-sel[data-hue]`. Chips: `a.issue-chip[data-key]` (with `data-state` for deleted and unknown issues) holding `svg.tk-type[data-type]`, `.issue-chip__key`, `.issue-chip__summary` and `.tk-lozenge.issue-chip__status[data-cat]`. Images: `figure.doc-image` with `button.doc-image__alt`. The shadow root's parts are `toolbar`, `dialog`, `body` and `status`. The component's own CSS uses `--tk-*` tokens with fallbacks; the light-DOM rules in `src/styles.js` sit in `@layer base` behind `:where()`, so any design-system rule wins.

The protocol's Node tests (converging editors, refusals, resets, the readonly state, comment ranges) are lith-coedit's: `cd editor && npm test` in coedit.
