package web_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
	"github.com/mbolli/tickit/internal/tracker"
	"github.com/mbolli/tickit/internal/web/webtest"
	"github.com/mbolli/tickit/internal/wiki"
)

// One store of the tracker's and the wiki's parts, bound as cmd/tickit
// binds them, so the wiki's projector of issue events runs (FR-100.2).
var wikiStore = sync.OnceValue(func() *events.Store {
	st := events.Must(events.New(append(tracker.Parts(), wiki.Parts()...)...))
	tracker.Bind(st)
	wiki.Bind(st)
	return st
})

func startWiki(t *testing.T) *webtest.Env {
	t.Helper()
	return webtest.StartWith(t, webtest.Options{Store: wikiStore()})
}

var gotoRe = regexp.MustCompile(`"_goto":"([^"]+)"`)

// act posts an action and returns its status and body.
func act(t *testing.T, e *webtest.Env, c *http.Client, path, tab string, fields map[string]any) (int, string) {
	t.Helper()
	if fields == nil {
		fields = map[string]any{}
	}
	fields["tab"] = tab
	if _, ok := fields["op"]; !ok {
		fields["op"] = fmt.Sprintf("op%d", opCount.Add(1))
	}
	b, _ := json.Marshal(fields)
	return webtest.Post(t, c, e.URL+path, string(b))
}

var opCount atomicInt

type atomicInt struct {
	mu sync.Mutex
	n  int
}

func (a *atomicInt) Add(d int) int { a.mu.Lock(); defer a.mu.Unlock(); a.n += d; return a.n }

// wikiSpace creates a space through its action and returns its key's
// address, its id and its home page.
func wikiSpace(t *testing.T, e *webtest.Env, c *http.Client, key string) (space, home string) {
	t.Helper()
	code, body := act(t, e, c, "/cmd/spaces/create", "tab1", map[string]any{"key": key, "name": key + " space", "description": "Notes"})
	if m := gotoRe.FindStringSubmatch(body); code != http.StatusOK || m == nil || m[1] != "/spaces/"+key {
		t.Fatalf("create space: %d %s", code, body)
	}
	e.DB.View(context.Background(), func(tx *sql.Tx) error {
		sp, err := wiki.SpaceByKey(context.Background(), tx, key)
		space, home = sp.ID, sp.Home
		return err
	})
	return space, home
}

func wikiPage(t *testing.T, e *webtest.Env, c *http.Client, space, parent, title string) string {
	t.Helper()
	code, body := act(t, e, c, "/cmd/pages/create", "tab1", map[string]any{"space": space, "parent": parent, "title": title, "template": "Meeting notes"})
	m := regexp.MustCompile(`"_goto":"/pages/([^/]+)/edit"`).FindStringSubmatch(body)
	if code != http.StatusOK || m == nil {
		t.Fatalf("create page %q: %d %s", title, code, body)
	}
	return m[1]
}

// FR-75, FR-76, FR-80, FR-81: spaces and pages through their actions and
// addresses.
func TestWikiPages(t *testing.T) {
	e := startWiki(t)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	space, home := wikiSpace(t, e, c, "ENG")
	if code, page := webtest.Get(t, c, e.URL+"/spaces"); code != http.StatusOK || !strings.Contains(page, "ENG space") {
		t.Errorf("spaces: %d", code)
	}
	if code, page := webtest.Get(t, c, e.URL+"/spaces/ENG"); code != http.StatusOK || !strings.Contains(page, `id="page-tree"`) || !strings.Contains(page, `<h1 class="wk-title">ENG space</h1>`) {
		t.Errorf("space home: %d", code)
	}
	if code, _ := webtest.Get(t, c, e.URL+"/spaces/eng"); code != http.StatusFound {
		t.Errorf("a key in lower case: %d", code)
	}
	id := wikiPage(t, e, c, space, home, "Weekly sync")
	url := "/spaces/ENG/pages/" + id + "/weekly-sync"
	for _, from := range []string{"/pages/" + id, "/spaces/ENG/pages/" + id, "/spaces/ENG/pages/" + id + "/old-title"} {
		res, err := c.Get(e.URL + from)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusFound || res.Header.Get("Location") != url {
			t.Errorf("%s: %d to %q", from, res.StatusCode, res.Header.Get("Location"))
		}
	}
	code, page := webtest.Get(t, c, e.URL+url)
	if code != http.StatusOK || !strings.Contains(page, "Weekly sync") || !strings.Contains(page, "Attendees") {
		t.Errorf("the page from a template: %d", code)
	}
	// FR-81.2: a taken title is refused next to the title.
	events := webtest.Listen(t, c, e.URL+"/stream?tab=tab1", nil)
	code, _ = act(t, e, c, "/cmd/pages/create", "tab1", map[string]any{"space": space, "title": "weekly SYNC", "op": "dup1"})
	if code != http.StatusNoContent {
		t.Errorf("a taken title: %d", code)
	}
	webtest.Next(t, events, `"dup1":{"ok":false,"msg":"A page titled \"weekly SYNC\" exists in this space.","field":"title"}`)
	if code, page := webtest.Get(t, c, e.URL+"/spaces/ENG/pages/"+id+"/edit"); code != http.StatusOK || !strings.Contains(page, "<sb-rich-editor") || !strings.Contains(page, `<script type="application/json" data-ignore-morph>{"type":"doc"`) {
		t.Errorf("the editor: %d", code)
	}
	for _, p := range []string{"/history", "/versions/1"} {
		if code, _ := webtest.Get(t, c, e.URL+"/spaces/ENG/pages/"+id+p); code != http.StatusOK {
			t.Errorf("%s: %d", p, code)
		}
	}
	if code, _ := webtest.Get(t, c, e.URL+"/spaces/ENG/pages/"+id+"/versions/9"); code != http.StatusNotFound {
		t.Errorf("a version that does not exist: %d", code)
	}
	if code, _ := webtest.Get(t, c, e.URL+"/img/lib/architecture-overview"); code != http.StatusOK {
		t.Errorf("FR-84: a library image: %d", code)
	}
}

// NFR-01: an action without the right answers 403 and changes nothing;
// one on nothing answers 404; a malformed one 400.
func TestWikiRefusals(t *testing.T) {
	e := startWiki(t)
	admin, guest := webtest.Client(t), webtest.Client(t)
	webtest.Get(t, admin, e.URL+"/")
	webtest.Get(t, guest, e.URL+"/")
	space, home := wikiSpace(t, e, admin, "ENG")
	page := wikiPage(t, e, admin, space, home, "Notes")
	// The guest acts once while the space is open, then becomes a viewer of
	// the closed space.
	act(t, e, guest, "/cmd/pages/watch", "tab2", map[string]any{"page": page, "watch": true})
	var guestID string
	e.DB.View(context.Background(), func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT person_id FROM page_watchers WHERE page_id = ? AND person_id != (SELECT created_by FROM pages WHERE id = ?)`, page, page).Scan(&guestID)
	})
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		if _, err := tx.Exec(`UPDATE scopes SET open = 0 WHERE id = ?`, space); err != nil {
			return err
		}
		return access.SetMember(tx, access.SpaceScope(space), guestID, access.Viewer, guestID, timeNow())
	}))
	state := func() string {
		var s string
		e.DB.View(context.Background(), func(tx *sql.Tx) error {
			return tx.QueryRow(`SELECT (SELECT count(*) FROM lith_events) || '/' || (SELECT group_concat(title || state || labels) FROM pages) || '/' || (SELECT count(*) FROM page_titles)`).Scan(&s)
		})
		return s
	}
	before := state()
	for _, a := range []struct {
		path   string
		fields map[string]any
	}{
		{"/cmd/pages/create", map[string]any{"space": space, "title": "Mine"}},
		{"/cmd/pages/rename", map[string]any{"page": page, "title": "Theirs"}},
		{"/cmd/pages/trash", map[string]any{"page": page}},
		{"/cmd/pages/restore-version", map[string]any{"page": page, "number": 1}},
		{"/cmd/pages/labels", map[string]any{"page": page, "add": []string{"x"}}},
	} {
		if code, _ := act(t, e, guest, a.path, "tab2", a.fields); code != http.StatusForbidden {
			t.Errorf("a viewer's %s: %d", a.path, code)
		}
	}
	if state() != before {
		t.Error("refused actions changed pages or the event log")
	}
	for path, body := range map[string]string{
		"/cmd/pages/rename":                  `{"tab":"t","op":"o","page":"nope","title":"x"}`,
		"/cmd/pages/" + page + "/commit":     `{"tab":"t","epoch":"e","base":1,"ref":"r","client":"0123456789ab","steps":"no"}`,
		"/cmd/pages/" + page + "/commit?x=1": `{"tab":"t","epoch":"e","base":1,"ref":"","client":"0123456789ab","steps":[]}`,
		"/cmd/pages/move":                    `{"tab":"t","op":"o","page":"` + page + `","target":"elsewhere"}`,
	} {
		code, _ := webtest.Post(t, admin, e.URL+path, body)
		if code != http.StatusBadRequest && code != http.StatusNotFound {
			t.Errorf("%s with %s: %d", path, body, code)
		}
	}
}

func timeNow() time.Time { return time.Now() }

func byOf(p people.Person, op string) tabs.By { return tabs.By{Actor: p, Op: op, At: time.Now()} }

// The commit's answers (docs/editor.md, "Wire format"): 204, 409, 422,
// 403 and 429.
func TestCommitAnswers(t *testing.T) {
	e := startWiki(t)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	space, home := wikiSpace(t, e, c, "ENG")
	page := wikiPage(t, e, c, space, home, "Notes")
	h := head(t, e, page)
	if code, _ := act(t, e, c, "/cmd/pages/"+page+"/open", "tab1", map[string]any{"base": h.Version, "client": "0123456789ab"}); code != http.StatusNoContent {
		t.Fatalf("open: %d", code)
	}
	commit := func(ref, epoch string, base int64, steps string) int {
		code, _ := webtest.Post(t, c, e.URL+"/cmd/pages/"+page+"/commit",
			fmt.Sprintf(`{"tab":"tab1","epoch":%q,"base":%d,"ref":%q,"client":"0123456789ab","steps":%s}`, epoch, base, ref, steps))
		return code
	}
	text := `[{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"text","text":"Hi"}]}}]`
	if code := commit("r1", h.Epoch, h.Version, text); code != http.StatusNoContent {
		t.Errorf("applied: %d", code)
	}
	if code := commit("r1", h.Epoch, h.Version, text); code != http.StatusNoContent {
		t.Errorf("applied before: %d", code)
	}
	if code := commit("r2", "e-other", h.Version, text); code != http.StatusConflict {
		t.Errorf("MP-13.2, another epoch: %d", code)
	}
	bad := `[{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"page_link","attrs":{"pageId":"not-an-id"}}]}}]`
	if code := commit("r3", h.Epoch, h.Version+1, bad); code != http.StatusUnprocessableEntity {
		t.Errorf("a rule: %d", code)
	}
	var refusal string
	e.DB.View(context.Background(), func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT refusal FROM page_editors WHERE tab_id = 'tab1'`).Scan(&refusal)
	})
	if !strings.Contains(refusal, `"ref":"r3"`) {
		t.Errorf("the refusal in the editor's row: %q", refusal)
	}
	// DM-04: 10 batches a second, 50 at once.
	limited := 0
	for i := range 80 {
		if commit(fmt.Sprintf("x%d", i), h.Epoch, h.Version+1, `[]`) == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 {
		t.Error("no commit over the rate limit answered 429")
	}
	// A reader's commit answers 403.
	other := webtest.Client(t)
	webtest.Get(t, other, e.URL+"/")
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		_, err := tx.Exec(`UPDATE scopes SET open = 0 WHERE id = ?`, space)
		return err
	}))
	code, _ := webtest.Post(t, other, e.URL+"/cmd/pages/"+page+"/commit",
		fmt.Sprintf(`{"tab":"tab9","epoch":%q,"base":2,"ref":"o1","client":"0123456789ab","steps":%s}`, h.Epoch, text))
	if code != http.StatusForbidden && code != http.StatusConflict {
		t.Errorf("an outsider's commit: %d", code)
	}
}

func head(t *testing.T, e *webtest.Env, page string) wiki.Head {
	t.Helper()
	var h wiki.Head
	if err := e.DB.View(context.Background(), func(tx *sql.Tx) (err error) { h, err = wiki.HeadOf(context.Background(), tx, page); return err }); err != nil {
		t.Fatal(err)
	}
	return h
}

// FR-80.3 (Q18), docs/editor.md: a reader sees another's edit in the
// next frame; an editing tab's view brings every commit after its ack;
// the tree shows a new page (FR-78.3); presence marks the editor
// (MP-07.3).
func TestWikiLive(t *testing.T) {
	e := startWiki(t)
	editor, reader := webtest.Client(t), webtest.Client(t)
	webtest.Get(t, editor, e.URL+"/")
	webtest.Get(t, reader, e.URL+"/")
	space, home := wikiSpace(t, e, editor, "ENG")
	page := wikiPage(t, e, editor, space, home, "Notes")
	read := webtest.Listen(t, reader, e.URL+"/stream?tab=rtab&v=pagedoc:"+page+",tree:"+page+",pagepeople:"+page, nil)
	webtest.Next(t, read, `id="page-body"`)
	edit := webtest.Listen(t, editor, e.URL+"/stream?tab=etab&v=carets:"+page+",pagepeople:"+page, nil)
	h := head(t, e, page)
	if code, _ := act(t, e, editor, "/cmd/pages/"+page+"/open", "etab", map[string]any{"base": h.Version, "client": "0123456789ab"}); code != http.StatusNoContent {
		t.Fatalf("open: %d", code)
	}
	webtest.Next(t, edit, `<sb-rich-editor id="editor"`)
	webtest.Next(t, read, `, editing"`)
	code, _ := webtest.Post(t, editor, e.URL+"/cmd/pages/"+page+"/commit",
		fmt.Sprintf(`{"tab":"etab","epoch":%q,"base":%d,"ref":"k1","client":"0123456789ab","steps":[{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"text","text":"Typed live"}]}}]}`, h.Epoch, h.Version))
	if code != http.StatusNoContent {
		t.Fatalf("commit: %d", code)
	}
	webtest.Next(t, read, `Typed live`)
	webtest.Next(t, edit, `&#34;ref&#34;:&#34;k1&#34;`)
	// Presence moves the caret in the shared carets view.
	webtest.Post(t, editor, e.URL+"/cmd/pages/"+page+"/presence", fmt.Sprintf(`{"tab":"etab","epoch":%q,"ack":%d,"v":%d,"anchor":3,"head":5,"seq":1}`, h.Epoch, h.Version+1, h.Version+1))
	webtest.Next(t, edit, `\"anchor\":3,\"head\":5`)
	wikiPage(t, e, editor, space, home, "Brand new")
	webtest.Next(t, read, `Brand new`)
}

// FR-100.2: a chip on a page follows its issue's status.
func TestChipsLive(t *testing.T) {
	e := startWiki(t)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	space, home := wikiSpace(t, e, c, "ENG")
	page := wikiPage(t, e, c, space, home, "Notes")
	ada := people.Seeded[0]
	e.Do(t, people.SeedPeople{At: time.Now()})
	var issue string
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		b, err := tracker.CreateProject{By: byOf(ada, "p1"), Key: "WEB", Name: "Web", Open: true}.ApplyResult(tx)
		if err != nil {
			return err
		}
		var m map[string]string
		json.Unmarshal(b, &m)
		b, err = tracker.CreateIssue{By: byOf(ada, "i1"), Project: m["id"], Type: tracker.Bug, Summary: "Fix the login"}.ApplyResult(tx)
		json.Unmarshal(b, &m)
		issue = m["id"]
		return err
	}))
	h := head(t, e, page)
	act(t, e, c, "/cmd/pages/"+page+"/open", "tab1", map[string]any{"base": h.Version, "client": "0123456789ab"})
	webtest.Post(t, c, e.URL+"/cmd/pages/"+page+"/commit", fmt.Sprintf(`{"tab":"tab1","epoch":%q,"base":%d,"ref":"c1","client":"0123456789ab","steps":[{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"issue_chip","attrs":{"key":"WEB-1"}}]}}]}`, h.Epoch, h.Version))
	read := webtest.Listen(t, c, e.URL+"/stream?tab=rtab&v=pagedoc:"+page, nil)
	webtest.Next(t, read, `data-cat="todo">To Do</span>`)
	e.Do(t, tracker.SetStatus{By: byOf(ada, "s1"), Issue: issue, Seen: 1 << 40, Status: tracker.StatusInReview})
	webtest.Next(t, read, `data-cat="progress">In Review</span>`)
}
