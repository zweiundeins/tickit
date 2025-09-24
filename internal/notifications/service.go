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

// CreateGeneralNotification creates a general notification without user context
func (s *NotificationService) CreateGeneralNotification(ctx context.Context, title, body string) error {
	cmd := CreateNotificationCommand{
		UserID: "", // No user context
		Title:  title,
		Body:   body,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// CreateNotification creates a new notification with user context
func (s *NotificationService) CreateNotification(ctx context.Context, title, body string) error {
	cmd := CreateNotificationCommand{
		UserID: ctx.Value("user_id").(string),
		Title:  title,
		Body:   body,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// DeleteNotification deletes a specific notification or all notifications
func (s *NotificationService) DeleteNotification(ctx context.Context, notificationUID string) error {
	cmd := DeleteNotificationCommand{
		UserID:          ctx.Value("user_id").(string),
		NotificationUID: notificationUID,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// DeleteNotificationWithUser deletes a specific notification or all notifications with user context
func (s *NotificationService) DeleteNotificationWithUser(ctx context.Context, notificationUID string) error {
	cmd := DeleteNotificationCommand{
		UserID:          ctx.Value("user_id").(string),
		NotificationUID: notificationUID,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// MarkNotificationRead marks a specific notification or all notifications as read
func (s *NotificationService) MarkNotificationRead(ctx context.Context, notificationUID string) error {
	cmd := MarkNotificationReadCommand{
		UserID:          ctx.Value("user_id").(string),
		NotificationUID: notificationUID,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// MarkNotificationReadWithUser marks a specific notification or all notifications as read with user context
func (s *NotificationService) MarkNotificationReadWithUser(ctx context.Context, notificationUID string) error {
	cmd := MarkNotificationReadCommand{
		UserID:          ctx.Value("user_id").(string),
		NotificationUID: notificationUID,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// GetNotifications retrieves all notifications for a session
func (s *NotificationService) GetNotifications(ctx context.Context) (*Notifications, error) {
	query := GetNotificationsQuery{
		UserID: ctx.Value("user_id").(string),
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
func (s *NotificationService) GetUnreadCount(ctx context.Context) (*UnreadCountResult, error) {
	query := GetUnreadCountQuery{
		UserID: ctx.Value("user_id").(string),
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
func (s *NotificationService) GetNotificationByUID(ctx context.Context, notificationUID string) (*Notification, error) {
	query := GetNotificationByUIDQuery{
		UserID:          ctx.Value("user_id").(string),
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
