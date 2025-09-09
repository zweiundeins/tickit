package users

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// SecurityConfig holds security-related configuration
type SecurityConfig struct {
	// Rate limiting
	AuthRateLimit   int // requests per minute for auth endpoints
	AuthRateBurst   int // burst capacity for auth endpoints
	GlobalRateLimit int // requests per minute for all endpoints
	GlobalRateBurst int // burst capacity for all endpoints

	// Brute force protection
	MaxFailedAttempts   int           // max failed attempts before lockout
	LockoutDuration     time.Duration // how long to lock account
	FailedAttemptWindow time.Duration // time window for counting failed attempts

	// CSRF protection
	CSRFEnabled  bool          // enable CSRF protection
	CSRFSecure   bool          // require HTTPS for CSRF cookies
	CSRFSameSite http.SameSite // SameSite setting for CSRF cookies

	// Audit logging
	AuditLogEnabled bool   // enable audit logging
	AuditLogLevel   string // log level for audit events
}

// DefaultSecurityConfig returns default security configuration
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		AuthRateLimit:   30,  // 30 requests per minute for auth
		AuthRateBurst:   10,  // burst of 10
		GlobalRateLimit: 300, // 300 requests per minute globally
		GlobalRateBurst: 50,  // burst of 50

		MaxFailedAttempts:   5,
		LockoutDuration:     15 * time.Minute,
		FailedAttemptWindow: 15 * time.Minute,

		CSRFEnabled:  true,
		CSRFSecure:   true,
		CSRFSameSite: http.SameSiteStrictMode,

		AuditLogEnabled: true,
		AuditLogLevel:   "INFO",
	}
}

// RateLimiter provides rate limiting functionality
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rateLimit int, burst int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(rateLimit) / 60, // convert per minute to per second
		burst:    burst,
	}
}

// GetLimiter returns a rate limiter for the given key (usually IP address)
func (rl *RateLimiter) GetLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[key]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[key] = limiter
	}

	return limiter
}

// Allow checks if a request should be allowed
func (rl *RateLimiter) Allow(key string) bool {
	return rl.GetLimiter(key).Allow()
}

// CleanupOldLimiters removes unused limiters (should be called periodically)
func (rl *RateLimiter) CleanupOldLimiters() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Simple cleanup: remove all limiters
	// In production, you might want more sophisticated cleanup
	rl.limiters = make(map[string]*rate.Limiter)
}

// BruteForceProtection tracks failed login attempts
type BruteForceProtection struct {
	attempts map[string]*FailedAttempts
	mu       sync.RWMutex
	config   *SecurityConfig
}

// FailedAttempts tracks failed attempts for an IP or user
type FailedAttempts struct {
	Count       int
	FirstFail   time.Time
	LastFail    time.Time
	LockedUntil time.Time
}

// NewBruteForceProtection creates a new brute force protection instance
func NewBruteForceProtection(config *SecurityConfig) *BruteForceProtection {
	return &BruteForceProtection{
		attempts: make(map[string]*FailedAttempts),
		config:   config,
	}
}

// RecordFailedAttempt records a failed login attempt
func (bfp *BruteForceProtection) RecordFailedAttempt(key string) {
	bfp.mu.Lock()
	defer bfp.mu.Unlock()

	now := time.Now()
	attempt, exists := bfp.attempts[key]

	if !exists {
		bfp.attempts[key] = &FailedAttempts{
			Count:     1,
			FirstFail: now,
			LastFail:  now,
		}
		return
	}

	// Reset count if outside the window
	if now.Sub(attempt.FirstFail) > bfp.config.FailedAttemptWindow {
		attempt.Count = 1
		attempt.FirstFail = now
	} else {
		attempt.Count++
	}

	attempt.LastFail = now

	// Lock if exceeded max attempts
	if attempt.Count >= bfp.config.MaxFailedAttempts {
		attempt.LockedUntil = now.Add(bfp.config.LockoutDuration)
	}
}

// IsLocked checks if an IP or user is currently locked out
func (bfp *BruteForceProtection) IsLocked(key string) bool {
	bfp.mu.RLock()
	defer bfp.mu.RUnlock()

	attempt, exists := bfp.attempts[key]
	if !exists {
		return false
	}

	return time.Now().Before(attempt.LockedUntil)
}

// ClearFailedAttempts clears failed attempts for a key (on successful login)
func (bfp *BruteForceProtection) ClearFailedAttempts(key string) {
	bfp.mu.Lock()
	defer bfp.mu.Unlock()

	delete(bfp.attempts, key)
}

// GetFailedAttempts returns the current failed attempt count
func (bfp *BruteForceProtection) GetFailedAttempts(key string) int {
	bfp.mu.RLock()
	defer bfp.mu.RUnlock()

	attempt, exists := bfp.attempts[key]
	if !exists {
		return 0
	}

	// Reset if outside window
	if time.Now().Sub(attempt.FirstFail) > bfp.config.FailedAttemptWindow {
		return 0
	}

	return attempt.Count
}

// CleanupOldAttempts removes old failed attempts (should be called periodically)
func (bfp *BruteForceProtection) CleanupOldAttempts() {
	bfp.mu.Lock()
	defer bfp.mu.Unlock()

	now := time.Now()
	for key, attempt := range bfp.attempts {
		// Remove if outside window and not locked
		if now.Sub(attempt.FirstFail) > bfp.config.FailedAttemptWindow &&
			now.After(attempt.LockedUntil) {
			delete(bfp.attempts, key)
		}
	}
}

// SecurityManager coordinates all security measures
type SecurityManager struct {
	config               *SecurityConfig
	authRateLimiter      *RateLimiter
	globalRateLimiter    *RateLimiter
	bruteForceProtection *BruteForceProtection
	auditLogger          *AuditLogger
}

// NewSecurityManager creates a new security manager
func NewSecurityManager(config *SecurityConfig, auditLogger *AuditLogger) *SecurityManager {
	if config == nil {
		config = DefaultSecurityConfig()
	}

	sm := &SecurityManager{
		config:               config,
		authRateLimiter:      NewRateLimiter(config.AuthRateLimit, config.AuthRateBurst),
		globalRateLimiter:    NewRateLimiter(config.GlobalRateLimit, config.GlobalRateBurst),
		bruteForceProtection: NewBruteForceProtection(config),
		auditLogger:          auditLogger,
	}

	// Start cleanup routine
	go sm.startCleanupRoutine()

	return sm
}

// startCleanupRoutine starts a goroutine to periodically clean up old data
func (sm *SecurityManager) startCleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		sm.authRateLimiter.CleanupOldLimiters()
		sm.globalRateLimiter.CleanupOldLimiters()
		sm.bruteForceProtection.CleanupOldAttempts()
	}
}

// GetClientIP extracts the real client IP from the request
func GetClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if net.ParseIP(xri) != nil {
			return xri
		}
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RateLimitMiddleware provides rate limiting middleware
func (sm *SecurityManager) RateLimitMiddleware(authEndpoint bool) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			clientIP := GetClientIP(r)

			// Check global rate limit first
			if !sm.globalRateLimiter.Allow(clientIP) {
				sm.auditLogger.LogSecurityEvent(r.Context(), "RATE_LIMIT_EXCEEDED", map[string]interface{}{
					"client_ip": clientIP,
					"endpoint":  r.URL.Path,
					"type":      "global",
				})
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			// Check auth-specific rate limit for auth endpoints
			if authEndpoint && !sm.authRateLimiter.Allow(clientIP) {
				sm.auditLogger.LogSecurityEvent(r.Context(), "AUTH_RATE_LIMIT_EXCEEDED", map[string]interface{}{
					"client_ip": clientIP,
					"endpoint":  r.URL.Path,
					"type":      "auth",
				})
				http.Error(w, "Authentication rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		}
	}
}

// BruteForceProtectionMiddleware provides brute force protection
func (sm *SecurityManager) BruteForceProtectionMiddleware() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			clientIP := GetClientIP(r)

			// Check if IP is locked out
			if sm.bruteForceProtection.IsLocked(clientIP) {
				sm.auditLogger.LogSecurityEvent(r.Context(), "BRUTE_FORCE_LOCKOUT", map[string]interface{}{
					"client_ip": clientIP,
					"endpoint":  r.URL.Path,
				})
				http.Error(w, "Account temporarily locked due to too many failed attempts", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		}
	}
}

// RecordFailedLogin records a failed login attempt
func (sm *SecurityManager) RecordFailedLogin(ctx context.Context, clientIP, email string) {
	// Record by IP
	sm.bruteForceProtection.RecordFailedAttempt(clientIP)

	// Also record by email if provided
	if email != "" {
		sm.bruteForceProtection.RecordFailedAttempt("email:" + email)
	}

	sm.auditLogger.LogSecurityEvent(ctx, "LOGIN_FAILED", map[string]interface{}{
		"client_ip": clientIP,
		"email":     email,
		"attempts":  sm.bruteForceProtection.GetFailedAttempts(clientIP),
	})
}

// RecordSuccessfulLogin clears failed attempts and logs success
func (sm *SecurityManager) RecordSuccessfulLogin(ctx context.Context, clientIP, email, userID string) {
	// Clear failed attempts
	sm.bruteForceProtection.ClearFailedAttempts(clientIP)
	if email != "" {
		sm.bruteForceProtection.ClearFailedAttempts("email:" + email)
	}

	sm.auditLogger.LogSecurityEvent(ctx, "LOGIN_SUCCESS", map[string]interface{}{
		"client_ip": clientIP,
		"email":     email,
		"user_id":   userID,
	})
}

// IsEmailLocked checks if an email is locked due to failed attempts
func (sm *SecurityManager) IsEmailLocked(email string) bool {
	return sm.bruteForceProtection.IsLocked("email:" + email)
}

// SanitizeInput sanitizes user input to prevent XSS and other attacks
func SanitizeInput(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	// Trim whitespace
	input = strings.TrimSpace(input)

	// Remove control characters except newlines and tabs
	var result strings.Builder
	for _, r := range input {
		if r >= 32 || r == '\n' || r == '\t' {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// ValidateContentLength validates that content length is within limits
func ValidateContentLength(r *http.Request, maxSize int64) error {
	if r.ContentLength > maxSize {
		return fmt.Errorf("request body too large: %d bytes (max: %d)", r.ContentLength, maxSize)
	}

	// Also check Content-Length header
	if contentLength := r.Header.Get("Content-Length"); contentLength != "" {
		if size, err := strconv.ParseInt(contentLength, 10, 64); err == nil && size > maxSize {
			return fmt.Errorf("content-length header too large: %d bytes (max: %d)", size, maxSize)
		}
	}

	return nil
}

// SecurityHeaders adds security headers to responses
func SecurityHeaders(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Enable XSS protection
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Strict transport security (only if HTTPS)
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// Content Security Policy
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self'")

		// Referrer policy
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		next.ServeHTTP(w, r)
	}
}
