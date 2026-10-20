package e2e

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/browsertest"
	"github.com/mbolli/tickit/internal/web/webtest"
)

// The display menu sets the session's theme, style and density: <html>
// follows when the stream brings them, in every tab of the session, and
// the next page has them from its first byte (NFR-31: no cookie of their
// own). Dismissing the first-visit hint (DM-11) and the demo notice
// (DM-10.1) lasts too.
func TestDisplayPrefs(t *testing.T) {
	e := webtest.Start(t)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1024, 768)
	p.Navigate(t, e.URL+"/")
	// A second tab of the same session: the first one's cookie.
	var jar struct{ Cookies []map[string]any }
	if err := json.Unmarshal(p.Call(t, "Network.getCookies", map[string]any{"urls": []string{e.URL}}), &jar); err != nil || len(jar.Cookies) != 1 {
		t.Fatalf("cookies: %v %v", jar, err)
	}
	other := b.NewPage(t, 1024, 768)
	c := jar.Cookies[0]
	other.Call(t, "Network.setCookie", map[string]any{"name": c["name"], "value": c["value"], "url": e.URL, "httpOnly": true, "sameSite": c["sameSite"]})
	other.Navigate(t, e.URL+"/privacy")
	for _, page := range []*browsertest.Page{p, other} {
		page.Wait(t, `document.documentElement.dataset.tkTheme === 'system' && document.getElementById('reconnecting').hidden`, 5*time.Second)
	}
	deadline := time.Now().Add(5 * time.Second)
	for e.Hub.Stats().Streams != 2 {
		if time.Now().After(deadline) {
			t.Fatalf("no streams after 5s: %+v", e.Hub.Stats())
		}
		time.Sleep(20 * time.Millisecond)
	}

	choose := func(value string) {
		t.Helper()
		p.Eval(t, `(document.getElementById('tk-display').showPopover(), document.querySelector('#tk-display button[data-value=`+value+`]').click(), true)`, nil)
	}
	choose("light")
	p.Wait(t, `document.documentElement.dataset.tkTheme === 'light' && document.querySelector('meta[name=color-scheme]').content === 'light' && document.querySelector('#tk-display button[data-value=light]').getAttribute('aria-pressed') === 'true' && document.querySelector('#tk-display button[data-value=system]').getAttribute('aria-pressed') === 'false'`, 5*time.Second)
	choose("compact")
	p.Wait(t, `document.documentElement.dataset.tkDensity === 'compact'`, 5*time.Second)
	choose("smooth")
	p.Wait(t, `document.documentElement.dataset.tkStyle === 'smooth'`, 5*time.Second)
	other.Wait(t, `document.documentElement.dataset.tkTheme === 'light' && document.documentElement.dataset.tkStyle === 'smooth'`, 5*time.Second)

	p.Eval(t, `(document.querySelector('#first-visit button').click(), true)`, nil)
	p.Wait(t, `document.getElementById('first-visit').hidden`, 5*time.Second)
	p.Eval(t, `(document.querySelector('#demo-notice button').click(), true)`, nil)
	p.Wait(t, `document.getElementById('demo-notice').hidden`, 5*time.Second)

	p.Navigate(t, e.URL+"/privacy")
	var got struct {
		Theme, Style, Density string
		Hint, Demo            bool
	}
	p.Eval(t, `(() => {
		const d = document.documentElement.dataset
		return {theme: d.tkTheme, style: d.tkStyle, density: d.tkDensity, hint: !!document.getElementById('first-visit'), demo: document.getElementById('demo-notice').hidden}
	})()`, &got)
	if got.Theme != "light" || got.Style != "smooth" || got.Density != "compact" || got.Hint || !got.Demo {
		t.Errorf("the next page: %+v", got)
	}
	for _, page := range []*browsertest.Page{p, other} {
		if errs := page.Errors(); len(errs) > 0 {
			t.Errorf("the page reported: %q", errs)
		}
	}
}
