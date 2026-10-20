package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/browsertest"
	"github.com/mbolli/tickit/internal/tracker/trackertest"
	"github.com/mbolli/tickit/internal/web/webtest"
)

// TestBoardShots writes screenshots of the projects page, the board in
// both themes at 1440 and 375 pixels, and the backlog, into the directory
// TICKIT_SHOTS names; without it, it skips.
func TestBoardShots(t *testing.T) {
	dir := os.Getenv("TICKIT_SHOTS")
	if dir == "" {
		t.Skip("TICKIT_SHOTS names no directory")
	}
	e := webtest.Start(t)
	trackertest.Board(t, e.W, "WEB", 24)
	trackertest.Board(t, e.W, "OPS", 6)
	b := browsertest.Start(t)
	shot := func(name, url, theme string, w, h int, full bool) {
		t.Helper()
		p := b.NewPage(t, w, h)
		defer p.Close(t)
		p.Navigate(t, e.URL+url)
		p.Wait(t, `document.querySelector('#presence-boardhead-`+``+`') !== undefined`, time.Second)
		time.Sleep(600 * time.Millisecond)
		p.Eval(t, `(() => { document.documentElement.dataset.tkTheme = '`+theme+`'; document.querySelector('meta[name=color-scheme]').content = '`+theme+`'; return true })()`, nil)
		time.Sleep(200 * time.Millisecond)
		p.Screenshot(t, filepath.Join(dir, "board-"+name+".png"), full)
		if errs := p.Errors(); len(errs) > 0 {
			t.Errorf("%s: %v", name, errs)
		}
	}
	shot("projects", "/projects", "dark", 1440, 900, false)
	shot("dark-1440", "/projects/WEB/board", "dark", 1440, 1000, false)
	shot("light-1440", "/projects/WEB/board", "light", 1440, 1000, false)
	shot("dark-375", "/projects/WEB/board", "dark", 375, 812, false)
	shot("light-375", "/projects/WEB/board", "light", 375, 812, false)
	shot("backlog", "/projects/WEB/backlog", "dark", 1440, 1000, true)
	shot("backlog-light", "/projects/WEB/backlog", "light", 1440, 1000, true)
}
