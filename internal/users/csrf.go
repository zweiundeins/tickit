package users

import (
	"net/http"

	"github.com/justinas/nosurf"
)

// CSRFConfig holds CSRF protection configuration
type CSRFConfig struct {
	Enabled   bool
	Secure    bool                                                       // Require HTTPS for CSRF cookies
	SameSite  http.SameSite                                              // SameSite setting for CSRF cookies
	MaxAge    int                                                        // Cookie max age in seconds
	Domain    string                                                     // Cookie domain
	Path      string                                                     // Cookie path
	HttpOnly  bool                                                       // HttpOnly flag for cookies
	ErrorFunc func(w http.ResponseWriter, r *http.Request, reason error) // Custom error handler
}

// DefaultCSRFConfig returns default CSRF configuration
func DefaultCSRFConfig() *CSRFConfig {
	return &CSRFConfig{
		Enabled:   true,
		Secure:    true, // Should be true in production with HTTPS
		SameSite:  http.SameSiteStrictMode,
		MaxAge:    3600, // 1 hour
		Domain:    "",   // Use default domain
		Path:      "/",
		HttpOnly:  true,
		ErrorFunc: defaultCSRFErrorHandler,
	}
}

// NewCSRFMiddleware creates a new CSRF protection middleware using justinas/nosurf
func NewCSRFMiddleware(config *CSRFConfig) func(http.Handler) http.Handler {
	if config == nil {
		config = DefaultCSRFConfig()
	}

	// If CSRF is disabled, return a no-op middleware
	if !config.Enabled {
		return func(next http.Handler) http.Handler {
			return next
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
func defaultCSRFErrorHandler(w http.ResponseWriter, r *http.Request, reason error) {
	// Log the CSRF failure for security monitoring
	// In a real application, you'd use your audit logger here

	// Return appropriate error response
	if isAjaxRequest(r) {
		// For AJAX requests, return JSON error
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "CSRF token validation failed", "code": "CSRF_FAILURE"}`))
	} else {
		// For regular requests, return HTML error or redirect
		http.Error(w, "CSRF token validation failed. Please refresh the page and try again. "+reason.Error(), http.StatusForbidden)
	}
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
