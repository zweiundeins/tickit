package web_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zweiundeins/lith/edge"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/sse"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/views"
	"github.com/mbolli/tickit/internal/web/webtest"
)

// personName is the name the page's header shows.
var personName = regexp.MustCompile(`<span id="person-name" class="person-name">([^<]*)</span>`)

// The document and its stream: the person's name, the demo notice, the
// tab's outcomes; FR-120.1: one guest in every tab of a browser.
func TestPageAndStream(t *testing.T) {
	e := webtest.Start(t)
	c := webtest.Client(t)
	code, page := webtest.Get(t, c, e.URL+"/")
	if code != http.StatusOK || !strings.Contains(page, `<html lang="en">`) || !strings.Contains(page, `<div id="stream" hidden data-init=`) ||
		!strings.Contains(page, "Public demo. Everything you write is visible to everyone and deleted at the nightly reset (next in ") {
		t.Fatalf("the home page: %d\n%s", code, page)
	}
	m := personName.FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no name in the header:\n%s", page)
	}
	if _, again := webtest.Get(t, c, e.URL+"/privacy"); !strings.Contains(again, ">"+m[1]+"</span>") {
		t.Errorf("another page names another guest")
	}
	events := webtest.Listen(t, c, e.URL+"/stream?tab=tab1", nil)
	var frames []string
	for _, want := range []string{`<span id="person-name" class="person-name">` + m[1] + `</span>`, `id="demo-notice"`, `"_stale":false`, `"_outcomes":{}`} {
		frames = append(frames, webtest.Next(t, events, want))
	}
	// FR-120.2: the session id shows nowhere.
	if sid := cookie(c, e.URL); sid == "" || strings.Contains(page, sid) || strings.Contains(strings.Join(frames, ""), sid) {
		t.Errorf("the session id %q shows", sid)
	}
	// FR-120.3: another browser is another guest.
	_, other := webtest.Get(t, webtest.Client(t), e.URL+"/")
	if me, them := guestID.FindStringSubmatch(page), guestID.FindStringSubmatch(other); me == nil || them == nil || me[1] == them[1] {
		t.Errorf("two browsers: %v and %v", me, them)
	}
}

// guestID is the person id the document marks as the viewer's.
var guestID = regexp.MustCompile(`<style>\[data-who="(g-[a-z2-7]{20})"\]`)

// NFR-46: the page lets people zoom.
func TestZoom(t *testing.T) {
	e := webtest.Start(t)
	_, page := webtest.Get(t, webtest.Client(t), e.URL+"/")
	if !strings.Contains(page, `<meta name="viewport" content="width=device-width, initial-scale=1">`) || strings.Contains(page, "maximum-scale") || strings.Contains(page, "user-scalable") {
		t.Errorf("the viewport: %s", page[:400])
	}
}

// MP-09: an action answers 204 and its outcome arrives on the tab's
// stream; a refusal brings its message and field; a retry of an op runs
// nothing; two quick actions both settle.
func TestOutcomesOnTheStream(t *testing.T) {
	e := webtest.Start(t)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	events := webtest.Listen(t, c, e.URL+"/stream?tab=tab1", nil)
	webtest.Next(t, events, `"_outcomes":{}`)

	rename := func(op, name string) {
		t.Helper()
		if code, body := webtest.Post(t, c, e.URL+"/cmd/people/rename", fmt.Sprintf(`{"tab":"tab1","op":%q,"name":%q}`, op, name)); code != http.StatusNoContent {
			t.Fatalf("rename %s: %d %s", op, code, body)
		}
	}
	rename("op1", "Ada Lovelace")
	webtest.Next(t, events, `<span id="person-name" class="person-name">Ada Lovelace</span>`)
	webtest.Next(t, events, `"op1":{"ok":true,"msg":"","field":""}`)

	rename("op2", "A")
	webtest.Next(t, events, `"op2":{"ok":false,"msg":"A name has 2 to 30 characters.","field":"name"}`)
	rename("op2", "A") // a retry: the same answer, nothing new on the stream
	webtest.Silent(t, events, `"_outcomes"`, 200*time.Millisecond)

	var wg sync.WaitGroup
	for _, op := range []string{"op3", "op4"} {
		wg.Go(func() { rename(op, "Ada "+op) })
	}
	wg.Wait()
	for {
		ev := webtest.Next(t, events, `"_outcomes"`)
		if strings.Contains(ev, `"op3":{"ok":true`) && strings.Contains(ev, `"op4":{"ok":true`) {
			break
		}
	}
}

// MP-10, NFR-42: a deploy ends every stream with sse.Reconnect, which
// re-creates the stream's element, whose data-init opens it again.
func TestReconnect(t *testing.T) {
	e := webtest.Start(t)
	c := webtest.Client(t)
	events := webtest.Listen(t, c, e.URL+"/stream?tab=tab1", nil)
	webtest.Next(t, events, `"_outcomes"`)
	e.StopStreams()
	webtest.Next(t, events, `document.getElementById(&#34;stream&#34;)`)
	if _, ok := <-events; ok {
		t.Error("the stream goes on after the reconnect")
	}
}

// The page measures its link and asks for Chromium's network hints; a
// stream with Save-Data: on brings the same views.
func TestSlowLink(t *testing.T) {
	e := webtest.Start(t)
	res, err := http.Get(e.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if got := res.Header.Get("Accept-CH"); got != edge.NetworkHints {
		t.Errorf("Accept-CH: %q", got)
	}
	if !strings.Contains(string(page), "_slow: ((c, n) =&gt;") || !strings.Contains(string(page), "{headers: $_slow ? {&#39;Save-Data&#39;: &#39;on&#39;} : {}") {
		t.Errorf("the page does not measure its link:\n%s", page)
	}
	events := webtest.Listen(t, webtest.Client(t), e.URL+"/stream?tab=tab1", http.Header{"Save-Data": {"on"}})
	webtest.Next(t, events, `"_outcomes"`)
}

// The page loads newest-frame.js after Datastar, with an import map that
// maps 'datastar' to Datastar's URL, and turns it on on its body.
func TestNewestFrame(t *testing.T) {
	e := webtest.Start(t)
	_, page := webtest.Get(t, webtest.Client(t), e.URL+"/")
	m := regexp.MustCompile(`<script type="importmap" nonce="[^"]+">(.*?)</script><script type="module" src="(/static/vendor/datastar-rocket\.[0-9a-f]{12}\.js)"></script><script type="module" src="(/static/lith/newest-frame\.[0-9a-f]{12}\.js)"></script>`).FindStringSubmatch(page)
	if m == nil || !strings.Contains(page, "<body data-newest-frame ") {
		t.Fatalf("the page does not load newest-frame.js after Datastar, or does not turn it on:\n%s", page)
	}
	var importMap struct{ Imports map[string]string }
	if err := json.Unmarshal([]byte(m[1]), &importMap); err != nil || importMap.Imports["datastar"] != m[2] {
		t.Errorf("import map %s (%v), Datastar at %s", m[1], err, m[2])
	}
	res, err := http.Get(e.URL + m[3])
	if err != nil {
		t.Fatal(err)
	}
	js, _ := io.ReadAll(res.Body)
	res.Body.Close()
	written, _ := fs.ReadFile(live.Scripts, live.NewestFrame)
	if res.StatusCode != http.StatusOK || !strings.Contains(res.Header.Get("Cache-Control"), "immutable") || len(js) >= len(written) {
		t.Errorf("%s: %d, %s, %d bytes of %d", m[3], res.StatusCode, res.Header.Get("Cache-Control"), len(js), len(written))
	}
}

// things registers view kinds for the tests: thing (with presence),
// secret (forbidden), missing (not found).
func things(e *webtest.Env) {
	vr := e.Server.Views()
	vr.RegisterWithPresence("thing", func(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
		return live.View{Key: "thing:" + arg, Topics: []string{"thing"}, Render: func(context.Context, *sql.Tx) (any, error) {
			return sse.PatchElements([]byte(`<p id="thing-` + arg + `">thing ` + arg + `</p>`)), nil
		}}, nil
	})
	vr.Register("secret", func(context.Context, sqlite.Querier, people.Person, string) (live.View, error) {
		return live.View{}, &access.Denial{Refusal: writer.Refusal{Msg: "no"}}
	})
	vr.Register("missing", func(context.Context, sqlite.Querier, people.Person, string) (live.View, error) {
		return live.View{}, access.ErrNotFound
	})
}

// A stream refuses views it cannot make, before it opens (NFR-16, NFR-01.2).
func TestStreamViews(t *testing.T) {
	e := webtest.Start(t)
	things(e)
	c := webtest.Client(t)
	for v, want := range map[string]int{
		"thing:1":      http.StatusOK,
		"nope:1":       http.StatusBadRequest,
		"thing:a b":    http.StatusBadRequest,
		"Thing:1":      http.StatusBadRequest,
		"secret:1":     http.StatusForbidden,
		"missing:1":    http.StatusNotFound,
		"thing:1,nope": http.StatusBadRequest,
		strings.Repeat("thing:1,", 9) + "thing:2": http.StatusBadRequest,
	} {
		req, _ := http.NewRequestWithContext(t.Context(), "GET", e.URL+"/stream?tab=tab1&v="+v, nil)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		res, err := c.Do(req.WithContext(ctx))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		cancel()
		if res.StatusCode != want {
			t.Errorf("v=%s: %d, want %d", v, res.StatusCode, want)
		}
	}
	for _, tab := range []string{"", "bad tab", strings.Repeat("x", 65)} {
		if code, _ := webtest.Get(t, c, e.URL+"/stream?tab="+tab); code != http.StatusBadRequest {
			t.Errorf("tab %q: %d", tab, code)
		}
	}
}

// MP-01, MP-02: one face per person, however many tabs, in the order they
// came, with the tab count in the tooltip; a face goes when its last tab
// closes.
func TestPresence(t *testing.T) {
	e := webtest.Start(t)
	things(e)
	ada, bob := webtest.Client(t), webtest.Client(t)
	_, page := webtest.Get(t, ada, e.URL+"/")
	adaName := personName.FindStringSubmatch(page)[1]
	_, page = webtest.Get(t, bob, e.URL+"/")
	bobName := personName.FindStringSubmatch(page)[1]
	row := `<ul id="` + views.PresenceID("thing:1") + `" class="presence"`

	first := webtest.Listen(t, ada, e.URL+"/stream?tab=ada1&v=thing:1", nil)
	webtest.Next(t, first, `<p id="thing-1">thing 1</p>`)
	webtest.Next(t, first, row)
	bobs, closeBob := webtest.Open(t, bob, e.URL+"/stream?tab=bob1&v=thing:1", nil)
	webtest.Next(t, bobs, row)
	webtest.Listen(t, ada, e.URL+"/stream?tab=ada2&v=thing:1", nil)
	both := regexp.MustCompile(`data-who="(g-[a-z2-7]{20})" title="` + regexp.QuoteMeta(adaName) + `, 2 tabs"[^>]*>.*data-who="(g-[a-z2-7]{20})" title="` + regexp.QuoteMeta(bobName) + `"`)
	for {
		ev := webtest.Next(t, first, row)
		if m := both.FindStringSubmatch(ev); m != nil {
			if strings.Count(ev, "<li data-who") != 2 || strings.Contains(ev, "ada1") || strings.Contains(ev, "bob1") {
				t.Errorf("the row: %s", ev)
			}
			break
		}
	}
	closeBob()
	start := time.Now()
	for {
		ev := webtest.Next(t, first, row)
		if !strings.Contains(ev, bobName) {
			break
		}
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("bob's face went after %v", d) // MP-02.3
	}
}

// A side panel's view joins the open stream (POST /cmd/watch), comes back
// with a stream that reconnects, and leaves with POST /cmd/unwatch.
func TestWatch(t *testing.T) {
	e := webtest.Start(t)
	things(e)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	events := webtest.Listen(t, c, e.URL+"/stream?tab=tab1", nil)
	webtest.Next(t, events, `"_outcomes"`)
	watch := func(path, view string) int {
		t.Helper()
		code, _ := webtest.Post(t, c, e.URL+path, fmt.Sprintf(`{"tab":"tab1","view":%q}`, view))
		return code
	}
	if code := watch("/cmd/watch", "thing:2"); code != http.StatusNoContent {
		t.Fatalf("watch: %d", code)
	}
	webtest.Next(t, events, `<p id="thing-2">`)
	webtest.Next(t, events, views.PresenceID("thing:2"))

	again := webtest.Listen(t, c, e.URL+"/stream?tab=tab1", nil) // the page reconnects
	webtest.Next(t, again, `<p id="thing-2">`)
	if code := watch("/cmd/unwatch", "thing:2"); code != http.StatusNoContent {
		t.Fatalf("unwatch: %d", code)
	}
	third := webtest.Listen(t, c, e.URL+"/stream?tab=tab1", nil)
	webtest.Next(t, third, `"_outcomes"`)
	webtest.Silent(t, third, `thing-2`, 200*time.Millisecond)

	for view, want := range map[string]int{"secret:1": http.StatusForbidden, "missing:1": http.StatusNotFound, "nope:1": http.StatusBadRequest, "thing:1,thing:2": http.StatusBadRequest, "": http.StatusBadRequest} {
		if code := watch("/cmd/watch", view); code != want {
			t.Errorf("watch %q: %d, want %d", view, code, want)
		}
	}
}
