// sb-rich-editor: tickit's co-editing component (docs/editor.md, editor/README.md).
//
// A Rocket host whose shadow root holds the toolbar, the dialogs and the
// status line, with a default slot for the light-DOM children the server
// renders: the document at version `base` in <script type="application/json">
// and <div class="pm-mount prose" data-ignore-morph data-ignore>, where
// ProseMirror mounts. The stream morphs only the host's attributes. The
// editor itself (ProseMirror and the protocol) loads on first use.
import { rocket, startPeeking, stopPeeking } from 'datastar'

// The build replaces this with the core's file name as lith's edge.Static
// names it (pm-core.min.<sha256[:12]>.js), so the import is immutable.
const CORE = './pm-core.js'

const peek = (fn) => {
	startPeeking()
	try {
		return fn()
	} finally {
		stopPeeking()
	}
}

// The editor outlives a re-attach (setup runs again), and goes 1 s after the
// host left the document for good.
const live = new WeakMap()
let core

const icon = (d) => `<svg viewBox="0 0 16 16" aria-hidden="true" focusable="false"><path fill-rule="evenodd" d="${d}"/></svg>`
const I = {
	link: 'M1 5h6v1H2v4h5v1H1zM9 5h6v6H9v-1h5V6H9zM5 7h6v2H5z',
	bullet: 'M2 3h2v2H2zM6 3h8v2H6zM2 7h2v2H2zM6 7h8v2H6zM2 11h2v2H2zM6 11h8v2H6z',
	ordered: 'M2 2h2v4H3V3H2zM6 3h8v2H6zM2 8h3v3H3v1h2v1H2v-3h2V9H2zM6 10h8v2H6z',
	task: 'M1 2h5v5H1zM2 3v3h3V3zM8 4h7v1H8zM1 9h5v5H1zM2 10v3h3v-3zM8 11h7v1H8z',
	quote: 'M2 4h4v4H4v3H2zM9 4h4v4h-2v3H9z',
	codeblock: 'M5 2h2v1H6v4H5v2h1v4h1v1H5v-1H4V9H3V7h1V3h1zM9 2h2v1h1v4h1v2h-1v4h-1v1H9v-1h1V9h1V7h-1V3H9z',
	rule: 'M1 7h14v2H1z',
	table: 'M1 2h14v12H1zM2 3v3h5V3zM8 3v3h6V3zM2 7v3h5V7zM8 7v3h6V7zM2 11v2h5v-2zM8 11v2h6v-2z',
	image: 'M1 2h14v12H1zM2 3v10h12V3zM4 5h2v2H4zM3 12h10v-1h-1v-1h-1V9h-1v1H9v1H8v-1H7V9H6v1H5v1H3z',
	undo: 'M5 3h1v2h5v1h1v1h1v4h-1v1h-1v1H7v-2h4V7H6v2H5V8H4V7H3V6h1V5h1z',
	redo: 'M11 3h-1v2H5v1H4v1H3v4h1v1h1v1h4v-2H5V7h5v2h1V8h1V7h1V6h-1V5h-1z',
}
// A button: value is the command, keys its shortcut (Mod is Ctrl or Cmd).
const btn = (value, label, content, { keys, toggle = true } = {}) =>
	`<button type="button" value="${value}" aria-label="${label}"${toggle ? ' aria-pressed="false"' : ''}${keys ? ` data-keys="${keys}" aria-keyshortcuts="${keys.replace('Mod', 'Control')}"` : ''} tabindex="-1">${content}</button>`
const txt = (value, label) => `<button type="button" value="${value}" class="text" tabindex="-1">${label}</button>`

const MARKUP = `<div class="toolbar" part="toolbar" role="toolbar" aria-label="Formatting">
<span class="group">${btn('h1', 'Heading H1', 'H1')}${btn('h2', 'Heading H2', 'H2')}${btn('h3', 'Heading H3', 'H3')}</span>
<span class="group">${btn('strong', 'Bold', '<b>B</b>', { keys: 'Mod+B' })}${btn('em', 'Italic', '<i>I</i>', { keys: 'Mod+I' })}${btn('strike', 'Strikethrough', '<s>S</s>', { keys: 'Mod+Shift+S' })}${btn('code', 'Inline code', '<code>&lt;/&gt;</code>', { keys: 'Mod+E' })}${btn('link', 'Link', icon(I.link), { keys: 'Mod+K', toggle: false })}</span>
<span class="group">${btn('bullet', 'Bulleted list', icon(I.bullet))}${btn('ordered', 'Numbered list', icon(I.ordered))}${btn('task', 'Task list', icon(I.task))}</span>
<span class="group">${btn('quote', 'Quote', icon(I.quote))}${btn('codeblock', 'Code block', icon(I.codeblock))}${btn('rule', 'Horizontal rule', icon(I.rule), { toggle: false })}${btn('table', 'Table', icon(I.table), { toggle: false })}${btn('image', 'Image', icon(I.image), { toggle: false })}</span>
<span class="group group--code" hidden><label class="vh" for="lang">Code language</label><select id="lang" tabindex="-1"></select></span>
<span class="group group--table" hidden aria-label="Table">${txt('rowBefore', 'Row above')}${txt('rowAfter', 'Row below')}${txt('rowDelete', 'Delete row')}${txt('colBefore', 'Column left')}${txt('colAfter', 'Column right')}${txt('colDelete', 'Delete column')}${txt('headerRow', 'Header row')}${txt('tableDelete', 'Delete table')}</span>
<span class="group">${btn('undo', 'Undo', icon(I.undo), { keys: 'Mod+Z', toggle: false })}${btn('redo', 'Redo', icon(I.redo), { keys: 'Mod+Shift+Z', toggle: false })}</span>
</div>
<form class="dialog" id="link-dialog" part="dialog" role="dialog" aria-label="Link" hidden>
<label for="href">Link address</label><input id="href" name="href" type="text" inputmode="url" maxlength="2048" autocomplete="off" spellcheck="false" placeholder="https://">
<p class="error" id="link-error" aria-live="polite"></p>
<div class="actions"><button value="apply">Apply</button><button value="unlink">Remove link</button><button value="cancel">Cancel</button></div>
</form>
<form class="dialog" id="alt-dialog" part="dialog" role="dialog" aria-label="Alt text" hidden>
<label for="alt">Alt text: what the image shows</label><input id="alt" name="alt" type="text" maxlength="500" autocomplete="off">
<div class="actions"><button value="apply">Apply</button><button value="cancel">Cancel</button></div>
</form>
<div class="dialog" id="image-dialog" part="dialog" role="dialog" aria-label="Insert an image" hidden>
<div class="images"></div><div class="actions"><button type="button" value="cancel">Cancel</button></div>
</div>
<div class="body" part="body"><slot></slot></div>
<div class="statusbar" part="status">
<span class="saved" id="saved"></span>
<div class="notice" id="notice" role="status" aria-live="polite" hidden><span id="notice-text"></span> <button type="button" id="notice-act" hidden></button> <button type="button" id="notice-close" hidden>Dismiss</button></div>
</div>`

const styles = /* css */ `
:host {
	--_bg: var(--tk-surface-1, #fff);
	--_bar: var(--tk-surface-2, #f1f3f6);
	--_border: var(--tk-border, #c9ced8);
	--_text: var(--tk-text-1, #1d2330);
	--_muted: var(--tk-text-muted, #6b7385);
	--_accent: var(--tk-accent, #1c9fbf);
	--_focus: var(--tk-focus, #0f6a80);
	--_wash: var(--tk-surface-selected, rgb(28 159 191 / .13));
	--_danger: var(--tk-danger, #b3261e);
	--_control: var(--tk-control-sm, 1.75rem);
	--_radius: calc(var(--tk-radius-sm, 4px) * (1 - var(--tk-notch, 0)));
	display: block;
	position: relative;
	color: var(--_text);
}
:host([hidden]) { display: none; }
.vh { position: absolute; inline-size: 1px; block-size: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
.toolbar {
	position: sticky; inset-block-start: var(--tk-header-height, 0); z-index: var(--tk-z-sticky, 10);
	display: flex; flex-wrap: wrap; gap: var(--tk-space-1, .25rem) var(--tk-space-3, .75rem); align-items: center;
	padding: var(--tk-space-1, .25rem) var(--tk-space-2, .5rem);
	background: var(--_bar); border: 1px solid var(--_border); border-radius: var(--_radius);
	font: var(--tk-weight-medium, 500) var(--tk-text-sm, .8125rem)/1 var(--tk-font-mono, ui-monospace, monospace);
}
.group { display: inline-flex; gap: 2px; align-items: center; }
.group[hidden] { display: none; }
button, select {
	font: inherit; color: inherit; background: transparent; border: 1px solid transparent; border-radius: var(--_radius);
	min-block-size: var(--_control); min-inline-size: var(--_control); padding: 0 var(--tk-space-1_5, .375rem);
	display: inline-flex; align-items: center; justify-content: center; gap: .25em; cursor: pointer;
}
select { padding-inline-end: var(--tk-space-1, .25rem); border-color: var(--_border); background: var(--_bg); }
.toolbar button:hover:not(:disabled) { background: var(--tk-surface-hover, rgb(0 0 0 / .06)); }
.toolbar button[aria-pressed="true"] { background: var(--_wash); border-color: var(--_accent); color: var(--tk-accent-text, var(--_text)); }
button:disabled, select:disabled { opacity: .45; cursor: default; }
button:focus-visible, select:focus-visible, input:focus-visible { outline: 2px solid var(--_focus); outline-offset: 1px; }
button svg { inline-size: 1rem; block-size: 1rem; fill: currentColor; shape-rendering: crispEdges; }
button b, button i, button s { font-family: var(--tk-font-ui, system-ui, sans-serif); font-size: 1rem; }
button code { font-size: .75rem; }
button.text { font-size: var(--tk-text-xs, .75rem); padding-inline: var(--tk-space-2, .5rem); }
.dialog {
	position: absolute; inset-inline: var(--tk-space-2, .5rem); inset-block-start: calc(100% + 4px); max-inline-size: 32rem;
	z-index: var(--tk-z-dropdown, 20); display: grid; gap: var(--tk-space-2, .5rem); padding: var(--tk-space-3, .75rem);
	background: var(--tk-surface-overlay, var(--_bg)); border: 1px solid var(--tk-border-strong, var(--_border)); border-radius: var(--_radius);
	box-shadow: var(--tk-shadow-2, 0 10px 30px -12px rgb(0 0 0 / .3)); font: var(--tk-text-md, .875rem)/var(--tk-leading-ui, 1.45) var(--tk-font-ui, system-ui, sans-serif);
}
.toolbar + .dialog, .dialog { inset-block-start: calc(var(--_control) + 1.25rem); }
.dialog[hidden] { display: none; }
.dialog input { font: inherit; color: inherit; background: var(--tk-surface-inset, #fff); border: 1px solid var(--_border); border-radius: var(--_radius); min-block-size: var(--tk-control-md, 2rem); padding: 0 var(--tk-space-2, .5rem); }
.dialog .actions { display: flex; gap: var(--tk-space-2, .5rem); flex-wrap: wrap; }
.dialog .actions button { border-color: var(--_border); }
.dialog .actions button:first-child { background: var(--_accent); color: var(--tk-on-accent, #fff); border-color: var(--_accent); }
.dialog .actions button[hidden] { display: none; }
.error { margin: 0; color: var(--_danger); min-block-size: 1.2em; }
.images { display: grid; grid-template-columns: repeat(auto-fill, minmax(7rem, 1fr)); gap: var(--tk-space-2, .5rem); max-block-size: 20rem; overflow: auto; }
.images button { flex-direction: column; border-color: var(--_border); padding: var(--tk-space-1, .25rem); block-size: auto; }
.images img { inline-size: 100%; aspect-ratio: 4 / 3; object-fit: cover; }
.body { padding-block: var(--tk-space-3, .75rem); }
.statusbar { display: flex; gap: var(--tk-space-3, .75rem); align-items: baseline; flex-wrap: wrap; font: var(--tk-text-xs, .75rem)/var(--tk-leading-ui, 1.45) var(--tk-font-mono, ui-monospace, monospace); color: var(--_muted); }
.notice { color: var(--_text); font-family: var(--tk-font-ui, system-ui, sans-serif); font-size: var(--tk-text-sm, .8125rem); }
.notice[hidden] { display: none; }
.notice[data-kind="refusal"], .notice[data-kind="stopped"], .notice[data-kind="limit"] { color: var(--_danger); }
.notice button { border-color: var(--_border); min-block-size: var(--tk-control-sm, 1.75rem); color: var(--_text); }
@media (forced-colors: active) { .toolbar button[aria-pressed="true"] { outline: 2px solid Highlight; } }
`

rocket('sb-rich-editor', {
	props: ({ string, number, bool }) => ({
		page: string.docs({ description: 'Page id; goes into every event.' }),
		epoch: string.docs({ description: 'The page stream epoch the document belongs to; a frame with another one stops the editor (reset).' }),
		client: string.docs({ description: 'This tab as others see it (12 hex digits of SHA-256 of the tab id); its own caret is left out of carets.' }),
		tab: string.docs({ description: 'Optional: the tab id, copied into sb-commit and sb-presence. Without it the page adds the tab.' }),
		base: number.docs({ description: 'Stream version of the document in the child script.' }),
		head: number.docs({ description: 'Stream version of the newest commit on the server.' }),
		batches: string.default('[]').docs({ description: 'JSON [{v, ref, client, steps}]: every commit after the version the tab acknowledged, as the writer applied it.' }),
		carets: string.default('[]').docs({ description: 'JSON [{id, name, hue, v, anchor, head, idle}]: other editors\' selections at version v.' }),
		comments: string.docs({ description: 'JSON {v, list: [{id, from, to}]}: the inline comments\' ranges at version v, highlighted (FR-91).' }),
		chips: string.default('{}').docs({ description: 'JSON: issue keys to {summary, status, category, type} or {state}; "people" and "pages" maps for names and titles.' }),
		state: string.default('ok').docs({ description: 'ok, gap, reset, gone or readonly. Anything but ok stops sending.' }),
		refusal: string.docs({ description: 'JSON {ref, message} of a refused commit (its steps are taken out), or plain text to show.' }),
		readonly: bool.docs({ description: 'Present: the person cannot edit (the attribute is boolean; readonly="false" is still read-only).' }),
		images: string.default('[]').docs({ description: 'JSON [{id, alt}]: the image library the Image button offers.' }),
		label: string.default('Page content').docs({ description: 'Accessible name of the editing area.' }),
	}),
	manifest: {
		slots: [{ name: 'default', description: 'The server-rendered <script type="application/json" data-ignore-morph> and <div class="pm-mount prose" data-ignore-morph data-ignore>.' }],
		events: [
			{ name: 'sb-commit', kind: 'custom-event', bubbles: true, composed: true, description: '{page, tab?, epoch, base, ref, steps}: post it as the payload of POST /cmd/pages/{page}/commit, then call posted(ref, ok).' },
			{ name: 'sb-presence', kind: 'custom-event', bubbles: true, composed: true, description: '{page, tab?, epoch, ack, v, anchor, head, seq}, at most every 100 ms.' },
			{ name: 'sb-ack', kind: 'custom-event', bubbles: true, composed: true, description: '{page, epoch, ack} after a frame\'s commits were applied.' },
			{ name: 'sb-status', kind: 'custom-event', bubbles: true, composed: true, description: '{page, status: saving|saved, unsaved}.' },
			{ name: 'sb-stopped', kind: 'custom-event', bubbles: true, composed: true, description: '{page, state, unsaved}: the editor stopped sending (reset, gone, gap, readonly, diverged).' },
			{ name: 'sb-refused', kind: 'custom-event', bubbles: true, composed: true, description: '{page, ref, message, dropped}.' },
			{ name: 'sb-limit', kind: 'custom-event', bubbles: true, composed: true, description: '{page, limit, max, value, message}: input stopped at a DM-05 limit.' },
			{ name: 'sb-insert-image', kind: 'custom-event', bubbles: true, composed: true, description: 'Image button without an images library; cancel it and call insertImage(id, alt).' },
		],
	},
	renderOnPropChange: false,
	setup: ({ host, props, observeProps, emit, cleanup, adoptStyles, defineHostProp }) => {
		adoptStyles(host, styles)
		let ed = live.get(host)
		if (ed?.dying) clearTimeout(ed.dying), (ed.dying = 0)
		const read = () =>
			peek(() => ({
				page: props.page, epoch: props.epoch, client: props.client, tab: props.tab, base: props.base, head: props.head,
				batches: props.batches, carets: props.carets, comments: props.comments, chips: props.chips, state: props.state, refusal: props.refusal,
				readonly: props.readonly, images: props.images, label: props.label,
			}))
		// One sync for a morph's burst of attribute writes.
		let queued = false
		const sync = () => {
			if (queued) return
			queued = true
			queueMicrotask(() => ((queued = false), live.get(host)?.ed?.sync()))
		}
		observeProps(sync)

		const start = () => {
			if (!host.isConnected || live.get(host)?.ed) return
			const mount = host.querySelector(':scope > .pm-mount')
			if (!mount || !host.shadowRoot) return
			const script = host.querySelector(':scope > script[type="application/json"]')
			let doc = null
			try {
				doc = JSON.parse(script?.textContent || 'null')
			} catch {}
			const e = core.createEditor({ host, mount, doc, read, emit, ui: host.shadowRoot })
			live.set(host, { ed: e, dying: 0 })
			host.dispatchEvent(new CustomEvent('sb-ready', { bubbles: true, composed: true, detail: { page: read().page } }))
		}
		if (ed?.ed) queueMicrotask(() => (host.shadowRoot && ed.ed.rewire(host.shadowRoot), ed.ed.sync()))
		else {
			core ??= import(CORE).then((m) => (core = m))
			Promise.resolve(core).then(() => queueMicrotask(start), (err) => console.error('sb-rich-editor: the editor did not load', err))
		}
		cleanup(() => {
			const entry = live.get(host)
			if (entry?.ed) entry.dying = setTimeout(() => (entry.ed.destroy(), live.delete(host)), 1000)
		})

		const call = (name, ...args) => live.get(host)?.ed?.[name](...args)
		defineHostProp('posted', { value: (ref, ok, status) => call('posted', ref, !!ok, status) })
		defineHostProp('copy', { value: () => call('copy') })
		defineHostProp('insertImage', { value: (id, alt) => call('insertImage', id, alt) })
		defineHostProp('insertChip', { value: (key) => call('insertNode', 'issue_chip', { key }) })
		defineHostProp('insertMention', { value: (personId, label) => call('insertNode', 'mention', { personId, label }) })
		defineHostProp('insertPageLink', { value: (pageId) => call('insertNode', 'page_link', { pageId }) })
		defineHostProp('focus', { value: () => live.get(host)?.ed?.view.focus() })
		defineHostProp('html', { value: () => call('html') })
		defineHostProp('text', { value: () => call('text') })
		defineHostProp('commentRange', { value: () => live.get(host)?.ed?.commentRange() ?? null })
		defineHostProp('json', { value: () => live.get(host)?.ed?.view.state.doc.toJSON() })
		// For tests and the page: the ProseMirror view, the collab version, unsaved steps.
		defineHostProp('view', { get: () => live.get(host)?.ed?.view })
		defineHostProp('editor', { get: () => live.get(host)?.ed })
	},
	render: ({ html }) => html({ raw: [MARKUP] }),
})
