package users

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
)

// OAuthService handles OAuth authentication flows
type OAuthService struct {
	oauthManager *OAuthManager
	userStore    UserStore
	sessionStore map[string]*OAuthSession // In production, use Redis or similar
}

// OAuthSession represents a temporary OAuth session
type OAuthSession struct {
	State     string    `json:"state"`
	Provider  string    `json:"provider"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewOAuthService creates a new OAuth service instance
func NewOAuthService(oauthManager *OAuthManager, userStore UserStore) *OAuthService {
	return &OAuthService{
		oauthManager: oauthManager,
		userStore:    userStore,
		sessionStore: make(map[string]*OAuthSession),
	}
}

// BeginOAuthFlow initiates the OAuth authentication flow
func (s *OAuthService) BeginOAuthFlow(ctx context.Context, provider string, w http.ResponseWriter, r *http.Request) error {
	// Normalize provider name
	provider = NormalizeProviderName(provider)

	// Validate provider
	if !s.oauthManager.IsProviderEnabled(provider) {
		return fmt.Errorf("OAuth provider %s is not enabled", provider)
	}

	// Generate secure state parameter for CSRF protection
	state, err := s.generateSecureState()
	if err != nil {
		return fmt.Errorf("failed to generate state parameter: %w", err)
	}

	// Store OAuth session
	now := time.Now()
	session := &OAuthSession{
		State:     state,
		Provider:  provider,
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute), // OAuth flow expires in 10 minutes
	}
	s.sessionStore[state] = session

	// Set provider in context for Gothic
	r = r.WithContext(context.WithValue(r.Context(), "provider", provider))

	// Use Gothic to begin OAuth flow
	gothic.BeginAuthHandler(w, r)

	return nil
}

// CompleteOAuthFlow completes the OAuth authentication flow
func (s *OAuthService) CompleteOAuthFlow(ctx context.Context, provider, code, state string, r *http.Request) (*goth.User, error) {
	// Normalize provider name
	provider = NormalizeProviderName(provider)

	// Validate state parameter
	session, exists := s.sessionStore[state]
	if !exists {
		return nil, fmt.Errorf("invalid or expired OAuth state")
	}

	// Check session expiration
	if time.Now().After(session.ExpiresAt) {
		delete(s.sessionStore, state)
		return nil, fmt.Errorf("OAuth session has expired")
	}

	// Validate provider matches
	if session.Provider != provider {
		delete(s.sessionStore, state)
		return nil, fmt.Errorf("provider mismatch in OAuth flow")
	}

	// Clean up session (one-time use)
	delete(s.sessionStore, state)

	// Validate provider is enabled
	if !s.oauthManager.IsProviderEnabled(provider) {
		return nil, fmt.Errorf("OAuth provider %s is not enabled", provider)
	}

	// Set provider in context for Gothic
	r = r.WithContext(context.WithValue(r.Context(), "provider", provider))

	// Use Gothic to complete OAuth flow
	gothUser, err := gothic.CompleteUserAuth(nil, r)
	if err != nil {
		return nil, fmt.Errorf("failed to complete OAuth authentication: %w", err)
	}

	return &gothUser, nil
}

// CreateOrLinkUser creates a new user or links OAuth account to existing user
func (s *OAuthService) CreateOrLinkUser(ctx context.Context, gothUser *goth.User, ipAddress, userAgent string) (*User, *UserSession, error) {
	// Check if user already exists with this OAuth provider
	existingUser, err := s.userStore.GetUserByOAuthProvider(ctx, gothUser.Provider, gothUser.UserID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to check existing OAuth user: %w", err)
	}

	var user *User

	if existingUser != nil {
		// User already exists with this OAuth provider
		user = existingUser
	} else {
		// Check if user exists with the same email
		if gothUser.Email != "" {
			existingEmailUser, err := s.userStore.GetUserByEmail(ctx, strings.ToLower(gothUser.Email))
			if err != nil {
				return nil, nil, fmt.Errorf("failed to check existing email user: %w", err)
			}

			if existingEmailUser != nil {
				// Link OAuth account to existing user
				user = existingEmailUser
				if err := s.linkOAuthToUser(ctx, user, gothUser); err != nil {
					return nil, nil, fmt.Errorf("failed to link OAuth account: %w", err)
				}
			} else {
				// Create new user from OAuth data
				user, err = s.createUserFromOAuth(ctx, gothUser)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to create user from OAuth: %w", err)
				}
			}
		} else {
			return nil, nil, fmt.Errorf("OAuth provider did not provide email address")
		}
	}

	// Check user status
	if user.Status != "Active" {
		if user.Status == "Inactive" {
			return nil, nil, fmt.Errorf("account is disabled")
		}
		// For OAuth users, we can activate them immediately since email is verified by the provider
		if user.Status == "Pending" {
			user.Status = "Active"
			user.EmailVerified = true
			user.UpdatedAt = time.Now()
			if err := s.userStore.UpdateUser(ctx, user); err != nil {
				return nil, nil, fmt.Errorf("failed to activate OAuth user: %w", err)
			}
		}
	}

	// Create session
	session, err := s.createUserSession(ctx, user.ID, ipAddress, userAgent)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Update last login time
	now := time.Now()
	user.LastLoginAt = &now
	user.UpdatedAt = now
	if err := s.userStore.UpdateUser(ctx, user); err != nil {
		// Log error but don't fail login
		fmt.Printf("Failed to update user last login time: %v\n", err)
	}

	return user, session, nil
}

// linkOAuthToUser links an OAuth account to an existing user
func (s *OAuthService) linkOAuthToUser(ctx context.Context, user *User, gothUser *goth.User) error {
	// Check if this OAuth provider is already linked
	existingCredential, err := s.userStore.GetUserCredentialByProvider(ctx, user.ID, gothUser.Provider)
	if err != nil {
		return fmt.Errorf("failed to check existing OAuth credential: %w", err)
	}

	now := time.Now()

	if existingCredential != nil {
		// Update existing credential
		existingCredential.ProviderID = gothUser.UserID
		existingCredential.CredentialData = s.serializeOAuthData(gothUser)
		existingCredential.UpdatedAt = now

		if err := s.userStore.UpdateUserCredential(ctx, existingCredential); err != nil {
			return fmt.Errorf("failed to update OAuth credential: %w", err)
		}
	} else {
		// Create new OAuth credential
		credential := &UserCredential{
			ID:             generateID(),
			UserID:         user.ID,
			Type:           "oauth",
			Provider:       gothUser.Provider,
			ProviderID:     gothUser.UserID,
			CredentialData: s.serializeOAuthData(gothUser),
			CreatedAt:      now,
			UpdatedAt:      now,
		}

		if err := s.userStore.CreateUserCredential(ctx, credential); err != nil {
			return fmt.Errorf("failed to create OAuth credential: %w", err)
		}
	}

	return nil
}

// createUserFromOAuth creates a new user from OAuth data
func (s *OAuthService) createUserFromOAuth(ctx context.Context, gothUser *goth.User) (*User, error) {
	now := time.Now()

	// Generate username from OAuth data
	username := s.generateUsernameFromOAuth(gothUser)

	// Ensure username is unique
	uniqueUsername, err := s.ensureUniqueUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to generate unique username: %w", err)
	}

	// Create user
	user := &User{
		ID:            generateID(),
		Email:         strings.ToLower(strings.TrimSpace(gothUser.Email)),
		Username:      uniqueUsername,
		FirstName:     gothUser.FirstName,
		LastName:      gothUser.LastName,
		AvatarURL:     gothUser.AvatarURL,
		Role:          "Viewer", // Default role
		Status:        "Active", // OAuth users are immediately active
		EmailVerified: true,     // Email is verified by OAuth provider
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.userStore.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Create OAuth credential
	credential := &UserCredential{
		ID:             generateID(),
		UserID:         user.ID,
		Type:           "oauth",
		Provider:       gothUser.Provider,
		ProviderID:     gothUser.UserID,
		CredentialData: s.serializeOAuthData(gothUser),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.userStore.CreateUserCredential(ctx, credential); err != nil {
		return nil, fmt.Errorf("failed to create OAuth credential: %w", err)
	}

	return user, nil
}

// createUserSession creates a new user session
func (s *OAuthService) createUserSession(ctx context.Context, userID, ipAddress, userAgent string) (*UserSession, error) {
	now := time.Now()
	session := &UserSession{
		ID:        generateID(),
		UserID:    userID,
		SessionID: generateID(),            // This would be the Gorilla session ID in practice
		ExpiresAt: now.Add(24 * time.Hour), // Session expires in 24 hours
		CreatedAt: now,
		IPAddress: ipAddress,
		UserAgent: userAgent,
	}

	if err := s.userStore.CreateUserSession(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

// generateUsernameFromOAuth generates a username from OAuth data
func (s *OAuthService) generateUsernameFromOAuth(gothUser *goth.User) string {
	// Try nickname first
	if gothUser.NickName != "" {
		return gothUser.NickName
	}

	// Try name
	if gothUser.Name != "" {
		// Replace spaces with underscores and make lowercase
		return strings.ToLower(strings.ReplaceAll(gothUser.Name, " ", "_"))
	}

	// Try first name + last name
	if gothUser.FirstName != "" && gothUser.LastName != "" {
		return strings.ToLower(gothUser.FirstName + "_" + gothUser.LastName)
	}

	// Try first name only
	if gothUser.FirstName != "" {
		return strings.ToLower(gothUser.FirstName)
	}

	// Fall back to email prefix
	if gothUser.Email != "" {
		parts := strings.Split(gothUser.Email, "@")
		if len(parts) > 0 {
			return strings.ToLower(parts[0])
		}
	}

	// Last resort: use provider + user ID
	return strings.ToLower(gothUser.Provider + "_" + gothUser.UserID)
}

// ensureUniqueUsername ensures the username is unique by appending numbers if needed
func (s *OAuthService) ensureUniqueUsername(ctx context.Context, baseUsername string) (string, error) {
	username := baseUsername
	counter := 1

	for {
		// Check if username exists
		existingUser, err := s.userStore.GetUserByUsername(ctx, username)
		if err != nil {
			return "", fmt.Errorf("failed to check username uniqueness: %w", err)
		}

		if existingUser == nil {
			// Username is available
			return username, nil
		}

		// Try with counter
		username = fmt.Sprintf("%s_%d", baseUsername, counter)
		counter++

		// Prevent infinite loop
		if counter > 1000 {
			return "", fmt.Errorf("failed to generate unique username after 1000 attempts")
		}
	}
}

// serializeOAuthData serializes OAuth user data for storage
func (s *OAuthService) serializeOAuthData(gothUser *goth.User) string {
	// In a real implementation, you might want to store this as JSON
	// For now, we'll store the access token and refresh token
	data := fmt.Sprintf("access_token:%s", gothUser.AccessToken)
	if gothUser.RefreshToken != "" {
		data += fmt.Sprintf("|refresh_token:%s", gothUser.RefreshToken)
	}
	return data
}

// generateSecureState generates a cryptographically secure state parameter
func (s *OAuthService) generateSecureState() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// CleanupExpiredSessions removes expired OAuth sessions
func (s *OAuthService) CleanupExpiredSessions() {
	now := time.Now()
	for state, session := range s.sessionStore {
		if now.After(session.ExpiresAt) {
			delete(s.sessionStore, state)
		}
	}
}

// generateID generates a new UUID string
func generateID() string {
	// This would use uuid.New().String() in practice
	// For now, we'll use a simple implementation
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
}
