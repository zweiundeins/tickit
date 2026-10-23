package e2e

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// MP-10.1, NFR-42: a stream that cannot connect shows "Reconnecting" at
// once, and the stream that comes back hides it; while the server stops,
// the notice says "Updating, back in a moment" instead.
func TestReconnectingNotice(t *testing.T) {
	e := webtest.Start(t)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1024, 768)
	p.Navigate(t, e.URL+"/")
	notice := `document.getElementById('reconnecting')`
	text := notice + `.querySelector('p').textContent`
	p.Wait(t, notice+`.hidden && typeof tkOps === 'object'`, 5*time.Second)

	// Every request for the stream fails while blocked.
	paused := make(chan string, 64)
	p.OnEvent("Fetch.requestPaused", func(params json.RawMessage) {
		var ev struct{ RequestID string }
		json.Unmarshal(params, &ev)
		paused <- ev.RequestID
	})
	p.Call(t, "Fetch.enable", map[string]any{"patterns": []map[string]any{{"urlPattern": "*/stream?*"}}})
	done := make(chan struct{})
	go func() {
		for {
			select {
			case id := <-paused: // failed from here: OnEvent's callback must not call the page
				p.Call(t, "Fetch.failRequest", map[string]any{"requestId": id, "errorReason": "ConnectionRefused"})
			case <-done:
				return
			}
		}
	}()
	p.Eval(t, reopen+`, true`, nil)
	p.Wait(t, `!`+notice+`.hidden && `+text+` === 'Reconnecting…'`, 3*time.Second)
	time.Sleep(time.Second) // a heartbeat or a retry must not hide it while the stream is down
	p.Wait(t, `!`+notice+`.hidden`, time.Second)

	close(done)
	p.Call(t, "Fetch.disable", nil)
	p.Wait(t, notice+`.hidden`, 15*time.Second) // Datastar's retry backs off to at most 10 s

	e.StopStreams()
	p.Wait(t, `!`+notice+`.hidden && `+text+` === 'Updating, back in a moment.'`, 5*time.Second)
}
