package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/zweiundeins/lith/edge"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/sse"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/views"
)

// routesCore registers what every page needs: the stream
// and its side panels, login and logout, the name of a person, privacy,
// robots.txt, the static files, and the page of an unknown address.
func (s *Server) routesCore() {
	s.get("GET /stream", http.HandlerFunc(s.stream), "/stream?tab=probe&v=", "/stream?tab=probe&v=nope:1")
	s.post("POST /cmd/watch", s.watch)
	s.post("POST /cmd/unwatch", s.unwatch)
	s.get("GET /login", http.HandlerFunc(s.loginPage), "/login", "/login?next=/projects")
	s.post("POST /cmd/login", s.login)
	s.post("POST /cmd/logout", s.logout)
	s.post("POST /cmd/people/rename", s.rename)
	s.get("GET /privacy", http.HandlerFunc(s.privacy))
	s.get("GET /robots.txt", http.HandlerFunc(robots))
	s.post("POST /cmd/prefs", s.setPref)
	s.get("GET /static/", s.static, s.static.URL(Bundle), "/static/nope.css")
	s.get("GET /", http.HandlerFunc(s.notFound), "/no/such/page")
}

func (s *Server) privacy(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, func(context.Context, sqlite.Querier) (Page, error) {
		return Page{Title: "Privacy", Body: views.Privacy(), Width: "prose"}, nil
	})
}

// robots keeps search engines to the home page (DM-09.1).
func robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("User-agent: *\nAllow: /$\nDisallow: /\n"))
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, func(context.Context, sqlite.Querier) (Page, error) {
		return Page{Title: "Log in", Body: views.Login()}, nil
	})
}

// loginRequest is the login form's payload.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Next     string `json:"next"`
}

// login logs a staff account in (FR-122): the same answer, after the same
// work, for an unknown email and a wrong password (NFR-10); at most 30
// tries a minute from an address, and an email that failed 5 times from
// an address waits 15 minutes there (NFR-09). On success the session id
// is renewed, and the page goes where it asked to.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ctx, now := r.Context(), time.Now()
	addr := edge.ClientKey(r)
	if !s.signIn.Allow(addr, 1, now) {
		s.metrics.limited.WithLabelValues("sign-in").Inc()
		http.Error(w, tooMany, http.StatusTooManyRequests)
		return
	}
	var req loginRequest
	if err := sse.ReadSignalsLimit(w, r, &req, 4<<10); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	key := people.LockoutKey(req.Email, addr)
	if s.lockout.Locked(key, now) {
		s.loginFailed(w, r, people.LogSecurity{Kind: people.EventLogin, Outcome: people.OutcomeLocked, At: now}, people.MsgLocked)
		return
	}
	creds, err := people.Verify(ctx, s.db.R, req.Email, req.Password)
	if err != nil && !errors.Is(err, people.ErrWrongLogin) && !errors.Is(err, people.ErrPending) && !errors.Is(err, people.ErrInactive) {
		s.fail(w, r, err)
		return
	}
	if err != nil {
		outcome := people.OutcomeWrong
		switch {
		case errors.Is(err, people.ErrPending):
			outcome = people.OutcomePending
		case errors.Is(err, people.ErrInactive):
			outcome = people.OutcomeInactive
		default:
			s.lockout.Fail(key, now)
		}
		s.loginFailed(w, r, people.LogSecurity{Kind: people.EventLogin, Outcome: outcome, Person: creds.Person.ID, At: now}, people.LoginMessage(err))
		return
	}
	sid := s.sessions.Renew(w) // the request's context keeps the old id
	err = s.w.Do(ctx, people.Login{Credentials: creds, Session: sid, UserAgent: r.UserAgent(), TTL: s.ttl, At: now})
	var refusal *writer.Refusal
	switch {
	case errors.As(err, &refusal):
		s.metrics.logins.WithLabelValues(people.OutcomeWrong).Inc()
		s.signals(w, map[string]any{"_login": map[string]any{"problem": refusal.Msg, "busy": false}})
		return
	case err != nil:
		s.failAction(w, r, err)
		return
	}
	s.lockout.Succeed(key)
	s.metrics.logins.WithLabelValues(people.OutcomeOK).Inc()
	s.signals(w, map[string]any{"_goto": localPath(req.Next)})
}

// loginFailed records a failed login in the security log, with no secret
// (FR-156), and tells the page why.
func (s *Server) loginFailed(w http.ResponseWriter, r *http.Request, entry people.LogSecurity, msg string) {
	if err := s.w.Do(r.Context(), entry); err != nil {
		s.failAction(w, r, err)
		return
	}
	s.metrics.logins.WithLabelValues(entry.Outcome).Inc()
	s.signals(w, map[string]any{"_login": map[string]any{"problem": msg, "password": "", "busy": false}})
}

// localPath is next if it is a path on this site, else the home page: a
// login never sends the browser elsewhere.
func localPath(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.ContainsAny(next, "\\\r\n\t") || strings.HasPrefix(next, "/login") {
		return "/"
	}
	return next
}

// logout ends the session's login session and its cookie (FR-123): the
// browser continues as a new guest.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !s.signIn.Allow(edge.ClientKey(r), 1, time.Now()) {
		http.Error(w, tooMany, http.StatusTooManyRequests)
		return
	}
	if err := s.w.Do(r.Context(), people.Logout{Session: edge.SessionID(r.Context()), At: time.Now()}); err != nil {
		s.failAction(w, r, err)
		return
	}
	s.sessions.Clear(w)
	s.signals(w, map[string]any{"_goto": "/"})
}

// rename changes the name others see beside what the person does (FR-121).
func (s *Server) rename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Payload
		Name string `json:"name"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	s.act(w, r, a, people.Rename{Person: a.By.Actor.ID, Name: req.Name})
}
