package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/views"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// openIssue opens WEB-2's page in a new browser and waits for its stream.
func openIssue(t *testing.T, e *webtest.Env, width, height int) *browsertest.Page {
	t.Helper()
	p := browsertest.Start(t).NewPage(t, width, height)
	p.Navigate(t, e.URL+"/WEB-2")
	p.Wait(t, `document.querySelectorAll('#issue-presence-slot li[data-who]').length > 0`, 5*time.Second)
	return p
}

// choose opens a menu of the issue's details and picks the item labelled
// label.
func choose(t *testing.T, p *browsertest.Page, menu, label string) {
	t.Helper()
	p.Eval(t, `(() => {
		document.querySelector('[popovertarget=`+menu+`]').click()
		return true
	})()`, nil)
	pick(t, p, menu, label)
}

func pick(t *testing.T, p *browsertest.Page, menu, label string) {
	t.Helper()
	p.Eval(t, `(() => {
		const item = [...document.querySelectorAll('#`+menu+` .tk-menu__item')].find(b => b.textContent.trim() === `+jsQuote(label)+`)
		item.click()
		return true
	})()`, nil)
}

const priorityIs = `document.querySelector('[popovertarget=issue-priority]').textContent.trim() === `

// MP-02, MP-03, MP-08, MP-09: two people on one issue. Each sees the
// other's face; a priority one sets shows in the other's page within a
// second; when both change it from the same version, the second gets
// MP-08.3's message and "Use Critical anyway" sends it again.
func TestIssueTwoPeople(t *testing.T) {
	e := webtest.Start(t)
	d := issueData(t, e)
	a := openIssue(t, e, 1280, 900)
	b := openIssue(t, e, 1280, 900)
	faces := `document.querySelectorAll('#presence-issue-` + d.story + ` li[data-who]').length === 2`
	a.Wait(t, faces, 3*time.Second)
	b.Wait(t, faces, 3*time.Second)

	// MP-03.1: within a second.
	start := time.Now()
	choose(t, a, "issue-priority", "Low")
	b.Wait(t, priorityIs+`'Low'`, time.Second)
	t.Logf("the other page showed the change after %v", time.Since(start).Round(time.Millisecond))

	// MP-08.3: both from the version they saw when they opened the menu.
	for _, p := range []*browsertest.Page{a, b} {
		p.Eval(t, `(document.querySelector('[popovertarget=issue-priority]').click(), true)`, nil)
	}
	pick(t, a, "issue-priority", "High")
	b.Wait(t, priorityIs+`'High'`, 2*time.Second)
	pick(t, b, "issue-priority", "Critical")
	conflict := `[...document.querySelectorAll('#issue-side .tk-issue__conflict')].find(n => !n.closest('[hidden]'))`
	b.Wait(t, conflict+`?.textContent.includes('changed the priority to High') && `+conflict+`.textContent.includes('Your Critical was not saved.')`, 3*time.Second)
	b.Wait(t, conflict+`.querySelector('button').textContent.trim() === 'Use Critical anyway'`, time.Second)
	b.Eval(t, `(`+conflict+`.querySelector('button').click(), true)`, nil)
	a.Wait(t, priorityIs+`'Critical'`, 2*time.Second)
	b.Wait(t, `!`+conflict, 2*time.Second)

	// MP-07.2: typing shows to the other, and goes after 5 s.
	a.Eval(t, `(() => { const c = document.getElementById('issue-comment'); c.focus(); c.value = 'Typing'; c.dispatchEvent(new Event('input', {bubbles: true})); return true })()`, nil)
	b.Wait(t, `document.getElementById('issue-typing-comment').textContent.includes('is typing')`, 3*time.Second)
	a.Wait(t, `getComputedStyle(document.querySelector('#issue-typing-comment .tk-typing__who') || document.body).display === 'none' || !document.querySelector('#issue-typing-comment .tk-typing__who')`, time.Second)
	b.Wait(t, `document.getElementById('issue-typing-comment').textContent.trim() === ''`, 8*time.Second)

	for _, p := range []*browsertest.Page{a, b} {
		if errs := p.Errors(); len(errs) > 0 {
			t.Errorf("the page reported: %q", errs)
		}
	}
}

// FR-14.1, MP-09.1, MP-03.3: the summary edits in place; what someone
// types stays while frames arrive.
func TestIssueEditInPlace(t *testing.T) {
	e := webtest.Start(t)
	issueData(t, e)
	a := openIssue(t, e, 1280, 900)
	b := openIssue(t, e, 1280, 900)
	a.Eval(t, `(() => {
		document.querySelector('h1.tk-issue__title').click()
		return true
	})()`, nil)
	a.Wait(t, `document.activeElement?.id === 'issue-edit-summary'`, 2*time.Second)
	a.Eval(t, `(() => { const i = document.getElementById('issue-edit-summary'); i.value = 'Half typed'; i.dispatchEvent(new Event('input', {bubbles: true})); return true })()`, nil)
	// Someone else's change re-renders the region around the editor.
	choose(t, b, "issue-priority", "Low")
	a.Wait(t, priorityIs+`'Low'`, 2*time.Second)
	a.Wait(t, `document.getElementById('issue-edit-summary').value === 'Half typed' && document.activeElement?.id === 'issue-edit-summary'`, time.Second)
	b.Wait(t, `document.getElementById('issue-typing-summary').textContent.includes('is editing the summary')`, 3*time.Second)
	a.Eval(t, `(document.getElementById('issue-edit-summary').dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true})), true)`, nil)
	b.Wait(t, `document.getElementById('issue-summary-text').textContent === 'Half typed'`, 2*time.Second)
	b.Wait(t, `document.getElementById('issue-typing-summary').textContent.trim() === ''`, 2*time.Second)
	a.Wait(t, `getComputedStyle(document.getElementById('issue-edit-summary').closest('.tk-issue__editor')).display === 'none'`, time.Second)
}

// FR-13.2: a link with views.IssueLink opens the issue in the panel beside
// the page, with the issue's URL; closing it gives the page its URL back.
func TestIssuePanel(t *testing.T) {
	e := webtest.Start(t)
	d := issueData(t, e)
	var panel bytes.Buffer
	if err := views.IssuePanel("", false, views.Deletes{}).Render(context.Background(), &panel); err != nil {
		t.Fatal(err)
	}
	p := browsertest.Start(t).NewPage(t, 1280, 900)
	p.Navigate(t, e.URL+"/privacy")
	p.Wait(t, `document.querySelector('#reconnecting')?.hidden !== false && !!window.history`, 5*time.Second)
	p.Eval(t, `(() => {
		document.querySelector('main').insertAdjacentHTML('beforeend', `+jsonString(panel.String())+` + '<p><a id="row" href="/WEB-2" `+views.IssueLink+`="`+d.story+`">WEB-2</a></p>')
		return true
	})()`, nil)
	p.Wait(t, `!!document.getElementById('issue-panel')`, time.Second)
	time.Sleep(200 * time.Millisecond) // Datastar applies the new attributes
	p.Eval(t, `(window.scrollTo(0, 120), document.getElementById('row').click(), true)`, nil)
	p.Wait(t, `document.getElementById('issue-panel').open && location.pathname === '/WEB-2'`, 2*time.Second)
	p.Wait(t, `document.getElementById('issue-summary-text')?.textContent.startsWith('Cards glide')`, 3*time.Second)
	p.Wait(t, `document.querySelectorAll('#presence-issue-`+d.story+` li[data-who]').length === 1`, 3*time.Second)
	// Its controls work there too.
	choose(t, p, "issue-priority", "Low")
	p.Wait(t, priorityIs+`'Low'`, 2*time.Second)
	p.Eval(t, `(document.getElementById('issue-panel').close(), true)`, nil)
	p.Wait(t, `!document.getElementById('issue-panel').open && location.pathname === '/privacy'`, 2*time.Second)
	if errs := p.Errors(); len(errs) > 0 {
		t.Errorf("the page reported: %q", errs)
	}
}

// Screenshots of the issue page, the panel and the create dialog in both
// themes at 1440 and 375 pixels: TICKIT_SHOTS=<dir>.
func TestIssueShots(t *testing.T) {
	dir := os.Getenv("TICKIT_SHOTS")
	if dir == "" {
		t.Skip("TICKIT_SHOTS is not set")
	}
	e := webtest.Start(t)
	issueData(t, e)
	b := browsertest.Start(t)
	for _, w := range []int{1440, 375} {
		for _, theme := range []string{"dark", "light"} {
			p := b.NewPage(t, w, map[int]int{1440: 1000, 375: 812}[w])
			p.Navigate(t, e.URL+"/WEB-2")
			p.Wait(t, `document.querySelectorAll('#issue-presence-slot li[data-who]').length > 0`, 5*time.Second)
			p.Eval(t, `(document.documentElement.dataset.tkTheme = '`+theme+`', document.querySelector('meta[name=color-scheme]').content = '`+theme+`', document.getElementById('first-visit')?.remove(), true)`, nil)
			time.Sleep(300 * time.Millisecond)
			name := func(what string) string {
				return filepath.Join(dir, "issue-"+what+"-"+theme+"-"+strings.TrimSpace(itoa(w))+".png")
			}
			p.Screenshot(t, name("page"), true)
			p.Eval(t, `(document.getElementById('create').showModal(), true)`, nil)
			time.Sleep(300 * time.Millisecond)
			p.Screenshot(t, name("dialog"), false)
			p.Close(t)
		}
	}
}

func itoa(n int) string {
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// jsonString is s as a JavaScript string literal.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
