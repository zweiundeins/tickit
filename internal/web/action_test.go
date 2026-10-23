package web_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// NFR-16: malformed input answers 400, never 500.
func TestMalformedActions(t *testing.T) {
	e := webtest.Start(t)
	c := webtest.Client(t)
	for _, body := range []string{
		``,
		`not json`,
		`{}`,
		`{"tab":"tab1"}`,
		`{"op":"op1"}`,
		`{"tab":"tab 1","op":"op1","name":"Ada"}`,
		`{"tab":"tab1","op":"` + strings.Repeat("o", 65) + `","name":"Ada"}`,
		`{"tab":"tab1","op":"op1","name":17}`,
		`{"tab":"tab1","op":"op1","name":"` + strings.Repeat("x", 70<<10) + `"}`,
		`[1,2,3]`,
	} {
		if code, _ := webtest.Post(t, c, e.URL+"/cmd/people/rename", body); code != http.StatusBadRequest {
			t.Errorf("%.40q: %d", body, code)
		}
	}
	for _, path := range []string{"/cmd/nope", "/cmd/people/nope", "/cmd/"} {
		if code, _ := webtest.Post(t, c, e.URL+path, `{}`); code >= 500 || code < 400 {
			t.Errorf("%s: %d", path, code)
		}
	}
}

// DM-04: actions are limited per session and per address, the address
// getting four times the session's allowance. DM-04.2: a limited action
// answers 429 alone, and its tab hears why in a notice that ends the op.
func TestRateLimits(t *testing.T) {
	e := webtest.Start(t)
	ada, bob := webtest.Client(t), webtest.Client(t)
	webtest.Get(t, ada, e.URL+"/")
	stream := webtest.Listen(t, ada, e.URL+"/stream?tab=tab1&v=", nil)
	webtest.Next(t, stream, `"_outcomes"`)
	limited := ""
	n := 0
	for i := range 30 {
		code, body := webtest.Post(t, ada, e.URL+"/cmd/people/rename", fmt.Sprintf(`{"tab":"tab1","op":"op%d","name":"Ada %d"}`, i, i))
		switch code {
		case http.StatusNoContent:
		case http.StatusTooManyRequests:
			n++
			if limited == "" {
				limited = fmt.Sprintf("op%d", i)
			}
			if body != "" {
				t.Errorf("429 says %q", body)
			}
		default:
			t.Fatalf("rename %d: %d %s", i, code, body)
		}
	}
	if n < 5 || n > 10 {
		t.Errorf("%d of 30 quick changes limited, with a burst of 20", n)
	}
	if limited != "" {
		webtest.Next(t, stream, `"`+limited+`":{"ok":false,"msg":"Too many changes at once. Wait a moment.","field":"","failed":true}`)
	}
	if code, _ := webtest.Post(t, bob, e.URL+"/cmd/people/rename", `{"tab":"tab1","op":"op1","name":"Bob"}`); code != http.StatusNoContent {
		t.Errorf("another session at the same address: %d", code)
	}
}

// NFR-03: a cross-site POST answers 403 and changes nothing, as does a
// POST to /cmd/ without Datastar's header.
func TestCrossSite(t *testing.T) {
	e := webtest.Start(t)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	before := e.Fingerprint(t)
	for name, header := range map[string]http.Header{
		"cross-site":      {"Sec-Fetch-Site": {"cross-site"}, "Datastar-Request": {"true"}},
		"same-site":       {"Sec-Fetch-Site": {"same-site"}, "Datastar-Request": {"true"}},
		"another origin":  {"Origin": {"https://evil.example"}, "Datastar-Request": {"true"}},
		"no origin":       {"Datastar-Request": {"true"}},
		"not by Datastar": {"Sec-Fetch-Site": {"same-origin"}},
	} {
		for _, path := range []string{"/cmd/people/rename", "/cmd/login", "/cmd/logout", "/cmd/watch"} {
			req, _ := http.NewRequest("POST", e.URL+path, strings.NewReader(`{"tab":"tab1","op":"op1","name":"Mallory","email":"a@b.c","password":"x"}`))
			req.Header = header.Clone()
			req.Header.Set("Content-Type", "application/json")
			res, err := c.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != http.StatusForbidden {
				t.Errorf("%s POST %s: %d", name, path, res.StatusCode)
			}
		}
	}
	if e.Fingerprint(t) != before {
		t.Error("a refused request changed the database")
	}
}

// NFR-19, DM-09: security headers and noindex on every response; robots.txt
// lets search engines see the home page only.
func TestHeaders(t *testing.T) {
	e := webtest.StartWith(t, webtest.Options{Secure: true})
	for _, path := range []string{"/", "/privacy", "/no/such/page", "/robots.txt"} {
		res, err := http.Get(e.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		h := res.Header
		csp := h.Get("Content-Security-Policy")
		if h.Get("X-Robots-Tag") != "noindex, nofollow" || !strings.Contains(csp, "frame-ancestors 'none'") || !strings.Contains(csp, "'nonce-") ||
			h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Referrer-Policy") == "" || h.Get("Cross-Origin-Opener-Policy") != "same-origin" ||
			!strings.HasPrefix(h.Get("Strict-Transport-Security"), "max-age=") {
			t.Errorf("%s: %v", path, h)
		}
		if path == "/" && !strings.HasPrefix(h.Get("Set-Cookie"), "__Host-sid=") {
			t.Errorf("the cookie: %s", h.Get("Set-Cookie"))
		}
		if path == "/robots.txt" && string(body) != "User-agent: *\nAllow: /$\nDisallow: /\n" {
			t.Errorf("robots.txt: %q", body)
		}
		if path == "/no/such/page" && (res.StatusCode != http.StatusNotFound || !strings.Contains(string(body), ">Not found</h1>")) {
			t.Errorf("an unknown page: %d", res.StatusCode)
		}
	}
	// NFR-14: no debug route in the binary.
	for _, path := range []string{"/debug/pprof/", "/debug/vars", "/reload", "/hotreload"} {
		if code, _ := webtest.Get(t, http.DefaultClient, e.URL+path); code != http.StatusNotFound {
			t.Errorf("%s: %d", path, code)
		}
	}
}

func cookie(c *http.Client, base string) string {
	u, _ := url.Parse(base)
	for _, k := range c.Jar.Cookies(u) {
		if k.Name == "sid" {
			return k.Value
		}
	}
	return ""
}

// withCookie is a client with a copy of a session cookie.
func withCookie(t *testing.T, base, sid string) *http.Client {
	c := webtest.Client(t)
	u, _ := url.Parse(base)
	c.Jar.SetCookies(u, []*http.Cookie{{Name: "sid", Value: sid, Path: "/"}})
	return c
}

// FR-122, FR-123, FR-137, NFR-04, NFR-10, NFR-11: login renews the
// session; an unknown email and a wrong password hear the same; logout
// ends the login session, and a cookie copied before is a guest's.
func TestLogin(t *testing.T) {
	e := webtest.Start(t)
	e.Account(t, "mod@example.com", "correct horse battery", people.Moderator)
	c := webtest.Client(t)
	_, page := webtest.Get(t, c, e.URL+"/login?next=/privacy")
	if !strings.Contains(page, `>Log in</h1>`) || strings.Contains(page, ">Moderation<") || !strings.Contains(page, "Log in (staff)") {
		t.Fatalf("the login page:\n%s", page)
	}
	guest := cookie(c, e.URL)

	_, wrong := webtest.Post(t, c, e.URL+"/cmd/login", `{"email":"mod@example.com","password":"wrong horse battery","next":"/"}`)
	_, unknown := webtest.Post(t, c, e.URL+"/cmd/login", `{"email":"nobody@example.com","password":"wrong horse battery","next":"/"}`)
	if !strings.Contains(wrong, `"problem":"Email or password is wrong."`) || wrong != unknown {
		t.Errorf("a wrong password: %s\nan unknown email: %s", wrong, unknown)
	}
	if cookie(c, e.URL) != guest {
		t.Error("a failed login changed the session")
	}

	code, body := webtest.Post(t, c, e.URL+"/cmd/login", `{"email":"MOD@example.com","password":"correct horse battery","next":"/privacy"}`)
	if code != http.StatusOK || !strings.Contains(body, `"_goto":"/privacy"`) {
		t.Fatalf("login: %d %s", code, body)
	}
	account := cookie(c, e.URL)
	if account == "" || account == guest {
		t.Fatal("the session was not renewed")
	}
	_, page = webtest.Get(t, c, e.URL+"/")
	if !strings.Contains(page, nameTag("mod")) || !strings.Contains(page, ">Log out<") || !strings.Contains(page, `href="/moderation"`) || strings.Contains(page, `href="/admin"`) {
		t.Errorf("logged in:\n%s", page)
	}
	if _, page := webtest.Get(t, withCookie(t, e.URL, guest), e.URL+"/"); strings.Contains(page, ">Log out<") {
		t.Error("the session before the login is logged in")
	}
	copied := withCookie(t, e.URL, account)

	code, body = webtest.Post(t, c, e.URL+"/cmd/logout", ``)
	if code != http.StatusOK || !strings.Contains(body, `"_goto":"/"`) || cookie(c, e.URL) != "" {
		t.Fatalf("logout: %d %s, cookie %q", code, body, cookie(c, e.URL))
	}
	if _, page := webtest.Get(t, copied, e.URL+"/"); strings.Contains(page, ">Log out<") || !strings.Contains(page, "You are a guest") {
		t.Error("a cookie copied before the logout is still logged in")
	}

	// A login never sends the page elsewhere.
	for next, want := range map[string]string{"//evil.example/": "/", "https://evil.example": "/", "/login": "/", "": "/", "/projects?x=1": "/projects?x=1"} {
		c := webtest.Client(t)
		_, body := webtest.Post(t, c, e.URL+"/cmd/login", fmt.Sprintf(`{"email":"mod@example.com","password":"correct horse battery","next":%q}`, next))
		if !strings.Contains(body, fmt.Sprintf(`"_goto":%q`, want)) {
			t.Errorf("next %q: %s", next, body)
		}
	}
	// NFR-11: no password and no session id in the log.
	for _, secret := range []string{"correct horse battery", "wrong horse battery", guest, account} {
		if strings.Contains(e.Logs(), secret) {
			t.Errorf("the log holds %q", secret)
		}
	}
}

// FR-137.2: the admin link shows to site admins.
func TestAdminLink(t *testing.T) {
	e := webtest.Start(t)
	e.Account(t, "root@example.com", "correct horse battery", people.Admin)
	c := webtest.Client(t)
	e.Login(t, c, "root@example.com", "correct horse battery")
	if _, page := webtest.Get(t, c, e.URL+"/"); !strings.Contains(page, `href="/admin"`) || !strings.Contains(page, `href="/moderation"`) {
		t.Errorf("an admin's menu:\n%s", page)
	}
}

// NFR-09: 30 sign-in requests a minute per address, 10 at once; 5 failures
// for one email from one address lock it there for 15 minutes.
func TestSignInLimits(t *testing.T) {
	e := webtest.Start(t)
	e.Account(t, "ada@example.com", "correct horse battery", people.NoRole)
	c := webtest.Client(t)
	for i := range 5 {
		_, body := webtest.Post(t, c, e.URL+"/cmd/login", `{"email":"ada@example.com","password":"wrong horse battery"}`)
		if !strings.Contains(body, people.MsgWrongLogin) {
			t.Fatalf("failure %d: %s", i, body)
		}
	}
	_, body := webtest.Post(t, c, e.URL+"/cmd/login", `{"email":"ada@example.com","password":"correct horse battery"}`)
	if !strings.Contains(body, people.MsgLocked) {
		t.Errorf("after 5 failures: %s", body)
	}
	limited := 0
	for range 10 {
		if code, _ := webtest.Post(t, c, e.URL+"/cmd/login", `{"email":"bob@example.com","password":"x"}`); code == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 {
		t.Error("16 sign-ins at once, none limited")
	}
}

// NFR-06: no GET route writes, whoever asks: a browser without a cookie,
// a guest, a guest who acted, an account.
func TestGetNeverWrites(t *testing.T) {
	e := webtest.Start(t)
	seedProbes(t, e) // the issues the issue routes' probes name
	e.Account(t, "ada@example.com", "correct horse battery", people.Admin)
	acted, staff := webtest.Client(t), webtest.Client(t)
	webtest.Post(t, acted, e.URL+"/cmd/people/rename", `{"tab":"tab1","op":"op1","name":"Bob"}`)
	webtest.Post(t, acted, e.URL+"/cmd/prefs", `{"tab":"tab1","op":"op1p","key":"theme","value":"light"}`) // a GET draws them
	e.Login(t, staff, "ada@example.com", "correct horse battery")
	clients := map[string]*http.Client{"no cookie": http.DefaultClient, "a guest": webtest.Client(t), "a guest who acted": acted, "an account": staff}
	for _, c := range clients {
		webtest.Get(t, c, e.URL+"/") // the cookie, before the fingerprint
	}
	if err := e.W.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := e.Fingerprint(t)
	probes := e.Server.Probes()
	if len(probes) < 8 {
		t.Fatalf("probes: %v", probes)
	}
	for name, c := range clients {
		for _, p := range probes {
			ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond) // a stream ends with it
			req, _ := http.NewRequestWithContext(ctx, "GET", e.URL+p, nil)
			res, err := c.Do(req)
			if err == nil {
				io.Copy(io.Discard, res.Body)
				res.Body.Close()
				if res.StatusCode >= 500 {
					t.Errorf("%s GET %s: %d", name, p, res.StatusCode)
				}
			}
			cancel()
		}
	}
	if err := e.W.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.Fingerprint(t) != before {
		t.Errorf("GET wrote to the database; probes %v", probes)
	}
	// The fingerprint sees a write.
	webtest.Post(t, acted, e.URL+"/cmd/people/rename", `{"tab":"tab1","op":"op2","name":"Bobby"}`)
	if e.Fingerprint(t) == before {
		t.Error("the fingerprint missed a rename")
	}
}
