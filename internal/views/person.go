package views

import (
	"net/url"
	"time"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
)

// PersonPage is /people/<id> (FR-67.2): a person's face and name, the
// issues assigned to them and those they reported, and what they did
// lately in what the viewer may read. Its view (profile:<id>) patches it
// by PersonPageID; viewers who read the same projects and spaces share it.
type PersonPage struct {
	Person   people.Person
	Assigned tracker.Results // open, newest change first
	Reported tracker.Results // newest first
	Feed     ActivityFeed
	Now      time.Time
}

// PersonPageID is the id of a person page's region.
const PersonPageID = "person-page"

// PersonHref is the address of a person's page.
func PersonHref(id string) string { return "/people/" + url.PathEscape(id) }

// PersonShown is how many issues each list of a person's page shows.
const PersonShown = 10

func (p PersonPage) kind() string {
	switch p.Person.Kind {
	case people.Guest:
		return "Guest"
	case people.Seed:
		return "Demo person"
	}
	return "Account"
}

func (p PersonPage) assignedHref() string {
	return "/issues?" + url.Values{"assignee": {p.Person.ID}, "category": {"todo", "inprogress"}, "sort": {"updated"}, "dir": {"desc"}}.Encode()
}

func (p PersonPage) reportedHref() string {
	return "/issues?" + url.Values{"reporter": {p.Person.ID}}.Encode()
}

func issueTotal(n int) string {
	if n == 1 {
		return "1 issue"
	}
	return itoa(n) + " issues"
}
