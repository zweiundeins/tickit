package web_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web"
	"github.com/zweiundeins/tickit/internal/web/webtest"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// NFR-28: the app's log of a full run, at debug level, holds no text of
// issues, pages or comments. Every piece of content carries a marker of
// its own, and the log is searched for each, as webtest searches it for
// secrets (NFR-11).
func TestLogsHoldNoContent(t *testing.T) {
	e := webtest.StartWith(t, webtest.Options{Store: wikiStore(), Configure: web.Unlimited})
	seedProbes(t, e)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	events := webtest.Listen(t, c, e.URL+"/stream?tab=tab1", nil)
	webtest.Next(t, events, `"_outcomes"`)

	salt := make([]byte, 4)
	rand.Read(salt)
	var markers []string
	marker := func(what string) string {
		m := "zqmark" + what + hex.EncodeToString(salt)
		markers = append(markers, m)
		return m
	}
	ok := func(what string, code int, body string) {
		t.Helper()
		if code != http.StatusNoContent && code != http.StatusOK {
			t.Fatalf("%s: %d %s", what, code, body)
		}
	}

	// An issue: created, edited, commented on, its comment edited and
	// deleted, and changes refused with the content in them.
	label := marker("label")
	code, body := act(t, e, c, "/cmd/issues/create", "tab1", map[string]any{"project": probeWeb, "type": "task",
		"summary": marker("summary"), "description": "Text " + marker("description"), "labels": label})
	ok("create issue", code, body)
	key, err := tracker.IssueByKey(context.Background(), e.DB.R, "WEB-4")
	if err != nil {
		t.Fatal(err)
	}
	issue := key.Issue
	seen := func() int64 {
		d, err := tracker.IssueDetail(context.Background(), e.DB.R, issue)
		if err != nil {
			t.Fatal(err)
		}
		return d.Version
	}
	field := func(name string, fields map[string]any) {
		t.Helper()
		code, body := act(t, e, c, "/cmd/issues/"+issue+"/"+name, "tab1", fields)
		ok(name, code, body)
	}
	summary := marker("summaryedit")
	field("summary", map[string]any{"seen": seen(), "value": summary})
	field("description", map[string]any{"seen": seen(), "value": "More " + marker("descriptionedit")})
	field("summary", map[string]any{"seen": seen(), "value": strings.Repeat(marker("toolong"), 30)})
	field("summary", map[string]any{"seen": 1, "value": marker("conflict")})
	field("comment", map[string]any{"value": "A comment " + marker("comment")})
	comments, err := tracker.Comments(context.Background(), e.DB.R, issue)
	if err != nil || len(comments) != 1 {
		t.Fatalf("comments: %v %d", err, len(comments))
	}
	field("cedit", map[string]any{"value": "Edited " + marker("commentedit"), "ref": comments[0].ID})
	field("cdelete", map[string]any{"ref": comments[0].ID})
	// A malformed body that holds content answers 400.
	if code, _ := webtest.Post(t, c, e.URL+"/cmd/issues/"+issue+"/summary", `{"tab":"tab1","op":"m1","seen":"x","value":"`+marker("malformed")+`"}`); code != http.StatusBadRequest {
		t.Errorf("a malformed action: %d", code)
	}

	// A page: created, renamed, written in the editor, saved as a version,
	// commented on with a reply, the comment deleted.
	space, home := wikiSpace(t, e, c, "ENG")
	page := wikiPage(t, e, c, space, home, marker("title"))
	title := marker("retitle")
	code, body = act(t, e, c, "/cmd/pages/rename", "tab1", map[string]any{"page": page, "title": title})
	ok("rename", code, body)
	h := head(t, e, page)
	code, body = act(t, e, c, "/cmd/pages/"+page+"/open", "tab1", map[string]any{"base": h.Version, "client": "0123456789ab"})
	ok("open", code, body)
	code, body = webtest.Post(t, c, e.URL+"/cmd/pages/"+page+"/commit",
		fmt.Sprintf(`{"tab":"tab1","epoch":%q,"base":%d,"ref":"k1","client":"0123456789ab","steps":[{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"text","text":%q}]}}]}`,
			h.Epoch, h.Version, "Typed "+marker("pagetext")))
	ok("commit", code, body)
	// A commit the rules refuse, with content in it.
	webtest.Post(t, c, e.URL+"/cmd/pages/"+page+"/commit",
		fmt.Sprintf(`{"tab":"tab1","epoch":%q,"base":%d,"ref":"k2","client":"0123456789ab","steps":[{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"page_link","attrs":{"pageId":%q}}]}}]}`,
			h.Epoch, h.Version+1, marker("badlink")))
	code, body = act(t, e, c, "/cmd/pages/save-version", "tab1", map[string]any{"page": page, "message": marker("versionmessage")})
	ok("save version", code, body)
	code, body = act(t, e, c, "/cmd/pages/comment", "tab1", map[string]any{"page": page, "body": "On the page " + marker("pagecomment")})
	ok("page comment", code, body)
	pageComments, err := wiki.Comments(context.Background(), e.DB.R, page)
	if err != nil || len(pageComments) == 0 {
		t.Fatalf("page comments: %v %d", err, len(pageComments))
	}
	code, body = act(t, e, c, "/cmd/pages/comment", "tab1", map[string]any{"page": page, "parent": pageComments[0].ID, "body": "A reply " + marker("reply")})
	ok("reply", code, body)
	code, body = act(t, e, c, "/cmd/pages/comment/delete", "tab1", map[string]any{"page": page, "comment": pageComments[0].ID})
	ok("delete page comment", code, body)

	// Every page that shows them, the searches for them and their views.
	q := url.QueryEscape(summary)
	for _, p := range []string{"/WEB-4", "/issues?q=" + q, "/search?q=" + q, "/search/suggest?q=" + q, "/search?q=" + url.QueryEscape(marker("query")),
		"/pages/" + page, wiki.PageURL("ENG", page, title), "/spaces/ENG/pages/" + page + "/edit", "/spaces/ENG", "/spaces/ENG/pages/" + page + "/history", "/spaces/ENG/pages/" + page + "/versions/1",
		"/projects/WEB/board", "/labels/" + label, "/projects/WEB/activity", "/spaces/ENG/pages/" + page + "/compare?from=1&to=2"} {
		res, err := c.Get(e.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	views := webtest.Listen(t, c, e.URL+"/stream?tab=tab2&v=issue:"+issue+",pagedoc:"+page+",page:"+page+",board:WEB", nil)
	webtest.Next(t, views, summary)

	e.StopStreams()
	log := e.Logs()
	if !strings.Contains(log, "/spaces/ENG/pages/"+page+"/- ") || !strings.Contains(log, "path=/labels/- ") {
		t.Error("the page's and the label's requests are not in the log as LogPath writes them")
	}
	if !strings.Contains(log, "request") {
		t.Fatal("the server logged no requests")
	}
	for _, m := range markers {
		if strings.Contains(log, m) {
			for _, line := range strings.Split(log, "\n") {
				if strings.Contains(line, m) {
					t.Errorf("NFR-28: the log holds content: %s", line)
					break
				}
			}
		}
	}
}
