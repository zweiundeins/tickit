package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// marked is the text the page shows highlighted, in the editor or the
// reader.
const marked = `[...document.querySelectorAll('.pm-mount mark.tk-ic, #page-body mark.tk-ic')].map((m) => m.textContent).join('')`

// selectWord selects word in the first text of the page's document whose
// node holds it, as a person's mouse would.
func selectWord(t *testing.T, p *browsertest.Page, root, word string) {
	t.Helper()
	var ok bool
	p.Eval(t, `((root, word) => {
		const w = document.createTreeWalker(document.querySelector(root), NodeFilter.SHOW_TEXT)
		for (let n = w.nextNode(); n; n = w.nextNode()) {
			const i = n.data.indexOf(word)
			if (i >= 0) { document.getSelection().setBaseAndExtent(n, i, n, i + word.length); return true }
		}
		return false
	})(`+jsString(root)+`, `+jsString(word)+`)`, &ok)
	if !ok {
		t.Fatalf("no %q in %s", word, root)
	}
}

// commentOn selects word and comments on it through the Comment button
// beside the selection (FR-91.1).
func commentOn(t *testing.T, p *browsertest.Page, root, word, body string) {
	t.Helper()
	selectWord(t, p, root, word)
	p.Wait(t, `!document.getElementById('wk-ic-add').closest('.wk-sel').hidden`, 5*time.Second)
	p.Eval(t, `(document.getElementById('wk-ic-add').click(), true)`, nil)
	p.Wait(t, `getComputedStyle(document.getElementById('wk-ic-form')).display !== 'none' && document.activeElement?.id === 'ic-body'`, 5*time.Second)
	p.Eval(t, `((t) => { t.value = `+jsString(body)+`; t.dispatchEvent(new Event('input', {bubbles: true})); t.form.requestSubmit(); return true })(document.getElementById('ic-body'))`, nil)
}

func jsString(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

// FR-91.1, FR-91.2: A comments on a word in the editor; B types before
// it, and the highlight stays on the word for both; B deletes the word,
// and the comment lists as detached for both.
func TestInlineCommentsCoEditing(t *testing.T) {
	s := newRealServer(t)
	page, edit := s.wikiPage(t)
	if err := s.do(wiki.SetDocument{By: tabs.By{Actor: people.Seeded[0], Op: "d1", At: time.Now()}, Page: page, Doc: tracker.PlainDoc("The quick brown fox"), Message: "Text"}); err != nil {
		t.Fatal(err)
	}
	b := browsertest.Start(t)
	a, bb := b.NewPage(t, 1200, 800), b.NewPage(t, 1200, 800)
	for _, p := range []*browsertest.Page{a, bb} {
		p.Navigate(t, edit)
		editorReady(t, p)
	}
	defer func() {
		if t.Failed() {
			for _, p := range []*browsertest.Page{a, bb} {
				var st struct{ Text, Marked, Comments, Panel string }
				p.Eval(t, `({text: document.getElementById('editor').text(), marked: `+marked+`, comments: document.getElementById('editor').getAttribute('comments'), panel: document.getElementById('page-inline').textContent})`, &st)
				t.Logf("%+v errors %v", st, p.Errors())
			}
		}
	}()
	commentOn(t, a, ".pm-mount", "brown", "Which brown?")
	for _, p := range []*browsertest.Page{a, bb} {
		p.Wait(t, marked+` === 'brown'`, 10*time.Second)
		p.Wait(t, `document.querySelector('#page-inline [data-state="open"]')?.textContent.includes('Which brown?')`, 10*time.Second)
	}
	// The new comment opens beside its text: marked as chosen.
	a.Wait(t, `[...document.head.querySelectorAll('style')].some((s) => s.textContent.includes('mark.tk-ic[data-c~='))`, 5*time.Second)

	// B types before the word.
	bb.Eval(t, `((ed) => { const S = ed.view.state.selection.constructor; ed.view.dispatch(ed.view.state.tr.setSelection(S.create(ed.view.state.doc, 1))); ed.focus(); return true })(document.getElementById('editor'))`, nil)
	typeText(t, bb, "Big ")
	waitFor(t, 10*time.Second, func() bool { return headText(t, s, page) == "Big The quick brown fox" })
	for _, p := range []*browsertest.Page{a, bb} {
		p.Wait(t, `document.getElementById('editor').text() === 'Big The quick brown fox'`, 10*time.Second)
		p.Wait(t, marked+` === 'brown'`, 10*time.Second)
	}
	// And types at the word's edges: the highlight stays on the word.
	bb.Eval(t, `((ed) => { const S = ed.view.state.selection.constructor, d = ed.view.state.doc; let at = 0; d.descendants((n, pos) => { if (n.isText && n.text.includes('brown')) at = pos + n.text.indexOf('brown') }); ed.view.dispatch(ed.view.state.tr.setSelection(S.create(d, at + 5))); return true })(document.getElementById('editor'))`, nil)
	typeText(t, bb, "ish")
	for _, p := range []*browsertest.Page{a, bb} {
		p.Wait(t, `document.getElementById('editor').text() === 'Big The quick brownish fox'`, 10*time.Second)
		p.Wait(t, marked+` === 'brown'`, 10*time.Second)
	}
	bb.Eval(t, `((ed) => { const S = ed.view.state.selection.constructor, d = ed.view.state.doc; let at = 0; d.descendants((n, pos) => { if (n.isText && n.text.includes('brown')) at = pos + n.text.indexOf('brown') }); ed.view.dispatch(ed.view.state.tr.setSelection(S.create(d, at))); return true })(document.getElementById('editor'))`, nil)
	typeText(t, bb, "dark")
	for _, p := range []*browsertest.Page{a, bb} {
		p.Wait(t, `document.getElementById('editor').text() === 'Big The quick darkbrownish fox'`, 10*time.Second)
		p.Wait(t, marked+` === 'brown'`, 10*time.Second)
	}

	// B deletes the word: the comment is detached for both.
	bb.Eval(t, `((ed) => { const S = ed.view.state.selection.constructor, d = ed.view.state.doc; let at = 0; d.descendants((n, pos) => { if (n.isText && n.text.includes('brownish')) at = pos + n.text.indexOf('brownish') }); ed.view.dispatch(ed.view.state.tr.setSelection(S.create(d, at, at + 8))); ed.view.dispatch(ed.view.state.tr.deleteSelection()); return true })(document.getElementById('editor'))`, nil)
	for _, p := range []*browsertest.Page{a, bb} {
		p.Wait(t, `document.getElementById('editor').text() === 'Big The quick dark fox'`, 10*time.Second)
		p.Wait(t, `document.querySelector('#page-inline [data-state="detached"]')?.textContent.includes('Which brown?')`, 10*time.Second)
		p.Wait(t, marked+` === ''`, 10*time.Second)
	}
	for _, p := range []*browsertest.Page{a, bb} {
		if errs := p.Errors(); len(errs) > 0 {
			t.Errorf("the page reported: %q", errs)
		}
	}
}

// FR-91.1, FR-91.3: a reader comments on a word inside a list, past bold
// text, a chip and an issue list (FR-104); another reader sees the highlight and the comment;
// resolving hides the highlight for both and lists it as resolved. At 320
// px the comments sit below the text without a sideways scroll.
func TestInlineCommentsReading(t *testing.T) {
	s := newRealServer(t)
	page, _ := s.wikiPage(t)
	body := `{"type":"doc","content":[{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"Plan"}]},` +
		`{"type":"paragraph","content":[{"type":"text","text":"Ship "},{"type":"text","marks":[{"type":"strong"}],"text":"soon"},{"type":"text","text":" with "},{"type":"issue_chip","attrs":{"key":"NOPE-1"}},{"type":"text","text":" done"}]},` +
		`{"type":"issue_list","attrs":{"filter":"status=todo","limit":5}},` +
		`{"type":"bullet_list","content":[{"type":"list_item","content":[{"type":"paragraph","content":[{"type":"text","text":"first step"}]}]},{"type":"list_item","content":[{"type":"paragraph","content":[{"type":"text","text":"second careful step"}]}]}]}]}`
	if err := s.do(wiki.SetDocument{By: tabs.By{Actor: people.Seeded[0], Op: "d1", At: time.Now()}, Page: page, Doc: []byte(body), Message: "Text"}); err != nil {
		t.Fatal(err)
	}
	b := browsertest.Start(t)
	r1, r2 := b.NewPage(t, 1200, 800), b.NewPage(t, 320, 700)
	for _, p := range []*browsertest.Page{r1, r2} {
		p.Navigate(t, s.URL+"/pages/"+page)
		p.Wait(t, `document.getElementById('reconnecting')?.hidden && document.querySelector('#page-body li')`, 10*time.Second)
	}
	commentOn(t, r1, "#page-body", "careful", "Why careful?")
	for _, p := range []*browsertest.Page{r1, r2} {
		p.Wait(t, marked+` === 'careful'`, 10*time.Second)
		p.Wait(t, `document.querySelector('#page-inline [data-state="open"]')?.textContent.includes('Why careful?')`, 10*time.Second)
	}
	var quote string
	if err := s.db.R.QueryRowContext(context.Background(), `SELECT quote FROM page_inline WHERE page_id = ?`, page).Scan(&quote); err != nil || quote != "careful" {
		t.Errorf("the quote %q: %v", quote, err)
	}
	// Narrow: below the text, no sideways scroll.
	var w struct {
		Scroll, Inner int
		Below         bool
	}
	r2.Eval(t, `({scroll: document.documentElement.scrollWidth, inner: innerWidth, below: document.querySelector('.wk-ic-panel').getBoundingClientRect().top >= document.getElementById('page-body').getBoundingClientRect().bottom})`, &w)
	if w.Scroll > w.Inner || !w.Below {
		t.Errorf("NFR-46: at 320 px %+v", w)
	}
	r2.Eval(t, `(document.querySelector('#page-inline [data-state="open"] button[data-resolved]').click(), true)`, nil)
	for _, p := range []*browsertest.Page{r1, r2} {
		p.Wait(t, marked+` === ''`, 10*time.Second)
		p.Wait(t, `document.querySelector('#page-inline .wk-ic-resolved summary')?.textContent.includes('Resolved (1)')`, 10*time.Second)
	}
	for _, p := range []*browsertest.Page{r1, r2} {
		if errs := p.Errors(); len(errs) > 0 {
			t.Errorf("the page reported: %q", errs)
		}
	}
}
