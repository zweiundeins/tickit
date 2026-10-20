package web_test

import (
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// NFR-31, requirements 4.1 rule 6: POST /cmd/prefs stores a display
// preference per session, its outcome and the new _prefs reach the tab's
// stream, the next page draws it on <html> from the first byte, no cookie
// carries it, and another session keeps the defaults.
func TestPrefs(t *testing.T) {
	e := webtest.Start(t)
	c := webtest.Client(t)
	_, page := webtest.Get(t, c, e.URL+"/")
	if !strings.Contains(page, `<html lang="en" data-tk-density="comfortable" data-tk-style="pixel" data-tk-theme="system">`) ||
		!strings.Contains(page, `<meta name="color-scheme" content="light dark">`) || !strings.Contains(page, `id="first-visit"`) {
		t.Fatalf("the defaults:\n%.1500s", page)
	}
	events := webtest.Listen(t, c, e.URL+"/stream?tab=tab1", nil)
	webtest.Next(t, events, `"_prefs":{"theme":"system"`)
	before := cookie(c, e.URL)

	for i, kv := range [][2]string{{"theme", "light"}, {"density", "compact"}, {"style", "smooth"}, {"hint", "hidden"}, {"demo", "hidden"}} {
		body := `{"tab":"tab1","op":"op` + string(rune('a'+i)) + `","key":"` + kv[0] + `","value":"` + kv[1] + `"}`
		if code, res := webtest.Post(t, c, e.URL+"/cmd/prefs", body); code != http.StatusNoContent {
			t.Fatalf("%s=%s: %d %s", kv[0], kv[1], code, res)
		}
	}
	webtest.Next(t, events, `"_prefs":{"theme":"light","style":"smooth","density":"compact","hideDemo":true,"hideHint":true}`)
	webtest.Next(t, events, `"ope":{"ok":true`)
	if cookie(c, e.URL) != before || len(c.Jar.Cookies(mustURL(e.URL))) != 1 {
		t.Error("a preference set a cookie")
	}

	_, page = webtest.Get(t, c, e.URL+"/privacy")
	if !strings.Contains(page, `<html lang="en" data-tk-density="compact" data-tk-style="smooth" data-tk-theme="light">`) ||
		!strings.Contains(page, `<meta name="color-scheme" content="light">`) || strings.Contains(page, `id="first-visit"`) ||
		!regexp.MustCompile(`<aside id="demo-notice"[^>]* hidden[ >]`).MatchString(page) {
		t.Errorf("the next page:\n%.1500s", page)
	}
	if _, other := webtest.Get(t, webtest.Client(t), e.URL+"/"); !strings.Contains(other, `data-tk-theme="system"`) || !strings.Contains(other, `id="first-visit"`) {
		t.Error("another session got the preferences")
	}

	for _, body := range []string{
		`{"tab":"tab1","op":"x1","key":"theme","value":"pink"}`,
		`{"tab":"tab1","op":"x2","key":"font","value":"comic"}`,
		`{"tab":"tab1","op":"x3","key":"theme"}`,
		`{"tab":"tab1","op":"x4","key":"theme","value":7}`,
	} {
		if code, _ := webtest.Post(t, c, e.URL+"/cmd/prefs", body); code != http.StatusBadRequest {
			t.Errorf("%s: %d", body, code)
		}
	}
}

// NFR-46, NFR-51, docs/design-system.md: every page declares lang="en",
// lets people zoom, links the one stylesheet bundle and the favicon, and
// carries no style attribute.
func TestEveryPage(t *testing.T) {
	e := webtest.Start(t)
	c := webtest.Client(t)
	pages := 0
	for _, p := range e.Server.Probes() {
		if strings.HasPrefix(p, "/stream") || strings.HasPrefix(p, "/static/") || p == "/robots.txt" {
			continue
		}
		res, err := c.Get(e.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") {
			continue
		}
		pages++
		page := string(b)
		viewport := regexp.MustCompile(`<meta name="viewport" content="([^"]*)">`).FindStringSubmatch(page)
		switch {
		case !strings.Contains(page, `<html lang="en"`):
			t.Errorf("%s: no lang=\"en\"", p)
		case viewport == nil || strings.Contains(viewport[1], "maximum-scale") || strings.Contains(viewport[1], "user-scalable"):
			t.Errorf("%s: the viewport %v", p, viewport)
		case strings.Count(page, `rel="stylesheet"`) != strings.Count(page, `<link rel="stylesheet" href="/static/css/`) ||
			!regexp.MustCompile(`<link rel="stylesheet" href="/static/css/tickit\.[0-9a-f]+\.css">`).MatchString(page):
			t.Errorf("%s: not the bundle alone", p)
		case !regexp.MustCompile(`<link rel="icon" type="image/svg\+xml" href="/static/art/mark\.[0-9a-f]+\.svg">`).MatchString(page):
			t.Errorf("%s: no favicon", p)
		case styleAttr.MatchString(strip(page)):
			t.Errorf("%s: a style attribute: %s", p, styleAttr.FindString(strip(page)))
		}
	}
	if pages < 5 {
		t.Errorf("%d pages checked", pages)
	}
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
