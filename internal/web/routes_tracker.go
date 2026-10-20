package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/sse"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/demo"
	"github.com/zweiundeins/tickit/internal/doc"
	"github.com/zweiundeins/tickit/internal/markup"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/ui"
	"github.com/zweiundeins/tickit/internal/views"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// routesTracker registers projects, workflows, issues, their comments, links and history (FR-01 to FR-38, FR-65 to FR-70).
// Register pages with s.get (with a probe URL for a pattern with wildcards:
// the test that GET never writes requests it), actions with s.post and
// s.decode, s.act or s.actResult, and views with s.views.Register.
func (s *Server) routesTracker() {
	// The probes name what TestGetNeverWrites's data holds (seedProbes):
	// WEB-1, WEB-2 deleted, WEB-3 moved to APP-1, and keys that name
	// nothing.
	s.get("GET /{key}", http.HandlerFunc(s.issuePage), "/WEB-1", "/web-1", "/WEB-2", "/WEB-3", "/APP-1", "/WEB-999", "/not-a-key")
	s.get("GET /ticket/{id}", http.HandlerFunc(s.oldIssueURL), "/ticket/"+ProbeIssue, "/ticket/nope")
	s.get("GET /dashboard/{id}", http.HandlerFunc(s.oldIssueURL), "/dashboard/"+ProbeIssue)
	s.views.RegisterWithPresence("issue", s.issueView)
	s.views.Register("issue-at", s.issueAtView)
	s.views.Register("issue-typing", s.issueTypingView)
	s.post("POST /cmd/issues/create", s.createIssue)
	s.post("POST /cmd/issues/{id}/{field}", s.issueAction)
}

// ProbeIssue is the id of WEB-1 in the probes' data (seedProbes).
const ProbeIssue = "01920000-0000-7000-8000-0000000000a1"

// issueAgain is how often an issue's view renders without a change, so
// that its relative times move on ("5 minutes ago", NFR-51).
const issueAgain = 30 * time.Second

// issuePage is /WEB-12 (FR-12): the issue's page, matched without regard
// to case; another spelling or an old key redirects to the key now; an
// unknown key answers 404 with a search box, a deleted issue's key 410.
func (s *Server) issuePage(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if _, _, ok := tracker.ParseIssueKey(key); !ok {
		s.notFound(w, r)
		return
	}
	ctx := r.Context()
	actor := people.Actor(ctx)
	type found struct {
		k       tracker.KeyLookup
		by      string
		visible bool
	}
	f, err := s.db.ViewValue(ctx, func(tx *sql.Tx) (found, error) {
		k, err := tracker.IssueByKey(ctx, tx, key)
		if err != nil {
			return found{}, err
		}
		ok, err := access.Can(ctx, tx, actor, access.Read, access.ProjectScope(k.Project))
		if err != nil || !ok {
			return found{}, err
		}
		f := found{k: k, visible: true}
		if k.Deleted {
			p, err := people.Get(ctx, tx, k.DeletedBy)
			if err != nil {
				return f, err
			}
			f.by = p.Name
		}
		return f, nil
	})
	switch {
	case errors.Is(err, access.ErrNotFound) || err == nil && !f.visible:
		s.issueMissing(w, r, strings.ToUpper(key))
		return
	case err != nil:
		s.fail(w, r, err)
		return
	case f.k.Deleted && !(f.k.Hidden && access.SiteCan(actor, access.Moderate)):
		s.render(w, r, http.StatusGone, func(context.Context, sqlite.Querier) (Page, error) {
			return Page{Title: f.k.Key + " was deleted", Nav: "projects", Project: f.k.Project,
				Body: views.IssueGone(f.k.Key, f.by, f.k.DeletedAt.In(views.Zone).Format("2 January"))}, nil
		})
		return
	case f.k.Key != key:
		http.Redirect(w, r, "/"+f.k.Key, http.StatusMovedPermanently)
		return
	}
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		m, err := loadIssue(ctx, q, f.k.Issue, time.Now())
		if err != nil {
			return Page{}, err
		}
		edit, err := access.Can(ctx, q, actor, access.EditIssues, access.ProjectScope(m.ProjectID))
		if err != nil {
			return Page{}, err
		}
		hide, err := access.Can(ctx, q, actor, access.HideComment, access.ProjectScope(m.ProjectID))
		if err != nil {
			return Page{}, err
		}
		return Page{
			Title:   m.Key + " " + m.Summary,
			Nav:     "projects",
			Project: m.ProjectID,
			Views:   []string{"issue:" + m.ID, "issue-typing:" + m.ID},
			Body: views.IssuePage(views.IssueFrameProps{
				Issue: &m, ID: m.ID, Key: m.Key, Me: actor.ID,
				ReadOnly: !edit || m.Project.Archived || !m.Deleted.IsZero(), Moderates: hide,
				Deletes: deletes(ctx, q, actor, []string{m.ProjectID}),
			}),
		}, nil
	})
}

// issueMissing is the 404 of an issue key that names nothing the person
// may see, with a search box (FR-12.2).
func (s *Server) issueMissing(w http.ResponseWriter, r *http.Request, key string) {
	s.render(w, r, http.StatusNotFound, func(context.Context, sqlite.Querier) (Page, error) {
		return Page{Title: "No issue " + key, Body: views.IssueNotFound(key)}, nil
	})
}

// oldIssueURL redirects tickit's old /ticket/<id> and /dashboard/<id> to
// the issue's one URL (FR-12.3).
func (s *Server) oldIssueURL(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !tracker.ValidID(id) {
		s.notFound(w, r)
		return
	}
	d, err := s.db.ViewValue(r.Context(), func(tx *sql.Tx) (tracker.Detail, error) {
		if _, err := readableIssue(r.Context(), tx, people.Actor(r.Context()), id); err != nil {
			return tracker.Detail{}, err // a key the person may not read stays unknown (NFR-01.2)
		}
		return tracker.IssueDetail(r.Context(), tx, id)
	})
	switch {
	case errors.Is(err, access.ErrNotFound):
		s.notFound(w, r)
	case err != nil:
		s.fail(w, r, err)
	default:
		http.Redirect(w, r, "/"+d.Key, http.StatusMovedPermanently)
	}
}

// deletes is where actor may delete issues among projects, nil for those
// they may read (views.Deletes): the document shows "Delete issue" there
// only (FR-28.1).
func deletes(ctx context.Context, q sqlite.Querier, actor people.Person, projects []string) views.Deletes {
	if access.SiteCan(actor, access.Moderate) {
		return views.Deletes{All: true}
	}
	if projects == nil {
		var err error
		if projects, err = tracker.Readable(ctx, q, actor); err != nil {
			return views.Deletes{}
		}
	}
	var d views.Deletes
	for _, id := range projects {
		if can(ctx, q, actor, access.DeleteIssue, id) {
			d.Projects = append(d.Projects, id)
		}
	}
	return d
}

// readableIssue checks that actor may read the issue id, which exists,
// and returns its project.
func readableIssue(ctx context.Context, q sqlite.Querier, actor people.Person, id string) (string, error) {
	if !tracker.ValidID(id) {
		return "", access.ErrNotFound
	}
	project, err := issueProject(ctx, q, id)
	if err != nil {
		return "", err
	}
	ok, err := access.Can(ctx, q, actor, access.Read, access.ProjectScope(project))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", access.ErrNotFound
	}
	return project, nil
}

func issueProject(ctx context.Context, q sqlite.Querier, id string) (string, error) {
	d, err := tracker.IssueDetail(ctx, q, id)
	if err != nil {
		return "", err
	}
	return d.ProjectID, nil
}

// issueView is the live view issue:<id>: every region of the issue's page
// and panel, for everyone who has it open (FR-13, MP-03).
func (s *Server) issueView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	project, err := readableIssue(ctx, q, actor, arg)
	if err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    "issue:" + arg,
		Topics: []string{tracker.IssueTopic(arg), people.Topic, tracker.ProjectTopic(project)},
		Every:  issueAgain,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			m, err := loadIssue(ctx, querier(ctx, tx), arg, time.Now())
			if err != nil {
				return nil, err
			}
			frame, err := patch(ctx, views.IssueRegions(m))
			if err != nil {
				return nil, err
			}
			return append(frame, sse.PatchSignals([]byte(`{"_is":{"max":`+strconv.FormatInt(m.Version, 10)+`}}`), false)...), nil
		},
	}, nil
}

// issueAtView is issue-at:<id>.<version>, the issue as it stood then
// (FR-70), keyed issue:<id>:at:<version>: tabs at one version share it.
func (s *Server) issueAtView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	id, v, ok := strings.Cut(arg, ".")
	version, err := strconv.ParseInt(v, 10, 64)
	if !ok || err != nil || version < 1 {
		return live.View{}, fmt.Errorf("%w: issue-at:%s", errBadView, arg)
	}
	if _, err := readableIssue(ctx, q, actor, id); err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    "issue:" + id + ":at:" + v,
		Topics: []string{people.Topic},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			a, err := loadIssueAt(ctx, querier(ctx, tx), id, version, time.Now())
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.IssueAsOfRegion(a))
		},
	}, nil
}

// issueTypingView is issue-typing:<id>: who types in the issue's fields
// now (MP-07). It renders every second, so an indicator goes 5 s after
// the last keystroke; equal frames are not sent.
func (s *Server) issueTypingView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	if _, err := readableIssue(ctx, q, actor, arg); err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    "issue-typing:" + arg,
		Topics: []string{tracker.TypingTopic(arg), people.Topic},
		Every:  time.Second,
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			q := querier(ctx, tx)
			list, err := tracker.Typing(ctx, q, arg, time.Now())
			if err != nil {
				return nil, err
			}
			ids := make([]string, len(list))
			for i, t := range list {
				ids[i] = t.Person
			}
			found, err := people.Lookup(ctx, q, ids...)
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.IssueTypingRegions(views.Typing{Typists: list, People: found}))
		},
	}, nil
}

// loadIssue reads everything the issue's view shows.
func loadIssue(ctx context.Context, q sqlite.Querier, id string, now time.Time) (views.Issue, error) {
	d, err := tracker.IssueDetail(ctx, q, id)
	if err != nil {
		return views.Issue{}, err
	}
	m := views.Issue{Detail: d, Now: now}
	if m.Pages, err = wiki.PagesMentioningIssue(ctx, q, id); err != nil { // FR-101
		return m, err
	}
	project := d.ProjectID
	if m.Workflow, err = tracker.ProjectWorkflow(ctx, q, project); err != nil {
		return m, err
	}
	if m.Statuses, err = tracker.Statuses(ctx, q, project); err != nil {
		return m, err
	}
	if m.Sprints, err = tracker.Sprints(ctx, q, project); err != nil {
		return m, err
	}
	if m.Components, err = tracker.Components(ctx, q, project); err != nil {
		return m, err
	}
	if m.Assignable, err = tracker.Assignable(ctx, q, project, "", now); err != nil {
		return m, err
	}
	if m.Labels, err = tracker.Labels(ctx, q, project, ""); err != nil {
		return m, err
	}
	if m.Epics, err = tracker.Epics(ctx, q, project); err != nil {
		return m, err
	}
	if m.Others, err = tracker.Recent(ctx, q, project, tracker.PageSize); err != nil {
		return m, err
	}
	comments, err := tracker.Comments(ctx, q, id)
	if err != nil {
		return m, err
	}
	if m.History, err = tracker.History(ctx, q, id); err != nil {
		return m, err
	}
	if m.Activity, err = tracker.Activity(ctx, q, id); err != nil {
		return m, err
	}
	ids := append([]string{d.Reporter, d.Assignee, d.DeletedBy}, d.Watchers...)
	for _, c := range comments {
		ids = append(ids, c.Author)
	}
	for _, e := range m.History {
		ids = append(ids, e.Actor)
	}
	if m.People, err = people.Lookup(ctx, q, ids...); err != nil {
		return m, err
	}
	res := &docResolver{ctx: ctx, q: q, people: m.People}
	if m.DescHTML, m.DescMarkup, err = renderDoc(d.Description, res); err != nil {
		return m, err
	}
	for _, c := range comments {
		vc := views.Comment{Comment: c}
		if vc.HTML, vc.Markup, err = renderDoc(c.Body, res); err != nil {
			return m, err
		}
		m.Comments = append(m.Comments, vc)
	}
	// The texts to edit show the names of the people they mention, which
	// the page turns back into ids when it sends them (FR-67.2).
	var named []string
	add := func(ids []string) {
		for _, id := range ids {
			if !slices.Contains(named, id) {
				named = append(named, id)
				m.Mentioned = append(m.Mentioned, m.People[id])
			}
		}
	}
	var ids2 []string
	m.DescMarkup, ids2 = views.NamedMentions(m.DescMarkup, m.People)
	add(ids2)
	for i := range m.Comments {
		m.Comments[i].Markup, ids2 = views.NamedMentions(m.Comments[i].Markup, m.People)
		add(ids2)
	}
	return m, nil
}

// loadIssueAt reads an issue as it stood at a version (FR-70).
func loadIssueAt(ctx context.Context, q sqlite.Querier, id string, version int64, now time.Time) (views.IssueAsOf, error) {
	st, v, err := tracker.StateAt(ctx, q, id, version)
	if err != nil {
		return views.IssueAsOf{}, err
	}
	a := views.IssueAsOf{ID: id, Key: st.Key, State: st, Version: v, Now: now, Keys: map[string]string{}}
	history, err := tracker.History(ctx, q, id)
	if err != nil {
		return a, err
	}
	ids := []string{st.Reporter, st.Assignee}
	for _, e := range history {
		a.Latest = max(a.Latest, e.Version)
		if e.Version == v {
			a.Changes = append(a.Changes, e)
			ids = append(ids, e.Actor)
		}
	}
	if a.Workflow, err = tracker.ProjectWorkflow(ctx, q, st.Project); err != nil {
		return a, err
	}
	if st.Parent != "" {
		if d, err := tracker.IssueDetail(ctx, q, st.Parent); err == nil {
			a.Keys[st.Parent] = d.Key
		}
	}
	if a.People, err = people.Lookup(ctx, q, ids...); err != nil {
		return a, err
	}
	a.DescHTML, _, err = renderDoc(st.Description, &docResolver{ctx: ctx, q: q, people: a.People})
	return a, err
}

// renderDoc renders a stored document as HTML and as markup; nil gives "".
func renderDoc(raw json.RawMessage, r *docResolver) (string, string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", "", nil
	}
	d, err := doc.ParseDoc(raw)
	if err != nil {
		return "", "", nil // a document the schema refuses shows as nothing, not as an error page
	}
	text, err := markup.Write(raw, nil)
	if err != nil {
		text = ""
	}
	html := doc.RenderHTML(d, r)
	if strings.TrimSpace(doc.Text(d)) == "" && !strings.Contains(html, "<img") {
		return "", text, nil
	}
	return html, text, nil
}

// docResolver answers what documents name, in the read they render in: an
// issue chip's issue as it is now, a mention's person by name, a page by
// its title.
type docResolver struct {
	ctx    context.Context
	q      sqlite.Querier
	people map[string]people.Person
}

func (r *docResolver) Issue(key string) (doc.Chip, bool) {
	ref, err := tracker.RefByKey(r.ctx, r.q, key)
	if err != nil || ref == nil {
		return doc.Chip{}, false
	}
	cat := string(ref.Category)
	if ref.Category == tracker.InProgress {
		cat = "progress"
	}
	return doc.Chip{Summary: ref.Summary, Status: ref.StatusName, Category: cat, Type: string(ref.Type), Deleted: ref.Deleted}, true
}

func (r *docResolver) Person(id string) (string, bool) {
	if p, ok := r.people[id]; ok {
		return p.Name, p.Kind != people.Deleted
	}
	p, err := people.Get(r.ctx, r.q, id)
	if err != nil || p.Kind == people.Deleted {
		return "", false
	}
	r.people[id] = p
	return p.Name, true
}

// Face is a mentioned person's face (FR-67.2), for one Person found.
func (r *docResolver) Face(id string) string {
	p, ok := r.people[id]
	if !ok || p.Kind == people.Deleted {
		return ""
	}
	return ui.FaceHTML(views.UIPerson(p))
}

// Page is a linked page's title while it is current; one in the trash or
// deleted shows as removed.
func (r *docResolver) Page(id string) (string, bool) {
	var title string
	if err := r.q.QueryRowContext(r.ctx, `SELECT title FROM pages WHERE id = ? AND state = 'current'`, id).Scan(&title); err != nil {
		return "", false
	}
	return title, true
}

// textResolver answers what a person's markup names while it is parsed:
// issues by key and people by id, unknown ones staying text.
type textResolver struct {
	ctx context.Context
	q   sqlite.Querier
}

func (r textResolver) Issue(key string) bool {
	ref, err := tracker.RefByKey(r.ctx, r.q, key)
	return err == nil && ref != nil
}

func (r textResolver) Person(id string) (string, bool) {
	if !people.ValidID(id) {
		return "", false
	}
	p, err := people.Get(r.ctx, r.q, id)
	if err != nil || p.Kind == people.Deleted {
		return "", false
	}
	return p.Name, true
}

func (textResolver) Page(string) (string, bool) { return "", false }
func (textResolver) Image(string) bool          { return false }

// parseText turns what a person typed into a document; empty text is the
// empty document, nil. A text over the limits is refused next to field.
func (s *Server) parseText(ctx context.Context, text string, mode markup.Mode) (json.RawMessage, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	return s.db.ViewValue(ctx, func(tx *sql.Tx) (json.RawMessage, error) {
		return markup.ParseMode(text, textResolver{ctx, tx}, mode)
	})
}

// issueRequest is the payload of every action on an issue's field: the
// version the page showed (MP-08.1), the value, and what it refers to (a
// comment's id, a link's type, add or remove).
type issueRequest struct {
	Payload
	Seen  int64  `json:"seen"`
	Value string `json:"value"`
	Ref   string `json:"ref"`
}

// typingRequest is the payload of POST /cmd/issues/{id}/typing.
type typingRequest struct {
	Payload
	Field string `json:"field"`
	Stop  bool   `json:"stop"`
}

// issueAction is POST /cmd/issues/{id}/{field}: one change to an issue
// from its page or panel, through tabs.Act; the outcome travels on the
// tab's stream (MP-09). Unknown fields and malformed ids answer 400.
func (s *Server) issueAction(w http.ResponseWriter, r *http.Request) {
	id, field := r.PathValue("id"), r.PathValue("field")
	if field == "typing" {
		s.issueTyping(w, r, id)
		return
	}
	c := classField
	switch field {
	case "comment", "cedit":
		c = classComment
	case "subtask":
		c = classCreate
	}
	var req issueRequest
	a, ok := s.decode(w, r, c, &req)
	if !ok {
		return
	}
	if !tracker.ValidID(id) {
		http.Error(w, "bad issue id", http.StatusBadRequest)
		return
	}
	cmd, err := s.issueCommand(r.Context(), a, id, field, req)
	if errors.Is(err, errField) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		s.failAction(w, r, err)
		return
	}
	s.act(w, r, a, cmd)
}

// errField is an action on a field the page has not.
var errField = errors.New("no such field")

// issueCommand is the command of an action on an issue's field.
func (s *Server) issueCommand(ctx context.Context, a action, id, field string, req issueRequest) (writer.Command, error) {
	by, v := a.By, req.Value
	stop := func(f string) tracker.SetTyping {
		return tracker.SetTyping{By: by, Issue: id, Field: f, Stop: true}
	}
	switch field {
	case "summary":
		return also{tracker.SetSummary{By: by, Issue: id, Seen: req.Seen, Summary: v}, stop(tracker.TypingSummary)}, nil
	case "description":
		body, err := s.parseText(ctx, v, markup.Text)
		if err != nil {
			return refuse(tracker.FieldDescription, textProblem(err)), nil
		}
		return also{tracker.SetDescription{By: by, Issue: id, Seen: req.Seen, Description: body}, stop(tracker.TypingDescription)}, nil
	case "type":
		return tracker.SetType{By: by, Issue: id, Seen: req.Seen, Type: tracker.IssueType(v)}, nil
	case "status":
		return tracker.SetStatus{By: by, Issue: id, Seen: req.Seen, Status: v}, nil
	case "priority":
		return tracker.SetPriority{By: by, Issue: id, Seen: req.Seen, Priority: tracker.Priority(v)}, nil
	case "assignee":
		if v == "me" {
			v = by.Actor.ID
		}
		if v != "" && !people.ValidID(v) {
			return nil, fmt.Errorf("%w: assignee", errField)
		}
		return tracker.Assign{By: by, Issue: id, Seen: req.Seen, Assignee: v}, nil
	case "parent":
		if v != "" && !tracker.ValidID(v) {
			return nil, fmt.Errorf("%w: parent", errField)
		}
		return tracker.SetParent{By: by, Issue: id, Seen: req.Seen, Parent: v}, nil
	case "due":
		return tracker.SetDue{By: by, Issue: id, Seen: req.Seen, Due: strings.TrimSpace(v)}, nil
	case "points":
		n, ok := optInt(v)
		if !ok {
			return refuse(tracker.FieldPoints, "Story points are a number from 0 to 100."), nil
		}
		return tracker.SetPoints{By: by, Issue: id, Seen: req.Seen, Points: n}, nil
	case "estimate", "remaining":
		m, ok := ParseMinutes(v)
		if !ok {
			return refuse(field, "Write an estimate as 2h 30m, 45m or 3h."), nil
		}
		if field == "estimate" {
			return tracker.SetEstimate{By: by, Issue: id, Seen: req.Seen, Estimate: m}, nil
		}
		return tracker.SetRemaining{By: by, Issue: id, Seen: req.Seen, Remaining: m}, nil
	case "sprint":
		if v != "" && !tracker.ValidID(v) {
			return nil, fmt.Errorf("%w: sprint", errField)
		}
		return tracker.SetSprint{By: by, Issue: id, Seen: req.Seen, Sprint: v}, nil
	case "labels", "components":
		var add, remove []string
		switch req.Ref {
		case "add":
			add = []string{v}
		case "remove":
			remove = []string{v}
		default:
			return nil, fmt.Errorf("%w: add or remove", errField)
		}
		if field == "labels" {
			return tracker.SetLabels{By: by, Issue: id, Add: add, Remove: remove}, nil
		}
		return tracker.SetComponents{By: by, Issue: id, Add: add, Remove: remove}, nil
	case "watch":
		return toggleWatch{By: by, Issue: id}, nil
	case "link":
		t := tracker.LinkType(req.Ref)
		if !t.Valid() {
			return nil, fmt.Errorf("%w: link type", errField)
		}
		return linkByKey{By: by, Issue: id, Type: t, Key: v}, nil
	case "unlink":
		t := tracker.LinkType(req.Ref)
		if !t.Valid() || !tracker.ValidID(v) {
			return nil, fmt.Errorf("%w: link", errField)
		}
		return tracker.UnlinkIssues{By: by, Issue: id, Type: t, Other: v}, nil
	case "subtask":
		return subtask{By: by, Parent: id, Summary: v}, nil
	case "comment":
		body, err := s.parseText(ctx, v, markup.Comment)
		if err != nil {
			return refuse("comment", textProblem(err)), nil
		}
		if body == nil {
			return refuse("comment", "Write a comment first."), nil
		}
		return also{tracker.AddComment{By: by, Issue: id, Body: body}, stop(tracker.TypingComment)}, nil
	case "cedit", "cdelete", "chide":
		if !tracker.ValidID(req.Ref) {
			return nil, fmt.Errorf("%w: comment id", errField)
		}
		switch field {
		case "cdelete":
			return tracker.DeleteComment{By: by, Comment: req.Ref}, nil
		case "chide":
			return tracker.HideComment{By: by, Comment: req.Ref, Hidden: v != "show"}, nil
		}
		body, err := s.parseText(ctx, v, markup.Comment)
		if err != nil {
			return refuse("comment", textProblem(err)), nil
		}
		if body == nil {
			return refuse("comment", "A comment cannot be empty. Delete it instead."), nil
		}
		return tracker.EditComment{By: by, Comment: req.Ref, Body: body}, nil
	case "delete":
		return tracker.DeleteIssue{By: by, Issue: id}, nil
	}
	return nil, fmt.Errorf("%w %q", errField, field)
}

// textProblem says why a text was refused, for a person.
func textProblem(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, ": "); strings.HasPrefix(msg, "line ") && i > 0 {
		msg = msg[i+2:]
	}
	if msg == "" {
		return "This text cannot be saved."
	}
	return "This text cannot be saved: " + msg + "."
}

// issueTyping is POST /cmd/issues/{id}/typing (MP-07): the actor types
// in a field now, or closed it. Rate-limited as a caret; sent without
// waiting, since the next keystroke replaces it, and with no outcome.
func (s *Server) issueTyping(w http.ResponseWriter, r *http.Request, id string) {
	var req typingRequest
	a, ok := s.decode(w, r, classCaret, &req)
	if !ok {
		return
	}
	cmd := tracker.SetTyping{By: a.By, Issue: id, Field: req.Field, Stop: req.Stop}
	if err := cmd.Validate(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if s.paused(w, r, a.By.Actor) {
		return
	}
	// The read right before it is sent: the command checks it again.
	if _, err := s.db.ViewValue(r.Context(), func(tx *sql.Tx) (string, error) { return readableIssue(r.Context(), tx, a.By.Actor, id) }); err != nil {
		s.failAction(w, r, err)
		return
	}
	if err := s.w.Send(cmd); err != nil {
		s.failAction(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// createRequest is the create dialog's payload (FR-10).
type createRequest struct {
	Payload
	Project     string `json:"project"`
	Type        string `json:"type"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Assignee    string `json:"assignee"`
	Labels      string `json:"labels"`
	Parent      string `json:"parent"`
	Status      string `json:"status"`
	Stay        bool   `json:"stay"`
}

var labelSep = regexp.MustCompile(`[\s,]+`)

// createIssue is POST /cmd/issues/create: a refusal stays next to its
// field with what the person typed (FR-10.4); a new issue sends the page
// to it ($_goto), or, on a page with the issue panel, opens it there.
func (s *Server) createIssue(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	a, ok := s.decode(w, r, classCreate, &req)
	if !ok {
		return
	}
	cmd, err := s.createCommand(r.Context(), a, req)
	if err != nil {
		s.failAction(w, r, err)
		return
	}
	res, ok := s.run(w, r, a, cmd)
	switch {
	case !ok:
	case res.Denied:
		http.Error(w, "forbidden", http.StatusForbidden)
	case !res.OK:
		w.WriteHeader(http.StatusNoContent)
	default:
		var created struct{ ID, Key string }
		json.Unmarshal(res.Value, &created)
		if req.Stay {
			s.signals(w, map[string]any{"_new": map[string]any{"created": created.ID + " " + created.Key}})
			return
		}
		s.signals(w, map[string]any{"_goto": "/" + created.Key})
	}
}

// createCommand reads what the dialog names (a parent by key, the
// description's references) and returns the command, or the refusal of
// a field.
func (s *Server) createCommand(ctx context.Context, a action, req createRequest) (resultCommand, error) {
	if !tracker.ValidID(req.Project) {
		return refuse("project", "Pick a project."), nil
	}
	t := tracker.IssueType(req.Type)
	if !t.Valid() {
		return refuse(tracker.FieldType, "Pick one of Epic, Story, Task, Bug and Subtask."), nil
	}
	if req.Priority == "" {
		req.Priority = string(tracker.Medium)
	}
	assignee := req.Assignee
	if assignee == "me" {
		assignee = a.By.Actor.ID
	}
	if assignee != "" && !people.ValidID(assignee) {
		return refuse(tracker.FieldAssignee, "Pick a person from the list."), nil
	}
	if req.Status != "" && !validStatusID(req.Status) {
		return refuse(tracker.FieldStatus, "This project's workflow has no such status."), nil
	}
	cmd := tracker.CreateIssue{By: a.By, Project: req.Project, Type: t, Summary: req.Summary,
		Priority: tracker.Priority(req.Priority), Assignee: assignee, Status: req.Status}
	if parent := strings.TrimSpace(req.Parent); parent != "" {
		ref, err := s.db.ViewValue(ctx, func(tx *sql.Tx) (*tracker.IssueRef, error) { return readableRef(ctx, tx, a.By.Actor, parent) })
		if err != nil {
			return nil, err
		}
		if ref == nil {
			return refuse(tracker.FieldParent, "No issue has the key "+strings.ToUpper(parent)+"."), nil
		}
		cmd.Parent = ref.ID
	}
	for _, l := range labelSep.Split(req.Labels, -1) {
		if l != "" {
			cmd.Labels = append(cmd.Labels, l)
		}
	}
	body, err := s.parseText(ctx, req.Description, markup.Text)
	if err != nil {
		return refuse(tracker.FieldDescription, textProblem(err)), nil
	}
	cmd.Description = body
	return cmd, nil
}

var statusID = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

func validStatusID(s string) bool { return statusID.MatchString(s) }

// createDialog is the header's create dialog for a person (FR-10.1), on
// every page, with project preselected: the page's (Page.Project), else
// their first.
func (s *Server) createDialog(ctx context.Context, q sqlite.Querier, actor people.Person, project string) (templ.Component, error) {
	readable, err := tracker.Readable(ctx, q, actor)
	if err != nil {
		return nil, err
	}
	all, err := tracker.Projects(ctx, q, readable)
	if err != nil {
		return nil, err
	}
	p := views.CreateIssueProps{}
	for _, pr := range all {
		if pr.Archived {
			continue
		}
		ok, err := access.Can(ctx, q, actor, access.EditIssues, access.ProjectScope(pr.ID))
		if err != nil {
			return nil, err
		}
		if ok {
			p.Projects = append(p.Projects, pr)
		}
	}
	for _, pr := range p.Projects {
		if pr.ID == project || p.Project == "" {
			p.Project = pr.ID
		}
	}
	if p.Project != "" {
		if p.People, err = tracker.Assignable(ctx, q, p.Project, "", time.Now()); err != nil {
			return nil, err
		}
	}
	return views.CreateIssueDialog(p), nil
}

// optInt reads an optional whole number: "" is none.
func optInt(s string) (*int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, true
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil, false
	}
	return &n, true
}

var estimateRe = regexp.MustCompile(`^(?:(\d+(?:[.,]\d+)?)\s*h)?\s*(?:(\d+)\s*m)?$`)

// ParseMinutes reads an estimate as a person writes it: "2h 30m", "45m",
// "3h", "1.5h", or a bare number of hours; "" is none (FR-26).
func ParseMinutes(s string) (*int, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil, true
	}
	if h, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64); err == nil {
		if h < 0 || h > 1e6 {
			return nil, false
		}
		m := int(h*60 + 0.5)
		return &m, true
	}
	g := estimateRe.FindStringSubmatch(s)
	if g == nil || g[1] == "" && g[2] == "" {
		return nil, false
	}
	m := 0
	if g[1] != "" {
		h, _ := strconv.ParseFloat(strings.ReplaceAll(g[1], ",", "."), 64)
		if h > 1e6 {
			return nil, false
		}
		m += int(h*60 + 0.5)
	}
	if g[2] != "" {
		n, err := strconv.Atoi(g[2])
		if err != nil || n > 1e6 {
			return nil, false
		}
		m += n
	}
	return &m, true
}

// refusal refuses an action next to a field before any command runs, as
// a command's refusal would: tabs.Act records it as the outcome.
type refusal struct{ field, msg string }

func refuse(field, msg string) refusal { return refusal{field, msg} }

func (r refusal) Apply(*writer.Tx) error                 { return tabs.Refuse(r.field, r.msg) }
func (r refusal) ApplyResult(*writer.Tx) ([]byte, error) { return nil, tabs.Refuse(r.field, r.msg) }

// also runs a command, then, if it went through, a second one: an edit
// ends its typing indicator at once (MP-07.4).
type also struct {
	cmd  writer.Command
	then writer.Command
}

func (c also) Validate() error {
	for _, x := range []writer.Command{c.cmd, c.then} {
		if v, ok := x.(interface{ Validate() error }); ok {
			if err := v.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c also) Apply(tx *writer.Tx) error {
	if err := c.cmd.Apply(tx); err != nil {
		return err
	}
	return c.then.Apply(tx)
}

// toggleWatch starts the actor watching an issue, or stops it (FR-27.1),
// from what the issue's watchers are when it runs.
type toggleWatch struct {
	By    tabs.By
	Issue string
}

func (c toggleWatch) Apply(tx *writer.Tx) error {
	list, err := tracker.Watchers(context.Background(), tx, c.Issue)
	if err != nil {
		return err
	}
	watching := false
	for _, p := range list {
		watching = watching || p == c.By.Actor.ID
	}
	return tracker.Watch{By: c.By, Issue: c.Issue, Watch: !watching}.Apply(tx)
}

// linkByKey links an issue to the one a key names (FR-22.1).
type linkByKey struct {
	By    tabs.By
	Issue string
	Type  tracker.LinkType
	Key   string
}

func (c linkByKey) Apply(tx *writer.Tx) error {
	if _, err := readableIssue(tx.Context(), tx, c.By.Actor, c.Issue); err != nil {
		return err // the issue first: a refusal of the key tells nothing of it
	}
	key, _, _ := strings.Cut(strings.TrimSpace(c.Key), " ")
	ref, err := readableRef(tx.Context(), tx, c.By.Actor, key)
	if err != nil {
		return err
	}
	if ref == nil {
		return tabs.Refuse(tracker.FieldLinks, "No issue has the key "+strings.ToUpper(key)+".")
	}
	return tracker.LinkIssues{By: c.By, Issue: c.Issue, Type: c.Type, Other: ref.ID}.Apply(tx)
}

// A subtask is a new issue for the demo's cap (DM-04.3).
func init() {
	demo.CountCreations(func(cmd writer.Command) string {
		if _, ok := cmd.(subtask); ok {
			return demo.CapIssue
		}
		return ""
	})
}

// readableRef is the issue a key names, nil for none or for one the
// actor may not read: a refusal tells no more than for an unknown key.
func readableRef(ctx context.Context, q sqlite.Querier, actor people.Person, key string) (*tracker.IssueRef, error) {
	ref, err := tracker.RefByKey(ctx, q, key)
	if err != nil || ref == nil {
		return ref, err
	}
	if _, err := readableIssue(ctx, q, actor, ref.ID); errors.Is(err, access.ErrNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return ref, nil
}

// subtask adds a subtask to an issue, with the next number of its
// project (FR-21.1).
type subtask struct {
	By      tabs.By
	Parent  string
	Summary string
}

func (c subtask) Apply(tx *writer.Tx) error {
	project, err := issueProject(context.Background(), tx, c.Parent)
	if err != nil {
		return err
	}
	return tracker.CreateIssue{By: c.By, Project: project, Type: tracker.Subtask, Summary: c.Summary, Parent: c.Parent}.Apply(tx)
}
