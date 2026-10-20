package views

import (
	"net/url"
	"strconv"
	"time"

	"github.com/zweiundeins/tickit/internal/activity"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/ui"
)

// Work is the home page's part of one person (FR-59): their open issues,
// what they changed lately, the projects and spaces they may read. The
// home view (home:<person>) patches it by HomeWorkID.
type Work struct {
	Me       people.Person
	Now      time.Time
	Assigned []tracker.Issue // open, newest change first
	Total    int             // all the open issues assigned to them
	Recent   []activity.Entry
	// Viewed are the issues and pages they viewed lately (FR-59.1).
	Viewed   []RecentItem
	Projects []tracker.Project
	Spaces   []Space
}

// RecentItem is an issue or a page a person viewed: Key and Type for an
// issue, Space for a page.
type RecentItem struct {
	Kind  string // "issue" or "page"
	Href  string
	Title string
	Key   string
	Type  tracker.IssueType
	Space string
	At    time.Time
}

// HomeViewed is how many viewed issues and pages the home page lists.
const HomeViewed = 8

// Space is a space the home page links to.
type Space struct {
	Key, Name string
}

// HomeWorkID is the id of the person's part of the home page.
const HomeWorkID = "home-work"

// HomeAssigned is how many assigned issues the home page lists.
const HomeAssigned = 10

// Addresses of what the home page links to; the areas' routes own them.
func projectHref(key string) string { return "/projects/" + url.PathEscape(key) }
func spaceHref(key string) string   { return "/spaces/" + url.PathEscape(key) }

// assignedHref is the issue list of the open issues assigned to the person.
const assignedHref = "/search?assignee=me&category=todo&category=inprogress&sort=updated&dir=desc"

func (w Work) stats() []ui.Stat {
	week := w.Now.AddDate(0, 0, 7).Format(time.DateOnly)
	due, overdue := 0, 0
	for _, i := range w.Assigned {
		switch {
		case i.Overdue(w.Now):
			overdue++
		case i.Due != "" && i.Due <= week:
			due++
		}
	}
	return []ui.Stat{
		{Label: "Assigned to you", Value: strconv.Itoa(w.Total), Href: assignedHref},
		{Label: "Due this week", Value: strconv.Itoa(due), Tone: tone(due > 0, ui.ToneWarn)},
		{Label: "Overdue", Value: strconv.Itoa(overdue), Tone: tone(overdue > 0, ui.ToneDanger)},
	}
}

func tone(on bool, t ui.Tone) ui.Tone {
	if on {
		return t
	}
	return ui.ToneNeutral
}

// weekday is the eyebrow over the home page's title: "Friday".
func weekday(t time.Time) string { return t.Format("Monday") }

func openIssues(n int) string {
	if n == 1 {
		return "1 open issue"
	}
	return strconv.Itoa(n) + " open issues"
}

// dueDate is a due date (YYYY-MM-DD) as a time in now's zone.
func dueDate(d string, now time.Time) time.Time {
	t, _ := time.ParseInLocation(time.DateOnly, d, now.Location())
	return t
}
