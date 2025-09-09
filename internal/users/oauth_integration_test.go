package users

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/markbates/goth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuthManager_LoadConfigFromEnvironment(t *testing.T) {
	// Save original environment
	originalGitHubClientID := os.Getenv("GITHUB_CLIENT_ID")
	originalGitHubClientSecret := os.Getenv("GITHUB_CLIENT_SECRET")
	originalGoogleClientID := os.Getenv("GOOGLE_CLIENT_ID")
	originalGoogleClientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	originalBaseURL := os.Getenv("BASE_URL")

	// Cleanup function
	defer func() {
		os.Setenv("GITHUB_CLIENT_ID", originalGitHubClientID)
		os.Setenv("GITHUB_CLIENT_SECRET", originalGitHubClientSecret)
		os.Setenv("GOOGLE_CLIENT_ID", originalGoogleClientID)
		os.Setenv("GOOGLE_CLIENT_SECRET", originalGoogleClientSecret)
		os.Setenv("BASE_URL", originalBaseURL)
	}()

	t.Run("loads GitHub configuration", func(t *testing.T) {
		// Set test environment variables
		os.Setenv("GITHUB_CLIENT_ID", "test-github-client-id")
		os.Setenv("GITHUB_CLIENT_SECRET", "test-github-client-secret")
		os.Setenv("BASE_URL", "http://localhost:8080")

		manager := NewOAuthManager()
		err := manager.LoadConfigFromEnvironment()
		require.NoError(t, err)

		assert.True(t, manager.IsProviderEnabled("github"))

		config, err := manager.GetProviderConfig("github")
		require.NoError(t, err)
		assert.Equal(t, "test-github-client-id", config.ClientID)
		assert.Equal(t, "test-github-client-secret", config.ClientSecret)
		assert.Equal(t, "http://localhost:8080/auth/oauth/github/callback", config.CallbackURL)
		assert.Contains(t, config.Scopes, "user:email")
	})

	t.Run("loads Google configuration", func(t *testing.T) {
		// Set test environment variables
		os.Setenv("GOOGLE_CLIENT_ID", "test-google-client-id")
		os.Setenv("GOOGLE_CLIENT_SECRET", "test-google-client-secret")
		os.Setenv("BASE_URL", "http://localhost:8080")

		manager := NewOAuthManager()
		err := manager.LoadConfigFromEnvironment()
		require.NoError(t, err)

		assert.True(t, manager.IsProviderEnabled("google"))

		config, err := manager.GetProviderConfig("google")
		require.NoError(t, err)
		assert.Equal(t, "test-google-client-id", config.ClientID)
		assert.Equal(t, "test-google-client-secret", config.ClientSecret)
		assert.Equal(t, "http://localhost:8080/auth/oauth/google/callback", config.CallbackURL)
		assert.Contains(t, config.Scopes, "email")
		assert.Contains(t, config.Scopes, "profile")
	})

	t.Run("handles missing configuration", func(t *testing.T) {
		// Clear environment variables
		os.Unsetenv("GITHUB_CLIENT_ID")
		os.Unsetenv("GITHUB_CLIENT_SECRET")
		os.Unsetenv("GOOGLE_CLIENT_ID")
		os.Unsetenv("GOOGLE_CLIENT_SECRET")

		manager := NewOAuthManager()
		err := manager.LoadConfigFromEnvironment()
		require.NoError(t, err)

		assert.False(t, manager.IsProviderEnabled("github"))
		assert.False(t, manager.IsProviderEnabled("google"))
		assert.Empty(t, manager.GetEnabledProviders())
	})

	t.Run("handles disabled providers", func(t *testing.T) {
		// Set test environment variables with disabled flag
		os.Setenv("GITHUB_CLIENT_ID", "test-github-client-id")
		os.Setenv("GITHUB_CLIENT_SECRET", "test-github-client-secret")
		os.Setenv("GITHUB_ENABLED", "false")

		manager := NewOAuthManager()
		err := manager.LoadConfigFromEnvironment()
		require.NoError(t, err)

		assert.False(t, manager.IsProviderEnabled("github"))
	})
}

func TestOAuthManager_ProviderValidation(t *testing.T) {
	manager := NewOAuthManager()

	t.Run("validates supported providers", func(t *testing.T) {
		assert.True(t, IsProviderSupported("github"))
		assert.True(t, IsProviderSupported("google"))
		assert.True(t, IsProviderSupported("microsoft"))
		assert.False(t, IsProviderSupported("unsupported"))
	})

	t.Run("normalizes provider names", func(t *testing.T) {
		assert.Equal(t, "github", NormalizeProviderName("GitHub"))
		assert.Equal(t, "github", NormalizeProviderName("GITHUB"))
		assert.Equal(t, "github", NormalizeProviderName(" github "))
	})

	t.Run("returns supported providers list", func(t *testing.T) {
		providers := GetSupportedProviders()
		assert.Contains(t, providers, "github")
		assert.Contains(t, providers, "google")
		assert.Contains(t, providers, "microsoft")
	})
}

func TestOAuthService_StateGeneration(t *testing.T) {
	manager := NewOAuthManager()
	service := NewOAuthService(manager, nil)

	t.Run("generates secure state", func(t *testing.T) {
		state1, err := service.generateSecureState()
		require.NoError(t, err)
		assert.NotEmpty(t, state1)

		state2, err := service.generateSecureState()
		require.NoError(t, err)
		assert.NotEmpty(t, state2)

		// States should be different
		assert.NotEqual(t, state1, state2)

		// States should be reasonable length
		assert.Greater(t, len(state1), 20)
		assert.Greater(t, len(state2), 20)
	})
}

func TestOAuthService_UsernameGeneration(t *testing.T) {
	manager := NewOAuthManager()
	service := NewOAuthService(manager, nil)

	tests := []struct {
		name     string
		gothUser *goth.User
		expected string
	}{
		{
			name: "uses nickname",
			gothUser: &goth.User{
				NickName: "testuser",
				Name:     "Test User",
				Email:    "test@example.com",
			},
			expected: "testuser",
		},
		{
			name: "uses name when no nickname",
			gothUser: &goth.User{
				Name:  "Test User",
				Email: "test@example.com",
			},
			expected: "test_user",
		},
		{
			name: "uses first and last name",
			gothUser: &goth.User{
				FirstName: "Test",
				LastName:  "User",
				Email:     "test@example.com",
			},
			expected: "test_user",
		},
		{
			name: "uses first name only",
			gothUser: &goth.User{
				FirstName: "Test",
				Email:     "test@example.com",
			},
			expected: "test",
		},
		{
			name: "uses email prefix as fallback",
			gothUser: &goth.User{
				Email: "testuser@example.com",
			},
			expected: "testuser",
		},
		{
			name: "uses provider and user ID as last resort",
			gothUser: &goth.User{
				Provider: "github",
				UserID:   "12345",
			},
			expected: "github_12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username := service.generateUsernameFromOAuth(tt.gothUser)
			assert.Equal(t, tt.expected, username)
		})
	}
}

func TestOAuthService_SessionManagement(t *testing.T) {
	manager := NewOAuthManager()
	service := NewOAuthService(manager, nil)

	t.Run("stores and validates OAuth sessions", func(t *testing.T) {
		// Create a session
		state := "test-state"
		provider := "github"
		session := &OAuthSession{
			State:     state,
			Provider:  provider,
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(10 * time.Minute),
		}
		service.sessionStore[state] = session

		// Validate session exists
		storedSession, exists := service.sessionStore[state]
		assert.True(t, exists)
		assert.Equal(t, provider, storedSession.Provider)
		assert.Equal(t, state, storedSession.State)
	})

	t.Run("cleans up expired sessions", func(t *testing.T) {
		// Create expired session
		expiredState := "expired-state"
		expiredSession := &OAuthSession{
			State:     expiredState,
			Provider:  "github",
			CreatedAt: time.Now().Add(-20 * time.Minute),
			ExpiresAt: time.Now().Add(-10 * time.Minute),
		}
		service.sessionStore[expiredState] = expiredSession

		// Create valid session
		validState := "valid-state"
		validSession := &OAuthSession{
			State:     validState,
			Provider:  "github",
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(10 * time.Minute),
		}
		service.sessionStore[validState] = validSession

		// Cleanup expired sessions
		service.CleanupExpiredSessions()

		// Expired session should be removed
		_, exists := service.sessionStore[expiredState]
		assert.False(t, exists)

		// Valid session should remain
		_, exists = service.sessionStore[validState]
		assert.True(t, exists)
	})
}

func TestOAuthCommandHandlers(t *testing.T) {
	// This would test the command handlers, but requires a full setup
	// with user store, event bus, etc. For now, we'll test the basic structure

	t.Run("validates OAuth command input", func(t *testing.T) {
		handler := &UserCommandHandler{}

		// Test with empty provider
		cmd := LinkOAuthAccountCommand{
			UserID:     "user-123",
			Provider:   "",
			ProviderID: "provider-123",
		}

		err := handler.HandleLinkOAuthAccount(context.Background(), cmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth provider is required")

		// Test with unsupported provider
		cmd.Provider = "unsupported"
		err = handler.HandleLinkOAuthAccount(context.Background(), cmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not supported")
	})
}

// Mock implementations for testing

type MockUserStoreForOAuth struct {
	users       map[string]*User
	credentials map[string][]*UserCredential
}

func NewMockUserStoreForOAuth() *MockUserStoreForOAuth {
	return &MockUserStore{
		users:       make(map[string]*User),
		credentials: make(map[string][]*UserCredential),
	}
}

func (m *MockUserStoreForOAuth) GetUser(ctx context.Context, userID string) (*User, error) {
	user, exists := m.users[userID]
	if !exists {
		return nil, nil
	}
	return user, nil
}

func (m *MockUserStoreForOAuth) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	for _, user := range m.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, nil
}

func (m *MockUserStoreForOAuth) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	for _, user := range m.users {
		if user.Username == username {
			return user, nil
		}
	}
	return nil, nil
}

func (m *MockUserStoreForOAuth) GetUserByOAuthProvider(ctx context.Context, provider, providerID string) (*User, error) {
	for userID, creds := range m.credentials {
		for _, cred := range creds {
			if cred.Type == "oauth" && cred.Provider == provider && cred.ProviderID == providerID {
				return m.users[userID], nil
			}
		}
	}
	return nil, nil
}

func (m *MockUserStoreForOAuth) CreateUser(ctx context.Context, user *User) error {
	m.users[user.ID] = user
	return nil
}

func (m *MockUserStoreForOAuth) UpdateUser(ctx context.Context, user *User) error {
	m.users[user.ID] = user
	return nil
}

func (m *MockUserStoreForOAuth) CreateUserCredential(ctx context.Context, credential *UserCredential) error {
	m.credentials[credential.UserID] = append(m.credentials[credential.UserID], credential)
	return nil
}

func (m *MockUserStoreForOAuth) CreateUserSession(ctx context.Context, session *UserSession) error {
	return nil
}

// Implement other required methods as no-ops for testing
func (m *MockUserStoreForOAuth) ListUsers(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*User, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) SearchUsers(ctx context.Context, query string, limit, offset int) ([]*User, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) PatchUser(ctx context.Context, patch *PatchUserInput) error { return nil }
func (m *MockUserStoreForOAuth) DeleteUser(ctx context.Context, userID string) error        { return nil }
func (m *MockUserStoreForOAuth) GetUserCredentials(ctx context.Context, userID string) ([]*UserCredential, error) {
	return m.credentials[userID], nil
}
func (m *MockUserStoreForOAuth) GetUserCredentialByProvider(ctx context.Context, userID, provider string) (*UserCredential, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) UpdateUserCredential(ctx context.Context, credential *UserCredential) error {
	return nil
}
func (m *MockUserStoreForOAuth) DeleteUserCredential(ctx context.Context, credentialID string) error {
	return nil
}
func (m *MockUserStoreForOAuth) GetUserSession(ctx context.Context, sessionID string) (*UserSession, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) GetUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) GetActiveUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) UpdateUserSession(ctx context.Context, session *UserSession) error {
	return nil
}
func (m *MockUserStoreForOAuth) DeleteUserSession(ctx context.Context, sessionID string) error  { return nil }
func (m *MockUserStoreForOAuth) DeleteAllUserSessions(ctx context.Context, userID string) error { return nil }
func (m *MockUserStoreForOAuth) DeleteExpiredSessions(ctx context.Context) (int, error)         { return 0, nil }
func (m *MockUserStoreForOAuth) GetProjectMembers(ctx context.Context, projectID string) ([]*User, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) GetUserProjects(ctx context.Context, userID string) ([]*ProjectMembership, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) GetProjectMembership(ctx context.Context, userID, projectID string) (*ProjectMembership, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) CheckUserProjectAccess(ctx context.Context, userID, projectID string) (bool, error) {
	return false, nil
}
func (m *MockUserStoreForOAuth) CreateProjectMembership(ctx context.Context, membership *ProjectMembership) error {
	return nil
}
func (m *MockUserStoreForOAuth) UpdateProjectMembership(ctx context.Context, membership *ProjectMembership) error {
	return nil
}
func (m *MockUserStoreForOAuth) DeleteProjectMembership(ctx context.Context, userID, projectID string) error {
	return nil
}
func (m *MockUserStoreForOAuth) GetEmailVerificationToken(ctx context.Context, token string) (*EmailVerificationToken, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) GetEmailVerificationTokenByUserID(ctx context.Context, userID string) (*EmailVerificationToken, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) CreateEmailVerificationToken(ctx context.Context, token *EmailVerificationToken) error {
	return nil
}
func (m *MockUserStoreForOAuth) DeleteEmailVerificationToken(ctx context.Context, token string) error {
	return nil
}
func (m *MockUserStoreForOAuth) GetPasswordResetToken(ctx context.Context, token string) (*PasswordResetToken, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) CreatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error {
	return nil
}
func (m *MockUserStoreForOAuth) UpdatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error {
	return nil
}
func (m *MockUserStoreForOAuth) DeletePasswordResetToken(ctx context.Context, token string) error { return nil }
func (m *MockUserStoreForOAuth) GetMagicMailToken(ctx context.Context, token string) (*MagicMailToken, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) CreateMagicMailToken(ctx context.Context, token *MagicMailToken) error {
	return nil
}
func (m *MockUserStoreForOAuth) UpdateMagicMailToken(ctx context.Context, token *MagicMailToken) error {
	return nil
}
func (m *MockUserStoreForOAuth) DeleteMagicMailToken(ctx context.Context, token string) error { return nil }
func (m *MockUserStoreForOAuth) GetProjectInvitations(ctx context.Context, projectID string) ([]*UserInvitation, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) GetUserInvitations(ctx context.Context, email string) ([]*UserInvitation, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) GetInvitationByToken(ctx context.Context, token string) (*UserInvitation, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) CreateInvitation(ctx context.Context, invitation *UserInvitation) error {
	return nil
}
func (m *MockUserStoreForOAuth) UpdateInvitation(ctx context.Context, invitation *UserInvitation) error {
	return nil
}
func (m *MockUserStoreForOAuth) DeleteInvitation(ctx context.Context, invitationID string) error { return nil }
func (m *MockUserStoreForOAuth) GetUserActivity(ctx context.Context, userID string, limit, offset int) ([]interface{}, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) GetUserStats(ctx context.Context, userID string) (map[string]interface{}, error) {
	return nil, nil
}
func (m *MockUserStoreForOAuth) GetSystemUserStats(ctx context.Context) (map[string]interface{}, error) {
	return nil, nil
}
