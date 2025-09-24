# User Profile Management Implementation

This document describes the implementation of user profile management functionality for the Tickit system.

## Overview

The profile management system provides comprehensive functionality for users to manage their profiles, including:

- Profile information updates (email, username, names)
- Avatar upload and management
- Account deactivation with data preservation
- Profile validation and sanitization

## Components

### 1. Command Handlers

#### UpdateUserProfileCommand
- **Purpose**: Updates user profile information
- **Validation**: Email uniqueness, username uniqueness, field validation
- **Features**: 
  - Email change triggers re-verification
  - Old avatar cleanup when avatar is changed
  - Change tracking for audit purposes

#### DeactivateUserCommand
- **Purpose**: Deactivates user account while preserving data
- **Features**:
  - Sets user status to "Inactive"
  - Invalidates all user sessions
  - Cleans up user avatar file
  - Publishes deactivation events

#### ReactivateUserCommand
- **Purpose**: Reactivates a previously deactivated account
- **Features**:
  - Sets user status to "Active"
  - Only works on "Inactive" users
  - Publishes reactivation events

### 2. Avatar Management

#### AvatarService
- **Purpose**: Handles avatar file operations
- **Features**:
  - File cleanup when avatars are changed
  - Validation of avatar files
  - Support for both local uploads and external URLs
  - Path security (prevents directory traversal)

#### Avatar Upload Handler
- **Endpoint**: `POST /api/users/profile/avatar`
- **Features**:
  - File type validation (JPEG, PNG, GIF, WebP)
  - File size limits (5MB maximum)
  - Unique filename generation
  - Automatic profile update after upload

### 3. Validation

#### Profile Field Validation
- **Email**: Format validation, uniqueness checking
- **Username**: Length, character restrictions, uniqueness checking
- **Names**: Character restrictions, length limits
- **Avatar URL**: Format validation, security checks

#### Avatar URL Validation
- Supports HTTP/HTTPS URLs and local upload paths
- Validates file extensions for local uploads
- Prevents directory traversal attacks
- Maximum URL length enforcement

### 4. HTTP Routes

#### Profile Management Routes
```
GET    /api/users/profile           - Get current user profile
PUT    /api/users/profile           - Update user profile
POST   /api/users/profile/avatar    - Upload avatar
POST   /api/users/profile/deactivate - Deactivate account
```

## Usage Examples

### Update Profile
```json
PUT /api/users/profile
{
  "username": "newusername",
  "first_name": "John",
  "last_name": "Doe",
  "email": "john.doe@example.com"
}
```

### Upload Avatar
```
POST /api/users/profile/avatar
Content-Type: multipart/form-data

avatar: [image file]
```

### Deactivate Account
```json
POST /api/users/profile/deactivate
{
  "confirm": true
}
```

## Security Features

### Input Validation
- All profile fields are validated before processing
- Email format and uniqueness validation
- Username format and uniqueness validation
- Avatar URL security validation

### File Upload Security
- File type restrictions (images only)
- File size limits (5MB maximum)
- Unique filename generation to prevent conflicts
- Directory traversal prevention

### Session Management
- Account deactivation invalidates all user sessions
- Session-based authentication for all profile operations

## Data Preservation

### Account Deactivation
- User data is preserved in the database
- User status is set to "Inactive"
- Historical data (issues, comments, etc.) remains intact
- Avatar files are cleaned up for privacy

### Reactivation
- Accounts can be reactivated by administrators
- All preserved data becomes accessible again
- New avatar must be uploaded after reactivation

## Events

### Profile Events
- `UserProfileUpdatedEvent`: Published when profile is updated
- `UserDeactivatedEvent`: Published when account is deactivated
- `UserReactivatedEvent`: Published when account is reactivated
- `AllUserSessionsInvalidatedEvent`: Published when sessions are invalidated

## Error Handling

### Validation Errors
- Invalid email format
- Email already in use
- Username already taken
- Invalid avatar URL format
- File too large or invalid type

### System Errors
- Database connection issues
- File system errors during avatar operations
- Email service failures (non-blocking)

## Testing

### Unit Tests
- Profile validation functions
- Command handler logic
- Avatar service operations
- Error handling scenarios

### Integration Tests
- Complete profile update workflows
- Avatar upload and cleanup
- Account deactivation and reactivation
- Event publishing verification

## Requirements Satisfied

This implementation satisfies the following requirements from the specification:

- **4.1**: Profile update functionality
- **4.2**: Email change with re-verification
- **4.3**: Password change (handled separately)
- **4.4**: Avatar upload and storage
- **4.5**: Profile validation and sanitization
- **4.6**: Account deactivation with data preservation

## Future Enhancements

### Potential Improvements
- Image resizing and optimization for avatars
- Multiple avatar sizes (thumbnails)
- Profile change history tracking
- Bulk profile operations for administrators
- Advanced avatar validation (content scanning)