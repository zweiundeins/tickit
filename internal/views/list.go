package views

import (
	"net/url"
	"strconv"
	"time"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/ui"
)

// The issue list (FR-50 to FR-55): a filter form in the document and the
// list itself, a shared view keyed by its normalised query (MP-11), which
// the stream patches as #issue-list.

// IssueListID is the id of the list's region, which its view patches.
const IssueListID = "issue-list"

// ListFormID is the id of the list's filter form.
const ListFormID = "list-filter"

// IssueList is a page of a list as its shared view renders it: nothing
// about the viewer.
type IssueList struct {
	Results tracker.Results
	People  map[string]people.Person
	Sort    tracker.Sort
	// Values is the list's query with "me" resolved, for the links of its
	// pages (the page's script keeps "me" in the URL).
	Values   url.Values
	Filtered bool
	Now      time.Time
}

// faces are the people the rows show, for the region's FaceStyles.
func (l IssueList) faces() []ui.Person {
	var out []ui.Person
	for _, i := range l.Results.Issues {
		for _, id := range []string{i.Assignee, i.Reporter} {
			if id != "" {
				out = append(out, UIPerson(l.People[id]))
			}
		}
	}
	return out
}

// PageHref is the address of page n of the list.
func (l IssueList) PageHref(n int) string {
	v := url.Values{}
	for k, x := range l.Values {
		v[k] = x
	}
	v.Del("page")
	if n > 1 {
		v.Set("page", strconv.Itoa(n))
	}
	return listHref(v)
}

func listHref(v url.Values) string {
	if len(v) == 0 {
		return "/issues"
	}
	return "/issues?" + v.Encode()
}

// dir is aria-sort of a column: the direction the list sorts by it.
func (l IssueList) dir(by string) string {
	s := l.Sort
	if !s.Valid() {
		s = tracker.DefaultSort
	}
	switch {
	case s.By != by:
		return ""
	case s.Desc:
		return "descending"
	}
	return "ascending"
}

// sortColumn is a column of the list and what it sorts by.
type sortColumn struct{ Label, By, Align string }

var listColumns = []sortColumn{
	{"Type", tracker.SortType, ""},
	{"Key", tracker.SortKey, ""},
	{"Summary", tracker.SortSummary, ""},
	{"Priority", tracker.SortPriority, ""},
	{"Status", tracker.SortStatus, ""},
	{"Assignee", tracker.SortAssignee, ""},
	{"Reporter", tracker.SortReporter, ""},
	{"Project", tracker.SortProject, ""},
	{"Created", tracker.SortCreated, "end"},
	{"Updated", tracker.SortUpdated, "end"},
	{"Due", tracker.SortDue, "end"},
	{"Resolved", tracker.SortResolved, "end"},
}

// LabelHref is the issue list of the issues with a label (FR-23.2).
func LabelHref(label string) string { return "/issues?label=" + url.QueryEscape(label) }

// Category is a status category as internal/ui draws it.
func Category(c tracker.Category) ui.StatusCategory {
	switch c {
	case tracker.InProgress:
		return ui.InProgress
	case tracker.Done:
		return ui.Done
	}
	return ui.ToDo
}

// due is an issue's due date as a time, zero for none.
func due(i tracker.Issue) time.Time {
	t, _ := time.Parse(time.DateOnly, i.Due)
	return t
}

func issueCount(n int) string {
	if n == 1 {
		return "1 issue"
	}
	return strconv.Itoa(n) + " issues"
}

// ListPage is the list's page: the filter form, the saved filters and the
// list.
type ListPage struct {
	List    IssueList
	Filter  tracker.Filter // as the URL has it, "me" kept
	Sort    tracker.Sort
	Page    int
	Options tracker.FilterOptions
	People  map[string]people.Person // of Options.People
	Saved   []tracker.SavedFilter
	// Query is the list's query string, for the export's link.
	Query string
	// Unwatched: the stream has no view of this list yet (a query too long
	// for a view's name that no action stored): the page posts it once.
	Unwatched bool
	// Me and Moderates are for the issue panel (views.IssuePanel).
	Me        string
	Moderates bool
	Deletes   Deletes
}

// filterOption is a value a filter offers.
type filterOption struct {
	Value, Label string
	Checked      bool
}

// filterGroup is a filter of the form: its name in the URL and its values.
type filterGroup struct {
	ID, Label, Name string
	Options         []filterOption
}

func (g filterGroup) count() int {
	n := 0
	for _, o := range g.Options {
		if o.Checked {
			n++
		}
	}
	return n
}

func checked(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// groups are the form's filters, each with the model's values (FR-51.2).
func (p ListPage) groups() []filterGroup {
	f := p.Filter
	var g []filterGroup
	add := func(id, label, name string, sel []string, opts ...filterOption) {
		for i := range opts {
			opts[i].Checked = checked(sel, opts[i].Value)
		}
		g = append(g, filterGroup{ID: "list-f-" + id, Label: label, Name: name, Options: opts})
	}
	var o []filterOption
	for _, pr := range p.Options.Projects {
		o = append(o, filterOption{Value: pr.Key, Label: pr.Name + " (" + pr.Key + ")"})
	}
	add("project", "Project", "project", f.Projects, o...)
	o = nil
	for _, t := range tracker.Types {
		o = append(o, filterOption{Value: string(t), Label: t.Name()})
	}
	add("type", "Type", "type", strs(f.Types), o...)
	o = nil
	for _, s := range p.Options.Statuses {
		o = append(o, filterOption{Value: s.ID, Label: s.Name})
	}
	add("status", "Status", "status", f.Statuses, o...)
	o = nil
	for _, c := range tracker.Categories {
		o = append(o, filterOption{Value: string(c), Label: c.Name()})
	}
	add("category", "Category", "category", strs(f.Categories), o...)
	o = nil
	for i := len(tracker.Priorities) - 1; i >= 0; i-- {
		pr := tracker.Priorities[i]
		o = append(o, filterOption{Value: string(pr), Label: pr.Name()})
	}
	add("priority", "Priority", "priority", strs(f.Priorities), o...)
	people := func(extra ...filterOption) []filterOption {
		out := extra
		for _, id := range p.Options.People {
			out = append(out, filterOption{Value: id, Label: p.People[id].Name})
		}
		return out
	}
	add("assignee", "Assignee", "assignee", f.Assignees, people(filterOption{Value: tracker.Me, Label: "Me"}, filterOption{Value: tracker.Unassigned, Label: "Unassigned"})...)
	add("reporter", "Reporter", "reporter", f.Reporters, people(filterOption{Value: tracker.Me, Label: "Me"})...)
	o = nil
	for _, l := range p.Options.Labels {
		o = append(o, filterOption{Value: l, Label: l})
	}
	add("label", "Label", "label", f.Labels, o...)
	o = nil
	for _, c := range p.Options.Components {
		o = append(o, filterOption{Value: c, Label: c})
	}
	add("component", "Component", "component", f.Components, o...)
	o = []filterOption{{Value: tracker.NoSprint, Label: "No sprint"}}
	for _, s := range p.Options.Sprints {
		label := s.ProjectKey + " · " + s.Name
		if s.State == tracker.SprintActive {
			label += " (active)"
		}
		o = append(o, filterOption{Value: s.ID, Label: label})
	}
	add("sprint", "Sprint", "sprint", f.Sprints, o...)
	return g
}

func strs[T ~string](in []T) []string {
	out := make([]string, len(in))
	for i, x := range in {
		out[i] = string(x)
	}
	return out
}

func rangeCount(a, b string) int {
	n := 0
	if a != "" {
		n++
	}
	if b != "" {
		n++
	}
	return n
}

func countText(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// savedHref is the list a saved filter shows.
func savedHref(f tracker.SavedFilter) string {
	q := tracker.Query{Filter: f.Filter, Sort: f.Sort}
	return listHref(q.Values())
}

// The list's Datastar expressions. The page's script reads the form, never
// a person's value inside an expression (NFR-13).

// listQS is the form as a query string, empty values left out.
const listQS = `((f) => { const p = new URLSearchParams(); for (const [k, v] of new FormData(f)) if (v !== '') p.append(k, v); return p.toString() })(document.getElementById('` + ListFormID + `'))`

// listSend shows the form's list: it puts the query in the address (a new
// history entry when push holds, else in place of the current one) and
// posts it, which moves the tab's stream to the list's view (FR-55).
func listSend(push string) string {
	return `(((qs) => { const url = '/issues' + (qs ? '?' + qs : ''); if (url !== location.pathname + location.search) history[(` + push + `) ? 'pushState' : 'replaceState'](null, '', url); $_list.query = qs })(` + listQS + `), ` +
		`@post('/cmd/list/view', {payload: {tab: $_tab, view: $_views, query: $_list.query}}))`
}

// listCounts writes how many values each filter has on its button.
const listCounts = `document.getElementById('` + ListFormID + `').querySelectorAll('[data-count]').forEach(c => { const n = [...document.querySelectorAll('[data-in="' + c.dataset.count + '"]')].filter(i => i.type === 'checkbox' ? i.checked : i.value).length; c.textContent = n || ''; c.hidden = !n })`

var (
	listSignals = `{_list: {query: ''}, _save: {op: '', name: '', msg: '', done: ''}}`
	// listInput: a changed filter shows its first page (FR-51.3); typing
	// replaces the history entry, other changes add one.
	listInput  = `(evt.target.name !== 'page' && (el.elements.page.value = ''), ` + listCounts + `, ` + listSend(`evt.target.type !== 'search'`) + `)`
	listSubmit = listSend(`true`)
	// listPop restores the form from the address on back and forward.
	listPop = `location.pathname === '/issues' && (((f, p) => { for (const i of f.elements) { if (!i.name) continue; if (i.type === 'checkbox') i.checked = p.getAll(i.name).includes(i.value); else i.value = p.get(i.name) ?? '' } })(el, new URLSearchParams(location.search)), ` + listCounts + `, ` + listSend(`false`) + `)`
	// listClear empties the filters, the sort kept.
	listClear = `(((f) => { for (const i of f.elements) { if (i.type === 'checkbox') i.checked = false; else if (i.name && i.name !== 'sort' && i.name !== 'dir') i.value = '' } })(document.getElementById('` + ListFormID + `')), ` + listCounts + `, ` + listSend(`true`) + `)`
	// listClick, on the list's container: a sort header cycles ascending,
	// descending and the default, created newest first (FR-53.1); a page
	// link shows that page; a click on a row opens its issue.
	listClick = `((t, f) => {
	const s = t.closest('[data-sort]'), a = t.closest('.tk-pages a[href]'), row = t.closest('tr[data-href]')
	if (s) {
		const col = s.dataset.sort, cs = f.elements.sort.value, ec = cs || 'created', ed = cs ? (f.elements.dir.value || 'asc') : 'desc'
		let ns = col, nd = 'asc'
		if (ec === col && ed === 'asc') nd = 'desc'
		else if (ec === col && cs) ns = nd = ''
		if (ns === 'created' && nd === 'desc') ns = nd = ''
		f.elements.sort.value = ns; f.elements.dir.value = nd; f.elements.page.value = ''
		return true
	}
	if (a) {
		evt.preventDefault()
		f.elements.page.value = new URL(a.href).searchParams.get('page') || ''
		document.getElementById('` + IssueListID + `').scrollIntoView({block: 'nearest'})
		return true
	}
	if (row && !t.closest('a, button, input, label')) row.querySelector('a[href]')?.click()
	return false
})(evt.target, document.getElementById('` + ListFormID + `')) && ` + listSend(`true`)
	listFailed = `evt.detail.el === el && ['error', 'retries-failed'].includes(evt.detail.type) && ($_problem = 'The list did not update. Try again in a moment.')`
	// listInit posts a list whose view the stream lacks (ListPage.Unwatched).
	listInit = listSend(`false`)
	// syncExpanded keeps a filter button's aria-expanded with its popover.
	listToggle = `document.querySelectorAll('[popovertarget="' + el.id + '"]').forEach(b => b.setAttribute('aria-expanded', evt.newState === 'open'))`
)

// Saving the list's filter (FR-56.1).
var (
	saveFilter        = `$_save.name.trim() && ` + Act("_save.op", "/cmd/filters/save", "name: $_save.name, query: "+listQS)
	saveFilterSettled = Settle("_save.op", "o.ok ? ($_save.msg = '', $_save.name = '', $_save.done = 'Saved. It is in Filters now.', document.getElementById('save-filter').close()) : ($_save.msg = o.msg)")
	saveFilterFailed  = Failed("_save.op")
)
