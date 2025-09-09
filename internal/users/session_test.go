package users

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteExpiredSessions(t *testing.T) {
	ctx := context.Background()

	// Create a temporary database file
	dbFile := "test_sessions.db"
	defer os.Remove(dbFile)

	// Create a test database
	store, err := NewUserStore(ctx, dbFile)
	require.NoError(t, err)
	defer store.db.Close()

	// Create test users first (required for foreign key constraint)
	user1 := &User{
		ID:        "user-1",
		Email:     "user1@example.com",
		Username:  "user1",
		Role:      "Viewer",
		Status:    "Active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	user2 := &User{
		ID:        "user-2",
		Email:     "user2@example.com",
		Username:  "user2",
		Role:      "Viewer",
		Status:    "Active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = store.CreateUser(ctx, user1)
	require.NoError(t, err)

	err = store.CreateUser(ctx, user2)
	require.NoError(t, err)

	// Create some test sessions - some expired, some active
	now := time.Now()

	// Active session
	activeSession := &UserSession{
		ID:        "active-session",
		UserID:    "user-1",
		SessionID: "session-1",
		ExpiresAt: now.Add(time.Hour), // Expires in 1 hour
		CreatedAt: now,
		IPAddress: "127.0.0.1",
		UserAgent: "test-agent",
	}

	// Expired session
	expiredSession := &UserSession{
		ID:        "expired-session",
		UserID:    "user-2",
		SessionID: "session-2",
		ExpiresAt: now.Add(-time.Hour), // Expired 1 hour ago
		CreatedAt: now.Add(-2 * time.Hour),
		IPAddress: "127.0.0.1",
		UserAgent: "test-agent",
	}

	// Create sessions
	err = store.CreateUserSession(ctx, activeSession)
	require.NoError(t, err)

	err = store.CreateUserSession(ctx, expiredSession)
	require.NoError(t, err)

	// Verify both sessions exist
	sessions, err := store.GetUserSessions(ctx, "user-1")
	require.NoError(t, err)
	assert.Len(t, sessions, 1)

	sessions, err = store.GetUserSessions(ctx, "user-2")
	require.NoError(t, err)
	assert.Len(t, sessions, 1)

	// Delete expired sessions
	deletedCount, err := store.DeleteExpiredSessions(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, deletedCount) // Should delete 1 expired session

	// Verify only active session remains
	sessions, err = store.GetUserSessions(ctx, "user-1")
	require.NoError(t, err)
	assert.Len(t, sessions, 1) // Active session should remain

	sessions, err = store.GetUserSessions(ctx, "user-2")
	require.NoError(t, err)
	assert.Len(t, sessions, 0) // Expired session should be gone
}

func TestSessionConfig(t *testing.T) {
	// Test default config
	config := DefaultSessionConfig()
	assert.NotNil(t, config)
	assert.Equal(t, "tickit_session", config.SessionName)
	assert.Equal(t, 24*time.Hour, config.SessionTimeout)
	assert.True(t, config.HttpOnly)

	// Test config validation
	err := config.ValidateConfig()
	assert.NoError(t, err)

	// Test session store creation
	store := config.CreateSessionStore()
	assert.NotNil(t, store)
}

func TestUserSessionManagerConfig(t *testing.T) {
	config := &UserSessionManagerConfig{
		SessionTimeout:  2 * time.Hour,
		MaxSessions:     3,
		CleanupInterval: 30 * time.Minute,
	}

	assert.Equal(t, 2*time.Hour, config.SessionTimeout)
	assert.Equal(t, 3, config.MaxSessions)
	assert.Equal(t, 30*time.Minute, config.CleanupInterval)
}
