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
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
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

// choose opens a menu of the issue's details (sb-dropdown) and picks the
// item labelled label.
func choose(t *testing.T, p *browsertest.Page, menu, label string) {
	t.Helper()
	openMenu(t, p, menu)
	pick(t, p, menu, label)
}

// openMenu clicks a menu's trigger and waits for its rows.
func openMenu(t *testing.T, p *browsertest.Page, menu string) {
	t.Helper()
	p.Wait(t, `customElements.get('sb-dropdown') && !!document.getElementById('`+menu+`')?.shadowRoot?.querySelector('.trigger')`, 3*time.Second)
	p.Eval(t, `(document.getElementById('`+menu+`').shadowRoot.querySelector('.trigger').click(), true)`, nil)
	p.Wait(t, menuRows(menu)+`.length > 0`, 2*time.Second)
}

// menuRows are the rows of a menu's open list.
func menuRows(menu string) string {
	return `[...(document.getElementById('` + menu + `')?.shadowRoot?.querySelectorAll('.lvl0:popover-open [role^=menuitem]') ?? [])]`
}

func pick(t *testing.T, p *browsertest.Page, menu, label string) {
	t.Helper()
	p.Eval(t, `(`+menuRows(menu)+`.find(r => r.querySelector('.label').textContent.trim() === `+jsQuote(label)+`).click(), true)`, nil)
}

// menuShows is what a menu's trigger names: "Priority: High".
func menuShows(menu string) string {
	return `document.getElementById('` + menu + `')?.shadowRoot?.querySelector('.trigger')?.getAttribute('aria-label')`
}

const priorityIs = `document.getElementById('issue-priority')?.shadowRoot?.querySelector('.trigger')?.getAttribute('aria-label') === 'Priority: ' + `

const statusIs = `document.getElementById('issue-status')?.shadowRoot?.querySelector('.trigger')?.getAttribute('aria-label') === 'Status: ' + `

// typeKey types one letter into the focused element.
func typeKey(t *testing.T, p *browsertest.Page, key string) {
	t.Helper()
	p.Call(t, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": key, "text": key})
	p.Call(t, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": key})
}

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
		openMenu(t, p, "issue-priority")
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

// MP-08.3, MP-09.1, NFR-47: the priority menu by keyboard, and a conflict
// through it. The pick shows pending at once; refused as a conflict, the
// menu shows the server's value again, the notice beside it the person's;
// "Use Critical anyway" shows Critical pending until the server confirms it.
func TestPriorityMenuConflict(t *testing.T) {
	e := webtest.Start(t)
	d := issueData(t, e)
	a := openIssue(t, e, 1280, 900)
	b := openIssue(t, e, 1280, 900)
	faces := `document.querySelectorAll('#presence-issue-` + d.story + ` li[data-who]').length === 2`
	b.Wait(t, faces, 3*time.Second)
	const menu = `document.getElementById('issue-priority')`
	b.Wait(t, `customElements.get('sb-dropdown') && `+menu+`.getAttribute('value') === 'high' && `+priorityIs+`'High'`, 3*time.Second)

	// The keyboard opens b's menu on its first row, from the version it shows.
	b.Eval(t, `(`+menu+`.shadowRoot.querySelector('.trigger').focus(), true)`, nil)
	enter(t, b)
	b.Wait(t, menuRows("issue-priority")+`[0] === `+menu+`.shadowRoot.activeElement`, 2*time.Second)
	var rows []struct{ Name, Checked string }
	b.Eval(t, menuRows("issue-priority")+`.map(r => ({name: r.querySelector('.label').textContent.trim(), checked: r.getAttribute('aria-checked')}))`, &rows)
	if len(rows) != 4 || rows[0].Name != "Low" || rows[2].Name != "High" || rows[2].Checked != "true" || rows[3].Checked != "false" {
		t.Fatalf("the rows: %+v", rows)
	}

	// Someone else sets Medium meanwhile.
	choose(t, a, "issue-priority", "Medium")
	b.Wait(t, menu+`.getAttribute('value') === 'medium'`, 2*time.Second)

	// End, then Enter: Critical, pending at once.
	b.Eval(t, `(`+menu+`.addEventListener('sb-change', () => ((pending) => setTimeout(() => (window.sent = {pending, name: `+menuShows("issue-priority")+`})))(`+menu+`.matches(':state(pending)')), {once: true}), true)`, nil)
	b.Press(t, "End")
	enter(t, b)
	var sent struct {
		Pending bool
		Name    string
	}
	b.Wait(t, `!!window.sent`, 2*time.Second)
	b.Eval(t, `window.sent`, &sent)
	if !sent.Pending || sent.Name != "Priority: Critical" {
		t.Errorf("as it was sent the menu was %+v: want pending and Critical", sent)
	}

	conflict := `[...document.querySelectorAll('#issue-side .tk-issue__conflict')].find(n => !n.closest('[hidden]'))`
	b.Wait(t, conflict+`?.textContent.includes('changed the priority to Medium') && `+conflict+`.textContent.includes('Your Critical was not saved.')`, 3*time.Second)
	b.Wait(t, `!`+menu+`.matches(':state(pending)') && `+menu+`.value === 'medium' && `+priorityIs+`'Medium'`, 2*time.Second)
	b.Wait(t, conflict+`.querySelector('button').textContent.trim() === 'Use Critical anyway'`, time.Second)
	b.Eval(t, `(`+conflict+`.querySelector('button').click(), true)`, nil)
	b.Wait(t, `(`+menu+`.matches(':state(pending)') && `+priorityIs+`'Critical') || `+menu+`.getAttribute('value') === 'critical'`, 2*time.Second)
	a.Wait(t, priorityIs+`'Critical'`, 2*time.Second)
	b.Wait(t, `!`+conflict+` && !`+menu+`.matches(':state(pending)') && `+menu+`.getAttribute('value') === 'critical' && `+priorityIs+`'Critical'`, 2*time.Second)

	// Escape closes the menu without a change and gives the focus back.
	b.Eval(t, `(`+menu+`.shadowRoot.querySelector('.trigger').focus(), true)`, nil)
	b.Press(t, "ArrowDown")
	b.Wait(t, menuRows("issue-priority")+`.length === 4`, 2*time.Second)
	b.Press(t, "Escape")
	b.Wait(t, `!`+menu+`.open && `+menu+`.shadowRoot.activeElement === `+menu+`.shadowRoot.querySelector('.trigger')`, 2*time.Second)
	noErrors(t, a, b)
}

// FR-37.1, FR-58, MP-09.1, NFR-47: the status menu groups the workflow's
// next statuses under "Move to", which assistive tech reads as a named
// group; the keys and the type-ahead pass over the heading. A transition
// the workflow refuses shows pending, then the refusal, and the menu the
// server's status again.
func TestStatusMenuRefused(t *testing.T) {
	e := webtest.Start(t)
	d := issueData(t, e)
	p := openIssue(t, e, 1280, 900)
	const menu = `document.getElementById('issue-status')`
	p.Wait(t, `customElements.get('sb-dropdown') && `+menu+`.getAttribute('value') === 'inprogress' && `+statusIs+`'In Progress'`, 3*time.Second)

	p.Eval(t, `(`+menu+`.shadowRoot.querySelector('.trigger').focus(), true)`, nil)
	enter(t, p)
	focused := `(` + menu + `.shadowRoot.activeElement?.querySelector('.label')?.textContent.trim())`
	p.Wait(t, focused+` === 'Backlog'`, 2*time.Second)
	var rows []struct{ Name, Role, Checked string }
	p.Eval(t, menuRows("issue-status")+`.map(r => ({name: r.querySelector('.label').textContent.trim(), role: r.getAttribute('role'), checked: r.getAttribute('aria-checked')}))`, &rows)
	want := []string{"Backlog", "Selected for Development", "To Do", "In Progress", "In Review", "Blocked", "Done"}
	if len(rows) != len(want) {
		t.Fatalf("the rows: %+v", rows)
	}
	for i, r := range rows {
		if r.Name != want[i] || r.Role != "menuitemradio" || (r.Checked == "true") != (r.Name == "In Progress") {
			t.Errorf("row %d: %+v", i, r)
		}
	}
	var lozenge string
	p.Eval(t, menuRows("issue-status")+`[3].querySelector('.rich [part~=lozenge]')?.getAttribute('part')`, &lozenge)
	if lozenge != "hue lozenge status-progress" {
		t.Errorf("In Progress's lozenge has the parts %q", lozenge)
	}

	// The AX tree: the menu holds a group named by the heading, which
	// holds the rows.
	tree := p.Call(t, "Accessibility.getFullAXTree", map[string]any{})
	var ax struct {
		Nodes []struct {
			NodeID   string   `json:"nodeId"`
			ChildIDs []string `json:"childIds"`
			Ignored  bool     `json:"ignored"`
			Role     struct{ Value string }
			Name     struct{ Value string }
		}
	}
	if err := json.Unmarshal(tree, &ax); err != nil {
		t.Fatal(err)
	}
	roles := map[string]string{}
	for _, n := range ax.Nodes {
		roles[n.NodeID] = n.Role.Value
	}
	found := false
	for _, n := range ax.Nodes {
		if n.Role.Value != "group" || n.Name.Value != "Move to" {
			continue
		}
		radios := 0
		for _, c := range n.ChildIDs {
			if roles[c] == "menuitemradio" {
				radios++
			}
		}
		found = radios == len(want)
	}
	if !found {
		t.Error(`the AX tree has no group "Move to" with the seven rows`)
	}

	// Up wraps from the first row to the last, Down back, Home and End
	// land on rows; the heading never takes the focus.
	for _, k := range []struct{ key, want string }{{"ArrowUp", "Done"}, {"ArrowDown", "Backlog"}, {"End", "Done"}, {"Home", "Backlog"}} {
		p.Press(t, k.key)
		p.Wait(t, focused+` === '`+k.want+`'`, time.Second)
	}
	typeKey(t, p, "d")
	p.Wait(t, focused+` === 'Done'`, time.Second)

	// Someone makes Done follow only In Review while this menu still
	// offers it: the frame that would take Done out is held back here.
	p.Eval(t, `(`+menu+`.setAttribute('data-ignore-morph', ''), true)`, nil)
	e.Do(t, tracker.SetTransitionRule{By: tabs.By{Actor: people.Seeded[0], Op: "rule1", At: time.Now()}, Project: d.project, Status: tracker.StatusDone, From: []string{tracker.StatusInReview}})
	p.Eval(t, `(`+menu+`.addEventListener('sb-change', () => ((pending) => setTimeout(() => (window.sent = {pending, name: `+menuShows("issue-status")+`})))(`+menu+`.matches(':state(pending)')), {once: true}), true)`, nil)
	enter(t, p)
	var sent struct {
		Pending bool
		Name    string
	}
	p.Wait(t, `!!window.sent`, 2*time.Second)
	p.Eval(t, `window.sent`, &sent)
	if !sent.Pending || sent.Name != "Status: Done" {
		t.Errorf("as it was sent the menu was %+v: want pending and Done", sent)
	}
	refused := `[...document.querySelectorAll('#issue .tk-outcome[data-kind=refused]')].find(n => !n.closest('[hidden]'))`
	p.Wait(t, refused+`?.textContent.includes('Done can only follow In Review.')`, 3*time.Second)
	p.Wait(t, `!`+menu+`.matches(':state(pending)') && `+menu+`.value === 'inprogress' && `+statusIs+`'In Progress'`, 2*time.Second)

	// With the frames back, the menu no longer offers Done.
	p.Eval(t, `(`+menu+`.removeAttribute('data-ignore-morph'), true)`, nil)
	e.Do(t, tracker.SetPriority{By: tabs.By{Actor: people.Seeded[0], Op: "prio1", At: time.Now()}, Issue: d.story, Seen: 1 << 40, Priority: tracker.Low})
	p.Wait(t, priorityIs+`'Low'`, 3*time.Second)
	p.Wait(t, `![...`+menu+`.querySelectorAll('[slot=item]')].some(b => b.value === 'done')`, 2*time.Second)
	noErrors(t, p)
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
	// Its controls work there too, with the modules a page with the panel
	// loads (issueComponents), which /privacy does not.
	p.Eval(t, `fetch('/WEB-2').then((r) => r.text()).then((h) => import(h.match(/\/static\/vendor\/starbase\/dropdown\/dropdown\.[^"]+\.js/)[0])).then(() => true)`, nil)
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
