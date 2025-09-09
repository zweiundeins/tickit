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

func TestUserModelsAndDatabase(t *testing.T) {
	// Create a temporary database file
	dbFile := "test_models.db"
	defer os.Remove(dbFile)

	ctx := context.Background()
	store, err := NewUserStore(ctx, dbFile)
	require.NoError(t, err)
	defer store.db.Close()

	t.Run("All User Models CRUD", func(t *testing.T) {
		// Create test user
		userID := uuid.New().String()
		user := &User{
			ID:            userID,
			Email:         "test-" + userID + "@example.com",
			Username:      "testuser",
			FirstName:     "Test",
			LastName:      "User",
			AvatarURL:     "https://example.com/avatar.jpg",
			Role:          "Developer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		err := store.CreateUser(ctx, user)
		require.NoError(t, err)

		// Test UserCredential
		credential := &UserCredential{
			ID:             uuid.New().String(),
			UserID:         user.ID,
			Type:           "password",
			Provider:       "local",
			ProviderID:     "",
			CredentialData: "hashed_password_data",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		err = store.CreateUserCredential(ctx, credential)
		require.NoError(t, err)

		// Test UserSession
		session := &UserSession{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			SessionID: "session_" + uuid.New().String(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
			CreatedAt: time.Now(),
			IPAddress: "192.168.1.1",
			UserAgent: "Mozilla/5.0 Test Browser",
		}
		err = store.CreateUserSession(ctx, session)
		require.NoError(t, err)

		// Test ProjectMembership
		projectID := uuid.New().String()
		membership := &ProjectMembership{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			ProjectID: projectID,
			Role:      "Developer",
			CreatedAt: time.Now(),
			CreatedBy: user.ID,
		}
		err = store.CreateProjectMembership(ctx, membership)
		require.NoError(t, err)

		// Test EmailVerificationToken
		emailToken := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     "email_verify_" + uuid.New().String(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
			CreatedAt: time.Now(),
		}
		err = store.CreateEmailVerificationToken(ctx, emailToken)
		require.NoError(t, err)

		// Test PasswordResetToken
		resetToken := &PasswordResetToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     "reset_" + uuid.New().String(),
			ExpiresAt: time.Now().Add(1 * time.Hour),
			CreatedAt: time.Now(),
		}
		err = store.CreatePasswordResetToken(ctx, resetToken)
		require.NoError(t, err)

		// Test MagicMailToken
		magicToken := &MagicMailToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     "magic_" + uuid.New().String(),
			ExpiresAt: time.Now().Add(15 * time.Minute),
			CreatedAt: time.Now(),
		}
		err = store.CreateMagicMailToken(ctx, magicToken)
		require.NoError(t, err)

		// Test UserInvitation
		invitation := &UserInvitation{
			ID:        uuid.New().String(),
			ProjectID: projectID,
			Email:     "invited-" + uuid.New().String() + "@example.com",
			Role:      "Viewer",
			Token:     "invite_" + uuid.New().String(),
			ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
			CreatedAt: time.Now(),
			CreatedBy: user.ID,
		}
		err = store.CreateInvitation(ctx, invitation)
		require.NoError(t, err)

		// Verify all data can be retrieved
		retrievedUser, err := store.GetUser(ctx, user.ID)
		require.NoError(t, err)
		assert.Equal(t, user.Email, retrievedUser.Email)

		credentials, err := store.GetUserCredentials(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, credentials, 1)

		sessions, err := store.GetUserSessions(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, sessions, 1)

		memberships, err := store.GetUserProjects(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, memberships, 1)

		retrievedEmailToken, err := store.GetEmailVerificationToken(ctx, emailToken.Token)
		require.NoError(t, err)
		assert.Equal(t, emailToken.UserID, retrievedEmailToken.UserID)

		retrievedResetToken, err := store.GetPasswordResetToken(ctx, resetToken.Token)
		require.NoError(t, err)
		assert.Equal(t, resetToken.UserID, retrievedResetToken.UserID)

		retrievedMagicToken, err := store.GetMagicMailToken(ctx, magicToken.Token)
		require.NoError(t, err)
		assert.Equal(t, magicToken.UserID, retrievedMagicToken.UserID)

		retrievedInvitation, err := store.GetInvitationByToken(ctx, invitation.Token)
		require.NoError(t, err)
		assert.Equal(t, invitation.Email, retrievedInvitation.Email)
	})

	t.Run("Database Schema Validation", func(t *testing.T) {
		// Test that all required tables exist and have correct structure
		// This is implicitly tested by the successful CRUD operations above

		// Test user stats functionality
		userID := uuid.New().String()
		user := &User{
			ID:            userID,
			Email:         "stats-" + userID + "@example.com",
			Username:      "statsuser",
			FirstName:     "Stats",
			LastName:      "User",
			Role:          "Admin",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		err := store.CreateUser(ctx, user)
		require.NoError(t, err)

		// Test user stats
		userStats, err := store.GetUserStats(ctx, user.ID)
		require.NoError(t, err)
		assert.Contains(t, userStats, "created_at")
		assert.Contains(t, userStats, "email_verified")
		assert.Contains(t, userStats, "status")

		// Test system stats
		systemStats, err := store.GetSystemUserStats(ctx)
		require.NoError(t, err)
		assert.Contains(t, systemStats, "total_users")
		assert.Contains(t, systemStats, "active_users")
		assert.Contains(t, systemStats, "verified_users")
		assert.Contains(t, systemStats, "users_by_role")
	})

	t.Run("PatchUser Functionality", func(t *testing.T) {
		// Create a user to patch
		userID := uuid.New().String()
		user := &User{
			ID:            userID,
			Email:         "patch-" + userID + "@example.com",
			Username:      "patchuser",
			FirstName:     "Patch",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Pending",
			EmailVerified: false,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		err := store.CreateUser(ctx, user)
		require.NoError(t, err)

		// Test partial updates
		patch := &PatchUserInput{
			ID:            user.ID,
			Role:          stringPtr("Developer"),
			Status:        stringPtr("Active"),
			EmailVerified: boolPtr(true),
		}
		err = store.PatchUser(ctx, patch)
		require.NoError(t, err)

		// Verify changes
		updatedUser, err := store.GetUser(ctx, user.ID)
		require.NoError(t, err)
		assert.Equal(t, "Developer", updatedUser.Role)
		assert.Equal(t, "Active", updatedUser.Status)
		assert.True(t, updatedUser.EmailVerified)
		// Unchanged fields should remain the same
		assert.Equal(t, user.FirstName, updatedUser.FirstName)
		assert.Equal(t, user.LastName, updatedUser.LastName)
	})
}

// Helper functions are now in test_helpers.go
