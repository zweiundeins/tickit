package projects

import (
	"github.com/mbolli/tickit/internal/shared"
)

// GetEventHandlers returns functional event handlers for the projects module
// Following the same pattern as the users module
func GetEventHandlers() shared.EventHandlers {
	// Currently no functional event handlers needed for projects module
	// The actual business logic handlers are in the users module for cross-module integration
	return shared.EventHandlers{}
}
