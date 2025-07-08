package notifications

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/internal/shared"
)

// NotificationService coordinates between HTTP handlers and CQRS buses
type NotificationService struct {
	commandBus *shared.CommandBus
	queryBus   *shared.QueryBus
}

// NewNotificationService creates a new notification service
func NewNotificationService(commandBus *shared.CommandBus, queryBus *shared.QueryBus) *NotificationService {
	return &NotificationService{
		commandBus: commandBus,
		queryBus:   queryBus,
	}
}

// CreateNotification creates a new notification
func (s *NotificationService) CreateNotification(ctx context.Context, sessionID, title, body string) error {
	cmd := CreateNotificationCommand{
		SessionID: sessionID,
		Title:     title,
		Body:      body,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// DeleteNotification deletes a specific notification or all notifications
func (s *NotificationService) DeleteNotification(ctx context.Context, sessionID, notificationUID string) error {
	cmd := DeleteNotificationCommand{
		SessionID:       sessionID,
		NotificationUID: notificationUID,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// MarkNotificationRead marks a specific notification or all notifications as read
func (s *NotificationService) MarkNotificationRead(ctx context.Context, sessionID, notificationUID string) error {
	cmd := MarkNotificationReadCommand{
		SessionID:       sessionID,
		NotificationUID: notificationUID,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// GetNotifications retrieves all notifications for a session
func (s *NotificationService) GetNotifications(ctx context.Context, sessionID string) (*Notifications, error) {
	query := GetNotificationsQuery{
		SessionID: sessionID,
	}

	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	notificationsResult, ok := result.(*NotificationsResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	return &Notifications{Notifications: notificationsResult.Notifications}, nil
}

// GetUnreadCount retrieves only the unread count
func (s *NotificationService) GetUnreadCount(ctx context.Context, sessionID string) (*UnreadCountResult, error) {
	query := GetUnreadCountQuery{
		SessionID: sessionID,
	}

	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	unreadCountResult, ok := result.(*UnreadCountResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	return unreadCountResult, nil
}

// GetNotificationByUID retrieves a specific notification by UID
func (s *NotificationService) GetNotificationByUID(ctx context.Context, sessionID, notificationUID string) (*Notification, error) {
	query := GetNotificationByUIDQuery{
		SessionID:       sessionID,
		NotificationUID: notificationUID,
	}

	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	notification, ok := result.(*Notification)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	return notification, nil
}
