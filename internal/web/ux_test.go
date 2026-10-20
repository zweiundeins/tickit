package web_test

import (
	"html"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/markup"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// FR-38: the list shows when an issue was resolved, sorts by it and
// filters by the day it was resolved; an open issue matches no such day.
func TestListResolved(t *testing.T) {
	e := newList(t)
	c := webtest.Client(t)
	today := time.Now().UTC().Format(time.DateOnly)
	if got := e.list(c, "resolved_from="+today+"&resolved_to="+today); !slices.Equal(got, []string{"WEB-3"}) {
		t.Errorf("resolved today: %v, want WEB-3", got)
	}
	if got := e.list(c, "resolved_to=2000-01-01"); len(got) != 0 {
		t.Errorf("resolved before 2000: %v, want none", got)
	}
	if got := e.list(c, "sort=resolved&dir=asc"); len(got) != 4 || got[0] != "WEB-3" {
		t.Errorf("by resolved: %v, the resolved issue first and the open ones last", got)
	}
	_, page := webtest.Get(t, c, e.URL+"/issues?project=WEB")
	for _, want := range []string{">Resolved<", `name="resolved_from"`, `name="resolved_to"`, "Not resolved"} {
		if !strings.Contains(page, want) {
			t.Errorf("the list has no %q", want)
		}
	}
}

// FR-23.2, FR-25.2: labels in the list and on cards link to the list of
// their issues, and an overdue issue says "Overdue" in text beside its
// icon, not only in colour.
func TestLabelsAndOverdue(t *testing.T) {
	e := newList(t)
	e.issue("WEB-4", tracker.CreateIssue{Project: e.web, Summary: "Late & loud", Due: "2020-01-02", Labels: []string{"a&b"}})
	c := webtest.Client(t)
	_, list := webtest.Get(t, c, e.URL+"/issues?project=WEB")
	_, board := webtest.Get(t, c, e.URL+"/projects/WEB/board")
	for name, page := range map[string]string{"list": list, "board": board} {
		for _, want := range []string{
			`class="tk-label" data-hue="`, `href="/issues?label=ux"`, `href="/issues?label=a%26b"`,
			`<b>Overdue</b>`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: no %q", name, want)
			}
		}
	}
	if code, _ := webtest.Get(t, c, e.URL+"/issues?label=a%26b"); code != http.StatusOK {
		t.Errorf("the label's list: %d", code)
	}
	if got := e.list(c, "label=a%26b"); !slices.Equal(got, []string{"WEB-4"}) {
		t.Errorf("label a&b: %v", got)
	}
}

// QA (FR-28.1, FR-137.2): a guest is not offered "Delete issue", which
// only a project's Admin, a moderator or a site admin may use; the
// document decides it, and the issue's shared region is the same for all.
func TestDeleteOfferedToThoseWhoMay(t *testing.T) {
	e := newList(t)
	guest := webtest.Client(t)
	hide := `.tk-issue .tk-issue__main [data-del]{display:none}`
	_, page := webtest.Get(t, guest, e.URL+"/WEB-1")
	if !strings.Contains(page, hide) {
		t.Errorf("a guest's issue page offers Delete issue")
	}
	if !strings.Contains(page, `data-pid="`+e.web+`"`) || !strings.Contains(page, `data-del`) {
		t.Errorf("the issue's region has no project or no delete item")
	}
	_, list := webtest.Get(t, guest, e.URL+"/issues?project=WEB")
	if !strings.Contains(list, hide) {
		t.Errorf("a guest's list offers Delete issue in the panel")
	}
	e.Account(t, "mod@example.com", "a long password 1", people.Moderator)
	mod := webtest.Client(t)
	e.Login(t, mod, "mod@example.com", "a long password 1")
	for _, path := range []string{"/WEB-1", "/issues?project=WEB", "/projects/WEB/board"} {
		if _, p := webtest.Get(t, mod, e.URL+path); strings.Contains(p, `[data-del]{display:none}`) {
			t.Errorf("%s: a moderator is not offered Delete issue", path)
		}
	}
}

// FR-56.2: a saved filter feeds the board's quick filter: the Filters
// menu above the board offers it with its quick parts, and the board's
// page and its view apply what it adds (here, its categories).
func TestBoardSavedFilter(t *testing.T) {
	e := newList(t)
	id := e.result(tracker.SaveFilter{By: e.by(e.ada), Name: "Not done yet", Filter: tracker.Filter{Categories: []tracker.Category{tracker.ToDo, tracker.InProgress}, Labels: []string{"ux"}}})
	c := webtest.Client(t)
	_, page := webtest.Get(t, c, e.URL+"/projects/WEB/board")
	for _, want := range []string{`id="board-saved"`, `data-v="` + id + `"`, `data-filter="saved"`, `data-name="Not done yet"`, `data-l="ux"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the board's Filters menu has no %q", want)
		}
	}
	if !strings.Contains(page, `id="c-WEB-3"`) {
		t.Fatalf("WEB-3 is not on the unfiltered board")
	}
	_, page = webtest.Get(t, c, e.URL+"/projects/WEB/board?filter="+id)
	if strings.Contains(page, `id="c-WEB-3"`) || !strings.Contains(page, `id="c-WEB-1"`) || !strings.Contains(page, `id="c-WEB-2"`) {
		t.Errorf("the saved filter's categories do not apply to the board's page")
	}
	if !strings.Contains(page, `data-arg="WEB.f`+id+`"`) || !strings.Contains(page, "Not done yet") {
		t.Errorf("the board's page does not name the saved filter")
	}
	ev := webtest.Listen(t, c, e.URL+"/stream?tab=tabsf&v=board:WEB.f"+id, nil)
	frame := webtest.Next(t, ev, `id="board-lanes"`)
	if strings.Contains(frame, `id="c-WEB-3"`) || !strings.Contains(frame, `id="c-WEB-2"`) {
		t.Errorf("the board view with the saved filter shows the wrong cards")
	}
	// A saved filter that no longer exists filters nothing.
	ev2 := webtest.Listen(t, c, e.URL+"/stream?tab=tabsg&v=board:WEB.f01920000-0000-7000-8000-00000000ffff", nil)
	if frame := webtest.Next(t, ev2, `id="board-lanes"`); !strings.Contains(frame, `id="c-WEB-3"`) {
		t.Errorf("an unknown saved filter hides cards")
	}
}

type namesOnly map[string]string

func (namesOnly) Issue(string) bool                 { return false }
func (n namesOnly) Person(id string) (string, bool) { s, ok := n[id]; return s, ok }
func (namesOnly) Page(string) (string, bool)        { return "", false }
func (namesOnly) Image(string) bool                 { return false }

// FR-67.2: a mention shows the person's face and name and links to their
// page, which shows their face, name, issues and what they did; the text
// to edit names them, not their ids (QA: "@g-m3z2…" in the draft).
func TestMentionsAndPersonPage(t *testing.T) {
	e := newList(t)
	body, err := markup.ParseMode("Over to @"+e.bo.ID+" for the login.", namesOnly{e.bo.ID: e.bo.Name}, markup.Comment)
	if err != nil {
		t.Fatal(err)
	}
	e.Do(t, tracker.AddComment{By: e.by(e.ada), Issue: e.ids["WEB-1"], Body: body})
	c := webtest.Client(t)
	_, page := webtest.Get(t, c, e.URL+"/WEB-1")
	for _, want := range []string{
		`<a class="mention" data-person="` + e.bo.ID + `" href="/people/` + e.bo.ID + `" data-face><span class="tk-av tk-av--xs"`,
		`</svg></span>` + html.EscapeString(e.bo.Name) + `</a>`,
		`Over to @` + html.EscapeString(e.bo.Name) + ` for the login.`, // the comment's markup to edit
		`<span hidden data-id="` + e.bo.ID + `" data-name="` + html.EscapeString(e.bo.Name) + `"></span>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the issue page has no %q", want)
		}
	}
	if strings.Contains(page, "@"+e.bo.ID+" ") {
		t.Errorf("the comment's markup shows the mention's id")
	}

	code, pp := webtest.Get(t, c, e.URL+"/people/"+e.bo.ID)
	if code != http.StatusOK {
		t.Fatalf("/people/%s: %d", e.bo.ID, code)
	}
	for _, want := range []string{"<h1>" + html.EscapeString(e.bo.Name) + "</h1>", `class="tk-av tk-av--lg"`, "Fix the login", "Crash on start", `data-views`} {
		if want == "data-views" {
			want = "profile:" + e.bo.ID
		}
		if !strings.Contains(pp, want) {
			t.Errorf("Bo's page has no %q", want)
		}
	}
	_, ap := webtest.Get(t, c, e.URL+"/people/"+e.ada.ID)
	if !strings.Contains(ap, "Search_box") || !strings.Contains(ap, "Cart 100% broken") || !strings.Contains(ap, "commented on") {
		t.Errorf("Ada's page lacks what she reported or did")
	}
	for _, bad := range []string{"/people/nope", "/people/a-01920000-0000-7000-8000-000000000001"} {
		if code, _ := webtest.Get(t, c, e.URL+bad); code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", bad, code)
		}
	}
	// The page is live: a new issue assigned to Bo shows in its next frame.
	ev := webtest.Listen(t, c, e.URL+"/stream?tab=tabpp&v=profile:"+e.bo.ID, nil)
	webtest.Next(t, ev, `id="person-page"`)
	e.issue("WEB-9", tracker.CreateIssue{Project: e.web, Summary: "Bo's new one", Assignee: e.bo.ID})
	webtest.Next(t, ev, "Bo&#39;s new one")
}
