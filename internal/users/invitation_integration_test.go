package users

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mbolli/tickit/internal/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// setupTestStore creates a test database and returns the store and cleanup function
func setupTestStore(t *testing.T) (*UserStoreImpl, func()) {
	// Create a temporary database file
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewUserStore(context.Background(), dbPath)
	require.NoError(t, err)

	cleanup := func() {
		// Cleanup is handled by t.TempDir()
		os.Remove(dbPath)
	}

	return store, cleanup
}

func TestInvitationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Setup test database
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Setup command handler
	eventStore := &shared.EventStore{}
	mockEventBus := new(MockEventBus)
	mockEmailService := new(MockEmailService)

	handler := NewUserCommandHandler(mockEventBus, eventStore, store, nil, mockEmailService, nil)

	// Create test users
	inviterID := uuid.New().String()
	inviter := &User{
		ID:            inviterID,
		Email:         "inviter@example.com",
		Username:      "inviter",
		Role:          RoleAdmin,
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, store.CreateUser(ctx, inviter))

	projectID := uuid.New().String()
	inviteeEmail := "invitee@example.com"

	t.Run("complete invitation flow", func(t *testing.T) {
		// Setup email service mock
		mockEmailService.On("SendInvitationEmail", ctx, inviteeEmail, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
		mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("shared.Event")).Return(nil)

		// Step 1: Send invitation
		inviteCmd := InviteUserToProjectCommand{
			ProjectID: projectID,
			Email:     inviteeEmail,
			Role:      ProjectRoleDeveloper,
			InvitedBy: inviterID,
		}

		err := handler.HandleInviteUserToProject(ctx, inviteCmd)
		require.NoError(t, err)

		// Verify invitation was created
		invitations, err := store.GetUserInvitations(ctx, inviteeEmail)
		require.NoError(t, err)
		require.Len(t, invitations, 1)

		invitation := invitations[0]
		assert.Equal(t, projectID, invitation.ProjectID)
		assert.Equal(t, inviteeEmail, invitation.Email)
		assert.Equal(t, ProjectRoleDeveloper, invitation.Role)
		assert.Equal(t, inviterID, invitation.CreatedBy)
		assert.Nil(t, invitation.AcceptedAt)
		assert.Nil(t, invitation.DeclinedAt)
		assert.True(t, invitation.ExpiresAt.After(time.Now()))

		// Step 2: Accept invitation
		acceptCmd := AcceptProjectInvitationCommand{
			Token: invitation.Token,
		}

		err = handler.HandleAcceptProjectInvitation(ctx, acceptCmd)
		require.NoError(t, err)

		// Verify invitation was accepted
		updatedInvitation, err := store.GetInvitationByToken(ctx, invitation.Token)
		require.NoError(t, err)
		assert.NotNil(t, updatedInvitation.AcceptedAt)
		assert.Nil(t, updatedInvitation.DeclinedAt)

		// Verify user was created and added to project
		user, err := store.GetUserByEmail(ctx, inviteeEmail)
		require.NoError(t, err)
		assert.Equal(t, inviteeEmail, user.Email)
		assert.True(t, user.EmailVerified) // Auto-verified for invited users

		membership, err := store.GetProjectMembership(ctx, user.ID, projectID)
		require.NoError(t, err)
		assert.Equal(t, ProjectRoleDeveloper, membership.Role)
		assert.Equal(t, inviterID, membership.CreatedBy)

		// Verify mocks were called
		mockEmailService.AssertExpectations(t)
		mockEventBus.AssertExpectations(t)
	})

	t.Run("decline invitation flow", func(t *testing.T) {
		// Setup new invitation
		inviteeEmail2 := "invitee2@example.com"
		mockEmailService.On("SendInvitationEmail", ctx, inviteeEmail2, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
		mockEventBus.On("PublishEvent", ctx, mock.AnythingOfType("shared.Event")).Return(nil)

		inviteCmd := InviteUserToProjectCommand{
			ProjectID: projectID,
			Email:     inviteeEmail2,
			Role:      ProjectRoleViewer,
			InvitedBy: inviterID,
		}

		err := handler.HandleInviteUserToProject(ctx, inviteCmd)
		require.NoError(t, err)

		// Get the invitation
		invitations, err := store.GetUserInvitations(ctx, inviteeEmail2)
		require.NoError(t, err)
		require.Len(t, invitations, 1)
		invitation := invitations[0]

		// Decline invitation
		declineCmd := DeclineProjectInvitationCommand{
			Token: invitation.Token,
		}

		err = handler.HandleDeclineProjectInvitation(ctx, declineCmd)
		require.NoError(t, err)

		// Verify invitation was declined
		updatedInvitation, err := store.GetInvitationByToken(ctx, invitation.Token)
		require.NoError(t, err)
		assert.Nil(t, updatedInvitation.AcceptedAt)
		assert.NotNil(t, updatedInvitation.DeclinedAt)

		// Verify no user was created or added to project
		user, err := store.GetUserByEmail(ctx, inviteeEmail2)
		assert.Nil(t, user) // User should not exist

		// Verify mocks were called
		mockEmailService.AssertExpectations(t)
		mockEventBus.AssertExpectations(t)
	})
}

func TestInvitationValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Setup test database
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Setup command handler
	eventStore := &shared.EventStore{}
	handler := NewUserCommandHandler(nil, eventStore, store, nil, nil, nil)

	// Create test users
	adminID := uuid.New().String()
	admin := &User{
		ID:            adminID,
		Email:         "admin@example.com",
		Username:      "admin",
		Role:          RoleAdmin,
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, store.CreateUser(ctx, admin))

	regularUserID := uuid.New().String()
	regularUser := &User{
		ID:            regularUserID,
		Email:         "user@example.com",
		Username:      "user",
		Role:          RoleViewer,
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, store.CreateUser(ctx, regularUser))

	projectID := uuid.New().String()

	t.Run("non-admin cannot invite without project admin role", func(t *testing.T) {
		inviteCmd := InviteUserToProjectCommand{
			ProjectID: projectID,
			Email:     "newuser@example.com",
			Role:      ProjectRoleDeveloper,
			InvitedBy: regularUserID,
		}

		err := handler.HandleInviteUserToProject(ctx, inviteCmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "insufficient permissions")
	})

	t.Run("admin can invite to any project", func(t *testing.T) {
		inviteCmd := InviteUserToProjectCommand{
			ProjectID: projectID,
			Email:     "newuser@example.com",
			Role:      ProjectRoleDeveloper,
			InvitedBy: adminID,
		}

		err := handler.HandleInviteUserToProject(ctx, inviteCmd)
		assert.NoError(t, err)
	})

	t.Run("cannot invite user who is already a member", func(t *testing.T) {
		// First create a user and add them to the project
		existingUserID := uuid.New().String()
		existingUser := &User{
			ID:            existingUserID,
			Email:         "existing@example.com",
			Username:      "existing",
			Role:          RoleViewer,
			Status:        "Active",
			EmailVerified: true,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		require.NoError(t, store.CreateUser(ctx, existingUser))

		membership := &ProjectMembership{
			ID:        uuid.New().String(),
			UserID:    existingUserID,
			ProjectID: projectID,
			Role:      ProjectRoleViewer,
			CreatedAt: time.Now(),
			CreatedBy: adminID,
		}
		require.NoError(t, store.CreateProjectMembership(ctx, membership))

		// Try to invite the existing member
		inviteCmd := InviteUserToProjectCommand{
			ProjectID: projectID,
			Email:     "existing@example.com",
			Role:      ProjectRoleDeveloper,
			InvitedBy: adminID,
		}

		err := handler.HandleInviteUserToProject(ctx, inviteCmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already a member")
	})
}
