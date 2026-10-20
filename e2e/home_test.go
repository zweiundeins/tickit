// Package e2e drives tickit in real browsers: what Go tests of the bytes
// on the wire cannot show, that the page works. The tests skip without
// Chromium; CHROME names it (/usr/bin/chromium on the development
// machine), LITH_CHROME_NO_SANDBOX=1 runs it where it has no sandbox.
package e2e

import (
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/browsertest"
	"github.com/mbolli/tickit/internal/web/webtest"
)

// The home page opens its stream, and an action's outcome settles its
// control: a refusal shows its reason, an accepted change its effect
// (MP-09, FR-121). The page reports no error under the real CSP.
func TestHome(t *testing.T) {
	e := webtest.Start(t)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1024, 768)
	p.Navigate(t, e.URL+"/")

	deadline := time.Now().Add(5 * time.Second)
	for e.Hub.Stats().Streams != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("no stream after 5s: %+v", e.Hub.Stats())
		}
		time.Sleep(20 * time.Millisecond)
	}
	p.Wait(t, `document.querySelector('#demo-notice') && document.getElementById('reconnecting').hidden`, 5*time.Second)

	rename := func(name string) {
		t.Helper()
		p.Eval(t, `(() => {
			document.getElementById('tk-me').showPopover()
			const input = document.getElementById('rename-name')
			input.value = `+jsQuote(name)+`
			input.dispatchEvent(new Event('input', {bubbles: true}))
			document.querySelector('.tk-rename button[type=submit]').click()
			return true
		})()`, nil)
	}
	rename("A")
	p.Wait(t, `document.getElementById('rename-outcome').textContent === 'A name has 2 to 30 characters.' && document.getElementById('rename-outcome').dataset.kind === 'refused'`, 5*time.Second)
	p.Wait(t, `!document.getElementById('rename-name').readOnly`, 5*time.Second)
	rename("Ada Browser")
	p.Wait(t, `document.getElementById('rename-outcome').textContent.startsWith('Saved.')`, 5*time.Second)
	p.Wait(t, `document.getElementById('tk-me-label').textContent === 'Ada Browser' && document.getElementById('tk-me-name').textContent === 'Ada Browser'`, 5*time.Second)
	if errs := p.Errors(); len(errs) > 0 {
		t.Errorf("the page reported: %q", errs)
	}
}

func jsQuote(s string) string {
	b := []byte{'\''}
	for _, r := range s {
		if r == '\'' || r == '\\' {
			b = append(b, '\\')
		}
		b = append(b, string(r)...)
	}
	return string(append(b, '\''))
}
