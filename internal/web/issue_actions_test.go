package web_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// issueEnv is the server with the probes' data, and a guest's client
// with its tab's stream open.
type issueEnv struct {
	*webtest.Env
	c      *http.Client
	events <-chan string
	n      int
}

func newIssueEnv(t *testing.T) *issueEnv {
	t.Helper()
	e := webtest.Start(t)
	seedProbes(t, e)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/WEB-1")
	ev := webtest.Listen(t, c, e.URL+"/stream?tab=tab1&v=issue:"+web.ProbeIssue, nil)
	webtest.Next(t, ev, `"_outcomes"`)
	return &issueEnv{Env: e, c: c, events: ev}
}

// post sends an action on WEB-1's field and returns its status and op id.
func (e *issueEnv) post(t *testing.T, field, body string) (int, string) {
	t.Helper()
	e.n++
	op := fmt.Sprintf("op%d", e.n)
	status, _ := webtest.Post(t, e.c, e.URL+"/cmd/issues/"+web.ProbeIssue+"/"+field, `{"tab":"tab1","op":"`+op+`"`+body+`}`)
	return status, op
}

// outcome waits for the op's outcome on the tab's stream.
func (e *issueEnv) outcome(t *testing.T, op string) string {
	t.Helper()
	ev := webtest.Next(t, e.events, `"`+op+`":{`)
	i := strings.Index(ev, `"`+op+`":{`)
	return ev[i:]
}

func (e *issueEnv) detail(t *testing.T) tracker.Detail {
	t.Helper()
	d, err := tracker.IssueDetail(context.Background(), e.DB.R, web.ProbeIssue)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// FR-14, FR-16 to FR-18, FR-23, FR-25 to FR-27, FR-65, MP-09: every
// field's action answers 204 and its outcome arrives on the stream.
func TestIssueActionsAccepted(t *testing.T) {
	e := newIssueEnv(t)
	v := e.detail(t).Version
	cases := []struct{ field, body, want string }{
		{"summary", `,"seen":%d,"value":"A new summary"`, ""},
		{"priority", `,"seen":%d,"value":"critical"`, ""},
		{"status", `,"seen":%d,"value":"inreview"`, ""},
		{"assignee", `,"seen":%d,"value":"me"`, ""},
		{"due", `,"seen":%d,"value":"2026-12-24"`, ""},
		{"points", `,"seen":%d,"value":"5"`, ""},
		{"estimate", `,"seen":%d,"value":"2h 30m"`, ""},
		{"labels", `,"value":"safari","ref":"add"`, ""},
		{"watch", `,"value":""`, ""},
		{"comment", `,"value":"Hello @s-priya, see [[WEB-1]]"`, ""},
		{"subtask", `,"value":"A subtask"`, ""},
		{"link", `,"value":"APP-1","ref":"relates"`, ""},
		{"type", `,"seen":%d,"value":"bug"`, ""},
	}
	for _, c := range cases {
		body := c.body
		if strings.Contains(body, "%d") {
			body = fmt.Sprintf(body, v)
		}
		status, op := e.post(t, c.field, body)
		if status != http.StatusNoContent {
			t.Fatalf("%s: %d", c.field, status)
		}
		if o := e.outcome(t, op); !strings.HasPrefix(o, `"`+op+`":{"ok":true`) {
			t.Errorf("%s: %s", c.field, o[:min(len(o), 200)])
		}
		v = e.detail(t).Version
	}
	d := e.detail(t)
	if d.Summary != "A new summary" || d.Priority != tracker.Critical || d.Status != "inreview" || d.Due != "2026-12-24" ||
		d.Points == nil || *d.Points != 5 || d.Estimate == nil || *d.Estimate != 150 || len(d.Labels) != 1 || len(d.Children) != 1 || len(d.Links) != 1 {
		t.Errorf("the issue is %+v", d)
	}
	comments, _ := tracker.Comments(context.Background(), e.DB.R, web.ProbeIssue)
	if len(comments) != 2 || !strings.Contains(string(comments[1].Body), `"mention"`) {
		t.Errorf("comments %+v", comments)
	}
	// The own comment: edit, then delete.
	id := comments[1].ID
	_, op := e.post(t, "cedit", `,"value":"Edited","ref":"`+id+`"`)
	if o := e.outcome(t, op); !strings.Contains(o, `"ok":true`) {
		t.Errorf("cedit: %s", o)
	}
	_, op = e.post(t, "cdelete", `,"ref":"`+id+`"`)
	// A frame of the issue's view carries the change (MP-03), before the
	// outcome in the same tick.
	webtest.Next(t, e.events, `Comment deleted`)
	if o := e.outcome(t, op); !strings.Contains(o, `"ok":true`) {
		t.Errorf("cdelete: %s", o)
	}
}

// FR-14.2, FR-19.2, FR-22.2, MP-09.2: refusals are outcomes with words,
// next to their field; nothing changes.
func TestIssueActionsRefused(t *testing.T) {
	e := newIssueEnv(t)
	before := e.detail(t)
	cases := []struct{ field, body, msg string }{
		{"summary", `,"seen":%d,"value":"  "`, "A summary has 1 to 255 characters."},
		{"points", `,"seen":%d,"value":"lots"`, "Story points are a number from 0 to 100."},
		{"estimate", `,"seen":%d,"value":"soon"`, "Write an estimate as"},
		{"link", `,"value":"NOPE-9","ref":"blocks"`, "No issue has the key NOPE-9."},
		{"link", `,"value":"WEB-1","ref":"blocks"`, ""},
		{"comment", `,"value":"   "`, "Write a comment first."},
		{"status", `,"seen":%d,"value":"nowhere"`, "This project's workflow has no such status."},
	}
	for _, c := range cases {
		body := c.body
		if strings.Contains(body, "%d") {
			body = fmt.Sprintf(body, before.Version)
		}
		status, op := e.post(t, c.field, body)
		if status != http.StatusNoContent {
			t.Fatalf("%s: %d", c.field, status)
		}
		o := e.outcome(t, op)
		if !strings.HasPrefix(o, `"`+op+`":{"ok":false,"msg":"`+c.msg) {
			t.Errorf("%s: %s", c.field, o[:min(len(o), 200)])
		}
	}
	if after := e.detail(t); after.Version != before.Version {
		t.Errorf("a refusal changed the issue: version %d to %d", before.Version, after.Version)
	}
}

// MP-08.3: a change against an old version of a field someone else
// changed is refused with the conflict's data; resending with its version
// goes through.
func TestIssueConflict(t *testing.T) {
	e := newIssueEnv(t)
	seen := e.detail(t).Version
	other := webtest.Client(t)
	webtest.Get(t, other, e.URL+"/WEB-1")
	if status, _ := webtest.Post(t, other, e.URL+"/cmd/issues/"+web.ProbeIssue+"/priority", fmt.Sprintf(`{"tab":"tab2","op":"x1","seen":%d,"value":"high"}`, seen)); status != http.StatusNoContent {
		t.Fatal(status)
	}
	_, op := e.post(t, "priority", fmt.Sprintf(`,"seen":%d,"value":"critical"`, seen))
	o := e.outcome(t, op)
	for _, want := range []string{`"ok":false`, `changed the priority to High`, `Your Critical was not saved.`, `"data":{"kind":"conflict","field":"priority"`, `"theirs":"High","yours":"Critical"`} {
		if !strings.Contains(o, want) {
			t.Errorf("no %s in %s", want, o)
		}
	}
	now := e.detail(t).Version
	if !strings.Contains(o, fmt.Sprintf(`"version":%d`, now)) {
		t.Errorf("the conflict's version is not %d: %s", now, o)
	}
	_, op = e.post(t, "priority", fmt.Sprintf(`,"seen":%d,"value":"critical"`, now))
	if o := e.outcome(t, op); !strings.Contains(o, `"ok":true`) {
		t.Errorf("use anyway: %s", o)
	}
	if p := e.detail(t).Priority; p != tracker.Critical {
		t.Errorf("priority %s", p)
	}
}

// NFR-16: malformed input answers 400, an unknown issue 404, never 500.
func TestIssueActionsMalformed(t *testing.T) {
	e := newIssueEnv(t)
	url := e.URL + "/cmd/issues/"
	cases := []struct {
		path, body string
		want       int
	}{
		{web.ProbeIssue + "/priority", `not json`, 400},
		{web.ProbeIssue + "/priority", `{"tab":"t","op":""}`, 400},
		{"nope/priority", `{"tab":"t","op":"a1"}`, 400},
		{web.ProbeIssue + "/nofield", `{"tab":"t","op":"a2"}`, 400},
		{web.ProbeIssue + "/labels", `{"tab":"t","op":"a3","value":"x","ref":"sideways"}`, 400},
		{web.ProbeIssue + "/assignee", `{"tab":"t","op":"a4","value":"robert'); drop"}`, 400},
		{web.ProbeIssue + "/cedit", `{"tab":"t","op":"a5","ref":"x"}`, 400},
		{web.ProbeIssue + "/typing", `{"tab":"t","op":"a6","field":"nose"}`, 400},
		{"01920000-0000-7000-8000-00000000ffff/priority", `{"tab":"t","op":"a7","value":"low"}`, 404},
		{web.ProbeIssue + "/priority", `{"tab":"t","op":"a8","seen":"seven"}`, 400},
		{"create", `{"tab":"t","op":"a9","project":5}`, 400},
	}
	for _, c := range cases {
		status, body := webtest.Post(t, e.c, url+c.path, c.body)
		if status != c.want {
			t.Errorf("%s %s: %d %s, want %d", c.path, c.body, status, body, c.want)
		}
	}
	for _, f := range []string{"summary", "description", "type", "status", "priority", "assignee", "parent", "due", "points", "estimate", "remaining", "sprint", "labels", "components", "watch", "link", "unlink", "subtask", "comment", "cedit", "cdelete", "chide", "delete", "typing"} {
		for _, body := range []string{`{}`, `{"tab":"t","op":"z` + f + `"}`, `{"tab":"t","op":"y` + f + `","value":"` + strings.Repeat("x", 300) + `","ref":"` + strings.Repeat("y", 70) + `","seen":-1}`} {
			if status, b := webtest.Post(t, e.c, url+web.ProbeIssue+"/"+f, body); status >= 500 {
				t.Errorf("%s %s: %d %s", f, body, status, b)
			}
		}
	}
}

// NFR-01, 3.2: a Viewer of a closed project is refused every change with
// 403, and may still comment.
func TestIssueViewer(t *testing.T) {
	e := webtest.Start(t)
	seedProbes(t, e)
	closed, issue := tracker.NewID(), tracker.NewID()
	ada := people.Seeded[0]
	e.Do(t, tracker.CreateProject{By: byOf(ada, "c1"), ID: closed, Key: "SEC", Name: "Secret"})
	e.Do(t, tracker.CreateIssue{By: byOf(ada, "c2"), ID: issue, Project: closed, Type: tracker.Task, Summary: "Hidden"})
	id := e.Account(t, "vera@example.com", "correct horse battery", people.NoRole)
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		return access.SetMember(tx, access.ProjectScope(closed), id, access.Viewer, ada.ID, time.Now())
	}))
	vera, guest := webtest.Client(t), webtest.Client(t)
	e.Login(t, vera, "vera@example.com", "correct horse battery")
	if status, _ := webtest.Get(t, guest, e.URL+"/SEC-1"); status != http.StatusNotFound {
		t.Errorf("a guest sees a closed project's issue: %d", status)
	}
	if status, body := webtest.Get(t, vera, e.URL+"/SEC-1"); status != http.StatusOK || !strings.Contains(body, `\"ro\":true`) && !strings.Contains(body, `&#34;ro&#34;:true`) && !strings.Contains(body, `"ro":true`) {
		t.Errorf("the viewer's page: %d, read-only %v", status, strings.Contains(body, `ro&#34;:true`))
	}
	post := func(field, body string) int {
		status, _ := webtest.Post(t, vera, e.URL+"/cmd/issues/"+issue+"/"+field, body)
		return status
	}
	for _, f := range []string{"priority", "summary", "status", "delete"} {
		if status := post(f, `{"tab":"t","op":"v`+f+`","seen":1,"value":"high"}`); status != http.StatusForbidden {
			t.Errorf("a viewer's %s: %d", f, status)
		}
	}
	if status := post("comment", `{"tab":"t","op":"vc","value":"Viewers comment."}`); status != http.StatusNoContent {
		t.Errorf("a viewer's comment: %d", status)
	}
	if status, _ := webtest.Post(t, guest, e.URL+"/cmd/issues/"+issue+"/priority", `{"tab":"t","op":"g1","seen":1,"value":"high"}`); status != http.StatusNotFound {
		t.Errorf("a guest's change to a closed project: %d", status)
	}
}

// FR-12: keys open the issue without regard to case and redirect to the
// canonical one; an old key redirects to the new; an unknown key is 404
// with a search box; a deleted one 410 with who and when; old URLs
// redirect (FR-12.3).
func TestIssueURLs(t *testing.T) {
	e := webtest.Start(t)
	seedProbes(t, e)
	c := webtest.Client(t)
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	get := func(path string) (int, string, string) {
		res, err := c.Get(e.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var b strings.Builder
		buf := make([]byte, 1<<16)
		for {
			n, err := res.Body.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		return res.StatusCode, res.Header.Get("Location"), b.String()
	}
	if s, _, body := get("/WEB-1"); s != 200 || !strings.Contains(body, "Probe a1") || !strings.Contains(body, `id="presence-issue-`+web.ProbeIssue+`"`) {
		t.Errorf("/WEB-1: %d", s)
	}
	for path, want := range map[string]string{"/web-1": "/WEB-1", "/WEB-3": "/APP-1", "/ticket/" + web.ProbeIssue: "/WEB-1", "/dashboard/" + web.ProbeIssue: "/WEB-1"} {
		if s, loc, _ := get(path); s != http.StatusMovedPermanently || loc != want {
			t.Errorf("%s: %d %s, want %s", path, s, loc, want)
		}
	}
	if s, _, body := get("/WEB-404"); s != 404 || !strings.Contains(body, `role="search"`) || !strings.Contains(body, "No issue WEB-404") {
		t.Errorf("/WEB-404: %d", s)
	}
	if s, _, body := get("/WEB-2"); s != 410 || !strings.Contains(body, "WEB-2 was deleted by "+people.Seeded[0].Name) || !strings.Contains(body, time.Now().In(time.Local).Format("January")) {
		t.Errorf("/WEB-2: %d %s", s, body[strings.Index(body, "<main"):][:400])
	}
	if s, _, _ := get("/ticket/nope"); s != 404 {
		t.Errorf("/ticket/nope: %d", s)
	}
}

// FR-10: the create dialog is on every page; a refusal names its field
// and keeps the dialog; an issue made answers where to go.
func TestCreateIssue(t *testing.T) {
	e := newIssueEnv(t)
	_, home := webtest.Get(t, e.c, e.URL+"/privacy")
	if !strings.Contains(home, `id="create"`) || !strings.Contains(home, "Create an issue") || !strings.Contains(home, `&#34;value&#34;:&#34;`+probeWeb+`&#34;`) {
		t.Fatal("no create dialog on /privacy")
	}
	post := func(op, body string) (int, string) {
		return webtest.Post(t, e.c, e.URL+"/cmd/issues/create", `{"tab":"tab1","op":"`+op+`"`+body+`}`)
	}
	if s, _ := post("n1", `,"project":"`+probeWeb+`","type":"task","summary":""`); s != 204 {
		t.Fatal(s)
	}
	if o := e.outcome(t, "n1"); !strings.Contains(o, `"field":"summary"`) {
		t.Errorf("empty summary: %s", o)
	}
	if s, _ := post("n2", `,"project":"`+probeWeb+`","type":"story","summary":"x","parent":"ZZ-1"`); s != 204 {
		t.Fatal(s)
	}
	if o := e.outcome(t, "n2"); !strings.Contains(o, `"msg":"No issue has the key ZZ-1.","field":"parent"`) {
		t.Errorf("unknown parent: %s", o)
	}
	s, body := post("n3", `,"project":"`+probeWeb+`","type":"bug","summary":"Made in the dialog","priority":"","labels":"a, b","description":"It *breaks*."`)
	if s != 200 || !strings.Contains(body, `"_goto":"/WEB-4"`) {
		t.Errorf("create: %d %s", s, body)
	}
	s, body = post("n4", `,"project":"`+probeWeb+`","type":"task","summary":"Stays in the panel","stay":true`)
	if s != 200 || !strings.Contains(body, ` WEB-5"`) {
		t.Errorf("create in the panel: %d %s", s, body)
	}
	k, err := tracker.IssueByKey(context.Background(), e.DB.R, "WEB-4")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := tracker.IssueDetail(context.Background(), e.DB.R, k.Issue)
	if d.Priority != tracker.Medium || len(d.Labels) != 2 || !strings.Contains(string(d.Description), `"em"`) {
		t.Errorf("WEB-4: %+v", d.Issue)
	}
}
