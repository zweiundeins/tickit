package shared

import (
	"context"
	"time"

	"github.com/mbolli/tickit/internal/logger"
)

// CreateEventStoreHandler creates an event store handler that persists events
// This is the main infrastructure handler for event persistence
func CreateEventStoreHandler(eventStore *EventStore) EventHandler {
	return func(ctx context.Context, event Event) error {
		timestamp, ok := event.GetTimestamp().(time.Time)
		if !ok {
			timestamp = time.Now()
		}
		logger.Info().
			Str("event_type", event.GetEventType()).
			Time("timestamp", timestamp).
			Msg("persisting event to event store")
		return eventStore.StoreEvent(ctx, event)
	}
}
