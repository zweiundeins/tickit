package users

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestEmailVerificationIntegration tests the complete email verification flow
func TestEmailVerificationIntegration(t *testing.T) {
	ctx := context.Background()

	// Create a test database
	store, err := NewUserStore(ctx, ":memory:")
	require.NoError(t, err)

	// Create dependencies
	passwordHasher := NewDefaultBcryptPasswordHasher()
	emailService := NewEmailService(true, "http://localhost:8080") // Development mode
	mockEventBus := new(MockEventBus)

	// Create command handler
	handler := NewUserCommandHandler(
		mockEventBus,
		nil, // eventStore not needed for this test
		store,
		passwordHasher,
		emailService,
		nil, // webauthnService not needed for this test
	)

	t.Run("complete email verification flow", func(t *testing.T) {
		// Setup event bus expectations
		mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserRegisteredEvent")).Return(nil)
		mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserEmailVerifiedEvent")).Return(nil)

		// Step 1: Register a new user
		userID := uuid.New().String()
		registerCmd := RegisterUserCommand{
			ID:        userID,
			Email:     "test@example.com",
			Password:  "Password123",
			FirstName: "Test",
			LastName:  "User",
			Username:  "testuser",
		}

		err := handler.HandleRegisterUser(ctx, registerCmd)
		require.NoError(t, err)

		// Verify user was created with pending status
		user, err := store.GetUser(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, "Pending", user.Status)
		assert.False(t, user.EmailVerified)

		// Step 2: Get the verification token from the database
		// In a real scenario, this would be sent via email
		// For testing, we'll use the store method to get the token
		verificationToken, err := store.GetEmailVerificationTokenByUserID(ctx, userID)
		require.NoError(t, err)
		require.NotNil(t, verificationToken)
		assert.Equal(t, userID, verificationToken.UserID)
		assert.NotEmpty(t, verificationToken.Token)
		assert.True(t, verificationToken.ExpiresAt.After(time.Now()))

		// Step 3: Verify the email using the token
		verifyCmd := VerifyEmailCommand{
			Token: verificationToken.Token,
		}

		err = handler.HandleVerifyEmail(ctx, verifyCmd)
		require.NoError(t, err)

		// Step 4: Verify user status was updated
		user, err = store.GetUser(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, "Active", user.Status)
		assert.True(t, user.EmailVerified)

		// Step 5: Verify the token was deleted
		deletedToken, err := store.GetEmailVerificationToken(ctx, verificationToken.Token)
		require.NoError(t, err)
		assert.Nil(t, deletedToken)

		// Verify all expectations were met
		mockEventBus.AssertExpectations(t)
	})

	t.Run("token expiration handling", func(t *testing.T) {
		// Create an expired token directly in the database
		userID := uuid.New().String()
		user := &User{
			ID:            userID,
			Email:         "expired@example.com",
			Username:      "expireduser",
			FirstName:     "Expired",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Pending",
			EmailVerified: false,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		err := store.CreateUser(ctx, user)
		require.NoError(t, err)

		// Create an expired token
		expiredToken := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    userID,
			Token:     uuid.New().String(),
			ExpiresAt: time.Now().Add(-time.Hour), // Expired 1 hour ago
			CreatedAt: time.Now().Add(-2 * time.Hour),
		}

		err = store.CreateEmailVerificationToken(ctx, expiredToken)
		require.NoError(t, err)

		// Try to verify with expired token
		verifyCmd := VerifyEmailCommand{
			Token: expiredToken.Token,
		}

		err = handler.HandleVerifyEmail(ctx, verifyCmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "verification token has expired")

		// Verify user status remains unchanged
		user, err = store.GetUser(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, "Pending", user.Status)
		assert.False(t, user.EmailVerified)

		// Verify the expired token was cleaned up
		deletedToken, err := store.GetEmailVerificationToken(ctx, expiredToken.Token)
		require.NoError(t, err)
		assert.Nil(t, deletedToken)
	})

	t.Run("duplicate verification attempt", func(t *testing.T) {
		// Setup event bus expectations
		mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserRegisteredEvent")).Return(nil)
		mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("users.UserEmailVerifiedEvent")).Return(nil)

		// Register a new user
		userID := uuid.New().String()
		registerCmd := RegisterUserCommand{
			ID:        userID,
			Email:     "duplicate@example.com",
			Password:  "Password123",
			FirstName: "Duplicate",
			LastName:  "User",
			Username:  "duplicateuser",
		}

		err := handler.HandleRegisterUser(ctx, registerCmd)
		require.NoError(t, err)

		// Get the verification token
		verificationToken, err := store.GetEmailVerificationTokenByUserID(ctx, userID)
		require.NoError(t, err)
		require.NotNil(t, verificationToken)

		// First verification - should succeed
		verifyCmd := VerifyEmailCommand{
			Token: verificationToken.Token,
		}

		err = handler.HandleVerifyEmail(ctx, verifyCmd)
		require.NoError(t, err)

		// Second verification attempt - should fail
		err = handler.HandleVerifyEmail(ctx, verifyCmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid verification token")

		// Verify all expectations were met
		mockEventBus.AssertExpectations(t)
	})
}

// TestEmailVerificationTokenGeneration tests token generation properties
func TestEmailVerificationTokenGeneration(t *testing.T) {
	ctx := context.Background()

	// Create a test database
	store, err := NewUserStore(ctx, ":memory:")
	require.NoError(t, err)

	t.Run("token uniqueness", func(t *testing.T) {
		// Create multiple users and verify their tokens are unique
		tokens := make(map[string]bool)

		for i := 0; i < 10; i++ {
			userID := uuid.New().String()
			user := &User{
				ID:            userID,
				Email:         fmt.Sprintf("user%d@example.com", i),
				Username:      fmt.Sprintf("user%d", i),
				FirstName:     "Test",
				LastName:      "User",
				Role:          "Viewer",
				Status:        "Pending",
				EmailVerified: false,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}

			err := store.CreateUser(ctx, user)
			require.NoError(t, err)

			// Create verification token
			token := &EmailVerificationToken{
				ID:        uuid.New().String(),
				UserID:    userID,
				Token:     uuid.New().String(),
				ExpiresAt: time.Now().Add(24 * time.Hour),
				CreatedAt: time.Now(),
			}

			err = store.CreateEmailVerificationToken(ctx, token)
			require.NoError(t, err)

			// Verify token is unique
			assert.False(t, tokens[token.Token], "Token should be unique")
			tokens[token.Token] = true
		}
	})

	t.Run("token expiration time", func(t *testing.T) {
		userID := uuid.New().String()
		user := &User{
			ID:            userID,
			Email:         "expiry@example.com",
			Username:      "expiryuser",
			FirstName:     "Expiry",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Pending",
			EmailVerified: false,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		err := store.CreateUser(ctx, user)
		require.NoError(t, err)

		// Create verification token with specific expiration
		now := time.Now()
		expectedExpiry := now.Add(24 * time.Hour)

		token := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    userID,
			Token:     uuid.New().String(),
			ExpiresAt: expectedExpiry,
			CreatedAt: now,
		}

		err = store.CreateEmailVerificationToken(ctx, token)
		require.NoError(t, err)

		// Retrieve and verify expiration time
		retrievedToken, err := store.GetEmailVerificationToken(ctx, token.Token)
		require.NoError(t, err)
		require.NotNil(t, retrievedToken)

		// Allow for small time differences due to processing
		timeDiff := retrievedToken.ExpiresAt.Sub(expectedExpiry)
		assert.True(t, timeDiff < time.Second && timeDiff > -time.Second,
			"Expiration time should match expected value")
	})
}
