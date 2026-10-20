package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/demo"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// logins keep one login per email: logins are rate-limited (NFR-09).
var logins = map[string]*http.Client{}

// staffPage is a browser page logged in as a staff account: the login
// goes through the server, and the browser takes the session's cookie.
func staffPage(t *testing.T, e *webtest.Env, b *browsertest.Browser, email string, width int) *browsertest.Page {
	t.Helper()
	c, ok := logins[e.URL+email]
	if !ok {
		c = webtest.Client(t)
		e.Login(t, c, email, "correct horse battery")
		logins[e.URL+email] = c
	}
	u, _ := url.Parse(e.URL)
	p := b.NewPage(t, width, 900)
	for _, k := range c.Jar.Cookies(u) {
		p.Call(t, "Network.setCookie", map[string]any{"name": k.Name, "value": k.Value, "url": e.URL, "httpOnly": true, "sameSite": "Lax"})
	}
	return p
}

// seededDemo is a server with the seed, a moderator, an admin, and what
// three guests wrote: issues, comments, a hidden one, a block.
func seededDemo(t *testing.T) (*webtest.Env, map[string]string) {
	t.Helper()
	e := webtest.Start(t)
	if _, err := demo.Seed(context.Background(), e.W, e.Server.Demo().Content(), time.Now(), 1); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{
		"mo":  e.Account(t, "mo@example.com", "correct horse battery", people.Moderator),
		"ada": e.Account(t, "ada@example.com", "correct horse battery", people.Admin),
	}
	e.Account(t, "bea@example.com", "correct horse battery", people.NoRole)
	var web, issue string
	e.DB.R.QueryRow(`SELECT id FROM projects WHERE key = 'WEB'`).Scan(&web)
	e.DB.R.QueryRow(`SELECT id FROM issues WHERE key = 'WEB-3'`).Scan(&issue)
	for i, w := range []struct{ session, summary, comment string }{
		{"shots-otter", "Checkout button overlaps the cookie hint on small phones", "Seen on an iPhone SE in Safari: the hint covers half of the button."},
		{"shots-lynx", "BUY CHEAP WATCHES www.example.com", "Visit my shop for the best prices on watches!!!"},
		{"shots-heron", "Search finds nothing for umlauts", "Searching for Rucksäcke gives no result, Rucksack works."},
	} {
		g := people.GuestOf(w.session)
		id := tracker.NewID()
		by := tabs.By{Actor: g, Op: fmt.Sprint("i", i), At: time.Now().Add(time.Duration(i-40) * time.Minute)}
		e.Do(t, tabs.Act{Session: "s" + w.session, Tab: "tab", By: by, Cmd: tracker.CreateIssue{By: by, ID: id, Project: web, Type: tracker.Bug, Summary: w.summary}})
		by.Op += "c"
		e.Do(t, tabs.Act{Session: "s" + w.session, Tab: "tab", By: by, Cmd: tracker.AddComment{By: by, Issue: issue, Body: tracker.PlainDoc(w.comment)}})
		ids[w.session] = id
	}
	mo := people.Person{ID: ids["mo"], Kind: people.Account, Role: people.Moderator, Name: "mo"}
	e.Do(t, demo.Moderate{By: tabs.By{Actor: mo, Op: "m1", At: time.Now()}, Action: demo.ActHide, Kind: "issue", ID: ids["shots-lynx"], Reason: "Spam"})
	e.Do(t, e.Server.Demo().Block(tabs.By{Actor: mo, Op: "m2", At: time.Now()}, people.GuestOf("shots-lynx").ID, "Spam"))
	return e, ids
}

// DM-07.1, DM-07.3: a moderator hides a visitor's comment with one click
// on the moderation page, which then shows it as hidden.
func TestModerationHide(t *testing.T) {
	e, _ := seededDemo(t)
	b := browsertest.Start(t)
	p := staffPage(t, e, b, "mo@example.com", 1280)
	p.Navigate(t, e.URL+"/moderation")
	p.Wait(t, `document.getElementById('reconnecting').hidden && document.querySelector('#moderation [data-kind=comment][data-action=hide]')`, 5*time.Second)
	p.Eval(t, `(() => {
		const b = document.querySelector('#moderation [data-kind=comment][data-action=hide]')
		window.e2eID = b.dataset.id
		b.click()
		return true
	})()`, nil)
	p.Wait(t, `document.querySelector('#moderation [data-id="' + window.e2eID + '"][data-action=restore]') !== null && document.getElementById('mod-outcome').textContent === 'Done.'`, 5*time.Second)
	if errs := p.Errors(); len(errs) > 0 {
		t.Errorf("the page reported: %q", errs)
	}
}

// DM-03.2, DM-03.4: while the demo resets, every open page says so, and
// stops saying it after.
func TestResettingShows(t *testing.T) {
	e := webtest.Start(t)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1024, 768)
	p.Navigate(t, e.URL+"/")
	p.Wait(t, `document.getElementById('reconnecting').hidden && document.getElementById('demo-alert')`, 5*time.Second)
	release := e.Server.Demo().Hold()
	p.Wait(t, `document.getElementById('demo-alert').textContent.includes('The demo is resetting')`, 5*time.Second)
	release()
	p.Wait(t, `document.getElementById('demo-alert').textContent.trim() === ''`, 5*time.Second)
}

// Screenshots of the admin and moderation pages, dark and light, at 1440
// and 375 pixels, into $TICKIT_SHOTS (demo-*.png), for a look against
// /design. It runs only when the directory is given.
func TestDemoShots(t *testing.T) {
	dir := os.Getenv("TICKIT_SHOTS")
	if dir == "" {
		t.Skip("TICKIT_SHOTS names no directory")
	}
	e, _ := seededDemo(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b := browsertest.Start(t)
	for _, shot := range []struct{ name, path, email string }{
		{"admin", "/admin", "ada@example.com"},
		{"admin-demo", "/admin/demo", "ada@example.com"},
		{"admin-security", "/admin/security", "ada@example.com"},
		{"moderation", "/moderation", "mo@example.com"},
	} {
		for _, theme := range []string{"dark", "light"} {
			for _, width := range []int{1440, 375} {
				page := staffPage(t, e, b, shot.email, width)
				page.Navigate(t, e.URL+shot.path)
				page.Eval(t, `(document.documentElement.dataset.tkTheme = '`+theme+`', document.querySelector('meta[name=color-scheme]').content = '`+theme+`', true)`, nil)
				page.Wait(t, `document.getElementById('reconnecting').hidden && document.fonts.status === 'loaded'`, 5*time.Second)
				page.Eval(t, `(document.querySelector('#first-visit:not([hidden]) button')?.click(), true)`, nil)
				time.Sleep(400 * time.Millisecond)
				if w := evalInt(t, page, `document.documentElement.scrollWidth`); w > width {
					t.Errorf("%s %s at %d: the page is %d pixels wide", shot.name, theme, width, w) // NFR-46
				}
				page.Screenshot(t, filepath.Join(dir, fmt.Sprintf("demo-%s-%s-%d.png", shot.name, theme, width)), true)
				if errs := page.Errors(); len(errs) > 0 {
					t.Errorf("%s: the page reported %q", shot.name, errs)
				}
				page.Close(t)
			}
		}
	}
}
