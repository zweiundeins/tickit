package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zweiundeins/prosemirror-go"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
)

// descIssue makes an open project with an issue whose description is
// "Start." on the server, and returns the issue's id.
func (s *realServer) descIssue(t testing.TB) string {
	t.Helper()
	ada := people.Seeded[0]
	if err := s.do(people.SeedPeople{At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	b, err := s.w.DoValue(context.Background(), func(tx *writerTx) ([]byte, error) {
		return tracker.CreateProject{By: tabs.By{Actor: ada, Op: "p1", At: time.Now()}, Key: "CO", Name: "Co-editing", Open: true}.ApplyResult(tx)
	})
	if err != nil {
		t.Fatal(err)
	}
	project := jsonField(b, "id")
	b, err = s.w.DoValue(context.Background(), func(tx *writerTx) ([]byte, error) {
		return tracker.CreateIssue{By: tabs.By{Actor: ada, Op: "i1", At: time.Now()}, Project: project, Type: tracker.Task,
			Summary: "Write it together", Description: tracker.PlainDoc("Start.")}.ApplyResult(tx)
	})
	if err != nil {
		t.Fatal(err)
	}
	return jsonField(b, "id")
}

// FR-15.2, MP-05.1, MP-03: two people edit one issue's description at
// once, each in their own browser, and end with identical documents that
// hold every character they typed; a third, who only reads the issue,
// sees the description change live; once both are done, the version is
// in the issue's history (FR-68).
func TestDescriptionCoEditing(t *testing.T) {
	s := newRealServer(t)
	id := s.descIssue(t)
	b := browsertest.Start(t)
	open := func(i int) *browsertest.Page {
		p := b.NewPage(t, 1000, 800)
		p.Call(t, "Network.enable", nil)
		p.Call(t, "Network.setExtraHTTPHeaders", map[string]any{"headers": map[string]string{"X-Real-IP": fmt.Sprintf("10.2.0.%d", i+1)}})
		p.Navigate(t, s.URL+"/CO-1")
		return p
	}
	reader := open(9)
	editors := []*browsertest.Page{open(0), open(1)}
	for _, p := range editors {
		p.Wait(t, `document.querySelector('#issue-desc [data-act="desc-edit"]') && !document.querySelector('#issue-desc [data-act="desc-edit"]').disabled`, 10*time.Second)
		p.Eval(t, `(document.querySelector('#issue-desc [data-act="desc-edit"]').click(), true)`, nil)
		editorReady(t, p)
	}
	letters := "xy" // letters "Start." does not hold
	typed := make([]int, len(editors))
	var wg sync.WaitGroup
	start := time.Now()
	for i, p := range editors {
		wg.Go(func() {
			time.Sleep(time.Duration(i) * 100 * time.Millisecond)
			tick := time.NewTicker(200 * time.Millisecond) // 5 characters a second
			defer tick.Stop()
			for time.Since(start) < 6*time.Second {
				<-tick.C
				p.Call(t, "Input.insertText", map[string]any{"text": string(letters[i])})
				typed[i]++
			}
		})
	}
	wg.Wait()
	// MP-05.4: both saved and holding the same document, with every
	// keystroke.
	var docs []string
	for deadline := time.Now().Add(30 * time.Second); ; {
		docs = docs[:0]
		for _, p := range editors {
			var j string
			p.Eval(t, `JSON.stringify(document.getElementById('editor').json())`, &j)
			docs = append(docs, j)
		}
		if docs[0] == docs[1] && countAll(docs[0], letters) == typed[0]+typed[1] {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("MP-05.1: the editors differ or lost keystrokes (typed %v):\n%s\n%s", typed, docs[0], docs[1])
		}
		time.Sleep(20 * time.Millisecond)
	}
	for i := range editors {
		if got := strings.Count(docText(docs[0]), string(letters[i])); got != typed[i] {
			t.Errorf("editor %d typed %d %c, the description holds %d", i, typed[i], letters[i], got)
		}
	}
	text := docText(docs[0])
	if !strings.Contains(text, "Start.") {
		t.Errorf("the description lost its text: %q", text)
	}
	var head tracker.DescHead
	s.db.View(context.Background(), func(tx *sql.Tx) (err error) {
		head, err = tracker.DescriptionHead(context.Background(), tx, id)
		return err
	})
	serverDoc, _ := prosemirror.MarshalDoc(head.Doc)
	if !sameDoc(string(serverDoc), docs[0]) {
		t.Errorf("the server's description differs from the editors'\n%s\n%s", serverDoc, docs[0])
	}
	// MP-03: the reader sees it, without a reload.
	reader.Wait(t, fmt.Sprintf(`document.getElementById('issue-desc-body').textContent.trim() === %q`, text), 10*time.Second)
	for _, p := range editors {
		var status string
		p.Eval(t, `document.querySelector('.tk-issue__desceditor .wk-status').dataset.state`, &status)
		if status != "saved" {
			t.Errorf("MP-05.5: the status is %q", status)
		}
	}
	// Done in both: the editor goes, the version is cut ("left") and the
	// history says so.
	for _, p := range editors {
		p.Eval(t, `(document.querySelector('[data-act="desc-done"]').click(), true)`, nil)
		p.Wait(t, `!document.getElementById('editor') && !document.getElementById('issue-desc-body').hidden`, 5*time.Second)
	}
	reader.Wait(t, `document.getElementById('issue-history').textContent.includes('changed the description')`, 15*time.Second)
}
