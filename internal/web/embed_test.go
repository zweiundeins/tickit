package web_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"
	"github.com/zweiundeins/prosemirror-go"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web/webtest"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// embedWorld is a page of an open space, an open project WEB with a bug
// and a task, a closed project SEC with a task, and a logged-in member of
// SEC beside the guest who wrote the page.
type embedWorld struct {
	e              *webtest.Env
	guest, member  *http.Client
	page, web, sec string
	bug            string // WEB-1's id
	ada            people.Person
}

func newEmbedWorld(t *testing.T) *embedWorld {
	w := &embedWorld{e: startWiki(t), guest: webtest.Client(t), member: webtest.Client(t)}
	e := w.e
	webtest.Get(t, w.guest, e.URL+"/")
	space, home := wikiSpace(t, e, w.guest, "ENG")
	w.page = wikiPage(t, e, w.guest, space, home, "Notes")
	w.ada = people.Seeded[0]
	e.Do(t, people.SeedPeople{At: time.Now()})
	staff := e.Account(t, "staff@example.com", "a long password here", people.NoRole)
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		create := func(cmd interface {
			ApplyResult(*writer.Tx) ([]byte, error)
		}) string {
			b, err := cmd.ApplyResult(tx)
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]string
			json.Unmarshal(b, &m)
			return m["id"]
		}
		w.web = create(tracker.CreateProject{By: wikiBy(w.ada, "p1"), Key: "WEB", Name: "Web", Open: true})
		w.sec = create(tracker.CreateProject{By: wikiBy(w.ada, "p2"), Key: "SEC", Name: "Secret"})
		w.bug = create(tracker.CreateIssue{By: wikiBy(w.ada, "i1"), Project: w.web, Type: tracker.Bug, Summary: "Fix the login"})
		create(tracker.CreateIssue{By: wikiBy(w.ada, "i2"), Project: w.web, Type: tracker.Task, Summary: "Write the docs"})
		create(tracker.CreateIssue{By: wikiBy(w.ada, "i3"), Project: w.sec, Type: tracker.Task, Summary: "The secret plan"})
		return access.SetMember(tx, access.ProjectScope(w.sec), staff, access.Member, w.ada.ID, time.Now())
	}))
	e.Login(t, w.member, "staff@example.com", "a long password here")
	return w
}

// commit sends one commit of steps to the page as the guest's tab.
func (w *embedWorld) commit(t *testing.T, ref, steps string) {
	t.Helper()
	h := head(t, w.e, w.page)
	act(t, w.e, w.guest, "/cmd/pages/"+w.page+"/open", "tab1", map[string]any{"base": h.Version, "client": "0123456789ab"})
	code, body := webtest.Post(t, w.guest, w.e.URL+"/cmd/pages/"+w.page+"/commit",
		fmt.Sprintf(`{"tab":"tab1","epoch":%q,"base":%d,"ref":%q,"client":"0123456789ab","steps":%s}`, h.Epoch, h.Version, ref, steps))
	if code != http.StatusNoContent {
		t.Fatalf("commit %s: %d %s", ref, code, body)
	}
}

func listStep(attrs string) string {
	return `[{"stepType":"replace","from":0,"to":0,"slice":{"content":[{"type":"issue_list","attrs":` + attrs + `}]}}]`
}

var rowRe = regexp.MustCompile(`<tr data-key="([A-Z]+-\d+)">`)

func rows(frame string) string {
	var keys []string
	for _, m := range rowRe.FindAllStringSubmatch(frame, -1) {
		keys = append(keys, m[1])
	}
	return strings.Join(keys, " ")
}

// FR-104.1: a page's issue list shows key, summary, status and assignee
// of the issues its filter matches, live: a status change shows in the
// next frame. A reader sees the rows of the projects they may read only
// (NFR-01), and readers who read the same projects share one render
// (MP-11).
func TestIssueListOnPage(t *testing.T) {
	w := newEmbedWorld(t)
	e := w.e
	w.commit(t, "l1", listStep(`{"filter":"type=bug&type=task&sort=key&dir=asc"}`))

	other := webtest.Client(t)
	webtest.Get(t, other, e.URL+"/")
	guest := webtest.Listen(t, w.guest, e.URL+"/stream?tab=g1&v=pagedoc:"+w.page, nil)
	guest2 := webtest.Listen(t, other, e.URL+"/stream?tab=g2&v=pagedoc:"+w.page, nil)
	member := webtest.Listen(t, w.member, e.URL+"/stream?tab=m1&v=pagedoc:"+w.page, nil)
	frame := webtest.Next(t, guest, `class="issue-list"`)
	if got := rows(frame); got != "WEB-1 WEB-2" {
		t.Errorf("FR-104.1: the guest's rows: %q", got)
	}
	for _, want := range []string{"Fix the login", `data-cat="todo"`, "Unassigned", `Issues matching <code>type=bug&amp;type=task&amp;sort=key&amp;dir=asc</code>`, `href="/issues?`} {
		if !strings.Contains(frame, want) {
			t.Errorf("FR-104.1: the list lacks %q", want)
		}
	}
	webtest.Next(t, guest2, `<tr data-key="WEB-1">`)
	if got := rows(webtest.Next(t, member, `class="issue-list"`)); got != "SEC-1 WEB-1 WEB-2" {
		t.Errorf("NFR-01: the member's rows: %q", got)
	}
	_, html := webtest.Get(t, w.guest, e.URL+wiki.PageURL("ENG", w.page, "Notes"))
	if got := rows(html); got != "WEB-1 WEB-2" || strings.Contains(html, "The secret plan") {
		t.Errorf("NFR-01: the page as the guest loads it: %q", got)
	}

	e.Do(t, tracker.SetStatus{By: wikiBy(w.ada, "s1"), Issue: w.bug, Seen: 1 << 40, Status: tracker.StatusInReview})
	if f := webtest.Next(t, guest, `data-cat="progress"`); !strings.Contains(f, "In Review") || rows(f) != "WEB-1 WEB-2" {
		t.Errorf("FR-104.1: after the status change: %s", rows(f))
	}
	webtest.Next(t, member, `data-cat="progress"`)
	keys := 0
	for k := range e.Hub.Stats().RendersByKey {
		if strings.HasPrefix(k, "pagedoc:"+w.page) {
			keys++
		}
	}
	if keys != 2 {
		t.Errorf("MP-11: %d keys of the page's reader view for two sets of rights", keys)
	}

	// A saved filter, by name; a typed filter that excludes the issue now.
	var saved string
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		b, err := tracker.SaveFilter{By: wikiBy(w.ada, "f1"), Name: "Bugs", Filter: tracker.Filter{Types: []tracker.IssueType{tracker.Bug}}}.ApplyResult(tx)
		var m map[string]string
		json.Unmarshal(b, &m)
		saved = m["id"]
		return err
	}))
	w.commit(t, "l2", listStep(`{"savedFilter":"`+saved+`","limit":1}`))
	frame = webtest.Next(t, guest, `Issues of the saved filter <strong>Bugs</strong>, at most 1`)
	if !strings.Contains(frame, `<tr data-key="WEB-1">`) {
		t.Errorf("FR-56.2: the saved filter's rows: %s", rows(frame))
	}
	e.Do(t, tracker.RenameFilter{By: wikiBy(w.ada, "f2"), Filter: saved, Name: "Our bugs"})
	webtest.Next(t, guest, `<strong>Our bugs</strong>`)

	// The editor's lists element: every list's rows by its key, and the
	// saved filters.
	if code, html := webtest.Get(t, w.guest, e.URL+wiki.EditURL("ENG", w.page)); code != http.StatusOK || !strings.Contains(html, `lists="wk-lists"`) || !strings.Contains(html, "pagelists:"+w.page) {
		t.Errorf("the editor page has no lists element: %d", code)
	}
	lists := webtest.Listen(t, w.guest, e.URL+"/stream?tab=g3&v=pagelists:"+w.page, nil)
	frame = webtest.Next(t, lists, `id="wk-lists"`)
	for _, want := range []string{`data-list="` + saved + `||1"`, `data-list="|type=bug&amp;type=task&amp;sort=key&amp;dir=asc|10"`, `<li data-id="` + saved + `">Our bugs</li>`} {
		if !strings.Contains(frame, want) {
			t.Errorf("the lists element lacks %s", want)
		}
	}

	// Versions show the filter, not rows.
	if code, _ := act(t, e, w.guest, "/cmd/pages/save-version", "tab1", map[string]any{"page": w.page}); code != http.StatusNoContent {
		t.Fatalf("save version: %d", code)
	}
	var n int
	e.DB.R.QueryRow(`SELECT max(number) FROM page_versions WHERE page_id = ?`, w.page).Scan(&n)
	_, html = webtest.Get(t, w.guest, e.URL+wiki.VersionURL("ENG", w.page, n))
	if !strings.Contains(html, `Issues of the saved filter <strong>Our bugs</strong>`) || strings.Contains(html, "issue-list__table") {
		t.Error("a version shows an issue list's rows, or not its filter")
	}
}

// FR-105.1: "Create issue" on selected text makes a task with the text as
// its summary, links the page to it, and in view mode puts its chip in
// place of the text; in the editor the editor does (an editor commit).
func TestCreateIssueFromPage(t *testing.T) {
	w := newEmbedWorld(t)
	e := w.e
	w.commit(t, "t1", `[{"stepType":"replace","from":0,"to":0,"slice":{"content":[{"type":"paragraph","content":[{"type":"text","text":"Ship the new  login page, then the new login page tests."}]}]}}]`)
	create := func(fields map[string]any) (int, string) {
		fields["page"] = w.page
		if _, ok := fields["project"]; !ok {
			fields["project"] = w.web
		}
		return act(t, e, w.guest, "/cmd/pages/issue", "tab1", fields)
	}
	code, body := create(map[string]any{"summary": " new login\npage ", "find": "new login page", "block": 0, "nth": 1})
	if code != http.StatusOK || !strings.Contains(body, `"key":"WEB-3"`) || !strings.Contains(body, `"chip":true`) {
		t.Fatalf("create: %d %s", code, body)
	}
	h := head(t, e, w.page)
	raw, _ := prosemirror.MarshalDoc(h.Doc)
	if want := `{"type":"text","text":"Ship the new  login page, then the "},{"type":"issue_chip","attrs":{"key":"WEB-3"}},{"type":"text","text":" tests."}`; !strings.Contains(string(raw), want) {
		t.Errorf("FR-105.1: the chip is not in place of the second occurrence: %s", raw)
	}
	var summary string
	e.DB.R.QueryRow(`SELECT summary FROM issues WHERE key = 'WEB-3'`).Scan(&summary)
	if summary != "new login page" {
		t.Errorf("FR-105.1: the summary %q", summary)
	}
	if _, html := webtest.Get(t, w.guest, e.URL+"/WEB-3"); !strings.Contains(html, `data-page="`+w.page+`"`) {
		t.Error("FR-105.1: the issue does not link back to the page")
	}
	if _, html := webtest.Get(t, w.guest, e.URL+wiki.PageURL("ENG", w.page, "Notes")); !strings.Contains(html, `id="page-refs"`) || !strings.Contains(html[strings.Index(html, `id="page-refs"`):], "WEB-3") {
		t.Error("the page does not list the new issue under Linked issues")
	}

	// In the editor: no text to find; the editor puts the chip in itself.
	code, body = create(map[string]any{"summary": strings.Repeat("long ", 80)})
	if code != http.StatusOK || !strings.Contains(body, `"key":"WEB-4"`) || !strings.Contains(body, `"chip":false`) {
		t.Fatalf("create in the editor: %d %s", code, body)
	}
	if head(t, e, w.page).Version != h.Version {
		t.Error("an editor's create changed the document")
	}
	e.DB.R.QueryRow(`SELECT summary FROM issues WHERE key = 'WEB-4'`).Scan(&summary)
	if len([]rune(summary)) > tracker.MaxSummary || !strings.HasPrefix(summary, "long long") {
		t.Errorf("DM-05.1: the summary is not trimmed to 255 characters: %d", len(summary))
	}
	// Text that is no longer there: the issue stands without its chip.
	if code, body = create(map[string]any{"summary": "gone", "find": "not on the page", "block": 0}); code != http.StatusOK || !strings.Contains(body, `"chip":false`) {
		t.Errorf("a text that is gone: %d %s", code, body)
	}
	// Refusals: no summary, a project the guest may not see.
	if code, _ := create(map[string]any{"summary": "  "}); code != http.StatusNoContent {
		t.Errorf("an empty summary: %d", code)
	}
	if code, _ := create(map[string]any{"summary": "x", "project": w.sec}); code < 400 {
		t.Errorf("NFR-01: an issue in a closed project: %d", code)
	}
	var n int
	e.DB.R.QueryRow(`SELECT count(*) FROM issues WHERE project_id = ?`, w.sec).Scan(&n)
	if n != 1 {
		t.Errorf("NFR-01: %d issues in the closed project", n)
	}
}
