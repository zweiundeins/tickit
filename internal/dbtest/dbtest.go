// Package dbtest runs commands against a fresh database for tests: a
// temporary SQLite file with every migration, a writer whose commits are
// recorded by topic, and an events runner when a test needs projectors.
// It is the harness of lith's site (demotest), for tickit.
package dbtest

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/migrations"
)

// Env is a database and its writer.
type Env struct {
	DB *sqlite.DB
	W  *writer.Writer

	mu     sync.Mutex
	topics []string
}

// Open migrates a new database in t's temporary directory and runs a
// writer on it, until the test ends.
func Open(t testing.TB) *Env {
	t.Helper()
	db, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"), sqlite.Options{
		Synchronous: "NORMAL",
		Migrations:  migrations.FS,
	})
	if err != nil {
		t.Fatal(err)
	}
	e := &Env{DB: db}
	e.W, err = writer.New(db, writer.Options{OnCommit: func(ts []string) {
		e.mu.Lock()
		e.topics = append(e.topics, ts...)
		e.mu.Unlock()
	}})
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	go e.W.Run(context.Background())
	t.Cleanup(func() {
		e.W.Close()
		db.Close()
	})
	return e
}

// Do runs cmd and fails the test on its error.
func (e *Env) Do(t testing.TB, cmd writer.Command) {
	t.Helper()
	if err := e.W.Do(context.Background(), cmd); err != nil {
		t.Fatalf("%T: %v", cmd, err)
	}
}

// View runs fn in a read transaction and fails the test on its error.
func (e *Env) View(t testing.TB, fn func(tx *sql.Tx) error) {
	t.Helper()
	if err := e.DB.View(context.Background(), fn); err != nil {
		t.Fatal(err)
	}
}

// Touched returns the topics committed since its last call, sorted, each
// once: the views the commands will render again.
func (e *Env) Touched() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	got := e.topics
	e.topics = nil
	slices.Sort(got)
	return slices.Compact(got)
}

// Run starts an events runner of st on the database, stops it before the
// writer closes, and waits until every projector is live.
func (e *Env) Run(t testing.TB, st *events.Store) *events.Runner {
	t.Helper()
	r := events.NewRunner(st, e.DB, e.W, events.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := r.Run(ctx); err != nil {
			t.Errorf("events runner: %v", err)
		}
	}()
	t.Cleanup(func() { // before the writer closes: cleanups run last first
		cancel()
		<-done
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		live := true
		for _, p := range r.Stats().Projectors {
			live = live && p.Status == "live"
		}
		if live {
			return r
		}
		if time.Now().After(deadline) {
			t.Fatalf("projectors not live after 5s: %+v", r.Stats().Projectors)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
