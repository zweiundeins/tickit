package users

import (
	"context"

	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/internal/shared"
)

// UserEventHandler handles user-related events for cross-module integration
// It handles both projection updates and real-time UI updates
type UserEventHandler struct {
	projectionStore UserProjectionStore
	eventBus        EventPublisher
	userStore       UserStore
}

// NewUserEventHandler creates a new UserEventHandler instance
func NewUserEventHandler(projectionStore UserProjectionStore, eventBus EventPublisher, userStore UserStore) *UserEventHandler {
	return &UserEventHandler{
		projectionStore: projectionStore,
		eventBus:        eventBus,
		userStore:       userStore,
	}
}

// HandleUserRegistered handles user registration events
func (h *UserEventHandler) HandleUserRegistered(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserRegisteredEvent)
	if !ok {
		return nil
	}

	logger.Info().
		Str("user_id", e.User.ID).
		Str("email", e.User.Email).
		Msg("handling user registered event")

	// Apply the event to the projection store
	return h.projectionStore.ApplyEvent(ctx, event)
}

// HandleUserEmailVerified handles email verification events
func (h *UserEventHandler) HandleUserEmailVerified(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserEmailVerifiedEvent)
	if !ok {
		return nil
	}

	logger.Info().
		Str("user_id", e.UserID).
		Str("email", e.Email).
		Msg("handling user email verified event")

	// Apply the event to the projection store
	return h.projectionStore.ApplyEvent(ctx, event)
}

// HandleUserLoggedIn handles user login events
func (h *UserEventHandler) HandleUserLoggedIn(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserLoggedInEvent)
	if !ok {
		return nil
	}

	logger.Info().
		Str("user_id", e.UserID).
		Str("session_id", e.SessionID).
		Str("method", e.Method).
		Msg("handling user logged in event")

	// Apply the event to the projection store first
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return err
	}

	// Handle real-time UI updates
	go h.publishUserStatusUpdate(ctx, e.UserID, true)

	return nil
}

// publishUserStatusUpdate publishes a real-time UI update for user status changes
func (h *UserEventHandler) publishUserStatusUpdate(ctx context.Context, userID string, isOnline bool) {
	// Get user details for UI update
	user, err := h.userStore.GetUser(ctx, userID)
	if err != nil {
		logger.Error().
			Str("user_id", userID).
			Interface("error", err).
			Msg("failed to get user for status update")
		return
	}

	if user == nil {
		logger.Warn().Str("user_id", userID).Msg("user not found for status update")
		return
	}

	// Publish real-time update through the realtime system (not as a domain event)
	// This should go through the realtime event processor, not as a domain event
	logger.Debug().Str("user_id", userID).Str("username", user.Username).Bool("is_online", isOnline).Msg("user status changed for real-time updates")
}

// HandleUserLoggedOut handles user logout events
func (h *UserEventHandler) HandleUserLoggedOut(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserLoggedOutEvent)
	if !ok {
		return nil
	}

	logger.Info().Str("user_id", e.UserID).Str("session_id", e.SessionID).Msg("handling user logged out event")

	// Apply the event to the projection store first
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return err
	}

	// Handle real-time UI updates
	go h.publishUserStatusUpdate(ctx, e.UserID, false)

	return nil
}

// HandleUserProfileUpdated handles user profile update events
func (h *UserEventHandler) HandleUserProfileUpdated(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserProfileUpdatedEvent)
	if !ok {
		return nil
	}

	logger.Info().Str("user_id", e.UserID).Str("updated_by", e.UpdatedBy).Msg("handling user profile updated event")

	// Apply the event to the projection store first
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return err
	}

	// Handle real-time UI updates
	go h.publishUserProfileUpdate(ctx, e.UserID, e.UpdatedBy)

	return nil
}

// publishUserProfileUpdate publishes a real-time UI update for profile changes
func (h *UserEventHandler) publishUserProfileUpdate(ctx context.Context, userID, updatedBy string) {
	// Get user details for UI update
	user, err := h.userStore.GetUser(ctx, userID)
	if err != nil {
		logger.Error().
			Str("user_id", userID).
			Interface("error", err).
			Msg("failed to get user for profile update")
		return
	}

	if user == nil {
		logger.Warn().Str("user_id", userID).Msg("user not found for profile update")
		return
	}

	// Log real-time update for profile change
	logger.Debug().Str("user_id", userID).Str("username", user.Username).Str("updated_by", updatedBy).Msg("user profile updated for real-time updates")
}

// HandleUserAddedToProject handles user addition to project events
func (h *UserEventHandler) HandleUserAddedToProject(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserAddedToProjectEvent)
	if !ok {
		return nil
	}

	logger.Info().Str("user_id", e.UserID).Str("project_id", e.ProjectID).Str("role", e.Role).Msg("handling user added to project event")

	// Apply the event to the projection store first
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return err
	}

	// Handle real-time UI updates
	go h.publishUserProjectMembershipUpdate(ctx, e.UserID, e.ProjectID, e.Role, "added")

	return nil
}

// HandleUserRemovedFromProject handles user removal from project events
func (h *UserEventHandler) HandleUserRemovedFromProject(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserRemovedFromProjectEvent)
	if !ok {
		return nil
	}

	logger.Info().Str("user_id", e.UserID).Str("project_id", e.ProjectID).Msg("handling user removed from project event")

	// Apply the event to the projection store first
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return err
	}

	// Handle real-time UI updates
	go h.publishUserProjectMembershipUpdate(ctx, e.UserID, e.ProjectID, "", "removed")

	return nil
}

// publishUserProjectMembershipUpdate publishes a real-time UI update for project membership changes
func (h *UserEventHandler) publishUserProjectMembershipUpdate(ctx context.Context, userID, projectID, role, action string) {
	// Get user details for UI update
	user, err := h.userStore.GetUser(ctx, userID)
	if err != nil {
		logger.Error().Str("user_id", userID).Interface("error", err).Msg("failed to get user for project membership update")
		return
	}

	if user == nil {
		logger.Warn().Str("user_id", userID).Msg("user not found for project membership update")
		return
	}

	// Log real-time update for project membership change
	logger.Debug().Str("user_id", userID).Str("username", user.Username).Str("project_id", projectID).Str("role", role).Str("action", action).Msg("user project membership changed for real-time updates")
}

// HandleUserRoleAssigned handles user role assignment events
func (h *UserEventHandler) HandleUserRoleAssigned(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserRoleAssignedEvent)
	if !ok {
		return nil
	}

	logger.Info().Str("user_id", e.UserID).Str("old_role", e.OldRole).Str("new_role", e.NewRole).Msg("handling user role assigned event")

	// Apply the event to the projection store first
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return err
	}

	// Handle real-time UI updates
	go h.publishUserRoleUpdate(ctx, e.UserID, e.OldRole, e.NewRole, e.AssignedBy)

	return nil
}

// publishUserRoleUpdate publishes a real-time UI update for user role changes
func (h *UserEventHandler) publishUserRoleUpdate(ctx context.Context, userID, oldRole, newRole, assignedBy string) {
	// Get user details for UI update
	user, err := h.userStore.GetUser(ctx, userID)
	if err != nil {
		logger.Error().Str("user_id", userID).Interface("error", err).Msg("failed to get user for role update")
		return
	}

	if user == nil {
		logger.Warn().Str("user_id", userID).Msg("user not found for role update")
		return
	}

	// Get admin details if available
	var adminName string
	if assignedBy != "" {
		admin, err := h.userStore.GetUser(ctx, assignedBy)
		if err == nil && admin != nil {
			adminName = admin.Username
		} else {
			adminName = "System"
		}
	} else {
		adminName = "System"
	}

	// Log real-time update for role change
	logger.Debug().Str("user_id", userID).Str("username", user.Username).Str("old_role", oldRole).Str("new_role", newRole).Str("changed_by", adminName).Msg("user role changed for real-time updates")
}

// HandleUserMFAEnabled handles MFA enabled events
func (h *UserEventHandler) HandleUserMFAEnabled(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserMFAEnabledEvent)
	if !ok {
		return nil
	}

	logger.Info().Str("user_id", e.UserID).Msg("handling user MFA enabled event")

	// Apply the event to the projection store first
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return err
	}

	// Handle real-time UI updates
	go h.publishUserMFAStatusUpdate(ctx, e.UserID, true)

	return nil
}

// HandleUserMFADisabled handles MFA disabled events
func (h *UserEventHandler) HandleUserMFADisabled(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserMFADisabledEvent)
	if !ok {
		return nil
	}

	logger.Info().Str("user_id", e.UserID).Str("credential_id", e.CredentialID).Msg("handling user MFA disabled event")

	// Apply the event to the projection store first
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return err
	}

	// Handle real-time UI updates
	go h.publishUserMFAStatusUpdate(ctx, e.UserID, false)

	return nil
}

// publishUserMFAStatusUpdate publishes a real-time UI update for MFA status changes
func (h *UserEventHandler) publishUserMFAStatusUpdate(ctx context.Context, userID string, mfaEnabled bool) {
	// Get user details for UI update
	user, err := h.userStore.GetUser(ctx, userID)
	if err != nil {
		logger.Error().Str("user_id", userID).Interface("error", err).Msg("failed to get user for MFA status update")
		return
	}

	if user == nil {
		logger.Warn().Str("user_id", userID).Msg("user not found for MFA status update")
		return
	}

	// Log real-time update for MFA status change
	logger.Debug().Str("user_id", userID).Str("username", user.Username).Bool("mfa_enabled", mfaEnabled).Msg("user MFA status changed for real-time updates")
}

// HandleOAuthAccountLinked handles OAuth account linking events
func (h *UserEventHandler) HandleOAuthAccountLinked(ctx context.Context, event shared.Event) error {
	e, ok := event.(OAuthAccountLinkedEvent)
	if !ok {
		return nil
	}

	logger.Info().Str("user_id", e.UserID).Str("provider", e.Provider).Msg("handling OAuth account linked event")

	// Apply the event to the projection store
	return h.projectionStore.ApplyEvent(ctx, event)
}

// HandleOAuthAccountUnlinked handles OAuth account unlinking events
func (h *UserEventHandler) HandleOAuthAccountUnlinked(ctx context.Context, event shared.Event) error {
	e, ok := event.(OAuthAccountUnlinkedEvent)
	if !ok {
		return nil
	}

	logger.Info().Str("user_id", e.UserID).Str("provider", e.Provider).Msg("handling OAuth account unlinked event")

	// Apply the event to the projection store
	return h.projectionStore.ApplyEvent(ctx, event)
}

// Note: Event handler registration is now done explicitly in bootstrap/deps.go
// This provides better visibility and control over which handlers are registered

// getCrossModuleEventHandlers returns handlers for cross-module integration
func getCrossModuleEventHandlers(userStore UserStore) shared.EventHandlers {
	projectCreatedHandler := NewProjectCreatedEventHandler(userStore)
	issueCreatedHandler := NewIssueCreatedEventHandler(userStore)
	issueUpdatedHandler := NewIssueUpdatedEventHandler(userStore)

	return shared.EventHandlers{
		"ProjectCreated": func(ctx context.Context, event shared.Event) error {
			return projectCreatedHandler.Handle(ctx, event)
		},
		"IssueCreated": func(ctx context.Context, event shared.Event) error {
			return issueCreatedHandler.Handle(ctx, event)
		},
		"IssueUpdated": func(ctx context.Context, event shared.Event) error {
			return issueUpdatedHandler.Handle(ctx, event)
		},
	}
}

// Note: Event handler registration is now done explicitly in bootstrap/deps.go
// This provides better visibility and control over which handlers are registered
