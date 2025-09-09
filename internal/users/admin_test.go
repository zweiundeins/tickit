package users

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdministrativeUserManagement(t *testing.T) {
	ctx := context.Background()

	// Create test store
	store, err := NewUserStore(ctx, ":memory:")
	require.NoError(t, err)

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
		{
			ID:            "user-2",
			Email:         "user2@test.com",
			Username:      "user2",
			FirstName:     "Test",
			LastName:      "User2",
			Role:          "Viewer",
			Status:        "Inactive",
			EmailVerified: false,
		},
	}

	// Create users in store
	for _, user := range testUsers {
		err := store.CreateUser(ctx, user)
		require.NoError(t, err)
	}

	t.Run("ListUsers", func(t *testing.T) {
		// Test listing all users
		users, err := store.ListUsers(ctx, 10, 0, nil)
		require.NoError(t, err)
		assert.Len(t, users, 3)

		// Test filtering by status
		activeUsers, err := store.ListUsers(ctx, 10, 0, map[string]interface{}{"status": "Active"})
		require.NoError(t, err)
		assert.Len(t, activeUsers, 2)

		// Test filtering by role
		adminUsers, err := store.ListUsers(ctx, 10, 0, map[string]interface{}{"role": "Admin"})
		require.NoError(t, err)
		assert.Len(t, adminUsers, 1)
		assert.Equal(t, "admin@test.com", adminUsers[0].Email)
	})

	t.Run("SearchUsers", func(t *testing.T) {
		// Test search by email
		users, err := store.SearchUsers(ctx, "user1@test.com", 10, 0)
		require.NoError(t, err)
		assert.Len(t, users, 1)
		assert.Equal(t, "user1@test.com", users[0].Email)

		// Test search by name
		users, err = store.SearchUsers(ctx, "Test", 10, 0)
		require.NoError(t, err)
		assert.Len(t, users, 2) // Should find user1 and user2
	})

	t.Run("GetUserStats", func(t *testing.T) {
		// Test getting user stats
		stats, err := store.GetUserStats(ctx, "user-1")
		require.NoError(t, err)
		assert.NotNil(t, stats)
		assert.Contains(t, stats, "status")
		assert.Contains(t, stats, "email_verified")
		assert.Equal(t, "Active", stats["status"])
		assert.Equal(t, true, stats["email_verified"])
	})

	t.Run("GetSystemUserStats", func(t *testing.T) {
		// Test getting system stats
		stats, err := store.GetSystemUserStats(ctx)
		require.NoError(t, err)
		assert.NotNil(t, stats)
		assert.Contains(t, stats, "total_users")
		assert.Contains(t, stats, "active_users")
		assert.Contains(t, stats, "verified_users")
		assert.Contains(t, stats, "users_by_role")

		assert.Equal(t, int64(3), stats["total_users"])
		assert.Equal(t, int64(2), stats["active_users"])
		assert.Equal(t, int64(2), stats["verified_users"])

		roleStats := stats["users_by_role"].(map[string]int64)
		assert.Equal(t, int64(1), roleStats["Admin"])
		assert.Equal(t, int64(1), roleStats["Developer"])
		assert.Equal(t, int64(1), roleStats["Viewer"])
	})

	t.Run("GetUserActivity", func(t *testing.T) {
		// Create a test session for activity
		session := &UserSession{
			ID:        "session-1",
			UserID:    "user-1",
			SessionID: "test-session",
			IPAddress: "127.0.0.1",
			UserAgent: "Test Agent",
		}
		err := store.CreateUserSession(ctx, session)
		require.NoError(t, err)

		// Test getting user activity
		activity, err := store.GetUserActivity(ctx, "user-1", 10, 0)
		require.NoError(t, err)
		assert.Len(t, activity, 1)

		activityItem := activity[0].(map[string]interface{})
		assert.Equal(t, "login", activityItem["type"])
		assert.Equal(t, "127.0.0.1", activityItem["ip_address"])
		assert.Equal(t, "Test Agent", activityItem["user_agent"])
	})

	t.Run("PatchUser", func(t *testing.T) {
		// Test patching user data
		newRole := "ProjectManager"
		newStatus := "Inactive"
		patch := &PatchUserInput{
			ID:     "user-1",
			Role:   &newRole,
			Status: &newStatus,
		}

		err := store.PatchUser(ctx, patch)
		require.NoError(t, err)

		// Verify the changes
		user, err := store.GetUser(ctx, "user-1")
		require.NoError(t, err)
		assert.Equal(t, "ProjectManager", user.Role)
		assert.Equal(t, "Inactive", user.Status)
	})
}
