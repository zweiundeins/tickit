package users

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mbolli/tickit/internal/shared"
	"github.com/stretchr/testify/mock"
)

// MockEventPublisherForForgotPassword for testing
type MockEventPublisherForForgotPassword struct {
	publishedEvents []shared.Event
}

func (m *MockEventPublisherForForgotPassword) PublishEvent(ctx context.Context, event shared.Event) error {
	m.publishedEvents = append(m.publishedEvents, event)
	return nil
}

// Use existing MockEmailService from command_handlers_test.go

func TestForgotPasswordFlow(t *testing.T) {
	// Create a temporary database file
	dbFile := "test_forgot_password.db"
	defer os.Remove(dbFile)

	ctx := context.Background()

	// Create test database
	userStore, err := NewUserStore(ctx, dbFile)
	if err != nil {
		t.Fatalf("Failed to create user store: %v", err)
	}
	defer userStore.db.Close()

	// Create mock services
	mockEventPublisher := &MockEventPublisherForForgotPassword{}
	mockEmailService := new(MockEmailService)
	passwordHasher := NewPasswordHasher()
	webauthnService := NewWebAuthnService()

	// Create event store (mock)
	eventStoreFile := "test_forgot_password_events.db"
	defer os.Remove(eventStoreFile)
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
	mockEmailService.On("SendPasswordResetEmail", ctx, "test@example.com", mock.AnythingOfType("string")).Return(nil)

	// Test forgot password
	forgotPasswordCmd := ForgotPasswordCommand{
		Email: "test@example.com",
	}

	err = commandHandler.HandleForgotPassword(ctx, forgotPasswordCmd)
	if err != nil {
		t.Fatalf("HandleForgotPassword failed: %v", err)
	}

	// Verify email was sent
	mockEmailService.AssertCalled(t, "SendPasswordResetEmail", ctx, "test@example.com", mock.AnythingOfType("string"))

	// Verify event was published
	if len(mockEventPublisher.publishedEvents) != 1 {
		t.Fatalf("Expected 1 event to be published, got %d", len(mockEventPublisher.publishedEvents))
	}

	event, ok := mockEventPublisher.publishedEvents[0].(PasswordResetRequestedEvent)
	if !ok {
		t.Fatalf("Expected PasswordResetRequestedEvent, got %T", mockEventPublisher.publishedEvents[0])
	}

	if event.UserID != testUser.ID {
		t.Fatalf("Expected event UserID to be %s, got %s", testUser.ID, event.UserID)
	}

	// Test reset password with the token
	resetPasswordCmd := ResetPasswordCommand{
		Token:       event.Token,
		NewPassword: "NewPassword123",
	}

	err = commandHandler.HandleResetPassword(ctx, resetPasswordCmd)
	if err != nil {
		t.Fatalf("HandleResetPassword failed: %v", err)
	}

	// Verify password reset completed event was published
	if len(mockEventPublisher.publishedEvents) != 3 { // ForgotPassword + ResetPassword + SessionsInvalidated
		t.Fatalf("Expected 3 events to be published after reset, got %d", len(mockEventPublisher.publishedEvents))
	}

	resetEvent, ok := mockEventPublisher.publishedEvents[1].(PasswordResetCompletedEvent)
	if !ok {
		t.Fatalf("Expected PasswordResetCompletedEvent, got %T", mockEventPublisher.publishedEvents[1])
	}

	if resetEvent.UserID != testUser.ID {
		t.Fatalf("Expected reset event UserID to be %s, got %s", testUser.ID, resetEvent.UserID)
	}
}

func TestForgotPasswordNonExistentUser(t *testing.T) {
	// Create a temporary database file
	dbFile := "test_forgot_password_nonexistent.db"
	defer os.Remove(dbFile)

	ctx := context.Background()

	// Create test database
	userStore, err := NewUserStore(ctx, dbFile)
	if err != nil {
		t.Fatalf("Failed to create user store: %v", err)
	}
	defer userStore.db.Close()

	// Create mock services
	mockEventPublisher := &MockEventPublisherForForgotPassword{}
	mockEmailService := new(MockEmailService)
	passwordHasher := NewPasswordHasher()
	webauthnService := NewWebAuthnService()

	// Create event store (mock)
	eventStoreFile2 := "test_forgot_password_nonexistent_events.db"
	defer os.Remove(eventStoreFile2)
	eventStore, err := shared.NewEventStore(ctx, eventStoreFile2)
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

	// Test forgot password for non-existent user
	forgotPasswordCmd := ForgotPasswordCommand{
		Email: "nonexistent@example.com",
	}

	err = commandHandler.HandleForgotPassword(ctx, forgotPasswordCmd)
	// Should not return error for security reasons (prevent user enumeration)
	if err != nil {
		t.Fatalf("HandleForgotPassword should not fail for non-existent user: %v", err)
	}

	// Verify no email was sent
	mockEmailService.AssertNotCalled(t, "SendPasswordResetEmail")

	// Verify no event was published
	if len(mockEventPublisher.publishedEvents) != 0 {
		t.Fatalf("Expected 0 events to be published for non-existent user, got %d", len(mockEventPublisher.publishedEvents))
	}
}
