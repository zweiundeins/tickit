package shared

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/delaneyj/toolbelt"
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

// PublishEvent publishes a domain event (to local bus and JetStream)
func (eb *EventBusWrapper) PublishEvent(ctx context.Context, event Event) error {
	slog.Info("publishing domain event",
		slog.String("event_type", event.GetEventType()),
	)
	// Publish to local event bus
	err := eb.eventBus.Emit(ctx, event)
	if err != nil {
		slog.Error("[ERROR] Failed to emit event to local event bus", slog.String("event_type", event.GetEventType()), slog.Any("error", err))
		return err
	}
	// Publish to JetStream unified stream with envelope
	type eventEnvelope struct {
		EventType string      `json:"event_type"`
		Payload   interface{} `json:"payload"`
	}
	env := eventEnvelope{
		EventType: event.GetEventType(),
		Payload:   event,
	}
	b, err := json.Marshal(env)
	if err != nil {
		slog.Error("[ERROR] Failed to marshal event envelope for JetStream", slog.String("event_type", event.GetEventType()), slog.Any("error", err))
		return err
	}
	if eb.jetStream != nil {
		_, err = eb.jetStream.Publish(ctx, "events", b)
		if err != nil {
			slog.Error("[ERROR] Failed to publish event to JetStream", slog.String("event_type", event.GetEventType()), slog.Any("error", err))
		}
		return err
	}
	return nil
}

// SubscribeToEvents subscribes to all domain events
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
			slog.Warn("no handler found for event type",
				slog.String("event_type", event.GetEventType()),
			)
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
