// The editor behind sb-rich-editor, loaded by dynamic import on first use.
// ProseMirror mounts on the light-DOM .pm-mount the server renders with
// data-ignore-morph and data-ignore; the host's attributes bring the writer's
// commits, carets and chips; events take our commits and presence out.
import { EditorState, Plugin, TextSelection } from 'prosemirror-state'
import { EditorView } from 'prosemirror-view'
import { DOMSerializer } from 'prosemirror-model'
import { Mapping } from 'prosemirror-transform'
import { history } from 'prosemirror-history'
import { gapCursor } from 'prosemirror-gapcursor'
import { dropCursor } from 'prosemirror-dropcursor'
import { tableEditing } from 'prosemirror-tables'
import { schema, limits, ISSUE_KEY, ID, normalizeHref, setLabelHooks, imageSrc, MAX_ALT } from './schema.js'
import { collab, collabState, getVersion, makeCommit, batchTransaction, receiveTransaction, REMOTE } from './collab.js'
import { limitsPlugin } from './limits.js'
import { caretPlugin, Carets } from './carets.js'
import { IssueChipView, MentionView, PageLinkView, TaskItemView, ImageView } from './nodeviews.js'
import { COMMANDS, rules, keymaps, inTable, inCode, setLanguage, shortcutText } from './commands.js'
import { documentCSS } from './styles.js'

const MAX_STEPS = 20
const PRESENCE_MS = 100
const CONFIRM_WAIT = 3000 // an answered commit the frames have not confirmed goes out again
const BACKOFF = [1000, 2000, 4000, 8000, 15000]

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
		this.epoch = this.p.epoch
		this.inflight = null // {detail, last, tries, timer}
		this.stopped = null
		this.status = ''
		this.seq = 0
		this.raw = { batches: null, carets: null, chips: null, refusal: null, images: null }
		this.chips = {}
		this.images = []
		this.atoms = new Set()
		this.handledRefusals = new Set()
		this.offer = null // {from, to, target}
		this.stats = { resyncs: 0, resends: 0, commits: 0, received: 0, refused: 0 }
		this.presenceAt = 0
		this.presenceTimer = 0

		let start
		try {
			start = schema.nodeFromJSON(doc)
			start.check()
		} catch (e) {
			console.error('sb-rich-editor: the initial document does not fit the schema', e)
			start = schema.topNodeType.createAndFill()
			this.broken = true
		}
		const ctx = {
			chips: () => this.chips,
			atoms: this.atoms,
			editable: () => this.editable(),
			editAlt: (pos) => this.openAlt(pos),
		}
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
				new Plugin({ props: { handlePaste: (view, event) => this.paste(event) } }),
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
		this.carets = new Carets(this.view)
		this.wireUI()
		this.sync()
		if (this.broken) this.stop('diverged')
	}

	editable() {
		return !this.stopped && !this.p.readonly && this.p.state === 'ok'
	}

	dispatch(tr) {
		if (this.destroyed) return
		const before = this.view.state
		const state = before.apply(tr)
		this.view.updateState(state)
		if (this.offer && tr.docChanged) {
			this.offer.from = tr.mapping.map(this.offer.from, 1)
			this.offer.to = tr.mapping.map(this.offer.to, -1)
			if (this.offer.from >= this.offer.to) this.clearOffer()
		}
		if (tr.docChanged && !tr.getMeta(REMOTE)) this.send()
		if (tr.docChanged || tr.selectionSet) this.presence()
		if (tr.selectionSet && !tr.getMeta(REMOTE)) this.carets.tags()
		this.updateStatus()
		this.updateToolbar()
	}

	// sync reads the host's attributes and applies what changed. The page's
	// frames morph them; a burst of attribute writes arrives as one call.
	sync() {
		if (this.destroyed) return
		const p = (this.p = this.read())
		if (this.stopped && this.stopped !== 'readonly') return
		if (p.epoch && this.epoch && p.epoch !== this.epoch) return this.stop('reset')
		if (['reset', 'gone', 'gap'].includes(p.state)) return this.stop(p.state)
		if (p.state === 'readonly' && !this.stopped) this.stop('readonly')
		else if (p.state === 'ok' && this.stopped === 'readonly') this.resume()

		let applied = false
		if (p.batches !== this.raw.batches) {
			const batches = parseJSON(p.batches, [])
			const done = this.receive(Array.isArray(batches) ? batches : [])
			if (done === false) return
			applied = done > 0
			// A frame that left a gap is read again with the next one.
			if (!this.gapPending) this.raw.batches = p.batches
		}
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
		if (p.refusal !== this.raw.refusal) {
			this.raw.refusal = p.refusal
			if (p.refusal) this.refused(p.refusal)
		}
		if (applied || p.carets !== this.raw.carets) {
			this.raw.carets = p.carets
			this.carets.set(parseJSON(p.carets, []), p.client)
		}
		this.view.setProps({}) // editable may have changed
		if (applied) {
			const ack = getVersion(this.view.state)
			this.emit('sb-ack', { page: this.page, epoch: this.epoch, ack })
			this.presence()
		}
		this.send()
		this.updateStatus()
		this.updateToolbar()
	}

	// receive applies the frame's commits in version order and returns how
	// many it applied, or false when the editor had to stop.
	receive(batches) {
		let n = 0
		this.gapPending = false
		const sorted = batches.filter((b) => b && Number.isInteger(b.v) && Array.isArray(b.steps)).sort((a, b) => a.v - b.v)
		for (const b of sorted) {
			const v = getVersion(this.view.state)
			if (b.v <= v) continue
			if (b.v !== v + 1) {
				this.gapPending = true // wait for the frame that brings v + 1
				break
			}
			let r
			try {
				r = batchTransaction(this.view.state, schema, b, this.inflight ? { ref: this.inflight.detail.ref, last: this.inflight.last } : null)
			} catch (e) {
				console.error('sb-rich-editor: a commit from the server does not parse', b, e)
				this.stop('diverged')
				return false
			}
			if (!r.tr) {
				console.error('sb-rich-editor: a commit from the server does not apply', b)
				this.stop('diverged')
				return false
			}
			if (r.own) {
				clearTimeout(this.inflight.timer)
				this.inflight = null
				if (r.reconciled) {
					this.stats.resyncs++
					console.warn('sb-rich-editor: the server applied commit %s differently; the editor took the server\'s steps', b.ref)
				}
			} else this.stats.received++
			this.carets.confirmed(b.v, r.steps)
			this.view.dispatch(r.tr)
			n++
		}
		return n
	}

	// send emits our next commit unless one is in flight (one at a time, at
	// most 20 steps).
	send() {
		if (this.inflight || this.stopped || this.p.state !== 'ok' || this.p.readonly) return
		const c = makeCommit(this.view.state, MAX_STEPS)
		if (!c) return
		const detail = { page: this.page, epoch: this.epoch, base: c.base, ref: c.ref, steps: c.steps }
		if (this.p.tab) detail.tab = this.p.tab
		this.inflight = { detail, last: c.last, tries: 0, timer: 0 }
		this.stats.commits++
		this.emit('sb-commit', detail)
		// If nobody calls posted() (no page wiring), the commit still goes out again.
		this.inflight.timer = setTimeout(() => this.resend(), CONFIRM_WAIT * 2)
	}

	resend() {
		const f = this.inflight
		if (!f || this.stopped || this.p.state !== 'ok') return
		f.tries++
		this.stats.resends++
		this.emit('sb-commit', f.detail)
		clearTimeout(f.timer)
		f.timer = setTimeout(() => this.resend(), CONFIRM_WAIT * 2)
	}

	// posted is called by the page when its @post of commit ref settled: ok
	// when the server answered (Datastar's @post resolves for any status),
	// false on a network error. status, when the page has it, refines that.
	posted(ref, ok, status) {
		const f = this.inflight
		if (!f || f.detail.ref !== ref) return
		clearTimeout(f.timer)
		if (status === 422 || status === 409) {
			// The frame says why (refusal, or state); wait for it.
			f.timer = setTimeout(() => this.resend(), CONFIRM_WAIT * 3)
			return
		}
		if (!ok || status === 429 || (status >= 500 && status < 600)) {
			if (status === 429) this.notice('Too many changes at once. Wait a moment.', { kind: 'rate' })
			const wait = BACKOFF[Math.min(f.tries, BACKOFF.length - 1)]
			f.timer = setTimeout(() => this.resend(), wait)
			return
		}
		f.timer = setTimeout(() => this.resend(), CONFIRM_WAIT)
	}

	// refused handles the refusal attribute: {"ref", "message"} takes that
	// commit's steps out (the writer will never apply them); plain text only
	// shows.
	refused(raw) {
		const r = parseJSON(raw, null) ?? { message: String(raw) }
		const key = r.ref ?? raw
		if (this.handledRefusals.has(key)) return
		this.handledRefusals.add(key)
		const f = this.inflight
		let dropped = 0
		if (r.ref && f && f.detail.ref === r.ref) {
			const before = collabState(this.view.state).unconfirmed.length
			const tr = receiveTransaction(this.view.state, [], { drop: f.last })
			clearTimeout(f.timer)
			this.inflight = null
			if (tr) {
				this.view.dispatch(tr)
				dropped = before - collabState(this.view.state).unconfirmed.length
			}
			this.stats.refused++
		}
		const message = String(r.message || 'This change was not saved.')
		this.notice(message, { kind: 'refusal' })
		this.emit('sb-refused', { page: this.page, ref: r.ref ?? '', message, dropped })
		this.send()
	}

	stop(state) {
		if (this.stopped === state) return
		this.stopped = state
		if (this.inflight) clearTimeout(this.inflight.timer)
		if (state !== 'readonly') {
			this.inflight = null
			clearTimeout(this.presenceTimer)
			this.presenceTimer = 0
		}
		this.view.setProps({})
		const unsaved = collabState(this.view.state).unconfirmed.length
		this.notice(this.p.refusal && state === 'readonly' ? parseJSON(this.p.refusal, null)?.message ?? this.p.refusal : STOPPED[state] ?? STOPPED.diverged, {
			kind: 'stopped',
			action: state === 'readonly' && !unsaved ? null : { label: 'Copy my text', run: () => this.copy() },
		})
		this.emit('sb-stopped', { page: this.page, state, unsaved })
		this.updateStatus()
		this.updateToolbar()
	}

	resume() {
		this.stopped = null
		this.clearNotice('stopped')
		this.view.setProps({})
		this.send()
	}

	// presence sends our selection at the confirmed version, at most every 100 ms.
	presence() {
		if (this.presenceTimer || (this.stopped && this.stopped !== 'readonly')) return
		const wait = Math.max(0, PRESENCE_MS - (Date.now() - this.presenceAt))
		this.presenceTimer = setTimeout(() => {
			this.presenceTimer = 0
			if (this.destroyed) return
			this.presenceAt = Date.now()
			const state = this.view.state
			const { version, unconfirmed } = collabState(state)
			const back = new Mapping(unconfirmed.map((u) => u.step.getMap())).invert()
			const { anchor, head } = state.selection
			const detail = { page: this.page, epoch: this.epoch, ack: version, v: version, anchor: back.map(anchor), head: back.map(head), seq: ++this.seq }
			if (this.p.tab) detail.tab = this.p.tab
			this.emit('sb-presence', detail)
		}, wait)
	}

	updateStatus() {
		const unsaved = collabState(this.view.state).unconfirmed.length
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
			const img = this.images.find((i) => String(i.id) === b.value)
			if (img) this.insertImage(img.id, img.alt ?? '')
			this.closeDialogs()
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
		if (!this.images.length) {
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
		this.showDialog(d)
		list.querySelector('button')?.focus()
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
		if (!this.editable()) return false
		let node
		try {
			node = schema.nodes[type].create(attrs)
			node.check()
		} catch {
			return false
		}
		this.view.dispatch(this.view.state.tr.replaceSelectionWith(node).scrollIntoView())
		return true
	}

	html() {
		const div = document.createElement('div')
		div.append(DOMSerializer.fromSchema(schema).serializeFragment(this.view.state.doc.content))
		return div.innerHTML
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
		if (this.inflight) clearTimeout(this.inflight.timer)
		clearTimeout(this.presenceTimer)
		cancelAnimationFrame(this.toolbarFrame)
		this.carets.destroy()
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

