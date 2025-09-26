package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// CommandRegistry provides command creation and discovery using the existing CommandBus
type CommandRegistry struct {
	commandBus *CommandBus
	typeMap    map[string]reflect.Type // commandType -> reflect.Type
}

// NewCommandRegistry creates a registry that uses the existing CommandBus
func NewCommandRegistry(bus *CommandBus) *CommandRegistry {
	registry := &CommandRegistry{
		commandBus: bus,
		typeMap:    make(map[string]reflect.Type),
	}

	// Discover command types from registered handlers
	registry.discoverCommandTypes()

	return registry
}

// discoverCommandTypes uses reflection to find command types from handlers
func (r *CommandRegistry) discoverCommandTypes() {
	// Get registered command types from CommandBus
	commandTypes := r.commandBus.GetRegisteredCommandTypes()

	for _, commandType := range commandTypes {
		handler, exists := r.commandBus.GetHandler(commandType)
		if !exists {
			continue
		}

		// Get the handler type
		handlerType := reflect.TypeOf(handler)

		// Look for Handle method: func (h *Handler) Handle(ctx context.Context, cmd SomeCommand) error
		if method, exists := handlerType.MethodByName("Handle"); exists {
			if method.Type.NumIn() >= 3 { // receiver + context + command parameter
				cmdType := method.Type.In(2) // Third parameter is the command

				// Remove pointer if it's a pointer type
				if cmdType.Kind() == reflect.Ptr {
					cmdType = cmdType.Elem()
				}

				r.typeMap[commandType] = cmdType
			}
		}
	}
}

// CreateCommand creates a command instance from JSON data
func (r *CommandRegistry) CreateCommand(commandType string, jsonData []byte) (Command, error) {
	cmdType, exists := r.typeMap[commandType]
	if !exists {
		return nil, fmt.Errorf("unknown command type: %s", commandType)
	}

	// Create new instance
	cmdPtr := reflect.New(cmdType)
	cmdInterface := cmdPtr.Interface()

	// Unmarshal JSON into the command
	if err := json.Unmarshal(jsonData, cmdInterface); err != nil {
		return nil, fmt.Errorf("failed to unmarshal command %s: %w", commandType, err)
	}

	// Return the command (dereferenced if it was a pointer)
	cmd := cmdPtr.Elem().Interface()
	if command, ok := cmd.(Command); ok {
		return command, nil
	}

	return nil, fmt.Errorf("command %s does not implement Command interface", commandType)
}

// CreateCommandFromMap creates a command from a map (for form data)
func (r *CommandRegistry) CreateCommandFromMap(commandType string, data map[string]interface{}) (Command, error) {
	// Convert map to JSON first
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal map to JSON: %w", err)
	}

	return r.CreateCommand(commandType, jsonData)
}

// GetAvailableCommands returns all registered command types
func (r *CommandRegistry) GetAvailableCommands() []string {
	commands := make([]string, 0, len(r.typeMap))
	for cmdType := range r.typeMap {
		commands = append(commands, cmdType)
	}
	return commands
}

// GetCommandSchema returns the JSON schema/structure for a command type
func (r *CommandRegistry) GetCommandSchema(commandType string) (map[string]interface{}, error) {
	cmdType, exists := r.typeMap[commandType]
	if !exists {
		return nil, fmt.Errorf("unknown command type: %s", commandType)
	}

	schema := make(map[string]interface{})
	r.buildSchema(cmdType, schema)
	return schema, nil
}

// buildSchema builds a simple schema from a struct type
func (r *CommandRegistry) buildSchema(t reflect.Type, schema map[string]interface{}) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		fieldName := field.Name
		fieldType := field.Type

		// Handle pointer types
		optional := false
		if fieldType.Kind() == reflect.Ptr {
			optional = true
			fieldType = fieldType.Elem()
		}

		fieldInfo := map[string]interface{}{
			"type":     fieldType.Kind().String(),
			"optional": optional,
		}

		// Add JSON tag if present
		if jsonTag := field.Tag.Get("json"); jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				fieldName = parts[0]
			}
		}

		schema[fieldName] = fieldInfo
	}
}

// ExecuteCommand executes a command using the existing CommandBus
func (r *CommandRegistry) ExecuteCommand(ctx context.Context, cmd Command) error {
	return r.commandBus.Execute(ctx, cmd)
}
