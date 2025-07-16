package projects

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/internal/shared"
)

// Project command handlers

type CreateProjectHandler struct {
	handler *ProjectCommandHandler
}

var _ shared.CommandHandler = (*CreateProjectHandler)(nil)

func NewCreateProjectHandler(handler *ProjectCommandHandler) *CreateProjectHandler {
	return &CreateProjectHandler{handler: handler}
}

func (h *CreateProjectHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(CreateProjectCommand)
	if !ok {
		return fmt.Errorf("invalid command type for CreateProjectHandler")
	}
	_, err := h.handler.HandleCreateProject(ctx, cmd)
	return err
}

type UpdateProjectHandler struct {
	handler *ProjectCommandHandler
}

var _ shared.CommandHandler = (*UpdateProjectHandler)(nil)

func NewUpdateProjectHandler(handler *ProjectCommandHandler) *UpdateProjectHandler {
	return &UpdateProjectHandler{handler: handler}
}

func (h *UpdateProjectHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(UpdateProjectCommand)
	if !ok {
		return fmt.Errorf("invalid command type for UpdateProjectHandler")
	}
	return h.handler.HandleUpdateProject(ctx, cmd)
}

type DeleteProjectHandler struct {
	handler *ProjectCommandHandler
}

var _ shared.CommandHandler = (*DeleteProjectHandler)(nil)

func NewDeleteProjectHandler(handler *ProjectCommandHandler) *DeleteProjectHandler {
	return &DeleteProjectHandler{handler: handler}
}

func (h *DeleteProjectHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(DeleteProjectCommand)
	if !ok {
		return fmt.Errorf("invalid command type for DeleteProjectHandler")
	}
	return h.handler.HandleDeleteProject(ctx, cmd)
}
