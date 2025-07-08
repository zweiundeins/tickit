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
internal/cqrs/
├── application/        # Application services (API layer)
├── bus/                # Command & Query buses (generic)
├── commands/           # Command handlers (notifications, issues)
├── events/             # Domain events, event bus, event store, projections
├── queries/            # Query handlers (notifications, issues)
└── README.md
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
// Initialize event store and buses
store, _ := events.NewEventStore(ctx, "data/events.db")
eventBus := events.NewEventBusWithHandlers(events.CreateDefaultEventHandlers())
cmdBus := shared.NewCommandBus()
qryBus := shared.NewQueryBus()

// Register handlers (notifications, issues)
cmdBus.RegisterHandler("CreateNotification", shared.NewCreateNotificationHandler(...))
cmdBus.RegisterHandler("CreateIssue", shared.NewCreateIssueHandler(...))
// ...

// Application service usage
svc := application.NewNotificationService(cmdBus, qryBus)
svc.CreateNotification(ctx, sessionID, title, body)
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