package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/demo/seed"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/search"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// seeded loads the demo's projects and issues through the tracker's
// commands, and its pages into the search index as the wiki's hook would.
func seeded(t *testing.T) *webtest.Env {
	t.Helper()
	e := webtest.Start(t)
	c, err := seed.Load()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	n := 0
	by := func(id string, at time.Time) tabs.By {
		n++
		for _, p := range people.Seeded {
			if p.ID == id {
				return tabs.By{Actor: p, Op: fmt.Sprint("seed", n), At: at}
			}
		}
		return tabs.By{Actor: people.Seeded[0], Op: fmt.Sprint("seed", n), At: at}
	}
	do := func(cmd writer.ResultCommand) string {
		b, err := e.W.DoValue(context.Background(), func(tx *writer.Tx) ([]byte, error) { return cmd.ApplyResult(tx) })
		if err != nil {
			t.Fatalf("%T: %v", cmd, err)
		}
		var m map[string]string
		json.Unmarshal(b, &m)
		return m["id"]
	}
	e.Do(t, people.SeedPeople{At: now})
	projects := map[string]string{}
	for _, p := range c.Projects {
		projects[p.Key] = do(tracker.CreateProject{By: by(p.Lead, now.AddDate(0, 0, -40)), Key: p.Key, Name: p.Name, Description: p.Description, Lead: p.Lead, Open: true})
	}
	status := map[string]string{}
	for _, st := range tracker.DefaultWorkflow().Statuses {
		status[st.Name] = st.ID
	}
	ids := map[string]string{}
	for _, i := range c.Issues {
		var due string
		if i.Due != nil {
			due = seed.DueDate(now, *i.Due).Format(time.DateOnly)
		}
		id := do(tracker.CreateIssue{By: by(i.Reporter, i.Created.Time(now)), Project: projects[i.Project], Type: tracker.IssueType(strings.ToLower(i.Type)),
			Summary: i.Summary, Description: tracker.PlainDoc(string(i.Description)), Status: status[i.Status], Priority: tracker.Priority(strings.ToLower(i.Priority)),
			Assignee: i.Assignee, Labels: i.Labels, Due: due, Points: i.Points, Parent: ids[i.Parent]})
		ids[i.Key] = id
		for _, cm := range i.Comments {
			do(tracker.AddComment{By: by(cm.By, cm.At.Time(now)), Issue: id, Body: tracker.PlainDoc(string(cm.Body))})
		}
	}
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		for _, s := range c.Spaces {
			if err := access.AddScope(tx, access.Space, "space-"+strings.ToLower(s.Key), s.Key, true, "s-priya", now); err != nil {
				return err
			}
		}
		for i, p := range c.Pages {
			id := fmt.Sprintf("page-%03d", i)
			err := search.Put(tx, search.Page, search.Doc{ID: id, Scope: "space-" + strings.ToLower(p.Space), Title: p.Title, Body: p.Body,
				Labels: p.Labels, People: []string{p.Author}, Place: p.Space, Href: "/spaces/" + p.Space + "/pages/" + id, Updated: p.Created.Time(now)})
			if err != nil {
				return err
			}
		}
		return nil
	}))
	return e
}

var rowKeys = `[...document.querySelectorAll('#issue-list tbody td.tk-key')].map(td => td.textContent)`

// typeInto types text into the field with id, one key at a time.
func typeInto(t *testing.T, p *browsertest.Page, id, text string) {
	t.Helper()
	p.Eval(t, `(() => { const i = document.getElementById(`+jsQuote(id)+`); i.focus(); i.select(); return true })()`, nil)
	for _, r := range text {
		p.Call(t, "Input.insertText", map[string]any{"text": string(r)})
		time.Sleep(60 * time.Millisecond)
	}
}

// FR-55.3, FR-52.2: typing in the list's text box updates the list while
// typing, without a page load; FR-55.1: back and forward restore earlier
// filters, sort and the list with them.
func TestListTypingAndHistory(t *testing.T) {
	e := seeded(t)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1440, 900)
	p.Navigate(t, e.URL+"/issues")
	p.Wait(t, `document.querySelectorAll('#issue-list tbody tr').length === 25`, 5*time.Second)
	waitStreams(t, e, 1)
	p.Eval(t, `(window.__same = 'document', true)`, nil)

	typeInto(t, p, "list-q", "checkout")
	p.Wait(t, `location.search === '?q=checkout' && `+rowKeys+`.length > 0 && `+rowKeys+`.length < 25`, 5*time.Second)
	var keys []string
	p.Eval(t, rowKeys, &keys)
	var summaries []string
	p.Eval(t, `[...document.querySelectorAll('#issue-list tbody td.tk-grow')].map(td => td.textContent.toLowerCase())`, &summaries)
	t.Logf("checkout: %v", keys)

	// A status filter, then a sort: each a new entry in the history.
	p.Eval(t, `(() => { document.querySelector('[popovertarget=list-f-category]').click(); document.querySelector('#list-f-category input[value=done]').click(); return true })()`, nil)
	p.Wait(t, `location.search === '?q=checkout&category=done' && document.querySelector('[data-count=category]').textContent === '1'`, 5*time.Second)
	var done []string
	p.Wait(t, rowKeys+`.length < `+fmt.Sprint(len(keys)), 5*time.Second)
	p.Eval(t, rowKeys, &done)
	p.Eval(t, `(document.getElementById('list-f-category').hidePopover(), document.querySelector('th [data-sort=key]').click(), true)`, nil)
	p.Wait(t, `location.search === '?q=checkout&category=done&sort=key&dir=asc' && document.querySelector('th[aria-sort=ascending] [data-sort=key]') !== null`, 5*time.Second)

	p.Eval(t, `(history.back(), true)`, nil)
	p.Wait(t, `location.search === '?q=checkout&category=done' && document.querySelector('[name=sort]').value === '' && document.querySelector('th[aria-sort] [data-sort=created]') !== null`, 5*time.Second)
	p.Eval(t, `(history.back(), true)`, nil)
	p.Wait(t, `location.search === '?q=checkout' && !document.querySelector('#list-f-category input[value=done]').checked && document.querySelector('[data-count=category]').hidden`, 5*time.Second)
	p.Wait(t, rowKeys+`.join() === `+jsQuote(strings.Join(keys, ",")), 5*time.Second)
	p.Eval(t, `(history.forward(), true)`, nil)
	p.Wait(t, `location.search === '?q=checkout&category=done' && document.querySelector('#list-f-category input[value=done]').checked`, 5*time.Second)
	p.Wait(t, rowKeys+`.join() === `+jsQuote(strings.Join(done, ",")), 5*time.Second)

	var same string
	p.Eval(t, `window.__same`, &same)
	if same != "document" {
		t.Errorf("the page loaded again")
	}
	if n := e.Hub.Stats().Streams; n != 1 {
		t.Errorf("%d streams: the list reopened its stream", n)
	}
	// FR-55.2: back to /issues, the list is the one the tab showed last.
	p.Navigate(t, e.URL+"/issues")
	p.Wait(t, `location.search === '?category=done&q=checkout' && document.querySelector('#list-f-category input[value=done]').checked`, 5*time.Second)
	if errs := p.Errors(); len(errs) > 0 {
		t.Errorf("the page reported: %q", errs)
	}
}

// FR-57.1: the header's search box shows the first issues and pages while
// typing; Enter opens the results page.
func TestSearchBox(t *testing.T) {
	e := seeded(t)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1440, 900)
	p.Navigate(t, e.URL+"/")
	waitStreams(t, e, 1)
	typeInto(t, p, "tk-search", "web-1")
	p.Wait(t, `document.querySelector('#search-suggest .search-hit__key')?.textContent === 'WEB-1'`, 5*time.Second)
	typeInto(t, p, "tk-search", "release")
	p.Wait(t, `document.querySelectorAll('#search-suggest .search-suggest__group').length === 2`, 5*time.Second)
	for _, typ := range []string{"keyDown", "keyUp"} {
		p.Call(t, "Input.dispatchKeyEvent", map[string]any{"type": typ, "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13, "text": "\r"})
	}
	p.Wait(t, `location.pathname === '/search' && document.querySelectorAll('.search-hit').length > 0`, 5*time.Second)
	if errs := p.Errors(); len(errs) > 0 {
		t.Errorf("the page reported: %q", errs)
	}
}

func waitStreams(t *testing.T, e *webtest.Env, n int) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); e.Hub.Stats().Streams != n; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("streams: %+v", e.Hub.Stats())
		}
	}
}

// Screenshots of /issues and /search in both themes at 1440 and 375
// pixels, for a look against /design (SHOTS names the directory).
func TestListShots(t *testing.T) {
	dir := os.Getenv("SHOTS")
	if dir == "" {
		t.Skip("SHOTS names the directory for the screenshots")
	}
	e := seeded(t)
	for i, f := range []struct {
		name  string
		query string
	}{{"Open checkout bugs", "type=bug&label=checkout&category=todo&category=inprogress"}, {"Critical, unassigned", "priority=critical&assignee=unassigned"}, {"Sale readiness", "q=sale&sort=priority&dir=desc"}} {
		v, _ := url.ParseQuery(f.query)
		e.Do(t, tracker.SaveFilter{By: tabs.By{Actor: people.Seeded[i], Op: fmt.Sprint("f", i), At: time.Now()}, Name: f.name, Filter: tracker.ParseFilter(v), Sort: tracker.ParseSort(v)})
	}
	b := browsertest.Start(t)
	for _, theme := range []string{"dark", "light"} {
		for _, w := range []int{1440, 375} {
			p := b.NewPage(t, w, 1000)
			p.Call(t, "Emulation.setEmulatedMedia", map[string]any{"features": []map[string]string{{"name": "prefers-color-scheme", "value": theme}}})
			for name, path := range map[string]string{"issues": "/issues?sort=priority&dir=desc", "search": "/search?q=checkout", "filters": "/filters", "suggest": "/"} {
				p.Navigate(t, e.URL+path)
				p.Wait(t, `document.readyState === 'complete'`, 5*time.Second)
				if name == "suggest" {
					typeInto(t, p, "tk-search", "check")
					p.Wait(t, `document.querySelectorAll('#search-suggest .search-suggest__item').length > 0`, 5*time.Second)
				}
				if name == "issues" && w == 1440 {
					p.Eval(t, `(document.querySelector('[popovertarget=list-f-status]').click(), true)`, nil)
				}
				time.Sleep(200 * time.Millisecond)
				p.Screenshot(t, filepath.Join(dir, fmt.Sprintf("list-%s-%s-%d.png", name, theme, w)), name != "suggest")
			}
			if errs := p.Errors(); len(errs) > 0 {
				t.Errorf("%s %d: %q", theme, w, errs)
			}
			p.Close(t)
		}
	}
}

// FR-50.3: a click on a row opens the issue in the side panel; closing it
// brings the list's address back, the list as it was.
func TestListPanel(t *testing.T) {
	e := seeded(t)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1440, 900)
	p.Navigate(t, e.URL+"/issues?q=checkout")
	waitStreams(t, e, 1)
	p.Wait(t, rowKeys+`.length > 3`, 5*time.Second)
	var key string
	p.Eval(t, `(() => { const row = document.querySelectorAll('#issue-list tbody tr')[2]; row.querySelector('td:nth-child(5)').click(); return row.querySelector('td.tk-key').textContent })()`, &key)
	p.Wait(t, `document.getElementById('issue-panel').open && location.pathname === '/`+key+`'`, 5*time.Second)
	p.Wait(t, `document.querySelector('#issue-panel .mono')?.textContent === '`+key+`'`, 5*time.Second)
	p.Press(t, "Escape")
	p.Wait(t, `!document.getElementById('issue-panel').open && location.search === '?q=checkout' && `+rowKeys+`.length > 3`, 5*time.Second)
	if errs := p.Errors(); len(errs) > 0 {
		t.Errorf("the page reported: %q", errs)
	}
}
