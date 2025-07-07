package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mbolli/tickit/internal/cqrs/events"
	"github.com/mbolli/tickit/internal/models"
)

// Command handlers for notification write operations

type CreateNotificationCommand struct {
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
}

type DeleteNotificationCommand struct {
	SessionID       string `json:"session_id"`
	NotificationUID string `json:"notification_uid,omitempty"` // If empty, delete all
}

type MarkNotificationReadCommand struct {
	SessionID       string `json:"session_id"`
	NotificationUID string `json:"notification_uid,omitempty"` // If empty, mark all as read
}

// CommandHandler handles all write operations for notifications
type CommandHandler struct {
	eventStore      *events.EventStore
	projectionStore *events.NotificationProjectionStore
	eventBus        *events.EventBusWrapper
}

// NewCommandHandler creates a new command handler
func NewCommandHandler(eventStore *events.EventStore, projectionStore *events.NotificationProjectionStore, eventBus *events.EventBusWrapper) *CommandHandler {
	return &CommandHandler{
		eventStore:      eventStore,
		projectionStore: projectionStore,
		eventBus:        eventBus,
	}
}

// HandleCreateNotification creates a new notification
func (h *CommandHandler) HandleCreateNotification(ctx context.Context, cmd CreateNotificationCommand) error {
	notification := &models.Notification{
		Uid:   uuid.New().String(),
		Date:  time.Now(),
		Title: cmd.Title,
		Body:  cmd.Body,
		Read:  false,
	}
	event := events.NotificationCreatedEvent{
		SessionID:    cmd.SessionID,
		Notification: notification,
		Timestamp:    time.Now(),
	}
	// 1. Persist event to SQLite event store
	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to store event: %w", err)
	}
	// 2. Update projection
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to update projection: %w", err)
	}
	// 3. Publish signal to JetStream
	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}
	return nil
}

// HandleDeleteNotification deletes notification(s)
func (h *CommandHandler) HandleDeleteNotification(ctx context.Context, cmd DeleteNotificationCommand) error {
	// Compose event
	deletedAll := cmd.NotificationUID == ""
	// For event, we need to know which IDs will be deleted
	notifs, err := h.projectionStore.GetNotifications(ctx, cmd.SessionID)
	if err != nil {
		return fmt.Errorf("failed to get notifications: %w", err)
	}
	var deletedNotificationIDs []string
	if deletedAll {
		for _, ntf := range notifs.Notifications {
			deletedNotificationIDs = append(deletedNotificationIDs, ntf.Uid)
		}
	} else {
		for _, ntf := range notifs.Notifications {
			if ntf.Uid == cmd.NotificationUID {
				deletedNotificationIDs = append(deletedNotificationIDs, ntf.Uid)
				break
			}
		}
	}
	event := events.NotificationDeletedEvent{
		SessionID:              cmd.SessionID,
		DeletedNotificationUID: cmd.NotificationUID,
		DeletedAll:             deletedAll,
		DeletedNotificationIDs: deletedNotificationIDs,
		Timestamp:              time.Now(),
	}
	// 1. Persist event to SQLite event store
	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to store event: %w", err)
	}
	// 2. Update projection
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to update projection: %w", err)
	}
	// 3. Publish signal to JetStream
	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}
	return nil
}

// HandleMarkNotificationRead marks notification(s) as read
func (h *CommandHandler) HandleMarkNotificationRead(ctx context.Context, cmd MarkNotificationReadCommand) error {
	notifs, err := h.projectionStore.GetNotifications(ctx, cmd.SessionID)
	if err != nil {
		return fmt.Errorf("failed to get notifications: %w", err)
	}
	var readNotificationIDs []string
	markedAllAsRead := cmd.NotificationUID == ""
	previousUnreadCount := notifs.UnreadCount()
	if markedAllAsRead {
		for _, ntf := range notifs.Notifications {
			if !ntf.Read {
				readNotificationIDs = append(readNotificationIDs, ntf.Uid)
			}
		}
	} else {
		for _, ntf := range notifs.Notifications {
			if ntf.Uid == cmd.NotificationUID && !ntf.Read {
				readNotificationIDs = append(readNotificationIDs, ntf.Uid)
				break
			}
		}
	}
	event := events.NotificationReadEvent{
		SessionID:           cmd.SessionID,
		ReadNotificationUID: cmd.NotificationUID,
		MarkedAllAsRead:     markedAllAsRead,
		ReadNotificationIDs: readNotificationIDs,
		PreviousUnreadCount: previousUnreadCount,
		NewUnreadCount:      previousUnreadCount - len(readNotificationIDs), // estimate
		Timestamp:           time.Now(),
	}
	// 1. Persist event to SQLite event store
	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to store event: %w", err)
	}
	// 2. Update projection
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to update projection: %w", err)
	}
	// 3. Publish signal to JetStream
	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}
	return nil
}
