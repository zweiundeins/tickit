# CQRS & Event Sourcing (ES)

This module implements CQRS (Command Query Responsibility Segregation) and Event Sourcing for Tickit, supporting both notifications and issues.

## Overview
- **Commands**: Write operations (create, update, delete)
- **Queries**: Read operations (get, list, count)
- **Events**: Domain events, persisted for event sourcing
- **Buses**: Generic command/query/event buses for routing
- **Projections**: Read models built from events
- **Real-time Updates**: JetStream integration for frontend push

## Architecture
```
HTTP/API → Application Service → Command/Query Bus → Handlers → Events → Event Store (SQLite: data/events.db)
                                                        ↓
                                                Projections/Read Models
                                                        ↓
                                                JetStream (push to frontend)
```

## Directory Structure
```
internal/shared/        # CQRS implementation
├── command_bus.go      # Command bus (generic)
├── query_bus.go        # Query bus (generic)
├── eventbus.go         # Event bus
├── eventstore.go       # Event store
├── event_handlers.go   # Event handlers
└── signals.go          # Signal handling

Domain modules implement their own handlers:
internal/issues/        # Issue domain
├── command_handlers.go # Issue command handlers
├── query_handlers.go   # Issue query handlers
├── commands.go         # Issue commands
├── queries.go          # Issue queries
└── events.go           # Issue events

internal/users/         # User domain
├── command_handlers.go # User command handlers
├── query_handlers.go   # User query handlers
├── commands.go         # User commands
├── queries.go          # User queries
└── events.go           # User events
```

## Event Sourcing
- All domain events are persisted to SQLite at `data/events.db` (`EventStore`).
- Projections/read models are rebuilt from events for efficient queries.
- Supports event replay, querying by type/aggregate/time, and versioning.
- Read models can be rebuilt at any time from the event log for consistency.

## Real-time Updates with JetStream
- When events are published (e.g., notification created), they are also pushed to JetStream.
- JetStream acts as a message broker, allowing the backend to push updates to subscribed frontend clients (e.g., via websockets or SSE).
- This enables real-time UI updates for notifications, issues, etc.
- Example: When a notification is created, the event is published to JetStream, and the frontend receives the update instantly.

## Usage Example
```go
// Initialize event store and buses (from bootstrap/deps.go)
eventStore := shared.NewEventStore(db)
eventBus := shared.NewEventBusWrapper(natsConn)
cmdBus := shared.NewCommandBus()
qryBus := shared.NewQueryBus()

// Register domain handlers
userHandler := users.NewUserCommandHandler(eventBus, eventStore, userStore, ...)
cmdBus.RegisterHandler("RegisterUser", userHandler.HandleRegisterUser)
cmdBus.RegisterHandler("LoginUser", userHandler.HandleLoginUser)

issueHandler := issues.NewIssueCommandHandler(eventBus, eventStore, issueStore, ...)
cmdBus.RegisterHandler("CreateIssue", issueHandler.HandleCreateIssue)

// Execute commands
cmd := users.RegisterUserCommand{
    Email: "user@example.com",
    Password: "password",
    // ...
}
err := cmdBus.Execute(ctx, cmd)
```

// Application service usage
svc := application.NewNotificationService(cmdBus, qryBus)
svc.CreateNotification(ctx, title, body)
```

## Features
- Separation of read/write logic for scalability and maintainability
- Async, type-safe event bus with multi-handler support
- Full event sourcing with SQLite persistence and replay (`data/events.db`)
- Projections for fast, query-optimized read models
- JetStream integration for real-time frontend updates
- Extensible: add new commands, queries, events, or projections easily

---
For details, see code and comments in each subdirectory. 