package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// rowOf is the list's row of an issue key.
func rowOf(key string) string {
	return `[...document.querySelectorAll('#issue-list tbody tr')].find(r => r.querySelector('td.tk-key')?.textContent === ` + jsQuote(key) + `)`
}

// FR-58.1, FR-58.2, FR-16.1, NFR-47: an account selects three issues in
// the list, by keyboard and by pointer, and sets their status at once;
// the one in a project where it is a Viewer is refused, named with why
// and stays selected, and another person's list shows the others' new
// status without a reload.
func TestBulkStatusLive(t *testing.T) {
	e := webtest.Start(t)
	bea := e.Account(t, "bea@example.com", "correct horse battery", people.NoRole)
	ada := people.Seeded[0]
	n := 0
	do := func(cmd writer.ResultCommand) string {
		b, err := e.W.DoValue(context.Background(), func(tx *writer.Tx) ([]byte, error) { return cmd.ApplyResult(tx) })
		if err != nil {
			t.Fatalf("%T: %v", cmd, err)
		}
		var m map[string]string
		json.Unmarshal(b, &m)
		return m["id"]
	}
	by := func() tabs.By {
		n++
		return tabs.By{Actor: ada, Op: fmt.Sprint("seed", n), At: time.Now().Add(time.Duration(n-100) * time.Minute)}
	}
	e.Do(t, people.SeedPeople{At: time.Now()})
	web := do(tracker.CreateProject{By: by(), Key: "WEB", Name: "Website", Open: true})
	sec := do(tracker.CreateProject{By: by(), Key: "SEC", Name: "Secret"})
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		return access.SetMember(tx, access.ProjectScope(sec), bea, access.Viewer, ada.ID, time.Now())
	}))
	for _, s := range []string{"Fix the login", "Cart total is wrong"} {
		do(tracker.CreateIssue{By: by(), Project: web, Type: tracker.Task, Summary: s})
	}
	do(tracker.CreateIssue{By: by(), Project: sec, Type: tracker.Task, Summary: "Rotate the keys"})

	b := browsertest.Start(t)
	other := b.NewPage(t, 1280, 900)
	other.Navigate(t, e.URL+"/issues?status=inprogress")
	other.Wait(t, `document.querySelector('#issue-list') !== null`, 5*time.Second)
	p := staffPage(t, e, b, "bea@example.com", 1280)
	p.Navigate(t, e.URL+"/issues")
	p.Wait(t, rowKeys+`.join() === 'SEC-1,WEB-2,WEB-1'`, 5*time.Second)
	waitStreams(t, e, 2)

	// A Space on the focused box selects it; two clicks select the others.
	p.Eval(t, `(`+rowOf("WEB-1")+`.querySelector('.list-check').focus(), true)`, nil)
	for _, typ := range []string{"keyDown", "keyUp"} {
		p.Call(t, "Input.dispatchKeyEvent", map[string]any{"type": typ, "key": " ", "code": "Space", "windowsVirtualKeyCode": 32, "text": " "})
	}
	p.Wait(t, `document.querySelector('.list-bulk__count').textContent === '1 issue selected'`, 5*time.Second)
	p.Eval(t, `(`+rowOf("WEB-2")+`.querySelector('.list-check').click(), `+rowOf("SEC-1")+`.querySelector('.list-check').click(), true)`, nil)
	p.Wait(t, `document.querySelector('.list-bulk__count').textContent === '3 issues selected' && document.querySelector('.list-check-all').checked`, 5*time.Second)

	p.Eval(t, `(document.querySelector('[popovertarget=bulk-status]').click(), true)`, nil)
	p.Wait(t, `document.getElementById('bulk-status').matches(':popover-open')`, 5*time.Second)
	p.Eval(t, `(document.querySelector('#bulk-status [data-value=inprogress]').click(), true)`, nil)

	want := "2 issues changed. 1 was not:\nSEC-1: Only members of this project can change its issues."
	quoted, _ := json.Marshal(want)
	p.Wait(t, `document.querySelector('.list-bulk__msg').textContent === `+string(quoted)+` && !document.querySelector('.list-bulk__msg').hidden`, 5*time.Second)
	p.Wait(t, rowOf("WEB-1")+`.textContent.includes('In Progress') && !`+rowOf("WEB-1")+`.querySelector('.list-check').checked && `+rowOf("SEC-1")+`.querySelector('.list-check').checked`, 5*time.Second)
	p.Wait(t, `document.querySelector('.list-bulk__count').textContent === '1 issue selected'`, 5*time.Second)
	other.Wait(t, rowKeys+`.join() === 'WEB-2,WEB-1'`, 5*time.Second)

	// By keyboard: Enter opens a menu on its first item, Down goes to the
	// next, Enter sends it; every issue refused for want of a right is a
	// 403 whose outcome still names them.
	p.Eval(t, `(document.querySelector('[popovertarget=bulk-priority]').focus(), true)`, nil)
	enter(t, p)
	p.Wait(t, `document.activeElement?.dataset.value === 'critical'`, 5*time.Second)
	for _, typ := range []string{"rawKeyDown", "keyUp"} {
		p.Call(t, "Input.dispatchKeyEvent", map[string]any{"type": typ, "key": "ArrowDown", "code": "ArrowDown", "windowsVirtualKeyCode": 40})
	}
	p.Wait(t, `document.activeElement?.dataset.value === 'high'`, 5*time.Second)
	enter(t, p)
	quoted, _ = json.Marshal("No issue was changed.\nSEC-1: Only members of this project can change its issues.")
	p.Wait(t, `document.querySelector('.list-bulk__msg').textContent === `+string(quoted)+` && document.querySelector('.list-bulk__msg').dataset.kind === 'refused'`, 5*time.Second)
	// SEC-1 stays selected, and the focus stays on the menu's button,
	// which was aria-disabled while the change was on its way.
	p.Wait(t, `document.activeElement?.getAttribute('popovertarget') === 'bulk-priority' && !document.activeElement.disabled && document.activeElement.getAttribute('aria-disabled') !== 'true'`, time.Second)

	for _, pg := range []*browsertest.Page{p, other} {
		for _, err := range pg.Errors() {
			if !strings.Contains(err, "status of 403") || !strings.HasSuffix(err, "/cmd/issues/bulk") { // the denial's answer
				t.Errorf("the page reported: %q", err)
			}
		}
	}
	var lines string
	p.Eval(t, `document.querySelector('.list-bulk__msg').innerText`, &lines)
	if strings.Count(lines, "\n") != 1 {
		t.Errorf("the message shows as %q, not on two lines", lines)
	}
}

// FR-58.1, NFR-47: the bulk bar's Assign is a picker: Enter on Assign
// opens it with the focus in its search, a typed name and Enter assign
// the selected issues, and the picker is empty again for the next time.
func TestBulkAssignByTyping(t *testing.T) {
	e := webtest.Start(t)
	issueData(t, e)
	p := browsertest.Start(t).NewPage(t, 1280, 900)
	p.Navigate(t, e.URL+"/issues?sort=key&dir=asc")
	p.Wait(t, `document.getElementById('reconnecting')?.hidden && customElements.get('sb-select') && `+rowKeys+`.length >= 2`, 10*time.Second)
	var keys []string
	p.Eval(t, rowKeys, &keys)
	p.Eval(t, `(`+rowOf(keys[0])+`.querySelector('.list-check').click(), `+rowOf(keys[1])+`.querySelector('.list-check').click(), true)`, nil)
	p.Wait(t, `document.querySelector('.list-bulk__count').textContent === '2 issues selected'`, 5*time.Second)

	pick := `document.getElementById('bulk-assignee')`
	p.Eval(t, `(document.querySelector('[popovertarget=bulk-assign]').focus(), true)`, nil)
	enter(t, p)
	p.Wait(t, `document.getElementById('bulk-assign').matches(':popover-open') && document.activeElement === `+pick+` && `+pick+`.shadowRoot.activeElement?.localName === 'input'`, 5*time.Second)
	typeKeys(t, p, "tom")
	p.Wait(t, pick+`.shadowRoot.querySelector('[popover]').matches(':popover-open') && [...`+pick+`.shadowRoot.querySelectorAll('[role=option]')].filter((o) => o.checkVisibility()).map((o) => o.textContent.trim()).join() === 'Tomasz Nowak'`, 2*time.Second)
	shoot(t, p, "bulk-assign")
	enter(t, p)
	p.Wait(t, `!document.getElementById('bulk-assign').matches(':popover-open') && document.querySelector('.list-bulk__msg').textContent === '2 issues changed.'`, 5*time.Second)
	p.Wait(t, rowOf(keys[0])+`.textContent.includes('Tomasz Nowak') && `+rowOf(keys[1])+`.textContent.includes('Tomasz Nowak')`, 5*time.Second)
	var left string
	p.Eval(t, pick+`.value`, &left)
	if left != "" {
		t.Errorf("the picker kept %q", left)
	}
	// Nothing is selected any more and the bar's buttons are disabled: the
	// focus is on the page's box, not lost to the document (NFR-47).
	p.Wait(t, `document.activeElement?.classList.contains('list-check-all')`, time.Second)

	// So it is after Clear selection by keyboard.
	p.Eval(t, `(`+rowOf(keys[0])+`.querySelector('.list-check').click(), true)`, nil)
	p.Wait(t, `document.querySelector('.list-bulk__count').textContent === '1 issue selected'`, 5*time.Second)
	p.Eval(t, `(document.querySelector('[data-bulk-clear]').focus(), true)`, nil)
	enter(t, p)
	p.Wait(t, `document.querySelector('.list-bulk__count').textContent === 'Select issues to change several at once.' && document.activeElement?.classList.contains('list-check-all')`, time.Second)
	noErrors(t, p)
}
