package notifications

import (
	"context"
	"log/slog"

	"github.com/mbolli/tickit/internal/shared"
)

// NotificationEventHandler handles notification-specific events
func NotificationEventHandler(ctx context.Context, event shared.Event) error {
	switch e := event.(type) {
	case NotificationCreatedEvent:
		slog.Info("notification created",
			slog.String("session_id", e.SessionID),
			slog.String("notification_uid", e.Notification.Uid),
			slog.String("title", e.Notification.Title),
		)
	case NotificationDeletedEvent:
		slog.Info("notification deleted",
			slog.String("session_id", e.SessionID),
			slog.String("deleted_notification_uid", e.DeletedNotificationUID),
			slog.Bool("deleted_all", e.DeletedAll),
			slog.Any("deleted_notification_ids", e.DeletedNotificationIDs),
		)
	case NotificationReadEvent:
		slog.Info("notification marked as read",
			slog.String("session_id", e.SessionID),
			slog.String("read_notification_uid", e.ReadNotificationUID),
			slog.Bool("marked_all_as_read", e.MarkedAllAsRead),
			slog.Any("read_notification_ids", e.ReadNotificationIDs),
			slog.Int("previous_unread_count", e.PreviousUnreadCount),
			slog.Int("new_unread_count", e.NewUnreadCount),
		)
	}
	return nil
}
func CreateDefaultEventHandlers() shared.EventHandlers {
	return shared.EventHandlers{
		"NotificationCreated": NotificationEventHandler,
		"NotificationDeleted": NotificationEventHandler,
		"NotificationRead":    NotificationEventHandler,
	}
}
