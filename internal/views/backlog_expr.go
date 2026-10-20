package views

// The backlog's protocol in the page, as the board's (board_expr.go): a
// row dragged by its grip or anywhere with a mouse goes to its new place
// at once, marked pending, and the plan goes to the server with the
// row's version and neighbours; checked rows move together to the top,
// the bottom or a sprint. Rows keep their checkbox and their state
// through frames (data-preserve-attr).

const backlogInit = `el.bl = {press: null, origin: null, rev: '', suppress: false, m: null, aim: null}
const B = el.bl
B.region = () => document.getElementById('` + BacklogID + `')
B.row = (key) => key ? document.getElementById('b-' + key) : null
B.keyOf = (li) => li.id.slice(2)
B.rows = (sec, but) => [...sec.querySelectorAll('.tk-blrow')].filter((r) => r !== but)
B.say = (t) => { $_bl.say = $_bl.say === t ? t + ' ' : t }
B.mark = (li, state) => {
	if (!li) return
	state ? li.setAttribute('data-state', state) : li.removeAttribute('data-state')
	state === 'pending' ? li.setAttribute('aria-busy', 'true') : li.removeAttribute('aria-busy')
}
B.put = (list, li, before) => { 'moveBefore' in list ? list.moveBefore(li, before) : list.insertBefore(li, before) }
B.sib = (li, dir) => { let s = li[dir]; while (s && !s.classList.contains('tk-blrow')) s = s[dir]; return s }
B.marker = () => {
	if (!B.m) { B.m = document.createElement('li'); B.m.className = 'tk-drop'; B.m.setAttribute('aria-hidden', 'true') }
	return B.m
}
B.unaim = () => { B.m && B.m.remove(); B.aim = null }
B.aimAt = (x, y, li) => {
	let sec = null, best = Infinity
	for (const s of el.querySelectorAll('.tk-blsec')) {
		const r = s.getBoundingClientRect()
		const d = y < r.top ? r.top - y : y > r.bottom ? y - r.bottom : 0
		if (d < best) { best = d; sec = s }
	}
	if (!sec || best > 80) { B.unaim(); return }
	const before = B.rows(sec, li).find((c) => { const r = c.getBoundingClientRect(); return y < r.top + r.height / 2 }) || null
	const list = sec.querySelector('.tk-blsec__rows'), m = B.marker()
	const at = before || list.querySelector('.tk-blsec__empty')
	if (m.parentElement !== list || m.nextElementSibling !== at) list.insertBefore(m, at)
	B.aim = {sec, before}
}
B.lift = () => {
	const p = B.press
	p.lifted = true
	B.mark(p.li, 'dragged')
	const g = el.querySelector('.tk-bd__ghost')
	g.replaceChildren(p.li.cloneNode(true))
	g.firstElementChild.removeAttribute('id')
	g.style.inlineSize = p.w + 'px'
	g.hidden = false
	el.setPointerCapture(p.id)
	B.follow(p.x, p.y)
}
B.follow = (x, y) => { el.querySelector('.tk-bd__ghost').style.translate = (x - B.press.dx) + 'px ' + (y - B.press.dy) + 'px' }
B.endDrag = () => {
	const p = B.press, aim = B.aim
	B.press = null
	B.unaim()
	el.querySelector('.tk-bd__ghost').hidden = true
	if (p && p.li.isConnected) B.mark(p.li, '')
	if (p && el.hasPointerCapture(p.id)) el.releasePointerCapture(p.id)
	return aim
}
B.begin = (rows) => {
	if ($_bl.op) { B.say('One change at a time: the last one is on its way.'); return false }
	B.rev = B.region().dataset.rev
	rows.forEach((r) => B.mark(r, 'pending'))
	$_bl.rows = rows.map(B.keyOf).join(' ')
	$_bl.msg = ''
	$_bl.op = ` + newOp + `
	return true
}
B.drop = (li, aim) => {
	const sec = aim.sec, list = sec.querySelector('.tk-blsec__rows')
	if (aim.before === li || (li.closest('.tk-blsec') === sec && B.sib(li, 'nextElementSibling') === aim.before)) { B.say(B.keyOf(li) + ' stays where it is.'); return false }
	if ($_bl.op) { B.say('One change at a time: the last one is on its way.'); return false }
	B.origin = {list: li.parentElement, next: B.sib(li, 'nextElementSibling')}
	B.put(list, li, aim.before || list.querySelector('.tk-blsec__empty'))
	const prev = B.sib(li, 'previousElementSibling'), next = B.sib(li, 'nextElementSibling')
	$_bl.move = {issues: [{issue: B.keyOf(li), seen: +li.dataset.v}], sprint: sec.dataset.section, rank: true, before: next ? B.keyOf(next) : '', after: prev ? B.keyOf(prev) : ''}
	return B.begin([li])
}
B.checked = () => [...el.querySelectorAll('.tk-blrow__check:checked')].map((c) => c.closest('.tk-blrow'))
B.bulk = (how, sprint) => {
	const rows = B.checked()
	if (!rows.length) return false
	B.origin = null
	const issues = rows.map((r) => ({issue: B.keyOf(r), seen: +r.dataset.v}))
	if (how === 'sprint') $_bl.move = {issues, sprint, rank: false, before: '', after: ''}
	else {
		const rest = B.rows(el.querySelector('.tk-blsec[data-section=""]')).filter((r) => !rows.includes(r))
		$_bl.move = how === 'top'
			? {issues, sprint: '', rank: true, before: rest.length ? B.keyOf(rest[0]) : '', after: ''}
			: {issues, sprint: '', rank: true, before: '', after: rest.length ? B.keyOf(rest[rest.length - 1]) : ''}
	}
	return B.begin(rows)
}`

const backlogPost = `@post('/cmd/backlog/plan', {payload: {tab: $_tab, op: $_bl.op, project: el.dataset.project, issues: $_bl.move.issues, sprint: $_bl.move.sprint, rank: $_bl.move.rank, before: $_bl.move.before, after: $_bl.move.after}, requestCancellation: 'disabled'})`

const backlogPress = `if (evt.button !== 0 || $_bl.op || !el.dataset.edit) return
const li = evt.target.closest('.tk-blrow')
if (!li || evt.target.closest('input, button')) return
const r = li.getBoundingClientRect(), x = evt.clientX, y = evt.clientY
const touch = evt.pointerType === 'touch'
if (touch && x - r.left > 22) return
el.bl.press = {li, id: evt.pointerId, x, y, dx: x - r.left, dy: y - r.top, w: r.width, lifted: false}
if (touch) el.bl.lift()`

const backlogDrag = `const p = el.bl.press
if (!p || evt.pointerId !== p.id) return
if (!p.lifted) {
	if (Math.hypot(evt.clientX - p.x, evt.clientY - p.y) < 5) return
	el.bl.lift()
}
el.bl.follow(evt.clientX, evt.clientY)
el.bl.aimAt(evt.clientX, evt.clientY, p.li)`

const backlogRelease = `const p = el.bl.press
if (!p || evt.pointerId !== p.id) return
if (!p.lifted) { el.bl.press = null; return }
el.bl.suppress = true
setTimeout(() => { el.bl.suppress = false }, 0)
const aim = el.bl.endDrag()
aim && el.bl.drop(p.li, aim) && ` + backlogPost

const backlogLost = `const p = el.bl.press
if (!p || evt.pointerId !== p.id) return
p.lifted ? el.bl.endDrag() : (el.bl.press = null)`

// backlogClick: the bulk moves, the sprint menu, Start sprint.
const backlogClick = `const B = el.bl
if (B.suppress) { evt.preventDefault(); evt.stopPropagation(); return }
const b = evt.target.closest('button')
if (!b || !el.contains(b)) return
if (b.dataset.bulk) { B.bulk(b.dataset.bulk) && ` + backlogPost + `; return }
if (b.dataset.sprint !== undefined && b.closest('#bl-sprints')) {
	document.getElementById('bl-sprint').hidePopover()
	B.bulk('sprint', b.dataset.sprint) && ` + backlogPost + `
	return
}
if (b.dataset.start) { $_ss.op === '' && ($_ss.msg = '', $_ss.op = ` + newOp + `, @post('/cmd/sprints/start', {payload: {tab: $_tab, op: $_ss.op, project: el.dataset.project, sprint: b.dataset.start}})) }`

// backlogCount counts the checked rows.
const backlogCount = `evt.target.classList.contains('tk-blrow__check') && ($_bl.n = el.querySelectorAll('.tk-blrow__check:checked').length)`

const backlogSettle = `const op = $_bl.op
if (!op) return
const o = $_outcomes[op]
if (!o) return
const B = el.bl, rows = $_bl.rows.split(' ').filter(Boolean).map(B.row)
rows.forEach((r) => B.mark(r, ''))
if (o.ok) {
	rows.forEach((r) => { const c = r && r.querySelector('.tk-blrow__check'); if (c) c.checked = false })
	$_bl.n = el.querySelectorAll('.tk-blrow__check:checked').length
	B.say(rows.length === 1 ? 'Moved.' : rows.length + ' issues moved.')
} else {
	if (B.origin && rows[0] && B.region().dataset.rev === B.rev) B.put(B.origin.list, rows[0], B.origin.next && B.origin.next.parentElement === B.origin.list ? B.origin.next : null)
	$_bl.msg = o.msg
	B.say(o.msg)
}
B.origin = null
$_bl.op = ''
$_bl.rows = ''`

const backlogFailed = `if (evt.detail.el !== el || !$_bl.op) return
const t = evt.detail.type
if (t !== 'error' && t !== 'retries-failed') return
const status = evt.detail.argsRaw?.status
if (status === '403') return
const B = el.bl, rows = $_bl.rows.split(' ').filter(Boolean).map(B.row)
rows.forEach((r) => B.mark(r, ''))
if (B.origin && rows[0]) B.put(B.origin.list, rows[0], B.origin.next && B.origin.next.parentElement === B.origin.list ? B.origin.next : null)
$_bl.msg = ` + problem + `
$_bl.op = ''
$_bl.rows = ''`

var (
	sprintCreate  = `$_sp.name.trim() && ` + Act("_sp.op", "/cmd/sprints/create", "project: document.querySelector('.tk-bl').dataset.project, name: $_sp.name, goal: $_sp.goal, start: $_sp.start, end: $_sp.end")
	sprintSettled = Settle("_sp.op", "o.ok ? ($_sp.msg = '', $_sp.name = '', $_sp.goal = '', el.close()) : ($_sp.msg = o.msg)")
)
