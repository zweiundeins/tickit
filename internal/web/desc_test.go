package web_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

func descHead(t *testing.T, e *webtest.Env, issue string) tracker.DescHead {
	t.Helper()
	var h tracker.DescHead
	if err := e.DB.View(context.Background(), func(tx *sql.Tx) (err error) {
		h, err = tracker.DescriptionHead(context.Background(), tx, issue)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return h
}

// FR-15, MP-03, MP-05, MP-06: the description's editor is a host the
// issue's page fetches; an editing tab's view brings it the commits, the
// carets view the others' carets, and every reader of the issue sees the
// description change in the next frame. Done closes it.
func TestDescriptionLive(t *testing.T) {
	e := webtest.Start(t)
	seedProbes(t, e)
	id := web.ProbeIssue
	editor, reader := webtest.Client(t), webtest.Client(t)
	if code, body := webtest.Get(t, editor, e.URL+"/WEB-1"); code != http.StatusOK || !strings.Contains(body, `id="issue-desc-editor"`) || !strings.Contains(body, "A description with") {
		t.Fatalf("the issue's page: %d", code)
	}
	webtest.Get(t, reader, e.URL+"/WEB-1")
	read := webtest.Listen(t, reader, e.URL+"/stream?tab=rtab&v=issue-desc:"+id, nil)
	webtest.Next(t, read, `id="issue-desc-body"`)
	edit := webtest.Listen(t, editor, e.URL+"/stream?tab=etab&v=issue:"+id, nil)
	webtest.Next(t, edit, `"_outcomes"`)

	code, host := webtest.Get(t, editor, e.URL+"/issues/"+id+"/description/editor")
	if code != http.StatusOK || !strings.Contains(host, `<sb-rich-editor id="editor"`) || !strings.Contains(host, `data-cmd="/cmd/issues/`+id+`/description"`) ||
		!strings.Contains(host, `A description with **markup**.`) || !strings.Contains(host, `label="Description"`) {
		t.Fatalf("the editor's host: %d %.400s", code, host)
	}
	h := descHead(t, e, id)
	if code, _ := act(t, e, editor, "/cmd/issues/"+id+"/description/open", "etab", map[string]any{"base": h.Version, "client": "0123456789ab"}); code != http.StatusNoContent {
		t.Fatalf("open: %d", code)
	}
	webtest.Next(t, edit, `<sb-rich-editor id="editor"`)
	webtest.Next(t, edit, `"_carets"`)
	code, _ = webtest.Post(t, editor, e.URL+"/cmd/issues/"+id+"/description/commit",
		fmt.Sprintf(`{"tab":"etab","epoch":%q,"base":%d,"ref":"k1","client":"0123456789ab","steps":[{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"text","text":"Typed live. "}]}}]}`, h.Epoch, h.Version))
	if code != http.StatusNoContent {
		t.Fatalf("commit: %d", code)
	}
	webtest.Next(t, read, `Typed live. A description with`)
	webtest.Next(t, edit, `&#34;ref&#34;:&#34;k1&#34;`)
	webtest.Post(t, editor, e.URL+"/cmd/issues/"+id+"/description/presence", fmt.Sprintf(`{"tab":"etab","epoch":%q,"ack":%d,"v":%d,"anchor":3,"head":5,"seq":1}`, h.Epoch, h.Version+1, h.Version+1))
	webtest.Next(t, edit, `\"anchor\":3,\"head\":5`)
	if !e.Server.EditingDescription(id) {
		t.Error("the editing tab does not watch the carets")
	}
	// Done: the row and both views go; the version is cut ("left").
	if code, _ := act(t, e, editor, "/cmd/issues/"+id+"/description/close", "etab", nil); code != http.StatusNoContent {
		t.Fatalf("close: %d", code)
	}
	var rows int
	e.DB.R.QueryRow(`SELECT count(*) FROM issue_editors`).Scan(&rows)
	if rows != 0 || e.Server.EditingDescription(id) {
		t.Errorf("after Done: %d rows, editing %v", rows, e.Server.EditingDescription(id))
	}
	e.Do(t, tracker.CutDescriptions{At: time.Now(), Editing: e.Server.EditingDescription})
	d, err := tracker.IssueDetail(context.Background(), e.DB.R, id)
	if err != nil || !strings.Contains(string(d.Description), "Typed live.") {
		t.Fatalf("the cut: %s %v", d.Description, err)
	}
	// FR-68: the history's entry, and its comparison with the version
	// before.
	hist, _ := tracker.History(context.Background(), e.DB.R, id)
	last := hist[len(hist)-1]
	if last.Field != tracker.FieldDescription {
		t.Fatalf("history %+v", last)
	}
	code, body := webtest.Get(t, editor, fmt.Sprintf("%s/issues/%s/description?v=%d", e.URL, id, last.Version))
	if code != http.StatusOK || !strings.Contains(body, `<ins class="diff-ins">Typed live. </ins>`) {
		t.Errorf("the comparison: %d %.300s", code, body)
	}
	if code, _ := webtest.Get(t, editor, fmt.Sprintf("%s/issues/%s/description?v=1", e.URL, id)); code != http.StatusOK {
		t.Errorf("the first description: %d", code)
	}
	if code, _ := webtest.Get(t, editor, fmt.Sprintf("%s/issues/%s/description?v=%d", e.URL, id, last.Version+1)); code != http.StatusNotFound {
		t.Errorf("a version that is no description's: %d", code)
	}
}

// The description commit's answers (docs/editor.md, "Wire format"): 204,
// 409, 422, 429 and 403; the editor's host and open answer 403 to a
// Viewer.
func TestDescriptionAnswers(t *testing.T) {
	e := webtest.Start(t)
	seedProbes(t, e)
	id := web.ProbeIssue
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/WEB-1")
	webtest.Listen(t, c, e.URL+"/stream?tab=tab1&v=issue:"+id, nil)
	h := descHead(t, e, id)
	if code, _ := act(t, e, c, "/cmd/issues/"+id+"/description/open", "tab1", map[string]any{"base": h.Version, "client": "0123456789ab"}); code != http.StatusNoContent {
		t.Fatalf("open: %d", code)
	}
	commit := func(c *http.Client, tab, ref, epoch string, base int64, steps string) int {
		code, _ := webtest.Post(t, c, e.URL+"/cmd/issues/"+id+"/description/commit",
			fmt.Sprintf(`{"tab":%q,"epoch":%q,"base":%d,"ref":%q,"client":"0123456789ab","steps":%s}`, tab, epoch, base, ref, steps))
		return code
	}
	text := `[{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"text","text":"Hi"}]}}]`
	if code := commit(c, "tab1", "r1", h.Epoch, h.Version, text); code != http.StatusNoContent {
		t.Errorf("applied: %d", code)
	}
	if code := commit(c, "tab1", "r1", h.Epoch, h.Version, text); code != http.StatusNoContent {
		t.Errorf("applied before: %d", code)
	}
	if code := commit(c, "tab1", "r2", "e-other", h.Version, text); code != http.StatusConflict {
		t.Errorf("MP-13.2, another epoch: %d", code)
	}
	long := `[{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"text","text":"` + strings.Repeat("x", tracker.MaxIssueText) + `"}]}}]`
	if code := commit(c, "tab1", "r3", h.Epoch, h.Version+1, long); code != http.StatusUnprocessableEntity {
		t.Errorf("DM-05, MP-14: %d", code)
	}
	var refusal string
	e.DB.R.QueryRow(`SELECT refusal FROM issue_editors WHERE tab_id = 'tab1'`).Scan(&refusal)
	if !strings.Contains(refusal, `"ref":"r3"`) || !strings.Contains(refusal, "20,000") {
		t.Errorf("the refusal in the editor's row: %q", refusal)
	}
	if code := commit(c, "tab1", "r4", h.Epoch, h.Version+1, `[{"stepType":"nope"}]`); code != http.StatusBadRequest {
		t.Errorf("malformed steps: %d", code)
	}
	// A Viewer of a closed project: 403 for the host, the open and a commit.
	closed, issue := tracker.NewID(), tracker.NewID()
	ada := people.Seeded[0]
	e.Do(t, tracker.CreateProject{By: byOf(ada, "c1"), ID: closed, Key: "SEC", Name: "Secret"})
	e.Do(t, tracker.CreateIssue{By: byOf(ada, "c2"), ID: issue, Project: closed, Type: tracker.Task, Summary: "Hidden", Description: tracker.PlainDoc("Secret")})
	vid := e.Account(t, "vera@example.com", "correct horse battery", people.NoRole)
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		return access.SetMember(tx, access.ProjectScope(closed), vid, access.Viewer, ada.ID, time.Now())
	}))
	vera := webtest.Client(t)
	e.Login(t, vera, "vera@example.com", "correct horse battery")
	webtest.Get(t, vera, e.URL+"/SEC-1")
	webtest.Listen(t, vera, e.URL+"/stream?tab=vtab&v=issue:"+issue, nil)
	if code, _ := webtest.Get(t, vera, e.URL+"/issues/"+issue+"/description/editor"); code != http.StatusForbidden {
		t.Errorf("a Viewer's host: %d", code)
	}
	if code, _ := act(t, e, vera, "/cmd/issues/"+issue+"/description/open", "vtab", map[string]any{"base": 1, "client": "0123456789ab"}); code != http.StatusForbidden {
		t.Errorf("a Viewer's open: %d", code)
	}
	vh := descHead(t, e, issue)
	code, _ := webtest.Post(t, vera, e.URL+"/cmd/issues/"+issue+"/description/commit",
		fmt.Sprintf(`{"tab":"vtab","epoch":%q,"base":%d,"ref":"v2","client":"0123456789ab","steps":%s}`, vh.Epoch, vh.Version, text))
	if code != http.StatusConflict {
		t.Errorf("a Viewer's commit: %d", code) // collab's Readonly; the frame says readonly
	}
	// A deleted issue stops its editors: 409.
	e.Do(t, tracker.DeleteIssue{By: byOf(ada, "d1"), Issue: id})
	if code := commit(c, "tab1", "r5", h.Epoch, descHead(t, e, id).Version, text); code != http.StatusConflict {
		t.Errorf("a deleted issue: %d", code)
	}
	limited := 0
	for i := range 80 {
		if commit(c, "tab1", fmt.Sprintf("x%d", i), h.Epoch, h.Version+1, text) == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 {
		t.Error("DM-04: no commit over the rate limit answered 429")
	}
}

// NFR-13: the description's host holds the document in a script whose
// "<" is escaped, so a description's text never becomes markup there.
// NFR-02.3: a tab id another person names gets an editor view of its own.
func TestDescriptionHostHoldsNoMarkup(t *testing.T) {
	e := webtest.Start(t)
	seedProbes(t, e)
	id := web.ProbeIssue
	evil := `</script><div data-init='alert(1)'>x</div><!--`
	e.Do(t, tracker.SetDescription{By: byOf(people.Seeded[0], "evil"), Issue: id, Description: tracker.PlainDoc(evil)})
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/WEB-1")
	code, host := webtest.Get(t, c, e.URL+"/issues/"+id+"/description/editor")
	if code != http.StatusOK || strings.Contains(host, `<div data-init='alert(1)'>`) || !strings.Contains(host, `\u003c/script>`) {
		i := strings.Index(host, "application/json")
		t.Errorf("the host: %d %.600s", code, host[max(i, 0):])
	}
	if _, page := webtest.Get(t, c, e.URL+"/WEB-1"); strings.Contains(page, `<div data-init='alert(1)'>`) {
		t.Error("the description's text reached the issue's page as markup")
	}
	webtest.Listen(t, c, e.URL+"/stream?tab=tab1&v=issue:"+id, nil)
	h := descHead(t, e, id)
	if code, _ := act(t, e, c, "/cmd/issues/"+id+"/description/open", "tab1", map[string]any{"base": h.Version, "client": "0123456789ab"}); code != http.StatusNoContent {
		t.Fatalf("open: %d", code)
	}
	other := webtest.Client(t)
	webtest.Get(t, other, e.URL+"/WEB-1")
	ev := webtest.Listen(t, other, e.URL+"/stream?tab=t2&v=desc-editor:"+id+".tab1", nil)
	frame := webtest.Next(t, ev, `<sb-rich-editor id="editor"`)
	if !strings.Contains(frame, `state="reset"`) {
		t.Errorf("NFR-02.3: another person's view of the tab's editor: %.400s", frame)
	}
}
