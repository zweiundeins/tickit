package tracker

import (
	"context"
	"database/sql"

	"github.com/zweiundeins/lith/sqlite"
)

// The reads of the issue list and search beside Search: the issues of
// some ids, every issue of a query (CSV export) and the values a list's
// filters offer (FR-51.2).

// IssuesByID reads the issues with the ids, deleted ones left out.
func IssuesByID(ctx context.Context, q sqlite.Querier, ids []string) (map[string]Issue, error) {
	out := make(map[string]Issue, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT `+issueColumns+issueFrom+`
		WHERE i.id IN (SELECT value FROM json_each(?)) AND i.deleted_at IS NULL`, jsonList(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		i, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		out[i.ID] = i
	}
	return out, rows.Err()
}

// MaxExport is how many issues an export holds at most (FR-152).
const MaxExport = 10000

// SearchAll returns every issue that matches a query, in its order, up to
// limit: Search without pages.
func SearchAll(ctx context.Context, q sqlite.Querier, query Query, limit int) ([]Issue, error) {
	f := query.Filter.Normalize(query.Me)
	args := filterArgs(query.Readable, f)
	joins := ` LEFT JOIN people pa ON pa.id = i.assignee LEFT JOIN people pr ON pr.id = i.reporter`
	rows, err := q.QueryContext(ctx, `SELECT `+issueColumns+issueFrom+joins+filterWhere+query.Sort.orderBy()+` LIMIT ?18`,
		append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Issue
	for rows.Next() {
		i, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// SprintOption is a sprint as a filter offers it.
type SprintOption struct {
	ID, Name, ProjectKey, State string
}

// FilterOptions are the values the filters of a list offer, from the
// projects a person may read: every value is one the model has.
type FilterOptions struct {
	Projects   []Project
	Statuses   []Status // each id once, in workflow order
	Labels     []string
	Components []string
	Sprints    []SprintOption
	People     []string // ids of the assignees and reporters of the issues
}

// ListOptions reads the values of a list's filters.
func ListOptions(ctx context.Context, q sqlite.Querier, readable []string) (FilterOptions, error) {
	var o FilterOptions
	var err error
	if o.Projects, err = Projects(ctx, q, readable); err != nil {
		return o, err
	}
	in := jsonList(readable)
	strs := func(query string, scan func(*sql.Rows) error) error {
		rows, err := q.QueryContext(ctx, query, in)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if err := scan(rows); err != nil {
				return err
			}
		}
		return rows.Err()
	}
	err = strs(`SELECT id, min(name), min(category) FROM statuses WHERE project_id IN (SELECT value FROM json_each(?))
		GROUP BY id ORDER BY min(position), id`, func(r *sql.Rows) error {
		var s Status
		err := r.Scan(&s.ID, &s.Name, &s.Category)
		o.Statuses = append(o.Statuses, s)
		return err
	})
	if err != nil {
		return o, err
	}
	err = strs(`SELECT DISTINCT name FROM labels WHERE project_id IN (SELECT value FROM json_each(?)) ORDER BY name`, func(r *sql.Rows) error {
		var s string
		err := r.Scan(&s)
		o.Labels = append(o.Labels, s)
		return err
	})
	if err != nil {
		return o, err
	}
	err = strs(`SELECT DISTINCT name FROM components WHERE project_id IN (SELECT value FROM json_each(?)) ORDER BY name COLLATE NOCASE`, func(r *sql.Rows) error {
		var s string
		err := r.Scan(&s)
		o.Components = append(o.Components, s)
		return err
	})
	if err != nil {
		return o, err
	}
	err = strs(`SELECT s.id, s.name, p.key, s.state FROM sprints s JOIN projects p ON p.id = s.project_id
		WHERE s.project_id IN (SELECT value FROM json_each(?))
		ORDER BY CASE s.state WHEN 'active' THEN 0 WHEN 'future' THEN 1 ELSE 2 END, p.key, s.created_at`, func(r *sql.Rows) error {
		var s SprintOption
		err := r.Scan(&s.ID, &s.Name, &s.ProjectKey, &s.State)
		o.Sprints = append(o.Sprints, s)
		return err
	})
	if err != nil {
		return o, err
	}
	err = strs(`SELECT person FROM (SELECT assignee AS person FROM issues WHERE project_id IN (SELECT value FROM json_each(?1)) AND deleted_at IS NULL
		UNION SELECT reporter FROM issues WHERE project_id IN (SELECT value FROM json_each(?1)) AND deleted_at IS NULL) WHERE person != '' ORDER BY person`,
		func(r *sql.Rows) error {
			var s string
			err := r.Scan(&s)
			o.People = append(o.People, s)
			return err
		})
	return o, err
}
