package users

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// AuditEvent represents a security or authentication event
type AuditEvent struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	EventType string                 `json:"event_type"`
	UserID    string                 `json:"user_id,omitempty"`
	Email     string                 `json:"email,omitempty"`
	ClientIP  string                 `json:"client_ip,omitempty"`
	UserAgent string                 `json:"user_agent,omitempty"`
	Endpoint  string                 `json:"endpoint,omitempty"`
	Method    string                 `json:"method,omitempty"`
	Success   bool                   `json:"success"`
	Message   string                 `json:"message,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Risk      string                 `json:"risk,omitempty"` // LOW, MEDIUM, HIGH, CRITICAL
}

// AuditLogger handles audit logging for security events
type AuditLogger struct {
	enabled bool
	level   string
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(enabled bool, level string) *AuditLogger {
	return &AuditLogger{
		enabled: enabled,
		level:   level,
	}
}

// LogSecurityEvent logs a security-related event
func (al *AuditLogger) LogSecurityEvent(ctx context.Context, eventType string, details map[string]interface{}) {
	if !al.enabled {
		return
	}

	event := &AuditEvent{
		ID:        generateEventID(),
		Timestamp: time.Now().UTC(),
		EventType: eventType,
		Details:   details,
		Risk:      al.determineRiskLevel(eventType),
	}

	// Extract common fields from details
	if clientIP, ok := details["client_ip"].(string); ok {
		event.ClientIP = clientIP
	}
	if userID, ok := details["user_id"].(string); ok {
		event.UserID = userID
	}
	if email, ok := details["email"].(string); ok {
		event.Email = email
	}
	if endpoint, ok := details["endpoint"].(string); ok {
		event.Endpoint = endpoint
	}
	if method, ok := details["method"].(string); ok {
		event.Method = method
	}
	if success, ok := details["success"].(bool); ok {
		event.Success = success
	}
	if message, ok := details["message"].(string); ok {
		event.Message = message
	}

	al.writeAuditLog(event)
}

// LogAuthenticationEvent logs authentication-related events
func (al *AuditLogger) LogAuthenticationEvent(ctx context.Context, r *http.Request, eventType string, userID, email string, success bool, details map[string]interface{}) {
	if !al.enabled {
		return
	}

	if details == nil {
		details = make(map[string]interface{})
	}

	// Add request context
	details["client_ip"] = GetClientIP(r)
	details["user_agent"] = r.UserAgent()
	details["endpoint"] = r.URL.Path
	details["method"] = r.Method
	details["success"] = success
	details["user_id"] = userID
	details["email"] = email

	al.LogSecurityEvent(ctx, eventType, details)
}

// LogUserRegistration logs user registration events
func (al *AuditLogger) LogUserRegistration(ctx context.Context, r *http.Request, userID, email string, success bool, message string) {
	details := map[string]interface{}{
		"message": message,
	}
	al.LogAuthenticationEvent(ctx, r, "USER_REGISTRATION", userID, email, success, details)
}

// LogUserLogin logs user login events
func (al *AuditLogger) LogUserLogin(ctx context.Context, r *http.Request, userID, email, method string, success bool, message string) {
	details := map[string]interface{}{
		"auth_method": method,
		"message":     message,
	}
	al.LogAuthenticationEvent(ctx, r, "USER_LOGIN", userID, email, success, details)
}

// LogUserLogout logs user logout events
func (al *AuditLogger) LogUserLogout(ctx context.Context, r *http.Request, userID, email string, success bool, message string) {
	details := map[string]interface{}{
		"message": message,
	}
	al.LogAuthenticationEvent(ctx, r, "USER_LOGOUT", userID, email, success, details)
}

// LogPasswordChange logs password change events
func (al *AuditLogger) LogPasswordChange(ctx context.Context, r *http.Request, userID, email string, success bool, message string) {
	details := map[string]interface{}{
		"message": message,
	}
	al.LogAuthenticationEvent(ctx, r, "PASSWORD_CHANGE", userID, email, success, details)
}

// LogPasswordReset logs password reset events
func (al *AuditLogger) LogPasswordReset(ctx context.Context, r *http.Request, userID, email string, success bool, message string) {
	details := map[string]interface{}{
		"message": message,
	}
	al.LogAuthenticationEvent(ctx, r, "PASSWORD_RESET", userID, email, success, details)
}

// LogMFAEvent logs multi-factor authentication events
func (al *AuditLogger) LogMFAEvent(ctx context.Context, r *http.Request, userID, email, action string, success bool, message string) {
	details := map[string]interface{}{
		"mfa_action": action, // "enable", "disable", "authenticate"
		"message":    message,
	}
	al.LogAuthenticationEvent(ctx, r, "MFA_EVENT", userID, email, success, details)
}

// LogOAuthEvent logs OAuth-related events
func (al *AuditLogger) LogOAuthEvent(ctx context.Context, r *http.Request, userID, email, provider, action string, success bool, message string) {
	details := map[string]interface{}{
		"oauth_provider": provider,
		"oauth_action":   action, // "login", "link", "unlink"
		"message":        message,
	}
	al.LogAuthenticationEvent(ctx, r, "OAUTH_EVENT", userID, email, success, details)
}

// LogProfileUpdate logs profile update events
func (al *AuditLogger) LogProfileUpdate(ctx context.Context, r *http.Request, userID, email string, fields []string, success bool, message string) {
	details := map[string]interface{}{
		"updated_fields": fields,
		"message":        message,
	}
	al.LogAuthenticationEvent(ctx, r, "PROFILE_UPDATE", userID, email, success, details)
}

// LogRoleChange logs role change events
func (al *AuditLogger) LogRoleChange(ctx context.Context, r *http.Request, targetUserID, targetEmail, adminUserID, oldRole, newRole string, success bool, message string) {
	details := map[string]interface{}{
		"target_user_id": targetUserID,
		"target_email":   targetEmail,
		"admin_user_id":  adminUserID,
		"old_role":       oldRole,
		"new_role":       newRole,
		"message":        message,
	}
	al.LogAuthenticationEvent(ctx, r, "ROLE_CHANGE", adminUserID, "", success, details)
}

// LogAccountLockout logs account lockout events
func (al *AuditLogger) LogAccountLockout(ctx context.Context, r *http.Request, userID, email, reason string) {
	details := map[string]interface{}{
		"lockout_reason": reason,
		"message":        fmt.Sprintf("Account locked: %s", reason),
	}
	al.LogAuthenticationEvent(ctx, r, "ACCOUNT_LOCKOUT", userID, email, true, details)
}

// LogSuspiciousActivity logs suspicious activity
func (al *AuditLogger) LogSuspiciousActivity(ctx context.Context, r *http.Request, userID, email, activity, reason string) {
	details := map[string]interface{}{
		"suspicious_activity": activity,
		"reason":              reason,
		"message":             fmt.Sprintf("Suspicious activity detected: %s", activity),
	}
	al.LogAuthenticationEvent(ctx, r, "SUSPICIOUS_ACTIVITY", userID, email, false, details)
}

// LogDataAccess logs data access events
func (al *AuditLogger) LogDataAccess(ctx context.Context, r *http.Request, userID, email, resource, action string, success bool, message string) {
	details := map[string]interface{}{
		"resource": resource,
		"action":   action,
		"message":  message,
	}
	al.LogAuthenticationEvent(ctx, r, "DATA_ACCESS", userID, email, success, details)
}

// LogAdminAction logs administrative actions
func (al *AuditLogger) LogAdminAction(ctx context.Context, r *http.Request, adminUserID, targetUserID, action string, success bool, message string) {
	details := map[string]interface{}{
		"target_user_id": targetUserID,
		"admin_action":   action,
		"message":        message,
	}
	al.LogAuthenticationEvent(ctx, r, "ADMIN_ACTION", adminUserID, "", success, details)
}

// determineRiskLevel determines the risk level based on event type
func (al *AuditLogger) determineRiskLevel(eventType string) string {
	switch eventType {
	case "RATE_LIMIT_EXCEEDED", "AUTH_RATE_LIMIT_EXCEEDED", "BRUTE_FORCE_LOCKOUT":
		return "HIGH"
	case "LOGIN_FAILED", "SUSPICIOUS_ACTIVITY", "ACCOUNT_LOCKOUT":
		return "MEDIUM"
	case "PASSWORD_CHANGE", "PASSWORD_RESET", "MFA_EVENT", "ROLE_CHANGE":
		return "MEDIUM"
	case "USER_LOGIN", "USER_LOGOUT", "USER_REGISTRATION":
		return "LOW"
	case "PROFILE_UPDATE", "DATA_ACCESS":
		return "LOW"
	default:
		return "LOW"
	}
}

// writeAuditLog writes the audit event to the log
func (al *AuditLogger) writeAuditLog(event *AuditEvent) {
	// Convert to JSON for structured logging
	eventJSON, err := json.Marshal(event)
	if err != nil {
		log.Printf("ERROR: Failed to marshal audit event: %v", err)
		return
	}

	// Log with appropriate level based on risk
	switch event.Risk {
	case "CRITICAL":
		log.Printf("AUDIT [CRITICAL] %s", string(eventJSON))
	case "HIGH":
		log.Printf("AUDIT [HIGH] %s", string(eventJSON))
	case "MEDIUM":
		log.Printf("AUDIT [MEDIUM] %s", string(eventJSON))
	default:
		log.Printf("AUDIT [LOW] %s", string(eventJSON))
	}

	// In a production system, you might want to:
	// 1. Send to a centralized logging system (ELK, Splunk, etc.)
	// 2. Store in a database for querying
	// 3. Send alerts for high-risk events
	// 4. Integrate with SIEM systems
}

// generateEventID generates a unique event ID
func generateEventID() string {
	// Use timestamp + random component for uniqueness
	return fmt.Sprintf("audit_%d_%s", time.Now().UnixNano(), generateSecureToken(8))
}

// Helper function to generate secure tokens (reuse from existing code)
func generateSecureToken(length int) string {
	// This should use the same secure token generation as other parts of the system
	// For now, using a simple implementation
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// AuditMiddleware logs all requests for audit purposes
func (al *AuditLogger) AuditMiddleware() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !al.enabled {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()

			// Create a response writer wrapper to capture status code
			wrapper := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Process request
			next.ServeHTTP(wrapper, r)

			// Log the request
			duration := time.Since(start)

			details := map[string]interface{}{
				"client_ip":      GetClientIP(r),
				"user_agent":     r.UserAgent(),
				"endpoint":       r.URL.Path,
				"method":         r.Method,
				"status_code":    wrapper.statusCode,
				"duration_ms":    duration.Milliseconds(),
				"content_length": r.ContentLength,
			}

			// Add user context if available
			if userID, ok := r.Context().Value("user_id").(string); ok {
				details["user_id"] = userID
			}

			al.LogSecurityEvent(r.Context(), "HTTP_REQUEST", details)
		}
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
