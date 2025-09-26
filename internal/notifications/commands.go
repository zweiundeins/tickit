package notifications

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mbolli/tickit/internal/shared"
)

// Commands

type CreateNotificationCommand struct {
	UserID string `json:"user_id,omitempty"` // Optional user ID for user-specific notifications
	Title  string `json:"title"`
	Body   string `json:"body"`
}

type DeleteNotificationCommand struct {
	UserID          string `json:"user_id,omitempty"`          // Optional user ID
	NotificationUID string `json:"notification_uid,omitempty"` // If empty, delete all
}

type MarkNotificationReadCommand struct {
	UserID          string `json:"user_id,omitempty"`          // Optional user ID
	NotificationUID string `json:"notification_uid,omitempty"` // If empty, mark all as read
}

// CommandHandler handles all write operations
type CommandHandler struct {
	eventStore      *shared.EventStore
	projectionStore *NotificationProjectionStore
	eventBus        shared.EventPublisher
}

// NewCommandHandler creates a new command handler
func NewCommandHandler(eventStore *shared.EventStore, projectionStore *NotificationProjectionStore, eventBus shared.EventPublisher) *CommandHandler {
	return &CommandHandler{
		eventStore:      eventStore,
		projectionStore: projectionStore,
		eventBus:        eventBus,
	}
}

// HandleCreateNotification creates a new notification
func (h *CommandHandler) HandleCreateNotification(ctx context.Context, cmd CreateNotificationCommand) error {
	// Security: For user-specific notifications, validate authenticated user context
	// General notifications (empty UserID) can be created by any authenticated user (system notifications)
	if cmd.UserID != "" {
		authenticatedUserID, ok := ctx.Value("user_id").(string)
		if !ok || authenticatedUserID == "" {
			return fmt.Errorf("authentication required")
		}
		if cmd.UserID == "" {
			cmd.UserID = authenticatedUserID // default to self
		}
		// For now, allow users to create notifications for themselves
		// In the future, you might want admin-only creation for other users
		if cmd.UserID != authenticatedUserID {
			return fmt.Errorf("unauthorized: cannot create notifications for user %s", cmd.UserID)
		}
	}

	notification := &Notification{
		Uid:   uuid.New().String(),
		Date:  time.Now(),
		Title: cmd.Title,
		Body:  cmd.Body,
		Read:  false,
	}
	event := NotificationCreatedEvent{
		UserID:       cmd.UserID,
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
	// Security: Validate that the command UserID matches the authenticated user
	authenticatedUserID, ok := ctx.Value("user_id").(string)
	if !ok || authenticatedUserID == "" {
		return fmt.Errorf("authentication required")
	}
	if cmd.UserID == "" {
		cmd.UserID = authenticatedUserID // default to self
	}
	if cmd.UserID != authenticatedUserID {
		return fmt.Errorf("unauthorized: cannot delete notifications for user %s", cmd.UserID)
	}

	// Compose event
	deletedAll := cmd.NotificationUID == ""
	// For event, we need to know which IDs will be deleted
	notifs, err := h.projectionStore.GetNotifications(ctx, cmd.UserID)
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
	event := NotificationDeletedEvent{
		UserID:                 cmd.UserID,
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
	// Security: Validate that the command UserID matches the authenticated user
	authenticatedUserID, ok := ctx.Value("user_id").(string)
	if !ok || authenticatedUserID == "" {
		return fmt.Errorf("authentication required")
	}
	if cmd.UserID == "" {
		cmd.UserID = authenticatedUserID // default to self
	}
	// Only allow marking own notifications as read
	if cmd.UserID != authenticatedUserID {
		return fmt.Errorf("unauthorized: cannot mark notifications as read for user %s", cmd.UserID)
	}

	notifs, err := h.projectionStore.GetNotifications(ctx, cmd.UserID)
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
	event := NotificationReadEvent{
		UserID:              cmd.UserID,
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

func (c CreateNotificationCommand) GetCommandType() string { return "CreateNotification" }

var _ shared.Command = (*CreateNotificationCommand)(nil)

func (c DeleteNotificationCommand) GetCommandType() string { return "DeleteNotification" }

var _ shared.Command = (*DeleteNotificationCommand)(nil)

func (c MarkNotificationReadCommand) GetCommandType() string { return "MarkNotificationRead" }

var _ shared.Command = (*MarkNotificationReadCommand)(nil)
