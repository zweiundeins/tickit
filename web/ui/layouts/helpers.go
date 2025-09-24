package layouts

import (
	"github.com/mbolli/tickit/internal/notifications"
)

// EmptyNotifications returns an empty notifications structure for initial rendering
func EmptyNotifications() *notifications.Notifications {
	return &notifications.Notifications{
		Notifications: []*notifications.Notification{},
	}
}
