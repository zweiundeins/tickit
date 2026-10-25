// The ProseMirror schema of tickit's documents, built from the one shared
// spec (internal/docnodes/schema.json, bundled at build time) plus the DOM rules
// of docs/editor.md's "HTML of a document" table. The Go renderer writes the
// same elements, so prose.css styles the editor and the reader view alike.
//
// Nothing here touches the DOM until ProseMirror parses or serialises, so the
// test authority builds the same schema in a worker.
import { Schema } from 'prosemirror-model'
import { schemaSpec } from 'coedit'
import spec from '../../internal/docnodes/schema.json' with { type: 'json' }

export { spec }
export const limits = spec.limits

export const ISSUE_KEY = /^[A-Z]{2,10}-[1-9][0-9]{0,6}$/
export const ID = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/
const LIB_IMAGE = /^\/img\/lib\/([A-Za-z0-9][A-Za-z0-9._-]{0,127})$/
const SCHEMES = new Set(limits.linkSchemes.map((s) => s + ':'))
const LANGS = new Set(limits.codeLanguages)
const ALIASES = { javascript: 'js', jsx: 'js', typescript: 'ts', tsx: 'ts', golang: 'go', sh: 'bash', shell: 'bash', zsh: 'bash', py: 'python', yml: 'yaml', md: 'markdown', htm: 'html', patch: 'diff', plain: 'text', txt: 'text', plaintext: 'text', rs: 'rust' }
export const MAX_ALT = 500
const MAX_LABEL = 30

const clampInt = (raw, min, max, dflt) => {
	const n = Number.parseInt(raw, 10)
	return Number.isFinite(n) ? Math.min(max, Math.max(min, n)) : dflt
}

// A language from the allowlist, or ''.
export function language(raw) {
	const l = String(raw ?? '').trim().toLowerCase()
	const name = ALIASES[l] ?? l
	return LANGS.has(name) ? name : ''
}

// The href a link keeps: absolute http, https or mailto, at most 2,048
// characters (DM-06.2). Relative URLs resolve against base (the page), so a
// pasted reader view keeps its links. Returns '' for anything else.
export function normalizeHref(raw, base) {
	const s = String(raw ?? '').trim()
	if (!s || s.length > limits.attrLength) return ''
	let url
	try {
		url = base ? new URL(s, base) : new URL(s)
	} catch {
		return ''
	}
	if (!SCHEMES.has(url.protocol)) return ''
	if (url.protocol !== 'mailto:' && !url.hostname) return ''
	const href = url.href
	return href.length <= limits.attrLength ? href : ''
}

// isSafeHref is the check toDOM makes on a stored href: already normalised.
export const isSafeHref = (href) => typeof href === 'string' && normalizeHref(href) === href

// What a link shows on hover (DM-06.2): its domain, or the mail address.
export function linkTitle(href) {
	try {
		const url = new URL(href)
		return url.protocol === 'mailto:' ? decodeURIComponent(url.pathname) : url.host
	} catch {
		return ''
	}
}

const docBase = () => (typeof document === 'undefined' ? undefined : document.baseURI)
const sameOrigin = (url) => typeof location !== 'undefined' && url.origin === location.origin

// The library image a src names: /img/lib/<id> on this site, or null.
export function libraryImage(src) {
	if (!src) return null
	let url
	try {
		url = new URL(src, docBase())
	} catch {
		return null
	}
	if (!sameOrigin(url)) return null
	return LIB_IMAGE.exec(url.pathname)?.[1] ?? null
}

export const imageSrc = (imageId) => '/img/lib/' + encodeURIComponent(imageId)

// Labels for atoms whose text comes from the server (page titles, people's
// names): node views read the editor's chips attribute; toDOM (copy) asks
// this hook, which the editor sets.
const labels = { page: () => '', person: () => '' }
export function setLabelHooks(hooks) {
	Object.assign(labels, hooks)
}

const cellAttrs = (dom) => ({
	colspan: clampInt(dom.getAttribute('colspan'), 1, limits.tableColumns, 1),
	rowspan: clampInt(dom.getAttribute('rowspan'), 1, limits.tableRows, 1),
	colwidth: null,
})
const cellDOM = (node) => {
	const a = {}
	if (node.attrs.colspan !== 1) a.colspan = node.attrs.colspan
	if (node.attrs.rowspan !== 1) a.rowspan = node.attrs.rowspan
	return a
}

const imageAttrs = (img) => {
	const id = img && libraryImage(img.getAttribute('src'))
	if (!id) return false
	return { imageId: id, src: imageSrc(id), alt: (img.getAttribute('alt') ?? '').slice(0, MAX_ALT) }
}

const codeLanguage = (pre) =>
	language(pre.getAttribute('data-language')) ||
	language(/(?:^|\s)(?:language|lang)-(\S+)/.exec((pre.className || '') + ' ' + (pre.querySelector('code')?.className || ''))?.[1])

export const SAVED_FILTER = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

// An issue list's attributes as the server's rules take them (FR-104): a
// typed filter is the issue list's query string, a pasted /issues address
// gives its query.
export function listAttrs({ filter = '', savedFilter = '', limit = 10 } = {}) {
	let f = String(filter ?? '').trim()
	const q = f.indexOf('?')
	if (q >= 0 && /^(https?:\/\/[^/]*)?\/issues\b/.test(f)) f = f.slice(q + 1)
	// Go's url.ParseQuery refuses a stray % and a semicolon.
	f = f.replace(/^\?/, '').replace(/[\u0000-\u001f\u007f]/g, '').replace(/%(?![0-9a-fA-F]{2})/g, '%25').replace(/;/g, '%3B').slice(0, limits.listFilter)
	const id = String(savedFilter ?? '')
	return { filter: f, savedFilter: SAVED_FILTER.test(id) ? id : '', limit: clampInt(limit, 1, limits.listRows, 10) }
}

// What an issue list shows in place of its rows, as the server's
// doc.ListDescription writes it.
export function listDescription(attrs, name = '') {
	const tail = ', at most ' + attrs.limit
	if (attrs.savedFilter && name) return ['Issues of the saved filter ', ['strong', name], tail]
	if (attrs.savedFilter) return ['Issues of a saved filter' + tail]
	if (attrs.filter) return ['Issues matching ', ['code', attrs.filter], tail]
	return ['An issue list without a filter' + tail]
}

const mentionAttrs = (dom) => {
	const personId = dom.getAttribute('data-person')
	const label = (dom.textContent || '').trim().replace(/^@/, '').slice(0, MAX_LABEL)
	return ID.test(personId ?? '') && label.length >= 2 ? { personId, label } : false
}

const DOM = {
	nodes: {
		paragraph: { parseDOM: [{ tag: 'p' }], toDOM: () => ['p', 0] },
		heading: {
			// h1 (pasted) and h2 are level 1; the page title is the page's own h1.
			parseDOM: [1, 2, 3, 4, 5, 6].map((n) => ({ tag: 'h' + n, attrs: { level: Math.min(3, Math.max(1, n - 1)) } })),
			toDOM: (node) => ['h' + (clampInt(node.attrs.level, 1, 3, 1) + 1), 0],
		},
		blockquote: { parseDOM: [{ tag: 'blockquote' }], toDOM: () => ['blockquote', 0] },
		code_block: {
			parseDOM: [{ tag: 'pre', preserveWhitespace: 'full', getAttrs: (dom) => ({ language: codeLanguage(dom) }) }],
			toDOM: (node) => ['pre', node.attrs.language ? { 'data-language': node.attrs.language } : {}, ['code', 0]],
		},
		horizontal_rule: { parseDOM: [{ tag: 'hr' }], toDOM: () => ['hr'] },
		bullet_list: { parseDOM: [{ tag: 'ul' }], toDOM: () => ['ul', 0] },
		ordered_list: {
			parseDOM: [{ tag: 'ol', getAttrs: (dom) => ({ order: clampInt(dom.getAttribute('start'), 1, 9999, 1) }) }],
			toDOM: (node) => (node.attrs.order === 1 ? ['ol', 0] : ['ol', { start: node.attrs.order }, 0]),
		},
		list_item: { parseDOM: [{ tag: 'li' }], toDOM: () => ['li', 0] },
		task_list: {
			parseDOM: [{ tag: 'ul.task-list', priority: 60 }, { tag: 'ul.contains-task-list', priority: 60 }],
			toDOM: () => ['ul', { class: 'task-list' }, 0],
		},
		task_item: {
			parseDOM: [
				{
					tag: 'li.task-item',
					priority: 60,
					contentElement: (dom) => dom.querySelector(':scope > div') ?? dom,
					getAttrs: (dom) => ({ checked: dom.getAttribute('data-checked') === 'true' }),
				},
				// GitHub's rendering of "- [x] text".
				{ tag: 'li.task-list-item', priority: 60, getAttrs: (dom) => ({ checked: !!dom.querySelector('input[type=checkbox]')?.checked }) },
			],
			toDOM: (node) => [
				'li',
				{ class: 'task-item', 'data-checked': node.attrs.checked ? 'true' : 'false' },
				['input', node.attrs.checked ? { type: 'checkbox', disabled: '', checked: '' } : { type: 'checkbox', disabled: '' }],
				['div', 0],
			],
		},
		table: { parseDOM: [{ tag: 'table' }], toDOM: () => ['table', ['tbody', 0]] },
		table_row: { parseDOM: [{ tag: 'tr' }], toDOM: () => ['tr', 0] },
		table_cell: { parseDOM: [{ tag: 'td', getAttrs: cellAttrs }], toDOM: (node) => ['td', cellDOM(node), 0] },
		table_header: { parseDOM: [{ tag: 'th', getAttrs: cellAttrs }], toDOM: (node) => ['th', cellDOM(node), 0] },
		image: {
			parseDOM: [
				{ tag: 'figure.doc-image', getAttrs: (dom) => imageAttrs(dom.querySelector('img')) },
				{ tag: 'img[src]', getAttrs: imageAttrs },
			],
			// The URL comes from imageId alone, never from src (DM-06.3).
			toDOM: (node) => ['figure', { class: 'doc-image' }, ['img', { src: imageSrc(node.attrs.imageId), alt: node.attrs.alt, loading: 'lazy' }]],
		},
		issue_list: {
			parseDOM: [
				{
					tag: 'div.issue-list',
					priority: 60,
					getAttrs: (dom) => listAttrs({ filter: dom.getAttribute('data-filter'), savedFilter: dom.getAttribute('data-saved-filter'), limit: dom.getAttribute('data-limit') }),
				},
			],
			toDOM: (node) => [
				'div',
				{ class: 'issue-list', 'data-filter': node.attrs.filter, 'data-saved-filter': node.attrs.savedFilter, 'data-limit': String(node.attrs.limit) },
				['p', { class: 'issue-list__desc' }, ...listDescription(node.attrs)],
			],
		},
		hard_break: { parseDOM: [{ tag: 'br' }], toDOM: () => ['br'] },
		issue_chip: {
			parseDOM: [
				{
					tag: 'a.issue-chip[data-key]',
					priority: 60,
					getAttrs: (dom) => (ISSUE_KEY.test(dom.getAttribute('data-key') ?? '') ? { key: dom.getAttribute('data-key') } : false),
				},
			],
			toDOM: (node) => ['a', { class: 'issue-chip', 'data-key': node.attrs.key, href: '/' + node.attrs.key }, node.attrs.key],
		},
		mention: {
			parseDOM: [{ tag: 'a.mention[data-person]', priority: 60, getAttrs: mentionAttrs }],
			toDOM: (node) => [
				'a',
				{ class: 'mention', 'data-person': node.attrs.personId, href: '/people/' + encodeURIComponent(node.attrs.personId) },
				'@' + (labels.person(node.attrs.personId) || node.attrs.label),
			],
		},
		page_link: {
			parseDOM: [
				{
					tag: '.page-link[data-page]',
					priority: 60,
					getAttrs: (dom) => (ID.test(dom.getAttribute('data-page') ?? '') ? { pageId: dom.getAttribute('data-page') } : false),
				},
			],
			toDOM: (node) => [
				'a',
				{ class: 'page-link', 'data-page': node.attrs.pageId, href: '/pages/' + encodeURIComponent(node.attrs.pageId) },
				labels.page(node.attrs.pageId) || 'a page',
			],
		},
	},
	marks: {
		link: {
			parseDOM: [
				{
					tag: 'a[href]',
					getAttrs: (dom) => {
						const href = normalizeHref(dom.getAttribute('href'), docBase())
						return href ? { href } : false
					},
				},
			],
			toDOM: (mark) =>
				isSafeHref(mark.attrs.href)
					? ['a', { href: mark.attrs.href, rel: 'nofollow ugc noopener', title: linkTitle(mark.attrs.href) }, 0]
					: ['span', { class: 'link-unsafe' }, 0],
		},
		strong: {
			parseDOM: [
				{ tag: 'strong' },
				// Google Docs wraps everything in <b style="font-weight:normal">.
				{ tag: 'b', getAttrs: (dom) => dom.style.fontWeight !== 'normal' && null },
				{ style: 'font-weight=400', clearMark: (m) => m.type.name === 'strong' },
				{ style: 'font-weight', getAttrs: (v) => /^(bold(er)?|[5-9]\d{2,})$/.test(v) && null },
			],
			toDOM: () => ['strong', 0],
		},
		em: {
			parseDOM: [{ tag: 'em' }, { tag: 'i' }, { style: 'font-style=italic' }, { style: 'font-style=normal', clearMark: (m) => m.type.name === 'em' }],
			toDOM: () => ['em', 0],
		},
		strike: {
			parseDOM: [
				{ tag: 's' },
				{ tag: 'del' },
				{ tag: 'strike' },
				{ style: 'text-decoration', getAttrs: (v) => v.includes('line-through') && null },
				{ style: 'text-decoration-line', getAttrs: (v) => v.includes('line-through') && null },
			],
			toDOM: () => ['s', 0],
		},
		code: { parseDOM: [{ tag: 'code' }], toDOM: () => ['code', 0] },
	},
}

// buildSchema returns the schema of spec; with dom false, without the DOM
// rules (what the server sees).
export function buildSchema({ dom = true } = {}) {
	return new Schema(schemaSpec(spec, dom ? DOM : {}))
}

export const schema = buildSchema()
