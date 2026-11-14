package e2e

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
)

// setDoc replaces the editor's document with blocks (JSON) as one commit.
func setDoc(t *testing.T, p *browsertest.Page, blocks string) {
	t.Helper()
	p.Eval(t, `(() => {
		const v = document.getElementById('editor').view
		const doc = v.state.schema.nodeFromJSON({type: 'doc', content: `+blocks+`})
		v.dispatch(v.state.tr.replaceWith(0, v.state.doc.content.size, doc.content))
		return true
	})()`, nil)
}

func longPage(headings int) string {
	var b []string
	for i := 1; i <= headings; i++ {
		b = append(b, fmt.Sprintf(`{type: 'heading', attrs: {level: %d}, content: [{type: 'text', text: 'Section %d'}]}`, 1+(i+1)%2, i))
		for k := 0; k < 8; k++ {
			b = append(b, `{type: 'paragraph', content: [{type: 'text', text: '`+strings.Repeat("Words to read on the way down the page. ", 12)+`'}]}`)
		}
	}
	return "[" + strings.Join(b, ",") + "]"
}

// The outline of a long page (sb-toc): one "On this page" landmark beside
// the text, its current section marked as the reader scrolls, a link that
// jumps to its heading; none for a short page; no sideways scroll at 320 px.
func TestPageOutline(t *testing.T) {
	s := newRealServer(t)
	page, edit := s.wikiPage(t)
	br := browsertest.Start(t)
	editor, reader := br.NewPage(t, 1000, 800), br.NewPage(t, 1700, 900)
	editor.Navigate(t, edit)
	editorReady(t, editor)
	setDoc(t, editor, longPage(6))
	reader.Navigate(t, s.URL+"/pages/"+page)
	reader.Wait(t, `customElements.get('sb-toc') && document.querySelector('#page-outline sb-toc')?.shadowRoot?.querySelectorAll('a[href^="#h-p-"]').length === 6`, 10*time.Second)
	var landmarks int
	reader.Eval(t, `[...document.querySelectorAll('nav, [role=navigation]'), ...[...document.querySelectorAll('sb-toc')].flatMap((e) => [...e.shadowRoot.querySelectorAll('nav')])].filter((n) => n.getAttribute('aria-label') === 'On this page').length`, &landmarks)
	if landmarks != 1 {
		t.Errorf("%d landmarks named On this page", landmarks)
	}
	// Scrolled to the fourth section, the outline marks it.
	reader.Eval(t, `(document.querySelector('#page-body h3[id$="-section-4"], #page-body h2[id$="-section-4"]').scrollIntoView(), window.scrollBy(0, 40), true)`, nil)
	reader.Wait(t, `document.querySelector('#page-outline sb-toc').shadowRoot.querySelector('[aria-current=location]')?.textContent.trim() === 'Section 4'`, 5*time.Second)
	// A link jumps to its heading.
	reader.Eval(t, `(document.querySelector('#page-outline sb-toc').shadowRoot.querySelector('a[href$="-section-6"]').click(), true)`, nil)
	reader.Wait(t, `location.hash.endsWith('-section-6') && Math.abs(document.querySelector('#page-body [id$="-section-6"]').getBoundingClientRect().top) < 200`, 5*time.Second)
	if src := os.Getenv("TICKIT_AXE"); src != "" {
		axe, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		reader.Eval(t, string(axe)+";true", nil)
		var res []struct{ ID, Impact string }
		reader.Eval(t, `axe.run(document, {resultTypes: ['violations']}).then(r => r.violations.map(v => ({id: v.id, impact: v.impact})))`, &res)
		for _, v := range res {
			if v.Impact == "serious" || v.Impact == "critical" {
				t.Errorf("axe: %s (%s)", v.ID, v.Impact)
			}
		}
	}
	// At 320 px the outline is a bar, and nothing scrolls sideways.
	narrow := br.NewPage(t, 320, 700)
	narrow.Navigate(t, s.URL+"/pages/"+page)
	narrow.Wait(t, `document.querySelector('#page-outline sb-toc')?.shadowRoot?.querySelector('[part~=bar], button')`, 10*time.Second)
	var wide bool
	narrow.Eval(t, `document.documentElement.scrollWidth > document.documentElement.clientWidth`, &wide)
	if wide {
		t.Error("the page scrolls sideways at 320 px")
	}
	// Down to three headings, the outline goes.
	setDoc(t, editor, longPage(3))
	reader.Wait(t, `document.getElementById('page-outline') && !document.querySelector('#page-outline sb-toc') && document.querySelectorAll('#page-body [id$="-section-3"]').length === 1`, 10*time.Second)
}
