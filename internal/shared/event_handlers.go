package shared

import (
	"context"
	"log/slog"
	"time"
)

// EventHandlers provides various handlers for domain events

// LoggingEventHandler logs all domain events
func LoggingEventHandler(ctx context.Context, event Event) error {
	timestamp, ok := event.GetTimestamp().(time.Time)
	if !ok {
		timestamp = time.Now()
	}
	slog.Info("domain event occurred",
		slog.String("event_type", event.GetEventType()),
		slog.Time("timestamp", timestamp),
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
	timestamp, ok := event.GetTimestamp().(time.Time)
	if !ok {
		timestamp = time.Now()
	}
	slog.Info("audit event",
		slog.String("event_type", event.GetEventType()),
		slog.Time("timestamp", timestamp),
		slog.String("audit_level", "INFO"),
	)
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
		timestamp, ok := event.GetTimestamp().(time.Time)
		if !ok {
			timestamp = time.Now()
		}
		slog.Info("persisting event to event store",
			slog.String("event_type", event.GetEventType()),
			slog.Time("timestamp", timestamp),
		)
		return eventStore.StoreEvent(ctx, event)
	}
}

// CreateDefaultEventHandlers creates a map of default event handlers
func CreateDefaultEventHandlers() EventHandlers {
	return EventHandlers{}
}

// CreateAllEventHandlers creates handlers that process all events
func CreateAllEventHandlers() EventHandlers {
	return EventHandlers{
		"*": LoggingEventHandler, // Wildcard handler for all events
	}
}
