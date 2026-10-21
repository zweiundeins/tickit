// Node views for the atoms whose text the server knows (FR-100, FR-103,
// FR-67), the clickable task checkbox, images with alt text (FR-84) and
// issue lists with their filter form and rows (FR-104).
// Chips, mentions and page links read the editor's chips attribute:
//   {"WEB-12": {summary, status, category, type} | {state: "deleted"|"unknown"},
//    "people": {"<id>": {name}}, "pages": {"<id>": {title} | {state: "gone"}}}
// so a changed issue shows in every open editor with the next frame.
import { imageSrc, limits, listAttrs, listDescription } from './schema.js'

// The issue type icons of the design system (design.md 7.5), 16 grid.
const TILE = 'M1 0h14v1h1v14h-1v1H1v-1H0V1h1z'
const GLYPHS = {
	epic: 'M8 2h4v1h-4zM7 3h4v1h-4zM6 4h4v1h-4zM5 5h4v1h-4zM4 6h8v1h-8zM7 7h4v1h-4zM6 8h4v1h-4zM5 9h4v1h-4zM4 10h3v1h-3zM3 11h2v1h-2z',
	story: 'M4 2h8v9h-8zM4 11h3v1h-3zM9 11h3v1h-3zM4 12h2v1h-2zM10 12h2v1h-2zM4 13h1v1h-1zM11 13h1v1h-1z',
	task: 'M11 4h2v1h-2zM10 5h3v1h-3zM9 6h3v1h-3zM2 7h2v1h-2zM8 7h3v1h-3zM2 8h3v1h-3zM7 8h3v1h-3zM3 9h6v1h-6zM4 10h4v1h-4zM5 11h2v1h-2z',
	bug: 'M5 2h1v1h-1zM10 2h1v1h-1zM6 3h4v1h-4zM4 4h3v1h-3zM9 4h3v1h-3zM1 5h1v1h-1zM3 5h4v1h-4zM9 5h4v1h-4zM14 5h1v1h-1zM2 6h5v1h-5zM9 6h5v1h-5zM3 7h4v1h-4zM9 7h4v1h-4zM1 8h1v1h-1zM3 8h4v1h-4zM9 8h4v1h-4zM14 8h1v1h-1zM2 9h5v1h-5zM9 9h5v1h-5zM3 10h4v1h-4zM9 10h4v1h-4zM4 11h3v1h-3zM9 11h3v1h-3zM6 12h1v1h-1zM9 12h1v1h-1z',
	subtask: 'M2 2h6v1h-6zM2 3h1v3h-1zM7 3h1v3h-1zM2 6h6v1h-6zM4 7h1v2h-1zM8 8h6v1h-6zM4 9h5v1h-5zM13 9h1v3h-1zM8 10h1v2h-1zM8 12h6v1h-6z',
}
const SVG = 'http://www.w3.org/2000/svg'

function el(tag, attrs = {}, ...kids) {
	const e = document.createElement(tag)
	for (const [k, v] of Object.entries(attrs)) if (v != null && v !== false) e.setAttribute(k, v === true ? '' : v)
	e.append(...kids)
	return e
}

function typeIcon(type) {
	const svg = document.createElementNS(SVG, 'svg')
	svg.setAttribute('class', 'tk-type')
	svg.setAttribute('data-type', type)
	svg.setAttribute('viewBox', '0 0 16 16')
	svg.setAttribute('aria-hidden', 'true')
	for (const [cls, d] of [['tk-type__tile', TILE], ['tk-type__glyph', GLYPHS[type]]]) {
		const p = document.createElementNS(SVG, 'path')
		p.setAttribute('class', cls)
		p.setAttribute('d', d)
		svg.append(p)
	}
	return svg
}

// Atom views share these: the server's data may change under them.
class AtomView {
	constructor(node, ctx) {
		this.node = node
		this.ctx = ctx
		ctx.atoms.add(this)
	}
	update(node) {
		if (node.type !== this.node.type) return false
		this.node = node
		this.render()
		return true
	}
	selectNode() {
		this.dom.classList.add('ProseMirror-selectednode')
	}
	deselectNode() {
		this.dom.classList.remove('ProseMirror-selectednode')
	}
	ignoreMutation() {
		return true
	}
	destroy() {
		this.ctx.atoms.delete(this)
	}
	// A plain click selects the atom; Ctrl or Cmd with it opens the link.
	follow(dom) {
		dom.addEventListener('click', (e) => {
			e.preventDefault()
			if ((e.ctrlKey || e.metaKey) && dom.href) window.open(dom.href, '_blank', 'noopener')
		})
	}
}

export class IssueChipView extends AtomView {
	constructor(node, view, getPos, ctx) {
		super(node, ctx)
		this.dom = el('a', { class: 'issue-chip', contenteditable: 'false' })
		this.follow(this.dom)
		this.render()
	}
	render() {
		const { key } = this.node.attrs
		const info = this.ctx.chips()[key]
		const d = this.dom
		d.dataset.key = key
		d.href = '/' + key
		d.title = 'Ctrl+click to open ' + key
		if (info?.state) d.dataset.state = info.state
		else delete d.dataset.state
		const kids = []
		if (info?.type && GLYPHS[info.type]) kids.push(typeIcon(info.type))
		kids.push(el('span', { class: 'issue-chip__key' }, key))
		if (info?.state === 'deleted') kids.push(el('span', { class: 'issue-chip__summary' }, 'deleted issue'))
		else if (info?.state === 'unknown') kids.push(el('span', { class: 'issue-chip__summary' }, 'unknown issue'))
		else if (info) {
			if (info.summary) kids.push(el('span', { class: 'issue-chip__summary' }, info.summary))
			if (info.status) kids.push(el('span', { class: 'tk-lozenge issue-chip__status', 'data-cat': info.category || 'todo' }, info.status))
		}
		d.replaceChildren(...kids)
	}
}

export class MentionView extends AtomView {
	constructor(node, view, getPos, ctx) {
		super(node, ctx)
		this.dom = el('a', { class: 'mention', contenteditable: 'false' })
		this.follow(this.dom)
		this.render()
	}
	render() {
		const { personId, label } = this.node.attrs
		const name = this.ctx.chips().people?.[personId]?.name || label
		this.dom.dataset.person = personId
		this.dom.href = '/people/' + encodeURIComponent(personId)
		this.dom.textContent = '@' + name
	}
}

export class PageLinkView extends AtomView {
	constructor(node, view, getPos, ctx) {
		super(node, ctx)
		this.dom = el('a', { class: 'page-link', contenteditable: 'false' })
		this.follow(this.dom)
		this.render()
	}
	render() {
		const { pageId } = this.node.attrs
		const info = this.ctx.chips().pages?.[pageId]
		const d = this.dom
		d.dataset.page = pageId
		if (info?.state === 'gone') {
			d.removeAttribute('href')
			d.dataset.state = 'gone'
			d.textContent = 'a removed page'
		} else {
			d.href = '/pages/' + encodeURIComponent(pageId)
			delete d.dataset.state
			d.textContent = info?.title || 'a page'
		}
	}
}

// task_item: the checkbox is clickable and toggles `checked` with one attr
// step; the content lives in the div, as in the reader view.
export class TaskItemView {
	constructor(node, view, getPos, ctx) {
		this.node = node
		this.box = el('input', { type: 'checkbox', 'aria-label': 'Done', contenteditable: 'false' })
		this.box.addEventListener('mousedown', (e) => e.preventDefault())
		this.box.addEventListener('click', (e) => {
			e.preventDefault()
			const pos = getPos()
			if (pos == null || !ctx.editable()) return
			view.dispatch(view.state.tr.setNodeAttribute(pos, 'checked', !this.node.attrs.checked))
		})
		this.contentDOM = el('div')
		this.dom = el('li', { class: 'task-item' }, this.box, this.contentDOM)
		this.render()
	}
	render() {
		this.dom.dataset.checked = this.node.attrs.checked ? 'true' : 'false'
		this.box.checked = this.node.attrs.checked
	}
	update(node) {
		if (node.type !== this.node.type) return false
		this.node = node
		this.render()
		return true
	}
	stopEvent(e) {
		return e.target === this.box
	}
	ignoreMutation(m) {
		return m.type !== 'selection' && !this.contentDOM.contains(m.target)
	}
}

// image: an atom block; "Alt text" edits alt with one attr step.
export class ImageView {
	constructor(node, view, getPos, ctx) {
		this.node = node
		this.img = el('img', { loading: 'lazy' })
		this.btn = el('button', { type: 'button', class: 'doc-image__alt', contenteditable: 'false' }, 'Alt text')
		this.btn.addEventListener('mousedown', (e) => e.preventDefault())
		this.btn.addEventListener('click', (e) => {
			e.preventDefault()
			const pos = getPos()
			if (pos != null && ctx.editable()) ctx.editAlt(pos)
		})
		this.dom = el('figure', { class: 'doc-image' }, this.img, this.btn)
		this.render()
	}
	render() {
		const { imageId, alt } = this.node.attrs
		const src = imageSrc(imageId)
		if (this.img.getAttribute('src') !== src) this.img.src = src
		this.img.alt = alt
		this.btn.setAttribute('aria-label', alt ? `Alt text: ${alt}. Change it` : 'Add alt text')
	}
	update(node) {
		if (node.type !== this.node.type) return false
		this.node = node
		this.render()
		return true
	}
	selectNode() {
		this.dom.classList.add('ProseMirror-selectednode')
	}
	deselectNode() {
		this.dom.classList.remove('ProseMirror-selectednode')
	}
	stopEvent(e) {
		return this.btn.contains(e.target)
	}
	ignoreMutation() {
		return true
	}
}

// issue_list: an atom block with a form for its filter (a saved filter, or
// one typed as the issue list's query string, and a row cap) whose changes
// are attr steps, and the rows the server renders for it now into the
// page's lists element (ctx.list), which a stream view keeps live.
export class IssueListView {
	constructor(node, view, getPos, ctx) {
		this.node = node
		this.view = view
		this.getPos = getPos
		this.ctx = ctx
		ctx.lists.add(this)
		const field = (text, input) => el('label', { class: 'issue-list__field' }, el('span', {}, text), input)
		this.saved = el('select', { 'aria-label': 'Saved filter' })
		this.filter = el('input', { type: 'text', 'aria-label': 'Filter', placeholder: 'status=todo&project=WEB', maxlength: String(limits.listFilter), autocomplete: 'off', spellcheck: 'false' })
		this.limit = el('input', { type: 'number', 'aria-label': 'Rows', min: '1', max: String(limits.listRows), step: '1' })
		this.form = el('div', { class: 'issue-list__form' }, field('Saved filter', this.saved), field('or a filter', this.filter), field('Rows', this.limit))
		this.desc = el('p', { class: 'issue-list__desc' })
		this.rows = el('div', { class: 'issue-list__rows' })
		this.dom = el('div', { class: 'issue-list issue-list--edit', contenteditable: 'false' }, this.desc, this.form, this.rows)
		this.saved.addEventListener('change', () => this.set({ savedFilter: this.saved.value, filter: this.saved.value ? '' : this.node.attrs.filter }))
		let timer = 0
		this.filter.addEventListener('input', () => {
			clearTimeout(timer)
			timer = setTimeout(() => this.set({ filter: this.filter.value, savedFilter: '' }), 500)
		})
		this.filter.addEventListener('change', () => (clearTimeout(timer), this.set({ filter: this.filter.value, savedFilter: '' })))
		this.limit.addEventListener('change', () => this.set({ limit: this.limit.value }))
		this.form.addEventListener('keydown', (e) => {
			if (e.key === 'Enter') e.preventDefault(), e.target.dispatchEvent(new Event('change'))
			if (e.key === 'Escape') e.preventDefault(), view.focus()
		})
		// A row's link opens with Ctrl or Cmd, as a chip's does.
		this.rows.addEventListener('click', (e) => {
			const a = e.target.closest('a[href]')
			if (!a) return
			e.preventDefault()
			if (e.ctrlKey || e.metaKey) window.open(a.href, '_blank', 'noopener')
		})
		this.render()
		if (ctx.focusList) (ctx.focusList = false), requestAnimationFrame(() => this.filter.focus())
	}
	// set changes the node's attributes, one attr step each.
	set(changes) {
		const pos = this.getPos()
		if (pos == null || !this.ctx.editable()) return
		const next = listAttrs({ ...this.node.attrs, ...changes })
		const tr = this.view.state.tr
		for (const k of ['filter', 'savedFilter', 'limit']) if (next[k] !== this.node.attrs[k]) tr.setNodeAttribute(pos, k, next[k])
		if (tr.docChanged) this.view.dispatch(tr)
	}
	render() {
		const a = this.node.attrs
		const filters = this.ctx.filters()
		const active = document.activeElement
		const opts = [['', 'None']].concat(filters.map((f) => [f.id, f.name]))
		if (a.savedFilter && !filters.some((f) => f.id === a.savedFilter)) opts.push([a.savedFilter, 'A saved filter'])
		const sig = JSON.stringify(opts)
		if (sig !== this.optsSig) {
			this.optsSig = sig
			this.saved.replaceChildren(...opts.map(([v, t]) => new Option(t, v)))
		}
		this.saved.value = a.savedFilter
		if (active !== this.filter) this.filter.value = a.filter
		if (active !== this.limit) this.limit.value = String(a.limit)
		const off = !this.ctx.editable()
		for (const i of [this.saved, this.filter, this.limit]) i.disabled = off
		const name = filters.find((f) => f.id === a.savedFilter)?.name ?? ''
		this.desc.replaceChildren(...listDescription(a, name).map((p) => (typeof p === 'string' ? p : el(p[0], {}, p[1]))))
		const src = this.ctx.list(this.key())
		if (src) this.rows.replaceChildren(...[...src.childNodes].map((n) => n.cloneNode(true)))
		else this.rows.replaceChildren(el('p', { class: 'issue-list__wait muted' }, this.ctx.listsShown() ? 'Loading the issues…' : 'The issues show when the page is read.'))
	}
	key() {
		const a = this.node.attrs
		return a.savedFilter + '|' + a.filter + '|' + a.limit
	}
	update(node) {
		if (node.type !== this.node.type) return false
		this.node = node
		this.render()
		return true
	}
	selectNode() {
		this.dom.classList.add('ProseMirror-selectednode')
	}
	deselectNode() {
		this.dom.classList.remove('ProseMirror-selectednode')
	}
	stopEvent(e) {
		return this.form.contains(e.target) || (e.type === 'click' && this.rows.contains(e.target))
	}
	ignoreMutation() {
		return true
	}
	destroy() {
		this.ctx.lists.delete(this)
	}
}
