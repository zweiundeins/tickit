package e2e

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// delayProxy is a link with a round trip of 2×d in front of a server, for
// MP-05.3's 50 ms: every request reaches the server d after the browser
// sent it, and every write of an answer, each frame of a stream too,
// reaches the browser d after the server wrote it. The writes keep their
// order and their spacing: a delay line, not a queue that adds up.
type delayProxy struct {
	target *url.URL
	d      time.Duration
	tr     *http.Transport
	URL    string
}

// newDelayProxy serves the proxy until the test ends.
func newDelayProxy(t testing.TB, target string, d time.Duration) *delayProxy {
	t.Helper()
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	p := &delayProxy{target: u, d: d, tr: &http.Transport{DisableCompression: true, MaxIdleConnsPerHost: 200, ForceAttemptHTTP2: false}}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: p}
	go srv.Serve(ln)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		srv.Shutdown(ctx)
		srv.Close()
		p.tr.CloseIdleConnections()
	})
	p.URL = "http://" + ln.Addr().String()
	return p
}

type chunk struct {
	at time.Time
	b  []byte
}

func (p *delayProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sent := time.Now()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	time.Sleep(time.Until(sent.Add(p.d))) // the way there
	out, err := http.NewRequestWithContext(r.Context(), r.Method, p.target.Scheme+"://"+p.target.Host+r.URL.RequestURI(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	out.Header = r.Header.Clone()
	out.Host = r.Host // the page's origin, as the edge's CSRF check reads it
	if len(body) > 0 {
		out.Body, out.ContentLength = io.NopCloser(bytes.NewReader(body)), int64(len(body))
	}
	res, err := p.tr.RoundTrip(out)
	if err != nil {
		if r.Context().Err() == nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		return
	}
	defer res.Body.Close()
	answered := time.Now()
	chunks := make(chan chunk, 4096)
	go func() {
		defer close(chunks)
		buf := make([]byte, 64<<10)
		for {
			n, err := res.Body.Read(buf)
			if n > 0 {
				chunks <- chunk{time.Now(), append([]byte(nil), buf[:n]...)}
			}
			if err != nil {
				return
			}
		}
	}()
	time.Sleep(time.Until(answered.Add(p.d))) // the way back
	for k, v := range res.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(res.StatusCode)
	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}
	for c := range chunks {
		time.Sleep(time.Until(c.at.Add(p.d)))
		if _, err := w.Write(c.b); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// TestDelayProxy: the proxy adds its round trip to a request and passes a
// stream's writes on, each d late.
func TestDelayProxy(t *testing.T) {
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		for i := range 3 {
			w.Write([]byte{byte('a' + i)})
			w.(http.Flusher).Flush()
			time.Sleep(50 * time.Millisecond)
		}
	})}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go srv.Serve(ln)
	defer srv.Close()
	p := newDelayProxy(t, "http://"+ln.Addr().String(), 25*time.Millisecond)
	start := time.Now()
	res, err := http.Get(p.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	head := time.Since(start)
	buf := make([]byte, 1)
	var at []time.Duration
	for range 3 {
		if _, err := io.ReadFull(res.Body, buf); err != nil {
			t.Fatal(err)
		}
		at = append(at, time.Since(start))
	}
	res.Body.Close()
	if head < 50*time.Millisecond || head > 90*time.Millisecond || at[2]-at[0] < 90*time.Millisecond || at[2]-at[0] > 130*time.Millisecond {
		t.Errorf("headers after %v, writes at %v", head, at)
	}
}
