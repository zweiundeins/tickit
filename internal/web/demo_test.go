package web_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/zweiundeins/tickit/internal/demo"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// seededEnv is a server with the demo's seed.
func seededEnv(t *testing.T) *webtest.Env {
	t.Helper()
	e := webtest.Start(t)
	if _, err := demo.Seed(context.Background(), e.W, e.Server.Demo().Content(), time.Now(), 1); err != nil {
		t.Fatal(err)
	}
	return e
}

func rowCount(t *testing.T, e *webtest.Env, q string, args ...any) int {
	t.Helper()
	var n int
	if err := e.DB.R.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

// outcome is what the tab's row says of an op.
func outcome(t *testing.T, e *webtest.Env, p people.Person, op string) tabs.Outcome {
	t.Helper()
	var st tabs.State
	err := e.DB.View(context.Background(), func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT sid FROM tab_state`)
		if err != nil {
			return err
		}
		var sids []string
		for rows.Next() {
			var sid string
			rows.Scan(&sid)
			sids = append(sids, sid)
		}
		rows.Close()
		for _, sid := range sids {
			if people.GuestID(sid) != p.ID && p.Kind == people.Guest {
				continue
			}
			s, err := tabs.Get(context.Background(), tx, sid, "tab1")
			if err != nil {
				return err
			}
			for _, o := range s.Outcomes {
				if o.Op == op {
					st = s
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range st.Outcomes {
		if o.Op == op {
			return o
		}
	}
	t.Fatalf("no outcome of %s", op)
	return tabs.Outcome{}
}

// guestIssue has a guest create an issue in WEB, as the create action
// would, and returns its id.
func guestIssue(t *testing.T, e *webtest.Env, g people.Person, summary string) string {
	t.Helper()
	var project string
	if err := e.DB.R.QueryRow(`SELECT id FROM projects WHERE key = 'WEB'`).Scan(&project); err != nil {
		t.Fatal(err)
	}
	id := tracker.NewID()
	by := tabs.By{Actor: g, Op: "create-" + id[:8], At: time.Now()}
	e.Do(t, tabs.Act{Session: "s-" + g.ID, Tab: "tabx", By: by, Cmd: tracker.CreateIssue{By: by, ID: id, Project: project, Type: tracker.Task, Summary: summary}})
	return id
}

// NFR-01.1: every admin and moderation page and action without the right
// answers 403, and the actions change nothing.
func TestStaffOnly(t *testing.T) {
	e := seededEnv(t)
	e.Account(t, "mo@example.com", "correct horse battery", people.Moderator)
	admin := e.Account(t, "ada@example.com", "correct horse battery", people.Admin)
	g, mo := webtest.Client(t), webtest.Client(t)
	webtest.Get(t, g, e.URL+"/")
	e.Login(t, mo, "mo@example.com", "correct horse battery")
	issue := guestIssue(t, e, people.GuestOf("session-visitor"), "Visit my shop")
	var comment string
	e.DB.R.QueryRow(`SELECT id FROM comments LIMIT 1`).Scan(&comment)
	type try struct {
		who  string
		c    *http.Client
		path string
		body string
	}
	var tries []try
	for _, who := range []struct {
		name string
		c    *http.Client
	}{{"a guest", g}, {"a moderator", mo}} {
		for _, a := range []struct{ path, body string }{
			{"/cmd/admin/create", `"email":"x@example.com","username":"xx","name":"X","role":"admin","password":"correct horse battery"`},
			{"/cmd/admin/role", `"account":"` + admin + `","role":"none"`},
			{"/cmd/admin/active", `"account":"` + admin + `","active":false`},
			{"/cmd/admin/reset", ``},
		} {
			tries = append(tries, try{who.name, who.c, a.path, a.body})
		}
	}
	for _, a := range []struct{ path, body string }{
		{"/cmd/moderation/act", `"action":"hide","kind":"issue","id":"` + issue + `"`},
		{"/cmd/moderation/act", `"action":"delete","kind":"comment","id":"` + comment + `"`},
		{"/cmd/moderation/block", `"person":"` + people.GuestOf("session-visitor").ID + `"`},
		{"/cmd/moderation/unblock", `"person":"` + people.GuestOf("session-visitor").ID + `"`},
	} {
		tries = append(tries, try{"a guest", g, a.path, a.body})
	}
	for i, tr := range tries {
		body := fmt.Sprintf(`{"tab":"tab1","op":"op%d"`, i)
		if tr.body != "" {
			body += "," + tr.body
		}
		if code, resp := webtest.Post(t, tr.c, e.URL+tr.path, body+"}"); code != http.StatusForbidden {
			t.Errorf("%s posts %s: %d %s", tr.who, tr.path, code, resp)
		}
	}
	time.Sleep(50 * time.Millisecond) // a reset that would have started
	// A refused action records its outcome in the tab's row and nothing else.
	if rowCount(t, e, `SELECT count(*) FROM issues WHERE deleted_at IS NOT NULL`) != 0 || rowCount(t, e, `SELECT count(*) FROM blocks`) != 0 ||
		rowCount(t, e, `SELECT count(*) FROM accounts`) != 2 || rowCount(t, e, `SELECT count(*) FROM accounts WHERE status = 'active' AND site_role = 'admin'`) != 1 ||
		rowCount(t, e, `SELECT count(*) FROM demo_resets`) != 0 || rowCount(t, e, `SELECT count(*) FROM moderation_log`) != 0 {
		t.Error("a refused action changed something")
	}
	for _, path := range []string{"/admin", "/admin/guests", "/admin/stats", "/admin/security", "/admin/demo"} {
		for name, c := range map[string]*http.Client{"a guest": g, "a moderator": mo} {
			if code, _ := webtest.Get(t, c, e.URL+path); code != http.StatusForbidden {
				t.Errorf("%s gets %s: %d", name, path, code)
			}
		}
	}
	if code, _ := webtest.Get(t, g, e.URL+"/moderation"); code != http.StatusForbidden {
		t.Errorf("a guest gets /moderation: %d", code)
	}
	if code, _ := webtest.Get(t, mo, e.URL+"/moderation"); code != http.StatusOK {
		t.Errorf("a moderator gets /moderation: %d", code)
	}
	if code, _ := webtest.Get(t, g, e.URL+"/stream?tab=t2&v=moderation:"); code != http.StatusForbidden {
		t.Errorf("a guest watches moderation: %d", code)
	}
	if code, _ := webtest.Get(t, mo, e.URL+"/stream?tab=t3&v=admin-stats:"); code != http.StatusForbidden {
		t.Errorf("a moderator watches the admin's statistics: %d", code)
	}
}

// DM-03.1, DM-03.3, DM-03.4: "Reset now" deletes what visitors wrote,
// seeds again and keeps the staff; while it runs, actions answer 503 and
// pages say so; after it, open streams render again.
func TestResetNow(t *testing.T) {
	e := seededEnv(t)
	e.Account(t, "ada@example.com", "correct horse battery", people.Admin)
	admin, g := webtest.Client(t), webtest.Client(t)
	e.Login(t, admin, "ada@example.com", "correct horse battery")
	me := guest(t, e, g)
	if code, _ := webtest.Post(t, g, e.URL+"/cmd/people/rename", `{"tab":"tab1","op":"op1","name":"Visitor"}`); code != http.StatusNoContent {
		t.Fatalf("rename: %d", code)
	}
	guestIssue(t, e, me, "Spam spam spam")
	stream := webtest.Listen(t, g, e.URL+"/stream?tab=tab1", nil)
	webtest.Next(t, stream, `id="demo-notice"`)

	// DM-03.4: actions answer 503 while a reset holds the demo.
	release := e.Server.Demo().Hold()
	webtest.Next(t, stream, demo.MsgResetting)
	code, body := webtest.Post(t, g, e.URL+"/cmd/people/rename", `{"tab":"tab1","op":"op2","name":"Later"}`)
	if code != http.StatusServiceUnavailable || !strings.Contains(body, demo.MsgResetting) {
		t.Errorf("an action during a reset: %d %q", code, body)
	}
	if _, page := webtest.Get(t, g, e.URL+"/privacy"); !strings.Contains(page, demo.MsgResetting) {
		t.Error("a page during a reset does not say so")
	}
	release()

	if code, body := webtest.Post(t, admin, e.URL+"/cmd/admin/reset", `{"tab":"tab1","op":"r1"}`); code != http.StatusNoContent {
		t.Fatalf("reset now: %d %s", code, body)
	}
	deadline := time.Now().Add(10 * time.Second)
	for rowCount(t, e, `SELECT count(*) FROM demo_resets WHERE outcome = 'ok'`) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no reset in 10 s")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n := rowCount(t, e, `SELECT count(*) FROM people WHERE kind = 'guest'`); n != 0 {
		t.Errorf("%d guests after the reset", n)
	}
	if n := rowCount(t, e, `SELECT count(*) FROM issues WHERE summary = 'Spam spam spam'`); n != 0 {
		t.Error("the guest's issue survived")
	}
	if n := rowCount(t, e, `SELECT count(*) FROM issues`); n != len(e.Server.Demo().Content().Issues) {
		t.Errorf("%d issues after the reset", n)
	}
	if code, _ := webtest.Get(t, admin, e.URL+"/admin/demo"); code != http.StatusOK {
		t.Errorf("the admin is logged out after the reset: %d", code)
	}
	// The stream renders again: the warning goes, the guest's name is the
	// one its id draws again.
	webtest.Next(t, stream, `<div id="demo-alert" class="tk-demo-alert"></div>`)
	if n := rowCount(t, e, `SELECT count(*) FROM security_log WHERE kind = ?`, demo.EventReset); n != 1 {
		t.Errorf("%d resets in the security log", n)
	}
}

// The alerts read tickit_resets_total for both outcomes from 0 on.
func TestResetMetrics(t *testing.T) {
	e := webtest.Start(t)
	reg := prometheus.NewRegistry()
	reg.MustRegister(e.Server.Collectors()...)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, mf := range mfs {
		if mf.GetName() != "tickit_resets_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "outcome" && m.GetCounter().GetValue() == 0 {
					got[l.GetValue()] = true
				}
			}
		}
	}
	if !got["ok"] || !got["failed"] {
		t.Errorf("tickit_resets_total: %v", got)
	}
}

// DM-07.1, DM-07.3, DM-07.4: a moderator hides a visitor's issue from the
// moderation page; it leaves the lists, shows as hidden on the page in
// the next frame, and the log says who did it and why. Restoring brings it
// back.
func TestModerateLive(t *testing.T) {
	e := seededEnv(t)
	e.Account(t, "mo@example.com", "correct horse battery", people.Moderator)
	mo := webtest.Client(t)
	e.Login(t, mo, "mo@example.com", "correct horse battery")
	visitor := people.GuestOf("session-visitor")
	id := guestIssue(t, e, visitor, "Cheap watches here")
	code, page := webtest.Get(t, mo, e.URL+"/moderation")
	if code != http.StatusOK || !strings.Contains(page, "Cheap watches here") || !strings.Contains(page, `data-id="`+id+`"`) {
		t.Fatalf("the moderation page: %d", code)
	}
	stream := webtest.Listen(t, mo, e.URL+"/stream?tab=tab1&v=moderation:", nil)
	webtest.Next(t, stream, "Cheap watches here")
	if code, body := webtest.Post(t, mo, e.URL+"/cmd/moderation/act", `{"tab":"tab1","op":"m1","action":"hide","kind":"issue","id":"`+id+`","reason":"spam"}`); code != http.StatusNoContent {
		t.Fatalf("hide: %d %s", code, body)
	}
	ev := webtest.Next(t, stream, "tk-admin__status--warn")
	if !strings.Contains(ev, "Restore") || !strings.Contains(ev, "spam") {
		t.Errorf("the hidden issue's frame: %s", ev)
	}
	if n := rowCount(t, e, `SELECT count(*) FROM issues WHERE id = ? AND deleted_at IS NOT NULL`, id); n != 1 {
		t.Error("the hidden issue is still in the lists")
	}
	if n := rowCount(t, e, `SELECT count(*) FROM moderation_log WHERE action = 'hide' AND reason = 'spam' AND target_id = ?`, id); n != 1 {
		t.Error("no entry in the moderation log")
	}
	if n := rowCount(t, e, `SELECT count(*) FROM security_log WHERE kind = 'moderation'`); n != 1 {
		t.Error("no entry in the security log")
	}
	// The moderator still opens the hidden issue, with who hid it and why;
	// everyone else gets 410.
	var key string
	e.DB.R.QueryRow(`SELECT key FROM issues WHERE id = ?`, id).Scan(&key)
	if code, page := webtest.Get(t, mo, e.URL+"/"+key); code != http.StatusOK || !strings.Contains(page, "This issue is hidden") || !strings.Contains(page, ": spam. Only moderators") {
		t.Errorf("the moderator's page of the hidden issue: %d", code)
	}
	if code, _ := webtest.Get(t, webtest.Client(t), e.URL+"/"+key); code != http.StatusGone {
		t.Errorf("a guest's page of the hidden issue: %d, want 410", code)
	}
	if code, _ := webtest.Post(t, mo, e.URL+"/cmd/moderation/act", `{"tab":"tab1","op":"m2","action":"restore","kind":"issue","id":"`+id+`"}`); code != http.StatusNoContent {
		t.Fatalf("restore: %d", code)
	}
	if n := rowCount(t, e, `SELECT count(*) FROM issues WHERE id = ? AND deleted_at IS NULL`, id); n != 1 {
		t.Error("the restored issue is not back")
	}
	if code, _ := webtest.Post(t, mo, e.URL+"/cmd/moderation/act", `{"tab":"tab1","op":"m3","action":"nuke","kind":"issue","id":"`+id+`"}`); code != http.StatusBadRequest {
		t.Errorf("an unknown action: %d", code)
	}
}

// DM-07.2: a blocked guest still reads, and every change answers
// "Changes from this browser are paused.", also from another session at
// the same address; staff there are not blocked. Unblocking lifts it.
func TestBlockedGuest(t *testing.T) {
	e := seededEnv(t)
	e.Account(t, "mo@example.com", "correct horse battery", people.Moderator)
	mo, g := webtest.Client(t), webtest.Client(t)
	e.Login(t, mo, "mo@example.com", "correct horse battery")
	me := guest(t, e, g)
	if code, _ := webtest.Post(t, g, e.URL+"/cmd/people/rename", `{"tab":"tab1","op":"g1","name":"Spammer"}`); code != http.StatusNoContent {
		t.Fatalf("rename: %d", code)
	}
	if code, body := webtest.Post(t, mo, e.URL+"/cmd/moderation/block", `{"tab":"tab1","op":"b1","person":"`+me.ID+`","reason":"spam"}`); code != http.StatusNoContent {
		t.Fatalf("block: %d %s", code, body)
	}
	if code, _ := webtest.Post(t, g, e.URL+"/cmd/people/rename", `{"tab":"tab1","op":"g2","name":"Again"}`); code != http.StatusForbidden {
		t.Errorf("a blocked guest's change: %d", code)
	}
	if o := outcome(t, e, me, "g2"); o.OK || o.Msg != demo.MsgBlocked {
		t.Errorf("the outcome: %+v", o)
	}
	if code, _ := webtest.Get(t, g, e.URL+"/"); code != http.StatusOK {
		t.Errorf("a blocked guest reads: %d", code)
	}
	other := webtest.Client(t)
	webtest.Get(t, other, e.URL+"/")
	if code, _ := webtest.Post(t, other, e.URL+"/cmd/people/rename", `{"tab":"tab1","op":"o1","name":"Fresh"}`); code != http.StatusForbidden {
		t.Errorf("another session from the blocked address: %d", code)
	}
	if code, _ := webtest.Post(t, mo, e.URL+"/cmd/people/rename", `{"tab":"tab1","op":"m1","name":"Mo"}`); code != http.StatusNoContent {
		t.Errorf("staff at the blocked address: %d", code)
	}
	if code, _ := webtest.Post(t, mo, e.URL+"/cmd/moderation/unblock", `{"tab":"tab1","op":"b2","person":"`+me.ID+`"}`); code != http.StatusNoContent {
		t.Fatalf("unblock: %d", code)
	}
	if code, _ := webtest.Post(t, g, e.URL+"/cmd/people/rename", `{"tab":"tab1","op":"g3","name":"Sorry"}`); code != http.StatusNoContent {
		t.Errorf("after the unblock: %d", code)
	}
}

// FR-141, FR-125, FR-142.2, FR-143: an admin creates an account, whose
// first password must be changed before anything else; the last admin
// keeps their role; a deactivated account's sessions end.
func TestAccounts(t *testing.T) {
	e := webtest.Start(t)
	ada := e.Account(t, "ada@example.com", "correct horse battery", people.Admin)
	admin := webtest.Client(t)
	e.Login(t, admin, "ada@example.com", "correct horse battery")
	if code, body := webtest.Post(t, admin, e.URL+"/cmd/admin/create", `{"tab":"tab1","op":"c1","email":"bea@example.com","username":"bea","name":"Bea Moderator","role":"moderator","password":"first password"}`); code != http.StatusNoContent {
		t.Fatalf("create: %d %s", code, body)
	}
	if o := outcome(t, e, people.Person{ID: ada, Kind: people.Account}, "c1"); !o.OK {
		t.Fatalf("create: %+v", o)
	}
	bea := webtest.Client(t)
	e.Login(t, bea, "bea@example.com", "first password")
	if code, _ := webtest.Get(t, bea, e.URL+"/"); code != http.StatusSeeOther {
		t.Errorf("the first page with a first password: %d", code)
	}
	if code, _ := webtest.Post(t, bea, e.URL+"/cmd/people/rename", `{"tab":"tab1","op":"b1","name":"Bea"}`); code != http.StatusForbidden {
		t.Errorf("an action with a first password: %d", code)
	}
	if code, _ := webtest.Get(t, bea, e.URL+"/account"); code != http.StatusOK {
		t.Errorf("the account page: %d", code)
	}
	if code, _ := webtest.Post(t, bea, e.URL+"/cmd/account/password", `{"tab":"tab1","op":"p1","current":"wrong one","password":"my own password"}`); code != http.StatusNoContent {
		t.Errorf("a wrong current password: %d", code)
	}
	if code, _ := webtest.Post(t, bea, e.URL+"/cmd/account/password", `{"tab":"tab1","op":"p2","current":"first password","password":"my own password"}`); code != http.StatusNoContent {
		t.Fatalf("change: %d", code)
	}
	if code, _ := webtest.Get(t, bea, e.URL+"/"); code != http.StatusOK {
		t.Errorf("the first page after the change: %d", code)
	}
	var beaID string
	e.DB.R.QueryRow(`SELECT person_id FROM accounts WHERE email = 'bea@example.com'`).Scan(&beaID)

	// FR-142.2: the last admin keeps their role.
	webtest.Post(t, admin, e.URL+"/cmd/admin/role", `{"tab":"tab1","op":"r1","account":"`+ada+`","role":"none"}`)
	if o := outcome(t, e, people.Person{ID: ada, Kind: people.Account}, "r1"); o.OK || !strings.Contains(o.Msg, "last site admin") {
		t.Errorf("the last admin's own role: %+v", o)
	}
	webtest.Post(t, admin, e.URL+"/cmd/admin/role", `{"tab":"tab1","op":"r2","account":"`+beaID+`","role":"admin"}`)
	if n := rowCount(t, e, `SELECT count(*) FROM accounts WHERE site_role = 'admin'`); n != 2 {
		t.Errorf("%d admins", n)
	}
	if code, _ := webtest.Get(t, bea, e.URL+"/admin"); code != http.StatusOK {
		t.Errorf("the new admin's next page: %d", code)
	}
	// NFR-01.2: a removed role takes the staff's views off the open streams.
	stream := webtest.Listen(t, bea, e.URL+"/stream?tab=tab2&v=admin-demo:", nil)
	webtest.Next(t, stream, `id="admin-demo"`)
	webtest.Post(t, admin, e.URL+"/cmd/admin/role", `{"tab":"tab1","op":"r3","account":"`+beaID+`","role":"moderator"}`)
	for deadline := time.Now().Add(2 * time.Second); len(e.Hub.Watchers("admin-demo")) > 0; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the demoted admin still watches the demo tab")
		}
	}

	// FR-143: deactivation ends the sessions.
	webtest.Post(t, admin, e.URL+"/cmd/admin/active", `{"tab":"tab1","op":"a1","account":"`+beaID+`","active":false}`)
	if code, _ := webtest.Get(t, bea, e.URL+"/admin"); code != http.StatusForbidden {
		t.Errorf("a deactivated account's page: %d", code)
	}
	if n := rowCount(t, e, `SELECT count(*) FROM security_log WHERE kind IN ('role_changed', 'deactivated', 'password_changed', 'account_created')`); n < 4 {
		t.Errorf("%d entries in the security log", n)
	}
	if code, page := webtest.Get(t, admin, e.URL+"/admin/security?kind=role_changed"); code != http.StatusOK || !strings.Contains(page, "Role changed") {
		t.Errorf("the security log page: %d", code)
	}
}
