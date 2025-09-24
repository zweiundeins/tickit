package users

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecurityConfig(t *testing.T) {
	config := DefaultSecurityConfig()

	if config.AuthRateLimit != 30 {
		t.Errorf("Expected AuthRateLimit to be 30, got %d", config.AuthRateLimit)
	}

	if config.MaxFailedAttempts != 5 {
		t.Errorf("Expected MaxFailedAttempts to be 5, got %d", config.MaxFailedAttempts)
	}

	if !config.CSRFEnabled {
		t.Error("Expected CSRF to be enabled by default")
	}

	if !config.AuditLogEnabled {
		t.Error("Expected audit logging to be enabled by default")
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(60, 10) // 60 per minute, burst of 10

	// Test that initial requests are allowed
	for i := 0; i < 10; i++ {
		if !limiter.Allow("test-ip") {
			t.Errorf("Request %d should have been allowed", i+1)
		}
	}

	// Test that requests beyond burst are denied
	if limiter.Allow("test-ip") {
		t.Error("Request beyond burst should have been denied")
	}

	// Test different IP gets its own limiter
	if !limiter.Allow("different-ip") {
		t.Error("Different IP should have its own rate limit")
	}
}

func TestBruteForceProtection(t *testing.T) {
	config := DefaultSecurityConfig()
	bfp := NewBruteForceProtection(config)

	testKey := "test-ip"

	// Test that initial attempts are not locked
	if bfp.IsLocked(testKey) {
		t.Error("Should not be locked initially")
	}

	// Record failed attempts up to the limit
	for i := 0; i < config.MaxFailedAttempts; i++ {
		bfp.RecordFailedAttempt(testKey)
	}

	// Should now be locked
	if !bfp.IsLocked(testKey) {
		t.Error("Should be locked after max failed attempts")
	}

	// Test clearing attempts
	bfp.ClearFailedAttempts(testKey)
	if bfp.IsLocked(testKey) {
		t.Error("Should not be locked after clearing attempts")
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name          string
		remoteAddr    string
		xForwardedFor string
		xRealIP       string
		expectedIP    string
	}{
		{
			name:       "RemoteAddr only",
			remoteAddr: "192.168.1.1:12345",
			expectedIP: "192.168.1.1",
		},
		{
			name:          "X-Forwarded-For header",
			remoteAddr:    "10.0.0.1:12345",
			xForwardedFor: "203.0.113.1, 192.168.1.1",
			expectedIP:    "203.0.113.1",
		},
		{
			name:       "X-Real-IP header",
			remoteAddr: "10.0.0.1:12345",
			xRealIP:    "203.0.113.2",
			expectedIP: "203.0.113.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remoteAddr

			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}

			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}

			ip := GetClientIP(req)
			if ip != tt.expectedIP {
				t.Errorf("Expected IP %s, got %s", tt.expectedIP, ip)
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	expectedHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "1; mode=block",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}

	for header, expectedValue := range expectedHeaders {
		if got := w.Header().Get(header); got != expectedValue {
			t.Errorf("Expected %s header to be %s, got %s", header, expectedValue, got)
		}
	}

	// Check CSP header exists
	if csp := w.Header().Get("Content-Security-Policy"); csp == "" {
		t.Error("Expected Content-Security-Policy header to be set")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	config := &SecurityConfig{
		AuthRateLimit:   1, // Very low limit for testing
		AuthRateBurst:   1,
		GlobalRateLimit: 10,
		GlobalRateBurst: 5,
	}

	auditLogger := NewAuditLogger(false, "INFO") // Disable for testing
	sm := NewSecurityManager(config, auditLogger)

	middleware := sm.RateLimitMiddleware(true)

	handler := middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/auth/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	// First request should succeed
	w1 := httptest.NewRecorder()
	handler(w1, req)
	if w1.Code != http.StatusOK {
		t.Errorf("First request should succeed, got status %d", w1.Code)
	}

	// Second request should be rate limited
	w2 := httptest.NewRecorder()
	handler(w2, req)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("Second request should be rate limited, got status %d", w2.Code)
	}
}

func TestBruteForceProtectionMiddleware(t *testing.T) {
	config := DefaultSecurityConfig()
	auditLogger := NewAuditLogger(false, "INFO") // Disable for testing
	sm := NewSecurityManager(config, auditLogger)

	// Simulate lockout
	testIP := "192.168.1.1"
	for i := 0; i < config.MaxFailedAttempts; i++ {
		sm.bruteForceProtection.RecordFailedAttempt(testIP)
	}

	middleware := sm.BruteForceProtectionMiddleware()

	handler := middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = testIP + ":12345"

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status %d for locked IP, got %d", http.StatusTooManyRequests, w.Code)
	}
}

func TestAuditLogger(t *testing.T) {
	logger := NewAuditLogger(true, "INFO")

	ctx := context.Background()
	details := map[string]interface{}{
		"client_ip": "192.168.1.1",
		"endpoint":  "/auth/login",
		"success":   false,
	}

	// This should not panic
	logger.LogSecurityEvent(ctx, "LOGIN_FAILED", details)

	// Test with disabled logger
	disabledLogger := NewAuditLogger(false, "INFO")
	disabledLogger.LogSecurityEvent(ctx, "LOGIN_FAILED", details)
}

func TestInputValidation(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "Normal input",
			input:       "normal@example.com",
			expectError: false,
		},
		{
			name:        "SQL injection attempt",
			input:       "test'; DROP TABLE users; --",
			expectError: true,
		},
		{
			name:        "Script injection attempt",
			input:       "<script>alert('xss')</script>",
			expectError: true,
		},
		{
			name:        "Null byte injection",
			input:       "test\x00malicious",
			expectError: false, // Should be sanitized, not error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test SQL injection validation
			err := ValidateNoSQLInjection(tt.input)
			if tt.expectError && err == nil {
				t.Error("Expected SQL injection validation to fail")
			}
			if !tt.expectError && err != nil && strings.Contains(tt.input, "'") {
				t.Errorf("Expected SQL injection validation to pass, got: %v", err)
			}

			// Test script injection validation
			err = ValidateNoScriptInjection(tt.input)
			if tt.expectError && strings.Contains(tt.input, "<script") && err == nil {
				t.Error("Expected script injection validation to fail")
			}

			// Test sanitization
			sanitized := SanitizeString(tt.input)
			if strings.Contains(sanitized, "\x00") {
				t.Error("Sanitization should remove null bytes")
			}
		})
	}
}

func TestComprehensiveInputValidation(t *testing.T) {
	options := ValidationOptions{
		Required:             true,
		MinLength:            3,
		MaxLength:            50,
		CheckSQLInjection:    true,
		CheckScriptInjection: true,
		MaxRepeatedChars:     3,
	}

	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "Valid input",
			input:       "validinput123",
			expectError: false,
		},
		{
			name:        "Too short",
			input:       "ab",
			expectError: true,
		},
		{
			name:        "Too many repeated chars",
			input:       "aaaaa",
			expectError: true,
		},
		{
			name:        "Empty required field",
			input:       "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ComprehensiveInputValidation(tt.input, "test_field", options)
			if tt.expectError && err == nil {
				t.Error("Expected validation to fail")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected validation to pass, got: %v", err)
			}
		})
	}
}

func TestSecurityInjectionPrevention(t *testing.T) {
	mockStore := NewMockSessionStore()
	config := DefaultSecurityMiddlewareConfig()
	stack := NewSecurityMiddlewareStack(config, mockStore)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	// Test auth endpoint wrapper
	wrappedHandler := stack.ApplySecurityMiddleware(handler, true)

	// Test various injection attempts
	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{"SQL Injection", "'; DROP TABLE users; --", false},
		{"XSS Script", "<script>alert('xss')</script>", false},
		{"Normal Input", "normal@email.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/test", strings.NewReader(fmt.Sprintf(`{"email":"%s"}`, tt.input)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(w, req)

			if tt.expect {
				assert.Equal(t, http.StatusOK, w.Code)
			} else {
				assert.NotEqual(t, http.StatusOK, w.Code)
			}
		})
	}
}

func TestValidateAndSanitizeAuthInput(t *testing.T) {
	mockStore := NewMockSessionStore()
	config := DefaultSecurityMiddlewareConfig()
	stack := NewSecurityMiddlewareStack(config, mockStore)

	input := map[string]interface{}{
		"email":      "test@example.com",
		"password":   "ValidPass123",
		"username":   "testuser",
		"first_name": "John",
		"last_name":  "Doe",
	}

	result, err := stack.ValidateAndSanitizeAuthInput(input)
	if err != nil {
		t.Errorf("Expected validation to pass, got: %v", err)
	}

	if result["email"] != "test@example.com" {
		t.Error("Email should be preserved")
	}

	// Test with invalid input
	invalidInput := map[string]interface{}{
		"email": "invalid-email",
	}

	_, err = stack.ValidateAndSanitizeAuthInput(invalidInput)
	if err == nil {
		t.Error("Expected validation to fail for invalid email")
	}
}

func TestBruteForceProtectionIntegration(t *testing.T) {
	mockStore := NewMockSessionStore()
	config := DefaultSecurityMiddlewareConfig()
	stack := NewSecurityMiddlewareStack(config, mockStore)

	clientIP := "192.168.1.1"
	email := "test@example.com"

	// Initially should not be locked
	if stack.CheckBruteForceProtection(clientIP, email) {
		t.Error("Should not be locked initially")
	}

	// Simulate failed attempts
	ctx := context.Background()
	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = clientIP + ":12345"

	for i := 0; i < config.Security.MaxFailedAttempts; i++ {
		stack.RecordAuthenticationAttempt(ctx, req, email, "", false, "password", "Invalid credentials")
	}

	// Should now be locked
	if !stack.CheckBruteForceProtection(clientIP, email) {
		t.Error("Should be locked after max failed attempts")
	}

	// Successful login should clear lockout
	stack.RecordAuthenticationAttempt(ctx, req, email, "user123", true, "password", "Login successful")

	if stack.CheckBruteForceProtection(clientIP, email) {
		t.Error("Should not be locked after successful login")
	}
}
