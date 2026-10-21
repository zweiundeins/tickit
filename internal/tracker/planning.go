package tracker

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// The board's and the backlog's commands on several issues at once, the
// members of a project and the reads the board's quick filters need.

// MaxBatch is how many issues one backlog action moves at most.
const MaxBatch = 100

// Seen is an issue with the version the page showed it at.
type Seen struct {
	Issue string
	Seen  int64
}

func checkBatch(by tabs.By, issues []Seen) error {
	if err := checkBy(by); err != nil {
		return err
	}
	if len(issues) == 0 || len(issues) > MaxBatch {
		return errors.New("tracker: 1 to 100 issues")
	}
	for i, s := range issues {
		if !ValidID(s.Issue) || slices.ContainsFunc(issues[:i], func(o Seen) bool { return o.Issue == s.Issue }) {
			return errors.New("tracker: a malformed or repeated issue")
		}
	}
	return nil
}

// RankIssues puts issues, in their order, between After and Before in the
// project's one order (FR-45.1, FR-45.2): the first goes where a Move to
// those neighbours puts it, each next one after the one before. Their
// statuses stay. One refusal stores nothing of any.
type RankIssues struct {
	By            tabs.By
	Issues        []Seen
	Before, After string
}

func (c RankIssues) Validate() error { return checkBatch(c.By, c.Issues) }

func (c RankIssues) Apply(tx *writer.Tx) error {
	after := c.After
	for i, s := range c.Issues {
		m := Move{By: c.By, Issue: s.Issue, Seen: s.Seen, After: after}
		if i == 0 || after == "" {
			m.Before = c.Before
		}
		if err := m.Apply(tx); err != nil {
			return err
		}
		after = s.Issue
	}
	return nil
}

// PlanIssues puts issues in a sprint, or in no sprint when Sprint is
// empty (FR-46.1), and, when Rank is set, ranks them as RankIssues does:
// a drag from the backlog into a sprint does both.
type PlanIssues struct {
	By            tabs.By
	Issues        []Seen
	Sprint        string
	Rank          bool
	Before, After string
}

func (c PlanIssues) Validate() error {
	if c.Sprint != "" && !ValidID(c.Sprint) {
		return errors.New("tracker: a malformed sprint")
	}
	return checkBatch(c.By, c.Issues)
}

func (c PlanIssues) Apply(tx *writer.Tx) error {
	for _, s := range c.Issues {
		if err := (SetSprint{By: c.By, Issue: s.Issue, Seen: s.Seen, Sprint: c.Sprint}).Apply(tx); err != nil {
			return err
		}
	}
	if !c.Rank {
		return nil
	}
	return RankIssues{By: c.By, Issues: c.Issues, Before: c.Before, After: c.After}.Apply(tx)
}

// SetProjectMember gives a person a role in a project, or changes it
// (FR-05.1): the project's Admins do. The last Admin keeps the role
// (FR-05.2).
type SetProjectMember struct {
	By      tabs.By
	Project string
	Person  string
	Role    access.Role
}

func (c SetProjectMember) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	if !ValidID(c.Project) || !people.ValidID(c.Person) || !c.Role.Valid() {
		return errors.New("tracker: a malformed membership")
	}
	return nil
}

func (c SetProjectMember) Apply(tx *writer.Tx) error {
	if _, err := projectState(tx, c.Project); err != nil {
		return err
	}
	if err := require(tx, c.By, access.ManageMembers, c.Project); err != nil {
		return err
	}
	p, err := people.Get(tx.Context(), tx, c.Person)
	if err != nil {
		return err
	}
	if p.Kind == people.Deleted || p.Kind == people.Seed {
		return tabs.Refuse("person", "Only guests and accounts can be members.")
	}
	return access.SetMember(tx, access.ProjectScope(c.Project), c.Person, c.Role, c.By.Actor.ID, at(c.By))
}

// RemoveProjectMember ends a person's membership of a project (FR-05.1).
// In an open project they keep a Member's rights, as everyone does (Q14).
type RemoveProjectMember struct {
	By      tabs.By
	Project string
	Person  string
}

func (c RemoveProjectMember) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	if !ValidID(c.Project) || !people.ValidID(c.Person) {
		return errors.New("tracker: a malformed membership")
	}
	return nil
}

func (c RemoveProjectMember) Apply(tx *writer.Tx) error {
	if _, err := projectState(tx, c.Project); err != nil {
		return err
	}
	if err := require(tx, c.By, access.ManageMembers, c.Project); err != nil {
		return err
	}
	return access.RemoveMember(tx, access.ProjectScope(c.Project), c.Person)
}

// Member is a person's membership of a project.
type Member struct {
	Person  string
	Role    access.Role
	AddedAt time.Time
}

// Members lists a project's members: Admins first, then by when they
// were added.
func Members(ctx context.Context, q sqlite.Querier, project string) ([]Member, error) {
	rows, err := q.QueryContext(ctx, `SELECT person_id, role, added_at FROM memberships WHERE scope_kind = 'project' AND scope_id = ?
		ORDER BY CASE role WHEN 'admin' THEN 0 WHEN 'member' THEN 1 ELSE 2 END, added_at, person_id`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		var added int64
		if err := rows.Scan(&m.Person, &m.Role, &added); err != nil {
			return nil, err
		}
		m.AddedAt = time.Unix(added, 0)
		out = append(out, m)
	}
	return out, rows.Err()
}

// BoardAssignees lists the people assigned to the issues a project's
// board shows, whatever the quick filter: the faces above the board
// (FR-42), most cards first.
func BoardAssignees(ctx context.Context, q sqlite.Querier, project string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT i.assignee FROM issues i JOIN statuses s ON s.project_id = i.project_id AND s.id = i.status
		WHERE i.project_id = ?1 AND i.deleted_at IS NULL AND s.board_column != '' AND i.assignee != ''
		AND coalesce((SELECT id FROM sprints WHERE project_id = ?1 AND state = 'active'), i.sprint) = i.sprint
		GROUP BY i.assignee ORDER BY count(*) DESC, i.assignee LIMIT 12`, project)
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

// BoardCounts counts the cards of each column of a project's board
// without a quick filter, by column id, for its work-in-progress limits
// (FR-43).
func BoardCounts(ctx context.Context, q sqlite.Querier, project string) (map[string]int, error) {
	rows, err := q.QueryContext(ctx, `SELECT s.board_column, count(*) FROM issues i JOIN statuses s ON s.project_id = i.project_id AND s.id = i.status
		WHERE i.project_id = ?1 AND i.deleted_at IS NULL AND s.board_column != ''
		AND coalesce((SELECT id FROM sprints WHERE project_id = ?1 AND state = 'active'), i.sprint) = i.sprint GROUP BY s.board_column`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}

// SprintProgress is how far a sprint is (FR-46.2): its issues, those not
// done, their story points and those of the done ones.
type SprintProgress struct {
	Issues, Open, Points, Done int
}

// ProgressOf reads a sprint's progress.
func ProgressOf(ctx context.Context, q sqlite.Querier, sprint string) (SprintProgress, error) {
	var p SprintProgress
	err := q.QueryRowContext(ctx, `SELECT count(*), count(*) FILTER (WHERE status_category != 'done'), coalesce(sum(points), 0),
		coalesce(sum(points) FILTER (WHERE status_category = 'done'), 0) FROM issues WHERE sprint = ? AND deleted_at IS NULL`, sprint).
		Scan(&p.Issues, &p.Open, &p.Points, &p.Done)
	return p, err
}

// OpenProjects lists the ids of the projects open to everyone (Q14),
// archived ones included; AllProjects lists every project's.
func OpenProjects(ctx context.Context, q sqlite.Querier) ([]string, error) {
	return ids(ctx, q, `SELECT id FROM scopes WHERE kind = 'project' AND open = 1 ORDER BY id`)
}

// AllProjects lists the ids of every project.
func AllProjects(ctx context.Context, q sqlite.Querier) ([]string, error) {
	return ids(ctx, q, `SELECT id FROM projects ORDER BY id`)
}

func ids(ctx context.Context, q sqlite.Querier, query string) ([]string, error) {
	rows, err := q.QueryContext(ctx, query)
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
