package e2e

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/a-h/templ"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/views"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// MP-02.1: a presence row shows the viewer's own face first and at most 8
// faces, with "+n" for the rest, wherever the viewer is in the shared row.
func TestPresenceRow(t *testing.T) {
	e := webtest.Start(t)
	_, home := webtest.Get(t, webtest.Client(t), e.URL+"/")
	css := regexp.MustCompile(`href="(/static/css/tickit\.[0-9a-f]+\.css)"`).FindStringSubmatch(home)
	if css == nil {
		t.Fatal("no stylesheet")
	}
	var list []views.Present
	for i := range 12 {
		list = append(list, views.Present{Person: people.GuestOf(fmt.Sprint("session ", i)), Tabs: 1})
	}
	var doc bytes.Buffer
	me := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc.Reset()
		shell := views.Shell{CSS: e.URL + css[1], Me: list[me].Person}
		views.Layout(shell).Render(templ.WithChildren(context.Background(), views.Presence("row", list)), &doc)
		w.Header().Set("Content-Type", "text/html")
		w.Write(doc.Bytes())
	}))
	defer srv.Close()

	b := browsertest.Start(t)
	p := b.NewPage(t, 1024, 768)
	for _, me = range []int{2, 10} {
		p.Navigate(t, srv.URL)
		var got struct {
			Shown []string
			More  string
		}
		p.Eval(t, `(() => {
			const shown = [...document.querySelectorAll('#row > li[data-who]')].filter(li => getComputedStyle(li).display !== 'none')
			shown.sort((a, b) => a.getBoundingClientRect().left - b.getBoundingClientRect().left)
			return {shown: shown.map(li => li.dataset.who), more: document.querySelector('#row .tk-presence__more').textContent}
		})()`, &got)
		if len(got.Shown) != views.Shown || got.Shown[0] != list[me].ID || got.More != "+4" {
			t.Errorf("me the %dth: %d faces, first %s (want %s), %q", me+1, len(got.Shown), got.Shown[0], list[me].ID, got.More)
		}
	}
}
