package projects

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	cwd, _ := os.Getwd()
	absPath, _ := filepath.Abs(dbFilename)
	fmt.Printf("[DEBUG] CWD: %s\n", cwd)
	fmt.Printf("[DEBUG] ProjectProjection dbFilename: %s (abs: %s)\n", dbFilename, absPath)
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS projects_projection (
			id TEXT PRIMARY KEY,
			data TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
	}
	db, err := toolbelt.NewDatabase(ctx, dbFilename, migrations)
	if err != nil {
		fmt.Printf("[ERROR] Failed to create projects projection DB at %s: %v\n", dbFilename, err)
		return nil, fmt.Errorf("failed to create projects projection db: %w", err)
	}
	fmt.Printf("[DEBUG] toolbelt.NewDatabase returned for %s\n", dbFilename)
	if _, err := os.Stat(dbFilename); err == nil {
		fmt.Printf("[INFO] Projects projection DB file created: %s\n", dbFilename)
	} else {
		fmt.Printf("[WARN] Projects projection DB file missing after creation: %s\n", dbFilename)
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
	case ProjectPatchedEvent:
		return ps.applyPatched(ctx, e)
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

func (ps *ProjectProjectionStore) applyPatched(ctx context.Context, e ProjectPatchedEvent) error {
	project, err := ps.GetProject(ctx, e.ID)
	if err != nil || project == nil {
		return err
	}
	for field, value := range e.Changes {
		switch field {
		case "Name":
			if v, ok := value.(string); ok {
				project.Name = v
			}
		case "Slug":
			if v, ok := value.(string); ok {
				project.Slug = v
			}
		case "Description":
			if v, ok := value.(string); ok {
				project.Description = v
			}
		case "Archived":
			if v, ok := value.(bool); ok {
				project.Archived = v
			}
		case "UpdatedAt":
			if v, ok := value.(time.Time); ok {
				project.UpdatedAt = v
			}
		}
	}
	return ps.save(ctx, e.ID, project)
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
		fmt.Printf("[ERROR] Failed to marshal project: %v\n", err)
		return err
	}
	return ps.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`INSERT OR REPLACE INTO projects_projection (id, data, updated_at) VALUES (?, ?, ?)`)
		if err != nil {
			fmt.Printf("[ERROR] Failed to prepare project insert statement: %v\n", err)
			return err
		}
		defer stmt.Finalize()
		stmt.BindText(1, id)
		stmt.BindText(2, string(b))
		stmt.BindInt64(3, time.Now().Unix())
		_, err = stmt.Step()
		if err != nil {
			fmt.Printf("[ERROR] Failed to execute project insert statement: %v\n", err)
		}
		return err
	})
}

func (ps *ProjectProjectionStore) delete(ctx context.Context, id string) error {
	return ps.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`DELETE FROM projects_projection WHERE id = ?`)
		if err != nil {
			fmt.Printf("[ERROR] Failed to prepare project delete statement: %v\n", err)
			return err
		}
		defer stmt.Finalize()
		stmt.BindText(1, id)
		_, err = stmt.Step()
		if err != nil {
			fmt.Printf("[ERROR] Failed to execute project delete statement: %v\n", err)
		}
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
