package e2e

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zweiundeins/lith-coedit/collab"
	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/dbtest"
	"github.com/zweiundeins/tickit/internal/demo"
	"github.com/zweiundeins/tickit/internal/demo/seed"
	"github.com/zweiundeins/tickit/internal/notify"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web"
	"github.com/zweiundeins/tickit/internal/wiki"
	"github.com/zweiundeins/tickit/migrations"
)

// The one store of every area's parts, as cmd/tickit binds it.
var bindStores = sync.OnceValue(func() *events.Store {
	st := events.Must(events.New(append(append(tracker.Parts(), notify.Parts()...), wiki.Parts()...)...))
	tracker.Bind(st)
	wiki.Bind(st)
	return st
})

// realServer is tickit as cmd/tickit serves it: the database file, the
// writer, the hub with its 50 ms tick, the events runner, the wiki's jobs,
// on a port that a restart keeps. Stop and Start again is a deploy.
type realServer struct {
	t    testing.TB
	path string
	addr string
	URL  string

	db     *sqlite.DB
	w      *writer.Writer
	hub    *live.Hub
	site   *web.Server
	srv    *http.Server
	stop   func()
	logs   io.Writer
	closed chan struct{}
}

func newRealServer(t testing.TB) *realServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	s := &realServer{t: t, path: filepath.Join(t.TempDir(), "tickit.db"), addr: addr, URL: "http://" + addr, logs: io.Discard}
	if os.Getenv("WIKI_LOG") != "" {
		s.logs = os.Stderr
	}
	s.Start()
	t.Cleanup(s.Stop)
	return s
}

// Start serves on the server's port, from its database.
func (s *realServer) Start() {
	t := s.t
	t.Helper()
	store := bindStores()
	collab.ResetCaches() // a new process
	log := slog.New(slog.NewTextHandler(s.logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx := context.Background()
	db, err := sqlite.Open(ctx, s.path, sqlite.Options{Synchronous: dbtest.Synchronous(), Migrations: migrations.FS, Readers: runtime.GOMAXPROCS(0) + 4, Checkpoint: sqlite.CheckpointWriter})
	if err != nil {
		t.Fatal(err)
	}
	hub := live.New(live.Options{Read: db.R, MaxStreams: 400, MaxStreamsPerClient: web.MaxStreamsPerAddr, HeartbeatSignal: "_ping", Log: log})
	w, err := writer.New(db, writer.Options{OnCommit: hub.Changed, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	notify.Bind(w)
	runner := events.NewRunner(store, db, w, events.Options{Log: log})
	site, err := web.New(web.Config{DB: db, W: w, Hub: hub, Store: store, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	var ln net.Listener
	for i := 0; ; i++ {
		if ln, err = net.Listen("tcp", s.addr); err == nil || i == 50 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	s.db, s.w, s.hub, s.site = db, w, hub, site
	s.srv = &http.Server{Handler: site.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go w.Run(context.Background())
	hubCtx, stopHub := context.WithCancel(context.Background())
	hubDone := make(chan struct{})
	go func() { defer close(hubDone); hub.Run(hubCtx) }()
	runCtx, stopRunner := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() { defer close(runDone); runner.Run(runCtx) }()
	jobsCtx, stopJobs := context.WithCancel(context.Background())
	jobsDone := make(chan struct{})
	go func() { defer close(jobsDone); site.WikiJobs(jobsCtx) }()
	go site.PresenceJobs(jobsCtx)
	s.closed = make(chan struct{})
	go func() { defer close(s.closed); s.srv.Serve(ln) }()
	s.stop = func() {
		stopJobs()
		<-jobsDone
		stopHub()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.srv.Shutdown(sctx)
		<-hubDone
		stopRunner()
		<-runDone
		w.Close()
		db.Close()
		<-s.closed
	}
}

// Stop ends the server as a deploy does: streams first, then the writer.
func (s *realServer) Stop() {
	if s.stop != nil {
		s.stop()
		s.stop = nil
	}
}

func (s *realServer) do(cmd writer.Command) error { return s.w.Do(context.Background(), cmd) }

// seedAll writes the demo's seed into the server, as the binary does on
// an empty database: people, projects, issues, spaces, pages, versions,
// comments.
func (s *realServer) seedAll() {
	t := s.t
	t.Helper()
	c, err := seed.Load()
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := demo.Epoch(context.Background(), s.db.R)
	if err != nil {
		t.Fatal(err)
	}
	n, err := demo.Seed(context.Background(), s.w, c, time.Now(), epoch)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("seeded %d issues and %d page steps in %v", n.Issues, n.PageSteps, n.Took)
}

// statusID is a status's id from its name: "In Review" is "inreview".
func statusID(name string) string {
	id := strings.ToLower(strings.ReplaceAll(name, " ", ""))
	if id == "selectedfordevelopment" {
		return "selected"
	}
	return id
}

func jsonField(b []byte, key string) string {
	var m map[string]string
	if err := jsonUnmarshal(b, &m); err != nil {
		return ""
	}
	return m[key]
}

// TestServeWiki serves the seeded demo for a person to look at: set
// WIKI_SERVE to a duration ("30m") and WIKI_ADDR to the address.
func TestServeWiki(t *testing.T) {
	d, err := time.ParseDuration(os.Getenv("WIKI_SERVE"))
	if err != nil {
		t.Skip("WIKI_SERVE unset")
	}
	s := &realServer{t: t, path: filepath.Join(t.TempDir(), "tickit.db"), addr: os.Getenv("WIKI_ADDR"), logs: os.Stderr}
	if s.addr == "" {
		s.addr = "127.0.0.1:8099"
	}
	s.URL = "http://" + s.addr
	s.Start()
	defer s.Stop()
	s.seedAll()
	t.Logf("serving %s for %v", s.URL, d)
	select {
	case <-time.After(d):
	case <-t.Context().Done():
	}
}

var errNoBrowser = errors.New("no browser")
