package users

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectIntegration(t *testing.T) {
	ctx := context.Background()

	// Create test user store
	userStore, err := NewUserStore(ctx, ":memory:")
	require.NoError(t, err)
	defer userStore.Close()

	// Create a test user
	testUser := &User{
		ID:            uuid.New().String(),
		Email:         "test@example.com",
		Username:      "testuser",
		FirstName:     "Test",
		LastName:      "User",
		Role:          RoleProjectManager,
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err = userStore.CreateUser(ctx, testUser)
	require.NoError(t, err)

	t.Run("ProjectCreatedEventHandler", func(t *testing.T) {
		handler := NewProjectCreatedEventHandler(userStore)

		// Create a mock project created event
		projectID := uuid.New().String()
		eventData := map[string]interface{}{
			"Project": map[string]interface{}{
				"id":   projectID,
				"name": "Test Project",
			},
			"CreatedBy": testUser.ID,
			"Timestamp": time.Now(),
		}

		// Handle the event
		err := handler.Handle(ctx, eventData)
		require.NoError(t, err)

		// Verify project membership was created
		membership, err := userStore.GetProjectMembership(ctx, testUser.ID, projectID)
		require.NoError(t, err)
		assert.NotNil(t, membership)
		assert.Equal(t, testUser.ID, membership.UserID)
		assert.Equal(t, projectID, membership.ProjectID)
		assert.Equal(t, ProjectRoleAdmin, membership.Role)
		assert.Equal(t, testUser.ID, membership.CreatedBy)
	})

	t.Run("ProjectCreatedEventHandler_InvalidEvent", func(t *testing.T) {
		handler := NewProjectCreatedEventHandler(userStore)

		// Test with invalid event format
		err := handler.Handle(ctx, "invalid event")
		require.NoError(t, err) // Should not fail, just log warning
	})

	t.Run("ProjectCreatedEventHandler_MissingCreator", func(t *testing.T) {
		handler := NewProjectCreatedEventHandler(userStore)

		// Create event without creator
		eventData := map[string]interface{}{
			"Project": map[string]interface{}{
				"id":   uuid.New().String(),
				"name": "Test Project",
			},
			"Timestamp": time.Now(),
		}

		// Handle the event
		err := handler.Handle(ctx, eventData)
		require.NoError(t, err) // Should not fail, just log warning
	})

	t.Run("ProjectCreatedEventHandler_NonexistentCreator", func(t *testing.T) {
		handler := NewProjectCreatedEventHandler(userStore)

		// Create event with nonexistent creator
		eventData := map[string]interface{}{
			"Project": map[string]interface{}{
				"id":   uuid.New().String(),
				"name": "Test Project",
			},
			"CreatedBy": "nonexistent-user",
			"Timestamp": time.Now(),
		}

		// Handle the event
		err := handler.Handle(ctx, eventData)
		require.NoError(t, err) // Should not fail, just log warning
	})
}

func TestIssueAssignmentValidation(t *testing.T) {
	ctx := context.Background()

	// Create test user store
	userStore, err := NewUserStore(ctx, ":memory:")
	require.NoError(t, err)
	defer userStore.Close()

	// Create test users
	adminUser := &User{
		ID:            uuid.New().String(),
		Email:         "admin@example.com",
		Username:      "admin",
		Role:          RoleAdmin,
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	projectUser := &User{
		ID:            uuid.New().String(),
		Email:         "project@example.com",
		Username:      "projectuser",
		Role:          RoleDeveloper,
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	nonProjectUser := &User{
		ID:            uuid.New().String(),
		Email:         "other@example.com",
		Username:      "otheruser",
		Role:          RoleDeveloper,
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err = userStore.CreateUser(ctx, adminUser)
	require.NoError(t, err)
	err = userStore.CreateUser(ctx, projectUser)
	require.NoError(t, err)
	err = userStore.CreateUser(ctx, nonProjectUser)
	require.NoError(t, err)

	// Create project membership for projectUser
	projectID := uuid.New().String()
	membership := &ProjectMembership{
		ID:        uuid.New().String(),
		UserID:    projectUser.ID,
		ProjectID: projectID,
		Role:      ProjectRoleDeveloper,
		CreatedAt: time.Now(),
		CreatedBy: adminUser.ID,
	}
	err = userStore.CreateProjectMembership(ctx, membership)
	require.NoError(t, err)

	validator := NewIssueAssignmentValidator(userStore)

	t.Run("ValidateIssueAssignment_EmptyUser", func(t *testing.T) {
		err := validator.ValidateIssueAssignment(ctx, "", projectID)
		assert.NoError(t, err) // Empty assignment should be allowed
	})

	t.Run("ValidateIssueAssignment_AdminUser", func(t *testing.T) {
		err := validator.ValidateIssueAssignment(ctx, adminUser.ID, projectID)
		assert.NoError(t, err) // Admin should be allowed
	})

	t.Run("ValidateIssueAssignment_ProjectMember", func(t *testing.T) {
		err := validator.ValidateIssueAssignment(ctx, projectUser.ID, projectID)
		assert.NoError(t, err) // Project member should be allowed
	})

	t.Run("ValidateIssueAssignment_NonProjectMember", func(t *testing.T) {
		err := validator.ValidateIssueAssignment(ctx, nonProjectUser.ID, projectID)
		assert.Error(t, err) // Non-project member should be rejected
		assert.Contains(t, err.Error(), "does not have access to project")
	})

	t.Run("ValidateIssueAssignment_NonexistentUser", func(t *testing.T) {
		err := validator.ValidateIssueAssignment(ctx, "nonexistent", projectID)
		assert.Error(t, err) // Nonexistent user should be rejected
		assert.Contains(t, err.Error(), "user not found")
	})
}

func TestMigrationHelpers(t *testing.T) {
	ctx := context.Background()

	// Create test user store
	userStore, err := NewUserStore(ctx, ":memory:")
	require.NoError(t, err)
	defer userStore.Close()

	helper := NewMigrationHelper(userStore)

	t.Run("EnsureBackwardCompatibility_NoAdminUsers", func(t *testing.T) {
		err := helper.EnsureBackwardCompatibility(ctx)
		require.NoError(t, err)

		// Verify default admin was created
		users, err := userStore.ListUsers(ctx, 10, 0, map[string]interface{}{
			"role": RoleAdmin,
		})
		require.NoError(t, err)
		assert.Len(t, users, 1)
		assert.Equal(t, "default-admin-user", users[0].ID)
		assert.Equal(t, "admin@tickit.local", users[0].Email)
	})

	t.Run("EnsureBackwardCompatibility_AdminExists", func(t *testing.T) {
		// Create an admin user first
		adminUser := &User{
			ID:            uuid.New().String(),
			Email:         "existing@example.com",
			Username:      "existing",
			Role:          RoleAdmin,
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		err := userStore.CreateUser(ctx, adminUser)
		require.NoError(t, err)

		// Run backward compatibility check
		err = helper.EnsureBackwardCompatibility(ctx)
		require.NoError(t, err)

		// Verify no additional admin was created
		users, err := userStore.ListUsers(ctx, 10, 0, map[string]interface{}{
			"role": RoleAdmin,
		})
		require.NoError(t, err)
		assert.Len(t, users, 2) // Original default + new admin
	})

	t.Run("GetOrCreateDefaultUser_ExistingUser", func(t *testing.T) {
		// Create a test user
		testUser := &User{
			ID:            uuid.New().String(),
			Email:         "test@example.com",
			Username:      "testuser",
			Role:          RoleDeveloper,
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		err := userStore.CreateUser(ctx, testUser)
		require.NoError(t, err)

		// Get the user
		user, err := helper.GetOrCreateDefaultUser(ctx, testUser.ID)
		require.NoError(t, err)
		assert.Equal(t, testUser.ID, user.ID)
		assert.Equal(t, testUser.Email, user.Email)
	})

	t.Run("GetOrCreateDefaultUser_NonexistentUser", func(t *testing.T) {
		userID := uuid.New().String()

		// Get or create the user
		user, err := helper.GetOrCreateDefaultUser(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, userID, user.ID)
		assert.Contains(t, user.Email, userID)
		assert.Equal(t, "Inactive", user.Status) // Placeholder users are inactive
	})

	t.Run("GetOrCreateDefaultUser_EmptyUserID", func(t *testing.T) {
		user, err := helper.GetOrCreateDefaultUser(ctx, "")
		require.NoError(t, err)
		assert.Nil(t, user)
	})
}