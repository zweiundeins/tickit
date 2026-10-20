package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/browsertest"
	"github.com/mbolli/tickit/internal/wiki"
)

// TestWikiShots writes screenshots of the wiki to WIKI_SHOTS: the space's
// home, a page, the editor with two people's carets, the history and a
// comparison, dark and light, 1440 and 375 pixels wide.
func TestWikiShots(t *testing.T) {
	dir := os.Getenv("WIKI_SHOTS")
	if dir == "" {
		t.Skip("WIKI_SHOTS unset")
	}
	os.MkdirAll(dir, 0o755)
	s := newRealServer(t)
	s.seedAll()
	var deploy, home string
	s.db.View(context.Background(), func(tx *sql.Tx) error {
		if err := tx.QueryRow(`SELECT id FROM pages WHERE title = 'Deploy to production'`).Scan(&deploy); err != nil {
			return err
		}
		return tx.QueryRow(`SELECT home_id FROM spaces WHERE key = 'ENG'`).Scan(&home)
	})
	b := browsertest.Start(t)
	other := b.NewPage(t, 1000, 800)
	other.Navigate(t, s.URL+wiki.EditURL("ENG", deploy))
	editorReady(t, other)
	other.Eval(t, `(() => { const ed = document.getElementById('editor'); const s = ed.view.state; ed.view.dispatch(s.tr.setSelection(s.selection.constructor.create(s.doc, 120, 160))); return true })()`, nil)
	for _, theme := range []string{"dark", "light"} {
		for _, w := range []int{1440, 375} {
			p := b.NewPage(t, w, 900)
			p.Navigate(t, s.URL+"/spaces")
			p.Eval(t, fmt.Sprintf(`fetch('/cmd/prefs', {method: 'POST', headers: {'Content-Type': 'application/json', 'Datastar-Request': 'true'}, body: JSON.stringify({tab: 'shots', op: 'th%d', key: 'theme', value: %q})}).then(r => r.status)`, w, theme), nil)
			shot := func(name, url string, full bool, ready string) {
				p.Navigate(t, s.URL+url)
				if ready != "" {
					p.Wait(t, ready, 10*time.Second)
				}
				time.Sleep(400 * time.Millisecond)
				p.Screenshot(t, filepath.Join(dir, fmt.Sprintf("wiki-%s-%s-%d.png", name, theme, w)), full)
			}
			shot("space", "/spaces/ENG", false, `document.querySelector('.tk-presence__item')`)
			shot("page", "/pages/"+deploy, true, `document.querySelector('.tk-presence__item[data-editing]')`)
			shot("history", wiki.HistoryURL("ENG", deploy), false, "")
			shot("diff", wiki.CompareURL("ENG", deploy, 2, 3), false, "")
			p.Navigate(t, s.URL+wiki.EditURL("ENG", deploy))
			editorReady(t, p)
			p.Eval(t, `(() => { const ed = document.getElementById('editor'); const s = ed.view.state; ed.view.dispatch(s.tr.setSelection(s.selection.constructor.create(s.doc, 40))); return true })()`, nil)
			p.Wait(t, `document.querySelector('.tk-caret')`, 10*time.Second)
			time.Sleep(500 * time.Millisecond)
			p.Screenshot(t, filepath.Join(dir, fmt.Sprintf("wiki-editor-%s-%d.png", theme, w)), false)
			p.Close(t)
		}
	}
}
