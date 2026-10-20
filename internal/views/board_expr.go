package views

import "strings"

// The board's move protocol in the page (MP-04, MP-09), as Datastar
// expressions on the board's stage, which the stream never morphs. Every
// handler is on the stage and delegates: nothing repeats on a card.
//
// A drag (pointer, or a finger on a card's grip) or the card menu picks a
// place. The page moves the card's list item there at once and marks it
// pending (data-state, which the cards keep through morphs by
// data-preserve-attr), then posts the move with the card's version as
// seen and its new neighbours. The tab's outcome ends the pending state:
// accepted, the board's frame has already put the card where the server
// has it (shared views come before the tab's in a tick); refused, the
// card goes back where it was unless a frame came since the drop (the
// lanes' data-rev changed: the frame shows where the card is now), and
// the reason shows beside it. A request that reached no outcome puts the
// card back and offers Retry with the same op id.

// boardInit is the stage's controller: helpers on el.bd for the handlers.
const boardInit = `el.bd = {press: null, origin: null, rev: '', menuFor: null, suppress: false, m: null, aim: null, states: {}}
const B = el.bd
B.board = () => document.getElementById('` + BoardLanesID + `')
B.slot = (key) => key ? document.getElementById('c-' + key) : null
B.keyOf = (li) => li.id.slice(2)
B.laneOf = (li) => li.closest('.tk-lane')
B.list = (lane) => lane.querySelector('.tk-lane__cards')
B.lane = (status) => el.querySelector('.tk-lane[data-lane="' + status + '"]')
B.cards = (lane, but) => [...B.list(lane).children].filter((c) => c.classList.contains('tk-card') && c !== but)
B.say = (t) => { $_board.say = $_board.say === t ? t + ' ' : t }
B.paint = (c, state) => {
	if ((c.getAttribute('data-state') || '') === state) return
	state ? c.setAttribute('data-state', state) : c.removeAttribute('data-state')
	state === 'pending' ? c.setAttribute('aria-busy', 'true') : c.removeAttribute('aria-busy')
}
B.mark = (li, state) => {
	if (!li) return
	state ? (B.states[li.id] = state) : delete B.states[li.id]
	B.paint(li, state)
}
// A card's move state is the page's: the frames know nothing of it, and
// a morph that drops it gets it back before the next paint.
B.keep = new MutationObserver(() => {
	for (const id in B.states) {
		const c = document.getElementById(id)
		c ? B.paint(c, B.states[id]) : delete B.states[id]
	}
})
B.keep.observe(B.board(), {subtree: true, childList: true, attributes: true, attributeFilter: ['data-state']})
B.put = (list, li, before) => { 'moveBefore' in list ? list.moveBefore(li, before) : list.insertBefore(li, before) }
B.sibling = (li, dir) => { let s = li[dir]; while (s && !s.classList.contains('tk-card')) s = s[dir]; return s }
B.clearNote = () => {
	const n = B.slot($_board.ncard)
	if (n && B.states[n.id] === 'refused') B.mark(n, '')
	$_board.note = ''
	$_board.ncard = ''
}
B.marker = () => {
	if (!B.m) { B.m = document.createElement('li'); B.m.className = 'tk-drop'; B.m.setAttribute('aria-hidden', 'true') }
	return B.m
}
B.unaim = () => {
	B.m && B.m.remove()
	B.aim = null
	el.querySelectorAll('.tk-lane[data-target]').forEach((l) => l.removeAttribute('data-target'))
}
B.aimAt = (x, y, li) => {
	const box = B.board().getBoundingClientRect()
	if (x < box.left - 48 || x > box.right + 48 || y < box.top - 64 || y > box.bottom + 96) { B.unaim(); return }
	let lane = null, best = Infinity
	for (const l of el.querySelectorAll('.tk-lane[data-lane]')) {
		const r = l.getBoundingClientRect()
		const dx = x < r.left ? r.left - x : x > r.right ? x - r.right : 0
		const dy = y < r.top ? r.top - y : y > r.bottom ? y - r.bottom : 0
		if (dx * dx + dy * dy < best) { best = dx * dx + dy * dy; lane = l }
	}
	if (!lane) return
	const before = B.cards(lane, li).find((c) => { const r = c.getBoundingClientRect(); return y < r.top + r.height / 2 }) || null
	el.querySelectorAll('.tk-lane[data-target]').forEach((l) => l !== lane && l.removeAttribute('data-target'))
	lane.setAttribute('data-target', '')
	const list = B.list(lane), m = B.marker()
	if (m.parentElement !== list || m.nextElementSibling !== before) list.insertBefore(m, before)
	B.aim = {lane, before}
}
B.scrollEdge = (x) => {
	const s = B.board(), r = s.getBoundingClientRect()
	if (x < r.left + 40) s.scrollLeft -= 14
	else if (x > r.right - 40) s.scrollLeft += 14
}
B.lift = () => {
	const p = B.press
	p.lifted = true
	B.clearNote()
	B.mark(p.li, 'dragged')
	$_board.drag = B.keyOf(p.li)
	const g = el.querySelector('.tk-bd__ghost')
	const copy = p.li.cloneNode(true)
	copy.removeAttribute('id')
	copy.removeAttribute('data-state')
	g.replaceChildren(copy)
	g.style.inlineSize = p.w + 'px'
	g.hidden = false
	el.setPointerCapture(p.id)
	B.follow(p.x, p.y)
}
B.follow = (x, y) => {
	const p = B.press, g = el.querySelector('.tk-bd__ghost')
	g.style.translate = (x - p.dx) + 'px ' + (y - p.dy) + 'px'
}
B.endDrag = () => {
	const p = B.press, aim = B.aim
	B.press = null
	B.unaim()
	el.querySelector('.tk-bd__ghost').hidden = true
	$_board.drag = ''
	if (p && p.li.isConnected) B.mark(p.li, '')
	if (p && el.hasPointerCapture(p.id)) el.releasePointerCapture(p.id)
	return aim
}
B.move = (li, lane, before, glide) => {
	if (!li || !lane) return false
	const key = B.keyOf(li)
	if ($_board.op) { B.say('One move at a time: the last one is on its way.'); return false }
	if (before === li || (li.parentElement === B.list(lane) && B.sibling(li, 'nextElementSibling') === before)) { B.say(key + ' stays where it is.'); return false }
	B.clearNote()
	B.origin = {list: li.parentElement, next: B.sibling(li, 'nextElementSibling'), key}
	B.rev = B.board().dataset.rev
	const go = () => B.put(B.list(lane), li, before)
	go()
	B.mark(li, 'pending')
	const prev = B.sibling(li, 'previousElementSibling'), next = B.sibling(li, 'nextElementSibling')
	$_board.move = {issue: key, status: lane.dataset.lane, before: next ? B.keyOf(next) : '', after: prev ? B.keyOf(prev) : '', seen: +li.dataset.v}
	$_board.pcard = key
	$_board.problem = ''
	$_board.op = ` + newOp + `
	B.say('Moving ' + key + ' to ' + lane.dataset.name + '.')
	return true
}
B.back = (li) => {
	const o = B.origin
	if (!li || !o || !o.list.isConnected) return
	B.put(o.list, li, o.next && o.next.parentElement === o.list ? o.next : null)
}
B.refocus = (li) => {
	if (li && (document.activeElement === document.body || !document.activeElement)) li.querySelector(':scope > button')?.focus()
}
B.pick = (item) => {
	const li = B.menuFor
	document.getElementById('` + CardMenuID + `').hidePopover()
	if (!li || !li.isConnected) return false
	if (item.dataset.open !== undefined) { li.querySelector('.tk-card__title').click(); return false }
	if (item.getAttribute('aria-disabled') === 'true') return false
	if (item.dataset.lane) return B.move(li, B.lane(item.dataset.lane), null, true)
	const lane = B.laneOf(li)
	if (item.dataset.move === 'up') {
		const prev = B.sibling(li, 'previousElementSibling')
		if (!prev) { B.say(B.keyOf(li) + ' is first in ' + lane.dataset.name + '.'); return false }
		return B.move(li, lane, prev, true)
	}
	const next = B.sibling(li, 'nextElementSibling')
	if (!next) { B.say(B.keyOf(li) + ' is last in ' + lane.dataset.name + '.'); return false }
	return B.move(li, lane, B.sibling(next, 'nextElementSibling'), true)
}
B.retry = () => {
	const m = $_board.move, li = B.slot(m.issue), lane = B.lane(m.status)
	if (!li || !lane) return false
	B.origin = {list: li.parentElement, next: B.sibling(li, 'nextElementSibling'), key: m.issue}
	B.rev = B.board().dataset.rev
	B.put(B.list(lane), li, B.slot(m.before))
	B.mark(li, 'pending')
	$_board.pcard = m.issue
	$_board.op = $_board.retry
	$_board.problem = ''
	return true
}
B.uuid = (s) => [...atob(s.replace(/-/g, '+').replace(/_/g, '/'))].map((c) => c.charCodeAt(0).toString(16).padStart(2, '0')).join('').replace(/^(.{8})(.{4})(.{4})(.{4})/, '$1-$2-$3-$4-')
B.toggle = (list, v) => { const s = list.split(' ').filter(Boolean); return (s.includes(v) ? s.filter((x) => x !== v) : [...s, v]).join(' ') }
B.b64 = (s) => btoa(String.fromCharCode(...new TextEncoder().encode(s))).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
B.arg = () => {
	const words = (s) => s.split(' ').filter(Boolean).sort()
	let a = el.dataset.project
	const who = words($_bf.who)
	if ($_bf.mine) who.push('me')
	for (const x of [...new Set(who)].sort()) a += '.a' + x
	for (const x of words($_bf.types)) a += '.t' + x
	for (const x of words($_bf.labels).map(B.b64).sort()) a += '.l' + x
	const q = $_bf.q.trim().split(/\s+/).join(' ')
	if (q) a += '.q' + B.b64([...q].slice(0, 60).join(''))
	return a
}
B.query = () => {
	const u = new URLSearchParams(), words = (s) => s.split(' ').filter(Boolean)
	if ($_bf.mine) u.append('assignee', 'me')
	words($_bf.who).forEach((x) => u.append('assignee', x))
	words($_bf.types).forEach((x) => u.append('type', x))
	words($_bf.labels).forEach((x) => u.append('label', x))
	$_bf.q.trim() && u.set('q', $_bf.q.trim())
	return u.toString()
}
B.applied = el.dataset.arg
// A view transition that Chromium skips or aborts (a newer one started,
// a frame that changed the lanes too much) still applied its frame: it
// only did not animate.
window.tkQuietTransitions || addEventListener('unhandledrejection', (e) => {
	if (['AbortError', 'InvalidStateError'].includes(e.reason?.name) && /transition/i.test(e.reason?.message)) e.preventDefault()
})
window.tkQuietTransitions = true`

// boardRefilter, a data-effect, follows the quick filter: a new filter
// names another view, so the page puts it in the URL and opens its
// stream again with the views of the new filter. The stream with the
// tab's id replaces the old one on the server.
const boardRefilter = `const B = el.closest('.tk-bd')?.bd
if (!B) return
$_bf.mine; $_bf.who; $_bf.types; $_bf.labels; $_bf.q
const arg = B.arg()
if (arg === B.applied) return
B.applied = arg
const q = B.query()
history.replaceState(history.state, '', location.pathname + (q ? '?' + q : ''))
$_views = $_views.split(',').map((v) => v.startsWith('board:') ? 'board:' + arg : v).join(',')
const s = document.getElementById('` + StreamID + `')
s && s.replaceWith(s.cloneNode(true))`

// boardPress starts a drag: the main button of a mouse or pen on a card,
// a finger on its grip (the rest of the card scrolls the board). It
// listens on the window: while a view transition runs (someone's move
// gliding), Chromium gives presses to the board beneath or to the root,
// and the card under the pointer is found by its box.
const boardPress = `if (evt.button !== 0 || $_board.op) return
if (evt.target !== document.documentElement && !el.contains(evt.target)) return
const x = evt.clientX, y = evt.clientY
const under = (e) => { const r = e.getBoundingClientRect(); return x >= r.left && x < r.right && y >= r.top && y < r.bottom }
let li = evt.target.closest?.('#` + BoardLanesID + ` .tk-card')
if (!li && (evt.target === document.documentElement || evt.target.contains?.(el.bd.board()))) li = [...el.querySelectorAll('#` + BoardLanesID + ` .tk-card')].find(under)
if (!li || !el.dataset.edit || evt.target.closest?.('#` + BoardLanesID + ` .tk-card > button')) return
const r = li.getBoundingClientRect()
const touch = evt.pointerType === 'touch'
if (touch && x - r.left > 22) return
el.bd.press = {li, id: evt.pointerId, x, y, dx: x - r.left, dy: y - r.top, w: r.width, lifted: false}
if (touch) el.bd.lift()`

// boardDrag moves the copy with the pointer and aims at the place under
// it, scrolling the board at its edges.
const boardDrag = `const p = el.bd.press
if (!p || evt.pointerId !== p.id) return
if (!p.lifted) {
	if (Math.hypot(evt.clientX - p.x, evt.clientY - p.y) < 5) return
	el.bd.lift()
}
el.bd.follow(evt.clientX, evt.clientY)
el.bd.aimAt(evt.clientX, evt.clientY, p.li)
el.bd.scrollEdge(evt.clientX)`

// post sends the move in hand: requestCancellation 'disabled', so that a
// move never cancels one before it to the same URL.
const boardPost = `@post('/cmd/board/move', {payload: {tab: $_tab, op: $_board.op, issue: $_board.move.issue, status: $_board.move.status, before: $_board.move.before, after: $_board.move.after, seen: $_board.move.seen}, requestCancellation: 'disabled'})`

// boardRelease drops the card where the drag aims.
const boardRelease = `const p = el.bd.press
if (!p || evt.pointerId !== p.id) return
if (!p.lifted) { el.bd.press = null; return }
el.bd.suppress = true
setTimeout(() => { el.bd.suppress = false }, 0)
const aim = el.bd.endDrag()
aim ? el.bd.move(p.li, aim.lane, aim.before, false) && ` + boardPost + ` : el.bd.say(el.bd.keyOf(p.li) + ' stays where it is.')`

// boardLost ends a drag the browser took over (a scroll, a second finger).
const boardLost = `const p = el.bd.press
if (!p || evt.pointerId !== p.id) return
p.lifted ? el.bd.endDrag() : (el.bd.press = null)`

// boardKeys: Escape puts a dragged card back.
const boardKeys = `evt.key === 'Escape' && el.bd.press?.lifted && (evt.preventDefault(), el.bd.endDrag(), el.bd.say('The card stays where it was.'))`

// boardClick is every click on the stage: it swallows the click that ends
// a drag, remembers which card's "…" opened the card menu (and marks its
// column there), toggles quick filters (data-filter, data-v), and runs
// the menu's moves.
var boardClick = `const B = el.bd
if (B.suppress) { evt.preventDefault(); evt.stopPropagation(); return }
const a = evt.target.closest?.('#` + BoardLanesID + ` .tk-card__title[data-i]')
if (a && !a.hasAttribute('` + IssueLink + `')) a.setAttribute('` + IssueLink + `', B.uuid(a.dataset.i))
const b = evt.target.closest('button')
if (!b || !el.contains(b)) return
if (b.parentElement?.matches('#` + BoardLanesID + ` .tk-card')) {
	const menu = document.getElementById('` + CardMenuID + `')
	B.menuFor = b.parentElement
	const lane = B.laneOf(B.menuFor).dataset.lane
	menu.querySelectorAll('[data-lane]').forEach((i) => i.setAttribute('aria-disabled', i.dataset.lane === lane ? 'true' : 'false'))
	menu.matches(':popover-open') ? menu.hidePopover() : menu.showPopover({source: b})
	return
}
if (b.classList.contains('tk-lane__add')) { ` + strings.NewReplacer("el.dataset.status", "b.dataset.status", "el.dataset.project", "el.dataset.pid").Replace(CreateWithStatus) + `; return }
if (b.dataset.filter) {
	const k = b.dataset.filter
	const v = b.dataset.v
	if (k === 'mine') $_bf.mine = !$_bf.mine
	else if (k === 'who') $_bf.who = B.toggle($_bf.who, v)
	else if (k === 'types') $_bf.types = B.toggle($_bf.types, v)
	else if (k === 'labels') $_bf.labels = B.toggle($_bf.labels, v)
	else if (k === 'clear') { $_bf.mine = false; $_bf.who = ''; $_bf.types = ''; $_bf.labels = ''; $_bf.text = ''; $_bf.q = '' }
	return
}
if (b.dataset.retry !== undefined) { B.retry() && ` + boardPost + `; return }
if (b.closest('#` + CardMenuID + `')) { B.pick(b) && ` + boardPost + ` }`

// boardDragStart keeps links and images from starting the browser's own
// drag.
const boardDragStart = `evt.preventDefault()`

// boardSettle ends the pending move once its outcome has come.
const boardSettle = `const op = $_board.op
if (!op) return
const o = $_outcomes[op]
if (!o) return
const B = el.bd, li = B.slot($_board.pcard)
B.mark(li, '')
if (o.ok) {
	const lane = li && B.laneOf(li)
	B.say($_board.pcard + ' is in ' + (lane ? lane.dataset.name : 'its new place') + '.')
} else {
	// A frame that came with the outcome may still wait in its view
	// transition: the card goes back only once the lanes are up to date
	// and no frame came since the drop.
	const rev = B.rev, back = () => { if (B.board().dataset.rev === rev) B.back(li) }
	const vt = B.board().activeViewTransition
	vt ? vt.updateCallbackDone.then(back, back) : requestAnimationFrame(() => { const v = B.board().activeViewTransition; v ? v.updateCallbackDone.then(back, back) : back() })
	B.mark(li, 'refused')
	$_board.note = o.msg
	$_board.ncard = $_board.pcard
	B.say(o.msg)
}
$_board.op = ''
$_board.pcard = ''
B.refocus(li)`

// boardFailed ends a pending move whose request reached no outcome (the
// network, a rate limit, the server stopping, a malformed move): the card
// goes back, and Retry sends it again with the same op id. A 403's
// refusal comes on the stream.
const boardFailed = `if (evt.detail.el !== el || !$_board.op) return
const t = evt.detail.type
if (t === 'retrying') { $_board.problem = 'The server does not answer. Trying again.'; return }
if (t !== 'error' && t !== 'retries-failed') return
const status = evt.detail.argsRaw?.status
if (status === '403') return
const li = el.bd.slot($_board.pcard)
el.bd.mark(li, '')
el.bd.back(li)
$_board.problem = status === '429' ? 'Too many changes at once. Wait a moment.' : status === '503' ? ` + Unavailable + ` : 'This did not go through.'
$_board.retry = $_board.op
$_board.op = ''
$_board.pcard = ''`

// Pressed states of the quick filters, on their buttons.
const (
	mineOn    = `$_bf.mine ? 'true' : 'false'`
	whoOn     = `$_bf.who.split(' ').includes(el.dataset.v) ? 'true' : 'false'`
	typeOn    = `$_bf.types.split(' ').includes(el.dataset.v) ? 'true' : 'false'`
	labelOn   = `$_bf.labels.split(' ').includes(el.dataset.v) ? 'true' : 'false'`
	anyFilter = `$_bf.mine || $_bf.who || $_bf.types || $_bf.labels || $_bf.q.trim()`
	// textApply copies the text box to the filter 300 ms after the last
	// keystroke (FR-52.2).
	textApply = `$_bf.q = $_bf.text`
)

// The dialog that completes the active sprint (FR-46.1).
var (
	completeSprint  = Act("_done.op", "/cmd/sprints/complete", "project: el.closest('dialog').dataset.project, sprint: el.closest('dialog').dataset.sprint, move_to: $_done.to")
	completeSettled = Settle("_done.op", "o.ok ? ($_done.msg = '', el.close()) : ($_done.msg = o.msg)")
)
