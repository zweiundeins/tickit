package notifications

import (
	"github.com/mbolli/tickit/internal/shared"
)

// GetEventHandlers returns functional event handlers for the notifications module
// Following the same pattern as the users module
func GetEventHandlers() shared.EventHandlers {
	// Event handlers are now registered directly in bootstrap/deps.go
	// following the same pattern as other cross-module integration handlers
	return shared.EventHandlers{}
}
