package users

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserStore_BasicCRUD(t *testing.T) {
	// Create a temporary database file
	dbFile := "test_users.db"
	defer os.Remove(dbFile)

	ctx := context.Background()
	store, err := NewUserStore(ctx, dbFile)
	require.NoError(t, err)
	defer store.db.Close()

	// Test User CRUD operations
	t.Run("User CRUD", func(t *testing.T) {
		// Create a test user
		user := &User{
			ID:            uuid.New().String(),
			Email:         "test@example.com",
			Username:      "testuser",
			FirstName:     "Test",
			LastName:      "User",
			Role:          "Developer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		// Test CreateUser
		err := store.CreateUser(ctx, user)
		assert.NoError(t, err)

		// Test GetUser
		retrievedUser, err := store.GetUser(ctx, user.ID)
		require.NoError(t, err)
		require.NotNil(t, retrievedUser)
		assert.Equal(t, user.Email, retrievedUser.Email)
		assert.Equal(t, user.Username, retrievedUser.Username)
		assert.Equal(t, user.Role, retrievedUser.Role)

		// Test GetUserByEmail
		userByEmail, err := store.GetUserByEmail(ctx, user.Email)
		require.NoError(t, err)
		require.NotNil(t, userByEmail)
		assert.Equal(t, user.ID, userByEmail.ID)

		// Test UpdateUser
		user.FirstName = "Updated"
		user.UpdatedAt = time.Now()
		err = store.UpdateUser(ctx, user)
		assert.NoError(t, err)

		updatedUser, err := store.GetUser(ctx, user.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated", updatedUser.FirstName)

		// Test PatchUser
		patch := &PatchUserInput{
			ID:       user.ID,
			LastName: stringPtr("Patched"),
		}
		err = store.PatchUser(ctx, patch)
		assert.NoError(t, err)

		patchedUser, err := store.GetUser(ctx, user.ID)
		require.NoError(t, err)
		assert.Equal(t, "Patched", patchedUser.LastName)

		// Test ListUsers
		users, err := store.ListUsers(ctx, 10, 0, nil)
		require.NoError(t, err)
		assert.Len(t, users, 1)

		// Test SearchUsers
		searchResults, err := store.SearchUsers(ctx, "test", 10, 0)
		require.NoError(t, err)
		assert.Len(t, searchResults, 1)

		// Test DeleteUser
		err = store.DeleteUser(ctx, user.ID)
		assert.NoError(t, err)

		deletedUser, err := store.GetUser(ctx, user.ID)
		assert.NoError(t, err)
		assert.Nil(t, deletedUser)
	})

	t.Run("UserCredential CRUD", func(t *testing.T) {
		// Create a test user first
		user := &User{
			ID:            uuid.New().String(),
			Email:         "cred@example.com",
			Username:      "creduser",
			FirstName:     "Cred",
			LastName:      "User",
			Role:          "Developer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		err := store.CreateUser(ctx, user)
		require.NoError(t, err)

		// Create a test credential
		credential := &UserCredential{
			ID:             uuid.New().String(),
			UserID:         user.ID,
			Type:           "password",
			Provider:       "local",
			CredentialData: "hashed_password_here",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		// Test CreateUserCredential
		err = store.CreateUserCredential(ctx, credential)
		assert.NoError(t, err)

		// Test GetUserCredentials
		credentials, err := store.GetUserCredentials(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, credentials, 1)
		assert.Equal(t, credential.Type, credentials[0].Type)

		// Test GetUserCredentialByProvider
		credByProvider, err := store.GetUserCredentialByProvider(ctx, user.ID, "local")
		require.NoError(t, err)
		require.NotNil(t, credByProvider)
		assert.Equal(t, credential.ID, credByProvider.ID)

		// Test UpdateUserCredential
		credential.CredentialData = "updated_password_hash"
		credential.UpdatedAt = time.Now()
		err = store.UpdateUserCredential(ctx, credential)
		assert.NoError(t, err)

		// Test DeleteUserCredential
		err = store.DeleteUserCredential(ctx, credential.ID)
		assert.NoError(t, err)

		credentials, err = store.GetUserCredentials(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, credentials, 0)
	})

	t.Run("UserSession CRUD", func(t *testing.T) {
		// Create a test user first
		user := &User{
			ID:            uuid.New().String(),
			Email:         "session@example.com",
			Username:      "sessionuser",
			FirstName:     "Session",
			LastName:      "User",
			Role:          "Developer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		err := store.CreateUser(ctx, user)
		require.NoError(t, err)

		// Create a test session
		session := &UserSession{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			SessionID: "session_123",
			ExpiresAt: time.Now().Add(24 * time.Hour),
			CreatedAt: time.Now(),
			IPAddress: "127.0.0.1",
			UserAgent: "Test Agent",
		}

		// Test CreateUserSession
		err = store.CreateUserSession(ctx, session)
		assert.NoError(t, err)

		// Test GetUserSession
		retrievedSession, err := store.GetUserSession(ctx, session.SessionID)
		require.NoError(t, err)
		require.NotNil(t, retrievedSession)
		assert.Equal(t, session.UserID, retrievedSession.UserID)

		// Test GetUserSessions
		sessions, err := store.GetUserSessions(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, sessions, 1)

		// Test GetActiveUserSessions
		activeSessions, err := store.GetActiveUserSessions(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, activeSessions, 1)

		// Test DeleteUserSession
		err = store.DeleteUserSession(ctx, session.SessionID)
		assert.NoError(t, err)

		deletedSession, err := store.GetUserSession(ctx, session.SessionID)
		assert.NoError(t, err)
		assert.Nil(t, deletedSession)
	})
}

func TestUserStore_EmailVerificationTokens(t *testing.T) {
	// Create a temporary database file
	dbFile := "test_email_tokens.db"
	defer os.Remove(dbFile)

	ctx := context.Background()
	store, err := NewUserStore(ctx, dbFile)
	require.NoError(t, err)
	defer store.db.Close()

	// First create a user to associate with tokens
	user := &User{
		ID:            uuid.New().String(),
		Email:         "test@example.com",
		Username:      "testuser",
		FirstName:     "Test",
		LastName:      "User",
		Role:          "Viewer",
		Status:        "Pending",
		EmailVerified: false,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	t.Run("Create and retrieve email verification token", func(t *testing.T) {
		token := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     "test-verification-token-123",
			ExpiresAt: time.Now().Add(24 * time.Hour),
			CreatedAt: time.Now(),
		}

		// Test CreateEmailVerificationToken
		err := store.CreateEmailVerificationToken(ctx, token)
		assert.NoError(t, err)

		// Test GetEmailVerificationToken
		retrievedToken, err := store.GetEmailVerificationToken(ctx, token.Token)
		require.NoError(t, err)
		require.NotNil(t, retrievedToken)
		assert.Equal(t, token.ID, retrievedToken.ID)
		assert.Equal(t, token.UserID, retrievedToken.UserID)
		assert.Equal(t, token.Token, retrievedToken.Token)
		assert.WithinDuration(t, token.ExpiresAt, retrievedToken.ExpiresAt, time.Second)
		assert.WithinDuration(t, token.CreatedAt, retrievedToken.CreatedAt, time.Second)
	})

	t.Run("Get non-existent token returns nil", func(t *testing.T) {
		retrievedToken, err := store.GetEmailVerificationToken(ctx, "non-existent-token")
		assert.NoError(t, err)
		assert.Nil(t, retrievedToken)
	})

	t.Run("Delete email verification token", func(t *testing.T) {
		token := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     "token-to-delete",
			ExpiresAt: time.Now().Add(24 * time.Hour),
			CreatedAt: time.Now(),
		}

		// Create token
		err := store.CreateEmailVerificationToken(ctx, token)
		require.NoError(t, err)

		// Verify it exists
		retrievedToken, err := store.GetEmailVerificationToken(ctx, token.Token)
		require.NoError(t, err)
		require.NotNil(t, retrievedToken)

		// Delete token
		err = store.DeleteEmailVerificationToken(ctx, token.Token)
		assert.NoError(t, err)

		// Verify it's gone
		retrievedToken, err = store.GetEmailVerificationToken(ctx, token.Token)
		assert.NoError(t, err)
		assert.Nil(t, retrievedToken)
	})

	t.Run("Delete non-existent token succeeds", func(t *testing.T) {
		err := store.DeleteEmailVerificationToken(ctx, "non-existent-token")
		assert.NoError(t, err)
	})

	t.Run("Token uniqueness constraint", func(t *testing.T) {
		tokenValue := "duplicate-token-test"

		token1 := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     tokenValue,
			ExpiresAt: time.Now().Add(24 * time.Hour),
			CreatedAt: time.Now(),
		}

		token2 := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     tokenValue, // Same token value
			ExpiresAt: time.Now().Add(24 * time.Hour),
			CreatedAt: time.Now(),
		}

		// First token should succeed
		err := store.CreateEmailVerificationToken(ctx, token1)
		assert.NoError(t, err)

		// Second token with same value should fail
		err = store.CreateEmailVerificationToken(ctx, token2)
		assert.Error(t, err)
	})

	t.Run("Token expiration handling", func(t *testing.T) {
		// Create an expired token
		expiredToken := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     "expired-token-test",
			ExpiresAt: time.Now().Add(-time.Hour), // Expired 1 hour ago
			CreatedAt: time.Now().Add(-2 * time.Hour),
		}

		err := store.CreateEmailVerificationToken(ctx, expiredToken)
		assert.NoError(t, err)

		// Should still be able to retrieve expired token (expiration logic is in business layer)
		retrievedToken, err := store.GetEmailVerificationToken(ctx, expiredToken.Token)
		assert.NoError(t, err)
		assert.NotNil(t, retrievedToken)
		assert.True(t, time.Now().After(retrievedToken.ExpiresAt))
	})

	t.Run("Multiple tokens per user", func(t *testing.T) {
		// Create multiple tokens for the same user (e.g., if user requests multiple verifications)
		token1 := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     "user-token-1",
			ExpiresAt: time.Now().Add(24 * time.Hour),
			CreatedAt: time.Now(),
		}

		token2 := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     "user-token-2",
			ExpiresAt: time.Now().Add(24 * time.Hour),
			CreatedAt: time.Now(),
		}

		err := store.CreateEmailVerificationToken(ctx, token1)
		assert.NoError(t, err)

		err = store.CreateEmailVerificationToken(ctx, token2)
		assert.NoError(t, err)

		// Both tokens should be retrievable
		retrievedToken1, err := store.GetEmailVerificationToken(ctx, token1.Token)
		assert.NoError(t, err)
		assert.NotNil(t, retrievedToken1)

		retrievedToken2, err := store.GetEmailVerificationToken(ctx, token2.Token)
		assert.NoError(t, err)
		assert.NotNil(t, retrievedToken2)
	})
}
