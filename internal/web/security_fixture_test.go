package web_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/dashboards"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web"
	"github.com/zweiundeins/tickit/internal/web/webtest"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// The security suite's world (docs/security.md): a closed project SEC and
// a closed space SECS, whose Admin is owner, with member as Member and
// viewer as Viewer; an open project OPN and an open space OPS of root's;
// and every kind of person who may lack a right.

// who is a kind of person the suite sends requests as.
type who string

const (
	wGuest   who = "guest"   // a guest, without a role in the closed scopes
	wViewer  who = "viewer"  // an account, Viewer of SEC and SECS
	wMember  who = "member"  // an account, Member of SEC and SECS
	wOwner   who = "owner"   // an account, Admin of SEC and SECS, no site role
	wOther   who = "other"   // an account without any role: the victim of forged fields
	wMod     who = "mod"     // a moderator
	wRoot    who = "root"    // a site admin
	wBlocked who = "blocked" // a guest a moderator blocked
	wMust    who = "must"    // an account whose first password must change
)

var accountsOf = []who{wViewer, wMember, wOwner, wOther, wMod, wRoot, wMust}

const fixturePassword = "correct horse battery"

// fixtureHash is one argon2id hash for every account of the suite.
var fixtureHash = sync.OnceValue(func() string {
	h, err := people.HashPassword(fixturePassword)
	if err != nil {
		panic(err)
	}
	return h
})

// fixture is the suite's world on one server.
type fixture struct {
	t   testing.TB
	e   *webtest.Env
	p   map[who]people.Person
	ids map[string]string // "issue" is SEC's, "o.issue" OPN's: see id
	mu  sync.Mutex
	c   map[who]*http.Client
	n   atomic.Int64
}

// newFixture starts a server without rate limits and writes the world.
func newFixture(t testing.TB) *fixture {
	t.Helper()
	e := webtest.StartWith(t, webtest.Options{Store: wikiStore(), Configure: web.Unlimited})
	f := &fixture{t: t, e: e, p: map[who]people.Person{}, ids: map[string]string{}, c: map[who]*http.Client{}}
	now := time.Now().Add(-time.Hour)
	for _, w := range accountsOf {
		role := people.NoRole
		switch w {
		case wMod:
			role = people.Moderator
		case wRoot:
			role = people.Admin
		}
		id := people.NewAccountID()
		webtest.Secret(fixturePassword)
		e.Do(t, people.CreateAccount{ID: id, Email: string(w) + "@example.com", Username: string(w), FullName: "The " + string(w),
			Role: role, PasswordHash: fixtureHash(), At: now})
		f.p[w] = people.Person{ID: id, Kind: people.Account, Name: "The " + string(w), Role: role}
	}
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		_, err := tx.Exec(`UPDATE accounts SET must_change_password = 1 WHERE person_id = ?`, f.p[wMust].ID)
		return err
	}))
	by := f.by
	nid := func(name string) string { f.ids[name] = tracker.NewID(); return f.ids[name] }
	doc := tracker.PlainDoc

	// The closed project, its members and what it holds.
	members := func(s access.Scope) {
		e.Do(t, writer.Func(func(tx *writer.Tx) error {
			if err := access.SetMember(tx, s, f.p[wMember].ID, access.Member, f.p[wOwner].ID, now); err != nil {
				return err
			}
			return access.SetMember(tx, s, f.p[wViewer].ID, access.Viewer, f.p[wOwner].ID, now)
		}))
	}
	e.Do(t, tracker.CreateProject{By: by(wOwner), ID: nid("proj"), Key: "SEC", Name: "Secret", Lead: f.p[wOwner].ID})
	f.ids["projKey"] = "SEC"
	members(access.ProjectScope(f.ids["proj"]))
	e.Do(t, tracker.CreateProject{By: by(wRoot), ID: nid("o.proj"), Key: "OPN", Name: "Open", Lead: f.p[wRoot].ID, Open: true})
	f.ids["o.projKey"] = "OPN"
	for _, scope := range []string{"", "o."} {
		admin := wOwner
		if scope != "" {
			admin = wRoot
		}
		proj := f.ids[scope+"proj"]
		e.Do(t, tracker.CreateIssue{By: by(admin), ID: nid(scope + "issue"), Project: proj, Type: tracker.Task, Summary: "The first", Description: doc("Text")})
		e.Do(t, tracker.CreateIssue{By: by(admin), ID: nid(scope + "issue2"), Project: proj, Type: tracker.Bug, Summary: "The second"})
		e.Do(t, tracker.CreateIssue{By: by(admin), ID: nid(scope + "epic"), Project: proj, Type: tracker.Epic, Summary: "The epic"})
		e.Do(t, tracker.AddComment{By: by(map[string]who{"": wMember, "o.": wRoot}[scope]), ID: nid(scope + "comment"), Issue: f.ids[scope+"issue"], Body: doc("A comment")})
		e.Do(t, tracker.DefineComponent{By: by(admin), Project: proj, Name: "core"})
		e.Do(t, tracker.CreateSprint{By: by(admin), Project: proj, ID: nid(scope + "sprint"), Name: "Planned"})
		e.Do(t, tracker.CreateSprint{By: by(admin), Project: proj, ID: nid(scope + "sprint2"), Name: "Running"})
		e.Do(t, tracker.StartSprint{By: by(admin), Project: proj, Sprint: f.ids[scope+"sprint2"]})
		e.Do(t, tracker.LinkIssues{By: by(admin), Issue: f.ids[scope+"issue"], Type: tracker.RelatesTo, Other: f.ids[scope+"issue2"]})
	}
	// The closed space and the open one, their pages and comments.
	e.Do(t, wiki.CreateSpace{By: by(wOwner), ID: nid("space"), Home: nid("home"), Key: "SECS", Name: "Secret space"})
	f.ids["spaceKey"] = "SECS"
	members(access.SpaceScope(f.ids["space"]))
	e.Do(t, wiki.CreateSpace{By: by(wRoot), ID: nid("o.space"), Home: nid("o.home"), Key: "OPS", Name: "Open space", Open: true})
	f.ids["o.spaceKey"] = "OPS"
	for _, scope := range []string{"", "o."} {
		admin, writer_ := wOwner, wMember
		if scope != "" {
			admin, writer_ = wRoot, wRoot
		}
		space, home := f.ids[scope+"space"], f.ids[scope+"home"]
		e.Do(t, wiki.CreatePage{By: by(admin), ID: nid(scope + "page"), Space: space, Parent: home, Title: "The plan"})
		e.Do(t, wiki.CreatePage{By: by(writer_), ID: nid(scope + "mpage"), Space: space, Parent: home, Title: "A member's page"})
		e.Do(t, wiki.CreatePage{By: by(admin), ID: nid(scope + "tpage"), Space: space, Parent: home, Title: "Old notes"})
		e.Do(t, wiki.TrashPage{By: by(admin), Page: f.ids[scope+"tpage"]})
		e.Do(t, wiki.AddComment{By: by(writer_), ID: nid(scope + "pcomment"), Page: f.ids[scope+"page"], Body: doc("A page comment")})
	}
	// A saved filter and a notification of member's.
	e.Do(t, tracker.SaveFilter{By: by(wMember), ID: nid("filter"), Name: "Mine", Filter: tracker.Filter{Projects: []string{"SEC"}}})
	f.ids["o.filter"] = f.ids["filter"]
	// A dashboard of member's with two gadgets (FR-60).
	e.Do(t, dashboards.Create{By: by(wMember), ID: nid("dashboard"), Name: "Mine"})
	e.Do(t, dashboards.AddGadget{By: by(wMember), Dashboard: f.ids["dashboard"], ID: nid("gadget"), Kind: dashboards.Status, Project: f.ids["proj"]})
	e.Do(t, dashboards.AddGadget{By: by(wMember), Dashboard: f.ids["dashboard"], ID: nid("gadget2"), Kind: dashboards.Assigned})
	for _, k := range []string{"dashboard", "gadget", "gadget2"} {
		f.ids["o."+k] = f.ids[k]
	}
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		res, err := tx.Exec(`INSERT INTO notifications (recipient, kind, subject_kind, subject_id, subject_label, href, actor, source, last_source, created_at, updated_at)
			VALUES (?, 'assigned', 'issue', ?, 'SEC-1', '/SEC-1', ?, 1, 1, ?, ?)`, f.p[wMember].ID, f.ids["issue"], f.p[wOwner].ID, now.UnixMilli(), now.UnixMilli())
		if err != nil {
			return err
		}
		n, _ := res.LastInsertId()
		f.ids["notification"] = fmt.Sprint(n)
		f.ids["o.notification"] = f.ids["notification"]
		return nil
	}))
	f.ids["victim"] = f.p[wOther].ID
	f.ids["o.victim"] = f.p[wOther].ID
	f.ids["session"] = "0123456789abcdef0123456789abcdef" // no session's
	f.ids["member"] = f.p[wMember].ID
	f.ids["o.member"] = f.p[wMember].ID
	return f
}

// by is w acting in a command of the fixture's.
func (f *fixture) by(w who) tabs.By {
	return tabs.By{Actor: f.p[w], Op: fmt.Sprintf("fx%d", f.n.Add(1)), At: time.Now().Add(-time.Minute)}
}

// id is a fixture's id of name: the closed scope's, or the open one's.
func (f *fixture) id(name string, open bool) string {
	if open {
		if v, ok := f.ids["o."+name]; ok {
			return v
		}
	}
	v, ok := f.ids[name]
	if !ok {
		f.t.Fatalf("the fixture has no %q", name)
	}
	return v
}

// addrs gives each kind of person a client address of their own, so a
// block of one address blocks no one else.
var addrs = map[who]string{wGuest: "203.0.113.1", wViewer: "203.0.113.2", wMember: "203.0.113.3", wOwner: "203.0.113.4",
	wOther: "203.0.113.5", wMod: "203.0.113.6", wRoot: "203.0.113.7", wBlocked: "203.0.113.8", wMust: "203.0.113.9"}

type fromAddr struct {
	next http.RoundTripper
	addr string
}

func (a fromAddr) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("X-Forwarded-For", a.addr)
	return a.next.RoundTrip(r)
}

// client is w's browser: a guest with a cookie, or an account logged in;
// the blocked guest is blocked by mod on first use.
func (f *fixture) client(w who) *http.Client {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.c[w]; ok {
		return c
	}
	t, e := f.t, f.e
	c := webtest.Client(t)
	c.Transport = fromAddr{c.Transport, addrs[w]}
	webtest.Get(t, c, e.URL+"/")
	switch w {
	case wGuest, wBlocked:
		f.p[w] = guestOf(t, e, c)
	default:
		e.Login(t, c, string(w)+"@example.com", fixturePassword)
	}
	if w == wGuest || w == wBlocked { // a guest's row, with their first action
		if code, _ := webtest.Post(t, c, e.URL+"/cmd/prefs", `{"tab":"warm","op":"warm","key":"theme","value":"light"}`); code != http.StatusNoContent {
			t.Fatalf("%s's first action: %d", w, code)
		}
	}
	if w == wBlocked {
		e.Do(t, e.Server.Demo().Block(tabs.By{Actor: f.p[wMod], Op: "block-it", At: time.Now()}, f.p[wBlocked].ID, "spam"))
	}
	f.c[w] = c
	return c
}

func guestOf(t testing.TB, e *webtest.Env, c *http.Client) people.Person {
	u, _ := url.Parse(e.URL)
	for _, k := range c.Jar.Cookies(u) {
		if k.Name == "sid" {
			return people.GuestOf(k.Value)
		}
	}
	t.Fatal("no session cookie")
	return people.Person{}
}

// sid is w's session id.
func (f *fixture) sid(w who) string {
	u, _ := url.Parse(f.e.URL)
	for _, k := range f.client(w).Jar.Cookies(u) {
		if k.Name == "sid" {
			return k.Value
		}
	}
	f.t.Fatalf("%s has no session", w)
	return ""
}

// post sends body to path as w, as Datastar does from the site's page.
func (f *fixture) post(w who, path string, body map[string]any) (int, string) {
	f.t.Helper()
	b, _ := json.Marshal(body)
	return webtest.Post(f.t, f.client(w), f.e.URL+path, string(b))
}

// get answers a GET as w within d: a stream is cut off then.
func (f *fixture) get(c *http.Client, path string, d time.Duration) (int, string) {
	ctx, cancel := context.WithTimeout(f.t.Context(), d)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", f.e.URL+path, nil)
	res, err := c.Do(req)
	if err != nil {
		return 0, ""
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// read answers a GET: a page whole, a stream until the tab's view came,
// which comes last of its first frames, or 2 s.
func (f *fixture) read(c *http.Client, path string) (int, string) {
	ctx, cancel := context.WithTimeout(f.t.Context(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", f.e.URL+path, nil)
	req.Header.Set("Accept-Encoding", "identity")
	res, err := c.Do(req)
	if err != nil {
		return 0, ""
	}
	defer res.Body.Close()
	if !strings.HasPrefix(path, "/stream") || res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	var got strings.Builder
	buf := make([]byte, 32<<10)
	for !strings.Contains(got.String(), `"_outcomes"`) {
		n, err := res.Body.Read(buf)
		got.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return res.StatusCode, got.String()
}

// Bookkeeping that a refused action writes besides nothing: the op's
// answer for a retry (lith_results, lith_ops), the tab's outcome, which
// says why it was refused (tab_state), and when an account's login
// session was last used (login_sessions.last_seen_at).
var bookkeeping = []string{"lith_results", "lith_ops", "tab_state", "login_sessions.last_seen_at"}

// state is a digest of every table but the bookkeeping, flushed first.
func (f *fixture) state() map[string]string {
	f.t.Helper()
	if err := f.e.W.Flush(context.Background()); err != nil {
		f.t.Fatal(err)
	}
	return f.e.Digests(f.t, bookkeeping...)
}

// changed lists the tables that differ between two states.
func changed(a, b map[string]string) []string {
	var out []string
	for k, v := range b {
		if a[k] != v {
			out = append(out, k)
		}
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			out = append(out, k)
		}
	}
	return out
}

// outcomeOf is what w's tab says of op, and whether it holds one.
func (f *fixture) outcomeOf(w who, tab, op string) (tabs.Outcome, bool) {
	var st tabs.State
	err := f.e.DB.View(context.Background(), func(tx *sql.Tx) (err error) {
		st, err = tabs.Get(context.Background(), tx, f.sid(w), tab)
		return err
	})
	if err != nil {
		f.t.Fatal(err)
	}
	for _, o := range st.Outcomes {
		if o.Op == op {
			return o, true
		}
	}
	return tabs.Outcome{}, false
}

// views lists the stream's views a page names ($_views).
func viewsOf(page string) []string {
	const mark = "_views: &#34;"
	i := strings.Index(page, mark)
	if i < 0 {
		return nil
	}
	rest := page[i+len(mark):]
	j := strings.Index(rest, "&#34;")
	if j <= 0 {
		return nil
	}
	return strings.Split(rest[:j], ",")
}
