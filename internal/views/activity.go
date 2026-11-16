package views

import (
	"strconv"
	"time"

	"github.com/a-h/templ"

	"github.com/zweiundeins/tickit/internal/activity"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/ui"
)

// ActivityFeed is a feed of recent changes (FR-69) as a page shows it:
// the home page's, a project's, a space's. A page renders it with the id
// its live view patches (ActivityID) and names the view in its stream.
//
// A feed with a Kind is a window of a long feed in sb-virtual-scroll: the
// entries from From on, of Total, which is -1 until a window reaches the
// feed's end. The page asks for another window by the view's name,
// Kind ":" and From, or Kind ":" Scope "." From (FeedSpec); the links to
// older and newer entries name the same window in Path's ?from=, for a
// page without script.
type ActivityFeed struct {
	ID      string // ActivityID of the feed's scope
	Entries []activity.Entry
	People  map[string]people.Person
	Now     time.Time
	// Label names the list for screen readers: "Recent activity".
	Label string

	Kind  string // the view's kind: "activity-home", "activity-project"
	Scope string // the project's or space's id, "" for the home feed
	From  int
	Total int
	// Window is how many entries a window holds, MaxFrom the last From a
	// window may start at.
	Window, MaxFrom int
	Path            string
}

// FeedStep is what a window's From is a multiple of, so that tabs scrolled
// to about the same place share a window's render. sb-virtual-scroll
// rounds the windows it asks for to it (step); rounding adds up to
// FeedStep-1 entries at each end, so it stays well under half a window.
const FeedStep = 10

// FeedBuffer is how far, in px, a window reaches past the feed's box: 8
// rows of 60px, which with the box and the rounding fits the 50 entries a
// window holds.
const FeedBuffer = 480

// FeedSpec is the name of the view of a feed's window from from.
func FeedSpec(kind, scope string, from int) string {
	if scope == "" {
		return kind + ":" + strconv.Itoa(from)
	}
	return kind + ":" + scope + "." + strconv.Itoa(from)
}

func (f ActivityFeed) faces() []ui.Person {
	out := make([]ui.Person, 0, len(f.Entries))
	for _, e := range f.Entries {
		out = append(out, UIPerson(f.person(e.Actor)))
	}
	return out
}

// Known says whether the feed's length is known.
func (f ActivityFeed) Known() bool { return f.Total >= 0 }

// entryKey keys an entry for sb-virtual-scroll. No id: with ids the morph
// moves the entries, and their label ids go by slot so it patches in place.
func (f ActivityFeed) entryKey(e activity.Entry) string {
	return strconv.FormatInt(e.Position, 10)
}

// older and newer are the windows after and before this one, "" for none.
func (f ActivityFeed) older() string {
	if f.Known() && f.From+len(f.Entries) >= f.Total || f.From+f.Window > f.MaxFrom {
		return ""
	}
	return f.Path + "?from=" + strconv.Itoa(f.From+f.Window)
}

func (f ActivityFeed) newer() string {
	if f.From == 0 {
		return ""
	}
	if f.From <= f.Window {
		return f.Path
	}
	return f.Path + "?from=" + strconv.Itoa(f.From-f.Window)
}

func (f ActivityFeed) listAttrs() templ.Attributes {
	return templ.Attributes{
		"data-feed-kind":    f.Kind,
		"data-feed-scope":   f.Scope,
		"data-feed-window":  strconv.Itoa(f.Window),
		"data-feed-max":     strconv.Itoa(f.MaxFrom),
		"data-on:sb-window": feedWindow,
	}
}

// ActivityID is the id of a feed's region: "activity-home",
// "activity-project-<id>", "activity-space-<id>".
func ActivityID(scope string) string { return "activity-" + scope }

func (f ActivityFeed) person(id string) people.Person {
	if p, ok := f.People[id]; ok {
		return p
	}
	return people.Person{ID: id, Kind: people.Deleted, Name: people.DeletedName}
}

// uiCategory is a status category as internal/ui names it.
func uiCategory(c string) ui.StatusCategory {
	switch c {
	case "inprogress":
		return ui.InProgress
	case "done":
		return ui.Done
	}
	return ui.ToDo
}

// editsText is how many edits an entry stands for: "3 times".
func editsText(n int) string {
	if n == 2 {
		return "twice"
	}
	return strconv.Itoa(n) + " times"
}

func (f ActivityFeed) label() string {
	if f.Label == "" {
		return "Recent activity"
	}
	return f.Label
}
