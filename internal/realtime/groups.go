package realtime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/mbolli/tickit/internal/logger"
	"github.com/starfederation/datastar-go/datastar"
)

// SessionGroup represents a group of sessions with shared subscription criteria
type SessionGroup struct {
	ID          string                 `json:"id"`
	EntityType  string                 `json:"entity_type"` // "issues", "projects", "users"
	EntityID    *string                `json:"entity_id"`   // nil for lists, specific ID for details
	Filter      map[string]interface{} `json:"filter"`      // pagination and filtering criteria
	Sessions    map[string]*Session    `json:"sessions"`    // connectionID -> Session
	Renderer    ComponentRenderer      `json:"-"`           // how to render updates for this group
	CreatedAt   time.Time              `json:"created_at"`
	LastUpdated time.Time              `json:"last_updated"`
	mu          sync.RWMutex           `json:"-"`
}

// Session represents an active SSE connection
type Session struct {
	ID            string              `json:"id"`         // Unique connection ID (not session ID)
	SessionID     string              `json:"session_id"` // Browser session ID (shared across tabs)
	UserID        string              `json:"user_id"`    // User identifier
	CreatedAt     time.Time           `json:"created_at"`
	Context       context.Context     `json:"-"`
	Cancel        context.CancelFunc  `json:"-"`
	UpdateCh      chan *UIUpdateEvent `json:"-"` // Channel to receive updates for this connection
	channelClosed bool                `json:"-"` // Track if channel is already closed
	mu            sync.Mutex          `json:"-"` // Protect channelClosed flag
}

// ComponentRenderer interface for rendering updates
type ComponentRenderer interface {
	// RenderUpdate[T realtime.GroupUpdateData](ctx context.Context, data T) (datastar.TemplComponent, map[string]any, error)
	RenderUpdate(ctx context.Context, data interface{}) (datastar.TemplComponent, map[string]any, error)
	GetEntityType() string
}

// GroupManager manages session groups and routing
type GroupManager struct {
	groups map[string]*SessionGroup // groupID -> SessionGroup
	mu     sync.RWMutex
}

// NewGroupManager creates a new group manager
func NewGroupManager() *GroupManager {
	return &GroupManager{
		groups: make(map[string]*SessionGroup),
	}
}

// GenerateGroupKey creates a deterministic group key based on subscription criteria
func GenerateGroupKey(entityType string, entityID *string, filter map[string]interface{}) string {
	data := map[string]interface{}{
		"entity_type": entityType,
		"entity_id":   entityID,
		"filter":      filter,
	}

	jsonData, _ := json.Marshal(data)
	hash := sha256.Sum256(jsonData)
	return fmt.Sprintf("%s:%x", entityType, hash[:8])
}

// AddSession adds a session to the appropriate group
// Returns the SessionGroup and a channel (updateCh) for receiving UI update events for this session.
func (gm *GroupManager) AddSession(
	connectionID, // Unique connection ID
	sessionID, // Browser session ID (shared across tabs)
	userID,
	entityType string,
	entityID *string,
	filter map[string]interface{},
	renderer ComponentRenderer,
) (*SessionGroup, chan *UIUpdateEvent) {
	// Create new session
	ctx, cancel := context.WithCancel(context.Background())
	updateCh := make(chan *UIUpdateEvent, 10) // Buffered channel for updates

	newSession := &Session{
		ID:        connectionID, // Unique connection ID
		SessionID: sessionID,    // Browser session ID
		UserID:    userID,
		CreatedAt: time.Now(),
		Context:   ctx,
		Cancel:    cancel,
		UpdateCh:  updateCh,
	}

	// Get or create group
	gm.mu.Lock()
	defer gm.mu.Unlock()

	groupKey := GenerateGroupKey(entityType, entityID, filter)
	group, exists := gm.groups[groupKey]
	if !exists {
		group = &SessionGroup{
			ID:          groupKey,
			EntityType:  entityType,
			EntityID:    entityID,
			Filter:      filter,
			Sessions:    make(map[string]*Session),
			Renderer:    renderer,
			CreatedAt:   time.Now(),
			LastUpdated: time.Now(),
		}
		gm.groups[groupKey] = group
	}

	// Add session to group
	group.mu.Lock()
	group.Sessions[connectionID] = newSession
	group.LastUpdated = time.Now()
	group.mu.Unlock()

	return group, updateCh
}

// RemoveSession removes a connection from its group
func (gm *GroupManager) RemoveSession(connectionID string) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	for groupID, group := range gm.groups {
		group.mu.Lock()
		if session, exists := group.Sessions[connectionID]; exists {
			session.Cancel()

			// Safely close the update channel
			session.mu.Lock()
			if !session.channelClosed {
				logger.Info().Str("connection_id", connectionID).Msg("RemoveSession: Closing update channel")
				close(session.UpdateCh)
				session.channelClosed = true
			} else {
				logger.Warn().Str("connection_id", connectionID).Msg("RemoveSession: Channel already closed")
			}
			session.mu.Unlock()

			delete(group.Sessions, connectionID)
			group.LastUpdated = time.Now()

			// Remove empty groups
			if len(group.Sessions) == 0 {
				delete(gm.groups, groupID)
			}
		}
		group.mu.Unlock()
	}
}

// GetGroupByConnectionID finds the group containing a specific connection
func (gm *GroupManager) GetGroupByConnectionID(connectionID string) *SessionGroup {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	for _, group := range gm.groups {
		group.mu.RLock()
		if _, exists := group.Sessions[connectionID]; exists {
			group.mu.RUnlock()
			return group
		}
		group.mu.RUnlock()
	}
	return nil
}

// GetGroupsForEntity returns all groups that should receive updates for a specific entity
func (gm *GroupManager) GetGroupsForEntity(entityType, entityID string) []*SessionGroup {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	var matchingGroups []*SessionGroup

	for _, group := range gm.groups {
		if group.EntityType == entityType {
			// Check if this group cares about this entity
			if group.ShouldReceiveUpdate(entityID) {
				matchingGroups = append(matchingGroups, group)
			}
		}
	}

	return matchingGroups
}

// ShouldReceiveUpdate determines if a group should receive an update for a specific entity
func (sg *SessionGroup) ShouldReceiveUpdate(entityID string) bool {
	sg.mu.RLock()
	defer sg.mu.RUnlock()

	// For detail views, only update if it's the exact entity
	if sg.EntityID != nil {
		return *sg.EntityID == entityID
	}

	// For list views, check if entity matches current filters
	// This is a simplified version - in practice, you'd query your data store
	// to see if the entity would appear in the current filtered/paginated view
	return sg.entityMatchesFilter(entityID)
}

// entityMatchesFilter checks if an entity matches the group's filter criteria
func (sg *SessionGroup) entityMatchesFilter(entityID string) bool {
	// Simplified implementation - in practice, this would:
	// 1. Load the entity from the projection store
	// 2. Apply the group's filter criteria
	// 3. Check if it would appear in the current page

	// For now, assume all entities match (will be refined in implementation)
	return true
}

// GetActiveSessions returns all active sessions in the group
func (sg *SessionGroup) GetActiveSessions() []*Session {
	sg.mu.RLock()
	defer sg.mu.RUnlock()

	sessions := make([]*Session, 0, len(sg.Sessions))
	for _, session := range sg.Sessions {
		select {
		case <-session.Context.Done():
			// Session is closed, skip it
			continue
		default:
			sessions = append(sessions, session)
		}
	}

	return sessions
}

// SendUpdateToAllSessions sends an update to all active sessions in the group
func (sg *SessionGroup) SendUpdateToAllSessions(ctx context.Context, uiEvent *UIUpdateEvent) error {
	sg.mu.RLock()
	defer sg.mu.RUnlock()

	if sg.Renderer == nil {
		logger.Error().Str("group_id", sg.ID).Msg("Cannot send update to group: renderer is nil")
		return fmt.Errorf("renderer is nil for group %s", sg.ID)
	}

	// Send to all active sessions in this group via their update channels
	sentCount := 0
	for sessionID, session := range sg.Sessions {
		select {
		case <-session.Context.Done():
			// Session is closed, skip it
			logger.Info().Str("session_id", sessionID).Str("group_id", sg.ID).Msg("Skipping closed session (context done)")
			continue
		case session.UpdateCh <- uiEvent:
			// Successfully sent to session's channel
			logger.Info().Str("session_id", sessionID).Str("group_id", sg.ID).Msg("Successfully sent update to session")
			sentCount++
		default:
			// Channel is full, skip this session to avoid blocking
			logger.Warn().Str("session_id", sessionID).Msg("Session channel full, skipping update")
		}
	}

	logger.Info().Int("sent_count", sentCount).Str("group_id", sg.ID).Msg("Sent update to sessions")
	return nil
}

// GetStats returns statistics about the group manager
func (gm *GroupManager) GetStats() map[string]interface{} {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	totalSessions := 0
	groupsByType := make(map[string]int)

	for _, group := range gm.groups {
		group.mu.RLock()
		totalSessions += len(group.Sessions)
		groupsByType[group.EntityType]++
		group.mu.RUnlock()
	}

	return map[string]interface{}{
		"total_groups":   len(gm.groups),
		"total_sessions": totalSessions,
		"groups_by_type": groupsByType,
	}
}

// GetSession safely retrieves a session from the group
func (sg *SessionGroup) GetSession(connectionID string) *Session {
	sg.mu.RLock()
	defer sg.mu.RUnlock()
	return sg.Sessions[connectionID]
}

// RemoveSessionFromGroup removes a session from a specific group with optional cleanup
func (gm *GroupManager) RemoveSessionFromGroup(group *SessionGroup, connectionID string, cleanup bool) {
	group.mu.Lock()
	defer group.mu.Unlock()

	if session, exists := group.Sessions[connectionID]; exists {
		if cleanup {
			session.Cancel()

			// Safely close the update channel
			session.mu.Lock()
			if !session.channelClosed {
				logger.Info().Str("connection_id", connectionID).Msg("RemoveSessionFromGroup: Closing update channel")
				close(session.UpdateCh)
				session.channelClosed = true
			}
			session.mu.Unlock()
		}

		delete(group.Sessions, connectionID)
		group.LastUpdated = time.Now()

		// Remove empty groups
		if len(group.Sessions) == 0 {
			gm.mu.Lock()
			defer gm.mu.Unlock()
			for groupID, g := range gm.groups {
				if g == group {
					delete(gm.groups, groupID)
					break
				}
			}
		}
	}
}

// AddSessionToGroup adds an existing session to a group (for moving between groups)
func (gm *GroupManager) AddSessionToGroup(
	connectionID,
	sessionID,
	userID,
	entityType string,
	entityID *string,
	filter map[string]interface{},
	renderer ComponentRenderer,
	existingSession *Session,
) (*SessionGroup, chan *UIUpdateEvent) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	groupKey := GenerateGroupKey(entityType, entityID, filter)

	// Get or create group
	group, exists := gm.groups[groupKey]
	if !exists {
		group = &SessionGroup{
			ID:          groupKey,
			EntityType:  entityType,
			EntityID:    entityID,
			Filter:      filter,
			Sessions:    make(map[string]*Session),
			Renderer:    renderer,
			CreatedAt:   time.Now(),
			LastUpdated: time.Now(),
		}
		gm.groups[groupKey] = group
	}

	// Add existing session to group
	group.mu.Lock()
	group.Sessions[connectionID] = existingSession
	group.LastUpdated = time.Now()
	group.mu.Unlock()

	return group, existingSession.UpdateCh
}
