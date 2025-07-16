package issues

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/internal/shared"
)

// Issue command handlers

type CreateIssueHandler struct {
	handler *IssueCommandHandler
}

var _ shared.CommandHandler = (*CreateIssueHandler)(nil)

func NewCreateIssueHandler(handler *IssueCommandHandler) *CreateIssueHandler {
	return &CreateIssueHandler{
		handler: handler,
	}
}

func (h *CreateIssueHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(CreateIssueCommand)
	if !ok {
		return fmt.Errorf("invalid command type for CreateIssueHandler")
	}
	_, err := h.handler.HandleCreateIssue(ctx, cmd)
	return err
}

type UpdateIssueHandler struct {
	handler *IssueCommandHandler
}

var _ shared.CommandHandler = (*UpdateIssueHandler)(nil)

func NewUpdateIssueHandler(handler *IssueCommandHandler) *UpdateIssueHandler {
	return &UpdateIssueHandler{
		handler: handler,
	}
}

func (h *UpdateIssueHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(UpdateIssueCommand)
	if !ok {
		return fmt.Errorf("invalid command type for UpdateIssueHandler")
	}
	return h.handler.HandleUpdateIssue(ctx, cmd)
}

type DeleteIssueHandler struct {
	handler *IssueCommandHandler
}

var _ shared.CommandHandler = (*DeleteIssueHandler)(nil)

func NewDeleteIssueHandler(handler *IssueCommandHandler) *DeleteIssueHandler {
	return &DeleteIssueHandler{
		handler: handler,
	}
}

func (h *DeleteIssueHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(DeleteIssueCommand)
	if !ok {
		return fmt.Errorf("invalid command type for DeleteIssueHandler")
	}
	return h.handler.HandleDeleteIssue(ctx, cmd)
}

type PatchIssueHandler struct {
	handler *IssueCommandHandler
}

var _ shared.CommandHandler = (*PatchIssueHandler)(nil)

func NewPatchIssueHandler(handler *IssueCommandHandler) *PatchIssueHandler {
	return &PatchIssueHandler{
		handler: handler,
	}
}

func (h *PatchIssueHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(PatchIssueCommand)
	if !ok {
		return fmt.Errorf("invalid command type for PatchIssueHandler")
	}
	return h.handler.HandlePatchIssue(ctx, cmd)
}
