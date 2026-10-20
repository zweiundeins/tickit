package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/browsertest"
	"github.com/mbolli/tickit/internal/demo"
	"github.com/mbolli/tickit/internal/web/webtest"
	"github.com/mbolli/tickit/internal/wiki"
)

// a11yPages are the pages of NFR-45.1, on the seeded demo: what to open,
// and what to do before axe looks (the notification panel opens).
func a11yPages(t *testing.T, e *webtest.Env) []struct{ name, path, before string } {
	t.Helper()
	var page string
	if err := e.DB.R.QueryRow(`SELECT id FROM pages WHERE title = 'Deploy to production'`).Scan(&page); err != nil {
		t.Fatal(err)
	}
	return []struct{ name, path, before string }{
		{"home", "/", ""},
		{"board", "/projects/WEB/board", ""},
		{"issue", "/WEB-2", ""},
		{"issue list", "/issues?sort=key&dir=asc", ""},
		{"page", wiki.PageURL("ENG", page, "Deploy to production"), ""},
		{"page edit", wiki.EditURL("ENG", page), `document.getElementById('editor')?.view`},
		{"search", "/search?q=checkout", ""},
		{"notifications", "/", `(document.querySelector('[popovertarget=tk-notifications]').click(), true)`},
	}
}

// NFR-45.1, section 12 item 8: axe finds no serious or critical issue on
// the home page, a board, an issue, the issue list, a page in view and in
// edit, search and the open notification panel, in both themes. It runs
// when TICKIT_AXE names axe-core's axe.min.js (npm install axe-core);
// without it, it skips.
func TestAxe(t *testing.T) {
	src := os.Getenv("TICKIT_AXE")
	if src == "" {
		t.Skip("TICKIT_AXE names no axe.min.js")
	}
	axe, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	e := webtest.Start(t)
	if _, err := demo.Seed(context.Background(), e.W, e.Server.Demo().Content(), time.Now(), 1); err != nil {
		t.Fatal(err)
	}
	b := browsertest.Start(t)
	for _, theme := range []string{"dark", "light"} {
		p := b.NewPage(t, 1280, 900)
		p.Call(t, "Emulation.setEmulatedMedia", map[string]any{"features": []map[string]string{{"name": "prefers-color-scheme", "value": theme}}})
		for _, pg := range a11yPages(t, e) {
			p.Navigate(t, e.URL+pg.path)
			p.Wait(t, `document.getElementById('reconnecting')?.hidden && document.getElementById('stream')`, 10*time.Second)
			if pg.before != "" {
				p.Wait(t, pg.before, 10*time.Second)
			}
			time.Sleep(700 * time.Millisecond) // the stream's first frames, the popover's transition
			// DevTools evaluates outside the page's CSP.
			p.Eval(t, string(axe)+";true", nil)
			var res []struct {
				ID, Impact, Help string
				Nodes            []struct{ Target []any }
			}
			p.Eval(t, `axe.run(document, {resultTypes: ['violations']}).then(r => r.violations.map(v => ({id: v.id, impact: v.impact, help: v.help, nodes: v.nodes.slice(0, 5).map(n => ({target: n.target}))})))`, &res)
			for _, v := range res {
				var where []string
				for _, n := range v.Nodes {
					b, _ := json.Marshal(n.Target)
					where = append(where, string(b))
				}
				msg := fmt.Sprintf("%s (%s, %s): %s %s", pg.name, theme, v.Impact, v.ID+": "+v.Help, strings.Join(where, " "))
				if v.Impact == "serious" || v.Impact == "critical" {
					t.Error(msg)
				} else {
					t.Log(msg)
				}
			}
		}
		p.Close(t)
	}
}

// NFR-46.1, NFR-56.1: at 320 CSS pixels (400 % of 1280) no main page
// scrolls sideways; the board's lanes scroll inside their region.
func TestNarrowPages(t *testing.T) {
	e := webtest.Start(t)
	if _, err := demo.Seed(context.Background(), e.W, e.Server.Demo().Content(), time.Now(), 1); err != nil {
		t.Fatal(err)
	}
	b := browsertest.Start(t)
	p := b.NewPage(t, 320, 800)
	for _, pg := range a11yPages(t, e) {
		p.Navigate(t, e.URL+pg.path)
		p.Wait(t, `document.getElementById('reconnecting')?.hidden && document.getElementById('stream')`, 10*time.Second)
		time.Sleep(400 * time.Millisecond)
		var w struct{ Scroll, Inner int }
		p.Eval(t, `({scroll: document.documentElement.scrollWidth, inner: innerWidth})`, &w)
		if w.Scroll > w.Inner {
			t.Errorf("%s scrolls sideways at 320 px: %d", pg.name, w.Scroll)
		}
	}
	if errs := p.Errors(); len(errs) > 0 {
		t.Errorf("the pages reported: %q", errs)
	}
}
