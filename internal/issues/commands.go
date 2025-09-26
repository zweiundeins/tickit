package issues

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/internal/shared"
)

// Commands

type CreateIssueCommand struct {
	Title       string
	Description string
	Assignee    string
	Reporter    string
	ProjectID   string
	Priority    string
	Number      int // Per-project incrementing number
}

type UpdateIssueCommand struct {
	ID          string
	Title       *string
	Description *string
	Status      *string
	Assignee    *string
	ProjectID   *string
	Priority    *string
}

type DeleteIssueCommand struct {
	ID string
}

type PatchIssueCommand struct {
	ID          string
	Title       *string
	Description *string
	Status      *string
	Priority    *string
	Assignee    *string
	Reporter    *string
	ProjectID   *string
}

// UserValidator interface for validating user assignments
type UserValidator interface {
	ValidateIssueAssignment(ctx context.Context, userID, projectID string) error
}

// Ensure IssueAssignmentValidator from users package implements UserValidator
var _ UserValidator = (*IssueAssignmentValidatorAdapter)(nil)

// IssueAssignmentValidatorAdapter adapts the users.IssueAssignmentValidator to the UserValidator interface
type IssueAssignmentValidatorAdapter struct {
	validator interface {
		ValidateIssueAssignment(ctx context.Context, userID, projectID string) error
	}
}

func NewIssueAssignmentValidatorAdapter(validator interface {
	ValidateIssueAssignment(ctx context.Context, userID, projectID string) error
}) *IssueAssignmentValidatorAdapter {
	return &IssueAssignmentValidatorAdapter{validator: validator}
}

func (a *IssueAssignmentValidatorAdapter) ValidateIssueAssignment(ctx context.Context, userID, projectID string) error {
	return a.validator.ValidateIssueAssignment(ctx, userID, projectID)
}

// CommandHandler for issues
// (Stub, to be implemented with storage/event sourcing)
type IssueCommandHandler struct {
	eventBus        shared.EventPublisher
	eventStore      *shared.EventStore
	projectionStore *IssueProjectionStore
	userValidator   UserValidator
}

func NewIssueCommandHandler(eventBus shared.EventPublisher, eventStore *shared.EventStore, projectionStore *IssueProjectionStore) *IssueCommandHandler {
	return &IssueCommandHandler{eventBus: eventBus, eventStore: eventStore, projectionStore: projectionStore}
}

func NewIssueCommandHandlerWithValidator(eventBus shared.EventPublisher, eventStore *shared.EventStore, projectionStore *IssueProjectionStore, userValidator UserValidator) *IssueCommandHandler {
	return &IssueCommandHandler{eventBus: eventBus, eventStore: eventStore, projectionStore: projectionStore, userValidator: userValidator}
}

// Allowed issue statuses
var allowedIssueStatuses = map[string]struct{}{
	IssueStatusToDo:       {},
	IssueStatusInProgress: {},
	IssueStatusDone:       {},
	IssueStatusBacklog:    {},
	IssueStatusSelected:   {},
	IssueStatusReview:     {},
	IssueStatusBlocked:    {},
}

func isValidIssueStatus(status string) bool {
	_, ok := allowedIssueStatuses[status]
	return ok
}

// Allowed issue priorities
var allowedIssuePriorities = map[string]struct{}{
	"Low":      {},
	"Medium":   {},
	"High":     {},
	"Critical": {},
}

func isValidIssuePriority(priority string) bool {
	_, ok := allowedIssuePriorities[priority]
	return ok
}

func (h *IssueCommandHandler) HandleCreateIssue(ctx context.Context, cmd CreateIssueCommand) error {
	status := IssueStatusToDo // default
	if !isValidIssueStatus(status) {
		return fmt.Errorf("invalid issue status: %s", status)
	}
	priority := cmd.Priority
	if priority == "" {
		priority = "Medium"
	}
	if !isValidIssuePriority(priority) {
		return fmt.Errorf("invalid issue priority: %s", priority)
	}

	// Validate user assignments if validator is available
	if h.userValidator != nil {
		if cmd.Assignee != "" {
			if err := h.userValidator.ValidateIssueAssignment(ctx, cmd.Assignee, cmd.ProjectID); err != nil {
				return fmt.Errorf("invalid assignee: %w", err)
			}
		}
		if cmd.Reporter != "" {
			if err := h.userValidator.ValidateIssueAssignment(ctx, cmd.Reporter, cmd.ProjectID); err != nil {
				return fmt.Errorf("invalid reporter: %w", err)
			}
		}
	}
	issue := &Issue{
		ID:          uuid.New().String(),
		Number:      cmd.Number,
		Title:       cmd.Title,
		Description: cmd.Description,
		Status:      status,
		Priority:    priority,
		Assignee:    cmd.Assignee,
		Reporter:    cmd.Reporter,
		ProjectID:   cmd.ProjectID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	event := IssueCreatedEvent{
		Issue:     issue,
		Timestamp: issue.CreatedAt,
	}
	// 1. Persist event to SQLite event store
	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		return err
	}
	// 2. Update projection
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return err
	}
	// 3. Publish signal to JetStream for real-time updates
	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		return err
	}
	return nil
}

func (h *IssueCommandHandler) HandleUpdateIssue(ctx context.Context, cmd UpdateIssueCommand) error {
	status := derefString(cmd.Status)
	if status != "" && !isValidIssueStatus(status) {
		return fmt.Errorf("invalid issue status: %s", status)
	}
	priority := derefString(cmd.Priority)
	if priority != "" && !isValidIssuePriority(priority) {
		return fmt.Errorf("invalid issue priority: %s", priority)
	}

	// Validate user assignments if validator is available
	if h.userValidator != nil {
		assignee := derefString(cmd.Assignee)
		if assignee != "" {
			projectID := derefString(cmd.ProjectID)
			if projectID == "" {
				// Get current issue to find project ID
				issue, err := h.projectionStore.GetIssue(ctx, cmd.ID)
				if err != nil {
					return fmt.Errorf("failed to get issue for validation: %w", err)
				}
				if issue != nil {
					projectID = issue.ProjectID
				}
			}
			if projectID != "" {
				if err := h.userValidator.ValidateIssueAssignment(ctx, assignee, projectID); err != nil {
					return fmt.Errorf("invalid assignee: %w", err)
				}
			}
		}
	}
	issue := &Issue{
		ID:          cmd.ID,
		Title:       derefString(cmd.Title),
		Description: derefString(cmd.Description),
		Status:      status,
		Priority:    priority,
		Assignee:    derefString(cmd.Assignee),
		ProjectID:   derefString(cmd.ProjectID),
		UpdatedAt:   time.Now(),
	}
	event := IssueUpdatedEvent{
		Issue:     issue,
		Timestamp: issue.UpdatedAt,
	}
	// 1. Persist event to SQLite event store
	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		return err
	}
	// 2. Update projection
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return err
	}
	// 3. Publish signal to JetStream for real-time updates
	return h.eventBus.PublishEvent(ctx, event)
}

func (h *IssueCommandHandler) HandleDeleteIssue(ctx context.Context, cmd DeleteIssueCommand) error {
	event := IssueDeletedEvent{
		ID:        cmd.ID,
		Timestamp: time.Now(),
	}
	// 1. Persist event to SQLite event store
	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		return err
	}
	// 2. Update projection
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return err
	}
	// 3. Publish signal to JetStream for real-time updates
	return h.eventBus.PublishEvent(ctx, event)
}

func (h *IssueCommandHandler) HandlePatchIssue(ctx context.Context, cmd PatchIssueCommand) error {
	// Get the current issue from the projection store
	issue, err := h.projectionStore.GetIssue(ctx, cmd.ID)
	if err != nil {
		return err
	}
	if issue == nil {
		return fmt.Errorf("issue not found")
	}

	changes := make(map[string]interface{})

	// Patch only provided fields and record changes
	if cmd.Title != nil {
		issue.Title = *cmd.Title
		changes["Title"] = *cmd.Title
	}
	if cmd.Description != nil {
		issue.Description = *cmd.Description
		changes["Description"] = *cmd.Description
	}
	if cmd.Status != nil {
		if !isValidIssueStatus(*cmd.Status) {
			return fmt.Errorf("invalid issue status: %s", *cmd.Status)
		}
		issue.Status = *cmd.Status
		changes["Status"] = *cmd.Status
	}
	if cmd.Priority != nil {
		if !isValidIssuePriority(*cmd.Priority) {
			return fmt.Errorf("invalid issue priority: %s", *cmd.Priority)
		}
		issue.Priority = *cmd.Priority
		changes["Priority"] = *cmd.Priority
	}
	if cmd.Assignee != nil {
		// Validate assignee if validator is available
		if h.userValidator != nil && *cmd.Assignee != "" {
			if err := h.userValidator.ValidateIssueAssignment(ctx, *cmd.Assignee, issue.ProjectID); err != nil {
				return fmt.Errorf("invalid assignee: %w", err)
			}
		}
		issue.Assignee = *cmd.Assignee
		changes["Assignee"] = *cmd.Assignee
	}
	if cmd.Reporter != nil {
		// Validate reporter if validator is available
		if h.userValidator != nil && *cmd.Reporter != "" {
			if err := h.userValidator.ValidateIssueAssignment(ctx, *cmd.Reporter, issue.ProjectID); err != nil {
				return fmt.Errorf("invalid reporter: %w", err)
			}
		}
		issue.Reporter = *cmd.Reporter
		changes["Reporter"] = *cmd.Reporter
	}
	if cmd.ProjectID != nil {
		issue.ProjectID = *cmd.ProjectID
		changes["ProjectID"] = *cmd.ProjectID
	}

	// Ignore noop
	if len(changes) == 0 {
		return nil
	}

	// Create the event for patching the issue
	event := IssuePatchedEvent{
		ID:        issue.ID,
		Changes:   changes,
		Timestamp: time.Now(),
	}

	logger.Info().Str("id", event.ID).Interface("changes", event.Changes).Msg("🔧 IssuePatched event created")

	// 1. Persist event to SQLite event store
	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		return err
	}
	// 2. Update projection
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return err
	}
	// 3. Publish signal to JetStream for real-time updates
	logger.Info().Str("id", event.ID).Msg("🚀 Publishing IssuePatched event to event bus")
	return h.eventBus.PublishEvent(ctx, event)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (c CreateIssueCommand) GetCommandType() string { return "CreateIssue" }

var _ shared.Command = (*CreateIssueCommand)(nil)

func (c UpdateIssueCommand) GetCommandType() string { return "UpdateIssue" }

var _ shared.Command = (*UpdateIssueCommand)(nil)

func (c DeleteIssueCommand) GetCommandType() string { return "DeleteIssue" }

var _ shared.Command = (*DeleteIssueCommand)(nil)

func (c PatchIssueCommand) GetCommandType() string { return "PatchIssue" }

var _ shared.Command = (*PatchIssueCommand)(nil)
