package users

import (
	"context"
	"log/slog"
	"time"

	"github.com/mbolli/tickit/internal/shared"
)

// UIEventService handles the conversion of domain events to UI-specific events
// It listens to domain events and publishes sanitized UI events for real-time updates
type UIEventService struct {
	eventBus  EventPublisher
	userStore UserStore
}

// NewUIEventService creates a new UIEventService instance
func NewUIEventService(eventBus EventPublisher, userStore UserStore) *UIEventService {
	return &UIEventService{
		eventBus:  eventBus,
		userStore: userStore,
	}
}

// HandleUserLoggedIn converts UserLoggedInEvent to UserLoginStatusEvent
func (s *UIEventService) HandleUserLoggedIn(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserLoggedInEvent)
	if !ok {
		return nil
	}

	// Get user details for UI event
	user, err := s.userStore.GetUser(ctx, e.UserID)
	if err != nil {
		slog.Error("failed to get user for UI event", 
			slog.String("user_id", e.UserID), 
			slog.Any("error", err))
		return nil // Don't fail the event processing
	}

	if user == nil {
		slog.Warn("user not found for UI event", slog.String("user_id", e.UserID))
		return nil
	}

	// Create UI event with non-sensitive data
	uiEvent := UserLoginStatusEvent{
		UserID:    e.UserID,
		Username:  user.Username,
		Action:    "login",
		Method:    e.Method,
		Provider:  e.Provider,
		Timestamp: e.Timestamp,
	}

	// Also create user status changed event to update online status
	statusEvent := UserStatusChangedEvent{
		UserID:    e.UserID,
		Username:  user.Username,
		Status:    user.Status,
		IsOnline:  true,
		Timestamp: e.Timestamp,
	}

	// Publish both events asynchronously to avoid deadlocks
	go func() {
		if err := s.eventBus.PublishEvent(context.Background(), uiEvent); err != nil {
			slog.Error("failed to publish UserLoginStatusEvent", slog.Any("error", err))
		}
	}()

	go func() {
		if err := s.eventBus.PublishEvent(context.Background(), statusEvent); err != nil {
			slog.Error("failed to publish UserStatusChangedEvent", slog.Any("error", err))
		}
	}()

	return nil
}

// HandleUserLoggedOut converts UserLoggedOutEvent to UserLoginStatusEvent
func (s *UIEventService) HandleUserLoggedOut(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserLoggedOutEvent)
	if !ok {
		return nil
	}

	// Get user details for UI event
	user, err := s.userStore.GetUser(ctx, e.UserID)
	if err != nil {
		slog.Error("failed to get user for UI event", 
			slog.String("user_id", e.UserID), 
			slog.Any("error", err))
		return nil
	}

	if user == nil {
		slog.Warn("user not found for UI event", slog.String("user_id", e.UserID))
		return nil
	}

	// Create UI events
	uiEvent := UserLoginStatusEvent{
		UserID:    e.UserID,
		Username:  user.Username,
		Action:    "logout",
		Method:    "", // Not applicable for logout
		Provider:  "",
		Timestamp: e.Timestamp,
	}

	statusEvent := UserStatusChangedEvent{
		UserID:    e.UserID,
		Username:  user.Username,
		Status:    user.Status,
		IsOnline:  false,
		Timestamp: e.Timestamp,
	}

	// Publish both events asynchronously to avoid deadlocks
	go func() {
		if err := s.eventBus.PublishEvent(context.Background(), uiEvent); err != nil {
			slog.Error("failed to publish UserLoginStatusEvent", slog.Any("error", err))
		}
	}()

	go func() {
		if err := s.eventBus.PublishEvent(context.Background(), statusEvent); err != nil {
			slog.Error("failed to publish UserStatusChangedEvent", slog.Any("error", err))
		}
	}()

	return nil
}

// HandleUserRoleAssigned converts UserRoleAssignedEvent to UserRoleChangedEvent
func (s *UIEventService) HandleUserRoleAssigned(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserRoleAssignedEvent)
	if !ok {
		return nil
	}

	// Get user details
	user, err := s.userStore.GetUser(ctx, e.UserID)
	if err != nil {
		slog.Error("failed to get user for UI event", 
			slog.String("user_id", e.UserID), 
			slog.Any("error", err))
		return nil
	}

	if user == nil {
		slog.Warn("user not found for UI event", slog.String("user_id", e.UserID))
		return nil
	}

	// Get admin details
	var adminName string
	if e.AssignedBy != "" {
		admin, err := s.userStore.GetUser(ctx, e.AssignedBy)
		if err == nil && admin != nil {
			adminName = admin.Username
		} else {
			adminName = "System"
		}
	} else {
		adminName = "System"
	}

	// Create UI event
	uiEvent := UserRoleChangedEvent{
		UserID:    e.UserID,
		Username:  user.Username,
		OldRole:   e.OldRole,
		NewRole:   e.NewRole,
		ChangedBy: adminName,
		Timestamp: e.Timestamp,
	}

	// Also create admin management event
	adminEvent := AdminUserManagementEvent{
		AdminID:    e.AssignedBy,
		AdminName:  adminName,
		TargetID:   e.UserID,
		TargetName: user.Username,
		Action:     "role_changed",
		Details:    "Role changed from " + e.OldRole + " to " + e.NewRole,
		Timestamp:  e.Timestamp,
	}

	// Publish both events
	if err := s.eventBus.PublishEvent(ctx, uiEvent); err != nil {
		slog.Error("failed to publish UserRoleChangedEvent", slog.Any("error", err))
	}

	if err := s.eventBus.PublishEvent(ctx, adminEvent); err != nil {
		slog.Error("failed to publish AdminUserManagementEvent", slog.Any("error", err))
	}

	return nil
}

// HandleUserDeactivated converts UserDeactivatedEvent to UI events
func (s *UIEventService) HandleUserDeactivated(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserDeactivatedEvent)
	if !ok {
		return nil
	}

	// Get user details
	user, err := s.userStore.GetUser(ctx, e.UserID)
	if err != nil {
		slog.Error("failed to get user for UI event", 
			slog.String("user_id", e.UserID), 
			slog.Any("error", err))
		return nil
	}

	if user == nil {
		slog.Warn("user not found for UI event", slog.String("user_id", e.UserID))
		return nil
	}

	// Get admin details
	var adminName string
	if e.DeactivatedBy != "" {
		admin, err := s.userStore.GetUser(ctx, e.DeactivatedBy)
		if err == nil && admin != nil {
			adminName = admin.Username
		} else {
			adminName = "System"
		}
	} else {
		adminName = "System"
	}

	// Create UI events
	statusEvent := UserStatusChangedEvent{
		UserID:    e.UserID,
		Username:  user.Username,
		Status:    "Inactive",
		IsOnline:  false,
		Timestamp: e.Timestamp,
	}

	adminEvent := AdminUserManagementEvent{
		AdminID:    e.DeactivatedBy,
		AdminName:  adminName,
		TargetID:   e.UserID,
		TargetName: user.Username,
		Action:     "deactivated",
		Details:    "User account deactivated",
		Timestamp:  e.Timestamp,
	}

	// Publish both events
	if err := s.eventBus.PublishEvent(ctx, statusEvent); err != nil {
		slog.Error("failed to publish UserStatusChangedEvent", slog.Any("error", err))
	}

	if err := s.eventBus.PublishEvent(ctx, adminEvent); err != nil {
		slog.Error("failed to publish AdminUserManagementEvent", slog.Any("error", err))
	}

	return nil
}

// HandleUserReactivated converts UserReactivatedEvent to UI events
func (s *UIEventService) HandleUserReactivated(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserReactivatedEvent)
	if !ok {
		return nil
	}

	// Get user details
	user, err := s.userStore.GetUser(ctx, e.UserID)
	if err != nil {
		slog.Error("failed to get user for UI event", 
			slog.String("user_id", e.UserID), 
			slog.Any("error", err))
		return nil
	}

	if user == nil {
		slog.Warn("user not found for UI event", slog.String("user_id", e.UserID))
		return nil
	}

	// Get admin details
	var adminName string
	if e.ReactivatedBy != "" {
		admin, err := s.userStore.GetUser(ctx, e.ReactivatedBy)
		if err == nil && admin != nil {
			adminName = admin.Username
		} else {
			adminName = "System"
		}
	} else {
		adminName = "System"
	}

	// Create UI events
	statusEvent := UserStatusChangedEvent{
		UserID:    e.UserID,
		Username:  user.Username,
		Status:    "Active",
		IsOnline:  false, // They're not online yet, just reactivated
		Timestamp: e.Timestamp,
	}

	adminEvent := AdminUserManagementEvent{
		AdminID:    e.ReactivatedBy,
		AdminName:  adminName,
		TargetID:   e.UserID,
		TargetName: user.Username,
		Action:     "activated",
		Details:    "User account reactivated",
		Timestamp:  e.Timestamp,
	}

	// Publish both events
	if err := s.eventBus.PublishEvent(ctx, statusEvent); err != nil {
		slog.Error("failed to publish UserStatusChangedEvent", slog.Any("error", err))
	}

	if err := s.eventBus.PublishEvent(ctx, adminEvent); err != nil {
		slog.Error("failed to publish AdminUserManagementEvent", slog.Any("error", err))
	}

	return nil
}

// HandleUserMFAEnabled converts UserMFAEnabledEvent to UserMFAStatusChangedEvent
func (s *UIEventService) HandleUserMFAEnabled(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserMFAEnabledEvent)
	if !ok {
		return nil
	}

	// Get user details
	user, err := s.userStore.GetUser(ctx, e.UserID)
	if err != nil {
		slog.Error("failed to get user for UI event", 
			slog.String("user_id", e.UserID), 
			slog.Any("error", err))
		return nil
	}

	if user == nil {
		slog.Warn("user not found for UI event", slog.String("user_id", e.UserID))
		return nil
	}

	// Create UI event
	uiEvent := UserMFAStatusChangedEvent{
		UserID:    e.UserID,
		Username:  user.Username,
		Enabled:   true,
		Method:    "webauthn", // Currently only WebAuthn is supported
		Timestamp: e.Timestamp,
	}

	if err := s.eventBus.PublishEvent(ctx, uiEvent); err != nil {
		slog.Error("failed to publish UserMFAStatusChangedEvent", slog.Any("error", err))
	}

	return nil
}

// HandleUserMFADisabled converts UserMFADisabledEvent to UserMFAStatusChangedEvent
func (s *UIEventService) HandleUserMFADisabled(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserMFADisabledEvent)
	if !ok {
		return nil
	}

	// Get user details
	user, err := s.userStore.GetUser(ctx, e.UserID)
	if err != nil {
		slog.Error("failed to get user for UI event", 
			slog.String("user_id", e.UserID), 
			slog.Any("error", err))
		return nil
	}

	if user == nil {
		slog.Warn("user not found for UI event", slog.String("user_id", e.UserID))
		return nil
	}

	// Create UI event
	uiEvent := UserMFAStatusChangedEvent{
		UserID:    e.UserID,
		Username:  user.Username,
		Enabled:   false,
		Method:    "webauthn",
		Timestamp: e.Timestamp,
	}

	if err := s.eventBus.PublishEvent(ctx, uiEvent); err != nil {
		slog.Error("failed to publish UserMFAStatusChangedEvent", slog.Any("error", err))
	}

	return nil
}

// HandleUserProfileUpdated converts UserProfileUpdatedEvent to UserProfileChangedEvent
func (s *UIEventService) HandleUserProfileUpdated(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserProfileUpdatedEvent)
	if !ok {
		return nil
	}

	// Get user details
	user, err := s.userStore.GetUser(ctx, e.UserID)
	if err != nil {
		slog.Error("failed to get user for UI event", 
			slog.String("user_id", e.UserID), 
			slog.Any("error", err))
		return nil
	}

	if user == nil {
		slog.Warn("user not found for UI event", slog.String("user_id", e.UserID))
		return nil
	}

	// Get updater details
	var updaterName string
	if e.UpdatedBy != "" {
		updater, err := s.userStore.GetUser(ctx, e.UpdatedBy)
		if err == nil && updater != nil {
			updaterName = updater.Username
		} else {
			updaterName = "System"
		}
	} else {
		updaterName = user.Username // Self-update
	}

	// Extract field names from changes map
	var fieldsChanged []string
	for field := range e.Changes {
		// Don't include sensitive fields in UI events
		if field != "password" && field != "password_hash" {
			fieldsChanged = append(fieldsChanged, field)
		}
	}

	// Create UI event
	uiEvent := UserProfileChangedEvent{
		UserID:        e.UserID,
		Username:      user.Username,
		ChangedBy:     updaterName,
		FieldsChanged: fieldsChanged,
		Timestamp:     e.Timestamp,
	}

	if err := s.eventBus.PublishEvent(ctx, uiEvent); err != nil {
		slog.Error("failed to publish UserProfileChangedEvent", slog.Any("error", err))
	}

	return nil
}

// HandleProjectMembershipEvents handles project membership changes
func (s *UIEventService) HandleUserAddedToProject(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserAddedToProjectEvent)
	if !ok {
		return nil
	}

	return s.handleProjectMembershipChange(ctx, e.UserID, e.ProjectID, "added", e.Role, "", e.AddedBy, e.Timestamp)
}

func (s *UIEventService) HandleUserRemovedFromProject(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserRemovedFromProjectEvent)
	if !ok {
		return nil
	}

	return s.handleProjectMembershipChange(ctx, e.UserID, e.ProjectID, "removed", "", "", e.RemovedBy, e.Timestamp)
}

func (s *UIEventService) HandleProjectMembershipUpdated(ctx context.Context, event shared.Event) error {
	e, ok := event.(ProjectMembershipUpdatedEvent)
	if !ok {
		return nil
	}

	return s.handleProjectMembershipChange(ctx, e.UserID, e.ProjectID, "role_changed", e.NewRole, e.OldRole, e.UpdatedBy, e.Timestamp)
}

// Helper method for project membership changes
func (s *UIEventService) handleProjectMembershipChange(ctx context.Context, userID, projectID, action, role, oldRole, changedBy string, timestamp time.Time) error {
	// Get user details
	user, err := s.userStore.GetUser(ctx, userID)
	if err != nil {
		slog.Error("failed to get user for UI event", 
			slog.String("user_id", userID), 
			slog.Any("error", err))
		return nil
	}

	if user == nil {
		slog.Warn("user not found for UI event", slog.String("user_id", userID))
		return nil
	}

	// Get project name (we'll need to add this to the user store or get it from project service)
	// For now, we'll use the project ID as the name
	projectName := projectID // TODO: Get actual project name

	// Get changer details
	var changerName string
	if changedBy != "" {
		changer, err := s.userStore.GetUser(ctx, changedBy)
		if err == nil && changer != nil {
			changerName = changer.Username
		} else {
			changerName = "System"
		}
	} else {
		changerName = "System"
	}

	// Create UI event
	uiEvent := UserProjectMembershipChangedEvent{
		UserID:      userID,
		Username:    user.Username,
		ProjectID:   projectID,
		ProjectName: projectName,
		Action:      action,
		Role:        role,
		OldRole:     oldRole,
		ChangedBy:   changerName,
		Timestamp:   timestamp,
	}

	if err := s.eventBus.PublishEvent(ctx, uiEvent); err != nil {
		slog.Error("failed to publish UserProjectMembershipChangedEvent", slog.Any("error", err))
	}

	return nil
}

// HandleInvitationEvents handles invitation status changes
func (s *UIEventService) HandleUserInvitedToProject(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserInvitedToProjectEvent)
	if !ok {
		return nil
	}

	// Get inviter details
	var inviterName string
	if e.InvitedBy != "" {
		inviter, err := s.userStore.GetUser(ctx, e.InvitedBy)
		if err == nil && inviter != nil {
			inviterName = inviter.Username
		} else {
			inviterName = "System"
		}
	} else {
		inviterName = "System"
	}

	// Get project name (using project ID for now)
	projectName := e.ProjectID // TODO: Get actual project name

	// Create UI event
	uiEvent := UserInvitationStatusEvent{
		InvitationID: e.InvitationID,
		ProjectID:    e.ProjectID,
		ProjectName:  projectName,
		Email:        e.Email,
		Status:       "sent",
		InvitedBy:    inviterName,
		Timestamp:    e.Timestamp,
	}

	if err := s.eventBus.PublishEvent(ctx, uiEvent); err != nil {
		slog.Error("failed to publish UserInvitationStatusEvent", slog.Any("error", err))
	}

	return nil
}

func (s *UIEventService) HandleProjectInvitationAccepted(ctx context.Context, event shared.Event) error {
	e, ok := event.(ProjectInvitationAcceptedEvent)
	if !ok {
		return nil
	}

	// Get project name (using project ID for now)
	projectName := e.ProjectID // TODO: Get actual project name

	// Create UI event
	uiEvent := UserInvitationStatusEvent{
		InvitationID: e.InvitationID,
		ProjectID:    e.ProjectID,
		ProjectName:  projectName,
		Email:        "", // We don't have email in this event, could be enhanced
		Status:       "accepted",
		InvitedBy:    "", // We don't have this info in this event
		Timestamp:    e.Timestamp,
	}

	if err := s.eventBus.PublishEvent(ctx, uiEvent); err != nil {
		slog.Error("failed to publish UserInvitationStatusEvent", slog.Any("error", err))
	}

	return nil
}

func (s *UIEventService) HandleProjectInvitationDeclined(ctx context.Context, event shared.Event) error {
	e, ok := event.(ProjectInvitationDeclinedEvent)
	if !ok {
		return nil
	}

	// Get project name (using project ID for now)
	projectName := e.ProjectID // TODO: Get actual project name

	// Create UI event
	uiEvent := UserInvitationStatusEvent{
		InvitationID: e.InvitationID,
		ProjectID:    e.ProjectID,
		ProjectName:  projectName,
		Email:        e.Email,
		Status:       "declined",
		InvitedBy:    "", // We don't have this info in this event
		Timestamp:    e.Timestamp,
	}

	if err := s.eventBus.PublishEvent(ctx, uiEvent); err != nil {
		slog.Error("failed to publish UserInvitationStatusEvent", slog.Any("error", err))
	}

	return nil
}

// UI Event Handlers - These handle the UI-specific events that are published by this service
// They are typically no-op handlers since these events are meant for external consumption

// HandleUserLoginStatusEvent handles UserLoginStatusEvent (no-op, for external consumption)
func (s *UIEventService) HandleUserLoginStatusEvent(ctx context.Context, event shared.Event) error {
	// This is a UI event published by this service, no additional processing needed
	// External systems (WebSocket, SSE, etc.) will consume this event
	return nil
}

// HandleUserStatusChangedEvent handles UserStatusChangedEvent (no-op, for external consumption)
func (s *UIEventService) HandleUserStatusChangedEvent(ctx context.Context, event shared.Event) error {
	// This is a UI event published by this service, no additional processing needed
	return nil
}

// HandleUserRoleChangedEvent handles UserRoleChangedEvent (no-op, for external consumption)
func (s *UIEventService) HandleUserRoleChangedEvent(ctx context.Context, event shared.Event) error {
	// This is a UI event published by this service, no additional processing needed
	return nil
}

// HandleUserProjectMembershipChangedEvent handles UserProjectMembershipChangedEvent (no-op, for external consumption)
func (s *UIEventService) HandleUserProjectMembershipChangedEvent(ctx context.Context, event shared.Event) error {
	// This is a UI event published by this service, no additional processing needed
	return nil
}

// HandleUserMFAStatusChangedEvent handles UserMFAStatusChangedEvent (no-op, for external consumption)
func (s *UIEventService) HandleUserMFAStatusChangedEvent(ctx context.Context, event shared.Event) error {
	// This is a UI event published by this service, no additional processing needed
	return nil
}

// HandleAdminUserManagementEvent handles AdminUserManagementEvent (no-op, for external consumption)
func (s *UIEventService) HandleAdminUserManagementEvent(ctx context.Context, event shared.Event) error {
	// This is a UI event published by this service, no additional processing needed
	return nil
}

// HandleUserProfileChangedEvent handles UserProfileChangedEvent (no-op, for external consumption)
func (s *UIEventService) HandleUserProfileChangedEvent(ctx context.Context, event shared.Event) error {
	// This is a UI event published by this service, no additional processing needed
	return nil
}

// HandleUserInvitationStatusEvent handles UserInvitationStatusEvent (no-op, for external consumption)
func (s *UIEventService) HandleUserInvitationStatusEvent(ctx context.Context, event shared.Event) error {
	// This is a UI event published by this service, no additional processing needed
	return nil
}

// GetUIEventHandlers returns a map of UI event handlers for registration with the event bus
func GetUIEventHandlers(uiEventService *UIEventService) shared.EventHandlers {
	return shared.EventHandlers{
		// Login/Logout events
		"UserLoggedIn":  uiEventService.HandleUserLoggedIn,
		"UserLoggedOut": uiEventService.HandleUserLoggedOut,

		// UI-specific events (these are published by the UI event service itself)
		"UserLoginStatus":           uiEventService.HandleUserLoginStatusEvent,
		"UserStatusChanged":         uiEventService.HandleUserStatusChangedEvent,
		"UserRoleChanged":           uiEventService.HandleUserRoleChangedEvent,
		"UserProjectMembershipChanged": uiEventService.HandleUserProjectMembershipChangedEvent,
		"UserMFAStatusChanged":      uiEventService.HandleUserMFAStatusChangedEvent,
		"AdminUserManagement":       uiEventService.HandleAdminUserManagementEvent,
		"UserProfileChanged":        uiEventService.HandleUserProfileChangedEvent,
		"UserInvitationStatus":      uiEventService.HandleUserInvitationStatusEvent,

		// Role and status changes
		"UserRoleAssigned": uiEventService.HandleUserRoleAssigned,
		"UserDeactivated":  uiEventService.HandleUserDeactivated,
		"UserReactivated":  uiEventService.HandleUserReactivated,

		// MFA events
		"UserMFAEnabled":  uiEventService.HandleUserMFAEnabled,
		"UserMFADisabled": uiEventService.HandleUserMFADisabled,

		// Profile events
		"UserProfileUpdated": uiEventService.HandleUserProfileUpdated,

		// Project membership events
		"UserAddedToProject":         uiEventService.HandleUserAddedToProject,
		"UserRemovedFromProject":     uiEventService.HandleUserRemovedFromProject,
		"ProjectMembershipUpdated":   uiEventService.HandleProjectMembershipUpdated,

		// Invitation events
		"UserInvitedToProject":        uiEventService.HandleUserInvitedToProject,
		"ProjectInvitationAccepted":   uiEventService.HandleProjectInvitationAccepted,
		"ProjectInvitationDeclined":   uiEventService.HandleProjectInvitationDeclined,
	}
}