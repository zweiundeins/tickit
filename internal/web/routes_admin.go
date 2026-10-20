package web

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/zweiundeins/lith/edge"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/sse"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/demo"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
	"github.com/mbolli/tickit/internal/ui"
	"github.com/mbolli/tickit/internal/views"
)

// PasswordPath is an account's own page: its password and sessions
// (FR-125, FR-129). An account that must change its first password sees
// no other page (FR-141.1).
const PasswordPath = "/account"

// routesAdmin registers administration: accounts, site roles, statistics,
// the security log and the demo's reset (FR-140 to FR-144, FR-156), and an
// account's own page. Every page and action needs a site admin's right
// and answers 403 without it (NFR-01.1); the commands check it again in
// the writer.
func (s *Server) routesAdmin() {
	s.get("GET /admin", s.adminPage(views.TabAccounts, s.adminAccounts), "/admin", "/admin?q=ada&role=admin&status=active&page=2")
	s.get("GET /admin/guests", s.adminPage(views.TabGuests, s.adminGuests))
	s.get("GET /admin/stats", s.adminPage(views.TabStats, s.adminStats))
	s.get("GET /admin/security", s.adminPage(views.TabSecurity, s.adminSecurity), "/admin/security", "/admin/security?kind=login&before=5&account=a-x")
	s.get("GET /admin/demo", s.adminPage(views.TabDemo, s.adminDemo))
	s.get("GET "+PasswordPath, http.HandlerFunc(s.accountPage))
	s.probes = append(s.probes, "/stream?tab=probe&v=admin-accounts:,admin-stats:,admin-demo:")
	s.post("POST /cmd/admin/create", s.adminCreate)
	s.post("POST /cmd/admin/role", s.adminRole)
	s.post("POST /cmd/admin/active", s.adminActive)
	s.post("POST /cmd/admin/reset", s.adminReset)
	s.post("POST /cmd/account/password", s.changePassword)
	s.post("POST /cmd/account/end-session", s.endSession)
	s.views.Register("admin-accounts", s.adminAccountsView)
	s.views.Register("admin-stats", s.adminStatsView)
	s.views.Register("admin-demo", s.adminDemoView)
}

// errForbidden is a view's answer to a person without the right: 403.
var errForbidden = &access.Denial{Refusal: writer.Refusal{Msg: "Only site admins see this."}}

// adminTab reads one tab of the admin pages, and the views it watches.
type adminTab func(ctx context.Context, q sqlite.Querier, r *http.Request, me people.Person) (templ.Component, []string, error)

func (s *Server) adminPage(tab views.AdminTab, build adminTab) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := people.Actor(r.Context())
		if !access.SiteCan(me, access.Administer) {
			s.forbidden(w, r, "The admin pages are for site admins. Log in with a staff account that has the role.")
			return
		}
		s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
			body, vs, err := build(ctx, q, r, me)
			if err != nil {
				return Page{}, err
			}
			return Page{Title: "Administration", Nav: "admin", Width: "full", Views: vs, Body: within(views.Admin(tab), body)}, nil
		})
	}
}

// within renders body as the children of frame.
func within(frame, body templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return frame.Render(templ.WithChildren(ctx, body), w)
	})
}

// forbidden answers a page the person may not see with 403 (NFR-01).
func (s *Server) forbidden(w http.ResponseWriter, r *http.Request, why string) {
	s.render(w, r, http.StatusForbidden, func(context.Context, sqlite.Querier) (Page, error) {
		return Page{Title: "Staff only", Body: views.Forbidden(why)}, nil
	})
}

// accountFilter reads the accounts list's filter from a query.
func accountFilter(v url.Values) people.AccountFilter {
	f := people.AccountFilter{Search: clipText(strings.TrimSpace(v.Get("q")), 100), Status: v.Get("status")}
	if r := people.SiteRole(v.Get("role")); r.Valid() {
		f.Role = r
	}
	switch f.Status {
	case "active", "inactive", "pending":
	default:
		f.Status = ""
	}
	f.Page, _ = strconv.Atoi(v.Get("page"))
	f.Page = max(1, min(f.Page, 1000))
	return f
}

func clipText(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// filterArg is a filter as a view's argument: its query, base64url.
func filterArg(f people.AccountFilter) string {
	v := url.Values{}
	if f.Search != "" {
		v.Set("q", f.Search)
	}
	if f.Role != "" {
		v.Set("role", string(f.Role))
	}
	if f.Status != "" {
		v.Set("status", f.Status)
	}
	if f.Page > 1 {
		v.Set("page", strconv.Itoa(f.Page))
	}
	return base64.RawURLEncoding.EncodeToString([]byte(v.Encode()))
}

func (s *Server) accountsOf(ctx context.Context, q sqlite.Querier, me people.Person, f people.AccountFilter) (views.AdminAccounts, error) {
	rows, total, err := people.Accounts(ctx, q, f)
	return views.AdminAccounts{Rows: rows, Total: total, Filter: f, Me: me.ID, Now: time.Now()}, err
}

func (s *Server) adminAccounts(ctx context.Context, q sqlite.Querier, r *http.Request, me people.Person) (templ.Component, []string, error) {
	f := accountFilter(r.URL.Query())
	a, err := s.accountsOf(ctx, q, me, f)
	return views.AdminAccountsTab(a), []string{"admin-accounts:" + filterArg(f)}, err
}

// adminAccountsView is the accounts table of one filter, for one admin
// (it leaves out their own Deactivate): it renders again when any person
// or account changes.
func (s *Server) adminAccountsView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	if !access.SiteCan(actor, access.Administer) {
		return live.View{}, errForbidden
	}
	raw, err := base64.RawURLEncoding.DecodeString(arg)
	if err != nil {
		return live.View{}, fmt.Errorf("%w: a filter", errBadView)
	}
	v, err := url.ParseQuery(string(raw))
	if err != nil {
		return live.View{}, fmt.Errorf("%w: a filter", errBadView)
	}
	f := accountFilter(v)
	return live.View{
		Key:    "admin-accounts:" + actor.ID + ":" + filterArg(f),
		Topics: []string{people.Topic},
		Every:  time.Minute,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			a, err := s.accountsOf(ctx, querier(ctx, tx), actor, f)
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.AdminAccountsTable(a))
		},
	}, nil
}

func (s *Server) adminGuests(ctx context.Context, q sqlite.Querier, r *http.Request, me people.Person) (templ.Component, []string, error) {
	now := time.Now()
	rows, err := people.ActiveGuests(ctx, q, now.Add(-24*time.Hour), 500)
	return views.AdminGuestsTab(views.AdminGuests{Rows: rows, Now: now}), nil, err
}

func (s *Server) adminStats(ctx context.Context, q sqlite.Querier, r *http.Request, me people.Person) (templ.Component, []string, error) {
	st, err := s.stats(ctx, q)
	return views.AdminStatsTab(st), []string{"admin-stats:"}, err
}

// adminStatsView renders the statistics every 5 seconds.
func (s *Server) adminStatsView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	if !access.SiteCan(actor, access.Administer) {
		return live.View{}, errForbidden
	}
	return live.View{
		Key:   "admin-stats",
		Every: 5 * time.Second,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			st, err := s.stats(ctx, querier(ctx, tx))
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.AdminStatsTab(st))
		},
	}, nil
}

// stats are FR-144's numbers: accounts, guests of the last 24 hours,
// issues, pages, open streams, the writer's and the hub's.
func (s *Server) stats(ctx context.Context, q sqlite.Querier) (views.AdminStats, error) {
	var errs []error
	n := func(query string, args ...any) string {
		var v int
		err := q.QueryRowContext(ctx, query, args...).Scan(&v)
		if err != nil && !strings.Contains(err.Error(), "no such table") { // the wiki's pages, before the wiki
			errs = append(errs, err)
		}
		return strconv.Itoa(v)
	}
	hub, wr := s.hub.Stats(), s.w.Stats()
	st := views.AdminStats{
		Counts: []ui.Stat{
			{Label: "Accounts", Value: n(`SELECT count(*) FROM accounts WHERE status = 'active'`), Href: "/admin"},
			{Label: "Guests today", Value: n(`SELECT count(*) FROM people WHERE kind = 'guest' AND last_seen_at > ?`, time.Now().Add(-24*time.Hour).Unix()), Href: "/admin/guests"},
			{Label: "Issues", Value: n(`SELECT count(*) FROM issues WHERE deleted_at IS NULL`)},
			{Label: "Pages", Value: n(`SELECT count(*) FROM pages`)},
			{Label: "Open streams", Value: strconv.Itoa(hub.Streams)},
		},
		Writer: []ui.Stat{
			{Label: "Commands", Value: strconv.FormatInt(wr.Commands, 10), Note: strconv.FormatInt(wr.Failures, 10) + " failed"},
			{Label: "Batches", Value: strconv.FormatInt(wr.Batches, 10), Note: "at most " + strconv.Itoa(wr.MaxBatch) + " commands"},
			{Label: "Queue", Value: strconv.Itoa(wr.Queue)},
			{Label: "Commit p99", Value: wr.P99Commit.Round(time.Microsecond).String(), Note: "last " + wr.LastCommit.Round(time.Microsecond).String()},
		},
		Hub: []ui.Stat{
			{Label: "Views", Value: strconv.Itoa(hub.Views)},
			{Label: "Renders", Value: strconv.FormatInt(hub.Renders, 10), Note: strconv.FormatInt(hub.Ticks, 10) + " ticks"},
			{Label: "Frames sent", Value: strconv.FormatInt(hub.FramesSent, 10), Note: strconv.FormatInt(hub.FramesDropped, 10) + " dropped"},
			{Label: "Overruns", Value: strconv.FormatInt(hub.Overruns, 10), Tone: overrunTone(hub.Overruns)},
		},
	}
	return st, errors.Join(errs...)
}

func overrunTone(n int64) ui.Tone {
	if n > 0 {
		return ui.ToneWarn
	}
	return ui.ToneNeutral
}

func (s *Server) adminSecurity(ctx context.Context, q sqlite.Querier, r *http.Request, me people.Person) (templ.Component, []string, error) {
	v := r.URL.Query()
	f := people.SecurityFilter{Person: v.Get("account"), Kind: v.Get("kind")}
	if !people.ValidID(f.Person) {
		f.Person = ""
	}
	if !contains(append(people.SecurityKinds, demo.EventReset), f.Kind) {
		f.Kind = ""
	}
	f.Before, _ = strconv.ParseInt(v.Get("before"), 10, 64)
	entries, err := people.SecurityLog(ctx, q, f)
	if err != nil {
		return nil, nil, err
	}
	var ids []string
	for _, e := range entries {
		for _, id := range []string{e.Person, e.Actor} {
			if id != "" {
				ids = append(ids, id)
			}
		}
	}
	found, err := people.Lookup(ctx, q, ids...)
	if err != nil {
		return nil, nil, err
	}
	accounts, _, err := people.Accounts(ctx, q, people.AccountFilter{})
	return views.AdminSecurityTab(views.AdminSecurity{Entries: entries, People: found, Filter: f, Accounts: accounts, Now: time.Now()}), nil, err
}

// demoState is the demo tab: the next reset, the caps, the last resets.
func (s *Server) demoState(ctx context.Context, q sqlite.Querier) (views.AdminDemo, error) {
	now := time.Now()
	d := views.AdminDemo{Status: s.demo.Status(now), Next: demo.NextReset(now), Now: now}
	var err error
	if d.Resets, err = demo.Resets(ctx, q, 10); err != nil {
		return d, err
	}
	var ids []string
	for _, r := range d.Resets {
		if r.By != "" {
			ids = append(ids, r.By)
		}
	}
	if d.People, err = people.Lookup(ctx, q, ids...); err != nil {
		return d, err
	}
	d.Created, err = demo.Created(ctx, q)
	return d, err
}

func (s *Server) adminDemo(ctx context.Context, q sqlite.Querier, r *http.Request, me people.Person) (templ.Component, []string, error) {
	d, err := s.demoState(ctx, q)
	return views.AdminDemoTab(d), []string{"admin-demo:"}, err
}

// adminDemoView renders the demo's state when a reset starts and ends,
// and every minute.
func (s *Server) adminDemoView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	if !access.SiteCan(actor, access.Administer) {
		return live.View{}, errForbidden
	}
	return live.View{
		Key:    "admin-demo",
		Topics: []string{demo.Topic},
		Every:  time.Minute,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			d, err := s.demoState(ctx, querier(ctx, tx))
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.AdminDemoTab(d))
		},
	}, nil
}

// adminCreate makes a staff account (FR-141) with a first password that
// must be changed at the first login.
func (s *Server) adminCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Payload
		Email    string `json:"email"`
		Username string `json:"username"`
		Name     string `json:"name"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	cmd, err := s.newAccount(a, req.Email, req.Username, req.Name, req.Role, req.Password)
	if err != nil {
		cmd = writer.Func(func(*writer.Tx) error { return err })
	}
	s.act(w, r, a, cmd)
}

// newAccount checks the form of a new account and hashes its password,
// outside the writer. Its error is a refusal the form shows.
func (s *Server) newAccount(a action, email, username, name, role, password string) (writer.Command, error) {
	if !access.SiteCan(a.By.Actor, access.Administer) {
		return nil, errForbidden // before the slow hash
	}
	email, username, name = strings.TrimSpace(email), strings.TrimSpace(username), strings.TrimSpace(name)
	if err := people.CheckPolicy(password); err != nil {
		return nil, tabs.Refuse("password", "A password has 8 to 128 characters.")
	}
	sr := people.SiteRole(role)
	if !sr.Valid() {
		return nil, tabs.Refuse("role", "Pick a site role.")
	}
	c := people.NewAccount{Admin: a.By.Actor, CreateAccount: people.CreateAccount{ID: people.NewAccountID(), Email: email, Username: username,
		FullName: name, Role: sr, PasswordHash: "$argon2id$", At: a.By.At}}
	if err := c.Validate(); err != nil {
		return nil, &writer.Refusal{Msg: capitalFirst(err.Error()) + "."}
	}
	h, err := people.HashPassword(password)
	if err != nil {
		return nil, err
	}
	c.PasswordHash = h
	return c, nil
}

func capitalFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// adminRole makes an account a moderator, an admin or neither (FR-142).
func (s *Server) adminRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Payload
		Account string `json:"account"`
		Role    string `json:"role"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	if s.actStaff(w, r, a, people.SetRole{Admin: a.By.Actor, Account: req.Account, Role: people.SiteRole(req.Role), At: a.By.At}) {
		s.dropStaffViews(req.Account)
	}
}

// adminActive deactivates an account, or activates it again (FR-143).
func (s *Server) adminActive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Payload
		Account string `json:"account"`
		Active  bool   `json:"active"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	if s.actStaff(w, r, a, people.SetActive{Admin: a.By.Actor, Account: req.Account, Active: req.Active, At: a.By.At}) {
		s.dropStaffViews(req.Account)
	}
}

// actStaff is act for a change of an account's rights, and reports whether
// it went through.
func (s *Server) actStaff(w http.ResponseWriter, r *http.Request, a action, cmd writer.Command) bool {
	res, ok := s.run(w, r, a, cmd)
	switch {
	case !ok:
		return false
	case res.Denied:
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	w.WriteHeader(http.StatusNoContent)
	return res.OK
}

// dropStaffViews takes the staff's views off the open streams of an
// account whose role or status changed (NFR-01.2): a page it opens again
// asks for them anew, and gets 403 without the right. Its other views
// follow the role at their next render.
func (s *Server) dropStaffViews(account string) {
	keys := []string{"moderation", "admin-stats", "admin-demo"}
	for key := range s.hub.Stats().RendersByKey {
		if strings.HasPrefix(key, "admin-accounts:"+account+":") {
			keys = append(keys, key)
		}
	}
	for _, key := range keys {
		for _, id := range s.hub.Watchers(key) {
			if personOf(id) == account {
				s.hub.Unwatch(id, key)
			}
		}
	}
}

// adminReset is "Reset now" (DM-03.1): the command checks the right and
// logs it; the reset runs after it, and every page shows it.
func (s *Server) adminReset(w http.ResponseWriter, r *http.Request) {
	var req struct{ Payload }
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	res, ok := s.run(w, r, a, writer.Func(func(tx *writer.Tx) error {
		if err := access.Require(tx, a.By.Actor, access.Administer, access.Scope{}); err != nil {
			return err
		}
		return people.LogSecurity{Kind: demo.EventReset, Outcome: people.OutcomeOK, Person: a.By.Actor.ID, Actor: a.By.Actor.ID, Detail: "reset now", At: a.By.At}.Apply(tx)
	}))
	switch {
	case !ok:
		return
	case res.Denied:
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	case res.OK && !res.replayed: // a retry starts no second reset
		go func() {
			if err := s.demo.Reset(context.WithoutCancel(r.Context()), a.By.Actor.ID); err != nil && !errors.Is(err, demo.ErrResetting) {
				s.log.Debug("reset now", "err", err) // Reset logged it at ERROR
			}
		}()
	}
	w.WriteHeader(http.StatusNoContent)
}

// accountPage is an account's own page (FR-125, FR-129).
func (s *Server) accountPage(w http.ResponseWriter, r *http.Request) {
	me := people.Actor(r.Context())
	if me.Kind != people.Account {
		s.forbidden(w, r, "Guests have no account: only staff log in.")
		return
	}
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		sessions, err := people.Sessions(ctx, q, me.ID, edge.SessionID(ctx), time.Now())
		if err != nil {
			return Page{}, err
		}
		return Page{Title: "Your account", Body: views.Account(views.AccountPage{Me: me, Sessions: sessions, Now: time.Now()})}, nil
	})
}

// changePassword changes an account's password (FR-125): the current one
// is checked outside the writer, at most 30 a minute from an address
// (NFR-09); the command keeps this session and ends the others.
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	ctx, now := r.Context(), time.Now()
	if !s.signIn.Allow(edge.ClientKey(r), 1, now) {
		s.metrics.limited.WithLabelValues("sign-in").Inc()
		http.Error(w, tooMany, http.StatusTooManyRequests)
		return
	}
	var req struct {
		Payload
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if err := sse.ReadSignalsLimit(w, r, &req, 4<<10); err != nil || !validID(req.Tab) || !validID(req.Op) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	me := people.Actor(ctx)
	if me.Kind != people.Account {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	a := action{Session: edge.SessionID(ctx), Tab: req.Tab, By: tabs.By{Actor: me, Op: req.Op, At: now}, Addr: edge.ClientKey(r)}
	var cmd writer.Command
	// NFR-09.2: a session cannot guess its account's password either.
	key := people.LockoutKey(me.ID, a.Addr)
	if s.lockout.Locked(key, now) {
		s.act(w, r, a, refuse("current", people.MsgLocked))
		return
	}
	cur, err := people.CheckCurrent(ctx, s.db.R, me.ID, req.Current)
	switch {
	case errors.Is(err, people.ErrWrongLogin):
		s.lockout.Fail(key, now)
		cmd = refuse("current", "The current password is wrong.")
	case err != nil:
		s.fail(w, r, err)
		return
	case people.CheckPolicy(req.Password) != nil:
		cmd = refuse("password", "A password has 8 to 128 characters.")
	default:
		h, err := people.HashPassword(req.Password)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		cmd = people.ChangePassword{Current: cur, Session: a.Session, NewHash: h, At: now}
	}
	s.act(w, r, a, cmd)
}

// endSession ends one of the account's other login sessions (FR-129).
func (s *Server) endSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Payload
		Session string `json:"session"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	if a.By.Actor.Kind != people.Account {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.act(w, r, a, people.EndSession{Person: a.By.Actor.ID, ID: req.Session, At: a.By.At})
}
