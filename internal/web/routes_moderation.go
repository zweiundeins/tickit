package web

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/demo"
	"github.com/zweiundeins/tickit/internal/images"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/views"
)

// routesModeration registers moderation (DM-07): the page of what
// visitors wrote lately with one-click hide, restore and delete, blocks
// of guests, and the log. Moderators and site admins only; everyone else
// gets 403, and the commands check the right again in the writer.
func (s *Server) routesModeration() {
	s.get("GET /moderation", http.HandlerFunc(s.moderationPage))
	s.probes = append(s.probes, "/stream?tab=probe&v=moderation:")
	s.post("POST /cmd/moderation/act", s.moderate)
	s.post("POST /cmd/moderation/block", s.block(true))
	s.post("POST /cmd/moderation/unblock", s.block(false))
	s.post("POST /cmd/moderation/dismiss", s.dismissReport)
	s.post("POST /cmd/report", s.report)
	s.views.Register("moderation", s.moderationView)
}

// moderationLimit is how many things the page lists.
const moderationLimit = 50

func (s *Server) moderationState(ctx context.Context, q sqlite.Querier) (views.Moderation, error) {
	now := time.Now()
	m := views.Moderation{Names: map[string]string{}, Delete: map[string]bool{}, Now: now}
	for _, k := range demo.ModeratedKinds() {
		m.Names[k.Kind], m.Delete[k.Kind] = k.Name, k.Delete != nil
	}
	var err error
	if m.Items, err = demo.Newest(ctx, q, moderationLimit); err != nil {
		return m, err
	}
	if m.Blocks, err = demo.Blocks(ctx, q, now); err != nil {
		return m, err
	}
	if m.Log, err = demo.ModerationLog(ctx, q, 30); err != nil {
		return m, err
	}
	if m.Reports, err = demo.OpenReports(ctx, q, moderationLimit); err != nil {
		return m, err
	}
	var ids []string
	for _, rp := range m.Reports {
		ids = append(ids, rp.By)
	}
	for _, t := range m.Items {
		ids = append(ids, t.Author)
	}
	for _, b := range m.Blocks {
		ids = append(ids, b.Person)
	}
	for _, e := range m.Log {
		ids = append(ids, e.Moderator)
	}
	m.People, err = people.Lookup(ctx, q, ids...)
	return m, err
}

func (s *Server) moderationPage(w http.ResponseWriter, r *http.Request) {
	if !access.SiteCan(people.Actor(r.Context()), access.Moderate) {
		s.forbidden(w, r, "Moderation is for moderators and site admins. Log in with a staff account that has the role.")
		return
	}
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		m, err := s.moderationState(ctx, q)
		if err != nil {
			return Page{}, err
		}
		return Page{Title: "Moderation", Nav: "moderation", Width: "full", Views: []string{"moderation:"}, Body: views.ModerationPage(m)}, nil
	})
}

// moderationView renders the page's region again whenever visitors write
// (issues, comments, pages) or moderators act, so a hidden thing shows as
// hidden in the next frame.
func (s *Server) moderationView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	if !access.SiteCan(actor, access.Moderate) {
		return live.View{}, errForbidden
	}
	return live.View{
		Key:    "moderation",
		Topics: []string{tracker.IssuesTopic, demo.ModerationTopic, people.Topic, "pages", images.Topic},
		Every:  time.Minute,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			m, err := s.moderationState(ctx, querier(ctx, tx))
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.ModerationRegion(m))
		},
	}, nil
}

// moderate hides, restores or deletes a thing (DM-07.1).
func (s *Server) moderate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Payload
		Action string `json:"action"`
		Kind   string `json:"kind"`
		ID     string `json:"id"`
		Reason string `json:"reason"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	res, ok := s.run(w, r, a, demo.Moderate{By: a.By, Action: req.Action, Kind: req.Kind, ID: req.ID, Reason: req.Reason})
	s.moderated(w, res.Result, ok, req.Action)
}

// report is any visitor's report of something with a reason (DM-08),
// which the moderation page lists first.
func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Payload
		Kind   string `json:"kind"`
		ID     string `json:"id"`
		Reason string `json:"reason"`
	}
	a, ok := s.decode(w, r, classComment, &req)
	if !ok {
		return
	}
	s.act(w, r, a, demo.Report{By: a.By, Kind: req.Kind, ID: req.ID, Reason: req.Reason})
}

// dismissReport closes a report that needs nothing done (DM-08).
func (s *Server) dismissReport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Payload
		Report int64 `json:"report"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	s.act(w, r, a, demo.CloseReport{By: a.By, Report: req.Report})
}

// block pauses a guest's changes for 24 hours, or lifts the block
// (DM-07.2).
func (s *Server) block(on bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Payload
			Person string `json:"person"`
			Reason string `json:"reason"`
		}
		a, ok := s.decode(w, r, classField, &req)
		if !ok {
			return
		}
		cmd, action := s.demo.Unblock(a.By, req.Person), demo.ActUnblock
		if on {
			cmd, action = s.demo.Block(a.By, req.Person, req.Reason), demo.ActBlock
		}
		res, ok := s.run(w, r, a, cmd)
		s.moderated(w, res.Result, ok, action)
	}
}

// moderated answers a moderation action as act does, and counts it.
func (s *Server) moderated(w http.ResponseWriter, res tabs.Result, ok bool, action string) {
	switch {
	case !ok:
	case res.Denied:
		http.Error(w, "forbidden", http.StatusForbidden)
	default:
		if res.OK {
			s.metrics.moderations.WithLabelValues(action).Inc()
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
