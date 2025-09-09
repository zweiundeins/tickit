package users

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestEmailVerificationCommandHandler tests the email verification command handlers
func TestEmailVerificationCommandHandler(t *testing.T) {
	ctx := context.Background()

	t.Run("HandleVerifyEmail - success", func(t *testing.T) {
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

		// Test data
		userID := uuid.New().String()
		token := uuid.New().String()
		now := time.Now()

		user := &User{
			ID:            userID,
			Email:         "test@example.com",
			Username:      "testuser",
			FirstName:     "Test",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Pending",
			EmailVerified: false,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		verificationToken := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    userID,
			Token:     token,
			ExpiresAt: now.Add(24 * time.Hour),
			CreatedAt: now,
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

	t.Run("HandleVerifyEmail - expired token", func(t *testing.T) {
		// Setup mocks
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

		// Test data
		token := uuid.New().String()
		now := time.Now()

		expiredToken := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    uuid.New().String(),
			Token:     token,
			ExpiresAt: now.Add(-time.Hour), // Expired 1 hour ago
			CreatedAt: now.Add(-2 * time.Hour),
		}

		// Setup expectations
		mockStore.On("GetEmailVerificationToken", ctx, token).Return(expiredToken, nil)
		mockStore.On("DeleteEmailVerificationToken", ctx, token).Return(nil)

		// Execute
		cmd := VerifyEmailCommand{Token: token}
		err := handler.HandleVerifyEmail(ctx, cmd)

		// Assert
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "verification token has expired")
		mockStore.AssertExpectations(t)
	})

	t.Run("HandleVerifyEmail - invalid token", func(t *testing.T) {
		// Setup mocks
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

		// Test data
		token := uuid.New().String()

		// Setup expectations - token not found
		mockStore.On("GetEmailVerificationToken", ctx, token).Return(nil, nil)

		// Execute
		cmd := VerifyEmailCommand{Token: token}
		err := handler.HandleVerifyEmail(ctx, cmd)

		// Assert
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid verification token")
		mockStore.AssertExpectations(t)
	})

	t.Run("HandleVerifyEmail - already verified", func(t *testing.T) {
		// Setup mocks
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

		// Test data
		userID := uuid.New().String()
		token := uuid.New().String()
		now := time.Now()

		user := &User{
			ID:            userID,
			Email:         "test@example.com",
			Username:      "testuser",
			FirstName:     "Test",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Active",
			EmailVerified: true, // Already verified
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		verificationToken := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    userID,
			Token:     token,
			ExpiresAt: now.Add(24 * time.Hour),
			CreatedAt: now,
		}

		// Setup expectations
		mockStore.On("GetEmailVerificationToken", ctx, token).Return(verificationToken, nil)
		mockStore.On("GetUser", ctx, userID).Return(user, nil)
		mockStore.On("DeleteEmailVerificationToken", ctx, token).Return(nil)

		// Execute
		cmd := VerifyEmailCommand{Token: token}
		err := handler.HandleVerifyEmail(ctx, cmd)

		// Assert
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "email is already verified")
		mockStore.AssertExpectations(t)
	})
}

// TestEmailVerificationTokenGenerationInRegistration tests token generation during registration
func TestEmailVerificationTokenGenerationInRegistration(t *testing.T) {
	t.Run("token generation during registration", func(t *testing.T) {
		ctx := context.Background()

		// Setup mocks
		mockStore := new(MockUserStore)
		mockHasher := new(MockPasswordHasher)
		mockEmailService := new(MockEmailService)
		mockEventBus := new(MockEventBus)

		handler := NewUserCommandHandler(
			mockEventBus,
			nil,
			mockStore,
			mockHasher,
			mockEmailService,
			nil,
		)

		// Test data
		userID := uuid.New().String()
		email := "test@example.com"
		password := "Password123"

		// Setup expectations
		mockStore.On("GetUserByEmail", ctx, email).Return(nil, nil)
		mockStore.On("GetUserByUsername", ctx, "testuser").Return(nil, nil)
		mockHasher.On("HashPassword", password).Return("hashed_password", nil)
		mockStore.On("CreateUser", ctx, mock.AnythingOfType("*users.User")).Return(nil)
		mockStore.On("CreateUserCredential", ctx, mock.AnythingOfType("*users.UserCredential")).Return(nil)
		mockStore.On("CreateEmailVerificationToken", ctx, mock.MatchedBy(func(token *EmailVerificationToken) bool {
			// Verify token properties
			return token.UserID == userID &&
				token.Token != "" &&
				token.ExpiresAt.After(time.Now()) &&
				token.ExpiresAt.Before(time.Now().Add(25*time.Hour)) // Should expire in ~24 hours
		})).Return(nil)
		mockEmailService.On("SendVerificationEmail", ctx, email, mock.AnythingOfType("string")).Return(nil)
		mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserRegisteredEvent")).Return(nil)

		// Execute
		cmd := RegisterUserCommand{
			ID:        userID,
			Email:     email,
			Password:  password,
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
}
