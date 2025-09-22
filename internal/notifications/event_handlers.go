package notifications

import (
	"github.com/mbolli/tickit/internal/shared"
)

// GetEventHandlers returns functional event handlers for the notifications module
// Following the same pattern as the users module
func GetEventHandlers() shared.EventHandlers {
	// Currently no functional event handlers needed for notifications module
	// Notifications are primarily created by command handlers, not event handlers
	return shared.EventHandlers{}
}
