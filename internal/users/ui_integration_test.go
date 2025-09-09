package users

import (
	"context"
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/shared"
	"github.com/stretchr/testify/assert"
)

// Simple event collector for testing
type EventCollector struct {
	events []shared.Event
}

func (ec *EventCollector) PublishEvent(ctx context.Context, event shared.Event) error {
	ec.events = append(ec.events, event)
	return nil
}

func (ec *EventCollector) GetEvents() []shared.Event {
	return ec.events
}

func (ec *EventCollector) GetEventsByType(eventType string) []shared.Event {
	var filtered []shared.Event
	for _, event := range ec.events {
		if event.GetEventType() == eventType {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func TestUIEventIntegration_LoginLogout(t *testing.T) {
	ctx := context.Background()
	
	// Create test infrastructure
	eventCollector := &EventCollector{}
	
	// Create a simple in-memory user store for testing
	userStore := &InMemoryUserStore{
		users: map[string]*User{
			"user-123": {
				ID:       "user-123",
				Username: "testuser",
				Email:    "test@example.com",
				Status:   "Active",
			},
		},
	}
	
	// Create UI event service
	uiEventService := NewUIEventService(eventCollector, userStore)
	
	// Test login event
	loginEvent := UserLoggedInEvent{
		UserID:    "user-123",
		SessionID: "session-123",
		Method:    "password",
		Provider:  "",
		Timestamp: time.Now(),
	}
	
	err := uiEventService.HandleUserLoggedIn(ctx, loginEvent)
	assert.NoError(t, err)
	
	// Verify UI events were published
	events := eventCollector.GetEvents()
	assert.Len(t, events, 2, "Should publish 2 UI events for login")
	
	// Check login status event
	loginStatusEvents := eventCollector.GetEventsByType("UserLoginStatus")
	assert.Len(t, loginStatusEvents, 1)
	
	loginStatusEvent := loginStatusEvents[0].(UserLoginStatusEvent)
	assert.Equal(t, "user-123", loginStatusEvent.UserID)
	assert.Equal(t, "testuser", loginStatusEvent.Username)
	assert.Equal(t, "login", loginStatusEvent.Action)
	assert.Equal(t, "password", loginStatusEvent.Method)
	
	// Check status changed event
	statusChangedEvents := eventCollector.GetEventsByType("UserStatusChanged")
	assert.Len(t, statusChangedEvents, 1)
	
	statusChangedEvent := statusChangedEvents[0].(UserStatusChangedEvent)
	assert.Equal(t, "user-123", statusChangedEvent.UserID)
	assert.Equal(t, "testuser", statusChangedEvent.Username)
	assert.Equal(t, "Active", statusChangedEvent.Status)
	assert.True(t, statusChangedEvent.IsOnline)
	
	// Reset collector for logout test
	eventCollector.events = nil
	
	// Test logout event
	logoutEvent := UserLoggedOutEvent{
		UserID:    "user-123",
		SessionID: "session-123",
		Timestamp: time.Now(),
	}
	
	err = uiEventService.HandleUserLoggedOut(ctx, logoutEvent)
	assert.NoError(t, err)
	
	// Verify logout UI events
	events = eventCollector.GetEvents()
	assert.Len(t, events, 2, "Should publish 2 UI events for logout")
	
	// Check logout status event
	loginStatusEvents = eventCollector.GetEventsByType("UserLoginStatus")
	assert.Len(t, loginStatusEvents, 1)
	
	logoutStatusEvent := loginStatusEvents[0].(UserLoginStatusEvent)
	assert.Equal(t, "user-123", logoutStatusEvent.UserID)
	assert.Equal(t, "testuser", logoutStatusEvent.Username)
	assert.Equal(t, "logout", logoutStatusEvent.Action)
	
	// Check status changed event for logout
	statusChangedEvents = eventCollector.GetEventsByType("UserStatusChanged")
	assert.Len(t, statusChangedEvents, 1)
	
	statusChangedEvent = statusChangedEvents[0].(UserStatusChangedEvent)
	assert.Equal(t, "user-123", statusChangedEvent.UserID)
	assert.Equal(t, "testuser", statusChangedEvent.Username)
	assert.Equal(t, "Active", statusChangedEvent.Status)
	assert.False(t, statusChangedEvent.IsOnline)
}

func TestUIEventIntegration_RoleChange(t *testing.T) {
	ctx := context.Background()
	
	// Create test infrastructure
	eventCollector := &EventCollector{}
	
	// Create a simple in-memory user store for testing
	userStore := &InMemoryUserStore{
		users: map[string]*User{
			"user-123": {
				ID:       "user-123",
				Username: "testuser",
				Email:    "test@example.com",
				Status:   "Active",
			},
			"admin-456": {
				ID:       "admin-456",
				Username: "admin",
				Email:    "admin@example.com",
				Status:   "Active",
			},
		},
	}
	
	// Create UI event service
	uiEventService := NewUIEventService(eventCollector, userStore)
	
	// Test role assignment event
	roleEvent := UserRoleAssignedEvent{
		UserID:     "user-123",
		OldRole:    "Developer",
		NewRole:    "ProjectManager",
		AssignedBy: "admin-456",
		Timestamp:  time.Now(),
	}
	
	err := uiEventService.HandleUserRoleAssigned(ctx, roleEvent)
	assert.NoError(t, err)
	
	// Verify UI events were published
	events := eventCollector.GetEvents()
	assert.Len(t, events, 2, "Should publish 2 UI events for role change")
	
	// Check role changed event
	roleChangedEvents := eventCollector.GetEventsByType("UserRoleChanged")
	assert.Len(t, roleChangedEvents, 1)
	
	roleChangedEvent := roleChangedEvents[0].(UserRoleChangedEvent)
	assert.Equal(t, "user-123", roleChangedEvent.UserID)
	assert.Equal(t, "testuser", roleChangedEvent.Username)
	assert.Equal(t, "Developer", roleChangedEvent.OldRole)
	assert.Equal(t, "ProjectManager", roleChangedEvent.NewRole)
	assert.Equal(t, "admin", roleChangedEvent.ChangedBy)
	
	// Check admin management event
	adminEvents := eventCollector.GetEventsByType("AdminUserManagement")
	assert.Len(t, adminEvents, 1)
	
	adminEvent := adminEvents[0].(AdminUserManagementEvent)
	assert.Equal(t, "admin-456", adminEvent.AdminID)
	assert.Equal(t, "admin", adminEvent.AdminName)
	assert.Equal(t, "user-123", adminEvent.TargetID)
	assert.Equal(t, "testuser", adminEvent.TargetName)
	assert.Equal(t, "role_changed", adminEvent.Action)
}

func TestUIEventIntegration_MFAStatusChange(t *testing.T) {
	ctx := context.Background()
	
	// Create test infrastructure
	eventCollector := &EventCollector{}
	
	// Create a simple in-memory user store for testing
	userStore := &InMemoryUserStore{
		users: map[string]*User{
			"user-123": {
				ID:       "user-123",
				Username: "testuser",
				Email:    "test@example.com",
				Status:   "Active",
			},
		},
	}
	
	// Create UI event service
	uiEventService := NewUIEventService(eventCollector, userStore)
	
	// Test MFA enabled event
	mfaEvent := UserMFAEnabledEvent{
		UserID:    "user-123",
		Timestamp: time.Now(),
	}
	
	err := uiEventService.HandleUserMFAEnabled(ctx, mfaEvent)
	assert.NoError(t, err)
	
	// Verify UI event was published
	events := eventCollector.GetEvents()
	assert.Len(t, events, 1, "Should publish 1 UI event for MFA enabled")
	
	// Check MFA status changed event
	mfaStatusEvents := eventCollector.GetEventsByType("UserMFAStatusChanged")
	assert.Len(t, mfaStatusEvents, 1)
	
	mfaStatusEvent := mfaStatusEvents[0].(UserMFAStatusChangedEvent)
	assert.Equal(t, "user-123", mfaStatusEvent.UserID)
	assert.Equal(t, "testuser", mfaStatusEvent.Username)
	assert.True(t, mfaStatusEvent.Enabled)
	assert.Equal(t, "webauthn", mfaStatusEvent.Method)
}

func TestUIEventIntegration_ProfileUpdate(t *testing.T) {
	ctx := context.Background()
	
	// Create test infrastructure
	eventCollector := &EventCollector{}
	
	// Create a simple in-memory user store for testing
	userStore := &InMemoryUserStore{
		users: map[string]*User{
			"user-123": {
				ID:       "user-123",
				Username: "testuser",
				Email:    "test@example.com",
				Status:   "Active",
			},
		},
	}
	
	// Create UI event service
	uiEventService := NewUIEventService(eventCollector, userStore)
	
	// Test profile update event
	profileEvent := UserProfileUpdatedEvent{
		UserID: "user-123",
		Changes: map[string]interface{}{
			"first_name": "John",
			"last_name":  "Doe",
			"password":   "should_not_appear", // This should be filtered out
		},
		UpdatedBy: "user-123", // Self-update
		Timestamp: time.Now(),
	}
	
	err := uiEventService.HandleUserProfileUpdated(ctx, profileEvent)
	assert.NoError(t, err)
	
	// Verify UI event was published
	events := eventCollector.GetEvents()
	assert.Len(t, events, 1, "Should publish 1 UI event for profile update")
	
	// Check profile changed event
	profileChangedEvents := eventCollector.GetEventsByType("UserProfileChanged")
	assert.Len(t, profileChangedEvents, 1)
	
	profileChangedEvent := profileChangedEvents[0].(UserProfileChangedEvent)
	assert.Equal(t, "user-123", profileChangedEvent.UserID)
	assert.Equal(t, "testuser", profileChangedEvent.Username)
	assert.Equal(t, "testuser", profileChangedEvent.ChangedBy)
	
	// Verify sensitive fields are filtered out
	for _, field := range profileChangedEvent.FieldsChanged {
		assert.NotEqual(t, "password", field, "Password field should be filtered out")
		assert.NotEqual(t, "password_hash", field, "Password hash field should be filtered out")
	}
	
	// Should have 2 fields (first_name and last_name)
	assert.Len(t, profileChangedEvent.FieldsChanged, 2)
}