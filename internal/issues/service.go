package issues

import (
	"context"
	"fmt"
	"strings"

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
	// Generate key using project slug and counter
	key, err := s.generateIssueKey(ctx, projectID)
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
		Key:         key,
	}
	if err := s.commandBus.Execute(ctx, cmd); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *IssueService) generateIssueKey(ctx context.Context, projectID string) (string, error) {
	// Get project to get the slug
	project, err := s.getProjectByID(ctx, projectID)
	if err != nil {
		return "", err
	}
	if project == nil {
		return "", fmt.Errorf("project not found: %s", projectID)
	}

	// Get next issue number for this project
	nextNumber, err := s.getNextIssueNumber(ctx, projectID)
	if err != nil {
		return "", err
	}

	// Generate key like "PROJ-24242"
	return fmt.Sprintf("%s-%d", strings.ToUpper(project.Slug), nextNumber), nil
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

func (s *IssueService) GetIssueByKey(ctx context.Context, key string) (*Issue, error) {
	query := GetIssueByKeyQuery{Key: key}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	issue, _ := result.(*Issue)
	return issue, nil
}
