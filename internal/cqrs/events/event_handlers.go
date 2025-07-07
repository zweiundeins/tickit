package events

import (
	"context"
	"log/slog"
)

// EventHandlers provides various handlers for domain events

// LoggingEventHandler logs all domain events
func LoggingEventHandler(ctx context.Context, event Event) error {
	slog.Info("domain event occurred",
		slog.String("event_type", event.GetEventType()),
		slog.Time("timestamp", event.GetTimestamp()),
	)
	return nil
}

// MetricsEventHandler tracks metrics for domain events
func MetricsEventHandler(ctx context.Context, event Event) error {
	// TODO: Implement metrics collection
	// Example: increment counters, record histograms, etc.
	slog.Debug("metrics event handler called",
		slog.String("event_type", event.GetEventType()),
	)
	return nil
}

// AuditEventHandler logs events for audit purposes
func AuditEventHandler(ctx context.Context, event Event) error {
	slog.Info("audit event",
		slog.String("event_type", event.GetEventType()),
		slog.Time("timestamp", event.GetTimestamp()),
		slog.String("audit_level", "INFO"),
	)
	return nil
}

// NotificationEventHandler handles notification-specific events
func NotificationEventHandler(ctx context.Context, event Event) error {
	switch e := event.(type) {
	case NotificationCreatedEvent:
		slog.Info("notification created",
			slog.String("session_id", e.SessionID),
			slog.String("notification_uid", e.Notification.Uid),
			slog.String("title", e.Notification.Title),
		)
	case NotificationDeletedEvent:
		slog.Info("notification deleted",
			slog.String("session_id", e.SessionID),
			slog.String("deleted_notification_uid", e.DeletedNotificationUID),
			slog.Bool("deleted_all", e.DeletedAll),
			slog.Any("deleted_notification_ids", e.DeletedNotificationIDs),
		)
	case NotificationReadEvent:
		slog.Info("notification marked as read",
			slog.String("session_id", e.SessionID),
			slog.String("read_notification_uid", e.ReadNotificationUID),
			slog.Bool("marked_all_as_read", e.MarkedAllAsRead),
			slog.Any("read_notification_ids", e.ReadNotificationIDs),
			slog.Int("previous_unread_count", e.PreviousUnreadCount),
			slog.Int("new_unread_count", e.NewUnreadCount),
		)
	}
	return nil
}

// EmailEventHandler could send emails for certain events
func EmailEventHandler(ctx context.Context, event Event) error {
	// TODO: Implement email notifications
	// Example: Send welcome email when user creates first notification
	slog.Debug("email event handler called",
		slog.String("event_type", event.GetEventType()),
	)
	return nil
}

// WebhookEventHandler could send webhooks to external systems
func WebhookEventHandler(ctx context.Context, event Event) error {
	// TODO: Implement webhook notifications
	// Example: Notify external systems about notification events
	slog.Debug("webhook event handler called",
		slog.String("event_type", event.GetEventType()),
	)
	return nil
}

// EventStoreHandler persists events to an event store
func EventStoreHandler(ctx context.Context, event Event) error {
	// This will be set by the application when initializing the event bus
	slog.Debug("event store handler called",
		slog.String("event_type", event.GetEventType()),
	)
	return nil
}

// CreateEventStoreHandler creates an event store handler that persists events
func CreateEventStoreHandler(eventStore *EventStore) EventHandler {
	return func(ctx context.Context, event Event) error {
		slog.Info("persisting event to event store",
			slog.String("event_type", event.GetEventType()),
			slog.Time("timestamp", event.GetTimestamp()),
		)
		return eventStore.StoreEvent(ctx, event)
	}
}

// IssueEventHandler handles issue-specific events
func IssueEventHandler(ctx context.Context, event Event) error {
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
func CreateDefaultEventHandlers() EventHandlers {
	return EventHandlers{
		"NotificationCreated":     NotificationEventHandler,
		"NotificationDeleted":     NotificationEventHandler,
		"NotificationRead":        NotificationEventHandler,
		"IssueCreated":            IssueEventHandler,
		"IssueUpdated":            IssueEventHandler,
		"IssueDeleted":            IssueEventHandler,
		"IssueAssigneeChanged":    IssueEventHandler,
		"IssueStatusChanged":      IssueEventHandler,
		"IssueTitleChanged":       IssueEventHandler,
		"IssueDescriptionChanged": IssueEventHandler,
	}
}

// CreateAllEventHandlers creates handlers that process all events
func CreateAllEventHandlers() EventHandlers {
	return EventHandlers{
		"*": LoggingEventHandler, // Wildcard handler for all events
	}
}
