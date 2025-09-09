package users

import "time"

// UI-specific events for real-time updates
// These events contain only non-sensitive data suitable for UI consumption
// They are published to JetStream for real-time UI updates

// UserStatusChangedEvent represents a change in user status for UI updates
type UserStatusChangedEvent struct {
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Status    string    `json:"status"`    // Active, Inactive, Pending
	IsOnline  bool      `json:"is_online"` // Based on recent login activity
	Timestamp time.Time `json:"timestamp"`
}

func (e UserStatusChangedEvent) GetEventType() string      { return "UserStatusChanged" }
func (e UserStatusChangedEvent) GetTimestamp() interface{} { return e.Timestamp }

// UserRoleChangedEvent represents a role change for UI updates
type UserRoleChangedEvent struct {
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	OldRole   string    `json:"old_role"`
	NewRole   string    `json:"new_role"`
	ChangedBy string    `json:"changed_by"` // Username of admin who made the change
	Timestamp time.Time `json:"timestamp"`
}

func (e UserRoleChangedEvent) GetEventType() string      { return "UserRoleChanged" }
func (e UserRoleChangedEvent) GetTimestamp() interface{} { return e.Timestamp }

// UserLoginStatusEvent represents login/logout events for UI updates
type UserLoginStatusEvent struct {
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`    // "login" or "logout"
	Method    string    `json:"method"`    // password, oauth, webauthn, magic_mail
	Provider  string    `json:"provider"`  // For OAuth logins (github, google, etc.)
	Timestamp time.Time `json:"timestamp"`
}

func (e UserLoginStatusEvent) GetEventType() string      { return "UserLoginStatus" }
func (e UserLoginStatusEvent) GetTimestamp() interface{} { return e.Timestamp }

// UserProjectMembershipChangedEvent represents project membership changes for UI updates
type UserProjectMembershipChangedEvent struct {
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	ProjectID   string    `json:"project_id"`
	ProjectName string    `json:"project_name"`
	Action      string    `json:"action"` // "added", "removed", "role_changed"
	Role        string    `json:"role"`   // New role for added/role_changed, empty for removed
	OldRole     string    `json:"old_role,omitempty"` // For role_changed action
	ChangedBy   string    `json:"changed_by"` // Username of admin who made the change
	Timestamp   time.Time `json:"timestamp"`
}

func (e UserProjectMembershipChangedEvent) GetEventType() string      { return "UserProjectMembershipChanged" }
func (e UserProjectMembershipChangedEvent) GetTimestamp() interface{} { return e.Timestamp }

// UserMFAStatusChangedEvent represents MFA status changes for UI updates
type UserMFAStatusChangedEvent struct {
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Enabled   bool      `json:"enabled"`
	Method    string    `json:"method"` // webauthn, totp, etc.
	Timestamp time.Time `json:"timestamp"`
}

func (e UserMFAStatusChangedEvent) GetEventType() string      { return "UserMFAStatusChanged" }
func (e UserMFAStatusChangedEvent) GetTimestamp() interface{} { return e.Timestamp }

// AdminUserManagementEvent represents admin actions for real-time admin UI updates
type AdminUserManagementEvent struct {
	AdminID     string    `json:"admin_id"`
	AdminName   string    `json:"admin_name"`
	TargetID    string    `json:"target_id"`
	TargetName  string    `json:"target_name"`
	Action      string    `json:"action"` // "activated", "deactivated", "role_changed", "mfa_reset", etc.
	Details     string    `json:"details,omitempty"` // Additional context
	Timestamp   time.Time `json:"timestamp"`
}

func (e AdminUserManagementEvent) GetEventType() string      { return "AdminUserManagement" }
func (e AdminUserManagementEvent) GetTimestamp() interface{} { return e.Timestamp }

// UserProfileChangedEvent represents profile changes for UI updates
type UserProfileChangedEvent struct {
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	ChangedBy   string    `json:"changed_by"` // Username of who made the change
	FieldsChanged []string `json:"fields_changed"` // List of fields that were changed
	Timestamp   time.Time `json:"timestamp"`
}

func (e UserProfileChangedEvent) GetEventType() string      { return "UserProfileChanged" }
func (e UserProfileChangedEvent) GetTimestamp() interface{} { return e.Timestamp }

// UserInvitationStatusEvent represents invitation status changes for UI updates
type UserInvitationStatusEvent struct {
	InvitationID string    `json:"invitation_id"`
	ProjectID    string    `json:"project_id"`
	ProjectName  string    `json:"project_name"`
	Email        string    `json:"email"`
	Status       string    `json:"status"` // "sent", "accepted", "declined", "expired"
	InvitedBy    string    `json:"invited_by"` // Username of who sent the invitation
	Timestamp    time.Time `json:"timestamp"`
}

func (e UserInvitationStatusEvent) GetEventType() string      { return "UserInvitationStatus" }
func (e UserInvitationStatusEvent) GetTimestamp() interface{} { return e.Timestamp }