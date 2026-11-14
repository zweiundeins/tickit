// DM-05: the editor stops input past the page's limits and says why. The
// writer refuses the same commits; measuring here keeps people from typing
// what would be refused. Sizes are measured per top-level block and cached by
// node, so a keystroke costs one block's measure.
import { Plugin } from 'prosemirror-state'
import { TableMap } from 'prosemirror-tables'
import { REMOTE } from 'lith-coedit'
import { limits } from './schema.js'

// UTF-8 length of a string, as the stored JSON counts it.
export function utf8Length(s) {
	let n = 0
	for (let i = 0; i < s.length; i++) {
		const c = s.charCodeAt(i)
		if (c < 0x80) n += 1
		else if (c < 0x800) n += 2
		else if (c >= 0xd800 && c <= 0xdbff && i + 1 < s.length) (n += 4), i++
		else n += 3
	}
	return n
}

const cache = new WeakMap()

// measureBlock returns {bytes, blocks, rows, cols} of a top-level node: its
// JSON's UTF-8 bytes, the block nodes in it (itself included), and the
// largest table inside it.
function measureBlock(node) {
	let m = cache.get(node)
	if (m) return m
	m = { bytes: utf8Length(JSON.stringify(node.toJSON())), blocks: 1, rows: 0, cols: 0 }
	const visit = (n) => {
		if (n.type.spec.tableRole === 'table') {
			const map = TableMap.get(n)
			m.rows = Math.max(m.rows, map.height)
			m.cols = Math.max(m.cols, map.width)
		}
	}
	visit(node)
	node.descendants((n) => {
		if (n.isBlock) m.blocks++
		visit(n)
		return !n.isTextblock
	})
	cache.set(node, m)
	return m
}

// measure returns the document's measures. bytes is the length of
// JSON.stringify(doc.toJSON()) in UTF-8; blocks counts every block node but
// the doc itself.
export function measure(doc) {
	const out = { bytes: '{"type":"doc","content":[]}'.length, blocks: 0, rows: 0, cols: 0 }
	doc.forEach((child, _, i) => {
		const m = measureBlock(child)
		out.bytes += m.bytes + (i ? 1 : 0)
		out.blocks += m.blocks
		out.rows = Math.max(out.rows, m.rows)
		out.cols = Math.max(out.cols, m.cols)
	})
	return out
}

// limitList is DM-05's limits for l: schema.json's, or with a private
// instance's sizes, which the host's limits attribute brings (PI-18).
export function limitList(l = limits) {
	return [
		{ name: 'size', key: 'bytes', max: l.storedBytes, message: `The page is at its limit of ${Math.round(l.storedBytes / 1024).toLocaleString('en')} KB. Remove something to add more.` },
		{ name: 'blocks', key: 'blocks', max: l.blocks, message: `The page is at its limit of ${l.blocks.toLocaleString('en')} blocks (paragraphs, headings, list items and the like).` },
		{ name: 'table-rows', key: 'rows', max: l.tableRows, message: `A table holds at most ${l.tableRows} rows.` },
		{ name: 'table-columns', key: 'cols', max: l.tableColumns, message: `A table holds at most ${l.tableColumns} columns.` },
	]
}

export const LIMITS = limitList()

// The limit a document passes that the one before stayed within (or grew
// less past), or null. A change that makes nothing worse always passes, so a
// page over a limit can still be cut down.
export function exceeded(before, after, list = LIMITS) {
	const a = measure(after)
	for (const l of list) {
		if (a[l.key] <= l.max) continue
		if (a[l.key] > measure(before)[l.key]) return { limit: l.name, max: l.max, value: a[l.key], message: l.message }
	}
	return null
}

// limitsPlugin refuses local transactions past a limit and calls report.
// Commits from the writer always pass.
export function limitsPlugin(report, list = LIMITS) {
	return new Plugin({
		filterTransaction(tr, state) {
			if (!tr.docChanged || tr.getMeta(REMOTE)) return true
			const hit = exceeded(state.doc, tr.doc, list)
			if (hit) report(hit)
			return !hit
		},
	})
}
