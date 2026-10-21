package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/demo"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// feedSeeded is a project with n issues, each an entry of the home feed.
func feedSeeded(t *testing.T, n int) (*webtest.Env, func(summary string)) {
	t.Helper()
	e := webtest.Start(t)
	now := time.Now()
	ops := 0
	do := func(cmd writer.ResultCommand) string {
		b, err := e.W.DoValue(context.Background(), func(tx *writer.Tx) ([]byte, error) { return cmd.ApplyResult(tx) })
		if err != nil {
			t.Fatalf("%T: %v", cmd, err)
		}
		var m map[string]string
		json.Unmarshal(b, &m)
		return m["id"]
	}
	by := func(at time.Time) tabs.By {
		ops++
		return tabs.By{Actor: people.Seeded[ops%len(people.Seeded)], Op: fmt.Sprint("feed", ops), At: at}
	}
	e.Do(t, people.SeedPeople{At: now})
	project := do(tracker.CreateProject{By: by(now.Add(-48 * time.Hour)), Key: "FEED", Name: "Feed", Lead: people.Seeded[0].ID, Open: true})
	for i := range n {
		do(tracker.CreateIssue{By: by(now.Add(time.Duration(i-n) * time.Minute)), Project: project, Type: tracker.Task, Summary: fmt.Sprintf("Entry number %d", i+1)})
	}
	return e, func(summary string) {
		do(tracker.CreateIssue{By: by(time.Now()), Project: project, Type: tracker.Task, Summary: summary})
	}
}

// The home page's feed scrolls far down in sb-virtual-scroll, by the mouse
// wheel and by the keyboard, with only a window of entries in the page;
// entries that arrive meanwhile at the top keep the reader's place, and
// the focused entry keeps the focus (FR-69, MP-03.3, NFR-37, NFR-47).
func TestHomeFeedScrolls(t *testing.T) {
	e, create := feedSeeded(t, 400)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1024, 900)
	p.Navigate(t, e.URL+"/")
	p.Wait(t, `document.getElementById('reconnecting').hidden && document.getElementById('activity-home-list')?.tkFeed === true`, 10*time.Second)

	// The top visible entry: its place in the feed and where it is drawn.
	const top = `(() => {
		const h = document.getElementById('activity-home-list'), sc = h.shadowRoot.querySelector('.scroller'), r = sc.getBoundingClientRect()
		const el = [...h.children].find((a) => a.getBoundingClientRect().top >= r.top - 1)
		return el ? {id: el.id, pos: +el.getAttribute('aria-posinset'), y: Math.round(el.getBoundingClientRect().top - r.top), n: h.children.length, offset: +h.getAttribute('offset')} : null
	})()`
	type place struct {
		ID     string
		Pos, Y int
		N      int
		Offset int
	}
	at := func() place {
		var pl place
		p.Eval(t, top, &pl)
		return pl
	}

	// By mouse: wheel over the list until entry 200 or so is at the top.
	var box struct{ X, Y float64 }
	p.Eval(t, `(() => { document.getElementById('activity-home-list').scrollIntoView({block: 'center'}); const r = document.getElementById('activity-home-list').getBoundingClientRect(); return {x: r.left + r.width / 2, y: r.top + r.height / 2} })()`, &box)
	for i := 0; at().Pos < 200; i++ {
		if i > 200 {
			t.Fatalf("the wheel got no further than %+v", at())
		}
		p.Call(t, "Input.dispatchMouseEvent", map[string]any{"type": "mouseWheel", "x": box.X, "y": box.Y, "deltaX": 0, "deltaY": 625})
		time.Sleep(60 * time.Millisecond)
	}
	p.Wait(t, `(() => { const h = document.getElementById('activity-home-list'); return !h.hasAttribute('aria-busy') && +h.getAttribute('offset') > 100 })()`, 5*time.Second)
	before := at()
	if before.N > 60 || before.Offset < 100 {
		t.Fatalf("the window holds %d entries from %d: want a window of at most 50 far down", before.N, before.Offset)
	}

	// Three entries arrive at the top: the entry the reader looks at stays
	// where it is drawn, three places further down the feed.
	for i := range 3 {
		create(fmt.Sprintf("Arrived while scrolled %d", i))
	}
	p.Wait(t, `+document.getElementById(`+jsQuote(before.ID)+`)?.getAttribute('aria-posinset') === `+fmt.Sprint(before.Pos+3), 10*time.Second)
	time.Sleep(200 * time.Millisecond)
	after := at()
	t.Logf("scrolled to %+v, after new entries %+v", before, after)
	if after.ID != before.ID || after.Y != before.Y {
		t.Errorf("new entries moved the reader: before %+v, after %+v", before, after)
	}
	p.Wait(t, `!document.querySelector('#activity-home .tk-vfeed__newer').hidden`, 2*time.Second)
	shoot(t, p, "feed-home-scrolled")

	// By keyboard: Page Down from the entry at the top moves the focus entry
	// by entry, past the windows the page has, and the frame of a new entry
	// leaves the focus on the entry it was on.
	p.Eval(t, `(document.getElementById(`+jsQuote(after.ID)+`).focus(), true)`, nil)
	start := after.Pos
	for range 120 {
		p.Press(t, "PageDown")
		time.Sleep(15 * time.Millisecond)
	}
	want := start + 120
	p.Wait(t, `+document.activeElement?.getAttribute('aria-posinset') === `+fmt.Sprint(want), 10*time.Second)
	var focused place
	p.Eval(t, `(() => { const a = document.activeElement, sc = document.getElementById('activity-home-list').shadowRoot.querySelector('.scroller'); return {id: a.id, pos: +a.getAttribute('aria-posinset'), y: Math.round(a.getBoundingClientRect().top - sc.getBoundingClientRect().top)} })()`, &focused)
	shoot(t, p, "feed-home-keyboard")
	if focused.Y < 0 {
		t.Errorf("the focused entry is out of view: %+v", focused)
	}
	create("Arrived while focused")
	p.Wait(t, `+document.activeElement?.getAttribute('aria-posinset') === `+fmt.Sprint(want+1), 10*time.Second)
	var still place
	p.Eval(t, `(() => { const a = document.activeElement, sc = document.getElementById('activity-home-list').shadowRoot.querySelector('.scroller'); return {id: a.id, pos: +a.getAttribute('aria-posinset'), y: Math.round(a.getBoundingClientRect().top - sc.getBoundingClientRect().top)} })()`, &still)
	time.Sleep(300 * time.Millisecond) // the component's own refocus, a frame later
	var id string
	p.Eval(t, `document.activeElement.id`, &id)
	t.Logf("focus on %+v, after a new entry %+v", focused, still)
	if id != focused.ID || still.Y != focused.Y {
		t.Errorf("the focus or its place moved: before %+v, after %+v (focus on %s)", focused, still, id)
	}
	p.Press(t, "PageUp")
	p.Wait(t, `+document.activeElement?.getAttribute('aria-posinset') === `+fmt.Sprint(want), 5*time.Second)

	// Newer activity leads back to the top, where the newest entry is.
	p.Eval(t, `(document.querySelector('#activity-home .tk-vfeed__newer').click(), true)`, nil)
	p.Wait(t, `document.activeElement?.getAttribute('aria-posinset') === '1' && document.activeElement.textContent.includes('Arrived while focused')`, 10*time.Second)

	if errs := p.Errors(); len(errs) > 0 {
		t.Errorf("the page reported: %q", errs)
	}
}

// shoot writes a screenshot of the page into the directory TICKIT_SHOTS
// names, when it names one.
func shoot(t *testing.T, p *browsertest.Page, name string) {
	t.Helper()
	if dir := os.Getenv("TICKIT_SHOTS"); dir != "" {
		p.Screenshot(t, filepath.Join(dir, name+".png"), false)
	}
}

// TestFeedShots writes screenshots of the seeded demo's feeds and the
// issue's tabs into the directory TICKIT_SHOTS names; without it, it skips.
func TestFeedShots(t *testing.T) {
	if os.Getenv("TICKIT_SHOTS") == "" {
		t.Skip("TICKIT_SHOTS names no directory")
	}
	e := webtest.Start(t)
	if _, err := demo.Seed(context.Background(), e.W, e.Server.Demo().Content(), time.Now(), 1); err != nil {
		t.Fatal(err)
	}
	b := browsertest.Start(t)
	for _, s := range []struct {
		name, path, before string
		w, h               int
	}{
		{"feed-home", "/", `document.getElementById('activity-home-list').scrollIntoView({block: 'center'})`, 1280, 900},
		{"feed-home-320", "/", `document.getElementById('activity-home-list').scrollIntoView({block: 'start'})`, 320, 800},
		{"feed-project", "/projects/WEB/activity", "", 1280, 900},
		{"feed-space", "/spaces/ENG", `document.getElementById('space-activity-title').scrollIntoView({block: 'start'})`, 1280, 900},
		{"issue-tabs", "/WEB-2", `(document.querySelector('sb-tabs').shadowRoot.querySelectorAll('[role=tab]')[1].click(), document.querySelector('sb-tabs').scrollIntoView({block: 'start'}))`, 1280, 900},
		{"issue-tabs-320", "/WEB-2", `document.querySelector('sb-tabs').scrollIntoView({block: 'start'})`, 320, 800},
	} {
		p := b.NewPage(t, s.w, s.h)
		p.Navigate(t, e.URL+s.path)
		p.Wait(t, `document.getElementById('reconnecting')?.hidden`, 10*time.Second)
		time.Sleep(500 * time.Millisecond)
		if s.before != "" {
			p.Eval(t, "("+s.before+", true)", nil)
			time.Sleep(400 * time.Millisecond)
		}
		shoot(t, p, s.name)
		if errs := p.Errors(); len(errs) > 0 {
			t.Errorf("%s: %q", s.name, errs)
		}
		p.Close(t)
	}
}
