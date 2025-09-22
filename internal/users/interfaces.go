package users

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/mbolli/tickit/internal/shared"
)

// UserStore defines the interface for user data storage operations
// This interface abstracts the database operations for user management
type UserStore interface {
	// User CRUD operations
	GetUser(ctx context.Context, userID string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	ListUsers(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*User, error)
	SearchUsers(ctx context.Context, query string, limit, offset int) ([]*User, error)
	CreateUser(ctx context.Context, user *User) error
	UpdateUser(ctx context.Context, user *User) error
	PatchUser(ctx context.Context, patch *PatchUserInput) error
	DeleteUser(ctx context.Context, userID string) error

	// User Credential operations
	GetUserCredentials(ctx context.Context, userID string) ([]*UserCredential, error)
	GetUserCredentialByProvider(ctx context.Context, userID, provider string) (*UserCredential, error)
	GetUserByOAuthProvider(ctx context.Context, provider, providerID string) (*User, error)
	CreateUserCredential(ctx context.Context, credential *UserCredential) error
	UpdateUserCredential(ctx context.Context, credential *UserCredential) error
	DeleteUserCredential(ctx context.Context, credentialID string) error

	// Session operations
	GetUserSession(ctx context.Context, sessionID string) (*UserSession, error)
	GetUserSessions(ctx context.Context, userID string) ([]*UserSession, error)
	GetActiveUserSessions(ctx context.Context, userID string) ([]*UserSession, error)
	CreateUserSession(ctx context.Context, session *UserSession) error
	UpdateUserSession(ctx context.Context, session *UserSession) error
	DeleteUserSession(ctx context.Context, sessionID string) error
	DeleteAllUserSessions(ctx context.Context, userID string) error
	DeleteExpiredSessions(ctx context.Context) (int, error)
	InvalidateAllUserSessions(ctx context.Context, userID string) error
	InvalidateAllUserSessionsExcept(ctx context.Context, userID, exceptSessionID string) error

	// Project Membership operations
	GetProjectMembers(ctx context.Context, projectID string) ([]*User, error)
	GetUserProjects(ctx context.Context, userID string) ([]*ProjectMembership, error)
	GetProjectMembership(ctx context.Context, userID, projectID string) (*ProjectMembership, error)
	CheckUserProjectAccess(ctx context.Context, userID, projectID string) (bool, error)
	CreateProjectMembership(ctx context.Context, membership *ProjectMembership) error
	UpdateProjectMembership(ctx context.Context, membership *ProjectMembership) error
	DeleteProjectMembership(ctx context.Context, userID, projectID string) error

	// Token operations
	GetEmailVerificationToken(ctx context.Context, token string) (*EmailVerificationToken, error)
	GetEmailVerificationTokenByUserID(ctx context.Context, userID string) (*EmailVerificationToken, error)
	CreateEmailVerificationToken(ctx context.Context, token *EmailVerificationToken) error
	DeleteEmailVerificationToken(ctx context.Context, token string) error

	GetPasswordResetToken(ctx context.Context, token string) (*PasswordResetToken, error)
	CreatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error
	UpdatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error
	DeletePasswordResetToken(ctx context.Context, token string) error

	GetMagicMailToken(ctx context.Context, token string) (*MagicMailToken, error)
	CreateMagicMailToken(ctx context.Context, token *MagicMailToken) error
	UpdateMagicMailToken(ctx context.Context, token *MagicMailToken) error
	DeleteMagicMailToken(ctx context.Context, token string) error

	// Invitation operations
	GetProjectInvitations(ctx context.Context, projectID string) ([]*UserInvitation, error)
	GetUserInvitations(ctx context.Context, email string) ([]*UserInvitation, error)
	GetInvitationByToken(ctx context.Context, token string) (*UserInvitation, error)
	CreateInvitation(ctx context.Context, invitation *UserInvitation) error
	UpdateInvitation(ctx context.Context, invitation *UserInvitation) error
	DeleteInvitation(ctx context.Context, invitationID string) error

	// Administrative operations
	GetUserActivity(ctx context.Context, userID string, limit, offset int) ([]interface{}, error)
	GetUserStats(ctx context.Context, userID string) (map[string]interface{}, error)
	GetSystemUserStats(ctx context.Context) (map[string]interface{}, error)
}

// AuthService defines the interface for authentication operations
// This service handles all authentication-related business logic
type AuthService interface {
	// User Registration
	RegisterUser(ctx context.Context, email, password, firstName, lastName, username string) (*User, error)
	VerifyEmail(ctx context.Context, token string) error

	// Authentication
	LoginUser(ctx context.Context, email, password, ipAddress, userAgent string) (*User, *UserSession, error)
	LoginOAuth(ctx context.Context, provider, code, state, ipAddress, userAgent string) (*User, *UserSession, error)
	LoginWebAuthn(ctx context.Context, userID string, assertion []byte, ipAddress, userAgent string) (*User, *UserSession, error)
	LoginMagicMail(ctx context.Context, token, ipAddress, userAgent string) (*User, *UserSession, error)
	LogoutUser(ctx context.Context, sessionID string) error

	// Password Management
	ForgotPassword(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, token, newPassword string) error
	ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error

	// Multi-Factor Authentication
	BeginMFARegistration(ctx context.Context, userID string) ([]byte, error)
	FinishMFARegistration(ctx context.Context, userID string, response []byte) error
	EnableMFA(ctx context.Context, userID string, credential []byte) error
	DisableMFA(ctx context.Context, userID, credentialID string) error

	// Session Management
	CreateSession(ctx context.Context, userID, sessionID, ipAddress, userAgent string, expiresAt time.Time) (*UserSession, error)
	ValidateSession(ctx context.Context, sessionID string) (*User, *UserSession, error)
	InvalidateSession(ctx context.Context, sessionID string) error
	InvalidateAllUserSessions(ctx context.Context, userID string) error
	InvalidateAllUserSessionsExcept(ctx context.Context, userID, exceptSessionID string) error

	// OAuth Account Management
	LinkOAuthAccount(ctx context.Context, userID, provider, providerID, code, state string) error
	UnlinkOAuthAccount(ctx context.Context, userID, provider string) error

	// Magic Mail (Development Only)
	SendMagicMail(ctx context.Context, email string) error

	// Utility Methods
	ValidateUserCredentials(ctx context.Context, email, password string) (*User, error)
	CheckUserMFAStatus(ctx context.Context, userID string) (bool, error)
	GetUserWebAuthnCredentials(ctx context.Context, userID string) ([]interface{}, error)
}

// PasswordHasher defines the interface for password hashing operations
type PasswordHasher interface {
	HashPassword(password string) (string, error)
	CheckPassword(hashedPassword, password string) error
}

// EmailService defines the interface for email operations
type EmailService interface {
	SendVerificationEmail(ctx context.Context, email, token string) error
	SendPasswordResetEmail(ctx context.Context, email, token string) error
	SendInvitationEmail(ctx context.Context, email, projectName, token string) error
	SendMagicMailLink(ctx context.Context, email, token string) error
}

// WebAuthnService defines the interface for WebAuthn operations
type WebAuthnService interface {
	BeginRegistration(ctx context.Context, userID string) ([]byte, error)
	FinishRegistration(ctx context.Context, userID string, response []byte) error
	BeginLogin(ctx context.Context, userID string) ([]byte, error)
	FinishLogin(ctx context.Context, userID string, response []byte) error
	GetUserCredentials(ctx context.Context, userID string) ([]interface{}, error)
	RemoveCredential(ctx context.Context, userID, credentialID string) error
}

// OAuthProvider defines the interface for OAuth provider operations
type OAuthProvider interface {
	GetAuthURL(state string) string
	ExchangeCodeForToken(code string) (*goth.User, error)
	GetUserInfo(token string) (*goth.User, error)
}

// SessionManager defines the interface for session management operations
type SessionManager interface {
	GetSession(r *http.Request, name string) (*sessions.Session, error)
	SaveSession(r *http.Request, w http.ResponseWriter, session *sessions.Session) error
	DestroySession(r *http.Request, w http.ResponseWriter, name string) error
}

// UserCommandHandlerInterface defines the interface for handling user commands
type UserCommandHandlerInterface interface {
	// Registration Commands
	HandleRegisterUser(ctx context.Context, cmd RegisterUserCommand) error
	HandleVerifyEmail(ctx context.Context, cmd VerifyEmailCommand) error

	// Authentication Commands
	HandleLoginUser(ctx context.Context, cmd LoginUserCommand) error
	HandleLoginOAuth(ctx context.Context, cmd LoginOAuthCommand) error
	HandleLoginWebAuthn(ctx context.Context, cmd LoginWebAuthnCommand) error
	HandleLoginMagicMail(ctx context.Context, cmd LoginMagicMailCommand) error
	HandleLogoutUser(ctx context.Context, cmd LogoutUserCommand) error

	// Password Management Commands
	HandleForgotPassword(ctx context.Context, cmd ForgotPasswordCommand) error
	HandleResetPassword(ctx context.Context, cmd ResetPasswordCommand) error
	HandleChangePassword(ctx context.Context, cmd ChangePasswordCommand) error

	// MFA Commands
	HandleEnableMFA(ctx context.Context, cmd EnableMFACommand) error
	HandleDisableMFA(ctx context.Context, cmd DisableMFACommand) error
	HandleBeginMFARegistration(ctx context.Context, cmd BeginMFARegistrationCommand) error
	HandleFinishMFARegistration(ctx context.Context, cmd FinishMFARegistrationCommand) error

	// Profile Management Commands
	HandleUpdateUserProfile(ctx context.Context, cmd UpdateUserProfileCommand) error
	HandleDeactivateUser(ctx context.Context, cmd DeactivateUserCommand) error
	HandleReactivateUser(ctx context.Context, cmd ReactivateUserCommand) error

	// Project Membership Commands
	HandleInviteUserToProject(ctx context.Context, cmd InviteUserToProjectCommand) error
	HandleAcceptProjectInvitation(ctx context.Context, cmd AcceptProjectInvitationCommand) error
	HandleDeclineProjectInvitation(ctx context.Context, cmd DeclineProjectInvitationCommand) error
	HandleAddUserToProject(ctx context.Context, cmd AddUserToProjectCommand) error
	HandleRemoveUserFromProject(ctx context.Context, cmd RemoveUserFromProjectCommand) error
	HandleUpdateProjectMembership(ctx context.Context, cmd UpdateProjectMembershipCommand) error

	// Administrative Commands
	HandleAssignUserRole(ctx context.Context, cmd AssignUserRoleCommand) error
	HandlePatchUser(ctx context.Context, cmd PatchUserCommand) error

	// OAuth Commands
	HandleLinkOAuthAccount(ctx context.Context, cmd LinkOAuthAccountCommand) error
	HandleUnlinkOAuthAccount(ctx context.Context, cmd UnlinkOAuthAccountCommand) error

	// Session Commands
	HandleCreateUserSession(ctx context.Context, cmd CreateUserSessionCommand) error
	HandleInvalidateUserSession(ctx context.Context, cmd InvalidateUserSessionCommand) error
	HandleInvalidateAllUserSessions(ctx context.Context, cmd InvalidateAllUserSessionsCommand) error

	// SSO Commands
	HandleLoginSSO(ctx context.Context, cmd LoginSSOCommand) error
	HandleLinkSSOAccount(ctx context.Context, cmd LinkSSOAccountCommand) error
	HandleUnlinkSSOAccount(ctx context.Context, cmd UnlinkSSOAccountCommand) error
	HandleInitiateSingleLogout(ctx context.Context, cmd InitiateSingleLogoutCommand) error
	HandleSyncUserGroups(ctx context.Context, cmd SyncUserGroupsCommand) error
}

// UserQueryHandlerInterface defines the interface for handling user queries
type UserQueryHandlerInterface interface {
	// User Queries
	HandleGetUserByID(ctx context.Context, query GetUserByIDQuery) (*User, error)
	HandleGetUserByEmail(ctx context.Context, query GetUserByEmailQuery) (*User, error)
	HandleGetUserByUsername(ctx context.Context, query GetUserByUsernameQuery) (*User, error)
	HandleListUsers(ctx context.Context, query ListUsersQuery) ([]*User, error)
	HandleSearchUsers(ctx context.Context, query SearchUsersQuery) ([]*User, error)

	// Credential Queries
	HandleGetUserCredentials(ctx context.Context, query GetUserCredentialsQuery) ([]*UserCredential, error)
	HandleGetUserCredentialByProvider(ctx context.Context, query GetUserCredentialByProviderQuery) (*UserCredential, error)
	HandleGetUserByOAuthProvider(ctx context.Context, query GetUserByOAuthProviderQuery) (*User, error)

	// Session Queries
	HandleGetUserSession(ctx context.Context, query GetUserSessionQuery) (*UserSession, error)
	HandleGetUserSessions(ctx context.Context, query GetUserSessionsQuery) ([]*UserSession, error)
	HandleGetActiveUserSessions(ctx context.Context, query GetActiveUserSessionsQuery) ([]*UserSession, error)

	// Project Membership Queries
	HandleGetProjectMembers(ctx context.Context, query GetProjectMembersQuery) ([]*User, error)
	HandleGetUserProjects(ctx context.Context, query GetUserProjectsQuery) ([]*ProjectMembership, error)
	HandleGetProjectMembership(ctx context.Context, query GetProjectMembershipQuery) (*ProjectMembership, error)
	HandleCheckUserProjectAccess(ctx context.Context, query CheckUserProjectAccessQuery) (bool, error)

	// Invitation Queries
	HandleGetProjectInvitations(ctx context.Context, query GetProjectInvitationsQuery) ([]*UserInvitation, error)
	HandleGetUserInvitations(ctx context.Context, query GetUserInvitationsQuery) ([]*UserInvitation, error)
	HandleGetInvitationByToken(ctx context.Context, query GetInvitationByTokenQuery) (*UserInvitation, error)

	// Token Queries
	HandleGetEmailVerificationToken(ctx context.Context, query GetEmailVerificationTokenQuery) (*EmailVerificationToken, error)
	HandleGetPasswordResetToken(ctx context.Context, query GetPasswordResetTokenQuery) (*PasswordResetToken, error)
	HandleGetMagicMailToken(ctx context.Context, query GetMagicMailTokenQuery) (*MagicMailToken, error)

	// Administrative Queries
	HandleGetUserActivity(ctx context.Context, query GetUserActivityQuery) ([]interface{}, error)
	HandleGetUserStats(ctx context.Context, query GetUserStatsQuery) (map[string]interface{}, error)
	HandleGetSystemUserStats(ctx context.Context, query GetSystemUserStatsQuery) (map[string]interface{}, error)

	// Authentication Queries
	HandleValidateUserCredentials(ctx context.Context, query ValidateUserCredentialsQuery) (*User, error)
	HandleCheckUserMFAStatus(ctx context.Context, query CheckUserMFAStatusQuery) (bool, error)
	HandleGetUserWebAuthnCredentials(ctx context.Context, query GetUserWebAuthnCredentialsQuery) ([]interface{}, error)

	// SSO Queries
	HandleGetSSOProviders(ctx context.Context, query GetSSOProvidersQuery) ([]string, error)
	HandleGetSSOProviderConfig(ctx context.Context, query GetSSOProviderConfigQuery) (*SSOProviderConfig, error)
	HandleGetUserSSOCredentials(ctx context.Context, query GetUserSSOCredentialsQuery) ([]*UserCredential, error)
	HandleValidateSSOProvider(ctx context.Context, query ValidateSSOProviderQuery) (bool, error)
}

// UserProjectionStore defines the interface for user projection operations
// This follows the same pattern as other projection stores in the system
type UserProjectionStore interface {
	shared.ProjectionStore
	shared.ProjectionRebuilder

	// User operations
	GetUser(ctx context.Context, userID string) (*User, error)
	GetUsers(ctx context.Context, userIDs []string) ([]*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	ListUsers(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*User, error)
	SearchUsers(ctx context.Context, query string, limit, offset int) ([]*User, error)

	// Credential operations
	GetUserCredentials(ctx context.Context, userID string) ([]*UserCredential, error)
	GetUserCredentialByProvider(ctx context.Context, userID, provider string) (*UserCredential, error)
	GetUserByOAuthProvider(ctx context.Context, provider, providerID string) (*User, error)

	// Session operations
	GetUserSession(ctx context.Context, sessionID string) (*UserSession, error)
	GetUserSessions(ctx context.Context, userID string) ([]*UserSession, error)
	GetActiveUserSessions(ctx context.Context, userID string) ([]*UserSession, error)

	// Project membership operations
	GetProjectMembers(ctx context.Context, projectID string) ([]*User, error)
	GetUserProjects(ctx context.Context, userID string) ([]*ProjectMembership, error)
	GetProjectMembership(ctx context.Context, userID, projectID string) (*ProjectMembership, error)
	CheckUserProjectAccess(ctx context.Context, userID, projectID string) (bool, error)

	// Token operations
	GetEmailVerificationToken(ctx context.Context, token string) (*EmailVerificationToken, error)
	GetEmailVerificationTokenByUserID(ctx context.Context, userID string) (*EmailVerificationToken, error)
	GetPasswordResetToken(ctx context.Context, token string) (*PasswordResetToken, error)
	GetMagicMailToken(ctx context.Context, token string) (*MagicMailToken, error)

	// Invitation operations
	GetProjectInvitations(ctx context.Context, projectID string) ([]*UserInvitation, error)
	GetUserInvitations(ctx context.Context, email string) ([]*UserInvitation, error)
	GetInvitationByToken(ctx context.Context, token string) (*UserInvitation, error)

	// Administrative operations
	GetUserActivity(ctx context.Context, userID string, limit, offset int) ([]interface{}, error)
	GetUserStats(ctx context.Context, userID string) (map[string]interface{}, error)
	GetSystemUserStats(ctx context.Context) (map[string]interface{}, error)
}
