package views

import (
	"context"
	"github.com/mbolli/tickit/internal/wiki"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tracker"
	"github.com/mbolli/tickit/internal/ui"
)

// Issue is everything the issue's live view renders (issue:<id>), read in
// one transaction: the same bytes go to everyone who has it open, so it
// holds nothing about its viewer. Names and faces come from People, never
// ids (FR-13.1).
type Issue struct {
	tracker.Detail
	People     map[string]people.Person
	Workflow   tracker.Workflow
	Statuses   []tracker.Column
	Sprints    []tracker.Sprint
	Components []tracker.Component // the project's
	Assignable []people.Person
	Labels     []string           // the project's, for the autocomplete
	Epics      []tracker.IssueRef // the epic picker's
	Others     []tracker.Issue    // issues of the project, for linking by key or summary
	Comments   []Comment
	DescHTML   string // the description rendered (doc.RenderHTML)
	DescMarkup string // the description as markup, for editing it
	// Pages are the pages whose head document shows the issue as a chip
	// (FR-101), live: the wiki touches the issue's topic.
	Pages    []wiki.PageRef
	History  []tracker.Entry
	Activity []tracker.ActivityItem
	Now      time.Time
}

// Comment is a comment with its body rendered and as markup.
type Comment struct {
	tracker.Comment
	HTML, Markup string
}

// IssueAsOf is an issue as it stood at a version (FR-70): the view
// issue:<id>:at:<v>, which every tab at that version shares.
type IssueAsOf struct {
	ID, Key  string
	State    tracker.IssueState
	Version  int64
	Latest   int64
	Workflow tracker.Workflow
	Sprints  []tracker.Sprint
	People   map[string]people.Person
	Keys     map[string]string // issue ids to keys, for the parent and links
	DescHTML string
	Changes  []tracker.Entry // what changed at that version
	Now      time.Time
}

// Typing is who types where on an issue (MP-07): the view issue-typing:<id>.
type Typing struct {
	Typists []tracker.Typist
	People  map[string]people.Person
}

// Zone is the time zone of absolute times (NFR-51): the demo's, Zurich.
var Zone = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Zurich"); err == nil {
		return l
	}
	return time.UTC
}()

func (m Issue) person(id string) people.Person { return lookup(m.People, id) }

func lookup(found map[string]people.Person, id string) people.Person {
	if p, ok := found[id]; ok {
		return p
	}
	return people.Person{ID: id, Kind: people.Deleted, Name: people.DeletedName}
}

func uiType(t tracker.IssueType) ui.IssueType { return ui.IssueType(t) }
func uiPrio(p tracker.Priority) ui.Priority   { return ui.Priority(p) }
func uiCat(c tracker.Category) ui.StatusCategory {
	switch c {
	case tracker.InProgress:
		return ui.InProgress
	case tracker.Done:
		return ui.Done
	}
	return ui.ToDo
}

func issueHref(key string) string { return "/" + key }

func chip(r tracker.IssueRef) ui.IssueChipProps {
	p := ui.IssueChipProps{Key: r.Key, Summary: r.Summary, Type: uiType(r.Type), Status: r.StatusName, Category: uiCat(r.Category), Href: issueHref(r.Key)}
	if r.Deleted {
		p.State = ui.ChipDeleted
	}
	return p
}

// at is t in the zone absolute times show.
func at(t time.Time) time.Time { return t.In(Zone) }

func itoa(n int) string  { return strconv.Itoa(n) }
func i64(n int64) string { return strconv.FormatInt(n, 10) }
func intp(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}
func minutes(p *int) string {
	if p == nil {
		return ""
	}
	return tracker.Minutes(*p)
}

// dueTime is a due date as a time, zero for none.
func dueTime(d string) time.Time {
	t, err := time.ParseInLocation(time.DateOnly, d, Zone)
	if err != nil {
		return time.Time{}
	}
	return t
}

// transitions are the statuses the workflow lets the issue go to, in
// workflow order, its own first (FR-16.1).
func (m Issue) transitions() []tracker.Column {
	var out []tracker.Column
	for _, c := range m.Statuses {
		if c.ID == m.Status || m.Workflow.Allows(m.Status, c.ID) {
			out = append(out, c)
		}
	}
	return out
}

func (m Issue) sprintName(id string) string {
	for _, s := range m.Sprints {
		if s.ID == id {
			return s.Name
		}
	}
	return ""
}

// openSprints are the sprints an issue may go to: not closed.
func (m Issue) openSprints() []tracker.Sprint {
	var out []tracker.Sprint
	for _, s := range m.Sprints {
		if s.State != tracker.SprintClosed {
			out = append(out, s)
		}
	}
	return out
}

func (m Issue) watching() []people.Person {
	out := make([]people.Person, 0, len(m.Watchers))
	for _, id := range m.Watchers {
		out = append(out, m.person(id))
	}
	return out
}

func (m Issue) hasComponent(name string) bool {
	for _, c := range m.Detail.Components {
		if c == name {
			return true
		}
	}
	return false
}

// commentHTML is the body of a comment of the Activity tab.
func (m Issue) comment(id string) (Comment, bool) {
	for _, c := range m.Comments {
		if c.ID == id {
			return c, true
		}
	}
	return Comment{}, false
}

func (m Issue) canHaveParent() bool { return m.Type != tracker.Epic && m.Type != tracker.Subtask }
func (m Issue) canHaveSubtasks() bool {
	return m.Type != tracker.Epic && m.Type != tracker.Subtask
}

// counted is "1 comment", "3 comments".
func counted(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// entryText says what a history entry changed, after the person's name:
// "changed the priority from Medium to High".
func entryText(e tracker.Entry) string {
	label := tracker.FieldLabel(e.Field)
	switch e.Field {
	case tracker.EntryCreated:
		return "created the issue"
	case tracker.EntryDeleted:
		return "deleted the issue"
	case tracker.FieldDescription:
		return "changed the description"
	case tracker.FieldRank:
		return "changed the rank"
	case tracker.FieldLabels, tracker.FieldComponents, tracker.FieldWatchers, tracker.FieldLinks:
		switch {
		case e.New != "" && e.Old != "":
			return "changed the " + label + ": added " + e.New + ", removed " + e.Old
		case e.New != "":
			return "added " + e.New + " to the " + label
		case e.Old != "":
			return "removed " + e.Old + " from the " + label
		}
		return "changed the " + label
	}
	switch {
	case e.Old == "" && e.New == "":
		return "changed the " + label
	case e.Old == "":
		return "set the " + label + " to " + e.New
	case e.New == "":
		return "cleared the " + label + " (was " + e.Old + ")"
	}
	return "changed the " + label + " from " + e.Old + " to " + e.New
}

// viewerStyle shows the viewer's own parts of the shared regions: the
// edit and delete buttons of their comments, the watch button's state,
// and hides their own typing indicator. The views know nothing of who
// looks at them; the document does (MP-02.1).
func viewerStyle(me string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if !people.ValidID(me) {
			return nil
		}
		var b strings.Builder
		b.WriteString(`<style>`)
		b.WriteString(`.tk-issue .tk-comment[data-author="` + me + `"] .tk-comment__own{display:inline-flex}`)
		b.WriteString(`.tk-issue .tk-typing [data-who="` + me + `"]{display:none}`)
		b.WriteString(`.tk-issue:has([data-watcher="` + me + `"]) .tk-watch__off{display:none}`)
		b.WriteString(`.tk-issue:has([data-watcher="` + me + `"]) .tk-watch__on{display:inline}`)
		b.WriteString(`</style>`)
		_, err := io.WriteString(w, b.String())
		return err
	})
}

// IssueLink is the attribute a row or a card puts on its link to an issue
// so that the issue opens in the side panel (IssuePanel) beside the list:
// <a href="/WEB-12" data-issue-panel="<issue id>">. A click with a
// modifier key, or without the panel on the page, follows the link.
const IssueLink = "data-issue-panel"

// IssueLinkAttrs are the attributes of such a link, for templ.
func IssueLinkAttrs(id string) templ.Attributes { return templ.Attributes{IssueLink: id} }

// showIf shows an element while expr holds. In a region the stream
// morphs, the morph keeps the hidden state the expression set
// (data-preserve-attr), and the element starts hidden, so nothing flashes
// before Datastar runs.
func showIf(expr string) templ.Attributes {
	return templ.Attributes{"hidden": true, "data-attr:hidden": "!(" + expr + ")", "data-preserve-attr": "hidden"}
}

// merge joins attribute sets; data-preserve-attr lists join too.
func merge(sets ...templ.Attributes) templ.Attributes {
	out := templ.Attributes{}
	for _, s := range sets {
		for k, v := range s {
			if k == "data-preserve-attr" {
				if old, ok := out[k].(string); ok {
					v = old + " " + v.(string)
				}
			}
			out[k] = v
		}
	}
	return out
}

// pending marks a control of field f busy and read-only while its op is
// pending (MP-09.1).
func pending(f string) templ.Attributes {
	return templ.Attributes{
		"data-attr:aria-busy": fieldExpr(noteBusy, f),
		"data-preserve-attr":  "aria-busy",
	}
}

// choice is a control that sends field f with a value and a ref.
func choice(f, value, ref string) templ.Attributes {
	a := templ.Attributes{"data-act": "choose", "data-field": f, "data-value": value}
	if ref != "" {
		a["data-ref"] = ref
	}
	return a
}

// menu is the attributes of a menu's button for field f: disabled while
// read-only, busy while f is pending, and the version it showed kept.
func menu(f string) templ.Attributes {
	return merge(locked, pending(f), templ.Attributes{"data-on:click": menuOpened})
}

// locked disables a control of a read-only page or an old version.
var locked = templ.Attributes{"data-attr:disabled": disabledWhile, "data-preserve-attr": "disabled"}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func joinComma(list []string) string { return strings.Join(list, ", ") }

// The hints under the text fields: what the markup reads.
const (
	descHint    = "**bold**, _italic_, `code`, [a link](https://…), - lists, [[WEB-12]] for an issue, @ and a person's id to mention them. Ctrl Enter or a click outside saves, Escape cancels."
	markupHint  = "**bold**, _italic_, `code`, [a link](https://…), - lists, [[WEB-12]] for an issue, @ and a person's id to mention them."
	commentHint = "**bold**, _italic_, `code`, links, - lists, @ to mention someone, [[WEB-12]] for an issue. Ctrl Enter sends."
)
