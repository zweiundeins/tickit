package realtime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mbolli/tickit/internal/logger"
	"github.com/starfederation/datastar-go/datastar"
)

// SessionGroup represents a group of sessions with shared filtering criteria
type SessionGroup struct {
	ID            string                 `json:"id"`             // Unique identifier for the group
	Filter        map[string]interface{} `json:"filter"`         // Filtering criteria shared by sessions in this group
	Sessions      map[string]*Session    `json:"sessions"`       // Sessions in this group
	EventPatterns []string               `json:"event_patterns"` // Event patterns this group subscribes to
	mu            sync.RWMutex           `json:"-"`              // Mutex for thread-safe access
}

// GetRenderers returns all renderers that match this group's event patterns
func (sg *SessionGroup) GetRenderers() map[string]ComponentRenderer {
	return GetRenderersForPatterns(sg.EventPatterns)
}

// GetRenderer returns the best matching renderer for a specific event type
func (sg *SessionGroup) GetRenderer(eventType string) ComponentRenderer {
	return GetRenderer(eventType)
} // Session represents an active SSE connection
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
func GenerateGroupKey(filter map[string]interface{}, eventPatterns []string) string {
	data := map[string]interface{}{
		"filter":         filter,
		"event_patterns": eventPatterns,
	}

	jsonData, _ := json.Marshal(data)
	hash := sha256.Sum256(jsonData)
	// Use first pattern as prefix if available, otherwise use "multi"
	prefix := "multi"
	if len(eventPatterns) > 0 {
		prefix = strings.Split(eventPatterns[0], ".")[0]
	}
	return fmt.Sprintf("%s:%x", prefix, hash[:8])
}

// AddSession adds a session to the appropriate group
// Returns the SessionGroup and a channel (updateCh) for receiving UI update events for this session.
func (gm *GroupManager) AddSession(
	connectionID, // Unique connection ID
	sessionID, // Browser session ID (shared across tabs)
	userID string,
	filter map[string]interface{},
	eventPatterns []string,
	renderer ComponentRenderer,
) (*SessionGroup, chan *UIUpdateEvent) {
	// Create new session
	ctx, cancel := context.WithCancel(context.Background())
	updateCh := make(chan *UIUpdateEvent, 10) // Buffered channel for updates

	newSession := &Session{
		ID:        connectionID, // Unique connection ID
		SessionID: sessionID,    // Browser session ID
		UserID:    userID,
		Context:   ctx,
		Cancel:    cancel,
		UpdateCh:  updateCh,
	}

	// Get or create group
	gm.mu.Lock()
	defer gm.mu.Unlock()

	groupKey := GenerateGroupKey(filter, eventPatterns)
	group, exists := gm.groups[groupKey]
	if !exists {
		group = &SessionGroup{
			ID:            groupKey,
			Filter:        filter,
			EventPatterns: eventPatterns,
			Sessions:      make(map[string]*Session),
		}
		gm.groups[groupKey] = group
	}

	// Add session to group
	group.mu.Lock()
	group.Sessions[connectionID] = newSession
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

// GetGroupsForEventType returns all groups that should receive updates for a specific event type
// based on their event patterns rather than entity type matching
func (gm *GroupManager) GetGroupsForEventType(eventType string) []*SessionGroup {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	var matchingGroups []*SessionGroup

	for _, group := range gm.groups {
		// Check if the event type matches any of the group's event patterns
		if group.MatchesEventPattern(eventType) {
			matchingGroups = append(matchingGroups, group)
		}
	}

	return matchingGroups
}

// MatchesEventPattern checks if an event type matches any of the group's event patterns
func (sg *SessionGroup) MatchesEventPattern(eventType string) bool {
	sg.mu.RLock()
	defer sg.mu.RUnlock()

	for _, pattern := range sg.EventPatterns {
		if sg.matchesPattern(eventType, pattern) {
			return true
		}
	}
	return false
}

// matchesPattern checks if an event type matches a pattern with wildcards
func (sg *SessionGroup) matchesPattern(eventType, pattern string) bool {
	// Exact match
	if eventType == pattern {
		return true
	}

	// Handle wildcard patterns
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(eventType, prefix)
	}

	return false
}

// Note: Pattern matching in GetGroupsForEventType handles all event filtering
// No need for ShouldReceiveUpdate since patterns are more precise

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

	// Renderer will be resolved per event type using the registry when needed

	// Send to all active sessions in this group via their update channels
	sentCount := 0
	for sessionID, session := range sg.Sessions {
		select {
		case <-session.Context.Done():
			// Session is closed, skip it
			logger.Debug().Str("session_id", sessionID).Str("group_id", sg.ID).Msg("Skipping closed session (context done)")
			continue
		case session.UpdateCh <- uiEvent:
			// Successfully sent to session's channel
			logger.Debug().Str("session_id", sessionID).Str("group_id", sg.ID).Msg("Successfully sent update to session")
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
	groupsByPattern := make(map[string]int)

	for _, group := range gm.groups {
		group.mu.RLock()
		totalSessions += len(group.Sessions)
		// Count by first event pattern
		if len(group.EventPatterns) > 0 {
			groupsByPattern[group.EventPatterns[0]]++
		} else {
			groupsByPattern["no-pattern"]++
		}
		group.mu.RUnlock()
	}

	return map[string]interface{}{
		"total_groups":      len(gm.groups),
		"total_sessions":    totalSessions,
		"groups_by_pattern": groupsByPattern,
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
	userID string,
	filter map[string]interface{},
	eventPatterns []string,
	existingSession *Session,
) (*SessionGroup, chan *UIUpdateEvent) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	groupKey := GenerateGroupKey(filter, eventPatterns)

	// Get or create group
	group, exists := gm.groups[groupKey]
	if !exists {
		group = &SessionGroup{
			ID:            groupKey,
			Filter:        filter,
			EventPatterns: eventPatterns,
			Sessions:      make(map[string]*Session),
		}
		gm.groups[groupKey] = group
	}

	// Add existing session to group
	group.mu.Lock()
	group.Sessions[connectionID] = existingSession
	group.mu.Unlock()

	return group, existingSession.UpdateCh
}
