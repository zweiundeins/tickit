package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/browsertest"
	"github.com/mbolli/tickit/internal/tracker/trackertest"
	"github.com/mbolli/tickit/internal/web/webtest"
)

// boardPage opens the board of WEB in a new browser context, a guest of
// its own, and waits until its stream shows the viewer on the board.
func boardPage(t *testing.T, b *browsertest.Browser, url string, w, h int) *browsertest.Page {
	t.Helper()
	p := b.NewPage(t, w, h)
	p.Navigate(t, url)
	p.Wait(t, `document.querySelector('[id^="presence-boardhead"] [data-who]') && document.getElementById('board-lanes').dataset.rev !== ''`, 5*time.Second)
	return p
}

// laneOf is an expression: the status of the column a card is in.
func laneOf(key string) string {
	return `document.getElementById('c-` + key + `')?.closest('.tk-lane')?.dataset.lane`
}

// dragTo drags a card with the mouse to the middle of a column's foot.
func dragTo(t *testing.T, p *browsertest.Page, key, status string) {
	t.Helper()
	var r struct{ X, Y, TX, TY float64 }
	p.Eval(t, `(() => {
		const l = document.querySelector('.tk-lane[data-lane="`+status+`"]')
		l.scrollIntoView({inline: 'nearest', block: 'nearest', behavior: 'instant'})
		document.getElementById('c-`+key+`').scrollIntoView({inline: 'nearest', block: 'nearest', behavior: 'instant'})
		const c = document.getElementById('c-`+key+`').querySelector('.tk-card__key').getBoundingClientRect()
		const box = l.getBoundingClientRect()
		return {x: c.left + 4, y: c.top + c.height / 2, tx: box.left + box.width / 2, ty: box.bottom - 8}
	})()`, &r)
	mouse := func(typ string, x, y float64, buttons int) {
		p.Call(t, "Input.dispatchMouseEvent", map[string]any{"type": typ, "x": x, "y": y, "button": "left", "buttons": buttons, "clickCount": 1})
	}
	mouse("mousePressed", r.X, r.Y, 1)
	for i := 1; i <= 8; i++ {
		f := float64(i) / 8
		mouse("mouseMoved", r.X+(r.TX-r.X)*f, r.Y+(r.TY-r.Y)*f, 1)
	}
	p.Eval(t, `window.__dropAt = Date.now()`, nil)
	mouse("mouseReleased", r.TX, r.TY, 0)
}

// watchArrival makes the page note when the card lands in the column.
func watchArrival(t *testing.T, p *browsertest.Page, key, status string) {
	t.Helper()
	p.Eval(t, `(() => {
		window.__seenAt = 0
		const check = () => { if (!window.__seenAt && `+laneOf(key)+` === '`+status+`') window.__seenAt = Date.now() }
		new MutationObserver(check).observe(document.body, {subtree: true, childList: true})
		return true
	})()`, nil)
}

// MP-04.1, MP-04.2, section 12 item 4: a card dropped in one browser
// shows in its new column there at once, pending until its outcome, and
// in the others within 250 ms on loopback.
func TestBoardMoveLive(t *testing.T) {
	e := webtest.Start(t)
	pr := trackertest.Board(t, e.W, "WEB", 24)
	b := browsertest.Start(t)
	url := e.URL + "/projects/WEB/board"
	a := boardPage(t, b, url, 1600, 1000)
	others := []*browsertest.Page{boardPage(t, b, url, 1600, 1000), boardPage(t, b, url, 1600, 1000)}
	key := pr.Keys[1] // in To Do
	var from string
	a.Eval(t, laneOf(key), &from)
	if from != "todo" {
		t.Fatalf("%s is in %q", key, from)
	}
	for _, o := range others {
		watchArrival(t, o, key, "inreview")
	}
	a.Eval(t, `(() => { window.__pending = ''; new MutationObserver(() => { const c = document.querySelector('#c-`+key+`'); if (c?.dataset.state === 'pending' && !window.__pending) window.__pending = `+laneOf(key)+` }).observe(document.getElementById('board-lanes'), {subtree: true, attributes: true, attributeFilter: ['data-state']}); return true })()`, nil)
	dragTo(t, a, key, "inreview")
	a.Wait(t, `window.__pending === 'inreview'`, 2*time.Second)
	a.Wait(t, laneOf(key)+` === 'inreview' && !document.querySelector('#c-`+key+`').dataset.state`, 3*time.Second)
	var dropAt int64
	a.Eval(t, `window.__dropAt`, &dropAt)
	for i, o := range others {
		o.Wait(t, `window.__seenAt > 0`, 3*time.Second)
		var seenAt int64
		o.Eval(t, `window.__seenAt`, &seenAt)
		lag := time.Duration(seenAt-dropAt) * time.Millisecond
		t.Logf("browser %d saw the move %v after the drop", i+2, lag)
		if lag > 250*time.Millisecond {
			t.Errorf("browser %d saw the move after %v", i+2, lag)
		}
	}
	// MP-04.4: the same order everywhere.
	order := func(p *browsertest.Page) string {
		var s string
		p.Eval(t, `[...document.querySelectorAll('#board-lanes .tk-card')].map(c => c.id).join(' ')`, &s)
		return s
	}
	want := order(a)
	for i, o := range others {
		if got := order(o); got != want {
			t.Errorf("browser %d shows another order:\n%s\n%s", i+2, got, want)
		}
	}
	for _, p := range append(others, a) {
		if errs := p.Errors(); len(errs) > 0 {
			t.Errorf("the page reported: %q", errs)
		}
	}
}

// MP-04.3: of two moves of one card at once, the first wins; the second
// browser sees the card in the first one's place with the first mover's
// name beside it, and its own move stored nothing.
func TestBoardMoveConflict(t *testing.T) {
	e := webtest.Start(t)
	pr := trackertest.Board(t, e.W, "WEB", 24)
	b := browsertest.Start(t)
	url := e.URL + "/projects/WEB/board"
	a, c := boardPage(t, b, url, 1600, 1000), boardPage(t, b, url, 1600, 1000)
	key := pr.Keys[2]
	// Both pick a column from the card menu before either frame comes:
	// the menu runs in each page as a click would.
	pick := func(p *browsertest.Page, status string) string {
		return `(() => { document.querySelector('#c-` + key + ` > button').click(); document.querySelector('#card-menu [data-lane="` + status + `"]').click(); return true })()`
	}
	done := make(chan struct{})
	go func() { defer close(done); a.Eval(t, pick(a, "inreview"), nil) }()
	c.Eval(t, pick(c, "blocked"), nil)
	<-done
	var notes []string
	for _, p := range []*browsertest.Page{a, c} {
		p.Wait(t, `!document.querySelector('#c-`+key+`[data-state="pending"]')`, 3*time.Second)
		var note string
		p.Eval(t, `document.querySelector('.tk-bd__reason').hidden ? '' : document.querySelector('.tk-bd__reason').textContent`, &note)
		notes = append(notes, note)
	}
	var lanes [2]string
	a.Eval(t, laneOf(key), &lanes[0])
	c.Eval(t, laneOf(key), &lanes[1])
	if lanes[0] != lanes[1] {
		t.Fatalf("the browsers show %s in %v", key, lanes)
	}
	refused := 0
	for _, n := range notes {
		if n != "" {
			refused++
			if !strings.Contains(n, " moved "+key+" to ") || !strings.HasSuffix(n, " first.") {
				t.Errorf("the refusal says %q", n)
			}
		}
	}
	if refused != 1 {
		t.Errorf("refusals: %q (the card is in %s)", notes, lanes[0])
	}
	t.Logf("the card is in %s; notes %q", lanes[0], notes)
}

// FR-41.2, NFR-47.1: a card moves by keyboard alone through its menu.
func TestBoardKeyboardMove(t *testing.T) {
	e := webtest.Start(t)
	pr := trackertest.Board(t, e.W, "WEB", 24)
	b := browsertest.Start(t)
	p := boardPage(t, b, e.URL+"/projects/WEB/board", 1440, 900)
	key := pr.Keys[2]
	p.Eval(t, `document.querySelector('#c-`+key+` > button').focus()`, nil)
	enter(t, p)
	p.Wait(t, `document.getElementById('card-menu').matches(':popover-open')`, 2*time.Second)
	p.Eval(t, `document.querySelector('#card-menu [data-lane="inprogress"]').focus()`, nil)
	enter(t, p)
	p.Wait(t, laneOf(key)+` === 'inprogress' && !document.querySelector('#c-`+key+`').dataset.state`, 3*time.Second)
	var focus string
	p.Eval(t, `document.activeElement.closest('.tk-card')?.id || document.activeElement.tagName`, &focus)
	if focus != "c-"+key {
		t.Errorf("the focus is on %s after the move", focus)
	}
	// Move up, by the menu again.
	var before string
	p.Eval(t, `document.getElementById('c-`+key+`').previousElementSibling?.id || ''`, &before)
	if before != "" {
		p.Eval(t, `document.querySelector('#c-`+key+` > button').focus()`, nil)
		enter(t, p)
		p.Wait(t, `document.getElementById('card-menu').matches(':popover-open')`, 2*time.Second)
		p.Eval(t, `document.querySelector('#card-menu [data-move="up"]').focus()`, nil)
		enter(t, p)
		p.Wait(t, `document.getElementById('c-`+key+`').nextElementSibling?.id === '`+before+`' && !document.querySelector('#c-`+key+`').dataset.state`, 3*time.Second)
	}
	if errs := p.Errors(); len(errs) > 0 {
		t.Errorf("the page reported: %q", errs)
	}
}

// enter presses Enter as a keyboard does, with the character that
// activates a button.
func enter(t *testing.T, p *browsertest.Page) {
	t.Helper()
	p.Call(t, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13, "text": "\r"})
	p.Call(t, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13})
}

// MP-02.2, MP-02.3: a newcomer's face shows on the others' boards, and
// goes within a second of their tab closing.
func TestBoardPresence(t *testing.T) {
	e := webtest.Start(t)
	trackertest.Board(t, e.W, "WEB", 8)
	b := browsertest.Start(t)
	url := e.URL + "/projects/WEB/board"
	a := boardPage(t, b, url, 1440, 900)
	faces := `document.querySelectorAll('[id^="presence-boardhead"] [data-who]').length`
	a.Wait(t, faces+` === 1`, 2*time.Second)
	c := boardPage(t, b, url+"?type=bug", 1440, 900) // another filter, the same board
	a.Wait(t, faces+` === 2`, 2*time.Second)
	start := time.Now()
	c.Close(t)
	a.Wait(t, faces+` === 1`, 2*time.Second)
	if d := time.Since(start); d > time.Second {
		t.Errorf("the face went after %v", d)
	} else {
		t.Logf("the face went after %v", d)
	}
}

// FR-42: a quick filter shows only its cards, lives in the URL, and the
// page's stream follows it.
func TestBoardQuickFilter(t *testing.T) {
	e := webtest.Start(t)
	trackertest.Board(t, e.W, "WEB", 24)
	b := browsertest.Start(t)
	p := boardPage(t, b, e.URL+"/projects/WEB/board", 2400, 900)
	count := `document.querySelectorAll('#board-lanes .tk-card').length`
	var all, bugs int
	p.Eval(t, count, &all)
	p.Eval(t, `document.querySelector('[data-filter="types"][data-v="bug"]').click()`, nil)
	p.Wait(t, `location.search === '?type=bug'`, 2*time.Second)
	p.Wait(t, `[...document.querySelectorAll('#board-lanes i[data-type]')].every(t => t.dataset.type === 'bug')`, 3*time.Second)
	p.Eval(t, count, &bugs)
	if bugs == 0 || bugs >= all {
		t.Errorf("%d cards, %d bugs", all, bugs)
	}
	var views string
	p.Eval(t, `document.querySelector('[data-signals]') && document.getElementById('stream') && JSON.stringify(location.search)`, &views)
	p.Eval(t, `document.querySelector('[data-filter="clear"]').click()`, nil)
	p.Wait(t, fmt.Sprintf(`location.search === '' && %s === %d`, count, all), 3*time.Second)
	if errs := p.Errors(); len(errs) > 0 {
		t.Errorf("the page reported: %q", errs)
	}
}

// FR-40.3: a click on a card opens the issue in the side panel; a
// column's "+" opens the create dialog in that column's status.
func TestBoardPanelAndCreate(t *testing.T) {
	e := webtest.Start(t)
	pr := trackertest.Board(t, e.W, "WEB", 12)
	b := browsertest.Start(t)
	p := boardPage(t, b, e.URL+"/projects/WEB/board", 1440, 900)
	key := pr.Keys[1]
	p.Eval(t, `document.querySelector('#c-`+key+` .tk-card__title').click()`, nil)
	p.Wait(t, `document.getElementById('issue-panel').open && location.pathname === '/`+key+`'`, 3*time.Second)
	p.Wait(t, `document.getElementById('issue-panel').textContent.includes('`+key+`')`, 3*time.Second)
	p.Press(t, "Escape")
	p.Wait(t, `!document.getElementById('issue-panel').open && location.pathname === '/projects/WEB/board'`, 3*time.Second)
	p.Eval(t, `document.querySelector('.tk-lane[data-lane="inreview"] .tk-lane__add').click()`, nil)
	p.Wait(t, `document.getElementById('create').open`, 2*time.Second)
	if errs := p.Errors(); len(errs) > 0 {
		t.Errorf("the page reported: %q", errs)
	}
}

// FR-45.1, FR-45.2: a backlog row dragged to the top shows there at
// once and for another browser; checked rows go to the bottom at once.
func TestBacklogRank(t *testing.T) {
	e := webtest.Start(t)
	trackertest.Board(t, e.W, "WEB", 12)
	b := browsertest.Start(t)
	open := func() *browsertest.Page {
		p := b.NewPage(t, 1440, 900)
		p.Navigate(t, e.URL+"/projects/WEB/backlog")
		p.Wait(t, `document.querySelector('[id^="presence-backlog"] [data-who]') && document.getElementById('backlog').dataset.rev !== ''`, 5*time.Second)
		return p
	}
	a, c := open(), open()
	order := `[...document.querySelectorAll('#backlog .tk-blrow')].map(r => r.id).join(' ')`
	var before string
	a.Eval(t, order, &before)
	ids := strings.Fields(before)
	last := ids[len(ids)-1]
	var r struct{ X, Y, TX, TY float64 }
	a.Eval(t, `(() => { const s = document.getElementById('`+last+`').getBoundingClientRect(), f = document.getElementById('`+ids[0]+`').getBoundingClientRect(); return {x: s.left + 120, y: s.top + s.height / 2, tx: f.left + 120, ty: f.top + 4} })()`, &r)
	mouse := func(typ string, x, y float64, buttons int) {
		a.Call(t, "Input.dispatchMouseEvent", map[string]any{"type": typ, "x": x, "y": y, "button": "left", "buttons": buttons, "clickCount": 1})
	}
	mouse("mousePressed", r.X, r.Y, 1)
	for i := 1; i <= 8; i++ {
		f := float64(i) / 8
		mouse("mouseMoved", r.X+(r.TX-r.X)*f, r.Y+(r.TY-r.Y)*f, 1)
	}
	mouse("mouseReleased", r.TX, r.TY, 0)
	first := `document.querySelector('#backlog .tk-blrow').id === '` + last + `'`
	a.Wait(t, first, time.Second)
	a.Wait(t, `!document.querySelector('#backlog .tk-blrow[data-state]')`, 3*time.Second)
	c.Wait(t, first, 3*time.Second)
	// Two checked rows to the bottom.
	a.Eval(t, `(() => { const rows = document.querySelectorAll('#backlog .tk-blrow'); [rows[0], rows[1]].forEach(r => r.querySelector('.tk-blrow__check').click()); return true })()`, nil)
	a.Eval(t, `document.querySelector('[data-bulk="bottom"]').click()`, nil)
	end := `(() => { const r = [...document.querySelectorAll('#backlog .tk-blrow')].map(r => r.id); return r[r.length - 2] === '` + last + `' && r[r.length - 1] === '` + ids[0] + `' })()`
	c.Wait(t, end, 3*time.Second)
	a.Wait(t, end+` && !document.querySelector('.tk-blrow__check:checked')`, 3*time.Second)
	for _, p := range []*browsertest.Page{a, c} {
		if errs := p.Errors(); len(errs) > 0 {
			t.Errorf("the page reported: %q", errs)
		}
	}
}
