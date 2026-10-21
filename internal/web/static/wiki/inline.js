// Inline comments on a page (FR-91), in view and in the editor: a Comment
// button beside a selection of the page's text (or Ctrl+Alt+M), which hands
// the composer the range as positions of the document at a version, and
// the highlights' clicks, which mark a comment and its card. The page's
// wiring is Datastar's: this module dispatches tk-ic-select on the composer
// and listens for tk-ic-made.
//
// In view the positions come from the reader's HTML, which internal/doc
// renders with one element per node (docs/editor.md, "HTML of a document");
// the server compares the selected characters with the document's and asks
// for a new selection when they differ. In the editor the editor's
// commentRange() says.

const ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
const ATOM = 'a.issue-chip, a.mention, .page-link'
const TEXTBLOCK = /^(P|H[1-6]|PRE)$/
// The elements of nodes with block content; any other block element is a
// leaf of size 1 (an image, a rule, an atom block such as an issue list).
const CONTAINER = /^(BLOCKQUOTE|UL|OL|LI|TABLE|TR|TD|TH)$/

// blocks are an element's children that are document nodes.
function blocks(el) {
	if (el.tagName === 'TABLE') return [...el.querySelectorAll(':scope > tbody > tr')]
	if (el.tagName === 'LI' && el.classList.contains('task-item')) return [...(el.querySelector(':scope > div')?.children ?? [])]
	return [...el.children]
}

// segments lists the reader's text nodes and atoms with their positions.
function segments(root) {
	const out = []
	let pos = 0
	const inline = (el) => {
		for (const n of el.childNodes) {
			if (n.nodeType === Node.TEXT_NODE) {
				out.push({ node: n, pos, len: n.data.length, text: true })
				pos += n.data.length
			} else if (n.nodeType === Node.ELEMENT_NODE) {
				if (n.matches(ATOM) || n.tagName === 'BR') {
					out.push({ node: n, pos, len: 1, text: false })
					pos += 1
				} else inline(n)
			}
		}
	}
	const block = (el) => {
		if (TEXTBLOCK.test(el.tagName)) {
			pos += 1
			inline(el)
			pos += 1
		} else if (CONTAINER.test(el.tagName)) {
			pos += 1
			for (const c of blocks(el)) block(c)
			pos += 1
		} else pos += 1
	}
	for (const c of root.children) block(c)
	return out
}

// pointPos is the document position of a range's boundary; start says
// which end it is, for a boundary between nodes or inside an atom.
function pointPos(segs, container, offset, start) {
	const atom = (container.nodeType === Node.ELEMENT_NODE ? container : container.parentElement)?.closest(ATOM)
	if (atom) {
		const s = segs.find((x) => x.node === atom)
		return s ? s.pos + (start ? 0 : 1) : null
	}
	if (container.nodeType === Node.TEXT_NODE) {
		const s = segs.find((x) => x.node === container)
		if (s) return s.pos + Math.min(offset, s.len)
	}
	// Between nodes: the first segment after the boundary, or the end of
	// the last one before it.
	const r = document.createRange()
	r.setStart(container, offset)
	if (start) {
		const s = segs.find((x) => r.comparePoint(x.node, 0) >= 0)
		return s ? s.pos : null
	}
	let last = null
	for (const s of segs) if (r.comparePoint(s.node, 0) < 0) last = s
	return last ? last.pos + last.len : null
}

// readerRange is the selection in the reader as {v, epoch, from, to, text}.
function readerRange(body, range) {
	const segs = segments(body)
	const from = pointPos(segs, range.startContainer, range.startOffset, true)
	const to = pointPos(segs, range.endContainer, range.endOffset, false)
	if (from == null || to == null || from >= to) return null
	let text = ''
	for (const s of segs) {
		if (!s.text || s.pos + s.len <= from || s.pos >= to) continue
		text += s.node.data.slice(Math.max(0, from - s.pos), Math.min(s.len, to - s.pos))
	}
	if (!text) return null
	return { v: Number(body.dataset.v), epoch: body.dataset.epoch || '', from, to, text }
}

const quoteOf = (sel) => {
	const q = sel.toString().replace(/\s+/g, ' ').trim()
	return q.length > 200 ? q.slice(0, 199) + '…' : q
}

function start() {
	const add = document.getElementById('wk-ic-add')
	const form = document.getElementById('wk-ic-form')
	const style = document.createElement('style')
	document.head.append(style)
	let pending = null // {detail, rect}

	// current is the selection as an inline comment's range, or null.
	const current = () => {
		const sel = document.getSelection()
		if (!sel || sel.isCollapsed || !sel.rangeCount) return null
		const range = sel.getRangeAt(0)
		const body = document.getElementById('page-body')
		const editor = document.getElementById('editor')
		let detail = null
		if (body && body.contains(range.commonAncestorContainer)) detail = readerRange(body, range)
		else if (editor && editor.querySelector('.pm-mount')?.contains(range.commonAncestorContainer)) detail = editor.commentRange?.() ?? null
		if (!detail || !Number.isInteger(detail.v)) return null
		return { detail: { ...detail, quote: quoteOf(sel) }, rect: range.getBoundingClientRect() }
	}

	const place = () => {
		if (!add || !form) return
		pending = current()
		if (!pending) {
			add.hidden = true
			return
		}
		const box = add.offsetParent?.getBoundingClientRect() ?? { left: 0, top: 0 }
		const r = pending.rect
		add.hidden = false
		const left = Math.min(r.right - box.left, (add.offsetParent?.clientWidth ?? innerWidth) - add.offsetWidth)
		add.style.insetInlineStart = Math.max(0, left) + 'px'
		add.style.insetBlockStart = r.bottom - box.top + 6 + 'px'
	}

	let queued = 0
	document.addEventListener('selectionchange', () => {
		cancelAnimationFrame(queued)
		queued = requestAnimationFrame(place)
	})

	const open = () => {
		const p = pending ?? current()
		if (!p || !form) return false
		add.hidden = true
		form.dispatchEvent(new CustomEvent('tk-ic-select', { detail: p.detail }))
		requestAnimationFrame(() => {
			const t = document.getElementById('ic-body')
			t?.focus()
			t?.scrollIntoView({ block: 'nearest' })
		})
		return true
	}
	// The button keeps the selection: a press would collapse it.
	add?.addEventListener('pointerdown', (e) => e.preventDefault())
	add?.addEventListener('click', open)
	document.addEventListener('keydown', (e) => {
		if (e.ctrlKey && e.altKey && !e.shiftKey && (e.key === 'm' || e.key === 'M' || e.code === 'KeyM') && open()) e.preventDefault()
	})

	// A highlight's click marks its comment and brings its card into view;
	// a card's quote brings its text into view.
	const activate = (id, scroll) => {
		if (!ID.test(id)) return
		style.textContent = `mark.tk-ic[data-c~="${id}"]{background:var(--tk-ic-active);outline:1px solid var(--tk-ic-edge)}#ic-${id}{border-color:var(--tk-ic-edge);box-shadow:inset 3px 0 0 var(--tk-ic-edge)}`
		if (scroll === 'card') document.getElementById('ic-' + id)?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
		if (scroll === 'text') document.querySelector(`mark.tk-ic[data-c~="${id}"]`)?.scrollIntoView({ block: 'center', behavior: 'smooth' })
	}
	document.addEventListener('click', (e) => {
		const go = e.target.closest?.('[data-ic-go]')
		if (go) return activate(go.dataset.c, 'text')
		const m = e.target.closest?.('mark.tk-ic')
		if (m && document.getSelection()?.isCollapsed !== false) activate((m.dataset.c || '').split(' ').pop(), 'card')
	})
	// A new comment opens beside its text once the frame brings its card.
	document.addEventListener('tk-ic-made', (e) => {
		const id = String(e.detail || '')
		let tries = 0
		const look = () => {
			if (document.getElementById('ic-' + id)) return activate(id, 'card')
			if (++tries < 50) setTimeout(look, 100)
		}
		look()
	})
}

if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start, { once: true })
else start()
