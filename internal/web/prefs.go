package web

import (
	"net/http"

	"github.com/mbolli/tickit/internal/tabs"
)

// setPref sets one display preference of the session (theme, style,
// density, a dismissed notice): POST /cmd/prefs {tab, op, key, value}. It
// answers 204, and the tab's stream brings $_prefs, which the document
// puts on <html>; the session's other tabs follow. An unknown key or value
// answers 400. No cookie carries it (NFR-31).
func (s *Server) setPref(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Payload
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	s.act(w, r, a, tabs.SetPref{Session: a.Session, Key: req.Key, Value: req.Value, At: a.By.At})
}
