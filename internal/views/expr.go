package views

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
// measured as the page starts or by newest-frame.js once the page falls
// behind, it sends Save-Data: on (edge.SlowLink). The URL stays the same,
// so a stream opened again aborts the old request. retry 'error' retries
// through network errors, 502s and 503s and never gives up, but leaves a
// stream the server ends closed: on a deploy the server ends it with
// sse.Reconnect, which runs this again.
const openStream = `@get('/stream?tab=' + $_tab + '&v=' + encodeURIComponent($_views), {headers: $_slow ? {'Save-Data': 'on'} : {}, retry: 'error', retryMaxCount: Infinity, retryMaxWait: 10000})`

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
const problem = `evt.detail.argsRaw.status === '429' ? 'Too many changes at once. Wait a moment.' : evt.detail.argsRaw.status === '503' ? 'The server is restarting. Try again in a moment.' : 'This did not go through. Try again in a moment.'`

// The rename form of the person menu (FR-121).
var (
	rename        = `$_rename.name.trim() && ` + Act("_rename.op", "/cmd/people/rename", "name: $_rename.name")
	renameSettled = Settle("_rename.op", "$_rename.msg = o.ok ? 'Saved. Pages show it from their next load.' : o.msg")
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
	loginFailed  = `evt.detail.el === el && evt.detail.type === 'error' && ($_login.problem = evt.detail.argsRaw.status === '429' ? 'Too many attempts. Wait a minute.' : 'This did not go through. Try again in a moment.')`
	loginSignals = `{_login: {email: '', password: '', problem: '', busy: false}}`
)
