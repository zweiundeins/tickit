package web_test

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/notify"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web/webtest"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// guest is the person a client's session makes, after its first page.
func guest(t *testing.T, e *webtest.Env, c *http.Client) people.Person {
	t.Helper()
	webtest.Get(t, c, e.URL+"/")
	u, _ := url.Parse(e.URL)
	for _, k := range c.Jar.Cookies(u) {
		if k.Name == "sid" {
			return people.GuestOf(k.Value)
		}
	}
	t.Fatal("no session cookie")
	return people.Person{}
}

// notifyOf stores a notification for a person, as a reaction would, and
// returns its id.
func notifyOf(t *testing.T, e *webtest.Env, to, actor people.Person, key string) int64 {
	t.Helper()
	var id int64
	err := e.W.Do(context.Background(), writer.Func(func(tx *writer.Tx) error {
		now := time.Now().UnixMilli()
		res, err := tx.Exec(`INSERT INTO notifications (recipient, kind, subject_kind, subject_id, subject_label, href, actor, source, last_source, created_at, updated_at)
			VALUES (?, 'assigned', 'issue', ?, ?, ?, ?, (SELECT coalesce(max(source), 0) + 1 FROM notifications), (SELECT coalesce(max(source), 0) + 1 FROM notifications), ?, ?)`, to.ID, "id-"+key, key, "/"+key, actor.ID, now, now)
		if err == nil {
			id, err = res.LastInsertId()
		}
		tx.Touch(people.PersonTopic(to.ID))
		return err
	}))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func unread(t *testing.T, e *webtest.Env, p people.Person) int {
	t.Helper()
	var n int
	e.DB.View(context.Background(), func(tx *sql.Tx) error {
		var err error
		n, err = notify.Unread(context.Background(), tx, p.ID)
		return err
	})
	return n
}

// FR-110, FR-115, MP-01.2: the bell's count, its live region, its panel
// and $_unread reach every stream of the person; marking read and
// dismissing work for one's own notifications, and someone else's answer
// 403 and change nothing.
func TestBellAndActions(t *testing.T) {
	e := webtest.Start(t)
	c, other := webtest.Client(t), webtest.Client(t)
	me, them := guest(t, e, c), guest(t, e, other)
	tab1 := webtest.Listen(t, c, e.URL+"/stream?tab=tab1", nil)
	tab2 := webtest.Listen(t, c, e.URL+"/stream?tab=tab2", nil)
	for _, s := range []<-chan string{tab1, tab2} {
		webtest.Next(t, s, `"_unread":0`)
	}
	first := notifyOf(t, e, me, them, "WEB-12")
	second := notifyOf(t, e, me, them, "WEB-13")
	for _, s := range []<-chan string{tab1, tab2} {
		ev := webtest.Next(t, s, `"_unread":2`)
		_ = ev
	}
	_, page := webtest.Get(t, c, e.URL+"/privacy")
	for _, want := range []string{`id="tk-bell-count"`, `>2</span>`, `2 unread notifications`, `id="tk-notify-panel"`, `WEB-13`, `data-id="` + fmt.Sprint(first) + `"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %s", want)
		}
	}

	post := func(c *http.Client, path, body string) int {
		code, _ := webtest.Post(t, c, e.URL+path, body)
		return code
	}
	if code := post(other, "/cmd/notify/read", fmt.Sprintf(`{"tab":"t9","op":"o1","id":%d}`, first)); code != http.StatusForbidden {
		t.Errorf("reading someone else's: %d", code)
	}
	if code := post(other, "/cmd/notify/dismiss", fmt.Sprintf(`{"tab":"t9","op":"o2","id":%d}`, first)); code != http.StatusForbidden {
		t.Errorf("dismissing someone else's: %d", code)
	}
	if code := post(other, "/cmd/notify/read", `{"tab":"t9","op":"o3","id":424242}`); code != http.StatusNotFound {
		t.Errorf("an unknown one: %d", code)
	}
	if n := unread(t, e, me); n != 2 {
		t.Fatalf("unread %d after someone else's tries", n)
	}
	if code := post(c, "/cmd/notify/read", fmt.Sprintf(`{"tab":"tab1","op":"o4","id":%d}`, first)); code != http.StatusNoContent {
		t.Errorf("read: %d", code)
	}
	for _, s := range []<-chan string{tab1, tab2} {
		webtest.Next(t, s, `"_unread":1`)
	}
	code, body := webtest.Post(t, c, e.URL+"/cmd/notify/open", fmt.Sprintf(`{"tab":"tab1","op":"o5","id":%d}`, second))
	if code != http.StatusOK || !strings.Contains(body, `"_goto":"/WEB-13"`) {
		t.Errorf("open: %d %s", code, body)
	}
	webtest.Next(t, tab2, `"_unread":0`)
	if code := post(c, "/cmd/notify/dismiss-all", `{"tab":"tab1","op":"o6"}`); code != http.StatusNoContent {
		t.Errorf("dismiss all: %d", code)
	}
	webtest.Next(t, tab2, `Nothing new`)
	third := notifyOf(t, e, me, them, "WEB-14")
	notifyOf(t, e, me, them, "WEB-15")
	if code := post(c, "/cmd/notify/dismiss", fmt.Sprintf(`{"tab":"tab1","op":"o7","id":%d}`, third)); code != http.StatusNoContent {
		t.Errorf("dismiss: %d", code)
	}
	if code := post(c, "/cmd/notify/read-all", `{"tab":"tab1","op":"o8"}`); code != http.StatusNoContent {
		t.Errorf("read all: %d", code)
	}
	if code := post(c, "/cmd/notify/seen", `{"tab":"tab1","op":"o9","subject":"id-WEB-15"}`); code != http.StatusNoContent {
		t.Errorf("seen: %d", code)
	}
	if n := unread(t, e, me); n != 0 {
		t.Errorf("unread %d", n)
	}
	if code := post(c, "/cmd/notify/read", `{"tab":"tab1","op":"o10","id":"x"}`); code != http.StatusBadRequest {
		t.Errorf("a malformed id: %d", code)
	}
}

// FR-59, FR-69, FR-111 end to end: an assignment made by one guest
// reaches the other's bell through the reaction, and the home page lists
// the issue, the project and the activity, live.
func TestHomeAndReactions(t *testing.T) {
	store := events.Must(events.New(append(tracker.Parts(), notify.Parts()...)...))
	tracker.Bind(store)
	e := webtest.StartWith(t, webtest.Options{Store: store})
	notify.Bind(e.W)
	t.Cleanup(func() { tracker.Bind(nil); notify.Bind(nil) })
	runner := events.NewRunner(store, e.DB, e.W, events.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); runner.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		live := len(runner.Stats().Projectors) > 0
		for _, p := range runner.Stats().Projectors {
			live = live && p.Status == "live"
		}
		if live {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the runner did not start")
		}
	}

	c, other := webtest.Client(t), webtest.Client(t)
	me, them := guest(t, e, c), guest(t, e, other)
	by := func(p people.Person, op string) tabs.By { return tabs.By{Actor: p, Op: op, At: time.Now()} }
	result := func(cmd writer.ResultCommand) string {
		b, err := e.W.DoValue(context.Background(), func(tx *writer.Tx) ([]byte, error) {
			if err := people.Ensure(tx, me, time.Now()); err != nil {
				return nil, err
			}
			if err := people.Ensure(tx, them, time.Now()); err != nil {
				return nil, err
			}
			return cmd.ApplyResult(tx)
		})
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.Split(string(b), `"id":"`)[1], `"`)[0]
	}
	project := result(tracker.CreateProject{By: by(them, "p1"), Key: "WEB", Name: "Website", Open: true})
	result(tracker.CreateIssue{By: by(me, "p2"), Project: project, Type: tracker.Task, Summary: "Mine to start"})
	home := webtest.Listen(t, c, e.URL+"/stream?tab=tab1&v=home:,activity-home:0", nil)
	webtest.Next(t, home, `id="home-work"`)
	issue := result(tracker.CreateIssue{By: by(them, "p3"), Project: project, Type: tracker.Bug, Summary: "Broken login"})
	e.Do(t, tracker.Assign{By: by(them, "p4"), Issue: issue, Seen: 1 << 40, Assignee: me.ID})
	nextAll(t, home, `"_unread":1`, `Broken login`, `assigned`)

	code, page := webtest.Get(t, c, e.URL+"/")
	for _, want := range []string{"Your work", "Assigned to you", "WEB-2", "Broken login", "Recently edited by you", "WEB-1", "Website", `href="/projects/WEB"`,
		`id="activity-home"`, "created", "assigned", `>WEB-2</a>`, "to you"} {
		if !strings.Contains(page, want) {
			t.Errorf("the home page lacks %q", want)
		}
	}
	if code != http.StatusOK {
		t.Errorf("home: %d", code)
	}
	// The open counts come from a cache (projectsCache) that every change
	// to an issue of the project, which touches its board, refreshes.
	third := result(tracker.CreateIssue{By: by(them, "p5"), Project: project, Type: tracker.Task, Summary: "A third"})
	webtest.Next(t, home, "3 open issues")
	e.Do(t, tracker.SetStatus{By: by(them, "p6"), Issue: third, Seen: 1 << 40, Status: tracker.StatusDone})
	webtest.Next(t, home, "2 open issues")

	// The project's feed is a view of its own.
	feed := webtest.Listen(t, other, e.URL+"/stream?tab=tab2&v=activity-project:"+project, nil)
	webtest.Next(t, feed, `id="activity-project-`+project+`"`)
	// NFR-35: an append that no feed shows, such as a commit to a page's
	// document, renders neither the feeds nor the home page's own view.
	feeds := func() map[string]int64 {
		out := map[string]int64{}
		for key, n := range e.Hub.Stats().RendersByKey {
			if strings.HasPrefix(key, "home:") || strings.HasPrefix(key, "activity:") {
				out[key] = n
			}
		}
		return out
	}
	time.Sleep(50 * time.Millisecond)
	before := feeds()
	e.Do(t, writer.Func(func(tx *writer.Tx) error { tx.Touch(events.Topic, wiki.DocTopic("p1")); return nil }))
	time.Sleep(100 * time.Millisecond)
	if after := feeds(); len(before) != 3 || !maps.Equal(before, after) {
		t.Errorf("renders of the feeds and the home view before a commit to a document %v, after %v", before, after)
	}
	if code, _ := webtest.Get(t, other, e.URL+"/stream?tab=tab3&v=activity-project:nope"); code != http.StatusNotFound {
		t.Errorf("an unknown project's feed: %d", code)
	}
}

// nextAll reads events until each of wants showed in one, in any order.
func nextAll(t *testing.T, events <-chan string, wants ...string) {
	t.Helper()
	left := map[string]bool{}
	for _, w := range wants {
		left[w] = true
	}
	timeout := time.After(5 * time.Second)
	for len(left) > 0 {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("the stream ended before %v", left)
			}
			for w := range left {
				if strings.Contains(ev, w) {
					delete(left, w)
				}
			}
		case <-timeout:
			t.Fatalf("no events with %v in 5s", left)
		}
	}
}
