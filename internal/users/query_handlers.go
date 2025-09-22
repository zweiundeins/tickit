package users

import (
	"context"
	"fmt"
)

// UserQueryHandler handles all user-related queries
// Following the same pattern as other query handlers in the system
type UserQueryHandler struct {
	projectionStore UserProjectionStore
	userStore       UserStore
}

// NewUserQueryHandler creates a new UserQueryHandler instance
func NewUserQueryHandler(projectionStore UserProjectionStore, userStore UserStore) *UserQueryHandler {
	return &UserQueryHandler{
		projectionStore: projectionStore,
		userStore:       userStore,
	}
}

// Handle implements the shared.QueryHandler interface
func (h *UserQueryHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	switch q := query.(type) {
	// User Queries
	case GetUserByIDQuery:
		return h.HandleGetUserByID(ctx, q)
	case GetUsersByIDQuery:
		return h.HandleGetUsersByID(ctx, q)
	case GetUserByEmailQuery:
		return h.HandleGetUserByEmail(ctx, q)
	case GetUserByUsernameQuery:
		return h.HandleGetUserByUsername(ctx, q)
	case ListUsersQuery:
		return h.HandleListUsers(ctx, q)
	case SearchUsersQuery:
		return h.HandleSearchUsers(ctx, q)

	// Credential Queries
	case GetUserCredentialsQuery:
		return h.HandleGetUserCredentials(ctx, q)
	case GetUserCredentialByProviderQuery:
		return h.HandleGetUserCredentialByProvider(ctx, q)
	case GetUserByOAuthProviderQuery:
		return h.HandleGetUserByOAuthProvider(ctx, q)

	// Session Queries
	case GetUserSessionQuery:
		return h.HandleGetUserSession(ctx, q)
	case GetUserSessionsQuery:
		return h.HandleGetUserSessions(ctx, q)
	case GetActiveUserSessionsQuery:
		return h.HandleGetActiveUserSessions(ctx, q)

	// Project Membership Queries
	case GetProjectMembersQuery:
		return h.HandleGetProjectMembers(ctx, q)
	case GetUserProjectsQuery:
		return h.HandleGetUserProjects(ctx, q)
	case GetProjectMembershipQuery:
		return h.HandleGetProjectMembership(ctx, q)
	case CheckUserProjectAccessQuery:
		return h.HandleCheckUserProjectAccess(ctx, q)

	// Invitation Queries
	case GetProjectInvitationsQuery:
		return h.HandleGetProjectInvitations(ctx, q)
	case GetUserInvitationsQuery:
		return h.HandleGetUserInvitations(ctx, q)
	case GetInvitationByTokenQuery:
		return h.HandleGetInvitationByToken(ctx, q)

	// Token Queries
	case GetEmailVerificationTokenQuery:
		return h.HandleGetEmailVerificationToken(ctx, q)
	case GetPasswordResetTokenQuery:
		return h.HandleGetPasswordResetToken(ctx, q)
	case GetMagicMailTokenQuery:
		return h.HandleGetMagicMailToken(ctx, q)

	// Administrative Queries
	case GetUserActivityQuery:
		return h.HandleGetUserActivity(ctx, q)
	case GetUserStatsQuery:
		return h.HandleGetUserStats(ctx, q)
	case GetSystemUserStatsQuery:
		return h.HandleGetSystemUserStats(ctx, q)

	// Authentication Queries
	case ValidateUserCredentialsQuery:
		return h.HandleValidateUserCredentials(ctx, q)
	case CheckUserMFAStatusQuery:
		return h.HandleCheckUserMFAStatus(ctx, q)
	case GetUserWebAuthnCredentialsQuery:
		return h.HandleGetUserWebAuthnCredentials(ctx, q)

	default:
		return nil, nil // Ignore unknown queries
	}
}

// User Query Handlers

func (h *UserQueryHandler) HandleGetUserByID(ctx context.Context, query GetUserByIDQuery) (*User, error) {
	return h.projectionStore.GetUser(ctx, query.ID)
}

func (h *UserQueryHandler) HandleGetUsersByID(ctx context.Context, query GetUsersByIDQuery) ([]*User, error) {
	return h.projectionStore.GetUsers(ctx, query.IDs)
}

func (h *UserQueryHandler) HandleGetUserByEmail(ctx context.Context, query GetUserByEmailQuery) (*User, error) {
	return h.projectionStore.GetUserByEmail(ctx, query.Email)
}

func (h *UserQueryHandler) HandleGetUserByUsername(ctx context.Context, query GetUserByUsernameQuery) (*User, error) {
	return h.projectionStore.GetUserByUsername(ctx, query.Username)
}

func (h *UserQueryHandler) HandleListUsers(ctx context.Context, query ListUsersQuery) ([]*User, error) {
	filters := make(map[string]interface{})
	if query.Status != "" {
		filters["status"] = query.Status
	}
	if query.Role != "" {
		filters["role"] = query.Role
	}
	return h.projectionStore.ListUsers(ctx, query.Limit, query.Offset, filters)
}

func (h *UserQueryHandler) HandleSearchUsers(ctx context.Context, query SearchUsersQuery) ([]*User, error) {
	return h.projectionStore.SearchUsers(ctx, query.Query, query.Limit, query.Offset)
}

// Credential Query Handlers

func (h *UserQueryHandler) HandleGetUserCredentials(ctx context.Context, query GetUserCredentialsQuery) ([]*UserCredential, error) {
	return h.projectionStore.GetUserCredentials(ctx, query.UserID)
}

func (h *UserQueryHandler) HandleGetUserCredentialByProvider(ctx context.Context, query GetUserCredentialByProviderQuery) (*UserCredential, error) {
	return h.projectionStore.GetUserCredentialByProvider(ctx, query.UserID, query.Provider)
}

func (h *UserQueryHandler) HandleGetUserByOAuthProvider(ctx context.Context, query GetUserByOAuthProviderQuery) (*User, error) {
	return h.projectionStore.GetUserByOAuthProvider(ctx, query.Provider, query.ProviderID)
}

// Session Query Handlers

func (h *UserQueryHandler) HandleGetUserSession(ctx context.Context, query GetUserSessionQuery) (*UserSession, error) {
	return h.projectionStore.GetUserSession(ctx, query.SessionID)
}

func (h *UserQueryHandler) HandleGetUserSessions(ctx context.Context, query GetUserSessionsQuery) ([]*UserSession, error) {
	return h.projectionStore.GetUserSessions(ctx, query.UserID)
}

func (h *UserQueryHandler) HandleGetActiveUserSessions(ctx context.Context, query GetActiveUserSessionsQuery) ([]*UserSession, error) {
	return h.projectionStore.GetActiveUserSessions(ctx, query.UserID)
}

// Project Membership Query Handlers

func (h *UserQueryHandler) HandleGetProjectMembers(ctx context.Context, query GetProjectMembersQuery) ([]*User, error) {
	return h.projectionStore.GetProjectMembers(ctx, query.ProjectID)
}

func (h *UserQueryHandler) HandleGetUserProjects(ctx context.Context, query GetUserProjectsQuery) ([]*ProjectMembership, error) {
	return h.projectionStore.GetUserProjects(ctx, query.UserID)
}

func (h *UserQueryHandler) HandleGetProjectMembership(ctx context.Context, query GetProjectMembershipQuery) (*ProjectMembership, error) {
	return h.projectionStore.GetProjectMembership(ctx, query.UserID, query.ProjectID)
}

func (h *UserQueryHandler) HandleCheckUserProjectAccess(ctx context.Context, query CheckUserProjectAccessQuery) (bool, error) {
	return h.projectionStore.CheckUserProjectAccess(ctx, query.UserID, query.ProjectID)
}

// Invitation Query Handlers

func (h *UserQueryHandler) HandleGetProjectInvitations(ctx context.Context, query GetProjectInvitationsQuery) ([]*UserInvitation, error) {
	return h.projectionStore.GetProjectInvitations(ctx, query.ProjectID)
}

func (h *UserQueryHandler) HandleGetUserInvitations(ctx context.Context, query GetUserInvitationsQuery) ([]*UserInvitation, error) {
	return h.projectionStore.GetUserInvitations(ctx, query.Email)
}

func (h *UserQueryHandler) HandleGetInvitationByToken(ctx context.Context, query GetInvitationByTokenQuery) (*UserInvitation, error) {
	return h.projectionStore.GetInvitationByToken(ctx, query.Token)
}

// Token Query Handlers

func (h *UserQueryHandler) HandleGetEmailVerificationToken(ctx context.Context, query GetEmailVerificationTokenQuery) (*EmailVerificationToken, error) {
	return h.projectionStore.GetEmailVerificationToken(ctx, query.Token)
}

func (h *UserQueryHandler) HandleGetPasswordResetToken(ctx context.Context, query GetPasswordResetTokenQuery) (*PasswordResetToken, error) {
	return h.projectionStore.GetPasswordResetToken(ctx, query.Token)
}

func (h *UserQueryHandler) HandleGetMagicMailToken(ctx context.Context, query GetMagicMailTokenQuery) (*MagicMailToken, error) {
	return h.projectionStore.GetMagicMailToken(ctx, query.Token)
}

// Administrative Query Handlers

func (h *UserQueryHandler) HandleGetUserActivity(ctx context.Context, query GetUserActivityQuery) ([]interface{}, error) {
	return h.projectionStore.GetUserActivity(ctx, query.UserID, query.Limit, query.Offset)
}

func (h *UserQueryHandler) HandleGetUserStats(ctx context.Context, query GetUserStatsQuery) (map[string]interface{}, error) {
	return h.projectionStore.GetUserStats(ctx, query.UserID)
}

func (h *UserQueryHandler) HandleGetSystemUserStats(ctx context.Context, query GetSystemUserStatsQuery) (map[string]interface{}, error) {
	return h.projectionStore.GetSystemUserStats(ctx)
}

// Authentication Query Handlers

func (h *UserQueryHandler) HandleValidateUserCredentials(ctx context.Context, query ValidateUserCredentialsQuery) (*User, error) {
	// TODO: Implement credential validation logic
	// This should validate the password against the stored hash
	// For now, return nil to indicate not implemented
	return nil, nil
}

func (h *UserQueryHandler) HandleCheckUserMFAStatus(ctx context.Context, query CheckUserMFAStatusQuery) (bool, error) {
	// TODO: Implement MFA status check logic
	// This should check if the user has MFA enabled
	// For now, return false to indicate MFA not enabled
	return false, nil
}

func (h *UserQueryHandler) HandleGetUserWebAuthnCredentials(ctx context.Context, query GetUserWebAuthnCredentialsQuery) ([]interface{}, error) {
	// Get all user credentials
	credentials, err := h.userStore.GetUserCredentials(ctx, query.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user credentials: %w", err)
	}

	// Filter for WebAuthn credentials
	var webauthnCredentials []interface{}
	for _, cred := range credentials {
		if cred.Type == "webauthn" {
			// Convert credential data to appropriate format
			credentialData := map[string]interface{}{
				"id":         cred.ID,
				"user_id":    cred.UserID,
				"type":       cred.Type,
				"provider":   cred.Provider,
				"created_at": cred.CreatedAt,
				"updated_at": cred.UpdatedAt,
				// Note: We don't return the actual credential data for security
			}
			webauthnCredentials = append(webauthnCredentials, credentialData)
		}
	}

	return webauthnCredentials, nil
}

// SSO Query Handlers

func (h *UserQueryHandler) HandleGetSSOProviders(ctx context.Context, query GetSSOProvidersQuery) ([]string, error) {
	// This would typically return the list of enabled SSO providers
	// For now, return the supported providers
	return GetSupportedSSOProviders(), nil
}

func (h *UserQueryHandler) HandleGetSSOProviderConfig(ctx context.Context, query GetSSOProviderConfigQuery) (*SSOProviderConfig, error) {
	// This would return the configuration for a specific SSO provider
	// For security reasons, sensitive information like client secrets should not be returned
	// This is a placeholder implementation
	return nil, fmt.Errorf("SSO provider configuration not available")
}

func (h *UserQueryHandler) HandleGetUserSSOCredentials(ctx context.Context, query GetUserSSOCredentialsQuery) ([]*UserCredential, error) {
	// Get all SSO credentials for the user
	credentials, err := h.userStore.GetUserCredentials(ctx, query.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user credentials: %w", err)
	}

	// Filter for SSO credentials only
	ssoCredentials := make([]*UserCredential, 0)
	for _, credential := range credentials {
		if credential.Type == "sso" {
			ssoCredentials = append(ssoCredentials, credential)
		}
	}

	return ssoCredentials, nil
}

func (h *UserQueryHandler) HandleValidateSSOProvider(ctx context.Context, query ValidateSSOProviderQuery) (bool, error) {
	// Check if the SSO provider is supported and configured
	return IsSSOProviderSupported(query.Provider), nil
}
