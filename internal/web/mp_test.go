package web_test

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/views"
	"github.com/zweiundeins/tickit/internal/web"
	"github.com/zweiundeins/tickit/internal/web/webtest"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// MP-11.4: the renders of each view are visible, by kind in the metrics
// (tickit_view_renders_total{kind}) and by key on the admin statistics,
// where keys that hold a tab id show only as a hash.
func TestViewRenders(t *testing.T) {
	e := webtest.Start(t)
	things(e)
	e.Account(t, "ada@example.com", "correct horse battery", people.Admin)
	admin := webtest.Client(t)
	e.Login(t, admin, "ada@example.com", "correct horse battery")
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	events := webtest.Listen(t, c, e.URL+"/stream?tab=renders1&v=thing:7", nil)
	webtest.Next(t, events, `<p id="thing-7">`)
	for range 3 {
		e.Do(t, writer.Func(func(tx *writer.Tx) error { tx.Touch("thing"); return nil }))
		time.Sleep(30 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)

	reg := prometheus.NewRegistry()
	reg.MustRegister(e.Server.Collectors()...)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]float64{}
	for _, mf := range mfs {
		if mf.GetName() != "tickit_view_renders_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "kind" {
					kinds[l.GetValue()] = m.GetCounter().GetValue()
				}
			}
		}
	}
	if kinds["thing"] < 3 || kinds["tab"] < 1 || kinds["presence"] < 1 {
		t.Errorf("tickit_view_renders_total by kind: %v", kinds)
	}

	code, page := webtest.Get(t, admin, e.URL+"/admin/stats")
	if code != http.StatusOK || !strings.Contains(page, "Renders per view") || !strings.Contains(page, "<code>thing:7</code>") {
		t.Fatalf("the statistics: %d\n%s", code, page)
	}
	if strings.Contains(page, "renders1") || !strings.Contains(page, "<code>tab:#") {
		t.Errorf("the statistics show a tab id, or no hashed tab key")
	}
}

// rawStream opens the tab's stream on a connection of its own with c's
// cookies, reads its first bytes and then stops reading without closing:
// a client whose network went away (MP-02.3). It returns the connection.
func rawStream(t *testing.T, c *http.Client, base, path string) net.Conn {
	t.Helper()
	u, _ := url.Parse(base)
	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	var cookies []string
	for _, ck := range c.Jar.Cookies(u) {
		cookies = append(cookies, ck.Name+"="+ck.Value)
	}
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nAccept: text/event-stream\r\nAccept-Encoding: identity\r\nCookie: %s\r\n\r\n", path, u.Host, strings.Join(cookies, "; "))
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if n, err := conn.Read(buf); err != nil || !strings.Contains(string(buf[:n]), "200 OK") {
		t.Fatalf("the raw stream: %v %q", err, buf[:n])
	}
	return conn
}

// MP-02.3: a face goes within 1 s when its person's last tab closes, and
// within 60 s when its connection drops without closing (the page stops
// answering heartbeats, alive.go), while a page that answers them stays.
func TestPresenceGoes(t *testing.T) {
	if web.AliveTimeout >= 60*time.Second {
		t.Fatalf("a dropped connection's stream stays %v", web.AliveTimeout)
	}
	d := web.AliveTimeout
	web.AliveTimeout = 3 * time.Second // as 50 s, sooner
	e := webtest.Start(t)
	web.AliveTimeout = d
	things(e)
	ada, bob, cy := webtest.Client(t), webtest.Client(t), webtest.Client(t)
	webtest.Get(t, ada, e.URL+"/")
	name := func(c *http.Client) string {
		_, page := webtest.Get(t, c, e.URL+"/")
		return personName.FindStringSubmatch(page)[1]
	}
	bobName, cyName := name(bob), name(cy)
	row := `<ul id="` + views.PresenceID("thing:1") + `" class="tk-presence"`
	first := webtest.Listen(t, ada, e.URL+"/stream?tab=ada1&v=thing:1", nil) // a stream that never answers heartbeats, as a Go client
	webtest.Next(t, first, row)
	until := func(d time.Duration, ok func(string) bool) time.Duration {
		t.Helper()
		start := time.Now()
		deadline := time.After(d)
		for {
			select {
			case ev := <-first:
				if strings.Contains(ev, row) && ok(ev) {
					return time.Since(start)
				}
			case <-deadline:
				t.Fatalf("not within %v", d)
			}
		}
	}
	there := func(n string) func(string) bool { return func(ev string) bool { return strings.Contains(ev, n) } }
	gone := func(n string) func(string) bool { return func(ev string) bool { return !strings.Contains(ev, n) } }

	_, close1 := webtest.Open(t, bob, e.URL+"/stream?tab=bob1&alive=1&v=thing:1", nil)
	_, close2 := webtest.Open(t, bob, e.URL+"/stream?tab=bob2&alive=1&v=thing:1", nil)
	until(5*time.Second, there(bobName+", 2 tabs"))
	close1()
	until(time.Second, there(bobName+`"`))
	close2()
	if d := until(time.Second, gone(bobName)); d > time.Second {
		t.Errorf("the face went %v after the last tab closed", d)
	}

	// cy's page answers every heartbeat; bob's connection drops.
	webtest.Listen(t, cy, e.URL+"/stream?tab=cy1&alive=1&v=thing:1", nil)
	go func() {
		for {
			select {
			case <-t.Context().Done():
				return
			case <-time.After(time.Second):
				webtest.Post(t, cy, e.URL+"/alive?tab=cy1", "")
			}
		}
	}()
	rawStream(t, bob, e.URL, "/stream?tab=bob3&alive=1&v=thing:1")
	until(5*time.Second, func(ev string) bool { return strings.Contains(ev, bobName) && strings.Contains(ev, cyName) })
	d = until(10*time.Second, gone(bobName))
	if d > 4*time.Second {
		t.Errorf("the face of a dropped connection went after %v", d)
	}
	if code, _ := webtest.Get(t, ada, e.URL+"/"); code != http.StatusOK {
		t.Fatal(code)
	}
	if !e.Hub.HasStream(people.GuestID(cookie(cy, e.URL)) + ".cy1") {
		t.Errorf("the stream of a page that answers its heartbeats ended after %v", d)
	}
	t.Logf("a dropped connection's face went after %v", d)
}

// MP-07.4: a typing indicator goes at once when the tab that types goes
// (its stream ends), not 5 s after the last keystroke; another tab of
// the same person that took the field over keeps it.
func TestTypingGoesWithTab(t *testing.T) {
	e := webtest.Start(t)
	seedProbes(t, e)
	ada, bob := webtest.Client(t), webtest.Client(t)
	webtest.Get(t, ada, e.URL+"/")
	me := guest(t, e, bob)
	view := "issue-typing:" + web.ProbeIssue
	first := webtest.Listen(t, ada, e.URL+"/stream?tab=ada1&v="+view, nil)
	webtest.Next(t, first, `id="issue-typing-comment"`)
	_, closeBob := webtest.Open(t, bob, e.URL+"/stream?tab=bob1&v="+view, nil)
	typing := `data-who="` + me.ID + `"`
	if code, _ := webtest.Post(t, bob, e.URL+"/cmd/issues/"+web.ProbeIssue+"/typing", `{"tab":"bob1","op":"t1","field":"comment"}`); code != http.StatusNoContent {
		t.Fatalf("typing: %d", code)
	}
	webtest.Next(t, first, typing)
	closeBob()
	start := time.Now()
	for {
		ev := webtest.Next(t, first, `id="issue-typing-comment"`)
		if !strings.Contains(ev, typing) {
			break
		}
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("the indicator went %v after its tab", d)
	}

	// Typed in bob2 after bob3: bob3's end leaves bob2's indicator.
	webtest.Listen(t, bob, e.URL+"/stream?tab=bob2&v="+view, nil)
	_, close3 := webtest.Open(t, bob, e.URL+"/stream?tab=bob3&v="+view, nil)
	webtest.Post(t, bob, e.URL+"/cmd/issues/"+web.ProbeIssue+"/typing", `{"tab":"bob3","op":"t2","field":"summary"}`)
	webtest.Post(t, bob, e.URL+"/cmd/issues/"+web.ProbeIssue+"/typing", `{"tab":"bob2","op":"t3","field":"summary"}`)
	webtest.Next(t, first, typing)
	close3()
	time.Sleep(300 * time.Millisecond)
	e.W.Flush(t.Context())
	if rowCount(t, e, `SELECT count(*) FROM issue_typing WHERE person_id = ? AND tab = 'bob2'`, me.ID) != 1 {
		t.Error("another tab's indicator went")
	}
}

// MP-02.4: a person without input for 5 minutes shows as away in every
// presence row, a dimmed face and "away" in its tooltip, and as soon as
// their page reports input again, not away. A report within half a
// minute of the last writes nothing.
func TestAway(t *testing.T) {
	e := webtest.Start(t)
	things(e)
	ada, bob := webtest.Client(t), webtest.Client(t)
	webtest.Get(t, ada, e.URL+"/")
	me := guest(t, e, bob)
	_, page := webtest.Get(t, bob, e.URL+"/")
	bobName := personName.FindStringSubmatch(page)[1]
	row := `<ul id="` + views.PresenceID("thing:1") + `" class="tk-presence"`
	first := webtest.Listen(t, ada, e.URL+"/stream?tab=ada1&v=thing:1", nil)
	webtest.Listen(t, bob, e.URL+"/stream?tab=bob1&v=thing:1", nil)
	away := regexp.MustCompile(`data-who="` + me.ID + `"[^>]*>\s*<span class="tk-tip" role="img" tabindex="0" aria-label="` + regexp.QuoteMeta(bobName) + `, away">\s*<span class="tk-av[^"]*" data-hue="\d+" data-state="away"`)
	next := func(want bool) {
		t.Helper()
		for {
			ev := webtest.Next(t, first, row)
			if strings.Contains(ev, bobName) && away.MatchString(ev) == want {
				return
			}
		}
	}
	if code, _ := webtest.Post(t, bob, e.URL+"/cmd/input", `{"tab":"bob1","op":"i1"}`); code != http.StatusNoContent {
		t.Fatalf("input: %d", code)
	}
	next(false)
	// Six minutes pass without input.
	e.W.Flush(t.Context())
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		_, err := tx.Exec(`UPDATE presence_input SET at = ? WHERE person_id = ?`, time.Now().Add(-6*time.Minute).Unix(), me.ID)
		return err
	}))
	e.Server.TurnAway(t.Context(), time.Now().Add(-2*time.Minute), time.Now())
	next(true)
	if code, _ := webtest.Post(t, bob, e.URL+"/cmd/input", `{"tab":"bob1","op":"i2"}`); code != http.StatusNoContent {
		t.Fatalf("input: %d", code)
	}
	next(false)
	e.W.Flush(t.Context())
	before := e.Fingerprint(t)
	webtest.Post(t, bob, e.URL+"/cmd/input", `{"tab":"bob1","op":"i3"}`)
	e.W.Flush(t.Context())
	if e.Fingerprint(t) != before {
		t.Error("a report within a minute wrote")
	}
	// The page reports input as it opens and while there is input, at
	// most once a minute.
	if !strings.Contains(page, `data-on:pointermove__window__throttle.5s="Date.now() - $_input &gt; 60000 &amp;&amp;`) || !strings.Contains(page, `@post(&#39;/cmd/input&#39;`) {
		t.Error("the page does not report input")
	}
}

// MP-02.1: a space's page tree has a presence row of everyone on any page
// of the space, whichever page each is on.
func TestSpaceTreePresence(t *testing.T) {
	e := startWiki(t)
	ada, bob := webtest.Client(t), webtest.Client(t)
	webtest.Get(t, ada, e.URL+"/")
	_, page := webtest.Get(t, bob, e.URL+"/")
	bobName := personName.FindStringSubmatch(page)[1]
	space, home := wikiSpace(t, e, ada, "ENG")
	one := wikiPage(t, e, ada, space, home, "One")
	two := wikiPage(t, e, ada, space, home, "Two")
	rowID := views.PresenceID("pages-in:" + space)
	if code, doc := webtest.Get(t, ada, e.URL+wiki.PageURL("ENG", one, "One")); code != http.StatusOK || !strings.Contains(doc, `<ul id="`+rowID+`"`) {
		t.Fatalf("the page's tree has no presence row: %d", code)
	}
	first := webtest.Listen(t, ada, e.URL+"/stream?tab=ada1&v=page:"+one+",tree:"+one, nil)
	webtest.Next(t, first, `<ul id="`+rowID+`"`)
	webtest.Listen(t, bob, e.URL+"/stream?tab=bob1&v=page:"+two+",tree:"+two, nil)
	for {
		ev := webtest.Next(t, first, `<ul id="`+rowID+`"`)
		if strings.Contains(ev, bobName) {
			break
		}
	}
}

// NFR-01.2: removed from a closed project, a person's open pages stop
// showing its issues at once: the issue page's stream is told to load
// again (and the page then answers 404), and the issue list's frame no
// longer holds the issue.
func TestRemovedMemberViews(t *testing.T) {
	e := webtest.Start(t)
	ownerID := e.Account(t, "owner@example.com", "correct horse battery", people.NoRole)
	owner := webtest.Client(t)
	e.Login(t, owner, "owner@example.com", "correct horse battery")
	found, err := people.Lookup(t.Context(), e.DB.R, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	by := func(op string) tabs.By { return tabs.By{Actor: found[ownerID], Op: op, At: time.Now()} }
	project, issue := tracker.NewID(), tracker.NewID()
	e.Do(t, tracker.CreateProject{By: by("p1"), ID: project, Key: "SEC", Name: "Secret", Open: false})
	e.Do(t, tracker.CreateIssue{By: by("i1"), ID: issue, Project: project, Type: tracker.Task, Summary: "The secret plan"})
	bob := webtest.Client(t)
	me := guest(t, e, bob)
	webtest.Post(t, bob, e.URL+"/cmd/people/rename", `{"tab":"bob0","op":"r1","name":"Bob Member"}`)
	e.Do(t, tracker.SetProjectMember{By: by("m1"), Project: project, Person: me.ID, Role: access.Member})

	code, page := webtest.Get(t, bob, e.URL+"/SEC-1")
	if code != http.StatusOK {
		t.Fatalf("the member's issue page: %d", code)
	}
	issuePage := webtest.Listen(t, bob, e.URL+"/stream?tab=bob1&v=issue:"+issue, nil)
	webtest.Next(t, issuePage, "The secret plan")
	_, page = webtest.Get(t, bob, e.URL+"/issues?sort=key")
	spec := regexp.MustCompile(`list:[A-Za-z0-9_.-]+`).FindString(page)
	if spec == "" {
		t.Fatal("no list view in the list's page")
	}
	list := webtest.Listen(t, bob, e.URL+"/stream?tab=bob2&v="+spec, nil)
	webtest.Next(t, list, "The secret plan")

	if code, body := webtest.Post(t, owner, e.URL+"/cmd/projects/member/remove", `{"tab":"own1","op":"x1","project":"SEC","person":"`+me.ID+`"}`); code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", code, body)
	}
	start := time.Now()
	webtest.Next(t, issuePage, `"_revoked":true`)
	for {
		ev := webtest.Next(t, list, `id="issue-list"`)
		if !strings.Contains(ev, "The secret plan") {
			break
		}
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("the views changed %v after the removal", d)
	}
	if code, _ := webtest.Get(t, bob, e.URL+"/SEC-1"); code != http.StatusNotFound {
		t.Errorf("the issue page loaded again: %d", code)
	}
	// The person's other views stay: the issue list's page keeps its stream.
	if !e.Hub.HasStream(me.ID + ".bob2") {
		t.Error("the list's stream ended")
	}
}

// NFR-01.2: a change of role applies to the person's open pages of the
// project at once: they load again with what the new role may do, while
// pages elsewhere keep their streams.
func TestRoleChangeReloads(t *testing.T) {
	e := webtest.Start(t)
	seedProbes(t, e)
	ownerID := e.Account(t, "owner@example.com", "correct horse battery", people.NoRole)
	owner := webtest.Client(t)
	e.Login(t, owner, "owner@example.com", "correct horse battery")
	e.Do(t, tracker.SetProjectMember{By: byOf(people.Seeded[0], "m0"), Project: probeWeb, Person: ownerID, Role: access.Admin})
	bobID := e.Account(t, "bob@example.com", "correct horse battery", people.NoRole)
	bob := webtest.Client(t)
	e.Login(t, bob, "bob@example.com", "correct horse battery")
	issuePage := webtest.Listen(t, bob, e.URL+"/stream?tab=bob1&v=issue:"+web.ProbeIssue, nil)
	webtest.Next(t, issuePage, `id="issue-main"`)
	elsewhere := webtest.Listen(t, bob, e.URL+"/stream?tab=bob2&v=", nil)
	webtest.Next(t, elsewhere, `"_outcomes"`)
	if code, body := webtest.Post(t, owner, e.URL+"/cmd/projects/member", `{"tab":"own1","op":"x1","project":"WEB","person":"`+bobID+`","role":"viewer"}`); code != http.StatusNoContent {
		t.Fatalf("set role: %d %s", code, body)
	}
	webtest.Next(t, issuePage, `"_revoked":true`)
	webtest.Silent(t, elsewhere, `_revoked`, 200*time.Millisecond)
}

// MP-14.1: a blocked guest's commit answers 403, and the tab's editor
// frame stops the editor read-only with the reason, which the editor
// shows at the editor and keeps the text unsaved.
func TestBlockedCommitSaysWhy(t *testing.T) {
	e := startWiki(t)
	e.Account(t, "mo@example.com", "correct horse battery", people.Moderator)
	mo := webtest.Client(t)
	e.Login(t, mo, "mo@example.com", "correct horse battery")
	c := webtest.Client(t)
	me := guest(t, e, c)
	space, home := wikiSpace(t, e, c, "ENG")
	page := wikiPage(t, e, c, space, home, "Notes")
	h := head(t, e, page)
	editor := webtest.Listen(t, c, e.URL+"/stream?tab=tab1&v=carets:"+page, nil)
	if code, _ := act(t, e, c, "/cmd/pages/"+page+"/open", "tab1", map[string]any{"base": h.Version, "client": "0123456789ab"}); code != http.StatusNoContent {
		t.Fatalf("open: %d", code)
	}
	webtest.Next(t, editor, `state="ok"`)
	if code, body := webtest.Post(t, mo, e.URL+"/cmd/moderation/block", `{"tab":"m1","op":"b1","person":"`+me.ID+`","reason":"spam"}`); code != http.StatusNoContent {
		t.Fatalf("block: %d %s", code, body)
	}
	text := `[{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"text","text":"Hi"}]}}]`
	code, _ := webtest.Post(t, c, e.URL+"/cmd/pages/"+page+"/commit",
		fmt.Sprintf(`{"tab":"tab1","epoch":%q,"base":%d,"ref":"r1","client":"0123456789ab","steps":%s}`, h.Epoch, h.Version, text))
	if code != http.StatusForbidden {
		t.Errorf("a blocked guest's commit: %d", code)
	}
	ev := webtest.Next(t, editor, `state="readonly"`)
	if !strings.Contains(ev, `refusal="Changes from this browser are paused."`) || !strings.Contains(ev, " readonly") {
		t.Errorf("the editor's frame: %.400s", ev)
	}
	if got := head(t, e, page); got.Version != h.Version {
		t.Errorf("the commit applied: version %d", got.Version)
	}
}
