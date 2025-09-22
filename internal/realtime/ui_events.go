package realtime

import (
	"time"
)

// UIUpdateEvent represents a UI-specific event for real-time updates
type UIUpdateEvent struct {
	EventType  string                 `json:"event_type"`  // "issues.patched", "projects.created", etc.
	EntityType string                 `json:"entity_type"` // "issues", "projects", "users"
	EntityID   string                 `json:"entity_id"`   // ID of the affected entity
	Timestamp  time.Time              `json:"timestamp"`
	Data       map[string]interface{} `json:"data"` // Event-specific data
}
