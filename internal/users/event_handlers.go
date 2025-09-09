package users

import (
	"context"
	"log/slog"

	"github.com/mbolli/tickit/internal/shared"
)

// UserEventHandler handles user-related events for cross-module integration
// Following the same pattern as other event handlers in the system
type UserEventHandler struct {
	projectionStore UserProjectionStore
}

// NewUserEventHandler creates a new UserEventHandler instance
func NewUserEventHandler(projectionStore UserProjectionStore) *UserEventHandler {
	return &UserEventHandler{
		projectionStore: projectionStore,
	}
}

// HandleUserRegistered handles user registration events
func (h *UserEventHandler) HandleUserRegistered(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserRegisteredEvent)
	if !ok {
		return nil
	}

	slog.Info("handling user registered event",
		slog.String("user_id", e.User.ID),
		slog.String("email", e.User.Email),
	)

	// Apply the event to the projection store
	return h.projectionStore.ApplyEvent(ctx, event)
}

// HandleUserEmailVerified handles email verification events
func (h *UserEventHandler) HandleUserEmailVerified(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserEmailVerifiedEvent)
	if !ok {
		return nil
	}

	slog.Info("handling user email verified event",
		slog.String("user_id", e.UserID),
		slog.String("email", e.Email),
	)

	// Apply the event to the projection store
	return h.projectionStore.ApplyEvent(ctx, event)
}

// HandleUserLoggedIn handles user login events
func (h *UserEventHandler) HandleUserLoggedIn(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserLoggedInEvent)
	if !ok {
		return nil
	}

	slog.Info("handling user logged in event",
		slog.String("user_id", e.UserID),
		slog.String("session_id", e.SessionID),
		slog.String("method", e.Method),
	)

	// Apply the event to the projection store
	return h.projectionStore.ApplyEvent(ctx, event)
}

// HandleUserLoggedOut handles user logout events
func (h *UserEventHandler) HandleUserLoggedOut(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserLoggedOutEvent)
	if !ok {
		return nil
	}

	slog.Info("handling user logged out event",
		slog.String("user_id", e.UserID),
		slog.String("session_id", e.SessionID),
	)

	// Apply the event to the projection store
	return h.projectionStore.ApplyEvent(ctx, event)
}

// HandleUserProfileUpdated handles user profile update events
func (h *UserEventHandler) HandleUserProfileUpdated(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserProfileUpdatedEvent)
	if !ok {
		return nil
	}

	slog.Info("handling user profile updated event",
		slog.String("user_id", e.UserID),
		slog.String("updated_by", e.UpdatedBy),
	)

	// Apply the event to the projection store
	return h.projectionStore.ApplyEvent(ctx, event)
}

// HandleUserAddedToProject handles user addition to project events
func (h *UserEventHandler) HandleUserAddedToProject(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserAddedToProjectEvent)
	if !ok {
		return nil
	}

	slog.Info("handling user added to project event",
		slog.String("user_id", e.UserID),
		slog.String("project_id", e.ProjectID),
		slog.String("role", e.Role),
	)

	// Apply the event to the projection store
	return h.projectionStore.ApplyEvent(ctx, event)
}

// HandleUserRemovedFromProject handles user removal from project events
func (h *UserEventHandler) HandleUserRemovedFromProject(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserRemovedFromProjectEvent)
	if !ok {
		return nil
	}

	slog.Info("handling user removed from project event",
		slog.String("user_id", e.UserID),
		slog.String("project_id", e.ProjectID),
	)

	// Apply the event to the projection store
	return h.projectionStore.ApplyEvent(ctx, event)
}

// HandleUserRoleAssigned handles user role assignment events
func (h *UserEventHandler) HandleUserRoleAssigned(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserRoleAssignedEvent)
	if !ok {
		return nil
	}

	slog.Info("handling user role assigned event",
		slog.String("user_id", e.UserID),
		slog.String("old_role", e.OldRole),
		slog.String("new_role", e.NewRole),
	)

	// Apply the event to the projection store
	return h.projectionStore.ApplyEvent(ctx, event)
}

// HandleUserMFAEnabled handles MFA enabled events
func (h *UserEventHandler) HandleUserMFAEnabled(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserMFAEnabledEvent)
	if !ok {
		return nil
	}

	slog.Info("handling user MFA enabled event",
		slog.String("user_id", e.UserID),
	)

	// Apply the event to the projection store
	return h.projectionStore.ApplyEvent(ctx, event)
}

// HandleUserMFADisabled handles MFA disabled events
func (h *UserEventHandler) HandleUserMFADisabled(ctx context.Context, event shared.Event) error {
	e, ok := event.(UserMFADisabledEvent)
	if !ok {
		return nil
	}

	slog.Info("handling user MFA disabled event",
		slog.String("user_id", e.UserID),
		slog.String("credential_id", e.CredentialID),
	)

	// Apply the event to the projection store
	return h.projectionStore.ApplyEvent(ctx, event)
}

// HandleOAuthAccountLinked handles OAuth account linking events
func (h *UserEventHandler) HandleOAuthAccountLinked(ctx context.Context, event shared.Event) error {
	e, ok := event.(OAuthAccountLinkedEvent)
	if !ok {
		return nil
	}

	slog.Info("handling OAuth account linked event",
		slog.String("user_id", e.UserID),
		slog.String("provider", e.Provider),
	)

	// Apply the event to the projection store
	return h.projectionStore.ApplyEvent(ctx, event)
}

// HandleOAuthAccountUnlinked handles OAuth account unlinking events
func (h *UserEventHandler) HandleOAuthAccountUnlinked(ctx context.Context, event shared.Event) error {
	e, ok := event.(OAuthAccountUnlinkedEvent)
	if !ok {
		return nil
	}

	slog.Info("handling OAuth account unlinked event",
		slog.String("user_id", e.UserID),
		slog.String("provider", e.Provider),
	)

	// Apply the event to the projection store
	return h.projectionStore.ApplyEvent(ctx, event)
}

// GetEventHandlers returns a map of event handlers for registration with the event bus
// This follows the pattern used in other modules
func GetEventHandlers(projectionStore UserProjectionStore) shared.EventHandlers {
	handler := NewUserEventHandler(projectionStore)

	return shared.EventHandlers{
		"UserRegistered":         handler.HandleUserRegistered,
		"UserEmailVerified":      handler.HandleUserEmailVerified,
		"UserLoggedIn":           handler.HandleUserLoggedIn,
		"UserLoggedOut":          handler.HandleUserLoggedOut,
		"UserProfileUpdated":     handler.HandleUserProfileUpdated,
		"UserAddedToProject":     handler.HandleUserAddedToProject,
		"UserRemovedFromProject": handler.HandleUserRemovedFromProject,
		"UserRoleAssigned":       handler.HandleUserRoleAssigned,
		"UserMFAEnabled":         handler.HandleUserMFAEnabled,
		"UserMFADisabled":        handler.HandleUserMFADisabled,
		"OAuthAccountLinked":     handler.HandleOAuthAccountLinked,
		"OAuthAccountUnlinked":   handler.HandleOAuthAccountUnlinked,
	}
}

// GetAllEventHandlers returns both projection and UI event handlers
// This should be used when initializing the event bus to handle both data persistence and UI updates
func GetAllEventHandlers(projectionStore UserProjectionStore, eventBus EventPublisher, userStore UserStore) shared.EventHandlers {
	// Get projection handlers
	projectionHandlers := GetEventHandlers(projectionStore)
	
	// Get UI event handlers
	uiEventService := NewUIEventService(eventBus, userStore)
	uiHandlers := GetUIEventHandlers(uiEventService)
	
	// Merge both handler maps
	allHandlers := make(shared.EventHandlers)
	
	// Add projection handlers
	for eventType, handler := range projectionHandlers {
		allHandlers[eventType] = handler
	}
	
	// Add UI handlers
	// The UI handlers will be called in addition to projection handlers
	for eventType, handler := range uiHandlers {
		// Create a composite handler that calls both projection and UI handlers
		projectionHandler := projectionHandlers[eventType]
		uiHandler := handler
		
		if projectionHandler != nil {
			// Both handlers exist, create composite
			allHandlers[eventType] = func(ctx context.Context, event shared.Event) error {
				// Call projection handler first
				if err := projectionHandler(ctx, event); err != nil {
					return err
				}
				// Then call UI handler (don't fail if UI handler fails)
				uiHandler(ctx, event)
				return nil
			}
		} else {
			// Only UI handler exists, register it directly
			allHandlers[eventType] = uiHandler
		}
	}
	
	return allHandlers
}
