package shared

import (
	"context"
	"fmt"
)

// Command interface for all commands
// All commands must implement GetCommandType() string
// Example: func (c CreateIssueCommand) GetCommandType() string { return "CreateIssue" }
type Command interface {
	GetCommandType() string
}

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
func (b *CommandBus) Execute(ctx context.Context, command Command) error {
	commandType := command.GetCommandType()

	handler, exists := b.handlers[commandType]
	if !exists {
		return fmt.Errorf("no handler registered for command type: %s", commandType)
	}

	return handler.Handle(ctx, command)
}

// ProjectionStore defines the interface for applying events to projections
// Used by issues, notifications, and shared command handlers
type ProjectionStore interface {
	ApplyEvent(ctx context.Context, event Event) error
}

// ProjectionRebuilder interface for projection stores that can rebuild from events
// All projection stores should implement this interface
// Example: func (ps *NotificationProjectionStore) RebuildAllFromEvents(ctx context.Context, eventStore *EventStore) error
// Example: func (ps *IssueProjectionStore) RebuildAllFromEvents(ctx context.Context, eventStore *EventStore) error
type ProjectionRebuilder interface {
	RebuildAllFromEvents(ctx context.Context, eventStore *EventStore) error
}
