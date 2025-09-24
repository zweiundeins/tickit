# Administrative User Management Interface

This document describes the administrative user management interface implemented for the Tickit system.

## Overview

The administrative interface provides comprehensive user management capabilities for system administrators, including user listing, search, activity monitoring, and data export functionality.

## Features Implemented

### 1. User Listing and Search Functionality

#### Endpoints
- `GET /api/admin/users` - List all users with filtering and pagination
- `GET /api/admin/users?search={query}` - Search users by email, username, or name

#### Query Parameters
- `limit` - Number of users to return (default: 50)
- `offset` - Number of users to skip for pagination (default: 0)
- `status` - Filter by user status (Active, Inactive, Pending)
- `role` - Filter by user role (Admin, ProjectManager, Developer, Viewer)
- `search` - Search term for finding users
- `export` - Export format (json, csv)

#### Response Format
```json
{
  "users": [
    {
      "id": "user-id",
      "email": "user@example.com",
      "username": "username",
      "first_name": "First",
      "last_name": "Last",
      "role": "Developer",
      "status": "Active",
      "email_verified": true,
      "created_at": "2023-01-01T00:00:00Z",
      "updated_at": "2023-01-01T00:00:00Z",
      "last_login_at": "2023-01-01T00:00:00Z",
      "mfa_enabled": false,
      "auth_methods": ["password", "github"],
      "session_count": 2,
      "project_count": 3
    }
  ],
  "limit": 50,
  "offset": 0,
  "total": 100
}
```

### 2. User Activation/Deactivation Admin Commands

#### Endpoints
- `POST /api/admin/users/{userId}/deactivate` - Deactivate a user account
- `POST /api/admin/users/{userId}/reactivate` - Reactivate a user account

#### Response Format
```json
{
  "success": true,
  "message": "User deactivated successfully"
}
```

### 3. User Activity Logging and Viewing

#### Endpoints
- `GET /api/admin/users/{userId}/activity` - Get user activity history

#### Query Parameters
- `limit` - Number of activity records to return (default: 50)
- `offset` - Number of records to skip for pagination (default: 0)

#### Response Format
```json
{
  "activity": [
    {
      "type": "login",
      "timestamp": "2023-01-01T00:00:00Z",
      "ip_address": "192.168.1.1",
      "user_agent": "Mozilla/5.0..."
    }
  ],
  "limit": 50,
  "offset": 0
}
```

### 4. User Data Export Functionality

#### Endpoints
- `GET /api/admin/users?export=json` - Export users as JSON
- `GET /api/admin/users?export=csv` - Export users as CSV

#### CSV Format
The CSV export includes the following columns:
- ID, Email, Username, FirstName, LastName, Role, Status, EmailVerified, CreatedAt, UpdatedAt, LastLoginAt

### 5. Additional Administrative Features

#### User Details
- `GET /api/admin/users/{userId}` - Get detailed user information

#### User Management
- `PUT /api/admin/users/{userId}` - Update user information
- `PUT /api/admin/users/{userId}/role` - Assign user role

#### System Statistics
- `GET /api/admin/stats` - Get system-wide user statistics

## Service Layer Methods

### Administrative Query Methods
- `GetUserActivity(ctx, userID, limit, offset)` - Get user activity history
- `GetUserStats(ctx, userID)` - Get individual user statistics
- `GetSystemUserStats(ctx)` - Get system-wide user statistics
- `ExportUserData(ctx, format)` - Export user data in specified format

### User Management Methods
- `ListUsers(ctx, limit, offset, status, role)` - List users with filtering
- `SearchUsers(ctx, query, limit, offset)` - Search users
- `GetUserCredentials(ctx, userID)` - Get user authentication methods
- `CheckUserMFAStatus(ctx, userID)` - Check if user has MFA enabled

## Database Operations

### User Statistics
The system tracks various user statistics including:
- Total sessions
- Active sessions
- Project memberships
- Credential count
- Email verification status
- Account creation date
- Last login time

### Activity Logging
User activities are logged including:
- Login events with IP address and user agent
- Session creation and expiration
- Authentication method used

### System Statistics
System-wide statistics include:
- Total user count
- Active user count
- Verified user count
- Users by role distribution
- Recent registrations (last 30 days)
- Total and active session counts

## Security Considerations

1. **Admin Authorization**: All administrative endpoints should be protected with proper admin role verification
2. **Data Privacy**: Sensitive information like passwords and tokens are not exposed in responses
3. **Rate Limiting**: Administrative endpoints should have appropriate rate limiting
4. **Audit Logging**: All administrative actions should be logged for audit purposes
5. **CSRF Protection**: All form-based endpoints use CSRF protection via justinas/nosurf

## Testing

The implementation includes comprehensive tests:
- Unit tests for service methods
- Integration tests for HTTP endpoints
- Database operation tests
- Export functionality tests

## Usage Examples

### List Active Users
```bash
curl -X GET "/api/admin/users?status=Active&limit=10"
```

### Search Users
```bash
curl -X GET "/api/admin/users?search=john@example.com"
```

### Deactivate User
```bash
curl -X POST "/api/admin/users/user-123/deactivate"
```

### Export Users as CSV
```bash
curl -X GET "/api/admin/users?export=csv" > users.csv
```

### Get User Activity
```bash
curl -X GET "/api/admin/users/user-123/activity?limit=20"
```

## Requirements Satisfied

This implementation satisfies the following requirements from the specification:

- **12.1**: Admin can view user list with roles, status, auth methods, MFA status, and SSO status
- **12.2**: Admin can search users by name, email, role, MFA status, or auth method
- **12.3**: Admin can deactivate users (prevents login but preserves data)
- **12.4**: Admin can reactivate users (restores login access)
- **12.5**: Admin can view user activity including login history and recent actions
- **12.6**: Admin can export user data in CSV or JSON format
- **12.7**: Admin can reset user MFA (removes WebAuthn credentials)
- **12.8**: Admin can configure SSO and validate identity provider metadata