package web

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/search"
	"github.com/mbolli/tickit/internal/tracker"
	"github.com/mbolli/tickit/internal/views"
)

// routesSearch registers search across issues and pages (FR-57): the
// results page /search?q=, whose results are the shared view
// search:<params in base64url> (key: the readable scopes and the params),
// and the typeahead GET /search/suggest?q=, a read that answers a patch of
// #search-suggest with the first 5 issues and 5 pages. GET never writes.
func (s *Server) routesSearch() {
	s.get("GET /search", http.HandlerFunc(s.searchPage), "/search", "/search?q=web-1&kind=issue&project=WEB&label=ux&page=2",
		"/search?q=zebra&kind=page&space=ENG&person=s-ada")
	s.get("GET /search/suggest", http.HandlerFunc(s.searchSuggest), "/search/suggest", "/search/suggest?q=we")
	s.views.Register("search", s.searchViewOf)
}

// MaxSearchFilter is how many values one filter of a search takes.
const MaxSearchFilter = 20

// parseSearch reads a search from its URL parameters.
func parseSearch(v url.Values) views.SearchParams {
	q := strings.Join(strings.Fields(v.Get("q")), " ")
	if utf8.RuneCountInString(q) > search.MaxText {
		q = string([]rune(q)[:search.MaxText])
	}
	list := func(in []string, norm func(string) (string, bool)) []string {
		var out []string
		for _, x := range in {
			if n, ok := norm(strings.TrimSpace(x)); ok && n != "" && !slices.Contains(out, n) && len(out) < MaxSearchFilter {
				out = append(out, n)
			}
		}
		slices.Sort(out)
		return out
	}
	p := views.SearchParams{
		Q:        q,
		Projects: list(v["project"], tracker.NormalizeKey),
		Spaces:   list(v["space"], tracker.NormalizeKey),
		Labels:   list(v["label"], tracker.NormalizeLabel),
		People:   list(v["person"], func(s string) (string, bool) { return s, people.ValidID(s) }),
		Page:     tracker.ParsePage(v),
	}
	if k := v.Get("kind"); k == string(search.Issue) || k == string(search.Page) {
		p.Kind = k
	}
	return p
}

// scopes are what a person may search: the projects and the spaces they
// may read.
type scopes struct {
	projects []tracker.Project
	ids      []string          // the projects' ids
	spaces   map[string]string // space key to id
}

func readableScopes(ctx context.Context, q sqlite.Querier, actor people.Person) (scopes, error) {
	var sc scopes
	var err error
	if sc.ids, err = tracker.Readable(ctx, q, actor); err != nil {
		return sc, err
	}
	if sc.projects, err = tracker.Projects(ctx, q, sc.ids); err != nil {
		return sc, err
	}
	rows, err := q.QueryContext(ctx, `SELECT id, key FROM scopes WHERE kind = 'space' ORDER BY key`)
	if err != nil {
		return sc, err
	}
	all := map[string]string{}
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			rows.Close()
			return sc, err
		}
		all[key] = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return sc, err
	}
	sc.spaces = map[string]string{}
	for key, id := range all {
		ok, err := access.Can(ctx, q, actor, access.Read, access.SpaceScope(id))
		if err != nil && !errors.Is(err, access.ErrNotFound) {
			return sc, err
		}
		if ok {
			sc.spaces[key] = id
		}
	}
	return sc, nil
}

func (sc scopes) spaceIDs() []string {
	var out []string
	for _, id := range sc.spaces {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// query is the index's query of a search, for the person of sc.
func (sc scopes) query(p views.SearchParams) search.Query {
	q := search.Query{Text: p.Q, Projects: sc.ids, Spaces: sc.spaceIDs(), Labels: p.Labels, People: p.People,
		Offset: (max(p.Page, 1) - 1) * search.PageSize, Limit: search.PageSize}
	if p.Kind != "" {
		q.Kinds = []search.Kind{search.Kind(p.Kind)}
	}
	for _, key := range p.Projects {
		for _, pr := range sc.projects {
			if pr.Key == key {
				q.InProjects = append(q.InProjects, pr.ID)
			}
		}
	}
	if len(p.Projects) > 0 && len(q.InProjects) == 0 {
		q.InProjects = []string{"none"} // only projects the person may not read: nothing
	}
	for _, key := range p.Spaces {
		if id, ok := sc.spaces[key]; ok {
			q.InSpaces = append(q.InSpaces, id)
		}
	}
	if len(p.Spaces) > 0 && len(q.InSpaces) == 0 {
		q.InSpaces = []string{"none"}
	}
	return q
}

// key is a search's view key: what it reads and what it asks.
func (sc scopes) key(p views.SearchParams) string {
	sum := sha256.Sum256([]byte(strings.Join(sc.ids, ",") + "\n" + strings.Join(sc.spaceIDs(), ",") + "\n" + p.Values().Encode()))
	return "search:" + hex.EncodeToString(sum[:8])
}

// searchData reads a page of results; a page past the end is the last.
func searchData(ctx context.Context, q sqlite.Querier, sc scopes, p views.SearchParams) (views.SearchResults, error) {
	res, err := search.Search(ctx, q, sc.query(p))
	if err != nil {
		return views.SearchResults{}, err
	}
	if pages := max(1, (res.Total+search.PageSize-1)/search.PageSize); p.Page > pages {
		p.Page = pages
		if res, err = search.Search(ctx, q, sc.query(p)); err != nil {
			return views.SearchResults{}, err
		}
	}
	var ids []string
	for _, h := range res.Hits {
		if h.Kind == search.Issue {
			ids = append(ids, h.ID)
		}
	}
	issues, err := tracker.IssuesByID(ctx, q, ids)
	if err != nil {
		return views.SearchResults{}, err
	}
	return views.SearchResults{Params: p, Results: res, Issues: issues, Now: time.Now()}, nil
}

// searchView is the results' shared view: it renders again when the index
// changes, and when issues or names do (statuses, projects).
func (s *Server) searchView(sc scopes, p views.SearchParams) live.View {
	topics := append([]string{search.Topic, tracker.IssuesTopic, people.Topic}, search.ProjectionTopics...)
	return live.View{
		Key:    sc.key(p),
		Topics: topics,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			r, err := searchData(ctx, querier(ctx, tx), sc, p)
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.SearchResultsRegion(r))
		},
	}
}

func searchSpec(p views.SearchParams) string {
	enc := base64.RawURLEncoding.EncodeToString([]byte(p.Values().Encode()))
	if len(enc) > maxListArg {
		return "" // a search too long for a view's name stays as loaded
	}
	return "search:" + enc
}

func (s *Server) searchViewOf(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	b, err := base64.RawURLEncoding.DecodeString(arg)
	if err != nil {
		return live.View{}, fmt.Errorf("%w: search %q", errBadView, arg)
	}
	v, err := url.ParseQuery(string(b))
	if err != nil {
		return live.View{}, fmt.Errorf("%w: search %q", errBadView, arg)
	}
	sc, err := readableScopes(ctx, q, actor)
	if err != nil {
		return live.View{}, err
	}
	return s.searchView(sc, parseSearch(v)), nil
}

// searchPage is /search?q= (FR-57.2).
func (s *Server) searchPage(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	if v.Get("q") == "" && !tracker.ParseFilter(v).IsZero() {
		http.Redirect(w, r, "/issues?"+r.URL.RawQuery, http.StatusSeeOther) // a list's filter: the issue list shows it
		return
	}
	p := parseSearch(v)
	actor := people.Actor(r.Context())
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		sc, err := readableScopes(ctx, q, actor)
		if err != nil {
			return Page{}, err
		}
		res, err := searchData(ctx, q, sc, p)
		if err != nil {
			return Page{}, err
		}
		opts, err := tracker.ListOptions(ctx, q, sc.ids)
		if err != nil {
			return Page{}, err
		}
		names, err := people.Lookup(ctx, q, append(slices.Clone(opts.People), p.People...)...)
		if err != nil {
			return Page{}, err
		}
		page := views.SearchPage{Results: res, Projects: sc.projects, Labels: opts.Labels, People: opts.People, Names: names,
			Me: actor.ID, Moderates: access.SiteCan(actor, access.Moderate)}
		for key := range sc.spaces {
			page.Spaces = append(page.Spaces, key)
		}
		slices.Sort(page.Spaces)
		var watch []string
		if spec := searchSpec(p); spec != "" && p.Q != "" {
			watch = append(watch, spec)
		}
		title := "Search"
		if p.Q != "" {
			title = p.Q + " · Search"
		}
		return Page{Title: title, Query: p.Q, Views: watch, Body: views.SearchBody(page)}, nil
	})
}

// searchSuggest answers the header's typeahead (FR-57.1): the first 5
// issues and 5 pages for the text, as a patch of #search-suggest. An exact
// key comes first (FR-52.1).
func (s *Server) searchSuggest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	text := parseSearch(r.URL.Query()).Q
	actor := people.Actor(ctx)
	sg := views.Suggestions{Q: text}
	err := s.db.View(ctx, func(tx *sql.Tx) error {
		if text == "" {
			return nil
		}
		sc, err := readableScopes(ctx, tx, actor)
		if err != nil {
			return err
		}
		q := sc.query(views.SearchParams{Q: text})
		q.Limit, q.Offset = search.Suggest, 0
		for _, k := range search.Kinds {
			q.Kinds = []search.Kind{k}
			res, err := search.Search(ctx, tx, q)
			if err != nil {
				return err
			}
			if k == search.Issue {
				sg.Issues = res.Hits
			} else {
				sg.Pages = res.Hits
			}
		}
		return nil
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	b, err := patch(ctx, views.SearchSuggest(sg))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	w.Write(b)
}
