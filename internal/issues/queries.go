package issues

import (
	"context"
	"time"

	"github.com/mbolli/tickit/internal/shared"
)

// Queries

type GetIssueByIDQuery struct {
	ID string
}

func (q GetIssueByIDQuery) GetQueryType() string { return "GetIssueByID" }

var _ shared.Query = (*GetIssueByIDQuery)(nil)

type ListIssuesQuery struct {
	Status        *string
	Assignee      *string
	Reporter      *string
	Priority      *string
	ProjectID     *string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	ContainsText  *string
	OrderBy       *string // NEW: column to order by
	OrderDir      *string // NEW: "asc" or "desc"
}

func (q ListIssuesQuery) GetQueryType() string { return "ListIssues" }

var _ shared.Query = (*ListIssuesQuery)(nil)

type ListIssuesFilter struct {
	Status        string `json:"status"`
	Assignee      string `json:"assignee"`
	Reporter      string `json:"reporter"`
	Priority      string `json:"priority"`
	ProjectID     string `json:"project_id"`
	CreatedAfter  string `json:"created_after"`
	CreatedBefore string `json:"created_before"`
	ContainsText  string `json:"contains_text"`
	OrderBy       string `json:"order_by"`  // NEW
	OrderDir      string `json:"order_dir"` // NEW
}

func (f *ListIssuesFilter) ToQuery() ListIssuesQuery {
	var (
		statusPtr, assigneePtr, reporterPtr, priorityPtr, projectIDPtr, containsTextPtr *string
		createdAfterPtr, createdBeforePtr                                               *time.Time
		orderByPtr, orderDirPtr                                                         *string // NEW
	)
	if f.Status != "" {
		statusPtr = &f.Status
	}
	if f.Assignee != "" {
		assigneePtr = &f.Assignee
	}
	if f.Reporter != "" {
		reporterPtr = &f.Reporter
	}
	if f.Priority != "" {
		priorityPtr = &f.Priority
	}
	if f.ProjectID != "" {
		projectIDPtr = &f.ProjectID
	}
	if f.ContainsText != "" {
		containsTextPtr = &f.ContainsText
	}
	if f.CreatedAfter != "" {
		if t, err := time.Parse(time.RFC3339, f.CreatedAfter); err == nil {
			createdAfterPtr = &t
		}
	}
	if f.CreatedBefore != "" {
		if t, err := time.Parse(time.RFC3339, f.CreatedBefore); err == nil {
			createdBeforePtr = &t
		}
	}
	if f.OrderBy != "" {
		orderByPtr = &f.OrderBy
	}
	if f.OrderDir != "" {
		orderDirPtr = &f.OrderDir
	}
	return ListIssuesQuery{
		Status:        statusPtr,
		Assignee:      assigneePtr,
		Reporter:      reporterPtr,
		Priority:      priorityPtr,
		ProjectID:     projectIDPtr,
		ContainsText:  containsTextPtr,
		CreatedAfter:  createdAfterPtr,
		CreatedBefore: createdBeforePtr,
		OrderBy:       orderByPtr,  // NEW
		OrderDir:      orderDirPtr, // NEW
	}
}

type GetIssueByKeyQuery struct {
	Key string
}

func (q GetIssueByKeyQuery) GetQueryType() string { return "GetIssueByKey" }

var _ shared.Query = (*GetIssueByKeyQuery)(nil)

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
	return h.projectionStore.ListIssues(ctx, query)
}

func (h *IssueQueryHandler) HandleGetIssueByKey(ctx context.Context, query GetIssueByKeyQuery) (*Issue, error) {
	// For now, we'll need to list all issues and find by key
	// In a production system, you'd want an index on the key field
	issues, err := h.projectionStore.ListIssues(ctx, ListIssuesQuery{})
	if err != nil {
		return nil, err
	}

	for _, issue := range issues {
		if issue.Key == query.Key {
			return issue, nil
		}
	}

	return nil, nil // Not found
}
