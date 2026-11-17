package web_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// TestPagesLoadTheirComponents follows the links of the seeded site as an
// admin: no page renders a Starbase component that its Page.Components
// does not load (sb.Collect sees what the wrappers render).
func TestPagesLoadTheirComponents(t *testing.T) {
	e := seededEnv(t)
	e.Account(t, "root@example.com", "a long password 1", people.Admin)
	c := webtest.Client(t)
	e.Login(t, c, "root@example.com", "a long password 1")
	queue := []string{"/", "/admin", "/projects", "/spaces", "/issues", "/dashboards", "/filters", "/search?q=login"}
	seen := map[string]bool{}
	for _, u := range queue {
		seen[u] = true
	}
	href := regexp.MustCompile(`href="(/[^"#]*)"`)
	for i := 0; i < len(queue) && i < 600; i++ {
		code, body := webtest.Get(t, c, e.URL+queue[i])
		if code != http.StatusOK {
			continue
		}
		for _, m := range href.FindAllStringSubmatch(body, -1) {
			l := strings.ReplaceAll(m[1], "&amp;", "&")
			if strings.HasPrefix(l, "/static/") || strings.HasPrefix(l, "/img/") || strings.HasPrefix(l, "/design") || strings.Contains(l, ".csv") || seen[l] {
				continue
			}
			seen[l] = true
			queue = append(queue, l)
		}
	}
	if len(seen) < 100 {
		t.Errorf("only %d pages found", len(seen))
	}
	for _, line := range strings.Split(e.Logs(), "\n") {
		if strings.Contains(line, "does not load") {
			t.Error(line)
		}
	}
}
