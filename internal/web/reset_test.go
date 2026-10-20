package web_test

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/demo"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

var cardRe = regexp.MustCompile(`id="c-WEB-\d+"`)

// DM-03.4: while the demo resets, an open board shows what it showed or
// the seeded state, never the emptied or half-seeded board in between,
// and a page loaded meanwhile says that the demo resets and loads again.
func TestResetShowsNoHalfSeed(t *testing.T) {
	e := seededEnv(t)
	g := webtest.Client(t)
	me := guest(t, e, g)
	code, page := webtest.Get(t, g, e.URL+"/projects/WEB/board")
	if code != http.StatusOK {
		t.Fatalf("the board: %d", code)
	}
	seeded := len(cardRe.FindAllString(page, -1))
	if seeded < 10 {
		t.Fatalf("the seeded board has %d cards", seeded)
	}
	guestIssue(t, e, me, "Gone after the reset")
	arg := regexp.MustCompile(`data-arg="([^"]+)"`).FindStringSubmatch(page)[1]
	events := webtest.Listen(t, g, e.URL+"/stream?tab=tabR&v=board:"+arg, nil)
	if n := len(cardRe.FindAllString(webtest.Next(t, events, `id="board-lanes"`), -1)); n != seeded+1 {
		t.Fatalf("the board before the reset has %d cards, not %d", n, seeded+1)
	}

	release := e.Server.Demo().Hold()
	code, page = webtest.Get(t, g, e.URL+"/projects/WEB/board")
	if code != http.StatusServiceUnavailable || !strings.Contains(page, demo.MsgResetting) || !strings.Contains(page, `http-equiv="refresh"`) || cardRe.MatchString(page) {
		t.Errorf("a page during a reset: %d, %d cards", code, len(cardRe.FindAllString(page, -1)))
	}
	release()

	done := make(chan error, 1)
	go func() { done <- e.Server.Demo().Reset(context.Background(), "") }()
	var last int
	timeout := time.After(10 * time.Second)
	for finished := false; !finished || last != seeded; {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			finished, done = true, nil
		case ev := <-events:
			if !strings.Contains(ev, `id="board-lanes"`) {
				continue
			}
			last = len(cardRe.FindAllString(ev, -1))
			if last != seeded && last != seeded+1 {
				t.Fatalf("a frame during the reset shows %d cards: neither the board before (%d) nor the seeded one (%d)", last, seeded+1, seeded)
			}
		case <-timeout:
			t.Fatalf("no frame of the seeded board after the reset (last %d cards)", last)
		}
	}
}
