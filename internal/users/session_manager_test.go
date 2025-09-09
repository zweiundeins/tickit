package users

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewUserSessionManager(t *testing.T) {
	mockStore := &MockUserStore{}
	mockEventBus := &shared.EventBusWrapper{}
	sessionStore := CreateTestSessionStore("test-key")

	config := &UserSessionManagerConfig{
		SessionTimeout:  time.Hour,
		MaxSessions:     5,
		CleanupInterval: 30 * time.Minute,
	}

	sm := NewUserSessionManager(sessionStore, mockStore, mockEventBus, config)

	assert.NotNil(t, sm)
	assert.Equal(t, time.Hour, sm.sessionTimeout)
	assert.Equal(t, 5, sm.maxSessions)
	assert.Equal(t, 30*time.Minute, sm.cleanupInterval)
}

func TestNewUserSessionManagerWithDefaultConfig(t *testing.T) {
	mockStore := &MockUserStore{}
	mockEventBus := &shared.EventBusWrapper{}
	sessionStore := CreateTestSessionStore("test-key")

	sm := NewUserSessionManager(sessionStore, mockStore, mockEventBus, nil)

	assert.NotNil(t, sm)
	assert.Equal(t, 24*time.Hour, sm.sessionTimeout)
	assert.Equal(t, 0, sm.maxSessions) // unlimited
	assert.Equal(t, time.Hour, sm.cleanupInterval)
}

func TestCreateUserSession(t *testing.T) {
	mockStore := &MockUserStore{}
	mockEventBus := &shared.EventBusWrapper{}
	sessionStore := CreateTestSessionStore("test-key")

	config := &UserSessionManagerConfig{
		SessionTimeout:  time.Hour,
		MaxSessions:     0, // unlimited
		CleanupInterval: time.Hour,
	}

	sm := NewUserSessionManager(sessionStore, mockStore, mockEventBus, config)

	// Create test request and response
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	userID := "test-user-id"

	// Mock store expectations
	mockStore.On("CreateUserSession", mock.Anything, mock.AnythingOfType("*users.UserSession")).Return(nil)

	// Create user session
	userSession, err := sm.CreateUserSession(context.Background(), w, req, userID)

	require.NoError(t, err)
	assert.NotNil(t, userSession)
	assert.Equal(t, userID, userSession.UserID)
	assert.NotEmpty(t, userSession.SessionID)
	assert.NotEmpty(t, userSession.ID)

	mockStore.AssertExpectations(t)
}

func TestValidateSession_Success(t *testing.T) {
	mockStore := &MockUserStore{}
	mockEventBus := &shared.EventBusWrapper{}
	sessionStore := CreateTestSessionStore("test-key")

	sm := NewUserSessionManager(sessionStore, mockStore, mockEventBus, nil)

	// Create a request with session data
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// First create a session to get the cookie
	userID := "test-user-id"
	mockStore.On("CreateUserSession", mock.Anything, mock.AnythingOfType("*users.UserSession")).Return(nil)
	userSession, err := sm.CreateUserSession(context.Background(), w, req, userID)
	require.NoError(t, err)

	// Create a new request with the session cookie
	req2 := httptest.NewRequest("GET", "/", nil)
	for _, cookie := range w.Result().Cookies() {
		req2.AddCookie(cookie)
	}

	// Mock expectations for validation
	mockStore.On("GetUserSession", mock.Anything, userSession.SessionID).Return(userSession, nil)

	testUser := &User{
		ID:     userID,
		Email:  "test@example.com",
		Status: "Active",
	}
	mockStore.On("GetUser", mock.Anything, userID).Return(testUser, nil)

	// Validate session
	user, session, err := sm.ValidateSession(context.Background(), req2)

	require.NoError(t, err)
	assert.NotNil(t, user)
	assert.NotNil(t, session)
	assert.Equal(t, userID, user.ID)
	assert.Equal(t, userSession.SessionID, session.SessionID)

	mockStore.AssertExpectations(t)
}

func TestValidateSession_Expired(t *testing.T) {
	mockStore := &MockUserStore{}
	mockEventBus := &shared.EventBusWrapper{}
	sessionStore := CreateTestSessionStore("test-key")

	sm := NewUserSessionManager(sessionStore, mockStore, mockEventBus, nil)

	// Create a request with session data
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// First create a session
	userID := "test-user-id"
	mockStore.On("CreateUserSession", mock.Anything, mock.AnythingOfType("*users.UserSession")).Return(nil)
	userSession, err := sm.CreateUserSession(context.Background(), w, req, userID)
	require.NoError(t, err)

	// Make the session expired
	userSession.ExpiresAt = time.Now().Add(-time.Hour)

	// Create a new request with the session cookie
	req2 := httptest.NewRequest("GET", "/", nil)
	for _, cookie := range w.Result().Cookies() {
		req2.AddCookie(cookie)
	}

	// Mock expectations for validation
	mockStore.On("GetUserSession", mock.Anything, userSession.SessionID).Return(userSession, nil)
	mockStore.On("DeleteUserSession", mock.Anything, userSession.SessionID).Return(nil)

	// Validate session - should fail due to expiration
	user, session, err := sm.ValidateSession(context.Background(), req2)

	assert.Error(t, err)
	assert.Nil(t, user)
	assert.Nil(t, session)
	assert.Contains(t, err.Error(), "session expired")

	mockStore.AssertExpectations(t)
}

func TestInvalidateSession(t *testing.T) {
	mockStore := &MockUserStore{}
	mockEventBus := &shared.EventBusWrapper{}
	sessionStore := CreateTestSessionStore("test-key")

	sm := NewUserSessionManager(sessionStore, mockStore, mockEventBus, nil)

	sessionID := "test-session-id"

	// Mock store expectations
	mockStore.On("DeleteUserSession", mock.Anything, sessionID).Return(nil)

	// Invalidate session
	err := sm.InvalidateSession(context.Background(), nil, nil, sessionID)

	require.NoError(t, err)
	mockStore.AssertExpectations(t)
}

func TestInvalidateAllUserSessions(t *testing.T) {
	mockStore := &MockUserStore{}
	mockEventBus := &shared.EventBusWrapper{}
	sessionStore := CreateTestSessionStore("test-key")

	sm := NewUserSessionManager(sessionStore, mockStore, mockEventBus, nil)

	userID := "test-user-id"
	sessions := []*UserSession{
		{SessionID: "session-1", UserID: userID},
		{SessionID: "session-2", UserID: userID},
	}

	// Mock store expectations
	mockStore.On("GetUserSessions", mock.Anything, userID).Return(sessions, nil)
	mockStore.On("DeleteAllUserSessions", mock.Anything, userID).Return(nil)

	// Invalidate all user sessions
	err := sm.InvalidateAllUserSessions(context.Background(), userID)

	require.NoError(t, err)
	mockStore.AssertExpectations(t)
}

func TestConcurrentSessionLimits(t *testing.T) {
	mockStore := &MockUserStore{}
	mockEventBus := &shared.EventBusWrapper{}
	sessionStore := CreateTestSessionStore("test-key")

	config := &UserSessionManagerConfig{
		SessionTimeout:  time.Hour,
		MaxSessions:     2, // Limit to 2 sessions
		CleanupInterval: time.Hour,
	}

	sm := NewUserSessionManager(sessionStore, mockStore, mockEventBus, config)

	userID := "test-user-id"

	// Mock existing sessions (at the limit)
	existingSessions := []*UserSession{
		{SessionID: "session-1", UserID: userID, CreatedAt: time.Now().Add(-time.Hour)},
		{SessionID: "session-2", UserID: userID, CreatedAt: time.Now().Add(-30 * time.Minute)},
	}

	// Create test request and response
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// Mock store expectations
	mockStore.On("CreateUserSession", mock.Anything, mock.AnythingOfType("*users.UserSession")).Return(nil)
	mockStore.On("GetActiveUserSessions", mock.Anything, userID).Return(existingSessions, nil)
	mockStore.On("DeleteUserSession", mock.Anything, "session-1").Return(nil) // Should remove oldest

	// Create user session - should enforce limits
	userSession, err := sm.CreateUserSession(context.Background(), w, req, userID)

	require.NoError(t, err)
	assert.NotNil(t, userSession)

	mockStore.AssertExpectations(t)
}

func TestCleanupExpiredSessions(t *testing.T) {
	mockStore := &MockUserStore{}
	mockEventBus := &shared.EventBusWrapper{}
	sessionStore := CreateTestSessionStore("test-key")

	sm := NewUserSessionManager(sessionStore, mockStore, mockEventBus, nil)

	// Mock store expectations
	mockStore.On("DeleteExpiredSessions", mock.Anything).Return(3, nil) // 3 sessions deleted

	// Test cleanup
	err := sm.cleanupExpiredSessions(context.Background())

	require.NoError(t, err)
	mockStore.AssertExpectations(t)
}

func TestSessionMiddleware(t *testing.T) {
	mockStore := &MockUserStore{}
	mockEventBus := &shared.EventBusWrapper{}
	sessionStore := CreateTestSessionStore("test-key")

	sm := NewUserSessionManager(sessionStore, mockStore, mockEventBus, nil)

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := GetUserFromContext(r.Context())
		assert.True(t, ok)
		assert.NotNil(t, user)
		assert.Equal(t, "test-user-id", user.ID)

		session, ok := GetSessionFromContext(r.Context())
		assert.True(t, ok)
		assert.NotNil(t, session)

		w.WriteHeader(http.StatusOK)
	})

	// Wrap with session middleware
	handler := sm.SessionMiddleware(testHandler)

	// Create request with valid session
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// First create a session to get the cookie
	userID := "test-user-id"
	mockStore.On("CreateUserSession", mock.Anything, mock.AnythingOfType("*users.UserSession")).Return(nil)
	userSession, err := sm.CreateUserSession(context.Background(), w, req, userID)
	require.NoError(t, err)

	// Create a new request with the session cookie
	req2 := httptest.NewRequest("GET", "/", nil)
	for _, cookie := range w.Result().Cookies() {
		req2.AddCookie(cookie)
	}

	// Mock expectations for validation
	mockStore.On("GetUserSession", mock.Anything, userSession.SessionID).Return(userSession, nil)

	testUser := &User{
		ID:     userID,
		Email:  "test@example.com",
		Status: "Active",
	}
	mockStore.On("GetUser", mock.Anything, userID).Return(testUser, nil)
	mockStore.On("UpdateUserSession", mock.Anything, mock.AnythingOfType("*users.UserSession")).Return(nil)

	// Test the middleware
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	mockStore.AssertExpectations(t)
}

func TestGetUserFromContext(t *testing.T) {
	user := &User{ID: "test-user-id", Email: "test@example.com"}
	ctx := context.WithValue(context.Background(), "user", user)

	retrievedUser, ok := GetUserFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, user, retrievedUser)

	// Test with no user in context
	emptyCtx := context.Background()
	retrievedUser, ok = GetUserFromContext(emptyCtx)
	assert.False(t, ok)
	assert.Nil(t, retrievedUser)
}

func TestGetSessionFromContext(t *testing.T) {
	session := &UserSession{ID: "test-session-id", UserID: "test-user-id"}
	ctx := context.WithValue(context.Background(), "session", session)

	retrievedSession, ok := GetSessionFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, session, retrievedSession)

	// Test with no session in context
	emptyCtx := context.Background()
	retrievedSession, ok = GetSessionFromContext(emptyCtx)
	assert.False(t, ok)
	assert.Nil(t, retrievedSession)
}
