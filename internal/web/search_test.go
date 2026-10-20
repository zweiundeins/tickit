package web_test

import (
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/search"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// newSearchEnv is listEnv with a description, a comment and pages: the
// open space ENG and the closed space HR, whose pages a guest never sees.
// The pages go in through the hook the wiki's projector calls.
func newSearchEnv(t *testing.T) *listEnv {
	e := newList(t)
	e.Do(t, tracker.SetDescription{By: e.by(e.ada), Issue: e.ids["WEB-2"], Description: tracker.PlainDoc("The <b>cart</b> & the login fail")})
	e.result(tracker.AddComment{By: e.by(e.bo), Issue: e.ids["APP-1"], Body: tracker.PlainDoc("Seen with the login screen")})
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		if err := access.AddScope(tx, access.Space, "space-eng", "ENG", true, e.ada.ID, time.Now()); err != nil {
			return err
		}
		if err := access.AddScope(tx, access.Space, "space-hr", "HR", false, e.ada.ID, time.Now()); err != nil {
			return err
		}
		for _, p := range []search.Doc{
			{ID: "p1", Scope: "space-eng", Title: "Login", Body: "How the login works", Place: "ENG", Href: "/spaces/ENG/pages/p1"},
			{ID: "p2", Scope: "space-eng", Title: "Release notes", Body: "Fixed the login", Place: "ENG", Href: "/spaces/ENG/pages/p2", Labels: []string{"release"}},
			{ID: "p3", Scope: "space-hr", Title: "Login salaries", Body: "Secret", Place: "HR", Href: "/spaces/HR/pages/p3"},
		} {
			p.Updated = time.Now()
			if err := search.Put(tx, search.Page, p); err != nil {
				return err
			}
		}
		return nil
	}))
	return e
}

var hitTitle = regexp.MustCompile(`<p class="search-hit__title">(?:<span class="search-hit__key">([A-Z]+-[0-9]+)</span> )?<a href="[^"]*"[^>]*>([^<]*)</a>`)

// hits are a results page's hits as keys or titles.
func hits(page string) []string {
	var out []string
	for _, m := range hitTitle.FindAllStringSubmatch(page, -1) {
		if m[1] != "" {
			out = append(out, m[1])
		} else {
			out = append(out, m[2])
		}
	}
	return out
}

// FR-57.2, FR-57.3: one results page for issues and pages, ranked exact,
// title, body; filters by kind, project, space, label and person; a
// snippet with the matched words marked and escaped; nothing the person
// may not read.
func TestSearchPage(t *testing.T) {
	e := newSearchEnv(t)
	c := webtest.Client(t)
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"q=login", []string{"Login", "WEB-1", "Release notes", "WEB-2", "APP-1"}},
		{"q=web-2", []string{"WEB-2"}},
		{"q=login&kind=issue", []string{"WEB-1", "WEB-2", "APP-1"}},
		{"q=login&kind=page", []string{"Login", "Release notes"}},
		{"q=login&project=APP", []string{"Login", "Release notes", "APP-1"}},
		{"q=login&space=ENG&kind=page", []string{"Login", "Release notes"}},
		{"q=login&space=HR", []string{"WEB-1", "WEB-2", "APP-1"}}, // a space the guest may not read
		{"q=login&label=release&label=ux", []string{"WEB-1", "Release notes"}},
		{"q=login&person=" + e.bo.ID, []string{"WEB-1", "APP-1"}},
		{"q=salaries", nil},
		{"q=", nil},
	} {
		_, page := webtest.Get(t, c, e.URL+"/search?"+tc.query)
		if got := hits(page); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.query, got, tc.want)
		}
	}
	_, page := webtest.Get(t, c, e.URL+"/search?q=cart")
	if !strings.Contains(page, `The &lt;b&gt;<mark>cart</mark>&lt;/b&gt; &amp; the login fail`) {
		t.Errorf("the snippet is not marked and escaped:\n%s", page)
	}
	if !strings.Contains(page, `value="cart"`) || !strings.Contains(page, `id="tk-search"`) {
		t.Errorf("the header's search box lacks the text")
	}
	if !strings.Contains(page, `data-issue-panel="`+e.ids["WEB-2"]+`"`) || !strings.Contains(page, `id="issue-panel"`) {
		t.Errorf("the results have no issue panel")
	}
	// The home page's link to the person's work: a list's filter without
	// text is the issue list.
	resp, err := c.Get(e.URL + "/search?assignee=me&category=todo&sort=updated&dir=desc")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/issues?assignee=me&category=todo&sort=updated&dir=desc" {
		t.Errorf("a filter on /search: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	_, page = webtest.Get(t, c, e.URL+"/search?q=screen")
	if !strings.Contains(page, "In a comment: ") {
		t.Errorf("a hit by its comment does not say so")
	}
}

// FR-57.1: the typeahead shows the first 5 issues and 5 pages while
// typing; FR-52.1: an exact key comes first.
func TestSearchSuggest(t *testing.T) {
	e := newSearchEnv(t)
	for i := range 6 {
		e.issue("x", tracker.CreateIssue{Project: e.web, Summary: "Login again " + string(rune('a'+i))})
	}
	c := webtest.Client(t)
	code, body := webtest.Get(t, c, e.URL+"/search/suggest?q=logi")
	if code != http.StatusOK || !strings.HasPrefix(body, "event: datastar-patch-elements") || !strings.Contains(body, `id="search-suggest"`) {
		t.Fatalf("suggest: %d %s", code, body)
	}
	if n := strings.Count(body, `class="search-hit__key"`); n != 5 {
		t.Errorf("%d issues", n)
	}
	if !strings.Contains(body, ">Login</span>") || !strings.Contains(body, ">Release notes</span>") || strings.Contains(body, "salaries") {
		t.Errorf("the pages: %s", body)
	}
	if !strings.Contains(body, `href="/search?q=logi"`) || !strings.Contains(body, "7 suggestions") {
		t.Errorf("the way to every result: %s", body)
	}
	_, body = webtest.Get(t, c, e.URL+"/search/suggest?q=web-3")
	if i := strings.Index(body, `class="search-hit__key"`); i < 0 || !strings.HasPrefix(body[i:], `class="search-hit__key">WEB-3<`) {
		t.Errorf("web-3 is not first: %s", body)
	}
	if code, body := webtest.Get(t, c, e.URL+"/search/suggest"); code != http.StatusOK || !strings.Contains(body, "No issues or pages match") {
		t.Errorf("no text: %d %s", code, body)
	}
	_, home := webtest.Get(t, c, e.URL+"/")
	if !strings.Contains(home, `aria-controls="search-suggest"`) || !strings.Contains(home, `<div id="search-suggest" class="search-suggest"`) {
		t.Errorf("the header's search box has no typeahead")
	}
}

// MP-03.1: open results follow changes to what they find.
func TestSearchLive(t *testing.T) {
	e := newSearchEnv(t)
	c := webtest.Client(t)
	_, page := webtest.Get(t, c, e.URL+"/search?q=login")
	m := regexp.MustCompile(`_views: &#34;(search:[^&]*)&#34;`).FindStringSubmatch(page)
	if m == nil {
		t.Fatal("no search view")
	}
	events := webtest.Listen(t, c, e.URL+"/stream?tab=t1&v="+url.QueryEscape(m[1]), nil)
	webtest.Next(t, events, `id="search-results"`)
	e.Do(t, tracker.SetSummary{By: e.by(e.bo), Issue: e.ids["WEB-3"], Seen: 1 << 40, Summary: "Login box"})
	frame := webtest.Next(t, events, `id="search-results"`)
	if got := hits(frame); !slices.Contains(got, "WEB-3") || len(got) != 6 {
		t.Errorf("after the rename: %v", got)
	}
}
