package realtime

import (
	"context"
	"net/http"

	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/internal/notifications"
	"github.com/mbolli/tickit/web/ui/components"
	"github.com/starfederation/datastar-go/datastar"
)

// NotificationRenderer handles rendering notification updates
type NotificationRenderer struct {
	notificationService *notifications.NotificationService
}

// NewNotificationRenderer creates a new notification renderer
func NewNotificationRenderer(notificationService *notifications.NotificationService) *NotificationRenderer {
	return &NotificationRenderer{
		notificationService: notificationService,
	}
}

// GetEntityType returns the entity type this renderer handles
func (r *NotificationRenderer) GetEntityType() string {
	return "notifications"
}

// RenderUpdate renders notification updates
func (r *NotificationRenderer) RenderUpdate(ctx context.Context, data interface{}) (datastar.TemplComponent, map[string]any, error) {
	logger.Debug().Msg("🔔 NotificationRenderer: Rendering notification update")

	// Fetch current notifications for this session
	ntfs, err := r.notificationService.GetNotifications(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("NotificationRenderer: Failed to get notifications")
		return nil, nil, err
	}

	logger.Debug().
		Int("count", len(ntfs.Notifications)).
		Int("unread", ntfs.UnreadCount()).
		Msg("NotificationRenderer: Fetched notifications")

	// Return the notifications component
	component := components.Notifications(ctx, ntfs)
	return component, nil, nil
}

// NotificationSubscription creates middleware for notification subscriptions
func NotificationSubscription(renderer *NotificationRenderer) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Create subscription context for notifications
			connectionID, ok := r.Context().Value("connection_id").(string)
			if ok {
				subCtx := &SubscriptionContext{
					Filter:        make(map[string]interface{}),
					EventPatterns: []string{"notifications.*"}, // Subscribe to all notification events
				}
				StoreSubscriptionContext(connectionID, subCtx)
				logger.Debug().
					Str("connection_id", connectionID).
					Msg("Created notification subscription context")
			}
			next.ServeHTTP(w, r)
		})
	}
}
