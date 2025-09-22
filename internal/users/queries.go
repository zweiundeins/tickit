package users

// User Queries

type GetUserByIDQuery struct {
	ID string `json:"id"`
}

func (q GetUserByIDQuery) GetQueryType() string { return "GetUserByID" }

type GetUsersByIDQuery struct {
	IDs []string `json:"ids"`
}

func (q GetUsersByIDQuery) GetQueryType() string { return "GetUsersByID" }

type GetUserByEmailQuery struct {
	Email string `json:"email"`
}

func (q GetUserByEmailQuery) GetQueryType() string { return "GetUserByEmail" }

type GetUserByUsernameQuery struct {
	Username string `json:"username"`
}

func (q GetUserByUsernameQuery) GetQueryType() string { return "GetUserByUsername" }

type ListUsersQuery struct {
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
	Status string `json:"status,omitempty"` // Filter by status
	Role   string `json:"role,omitempty"`   // Filter by role
}

func (q ListUsersQuery) GetQueryType() string { return "ListUsers" }

type SearchUsersQuery struct {
	Query  string `json:"query"` // Search term
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

func (q SearchUsersQuery) GetQueryType() string { return "SearchUsers" }

// User Credential Queries

type GetUserCredentialsQuery struct {
	UserID string `json:"user_id"`
}

func (q GetUserCredentialsQuery) GetQueryType() string { return "GetUserCredentials" }

type GetUserCredentialByProviderQuery struct {
	UserID   string `json:"user_id"`
	Provider string `json:"provider"`
}

func (q GetUserCredentialByProviderQuery) GetQueryType() string { return "GetUserCredentialByProvider" }

type GetUserByOAuthProviderQuery struct {
	Provider   string `json:"provider"`
	ProviderID string `json:"provider_id"`
}

func (q GetUserByOAuthProviderQuery) GetQueryType() string { return "GetUserByOAuthProvider" }

// Session Queries

type GetUserSessionQuery struct {
	SessionID string `json:"session_id"`
}

func (q GetUserSessionQuery) GetQueryType() string { return "GetUserSession" }

type GetUserSessionsQuery struct {
	UserID string `json:"user_id"`
}

func (q GetUserSessionsQuery) GetQueryType() string { return "GetUserSessions" }

type GetActiveUserSessionsQuery struct {
	UserID string `json:"user_id"`
}

func (q GetActiveUserSessionsQuery) GetQueryType() string { return "GetActiveUserSessions" }

// Project Membership Queries

type GetProjectMembersQuery struct {
	ProjectID string `json:"project_id"`
}

func (q GetProjectMembersQuery) GetQueryType() string { return "GetProjectMembers" }

type GetUserProjectsQuery struct {
	UserID string `json:"user_id"`
}

func (q GetUserProjectsQuery) GetQueryType() string { return "GetUserProjects" }

type GetProjectMembershipQuery struct {
	UserID    string `json:"user_id"`
	ProjectID string `json:"project_id"`
}

func (q GetProjectMembershipQuery) GetQueryType() string { return "GetProjectMembership" }

type CheckUserProjectAccessQuery struct {
	UserID    string `json:"user_id"`
	ProjectID string `json:"project_id"`
}

func (q CheckUserProjectAccessQuery) GetQueryType() string { return "CheckUserProjectAccess" }

// Invitation Queries

type GetProjectInvitationsQuery struct {
	ProjectID string `json:"project_id"`
}

func (q GetProjectInvitationsQuery) GetQueryType() string { return "GetProjectInvitations" }

type GetUserInvitationsQuery struct {
	Email string `json:"email"`
}

func (q GetUserInvitationsQuery) GetQueryType() string { return "GetUserInvitations" }

type GetInvitationByTokenQuery struct {
	Token string `json:"token"`
}

func (q GetInvitationByTokenQuery) GetQueryType() string { return "GetInvitationByToken" }

// Token Queries

type GetEmailVerificationTokenQuery struct {
	Token string `json:"token"`
}

func (q GetEmailVerificationTokenQuery) GetQueryType() string { return "GetEmailVerificationToken" }

type GetPasswordResetTokenQuery struct {
	Token string `json:"token"`
}

func (q GetPasswordResetTokenQuery) GetQueryType() string { return "GetPasswordResetToken" }

type GetMagicMailTokenQuery struct {
	Token string `json:"token"`
}

func (q GetMagicMailTokenQuery) GetQueryType() string { return "GetMagicMailToken" }

// Administrative Queries

type GetUserActivityQuery struct {
	UserID string `json:"user_id"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

func (q GetUserActivityQuery) GetQueryType() string { return "GetUserActivity" }

type GetUserStatsQuery struct {
	UserID string `json:"user_id"`
}

func (q GetUserStatsQuery) GetQueryType() string { return "GetUserStats" }

type GetSystemUserStatsQuery struct{}

func (q GetSystemUserStatsQuery) GetQueryType() string { return "GetSystemUserStats" }

// Authentication Queries

type ValidateUserCredentialsQuery struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (q ValidateUserCredentialsQuery) GetQueryType() string { return "ValidateUserCredentials" }

type CheckUserMFAStatusQuery struct {
	UserID string `json:"user_id"`
}

func (q CheckUserMFAStatusQuery) GetQueryType() string { return "CheckUserMFAStatus" }

type GetUserWebAuthnCredentialsQuery struct {
	UserID string `json:"user_id"`
}

func (q GetUserWebAuthnCredentialsQuery) GetQueryType() string { return "GetUserWebAuthnCredentials" }

// SSO Queries

type GetSSOProvidersQuery struct{}

func (q GetSSOProvidersQuery) GetQueryType() string { return "GetSSOProviders" }

type GetSSOProviderConfigQuery struct {
	Provider string `json:"provider"`
}

func (q GetSSOProviderConfigQuery) GetQueryType() string { return "GetSSOProviderConfig" }

type GetUserSSOCredentialsQuery struct {
	UserID string `json:"user_id"`
}

func (q GetUserSSOCredentialsQuery) GetQueryType() string { return "GetUserSSOCredentials" }

type ValidateSSOProviderQuery struct {
	Provider string `json:"provider"`
}

func (q ValidateSSOProviderQuery) GetQueryType() string { return "ValidateSSOProvider" }
