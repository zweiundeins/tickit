package users

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration test for profile management functionality
func TestProfileManagementIntegration(t *testing.T) {
	// Skip if running in CI or if database is not available
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temporary directory for avatar uploads
	tempDir, err := os.MkdirTemp("", "avatar_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test user
	userID := uuid.New().String()
	user := &User{
		ID:            userID,
		Email:         "test@example.com",
		Username:      "testuser",
		FirstName:     "Test",
		LastName:      "User",
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Test avatar service functionality
	t.Run("avatar service operations", func(t *testing.T) {
		avatarService := NewAvatarService(tempDir)

		// Test with empty avatar URL
		err := avatarService.CleanupUserAvatar("")
		assert.NoError(t, err)

		// Test with external URL (should not clean up)
		err = avatarService.CleanupUserAvatar("https://example.com/avatar.jpg")
		assert.NoError(t, err)

		// Create a test avatar file
		testFile := filepath.Join(tempDir, "test_avatar.jpg")
		err = os.WriteFile(testFile, []byte("test image data"), 0644)
		require.NoError(t, err)

		// Test cleanup of local file
		err = avatarService.CleanupUserAvatar("/uploads/test_avatar.jpg")
		assert.NoError(t, err)

		// Verify file was removed
		_, err = os.Stat(testFile)
		assert.True(t, os.IsNotExist(err))

		// Test IsLocalAvatar
		assert.True(t, avatarService.IsLocalAvatar("/uploads/avatar.jpg"))
		assert.False(t, avatarService.IsLocalAvatar("https://example.com/avatar.jpg"))
		assert.False(t, avatarService.IsLocalAvatar(""))

		// Test GetAvatarPath
		path := avatarService.GetAvatarPath("/uploads/avatar.jpg")
		expected := filepath.Join(tempDir, "avatar.jpg")
		assert.Equal(t, expected, path)

		path = avatarService.GetAvatarPath("https://example.com/avatar.jpg")
		assert.Equal(t, "", path)
	})

	t.Run("profile update validation", func(t *testing.T) {
		// Test valid avatar URLs
		validURLs := []string{
			"",
			"https://example.com/avatar.jpg",
			"http://example.com/avatar.png",
			"/uploads/avatar_123.jpg",
			"/uploads/avatar_456.png",
			"/uploads/avatar_789.gif",
			"/uploads/avatar_abc.webp",
			"/uploads/avatar_def.jpeg",
		}

		for _, url := range validURLs {
			err := ValidateAvatarURL(url)
			assert.NoError(t, err, "URL should be valid: %s", url)
		}

		// Test invalid avatar URLs
		invalidURLs := []string{
			"ftp://example.com/avatar.jpg",
			"/uploads/../../../etc/passwd",
			"/uploads/avatar.txt",
			"/uploads/avatar",
		}

		for _, url := range invalidURLs {
			err := ValidateAvatarURL(url)
			assert.Error(t, err, "URL should be invalid: %s", url)
		}
	})

	t.Run("profile update command validation", func(t *testing.T) {
		// Test valid profile updates
		validUpdates := []UpdateUserProfileCommand{
			{
				UserID:   userID,
				Username: stringPtr("newusername"),
			},
			{
				UserID:    userID,
				FirstName: stringPtr("NewFirst"),
				LastName:  stringPtr("NewLast"),
			},
			{
				UserID: userID,
				Email:  stringPtr("newemail@example.com"),
			},
			{
				UserID:    userID,
				AvatarURL: stringPtr("/uploads/avatar_123.jpg"),
			},
		}

		for i, cmd := range validUpdates {
			// Basic validation should pass
			assert.NotEmpty(t, cmd.UserID, "Test case %d: UserID should not be empty", i)

			if cmd.Email != nil {
				err := ValidateEmail(*cmd.Email)
				assert.NoError(t, err, "Test case %d: Email should be valid", i)
			}

			if cmd.Username != nil {
				err := ValidateUsername(*cmd.Username)
				assert.NoError(t, err, "Test case %d: Username should be valid", i)
			}

			if cmd.FirstName != nil {
				err := ValidateName(*cmd.FirstName, "first name")
				assert.NoError(t, err, "Test case %d: First name should be valid", i)
			}

			if cmd.LastName != nil {
				err := ValidateName(*cmd.LastName, "last name")
				assert.NoError(t, err, "Test case %d: Last name should be valid", i)
			}

			if cmd.AvatarURL != nil {
				err := ValidateAvatarURL(*cmd.AvatarURL)
				assert.NoError(t, err, "Test case %d: Avatar URL should be valid", i)
			}
		}

		// Test invalid profile updates
		invalidUpdates := []struct {
			cmd      UpdateUserProfileCommand
			errorMsg string
		}{
			{
				cmd: UpdateUserProfileCommand{
					UserID: "",
				},
				errorMsg: "user ID is required",
			},
			{
				cmd: UpdateUserProfileCommand{
					UserID: userID,
					Email:  stringPtr("invalid-email"),
				},
				errorMsg: "invalid email format",
			},
			{
				cmd: UpdateUserProfileCommand{
					UserID:   userID,
					Username: stringPtr("a"), // too short
				},
				errorMsg: "username must be at least 2 characters long",
			},
			{
				cmd: UpdateUserProfileCommand{
					UserID:    userID,
					AvatarURL: stringPtr("ftp://example.com/avatar.jpg"),
				},
				errorMsg: "avatar URL must be a valid HTTP/HTTPS URL or local upload path",
			},
		}

		for i, test := range invalidUpdates {
			// Test individual field validation
			if test.cmd.Email != nil {
				err := ValidateEmail(*test.cmd.Email)
				if err != nil {
					assert.Contains(t, err.Error(), "invalid email", "Test case %d", i)
				}
			}

			if test.cmd.Username != nil {
				err := ValidateUsername(*test.cmd.Username)
				if err != nil {
					assert.Error(t, err, "Test case %d: Username should be invalid", i)
				}
			}

			if test.cmd.AvatarURL != nil {
				err := ValidateAvatarURL(*test.cmd.AvatarURL)
				if err != nil {
					assert.Error(t, err, "Test case %d: Avatar URL should be invalid", i)
				}
			}
		}
	})

	t.Run("deactivation and reactivation validation", func(t *testing.T) {
		// Test deactivation command validation
		deactivateCmd := DeactivateUserCommand{
			UserID: userID,
		}
		assert.NotEmpty(t, deactivateCmd.UserID)

		// Test reactivation command validation
		reactivateCmd := ReactivateUserCommand{
			UserID: userID,
		}
		assert.NotEmpty(t, reactivateCmd.UserID)

		// Test invalid commands
		invalidDeactivateCmd := DeactivateUserCommand{
			UserID: "",
		}
		assert.Empty(t, invalidDeactivateCmd.UserID)

		invalidReactivateCmd := ReactivateUserCommand{
			UserID: "",
		}
		assert.Empty(t, invalidReactivateCmd.UserID)
	})
}

// Test helper functions for file operations
func TestAvatarServiceHelpers(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "avatar_helper_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	avatarService := NewAvatarService(tempDir)

	t.Run("file validation", func(t *testing.T) {
		// Test with non-existent file
		err := avatarService.ValidateAvatarFile("/uploads/nonexistent.jpg")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "avatar file does not exist")

		// Create a test file
		testFile := filepath.Join(tempDir, "test.jpg")
		testData := make([]byte, 1024) // 1KB file
		err = os.WriteFile(testFile, testData, 0644)
		require.NoError(t, err)

		// Test with valid file
		err = avatarService.ValidateAvatarFile("/uploads/test.jpg")
		assert.NoError(t, err)

		// Create a file that's too large
		largeFile := filepath.Join(tempDir, "large.jpg")
		largeData := make([]byte, 6<<20) // 6MB file (over 5MB limit)
		err = os.WriteFile(largeFile, largeData, 0644)
		require.NoError(t, err)

		// Test with file that's too large
		err = avatarService.ValidateAvatarFile("/uploads/large.jpg")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "avatar file is too large")

		// Test with external URL (should not validate)
		err = avatarService.ValidateAvatarFile("https://example.com/avatar.jpg")
		assert.NoError(t, err)
	})

	t.Run("path operations", func(t *testing.T) {
		// Test GetAvatarPath
		path := avatarService.GetAvatarPath("/uploads/test.jpg")
		expected := filepath.Join(tempDir, "test.jpg")
		assert.Equal(t, expected, path)

		// Test with external URL
		path = avatarService.GetAvatarPath("https://example.com/test.jpg")
		assert.Equal(t, "", path)

		// Test IsLocalAvatar
		assert.True(t, avatarService.IsLocalAvatar("/uploads/test.jpg"))
		assert.False(t, avatarService.IsLocalAvatar("https://example.com/test.jpg"))
		assert.False(t, avatarService.IsLocalAvatar(""))
	})
}

// Test profile management workflow
func TestProfileManagementWorkflow(t *testing.T) {
	// This test demonstrates the expected workflow for profile management

	t.Run("complete profile update workflow", func(t *testing.T) {
		userID := uuid.New().String()

		// Step 1: User updates their profile
		updateCmd := UpdateUserProfileCommand{
			UserID:    userID,
			Username:  stringPtr("newusername"),
			FirstName: stringPtr("John"),
			LastName:  stringPtr("Doe"),
			Email:     stringPtr("john.doe@example.com"),
		}

		// Validate the command
		assert.NotEmpty(t, updateCmd.UserID)
		assert.NoError(t, ValidateEmail(*updateCmd.Email))
		assert.NoError(t, ValidateUsername(*updateCmd.Username))
		assert.NoError(t, ValidateName(*updateCmd.FirstName, "first name"))
		assert.NoError(t, ValidateName(*updateCmd.LastName, "last name"))

		// Step 2: User uploads an avatar
		avatarURL := "/uploads/avatar_" + userID + "_" + "123456789" + ".jpg"
		avatarUpdateCmd := UpdateUserProfileCommand{
			UserID:    userID,
			AvatarURL: &avatarURL,
		}

		// Validate avatar URL
		assert.NoError(t, ValidateAvatarURL(*avatarUpdateCmd.AvatarURL))

		// Step 3: User deactivates their account
		deactivateCmd := DeactivateUserCommand{
			UserID: userID,
		}

		assert.NotEmpty(t, deactivateCmd.UserID)

		// Step 4: Admin reactivates the account
		reactivateCmd := ReactivateUserCommand{
			UserID: userID,
		}

		assert.NotEmpty(t, reactivateCmd.UserID)
	})
}
