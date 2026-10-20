package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/sse"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/activity"
	"github.com/zweiundeins/tickit/internal/dashboards"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/views"
)

// routesDashboards registers dashboards (FR-60, FR-137.1) and their views:
//
//	dashboards:       the list of every dashboard (shared)
//	dashboard:<id>    a dashboard's grid, keyed by the scopes the viewer
//	                  reads: one render a tick for all who read the same
//	dashme:<id>       the viewer's own "Assigned to me" gadgets of it
//
// A gadget's counts (by status, created and resolved per day) come from
// s.counts, kept against the hub's version of their project's board, so
// a dashboard renders again on a change of its issues without counting
// them again for every dashboard that shows them.
func (s *Server) routesDashboards() {
	s.get("GET /dashboards", http.HandlerFunc(s.dashboardsPage))
	s.get("GET /dashboards/{id}", http.HandlerFunc(s.dashboardPage), "/dashboards/"+ProbeDashboard, "/dashboards/nope", "/dashboards/01920000-0000-7000-8000-00000000dddd")
	s.views.Register("dashboards", s.dashboardsView)
	s.views.Register("dashboard", s.dashboardView)
	s.views.Register("dashme", s.dashmeView)
	s.post("POST /cmd/dashboards/create", s.createDashboard)
	s.post("POST /cmd/dashboards/rename", s.dashboardAction(func(a action, req dashRequest) writer.Command {
		return dashboards.Rename{By: a.By, Dashboard: req.Dashboard, Name: req.Name}
	}))
	s.post("POST /cmd/dashboards/delete", s.deleteDashboard)
	s.post("POST /cmd/dashboards/gadget", s.dashboardAction(func(a action, req dashRequest) writer.Command {
		return dashboards.AddGadget{By: a.By, Dashboard: req.Dashboard, Kind: dashboards.Kind(req.Kind), Project: req.Project, SavedFilter: req.Filter}
	}))
	s.post("POST /cmd/dashboards/gadget/remove", s.dashboardAction(func(a action, req dashRequest) writer.Command {
		return dashboards.RemoveGadget{By: a.By, Dashboard: req.Dashboard, Gadget: req.Gadget}
	}))
	s.post("POST /cmd/dashboards/gadget/move", s.dashboardAction(func(a action, req dashRequest) writer.Command {
		return dashboards.MoveGadget{By: a.By, Dashboard: req.Dashboard, Gadget: req.Gadget, Earlier: req.Earlier}
	}))
}

// ProbeDashboard is the id of the dashboard in the probes' data
// (seedProbes), one of every kind of gadget.
const ProbeDashboard = "01920000-0000-7000-8000-0000000000d1"

// ── Pages ─────────────────────────────────────────────────────────────

func (s *Server) dashboardsPage(w http.ResponseWriter, r *http.Request) {
	me := people.Actor(r.Context())
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		l, err := dashboardList(ctx, q)
		if err != nil {
			return Page{}, err
		}
		return Page{Title: "Dashboards", Nav: "dashboards", Views: []string{"dashboards:"}, Body: views.DashboardsBody(views.DashboardsPage{List: l, Me: me.ID})}, nil
	})
}

func dashboardList(ctx context.Context, q sqlite.Querier) (views.DashboardList, error) {
	list, err := dashboards.List(ctx, q)
	if err != nil {
		return views.DashboardList{}, err
	}
	var ids []string
	for _, d := range list {
		ids = append(ids, d.Owner)
	}
	who, err := people.Lookup(ctx, q, ids...)
	return views.DashboardList{Dashboards: list, People: who, Now: time.Now()}, err
}

func (s *Server) dashboardsView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	if arg != "" {
		return live.View{}, fmt.Errorf("%w: dashboards takes no argument", errBadView)
	}
	return live.View{
		Key:    "dashboards:",
		Topics: []string{dashboards.ListTopic, people.Topic},
		Every:  time.Minute,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			l, err := dashboardList(ctx, querier(ctx, tx))
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.DashboardListRegion(l))
		},
	}, nil
}

func (s *Server) dashboardPage(w http.ResponseWriter, r *http.Request) {
	me := people.Actor(r.Context())
	id := r.PathValue("id")
	seq := s.hub.Seq() // before the transaction: what the counts' cache may count as read
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		d, err := dashboards.Get(ctx, q, id)
		if err != nil {
			return Page{}, err
		}
		readable, err := activity.Readable(ctx, q, me)
		if err != nil {
			return Page{}, err
		}
		grid, err := s.dashboardGrid(ctx, q, d, readable, seq)
		if err != nil {
			return Page{}, err
		}
		p := views.DashboardPage{Grid: grid, Me: me.ID, Mine: !me.IsZero() && (d.Owner == me.ID || me.Role == people.Admin)}
		if p.Mine {
			ids, err := tracker.Readable(ctx, q, me)
			if err != nil {
				return Page{}, err
			}
			if p.Projects, err = tracker.Projects(ctx, q, ids); err != nil {
				return Page{}, err
			}
			if p.Filters, err = tracker.SavedFilters(ctx, q, me.ID); err != nil {
				return Page{}, err
			}
		}
		names := []string{"dashboard:" + d.ID}
		if hasKind(d, dashboards.Assigned) {
			names = append(names, "dashme:"+d.ID)
		}
		return Page{Title: d.Name, Nav: "dashboards", Width: "full", Views: names, Body: views.DashboardBody(p)}, nil
	})
}

func hasKind(d dashboards.Dashboard, k dashboards.Kind) bool {
	for _, g := range d.Gadgets {
		if g.Kind == k {
			return true
		}
	}
	return false
}

// ── Views ─────────────────────────────────────────────────────────────

// dashboardView is dashboard:<id>, the grid of a dashboard for everyone
// who reads the same projects and spaces: what its gadgets show depends
// on those alone (a saved filter's "me" is the dashboard's owner).
func (s *Server) dashboardView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	if _, err := dashboards.Get(ctx, q, arg); err != nil {
		return live.View{}, err
	}
	readable, err := activity.Readable(ctx, q, actor)
	if err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    "dashboard:" + arg + ":" + activity.Key(readable),
		Topics: []string{dashboards.Topic(arg), tracker.ProjectsTopic, tracker.FiltersTopic, people.Topic},
		Every:  time.Minute,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			q := querier(ctx, tx)
			d, err := dashboards.Get(ctx, q, arg)
			if errors.Is(err, access.ErrNotFound) {
				return patch(ctx, views.DashboardGone())
			}
			if err != nil {
				return nil, err
			}
			grid, err := s.dashboardGrid(ctx, q, d, readable, live.Seq(ctx))
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.DashboardGridRegion(grid))
		},
	}, nil
}

// dashboardGrid reads what a dashboard's gadgets show, for a viewer who
// reads readable; seq is what the counts' cache may count as read. In a
// render it declares the topics its gadgets read (live.Depends): the
// boards of their projects, the issues of a saved filter, the feeds.
func (s *Server) dashboardGrid(ctx context.Context, q sqlite.Querier, d dashboards.Dashboard, readable []access.Scope, seq uint64) (views.DashboardGrid, error) {
	now := time.Now()
	owner, err := people.Get(ctx, q, d.Owner)
	if err != nil {
		return views.DashboardGrid{}, err
	}
	g := views.DashboardGrid{Dashboard: d, Owner: owner, Now: now}
	keep := activity.In(readable)
	var projects []string
	for _, sc := range readable {
		if sc.Kind == access.Project {
			projects = append(projects, sc.ID)
		}
	}
	for _, gd := range d.Gadgets {
		v := views.Gadget{Gadget: gd, Title: gd.Kind.Name()}
		switch gd.Kind {
		case dashboards.Filter:
			live.Depends(ctx, tracker.IssuesTopic)
			sf, err := tracker.SavedFilterByID(ctx, q, gd.SavedFilter)
			if errors.Is(err, access.ErrNotFound) {
				v.Missing = "Its saved filter was deleted."
				break
			}
			if err != nil {
				return g, err
			}
			v.Title, v.Href = sf.Name, "/issues?"+tracker.Query{Filter: sf.Filter, Sort: sf.Sort}.Values().Encode()
			if v.Issues, err = tracker.Search(ctx, q, tracker.Query{Readable: projects, Me: d.Owner, Filter: sf.Filter, Sort: sf.Sort, Page: 1}); err != nil {
				return g, err
			}
			if hasMe(sf.Filter) {
				v.Note = "“Me” in this filter is " + owner.Name + ", the dashboard's owner."
			}
		case dashboards.Status, dashboards.Created:
			if !keep(access.ProjectScope(gd.Project)) {
				v.Missing = "You cannot see its project."
				break
			}
			live.Depends(ctx, tracker.BoardTopic(gd.Project), tracker.ProjectTopic(gd.Project))
			p, err := tracker.ProjectByID(ctx, q, gd.Project)
			if errors.Is(err, access.ErrNotFound) {
				v.Missing = "Its project is gone."
				break
			}
			if err != nil {
				return g, err
			}
			v.ProjectKey = p.Key
			if gd.Kind == dashboards.Status {
				v.Title, v.Href = p.Name+" by status", "/issues?"+url.Values{"project": {p.Key}}.Encode()
				if v.Counts, err = s.counts.status(ctx, q, gd.Project, seq); err != nil {
					return g, err
				}
			} else {
				v.Title, v.Href = p.Name+": created and resolved", "/issues?"+url.Values{"project": {p.Key}, "sort": {"resolved"}, "dir": {"desc"}}.Encode()
				if v.Days, err = s.counts.days(ctx, q, gd.Project, now, seq); err != nil {
					return g, err
				}
			}
		case dashboards.Assigned:
			v.Href = "/issues?assignee=me&category=todo&category=inprogress&sort=updated&dir=desc"
			v.Note = "Each person sees their own."
		case dashboards.Activity:
			live.Depends(ctx, feedTopics...)
			entries, _, err := activity.Home(ctx, q, readable, 10)
			if err != nil {
				return g, err
			}
			if v.Feed, err = s.feed(ctx, q, views.ActivityID("gadget-"+gd.ID), entries, now); err != nil {
				return g, err
			}
		}
		g.Gadgets = append(g.Gadgets, v)
	}
	return g, nil
}

func hasMe(f tracker.Filter) bool {
	for _, x := range append(append([]string{}, f.Assignees...), f.Reporters...) {
		if x == tracker.Me {
			return true
		}
	}
	return false
}

// dashmeView is dashme:<id>: the viewer's own "Assigned to me" gadgets of
// a dashboard, small and per person, patched into the slots the shared
// grid leaves for them.
func (s *Server) dashmeView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	if _, err := dashboards.Get(ctx, q, arg); err != nil {
		return live.View{}, err
	}
	readable, err := tracker.Readable(ctx, q, actor)
	if err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    "dashme:" + arg + ":" + actor.ID,
		Topics: []string{dashboards.Topic(arg), tracker.IssuesTopic, people.PersonTopic(actor.ID)},
		Every:  time.Minute,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			q := querier(ctx, tx)
			d, err := dashboards.Get(ctx, q, arg)
			if errors.Is(err, access.ErrNotFound) {
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			a := views.Assigned{Now: time.Now()}
			for _, g := range d.Gadgets {
				if g.Kind == dashboards.Assigned {
					a.Gadgets = append(a.Gadgets, g.ID)
				}
			}
			if len(a.Gadgets) == 0 {
				return nil, nil
			}
			if a.Issues, err = tracker.Search(ctx, q, tracker.Query{Readable: readable, Me: actor.ID, Page: 1,
				Filter: tracker.Filter{Assignees: []string{tracker.Me}, Categories: []tracker.Category{tracker.ToDo, tracker.InProgress}},
				Sort:   tracker.Sort{By: tracker.SortUpdated, Desc: true}}); err != nil {
				return nil, err
			}
			// Replaced, not morphed: the slots sit in data-ignore-morph, so
			// that the shared grid's frames leave them alone.
			return patch(ctx, views.AssignedRegions(a), sse.Patch{Mode: sse.Replace})
		},
	}, nil
}

// ── Actions ───────────────────────────────────────────────────────────

type dashRequest struct {
	Payload
	Dashboard string `json:"dashboard"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Project   string `json:"project"`
	Filter    string `json:"filter"`
	Gadget    string `json:"gadget"`
	Earlier   bool   `json:"earlier"`
}

func (s *Server) decodeDashboard(w http.ResponseWriter, r *http.Request, c class) (action, dashRequest, bool) {
	var req dashRequest
	a, ok := s.decode(w, r, c, &req)
	if !ok {
		return a, req, false
	}
	bad := !dashboards.ValidID(req.Dashboard) || req.Gadget != "" && !dashboards.ValidID(req.Gadget) ||
		req.Project != "" && !tracker.ValidID(req.Project) || req.Filter != "" && !tracker.ValidID(req.Filter) || len(req.Kind) > 20
	if bad {
		http.Error(w, "bad request", http.StatusBadRequest)
		return a, req, false
	}
	return a, req, true
}

// dashboardAction answers an action on a dashboard.
func (s *Server) dashboardAction(cmd func(a action, req dashRequest) writer.Command) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a, req, ok := s.decodeDashboard(w, r, classField); ok {
			s.act(w, r, a, cmd(a, req))
		}
	}
}

// createDashboard makes a dashboard and sends the page to it.
func (s *Server) createDashboard(w http.ResponseWriter, r *http.Request) {
	var req dashRequest
	a, ok := s.decode(w, r, classCreate, &req)
	if !ok {
		return
	}
	s.actResult(w, r, a, dashboards.Create{By: a.By, Name: req.Name}, func(res []byte) string {
		var v struct{ ID string }
		json.Unmarshal(res, &v)
		return views.DashboardHref(v.ID)
	})
}

// deleteDashboard deletes a dashboard and sends the page to the list.
func (s *Server) deleteDashboard(w http.ResponseWriter, r *http.Request) {
	a, req, ok := s.decodeDashboard(w, r, classField)
	if !ok {
		return
	}
	s.actResult(w, r, a, resultOf{dashboards.Delete{By: a.By, Dashboard: req.Dashboard}}, func([]byte) string { return "/dashboards" })
}

// resultOf is a command whose result is nothing, for actResult.
type resultOf struct{ writer.Command }

func (c resultOf) ApplyResult(tx *writer.Tx) ([]byte, error) {
	return []byte("{}"), c.Command.Apply(tx)
}

// ── The counts' cache ─────────────────────────────────────────────────

// countsCache keeps a project's issues by status and created against
// resolved per day, as of a commit's number (lith's recipe "Caching an
// aggregate against a version"): good while no commit touched the
// project's board or the project since. Every change to an issue touches
// its project's board, and the hub numbers the commits.
type countsCache struct {
	hub   *live.Hub
	mu    sync.Mutex
	m     map[string]cachedCount
	reads atomic.Int64 // counted anew, for the cache's test
}

type cachedCount struct {
	seq uint64
	val any
}

// maxCachedCounts bounds the cache: two entries a project.
const maxCachedCounts = 1024

func newCountsCache(hub *live.Hub) *countsCache {
	return &countsCache{hub: hub, m: map[string]cachedCount{}}
}

func (c *countsCache) get(key, project string, seq uint64, read func() (any, error)) (any, error) {
	c.mu.Lock()
	e, ok := c.m[key]
	c.mu.Unlock()
	if ok && c.hub.Version(tracker.BoardTopic(project), tracker.ProjectTopic(project)) <= e.seq {
		return e.val, nil
	}
	c.reads.Add(1)
	v, err := read()
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if len(c.m) >= maxCachedCounts {
		clear(c.m)
	}
	if old, ok := c.m[key]; !ok || old.seq < seq {
		c.m[key] = cachedCount{seq: seq, val: v}
	}
	c.mu.Unlock()
	return v, nil
}

func (c *countsCache) status(ctx context.Context, q sqlite.Querier, project string, seq uint64) ([]tracker.StatusCount, error) {
	v, err := c.get("status:"+project, project, seq, func() (any, error) { return tracker.StatusCounts(ctx, q, project) })
	if err != nil {
		return nil, err
	}
	return v.([]tracker.StatusCount), nil
}

func (c *countsCache) days(ctx context.Context, q sqlite.Querier, project string, now time.Time, seq uint64) ([]tracker.DayCount, error) {
	day := now.In(views.Zone).Format(time.DateOnly) // a new day counts anew
	v, err := c.get("days:"+project+":"+day, project, seq, func() (any, error) {
		return tracker.CreatedResolved(ctx, q, project, now, views.DashboardDays, views.Zone)
	})
	if err != nil {
		return nil, err
	}
	return v.([]tracker.DayCount), nil
}
