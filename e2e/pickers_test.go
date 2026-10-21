package e2e

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// Starbase's form components on real pages: sb-select, sb-date-picker and
// sb-copy-button, driven by real key events.

// key presses a key that types no text: "ArrowRight", "Escape".
func key(t *testing.T, p *browsertest.Page, name string) {
	t.Helper()
	codes := map[string]int{"ArrowLeft": 37, "ArrowUp": 38, "ArrowRight": 39, "ArrowDown": 40, "Escape": 27, "Tab": 9, "Home": 36, "End": 35}
	for _, typ := range []string{"rawKeyDown", "keyUp"} {
		p.Call(t, "Input.dispatchKeyEvent", map[string]any{"type": typ, "key": name, "code": name, "windowsVirtualKeyCode": codes[name]})
	}
}

// typeKeys types text into the focused element, one key at a time.
func typeKeys(t *testing.T, p *browsertest.Page, text string) {
	t.Helper()
	for _, r := range text {
		p.Call(t, "Input.insertText", map[string]any{"text": string(r)})
		time.Sleep(40 * time.Millisecond)
	}
}

// focusIn focuses the element sel names inside the shadow root of the
// element with id.
func focusIn(t *testing.T, p *browsertest.Page, id, sel string) {
	t.Helper()
	p.Eval(t, `(() => { document.getElementById(`+jsQuote(id)+`).shadowRoot.querySelector(`+jsQuote(sel)+`).focus(); return true })()`, nil)
}

func noErrors(t *testing.T, pages ...*browsertest.Page) {
	t.Helper()
	for _, p := range pages {
		if errs := p.Errors(); len(errs) > 0 {
			t.Errorf("the page reported %q", errs)
		}
	}
}

// FR-17.1, MP-03.3, MP-09.1: the assignee is picked by typing a name into
// sb-select; a frame that someone else's change brings leaves the open
// list and the typed search alone; the other page shows the new assignee.
func TestPickAssigneeByTyping(t *testing.T) {
	e := webtest.Start(t)
	d := issueData(t, e)
	a := openIssue(t, e, 1280, 900)
	b := openIssue(t, e, 1280, 900)
	faces := `document.querySelectorAll('#presence-issue-` + d.story + ` li[data-who]').length === 2`
	a.Wait(t, faces, 3*time.Second)
	a.Wait(t, `customElements.get('sb-select') && document.getElementById('issue-assignee').value === 's-priya'`, 3*time.Second)

	// Tab into the picker, as the keyboard would: typing searches the typed
	// name alone, not "Priya Raman" with it.
	focusIn(t, a, "issue-assignee", "input")
	typeKeys(t, a, "tom")
	open := `document.getElementById('issue-assignee').shadowRoot`
	a.Wait(t, open+`.querySelector('[popover]').matches(':popover-open') && `+open+`.querySelector('input').value === 'tom'`, 2*time.Second)

	choose(t, b, "issue-priority", "Low")
	a.Wait(t, priorityIs+`'Low'`, 2*time.Second)
	a.Wait(t, open+`.querySelector('[popover]').matches(':popover-open') && `+open+`.querySelector('input').value === 'tom' && document.activeElement?.id === 'issue-assignee'`, time.Second)

	enter(t, a)
	b.Wait(t, `document.getElementById('issue-assignee').value === 's-tomasz' && document.querySelector('#issue-side .tk-issue__assignee').dataset.assignee === 's-tomasz'`, 3*time.Second)
	a.Wait(t, `document.getElementById('issue-assignee').getAttribute('value') === 's-tomasz' && !document.getElementById('issue-assignee').matches(':state(pending)')`, 3*time.Second)
	a.Wait(t, open+`.querySelector('input').value === 'Tomasz Nowak'`, time.Second)

	// After an Enter pick, typing searches the typed text alone too; Escape
	// on an open list closes it and leaves the value.
	typeKeys(t, a, "pri")
	a.Wait(t, open+`.querySelector('[popover]').matches(':popover-open') && `+open+`.querySelector('input').value === 'pri'`, 2*time.Second)
	key(t, a, "Escape")
	a.Wait(t, `!`+open+`.querySelector('[popover]').matches(':popover-open') && document.getElementById('issue-assignee').value === 's-tomasz'`, time.Second)
	noErrors(t, a, b)
}

// FR-25, NFR-47: the due date is set with sb-date-picker by keyboard: Enter
// on its button opens the calendar on the date, Right moves a day, Enter
// picks it; the other page shows the new date.
func TestDueDateByKeyboard(t *testing.T) {
	e := webtest.Start(t)
	issueData(t, e)
	a := openIssue(t, e, 1280, 900)
	b := openIssue(t, e, 1280, 900)
	due := time.Now().AddDate(0, 0, 4).Format(time.DateOnly)
	want := time.Now().AddDate(0, 0, 5).Format(time.DateOnly)
	a.Wait(t, `customElements.get('sb-date-picker') && document.getElementById('issue-due').value === '`+due+`'`, 3*time.Second)

	focusIn(t, a, "issue-due", "[part~=button]")
	enter(t, a)
	cal := `document.getElementById('issue-due').shadowRoot`
	a.Wait(t, cal+`.getElementById('cal').matches(':popover-open') && `+cal+`.activeElement?.localName === 'td'`, 2*time.Second)
	key(t, a, "ArrowRight")
	enter(t, a)
	b.Wait(t, `document.getElementById('issue-due').getAttribute('value') === '`+want+`'`, 3*time.Second)
	a.Wait(t, `!`+cal+`.getElementById('cal').matches(':popover-open') && !document.getElementById('issue-due').matches(':state(pending)')`, 3*time.Second)
	noErrors(t, a, b)
}

// FR-12: "Copy link" on the issue page and on a page in view copies the
// address with sb-copy-button, by keyboard, and says so beside itself.
func TestCopyLink(t *testing.T) {
	e, _ := seededDemo(t)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1280, 900)
	var home string
	e.DB.R.QueryRow(`SELECT id FROM pages WHERE title = 'Deploy to production'`).Scan(&home)
	for _, c := range []struct{ path, label, want string }{
		{"/WEB-2", "Copy link to WEB-2", "/WEB-2"},
		{"/spaces/ENG/pages/" + home + "/x", "Copy link to this page", "/spaces/ENG/pages/" + home + "/deploy-to-production"},
	} {
		p.Navigate(t, e.URL+c.path)
		sel, _ := json.Marshal(`sb-copy-button[label="` + c.label + `"]`)
		btn := `document.querySelector(` + string(sel) + `)`
		p.Wait(t, `customElements.get('sb-copy-button') && `+btn+`?.shadowRoot?.querySelector('button')`, 5*time.Second)
		p.Eval(t, `(() => { window.copied = ''; document.addEventListener('sb-copy', (e) => window.copied = e.detail.value); `+btn+`.shadowRoot.querySelector('button').focus(); return true })()`, nil)
		enter(t, p)
		p.Wait(t, `window.copied === location.origin + `+jsQuote(c.want)+` && `+btn+`.value === window.copied`, 3*time.Second)
		p.Wait(t, btn+`.shadowRoot.querySelector('[role=status]').textContent === 'Link copied'`, time.Second)
	}
	noErrors(t, p)
}

// FR-51, FR-55: the list's people filter is a picker to type a name in,
// and a date range is two sb-date-pickers; both put their values in the
// address, and Back sets them again.
func TestListPickers(t *testing.T) {
	e := webtest.Start(t)
	issueData(t, e)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1280, 900)
	p.Navigate(t, e.URL+"/issues")
	p.Wait(t, `customElements.get('sb-select') && document.querySelectorAll('#issue-list tbody tr').length === 5`, 5*time.Second)

	p.Eval(t, `(document.querySelector('[popovertarget=list-f-assignee]').click(), true)`, nil)
	focusIn(t, p, "list-f-assignee-pick", "input")
	typeKeys(t, p, "pri")
	enter(t, p)
	p.Wait(t, `location.search === '?assignee=s-priya' && document.querySelector('[data-count=assignee]').textContent === '1'`, 3*time.Second)
	p.Wait(t, rowKeys+`.join() === 'WEB-2'`, 3*time.Second)
	key(t, p, "Escape") // the list
	key(t, p, "Escape") // the popover
	p.Wait(t, `!document.getElementById('list-f-assignee').matches(':popover-open')`, time.Second)

	today := time.Now().Format(time.DateOnly)
	p.Eval(t, `(document.querySelector('[popovertarget=list-f-created]').click(), true)`, nil)
	focusIn(t, p, "list-f-created-from", "[part~=button]")
	enter(t, p)
	p.Wait(t, `document.getElementById('list-f-created-from').shadowRoot.activeElement?.localName === 'td'`, 2*time.Second)
	enter(t, p)
	p.Wait(t, `location.search === '?assignee=s-priya&created_from=`+today+`' && document.querySelector('[data-count=created]').textContent === '1'`, 3*time.Second)

	p.Eval(t, `(history.back(), true)`, nil)
	p.Wait(t, `location.search === '?assignee=s-priya' && document.getElementById('list-f-created-from').value === '' && JSON.stringify(document.getElementById('list-f-assignee-pick').value) === '["s-priya"]' && document.querySelector('[data-count=created]').hidden`, 3*time.Second)
	p.Eval(t, `([...document.querySelectorAll('#list-filter button')].find((b) => b.textContent.trim() === 'Clear').click(), true)`, nil)
	p.Wait(t, `location.search === '' && JSON.stringify(document.getElementById('list-f-assignee-pick').value) === '[]'`, 3*time.Second)
	noErrors(t, p)
}

// FR-10: the create dialog picks the assignee by typing a name; the new
// issue has them.
func TestCreatePickAssignee(t *testing.T) {
	e := webtest.Start(t)
	issueData(t, e)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1280, 900)
	p.Navigate(t, e.URL+"/issues")
	p.Wait(t, `customElements.get('sb-select') && document.getElementById('create-assignee')?.shadowRoot`, 5*time.Second)
	p.Eval(t, `(document.querySelector('[commandfor=create]').click(), true)`, nil)
	p.Wait(t, `document.activeElement?.id === 'create-summary'`, 2*time.Second)
	typeKeys(t, p, "Picked by name")
	focusIn(t, p, "create-assignee", "input")
	typeKeys(t, p, "tom")
	enter(t, p)
	p.Wait(t, `document.getElementById('create-assignee').value === 's-tomasz'`, 2*time.Second)
	p.Eval(t, `(document.querySelector('#create .tk-btn--primary').click(), true)`, nil)
	p.Wait(t, `document.querySelector('#issue-panel #issue-assignee')?.getAttribute('value') === 's-tomasz'`, 5*time.Second)
	var summary string
	p.Eval(t, `document.getElementById('issue-summary-text')?.textContent`, &summary)
	if summary != "Picked by name" {
		t.Errorf("the panel shows %q", summary)
	}
	noErrors(t, p)
}
