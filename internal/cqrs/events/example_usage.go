package events

import (
	"context"
	"log/slog"
	"time"

	"github.com/mbolli/tickit/internal/models"
)

// ExampleEventBusUsage demonstrates how to use the event bus
func ExampleEventBusUsage() {
	// Create a simple event bus
	eventBus := NewEventBusWrapper(nil)

	// Subscribe to all events
	cancel := eventBus.SubscribeToEvents(context.Background(), func(event Event) error {
		slog.Info("received event", slog.String("type", event.GetEventType()))
		return nil
	})
	defer cancel()

	// Subscribe to specific event type
	cancelCreated := eventBus.SubscribeToEventType(context.Background(), "NotificationCreated", func(event Event) error {
		if e, ok := event.(NotificationCreatedEvent); ok {
			slog.Info("notification created", slog.String("title", e.Notification.Title))
		}
		return nil
	})
	defer cancelCreated()

	// Publish an event
	event := NotificationCreatedEvent{
		SessionID: "user123",
		Notification: &models.Notification{
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
	handlers := EventHandlers{
		"NotificationCreated": func(ctx context.Context, event Event) error {
			slog.Info("custom handler for notification created")
			return nil
		},
		"NotificationDeleted": func(ctx context.Context, event Event) error {
			slog.Info("custom handler for notification deleted")
			return nil
		},
	}

	// Create event bus with handlers
	eventBus := NewEventBusWithHandlers(handlers, nil)

	// Add additional handler
	eventBus.AddHandler("NotificationRead", func(ctx context.Context, event Event) error {
		slog.Info("notification read handler")
		return nil
	})

	// Publish events
	ctx := context.Background()

	createdEvent := NotificationCreatedEvent{
		SessionID: "user123",
		Notification: &models.Notification{
			Uid:   "notif-123",
			Title: "Test",
		},
		Timestamp: time.Now(),
	}

	if err := eventBus.PublishEvent(ctx, createdEvent); err != nil {
		slog.Error("failed to publish created event", slog.Any("error", err))
	}

	deletedEvent := NotificationDeletedEvent{
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
	eventBus := NewEventBusWrapper(nil)

	// Handler 1: Logging
	eventBus.SubscribeToEvents(context.Background(), func(event Event) error {
		return LoggingEventHandler(context.Background(), event)
	})

	// Handler 2: Metrics
	eventBus.SubscribeToEvents(context.Background(), func(event Event) error {
		return MetricsEventHandler(context.Background(), event)
	})

	// Handler 3: Audit
	eventBus.SubscribeToEvents(context.Background(), func(event Event) error {
		return AuditEventHandler(context.Background(), event)
	})

	// Handler 4: Email notifications
	eventBus.SubscribeToEventType(context.Background(), "NotificationCreated", func(event Event) error {
		return EmailEventHandler(context.Background(), event)
	})

	// Handler 5: Webhooks
	eventBus.SubscribeToEventType(context.Background(), "NotificationCreated", func(event Event) error {
		return WebhookEventHandler(context.Background(), event)
	})

	// Publish event - all handlers will be called
	event := NotificationCreatedEvent{
		SessionID: "user123",
		Notification: &models.Notification{
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
