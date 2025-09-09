package users

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/shared"
	"github.com/mbolli/tickit/internal/sharedhttp"
)

// SessionIntegration provides a complete session management integration
// It bridges the gap between the existing sharedhttp session utilities
// and the new comprehensive session management system
type SessionIntegration struct {
	sessionManager *UserSessionManager
	sessionStore   sessions.Store
	config         *SessionConfig
}

// NewSessionIntegration creates a new session integration instance
func NewSessionIntegration(
	userStore UserStore,
	eventBus *shared.EventBusWrapper,
	config *SessionConfig,
) (*SessionIntegration, error) {
	if config == nil {
		config = LoadSessionConfigFromEnv()
	}

	// Validate configuration
	if err := config.ValidateConfig(); err != nil {
		return nil, fmt.Errorf("invalid session configuration: %w", err)
	}

	// Create session store
	sessionStore := config.CreateSessionStore()

	// Create session manager
	sessionManager := NewUserSessionManager(
		sessionStore,
		userStore,
		eventBus,
		config.ToUserSessionManagerConfig(),
	)

	return &SessionIntegration{
		sessionManager: sessionManager,
		sessionStore:   sessionStore,
		config:         config,
	}, nil
}

// GetSessionManager returns the session manager instance
func (si *SessionIntegration) GetSessionManager() *UserSessionManager {
	return si.sessionManager
}

// GetSessionStore returns the session store instance
func (si *SessionIntegration) GetSessionStore() sessions.Store {
	return si.sessionStore
}

// GetConfig returns the session configuration
func (si *SessionIntegration) GetConfig() *SessionConfig {
	return si.config
}

// CreateUserSession creates a new authenticated session for a user
// This is a convenience method that wraps the session manager
func (si *SessionIntegration) CreateUserSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID string) (*UserSession, error) {
	return si.sessionManager.CreateUserSession(ctx, w, r, userID)
}

// ValidateSession validates an existing session and returns user and session info
func (si *SessionIntegration) ValidateSession(ctx context.Context, r *http.Request) (*User, *UserSession, error) {
	return si.sessionManager.ValidateSession(ctx, r)
}

// InvalidateSession invalidates a specific session
func (si *SessionIntegration) InvalidateSession(ctx context.Context, w http.ResponseWriter, r *http.Request, sessionID string) error {
	return si.sessionManager.InvalidateSession(ctx, w, r, sessionID)
}

// InvalidateAllUserSessions invalidates all sessions for a specific user
func (si *SessionIntegration) InvalidateAllUserSessions(ctx context.Context, userID string) error {
	return si.sessionManager.InvalidateAllUserSessions(ctx, userID)
}

// GetActiveSessions returns all active sessions for a user
func (si *SessionIntegration) GetActiveSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	return si.sessionManager.GetActiveSessions(ctx, userID)
}

// SessionMiddleware provides HTTP middleware for session validation
func (si *SessionIntegration) SessionMiddleware(next http.Handler) http.Handler {
	return si.sessionManager.SessionMiddleware(next)
}

// RequireAuthenticationMiddleware provides middleware that requires authentication
// This is compatible with the existing sharedhttp.RequireAuthentication but uses
// the new session management system
func (si *SessionIntegration) RequireAuthenticationMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Validate session using the new session manager
		user, session, err := si.sessionManager.ValidateSession(ctx, r)
		if err != nil {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// Add user and session to request context
		ctx = context.WithValue(ctx, "user", user)
		ctx = context.WithValue(ctx, "session", session)
		ctx = context.WithValue(ctx, "user_id", user.ID) // For compatibility with existing code

		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// GetAuthenticatedUserID extracts the authenticated user ID from the request
// This provides compatibility with existing code that uses sharedhttp.GetUserIDFromContext
func (si *SessionIntegration) GetAuthenticatedUserID(r *http.Request) (string, bool) {
	ctx := r.Context()

	// Try to get from the new session system first
	if user, ok := GetUserFromContext(ctx); ok {
		return user.ID, true
	}

	// Fall back to the existing method for compatibility
	return sharedhttp.GetUserIDFromContext(ctx)
}

// SetAuthenticatedUser sets the user as authenticated in the session
// This provides compatibility with existing code
func (si *SessionIntegration) SetAuthenticatedUser(w http.ResponseWriter, r *http.Request, userID string) error {
	ctx := r.Context()

	// Create a new session using the session manager
	_, err := si.sessionManager.CreateUserSession(ctx, w, r, userID)
	if err != nil {
		return fmt.Errorf("failed to create user session: %w", err)
	}

	// Also set in the legacy way for compatibility
	return sharedhttp.SetAuthenticatedUser(si.sessionStore, w, r, userID)
}

// ClearAuthenticatedUser removes authentication from the session
func (si *SessionIntegration) ClearAuthenticatedUser(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	// Get current session to invalidate it
	if session, ok := GetSessionFromContext(ctx); ok {
		if err := si.sessionManager.InvalidateSession(ctx, w, r, session.SessionID); err != nil {
			return fmt.Errorf("failed to invalidate session: %w", err)
		}
	}

	// Also clear the legacy way for compatibility
	return sharedhttp.ClearAuthenticatedUser(si.sessionStore, w, r)
}

// WithSessionAndSignals provides compatibility with existing datastar integration
func (si *SessionIntegration) WithSessionAndSignals(w http.ResponseWriter, r *http.Request, signals any, handler func(sessionID string, signals any)) {
	// Use the existing sharedhttp function for datastar compatibility
	sharedhttp.WithSessionAndSignals(si.sessionStore, w, r, signals, handler)
}

// UpsertSessionID ensures a session ID exists and returns it
// This provides compatibility with existing code
func (si *SessionIntegration) UpsertSessionID(r *http.Request, w http.ResponseWriter) (string, error) {
	return sharedhttp.UpsertSessionID(si.sessionStore, r, w)
}
