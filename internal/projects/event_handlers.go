package projects

import (
	"context"
	"log/slog"

	"github.com/mbolli/tickit/internal/shared"
)

// ProjectEventHandler handles project-specific events
func ProjectEventHandler(ctx context.Context, event shared.Event) error {
	switch e := event.(type) {
	case ProjectCreatedEvent:
		slog.Info("project created",
			slog.String("project_id", e.Project.ID),
			slog.String("name", e.Project.Name),
		)
	case ProjectUpdatedEvent:
		slog.Info("project updated",
			slog.String("project_id", e.Project.ID),
			slog.String("name", e.Project.Name),
		)
	case ProjectDeletedEvent:
		slog.Info("project deleted",
			slog.String("project_id", e.ID),
		)
	}
	return nil
}

// CreateDefaultProjectEventHandlers creates a map of default event handlers for projects
func CreateDefaultProjectEventHandlers() shared.EventHandlers {
	return shared.EventHandlers{
		"ProjectCreated": ProjectEventHandler,
		"ProjectUpdated": ProjectEventHandler,
		"ProjectDeleted": ProjectEventHandler,
	}
}
