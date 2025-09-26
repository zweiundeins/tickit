package issues

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/delaneyj/toolbelt"
	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/internal/shared"
	"zombiezen.com/go/sqlite"
)

type IssueProjectionStore struct {
	db *toolbelt.Database
}

var _ shared.ProjectionRebuilder = (*IssueProjectionStore)(nil)

func NewIssueProjectionStore(ctx context.Context, dbFilename string) (*IssueProjectionStore, error) {
	absPath, _ := filepath.Abs(dbFilename)
	logger.Debug().
		Str("dbFilename", dbFilename).
		Str("absPath", absPath).
		Msg("IssueProjection dbFilename")
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS issues_projection (
			id TEXT PRIMARY KEY,
			data TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS issue_counters (
			project_id TEXT PRIMARY KEY,
			next_number INTEGER NOT NULL DEFAULT 1
		)`,
	}
	db, err := toolbelt.NewDatabase(ctx, dbFilename, migrations)
	if err != nil {
		logger.Error().
			Str("dbFilename", dbFilename).
			Err(err).
			Msg("Failed to create issues projection DB")
		return nil, fmt.Errorf("failed to create issues projection db: %w", err)
	}
	if _, err := os.Stat(dbFilename); err == nil {
		logger.Info().Msgf("Issues projection DB file created: %s\n", dbFilename)
	} else {
		logger.Warn().Msgf("Issues projection DB file missing after creation: %s\n", dbFilename)
	}
	return NewIssueProjectionStoreWithDB(db), nil
}

func NewIssueProjectionStoreWithDB(db *toolbelt.Database) *IssueProjectionStore {
	return &IssueProjectionStore{db: db}
}

// ApplyEvent updates the projection based on an issue event
func (ps *IssueProjectionStore) ApplyEvent(ctx context.Context, event shared.Event) error {
	switch e := event.(type) {
	case IssueCreatedEvent:
		return ps.applyCreated(ctx, e)
	case IssueUpdatedEvent:
		return ps.applyUpdated(ctx, e)
	case IssueDeletedEvent:
		return ps.applyDeleted(ctx, e)
	case IssuePatchedEvent:
		return ps.applyPatched(ctx, e)
	case IssueAssigneeChangedEvent:
		return ps.applyAssigneeChanged(ctx, e)
	case IssueStatusChangedEvent:
		return ps.applyStatusChanged(ctx, e)
	case IssueTitleChangedEvent:
		return ps.applyTitleChanged(ctx, e)
	case IssueDescriptionChangedEvent:
		return ps.applyDescriptionChanged(ctx, e)
	default:
		return nil // ignore unrelated events
	}
}

func (ps *IssueProjectionStore) applyCreated(ctx context.Context, e IssueCreatedEvent) error {
	return ps.save(ctx, e.Issue.ID, e.Issue)
}

func (ps *IssueProjectionStore) applyUpdated(ctx context.Context, e IssueUpdatedEvent) error {
	return ps.save(ctx, e.Issue.ID, e.Issue)
}

func (ps *IssueProjectionStore) applyDeleted(ctx context.Context, e IssueDeletedEvent) error {
	return ps.delete(ctx, e.ID)
}

func (ps *IssueProjectionStore) applyPatched(ctx context.Context, e IssuePatchedEvent) error {
	issue, err := ps.GetIssue(ctx, e.ID)
	if err != nil || issue == nil {
		return err
	}
	for field, value := range e.Changes {
		switch field {
		case "Title":
			if v, ok := value.(string); ok {
				issue.Title = v
			}
		case "Description":
			if v, ok := value.(string); ok {
				issue.Description = v
			}
		case "Status":
			if v, ok := value.(string); ok {
				issue.Status = v
			}
		case "Priority":
			if v, ok := value.(string); ok {
				issue.Priority = v
			}
		case "Assignee":
			if v, ok := value.(string); ok {
				issue.Assignee = v
			}
		case "Reporter":
			if v, ok := value.(string); ok {
				issue.Reporter = v
			}
		case "ProjectID":
			if v, ok := value.(string); ok {
				issue.ProjectID = v
			}
		}
	}
	return ps.save(ctx, e.ID, issue)
}

func (ps *IssueProjectionStore) applyAssigneeChanged(ctx context.Context, e IssueAssigneeChangedEvent) error {
	issue, err := ps.GetIssue(ctx, e.ID)
	if err != nil || issue == nil {
		return err
	}
	issue.Assignee = e.NewAssignee
	return ps.save(ctx, e.ID, issue)
}

func (ps *IssueProjectionStore) applyStatusChanged(ctx context.Context, e IssueStatusChangedEvent) error {
	issue, err := ps.GetIssue(ctx, e.ID)
	if err != nil || issue == nil {
		return err
	}
	issue.Status = e.NewStatus
	return ps.save(ctx, e.ID, issue)
}

func (ps *IssueProjectionStore) applyTitleChanged(ctx context.Context, e IssueTitleChangedEvent) error {
	issue, err := ps.GetIssue(ctx, e.ID)
	if err != nil || issue == nil {
		return err
	}
	issue.Title = e.NewTitle
	return ps.save(ctx, e.ID, issue)
}

func (ps *IssueProjectionStore) applyDescriptionChanged(ctx context.Context, e IssueDescriptionChangedEvent) error {
	issue, err := ps.GetIssue(ctx, e.ID)
	if err != nil || issue == nil {
		return err
	}
	issue.Description = e.NewDescription
	return ps.save(ctx, e.ID, issue)
}

func (ps *IssueProjectionStore) GetIssue(ctx context.Context, id string) (*Issue, error) {
	var data string
	err := ps.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`SELECT data FROM issues_projection WHERE id = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()
		stmt.BindText(1, id)
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if !rowReturned {
			return nil // not found
		}
		data = stmt.ColumnText(0)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if data == "" {
		return nil, nil
	}
	var issue Issue
	if err := json.Unmarshal([]byte(data), &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

func (ps *IssueProjectionStore) ListIssues(ctx context.Context, query ListIssuesQuery) ([]*Issue, error) {
	var issues []*Issue
	sql := "SELECT data FROM issues_projection WHERE 1=1"
	args := []interface{}{}

	if query.Status != nil {
		sql += " AND json_extract(data, '$.status') = ?"
		args = append(args, *query.Status)
	}
	if query.Assignee != nil {
		sql += " AND json_extract(data, '$.assignee') = ?"
		args = append(args, *query.Assignee)
	}
	if query.Reporter != nil {
		sql += " AND json_extract(data, '$.reporter') = ?"
		args = append(args, *query.Reporter)
	}
	if query.Priority != nil {
		sql += " AND json_extract(data, '$.priority') = ?"
		args = append(args, *query.Priority)
	}
	if query.ProjectID != nil {
		sql += " AND json_extract(data, '$.project_id') = ?"
		args = append(args, *query.ProjectID)
	}
	if query.ContainsText != nil {
		sql += " AND (json_extract(data, '$.title') LIKE ? OR json_extract(data, '$.description') LIKE ?)"
		likeVal := "%" + *query.ContainsText + "%"
		args = append(args, likeVal, likeVal)
	}
	if query.CreatedAfter != nil {
		sql += " AND json_extract(data, '$.created_at') >= ?"
		args = append(args, query.CreatedAfter.Format(time.RFC3339))
	}
	if query.CreatedBefore != nil {
		sql += " AND json_extract(data, '$.created_at') <= ?"
		args = append(args, query.CreatedBefore.Format(time.RFC3339))
	}
	// ORDER BY logic
	orderBy := "id"
	orderDir := "DESC"
	if query.OrderBy != nil {
		switch *query.OrderBy {
		case "id":
			orderBy = *query.OrderBy
		case "number":
			orderBy = "json_extract(data, '$.number')"
		case "updated_at", "created_at":
			orderBy = "datetime(json_extract(data, '$." + *query.OrderBy + "'))"
		case "title", "status", "priority", "assignee", "reporter", "project_id":
			// Use json_extract for these fields
			orderBy = "json_extract(data, '$." + *query.OrderBy + "')"
		}
	}
	if query.OrderDir != nil && (*query.OrderDir == "asc" || *query.OrderDir == "desc" || *query.OrderDir == "ASC" || *query.OrderDir == "DESC") {
		orderDir = *query.OrderDir
	}
	sql += " ORDER BY " + orderBy + " " + orderDir
	logger.Debug().
		Str("sql", sql).
		Interface("args", args).
		Msg("ListIssues query")

	err := ps.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(sql)
		if err != nil {
			return err
		}
		defer stmt.Finalize()
		for i, arg := range args {
			stmt.BindText(i+1, fmt.Sprintf("%v", arg))
		}
		for {
			rowReturned, err := stmt.Step()
			if err != nil {
				return err
			}
			if !rowReturned {
				break
			}
			data := stmt.ColumnText(0)
			var issue Issue
			if err := json.Unmarshal([]byte(data), &issue); err != nil {
				return err
			}
			issues = append(issues, &issue)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return issues, nil
}

func (ps *IssueProjectionStore) GetIssueByKey(ctx context.Context, projectID string, number int) (*Issue, error) {
	var data string
	err := ps.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`SELECT data FROM issues_projection WHERE json_extract(data, '$.project_id') = ? AND json_extract(data, '$.number') = ? LIMIT 1`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()
		stmt.BindText(1, projectID)
		stmt.BindInt64(2, int64(number))
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if !rowReturned {
			return nil // not found
		}
		data = stmt.ColumnText(0)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if data == "" {
		return nil, nil
	}
	var issue Issue
	if err := json.Unmarshal([]byte(data), &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

func (ps *IssueProjectionStore) save(ctx context.Context, id string, issue *Issue) error {
	b, err := json.Marshal(issue)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to marshal issue")
		return err
	}
	return ps.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`INSERT OR REPLACE INTO issues_projection (id, data, updated_at) VALUES (?, ?, ?)`)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to prepare issue insert statement")
			return err
		}
		defer stmt.Finalize()
		stmt.BindText(1, id)
		stmt.BindText(2, string(b))
		stmt.BindInt64(3, time.Now().Unix())
		_, err = stmt.Step()
		if err != nil {
			logger.Error().Err(err).Msg("Failed to execute issue insert statement")
		}
		return err
	})
}

func (ps *IssueProjectionStore) delete(ctx context.Context, id string) error {
	return ps.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`DELETE FROM issues_projection WHERE id = ?`)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to prepare issue delete statement")
			return err
		}
		defer stmt.Finalize()
		stmt.BindText(1, id)
		_, err = stmt.Step()
		if err != nil {
			logger.Error().Err(err).Msg("Failed to execute issue delete statement")
		}
		return err
	})
}

func (ps *IssueProjectionStore) GetNextIssueNumber(ctx context.Context, projectID string) (int, error) {
	var nextNumber int
	err := ps.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		// Try to get current number
		stmt, err := tx.Prepare(`SELECT next_number FROM issue_counters WHERE project_id = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()
		stmt.BindText(1, projectID)
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}

		if rowReturned {
			// Counter exists, get current number
			nextNumber = int(stmt.ColumnInt64(0))
		} else {
			// Counter doesn't exist, start at 1
			nextNumber = 1
		}

		// Increment and save
		nextNumber++
		stmt2, err := tx.Prepare(`INSERT OR REPLACE INTO issue_counters (project_id, next_number) VALUES (?, ?)`)
		if err != nil {
			return err
		}
		defer stmt2.Finalize()
		stmt2.BindText(1, projectID)
		stmt2.BindInt64(2, int64(nextNumber-1)) // Save the number we just used
		_, err = stmt2.Step()
		return err
	})
	return nextNumber - 1, err // Return the number we used
}

// RebuildAllFromEvents rebuilds all issue projections from the event store
func (ps *IssueProjectionStore) RebuildAllFromEvents(ctx context.Context, eventStore *shared.EventStore) error {
	events, err := eventStore.GetEventsSince(ctx, time.Time{})
	if err != nil {
		return fmt.Errorf("failed to get events: %w", err)
	}
	for _, event := range events {
		if err := ps.ApplyEvent(ctx, event); err != nil {
			return fmt.Errorf("failed to apply event: %w", err)
		}
	}
	return nil
}
