# CQRS Implementation for TickIt

This directory contains the CQRS (Command Query Responsibility Segregation) implementation for the TickIt notification system.

## Overview

CQRS separates read and write operations, allowing each side to be optimized independently. This implementation provides:

- **Commands**: Handle all write operations (create, update, delete)
- **Queries**: Handle all read operations (get, list, count)
- **Events**: Domain events for future event sourcing
- **Buses**: Command and Query buses for routing
- **Application Services**: Clean API for the presentation layer

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   HTTP Layer    │    │  Application    │    │   CQRS Buses    │
│   (Routes)      │───▶│   Services      │───▶│  (Command/Query)│
└─────────────────┘    └─────────────────┘    └─────────────────┘
                                                       │
                                                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Domain        │    │   Event Bus     │    │   Storage       │
│   Models        │◀───│   (Toolbelt)    │◀───│   (NATS KV)     │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                                │
                                ▼
                       ┌─────────────────┐
                       │   Event         │
                       │   Handlers      │
                       │   (Logging,     │
                       │    Metrics,     │
                       │    Audit, etc.) │
                       └─────────────────┘
```

## Directory Structure

```
internal/cqrs/
├── commands/           # Command handlers for write operations
│   └── notification_commands.go
├── queries/            # Query handlers for read operations
│   └── notification_queries.go
├── events/             # Domain events, event bus, and event store
│   ├── notification_events.go
│   ├── event_bus.go
│   ├── event_handlers.go
│   ├── event_store.go
│   ├── read_model_builder.go
│   ├── example_usage.go
│   ├── event_sourcing_example.go
│   └── event_store_test.go
├── bus/                # Command and Query buses
│   ├── command_bus.go
│   └── query_bus.go
├── application/        # Application services
│   └── notification_service.go
└── README.md          # This file
```

## Components

### Commands (`commands/`)

Commands represent write operations and contain all the business logic for modifying data.

**Available Commands:**
- `CreateNotificationCommand`: Creates a new notification
- `DeleteNotificationCommand`: Deletes notification(s)
- `MarkNotificationReadCommand`: Marks notification(s) as read

**Example:**
```go
cmd := commands.CreateNotificationCommand{
    SessionID: "user123",
    Title:     "New Message",
    Body:      "You have a new message",
}

err := commandBus.Execute(ctx, cmd)
```

### Queries (`queries/`)

Queries represent read operations and can be optimized for specific use cases.

**Available Queries:**
- `GetNotificationsQuery`: Retrieves all notifications
- `GetUnreadCountQuery`: Retrieves only unread count (optimized)
- `GetNotificationByUIDQuery`: Retrieves specific notification

**Example:**
```go
query := queries.GetNotificationsQuery{
    SessionID: "user123",
}

result, err := queryBus.Execute(ctx, query)
notifications := result.(*queries.NotificationsResult)
```

### Events (`events/`)

Domain events and event bus implementation using `delaneyj/toolbelt`.

**Available Events:**
- `NotificationCreatedEvent`
- `NotificationDeletedEvent`
- `NotificationReadEvent`

**Event Bus Features:**
- **Type-safe**: Uses Go generics for type safety
- **Async**: Non-blocking event publishing
- **Flexible**: Subscribe to all events or specific event types
- **Extensible**: Easy to add new event handlers

**Built-in Event Handlers:**
- `LoggingEventHandler`: Logs all domain events
- `MetricsEventHandler`: Tracks metrics for events
- `AuditEventHandler`: Logs events for audit purposes
- `NotificationEventHandler`: Handles notification-specific events
- `EmailEventHandler`: Could send emails for certain events
- `WebhookEventHandler`: Could send webhooks to external systems
- `EventStoreHandler`: Persists events to SQLite event store

**Event Store Features:**
- **Persistence**: Events are automatically persisted to SQLite using toolbelt's Database
- **Event Sourcing**: Full event sourcing with event replay capabilities
- **Read Model Building**: Rebuild read models from events
- **Querying**: Query events by type, time range, or aggregate ID
- **Versioning**: Automatic versioning for event ordering

### Buses (`bus/`)

Command and Query buses handle routing and execution of commands and queries.

**Command Bus:**
```go
commandBus := bus.NewCommandBus()
commandBus.RegisterHandler("CreateNotification", handler)
err := commandBus.Execute(ctx, command)
```

**Query Bus:**
```go
queryBus := bus.NewQueryBus()
queryBus.RegisterHandler("GetNotifications", handler)
result, err := queryBus.Execute(ctx, query)
```

### Application Services (`application/`)

Application services provide a clean API for the presentation layer, coordinating between HTTP handlers and CQRS buses.

**Example:**
```go
service := application.NewNotificationService(commandBus, queryBus)

// Create notification
err := service.CreateNotification(ctx, sessionID, title, body)

// Get notifications
result, err := service.GetNotifications(ctx, sessionID)
```

## Usage

### 1. Initialize CQRS Components

```go
// Initialize event store
eventStore, err := events.NewEventStore(ctx, "data/events.db")
if err != nil {
    return fmt.Errorf("failed to create event store: %w", err)
}

// Initialize event bus with event store handler
eventHandlers := events.CreateDefaultEventHandlers()
eventHandlers["*"] = events.CreateEventStoreHandler(eventStore) // Persist all events
eventBus := events.NewEventBusWithHandlers(eventHandlers)

// Initialize handlers
commandHandler := commands.NewCommandHandler(kv, eventBus.EventBusWrapper)
queryHandler := queries.NewQueryHandler(kv)

// Initialize buses
commandBus := bus.NewCommandBus()
queryBus := bus.NewQueryBus()

// Register handlers
commandBus.RegisterHandler("CreateNotification", bus.NewCreateNotificationHandler(commandHandler))
queryBus.RegisterHandler("GetNotifications", bus.NewGetNotificationsHandler(queryHandler))

// Initialize application service
notificationService := application.NewNotificationService(commandBus, queryBus)
```

### 2. Use in HTTP Handlers

```go
// Create notification
func createNotification(w http.ResponseWriter, r *http.Request) {
    sessionID := getSessionID(r)
    title := r.FormValue("title")
    body := r.FormValue("body")
    
    err := notificationService.CreateNotification(r.Context(), sessionID, title, body)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    w.WriteHeader(http.StatusCreated)
}

// Get notifications
func getNotifications(w http.ResponseWriter, r *http.Request) {
    sessionID := getSessionID(r)
    
    result, err := notificationService.GetNotifications(r.Context(), sessionID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    // Return result as JSON
    json.NewEncoder(w).Encode(result)
}
```

## Benefits of This CQRS Implementation

### 1. **Separation of Concerns**
- Commands handle write operations
- Queries handle read operations
- Clear boundaries between read and write logic

### 2. **Scalability**
- Read and write operations can be scaled independently
- Query handlers can be optimized for specific use cases
- Command handlers can be optimized for consistency

### 3. **Maintainability**
- Clear structure makes code easier to understand
- Each component has a single responsibility
- Easy to add new commands or queries

### 4. **Testability**
- Each component can be tested in isolation
- Mock buses for testing HTTP handlers
- Clear interfaces for dependency injection

### 5. **Future-Proofing**
- Events are ready for event sourcing
- Easy to add new storage backends
- Can implement read models for complex queries

## Migration from Current Implementation

The current implementation in `routes_index.go` can be gradually migrated:

1. **Phase 1**: Use CQRS alongside existing code
2. **Phase 2**: Replace direct model calls with CQRS
3. **Phase 3**: Remove old implementation

### Example Migration

**Before (Current):**
```go
ntfs, err := models.GetNotifications(sessionID, r.Context(), kv)
ntfs.Notifications = append(ntfs.Notifications, &models.Notification{...})
models.SaveNotifications(r.Context(), kv, sessionID, ntfs)
```

**After (CQRS):**
```go
err := notificationService.CreateNotification(ctx, sessionID, title, body)
```

## Event Bus Integration

The CQRS implementation now includes a powerful event bus using `delaneyj/toolbelt`'s `EventBus`. This provides:

### Benefits of Using Toolbelt's EventBus

1. **Type Safety**: Uses Go generics for compile-time type checking
2. **Performance**: Async event publishing with non-blocking operations
3. **Flexibility**: Subscribe to all events or specific event types
4. **Reliability**: Built-in error handling and context support
5. **Integration**: Seamless integration with existing toolbelt ecosystem

### Event Publishing

Events are automatically published when commands are executed:

```go
// When a notification is created, this event is published:
event := events.NotificationCreatedEvent{
    SessionID:    "user123",
    Notification: notification,
    Timestamp:    time.Now(),
}
```

### Event Handling

Multiple handlers can process the same event:

```go
// Subscribe to all events
eventBus.SubscribeToEvents(ctx, func(event Event) error {
    // Handle all events
    return nil
})

// Subscribe to specific event type
eventBus.SubscribeToEventType(ctx, "NotificationCreated", func(event Event) error {
    // Handle only notification created events
    return nil
})
```

## Event Sourcing with Toolbelt Database

The CQRS implementation now includes full event sourcing using `delaneyj/toolbelt`'s `Database`:

### Event Store Features

1. **Automatic Persistence**: All domain events are automatically persisted to SQLite
2. **Event Replay**: Replay events to rebuild read models
3. **Querying**: Query events by type, time range, or aggregate ID
4. **Versioning**: Automatic versioning for event ordering
5. **Read Model Building**: Rebuild read models from events

### Event Sourcing Example

```go
// Initialize event store
eventStore, err := events.NewEventStore(ctx, "data/events.db")
if err != nil {
    return err
}

// Initialize read model builder
readModelBuilder := events.NewReadModelBuilder(eventStore)

// Rebuild notifications from events
notifications, err := readModelBuilder.BuildNotificationsFromEvents(ctx, sessionID)
if err != nil {
    return err
}

// Rebuild all read models
err = readModelBuilder.RebuildReadModels(ctx)
if err != nil {
    return err
}
```

### Event Store Queries

```go
// Get events for a session
events, err := eventStore.GetEvents(ctx, sessionID)

// Get events by type
createdEvents, err := eventStore.GetEventsByType(ctx, "NotificationCreated")

// Get events since a timestamp
recentEvents, err := eventStore.GetEventsSince(ctx, time.Now().Add(-1*time.Hour))

// Get total event count
count, err := eventStore.GetEventCount(ctx)
```

## Next Steps

1. **Read Model Optimization**: Create optimized read models for complex queries
2. **Caching**: Add caching layer for frequently accessed data
3. **Validation**: Add command validation middleware
4. **Logging**: Add structured logging for commands and queries
5. **Metrics**: Add metrics for command/query performance
6. **Event Streaming**: Stream events to external systems
7. **Event Archiving**: Archive old events for performance
8. **Event Schema Evolution**: Handle event schema changes over time

## Testing

Each component can be tested independently:

```go
func TestCreateNotificationCommand(t *testing.T) {
    // Test command handler
    handler := commands.NewCommandHandler(mockKV)
    cmd := commands.CreateNotificationCommand{...}
    err := handler.HandleCreateNotification(ctx, cmd)
    assert.NoError(t, err)
}

func TestNotificationService(t *testing.T) {
    // Test application service with mock buses
    service := application.NewNotificationService(mockCommandBus, mockQueryBus)
    err := service.CreateNotification(ctx, sessionID, title, body)
    assert.NoError(t, err)
}
``` 