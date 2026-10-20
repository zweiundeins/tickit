package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// transport is every simulated browser's: connections are kept, nothing
// is decompressed behind the client's back.
var transport = &http.Transport{
	MaxIdleConns:        2000,
	MaxIdleConnsPerHost: 2000,
	IdleConnTimeout:     90 * time.Second,
	DisableCompression:  true,
}

// browserAccept is what Chromium sends with a document's request.
const browserAccept = "gzip, deflate, br, zstd"

// tab is one browser tab: its own session (a cookie jar), its own client
// address (X-Real-IP, as Caddy reports it from loopback), the document it
// loaded and the stream that document opens.
type tab struct {
	kind   string
	n      int // its number among all tabs
	c      *http.Client
	addr   string
	base   string // the server's URL
	path   string // the document
	id     string // _tab
	views  string // _views
	coding string // the stream's Accept-Encoding: "identity" or "zstd"
	slow   bool   // Save-Data: on (Spec.Slow)

	onEvent func(t *tab, ev *event) // the kind's reading of a frame

	mu        sync.Mutex
	regions   map[string]*region // frames by the id of the element they patch
	opened    time.Time          // when the stream's request went out
	firstAt   time.Time          // when its first frame came
	status    int                // the stream's last answer
	connected chan struct{}      // closed at the first frame of each connection
	frames    atomic.Int64
	bytes     atomic.Int64 // bytes of frames, uncompressed
	wire      atomic.Int64 // bytes on the wire

	reconnects atomic.Int64
	stop       atomic.Bool
}

// region counts the frames of one patched element: how many, how big, and
// the shortest time between two (NFR-37.2: a slow link gets at most four
// a second).
type region struct {
	frames  int64
	bytes   int64
	maxSize int
	last    time.Time
	minGap  time.Duration
	gaps    []time.Duration
}

type event struct {
	kind string // datastar-patch-elements, datastar-patch-signals, ...
	data string // the data lines, without "data: ", joined by newlines
	size int    // bytes of the event as sent, before compression
	at   time.Time
}

var (
	tabRe   = regexp.MustCompile(`_tab: &#34;([^&]+)&#34;, _views: &#34;([^&]*)&#34;`)
	firstID = regexp.MustCompile(`^elements <[a-z0-9-]+ [^>]*?\bid="([^"]+)"`)
)

func newTab(kind string, n int, base string) *tab {
	jar, _ := cookiejar.New(nil)
	return &tab{kind: kind, n: n, base: base, c: &http.Client{Transport: transport, Jar: jar},
		addr: fmt.Sprintf("10.%d.%d.%d", 1+n/60000, n/250%250, n%250+1), regions: map[string]*region{}, coding: "identity"}
}

// load gets the tab's document as a browser does and reads its tab id and
// views. It returns how long the document took, to its last byte.
func (t *tab) load(ctx context.Context, path string) (time.Duration, []byte, error) {
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, "GET", t.base+path, nil)
	req.Header.Set("Accept-Encoding", browserAccept)
	req.Header.Set("X-Real-IP", t.addr)
	res, err := t.c.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	body, err := decode(res)
	if err != nil {
		return 0, nil, err
	}
	took := time.Since(start)
	if res.StatusCode != http.StatusOK {
		return took, body, fmt.Errorf("GET %s: %d", path, res.StatusCode)
	}
	m := tabRe.FindSubmatch(body)
	if m == nil {
		return took, body, fmt.Errorf("GET %s: no stream in the document", path)
	}
	t.path, t.id, t.views = res.Request.URL.Path, string(m[1]), html.UnescapeString(string(m[2]))
	return took, body, nil
}

// decode reads a response's body in the encoding the server chose.
func decode(res *http.Response) ([]byte, error) {
	switch res.Header.Get("Content-Encoding") {
	case "":
		return io.ReadAll(res.Body)
	case "zstd":
		zr, err := zstd.NewReader(res.Body, zstd.WithDecoderConcurrency(1))
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		return io.ReadAll(zr)
	case "br":
		return io.ReadAll(brotli.NewReader(res.Body))
	case "gzip":
		zr, err := gzip.NewReader(res.Body)
		if err != nil {
			return nil, err
		}
		return io.ReadAll(zr)
	default:
		b, err := io.ReadAll(res.Body)
		return b, errors.Join(err, fmt.Errorf("content encoding %q not read", res.Header.Get("Content-Encoding")))
	}
}

// post sends an action as Datastar does and returns its status and how
// long the answer took.
func (t *tab) post(path, body string) (int, time.Duration) {
	req, _ := http.NewRequest("POST", t.base+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("X-Real-IP", t.addr)
	start := time.Now()
	res, err := t.c.Do(req)
	if err != nil {
		return 0, time.Since(start)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	return res.StatusCode, time.Since(start)
}

func (t *tab) streamURL() string {
	return t.base + "/stream?tab=" + url.QueryEscape(t.id) + "&v=" + url.QueryEscape(t.views)
}

// open opens the tab's stream and reads it until it ends; then, as
// Datastar's retry does (retry 'error', retryMaxWait 10 s), it opens it
// again after 1, 2, 4, 8 and then 10 s, until stop.
func (t *tab) run(ctx context.Context) {
	wait := time.Duration(0)
	for !t.stop.Load() && ctx.Err() == nil {
		if wait > 0 {
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return
			}
		}
		ok := t.once(ctx)
		if t.stop.Load() || ctx.Err() != nil {
			return
		}
		t.reconnects.Add(1)
		switch {
		case ok:
			wait = 0 // the stream ended after frames: the page opens it again at once
		case wait == 0:
			wait = time.Second
		default:
			wait = min(2*wait, 10*time.Second)
		}
	}
}

// once is one connection of the stream; it reports whether frames came.
func (t *tab) once(ctx context.Context) bool {
	req, _ := http.NewRequestWithContext(ctx, "GET", t.streamURL(), nil)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Accept-Encoding", t.coding)
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("X-Real-IP", t.addr)
	if t.slow {
		req.Header.Set("Save-Data", "on")
	}
	t.mu.Lock()
	t.opened = time.Now()
	t.mu.Unlock()
	res, err := t.c.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	t.mu.Lock()
	t.status = res.StatusCode
	t.mu.Unlock()
	if res.StatusCode != http.StatusOK {
		io.Copy(io.Discard, res.Body)
		return false
	}
	var body io.Reader = &counting{r: res.Body, n: &t.wire}
	if res.Header.Get("Content-Encoding") == "zstd" {
		zr, err := zstd.NewReader(body, zstd.WithDecoderConcurrency(1))
		if err != nil {
			return false
		}
		defer zr.Close()
		body = zr
	}
	return t.read(body)
}

type counting struct {
	r io.Reader
	n *atomic.Int64
}

func (c *counting) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}

// read parses the stream's events.
func (t *tab) read(r io.Reader) bool {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	var ev event
	var data strings.Builder
	got := false
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if ev.kind == "" {
				continue
			}
			ev.data, ev.at = data.String(), time.Now()
			if !got {
				got = true
				t.mu.Lock()
				t.firstAt = ev.at
				ch := t.connected
				t.mu.Unlock()
				if ch != nil {
					select {
					case <-ch:
					default:
						close(ch)
					}
				}
			}
			t.frame(&ev)
			ev, data = event{}, strings.Builder{}
			continue
		}
		ev.size += len(line) + 1
		if k, ok := strings.CutPrefix(line, "event: "); ok {
			ev.kind = k
		} else if d, ok := strings.CutPrefix(line, "data: "); ok {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(d)
		}
	}
	return got
}

func (t *tab) frame(ev *event) {
	t.frames.Add(1)
	t.bytes.Add(int64(ev.size))
	if ev.kind == "datastar-patch-elements" {
		var m []string
		if i := strings.Index(ev.data, "elements <"); i >= 0 {
			m = firstID.FindStringSubmatch(ev.data[i:])
		}
		if m != nil {
			t.mu.Lock()
			g := t.regions[m[1]]
			if g == nil {
				g = &region{}
				t.regions[m[1]] = g
			}
			g.frames++
			g.bytes += int64(ev.size)
			g.maxSize = max(g.maxSize, ev.size)
			if !g.last.IsZero() {
				gap := ev.at.Sub(g.last)
				if g.minGap == 0 || gap < g.minGap {
					g.minGap = gap
				}
				g.gaps = append(g.gaps, gap)
			}
			g.last = ev.at
			t.mu.Unlock()
		}
	}
	if t.onEvent != nil {
		t.onEvent(t, ev)
	}
}

// expect returns a channel closed at the first frame of the next
// connection.
func (t *tab) expect() chan struct{} {
	ch := make(chan struct{})
	t.mu.Lock()
	t.connected = ch
	t.mu.Unlock()
	return ch
}
