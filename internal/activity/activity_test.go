package activity

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/dbtest"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
	"github.com/mbolli/tickit/internal/tracker"
)

var (
	t0     = time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	sunny  = people.GuestOf("session-sunny")
	rainy  = people.GuestOf("session-rainy")
	member = people.Person{ID: "a-0192a000-0000-7000-8000-00000000000b", Kind: people.Account, Name: "Mia Member", Role: people.NoRole}
)

type env struct {
	*dbtest.Env
	t   *testing.T
	ops int
	now time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{Env: dbtest.Open(t), t: t, now: t0}
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		for _, g := range []people.Person{sunny, rainy} {
			if err := people.Ensure(tx, g, t0); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`INSERT INTO people (id, kind, display_name, hue, created_at, last_seen_at) VALUES (?, 'account', ?, 0, 0, 0)`, member.ID, member.Name)
		if err == nil {
			_, err = tx.Exec(`INSERT INTO accounts (person_id, email, username, full_name, password_hash) VALUES (?, 'm@example.com', 'mia', 'Mia', 'x')`, member.ID)
		}
		return err
	}))
	return e
}

func (e *env) by(p people.Person) tabs.By {
	e.ops++
	e.now = e.now.Add(time.Second)
	return tabs.By{Actor: p, Op: fmt.Sprint("op", e.ops), At: e.now}
}

func (e *env) must(cmd writer.Command) {
	e.t.Helper()
	if err := e.W.Do(context.Background(), cmd); err != nil {
		e.t.Fatalf("%T: %v", cmd, err)
	}
}

func (e *env) id(cmd writer.ResultCommand) string {
	e.t.Helper()
	b, err := e.W.DoValue(context.Background(), func(tx *writer.Tx) ([]byte, error) { return cmd.ApplyResult(tx) })
	if err != nil {
		e.t.Fatalf("%T: %v", cmd, err)
	}
	var m map[string]string
	json.Unmarshal(b, &m)
	return m["id"]
}

// lines are entries as "actor action label [status|text] xN".
func lines(list []Entry) []string {
	var out []string
	for _, e := range list {
		s := e.Actor[:4] + " " + string(e.Action) + " " + e.Subject.Label
		if e.Status != "" {
			s += " " + e.Status
		}
		if e.Text != "" {
			s += " " + e.Text
		}
		if e.Edits > 1 {
			s += fmt.Sprintf(" x%d", e.Edits)
		}
		out = append(out, s)
	}
	return out
}

func (e *env) feed(f func(ctx context.Context, q sqlite.Querier) ([]Entry, bool, error)) ([]string, bool) {
	e.t.Helper()
	var list []Entry
	var more bool
	e.View(e.t, func(tx *sql.Tx) error {
		var err error
		list, more, err = f(context.Background(), tx)
		return err
	})
	return lines(list), more
}

func check(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n\t%s\nwant\n\t%s", strings.Join(got, "\n\t"), strings.Join(want, "\n\t"))
	}
}

// FR-69.1, FR-69.2: issues created, moved and commented on, newest first,
// with what moved them where; other changes and deleted issues are left
// out; a closed project's activity only for its members; 50 at a time.
func TestFeeds(t *testing.T) {
	e := newEnv(t)
	g := sunny.ID[:4]
	web := e.id(tracker.CreateProject{By: e.by(sunny), Key: "WEB", Name: "Website", Open: true})
	ops := e.id(tracker.CreateProject{By: e.by(member), Key: "OPS", Name: "Ops", Open: false})
	a := e.id(tracker.CreateIssue{By: e.by(sunny), Project: web, Type: tracker.Task, Summary: "Login"})
	b := e.id(tracker.CreateIssue{By: e.by(sunny), Project: web, Type: tracker.Bug, Summary: "Crash"})
	e.id(tracker.CreateIssue{By: e.by(member), Project: ops, Type: tracker.Task, Summary: "Secret"})
	e.must(tracker.SetStatus{By: e.by(sunny), Issue: a, Seen: 1 << 40, Status: tracker.StatusInProgress})
	e.must(tracker.SetPriority{By: e.by(sunny), Issue: a, Seen: 1 << 40, Priority: tracker.High})
	e.must(tracker.Move{By: e.by(rainy), Issue: a, Status: tracker.StatusInProgress, Seen: 1 << 40}) // the same column
	e.id(tracker.AddComment{By: e.by(rainy), Issue: a, Body: tracker.PlainDoc("Looks   fine\nto me.")})
	e.must(tracker.DeleteIssue{By: e.by(sunny), Issue: b})

	var guest, staff []access.Scope
	e.View(t, func(tx *sql.Tx) error {
		var err error
		if guest, err = Readable(context.Background(), tx, rainy); err == nil {
			staff, err = Readable(context.Background(), tx, member)
		}
		return err
	})
	if len(guest) != 1 || len(staff) != 2 || Key(guest) == Key(staff) {
		t.Fatalf("readable: %v and %v", guest, staff)
	}
	r := rainy.ID[:4]
	got, more := e.feed(func(ctx context.Context, q sqlite.Querier) ([]Entry, bool, error) { return Home(ctx, q, guest, 50) })
	check(t, got, r+" issue_commented WEB-1 Looks fine to me.", g+" issue_moved WEB-1 In Progress", g+" issue_created WEB-1")
	if more {
		t.Error("more")
	}
	got, _ = e.feed(func(ctx context.Context, q sqlite.Querier) ([]Entry, bool, error) { return Home(ctx, q, staff, 50) })
	if len(got) != 4 || !strings.Contains(got[2], "OPS-1") {
		t.Errorf("staff: %q", got)
	}
	got, _ = e.feed(func(ctx context.Context, q sqlite.Querier) ([]Entry, bool, error) { return Project(ctx, q, ops, 50) })
	check(t, got, member.ID[:4]+" issue_created OPS-1")
	got, more = e.feed(func(ctx context.Context, q sqlite.Querier) ([]Entry, bool, error) { return Home(ctx, q, guest, 2) })
	if len(got) != 2 || !more {
		t.Errorf("a page of 2: %q %v", got, more)
	}

	// Many more events than a window: the feed goes back through them.
	for i := range 300 {
		e.must(tracker.SetPriority{By: e.by(sunny), Issue: a, Seen: 1 << 40, Priority: tracker.Priorities[i%4]})
	}
	got, _ = e.feed(func(ctx context.Context, q sqlite.Querier) ([]Entry, bool, error) { return Project(ctx, q, web, 50) })
	if len(got) != 3 {
		t.Errorf("behind 600 events: %q", got)
	}

	// Recently edited by you: each subject once, newest first.
	var mine []Entry
	e.View(t, func(tx *sql.Tx) error {
		var err error
		mine, err = Mine(context.Background(), tx, rainy.ID, In(guest), 10)
		return err
	})
	if len(mine) != 1 || mine[0].Subject.Label != "WEB-1" {
		t.Errorf("mine: %+v", mine)
	}
}

// FR-69.3: consecutive edits of one page by one person within 10 minutes
// are one entry; another person's edit between them, or a pause, splits
// them. The wiki's source is faked here, with its events in the log.
func TestPageEditsMerge(t *testing.T) {
	e := newEnv(t)
	space := access.SpaceScope("space-1")
	addSource("testpage", []string{"testpage.edited"}, func(ctx context.Context, q sqlite.Querier, evs []events.Raw) ([]Entry, error) {
		var out []Entry
		for _, ev := range evs {
			out = append(out, Entry{Position: ev.Position, At: ev.Meta.At, Actor: ev.Meta.Actor, Action: PageEdited,
				Subject: Subject{Kind: "page", ID: fmt.Sprint(ev.ID), Label: "Page " + fmt.Sprint(ev.ID), Scope: space}})
		}
		return out, nil
	})
	defer delete(sources, "testpage")
	e.must(tracker.CreateProject{By: e.by(sunny), Key: "WEB", Name: "Website", Open: true}) // the log's tables
	edit := func(page string, who people.Person, minute int) {
		e.must(writer.Func(func(tx *writer.Tx) error {
			_, err := tx.Exec(`INSERT INTO lith_events (stream, stream_id, version, type, data, at, actor)
				VALUES ('testpage', ?, (SELECT count(*) + 1 FROM lith_events WHERE stream = 'testpage' AND stream_id = ?), 'testpage.edited', '{}', ?, ?)`,
				page, page, t0.Add(time.Duration(minute)*time.Minute).UnixMilli(), who.ID)
			return err
		}))
	}
	edit("a", sunny, 0)
	edit("a", sunny, 5)
	edit("a", sunny, 14) // within 10 minutes of the one before
	edit("a", rainy, 15)
	edit("a", sunny, 16)
	edit("a", sunny, 40) // a pause
	edit("b", sunny, 41)
	// After them, what no feed shows: a type the source does not name, and
	// a co-edited page's 3,000 commits, read past by the log's index.
	e.must(writer.Func(func(tx *writer.Tx) error {
		_, err := tx.Exec(`INSERT INTO lith_events (stream, stream_id, version, type, data, at, actor)
			VALUES ('testpage', 'a', 99, 'testpage.other', '{}', ?, ?)`, t0.Add(42*time.Minute).UnixMilli(), sunny.ID)
		for i := range 3000 {
			if err != nil {
				return err
			}
			_, err = tx.Exec(`INSERT INTO lith_events (stream, stream_id, version, type, data, at, actor)
				VALUES ('noise', 'n', ?, 'noise.committed', '{}', ?, ?)`, i+1, t0.Add(43*time.Minute).UnixMilli(), sunny.ID)
		}
		return err
	}))
	got, _ := e.feed(func(ctx context.Context, q sqlite.Querier) ([]Entry, bool, error) {
		return Space(ctx, q, "space-1", 50)
	})
	s, r := sunny.ID[:4], rainy.ID[:4]
	check(t, got, s+" page_edited Page b", s+" page_edited Page a", s+" page_edited Page a", r+" page_edited Page a", s+" page_edited Page a x3")
}

// FR-69: an entry quotes a comment with the spaces beside its mentions and
// chips, as the comment reads.
func TestExcerptKeepsSpaces(t *testing.T) {
	body := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Past its date. "},{"type":"mention","attrs":{"personId":"s-tomasz","label":"Tomasz Nowak"}},{"type":"text","text":", after "},{"type":"issue_chip","attrs":{"key":"WEB-44"}},{"type":"text","text":"."}]},{"type":"paragraph","content":[{"type":"text","text":"Next  one"}]}]}`
	if got, want := excerpt([]byte(body)), "Past its date. @Tomasz Nowak, after WEB-44. Next one"; got != want {
		t.Errorf("excerpt = %q, want %q", got, want)
	}
}
