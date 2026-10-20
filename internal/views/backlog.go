package views

import (
	"github.com/mbolli/tickit/internal/tracker"
	"github.com/mbolli/tickit/internal/ui"
)

func sectionID(id string) string {
	if id == "" {
		return "backlog"
	}
	return id
}

func sprintMeta(s tracker.Sprint) string {
	switch {
	case s.Start != "" && s.End != "":
		return tracker.Date(s.Start) + " to " + tracker.Date(s.End)
	case s.End != "":
		return "ends " + tracker.Date(s.End)
	}
	return ""
}

// backlogFaces are the faces the backlog's frame refers to.
func backlogFaces(d BacklogData) []ui.Person {
	var out []ui.Person
	seen := map[string]bool{}
	add := func(cards []tracker.Card) {
		for _, c := range cards {
			if c.Assignee != "" && !seen[c.Assignee] {
				seen[c.Assignee] = true
				out = append(out, UIPerson(d.People[c.Assignee]))
			}
		}
	}
	for _, s := range d.View.Sprints {
		add(s.Cards)
	}
	add(d.View.Cards)
	return out
}
