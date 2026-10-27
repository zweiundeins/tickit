package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// patchSignals is a signal patch as the stream would bring it.
func patchSignals(json string) string {
	return `document.dispatchEvent(new CustomEvent('datastar-fetch', {detail: {type: 'datastar-patch-signals', el: document.documentElement, argsRaw: {signals: ` + jsQuote(json) + `}}}))`
}

// reopen opens the tab's stream again, as sse.Reconnect has the page do.
const reopen = `(s => s.replaceWith(s.cloneNode(true)))(document.getElementById('stream'))`

// DM-04.2, MP-09.3: an op ends on the stream, never on a timer. A rate
// limit's notice ends it with the server's words and the input stays; an
// op whose outcome the stream does not bring when it connects again ends
// in the page's words; a page a former build rendered offers a reload; a
// server that stops says so.
func TestOpsEnd(t *testing.T) {
	e := webtest.Start(t)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1024, 768)
	p.Navigate(t, e.URL+"/")
	p.Wait(t, `document.querySelector('#demo-notice') && document.getElementById('reconnecting').hidden && typeof tkOps === 'object'`, 5*time.Second)

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
	outcome := `document.getElementById('rename-outcome').textContent`

	// The session's burst of changes, used up from the page.
	p.Eval(t, `(async () => {
		const tab = document.body.getAttribute('data-signals').match(/_tab: "([^"]+)"/)[1]
		for (let i = 0; i < 25; i++) await fetch('/cmd/people/rename', {method: 'POST', headers: {'Datastar-Request': 'true', 'Content-Type': 'application/json', 'Tab-Id': tab, 'Op-Id': 'burst' + i}, body: JSON.stringify({tab, op: 'burst' + i, name: 'Burst ' + i})})
		return true
	})()`, nil)
	rename("Ada Limited")
	p.Wait(t, outcome+` === 'Too many changes at once. Wait a moment.' && !document.getElementById('rename-name').readOnly`, 5*time.Second)
	var v string
	if p.Eval(t, `document.getElementById('rename-name').value`, &v); v != "Ada Limited" {
		t.Errorf("the input after a rate limit: %q", v)
	}

	// An op whose request ended and whose outcome no frame brings.
	p.Eval(t, `(tkOps.send('lost1', Promise.resolve()), `+patchSignals(`{"_rename":{"op":"lost1","msg":""}}`)+`, true)`, nil)
	p.Eval(t, reopen+`, true`, nil)
	p.Wait(t, outcome+` === 'This did not go through. Try again.'`, 5*time.Second)

	// A page of another build.
	p.Eval(t, patchSignals(`{"_build":"old"}`)+`, true`, nil)
	p.Eval(t, reopen+`, true`, nil)
	p.Wait(t, `!document.getElementById('updated').hidden && document.querySelector('#updated p').textContent.startsWith('tickit has a new version.')`, 5*time.Second)

	e.StopStreams()
	p.Wait(t, `!document.getElementById('reconnecting').hidden && document.querySelector('#reconnecting p').textContent === 'Updating, back in a moment.'`, 5*time.Second)
	for _, err := range p.Errors() {
		if !strings.HasPrefix(err, "error network:") { // the 429s of the burst, the 503s of the stopped hub
			t.Errorf("the page reported: %q", err)
		}
	}
}
