package web_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/zweiundeins/tickit/internal/web"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// MP-11.4, NFR-76.2: the metrics name the views watched and their renders
// by kind, and count the streams opened and closed.
func TestViewAndStreamMetrics(t *testing.T) {
	e := webtest.Start(t)
	seedProbes(t, e)
	reg := prometheus.NewRegistry()
	reg.MustRegister(e.Server.Collectors()...)
	gather := func() map[string]map[string]float64 {
		t.Helper()
		mfs, err := reg.Gather()
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]map[string]float64{}
		for _, mf := range mfs {
			byKind := map[string]float64{}
			for _, m := range mf.GetMetric() {
				kind := ""
				for _, l := range m.GetLabel() {
					if l.GetName() == "kind" {
						kind = l.GetValue()
					}
				}
				byKind[kind] = m.GetCounter().GetValue() + m.GetGauge().GetValue()
			}
			out[mf.GetName()] = byKind
		}
		return out
	}
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/WEB-1")
	events, stop := webtest.Open(t, c, e.URL+"/stream?tab=tab1&v=issue:"+web.ProbeIssue, nil)
	webtest.Next(t, events, `"_outcomes"`)
	got := gather()
	if got["tickit_streams_opened_total"][""] != 1 || got["tickit_streams_closed_total"][""] != 0 {
		t.Errorf("streams opened %v, closed %v", got["tickit_streams_opened_total"], got["tickit_streams_closed_total"])
	}
	if got["tickit_views_watched"]["issue"] != 1 || got["tickit_view_renders"]["issue"] < 1 {
		t.Errorf("views %v, renders %v", got["tickit_views_watched"], got["tickit_view_renders"])
	}
	stop()
	deadline := time.Now().Add(5 * time.Second)
	for gather()["tickit_streams_closed_total"][""] != 1 {
		if time.Now().After(deadline) {
			t.Fatal("the closed stream is not counted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// A stream refused before the hub takes it is neither.
	res, err := c.Get(e.URL + "/stream?tab=bad%20tab")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest || gather()["tickit_streams_opened_total"][""] != 1 {
		t.Errorf("a refused stream: %d", res.StatusCode)
	}
}

// A stream writes its status once: the hub flushes the headers before
// its first frame, which must not write them again (MP-11.4).
func TestStreamWritesItsStatusOnce(t *testing.T) {
	e := webtest.Start(t)
	seedProbes(t, e)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/WEB-1")
	events, stop := webtest.Open(t, c, e.URL+"/stream?tab=tab1&alive=1&v=issue:"+web.ProbeIssue, nil)
	defer stop()
	webtest.Next(t, events, `"_outcomes"`)
	for _, u := range []string{"/alive?tab=tab1", "/pace?tab=tab1"} {
		if code, _ := webtest.Post(t, c, e.URL+u, ""); code != http.StatusNoContent {
			t.Fatalf("%s: %d", u, code)
		}
	}
	if code, _ := webtest.Post(t, c, e.URL+"/cmd/input", `{"tab":"tab1","op":"i1"}`); code != http.StatusNoContent {
		t.Fatalf("/cmd/input: %d", code)
	}
	if strings.Contains(e.Logs(), "superfluous") {
		t.Fatalf("a response wrote its status twice:\n%s", e.Logs())
	}
}
