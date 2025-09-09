package users

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mbolli/tickit/internal/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock implementations for testing

type MockUserStoreWithMock struct {
	mock.Mock
}

func (m *MockUserStoreWithMock) GetUser(ctx context.Context, userID string) (*User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockUserStoreWithMock) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockUserStoreWithMock) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockUserStoreWithMock) CreateUser(ctx context.Context, user *User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserStoreWithMock) UpdateUser(ctx context.Context, user *User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserStoreWithMock) GetUserCredentials(ctx context.Context, userID string) ([]*UserCredential, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*UserCredential), args.Error(1)
}

func (m *MockUserStoreWithMock) CreateUserCredential(ctx context.Context, credential *UserCredential) error {
	args := m.Called(ctx, credential)
	return args.Error(0)
}

func (m *MockUserStoreWithMock) CreateEmailVerificationToken(ctx context.Context, token *EmailVerificationToken) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockUserStoreWithMock) GetEmailVerificationToken(ctx context.Context, token string) (*EmailVerificationToken, error) {
	args := m.Called(ctx, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*EmailVerificationToken), args.Error(1)
}

func (m *MockUserStoreWithMock) DeleteEmailVerificationToken(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockUserStoreWithMock) CreateUserSession(ctx context.Context, session *UserSession) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

// Implement remaining UserStore methods as no-ops for testing
func (m *MockUserStoreWithMock) ListUsers(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*User, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) SearchUsers(ctx context.Context, query string, limit, offset int) ([]*User, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) PatchUser(ctx context.Context, patch *PatchUserInput) error { return nil }
func (m *MockUserStoreWithMock) DeleteUser(ctx context.Context, userID string) error        { return nil }
func (m *MockUserStoreWithMock) GetUserCredentialByProvider(ctx context.Context, userID, provider string) (*UserCredential, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) GetUserByOAuthProvider(ctx context.Context, provider, providerID string) (*User, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) UpdateUserCredential(ctx context.Context, credential *UserCredential) error {
	return nil
}
func (m *MockUserStoreWithMock) DeleteUserCredential(ctx context.Context, credentialID string) error {
	return nil
}
func (m *MockUserStoreWithMock) GetUserSession(ctx context.Context, sessionID string) (*UserSession, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) GetUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) GetActiveUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) UpdateUserSession(ctx context.Context, session *UserSession) error {
	return nil
}
func (m *MockUserStoreWithMock) DeleteUserSession(ctx context.Context, sessionID string) error  { return nil }
func (m *MockUserStoreWithMock) DeleteAllUserSessions(ctx context.Context, userID string) error { return nil }
func (m *MockUserStoreWithMock) DeleteExpiredSessions(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}
func (m *MockUserStoreWithMock) GetProjectMembers(ctx context.Context, projectID string) ([]*User, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) GetUserProjects(ctx context.Context, userID string) ([]*ProjectMembership, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) GetProjectMembership(ctx context.Context, userID, projectID string) (*ProjectMembership, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) CheckUserProjectAccess(ctx context.Context, userID, projectID string) (bool, error) {
	return false, nil
}
func (m *MockUserStoreWithMock) CreateProjectMembership(ctx context.Context, membership *ProjectMembership) error {
	return nil
}
func (m *MockUserStoreWithMock) UpdateProjectMembership(ctx context.Context, membership *ProjectMembership) error {
	return nil
}
func (m *MockUserStoreWithMock) DeleteProjectMembership(ctx context.Context, userID, projectID string) error {
	return nil
}

func (m *MockUserStoreWithMock) GetPasswordResetToken(ctx context.Context, token string) (*PasswordResetToken, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) CreatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error {
	return nil
}
func (m *MockUserStoreWithMock) UpdatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error {
	return nil
}
func (m *MockUserStoreWithMock) DeletePasswordResetToken(ctx context.Context, token string) error { return nil }
func (m *MockUserStoreWithMock) GetMagicMailToken(ctx context.Context, token string) (*MagicMailToken, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) CreateMagicMailToken(ctx context.Context, token *MagicMailToken) error {
	return nil
}
func (m *MockUserStoreWithMock) UpdateMagicMailToken(ctx context.Context, token *MagicMailToken) error {
	return nil
}
func (m *MockUserStoreWithMock) DeleteMagicMailToken(ctx context.Context, token string) error { return nil }
func (m *MockUserStoreWithMock) GetProjectInvitations(ctx context.Context, projectID string) ([]*UserInvitation, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) GetUserInvitations(ctx context.Context, email string) ([]*UserInvitation, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) GetInvitationByToken(ctx context.Context, token string) (*UserInvitation, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) CreateInvitation(ctx context.Context, invitation *UserInvitation) error {
	return nil
}
func (m *MockUserStoreWithMock) UpdateInvitation(ctx context.Context, invitation *UserInvitation) error {
	return nil
}
func (m *MockUserStoreWithMock) DeleteInvitation(ctx context.Context, invitationID string) error { return nil }
func (m *MockUserStoreWithMock) GetUserActivity(ctx context.Context, userID string, limit, offset int) ([]interface{}, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) GetUserStats(ctx context.Context, userID string) (map[string]interface{}, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) GetSystemUserStats(ctx context.Context) (map[string]interface{}, error) {
	return nil, nil
}
func (m *MockUserStoreWithMock) GetEmailVerificationTokenByUserID(ctx context.Context, userID string) (*EmailVerificationToken, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*EmailVerificationToken), args.Error(1)
}

type MockPasswordHasher struct {
	mock.Mock
}

func (m *MockPasswordHasher) HashPassword(password string) (string, error) {
	args := m.Called(password)
	return args.String(0), args.Error(1)
}

func (m *MockPasswordHasher) CheckPassword(hashedPassword, password string) error {
	args := m.Called(hashedPassword, password)
	return args.Error(0)
}

type MockEmailServiceWithMock struct {
	mock.Mock
}

func (m *MockEmailServiceWithMock) SendVerificationEmail(ctx context.Context, email, token string) error {
	args := m.Called(ctx, email, token)
	return args.Error(0)
}

func (m *MockEmailServiceWithMock) SendPasswordResetEmail(ctx context.Context, email, token string) error {
	args := m.Called(ctx, email, token)
	return args.Error(0)
}

func (m *MockEmailServiceWithMock) SendInvitationEmail(ctx context.Context, email, projectName, token string) error {
	args := m.Called(ctx, email, projectName, token)
	return args.Error(0)
}

func (m *MockEmailServiceWithMock) SendMagicMailLink(ctx context.Context, email, token string) error {
	args := m.Called(ctx, email, token)
	return args.Error(0)
}

func TestUserCommandHandler_HandleRegisterUser(t *testing.T) {
	ctx := context.Background()

	t.Run("successful registration", func(t *testing.T) {
		// Setup mocks
		mockStore := new(MockUserStore)
		mockHasher := new(MockPasswordHasher)
		mockEmailService := new(MockEmailService)
		mockEventBus := new(MockEventBus)

		handler := NewUserCommandHandler(
			mockEventBus,
			nil, // eventStore not needed for this test
			mockStore,
			mockHasher,
			mockEmailService,
			nil, // webauthnService not needed for this test
			nil,
		)

		// Setup expectations
		mockStore.On("GetUserByEmail", ctx, "test@example.com").Return(nil, nil)
		mockStore.On("GetUserByUsername", ctx, "testuser").Return(nil, nil)
		mockHasher.On("HashPassword", "Password123").Return("hashed_password", nil)
		mockStore.On("CreateUser", ctx, mock.AnythingOfType("*users.User")).Return(nil)
		mockStore.On("CreateUserCredential", ctx, mock.AnythingOfType("*users.UserCredential")).Return(nil)
		mockStore.On("CreateEmailVerificationToken", ctx, mock.AnythingOfType("*users.EmailVerificationToken")).Return(nil)
		mockEmailService.On("SendVerificationEmail", ctx, "test@example.com", mock.AnythingOfType("string")).Return(nil)
		mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserRegisteredEvent")).Return(nil)

		// Execute
		cmd := RegisterUserCommand{
			ID:        uuid.New().String(),
			Email:     "test@example.com",
			Password:  "Password123",
			FirstName: "Test",
			LastName:  "User",
			Username:  "testuser",
		}

		err := handler.HandleRegisterUser(ctx, cmd)

		// Assert
		assert.NoError(t, err)
		mockStore.AssertExpectations(t)
		mockHasher.AssertExpectations(t)
		mockEmailService.AssertExpectations(t)
		mockEventBus.AssertExpectations(t)
	})

	t.Run("invalid email", func(t *testing.T) {
		handler := NewUserCommandHandler(nil, nil, nil, nil, nil, nil)

		cmd := RegisterUserCommand{
			ID:       uuid.New().String(),
			Email:    "invalid-email",
			Password: "Password123",
			Username: "testuser",
		}

		err := handler.HandleRegisterUser(ctx, cmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid email")
	})

	t.Run("weak password", func(t *testing.T) {
		handler := NewUserCommandHandler(nil, nil, nil, nil, nil, nil)

		cmd := RegisterUserCommand{
			ID:       uuid.New().String(),
			Email:    "test@example.com",
			Password: "weak",
			Username: "testuser",
		}

		err := handler.HandleRegisterUser(ctx, cmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid password")
	})

	t.Run("user already exists", func(t *testing.T) {
		mockStore := new(MockUserStore)
		handler := NewUserCommandHandler(nil, nil, mockStore, nil, nil, nil)

		existingUser := &User{
			ID:    uuid.New().String(),
			Email: "test@example.com",
		}
		mockStore.On("GetUserByEmail", ctx, "test@example.com").Return(existingUser, nil)

		cmd := RegisterUserCommand{
			ID:       uuid.New().String(),
			Email:    "test@example.com",
			Password: "Password123",
			Username: "testuser",
		}

		err := handler.HandleRegisterUser(ctx, cmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
		mockStore.AssertExpectations(t)
	})
}

func TestUserCommandHandler_HandleLoginUser(t *testing.T) {
	ctx := context.Background()

	t.Run("successful login", func(t *testing.T) {
		// Setup mocks
		mockStore := new(MockUserStore)
		mockHasher := new(MockPasswordHasher)
		mockEventBus := new(MockEventBus)

		handler := NewUserCommandHandler(
			mockEventBus,
			nil,
			mockStore,
			mockHasher,
			nil,
			nil,
		)

		// Create test user
		user := &User{
			ID:            uuid.New().String(),
			Email:         "test@example.com",
			Status:        "Active",
			EmailVerified: true,
		}

		// Create test credential
		credential := &UserCredential{
			ID:             uuid.New().String(),
			UserID:         user.ID,
			Type:           "password",
			Provider:       "local",
			CredentialData: "hashed_password",
		}

		// Setup expectations
		mockStore.On("GetUserByEmail", ctx, "test@example.com").Return(user, nil)
		mockStore.On("GetUserCredentials", ctx, user.ID).Return([]*UserCredential{credential}, nil)
		mockHasher.On("CheckPassword", "hashed_password", "Password123").Return(nil)
		mockStore.On("CreateUserSession", ctx, mock.AnythingOfType("*users.UserSession")).Return(nil)
		mockStore.On("UpdateUser", ctx, mock.AnythingOfType("*users.User")).Return(nil)
		mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserLoggedInEvent")).Return(nil)

		// Execute
		cmd := LoginUserCommand{
			Email:     "test@example.com",
			Password:  "Password123",
			IPAddress: "127.0.0.1",
			UserAgent: "test-agent",
		}

		err := handler.HandleLoginUser(ctx, cmd)

		// Assert
		assert.NoError(t, err)
		mockStore.AssertExpectations(t)
		mockHasher.AssertExpectations(t)
		mockEventBus.AssertExpectations(t)
	})

	t.Run("user not found", func(t *testing.T) {
		mockStore := new(MockUserStore)
		mockEventBus := new(MockEventBus)

		handler := NewUserCommandHandler(
			mockEventBus,
			nil,
			mockStore,
			nil,
			nil,
			nil,
		)

		mockStore.On("GetUserByEmail", ctx, "nonexistent@example.com").Return(nil, nil)
		mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserLoginFailedEvent")).Return(nil)

		cmd := LoginUserCommand{
			Email:     "nonexistent@example.com",
			Password:  "Password123",
			IPAddress: "127.0.0.1",
			UserAgent: "test-agent",
		}

		err := handler.HandleLoginUser(ctx, cmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid credentials")
		mockStore.AssertExpectations(t)
		mockEventBus.AssertExpectations(t)
	})

	t.Run("email not verified", func(t *testing.T) {
		mockStore := new(MockUserStore)
		mockEventBus := new(MockEventBus)

		handler := NewUserCommandHandler(
			mockEventBus,
			nil,
			mockStore,
			nil,
			nil,
			nil,
		)

		user := &User{
			ID:            uuid.New().String(),
			Email:         "test@example.com",
			Status:        "Pending",
			EmailVerified: false,
		}

		mockStore.On("GetUserByEmail", ctx, "test@example.com").Return(user, nil)
		mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserLoginFailedEvent")).Return(nil)

		cmd := LoginUserCommand{
			Email:     "test@example.com",
			Password:  "Password123",
			IPAddress: "127.0.0.1",
			UserAgent: "test-agent",
		}

		err := handler.HandleLoginUser(ctx, cmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "email not verified")
		mockStore.AssertExpectations(t)
		mockEventBus.AssertExpectations(t)
	})

	t.Run("wrong password", func(t *testing.T) {
		mockStore := new(MockUserStore)
		mockHasher := new(MockPasswordHasher)
		mockEventBus := new(MockEventBus)

		handler := NewUserCommandHandler(
			mockEventBus,
			nil,
			mockStore,
			mockHasher,
			nil,
			nil,
		)

		user := &User{
			ID:            uuid.New().String(),
			Email:         "test@example.com",
			Status:        "Active",
			EmailVerified: true,
		}

		credential := &UserCredential{
			ID:             uuid.New().String(),
			UserID:         user.ID,
			Type:           "password",
			Provider:       "local",
			CredentialData: "hashed_password",
		}

		mockStore.On("GetUserByEmail", ctx, "test@example.com").Return(user, nil)
		mockStore.On("GetUserCredentials", ctx, user.ID).Return([]*UserCredential{credential}, nil)
		mockHasher.On("CheckPassword", "hashed_password", "WrongPassword").Return(assert.AnError)
		mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserLoginFailedEvent")).Return(nil)

		cmd := LoginUserCommand{
			Email:     "test@example.com",
			Password:  "WrongPassword",
			IPAddress: "127.0.0.1",
			UserAgent: "test-agent",
		}

		err := handler.HandleLoginUser(ctx, cmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid credentials")
		mockStore.AssertExpectations(t)
		mockHasher.AssertExpectations(t)
		mockEventBus.AssertExpectations(t)
	})
}

func TestUserCommandHandler_HandleVerifyEmail(t *testing.T) {
	ctx := context.Background()

	t.Run("successful email verification", func(t *testing.T) {
		// Setup mocks
		mockStore := new(MockUserStore)
		mockEventBus := new(MockEventBus)

		handler := NewUserCommandHandler(
			mockEventBus,
			nil, // eventStore not needed for this test
			mockStore,
			nil, // passwordHasher not needed for this test
			nil, // emailService not needed for this test
			nil, // webauthnService not needed for this test
		)

		// Create test data
		userID := uuid.New().String()
		token := "valid-verification-token"

		verificationToken := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    userID,
			Token:     token,
			ExpiresAt: time.Now().Add(time.Hour), // Valid token
			CreatedAt: time.Now(),
		}

		user := &User{
			ID:            userID,
			Email:         "test@example.com",
			Username:      "testuser",
			EmailVerified: false,
			Status:        "Pending",
		}

		// Setup expectations
		mockStore.On("GetEmailVerificationToken", ctx, token).Return(verificationToken, nil)
		mockStore.On("GetUser", ctx, userID).Return(user, nil)
		mockStore.On("UpdateUser", ctx, mock.MatchedBy(func(u *User) bool {
			return u.ID == userID && u.EmailVerified == true && u.Status == "Active"
		})).Return(nil)
		mockStore.On("DeleteEmailVerificationToken", ctx, token).Return(nil)
		mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserEmailVerifiedEvent")).Return(nil)

		// Execute
		cmd := VerifyEmailCommand{Token: token}
		err := handler.HandleVerifyEmail(ctx, cmd)

		// Assert
		assert.NoError(t, err)
		mockStore.AssertExpectations(t)
		mockEventBus.AssertExpectations(t)
	})

	t.Run("empty token", func(t *testing.T) {
		handler := NewUserCommandHandler(nil, nil, nil, nil, nil, nil)

		cmd := VerifyEmailCommand{Token: ""}
		err := handler.HandleVerifyEmail(ctx, cmd)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "verification token is required")
	})

	t.Run("invalid token", func(t *testing.T) {
		mockStore := new(MockUserStore)
		handler := NewUserCommandHandler(nil, nil, mockStore, nil, nil, nil)

		token := "invalid-token"
		mockStore.On("GetEmailVerificationToken", ctx, token).Return(nil, nil)

		cmd := VerifyEmailCommand{Token: token}
		err := handler.HandleVerifyEmail(ctx, cmd)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid verification token")
		mockStore.AssertExpectations(t)
	})

	t.Run("expired token", func(t *testing.T) {
		mockStore := new(MockUserStore)
		handler := NewUserCommandHandler(nil, nil, mockStore, nil, nil, nil)

		token := "expired-token"
		expiredToken := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    uuid.New().String(),
			Token:     token,
			ExpiresAt: time.Now().Add(-time.Hour), // Expired token
			CreatedAt: time.Now().Add(-2 * time.Hour),
		}

		mockStore.On("GetEmailVerificationToken", ctx, token).Return(expiredToken, nil)
		mockStore.On("DeleteEmailVerificationToken", ctx, token).Return(nil)

		cmd := VerifyEmailCommand{Token: token}
		err := handler.HandleVerifyEmail(ctx, cmd)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "verification token has expired")
		mockStore.AssertExpectations(t)
	})

	t.Run("user not found", func(t *testing.T) {
		mockStore := new(MockUserStore)
		handler := NewUserCommandHandler(nil, nil, mockStore, nil, nil, nil)

		userID := uuid.New().String()
		token := "valid-token"

		verificationToken := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    userID,
			Token:     token,
			ExpiresAt: time.Now().Add(time.Hour),
			CreatedAt: time.Now(),
		}

		mockStore.On("GetEmailVerificationToken", ctx, token).Return(verificationToken, nil)
		mockStore.On("GetUser", ctx, userID).Return(nil, nil)

		cmd := VerifyEmailCommand{Token: token}
		err := handler.HandleVerifyEmail(ctx, cmd)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user not found")
		mockStore.AssertExpectations(t)
	})

	t.Run("already verified user", func(t *testing.T) {
		mockStore := new(MockUserStore)
		handler := NewUserCommandHandler(nil, nil, mockStore, nil, nil, nil)

		userID := uuid.New().String()
		token := "valid-token"

		verificationToken := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    userID,
			Token:     token,
			ExpiresAt: time.Now().Add(time.Hour),
			CreatedAt: time.Now(),
		}

		user := &User{
			ID:            userID,
			Email:         "test@example.com",
			EmailVerified: true, // Already verified
			Status:        "Active",
		}

		mockStore.On("GetEmailVerificationToken", ctx, token).Return(verificationToken, nil)
		mockStore.On("GetUser", ctx, userID).Return(user, nil)
		mockStore.On("DeleteEmailVerificationToken", ctx, token).Return(nil)

		cmd := VerifyEmailCommand{Token: token}
		err := handler.HandleVerifyEmail(ctx, cmd)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "email is already verified")
		mockStore.AssertExpectations(t)
	})

	t.Run("database error on token lookup", func(t *testing.T) {
		mockStore := new(MockUserStore)
		handler := NewUserCommandHandler(nil, nil, mockStore, nil, nil, nil)

		token := "valid-token"
		mockStore.On("GetEmailVerificationToken", ctx, token).Return(nil, assert.AnError)

		cmd := VerifyEmailCommand{Token: token}
		err := handler.HandleVerifyEmail(ctx, cmd)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get verification token")
		mockStore.AssertExpectations(t)
	})

	t.Run("database error on user update", func(t *testing.T) {
		mockStore := new(MockUserStore)
		handler := NewUserCommandHandler(nil, nil, mockStore, nil, nil, nil)

		userID := uuid.New().String()
		token := "valid-token"

		verificationToken := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    userID,
			Token:     token,
			ExpiresAt: time.Now().Add(time.Hour),
			CreatedAt: time.Now(),
		}

		user := &User{
			ID:            userID,
			Email:         "test@example.com",
			EmailVerified: false,
			Status:        "Pending",
		}

		mockStore.On("GetEmailVerificationToken", ctx, token).Return(verificationToken, nil)
		mockStore.On("GetUser", ctx, userID).Return(user, nil)
		mockStore.On("UpdateUser", ctx, mock.AnythingOfType("*users.User")).Return(assert.AnError)

		cmd := VerifyEmailCommand{Token: token}
		err := handler.HandleVerifyEmail(ctx, cmd)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update user verification status")
		mockStore.AssertExpectations(t)
	})
}

func TestUserCommandHandler_HandleInviteUserToProject(t *testing.T) {
	ctx := context.Background()

	t.Run("successful invitation", func(t *testing.T) {
		// Setup mocks
		mockStore := new(MockUserStore)
		mockEventBus := new(MockEventBus)
		mockEventStore := &shared.EventStore{}
		mockEmailService := new(MockEmailService)

		handler := NewUserCommandHandler(mockEventBus, mockEventStore, mockStore, nil, mockEmailService, nil)

		// Mock data
		inviterID := uuid.New().String()
		projectID := uuid.New().String()
		email := "invitee@example.com"
		role := ProjectRoleDeveloper

		inviter := &User{
			ID:    inviterID,
			Email: "inviter@example.com",
			Role:  RoleAdmin, // Admin can invite to any project
		}

		// Setup expectations
		mockStore.On("GetUser", ctx, inviterID).Return(inviter, nil)
		mockStore.On("GetUserByEmail", ctx, email).Return(nil, nil)                     // User doesn't exist yet
		mockStore.On("GetUserInvitations", ctx, email).Return([]*UserInvitation{}, nil) // No existing invitations
		mockStore.On("CreateInvitation", ctx, mock.AnythingOfType("*users.UserInvitation")).Return(nil)
		mockEmailService.On("SendInvitationEmail", ctx, email, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
		mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("shared.Event")).Return(nil)

		// Execute command
		cmd := InviteUserToProjectCommand{
			ProjectID: projectID,
			Email:     email,
			Role:      role,
			InvitedBy: inviterID,
		}

		err := handler.HandleInviteUserToProject(ctx, cmd)

		// Assertions
		assert.NoError(t, err)
		mockStore.AssertExpectations(t)
		mockEmailService.AssertExpectations(t)
		mockEventBus.AssertExpectations(t)
	})

	t.Run("invalid email", func(t *testing.T) {
		handler := NewUserCommandHandler(nil, nil, nil, nil, nil, nil)

		cmd := InviteUserToProjectCommand{
			ProjectID: "project-123",
			Email:     "invalid-email",
			Role:      ProjectRoleDeveloper,
			InvitedBy: "user-123",
		}

		err := handler.HandleInviteUserToProject(ctx, cmd)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid email")
	})

	t.Run("invalid role", func(t *testing.T) {
		handler := NewUserCommandHandler(nil, nil, nil, nil, nil, nil)

		cmd := InviteUserToProjectCommand{
			ProjectID: "project-123",
			Email:     "test@example.com",
			Role:      "InvalidRole",
			InvitedBy: "user-123",
		}

		err := handler.HandleInviteUserToProject(ctx, cmd)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid project role")
	})
}

func TestUserCommandHandler_HandleAcceptProjectInvitation(t *testing.T) {
	ctx := context.Background()

	t.Run("successful acceptance", func(t *testing.T) {
		// Setup mocks
		mockStore := new(MockUserStore)
		mockEventBus := new(MockEventBus)
		mockEventStore := &shared.EventStore{}

		handler := NewUserCommandHandler(mockEventBus, mockEventStore, mockStore, nil, nil, nil)

		// Mock data
		token := "valid-invitation-token"
		userID := uuid.New().String()
		projectID := uuid.New().String()
		email := "user@example.com"

		invitation := &UserInvitation{
			ID:         uuid.New().String(),
			ProjectID:  projectID,
			Email:      email,
			Role:       ProjectRoleDeveloper,
			Token:      token,
			ExpiresAt:  time.Now().Add(24 * time.Hour),
			CreatedAt:  time.Now(),
			CreatedBy:  "inviter-123",
			AcceptedAt: nil,
			DeclinedAt: nil,
		}

		user := &User{
			ID:    userID,
			Email: email,
		}

		// Setup expectations
		mockStore.On("GetInvitationByToken", ctx, token).Return(invitation, nil)
		mockStore.On("GetUserByEmail", ctx, email).Return(user, nil)
		mockStore.On("GetProjectMembership", ctx, userID, projectID).Return(nil, nil) // Not already a member
		mockStore.On("CreateProjectMembership", ctx, mock.AnythingOfType("*users.ProjectMembership")).Return(nil)
		mockStore.On("UpdateInvitation", ctx, mock.AnythingOfType("*users.UserInvitation")).Return(nil)
		mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("shared.Event")).Return(nil)

		// Execute command
		cmd := AcceptProjectInvitationCommand{Token: token}
		err := handler.HandleAcceptProjectInvitation(ctx, cmd)

		// Assertions
		assert.NoError(t, err)
		mockStore.AssertExpectations(t)
		mockEventBus.AssertExpectations(t)
	})

	t.Run("invalid token", func(t *testing.T) {
		mockStore := new(MockUserStore)
		handler := NewUserCommandHandler(nil, nil, mockStore, nil, nil, nil)

		mockStore.On("GetInvitationByToken", ctx, "invalid-token").Return(nil, nil)

		cmd := AcceptProjectInvitationCommand{Token: "invalid-token"}
		err := handler.HandleAcceptProjectInvitation(ctx, cmd)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid invitation token")
		mockStore.AssertExpectations(t)
	})

	t.Run("expired invitation", func(t *testing.T) {
		mockStore := new(MockUserStore)
		handler := NewUserCommandHandler(nil, nil, mockStore, nil, nil, nil)

		token := "expired-token"
		invitation := &UserInvitation{
			ID:        uuid.New().String(),
			Token:     token,
			ExpiresAt: time.Now().Add(-time.Hour), // Expired
		}

		mockStore.On("GetInvitationByToken", ctx, token).Return(invitation, nil)

		cmd := AcceptProjectInvitationCommand{Token: token}
		err := handler.HandleAcceptProjectInvitation(ctx, cmd)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invitation has expired")
		mockStore.AssertExpectations(t)
	})
}

func TestUserCommandHandler_HandleDeclineProjectInvitation(t *testing.T) {
	ctx := context.Background()

	t.Run("successful decline", func(t *testing.T) {
		// Setup mocks
		mockStore := new(MockUserStore)
		mockEventBus := new(MockEventBus)
		mockEventStore := &shared.EventStore{}

		handler := NewUserCommandHandler(mockEventBus, mockEventStore, mockStore, nil, nil, nil)

		// Mock data
		token := "valid-invitation-token"
		invitation := &UserInvitation{
			ID:         uuid.New().String(),
			ProjectID:  "project-123",
			Email:      "user@example.com",
			Token:      token,
			ExpiresAt:  time.Now().Add(24 * time.Hour),
			AcceptedAt: nil,
			DeclinedAt: nil,
		}

		// Setup expectations
		mockStore.On("GetInvitationByToken", ctx, token).Return(invitation, nil)
		mockStore.On("UpdateInvitation", ctx, mock.AnythingOfType("*users.UserInvitation")).Return(nil)
		mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("shared.Event")).Return(nil)

		// Execute command
		cmd := DeclineProjectInvitationCommand{Token: token}
		err := handler.HandleDeclineProjectInvitation(ctx, cmd)

		// Assertions
		assert.NoError(t, err)
		mockStore.AssertExpectations(t)
		mockEventBus.AssertExpectations(t)
	})

	t.Run("already declined", func(t *testing.T) {
		mockStore := new(MockUserStore)
		handler := NewUserCommandHandler(nil, nil, mockStore, nil, nil, nil)

		token := "declined-token"
		declinedTime := time.Now().Add(-time.Hour)
		invitation := &UserInvitation{
			ID:         uuid.New().String(),
			Token:      token,
			DeclinedAt: &declinedTime,
		}

		mockStore.On("GetInvitationByToken", ctx, token).Return(invitation, nil)

		cmd := DeclineProjectInvitationCommand{Token: token}
		err := handler.HandleDeclineProjectInvitation(ctx, cmd)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invitation has already been declined")
		mockStore.AssertExpectations(t)
	})
}
