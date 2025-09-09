package session

import (
	"os"
	"strconv"
	"time"

	"net/http"
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
func LoadSessionConfigFromEnv() *SessionConfig {
	config := DefaultSessionConfig()

	// Load from environment variables
	if sessionName := os.Getenv("SESSION_NAME"); sessionName != "" {
		config.SessionName = sessionName
	}

	if sessionSecret := os.Getenv("SESSION_SECRET"); sessionSecret != "" {
		config.SessionSecret = sessionSecret
	}

	if sessionTimeoutStr := os.Getenv("SESSION_TIMEOUT"); sessionTimeoutStr != "" {
		if timeout, err := time.ParseDuration(sessionTimeoutStr); err == nil {
			config.SessionTimeout = timeout
			config.MaxAge = int(timeout.Seconds())
		}
	}

	if httpOnlyStr := os.Getenv("SESSION_HTTP_ONLY"); httpOnlyStr != "" {
		if httpOnly, err := strconv.ParseBool(httpOnlyStr); err == nil {
			config.HttpOnly = httpOnly
		}
	}

	if secureStr := os.Getenv("SESSION_SECURE"); secureStr != "" {
		if secure, err := strconv.ParseBool(secureStr); err == nil {
			config.Secure = secure
		}
	}

	if maxSessionsStr := os.Getenv("SESSION_MAX_CONCURRENT"); maxSessionsStr != "" {
		if maxSessions, err := strconv.Atoi(maxSessionsStr); err == nil {
			config.MaxSessions = maxSessions
		}
	}

	if cleanupIntervalStr := os.Getenv("SESSION_CLEANUP_INTERVAL"); cleanupIntervalStr != "" {
		if interval, err := time.ParseDuration(cleanupIntervalStr); err == nil {
			config.CleanupInterval = interval
		}
	}

	return config
}

// GetSessionName returns the configured session name, loading from environment if needed
func GetSessionName() string {
	config := LoadSessionConfigFromEnv()
	return config.SessionName
}
