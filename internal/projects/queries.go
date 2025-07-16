package projects

import (
	"context"

	"github.com/mbolli/tickit/internal/shared"
)

// Queries

type GetProjectByIDQuery struct {
	ID string
}

func (q GetProjectByIDQuery) GetQueryType() string { return "GetProjectByID" }

var _ shared.Query = (*GetProjectByIDQuery)(nil)

type ListProjectsQuery struct{}

func (q ListProjectsQuery) GetQueryType() string { return "ListProjects" }

var _ shared.Query = (*ListProjectsQuery)(nil)

// QueryHandler for projects
// (Stub, to be implemented with storage/event sourcing)
type ProjectQueryHandler struct {
	projectionStore *ProjectProjectionStore
}

func NewProjectQueryHandler(projectionStore *ProjectProjectionStore) *ProjectQueryHandler {
	return &ProjectQueryHandler{projectionStore: projectionStore}
}

func (h *ProjectQueryHandler) HandleGetProjectByID(ctx context.Context, query GetProjectByIDQuery) (*Project, error) {
	return h.projectionStore.GetProject(ctx, query.ID)
}

func (h *ProjectQueryHandler) HandleListProjects(ctx context.Context, query ListProjectsQuery) ([]*Project, error) {
	return h.projectionStore.ListProjects(ctx)
}
