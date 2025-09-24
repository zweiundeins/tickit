# Security Implementation for User Management

This document describes the comprehensive security measures implemented for the user management system in Tickit.

## Overview

The security implementation includes:

1. **Rate Limiting** - Prevents abuse of authentication endpoints
2. **Brute Force Protection** - Account lockout after failed attempts
3. **CSRF Protection** - Using justinas/nosurf middleware
4. **Audit Logging** - Comprehensive logging of all authentication events
5. **Input Validation and Sanitization** - Prevents injection attacks

## Components

### 1. Rate Limiting (`security.go`)

- **Global Rate Limiting**: 300 requests per minute per IP (configurable)
- **Authentication Rate Limiting**: 30 requests per minute per IP for auth endpoints
- **Burst Capacity**: Allows short bursts of requests
- **Per-IP Tracking**: Each IP address gets its own rate limiter
- **Automatic Cleanup**: Old rate limiters are cleaned up periodically

```go
// Example usage
rateLimiter := NewRateLimiter(60, 10) // 60 per minute, burst of 10
if !rateLimiter.Allow("192.168.1.1") {
    // Rate limit exceeded
}
```

### 2. Brute Force Protection (`security.go`)

- **Failed Attempt Tracking**: Tracks failed login attempts per IP and email
- **Account Lockout**: Locks accounts after 5 failed attempts (configurable)
- **Time Windows**: Failed attempts are counted within a 15-minute window
- **Lockout Duration**: Accounts are locked for 15 minutes (configurable)
- **Automatic Cleanup**: Old failed attempts are cleaned up periodically

```go
// Example usage
bfp := NewBruteForceProtection(config)
bfp.RecordFailedAttempt("192.168.1.1")
if bfp.IsLocked("192.168.1.1") {
    // IP is locked out
}
```

### 3. CSRF Protection (`csrf.go`)

- **justinas/nosurf Integration**: Uses the industry-standard CSRF library
- **Secure Cookies**: CSRF tokens stored in secure, HttpOnly cookies
- **SameSite Protection**: Cookies use SameSite=Strict for additional security
- **Selective Exemption**: API endpoints and OAuth callbacks are exempt
- **Custom Error Handling**: Provides user-friendly error messages

```go
// Example usage
csrfMiddleware := NewCSRFMiddleware(config)
router.Use(csrfMiddleware)
```

### 4. Audit Logging (`audit.go`)

- **Structured Logging**: All events logged in JSON format
- **Risk Classification**: Events classified as LOW, MEDIUM, HIGH, or CRITICAL
- **Comprehensive Coverage**: Logs authentication, authorization, and security events
- **Context Enrichment**: Includes IP address, user agent, and other context
- **Security Monitoring**: Enables detection of suspicious activities

```go
// Example usage
auditLogger := NewAuditLogger(true, "INFO")
auditLogger.LogUserLogin(ctx, r, userID, email, "password", true, "Login successful")
```

### 5. Input Validation and Sanitization (`validation.go`)

- **XSS Prevention**: HTML escaping and script injection detection
- **SQL Injection Prevention**: Pattern detection for malicious SQL
- **Input Sanitization**: Removes null bytes and control characters
- **Length Validation**: Enforces minimum and maximum field lengths
- **Format Validation**: Email, URL, and other format validation
- **Comprehensive Validation**: Configurable validation options

```go
// Example usage
sanitized := SanitizeString(userInput)
err := ValidateNoSQLInjection(sanitized)
err = ValidateNoScriptInjection(sanitized)
```

## Security Middleware Stack (`security_middleware.go`)

The `SecurityMiddlewareStack` combines all security measures into a cohesive system:

```go
// Create security stack
config := DefaultSecurityMiddlewareConfig()
stack := NewSecurityMiddlewareStack(config)

// Apply to authentication endpoints
handler := stack.AuthenticationSecurityWrapper(loginHandler)

// Apply to standard endpoints
handler := stack.StandardSecurityWrapper(profileHandler)
```

### Middleware Order

The middleware is applied in the following order (last applied = first executed):

1. **CSRF Protection** - Validates CSRF tokens
2. **Security Headers** - Adds security headers to responses
3. **Rate Limiting** - Enforces rate limits
4. **Brute Force Protection** - Checks for account lockouts
5. **Audit Logging** - Logs all requests
6. **Input Validation** - Validates and sanitizes input
7. **Handler Execution** - Executes the actual handler

## Secure Routes (`secure_routes.go`)

The `SecureUserRoutes` provides enhanced route handlers with integrated security:

- **Authentication Routes**: Login, register, logout with full security
- **Protected Routes**: Profile, admin functions with authentication checks
- **Role-Based Access**: Different security levels for different user roles
- **Comprehensive Logging**: All actions are logged for audit purposes

## Configuration

### Security Configuration

```go
config := &SecurityConfig{
    AuthRateLimit:       30,  // Auth requests per minute
    AuthRateBurst:       10,  // Auth burst capacity
    GlobalRateLimit:     300, // Global requests per minute
    GlobalRateBurst:     50,  // Global burst capacity
    
    MaxFailedAttempts:   5,                    // Failed attempts before lockout
    LockoutDuration:     15 * time.Minute,    // Lockout duration
    FailedAttemptWindow: 15 * time.Minute,    // Window for counting attempts
    
    CSRFEnabled:         true,                 // Enable CSRF protection
    CSRFSecure:          true,                 // Require HTTPS for CSRF cookies
    CSRFSameSite:        http.SameSiteStrictMode,
    
    AuditLogEnabled:     true,                 // Enable audit logging
    AuditLogLevel:       "INFO",               // Log level
}
```

### CSRF Configuration

```go
csrfConfig := &CSRFConfig{
    Enabled:   true,
    Secure:    true,                    // Require HTTPS
    SameSite:  http.SameSiteStrictMode, // Strict SameSite policy
    MaxAge:    3600,                    // 1 hour cookie lifetime
    HttpOnly:  true,                    // HttpOnly cookies
}
```

## Security Headers

The following security headers are automatically added to all responses:

- `X-Content-Type-Options: nosniff` - Prevents MIME type sniffing
- `X-Frame-Options: DENY` - Prevents clickjacking
- `X-XSS-Protection: 1; mode=block` - Enables XSS protection
- `Strict-Transport-Security` - Forces HTTPS (when using HTTPS)
- `Content-Security-Policy` - Restricts resource loading
- `Referrer-Policy: strict-origin-when-cross-origin` - Controls referrer information

## Testing

Comprehensive tests are provided in `security_test.go`:

- Rate limiting functionality
- Brute force protection
- Input validation and sanitization
- Security middleware integration
- CSRF protection
- Audit logging

Run tests with:
```bash
go test ./internal/users -run TestSecurity -v
```

## Best Practices

1. **Defense in Depth**: Multiple layers of security protection
2. **Fail Secure**: Default to denying access when in doubt
3. **Least Privilege**: Users get minimum necessary permissions
4. **Input Validation**: All user input is validated and sanitized
5. **Audit Everything**: All security-relevant events are logged
6. **Rate Limiting**: Prevents abuse and DoS attacks
7. **Secure Defaults**: Security features enabled by default

## Monitoring and Alerting

The audit logging system provides structured logs that can be integrated with:

- **SIEM Systems**: Security Information and Event Management
- **Log Aggregation**: ELK Stack, Splunk, etc.
- **Alerting Systems**: PagerDuty, Slack notifications
- **Metrics Collection**: Prometheus, Grafana

## Compliance

This implementation helps meet various compliance requirements:

- **OWASP Top 10**: Addresses common web application security risks
- **GDPR**: Audit logging for data access and modifications
- **SOC 2**: Security controls and monitoring
- **PCI DSS**: If handling payment data (additional measures may be needed)

## Future Enhancements

Potential future security enhancements:

1. **Behavioral Analysis**: Detect unusual user behavior patterns
2. **Device Fingerprinting**: Track and validate known devices
3. **Geolocation Validation**: Alert on logins from unusual locations
4. **Advanced Threat Detection**: Machine learning-based threat detection
5. **Integration with External Services**: Threat intelligence feeds
6. **Enhanced MFA**: Additional MFA methods and policies
7. **Session Security**: Advanced session management and validation