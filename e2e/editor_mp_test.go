package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// offline cuts the page's network, or gives it back.
func offline(t *testing.T, p *browsertest.Page, off bool) {
	t.Helper()
	p.Call(t, "Network.emulateNetworkConditions", map[string]any{"offline": off, "latency": 0, "downloadThroughput": -1, "uploadThroughput": -1})
}

// editorState is what the page and its editor show.
type editorState struct {
	Text, State, Notice, Status, Saved, Copy string
	CopyShown                                bool
}

const readEditor = `(() => {
	const ed = document.getElementById('editor'), copy = document.getElementById('wk-copy')
	return {text: ed.text(), state: ed.getAttribute('state'), notice: ed.shadowRoot.getElementById('notice-text')?.textContent || '',
		status: [...document.querySelectorAll('.wk-status > span')].filter(s => s.style.display !== 'none').map(s => s.textContent.trim()).join('|'),
		saved: ed.shadowRoot.getElementById('saved')?.textContent || '',
		copy: document.getElementById('wk-copy-body').textContent, copyShown: !copy.hidden}
})()`

// MP-05.6: after a gap the editor cannot merge (the commits after its
// version were folded while it was offline), the person's unsaved text
// shows as a copy beside the page, and stays there, in the tab, when the
// page loads again with the page as it is now.
func TestEditorGapCopy(t *testing.T) {
	s := newRealServer(t)
	page, edit := s.wikiPage(t)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1300, 800)
	p.Call(t, "Network.enable", nil)
	p.Navigate(t, edit)
	editorReady(t, p)
	typeText(t, p, "before ")
	waitFor(t, 10*time.Second, func() bool { return headText(t, s, page) == "before " })
	offline(t, p, true)
	typeText(t, p, "while away")
	p.Wait(t, `document.getElementById('editor').text().includes('while away')`, 5*time.Second)

	// Meanwhile the server restarts (the tab's stream is gone), someone
	// else rewrites the page, a version is cut, and the fold folds the
	// commits the tab would need to merge.
	s.Stop()
	s.Start()
	ada := people.Seeded[0]
	if err := s.do(wiki.SetDocument{By: tabs.By{Actor: ada, Op: "sd1", At: time.Now()}, Page: page, Doc: tracker.PlainDoc("Someone else wrote this."), Message: "Rewritten"}); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		f := &wiki.Fold{At: time.Now().Add(wiki.FoldAfter + time.Hour)}
		if err := s.do(f); err != nil {
			t.Fatal(err)
		}
		if f.Deleted == 0 {
			break
		}
	}
	s.Stop() // and a deploy: the commits are in no cache either
	s.Start()
	offline(t, p, false)
	var st editorState
	defer func() {
		if t.Failed() {
			p.Eval(t, readEditor, &st)
			t.Logf("editor %+v; server %q; errors %v", st, headText(t, s, page), p.Errors())
		}
	}()
	p.Wait(t, `document.getElementById('editor').editor?.stopped === 'gap'`, 30*time.Second)
	p.Eval(t, readEditor, &st)
	if !st.CopyShown || !strings.Contains(st.Copy, "before while away") {
		t.Fatalf("MP-05.6: no copy beside the page: %+v", st)
	}
	if st.Status != "Not saved" || st.Saved != "Not saved" {
		t.Errorf("the status: page %q, editor %q", st.Status, st.Saved)
	}
	if strings.Contains(headText(t, s, page), "while away") {
		t.Error("the text that could not merge reached the page")
	}
	// Loaded again: the page as it is, and the copy still beside it.
	p.Eval(t, `(document.querySelector('.wk-stopped:not([style*="none"]) button').click(), true)`, nil)
	p.WaitLoad(t)
	editorReady(t, p)
	p.Wait(t, `!document.getElementById('wk-copy').hidden`, 5*time.Second)
	p.Eval(t, readEditor, &st)
	if st.State != "ok" || !strings.Contains(st.Text, "Someone else wrote this.") || !strings.Contains(st.Copy, "before while away") {
		t.Errorf("loaded again: %+v", st)
	}
	p.Eval(t, `([...document.querySelectorAll('#wk-copy button')].find(b => b.textContent.includes('Dismiss')).click(), true)`, nil)
	p.Wait(t, `document.getElementById('wk-copy').hidden`, 2*time.Second)
}

// MP-14.1, MP-14.2: a blocked guest's commit is refused, and the editor
// says why at the editor, "Changes from this browser are paused.", stops
// sending and keeps the text, marked as not saved, in the browser only.
func TestBlockedEditor(t *testing.T) {
	s := newRealServer(t)
	page, edit := s.wikiPage(t)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1000, 700)
	p.Navigate(t, edit)
	editorReady(t, p)
	typeText(t, p, "fine ")
	waitFor(t, 10*time.Second, func() bool { return headText(t, s, page) == "fine " })
	var me string
	p.Eval(t, `document.head.innerHTML.match(/data-who="(g-[a-z2-7]{20})"/)[1]`, &me)
	mo := people.Person{ID: "a-0192d000-0000-7000-8000-00000000000m", Kind: people.Account, Role: people.Moderator, Name: "Mo"}
	if err := s.do(s.site.Demo().Block(tabs.By{Actor: mo, Op: "b1", At: time.Now()}, me, "spam")); err != nil {
		t.Fatal(err)
	}
	typeText(t, p, "blocked")
	var st editorState
	defer func() {
		if t.Failed() {
			p.Eval(t, readEditor, &st)
			t.Logf("editor %+v; server %q; errors %v", st, headText(t, s, page), p.Errors())
		}
	}()
	p.Wait(t, `document.getElementById('editor').editor?.stopped === 'readonly'`, 10*time.Second)
	p.Eval(t, readEditor, &st)
	if st.Notice != "Changes from this browser are paused." {
		t.Errorf("the editor says %q", st.Notice)
	}
	var above string
	p.Eval(t, `[...document.querySelectorAll('.wk-stopped')].find(d => d.style.display !== 'none')?.textContent.replace(/\s+/g, ' ').trim() || ''`, &above)
	if !strings.Contains(above, "Changes from this browser are paused. Your text stays in this tab only.") {
		t.Errorf("above the editor: %q", above)
	}
	if !strings.HasPrefix(st.Text, "fine b") || st.Saved != "Not saved" || st.Status != "Not saved" {
		t.Errorf("the text or its status: %+v", st)
	}
	time.Sleep(time.Second)
	if got := headText(t, s, page); got != "fine " {
		t.Errorf("the server holds %q", got)
	}
}
