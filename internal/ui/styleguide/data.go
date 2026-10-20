// Package styleguide renders /design: every component of internal/ui in
// every state, with tickit's own content, and composed screens that
// feature code copies. It imports internal/ui only.
package styleguide

import (
	"strconv"
	"time"

	"github.com/a-h/templ"

	"github.com/mbolli/tickit/internal/ui"
)

// Props is what the page needs from the server.
type Props struct {
	Prefs ui.Prefs
	// Nonce is the request's CSP nonce, for the import map.
	Nonce string
	// The hashed URLs of the files the page loads.
	CSS, DesignCSS, Mark, Datastar string
	Fonts                          []string
	Scripts                        []string
	// ImportMap is the import map's JSON.
	ImportMap string
	Now       time.Time
	// Href is this page's URL with one display choice changed.
	Href func(key, value string) string
}

// Components are the Starbase components /design loads.
var Components = []string{"copy-button", "date-picker", "dropdown", "select", "tabs", "toast"}

var (
	sunny  = ui.Person{Seed: "g-sunny-otter", Name: "Sunny Otter"}
	heron  = ui.Person{Seed: "g-brisk-heron", Name: "Brisk Heron"}
	lynx   = ui.Person{Seed: "g-calm-lynx", Name: "Calm Lynx"}
	mbolli = ui.Person{Seed: "a-michael-bolli", Name: "Michael Bolli"}
	badger = ui.Person{Seed: "g-keen-badger", Name: "Keen Badger"}
	finch  = ui.Person{Seed: "g-merry-finch", Name: "Merry Finch", Away: true}
	marten = ui.Person{Seed: "g-lucid-marten", Name: "Lucid Marten"}
	ibex   = ui.Person{Seed: "g-hardy-ibex", Name: "Hardy Ibex"}
	wren   = ui.Person{Seed: "g-plucky-wren", Name: "Plucky Wren"}
	vole   = ui.Person{Seed: "g-quick-vole", Name: "Quick Vole"}
	ada    = ui.Person{Seed: "s-ada", Name: "Ada Quartz", Deactivated: true}
)

// me is the viewer on every mock screen.
var me = mbolli

type issue struct {
	Key, Summary string
	Type         ui.IssueType
	Prio         ui.Priority
	Status       string
	Cat          ui.StatusCategory
	Who          *ui.Person
	Labels       []string
	Epic, Points string
	DueIn        int // days from now; 0 for none
	Overdue      bool
	Updated      time.Duration
}

func p(x ui.Person) *ui.Person { return &x }

var issues = []issue{
	{"WEB-12", "Card drag drops the card in the wrong lane on Safari", ui.Bug, ui.High, "In Review", ui.InProgress, p(sunny), []string{"board", "safari"}, "Board v1", "3", 2, false, 4 * time.Minute},
	{"WEB-14", "Keyboard moves for board cards: Move to, Move up, Move down", ui.Story, ui.Medium, "In Progress", ui.InProgress, p(mbolli), []string{"a11y", "board"}, "Board v1", "5", 6, false, 38 * time.Minute},
	{"WEB-7", "Rate limit the search endpoint per client address", ui.Task, ui.Critical, "To Do", ui.ToDo, p(heron), []string{"security"}, "", "2", -3, true, 3 * time.Hour},
	{"WEB-21", "Dark theme: lozenge contrast on hovered rows", ui.Bug, ui.Low, "Done", ui.Done, p(lynx), []string{"design"}, "Design system", "1", 0, false, 26 * time.Hour},
	{"WEB-3", "Board v1", ui.Epic, ui.High, "In Progress", ui.InProgress, p(mbolli), nil, "", "", 14, false, 2 * 24 * time.Hour},
	{"WEB-15", "Write the drop marker's anchor positioning fallback", ui.Subtask, ui.Medium, "To Do", ui.ToDo, nil, nil, "Board v1", "", 0, false, 5 * time.Hour},
	{"WEB-18", "Presence row shows a person twice after a reconnect", ui.Bug, ui.High, "To Do", ui.ToDo, p(badger), []string{"presence"}, "Multiplayer", "2", 1, false, 52 * time.Minute},
	{"WEB-9", "Saved filters keep their sort order", ui.Story, ui.Low, "Done", ui.Done, p(marten), []string{"filters"}, "", "3", 0, false, 4 * 24 * time.Hour},
	{"WEB-22", "Nightly reset notice counts down in the header", ui.Task, ui.Medium, "In Review", ui.InProgress, p(wren), []string{"demo"}, "Multiplayer", "1", 3, false, 12 * time.Minute},
	{"WEB-24", "Umlauts in lane names break the WIP limit badge: Übersicht für Größen", ui.Bug, ui.Medium, "In Progress", ui.InProgress, p(ibex), []string{"i18n"}, "Design system", "2", 0, false, 2 * time.Hour},
}

func (s issue) card(now time.Time, state ui.CardState, note string) ui.CardProps {
	c := ui.CardProps{
		Key: s.Key, Summary: s.Summary, Href: "/" + s.Key, Type: s.Type, Priority: s.Prio,
		Assignee: s.Who, Labels: s.Labels, Epic: s.Epic, Points: s.Points, Now: now,
		State: state, Note: note, MenuID: "sg-card-menu", Overdue: s.Overdue,
	}
	if s.DueIn != 0 {
		c.Due = now.AddDate(0, 0, s.DueIn)
	}
	if s.Type == ui.Subtask {
		c.Parent = "WEB-14"
	}
	return c
}

func (s issue) chip() ui.IssueChipProps {
	return ui.IssueChipProps{Key: s.Key, Summary: s.Summary, Type: s.Type, Status: s.Status, Category: s.Cat, Href: "/" + s.Key}
}

func find(key string) issue {
	for _, s := range issues {
		if s.Key == key {
			return s
		}
	}
	return issues[0]
}

var lanes = []ui.LaneRef{
	{ID: "st-todo", Name: "To Do", Cat: ui.ToDo},
	{ID: "st-progress", Name: "In Progress", Cat: ui.InProgress},
	{ID: "st-review", Name: "In Review", Cat: ui.InProgress},
	{ID: "st-done", Name: "Done", Cat: ui.Done},
}

func projectNav(current string) ui.SideNavProps {
	item := func(label, icon string, count *int) ui.NavItem {
		return ui.NavItem{Label: label, Href: "#" + current, Icon: icon, Current: label == current, Count: count}
	}
	return ui.SideNavProps{
		Title: "Website", Key: "WEB", Href: "#screens",
		Sections: []ui.NavSection{
			{Label: "Planning", Items: []ui.NavItem{item("Board", "board", nil), item("Backlog", "backlog", ui.Count(23)), item("Issues", "list", ui.Count(312)), item("Reports", "chart", nil)}},
			{Label: "Project", Items: []ui.NavItem{item("Pages", "page", nil), item("Components", "component", nil), item("Settings", "settings", nil)}},
		},
		Collapsible: true,
	}
}

func presence(editing bool) []ui.PresenceEntry {
	return []ui.PresenceEntry{
		{Person: sunny, Tabs: 2, Editing: editing},
		{Person: mbolli, You: true},
		{Person: heron},
		{Person: finch},
		{Person: lynx, Tabs: 3},
		{Person: badger},
		{Person: marten},
		{Person: ibex},
		{Person: wren},
		{Person: vole},
		{Person: ada},
	}
}

func ago(now time.Time, d time.Duration) time.Time { return now.Add(-d) }

func itoa(n int) string { return strconv.Itoa(n) }

func attrs(kv ...string) templ.Attributes {
	a := templ.Attributes{}
	for i := 0; i+1 < len(kv); i += 2 {
		a[kv[i]] = kv[i+1]
	}
	return a
}

// zurich is the zone the mock times show in their titles.
var zurich = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Zurich"); err == nil {
		return l
	}
	return time.UTC
}()
