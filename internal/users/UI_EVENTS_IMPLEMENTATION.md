# User Management UI Events Implementation

## Overview

This implementation adds real-time UI updates via JetStream for the user management system. The system publishes sanitized, non-sensitive events to JetStream that can be consumed by the frontend for real-time UI updates.

## Architecture

### Event Flow
```
Domain Event → UI Event Service → UI Event → JetStream → Frontend
```

1. **Domain Events**: Standard user management events (login, logout, role changes, etc.)
2. **UI Event Service**: Converts domain events to UI-specific events with sanitized data
3. **UI Events**: Lightweight events containing only non-sensitive data for UI consumption
4. **JetStream**: Message broker for real-time event delivery
5. **Frontend**: Consumes UI events for real-time updates

### Key Components

#### UI Events (`ui_events.go`)
- `UserStatusChangedEvent`: User status and online status changes
- `UserRoleChangedEvent`: Role assignment changes
- `UserLoginStatusEvent`: Login/logout events
- `UserProjectMembershipChangedEvent`: Project membership changes
- `UserMFAStatusChangedEvent`: MFA status changes
- `AdminUserManagementEvent`: Admin actions for real-time admin UI
- `UserProfileChangedEvent`: Profile updates (non-sensitive fields only)
- `UserInvitationStatusEvent`: Invitation status changes

#### UI Event Service (`ui_event_service.go`)
- Listens to domain events
- Converts domain events to UI events
- Sanitizes data (removes sensitive information)
- Publishes UI events to JetStream

#### Event Registration (`event_handlers.go`)
- `GetAllEventHandlers()`: Returns both projection and UI event handlers
- Composite handlers that call both projection and UI handlers

## Security Considerations

### Data Sanitization
- **No sensitive data**: Passwords, tokens, credentials are never included
- **User identification**: Only user ID and username are included
- **Minimal context**: Only necessary information for UI updates

### Event Types
- **Login/Logout**: User ID, username, method (not credentials)
- **Role Changes**: User ID, username, old/new roles, admin who made change
- **Profile Updates**: User ID, username, list of changed fields (not values)
- **MFA Status**: User ID, username, enabled/disabled status

## Integration

### Bootstrap Integration
The UI event handlers are registered in the bootstrap process:

```go
// Create event bus with handlers
eventBusWithHandlers := shared.NewEventBusWithHandlers(eventHandlers, js)

// Add user event handlers for both projection and UI updates
for k, v := range users.GetAllEventHandlers(userProjectionStore, eventBusWithHandlers, userStore) {
    eventBusWithHandlers.AddHandler(k, v)
}
```

### Event Handler Registration
UI event handlers are registered alongside projection handlers:

```go
func GetAllEventHandlers(projectionStore UserProjectionStore, eventBus EventPublisher, userStore UserStore) shared.EventHandlers {
    // Get projection handlers
    projectionHandlers := GetEventHandlers(projectionStore)
    
    // Get UI event handlers
    uiEventService := NewUIEventService(eventBus, userStore)
    uiHandlers := GetUIEventHandlers(uiEventService)
    
    // Create composite handlers that call both
    // ...
}
```

## Event Examples

### User Login
**Domain Event:**
```json
{
  "event_type": "UserLoggedIn",
  "user_id": "user-123",
  "session_id": "session-456",
  "ip_address": "192.168.1.1",
  "user_agent": "Mozilla/5.0...",
  "method": "password"
}
```

**UI Events Generated:**
```json
{
  "event_type": "UserLoginStatus",
  "user_id": "user-123",
  "username": "john_doe",
  "action": "login",
  "method": "password"
}
```

```json
{
  "event_type": "UserStatusChanged",
  "user_id": "user-123",
  "username": "john_doe",
  "status": "Active",
  "is_online": true
}
```

### Role Assignment
**Domain Event:**
```json
{
  "event_type": "UserRoleAssigned",
  "user_id": "user-123",
  "old_role": "Developer",
  "new_role": "ProjectManager",
  "assigned_by": "admin-456"
}
```

**UI Events Generated:**
```json
{
  "event_type": "UserRoleChanged",
  "user_id": "user-123",
  "username": "john_doe",
  "old_role": "Developer",
  "new_role": "ProjectManager",
  "changed_by": "admin_user"
}
```

```json
{
  "event_type": "AdminUserManagement",
  "admin_id": "admin-456",
  "admin_name": "admin_user",
  "target_id": "user-123",
  "target_name": "john_doe",
  "action": "role_changed",
  "details": "Role changed from Developer to ProjectManager"
}
```

## Frontend Integration

### JetStream Subscription
The frontend can subscribe to the unified "events" stream and filter by event types:

```javascript
// Subscribe to user-related UI events
const subscription = jetstream.subscribe('events', {
  filter: (event) => event.event_type.startsWith('User') || event.event_type === 'AdminUserManagement'
});

subscription.on('message', (event) => {
  switch (event.event_type) {
    case 'UserLoginStatus':
      updateUserOnlineStatus(event);
      break;
    case 'UserRoleChanged':
      updateUserRole(event);
      break;
    case 'AdminUserManagement':
      updateAdminDashboard(event);
      break;
    // ... other event types
  }
});
```

### Real-time UI Updates
- **User Lists**: Update online status, roles, and status in real-time
- **Admin Dashboard**: Show real-time user management actions
- **Project Member Lists**: Update membership and roles
- **User Profiles**: Show real-time profile changes
- **Security Dashboard**: Show MFA status changes

## Testing

### Unit Tests (`ui_event_service_test.go`)
- Tests individual event handler methods
- Verifies correct UI event generation
- Ensures sensitive data is filtered out

### Integration Tests (`ui_integration_test.go`)
- Tests complete event flow from domain event to UI event
- Verifies event data correctness
- Tests multiple event scenarios

### Test Coverage
- Login/logout event handling
- Role assignment event handling
- MFA status change handling
- Profile update handling (with sensitive data filtering)
- Error handling for missing users

## Performance Considerations

### Event Volume
- UI events are lightweight and contain minimal data
- Events are published asynchronously
- Failed UI event publishing doesn't affect domain operations

### Memory Usage
- UI events are ephemeral (stored in JetStream memory)
- No persistent storage of UI events
- Automatic cleanup of expired events

### Network Traffic
- Events contain only necessary data for UI updates
- JSON serialization is efficient
- Events are published to a single JetStream stream

## Monitoring and Debugging

### Logging
- All UI event publishing is logged at debug level
- Errors in UI event handling are logged but don't fail domain operations
- Event type and user ID are included in logs

### Metrics
- Event publishing success/failure rates
- Event processing latency
- JetStream message queue depth

### Debugging
- UI events can be inspected via JetStream monitoring
- Event collector in tests allows verification of event generation
- Separate logging for UI events vs domain events

## Future Enhancements

### Additional Event Types
- User session management events
- OAuth provider linking/unlinking
- Password policy compliance events
- Account lockout/unlock events

### Enhanced Filtering
- Project-specific event filtering
- Role-based event filtering
- User preference-based event filtering

### Performance Optimizations
- Event batching for high-volume scenarios
- Compression for large event payloads
- Event deduplication for rapid state changes

## Configuration

### Environment Variables
- `JETSTREAM_ENABLED`: Enable/disable JetStream publishing
- `UI_EVENTS_ENABLED`: Enable/disable UI event generation
- `EVENT_LOG_LEVEL`: Logging level for event processing

### JetStream Configuration
- Stream: "events"
- Storage: Memory
- Retention: Limits policy
- Max bytes: 16MB (configurable)

This implementation provides a robust foundation for real-time UI updates while maintaining security and performance standards.