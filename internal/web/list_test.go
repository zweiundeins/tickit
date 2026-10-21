package web_test

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// listEnv is a server with the projects WEB and APP and these issues, by
// Ada unless said:
//
//	WEB-1 story "Fix the login"      todo       high     assignee Bo    label ux
//	WEB-2 bug   "Cart 100% broken"   inreview   critical               label checkout
//	WEB-3 task  "Search_box"         done       low      assignee Ada
//	APP-1 task  "Crash on start"     inprogress medium   assignee Bo
type listEnv struct {
	*webtest.Env
	t        *testing.T
	n        int
	web, app string
	ids      map[string]string
	ada, bo  people.Person
}

func newList(t *testing.T) *listEnv { return newListOn(t, webtest.Start(t)) }

func newListOn(t *testing.T, w *webtest.Env) *listEnv {
	t.Helper()
	e := &listEnv{Env: w, t: t, ids: map[string]string{}, ada: people.Seeded[0], bo: people.Seeded[1]}
	e.Do(t, people.SeedPeople{At: time.Now()})
	e.web = e.result(tracker.CreateProject{By: e.by(e.ada), Key: "WEB", Name: "Website", Open: true})
	e.app = e.result(tracker.CreateProject{By: e.by(e.ada), Key: "APP", Name: "App", Open: true})
	e.issue("WEB-1", tracker.CreateIssue{Project: e.web, Type: tracker.Story, Summary: "Fix the login", Priority: tracker.High, Assignee: e.bo.ID, Labels: []string{"ux"}})
	e.issue("WEB-2", tracker.CreateIssue{Project: e.web, Type: tracker.Bug, Summary: "Cart 100% broken", Status: tracker.StatusInReview, Priority: tracker.Critical, Labels: []string{"checkout"}})
	e.issue("WEB-3", tracker.CreateIssue{Project: e.web, Summary: "Search_box", Status: tracker.StatusDone, Priority: tracker.Low, Assignee: e.ada.ID})
	e.issue("APP-1", tracker.CreateIssue{Project: e.app, Summary: "Crash on start", Status: tracker.StatusInProgress, Assignee: e.bo.ID})
	return e
}

func (e *listEnv) by(p people.Person) tabs.By {
	e.n++
	return tabs.By{Actor: p, Op: fmt.Sprint("seed", e.n), At: time.Now().Add(time.Duration(e.n-1000) * time.Minute)}
}

func (e *listEnv) result(cmd writer.ResultCommand) string {
	e.t.Helper()
	b, err := e.W.DoValue(context.Background(), func(tx *writer.Tx) ([]byte, error) { return cmd.ApplyResult(tx) })
	if err != nil {
		e.t.Fatalf("%T: %v", cmd, err)
	}
	var m map[string]string
	json.Unmarshal(b, &m)
	return m["id"]
}

func (e *listEnv) issue(key string, c tracker.CreateIssue) {
	e.t.Helper()
	c.By = e.by(e.ada)
	if c.Type == "" {
		c.Type = tracker.Task
	}
	e.ids[key] = e.result(c)
}

var rowKey = regexp.MustCompile(`<td class="tk-key">([A-Z]+-[0-9]+)</td>`)

// keys are the issue keys a list shows, in order.
func keys(page string) []string {
	var out []string
	for _, m := range rowKey.FindAllStringSubmatch(page, -1) {
		out = append(out, m[1])
	}
	return out
}

func (e *listEnv) list(c *http.Client, query string) []string {
	e.t.Helper()
	code, page := webtest.Get(e.t, c, e.URL+"/issues?"+query)
	if code != http.StatusOK {
		e.t.Fatalf("/issues?%s: %d", query, code)
	}
	return keys(page)
}

var viewsSignal = regexp.MustCompile(`_views: &#34;([^&]*)&#34;`)

// spec is the list view a page's stream watches.
func spec(t *testing.T, page string) string {
	t.Helper()
	m := viewsSignal.FindStringSubmatch(page)
	if m == nil || !strings.HasPrefix(m[1], "list:") {
		t.Fatalf("no list view in the page: %v", m)
	}
	return m[1]
}

// FR-51: filters of every kind, AND across filters and OR within one;
// values the model lacks filter nothing; "me" is the person who looks.
func TestListFilters(t *testing.T) {
	e := newList(t)
	c := webtest.Client(t)
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"", []string{"APP-1", "WEB-3", "WEB-2", "WEB-1"}}, // created, newest first (FR-53.1)
		{"project=WEB", []string{"WEB-3", "WEB-2", "WEB-1"}},
		{"project=web&type=bug&type=story", []string{"WEB-2", "WEB-1"}},
		{"status=todo&status=inreview", []string{"WEB-2", "WEB-1"}},
		{"category=inprogress", []string{"APP-1", "WEB-2"}},
		{"priority=critical&priority=low", []string{"WEB-3", "WEB-2"}},
		{"assignee=" + e.bo.ID, []string{"APP-1", "WEB-1"}},
		{"assignee=unassigned", []string{"WEB-2"}},
		{"assignee=unassigned&project=APP", nil},
		{"label=ux&label=checkout", []string{"WEB-2", "WEB-1"}},
		{"reporter=" + e.ada.ID + "&status=done", []string{"WEB-3"}},
		{"q=search_", []string{"WEB-3"}},                              // _ matches itself (FR-52.2)
		{"q=100%25", []string{"WEB-2"}},                               // so does %
		{"q=web-2", []string{"WEB-2"}},                                // a key (FR-52.1)
		{"status=open", []string{"APP-1", "WEB-3", "WEB-2", "WEB-1"}}, // no such status in the model: no filter
		{"created_from=2000-01-01&created_to=2000-01-02", nil},
		{"assignee=me", nil}, // a guest has nothing assigned
	} {
		if got := e.list(c, tc.query); !slices.Equal(got, tc.want) {
			t.Errorf("%q: %v, want %v", tc.query, got, tc.want)
		}
	}
	// FR-50.1: names, never ids; type, priority with its name, status
	// lozenge, relative times.
	_, page := webtest.Get(t, c, e.URL+"/issues?project=WEB")
	for _, want := range []string{e.bo.Name + "</span>", "Website", `class="tk-lozenge"`, ">In Review", "Critical", `class="tk-time"`, `aria-sort="descending"`, "3 issues"} {
		if !strings.Contains(page, want) {
			t.Errorf("the list lacks %q", want)
		}
	}
	// FR-50.3: a row's link opens the issue's side panel.
	if !strings.Contains(page, `data-issue-panel="`+e.ids["WEB-1"]+`"`) || !strings.Contains(page, `id="issue-panel"`) {
		t.Errorf("the list has no issue panel")
	}
	if strings.Contains(page, e.bo.ID+"<") || strings.Contains(page, ">"+e.bo.ID) {
		t.Errorf("the list shows a person id")
	}
}

// FR-53: sorts by severity, workflow order and names, ties by key; FR-54:
// 25 a page, a page past the end shows the last.
func TestListSortAndPages(t *testing.T) {
	e := newList(t)
	c := webtest.Client(t)
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"sort=priority&dir=desc", []string{"WEB-2", "WEB-1", "APP-1", "WEB-3"}},
		{"sort=status&dir=asc", []string{"WEB-1", "APP-1", "WEB-2", "WEB-3"}},
		{"sort=key&dir=asc", []string{"APP-1", "WEB-1", "WEB-2", "WEB-3"}},
		{"sort=assignee&dir=asc", []string{"WEB-3", "APP-1", "WEB-1", "WEB-2"}}, // Ada, Bo, Bo by key, unassigned last
		{"sort=nonsense", []string{"APP-1", "WEB-3", "WEB-2", "WEB-1"}},
	} {
		if got := e.list(c, tc.query); !slices.Equal(got, tc.want) {
			t.Errorf("%q: %v, want %v", tc.query, got, tc.want)
		}
	}
	for i := range 26 {
		e.issue(fmt.Sprint("WEB-", i+4), tracker.CreateIssue{Project: e.web, Summary: fmt.Sprint("More ", i)})
	}
	if got := e.list(c, ""); len(got) != 25 {
		t.Errorf("page 1 has %d issues", len(got))
	}
	_, page := webtest.Get(t, c, e.URL+"/issues?page=99")
	if got := keys(page); len(got) != 5 || !strings.Contains(page, "Page 2 of 2") || !strings.Contains(page, "30 issues") {
		t.Errorf("a page past the end: %v", got)
	}
	_, page = webtest.Get(t, c, e.URL+"/issues?sort=key&dir=asc")
	if !strings.Contains(page, html.EscapeString(`href="/issues?dir=asc&page=2&sort=key"`)) && !strings.Contains(page, `href="/issues?dir=asc&amp;page=2&amp;sort=key"`) {
		t.Errorf("the next page's link keeps the sort")
	}
}

// FR-55.1: the URL holds filter, sort and page; the view's name holds
// them as the URL does, and two tabs that see the same list share one key.
func TestListURL(t *testing.T) {
	e := newList(t)
	c := webtest.Client(t)
	_, page := webtest.Get(t, c, e.URL+"/issues?status=todo&assignee=me&sort=priority&dir=desc&page=2&q=login")
	for _, want := range []string{`value="login"`, `name="status" value="todo" checked`, `name="assignee" options=`, `value="[&#34;me&#34;]"`, `name="sort" value="priority"`, `name="dir" value="desc"`, `name="page" value="2"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the form lacks %s", want)
		}
	}
	a := spec(t, page)
	_, again := webtest.Get(t, c, e.URL+"/issues?q=login&page=2&dir=desc&sort=priority&assignee=me&status=todo")
	if b := spec(t, again); a != b {
		t.Errorf("the same list, two names: %s and %s", a, b)
	}
	// A view with a malformed name is refused.
	if code, _ := webtest.Get(t, c, e.URL+"/stream?tab=t1&v=list:!!"); code != http.StatusBadRequest {
		t.Errorf("a malformed list: %d", code)
	}
}

// listener is a browser with a list open in a tab.
type listener struct {
	c      *http.Client
	tab    string
	view   string
	events <-chan string
}

func (e *listEnv) open(t *testing.T, c *http.Client, tab, query string) *listener {
	t.Helper()
	_, page := webtest.Get(t, c, e.URL+"/issues?"+query)
	l := &listener{c: c, tab: tab, view: spec(t, page)}
	l.events = webtest.Listen(t, c, e.URL+"/stream?tab="+tab+"&v="+url.QueryEscape(l.view), nil)
	webtest.Next(t, l.events, `id="issue-list"`)
	return l
}

// MP-03.1: a change by another person that moves an issue into or out of
// the filter shows on the open list in the next frames.
func TestListLive(t *testing.T) {
	e := newList(t)
	l := e.open(t, webtest.Client(t), "tab1", "status=todo")
	far := int64(1 << 40)
	e.Do(t, tracker.SetStatus{By: e.by(e.bo), Issue: e.ids["APP-1"], Seen: far, Status: tracker.StatusToDo})
	frame := webtest.Next(t, l.events, `id="issue-list"`)
	if got := keys(frame); !slices.Equal(got, []string{"APP-1", "WEB-1"}) {
		t.Errorf("moved in: %v", got)
	}
	e.Do(t, tracker.SetStatus{By: e.by(e.bo), Issue: e.ids["WEB-1"], Seen: far, Status: tracker.StatusDone})
	frame = webtest.Next(t, l.events, `id="issue-list"`)
	if got := keys(frame); !slices.Equal(got, []string{"APP-1"}) {
		t.Errorf("moved out: %v", got)
	}
	// MP-03.2: a change the list does not show sends no frame of it.
	e.Do(t, tracker.SetPoints{By: e.by(e.bo), Issue: e.ids["WEB-2"], Seen: far, Points: new(3)})
	webtest.Silent(t, l.events, `id="issue-list"`, 200*time.Millisecond)
}

// MP-11: tabs that see the same list watch one view, which the hub renders
// once per change however many tabs watch it; "me" is resolved, so one
// guest's "me" and another's naming that guest share it.
func TestListOneRender(t *testing.T) {
	e := newList(t)
	first := webtest.Client(t)
	_, home := webtest.Get(t, first, e.URL+"/")
	me := guestID.FindStringSubmatch(home)[1]
	ls := []*listener{e.open(t, first, "tab1", "assignee=me&assignee=unassigned")}
	for i := range 3 {
		ls = append(ls, e.open(t, webtest.Client(t), fmt.Sprint("tab", i+2), "assignee=unassigned&assignee="+me))
	}
	var key string
	for k := range e.Hub.Stats().RendersByKey {
		if strings.HasPrefix(k, "list:") {
			if key != "" {
				t.Fatalf("two list views: %s and %s", key, k)
			}
			key = k
		}
	}
	before := e.Hub.Stats().RendersByKey[key]
	e.Do(t, tracker.SetSummary{By: e.by(e.bo), Issue: e.ids["WEB-2"], Seen: 1 << 40, Summary: "Cart broken for good"})
	for _, l := range ls {
		webtest.Next(t, l.events, "Cart broken for good")
	}
	if n := e.Hub.Stats().RendersByKey[key] - before; n != 1 {
		t.Errorf("MP-11.1: %d renders of one change for %d tabs", n, len(ls))
	}
}

// FR-52.2, FR-55: changing the filter moves the open stream to the new
// list without a page load; the tab keeps it, and /issues shows it again.
func TestListChange(t *testing.T) {
	e := newList(t)
	c := webtest.Client(t)
	l := e.open(t, c, "tab1", "status=todo")
	code, body := webtest.Post(t, c, e.URL+"/cmd/list/view", fmt.Sprintf(`{"tab":"tab1","view":%q,"query":"project=APP"}`, l.view))
	if code != http.StatusOK || !strings.Contains(body, `"_views":"list:`) {
		t.Fatalf("change: %d %s", code, body)
	}
	frame := webtest.Next(t, l.events, `id="issue-list"`)
	if got := keys(frame); !slices.Equal(got, []string{"APP-1"}) {
		t.Errorf("the new list: %v", got)
	}
	// The old list's view is gone from the stream: a change to WEB-1 (todo)
	// sends no frame of it, a change to APP-1 a frame of the new one.
	e.Do(t, tracker.SetSummary{By: e.by(e.bo), Issue: e.ids["WEB-1"], Seen: 1 << 40, Summary: "Fix the login now"})
	webtest.Silent(t, l.events, "Fix the login now", 200*time.Millisecond)
	e.Do(t, tracker.SetSummary{By: e.by(e.bo), Issue: e.ids["APP-1"], Seen: 1 << 40, Summary: "Crash on start, again"})
	webtest.Next(t, l.events, "Crash on start, again")
	if n := e.Hub.Stats().Views; n == 0 {
		t.Errorf("no views")
	}
	// FR-55.2: /issues without a query shows the tab's last list.
	resp, err := c.Get(e.URL + "/issues")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/issues?project=APP" {
		t.Errorf("back to the list: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	// A query too long for a view's name is stored, and its name holds a hash.
	long := "q=" + strings.Repeat("x", 190)
	code, body = webtest.Post(t, c, e.URL+"/cmd/list/view", `{"tab":"tab1","view":"","query":"`+long+`"}`)
	if code != http.StatusOK || !strings.Contains(body, `"_views":"list:.`) {
		t.Fatalf("a long query: %d %s", code, body)
	}
	_, page := webtest.Get(t, c, e.URL+"/issues?"+long)
	if !strings.HasPrefix(spec(t, page), "list:.") {
		t.Errorf("the stored list's page")
	}
	if code, _ := webtest.Post(t, c, e.URL+"/cmd/list/view", `{"tab":"no tab!","query":""}`); code != http.StatusBadRequest {
		t.Errorf("a bad tab: %d", code)
	}
}

// FR-152.1: the list's whole result as CSV, a person's text never read
// as a formula.
func TestListCSV(t *testing.T) {
	e := newList(t)
	e.issue("WEB-4", tracker.CreateIssue{Project: e.web, Summary: "=HYPERLINK(1)"})
	code, body := webtest.Get(t, webtest.Client(t), e.URL+"/issues.csv?project=WEB&sort=key&dir=asc")
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if code != http.StatusOK || len(lines) != 5 || !strings.HasPrefix(lines[1], "WEB-1,Story,Fix the login,High,To Do,"+e.bo.Name+",") ||
		!strings.Contains(lines[4], `'=HYPERLINK(1)`) {
		t.Errorf("csv: %d\n%s", code, body)
	}
}

// FR-56: save the list's filter with a name, find it under Filters, star
// it; only its owner renames and deletes it.
func TestSavedFilters(t *testing.T) {
	e := newList(t)
	owner, other := webtest.Client(t), webtest.Client(t)
	webtest.Get(t, owner, e.URL+"/")
	webtest.Get(t, other, e.URL+"/")
	post := func(c *http.Client, path, body string) int {
		t.Helper()
		code, _ := webtest.Post(t, c, e.URL+path, body)
		return code
	}
	if code := post(owner, "/cmd/filters/save", `{"tab":"t1","op":"o1","name":"My bugs","query":"type=bug&assignee=me&sort=key&dir=asc"}`); code != http.StatusNoContent {
		t.Fatalf("save: %d", code)
	}
	_, page := webtest.Get(t, other, e.URL+"/filters")
	m := regexp.MustCompile(`data-id="([0-9a-f-]{36})"`).FindStringSubmatch(page)
	if m == nil || !strings.Contains(page, "My bugs") || !strings.Contains(page, "Type: Bug · Assignee: me") ||
		!strings.Contains(page, html.EscapeString("/issues?assignee=me&dir=asc&sort=key&type=bug")) {
		t.Fatalf("the filters page:\n%s", page)
	}
	id := m[1]
	if strings.Contains(page, `data-act="rename"`) {
		t.Errorf("someone else may rename it")
	}
	if code := post(other, "/cmd/filters/rename", `{"tab":"t2","op":"o2","filter":"`+id+`","name":"Mine now"}`); code != http.StatusForbidden {
		t.Errorf("FR-56.3: someone else renames: %d", code)
	}
	if code := post(other, "/cmd/filters/star", `{"tab":"t2","op":"o3","filter":"`+id+`","star":true}`); code != http.StatusNoContent {
		t.Errorf("star: %d", code)
	}
	_, page = webtest.Get(t, other, e.URL+"/filters")
	if !strings.Contains(page, `aria-pressed="true"`) {
		t.Errorf("not starred")
	}
	_, page = webtest.Get(t, other, e.URL+"/issues")
	if !strings.Contains(page, "My bugs") {
		t.Errorf("the list's Filters menu lacks it")
	}
	if code := post(owner, "/cmd/filters/rename", `{"tab":"t1","op":"o4","filter":"`+id+`","name":"Bugs of mine"}`); code != http.StatusNoContent {
		t.Errorf("rename: %d", code)
	}
	if code := post(owner, "/cmd/filters/delete", `{"tab":"t1","op":"o5","filter":"`+id+`"}`); code != http.StatusNoContent {
		t.Errorf("delete: %d", code)
	}
	if _, page = webtest.Get(t, other, e.URL+"/filters"); strings.Contains(page, "Bugs of mine") || !strings.Contains(page, "No saved filters") {
		t.Errorf("deleted, still listed")
	}
	if code := post(owner, "/cmd/filters/star", `{"tab":"t1","op":"o6","filter":"nope"}`); code != http.StatusBadRequest {
		t.Errorf("a bad id: %d", code)
	}
}
