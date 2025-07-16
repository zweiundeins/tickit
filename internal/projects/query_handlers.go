package projects

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/internal/shared"
)

// Project query handlers

type ProjectGetByIDHandler struct {
	handler *ProjectQueryHandler
}

var _ shared.QueryHandler = (*ProjectGetByIDHandler)(nil)

func NewGetProjectByIDHandler(handler *ProjectQueryHandler) *ProjectGetByIDHandler {
	return &ProjectGetByIDHandler{handler: handler}
}

func (h *ProjectGetByIDHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(GetProjectByIDQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for ProjectGetByIDHandler")
	}
	return h.handler.HandleGetProjectByID(ctx, q)
}

type ProjectListHandler struct {
	handler *ProjectQueryHandler
}

var _ shared.QueryHandler = (*ProjectListHandler)(nil)

func NewListProjectsHandler(handler *ProjectQueryHandler) *ProjectListHandler {
	return &ProjectListHandler{handler: handler}
}

func (h *ProjectListHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	_, ok := query.(ListProjectsQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for ProjectListHandler")
	}
	return h.handler.HandleListProjects(ctx, ListProjectsQuery{})
}
