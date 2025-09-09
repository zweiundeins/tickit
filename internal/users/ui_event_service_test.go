package users

import (
	"context"
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockEventPublisher for testing
type MockEventPublisherForUI struct {
	mock.Mock
}

func (m *MockEventPublisherForUI) PublishEvent(ctx context.Context, event shared.Event) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

// MockUserStore for testing
type MockUserStoreForUI struct {
	mock.Mock
}

func (m *MockUserStoreForUI) GetUser(ctx context.Context, userID string) (*User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

// Implement other required methods with minimal implementations for testing
func (m *MockUserStoreForUI) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockUserStoreForUI) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) ListUsers(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*User, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) SearchUsers(ctx context.Context, query string, limit, offset int) ([]*User, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) CreateUser(ctx context.Context, user *User) error {
	return nil
}

func (m *MockUserStoreForUI) UpdateUser(ctx context.Context, user *User) error {
	return nil
}

func (m *MockUserStoreForUI) PatchUser(ctx context.Context, patch *PatchUserInput) error {
	return nil
}

func (m *MockUserStoreForUI) DeleteUser(ctx context.Context, userID string) error {
	return nil
}

func (m *MockUserStoreForUI) GetUserCredentials(ctx context.Context, userID string) ([]*UserCredential, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) GetUserCredentialByProvider(ctx context.Context, userID, provider string) (*UserCredential, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) GetUserByOAuthProvider(ctx context.Context, provider, providerID string) (*User, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) CreateUserCredential(ctx context.Context, credential *UserCredential) error {
	return nil
}

func (m *MockUserStoreForUI) UpdateUserCredential(ctx context.Context, credential *UserCredential) error {
	return nil
}

func (m *MockUserStoreForUI) DeleteUserCredential(ctx context.Context, credentialID string) error {
	return nil
}

func (m *MockUserStoreForUI) GetUserSession(ctx context.Context, sessionID string) (*UserSession, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) GetUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) GetActiveUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) CreateUserSession(ctx context.Context, session *UserSession) error {
	return nil
}

func (m *MockUserStoreForUI) UpdateUserSession(ctx context.Context, session *UserSession) error {
	return nil
}

func (m *MockUserStoreForUI) DeleteUserSession(ctx context.Context, sessionID string) error {
	return nil
}

func (m *MockUserStoreForUI) DeleteAllUserSessions(ctx context.Context, userID string) error {
	return nil
}

func (m *MockUserStoreForUI) DeleteExpiredSessions(ctx context.Context) (int, error) {
	return 0, nil
}

func (m *MockUserStoreForUI) GetProjectMembers(ctx context.Context, projectID string) ([]*User, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) GetUserProjects(ctx context.Context, userID string) ([]*ProjectMembership, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) GetProjectMembership(ctx context.Context, userID, projectID string) (*ProjectMembership, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) CheckUserProjectAccess(ctx context.Context, userID, projectID string) (bool, error) {
	return false, nil
}

func (m *MockUserStoreForUI) CreateProjectMembership(ctx context.Context, membership *ProjectMembership) error {
	return nil
}

func (m *MockUserStoreForUI) UpdateProjectMembership(ctx context.Context, membership *ProjectMembership) error {
	return nil
}

func (m *MockUserStoreForUI) DeleteProjectMembership(ctx context.Context, userID, projectID string) error {
	return nil
}

func (m *MockUserStoreForUI) GetEmailVerificationToken(ctx context.Context, token string) (*EmailVerificationToken, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) GetEmailVerificationTokenByUserID(ctx context.Context, userID string) (*EmailVerificationToken, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) CreateEmailVerificationToken(ctx context.Context, token *EmailVerificationToken) error {
	return nil
}

func (m *MockUserStoreForUI) DeleteEmailVerificationToken(ctx context.Context, token string) error {
	return nil
}

func (m *MockUserStoreForUI) GetPasswordResetToken(ctx context.Context, token string) (*PasswordResetToken, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) CreatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error {
	return nil
}

func (m *MockUserStoreForUI) UpdatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error {
	return nil
}

func (m *MockUserStoreForUI) DeletePasswordResetToken(ctx context.Context, token string) error {
	return nil
}

func (m *MockUserStoreForUI) GetMagicMailToken(ctx context.Context, token string) (*MagicMailToken, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) CreateMagicMailToken(ctx context.Context, token *MagicMailToken) error {
	return nil
}

func (m *MockUserStoreForUI) UpdateMagicMailToken(ctx context.Context, token *MagicMailToken) error {
	return nil
}

func (m *MockUserStoreForUI) DeleteMagicMailToken(ctx context.Context, token string) error {
	return nil
}

func (m *MockUserStoreForUI) GetProjectInvitations(ctx context.Context, projectID string) ([]*UserInvitation, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) GetUserInvitations(ctx context.Context, email string) ([]*UserInvitation, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) GetInvitationByToken(ctx context.Context, token string) (*UserInvitation, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) CreateInvitation(ctx context.Context, invitation *UserInvitation) error {
	return nil
}

func (m *MockUserStoreForUI) UpdateInvitation(ctx context.Context, invitation *UserInvitation) error {
	return nil
}

func (m *MockUserStoreForUI) DeleteInvitation(ctx context.Context, invitationID string) error {
	return nil
}

func (m *MockUserStoreForUI) GetUserActivity(ctx context.Context, userID string, limit, offset int) ([]interface{}, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) GetUserStats(ctx context.Context, userID string) (map[string]interface{}, error) {
	return nil, nil
}

func (m *MockUserStoreForUI) GetSystemUserStats(ctx context.Context) (map[string]interface{}, error) {
	return nil, nil
}

func TestUIEventService_HandleUserLoggedIn(t *testing.T) {
	ctx := context.Background()
	mockEventBus := &MockEventPublisher{}
	mockUserStore := &MockUserStore{}
	
	service := NewUIEventService(mockEventBus, mockUserStore)
	
	// Test data
	userID := "user-123"
	user := &User{
		ID:       userID,
		Username: "testuser",
		Status:   "Active",
	}
	
	loginEvent := UserLoggedInEvent{
		UserID:    userID,
		SessionID: "session-123",
		Method:    "password",
		Provider:  "",
		Timestamp: time.Now(),
	}
	
	// Setup mocks
	mockUserStore.On("GetUser", ctx, userID).Return(user, nil)
	mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserLoginStatusEvent")).Return(nil)
	mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserStatusChangedEvent")).Return(nil)
	
	// Execute
	err := service.HandleUserLoggedIn(ctx, loginEvent)
	
	// Verify
	assert.NoError(t, err)
	mockUserStore.AssertExpectations(t)
	mockEventBus.AssertExpectations(t)
	
	// Verify the UI events were published with correct data
	mockEventBus.AssertCalled(t, "PublishEvent", ctx, mock.MatchedBy(func(event shared.Event) bool {
		if uiEvent, ok := event.(UserLoginStatusEvent); ok {
			return uiEvent.UserID == userID &&
				uiEvent.Username == "testuser" &&
				uiEvent.Action == "login" &&
				uiEvent.Method == "password"
		}
		return false
	}))
	
	mockEventBus.AssertCalled(t, "PublishEvent", ctx, mock.MatchedBy(func(event shared.Event) bool {
		if statusEvent, ok := event.(UserStatusChangedEvent); ok {
			return statusEvent.UserID == userID &&
				statusEvent.Username == "testuser" &&
				statusEvent.Status == "Active" &&
				statusEvent.IsOnline == true
		}
		return false
	}))
}

func TestUIEventService_HandleUserLoggedOut(t *testing.T) {
	ctx := context.Background()
	mockEventBus := &MockEventPublisher{}
	mockUserStore := &MockUserStore{}
	
	service := NewUIEventService(mockEventBus, mockUserStore)
	
	// Test data
	userID := "user-123"
	user := &User{
		ID:       userID,
		Username: "testuser",
		Status:   "Active",
	}
	
	logoutEvent := UserLoggedOutEvent{
		UserID:    userID,
		SessionID: "session-123",
		Timestamp: time.Now(),
	}
	
	// Setup mocks
	mockUserStore.On("GetUser", ctx, userID).Return(user, nil)
	mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserLoginStatusEvent")).Return(nil)
	mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserStatusChangedEvent")).Return(nil)
	
	// Execute
	err := service.HandleUserLoggedOut(ctx, logoutEvent)
	
	// Verify
	assert.NoError(t, err)
	mockUserStore.AssertExpectations(t)
	mockEventBus.AssertExpectations(t)
	
	// Verify the UI events were published with correct data
	mockEventBus.AssertCalled(t, "PublishEvent", ctx, mock.MatchedBy(func(event shared.Event) bool {
		if uiEvent, ok := event.(UserLoginStatusEvent); ok {
			return uiEvent.UserID == userID &&
				uiEvent.Username == "testuser" &&
				uiEvent.Action == "logout"
		}
		return false
	}))
	
	mockEventBus.AssertCalled(t, "PublishEvent", ctx, mock.MatchedBy(func(event shared.Event) bool {
		if statusEvent, ok := event.(UserStatusChangedEvent); ok {
			return statusEvent.UserID == userID &&
				statusEvent.Username == "testuser" &&
				statusEvent.Status == "Active" &&
				statusEvent.IsOnline == false
		}
		return false
	}))
}

func TestUIEventService_HandleUserRoleAssigned(t *testing.T) {
	ctx := context.Background()
	mockEventBus := &MockEventPublisher{}
	mockUserStore := &MockUserStore{}
	
	service := NewUIEventService(mockEventBus, mockUserStore)
	
	// Test data
	userID := "user-123"
	adminID := "admin-456"
	user := &User{
		ID:       userID,
		Username: "testuser",
		Status:   "Active",
	}
	admin := &User{
		ID:       adminID,
		Username: "admin",
		Status:   "Active",
	}
	
	roleEvent := UserRoleAssignedEvent{
		UserID:     userID,
		OldRole:    "Developer",
		NewRole:    "ProjectManager",
		AssignedBy: adminID,
		Timestamp:  time.Now(),
	}
	
	// Setup mocks
	mockUserStore.On("GetUser", ctx, userID).Return(user, nil)
	mockUserStore.On("GetUser", ctx, adminID).Return(admin, nil)
	mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserRoleChangedEvent")).Return(nil)
	mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.AdminUserManagementEvent")).Return(nil)
	
	// Execute
	err := service.HandleUserRoleAssigned(ctx, roleEvent)
	
	// Verify
	assert.NoError(t, err)
	mockUserStore.AssertExpectations(t)
	mockEventBus.AssertExpectations(t)
	
	// Verify the UI events were published with correct data
	mockEventBus.AssertCalled(t, "PublishEvent", ctx, mock.MatchedBy(func(event shared.Event) bool {
		if uiEvent, ok := event.(UserRoleChangedEvent); ok {
			return uiEvent.UserID == userID &&
				uiEvent.Username == "testuser" &&
				uiEvent.OldRole == "Developer" &&
				uiEvent.NewRole == "ProjectManager" &&
				uiEvent.ChangedBy == "admin"
		}
		return false
	}))
	
	mockEventBus.AssertCalled(t, "PublishEvent", ctx, mock.MatchedBy(func(event shared.Event) bool {
		if adminEvent, ok := event.(AdminUserManagementEvent); ok {
			return adminEvent.AdminID == adminID &&
				adminEvent.AdminName == "admin" &&
				adminEvent.TargetID == userID &&
				adminEvent.TargetName == "testuser" &&
				adminEvent.Action == "role_changed"
		}
		return false
	}))
}

func TestUIEventService_HandleUserMFAEnabled(t *testing.T) {
	ctx := context.Background()
	mockEventBus := &MockEventPublisher{}
	mockUserStore := &MockUserStore{}
	
	service := NewUIEventService(mockEventBus, mockUserStore)
	
	// Test data
	userID := "user-123"
	user := &User{
		ID:       userID,
		Username: "testuser",
		Status:   "Active",
	}
	
	mfaEvent := UserMFAEnabledEvent{
		UserID:    userID,
		Timestamp: time.Now(),
	}
	
	// Setup mocks
	mockUserStore.On("GetUser", ctx, userID).Return(user, nil)
	mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserMFAStatusChangedEvent")).Return(nil)
	
	// Execute
	err := service.HandleUserMFAEnabled(ctx, mfaEvent)
	
	// Verify
	assert.NoError(t, err)
	mockUserStore.AssertExpectations(t)
	mockEventBus.AssertExpectations(t)
	
	// Verify the UI event was published with correct data
	mockEventBus.AssertCalled(t, "PublishEvent", ctx, mock.MatchedBy(func(event shared.Event) bool {
		if uiEvent, ok := event.(UserMFAStatusChangedEvent); ok {
			return uiEvent.UserID == userID &&
				uiEvent.Username == "testuser" &&
				uiEvent.Enabled == true &&
				uiEvent.Method == "webauthn"
		}
		return false
	}))
}

func TestUIEventService_HandleUserProfileUpdated(t *testing.T) {
	ctx := context.Background()
	mockEventBus := &MockEventPublisher{}
	mockUserStore := &MockUserStore{}
	
	service := NewUIEventService(mockEventBus, mockUserStore)
	
	// Test data
	userID := "user-123"
	user := &User{
		ID:       userID,
		Username: "testuser",
		Status:   "Active",
	}
	
	profileEvent := UserProfileUpdatedEvent{
		UserID: userID,
		Changes: map[string]interface{}{
			"first_name": "John",
			"last_name":  "Doe",
			"password":   "should_not_appear", // This should be filtered out
		},
		UpdatedBy: userID, // Self-update
		Timestamp: time.Now(),
	}
	
	// Setup mocks
	mockUserStore.On("GetUser", ctx, userID).Return(user, nil)
	mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserProfileChangedEvent")).Return(nil)
	
	// Execute
	err := service.HandleUserProfileUpdated(ctx, profileEvent)
	
	// Verify
	assert.NoError(t, err)
	mockUserStore.AssertExpectations(t)
	mockEventBus.AssertExpectations(t)
	
	// Verify the UI event was published with correct data
	mockEventBus.AssertCalled(t, "PublishEvent", ctx, mock.MatchedBy(func(event shared.Event) bool {
		if uiEvent, ok := event.(UserProfileChangedEvent); ok {
			// Check that password field is not included
			for _, field := range uiEvent.FieldsChanged {
				if field == "password" || field == "password_hash" {
					return false
				}
			}
			return uiEvent.UserID == userID &&
				uiEvent.Username == "testuser" &&
				uiEvent.ChangedBy == "testuser" &&
				len(uiEvent.FieldsChanged) == 2 // Should only have first_name and last_name
		}
		return false
	}))
}

func TestUIEventService_HandleNonMatchingEvent(t *testing.T) {
	ctx := context.Background()
	mockEventBus := &MockEventPublisher{}
	mockUserStore := &MockUserStore{}
	
	service := NewUIEventService(mockEventBus, mockUserStore)
	
	// Test with a non-matching event type
	nonMatchingEvent := UserRegisteredEvent{
		User:      &User{ID: "user-123"},
		Timestamp: time.Now(),
	}
	
	// Execute - should not call any methods
	err := service.HandleUserLoggedIn(ctx, nonMatchingEvent)
	
	// Verify - no error and no calls made
	assert.NoError(t, err)
	mockUserStore.AssertNotCalled(t, "GetUser")
	mockEventBus.AssertNotCalled(t, "PublishEvent")
}