// The fake server for the browser harness (lith-coedit's Authority), in a SharedWorker so that every
// tab (a separate page, as separate people are) talks to one authority.
// Each page id has its own Authority. Every 50 ms (lith's tick) it renders a
// frame for each tab whose view changed and posts it to that tab's port.
import { schema } from '../../src/schema.js'
import { Authority } from 'lith-coedit/test/authority/authority.js'

const TICK = 50
const pages = new Map() // page -> {auth, ports: Map(tab -> port)}

const seed = {
	type: 'doc',
	content: [
		{ type: 'heading', attrs: { level: 1 }, content: [{ type: 'text', text: 'Release notes' }] },
		{ type: 'paragraph', content: [{ type: 'text', text: 'Type here: ' }] },
		{ type: 'paragraph', content: [{ type: 'text', text: 'See ' }, { type: 'issue_chip', attrs: { key: 'WEB-12' } }, { type: 'text', text: ' and ' }, { type: 'page_link', attrs: { pageId: 'pg_roadmap' } }, { type: 'text', text: '.' }] },
		{ type: 'task_list', content: [{ type: 'task_item', attrs: { checked: false }, content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Ship it' }] }] }] },
	],
}
const chips = {
	'WEB-12': { summary: 'Fix the login', status: 'In Review', category: 'progress', type: 'bug' },
	'WEB-99': { state: 'deleted' },
	pages: { pg_roadmap: { title: 'Roadmap' } },
	people: {},
}

function pageOf(page) {
	let p = pages.get(page)
	if (!p) {
		p = { auth: new Authority({ schema, doc: seed, page, attrs: { chips: structuredClone(chips) } }), ports: new Map() }
		pages.set(page, p)
	}
	return p
}

setInterval(() => {
	for (const { auth, ports } of pages.values())
		for (const t of auth.tabs.values()) {
			if (!t.dirty) continue
			const port = ports.get(t.tab)
			const frame = auth.frame(t.tab)
			if (port && frame) port.postMessage({ type: 'frame', frame })
		}
}, TICK)

function handle(port, m) {
	const p = pageOf(m.page)
	const { auth } = p
	switch (m.type) {
		case 'open':
			p.ports.set(m.tab, port)
			return auth.open(m.tab, m)
		case 'close':
			p.ports.delete(m.tab)
			return auth.close(m.tab)
		case 'commit':
			return auth.commit(m.body)
		case 'presence':
			return auth.presence(m.body)
		case 'reset':
			return auth.reset(m.doc), auth.epoch
		case 'fold':
			return auth.fold(m.upto ?? auth.version)
		case 'force': {
			const t = auth.tabs.get(m.tab)
			if (t) (t.force = m.state || null), auth.touch()
			return !!t
		}
		case 'refuse':
			// Refuse every commit whose document holds this text, with message.
			auth.refuse = m.text ? (doc) => (doc.textContent.includes(m.text) ? m.message : '') : null
			return true
		case 'touch':
			// A stream that opens again gets a frame at once.
			return auth.touch()
		case 'chips':
			auth.attrs.chips = m.chips
			return auth.touch()
		case 'state':
			return {
				epoch: auth.epoch,
				version: auth.version,
				doc: auth.doc.toJSON(),
				stats: auth.stats,
				refs: [...auth.commits.reduce((m, c) => m.set(c.ref, (m.get(c.ref) ?? 0) + 1), new Map())].filter(([, n]) => n > 1),
				commits: auth.commits.length,
			}
	}
	throw new Error('unknown message ' + m.type)
}

self.onconnect = (e) => {
	const port = e.ports[0]
	port.onmessage = ({ data }) => {
		let result, error
		try {
			result = handle(port, data)
		} catch (err) {
			error = String(err?.stack || err)
		}
		port.postMessage({ type: 'reply', id: data.id, result, error })
	}
	port.start()
}
