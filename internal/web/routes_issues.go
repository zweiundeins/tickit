package web

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/zweiundeins/lith/edge"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/sse"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/views"
)

// routesIssues registers the issue list, its filters, saved filters, bulk edit and dashboards (FR-50 to FR-61).
//
// The list is the view list:<query>, its query (filter, sort and page, as
// the URL has them, "me" kept) in base64url, or ".<hash>" of a query
// stored in list_queries when it is longer than a view's name allows.
// Its key is "list:" + tracker.Query.Key(): the projects the person reads,
// the filter with "me" resolved, the sort and the page, so tabs that show
// the same list share one render (MP-11). A change of the filter in the
// page posts /cmd/list/view, which moves the tab's open stream from the
// old list's view to the new one (hub.Unwatch, hub.Watch, after the
// commit and in its order), keeps the query in the tab's row and answers
// the new view's name as $_views, for the stream's next reconnect. The
// stream stays open: only the new list's frame arrives.
func (s *Server) routesIssues() {
	s.get("GET /issues", http.HandlerFunc(s.issueList), "/issues",
		"/issues?status=todo&assignee=me&sort=priority&dir=desc&page=3&q=web-1&due_from=2026-01-01&created_to=2026-12-31")
	s.get("GET /issues.csv", http.HandlerFunc(s.issueCSV), "/issues.csv", "/issues.csv?assignee=me&sort=key&dir=asc")
	s.views.Register("list", s.listViewOf)
	s.post("POST /cmd/list/view", s.listChange)
	s.filterRoutes()
}

// listState is the list area's state of a tab (FR-55.2): its query and
// the key of the view its stream watches.
type listState struct {
	Query string `json:"query"`
	Key   string `json:"key"`
}

const listArea = "list"

// listQueryOf is the canonical query string of a list's URL values, "me"
// kept: filter, sort and page.
func listQueryOf(v url.Values) string {
	return tracker.Query{Filter: tracker.ParseFilter(v), Sort: tracker.ParseSort(v), Page: tracker.ParsePage(v)}.Values().Encode()
}

// maxListArg is how long a list's query may be in a view's name.
const maxListArg = 200

// listSpec is the name of a list's view; stored reports that the name
// holds the query's hash, whose query list_queries must hold.
func listSpec(query string) (spec string, stored bool) {
	if enc := base64.RawURLEncoding.EncodeToString([]byte(query)); len(enc) <= maxListArg {
		return "list:" + enc, false
	}
	sum := sha256.Sum256([]byte(query))
	return "list:." + hex.EncodeToString(sum[:8]), true
}

func storedHash(spec string) string { return strings.TrimPrefix(spec, "list:.") }

// listQuery is the list that values name, as actor sees it.
func listQuery(ctx context.Context, q sqlite.Querier, actor people.Person, v url.Values) (tracker.Query, error) {
	readable, err := tracker.Readable(ctx, q, actor)
	if err != nil {
		return tracker.Query{}, err
	}
	f := tracker.ParseFilter(v)
	if len(f.Statuses) > 0 { // a status no project has filters nothing (FR-51.2)
		known, err := knownStatuses(ctx, q, readable)
		if err != nil {
			return tracker.Query{}, err
		}
		f.Statuses = slices.DeleteFunc(f.Statuses, func(s string) bool { return !slices.Contains(known, s) })
	}
	return tracker.Query{Readable: readable, Me: actor.ID, Filter: f, Sort: tracker.ParseSort(v), Page: tracker.ParsePage(v)}, nil
}

func knownStatuses(ctx context.Context, q sqlite.Querier, projects []string) ([]string, error) {
	list, _ := json.Marshal(projects)
	rows, err := q.QueryContext(ctx, `SELECT DISTINCT id FROM statuses WHERE project_id IN (SELECT value FROM json_each(?))`, string(list))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// listArgQuery reads the query of a list view's name: errBadView for a
// malformed one and for a hash no row holds.
func listArgQuery(ctx context.Context, q sqlite.Querier, arg string) (url.Values, error) {
	if h, ok := strings.CutPrefix(arg, "."); ok {
		var query string
		err := q.QueryRowContext(ctx, `SELECT query FROM list_queries WHERE hash = ?`, h).Scan(&query)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: no list %s", errBadView, h)
		}
		if err != nil {
			return nil, err
		}
		return url.ParseQuery(query)
	}
	b, err := base64.RawURLEncoding.DecodeString(arg)
	if err != nil {
		return nil, fmt.Errorf("%w: list %q", errBadView, arg)
	}
	v, err := url.ParseQuery(string(b))
	if err != nil {
		return nil, fmt.Errorf("%w: list %q", errBadView, arg)
	}
	return v, nil
}

// listViewOf is the view a list's name names, for actor.
func (s *Server) listViewOf(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	v, err := listArgQuery(ctx, q, arg)
	if err != nil {
		return live.View{}, err
	}
	query, err := listQuery(ctx, q, actor, v)
	if err != nil {
		return live.View{}, err
	}
	return s.listView(query), nil
}

// listView is a list's shared view: it renders again on any change to an
// issue, a project or a name, so a change that moves an issue into or out
// of the filter shows in the next frame (MP-03.1).
func (s *Server) listView(query tracker.Query) live.View {
	return live.View{
		Key:    "list:" + query.Key(),
		Topics: []string{tracker.IssuesTopic, tracker.ProjectsTopic, people.Topic},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			l, err := listData(ctx, querier(ctx, tx), query)
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.IssueListRegion(l))
		},
	}
}

// listData reads a page of a list for its view.
func listData(ctx context.Context, q sqlite.Querier, query tracker.Query) (views.IssueList, error) {
	res, err := tracker.Search(ctx, q, query)
	if err != nil {
		return views.IssueList{}, err
	}
	var ids []string
	for _, i := range res.Issues {
		ids = append(ids, i.Reporter)
		if i.Assignee != "" {
			ids = append(ids, i.Assignee)
		}
	}
	found, err := people.Lookup(ctx, q, ids...)
	if err != nil {
		return views.IssueList{}, err
	}
	v := tracker.Query{Filter: query.Filter.Normalize(query.Me), Sort: query.Sort}.Values()
	return views.IssueList{Results: res, People: found, Sort: query.Sort, Values: v, Filtered: !query.Filter.IsZero(), Now: time.Now()}, nil
}

// lastList is the query of the list the session showed last, if a tab of
// it changed one (FR-55.2).
func lastList(ctx context.Context, q sqlite.Querier, sid string) (string, error) {
	var query string
	err := q.QueryRowContext(ctx, `SELECT json_extract(data, '$.areas.list.query') FROM tab_state
		WHERE sid = ? AND json_extract(data, '$.areas.list.query') IS NOT NULL ORDER BY seen DESC LIMIT 1`, sid).Scan(&query)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return query, err
}

// issueList is /issues (FR-50 to FR-55). Without a query it shows the
// list the session showed last: a tab keeps its filter when the person
// opens an issue and comes back.
func (s *Server) issueList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if r.URL.RawQuery == "" {
		last, err := s.db.ViewValue(ctx, func(tx *sql.Tx) (string, error) { return lastList(ctx, tx, edge.SessionID(ctx)) })
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if last != "" {
			http.Redirect(w, r, "/issues?"+last, http.StatusSeeOther)
			return
		}
	}
	v := r.URL.Query()
	actor := people.Actor(ctx)
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		query, err := listQuery(ctx, q, actor, v)
		if err != nil {
			return Page{}, err
		}
		l, err := listData(ctx, q, query)
		if err != nil {
			return Page{}, err
		}
		opts, err := tracker.ListOptions(ctx, q, query.Readable)
		if err != nil {
			return Page{}, err
		}
		found, err := people.Lookup(ctx, q, opts.People...)
		if err != nil {
			return Page{}, err
		}
		saved, err := tracker.SavedFilters(ctx, q, actor.ID)
		if err != nil {
			return Page{}, err
		}
		p := views.ListPage{List: l, Filter: query.Filter, Sort: query.Sort, Page: query.Page, Options: opts, People: found, Saved: saved,
			Query: tracker.Query{Filter: query.Filter, Sort: query.Sort}.Values().Encode(), Me: actor.ID, Moderates: access.SiteCan(actor, access.Moderate), Deletes: deletes(ctx, q, actor, query.Readable)}
		spec, stored := listSpec(listQueryOf(v))
		if stored {
			if _, err := listArgQuery(ctx, q, "."+storedHash(spec)); err != nil {
				spec, p.Unwatched = "", true
			}
		}
		var names []string
		if spec != "" {
			names = append(names, spec)
		}
		return Page{Title: "Issues", Width: "full", Views: names, Body: views.IssueListBody(p)}, nil
	})
}

// listChange moves a tab's stream to the list that the page's filter
// form now names, and keeps the list in the tab's row (FR-52.2, FR-55).
// It answers the list's view name as $_views.
func (s *Server) listChange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tab   string `json:"tab"`
		View  string `json:"view"`
		Query string `json:"query"`
	}
	if !s.allow(w, r, classView) {
		return
	}
	ctx := r.Context()
	actor, sid := people.Actor(ctx), edge.SessionID(ctx)
	if err := sse.ReadSignalsLimit(w, r, &req, 8<<10); err != nil || !validID(req.Tab) || actor.IsZero() {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if mustChange(w, actor) || !s.connected(w, r, actor, req.Tab) {
		return
	}
	v, err := url.ParseQuery(req.Query)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	canon := listQueryOf(v)
	spec, stored := listSpec(canon)
	var view live.View
	var prev string // the key of the list the page's stream watched
	err = s.db.View(ctx, func(tx *sql.Tx) error {
		query, err := listQuery(ctx, tx, actor, v)
		if err != nil {
			return err
		}
		view = s.quiet(s.listView(query))
		if arg, ok := strings.CutPrefix(req.View, "list:"); ok && !strings.Contains(arg, ",") {
			if old, err := s.listViewOf(ctx, tx, actor, arg); err == nil {
				prev = old.Key
			}
		}
		return nil
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	id, now := streamID(actor, req.Tab), time.Now()
	err = s.w.Do(ctx, writer.Func(func(tx *writer.Tx) error {
		if stored {
			if _, err := tx.Exec(`INSERT INTO list_queries (hash, query, seen) VALUES (?, ?, ?)
				ON CONFLICT (hash) DO UPDATE SET seen = excluded.seen`, storedHash(spec), canon, now.Unix()); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM list_queries WHERE seen < ?`, now.Add(-tabs.TTL).Unix()); err != nil {
				return err
			}
		}
		var old string
		err := tabs.Update(tx, sid, req.Tab, listArea, now, func(st *listState) error {
			old, st.Query, st.Key = st.Key, canon, view.Key
			return nil
		})
		if err != nil {
			return err
		}
		// In the order of the commits: of two quick changes, the later
		// list is the one the stream keeps.
		tx.AfterCommit(func() {
			for _, k := range []string{old, prev} {
				if k != "" && k != view.Key {
					s.hub.Unwatch(id, k)
				}
			}
			s.hub.Watch(id, view)
		})
		return nil
	}))
	if err != nil {
		s.failAction(w, r, err)
		return
	}
	s.signals(w, map[string]any{"_views": spec})
}

// issueCSV exports the list's whole result, in its order (FR-152.1).
func (s *Server) issueCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := people.Actor(ctx)
	var list []tracker.Issue
	var names map[string]people.Person
	err := s.db.View(ctx, func(tx *sql.Tx) error {
		query, err := listQuery(ctx, tx, actor, r.URL.Query())
		if err != nil {
			return err
		}
		if list, err = tracker.SearchAll(ctx, tx, query, tracker.MaxExport); err != nil {
			return err
		}
		var ids []string
		for _, i := range list {
			ids = append(ids, i.Reporter, i.Assignee)
		}
		names, err = people.Lookup(ctx, tx, ids...)
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/csv; charset=utf-8")
	h.Set("Content-Disposition", `attachment; filename="issues.csv"`)
	h.Set("Cache-Control", "no-store")
	c := csv.NewWriter(w)
	c.Write([]string{"Key", "Type", "Summary", "Priority", "Status", "Assignee", "Reporter", "Project", "Created", "Updated", "Due", "Labels"})
	for _, i := range list {
		assignee := ""
		if i.Assignee != "" {
			assignee = names[i.Assignee].Name
		}
		c.Write([]string{i.Key, i.Type.Name(), cell(i.Summary), i.Priority.Name(), cell(i.StatusName), cell(assignee), cell(names[i.Reporter].Name),
			cell(i.ProjectName), i.Created.UTC().Format(time.RFC3339), i.Updated.UTC().Format(time.RFC3339), i.Due, cell(strings.Join(i.Labels, " "))})
	}
	c.Flush()
}

// cell keeps a spreadsheet from reading a person's text as a formula.
func cell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// filterRoutes registers saved filters (FR-56): /filters lists every one
// (everyone sees all in the demo), the person's starred first; actions
// save the list's filter, star, rename and delete. The page's view,
// filters:, is the person's own (key filters:<person>): it marks their
// stars and the filters they may change.
func (s *Server) filterRoutes() {
	s.get("GET /filters", http.HandlerFunc(s.filtersPage))
	s.views.Register("filters", func(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
		if arg != "" {
			return live.View{}, fmt.Errorf("%w: filters:%s", errBadView, arg)
		}
		return s.filtersView(actor), nil
	})
	s.post("POST /cmd/filters/save", s.saveFilter)
	s.post("POST /cmd/filters/rename", s.renameFilter)
	s.post("POST /cmd/filters/delete", s.deleteFilter)
	s.post("POST /cmd/filters/star", s.starFilter)
}

func filtersData(ctx context.Context, q sqlite.Querier, actor people.Person) (views.SavedFilters, error) {
	list, err := tracker.SavedFilters(ctx, q, actor.ID)
	if err != nil {
		return views.SavedFilters{}, err
	}
	var ids []string
	for _, f := range list {
		ids = append(ids, f.Owner)
		ids = append(ids, f.Filter.Assignees...)
		ids = append(ids, f.Filter.Reporters...)
	}
	found, err := people.Lookup(ctx, q, ids...)
	if err != nil {
		return views.SavedFilters{}, err
	}
	return views.SavedFilters{Filters: list, People: found, Me: actor}, nil
}

func (s *Server) filtersView(actor people.Person) live.View {
	return live.View{
		Key:    "filters:" + actor.ID,
		Topics: []string{tracker.FiltersTopic, people.Topic},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			d, err := filtersData(ctx, querier(ctx, tx), actor)
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.SavedFiltersRegion(d))
		},
	}
}

func (s *Server) filtersPage(w http.ResponseWriter, r *http.Request) {
	actor := people.Actor(r.Context())
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		d, err := filtersData(ctx, q, actor)
		if err != nil {
			return Page{}, err
		}
		return Page{Title: "Filters", Nav: "filters", Views: []string{"filters:"}, Body: views.SavedFiltersBody(d)}, nil
	})
}

// saveFilter saves the list's filter and sort under a name (FR-56.1).
func (s *Server) saveFilter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Payload
		Name  string `json:"name"`
		Query string `json:"query"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	v, err := url.ParseQuery(req.Query)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.act(w, r, a, tracker.SaveFilter{By: a.By, Name: req.Name, Filter: tracker.ParseFilter(v), Sort: tracker.ParseSort(v)})
}

type filterRequest struct {
	Payload
	Filter string `json:"filter"`
	Name   string `json:"name"`
	Star   bool   `json:"star"`
}

func (s *Server) decodeFilter(w http.ResponseWriter, r *http.Request) (action, filterRequest, bool) {
	var req filterRequest
	a, ok := s.decode(w, r, classField, &req)
	if ok && !tracker.ValidID(req.Filter) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return a, req, false
	}
	return a, req, ok
}

// renameFilter renames a saved filter: its owner does (FR-56.3).
func (s *Server) renameFilter(w http.ResponseWriter, r *http.Request) {
	if a, req, ok := s.decodeFilter(w, r); ok {
		s.act(w, r, a, tracker.RenameFilter{By: a.By, Filter: req.Filter, Name: req.Name})
	}
}

// deleteFilter deletes a saved filter: its owner does (FR-56.3).
func (s *Server) deleteFilter(w http.ResponseWriter, r *http.Request) {
	if a, req, ok := s.decodeFilter(w, r); ok {
		s.act(w, r, a, tracker.DeleteFilter{By: a.By, Filter: req.Filter})
	}
}

// starFilter stars a saved filter for the person, or takes the star off.
func (s *Server) starFilter(w http.ResponseWriter, r *http.Request) {
	if a, req, ok := s.decodeFilter(w, r); ok {
		s.act(w, r, a, tracker.StarFilter{By: a.By, Filter: req.Filter, Star: req.Star})
	}
}
