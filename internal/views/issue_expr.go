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
	"summary", "description", "type", "status", "priority", "assignee", "parent", "due", "points",
	"estimate", "remaining", "sprint", "labels", "components", "watch", "link", "unlink", "subtask",
	"comment", "cedit", "cdelete", "chide", "delete",
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
		"edit": "", "val": "", "orig": "", "seen": 0, "ref": "",
		"ops": per(""), "last": per(""), "msg": per(""), "fail": per(""), "conflict": per(false), "want": per(""), "sent": per(0), "refs": per(""),
		"aq": "", "lq": "", "mseen": 0, "sub": "", "link": map[string]any{"type": "blocks", "key": ""},
		"draft": "", "mq": false, "cedit": "", "ctext": "",
		"tab": "comments", "at": 0, "max": 1, "copied": false, "back": "", "pop": false,
	}})
	return string(b)
}

// curVersion is the issue's version the page shows now: what a choice
// sends as Seen (MP-08.1).
const curVersion = `+(document.getElementById('issue-main')?.dataset.v || 0)`

// issueSendFn sends field f with value v, seen s and ref r, unless f is
// pending: a new op id in ops[f], the value kept in want[f] for a retry.
const issueSendFn = `((f, v, s, r) => (!$_is.ro || f === 'comment' || f === 'cedit' || f === 'cdelete') && $_is.ops[f] === '' && ($_is.msg[f] = '', $_is.fail[f] = '', $_is.conflict[f] = false, $_is.want[f] = v, $_is.sent[f] = s, $_is.refs[f] = r, $_is.ops[f] = ` + newOp + `, @post('/cmd/issues/' + $_is.id + '/' + f, {payload: {tab: $_tab, op: $_is.ops[f], seen: s, value: v, ref: r}, retryMaxCount: 2})))`

// issueChoose, on a control with data-field, data-value and data-ref
// (a menu item, a label's remove button), sends its choice against the
// version shown, and closes the menu it is in.
const issueChoose = `((m) => (m?.hidePopover(), ` + issueSendFn + `(el.dataset.field, el.dataset.value ?? '', (m && $_is.mseen) || ` + curVersion + `, el.dataset.ref ?? '')))(el.closest('[popover]'))`

// menuOpened keeps the version a menu showed when it opened: a choice in
// it sends that version, so a change someone made while the menu was open
// is a conflict, not overwritten (MP-08.2). On a menu's button.
const menuOpened = `$_is.mseen = ` + curVersion

// issueRetry sends a request that reached no outcome again with its op
// id, so that it applies once (MP-09.3, MP-09.4). On a button with
// data-field.
const issueRetry = `((f) => $_is.ops[f] === '' && $_is.last[f] !== '' && ($_is.fail[f] = '', $_is.ops[f] = $_is.last[f], @post('/cmd/issues/' + $_is.id + '/' + f, {payload: {tab: $_tab, op: $_is.ops[f], seen: $_is.sent[f], value: $_is.want[f], ref: $_is.refs[f]}, retryMaxCount: 2})))(el.dataset.field)`

// issueAnyway sends the refused value again against the version the
// conflict reports (MP-08.3: "Use Critical anyway"). On a button with
// data-field.
const issueAnyway = `((f) => $_is.conflict[f] && ($_is.edit === f && (f === 'summary' || f === 'description' || f === 'due' || f === 'points' || f === 'estimate' || f === 'remaining') ? ($_is.seen = $_is.conflict[f].version, ` + issueSendFn + `(f, $_is.val, $_is.seen, '')) : ` + issueSendFn + `(f, $_is.want[f], $_is.conflict[f].version, $_is.refs[f])))(el.dataset.field)`

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
		if (f === 'cedit') $_is.cedit = ''
		if (f === 'delete') document.getElementById('issue-delete')?.close()
	} else {
		$_is.msg[f] = o.msg || 'This was refused.'
		$_is.conflict[f] = o.data && o.data.kind === 'conflict' ? o.data : false
	}
})`

// issueFailed, on data-on:datastar-fetch, marks the field of a request
// that reached no outcome (network, rate limit, a restart): "This did not
// go through" with Retry, and the person's input stays (MP-09.3). A 403
// brings its reason on the stream like any refusal.
const issueFailed = `((f, d) => f && f in $_is.ops && $_is.ops[f] !== '' && ['error', 'retries-failed'].includes(d.type) && d.argsRaw.status !== '403' && ($_is.last[f] = $_is.ops[f], $_is.ops[f] = '', $_is.fail[f] = d.argsRaw.status === '429' ? 'Too many changes at once. Wait a moment.' : d.argsRaw.status === '503' ? 'The server is restarting. Try again in a moment.' : 'This did not go through.'))(evt.detail.el?.dataset?.field, evt.detail)`

// issueEdit opens the editor of a field: the text the page shows (the
// element data-src names) goes into $_is.val, and the version shown into
// $_is.seen, which the save sends (MP-08.1). On the field's display, with
// data-field and data-src.
const issueEdit = `!$_is.ro && $_is.at === 0 && $_is.ops[el.dataset.field] === '' && ($_is.edit = el.dataset.field, $_is.val = $_is.orig = (document.getElementById(el.dataset.src)?.textContent ?? '').trim(), $_is.seen = ` + curVersion + `, $_is.msg[el.dataset.field] = '', $_is.conflict[el.dataset.field] = false, setTimeout(() => document.getElementById('issue-edit-' + el.dataset.field)?.focus()))`

// issueSave saves the open editor: nothing changed closes it (FR-14.3).
const issueSave = `$_is.edit !== '' && $_is.ops[$_is.edit] === '' && ($_is.val.trim() === $_is.orig ? ($_is.edit = '', ` + issueStopTyping + `) : ` + issueSendFn + `($_is.edit, $_is.val, $_is.seen, ''))`

// issueCancel closes the editor without saving, unless it waits for an
// outcome.
const issueCancel = `$_is.edit !== '' && $_is.ops[$_is.edit] === '' && (` + issueStopTyping + `, $_is.msg[$_is.edit] = '', $_is.fail[$_is.edit] = '', $_is.conflict[$_is.edit] = false, $_is.edit = '')`

// issueEditKeys: Enter saves a one-line field, Ctrl or Cmd Enter the
// description; Escape cancels (FR-14.1).
const issueEditKeys = `evt.key === 'Escape' ? (evt.preventDefault(), ` + issueCancel + `) : evt.key === 'Enter' && (el.tagName !== 'TEXTAREA' || evt.ctrlKey || evt.metaKey) && (evt.preventDefault(), ` + issueSave + `)`

// issueEditOut saves when the focus leaves the editor for the page: a
// click outside saves (FR-14.1).
const issueEditOut = `!el.contains(evt.relatedTarget) && ` + issueSave

// typingField is the field whose indicator a keystroke refreshes.
const issueTyping = `$_is.at === 0 && @post('/cmd/issues/' + $_is.id + '/typing', {payload: {tab: $_tab, op: ` + newOp + `, field: el.dataset.typing === 'edit' ? $_is.edit : el.dataset.typing, stop: false}, retryMaxCount: 0})`

// issueStopTyping ends the indicator of the open editor at once (MP-07.4).
const issueStopTyping = `(($_is.edit === 'summary' || $_is.edit === 'description') && @post('/cmd/issues/' + $_is.id + '/typing', {payload: {tab: $_tab, op: ` + newOp + `, field: $_is.edit, stop: true}, retryMaxCount: 0}))`

// The comment box (FR-65): Ctrl or Cmd Enter sends; the draft stays in the
// tab across a reconnect and a reload (sessionStorage is per tab).
const (
	commentSend   = `$_is.draft.trim() !== '' && ` + issueSendFn + `('comment', $_is.draft, 0, '')`
	commentKeys   = `(evt.ctrlKey || evt.metaKey) && evt.key === 'Enter' ? (evt.preventDefault(), ` + commentSend + `) : evt.key === 'Escape' && $_is.mq !== false && (evt.preventDefault(), $_is.mq = false)`
	commentInput  = `(m => $_is.mq = m ? m[1] : false)(/(?:^|\s)@([^\s@]{0,20})$/.exec(el.value.slice(0, el.selectionStart))); try { sessionStorage.setItem('tickit-draft:' + $_is.id, el.value) } catch (e) {}`
	commentInit   = `(() => { try { $_is.draft = sessionStorage.getItem('tickit-draft:' + $_is.id) || $_is.draft } catch (e) {} })()`
	mentionPick   = `((t) => { const pos = t.selectionStart, before = t.value.slice(0, pos).replace(/@[^\s@]*$/, '@' + el.dataset.id + ' '); $_is.draft = before + t.value.slice(pos); $_is.mq = false; t.focus(); setTimeout(() => t.setSelectionRange(before.length, before.length)) })(document.getElementById('issue-comment'))`
	mentionShow   = `$_is.mq !== false && el.dataset.name.toLowerCase().includes($_is.mq.toLowerCase())`
	mentionsShown = `$_is.mq !== false`
)

// Editing one's own comment (FR-66.1): its markup goes into $_is.ctext.
const (
	commentEdit     = `$_is.cedit = el.dataset.comment, $_is.ctext = (document.getElementById('comment-src-' + el.dataset.comment)?.textContent ?? '').trim(), setTimeout(() => document.getElementById('comment-edit-' + el.dataset.comment)?.focus())`
	commentEditing  = `$_is.cedit === el.dataset.comment`
	commentEditSave = `$_is.ctext.trim() !== '' && ` + issueSendFn + `('cedit', $_is.ctext, 0, el.dataset.comment)`
	commentEditKeys = `evt.key === 'Escape' ? ($_is.cedit = '', $_is.msg.cedit = '') : (evt.ctrlKey || evt.metaKey) && evt.key === 'Enter' && (evt.preventDefault(), ` + commentEditSave + `)`
	commentDelete   = issueSendFn + `('cdelete', '', 0, el.dataset.comment)`
	commentHide     = issueSendFn + `('chide', el.dataset.value, 0, el.dataset.comment)`
)

// The assignee picker's search (FR-17.1) and the label field (FR-23.1).
const (
	assigneeMatch = `$_is.aq === '' || el.dataset.name.toLowerCase().includes($_is.aq.toLowerCase())`
	labelKeys     = `evt.key === 'Enter' && $_is.lq.trim() !== '' && (evt.preventDefault(), ` + issueSendFn + `('labels', $_is.lq.trim(), 0, 'add'))`
	subtaskKeys   = `evt.key === 'Enter' && $_is.sub.trim() !== '' && (evt.preventDefault(), ` + issueSendFn + `('subtask', $_is.sub.trim(), 0, ''))`
	subtaskAdd    = `$_is.sub.trim() !== '' && ` + issueSendFn + `('subtask', $_is.sub.trim(), 0, '')`
	linkAdd       = `$_is.link.key.trim() !== '' && ` + issueSendFn + `('link', $_is.link.key.trim(), 0, $_is.link.type)`
	linkKeys      = `evt.key === 'Enter' && (evt.preventDefault(), ` + linkAdd + `)`
	issueDelete   = issueSendFn + `('delete', '', 0, '')`
	copyLink      = `navigator.clipboard?.writeText(location.origin + '/' + $_is.key).then(() => { $_is.copied = true; setTimeout(() => $_is.copied = false, 1500) })`
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
	panelRegions = `['issue-main', 'issue-desc', 'issue-related', 'issue-side', 'issue-comments', 'issue-history', 'issue-activity', 'issue-mentions', 'issue-asof', 'issue-typing-summary', 'issue-typing-description', 'issue-typing-comment'].forEach(i => document.getElementById(i)?.replaceChildren())`
	panelUnwatch = `((id, at) => { if (!id) return; for (const v of ['issue:' + id, 'issue-typing:' + id].concat(at ? ['issue-at:' + id + '.' + at] : [])) @post('/cmd/unwatch', {payload: {tab: $_tab, view: v}, requestCancellation: 'disabled'}) })($_is.id, $_is.at)`
	panelOpen    = `((a) => {
	if (!a || evt.defaultPrevented || evt.button !== 0 || evt.ctrlKey || evt.metaKey || evt.shiftKey || evt.altKey) return
	evt.preventDefault()
	const id = a.getAttribute('data-issue-panel'), d = document.getElementById('issue-panel')
	if (!d || $_is.id === id && d.open) return
	if ($_is.id && $_is.id !== id) ` + panelUnwatch + `
	$_is.id = id
	$_is.key = (a.getAttribute('href') || '').replace(/^\//, '')
	$_is.edit = ''; $_is.at = 0; $_is.max = 1; $_is.tab = 'comments'; $_is.cedit = ''; $_is.mq = false; $_is.aq = ''; $_is.lq = ''; $_is.sub = ''
	Object.keys($_is.msg).forEach(f => { $_is.msg[f] = ''; $_is.fail[f] = ''; $_is.conflict[f] = false })
	try { $_is.draft = sessionStorage.getItem('tickit-draft:' + id) || '' } catch (e) { $_is.draft = '' }
	` + panelRegions + `
	document.getElementById('issue-presence-slot').replaceChildren(Object.assign(document.createElement('ul'), {id: 'presence-issue-' + id, className: 'tk-presence'}))
	if (d.open) history.replaceState({issuePanel: id}, '', a.href)
	else { $_is.back = location.href; history.pushState({issuePanel: id}, '', a.href); d.showModal() }
	for (const v of ['issue:' + id, 'issue-typing:' + id]) @post('/cmd/watch', {payload: {tab: $_tab, view: v}, requestCancellation: 'disabled'})
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
	CreateWithStatus = `$_new.status = el.dataset.status || '', el.dataset.project && ($_new.project = el.dataset.project), document.getElementById('` + CreateID + `').showModal(), setTimeout(() => document.getElementById('create-summary')?.focus())`
)

// createSignals are the create dialog's state, with the project it
// preselects.
func createSignals(project, status string) string {
	b, _ := json.Marshal(map[string]any{"_new": map[string]any{
		"op": "", "msg": "", "field": "", "fail": "", "created": "",
		"project": project, "type": "task", "summary": "", "description": "", "priority": "medium",
		"assignee": "", "labels": "", "parent": "", "status": status,
	}})
	return string(b)
}
