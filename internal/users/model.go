package users

import "time"

// User represents the core user entity
type User struct {
	ID            string     `json:"id"`       // UUID
	Email         string     `json:"email"`    // Primary identifier
	Username      string     `json:"username"` // Display name
	FirstName     string     `json:"first_name"`
	LastName      string     `json:"last_name"`
	AvatarURL     string     `json:"avatar_url"`
	Role          string     `json:"role"`   // Admin, ProjectManager, Developer, Viewer
	Status        string     `json:"status"` // Active, Inactive, Pending
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	LastLoginAt   *time.Time `json:"last_login_at"`
	EmailVerified bool       `json:"email_verified"`
}

// UserCredential stores authentication credentials
type UserCredential struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	Type           string    `json:"type"`            // password, oauth, webauthn
	Provider       string    `json:"provider"`        // local, github, google, microsoft
	ProviderID     string    `json:"provider_id"`     // External provider user ID
	CredentialData string    `json:"credential_data"` // Encrypted/hashed credential
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// UserSession manages user sessions
type UserSession struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	SessionID string    `json:"session_id"` // Gorilla session ID
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
}

// ProjectMembership defines user access to projects
type ProjectMembership struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ProjectID string    `json:"project_id"`
	Role      string    `json:"role"` // ProjectAdmin, Developer, Viewer
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"` // User ID who added this member
}

// EmailVerificationToken stores email verification tokens
type EmailVerificationToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// PasswordResetToken stores password reset tokens
type PasswordResetToken struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Token     string     `json:"token"`
	ExpiresAt time.Time  `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
	UsedAt    *time.Time `json:"used_at"`
}

// MagicMailToken stores magic mail login tokens (development only)
type MagicMailToken struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Token     string     `json:"token"`
	ExpiresAt time.Time  `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
	UsedAt    *time.Time `json:"used_at"`
}

// UserInvitation represents project invitations
type UserInvitation struct {
	ID         string     `json:"id"`
	ProjectID  string     `json:"project_id"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	Token      string     `json:"token"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
	CreatedBy  string     `json:"created_by"`
	AcceptedAt *time.Time `json:"accepted_at"`
	DeclinedAt *time.Time `json:"declined_at"`
}

// PatchUserInput represents input for patching user data
// Only non-nil fields will be updated, ID is required
type PatchUserInput struct {
	ID            string  `json:"id"`
	Email         *string `json:"email"`
	Username      *string `json:"username"`
	FirstName     *string `json:"first_name"`
	LastName      *string `json:"last_name"`
	AvatarURL     *string `json:"avatar_url"`
	Role          *string `json:"role"`
	Status        *string `json:"status"`
	EmailVerified *bool   `json:"email_verified"`
}