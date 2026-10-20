package e2e

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/wiki"
)

func headText(t *testing.T, s *realServer, page string) string {
	t.Helper()
	var h wiki.Head
	if err := s.db.View(context.Background(), func(tx *sql.Tx) (err error) { h, err = wiki.HeadOf(context.Background(), tx, page); return err }); err != nil {
		return ""
	}
	b, _ := marshal(h)
	return docText(b)
}

func typeText(t *testing.T, p *browsertest.Page, text string) {
	t.Helper()
	for _, r := range text {
		p.Call(t, "Input.insertText", map[string]any{"text": string(r)})
		time.Sleep(20 * time.Millisecond)
	}
}

// MP-05.6, MP-10.3: a server restart in the middle of typing keeps the
// unconfirmed edits; after the restart they merge into the page.
func TestEditorReconnect(t *testing.T) {
	s := newRealServer(t)
	page, edit := s.wikiPage(t)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1000, 700)
	p.Navigate(t, edit)
	editorReady(t, p)
	typeText(t, p, "before ")
	waitFor(t, 10*time.Second, func() bool { return headText(t, s, page) == "before " })
	s.Stop()
	typeText(t, p, "while away ")
	p.Wait(t, `document.getElementById('editor').text().includes('while away')`, 5*time.Second)
	time.Sleep(time.Second)
	s.Start()
	typeText(t, p, "back")
	var state struct {
		Text, Status, Stopped, State, Problem string
		Batches                               int
	}
	defer func() {
		if t.Failed() {
			p.Eval(t, `(() => { const ed = document.getElementById('editor'); return {text: ed.text(), status: ed.closest('article').querySelector('.wk-status').dataset.state || '', stopped: '', state: ed.getAttribute('state'), problem: document.querySelector('.wk-editor-problem').textContent, batches: JSON.parse(ed.getAttribute('batches')).length} })()`, &state)
			t.Logf("server %q; editor %+v; errors %v; logs %v", headText(t, s, page), state, p.Errors(), p.Logs())
		}
	}()
	waitFor(t, 40*time.Second, func() bool { return headText(t, s, page) == "before while away back" })
	p.Wait(t, `document.getElementById('editor').text() === 'before while away back'`, 10*time.Second)
}

// FR-80.3 (Q18), FR-100.2: a reader sees an editor's typing as it
// happens, and a chip in the page follows its issue's status.
func TestReaderSeesEdits(t *testing.T) {
	s := newRealServer(t)
	page, edit := s.wikiPage(t)
	ada := people.Seeded[0]
	b, err := s.w.DoValue(context.Background(), func(tx *writerTx) ([]byte, error) {
		return tracker.CreateProject{By: tabs.By{Actor: ada, Op: "pr1", At: time.Now()}, Key: "WEB", Name: "Web shop", Open: true}.ApplyResult(tx)
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err = s.w.DoValue(context.Background(), func(tx *writerTx) ([]byte, error) {
		return tracker.CreateIssue{By: tabs.By{Actor: ada, Op: "is1", At: time.Now()}, Project: jsonField(b, "id"), Type: tracker.Bug, Summary: "Fix the login"}.ApplyResult(tx)
	})
	if err != nil {
		t.Fatal(err)
	}
	issue := jsonField(b, "id")
	br := browsertest.Start(t)
	editor, reader := br.NewPage(t, 1000, 700), br.NewPage(t, 1000, 700)
	editor.Navigate(t, edit)
	editorReady(t, editor)
	reader.Navigate(t, s.URL+"/pages/"+page)
	reader.Wait(t, `document.querySelector('.tk-presence__item[data-editing]')`, 10*time.Second) // MP-07.3
	typeText(t, editor, "Hello readers")
	reader.Wait(t, `document.getElementById('page-body').textContent.includes('Hello readers')`, 5*time.Second)
	editor.Eval(t, `(document.getElementById('editor').insertChip('WEB-1'), true)`, nil)
	reader.Wait(t, `document.querySelector('#page-body .issue-chip .issue-chip__status')?.textContent === 'To Do'`, 5*time.Second)
	if err := s.do(tracker.SetStatus{By: tabs.By{Actor: ada, Op: "st1", At: time.Now()}, Issue: issue, Seen: 1 << 40, Status: tracker.StatusInReview}); err != nil {
		t.Fatal(err)
	}
	reader.Wait(t, `document.querySelector('#page-body .issue-chip .issue-chip__status')?.textContent === 'In Review'`, 5*time.Second)
	editor.Wait(t, `document.querySelector('.pm-mount .issue-chip')?.textContent.includes('In Review')`, 5*time.Second)
	if body := strings.TrimSpace(func() string {
		var s string
		reader.Eval(t, `document.getElementById('page-body').textContent`, &s)
		return s
	}()); !strings.Contains(body, "WEB-1") {
		t.Errorf("reader body %q", body)
	}
}

func waitFor(t *testing.T, d time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("not after %v", d)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
