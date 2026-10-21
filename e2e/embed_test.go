package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// FR-104, FR-105 in real browsers: tab A types a filter into an issue
// list in the editor; tab B changes the status of an issue it shows, and
// A's rows and a reader's table follow. Then A selects text and creates
// an issue from it: its chip replaces the text in a co-editor's editor.
func TestIssueListAndCreateFromPage(t *testing.T) {
	s := newRealServer(t)
	s.seedAll()
	page, edit := s.wikiPage(t)
	b := browsertest.Start(t)

	a := b.NewPage(t, 1200, 900)
	a.Navigate(t, edit)
	editorReady(t, a)
	co := b.NewPage(t, 1200, 900)
	co.Navigate(t, edit)
	editorReady(t, co)
	reader := b.NewPage(t, 1200, 900)
	reader.Navigate(t, s.URL+wiki.PageURL("CO", page, "Typing together"))

	// A inserts an issue list and types its filter.
	a.Eval(t, `[...document.querySelectorAll('.wk-insert button')].find(b => b.textContent.includes('Issue list')).click(), true`, nil)
	a.Wait(t, `document.activeElement?.closest('.issue-list--edit') && document.activeElement.type === 'text'`, 5*time.Second)
	for _, r := range "project=WEB&type=bug&sort=key&dir=asc" {
		a.Call(t, "Input.insertText", map[string]any{"text": string(r)})
	}
	a.Press(t, "Enter")
	rows := `document.querySelectorAll('.pm-mount .issue-list__rows tr[data-key]').length > 0`
	a.Wait(t, rows, 10*time.Second)
	var key string
	a.Eval(t, `document.querySelector('.pm-mount .issue-list__rows tr[data-key]').dataset.key`, &key)
	co.Wait(t, `document.querySelector('.pm-mount .issue-list--edit input[type=text]')?.value === 'project=WEB&type=bug&sort=key&dir=asc'`, 5*time.Second)
	reader.Wait(t, `document.querySelector('#page-body .issue-list tr[data-key=`+jsonString(key)+`]')`, 10*time.Second)

	// B changes the issue's status through its own action.
	var id, status string
	if err := s.db.View(context.Background(), func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT id, status FROM issues WHERE key = ?`, key).Scan(&id, &status)
	}); err != nil {
		t.Fatal(err)
	}
	to, name := tracker.StatusInReview, "In Review"
	if status == to {
		to, name = tracker.StatusToDo, "To Do"
	}
	tb := b.NewPage(t, 1200, 900)
	tb.Navigate(t, s.URL+"/"+key)
	tb.Wait(t, `document.querySelector('#issue-main[data-pid]')`, 10*time.Second)
	var code int
	tb.Eval(t, fmt.Sprintf(`fetch('/cmd/issues/%s/status', {method: 'POST', headers: {'Content-Type': 'application/json', 'Datastar-Request': 'true'}, body: JSON.stringify({tab: document.body.dataset.signals.match(/_tab: "([^"]+)"/)[1], op: 'e2e-st', seen: 1099511627776, value: %q})}).then(r => r.status)`, id, to), &code)
	if code != 204 {
		t.Fatalf("FR-16: B's status change: %d", code)
	}
	row := func(scope string) string {
		return `document.querySelector('` + scope + ` tr[data-key=` + strings.ReplaceAll(jsonString(key), `"`, `\"`) + `] .tk-lozenge')?.textContent.trim() === ` + jsonString(name)
	}
	a.Wait(t, row(".pm-mount .issue-list__rows"), 5*time.Second)
	reader.Wait(t, row("#page-body .issue-list"), 5*time.Second)

	// A selects text in the editor and creates an issue from it.
	a.Eval(t, `(() => { const v = document.getElementById('editor').view; let at = 0;
		v.state.doc.descendants((n, pos) => { if (!at && n.isTextblock) at = pos + 1 + n.content.size; return !at });
		v.focus(); v.dispatch(v.state.tr.setSelection(v.state.selection.constructor.near(v.state.doc.resolve(at)))); return true })()`, nil)
	for _, r := range "Rotate the signing keys" {
		a.Call(t, "Input.insertText", map[string]any{"text": string(r)})
	}
	a.Eval(t, `(() => { const v = document.getElementById('editor').view, $h = v.state.selection.$head;
		v.dispatch(v.state.tr.setSelection(v.state.selection.constructor.create(v.state.doc, $h.pos - 'signing keys'.length, $h.pos))); return true })()`, nil)
	a.Wait(t, `!document.querySelector('.wk-sel').hidden`, 5*time.Second)
	a.Eval(t, `document.querySelector('.wk-sel button').click(), true`, nil)
	a.Wait(t, `document.getElementById('page-issue').open && document.getElementById('page-issue-summary').value === 'signing keys'`, 5*time.Second)
	a.Eval(t, `[...document.querySelectorAll('#page-issue button')].find(b => b.textContent.includes('Create issue')).click(), true`, nil)
	a.Wait(t, `document.querySelector('.wk-issue-done')?.textContent.startsWith('Created ')`, 10*time.Second)
	var made string
	a.Eval(t, `document.querySelector('.wk-issue-done').textContent.split(' ')[1].replace(',', '')`, &made)
	co.Wait(t, `[...document.querySelectorAll('.pm-mount a.issue-chip')].some(c => c.dataset.key === `+jsonString(made)+`) && document.querySelector('.pm-mount').textContent.includes('Rotate the ') && !document.querySelector('.pm-mount').textContent.includes('signing keys')`, 10*time.Second)
	var linked int
	if err := s.db.View(context.Background(), func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM issue_pages p JOIN issues i ON i.id = p.issue_id WHERE i.key = ? AND i.summary = 'signing keys' AND p.page_id = ?`, made, page).Scan(&linked)
	}); err != nil {
		t.Fatal(err)
	}
	if linked != 1 {
		t.Errorf("FR-105.1: %s does not link back to the page", made)
	}
	for _, p := range []*browsertest.Page{a, co, reader, tb} {
		if errs := p.Errors(); len(errs) > 0 {
			t.Errorf("page errors: %v", errs)
		}
	}
}
