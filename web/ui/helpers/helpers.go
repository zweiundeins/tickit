package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/mbolli/tickit/internal/notifications"
	"github.com/mbolli/tickit/internal/shared"
)

// EmptyNotifications returns an empty notifications structure for initial rendering
func EmptyNotifications() *notifications.Notifications {
	return &notifications.Notifications{
		Notifications: []*notifications.Notification{},
	}
}

// Command generates a datastar action script for executing a command struct
// Usage: data-on-click={ Command(DeleteNotificationCommand{UserID: userID, NotificationUID: notificationUID}) }
// Returns: "$command = 'DeleteNotification'; $commandParams.user_id='123'; $commandParams.notification_uid='456'; @post('/command'); $command = null; $commandParams = null;"
func Command(ctx context.Context, cmd shared.Command) string {
	var script strings.Builder

	// Get CSRF token from context
	csrf, ok := ctx.Value("csrf_token").(string)
	if !ok || csrf == "" {
		// Fallback: return empty script if no CSRF token is found
		return ""
	}

	// Get command type from the command struct
	commandType := cmd.GetCommandType()
	script.WriteString(fmt.Sprintf("$command = '%s';", commandType))

	// Convert command struct to map using JSON marshaling for proper field mapping
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		// Fallback: use reflection
		return commandWithReflection(ctx, cmd)
	}

	var commandData map[string]interface{}
	if err := json.Unmarshal(cmdBytes, &commandData); err != nil {
		// Fallback: use reflection
		return commandWithReflection(ctx, cmd)
	}

	// Set command parameters
	for key, value := range commandData {
		// Skip nil/empty values
		if value == nil {
			script.WriteString(fmt.Sprintf(" $commandParams.%s=null;", key))
			continue
		}

		switch v := value.(type) {
		case string:
			if v != "" {
				script.WriteString(fmt.Sprintf(" $commandParams.%s='%s';", key, v))
			}
		case float64: // JSON unmarshals numbers as float64
			// Check if it's actually an integer
			if v == float64(int(v)) {
				script.WriteString(fmt.Sprintf(" $commandParams.%s=%d;", key, int(v)))
			} else {
				script.WriteString(fmt.Sprintf(" $commandParams.%s=%f;", key, v))
			}
		case bool:
			script.WriteString(fmt.Sprintf(" $commandParams.%s=%t;", key, v))
		default:
			// For complex types, convert to string
			if str := fmt.Sprintf("%v", v); str != "" && str != "<nil>" {
				script.WriteString(fmt.Sprintf(" $commandParams.%s='%v';", key, v))
			}
		}
	}

	// Execute command and cleanup
	script.WriteString(" console.log($command, $commandParams); @post('/command', { headers: { 'X-CSRF-Token': '" + csrf + "' } }); $command = null; $commandParams = null;")

	return script.String()
}

// commandWithReflection is a fallback that uses reflection when JSON marshaling fails
func commandWithReflection(ctx context.Context, cmd shared.Command) string {
	// Get CSRF token from context
	csrf, ok := ctx.Value("csrf_token").(string)
	if !ok || csrf == "" {
		// Fallback: return empty script if no CSRF token is found
		return ""
	}

	var script strings.Builder

	commandType := cmd.GetCommandType()
	script.WriteString(fmt.Sprintf("$command = '%s';", commandType))

	// Use reflection to get struct fields
	val := reflect.ValueOf(cmd)
	typ := reflect.TypeOf(cmd)

	// Handle pointer types
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
		typ = typ.Elem()
	}

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		fieldValue := val.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get field name from JSON tag or use field name
		fieldName := field.Name
		if jsonTag := field.Tag.Get("json"); jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				fieldName = parts[0]
			}
		}

		// Skip empty/nil values
		if !fieldValue.IsValid() || fieldValue.IsZero() {
			continue
		}

		switch fieldValue.Kind() {
		case reflect.String:
			if str := fieldValue.String(); str != "" {
				script.WriteString(fmt.Sprintf(" $commandParams.%s='%s';", fieldName, str))
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if num := fieldValue.Int(); num != 0 {
				script.WriteString(fmt.Sprintf(" $commandParams.%s=%d;", fieldName, num))
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if num := fieldValue.Uint(); num != 0 {
				script.WriteString(fmt.Sprintf(" $commandParams.%s=%d;", fieldName, num))
			}
		case reflect.Float32, reflect.Float64:
			if num := fieldValue.Float(); num != 0 {
				script.WriteString(fmt.Sprintf(" $commandParams.%s=%f;", fieldName, num))
			}
		case reflect.Bool:
			script.WriteString(fmt.Sprintf(" $commandParams.%s=%t;", fieldName, fieldValue.Bool()))
		default:
			// For other types, convert to string
			if str := fmt.Sprintf("%v", fieldValue.Interface()); str != "" && str != "<nil>" {
				script.WriteString(fmt.Sprintf(" $commandParams.%s='%s';", fieldName, str))
			}
		}
	}

	// Execute command and cleanup
	script.WriteString(" @post('/command', { headers: { 'X-CSRF-Token': '" + csrf + "' } }); $command = null; $commandParams = null;")

	return script.String()
}
