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
