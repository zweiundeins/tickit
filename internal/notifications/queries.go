package notifications

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/internal/shared"
)

// Query handlers for notification read operations

type GetNotificationsQuery struct {
	UserID string `json:"user_id,omitempty"` // Optional user ID for user-specific lookup
}

func (q GetNotificationsQuery) GetQueryType() string { return "GetNotifications" }

var _ shared.Query = (*GetNotificationsQuery)(nil)

type GetUnreadCountQuery struct {
	UserID string `json:"user_id"`
}

func (q GetUnreadCountQuery) GetQueryType() string { return "GetUnreadCount" }

var _ shared.Query = (*GetUnreadCountQuery)(nil)

type GetNotificationByUIDQuery struct {
	UserID          string `json:"user_id"`
	NotificationUID string `json:"notification_uid"`
}

func (q GetNotificationByUIDQuery) GetQueryType() string { return "GetNotificationByUID" }

var _ shared.Query = (*GetNotificationByUIDQuery)(nil)

// Query results
type NotificationsResult struct {
	Notifications []*Notification `json:"notifications"`
	UnreadCount   int             `json:"unread_count"`
	TotalCount    int             `json:"total_count"`
}

type UnreadCountResult struct {
	Count int `json:"count"`
}

// QueryHandler handles all read operations for notifications
type QueryHandler struct {
	projectionStore *NotificationProjectionStore
}

// NewQueryHandler creates a new query handler
func NewQueryHandler(projectionStore *NotificationProjectionStore) *QueryHandler {
	return &QueryHandler{
		projectionStore: projectionStore,
	}
}

// HandleGetNotifications retrieves all notifications for a session
func (h *QueryHandler) HandleGetNotifications(ctx context.Context, query GetNotificationsQuery) (*NotificationsResult, error) {
	ntfs, err := h.getNotifications(ctx, query.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get notifications: %w", err)
	}

	return &NotificationsResult{
		Notifications: ntfs.Notifications,
		UnreadCount:   ntfs.UnreadCount(),
		TotalCount:    len(ntfs.Notifications),
	}, nil
}

// HandleGetUnreadCount retrieves only the unread count (optimized query)
func (h *QueryHandler) HandleGetUnreadCount(ctx context.Context, query GetUnreadCountQuery) (*UnreadCountResult, error) {
	ntfs, err := h.getNotifications(ctx, query.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get notifications: %w", err)
	}

	return &UnreadCountResult{
		Count: ntfs.UnreadCount(),
	}, nil
}

// HandleGetNotificationByUID retrieves a specific notification
func (h *QueryHandler) HandleGetNotificationByUID(ctx context.Context, query GetNotificationByUIDQuery) (*NotificationsResult, error) {
	ntfs, err := h.getNotifications(ctx, query.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get notifications: %w", err)
	}

	return &NotificationsResult{
		Notifications: ntfs.Notifications,
		UnreadCount:   ntfs.UnreadCount(),
		TotalCount:    len(ntfs.Notifications),
	}, nil
}

// Helper method to get notifications from storage
func (h *QueryHandler) getNotifications(ctx context.Context, userID string) (*Notifications, error) {
	return h.projectionStore.GetNotifications(ctx, userID)
}
