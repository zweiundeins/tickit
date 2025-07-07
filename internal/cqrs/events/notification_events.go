package events

import (
	"time"

	"github.com/mbolli/tickit/internal/models"
)

// Domain events for notifications

type NotificationCreatedEvent struct {
	SessionID    string               `json:"session_id"`
	Notification *models.Notification `json:"notification"` // Contains the complete created notification
	Timestamp    time.Time            `json:"timestamp"`
}

type NotificationDeletedEvent struct {
	SessionID              string    `json:"session_id"`
	DeletedNotificationUID string    `json:"deleted_notification_uid,omitempty"` // Specific notification deleted
	DeletedAll             bool      `json:"deleted_all"`                        // Whether all notifications were deleted
	DeletedNotificationIDs []string  `json:"deleted_notification_ids"`           // All notification IDs that were deleted
	Timestamp              time.Time `json:"timestamp"`
}

type NotificationReadEvent struct {
	SessionID           string    `json:"session_id"`
	ReadNotificationUID string    `json:"read_notification_uid,omitempty"` // Specific notification marked as read
	MarkedAllAsRead     bool      `json:"marked_all_as_read"`              // Whether all notifications were marked as read
	ReadNotificationIDs []string  `json:"read_notification_ids"`           // All notification IDs that were marked as read
	PreviousUnreadCount int       `json:"previous_unread_count"`           // Count before marking as read
	NewUnreadCount      int       `json:"new_unread_count"`                // Count after marking as read
	Timestamp           time.Time `json:"timestamp"`
}

// Event interface for future event sourcing
type Event interface {
	GetEventType() string
	GetTimestamp() time.Time
}

func (e NotificationCreatedEvent) GetEventType() string {
	return "NotificationCreated"
}

func (e NotificationCreatedEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

func (e NotificationDeletedEvent) GetEventType() string {
	return "NotificationDeleted"
}

func (e NotificationDeletedEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

func (e NotificationReadEvent) GetEventType() string {
	return "NotificationRead"
}

func (e NotificationReadEvent) GetTimestamp() time.Time {
	return e.Timestamp
}
