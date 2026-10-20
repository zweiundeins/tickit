package webtest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/mbolli/tickit/internal/web"
)

// The secrets of every test: passwords sent and session ids handed out.
// Every server's log is searched for all of them when its test ends, so a
// run of the tests is a test that no log line holds one (NFR-11).
var secrets sync.Map

// Secret adds s to the secrets no log may hold. Values shorter than 8
// bytes are no secrets anyone types and would match by chance.
func Secret(s string) {
	if len(s) >= 8 {
		secrets.Store(s, true)
	}
}

// checkSecrets fails t if log holds a secret.
func checkSecrets(t testing.TB, log string) {
	secrets.Range(func(k, _ any) bool {
		if s := k.(string); strings.Contains(log, s) {
			t.Errorf("NFR-11: the server's log holds the secret %.12q...", s)
		}
		return true
	})
}

// recorder is a client's transport that notes the secrets of its
// requests and answers: the fields of a body that hold passwords, and
// the session cookies the server sets. It counts the client's open
// streams by tab, and opens a tab's stream for an action from a tab
// without one (connect), for as long as ctx lasts.
type recorder struct {
	ctx  context.Context
	mu   sync.Mutex
	open map[string]int // the client's open streams by tab
}

// secretFields are the fields of an action's body that hold a password.
var secretFields = []string{"password", "current"}

func (rec *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	var tab string
	if req.Body != nil && req.Method == http.MethodPost {
		b, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		body = b
		req.Body = io.NopCloser(bytes.NewReader(b))
		var m map[string]any
		if json.Unmarshal(b, &m) == nil {
			for _, f := range secretFields {
				if s, ok := m[f].(string); ok {
					Secret(s)
				}
			}
			tab, _ = m["tab"].(string)
		}
	}
	for _, c := range req.Cookies() {
		Secret(c.Value)
	}
	var minted []string // the session the stream got, for a client without one
	if tab != "" && strings.HasPrefix(req.URL.Path, "/cmd/") && rec.streams(tab) == 0 {
		if cookies, ok := rec.stream(req.URL, req.Header, tab); ok && len(req.Cookies()) == 0 {
			minted = cookies
			for _, c := range (&http.Response{Header: http.Header{"Set-Cookie": cookies}}).Cookies() {
				req.Header.Add("Cookie", c.Name+"="+c.Value)
			}
		}
	}
	res, err := http.DefaultTransport.RoundTrip(req)
	if err == nil && len(minted) > 0 && res.Header.Get("Set-Cookie") == "" {
		res.Header["Set-Cookie"] = minted
	}
	if err == nil {
		for _, c := range res.Cookies() {
			Secret(c.Value)
		}
		if req.Method == http.MethodGet && req.URL.Path == "/stream" && res.StatusCode == http.StatusOK {
			rec.track(req.URL.Query().Get("tab"), res)
		}
	}
	if err != nil || tab == "" || res.StatusCode != http.StatusConflict {
		return res, err
	}
	answer, _ := io.ReadAll(res.Body)
	res.Body.Close()
	res.Body = io.NopCloser(bytes.NewReader(answer))
	if strings.TrimSpace(string(answer)) != web.NotConnected {
		return res, nil
	}
	return rec.connect(req, res, body, tab)
}

// connect opens the stream of the tab that req acts from, with the
// session that req or its answer res carries, and sends req again.
func (rec *recorder) connect(req *http.Request, res *http.Response, body []byte, tab string) (*http.Response, error) {
	header := req.Header.Clone()
	for _, c := range res.Cookies() { // a first request's new session
		header.Add("Cookie", c.Name+"="+c.Value)
	}
	if _, ok := rec.stream(req.URL, header, tab); !ok {
		return res, nil
	}
	again := req.Clone(req.Context())
	again.Header = header
	again.Body = io.NopCloser(bytes.NewReader(body))
	res2, err := http.DefaultTransport.RoundTrip(again)
	if err == nil && res2.Header.Get("Set-Cookie") == "" {
		res2.Header["Set-Cookie"] = res.Header.Values("Set-Cookie")
	}
	return res2, err
}

// stream opens the tab's stream with header's session and client address,
// as the page of the tab would have, and reads it until ctx ends. It
// returns the cookies the stream's answer set: a new session without one.
func (rec *recorder) stream(at *url.URL, header http.Header, tab string) ([]string, bool) {
	u := *at
	u.Path, u.RawQuery = "/stream", url.Values{"tab": {tab}, "v": {""}}.Encode()
	req, err := http.NewRequestWithContext(rec.ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, false
	}
	for _, k := range []string{"Cookie", "X-Forwarded-For", "X-Real-Ip"} {
		req.Header[k] = header[k]
	}
	req.Header.Set("Accept-Encoding", "identity")
	res, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, false
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, false
	}
	for _, c := range res.Cookies() {
		Secret(c.Value)
	}
	rec.track(tab, res)
	go io.Copy(io.Discard, res.Body)
	return res.Header.Values("Set-Cookie"), true
}

// streams is how many of the client's streams of tab are open.
func (rec *recorder) streams(tab string) int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return rec.open[tab]
}

// track counts res, a stream of tab, until its body ends or closes.
func (rec *recorder) track(tab string, res *http.Response) {
	rec.mu.Lock()
	rec.open[tab]++
	rec.mu.Unlock()
	res.Body = &tracked{ReadCloser: res.Body, done: func() {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.open[tab]--
	}}
}

// tracked is a stream's body that calls done once, when it ends.
type tracked struct {
	io.ReadCloser
	once sync.Once
	done func()
}

func (t *tracked) Read(p []byte) (int, error) {
	n, err := t.ReadCloser.Read(p)
	if err != nil {
		t.once.Do(t.done)
	}
	return n, err
}

func (t *tracked) Close() error {
	t.once.Do(t.done)
	return t.ReadCloser.Close()
}
