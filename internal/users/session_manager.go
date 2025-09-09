package users

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/delaneyj/toolbelt"
	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/session"
	"github.com/mbolli/tickit/internal/shared"
)

// UserSessionManager provides comprehensive session management for user authentication
// It extends the existing Gorilla Sessions functionality with user-specific features
type UserSessionManager struct {
	store           sessions.Store
	userStore       UserStore
	eventBus        *shared.EventBusWrapper
	sessionTimeout  time.Duration
	maxSessions     int // Maximum concurrent sessions per user (0 = unlimited)
	cleanupInterval time.Duration
}

// UserSessionManagerConfig holds configuration for the session manager
type UserSessionManagerConfig struct {
	SessionTimeout  time.Duration // Default: 24 hours
	MaxSessions     int           // Default: 0 (unlimited)
	CleanupInterval time.Duration // Default: 1 hour
}

// NewUserSessionManager creates a new UserSessionManager instance
func NewUserSessionManager(
	store sessions.Store,
	userStore UserStore,
	eventBus *shared.EventBusWrapper,
	config *UserSessionManagerConfig,
) *UserSessionManager {
	if config == nil {
		config = &UserSessionManagerConfig{
			SessionTimeout:  24 * time.Hour,
			MaxSessions:     0, // unlimited
			CleanupInterval: time.Hour,
		}
	}

	sm := &UserSessionManager{
		store:           store,
		userStore:       userStore,
		eventBus:        eventBus,
		sessionTimeout:  config.SessionTimeout,
		maxSessions:     config.MaxSessions,
		cleanupInterval: config.CleanupInterval,
	}

	// Start background cleanup routine
	go sm.startCleanupRoutine()

	return sm
}

// CreateUserSession creates a new authenticated session for a user
func (sm *UserSessionManager) CreateUserSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID string) (*UserSession, error) {
	// Get or create Gorilla session
	sessionName := session.GetSessionName()
	session, err := sm.store.Get(r, sessionName)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Generate session ID if not present
	sessionID, ok := session.Values["id"].(string)
	if !ok || sessionID == "" {
		sessionID = toolbelt.NextEncodedID()
		session.Values["id"] = sessionID
	}

	// Set user ID in session
	session.Values["user_id"] = userID
	session.Values["authenticated"] = true
	session.Values["created_at"] = time.Now().Unix()

	// Configure session options - inherit from the store configuration
	// Don't override the store's options to maintain consistency
	if session.Options == nil {
		session.Options = &sessions.Options{}
	}
	// Only set MaxAge if not already set by the store
	if session.Options.MaxAge == 0 {
		session.Options.MaxAge = int(sm.sessionTimeout.Seconds())
	}

	// Save session to store
	if err := session.Save(r, w); err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	// Check concurrent session limits
	if sm.maxSessions > 0 {
		if err := sm.enforceConcurrentSessionLimits(ctx, userID); err != nil {
			return nil, fmt.Errorf("failed to enforce session limits: %w", err)
		}
	}

	// Get client information
	ipAddress := sm.getClientIP(r)
	userAgent := r.Header.Get("User-Agent")

	// Create user session record
	userSession := &UserSession{
		ID:        toolbelt.NextEncodedID(),
		UserID:    userID,
		SessionID: sessionID,
		ExpiresAt: time.Now().Add(sm.sessionTimeout),
		CreatedAt: time.Now(),
		IPAddress: ipAddress,
		UserAgent: userAgent,
	}

	// Store session in database
	if err := sm.userStore.CreateUserSession(ctx, userSession); err != nil {
		return nil, fmt.Errorf("failed to create user session: %w", err)
	}

	// Publish session created event
	event := UserSessionCreatedEvent{
		SessionID: userSession.SessionID,
		UserID:    userSession.UserID,
		IPAddress: userSession.IPAddress,
		UserAgent: userSession.UserAgent,
		ExpiresAt: userSession.ExpiresAt,
		Timestamp: time.Now(),
	}

	if err := sm.eventBus.PublishEvent(ctx, event); err != nil {
		// Log error but don't fail - session is already created
		// In a production system, you might want to use a proper logger
		fmt.Printf("Failed to publish UserSessionCreatedEvent: %v\n", err)
	}

	return userSession, nil
}

// ValidateSession validates an existing session and returns the user and session info
func (sm *UserSessionManager) ValidateSession(ctx context.Context, r *http.Request) (*User, *UserSession, error) {
	// Get session from store
	sessionName := session.GetSessionName()
	session, err := sm.store.Get(r, sessionName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Check if session is authenticated
	authenticated, ok := session.Values["authenticated"].(bool)
	if !ok || !authenticated {
		return nil, nil, fmt.Errorf("session not authenticated")
	}

	// Get session ID
	sessionID, ok := session.Values["id"].(string)
	if !ok || sessionID == "" {
		return nil, nil, fmt.Errorf("invalid session ID")
	}

	// Get user ID from session
	userID, ok := session.Values["user_id"].(string)
	if !ok || userID == "" {
		return nil, nil, fmt.Errorf("invalid user ID in session")
	}

	// Get session from database
	userSession, err := sm.userStore.GetUserSession(ctx, sessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get user session: %w", err)
	}
	if userSession == nil {
		return nil, nil, fmt.Errorf("session not found")
	}

	// Check if session has expired
	if time.Now().After(userSession.ExpiresAt) {
		// Clean up expired session
		sm.InvalidateSession(ctx, nil, r, sessionID)
		return nil, nil, fmt.Errorf("session expired")
	}

	// Get user information
	user, err := sm.userStore.GetUser(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, nil, fmt.Errorf("user not found")
	}

	// Check if user account is active
	if user.Status != "Active" {
		return nil, nil, fmt.Errorf("user account is not active")
	}

	return user, userSession, nil
}

// InvalidateSession invalidates a specific session
func (sm *UserSessionManager) InvalidateSession(ctx context.Context, w http.ResponseWriter, r *http.Request, sessionID string) error {
	// Remove from database
	if err := sm.userStore.DeleteUserSession(ctx, sessionID); err != nil {
		return fmt.Errorf("failed to delete user session: %w", err)
	}

	// Clear Gorilla session if request context is available
	if r != nil && w != nil {
		sessionName := session.GetSessionName()
		session, err := sm.store.Get(r, sessionName)
		if err == nil {
			// Clear session values
			session.Values = make(map[interface{}]interface{})
			session.Options.MaxAge = -1 // Delete cookie
			session.Save(r, w)
		}
	}

	// Publish session invalidated event
	event := UserSessionInvalidatedEvent{
		SessionID: sessionID,
		Timestamp: time.Now(),
	}

	if err := sm.eventBus.PublishEvent(ctx, event); err != nil {
		// Log error but don't fail
		fmt.Printf("Failed to publish UserSessionInvalidatedEvent: %v\n", err)
	}

	return nil
}

// InvalidateAllUserSessions invalidates all sessions for a specific user
func (sm *UserSessionManager) InvalidateAllUserSessions(ctx context.Context, userID string) error {
	// Get all user sessions
	sessions, err := sm.userStore.GetUserSessions(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user sessions: %w", err)
	}

	// Remove all sessions from database
	if err := sm.userStore.DeleteAllUserSessions(ctx, userID); err != nil {
		return fmt.Errorf("failed to delete all user sessions: %w", err)
	}

	// Publish events for each invalidated session
	for _, session := range sessions {
		event := UserSessionInvalidatedEvent{
			SessionID: session.SessionID,
			UserID:    &userID,
			Timestamp: time.Now(),
		}

		if err := sm.eventBus.PublishEvent(ctx, event); err != nil {
			// Log error but continue
			fmt.Printf("Failed to publish UserSessionInvalidatedEvent for session %s: %v\n", session.SessionID, err)
		}
	}

	return nil
}

// RefreshSession extends the expiration time of an existing session
func (sm *UserSessionManager) RefreshSession(ctx context.Context, w http.ResponseWriter, r *http.Request, sessionID string) error {
	// Get session from database
	userSession, err := sm.userStore.GetUserSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get user session: %w", err)
	}
	if userSession == nil {
		return fmt.Errorf("session not found")
	}

	// Update expiration time
	userSession.ExpiresAt = time.Now().Add(sm.sessionTimeout)

	// Update in database
	if err := sm.userStore.UpdateUserSession(ctx, userSession); err != nil {
		return fmt.Errorf("failed to update user session: %w", err)
	}

	// Update Gorilla session
	sessionName := session.GetSessionName()
	session, err := sm.store.Get(r, sessionName)
	if err == nil {
		session.Options.MaxAge = int(sm.sessionTimeout.Seconds())
		session.Save(r, w)
	}

	return nil
}

// GetActiveSessions returns all active sessions for a user
func (sm *UserSessionManager) GetActiveSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	return sm.userStore.GetActiveUserSessions(ctx, userID)
}

// enforceConcurrentSessionLimits removes oldest sessions if limit is exceeded
func (sm *UserSessionManager) enforceConcurrentSessionLimits(ctx context.Context, userID string) error {
	if sm.maxSessions <= 0 {
		return nil // No limit
	}

	// Get all active sessions for user
	sessions, err := sm.userStore.GetActiveUserSessions(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get active sessions: %w", err)
	}

	// If we're at or over the limit, remove oldest sessions
	if len(sessions) >= sm.maxSessions {
		// Sort by creation time (oldest first)
		// Note: sessions are already ordered by created_at DESC from the store
		// So we need to reverse the logic - remove from the end
		sessionsToRemove := len(sessions) - sm.maxSessions + 1

		for i := len(sessions) - sessionsToRemove; i < len(sessions); i++ {
			if err := sm.userStore.DeleteUserSession(ctx, sessions[i].SessionID); err != nil {
				return fmt.Errorf("failed to delete old session: %w", err)
			}

			// Publish session invalidated event
			event := UserSessionInvalidatedEvent{
				SessionID: sessions[i].SessionID,
				UserID:    &userID,
				Reason:    "concurrent_session_limit",
				Timestamp: time.Now(),
			}

			if err := sm.eventBus.PublishEvent(ctx, event); err != nil {
				fmt.Printf("Failed to publish UserSessionInvalidatedEvent: %v\n", err)
			}
		}
	}

	return nil
}

// startCleanupRoutine runs a background routine to clean up expired sessions
func (sm *UserSessionManager) startCleanupRoutine() {
	ticker := time.NewTicker(sm.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()
		if err := sm.cleanupExpiredSessions(ctx); err != nil {
			fmt.Printf("Failed to cleanup expired sessions: %v\n", err)
		}
	}
}

// cleanupExpiredSessions removes expired sessions from the database
func (sm *UserSessionManager) cleanupExpiredSessions(ctx context.Context) error {
	// Use the new DeleteExpiredSessions method for efficient cleanup
	deletedCount, err := sm.userStore.DeleteExpiredSessions(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete expired sessions: %w", err)
	}

	// Log cleanup activity if sessions were deleted
	if deletedCount > 0 {
		fmt.Printf("Cleaned up %d expired sessions\n", deletedCount)
	}

	return nil
}

// getClientIP extracts the client IP address from the request
func (sm *UserSessionManager) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies/load balancers)
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return forwarded
	}

	// Check X-Real-IP header
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// SessionMiddleware provides HTTP middleware for session validation
func (sm *UserSessionManager) SessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Validate session
		user, session, err := sm.ValidateSession(ctx, r)
		if err != nil {
			// Session is invalid - redirect to login or return 401
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Add user and session to request context
		ctx = context.WithValue(ctx, "user", user)
		ctx = context.WithValue(ctx, "session", session)

		// Optionally refresh session on each request
		if err := sm.RefreshSession(ctx, w, r, session.SessionID); err != nil {
			fmt.Printf("Failed to refresh session: %v\n", err)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserFromContext extracts the user from the request context
func GetUserFromContext(ctx context.Context) (*User, bool) {
	user, ok := ctx.Value("user").(*User)
	return user, ok
}

// GetSessionFromContext extracts the session from the request context
func GetSessionFromContext(ctx context.Context) (*UserSession, bool) {
	session, ok := ctx.Value("session").(*UserSession)
	return session, ok
}
