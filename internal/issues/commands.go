package issues

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mbolli/tickit/internal/shared"
)

// Commands

type CreateIssueCommand struct {
	Title       string
	Description string
	Assignee    string
	Reporter    string
}

type UpdateIssueCommand struct {
	ID          string
	Title       *string
	Description *string
	Status      *string
	Assignee    *string
}

type DeleteIssueCommand struct {
	ID string
}

// CommandHandler for issues
// (Stub, to be implemented with storage/event sourcing)
type IssueCommandHandler struct {
	eventBus        *shared.EventBusWrapper
	eventStore      *shared.EventStore
	projectionStore *IssueProjectionStore
}

func NewIssueCommandHandler(eventBus *shared.EventBusWrapper, eventStore *shared.EventStore, projectionStore *IssueProjectionStore) *IssueCommandHandler {
	return &IssueCommandHandler{eventBus: eventBus, eventStore: eventStore, projectionStore: projectionStore}
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

func (h *IssueCommandHandler) HandleCreateIssue(ctx context.Context, cmd CreateIssueCommand) (*Issue, error) {
	status := IssueStatusToDo // default
	if !isValidIssueStatus(status) {
		return nil, fmt.Errorf("invalid issue status: %s", status)
	}
	issue := &Issue{
		ID:          uuid.New().String(),
		Title:       cmd.Title,
		Description: cmd.Description,
		Status:      status,
		Assignee:    cmd.Assignee,
		Reporter:    cmd.Reporter,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	event := IssueCreatedEvent{
		Issue:     issue,
		Timestamp: issue.CreatedAt,
	}
	// 1. Persist event to SQLite event store
	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		return nil, err
	}
	// 2. Update projection
	if err := h.projectionStore.ApplyEvent(ctx, event); err != nil {
		return nil, err
	}
	// 3. Publish signal to JetStream for real-time updates
	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		return nil, err
	}
	return issue, nil
}

func (h *IssueCommandHandler) HandleUpdateIssue(ctx context.Context, cmd UpdateIssueCommand) error {
	status := derefString(cmd.Status)
	if status != "" && !isValidIssueStatus(status) {
		return fmt.Errorf("invalid issue status: %s", status)
	}
	issue := &Issue{
		ID:          cmd.ID,
		Title:       derefString(cmd.Title),
		Description: derefString(cmd.Description),
		Status:      status,
		Assignee:    derefString(cmd.Assignee),
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
