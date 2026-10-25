package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/demo"
	"github.com/zweiundeins/tickit/internal/docnodes"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/views"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// routesEmbed registers issue lists on pages (FR-104) and issues created
// from a page's text (FR-105).
//
// An issue list is a block of the document (docnodes.IssueList: a saved
// filter or a filter in the issue list's query syntax, and a row cap).
// The reader view (pagedoc:<page>, routes_wiki.go) renders its rows in
// place: its key holds the projects the reader may read, and a render of
// a page with lists declares the issues, projects and saved filters as
// topics (live.Depends), so a change that moves an issue into or out of a
// list shows in the next frame. The rows of one list for one set of
// readable projects render once per tick whatever page and reader asks
// (listsCache). In the editor, the view pagelists:<page> patches the
// page's hidden lists element with every list's rows and the saved
// filters, which the editor's node views copy. Versions and comparisons
// show a list's description (docnodes.ListDescription) without rows.
//
// POST /cmd/pages/issue {tab, op, page, project, summary, find, block,
// nth} creates a task from a page's selected text, links the page to it
// (FR-102) and, in view mode (find set), puts its chip in place of the
// text (wiki.ChipSelection). It answers $_pissue.key and $_pissue.chip;
// the editor puts the chip in itself, as a commit of its own.
func (s *Server) routesEmbed() {
	s.post("POST /cmd/pages/issue", s.pageIssue)
	s.views.Register("pagelists", s.pageListsView)
}

// listTopics are what an issue list's rows depend on beyond names.
var listTopics = []string{tracker.IssuesTopic, tracker.ProjectsTopic, tracker.FiltersTopic}

// rightsKey names a set of readable projects in a view's key.
func rightsKey(readable []string) string {
	sum := sha256.Sum256([]byte(strings.Join(slices.Sorted(slices.Values(readable)), ",")))
	return hex.EncodeToString(sum[:8])
}

// listsFor is how a page shows its issue lists to a reader: the projects
// they may read, and the page's creator, who is "me" in a filter.
type listsFor struct {
	readable []string
	owner    string
	seq      uint64 // live.Seq in a Render; 0 reads without the cache
}

// listResolver resolves a page's atoms and fills its issue lists.
type listResolver struct {
	docnodes.StaticResolver
	list func(docnodes.IssueList) string
}

func (r listResolver) List(l docnodes.IssueList) string { return r.list(l) }

// describeLists is a version's resolver: its issue lists show their
// filters, saved filters by their names, never rows (FR-104).
func describeLists(ctx context.Context, q sqlite.Querier, res docnodes.StaticResolver) docnodes.Resolver {
	return listResolver{res, func(l docnodes.IssueList) string {
		name := ""
		if l.SavedFilter != "" {
			if sf, err := tracker.SavedFilterByID(ctx, q, l.SavedFilter); err == nil {
				name = sf.Name
			}
		}
		return docnodes.ListDescription(l, name)
	}}
}

// listHTML is the inside of an issue list as lf sees it (FR-104); "" on
// an error, which leaves the list's description.
func (s *Server) listHTML(ctx context.Context, q sqlite.Querier, lf listsFor, l docnodes.IssueList) string {
	h, err := s.lists.get(ctx, lf.seq, l.Key()+"\n"+rightsKey(lf.readable)+"\n"+lf.owner, func() (string, error) {
		el, err := embeddedList(ctx, q, lf, l)
		if err != nil {
			return "", err
		}
		var buf bytes.Buffer
		err = views.IssueListEmbed(el).Render(ctx, &buf)
		return buf.String(), err
	})
	if err != nil && ctx.Err() == nil {
		s.log.Warn("issue list", "err", err)
	}
	return h
}

// embeddedList reads an issue list's rows.
func embeddedList(ctx context.Context, q sqlite.Querier, lf listsFor, l docnodes.IssueList) (views.EmbeddedList, error) {
	el := views.EmbeddedList{Limit: max(l.Limit, 1)}
	var f tracker.Filter
	var sort tracker.Sort
	if l.SavedFilter != "" {
		sf, err := tracker.SavedFilterByID(ctx, q, l.SavedFilter)
		if errors.Is(err, access.ErrNotFound) {
			el.Desc, el.Missing = docnodes.ListDescription(l, ""), "Its saved filter was deleted."
			return el, nil
		}
		if err != nil {
			return el, err
		}
		f, sort, el.Desc = sf.Filter, sf.Sort, docnodes.ListDescription(l, sf.Name)
	} else {
		v, _ := url.ParseQuery(l.Filter)
		f, sort, el.Desc = tracker.ParseFilter(v), tracker.ParseSort(v), docnodes.ListDescription(l, "")
	}
	if len(f.Statuses) > 0 { // a status no project has filters nothing (FR-51.2)
		known, err := knownStatuses(ctx, q, lf.readable)
		if err != nil {
			return el, err
		}
		f.Statuses = slices.DeleteFunc(f.Statuses, func(s string) bool { return !slices.Contains(known, s) })
	}
	var err error
	if el.Results, err = tracker.Search(ctx, q, tracker.Query{Readable: lf.readable, Me: lf.owner, Filter: f, Sort: sort, Page: 1}); err != nil {
		return el, err
	}
	ids := []string{lf.owner}
	for _, i := range el.Results.Issues {
		if i.Assignee != "" {
			ids = append(ids, i.Assignee)
		}
	}
	if el.People, err = people.Lookup(ctx, q, ids...); err != nil {
		return el, err
	}
	if hasMe(f) {
		el.Note = "“Me” in this filter is " + el.People[lf.owner].Name + ", who created this page."
	}
	el.Href = "/issues?" + tracker.Query{Filter: f, Sort: sort}.Values().Encode()
	return el, nil
}

// listsCache keeps the HTML of issue lists by list, readable projects and
// page creator, as of a commit's number: good while no commit touched
// issues, projects, saved filters or names since (lith's Hub.Version).
// Pages that show the same list to readers of the same projects render
// it once a tick (MP-11).
type listsCache struct {
	hub *live.Hub
	mu  sync.Mutex
	m   map[string]*cachedList
}

type cachedList struct {
	seq  uint64
	done chan struct{}
	html string
	err  error
}

const maxCachedLists = 512

var listCacheTopics = append(slices.Clone(listTopics), people.Topic)

// get returns the cached HTML of key, or what read makes of it: read once
// for every render of the tick that asks.
func (c *listsCache) get(ctx context.Context, seq uint64, key string, read func() (string, error)) (string, error) {
	if seq == 0 {
		return read()
	}
	c.mu.Lock()
	if c.m == nil {
		c.m = map[string]*cachedList{}
	}
	if e, ok := c.m[key]; ok && c.hub.Version(listCacheTopics...) <= e.seq {
		c.mu.Unlock()
		select {
		case <-e.done:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		if e.err == nil {
			return e.html, nil
		}
		return read()
	}
	if len(c.m) >= maxCachedLists {
		clear(c.m)
	}
	e := &cachedList{seq: seq, done: make(chan struct{})}
	c.m[key] = e
	c.mu.Unlock()
	e.html, e.err = read()
	close(e.done)
	if e.err != nil {
		c.mu.Lock()
		if c.m[key] == e {
			delete(c.m, key)
		}
		c.mu.Unlock()
	}
	return e.html, e.err
}

// pageListsView is pagelists:<page>, the editor page's lists element: the
// rows of every issue list of the page's head document and the saved
// filters, for the editor's node views (FR-104). Its key holds the
// projects the person may read.
func (s *Server) pageListsView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	p, _, err := readable(ctx, q, actor, arg)
	if err != nil {
		return live.View{}, err
	}
	projects, err := tracker.Readable(ctx, q, actor)
	if err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    "pagelists:" + p.ID + ":" + rightsKey(projects),
		Topics: []string{wiki.DocTopic(p.ID), tracker.FiltersTopic, people.Topic},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			q := querier(ctx, tx)
			var props views.PageListsProps
			var err error
			if props.Filters, err = tracker.SavedFilters(ctx, q, ""); err != nil {
				return nil, err
			}
			h, err := wiki.HeadOf(ctx, q, p.ID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			var lists []docnodes.IssueList
			if err == nil {
				lists = docnodes.IssueLists(h.Doc)
			}
			if len(lists) > 0 {
				live.Depends(ctx, listTopics...)
			}
			lf := listsFor{readable: projects, owner: p.CreatedBy, seq: live.Seq(ctx)}
			for _, l := range lists {
				if !slices.ContainsFunc(props.Lists, func(r views.ListRows) bool { return r.Key == l.Key() }) {
					props.Lists = append(props.Lists, views.ListRows{Key: l.Key(), HTML: s.listHTML(ctx, q, lf, l)})
				}
			}
			return patch(ctx, views.PageListsRegion(props))
		},
	}, nil
}

// lastReported is the project of the issue a person created last
// (issues_reporter, migration 088).
const lastReported = `SELECT project_id FROM issues WHERE reporter = ? AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1`

// pageIssueProps is "Create issue" on a page for a person who may edit it
// (FR-105): the projects they may create issues in, the space's own
// project preselected (the project whose key is the space's), else the
// project they last created an issue in, else the first.
func pageIssueProps(ctx context.Context, q sqlite.Querier, me people.Person, p wiki.Page, sp wiki.Space) (views.PageIssueProps, error) {
	props := views.PageIssueProps{Page: p.ID}
	projects, err := tracker.Readable(ctx, q, me)
	if err != nil {
		return props, err
	}
	all, err := tracker.Projects(ctx, q, projects)
	if err != nil {
		return props, err
	}
	for _, pr := range all {
		if pr.Archived {
			continue
		}
		if ok, err := access.Can(ctx, q, me, access.EditIssues, access.ProjectScope(pr.ID)); err != nil {
			return props, err
		} else if ok {
			props.Projects = append(props.Projects, pr)
		}
	}
	if len(props.Projects) == 0 {
		return props, nil
	}
	has := func(id string) bool {
		return slices.ContainsFunc(props.Projects, func(pr tracker.Project) bool { return pr.ID == id })
	}
	for _, pr := range props.Projects {
		if pr.Key == sp.Key {
			props.Project = pr.ID
			return props, nil
		}
	}
	var last string
	err = q.QueryRowContext(ctx, lastReported, me.ID).Scan(&last)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return props, err
	}
	if has(last) {
		props.Project = last
	} else {
		props.Project = props.Projects[0].ID
	}
	return props, nil
}

type pageIssueRequest struct {
	Payload
	Page    string `json:"page"`
	Project string `json:"project"`
	Summary string `json:"summary"`
	Find    string `json:"find"`
	Block   int    `json:"block"`
	Nth     int    `json:"nth"`
}

// pageIssue is POST /cmd/pages/issue (FR-105): a task from a page's
// selected text, linked to the page, with its chip in place of the text
// in view mode. A creation like any other for the limits (DM-04).
func (s *Server) pageIssue(w http.ResponseWriter, r *http.Request) {
	var req pageIssueRequest
	a, ok := s.decode(w, r, classCreate, &req)
	if !ok {
		return
	}
	if !wiki.ValidID(req.Page) || utf8.RuneCountInString(req.Find) > 2000 || req.Block < -1 || req.Block > 10000 || req.Nth < 0 || req.Nth > 10000 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	cmd := pageIssue{By: a.By, Page: req.Page, Project: req.Project, Summary: selectedSummary(req.Summary), Find: req.Find, Block: req.Block, Nth: req.Nth}
	res, ok := s.run(w, r, a, cmd)
	switch {
	case !ok:
	case res.Denied:
		http.Error(w, "forbidden", http.StatusForbidden)
	case !res.OK:
		w.WriteHeader(http.StatusNoContent)
	default:
		var made struct {
			Key  string `json:"key"`
			Chip bool   `json:"chip"`
		}
		json.Unmarshal(res.Value, &made)
		s.signals(w, map[string]any{"_pissue": map[string]any{"key": made.Key, "chip": made.Chip}})
	}
}

// selectedSummary is a selection's text as a summary: white space folded,
// at most the summary's 255 characters.
func selectedSummary(text string) string {
	t := strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(t) > tracker.MaxSummary {
		t = strings.TrimSpace(string([]rune(t)[:tracker.MaxSummary]))
	}
	return t
}

// pageIssue creates a task from a page's text and links the page to it
// (FR-105), and with Find puts its chip in place of the text. The actor
// must be a Member of the page's space and may create issues in the
// project. Its result is {"id", "key", "chip"}.
type pageIssue struct {
	By      tabs.By
	Page    string
	Project string
	Summary string
	Find    string
	Block   int
	Nth     int
}

func (c pageIssue) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

func (c pageIssue) ApplyResult(tx *writer.Tx) ([]byte, error) {
	p, sp, err := readable(tx.Context(), tx, c.By.Actor, c.Page)
	if err != nil {
		return nil, err
	}
	if err := access.Require(tx, c.By.Actor, access.EditIssues, access.SpaceScope(sp.ID)); err != nil {
		return nil, err
	}
	switch {
	case p.State != wiki.Current:
		return nil, tabs.Refuse(tracker.FieldSummary, "This page is in the trash.")
	case !tracker.ValidID(c.Project):
		return nil, tabs.Refuse("project", "Pick a project.")
	case c.Summary == "":
		return nil, tabs.Refuse(tracker.FieldSummary, "Select some text for the issue's summary.")
	}
	b, err := tracker.CreateIssue{By: c.By, Project: c.Project, Type: tracker.Task, Summary: c.Summary}.ApplyResult(tx)
	if err != nil {
		return nil, err
	}
	var made struct{ ID, Key string }
	if err := json.Unmarshal(b, &made); err != nil {
		return nil, err
	}
	if err := (tracker.LinkPage{By: c.By, Issue: made.ID, Page: p.ID, Title: p.Title}).Apply(tx); err != nil {
		return nil, err
	}
	chip := false
	if c.Find != "" {
		var refusal *writer.Refusal
		chip, err = wiki.ChipSelection(tx, c.By, p.ID, c.Block, c.Nth, c.Find, made.Key)
		if errors.As(err, &refusal) && !errors.As(err, new(*access.Denial)) {
			chip, err = false, nil // the issue stands without its chip
		}
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(map[string]any{"id": made.ID, "key": made.Key, "chip": chip})
}

// An issue made from a page's text counts as a new issue (DM-04.3).
func init() {
	demo.CountCreations(func(cmd writer.Command) string {
		if _, ok := cmd.(pageIssue); ok {
			return demo.CapIssue
		}
		return ""
	})
}
