package issues

import (
	"context"

	"github.com/mbolli/tickit/internal/shared"
)

// Queries

type GetIssueByIDQuery struct {
	ID string
}

func (q GetIssueByIDQuery) GetQueryType() string { return "GetIssueByID" }

var _ shared.Query = (*GetIssueByIDQuery)(nil)

type ListIssuesQuery struct {
	Status   *string
	Assignee *string
	Reporter *string
}

func (q ListIssuesQuery) GetQueryType() string { return "ListIssues" }

var _ shared.Query = (*ListIssuesQuery)(nil)

// QueryHandler for issues
// (Stub, to be implemented with storage/event sourcing)
type IssueQueryHandler struct {
	projectionStore *IssueProjectionStore
}

func NewIssueQueryHandler(projectionStore *IssueProjectionStore) *IssueQueryHandler {
	return &IssueQueryHandler{projectionStore: projectionStore}
}

func (h *IssueQueryHandler) HandleGetIssueByID(ctx context.Context, query GetIssueByIDQuery) (*Issue, error) {
	return h.projectionStore.GetIssue(ctx, query.ID)
}

func (h *IssueQueryHandler) HandleListIssues(ctx context.Context, query ListIssuesQuery) ([]*Issue, error) {
	issues, err := h.projectionStore.ListIssues(ctx)
	if err != nil {
		return nil, err
	}
	var result []*Issue
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
