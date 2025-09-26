package users

import (
	"log"
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/justinas/nosurf"
	"github.com/mbolli/tickit/internal/sharedhttp"
)

// CSRFConfig holds CSRF protection configuration
type CSRFConfig struct {
	Enabled      bool
	Secure       bool                                                       // Require HTTPS for CSRF cookies
	SameSite     http.SameSite                                              // SameSite setting for CSRF cookies
	MaxAge       int                                                        // Cookie max age in seconds
	Domain       string                                                     // Cookie domain
	Path         string                                                     // Cookie path
	HttpOnly     bool                                                       // HttpOnly flag for cookies
	ErrorFunc    func(w http.ResponseWriter, r *http.Request, reason error) // Custom error handler
	SessionStore sessions.Store                                             // Session store for flash messages
}

// DefaultCSRFConfig returns default CSRF configuration
func DefaultCSRFConfig() *CSRFConfig {
	return &CSRFConfig{
		Enabled:      true,
		Secure:       true,                    // Temporarily disable for debugging
		SameSite:     http.SameSiteStrictMode, // Less restrictive for debugging
		MaxAge:       3600,                    // 1 hour
		Domain:       "",                      // Use default domain
		Path:         "/",
		HttpOnly:     true,
		ErrorFunc:    nil, // Will be set to defaultCSRFErrorHandler in NewCSRFMiddleware
		SessionStore: nil, // Will be set in NewCSRFMiddleware
	}
}

// NewCSRFMiddleware creates a new CSRF protection middleware using justinas/nosurf
func NewCSRFMiddleware(config *CSRFConfig, sessionStore sessions.Store) func(http.Handler) http.Handler {
	// If config is nil, create default config
	if config == nil {
		config = DefaultCSRFConfig()
	}

	// Set the session store
	config.SessionStore = sessionStore

	// If CSRF is disabled, return a no-op middleware
	if !config.Enabled {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	// If sessionStore is nil, we can't use flash messages, but CSRF can still work
	// Set default error handler if none provided
	if config.ErrorFunc == nil {
		if sessionStore != nil {
			config.ErrorFunc = func(w http.ResponseWriter, r *http.Request, reason error) {
				defaultCSRFErrorHandler(config.SessionStore, w, r, reason)
			}
		} else {
			// Fallback to simple error handler if no session store
			config.ErrorFunc = func(w http.ResponseWriter, r *http.Request, reason error) {
				if isAjaxRequest(r) {
					w.WriteHeader(http.StatusForbidden)
				} else {
					errorMsg := "CSRF token validation failed. Please refresh the page and try again."
					if reason != nil {
						errorMsg += " " + reason.Error()
					}
					http.Error(w, errorMsg, http.StatusForbidden)
				}
			}
		}
	}

	// The nosurf.CSRFHandler implements http.Handler and wraps the next handler internally
	// when we call nosurf.New(next), it returns a configured handler
	return func(next http.Handler) http.Handler {
		// Configure the handler to wrap the next handler
		wrappedHandler := nosurf.New(next)

		// Apply our configuration to the wrapped handler
		wrappedHandler.SetBaseCookie(http.Cookie{
			Name:     "tickit_" + nosurf.CookieName,
			MaxAge:   config.MaxAge,
			Domain:   config.Domain,
			Path:     config.Path,
			Secure:   config.Secure,
			HttpOnly: config.HttpOnly,
			SameSite: config.SameSite,
		})

		// Set custom error handler if provided
		if config.ErrorFunc != nil {
			wrappedHandler.SetFailureHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				config.ErrorFunc(w, r, nosurf.Reason(r))
			}))
		}

		// Set exemptions
		wrappedHandler.ExemptFunc(func(r *http.Request) bool {
			// Exempt API endpoints that use token-based authentication
			if isAPIEndpoint(r.URL.Path) {
				return true
			}

			// Exempt OAuth callback endpoints
			if isOAuthCallback(r.URL.Path) {
				return true
			}

			// Exempt WebAuthn endpoints (they have their own protection)
			if isWebAuthnEndpoint(r.URL.Path) {
				return true
			}

			// Exempt magic mail login (development only)
			if isMagicMailEndpoint(r.URL.Path) {
				return true
			}

			return false
		})

		return wrappedHandler
	}
}

// defaultCSRFErrorHandler is the default error handler for CSRF failures
func defaultCSRFErrorHandler(sessionStore sessions.Store, w http.ResponseWriter, r *http.Request, reason error) {
	// Log the CSRF failure for security monitoring
	log.Printf("CSRF validation failed - URL: %s, Method: %s", r.URL.Path, r.Method)

	// Return appropriate error response
	if isAjaxRequest(r) {
		// For AJAX requests, return JSON error
		w.WriteHeader(http.StatusForbidden)
	} else {
		// For regular requests, store error in flash message and redirect to login
		errorMsg := "CSRF token validation failed. Please refresh the page and try again."
		if reason != nil {
			errorMsg += " " + reason.Error()
		}

		// Store error message as flash message
		if err := sharedhttp.SetFlashMessage(sessionStore, w, r, sharedhttp.FlashError, errorMsg); err != nil {
			// If we can't set flash message, fall back to direct error
			http.Error(w, errorMsg, http.StatusForbidden)
			return
		}
		// Redirect to login page where the flash message will be displayed
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// isAPIEndpoint checks if the path is an API endpoint
func isAPIEndpoint(path string) bool {
	// API endpoints typically start with /api/
	return len(path) >= 5 && path[:5] == "/api/"
}

// isOAuthCallback checks if the path is an OAuth callback
func isOAuthCallback(path string) bool {
	return len(path) >= 15 && path[:15] == "/auth/oauth/" &&
		(len(path) > 15 && path[len(path)-9:] == "/callback")
}

// isWebAuthnEndpoint checks if the path is a WebAuthn endpoint
func isWebAuthnEndpoint(path string) bool {
	return len(path) >= 15 && path[:15] == "/auth/webauthn/"
}

// isMagicMailEndpoint checks if the path is a magic mail endpoint
func isMagicMailEndpoint(path string) bool {
	return path == "/auth/magic-mail/login"
}

// isAjaxRequest checks if the request is an AJAX request
func isAjaxRequest(r *http.Request) bool {
	return r.Header.Get("X-Requested-With") == "XMLHttpRequest" ||
		r.Header.Get("Content-Type") == "application/json" ||
		r.Header.Get("Accept") == "application/json"
}
