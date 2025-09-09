package projects

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/mbolli/tickit/internal/shared"
)

var slugPattern = regexp.MustCompile(`^[A-Z]{2,6}$`)

func isValidSlug(slug string) bool {
	return slugPattern.MatchString(slug)
}

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

func (s *ProjectService) CreateProject(ctx context.Context, name, slug, description string) (string, error) {
	return s.CreateProjectWithCreator(ctx, name, slug, description, "")
}

func (s *ProjectService) CreateProjectWithCreator(ctx context.Context, name, slug, description, createdBy string) (string, error) {
	slug = strings.ToUpper(slug)
	if !isValidSlug(slug) {
		return "", fmt.Errorf("project slug must be 2-6 uppercase letters (A-Z)")
	}
	id := uuid.New().String()
	cmd := CreateProjectCommand{
		ID:          id,
		Name:        name,
		Slug:        slug,
		Description: description,
		CreatedBy:   createdBy,
	}
	if err := s.commandBus.Execute(ctx, cmd); err != nil {
		return "", err
	}
	return id, nil
}

func (s *ProjectService) UpdateProject(ctx context.Context, id string, name, slug, description *string, archived *bool) error {
	if slug != nil {
		*slug = strings.ToUpper(*slug)
		if !isValidSlug(*slug) {
			return fmt.Errorf("project slug must be 2-6 uppercase letters (A-Z)")
		}
	}
	cmd := UpdateProjectCommand{
		ID:          id,
		Name:        name,
		Slug:        slug,
		Description: description,
		Archived:    archived,
	}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *ProjectService) DeleteProject(ctx context.Context, id string) error {
	cmd := DeleteProjectCommand{ID: id}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *ProjectService) ArchiveProject(ctx context.Context, id string) error {
	archived := true
	cmd := UpdateProjectCommand{
		ID:       id,
		Archived: &archived,
	}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *ProjectService) UnarchiveProject(ctx context.Context, id string) error {
	archived := false
	cmd := UpdateProjectCommand{
		ID:       id,
		Archived: &archived,
	}
	return s.commandBus.Execute(ctx, cmd)
}

func (s *ProjectService) PatchProject(ctx context.Context, input PatchProjectInput) error {
	cmd := PatchProjectCommand{
		ID:          input.ID,
		Name:        input.Name,
		Slug:        input.Slug,
		Description: input.Description,
		Archived:    input.Archived,
	}
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

func (s *ProjectService) GetProjectBySlug(ctx context.Context, slug string) (*Project, error) {
	projects, err := s.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	for _, project := range projects {
		if project.Slug == slug {
			return project, nil
		}
	}
	return nil, nil
}
