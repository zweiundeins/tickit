package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/a-h/templ"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/sse"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/activity"
	"github.com/zweiundeins/tickit/internal/notify"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/recent"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/ui"
	"github.com/zweiundeins/tickit/internal/views"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// routesNotify registers the home page (FR-59), notifications (FR-110 to
// FR-117) and the activity feeds (FR-69): the bell in the person view,
// marking read and dismissing, and the feeds' views for the home page,
// projects and spaces.
func (s *Server) routesNotify() {
	s.get("GET /{$}", http.HandlerFunc(s.home), "/", "/?from=50", "/?from=nope")
	// The stream with this area's views, for the test that GET never writes.
	s.probes = append(s.probes, "/stream?tab=probe&v=home:,activity-home:0,activity-home:20,activity-project:nope,activity-space:nope")
	s.post("POST /cmd/notify/read", s.notifyAction(func(a action, id int64) writer.Command { return notify.Read{By: a.By, ID: id} }))
	s.post("POST /cmd/notify/dismiss", s.notifyAction(func(a action, id int64) writer.Command { return notify.Dismiss{By: a.By, ID: id} }))
	s.post("POST /cmd/notify/read-all", s.notifyAction(func(a action, _ int64) writer.Command { return notify.ReadAll{By: a.By} }))
	s.post("POST /cmd/notify/dismiss-all", s.notifyAction(func(a action, _ int64) writer.Command { return notify.DismissAll{By: a.By} }))
	s.post("POST /cmd/notify/open", s.notifyOpen)
	s.post("POST /cmd/notify/seen", s.notifySeen)
	s.views.Register("home", s.homeView)
	s.routesFeed()
}

// personView is what concerns one person in every tab of theirs: their
// name in the header, the bell's count, its live region and its panel,
// and $_unread for the window title (FR-110, MP-01.2). Commands that
// change it touch people.PersonTopic(id); names come with people.Topic,
// and relative times move on every minute.
func (s *Server) personView(p people.Person) live.View {
	key := people.PersonTopic(p.ID)
	return live.View{
		Key:    key,
		Topics: []string{key, people.Topic},
		Every:  time.Minute,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			q := querier(ctx, tx)
			me, err := people.Get(ctx, q, p.ID)
			if err != nil {
				return nil, err
			}
			bell, err := s.bell(ctx, q, me)
			if err != nil {
				return nil, err
			}
			out, err := patch(ctx, templ.Join(views.PersonName(me), ui.BellCount(bell.Unread), ui.UnreadStatus(bell.Unread), views.NotificationPanel(bell)))
			if err != nil {
				return nil, err
			}
			unread, _ := json.Marshal(map[string]int{views.UnreadSignal: bell.Unread})
			return append(out, sse.PatchSignals(unread, false)...), nil
		},
	}
}

// bell reads what the header's bell shows for a person: render.go puts it
// in every document.
func (s *Server) bell(ctx context.Context, q sqlite.Querier, me people.Person) (views.Bell, error) {
	b := views.Bell{Now: time.Now()}
	if me.IsZero() {
		return b, nil
	}
	var err error
	if b.Panel, err = notify.PanelOf(ctx, q, me.ID); err != nil {
		return b, err
	}
	ids := make([]string, 0, len(b.Items))
	for _, n := range b.Items {
		ids = append(ids, n.Actor)
	}
	b.People, err = people.Lookup(ctx, q, ids...)
	return b, err
}

// notifyAction answers an action on one notification ({id}) or on all.
func (s *Server) notifyAction(cmd func(a action, id int64) writer.Command) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Payload
			ID int64 `json:"id"`
		}
		a, ok := s.decode(w, r, classField, &req)
		if !ok {
			return
		}
		s.act(w, r, a, cmd(a, req.ID))
	}
}

// notifyOpen marks a notification read and sends the page to its subject
// (FR-115.2).
func (s *Server) notifyOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Payload
		ID int64 `json:"id"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	s.actResult(w, r, a, notify.Open{By: a.By, ID: req.ID}, func(b []byte) string {
		var res struct {
			Href string `json:"href"`
		}
		json.Unmarshal(b, &res)
		return localPath(res.Href)
	})
}

// notifySeen marks the person's notifications about a subject read, as a
// page that opened it asks (views.NotifySeen, FR-115.2).
func (s *Server) notifySeen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Payload
		Subject string `json:"subject"`
		Kind    string `json:"kind"`
	}
	a, ok := s.decode(w, r, classView, &req)
	if !ok {
		return
	}
	var cmd writer.Command = notify.Seen{By: a.By, Subject: req.Subject}
	if req.Kind != "" { // and a view for "Recently viewed" (FR-59.1), of what the person may read
		ctx := r.Context()
		err := s.db.View(ctx, func(tx *sql.Tx) error {
			switch req.Kind {
			case recent.Issue:
				_, err := readableIssue(ctx, tx, a.By.Actor, req.Subject)
				return err
			case recent.Page:
				_, _, err := readable(ctx, tx, a.By.Actor, req.Subject)
				return err
			}
			return errField
		})
		switch {
		case errors.Is(err, errField):
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		case err != nil:
			s.failOp(w, r, a, err)
			return
		}
		cmd = also{cmd, recent.Record{By: a.By, Kind: req.Kind, Subject: req.Subject}}
	}
	s.act(w, r, a, cmd)
}

// home is the home page (FR-59): the person's work and the feed of what
// they may read, its window from ?from=n.
func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	me := people.Actor(r.Context())
	from := feedFrom(r)
	seq := s.hub.Seq() // before the transaction: the commits the cache may count as read
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		readable, err := activity.Readable(ctx, q, me)
		if err != nil {
			return Page{}, err
		}
		now := time.Now()
		work, err := s.work(ctx, q, me, readable, now, seq)
		if err != nil {
			return Page{}, err
		}
		feed, err := s.homeFeed(ctx, q, readable, from, now)
		if err != nil {
			return Page{}, err
		}
		return Page{
			Views:      []string{"home:", views.FeedSpec(homeFeedKind, "", feed.From)},
			Components: feedComponents,
			Scripts:    []string{views.FeedScript},
			Body:       views.Home(work, feed),
		}, nil
	})
}

// work reads a person's part of the home page; seq is what the projects'
// cache may count as read (projectsCache.get).
func (s *Server) work(ctx context.Context, q sqlite.Querier, me people.Person, readable []access.Scope, now time.Time, seq uint64) (views.Work, error) {
	w := views.Work{Me: me, Now: now}
	var projects []string
	for _, sc := range readable {
		if sc.Kind == access.Project {
			projects = append(projects, sc.ID)
		}
	}
	res, err := tracker.Search(ctx, q, tracker.Query{
		Readable: projects, Me: me.ID, Page: 1,
		Filter: tracker.Filter{Assignees: []string{tracker.Me}, Categories: []tracker.Category{tracker.ToDo, tracker.InProgress}},
		Sort:   tracker.Sort{By: tracker.SortUpdated, Desc: true},
	})
	if err != nil {
		return w, err
	}
	w.Assigned, w.Total = res.Issues[:min(len(res.Issues), views.HomeAssigned)], res.Total
	if w.Recent, err = activity.Mine(ctx, q, me.ID, activity.In(readable), 8); err != nil {
		return w, err
	}
	if w.Viewed, err = recentlyViewed(ctx, q, me, readable); err != nil {
		return w, err
	}
	if w.Projects, err = s.projects.get(ctx, q, projects, seq); err != nil {
		return w, err
	}
	for _, sc := range readable {
		if sc.Kind != access.Space {
			continue
		}
		info, err := access.Get(ctx, q, sc)
		if err != nil {
			return w, err
		}
		if info.Archived {
			continue
		}
		name := info.Key
		switch n, err := wiki.SpaceName(ctx, q, sc.ID); {
		case err == nil:
			name = n
		case !errors.Is(err, access.ErrNotFound):
			return w, err
		}
		w.Spaces = append(w.Spaces, views.Space{Key: info.Key, Name: name})
	}
	return w, nil
}

// recentlyViewed are the issues and pages a person viewed lately that
// they may still read (FR-59.1), newest first.
func recentlyViewed(ctx context.Context, q sqlite.Querier, me people.Person, readable []access.Scope) ([]views.RecentItem, error) {
	if me.IsZero() {
		return nil, nil
	}
	seen, err := recent.List(ctx, q, me.ID)
	if err != nil || len(seen) == 0 {
		return nil, err
	}
	var ids []string
	for _, v := range seen {
		if v.Kind == recent.Issue {
			ids = append(ids, v.Subject)
		}
	}
	issues, err := tracker.IssuesByID(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	keep := activity.In(readable)
	var out []views.RecentItem
	for _, v := range seen {
		if len(out) == views.HomeViewed {
			break
		}
		switch v.Kind {
		case recent.Issue:
			i, ok := issues[v.Subject]
			if ok && keep(access.ProjectScope(i.ProjectID)) {
				out = append(out, views.RecentItem{Kind: v.Kind, Href: "/" + i.Key, Title: i.Summary, Key: i.Key, Type: i.Type, At: v.At})
			}
		case recent.Page:
			p, err := wiki.PageByID(ctx, q, v.Subject)
			if errors.Is(err, access.ErrNotFound) || err == nil && (p.State != wiki.Current || !keep(access.SpaceScope(p.Space))) {
				continue
			}
			if err != nil {
				return nil, err
			}
			sp, err := wiki.SpaceByID(ctx, q, p.Space)
			if err != nil {
				return nil, err
			}
			out = append(out, views.RecentItem{Kind: v.Kind, Href: wiki.PageURL(sp.Key, p.ID, p.Title), Title: p.Title, Space: sp.Name, At: v.At})
		}
	}
	return out, nil
}

// feedTopics are what the feeds and the home page's own view listen to:
// the topics of the events they show (issues and their comments, projects,
// pages with their versions and comments, spaces) and names. Not
// events.Topic, which every append touches: co-editing appends many
// commits a second, and a feed shows a page's versions, never its commits.
var feedTopics = []string{tracker.IssuesTopic, tracker.ProjectsTopic, wiki.PagesTopic, wiki.SpacesTopic, people.Topic}

// homeView is the person's part of the home page: their issues, what they
// changed, their projects and spaces. It renders on every change to an
// issue, a project, a page or a space, as an assignment, a status or a new
// project changes it, and on the person's own (a membership).
func (s *Server) homeView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	if arg != "" {
		return live.View{}, fmt.Errorf("%w: home takes no argument", errBadView)
	}
	return live.View{
		Key:    "home:" + actor.ID,
		Topics: append(slices.Clone(feedTopics), people.PersonTopic(actor.ID), recent.Topic(actor.ID)),
		Every:  time.Minute,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			q := querier(ctx, tx)
			readable, err := activity.Readable(ctx, q, actor)
			if err != nil {
				return nil, err
			}
			w, err := s.work(ctx, q, actor, readable, time.Now(), live.Seq(ctx))
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.HomeWork(w))
		},
	}, nil
}

// feed is a feed's entries with the people they name.
func (s *Server) feed(ctx context.Context, q sqlite.Querier, id string, entries []activity.Entry, now time.Time) (views.ActivityFeed, error) {
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.Actor)
	}
	found, err := people.Lookup(ctx, q, ids...)
	return views.ActivityFeed{ID: id, Entries: entries, People: found, Now: now}, err
}
