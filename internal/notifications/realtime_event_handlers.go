package notifications

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/internal/shared"
	"github.com/nats-io/nats.go"
)

// UIUpdateEvent represents a UI-specific event for real-time updates
// This is a local copy to avoid import cycles with realtime package
type UIUpdateEvent struct {
	EventType  string                 `json:"event_type"`  // "notifications.created", etc.
	EntityType string                 `json:"entity_type"` // "notifications"
	EntityID   string                 `json:"entity_id"`   // Session ID for notifications
	Timestamp  time.Time              `json:"timestamp"`
	Data       map[string]interface{} `json:"data"` // Event-specific data
}

// RealtimeEventHandler converts notification domain events to UI events for the realtime system
type RealtimeEventHandler struct {
	natsConn *nats.Conn
}

// NewRealtimeEventHandler creates a new realtime event handler for notifications
func NewRealtimeEventHandler(natsConn *nats.Conn) *RealtimeEventHandler {
	return &RealtimeEventHandler{
		natsConn: natsConn,
	}
}

// Handle processes notification domain events and converts them to UI events
func (h *RealtimeEventHandler) Handle(ctx context.Context, event shared.Event) error {
	switch e := event.(type) {
	case NotificationCreatedEvent:
		return h.handleNotificationCreated(ctx, e)
	case NotificationDeletedEvent:
		return h.handleNotificationDeleted(ctx, e)
	case NotificationReadEvent:
		return h.handleNotificationRead(ctx, e)
	}
	return nil
}

func (h *RealtimeEventHandler) handleNotificationCreated(ctx context.Context, event NotificationCreatedEvent) error {
	logger.Info().
		Str("user_id", event.UserID).
		Str("notification_title", event.Notification.Title).
		Msg("🔔 Handling NotificationCreated event for realtime updates")

	uiEvent := UIUpdateEvent{
		EventType:  "notifications.created",
		EntityType: "notifications",
		EntityID:   event.UserID, // Use user ID as entity ID for notifications
		Timestamp:  event.Timestamp,
		Data: map[string]interface{}{
			"notification": event.Notification,
			"user_id":      event.UserID,
		},
	}

	return h.publishUIEvent(ctx, uiEvent)
}

func (h *RealtimeEventHandler) handleNotificationDeleted(ctx context.Context, event NotificationDeletedEvent) error {
	uiEvent := UIUpdateEvent{
		EventType:  "notifications.deleted",
		EntityType: "notifications",
		EntityID:   event.UserID,
		Timestamp:  event.Timestamp,
		Data: map[string]interface{}{
			"user_id":                  event.UserID,
			"deleted_notification_uid": event.DeletedNotificationUID,
			"deleted_all":              event.DeletedAll,
			"deleted_notification_ids": event.DeletedNotificationIDs,
		},
	}

	return h.publishUIEvent(ctx, uiEvent)
}

func (h *RealtimeEventHandler) handleNotificationRead(ctx context.Context, event NotificationReadEvent) error {
	uiEvent := UIUpdateEvent{
		EventType:  "notifications.read",
		EntityType: "notifications",
		EntityID:   event.UserID,
		Timestamp:  event.Timestamp,
		Data: map[string]interface{}{
			"user_id":               event.UserID,
			"read_notification_uid": event.ReadNotificationUID,
			"marked_all_as_read":    event.MarkedAllAsRead,
			"read_notification_ids": event.ReadNotificationIDs,
			"previous_unread_count": event.PreviousUnreadCount,
			"new_unread_count":      event.NewUnreadCount,
		},
	}

	return h.publishUIEvent(ctx, uiEvent)
}

func (h *RealtimeEventHandler) publishUIEvent(ctx context.Context, uiEvent UIUpdateEvent) error {
	data, err := json.Marshal(uiEvent)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to marshal UI event")
		return err
	}

	err = h.natsConn.Publish("events", data)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to publish UI event to NATS")
		return err
	}

	logger.Debug().
		Str("event_type", uiEvent.EventType).
		Str("entity_type", uiEvent.EntityType).
		Str("entity_id", uiEvent.EntityID).
		Msg("Published notification UI event to NATS")

	return nil
}
