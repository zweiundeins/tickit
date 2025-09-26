package notifications

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/internal/shared"
)

// NotificationService handles notification-related business logic
type NotificationService struct {
	commandBus *shared.CommandBus
	queryBus   *shared.QueryBus
}

// getUserIDFromContext safely extracts user ID from context
func getUserIDFromContext(ctx context.Context) (string, error) {
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return "", fmt.Errorf("user_id not found in context or is empty")
	}
	return userID, nil
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
	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		return err
	}

	cmd := CreateNotificationCommand{
		UserID: userID,
		Title:  title,
		Body:   body,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// DeleteNotification deletes a specific notification or all notifications
func (s *NotificationService) DeleteNotification(ctx context.Context, notificationUID string) error {
	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		return err
	}

	cmd := DeleteNotificationCommand{
		UserID:          userID,
		NotificationUID: notificationUID,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// DeleteNotificationWithUser deletes a specific notification or all notifications with user context
func (s *NotificationService) DeleteNotificationWithUser(ctx context.Context, notificationUID string) error {
	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		return err
	}

	cmd := DeleteNotificationCommand{
		UserID:          userID,
		NotificationUID: notificationUID,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// MarkNotificationRead marks a specific notification or all notifications as read
func (s *NotificationService) MarkNotificationRead(ctx context.Context, notificationUID string) error {
	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		return err
	}

	cmd := MarkNotificationReadCommand{
		UserID:          userID,
		NotificationUID: notificationUID,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// MarkNotificationReadWithUser marks a specific notification or all notifications as read with user context
func (s *NotificationService) MarkNotificationReadWithUser(ctx context.Context, notificationUID string) error {
	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		return err
	}

	cmd := MarkNotificationReadCommand{
		UserID:          userID,
		NotificationUID: notificationUID,
	}

	return s.commandBus.Execute(ctx, cmd)
}

// GetNotifications retrieves all notifications for a session
func (s *NotificationService) GetNotifications(ctx context.Context) (*Notifications, error) {
	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	query := GetNotificationsQuery{
		UserID: userID,
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

// GetAllNotificationsForUser retrieves both user-specific and general notifications
func (s *NotificationService) GetAllNotificationsForUser(ctx context.Context) (*Notifications, error) {
	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Get user-specific notifications
	userQuery := GetNotificationsQuery{
		UserID: userID,
	}

	userResult, err := s.queryBus.Execute(ctx, userQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to execute user notifications query: %w", err)
	}

	userNotificationsResult, ok := userResult.(*NotificationsResult)
	if !ok {
		return nil, fmt.Errorf("unexpected user notifications result type: %T", userResult)
	}

	// Get general notifications (empty user_id)
	generalQuery := GetNotificationsQuery{
		UserID: "", // Empty string for general notifications
	}

	generalResult, err := s.queryBus.Execute(ctx, generalQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to execute general notifications query: %w", err)
	}

	generalNotificationsResult, ok := generalResult.(*NotificationsResult)
	if !ok {
		return nil, fmt.Errorf("unexpected general notifications result type: %T", generalResult)
	}

	// Merge both sets of notifications
	allNotifications := make([]*Notification, 0, len(userNotificationsResult.Notifications)+len(generalNotificationsResult.Notifications))
	allNotifications = append(allNotifications, userNotificationsResult.Notifications...)
	allNotifications = append(allNotifications, generalNotificationsResult.Notifications...)

	return &Notifications{Notifications: allNotifications}, nil
}

// GetUnreadCount retrieves only the unread count
func (s *NotificationService) GetUnreadCount(ctx context.Context) (*UnreadCountResult, error) {
	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	query := GetUnreadCountQuery{
		UserID: userID,
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
	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	query := GetNotificationByUIDQuery{
		UserID:          userID,
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
