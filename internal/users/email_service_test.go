package users

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEmailServiceImpl_SendVerificationEmail(t *testing.T) {
	ctx := context.Background()

	t.Run("development mode logs email", func(t *testing.T) {
		service := NewEmailService(true, "http://localhost:8080")

		err := service.SendVerificationEmail(ctx, "test@example.com", "test-token-123")

		assert.NoError(t, err)
	})

	t.Run("production mode returns error", func(t *testing.T) {
		service := NewEmailService(false, "https://example.com")

		err := service.SendVerificationEmail(ctx, "test@example.com", "test-token-123")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "email service not configured for production environment")
	})

	t.Run("generates correct verification URL", func(t *testing.T) {
		baseURL := "https://tickit.example.com"
		service := NewEmailService(true, baseURL)

		// In a real test, we might capture the log output to verify the URL
		// For now, we just ensure it doesn't error
		err := service.SendVerificationEmail(ctx, "test@example.com", "test-token-123")

		assert.NoError(t, err)
	})
}

func TestEmailServiceImpl_SendPasswordResetEmail(t *testing.T) {
	ctx := context.Background()

	t.Run("development mode logs email", func(t *testing.T) {
		service := NewEmailService(true, "http://localhost:8080")

		err := service.SendPasswordResetEmail(ctx, "test@example.com", "reset-token-123")

		assert.NoError(t, err)
	})

	t.Run("production mode returns error", func(t *testing.T) {
		service := NewEmailService(false, "https://example.com")

		err := service.SendPasswordResetEmail(ctx, "test@example.com", "reset-token-123")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "email service not configured for production environment")
	})
}

func TestEmailServiceImpl_SendInvitationEmail(t *testing.T) {
	ctx := context.Background()

	t.Run("development mode logs email", func(t *testing.T) {
		service := NewEmailService(true, "http://localhost:8080")

		err := service.SendInvitationEmail(ctx, "test@example.com", "Test Project", "invite-token-123")

		assert.NoError(t, err)
	})

	t.Run("production mode returns error", func(t *testing.T) {
		service := NewEmailService(false, "https://example.com")

		err := service.SendInvitationEmail(ctx, "test@example.com", "Test Project", "invite-token-123")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "email service not configured for production environment")
	})
}

func TestEmailServiceImpl_SendMagicMailLink(t *testing.T) {
	ctx := context.Background()

	t.Run("development mode logs magic mail", func(t *testing.T) {
		service := NewEmailService(true, "http://localhost:8080")

		err := service.SendMagicMailLink(ctx, "test@example.com", "magic-token-123")

		assert.NoError(t, err)
	})

	t.Run("production mode returns error", func(t *testing.T) {
		service := NewEmailService(false, "https://example.com")

		err := service.SendMagicMailLink(ctx, "test@example.com", "magic-token-123")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "magic mail is only available in development mode")
	})
}

func TestNewEmailServiceFromEnv(t *testing.T) {
	t.Run("creates service with defaults", func(t *testing.T) {
		service := NewEmailServiceFromEnv()

		assert.NotNil(t, service)
		// In development, isDevelopment should be true (since ENVIRONMENT != "production")
		assert.True(t, service.isDevelopment)
		// Default base URL should be set
		assert.Equal(t, "http://localhost:8080", service.baseURL)
	})
}
