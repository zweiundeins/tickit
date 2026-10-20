package web_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/web"
	"github.com/mbolli/tickit/internal/web/webtest"
)

// NFR-10: a login answers in the same time, within 50 ms, whether or not
// the email has an account: interleaved tries of an unknown email and of
// a known one with a wrong password, compared by mean and by median.
func TestLoginTiming(t *testing.T) {
	if testing.Short() {
		t.Skip("200 argon2id hashes")
	}
	e := webtest.StartWith(t, webtest.Options{Configure: web.Unlimited})
	e.Account(t, "ada@example.com", "correct horse battery", people.NoRole)
	try := func(email string, i int) time.Duration {
		c := webtest.Client(t)
		c.Transport = fromAddr{c.Transport, fmt.Sprintf("10.%d.%d.1", i/250, i%250)}
		start := time.Now()
		code, body := webtest.Post(t, c, e.URL+"/cmd/login", fmt.Sprintf(`{"email":%q,"password":"wrong horse %d"}`, email, i))
		took := time.Since(start)
		if code != http.StatusOK || !strings.Contains(body, people.MsgWrongLogin) {
			t.Fatalf("%s: %d %s", email, code, body)
		}
		return took
	}
	const n = 100
	var known, unknown []time.Duration
	for i := range n {
		// A new address each time: the lockout of NFR-09.2 stays out of it.
		known = append(known, try("ada@example.com", i))
		unknown = append(unknown, try(fmt.Sprintf("nobody%d@example.com", i), i))
	}
	mk, mu := mean(known), mean(unknown)
	dk, du := median(known), median(unknown)
	t.Logf("NFR-10 over %d tries each: wrong password mean %v median %v; unknown email mean %v median %v", n, mk, dk, mu, du)
	if d := (mk - mu).Abs(); d > 50*time.Millisecond {
		t.Errorf("the means differ by %v", d)
	}
	if d := (dk - du).Abs(); d > 50*time.Millisecond {
		t.Errorf("the medians differ by %v", d)
	}
}

func mean(d []time.Duration) time.Duration {
	var sum time.Duration
	for _, x := range d {
		sum += x
	}
	return sum / time.Duration(len(d))
}

func median(d []time.Duration) time.Duration {
	s := slices.Clone(d)
	slices.Sort(s)
	return s[len(s)/2]
}
