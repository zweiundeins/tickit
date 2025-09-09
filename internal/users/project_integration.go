package users

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/mbolli/tickit/internal/projects"
	"github.com/mbolli/tickit/internal/shared"
)

// Note: We use the ProjectCreatedEvent from the projects module
// to avoid duplication and ensure consistency

// ProjectCreatedEventHandler handles project creation events to automatically
// add the project creator as a project admin
type ProjectCreatedEventHandler struct {
	userStore UserStore
}

// NewProjectCreatedEventHandler creates a new handler for project creation events
func NewProjectCreatedEventHandler(userStore UserStore) *ProjectCreatedEventHandler {
	return &ProjectCreatedEventHandler{
		userStore: userStore,
	}
}

// Handle processes project creation events and adds the creator as project admin
func (h *ProjectCreatedEventHandler) Handle(ctx context.Context, event shared.Event) error {
	// Check if this is a ProjectCreated event
	if event.GetEventType() != "ProjectCreated" {
		return nil // Not our event type
	}

	// Cast the event to the proper ProjectCreatedEvent from projects module
	projectEvent, ok := event.(projects.ProjectCreatedEvent)
	if !ok {
		// If we can't cast, we'll skip processing for now
		slog.Warn("project created event handler: unable to cast event to projects.ProjectCreatedEvent")
		return nil
	}

	// Extract project ID and creator from the event
	projectID := projectEvent.Project.ID
	createdBy := projectEvent.CreatedBy

	if projectID == "" {
		slog.Warn("project created event handler: empty project ID",
			slog.String("created_by", createdBy))
		return nil
	}

	if createdBy == "" {
		slog.Warn("project created event handler: empty creator user ID",
			slog.String("project_id", projectID))
		return nil
	}

	// Verify the creator user exists
	user, err := h.userStore.GetUser(ctx, createdBy)
	if err != nil {
		slog.Error("project created event handler: failed to get creator user",
			slog.String("project_id", projectID),
			slog.String("created_by", createdBy),
			slog.String("error", err.Error()))
		return fmt.Errorf("failed to get creator user: %w", err)
	}

	if user == nil {
		slog.Warn("project created event handler: creator user not found",
			slog.String("project_id", projectID),
			slog.String("created_by", createdBy))
		return nil
	}

	// Create project membership for the creator as project admin
	membership := &ProjectMembership{
		ID:        uuid.New().String(),
		UserID:    createdBy,
		ProjectID: projectID,
		Role:      ProjectRoleAdmin,
		CreatedAt: time.Now(),
		CreatedBy: createdBy, // Self-assigned
	}

	err = h.userStore.CreateProjectMembership(ctx, membership)
	if err != nil {
		slog.Error("project created event handler: failed to create project membership",
			slog.String("project_id", projectID),
			slog.String("created_by", createdBy),
			slog.String("error", err.Error()))
		return fmt.Errorf("failed to create project membership: %w", err)
	}

	slog.Info("project created event handler: added creator as project admin",
		slog.String("project_id", projectID),
		slog.String("user_id", createdBy),
		slog.String("username", user.Username))

	return nil
}

// IssueAssignmentValidator validates issue assignments against project membership
type IssueAssignmentValidator struct {
	userStore UserStore
}

// NewIssueAssignmentValidator creates a new issue assignment validator
func NewIssueAssignmentValidator(userStore UserStore) *IssueAssignmentValidator {
	return &IssueAssignmentValidator{
		userStore: userStore,
	}
}

// ValidateIssueAssignment checks if a user can be assigned to an issue in a project
func (v *IssueAssignmentValidator) ValidateIssueAssignment(ctx context.Context, userID, projectID string) error {
	if userID == "" {
		return nil // Empty assignment is allowed (unassigned issue)
	}

	// Check if user exists
	user, err := v.userStore.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if user == nil {
		return fmt.Errorf("user not found: %s", userID)
	}

	// Admin users can be assigned to any issue
	if user.Role == RoleAdmin {
		return nil
	}

	// Check if user has access to the project
	hasAccess, err := v.userStore.CheckUserProjectAccess(ctx, userID, projectID)
	if err != nil {
		return fmt.Errorf("failed to check project access: %w", err)
	}

	if !hasAccess {
		return fmt.Errorf("user %s (%s) does not have access to project %s", user.Username, userID, projectID)
	}

	return nil
}

// IssueCreatedEventHandler handles issue creation events to validate assignments
type IssueCreatedEventHandler struct {
	validator *IssueAssignmentValidator
}

// NewIssueCreatedEventHandler creates a new handler for issue creation events
func NewIssueCreatedEventHandler(userStore UserStore) *IssueCreatedEventHandler {
	return &IssueCreatedEventHandler{
		validator: NewIssueAssignmentValidator(userStore),
	}
}

// Handle processes issue creation events to validate assignments
func (h *IssueCreatedEventHandler) Handle(ctx context.Context, event shared.Event) error {
	// Check if this is an IssueCreated event
	if event.GetEventType() != "IssueCreated" {
		return nil // Not our event type
	}

	// For now, we'll handle this as a generic event since we don't have the issue event types defined
	// In a real implementation, you'd want to define proper event types for issues
	slog.Info("issue created event handler: received issue created event",
		slog.String("event_type", event.GetEventType()))

	// Skip validation for now since we don't have proper issue event types
	return nil
}

// IssueUpdatedEventHandler handles issue update events to validate assignments
type IssueUpdatedEventHandler struct {
	validator *IssueAssignmentValidator
}

// NewIssueUpdatedEventHandler creates a new handler for issue update events
func NewIssueUpdatedEventHandler(userStore UserStore) *IssueUpdatedEventHandler {
	return &IssueUpdatedEventHandler{
		validator: NewIssueAssignmentValidator(userStore),
	}
}

// Handle processes issue update events to validate assignments
func (h *IssueUpdatedEventHandler) Handle(ctx context.Context, event shared.Event) error {
	// Check if this is an IssueUpdated event
	if event.GetEventType() != "IssueUpdated" {
		return nil // Not our event type
	}

	// For now, we'll handle this as a generic event since we don't have the issue event types defined
	// In a real implementation, you'd want to define proper event types for issues
	slog.Info("issue updated event handler: received issue updated event",
		slog.String("event_type", event.GetEventType()))

	// Skip validation for now since we don't have proper issue event types
	return nil
}
