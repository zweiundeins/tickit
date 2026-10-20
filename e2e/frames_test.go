package e2e

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// actor is a second browser that changes things through its own page's
// actions, as a person there would.
type actor struct {
	t *testing.T
	p *browsertest.Page
	n int
}

func newActor(t *testing.T, b *browsertest.Browser, e *webtest.Env) *actor {
	p := b.NewPage(t, 1000, 700)
	p.Navigate(t, e.URL+"/privacy")
	p.Wait(t, `document.querySelector('#reconnecting')?.hidden !== false`, 5*time.Second)
	return &actor{t: t, p: p}
}

// post sends an action from the actor's tab and fails on an answer other
// than 204 or 200.
func (a *actor) post(path string, body map[string]any) {
	a.t.Helper()
	a.n++
	body["op"] = fmt.Sprintf("actor%d", a.n)
	b, _ := json.Marshal(body)
	var status int
	a.p.Eval(a.t, `(async () => {
		const tab = JSON.parse(document.body.dataset.signals.match(/_tab: ("[^"]+")/)[1])
		const r = await fetch(`+jsQuote(path)+`, {method: 'POST', headers: {'Content-Type': 'application/json', 'Datastar-Request': 'true'}, body: JSON.stringify({tab, ...`+string(b)+`})})
		return r.status
	})()`, &status)
	if status != 204 && status != 200 {
		a.t.Fatalf("%s: %d", path, status)
	}
}

// where is the focused element, its field's value and caret, the scroll
// position and the viewport position of an element that marks the place.
type where struct {
	Focus  string
	Value  string
	Caret  int
	Scroll float64
	Top    float64
}

// whereJS reads where; mark is a selector of the element that marks the
// place, focus a function of the focused element that names it.
func whereJS(mark, focus string) string {
	return `(() => {
		const a = document.activeElement
		return {focus: (` + focus + `)(a) || '', value: a?.value ?? '', caret: a?.selectionStart ?? -1, scroll: window.scrollY,
			top: document.querySelector(` + jsQuote(mark) + `).getBoundingClientRect().top}
	})()`
}

func sameWhere(t *testing.T, page string, before, after where) {
	t.Helper()
	if after.Focus != before.Focus || after.Value != before.Value || after.Caret != before.Caret {
		t.Errorf("MP-03.3, %s: the focus or the field moved: before %+v, after %+v", page, before, after)
	}
	if d := after.Top - before.Top; d > 1 || d < -1 || before.Scroll == 0 {
		t.Errorf("MP-03.3, %s: the place moved on screen by %.1f px (scroll %.0f to %.0f)", page, d, before.Scroll, after.Scroll)
	}
}

func issueID(t *testing.T, e *webtest.Env, key string) string {
	t.Helper()
	var id string
	if err := e.DB.R.QueryRow(`SELECT id FROM issues WHERE key = ?`, key).Scan(&id); err != nil {
		t.Fatal(key, err)
	}
	return id
}

// MP-03.3, NFR-48.1: while another browser changes things, a frame never
// moves focus, never resets a field someone types in, and keeps the
// scroll position: on the issue list, the board and an issue's page.
func TestFramesKeepPlace(t *testing.T) {
	e := seeded(t)
	b := browsertest.Start(t)
	bob := newActor(t, b, e)
	never := float64(1 << 40)

	t.Run("list", func(t *testing.T) {
		p := b.NewPage(t, 1200, 600)
		p.Navigate(t, e.URL+"/issues?sort=key&dir=asc")
		p.Wait(t, `document.querySelectorAll('#issue-list tbody tr').length === 25`, 5*time.Second)
		var keys []string
		p.Eval(t, rowKeys, &keys)
		focusKey, aboveKey := keys[14], keys[12]
		mark := `#issue-list tbody tr:nth-child(15)`
		p.Eval(t, `(() => { const r = document.querySelector(`+jsQuote(mark)+`); r.scrollIntoView({block: 'center'}); r.querySelector('a').focus(); return true })()`, nil)
		focus := `(a) => a?.closest('tr')?.querySelector('td.tk-key')?.textContent + ' ' + a?.tagName`
		var before where
		p.Eval(t, whereJS(mark, focus), &before)
		bob.post("/cmd/issues/"+issueID(t, e, focusKey)+"/priority", map[string]any{"seen": never, "value": "low"})
		bob.post("/cmd/issues/"+issueID(t, e, aboveKey)+"/summary", map[string]any{"seen": never, "value": "Renamed while you looked"})
		bob.post("/cmd/issues/"+issueID(t, e, keys[3])+"/status", map[string]any{"seen": never, "value": "done"})
		p.Wait(t, `document.getElementById('issue-list').textContent.includes('Renamed while you looked')`, 5*time.Second)
		time.Sleep(200 * time.Millisecond) // the other frames
		var after where
		p.Eval(t, whereJS(mark, focus), &after)
		sameWhere(t, "the issue list", before, after)
	})

	t.Run("board", func(t *testing.T) {
		p := b.NewPage(t, 1200, 600)
		p.Navigate(t, e.URL+"/projects/WEB/board")
		p.Wait(t, `document.querySelectorAll('#board-lanes .tk-card').length > 10`, 5*time.Second)
		// The last card of the fullest column, and a card of another one.
		var cards struct{ Focus, Other, Lane string }
		p.Eval(t, `(() => {
			const lanes = [...document.querySelectorAll('#board-lanes ol, #board-lanes ul')].filter(l => l.querySelector('.tk-card'))
			lanes.sort((a, b) => b.querySelectorAll('.tk-card').length - a.querySelectorAll('.tk-card').length)
			const cs = lanes[0].querySelectorAll('.tk-card'), other = lanes[1].querySelector('.tk-card')
			return {focus: cs[cs.length - 1].id, other: other.id, lane: ''}
		})()`, &cards)
		mark := "#" + cards.Focus
		p.Eval(t, `(() => { const c = document.querySelector(`+jsQuote(mark)+`); c.scrollIntoView({block: 'center'}); c.querySelector('a.tk-card__title').focus(); return true })()`, nil)
		focus := `(a) => a?.closest('.tk-card')?.id + ' ' + a?.tagName`
		var before where
		p.Eval(t, whereJS(mark, focus), &before)
		key := func(id string) string { return id[len("c-"):] }
		bob.post("/cmd/issues/"+issueID(t, e, key(cards.Focus))+"/summary", map[string]any{"seen": never, "value": "A card renamed while you looked"})
		bob.post("/cmd/issues/"+issueID(t, e, key(cards.Other))+"/priority", map[string]any{"seen": never, "value": "critical"})
		p.Wait(t, `document.querySelector(`+jsQuote(mark)+`).textContent.includes('A card renamed while you looked')`, 5*time.Second)
		time.Sleep(300 * time.Millisecond) // view transitions, the other frames
		var after where
		p.Eval(t, whereJS(mark, focus), &after)
		sameWhere(t, "the board", before, after)
	})

	t.Run("issue", func(t *testing.T) {
		p := b.NewPage(t, 1200, 600)
		p.Call(t, "Emulation.setFocusEmulationEnabled", map[string]any{"enabled": true}) // a window the person types in has the focus
		p.Navigate(t, e.URL+"/WEB-5")
		p.Wait(t, `!!document.querySelector('#issue-comments') && document.querySelector('#reconnecting')?.hidden !== false`, 5*time.Second)
		mark := "#issue-comment"
		p.Eval(t, `(() => { const c = document.getElementById('issue-comment'); c.scrollIntoView({block: 'center'}); c.focus(); return true })()`, nil)
		for _, r := range "My half-written thought" {
			p.Call(t, "Input.insertText", map[string]any{"text": string(r)})
		}
		p.Eval(t, `(() => { const c = document.getElementById('issue-comment'); c.setSelectionRange(3, 3); return true })()`, nil)
		focus := `(a) => a?.id`
		var before where
		p.Eval(t, whereJS(mark, focus), &before)
		id := issueID(t, e, "WEB-5")
		bob.post("/cmd/issues/"+id+"/priority", map[string]any{"seen": never, "value": "high"})
		bob.post("/cmd/issues/"+id+"/summary", map[string]any{"seen": never, "value": "Changed while you wrote"})
		bob.post("/cmd/issues/"+id+"/comment", map[string]any{"value": "A comment from someone else."})
		p.Wait(t, `document.getElementById('issue-summary-text').textContent === 'Changed while you wrote' && document.getElementById('issue-comments').textContent.includes('A comment from someone else.')`, 5*time.Second)
		time.Sleep(200 * time.Millisecond)
		var after where
		p.Eval(t, whereJS(mark, focus), &after)
		if before.Value != "My half-written thought" || before.Caret != 3 {
			t.Fatalf("the draft: %+v", before)
		}
		sameWhere(t, "an issue's page", before, after)
		if errs := p.Errors(); len(errs) > 0 {
			t.Errorf("the page reported: %q", errs)
		}
	})
}
