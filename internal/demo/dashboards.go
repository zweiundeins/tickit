package demo

import (
	"time"

	"github.com/zweiundeins/tickit/internal/dashboards"
	"github.com/zweiundeins/tickit/internal/tracker"
)

// dashboardSteps are the seed's dashboards (FR-60): one per project, by
// its lead, with the seed's last change, so that a visitor sees one at once: the
// project's issues by status, created against resolved, a saved filter
// of its open bugs, the visitor's own issues and the activity.
func (r *Run) dashboardSteps(at time.Time) []Step {
	var out []Step
	for i, p := range r.Content.Projects {
		lead, project := p.Lead, r.ProjectID(p.Key)
		filter := stableID(kindFilter, i, "filter "+p.Key)
		dash := stableID(kindDashboard, i, "dashboard "+p.Key)
		out = append(out,
			Step{At: at, Phase: PhaseChange, Name: "filter " + p.Key, Cmd: tracker.SaveFilter{By: r.By(lead, at), ID: filter, Name: p.Key + " open bugs",
				Filter: tracker.Filter{Projects: []string{p.Key}, Types: []tracker.IssueType{tracker.Bug}, Categories: []tracker.Category{tracker.ToDo, tracker.InProgress}},
				Sort:   tracker.Sort{By: tracker.SortPriority, Desc: true}}},
			Step{At: at, Phase: PhaseChange, Name: "dashboard " + p.Key, Cmd: dashboards.Create{By: r.By(lead, at), ID: dash, Name: p.Name + " overview"}},
		)
		for n, g := range []dashboards.AddGadget{
			{Kind: dashboards.Status, Project: project},
			{Kind: dashboards.Created, Project: project},
			{Kind: dashboards.Filter, SavedFilter: filter},
			{Kind: dashboards.Assigned},
			{Kind: dashboards.Activity},
		} {
			g.By, g.Dashboard, g.ID = r.By(lead, at), dash, stableID(kindGadget, i*10+n, "gadget "+p.Key+" "+string(g.Kind))
			out = append(out, Step{At: at, Phase: PhaseChange, Name: "gadget " + p.Key, Cmd: g})
		}
	}
	return out
}
