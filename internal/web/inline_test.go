package web_test

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web/webtest"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// FR-91: an inline comment through the actions. The page shows the panel
// and the version a selection is at; every open reader sees the
// highlight and the comment within a tick, and every editor the range in
// its frame; resolving hides the highlight and lists it as resolved;
// deleting its text detaches it; versions render as before.
func TestInlineCommentsLive(t *testing.T) {
	e := startWiki(t)
	c, o := webtest.Client(t), webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	webtest.Get(t, o, e.URL+"/")
	space, home := wikiSpace(t, e, c, "ENG")
	page := wikiPage(t, e, c, space, home, "Notes")
	e.Do(t, people.SeedPeople{At: time.Now()})
	e.Do(t, wiki.SetDocument{By: wikiBy(people.Seeded[0], "d1"), Page: page, Doc: tracker.PlainDoc("Say hello to the team"), Message: "Text"})
	h := head(t, e, page)
	url := wiki.PageURL("ENG", page, "Notes")
	_, html := webtest.Get(t, o, e.URL+url)
	for _, want := range []string{`id="page-inline"`, `id="wk-ic-form"`, `id="wk-ic-add"`, `data-v="` + strconv.FormatInt(h.Version, 10) + `"`, `data-epoch="` + h.Epoch + `"`, `/static/wiki/inline.`} {
		if !strings.Contains(html, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
	reader := webtest.Listen(t, o, e.URL+"/stream?tab=rtab&v=pagedoc:"+page, nil)
	panel := webtest.Listen(t, o, e.URL+"/stream?tab=ptab&v=inline:"+page, nil)
	webtest.Next(t, reader, "Say hello")
	webtest.Next(t, panel, "page-inline")

	code, body := act(t, e, c, "/cmd/pages/comment/inline", "tab1", map[string]any{"page": page, "epoch": h.Epoch, "base": h.Version, "from": 5, "to": 10, "text": "hello", "body": "Which hello?"})
	if code != http.StatusOK || !strings.Contains(body, "_icMade") {
		t.Fatalf("inline comment: %d %s", code, body)
	}
	list := inlineOf(t, e, page)
	if len(list) != 1 {
		t.Fatalf("inline comments %+v", list)
	}
	id := list[0].ID
	webtest.Next(t, reader, `<mark class="tk-ic" data-c="`+id+`">hello</mark>`)
	if frame := webtest.Next(t, panel, "Which hello?"); !strings.Contains(frame, `id="ic-`+id+`"`) || !strings.Contains(frame, `data-state="open"`) {
		t.Errorf("the panel: %s", frame)
	}
	// The page's own comments leave it out.
	if _, html := webtest.Get(t, o, e.URL+url); !strings.Contains(html, "No comments yet.") {
		t.Error("the inline comment lists among the page's comments")
	}
	// An editor's frame carries the range at its head.
	act(t, e, c, "/cmd/pages/"+page+"/open", "etab", map[string]any{"base": h.Version, "client": "0123456789ab"})
	if _, html := webtest.Get(t, c, e.URL+wiki.EditURL("ENG", page)); !strings.Contains(html, `comments="{&#34;v&#34;:`+strconv.FormatInt(h.Version, 10)+`,&#34;list&#34;:[{&#34;id&#34;:&#34;`+id+`&#34;,&#34;from&#34;:5,&#34;to&#34;:10}]}"`) {
		t.Errorf("the editor's host has no comments attribute: %s", hostOf(html))
	}

	// A reply, as page comments reply.
	if code, _ := act(t, e, o, "/cmd/pages/comment", "tabo", map[string]any{"page": page, "parent": id, "body": "This one"}); code != http.StatusNoContent {
		t.Errorf("reply: %d", code)
	}
	webtest.Next(t, panel, "This one")

	// A stale selection maps; another text is refused next to the form.
	commit(t, e, c, page, h, `{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"text","text":"Now "}]}}`)
	webtest.Next(t, reader, `Now Say <mark`)
	if code, _ := act(t, e, c, "/cmd/pages/comment/inline", "tab1", map[string]any{"page": page, "epoch": h.Epoch, "base": h.Version, "from": 18, "to": 22, "text": "tame", "body": "No"}); code != http.StatusNoContent {
		t.Errorf("another text: %d", code)
	}
	if n := len(inlineOf(t, e, page)); n != 1 {
		t.Errorf("a refused comment was kept: %d", n)
	}

	// Resolve: the highlight goes, the comment lists as resolved.
	if code, _ := act(t, e, o, "/cmd/pages/comment/resolve", "tabo", map[string]any{"comment": id, "resolved": true}); code != http.StatusNoContent {
		t.Fatalf("resolve: %d", code)
	}
	if frame := webtest.Next(t, reader, "Now Say hello"); strings.Contains(frame, `class="tk-ic"`) {
		t.Errorf("a resolved comment's highlight: %s", frame)
	}
	webtest.Next(t, panel, "Resolved (1)")
	act(t, e, o, "/cmd/pages/comment/resolve", "tabo", map[string]any{"comment": id, "resolved": false})
	webtest.Next(t, reader, `data-c="`+id+`"`)

	// Deleting the word detaches the comment.
	h2 := head(t, e, page)
	commit(t, e, c, page, h2, `{"stepType":"replace","from":9,"to":14}`)
	if frame := webtest.Next(t, panel, `data-state="detached"`); !strings.Contains(frame, "Detached") {
		t.Errorf("detached: %s", frame)
	}
	// Versions and their comparison render as ever.
	e.Do(t, wiki.CutVersion{By: wikiBy(people.Seeded[0], "v1"), Page: page})
	for _, u := range []string{wiki.HistoryURL("ENG", page), "/spaces/ENG/pages/" + page + "/versions/1", "/spaces/ENG/pages/" + page + "/compare?from=1&to=3"} {
		if code, html := webtest.Get(t, o, e.URL+u); code != http.StatusOK || strings.Contains(html, `class="tk-ic"`) {
			t.Errorf("%s: %d", u, code)
		}
	}
}

func inlineOf(t *testing.T, e *webtest.Env, page string) []wiki.InlineComment {
	t.Helper()
	var list []wiki.InlineComment
	rows, err := e.DB.R.Query(`SELECT comment_id FROM page_inline WHERE page_id = ? ORDER BY created_at`, page)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var c wiki.InlineComment
		rows.Scan(&c.ID)
		list = append(list, c)
	}
	return list
}

func commit(t *testing.T, e *webtest.Env, c *http.Client, page string, h wiki.Head, step string) {
	t.Helper()
	code, body := webtest.Post(t, c, e.URL+"/cmd/pages/"+page+"/commit", fmt.Sprintf(`{"tab":"etab","epoch":%q,"base":%d,"ref":"r%d","client":"0123456789ab","steps":[%s]}`, h.Epoch, h.Version, opCount.Add(1), step))
	if code != http.StatusNoContent {
		t.Fatalf("commit: %d %s", code, body)
	}
}

func hostOf(html string) string {
	i := strings.Index(html, "<sb-rich-editor")
	if i < 0 {
		return ""
	}
	return html[i:min(len(html), i+600)]
}
