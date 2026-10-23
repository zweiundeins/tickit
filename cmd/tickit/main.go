// Command tickit serves tickit, an issue tracker and wiki for several
// people at once: one binary, one SQLite file.
//
//	tickit [serve] [flags]       run the server (the default)
//	tickit backup [flags]        copy the database while the server runs, and keep the newest copies
//	tickit account create [flags] make a staff account; the password comes from TICKIT_PASSWORD or stdin
//	tickit seed [flags]          write the demo's seed into a new database (-force empties one first)
//	tickit migrations-pending [flags] print how many migrations the database has not had yet
//	tickit --version             print the version
//
// Every flag can also come from the environment: -db from TICKIT_DB, -addr
// from TICKIT_ADDR. The command line wins. No setting is a secret, so
// none has a default that would be one (NFR-05).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/zweiundeins/lith/edge"
	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/prom"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/notify"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/recent"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/web"
	"github.com/zweiundeins/tickit/internal/wiki"
	"github.com/zweiundeins/tickit/migrations"
)

// version is set at build time: -ldflags "-X main.version=v1.2.3".
var version string

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println("tickit", buildVersion())
		return
	}
	cmd, args := "serve", os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(args)
	case "backup":
		err = backup(args)
	case "account":
		err = account(args)
	case "seed":
		err = seedCommand(args)
	case "migrations-pending":
		err = migrationsPending(args, os.Stdout)
	default:
		err = fmt.Errorf("unknown command %q: serve, backup, account, seed or migrations-pending", cmd)
	}
	if err != nil {
		slog.Error(cmd, "err", err)
		os.Exit(1)
	}
}

// parts are the projectors and upcasters of every domain package, for the
// one event store: append each area's here.
func parts() []events.Part {
	var p []events.Part
	p = append(p, tracker.Parts()...)
	p = append(p, notify.Parts()...)
	p = append(p, wiki.Parts()...)
	return p
}

func serve(args []string) error {
	f := flags("serve")
	addr := f.String("addr", "127.0.0.1:7105", "listen on host:port, or on a Unix socket as unix:/path (go tool task dev uses 127.0.0.1:8080)")
	metricsAddr := f.String("metrics", "127.0.0.1:7205", "serve Prometheus metrics on this loopback host:port, empty for none")
	dbPath := f.String("db", "data/tickit.db", "the SQLite database")
	synchronous := f.String("synchronous", "NORMAL", "NORMAL (a power loss can undo up to about 35 s of commits), or FULL (one fsync per batch, loses none)")
	streams := f.Int("streams", 200, "how many streams may be open at once; more get 503")
	sessionTTL := f.Duration("session-ttl", people.DefaultSessionTTL, "how long a login lasts")
	secure := f.Bool("secure", false, "served over HTTPS: a __Host- cookie and HSTS")
	dev := f.Bool("dev", false, "development: serve the static files as written, not minified, and seed an empty database")
	debugLog := f.Bool("debug", false, "log at debug level")
	if err := parse(f, args); err != nil {
		return err
	}
	level := slog.LevelInfo
	if *debugLog {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)
	if err := loopback(*metricsAddr); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := events.New(parts()...)
	if err != nil {
		return err
	}
	tracker.Bind(store)
	wiki.Bind(store)
	db, err := sqlite.Open(ctx, *dbPath, sqlite.Options{
		Synchronous: *synchronous,
		Migrations:  migrations.FS,
		Readers:     runtime.GOMAXPROCS(0) + 4, // the render workers hold one each, GETs need the rest
		Checkpoint:  sqlite.CheckpointWriter,
	})
	if err != nil {
		return err
	}
	hub := live.New(live.Options{Read: db.R, MaxStreams: *streams, MaxStreamsPerClient: web.MaxStreamsPerAddr, HeartbeatSignal: "_ping", Log: log,
		OnTick: func(d time.Duration) { tickSeconds.Observe(d.Seconds()) }, Stopping: web.Stopping})
	w, err := writer.New(db, writer.Options{OnCommit: hub.Changed, Log: log})
	if err != nil {
		return errors.Join(err, db.Close())
	}
	notify.Bind(w) // the reactions' commands
	runner := events.NewRunner(store, db, w, events.Options{Log: log})
	var ln, metricsLn net.Listener
	site, err := web.New(web.Config{DB: db, W: w, Hub: hub, Store: store, Log: log, Dev: *dev, Secure: *secure, SessionTTL: *sessionTTL, Projectors: runner.Stats})
	if err == nil {
		ln, err = listen(*addr)
	}
	if err == nil && *metricsAddr != "" {
		if metricsLn, err = net.Listen("tcp", *metricsAddr); err != nil {
			ln.Close()
		}
	}
	if err != nil {
		return errors.Join(err, w.Close(), db.Close())
	}

	var writerUp, hubUp, runnerUp atomic.Bool
	ready := &edge.Ready{Log: log}
	ready.Add("database", func(ctx context.Context) error {
		var one int
		return db.R.QueryRowContext(ctx, "SELECT 1").Scan(&one)
	})
	ready.Add("writer", running(&writerUp)) // never the writer itself: a probe must not queue behind it
	ready.Add("hub", running(&hubUp))
	ready.Add("events", running(&runnerUp))
	ready.Add("static", site.Static().Warmed) // until then, a file not compressed yet waits for it
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", edge.Health(buildVersion()))
	mux.Handle("GET /readyz", ready)
	mux.Handle("/", site.Handler()) // probes skip the edge: no cookie, no log line

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
		// No WriteTimeout: a stream gives each of its writes a deadline.
	}

	// The writer runs until Close, so actions still write while the HTTP
	// server drains. Cancelling the hub's context ends every stream at once.
	writerUp.Store(true)
	go func() {
		w.Run(context.Background())
		writerUp.Store(false)
	}()
	hubCtx, stopHub := context.WithCancel(context.Background())
	defer stopHub()
	hubDone := make(chan struct{})
	hubUp.Store(true)
	go func() {
		defer close(hubDone)
		defer hubUp.Store(false)
		if err := hub.Run(hubCtx); err != nil {
			log.Error("render loop", "err", err)
			stop()
		}
	}()
	// The events runner keeps the projections current, on a context of
	// its own: it stops before the writer closes.
	runCtx, stopRunner := context.WithCancel(context.Background())
	defer stopRunner()
	runnerDone := make(chan struct{})
	runnerUp.Store(true)
	go func() {
		defer close(runnerDone)
		defer runnerUp.Store(false)
		if err := runner.Run(runCtx); err != nil {
			log.Error("events runner", "err", err) // e.g. a projector's table is missing
			stop()
		}
	}()
	if *dev {
		if err := seedIfEmpty(ctx, db, w, site.Demo(), log); err != nil {
			log.Error("seed", "err", err)
		}
	}
	var jobs sync.WaitGroup
	jobs.Go(func() { sweep(ctx, w, log) })
	jobs.Go(func() { site.WikiJobs(ctx) })
	jobs.Go(func() { site.PresenceJobs(ctx) }) // who turned away (MP-02.4)
	jobs.Go(func() { site.Demo().Run(ctx) })   // the nightly reset (DM-03)

	var metricsSrv *http.Server
	if metricsLn != nil {
		metricsSrv = metricsServer(w, hub, runner, store, site, log)
		go metricsSrv.Serve(metricsLn)
	}

	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	log.Info("serving", "addr", *addr, "metrics", *metricsAddr, "db", *dbPath, "version", buildVersion())
	select {
	case err = <-served:
	case <-ctx.Done():
	}
	stop() // the jobs end; a second signal kills the process

	log.Info("shutting down")
	stopHub() // 1. every stream ends, so Shutdown does not wait for them
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil { // 2. actions finish, and still reach the writer
		log.Warn("shutdown", "err", err)
	}
	if metricsSrv != nil {
		metricsSrv.Shutdown(sctx)
	}
	jobs.Wait()
	<-hubDone
	stopRunner() // 3. the projectors stop, before the writer closes
	<-runnerDone
	return errors.Join(err, w.Close(), db.Close()) // 4. the writer applies what is queued; 5. the database closes
}

func running(up *atomic.Bool) func(context.Context) error {
	return func(context.Context) error {
		if !up.Load() {
			return errors.New("not running")
		}
		return nil
	}
}

// loopback refuses a metrics address that is not on loopback: the
// numbers are for the host's Prometheus, not for the public.
func loopback(addr string) error {
	if addr == "" {
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("-metrics %q: %w", addr, err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("-metrics %q: serve metrics on a loopback address only", addr)
	}
	return nil
}

// tickSeconds are the render loop's ticks (NFR-35): the time from a
// commit to its frame is the wait for the next tick plus one of these.
var tickSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{Namespace: "tickit", Name: "tick_seconds",
	Help:    "How long each tick that rendered views took.",
	Buckets: []float64{0.001, 0.002, 0.005, 0.01, 0.02, 0.03, 0.05, 0.075, 0.1, 0.2, 0.5}})

// debugHandlers adds pprof to the metrics listener in a binary built with
// -tags pprof (pprof.go), for load tests; never in the deployed one.
var debugHandlers func(*http.ServeMux)

// metricsServer serves Prometheus' text format at /metrics: the Go
// runtime, the process, lith's writer, hub, stream budget, event store and
// runner, and tickit's own numbers (NFR-76).
func metricsServer(w *writer.Writer, hub *live.Hub, runner *events.Runner, store *events.Store, site *web.Server, log *slog.Logger) *http.Server {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		prom.New("tickit", prom.Sources{Writer: w, Hub: hub, Events: runner, Store: store}),
		tickSeconds,
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: "tickit", Name: "build_info", Help: "1, labelled with the running version.",
			ConstLabels: prometheus.Labels{"version": buildVersion()},
		}, func() float64 { return 1 }),
	)
	reg.MustRegister(site.Collectors()...)
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelWarn)}))
	if debugHandlers != nil {
		debugHandlers(mux)
	}
	return &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
}

// sweep keeps the tables that grow bounded, every hour: the rows of tabs
// gone for 3 days, expired login sessions, the old security log, old
// notifications (FR-117), old views (FR-59.1) and old op ids.
func sweep(ctx context.Context, w *writer.Writer, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		now := time.Now()
		for _, cmd := range []writer.Command{tabs.Sweep{At: now}, people.Sweep{At: now}, notify.Sweep{At: now}, recent.Sweep{At: now}, writer.PruneOnce(24 * time.Hour)} {
			if err := w.Do(ctx, cmd); err != nil && ctx.Err() == nil {
				log.Warn("sweep", "err", err)
			}
		}
	}
}

// backup writes a consistent copy of the database (VACUUM INTO, safe while
// the server runs), checks it, and deletes the oldest copies beyond -keep.
// A copy that fails the check is kept as .bad, and no copy is deleted.
func backup(args []string) error {
	f := flags("backup")
	dbPath := f.String("db", "data/tickit.db", "the SQLite database")
	dir := f.String("backups", "data/backups", "the directory of the copies")
	keep := f.Int("keep", 30, "how many copies to keep")
	if err := parse(f, args); err != nil {
		return err
	}
	if *keep < 1 {
		return errors.New("-keep must be 1 or more")
	}
	if _, err := os.Stat(*dbPath); err != nil {
		return err // sqlite.Open would create an empty database
	}
	if err := os.MkdirAll(*dir, 0o700); err != nil {
		return err
	}
	ctx := context.Background()
	db, err := sqlite.Open(ctx, *dbPath, sqlite.Options{Readers: 1})
	if err != nil {
		return err
	}
	defer db.Close()
	to := filepath.Join(*dir, "tickit-"+time.Now().UTC().Format("20060102T150405Z")+".db")
	if err := db.Backup(ctx, to); err != nil {
		return err
	}
	if err := checkCopy(ctx, to); err != nil {
		return errors.Join(fmt.Errorf("backup %s: %w", to, err), os.Rename(to, to+".bad"))
	}
	copies, err := filepath.Glob(filepath.Join(*dir, "tickit-*.db"))
	if err != nil {
		return err
	}
	slices.Sort(copies) // oldest first: the names hold the time
	for _, old := range copies[:max(0, len(copies)-*keep)] {
		if err := os.Remove(old); err != nil {
			return err
		}
	}
	slog.Info("backup", "to", to, "copies", min(len(copies), *keep))
	return nil
}

// listen opens host:port, or unix:/path: a Unix socket for a proxy that runs
// as another user, where the socket's directory decides who may connect.
func listen(addr string) (net.Listener, error) {
	path, ok := strings.CutPrefix(addr, "unix:")
	if !ok {
		return net.Listen("tcp", addr)
	}
	os.Remove(path) // left by a process that did not exit cleanly
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o666); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

func flags(name string) *flag.FlagSet {
	f := flag.NewFlagSet(name, flag.ExitOnError)
	f.Usage = func() {
		fmt.Fprintf(f.Output(), "usage: tickit %s [flags]\n\nEach flag can also be set in the environment, -db as TICKIT_DB.\n\n", name)
		f.PrintDefaults()
	}
	return f
}

// parse reads the flags, then takes each one the command line left out from
// the environment: -addr from TICKIT_ADDR, -db from TICKIT_DB.
func parse(f *flag.FlagSet, args []string) error {
	if err := f.Parse(args); err != nil {
		return err
	}
	given := map[string]bool{}
	f.Visit(func(fl *flag.Flag) { given[fl.Name] = true })
	var errs []error
	f.VisitAll(func(fl *flag.Flag) {
		key := "TICKIT_" + strings.ToUpper(strings.ReplaceAll(fl.Name, "-", "_"))
		if v, ok := os.LookupEnv(key); ok && !given[fl.Name] {
			if err := fl.Value.Set(v); err != nil {
				errs = append(errs, fmt.Errorf("%s=%q: %w", key, v, err))
			}
		}
	})
	return errors.Join(errs...)
}

// buildVersion is version, or else the commit the binary was built from.
func buildVersion() string {
	if version != "" {
		return version
	}
	rev, dirty := "dev", ""
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch {
			case s.Key == "vcs.revision" && len(s.Value) >= 12:
				rev = s.Value[:12]
			case s.Key == "vcs.modified" && s.Value == "true":
				dirty = "-dirty"
			}
		}
	}
	return rev + dirty
}
