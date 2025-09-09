package users

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name        string
		email       string
		expectError bool
	}{
		{
			name:        "valid email",
			email:       "test@example.com",
			expectError: false,
		},
		{
			name:        "valid email with subdomain",
			email:       "user@mail.example.com",
			expectError: false,
		},
		{
			name:        "valid email with plus",
			email:       "user+tag@example.com",
			expectError: false,
		},
		{
			name:        "valid email with numbers",
			email:       "user123@example123.com",
			expectError: false,
		},
		{
			name:        "empty email",
			email:       "",
			expectError: true,
		},
		{
			name:        "email without @",
			email:       "testexample.com",
			expectError: true,
		},
		{
			name:        "email without domain",
			email:       "test@",
			expectError: true,
		},
		{
			name:        "email without local part",
			email:       "@example.com",
			expectError: true,
		},
		{
			name:        "email with spaces",
			email:       "test @example.com",
			expectError: true,
		},
		{
			name:        "email too long",
			email:       "verylongemailaddressthatexceedsthemaximumlengthallowedforemailaddresses@verylongdomainnamethatexceedsthemaximumlengthallowedfordomainnames.com",
			expectError: true,
		},
		{
			name:        "email with invalid characters",
			email:       "test<>@example.com",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		expectError bool
	}{
		{
			name:        "valid password",
			password:    "Password123",
			expectError: false,
		},
		{
			name:        "valid complex password",
			password:    "MySecureP@ssw0rd!",
			expectError: false,
		},
		{
			name:        "empty password",
			password:    "",
			expectError: true,
		},
		{
			name:        "password too short",
			password:    "Pass1",
			expectError: true,
		},
		{
			name:        "password without lowercase",
			password:    "PASSWORD123",
			expectError: true,
		},
		{
			name:        "password without uppercase",
			password:    "password123",
			expectError: true,
		},
		{
			name:        "password without digit",
			password:    "Password",
			expectError: true,
		},
		{
			name:        "password too long",
			password:    "ThisIsAnExtremelyLongPasswordThatExceedsTheMaximumLengthAllowedForPasswordsAndShouldBeRejectedByTheValidationFunction123",
			expectError: true,
		},
		{
			name:        "minimum valid password",
			password:    "Passw0rd",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		expectError bool
	}{
		{
			name:        "valid username",
			username:    "testuser",
			expectError: false,
		},
		{
			name:        "valid username with numbers",
			username:    "user123",
			expectError: false,
		},
		{
			name:        "valid username with underscore",
			username:    "test_user",
			expectError: false,
		},
		{
			name:        "valid username with hyphen",
			username:    "test-user",
			expectError: false,
		},
		{
			name:        "empty username",
			username:    "",
			expectError: true,
		},
		{
			name:        "username too short",
			username:    "a",
			expectError: true,
		},
		{
			name:        "username too long",
			username:    "verylongusernamethatexceedsthemaximumlengthallowed",
			expectError: true,
		},
		{
			name:        "username with spaces",
			username:    "test user",
			expectError: true,
		},
		{
			name:        "username with special characters",
			username:    "test@user",
			expectError: true,
		},
		{
			name:        "username with dots",
			username:    "test.user",
			expectError: true,
		},
		{
			name:        "minimum valid username",
			username:    "ab",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUsername(tt.username)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name        string
		inputName   string
		fieldName   string
		expectError bool
	}{
		{
			name:        "valid first name",
			inputName:   "John",
			fieldName:   "first name",
			expectError: false,
		},
		{
			name:        "valid last name with hyphen",
			inputName:   "Smith-Jones",
			fieldName:   "last name",
			expectError: false,
		},
		{
			name:        "valid name with apostrophe",
			inputName:   "O'Connor",
			fieldName:   "first name",
			expectError: false,
		},
		{
			name:        "valid name with spaces",
			inputName:   "Mary Jane",
			fieldName:   "first name",
			expectError: false,
		},
		{
			name:        "empty name (should be allowed)",
			inputName:   "",
			fieldName:   "first name",
			expectError: false,
		},
		{
			name:        "name too long",
			inputName:   "VeryLongNameThatExceedsTheMaximumLengthAllowedForNamesAndShouldBeRejectedByTheValidationFunctionBecauseItIsTooLong",
			fieldName:   "first name",
			expectError: true,
		},
		{
			name:        "name with numbers",
			inputName:   "John123",
			fieldName:   "first name",
			expectError: true,
		},
		{
			name:        "name with special characters",
			inputName:   "John@Doe",
			fieldName:   "first name",
			expectError: true,
		},
		{
			name:        "name with only spaces",
			inputName:   "   ",
			fieldName:   "first name",
			expectError: false, // Should be trimmed to empty and allowed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.inputName, tt.fieldName)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
