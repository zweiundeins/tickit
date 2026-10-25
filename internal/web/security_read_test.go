package web_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// NFR-06, requirements 12.3: no GET writes, whoever asks: every probe of
// every GET route, the pages of the fixture's closed and open scopes,
// and a stream of every kind of view the server registers, each as
// every kind of person. NFR-01.2: a view of a scope the person may not
// read answers 404 or 403, never its frames.
func TestEveryGetReadOnly(t *testing.T) {
	f := newFixture(t)
	everyone := []who{wGuest, wViewer, wMember, wOwner, wMod, wRoot, wBlocked, wMust}
	clients := map[string]*http.Client{"no cookie": {Transport: fromAddr{http.DefaultTransport, "198.51.100.1"}}}
	for _, w := range everyone {
		clients[string(w)] = f.client(w)
	}
	page, opage := f.id("page", false), f.id("page", true)
	urls := append([]string{}, f.e.Server.Probes()...)
	for _, scope := range []struct{ key, space, page, issue string }{{"SEC", "SECS", page, f.id("issue", false)}, {"OPN", "OPS", opage, f.id("issue", true)}} {
		sp := "/spaces/" + scope.space
		pg := sp + "/pages/" + scope.page
		urls = append(urls, "/"+scope.key+"-1", "/ticket/"+scope.issue, "/projects/"+scope.key, "/projects/"+scope.key+"/board", "/projects/"+scope.key+"/backlog",
			"/projects/"+scope.key+"/activity", "/projects/"+scope.key+"/settings", sp, pg, pg+"/the-plan", pg+"/history", pg+"/versions/1",
			pg+"/compare?from=1&to=1", pg+"/edit", sp+"/trash", "/pages/"+scope.page, "/pages/"+scope.page+"/edit",
			"/issues?project="+scope.key, "/issues.csv?project="+scope.key, "/search?q=plan&space="+scope.space, "/search/suggest?q=the",
			"/wiki/pick?kind=issue&q="+scope.key, "/wiki/pick?kind=page&q=plan", "/wiki/pick?kind=person&q=the",
			"/issues/"+scope.issue+"/description/editor", "/issues/"+scope.issue+"/description?v=1", "/"+scope.key+"-1?edit=description")
	}
	urls = append(urls, "/dashboards/"+f.id("dashboard", false), "/people/"+f.p[wOwner].ID)
	urls = append(urls, "/filters", "/admin", "/admin/guests", "/admin/stats", "/admin/security", "/admin/demo", "/account", "/moderation", "/labels/urgent")

	// The views pages name, as the people who may read them.
	specs := map[string][]string{} // kind to the specs of it
	add := func(spec string) {
		kind, _, _ := strings.Cut(spec, ":")
		if spec != "" && !slices.Contains(specs[kind], spec) {
			specs[kind] = append(specs[kind], spec)
		}
	}
	for _, w := range []who{wOwner, wRoot} {
		for _, u := range urls {
			if strings.Contains(u, "/stream") || strings.Contains(u, ".csv") {
				continue
			}
			if code, body := f.get(f.client(w), u, 2*time.Second); code == http.StatusOK {
				for _, v := range viewsOf(body) {
					add(v)
				}
			}
		}
	}
	// The views no page names: a tab's editor, versions, and the ones
	// actions add to a stream.
	for _, p := range []string{page, opage} {
		for _, k := range []string{"carets", "editchips", "editcomments", "pagetitle", "pagepeople", "history", "tree", "pagedoc", "page", "pagelists"} {
			add(k + ":" + p)
		}
		add("editor:" + p + ".probe")
	}
	for _, i := range []string{f.id("issue", false), f.id("issue", true)} {
		add("issue:" + i)
		add("issue-at:" + i + ".1")
		add("issue-typing:" + i)
		add("issue-desc:" + i)
		add("desc-carets:" + i)
		add("desc-chips:" + i)
		add("desc-editor:" + i + ".probe")
	}
	add("trash:" + f.id("space", false))
	add("activity-space:" + f.id("space", false))
	add("activity-project:" + f.id("proj", false))
	for _, kind := range f.e.Server.Views().Kinds() {
		if len(specs[kind]) == 0 {
			t.Errorf("NFR-06: no probe of the view kind %s", kind)
		}
	}
	for _, list := range specs {
		for _, spec := range list {
			urls = append(urls, "/stream?tab=probe&v="+spec)
		}
	}
	urls = append(urls, "/stream?tab=probe&v=tab:probe", "/stream?tab=probe&v=person:"+f.p[wOwner].ID, "/stream?tab=probe&v=prefs:probe")

	if err := f.e.W.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := f.e.Fingerprint(t)
	closed := []string{":SEC", ":SEC.", "%3ASEC"}
	for _, k := range []string{"proj", "space", "home", "page", "mpage", "tpage", "issue", "issue2", "epic"} {
		closed = append(closed, f.id(k, false))
	}
	var wg sync.WaitGroup
	for name, c := range clients {
		wg.Go(func() {
			for _, u := range urls {
				code, body := f.read(c, u)
				if code >= 500 {
					t.Errorf("%s GET %s: %d", name, u, code)
				}
				// A closed scope's view shows nothing to a guest.
				outsider := name == "guest" || name == "no cookie" || name == "blocked"
				if outsider && strings.HasPrefix(u, "/stream") && code == http.StatusOK && slices.ContainsFunc(closed, func(id string) bool { return strings.Contains(u, id) }) {
					t.Errorf("NFR-01.2: %s got the stream %s: %.200s", name, u, body)
				}
			}
		})
	}
	wg.Wait()
	if err := f.e.W.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.e.Fingerprint(t) != before {
		t.Errorf("NFR-06: a GET wrote to the database")
	}
	t.Logf("%d URLs, %d of them streams, as %d kinds of visitor", len(urls), strings.Count(strings.Join(urls, " "), "/stream?"), len(clients))

	// NFR-13, NFR-19, DM-09: every page has the CSP with a nonce of its
	// own, every script carries it, no element an inline handler; every
	// answer has the security headers.
	scripts := regexp.MustCompile(`<script\b[^>]*>`)
	handlers := regexp.MustCompile(`<[a-z][^>]*\son[a-z]+\s*=`)
	nonces := map[string]bool{}
	for _, u := range urls {
		if strings.HasPrefix(u, "/stream") {
			continue
		}
		res, err := f.client(wRoot).Get(f.e.URL + u)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		h := res.Header
		csp := h.Get("Content-Security-Policy")
		if h.Get("X-Robots-Tag") != "noindex, nofollow" || !strings.Contains(csp, "frame-ancestors 'none'") && h.Get("X-Frame-Options") != "DENY" || csp == "" || h.Get("X-Content-Type-Options") != "nosniff" ||
			h.Get("Referrer-Policy") == "" || h.Get("Cross-Origin-Opener-Policy") != "same-origin" {
			t.Errorf("NFR-19: %s: %v", u, h)
		}
		if !strings.HasPrefix(h.Get("Content-Type"), "text/html") {
			continue
		}
		m := regexp.MustCompile(`'nonce-([^']+)'`).FindStringSubmatch(csp)
		if m == nil {
			t.Errorf("NFR-13: %s has no nonce in its CSP: %s", u, csp)
			continue
		}
		if nonces[m[1]] {
			t.Errorf("NFR-13: %s has a nonce another page had", u)
		}
		nonces[m[1]] = true
		for _, tag := range scripts.FindAllString(string(b), -1) {
			switch {
			case strings.Contains(tag, ` src="`):
				if !strings.Contains(tag, ` src="/static/`) {
					t.Errorf("NFR-29: %s: a script from elsewhere: %s", u, tag)
				}
			case strings.Contains(tag, "application/json"): // data, which no CSP runs
			case !strings.Contains(tag, `nonce="`+m[1]+`"`):
				t.Errorf("NFR-13: %s: an inline script without the page's nonce: %s", u, tag)
			}
		}
		if bad := handlers.FindString(string(b)); bad != "" {
			t.Errorf("NFR-13: %s: an inline handler: %.120s", u, bad)
		}
	}
}

// NFR-02.3: tab state is keyed by session and tab. Another session that
// forges a tab's id in /stream, /cmd/watch, /cmd/unwatch, an editor's
// open or presence, reads nothing of that tab and changes nothing of it,
// and no stream names another's tab, person or preferences as a view.
func TestForgedTabs(t *testing.T) {
	f := newFixture(t)
	a, b := f.client(wMember), f.client(wGuest)
	const tab = "victim-tab"
	opage, issue := f.id("page", true), f.id("issue", false)
	events := webtest.Listen(t, a, f.e.URL+"/stream?tab="+tab+"&v=issue:"+issue, nil)
	webtest.Next(t, events, `"_outcomes"`)
	if code, _ := f.post(wMember, "/cmd/prefs", map[string]any{"tab": tab, "op": "victim-op", "key": "theme", "value": "dark"}); code != http.StatusNoContent {
		t.Fatalf("the victim's action: %d", code)
	}
	webtest.Next(t, events, "victim-op")
	if code, _ := f.post(wMember, "/cmd/pages/"+opage+"/open", map[string]any{"tab": tab, "op": "victim-open", "base": 0, "client": "aaaaaaaaaaaa"}); code != http.StatusNoContent {
		t.Fatalf("the victim opens the editor: %d", code)
	}
	webtest.Next(t, events, `client="aaaaaaaaaaaa"`)
	victimRows := func() string {
		rows, _ := f.e.DB.R.Query(`SELECT * FROM page_editors WHERE tab_id = ? AND sid = ?`, tab, f.sid(wMember))
		defer rows.Close()
		cols, _ := rows.Columns()
		var out strings.Builder
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			rows.Scan(ptrs...)
			for _, v := range vals {
				out.WriteString(toString(v) + "|")
			}
		}
		return out.String()
	}
	rowsBefore := victimRows()
	tabBefore, _ := f.outcomeOf(wMember, tab, "victim-op")

	// The forger's stream with the tab's id: its own outcomes.
	forged := webtest.Listen(t, b, f.e.URL+"/stream?tab="+tab+"&v=", nil)
	if first := webtest.Next(t, forged, `"_outcomes"`); strings.Contains(first, "victim-op") {
		t.Errorf("a forged tab id read the victim's outcomes: %s", first)
	}
	// Views of the forger's own, never into the victim's stream.
	if code, _ := f.post(wGuest, "/cmd/watch", map[string]any{"tab": tab, "view": "spaces:"}); code != http.StatusNoContent {
		t.Errorf("the forger's watch: %d", code)
	}
	webtest.Next(t, forged, `id="spaces`)
	webtest.Silent(t, events, `id="spaces`, 300*time.Millisecond)
	f.post(wGuest, "/cmd/unwatch", map[string]any{"tab": tab, "view": "issue:" + issue})
	if !slices.Contains(f.e.Hub.Watchers("issue:"+issue), f.p[wMember].ID+"."+tab) {
		t.Error("a forged unwatch took the victim's view")
	}
	f.post(wGuest, "/cmd/pages/"+opage+"/open", map[string]any{"tab": tab, "op": "forger-open", "base": 0, "client": "bbbbbbbbbbbb"})
	webtest.Silent(t, events, "bbbbbbbbbbbb", 300*time.Millisecond)
	f.post(wGuest, "/cmd/pages/"+opage+"/presence", map[string]any{"tab": tab, "epoch": epochOf(f, opage), "ack": 0, "v": 0, "anchor": 2, "head": 2, "seq": 99})
	f.post(wGuest, "/cmd/prefs", map[string]any{"tab": tab, "op": "forger-op", "key": "theme", "value": "light"})
	webtest.Silent(t, events, "forger-op", 300*time.Millisecond)
	f.e.W.Flush(context.Background())
	if victimRows() != rowsBefore {
		t.Error("a forged tab id changed the victim's editor row")
	}
	if after, _ := f.outcomeOf(wMember, tab, "victim-op"); fmt.Sprint(after) != fmt.Sprint(tabBefore) {
		t.Error("a forged tab id changed the victim's tab row")
	}
	if _, ok := f.outcomeOf(wMember, tab, "forger-op"); ok {
		t.Error("the forger's outcome is in the victim's tab")
	}
	// No view names a tab, a person or a session's preferences.
	for _, v := range []string{"tab:" + tab, "person:" + f.p[wMember].ID, "prefs:" + tab, "editor:" + opage + "." + tab} {
		code, body := f.get(b, "/stream?tab=forger&v="+v, 300*time.Millisecond)
		if code == http.StatusOK && (strings.Contains(body, "victim-op") || strings.Contains(body, "aaaaaaaaaaaa")) {
			t.Errorf("the view %s showed the victim's: %.200s", v, body)
		}
		if !strings.HasPrefix(v, "editor:") && code != http.StatusBadRequest {
			t.Errorf("the view %s: %d", v, code)
		}
	}
}

func toString(v any) string {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return fmt.Sprint(v)
}
