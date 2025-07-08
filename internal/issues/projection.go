package issues

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/delaneyj/toolbelt"
	"github.com/mbolli/tickit/internal/shared"
	"zombiezen.com/go/sqlite"
)

type IssueProjectionStore struct {
	db *toolbelt.Database
}

var _ shared.ProjectionRebuilder = (*IssueProjectionStore)(nil)

func NewIssueProjectionStore(ctx context.Context, dbFilename string) (*IssueProjectionStore, error) {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS issues_projection (
			id TEXT PRIMARY KEY,
			data TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
	}
	db, err := toolbelt.NewDatabase(ctx, dbFilename, migrations)
	if err != nil {
		return nil, fmt.Errorf("failed to create issues projection db: %w", err)
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

func (ps *IssueProjectionStore) ListIssues(ctx context.Context) ([]*Issue, error) {
	var issues []*Issue
	err := ps.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`SELECT data FROM issues_projection`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()
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

func (ps *IssueProjectionStore) save(ctx context.Context, id string, issue *Issue) error {
	b, err := json.Marshal(issue)
	if err != nil {
		return err
	}
	return ps.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`INSERT OR REPLACE INTO issues_projection (id, data, updated_at) VALUES (?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()
		stmt.BindText(1, id)
		stmt.BindText(2, string(b))
		stmt.BindInt64(3, time.Now().Unix())
		_, err = stmt.Step()
		return err
	})
}

func (ps *IssueProjectionStore) delete(ctx context.Context, id string) error {
	return ps.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`DELETE FROM issues_projection WHERE id = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()
		stmt.BindText(1, id)
		_, err = stmt.Step()
		return err
	})
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
