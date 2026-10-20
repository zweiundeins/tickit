package demo_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/dbtest"
	"github.com/mbolli/tickit/internal/demo"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
	"github.com/mbolli/tickit/internal/tracker"
)

func count(t *testing.T, e *dbtest.Env, q string, args ...any) int {
	t.Helper()
	var n int
	if err := e.DB.R.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

// guestWrites has a guest create an issue and comment on a seeded one,
// through tabs.Act as an action does.
func guestWrites(t *testing.T, e *dbtest.Env, d *demo.Demo, g people.Person, op string) (string, error) {
	t.Helper()
	var project, seeded string
	if err := e.DB.R.QueryRow(`SELECT project_id, id FROM issues WHERE key = 'WEB-1'`).Scan(&project, &seeded); err != nil {
		t.Fatal(err)
	}
	by := tabs.By{Actor: g, Op: op, At: time.Now()}
	id := tracker.NewID()
	err := e.W.Do(context.Background(), tabs.Act{Session: "sid-" + g.ID, Tab: "tab", By: by,
		Cmd: d.Guard(g, "203.0.113.7", tracker.CreateIssue{By: by, ID: id, Project: project, Type: tracker.Task, Summary: "A guest's issue"})})
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "Hello"}}}}})
	by.Op += "c"
	err = e.W.Do(context.Background(), tabs.Act{Session: "sid-" + g.ID, Tab: "tab", By: by, Cmd: d.Guard(g, "203.0.113.7", tracker.AddComment{By: by, Issue: seeded, Body: body})})
	return id, err
}

func newDemo(t *testing.T, e *dbtest.Env) *demo.Demo {
	t.Helper()
	d, err := demo.New(demo.Config{DB: e.DB, W: e.W, Content: content(t), Now: func() time.Time { return run.Add(time.Minute) }})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// DM-03.3, NFR-72: a reset deletes every visitor's issues, comments,
// notifications, tab state, events and guests, keeps staff accounts,
// their logins, the security and the moderation log, seeds again, and
// bumps the epoch (MP-13.2). The event log and the projections stay one:
// a rebuild after the reset equals the live tables (FR-155.2).
func TestReset(t *testing.T) {
	e, _ := seeded(t)
	d := newDemo(t, e)
	seededEvents := count(t, e, `SELECT count(*) FROM lith_events`)
	g := people.GuestOf("session-visitor")
	if _, err := guestWrites(t, e, d, g, "op1"); err != nil {
		t.Fatal(err)
	}
	staff := people.NewAccountID()
	hash, _ := people.HashPassword("correct horse battery")
	e.Do(t, people.CreateAccount{ID: staff, Email: "mo@example.com", Username: "mo", Role: people.Moderator, PasswordHash: hash, At: run})
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		_, err := tx.Exec(`INSERT INTO login_sessions (sid_hash, person_id, created_at, expires_at, last_seen_at, user_agent) VALUES ('h', ?, 0, ?, 0, 'x')`, staff, time.Now().Add(time.Hour).Unix())
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO moderation_log (at, moderator, action, target_kind, target_id) VALUES (1, ?, 'hide', 'issue', 'x')`, staff)
		return err
	}))
	if count(t, e, `SELECT count(*) FROM people WHERE id = ?`, g.ID) != 1 || count(t, e, `SELECT count(*) FROM tab_state`) != 1 {
		t.Fatal("the guest left no trace to delete")
	}
	before, _ := demo.Epoch(context.Background(), e.DB.R)
	if err := d.Reset(context.Background(), staff); err != nil {
		t.Fatal(err)
	}
	for q, want := range map[string]int{
		`SELECT count(*) FROM people WHERE kind = 'guest'`:                               0,
		`SELECT count(*) FROM issues WHERE summary = 'A guest''s issue'`:                 0,
		`SELECT count(*) FROM comments WHERE author LIKE 'g-%'`:                          0,
		`SELECT count(*) FROM tab_state`:                                                 0,
		`SELECT count(*) FROM lith_events WHERE actor LIKE 'g-%'`:                        0,
		`SELECT count(*) FROM issues`:                                                    len(content(t).Issues),
		`SELECT count(*) FROM lith_events`:                                               seededEvents,
		`SELECT count(*) FROM accounts WHERE person_id = '` + staff + `'`:                1,
		`SELECT count(*) FROM login_sessions WHERE person_id = '` + staff + `'`:          1,
		`SELECT count(*) FROM moderation_log`:                                            1,
		`SELECT count(*) FROM security_log WHERE kind = 'account_created'`:               1,
		`SELECT count(*) FROM demo_resets WHERE outcome = 'ok' AND by = '` + staff + `'`: 1,
		`SELECT count(*) FROM people WHERE kind = 'seed'`:                                6,
	} {
		if got := count(t, e, q); got != want {
			t.Errorf("%s: %d, want %d", q, got, want)
		}
	}
	if after, _ := demo.Epoch(context.Background(), e.DB.R); after != before+1 {
		t.Errorf("epoch %d after %d", after, before)
	}
	// The same keys as a seed of a new database.
	fresh, _ := seeded(t)
	if a, b := dump(t, e, `SELECT key, status, assignee, rank FROM issues ORDER BY key`), dump(t, fresh, `SELECT key, status, assignee, rank FROM issues ORDER BY key`); a != b {
		t.Error("the reset seeded other issues than a new database has")
	}
	// A rebuild of every tracker table equals the live ones.
	tables := []string{"projects", "statuses", "components", "issues", "issue_labels", "labels", "issue_links", "issue_watchers", "comments",
		"spaces", "pages", "page_labels", "page_watchers", "page_versions", "page_chips", "page_links", "page_comment_threads"}
	live := map[string]string{}
	for _, tb := range tables {
		live[tb] = dump(t, e, `SELECT * FROM `+tb+` ORDER BY 1, 2`)
	}
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		_, err := tx.Exec(`UPDATE lith_projections SET version = version + 100 WHERE name LIKE 'tracker_%' OR name LIKE 'wiki_%'`)
		return err
	}))
	r := e.Run(t, tracker.Store())
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		ps, done := r.Stats().Projectors, true
		for _, p := range ps {
			done = done && p.Status == "live"
		}
		if done && len(ps) >= len(tracker.Parts()) || time.Now().After(deadline) {
			break
		}
	}
	for _, tb := range tables {
		if got := dump(t, e, `SELECT * FROM `+tb+` ORDER BY 1, 2`); got != live[tb] {
			t.Errorf("%s rebuilt after the reset differs", tb)
			a, b := strings.Split(got, "\n"), strings.Split(live[tb], "\n")
			for i := range min(len(a), len(b)) {
				if a[i] != b[i] {
					t.Logf("rebuilt %s\nlive    %s", a[i], b[i])
					break
				}
			}
		}
	}
}

// DM-03.4: while the demo resets, every action is refused with
// ErrResetting and changes nothing.
func TestResetRefusesActions(t *testing.T) {
	e, _ := seeded(t)
	d := newDemo(t, e)
	release := d.Hold()
	_, err := guestWrites(t, e, d, people.GuestOf("session-early"), "op1")
	if !errors.Is(err, demo.ErrResetting) {
		t.Errorf("an action during a reset: %v", err)
	}
	if n := count(t, e, `SELECT count(*) FROM people WHERE kind = 'guest'`); n != 0 {
		t.Errorf("%d guests written during a reset", n)
	}
	if err := d.Reset(context.Background(), ""); !errors.Is(err, demo.ErrResetting) {
		t.Errorf("a second reset: %v", err)
	}
	release()
	if _, err := guestWrites(t, e, d, people.GuestOf("session-early"), "op2"); err != nil {
		t.Errorf("after the reset: %v", err)
	}
}

// DM-04.3: past the cap, a guest's new issue is refused with MsgFull;
// accounts are not counted, and the reset starts the count anew.
func TestCaps(t *testing.T) {
	e, _ := seeded(t)
	d := newDemo(t, e)
	old := demo.Caps[demo.CapIssue]
	demo.Caps[demo.CapIssue] = 2
	t.Cleanup(func() { demo.Caps[demo.CapIssue] = old })
	for i, want := range []bool{true, true, false} {
		g := people.GuestOf("session-cap-" + string(rune('a'+i)))
		_, err := guestWrites(t, e, d, g, "op")
		if err != nil {
			t.Fatal(err)
		}
		var data string
		e.View(t, func(tx *sql.Tx) error {
			return tx.QueryRow(`SELECT data FROM tab_state WHERE sid = ?`, "sid-"+g.ID).Scan(&data)
		})
		if full := strings.Contains(data, demo.MsgFull); full == want {
			t.Errorf("issue %d: refused %v (%s)", i+1, full, data)
		}
	}
	if n := count(t, e, `SELECT count FROM demo_caps WHERE kind = 'issue'`); n != 2 {
		t.Errorf("counted %d", n)
	}
	if err := d.Reset(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if n := count(t, e, `SELECT count(*) FROM demo_caps`); n != 0 {
		t.Errorf("%d caps after the reset", n)
	}
}
