package web_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/sse"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/web/webtest"
)

// counter registers the view kind count, whose frame shows a number that
// bump raises, and returns bump.
func counter(t *testing.T, e *webtest.Env) (bump func()) {
	var n atomic.Int64
	e.Server.Views().Register("count", func(context.Context, sqlite.Querier, people.Person, string) (live.View, error) {
		return live.View{Key: "count", Topics: []string{"count"}, Render: func(context.Context, *sql.Tx) (any, error) {
			return sse.PatchElements(fmt.Appendf(nil, `<p id="count">count %d</p>`, n.Load())), nil
		}}, nil
	})
	return func() {
		n.Add(1)
		e.Do(t, writer.Func(func(tx *writer.Tx) error { tx.Touch("count"); return nil }))
	}
}

var ackToken = regexp.MustCompile(`/ack\?tab=tab1&amp;s=([0-9a-z]+)&amp;n=1&amp;rtt=`)

// A page that falls behind posts /pace (newest-frame-behind on <body>),
// which counts the report and paces its stream: the next write ends with
// an element that posts /ack after the paint, and the stream holds the
// write after it until /ack, with the stream's token, the write's number
// and the round trip, frees it.
func TestPacing(t *testing.T) {
	e := webtest.Start(t)
	bump := counter(t, e)
	c := webtest.Client(t)
	_, page := webtest.Get(t, c, e.URL+"/")
	if want := `<body data-newest-frame data-on:newest-frame-behind="@post(&#39;/pace?tab=&#39; + $_tab, {requestCancellation: &#39;disabled&#39;})"`; !regexp.MustCompile(regexp.QuoteMeta(want)).MatchString(page) {
		t.Fatalf("the page does not post /pace when it falls behind:\n%.2000s", page)
	}
	events := webtest.Listen(t, c, e.URL+"/stream?tab=tab1&v=count:", nil)
	webtest.Next(t, events, "count 0")
	if code, _ := webtest.Post(t, c, e.URL+"/pace?tab=tab1", ""); code != http.StatusNoContent {
		t.Fatalf("pace: %d", code)
	}
	if st := e.Hub.Stats(); st.Paces != 1 || st.Behind != 1 || st.BehindStreams != 1 {
		t.Fatalf("Paces %d, Behind %d, BehindStreams %d, want 1, 1 and 1", st.Paces, st.Behind, st.BehindStreams)
	}
	bump()
	webtest.Next(t, events, "count 1")
	m := ackToken.FindStringSubmatch(webtest.Next(t, events, `id="ack-1"`))
	if m == nil {
		t.Fatal("the paced write does not end with its acknowledgement")
	}
	bump()
	webtest.Silent(t, events, "count 2", 300*time.Millisecond)
	if code, _ := webtest.Post(t, c, e.URL+"/ack?tab=tab1&s="+m[1]+"&n=1&rtt=12", ""); code != http.StatusNoContent {
		t.Fatalf("ack: %d", code)
	}
	webtest.Next(t, events, "count 2")
	webtest.Next(t, events, `id="ack-2"`)
	if st := e.Hub.Stats(); st.Acks != 1 {
		t.Errorf("Acks %d, want 1", st.Acks)
	}
}

// /pace and /ack reach only the streams of the request's person: another
// person's posts for the same tab id answer 204 and reach nothing.
func TestPacingIsThePersonsOwn(t *testing.T) {
	e := webtest.Start(t)
	bump := counter(t, e)
	ada, bob := webtest.Client(t), webtest.Client(t)
	webtest.Get(t, ada, e.URL+"/")
	webtest.Get(t, bob, e.URL+"/")
	events := webtest.Listen(t, ada, e.URL+"/stream?tab=tab1&v=count:", nil)
	webtest.Next(t, events, "count 0")
	if code, _ := webtest.Post(t, bob, e.URL+"/pace?tab=tab1", ""); code != http.StatusNoContent {
		t.Fatalf("pace from another person: %d", code)
	}
	if st := e.Hub.Stats(); st.Paces != 0 || st.Behind != 0 {
		t.Fatalf("another person's /pace: Paces %d, Behind %d", st.Paces, st.Behind)
	}
	webtest.Post(t, ada, e.URL+"/pace?tab=tab1", "")
	bump()
	m := ackToken.FindStringSubmatch(webtest.Next(t, events, `id="ack-1"`))
	if m == nil {
		t.Fatal("no acknowledgement")
	}
	ack := e.URL + "/ack?tab=tab1&s=" + m[1] + "&n=1&rtt=12"
	if code, _ := webtest.Post(t, bob, ack, ""); code != http.StatusNoContent || e.Hub.Stats().Acks != 0 {
		t.Fatalf("another person's /ack: %d, Acks %d", code, e.Hub.Stats().Acks)
	}
	webtest.Post(t, ada, ack, "")
	if st := e.Hub.Stats(); st.Acks != 1 {
		t.Errorf("the person's own /ack: Acks %d, want 1", st.Acks)
	}
}

// /pace and /ack refuse what no page sends (NFR-16), and the edge refuses
// them without Datastar's header (NFR-03).
func TestPacingInput(t *testing.T) {
	e := webtest.Start(t)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	for _, url := range []string{
		"/pace?tab=no%20tab", "/pace",
		"/ack?tab=tab1&n=1", "/ack?tab=tab1&s=X&n=1", "/ack?tab=tab1&s=1&n=0", "/ack?tab=tab1&s=1&n=x",
		"/ack?tab=tab1&s=1&n=1&rtt=x", "/ack?s=1&n=1",
	} {
		if code, _ := webtest.Post(t, c, e.URL+url, ""); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", url, code)
		}
	}
	for _, url := range []string{"/pace?tab=tab1", "/ack?tab=tab1&s=1&n=1"} {
		req, _ := http.NewRequest("POST", e.URL+url, nil)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("%s without Datastar-Request: %d, want 403", url, res.StatusCode)
		}
	}
}
