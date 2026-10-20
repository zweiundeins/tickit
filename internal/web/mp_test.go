package web_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// MP-11.4: the renders of each view are visible, by kind in the metrics
// (tickit_view_renders_total{kind}) and by key on the admin statistics,
// where keys that hold a tab id show only as a hash.
func TestViewRenders(t *testing.T) {
	e := webtest.Start(t)
	things(e)
	e.Account(t, "ada@example.com", "correct horse battery", people.Admin)
	admin := webtest.Client(t)
	e.Login(t, admin, "ada@example.com", "correct horse battery")
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	events := webtest.Listen(t, c, e.URL+"/stream?tab=renders1&v=thing:7", nil)
	webtest.Next(t, events, `<p id="thing-7">`)
	for range 3 {
		e.Do(t, writer.Func(func(tx *writer.Tx) error { tx.Touch("thing"); return nil }))
		time.Sleep(30 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)

	reg := prometheus.NewRegistry()
	reg.MustRegister(e.Server.Collectors()...)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]float64{}
	for _, mf := range mfs {
		if mf.GetName() != "tickit_view_renders_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "kind" {
					kinds[l.GetValue()] = m.GetCounter().GetValue()
				}
			}
		}
	}
	if kinds["thing"] < 3 || kinds["tab"] < 1 || kinds["presence"] < 1 {
		t.Errorf("tickit_view_renders_total by kind: %v", kinds)
	}

	code, page := webtest.Get(t, admin, e.URL+"/admin/stats")
	if code != http.StatusOK || !strings.Contains(page, "Renders per view") || !strings.Contains(page, "<code>thing:7</code>") {
		t.Fatalf("the statistics: %d\n%s", code, page)
	}
	if strings.Contains(page, "renders1") || !strings.Contains(page, "<code>tab:#") {
		t.Errorf("the statistics show a tab id, or no hashed tab key")
	}
}
