package users

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/sessions"
)

// SecurityMiddlewareConfig holds configuration for all security middleware
type SecurityMiddlewareConfig struct {
	Security *SecurityConfig
	Audit    *AuditLogger
}

// DefaultSecurityMiddlewareConfig returns default configuration
func DefaultSecurityMiddlewareConfig() *SecurityMiddlewareConfig {
	auditLogger := NewAuditLogger(true, "INFO")

	return &SecurityMiddlewareConfig{
		Security: DefaultSecurityConfig(),
		Audit:    auditLogger,
	}
}

// SecurityMiddlewareStack provides a complete security middleware stack
type SecurityMiddlewareStack struct {
	config          *SecurityMiddlewareConfig
	securityManager *SecurityManager
}

// NewSecurityMiddlewareStack creates a new security middleware stack
func NewSecurityMiddlewareStack(config *SecurityMiddlewareConfig, sessionStore sessions.Store) *SecurityMiddlewareStack {
	if config == nil {
		config = DefaultSecurityMiddlewareConfig()
	}

	stack := &SecurityMiddlewareStack{
		config:          config,
		securityManager: NewSecurityManager(config.Security, config.Audit),
	}

	return stack
}

// ApplySecurityMiddleware applies all security middleware to a handler
func (sms *SecurityMiddlewareStack) ApplySecurityMiddleware(handler http.HandlerFunc, authEndpoint bool) http.HandlerFunc {
	// Apply middleware in reverse order (last applied = first executed)

	// 1. Start with the original handler
	var h http.HandlerFunc = handler

	// 2. Apply input validation and sanitization
	h = sms.InputValidationMiddleware()(h)

	// 3. Apply audit logging
	h = sms.config.Audit.AuditMiddleware()(h)

	// 4. Apply brute force protection for auth endpoints
	if authEndpoint {
		h = sms.securityManager.BruteForceProtectionMiddleware()(h)
	}

	// 5. Apply rate limiting
	h = sms.securityManager.RateLimitMiddleware(authEndpoint)(h)

	// 6. Apply security headers
	return SecurityHeaders(h)
}

// InputValidationMiddleware provides comprehensive input validation
func (sms *SecurityMiddlewareStack) InputValidationMiddleware() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Validate request size
			maxRequestSize := int64(10 << 20) // 10MB default
			if err := ValidateContentLength(r, maxRequestSize); err != nil {
				sms.config.Audit.LogSecurityEvent(r.Context(), "REQUEST_TOO_LARGE", map[string]interface{}{
					"client_ip": GetClientIP(r),
					"endpoint":  r.URL.Path,
					"error":     err.Error(),
				})
				http.Error(w, "Request too large", http.StatusRequestEntityTooLarge)
				return
			}

			// Validate User-Agent
			if err := ValidateUserAgent(r.UserAgent()); err != nil {
				sms.config.Audit.LogSecurityEvent(r.Context(), "INVALID_USER_AGENT", map[string]interface{}{
					"client_ip":  GetClientIP(r),
					"user_agent": r.UserAgent(),
					"error":      err.Error(),
				})
				http.Error(w, "Invalid user agent", http.StatusBadRequest)
				return
			}

			// For POST/PUT requests, validate JSON input
			if r.Method == "POST" || r.Method == "PUT" {
				if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						http.Error(w, "Failed to read request body", http.StatusBadRequest)
						return
					}

					// Validate JSON input
					if err := ValidateJSONInput(body, int(maxRequestSize)); err != nil {
						sms.config.Audit.LogSecurityEvent(r.Context(), "INVALID_JSON_INPUT", map[string]interface{}{
							"client_ip": GetClientIP(r),
							"endpoint":  r.URL.Path,
							"error":     err.Error(),
						})
						http.Error(w, "Invalid JSON input", http.StatusBadRequest)
						return
					}

					// Create a new reader with the body content
					r.Body = io.NopCloser(strings.NewReader(string(body)))
				}
			}

			next.ServeHTTP(w, r)
		}
	}
}

// AuthenticationSecurityWrapper wraps authentication handlers with security measures
func (sms *SecurityMiddlewareStack) AuthenticationSecurityWrapper(handler http.HandlerFunc) http.HandlerFunc {
	return sms.ApplySecurityMiddleware(handler, true)
}

// StandardSecurityWrapper wraps standard handlers with security measures
func (sms *SecurityMiddlewareStack) StandardSecurityWrapper(handler http.HandlerFunc) http.HandlerFunc {
	return sms.ApplySecurityMiddleware(handler, false)
}

// RecordAuthenticationAttempt records authentication attempts for security monitoring
func (sms *SecurityMiddlewareStack) RecordAuthenticationAttempt(ctx context.Context, r *http.Request, email, userID string, success bool, method, message string) {
	clientIP := GetClientIP(r)

	if success {
		sms.securityManager.RecordSuccessfulLogin(ctx, clientIP, email, userID)
		sms.config.Audit.LogUserLogin(ctx, r, userID, email, method, true, message)
	} else {
		sms.securityManager.RecordFailedLogin(ctx, clientIP, email)
		sms.config.Audit.LogUserLogin(ctx, r, userID, email, method, false, message)
	}
}

// CheckBruteForceProtection checks if an IP or email is locked out
func (sms *SecurityMiddlewareStack) CheckBruteForceProtection(clientIP, email string) bool {
	ipLocked := sms.securityManager.bruteForceProtection.IsLocked(clientIP)
	emailLocked := false

	if email != "" {
		emailLocked = sms.securityManager.IsEmailLocked(email)
	}

	return ipLocked || emailLocked
}

// ValidateAndSanitizeAuthInput validates and sanitizes authentication input
func (sms *SecurityMiddlewareStack) ValidateAndSanitizeAuthInput(input map[string]interface{}) (map[string]string, error) {
	result := make(map[string]string)

	for key, value := range input {
		strValue, ok := value.(string)
		if !ok {
			continue
		}

		// Sanitize the input
		sanitized := SanitizeString(strValue)

		// Apply field-specific validation
		switch key {
		case "email":
			if err := ValidateEmail(sanitized); err != nil {
				return nil, err
			}
		case "password":
			if err := ValidatePassword(sanitized); err != nil {
				return nil, err
			}
		case "username":
			if err := ValidateUsername(sanitized); err != nil {
				return nil, err
			}
		case "first_name", "last_name":
			if err := ValidateName(sanitized, key); err != nil {
				return nil, err
			}
		case "avatar_url":
			if sanitized != "" {
				var err error
				sanitized, err = SanitizeURL(sanitized)
				if err != nil {
					return nil, err
				}
			}
		default:
			// For other fields, apply comprehensive validation
			options := ValidationOptions{
				Required:             false,
				MinLength:            0,
				MaxLength:            500,
				CheckSQLInjection:    true,
				CheckScriptInjection: true,
				MaxRepeatedChars:     10,
			}
			if err := ComprehensiveInputValidation(sanitized, key, options); err != nil {
				return nil, err
			}
		}

		result[key] = sanitized
	}

	return result, nil
}

// LogSecurityEvent logs a security event
func (sms *SecurityMiddlewareStack) LogSecurityEvent(ctx context.Context, eventType string, details map[string]interface{}) {
	sms.config.Audit.LogSecurityEvent(ctx, eventType, details)
}

// Enhanced request parsing with security validation
func (sms *SecurityMiddlewareStack) ParseAndValidateJSONRequest(r *http.Request, target interface{}) error {
	// Limit request body size
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20) // 1MB limit

	// Read and validate body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}

	// Validate JSON input
	if err := ValidateJSONInput(body, 1<<20); err != nil {
		return err
	}

	// Parse JSON
	if err := json.Unmarshal(body, target); err != nil {
		return err
	}

	return nil
}

// SecurityContextMiddleware adds security context to requests
func (sms *SecurityMiddlewareStack) SecurityContextMiddleware() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Add security context
			ctx := context.WithValue(r.Context(), "security_manager", sms.securityManager)
			ctx = context.WithValue(ctx, "audit_logger", sms.config.Audit)
			ctx = context.WithValue(ctx, "client_ip", GetClientIP(r))

			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

// GetSecurityManagerFromContext extracts security manager from context
func GetSecurityManagerFromContext(ctx context.Context) (*SecurityManager, bool) {
	sm, ok := ctx.Value("security_manager").(*SecurityManager)
	return sm, ok
}

// GetAuditLoggerFromContext extracts audit logger from context
func GetAuditLoggerFromContext(ctx context.Context) (*AuditLogger, bool) {
	al, ok := ctx.Value("audit_logger").(*AuditLogger)
	return al, ok
}

// GetClientIPFromContext extracts client IP from context
func GetClientIPFromContext(ctx context.Context) (string, bool) {
	ip, ok := ctx.Value("client_ip").(string)
	return ip, ok
}
