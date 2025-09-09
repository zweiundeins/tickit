package users

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// BcryptPasswordHasher implements the PasswordHasher interface using bcrypt
type BcryptPasswordHasher struct {
	cost int
}

// NewBcryptPasswordHasher creates a new BcryptPasswordHasher with the specified cost
// Cost factor 12 is recommended for production use as per security best practices
func NewBcryptPasswordHasher(cost int) *BcryptPasswordHasher {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = 12 // Default to cost 12 for security
	}
	return &BcryptPasswordHasher{cost: cost}
}

// NewDefaultBcryptPasswordHasher creates a new BcryptPasswordHasher with default cost 12
func NewDefaultBcryptPasswordHasher() *BcryptPasswordHasher {
	return NewBcryptPasswordHasher(12)
}

// NewPasswordHasher creates a new PasswordHasher with default settings
func NewPasswordHasher() PasswordHasher {
	return NewDefaultBcryptPasswordHasher()
}

// HashPassword hashes a password using bcrypt with the configured cost factor
func (h *BcryptPasswordHasher) HashPassword(password string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("password cannot be empty")
	}

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	return string(hashedBytes), nil
}

// CheckPassword verifies a password against its hash using bcrypt
func (h *BcryptPasswordHasher) CheckPassword(hashedPassword, password string) error {
	if hashedPassword == "" {
		return fmt.Errorf("hashed password cannot be empty")
	}
	if password == "" {
		return fmt.Errorf("password cannot be empty")
	}

	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return fmt.Errorf("invalid password")
		}
		return fmt.Errorf("failed to verify password: %w", err)
	}

	return nil
}
