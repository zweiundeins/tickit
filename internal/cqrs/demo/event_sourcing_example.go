package demo

import (
	"context"
	"log/slog"
	"time"

	"github.com/mbolli/tickit/internal/notifications"
	"github.com/mbolli/tickit/internal/shared"
)

// ExampleEventSourcing demonstrates full event sourcing capabilities
func ExampleEventSourcing() {
	ctx := context.Background()

	// 1. Initialize event store
	eventStore, err := shared.NewEventStore(ctx, ":temp:")
	if err != nil {
		slog.Error("failed to create event store", slog.Any("error", err))
		return
	}
	defer eventStore.Close()

	// 2. Create some events
	sessionID := "user123"
	events := []shared.Event{
		notifications.NotificationCreatedEvent{
			SessionID: sessionID,
			Notification: &notifications.Notification{
				Uid:   "notif-1",
				Title: "Welcome!",
				Body:  "Welcome to our platform",
				Date:  time.Now(),
				Read:  false,
			},
			Timestamp: time.Now(),
		},
		notifications.NotificationCreatedEvent{
			SessionID: sessionID,
			Notification: &notifications.Notification{
				Uid:   "notif-2",
				Title: "Important Update",
				Body:  "Please check your settings",
				Date:  time.Now(),
				Read:  false,
			},
			Timestamp: time.Now(),
		},
		notifications.NotificationReadEvent{
			SessionID:           sessionID,
			ReadNotificationUID: "notif-1",
			MarkedAllAsRead:     false,
			ReadNotificationIDs: []string{"notif-1"},
			PreviousUnreadCount: 2,
			NewUnreadCount:      1,
			Timestamp:           time.Now(),
		},
		notifications.NotificationDeletedEvent{
			SessionID:              sessionID,
			DeletedNotificationUID: "notif-2",
			DeletedAll:             false,
			DeletedNotificationIDs: []string{"notif-2"},
			Timestamp:              time.Now(),
		},
	}

	// 3. Store events
	for _, event := range events {
		if err := eventStore.StoreEvent(ctx, event); err != nil {
			slog.Error("failed to store event", slog.Any("error", err))
			return
		}
		slog.Info("stored event", slog.String("event_type", event.GetEventType()))
	}

	// 4. Rebuild projections using NotificationProjectionStore
	projectionStore := notifications.NewNotificationProjectionStoreWithDB(eventStore.DB())
	if err := projectionStore.RebuildAllFromEvents(ctx, eventStore); err != nil {
		slog.Error("failed to rebuild notifications from events", slog.Any("error", err))
		return
	}

	// 5. Display results (fetch notifications as needed)
	// ...
}

// ExampleEventReplay demonstrates event replay capabilities
func ExampleEventReplay() {
	ctx := context.Background()

	// Initialize components
	eventStore, err := shared.NewEventStore(ctx, ":temp:")
	if err != nil {
		slog.Error("failed to create event store", slog.Any("error", err))
		return
	}
	defer eventStore.Close()

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
	// notificationsByType, err := readModelBuilder.BuildNotificationsByType(ctx, "NotificationCreated")
	// if err != nil {
	// 	slog.Error("failed to build notifications by type", slog.Any("error", err))
	// 	return
	// }

	// for sessionID, notifications := range notificationsByType {
	// 	slog.Info("notifications by type",
	// 		slog.String("session_id", sessionID),
	// 		slog.Int("count", len(notifications.Notifications)),
	// 	)
	// }
}

// ExampleFullRebuild demonstrates rebuilding all read models
func ExampleFullRebuild() {
	ctx := context.Background()

	// Initialize components
	eventStore, err := shared.NewEventStore(ctx, ":temp:")
	if err != nil {
		slog.Error("failed to create event store", slog.Any("error", err))
		return
	}
	defer eventStore.Close()

	// Rebuild all read models
	// if err := readModelBuilder.RebuildReadModels(ctx); err != nil {
	// 	slog.Error("failed to rebuild read models", slog.Any("error", err))
	// 	return
	// }

	slog.Info("completed full read model rebuild")
}

// ExampleEventStoreQueries demonstrates various event store queries
func ExampleEventStoreQueries() {
	ctx := context.Background()

	// Initialize event store
	eventStore, err := shared.NewEventStore(ctx, ":temp:")
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
