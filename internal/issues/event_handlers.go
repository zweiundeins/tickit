package issues

import (
	"context"
	"log/slog"

	"github.com/mbolli/tickit/internal/shared"
)

// IssueEventHandler handles issue-specific events
func IssueEventHandler(ctx context.Context, event shared.Event) error {
	switch e := event.(type) {
	case IssueCreatedEvent:
		slog.Info("issue created",
			slog.String("issue_id", e.Issue.ID),
			slog.String("title", e.Issue.Title),
		)
	case IssueUpdatedEvent:
		slog.Info("issue updated",
			slog.String("issue_id", e.Issue.ID),
			slog.String("title", e.Issue.Title),
		)
	case IssueDeletedEvent:
		slog.Info("issue deleted",
			slog.String("issue_id", e.ID),
		)
	case IssueAssigneeChangedEvent:
		slog.Info("issue assignee changed",
			slog.String("issue_id", e.ID),
			slog.String("old_assignee", e.OldAssignee),
			slog.String("new_assignee", e.NewAssignee),
		)
	case IssueStatusChangedEvent:
		slog.Info("issue status changed",
			slog.String("issue_id", e.ID),
			slog.String("old_status", e.OldStatus),
			slog.String("new_status", e.NewStatus),
		)
	case IssueTitleChangedEvent:
		slog.Info("issue title changed",
			slog.String("issue_id", e.ID),
			slog.String("old_title", e.OldTitle),
			slog.String("new_title", e.NewTitle),
		)
	case IssueDescriptionChangedEvent:
		slog.Info("issue description changed",
			slog.String("issue_id", e.ID),
			slog.String("old_description", e.OldDescription),
			slog.String("new_description", e.NewDescription),
		)
	}
	return nil
}

// CreateDefaultEventHandlers creates a map of default event handlers
func CreateDefaultEventHandlers() shared.EventHandlers {
	return shared.EventHandlers{
		"IssueCreated":            IssueEventHandler,
		"IssueUpdated":            IssueEventHandler,
		"IssueDeleted":            IssueEventHandler,
		"IssueAssigneeChanged":    IssueEventHandler,
		"IssueStatusChanged":      IssueEventHandler,
		"IssueTitleChanged":       IssueEventHandler,
		"IssueDescriptionChanged": IssueEventHandler,
	}
}
