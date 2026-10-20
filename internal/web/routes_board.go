package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/sse"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/views"
)

// routesBoard registers projects (FR-01 to FR-05), the board (FR-40 to
// FR-43, MP-04), the backlog and sprints (FR-45, FR-46), and their views:
//
//	projects:            the list of projects the person can read
//	board:<KEY>[.<f>]    a board's lanes for a quick filter (views.BoardArg)
//	boardhead:<KEY>      a board's head and faces, with presence (MP-02)
//	backlog:<KEY>        a backlog, with presence
//	settings:<KEY>       a project's members, components and limits (Admins)
func (s *Server) routesBoard() {
	s.get("GET /projects", http.HandlerFunc(s.projectsPage))
	s.get("GET /projects/{key}", http.HandlerFunc(s.boardPage), "/projects/WEB", "/projects/nope")
	s.get("GET /projects/{key}/board", http.HandlerFunc(s.boardPage), "/projects/WEB/board", "/projects/WEB/board?assignee=me&type=bug&label=x&q=y", "/projects/NOPE/board")
	s.get("GET /projects/{key}/backlog", http.HandlerFunc(s.backlogPage), "/projects/WEB/backlog")
	s.get("GET /projects/{key}/activity", http.HandlerFunc(s.activityPage), "/projects/WEB/activity")
	s.get("GET /projects/{key}/settings", http.HandlerFunc(s.settingsPage), "/projects/WEB/settings")

	s.post("POST /cmd/projects/create", s.createProject)
	s.post("POST /cmd/projects/edit", s.editProject)
	s.post("POST /cmd/projects/archive", s.archiveProject)
	s.post("POST /cmd/projects/component", s.defineComponent)
	s.post("POST /cmd/projects/component/remove", s.removeComponent)
	s.post("POST /cmd/projects/wip", s.setWIP)
	s.post("POST /cmd/projects/member", s.setMember)
	s.post("POST /cmd/projects/member/remove", s.removeMember)
	s.post("POST /cmd/board/move", s.moveCard)
	s.post("POST /cmd/backlog/plan", s.planIssues)
	s.post("POST /cmd/sprints/create", s.createSprint)
	s.post("POST /cmd/sprints/start", s.startSprint)
	s.post("POST /cmd/sprints/complete", s.completeSprint)

	s.views.Register("projects", s.projectsView)
	s.views.Register("board", s.boardView)
	s.views.RegisterWithPresence("boardhead", s.boardHeadView)
	s.views.RegisterWithPresence("backlog", s.backlogView)
	s.views.Register("settings", s.settingsView)
}

// readableProject is the project of a key that actor may read, or
// access.ErrNotFound: an unreadable project does not tell it exists.
func readableProject(ctx context.Context, q sqlite.Querier, actor people.Person, key string) (tracker.Project, error) {
	p, err := tracker.ProjectByKey(ctx, q, key)
	if err != nil {
		return p, err
	}
	ok, err := access.Can(ctx, q, actor, access.Read, access.ProjectScope(p.ID))
	if err != nil {
		return p, err
	}
	if !ok {
		return p, fmt.Errorf("project %s: %w", key, access.ErrNotFound)
	}
	return p, nil
}

func can(ctx context.Context, q sqlite.Querier, actor people.Person, r access.Right, project string) bool {
	ok, err := access.Can(ctx, q, actor, r, access.ProjectScope(project))
	return err == nil && ok
}

// ── Projects ──────────────────────────────────────────────────────────

func (s *Server) projectsPage(w http.ResponseWriter, r *http.Request) {
	me := people.Actor(r.Context())
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		ids, err := tracker.Readable(ctx, q, me)
		if err != nil {
			return Page{}, err
		}
		list, err := projectList(ctx, q, ids)
		if err != nil {
			return Page{}, err
		}
		return Page{Title: "Projects", Nav: "projects", Views: []string{"projects:"},
			Body: views.Projects(views.ProjectsPage{List: list, CanCreate: access.SiteCan(me, access.CreateScope)})}, nil
	})
}

func projectList(ctx context.Context, q sqlite.Querier, ids []string) (views.ProjectList, error) {
	list, err := tracker.Projects(ctx, q, ids)
	if err != nil {
		return views.ProjectList{}, err
	}
	var leads []string
	for _, p := range list {
		if p.Lead != "" {
			leads = append(leads, p.Lead)
		}
	}
	who, err := people.Lookup(ctx, q, leads...)
	return views.ProjectList{Projects: list, People: who}, err
}

// projectsView is the list of projects. Everyone reads the open ones,
// which the render reads anew, so a new open project shows for all; the
// closed ones a person may read are in the view's key, so viewers who
// read the same ones share it. Moderators and site admins read all.
func (s *Server) projectsView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	if arg != "" {
		return live.View{}, fmt.Errorf("%w: projects takes no argument", errBadView)
	}
	all := actor.Role == people.Moderator || actor.Role == people.Admin
	var closed []string
	if !all {
		ids, err := tracker.Readable(ctx, q, actor)
		if err != nil {
			return live.View{}, err
		}
		open, err := tracker.OpenProjects(ctx, q)
		if err != nil {
			return live.View{}, err
		}
		for _, id := range ids {
			if !slices.Contains(open, id) {
				closed = append(closed, id)
			}
		}
	}
	key := "projects:all"
	if !all {
		key = "projects:" + hashOf(strings.Join(closed, ","))
	}
	return live.View{
		Key:    key,
		Topics: []string{tracker.ProjectsTopic, tracker.IssuesTopic, people.Topic},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			q := querier(ctx, tx)
			ids, err := tracker.OpenProjects(ctx, q)
			if all {
				ids, err = tracker.AllProjects(ctx, q)
			}
			if err != nil {
				return nil, err
			}
			list, err := projectList(ctx, q, append(ids, closed...))
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.ProjectsList(list))
		},
	}, nil
}

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// createProject creates a project open to everyone (Q14), its creator its
// Admin and lead (FR-02), at most two a day per session (Q24), and sends
// the page to its board.
func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Payload
		Name        string `json:"name"`
		Key         string `json:"key"`
		Description string `json:"description"`
	}
	a, ok := s.decode(w, r, classCreateScope, &req)
	if !ok {
		return
	}
	cmd := tracker.CreateProject{By: a.By, Key: req.Key, Name: req.Name, Description: req.Description, Lead: a.By.Actor.ID, Open: true}
	s.actResult(w, r, a, cmd, func(res []byte) string {
		var v struct{ Key string }
		json.Unmarshal(res, &v)
		return "/projects/" + v.Key + "/board"
	})
}

// projectID resolves a project's key in an action, answering 404 itself
// for one the actor may not read.
func (s *Server) projectID(w http.ResponseWriter, r *http.Request, key string) (string, bool) {
	ctx := r.Context()
	p, err := s.db.ViewValue(ctx, func(tx *sql.Tx) (tracker.Project, error) {
		return readableProject(ctx, tx, people.Actor(ctx), key)
	})
	switch {
	case errors.Is(err, access.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
		return "", false
	case err != nil:
		s.fail(w, r, err)
		return "", false
	}
	return p.ID, true
}

type projectRequest struct {
	Payload
	Project string `json:"project"`
}

func (s *Server) editProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		projectRequest
		Name        string `json:"name"`
		Description string `json:"description"`
		Lead        string `json:"lead"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	if id, ok := s.projectID(w, r, req.Project); ok {
		s.act(w, r, a, tracker.EditProject{By: a.By, Project: id, Name: &req.Name, Description: &req.Description, Lead: &req.Lead})
	}
}

func (s *Server) archiveProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		projectRequest
		Archived bool `json:"archived"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	if id, ok := s.projectID(w, r, req.Project); ok {
		s.act(w, r, a, tracker.ArchiveProject{By: a.By, Project: id, Archived: req.Archived})
	}
}

func (s *Server) defineComponent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		projectRequest
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	if id, ok := s.projectID(w, r, req.Project); ok {
		s.act(w, r, a, tracker.DefineComponent{By: a.By, Project: id, Name: req.Name, Description: req.Description})
	}
}

func (s *Server) removeComponent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		projectRequest
		Name string `json:"name"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	if id, ok := s.projectID(w, r, req.Project); ok {
		s.act(w, r, a, tracker.RemoveComponent{By: a.By, Project: id, Name: req.Name})
	}
}

var statusPattern = regexp.MustCompile(`^[a-z0-9_-]{1,40}$`)

func (s *Server) setWIP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		projectRequest
		Status string `json:"status"`
		Limit  int    `json:"limit"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	if !statusPattern.MatchString(req.Status) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if id, ok := s.projectID(w, r, req.Project); ok {
		s.act(w, r, a, tracker.SetWIPLimit{By: a.By, Project: id, Status: req.Status, Limit: req.Limit})
	}
}

func (s *Server) setMember(w http.ResponseWriter, r *http.Request) {
	var req struct {
		projectRequest
		Person string `json:"person"`
		Role   string `json:"role"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	if id, ok := s.projectID(w, r, req.Project); ok {
		s.act(w, r, a, tracker.SetProjectMember{By: a.By, Project: id, Person: req.Person, Role: access.Role(req.Role)})
	}
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	var req struct {
		projectRequest
		Person string `json:"person"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	if id, ok := s.projectID(w, r, req.Project); ok {
		s.act(w, r, a, tracker.RemoveProjectMember{By: a.By, Project: id, Person: req.Person})
	}
}

// ── The board ─────────────────────────────────────────────────────────

// boardPage is a project's board, its page's first tab (FR-01.2): at
// /projects/{key} and /projects/{key}/board.
func (s *Server) boardPage(w http.ResponseWriter, r *http.Request) {
	me := people.Actor(r.Context())
	quick := views.QuickFilterOf(r.URL.Query())
	create := r.URL.Query().Get("create")
	if !statusPattern.MatchString(create) {
		create = ""
	}
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		p, err := readableProject(ctx, q, me, r.PathValue("key"))
		if err != nil {
			return Page{}, err
		}
		lanes, err := readLanes(ctx, q, p.ID, quick.Normalize(me.ID))
		if err != nil {
			return Page{}, err
		}
		head, err := readHead(ctx, q, p)
		if err != nil {
			return Page{}, err
		}
		labels, err := tracker.Labels(ctx, q, p.ID, "")
		if err != nil {
			return Page{}, err
		}
		if len(labels) > 30 {
			labels = labels[:30]
		}
		return Page{
			Title: p.Name + " board", Nav: "projects", Width: "full", Project: p.ID,
			Views: []string{"board:" + views.BoardArg(p.Key, quick), "boardhead:" + p.Key},
			Body: views.Board(views.BoardPage{Project: p, Filter: quick, Labels: labels, Lanes: lanes, Head: head,
				CanEdit: can(ctx, q, me, access.EditIssues, p.ID), Admin: can(ctx, q, me, access.Configure, p.ID), CreateStatus: create,
				Me: me.ID, Moderates: access.SiteCan(me, access.Moderate)}),
		}, nil
	})
}

// readLanes reads a board for a quick filter normalized for its viewer.
func readLanes(ctx context.Context, q sqlite.Querier, project string, f tracker.Filter) (views.Lanes, error) {
	b, err := tracker.Board(ctx, q, project, f)
	if err != nil {
		return views.Lanes{}, err
	}
	l := views.Lanes{Board: b, Now: time.Now()}
	if !f.IsZero() {
		if l.Totals, err = tracker.BoardCounts(ctx, q, project); err != nil {
			return l, err
		}
	}
	var ids []string
	for _, c := range b.Columns {
		for _, card := range c.Cards {
			if card.Assignee != "" {
				ids = append(ids, card.Assignee)
			}
		}
	}
	l.People, err = people.Lookup(ctx, q, ids...)
	return l, err
}

// revMark stands for a frame's revision until its bytes are hashed.
const revMark = "rev0000000000000"

// revised renders c with its revision, a hash of its bytes in place of
// revMark: the page tells by it whether a frame came since an action.
func revised(ctx context.Context, c templ.Component) ([]byte, error) {
	var buf bytes.Buffer
	if err := c.Render(ctx, &buf); err != nil {
		return nil, err
	}
	html := buf.Bytes()
	sum := sha256.Sum256(html)
	return bytes.Replace(html, []byte(revMark), []byte(hex.EncodeToString(sum[:8])), 1), nil
}

// lanesFrame is a board's frame: the lanes glide into place in a view
// transition scoped to them (MP-04.1).
func lanesFrame(ctx context.Context, l views.Lanes) ([]byte, error) {
	l.Rev = revMark
	html, err := revised(ctx, views.BoardLanes(l))
	if err != nil {
		return nil, err
	}
	// The spaces between the cards' parts lay nothing out (flex and grid).
	html = bytes.ReplaceAll(html, []byte("> <"), []byte("><"))
	return sse.PatchElements(html, sse.Patch{ViewTransition: true, ViewTransitionSelector: "#" + views.BoardLanesID}), nil
}

// boardView is a project's lanes for a quick filter: one view for every
// tab whose filter selects the same cards for the same person (MP-11).
func (s *Server) boardView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	key, f, err := views.ParseBoardArg(arg)
	if err != nil {
		return live.View{}, fmt.Errorf("%w: %v", errBadView, err)
	}
	p, err := readableProject(ctx, q, actor, key)
	if err != nil {
		return live.View{}, err
	}
	f = f.Normalize(actor.ID)
	return live.View{
		Key:    views.BoardKey(p.ID, f),
		Topics: []string{tracker.BoardTopic(p.ID), people.Topic},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			l, err := readLanes(ctx, querier(ctx, tx), p.ID, f)
			if err != nil {
				return nil, err
			}
			return lanesFrame(ctx, l)
		},
	}, nil
}

// readHead reads a board's head: the project, its active sprint and the
// people with cards on it.
func readHead(ctx context.Context, q sqlite.Querier, p tracker.Project) (views.BoardHead, error) {
	h := views.BoardHead{Project: p, Now: time.Now()}
	sprints, err := tracker.Sprints(ctx, q, p.ID)
	if err != nil {
		return h, err
	}
	for i, sp := range sprints {
		switch sp.State {
		case tracker.SprintActive:
			h.Sprint = &sprints[i]
		case tracker.SprintFuture:
			h.Next = append(h.Next, sp)
		}
	}
	if h.Sprint != nil {
		pr, err := tracker.ProgressOf(ctx, q, h.Sprint.ID)
		if err != nil {
			return h, err
		}
		h.Issues, h.Open, h.Points, h.Done = pr.Issues, pr.Open, pr.Points, pr.Done
	}
	ids, err := tracker.BoardAssignees(ctx, q, p.ID)
	if err != nil {
		return h, err
	}
	who, err := people.Lookup(ctx, q, ids...)
	if err != nil {
		return h, err
	}
	for _, id := range ids {
		h.Assignees = append(h.Assignees, who[id])
	}
	return h, nil
}

// boardHeadView is a board's head and the faces of its quick filter.
// Its presence row shows who is on the board, whatever their filter.
func (s *Server) boardHeadView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	p, err := readableProject(ctx, q, actor, arg)
	if err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    views.BoardHeadKey(p.ID),
		Topics: []string{tracker.BoardTopic(p.ID), people.Topic},
		Every:  10 * time.Minute, // the sprint's days left
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			q := querier(ctx, tx)
			pr, err := tracker.ProjectByID(ctx, q, p.ID)
			if err != nil {
				return nil, err
			}
			h, err := readHead(ctx, q, pr)
			if err != nil {
				return nil, err
			}
			return patches(ctx, views.BoardHeadRegion(h), views.BoardFaces(h))
		},
	}, nil
}

// patches renders components as one frame of several patches, each
// morphed in by its id.
func patches(ctx context.Context, cs ...templ.Component) ([]byte, error) {
	var out []byte
	for _, c := range cs {
		b, err := patch(ctx, c)
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
	}
	return out, nil
}

// keyPattern is an issue key as pages send it.
var keyPattern = regexp.MustCompile(`^[A-Z]{2,6}-[1-9][0-9]{0,8}$`)

// issueIDs resolves issue keys to ids in one read; "" stays "". An
// unknown key answers access.ErrNotFound.
func (s *Server) issueIDs(ctx context.Context, keys ...string) ([]string, error) {
	return s.db.ViewValue(ctx, func(tx *sql.Tx) ([]string, error) {
		out := make([]string, len(keys))
		for i, k := range keys {
			if k == "" {
				continue
			}
			l, err := tracker.IssueByKey(ctx, tx, k)
			if err != nil {
				return nil, err
			}
			out[i] = l.Issue
		}
		return out, nil
	})
}

// moveRequest is a board move: the card, its column, its new neighbours
// as keys, and the card's version the page showed (MP-04.3).
type moveRequest struct {
	Payload
	Issue  string `json:"issue"`
	Status string `json:"status"`
	Before string `json:"before"`
	After  string `json:"after"`
	Seen   int64  `json:"seen"`
}

func (m moveRequest) valid() bool {
	key := func(k string, optional bool) bool { return optional && k == "" || keyPattern.MatchString(k) }
	return key(m.Issue, false) && key(m.Before, true) && key(m.After, true) && statusPattern.MatchString(m.Status) && m.Seen >= 0
}

// moveCard moves a card to a column and between two others (FR-41.1).
// The outcome, a refusal included, reaches the page on the tab's stream.
func (s *Server) moveCard(w http.ResponseWriter, r *http.Request) {
	var req moveRequest
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	if !req.valid() {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ids, err := s.issueIDs(r.Context(), req.Issue, req.Before, req.After)
	if err != nil {
		s.failAction(w, r, err)
		return
	}
	s.act(w, r, a, tracker.Move{By: a.By, Issue: ids[0], Status: req.Status, Before: ids[1], After: ids[2], Seen: req.Seen})
}

// ── The backlog and sprints ───────────────────────────────────────────

func (s *Server) backlogPage(w http.ResponseWriter, r *http.Request) {
	me := people.Actor(r.Context())
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		p, err := readableProject(ctx, q, me, r.PathValue("key"))
		if err != nil {
			return Page{}, err
		}
		d, err := readBacklog(ctx, q, p.ID)
		if err != nil {
			return Page{}, err
		}
		return Page{
			Title: p.Name + " backlog", Nav: "projects", Project: p.ID, Views: []string{"backlog:" + p.Key},
			Body: views.Backlog(views.BacklogPage{Project: p, Data: d, CanEdit: can(ctx, q, me, access.EditIssues, p.ID), Admin: can(ctx, q, me, access.Configure, p.ID),
				Me: me.ID, Moderates: access.SiteCan(me, access.Moderate)}),
		}, nil
	})
}

func readBacklog(ctx context.Context, q sqlite.Querier, project string) (views.BacklogData, error) {
	b, err := tracker.Backlog(ctx, q, project)
	if err != nil {
		return views.BacklogData{}, err
	}
	var ids []string
	add := func(cards []tracker.Card) {
		for _, c := range cards {
			if c.Assignee != "" {
				ids = append(ids, c.Assignee)
			}
		}
	}
	for _, sp := range b.Sprints {
		add(sp.Cards)
	}
	add(b.Cards)
	who, err := people.Lookup(ctx, q, ids...)
	return views.BacklogData{View: b, People: who, Now: time.Now()}, err
}

// backlogView is a project's backlog: one view for everyone (FR-45.1).
func (s *Server) backlogView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	p, err := readableProject(ctx, q, actor, arg)
	if err != nil {
		return live.View{}, err
	}
	return live.View{
		Key:    views.BacklogKey(p.ID),
		Topics: []string{tracker.BoardTopic(p.ID), people.Topic},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			d, err := readBacklog(ctx, querier(ctx, tx), p.ID)
			if err != nil {
				return nil, err
			}
			d.Rev = revMark
			html, err := revised(ctx, views.BacklogRegion(d))
			if err != nil {
				return nil, err
			}
			sprints, err := patch(ctx, views.BacklogSprints(d.View))
			if err != nil {
				return nil, err
			}
			return append(sse.PatchElements(html), sprints...), nil
		},
	}, nil
}

// planIssues ranks issues and puts them in a sprint or in none (FR-45,
// FR-46.1): a drag in the backlog, or the checked issues' bulk move.
func (s *Server) planIssues(w http.ResponseWriter, r *http.Request) {
	var req struct {
		projectRequest
		Issues []struct {
			Issue string `json:"issue"`
			Seen  int64  `json:"seen"`
		} `json:"issues"`
		Sprint string `json:"sprint"`
		Rank   bool   `json:"rank"`
		Before string `json:"before"`
		After  string `json:"after"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	bad := len(req.Issues) == 0 || len(req.Issues) > tracker.MaxBatch || req.Sprint != "" && !tracker.ValidID(req.Sprint) ||
		req.Before != "" && !keyPattern.MatchString(req.Before) || req.After != "" && !keyPattern.MatchString(req.After)
	keys := []string{req.Before, req.After}
	for _, i := range req.Issues {
		bad = bad || !keyPattern.MatchString(i.Issue) || i.Seen < 0
		keys = append(keys, i.Issue)
	}
	if bad {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if _, ok := s.projectID(w, r, req.Project); !ok {
		return
	}
	ids, err := s.issueIDs(r.Context(), keys...)
	if err != nil {
		s.failAction(w, r, err)
		return
	}
	cmd := tracker.PlanIssues{By: a.By, Sprint: req.Sprint, Rank: req.Rank, Before: ids[0], After: ids[1]}
	for i, is := range req.Issues {
		cmd.Issues = append(cmd.Issues, tracker.Seen{Issue: ids[i+2], Seen: is.Seen})
	}
	s.act(w, r, a, cmd)
}

func (s *Server) createSprint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		projectRequest
		Name  string `json:"name"`
		Goal  string `json:"goal"`
		Start string `json:"start"`
		End   string `json:"end"`
	}
	a, ok := s.decode(w, r, classCreate, &req)
	if !ok {
		return
	}
	if id, ok := s.projectID(w, r, req.Project); ok {
		s.act(w, r, a, tracker.CreateSprint{By: a.By, Project: id, Name: req.Name, Goal: req.Goal, Start: req.Start, End: req.End})
	}
}

// SprintLength is how long a sprint started without dates runs.
const SprintLength = 14 * 24 * time.Hour

func (s *Server) startSprint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		projectRequest
		Sprint string `json:"sprint"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	id, ok := s.projectID(w, r, req.Project)
	if !ok {
		return
	}
	cmd := tracker.StartSprint{By: a.By, Project: id, Sprint: req.Sprint}
	sprints, err := s.db.ViewValue(r.Context(), func(tx *sql.Tx) ([]tracker.Sprint, error) { return tracker.Sprints(r.Context(), tx, id) })
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, sp := range sprints {
		if sp.ID == req.Sprint && sp.Start == "" && sp.End == "" {
			cmd.Start, cmd.End = a.By.At.Format(time.DateOnly), a.By.At.Add(SprintLength).Format(time.DateOnly)
		}
	}
	s.act(w, r, a, cmd)
}

func (s *Server) completeSprint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		projectRequest
		Sprint string `json:"sprint"`
		MoveTo string `json:"move_to"`
	}
	a, ok := s.decode(w, r, classField, &req)
	if !ok {
		return
	}
	if id, ok := s.projectID(w, r, req.Project); ok {
		s.act(w, r, a, tracker.CompleteSprint{By: a.By, Project: id, Sprint: req.Sprint, MoveTo: req.MoveTo})
	}
}

// ── Activity and settings ─────────────────────────────────────────────

func (s *Server) activityPage(w http.ResponseWriter, r *http.Request) {
	me := people.Actor(r.Context())
	s.render(w, r, http.StatusOK, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		p, err := readableProject(ctx, q, me, r.PathValue("key"))
		if err != nil {
			return Page{}, err
		}
		feed, err := s.ScopeActivity(ctx, q, access.ProjectScope(p.ID), time.Now())
		if err != nil {
			return Page{}, err
		}
		return Page{Title: p.Name + " activity", Nav: "projects", Project: p.ID, Views: []string{"activity-project:" + p.ID},
			Body: views.ProjectActivity(views.ProjectPage{Project: p, Admin: can(ctx, q, me, access.Configure, p.ID), Feed: feed})}, nil
	})
}

// settingsPage is a project's settings, for those who may configure it;
// anyone else who may read the project hears 403.
func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	me := people.Actor(ctx)
	status := http.StatusOK
	if allowed, err := s.db.ViewValue(ctx, func(tx *sql.Tx) (bool, error) {
		p, err := readableProject(ctx, tx, me, r.PathValue("key"))
		return err != nil || can(ctx, tx, me, access.Configure, p.ID), nil
	}); err == nil && !allowed {
		status = http.StatusForbidden
	}
	s.render(w, r, status, func(ctx context.Context, q sqlite.Querier) (Page, error) {
		p, err := readableProject(ctx, q, me, r.PathValue("key"))
		if err != nil {
			return Page{}, err
		}
		if !can(ctx, q, me, access.Configure, p.ID) {
			return Page{Title: p.Name + " settings", Nav: "projects", Body: views.ProjectForbidden(p, "Only admins of this project can change its settings.")}, nil
		}
		live, err := readSettings(ctx, q, p)
		if err != nil {
			return Page{}, err
		}
		cands, err := tracker.Assignable(ctx, q, p.ID, "", time.Now())
		if err != nil {
			return Page{}, err
		}
		var humans []people.Person
		for _, c := range cands {
			if c.Kind != people.Seed {
				humans = append(humans, c)
			}
		}
		return Page{Title: p.Name + " settings", Nav: "projects", Views: []string{"settings:" + p.Key},
			Body: views.SettingsPage(views.Settings{Project: p, Columns: live.Columns, Live: live, Candidates: humans})}, nil
	})
}

func readSettings(ctx context.Context, q sqlite.Querier, p tracker.Project) (views.SettingsLive, error) {
	l := views.SettingsLive{Project: p}
	var err error
	if l.Members, err = tracker.Members(ctx, q, p.ID); err != nil {
		return l, err
	}
	if l.Components, err = tracker.Components(ctx, q, p.ID); err != nil {
		return l, err
	}
	if l.Columns, err = tracker.Statuses(ctx, q, p.ID); err != nil {
		return l, err
	}
	ids := make([]string, len(l.Members))
	for i, m := range l.Members {
		ids[i] = m.Person
	}
	l.People, err = people.Lookup(ctx, q, ids...)
	return l, err
}

// settingsView is a project's members, components and limits, for those
// who may configure it.
func (s *Server) settingsView(ctx context.Context, q sqlite.Querier, actor people.Person, arg string) (live.View, error) {
	p, err := readableProject(ctx, q, actor, arg)
	if err != nil {
		return live.View{}, err
	}
	if !can(ctx, q, actor, access.Configure, p.ID) {
		return live.View{}, &access.Denial{}
	}
	return live.View{
		Key:    "settings:" + p.ID,
		Topics: []string{tracker.ProjectTopic(p.ID), people.Topic},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			q := querier(ctx, tx)
			pr, err := tracker.ProjectByID(ctx, q, p.ID)
			if err != nil {
				return nil, err
			}
			l, err := readSettings(ctx, q, pr)
			if err != nil {
				return nil, err
			}
			return patch(ctx, views.SettingsRegion(l))
		},
	}, nil
}
