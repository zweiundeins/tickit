package web_test

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/mbolli/tickit/internal/web/webtest"
)

// TestDesign renders /design in every theme and style: it answers 200,
// carries no style attribute (NFR-13: styles come from the stylesheets),
// declares lang="en" (NFR-51), and its import map carries the nonce of the
// page's CSP.
func TestDesign(t *testing.T) {
	base := startWithCSP(t)
	for _, q := range []string{"", "?theme=light&style=smooth", "?theme=system&density=compact"} {
		res, err := http.Get(base + "/design" + q)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		page := string(b)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("/design%s: %d\n%.2000s", q, res.StatusCode, page)
		}
		markup := strip(page)
		if m := styleAttr.FindString(markup); m != "" {
			t.Errorf("/design%s has a style attribute: %q", q, m)
		}
		if !strings.Contains(page, `<html lang="en"`) {
			t.Errorf("/design%s: no lang=\"en\"", q)
		}
		csp := res.Header.Get("Content-Security-Policy")
		nonce := regexp.MustCompile(`'nonce-([^']+)'`).FindStringSubmatch(csp)
		if nonce == nil || !strings.Contains(page, `<script type="importmap" nonce="`+nonce[1]+`">`) {
			t.Errorf("/design%s: the import map lacks the CSP's nonce (CSP %q)", q, csp)
		}
		for _, s := range regexp.MustCompile(`<script(?:\s[^>]*)?>`).FindAllString(page, -1) {
			if !strings.Contains(s, ` src="/static/`) && !strings.Contains(s, `nonce="`) {
				t.Errorf("/design%s: an inline script without the nonce: %s", q, s)
			}
		}
		for _, want := range []string{"/static/css/tickit.", "/static/vendor/datastar-rocket.", "/static/vendor/starbase/dropdown/dropdown."} {
			if !strings.Contains(page, want) {
				t.Errorf("/design%s does not load %s", q, want)
			}
		}
	}
	// The bundle is served, with the fonts at their hashed names.
	res, err := http.Get(base + "/design")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	css := regexp.MustCompile(`/static/css/tickit\.[0-9a-f]+\.css`).FindString(string(b))
	res, err = http.Get(base + css)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !regexp.MustCompile(`url\("?/static/fonts/jetbrains-mono\.[0-9a-f]+\.woff2"?\)`).Match(body) {
		t.Errorf("the bundle: %d, font URLs not rewritten", res.StatusCode)
	}
}

var styleAttr = regexp.MustCompile(`<[a-zA-Z][^>]*\sstyle=`)

// strip leaves out the contents of <script> and <pre>, where text that
// looks like an attribute is not one.
func strip(html string) string {
	for _, tag := range []string{"script", "pre"} {
		for {
			i := strings.Index(html, "<"+tag)
			if i < 0 {
				break
			}
			j := strings.Index(html[i:], "</"+tag+">")
			if j < 0 {
				break
			}
			html = html[:i] + html[i+j+len(tag)+3:]
		}
	}
	return html
}

// startWithCSP serves tickit behind its whole edge, the default CSP included,
// as cmd/tickit does.
func startWithCSP(t *testing.T) string {
	t.Helper()
	return webtest.Start(t).URL
}
