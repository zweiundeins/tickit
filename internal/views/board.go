package views

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/ui"
)

// The board (FR-40 to FR-42, MP-04): the page, the shared lanes of a
// project and quick filter (the view "board:<KEY>.<filter>", key
// board:<projectID>:<filter hash>), the board's head with the people on
// it and the active sprint ("boardhead:<KEY>", with presence), and the
// expressions of the move protocol (board_expr.go).

// BoardLanesID is the id of the lanes' region, whatever the filter: a new
// filter's first frame morphs the same element.
const BoardLanesID = "board-lanes"

// CardMenuID is the id of the board's one card menu.
const CardMenuID = "card-menu"

// QuickFilter keeps the parts of a filter the board's quick filters set
// (FR-42): assignees (with tracker.Me), types, labels and the text.
func QuickFilter(f tracker.Filter) tracker.Filter {
	return tracker.Filter{Assignees: f.Assignees, Types: f.Types, Labels: f.Labels, Text: f.Text}.Normalize("")
}

// QuickFilterOf reads a board's quick filter from its page's query:
// assignee (an id, me, unassigned), type, label and q, as the issue list
// writes them (FR-55).
func QuickFilterOf(v url.Values) tracker.Filter { return QuickFilter(tracker.ParseFilter(v)) }

// QuickQuery is the board page's query for a quick filter.
func QuickQuery(f tracker.Filter) string { return QuickFilter(f).Values().Encode() }

// maxBoardText is how much of a text filter the view's name carries: the
// stream's view names have 200 characters at most.
const maxBoardText = 60

var b64 = base64.RawURLEncoding

// BoardArg names the board view of a project's key and a quick filter:
// "WEB", then ".a" and an assignee, ".t" and a type, ".l" and a label in
// base64url, ".q" and the text in base64url, each sorted. Person ids,
// types and base64url hold only the characters a view's name may (the
// stream's argPattern); the page builds the same name (boardArg).
func BoardArg(key string, f tracker.Filter) string {
	f = QuickFilter(f)
	var b strings.Builder
	b.WriteString(key)
	for _, a := range f.Assignees {
		b.WriteString(".a" + a)
	}
	for _, t := range f.Types {
		b.WriteString(".t" + string(t))
	}
	for _, l := range f.Labels {
		b.WriteString(".l" + b64.EncodeToString([]byte(l)))
	}
	if f.Text != "" {
		q := []rune(f.Text)
		if len(q) > maxBoardText {
			q = q[:maxBoardText]
		}
		b.WriteString(".q" + b64.EncodeToString([]byte(string(q))))
	}
	return b.String()
}

var errBoardArg = errors.New("views: a malformed board view")

// ParseBoardArg reads BoardArg's name back: the key, and the quick filter
// with Me kept.
func ParseBoardArg(arg string) (string, tracker.Filter, error) {
	parts := strings.Split(arg, ".")
	key, ok := tracker.NormalizeKey(parts[0])
	if !ok || key != parts[0] {
		return "", tracker.Filter{}, errBoardArg
	}
	var f tracker.Filter
	for _, p := range parts[1:] {
		if len(p) < 2 {
			return "", f, errBoardArg
		}
		v := p[1:]
		switch p[0] {
		case 'a':
			f.Assignees = append(f.Assignees, v)
		case 't':
			f.Types = append(f.Types, tracker.IssueType(v))
		case 'l', 'q':
			raw, err := b64.DecodeString(v)
			if err != nil {
				return "", f, errBoardArg
			}
			if p[0] == 'l' {
				f.Labels = append(f.Labels, string(raw))
			} else {
				f.Text = string(raw)
			}
		default:
			return "", f, errBoardArg
		}
	}
	return key, QuickFilter(f), nil
}

// BoardKey is the key of a project's board view for a quick filter
// normalized for its viewer (Me replaced): the tabs that see the same
// cards share it (MP-11.1).
func BoardKey(projectID string, f tracker.Filter) string {
	return "board:" + projectID + ":" + f.Key()
}

// BoardHeadKey is the key of a project's board head, whose presence row
// shows who is on the board whatever their filter.
func BoardHeadKey(projectID string) string { return "boardhead:" + projectID }

// Lanes is what the lanes' region shows: the board and the people on its
// cards. It holds nothing about its viewer.
type Lanes struct {
	Board  tracker.BoardView
	People map[string]people.Person
	Now    time.Time
	// Totals are the columns' counts without the quick filter, set when
	// one filters.
	Totals map[string]int
	// Rev is the frame's revision, a hash of its bytes: the page tells by
	// it whether a frame came since a move (web fills it in).
	Rev string
}

// BoardHead is the head above the board: the project's name, the active
// sprint with its days left and points (FR-46.2), and the faces of the
// assignees to filter by (FR-42).
type BoardHead struct {
	Project   tracker.Project
	Sprint    *tracker.Sprint
	Done      int // story points of the sprint's issues in a Done status
	Points    int // story points of the sprint's issues
	Issues    int // the sprint's issues
	Open      int // those not done
	Next      []tracker.Sprint
	Assignees []people.Person
	Now       time.Time
}

// BoardPage is the board's page.
type BoardPage struct {
	Project tracker.Project
	Filter  tracker.Filter // the quick filter, with Me kept
	Labels  []string       // the project's labels, for the label filter
	Lanes   Lanes
	Head    BoardHead
	// Admin shows the Settings tab.
	Admin bool
	// CanEdit shows the controls of a Member: "+" per column and the
	// menu's moves. The server refuses what a Viewer sends anyway.
	CanEdit bool
	// CreateStatus opens the create dialog in a status, from ?create=.
	CreateStatus string
	// Me and Moderates are the issue panel's (docs/architecture.md).
	Me        string
	Moderates bool
}

// HeadID is the id of a board head's region.
const HeadID = "board-head"

// BoardSections are a project's tabs (FR-01.2).
func ProjectTabs(key, current string, admin bool) []ui.NavItem {
	items := []ui.NavItem{
		{Label: "Board", Href: "/projects/" + key + "/board", Icon: "board", Current: current == "board"},
		{Label: "Backlog", Href: "/projects/" + key + "/backlog", Icon: "backlog", Current: current == "backlog"},
		{Label: "Issues", Href: "/issues?project=" + key, Icon: "list", Current: current == "issues"},
		{Label: "Activity", Href: "/projects/" + key + "/activity", Icon: "history", Current: current == "activity"},
	}
	if admin {
		items = append(items, ui.NavItem{Label: "Settings", Href: "/projects/" + key + "/settings", Icon: "settings", Current: current == "settings"})
	}
	for i := range items {
		if !ui.HasIcon(items[i].Icon) {
			items[i].Icon = ""
		}
	}
	return items
}

// ProjectSideNav is a project's sidebar.
func ProjectSideNav(p tracker.Project, current string, admin bool) ui.SideNavProps {
	return ui.SideNavProps{Title: p.Name, Key: p.Key, Href: "/projects/" + p.Key + "/board",
		Sections: []ui.NavSection{{Label: "Planning", Items: ProjectTabs(p.Key, current, admin)}}, Collapsible: true}
}

func category(c tracker.Category) ui.StatusCategory {
	switch c {
	case tracker.InProgress:
		return ui.InProgress
	case tracker.Done:
		return ui.Done
	}
	return ui.ToDo
}

// CardID is the id of a card's list item: the morph moves it by its id.
func CardID(key string) string { return "c-" + key }

// laneID is the id of a column.
func laneID(status string) string { return "lane-" + status }

// shortID is an issue's id in 22 characters (base64url of its 16
// bytes): the board's card carries it, and the page puts the issue
// panel's data-issue-panel on the card's link when it is clicked
// (boardClick), 33 bytes a card less (NFR-37.1).
func shortID(id string) string {
	raw, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	if err != nil {
		return ""
	}
	return b64.EncodeToString(raw)
}

// cardProps is a tracker card as the board draws it.
func cardProps(c tracker.Card, who map[string]people.Person, now time.Time, menu string) ui.CardProps {
	p := ui.CardProps{
		ID: CardID(c.Key), Key: c.Key, Summary: c.Summary, Href: "/" + c.Key, LinkAttrs: map[string]any{"data-i": shortID(c.ID)},
		Type: ui.IssueType(c.Type), Priority: ui.Priority(c.Priority), Labels: c.Labels, LabelHref: LabelHref, Now: now, MenuID: menu,
		SlotAttrs: map[string]any{"data-v": strconv.FormatInt(c.Version, 10)},
	}
	if c.Assignee != "" {
		u := UIPerson(who[c.Assignee])
		p.Assignee = &u
	}
	if c.Due != "" {
		if d, err := time.Parse(time.DateOnly, c.Due); err == nil {
			p.Due, p.Overdue = d, c.Overdue(now)
		}
	}
	if c.Epic != nil {
		p.Epic = c.Epic.Summary
	}
	if c.Parent != nil {
		p.Parent = c.Parent.Key
	}
	if c.Points != nil {
		p.Points = strconv.Itoa(*c.Points)
	}
	return p
}

// facesOf are the faces a board's frame refers to.
func facesOf(b tracker.BoardView, who map[string]people.Person) []ui.Person {
	var out []ui.Person
	seen := map[string]bool{}
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			if c.Assignee != "" && !seen[c.Assignee] {
				seen[c.Assignee] = true
				out = append(out, UIPerson(who[c.Assignee]))
			}
		}
	}
	return out
}

// laneRefs are the columns the card menu moves to.
func laneRefs(b tracker.BoardView) []ui.LaneRef {
	out := make([]ui.LaneRef, len(b.Columns))
	for i, c := range b.Columns {
		out[i] = ui.LaneRef{ID: c.ID, Name: c.Name, Cat: category(c.Category)}
	}
	return out
}

// DaysLeft is the days to a sprint's end, 0 on its last day, negative
// past it; ok is false without an end date.
func DaysLeft(s tracker.Sprint, now time.Time) (int, bool) {
	end, err := time.Parse(time.DateOnly, s.End)
	if err != nil {
		return 0, false
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return int(end.Sub(today).Hours() / 24), true
}

func daysLeft(s tracker.Sprint, now time.Time) string {
	n, ok := DaysLeft(s, now)
	switch {
	case !ok:
		return "No end date"
	case n < 0:
		return plural(-n, "day") + " over"
	case n == 0:
		return "Ends today"
	}
	return plural(n, "day") + " left"
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

// boardSignals are the board page's local signals: the move in flight
// (_board) and the quick filter as the controls hold it (_bf; lists as
// space-separated words, text as typed and as applied).
func boardSignals(p BoardPage) string {
	f := p.Filter
	mine := slices.Contains(f.Assignees, tracker.Me)
	var who []string
	for _, a := range f.Assignees {
		if a != tracker.Me {
			who = append(who, a)
		}
	}
	types := make([]string, len(f.Types))
	for i, t := range f.Types {
		types[i] = string(t)
	}
	return `{_board: {op: '', pcard: '', drag: '', say: '', note: '', ncard: '', problem: '', retry: '', move: {issue: '', status: '', before: '', after: '', seen: 0}}, _bf: {mine: ` +
		strconv.FormatBool(mine) + `, who: ` + jsString(strings.Join(who, " ")) + `, types: ` + jsString(strings.Join(types, " ")) +
		`, labels: ` + jsString(strings.Join(f.Labels, " ")) + `, text: ` + jsString(f.Text) + `, q: ` + jsString(f.Text) + `}}`
}

// filtered reports whether a quick filter hides anything.
func filtered(f tracker.Filter) bool { return !QuickFilter(f).IsZero() }

func points(n int) string { return plural(n, "point") }
