package projects

import (
	"context"
	"fmt"
	"time"

	"github.com/mbolli/tickit/internal/shared"
)

// Commands

type CreateProjectCommand struct {
	ID          string
	Name        string
	Slug        string
	Description string
	Archived    bool
	CreatedBy   string // User ID of the project creator
}

func (c CreateProjectCommand) GetCommandType() string { return "CreateProject" }

type UpdateProjectCommand struct {
	ID          string
	Name        *string
	Slug        *string
	Description *string
	Archived    *bool
}

func (c UpdateProjectCommand) GetCommandType() string { return "UpdateProject" }

type DeleteProjectCommand struct {
	ID string
}

func (c DeleteProjectCommand) GetCommandType() string { return "DeleteProject" }

type PatchProjectCommand struct {
	ID          string
	Name        *string
	Slug        *string
	Description *string
	Archived    *bool
}

func (c PatchProjectCommand) GetCommandType() string { return "PatchProject" }

// CommandHandler for projects
// (Stub, to be implemented with storage/event sourcing)
type ProjectCommandHandler struct {
	eventBus        shared.EventPublisher
	eventStore      *shared.EventStore
	projectionStore *ProjectProjectionStore
}

func NewProjectCommandHandler(eventBus shared.EventPublisher, eventStore *shared.EventStore, projectionStore *ProjectProjectionStore) *ProjectCommandHandler {
	return &ProjectCommandHandler{eventBus: eventBus, eventStore: eventStore, projectionStore: projectionStore}
}

func (h *ProjectCommandHandler) HandleCreateProject(ctx context.Context, cmd CreateProjectCommand) error {
	project := &Project{
		ID:          cmd.ID,
		Name:        cmd.Name,
		Slug:        cmd.Slug,
		Description: cmd.Description,
		Archived:    cmd.Archived,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	event := ProjectCreatedEvent{
		Project:   project,
		CreatedBy: cmd.CreatedBy,
		Timestamp: project.CreatedAt,
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
	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		return err
	}
	return nil
}

func (h *ProjectCommandHandler) HandleUpdateProject(ctx context.Context, cmd UpdateProjectCommand) error {
	project := &Project{
		ID:          cmd.ID,
		Name:        derefString(cmd.Name),
		Slug:        derefString(cmd.Slug),
		Description: derefString(cmd.Description),
		Archived:    derefBool(cmd.Archived),
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

func (h *ProjectCommandHandler) HandlePatchProject(ctx context.Context, cmd PatchProjectCommand) error {
	// Get the current project from the projection store
	project, err := h.projectionStore.GetProject(ctx, cmd.ID)
	if err != nil {
		return err
	}
	if project == nil {
		return fmt.Errorf("project not found")
	}

	changes := make(map[string]interface{})

	if cmd.Name != nil {
		project.Name = *cmd.Name
		changes["Name"] = *cmd.Name
	}
	if cmd.Slug != nil {
		project.Slug = *cmd.Slug
		changes["Slug"] = *cmd.Slug
	}
	if cmd.Description != nil {
		project.Description = *cmd.Description
		changes["Description"] = *cmd.Description
	}
	if cmd.Archived != nil {
		project.Archived = *cmd.Archived
		changes["Archived"] = *cmd.Archived
	}

	if len(changes) == 0 {
		return nil // No changes
	}

	project.UpdatedAt = time.Now()
	changes["UpdatedAt"] = project.UpdatedAt

	event := ProjectPatchedEvent{
		ID:        project.ID,
		Changes:   changes,
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

// Helper for pointer deref
func derefString(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

func derefBool(b *bool) bool {
	if b != nil {
		return *b
	}
	return false
}
