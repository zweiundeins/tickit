package views

import "github.com/a-h/templ"

// The document's Datastar expressions, as constants: no value from a
// person goes into one (NFR-13). Values reach the page as escaped HTML or
// as signals. Areas build their actions with Act, Pending, Busy, Settle and
// Failed, each naming the signal that holds its control's pending op id
// (a constant such as "_rename.op").

// slowLink is true on a slow link, measured once as the page starts. Where
// the browser has navigator.connection (Chromium): data saving, or a link
// it calls 2g or 3g. Elsewhere, from Navigation Timing: the document's
// first byte more than 300 ms after its request, or under 100 KB/s.
const slowLink = `((c, n) => c ? c.saveData || ['slow-2g', '2g', '3g'].includes(c.effectiveType) : !!n && n.transferSize > 0 && (n.responseStart - n.requestStart > 300 || n.transferSize < 100 * (n.responseEnd - n.responseStart)))(navigator.connection, performance.getEntriesByType('navigation')[0])`

// openStream opens the tab's stream with the page's views. On a slow link,
// measured as the page starts, it sends Save-Data: on (edge.SlowLink). The URL stays the same,
// so a stream opened again aborts the old request. retry 'error' retries
// through network errors, 502s and 503s and never gives up, but leaves a
// stream the server ends closed: on a deploy the server ends it with
// sse.Reconnect, which runs this again.
const openStream = `@get('/stream?tab=' + $_tab + '&alive=1&v=' + encodeURIComponent($_views), {headers: $_slow ? {'Save-Data': 'on'} : {}, retry: 'error', retryMaxCount: Infinity, retryMaxWait: 10000})`

// behind asks the server to pace the tab's stream each time newest-frame.js
// reports that the page fell behind it (newest-frame-behind); /pace counts
// the report too. It runs on <body>, where the event bubbles to: a @post
// from the stream's element would abort its stream.
const behind = `@post('/pace?tab=' + $_tab, {requestCancellation: 'disabled'})`

// alive answers each heartbeat ($_ping), which the stream promised with
// alive=1: the server ends the stream of a page that went away without
// closing its connection, and with it the page's faces (MP-02.3). It runs
// on an element of its own: a @post from the stream's element would abort
// its stream.
const alive = `$_ping > 0 && @post('/alive?tab=' + $_tab, {requestCancellation: 'disabled', retry: 'never', retryMaxCount: 0}).catch(() => {})`

// inputSeen reports that the person gave input (MP-02.4), at most once a
// minute while there is input, so a person with no input for 5 minutes
// shows as away and one who comes back shows at once. $_input is when it
// last reported. firstInput reports at the stream's first heartbeat when
// nothing did before: the page's opening counts as input, but its stream
// may not be open as it opens.
const inputSeen = `Date.now() - $_input > 60000 && ($_input = Date.now(), @post('/cmd/input', {payload: {tab: $_tab, op: ` + newOp + `}, requestCancellation: 'disabled', retry: 'never', retryMaxCount: 0}).catch(() => {}))`

const firstInput = `$_ping > 0 && $_input === 0 && ` + inputSeen

// revoked loads the page again when the person may no longer read one of
// its views (NFR-01.2): the server answers it as it would anyone without
// the right.
const revoked = `$_revoked && location.reload()`

// keepPlace keeps the focused element where it is on screen when a frame
// changes the layout above it (MP-03.3, NFR-48.1): a comment that comes in
// above the comment box someone types in, a row that grows above a focused
// one. The browser's scroll anchoring holds the first visible element
// instead, which can sit above the change. It notes the focused element's
// place after every scroll, focus and keystroke, and when the page's size
// changes and the element moved without a scroll, scrolls it back.
const keepPlace = `(() => { let at = null, top = 0; ` +
	`const note = () => { at = document.activeElement; top = at && at !== document.body ? at.getBoundingClientRect().top : 0 }; ` +
	`addEventListener('scroll', note, {passive: true}); document.addEventListener('focusin', note); document.addEventListener('input', note, true); ` +
	`new ResizeObserver(() => { const a = document.activeElement; ` +
	`if (a && a !== document.body && a === at && a.isConnected) { const d = a.getBoundingClientRect().top - top; if (Math.abs(d) >= 2) scrollBy(0, d) } note() }).observe(document.body) })()`

// startStream is the stream element's data-init.
const startStream = `$_beat = Date.now(); ` + openStream

// beat notes a sign of life: the hub patches $_ping every 20 s
// (live.Options.HeartbeatSignal), frames or not.
const beat = `$_ping, $_beat = Date.now(), $_stale = false`

// watchdog opens the stream again when no heartbeat came for 50 s: a
// connection that went quiet without closing, as a laptop's after sleep
// (MP-10.1). A hidden tab has no stream (Datastar closes a @get there).
const watchdog = `!document.hidden && Date.now() - $_beat > 50000 && ($_beat = Date.now(), $_stale = true, ` + openStream + `)`

// streamFailed shows "Reconnecting" at once when the stream's request
// fails; the next heartbeat, or the first frame of a new stream, hides it.
const streamFailed = `evt.detail.el === el && ['error', 'retrying', 'retries-failed'].includes(evt.detail.type) && ($_stale = true)`

// follow goes where an action's answer says ($_goto, writer.OnceResult).
const follow = `$_goto && location.assign($_goto)`

// newOp is a new op id: what the page sends with an action, and what the
// tab's view brings back in $_outcomes.
const newOp = `Date.now().toString(36) + Math.random().toString(36).slice(2, 7)`

// Act posts an action to path with a new op id kept in the signal op,
// unless one is pending there. payload adds fields to {tab, op}, as
// "name: $_rename.name".
func Act(op, path, payload string) string {
	if payload != "" {
		payload = ", " + payload
	}
	return `$` + op + ` === '' && ($_problem = '', $` + op + ` = ` + newOp + `, @post('` + path + `', {payload: {tab: $_tab, op: $` + op + payload + `}}))`
}

// Pending is true from the click until the stream brings the outcome.
func Pending(op string) string { return `$` + op + ` !== ''` }

// OffKeepFocus disables a control while off holds, and while pending
// holds unless it has the focus: that one is aria-disabled instead, since
// Chromium drops the focus of an element that turns disabled.
func OffKeepFocus(off, pending string) templ.Attributes {
	return templ.Attributes{
		"data-attr:disabled":      `(` + off + `) || ((` + pending + `) && document.activeElement !== el)`,
		"data-attr:aria-disabled": `!(` + off + `) && (` + pending + `) && document.activeElement === el ? 'true' : false`,
	}
}

// Busy is Pending for aria-busy: data-attr writes true as an empty value,
// which aria-busy reads as false.
func Busy(op string) string { return `$` + op + ` !== '' ? 'true' : false` }

// Settle, a data-effect, ends the pending state when the op's outcome
// arrives: it runs then, with the outcome as o ({ok, msg, field}), and
// clears the op.
func Settle(op, then string) string {
	return `$` + op + ` !== '' && $_outcomes[$` + op + `] && ((o => (` + then + `))($_outcomes[$` + op + `]), $` + op + ` = '')`
}

// Failed, on data-on:datastar-fetch, ends the pending state when the
// action's request reached no outcome (MP-09.3). A 403 brings its reason
// on the stream like any refusal.
func Failed(op string) string {
	return `evt.detail.el === el && ['error', 'retries-failed'].includes(evt.detail.type) && evt.detail.argsRaw.status !== '403' && ($` + op + ` = '', $_problem = ` + problem + `)`
}

// problem is what the page says of a request that reached no outcome.
const problem = `evt.detail.argsRaw.status === '429' ? 'Too many changes at once. Wait a moment.' : evt.detail.argsRaw.status === '503' ? ` + Unavailable + ` : evt.detail.argsRaw.status === '409' ? 'This tab is not connected. Reload the page.' : 'This did not go through. Try again in a moment.'`

// Unavailable is what a page says of a 503: the demo's reset while the
// demo alert says one runs, else a restart.
const Unavailable = `(document.querySelector('#` + DemoAlertID + `[data-resetting]') ? 'The demo is resetting. Try again in a minute.' : 'The server is restarting. Try again in a moment.')`

// The rename form of the person menu (FR-121).
var (
	rename        = `$_rename.name.trim() && ` + Act("_rename.op", "/cmd/people/rename", "name: $_rename.name")
	renameSettled = Settle("_rename.op", "$_rename.ok = o.ok, $_rename.msg = o.ok ? 'Saved. Pages show it from their next load.' : o.msg")
	renameKind    = `$_rename.msg ? ($_rename.ok ? 'ok' : 'refused') : false`
	renameFailed  = Failed("_rename.op")
)

// logout logs out and goes where the answer says.
const logout = `@post('/cmd/logout')`

// The login form (FR-122): the password stays in a local signal and goes
// only in the payload; the page goes where the answer says. next is the
// page's own ?next=, which the server checks.
const (
	login        = `$_login.busy || ($_login.busy = true, $_login.problem = '', @post('/cmd/login', {payload: {email: $_login.email, password: $_login.password, next: new URLSearchParams(location.search).get('next') || '/'}}))`
	loginDone    = `evt.detail.el === el && evt.detail.type === 'finished' && ($_login.busy = false)`
	loginBusy    = `$_login.busy ? 'true' : false`
	loginKind    = `$_login.problem ? 'refused' : false`
	loginFailed  = `evt.detail.el === el && evt.detail.type === 'error' && ($_login.problem = evt.detail.argsRaw.status === '429' ? 'Too many attempts. Wait a minute.' : 'This did not go through. Try again in a moment.')`
	loginSignals = `{_login: {email: '', password: '', problem: '', busy: false}}`
)

// The display menu (theme, style, density) posts one preference; the
// stream brings the session's _prefs back, which applyPrefs puts on <html>
// and prefPressed on the menu's buttons. Every value names its own
// preference, so a button's value alone says whether it is pressed.
var (
	setPref        = Act("_display.op", "/cmd/prefs", "key: el.dataset.pref, value: el.dataset.value")
	displaySettled = Settle("_display.op", "o.ok || ($_problem = o.msg)")
	displayFailed  = Failed("_display.op")
)

const prefPressed = `[$_prefs.theme, $_prefs.style, $_prefs.density].includes(el.dataset.value) ? 'true' : 'false'`

// applyPrefs, a data-effect, keeps <html> and the color-scheme meta on the
// session's preferences.
const applyPrefs = `document.documentElement.dataset.tkTheme = $_prefs.theme; document.documentElement.dataset.tkStyle = $_prefs.style; document.documentElement.dataset.tkDensity = $_prefs.density; document.querySelector('meta[name=color-scheme]').content = $_prefs.theme === 'system' ? 'light dark' : $_prefs.theme`

// Dismissing the demo notice (DM-10.1) and the first-visit hint (DM-11)
// is a preference; each hides when the stream brings it.
var (
	dismissDemo     = Act("_dismiss.op", "/cmd/prefs", "key: 'demo', value: 'hidden'")
	dismissHint     = Act("_dismiss.op", "/cmd/prefs", "key: 'hint', value: 'hidden'")
	dismissSettled  = Settle("_dismiss.op", "o.ok || ($_problem = o.msg)")
	dismissFailed   = Failed("_dismiss.op")
	dismissBusy     = Busy("_dismiss.op")
	demoHidden      = `$_prefs.hideDemo`
	hintHidden      = `$_prefs.hideHint`
	reconnectHidden = `!$_stale`
	problemHidden   = `!$_problem`
)

// ConfirmID is the document's dialog that asks before an action that
// cannot be taken back (Ask).
const ConfirmID = "tk-confirm"

// Ask runs act once the person confirms question in the document's
// confirm dialog, whose button says label. The dialog clicks the asking
// element again, marked, and the focus goes back to it when the dialog
// closes. question and label are constants, never a person's text.
func Ask(question, label, act string) string {
	return `el.dataset.asked ? (delete el.dataset.asked, ` + act + `) : ((d) => { d.querySelector('.tk-confirm__text').textContent = ` + jsString(question) +
		`; d.querySelector('[data-confirm] .tk-btn__label').textContent = ` + jsString(label) + `; d.asker = el; d.showModal() })(document.getElementById('` + ConfirmID + `'))`
}

// confirmed is the confirm dialog's button: it closes the dialog and
// clicks the element that asked, marked as confirmed.
const confirmed = `((b) => { el.closest('dialog').close(); if (b?.isConnected) { b.dataset.asked = '1'; b.click() } })(el.closest('dialog').asker)`
