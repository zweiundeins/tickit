package web

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/zweiundeins/lith/edge"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
)

// The open streams, as the server needs them besides the hub: who watches
// what since the stream opened (NFR-01.2, access.go), and whether the page
// is still there (MP-02.3). An entry lives as long as its stream: it is
// the state of a connection, gone with it, never tab state.
//
// A page whose network went away without closing its connection keeps its
// stream open for minutes: the server's heartbeats and frames go into the
// kernel's buffers, and only a full buffer would make a write miss its
// deadline. So the page answers each heartbeat ($_ping, every 20 s) with
// POST /alive, and a stream that opened with alive=1 ends once none came
// for AliveTimeout: its faces leave every presence row within a minute.

// AliveTimeout is how long a stream that promised to answer heartbeats
// (alive=1) stays open without an answer: two heartbeats of 20 s and the
// time for one to come back. A server takes it as it starts.
var AliveTimeout = 50 * time.Second

// openStream is one open stream.
type openStream struct {
	actor people.Person
	sid   string
	tab   string
	specs []string // the views its URL named
	alive *time.Timer
}

type openStreams struct {
	mu sync.Mutex
	m  map[string]*openStream // by stream ID
}

func newOpenStreams() *openStreams { return &openStreams{m: map[string]*openStream{}} }

func (o *openStreams) add(id string, st *openStream) {
	o.mu.Lock()
	o.m[id] = st
	o.mu.Unlock()
}

// remove forgets the stream, unless a stream that replaced it holds its ID.
func (o *openStreams) remove(id string, st *openStream) {
	o.mu.Lock()
	if o.m[id] == st {
		delete(o.m, id)
	}
	o.mu.Unlock()
}

func (o *openStreams) get(id string) *openStream {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.m[id]
}

// of are the open streams of a person, by ID.
func (o *openStreams) of(person string) map[string]*openStream {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := map[string]*openStream{}
	for id, st := range o.m {
		if st.actor.ID == person {
			out[id] = st
		}
	}
	return out
}

// track registers a stream before it is served, and returns the context
// to serve it in, which ends when its page stops answering heartbeats,
// and the function to call once it ended.
func (s *Server) track(r *http.Request, id string, st *openStream) (context.Context, func()) {
	ctx, cancel := context.WithCancel(r.Context())
	if r.URL.Query().Get("alive") == "1" {
		st.alive = time.AfterFunc(s.aliveFor, cancel)
	}
	s.open.add(id, st)
	return ctx, func() {
		if st.alive != nil {
			st.alive.Stop()
		}
		cancel()
		s.open.remove(id, st)
		s.streamEnded(id, st)
	}
}

// streamEnded takes away what the tab showed others while it was there:
// its typing indicators (MP-07.4). A stream that another with its ID
// replaced (a reconnect) leaves them. It writes only when the tab had one
// up, so a GET of a stream writes nothing of its own (NFR-06).
func (s *Server) streamEnded(id string, st *openStream) {
	if s.hub.HasStream(id) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if up, err := tracker.TabTypes(ctx, s.db.R, st.actor.ID, st.tab); err != nil || !up {
		return
	}
	if err := s.w.Send(tracker.EndTyping{Person: st.actor.ID, Tab: st.tab}); err != nil {
		s.log.Debug("the typing of a tab that went", "err", err)
	}
}

// alive is the page's answer to a heartbeat (POST /alive?tab=): its
// stream stays open for another AliveTimeout. It reaches only the
// streams of the request's person, and answers 204, also over its limits.
func (s *Server) alive(w http.ResponseWriter, r *http.Request) {
	tab := r.URL.Query().Get("tab")
	actor := people.Actor(r.Context())
	if !validID(tab) || actor.IsZero() {
		http.Error(w, "bad tab id", http.StatusBadRequest)
		return
	}
	id, now := streamID(actor, tab), time.Now()
	if s.pacing.aliveAddrs.Allow(edge.ClientKey(r), 1, now) && s.pacing.alives.Allow(id, 1, now) {
		if st := s.open.get(id); st != nil && st.alive != nil {
			st.alive.Reset(s.aliveFor)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
