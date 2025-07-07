package application

import (
	"context"

	"github.com/mbolli/tickit/internal/cqrs/bus"
	"github.com/mbolli/tickit/internal/cqrs/commands"
	"github.com/mbolli/tickit/internal/cqrs/queries"
	"github.com/mbolli/tickit/internal/models"
)

type IssueService struct {
	commandBus *bus.CommandBus
	queryBus   *bus.QueryBus
}

func NewIssueService(commandBus *bus.CommandBus, queryBus *bus.QueryBus) *IssueService {
	return &IssueService{
		commandBus: commandBus,
		queryBus:   queryBus,
	}
}

func (s *IssueService) CreateIssue(ctx context.Context, title, description, assignee, reporter string) (*models.Issue, error) {
	cmd := commands.CreateIssueCommand{
		Title:       title,
		Description: description,
		Assignee:    assignee,
		Reporter:    reporter,
	}
	if err := s.commandBus.Execute(ctx, cmd); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *IssueService) UpdateIssue(ctx context.Context, id string, title, description, status, assignee *string) error {
	cmd := commands.UpdateIssueCommand{
		ID:          id,
		Title:       title,
		Description: description,
		Status:      status,
		Assignee:    assignee,
	}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *IssueService) DeleteIssue(ctx context.Context, id string) error {
	cmd := commands.DeleteIssueCommand{ID: id}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *IssueService) GetIssueByID(ctx context.Context, id string) (*models.Issue, error) {
	query := queries.GetIssueByIDQuery{ID: id}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	issue, _ := result.(*models.Issue)
	return issue, nil
}

func (s *IssueService) ListIssues(ctx context.Context, status, assignee, reporter *string) ([]*models.Issue, error) {
	query := queries.ListIssuesQuery{Status: status, Assignee: assignee, Reporter: reporter}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	issues, _ := result.([]*models.Issue)
	return issues, nil
}
