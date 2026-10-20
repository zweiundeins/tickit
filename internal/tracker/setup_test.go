package tracker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/dbtest"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
)

var t0 = time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)

// The cast: the guests Sunny and Rainy, accounts for each role, seeded
// Ada.
var (
	sunny     = people.GuestOf("session-sunny")
	rainy     = people.GuestOf("session-rainy")
	viewer    = people.Person{ID: "a-0192a000-0000-7000-8000-00000000000a", Kind: people.Account, Name: "Vera Viewer", Role: people.NoRole}
	member    = people.Person{ID: "a-0192a000-0000-7000-8000-00000000000b", Kind: people.Account, Name: "Mia Member", Role: people.NoRole}
	admin     = people.Person{ID: "a-0192a000-0000-7000-8000-00000000000c", Kind: people.Account, Name: "Adam Admin", Role: people.NoRole}
	moderator = people.Person{ID: "a-0192a000-0000-7000-8000-00000000000d", Kind: people.Account, Name: "Mo Moderator", Role: people.Moderator}
	siteAdmin = people.Person{ID: "a-0192a000-0000-7000-8000-00000000000e", Kind: people.Account, Name: "Sia Siteadmin", Role: people.Admin}
	ada       = people.Seeded[0]
)

// env is a database with the cast in it.
type env struct {
	*dbtest.Env
	t   *testing.T
	ops int
	now time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{Env: dbtest.Open(t), t: t, now: t0}
	e.Do(t, people.SeedPeople{At: t0})
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		for _, g := range []people.Person{sunny, rainy} {
			if err := people.Ensure(tx, g, t0); err != nil {
				return err
			}
		}
		for _, p := range []people.Person{viewer, member, admin, moderator, siteAdmin} {
			if _, err := tx.Exec(`INSERT INTO people (id, kind, display_name, hue, created_at, last_seen_at) VALUES (?, 'account', ?, 0, 0, 0)`, p.ID, p.Name); err != nil {
				return err
			}
			role := map[people.SiteRole]string{people.NoRole: "none", people.Moderator: "moderator", people.Admin: "admin"}[p.Role]
			if _, err := tx.Exec(`INSERT INTO accounts (person_id, email, username, full_name, password_hash, site_role) VALUES (?, ?, ?, ?, 'x', ?)`,
				p.ID, p.ID+"@example.com", p.ID, p.Name, role); err != nil {
				return err
			}
		}
		return nil
	}))
	e.Touched()
	return e
}

// by is p acting a second after the last action.
func (e *env) by(p people.Person) tabs.By {
	e.ops++
	e.now = e.now.Add(time.Second)
	return tabs.By{Actor: p, Op: "op" + itoa(e.ops), At: e.now}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// do runs cmd and returns its error.
func (e *env) do(cmd writer.Command) error {
	return e.W.Do(context.Background(), cmd)
}

// must runs cmd and fails the test on its error.
func (e *env) must(cmd writer.Command) {
	e.t.Helper()
	if err := e.do(cmd); err != nil {
		e.t.Fatalf("%T: %v", cmd, err)
	}
}

// result runs a command with a result and decodes it.
func (e *env) result(cmd writer.ResultCommand) map[string]string {
	e.t.Helper()
	b, err := e.W.DoValue(context.Background(), func(tx *writer.Tx) ([]byte, error) { return cmd.ApplyResult(tx) })
	if err != nil {
		e.t.Fatalf("%T: %v", cmd, err)
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		e.t.Fatal(err)
	}
	return m
}

// project creates a project as admin (who becomes its Admin), open or
// closed, with member and viewer as members of a closed one.
func (e *env) project(key string, open bool) string {
	e.t.Helper()
	id := e.result(CreateProject{By: e.by(admin), Key: key, Name: key + " project", Open: open})["id"]
	if !open {
		e.must(writer.Func(func(tx *writer.Tx) error {
			if err := access.SetMember(tx, access.ProjectScope(id), member.ID, access.Member, admin.ID, t0); err != nil {
				return err
			}
			return access.SetMember(tx, access.ProjectScope(id), viewer.ID, access.Viewer, admin.ID, t0)
		}))
	}
	return id
}

// issue creates an issue as p and returns its id.
func (e *env) issue(p people.Person, c CreateIssue) string {
	e.t.Helper()
	c.By = e.by(p)
	if c.Type == "" {
		c.Type = Task
	}
	if c.Summary == "" {
		c.Summary = "An issue"
	}
	return e.result(c)["id"]
}

// state is an issue's state row.
func (e *env) state(id string) IssueState {
	e.t.Helper()
	var s IssueState
	e.must(writer.Func(func(tx *writer.Tx) error {
		var err error
		s, err = issueState(tx, id)
		return err
	}))
	return s
}

// refusal returns err's refusal message, failing the test without one.
func refusal(t *testing.T, err error) string {
	t.Helper()
	var r *writer.Refusal
	if !errors.As(err, &r) {
		t.Fatalf("want a refusal, got %v", err)
	}
	return r.Msg
}

// row reads one value of a query.
func (e *env) row(query string, args ...any) string {
	e.t.Helper()
	var v sql.NullString
	if err := e.DB.R.QueryRow(query, args...).Scan(&v); err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
	return v.String
}
