package users

import (
	"context"
	"time"

	"github.com/mbolli/tickit/internal/shared"
	"github.com/stretchr/testify/mock"
)

// MockUserStore is a mock implementation of UserStore for testing
type MockUserStore struct {
	users                map[string]*User
	credentials          map[string][]*UserCredential
	sessions             map[string]*UserSession
	projectMemberships   map[string][]*ProjectMembership
	emailTokens          map[string]*EmailVerificationToken
	passwordResetTokens  map[string]*PasswordResetToken
	magicMailTokens      map[string]*MagicMailToken
	invitations          map[string]*UserInvitation
}

func NewMockUserStore() *MockUserStore {
	return &MockUserStore{
		users:               make(map[string]*User),
		credentials:         make(map[string][]*UserCredential),
		sessions:            make(map[string]*UserSession),
		projectMemberships:  make(map[string][]*ProjectMembership),
		emailTokens:         make(map[string]*EmailVerificationToken),
		passwordResetTokens: make(map[string]*PasswordResetToken),
		magicMailTokens:     make(map[string]*MagicMailToken),
		invitations:         make(map[string]*UserInvitation),
	}
}

// User CRUD operations
func (m *MockUserStore) GetUser(ctx context.Context, userID string) (*User, error) {
	user, exists := m.users[userID]
	if !exists {
		return nil, nil
	}
	return user, nil
}

func (m *MockUserStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	for _, user := range m.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, nil
}

func (m *MockUserStore) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	for _, user := range m.users {
		if user.Username == username {
			return user, nil
		}
	}
	return nil, nil
}

func (m *MockUserStore) ListUsers(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*User, error) {
	var users []*User
	for _, user := range m.users {
		users = append(users, user)
	}
	return users, nil
}

func (m *MockUserStore) SearchUsers(ctx context.Context, query string, limit, offset int) ([]*User, error) {
	return m.ListUsers(ctx, limit, offset, nil)
}

func (m *MockUserStore) CreateUser(ctx context.Context, user *User) error {
	m.users[user.ID] = user
	return nil
}

func (m *MockUserStore) UpdateUser(ctx context.Context, user *User) error {
	m.users[user.ID] = user
	return nil
}

func (m *MockUserStore) PatchUser(ctx context.Context, patch *PatchUserInput) error {
	if user, exists := m.users[patch.ID]; exists {
		if patch.FirstName != nil {
			user.FirstName = *patch.FirstName
		}
		if patch.LastName != nil {
			user.LastName = *patch.LastName
		}
		if patch.Email != nil {
			user.Email = *patch.Email
		}
		if patch.Username != nil {
			user.Username = *patch.Username
		}
		user.UpdatedAt = time.Now()
	}
	return nil
}

func (m *MockUserStore) DeleteUser(ctx context.Context, userID string) error {
	delete(m.users, userID)
	return nil
}

// User Credential operations
func (m *MockUserStore) GetUserCredentials(ctx context.Context, userID string) ([]*UserCredential, error) {
	return m.credentials[userID], nil
}

func (m *MockUserStore) GetUserCredentialByProvider(ctx context.Context, userID, provider string) (*UserCredential, error) {
	credentials := m.credentials[userID]
	for _, cred := range credentials {
		if cred.Provider == provider {
			return cred, nil
		}
	}
	return nil, nil
}

func (m *MockUserStore) GetUserByOAuthProvider(ctx context.Context, provider, providerID string) (*User, error) {
	for userID, credentials := range m.credentials {
		for _, cred := range credentials {
			if cred.Provider == provider && cred.ProviderID == providerID {
				return m.users[userID], nil
			}
		}
	}
	return nil, nil
}

func (m *MockUserStore) CreateUserCredential(ctx context.Context, credential *UserCredential) error {
	m.credentials[credential.UserID] = append(m.credentials[credential.UserID], credential)
	return nil
}

func (m *MockUserStore) UpdateUserCredential(ctx context.Context, credential *UserCredential) error {
	credentials := m.credentials[credential.UserID]
	for i, cred := range credentials {
		if cred.ID == credential.ID {
			credentials[i] = credential
			break
		}
	}
	return nil
}

func (m *MockUserStore) DeleteUserCredential(ctx context.Context, credentialID string) error {
	for userID, credentials := range m.credentials {
		for i, cred := range credentials {
			if cred.ID == credentialID {
				m.credentials[userID] = append(credentials[:i], credentials[i+1:]...)
				return nil
			}
		}
	}
	return nil
}

// Session operations
func (m *MockUserStore) GetUserSession(ctx context.Context, sessionID string) (*UserSession, error) {
	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, nil
	}
	return session, nil
}

func (m *MockUserStore) GetUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	var sessions []*UserSession
	for _, session := range m.sessions {
		if session.UserID == userID {
			sessions = append(sessions, session)
		}
	}
	return sessions, nil
}

func (m *MockUserStore) GetActiveUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	var sessions []*UserSession
	now := time.Now()
	for _, session := range m.sessions {
		if session.UserID == userID && session.ExpiresAt.After(now) {
			sessions = append(sessions, session)
		}
	}
	return sessions, nil
}

func (m *MockUserStore) CreateUserSession(ctx context.Context, session *UserSession) error {
	m.sessions[session.ID] = session
	return nil
}

func (m *MockUserStore) UpdateUserSession(ctx context.Context, session *UserSession) error {
	m.sessions[session.ID] = session
	return nil
}

func (m *MockUserStore) DeleteUserSession(ctx context.Context, sessionID string) error {
	delete(m.sessions, sessionID)
	return nil
}

func (m *MockUserStore) DeleteAllUserSessions(ctx context.Context, userID string) error {
	for sessionID, session := range m.sessions {
		if session.UserID == userID {
			delete(m.sessions, sessionID)
		}
	}
	return nil
}

func (m *MockUserStore) InvalidateAllUserSessions(ctx context.Context, userID string) error {
	return m.DeleteAllUserSessions(ctx, userID)
}

func (m *MockUserStore) InvalidateAllUserSessionsExcept(ctx context.Context, userID, exceptSessionID string) error {
	for sessionID, session := range m.sessions {
		if session.UserID == userID && sessionID != exceptSessionID {
			delete(m.sessions, sessionID)
		}
	}
	return nil
}

func (m *MockUserStore) GetEmailVerificationTokenByUserID(ctx context.Context, userID string) (*EmailVerificationToken, error) {
	for _, token := range m.emailTokens {
		if token.UserID == userID {
			return token, nil
		}
	}
	return nil, nil
}

func (m *MockUserStore) DeleteExpiredSessions(ctx context.Context) (int, error) {
	count := 0
	now := time.Now()
	for sessionID, session := range m.sessions {
		if session.ExpiresAt.Before(now) {
			delete(m.sessions, sessionID)
			count++
		}
	}
	return count, nil
}

// Project Membership operations
func (m *MockUserStore) GetProjectMembers(ctx context.Context, projectID string) ([]*User, error) {
	var users []*User
	for _, membership := range m.projectMemberships[projectID] {
		if user := m.users[membership.UserID]; user != nil {
			users = append(users, user)
		}
	}
	return users, nil
}

func (m *MockUserStore) GetUserProjects(ctx context.Context, userID string) ([]*ProjectMembership, error) {
	var memberships []*ProjectMembership
	for _, membershipList := range m.projectMemberships {
		for _, membership := range membershipList {
			if membership.UserID == userID {
				memberships = append(memberships, membership)
			}
		}
	}
	return memberships, nil
}

func (m *MockUserStore) GetProjectMembership(ctx context.Context, userID, projectID string) (*ProjectMembership, error) {
	for _, membership := range m.projectMemberships[projectID] {
		if membership.UserID == userID {
			return membership, nil
		}
	}
	return nil, nil
}

func (m *MockUserStore) CheckUserProjectAccess(ctx context.Context, userID, projectID string) (bool, error) {
	membership, err := m.GetProjectMembership(ctx, userID, projectID)
	return membership != nil, err
}

func (m *MockUserStore) CreateProjectMembership(ctx context.Context, membership *ProjectMembership) error {
	m.projectMemberships[membership.ProjectID] = append(m.projectMemberships[membership.ProjectID], membership)
	return nil
}

func (m *MockUserStore) UpdateProjectMembership(ctx context.Context, membership *ProjectMembership) error {
	memberships := m.projectMemberships[membership.ProjectID]
	for i, m := range memberships {
		if m.UserID == membership.UserID {
			memberships[i] = membership
			break
		}
	}
	return nil
}

func (m *MockUserStore) DeleteProjectMembership(ctx context.Context, userID, projectID string) error {
	memberships := m.projectMemberships[projectID]
	for i, membership := range memberships {
		if membership.UserID == userID {
			m.projectMemberships[projectID] = append(memberships[:i], memberships[i+1:]...)
			break
		}
	}
	return nil
}

// Token operations
func (m *MockUserStore) GetEmailVerificationToken(ctx context.Context, token string) (*EmailVerificationToken, error) {
	emailToken, exists := m.emailTokens[token]
	if !exists {
		return nil, nil
	}
	return emailToken, nil
}

func (m *MockUserStore) CreateEmailVerificationToken(ctx context.Context, token *EmailVerificationToken) error {
	m.emailTokens[token.Token] = token
	return nil
}

func (m *MockUserStore) DeleteEmailVerificationToken(ctx context.Context, token string) error {
	delete(m.emailTokens, token)
	return nil
}

func (m *MockUserStore) GetPasswordResetToken(ctx context.Context, token string) (*PasswordResetToken, error) {
	resetToken, exists := m.passwordResetTokens[token]
	if !exists {
		return nil, nil
	}
	return resetToken, nil
}

func (m *MockUserStore) CreatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error {
	m.passwordResetTokens[token.Token] = token
	return nil
}

func (m *MockUserStore) UpdatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error {
	m.passwordResetTokens[token.Token] = token
	return nil
}

func (m *MockUserStore) DeletePasswordResetToken(ctx context.Context, token string) error {
	delete(m.passwordResetTokens, token)
	return nil
}

func (m *MockUserStore) GetMagicMailToken(ctx context.Context, token string) (*MagicMailToken, error) {
	magicToken, exists := m.magicMailTokens[token]
	if !exists {
		return nil, nil
	}
	return magicToken, nil
}

func (m *MockUserStore) CreateMagicMailToken(ctx context.Context, token *MagicMailToken) error {
	m.magicMailTokens[token.Token] = token
	return nil
}

func (m *MockUserStore) UpdateMagicMailToken(ctx context.Context, token *MagicMailToken) error {
	m.magicMailTokens[token.Token] = token
	return nil
}

func (m *MockUserStore) DeleteMagicMailToken(ctx context.Context, token string) error {
	delete(m.magicMailTokens, token)
	return nil
}

// Invitation operations
func (m *MockUserStore) GetProjectInvitations(ctx context.Context, projectID string) ([]*UserInvitation, error) {
	var invitations []*UserInvitation
	for _, invitation := range m.invitations {
		if invitation.ProjectID == projectID {
			invitations = append(invitations, invitation)
		}
	}
	return invitations, nil
}

func (m *MockUserStore) GetUserInvitations(ctx context.Context, email string) ([]*UserInvitation, error) {
	var invitations []*UserInvitation
	for _, invitation := range m.invitations {
		if invitation.Email == email {
			invitations = append(invitations, invitation)
		}
	}
	return invitations, nil
}

func (m *MockUserStore) GetInvitationByToken(ctx context.Context, token string) (*UserInvitation, error) {
	invitation, exists := m.invitations[token]
	if !exists {
		return nil, nil
	}
	return invitation, nil
}

func (m *MockUserStore) CreateInvitation(ctx context.Context, invitation *UserInvitation) error {
	m.invitations[invitation.Token] = invitation
	return nil
}

func (m *MockUserStore) UpdateInvitation(ctx context.Context, invitation *UserInvitation) error {
	m.invitations[invitation.Token] = invitation
	return nil
}

func (m *MockUserStore) DeleteInvitation(ctx context.Context, invitationID string) error {
	for token, invitation := range m.invitations {
		if invitation.ID == invitationID {
			delete(m.invitations, token)
			break
		}
	}
	return nil
}

// Administrative operations
func (m *MockUserStore) GetUserActivity(ctx context.Context, userID string, limit, offset int) ([]interface{}, error) {
	return []interface{}{}, nil
}

func (m *MockUserStore) GetUserStats(ctx context.Context, userID string) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (m *MockUserStore) GetSystemUserStats(ctx context.Context) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

// MockEventPublisher is a mock implementation of EventPublisher for testing
type MockEventPublisher struct {
	PublishedEvents []shared.Event
}

func NewMockEventPublisher() *MockEventPublisher {
	return &MockEventPublisher{
		PublishedEvents: make([]shared.Event, 0),
	}
}

func (m *MockEventPublisher) PublishEvent(ctx context.Context, event shared.Event) error {
	m.PublishedEvents = append(m.PublishedEvents, event)
	return nil
}

// MockEmailService is a mock implementation of EmailService for testing
type MockEmailService struct {
	SentEmails []EmailRecord
}

type EmailRecord struct {
	Type    string
	Email   string
	Token   string
	Project string
}

func NewMockEmailService() *MockEmailService {
	return &MockEmailService{
		SentEmails: make([]EmailRecord, 0),
	}
}

func (m *MockEmailService) SendVerificationEmail(ctx context.Context, email, token string) error {
	m.SentEmails = append(m.SentEmails, EmailRecord{
		Type:  "verification",
		Email: email,
		Token: token,
	})
	return nil
}

func (m *MockEmailService) SendPasswordResetEmail(ctx context.Context, email, token string) error {
	m.SentEmails = append(m.SentEmails, EmailRecord{
		Type:  "password_reset",
		Email: email,
		Token: token,
	})
	return nil
}

func (m *MockEmailService) SendInvitationEmail(ctx context.Context, email, projectName, token string) error {
	m.SentEmails = append(m.SentEmails, EmailRecord{
		Type:    "invitation",
		Email:   email,
		Token:   token,
		Project: projectName,
	})
	return nil
}

func (m *MockEmailService) SendMagicMailLink(ctx context.Context, email, token string) error {
	m.SentEmails = append(m.SentEmails, EmailRecord{
		Type:  "magic_mail",
		Email: email,
		Token: token,
	})
	return nil
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

// InMemoryUserStore is a more complete in-memory implementation for integration tests
type InMemoryUserStore struct {
	*MockUserStore
}

func NewInMemoryUserStore() *InMemoryUserStore {
	return &InMemoryUserStore{
		MockUserStore: NewMockUserStore(),
	}
}

// MockEventBus is a mock implementation of EventPublisher for testing
type MockEventBus struct {
	mock.Mock
}

func (m *MockEventBus) PublishEvent(ctx context.Context, event shared.Event) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}