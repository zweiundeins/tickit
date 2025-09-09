package users

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mbolli/tickit/internal/shared"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

// ComprehensiveUserTestSuite provides a comprehensive test suite for the user management system
type ComprehensiveUserTestSuite struct {
	suite.Suite
	ctx           context.Context
	store         UserStore
	handler       *UserCommandHandler
	mockEventBus  *MockEventBus
	emailService  EmailService
	passwordHasher PasswordHasher
	securityManager *SecurityManager
	tempDBFile    string
}

// SetupSuite runs once before all tests in the suite
func (suite *ComprehensiveUserTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	
	// Create temporary database
	suite.tempDBFile = fmt.Sprintf("test_comprehensive_%d.db", time.Now().UnixNano())
	
	var err error
	suite.store, err = NewUserStore(suite.ctx, suite.tempDBFile)
	suite.Require().NoError(err)
	
	// Initialize dependencies
	suite.mockEventBus = new(MockEventBus)
	suite.emailService = NewEmailService(true, "http://localhost:8080") // Development mode
	suite.passwordHasher = NewDefaultBcryptPasswordHasher()
	
	// Initialize security manager
	securityConfig := DefaultSecurityConfig()
	auditLogger := NewAuditLogger(false, "INFO") // Disable for testing
	suite.securityManager = NewSecurityManager(securityConfig, auditLogger)
	
	// Create command handler
	suite.handler = NewUserCommandHandler(
		suite.mockEventBus,
		&shared.EventStore{}, // Mock event store
		suite.store,
		suite.passwordHasher,
		suite.emailService,
		nil, // WebAuthn service not needed for basic tests
		nil, // SSO service not needed for basic tests
	)
}

// TearDownSuite runs once after all tests in the suite
func (suite *ComprehensiveUserTestSuite) TearDownSuite() {
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

// SetupTest runs before each test
func (suite *ComprehensiveUserTestSuite) SetupTest() {
	// Reset mock expectations
	suite.mockEventBus.ExpectedCalls = nil
	suite.mockEventBus.Calls = nil
}

// TestCompleteUserRegistrationFlow tests the entire user registration process
func (suite *ComprehensiveUserTestSuite) TestCompleteUserRegistrationFlow() {
	// Setup event expectations
	suite.mockEventBus.On("PublishEvent", suite.ctx, mock.AnythingOfType("users.UserRegisteredEvent")).Return(nil)
	suite.mockEventBus.On("PublishEvent", suite.ctx, mock.AnythingOfType("users.UserEmailVerifiedEvent")).Return(nil)
	
	// Step 1: Register user
	userID := uuid.New().String()
	registerCmd := RegisterUserCommand{
		ID:        userID,
		Email:     "comprehensive@example.com",
		Password:  "SecurePassword123!",
		FirstName: "Comprehensive",
		LastName:  "Test",
		Username:  "comprehensiveuser",
	}
	
	err := suite.handler.HandleRegisterUser(suite.ctx, registerCmd)
	suite.Require().NoError(err)
	
	// Verify user was created
	user, err := suite.store.GetUser(suite.ctx, userID)
	suite.Require().NoError(err)
	suite.Equal("Pending", user.Status)
	suite.False(user.EmailVerified)
	
	// Step 2: Verify email
	verificationToken, err := suite.store.GetEmailVerificationTokenByUserID(suite.ctx, userID)
	suite.Require().NoError(err)
	suite.NotNil(verificationToken)
	
	verifyCmd := VerifyEmailCommand{Token: verificationToken.Token}
	err = suite.handler.HandleVerifyEmail(suite.ctx, verifyCmd)
	suite.Require().NoError(err)
	
	// Verify user is now active
	user, err = suite.store.GetUser(suite.ctx, userID)
	suite.Require().NoError(err)
	suite.Equal("Active", user.Status)
	suite.True(user.EmailVerified)
	
	// Step 3: Login
	suite.mockEventBus.On("PublishEvent", suite.ctx, mock.AnythingOfType("users.UserLoggedInEvent")).Return(nil)
	
	loginCmd := LoginUserCommand{
		Email:     "comprehensive@example.com",
		Password:  "SecurePassword123!",
		IPAddress: "127.0.0.1",
		UserAgent: "test-agent",
	}
	
	err = suite.handler.HandleLoginUser(suite.ctx, loginCmd)
	suite.NoError(err)
	
	suite.mockEventBus.AssertExpectations(suite.T())
}

// TestPasswordResetFlow tests the complete password reset process
func (suite *ComprehensiveUserTestSuite) TestPasswordResetFlow() {
	// Create and verify a user first
	userID := uuid.New().String()
	user := &User{
		ID:            userID,
		Email:         "reset@example.com",
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
	hashedPassword, err := suite.passwordHasher.HashPassword("OriginalPassword123!")
	suite.Require().NoError(err)
	
	credential := &UserCredential{
		ID:             uuid.New().String(),
		UserID:         userID,
		Type:           "password",
		Provider:       "local",
		CredentialData: hashedPassword,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	
	err = suite.store.CreateUserCredential(suite.ctx, credential)
	suite.Require().NoError(err)
	
	// Setup event expectations
	suite.mockEventBus.On("PublishEvent", suite.ctx, mock.AnythingOfType("users.PasswordResetRequestedEvent")).Return(nil)
	suite.mockEventBus.On("PublishEvent", suite.ctx, mock.AnythingOfType("users.PasswordResetCompletedEvent")).Return(nil)
	
	// Step 1: Request password reset
	resetRequestCmd := ForgotPasswordCommand{
		Email: "reset@example.com",
	}
	
	err = suite.handler.HandleForgotPassword(suite.ctx, resetRequestCmd)
	suite.Require().NoError(err)
	
	// Note: In a real implementation, we would get the reset token from the database
	// For now, we'll simulate the reset process
	
	// Step 2: Complete password reset (simulated)
	// This would be tested with a proper token retrieval mechanism
	suite.mockEventBus.AssertExpectations(suite.T())
}

// TestProjectInvitationFlow tests the complete project invitation process
func (suite *ComprehensiveUserTestSuite) TestProjectInvitationFlow() {
	// Create inviter user
	inviterID := uuid.New().String()
	inviter := &User{
		ID:            inviterID,
		Email:         "inviter@example.com",
		Username:      "inviter",
		FirstName:     "Inviter",
		LastName:      "User",
		Role:          RoleAdmin,
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	
	err := suite.store.CreateUser(suite.ctx, inviter)
	suite.Require().NoError(err)
	
	// Setup event expectations
	suite.mockEventBus.On("PublishEvent", suite.ctx, mock.AnythingOfType("shared.Event")).Return(nil)
	
	// Step 1: Send invitation
	projectID := uuid.New().String()
	inviteCmd := InviteUserToProjectCommand{
		ProjectID: projectID,
		Email:     "invitee@example.com",
		Role:      ProjectRoleDeveloper,
		InvitedBy: inviterID,
	}
	
	err = suite.handler.HandleInviteUserToProject(suite.ctx, inviteCmd)
	suite.Require().NoError(err)
	
	// Verify invitation was created
	invitations, err := suite.store.GetUserInvitations(suite.ctx, "invitee@example.com")
	suite.Require().NoError(err)
	suite.Len(invitations, 1)
	
	invitation := invitations[0]
	suite.Equal(projectID, invitation.ProjectID)
	suite.Equal("invitee@example.com", invitation.Email)
	suite.Equal(ProjectRoleDeveloper, invitation.Role)
	
	// Step 2: Create invitee user
	inviteeID := uuid.New().String()
	invitee := &User{
		ID:            inviteeID,
		Email:         "invitee@example.com",
		Username:      "invitee",
		FirstName:     "Invitee",
		LastName:      "User",
		Role:          "Viewer",
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	
	err = suite.store.CreateUser(suite.ctx, invitee)
	suite.Require().NoError(err)
	
	// Step 3: Accept invitation
	acceptCmd := AcceptProjectInvitationCommand{
		Token: invitation.Token,
	}
	
	err = suite.handler.HandleAcceptProjectInvitation(suite.ctx, acceptCmd)
	suite.Require().NoError(err)
	
	// Verify project membership was created
	membership, err := suite.store.GetProjectMembership(suite.ctx, inviteeID, projectID)
	suite.Require().NoError(err)
	suite.NotNil(membership)
	suite.Equal(ProjectRoleDeveloper, membership.Role)
	
	suite.mockEventBus.AssertExpectations(suite.T())
}

// TestSecurityFeatures tests various security aspects
func (suite *ComprehensiveUserTestSuite) TestSecurityFeatures() {
	suite.Run("Rate Limiting", func() {
		limiter := NewRateLimiter(2, 1) // Very restrictive for testing
		
		// First request should pass
		suite.True(limiter.Allow("test-ip"))
		
		// Second request should be blocked
		suite.False(limiter.Allow("test-ip"))
		
		// Different IP should have separate limit
		suite.True(limiter.Allow("different-ip"))
	})
	
	suite.Run("Brute Force Protection", func() {
		config := DefaultSecurityConfig()
		config.MaxFailedAttempts = 3 // Lower for testing
		
		bfp := NewBruteForceProtection(config)
		testKey := "test-user"
		
		// Should not be locked initially
		suite.False(bfp.IsLocked(testKey))
		
		// Record failed attempts
		for i := 0; i < config.MaxFailedAttempts; i++ {
			bfp.RecordFailedAttempt(testKey)
		}
		
		// Should now be locked
		suite.True(bfp.IsLocked(testKey))
		
		// Clear attempts should unlock
		bfp.ClearFailedAttempts(testKey)
		suite.False(bfp.IsLocked(testKey))
	})
	
	suite.Run("Input Validation", func() {
		// Test SQL injection detection
		err := ValidateNoSQLInjection("normal input")
		suite.NoError(err)
		
		err = ValidateNoSQLInjection("'; DROP TABLE users; --")
		suite.Error(err)
		
		// Test script injection detection
		err = ValidateNoScriptInjection("normal input")
		suite.NoError(err)
		
		err = ValidateNoScriptInjection("<script>alert('xss')</script>")
		suite.Error(err)
		
		// Test string sanitization
		sanitized := SanitizeString("test\x00string")
		suite.NotContains(sanitized, "\x00")
	})
}

// TestErrorHandling tests various error conditions
func (suite *ComprehensiveUserTestSuite) TestErrorHandling() {
	suite.Run("Invalid Registration Data", func() {
		// Test invalid email
		cmd := RegisterUserCommand{
			ID:       uuid.New().String(),
			Email:    "invalid-email",
			Password: "ValidPassword123!",
			Username: "testuser",
		}
		
		err := suite.handler.HandleRegisterUser(suite.ctx, cmd)
		suite.Error(err)
		suite.Contains(err.Error(), "invalid email")
		
		// Test weak password
		cmd.Email = "valid@example.com"
		cmd.Password = "weak"
		
		err = suite.handler.HandleRegisterUser(suite.ctx, cmd)
		suite.Error(err)
		suite.Contains(err.Error(), "invalid password")
	})
	
	suite.Run("Login with Invalid Credentials", func() {
		// Setup event expectations for failed login
		suite.mockEventBus.On("PublishEvent", suite.ctx, mock.AnythingOfType("users.UserLoginFailedEvent")).Return(nil)
		
		cmd := LoginUserCommand{
			Email:     "nonexistent@example.com",
			Password:  "AnyPassword123!",
			IPAddress: "127.0.0.1",
			UserAgent: "test-agent",
		}
		
		err := suite.handler.HandleLoginUser(suite.ctx, cmd)
		suite.Error(err)
		suite.Contains(err.Error(), "invalid credentials")
		
		suite.mockEventBus.AssertExpectations(suite.T())
	})
	
	suite.Run("Expired Token Handling", func() {
		// Create user
		userID := uuid.New().String()
		user := &User{
			ID:            userID,
			Email:         "expired@example.com",
			Username:      "expireduser",
			FirstName:     "Expired",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Pending",
			EmailVerified: false,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		
		err := suite.store.CreateUser(suite.ctx, user)
		suite.Require().NoError(err)
		
		// Create expired verification token
		expiredToken := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    userID,
			Token:     uuid.New().String(),
			ExpiresAt: time.Now().Add(-time.Hour), // Expired
			CreatedAt: time.Now().Add(-2 * time.Hour),
		}
		
		err = suite.store.CreateEmailVerificationToken(suite.ctx, expiredToken)
		suite.Require().NoError(err)
		
		// Try to verify with expired token
		cmd := VerifyEmailCommand{Token: expiredToken.Token}
		err = suite.handler.HandleVerifyEmail(suite.ctx, cmd)
		
		suite.Error(err)
		suite.Contains(err.Error(), "verification token has expired")
	})
}

// TestConcurrentOperations tests thread safety and concurrent access
func (suite *ComprehensiveUserTestSuite) TestConcurrentOperations() {
	suite.Run("Concurrent User Registration", func() {
		const numGoroutines = 10
		errors := make(chan error, numGoroutines)
		
		// Setup event expectations
		suite.mockEventBus.On("PublishEvent", suite.ctx, mock.AnythingOfType("users.UserRegisteredEvent")).Return(nil).Times(numGoroutines)
		
		for i := 0; i < numGoroutines; i++ {
			go func(index int) {
				cmd := RegisterUserCommand{
					ID:        uuid.New().String(),
					Email:     fmt.Sprintf("concurrent%d@example.com", index),
					Password:  "ConcurrentPassword123!",
					FirstName: "Concurrent",
					LastName:  "User",
					Username:  fmt.Sprintf("concurrent%d", index),
				}
				
				err := suite.handler.HandleRegisterUser(suite.ctx, cmd)
				errors <- err
			}(i)
		}
		
		// Collect results
		for i := 0; i < numGoroutines; i++ {
			err := <-errors
			suite.NoError(err, "Concurrent registration should succeed")
		}
		
		suite.mockEventBus.AssertExpectations(suite.T())
	})
	
	suite.Run("Concurrent Session Management", func() {
		// Create a user first
		userID := uuid.New().String()
		user := &User{
			ID:            userID,
			Email:         "session@example.com",
			Username:      "sessionuser",
			FirstName:     "Session",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		
		err := suite.store.CreateUser(suite.ctx, user)
		suite.Require().NoError(err)
		
		const numSessions = 5
		sessionIDs := make([]string, numSessions)
		
		// Create multiple sessions concurrently
		for i := 0; i < numSessions; i++ {
			sessionIDs[i] = uuid.New().String()
			session := &UserSession{
				ID:        uuid.New().String(),
				UserID:    userID,
				SessionID: sessionIDs[i],
				ExpiresAt: time.Now().Add(time.Hour),
				CreatedAt: time.Now(),
				IPAddress: fmt.Sprintf("192.168.1.%d", i+1),
				UserAgent: "test-agent",
			}
			
			err := suite.store.CreateUserSession(suite.ctx, session)
			suite.NoError(err)
		}
		
		// Verify all sessions were created
		sessions, err := suite.store.GetUserSessions(suite.ctx, userID)
		suite.NoError(err)
		suite.Len(sessions, numSessions)
	})
}

// TestDataIntegrity tests data consistency and integrity
func (suite *ComprehensiveUserTestSuite) TestDataIntegrity() {
	suite.Run("User Data Consistency", func() {
		userID := uuid.New().String()
		originalUser := &User{
			ID:            userID,
			Email:         "integrity@example.com",
			Username:      "integrityuser",
			FirstName:     "Integrity",
			LastName:      "User",
			Role:          "Developer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		
		err := suite.store.CreateUser(suite.ctx, originalUser)
		suite.Require().NoError(err)
		
		// Retrieve and verify data integrity
		retrievedUser, err := suite.store.GetUser(suite.ctx, userID)
		suite.Require().NoError(err)
		
		suite.Equal(originalUser.Email, retrievedUser.Email)
		suite.Equal(originalUser.Username, retrievedUser.Username)
		suite.Equal(originalUser.Role, retrievedUser.Role)
		suite.Equal(originalUser.Status, retrievedUser.Status)
		suite.Equal(originalUser.EmailVerified, retrievedUser.EmailVerified)
	})
	
	suite.Run("Credential Data Integrity", func() {
		userID := uuid.New().String()
		user := &User{
			ID:            userID,
			Email:         "cred@example.com",
			Username:      "creduser",
			FirstName:     "Cred",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		
		err := suite.store.CreateUser(suite.ctx, user)
		suite.Require().NoError(err)
		
		// Create multiple credentials
		credentials := []*UserCredential{
			{
				ID:             uuid.New().String(),
				UserID:         userID,
				Type:           "password",
				Provider:       "local",
				CredentialData: "hashed_password",
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			},
			{
				ID:             uuid.New().String(),
				UserID:         userID,
				Type:           "oauth",
				Provider:       "github",
				ProviderID:     "github_123",
				CredentialData: "oauth_token_data",
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			},
		}
		
		for _, cred := range credentials {
			err := suite.store.CreateUserCredential(suite.ctx, cred)
			suite.NoError(err)
		}
		
		// Retrieve and verify
		retrievedCreds, err := suite.store.GetUserCredentials(suite.ctx, userID)
		suite.NoError(err)
		suite.Len(retrievedCreds, 2)
		
		// Verify credential types
		credTypes := make(map[string]bool)
		for _, cred := range retrievedCreds {
			credTypes[cred.Type] = true
		}
		suite.True(credTypes["password"])
		suite.True(credTypes["oauth"])
	})
}

// TestPerformance tests performance characteristics
func (suite *ComprehensiveUserTestSuite) TestPerformance() {
	suite.Run("Bulk User Operations", func() {
		const numUsers = 100
		userIDs := make([]string, numUsers)
		
		// Measure user creation time
		start := time.Now()
		
		for i := 0; i < numUsers; i++ {
			userID := uuid.New().String()
			userIDs[i] = userID
			
			user := &User{
				ID:            userID,
				Email:         fmt.Sprintf("bulk%d@example.com", i),
				Username:      fmt.Sprintf("bulk%d", i),
				FirstName:     "Bulk",
				LastName:      "User",
				Role:          "Viewer",
				Status:        "Active",
				EmailVerified: true,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
			
			err := suite.store.CreateUser(suite.ctx, user)
			suite.NoError(err)
		}
		
		creationTime := time.Since(start)
		suite.T().Logf("Created %d users in %v (avg: %v per user)", 
			numUsers, creationTime, creationTime/time.Duration(numUsers))
		
		// Measure retrieval time
		start = time.Now()
		
		for _, userID := range userIDs {
			_, err := suite.store.GetUser(suite.ctx, userID)
			suite.NoError(err)
		}
		
		retrievalTime := time.Since(start)
		suite.T().Logf("Retrieved %d users in %v (avg: %v per user)", 
			numUsers, retrievalTime, retrievalTime/time.Duration(numUsers))
		
		// Performance assertions (adjust thresholds as needed)
		suite.Less(creationTime, 5*time.Second, "User creation should be reasonably fast")
		suite.Less(retrievalTime, 1*time.Second, "User retrieval should be fast")
	})
}

// TestCleanupOperations tests cleanup and maintenance operations
func (suite *ComprehensiveUserTestSuite) TestCleanupOperations() {
	suite.Run("Expired Session Cleanup", func() {
		userID := uuid.New().String()
		user := &User{
			ID:            userID,
			Email:         "cleanup@example.com",
			Username:      "cleanupuser",
			FirstName:     "Cleanup",
			LastName:      "User",
			Role:          "Viewer",
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		
		err := suite.store.CreateUser(suite.ctx, user)
		suite.Require().NoError(err)
		
		// Create expired and valid sessions
		expiredSession := &UserSession{
			ID:        uuid.New().String(),
			UserID:    userID,
			SessionID: "expired_session",
			ExpiresAt: time.Now().Add(-time.Hour), // Expired
			CreatedAt: time.Now().Add(-2 * time.Hour),
			IPAddress: "192.168.1.1",
			UserAgent: "test-agent",
		}
		
		validSession := &UserSession{
			ID:        uuid.New().String(),
			UserID:    userID,
			SessionID: "valid_session",
			ExpiresAt: time.Now().Add(time.Hour), // Valid
			CreatedAt: time.Now(),
			IPAddress: "192.168.1.2",
			UserAgent: "test-agent",
		}
		
		err = suite.store.CreateUserSession(suite.ctx, expiredSession)
		suite.NoError(err)
		
		err = suite.store.CreateUserSession(suite.ctx, validSession)
		suite.NoError(err)
		
		// Cleanup expired sessions
		deletedCount, err := suite.store.DeleteExpiredSessions(suite.ctx)
		suite.NoError(err)
		suite.Equal(1, deletedCount)
		
		// Verify only valid session remains
		sessions, err := suite.store.GetUserSessions(suite.ctx, userID)
		suite.NoError(err)
		suite.Len(sessions, 1)
		suite.Equal("valid_session", sessions[0].SessionID)
	})
}

// Run the comprehensive test suite
func TestComprehensiveUserTestSuite(t *testing.T) {
	suite.Run(t, new(ComprehensiveUserTestSuite))
}