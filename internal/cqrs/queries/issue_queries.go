package queries

import (
	"context"

	"github.com/mbolli/tickit/internal/cqrs/events"
	"github.com/mbolli/tickit/internal/models"
)

// Queries

type GetIssueByIDQuery struct {
	ID string
}

type ListIssuesQuery struct {
	Status   *string
	Assignee *string
	Reporter *string
}

// QueryHandler for issues
// (Stub, to be implemented with storage/event sourcing)
type IssueQueryHandler struct {
	projectionStore *events.IssueProjectionStore
}

func NewIssueQueryHandler(projectionStore *events.IssueProjectionStore) *IssueQueryHandler {
	return &IssueQueryHandler{projectionStore: projectionStore}
}

func (h *IssueQueryHandler) HandleGetIssueByID(ctx context.Context, query GetIssueByIDQuery) (*models.Issue, error) {
	return h.projectionStore.GetIssue(ctx, query.ID)
}

func (h *IssueQueryHandler) HandleListIssues(ctx context.Context, query ListIssuesQuery) ([]*models.Issue, error) {
	issues, err := h.projectionStore.ListIssues(ctx)
	if err != nil {
		return nil, err
	}
	var result []*models.Issue
	for _, issue := range issues {
		if query.Status != nil && issue.Status != *query.Status {
			continue
		}
		if query.Assignee != nil && issue.Assignee != *query.Assignee {
			continue
		}
		if query.Reporter != nil && issue.Reporter != *query.Reporter {
			continue
		}
		result = append(result, issue)
	}
	return result, nil
}
