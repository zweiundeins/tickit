package web_test

import (
	"context"
	"database/sql"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

var dashGoto = regexp.MustCompile(`"_goto":"/dashboards/([0-9a-f-]{36})"`)

// FR-60, FR-137.1: a person creates a dashboard from /dashboards and adds
// gadgets: a saved filter's issues, counts by status as a chart with its
// numbers in a table, issues assigned to the viewer, the activity, and
// created against resolved per day. Everyone sees it, its owner alone
// changes it, and its gadgets follow the issues behind them live, one
// render a tick for every tab on it.
func TestDashboards(t *testing.T) {
	e := newList(t)
	e.Account(t, "owner@example.com", "a long password 1", people.NoRole)
	owner := webtest.Client(t)
	e.Login(t, owner, "owner@example.com", "a long password 1")
	if code, page := webtest.Get(t, owner, e.URL+"/dashboards"); code != http.StatusOK || !strings.Contains(page, "No dashboards yet") {
		t.Fatalf("/dashboards: %d", code)
	}
	code, body := act(t, e.Env, owner, "/cmd/dashboards/create", "tab1", map[string]any{"name": "Release"})
	m := dashGoto.FindStringSubmatch(body)
	if code != http.StatusOK || m == nil {
		t.Fatalf("create: %d %s", code, body)
	}
	id := m[1]
	filter := e.result(tracker.SaveFilter{By: e.by(e.ada), Name: "UX work", Filter: tracker.Filter{Labels: []string{"ux"}}})
	for _, g := range []map[string]any{
		{"kind": "status", "project": e.web}, {"kind": "created", "project": e.web}, {"kind": "filter", "filter": filter},
		{"kind": "assigned"}, {"kind": "activity"},
	} {
		g["dashboard"] = id
		if code, _ := act(t, e.Env, owner, "/cmd/dashboards/gadget", "tab1", g); code != http.StatusNoContent {
			t.Fatalf("add %v: %d", g, code)
		}
	}
	// A gadget about a project needs one.
	act(t, e.Env, owner, "/cmd/dashboards/gadget", "tab1", map[string]any{"dashboard": id, "kind": "status"})

	guest := webtest.Client(t)
	code, page := webtest.Get(t, guest, e.URL+"/dashboards/"+id)
	if code != http.StatusOK {
		t.Fatalf("the dashboard: %d", code)
	}
	for _, want := range []string{
		"<h1>Release</h1>", "Website by status", `<table class="tk-bars">`, `<span class="tk-lozenge" data-cat="progress">`,
		"Website: created and resolved", "Show as a table", `role="img" aria-label="Issues created and resolved per day`,
		"UX work", "Fix the login", "Each person sees their own.", `id="activity-gadget-`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the dashboard has no %q", want)
		}
	}
	if strings.Contains(page, `id="dash-kind"`) {
		t.Errorf("a guest is offered the owner's form")
	}
	if !strings.Contains(page, `.tk-dash [data-owner="g-`) && !strings.Contains(page, `[data-owner="`) {
		t.Errorf("no owner style")
	}
	_, mine := webtest.Get(t, owner, e.URL+"/dashboards/"+id)
	if !strings.Contains(mine, `id="dash-kind"`) || !strings.Contains(mine, `.tk-gadget__own{display:inline-flex}`) {
		t.Errorf("the owner is not offered the add form and the gadgets' controls")
	}
	_, list := webtest.Get(t, guest, e.URL+"/dashboards")
	if !strings.Contains(list, ">Release</a>") {
		t.Errorf("the list has no Release")
	}

	// Only the owner changes it.
	if code, _ := act(t, e.Env, guest, "/cmd/dashboards/rename", "tab2", map[string]any{"dashboard": id, "name": "Mine now"}); code != http.StatusForbidden {
		t.Errorf("a guest's rename: %d, want 403", code)
	}

	// Live, and one render a tick for all who read the same.
	streams := make([]<-chan string, 3)
	for i := range streams {
		c := webtest.Client(t)
		streams[i] = webtest.Listen(t, c, e.URL+"/stream?tab=d"+string(rune('a'+i))+"&v=dashboard:"+id, nil)
		webtest.Next(t, streams[i], `id="dashboard-grid"`)
	}
	key := ""
	for k := range e.Hub.Stats().RendersByKey {
		if strings.HasPrefix(k, "dashboard:"+id+":") {
			key = k
		}
	}
	before := e.Hub.Stats().RendersByKey[key]
	e.Do(t, tracker.SetStatus{By: e.by(e.ada), Issue: e.ids["WEB-1"], Seen: 1 << 40, Status: tracker.StatusDone})
	for _, ev := range streams {
		frame := webtest.Next(t, ev, `id="dashboard-grid"`)
		if strings.Contains(frame, "Fix the login") && !strings.Contains(frame, "Done") {
			t.Errorf("the frame does not show the issue done")
		}
	}
	if got := e.Hub.Stats().RendersByKey[key] - before; got != 1 {
		t.Errorf("%d renders of %s for one change and three tabs", got, key)
	}

	// The viewer's own issues, in a view of their own.
	me := webtest.Listen(t, owner, e.URL+"/stream?tab=dme&v=dashme:"+id, nil)
	if frame := webtest.Next(t, me, `id="gme-`); !strings.Contains(frame, "Nothing is assigned to you.") {
		t.Errorf("the owner's assigned gadget: %.200s", frame)
	}
	if code, _ := act(t, e.Env, owner, "/cmd/issues/"+e.ids["WEB-2"]+"/assignee", "tab1", map[string]any{"value": "me", "seen": 1 << 40}); code != http.StatusNoContent {
		t.Fatalf("assign: %d", code)
	}
	webtest.Next(t, me, "Cart 100% broken")

	// The owner moves, removes, renames and deletes.
	gadgets := regexp.MustCompile(`data-act="gremove" data-g="([0-9a-f-]{36})"`).FindAllStringSubmatch(mine, -1)
	if len(gadgets) != 5 {
		t.Fatalf("%d gadgets", len(gadgets))
	}
	if code, _ := act(t, e.Env, owner, "/cmd/dashboards/gadget/move", "tab1", map[string]any{"dashboard": id, "gadget": gadgets[1][1], "earlier": true}); code != http.StatusNoContent {
		t.Errorf("move: %d", code)
	}
	if code, _ := act(t, e.Env, owner, "/cmd/dashboards/gadget/remove", "tab1", map[string]any{"dashboard": id, "gadget": gadgets[4][1]}); code != http.StatusNoContent {
		t.Errorf("remove: %d", code)
	}
	if code, _ := act(t, e.Env, owner, "/cmd/dashboards/rename", "tab1", map[string]any{"dashboard": id, "name": "Launch"}); code != http.StatusNoContent {
		t.Errorf("rename: %d", code)
	}
	frame := webtest.Next(t, streams[0], "<h1>Launch</h1>")
	if strings.Index(frame, "created and resolved") > strings.Index(frame, "by status") || strings.Contains(frame, `id="activity-gadget-`) {
		t.Errorf("the grid does not follow the move and the removal")
	}
	if code, body := act(t, e.Env, owner, "/cmd/dashboards/delete", "tab1", map[string]any{"dashboard": id}); code != http.StatusOK || !strings.Contains(body, `"_goto":"/dashboards"`) {
		t.Errorf("delete: %d %s", code, body)
	}
	webtest.Next(t, streams[0], "This dashboard was deleted")
	if code, _ := webtest.Get(t, guest, e.URL+"/dashboards/"+id); code != http.StatusNotFound {
		t.Errorf("a deleted dashboard: %d, want 404", code)
	}
}

// FR-60, NFR-38: the counts of a dashboard's gadgets are cached against
// the hub's version of their project's board: no change, no count; a
// change to an issue of the project counts again.
func TestDashboardCountsCached(t *testing.T) {
	e := newList(t)
	ctx := context.Background()
	get := func() ([]tracker.StatusCount, int64) {
		t.Helper()
		seq := e.Hub.Seq()
		var list []tracker.StatusCount
		var reads int64
		err := e.DB.View(ctx, func(tx *sql.Tx) (err error) {
			list, reads, err = web.DashboardStatus(e.Server, ctx, tx, e.web, seq)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return list, reads
	}
	first, n0 := get()
	_, n1 := get()
	if n1 != n0 {
		t.Errorf("counted again without a change: %d then %d", n0, n1)
	}
	count := func(list []tracker.StatusCount, status string) int {
		for _, c := range list {
			if c.Status == status {
				return c.Count
			}
		}
		return -1
	}
	if count(first, tracker.StatusInReview) != 1 {
		t.Errorf("in review: %v", first)
	}
	e.Do(t, tracker.SetStatus{By: e.by(e.ada), Issue: e.ids["WEB-1"], Seen: 1 << 40, Status: tracker.StatusInReview})
	if err := e.W.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		list, n2 := get()
		if n2 > n1 && count(list, tracker.StatusInReview) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not counted again after a change: %v", list)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Another project's change keeps WEB's count.
	_, n3 := get()
	e.Do(t, tracker.SetStatus{By: e.by(e.ada), Issue: e.ids["APP-1"], Seen: 1 << 40, Status: tracker.StatusDone})
	time.Sleep(50 * time.Millisecond)
	if _, n4 := get(); n4 != n3 {
		t.Errorf("APP's change counted WEB again")
	}
}
