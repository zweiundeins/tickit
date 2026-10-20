package dashboards_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/dashboards"
	"github.com/zweiundeins/tickit/internal/dbtest"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
)

var n int

func by(p people.Person) tabs.By {
	n++
	return tabs.By{Actor: p, Op: fmt.Sprint("op", n), At: time.Now()}
}

func guest(id string) people.Person { return people.Person{ID: id, Kind: people.Guest, Name: id} }

// FR-60: a dashboard's owner adds, moves and removes gadgets, renames and
// deletes it; anyone else is refused with a denial (403); names and
// counts have their limits; every change touches the dashboard and the
// list.
func TestDashboardCommands(t *testing.T) {
	e := dbtest.Open(t)
	ctx := context.Background()
	ada, bo := guest("g-aaaaaaaaaaaaaaaaaaaa"), guest("g-bbbbbbbbbbbbbbbbbbbb")
	e.Do(t, people.SeedPeople{At: time.Now()})
	var pid string
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		b, err := tracker.CreateProject{By: by(people.Seeded[0]), Key: "WEB", Name: "Web", Open: true}.ApplyResult(tx)
		var m map[string]string
		json.Unmarshal(b, &m)
		pid = m["id"]
		return err
	}))
	var id string
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		b, err := dashboards.Create{By: by(ada), Name: "  Release  "}.ApplyResult(tx)
		var m map[string]string
		json.Unmarshal(b, &m)
		id = m["id"]
		return err
	}))
	if got := e.Touched(); !slices.Contains(got, dashboards.ListTopic) || !slices.Contains(got, dashboards.Topic(id)) {
		t.Errorf("create touched %v", got)
	}
	do := func(cmd writer.Command) error { return e.W.Do(ctx, cmd) }
	for _, k := range []dashboards.Kind{dashboards.Status, dashboards.Assigned, dashboards.Activity} {
		if err := do(dashboards.AddGadget{By: by(ada), Dashboard: id, Kind: k, Project: pid}); err != nil {
			t.Fatalf("add %s: %v", k, err)
		}
	}
	refused := func(err error, msg string) {
		t.Helper()
		var r *writer.Refusal
		if !errors.As(err, &r) || r.Msg != msg {
			t.Errorf("got %v, want the refusal %q", err, msg)
		}
	}
	refused(do(dashboards.AddGadget{By: by(ada), Dashboard: id, Kind: dashboards.Created}), "Pick a project.")
	refused(do(dashboards.AddGadget{By: by(ada), Dashboard: id, Kind: dashboards.Filter, SavedFilter: tracker.NewID()}), "Pick a saved filter.")
	refused(do(dashboards.AddGadget{By: by(ada), Dashboard: id, Kind: "pie"}), "Pick a kind of gadget.")
	refused(do(dashboards.Rename{By: by(ada), Dashboard: id, Name: ""}), "A dashboard's name has 1 to 60 characters.")
	for _, cmd := range []writer.Command{
		dashboards.Rename{By: by(bo), Dashboard: id, Name: "Bo's"},
		dashboards.AddGadget{By: by(bo), Dashboard: id, Kind: dashboards.Activity},
		dashboards.Delete{By: by(bo), Dashboard: id},
	} {
		err := do(cmd)
		var d interface{ Denied() bool }
		if !errors.As(err, &d) || !d.Denied() {
			t.Errorf("%T by someone else: %v, want a denial", cmd, err)
		}
	}
	d, err := dashboards.Get(ctx, e.DB.R, id)
	if err != nil || d.Name != "Release" || len(d.Gadgets) != 3 {
		t.Fatalf("%+v %v", d, err)
	}
	if err := do(dashboards.MoveGadget{By: by(ada), Dashboard: id, Gadget: d.Gadgets[2].ID, Earlier: true}); err != nil {
		t.Fatal(err)
	}
	if err := do(dashboards.MoveGadget{By: by(ada), Dashboard: id, Gadget: d.Gadgets[0].ID, Earlier: true}); err != nil { // the first stays
		t.Fatal(err)
	}
	moved, _ := dashboards.Get(ctx, e.DB.R, id)
	if kinds := []dashboards.Kind{moved.Gadgets[0].Kind, moved.Gadgets[1].Kind, moved.Gadgets[2].Kind}; !slices.Equal(kinds, []dashboards.Kind{dashboards.Status, dashboards.Activity, dashboards.Assigned}) {
		t.Errorf("after the move: %v", kinds)
	}
	if err := do(dashboards.RemoveGadget{By: by(ada), Dashboard: id, Gadget: moved.Gadgets[1].ID}); err != nil {
		t.Fatal(err)
	}
	for range dashboards.MaxGadgets - 2 {
		if err := do(dashboards.AddGadget{By: by(ada), Dashboard: id, Kind: dashboards.Activity}); err != nil {
			t.Fatal(err)
		}
	}
	refused(do(dashboards.AddGadget{By: by(ada), Dashboard: id, Kind: dashboards.Activity}), "A dashboard holds 12 gadgets. Remove one first.")
	if err := do(dashboards.Delete{By: by(ada), Dashboard: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := dashboards.Get(ctx, e.DB.R, id); err == nil {
		t.Errorf("the deleted dashboard is still there")
	}
	var left int
	e.DB.R.QueryRow(`SELECT count(*) FROM dashboard_gadgets`).Scan(&left)
	if left != 0 {
		t.Errorf("%d gadgets left", left)
	}
}
