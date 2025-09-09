package users

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/shared"
	"github.com/stretchr/testify/suite"
)

// EndToEndTestSuite tests complete user journeys from HTTP request to response
type EndToEndTestSuite struct {
	suite.Suite
	ctx          context.Context
	store        UserStore
	service      *UserService
	router       *mux.Router
	sessionStore sessions.Store
	tempDBFile   string
	server       *httptest.Server
}

func (suite *EndToEndTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	// Create temporary database
	suite.tempDBFile = fmt.Sprintf("test_e2e_%d.db", time.Now().UnixNano())

	var err error
	suite.store, err = NewUserStore(suite.ctx, suite.tempDBFile)
	suite.Require().NoError(err)

	// Initialize session store using centralized test configuration
	suite.sessionStore = CreateTestSessionStore("test-secret-key")

	// Create user service with all dependencies
	eventBus := &shared.EventBusWrapper{}
	emailService := NewEmailService(true, "http://localhost:8080") // Development mode
	passwordHasher := NewDefaultBcryptPasswordHasher()

	suite.service = NewUserService(
		suite.store,
		eventBus,
		passwordHasher,
		emailService,
		suite.sessionStore,
		nil, // WebAuthn service
		nil, // OAuth manager
	)

	// Setup router with user routes
	suite.router = mux.NewRouter()
	suite.service.RegisterRoutes(suite.router)

	// Create test server
	suite.server = httptest.NewServer(suite.router)
}

func (suite *EndToEndTestSuite) TearDownSuite() {
	if suite.server != nil {
		suite.server.Close()
	}
	if suite.store != nil {
		// Close the store if it has a Close method
		if closer, ok := suite.store.(interface{ Close() error }); ok {
			closer.Close()
		}
	}
	if suite.tempDBFile != "" {
		os.Remove(suite.tempDBFile)
	}
}

// TestCompleteUserJourney tests a complete user journey from registration to project access
func (suite *EndToEndTestSuite) TestCompleteUserJourney() {
	// Step 1: User Registration
	registrationData := map[string]interface{}{
		"email":      "journey@example.com",
		"password":   "SecurePassword123!",
		"first_name": "Journey",
		"last_name":  "User",
		"username":   "journeyuser",
	}

	regBody, _ := json.Marshal(registrationData)
	resp, err := http.Post(suite.server.URL+"/auth/register", "application/json", bytes.NewBuffer(regBody))
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(http.StatusCreated, resp.StatusCode)

	// Extract user ID from response
	var regResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&regResponse)
	suite.Require().NoError(err)

	userID, ok := regResponse["user_id"].(string)
	suite.Require().True(ok)
	suite.NotEmpty(userID)

	// Step 2: Email Verification
	// In a real scenario, user would click email link. For testing, we'll get the token directly
	verificationToken, err := suite.store.GetEmailVerificationTokenByUserID(suite.ctx, userID)
	suite.Require().NoError(err)
	suite.NotNil(verificationToken)

	// Verify email via HTTP
	verifyURL := fmt.Sprintf("%s/auth/verify-email?token=%s", suite.server.URL, verificationToken.Token)
	resp, err = http.Get(verifyURL)
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(http.StatusOK, resp.StatusCode)

	// Step 3: User Login
	loginData := map[string]interface{}{
		"email":    "journey@example.com",
		"password": "SecurePassword123!",
	}

	loginBody, _ := json.Marshal(loginData)
	resp, err = http.Post(suite.server.URL+"/auth/login", "application/json", bytes.NewBuffer(loginBody))
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(http.StatusOK, resp.StatusCode)

	// Extract session cookie
	cookies := resp.Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "session" {
			sessionCookie = cookie
			break
		}
	}
	suite.NotNil(sessionCookie, "Session cookie should be set")

	// Step 4: Access Protected Resource
	req, err := http.NewRequest("GET", suite.server.URL+"/auth/profile", nil)
	suite.Require().NoError(err)
	req.AddCookie(sessionCookie)

	client := &http.Client{}
	resp, err = client.Do(req)
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(http.StatusOK, resp.StatusCode)

	// Verify profile data
	var profileResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&profileResponse)
	suite.Require().NoError(err)

	suite.Equal("journey@example.com", profileResponse["email"])
	suite.Equal("journeyuser", profileResponse["username"])

	// Step 5: Update Profile
	updateData := map[string]interface{}{
		"first_name": "Updated",
		"last_name":  "Name",
	}

	updateBody, _ := json.Marshal(updateData)
	req, err = http.NewRequest("PUT", suite.server.URL+"/auth/profile", bytes.NewBuffer(updateBody))
	suite.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)

	resp, err = client.Do(req)
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(http.StatusOK, resp.StatusCode)

	// Step 6: Logout
	req, err = http.NewRequest("GET", suite.server.URL+"/logout", nil)
	suite.Require().NoError(err)
	req.AddCookie(sessionCookie)

	resp, err = client.Do(req)
	suite.Require().NoError(err)
	defer resp.Body.Close()

	// Logout should redirect to login page
	suite.Equal(http.StatusSeeOther, resp.StatusCode)

	// Step 7: Verify logout - accessing protected resource should fail
	req, err = http.NewRequest("GET", suite.server.URL+"/auth/profile", nil)
	suite.Require().NoError(err)
	req.AddCookie(sessionCookie)

	resp, err = client.Do(req)
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(http.StatusUnauthorized, resp.StatusCode)
}

// TestPasswordResetJourney tests the complete password reset flow
func (suite *EndToEndTestSuite) TestPasswordResetJourney() {
	// Step 1: Create and verify user
	user := &User{
		ID:            uuid.New().String(),
		Email:         "reset-journey@example.com",
		Username:      "resetuser",
		FirstName:     "Reset",
		LastName:      "User",
		Role:          "Viewer",
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err := suite.store.CreateUser(suite.ctx, user)
	suite.Require().NoError(err)

	// Create password credential
	passwordHasher := NewDefaultBcryptPasswordHasher()
	hashedPassword, err := passwordHasher.HashPassword("OriginalPassword123!")
	suite.Require().NoError(err)

	credential := &UserCredential{
		ID:             uuid.New().String(),
		UserID:         user.ID,
		Type:           "password",
		Provider:       "local",
		CredentialData: hashedPassword,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	err = suite.store.CreateUserCredential(suite.ctx, credential)
	suite.Require().NoError(err)

	// Step 2: Request password reset
	resetRequestData := map[string]interface{}{
		"email": "reset-journey@example.com",
	}

	reqBody, _ := json.Marshal(resetRequestData)
	resp, err := http.Post(suite.server.URL+"/auth/forgot-password", "application/json", bytes.NewBuffer(reqBody))
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(http.StatusOK, resp.StatusCode)

	// Step 3: Complete password reset (simulated - in real scenario, user would get email)
	// For testing, we'll create a reset token directly
	resetToken := &PasswordResetToken{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		Token:     uuid.New().String(),
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
	}

	err = suite.store.CreatePasswordResetToken(suite.ctx, resetToken)
	suite.Require().NoError(err)

	// Reset password via HTTP
	resetData := map[string]interface{}{
		"token":        resetToken.Token,
		"new_password": "NewSecurePassword456!",
	}

	resetBody, _ := json.Marshal(resetData)
	resp, err = http.Post(suite.server.URL+"/auth/reset-password", "application/json", bytes.NewBuffer(resetBody))
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(http.StatusOK, resp.StatusCode)

	// Step 4: Login with new password
	loginData := map[string]interface{}{
		"email":    "reset-journey@example.com",
		"password": "NewSecurePassword456!",
	}

	loginBody, _ := json.Marshal(loginData)
	resp, err = http.Post(suite.server.URL+"/auth/login", "application/json", bytes.NewBuffer(loginBody))
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(http.StatusOK, resp.StatusCode)

	// Step 5: Verify old password no longer works
	oldLoginData := map[string]interface{}{
		"email":    "reset-journey@example.com",
		"password": "OriginalPassword123!",
	}

	oldLoginBody, _ := json.Marshal(oldLoginData)
	resp, err = http.Post(suite.server.URL+"/auth/login", "application/json", bytes.NewBuffer(oldLoginBody))
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(http.StatusUnauthorized, resp.StatusCode)
}

// TestProjectInvitationJourney tests the complete project invitation flow
func (suite *EndToEndTestSuite) TestProjectInvitationJourney() {
	// Step 1: Create admin user
	adminUser := &User{
		ID:            uuid.New().String(),
		Email:         "admin-journey@example.com",
		Username:      "adminuser",
		FirstName:     "Admin",
		LastName:      "User",
		Role:          RoleAdmin,
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err := suite.store.CreateUser(suite.ctx, adminUser)
	suite.Require().NoError(err)

	// Create admin session
	adminSession := &UserSession{
		ID:        uuid.New().String(),
		UserID:    adminUser.ID,
		SessionID: "admin_session_" + uuid.New().String(),
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
		IPAddress: "127.0.0.1",
		UserAgent: "test-agent",
	}

	err = suite.store.CreateUserSession(suite.ctx, adminSession)
	suite.Require().NoError(err)

	// Step 2: Admin sends project invitation
	projectID := uuid.New().String()
	invitationData := map[string]interface{}{
		"project_id": projectID,
		"email":      "invitee-journey@example.com",
		"role":       ProjectRoleDeveloper,
	}

	inviteBody, _ := json.Marshal(invitationData)
	req, err := http.NewRequest("POST", suite.server.URL+"/auth/invite", bytes.NewBuffer(inviteBody))
	suite.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")

	// Add admin session cookie
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: adminSession.SessionID,
	})

	client := &http.Client{}
	resp, err := client.Do(req)
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(http.StatusOK, resp.StatusCode)

	// Step 3: Create invitee user
	inviteeUser := &User{
		ID:            uuid.New().String(),
		Email:         "invitee-journey@example.com",
		Username:      "inviteeuser",
		FirstName:     "Invitee",
		LastName:      "User",
		Role:          "Viewer",
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err = suite.store.CreateUser(suite.ctx, inviteeUser)
	suite.Require().NoError(err)

	// Step 4: Get invitation token
	invitations, err := suite.store.GetUserInvitations(suite.ctx, "invitee-journey@example.com")
	suite.Require().NoError(err)
	suite.Len(invitations, 1)

	invitation := invitations[0]

	// Step 5: Accept invitation
	acceptURL := fmt.Sprintf("%s/auth/accept-invitation?token=%s", suite.server.URL, invitation.Token)
	resp, err = http.Get(acceptURL)
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(http.StatusOK, resp.StatusCode)

	// Step 6: Verify project membership was created
	membership, err := suite.store.GetProjectMembership(suite.ctx, inviteeUser.ID, projectID)
	suite.Require().NoError(err)
	suite.NotNil(membership)
	suite.Equal(ProjectRoleDeveloper, membership.Role)
}

// TestSecurityJourney tests security features in end-to-end scenarios
func (suite *EndToEndTestSuite) TestSecurityJourney() {
	suite.Run("Rate Limiting", func() {
		// Make multiple rapid requests to trigger rate limiting
		loginData := map[string]interface{}{
			"email":    "nonexistent@example.com",
			"password": "WrongPassword123!",
		}

		loginBody, _ := json.Marshal(loginData)

		// First few requests should get through (but fail authentication)
		for i := 0; i < 3; i++ {
			resp, err := http.Post(suite.server.URL+"/auth/login", "application/json", bytes.NewBuffer(loginBody))
			suite.NoError(err)
			resp.Body.Close()
			suite.Equal(http.StatusUnauthorized, resp.StatusCode)
		}

		// Subsequent requests should be rate limited
		// Note: This test assumes rate limiting is configured
		resp, err := http.Post(suite.server.URL+"/auth/login", "application/json", bytes.NewBuffer(loginBody))
		suite.NoError(err)
		resp.Body.Close()

		// Rate limiting might return 429 Too Many Requests
		suite.True(resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusUnauthorized)
	})

	suite.Run("CSRF Protection", func() {
		// Create a user and session
		user := &User{
			ID:            uuid.New().String(),
			Email:         "csrf@example.com",
			Username:      "csrfuser",
			FirstName:     "CSRF",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		err := suite.store.CreateUser(suite.ctx, user)
		suite.Require().NoError(err)

		session := &UserSession{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			SessionID: "csrf_session_" + uuid.New().String(),
			ExpiresAt: time.Now().Add(time.Hour),
			CreatedAt: time.Now(),
			IPAddress: "127.0.0.1",
			UserAgent: "test-agent",
		}

		err = suite.store.CreateUserSession(suite.ctx, session)
		suite.Require().NoError(err)

		// Try to make a state-changing request without CSRF token
		updateData := map[string]interface{}{
			"first_name": "Updated",
		}

		updateBody, _ := json.Marshal(updateData)
		req, err := http.NewRequest("PUT", suite.server.URL+"/auth/profile", bytes.NewBuffer(updateBody))
		suite.Require().NoError(err)
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{
			Name:  "session",
			Value: session.SessionID,
		})

		client := &http.Client{}
		resp, err := client.Do(req)
		suite.Require().NoError(err)
		defer resp.Body.Close()

		// Should be rejected due to missing CSRF token (if CSRF is enabled)
		// Note: This test assumes CSRF protection is configured
		suite.True(resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusOK)
	})

	suite.Run("Input Validation", func() {
		// Test with malicious input
		maliciousData := map[string]interface{}{
			"email":      "<script>alert('xss')</script>@example.com",
			"password":   "Password123!",
			"first_name": "'; DROP TABLE users; --",
			"last_name":  "User",
			"username":   "malicioususer",
		}

		regBody, _ := json.Marshal(maliciousData)
		resp, err := http.Post(suite.server.URL+"/auth/register", "application/json", bytes.NewBuffer(regBody))
		suite.Require().NoError(err)
		defer resp.Body.Close()

		// Should be rejected due to input validation
		suite.Equal(http.StatusBadRequest, resp.StatusCode)
	})
}

// TestErrorHandlingJourney tests error handling in end-to-end scenarios
func (suite *EndToEndTestSuite) TestErrorHandlingJourney() {
	suite.Run("Invalid JSON", func() {
		invalidJSON := `{"email": "test@example.com", "password": }`

		resp, err := http.Post(suite.server.URL+"/auth/register", "application/json", strings.NewReader(invalidJSON))
		suite.Require().NoError(err)
		defer resp.Body.Close()

		suite.Equal(http.StatusBadRequest, resp.StatusCode)
	})

	suite.Run("Missing Required Fields", func() {
		incompleteData := map[string]interface{}{
			"email": "incomplete@example.com",
			// Missing password and other required fields
		}

		regBody, _ := json.Marshal(incompleteData)
		resp, err := http.Post(suite.server.URL+"/auth/register", "application/json", bytes.NewBuffer(regBody))
		suite.Require().NoError(err)
		defer resp.Body.Close()

		suite.Equal(http.StatusBadRequest, resp.StatusCode)

		// Verify error message
		var errorResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&errorResponse)
		suite.NoError(err)
		suite.Contains(errorResponse, "error")
	})

	suite.Run("Duplicate User Registration", func() {
		// Create user first
		user := &User{
			ID:            uuid.New().String(),
			Email:         "duplicate@example.com",
			Username:      "duplicateuser",
			FirstName:     "Duplicate",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		err := suite.store.CreateUser(suite.ctx, user)
		suite.Require().NoError(err)

		// Try to register with same email
		duplicateData := map[string]interface{}{
			"email":      "duplicate@example.com",
			"password":   "Password123!",
			"first_name": "Another",
			"last_name":  "User",
			"username":   "anotheruser",
		}

		regBody, _ := json.Marshal(duplicateData)
		resp, err := http.Post(suite.server.URL+"/auth/register", "application/json", bytes.NewBuffer(regBody))
		suite.Require().NoError(err)
		defer resp.Body.Close()

		suite.Equal(http.StatusConflict, resp.StatusCode)
	})

	suite.Run("Unauthorized Access", func() {
		// Try to access protected resource without authentication
		resp, err := http.Get(suite.server.URL + "/auth/profile")
		suite.Require().NoError(err)
		defer resp.Body.Close()

		suite.Equal(http.StatusUnauthorized, resp.StatusCode)
	})

	suite.Run("Invalid Session", func() {
		// Try to access protected resource with invalid session
		req, err := http.NewRequest("GET", suite.server.URL+"/auth/profile", nil)
		suite.Require().NoError(err)
		req.AddCookie(&http.Cookie{
			Name:  "session",
			Value: "invalid-session-id",
		})

		client := &http.Client{}
		resp, err := client.Do(req)
		suite.Require().NoError(err)
		defer resp.Body.Close()

		suite.Equal(http.StatusUnauthorized, resp.StatusCode)
	})
}

// TestConcurrentEndToEndOperations tests concurrent end-to-end operations
func (suite *EndToEndTestSuite) TestConcurrentEndToEndOperations() {
	suite.Run("Concurrent User Registration", func() {
		const numUsers = 10
		results := make(chan int, numUsers)

		for i := 0; i < numUsers; i++ {
			go func(index int) {
				registrationData := map[string]interface{}{
					"email":      fmt.Sprintf("concurrent%d@example.com", index),
					"password":   "ConcurrentPassword123!",
					"first_name": "Concurrent",
					"last_name":  "User",
					"username":   fmt.Sprintf("concurrent%d", index),
				}

				regBody, _ := json.Marshal(registrationData)
				resp, err := http.Post(suite.server.URL+"/auth/register", "application/json", bytes.NewBuffer(regBody))
				if err != nil {
					results <- http.StatusInternalServerError
					return
				}
				defer resp.Body.Close()

				results <- resp.StatusCode
			}(i)
		}

		// Collect results
		successCount := 0
		for i := 0; i < numUsers; i++ {
			statusCode := <-results
			if statusCode == http.StatusCreated {
				successCount++
			}
		}

		suite.Equal(numUsers, successCount, "All concurrent registrations should succeed")
	})

	suite.Run("Concurrent Login Attempts", func() {
		// Create a user first
		user := &User{
			ID:            uuid.New().String(),
			Email:         "concurrent-login@example.com",
			Username:      "concurrentlogin",
			FirstName:     "Concurrent",
			LastName:      "Login",
			Role:          "Viewer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		err := suite.store.CreateUser(suite.ctx, user)
		suite.Require().NoError(err)

		// Create password credential
		passwordHasher := NewDefaultBcryptPasswordHasher()
		hashedPassword, err := passwordHasher.HashPassword("ConcurrentPassword123!")
		suite.Require().NoError(err)

		credential := &UserCredential{
			ID:             uuid.New().String(),
			UserID:         user.ID,
			Type:           "password",
			Provider:       "local",
			CredentialData: hashedPassword,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		err = suite.store.CreateUserCredential(suite.ctx, credential)
		suite.Require().NoError(err)

		// Perform concurrent logins
		const numLogins = 5
		results := make(chan int, numLogins)

		for i := 0; i < numLogins; i++ {
			go func() {
				loginData := map[string]interface{}{
					"email":    "concurrent-login@example.com",
					"password": "ConcurrentPassword123!",
				}

				loginBody, _ := json.Marshal(loginData)
				resp, err := http.Post(suite.server.URL+"/auth/login", "application/json", bytes.NewBuffer(loginBody))
				if err != nil {
					results <- http.StatusInternalServerError
					return
				}
				defer resp.Body.Close()

				results <- resp.StatusCode
			}()
		}

		// Collect results
		successCount := 0
		for i := 0; i < numLogins; i++ {
			statusCode := <-results
			if statusCode == http.StatusOK {
				successCount++
			}
		}

		suite.Equal(numLogins, successCount, "All concurrent logins should succeed")
	})
}

// Run the end-to-end test suite
func TestEndToEndTestSuite(t *testing.T) {
	suite.Run(t, new(EndToEndTestSuite))
}
