package views

import (
	"strconv"
	"time"

	"github.com/mbolli/tickit/internal/activity"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/ui"
)

// ActivityFeed is a feed of recent changes (FR-69) as a page shows it:
// the home page's, a project's, a space's. A page renders it with the id
// its live view patches (ActivityID) and names the view in its stream.
type ActivityFeed struct {
	ID      string // ActivityID of the feed's scope
	Entries []activity.Entry
	People  map[string]people.Person
	Now     time.Time
	// More links to the feed with 50 more entries, when there are more.
	More string
	// Label names the list for screen readers: "Recent activity".
	Label string
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
