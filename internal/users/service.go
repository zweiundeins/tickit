package users

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mbolli/tickit/internal/shared"
)

// UserService provides high-level user management operations
// It follows the same pattern as ProjectService in the existing codebase
type UserService struct {
	commandBus *shared.CommandBus
	queryBus   *shared.QueryBus
}

// NewUserService creates a new UserService instance
func NewUserService(commandBus *shared.CommandBus, queryBus *shared.QueryBus) *UserService {
	return &UserService{
		commandBus: commandBus,
		queryBus:   queryBus,
	}
}

// User Registration

func (s *UserService) RegisterUser(ctx context.Context, email, password, firstName, lastName, username string) (string, error) {
	id := uuid.New().String()
	cmd := RegisterUserCommand{
		ID:        id,
		Email:     email,
		Password:  password,
		FirstName: firstName,
		LastName:  lastName,
		Username:  username,
	}
	if err := s.commandBus.Execute(ctx, cmd); err != nil {
		return "", err
	}
	return id, nil
}

func (s *UserService) VerifyEmail(ctx context.Context, token string) error {
	cmd := VerifyEmailCommand{Token: token}
	return s.commandBus.Execute(ctx, cmd)
}

// Authentication

func (s *UserService) LoginUser(ctx context.Context, email, password, ipAddress, userAgent string) (*User, error) {
	cmd := LoginUserCommand{
		Email:     email,
		Password:  password,
		IPAddress: ipAddress,
		UserAgent: userAgent,
	}
	if err := s.commandBus.Execute(ctx, cmd); err != nil {
		return nil, err
	}

	// Get the user after successful login
	return s.GetUserByEmail(ctx, email)
}

func (s *UserService) LoginOAuth(ctx context.Context, provider, code, state, ipAddress, userAgent string) (*User, error) {
	cmd := LoginOAuthCommand{
		Provider:  provider,
		Code:      code,
		State:     state,
		IPAddress: ipAddress,
		UserAgent: userAgent,
	}
	if err := s.commandBus.Execute(ctx, cmd); err != nil {
		return nil, err
	}

	// The command handler should return the user, but for now we'll need to implement
	// a way to get the user from the OAuth provider info
	// This will be implemented in the command handler
	return nil, fmt.Errorf("OAuth login result retrieval not yet implemented")
}

func (s *UserService) LoginWebAuthn(ctx context.Context, userID string, assertion []byte, ipAddress, userAgent string) (*User, error) {
	cmd := LoginWebAuthnCommand{
		UserID:    userID,
		Assertion: assertion,
		IPAddress: ipAddress,
		UserAgent: userAgent,
	}
	if err := s.commandBus.Execute(ctx, cmd); err != nil {
		return nil, err
	}

	return s.GetUserByID(ctx, userID)
}

func (s *UserService) SendMagicMail(ctx context.Context, email string) error {
	cmd := SendMagicMailCommand{Email: email}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) LoginMagicMail(ctx context.Context, token, ipAddress, userAgent string) (*User, *UserSession, error) {
	cmd := LoginMagicMailCommand{
		Token:     token,
		IPAddress: ipAddress,
		UserAgent: userAgent,
	}
	if err := s.commandBus.Execute(ctx, cmd); err != nil {
		return nil, nil, err
	}

	// For now, return nil values as the command execution handles the login
	// In a full implementation, we would need to query for the user and session
	// after the command has been processed
	return nil, nil, nil
}

func (s *UserService) LogoutUser(ctx context.Context, sessionID, userID string) error {
	cmd := LogoutUserCommand{
		SessionID: sessionID,
		UserID:    userID,
	}
	return s.commandBus.Execute(ctx, cmd)
}

// Password Management

func (s *UserService) ForgotPassword(ctx context.Context, email string) error {
	cmd := ForgotPasswordCommand{Email: email}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) ResetPassword(ctx context.Context, token, newPassword string) error {
	cmd := ResetPasswordCommand{
		Token:       token,
		NewPassword: newPassword,
	}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	cmd := ChangePasswordCommand{
		UserID:      userID,
		OldPassword: oldPassword,
		NewPassword: newPassword,
	}
	return s.commandBus.Execute(ctx, cmd)
}

// Multi-Factor Authentication

func (s *UserService) BeginMFARegistration(ctx context.Context, userID string) error {
	cmd := BeginMFARegistrationCommand{UserID: userID}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) FinishMFARegistration(ctx context.Context, userID string, response []byte) error {
	cmd := FinishMFARegistrationCommand{
		UserID:   userID,
		Response: response,
	}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) EnableMFA(ctx context.Context, userID string, credential []byte) error {
	cmd := EnableMFACommand{
		UserID:     userID,
		Credential: credential,
	}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) DisableMFA(ctx context.Context, userID, credentialID string) error {
	cmd := DisableMFACommand{
		UserID:       userID,
		CredentialID: credentialID,
	}
	return s.commandBus.Execute(ctx, cmd)
}

// User Profile Management

func (s *UserService) UpdateUserProfile(ctx context.Context, userID string, email, username, firstName, lastName, avatarURL *string) error {
	cmd := UpdateUserProfileCommand{
		UserID:    userID,
		Email:     email,
		Username:  username,
		FirstName: firstName,
		LastName:  lastName,
		AvatarURL: avatarURL,
	}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) DeactivateUser(ctx context.Context, userID string) error {
	cmd := DeactivateUserCommand{UserID: userID}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) ReactivateUser(ctx context.Context, userID string) error {
	cmd := ReactivateUserCommand{UserID: userID}
	return s.commandBus.Execute(ctx, cmd)
}

// Project Membership

func (s *UserService) InviteUserToProject(ctx context.Context, projectID, email, role, invitedBy string) error {
	cmd := InviteUserToProjectCommand{
		ProjectID: projectID,
		Email:     email,
		Role:      role,
		InvitedBy: invitedBy,
	}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) AcceptProjectInvitation(ctx context.Context, token string) error {
	cmd := AcceptProjectInvitationCommand{Token: token}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) DeclineProjectInvitation(ctx context.Context, token string) error {
	cmd := DeclineProjectInvitationCommand{Token: token}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) AddUserToProject(ctx context.Context, userID, projectID, role, addedBy string) error {
	cmd := AddUserToProjectCommand{
		UserID:    userID,
		ProjectID: projectID,
		Role:      role,
		AddedBy:   addedBy,
	}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) RemoveUserFromProject(ctx context.Context, userID, projectID, removedBy string) error {
	cmd := RemoveUserFromProjectCommand{
		UserID:    userID,
		ProjectID: projectID,
		RemovedBy: removedBy,
	}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) UpdateProjectMembership(ctx context.Context, userID, projectID, role, updatedBy string) error {
	cmd := UpdateProjectMembershipCommand{
		UserID:    userID,
		ProjectID: projectID,
		Role:      role,
		UpdatedBy: updatedBy,
	}
	return s.commandBus.Execute(ctx, cmd)
}

// Administrative Functions

func (s *UserService) AssignUserRole(ctx context.Context, userID, role, assignedBy string) error {
	cmd := AssignUserRoleCommand{
		UserID:     userID,
		Role:       role,
		AssignedBy: assignedBy,
	}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) PatchUser(ctx context.Context, input PatchUserInput, updatedBy string) error {
	cmd := PatchUserCommand{
		ID:            input.ID,
		Email:         input.Email,
		Username:      input.Username,
		FirstName:     input.FirstName,
		LastName:      input.LastName,
		AvatarURL:     input.AvatarURL,
		Role:          input.Role,
		Status:        input.Status,
		EmailVerified: input.EmailVerified,
		UpdatedBy:     updatedBy,
	}
	return s.commandBus.Execute(ctx, cmd)
}

// OAuth Account Management

func (s *UserService) LinkOAuthAccount(ctx context.Context, userID, provider, providerID, code, state string) error {
	cmd := LinkOAuthAccountCommand{
		UserID:     userID,
		Provider:   provider,
		ProviderID: providerID,
		Code:       code,
		State:      state,
	}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) UnlinkOAuthAccount(ctx context.Context, userID, provider string) error {
	cmd := UnlinkOAuthAccountCommand{
		UserID:   userID,
		Provider: provider,
	}
	return s.commandBus.Execute(ctx, cmd)
}

// Session Management

func (s *UserService) CreateUserSession(ctx context.Context, userID, sessionID, ipAddress, userAgent string, expiresAt time.Time) error {
	cmd := CreateUserSessionCommand{
		UserID:    userID,
		SessionID: sessionID,
		ExpiresAt: expiresAt,
		IPAddress: ipAddress,
		UserAgent: userAgent,
	}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) InvalidateUserSession(ctx context.Context, sessionID string) error {
	cmd := InvalidateUserSessionCommand{SessionID: sessionID}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *UserService) InvalidateAllUserSessions(ctx context.Context, userID string) error {
	cmd := InvalidateAllUserSessionsCommand{UserID: userID}
	return s.commandBus.Execute(ctx, cmd)
}

// Query Methods

func (s *UserService) GetUserByID(ctx context.Context, id string) (*User, error) {
	query := GetUserByIDQuery{ID: id}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	user, _ := result.(*User)
	return user, nil
}

func (s *UserService) GetUsersByID(ctx context.Context, ids []string) ([]*User, error) {
	query := GetUsersByIDQuery{IDs: ids}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	users, _ := result.([]*User)
	return users, nil
}

func (s *UserService) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := GetUserByEmailQuery{Email: email}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	user, _ := result.(*User)
	return user, nil
}

func (s *UserService) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	query := GetUserByUsernameQuery{Username: username}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	user, _ := result.(*User)
	return user, nil
}

func (s *UserService) ListUsers(ctx context.Context, limit, offset int, status, role string) ([]*User, error) {
	query := ListUsersQuery{
		Limit:  limit,
		Offset: offset,
		Status: status,
		Role:   role,
	}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	users, _ := result.([]*User)
	return users, nil
}

func (s *UserService) SearchUsers(ctx context.Context, searchQuery string, limit, offset int) ([]*User, error) {
	query := SearchUsersQuery{
		Query:  searchQuery,
		Limit:  limit,
		Offset: offset,
	}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	users, _ := result.([]*User)
	return users, nil
}

func (s *UserService) GetProjectMembers(ctx context.Context, projectID string) ([]*User, error) {
	query := GetProjectMembersQuery{ProjectID: projectID}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	users, _ := result.([]*User)
	return users, nil
}

func (s *UserService) GetUserProjects(ctx context.Context, userID string) ([]*ProjectMembership, error) {
	query := GetUserProjectsQuery{UserID: userID}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	memberships, _ := result.([]*ProjectMembership)
	return memberships, nil
}

func (s *UserService) CheckUserProjectAccess(ctx context.Context, userID, projectID string) (bool, error) {
	query := CheckUserProjectAccessQuery{
		UserID:    userID,
		ProjectID: projectID,
	}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return false, err
	}
	hasAccess, _ := result.(bool)
	return hasAccess, nil
}

func (s *UserService) GetUserSession(ctx context.Context, sessionID string) (*UserSession, error) {
	query := GetUserSessionQuery{SessionID: sessionID}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	return result.(*UserSession), nil
}

func (s *UserService) GetUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	query := GetUserSessionsQuery{UserID: userID}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	sessions, _ := result.([]*UserSession)
	return sessions, nil
}

func (s *UserService) GetActiveUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	query := GetActiveUserSessionsQuery{UserID: userID}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	return result.([]*UserSession), nil
}

func (s *UserService) GetProjectInvitations(ctx context.Context, projectID string) ([]*UserInvitation, error) {
	query := GetProjectInvitationsQuery{ProjectID: projectID}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	invitations, _ := result.([]*UserInvitation)
	return invitations, nil
}

func (s *UserService) GetUserInvitations(ctx context.Context, email string) ([]*UserInvitation, error) {
	query := GetUserInvitationsQuery{Email: email}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	invitations, _ := result.([]*UserInvitation)
	return invitations, nil
}

// Administrative Query Methods

func (s *UserService) GetUserActivity(ctx context.Context, userID string, limit, offset int) ([]interface{}, error) {
	query := GetUserActivityQuery{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	activities, _ := result.([]interface{})
	return activities, nil
}

func (s *UserService) GetUserStats(ctx context.Context, userID string) (map[string]interface{}, error) {
	query := GetUserStatsQuery{UserID: userID}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	stats, _ := result.(map[string]interface{})
	return stats, nil
}

func (s *UserService) GetSystemUserStats(ctx context.Context) (map[string]interface{}, error) {
	query := GetSystemUserStatsQuery{}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	stats, _ := result.(map[string]interface{})
	return stats, nil
}

func (s *UserService) ExportUserData(ctx context.Context, format string) ([]byte, error) {
	// Get all users
	users, err := s.ListUsers(ctx, 0, 0, "", "")
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}

	switch format {
	case "json":
		return json.Marshal(users)
	case "csv":
		return s.exportUsersToCSV(users)
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}

func (s *UserService) exportUsersToCSV(users []*User) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write header
	header := []string{
		"ID", "Email", "Username", "FirstName", "LastName",
		"Role", "Status", "EmailVerified", "CreatedAt", "UpdatedAt", "LastLoginAt",
	}
	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write user data
	for _, user := range users {
		lastLogin := ""
		if user.LastLoginAt != nil {
			lastLogin = user.LastLoginAt.Format(time.RFC3339)
		}

		record := []string{
			user.ID,
			user.Email,
			user.Username,
			user.FirstName,
			user.LastName,
			user.Role,
			user.Status,
			fmt.Sprintf("%t", user.EmailVerified),
			user.CreatedAt.Format(time.RFC3339),
			user.UpdatedAt.Format(time.RFC3339),
			lastLogin,
		}
		if err := writer.Write(record); err != nil {
			return nil, fmt.Errorf("failed to write CSV record: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("CSV writer error: %w", err)
	}

	return buf.Bytes(), nil
}

func (s *UserService) GetUserCredentials(ctx context.Context, userID string) ([]*UserCredential, error) {
	query := GetUserCredentialsQuery{UserID: userID}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	credentials, _ := result.([]*UserCredential)
	return credentials, nil
}

func (s *UserService) CheckUserMFAStatus(ctx context.Context, userID string) (bool, error) {
	query := CheckUserMFAStatusQuery{UserID: userID}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return false, err
	}
	mfaEnabled, _ := result.(bool)
	return mfaEnabled, nil
}
