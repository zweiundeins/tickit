// Package web serves tickit: its pages, the one stream of every tab, and
// the actions under POST /cmd/. Each area registers its routes and views
// in a file of its own (routes_<area>.go), with the helpers of action.go
// (actions), render.go (pages) and stream.go (views and presence).
// Handler puts the edge around them.
package web

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/zweiundeins/lith/edge"
	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/minify"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/demo"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/ui"
)

//go:embed static
var staticFiles embed.FS

// Config is what the server serves and how.
type Config struct {
	DB    *sqlite.DB
	W     *writer.Writer
	Hub   *live.Hub
	Store *events.Store // the event store of every domain's parts
	Log   *slog.Logger
	// Dev serves the static files as written, not minified.
	Dev bool
	// Secure is for HTTPS: a __Host- cookie and HSTS.
	Secure bool
	// SessionTTL is how long a login lasts (FR-124.1), 24 hours when zero.
	SessionTTL time.Duration
	// Projectors are the events runner's stats, which the demo's reset
	// waits on (demo.Config.Projectors); nil without a runner.
	Projectors func() events.Stats
}

// Server holds what the handlers share.
type Server struct {
	db       *sqlite.DB
	w        *writer.Writer
	hub      *live.Hub
	store    *events.Store
	log      *slog.Logger
	static   *edge.Static
	files    fs.FS    // the embedded static files, before edge.Static
	fonts    []string // URLs of the fonts a page preloads
	sessions edge.Sessions
	secure   bool
	ttl      time.Duration

	limits  *limits         // the rate limits of actions (DM-04)
	signIn  *edge.Limiter   // logins and logouts per address (NFR-09.1)
	streams *streamLimits   // streams opened and open per address (NFR-41)
	lockout *people.Lockout // failed logins per email and address (NFR-09.2)
	metrics *metrics

	mux     *http.ServeMux
	probes  []string // a URL of every GET route, for the test that GET never writes (NFR-06)
	actions []string // the pattern of every action, for the security tests (NFR-01, NFR-03)
	views   *ViewRegistry
	demo    *demo.Demo // the reset, caps and blocks (routes_demo.go)
}

// New returns the server of cfg's database, writer and hub, with every
// area's routes and views registered.
func New(cfg Config) (*Server, error) {
	if cfg.DB == nil || cfg.W == nil || cfg.Hub == nil {
		return nil, errors.New("web: a server needs a database, a writer and a hub")
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = people.DefaultSessionTTL
	}
	files, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, err
	}
	var opts []edge.StaticOption
	if !cfg.Dev {
		opts = append(opts, edge.Minify(minify.Bytes))
	}
	static, err := edge.NewStatic(files, "/static/", opts...)
	if err != nil {
		return nil, err
	}
	// lith's newest-frame.js, which the page loads after Datastar.
	js, err := fs.ReadFile(live.Scripts, live.NewestFrame)
	if err != nil {
		return nil, err
	}
	static.Add("lith/"+live.NewestFrame, js)
	// The one stylesheet of every page and the favicon (docs/design-system.md).
	css, err := BundleCSS(files, static.URL)
	if err != nil {
		return nil, fmt.Errorf("web: the stylesheet bundle: %w", err)
	}
	static.Add(Bundle, css)
	static.Add(Mark, []byte(ui.MarkSVG()))
	var fonts []string
	for _, f := range Fonts {
		fonts = append(fonts, static.URL(f))
	}
	s := &Server{
		db: cfg.DB, w: cfg.W, hub: cfg.Hub, store: cfg.Store, log: cfg.Log, static: static, files: files, fonts: fonts,
		sessions: edge.Sessions{Secure: cfg.Secure},
		secure:   cfg.Secure,
		ttl:      cfg.SessionTTL,
		limits:   newLimits(),
		signIn:   edge.NewLimiter(0.5, 10), // 30 a minute, 10 at once
		streams:  newStreamLimits(),
		lockout:  people.NewLockout(),
		metrics:  newMetrics(cfg.DB),
		mux:      http.NewServeMux(),
		views:    newViewRegistry(),
	}
	if s.demo, err = demo.New(demo.Config{DB: cfg.DB, W: cfg.W, Log: cfg.Log, Changed: cfg.Hub.Changed, MarkAll: cfg.Hub.MarkAll,
		Projectors: cfg.Projectors, OnReset: s.metrics.reset}); err != nil {
		return nil, err
	}
	s.register()
	return s, nil
}

// Demo is the demo's reset, caps and blocks: cmd/tickit runs its schedule.
func (s *Server) Demo() *demo.Demo { return s.demo }

// register calls every area's registration, each in its own file, so that
// areas add routes and views without touching another's file.
func (s *Server) register() {
	s.routesCore()
	s.routesTracker()
	s.routesBoard()
	s.routesIssues()
	s.routesSearch()
	s.routesWiki()
	s.routesEditor()
	s.routesNotify()
	s.routesAdmin()
	s.routesModeration()
	s.routesDemo()
	s.routesDesign()
}

// get registers a GET route. probes are URLs that reach it, for the test
// that GET never writes (NFR-06); a pattern without wildcards is its own.
func (s *Server) get(pattern string, h http.Handler, probes ...string) {
	path, ok := strings.CutPrefix(pattern, "GET ")
	if !ok {
		panic("web: " + pattern + " is no GET pattern")
	}
	if len(probes) == 0 {
		if strings.Contains(strings.ReplaceAll(path, "{$}", ""), "{") {
			panic("web: " + pattern + " needs a probe URL")
		}
		probes = []string{strings.ReplaceAll(path, "{$}", "")}
	}
	s.mux.Handle(pattern, h)
	s.probes = append(s.probes, probes...)
}

// post registers an action. Every change is a POST under /cmd/, where the
// edge's CSRF check wants Datastar's header (NFR-02, NFR-03).
func (s *Server) post(pattern string, h http.HandlerFunc) {
	if !strings.HasPrefix(pattern, "POST /cmd/") {
		panic("web: " + pattern + " is no action: POST /cmd/...")
	}
	s.mux.HandleFunc(pattern, h)
	s.actions = append(s.actions, pattern)
}

// Routes returns the handlers without the edge.
func (s *Server) Routes() http.Handler { return s.mux }

// Probes are a URL of every GET route, for the test that GET never writes.
func (s *Server) Probes() []string { return s.probes }

// Actions are the patterns of every action, as registered with post.
func (s *Server) Actions() []string { return slices.Clone(s.actions) }

// Handler returns the routes behind the edge: the session, the actor,
// the CSRF check, the security headers, compression, recovery and the log
// of requests. /healthz and /readyz stay outside it (cmd/tickit).
func (s *Server) Handler() http.Handler {
	actor := people.Middleware(s.db, s.log)(s.mux)
	h := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") {
			s.mux.ServeHTTP(w, r) // no actor needed: nothing to read
			return
		}
		actor.ServeHTTP(w, r)
	}))
	h = s.sessions.Handler(h) // inside CSRF: a refused request gets no cookie
	h = edge.CSRF{Datastar: []string{"POST /cmd/"}}.Handler(h)
	h = edge.SecurityHeaders{CSP: edge.DefaultCSP(), HSTS: s.secure}.Handler(h)
	h = noIndex(h)
	h = edge.Compress(h)
	h = edge.Recover(s.log, h)
	return edge.LogRequests(s.log, h)
}

// noIndex keeps search engines out of what visitors write (DM-09.1).
func noIndex(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		next.ServeHTTP(w, r)
	})
}
