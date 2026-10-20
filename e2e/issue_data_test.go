package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
	"github.com/mbolli/tickit/internal/tracker"
	"github.com/mbolli/tickit/internal/web/webtest"
)

// issues is the data of the issue tests: the project Website (WEB) with an
// epic, a story in it with subtasks, labels, components, a link and
// comments, written as the seeded people.
type issues struct {
	project, epic, story, bug string
}

func issueData(t testing.TB, e *webtest.Env) issues {
	t.Helper()
	ada, ben := people.Seeded[0], people.Seeded[1]
	at := time.Now().Add(-3 * 24 * time.Hour)
	n := 0
	by := func(p people.Person) tabs.By {
		n++
		at = at.Add(17 * time.Minute)
		return tabs.By{Actor: p, Op: fmt.Sprintf("data%d", n), At: at}
	}
	d := issues{project: tracker.NewID(), epic: tracker.NewID(), story: tracker.NewID(), bug: tracker.NewID()}
	e.Do(t, people.SeedPeople{At: at})
	e.Do(t, tracker.CreateProject{By: by(ada), ID: d.project, Key: "WEB", Name: "Website", Open: true})
	e.Do(t, tracker.DefineComponent{By: by(ada), Project: d.project, Name: "Checkout"})
	e.Do(t, tracker.DefineComponent{By: by(ada), Project: d.project, Name: "Search"})
	e.Do(t, tracker.CreateSprint{By: by(ada), Project: d.project, Name: "Sprint 7"})
	e.Do(t, tracker.CreateIssue{By: by(ada), ID: d.epic, Project: d.project, Type: tracker.Epic, Summary: "Board v1"})
	pts, est := 3, 150
	e.Do(t, tracker.CreateIssue{By: by(ben), ID: d.story, Project: d.project, Type: tracker.Story, Parent: d.epic,
		Summary: "Cards glide between lanes when someone else moves them", Priority: tracker.High, Assignee: ada.ID,
		Labels: []string{"board", "motion"}, Components: []string{"Checkout"}, Points: &pts, Estimate: &est,
		Due: time.Now().AddDate(0, 0, 4).Format(time.DateOnly),
		Description: []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"When "},{"type":"text","marks":[{"type":"strong"}],"text":"someone else"},{"type":"text","text":" moves a card, it glides to its new place instead of jumping."}]},{"type":"bullet_list","content":[{"type":"list_item","content":[{"type":"paragraph","content":[{"type":"text","text":"Use view transitions where the browser has them."}]}]},{"type":"list_item","content":[{"type":"paragraph","content":[{"type":"text","text":"Respect reduced motion."}]}]}]}]}`)})
	e.Do(t, tracker.CreateIssue{By: by(ben), ID: d.bug, Project: d.project, Type: tracker.Bug, Parent: d.epic, Summary: "Safari drops a card one lane to the right", Priority: tracker.Critical})
	for _, s := range []string{"Measure the transition", "Fallback without view transitions"} {
		e.Do(t, tracker.CreateIssue{By: by(ada), Project: d.project, Type: tracker.Subtask, Parent: d.story, Summary: s})
	}
	e.Do(t, tracker.LinkIssues{By: by(ben), Issue: d.bug, Type: tracker.Blocks, Other: d.story})
	e.Do(t, tracker.SetStatus{By: by(ben), Issue: d.story, Seen: 1, Status: tracker.StatusInProgress})
	e.Do(t, tracker.AddComment{By: by(ben), Issue: d.story, Body: []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Reproduced on an iPad too; the drop marker is in the right place, the card is not."}]}]}`)})
	e.Do(t, tracker.AddComment{By: by(ada), Issue: d.story, Body: []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"The ghost uses page coordinates; Safari reports them before the scroll. See "},{"type":"issue_chip","attrs":{"key":"WEB-3"}},{"type":"text","text":"."}]}]}`)})
	return d
}

// TestServeIssues serves the issue data until TICKIT_SERVE's seconds pass,
// for looking at the pages by hand: TICKIT_SERVE=600 go test ./e2e -run TestServeIssues -v
func TestServeIssues(t *testing.T) {
	secs := os.Getenv("TICKIT_SERVE")
	if secs == "" {
		t.Skip("TICKIT_SERVE is not set")
	}
	var d time.Duration
	fmt.Sscan(secs, &d)
	e := webtest.Start(t)
	issueData(t, e)
	t.Logf("serving %s/WEB-2", e.URL)
	fmt.Println("URL", e.URL)
	time.Sleep(d * time.Second)
}
