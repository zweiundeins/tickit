package session

import (
	"github.com/mbolli/tickit/config"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/sessions"
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
		SessionSecret:   config.Global.SessionSecret,
		SessionTimeout:  24 * time.Hour,
		MaxAge:          int((24 * time.Hour).Seconds()),
		HttpOnly:        true,
		Secure:          true, // Set to true in production with HTTPS
		SameSite:        http.SameSiteStrictMode,
		MaxSessions:     0, // Unlimited by default
		CleanupInterval: time.Hour,
	}
}

// LoadSessionConfigFromEnv loads session configuration from environment variables
// This function is kept for backward compatibility but delegates to the session package
func LoadSessionConfigFromEnv() *SessionConfig {
	sessionConfig := DefaultSessionConfig()

	// Load from environment variables
	if sessionName := os.Getenv("SESSION_NAME"); sessionName != "" {
		sessionConfig.SessionName = sessionName
	}

	if sessionSecret := os.Getenv("SESSION_SECRET"); sessionSecret != "" {
		sessionConfig.SessionSecret = sessionSecret
	}

	if sessionTimeoutStr := os.Getenv("SESSION_TIMEOUT"); sessionTimeoutStr != "" {
		if timeout, err := time.ParseDuration(sessionTimeoutStr); err == nil {
			sessionConfig.SessionTimeout = timeout
			sessionConfig.MaxAge = int(timeout.Seconds())
		}
	}

	if httpOnlyStr := os.Getenv("SESSION_HTTP_ONLY"); httpOnlyStr != "" {
		if httpOnly, err := strconv.ParseBool(httpOnlyStr); err == nil {
			sessionConfig.HttpOnly = httpOnly
		}
	}

	if secureStr := os.Getenv("SESSION_SECURE"); secureStr != "" {
		if secure, err := strconv.ParseBool(secureStr); err == nil {
			sessionConfig.Secure = secure
		}
	}

	if maxSessionsStr := os.Getenv("SESSION_MAX_CONCURRENT"); maxSessionsStr != "" {
		if maxSessions, err := strconv.Atoi(maxSessionsStr); err == nil {
			sessionConfig.MaxSessions = maxSessions
		}
	}

	if cleanupIntervalStr := os.Getenv("SESSION_CLEANUP_INTERVAL"); cleanupIntervalStr != "" {
		if interval, err := time.ParseDuration(cleanupIntervalStr); err == nil {
			sessionConfig.CleanupInterval = interval
		}
	}

	// Convert from session package sessionConfig to users package sessionConfig
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

// GlobalSessionStore is the centralized session store instance
var GlobalSessionStore sessions.Store

// InitializeGlobalSessionStore initializes the global session store with environment configuration
func InitializeGlobalSessionStore() (sessions.Store, error) {
	config := LoadSessionConfigFromEnv()

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
	config := LoadSessionConfigFromEnv()
	return config.SessionName
}
