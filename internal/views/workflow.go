package views

import (
	"strings"

	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/ui"
)

// A project's workflow and board columns in its settings (FR-36, FR-37,
// FR-43): the shared region WorkflowRegion of the view settings:<KEY>,
// and the forms and dialogs of the page, which the region's rows fill
// from their data attributes when they open one.

// WorkflowID is the id of the workflow's region.
const WorkflowID = "project-workflow"

// WorkflowLive is what the workflow's region shows: the statuses in
// order with their rules and columns, the board's columns, and how many
// issues each status holds.
type WorkflowLive struct {
	Statuses []tracker.WorkflowStatus
	Columns  []tracker.Column
	Held     map[string]int
}

func categoryOptions(current tracker.Category) []ui.Option {
	out := make([]ui.Option, len(tracker.Categories))
	for i, c := range tracker.Categories {
		out[i] = ui.Option{Value: string(c), Label: c.Name(), Selected: c == current}
	}
	return out
}

func columnOptions(cols []tracker.Column, current string) []ui.Option {
	out := []ui.Option{{Value: "", Label: "Not on the board", Selected: current == ""}}
	for _, c := range cols {
		out = append(out, ui.Option{Value: c.ID, Label: c.Name, Selected: c.ID == current})
	}
	return out
}

// followsText says which statuses a status may follow (FR-37.1).
func followsText(w WorkflowLive, st tracker.WorkflowStatus) string {
	if st.Any {
		return "Any status"
	}
	var names []string
	for _, o := range w.Statuses {
		for _, id := range st.Follows {
			if o.ID == id {
				names = append(names, o.Name)
			}
		}
	}
	if len(names) == 0 {
		return "None: only new issues start here"
	}
	return strings.Join(names, ", ")
}

func joinIDs(ids []string) string { return strings.Join(ids, " ") }

func columnStatuses(c tracker.Column) string {
	if len(c.Statuses) == 0 {
		return "No status: cards cannot go here"
	}
	names := make([]string, len(c.Statuses))
	for i, s := range c.Statuses {
		names[i] = s.Name
	}
	return strings.Join(names, ", ")
}

// The workflow's expressions. Each action reads the project's key from
// the page and the status or column from its row (data-status,
// data-column); the dialogs keep what they edit in $_wf.
const wfRow = `el.closest('tr').dataset`

var (
	wfSignals = `{_wf: {op: '', msg: '', status: '', column: '', was: '', name: '', held: 0, to: '', any: true, limit: '', sname: '', scat: 'todo', cname: ''}}`

	wfAddStatus = `$_wf.sname.trim() && ` + Act("_wf.op", "/cmd/projects/status", "project: "+settingsProject+", name: $_wf.sname, category: $_wf.scat")
	wfAddColumn = `$_wf.cname.trim() && ` + Act("_wf.op", "/cmd/projects/column", "project: "+settingsProject+", name: $_wf.cname")
	wfCategory  = Act("_wf.op", "/cmd/projects/status/category", "project: "+settingsProject+", status: "+wfRow+".status, category: el.value")
	wfMap       = Act("_wf.op", "/cmd/projects/status/column", "project: "+settingsProject+", status: "+wfRow+".status, column: el.value")
	wfMove      = Act("_wf.op", "/cmd/projects/status/move", "project: "+settingsProject+", status: "+wfRow+".status, earlier: el.dataset.earlier === '1'")
	wfColMove   = Act("_wf.op", "/cmd/projects/column/move", "project: "+settingsProject+", column: "+wfRow+".column, earlier: el.dataset.earlier === '1'")
	wfColRemove = Act("_wf.op", "/cmd/projects/column/remove", "project: "+settingsProject+", column: "+wfRow+".column")

	// The dialogs: a row's button fills $_wf and opens one.
	wfOpen       = `const d = ` + wfRow + `; $_wf.msg = ''; $_wf.status = d.status || ''; $_wf.column = d.column || ''; $_wf.was = d.name; $_wf.name = d.name; `
	wfRenameOpen = wfOpen + `document.getElementById('wf-rename').showModal()`
	wfColumnOpen = wfOpen + `$_wf.limit = d.limit === '0' ? '' : d.limit; document.getElementById('wf-column').showModal()`
	// The rule's choices and the removal's targets are the region's other
	// rows, as they are when the dialog opens.
	wfRuleOpen = wfOpen + `$_wf.any = d.any !== undefined
const from = (d.follows || '').split(' '), me = el.closest('tr')
document.getElementById('wf-rule-from').replaceChildren(...[...document.querySelectorAll('#wf-statuses tr[data-status]')].filter((r) => r !== me).map((r) => {
	const l = document.createElement('label'), i = document.createElement('input'), t = document.createElement('span')
	l.className = 'tk-check'; i.className = 'tk-check__box'; i.type = 'checkbox'; i.value = r.dataset.status; i.checked = $_wf.any || from.includes(r.dataset.status)
	t.className = 'tk-check__text'; t.textContent = r.dataset.name
	l.append(i, t)
	return l
}))
document.getElementById('wf-rule').showModal()`
	wfRemoveOpen = wfOpen + `$_wf.held = +d.held
const me = el.closest('tr'), sel = document.getElementById('wf-move-to')
sel.replaceChildren(...[...document.querySelectorAll('#wf-statuses tr[data-status]')].filter((r) => r !== me).map((r) => new Option(r.dataset.name, r.dataset.status)))
$_wf.to = sel.value
document.getElementById('wf-remove').showModal()`

	wfRename = Act("_wf.op", "/cmd/projects/status/rename", "project: "+settingsProject+", status: $_wf.status, name: $_wf.name")
	wfRule   = Act("_wf.op", "/cmd/projects/status/rule", "project: "+settingsProject+", status: $_wf.status, any: $_wf.any, from: $_wf.any ? [] : [...document.querySelectorAll('#wf-rule-from input:checked')].map((i) => i.value)")
	wfRemove = Act("_wf.op", "/cmd/projects/status/remove", "project: "+settingsProject+", status: $_wf.status, move_to: $_wf.held ? $_wf.to : ''")
	wfColumn = Act("_wf.op", "/cmd/projects/column/edit", "project: "+settingsProject+", column: $_wf.column, name: $_wf.name, limit: +($_wf.limit || 0)")

	wfSettled = Settle("_wf.op", "o.ok ? ($_wf.msg = '', $_wf.sname = '', $_wf.cname = '', document.querySelectorAll('.tk-wf-dialog[open]').forEach((d) => d.close())) : ($_wf.msg = o.msg)")
	wfFailed  = Failed("_wf.op")

	wfRemoveText = `$_wf.held ? 'Its ' + ($_wf.held === 1 ? 'issue moves' : $_wf.held + ' issues move') + ' to the status you pick, each with the change in its history.' : 'No issue holds it.'`
)
