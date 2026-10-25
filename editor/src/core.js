// The editor behind sb-rich-editor, loaded by dynamic import on first use.
// ProseMirror mounts on the light-DOM .pm-mount the server renders with
// data-ignore-morph and data-ignore; the host's attributes bring the writer's
// commits, carets and chips; events take our commits and presence out.
import { EditorState, Plugin, TextSelection } from 'prosemirror-state'
import { EditorView } from 'prosemirror-view'
import { DOMSerializer } from 'prosemirror-model'
import { history } from 'prosemirror-history'
import { gapCursor } from 'prosemirror-gapcursor'
import { dropCursor } from 'prosemirror-dropcursor'
import { tableEditing } from 'prosemirror-tables'
import { schema, limits, ISSUE_KEY, ID, normalizeHref, setLabelHooks, imageSrc, MAX_ALT } from './schema.js'
import { Session, collab, caretPlugin, commentPlugin } from 'coedit'
import { limitsPlugin } from './limits.js'
import { IssueChipView, MentionView, PageLinkView, TaskItemView, ImageView, IssueListView } from './nodeviews.js'
import { COMMANDS, rules, keymaps, inTable, inCode, setLanguage, shortcutText } from './commands.js'
import { documentCSS } from './styles.js'
import { uploadPlugin, placeholder, placed, removePlaceholder, imageFiles, check, send } from './uploads.js'

const STOPPED = {
	reset: 'This page was reset. Your last changes were not saved.',
	gone: 'This page was removed. Your last changes were not saved.',
	gap: 'This page changed too much while you were away. Copy your text before the editor loads again.',
	diverged: 'The editor lost track of the page. Copy your text, then reload the page.',
	readonly: 'You cannot change this page. Your unsaved text stays here only.',
}

let cssAdopted = false
function adoptDocumentCSS() {
	if (cssAdopted) return
	cssAdopted = true
	try {
		const sheet = new CSSStyleSheet()
		sheet.replaceSync(documentCSS)
		document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet]
	} catch {
		// Without constructable stylesheets: a <style> (style-src allows inline styles).
		const style = document.createElement('style')
		style.textContent = documentCSS
		document.head.append(style)
	}
}

const parseJSON = (s, fallback) => {
	if (typeof s !== 'string' || !s) return fallback
	try {
		return JSON.parse(s)
	} catch {
		return fallback
	}
}

// Labels for toDOM (copy) come from every open editor's chips.
const openEditors = new Set()
setLabelHooks({
	page: (id) => {
		for (const e of openEditors) if (e.chips.pages?.[id]?.title) return e.chips.pages[id].title
		return ''
	},
	person: (id) => {
		for (const e of openEditors) if (e.chips.people?.[id]?.name) return e.chips.people[id].name
		return ''
	},
})

// The extent of the mark of type around pos (from <= pos <= to), or null.
function markExtent(doc, pos, type) {
	const $pos = doc.resolve(pos)
	if (!$pos.parent.inlineContent) return null
	const spans = []
	let off = $pos.start()
	$pos.parent.forEach((child) => {
		const m = type.isInSet(child.marks), end = off + child.nodeSize
		const last = spans[spans.length - 1]
		if (m && last && last.to === off && last.mark.eq(m)) last.to = end
		else if (m) spans.push({ from: off, to: end, mark: m })
		off = end
	})
	return spans.find((x) => x.from <= pos && pos <= x.to) ?? null
}

// What a pasted text could become: an issue chip or a page link (FR-82.3).
function pasteTarget(text) {
	if (ISSUE_KEY.test(text)) return { kind: 'chip', key: text }
	let url
	try {
		url = new URL(text)
	} catch {
		return null
	}
	if (url.origin !== location.origin) return null
	const issue = /^\/([A-Z]{2,10}-[1-9][0-9]{0,6})\/?$/.exec(url.pathname)
	if (issue) return { kind: 'chip', key: issue[1] }
	const page = /^\/pages\/([^/]+)\/?$/.exec(url.pathname)
	if (page && ID.test(decodeURIComponent(page[1]))) return { kind: 'page', pageId: decodeURIComponent(page[1]) }
	return null
}

export function createEditor(opts) {
	const ed = new Editor(opts)
	openEditors.add(ed)
	return ed
}

class Editor {
	// host: the sb-rich-editor; mount: its .pm-mount; doc: the JSON of the
	// child script; read(): the host's props; emit(type, detail); ui: the
	// shadow root.
	constructor({ host, mount, doc, read, emit, ui }) {
		adoptDocumentCSS()
		this.host = host
		this.read = read
		this.emit = emit
		this.ui = ui
		this.p = read()
		this.page = this.p.page
		this.status = ''
		this.raw = { chips: null, images: null }
		this.chips = {}
		this.images = []
		this.uploading = 0
		this.atoms = new Set()
		this.offer = null // {from, to, target}
		this.listViews = new Set()
		this.tracked = new Map() // id to {from, to}: selections an issue's chip will replace (FR-105)
		this.selSig = ''

		let start
		try {
			start = schema.nodeFromJSON(doc)
			start.check()
		} catch (e) {
			console.error('sb-rich-editor: the initial document does not fit the schema', e)
			start = schema.topNodeType.createAndFill()
			this.broken = true
		}
		const ctx = (this.ctx = {
			chips: () => this.chips,
			atoms: this.atoms,
			editable: () => this.editable(),
			editAlt: (pos) => this.openAlt(pos),
			lists: this.listViews,
			list: (key) => this.listRows(key),
			filters: () => this.savedFilters(),
			listsShown: () => !!this.listsRoot(),
			focusList: false,
		})
		const nodeView = (V) => (node, view, getPos) => new V(node, view, getPos, ctx)
		const state = EditorState.create({
			doc: start,
			plugins: [
				collab({ version: this.p.base }),
				history(),
				rules(),
				...keymaps({ openLink: () => (this.openLink(), true), focusToolbar: () => (this.focusToolbar(), true) }),
				gapCursor(),
				dropCursor(),
				tableEditing(),
				limitsPlugin((hit) => this.limitHit(hit)),
				caretPlugin(),
				commentPlugin(),
				uploadPlugin(),
				new Plugin({ props: { handlePaste: (view, event) => this.paste(event), handleDrop: (view, event, slice, moved) => !moved && this.drop(event) } }),
			],
		})
		this.view = new EditorView(
			{ mount },
			{
				state,
				editable: () => this.editable(),
				attributes: () => ({ role: 'textbox', 'aria-multiline': 'true', 'aria-label': this.read().label || 'Page content', spellcheck: 'true' }),
				nodeViews: {
					issue_chip: nodeView(IssueChipView),
					mention: nodeView(MentionView),
					page_link: nodeView(PageLinkView),
					task_item: nodeView(TaskItemView),
					image: nodeView(ImageView),
					issue_list: nodeView(IssueListView),
				},
				handleClick: (view, pos, event) => {
					// Ctrl or Cmd and a click follows a link.
					if (!(event.ctrlKey || event.metaKey)) return false
					const m = markExtent(view.state.doc, pos, schema.marks.link)?.mark
					if (m) window.open(m.attrs.href, '_blank', 'noopener')
					return !!m
				},
				dispatchTransaction: (tr) => this.dispatch(tr),
			},
		)
		// The commit protocol, carets and comment highlights: coedit's
		// Session, with the design system's class names.
		this.session = new Session({
			view: this.view,
			schema,
			read: this.read,
			emit: (type, detail) => this.protocol(type, detail),
			carets: { prefix: 'tk' },
			comments: { className: 'tk-ic', attr: 'data-c' },
		})
		this.wireUI()
		this.sync()
		if (this.broken) this.session.stop('diverged')
	}

	get stopped() {
		return this.session?.stopped ?? null
	}

	get stats() {
		return this.session.stats
	}

	editable() {
		if (this.session) return this.session.editable()
		return !this.p.readonly && this.p.state === 'ok'
	}

	dispatch(tr) {
		if (this.destroyed) return
		const before = this.view.state
		const state = before.apply(tr)
		this.view.updateState(state)
		if (tr.docChanged)
			for (const [id, t] of this.tracked) {
				const from = tr.mapping.map(t.from, 1)
				this.tracked.set(id, { from, to: Math.max(from, tr.mapping.map(t.to, -1)) })
			}
		if (tr.docChanged || tr.selectionSet) this.selectionChanged()
		if (this.offer && tr.docChanged) {
			this.offer.from = tr.mapping.map(this.offer.from, 1)
			this.offer.to = tr.mapping.map(this.offer.to, -1)
			if (this.offer.from >= this.offer.to) this.clearOffer()
		}
		this.session?.transaction(tr)
		this.updateStatus()
		this.updateToolbar()
	}

	// sync reads the host's attributes and applies what changed: the
	// session the protocol's, then tickit's own. The page's frames morph
	// them; a burst of attribute writes arrives as one call.
	sync() {
		if (this.destroyed) return
		const p = (this.p = this.read())
		if (!this.session.sync()) return
		if (p.chips !== this.raw.chips) {
			this.raw.chips = p.chips
			const c = parseJSON(p.chips, {})
			this.chips = c && typeof c === 'object' ? c : {}
			for (const a of this.atoms) a.render()
		}
		if (p.images !== this.raw.images) {
			this.raw.images = p.images
			const l = parseJSON(p.images, [])
			this.images = Array.isArray(l) ? l.filter((i) => i && ID.test(String(i.id ?? ''))) : []
		}
		this.watchLists()
		this.updateStatus()
		this.updateToolbar()
	}

	// protocol is what the session tells: posts go out as the host's
	// events, refusals and stopped states show as notices.
	protocol(type, detail) {
		switch (type) {
			case 'commit':
			case 'presence':
			case 'ack':
				return this.emit('sb-' + type, { page: this.page, ...detail })
			case 'throttled':
				return this.notice('Too many changes at once. Wait a moment.', { kind: 'rate' })
			case 'refused': {
				const message = detail.message || 'This change was not saved.'
				this.notice(message, { kind: 'refusal' })
				return this.emit('sb-refused', { page: this.page, ref: detail.ref, message, dropped: detail.dropped })
			}
			case 'stopped': {
				const { state, unsaved } = detail
				this.notice(this.p.refusal && state === 'readonly' ? parseJSON(this.p.refusal, null)?.message ?? this.p.refusal : STOPPED[state] ?? STOPPED.diverged, {
					kind: 'stopped',
					action: state === 'readonly' && !unsaved ? null : { label: 'Copy my text', run: () => this.copy() },
				})
				this.emit('sb-stopped', { page: this.page, state, unsaved })
				this.updateStatus()
				return this.updateToolbar()
			}
			case 'resumed':
				return this.clearNotice('stopped')
		}
	}

	// posted is called by the page when its @post of commit ref settled.
	posted(ref, ok, status) {
		this.session.posted(ref, ok, status)
	}

	updateStatus() {
		if (!this.session) return
		const unsaved = this.session.unsaved() + this.uploading
		const status = unsaved ? 'saving' : 'saved'
		const el = this.ui.getElementById('saved')
		// Stopped, nothing goes out: unconfirmed text is not saved (MP-14.1).
		if (el) el.textContent = this.stopped ? (unsaved ? 'Not saved' : this.stopped === 'readonly' ? 'Saved' : '') : unsaved ? 'Saving…' : 'Saved'
		if (status !== this.status) {
			this.status = status
			if (status === 'saved') this.clearNotice('rate')
			this.emit('sb-status', { page: this.page, status, unsaved })
		}
	}

	limitHit(hit) {
		this.notice(hit.message, { kind: 'limit' })
		this.emit('sb-limit', { page: this.page, ...hit })
	}

	// --- UI in the shadow root -------------------------------------------

	wireUI() {
		const ui = this.ui
		const bar = ui.querySelector('.toolbar')
		const lang = ui.getElementById('lang')
		if (lang && !lang.options.length) for (const l of limits.codeLanguages) lang.append(new Option(l || 'Plain', l))
		for (const b of ui.querySelectorAll('.toolbar button[data-keys]')) b.title = b.getAttribute('aria-label') + ' (' + shortcutText(b.dataset.keys) + ')'
		const on = (target, type, fn) => {
			target?.addEventListener(type, fn)
			;(this.unwire ??= []).push(() => target?.removeEventListener(type, fn))
		}
		// Clicks on the toolbar keep the editor's selection.
		on(bar, 'mousedown', (e) => e.target.closest('button') && e.preventDefault())
		on(bar, 'click', (e) => {
			const b = e.target.closest('button[value]')
			if (!b || b.disabled) return
			this.command(b.value)
		})
		on(bar, 'keydown', (e) => this.roving(e))
		on(lang, 'change', () => {
			setLanguage(lang.value)(this.view.state, this.view.dispatch)
			this.view.focus()
		})
		on(ui.getElementById('link-dialog'), 'submit', (e) => (e.preventDefault(), this.applyLink(e.submitter?.value)))
		on(ui.getElementById('alt-dialog'), 'submit', (e) => (e.preventDefault(), this.applyAlt(e.submitter?.value)))
		on(ui.getElementById('image-dialog'), 'click', (e) => {
			const b = e.target.closest('button')
			if (!b) return
			if (b.value === 'cancel') return this.closeDialogs()
			if (b.value === 'upload') return ui.getElementById('image-file')?.click()
			const img = this.images.find((i) => String(i.id) === b.value)
			if (img) this.insertImage(img.id, img.alt ?? '')
			this.closeDialogs()
		})
		const file = ui.getElementById('image-file')
		on(file, 'change', () => {
			const files = [...(file.files ?? [])]
			file.value = ''
			this.closeDialogs()
			if (files.length) this.uploadFiles(files)
		})
		for (const d of ui.querySelectorAll('.dialog')) on(d, 'keydown', (e) => e.key === 'Escape' && (e.preventDefault(), this.closeDialogs()))
		on(ui.getElementById('notice-act'), 'click', () => this.noticeAction?.())
		on(ui.getElementById('notice-close'), 'click', () => this.clearNotice())
		this.updateToolbar()
	}

	command(name) {
		if (!this.editable()) return
		if (name === 'link') return this.openLink()
		if (name === 'image') return this.openImages()
		const c = COMMANDS[name]
		if (!c) return
		c.run(this.view.state, this.view.dispatch, this.view)
		this.view.focus()
	}

	// The ARIA toolbar pattern: one tab stop, arrows move between controls.
	roving(e) {
		const items = [...this.ui.querySelectorAll('.toolbar button, .toolbar select')].filter((b) => !b.closest('[hidden]'))
		const i = items.indexOf(e.target)
		if (i < 0) return
		let next = null
		if (e.key === 'ArrowRight') next = items[(i + 1) % items.length]
		else if (e.key === 'ArrowLeft') next = items[(i - 1 + items.length) % items.length]
		else if (e.key === 'Home') next = items[0]
		else if (e.key === 'End') next = items[items.length - 1]
		else if (e.key === 'Escape') return (e.preventDefault(), this.view.focus())
		if (!next || (e.target.tagName === 'SELECT' && (e.key === 'Home' || e.key === 'End'))) return
		e.preventDefault()
		for (const b of items) b.tabIndex = b === next ? 0 : -1
		next.focus()
	}

	focusToolbar() {
		const items = [...this.ui.querySelectorAll('.toolbar button, .toolbar select')].filter((b) => !b.closest('[hidden]'))
		;(items.find((b) => b.tabIndex === 0) ?? items[0])?.focus()
	}

	updateToolbar() {
		if (this.toolbarFrame) return
		this.toolbarFrame = requestAnimationFrame(() => {
			this.toolbarFrame = 0
			if (this.destroyed) return
			const state = this.view.state
			const editable = this.editable()
			for (const b of this.ui.querySelectorAll('.toolbar button[value]')) {
				const c = COMMANDS[b.value]
				const on = !!c?.active?.(state)
				if (b.hasAttribute('aria-pressed')) b.setAttribute('aria-pressed', on ? 'true' : 'false')
				const can = editable && (!c || c.run(state, undefined, this.view))
				b.disabled = !can
			}
			const table = this.ui.querySelector('.group--table')
			if (table) table.hidden = !inTable(state)
			const code = this.ui.querySelector('.group--code')
			if (code) code.hidden = !inCode(state)
			const lang = this.ui.getElementById('lang')
			if (lang && inCode(state)) {
				lang.value = state.selection.$from.parent.attrs.language
				lang.disabled = !editable
			}
			// Keep one tab stop on a visible control.
			const items = [...this.ui.querySelectorAll('.toolbar button, .toolbar select')]
			const visible = items.filter((b) => !b.closest('[hidden]'))
			if (!visible.some((b) => b.tabIndex === 0)) for (const b of items) b.tabIndex = b === visible[0] ? 0 : -1
		})
	}

	// --- Notices: refusals, limits, offers and stopped states --------------

	notice(text, { kind = 'info', action = null } = {}) {
		this.noticeKind = kind
		this.noticeAction = action?.run ?? null
		const t = this.ui.getElementById('notice-text')
		const act = this.ui.getElementById('notice-act')
		const close = this.ui.getElementById('notice-close')
		const box = this.ui.getElementById('notice')
		if (!t) return
		t.textContent = text
		act.hidden = !action
		act.textContent = action?.label ?? ''
		close.hidden = kind === 'stopped'
		close.textContent = action?.dismiss ?? 'Dismiss'
		box.dataset.kind = kind
		box.hidden = false
	}

	clearNotice(kind) {
		if (kind && this.noticeKind !== kind) return
		const box = this.ui.getElementById('notice')
		if (!box || this.noticeKind === 'stopped' && kind !== 'stopped') return
		box.hidden = true
		this.noticeKind = null
		this.noticeAction = null
		this.offer = null
	}

	// --- Paste: supported formatting only, and chip or page-link offers ----

	paste(event) {
		const files = imageFiles(event.clipboardData)
		if (files.length && this.editable()) return this.uploadFiles(files)
		const text = event.clipboardData?.getData('text/plain')?.trim()
		const html = event.clipboardData?.getData('text/html')
		if (!text || (html && html.replace(/<[^>]*>/g, '').trim() !== text)) return false
		const target = pasteTarget(text)
		if (!target || !this.editable() || inCode(this.view.state)) return false
		const { from } = this.view.state.selection
		this.view.dispatch(this.view.state.tr.insertText(text).scrollIntoView())
		this.offer = { from, to: from + text.length, text, target }
		const what = target.kind === 'chip' ? `Turn ${target.key} into an issue chip?` : 'Turn this address into a page link?'
		this.notice(what, { kind: 'offer', action: { label: target.kind === 'chip' ? 'Make a chip' : 'Make a page link', dismiss: 'Keep the text', run: () => this.takeOffer() } })
		this.emit('sb-paste-offer', { page: this.page, kind: target.kind })
		return true
	}

	takeOffer() {
		const o = this.offer
		this.clearNotice()
		if (!o || !this.editable()) return
		const { doc } = this.view.state
		if (doc.textBetween(o.from, o.to) !== o.text) return
		const node = o.target.kind === 'chip' ? schema.nodes.issue_chip.create({ key: o.target.key }) : schema.nodes.page_link.create({ pageId: o.target.pageId })
		this.view.dispatch(this.view.state.tr.replaceWith(o.from, o.to, node))
		this.view.focus()
	}

	// --- Link dialog (Mod-K): http, https and mailto only (DM-06.2) --------

	openLink() {
		if (!this.editable()) return
		const { state } = this.view
		const { from, to, empty } = state.selection
		const ext = markExtent(state.doc, from, schema.marks.link)
		this.linkRange = empty ? ext ?? { from, to } : { from, to }
		const d = this.ui.getElementById('link-dialog')
		this.showDialog(d)
		const input = d.elements.href
		input.value = ext?.mark.attrs.href ?? ''
		d.querySelector('[value=unlink]').hidden = !ext && !state.doc.rangeHasMark(from, to, schema.marks.link)
		this.ui.getElementById('link-error').textContent = ''
		input.focus()
		input.select()
	}

	applyLink(action) {
		const d = this.ui.getElementById('link-dialog')
		const r = this.linkRange
		if (action === 'cancel' || !r) return this.closeDialogs()
		const { state } = this.view
		const link = schema.marks.link
		if (action === 'unlink') {
			const ext = r.from === r.to ? markExtent(state.doc, r.from, link) : r
			if (ext) this.view.dispatch(state.tr.removeMark(ext.from, ext.to, link))
			return this.closeDialogs()
		}
		let raw = d.elements.href.value.trim()
		if (raw && !/^[a-z][a-z0-9+.-]*:/i.test(raw)) raw = (raw.includes('@') && !raw.includes('/') ? 'mailto:' : 'https://') + raw
		const href = normalizeHref(raw)
		if (!href) {
			this.ui.getElementById('link-error').textContent = 'Links start with http://, https:// or mailto:.'
			d.elements.href.focus()
			return
		}
		const mark = link.create({ href })
		const tr = state.tr
		if (r.from === r.to) {
			tr.insert(r.from, schema.text(raw.replace(/^mailto:/, ''), [mark]))
		} else {
			tr.removeMark(r.from, r.to, link).addMark(r.from, r.to, mark)
		}
		this.view.dispatch(tr.scrollIntoView())
		this.closeDialogs()
	}

	// --- Images (FR-84): from the page's library, alt text editable --------

	openImages() {
		const upload = !!this.read().upload
		if (!this.images.length && !upload) {
			// The page may offer its own picker and call host.insertImage().
			const handled = !this.host.dispatchEvent(new CustomEvent('sb-insert-image', { bubbles: true, composed: true, cancelable: true, detail: { page: this.page } }))
			if (!handled) this.notice('No image library on this page.', { kind: 'info' })
			return
		}
		const d = this.ui.getElementById('image-dialog')
		const list = d.querySelector('.images')
		list.replaceChildren(
			...this.images.map((i) => {
				const b = document.createElement('button')
				b.type = 'button'
				b.value = String(i.id)
				const img = document.createElement('img')
				img.src = imageSrc(String(i.id))
				img.alt = ''
				img.loading = 'lazy'
				const span = document.createElement('span')
				span.textContent = i.alt || String(i.id)
				b.append(img, span)
				return b
			}),
		)
		d.querySelector('[value=upload]').hidden = !upload
		this.showDialog(d)
		;(list.querySelector('button') ?? d.querySelector('.actions button:not([hidden])'))?.focus()
	}

	// --- Uploads (FR-85): pasted, dropped or chosen image files ------------

	drop(event) {
		const files = imageFiles(event.dataTransfer)
		if (!files.length) return false
		event.preventDefault()
		if (!this.editable()) return true
		const at = this.view.posAtCoords({ left: event.clientX, top: event.clientY })
		return this.uploadFiles(files, at ? at.pos : undefined)
	}

	// uploadFiles sends each file and puts its image where pos was (the
	// selection by default) once the server stored it.
	uploadFiles(files, pos = this.view.state.selection.from) {
		const { upload, tab } = this.read()
		if (!this.editable()) return false
		if (!upload) {
			this.notice('Images cannot be uploaded here.', { kind: 'info' })
			return true
		}
		if (!tab) {
			this.notice('This tab is not connected. Reload the page.', { kind: 'refusal' })
			return true
		}
		for (const file of files) {
			const why = check(file)
			if (why) {
				this.notice(why, { kind: 'refusal' })
				continue
			}
			const id = {}
			const dom = placeholder(this.view, id, pos)
			this.uploading++
			this.updateStatus()
			const progress = (f) => (dom.textContent = f < 1 ? `Uploading image… ${Math.round(f * 100)}%` : 'Converting image…')
			send(upload, tab, file, progress).then(
				(r) => this.uploaded(id, r),
				(e) => this.uploadFailed(id, e.message),
			)
		}
		return true
	}

	uploaded(id, r) {
		this.uploading--
		if (this.destroyed) return
		const pos = placed(this.view.state, id)
		const tr = removePlaceholder(this.view.state.tr, id)
		if (pos != null && this.editable() && ID.test(r.id)) {
			tr.replaceRangeWith(pos, pos, schema.nodes.image.create({ imageId: r.id, src: imageSrc(r.id), alt: '' }))
		} else this.notice('The image was uploaded, but the editor stopped before it went into the page.', { kind: 'refusal' })
		this.view.dispatch(tr)
		this.updateStatus()
		this.emit('sb-uploaded', { page: this.page, id: r.id, width: r.width, height: r.height })
	}

	uploadFailed(id, message) {
		this.uploading--
		if (this.destroyed) return
		this.view.dispatch(removePlaceholder(this.view.state.tr, id))
		this.notice(message, { kind: 'refusal' })
		this.updateStatus()
	}

	insertImage(imageId, alt = '') {
		if (!this.editable() || !ID.test(String(imageId))) return false
		const node = schema.nodes.image.create({ imageId: String(imageId), src: imageSrc(String(imageId)), alt: String(alt).slice(0, MAX_ALT) })
		this.view.dispatch(this.view.state.tr.replaceSelectionWith(node).scrollIntoView())
		this.view.focus()
		return true
	}

	openAlt(pos) {
		const node = this.view.state.doc.nodeAt(pos)
		if (!node || node.type !== schema.nodes.image) return
		this.altPos = pos
		const d = this.ui.getElementById('alt-dialog')
		this.showDialog(d)
		d.elements.alt.value = node.attrs.alt
		d.elements.alt.focus()
	}

	applyAlt(action) {
		const pos = this.altPos
		const d = this.ui.getElementById('alt-dialog')
		if (action !== 'cancel' && pos != null) {
			const node = this.view.state.doc.nodeAt(pos)
			if (node?.type === schema.nodes.image) this.view.dispatch(this.view.state.tr.setNodeAttribute(pos, 'alt', d.elements.alt.value.slice(0, MAX_ALT)))
		}
		this.closeDialogs()
	}

	showDialog(d) {
		this.closeDialogs(false)
		d.hidden = false
	}

	closeDialogs(focus = true) {
		for (const d of this.ui.querySelectorAll('.dialog')) d.hidden = true
		if (focus) this.view.focus()
	}

	// --- Host API ---------------------------------------------------------

	insertNode(type, attrs) {
		return this.session.insertNode(type, attrs)
	}


	// --- Issue lists (FR-104): rows from the page's lists element ---------

	// listsRoot is the element the page's lists view patches with every
	// issue list's rows ([data-list] by key) and the saved filters
	// ([data-filters] > [data-id]); the host's lists attribute names it.
	listsRoot() {
		const id = this.p.lists
		return (id && document.getElementById(id)) || null
	}

	watchLists() {
		const root = this.listsRoot()
		if (!root || root === this.listsObserved) return
		this.listsObserver?.disconnect()
		this.listsObserved = root
		this.listsObserver = new MutationObserver(() => {
			if (this.listsFrame) return
			this.listsFrame = requestAnimationFrame(() => {
				this.listsFrame = 0
				this.filtersCache = null
				for (const v of this.listViews) v.render()
			})
		})
		this.listsObserver.observe(root, { childList: true, subtree: true, characterData: true, attributes: true })
		this.filtersCache = null
		for (const v of this.listViews) v.render()
	}

	listRows(key) {
		const root = this.listsRoot()
		if (!root) return null
		for (const e of root.querySelectorAll(':scope > [data-list]')) if (e.getAttribute('data-list') === key) return e
		return null
	}

	savedFilters() {
		if (this.filtersCache) return this.filtersCache
		const root = this.listsRoot()
		const list = root ? [...root.querySelectorAll(':scope > [data-filters] > [data-id]')].map((e) => ({ id: e.getAttribute('data-id'), name: e.textContent.trim() })) : []
		return (this.filtersCache = list)
	}

	insertIssueList(attrs = {}) {
		this.ctx.focusList = true
		const ok = this.insertNode('issue_list', attrs)
		if (!ok) this.ctx.focusList = false
		return ok
	}

	// --- Create an issue from the selection (FR-105) -----------------------

	// selectionText is the selected text as an issue's summary: chips by
	// key, whitespace folded.
	selectionText() {
		const { from, to } = this.view.state.selection
		return this.view.state.doc.textBetween(from, to, ' ', (n) => (n.type.name === 'issue_chip' ? n.attrs.key : n.type.name === 'mention' ? '@' + n.attrs.label : ' ')).replace(/\s+/g, ' ').trim()
	}

	// selectionChanged tells the page where a selection of text is, at most
	// once a frame: sb-select {page, empty, text, top, left, bottom}.
	selectionChanged() {
		if (this.selFrame) return
		this.selFrame = requestAnimationFrame(() => {
			this.selFrame = 0
			if (this.destroyed) return
			const sel = this.view.state.selection
			const text = sel.empty || !(sel instanceof TextSelection) || !this.editable() ? '' : this.selectionText()
			let detail = { page: this.page, empty: true, text: '', top: 0, left: 0, bottom: 0 }
			if (text) {
				const a = this.view.coordsAtPos(sel.from), b = this.view.coordsAtPos(sel.to)
				detail = { page: this.page, empty: false, text, top: Math.min(a.top, b.top), left: a.left, bottom: Math.max(a.bottom, b.bottom) }
			}
			const sig = detail.empty ? '' : `${sel.from}:${sel.to}:${detail.top}:${detail.left}`
			if (sig === this.selSig) return
			this.selSig = sig
			this.emit('sb-select', detail)
		})
	}

	// track keeps the selection under id, mapped through every change, for
	// chipTracked.
	track(id) {
		const { from, to, empty } = this.view.state.selection
		if (empty || !id) return false
		if (this.tracked.size >= 20) this.tracked.delete(this.tracked.keys().next().value)
		this.tracked.set(String(id), { from, to })
		return true
	}

	// chipTracked replaces the selection kept under id with key's chip, as
	// a commit of this editor: wherever the selection went meanwhile.
	chipTracked(id, key) {
		const t = this.tracked.get(String(id))
		this.tracked.delete(String(id))
		if (!t || !this.editable() || !ISSUE_KEY.test(String(key))) return false
		const chip = schema.nodes.issue_chip.create({ key: String(key) })
		const { doc } = this.view.state
		const $from = doc.resolve(t.from)
		if (!$from.parent.inlineContent) return false
		this.view.dispatch(this.view.state.tr.replaceWith(t.from, t.from === t.to ? t.from : t.to, chip))
		return true
	}

	html() {
		const div = document.createElement('div')
		div.append(DOMSerializer.fromSchema(schema).serializeFragment(this.view.state.doc.content))
		return div.innerHTML
	}

	// commentRange is the selection as an inline comment's range (FR-91).
	commentRange() {
		return this.session.commentRange()
	}

	text() {
		return this.view.state.doc.textBetween(0, this.view.state.doc.content.size, '\n\n', (n) => (n.type === schema.nodes.hard_break ? '\n' : n.type.name === 'issue_chip' ? n.attrs.key : ''))
	}

	// copy puts the editor's content on the clipboard (MP-13.1).
	async copy() {
		const html = this.html(), text = this.text()
		let ok = false
		try {
			if (typeof ClipboardItem !== 'undefined' && navigator.clipboard?.write) {
				await navigator.clipboard.write([new ClipboardItem({ 'text/html': new Blob([html], { type: 'text/html' }), 'text/plain': new Blob([text], { type: 'text/plain' }) })])
				ok = true
			}
		} catch {}
		if (!ok)
			try {
				await navigator.clipboard.writeText(text)
				ok = true
			} catch {}
		if (!ok) {
			// Last resort: select everything so Ctrl+C copies it.
			this.view.dispatch(this.view.state.tr.setSelection(TextSelection.create(this.view.state.doc, 0, this.view.state.doc.content.size)))
			this.view.focus()
		}
		const t = this.ui.getElementById('notice-act')
		if (t) t.textContent = ok ? 'Copied' : 'Press Ctrl+C to copy'
		this.emit('sb-copied', { page: this.page, ok })
		return ok
	}

	destroy() {
		this.destroyed = true
		openEditors.delete(this)
		this.session.destroy()
		cancelAnimationFrame(this.toolbarFrame)
		cancelAnimationFrame(this.selFrame)
		cancelAnimationFrame(this.listsFrame)
		this.listsObserver?.disconnect()
		for (const f of this.unwire ?? []) f()
		this.view.destroy()
	}

	// rewire moves the listeners to a new shadow root's controls (a re-attach).
	rewire(ui) {
		for (const f of this.unwire ?? []) f()
		this.unwire = []
		this.ui = ui
		this.wireUI()
	}
}

