package web_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// A feed's frame holds one window of 50 entries, whatever the feed's
// length (NFR-37); a tab moves its stream to another window with POST
// /cmd/feed/window, and that window stays live (FR-69, MP-03.1).
func TestFeedWindows(t *testing.T) {
	e := webtest.Start(t)
	e.Do(t, people.SeedPeople{At: time.Now()})
	n := 0
	by := func() tabs.By {
		n++
		return tabs.By{Actor: people.Seeded[0], Op: fmt.Sprint("f", n), At: time.Now()}
	}
	var project string
	b, err := e.W.DoValue(context.Background(), func(tx *writer.Tx) ([]byte, error) {
		return tracker.CreateProject{By: by(), Key: "FEED", Name: "Feed", Open: true}.ApplyResult(tx)
	})
	if err != nil {
		t.Fatal(err)
	}
	project = strings.Split(strings.Split(string(b), `"id":"`)[1], `"`)[0]
	create := func(summary string) {
		if _, err := e.W.DoValue(context.Background(), func(tx *writer.Tx) ([]byte, error) {
			return tracker.CreateIssue{By: by(), Project: project, Type: tracker.Task, Summary: summary}.ApplyResult(tx)
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 130 {
		create(fmt.Sprintf("Entry %03d", i+1))
	}

	c := webtest.Client(t)
	code, page := webtest.Get(t, c, e.URL+"/")
	if code != http.StatusOK || strings.Count(page, `class="tk-vfeed__item"`) != 50 || !strings.Contains(page, `href="/?from=50"`) {
		t.Fatalf("home: %d, %d entries", code, strings.Count(page, `class="tk-vfeed__item"`))
	}
	home := webtest.Listen(t, c, e.URL+"/stream?tab=tab1&v=home:,activity-home:0", nil)
	first := webtest.Next(t, home, `id="activity-home"`)
	if len(first) > 60<<10 || strings.Count(first, "<article") != 50 {
		t.Errorf("the window's frame: %d bytes, %d entries", len(first), strings.Count(first, "<article"))
	}

	if code, _ := webtest.Post(t, c, e.URL+"/cmd/feed/window", `{"tab":"tab1","views":"home:,activity-home:0","feed":"activity-home:60"}`); code != http.StatusNoContent {
		t.Fatalf("window: %d", code)
	}
	frame := webtest.Next(t, home, `offset="60"`)
	if !strings.Contains(frame, `aria-posinset="61"`) || !strings.Contains(frame, "Entry 070") || strings.Contains(frame, "Entry 080") {
		t.Errorf("the window from 60 lacks its entries: %.300s", frame)
	}
	// A new entry moves every entry of the window down by one, in the
	// window's next frame; the window from 0 is off the stream.
	create("Newest")
	frame = webtest.Next(t, home, `offset="60"`)
	if !strings.Contains(frame, "Entry 071") {
		t.Errorf("the window did not follow the new entry")
	}
	webtest.Silent(t, home, `offset="0"`, 200*time.Millisecond)

	for _, bad := range []string{"activity-home:nope", "activity-home:2000", "activity-project:" + project + ".x"} {
		if code, _ := webtest.Get(t, c, e.URL+"/stream?tab=tab2&v="+bad); code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, code)
		}
	}
	if code, _ := webtest.Post(t, c, e.URL+"/cmd/feed/window", `{"tab":"tab1","views":"","feed":"issue:x"}`); code != http.StatusBadRequest {
		t.Errorf("a window of another view: %d", code)
	}
}
