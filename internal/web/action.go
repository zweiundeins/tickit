package web

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/zweiundeins/lith/edge"
	"github.com/zweiundeins/lith/sse"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/demo"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
)

// An action, in an area's handler:
//
//	var req struct {
//		Payload
//		Issue string `json:"issue"`
//	}
//	a, ok := s.decode(w, r, classField, &req)
//	if !ok {
//		return
//	}
//	s.act(w, r, a, tracker.SetPriority{By: a.By, Issue: req.Issue})
//
// decode checks the rate limits, reads the body and the ids; act runs the
// command in tabs.Act and answers 204, or 403 when a right was missing.
// The outcome travels on the tab's stream.

// Payload is what every action's body holds: the tab that sends it and
// the action's op id. Embed it in an action's request.
type Payload struct {
	Tab string `json:"tab"`
	Op  string `json:"op"`
}

func (p *Payload) payload() *Payload { return p }

type payloader interface{ payload() *Payload }

// action is a decoded request: the session and tab it comes from, and
// who acts in which op, from the session (NFR-02.2).
type action struct {
	Session string
	Tab     string
	By      tabs.By
	Addr    string // edge.ClientKey: for a moderator's block (demo.Guard), never stored
}

// class is a rate-limit class of actions (DM-04.1): a session may take
// burst at once, then one every every; a client address four times as
// much. Unless separate, the action also counts against the limit all
// actions of an address share. body caps the request's size.
type class struct {
	name     string
	every    time.Duration
	burst    int
	window   time.Duration // instead of every: burst in any window, a promise (Q24)
	separate bool
	body     int64
}

var (
	classCreate      = class{name: "create", every: 5 * time.Second, burst: 5, body: 64 << 10}
	classComment     = class{name: "comment", every: 3 * time.Second, burst: 3, body: 64 << 10}
	classField       = class{name: "field", every: time.Second / 2, burst: 20, body: 64 << 10}
	classEditBatch   = class{name: "edit-batch", every: time.Second / 10, burst: 50, separate: true, body: 256 << 10}
	classCaret       = class{name: "caret", every: time.Second / 15, burst: 30, separate: true, body: 2 << 10}
	classCreateScope = class{name: "create-scope", window: 24 * time.Hour, burst: 2, body: 8 << 10}
	classUpload      = class{name: "upload", every: 30 * time.Second, burst: 3, body: 8 << 10}
	// classView is for watching and unwatching side panels, which DM-04
	// does not name.
	classView = class{name: "view", every: time.Second / 5, burst: 20, body: 2 << 10}
)

var classes = []class{classCreate, classComment, classField, classEditBatch, classCaret, classCreateScope, classUpload, classView}

// tooMany is what a request over a limit hears (DM-04.2).
const tooMany = "Too many changes at once. Wait a moment."

type limiter struct{ byAddr, bySession *edge.Limiter }

type limits struct {
	site    *edge.Limiter // every action of an address, but the separate ones
	classes map[string]limiter
}

func newLimits() *limits {
	l := &limits{site: edge.NewLimiter(20, 60), classes: map[string]limiter{}}
	for _, c := range classes {
		if c.window > 0 {
			l.classes[c.name] = limiter{byAddr: edge.NewSliding(4*c.burst, c.window), bySession: edge.NewSliding(c.burst, c.window)}
			continue
		}
		perSecond := float64(time.Second) / float64(c.every)
		l.classes[c.name] = limiter{byAddr: edge.NewLimiter(4*perSecond, 4*c.burst), bySession: edge.NewLimiter(perSecond, c.burst)}
	}
	return l
}

// allow checks the limits of class c for the request's address, then for
// its person (an address first: a client chooses its session for free).
func (s *Server) allow(w http.ResponseWriter, r *http.Request, c class) bool {
	l, ok := s.limits.classes[c.name]
	if !ok {
		panic("web: no rate-limit class " + c.name)
	}
	now := time.Now()
	addr := edge.ClientKey(r)
	if (!c.separate && !s.limits.site.Allow(addr, 1, now)) || !l.byAddr.Allow(addr, 1, now) ||
		!l.bySession.Allow(people.Actor(r.Context()).ID, 1, now) {
		s.metrics.limited.WithLabelValues(c.name).Inc()
		http.Error(w, tooMany, http.StatusTooManyRequests)
		return false
	}
	return true
}

// decode reads an action of class c into v, after the rate limits. It
// answers 429 over a limit and 400 for a body that is too large, not JSON,
// or without valid tab and op ids (NFR-16), and then reports false.
func (s *Server) decode(w http.ResponseWriter, r *http.Request, c class, v payloader) (action, bool) {
	if !s.allow(w, r, c) {
		return action{}, false
	}
	if err := sse.ReadSignalsLimit(w, r, v, c.body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return action{}, false
	}
	p := v.payload()
	actor := people.Actor(r.Context())
	if !validID(p.Tab) || !validID(p.Op) || actor.IsZero() {
		http.Error(w, "bad request", http.StatusBadRequest)
		return action{}, false
	}
	if mustChange(w, actor) {
		return action{}, false
	}
	return action{Session: edge.SessionID(r.Context()), Tab: p.Tab, By: tabs.By{Actor: actor, Op: p.Op, At: time.Now()}, Addr: edge.ClientKey(r)}, true
}

// mustChange answers 403 for an account that must change its first
// password before anything else (FR-141.1), and then reports true.
func mustChange(w http.ResponseWriter, actor people.Person) bool {
	if actor.MustChange {
		http.Error(w, "Change your password first.", http.StatusForbidden)
	}
	return actor.MustChange
}

// run runs cmd as action a in tabs.Act, once per op id: a retry gets the
// first answer and runs nothing. It answers the requests that reached no
// outcome itself and reports false then.
func (s *Server) run(w http.ResponseWriter, r *http.Request, a action, cmd writer.Command) (tabs.Result, bool) {
	// The demo's guard refuses while it resets, for a blocked guest, and
	// past the caps (DM-03.4, DM-07.2, DM-04.3).
	cmd = s.demo.Guard(a.By.Actor, a.Addr, cmd)
	op := writer.OnceResult(a.Session+":"+a.By.Op, tabs.Act{Session: a.Session, Tab: a.Tab, By: a.By, Cmd: cmd})
	if err := s.w.Do(r.Context(), op); err != nil {
		s.failAction(w, r, err)
		return tabs.Result{}, false
	}
	res := tabs.ParseResult(op.Result())
	if !res.OK && !op.Replayed() {
		s.metrics.refusals.WithLabelValues(r.Pattern).Inc()
	}
	return res, true
}

// act runs cmd as action a and answers 204: its outcome, a refusal
// included, reaches the page on the tab's stream. A refusal for want of a
// right answers 403 besides (NFR-01.1).
func (s *Server) act(w http.ResponseWriter, r *http.Request, a action, cmd writer.Command) {
	res, ok := s.run(w, r, a, cmd)
	switch {
	case !ok:
	case res.Denied:
		http.Error(w, "forbidden", http.StatusForbidden)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// resultCommand is a command with an answer, as writer.OnceResult wants:
// Apply for tests and tools, ApplyResult for actResult.
type resultCommand interface {
	writer.Command
	writer.ResultCommand
}

// actResult runs cmd, a command with an answer, as action a, and when it
// went through sends the page where goTo says, from the command's result
// (the key of a new issue to its address): a signal patch of $_goto, never
// elements. A refusal answers 204, and 403 for want of a right.
func (s *Server) actResult(w http.ResponseWriter, r *http.Request, a action, cmd resultCommand, goTo func(result []byte) string) {
	res, ok := s.run(w, r, a, cmd)
	switch {
	case !ok:
	case res.Denied:
		http.Error(w, "forbidden", http.StatusForbidden)
	case !res.OK:
		w.WriteHeader(http.StatusNoContent)
	default:
		s.signals(w, map[string]any{"_goto": goTo(res.Value)})
	}
}

// signals answers with a patch of the page's signals: for what concerns
// the tab that asked alone, such as a $_goto or a message of a form.
func (s *Server) signals(w http.ResponseWriter, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // a map of strings and bools
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	w.Write(sse.PatchSignals(b, false))
}

// failAction answers an action that reached no outcome: 400 for a command
// that Validate refused, 404 for an id that names nothing the actor may
// see, 503 while the server stops or the demo resets, 500 for the rest
// (logged).
func (s *Server) failAction(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, writer.ErrInvalid):
		http.Error(w, strings.TrimPrefix(err.Error(), writer.ErrInvalid.Error()+": "), http.StatusBadRequest)
	case errors.Is(err, access.ErrNotFound), errors.Is(err, people.ErrNoAccount):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, demo.ErrResetting):
		http.Error(w, demo.MsgResetting+". Try again in a minute.", http.StatusServiceUnavailable)
	case errors.Is(err, writer.ErrClosed), errors.Is(err, writer.ErrQueueFull), r.Context().Err() != nil:
		http.Error(w, "The server is restarting. Try again in a moment.", http.StatusServiceUnavailable)
	default:
		s.fail(w, r, err)
	}
}

// fail logs an error and answers 500.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	http.Error(w, "Something went wrong.", http.StatusInternalServerError)
}

// newID returns a tab id: 128 random bits in base64url.
func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// validID accepts tab ids and the op ids pages make: 1 to 64 letters,
// digits, - and _.
func validID(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, c := range []byte(s) {
		if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
