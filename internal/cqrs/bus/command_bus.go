package bus

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/internal/cqrs/commands"
)

// CommandBus handles command routing and execution
type CommandBus struct {
	handlers map[string]CommandHandler
}

// CommandHandler interface for all command handlers
type CommandHandler interface {
	Handle(ctx context.Context, command interface{}) error
}

// NewCommandBus creates a new command bus
func NewCommandBus() *CommandBus {
	return &CommandBus{
		handlers: make(map[string]CommandHandler),
	}
}

// RegisterHandler registers a command handler for a specific command type
func (b *CommandBus) RegisterHandler(commandType string, handler CommandHandler) {
	b.handlers[commandType] = handler
}

// Execute dispatches a command to its appropriate handler
func (b *CommandBus) Execute(ctx context.Context, command interface{}) error {
	commandType := getCommandType(command)

	handler, exists := b.handlers[commandType]
	if !exists {
		return fmt.Errorf("no handler registered for command type: %s", commandType)
	}

	return handler.Handle(ctx, command)
}

// getCommandType returns the type name of a command
func getCommandType(command interface{}) string {
	switch command.(type) {
	case commands.CreateNotificationCommand:
		return "CreateNotification"
	case commands.DeleteNotificationCommand:
		return "DeleteNotification"
	case commands.MarkNotificationReadCommand:
		return "MarkNotificationRead"
	case commands.CreateIssueCommand:
		return "CreateIssue"
	case commands.UpdateIssueCommand:
		return "UpdateIssue"
	case commands.DeleteIssueCommand:
		return "DeleteIssue"
	default:
		return fmt.Sprintf("%T", command)
	}
}

// Concrete command handlers

type CreateNotificationHandler struct {
	handler *commands.CommandHandler
}

func NewCreateNotificationHandler(handler *commands.CommandHandler) *CreateNotificationHandler {
	return &CreateNotificationHandler{
		handler: handler,
	}
}

func (h *CreateNotificationHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(commands.CreateNotificationCommand)
	if !ok {
		return fmt.Errorf("invalid command type for CreateNotificationHandler")
	}
	return h.handler.HandleCreateNotification(ctx, cmd)
}

type DeleteNotificationHandler struct {
	handler *commands.CommandHandler
}

func NewDeleteNotificationHandler(handler *commands.CommandHandler) *DeleteNotificationHandler {
	return &DeleteNotificationHandler{
		handler: handler,
	}
}

func (h *DeleteNotificationHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(commands.DeleteNotificationCommand)
	if !ok {
		return fmt.Errorf("invalid command type for DeleteNotificationHandler")
	}
	return h.handler.HandleDeleteNotification(ctx, cmd)
}

type MarkNotificationReadHandler struct {
	handler *commands.CommandHandler
}

func NewMarkNotificationReadHandler(handler *commands.CommandHandler) *MarkNotificationReadHandler {
	return &MarkNotificationReadHandler{
		handler: handler,
	}
}

func (h *MarkNotificationReadHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(commands.MarkNotificationReadCommand)
	if !ok {
		return fmt.Errorf("invalid command type for MarkNotificationReadHandler")
	}
	return h.handler.HandleMarkNotificationRead(ctx, cmd)
}

// Issue command handlers

type IssueCreateHandler struct {
	handler *commands.IssueCommandHandler
}

func NewIssueCreateHandler(handler *commands.IssueCommandHandler) *IssueCreateHandler {
	return &IssueCreateHandler{handler: handler}
}

func (h *IssueCreateHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(commands.CreateIssueCommand)
	if !ok {
		return fmt.Errorf("invalid command type for IssueCreateHandler")
	}
	_, err := h.handler.HandleCreateIssue(ctx, cmd)
	return err
}

type IssueUpdateHandler struct {
	handler *commands.IssueCommandHandler
}

func NewIssueUpdateHandler(handler *commands.IssueCommandHandler) *IssueUpdateHandler {
	return &IssueUpdateHandler{handler: handler}
}

func (h *IssueUpdateHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(commands.UpdateIssueCommand)
	if !ok {
		return fmt.Errorf("invalid command type for IssueUpdateHandler")
	}
	return h.handler.HandleUpdateIssue(ctx, cmd)
}

type IssueDeleteHandler struct {
	handler *commands.IssueCommandHandler
}

func NewIssueDeleteHandler(handler *commands.IssueCommandHandler) *IssueDeleteHandler {
	return &IssueDeleteHandler{handler: handler}
}

func (h *IssueDeleteHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(commands.DeleteIssueCommand)
	if !ok {
		return fmt.Errorf("invalid command type for IssueDeleteHandler")
	}
	return h.handler.HandleDeleteIssue(ctx, cmd)
}
