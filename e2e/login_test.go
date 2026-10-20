package e2e

import (
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/browsertest"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/web/webtest"
)

// FR-122, FR-123: staff log in on the login page, land where they asked
// to, and log out as a new guest.
func TestLogin(t *testing.T) {
	e := webtest.Start(t)
	e.Account(t, "mod@example.com", "correct horse battery", people.Moderator)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1024, 768)
	p.Navigate(t, e.URL+"/login?next=/privacy")
	login := func(password string) {
		t.Helper()
		p.Eval(t, `(() => {
			for (const [id, v] of [['login-email', 'mod@example.com'], ['login-password', `+jsQuote(password)+`]]) {
				const input = document.getElementById(id)
				input.value = v
				input.dispatchEvent(new Event('input', {bubbles: true}))
			}
			document.querySelector('.tk-login button[type=submit]').click()
			return true
		})()`, nil)
	}
	login("wrong horse battery")
	p.Wait(t, `document.getElementById('login-problem').textContent === 'Email or password is wrong.' && document.getElementById('login-problem').dataset.kind === 'refused'`, 5*time.Second)
	login("correct horse battery")
	p.Wait(t, `location.pathname === '/privacy' && document.getElementById('logout') !== null`, 5*time.Second)
	p.Wait(t, `document.getElementById('tk-me-label').textContent === 'mod'`, 5*time.Second)
	p.Eval(t, `(document.getElementById('tk-me').showPopover(), document.getElementById('logout').click(), true)`, nil)
	p.Wait(t, `location.pathname === '/' && document.getElementById('logout') === null && document.body.textContent.includes('You are a guest')`, 5*time.Second)
	if errs := p.Errors(); len(errs) > 0 {
		t.Errorf("the page reported: %q", errs)
	}
}
