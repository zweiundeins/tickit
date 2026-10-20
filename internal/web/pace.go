package web

import (
	"html"
	"net/http"
	"strconv"
	"time"

	"github.com/zweiundeins/lith/edge"
	"github.com/zweiundeins/lith/sse"

	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/views"
)

// A page whose newest-frame.js falls behind its stream posts /pace; the
// hub then paces the stream by the page's acknowledgements (/ack) until it
// keeps up again (lith v0.3.1's template, docs/architecture.md). Both reach
// only the streams of the request's person (streamID).

// paceLimits limit /pace and /ack per client address first, then per
// session or stream: a client mints a session with every request that
// sends no cookie, so a session id alone is no limiter key.
type paceLimits struct {
	paceAddrs, paces *edge.Limiter
	ackAddrs, acks   *edge.Limiter
}

func newPaceLimits() *paceLimits {
	return &paceLimits{
		paceAddrs: edge.NewLimiter(4, 20),   // per client address: a few people's tabs
		paces:     edge.NewLimiter(1, 5),    // per session: a page reports every 3 s at most
		ackAddrs:  edge.NewLimiter(80, 160), // per client address: four paced streams
		acks:      edge.NewLimiter(20, 40),  // per stream: a paced page acknowledges ten writes a second at most
	}
}

// ackEvent is the tab's live.Spec.Ack: once /pace has paced the stream,
// each write ends with an element in the stream's element whose data-init
// posts /ack for the write after the paint that shows it, with the round
// trip of the page's last /ack or /pace. tab is a valid id (validID).
func ackEvent(tab string) func(token string, n int64) []byte {
	return func(token string, n int64) []byte {
		num := strconv.FormatInt(n, 10)
		post := "@post('/ack?tab=" + tab + "&s=" + token + "&n=" + num + "&rtt=' + " + lastRoundTrip + ", {requestCancellation: 'disabled'})"
		el := `<i id="ack-` + num + `" data-init="` + html.EscapeString("requestAnimationFrame(() => setTimeout(() => "+post+"))") + `"></i>`
		return sse.PatchElements([]byte(el), sse.Patch{Selector: "#" + views.StreamID, Mode: sse.Inner})
	}
}

// lastRoundTrip is the round trip of the page's last /ack or /pace in ms,
// from its Resource Timing, or 0. It clears the entries, so that the
// buffer of 250 never fills with acknowledgements.
const lastRoundTrip = `((x) => (performance.clearResourceTimings(), x ? Math.round(x.responseStart - x.requestStart) : 0))(performance.getEntriesByType('resource').findLast(r => r.name.includes('/ack?') || r.name.includes('/pace?')))`

// pace paces the tab's stream once its page reports that it fell behind
// (newest-frame-behind): the hub then sends the page a write only once it
// has acknowledged the last (live.Hub.Pace), and every frame again once it
// keeps up. It counts the report in tickit_live_newest_frame_behind_total
// (live.Hub.Behind). It answers 204, also to an address or a session over
// its limit, whose report it drops.
func (s *Server) pace(w http.ResponseWriter, r *http.Request) {
	tab := r.URL.Query().Get("tab")
	actor := people.Actor(r.Context())
	if !validID(tab) || actor.IsZero() {
		http.Error(w, "bad tab id", http.StatusBadRequest)
		return
	}
	now := time.Now()
	if s.pacing.paceAddrs.Allow(edge.ClientKey(r), 1, now) && s.pacing.paces.Allow(edge.SessionID(r.Context()), 1, now) {
		id := streamID(actor, tab)
		s.hub.Behind(id)
		s.hub.Pace(id, 1)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ack takes the page's acknowledgement of its paced stream's write n
// (live.Hub.Ack), and the round trip it reports (live.Hub.RoundTrip). It
// answers 204, also to an address or a stream over its limit, whose
// acknowledgement it drops: the stream then sends its next write after
// live.AckTimeout. The second limit is per stream, so a session with
// several paced tabs does not share it.
func (s *Server) ack(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tab, token := q.Get("tab"), q.Get("s")
	n, err := strconv.ParseInt(q.Get("n"), 10, 64)
	rtt, rttErr := strconv.Atoi(q.Get("rtt"))
	actor := people.Actor(r.Context())
	if !validID(tab) || !validToken(token) || err != nil || n < 1 || q.Has("rtt") && rttErr != nil || actor.IsZero() {
		http.Error(w, "bad acknowledgement", http.StatusBadRequest)
		return
	}
	id, now := streamID(actor, tab), time.Now()
	if s.pacing.ackAddrs.Allow(edge.ClientKey(r), 1, now) && s.pacing.acks.Allow(id, 1, now) {
		if rtt > 0 && rtt <= 60_000 {
			s.hub.RoundTrip(id, time.Duration(rtt)*time.Millisecond)
		}
		s.hub.Ack(id, token, n)
	}
	w.WriteHeader(http.StatusNoContent)
}

// validToken accepts a stream's token (live.Spec.Ack): a number in base 36.
func validToken(s string) bool {
	if len(s) == 0 || len(s) > 13 {
		return false
	}
	for _, c := range []byte(s) {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'z') {
			return false
		}
	}
	return true
}
