// Node views for the atoms whose text the server knows (FR-100, FR-103,
// FR-67), the clickable task checkbox and images with alt text (FR-84).
// Chips, mentions and page links read the editor's chips attribute:
//   {"WEB-12": {summary, status, category, type} | {state: "deleted"|"unknown"},
//    "people": {"<id>": {name}}, "pages": {"<id>": {title} | {state: "gone"}}}
// so a changed issue shows in every open editor with the next frame.
import { imageSrc } from './schema.js'

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
