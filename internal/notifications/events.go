package notifications

import (
	"encoding/json"
	"time"

	"github.com/mbolli/tickit/internal/shared"
)

// Domain events for notifications

type NotificationCreatedEvent struct {
	SessionID    string        `json:"session_id"`
	UserID       string        `json:"user_id,omitempty"`    // Optional user ID for user-specific notifications
	Notification *Notification `json:"notification"`         // Contains the complete created notification
	Timestamp    time.Time     `json:"timestamp"`
}

func (e NotificationCreatedEvent) GetEventType() string      { return "NotificationCreated" }
func (e NotificationCreatedEvent) GetTimestamp() interface{} { return e.Timestamp }

var _ shared.Event = (*NotificationCreatedEvent)(nil)

type NotificationDeletedEvent struct {
	SessionID              string    `json:"session_id"`
	UserID                 string    `json:"user_id,omitempty"`                  // Optional user ID
	DeletedNotificationUID string    `json:"deleted_notification_uid,omitempty"` // Specific notification deleted
	DeletedAll             bool      `json:"deleted_all"`                        // Whether all notifications were deleted
	DeletedNotificationIDs []string  `json:"deleted_notification_ids"`           // All notification IDs that were deleted
	Timestamp              time.Time `json:"timestamp"`
}

func (e NotificationDeletedEvent) GetEventType() string      { return "NotificationDeleted" }
func (e NotificationDeletedEvent) GetTimestamp() interface{} { return e.Timestamp }

var _ shared.Event = (*NotificationDeletedEvent)(nil)

type NotificationReadEvent struct {
	SessionID           string    `json:"session_id"`
	UserID              string    `json:"user_id,omitempty"`               // Optional user ID
	ReadNotificationUID string    `json:"read_notification_uid,omitempty"` // Specific notification marked as read
	MarkedAllAsRead     bool      `json:"marked_all_as_read"`              // Whether all notifications were marked as read
	ReadNotificationIDs []string  `json:"read_notification_ids"`           // All notification IDs that were marked as read
	PreviousUnreadCount int       `json:"previous_unread_count"`           // Count before marking as read
	NewUnreadCount      int       `json:"new_unread_count"`                // Count after marking as read
	Timestamp           time.Time `json:"timestamp"`
}

func (e NotificationReadEvent) GetEventType() string      { return "NotificationRead" }
func (e NotificationReadEvent) GetTimestamp() interface{} { return e.Timestamp }

var _ shared.Event = (*NotificationReadEvent)(nil)

func init() {
	shared.RegisterEventDeserializer("NotificationCreated", func(eventData string, timestamp time.Time) (shared.Event, error) {
		var e NotificationCreatedEvent
		if err := json.Unmarshal([]byte(eventData), &e); err != nil {
			return nil, err
		}
		return e, nil
	})
	shared.RegisterEventDeserializer("NotificationDeleted", func(eventData string, timestamp time.Time) (shared.Event, error) {
		var e NotificationDeletedEvent
		if err := json.Unmarshal([]byte(eventData), &e); err != nil {
			return nil, err
		}
		return e, nil
	})
	shared.RegisterEventDeserializer("NotificationRead", func(eventData string, timestamp time.Time) (shared.Event, error) {
		var e NotificationReadEvent
		if err := json.Unmarshal([]byte(eventData), &e); err != nil {
			return nil, err
		}
		return e, nil
	})
}
