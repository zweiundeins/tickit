package web_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/demo"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/web"
	"github.com/mbolli/tickit/internal/web/webtest"
)

// Regression tests of the findings of the security review (docs/security.md).

// NFR-13, DM-06.1: text that closes the editor page's script of the
// document stays inside it as JSON.
func TestEditorScriptHoldsNoMarkup(t *testing.T) {
	e := startWiki(t)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	space, home := wikiSpace(t, e, c, "ENG")
	page := wikiPage(t, e, c, space, home, "Notes")
	h := head(t, e, page)
	if code, _ := act(t, e, c, "/cmd/pages/"+page+"/open", "tab1", map[string]any{"base": h.Version, "client": "0123456789ab"}); code != http.StatusNoContent {
		t.Fatalf("open: %d", code)
	}
	evil := `</script><div data-init='alert(1)'>x</div><!--`
	steps, _ := json.Marshal([]any{map[string]any{"stepType": "replace", "from": 1, "to": 1,
		"slice": map[string]any{"content": []any{map[string]any{"type": "text", "text": evil}}}}})
	code, body := webtest.Post(t, c, e.URL+"/cmd/pages/"+page+"/commit",
		fmt.Sprintf(`{"tab":"tab1","epoch":%q,"base":%d,"ref":"r1","client":"0123456789ab","steps":%s}`, h.Epoch, h.Version, steps))
	if code != http.StatusNoContent {
		t.Fatalf("commit: %d %s", code, body)
	}
	_, edit := webtest.Get(t, webtest.Client(t), e.URL+"/pages/"+page+"/edit")
	if loc := regexp.MustCompile(`/spaces/[^"]+/edit`).FindString(edit); loc != "" {
		_, edit = webtest.Get(t, webtest.Client(t), e.URL+loc)
	}
	m := regexp.MustCompile(`(?s)<script type="application/json"[^>]*>(.*?)</script>`).FindStringSubmatch(edit)
	if m == nil {
		t.Fatalf("no script of the document in the editor's page:\n%.2000s", edit)
	}
	if strings.Contains(edit, `<div data-init='alert(1)'>`) {
		t.Error("the person's markup reached the editor's page")
	}
	var d any
	if err := json.Unmarshal([]byte(m[1]), &d); err != nil || !strings.Contains(fmt.Sprint(d), evil) {
		t.Errorf("the script's document: %v %.300s", err, m[1])
	}
}

// NFR-16: a commit on a version beyond the head is malformed, not a 500.
func TestCommitBeyondHead(t *testing.T) {
	e := startWiki(t)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	space, home := wikiSpace(t, e, c, "ENG")
	page := wikiPage(t, e, c, space, home, "Notes")
	h := head(t, e, page)
	act(t, e, c, "/cmd/pages/"+page+"/open", "tab1", map[string]any{"base": h.Version, "client": "0123456789ab"})
	code, body := webtest.Post(t, c, e.URL+"/cmd/pages/"+page+"/commit",
		fmt.Sprintf(`{"tab":"tab1","epoch":%q,"base":%d,"ref":"r1","client":"0123456789ab","steps":[]}`, h.Epoch, h.Version+1000))
	if code >= 500 || code < 400 {
		t.Errorf("a commit beyond the head: %d %s", code, body)
	}
}

// FR-141.1: an account with a first password to change reads no view and
// changes nothing until it does: not by the stream, a watch, the list's
// view, a commit or a caret.
func TestMustChangeEverywhere(t *testing.T) {
	e := webtest.Start(t)
	e.Account(t, "ada@example.com", "correct horse battery", people.Admin)
	admin := webtest.Client(t)
	e.Login(t, admin, "ada@example.com", "correct horse battery")
	if code, body := webtest.Post(t, admin, e.URL+"/cmd/admin/create", `{"tab":"tab1","op":"c1","email":"bea@example.com","username":"bea","name":"Bea","role":"moderator","password":"first password"}`); code != http.StatusNoContent {
		t.Fatalf("create: %d %s", code, body)
	}
	bea := webtest.Client(t)
	e.Login(t, bea, "bea@example.com", "first password")
	if code := status(t, bea, e.URL+"/stream?tab=tab1&v=moderation:"); code != http.StatusForbidden {
		t.Errorf("a stream of the moderation page: %d", code)
	}
	page := "0192d000-0000-7000-8000-000000000001"
	for path, body := range map[string]string{
		"/cmd/watch":                       `{"tab":"tab1","view":"moderation:"}`,
		"/cmd/unwatch":                     `{"tab":"tab1","view":"moderation:"}`,
		"/cmd/list/view":                   `{"tab":"tab1","view":"","query":"assignee=me"}`,
		"/cmd/pages/" + page + "/commit":   `{"tab":"tab1","epoch":"e1","base":1,"ref":"r1","client":"0123456789ab","steps":[]}`,
		"/cmd/pages/" + page + "/presence": `{"tab":"tab1","epoch":"e1","ack":1,"v":1,"anchor":1,"head":1,"seq":1}`,
	} {
		if code, _ := webtest.Post(t, bea, e.URL+path, body); code != http.StatusForbidden {
			t.Errorf("%s with a first password: %d", path, code)
		}
	}
	// The account's page keeps its stream, with the views every page has.
	events := webtest.Listen(t, bea, e.URL+"/stream?tab=tab1&v=", nil)
	webtest.Next(t, events, `"_outcomes"`)
}

// status is the status of a GET, which a stream answers at once.
func status(t *testing.T, c *http.Client, url string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

// DM-07.2, DM-03.4: a blocked guest's typing and carets, which skip
// tabs.Act, are paused too.
func TestBlockedGuestTypes(t *testing.T) {
	e := webtest.Start(t)
	seedProbes(t, e)
	e.Account(t, "mo@example.com", "correct horse battery", people.Moderator)
	mo, g := webtest.Client(t), webtest.Client(t)
	e.Login(t, mo, "mo@example.com", "correct horse battery")
	me := guest(t, e, g)
	webtest.Post(t, g, e.URL+"/cmd/people/rename", `{"tab":"tab1","op":"g1","name":"Spammer"}`)
	if code, body := webtest.Post(t, mo, e.URL+"/cmd/moderation/block", `{"tab":"tab1","op":"b1","person":"`+me.ID+`","reason":"spam"}`); code != http.StatusNoContent {
		t.Fatalf("block: %d %s", code, body)
	}
	before := e.Fingerprint(t)
	if code, _ := webtest.Post(t, g, e.URL+"/cmd/issues/"+web.ProbeIssue+"/typing", `{"tab":"tab1","op":"t1","field":"summary"}`); code != http.StatusForbidden {
		t.Errorf("a blocked guest types: %d", code)
	}
	page := "0192d000-0000-7000-8000-000000000001"
	if code, _ := webtest.Post(t, g, e.URL+"/cmd/pages/"+page+"/presence", `{"tab":"tab1","epoch":"e1","ack":1,"v":1,"anchor":1,"head":1,"seq":1}`); code != http.StatusForbidden {
		t.Errorf("a blocked guest's caret: %d", code)
	}
	e.W.Flush(context.Background())
	if e.Fingerprint(t) != before {
		t.Error("a blocked guest's typing changed the database")
	}
}

// NFR-02.3: a tab's own views belong to its session: another session that
// names the same tab id gets its own outcomes and preferences, never the
// first one's, whichever stream opened first.
func TestTabViewsBySession(t *testing.T) {
	e := webtest.Start(t)
	a, b := webtest.Client(t), webtest.Client(t)
	webtest.Get(t, a, e.URL+"/")
	webtest.Get(t, b, e.URL+"/")
	const tab = "shared-tab-id"
	webtest.Post(t, a, e.URL+"/cmd/prefs", `{"tab":"`+tab+`","op":"a-pref","key":"theme","value":"dark"}`)
	as := webtest.Listen(t, a, e.URL+"/stream?tab="+tab+"&v=", nil)
	webtest.Next(t, as, `"a-pref"`)
	bs := webtest.Listen(t, b, e.URL+"/stream?tab="+tab+"&v=", nil)
	first := webtest.Next(t, bs, `"_outcomes"`)
	if strings.Contains(first, "a-pref") {
		t.Errorf("another session's stream with the tab's id got its outcomes: %s", first)
	}
	webtest.Silent(t, bs, `"dark"`, 300*time.Millisecond)
	webtest.Post(t, b, e.URL+"/cmd/prefs", `{"tab":"`+tab+`","op":"b-pref","key":"theme","value":"light"}`)
	webtest.Next(t, bs, `"b-pref"`)
	webtest.Silent(t, as, `"b-pref"`, 300*time.Millisecond)
}

// NFR-02.3: a stream that names another person's editing tab gets an
// editor of its own, never that tab's.
func TestEditorViewByPerson(t *testing.T) {
	e := startWiki(t)
	a, b := webtest.Client(t), webtest.Client(t)
	webtest.Get(t, a, e.URL+"/")
	webtest.Get(t, b, e.URL+"/")
	space, home := wikiSpace(t, e, a, "ENG")
	page := wikiPage(t, e, a, space, home, "Notes")
	h := head(t, e, page)
	as := webtest.Listen(t, a, e.URL+"/stream?tab=tab1&v=", nil)
	webtest.Next(t, as, `"_outcomes"`)
	if code, _ := act(t, e, a, "/cmd/pages/"+page+"/open", "tab1", map[string]any{"base": h.Version, "client": "aaaaaaaaaaaa"}); code != http.StatusNoContent {
		t.Fatalf("open: %d", code)
	}
	webtest.Next(t, as, `client="aaaaaaaaaaaa"`)
	bs := webtest.Listen(t, b, e.URL+"/stream?tab=tab9&v=editor:"+page+".tab1", nil)
	frame := webtest.Next(t, bs, `id="editor"`)
	if strings.Contains(frame, "aaaaaaaaaaaa") || !strings.Contains(frame, `state="reset"`) {
		t.Errorf("another person's stream got the tab's editor: %s", frame)
	}
}

// NFR-04.2, FR-124: a cookie copied before a logout is a guest's
// afterwards, on the streams it had open too.
func TestLogoutEndsStreams(t *testing.T) {
	e := webtest.Start(t)
	e.Account(t, "ada@example.com", "correct horse battery", people.Admin)
	c := webtest.Client(t)
	e.Login(t, c, "ada@example.com", "correct horse battery")
	copied := withCookie(t, e.URL, cookie(c, e.URL))
	events := webtest.Listen(t, copied, e.URL+"/stream?tab=tab1&v=admin-demo:", nil)
	webtest.Next(t, events, `id="admin-demo"`)
	if code, _ := webtest.Post(t, c, e.URL+"/cmd/logout", ``); code != http.StatusOK {
		t.Fatalf("logout: %d", code)
	}
	deadline := time.After(web.LoginCheck + 3*time.Second)
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("the copied cookie's stream of an admin view outlived the logout")
		}
	}
}

// DM-07.1: what a moderator hid stays hidden: the Admin a guest becomes
// of their own space or project cannot restore the page or show the
// comment again, and other visitors cannot open the hidden page.
func TestModeratorHideHolds(t *testing.T) {
	e := startWiki(t)
	e.Account(t, "mo@example.com", "correct horse battery", people.Moderator)
	mo, g, other := webtest.Client(t), webtest.Client(t), webtest.Client(t)
	e.Login(t, mo, "mo@example.com", "correct horse battery")
	webtest.Get(t, g, e.URL+"/")
	webtest.Get(t, other, e.URL+"/")
	space, home := wikiSpace(t, e, g, "SPM")
	page := wikiPage(t, e, g, space, home, "Cheap watches")
	if code, body := act(t, e, mo, "/cmd/moderation/act", "tab1", map[string]any{"action": "hide", "kind": "page", "id": page, "reason": "spam"}); code != http.StatusNoContent {
		t.Fatalf("hide the page: %d %s", code, body)
	}
	if code := status(t, other, e.URL+"/spaces/SPM/pages/"+page); code != http.StatusNotFound {
		t.Errorf("another visitor opens the hidden page: %d", code)
	}
	if code := status(t, mo, e.URL+"/spaces/SPM/pages/"+page); code == http.StatusNotFound {
		t.Errorf("the moderator opens the hidden page: %d", code)
	}
	if code, _ := act(t, e, g, "/cmd/pages/restore", "tab1", map[string]any{"page": page}); code != http.StatusForbidden {
		t.Errorf("the space's Admin restores what a moderator hid: %d", code)
	}
	if n := rowCount(t, e, `SELECT count(*) FROM pages WHERE id = ? AND state = 'trashed'`, page); n != 1 {
		t.Error("the hidden page is back")
	}

	code, body := act(t, e, g, "/cmd/projects/create", "tab1", map[string]any{"key": "SPM", "name": "Spam", "description": ""})
	if code != http.StatusOK {
		t.Fatalf("create a project: %d %s", code, body)
	}
	var project, issue, comment string
	e.DB.R.QueryRow(`SELECT id FROM projects WHERE key = 'SPM'`).Scan(&project)
	if code, body := act(t, e, g, "/cmd/issues/create", "tab1", map[string]any{"project": project, "type": "task", "summary": "Buy now"}); code != http.StatusOK {
		t.Fatalf("create an issue: %d %s", code, body)
	}
	e.DB.R.QueryRow(`SELECT id FROM issues WHERE project_id = ?`, project).Scan(&issue)
	act(t, e, g, "/cmd/issues/"+issue+"/comment", "tab1", map[string]any{"value": "Cheap watches"})
	e.DB.R.QueryRow(`SELECT id FROM comments WHERE subject_id = ?`, issue).Scan(&comment)
	if code, body := act(t, e, mo, "/cmd/moderation/act", "tab1", map[string]any{"action": "hide", "kind": "comment", "id": comment, "reason": "spam"}); code != http.StatusNoContent {
		t.Fatalf("hide the comment: %d %s", code, body)
	}
	if code, _ := act(t, e, g, "/cmd/issues/"+issue+"/chide", "tab1", map[string]any{"value": "show", "ref": comment}); code != http.StatusForbidden {
		t.Errorf("the project's Admin shows what a moderator hid: %d", code)
	}
	if n := rowCount(t, e, `SELECT count(*) FROM comments WHERE id = ? AND hidden_at IS NOT NULL`, comment); n != 1 {
		t.Error("the hidden comment shows again")
	}
	// What the project's Admin hid, they show again.
	act(t, e, g, "/cmd/issues/"+issue+"/comment", "tab1", map[string]any{"value": "Mine"})
	var own string
	e.DB.R.QueryRow(`SELECT id FROM comments WHERE subject_id = ? AND id != ?`, issue, comment).Scan(&own)
	act(t, e, g, "/cmd/issues/"+issue+"/chide", "tab1", map[string]any{"value": "hide", "ref": own})
	act(t, e, g, "/cmd/issues/"+issue+"/chide", "tab1", map[string]any{"value": "show", "ref": own})
	if n := rowCount(t, e, `SELECT count(*) FROM comments WHERE id = ? AND hidden_at IS NULL`, own); n != 1 {
		t.Error("an Admin cannot show a comment they hid")
	}
}

// DM-04.3: a subtask counts against the demo's cap of new issues.
func TestSubtaskCap(t *testing.T) {
	old := demo.Caps[demo.CapIssue]
	demo.Caps[demo.CapIssue] = 1
	defer func() { demo.Caps[demo.CapIssue] = old }()
	e := webtest.Start(t)
	seedProbes(t, e)
	g := webtest.Client(t)
	webtest.Get(t, g, e.URL+"/")
	before := rowCount(t, e, `SELECT count(*) FROM issues`)
	for i := range 3 {
		act(t, e, g, "/cmd/issues/"+web.ProbeIssue+"/subtask", "tab1", map[string]any{"value": fmt.Sprintf("Sub %d", i)})
	}
	if n := rowCount(t, e, `SELECT count(*) FROM issues`) - before; n != 1 {
		t.Errorf("%d subtasks past a cap of 1", n)
	}
}

// NFR-01.2: an issue in a project the person may not read stays unknown:
// its old address, a link to its key and a parent by its key answer as
// for a key that names nothing.
func TestClosedKeysStayUnknown(t *testing.T) {
	e := webtest.Start(t)
	seedProbes(t, e)
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		_, err := tx.Exec(`UPDATE scopes SET open = 0 WHERE id = ?`, probeWeb)
		return err
	}))
	g := webtest.Client(t)
	webtest.Get(t, g, e.URL+"/")
	if code := status(t, g, e.URL+"/ticket/"+web.ProbeIssue); code != http.StatusNotFound {
		t.Errorf("the old address of a closed project's issue: %d", code)
	}
	answer := func(key string) string {
		op := "op-" + strings.ToLower(strings.ReplaceAll(key, "-", ""))
		code, _ := act(t, e, g, "/cmd/issues/"+probeMoved+"/link", "tab1", map[string]any{"op": op + "l", "value": key, "ref": "relates"})
		o := outcome(t, e, guest(t, e, g), op+"l")
		code2, _ := act(t, e, g, "/cmd/issues/create", "tab1", map[string]any{"op": op + "c", "project": probeApp, "type": "task", "summary": "S", "parent": key})
		o2 := outcome(t, e, guest(t, e, g), op+"c")
		return fmt.Sprintf("%d %s %s | %d %s %s", code, o.Field, strings.ReplaceAll(o.Msg, key, "KEY"), code2, o2.Field, strings.ReplaceAll(o2.Msg, key, "KEY"))
	}
	if closed, unknown := answer("WEB-1"), answer("WEB-999"); closed != unknown {
		t.Errorf("a closed project's key: %s\nan unknown key: %s", closed, unknown)
	}
}

// NFR-41, DM-04: one address cannot take every stream: it holds 40 at
// once and opens them at a second each after a burst of 40.
func TestStreamsPerAddress(t *testing.T) {
	e := webtest.Start(t)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	from := func(addr string) http.Header { return http.Header{"X-Forwarded-For": {addr}} }
	code := func(addr, tab string) int {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "GET", e.URL+"/stream?tab="+tab+"&v=", nil)
		req.Header = from(addr)
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	var closers []func()
	for i := range web.MaxStreamsPerAddr {
		_, stop := webtest.Open(t, c, fmt.Sprintf("%s/stream?tab=t%d&v=", e.URL, i), from("203.0.113.7"))
		closers = append(closers, stop)
	}
	if got := code("203.0.113.7", "one-more"); got != http.StatusServiceUnavailable {
		t.Errorf("a stream past the address's cap: %d", got)
	}
	if got := code("198.51.100.9", "elsewhere"); got != http.StatusOK {
		t.Errorf("a stream from another address: %d", got)
	}
	for _, stop := range closers {
		stop()
	}
	limited := false
	for i := 0; i < web.MaxStreamsPerAddr+5 && !limited; i++ {
		limited = code("192.0.2.4", fmt.Sprintf("r%d", i)) == http.StatusTooManyRequests
	}
	if !limited {
		t.Error("an address opened streams without a limit")
	}
}

// NFR-09.2: a session guesses its account's current password 5 times at
// most from an address; then even the right one waits.
func TestPasswordChangeLockout(t *testing.T) {
	e := webtest.Start(t)
	ada := e.Account(t, "ada@example.com", "correct horse battery", people.NoRole)
	c := webtest.Client(t)
	e.Login(t, c, "ada@example.com", "correct horse battery")
	for i := range 5 {
		webtest.Post(t, c, e.URL+"/cmd/account/password", fmt.Sprintf(`{"tab":"tab1","op":"w%d","current":"guess %d","password":"a new password"}`, i, i))
	}
	webtest.Post(t, c, e.URL+"/cmd/account/password", `{"tab":"tab1","op":"right","current":"correct horse battery","password":"a new password"}`)
	if o := outcome(t, e, people.Person{ID: ada, Kind: people.Account}, "right"); o.OK || o.Msg != people.MsgLocked {
		t.Errorf("the right password after 5 wrong ones: %+v", o)
	}
}

// NFR-01.1: typing on an issue, and linking it, ask for the right to read
// it first: an outsider of a closed project hears 404, not 204.
func TestClosedIssueTypingAndLink(t *testing.T) {
	e := webtest.Start(t)
	seedProbes(t, e)
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		_, err := tx.Exec(`UPDATE scopes SET open = 0 WHERE id = ?`, probeWeb)
		return err
	}))
	g := webtest.Client(t)
	webtest.Get(t, g, e.URL+"/")
	if code, _ := webtest.Post(t, g, e.URL+"/cmd/issues/"+web.ProbeIssue+"/typing", `{"tab":"tab1","op":"t1","field":"comment"}`); code != http.StatusNotFound {
		t.Errorf("typing on a closed project's issue: %d", code)
	}
	if code, _ := webtest.Post(t, g, e.URL+"/cmd/issues/"+web.ProbeIssue+"/link", `{"tab":"tab1","op":"l1","value":"APP-9","ref":"relates"}`); code != http.StatusNotFound {
		t.Errorf("a link from a closed project's issue: %d", code)
	}
}

// NFR-01.1, DM-03.1: an op id of one's own earlier action, sent with an
// action one may not take, gets no replay of the first answer: a guest
// cannot start the reset with the op id of their rename.
func TestOpIDReplayAcrossActions(t *testing.T) {
	e := webtest.Start(t)
	g := webtest.Client(t)
	webtest.Get(t, g, e.URL+"/")
	if code, _ := webtest.Post(t, g, e.URL+"/cmd/people/rename", `{"tab":"tab1","op":"same-op","name":"Mallory"}`); code != http.StatusNoContent {
		t.Fatalf("rename: %d", code)
	}
	if code, _ := webtest.Post(t, g, e.URL+"/cmd/admin/reset", `{"tab":"tab1","op":"same-op"}`); code != http.StatusForbidden {
		t.Errorf("a guest's reset with the op id of their rename: %d", code)
	}
	time.Sleep(100 * time.Millisecond)
	if n := rowCount(t, e, `SELECT count(*) FROM demo_resets`); n != 0 || e.Server.Demo().Resetting() {
		t.Errorf("a guest reset the demo (%d resets)", n)
	}
	// A retry of the same action still gets its first answer.
	if code, _ := webtest.Post(t, g, e.URL+"/cmd/people/rename", `{"tab":"tab1","op":"same-op","name":"Other"}`); code != http.StatusNoContent {
		t.Errorf("a retry: %d", code)
	}
	if n := rowCount(t, e, `SELECT count(*) FROM people WHERE display_name = 'Other'`); n != 0 {
		t.Error("a retry ran again")
	}
}
