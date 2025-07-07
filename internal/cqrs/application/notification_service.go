package application

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/internal/cqrs/bus"
	"github.com/mbolli/tickit/internal/cqrs/commands"
	"github.com/mbolli/tickit/internal/cqrs/queries"
	"github.com/mbolli/tickit/internal/models"
)

// NotificationService coordinates between HTTP handlers and CQRS buses
type NotificationService struct {
	commandBus *bus.CommandBus
	queryBus   *bus.QueryBus
}

// NewNotificationService creates a new notification service
func NewNotificationService(commandBus *bus.CommandBus, queryBus *bus.QueryBus) *NotificationService {
	return &NotificationService{
		commandBus: commandBus,
		queryBus:   queryBus,
	}
}

// CreateNotification creates a new notification
func (s *NotificationService) CreateNotification(ctx context.Context, sessionID, title, body string) error {
	cmd := commands.CreateNotificationCommand{
		SessionID: sessionID,
		Title:     title,
		Body:      body,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// DeleteNotification deletes a specific notification or all notifications
func (s *NotificationService) DeleteNotification(ctx context.Context, sessionID, notificationUID string) error {
	cmd := commands.DeleteNotificationCommand{
		SessionID:       sessionID,
		NotificationUID: notificationUID,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// MarkNotificationRead marks a specific notification or all notifications as read
func (s *NotificationService) MarkNotificationRead(ctx context.Context, sessionID, notificationUID string) error {
	cmd := commands.MarkNotificationReadCommand{
		SessionID:       sessionID,
		NotificationUID: notificationUID,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// GetNotifications retrieves all notifications for a session
func (s *NotificationService) GetNotifications(ctx context.Context, sessionID string) (*models.Notifications, error) {
	query := queries.GetNotificationsQuery{
		SessionID: sessionID,
	}

	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	notificationsResult, ok := result.(*queries.NotificationsResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	return &models.Notifications{Notifications: notificationsResult.Notifications}, nil
}

// GetUnreadCount retrieves only the unread count
func (s *NotificationService) GetUnreadCount(ctx context.Context, sessionID string) (*queries.UnreadCountResult, error) {
	query := queries.GetUnreadCountQuery{
		SessionID: sessionID,
	}

	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	unreadCountResult, ok := result.(*queries.UnreadCountResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	return unreadCountResult, nil
}

// GetNotificationByUID retrieves a specific notification by UID
func (s *NotificationService) GetNotificationByUID(ctx context.Context, sessionID, notificationUID string) (*models.Notification, error) {
	query := queries.GetNotificationByUIDQuery{
		SessionID:       sessionID,
		NotificationUID: notificationUID,
	}

	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	notification, ok := result.(*models.Notification)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	return notification, nil
}
