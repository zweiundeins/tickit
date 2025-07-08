package demo

import (
	"context"
	"log/slog"
	"time"

	"github.com/mbolli/tickit/internal/notifications"
	"github.com/mbolli/tickit/internal/shared"
)

// ExampleEventBusUsage demonstrates how to use the event bus
func ExampleEventBusUsage() {
	// Create a simple event bus
	eventBus := shared.NewEventBusWrapper(nil)

	// Subscribe to all events
	cancel := eventBus.SubscribeToEvents(context.Background(), func(event shared.Event) error {
		slog.Info("received event", slog.String("type", event.GetEventType()))
		return nil
	})
	defer cancel()

	// Subscribe to specific event type
	cancelCreated := eventBus.SubscribeToEventType(context.Background(), "NotificationCreated", func(event shared.Event) error {
		if e, ok := event.(notifications.NotificationCreatedEvent); ok {
			slog.Info("notification created", slog.String("title", e.Notification.Title))
		}
		return nil
	})
	defer cancelCreated()

	// Publish an event
	event := notifications.NotificationCreatedEvent{
		SessionID: "user123",
		Notification: &notifications.Notification{
			Uid:   "notif-123",
			Title: "Test Notification",
			Body:  "This is a test",
		},
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	if err := eventBus.PublishEvent(ctx, event); err != nil {
		slog.Error("failed to publish event", slog.Any("error", err))
	}
}

// ExampleEventBusWithHandlers demonstrates using the event bus with predefined handlers
func ExampleEventBusWithHandlers() {
	// Create custom handlers
	handlers := shared.EventHandlers{
		"NotificationCreated": func(ctx context.Context, event shared.Event) error {
			slog.Info("custom handler for notification created")
			return nil
		},
		"NotificationDeleted": func(ctx context.Context, event shared.Event) error {
			slog.Info("custom handler for notification deleted")
			return nil
		},
	}

	// Create event bus with handlers
	eventBus := shared.NewEventBusWithHandlers(handlers, nil)

	// Add additional handler
	eventBus.AddHandler("NotificationRead", func(ctx context.Context, event shared.Event) error {
		slog.Info("notification read handler")
		return nil
	})

	// Publish events
	ctx := context.Background()

	createdEvent := notifications.NotificationCreatedEvent{
		SessionID: "user123",
		Notification: &notifications.Notification{
			Uid:   "notif-123",
			Title: "Test",
		},
		Timestamp: time.Now(),
	}

	if err := eventBus.PublishEvent(ctx, createdEvent); err != nil {
		slog.Error("failed to publish created event", slog.Any("error", err))
	}

	deletedEvent := notifications.NotificationDeletedEvent{
		SessionID:              "user123",
		DeletedNotificationUID: "notif-123",
		DeletedAll:             false,
		DeletedNotificationIDs: []string{"notif-123"},
		Timestamp:              time.Now(),
	}

	if err := eventBus.PublishEvent(ctx, deletedEvent); err != nil {
		slog.Error("failed to publish deleted event", slog.Any("error", err))
	}
}

// ExampleMultipleEventHandlers demonstrates using multiple handlers for the same event
func ExampleMultipleEventHandlers() {
	eventBus := shared.NewEventBusWrapper(nil)

	// Handler 1: Logging
	eventBus.SubscribeToEvents(context.Background(), func(event shared.Event) error {
		return shared.LoggingEventHandler(context.Background(), event)
	})

	// Handler 2: Metrics
	eventBus.SubscribeToEvents(context.Background(), func(event shared.Event) error {
		return shared.MetricsEventHandler(context.Background(), event)
	})

	// Handler 3: Audit
	eventBus.SubscribeToEvents(context.Background(), func(event shared.Event) error {
		return shared.AuditEventHandler(context.Background(), event)
	})

	// Handler 4: Email notifications
	eventBus.SubscribeToEventType(context.Background(), "NotificationCreated", func(event shared.Event) error {
		return shared.EmailEventHandler(context.Background(), event)
	})

	// Handler 5: Webhooks
	eventBus.SubscribeToEventType(context.Background(), "NotificationCreated", func(event shared.Event) error {
		return shared.WebhookEventHandler(context.Background(), event)
	})

	// Publish event - all handlers will be called
	event := notifications.NotificationCreatedEvent{
		SessionID: "user123",
		Notification: &notifications.Notification{
			Uid:   "notif-123",
			Title: "Important Notification",
		},
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	if err := eventBus.PublishEvent(ctx, event); err != nil {
		slog.Error("failed to publish event", slog.Any("error", err))
	}
}
