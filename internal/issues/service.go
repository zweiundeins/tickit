package issues

import (
	"context"

	"github.com/mbolli/tickit/internal/projects"
	"github.com/mbolli/tickit/internal/shared"
)

type IssueService struct {
	commandBus      *shared.CommandBus
	queryBus        *shared.QueryBus
	projectService  *projects.ProjectService
	projectionStore *IssueProjectionStore
}

func NewIssueService(commandBus *shared.CommandBus, queryBus *shared.QueryBus, projectService *projects.ProjectService, projectionStore *IssueProjectionStore) *IssueService {
	return &IssueService{
		commandBus:      commandBus,
		queryBus:        queryBus,
		projectService:  projectService,
		projectionStore: projectionStore,
	}
}

func (s *IssueService) CreateIssue(ctx context.Context, title, description, assignee, reporter, projectID, priority string) (*Issue, error) {
	// Get next issue number for this project
	number, err := s.getNextIssueNumber(ctx, projectID)
	if err != nil {
		return nil, err
	}

	cmd := CreateIssueCommand{
		Title:       title,
		Description: description,
		Assignee:    assignee,
		Reporter:    reporter,
		ProjectID:   projectID,
		Priority:    priority,
		Number:      number,
	}
	if err := s.commandBus.Execute(ctx, cmd); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *IssueService) getProjectByID(ctx context.Context, projectID string) (*projects.Project, error) {
	return s.projectService.GetProjectByID(ctx, projectID)
}

func (s *IssueService) getNextIssueNumber(ctx context.Context, projectID string) (int, error) {
	return s.projectionStore.GetNextIssueNumber(ctx, projectID)
}

func (s *IssueService) UpdateIssue(ctx context.Context, id string, title, description, status, assignee, projectID, priority *string) error {
	cmd := UpdateIssueCommand{
		ID:          id,
		Title:       title,
		Description: description,
		Status:      status,
		Assignee:    assignee,
		ProjectID:   projectID,
		Priority:    priority,
	}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *IssueService) DeleteIssue(ctx context.Context, id string) error {
	cmd := DeleteIssueCommand{ID: id}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *IssueService) GetIssueByID(ctx context.Context, id string) (*Issue, error) {
	query := GetIssueByIDQuery{ID: id}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	issue, _ := result.(*Issue)
	return issue, nil
}

func (s *IssueService) ListIssues(ctx context.Context, query ListIssuesQuery) ([]*Issue, error) {
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	issues, _ := result.([]*Issue)
	return issues, nil
}

func (s *IssueService) PatchIssue(ctx context.Context, input PatchIssueInput) (*Issue, error) {
	cmd := PatchIssueCommand{
		ID:          input.ID,
		Title:       input.Title,
		Description: input.Description,
		Status:      input.Status,
		Priority:    input.Priority,
		Assignee:    input.Assignee,
		Reporter:    input.Reporter,
		ProjectID:   input.ProjectID,
	}
	if err := s.commandBus.Execute(ctx, cmd); err != nil {
		return nil, err
	}
	return s.GetIssueByID(ctx, input.ID)
}

func (s *IssueService) GetIssueByKey(ctx context.Context, projectSlug string, number int) (*Issue, error) {
	// Look up the project by slug
	project, err := s.projectService.GetProjectBySlug(ctx, projectSlug)
	if err != nil || project == nil {
		return nil, err
	}
	// Use the efficient projection store method
	return s.projectionStore.GetIssueByKey(ctx, project.ID, number)
}
