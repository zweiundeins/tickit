package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/notify"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// reacting is a server whose event store holds the tracker and the
// reactions, with an events runner, as cmd/tickit runs them.
func reacting(t *testing.T) *webtest.Env {
	t.Helper()
	store := events.Must(events.New(append(tracker.Parts(), notify.Parts()...)...))
	tracker.Bind(store)
	e := webtest.StartWith(t, webtest.Options{Store: store})
	notify.Bind(e.W)
	t.Cleanup(func() { tracker.Bind(nil); notify.Bind(nil) })
	runner := events.NewRunner(store, e.DB, e.W, events.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); runner.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		live := len(runner.Stats().Projectors) > 0
		for _, p := range runner.Stats().Projectors {
			live = live && p.Status == "live"
		}
		if live {
			return e
		}
		if time.Now().After(deadline) {
			t.Fatal("the runner did not start")
		}
	}
}

// sessionOf is the guest of a page's session, and its cookie.
func sessionOf(t *testing.T, p *browsertest.Page, url string) (people.Person, map[string]any) {
	t.Helper()
	var jar struct{ Cookies []map[string]any }
	if err := json.Unmarshal(p.Call(t, "Network.getCookies", map[string]any{"urls": []string{url}}), &jar); err != nil || len(jar.Cookies) != 1 {
		t.Fatalf("cookies: %v %v", jar, err)
	}
	return people.GuestOf(fmt.Sprint(jar.Cookies[0]["value"])), jar.Cookies[0]
}

// tracked runs tracker commands as people, giving guests their rows.
type tracked struct {
	t   *testing.T
	e   *webtest.Env
	ops int
	at  time.Time
}

func (k *tracked) by(p people.Person) tabs.By {
	k.ops++
	if k.at.IsZero() {
		k.at = time.Now().Add(-3 * time.Hour)
	}
	k.at = k.at.Add(7 * time.Minute)
	return tabs.By{Actor: p, Op: fmt.Sprint("e2e", k.ops), At: k.at}
}

func (k *tracked) do(cmd writer.Command, as ...people.Person) string {
	k.t.Helper()
	b, err := k.e.W.DoValue(context.Background(), func(tx *writer.Tx) ([]byte, error) {
		for _, p := range as {
			if err := people.Ensure(tx, p, time.Now()); err != nil {
				return nil, err
			}
		}
		if rc, ok := cmd.(writer.ResultCommand); ok {
			return rc.ApplyResult(tx)
		}
		return nil, cmd.Apply(tx)
	})
	if err != nil {
		k.t.Fatalf("%T: %v", cmd, err)
	}
	var m map[string]string
	json.Unmarshal(b, &m)
	return m["id"]
}

// FR-110.2, FR-110.3, FR-115.1, MP-01.2, NFR-48.2: an assignment reaches
// the bell of both tabs of its person in the next frames, as the count,
// the live region, the window title and the panel; marking it read in one
// tab clears it in the other. The page reports no error.
func TestBellLive(t *testing.T) {
	e := reacting(t)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1280, 800)
	p.Navigate(t, e.URL+"/")
	me, cookie := sessionOf(t, p, e.URL)
	other := b.NewPage(t, 1280, 800)
	other.Call(t, "Network.setCookie", map[string]any{"name": cookie["name"], "value": cookie["value"], "url": e.URL, "httpOnly": true, "sameSite": cookie["sameSite"]})
	other.Navigate(t, e.URL+"/privacy")
	for deadline := time.Now().Add(5 * time.Second); e.Hub.Stats().Streams != 2; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("no streams after 5s: %+v", e.Hub.Stats())
		}
	}
	for _, page := range []*browsertest.Page{p, other} {
		page.Wait(t, `document.getElementById('tk-bell-count').hidden && document.getElementById('reconnecting').hidden`, 5*time.Second)
	}

	sunny := people.GuestOf("e2e-sunny")
	k := &tracked{t: t, e: e}
	project := k.do(tracker.CreateProject{By: k.by(sunny), Key: "WEB", Name: "Website", Open: true}, sunny)
	k.do(tracker.CreateIssue{By: k.by(me), Project: project, Type: tracker.Task, Summary: "Mine"}, me)
	issue := k.do(tracker.CreateIssue{By: k.by(sunny), Project: project, Type: tracker.Bug, Summary: "Broken login"}, sunny)
	k.do(tracker.Assign{By: k.by(sunny), Issue: issue, Seen: 1 << 40, Assignee: me.ID})

	for _, page := range []*browsertest.Page{p, other} {
		page.Wait(t, `(() => {
			const c = document.getElementById('tk-bell-count')
			return !c.hidden && c.textContent === '1' && document.getElementById('tk-unread').textContent === '1 unread notification' &&
				document.title.endsWith(' (1)') && document.querySelector('#tk-notify-panel .tk-feed__item[data-unread] .tk-notif__subject')?.textContent === 'WEB-2'
		})()`, 5*time.Second)
	}
	if got := evalString(t, other, `document.title`); got != "Privacy · tickit (1)" {
		t.Errorf("title %q", got)
	}
	// The home page's work shows the issue, live.
	p.Wait(t, `document.querySelector('#home-work')?.textContent.includes('Broken login')`, 5*time.Second)

	// Mark it read in one tab, by keyboard-free clicks in the open panel.
	p.Eval(t, `(document.querySelector('.tk-bell').click(), true)`, nil)
	p.Wait(t, `document.getElementById('tk-notifications').matches(':popover-open') && document.querySelector('.tk-bell').getAttribute('aria-expanded') === 'true'`, 5*time.Second)
	p.Eval(t, `(document.querySelector('#tk-notify-panel .tk-notif__actions button').click(), true)`, nil)
	for _, page := range []*browsertest.Page{p, other} {
		page.Wait(t, `document.getElementById('tk-bell-count').hidden && !document.title.includes('(') && !document.querySelector('#tk-notify-panel [data-unread]')`, 5*time.Second)
	}
	// The panel stayed open through the frames.
	if !evalBool(t, p, `document.getElementById('tk-notifications').matches(':popover-open')`) {
		t.Error("the frame closed the panel")
	}
	// Dismiss all empties both panels.
	p.Eval(t, `([...document.querySelectorAll('#tk-notify-panel .tk-notif__head button')].find(b => b.textContent.includes('Dismiss all')).click(), true)`, nil)
	other.Wait(t, `document.querySelector('#tk-notify-panel .tk-empty') !== null`, 5*time.Second)
	for _, page := range []*browsertest.Page{p, other} {
		if errs := page.Errors(); len(errs) > 0 {
			t.Errorf("the page reported: %q", errs)
		}
	}
}

func evalString(t *testing.T, p *browsertest.Page, expr string) string {
	t.Helper()
	var s string
	p.Eval(t, expr, &s)
	return s
}

func evalInt(t *testing.T, p *browsertest.Page, expr string) int {
	t.Helper()
	var n int
	p.Eval(t, expr, &n)
	return n
}

func evalBool(t *testing.T, p *browsertest.Page, expr string) bool {
	t.Helper()
	var ok bool
	p.Eval(t, expr, &ok)
	return ok
}

// TestNotifyShots draws the home page and the bell's panel in both themes
// at 1440 and 375 pixels into $TICKIT_SHOTS (notify-*.png), for a look
// against /design. It runs only when the directory is given.
func TestNotifyShots(t *testing.T) {
	dir := os.Getenv("TICKIT_SHOTS")
	if dir == "" {
		t.Skip("TICKIT_SHOTS names no directory")
	}
	e := reacting(t)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1440, 900)
	p.Navigate(t, e.URL+"/")
	me, _ := sessionOf(t, p, e.URL)
	sunny, lynx := people.GuestOf("shots-sunny"), people.GuestOf("shots-lynx")
	k := &tracked{t: t, e: e}
	web := k.do(tracker.CreateProject{By: k.by(sunny), Key: "WEB", Name: "Website relaunch", Open: true}, sunny)
	ops := k.do(tracker.CreateProject{By: k.by(lynx), Key: "OPS", Name: "Operations", Open: true}, lynx)
	k.do(tracker.CreateIssue{By: k.by(me), Project: web, Type: tracker.Story, Summary: "Keyboard moves for board cards"}, me)
	k.do(tracker.CreateIssue{By: k.by(me), Project: ops, Type: tracker.Task, Summary: "Check the alert routes"}, me)
	var issues []string
	for i, s := range []struct {
		who     people.Person
		project string
		typ     tracker.IssueType
		summary string
	}{
		{sunny, web, tracker.Bug, "The login form forgets the email after a refused password"},
		{lynx, ops, tracker.Task, "Rotate the backup keys"},
		{sunny, web, tracker.Story, "Search shows issues and pages together"},
		{lynx, web, tracker.Bug, "Umlauts in page titles break the breadcrumb"},
	} {
		id := k.do(tracker.CreateIssue{By: k.by(s.who), Project: s.project, Type: s.typ, Summary: s.summary, Due: time.Now().AddDate(0, 0, i*3-2).Format(time.DateOnly)}, s.who, me)
		issues = append(issues, id)
		k.do(tracker.Assign{By: k.by(s.who), Issue: id, Seen: 1 << 40, Assignee: me.ID})
	}
	k.do(tracker.SetStatus{By: k.by(sunny), Issue: issues[0], Seen: 1 << 40, Status: tracker.StatusInProgress})
	k.do(tracker.AddComment{By: k.by(lynx), Issue: issues[0], Body: tracker.PlainDoc("I can reproduce it in Firefox, not in Chromium. The autofill seems to clear the field on the second attempt.")})
	k.do(tracker.Move{By: k.by(lynx), Issue: issues[3], Status: tracker.StatusInReview, Seen: 1 << 40})
	time.Sleep(300 * time.Millisecond) // the reactions
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"dark", "light"} {
		for _, width := range []int{1440, 375} {
			page := b.NewPage(t, width, 900)
			_, cookie := sessionOf(t, p, e.URL)
			page.Call(t, "Network.setCookie", map[string]any{"name": cookie["name"], "value": cookie["value"], "url": e.URL, "httpOnly": true, "sameSite": cookie["sameSite"]})
			page.Navigate(t, e.URL+"/")
			page.Eval(t, `(document.documentElement.dataset.tkTheme = '`+theme+`', document.querySelector('meta[name=color-scheme]').content = '`+theme+`', true)`, nil)
			page.Wait(t, `document.getElementById('reconnecting').hidden && document.fonts.status === 'loaded'`, 5*time.Second)
			page.Eval(t, `(document.querySelector('#first-visit:not([hidden]) button')?.click(), true)`, nil)
			page.Wait(t, `!document.querySelector('#first-visit:not([hidden])')`, 5*time.Second)
			time.Sleep(400 * time.Millisecond) // the theme's transitions
			if w := evalInt(t, page, `document.documentElement.scrollWidth`); w > width {
				t.Errorf("%s at %d: the page is %d pixels wide", theme, width, w) // NFR-46
			}
			name := fmt.Sprintf("notify-home-%s-%d.png", theme, width)
			page.Screenshot(t, filepath.Join(dir, name), true)
			page.Eval(t, `(window.scrollTo(0, 0), document.querySelector('.tk-bell').click(), true)`, nil)
			page.Wait(t, `document.getElementById('tk-notifications').matches(':popover-open')`, 5*time.Second)
			time.Sleep(150 * time.Millisecond)
			page.Screenshot(t, filepath.Join(dir, strings.Replace(name, "home", "panel", 1)), false)
			if errs := page.Errors(); len(errs) > 0 {
				t.Errorf("%s: the page reported %q", name, errs)
			}
			page.Close(t)
		}
	}
}
