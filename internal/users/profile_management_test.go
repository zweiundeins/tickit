package users

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock implementations for testing
type MockEventPublisherForProfile struct {
	mock.Mock
}

func (m *MockEventPublisherForProfile) PublishEvent(ctx context.Context, event interface{}) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

type MockUserStoreForProfile struct {
	mock.Mock
}

func (m *MockUserStoreForProfile) GetUser(ctx context.Context, userID string) (*User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockUserStoreForProfile) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockUserStoreForProfile) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockUserStoreForProfile) UpdateUser(ctx context.Context, user *User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserStoreForProfile) CreateEmailVerificationToken(ctx context.Context, token *EmailVerificationToken) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockUserStoreForProfile) DeleteAllUserSessions(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

// Add other required methods as stubs
func (m *MockUserStoreForProfile) ListUsers(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*User, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) SearchUsers(ctx context.Context, query string, limit, offset int) ([]*User, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) CreateUser(ctx context.Context, user *User) error           { return nil }
func (m *MockUserStoreForProfile) PatchUser(ctx context.Context, patch *PatchUserInput) error { return nil }
func (m *MockUserStoreForProfile) DeleteUser(ctx context.Context, userID string) error        { return nil }
func (m *MockUserStoreForProfile) GetUserCredentials(ctx context.Context, userID string) ([]*UserCredential, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) GetUserCredentialByProvider(ctx context.Context, userID, provider string) (*UserCredential, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) GetUserByOAuthProvider(ctx context.Context, provider, providerID string) (*User, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) CreateUserCredential(ctx context.Context, credential *UserCredential) error {
	return nil
}
func (m *MockUserStoreForProfile) UpdateUserCredential(ctx context.Context, credential *UserCredential) error {
	return nil
}
func (m *MockUserStoreForProfile) DeleteUserCredential(ctx context.Context, credentialID string) error {
	return nil
}
func (m *MockUserStoreForProfile) GetUserSession(ctx context.Context, sessionID string) (*UserSession, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) GetUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) GetActiveUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) CreateUserSession(ctx context.Context, session *UserSession) error {
	return nil
}
func (m *MockUserStoreForProfile) UpdateUserSession(ctx context.Context, session *UserSession) error {
	return nil
}
func (m *MockUserStoreForProfile) DeleteUserSession(ctx context.Context, sessionID string) error { return nil }
func (m *MockUserStoreForProfile) DeleteExpiredSessions(ctx context.Context) (int, error)        { return 0, nil }
func (m *MockUserStoreForProfile) GetProjectMembers(ctx context.Context, projectID string) ([]*User, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) GetUserProjects(ctx context.Context, userID string) ([]*ProjectMembership, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) GetProjectMembership(ctx context.Context, userID, projectID string) (*ProjectMembership, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) CheckUserProjectAccess(ctx context.Context, userID, projectID string) (bool, error) {
	return false, nil
}
func (m *MockUserStoreForProfile) CreateProjectMembership(ctx context.Context, membership *ProjectMembership) error {
	return nil
}
func (m *MockUserStoreForProfile) UpdateProjectMembership(ctx context.Context, membership *ProjectMembership) error {
	return nil
}
func (m *MockUserStoreForProfile) DeleteProjectMembership(ctx context.Context, userID, projectID string) error {
	return nil
}
func (m *MockUserStoreForProfile) GetEmailVerificationToken(ctx context.Context, token string) (*EmailVerificationToken, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) GetEmailVerificationTokenByUserID(ctx context.Context, userID string) (*EmailVerificationToken, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) DeleteEmailVerificationToken(ctx context.Context, token string) error {
	return nil
}
func (m *MockUserStoreForProfile) GetPasswordResetToken(ctx context.Context, token string) (*PasswordResetToken, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) CreatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error {
	return nil
}
func (m *MockUserStoreForProfile) UpdatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error {
	return nil
}
func (m *MockUserStoreForProfile) DeletePasswordResetToken(ctx context.Context, token string) error { return nil }
func (m *MockUserStoreForProfile) GetMagicMailToken(ctx context.Context, token string) (*MagicMailToken, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) CreateMagicMailToken(ctx context.Context, token *MagicMailToken) error {
	return nil
}
func (m *MockUserStoreForProfile) UpdateMagicMailToken(ctx context.Context, token *MagicMailToken) error {
	return nil
}
func (m *MockUserStoreForProfile) DeleteMagicMailToken(ctx context.Context, token string) error { return nil }
func (m *MockUserStoreForProfile) GetProjectInvitations(ctx context.Context, projectID string) ([]*UserInvitation, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) GetUserInvitations(ctx context.Context, email string) ([]*UserInvitation, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) GetInvitationByToken(ctx context.Context, token string) (*UserInvitation, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) CreateInvitation(ctx context.Context, invitation *UserInvitation) error {
	return nil
}
func (m *MockUserStoreForProfile) UpdateInvitation(ctx context.Context, invitation *UserInvitation) error {
	return nil
}
func (m *MockUserStoreForProfile) DeleteInvitation(ctx context.Context, invitationID string) error { return nil }
func (m *MockUserStoreForProfile) GetUserActivity(ctx context.Context, userID string, limit, offset int) ([]interface{}, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) GetUserStats(ctx context.Context, userID string) (map[string]interface{}, error) {
	return nil, nil
}
func (m *MockUserStoreForProfile) GetSystemUserStats(ctx context.Context) (map[string]interface{}, error) {
	return nil, nil
}

type MockEmailServiceForProfile struct {
	mock.Mock
}

func (m *MockEmailServiceForProfile) SendVerificationEmail(ctx context.Context, email, token string) error {
	args := m.Called(ctx, email, token)
	return args.Error(0)
}

func (m *MockEmailServiceForProfile) SendPasswordResetEmail(ctx context.Context, email, token string) error {
	return nil
}
func (m *MockEmailServiceForProfile) SendInvitationEmail(ctx context.Context, email, projectName, token string) error {
	return nil
}
func (m *MockEmailServiceForProfile) SendMagicMailLink(ctx context.Context, email, token string) error {
	return nil
}

// Test HandleUpdateUserProfile
func TestHandleUpdateUserProfile(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		cmd         UpdateUserProfileCommand
		setupMocks  func(*MockUserStore, *MockEventPublisher, *MockEmailService)
		expectError bool
		errorMsg    string
	}{
		{
			name:   "successful profile update",
			userID: "user-123",
			cmd: UpdateUserProfileCommand{
				UserID:    "user-123",
				Username:  stringPtr("newusername"),
				FirstName: stringPtr("John"),
				LastName:  stringPtr("Doe"),
			},
			setupMocks: func(store *MockUserStore, eventBus *MockEventPublisher, emailService *MockEmailService) {
				user := &User{
					ID:        "user-123",
					Email:     "test@example.com",
					Username:  "oldusername",
					FirstName: "OldFirst",
					LastName:  "OldLast",
					Status:    "Active",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				store.On("GetUser", mock.Anything, "user-123").Return(user, nil)
				store.On("GetUserByUsername", mock.Anything, "newusername").Return(nil, nil)
				store.On("UpdateUser", mock.Anything, mock.AnythingOfType("*users.User")).Return(nil)
				eventBus.On("PublishEvent", mock.Anything, mock.AnythingOfType("users.UserProfileUpdatedEvent")).Return(nil)
			},
			expectError: false,
		},
		{
			name:   "user not found",
			userID: "nonexistent",
			cmd: UpdateUserProfileCommand{
				UserID:   "nonexistent",
				Username: stringPtr("newusername"),
			},
			setupMocks: func(store *MockUserStore, eventBus *MockEventPublisher, emailService *MockEmailService) {
				store.On("GetUser", mock.Anything, "nonexistent").Return(nil, nil)
			},
			expectError: true,
			errorMsg:    "user not found",
		},
		{
			name:   "email already in use",
			userID: "user-123",
			cmd: UpdateUserProfileCommand{
				UserID: "user-123",
				Email:  stringPtr("existing@example.com"),
			},
			setupMocks: func(store *MockUserStore, eventBus *MockEventPublisher, emailService *MockEmailService) {
				user := &User{
					ID:        "user-123",
					Email:     "test@example.com",
					Username:  "testuser",
					Status:    "Active",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				existingUser := &User{
					ID:    "other-user",
					Email: "existing@example.com",
				}
				store.On("GetUser", mock.Anything, "user-123").Return(user, nil)
				store.On("GetUserByEmail", mock.Anything, "existing@example.com").Return(existingUser, nil)
			},
			expectError: true,
			errorMsg:    "email existing@example.com is already in use",
		},
		{
			name:   "username already taken",
			userID: "user-123",
			cmd: UpdateUserProfileCommand{
				UserID:   "user-123",
				Username: stringPtr("existinguser"),
			},
			setupMocks: func(store *MockUserStore, eventBus *MockEventPublisher, emailService *MockEmailService) {
				user := &User{
					ID:        "user-123",
					Email:     "test@example.com",
					Username:  "testuser",
					Status:    "Active",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				existingUser := &User{
					ID:       "other-user",
					Username: "existinguser",
				}
				store.On("GetUser", mock.Anything, "user-123").Return(user, nil)
				store.On("GetUserByUsername", mock.Anything, "existinguser").Return(existingUser, nil)
			},
			expectError: true,
			errorMsg:    "username existinguser is already taken",
		},
		{
			name:   "email change triggers verification",
			userID: "user-123",
			cmd: UpdateUserProfileCommand{
				UserID: "user-123",
				Email:  stringPtr("newemail@example.com"),
			},
			setupMocks: func(store *MockUserStore, eventBus *MockEventPublisher, emailService *MockEmailService) {
				user := &User{
					ID:            "user-123",
					Email:         "old@example.com",
					Username:      "testuser",
					Status:        "Active",
					EmailVerified: true,
					CreatedAt:     time.Now(),
					UpdatedAt:     time.Now(),
				}
				store.On("GetUser", mock.Anything, "user-123").Return(user, nil)
				store.On("GetUserByEmail", mock.Anything, "newemail@example.com").Return(nil, nil)
				store.On("UpdateUser", mock.Anything, mock.AnythingOfType("*users.User")).Return(nil)
				store.On("CreateEmailVerificationToken", mock.Anything, mock.AnythingOfType("*users.EmailVerificationToken")).Return(nil)
				emailService.On("SendVerificationEmail", mock.Anything, "newemail@example.com", mock.AnythingOfType("string")).Return(nil)
				eventBus.On("PublishEvent", mock.Anything, mock.AnythingOfType("users.UserProfileUpdatedEvent")).Return(nil)
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockStore := &MockUserStoreForProfile{}
			mockEventBus := &MockEventPublisherForProfile{}
			mockEmailService := &MockEmailServiceForProfile{}

			tt.setupMocks(mockStore, mockEventBus, mockEmailService)

			// Create handler
			handler := NewUserCommandHandler(
				mockEventBus,
				nil, // eventStore not needed for this test
				mockStore,
				nil, // passwordHasher not needed for this test
				mockEmailService,
				nil, // webauthnService not needed for this test
			)

			// Execute
			err := handler.HandleUpdateUserProfile(context.Background(), tt.cmd)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			// Verify mocks
			mockStore.AssertExpectations(t)
			mockEventBus.AssertExpectations(t)
			mockEmailService.AssertExpectations(t)
		})
	}
}

// Test HandleDeactivateUser
func TestHandleDeactivateUser(t *testing.T) {
	tests := []struct {
		name        string
		cmd         DeactivateUserCommand
		setupMocks  func(*MockUserStore, *MockEventPublisher)
		expectError bool
		errorMsg    string
	}{
		{
			name: "successful deactivation",
			cmd: DeactivateUserCommand{
				UserID: "user-123",
			},
			setupMocks: func(store *MockUserStore, eventBus *MockEventPublisher) {
				user := &User{
					ID:        "user-123",
					Email:     "test@example.com",
					Username:  "testuser",
					Status:    "Active",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				store.On("GetUser", mock.Anything, "user-123").Return(user, nil)
				store.On("UpdateUser", mock.Anything, mock.AnythingOfType("*users.User")).Return(nil)
				store.On("DeleteAllUserSessions", mock.Anything, "user-123").Return(nil)
				eventBus.On("PublishEvent", mock.Anything, mock.AnythingOfType("users.UserDeactivatedEvent")).Return(nil)
				eventBus.On("PublishEvent", mock.Anything, mock.AnythingOfType("users.AllUserSessionsInvalidatedEvent")).Return(nil)
			},
			expectError: false,
		},
		{
			name: "user not found",
			cmd: DeactivateUserCommand{
				UserID: "nonexistent",
			},
			setupMocks: func(store *MockUserStore, eventBus *MockEventPublisher) {
				store.On("GetUser", mock.Anything, "nonexistent").Return(nil, nil)
			},
			expectError: true,
			errorMsg:    "user not found",
		},
		{
			name: "user already inactive",
			cmd: DeactivateUserCommand{
				UserID: "user-123",
			},
			setupMocks: func(store *MockUserStore, eventBus *MockEventPublisher) {
				user := &User{
					ID:        "user-123",
					Email:     "test@example.com",
					Username:  "testuser",
					Status:    "Inactive",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				store.On("GetUser", mock.Anything, "user-123").Return(user, nil)
			},
			expectError: true,
			errorMsg:    "user is already deactivated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockStore := &MockUserStoreForProfile{}
			mockEventBus := &MockEventPublisherForProfile{}

			tt.setupMocks(mockStore, mockEventBus)

			// Create handler
			handler := NewUserCommandHandler(
				mockEventBus,
				nil, // eventStore not needed for this test
				mockStore,
				nil, // passwordHasher not needed for this test
				nil, // emailService not needed for this test
				nil, // webauthnService not needed for this test
			)

			// Execute
			err := handler.HandleDeactivateUser(context.Background(), tt.cmd)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			// Verify mocks
			mockStore.AssertExpectations(t)
			mockEventBus.AssertExpectations(t)
		})
	}
}

// Test HandleReactivateUser
func TestHandleReactivateUser(t *testing.T) {
	tests := []struct {
		name        string
		cmd         ReactivateUserCommand
		setupMocks  func(*MockUserStore, *MockEventPublisher)
		expectError bool
		errorMsg    string
	}{
		{
			name: "successful reactivation",
			cmd: ReactivateUserCommand{
				UserID: "user-123",
			},
			setupMocks: func(store *MockUserStore, eventBus *MockEventPublisher) {
				user := &User{
					ID:        "user-123",
					Email:     "test@example.com",
					Username:  "testuser",
					Status:    "Inactive",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				store.On("GetUser", mock.Anything, "user-123").Return(user, nil)
				store.On("UpdateUser", mock.Anything, mock.AnythingOfType("*users.User")).Return(nil)
				eventBus.On("PublishEvent", mock.Anything, mock.AnythingOfType("users.UserReactivatedEvent")).Return(nil)
			},
			expectError: false,
		},
		{
			name: "user already active",
			cmd: ReactivateUserCommand{
				UserID: "user-123",
			},
			setupMocks: func(store *MockUserStore, eventBus *MockEventPublisher) {
				user := &User{
					ID:        "user-123",
					Email:     "test@example.com",
					Username:  "testuser",
					Status:    "Active",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				store.On("GetUser", mock.Anything, "user-123").Return(user, nil)
			},
			expectError: true,
			errorMsg:    "user is already active",
		},
		{
			name: "cannot reactivate pending user",
			cmd: ReactivateUserCommand{
				UserID: "user-123",
			},
			setupMocks: func(store *MockUserStore, eventBus *MockEventPublisher) {
				user := &User{
					ID:        "user-123",
					Email:     "test@example.com",
					Username:  "testuser",
					Status:    "Pending",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				store.On("GetUser", mock.Anything, "user-123").Return(user, nil)
			},
			expectError: true,
			errorMsg:    "user cannot be reactivated from status: Pending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockStore := &MockUserStoreForProfile{}
			mockEventBus := &MockEventPublisherForProfile{}

			tt.setupMocks(mockStore, mockEventBus)

			// Create handler
			handler := NewUserCommandHandler(
				mockEventBus,
				nil, // eventStore not needed for this test
				mockStore,
				nil, // passwordHasher not needed for this test
				nil, // emailService not needed for this test
				nil, // webauthnService not needed for this test
			)

			// Execute
			err := handler.HandleReactivateUser(context.Background(), tt.cmd)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			// Verify mocks
			mockStore.AssertExpectations(t)
			mockEventBus.AssertExpectations(t)
		})
	}
}


