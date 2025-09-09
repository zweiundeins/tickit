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

func TestMagicMailTokenLifecycle(t *testing.T) {
	ctx := context.Background()

	// Create test database
	dbFile := "test_magic_mail.db"
	defer os.Remove(dbFile)

	store, err := NewUserStore(ctx, dbFile)
	require.NoError(t, err)
	defer store.db.Close()

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

	t.Run("create and retrieve magic mail token", func(t *testing.T) {
		now := time.Now()
		token := &MagicMailToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     uuid.New().String(),
			ExpiresAt: now.Add(15 * time.Minute),
			CreatedAt: now,
		}

		// Create token
		err := store.CreateMagicMailToken(ctx, token)
		require.NoError(t, err)

		// Retrieve token
		retrievedToken, err := store.GetMagicMailToken(ctx, token.Token)
		require.NoError(t, err)
		require.NotNil(t, retrievedToken)

		assert.Equal(t, token.ID, retrievedToken.ID)
		assert.Equal(t, token.UserID, retrievedToken.UserID)
		assert.Equal(t, token.Token, retrievedToken.Token)
		assert.Nil(t, retrievedToken.UsedAt)
	})

	t.Run("update magic mail token as used", func(t *testing.T) {
		now := time.Now()
		token := &MagicMailToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     uuid.New().String(),
			ExpiresAt: now.Add(15 * time.Minute),
			CreatedAt: now,
		}

		// Create token
		err := store.CreateMagicMailToken(ctx, token)
		require.NoError(t, err)

		// Mark as used
		usedAt := time.Now()
		token.UsedAt = &usedAt
		err = store.UpdateMagicMailToken(ctx, token)
		require.NoError(t, err)

		// Retrieve and verify
		retrievedToken, err := store.GetMagicMailToken(ctx, token.Token)
		require.NoError(t, err)
		require.NotNil(t, retrievedToken)
		require.NotNil(t, retrievedToken.UsedAt)
		assert.WithinDuration(t, usedAt, *retrievedToken.UsedAt, time.Second)
	})

	t.Run("delete magic mail token", func(t *testing.T) {
		now := time.Now()
		token := &MagicMailToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     uuid.New().String(),
			ExpiresAt: now.Add(15 * time.Minute),
			CreatedAt: now,
		}

		// Create token
		err := store.CreateMagicMailToken(ctx, token)
		require.NoError(t, err)

		// Delete token
		err = store.DeleteMagicMailToken(ctx, token.Token)
		require.NoError(t, err)

		// Verify deletion
		retrievedToken, err := store.GetMagicMailToken(ctx, token.Token)
		require.NoError(t, err)
		assert.Nil(t, retrievedToken)
	})

	t.Run("get non-existent magic mail token", func(t *testing.T) {
		token, err := store.GetMagicMailToken(ctx, "non-existent-token")
		require.NoError(t, err)
		assert.Nil(t, token)
	})
}

func TestSendMagicMailCommand(t *testing.T) {
	ctx := context.Background()

	// Create test database
	dbFile := "test_send_magic_mail.db"
	defer os.Remove(dbFile)

	store, err := NewUserStore(ctx, dbFile)
	require.NoError(t, err)
	defer store.db.Close()

	// Create dependencies
	passwordHasher := NewDefaultBcryptPasswordHasher()
	mockEventBus := new(MockEventBus)

	t.Run("send magic mail in development mode", func(t *testing.T) {
		// Create email service in development mode
		emailService := NewEmailService(true, "http://localhost:8080")

		// Setup mock event bus
		mockEventBus.On("PublishEvent", mock.Anything, mock.AnythingOfType("users.MagicMailSentEvent")).Return(nil)

		// Create command handler
		handler := NewUserCommandHandler(
			mockEventBus,
			nil, // eventStore not needed for this test
			store,
			passwordHasher,
			emailService,
			nil, // webauthnService not needed for this test
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

		// Send magic mail
		cmd := SendMagicMailCommand{
			Email: user.Email,
		}
		err = handler.HandleSendMagicMail(ctx, cmd)
		require.NoError(t, err)

		// Verify event was published
		mockEventBus.AssertCalled(t, "PublishEvent", mock.Anything, mock.AnythingOfType("users.MagicMailSentEvent"))
	})

	t.Run("send magic mail in production mode should fail", func(t *testing.T) {
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

		// Send magic mail
		cmd := SendMagicMailCommand{
			Email: "test@example.com",
		}
		err = handler.HandleSendMagicMail(ctx, cmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "magic mail is only available in development mode")
	})

	t.Run("send magic mail for non-existent user", func(t *testing.T) {
		// Create email service in development mode
		emailService := NewEmailService(true, "http://localhost:8080")

		// Create command handler
		handler := NewUserCommandHandler(
			mockEventBus,
			nil,
			store,
			passwordHasher,
			emailService,
			nil,
		)

		// Send magic mail for non-existent user
		cmd := SendMagicMailCommand{
			Email: "nonexistent@example.com",
		}
		err = handler.HandleSendMagicMail(ctx, cmd)
		// Should not return error to avoid user enumeration
		require.NoError(t, err)

		// Should not publish event
		mockEventBus.AssertNotCalled(t, "PublishEvent")
	})

	t.Run("send magic mail for inactive user", func(t *testing.T) {
		// Create email service in development mode
		emailService := NewEmailService(true, "http://localhost:8080")

		// Create command handler
		handler := NewUserCommandHandler(
			mockEventBus,
			nil,
			store,
			passwordHasher,
			emailService,
			nil,
		)

		// Create inactive user
		user := &User{
			ID:            uuid.New().String(),
			Email:         "inactive@example.com",
			Username:      "inactiveuser",
			FirstName:     "Inactive",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Inactive",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		err = store.CreateUser(ctx, user)
		require.NoError(t, err)

		// Send magic mail
		cmd := SendMagicMailCommand{
			Email: user.Email,
		}
		err = handler.HandleSendMagicMail(ctx, cmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "account is disabled")
	})

	t.Run("send magic mail for unverified user", func(t *testing.T) {
		// Create email service in development mode
		emailService := NewEmailService(true, "http://localhost:8080")

		// Create command handler
		handler := NewUserCommandHandler(
			mockEventBus,
			nil,
			store,
			passwordHasher,
			emailService,
			nil,
		)

		// Create unverified user
		user := &User{
			ID:            uuid.New().String(),
			Email:         "unverified@example.com",
			Username:      "unverifieduser",
			FirstName:     "Unverified",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Pending",
			EmailVerified: false,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		err = store.CreateUser(ctx, user)
		require.NoError(t, err)

		// Send magic mail
		cmd := SendMagicMailCommand{
			Email: user.Email,
		}
		err = handler.HandleSendMagicMail(ctx, cmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "email not verified")
	})

	t.Run("send magic mail with invalid email", func(t *testing.T) {
		// Create email service in development mode
		emailService := NewEmailService(true, "http://localhost:8080")

		// Create command handler
		handler := NewUserCommandHandler(
			mockEventBus,
			nil,
			store,
			passwordHasher,
			emailService,
			nil,
		)

		// Send magic mail with invalid email
		cmd := SendMagicMailCommand{
			Email: "invalid-email",
		}
		err = handler.HandleSendMagicMail(ctx, cmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid email")
	})
}

func TestLoginMagicMailCommand(t *testing.T) {
	ctx := context.Background()

	// Create test database
	dbFile := "test_login_magic_mail.db"
	defer os.Remove(dbFile)

	store, err := NewUserStore(ctx, dbFile)
	require.NoError(t, err)
	defer store.db.Close()

	// Create dependencies
	passwordHasher := NewDefaultBcryptPasswordHasher()
	emailService := NewEmailService(true, "http://localhost:8080")
	mockEventBus := new(MockEventBus)

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

	t.Run("successful magic mail login", func(t *testing.T) {
		// Setup mock event bus
		mockEventBus.On("PublishEvent", mock.Anything, mock.AnythingOfType("users.UserLoggedInEvent")).Return(nil)

		// Create magic mail token
		now := time.Now()
		token := &MagicMailToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     uuid.New().String(),
			ExpiresAt: now.Add(15 * time.Minute),
			CreatedAt: now,
		}
		err = store.CreateMagicMailToken(ctx, token)
		require.NoError(t, err)

		// Login with magic mail
		cmd := LoginMagicMailCommand{
			Token:     token.Token,
			IPAddress: "192.168.1.1",
			UserAgent: "Test User Agent",
		}
		err = handler.HandleLoginMagicMail(ctx, cmd)
		require.NoError(t, err)

		// Verify token was marked as used
		updatedToken, err := store.GetMagicMailToken(ctx, token.Token)
		require.NoError(t, err)
		require.NotNil(t, updatedToken)
		assert.NotNil(t, updatedToken.UsedAt)

		// Verify session was created
		sessions, err := store.GetUserSessions(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, sessions, 1)
		assert.Equal(t, user.ID, sessions[0].UserID)
		assert.Equal(t, cmd.IPAddress, sessions[0].IPAddress)
		assert.Equal(t, cmd.UserAgent, sessions[0].UserAgent)

		// Verify event was published
		mockEventBus.AssertCalled(t, "PublishEvent", mock.Anything, mock.AnythingOfType("users.UserLoggedInEvent"))
	})

	t.Run("login with invalid token", func(t *testing.T) {
		cmd := LoginMagicMailCommand{
			Token:     "invalid-token",
			IPAddress: "192.168.1.1",
			UserAgent: "Test User Agent",
		}
		err = handler.HandleLoginMagicMail(ctx, cmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid magic mail token")
	})

	t.Run("login with empty token", func(t *testing.T) {
		cmd := LoginMagicMailCommand{
			Token:     "",
			IPAddress: "192.168.1.1",
			UserAgent: "Test User Agent",
		}
		err = handler.HandleLoginMagicMail(ctx, cmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "magic mail token is required")
	})

	t.Run("login with expired token", func(t *testing.T) {
		// Create expired magic mail token
		now := time.Now()
		token := &MagicMailToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     uuid.New().String(),
			ExpiresAt: now.Add(-1 * time.Hour), // Expired 1 hour ago
			CreatedAt: now.Add(-2 * time.Hour),
		}
		err = store.CreateMagicMailToken(ctx, token)
		require.NoError(t, err)

		// Login with expired token
		cmd := LoginMagicMailCommand{
			Token:     token.Token,
			IPAddress: "192.168.1.1",
			UserAgent: "Test User Agent",
		}
		err = handler.HandleLoginMagicMail(ctx, cmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "magic mail token has expired")

		// Verify token was deleted
		deletedToken, err := store.GetMagicMailToken(ctx, token.Token)
		require.NoError(t, err)
		assert.Nil(t, deletedToken)
	})

	t.Run("login with already used token", func(t *testing.T) {
		// Create used magic mail token
		now := time.Now()
		usedAt := now.Add(-5 * time.Minute)
		token := &MagicMailToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     uuid.New().String(),
			ExpiresAt: now.Add(15 * time.Minute),
			CreatedAt: now.Add(-10 * time.Minute),
			UsedAt:    &usedAt,
		}
		err = store.CreateMagicMailToken(ctx, token)
		require.NoError(t, err)

		// Login with used token
		cmd := LoginMagicMailCommand{
			Token:     token.Token,
			IPAddress: "192.168.1.1",
			UserAgent: "Test User Agent",
		}
		err = handler.HandleLoginMagicMail(ctx, cmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "magic mail token has already been used")
	})

	t.Run("login for inactive user", func(t *testing.T) {
		// Create inactive user
		inactiveUser := &User{
			ID:            uuid.New().String(),
			Email:         "inactive@example.com",
			Username:      "inactiveuser",
			FirstName:     "Inactive",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Inactive",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		err = store.CreateUser(ctx, inactiveUser)
		require.NoError(t, err)

		// Create magic mail token for inactive user
		now := time.Now()
		token := &MagicMailToken{
			ID:        uuid.New().String(),
			UserID:    inactiveUser.ID,
			Token:     uuid.New().String(),
			ExpiresAt: now.Add(15 * time.Minute),
			CreatedAt: now,
		}
		err = store.CreateMagicMailToken(ctx, token)
		require.NoError(t, err)

		// Login with magic mail
		cmd := LoginMagicMailCommand{
			Token:     token.Token,
			IPAddress: "192.168.1.1",
			UserAgent: "Test User Agent",
		}
		err = handler.HandleLoginMagicMail(ctx, cmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "account is disabled")
	})
}

func TestMagicMailEnvironmentToggle(t *testing.T) {
	t.Run("development environment detection", func(t *testing.T) {
		// Test default (development)
		service := NewEmailServiceFromEnv()
		assert.True(t, service.isDevelopment)

		// Test explicit development
		os.Setenv("ENVIRONMENT", "development")
		service = NewEmailServiceFromEnv()
		assert.True(t, service.isDevelopment)

		// Test production
		os.Setenv("ENVIRONMENT", "production")
		service = NewEmailServiceFromEnv()
		assert.False(t, service.isDevelopment)

		// Clean up
		os.Unsetenv("ENVIRONMENT")
	})

	t.Run("magic mail only works in development", func(t *testing.T) {
		ctx := context.Background()

		// Development mode - should work
		devService := NewEmailService(true, "http://localhost:8080")
		err := devService.SendMagicMailLink(ctx, "test@example.com", "test-token")
		assert.NoError(t, err)

		// Production mode - should fail
		prodService := NewEmailService(false, "https://example.com")
		err = prodService.SendMagicMailLink(ctx, "test@example.com", "test-token")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "magic mail is only available in development mode")
	})
}
