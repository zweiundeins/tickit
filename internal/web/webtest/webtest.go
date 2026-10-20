// Package webtest runs tickit's server in process for tests, behind the
// whole edge, and talks to it as a browser does: a page, a stream read
// event by event, actions with Datastar's and Sec-Fetch-Site's headers.
// It also tells whether requests wrote to the database (Fingerprint).
package webtest

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/web"
	"github.com/mbolli/tickit/migrations"
)

// Options change how Start runs the server.
type Options struct {
	Store      *events.Store // the event store; an empty one by default
	SessionTTL time.Duration // how long a login lasts; 24 hours by default
	Secure     bool          // as over HTTPS: a __Host- cookie and HSTS
	// Configure changes the server before it serves (a test's limits).
	Configure func(*web.Server)
	// StreamWait is web.Config.StreamWait; none by default, since a
	// Client opens a tab's stream when an action finds none.
	StreamWait time.Duration
}

// Env is a running server.
type Env struct {
	URL    string
	DB     *sqlite.DB
	W      *writer.Writer
	Hub    *live.Hub
	Server *web.Server

	logs    *syncBuffer
	stopHub context.CancelFunc
	hubDone chan struct{}
}

// Start serves tickit on a new database until the test ends.
func Start(t testing.TB) *Env { return StartWith(t, Options{}) }

// StartWith serves tickit with o.
func StartWith(t testing.TB, o Options) *Env {
	t.Helper()
	if o.Store == nil {
		o.Store = events.Must(events.New())
	}
	db, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"), sqlite.Options{
		Synchronous: "NORMAL",
		Migrations:  migrations.FS,
	})
	if err != nil {
		t.Fatal(err)
	}
	logs := &syncBuffer{}
	log := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	hub := live.New(live.Options{Read: db.R, Workers: 2, Tick: 10 * time.Millisecond, MaxStreamsPerClient: web.MaxStreamsPerAddr, HeartbeatSignal: "_ping", Log: log})
	w, err := writer.New(db, writer.Options{OnCommit: hub.Changed, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	if o.StreamWait == 0 {
		o.StreamWait = -1
	}
	site, err := web.New(web.Config{DB: db, W: w, Hub: hub, Store: o.Store, Log: log, SessionTTL: o.SessionTTL, Secure: o.Secure, StreamWait: o.StreamWait})
	if err != nil {
		t.Fatal(err)
	}
	if o.Configure != nil {
		o.Configure(site)
	}
	e := &Env{DB: db, W: w, Hub: hub, Server: site, logs: logs, hubDone: make(chan struct{})}
	var hubCtx context.Context
	hubCtx, e.stopHub = context.WithCancel(context.Background())
	go func() {
		defer close(e.hubDone)
		hub.Run(hubCtx)
	}()
	go w.Run(context.Background())
	srv := httptest.NewServer(site.Handler())
	e.URL = srv.URL
	t.Cleanup(func() {
		e.StopStreams()
		srv.Close()
		w.Close()
		db.Close()
		checkSecrets(t, logs.String()) // NFR-11
		if t.Failed() {
			t.Logf("server log:\n%s", logs.String())
		}
	})
	return e
}

// StopStreams stops the hub, as a deploy does: every stream ends with
// sse.Reconnect.
func (e *Env) StopStreams() {
	e.stopHub()
	<-e.hubDone
}

// Logs is what the server logged so far.
func (e *Env) Logs() string { return e.logs.String() }

// Do runs a command on the server's writer and fails the test on its error.
func (e *Env) Do(t testing.TB, cmd writer.Command) {
	t.Helper()
	if err := e.W.Do(context.Background(), cmd); err != nil {
		t.Fatalf("%T: %v", cmd, err)
	}
}

// Account makes an active staff account and returns its person id.
func (e *Env) Account(t testing.TB, email, password string, role people.SiteRole) string {
	t.Helper()
	h, err := people.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	Secret(password)
	id := people.NewAccountID()
	e.Do(t, people.CreateAccount{ID: id, Email: email, Username: strings.Split(email, "@")[0], Role: role, PasswordHash: h, At: time.Now()})
	return id
}

// Login logs c in and fails the test unless the answer sends the page on.
func (e *Env) Login(t testing.TB, c *http.Client, email, password string) {
	t.Helper()
	code, body := Post(t, c, e.URL+"/cmd/login", fmt.Sprintf(`{"email":%q,"password":%q,"next":"/"}`, email, password))
	if code != http.StatusOK || !strings.Contains(body, `"_goto":"/"`) {
		t.Fatalf("login: %d %s", code, body)
	}
}

// Fingerprint is a hash of every table's rows and of the schema: equal
// fingerprints mean nothing was written in between (NFR-06).
func (e *Env) Fingerprint(t testing.TB) string {
	t.Helper()
	h := sha256.New()
	err := e.DB.View(context.Background(), func(tx *sql.Tx) error {
		var tables []string
		rows, err := tx.Query(`SELECT name FROM sqlite_schema WHERE type = 'table' ORDER BY name`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				rows.Close()
				return err
			}
			tables = append(tables, name)
		}
		rows.Close()
		for _, table := range append([]string{"sqlite_schema"}, tables...) {
			lines, err := dump(tx, table)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "%s\n%s\n", table, strings.Join(lines, "\n"))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// dump returns a table's rows as text, sorted.
func dump(tx *sql.Tx, table string) ([]string, error) {
	rows, err := tx.Query(`SELECT * FROM "` + strings.ReplaceAll(table, `"`, `""`) + `"`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var lines []string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		lines = append(lines, fmt.Sprintf("%q", vals))
	}
	slices.Sort(lines)
	return lines, rows.Err()
}

// Client is a browser's cookie jar, which follows no redirect. Like a
// page, it has a stream open for the tab it acts from: before an action
// from a tab without one of its streams, it opens the tab's stream until
// the test ends, and so it does when an action answers web.NotConnected
// (a login changed who the session is), then sending the action again.
func Client(t testing.TB) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &http.Client{Jar: jar, Transport: &recorder{ctx: ctx, open: map[string]int{}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// Get requests url and returns the status and the body.
func Get(t testing.TB, c *http.Client, url string) (int, string) {
	t.Helper()
	res, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// Post sends an action the way Datastar does, from the page's own site,
// and returns the status and the body.
func Post(t testing.TB, c *http.Client, url, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// Listen opens a stream, with header on its request, and sends each of its
// events to the channel until the test ends.
func Listen(t testing.TB, c *http.Client, url string, header http.Header) <-chan string {
	t.Helper()
	events, _ := Open(t, c, url, header)
	return events
}

// Open is Listen with a function that closes the stream, as a tab that
// goes away does.
func Open(t testing.TB, c *http.Client, url string, header http.Header) (<-chan string, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("Accept-Encoding", "identity")
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("stream %s: %d %s", url, res.StatusCode, b)
	}
	events := make(chan string)
	go func() {
		defer res.Body.Close()
		defer close(events)
		lines := bufio.NewScanner(res.Body)
		lines.Buffer(nil, 1<<20)
		var event strings.Builder
		for lines.Scan() {
			if lines.Text() != "" {
				event.WriteString(lines.Text() + "\n")
				continue
			}
			if event.Len() > 0 {
				select {
				case events <- event.String():
				case <-ctx.Done():
					return
				}
				event.Reset()
			}
		}
	}()
	return events, cancel
}

// Next skips events until one contains want, and returns it.
func Next(t testing.TB, events <-chan string, want string) string {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatalf("the stream ended before an event with %q", want)
			}
			if strings.Contains(event, want) {
				return event
			}
		case <-timeout:
			t.Fatalf("no event with %q in 5s", want)
		}
	}
}

// Silent fails the test when an event with unwanted arrives within d.
func Silent(t testing.TB, events <-chan string, unwanted string, d time.Duration) {
	t.Helper()
	timeout := time.After(d)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			if strings.Contains(event, unwanted) {
				t.Fatalf("an event with %q: %s", unwanted, event)
			}
		case <-timeout:
			return
		}
	}
}

// syncBuffer is a log that several goroutines write.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}
