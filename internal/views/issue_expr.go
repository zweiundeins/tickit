package views

import (
	"encoding/json"
	"strings"
)

// The issue page's and the issue panel's Datastar expressions. Like every
// expression of the document they are constants: what a person wrote
// reaches them only through the DOM (textContent, data attributes) or
// signals, never inside one (NFR-13).
//
// The page keeps its state in the signal _is (issueSignals): the issue it
// shows, the field being edited and its text, and per field the pending op
// id (ops), the refusal (msg), the conflict's data (conflict), a request
// that reached no outcome (fail) and the value sent (want), so that
// "Retry" and "Use Critical anyway" send it again (MP-08, MP-09).

// IssueFields are the actions of an issue's page, each its own pending
// state: POST /cmd/issues/{id}/{field}.
var IssueFields = []string{
	"summary", "type", "status", "priority", "assignee", "parent", "due", "points",
	"estimate", "remaining", "sprint", "labels", "components", "watch", "link", "unlink", "subtask",
	"comment", "cedit", "cdelete", "chide", "delete", "pagelink", "pageunlink",
}

// issueSignals are the page's state of the issue it shows. The issue's id,
// key and the viewer's id are ids the server made, never a person's text.
func issueSignals(id, key, me string, readOnly, moderates bool) string {
	per := func(v any) map[string]any {
		m := map[string]any{}
		for _, f := range IssueFields {
			m[f] = v
		}
		return m
	}
	b, _ := json.Marshal(map[string]any{"_is": map[string]any{
		"id": id, "key": key, "me": me, "ro": readOnly, "mod": moderates,
		"edit": "", "val": "", "orig": "", "seen": 0, "ref": "", "desc": false,
		"ops": per(""), "last": per(""), "msg": per(""), "fail": per(""), "conflict": per(false), "want": per(""), "sent": per(0), "refs": per(""),
		"lq": "", "pq": "", "mseen": 0, "sub": "", "link": map[string]any{"type": "blocks", "key": ""},
		"draft": "", "mq": false, "cedit": "", "ctext": "",
		"tab": "comments", "at": 0, "max": 1, "back": "", "pop": false,
	}})
	return string(b)
}

// curVersion is the issue's version the page shows now: what a choice
// sends as Seen (MP-08.1).
const curVersion = `+(document.getElementById('issue-main')?.dataset.v || 0)`

// issueSendFn sends field f with value v, seen s and ref r, unless f is
// pending: a new op id in ops[f], the value kept in want[f] for a retry.
const issueSendFn = `((f, v, s, r) => (v = ['comment', 'cedit'].includes(f) ? ` + mentionIDs + `(v) : v, !$_is.ro || f === 'comment' || f === 'cedit' || f === 'cdelete') && $_is.ops[f] === '' && ($_is.msg[f] = '', $_is.fail[f] = '', $_is.conflict[f] = false, $_is.want[f] = v, $_is.sent[f] = s, $_is.refs[f] = r, $_is.ops[f] = ` + newOp + `, @post('/cmd/issues/' + $_is.id + '/' + f, {payload: {tab: $_tab, op: $_is.ops[f], seen: s, value: v, ref: r}, retryMaxCount: 2})))`

// menuOpened keeps the version a menu showed when it opened: a choice in
// it sends that version, so a change someone made while the menu was open
// is a conflict, not overwritten (MP-08.2). On a menu's button.
const menuOpened = `$_is.mseen = ` + curVersion

// mentionIDs turns the names of mentions in a text ("@Ada Lovelace", as
// the comment box and the editors show them) back into the ids the markup
// holds ("@s-ada"): the names of the people the picker offers and the
// texts mention, longest first (FR-67.2).
const mentionIDs = `((s) => { const m = new Map(); document.querySelectorAll('#issue-mentions [data-id]').forEach((e) => m.has(e.dataset.name) || m.set(e.dataset.name, e.dataset.id)); for (const n of [...m.keys()].sort((a, b) => b.length - a.length)) s = s.split('@' + n).join('@' + m.get(n)); return s })`

// issuePick sends what a Starbase picker of the side column (assignee,
// epic, due date) committed, on the frame's sb-change: with the version
// the page showed when the picker took the focus (issuePickFocus), as a
// menu keeps the one it opened at (MP-08.2). A pick while the field waits
// for an outcome goes back to the server's value.
const issuePick = `((t) => { if (!t) return; const f = t.dataset.field; if ($_is.ro || $_is.ops[f] !== '') return t.revert?.(); ` + issueSendFn + `(f, evt.detail.value ?? '', $_is.mseen || ` + curVersion + `, '') })(evt.target.closest?.('[data-act=pick]'))`

// issuePickFocus keeps the version a picker showed when it took the focus.
const issuePickFocus = `evt.target.closest?.('[data-act=pick]') && !evt.target.contains(evt.relatedTarget) && ($_is.mseen = ` + curVersion + `)`

// issueRetry sends field f's request that reached no outcome again with
// its op id, so that it applies once (MP-09.3, MP-09.4).
const issueRetryFn = `((f) => $_is.ops[f] === '' && $_is.last[f] !== '' && ($_is.fail[f] = '', $_is.ops[f] = $_is.last[f], @post('/cmd/issues/' + $_is.id + '/' + f, {payload: {tab: $_tab, op: $_is.ops[f], seen: $_is.sent[f], value: $_is.want[f], ref: $_is.refs[f]}, retryMaxCount: 2})))`

// issueAnywayFn sends field f's refused value again against the version
// the conflict reports (MP-08.3: "Use Critical anyway").
const issueAnywayFn = `((f) => $_is.conflict[f] && ($_is.edit === f ? ($_is.seen = $_is.conflict[f].version, ` + issueSendFn + `(f, $_is.val, $_is.seen, '')) : ` + issueSendFn + `(f, $_is.want[f], $_is.conflict[f].version, $_is.refs[f])))`

// issueEditFn opens the editor of the field on t: the text the page shows
// (the element t.dataset.src names) goes into $_is.val, and the version
// shown into $_is.seen, which the save sends (MP-08.1).
const issueEditFn = `((t) => { const f = t.dataset.field; !$_is.ro && $_is.at === 0 && $_is.ops[f] === '' && ($_is.edit = f, $_is.val = $_is.orig = (document.getElementById(t.dataset.src)?.textContent ?? '').trim(), $_is.seen = ` + curVersion + `, $_is.msg[f] = '', $_is.conflict[f] = false, setTimeout(() => document.getElementById('issue-edit-' + f)?.focus())) })`

// issueSave saves the open editor: nothing changed closes it (FR-14.3).
const issueSave = `$_is.edit !== '' && $_is.ops[$_is.edit] === '' && ($_is.val.trim() === $_is.orig ? ($_is.edit = '', ` + issueStopTyping + `) : ` + issueSendFn + `($_is.edit, $_is.val, $_is.seen, ''))`

// issueCancel closes the editor without saving, unless it waits for an
// outcome.
const issueCancel = `$_is.edit !== '' && $_is.ops[$_is.edit] === '' && (` + issueStopTyping + `, $_is.msg[$_is.edit] = '', $_is.fail[$_is.edit] = '', $_is.conflict[$_is.edit] = false, $_is.edit = '')`

// issueAddFn adds what field f's small input holds: a label, a subtask,
// a link.
const issueAddFn = `((f) => { const v = (f === 'labels' ? $_is.lq : f === 'subtask' ? $_is.sub : $_is.link.key).trim(); v && ` + issueSendFn + `(f, v, 0, f === 'labels' ? 'add' : f === 'link' ? $_is.link.type : '') })`

// The frame's delegated handlers: every control of the regions names what
// it does in data-act (and data-field, data-value, data-ref), so the
// stream's frames carry attributes, not expressions, and each handler
// compiles once (the lith rule for repeated elements).
const issueClick = `((t) => {
	if (!t || t.disabled || t.getAttribute('aria-disabled') === 'true') return
	const f = t.dataset.field
	switch (t.dataset.act) {
	case 'choose': { const m = t.closest('[popover]'); m?.hidePopover(); ` + issueSendFn + `(f, t.dataset.value ?? '', (m && $_is.mseen) || ` + curVersion + `, t.dataset.ref ?? ''); break }
	case 'edit': ` + issueEditFn + `(t); break
	case 'desc-edit': ` + descEditFn + `; break
	case 'desc-done': ` + descDone + `; break
	case 'save': ` + issueSave + `; break
	case 'cancel': ` + issueCancel + `; break
	case 'clear': $_is.val = ''; ` + issueSave + `; break
	case 'retry': ` + issueRetryFn + `(f); break
	case 'anyway': ` + issueAnywayFn + `(f); break
	case 'add': ` + issueAddFn + `(f); break
	case 'cedit': $_is.cedit = t.dataset.comment; $_is.ctext = (document.getElementById('comment-src-' + t.dataset.comment)?.textContent ?? '').trim(); setTimeout(() => document.getElementById('comment-edit-' + t.dataset.comment)?.focus()); break
	case 'csave': $_is.ctext.trim() !== '' && ` + issueSendFn + `('cedit', $_is.ctext, 0, t.dataset.comment); break
	case 'ccancel': $_is.cedit = ''; $_is.msg.cedit = ''; break
	case 'mention': { const c = document.getElementById('issue-comment'), pos = c.selectionStart, before = c.value.slice(0, pos).replace(/@[^\s@]*$/, '@' + t.dataset.name + ' '); $_is.draft = before + c.value.slice(pos); $_is.mq = false; c.focus(); setTimeout(() => c.setSelectionRange(before.length, before.length)); break }
	}
})(evt.target.closest?.('[data-act]'))`

// issueKeys: in an editor Enter saves a one-line field, Ctrl or Cmd Enter
// the description, Escape cancels (FR-14.1); Enter adds in a small input;
// Ctrl Enter saves a comment being edited.
const issueKeys = `((t) => {
	switch (t.dataset?.keys) {
	case 'edit':
		if (evt.key === 'Escape') { evt.preventDefault(); ` + issueCancel + ` }
		else if (evt.key === 'Enter' && (t.tagName !== 'TEXTAREA' || evt.ctrlKey || evt.metaKey)) { evt.preventDefault(); ` + issueSave + ` }
		break
	case 'add':
		if (evt.key === 'Enter') { evt.preventDefault(); ` + issueAddFn + `(t.dataset.field) }
		break
	case 'cedit':
		if (evt.key === 'Escape') { $_is.cedit = ''; $_is.msg.cedit = '' }
		else if ((evt.ctrlKey || evt.metaKey) && evt.key === 'Enter') { evt.preventDefault(); $_is.ctext.trim() !== '' && ` + issueSendFn + `('cedit', $_is.ctext, 0, t.dataset.comment) }
		break
	}
})(evt.target)`

// issueFocusOut saves an editor when the focus leaves it for the page: a
// click outside saves (FR-14.1).
const issueFocusOut = `((ed) => ed && ed.dataset.editor === $_is.edit && !ed.contains(evt.relatedTarget) && (` + issueSave + `))(evt.target.closest?.('[data-editor]'))`

// issueTyping refreshes the typing indicator of the field typed in
// (MP-07); throttled on the frame.
const issueTyping = `((t) => t && $_is.at === 0 && @post('/cmd/issues/' + $_is.id + '/typing', {payload: {tab: $_tab, op: ` + newOp + `, field: t.dataset.typing === 'edit' ? $_is.edit : t.dataset.typing, stop: false}, retryMaxCount: 0}))(evt.target.closest?.('[data-typing]'))`

// commentStopTyping ends the comment box's indicator when the focus
// leaves it (MP-07.4); a comment sent ends it too.
const commentStopTyping = `$_is.at === 0 && @post('/cmd/issues/' + $_is.id + '/typing', {payload: {tab: $_tab, op: ` + newOp + `, field: 'comment', stop: true}, retryMaxCount: 0}).catch(() => {})`

// issueStopTyping ends the indicator of the open editor at once (MP-07.4).
const issueStopTyping = `(($_is.edit === 'summary' || $_is.edit === 'description') && @post('/cmd/issues/' + $_is.id + '/typing', {payload: {tab: $_tab, op: ` + newOp + `, field: $_is.edit, stop: true}, retryMaxCount: 0}))`

// issueSettle, a data-effect, ends each field's pending state when its
// outcome arrives in $_outcomes: a refusal stays next to the field, a
// conflict with its data; an accepted edit closes the editor.
const issueSettle = `Object.keys($_is.ops).forEach(f => {
	const op = $_is.ops[f], o = op ? $_outcomes[op] : null
	if (!o) return
	$_is.ops[f] = ''
	$_is.last[f] = ''
	if (o.ok) {
		$_is.msg[f] = ''
		$_is.conflict[f] = false
		if ($_is.edit === f) $_is.edit = ''
		if (f === 'comment') { $_is.draft = ''; try { sessionStorage.removeItem('tickit-draft:' + $_is.id) } catch (e) {} }
		if (f === 'subtask') $_is.sub = ''
		if (f === 'link') $_is.link.key = ''
		if (f === 'labels') $_is.lq = ''
		if (f === 'pagelink') { $_is.pq = ''; document.getElementById('` + PagePickID + `')?.replaceChildren() }
		if (f === 'cedit') $_is.cedit = ''
		if (f === 'delete') document.getElementById('issue-delete')?.close()
	} else {
		$_is.msg[f] = o.msg || 'This was refused.'
		$_is.conflict[f] = o.data && o.data.kind === 'conflict' ? o.data : false
		// A refused pick shows the server's value again; after a conflict
		// the person's stays, for "Use it anyway" (MP-08.3).
		$_is.conflict[f] || document.querySelector('#issue [data-act=pick][data-field=' + f + ']')?.revert?.()
	}
})`

// issueFailed, on data-on:datastar-fetch, marks the field of a request
// that reached no outcome (network, rate limit, a restart): "This did not
// go through" with Retry, and the person's input stays (MP-09.3). A 403
// brings its reason on the stream like any refusal. A request the frame's
// delegated handler sent marks every pending field: a retry sends the
// same op, which applies once.
const issueFailed = `((d) => {
	if (!['error', 'retries-failed'].includes(d.type) || d.argsRaw.status === '403' || !d.el?.closest?.('#issue')) return
	const msg = d.argsRaw.status === '429' ? 'Too many changes at once. Wait a moment.' : d.argsRaw.status === '503' ? ` + Unavailable + ` : 'This did not go through.'
	const f = d.el.dataset.field
	for (const g of f && f in $_is.ops ? [f] : Object.keys($_is.ops)) if ($_is.ops[g] !== '') { $_is.last[g] = $_is.ops[g]; $_is.ops[g] = ''; $_is.fail[g] = msg }
})(evt.detail)`

// The description's co-edited editor (FR-15, docs/editor.md). Edit
// fetches the editor's host, with the document as it is now, into the
// page (a read: GET /issues/{id}/description/editor); its data-init posts
// open, as a page's does. In the issue panel, Edit opens the issue's page
// with the editor open (?edit=description). Done closes it, once nothing
// waits to be saved (or the editor stopped), and ends the "is editing"
// indicator at once (MP-07.4).
const (
	// DescBodyID is the description as readers see it (the view
	// issue-desc:<id>), DescEditorID where the editor's host goes.
	DescBodyID   = "issue-desc-body"
	DescEditorID = "issue-desc-editor"

	descEditorSignals = `{_editor: {status: 'saved', stopped: '', unsaved: 0, problem: ''}, _carets: '[]', _pick: {kind: '', q: '', label: ''}}`
	descOpen          = `($_is.desc = true, $_editor.status = 'saved', $_editor.stopped = '', $_editor.problem = '', @get('/issues/' + $_is.id + '/description/editor', {retry: 'error', retryMaxCount: 5}))`
	descEditFn        = `!$_is.ro && $_is.at === 0 && !$_is.desc && (document.getElementById('issue-panel') ? location.assign('/' + $_is.key + '?edit=description') : ` + descOpen + `)`
	descEditInit      = `!$_is.ro && new URLSearchParams(location.search).get('edit') === 'description' && ` + descOpen
	descDone          = `$_is.desc && ($_editor.status === 'saving' && !$_editor.stopped ? ($_editor.problem = 'Your last changes are still being saved. Wait a moment.') : ` +
		`($_is.desc = false, $_editor.problem = '', document.getElementById('` + DescEditorID + `').replaceChildren(), ` +
		`@post('/cmd/issues/' + $_is.id + '/description/close', {payload: {tab: $_tab, op: ` + newOp + `}, requestCancellation: 'disabled', retryMaxCount: 2}), ` +
		`@post('/cmd/issues/' + $_is.id + '/typing', {payload: {tab: $_tab, op: ` + newOp + `, field: 'description', stop: true}, requestCancellation: 'disabled', retryMaxCount: 0})))`
)

// The comment box (FR-65): Ctrl or Cmd Enter sends; the draft stays in the
// tab across a reconnect and a reload (sessionStorage is per tab).
const (
	commentSend   = `$_is.draft.trim() !== '' && ` + issueSendFn + `('comment', $_is.draft, 0, '')`
	commentKeys   = `(evt.ctrlKey || evt.metaKey) && evt.key === 'Enter' ? (evt.preventDefault(), ` + commentSend + `) : evt.key === 'Escape' && $_is.mq !== false && (evt.preventDefault(), $_is.mq = false)`
	commentInput  = `(m => $_is.mq = m ? m[1] : false)(/(?:^|\s)@([^\s@]{0,20})$/.exec(el.value.slice(0, el.selectionStart))); try { sessionStorage.setItem('tickit-draft:' + $_is.id, el.value) } catch (e) {}`
	commentInit   = `(() => { try { $_is.draft = sessionStorage.getItem('tickit-draft:' + $_is.id) || $_is.draft } catch (e) {} })()`
	mentionShow   = `$_is.mq !== false && el.dataset.name.toLowerCase().includes($_is.mq.toLowerCase())`
	mentionsShown = `$_is.mq !== false`
)

// A comment being edited shows its editor (FR-66.1).
const commentEditing = `$_is.cedit === el.dataset.comment`

const issueDelete = issueSendFn + `('delete', '', 0, '')`

// PagePickID is the list of pages that "Link page" offers for its search
// (FR-102), which GET /issues/link-page patches; pagePick asks for it.
const (
	PagePickID = "issue-page-pick"
	pagePick   = `$_is.pq.trim().length < 2 ? document.getElementById('` + PagePickID + `').replaceChildren() : @get('/issues/link-page?q=' + encodeURIComponent($_is.pq.trim()), {requestCancellation: 'auto'})`
)

// The tabs of the issue's page: Comments, History, Activity (FR-68).
const tabKeys = `['ArrowRight', 'ArrowLeft'].includes(evt.key) && ((tabs) => { const i = tabs.indexOf(document.activeElement), n = tabs.length, t = tabs[(i + (evt.key === 'ArrowRight' ? 1 : n - 1)) % n]; t.focus(); t.click() })([...el.querySelectorAll('[role=tab]')])`

// The as-of slider (FR-70): the version it is at, 0 for now, watched as
// the view issue-at:<id>.<v> instead of the live one; "Now" goes back.
const (
	asOfInput = `((v) => { const at = v >= $_is.max ? 0 : v; if (at === $_is.at) return; const old = $_is.at; $_is.at = at; old && @post('/cmd/unwatch', {payload: {tab: $_tab, view: 'issue-at:' + $_is.id + '.' + old}, requestCancellation: 'disabled'}); at && @post('/cmd/watch', {payload: {tab: $_tab, view: 'issue-at:' + $_is.id + '.' + at}, requestCancellation: 'disabled'}) })(+el.value)`
	asOfNow   = `((old) => { $_is.at = 0; old && @post('/cmd/unwatch', {payload: {tab: $_tab, view: 'issue-at:' + $_is.id + '.' + old}, requestCancellation: 'disabled'}) })($_is.at)`
	asOfValue = `$_is.at || $_is.max`
)

// fieldExpr puts a field's name into an expression template: f is one of
// IssueFields, a constant.
func fieldExpr(tmpl, f string) string { return strings.ReplaceAll(tmpl, "%f", f) }

// Per-field expressions of the notes beside a control.
const (
	noteShown     = `$_is.msg.%f !== '' || $_is.fail.%f !== ''`
	noteConflict  = `$_is.conflict.%f !== false`
	noteRefused   = `$_is.msg.%f !== '' && $_is.conflict.%f === false`
	noteFailed    = `$_is.fail.%f !== ''`
	notePending   = `$_is.ops.%f !== ''`
	noteBusy      = `$_is.ops.%f !== '' ? 'true' : false`
	noteMsg       = `$_is.msg.%f`
	noteFail      = `$_is.fail.%f`
	noteAnyway    = `'Use ' + ($_is.conflict.%f.yours || 'mine') + ' anyway'`
	editShown     = `$_is.edit === '%f'`
	editHidden    = `$_is.edit !== '%f'`
	editReadonly  = `$_is.ops.%f !== ''`
	disabledWhile = `$_is.ro || $_is.at !== 0`
)

// The issue panel (FR-13.2): a click on a link with data-issue-panel opens
// the issue beside the list. The panel empties its regions, makes the
// presence row's element for the issue, takes the issue's URL
// (history.pushState) and adds the issue's views to the tab's stream with
// POST /cmd/watch. Closing it takes them off and gives the list its URL
// back; the list never left, so its filter and scroll stay. A click with
// a modifier key follows the link.
const (
	panelRegions = `['issue-main', 'issue-desc', 'issue-desc-body', 'issue-related', 'issue-side', 'issue-comments', 'issue-history', 'issue-activity', 'issue-mentions', 'issue-asof', 'issue-typing-summary', 'issue-typing-description', 'issue-typing-comment'].forEach(i => document.getElementById(i)?.replaceChildren())`
	panelUnwatch = `((id, at) => { if (!id) return; for (const v of ['issue:' + id, 'issue-desc:' + id, 'issue-typing:' + id].concat(at ? ['issue-at:' + id + '.' + at] : [])) @post('/cmd/unwatch', {payload: {tab: $_tab, view: v}, requestCancellation: 'disabled'}) })($_is.id, $_is.at)`
	panelOpen    = `((a) => {
	if (!a || evt.defaultPrevented || evt.button !== 0 || evt.ctrlKey || evt.metaKey || evt.shiftKey || evt.altKey) return
	evt.preventDefault()
	const id = a.getAttribute('data-issue-panel'), d = document.getElementById('issue-panel')
	if (!d || $_is.id === id && d.open) return
	if ($_is.id && $_is.id !== id) ` + panelUnwatch + `
	$_is.id = id
	$_is.key = (a.getAttribute('href') || '').replace(/^\//, '')
	$_is.edit = ''; $_is.at = 0; $_is.max = 1; $_is.tab = 'comments'; $_is.cedit = ''; $_is.mq = false; $_is.lq = ''; $_is.sub = ''
	Object.keys($_is.msg).forEach(f => { $_is.msg[f] = ''; $_is.fail[f] = ''; $_is.conflict[f] = false })
	try { $_is.draft = sessionStorage.getItem('tickit-draft:' + id) || '' } catch (e) { $_is.draft = '' }
	` + panelRegions + `
	document.getElementById('issue-presence-slot').replaceChildren(Object.assign(document.createElement('ul'), {id: 'presence-issue-' + id, className: 'tk-presence'}))
	if (d.open) history.replaceState({issuePanel: id}, '', a.href)
	else { $_is.back = location.href; history.pushState({issuePanel: id}, '', a.href); d.showModal() }
	for (const v of ['issue:' + id, 'issue-desc:' + id, 'issue-typing:' + id]) @post('/cmd/watch', {payload: {tab: $_tab, view: v}, requestCancellation: 'disabled'})
	@post('/cmd/notify/seen', {payload: {tab: $_tab, op: ` + newOp + `, subject: id, kind: 'issue'}, requestCancellation: 'disabled'})
})(evt.target.closest?.('a[` + IssueLink + `]'))`
	panelClose = `$_is.id && (` + panelUnwatch + `, $_is.id = '', $_is.at = 0, $_is.pop ? ($_is.pop = false) : history.state?.issuePanel ? history.back() : $_is.back && history.replaceState(null, '', $_is.back))`
	panelPop   = `((d) => d?.open && !history.state?.issuePanel && ($_is.pop = true, d.close()))(document.getElementById('issue-panel'))`
	panelPage  = `'/' + $_is.key`
)

// The create dialog (FR-10). Its state is _new; the answer of a created
// issue is $_goto (the issue's page) or, on a page with the issue panel,
// $_new.created, which opens the new issue in the panel.
const (
	createSubmit = `$_new.op === '' && ($_new.msg = '', $_new.field = '', $_new.fail = '', $_new.created = '', $_new.op = ` + newOp + `, @post('/cmd/issues/create', {payload: {tab: $_tab, op: $_new.op, project: $_new.project, type: $_new.type, summary: $_new.summary, description: $_new.description, priority: $_new.priority, assignee: $_new.assignee, labels: $_new.labels, parent: $_new.parent, status: $_new.status, stay: !!document.getElementById('issue-panel')}, retryMaxCount: 2}))`
	createSettle = `$_new.op !== '' && $_outcomes[$_new.op] && ((o) => { $_new.op = ''; if (!o.ok) { $_new.msg = o.msg || 'This was refused.'; $_new.field = o.field || '' } })($_outcomes[$_new.op])`
	createFailed = `evt.detail.el === el && ['error', 'retries-failed'].includes(evt.detail.type) && evt.detail.argsRaw.status !== '403' && ($_new.op = '', $_new.fail = evt.detail.argsRaw.status === '429' ? 'Too many changes at once. Wait a moment.' : 'This did not go through.')`
	createdOpen  = `$_new.created && ((c) => { $_new.created = ''; $_new.summary = ''; $_new.description = ''; $_new.labels = ''; $_new.parent = ''; el.close(); const a = Object.assign(document.createElement('a'), {href: '/' + c.split(' ')[1]}); a.setAttribute('` + IssueLink + `', c.split(' ')[0]); a.hidden = true; document.body.append(a); a.click(); a.remove() })($_new.created)`
	createBusy   = `$_new.op !== '' ? 'true' : false`
	createType   = `evt.target.name === 'type' && ($_new.type = evt.target.value)`
	createPrio   = `evt.target.name === 'priority' && ($_new.priority = evt.target.value)`
	createKeys   = `(evt.ctrlKey || evt.metaKey) && evt.key === 'Enter' && (evt.preventDefault(), ` + createSubmit + `)`
	// CreateWithStatus opens the create dialog for a board column's "+":
	// on a button with data-status (a status id) and data-project.
	CreateWithStatus = `$_new.status = el.dataset.status || '', $_new.where = document.querySelector('.tk-lane[data-lane="' + $_new.status + '"]')?.dataset.name || '', el.dataset.project && ($_new.project = el.dataset.project, (document.getElementById('create-project') || {}).value = el.dataset.project), document.getElementById('` + CreateID + `').showModal(), setTimeout(() => document.getElementById('create-summary')?.focus())`
	// createClosed forgets a column's status, so the header's Create puts
	// the next issue at the start again.
	createClosed = `$_new.status = '', $_new.where = ''`
)

// createSignals are the create dialog's state, with the project it
// preselects.
func createSignals(project, status string) string {
	b, _ := json.Marshal(map[string]any{"_new": map[string]any{
		"op": "", "msg": "", "field": "", "fail": "", "created": "",
		"project": project, "type": "task", "summary": "", "description": "", "priority": "medium",
		"assignee": "", "labels": "", "parent": "", "status": status, "where": "",
	}})
	return string(b)
}
