// Package trackertest makes a project with a board's worth of issues, for
// the tests of pages and browsers: the seeded people act, as the demo's
// seed does.
package trackertest

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
)

// Project is a project made by Board.
type Project struct {
	ID, Key string
	Keys    []string          // the issues' keys, in the order they were made
	IDs     map[string]string // key to id
}

var summaries = []string{
	"Checkout shows the wrong VAT for Liechtenstein",
	"Make the board render a hundred cards in one frame",
	"Size guide for the new hiking boots",
	"Search does not find umlauts in product names",
	"Newsletter sign-up loses the consent checkbox",
	"Add Twint as a payment method",
	"Product photos load twice on slow links",
	"Return label PDF has the old address",
	"Stock count is off after a cancelled order",
	"Translate the care instructions to Italian",
	"Wishlist button is hidden on narrow screens",
	"Gift cards cannot be combined with vouchers",
}

// Board makes a project with key and n issues spread over the board's
// columns, as the seeded people would: types, priorities, assignees,
// labels, points, due dates and an epic, so that cards show every part of
// FR-40.2. w writes; it must hold the migrated schema.
func Board(t testing.TB, w *writer.Writer, key string, n int) Project {
	t.Helper()
	ctx := context.Background()
	at := time.Now().Add(-time.Hour)
	op := 0
	by := func(p people.Person) tabs.By {
		op++
		at = at.Add(time.Millisecond)
		return tabs.By{Actor: p, Op: fmt.Sprintf("seed-%s-%d", key, op), At: at}
	}
	result := func(cmd writer.ResultCommand) map[string]string {
		b, err := w.DoValue(ctx, func(tx *writer.Tx) ([]byte, error) { return cmd.ApplyResult(tx) })
		if err != nil {
			t.Fatalf("%T: %v", cmd, err)
		}
		var m map[string]string
		json.Unmarshal(b, &m)
		return m
	}
	if err := w.Do(ctx, people.SeedPeople{At: at}); err != nil {
		t.Fatal(err)
	}
	lead := people.Seeded[0]
	pid := result(tracker.CreateProject{By: by(lead), Key: key, Name: key + " shop", Description: "The web shop of Alpenglow.", Lead: lead.ID, Open: true})["id"]
	p := Project{ID: pid, Key: key, IDs: map[string]string{}}
	epic := result(tracker.CreateIssue{By: by(lead), Project: pid, Type: tracker.Epic, Summary: "Board v1", Status: tracker.StatusInProgress})
	p.Keys = append(p.Keys, epic["key"])
	p.IDs[epic["key"]] = epic["id"]
	statuses := []string{tracker.StatusToDo, tracker.StatusToDo, tracker.StatusInProgress, tracker.StatusInReview, tracker.StatusSelected, tracker.StatusDone, tracker.StatusBlocked}
	types := []tracker.IssueType{tracker.Story, tracker.Task, tracker.Bug, tracker.Story, tracker.Task}
	for i := 1; i < n; i++ {
		who := people.Seeded[i%len(people.Seeded)]
		c := tracker.CreateIssue{By: by(who), Project: pid, Type: types[i%len(types)], Summary: summaries[i%len(summaries)],
			Status: statuses[i%len(statuses)], Priority: tracker.Priorities[i%len(tracker.Priorities)]}
		if i%4 != 0 {
			c.Assignee = people.Seeded[(i*7)%len(people.Seeded)].ID
		}
		if i%3 == 0 {
			c.Labels = []string{"checkout", "a11y"}[:1+i%2]
		}
		if i%5 == 0 {
			pts := []int{1, 2, 3, 5, 8}[i%5]
			c.Points = &pts
		}
		if i%6 == 0 {
			c.Due = at.AddDate(0, 0, i%9-3).Format(time.DateOnly)
		}
		if i%4 == 1 {
			c.Parent = epic["id"]
		}
		m := result(c)
		p.Keys = append(p.Keys, m["key"])
		p.IDs[m["key"]] = m["id"]
	}
	return p
}
