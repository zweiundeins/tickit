package web

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/activity"
	"github.com/zweiundeins/tickit/internal/doc"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/ui"
	"github.com/zweiundeins/tickit/internal/views"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// routesPeople registers a person's page (FR-67.2), where a mention
// links: their face and name, their issues and what they did lately. Its
// view is profile:<id>, keyed by the person and the scopes the viewer
// reads, so viewers who read the same share one render.
func (s *Server) routesPeople() {
	s.get("GET /people/{id}", http.HandlerFunc(s.personPage), "/people/s-ada", "/people/g-aaaaaaaaaaaaaaaaaaaa", "/people/nope", "/people/a-01920000-0000-7000-8000-000000000001")
	s.views.Register("profile", s.profileView)
}

// person is the person an id names, access.ErrNotFound for nobody.
func person(ctx context.Context, q sqlite.Querier, id string) (people.Person, error) {
	if !people.ValidID(id) {
		return people.Person{}, fmt.Errorf("person %q: %w", id, access.ErrNotFound)
	}
	p, err := people.Get(ctx, q, id)
	if err != nil {
		return p, err
	}
	if p.Kind == people.Deleted {
		return p, fmt.Errorf("person %s: %w", id, access.ErrNotFound)
	}
	return p, nil
}

func (s *Server) personPage(w http.ResponseWriter, r *http.Request) {
	me := people.Actor(r.Context())
	id := r.PathValue("id")
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		p, err := person(ctx, q, id)
		if err != nil {
			return Page{}, err
		}
		readable, err := activity.Readable(ctx, q, me)
		if err != nil {
			return Page{}, err
		}
		pp, err := personData(ctx, q, p, readable, time.Now())
		if err != nil {
			return Page{}, err
		}
		return Page{Title: p.Name, Views: []string{"profile:" + p.ID}, Body: views.PersonBody(pp)}, nil
	})
}

// personData reads a person's page as a viewer who reads readable sees it.
func personData(ctx context.Context, q sqlite.Querier, p people.Person, readable []access.Scope, now time.Time) (views.PersonPage, error) {
	pp := views.PersonPage{Person: p, Now: now}
	var projects []string
	for _, sc := range readable {
		if sc.Kind == access.Project {
			projects = append(projects, sc.ID)
		}
	}
	var err error
	if pp.Assigned, err = tracker.Search(ctx, q, tracker.Query{Readable: projects, Page: 1,
		Filter: tracker.Filter{Assignees: []string{p.ID}, Categories: []tracker.Category{tracker.ToDo, tracker.InProgress}},
		Sort:   tracker.Sort{By: tracker.SortUpdated, Desc: true}}); err != nil {
		return pp, err
	}
	if pp.Reported, err = tracker.Search(ctx, q, tracker.Query{Readable: projects, Page: 1,
		Filter: tracker.Filter{Reporters: []string{p.ID}}, Sort: tracker.DefaultSort}); err != nil {
		return pp, err
	}
	entries, err := activity.By(ctx, q, p.ID, activity.In(readable), 20)
	if err != nil {
		return pp, err
	}
	feed := views.ActivityFeed{ID: views.ActivityID("person"), Entries: entries, People: map[string]people.Person{p.ID: p}, Now: now,
		Label: "What " + p.Name + " did lately"}
	pp.Feed = feed
	return pp, nil
}

// profileView is profile:<person id>: a person's page for everyone who
// reads the same projects and spaces (FR-67.2, FR-69.2).
func (s *Server) profileView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	p, err := person(ctx, q, arg)
	if err != nil {
		return live.View{}, err
	}
	readable, err := activity.Readable(ctx, q, actor)
	if err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    "profile:" + p.ID + ":" + activity.Key(readable),
		Topics: feedTopics,
		Every:  time.Minute,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			q := querier(ctx, tx)
			now, err := people.Get(ctx, q, p.ID)
			if err != nil {
				return nil, err
			}
			pp, err := personData(ctx, q, now, readable, time.Now())
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.PersonRegion(pp))
		},
	}, nil
}

// resolveFaces is wiki.Resolve with the faces of the people a document
// mentions, which its mentions show beside their names (FR-67.2).
func resolveFaces(ctx context.Context, q sqlite.Querier, refs doc.Refs) (doc.StaticResolver, error) {
	res, err := wiki.Resolve(ctx, q, refs)
	if err != nil || len(res.People) == 0 {
		return res, err
	}
	ids := make([]string, 0, len(res.People))
	for id := range res.People {
		ids = append(ids, id)
	}
	found, err := people.Lookup(ctx, q, ids...)
	if err != nil {
		return res, err
	}
	res.Faces = make(map[string]string, len(found))
	for id, p := range found {
		if p.Kind != people.Deleted {
			res.Faces[id] = ui.FaceHTML(views.UIPerson(p))
		}
	}
	return res, nil
}
