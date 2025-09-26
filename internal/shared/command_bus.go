package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

// Command interface for all commands
// All commands must implement GetCommandType() string
// Example: func (c CreateIssueCommand) GetCommandType() string { return "CreateIssue" }
type Command interface {
	GetCommandType() string
}

// CommandBus handles command routing and execution
type CommandBus struct {
	handlers  map[string]CommandHandler
	factories map[string]func() Command
}

// CommandHandler interface for all command handlers
type CommandHandler interface {
	Handle(ctx context.Context, command interface{}) error
}

// TypedCommandHandler interface for typed command handlers
type TypedCommandHandler[T any] interface {
	Handle(ctx context.Context, command T) error
}

// NewCommandBus creates a new command bus
func NewCommandBus() *CommandBus {
	return &CommandBus{
		handlers:  make(map[string]CommandHandler),
		factories: make(map[string]func() Command),
	}
}

// RegisterHandler registers a command handler for a specific command type
func (b *CommandBus) RegisterHandler(commandType string, handler CommandHandler) {
	b.handlers[commandType] = handler
}

// RegisterCommandFactory registers a factory function for creating command instances
func (b *CommandBus) RegisterCommandFactory(commandType string, factory func() Command) {
	b.factories[commandType] = factory
}

// RegisterTypedHandler registers a typed handler with automatic type conversion
func RegisterTypedHandler[T any](b *CommandBus, commandType string, handlerFunc func(context.Context, T) error) {
	wrapper := &typedHandlerWrapper[T]{
		handlerFunc: handlerFunc,
	}
	b.handlers[commandType] = wrapper
}

// RegisterCommandHandler registers both a typed handler and its factory in one call
func RegisterCommandHandler[T Command](b *CommandBus, commandType string, handlerFunc func(context.Context, T) error) {
	// Register the typed handler
	RegisterTypedHandler(b, commandType, handlerFunc)

	// Register the factory automatically
	b.RegisterCommandFactory(commandType, func() Command {
		var zero T
		// Create a pointer to T for proper JSON unmarshaling
		ptr := reflect.New(reflect.TypeOf(zero)).Interface()
		return ptr.(Command)
	})
}

// typedHandlerWrapper wraps a typed handler function to implement CommandHandler
type typedHandlerWrapper[T any] struct {
	handlerFunc func(context.Context, T) error
}

func (w *typedHandlerWrapper[T]) Handle(ctx context.Context, command interface{}) error {
	// Try pointer type first
	var cmd T
	if ptrCmd, ok := command.(*T); ok {
		cmd = *ptrCmd
	} else if valCmd, ok := command.(T); ok {
		cmd = valCmd
	} else {
		var zero T
		return fmt.Errorf("invalid command type, expected %T but got %T", zero, command)
	}

	return w.handlerFunc(ctx, cmd)
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

// GetRegisteredCommandTypes returns all registered command types
func (b *CommandBus) GetRegisteredCommandTypes() []string {
	types := make([]string, 0, len(b.handlers))
	for commandType := range b.handlers {
		types = append(types, commandType)
	}
	return types
}

// GetHandler returns the handler for a specific command type
func (b *CommandBus) GetHandler(commandType string) (CommandHandler, bool) {
	handler, exists := b.handlers[commandType]
	return handler, exists
}

// CreateCommand creates a command instance from JSON data
func (b *CommandBus) CreateCommand(commandType string, jsonData []byte) (Command, error) {
	factory, exists := b.factories[commandType]
	if !exists {
		return nil, fmt.Errorf("unknown command type: %s", commandType)
	}

	cmd := factory()
	if err := json.Unmarshal(jsonData, cmd); err != nil {
		return nil, fmt.Errorf("failed to unmarshal command %s: %w", commandType, err)
	}

	return cmd, nil
}

// CreateCommandFromMap creates a command from a map (for form data)
func (b *CommandBus) CreateCommandFromMap(commandType string, data map[string]interface{}) (Command, error) {
	// Convert map to JSON first
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal map to JSON: %w", err)
	}

	return b.CreateCommand(commandType, jsonData)
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
