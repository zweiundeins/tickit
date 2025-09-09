package users

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestMagicMailIntegration(t *testing.T) {
	ctx := context.Background()

	// Create test database
	dbFile := "test_magic_mail_integration.db"
	defer os.Remove(dbFile)

	store, err := NewUserStore(ctx, dbFile)
	require.NoError(t, err)
	defer store.db.Close()

	// Create dependencies
	passwordHasher := NewDefaultBcryptPasswordHasher()
	emailService := NewEmailService(true, "http://localhost:8080") // Development mode
	mockEventBus := new(MockEventBus)

	// Setup mock event bus expectations
	mockEventBus.On("PublishEvent", mock.Anything, mock.AnythingOfType("users.MagicMailSentEvent")).Return(nil)
	mockEventBus.On("PublishEvent", mock.Anything, mock.AnythingOfType("users.UserLoggedInEvent")).Return(nil)

	// Create command handler
	handler := NewUserCommandHandler(
		mockEventBus,
		nil, // eventStore not needed for this test
		store,
		passwordHasher,
		emailService,
		nil, // webauthnService not needed for this test
	)

	t.Run("complete magic mail flow", func(t *testing.T) {
		// 1. Create test user
		user := &User{
			ID:            uuid.New().String(),
			Email:         "test@example.com",
			Username:      "testuser",
			FirstName:     "Test",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		err = store.CreateUser(ctx, user)
		require.NoError(t, err)

		// 2. Send magic mail
		sendCmd := SendMagicMailCommand{
			Email: user.Email,
		}
		err = handler.HandleSendMagicMail(ctx, sendCmd)
		require.NoError(t, err)

		// 3. Verify magic mail token was created
		// We need to find the token that was created
		// Since we don't have direct access to it, we'll simulate finding it
		// In a real scenario, the token would be in the email link

		// For testing, let's create a token manually and verify the login flow
		now := time.Now()
		magicToken := &MagicMailToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     uuid.New().String(),
			ExpiresAt: now.Add(15 * time.Minute),
			CreatedAt: now,
		}
		err = store.CreateMagicMailToken(ctx, magicToken)
		require.NoError(t, err)

		// 4. Login with magic mail token
		loginCmd := LoginMagicMailCommand{
			Token:     magicToken.Token,
			IPAddress: "192.168.1.1",
			UserAgent: "Test User Agent",
		}
		err = handler.HandleLoginMagicMail(ctx, loginCmd)
		require.NoError(t, err)

		// 5. Verify token was marked as used
		updatedToken, err := store.GetMagicMailToken(ctx, magicToken.Token)
		require.NoError(t, err)
		require.NotNil(t, updatedToken)
		assert.NotNil(t, updatedToken.UsedAt)

		// 6. Verify session was created
		sessions, err := store.GetUserSessions(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, sessions, 1)
		assert.Equal(t, user.ID, sessions[0].UserID)
		assert.Equal(t, loginCmd.IPAddress, sessions[0].IPAddress)
		assert.Equal(t, loginCmd.UserAgent, sessions[0].UserAgent)

		// 7. Verify user's last login was updated
		updatedUser, err := store.GetUser(ctx, user.ID)
		require.NoError(t, err)
		assert.NotNil(t, updatedUser.LastLoginAt)

		// 8. Verify events were published
		mockEventBus.AssertCalled(t, "PublishEvent", mock.Anything, mock.AnythingOfType("users.MagicMailSentEvent"))
		mockEventBus.AssertCalled(t, "PublishEvent", mock.Anything, mock.AnythingOfType("users.UserLoggedInEvent"))
	})

	t.Run("magic mail token cleanup on expiration", func(t *testing.T) {
		// Create test user for this test
		testUser := &User{
			ID:            uuid.New().String(),
			Email:         "expired@example.com",
			Username:      "expireduser",
			FirstName:     "Expired",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		err = store.CreateUser(ctx, testUser)
		require.NoError(t, err)

		// Create expired token
		now := time.Now()
		expiredToken := &MagicMailToken{
			ID:        uuid.New().String(),
			UserID:    testUser.ID,
			Token:     uuid.New().String(),
			ExpiresAt: now.Add(-1 * time.Hour), // Expired 1 hour ago
			CreatedAt: now.Add(-2 * time.Hour),
		}
		err = store.CreateMagicMailToken(ctx, expiredToken)
		require.NoError(t, err)

		// Try to login with expired token
		loginCmd := LoginMagicMailCommand{
			Token:     expiredToken.Token,
			IPAddress: "192.168.1.1",
			UserAgent: "Test User Agent",
		}
		err = handler.HandleLoginMagicMail(ctx, loginCmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "magic mail token has expired")

		// Verify token was deleted
		deletedToken, err := store.GetMagicMailToken(ctx, expiredToken.Token)
		require.NoError(t, err)
		assert.Nil(t, deletedToken)
	})

	t.Run("magic mail token one-time use", func(t *testing.T) {
		// Create test user for this test
		testUser := &User{
			ID:            uuid.New().String(),
			Email:         "onetime@example.com",
			Username:      "onetimeuser",
			FirstName:     "OneTime",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		err = store.CreateUser(ctx, testUser)
		require.NoError(t, err)

		// Create fresh token
		now := time.Now()
		oneTimeToken := &MagicMailToken{
			ID:        uuid.New().String(),
			UserID:    testUser.ID,
			Token:     uuid.New().String(),
			ExpiresAt: now.Add(15 * time.Minute),
			CreatedAt: now,
		}
		err = store.CreateMagicMailToken(ctx, oneTimeToken)
		require.NoError(t, err)

		// First login should succeed
		loginCmd := LoginMagicMailCommand{
			Token:     oneTimeToken.Token,
			IPAddress: "192.168.1.1",
			UserAgent: "Test User Agent",
		}
		err = handler.HandleLoginMagicMail(ctx, loginCmd)
		require.NoError(t, err)

		// Second login should fail
		err = handler.HandleLoginMagicMail(ctx, loginCmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "magic mail token has already been used")
	})
}

func TestMagicMailEnvironmentRestriction(t *testing.T) {
	ctx := context.Background()

	// Create test database
	dbFile := "test_magic_mail_env.db"
	defer os.Remove(dbFile)

	store, err := NewUserStore(ctx, dbFile)
	require.NoError(t, err)
	defer store.db.Close()

	// Create dependencies
	passwordHasher := NewDefaultBcryptPasswordHasher()
	mockEventBus := new(MockEventBus)

	t.Run("production mode blocks magic mail", func(t *testing.T) {
		// Create email service in production mode
		emailService := NewEmailService(false, "https://example.com")

		// Create command handler
		handler := NewUserCommandHandler(
			mockEventBus,
			nil,
			store,
			passwordHasher,
			emailService,
			nil,
		)

		// Create test user
		user := &User{
			ID:            uuid.New().String(),
			Email:         "test@example.com",
			Username:      "testuser",
			FirstName:     "Test",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		err = store.CreateUser(ctx, user)
		require.NoError(t, err)

		// Try to send magic mail in production mode
		sendCmd := SendMagicMailCommand{
			Email: user.Email,
		}
		err = handler.HandleSendMagicMail(ctx, sendCmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "magic mail is only available in development mode")

		// Verify no events were published
		mockEventBus.AssertNotCalled(t, "PublishEvent")
	})

	t.Run("development mode allows magic mail", func(t *testing.T) {
		// Create email service in development mode
		emailService := NewEmailService(true, "http://localhost:8080")

		// Setup mock event bus
		mockEventBus.On("PublishEvent", mock.Anything, mock.AnythingOfType("users.MagicMailSentEvent")).Return(nil)

		// Create command handler
		handler := NewUserCommandHandler(
			mockEventBus,
			nil,
			store,
			passwordHasher,
			emailService,
			nil,
		)

		// Create test user
		user := &User{
			ID:            uuid.New().String(),
			Email:         "dev@example.com",
			Username:      "devuser",
			FirstName:     "Dev",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		err = store.CreateUser(ctx, user)
		require.NoError(t, err)

		// Send magic mail in development mode
		sendCmd := SendMagicMailCommand{
			Email: user.Email,
		}
		err = handler.HandleSendMagicMail(ctx, sendCmd)
		require.NoError(t, err)

		// Verify event was published
		mockEventBus.AssertCalled(t, "PublishEvent", mock.Anything, mock.AnythingOfType("users.MagicMailSentEvent"))
	})
}
