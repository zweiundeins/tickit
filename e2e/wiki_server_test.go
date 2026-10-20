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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/demo/seed"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
	"github.com/mbolli/tickit/internal/tracker"
	"github.com/mbolli/tickit/internal/web"
	"github.com/mbolli/tickit/internal/wiki"
	"github.com/mbolli/tickit/migrations"
)

// The one store of every area's parts, as cmd/tickit binds it.
var bindStores = sync.OnceValue(func() *events.Store {
	st := events.Must(events.New(append(tracker.Parts(), wiki.Parts()...)...))
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
	log := slog.New(slog.NewTextHandler(s.logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx := context.Background()
	db, err := sqlite.Open(ctx, s.path, sqlite.Options{Synchronous: "NORMAL", Migrations: migrations.FS, Readers: runtime.GOMAXPROCS(0) + 4, Checkpoint: sqlite.CheckpointWriter})
	if err != nil {
		t.Fatal(err)
	}
	hub := live.New(live.Options{Read: db.R, MaxStreams: 400, HeartbeatSignal: "_ping", Log: log})
	w, err := writer.New(db, writer.Options{OnCommit: hub.Changed, Log: log})
	if err != nil {
		t.Fatal(err)
	}
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

// seedAll writes the seed's people, projects, issues and wiki into the
// server: what the demo's seed does, enough for pages to show chips.
func (s *realServer) seedAll() {
	t := s.t
	t.Helper()
	c, err := seed.Load()
	if err != nil {
		t.Fatal(err)
	}
	run := time.Now()
	if err := s.do(people.SeedPeople{At: run.Add(-31 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	n := 0
	by := func(id string) tabs.By {
		n++
		for _, p := range people.Seeded {
			if p.ID == id {
				return tabs.By{Actor: p, Op: "s" + strconv.Itoa(n), At: run.Add(-30 * 24 * time.Hour)}
			}
		}
		return tabs.By{Actor: people.Seeded[0], Op: "s" + strconv.Itoa(n), At: run.Add(-30 * 24 * time.Hour)}
	}
	projects := map[string]string{}
	for _, p := range c.Projects {
		var id string
		b, err := s.w.DoValue(context.Background(), func(tx *writer.Tx) ([]byte, error) {
			return tracker.CreateProject{By: by(p.Lead), Key: p.Key, Name: p.Name, Description: p.Description, Open: true}.ApplyResult(tx)
		})
		if err != nil {
			t.Fatal(err)
		}
		id = jsonField(b, "id")
		projects[p.Key] = id
	}
	issues := map[string]string{}
	for _, is := range c.Issues {
		cmd := tracker.CreateIssue{By: by(is.Reporter), ID: tracker.NewID(), Project: projects[is.Project], Type: tracker.IssueType(strings.ToLower(is.Type)), Summary: is.Summary, Status: statusID(is.Status), Parent: issues[is.Parent]}
		issues[is.Key] = cmd.ID
		if cmd.Type == tracker.Subtask && cmd.Parent == "" {
			cmd.Type = tracker.Task
		}
		if err := s.do(cmd); err != nil {
			cmd.Parent = ""
			if cmd.Type == tracker.Subtask {
				cmd.Type = tracker.Task
			}
			cmd.Status = ""
			if err := s.do(cmd); err != nil {
				t.Fatalf("%s: %v", is.Key, err)
			}
		}
	}
	cmds, err := wiki.SeedCommands(c, run)
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range cmds {
		if err := s.do(cmd); err != nil {
			t.Fatalf("%T: %v", cmd, err)
		}
	}
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
