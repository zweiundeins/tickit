package web

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/zweiundeins/lith/edge"
	"github.com/zweiundeins/lith/sse"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/demo"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
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
	classUpload      = class{name: "upload", every: 30 * time.Second, burst: 3, body: 6 << 20}
	// classBulk is a bulk edit of up to 100 issues (FR-58), which would
	// take five times the field class's burst at once.
	classBulk = class{name: "bulk", every: 10 * time.Second, burst: 3, body: 16 << 10}
	// classView is for watching and unwatching side panels, which DM-04
	// does not name.
	classView = class{name: "view", every: time.Second / 5, burst: 20, body: 2 << 10}
)

var classes = []class{classCreate, classComment, classField, classEditBatch, classCaret, classCreateScope, classUpload, classView, classBulk}

// What the notice of an op that reached no outcome says (tabs.Notice): a
// request over a limit (DM-04.2), a failed write, the demo's reset, a
// server that stops.
const (
	tooMany    = "Too many changes at once. Wait a moment."
	notSaved   = "This did not go through. Try again in a moment."
	resetting  = demo.MsgResetting + ". Try again in a minute."
	restarting = "The server is restarting. Try again in a moment."
)

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

// allow checks the limits of class c and answers 429 over one, with no
// notice: for requests sent without an op.
func (s *Server) allow(w http.ResponseWriter, r *http.Request, c class) bool {
	if s.within(r, c) {
		return true
	}
	w.WriteHeader(http.StatusTooManyRequests)
	return false
}

// within reports whether the request is within the limits of class c for
// its address, then for its person (an address first: a client chooses
// its session for free), and counts it against them.
func (s *Server) within(r *http.Request, c class) bool {
	l, ok := s.limits.classes[c.name]
	if !ok {
		panic("web: no rate-limit class " + c.name)
	}
	now := time.Now()
	addr := edge.ClientKey(r)
	if (!c.separate && !s.limits.site.Allow(addr, 1, now)) || !l.byAddr.Allow(addr, 1, now) ||
		!l.bySession.Allow(people.Actor(r.Context()).ID, 1, now) {
		s.metrics.limited.WithLabelValues(c.name).Inc()
		return false
	}
	return true
}

// decode reads an action of class c into v. It answers 429 over a limit,
// with a notice that ends the op, and 400 for a body that is too large,
// not JSON, or without valid tab and op ids (NFR-16), and then reports
// false.
func (s *Server) decode(w http.ResponseWriter, r *http.Request, c class, v payloader) (action, bool) {
	ok := s.within(r, c)
	err := sse.ReadSignalsLimit(w, r, v, c.body)
	switch {
	case !ok:
		if err == nil {
			s.notice(r, v.payload().Tab, v.payload().Op, tooMany)
		}
		w.WriteHeader(http.StatusTooManyRequests)
		return action{}, false
	case err != nil:
		w.WriteHeader(http.StatusBadRequest)
		return action{}, false
	}
	return s.actionOf(w, r, v.payload())
}

// notice sends the tab's open stream the notice that ends op with msg
// (tabs.Notice), for an op that reached no outcome. The stream is the
// actor's own: a tab id names nobody else's.
func (s *Server) notice(r *http.Request, tab, op, msg string) {
	actor := people.Actor(r.Context())
	if actor.IsZero() || !validID(tab) || !validID(op) {
		return
	}
	s.hub.Notify(streamID(actor, tab), "notice:"+op, tabs.Notice(op, msg))
}

// actionOf is the action of a request whose payload p was read: 400
// without valid tab and op ids, 403 for an account that must change its
// password first.
func (s *Server) actionOf(w http.ResponseWriter, r *http.Request, p *Payload) (action, bool) {
	actor := people.Actor(r.Context())
	if !validID(p.Tab) || !validID(p.Op) || actor.IsZero() {
		w.WriteHeader(http.StatusBadRequest)
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
		w.WriteHeader(http.StatusForbidden)
	}
	return actor.MustChange
}

// paused answers a change sent without waiting, outside tabs.Act, during
// a reset (503) or from a blocked guest (403), and then reports true
// (DM-03.4, DM-07.2).
func (s *Server) paused(w http.ResponseWriter, r *http.Request, actor people.Person) bool {
	err := s.demo.Check(r.Context(), s.db.R, actor, edge.ClientKey(r))
	var denial *access.Denial
	switch {
	case err == nil:
		return false
	case errors.As(err, &denial):
		w.WriteHeader(http.StatusForbidden)
	default:
		s.failAction(w, r, err)
	}
	return true
}

// result is an action's outcome, and whether it is a retry's, which ran
// nothing: what follows an action outside the writer follows it once.
type result struct {
	tabs.Result
	replayed bool
}

// run runs cmd as action a in tabs.Act, once per session, path and op id:
// a retry gets the first answer and runs nothing. Another action that
// names the same op id runs: it must not get the first one's answer, as
// an OK that would start what it may not (a reset). It answers the
// requests that reached no outcome itself and reports false then.
func (s *Server) run(w http.ResponseWriter, r *http.Request, a action, cmd writer.Command) (result, bool) {
	return s.runNoted(w, r, a, cmd, nil)
}

// runNoted is run with tabs.Act's Note, which makes the result of a
// command that went through its outcome's message and data.
func (s *Server) runNoted(w http.ResponseWriter, r *http.Request, a action, cmd writer.Command, note func([]byte) (string, json.RawMessage)) (result, bool) {
	if !s.connected(w, r, a.By.Actor, a.Tab) {
		return result{}, false
	}
	// The demo's guard refuses while it resets, for a blocked guest, and
	// past the caps (DM-03.4, DM-07.2, DM-04.3).
	cmd = s.demo.Guard(a.By.Actor, a.Addr, cmd)
	op := writer.OnceResult(a.Session+":"+r.URL.Path+":"+a.By.Op, tabs.Act{Session: a.Session, Tab: a.Tab, By: a.By, Cmd: cmd, Note: note})
	if err := s.w.Do(r.Context(), op); err != nil {
		s.failOp(w, r, a, err)
		return result{}, false
	}
	res := result{tabs.ParseResult(op.Result()), op.Replayed()}
	if !res.OK && !res.replayed {
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
		w.WriteHeader(http.StatusForbidden)
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
		w.WriteHeader(http.StatusForbidden)
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

// failAction answers an action that reached no outcome, with the status
// alone: 400 for a command that Validate refused (no page of ours sends
// one), 404 for an id that names nothing the actor may see, 503 while the
// server stops or the demo resets, 500 for the rest (logged).
func (s *Server) failAction(w http.ResponseWriter, r *http.Request, err error) {
	s.failOp(w, r, action{}, err)
}

// failOp is failAction for action a, whose tab hears why in a notice that
// ends the op where the page does not: a 503 or a 500.
func (s *Server) failOp(w http.ResponseWriter, r *http.Request, a action, err error) {
	code, msg := http.StatusInternalServerError, notSaved
	switch {
	case errors.Is(err, writer.ErrInvalid):
		code, msg = http.StatusBadRequest, ""
	case errors.Is(err, access.ErrNotFound), errors.Is(err, people.ErrNoAccount):
		code, msg = http.StatusNotFound, ""
	case errors.Is(err, demo.ErrResetting):
		code, msg = http.StatusServiceUnavailable, resetting
	case errors.Is(err, writer.ErrClosed), errors.Is(err, writer.ErrQueueFull), r.Context().Err() != nil:
		code, msg = http.StatusServiceUnavailable, restarting
	default:
		s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	}
	if msg != "" && a.Tab != "" {
		s.notice(r, a.Tab, a.By.Op, msg)
	}
	w.WriteHeader(code)
}

// connected reports whether the tab's stream is open, waiting up to
// Config.StreamWait for it, and answers 409 otherwise, or 503 once the hub
// has stopped: with no notice, since no stream would carry it. The page
// waits for its stream, whose tab view ends the op (views/expr.go). An
// action that stores tab state checks it before its command (lith's
// live.Hub.HasStream).
func (s *Server) connected(w http.ResponseWriter, r *http.Request, actor people.Person, tab string) bool {
	id := streamID(actor, tab)
	if s.hub.HasStream(id) {
		return true
	}
	deadline := time.NewTimer(s.wait)
	defer deadline.Stop()
	poll := time.NewTicker(20 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-poll.C:
			if s.hub.HasStream(id) {
				return true
			}
			continue
		case <-deadline.C:
		case <-r.Context().Done():
		case <-s.hub.Done(): // a deploy: no stream comes, and the shutdown waits for this
			w.WriteHeader(http.StatusServiceUnavailable)
			return false
		}
		w.WriteHeader(http.StatusConflict)
		return false
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
