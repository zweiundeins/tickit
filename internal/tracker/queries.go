package tracker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/zweiundeins/lith/sqlite"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/people"
)

// The reads take a sqlite.Querier: a GET's transaction, a render's
// sqlite.QuerierFrom(ctx), or a command's *writer.Tx. They check no
// rights: a page or a view checks access.Can(Read) on the project first,
// and lists pass the projects the person may read (Readable).

// Project is a project as pages show it.
type Project struct {
	ID          string
	Key         string
	Name        string
	Description string
	Lead        string // a person id, or empty
	Open        bool
	Archived    bool
	CreatedBy   string
	CreatedAt   time.Time
	OpenIssues  int // issues not deleted and not in the Done category (FR-01.1)
}

const projectColumns = `p.id, p.key, p.name, p.description, p.lead, p.open, p.archived, p.created_by, p.created_at,
	(SELECT count(*) FROM issues i WHERE i.project_id = p.id AND i.deleted_at IS NULL AND i.status_category != 'done')`

type scanner interface{ Scan(...any) error }

func scanProject(r scanner) (Project, error) {
	var p Project
	var created int64
	err := r.Scan(&p.ID, &p.Key, &p.Name, &p.Description, &p.Lead, &p.Open, &p.Archived, &p.CreatedBy, &created, &p.OpenIssues)
	p.CreatedAt = fromMillis(created)
	return p, err
}

// ProjectByKey finds a project by its key, without regard to case.
func ProjectByKey(ctx context.Context, q sqlite.Querier, key string) (Project, error) {
	k, ok := NormalizeKey(key)
	if !ok {
		return Project{}, notFound("project", key)
	}
	p, err := scanProject(q.QueryRowContext(ctx, `SELECT `+projectColumns+` FROM projects p WHERE p.key = ?`, k))
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, notFound("project", key)
	}
	return p, err
}

// ProjectByID reads a project.
func ProjectByID(ctx context.Context, q sqlite.Querier, id string) (Project, error) {
	p, err := scanProject(q.QueryRowContext(ctx, `SELECT `+projectColumns+` FROM projects p WHERE p.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, notFound("project", id)
	}
	return p, err
}

// Projects lists the projects of the ids, by name, archived ones last
// (FR-01.1).
func Projects(ctx context.Context, q sqlite.Querier, readable []string) ([]Project, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+projectColumns+` FROM projects p
		WHERE p.id IN (SELECT value FROM json_each(?)) ORDER BY p.archived, p.name COLLATE NOCASE, p.key`, jsonList(readable))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Readable returns the ids of the projects a person may read, for the
// lists and Search.
func Readable(ctx context.Context, q sqlite.Querier, actor people.Person) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT id FROM projects ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []string
	for _, id := range ids {
		ok, err := access.Can(ctx, q, actor, access.Read, access.ProjectScope(id))
		if errors.Is(err, access.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, id)
		}
	}
	return out, nil
}

// Column is a status of a project's workflow as its board shows it.
type Column struct {
	Status
	Position int  // in the workflow, from 1
	WIP      int  // the work-in-progress limit, 0 for none (FR-43)
	Over     bool // more cards than the limit
	Cards    []Card
	Points   int // the cards' story points
}

// Statuses lists a project's workflow in order, with the board's limits.
func Statuses(ctx context.Context, q sqlite.Querier, project string) ([]Column, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, name, category, position, wip_limit FROM statuses WHERE project_id = ? ORDER BY position`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Column
	for rows.Next() {
		var c Column
		if err := rows.Scan(&c.ID, &c.Name, &c.Category, &c.Position, &c.WIP); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Components lists a project's components by name.
func Components(ctx context.Context, q sqlite.Querier, project string) ([]Component, error) {
	rows, err := q.QueryContext(ctx, `SELECT name, description FROM components WHERE project_id = ? ORDER BY name COLLATE NOCASE`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Component
	for rows.Next() {
		var c Component
		if err := rows.Scan(&c.Name, &c.Description); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Sprints lists a project's sprints: the active one, the future ones, the
// closed ones, each in the order they were created.
func Sprints(ctx context.Context, q sqlite.Querier, project string) ([]Sprint, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, name, goal, starts, ends, state FROM sprints WHERE project_id = ?
		ORDER BY CASE state WHEN 'active' THEN 0 WHEN 'future' THEN 1 ELSE 2 END, created_at, id`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Sprint
	for rows.Next() {
		var s Sprint
		if err := rows.Scan(&s.ID, &s.Name, &s.Goal, &s.Start, &s.End, &s.State); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Issue is an issue's row, as lists, boards and chips show it.
type Issue struct {
	ID          string
	ProjectID   string
	ProjectKey  string
	ProjectName string
	Key         string
	Number      int64
	Type        IssueType
	Summary     string
	Status      string // the status's id
	StatusName  string
	StatusOrder int // its position in the workflow
	Category    Category
	Priority    Priority
	Assignee    string // a person id, or empty
	Reporter    string
	ParentID    string
	EpicID      string
	Labels      []string
	Components  []string
	Due         string // YYYY-MM-DD, or empty
	Points      *int
	Estimate    *int // minutes
	Remaining   *int // minutes
	Rank        string
	Sprint      string
	Created     time.Time
	Updated     time.Time
	Resolved    time.Time // zero while not resolved (FR-38)
	Deleted     time.Time // zero while not deleted
	DeletedBy   string
	// Hidden is a deletion by a moderator (moderation.go), which they
	// may undo; Deleted and DeletedBy say when and who.
	Hidden       bool
	HiddenReason string
	Version      int64 // the version a change sends as Seen (MP-08.1)
}

// Overdue reports whether an issue is past its due date on the day of
// now and not done (FR-25.2).
func (i Issue) Overdue(now time.Time) bool {
	return i.Due != "" && i.Category != Done && i.Due < now.Format(time.DateOnly)
}

const issueColumns = `i.id, i.project_id, i.project_key, p.name, i.key, i.number, i.type, i.summary, i.status, coalesce(s.name, i.status),
	coalesce(s.position, 0), i.status_category, i.priority, i.assignee, i.reporter, i.parent_id, i.epic_id, i.labels, i.components,
	i.due, i.points, i.estimate, i.remaining, i.rank, i.sprint, i.created_at, i.updated_at, i.resolved_at, i.deleted_at,
	i.deleted_by, i.hidden_at IS NOT NULL, i.hidden_reason, i.version`

func scanIssue(r scanner) (Issue, error) {
	var i Issue
	var labels, comps string
	var points, estimate, remaining, resolved, deleted sql.NullInt64
	var created, updated int64
	err := r.Scan(&i.ID, &i.ProjectID, &i.ProjectKey, &i.ProjectName, &i.Key, &i.Number, &i.Type, &i.Summary, &i.Status, &i.StatusName,
		&i.StatusOrder, &i.Category, &i.Priority, &i.Assignee, &i.Reporter, &i.ParentID, &i.EpicID, &labels, &comps,
		&i.Due, &points, &estimate, &remaining, &i.Rank, &i.Sprint, &created, &updated, &resolved, &deleted,
		&i.DeletedBy, &i.Hidden, &i.HiddenReason, &i.Version)
	if err != nil {
		return i, err
	}
	i.Points, i.Estimate, i.Remaining = intPtr(points), intPtr(estimate), intPtr(remaining)
	i.Created, i.Updated, i.Resolved, i.Deleted = fromMillis(created), fromMillis(updated), nullTime(resolved), nullTime(deleted)
	if err := json.Unmarshal([]byte(labels), &i.Labels); err != nil {
		return i, err
	}
	err = json.Unmarshal([]byte(comps), &i.Components)
	return i, err
}

func intPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// KeyLookup is what an issue key names (FR-12): the issue, its key now
// (another after a move: redirect to it), and whether and by whom it was
// deleted (410).
type KeyLookup struct {
	Issue     string
	Project   string
	Key       string // the issue's key now
	Moved     bool   // the key looked up is an old one
	Deleted   bool
	DeletedBy string
	DeletedAt time.Time
	Hidden    bool // by a moderator: the deletion they may undo
}

// IssueByKey resolves an issue key, without regard to case; an unknown
// one answers access.ErrNotFound.
func IssueByKey(ctx context.Context, q sqlite.Querier, key string) (KeyLookup, error) {
	pk, n, ok := ParseIssueKey(key)
	if !ok {
		return KeyLookup{}, notFound("issue", key)
	}
	k := IssueKey(pk, n)
	var l KeyLookup
	var deleted sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT i.id, i.project_id, i.key, i.deleted_at, i.deleted_by, i.hidden_at IS NOT NULL
		FROM issue_keys k JOIN issues i ON i.id = k.issue_id WHERE k.key = ?`, k).Scan(&l.Issue, &l.Project, &l.Key, &deleted, &l.DeletedBy, &l.Hidden)
	if errors.Is(err, sql.ErrNoRows) {
		return KeyLookup{}, notFound("issue", key)
	}
	l.Moved, l.Deleted, l.DeletedAt = l.Key != k, deleted.Valid, nullTime(deleted)
	return l, err
}

// IssueRef is another issue as an issue's page names it: a parent, a
// child, an epic, a link's other end.
type IssueRef struct {
	ID         string
	Key        string
	Summary    string
	Type       IssueType
	Status     string
	StatusName string
	Category   Category
	Priority   Priority
	Assignee   string
	Points     *int
	Deleted    bool
}

const refColumns = `i.id, i.key, i.summary, i.type, i.status, coalesce(s.name, i.status), i.status_category, i.priority, i.assignee, i.points, i.deleted_at IS NOT NULL`

const refFrom = ` FROM issues i LEFT JOIN statuses s ON s.project_id = i.project_id AND s.id = i.status`

func scanRef(r scanner) (IssueRef, error) {
	var x IssueRef
	var points sql.NullInt64
	err := r.Scan(&x.ID, &x.Key, &x.Summary, &x.Type, &x.Status, &x.StatusName, &x.Category, &x.Priority, &x.Assignee, &points, &x.Deleted)
	x.Points = intPtr(points)
	return x, err
}

func refs(ctx context.Context, q sqlite.Querier, query string, args ...any) ([]IssueRef, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IssueRef
	for rows.Next() {
		x, err := scanRef(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func ref(ctx context.Context, q sqlite.Querier, id string) (*IssueRef, error) {
	if id == "" {
		return nil, nil
	}
	x, err := scanRef(q.QueryRowContext(ctx, `SELECT `+refColumns+refFrom+` WHERE i.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &x, err
}

// LinkRef is a link as one of its issues shows it (FR-22.1).
type LinkRef struct {
	Type    LinkType
	Outward bool
	Wording string // "blocks", "is blocked by", "relates to"
	Issue   IssueRef
}

// Progress is an epic's children done against all (FR-20.1).
type Progress struct {
	Done int
	All  int
}

// Detail is everything an issue's page shows but comments and history
// (FR-13.1): people are ids, for people.Lookup.
type Detail struct {
	Issue
	Description json.RawMessage // the document, nil when empty
	Project     Project
	Parent      *IssueRef
	Epic        *IssueRef
	Children    []IssueRef // in rank order
	Progress    Progress   // of the children
	Links       []LinkRef  // by wording, then key
	Watchers    []string   // person ids, in the order they started watching
	Keys        []string   // every key it had, the current last
}

// IssueDetail reads an issue's detail; an unknown id answers access.ErrNotFound.
// A deleted issue answers its detail with Deleted set.
func IssueDetail(ctx context.Context, q sqlite.Querier, id string) (Detail, error) {
	var d Detail
	var desc sql.NullString
	var oldKeys string
	row := q.QueryRowContext(ctx, `SELECT `+issueColumns+`, i.description, i.old_keys`+issueFrom+` WHERE i.id = ?`, id)
	i, err := scanIssue(scanFunc(func(dest ...any) error { return row.Scan(append(dest, &desc, &oldKeys)...) }))
	if errors.Is(err, sql.ErrNoRows) {
		return d, notFound("issue", id)
	}
	if err != nil {
		return d, err
	}
	d.Issue = i
	if desc.Valid {
		d.Description = json.RawMessage(desc.String)
	}
	d.Keys = append(strings.Fields(oldKeys), i.Key)
	if d.Project, err = ProjectByID(ctx, q, i.ProjectID); err != nil {
		return d, err
	}
	if d.Parent, err = ref(ctx, q, i.ParentID); err != nil {
		return d, err
	}
	if i.EpicID != i.ParentID {
		if d.Epic, err = ref(ctx, q, i.EpicID); err != nil {
			return d, err
		}
	} else {
		d.Epic = d.Parent
	}
	if d.Children, err = EpicChildren(ctx, q, id); err != nil {
		return d, err
	}
	for _, c := range d.Children {
		d.Progress.All++
		if c.Category == Done {
			d.Progress.Done++
		}
	}
	rows, err := q.QueryContext(ctx, `SELECT l.type, l.outward, `+refColumns+` FROM issue_links l JOIN issues i ON i.id = l.other_id
		LEFT JOIN statuses s ON s.project_id = i.project_id AND s.id = i.status WHERE l.issue_id = ? ORDER BY l.created_at, i.key`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var l LinkRef
		var points sql.NullInt64
		x := &l.Issue
		if err := rows.Scan(&l.Type, &l.Outward, &x.ID, &x.Key, &x.Summary, &x.Type, &x.Status, &x.StatusName, &x.Category, &x.Priority, &x.Assignee, &points, &x.Deleted); err != nil {
			rows.Close()
			return d, err
		}
		x.Points = intPtr(points)
		l.Wording = l.Type.Wording(l.Outward)
		d.Links = append(d.Links, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return d, err
	}
	slices.SortStableFunc(d.Links, func(a, b LinkRef) int {
		return strings.Compare(a.Wording+"\x00"+a.Issue.Key, b.Wording+"\x00"+b.Issue.Key)
	})
	d.Watchers, err = Watchers(ctx, q, id)
	return d, err
}

type scanFunc func(dest ...any) error

func (f scanFunc) Scan(dest ...any) error { return f(dest...) }

// Watchers lists who watches an issue, in the order they started.
func Watchers(ctx context.Context, q sqlite.Querier, issue string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT person_id FROM issue_watchers WHERE issue_id = ? ORDER BY since, person_id`, issue)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// EpicChildren lists an issue's children not deleted, in rank order: an
// epic's stories, tasks and bugs, or an issue's subtasks.
func EpicChildren(ctx context.Context, q sqlite.Querier, id string) ([]IssueRef, error) {
	return refs(ctx, q, `SELECT `+refColumns+refFrom+` WHERE i.parent_id = ? AND i.deleted_at IS NULL ORDER BY i.rank`, id)
}

// Epics lists a project's epics not deleted, for the epic picker
// (FR-20.2), in rank order.
func Epics(ctx context.Context, q sqlite.Querier, project string) ([]IssueRef, error) {
	return refs(ctx, q, `SELECT `+refColumns+refFrom+` WHERE i.project_id = ? AND i.type = 'epic' AND i.deleted_at IS NULL ORDER BY i.rank`, project)
}

// Card is an issue as a board or a backlog shows it (FR-40.2).
type Card struct {
	Issue
	Epic   *IssueRef // the epic's key and summary, when it has one
	Parent *IssueRef // a subtask's parent
}

const cardColumns = issueColumns + `, coalesce(e.key, ''), coalesce(e.summary, ''), coalesce(pp.key, ''), coalesce(pp.summary, ''), coalesce(pp.type, '')`

const cardFrom = issueFrom + ` LEFT JOIN issues e ON e.id = i.epic_id LEFT JOIN issues pp ON pp.id = i.parent_id`

func cards(ctx context.Context, q sqlite.Querier, query string, args ...any) ([]Card, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Card
	for rows.Next() {
		var c Card
		var ek, es, pk, ps, pt string
		i, err := scanIssue(scanFunc(func(dest ...any) error { return rows.Scan(append(dest, &ek, &es, &pk, &ps, &pt)...) }))
		if err != nil {
			return nil, err
		}
		c.Issue = i
		if ek != "" {
			c.Epic = &IssueRef{ID: i.EpicID, Key: ek, Summary: es, Type: Epic}
		}
		if pk != "" && i.Type == Subtask {
			c.Parent = &IssueRef{ID: i.ParentID, Key: pk, Summary: ps, Type: IssueType(pt)}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// BoardView is a project's board (FR-40): a column per status but
// Backlog, in workflow order, with its cards in rank order.
type BoardView struct {
	Project Project
	Sprint  *Sprint // the active sprint, whose issues alone the board shows (FR-46.2)
	Columns []Column
	Total   int // cards on the board
}

// Board reads a project's board, its cards filtered by quick (FR-42):
// types, priorities, assignees (with Unassigned), labels and the text
// count; normalize quick with the viewer's id for Me first.
func Board(ctx context.Context, q sqlite.Querier, project string, quick Filter) (BoardView, error) {
	var b BoardView
	p, err := ProjectByID(ctx, q, project)
	if err != nil {
		return b, err
	}
	b.Project = p
	cols, err := Statuses(ctx, q, project)
	if err != nil {
		return b, err
	}
	sprints, err := Sprints(ctx, q, project)
	if err != nil {
		return b, err
	}
	f := Filter{Types: quick.Types, Priorities: quick.Priorities, Assignees: quick.Assignees, Labels: quick.Labels, Text: quick.Text}.Normalize("")
	f.Projects = []string{p.Key}
	if len(sprints) > 0 && sprints[0].State == SprintActive {
		b.Sprint = &sprints[0]
		f.Sprints = []string{sprints[0].ID}
	}
	all, err := cards(ctx, q, `SELECT `+cardColumns+cardFrom+filterWhere+` AND i.status != 'backlog' AND ?17 = ?17 ORDER BY i.rank, i.number`,
		filterArgs([]string{project}, f)...)
	if err != nil {
		return b, err
	}
	for _, c := range cols {
		if !OnBoard(c.ID) {
			continue
		}
		for _, card := range all {
			if card.Status == c.ID {
				c.Cards = append(c.Cards, card)
				if card.Points != nil {
					c.Points += *card.Points
				}
			}
		}
		c.Over = c.WIP > 0 && len(c.Cards) > c.WIP
		b.Total += len(c.Cards)
		b.Columns = append(b.Columns, c)
	}
	return b, nil
}

// SprintCards is a sprint with its issues in rank order.
type SprintCards struct {
	Sprint Sprint
	Cards  []Card
	Points int
}

// BacklogView is a project's backlog (FR-45): the issues not in the Done
// category and not in the active sprint, in rank order; those in a
// future sprint under it, the rest below.
type BacklogView struct {
	Project Project
	Active  *Sprint
	Sprints []SprintCards // the future sprints, in order
	Cards   []Card        // in no sprint
	Points  int
}

// Backlog reads a project's backlog.
func Backlog(ctx context.Context, q sqlite.Querier, project string) (BacklogView, error) {
	var b BacklogView
	p, err := ProjectByID(ctx, q, project)
	if err != nil {
		return b, err
	}
	b.Project = p
	sprints, err := Sprints(ctx, q, project)
	if err != nil {
		return b, err
	}
	active := ""
	for _, s := range sprints {
		switch s.State {
		case SprintActive:
			b.Active, active = &s, s.ID
		case SprintFuture:
			b.Sprints = append(b.Sprints, SprintCards{Sprint: s})
		}
	}
	all, err := cards(ctx, q, `SELECT `+cardColumns+cardFrom+` WHERE i.project_id = ? AND i.deleted_at IS NULL
		AND i.status_category != 'done' AND (? = '' OR i.sprint != ?) ORDER BY i.rank, i.number`, project, active, active)
	if err != nil {
		return b, err
	}
	for _, c := range all {
		pts := 0
		if c.Points != nil {
			pts = *c.Points
		}
		i := slices.IndexFunc(b.Sprints, func(s SprintCards) bool { return s.Sprint.ID == c.Sprint })
		if i < 0 {
			b.Cards = append(b.Cards, c)
			b.Points += pts
			continue
		}
		b.Sprints[i].Cards = append(b.Sprints[i].Cards, c)
		b.Sprints[i].Points += pts
	}
	return b, nil
}

// Comment is a comment as an issue's page shows it: a deleted one keeps
// its place without its body ("Comment deleted", FR-66.1), a hidden one
// too (Hidden set; HiddenBody holds it for moderators).
type Comment struct {
	ID           string
	Issue        string
	Author       string
	Body         json.RawMessage // nil when deleted or hidden
	HiddenBody   json.RawMessage
	Mentions     []string
	Created      time.Time
	Edited       time.Time // zero unless edited
	Deleted      time.Time
	DeletedBy    string
	Hidden       time.Time
	HiddenBy     string
	HiddenReason string
	Version      int64
}

// IsDeleted and IsHidden say what became of a comment.
func (c Comment) IsDeleted() bool { return !c.Deleted.IsZero() }
func (c Comment) IsHidden() bool  { return !c.Hidden.IsZero() }

// Comments lists an issue's comments, oldest first (FR-65.2).
func Comments(ctx context.Context, q sqlite.Querier, issue string) ([]Comment, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, subject_id, author, body, mentions, created_at, edited_at, deleted_at, deleted_by,
		hidden_at, hidden_by, hidden_reason, version FROM comments WHERE subject = 'issue' AND subject_id = ? ORDER BY created_at, id`, issue)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Comment
	for rows.Next() {
		var c Comment
		var body sql.NullString
		var mentions string
		var created int64
		var edited, deleted, hidden sql.NullInt64
		if err := rows.Scan(&c.ID, &c.Issue, &c.Author, &body, &mentions, &created, &edited, &deleted, &c.DeletedBy,
			&hidden, &c.HiddenBy, &c.HiddenReason, &c.Version); err != nil {
			return nil, err
		}
		c.Created, c.Edited, c.Deleted, c.Hidden = fromMillis(created), nullTime(edited), nullTime(deleted), nullTime(hidden)
		if body.Valid {
			c.Body = json.RawMessage(body.String)
		}
		if c.IsHidden() {
			c.HiddenBody, c.Body = c.Body, nil
		}
		if err := json.Unmarshal([]byte(mentions), &c.Mentions); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Labels lists a project's labels that start with prefix, without regard
// to case, by name, 20 at most (FR-23.1).
func Labels(ctx context.Context, q sqlite.Querier, project, prefix string) ([]string, error) {
	p := strings.ToLower(strings.TrimSpace(prefix))
	rows, err := q.QueryContext(ctx, `SELECT name FROM labels WHERE project_id = ? AND substr(name, 1, length(?2)) = ?2 ORDER BY name LIMIT 20`, project, p)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Assignable lists who the assignee picker offers in a project (FR-17.1):
// its members, the people who acted in it in the 24 hours before now, and
// the seeded people, whose names contain query without regard to case,
// by name. A site admin may be assigned too, without being listed.
func Assignable(ctx context.Context, q sqlite.Querier, project, query string, now time.Time) ([]people.Person, error) {
	rows, err := q.QueryContext(ctx, `SELECT person_id FROM memberships WHERE scope_kind = 'project' AND scope_id = ?1 AND role IN ('member', 'admin')
		UNION SELECT person_id FROM tracker_actors WHERE project_id = ?1 AND at >= ?2
		UNION SELECT id FROM people WHERE kind = 'seed'`, project, now.Add(-AssignableRecent*time.Hour).UnixMilli())
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	found, err := people.Lookup(ctx, q, ids...)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	var out []people.Person
	for _, p := range found {
		if p.Kind == people.Deleted || p.Inactive || !strings.Contains(strings.ToLower(p.Name), needle) {
			continue
		}
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b people.Person) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}
