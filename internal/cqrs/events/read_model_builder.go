package events

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mbolli/tickit/internal/models"
)

// ReadModelBuilder rebuilds read models from events
type ReadModelBuilder struct {
	eventStore *EventStore
}

// NewReadModelBuilder creates a new read model builder
func NewReadModelBuilder(eventStore *EventStore) *ReadModelBuilder {
	return &ReadModelBuilder{
		eventStore: eventStore,
	}
}

// BuildNotificationsFromEvents rebuilds notifications from events for a session
func (rb *ReadModelBuilder) BuildNotificationsFromEvents(ctx context.Context, sessionID string) (*models.Notifications, error) {
	// Get all events for this session
	events, err := rb.eventStore.GetEvents(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	// Build notifications by replaying events
	notifications := &models.Notifications{
		Notifications: []*models.Notification{},
	}

	for _, event := range events {
		if err := rb.applyEventToNotifications(notifications, event); err != nil {
			return nil, fmt.Errorf("failed to apply event: %w", err)
		}
	}

	slog.Info("built notifications from events",
		slog.String("session_id", sessionID),
		slog.Int("event_count", len(events)),
		slog.Int("notification_count", len(notifications.Notifications)),
	)

	return notifications, nil
}

// BuildAllNotificationsFromEvents rebuilds all notifications from all events
func (rb *ReadModelBuilder) BuildAllNotificationsFromEvents(ctx context.Context) (map[string]*models.Notifications, error) {
	// Get all events since the beginning of time
	events, err := rb.eventStore.GetEventsSince(ctx, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	// Group events by session ID
	sessionEvents := make(map[string][]Event)
	for _, event := range events {
		sessionID := rb.getSessionIDFromEvent(event)
		sessionEvents[sessionID] = append(sessionEvents[sessionID], event)
	}

	// Build notifications for each session
	allNotifications := make(map[string]*models.Notifications)
	for sessionID, sessionEventList := range sessionEvents {
		notifications := &models.Notifications{
			Notifications: []*models.Notification{},
		}

		for _, event := range sessionEventList {
			if err := rb.applyEventToNotifications(notifications, event); err != nil {
				return nil, fmt.Errorf("failed to apply event for session %s: %w", sessionID, err)
			}
		}

		allNotifications[sessionID] = notifications
	}

	slog.Info("built all notifications from events",
		slog.Int("total_events", len(events)),
		slog.Int("session_count", len(allNotifications)),
	)

	return allNotifications, nil
}

// BuildNotificationsByType rebuilds notifications for a specific event type
func (rb *ReadModelBuilder) BuildNotificationsByType(ctx context.Context, eventType string) (map[string]*models.Notifications, error) {
	// Get all events of the specified type
	events, err := rb.eventStore.GetEventsByType(ctx, eventType)
	if err != nil {
		return nil, fmt.Errorf("failed to get events by type: %w", err)
	}

	// Group events by session ID
	sessionEvents := make(map[string][]Event)
	for _, event := range events {
		sessionID := rb.getSessionIDFromEvent(event)
		sessionEvents[sessionID] = append(sessionEvents[sessionID], event)
	}

	// Build notifications for each session
	allNotifications := make(map[string]*models.Notifications)
	for sessionID, sessionEventList := range sessionEvents {
		notifications := &models.Notifications{
			Notifications: []*models.Notification{},
		}

		for _, event := range sessionEventList {
			if err := rb.applyEventToNotifications(notifications, event); err != nil {
				return nil, fmt.Errorf("failed to apply event for session %s: %w", sessionID, err)
			}
		}

		allNotifications[sessionID] = notifications
	}

	slog.Info("built notifications by event type",
		slog.String("event_type", eventType),
		slog.Int("event_count", len(events)),
		slog.Int("session_count", len(allNotifications)),
	)

	return allNotifications, nil
}

// RebuildReadModels rebuilds all read models from events
func (rb *ReadModelBuilder) RebuildReadModels(ctx context.Context) error {
	slog.Info("starting read model rebuild from events")

	// Get event count for progress tracking
	totalEvents, err := rb.eventStore.GetEventCount(ctx)
	if err != nil {
		return fmt.Errorf("failed to get event count: %w", err)
	}

	slog.Info("rebuilding read models",
		slog.Int64("total_events", totalEvents),
	)

	// Build all notifications
	allNotifications, err := rb.BuildAllNotificationsFromEvents(ctx)
	if err != nil {
		return fmt.Errorf("failed to build notifications: %w", err)
	}

	// TODO: Here you would typically update your read model storage
	// For now, we just log the results
	for sessionID, notifications := range allNotifications {
		slog.Info("rebuilt notifications for session",
			slog.String("session_id", sessionID),
			slog.Int("notification_count", len(notifications.Notifications)),
			slog.Int("unread_count", notifications.UnreadCount()),
		)
	}

	slog.Info("completed read model rebuild",
		slog.Int("session_count", len(allNotifications)),
	)

	return nil
}

// Helper methods

func (rb *ReadModelBuilder) applyEventToNotifications(notifications *models.Notifications, event Event) error {
	switch e := event.(type) {
	case NotificationCreatedEvent:
		// Add the notification that was created
		notifications.Notifications = append(notifications.Notifications, e.Notification)

	case NotificationDeletedEvent:
		if e.DeletedAll {
			// All notifications were deleted
			notifications.Notifications = []*models.Notification{}
		} else {
			// Remove the specific notifications that were deleted
			for _, deletedID := range e.DeletedNotificationIDs {
				for i, ntf := range notifications.Notifications {
					if ntf.Uid == deletedID {
						notifications.Notifications = append(notifications.Notifications[:i], notifications.Notifications[i+1:]...)
						break
					}
				}
			}
		}

	case NotificationReadEvent:
		// Mark the specific notifications as read based on what actually happened
		for _, readID := range e.ReadNotificationIDs {
			for _, ntf := range notifications.Notifications {
				if ntf.Uid == readID {
					ntf.Read = true
					break
				}
			}
		}

	default:
		return fmt.Errorf("unknown event type: %s", event.GetEventType())
	}

	return nil
}

func (rb *ReadModelBuilder) getSessionIDFromEvent(event Event) string {
	switch e := event.(type) {
	case NotificationCreatedEvent:
		return e.SessionID
	case NotificationDeletedEvent:
		return e.SessionID
	case NotificationReadEvent:
		return e.SessionID
	default:
		return "unknown"
	}
}
