package e2e

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/tracker/trackertest"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// touchPage opens a page that emulates a touch screen: touch events,
// a coarse pointer and no hover, as a phone's browser has.
func touchPage(t *testing.T, b *browsertest.Browser, w, h int) *browsertest.Page {
	t.Helper()
	p := b.NewPage(t, w, h)
	p.Call(t, "Emulation.setTouchEmulationEnabled", map[string]any{"enabled": true, "maxTouchPoints": 5})
	p.Call(t, "Emulation.setDeviceMetricsOverride", map[string]any{"width": w, "height": h, "deviceScaleFactor": 2, "mobile": true})
	return p
}

// noTouchInFirefox skips a touch test in Firefox: touch emulation (a
// touch screen's media features and events) is a DevTools command that
// WebDriver BiDi has no counterpart for.
func noTouchInFirefox(t *testing.T) {
	browsertest.SkipFirefox(t, "touch emulation (Emulation.setTouchEmulationEnabled) has no WebDriver BiDi counterpart")
}

// touch sends one touch event with a finger at x, y (none for touchEnd).
func touch(t *testing.T, p *browsertest.Page, typ string, x, y float64) {
	t.Helper()
	pts := []map[string]any{{"x": x, "y": y, "id": 1, "radiusX": 4, "radiusY": 4, "force": 1}}
	if typ == "touchEnd" {
		pts = []map[string]any{}
	}
	p.Call(t, "Input.dispatchTouchEvent", map[string]any{"type": typ, "touchPoints": pts})
}

// tap touches the middle of the box an expression returns ({x, y, w, h}).
func tap(t *testing.T, p *browsertest.Page, box string) {
	t.Helper()
	var r struct{ X, Y, W, H float64 }
	p.Eval(t, `(() => { const r = (`+box+`).getBoundingClientRect(); return {x: r.left, y: r.top, w: r.width, h: r.height} })()`, &r)
	if r.W == 0 || r.H == 0 {
		t.Fatalf("nothing to tap at %s", box)
	}
	touch(t, p, "touchStart", r.X+r.W/2, r.Y+r.H/2)
	time.Sleep(30 * time.Millisecond)
	touch(t, p, "touchEnd", 0, 0)
}

// NFR-56.1: on a touch screen a finger on a card's grip drags the card to
// another column; the move is stored, so another browser's board shows it.
func TestBoardTouchDrag(t *testing.T) {
	noTouchInFirefox(t)
	e := webtest.Start(t)
	pr := trackertest.Board(t, e.W, "WEB", 24)
	b := browsertest.Start(t)
	url := e.URL + "/projects/WEB/board"
	p := touchPage(t, b, 600, 900)
	p.Navigate(t, url)
	p.Wait(t, `document.querySelector('[id^="presence-boardhead"] [data-who]') && document.getElementById('board-lanes').dataset.rev !== ''`, 5*time.Second)
	other := boardPage(t, b, url, 1600, 1000)
	var coarse bool
	p.Eval(t, `matchMedia('(pointer: coarse)').matches`, &coarse)
	if !coarse {
		t.Fatal("the emulated touch screen has no coarse pointer")
	}
	key := pr.Keys[1] // in To Do
	var from string
	p.Eval(t, laneOf(key), &from)
	if from != "todo" {
		t.Fatalf("%s is in %q", key, from)
	}
	const to = "inprogress"
	watchArrival(t, other, key, to)
	// NFR-56.1: columns at least 260 pixels wide, the board scrolling sideways.
	var lane struct{ W, Scroll, Client float64 }
	p.Eval(t, `(() => { const l = document.querySelector('.tk-lane'), s = document.getElementById('board-lanes'); return {w: l.getBoundingClientRect().width, scroll: s.scrollWidth, client: s.clientWidth} })()`, &lane)
	if lane.W < 260 {
		t.Errorf("a column is %v pixels wide", lane.W)
	}
	var r struct{ X, Y, TX, TY float64 }
	p.Eval(t, `(() => {
		const c = document.getElementById('c-`+key+`'), board = document.querySelector('.tk-board')
		c.scrollIntoView({inline: 'nearest', block: 'nearest', behavior: 'instant'})
		board.scrollLeft += c.closest('.tk-lane').getBoundingClientRect().left - board.getBoundingClientRect().left - 8
		const l = document.querySelector('.tk-lane[data-lane="`+to+`"]')
		const box = c.getBoundingClientRect(), lb = l.getBoundingClientRect()
		return {x: box.left + 7, y: box.top + box.height / 2, tx: Math.min(lb.left + lb.width / 2, innerWidth - 40), ty: Math.min(lb.bottom - 8, innerHeight - 40)}
	})()`, &r)
	if r.TX <= r.X+40 {
		t.Fatalf("the column %s is not beside the card: card at %v, column at %v", to, r.X, r.TX)
	}
	// While a view transition of the lanes runs (another viewer's frame
	// gliding in), the browser hit-tests the board around them, which pans.
	p.Wait(t, `document.elementFromPoint(`+jsNum(r.X)+`, `+jsNum(r.Y)+`)?.closest('#c-`+key+`')`, 3*time.Second)
	touch(t, p, "touchStart", r.X, r.Y)
	p.Wait(t, `!document.querySelector('.tk-bd__ghost').hidden`, 2*time.Second) // a finger on the grip lifts the card at once
	for i := 1; i <= 10; i++ {
		f := float64(i) / 10
		touch(t, p, "touchMove", r.X+(r.TX-r.X)*f, r.Y+(r.TY-r.Y)*f)
		time.Sleep(16 * time.Millisecond)
	}
	touch(t, p, "touchEnd", 0, 0)
	p.Wait(t, laneOf(key)+` === '`+to+`' && !document.querySelector('#c-`+key+`').dataset.state`, 3*time.Second)
	other.Wait(t, `window.__seenAt > 0`, 3*time.Second)
	// A finger elsewhere on a card scrolls the board and moves nothing.
	key2 := pr.Keys[2]
	var lane2 string
	p.Eval(t, laneOf(key2), &lane2)
	var r2 struct{ X, Y float64 }
	p.Eval(t, `(() => { const b = document.getElementById('c-`+key2+`').querySelector('.tk-card__title').getBoundingClientRect(); return {x: b.left + b.width / 2, y: b.top + b.height / 2} })()`, &r2)
	touch(t, p, "touchStart", r2.X, r2.Y)
	for i := 1; i <= 5; i++ {
		touch(t, p, "touchMove", r2.X-float64(i)*30, r2.Y)
		time.Sleep(16 * time.Millisecond)
	}
	touch(t, p, "touchEnd", 0, 0)
	time.Sleep(300 * time.Millisecond)
	var after string
	var ghost bool
	p.Eval(t, laneOf(key2), &after)
	p.Eval(t, `!document.querySelector('.tk-bd__ghost').hidden`, &ghost)
	if after != lane2 || ghost {
		t.Errorf("a touch beside the grip moved %s from %s to %s (ghost %v)", key2, lane2, after, ghost)
	}
	for _, q := range []*browsertest.Page{p, other} {
		if errs := q.Errors(); len(errs) > 0 {
			t.Errorf("the page reported: %q", errs)
		}
	}
}

// NFR-56.1: the page editor on a touch screen takes typing, a list from
// the toolbar and a link from its dialog, by taps alone, and the server
// stores them.
func TestEditorTouch(t *testing.T) {
	noTouchInFirefox(t)
	s := newRealServer(t)
	page, edit := s.wikiPage(t)
	b := browsertest.Start(t)
	p := touchPage(t, b, 390, 844)
	p.Navigate(t, edit)
	p.Wait(t, `document.querySelector('.pm-mount .ProseMirror, .pm-mount[contenteditable]') && document.getElementById('editor').view`, 15*time.Second)
	p.Wait(t, `performance.getEntriesByType('resource').some(r => r.name.endsWith('/open'))`, 10*time.Second)
	p.Call(t, "Emulation.setFocusEmulationEnabled", map[string]any{"enabled": true})
	tap(t, p, `document.querySelector('.pm-mount [contenteditable="true"], .pm-mount[contenteditable="true"]')`)
	p.Wait(t, `document.getElementById('editor').view.hasFocus()`, 3*time.Second)
	typeText(t, p, "Groceries")
	enterKey(t, p)
	button := func(value string) string {
		return `document.getElementById('editor').shadowRoot.querySelector('.toolbar button[value="` + value + `"]')`
	}
	tap(t, p, button("bullet"))
	p.Wait(t, `document.querySelector('.pm-mount ul li')`, 3*time.Second)
	typeText(t, p, "milk ")
	tap(t, p, button("link"))
	p.Wait(t, `!document.getElementById('editor').shadowRoot.getElementById('link-dialog').hidden`, 3*time.Second)
	tap(t, p, `document.getElementById('editor').shadowRoot.getElementById('href')`)
	p.Call(t, "Input.insertText", map[string]any{"text": "example.com"})
	tap(t, p, `document.getElementById('editor').shadowRoot.querySelector('#link-dialog [value="apply"]')`)
	p.Wait(t, `document.querySelector('.pm-mount ul li a[href="https://example.com/"]')`, 3*time.Second)
	var j json.RawMessage
	p.Eval(t, `document.getElementById('editor').json()`, &j)
	for _, want := range []string{`"bullet_list"`, `"link"`, `"href":"https://example.com/"`, `Groceries`, `milk`} {
		if !strings.Contains(string(j), want) {
			t.Errorf("the document lacks %s: %s", want, j)
		}
	}
	waitFor(t, 10*time.Second, func() bool {
		h := headText(t, s, page)
		return strings.Contains(h, "Groceries") && strings.Contains(h, "milk https://example.com")
	})
	if errs := p.Errors(); len(errs) > 0 {
		t.Errorf("the page reported: %q", errs)
	}
}

// enterKey presses Enter as an on-screen keyboard does in a text field.
func enterKey(t *testing.T, p *browsertest.Page) {
	t.Helper()
	p.Call(t, "Input.dispatchKeyEvent", map[string]any{"type": "rawKeyDown", "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13})
	p.Call(t, "Input.dispatchKeyEvent", map[string]any{"type": "char", "key": "Enter", "text": "\r"})
	p.Call(t, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13})
}

func jsNum(f float64) string { b, _ := json.Marshal(f); return string(b) }
