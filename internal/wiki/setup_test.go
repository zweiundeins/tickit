package wiki

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/dbtest"
	"github.com/zweiundeins/tickit/internal/doc"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
)

// One store of the tracker's and the wiki's parts, as cmd/tickit makes:
// the wiki's projector of issue events runs in the tracker's commands.
var both = events.Must(events.New(append(tracker.Parts(), Parts()...)...))

func TestMain(m *testing.M) {
	tracker.Bind(both)
	Bind(both)
	os.Exit(m.Run())
}

var t0 = time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)

// The cast, as the tracker's tests have it.
var (
	sunny     = people.GuestOf("session-sunny")
	rainy     = people.GuestOf("session-rainy")
	viewer    = people.Person{ID: "a-0192a000-0000-7000-8000-00000000000a", Kind: people.Account, Name: "Vera Viewer", Role: people.NoRole}
	member    = people.Person{ID: "a-0192a000-0000-7000-8000-00000000000b", Kind: people.Account, Name: "Mia Member", Role: people.NoRole}
	admin     = people.Person{ID: "a-0192a000-0000-7000-8000-00000000000c", Kind: people.Account, Name: "Adam Admin", Role: people.NoRole}
	moderator = people.Person{ID: "a-0192a000-0000-7000-8000-00000000000d", Kind: people.Account, Name: "Mo Moderator", Role: people.Moderator}
	ada       = people.Seeded[0]
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
	e.Do(t, people.SeedPeople{At: t0})
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		for _, g := range []people.Person{sunny, rainy} {
			if err := people.Ensure(tx, g, t0); err != nil {
				return err
			}
		}
		for _, p := range []people.Person{viewer, member, admin, moderator} {
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

func (e *env) by(p people.Person) tabs.By {
	e.ops++
	e.now = e.now.Add(time.Second)
	return tabs.By{Actor: p, Op: "op" + strconv.Itoa(e.ops), At: e.now}
}

func (e *env) do(cmd writer.Command) error { return e.W.Do(context.Background(), cmd) }

func (e *env) must(cmd writer.Command) {
	e.t.Helper()
	if err := e.do(cmd); err != nil {
		e.t.Fatalf("%T: %v", cmd, err)
	}
}

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

// space creates a space as admin, its Admin: open, or closed with member
// and viewer as members.
func (e *env) space(key string, open bool) (id, home string) {
	e.t.Helper()
	m := e.result(CreateSpace{By: e.by(admin), Key: key, Name: key + " space", Open: open})
	if !open {
		e.must(writer.Func(func(tx *writer.Tx) error {
			if err := access.SetMember(tx, access.SpaceScope(m["id"]), member.ID, access.Member, admin.ID, t0); err != nil {
				return err
			}
			return access.SetMember(tx, access.SpaceScope(m["id"]), viewer.ID, access.Viewer, admin.ID, t0)
		}))
	}
	return m["id"], m["home"]
}

// page creates a page as p and returns its id.
func (e *env) page(p people.Person, c CreatePage) string {
	e.t.Helper()
	c.By = e.by(p)
	return e.result(c)["id"]
}

func (e *env) pageState(id string) PageState {
	e.t.Helper()
	var s PageState
	e.must(writer.Func(func(tx *writer.Tx) (err error) { s, err = pageState(tx, id); return err }))
	return s
}

func (e *env) docState(id string) DocState {
	e.t.Helper()
	var s DocState
	e.must(writer.Func(func(tx *writer.Tx) (err error) { s, err = docState(tx, id); return err }))
	return s
}

// commit sends a commit of steps made on base as tab, and returns its
// answer.
func (e *env) commit(p people.Person, page string, base int64, ref, steps string) CommitAnswer {
	e.t.Helper()
	ds := e.docState(page)
	c := &Commit{By: e.by(p), Session: "sid-" + p.ID, Tab: "tab-" + p.ID, Page: page, Epoch: ds.Epoch, Base: base, Ref: ref, Client: Client("tab-" + p.ID), Steps: json.RawMessage(steps)}
	op := writer.OnceResult("commit:"+page+":"+ref, c)
	if err := e.W.Do(context.Background(), op); err != nil {
		e.t.Fatalf("commit %s: %v", ref, err)
	}
	return CommitAnswer(op.Result()[0])
}

// head is a page's head document as JSON.
func (e *env) head(page string) string {
	e.t.Helper()
	var h Head
	e.View(e.t, func(tx *sql.Tx) (err error) { h, err = HeadOf(context.Background(), tx, page); return err })
	b, _ := doc.MarshalDoc(h.Doc)
	return string(b)
}

// insert is a step that inserts text at pos.
func insert(pos int, text string) string {
	b, _ := json.Marshal(text)
	return fmt.Sprintf(`{"stepType":"replace","from":%d,"to":%d,"slice":{"content":[{"type":"text","text":%s}]}}`, pos, pos, b)
}

func steps(s ...string) string {
	out := "["
	for i, x := range s {
		if i > 0 {
			out += ","
		}
		out += x
	}
	return out + "]"
}

func refusal(t *testing.T, err error) string {
	t.Helper()
	var r *writer.Refusal
	if !errors.As(err, &r) {
		t.Fatalf("want a refusal, got %v", err)
	}
	return r.Msg
}

func (e *env) row(query string, args ...any) string {
	e.t.Helper()
	var v sql.NullString
	if err := e.DB.R.QueryRow(query, args...).Scan(&v); err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
	return v.String
}

// doc builds a document of paragraphs.
func paras(texts ...string) json.RawMessage {
	type n = map[string]any
	var content []any
	for _, t := range texts {
		content = append(content, n{"type": "paragraph", "content": []any{n{"type": "text", "text": t}}})
	}
	b, _ := json.Marshal(n{"type": "doc", "content": content})
	d, err := doc.ParseDoc(b)
	if err != nil {
		return b
	}
	out, _ := doc.MarshalDoc(d)
	return out
}
