package web_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/views"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// MP-02, DM-11: the home page and a dashboard have a presence row, one for
// everyone on them whatever projects each reads, and the first-visit hint
// promises a second person on this page only where the page has a row.
func TestHomeAndDashboardPresence(t *testing.T) {
	e := webtest.Start(t)
	adminID := e.Account(t, "root@example.com", "correct horse battery", people.Admin)
	admin := webtest.Client(t)
	e.Login(t, admin, "root@example.com", "correct horse battery")
	found, err := people.Lookup(t.Context(), e.DB.R, adminID)
	if err != nil {
		t.Fatal(err)
	}
	// A closed project the guest does not read: their feeds' and grids' keys differ.
	e.Do(t, tracker.CreateProject{By: tabs.By{Actor: found[adminID], Op: "p1", At: time.Now()}, ID: tracker.NewID(), Key: "SEC", Name: "Secret", Open: false})

	ada := webtest.Client(t)
	_, page := webtest.Get(t, ada, e.URL+"/")
	adaName := personName.FindStringSubmatch(page)[1]
	_, page = webtest.Get(t, admin, e.URL+"/")
	adminName := personName.FindStringSubmatch(page)[1]

	here := "Open this page in a private window to see a second person arrive."
	elsewhere := "Open a board, an issue or a page in a private window to see a second person arrive."
	home := views.PresenceID(views.HomePresence)
	if !strings.Contains(page, `<ul id="`+home+`"`) || !strings.Contains(page, here) || strings.Contains(page, elsewhere) {
		t.Errorf("the home page has no presence row or the wrong hint")
	}
	for _, path := range []string{"/dashboards", "/projects", "/issues?sort=key"} {
		if _, doc := webtest.Get(t, ada, e.URL+path); !strings.Contains(doc, elsewhere) || strings.Contains(doc, here) {
			t.Errorf("%s promises a second person on a page without a row", path)
		}
	}

	both := func(path, row string) {
		t.Helper()
		first := webtest.Listen(t, ada, e.URL+path, nil)
		webtest.Next(t, first, `<ul id="`+row+`"`)
		webtest.Listen(t, admin, e.URL+strings.Replace(path, "tab=ada", "tab=root", 1), nil)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			ev := webtest.Next(t, first, `<ul id="`+row+`"`)
			if strings.Contains(ev, adaName) && strings.Contains(ev, adminName) {
				return
			}
		}
		t.Errorf("%s: the row never shows both people", row)
	}
	both("/stream?tab=adahome&v=home:,activity-home:0", home)

	code, body := act(t, e, admin, "/cmd/dashboards/create", "root0", map[string]any{"name": "Release"})
	m := dashGoto.FindStringSubmatch(body)
	if code != http.StatusOK || m == nil {
		t.Fatalf("create: %d %s", code, body)
	}
	dash := views.PresenceID(views.DashboardPresence(m[1]))
	_, doc := webtest.Get(t, ada, e.URL+"/dashboards/"+m[1])
	if !strings.Contains(doc, `<ul id="`+dash+`"`) || !strings.Contains(doc, here) {
		t.Errorf("the dashboard has no presence row or the wrong hint")
	}
	both("/stream?tab=adadash&v=dashboard:"+m[1], dash)
}
