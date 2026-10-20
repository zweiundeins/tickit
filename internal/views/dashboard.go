package views

import (
	"context"
	"io"
	"net/url"
	"time"

	"github.com/a-h/templ"

	"github.com/zweiundeins/tickit/internal/dashboards"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/ui"
)

// Dashboards (FR-60): /dashboards lists every one (the shared view
// dashboards:), /dashboards/<id> shows one as a grid of gadgets (the
// shared view dashboard:<id>, keyed by the scopes the viewer reads) with
// the viewer's own "Assigned to me" gadgets filled by the view
// dashme:<id> of each person. The owner's controls are in the document
// (the add form, rename, delete) or shown by the document's style (each
// gadget's move and remove), so the shared views hold nothing about who
// looks.

// DashboardList is the list of dashboards.
type DashboardList struct {
	Dashboards []dashboards.Dashboard
	People     map[string]people.Person
	Now        time.Time
}

// DashboardsPage is /dashboards.
type DashboardsPage struct {
	List DashboardList
	Me   string
}

// DashboardListID is the id of the list's region.
const DashboardListID = "dashboard-list"

// DashboardPage is /dashboards/<id>.
type DashboardPage struct {
	Grid DashboardGrid
	// Mine: the viewer may change it (its owner or a site admin): the page
	// has the add form, rename and delete.
	Mine     bool
	Me       string
	Projects []tracker.Project     // for the add form
	Filters  []tracker.SavedFilter // for the add form
}

// DashboardGrid is the shared region of a dashboard: its head and its
// gadgets.
type DashboardGrid struct {
	Dashboard dashboards.Dashboard
	Owner     people.Person
	Gadgets   []Gadget
	Now       time.Time
}

// DashboardGridID is the id of a dashboard's shared region.
const DashboardGridID = "dashboard-grid"

// Gadget is one gadget as the grid renders it.
type Gadget struct {
	dashboards.Gadget
	Title string
	// Href opens what the gadget shows in full: the issue list, a board.
	Href string
	// Missing says why a gadget shows nothing: its saved filter or its
	// project is gone or not readable.
	Missing string
	// Filter: a page of the saved filter's issues, how many match.
	Issues tracker.Results
	// Status: the counts of the project's statuses.
	Counts []tracker.StatusCount
	// Created: created and resolved per day.
	Days []tracker.DayCount
	// Activity: the feed.
	Feed ActivityFeed
	// Note is a line under the gadget's title.
	Note string
	// ProjectKey is the key of a Status or Created gadget's project.
	ProjectKey string
}

// GadgetID is a gadget's element id; AssignedID the slot its viewer's
// view fills for "Assigned to me".
func GadgetID(id string) string   { return "g-" + id }
func AssignedID(id string) string { return "gme-" + id }

// GadgetShown is how many issues a gadget lists.
const GadgetShown = 8

// DashboardDays is how many days "Created and resolved" shows.
const DashboardDays = 14

// DashboardHref is a dashboard's address.
func DashboardHref(id string) string { return "/dashboards/" + url.PathEscape(id) }

// Assigned is the viewer's "Assigned to me" gadgets of a dashboard.
type Assigned struct {
	Gadgets []string // the ids of the dashboard's Assigned gadgets
	Issues  tracker.Results
	Now     time.Time
}

func countBars(g Gadget) []ui.CountBar {
	out := make([]ui.CountBar, 0, len(g.Counts))
	for _, c := range g.Counts {
		out = append(out, ui.CountBar{Label: c.Name, Category: Category(c.Category), Count: c.Count,
			Href: "/issues?" + url.Values{"project": {g.ProjectKey}, "status": {c.Status}}.Encode()})
	}
	return out
}

func dayPairs(days []tracker.DayCount) []ui.DayPair {
	out := make([]ui.DayPair, len(days))
	for i, d := range days {
		out[i] = ui.DayPair{Day: d.Day, Created: d.Created, Resolved: d.Resolved}
	}
	return out
}

// dashStyle shows a gadget's owner controls to the dashboard's owner and
// to a site admin; the grid is the same for every viewer.
func dashStyle(me string, admin bool) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		sel := ""
		switch {
		case admin:
			sel = `.tk-dash .tk-gadget__own`
		case people.ValidID(me):
			sel = `.tk-dash [data-owner="` + me + `"] .tk-gadget__own`
		default:
			return nil
		}
		_, err := io.WriteString(w, `<style>`+sel+`{display:inline-flex}</style>`)
		return err
	})
}

func gadgetKinds() []ui.Option {
	out := []ui.Option{{Value: "", Label: "Pick a gadget"}}
	for _, k := range dashboards.Kinds {
		out = append(out, ui.Option{Value: string(k), Label: k.Name()})
	}
	return out
}

func projectOptions(list []tracker.Project) []ui.Option {
	out := []ui.Option{{Value: "", Label: "Pick a project"}}
	for _, p := range list {
		out = append(out, ui.Option{Value: p.ID, Label: p.Key + " " + p.Name})
	}
	return out
}

func filterOptions(list []tracker.SavedFilter) []ui.Option {
	out := []ui.Option{{Value: "", Label: "Pick a saved filter"}}
	for _, f := range list {
		out = append(out, ui.Option{Value: f.ID, Label: f.Name})
	}
	return out
}

func gadgetCount(n int) string { return counted(n, "gadget", "gadgets") }

// dashFaces are the owners the list shows.
func dashFaces(l DashboardList) []ui.Person {
	var out []ui.Person
	for _, d := range l.Dashboards {
		out = append(out, UIPerson(l.People[d.Owner]))
	}
	return out
}

// The expressions of the dashboards' pages: each control names what it
// does in data-act and the gadget in data-g; the page's one handler posts
// it (NFR-13: no person's value in an expression).
var (
	dashSignals = `{_dash: {op: '', msg: '', name: '', kind: '', project: '', filter: '', g: ''}}`
	dashCreate  = `$_dash.name.trim() && ` + `$_dash.op === '' && ($_problem = '', $_dash.op = ` + newOp + `, @post('/cmd/dashboards/create', {payload: {tab: $_tab, op: $_dash.op, name: $_dash.name}}))`
	dashRename  = `$_dash.name.trim() && $_dash.op === '' && ($_problem = '', $_dash.op = ` + newOp + `, @post('/cmd/dashboards/rename', {payload: {tab: $_tab, op: $_dash.op, dashboard: el.dataset.d, name: $_dash.name}}))`
	dashDelete  = `$_dash.op === '' && ($_problem = '', $_dash.op = ` + newOp + `, @post('/cmd/dashboards/delete', {payload: {tab: $_tab, op: $_dash.op, dashboard: el.dataset.d}}))`
	dashAdd     = `$_dash.kind && $_dash.op === '' && ($_problem = '', $_dash.op = ` + newOp + `, @post('/cmd/dashboards/gadget', {payload: {tab: $_tab, op: $_dash.op, dashboard: el.dataset.d, kind: $_dash.kind, project: $_dash.project, filter: $_dash.filter}}))`
	// dashClick runs a gadget's move or remove, on the grid's container.
	dashClick = `((b) => b && $_dash.op === '' && ($_problem = '', $_dash.g = b.dataset.g, $_dash.op = ` + newOp + `,
	b.dataset.act === 'gremove' ? @post('/cmd/dashboards/gadget/remove', {payload: {tab: $_tab, op: $_dash.op, dashboard: el.dataset.d, gadget: $_dash.g}})
	: @post('/cmd/dashboards/gadget/move', {payload: {tab: $_tab, op: $_dash.op, dashboard: el.dataset.d, gadget: $_dash.g, earlier: b.dataset.act === 'gearlier'}})))(evt.target.closest('button[data-act]'))`
	dashSettled  = Settle("_dash.op", "$_dash.msg = o.ok ? '' : o.msg, o.ok && ($_dash.kind = '', document.querySelectorAll('#dash-rename[open], #dash-delete[open]').forEach((d) => d.close()))")
	dashFailed   = `evt.detail.type && ['error', 'retries-failed'].includes(evt.detail.type) && evt.detail.argsRaw.status !== '403' && ($_dash.op = '', $_problem = ` + problem + `)`
	dashBusy     = `$_dash.op !== '' ? 'true' : false`
	dashPending  = `$_dash.op !== ''`
	needsProject = `$_dash.kind === 'status' || $_dash.kind === 'created'`
	needsFilter  = `$_dash.kind === 'filter'`
)
