package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
)

// toolbar runs a script with the editor's host as ed, its shadow root as
// ui, and append(text), which adds a paragraph with text at the end and
// puts the caret at its end.
func toolbar(t *testing.T, p *browsertest.Page, script string) {
	t.Helper()
	p.Eval(t, `(() => {
		const ed = document.getElementById('editor'), ui = ed.shadowRoot, v = ed.view
		const append = (text) => {
			const S = v.state.schema
			const tr = v.state.tr.insert(v.state.doc.content.size, S.nodes.paragraph.create(null, text ? S.text(text) : null))
			tr.setSelection(v.state.selection.constructor.near(tr.doc.resolve(tr.doc.content.size - 1)))
			v.focus()
			v.dispatch(tr)
		}
		const pick = (id, value) => {
			const s = ui.getElementById(id)
			s.value = value
			s.dispatchEvent(new Event('change', { bubbles: true }))
		}
		`+script+`
		return true
	})()`, nil)
}

// FR-82.1: headings of levels 4 to 6, a panel, a text colour, a status and
// a table of contents made with the editor's toolbar render for a reader
// of the page, styled in their colours, and pass axe (TICKIT_AXE).
func TestEditorNewNodes(t *testing.T) {
	s := newRealServer(t)
	page, edit := s.wikiPage(t)
	br := browsertest.Start(t)
	editor, reader := br.NewPage(t, 1000, 800), br.NewPage(t, 1000, 800)
	editor.Navigate(t, edit)
	editorReady(t, editor)
	reader.Navigate(t, s.URL+"/pages/"+page)
	reader.Wait(t, `document.getElementById('page-body')`, 10*time.Second)
	typeText(t, editor, "Intro")
	toolbar(t, editor, `pick('heading-more', '4')`)
	toolbar(t, editor, `append('Careful now'); pick('panel', 'warning')`)
	toolbar(t, editor, `append('Purple prose'); v.dispatch(v.state.tr.setSelection(v.state.selection.constructor.create(v.state.doc, v.state.doc.content.size - 13, v.state.doc.content.size - 7))); pick('color', 'purple')`)
	toolbar(t, editor, `append('State: '); ui.querySelector('button[value=status]').click(); const d = ui.getElementById('status-dialog'); d.elements.text.value = 'Done'; d.elements.color.value = 'green'; d.requestSubmit(d.querySelector('[value=apply]'))`)
	toolbar(t, editor, `append(''); ui.querySelector('button[value=toc]').click()`)
	toolbar(t, editor, `append('Sixth'); pick('heading-more', '6')`)
	toolbar(t, editor, `append('Fifth'); pick('heading-more', '5')`)
	defer func() {
		if t.Failed() {
			var html string
			reader.Eval(t, `document.getElementById('page-body').innerHTML`, &html)
			t.Logf("reader: %s\nerrors %v %v", html, editor.Errors(), reader.Errors())
		}
	}()
	reader.Wait(t, `(() => {
		const b = document.getElementById('page-body')
		const toc = [...b.querySelectorAll('nav.doc-toc[aria-label=Contents] a')].map((a) => a.getAttribute('href')).join()
		return b.querySelector('h5#h-intro')?.textContent === 'Intro' &&
			b.querySelector('div.doc-panel[data-panel=warning][role=note][aria-label=Warning] > p')?.textContent === 'Careful now' &&
			b.querySelector('span.doc-color[data-color=purple]')?.textContent === 'Purple' &&
			b.querySelector('span.tk-lozenge.doc-status[data-color=green]')?.textContent === 'Done' &&
			b.querySelector('p[role=heading][aria-level="7"]#h-sixth') && b.querySelector('h6#h-fifth') &&
			toc === '#h-intro,#h-sixth,#h-fifth'
	})()`, 10*time.Second)
	// The palette's tokens reach them: the colour, the panel's wash and icon.
	var styled struct{ Text, Para, Wash, Icon string }
	reader.Eval(t, `(() => {
		const b = document.getElementById('page-body'), panel = b.querySelector('.doc-panel')
		return { text: getComputedStyle(b.querySelector('.doc-color')).color, para: getComputedStyle(b.querySelector('.doc-color').parentElement).color,
			wash: getComputedStyle(panel).backgroundColor, icon: getComputedStyle(panel, '::before').maskImage }
	})()`, &styled)
	if styled.Text == styled.Para || styled.Wash == "rgba(0, 0, 0, 0)" || styled.Icon == "none" || styled.Icon == "" {
		t.Errorf("not styled: %+v", styled)
	}
	// A toc's link goes to its heading.
	reader.Eval(t, `(document.querySelector('nav.doc-toc a[href="#h-fifth"]').click(), true)`, nil)
	reader.Wait(t, `location.hash === '#h-fifth'`, 5*time.Second)
	if src := os.Getenv("TICKIT_AXE"); src != "" {
		axe, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		for name, p := range map[string]*browsertest.Page{"reader": reader, "editor": editor} {
			p.Eval(t, string(axe)+";true", nil)
			var res []struct{ ID, Impact, Help string }
			p.Eval(t, `axe.run(document, {resultTypes: ['violations']}).then(r => r.violations.map(v => ({id: v.id, impact: v.impact, help: v.help})))`, &res)
			for _, v := range res {
				msg := fmt.Sprintf("%s: %s (%s): %s", name, v.ID, v.Impact, v.Help)
				if v.Impact == "serious" || v.Impact == "critical" {
					t.Error(msg)
				} else {
					t.Log(msg)
				}
			}
		}
	}
}
