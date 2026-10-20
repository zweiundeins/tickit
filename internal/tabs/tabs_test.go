package tabs_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/dbtest"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
)

var (
	now = time.Unix(1_800_000_000, 0)
	ada = people.GuestOf("s1")
)

// note writes a row of prefs, touches "notes", and refuses when refuse
// is set, after its write; runs counts its runs.
type note struct {
	key    string
	refuse error
	runs   *int
}

func (c note) Apply(tx *writer.Tx) error {
	if c.runs != nil {
		*c.runs++
	}
	if _, err := tx.Exec(`INSERT INTO prefs (sid, data, seen) VALUES (?, '{}', 0)`, c.key); err != nil {
		return err
	}
	tx.Touch("notes")
	return c.refuse
}

// act runs cmd as op of tab t1 in session s1, as the web layer does.
func act(e *dbtest.Env, op string, cmd writer.Command) (tabs.Result, bool, error) {
	o := writer.OnceResult("s1:"+op, tabs.Act{Session: "s1", Tab: "t1", By: tabs.By{Actor: ada, Op: op, At: now}, Cmd: cmd})
	err := e.W.Do(context.Background(), o)
	return tabs.ParseResult(o.Result()), o.Replayed(), err
}

func state(t *testing.T, e *dbtest.Env) (s tabs.State, notes int) {
	t.Helper()
	e.View(t, func(tx *sql.Tx) error {
		var err error
		if s, err = tabs.Get(context.Background(), tx, "s1", "t1"); err != nil {
			return err
		}
		return tx.QueryRow(`SELECT count(*) FROM prefs`).Scan(&notes)
	})
	return s, notes
}

// MP-09: an accepted action records its op; the guest who acted gets a row.
func TestAccepted(t *testing.T) {
	e := dbtest.Open(t)
	res, replayed, err := act(e, "op1", note{key: "a"})
	if err != nil || !res.OK || res.Denied || replayed {
		t.Fatalf("an accepted action: %+v %t %v", res, replayed, err)
	}
	if got := e.Touched(); !slices.Equal(got, []string{"notes", "tab:t1"}) {
		t.Errorf("touched %v", got)
	}
	s, notes := state(t, e)
	if o, ok := s.Outcome("op1"); !ok || !o.OK || o.Msg != "" || o.At != now.UnixMilli() || notes != 1 {
		t.Errorf("the outcome: %+v, %d notes", s, notes)
	}
	e.View(t, func(tx *sql.Tx) error {
		p, err := people.Get(context.Background(), tx, ada.ID)
		if p.Kind != people.Guest || p.Name != ada.Name {
			t.Errorf("the guest's row: %+v", p)
		}
		var rows int
		if err := tx.QueryRow(`SELECT count(*) FROM people WHERE id = ?`, ada.ID).Scan(&rows); err != nil || rows != 1 {
			t.Errorf("%d rows of the guest: %v", rows, err)
		}
		return err
	})
}

// MP-09.2, MP-09.4: a refusal is recorded with its field, the command's
// changes are undone, and a retry of the op runs nothing again.
func TestRefused(t *testing.T) {
	e := dbtest.Open(t)
	runs := 0
	cmd := note{key: "a", refuse: tabs.Refuse("title", "A title has 1 to 255 characters."), runs: &runs}
	res, _, err := act(e, "op1", cmd)
	if err != nil || res.OK || res.Denied {
		t.Fatalf("a refused action: %+v %v", res, err)
	}
	if got := e.Touched(); !slices.Equal(got, []string{"tab:t1"}) {
		t.Errorf("a refusal touched %v", got)
	}
	s, notes := state(t, e)
	if o, _ := s.Outcome("op1"); o.OK || o.Msg != "A title has 1 to 255 characters." || o.Field != "title" || notes != 0 {
		t.Errorf("the refusal: %+v, %d notes", o, notes)
	}
	res, replayed, err := act(e, "op1", cmd)
	if err != nil || !replayed || res.OK || runs != 1 {
		t.Errorf("the retry: %+v, replayed %t, %d runs, %v", res, replayed, runs, err)
	}
	if got := e.Touched(); len(got) != 0 {
		t.Errorf("the retry touched %v", got)
	}
}

// dataRefusal is a refusal with data for the page, as tracker.Conflict.
type dataRefusal struct {
	writer.Refusal
}

func (r *dataRefusal) Error() string   { return r.Msg }
func (r *dataRefusal) Unwrap() error   { return &r.Refusal }
func (r *dataRefusal) OnField() string { return "priority" }
func (r *dataRefusal) OutcomeData() json.RawMessage {
	return json.RawMessage(`{"kind":"conflict","version":7}`)
}

// MP-08.3: a refusal's data reaches the outcome and the tab's signal.
func TestRefusalData(t *testing.T) {
	e := dbtest.Open(t)
	msg := "Sunny Otter changed the priority to High 5 seconds ago. Your Critical was not saved."
	if _, _, err := act(e, "op1", note{key: "a", refuse: &dataRefusal{writer.Refusal{Msg: msg}}}); err != nil {
		t.Fatal(err)
	}
	s, _ := state(t, e)
	if o, _ := s.Outcome("op1"); o.OK || o.Msg != msg || o.Field != "priority" || string(o.Data) != `{"kind":"conflict","version":7}` {
		t.Errorf("the outcome: %+v", o)
	}
	e.View(t, func(tx *sql.Tx) error {
		frame, err := tabs.View("s1", "t1").Render(context.Background(), tx)
		if err != nil {
			return err
		}
		if b := string(frame.([]byte)); !strings.Contains(b, `"field":"priority","data":{"kind":"conflict","version":7}}`) {
			t.Errorf("the tab's frame: %s", b)
		}
		return nil
	})
}

// NFR-01.1: a refusal for want of a right is Denied: the action answers 403.
func TestDenied(t *testing.T) {
	e := dbtest.Open(t)
	res, _, err := act(e, "op1", note{key: "a", refuse: &access.Denial{Refusal: writer.Refusal{Msg: "Only admins of this project can delete issues."}}})
	if err != nil || res.OK || !res.Denied {
		t.Fatalf("a denial: %+v %v", res, err)
	}
	if s, _ := state(t, e); s.Outcomes[0].Msg != "Only admins of this project can delete issues." {
		t.Errorf("the denial's outcome: %+v", s.Outcomes)
	}
}

// Two quick actions in one tab both settle; the row keeps the newest 16.
func TestOutcomes(t *testing.T) {
	e := dbtest.Open(t)
	for i := range 20 {
		if _, _, err := act(e, fmt.Sprint("op", i), note{key: fmt.Sprint(i)}); err != nil {
			t.Fatal(err)
		}
	}
	s, _ := state(t, e)
	if len(s.Outcomes) != tabs.MaxOutcomes || s.Outcomes[0].Op != "op4" || s.Outcomes[15].Op != "op19" {
		t.Errorf("outcomes: %+v", s.Outcomes)
	}
	// The tab's view brings both of the last two.
	e.View(t, func(tx *sql.Tx) error {
		frame, err := tabs.View("s1", "t1").Render(context.Background(), tx)
		if err != nil {
			return err
		}
		b := string(frame.([]byte))
		if !strings.Contains(b, `"op18":{"ok":true,"msg":"","field":""}`) || !strings.Contains(b, `"op19":{"ok":true`) || strings.Contains(b, `"op3"`) {
			t.Errorf("the tab's frame: %s", b)
		}
		return nil
	})
}

// An error that is no refusal leaves no outcome and no record: a retry runs.
func TestFailed(t *testing.T) {
	e := dbtest.Open(t)
	boom := errors.New("boom")
	if _, _, err := act(e, "op1", note{key: "a", refuse: boom}); !errors.Is(err, boom) {
		t.Fatalf("a failure: %v", err)
	}
	if s, notes := state(t, e); len(s.Outcomes) != 0 || notes != 0 {
		t.Errorf("after a failure: %+v, %d notes", s, notes)
	}
	if res, replayed, err := act(e, "op1", note{key: "a"}); err != nil || replayed || !res.OK {
		t.Errorf("the retry: %+v %t %v", res, replayed, err)
	}
	for _, bad := range []tabs.Act{
		{Session: "s1", Tab: "t1", By: tabs.By{Actor: ada, At: now}, Cmd: note{}},
		{Session: "s1", Tab: "t1", By: tabs.By{Op: "op", At: now}, Cmd: note{}},
		{Session: "s1", By: tabs.By{Actor: ada, Op: "op", At: now}, Cmd: note{}},
		{Session: "s1", Tab: "t1", By: tabs.By{Actor: ada, Op: "op", At: now}},
	} {
		if err := e.W.Do(context.Background(), bad); !errors.Is(err, writer.ErrInvalid) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

// An area keeps its own state in the tab's row.
func TestAreas(t *testing.T) {
	e := dbtest.Open(t)
	type board struct {
		Collapsed []string `json:"collapsed"`
	}
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		return tabs.Update(tx, "s1", "t1", "board", now, func(b *board) error {
			b.Collapsed = append(b.Collapsed, "done")
			return nil
		})
	}))
	s, _ := state(t, e)
	b, err := tabs.Load[board](s, "board")
	if err != nil || !slices.Equal(b.Collapsed, []string{"done"}) {
		t.Errorf("the board's state: %+v %v", b, err)
	}
	if none, err := tabs.Load[board](s, "list"); err != nil || none.Collapsed != nil {
		t.Errorf("an area without state: %+v %v", none, err)
	}
}

// A tab's watched views survive in its row, at most MaxWatched of them.
func TestWatched(t *testing.T) {
	e := dbtest.Open(t)
	var dropped []string
	for i := range tabs.MaxWatched + 1 {
		v := tabs.Watched{View: fmt.Sprint("issue:", i), Key: fmt.Sprint("issue:", i)}
		d, err := e.W.DoValue(context.Background(), func(tx *writer.Tx) ([]string, error) { return tabs.AddWatched(tx, "s1", "t1", v, now) })
		if err != nil {
			t.Fatal(err)
		}
		dropped = append(dropped, d...)
	}
	e.Do(t, writer.Func(func(tx *writer.Tx) error { return tabs.RemoveWatched(tx, "s1", "t1", "issue:3", now) }))
	s, _ := state(t, e)
	if !slices.Equal(dropped, []string{"issue:0"}) || len(s.Watched) != tabs.MaxWatched-1 || s.Watched[0].Key != "issue:1" {
		t.Errorf("dropped %v, watched %+v", dropped, s.Watched)
	}
}

// The sweep forgets tabs not seen for 3 days (requirements 4.1).
func TestSweep(t *testing.T) {
	e := dbtest.Open(t)
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		if err := tabs.Save(tx, "s1", "gone", tabs.State{}, now.Add(-tabs.TTL-time.Minute)); err != nil {
			return err
		}
		return tabs.Save(tx, "s1", "here", tabs.State{}, now.Add(-tabs.TTL+time.Minute))
	}))
	e.Do(t, tabs.Sweep{At: now})
	e.View(t, func(tx *sql.Tx) error {
		var left string
		if err := tx.QueryRow(`SELECT group_concat(tab_id) FROM tab_state`).Scan(&left); err != nil {
			return err
		}
		if left != "here" {
			t.Errorf("after the sweep: %s", left)
		}
		return nil
	})
}
