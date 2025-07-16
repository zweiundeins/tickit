package projects

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/mbolli/tickit/internal/shared"
)

// Commands

type CreateProjectCommand struct {
	Name        string
	Slug        string
	Description string
}

func (c CreateProjectCommand) GetCommandType() string { return "CreateProject" }

type UpdateProjectCommand struct {
	ID          string
	Name        *string
	Slug        *string
	Description *string
}

func (c UpdateProjectCommand) GetCommandType() string { return "UpdateProject" }

type DeleteProjectCommand struct {
	ID string
}

func (c DeleteProjectCommand) GetCommandType() string { return "DeleteProject" }

// CommandHandler for projects
// (Stub, to be implemented with storage/event sourcing)
type ProjectCommandHandler struct {
	eventBus        *shared.EventBusWrapper
	eventStore      *shared.EventStore
	projectionStore *ProjectProjectionStore
}

func NewProjectCommandHandler(eventBus *shared.EventBusWrapper, eventStore *shared.EventStore, projectionStore *ProjectProjectionStore) *ProjectCommandHandler {
	return &ProjectCommandHandler{eventBus: eventBus, eventStore: eventStore, projectionStore: projectionStore}
}

func (h *ProjectCommandHandler) HandleCreateProject(ctx context.Context, cmd CreateProjectCommand) (*Project, error) {
	project := &Project{
		ID:          uuid.New().String(),
		Name:        cmd.Name,
		Slug:        cmd.Slug,
		Description: cmd.Description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	event := ProjectCreatedEvent{
		Project:   project,
		Timestamp: project.CreatedAt,
	}
	// 1. Persist event to SQLite event store
	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		return nil, err
	}
	// 2. Update projection
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return nil, err
	}
	// 3. Publish signal to JetStream for real-time updates
	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		return nil, err
	}
	return project, nil
}

func (h *ProjectCommandHandler) HandleUpdateProject(ctx context.Context, cmd UpdateProjectCommand) error {
	project := &Project{
		ID:          cmd.ID,
		Name:        derefString(cmd.Name),
		Slug:        derefString(cmd.Slug),
		Description: derefString(cmd.Description),
		UpdatedAt:   time.Now(),
	}
	event := ProjectUpdatedEvent{
		Project:   project,
		Timestamp: project.UpdatedAt,
	}
	// 1. Persist event to SQLite event store
	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		return err
	}
	// 2. Update projection
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return err
	}
	// 3. Publish signal to JetStream for real-time updates
	return h.eventBus.PublishEvent(ctx, event)
}

func (h *ProjectCommandHandler) HandleDeleteProject(ctx context.Context, cmd DeleteProjectCommand) error {
	event := ProjectDeletedEvent{
		ID:        cmd.ID,
		Timestamp: time.Now(),
	}
	// 1. Persist event to SQLite event store
	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		return err
	}
	// 2. Update projection
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return err
	}
	// 3. Publish signal to JetStream for real-time updates
	return h.eventBus.PublishEvent(ctx, event)
}

// Helper for pointer deref
func derefString(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}
