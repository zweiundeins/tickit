package projects

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/delaneyj/toolbelt"
	"github.com/mbolli/tickit/internal/shared"
	"zombiezen.com/go/sqlite"
)

type ProjectProjectionStore struct {
	db *toolbelt.Database
}

var _ shared.ProjectionRebuilder = (*ProjectProjectionStore)(nil)

func NewProjectProjectionStore(ctx context.Context, dbFilename string) (*ProjectProjectionStore, error) {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS projects_projection (
			id TEXT PRIMARY KEY,
			data TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
	}
	db, err := toolbelt.NewDatabase(ctx, dbFilename, migrations)
	if err != nil {
		return nil, fmt.Errorf("failed to create projects projection db: %w", err)
	}
	return NewProjectProjectionStoreWithDB(db), nil
}

func NewProjectProjectionStoreWithDB(db *toolbelt.Database) *ProjectProjectionStore {
	return &ProjectProjectionStore{db: db}
}

// ApplyEvent updates the projection based on a project event
func (ps *ProjectProjectionStore) ApplyEvent(ctx context.Context, event shared.Event) error {
	switch e := event.(type) {
	case ProjectCreatedEvent:
		return ps.applyCreated(ctx, e)
	case ProjectUpdatedEvent:
		return ps.applyUpdated(ctx, e)
	case ProjectDeletedEvent:
		return ps.applyDeleted(ctx, e)
	default:
		return nil // ignore unrelated events
	}
}

func (ps *ProjectProjectionStore) applyCreated(ctx context.Context, e ProjectCreatedEvent) error {
	return ps.save(ctx, e.Project.ID, e.Project)
}

func (ps *ProjectProjectionStore) applyUpdated(ctx context.Context, e ProjectUpdatedEvent) error {
	return ps.save(ctx, e.Project.ID, e.Project)
}

func (ps *ProjectProjectionStore) applyDeleted(ctx context.Context, e ProjectDeletedEvent) error {
	return ps.delete(ctx, e.ID)
}

func (ps *ProjectProjectionStore) GetProject(ctx context.Context, id string) (*Project, error) {
	var data string
	err := ps.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`SELECT data FROM projects_projection WHERE id = ?`)
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
	var project Project
	if err := json.Unmarshal([]byte(data), &project); err != nil {
		return nil, err
	}
	return &project, nil
}

func (ps *ProjectProjectionStore) ListProjects(ctx context.Context) ([]*Project, error) {
	var projects []*Project
	err := ps.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`SELECT data FROM projects_projection`)
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
			var project Project
			if err := json.Unmarshal([]byte(data), &project); err != nil {
				return err
			}
			projects = append(projects, &project)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return projects, nil
}

func (ps *ProjectProjectionStore) save(ctx context.Context, id string, project *Project) error {
	b, err := json.Marshal(project)
	if err != nil {
		return err
	}
	return ps.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`INSERT OR REPLACE INTO projects_projection (id, data, updated_at) VALUES (?, ?, ?)`)
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

func (ps *ProjectProjectionStore) delete(ctx context.Context, id string) error {
	return ps.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`DELETE FROM projects_projection WHERE id = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()
		stmt.BindText(1, id)
		_, err = stmt.Step()
		return err
	})
}

// RebuildAllFromEvents rebuilds all project projections from the event store
func (ps *ProjectProjectionStore) RebuildAllFromEvents(ctx context.Context, eventStore *shared.EventStore) error {
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
