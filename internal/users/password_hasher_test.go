package users

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBcryptPasswordHasher_HashPassword(t *testing.T) {
	hasher := NewDefaultBcryptPasswordHasher()

	tests := []struct {
		name        string
		password    string
		expectError bool
	}{
		{
			name:        "valid password",
			password:    "ValidPassword123",
			expectError: false,
		},
		{
			name:        "empty password",
			password:    "",
			expectError: true,
		},
		{
			name:        "long password",
			password:    "ThisIsAVeryLongPasswordThatShouldStillWork123456789",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := hasher.HashPassword(tt.password)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, hash)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, hash)
				assert.NotEqual(t, tt.password, hash)

				// Verify the hash starts with bcrypt prefix
				assert.True(t, len(hash) > 10)
				assert.Equal(t, "$2a$", hash[:4])
			}
		})
	}
}

func TestBcryptPasswordHasher_CheckPassword(t *testing.T) {
	hasher := NewDefaultBcryptPasswordHasher()
	password := "TestPassword123"

	// Generate a hash for testing
	hash, err := hasher.HashPassword(password)
	require.NoError(t, err)
	require.NotEmpty(t, hash)

	tests := []struct {
		name           string
		hashedPassword string
		password       string
		expectError    bool
	}{
		{
			name:           "correct password",
			hashedPassword: hash,
			password:       password,
			expectError:    false,
		},
		{
			name:           "incorrect password",
			hashedPassword: hash,
			password:       "WrongPassword",
			expectError:    true,
		},
		{
			name:           "empty hashed password",
			hashedPassword: "",
			password:       password,
			expectError:    true,
		},
		{
			name:           "empty password",
			hashedPassword: hash,
			password:       "",
			expectError:    true,
		},
		{
			name:           "invalid hash format",
			hashedPassword: "invalid-hash",
			password:       password,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := hasher.CheckPassword(tt.hashedPassword, tt.password)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBcryptPasswordHasher_DifferentCosts(t *testing.T) {
	password := "TestPassword123"

	tests := []struct {
		name string
		cost int
	}{
		{"minimum cost", 4},
		{"default cost", 12},
		{"high cost", 14},
		{"invalid low cost", 2},   // Should default to 12
		{"invalid high cost", 32}, // Should default to 12
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasher := NewBcryptPasswordHasher(tt.cost)

			hash, err := hasher.HashPassword(password)
			assert.NoError(t, err)
			assert.NotEmpty(t, hash)

			err = hasher.CheckPassword(hash, password)
			assert.NoError(t, err)
		})
	}
}

func TestBcryptPasswordHasher_ConsistentHashing(t *testing.T) {
	hasher := NewDefaultBcryptPasswordHasher()
	password := "TestPassword123"

	// Generate multiple hashes of the same password
	hash1, err1 := hasher.HashPassword(password)
	hash2, err2 := hasher.HashPassword(password)

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotEmpty(t, hash1)
	assert.NotEmpty(t, hash2)

	// Hashes should be different (due to salt)
	assert.NotEqual(t, hash1, hash2)

	// But both should verify correctly
	assert.NoError(t, hasher.CheckPassword(hash1, password))
	assert.NoError(t, hasher.CheckPassword(hash2, password))
}
