package issues

import (
	"context"

	"github.com/mbolli/tickit/internal/shared"
)

type IssueService struct {
	commandBus *shared.CommandBus
	queryBus   *shared.QueryBus
}

func NewIssueService(commandBus *shared.CommandBus, queryBus *shared.QueryBus) *IssueService {
	return &IssueService{
		commandBus: commandBus,
		queryBus:   queryBus,
	}
}

func (s *IssueService) CreateIssue(ctx context.Context, title, description, assignee, reporter string) (*Issue, error) {
	cmd := CreateIssueCommand{
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
	cmd := UpdateIssueCommand{
		ID:          id,
		Title:       title,
		Description: description,
		Status:      status,
		Assignee:    assignee,
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

func (s *IssueService) ListIssues(ctx context.Context, status, assignee, reporter *string) ([]*Issue, error) {
	query := ListIssuesQuery{Status: status, Assignee: assignee, Reporter: reporter}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	issues, _ := result.([]*Issue)
	return issues, nil
}
