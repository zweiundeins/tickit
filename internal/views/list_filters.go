package views

import (
	"strings"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
)

// SavedFilters is the Filters page's list (FR-56): every saved filter,
// the person's starred ones first. Its view is the person's own.
type SavedFilters struct {
	Filters []tracker.SavedFilter
	People  map[string]people.Person
	Me      people.Person
}

// mayChange reports whether the person may rename and delete f: its
// owner, or a site admin.
func (d SavedFilters) mayChange(f tracker.SavedFilter) bool {
	return f.Owner == d.Me.ID || d.Me.Role == people.Admin
}

// describe says what a saved filter selects, one part per filter.
func (d SavedFilters) describe(f tracker.SavedFilter) string {
	var parts []string
	add := func(label string, vals []string) {
		if len(vals) > 0 {
			parts = append(parts, label+": "+strings.Join(vals, ", "))
		}
	}
	who := func(ids []string) []string {
		out := make([]string, len(ids))
		for i, id := range ids {
			switch id {
			case tracker.Me:
				out[i] = "me"
			case tracker.Unassigned:
				out[i] = "unassigned"
			default:
				out[i] = d.People[id].Name
			}
		}
		return out
	}
	names := func(in []string, name func(string) string) []string {
		out := make([]string, len(in))
		for i, x := range in {
			out[i] = name(x)
		}
		return out
	}
	x := f.Filter
	if x.Text != "" {
		parts = append(parts, "Text: “"+x.Text+"”")
	}
	add("Project", x.Projects)
	add("Type", names(strs(x.Types), func(s string) string { return tracker.IssueType(s).Name() }))
	add("Status", x.Statuses)
	add("Category", names(strs(x.Categories), func(s string) string { return tracker.Category(s).Name() }))
	add("Priority", names(strs(x.Priorities), func(s string) string { return tracker.Priority(s).Name() }))
	add("Assignee", who(x.Assignees))
	add("Reporter", who(x.Reporters))
	add("Label", x.Labels)
	add("Component", x.Components)
	if len(x.Sprints) > 0 {
		parts = append(parts, "Sprint: "+itoaCount(len(x.Sprints)))
	}
	if x.DueFrom != "" || x.DueTo != "" {
		parts = append(parts, "Due: "+x.DueFrom+" to "+x.DueTo)
	}
	if x.CreatedFrom != "" || x.CreatedTo != "" {
		parts = append(parts, "Created: "+x.CreatedFrom+" to "+x.CreatedTo)
	}
	if len(parts) == 0 {
		return "Every issue"
	}
	return strings.Join(parts, " · ")
}

func itoaCount(n int) string {
	if n == 1 {
		return "1 sprint"
	}
	return countText(n) + " sprints"
}

func starLabel(f tracker.SavedFilter) string {
	if f.Starred {
		return "Unstar " + f.Name
	}
	return "Star " + f.Name
}

func pressed(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// The Filters page's actions: the buttons carry the filter's id and name
// (data-id, data-name), which the script reads from the element.
var (
	filtersSignals = `{_filters: {op: '', id: '', name: '', star: false, msg: ''}}`
	filtersClick   = `((b) => { if (!b) return false; $_filters.id = b.dataset.id; $_filters.name = b.dataset.name || ''; $_filters.msg = ''; $_filters.star = b.getAttribute('aria-pressed') !== 'true'; if (b.dataset.act !== 'star') { document.getElementById(b.dataset.act + '-filter').showModal(); return false } return true })(evt.target.closest('button[data-act]')) && ` +
		Act("_filters.op", "/cmd/filters/star", "filter: $_filters.id, star: $_filters.star")
	renameFilter   = `$_filters.name.trim() && ` + Act("_filters.op", "/cmd/filters/rename", "filter: $_filters.id, name: $_filters.name")
	deleteFilter   = Act("_filters.op", "/cmd/filters/delete", "filter: $_filters.id")
	filtersSettled = Settle("_filters.op", "o.ok ? document.querySelectorAll('#rename-filter[open], #delete-filter[open]').forEach(d => d.close()) : ($_filters.msg = o.msg)")
	filtersFailed  = Failed("_filters.op")
	filtersBusy    = Busy("_filters.op")
	// starBusy marks the star that was pressed pending (MP-09.1).
	starBusy        = `$_filters.op !== '' && $_filters.id === el.dataset.id ? 'true' : false`
	filtersReadonly = Pending("_filters.op")
)
