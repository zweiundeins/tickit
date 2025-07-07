package events

import (
	"context"
	"log/slog"
	"time"

	"github.com/mbolli/tickit/internal/models"
)

// ExampleEventSourcing demonstrates full event sourcing capabilities
func ExampleEventSourcing() {
	ctx := context.Background()

	// 1. Initialize event store
	eventStore, err := NewEventStore(ctx, ":temp:")
	if err != nil {
		slog.Error("failed to create event store", slog.Any("error", err))
		return
	}
	defer eventStore.Close()

	// 2. Initialize read model builder
	readModelBuilder := NewReadModelBuilder(eventStore)

	// 3. Create some events
	sessionID := "user123"
	events := []Event{
		NotificationCreatedEvent{
			SessionID: sessionID,
			Notification: &models.Notification{
				Uid:   "notif-1",
				Title: "Welcome!",
				Body:  "Welcome to our platform",
				Date:  time.Now(),
				Read:  false,
			},
			Timestamp: time.Now(),
		},
		NotificationCreatedEvent{
			SessionID: sessionID,
			Notification: &models.Notification{
				Uid:   "notif-2",
				Title: "Important Update",
				Body:  "Please check your settings",
				Date:  time.Now(),
				Read:  false,
			},
			Timestamp: time.Now(),
		},
		NotificationReadEvent{
			SessionID:           sessionID,
			ReadNotificationUID: "notif-1",
			MarkedAllAsRead:     false,
			ReadNotificationIDs: []string{"notif-1"},
			PreviousUnreadCount: 2,
			NewUnreadCount:      1,
			Timestamp:           time.Now(),
		},
		NotificationDeletedEvent{
			SessionID:              sessionID,
			DeletedNotificationUID: "notif-2",
			DeletedAll:             false,
			DeletedNotificationIDs: []string{"notif-2"},
			Timestamp:              time.Now(),
		},
	}

	// 4. Store events
	for _, event := range events {
		if err := eventStore.StoreEvent(ctx, event); err != nil {
			slog.Error("failed to store event", slog.Any("error", err))
			return
		}
		slog.Info("stored event", slog.String("event_type", event.GetEventType()))
	}

	// 5. Rebuild read model from events
	notifications, err := readModelBuilder.BuildNotificationsFromEvents(ctx, sessionID)
	if err != nil {
		slog.Error("failed to build notifications from events", slog.Any("error", err))
		return
	}

	// 6. Display results
	slog.Info("rebuilt notifications from events",
		slog.String("session_id", sessionID),
		slog.Int("notification_count", len(notifications.Notifications)),
		slog.Int("unread_count", notifications.UnreadCount()),
	)

	for _, ntf := range notifications.Notifications {
		slog.Info("notification",
			slog.String("uid", ntf.Uid),
			slog.String("title", ntf.Title),
			slog.Bool("read", ntf.Read),
		)
	}
}

// ExampleEventReplay demonstrates event replay capabilities
func ExampleEventReplay() {
	ctx := context.Background()

	// Initialize components
	eventStore, err := NewEventStore(ctx, ":temp:")
	if err != nil {
		slog.Error("failed to create event store", slog.Any("error", err))
		return
	}
	defer eventStore.Close()

	readModelBuilder := NewReadModelBuilder(eventStore)

	// Get event count
	count, err := eventStore.GetEventCount(ctx)
	if err != nil {
		slog.Error("failed to get event count", slog.Any("error", err))
		return
	}

	slog.Info("total events in store", slog.Int64("count", count))

	// Replay all events since a specific time
	since := time.Now().Add(-24 * time.Hour) // Last 24 hours
	events, err := eventStore.GetEventsSince(ctx, since)
	if err != nil {
		slog.Error("failed to get events since", slog.Any("error", err))
		return
	}

	slog.Info("events since", slog.Time("since", since), slog.Int("count", len(events)))

	// Replay events by type
	createdEvents, err := eventStore.GetEventsByType(ctx, "NotificationCreated")
	if err != nil {
		slog.Error("failed to get created events", slog.Any("error", err))
		return
	}

	slog.Info("notification created events", slog.Int("count", len(createdEvents)))

	// Build read models by event type
	notificationsByType, err := readModelBuilder.BuildNotificationsByType(ctx, "NotificationCreated")
	if err != nil {
		slog.Error("failed to build notifications by type", slog.Any("error", err))
		return
	}

	for sessionID, notifications := range notificationsByType {
		slog.Info("notifications by type",
			slog.String("session_id", sessionID),
			slog.Int("count", len(notifications.Notifications)),
		)
	}
}

// ExampleFullRebuild demonstrates rebuilding all read models
func ExampleFullRebuild() {
	ctx := context.Background()

	// Initialize components
	eventStore, err := NewEventStore(ctx, ":temp:")
	if err != nil {
		slog.Error("failed to create event store", slog.Any("error", err))
		return
	}
	defer eventStore.Close()

	readModelBuilder := NewReadModelBuilder(eventStore)

	// Rebuild all read models
	if err := readModelBuilder.RebuildReadModels(ctx); err != nil {
		slog.Error("failed to rebuild read models", slog.Any("error", err))
		return
	}

	slog.Info("completed full read model rebuild")
}

// ExampleEventStoreQueries demonstrates various event store queries
func ExampleEventStoreQueries() {
	ctx := context.Background()

	// Initialize event store
	eventStore, err := NewEventStore(ctx, ":temp:")
	if err != nil {
		slog.Error("failed to create event store", slog.Any("error", err))
		return
	}
	defer eventStore.Close()

	// Get total event count
	count, err := eventStore.GetEventCount(ctx)
	if err != nil {
		slog.Error("failed to get event count", slog.Any("error", err))
		return
	}
	slog.Info("total events", slog.Int64("count", count))

	// Get events for a specific session
	sessionID := "user123"
	events, err := eventStore.GetEvents(ctx, sessionID)
	if err != nil {
		slog.Error("failed to get events for session", slog.Any("error", err))
		return
	}
	slog.Info("events for session", slog.String("session_id", sessionID), slog.Int("count", len(events)))

	// Get events by type
	createdEvents, err := eventStore.GetEventsByType(ctx, "NotificationCreated")
	if err != nil {
		slog.Error("failed to get created events", slog.Any("error", err))
		return
	}
	slog.Info("created events", slog.Int("count", len(createdEvents)))

	// Get events since a timestamp
	since := time.Now().Add(-1 * time.Hour)
	recentEvents, err := eventStore.GetEventsSince(ctx, since)
	if err != nil {
		slog.Error("failed to get recent events", slog.Any("error", err))
		return
	}
	slog.Info("recent events", slog.Time("since", since), slog.Int("count", len(recentEvents)))
}
