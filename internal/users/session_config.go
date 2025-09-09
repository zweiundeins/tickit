package users

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/session"
)

// SessionConfig holds configuration for session management
type SessionConfig struct {
	// Session store configuration
	SessionName    string        // Name of the session cookie
	SessionSecret  string        // Secret key for session encryption
	SessionTimeout time.Duration // Session timeout duration
	MaxAge         int           // Cookie max age in seconds
	HttpOnly       bool          // HttpOnly cookie flag
	Secure         bool          // Secure cookie flag (HTTPS only)
	SameSite       http.SameSite // SameSite cookie attribute

	// Session management configuration
	MaxSessions     int           // Maximum concurrent sessions per user (0 = unlimited)
	CleanupInterval time.Duration // How often to clean up expired sessions
}

// DefaultSessionConfig returns a default session configuration
func DefaultSessionConfig() *SessionConfig {
	return &SessionConfig{
		SessionName:     "tickit_session",
		SessionSecret:   "change-me-in-production", // Should be overridden
		SessionTimeout:  24 * time.Hour,
		MaxAge:          int((24 * time.Hour).Seconds()),
		HttpOnly:        true,
		Secure:          false, // Set to true in production with HTTPS
		SameSite:        http.SameSiteStrictMode,
		MaxSessions:     0, // Unlimited by default
		CleanupInterval: time.Hour,
	}
}

// LoadSessionConfigFromEnv loads session configuration from environment variables
// This function is kept for backward compatibility but delegates to the session package
func LoadSessionConfigFromEnv() *SessionConfig {
	sessionConfig := session.LoadSessionConfigFromEnv()

	// Convert from session package config to users package config
	return &SessionConfig{
		SessionName:     sessionConfig.SessionName,
		SessionSecret:   sessionConfig.SessionSecret,
		SessionTimeout:  sessionConfig.SessionTimeout,
		MaxAge:          sessionConfig.MaxAge,
		HttpOnly:        sessionConfig.HttpOnly,
		Secure:          sessionConfig.Secure,
		SameSite:        sessionConfig.SameSite,
		MaxSessions:     sessionConfig.MaxSessions,
		CleanupInterval: sessionConfig.CleanupInterval,
	}
}

// CreateSessionStore creates a Gorilla Sessions store with the given configuration
func (c *SessionConfig) CreateSessionStore() sessions.Store {
	store := sessions.NewCookieStore([]byte(c.SessionSecret))

	// Configure session options
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   c.MaxAge,
		HttpOnly: c.HttpOnly,
		Secure:   c.Secure,
		SameSite: c.SameSite,
	}

	return store
}

// ToUserSessionManagerConfig converts SessionConfig to UserSessionManagerConfig
func (c *SessionConfig) ToUserSessionManagerConfig() *UserSessionManagerConfig {
	return &UserSessionManagerConfig{
		SessionTimeout:  c.SessionTimeout,
		MaxSessions:     c.MaxSessions,
		CleanupInterval: c.CleanupInterval,
	}
}

// ValidateConfig validates the session configuration
func (c *SessionConfig) ValidateConfig() error {
	if c.SessionSecret == "change-me-in-production" {
		// In production, this should be a strong random key
		// For development, we'll allow it but log a warning
		// In a real implementation, you might want to generate a random key
	}

	if c.SessionTimeout <= 0 {
		c.SessionTimeout = 24 * time.Hour
	}

	if c.CleanupInterval <= 0 {
		c.CleanupInterval = time.Hour
	}

	if c.MaxSessions < 0 {
		c.MaxSessions = 0 // Unlimited
	}

	return nil
}

// GlobalSessionStore is the centralized session store instance
var GlobalSessionStore sessions.Store

// InitializeGlobalSessionStore initializes the global session store with environment configuration
func InitializeGlobalSessionStore() (sessions.Store, error) {
	config := LoadSessionConfigFromEnv()

	// Validate configuration
	if err := config.ValidateConfig(); err != nil {
		return nil, fmt.Errorf("invalid session configuration: %w", err)
	}

	// Create the session store
	GlobalSessionStore = config.CreateSessionStore()
	return GlobalSessionStore, nil
}

// GetGlobalSessionStore returns the global session store, initializing it if necessary
func GetGlobalSessionStore() (sessions.Store, error) {
	if GlobalSessionStore == nil {
		return InitializeGlobalSessionStore()
	}
	return GlobalSessionStore, nil
}

// CreateTestSessionStore creates a session store for testing with a test-specific configuration
func CreateTestSessionStore(secret string) sessions.Store {
	if secret == "" {
		secret = "test-secret-key-32-bytes-long!"
	}

	config := &SessionConfig{
		SessionName:     "test_session",
		SessionSecret:   secret,
		SessionTimeout:  24 * time.Hour,
		MaxAge:          int((24 * time.Hour).Seconds()),
		HttpOnly:        true,
		Secure:          false, // Tests typically don't use HTTPS
		SameSite:        http.SameSiteLaxMode,
		MaxSessions:     0,
		CleanupInterval: time.Hour,
	}

	return config.CreateSessionStore()
}

// GetSessionName returns the configured session name, loading from environment if needed
// This function is kept for backward compatibility but delegates to the session package
func GetSessionName() string {
	return session.GetSessionName()
}
