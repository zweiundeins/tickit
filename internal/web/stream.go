package web

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/zweiundeins/lith/edge"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/sse"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
	"github.com/mbolli/tickit/internal/views"
)

// Every document opens one stream, GET /stream?tab=<tab>&v=<kind>:<arg>,...
// Each v names a view that an area registered, which the stream watches
// once the actor may read it; every stream watches the tab's view, the
// person's view and the demo's view besides. A side panel adds a view to
// the open stream with POST /cmd/watch, and the tab's row remembers it for
// the stream that reconnects (docs/architecture.md).

// ViewFactory makes the view that "kind:arg" names for actor, reading in
// q. It answers access.ErrNotFound for an arg that names nothing the actor
// may see (404), and a refusal that is Denied, such as an access.Denial,
// for one the actor may not read (403). The view's key must hold
// everything its render depends on, and a shared view nothing about its
// viewer.
type ViewFactory func(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error)

type viewKind struct {
	make     ViewFactory
	presence bool
}

// ViewRegistry holds the kinds of views pages may name. Areas register
// theirs from their routes file: s.views.Register("issue", s.issueView).
type ViewRegistry struct{ kinds map[string]viewKind }

// Views is the registry of view kinds: for areas, and for tests that
// register a kind before they open a stream.
func (s *Server) Views() *ViewRegistry { return s.views }

// Kinds are the registered kinds of views, sorted.
func (vr *ViewRegistry) Kinds() []string { return slices.Sorted(maps.Keys(vr.kinds)) }

func newViewRegistry() *ViewRegistry { return &ViewRegistry{kinds: map[string]viewKind{}} }

var kindPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// Register adds a kind of view.
func (vr *ViewRegistry) Register(kind string, f ViewFactory) { vr.add(kind, f, false) }

// RegisterWithPresence adds a kind of view whose watchers show as faces:
// a stream that watches one of its views watches its presence view too
// (presenceView, MP-02).
func (vr *ViewRegistry) RegisterWithPresence(kind string, f ViewFactory) { vr.add(kind, f, true) }

func (vr *ViewRegistry) add(kind string, f ViewFactory, presence bool) {
	if !kindPattern.MatchString(kind) || f == nil {
		panic("web: view kind " + kind + ": lowercase letters, digits and -, and a factory")
	}
	if _, ok := vr.kinds[kind]; ok {
		panic("web: view kind " + kind + " registered twice")
	}
	vr.kinds[kind] = viewKind{make: f, presence: presence}
}

// MaxViews is how many views a document may name in its stream's URL.
const MaxViews = 8

var argPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{0,200}$`)

// errBadView is the error of a malformed or unknown view (400).
var errBadView = errors.New("bad view")

// parseViews splits the v parameter into "kind:arg" specs, each once.
func parseViews(v string) ([]string, error) {
	if v == "" {
		return nil, nil
	}
	specs := strings.Split(v, ",")
	if len(specs) > MaxViews {
		return nil, fmt.Errorf("%w: more than %d views", errBadView, MaxViews)
	}
	var out []string
	for _, spec := range specs {
		kind, arg, _ := strings.Cut(spec, ":")
		if !kindPattern.MatchString(kind) || !argPattern.MatchString(arg) {
			return nil, fmt.Errorf("%w: %q", errBadView, spec)
		}
		if !contains(out, spec) {
			out = append(out, spec)
		}
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// resolve makes the views a spec names: its own, and its presence view.
func (s *Server) resolve(ctx context.Context, q sqlite.Querier, actor people.Person, spec string) ([]live.View, error) {
	kind, arg, _ := strings.Cut(spec, ":")
	k, ok := s.views.kinds[kind]
	if !ok {
		return nil, fmt.Errorf("%w: no view kind %q", errBadView, kind)
	}
	v, err := k.make(ctx, q, actor, arg)
	if err != nil {
		return nil, err
	}
	if v.Key == "" || v.Render == nil {
		return nil, fmt.Errorf("web: view %s has no key or no render", spec)
	}
	if k.presence {
		return []live.View{v, s.presenceView(v.Key)}, nil
	}
	return []live.View{v}, nil
}

// failView answers a view that could not be resolved: 400, 404 or 403.
func (s *Server) failView(w http.ResponseWriter, r *http.Request, err error) {
	var denial interface{ Denied() bool }
	switch {
	case errors.Is(err, errBadView):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, access.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.As(err, &denial) && denial.Denied():
		http.Error(w, "forbidden", http.StatusForbidden)
	default:
		s.fail(w, r, err)
	}
}

// streamID is the ID of a person's tab's stream: hub.Watchers then yields
// who watches a view without a read (MP-01, MP-02). Never render it: it
// holds the tab id, with which anyone could replace the tab's stream.
func streamID(actor people.Person, tab string) string { return actor.ID + "." + tab }

// personOf is the person id of a stream ID.
func personOf(id string) string {
	p, _, _ := strings.Cut(id, ".")
	return p
}

// stream serves the tab's stream: the views its URL names, the views its
// row says it added, the person's, the demo's and its own. It writes
// nothing (NFR-06).
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tab := r.URL.Query().Get("tab")
	if !validID(tab) {
		http.Error(w, "bad tab id", http.StatusBadRequest)
		return
	}
	specs, err := parseViews(r.URL.Query().Get("v"))
	if err != nil {
		s.failView(w, r, err)
		return
	}
	actor, sid := people.Actor(ctx), edge.SessionID(ctx)
	if actor.IsZero() {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	client, ok := s.streams.open(w, r)
	if !ok {
		return
	}
	if len(specs) > 0 && mustChange(w, actor) { // FR-141.1: the account's page alone, which has no views
		return
	}
	var list []live.View
	err = s.db.View(ctx, func(tx *sql.Tx) error {
		for _, spec := range specs {
			vs, err := s.resolve(ctx, tx, actor, spec)
			if err != nil {
				return err
			}
			list = append(list, vs...)
		}
		st, err := tabs.Get(ctx, tx, sid, tab)
		if err != nil {
			return err
		}
		for _, watched := range st.Watched { // a side panel open before a reconnect
			if actor.MustChange {
				break
			}
			vs, err := s.resolve(ctx, tx, actor, watched.View)
			if err != nil {
				s.log.Debug("a watched view is gone", "view", watched.View, "err", err)
				continue
			}
			list = append(list, vs...)
		}
		return nil
	})
	if err != nil {
		s.failView(w, r, err)
		return
	}
	// Shared views first, the tab's own last: its outcomes arrive with
	// the frames they concern, in the same tick.
	list = append(list, s.personView(actor), s.demoView(), connectedView, tabs.PrefsView(sid, tab), tabs.View(sid, tab))
	spec := live.Spec{
		ID:        streamID(actor, tab),
		Client:    client,
		Views:     unique(list),
		Reconnect: views.StreamID,
		Slow:      edge.SlowLink(r),
		Ack:       ackEvent(tab), // paced once the page reports it fell behind (pace.go)
	}
	if actor.Kind == people.Account { // NFR-04.2: a logout ends the login's streams too
		ctx, stop := context.WithCancel(ctx)
		defer stop()
		go s.endWithLogin(ctx, stop, sid, actor)
		r = r.WithContext(ctx)
	}
	if err := s.hub.Serve(w, r, spec); err != nil {
		s.log.Debug("stream ended", "err", err)
	}
}

// Streams per client address (NFR-41): the hub's cap of all streams is
// not for one client to take. The hub holds an address to
// MaxStreamsPerAddr open at once (live.Options.MaxStreamsPerClient, 429
// beyond), which leaves room for a room of people behind one NAT, and an
// address opens them at StreamOpens a second after a burst of as many. A
// client on the host itself (a probe, a load test: loopback without a
// proxy's header) has the hub's cap alone.
const (
	MaxStreamsPerAddr = 40
	StreamOpens       = 1
)

type streamLimits struct{ opens *edge.Limiter }

func newStreamLimits() *streamLimits {
	return &streamLimits{opens: edge.NewLimiter(StreamOpens, MaxStreamsPerAddr)}
}

// open checks the rate of the request's address and returns the key the
// hub counts its stream under (live.Spec.Client), or answers 429 and
// reports false.
func (l *streamLimits) open(w http.ResponseWriter, r *http.Request) (client string, ok bool) {
	if edge.ClientIP(r).IsLoopback() {
		return "", true
	}
	addr := edge.ClientKey(r)
	if !l.opens.Allow(addr, 1, time.Now()) {
		http.Error(w, tooMany, http.StatusTooManyRequests)
		return "", false
	}
	return addr, true
}

// LoginCheck is how often an account's stream checks that its login
// session still stands.
const LoginCheck = 2 * time.Second

// endWithLogin ends an account's stream, by stop, once its session no
// longer resolves to the account as it was: after a logout, a password
// change, an ended or expired session, a deactivation or a new site role.
// The page reconnects, as whoever the session is now.
func (s *Server) endWithLogin(ctx context.Context, stop context.CancelFunc, sid string, actor people.Person) {
	t := time.NewTicker(LoginCheck)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		p, err := people.Resolve(ctx, s.db.R, sid, time.Now())
		if err == nil && (p.ID != actor.ID || p.Role != actor.Role || p.MustChange != actor.MustChange) {
			stop()
			return
		}
	}
}

// unique keeps the first view of each key.
func unique(list []live.View) []live.View {
	seen := map[string]bool{}
	out := list[:0]
	for _, v := range list {
		if !seen[v.Key] {
			seen[v.Key] = true
			out = append(out, v)
		}
	}
	return out
}

// connectedView tells a page that a new stream delivers: its one frame
// goes out first thing on every stream, and hides "Reconnecting".
var connectedView = live.View{
	Key: "connected",
	Render: func(context.Context, *sql.Tx) (any, error) {
		return sse.PatchSignals([]byte(`{"_stale":false}`), false), nil
	},
}

// watchRequest names a view to add to the tab's stream, or to take off it.
type watchRequest struct {
	Tab  string `json:"tab"`
	View string `json:"view"`
}

// watch adds a view, a side panel's, to the tab's open stream (hub.Watch)
// and records it in the tab's row, so a stream that reconnects watches it
// again. Adding one beyond tabs.MaxWatched drops the oldest.
func (s *Server) watch(w http.ResponseWriter, r *http.Request) {
	req, ok := s.decodeWatch(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	actor, sid := people.Actor(ctx), edge.SessionID(ctx)
	vs, err := s.db.ViewValue(ctx, func(tx *sql.Tx) ([]live.View, error) { return s.resolve(ctx, tx, actor, req.View) })
	if err != nil {
		s.failView(w, r, err)
		return
	}
	dropped, err := s.w.DoValue(ctx, func(tx *writer.Tx) ([]string, error) {
		return tabs.AddWatched(tx, sid, req.Tab, tabs.Watched{View: req.View, Key: vs[0].Key}, time.Now())
	})
	if err != nil {
		s.failAction(w, r, err)
		return
	}
	id := streamID(actor, req.Tab)
	for _, key := range dropped {
		s.hub.Unwatch(id, key)
		s.hub.Unwatch(id, presenceKey(key))
	}
	for _, v := range vs {
		s.hub.Watch(id, v)
	}
	w.WriteHeader(http.StatusNoContent)
}

// unwatch takes a view the tab added off its stream, and out of its row.
func (s *Server) unwatch(w http.ResponseWriter, r *http.Request) {
	req, ok := s.decodeWatch(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	actor, sid := people.Actor(ctx), edge.SessionID(ctx)
	st, err := s.db.ViewValue(ctx, func(tx *sql.Tx) (tabs.State, error) { return tabs.Get(ctx, tx, sid, req.Tab) })
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, watched := range st.Watched {
		if watched.View != req.View {
			continue
		}
		err := s.w.Do(ctx, writer.Func(func(tx *writer.Tx) error { return tabs.RemoveWatched(tx, sid, req.Tab, watched.Key, time.Now()) }))
		if err != nil {
			s.failAction(w, r, err)
			return
		}
		s.hub.Unwatch(streamID(actor, req.Tab), watched.Key)
		s.hub.Unwatch(streamID(actor, req.Tab), presenceKey(watched.Key))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) decodeWatch(w http.ResponseWriter, r *http.Request) (watchRequest, bool) {
	var req watchRequest
	if !s.allow(w, r, classView) {
		return req, false
	}
	if err := sse.ReadSignalsLimit(w, r, &req, classView.body); err != nil || !validID(req.Tab) || people.Actor(r.Context()).IsZero() {
		http.Error(w, "bad request", http.StatusBadRequest)
		return req, false
	}
	if mustChange(w, people.Actor(r.Context())) {
		return req, false
	}
	specs, err := parseViews(req.View)
	if err != nil || len(specs) != 1 {
		http.Error(w, "bad view", http.StatusBadRequest)
		return req, false
	}
	return req, true
}

// presenceKey is the key of the presence view of the view with key.
func presenceKey(key string) string { return "presence:" + key }

// MaxPresent is how many people a presence row lists at most; it shows
// views.Shown of them and counts the rest.
const MaxPresent = 50

// presenceView is the row of faces of the people who watch the view with
// key: one face per person, however many tabs they have on it, in the
// order they came (MP-01, MP-02). It renders again when a stream starts or
// stops watching (Presence) and when names change (people.Topic). It is
// shared: the document marks the viewer's own face (views.Shell.Me).
func (s *Server) presenceView(of string) live.View {
	key := presenceKey(of)
	return live.View{
		Key:      key,
		Topics:   []string{people.Topic},
		Presence: true,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			var ids []string
			count := map[string]int{}
			for _, id := range s.hub.Watchers(key) {
				p := personOf(id)
				if count[p] == 0 && len(ids) < MaxPresent {
					ids = append(ids, p)
				}
				count[p]++
			}
			found, err := people.Lookup(ctx, querier(ctx, tx), ids...)
			if err != nil {
				return nil, err
			}
			list := make([]views.Present, 0, len(ids))
			for _, id := range ids {
				list = append(list, views.Present{Person: found[id], Tabs: count[id]})
			}
			return patch(ctx, views.Presence(views.PresenceID(of), list))
		},
	}
}

// querier is a render's prepared statements in its snapshot, or tx.
func querier(ctx context.Context, tx *sql.Tx) sqlite.Querier {
	if q, ok := sqlite.QuerierFrom(ctx); ok {
		return q
	}
	return tx
}

// patch renders c as an event that morphs it into the page by its id.
func patch(ctx context.Context, c templ.Component, p ...sse.Patch) ([]byte, error) {
	var buf bytes.Buffer
	if err := c.Render(ctx, &buf); err != nil {
		return nil, err
	}
	return sse.PatchElements(buf.Bytes(), p...), nil
}
