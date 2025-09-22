package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/delaneyj/toolbelt"
	"github.com/mbolli/tickit/internal/logger"
	"github.com/nats-io/nats.go/jetstream"
)

// Event is the interface for all domain events
// Each event must implement GetEventType and GetTimestamp
// (You may need to extend this interface as needed)
type Event interface {
	GetEventType() string
	GetTimestamp() interface{}
}

// EventBusWrapper wraps toolbelt's EventBus for domain events
type EventBusWrapper struct {
	eventBus  toolbelt.EventBus[Event]
	jetStream jetstream.JetStream // Injected JetStream client
}

// NewEventBusWrapper creates a new event bus wrapper
func NewEventBusWrapper(js jetstream.JetStream) *EventBusWrapper {
	return &EventBusWrapper{
		eventBus:  toolbelt.NewEventBusAsync[Event](),
		jetStream: js,
	}
}

// PublishEvent publishes an event to the NATS JetStream
func (eb *EventBusWrapper) PublishEvent(ctx context.Context, event Event) error {
	logger.Debug().
		Str("event_type", event.GetEventType()).
		Msg("EventBus.PublishEvent called")

	// Create a flattened event structure for UI consumption
	data := map[string]interface{}{
		"event_type": event.GetEventType(),
		"timestamp":  event.GetTimestamp(),
	}

	// Add entity-specific fields based on event type
	entityType := strings.ToLower(strings.TrimSuffix(event.GetEventType(), "Created"))
	entityType = strings.TrimSuffix(entityType, "Updated")
	entityType = strings.TrimSuffix(entityType, "Deleted")
	entityType = strings.TrimSuffix(entityType, "Patched")

	if entityType == "issue" {
		entityType = "issues" // Pluralize for UI consistency
	}

	data["entity_type"] = entityType

	// Extract entity ID and changes if available
	if entityIDProvider, ok := event.(interface{ GetIssueID() string }); ok {
		data["entity_id"] = entityIDProvider.GetIssueID()
	}

	if changesProvider, ok := event.(interface{ GetChanges() map[string]interface{} }); ok {
		data["data"] = changesProvider.GetChanges()
	}

	logger.Debug().Interface("data", data).Msg("Publishing event data")

	eventData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	_, err = eb.jetStream.Publish(ctx, "events", eventData)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to publish event to NATS")
		return fmt.Errorf("failed to publish event to NATS: %w", err)
	}

	logger.Debug().Msg("Successfully published event to NATS topic 'events'")
	return nil
} // SubscribeToEvents subscribes to all domain events
func (eb *EventBusWrapper) SubscribeToEvents(ctx context.Context, handler func(Event) error) context.CancelFunc {
	return eb.eventBus.Subscribe(ctx, handler)
}

// SubscribeToEventType subscribes to a specific event type
func (eb *EventBusWrapper) SubscribeToEventType(ctx context.Context, eventType string, handler func(Event) error) context.CancelFunc {
	return eb.eventBus.Subscribe(ctx, func(event Event) error {
		if event.GetEventType() == eventType {
			return handler(event)
		}
		return nil
	})
}

// GetSubscriberCount returns the number of active subscribers
func (eb *EventBusWrapper) GetSubscriberCount() int {
	return eb.eventBus.Count()
}

// EventHandler is a function that handles domain events
type EventHandler func(ctx context.Context, event Event) error

// EventHandlers maps event types to their handlers
type EventHandlers map[string]EventHandler

// EventBusWithHandlers extends EventBusWrapper with predefined handlers
type EventBusWithHandlers struct {
	*EventBusWrapper
	handlers EventHandlers
}

// NewEventBusWithHandlers creates a new event bus with predefined handlers
func NewEventBusWithHandlers(handlers EventHandlers, js jetstream.JetStream) *EventBusWithHandlers {
	eb := &EventBusWithHandlers{
		EventBusWrapper: NewEventBusWrapper(js),
		handlers:        handlers,
	}

	// Subscribe to all events and route to appropriate handlers
	eb.eventBus.Subscribe(context.Background(), func(event Event) error {
		handler, exists := eb.handlers[event.GetEventType()]
		if !exists {
			logger.Warn().
				Str("event_type", event.GetEventType()).
				Msg("no handler found for event type")
			return nil
		}

		ctx := context.Background()
		return handler(ctx, event)
	})

	return eb
}

// AddHandler adds a new event handler
func (eb *EventBusWithHandlers) AddHandler(eventType string, handler EventHandler) {
	eb.handlers[eventType] = handler
}

// RemoveHandler removes an event handler
func (eb *EventBusWithHandlers) RemoveHandler(eventType string) {
	delete(eb.handlers, eventType)
}
