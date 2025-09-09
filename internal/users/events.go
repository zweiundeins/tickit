package users

import "time"

// User Registration Events

type UserRegisteredEvent struct {
	User              *User     `json:"user"`
	VerificationToken string    `json:"verification_token"`
	Timestamp         time.Time `json:"timestamp"`
}

func (e UserRegisteredEvent) GetEventType() string      { return "UserRegistered" }
func (e UserRegisteredEvent) GetTimestamp() interface{} { return e.Timestamp }

type UserEmailVerifiedEvent struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Timestamp time.Time `json:"timestamp"`
}

func (e UserEmailVerifiedEvent) GetEventType() string      { return "UserEmailVerified" }
func (e UserEmailVerifiedEvent) GetTimestamp() interface{} { return e.Timestamp }

// Authentication Events

type UserLoggedInEvent struct {
	UserID    string    `json:"user_id"`
	SessionID string    `json:"session_id"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	Method    string    `json:"method"`             // password, oauth, webauthn, magic_mail
	Provider  string    `json:"provider,omitempty"` // For OAuth logins
	Timestamp time.Time `json:"timestamp"`
}

func (e UserLoggedInEvent) GetEventType() string      { return "UserLoggedIn" }
func (e UserLoggedInEvent) GetTimestamp() interface{} { return e.Timestamp }

type UserLoggedOutEvent struct {
	UserID    string    `json:"user_id"`
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
}

func (e UserLoggedOutEvent) GetEventType() string      { return "UserLoggedOut" }
func (e UserLoggedOutEvent) GetTimestamp() interface{} { return e.Timestamp }

type UserLoginFailedEvent struct {
	Email     string    `json:"email"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

func (e UserLoginFailedEvent) GetEventType() string      { return "UserLoginFailed" }
func (e UserLoginFailedEvent) GetTimestamp() interface{} { return e.Timestamp }

// Password Management Events

type PasswordResetRequestedEvent struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Token     string    `json:"token"`
	Timestamp time.Time `json:"timestamp"`
}

func (e PasswordResetRequestedEvent) GetEventType() string      { return "PasswordResetRequested" }
func (e PasswordResetRequestedEvent) GetTimestamp() interface{} { return e.Timestamp }

type PasswordResetCompletedEvent struct {
	UserID    string    `json:"user_id"`
	Timestamp time.Time `json:"timestamp"`
}

func (e PasswordResetCompletedEvent) GetEventType() string      { return "PasswordResetCompleted" }
func (e PasswordResetCompletedEvent) GetTimestamp() interface{} { return e.Timestamp }

type PasswordChangedEvent struct {
	UserID    string    `json:"user_id"`
	Timestamp time.Time `json:"timestamp"`
}

func (e PasswordChangedEvent) GetEventType() string      { return "PasswordChanged" }
func (e PasswordChangedEvent) GetTimestamp() interface{} { return e.Timestamp }

// Multi-Factor Authentication Events

type UserMFAEnabledEvent struct {
	UserID     string    `json:"user_id"`
	Credential []byte    `json:"credential_type"`
	Timestamp  time.Time `json:"timestamp"`
}

func (e UserMFAEnabledEvent) GetEventType() string      { return "UserMFAEnabled" }
func (e UserMFAEnabledEvent) GetTimestamp() interface{} { return e.Timestamp }

type UserMFARegistrationBeganEvent struct {
	UserID    string    `json:"user_id"`
	Timestamp time.Time `json:"timestamp"`
}

func (e UserMFARegistrationBeganEvent) GetEventType() string      { return "UserMFARegistrationBegan" }
func (e UserMFARegistrationBeganEvent) GetTimestamp() interface{} { return e.Timestamp }

type UserMFARegistrationCompletedEvent struct {
	UserID    string    `json:"user_id"`
	Timestamp time.Time `json:"timestamp"`
}

func (e UserMFARegistrationCompletedEvent) GetEventType() string {
	return "UserMFARegistrationCompleted"
}
func (e UserMFARegistrationCompletedEvent) GetTimestamp() interface{} { return e.Timestamp }

type UserMFADisabledEvent struct {
	UserID       string    `json:"user_id"`
	CredentialID string    `json:"credential_id"`
	Timestamp    time.Time `json:"timestamp"`
}

func (e UserMFADisabledEvent) GetEventType() string      { return "UserMFADisabled" }
func (e UserMFADisabledEvent) GetTimestamp() interface{} { return e.Timestamp }

type UserMFAAuthenticationSucceededEvent struct {
	UserID       string    `json:"user_id"`
	CredentialID string    `json:"credential_id"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	Timestamp    time.Time `json:"timestamp"`
}

func (e UserMFAAuthenticationSucceededEvent) GetEventType() string {
	return "UserMFAAuthenticationSucceeded"
}
func (e UserMFAAuthenticationSucceededEvent) GetTimestamp() interface{} { return e.Timestamp }

type UserMFAAuthenticationFailedEvent struct {
	UserID    string    `json:"user_id"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

func (e UserMFAAuthenticationFailedEvent) GetEventType() string      { return "UserMFAAuthenticationFailed" }
func (e UserMFAAuthenticationFailedEvent) GetTimestamp() interface{} { return e.Timestamp }

// User Profile Events

type UserProfileUpdatedEvent struct {
	UserID    string                 `json:"user_id"`
	Changes   map[string]interface{} `json:"changes"`
	UpdatedBy string                 `json:"updated_by"`
	Timestamp time.Time              `json:"timestamp"`
}

func (e UserProfileUpdatedEvent) GetEventType() string      { return "UserProfileUpdated" }
func (e UserProfileUpdatedEvent) GetTimestamp() interface{} { return e.Timestamp }

type UserDeactivatedEvent struct {
	UserID        string    `json:"user_id"`
	DeactivatedBy string    `json:"deactivated_by"`
	Timestamp     time.Time `json:"timestamp"`
}

func (e UserDeactivatedEvent) GetEventType() string      { return "UserDeactivated" }
func (e UserDeactivatedEvent) GetTimestamp() interface{} { return e.Timestamp }

type UserReactivatedEvent struct {
	UserID        string    `json:"user_id"`
	ReactivatedBy string    `json:"reactivated_by"`
	Timestamp     time.Time `json:"timestamp"`
}

func (e UserReactivatedEvent) GetEventType() string      { return "UserReactivated" }
func (e UserReactivatedEvent) GetTimestamp() interface{} { return e.Timestamp }

// Project Membership Events

type UserInvitedToProjectEvent struct {
	InvitationID string    `json:"invitation_id"`
	ProjectID    string    `json:"project_id"`
	Email        string    `json:"email"`
	Role         string    `json:"role"`
	InvitedBy    string    `json:"invited_by"`
	Token        string    `json:"token"`
	Timestamp    time.Time `json:"timestamp"`
}

func (e UserInvitedToProjectEvent) GetEventType() string      { return "UserInvitedToProject" }
func (e UserInvitedToProjectEvent) GetTimestamp() interface{} { return e.Timestamp }

type ProjectInvitationAcceptedEvent struct {
	InvitationID string    `json:"invitation_id"`
	UserID       string    `json:"user_id"`
	ProjectID    string    `json:"project_id"`
	Role         string    `json:"role"`
	Timestamp    time.Time `json:"timestamp"`
}

func (e ProjectInvitationAcceptedEvent) GetEventType() string      { return "ProjectInvitationAccepted" }
func (e ProjectInvitationAcceptedEvent) GetTimestamp() interface{} { return e.Timestamp }

type ProjectInvitationDeclinedEvent struct {
	InvitationID string    `json:"invitation_id"`
	Email        string    `json:"email"`
	ProjectID    string    `json:"project_id"`
	Timestamp    time.Time `json:"timestamp"`
}

func (e ProjectInvitationDeclinedEvent) GetEventType() string      { return "ProjectInvitationDeclined" }
func (e ProjectInvitationDeclinedEvent) GetTimestamp() interface{} { return e.Timestamp }

type UserAddedToProjectEvent struct {
	EventID     string    `json:"event_id"`
	UserID      string    `json:"user_id"`
	ProjectID   string    `json:"project_id"`
	Role        string    `json:"role"`
	AddedBy     string    `json:"added_by"`
	Timestamp   time.Time `json:"timestamp"`
	EventType   string    `json:"event_type"`
	AggregateID string    `json:"aggregate_id"`
}

func (e UserAddedToProjectEvent) GetEventType() string      { return "UserAddedToProject" }
func (e UserAddedToProjectEvent) GetTimestamp() interface{} { return e.Timestamp }

type UserRemovedFromProjectEvent struct {
	EventID     string    `json:"event_id"`
	UserID      string    `json:"user_id"`
	ProjectID   string    `json:"project_id"`
	RemovedBy   string    `json:"removed_by"`
	Timestamp   time.Time `json:"timestamp"`
	EventType   string    `json:"event_type"`
	AggregateID string    `json:"aggregate_id"`
}

func (e UserRemovedFromProjectEvent) GetEventType() string      { return "UserRemovedFromProject" }
func (e UserRemovedFromProjectEvent) GetTimestamp() interface{} { return e.Timestamp }

type ProjectMembershipUpdatedEvent struct {
	EventID     string    `json:"event_id"`
	UserID      string    `json:"user_id"`
	ProjectID   string    `json:"project_id"`
	OldRole     string    `json:"old_role"`
	NewRole     string    `json:"new_role"`
	UpdatedBy   string    `json:"updated_by"`
	Timestamp   time.Time `json:"timestamp"`
	EventType   string    `json:"event_type"`
	AggregateID string    `json:"aggregate_id"`
}

func (e ProjectMembershipUpdatedEvent) GetEventType() string      { return "ProjectMembershipUpdated" }
func (e ProjectMembershipUpdatedEvent) GetTimestamp() interface{} { return e.Timestamp }

// Administrative Events

type UserRoleAssignedEvent struct {
	EventID     string    `json:"event_id"`
	UserID      string    `json:"user_id"`
	OldRole     string    `json:"old_role"`
	NewRole     string    `json:"new_role"`
	AssignedBy  string    `json:"assigned_by"`
	Timestamp   time.Time `json:"timestamp"`
	EventType   string    `json:"event_type"`
	AggregateID string    `json:"aggregate_id"`
}

func (e UserRoleAssignedEvent) GetEventType() string      { return "UserRoleAssigned" }
func (e UserRoleAssignedEvent) GetTimestamp() interface{} { return e.Timestamp }

type UserPatchedEvent struct {
	UserID    string                 `json:"user_id"`
	Changes   map[string]interface{} `json:"changes"`
	UpdatedBy string                 `json:"updated_by"`
	Timestamp time.Time              `json:"timestamp"`
}

func (e UserPatchedEvent) GetEventType() string      { return "UserPatched" }
func (e UserPatchedEvent) GetTimestamp() interface{} { return e.Timestamp }

// OAuth Account Events

type OAuthAccountLinkedEvent struct {
	UserID     string    `json:"user_id"`
	Provider   string    `json:"provider"`
	ProviderID string    `json:"provider_id"`
	Timestamp  time.Time `json:"timestamp"`
}

func (e OAuthAccountLinkedEvent) GetEventType() string      { return "OAuthAccountLinked" }
func (e OAuthAccountLinkedEvent) GetTimestamp() interface{} { return e.Timestamp }

type OAuthAccountUnlinkedEvent struct {
	UserID    string    `json:"user_id"`
	Provider  string    `json:"provider"`
	Timestamp time.Time `json:"timestamp"`
}

func (e OAuthAccountUnlinkedEvent) GetEventType() string      { return "OAuthAccountUnlinked" }
func (e OAuthAccountUnlinkedEvent) GetTimestamp() interface{} { return e.Timestamp }

// Session Management Events

type UserSessionCreatedEvent struct {
	SessionID string    `json:"session_id"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	Timestamp time.Time `json:"timestamp"`
}

func (e UserSessionCreatedEvent) GetEventType() string      { return "UserSessionCreated" }
func (e UserSessionCreatedEvent) GetTimestamp() interface{} { return e.Timestamp }

type UserSessionInvalidatedEvent struct {
	SessionID string    `json:"session_id"`
	UserID    *string   `json:"user_id,omitempty"`
	Reason    string    `json:"reason,omitempty"` // logout, timeout, admin_action, concurrent_session_limit
	Timestamp time.Time `json:"timestamp"`
}

func (e UserSessionInvalidatedEvent) GetEventType() string      { return "UserSessionInvalidated" }
func (e UserSessionInvalidatedEvent) GetTimestamp() interface{} { return e.Timestamp }

type AllUserSessionsInvalidatedEvent struct {
	UserID    string    `json:"user_id"`
	Reason    string    `json:"reason"` // password_change, security_breach, admin_action
	Timestamp time.Time `json:"timestamp"`
}

func (e AllUserSessionsInvalidatedEvent) GetEventType() string      { return "AllUserSessionsInvalidated" }
func (e AllUserSessionsInvalidatedEvent) GetTimestamp() interface{} { return e.Timestamp }

// Magic Mail Events (Development Only)

type MagicMailSentEvent struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Token     string    `json:"token"`
	Timestamp time.Time `json:"timestamp"`
}

func (e MagicMailSentEvent) GetEventType() string      { return "MagicMailSent" }
func (e MagicMailSentEvent) GetTimestamp() interface{} { return e.Timestamp }

// SSO Events

type SSOAccountLinkedEvent struct {
	UserID     string    `json:"user_id"`
	Provider   string    `json:"provider"`
	ProviderID string    `json:"provider_id"`
	Timestamp  time.Time `json:"timestamp"`
}

func (e SSOAccountLinkedEvent) GetEventType() string      { return "SSOAccountLinked" }
func (e SSOAccountLinkedEvent) GetTimestamp() interface{} { return e.Timestamp }

type SSOAccountUnlinkedEvent struct {
	UserID    string    `json:"user_id"`
	Provider  string    `json:"provider"`
	Timestamp time.Time `json:"timestamp"`
}

func (e SSOAccountUnlinkedEvent) GetEventType() string      { return "SSOAccountUnlinked" }
func (e SSOAccountUnlinkedEvent) GetTimestamp() interface{} { return e.Timestamp }

type SingleLogoutInitiatedEvent struct {
	UserID    string    `json:"user_id"`
	Timestamp time.Time `json:"timestamp"`
}

func (e SingleLogoutInitiatedEvent) GetEventType() string      { return "SingleLogoutInitiated" }
func (e SingleLogoutInitiatedEvent) GetTimestamp() interface{} { return e.Timestamp }

type UserGroupsSyncedEvent struct {
	UserID    string    `json:"user_id"`
	Provider  string    `json:"provider"`
	Groups    []string  `json:"groups"`
	OldRole   string    `json:"old_role"`
	NewRole   string    `json:"new_role"`
	Timestamp time.Time `json:"timestamp"`
}

func (e UserGroupsSyncedEvent) GetEventType() string      { return "UserGroupsSynced" }
func (e UserGroupsSyncedEvent) GetTimestamp() interface{} { return e.Timestamp }
