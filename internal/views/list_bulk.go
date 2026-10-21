package views

import (
	"strconv"

	"github.com/a-h/templ"

	"github.com/zweiundeins/tickit/internal/tracker"
)

// Bulk edit in the issue list (FR-58): the selection is the tab's, the
// signal $_bulk.ids (issue ids, at most tracker.MaxBatch), kept across the
// list's pages and filters. The shared rows carry a checkbox each
// (.list-check, data-i), the head one for the page (.list-check-all);
// the list's container sets them from the selection after every frame and
// every change of it. The bar above the list posts the change.

// BulkID is the id of the bulk edit's bar; BulkDeleteID of its dialog.
const (
	BulkID       = "list-bulk"
	BulkDeleteID = "bulk-delete"
)

const bulkNone = "Select issues to change several at once."

var (
	bulkSignals = `{_bulk: {ids: [], op: '', msg: '', ok: true, field: '', value: '', ref: ''}}`
	bulkCount   = `$_bulk.ids.length === 0 ? '` + bulkNone + `' : $_bulk.ids.length === 1 ? '1 issue selected' : $_bulk.ids.length + ' issues selected'`
	bulkEmpty   = `$_bulk.ids.length === 0 || $_bulk.op !== ''`
	// bulkDeleteText is the delete dialog's question.
	bulkDeleteText = `$_bulk.ids.length === 1 ? 'The selected issue will be deleted.' : $_bulk.ids.length + ' issues will be deleted.'`
)

func bulkOff() templ.Attributes { return templ.Attributes{"data-attr:disabled": bulkEmpty} }

// bulkChoice is a control that changes field to value for the selection.
func bulkChoice(field, value string) templ.Attributes {
	return templ.Attributes{"data-bulk": field, "data-value": value}
}

// bulkLabel adds the label typed in #bulk-label, or removes it (ref).
func bulkLabel(ref string) templ.Attributes {
	return templ.Attributes{"data-bulk": tracker.FieldLabels, "data-ref": ref}
}

func sprintOption(s tracker.SprintOption) string {
	label := s.ProjectKey + " · " + s.Name
	if s.State == tracker.SprintActive {
		label += " (active)"
	}
	return label
}

// bulkInit, on the list's container: el.sync checks the rows' boxes from
// the selection, again after every frame.
const bulkInit = `el.sync = () => {
	const ids = $_bulk.ids, boxes = [...el.querySelectorAll('.list-check')]
	let n = 0
	for (const b of boxes) { b.checked = ids.includes(b.dataset.i); n += b.checked }
	const all = el.querySelector('.list-check-all')
	if (all) { all.checked = n > 0 && n === boxes.length; all.indeterminate = n > 0 && n < boxes.length }
}
new MutationObserver(() => el.sync()).observe(el, {childList: true, subtree: true, attributes: true, attributeFilter: ['data-i']})
el.sync()`

// bulkSync checks the boxes again when the selection changes.
const bulkSync = `$_bulk.ids.length >= 0 && el.sync && el.sync()`

// bulkPick, on the list's container: a row's box or the page's box
// changes the selection, up to its cap.
var bulkPick = `const t = evt.target
if (!t.matches('.list-check, .list-check-all')) return
const boxes = t.matches('.list-check') ? [t] : [...el.querySelectorAll('.list-check')]
const ids = $_bulk.ids.filter((id) => !boxes.some((b) => b.dataset.i === id))
if (t.checked) for (const b of boxes) {
	if (ids.length >= ` + strconv.Itoa(tracker.MaxBatch) + `) { $_bulk.ok = false; $_bulk.msg = 'You can change ` + strconv.Itoa(tracker.MaxBatch) + ` issues at once.'; break }
	ids.push(b.dataset.i)
}
$_bulk.ids = ids
el.sync()`

// bulkClick, on the bar: a choice posts the change for the selection;
// one at a time.
var bulkClick = `const b = evt.target.closest('[data-bulk], [data-bulk-clear]')
if (!b || !el.contains(b) || b.disabled) return
if (b.dataset.bulkClear) { $_bulk.ids = []; $_bulk.msg = ''; return }
if ($_bulk.op !== '' || $_bulk.ids.length === 0) return
let value = b.dataset.value || ''
if (b.dataset.bulk === '` + tracker.FieldLabels + `') {
	const i = document.getElementById('bulk-label')
	value = i.value.trim()
	if (!value) { i.focus(); return }
}
b.closest('[popover]')?.hidePopover()
b.closest('dialog')?.close()
$_bulk.field = b.dataset.bulk
$_bulk.value = value
$_bulk.ref = b.dataset.ref || ''
$_bulk.msg = ''
$_problem = ''
$_bulk.op = ` + newOp + `
@post('/cmd/issues/bulk', {payload: {tab: $_tab, op: $_bulk.op, issues: $_bulk.ids, field: $_bulk.field, value: $_bulk.value, ref: $_bulk.ref}, requestCancellation: 'disabled'})`

// bulkLabelEnter adds the typed label on Enter.
const bulkLabelEnter = `evt.key === 'Enter' && (evt.preventDefault(), el.closest('[popover]').querySelector('[data-ref="add"]').click())`

// bulkSettle shows the outcome: how many changed and, one a line, the
// issues refused and why (FR-58.2), which stay selected.
const bulkSettle = `const op = $_bulk.op
if (!op) return
const o = $_outcomes[op]
if (!o) return
$_bulk.ok = o.ok
$_bulk.msg = o.msg || (o.ok ? 'Done.' : 'This did not go through.')
if (o.data && o.data.refused) $_bulk.ids = o.data.refused.map((r) => r.id)
if (o.ok && $_bulk.field === '` + tracker.FieldLabels + `') document.getElementById('bulk-label').value = ''
$_bulk.op = ''`

var bulkFailed = Failed("_bulk.op")
