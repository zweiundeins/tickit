package e2e

import (
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker/trackertest"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// FR-37.1, FR-36.3: an admin restricts Done to follow In Review in the
// project's settings; another tab's board follows within a tick: its card
// menu no longer lists Done for a card in To Do, a drag dims the Done
// column, and a drop there is refused with the rule in words while the
// card goes back.
func TestWorkflowRuleLive(t *testing.T) {
	e := webtest.Start(t)
	pr := trackertest.Board(t, e.W, "WEB", 24)
	e.Account(t, "ada@example.com", "correct horse battery", people.Admin)
	b := browsertest.Start(t)

	admin := b.NewPage(t, 1280, 900)
	admin.Navigate(t, e.URL+"/login?next=/projects/WEB/settings")
	admin.Eval(t, `(() => {
		for (const [id, v] of [['login-email', 'ada@example.com'], ['login-password', 'correct horse battery']]) {
			const input = document.getElementById(id)
			input.value = v
			input.dispatchEvent(new Event('input', {bubbles: true}))
		}
		document.querySelector('.tk-login button[type=submit]').click()
		return true
	})()`, nil)
	admin.Wait(t, `location.pathname === '/projects/WEB/settings' && document.querySelector('#wf-statuses tr[data-status="done"]') !== null`, 5*time.Second)

	board := boardPage(t, b, e.URL+"/projects/WEB/board", 1600, 1000)
	key := pr.Keys[1] // in To Do
	var from string
	board.Eval(t, laneOf(key), &from)
	if from != "todo" {
		t.Fatalf("%s is in %q", key, from)
	}

	// The admin sets the rule through the dialog, as a person would.
	admin.Eval(t, `(() => {
		document.querySelector('#wf-statuses tr[data-status="done"] [aria-label="Which statuses Done may follow"]').click()
		return true
	})()`, nil)
	admin.Wait(t, `document.getElementById('wf-rule').open && document.querySelectorAll('#wf-rule-from input').length === 6`, 3*time.Second)
	admin.Eval(t, `(document.getElementById('wf-any').click(), true)`, nil)
	admin.Wait(t, `!document.querySelector('#wf-rule .tk-wf__from').disabled`, 2*time.Second)
	admin.Eval(t, `(() => {
		document.querySelectorAll('#wf-rule-from input').forEach((i) => { if (i.checked !== (i.value === 'inreview')) i.click() })
		document.querySelector('#wf-rule .tk-dialog__foot button[type=submit]').click()
		return true
	})()`, nil)
	admin.Wait(t, `!document.getElementById('wf-rule').open && document.querySelector('#wf-statuses tr[data-status="done"] .tk-wf__follows').textContent === 'In Review'`, 3*time.Second)

	// The board's tab follows: the rules arrive with the lanes.
	board.Wait(t, `document.getElementById('board-lanes').dataset.rules !== undefined`, 2*time.Second)
	board.Eval(t, `(() => { document.querySelector('#c-`+key+` > button').click(); return true })()`, nil)
	board.Wait(t, `document.getElementById('card-menu').matches(':popover-open')`, 2*time.Second)
	var listed []string
	board.Eval(t, `[...document.querySelectorAll('#card-menu [data-lane]')].filter((i) => !i.hidden).map((i) => i.dataset.lane)`, &listed)
	for _, l := range listed {
		if l == "done" {
			t.Errorf("the card menu lists Done: %v", listed)
		}
	}
	if len(listed) != 5 {
		t.Errorf("the card menu lists %v", listed)
	}
	board.Eval(t, `(document.getElementById('card-menu').hidePopover(), true)`, nil)

	// A drag dims Done; the drop is refused in words and the card goes back.
	var r struct{ X, Y, TX, TY float64 }
	board.Eval(t, `(() => {
		const l = document.querySelector('.tk-lane[data-lane="done"]')
		l.scrollIntoView({inline: 'nearest', block: 'nearest', behavior: 'instant'})
		const c = document.getElementById('c-`+key+`').querySelector('.tk-card__key').getBoundingClientRect()
		const box = l.getBoundingClientRect()
		return {x: c.left + 4, y: c.top + c.height / 2, tx: box.left + box.width / 2, ty: box.bottom - 8}
	})()`, &r)
	mouse := func(typ string, x, y float64, buttons int) {
		board.Call(t, "Input.dispatchMouseEvent", map[string]any{"type": typ, "x": x, "y": y, "button": "left", "buttons": buttons, "clickCount": 1})
	}
	mouse("mousePressed", r.X, r.Y, 1)
	for i := 1; i <= 8; i++ {
		f := float64(i) / 8
		mouse("mouseMoved", r.X+(r.TX-r.X)*f, r.Y+(r.TY-r.Y)*f, 1)
	}
	var dimmed []string
	board.Eval(t, `[...document.querySelectorAll('.tk-lane[data-forbidden]')].map((l) => l.dataset.lane)`, &dimmed)
	if len(dimmed) != 1 || dimmed[0] != "done" {
		t.Errorf("while dragging, the dimmed columns are %v", dimmed)
	}
	mouse("mouseReleased", r.TX, r.TY, 0)
	board.Wait(t, `document.querySelector('.tk-bd__reason').textContent === 'Done can only follow In Review.' && !document.querySelector('.tk-bd__reason').hidden`, 3*time.Second)
	board.Wait(t, laneOf(key)+` === 'todo' && document.querySelectorAll('.tk-lane[data-forbidden]').length === 0`, 3*time.Second)
	var status string
	if err := e.DB.R.QueryRow(`SELECT status FROM issues WHERE key = ?`, key).Scan(&status); err != nil || status != "todo" {
		t.Errorf("the issue is in %q (%v)", status, err)
	}
	for _, p := range []*browsertest.Page{admin, board} {
		if errs := p.Errors(); len(errs) > 0 {
			t.Errorf("the page reported: %q", errs)
		}
	}
}

// FR-44: the board's swimlanes are the tab's: by assignee in one tab, a
// row per person, while another tab keeps its columns; a move by menu in
// a row keeps the card in it.
func TestBoardSwimlanesTab(t *testing.T) {
	e := webtest.Start(t)
	pr := trackertest.Board(t, e.W, "WEB", 24)
	b := browsertest.Start(t)
	rows := boardPage(t, b, e.URL+"/projects/WEB/board", 1600, 1000)
	flat := boardPage(t, b, e.URL+"/projects/WEB/board", 1600, 1000)
	rows.Eval(t, `(() => { document.querySelector('[data-filter="group"][data-v="assignee"]').click(); return true })()`, nil)
	rows.Wait(t, `location.search === '?lanes=assignee' && document.querySelectorAll('#board-lanes .tk-swim[aria-label]').length > 1`, 3*time.Second)
	var n int
	flat.Eval(t, `document.querySelectorAll('#board-lanes .tk-swim').length`, &n)
	if n != 0 {
		t.Errorf("the other tab shows %d rows", n)
	}
	key := pr.Keys[2]
	var row string
	rows.Eval(t, `document.getElementById('c-`+key+`').closest('.tk-swim').getAttribute('aria-label')`, &row)
	rows.Eval(t, `(() => { document.querySelector('#c-`+key+` > button').click(); document.querySelector('#card-menu [data-lane="inreview"]').click(); return true })()`, nil)
	rows.Wait(t, laneOf(key)+` === 'inreview' && !document.querySelector('#c-`+key+`').dataset.state`, 3*time.Second)
	var after string
	rows.Eval(t, `document.getElementById('c-`+key+`').closest('.tk-swim').getAttribute('aria-label')`, &after)
	if after != row {
		t.Errorf("the card left its row %q for %q", row, after)
	}
	flat.Wait(t, laneOf(key)+` === 'inreview'`, 3*time.Second)
	rows.Navigate(t, e.URL+"/projects/WEB/board?lanes=assignee")
	rows.Wait(t, `document.querySelectorAll('#board-lanes .tk-swim[aria-label]').length > 1`, 3*time.Second)
	for _, p := range []*browsertest.Page{rows, flat} {
		if errs := p.Errors(); len(errs) > 0 {
			t.Errorf("the page reported: %q", errs)
		}
	}
}
