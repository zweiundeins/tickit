package tracker

import (
	"context"
	"time"

	"github.com/zweiundeins/lith/sqlite"
)

// The aggregates of dashboard gadgets (FR-60): a project's issues by
// status, and created against resolved per day (FR-38). The web keeps
// them in a cache against the hub's version of the project's board.

// StatusCount is how many issues of a project have a status.
type StatusCount struct {
	Status   string
	Name     string
	Category Category
	Count    int
}

// StatusCounts are a project's issues by status, in workflow order, every
// status of the workflow included.
func StatusCounts(ctx context.Context, q sqlite.Querier, project string) ([]StatusCount, error) {
	rows, err := q.QueryContext(ctx, `SELECT s.id, s.name, s.category,
		(SELECT count(*) FROM issues i WHERE i.project_id = s.project_id AND i.status = s.id AND i.deleted_at IS NULL)
		FROM statuses s WHERE s.project_id = ? ORDER BY s.position`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StatusCount
	for rows.Next() {
		var c StatusCount
		if err := rows.Scan(&c.Status, &c.Name, &c.Category, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DayCount is how many issues were created and resolved on a day.
type DayCount struct {
	Day      time.Time // midnight in the zone asked for
	Created  int
	Resolved int
}

// CreatedResolved are a project's issues created and resolved on each of
// the days days up to and with today, in loc, oldest first. An issue
// resolved and reopened counts as resolved on the day it was resolved
// last (resolved_at).
func CreatedResolved(ctx context.Context, q sqlite.Querier, project string, now time.Time, days int, loc *time.Location) ([]DayCount, error) {
	now = now.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	first := today.AddDate(0, 0, 1-days)
	out := make([]DayCount, days)
	for i := range out {
		out[i].Day = first.AddDate(0, 0, i)
	}
	index := func(ms int64) int {
		t := time.UnixMilli(ms).In(loc)
		d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		for i := range out {
			if out[i].Day.Equal(d) {
				return i
			}
		}
		return -1
	}
	rows, err := q.QueryContext(ctx, `SELECT created_at, coalesce(resolved_at, 0) FROM issues
		WHERE project_id = ? AND deleted_at IS NULL AND (created_at >= ?2 OR resolved_at >= ?2)`, project, first.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var created, resolved int64
		if err := rows.Scan(&created, &resolved); err != nil {
			return nil, err
		}
		if i := index(created); i >= 0 {
			out[i].Created++
		}
		if resolved > 0 {
			if i := index(resolved); i >= 0 {
				out[i].Resolved++
			}
		}
	}
	return out, rows.Err()
}
