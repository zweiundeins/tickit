# User Routes Implementation

This document describes the implementation of HTTP routes and handlers for the user management system.

## Overview

The user routes implementation provides comprehensive REST API endpoints and form-based web interface handlers for all user operations, including:

- User authentication (registration, login, logout)
- OAuth integration
- WebAuthn multi-factor authentication
- Password management (forgot/reset/change)
- Magic mail login (development only)
- User profile management
- Project membership management
- Administrative user management
- Session management

## Architecture

### Route Registration

Routes are registered in `internal/routes/users.go` through the `RegisterUserRoutes` function, which:

1. Initializes user service and dependencies
2. Creates user routes handlers
3. Configures CSRF protection using justinas/nosurf
4. Registers routes with appropriate middleware

### Security Implementation

The implementation includes comprehensive security measures:

#### CSRF Protection
- Uses `justinas/nosurf` library for CSRF protection
- Applied to all form-based routes
- OAuth and WebAuthn endpoints are exempt (they have their own protection)
- CSRF tokens are included in JSON responses for subsequent requests

#### Authentication Middleware
- `RequireAuthentication`: Ensures user is logged in
- `RequireAdminRole`: Ensures user has admin privileges
- Session-based authentication using Gorilla Sessions

#### Input Validation
- JSON request body validation
- Required field validation
- File upload validation (type, size)
- IP address and user agent tracking

## Route Groups

### Public Authentication Routes (with CSRF)
```
POST /auth/register          - User registration
POST /auth/login             - User login
POST /auth/logout            - User logout
GET  /auth/verify-email      - Email verification
POST /auth/forgot-password   - Forgot password
POST /auth/reset-password    - Reset password
POST /auth/change-password   - Change password
POST /auth/magic-mail        - Send magic mail (dev only)
```

### OAuth Routes (CSRF exempt)
```
GET /auth/oauth/{provider}           - OAuth login
GET /auth/oauth/{provider}/callback  - OAuth callback
```

### WebAuthn Routes (CSRF exempt)
```
POST /auth/webauthn/register/begin   - Begin WebAuthn registration
POST /auth/webauthn/register/finish  - Finish WebAuthn registration
POST /auth/webauthn/login/begin      - Begin WebAuthn login
POST /auth/webauthn/login/finish     - Finish WebAuthn login
```

### Protected User Profile Routes
```
GET  /api/users/profile        - Get user profile
PUT  /api/users/profile        - Update user profile
POST /api/users/profile/avatar - Upload avatar
```

### Protected Project Membership Routes
```
GET    /api/projects/{projectId}/members           - Get project members
POST   /api/projects/{projectId}/invite            - Invite user to project
DELETE /api/projects/{projectId}/members/{userId}  - Remove user from project
PUT    /api/projects/{projectId}/members/{userId}/role - Update member role
```

### Protected Invitation Routes
```
GET  /api/invitations                - Get user invitations
POST /api/invitations/{token}/accept - Accept invitation
POST /api/invitations/{token}/decline - Decline invitation
```

### Administrative Routes (Admin only)
```
GET  /api/admin/users                    - List users
GET  /api/admin/users/{userId}           - Get user details
PUT  /api/admin/users/{userId}           - Update user
POST /api/admin/users/{userId}/deactivate - Deactivate user
POST /api/admin/users/{userId}/reactivate - Reactivate user
PUT  /api/admin/users/{userId}/role      - Assign user role
GET  /api/admin/users/{userId}/activity  - Get user activity
GET  /api/admin/stats                    - Get system stats
```

### Session Management Routes
```
GET    /api/users/sessions            - Get user sessions
DELETE /api/users/sessions/{sessionId} - Invalidate specific session
DELETE /api/users/sessions            - Invalidate all sessions
```

## Handler Implementation

### Request/Response Format

All handlers follow consistent patterns:

#### Request Parsing
```go
var req struct {
    Field1 string `json:"field1"`
    Field2 string `json:"field2"`
}
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    ur.WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
    return
}
```

#### Response Format
```go
response := map[string]interface{}{
    "success": true,
    "message": "Operation successful",
    "data":    result,
}
ur.AddCSRFTokenToResponse(r, response)
ur.WriteJSONResponse(w, response)
```

#### Error Handling
```go
if err != nil {
    if strings.Contains(err.Error(), "specific_error") {
        ur.WriteJSONError(w, "User-friendly message", http.StatusBadRequest)
        return
    }
    ur.WriteJSONError(w, "Generic error message", http.StatusInternalServerError)
    return
}
```

### Key Features

#### Session Management
- Uses Gorilla Sessions for HTTP session management
- Integrates with existing `sharedhttp.GetAuthenticatedUser` pattern
- Proper session cleanup on logout

#### File Upload Handling
- Avatar upload with validation (type, size)
- Secure file storage in upload directory
- Cleanup on failure

#### Client Information Tracking
- IP address extraction (supports X-Forwarded-For)
- User agent tracking
- Used for audit logging and security

#### OAuth Integration
- State parameter for CSRF protection
- Provider validation
- Session-based state storage

## Integration with Existing System

### CQRS Integration
- Uses existing CommandBus and QueryBus
- Follows established command/query patterns
- Integrates with existing user service

### Middleware Integration
- Uses existing authentication middleware patterns
- Integrates with RBAC system
- Follows established error handling patterns

### Session Integration
- Uses existing session store configuration
- Integrates with shared HTTP utilities
- Maintains consistency with other modules

## Security Considerations

### Input Validation
- All user inputs are validated and sanitized
- File uploads are restricted by type and size
- SQL injection prevention through parameterized queries

### Authentication Security
- Generic error messages to prevent user enumeration
- Rate limiting (implemented in secure routes)
- Session security (HttpOnly, Secure, SameSite)

### CSRF Protection
- Comprehensive CSRF protection for form-based endpoints
- Proper exemptions for OAuth and WebAuthn
- Token validation and rotation

### Authorization
- Role-based access control
- Project-level permissions
- Admin-only endpoints protection

## Testing

The implementation includes:
- Route registration tests
- Handler unit tests (to be expanded)
- Integration tests with middleware
- Security tests for CSRF and authentication

## Future Enhancements

1. **Rate Limiting**: Enhanced rate limiting for authentication endpoints
2. **Audit Logging**: Comprehensive audit logging for all operations
3. **API Versioning**: Support for API versioning
4. **Webhook Support**: Webhooks for user events
5. **Bulk Operations**: Bulk user management operations

## Dependencies

- `github.com/go-chi/chi/v5` - HTTP router
- `github.com/gorilla/sessions` - Session management
- `github.com/justinas/nosurf` - CSRF protection
- Internal packages: `users`, `shared`, `bootstrap`