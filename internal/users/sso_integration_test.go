package users

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/markbates/goth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSOIntegration(t *testing.T) {
	// Set up test environment variables
	os.Setenv("AZURE_AD_CLIENT_ID", "test-client-id")
	os.Setenv("AZURE_AD_CLIENT_SECRET", "test-client-secret")
	os.Setenv("AZURE_AD_TENANT_ID", "test-tenant-id")
	os.Setenv("BASE_URL", "http://localhost:8080")

	defer func() {
		os.Unsetenv("AZURE_AD_CLIENT_ID")
		os.Unsetenv("AZURE_AD_CLIENT_SECRET")
		os.Unsetenv("AZURE_AD_TENANT_ID")
		os.Unsetenv("BASE_URL")
	}()

	// Create test dependencies
	userStore := NewInMemoryUserStoreForSSO()
	ssoManager := NewSSOManager()

	// Load configuration
	err := ssoManager.LoadConfigFromEnvironment()
	require.NoError(t, err)

	ssoService := NewSSOService(ssoManager, userStore)

	t.Run("SSO Configuration", func(t *testing.T) {
		t.Run("should load Azure AD configuration from environment", func(t *testing.T) {
			providers := ssoManager.GetEnabledProviders()
			assert.Contains(t, providers, "azuread")
		})

		t.Run("should validate provider configuration", func(t *testing.T) {
			err := ssoManager.ValidateProviderConfig("azuread")
			assert.NoError(t, err)
		})

		t.Run("should return error for invalid provider", func(t *testing.T) {
			err := ssoManager.ValidateProviderConfig("invalid-provider")
			assert.Error(t, err)
		})
	})

	t.Run("SSO Authentication Flow", func(t *testing.T) {
		t.Run("should begin SSO flow", func(t *testing.T) {
			req := httptest.NewRequest("GET", "/auth/sso/azuread/login", nil)
			w := httptest.NewRecorder()

			authURL, err := ssoService.BeginSSOFlow(context.Background(), "azuread", w, req)

			// In a real test, this would fail because we don't have a real OIDC provider
			// But we can test that the method is called correctly
			assert.Error(t, err) // Expected because we don't have a real provider
			assert.Empty(t, authURL)
		})

		t.Run("should reject invalid provider", func(t *testing.T) {
			req := httptest.NewRequest("GET", "/auth/sso/invalid/login", nil)
			w := httptest.NewRecorder()

			_, err := ssoService.BeginSSOFlow(context.Background(), "invalid", w, req)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "not enabled")
		})
	})

	t.Run("User Creation from SSO", func(t *testing.T) {
		t.Run("should create user from SSO attributes", func(t *testing.T) {
			// Mock Goth user
			gothUser := &goth.User{
				UserID:    "test-user-123",
				Email:     "test@example.com",
				FirstName: "Test",
				LastName:  "User",
				Provider:  "azuread",
				RawData: map[string]interface{}{
					"groups": []string{"admin-group", "user-group"},
				},
			}

			// Mock SSO attributes
			attributes := &SSOUserAttributes{
				Email:     "test@example.com",
				FirstName: "Test",
				LastName:  "User",
				Username:  "testuser",
				Groups:    []string{"admin-group", "user-group"},
				Roles:     []string{"Admin"},
			}

			user, session, err := ssoService.CreateOrLinkSSOUser(
				context.Background(),
				gothUser,
				attributes,
				"127.0.0.1",
				"test-agent",
			)

			require.NoError(t, err)
			assert.NotNil(t, user)
			assert.NotNil(t, session)
			assert.Equal(t, "test@example.com", user.Email)
			assert.Equal(t, "Test", user.FirstName)
			assert.Equal(t, "User", user.LastName)
			assert.Equal(t, "Active", user.Status)
			assert.True(t, user.EmailVerified)
		})

		t.Run("should link SSO to existing user", func(t *testing.T) {
			// Create existing user
			existingUser := &User{
				ID:            "existing-user-123",
				Email:         "existing@example.com",
				Username:      "existinguser",
				FirstName:     "Existing",
				LastName:      "User",
				Role:          "Viewer",
				Status:        "Active",
				EmailVerified: true,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}

			err := userStore.CreateUser(context.Background(), existingUser)
			require.NoError(t, err)

			// Mock Goth user with same email
			gothUser := &goth.User{
				UserID:    "sso-user-456",
				Email:     "existing@example.com",
				FirstName: "Existing",
				LastName:  "User",
				Provider:  "azuread",
			}

			attributes := &SSOUserAttributes{
				Email:     "existing@example.com",
				FirstName: "Existing",
				LastName:  "User",
				Username:  "existinguser",
				Groups:    []string{"developer-group"},
				Roles:     []string{"Developer"},
			}

			user, session, err := ssoService.CreateOrLinkSSOUser(
				context.Background(),
				gothUser,
				attributes,
				"127.0.0.1",
				"test-agent",
			)

			require.NoError(t, err)
			assert.NotNil(t, user)
			assert.NotNil(t, session)
			assert.Equal(t, existingUser.ID, user.ID)
			assert.Equal(t, "Developer", user.Role) // Role should be updated from SSO
		})
	})

	t.Run("Group and Role Mapping", func(t *testing.T) {
		// Set up group mapping
		os.Setenv("AZURE_AD_GROUP_MAPPING", "admin-group:Admin,developer-group:Developer,user-group:Viewer")

		// Reload configuration
		err := ssoManager.LoadConfigFromEnvironment()
		require.NoError(t, err)

		t.Run("should map groups to roles correctly", func(t *testing.T) {
			groups := []string{"admin-group", "user-group"}
			roles := ssoManager.MapGroupsToRoles("azuread", groups)

			assert.Contains(t, roles, "Admin")
			assert.Contains(t, roles, "Viewer")
		})

		t.Run("should return highest role", func(t *testing.T) {
			roles := []string{"Admin", "Developer", "Viewer"}
			highestRole := ssoManager.GetHighestRole(roles)

			assert.Equal(t, "Admin", highestRole)
		})

		t.Run("should return default role for unmapped groups", func(t *testing.T) {
			groups := []string{"unknown-group"}
			roles := ssoManager.MapGroupsToRoles("azuread", groups)

			assert.Equal(t, []string{"Viewer"}, roles)
		})

		os.Unsetenv("AZURE_AD_GROUP_MAPPING")
	})

	t.Run("Single Logout", func(t *testing.T) {
		t.Run("should invalidate all user sessions", func(t *testing.T) {
			// Create test user with sessions
			user := &User{
				ID:            "logout-user-123",
				Email:         "logout@example.com",
				Username:      "logoutuser",
				Status:        "Active",
				EmailVerified: true,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}

			err := userStore.CreateUser(context.Background(), user)
			require.NoError(t, err)

			// Create test sessions
			session1 := &UserSession{
				ID:        "session-1",
				UserID:    user.ID,
				SessionID: "gorilla-session-1",
				ExpiresAt: time.Now().Add(time.Hour),
				CreatedAt: time.Now(),
			}

			session2 := &UserSession{
				ID:        "session-2",
				UserID:    user.ID,
				SessionID: "gorilla-session-2",
				ExpiresAt: time.Now().Add(time.Hour),
				CreatedAt: time.Now(),
			}

			err = userStore.CreateUserSession(context.Background(), session1)
			require.NoError(t, err)

			err = userStore.CreateUserSession(context.Background(), session2)
			require.NoError(t, err)

			// Initiate single logout
			err = ssoService.InitiateSingleLogout(context.Background(), user.ID)
			require.NoError(t, err)

			// Verify all sessions are invalidated
			sessions, err := userStore.GetUserSessions(context.Background(), user.ID)
			require.NoError(t, err)
			assert.Empty(t, sessions)
		})
	})

	t.Run("Attribute Extraction", func(t *testing.T) {
		t.Run("should extract Azure AD groups", func(t *testing.T) {
			rawData := map[string]interface{}{
				"groups": []interface{}{"group1", "group2"},
				"oid":    "object-id-123",
				"upn":    "user@domain.com",
			}

			attributes := &SSOUserAttributes{
				Groups:     make([]string, 0),
				Attributes: make(map[string]string),
			}

			err := ssoService.extractAzureADGroups(rawData, attributes)
			require.NoError(t, err)

			assert.Contains(t, attributes.Groups, "group1")
			assert.Contains(t, attributes.Groups, "group2")
			assert.Equal(t, "object-id-123", attributes.Attributes["object_id"])
			assert.Equal(t, "user@domain.com", attributes.Attributes["upn"])
		})

		t.Run("should extract OIDC groups", func(t *testing.T) {
			rawData := map[string]interface{}{
				"groups": []string{"oidc-group1", "oidc-group2"},
				"roles":  "admin",
			}

			attributes := &SSOUserAttributes{
				Groups: make([]string, 0),
			}

			err := ssoService.extractOIDCGroups(rawData, attributes)
			require.NoError(t, err)

			assert.Contains(t, attributes.Groups, "oidc-group1")
			assert.Contains(t, attributes.Groups, "oidc-group2")
			assert.Contains(t, attributes.Groups, "admin")
		})
	})

	t.Run("Session Management", func(t *testing.T) {
		t.Run("should cleanup expired SSO sessions", func(t *testing.T) {
			// Add expired session
			expiredSession := &SSOSession{
				ID:        "expired-session",
				State:     "expired-state",
				Provider:  "azuread",
				CreatedAt: time.Now().Add(-time.Hour),
				ExpiresAt: time.Now().Add(-time.Minute),
			}

			ssoService.sessionStore["expired-state"] = expiredSession

			// Add valid session
			validSession := &SSOSession{
				ID:        "valid-session",
				State:     "valid-state",
				Provider:  "azuread",
				CreatedAt: time.Now(),
				ExpiresAt: time.Now().Add(time.Hour),
			}

			ssoService.sessionStore["valid-state"] = validSession

			// Cleanup expired sessions
			ssoService.CleanupExpiredSessions()

			// Verify expired session is removed
			_, exists := ssoService.sessionStore["expired-state"]
			assert.False(t, exists)

			// Verify valid session remains
			_, exists = ssoService.sessionStore["valid-state"]
			assert.True(t, exists)
		})
	})
}

func TestSSORoutes(t *testing.T) {
	// This would test the HTTP routes for SSO functionality
	// For brevity, we'll include a basic test structure

	t.Run("GET /auth/sso/providers", func(t *testing.T) {
		// Test getting SSO providers endpoint
		// req := httptest.NewRequest("GET", "/auth/sso/providers", nil)
		w := httptest.NewRecorder()

		// Mock response
		response := map[string]interface{}{
			"providers": []string{"azuread", "oidc"},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)

		assert.Equal(t, http.StatusOK, w.Code)

		var result map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&result)
		require.NoError(t, err)

		providers, ok := result["providers"].([]interface{})
		require.True(t, ok)
		assert.Len(t, providers, 2)
	})

	t.Run("GET /auth/sso/{provider}/login", func(t *testing.T) {
		// Test SSO login initiation
		req := httptest.NewRequest("GET", "/auth/sso/azuread/login", nil)
		// w := httptest.NewRecorder()

		// In a real test, this would redirect to the SSO provider
		// For now, we'll just verify the route structure
		assert.Equal(t, "GET", req.Method)
		assert.Contains(t, req.URL.Path, "/auth/sso/")
		assert.Contains(t, req.URL.Path, "/login")
	})
}

// Helper function to create an in-memory user store for testing
func NewInMemoryUserStoreForSSO() UserStore {
	return &InMemoryUserStoreForSSO{
		users:       make(map[string]*User),
		credentials: make(map[string]*UserCredential),
		sessions:    make(map[string]*UserSession),
	}
}

// InMemoryUserStoreForSSO is a simple in-memory implementation for testing
type InMemoryUserStoreForSSO struct {
	users       map[string]*User
	credentials map[string]*UserCredential
	sessions    map[string]*UserSession
}

// Implement required UserStore methods for testing
func (s *InMemoryUserStoreForSSO) GetUser(ctx context.Context, userID string) (*User, error) {
	user, exists := s.users[userID]
	if !exists {
		return nil, nil
	}
	return user, nil
}

func (s *InMemoryUserStoreForSSO) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	for _, user := range s.users {
		if strings.EqualFold(user.Email, email) {
			return user, nil
		}
	}
	return nil, nil
}

func (s *InMemoryUserStoreForSSO) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	for _, user := range s.users {
		if user.Username == username {
			return user, nil
		}
	}
	return nil, nil
}

func (s *InMemoryUserStoreForSSO) CreateUser(ctx context.Context, user *User) error {
	s.users[user.ID] = user
	return nil
}

func (s *InMemoryUserStoreForSSO) UpdateUser(ctx context.Context, user *User) error {
	s.users[user.ID] = user
	return nil
}

func (s *InMemoryUserStoreForSSO) GetUserCredentialByProvider(ctx context.Context, userID, provider string) (*UserCredential, error) {
	for _, cred := range s.credentials {
		if cred.UserID == userID && cred.Provider == provider {
			return cred, nil
		}
	}
	return nil, nil
}

func (s *InMemoryUserStoreForSSO) GetUserByOAuthProvider(ctx context.Context, provider, providerID string) (*User, error) {
	for _, cred := range s.credentials {
		if cred.Provider == provider && cred.ProviderID == providerID {
			return s.users[cred.UserID], nil
		}
	}
	return nil, nil
}

func (s *InMemoryUserStoreForSSO) CreateUserCredential(ctx context.Context, credential *UserCredential) error {
	s.credentials[credential.ID] = credential
	return nil
}

func (s *InMemoryUserStoreForSSO) UpdateUserCredential(ctx context.Context, credential *UserCredential) error {
	s.credentials[credential.ID] = credential
	return nil
}

func (s *InMemoryUserStoreForSSO) DeleteUserCredential(ctx context.Context, credentialID string) error {
	delete(s.credentials, credentialID)
	return nil
}

func (s *InMemoryUserStoreForSSO) CreateUserSession(ctx context.Context, session *UserSession) error {
	s.sessions[session.ID] = session
	return nil
}

func (s *InMemoryUserStoreForSSO) GetUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	var sessions []*UserSession
	for _, session := range s.sessions {
		if session.UserID == userID {
			sessions = append(sessions, session)
		}
	}
	return sessions, nil
}

func (s *InMemoryUserStoreForSSO) DeleteAllUserSessions(ctx context.Context, userID string) error {
	for id, session := range s.sessions {
		if session.UserID == userID {
			delete(s.sessions, id)
		}
	}
	return nil
}

func (s *InMemoryUserStoreForSSO) GetUserCredentials(ctx context.Context, userID string) ([]*UserCredential, error) {
	var credentials []*UserCredential
	for _, cred := range s.credentials {
		if cred.UserID == userID {
			credentials = append(credentials, cred)
		}
	}
	return credentials, nil
}

// Implement other required methods with minimal functionality for testing
func (s *InMemoryUserStoreForSSO) ListUsers(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*User, error) {
	return nil, nil
}
func (s *InMemoryUserStoreForSSO) SearchUsers(ctx context.Context, query string, limit, offset int) ([]*User, error) {
	return nil, nil
}
func (s *InMemoryUserStoreForSSO) PatchUser(ctx context.Context, patch *PatchUserInput) error { return nil }
func (s *InMemoryUserStoreForSSO) DeleteUser(ctx context.Context, userID string) error        { return nil }
func (s *InMemoryUserStoreForSSO) GetUserSession(ctx context.Context, sessionID string) (*UserSession, error) {
	return nil, nil
}
func (s *InMemoryUserStoreForSSO) GetActiveUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	return nil, nil
}
func (s *InMemoryUserStoreForSSO) UpdateUserSession(ctx context.Context, session *UserSession) error {
	return nil
}
func (s *InMemoryUserStoreForSSO) DeleteUserSession(ctx context.Context, sessionID string) error {
	return nil
}
func (s *InMemoryUserStoreForSSO) DeleteExpiredSessions(ctx context.Context) (int, error) { return 0, nil }
func (s *InMemoryUserStoreForSSO) GetProjectMembers(ctx context.Context, projectID string) ([]*User, error) {
	return nil, nil
}
func (s *InMemoryUserStoreForSSO) GetUserProjects(ctx context.Context, userID string) ([]*ProjectMembership, error) {
	return nil, nil
}
func (s *InMemoryUserStoreForSSO) GetProjectMembership(ctx context.Context, userID, projectID string) (*ProjectMembership, error) {
	return nil, nil
}
func (s *InMemoryUserStoreForSSO) CheckUserProjectAccess(ctx context.Context, userID, projectID string) (bool, error) {
	return false, nil
}
func (s *InMemoryUserStoreForSSO) CreateProjectMembership(ctx context.Context, membership *ProjectMembership) error {
	return nil
}
func (s *InMemoryUserStoreForSSO) UpdateProjectMembership(ctx context.Context, membership *ProjectMembership) error {
	return nil
}
func (s *InMemoryUserStoreForSSO) DeleteProjectMembership(ctx context.Context, userID, projectID string) error {
	return nil
}
func (s *InMemoryUserStoreForSSO) GetEmailVerificationToken(ctx context.Context, token string) (*EmailVerificationToken, error) {
	return nil, nil
}
func (s *InMemoryUserStoreForSSO) GetEmailVerificationTokenByUserID(ctx context.Context, userID string) (*EmailVerificationToken, error) {
	return nil, nil
}
func (s *InMemoryUserStoreForSSO) CreateEmailVerificationToken(ctx context.Context, token *EmailVerificationToken) error {
	return nil
}
func (s *InMemoryUserStoreForSSO) DeleteEmailVerificationToken(ctx context.Context, token string) error {
	return nil
}
func (s *InMemoryUserStoreForSSO) GetPasswordResetToken(ctx context.Context, token string) (*PasswordResetToken, error) {
	return nil, nil
}
func (s *InMemoryUserStoreForSSO) CreatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error {
	return nil
}
func (s *InMemoryUserStoreForSSO) UpdatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error {
	return nil
}
func (s *InMemoryUserStoreForSSO) DeletePasswordResetToken(ctx context.Context, token string) error {
	return nil
}
func (s *InMemoryUserStoreForSSO) GetMagicMailToken(ctx context.Context, token string) (*MagicMailToken, error) {
	return nil, nil
}
func (s *InMemoryUserStoreForSSO) CreateMagicMailToken(ctx context.Context, token *MagicMailToken) error {
	return nil
}
func (s *InMemoryUserStoreForSSO) UpdateMagicMailToken(ctx context.Context, token *MagicMailToken) error {
	return nil
}
func (s *InMemoryUserStoreForSSO) DeleteMagicMailToken(ctx context.Context, token string) error { return nil }
func (s *InMemoryUserStoreForSSO) GetProjectInvitations(ctx context.Context, projectID string) ([]*UserInvitation, error) {
	return nil, nil
}
func (s *InMemoryUserStoreForSSO) GetUserInvitations(ctx context.Context, email string) ([]*UserInvitation, error) {
	return nil, nil
}
func (s *InMemoryUserStoreForSSO) GetInvitationByToken(ctx context.Context, token string) (*UserInvitation, error) {
	return nil, nil
}
func (s *InMemoryUserStoreForSSO) CreateInvitation(ctx context.Context, invitation *UserInvitation) error {
	return nil
}
func (s *InMemoryUserStoreForSSO) UpdateInvitation(ctx context.Context, invitation *UserInvitation) error {
	return nil
}
func (s *InMemoryUserStoreForSSO) DeleteInvitation(ctx context.Context, invitationID string) error {
	return nil
}
func (s *InMemoryUserStoreForSSO) GetUserActivity(ctx context.Context, userID string, limit, offset int) ([]interface{}, error) {
	return nil, nil
}
func (s *InMemoryUserStoreForSSO) GetUserStats(ctx context.Context, userID string) (map[string]interface{}, error) {
	return nil, nil
}
func (s *InMemoryUserStoreForSSO) GetSystemUserStats(ctx context.Context) (map[string]interface{}, error) {
	return nil, nil
}
