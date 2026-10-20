// One editing tab of the harness: a page as tickit's server would render it,
// with Datastar, the component and the page's wiring, talking to the fake
// authority (a SharedWorker) instead of the network.
//
// - Posts: Datastar's @post calls fetch; a shim takes /cmd/pages/* to the
//   authority, 25 ms each way, and can drop the connection.
// - Frames: the authority renders the host's attributes every tick; the tab
//   waits 25 ms, may skip the frame (a slow stream), and applies it the way
//   the SSE stream does, as a datastar-patch-elements morph of the host.
//
// window.harness exposes knobs and measurements to the runner.
const q = new URLSearchParams(location.search)
const page = q.get('page') || 'p_demo'
const tab = q.get('tab') || 't_' + Math.random().toString(36).slice(2, 8)
const name = q.get('name') || 'Someone'
const hue = +(q.get('hue') || 0)
const now = () => performance.timeOrigin + performance.now()

const mode = { delay: +(q.get('delay') ?? 25), skip: +(q.get('skip') ?? 0), offline: false, loseResponse: false, frozen: false }
const stats = { framesIn: 0, framesSkipped: 0, framesApplied: 0, posts: 0, postsFailed: 0 }
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

// --- The authority --------------------------------------------------------
const worker = new SharedWorker('/test/out/authority.js', { name: 'authority' })
const port = worker.port
let nextId = 1
const waiting = new Map()
function rpc(msg) {
	const id = nextId++
	return new Promise((resolve, reject) => {
		waiting.set(id, { resolve, reject })
		port.postMessage({ ...msg, id, page })
	})
}
port.onmessage = ({ data }) => {
	if (data.type === 'reply') {
		const w = waiting.get(data.id)
		waiting.delete(data.id)
		data.error ? w.reject(new Error(data.error)) : w.resolve(data.result)
	} else if (data.type === 'frame') frame(data.frame)
}
port.start()

// --- Posts: the fetch Datastar makes -------------------------------------
const realFetch = window.fetch.bind(window)
window.fetch = async (input, init = {}) => {
	const url = new URL(typeof input === 'string' ? input : input.url, location.href)
	const m = /^\/cmd\/pages\/([^/]+)\/(commit|presence)$/.exec(url.pathname)
	if (!m) return realFetch(input, init)
	stats.posts++
	await sleep(mode.delay)
	if (mode.offline) throw (stats.postsFailed++, new TypeError('Failed to fetch'))
	const body = JSON.parse(init.body)
	const status = await rpc({ type: m[2], body })
	if (m[2] === 'commit') log.push({ t: now(), ref: body.ref, base: body.base, steps: body.steps.length, status, lost: mode.loseResponse })
	if (mode.loseResponse) throw (stats.postsFailed++, new TypeError('Failed to fetch'))
	await sleep(mode.delay)
	return new Response(null, { status })
}
const log = []

// --- Frames ---------------------------------------------------------------
const wiring = {
	'data-on:sb-commit':
		"@post('/cmd/pages/' + evt.detail.page + '/commit', {payload: {...evt.detail, tab: $_tab}, requestCancellation: 'disabled', retry: 'never', retryMaxCount: 0}).then(() => el.posted(evt.detail.ref, true), () => el.posted(evt.detail.ref, false))",
	'data-on:sb-presence': "@post('/cmd/pages/' + evt.detail.page + '/presence', {payload: {...evt.detail, tab: $_tab}, retry: 'never', retryMaxCount: 0}).catch(() => {})",
	'data-on:sb-status': '$_editor.status = evt.detail.status',
	'data-on:sb-stopped': '$_editor.stopped = evt.detail.state',
	'data-on:sb-refused': '$_editor.refusal = evt.detail.message',
}
const esc = (s) => String(s).replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;')
const ATTRS = ['id', 'page', 'epoch', 'client', 'base', 'head', 'batches', 'carets', 'chips', 'state', 'refusal']
function hostHTML(f, doc = '') {
	const attrs = ATTRS.map((k) => `${k}="${esc(f[k] ?? '')}"`).join(' ')
	const extra = (harness.extraAttrs || '') + (f.readonly ? ' readonly' : '') + (harness.images ? ` images="${esc(JSON.stringify(harness.images))}"` : '')
	const on = Object.entries(wiring).map(([k, v]) => `${k}="${esc(v)}"`).join(' ')
	const json = doc ? JSON.stringify(doc).replace(/</g, '\\u003c') : ''
	return `<sb-rich-editor ${attrs}${extra} ${on}><script type="application/json" data-ignore-morph>${json}</script><div class="pm-mount prose" data-ignore-morph data-ignore></div></sb-rich-editor>`
}
function patch(elements, extra = {}) {
	document.dispatchEvent(new CustomEvent('datastar-fetch', { detail: { type: 'datastar-patch-elements', el: document.documentElement, argsRaw: { elements, ...extra } } }))
}
let lastFrame = null
async function frame(f) {
	stats.framesIn++
	if (mode.offline || mode.frozen) return
	if (Math.random() < mode.skip) return void stats.framesSkipped++
	await sleep(mode.delay)
	if (mode.offline || mode.frozen) return
	lastFrame = f
	patch(hostHTML(f))
	stats.framesApplied++
}

// --- Measurements -----------------------------------------------------------
// typed: [char, time] of this tab's keystrokes. arrivals[j][n]: when the n-th
// character of editor j's alphabet showed here.
const typed = []
const arrivals = {}
let alphabets = []
document.addEventListener('keydown', (e) => e.key.length === 1 && !e.ctrlKey && !e.metaKey && typed.push([e.key, now()]), true)
function count() {
	const host = document.getElementById('editor')
	const text = host?.view?.state.doc.textContent ?? ''
	const t = now()
	alphabets.forEach((alpha, j) => {
		if (!alpha) return
		let n = 0
		for (const ch of text) if (alpha.includes(ch)) n++
		const a = (arrivals[j] ??= [])
		for (let k = a.length; k < n; k++) a.push(t)
	})
}
document.addEventListener('sb-ack', count)

// --- Start ------------------------------------------------------------------
const ready = new Promise((resolve) => document.addEventListener('sb-ready', resolve, { once: true }))
const opened = rpc({ type: 'open', tab, name, hue }).then((o) => {
	document.dispatchEvent(new CustomEvent('datastar-fetch', { detail: { type: 'datastar-patch-signals', el: document.documentElement, argsRaw: { signals: JSON.stringify({ _tab: tab }) } } }))
	patch(hostHTML({ id: 'editor', page: o.page, epoch: o.epoch, client: o.client, base: o.base, head: o.base, batches: '[]', carets: '[]', chips: '{}', state: 'ok', refusal: '' }, o.doc), { selector: '#slot', mode: 'inner' })
	return o
})

const violations = []
document.addEventListener('securitypolicyviolation', (e) => violations.push(`${e.violatedDirective} ${e.blockedURI} ${e.sample || ''}`))
const errors = []
addEventListener('error', (e) => errors.push(String(e.message)))
addEventListener('unhandledrejection', (e) => errors.push('rejection: ' + String(e.reason?.message ?? e.reason)))

window.harness = {
	tab,
	page,
	mode,
	stats,
	log,
	typed,
	arrivals,
	violations,
	errors,
	opened,
	ready,
	rpc,
	patch,
	hostHTML,
	get lastFrame() {
		return lastFrame
	},
	setAlphabets(a) {
		alphabets = a
		count()
	},
	host: () => document.getElementById('editor'),
	extraAttrs: '',
	images: null,
}
