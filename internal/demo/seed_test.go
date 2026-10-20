package demo_test

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/dbtest"
	"github.com/mbolli/tickit/internal/demo"
	"github.com/mbolli/tickit/internal/demo/seed"
	"github.com/mbolli/tickit/internal/tracker"
)

var zurich, _ = time.LoadLocation("Europe/Zurich")

// run is the seed's run time of the tests.
var run = time.Date(2026, 10, 20, 4, 0, 0, 0, zurich)

func content(t testing.TB) *seed.Content {
	t.Helper()
	c, err := seed.Load()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// seeded is a database with the seed in it.
func seeded(t testing.TB) (*dbtest.Env, demo.Seeded) {
	t.Helper()
	e := dbtest.Open(t)
	n, err := demo.Seed(context.Background(), e.W, content(t), run, 1)
	if err != nil {
		t.Fatal(err)
	}
	return e, n
}

// DM-02.1, DM-02.4: the seed writes every project, issue, change, link
// and comment of the content in under 30 s, and each issue ends as the
// content says, in the content's board order.
func TestSeed(t *testing.T) {
	c := content(t)
	e, n := seeded(t)
	t.Logf("seeded %+v", n)
	if n.Took > 30*time.Second {
		t.Errorf("the seed took %s", n.Took)
	}
	var changes, links, comments int
	for _, is := range c.Issues {
		changes += len(is.History)
		links += len(is.Links)
		comments += len(is.Comments)
	}
	if n.Projects != len(c.Projects) || n.Issues != len(c.Issues) || n.Changes != changes || n.Links != links || n.Comments != comments {
		t.Errorf("seeded %+v; the content has %d projects, %d issues, %d changes, %d links, %d comments",
			n, len(c.Projects), len(c.Issues), changes, links, comments)
	}
	count := func(q string, args ...any) int {
		var v int
		if err := e.DB.R.QueryRow(q, args...).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if got := count(`SELECT count(*) FROM issues`); got != len(c.Issues) {
		t.Errorf("%d issues", got)
	}
	if got := count(`SELECT count(*) FROM comments`); got != comments {
		t.Errorf("%d comments", got)
	}
	if got := count(`SELECT count(*) FROM issue_links WHERE outward = 1`); got != links {
		t.Errorf("%d links", got)
	}
	// The log's times grow with its positions, inside the 30 days.
	if got := count(`SELECT count(*) FROM lith_events a JOIN lith_events b ON b.position = a.position + 1 WHERE b.at < a.at`); got != 0 {
		t.Errorf("%d events older than the one before", got)
	}
	if got := count(`SELECT count(*) FROM lith_events WHERE at < ? OR at >= ?`, run.AddDate(0, 0, -31).UnixMilli(), run.UnixMilli()); got != 0 {
		t.Errorf("%d events outside the month before the run", got)
	}
	e.View(t, func(tx *sql.Tx) error {
		for _, is := range c.Issues {
			var status, priority, typ, assignee string
			if err := tx.QueryRow(`SELECT s.name, i.priority, i.type, i.assignee FROM issues i JOIN statuses s ON s.project_id = i.project_id AND s.id = i.status WHERE i.key = ?`, is.Key).
				Scan(&status, &priority, &typ, &assignee); err != nil {
				return fmt.Errorf("%s: %w", is.Key, err)
			}
			if status != is.Status || priority != strings.ToLower(is.Priority) || typ != strings.ToLower(is.Type) || assignee != is.Assignee {
				t.Errorf("%s: %s %s %s %q, want %+v", is.Key, status, priority, typ, assignee, is)
			}
		}
		for _, p := range c.Projects {
			rows, err := tx.Query(`SELECT key FROM issues WHERE project_key = ? ORDER BY rank`, p.Key)
			if err != nil {
				return err
			}
			var got []string
			for rows.Next() {
				var k string
				rows.Scan(&k)
				got = append(got, k)
			}
			rows.Close()
			var want []*seed.Issue
			for k := range c.Issues {
				if c.Issues[k].Project == p.Key {
					want = append(want, &c.Issues[k])
				}
			}
			slices.SortFunc(want, func(a, b *seed.Issue) int { return a.Rank - b.Rank })
			for i, is := range want {
				if i >= len(got) || got[i] != is.Key {
					t.Errorf("%s's board order: %v", p.Key, got)
					break
				}
			}
		}
		// The history tab reads like the content's history.
		is := c.Issue("WEB-1")
		var id string
		if err := tx.QueryRow(`SELECT id FROM issues WHERE key = 'WEB-1'`).Scan(&id); err != nil {
			return err
		}
		h, err := tracker.History(context.Background(), tx, id)
		if err != nil {
			return err
		}
		var statuses int
		for _, en := range h {
			if en.Field == tracker.FieldStatus {
				statuses++
			}
		}
		if want := countField(is, "status"); statuses != want {
			t.Errorf("WEB-1's history has %d status changes, the content %d", statuses, want)
		}
		return nil
	})
}

func countField(is *seed.Issue, field string) int {
	n := 0
	for _, h := range is.History {
		if h.Field == field {
			n++
		}
	}
	return n
}

// DM-02.4: two runs give the same ids, keys, history and log.
func TestSeedTwice(t *testing.T) {
	a, _ := seeded(t)
	b, _ := seeded(t)
	for _, q := range []string{
		`SELECT id, key, status, priority, assignee, rank, version, created_at FROM issues ORDER BY id`,
		`SELECT id, subject_id, author, body, created_at FROM comments ORDER BY id`,
		`SELECT position, stream, stream_id, version, type, data, at, actor FROM lith_events ORDER BY position`,
	} {
		if x, y := dump(t, a, q), dump(t, b, q); x != y {
			t.Errorf("two runs differ in %s", q)
		}
	}
}

func dump(t *testing.T, e *dbtest.Env, q string) string {
	t.Helper()
	var b strings.Builder
	rows, err := e.DB.R.Query(q)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		rows.Scan(ptrs...)
		fmt.Fprintf(&b, "%q\n", vals)
	}
	return b.String()
}
