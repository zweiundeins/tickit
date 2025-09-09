package users

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mbolli/tickit/internal/shared"
	"github.com/stretchr/testify/mock"
)

func TestForgotPasswordHTTPIntegration(t *testing.T) {
	// Create temporary database files
	dbFile := "test_forgot_password_http.db"
	defer os.Remove(dbFile)
	eventStoreFile := "test_forgot_password_http_events.db"
	defer os.Remove(eventStoreFile)

	ctx := context.Background()

	// Create test database
	userStore, err := NewUserStore(ctx, dbFile)
	if err != nil {
		t.Fatalf("Failed to create user store: %v", err)
	}
	defer userStore.db.Close()

	// Create mock services
	mockEventPublisher := &MockEventPublisher{}
	mockEmailService := new(MockEmailService)
	passwordHasher := NewPasswordHasher()
	webauthnService := NewWebAuthnService()

	// Create event store
	eventStore, err := shared.NewEventStore(ctx, eventStoreFile)
	if err != nil {
		t.Fatalf("Failed to create event store: %v", err)
	}
	defer eventStore.Close()

	// Create command handler
	commandHandler := NewUserCommandHandler(
		mockEventPublisher,
		eventStore,
		userStore,
		passwordHasher,
		mockEmailService,
		webauthnService,
	)

	// Create command bus and register handler
	commandBus := shared.NewCommandBus()
	commandBus.RegisterHandler("ForgotPassword", commandHandler)
	commandBus.RegisterHandler("ResetPassword", commandHandler)

	// Create query bus (not used in this test but required for service)
	queryBus := shared.NewQueryBus()

	// Create user service
	userService := NewUserService(commandBus, queryBus)

	// Create auth service
	authService := NewAuthServiceImpl(userService)

	// Create routes
	routes := NewUserRoutes(userService, nil, authService, webauthnService)

	// Create a test user first
	testUser := &User{
		ID:            uuid.New().String(),
		Email:         "test@example.com",
		Username:      "testuser",
		FirstName:     "Test",
		LastName:      "User",
		Role:          "Viewer",
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err = userStore.CreateUser(ctx, testUser)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Setup mock expectations
	mockEmailService.On("SendPasswordResetEmail", mock.Anything, "test@example.com", mock.AnythingOfType("string")).Return(nil)

	// Test forgot password HTTP endpoint
	t.Run("ForgotPassword HTTP", func(t *testing.T) {
		reqBody := map[string]string{
			"email": "test@example.com",
		}
		jsonBody, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/auth/forgot-password", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		routes.HandleForgotPassword(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var response map[string]string
		err := json.Unmarshal(w.Body.Bytes(), &response)
		if err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		expectedMessage := "If an account with that email exists, a password reset link has been sent"
		if response["message"] != expectedMessage {
			t.Errorf("Expected message '%s', got '%s'", expectedMessage, response["message"])
		}

		// Verify email was sent
		mockEmailService.AssertCalled(t, "SendPasswordResetEmail", mock.Anything, "test@example.com", mock.AnythingOfType("string"))

		// Verify event was published
		if len(mockEventPublisher.publishedEvents) != 1 {
			t.Fatalf("Expected 1 event to be published, got %d", len(mockEventPublisher.publishedEvents))
		}

		event, ok := mockEventPublisher.publishedEvents[0].(PasswordResetRequestedEvent)
		if !ok {
			t.Fatalf("Expected PasswordResetRequestedEvent, got %T", mockEventPublisher.publishedEvents[0])
		}

		// Test reset password HTTP endpoint
		t.Run("ResetPassword HTTP", func(t *testing.T) {
			resetReqBody := map[string]string{
				"token":        event.Token,
				"new_password": "NewPassword123",
			}
			resetJsonBody, _ := json.Marshal(resetReqBody)

			resetReq := httptest.NewRequest("POST", "/auth/reset-password", bytes.NewBuffer(resetJsonBody))
			resetReq.Header.Set("Content-Type", "application/json")
			resetW := httptest.NewRecorder()

			routes.HandleResetPassword(resetW, resetReq)

			if resetW.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resetW.Code)
			}

			var resetResponse map[string]string
			err := json.Unmarshal(resetW.Body.Bytes(), &resetResponse)
			if err != nil {
				t.Fatalf("Failed to parse reset response: %v", err)
			}

			expectedResetMessage := "Password has been reset successfully. All sessions have been invalidated."
			if resetResponse["message"] != expectedResetMessage {
				t.Errorf("Expected message '%s', got '%s'", expectedResetMessage, resetResponse["message"])
			}

			// Verify password reset completed event was published
			if len(mockEventPublisher.publishedEvents) != 3 { // ForgotPassword + ResetPassword + SessionsInvalidated
				t.Fatalf("Expected 3 events to be published after reset, got %d", len(mockEventPublisher.publishedEvents))
			}
		})
	})

	// Test forgot password with invalid email
	t.Run("ForgotPassword Invalid Email", func(t *testing.T) {
		reqBody := map[string]string{
			"email": "invalid-email",
		}
		jsonBody, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/auth/forgot-password", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		routes.HandleForgotPassword(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	// Test reset password with invalid token
	t.Run("ResetPassword Invalid Token", func(t *testing.T) {
		resetReqBody := map[string]string{
			"token":        "invalid-token",
			"new_password": "NewPassword123",
		}
		resetJsonBody, _ := json.Marshal(resetReqBody)

		resetReq := httptest.NewRequest("POST", "/auth/reset-password", bytes.NewBuffer(resetJsonBody))
		resetReq.Header.Set("Content-Type", "application/json")
		resetW := httptest.NewRecorder()

		routes.HandleResetPassword(resetW, resetReq)

		if resetW.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resetW.Code)
		}
	})
}
