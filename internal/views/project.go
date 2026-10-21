package views

import (
	"time"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/ui"
)

// Projects (FR-01 to FR-05): the list of the projects a person can read
// ("projects:", key projects:<hash of the readable ids>), the dialog that
// creates one, and a project's settings ("settings:<KEY>", key
// settings:<projectID>, for its Admins).

// ProjectsListID is the id of the projects list's region.
const ProjectsListID = "projects-list"

// ProjectList is what the list of projects shows.
type ProjectList struct {
	Projects []tracker.Project
	People   map[string]people.Person // the leads
}

// ProjectsPage is the page of projects.
type ProjectsPage struct {
	List      ProjectList
	CanCreate bool
}

// Settings is a project's settings page (FR-03, FR-04, FR-05, FR-24,
// FR-36, FR-37, FR-43).
type Settings struct {
	Project    tracker.Project
	Live       SettingsLive
	Workflow   WorkflowLive
	Leads      []people.Person // whom the lead's picker offers: who may be assigned, and the lead
	Candidates []people.Person // whom the members' picker offers
}

// SettingsLive is the settings' shared region: the members with their
// roles and the components.
type SettingsLive struct {
	Project    tracker.Project
	Members    []tracker.Member
	People     map[string]people.Person
	Components []tracker.Component
}

// SettingsID is the id of the settings' region.
const SettingsID = "project-settings"

// ProjectPage is a project's Activity tab: its feed (FR-69.1).
type ProjectPage struct {
	Project tracker.Project
	Admin   bool
	Feed    ActivityFeed
}

var roles = []access.Role{access.Viewer, access.Member, access.Admin}

func roleName(r access.Role) string {
	switch r {
	case access.Viewer:
		return "Viewer"
	case access.Member:
		return "Member"
	case access.Admin:
		return "Admin"
	}
	return string(r)
}

func roleOptions(current access.Role) []ui.Option {
	out := make([]ui.Option, len(roles))
	for i, r := range roles {
		out[i] = ui.Option{Value: string(r), Label: roleName(r), Selected: r == current}
	}
	return out
}

// pickPeople are people as a picker's options.
func pickPeople(list []people.Person) []ui.Option {
	out := make([]ui.Option, len(list))
	for i, p := range list {
		out[i] = ui.Option{Value: p.ID, Label: p.Name}
	}
	return out
}

func since(t time.Time) string { return t.Format("2 Jan 2006") }

// projectsSignals are the create dialog's: the fields, each one's
// refusal, and the op in flight.
const projectsSignals = `{_np: {op: '', name: '', key: '', description: '', err: {name: '', key: '', description: '', any: ''}}}`
