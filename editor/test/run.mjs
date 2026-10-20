// Browser tests of sb-rich-editor against the fake authority, in headless
// Chromium through puppeteer-core (CHROME=/path, default /usr/bin/chromium).
// Every tab is a page of its own: its own Datastar, its own component, its
// own focus, so five pages type at once as five people would.
//
//   node test/run.mjs [name-filter]     CONVERGE_SECONDS=45 EDITORS=5
//
// Writes test/out/results.json. Nothing here is Chromium-only except the
// launcher: puppeteer-core drives Firefox through WebDriver BiDi too
// (BROWSER=firefox CHROME=/path/to/firefox).
import { build } from 'esbuild'
import puppeteer from 'puppeteer-core'
import { existsSync, mkdirSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { startServer } from './server.mjs'

const here = dirname(fileURLToPath(import.meta.url))
const filter = process.argv[2] ?? ''
const CHROME = process.env.CHROME ?? '/usr/bin/chromium'
if (!existsSync(CHROME)) {
	console.log(`skip: no browser at ${CHROME}`)
	process.exit(0)
}
if (!existsSync(join(here, '../../internal/web/static/editor/sb-rich-editor.min.js'))) {
	console.error('run `node build.mjs` first')
	process.exit(1)
}
mkdirSync(join(here, 'out'), { recursive: true })
await build({ entryPoints: [join(here, 'authority/worker.js')], bundle: true, format: 'iife', target: 'es2022', outfile: join(here, 'out/authority.js'), logLevel: 'warning' })

const { server, url: base } = await startServer()
const browser = await puppeteer.launch({
	executablePath: CHROME,
	browser: process.env.BROWSER ?? 'chrome',
	headless: true,
	args: ['--no-sandbox', '--no-first-run', '--disable-background-timer-throttling', '--disable-renderer-backgrounding', '--disable-backgrounding-occluded-windows'],
})
const context = browser.defaultBrowserContext()
await context.setPermission(base, { permission: { name: 'clipboard-read' }, state: 'granted' }, { permission: { name: 'clipboard-write' }, state: 'granted' }).catch(() => {})

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
let pageSeq = 0
const results = {}

// open starts n tabs on a new page id and waits for their editors.
async function open(n, { skip = 0, delay = 25, csp = '', names } = {}) {
	const page = `p${++pageSeq}_${Date.now().toString(36)}`
	const tabs = []
	for (let i = 0; i < n; i++) {
		const p = await browser.newPage()
		await p.emulateFocusedPage?.(true)
		p.on('console', (m) => ['error', 'warn'].includes(m.type()) && !/favicon/.test(m.text()) && console.log(`  [tab ${i} ${m.type()}] ${m.text()}`))
		p.on('pageerror', (e) => console.log(`  [tab ${i} pageerror] ${e.message}`))
		const name = names?.[i] ?? ['Sunny Otter', 'Brave Lynx', 'Quiet Heron', 'Bold Marten', 'Calm Ibex', 'Swift Wren', 'Keen Vole'][i % 7]
		await p.goto(`${base}/test/tab.html?page=${page}&tab=t${i}&name=${encodeURIComponent(name)}&hue=${(i * 3) % 16}&skip=${skip}&delay=${delay}${csp ? '&csp=' + csp : ''}`)
		await p.evaluate(() => harness.ready)
		await p.waitForFunction(() => harness.lastFrame, { polling: 20 })
		tabs.push(p)
	}
	return { page, tabs, close: () => Promise.all(tabs.map((t) => t.close())) }
}

const authority = (tab, msg) => tab.evaluate((m) => harness.rpc(m), msg)
const docOf = (tab) => tab.evaluate(() => JSON.stringify(harness.host().json()))

// settled waits until every tab holds the authority's document with nothing
// unsaved, and returns the time it took.
async function settled(tabs, timeout = 10000) {
	const t0 = Date.now()
	for (;;) {
		const st = await authority(tabs[0], { type: 'state' })
		const want = JSON.stringify(st.doc)
		const docs = await Promise.all(tabs.map(docOf))
		const saving = await Promise.all(tabs.map((t) => t.evaluate(() => harness.host().editor.status)))
		if (docs.every((d) => d === want) && saving.every((s) => s === 'saved')) return { ms: Date.now() - t0, state: st, docs }
		if (Date.now() - t0 > timeout) return { ms: -1, state: st, docs, saving }
		await sleep(50)
	}
}

// caretIn puts the tab's caret into the text of top-level block i at offset.
const caretIn = (tab, i, offset = Infinity) =>
	tab.evaluate(
		(i, offset) => {
			const v = harness.host().view
			let pos = 0
			for (let k = 0; k < i; k++) pos += v.state.doc.child(k).nodeSize
			const node = v.state.doc.child(i)
			const at = pos + 1 + Math.min(offset === null ? node.content.size : offset, node.content.size)
			v.focus()
			v.dispatch(v.state.tr.setSelection(v.state.selection.constructor.near(v.state.doc.resolve(at))))
		},
		i,
		offset === Infinity ? null : offset,
	)

const tests = []
const test = (name, fn) => tests.push({ name, fn })
const ok = (cond, msg) => {
	if (!cond) throw new Error(msg)
}

test('smoke: one editor types and its commit is confirmed', async () => {
	const { tabs, close } = await open(1)
	const [t] = tabs
	await caretIn(t, 1)
	await t.keyboard.type('hello', { delay: 30 })
	const s = await settled(tabs)
	ok(s.ms >= 0, 'settles')
	ok(s.state.doc.content[1].content[0].text === 'Type here: hello', 'authority has the text: ' + JSON.stringify(s.state.doc.content[1]))
	const v = await t.evaluate(() => [harness.violations, harness.errors])
	ok(!v[0].length && !v[1].length, 'no CSP violations or errors: ' + JSON.stringify(v))
	await close()
})


// Letters the seed document does not hold, two per editor, so that every
// typed character can be counted in the final text.
const POOL = 'bcfgjkmquvwxzACDFGHIJKLMNOPQUVXYZ0123456789'
const alphabetOf = (i) => POOL.slice(2 * i, 2 * i + 2)
const pct = (xs, p) => {
	if (!xs.length) return null
	const s = [...xs].sort((a, b) => a - b)
	return Math.round(s[Math.min(s.length - 1, Math.floor((p / 100) * s.length))])
}

// type runs a typist: chars per second, for ms, in top-level block 1, moving
// its caret to a random place in the paragraph every so often.
async function typist(tab, alphabet, ms, cps) {
	const end = Date.now() + ms
	let n = 0
	while (Date.now() < end) {
		if (n % 25 === 0) await caretIn(tab, 1, Math.floor(Math.random() * 12))
		await tab.keyboard.type(alphabet[Math.floor(Math.random() * alphabet.length)])
		n++
		await sleep((1000 / cps) * (0.5 + Math.random()))
	}
	return n
}

async function converge({ editors, seconds, cps = 5, skip = 0, delay = 25 }) {
	const { tabs, close } = await open(editors, { skip, delay })
	const alphabets = tabs.map((_, i) => alphabetOf(i))
	await Promise.all(tabs.map((t) => t.evaluate((a) => harness.setAlphabets(a), alphabets)))
	await Promise.all(tabs.map((t) => caretIn(t, 1)))
	const t0 = Date.now()
	const counts = await Promise.all(tabs.map((t, i) => typist(t, alphabets[i], seconds * 1000, cps)))
	const typingMs = Date.now() - t0
	const s = await settled(tabs, 15000)
	ok(s.ms >= 0, 'every editor settles on the writer\'s document: ' + JSON.stringify(s.saving))
	const text = s.state.doc.content.map((b) => (b.content ?? []).map((c) => c.text ?? '').join('')).join('')
	const per = await Promise.all(tabs.map((t) => t.evaluate(() => ({ typed: harness.typed.map((x) => x[0]).join(''), times: harness.typed.map((x) => x[1]), arrivals: harness.arrivals, stats: harness.stats, ed: harness.host().editor.stats, errors: harness.errors, violations: harness.violations }))))
	// Every typed character is in the document, as often as it was typed.
	per.forEach((p, i) => {
		ok(p.typed.length === counts[i], `tab ${i} recorded ${p.typed.length} keystrokes of ${counts[i]}`)
		for (const ch of alphabets[i]) {
			const want = [...p.typed].filter((c) => c === ch).length
			const got = [...text].filter((c) => c === ch).length
			ok(want === got, `tab ${i}: '${ch}' typed ${want} times, ${got} in the document`)
		}
		ok(!p.errors.length && !p.violations.length, `tab ${i} errors ${JSON.stringify(p.errors)} violations ${JSON.stringify(p.violations)}`)
	})
	// Keystroke to another editor: the n-th character of i's alphabet showing in j.
	const lat = []
	per.forEach((pj, j) =>
		per.forEach((pi, i) => {
			if (i === j) return
			const a = pj.arrivals[i] ?? []
			for (let n = 0; n < Math.min(a.length, pi.times.length); n++) lat.push(a[n] - pi.times[n])
		}),
	)
	const st = s.state.stats
	await close()
	return {
		editors,
		seconds,
		cps,
		skip,
		delayEachWay: delay,
		keystrokes: counts.reduce((a, b) => a + b, 0),
		settleMs: s.ms,
		latency: { samples: lat.length, p50: pct(lat, 50), p95: pct(lat, 95), max: pct(lat, 100) },
		commitsPerSecond: +(st.commits / (typingMs / 1000)).toFixed(1),
		writer: st,
		resends: per.reduce((n, p) => n + p.ed.resends, 0),
		resyncs: per.reduce((n, p) => n + p.ed.resyncs, 0),
		framesSkipped: per.reduce((n, p) => n + p.stats.framesSkipped, 0),
		framesApplied: per.reduce((n, p) => n + p.stats.framesApplied, 0),
		identical: true,
	}
}

const SECONDS = +(process.env.CONVERGE_SECONDS ?? 40)
const EDITORS = +(process.env.EDITORS ?? 5)

test(`converge: ${EDITORS} editors type in one paragraph for ${SECONDS} s (Appendix B done 1, MP-05.1, MP-05.3, MP-05.4)`, async () => {
	const r = await converge({ editors: EDITORS, seconds: SECONDS })
	console.log('     ', JSON.stringify(r))
	return r
})


// --- Helpers for the tests below ---------------------------------------------
const waitFor = async (tab, fn, arg, timeout = 5000) => {
	const t0 = Date.now()
	for (;;) {
		const v = await tab.evaluate(fn, arg)
		if (v) return v
		if (Date.now() - t0 > timeout) throw new Error('timed out waiting for ' + fn.toString().slice(0, 120))
		await sleep(30)
	}
}
const shadow = (tab, fn, arg) => tab.evaluate((src, arg) => new Function('root', 'host', 'arg', 'return (' + src + ')(root, host, arg)')(harness.host().shadowRoot, harness.host(), arg), fn.toString(), arg)
const blockText = (doc, i) => (doc.content[i].content ?? []).map((c) => c.text ?? '').join('')
// appendParagraph adds an empty paragraph at the end and puts the caret in it.
const appendParagraph = (tab) =>
	tab.evaluate(() => {
		const v = harness.host().view
		const tr = v.state.tr.insert(v.state.doc.content.size, v.state.schema.nodes.paragraph.create())
		tr.setSelection(v.state.selection.constructor.near(tr.doc.resolve(tr.doc.content.size - 1)))
		v.focus()
		v.dispatch(tr)
		return v.state.doc.childCount - 1
	})
const paste = (tab, data) =>
	tab.evaluate((data) => {
		const dt = new DataTransfer()
		for (const [type, value] of Object.entries(data)) dt.setData(type, value)
		harness.host().view.dom.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }))
	}, data)
const selectIn = (tab, i, from, to) =>
	tab.evaluate(
		(i, from, to) => {
			const v = harness.host().view
			let pos = 0
			for (let k = 0; k < i; k++) pos += v.state.doc.child(k).nodeSize
			v.focus()
			v.dispatch(v.state.tr.setSelection(v.state.selection.constructor.create(v.state.doc, pos + 1 + from, pos + 1 + to)))
		},
		i,
		from,
		to,
	)

test("toDOM writes docs/editor.md's HTML of a document", async () => {
	const { tabs, close } = await open(1)
	const [a] = tabs
	const html = await a.evaluate(() => {
		const v = harness.host().view
		const S = v.state.schema
		const t = (x, marks = []) => S.text(x, marks.map((m) => (typeof m === 'string' ? S.marks[m].create() : m)))
		const p = (...c) => S.nodes.paragraph.create(null, c)
		const doc = S.nodes.doc.create(null, [
			S.nodes.heading.create({ level: 1 }, t('One')),
			S.nodes.heading.create({ level: 3 }, t('Three')),
			S.nodes.blockquote.create(null, p(t('q'))),
			S.nodes.code_block.create({ language: 'go' }, t('x := 1')),
			S.nodes.code_block.create(null, t('plain')),
			S.nodes.horizontal_rule.create(),
			S.nodes.bullet_list.create(null, S.nodes.list_item.create(null, p(t('b')))),
			S.nodes.ordered_list.create({ order: 3 }, S.nodes.list_item.create(null, p(t('o')))),
			S.nodes.ordered_list.create(null, S.nodes.list_item.create(null, p(t('o1')))),
			S.nodes.task_list.create(null, [S.nodes.task_item.create({ checked: true }, p(t('done'))), S.nodes.task_item.create(null, p(t('todo')))]),
			S.nodes.table.create(null, S.nodes.table_row.create(null, [S.nodes.table_header.create({ colspan: 2 }, p(t('h'))), S.nodes.table_cell.create(null, p(t('c')))])),
			S.nodes.image.create({ imageId: 'diagram-1', src: '/img/lib/diagram-1', alt: 'A diagram' }),
			p(
				t('a'), S.nodes.hard_break.create(), S.nodes.issue_chip.create({ key: 'WEB-12' }), S.nodes.mention.create({ personId: 'u_1', label: 'Sunny Otter' }), S.nodes.page_link.create({ pageId: 'pg_roadmap' }),
				t('s', ['strong']), t('e', ['em']), t('k', ['strike']), t('c', ['code']), t('l', [S.marks.link.create({ href: 'https://example.com/x' })]), t('u', [S.marks.link.create({ href: 'javascript:alert(1)' })]),
			),
		])
		v.dispatch(v.state.tr.replaceWith(0, v.state.doc.content.size, doc.content))
		return harness.host().html()
	})
	const want = [
		'<h2>One</h2>', '<h4>Three</h4>', '<blockquote><p>q</p></blockquote>', '<pre data-language="go"><code>x := 1</code></pre>', '<pre><code>plain</code></pre>', '<hr>',
		'<ul><li><p>b</p></li></ul>', '<ol start="3"><li><p>o</p></li></ol>', '<ol><li><p>o1</p></li></ol>',
		'<ul class="task-list"><li class="task-item" data-checked="true"><input type="checkbox" disabled="" checked=""><div><p>done</p></div></li><li class="task-item" data-checked="false"><input type="checkbox" disabled=""><div><p>todo</p></div></li></ul>',
		'<table><tbody><tr><th colspan="2"><p>h</p></th><td><p>c</p></td></tr></tbody></table>',
		'<figure class="doc-image"><img src="/img/lib/diagram-1" alt="A diagram" loading="lazy"></figure>',
		'<p>a<br><a class="issue-chip" data-key="WEB-12" href="/WEB-12">WEB-12</a><a class="mention" data-person="u_1" href="/people/u_1">@Sunny Otter</a><a class="page-link" data-page="pg_roadmap" href="/pages/pg_roadmap">Roadmap</a><strong>s</strong><em>e</em><s>k</s><code>c</code><a href="https://example.com/x" rel="nofollow ugc noopener" title="example.com">l</a><span class="link-unsafe">u</span></p>',
	].join('')
	ok(html === want, 'HTML differs:\n got  ' + html + '\n want ' + want)
	await close()
})

test('reconnect: unconfirmed steps go out again with the same ref and apply once (MP-05.6, MP-10.3)', async () => {
	const { tabs, close } = await open(2)
	const [a] = tabs
	await caretIn(a, 1)
	// The post reaches the writer, the answer is lost, and no frame comes: the
	// editor sends the same commit again after a second.
	await a.evaluate(() => Object.assign(harness.mode, { loseResponse: true, frozen: true }))
	await a.keyboard.type('xq')
	await sleep(1600)
	// Then the connection goes; typing goes on.
	await a.evaluate(() => Object.assign(harness.mode, { offline: true, loseResponse: false }))
	await a.keyboard.type('zz')
	await sleep(2500)
	const unsavedOffline = await a.evaluate(() => harness.host().editor.status)
	await a.evaluate(() => harness.reconnect())
	const s = await settled(tabs)
	ok(s.ms >= 0, 'settles after the reconnect')
	ok(blockText(s.state.doc, 1) === 'Type here: xqzz', 'text kept: ' + blockText(s.state.doc, 1))
	ok(s.state.stats.deduped >= 1, 'the resend of an applied commit was applied once (writer.Once)')
	ok(s.state.refs.length === 0, 'no ref applied twice')
	const log = await a.evaluate(() => harness.log)
	const byRef = log.reduce((m, l) => m.set(l.ref, (m.get(l.ref) ?? 0) + 1), new Map())
	ok([...byRef.values()].some((n) => n > 1), 'a commit went out more than once with one ref')
	ok(unsavedOffline === 'saving', 'Saving while offline (MP-05.5)')
	await close()
	return { deduped: s.state.stats.deduped, posts: log.length, refs: byRef.size }
})

test('skipped frames: with 60 % of frames held back and overtaken, only timing changes (MP-05.9)', async () => {
	const r = await converge({ editors: 3, seconds: 10, skip: 0.6 })
	ok(r.framesSkipped > 50, 'frames were skipped: ' + r.framesSkipped)
	console.log('     ', JSON.stringify(r))
	return r
})

test('reset: the editor stops sending, keeps its text and copies it (MP-13)', async () => {
	const { tabs, close } = await open(2)
	const [a, b] = tabs
	await caretIn(a, 1)
	await a.evaluate(() => (harness.mode.offline = true))
	await a.keyboard.type('unsaved words')
	await authority(a, { type: 'reset' })
	await a.evaluate(() => harness.reconnect())
	await waitFor(a, () => harness.host().editor.stopped === 'reset')
	await waitFor(b, () => harness.host().editor.stopped === 'reset')
	const posts = await a.evaluate(() => harness.stats.posts)
	await a.keyboard.type('more')
	await sleep(1500)
	const r = await a.evaluate(() => {
		const h = harness.host()
		return {
			text: h.text(),
			editable: h.view.editable,
			notice: h.shadowRoot.getElementById('notice-text').textContent,
			act: h.shadowRoot.getElementById('notice-act').textContent,
			posts: harness.stats.posts,
		}
	})
	ok(r.text.includes('unsaved words') && !r.text.includes('more'), 'text kept, no more input: ' + r.text)
	ok(!r.editable, 'read-only')
	ok(r.notice === 'This page was reset. Your last changes were not saved.', 'says so: ' + r.notice)
	ok(r.posts === posts, `no post after the stop (${posts} then ${r.posts})`)
	await a.evaluate(() => harness.host().shadowRoot.getElementById('notice-act').click())
	await sleep(300)
	const clip = await a.evaluate(() => navigator.clipboard.readText().catch((e) => 'error: ' + e.message))
	ok(clip.includes('unsaved words'), 'copied: ' + clip)
	await close()
	return { clipboard: clip.slice(0, 40) }
})

test('morph: frames, a morph around the editor and a signal patch change nothing in it (Appendix B done 3, MP-05.8)', async () => {
	const { tabs, close } = await open(1)
	const [a] = tabs
	await caretIn(a, 1)
	await a.keyboard.type('before')
	await settled(tabs)
	const before = await a.evaluate(() => {
		const h = harness.host()
		h.querySelector('.pm-mount').marker = 'kept'
		window.viewBefore = h.view
		return { doc: JSON.stringify(h.json()), sel: h.view.state.selection.toJSON() }
	})
	// 1. A frame that changes chips and carets, with content in the script and
	// the mount (the morph must keep both old elements). The authority's own
	// frames wait meanwhile.
	await a.evaluate(() => (harness.mode.frozen = true))
	await a.evaluate(() => {
		const f = { ...harness.lastFrame, chips: JSON.stringify({ 'WEB-12': { summary: 'Renamed', status: 'Done', category: 'done', type: 'task' } }), carets: JSON.stringify([{ id: 'c_ghost', name: 'Ghost Owl', hue: 5, v: +harness.lastFrame.head, anchor: 3, head: 3, idle: false }]) }
		const html = harness
			.hostHTML(f, { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'INJECTED' }] }] })
			.replace('data-ignore></div>', 'data-ignore><p>INJECTED <b data-text="1">x</b></p></div>')
		harness.patch(html)
	})
	await sleep(100)
	const mid = await a.evaluate(() => ({
		chip: harness.host().querySelector('.issue-chip .issue-chip__summary')?.textContent,
		ghost: harness.host().querySelector('.tk-caret .tk-caret__tag')?.textContent,
	}))
	// 2. A morph of the region around the editor.
	await a.evaluate(() => {
		const main = document.getElementById('main')
		const html = main.outerHTML.replace(/<sb-rich-editor[\s\S]*<\/sb-rich-editor>/, harness.hostHTML(harness.lastFrame)).replace('Around the editor:', 'Around the editor (morphed):')
		harness.patch(html)
	})
	// 3. A signal patch.
	await a.evaluate(() => document.dispatchEvent(new CustomEvent('datastar-fetch', { detail: { type: 'datastar-patch-signals', argsRaw: { signals: JSON.stringify({ _around: 7, _editor: { status: 'patched' } }) } } })))
	await sleep(200)
	const between = await a.evaluate(() => {
		const h = harness.host()
		return {
			doc: JSON.stringify(h.json()),
			sel: h.view.state.selection.toJSON(),
			marker: h.querySelector('.pm-mount').marker,
			sameView: window.viewBefore === h.view,
			focus: document.activeElement === h.querySelector('.pm-mount'),
			around: document.getElementById('around').textContent,
			injected: h.querySelector('.pm-mount').textContent.includes('INJECTED'),
		}
	})
	await a.evaluate(() => harness.reconnect())
	await a.keyboard.type(' after')
	const s = await settled(tabs)
	ok(mid.chip === 'Renamed', 'the chip shows the frame\'s data: ' + mid.chip)
	ok(mid.ghost === 'Ghost Owl', 'the frame\'s caret shows: ' + mid.ghost)
	ok(between.doc === before.doc, 'content unchanged by the morphs')
	ok(JSON.stringify(between.sel) === JSON.stringify(before.sel), 'caret unchanged: ' + JSON.stringify([before.sel, between.sel]))
	ok(between.marker === 'kept' && between.sameView, 'same mount element and view')
	ok(between.focus, 'focus kept')
	ok(between.around.includes('morphed') && between.around.includes('7'), 'the region around did change: ' + between.around)
	ok(!between.injected, 'nothing from the morph reached the editor')
	ok(blockText(s.state.doc, 1) === 'Type here: before after', 'typing goes on: ' + blockText(s.state.doc, 1))
	await close()
})

test('undo takes back only the own changes (MP-05.7, FR-82.4)', async () => {
	const { tabs, close } = await open(2)
	const [a, b] = tabs
	await caretIn(a, 1)
	await a.keyboard.type('AAA', { delay: 20 })
	await settled(tabs)
	await caretIn(b, 2, 0)
	await b.keyboard.type('BBB', { delay: 20 })
	await settled(tabs)
	await a.bringToFront()
	await a.evaluate(() => harness.host().view.focus())
	await a.keyboard.down('Control')
	await a.keyboard.press('z')
	await a.keyboard.up('Control')
	const s = await settled(tabs)
	ok(blockText(s.state.doc, 1) === 'Type here: ', 'A\'s text undone: ' + blockText(s.state.doc, 1))
	ok(blockText(s.state.doc, 2).startsWith('BBBSee'), 'B\'s text stays: ' + blockText(s.state.doc, 2))
	await close()
})

test('input rules and shortcuts (FR-82.2)', async () => {
	const { tabs, close } = await open(1)
	const [a] = tabs
	const cases = [
		['# ', 'heading', { level: 1 }, 'H2'],
		['## ', 'heading', { level: 2 }, 'H3'],
		['### ', 'heading', { level: 3 }, 'H4'],
		['- ', 'bullet_list', null, 'UL'],
		['* ', 'bullet_list', null, 'UL'],
		['1. ', 'ordered_list', null, 'OL'],
		['[ ] ', 'task_list', null, 'UL'],
		['> ', 'blockquote', null, 'BLOCKQUOTE'],
		['```', 'code_block', null, 'PRE'],
		['---', 'horizontal_rule', null, 'HR'],
	]
	const seen = []
	for (const [typed, type, attrs, tag] of cases) {
		await appendParagraph(a)
		await a.keyboard.type(typed + 'x', { delay: 10 })
		// "* " right after "- " joins that list: read the block the caret is in.
		const r = await a.evaluate((hr) => {
			const v = harness.host().view
			const i = v.state.selection.$from.index(0) - (hr ? 1 : 0)
			return { node: v.state.doc.child(i).toJSON(), tag: v.dom.children[i].tagName }
		}, type === 'horizontal_rule')
		seen.push([typed, r.node.type, r.tag])
		ok(r.node.type === type, `${JSON.stringify(typed)} makes ${type}, got ${r.node.type}`)
		if (attrs) ok(JSON.stringify(r.node.attrs) === JSON.stringify(attrs), `${typed} attrs ${JSON.stringify(r.node.attrs)}`)
		ok(r.tag === tag, `${typed} renders ${tag}, got ${r.tag}`)
	}
	// An issue key and a space become a chip (FR-100.1).
	const i = await appendParagraph(a)
	await a.keyboard.type('see WEB-12 now', { delay: 10 })
	const chip = await a.evaluate((i) => harness.host().view.state.doc.child(i).toJSON(), i)
	ok(chip.content.some((c) => c.type === 'issue_chip' && c.attrs.key === 'WEB-12'), 'chip: ' + JSON.stringify(chip))
	// Marks from the keyboard.
	const j = await appendParagraph(a)
	for (const [mod, mark] of [[['Control', 'b'], 'strong'], [['Control', 'i'], 'em'], [['Control', 'e'], 'code'], [['Control', 'Shift', 's'], 'strike']]) {
		for (const k of mod.slice(0, -1)) await a.keyboard.down(k)
		await a.keyboard.press(mod.at(-1))
		for (const k of mod.slice(0, -1).reverse()) await a.keyboard.up(k)
		await a.keyboard.type(mark)
		for (const k of mod.slice(0, -1)) await a.keyboard.down(k)
		await a.keyboard.press(mod.at(-1))
		for (const k of mod.slice(0, -1).reverse()) await a.keyboard.up(k)
	}
	const marks = await a.evaluate((j) => harness.host().view.state.doc.child(j).toJSON(), j)
	for (const m of ['strong', 'em', 'code', 'strike']) ok(marks.content.some((c) => c.text === m && c.marks?.[0]?.type === m), `${m}: ${JSON.stringify(marks)}`)
	await settled(tabs)
	await close()
	return { rules: seen }
})

test('paste keeps supported formatting only and offers chips and page links (FR-82.3, DM-06)', async () => {
	const { tabs, close } = await open(1)
	const [a] = tabs
	const i = await appendParagraph(a)
	await paste(a, {
		'text/html':
			'<p>Plain <b>bold</b> <i>it</i> <span style="font-weight:700;color:red" onclick="alert(1)">heavy</span><script>alert(1)</script><img src="https://evil.example/x.png"><img src="/img/lib/diagram-1" alt="A diagram"><a href="javascript:alert(1)">js</a> <a href="https://ok.example/a">ok</a> <a href="/WEB-3">rel</a></p><iframe src="https://evil.example"></iframe><h1>Big</h1><table><tr><th>h</th></tr><tr><td>c</td></tr></table><ul class="contains-task-list"><li class="task-list-item"><input type="checkbox" checked> done</li></ul>',
		'text/plain': 'Plain bold it heavy js ok rel',
	})
	const doc = await a.evaluate(() => harness.host().json())
	const json = JSON.stringify(doc)
	if (process.env.SHOW) console.log(json)
	ok(!/alert|evil|onclick|script|iframe|color/.test(json), 'nothing unsafe kept: ' + json)
	ok(json.includes('"marks":[{"type":"strong"}],"text":"bold"'), 'bold kept')
	ok(json.includes('"marks":[{"type":"em"}],"text":"it"'), 'italic kept')
	ok(json.includes('"marks":[{"type":"strong"}],"text":"heavy"'), 'bold from a style kept')
	ok(json.includes('"href":"https://ok.example/a"'), 'http link kept')
	ok(json.includes(`"href":"${base}/WEB-3"`), 'relative link made absolute')
	ok(json.includes('{"type":"text","text":"js"}'), 'javascript: link dropped, text kept')
	ok(json.includes('"imageId":"diagram-1"') && !json.includes('x.png'), 'library image kept, other site dropped')
	ok(json.includes('"type":"heading","attrs":{"level":1}'), 'h1 becomes a level 1 heading')
	ok(json.includes('"type":"table"') && json.includes('"type":"table_header"'), 'table kept')
	ok(json.includes('"type":"task_item","attrs":{"checked":true}'), 'GitHub task list kept')
	// An issue key as text: an offer to make it a chip.
	await appendParagraph(a)
	await paste(a, { 'text/plain': 'WEB-12' })
	const offer = await a.evaluate(() => harness.host().shadowRoot.getElementById('notice-text').textContent)
	ok(offer === 'Turn WEB-12 into an issue chip?', 'offer: ' + offer)
	await a.evaluate(() => harness.host().shadowRoot.getElementById('notice-act').click())
	const chip = await a.evaluate(() => harness.host().view.state.doc.lastChild.toJSON())
	ok(chip.content?.[0]?.type === 'issue_chip', 'chip made: ' + JSON.stringify(chip))
	await appendParagraph(a)
	await paste(a, { 'text/plain': `${base}/pages/pg_roadmap` })
	await a.evaluate(() => harness.host().shadowRoot.getElementById('notice-act').click())
	const link = await a.evaluate(() => harness.host().view.state.doc.lastChild.toJSON())
	ok(link.content?.[0]?.type === 'page_link' && link.content[0].attrs.pageId === 'pg_roadmap', 'page link made: ' + JSON.stringify(link))
	await settled(tabs)
	await close()
})

test('limits: input stops at 200 KB and at 50 table rows, and says why (DM-05)', async () => {
	const { tabs, close } = await open(1)
	const [a] = tabs
	await a.evaluate(() => {
		window.limitEvents = []
		harness.host().addEventListener('sb-limit', (e) => limitEvents.push(e.detail))
	})
	await appendParagraph(a)
	const before = await a.evaluate(() => JSON.stringify(harness.host().json()).length)
	await paste(a, { 'text/html': '<p>' + 'x'.repeat(210 * 1024) + '</p>', 'text/plain': 'x'.repeat(210 * 1024) })
	const r = await a.evaluate(() => ({ len: JSON.stringify(harness.host().json()).length, ev: limitEvents, notice: harness.host().shadowRoot.getElementById('notice-text').textContent }))
	ok(r.len === before, 'the paste was stopped')
	ok(r.ev[0]?.limit === 'size' && r.notice.includes('200 KB'), 'says why: ' + r.notice)
	// A table of 50 rows takes no 51st.
	const rows = await a.evaluate(() => {
		const v = harness.host().view
		const { nodes } = v.state.schema
		const row = () => nodes.table_row.create(null, [nodes.table_cell.createAndFill(), nodes.table_cell.createAndFill()])
		const table = nodes.table.create(null, Array.from({ length: 50 }, row))
		const end = v.state.doc.content.size
		const tr = v.state.tr.insert(end, table)
		tr.setSelection(v.state.selection.constructor.near(tr.doc.resolve(end + 4)))
		v.dispatch(tr)
		harness.host().editor.command('rowAfter')
		return v.state.doc.lastChild.childCount
	})
	ok(rows === 50, 'still 50 rows: ' + rows)
	const last = await a.evaluate(() => limitEvents.at(-1))
	ok(last.limit === 'table-rows', 'table limit: ' + JSON.stringify(last))
	await settled(tabs)
	await close()
})

test('toolbar: named buttons, aria-pressed, one tab stop, Alt+F10 and arrows (NFR-49, NFR-47)', async () => {
	const { tabs, close } = await open(1)
	const [a] = tabs
	const names = await shadow(a, (root) => [...root.querySelectorAll('.toolbar button, .toolbar select')].map((b) => b.getAttribute('aria-label') || b.textContent.trim() || (b.id && root.querySelector(`label[for=${b.id}]`)?.textContent)))
	ok(names.every(Boolean), 'every control has a name: ' + JSON.stringify(names))
	const stops = await shadow(a, (root) => [...root.querySelectorAll('.toolbar button, .toolbar select')].filter((b) => !b.closest('[hidden]') && b.tabIndex === 0).length)
	ok(stops === 1, 'one tab stop: ' + stops)
	await caretIn(a, 1)
	await a.keyboard.down('Control')
	await a.keyboard.press('b')
	await a.keyboard.up('Control')
	await a.keyboard.type('B')
	await sleep(50)
	const pressed = await shadow(a, (root) => root.querySelector('button[value=strong]').getAttribute('aria-pressed'))
	ok(pressed === 'true', 'Bold pressed: ' + pressed)
	await a.keyboard.down('Alt')
	await a.keyboard.press('F10')
	await a.keyboard.up('Alt')
	const f1 = await shadow(a, (root) => root.activeElement?.getAttribute('aria-label'))
	await a.keyboard.press('ArrowRight')
	const f2 = await shadow(a, (root) => root.activeElement?.getAttribute('aria-label'))
	ok(f1 && f2 && f1 !== f2, `Alt+F10 then ArrowRight: ${f1} -> ${f2}`)
	await a.keyboard.press('Escape')
	const back = await a.evaluate(() => document.activeElement === harness.host().querySelector('.pm-mount'))
	ok(back, 'Escape returns to the text')
	// The toolbar's list button by keyboard: Alt+F10, to "Bulleted list", Enter.
	await a.keyboard.down('Alt')
	await a.keyboard.press('F10')
	await a.keyboard.up('Alt')
	for (let k = 0; k < 20; k++) {
		if ((await shadow(a, (root) => root.activeElement?.getAttribute('aria-label'))) === 'Bulleted list') break
		await a.keyboard.press('ArrowRight')
	}
	await a.keyboard.press('Enter')
	const list = await a.evaluate(() => harness.host().view.state.doc.child(1).type.name)
	ok(list === 'bullet_list', 'list from the toolbar: ' + list)
	const role = await a.evaluate(() => [harness.host().querySelector('.pm-mount').getAttribute('role'), harness.host().querySelector('.pm-mount').getAttribute('aria-label')])
	ok(role[0] === 'textbox' && role[1], 'the text area is a named textbox')
	await settled(tabs)
	await close()
	return { controls: names.length }
})

test('remote carets: bar and name in the person\'s hue, at their place, hidden from screen readers (MP-06)', async () => {
	const { tabs, close } = await open(2)
	const [a, b] = tabs
	await caretIn(a, 1)
	await a.keyboard.type('hi')
	await settled(tabs)
	const caret = await waitFor(b, () => {
		const el = harness.host().querySelector('.tk-caret')
		return el && { hue: el.dataset.hue, hidden: el.getAttribute('aria-hidden'), tag: el.querySelector('.tk-caret__tag')?.textContent, shown: el.classList.contains('tk-caret--tag') }
	})
	ok(caret.hue === '0' && caret.hidden === 'true' && caret.tag === 'Sunny Otter' && caret.shown, 'caret: ' + JSON.stringify(caret))
	const posOf = (tab) => tab.evaluate(() => {
		const v = harness.host().view
		const set = v.someProp('decorations', (f) => f(v.state))
		return set?.find(undefined, undefined, (s) => s.caret)[0]?.from
	})
	const head = await a.evaluate(() => harness.host().view.state.selection.head)
	ok((await posOf(b)) === head, 'at A\'s caret')
	// B types before it: the caret moves with the text.
	await caretIn(b, 1, 0)
	await b.keyboard.type('123')
	await settled(tabs)
	await sleep(300)
	ok((await posOf(b)) === head + 3, 'mapped through B\'s typing')
	// A selection shows as a band.
	await selectIn(a, 1, 0, 4)
	const band = await waitFor(b, () => harness.host().querySelector('.tk-remote-sel')?.dataset.hue)
	ok(band === '0', 'selection band')
	await close()
})

test('link dialog (Mod-K) takes http, https and mailto only (DM-06.2)', async () => {
	const { tabs, close } = await open(1)
	const [a] = tabs
	await caretIn(a, 1)
	await a.keyboard.type('word')
	await selectIn(a, 1, 11, 15)
	await a.keyboard.down('Control')
	await a.keyboard.press('k')
	await a.keyboard.up('Control')
	await waitFor(a, () => !harness.host().shadowRoot.getElementById('link-dialog').hidden)
	await a.keyboard.type('javascript:alert(1)')
	await a.keyboard.press('Enter')
	const err = await shadow(a, (root) => root.getElementById('link-error').textContent)
	ok(err.includes('http'), 'refused: ' + err)
	await a.evaluate(() => (harness.host().shadowRoot.getElementById('href').value = ''))
	await a.keyboard.type('example.com/x')
	await a.keyboard.press('Enter')
	const doc = await a.evaluate(() => harness.host().json())
	const word = doc.content[1].content.find((c) => c.text === 'word')
	ok(word?.marks?.[0]?.attrs?.href === 'https://example.com/x', 'linked: ' + JSON.stringify(doc.content[1]))
	const dom = await a.evaluate(() => harness.host().querySelector('.pm-mount a[href]')?.outerHTML)
	ok(dom.includes('rel="nofollow ugc noopener"') && dom.includes('title="example.com"'), dom)
	await settled(tabs)
	await close()
})

test('a task checkbox toggles with one attr step; others see it (FR-82.1)', async () => {
	const { tabs, close } = await open(2)
	const [a, b] = tabs
	await a.click('li.task-item input')
	const s = await settled(tabs)
	ok(s.state.doc.content[3].content[0].attrs.checked === true, 'checked at the writer')
	const seen = await waitFor(b, () => harness.host().querySelector('li.task-item')?.dataset.checked === 'true' && harness.host().querySelector('li.task-item input').checked)
	ok(seen, 'B sees it')
	await close()
})

test('a refused commit is taken out and the refusal shows at the editor (MP-14)', async () => {
	const { tabs, close } = await open(1)
	const [a] = tabs
	await authority(a, { type: 'refuse', text: 'forbidden', message: 'That word is not allowed here.' })
	await caretIn(a, 1)
	await a.keyboard.type('forbidden', { delay: 120 })
	await waitFor(a, () => harness.host().shadowRoot.getElementById('notice-text').textContent === 'That word is not allowed here.')
	await authority(a, { type: 'refuse' })
	await a.keyboard.type('X')
	const s = await settled(tabs)
	const text = blockText(s.state.doc, 1)
	ok(!text.includes('forbidden') && text.endsWith('X'), 'refused step taken out, typing goes on: ' + text)
	await close()
	return { text }
})

test('readonly and gone stop sending (MP-13, MP-14.2)', async () => {
	const { tabs, close } = await open(1)
	const [a] = tabs
	await authority(a, { type: 'force', tab: 't0', state: 'readonly' })
	await waitFor(a, () => harness.host().editor.stopped === 'readonly')
	await caretIn(a, 1)
	await a.keyboard.type('nope')
	const t1 = await a.evaluate(() => harness.host().text())
	ok(!t1.includes('nope'), 'no input while read-only')
	await authority(a, { type: 'force', tab: 't0', state: '' })
	await waitFor(a, () => harness.host().editor.stopped === null)
	await caretIn(a, 1)
	await a.keyboard.type('yes')
	await settled(tabs)
	await authority(a, { type: 'force', tab: 't0', state: 'gone' })
	const notice = await waitFor(a, () => harness.host().editor.stopped === 'gone' && harness.host().shadowRoot.getElementById('notice-text').textContent)
	ok(notice.startsWith('This page was removed'), notice)
	await close()
})

test('works under a CSP without unsafe-eval (Datastar in nonce mode)', async () => {
	const { tabs, close } = await open(2, { csp: 'strict' })
	await caretIn(tabs[0], 1)
	await tabs[0].keyboard.type('strict', { delay: 30 })
	const s = await settled(tabs)
	ok(s.ms >= 0 && blockText(s.state.doc, 1) === 'Type here: strict', 'commits go out and come back')
	const v = await Promise.all(tabs.map((t) => t.evaluate(() => harness.violations)))
	ok(v.every((x) => !x.length), 'no violations: ' + JSON.stringify(v))
	await close()
})

test("what Datastar's @post returns (the posted() wiring)", async () => {
	const p = await browser.newPage()
	await p.goto(`${base}/test/post.html`)
	await sleep(300)
	const hits = async () => (await (await fetch(`${base}/cmd/test/hits`)).json()).drop
	const table = {}
	for (const id of ['b204', 'b409', 'b422', 'b429', 'b500', 'bdrop', 'bdropretry']) {
		const h0 = await hits()
		const n = await p.evaluate(() => window.outcomes.length)
		await p.click('#' + id)
		await waitFor(p, (n) => window.outcomes.length > n, n, 10000)
		const [k, o, v] = await p.evaluate(() => window.outcomes.at(-1))
		table[k] = o === 'rejected' ? `rejected: ${v.split('\n')[0]}` : 'resolved'
		if (id.startsWith('bdrop')) table[k] += `, ${(await hits()) - h0} request(s) reached the server`
	}
	await p.click('#babort')
	await p.click('#babort')
	await waitFor(p, () => window.outcomes.filter((o) => o[0] === 'auto-cancelled').length === 2, undefined, 5000)
	table['auto-cancelled by a newer post to the URL'] = (await p.evaluate(() => window.outcomes.find((o) => o[0] === 'auto-cancelled')[1]))
	await p.close()
	console.log('     ', JSON.stringify(table))
	return { outcomes: table }
})

for (const t of tests) {
	if (filter && !t.name.includes(filter)) continue
	const t0 = Date.now()
	try {
		const r = await t.fn()
		results[t.name] = { ok: true, ms: Date.now() - t0, ...(r ?? {}) }
		console.log(`ok   ${t.name} (${((Date.now() - t0) / 1000).toFixed(1)} s)`)
	} catch (e) {
		results[t.name] = { ok: false, error: String(e.stack ?? e) }
		console.log(`FAIL ${t.name}: ${e.stack ?? e}`)
	}
}
writeFileSync(join(here, 'out/results.json'), JSON.stringify(results, null, 2))
await browser.close()
server.close()
process.exit(Object.values(results).every((r) => r.ok) ? 0 : 1)
