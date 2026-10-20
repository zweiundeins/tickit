package webtest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
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
// the session cookies the server sets.
type recorder struct{}

// secretFields are the fields of an action's body that hold a password.
var secretFields = []string{"password", "current"}

func (recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil && req.Method == http.MethodPost {
		b, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(b))
		var m map[string]any
		if json.Unmarshal(b, &m) == nil {
			for _, f := range secretFields {
				if s, ok := m[f].(string); ok {
					Secret(s)
				}
			}
		}
	}
	for _, c := range req.Cookies() {
		Secret(c.Value)
	}
	res, err := http.DefaultTransport.RoundTrip(req)
	if err == nil {
		for _, c := range res.Cookies() {
			Secret(c.Value)
		}
	}
	return res, err
}
