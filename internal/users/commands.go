package users

import "time"

// User Registration Commands

type RegisterUserCommand struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

func (c RegisterUserCommand) GetCommandType() string { return "RegisterUser" }

type VerifyEmailCommand struct {
	Token string `json:"token"`
}

func (c VerifyEmailCommand) GetCommandType() string { return "VerifyEmail" }

// Authentication Commands

type LoginUserCommand struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
}

func (c LoginUserCommand) GetCommandType() string { return "LoginUser" }

type LoginOAuthCommand struct {
	Provider  string `json:"provider"`
	Code      string `json:"code"`
	State     string `json:"state"`
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
}

func (c LoginOAuthCommand) GetCommandType() string { return "LoginOAuth" }

type LoginWebAuthnCommand struct {
	UserID    string `json:"user_id"`
	Assertion []byte `json:"assertion"`
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
}

func (c LoginWebAuthnCommand) GetCommandType() string { return "LoginWebAuthn" }

type SendMagicMailCommand struct {
	Email string `json:"email"`
}

func (c SendMagicMailCommand) GetCommandType() string { return "SendMagicMail" }

type LoginMagicMailCommand struct {
	Token     string `json:"token"`
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
}

func (c LoginMagicMailCommand) GetCommandType() string { return "LoginMagicMail" }

type LogoutUserCommand struct {
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`
}

func (c LogoutUserCommand) GetCommandType() string { return "LogoutUser" }

// Password Management Commands

type ForgotPasswordCommand struct {
	Email string `json:"email"`
}

func (c ForgotPasswordCommand) GetCommandType() string { return "ForgotPassword" }

type ResetPasswordCommand struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

func (c ResetPasswordCommand) GetCommandType() string { return "ResetPassword" }

type ChangePasswordCommand struct {
	UserID           string `json:"user_id"`
	OldPassword      string `json:"old_password"`
	NewPassword      string `json:"new_password"`
	CurrentSessionID string `json:"current_session_id,omitempty"` // Optional: session to keep active
}

func (c ChangePasswordCommand) GetCommandType() string { return "ChangePassword" }

// Multi-Factor Authentication Commands

type EnableMFACommand struct {
	UserID     string `json:"user_id"`
	Credential []byte `json:"credential"` // WebAuthn credential
}

func (c EnableMFACommand) GetCommandType() string { return "EnableMFA" }

type DisableMFACommand struct {
	UserID       string `json:"user_id"`
	CredentialID string `json:"credential_id"`
}

func (c DisableMFACommand) GetCommandType() string { return "DisableMFA" }

type BeginMFARegistrationCommand struct {
	UserID string `json:"user_id"`
}

func (c BeginMFARegistrationCommand) GetCommandType() string { return "BeginMFARegistration" }

type FinishMFARegistrationCommand struct {
	UserID   string `json:"user_id"`
	Response []byte `json:"response"`
}

func (c FinishMFARegistrationCommand) GetCommandType() string { return "FinishMFARegistration" }

// User Profile Management Commands

type UpdateUserProfileCommand struct {
	UserID    string  `json:"user_id"`
	Email     *string `json:"email"`
	Username  *string `json:"username"`
	FirstName *string `json:"first_name"`
	LastName  *string `json:"last_name"`
	AvatarURL *string `json:"avatar_url"`
}

func (c UpdateUserProfileCommand) GetCommandType() string { return "UpdateUserProfile" }

type DeactivateUserCommand struct {
	UserID string `json:"user_id"`
}

func (c DeactivateUserCommand) GetCommandType() string { return "DeactivateUser" }

type ReactivateUserCommand struct {
	UserID string `json:"user_id"`
}

func (c ReactivateUserCommand) GetCommandType() string { return "ReactivateUser" }

// Project Membership Commands

type InviteUserToProjectCommand struct {
	ProjectID string `json:"project_id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	InvitedBy string `json:"invited_by"`
}

func (c InviteUserToProjectCommand) GetCommandType() string { return "InviteUserToProject" }

type AcceptProjectInvitationCommand struct {
	Token string `json:"token"`
}

func (c AcceptProjectInvitationCommand) GetCommandType() string { return "AcceptProjectInvitation" }

type DeclineProjectInvitationCommand struct {
	Token string `json:"token"`
}

func (c DeclineProjectInvitationCommand) GetCommandType() string { return "DeclineProjectInvitation" }

type AddUserToProjectCommand struct {
	UserID    string `json:"user_id"`
	ProjectID string `json:"project_id"`
	Role      string `json:"role"`
	AddedBy   string `json:"added_by"`
}

func (c AddUserToProjectCommand) GetCommandType() string { return "AddUserToProject" }

type RemoveUserFromProjectCommand struct {
	UserID    string `json:"user_id"`
	ProjectID string `json:"project_id"`
	RemovedBy string `json:"removed_by"`
}

func (c RemoveUserFromProjectCommand) GetCommandType() string { return "RemoveUserFromProject" }

type UpdateProjectMembershipCommand struct {
	UserID    string `json:"user_id"`
	ProjectID string `json:"project_id"`
	Role      string `json:"role"`
	UpdatedBy string `json:"updated_by"`
}

func (c UpdateProjectMembershipCommand) GetCommandType() string { return "UpdateProjectMembership" }

// Administrative Commands

type AssignUserRoleCommand struct {
	UserID     string `json:"user_id"`
	Role       string `json:"role"`
	AssignedBy string `json:"assigned_by"`
}

func (c AssignUserRoleCommand) GetCommandType() string { return "AssignUserRole" }

type PatchUserCommand struct {
	ID            string  `json:"id"`
	Email         *string `json:"email"`
	Username      *string `json:"username"`
	FirstName     *string `json:"first_name"`
	LastName      *string `json:"last_name"`
	AvatarURL     *string `json:"avatar_url"`
	Role          *string `json:"role"`
	Status        *string `json:"status"`
	EmailVerified *bool   `json:"email_verified"`
	UpdatedBy     string  `json:"updated_by"`
}

func (c PatchUserCommand) GetCommandType() string { return "PatchUser" }

// OAuth Account Linking Commands

type LinkOAuthAccountCommand struct {
	UserID     string `json:"user_id"`
	Provider   string `json:"provider"`
	ProviderID string `json:"provider_id"`
	Code       string `json:"code"`
	State      string `json:"state"`
}

func (c LinkOAuthAccountCommand) GetCommandType() string { return "LinkOAuthAccount" }

type UnlinkOAuthAccountCommand struct {
	UserID   string `json:"user_id"`
	Provider string `json:"provider"`
}

func (c UnlinkOAuthAccountCommand) GetCommandType() string { return "UnlinkOAuthAccount" }

// Session Management Commands

type CreateUserSessionCommand struct {
	UserID    string    `json:"user_id"`
	SessionID string    `json:"session_id"`
	ExpiresAt time.Time `json:"expires_at"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
}

func (c CreateUserSessionCommand) GetCommandType() string { return "CreateUserSession" }

type InvalidateUserSessionCommand struct {
	SessionID string `json:"session_id"`
}

func (c InvalidateUserSessionCommand) GetCommandType() string { return "InvalidateUserSession" }

type InvalidateAllUserSessionsCommand struct {
	UserID string `json:"user_id"`
}

func (c InvalidateAllUserSessionsCommand) GetCommandType() string { return "InvalidateAllUserSessions" }

// SSO Commands

type LoginSSOCommand struct {
	Provider  string `json:"provider"`
	Code      string `json:"code"`
	State     string `json:"state"`
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
}

func (c LoginSSOCommand) GetCommandType() string { return "LoginSSO" }

type LinkSSOAccountCommand struct {
	UserID     string `json:"user_id"`
	Provider   string `json:"provider"`
	ProviderID string `json:"provider_id"`
	Code       string `json:"code"`
	State      string `json:"state"`
}

func (c LinkSSOAccountCommand) GetCommandType() string { return "LinkSSOAccount" }

type UnlinkSSOAccountCommand struct {
	UserID   string `json:"user_id"`
	Provider string `json:"provider"`
}

func (c UnlinkSSOAccountCommand) GetCommandType() string { return "UnlinkSSOAccount" }

type InitiateSingleLogoutCommand struct {
	UserID string `json:"user_id"`
}

func (c InitiateSingleLogoutCommand) GetCommandType() string { return "InitiateSingleLogout" }

type SyncUserGroupsCommand struct {
	UserID   string   `json:"user_id"`
	Provider string   `json:"provider"`
	Groups   []string `json:"groups"`
}

func (c SyncUserGroupsCommand) GetCommandType() string { return "SyncUserGroups" }
