package users

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/mbolli/tickit/internal/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdministrativeIntegration(t *testing.T) {
	ctx := context.Background()

	// Create test store
	store, err := NewUserStore(ctx, ":memory:")
	require.NoError(t, err)

	// Create mock services
	commandBus := &shared.CommandBus{}
	queryBus := &shared.QueryBus{}
	userService := NewUserService(commandBus, queryBus)
	sessionStore := CreateTestSessionStore("test-key")

	// Create mock auth service
	mockAuthService := &MockAuthService{}
	mockWebAuthnService := &MockWebAuthnService{}

	// Create routes
	userRoutes := NewUserRoutes(userService, sessionStore, mockAuthService, mockWebAuthnService)

	// Create test users
	testUsers := []*User{
		{
			ID:            "admin-1",
			Email:         "admin@test.com",
			Username:      "admin",
			FirstName:     "Admin",
			LastName:      "User",
			Role:          "Admin",
			Status:        "Active",
			EmailVerified: true,
		},
		{
			ID:            "user-1",
			Email:         "user1@test.com",
			Username:      "user1",
			FirstName:     "Test",
			LastName:      "User1",
			Role:          "Developer",
			Status:        "Active",
			EmailVerified: true,
		},
	}

	// Create users in store
	for _, user := range testUsers {
		err := store.CreateUser(ctx, user)
		require.NoError(t, err)
	}

	t.Run("ListUsersEndpoint", func(t *testing.T) {
		// Create router
		router := mux.NewRouter()
		userRoutes.RegisterRoutes(router)

		// Create request
		req, err := http.NewRequest("GET", "/api/admin/users", nil)
		require.NoError(t, err)

		// Create response recorder
		rr := httptest.NewRecorder()

		// Serve request
		router.ServeHTTP(rr, req)

		// Check response
		assert.Equal(t, http.StatusOK, rr.Code)

		// Parse response
		var response map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify response structure
		assert.Contains(t, response, "users")
		assert.Contains(t, response, "limit")
		assert.Contains(t, response, "offset")
	})

	t.Run("GetUserEndpoint", func(t *testing.T) {
		// Create router
		router := mux.NewRouter()
		userRoutes.RegisterRoutes(router)

		// Create request
		req, err := http.NewRequest("GET", "/api/admin/users/user-1", nil)
		require.NoError(t, err)

		// Create response recorder
		rr := httptest.NewRecorder()

		// Serve request
		router.ServeHTTP(rr, req)

		// Check response
		assert.Equal(t, http.StatusOK, rr.Code)

		// Parse response
		var response map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify response structure
		assert.Contains(t, response, "user")
		assert.Contains(t, response, "stats")
		assert.Contains(t, response, "activity")
	})

	t.Run("DeactivateUserEndpoint", func(t *testing.T) {
		// Create router
		router := mux.NewRouter()
		userRoutes.RegisterRoutes(router)

		// Create request
		req, err := http.NewRequest("POST", "/api/admin/users/user-1/deactivate", nil)
		require.NoError(t, err)

		// Create response recorder
		rr := httptest.NewRecorder()

		// Serve request
		router.ServeHTTP(rr, req)

		// Check response
		assert.Equal(t, http.StatusOK, rr.Code)

		// Parse response
		var response map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify response
		assert.Equal(t, true, response["success"])
		assert.Contains(t, response, "message")
	})

	t.Run("AssignRoleEndpoint", func(t *testing.T) {
		// Create router
		router := mux.NewRouter()
		userRoutes.RegisterRoutes(router)

		// Create request body
		roleRequest := map[string]string{"role": "ProjectManager"}
		body, err := json.Marshal(roleRequest)
		require.NoError(t, err)

		// Create request
		req, err := http.NewRequest("PUT", "/api/admin/users/user-1/role", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		// Create response recorder
		rr := httptest.NewRecorder()

		// Serve request
		router.ServeHTTP(rr, req)

		// Check response
		assert.Equal(t, http.StatusOK, rr.Code)

		// Parse response
		var response map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify response
		assert.Equal(t, true, response["success"])
		assert.Contains(t, response, "message")
	})
}

// Mock services for testing
type MockAuthService struct{}

func (m *MockAuthService) RegisterUser(ctx context.Context, email, password string) error {
	return nil
}

func (m *MockAuthService) LoginUser(ctx context.Context, email, password string) (*User, error) {
	return nil, nil
}

func (m *MockAuthService) LogoutUser(ctx context.Context, sessionID string) error {
	return nil
}

func (m *MockAuthService) VerifyEmail(ctx context.Context, token string) error {
	return nil
}

func (m *MockAuthService) ForgotPassword(ctx context.Context, email string) error {
	return nil
}

func (m *MockAuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	return nil
}

func (m *MockAuthService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	return nil
}

func (m *MockAuthService) SendMagicMail(ctx context.Context, email string) error {
	return nil
}

func (m *MockAuthService) LoginMagicMail(ctx context.Context, token, ipAddress, userAgent string) (*User, *UserSession, error) {
	return nil, nil, nil
}

func (m *MockAuthService) BeginMFARegistration(ctx context.Context, userID string) error {
	return nil
}

func (m *MockAuthService) FinishMFARegistration(ctx context.Context, userID string, response []byte) error {
	return nil
}

func (m *MockAuthService) LoginWebAuthn(ctx context.Context, userID string, assertion []byte, ipAddress, userAgent string) (*User, *UserSession, error) {
	return nil, nil, nil
}

func (m *MockAuthService) CheckUserMFAStatus(ctx context.Context, userID string) (bool, error) {
	return false, nil
}

type MockWebAuthnService struct{}

func (m *MockWebAuthnService) BeginRegistration(ctx context.Context, userID string) ([]byte, error) {
	return []byte("{}"), nil
}

func (m *MockWebAuthnService) FinishRegistration(ctx context.Context, userID string, response []byte) error {
	return nil
}

func (m *MockWebAuthnService) BeginLogin(ctx context.Context, userID string) ([]byte, error) {
	return []byte("{}"), nil
}

func (m *MockWebAuthnService) FinishLogin(ctx context.Context, userID string, response []byte) error {
	return nil
}
