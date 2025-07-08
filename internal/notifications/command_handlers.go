package notifications

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/internal/shared"
)

// CommandHandler handles all write operations
// NewCommandHandler creates a new command handler

// Concrete command handlers

type CreateNotificationHandler struct {
	handler *CommandHandler
}

var _ shared.CommandHandler = (*CreateNotificationHandler)(nil)

func NewCreateNotificationHandler(handler *CommandHandler) *CreateNotificationHandler {
	return &CreateNotificationHandler{
		handler: handler,
	}
}

func (h *CreateNotificationHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(CreateNotificationCommand)
	if !ok {
		return fmt.Errorf("invalid command type for CreateNotificationHandler")
	}
	return h.handler.HandleCreateNotification(ctx, cmd)
}

type DeleteNotificationHandler struct {
	handler *CommandHandler
}

var _ shared.CommandHandler = (*DeleteNotificationHandler)(nil)

func NewDeleteNotificationHandler(handler *CommandHandler) *DeleteNotificationHandler {
	return &DeleteNotificationHandler{
		handler: handler,
	}
}

func (h *DeleteNotificationHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(DeleteNotificationCommand)
	if !ok {
		return fmt.Errorf("invalid command type for DeleteNotificationHandler")
	}
	return h.handler.HandleDeleteNotification(ctx, cmd)
}

type MarkNotificationReadHandler struct {
	handler *CommandHandler
}

var _ shared.CommandHandler = (*MarkNotificationReadHandler)(nil)

func NewMarkNotificationReadHandler(handler *CommandHandler) *MarkNotificationReadHandler {
	return &MarkNotificationReadHandler{
		handler: handler,
	}
}

func (h *MarkNotificationReadHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(MarkNotificationReadCommand)
	if !ok {
		return fmt.Errorf("invalid command type for MarkNotificationReadHandler")
	}
	return h.handler.HandleMarkNotificationRead(ctx, cmd)
}
