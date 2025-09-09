package users

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateAvatarURL(t *testing.T) {
	tests := []struct {
		name        string
		avatarURL   string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty URL is valid",
			avatarURL:   "",
			expectError: false,
		},
		{
			name:        "whitespace only URL is valid",
			avatarURL:   "   ",
			expectError: false,
		},
		{
			name:        "valid HTTPS URL",
			avatarURL:   "https://example.com/avatar.jpg",
			expectError: false,
		},
		{
			name:        "valid HTTP URL",
			avatarURL:   "http://example.com/avatar.png",
			expectError: false,
		},
		{
			name:        "valid local upload path",
			avatarURL:   "/uploads/avatar_123.jpg",
			expectError: false,
		},
		{
			name:        "valid local upload path with different extension",
			avatarURL:   "/uploads/avatar_456.png",
			expectError: false,
		},
		{
			name:        "valid local upload path with gif",
			avatarURL:   "/uploads/avatar_789.gif",
			expectError: false,
		},
		{
			name:        "valid local upload path with webp",
			avatarURL:   "/uploads/avatar_abc.webp",
			expectError: false,
		},
		{
			name:        "valid local upload path with jpeg",
			avatarURL:   "/uploads/avatar_def.jpeg",
			expectError: false,
		},
		{
			name:        "URL too long",
			avatarURL:   "https://example.com/" + string(make([]byte, 500)) + "avatar.jpg",
			expectError: true,
			errorMsg:    "avatar URL is too long",
		},
		{
			name:        "invalid protocol",
			avatarURL:   "ftp://example.com/avatar.jpg",
			expectError: true,
			errorMsg:    "avatar URL must be a valid HTTP/HTTPS URL or local upload path",
		},
		{
			name:        "local path with directory traversal",
			avatarURL:   "/uploads/../../../etc/passwd",
			expectError: true,
			errorMsg:    "avatar URL contains invalid path characters",
		},
		{
			name:        "local path with invalid extension",
			avatarURL:   "/uploads/avatar.txt",
			expectError: true,
			errorMsg:    "avatar URL must point to a valid image file",
		},
		{
			name:        "local path with no extension",
			avatarURL:   "/uploads/avatar",
			expectError: true,
			errorMsg:    "avatar URL must point to a valid image file",
		},
		{
			name:        "case insensitive extension validation",
			avatarURL:   "/uploads/avatar.JPG",
			expectError: false,
		},
		{
			name:        "case insensitive extension validation - PNG",
			avatarURL:   "/uploads/avatar.PNG",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAvatarURL(tt.avatarURL)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProfileUpdateValidation(t *testing.T) {
	// Test that existing validation functions work correctly for profile updates

	t.Run("email validation", func(t *testing.T) {
		validEmails := []string{
			"test@example.com",
			"user.name@domain.co.uk",
			"user+tag@example.org",
		}

		for _, email := range validEmails {
			assert.NoError(t, ValidateEmail(email), "Email should be valid: %s", email)
		}

		invalidEmails := []string{
			"",
			"invalid-email",
			"@example.com",
			"user@",
			"user@.com",
		}

		for _, email := range invalidEmails {
			assert.Error(t, ValidateEmail(email), "Email should be invalid: %s", email)
		}
	})

	t.Run("username validation", func(t *testing.T) {
		validUsernames := []string{
			"user123",
			"test_user",
			"user-name",
			"TestUser",
		}

		for _, username := range validUsernames {
			assert.NoError(t, ValidateUsername(username), "Username should be valid: %s", username)
		}

		invalidUsernames := []string{
			"",
			"a",                      // too short
			"user name",              // contains space
			"user@name",              // contains @
			string(make([]byte, 51)), // too long
		}

		for _, username := range invalidUsernames {
			assert.Error(t, ValidateUsername(username), "Username should be invalid: %s", username)
		}
	})

	t.Run("name validation", func(t *testing.T) {
		validNames := []string{
			"John",
			"Mary-Jane",
			"O'Connor",
			"Jean-Luc",
			"", // empty is allowed
		}

		for _, name := range validNames {
			assert.NoError(t, ValidateName(name, "test name"), "Name should be valid: %s", name)
		}

		invalidNames := []string{
			"John123",                 // contains numbers
			"John@Doe",                // contains @
			string(make([]byte, 101)), // too long
		}

		for _, name := range invalidNames {
			assert.Error(t, ValidateName(name, "test name"), "Name should be invalid: %s", name)
		}
	})
}
