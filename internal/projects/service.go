package projects

import (
	"context"

	"github.com/mbolli/tickit/internal/shared"
)

type ProjectService struct {
	commandBus *shared.CommandBus
	queryBus   *shared.QueryBus
}

func NewProjectService(commandBus *shared.CommandBus, queryBus *shared.QueryBus) *ProjectService {
	return &ProjectService{
		commandBus: commandBus,
		queryBus:   queryBus,
	}
}

func (s *ProjectService) CreateProject(ctx context.Context, name, slug, description string) (*Project, error) {
	cmd := CreateProjectCommand{
		Name:        name,
		Slug:        slug,
		Description: description,
	}
	if err := s.commandBus.Execute(ctx, cmd); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *ProjectService) UpdateProject(ctx context.Context, id string, name, slug, description *string) error {
	cmd := UpdateProjectCommand{
		ID:          id,
		Name:        name,
		Slug:        slug,
		Description: description,
	}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *ProjectService) DeleteProject(ctx context.Context, id string) error {
	cmd := DeleteProjectCommand{ID: id}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *ProjectService) GetProjectByID(ctx context.Context, id string) (*Project, error) {
	query := GetProjectByIDQuery{ID: id}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	project, _ := result.(*Project)
	return project, nil
}

func (s *ProjectService) ListProjects(ctx context.Context) ([]*Project, error) {
	query := ListProjectsQuery{}
	result, err := s.queryBus.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	projects, _ := result.([]*Project)
	return projects, nil
}
